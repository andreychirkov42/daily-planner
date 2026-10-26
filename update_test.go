package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.1", "1.2.0", true},
		{"v1.10.0", "1.9.9", true},
		{"v2.0.0", "1.99.99", true},
		{"v1.2.0", "1.2.0", false},
		{"v1.1.9", "1.2.0", false},
		{"v1.2.0", "1.2.0-beta.1", true}, // релиз новее своей беты
		{"v1.2.0-beta.1", "1.2.0", false},
		{"v1.2.0-beta.2", "1.2.0-beta.1", true},
		{"мусор", "1.2.0", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// fakeGitHub отдаёт «последний релиз» с файлом, SHA256SUMS.txt (sum — что в нём написано)
// и подписью сумм ключом signer (nil — ключом, который тест делает ключом обновлений).
func fakeGitHub(t *testing.T, tag string, file []byte, sum string, signer ed25519.PrivateKey) {
	t.Helper()
	pub, own, _ := ed25519.GenerateKey(rand.Reader)
	old := updatePubKey
	updatePubKey = hex.EncodeToString(pub)
	t.Cleanup(func() { updatePubKey = old })
	if signer == nil {
		signer = own
	}
	sums := fmt.Sprintf("%s  Clepsy.exe\nabc  Clepsy.apk\n", sum)
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"Clepsy.exe","browser_download_url":"%[2]s/exe"},`+
			`{"name":"SHA256SUMS.txt","browser_download_url":"%[2]s/sums"},{"name":"SHA256SUMS.txt.sig","browser_download_url":"%[2]s/sig"}]}`, tag, srv.URL)
	})
	mux.HandleFunc("/exe", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(file) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sums) })
	mux.HandleFunc("/sig", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, hex.EncodeToString(ed25519.Sign(signer, []byte(sums))))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("CLEPSY_UPDATE_URL", srv.URL+"/latest")
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchVerified(t *testing.T) {
	file := []byte("новая версия")
	fakeGitHub(t, "v9.0.0", file, sha(file), nil)
	r, err := fetchLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := fetchVerified(context.Background(), r, "Clepsy.exe")
	if err != nil || string(b) != string(file) {
		t.Fatalf("скачали %q, %v", b, err)
	}
}

func TestFetchVerifiedRejectsWrongSum(t *testing.T) {
	fakeGitHub(t, "v9.0.0", []byte("подменённый файл"), strings.Repeat("0", 64), nil)
	r, _ := fetchLatest(context.Background())
	if _, err := fetchVerified(context.Background(), r, "Clepsy.exe"); err == nil {
		t.Fatal("файл с неверной контрольной суммой нельзя ставить")
	}
}

// Взломанный GitHub: файл и суммы подменены согласованно, но подписать их ключом автора нельзя.
func TestFetchVerifiedRejectsForeignSignature(t *testing.T) {
	file := []byte("вредонос")
	_, attacker, _ := ed25519.GenerateKey(rand.Reader)
	fakeGitHub(t, "v9.0.0", file, sha(file), attacker)
	r, _ := fetchLatest(context.Background())
	if _, err := fetchVerified(context.Background(), r, "Clepsy.exe"); err == nil || !strings.Contains(err.Error(), "подпись") {
		t.Fatalf("чужая подпись должна отклоняться: %v", err)
	}
}

func TestUpdaterCheck(t *testing.T) {
	s := newTestStore(t)
	fakeGitHub(t, "v"+appVersion, []byte("x"), "y", nil)
	u := newUpdater(s, nil, "")
	_ = u.check(context.Background())
	if st := u.state(); st.Available || st.State != "" || st.Latest != appVersion {
		t.Fatalf("та же версия — обновления нет: %+v", st)
	}
	if !u.autoOn() {
		t.Fatal("автообновление по умолчанию включено")
	}
}
