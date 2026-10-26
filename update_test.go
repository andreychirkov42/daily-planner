package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// fakeRelease раздаёт latest.json с одной сборкой для updatePlatform-ключа "windows-x86_64",
// подпись манифеста ключом signer (nil — ключом, который тест делает ключом обновлений)
// и сам файл. sum — какая SHA-256 записана в манифест.
func fakeRelease(t *testing.T, version string, file []byte, sum string, signer ed25519.PrivateKey) {
	t.Helper()
	pub, own, _ := ed25519.GenerateKey(rand.Reader)
	old := updatePubKey
	updatePubKey = hex.EncodeToString(pub)
	t.Cleanup(func() { updatePubKey = old })
	if signer == nil {
		signer = own
	}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	manifest, _ := json.Marshal(Manifest{
		Version: version, Notes: "### Новое\n\n- что-то",
		Platforms: map[string]PlatformBuild{"windows-x86_64": {URL: srv.URL + "/Clepsy.exe", SHA256: sum}},
	})
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(manifest) })
	mux.HandleFunc("/latest.json.sig", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, hex.EncodeToString(ed25519.Sign(signer, manifest)))
	})
	mux.HandleFunc("/Clepsy.exe", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(file) })
	t.Setenv("CLEPSY_UPDATE_URL", srv.URL+"/latest.json")
}

func TestFetchBuild(t *testing.T) {
	file := []byte("новая версия")
	fakeRelease(t, "9.0.0", file, sha(file), nil)
	m, err := fetchManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Notes == "" {
		t.Error("описание изменений должно прийти из манифеста")
	}
	b, err := fetchBuild(context.Background(), m, "windows-x86_64")
	if err != nil || string(b) != string(file) {
		t.Fatalf("скачали %q, %v", b, err)
	}
	if _, err := fetchBuild(context.Background(), m, "linux-arm"); err == nil {
		t.Error("для системы без сборки — ошибка, а не паника")
	}
}

func TestFetchBuildRejectsWrongSum(t *testing.T) {
	fakeRelease(t, "9.0.0", []byte("подменённый файл"), strings.Repeat("0", 64), nil)
	m, _ := fetchManifest(context.Background())
	if _, err := fetchBuild(context.Background(), m, "windows-x86_64"); err == nil {
		t.Fatal("файл с неверной контрольной суммой нельзя ставить")
	}
}

// Взломанный GitHub: манифест и файл подменены согласованно, но подписать манифест ключом автора нельзя.
func TestManifestRejectsForeignSignature(t *testing.T) {
	file := []byte("вредонос")
	_, attacker, _ := ed25519.GenerateKey(rand.Reader)
	fakeRelease(t, "9.0.0", file, sha(file), attacker)
	if _, err := fetchManifest(context.Background()); err == nil || !strings.Contains(err.Error(), "подпись") {
		t.Fatalf("чужая подпись должна отклоняться: %v", err)
	}
}

func TestUpdaterCheck(t *testing.T) {
	s := newTestStore(t)
	fakeRelease(t, appVersion, []byte("x"), "y", nil)
	u := newUpdater(s, nil, "")
	_ = u.check(context.Background())
	if st := u.state(); st.Available || st.State != "" || st.Latest != appVersion || st.Notes == "" {
		t.Fatalf("та же версия — обновления нет, но «что нового» есть: %+v", st)
	}
	if !u.autoOn() {
		t.Fatal("автообновление по умолчанию включено")
	}
}

// Пока через API что-то меняют (окно закрыто, но Clepsy открыт во вкладке браузера) — не перезапускаться.
func TestIdleWaitsForRecentEdits(t *testing.T) {
	u := newUpdater(newTestStore(t), nil, "")
	old := lastEdit.Load()
	t.Cleanup(func() { lastEdit.Store(old) })
	lastEdit.Store(time.Now().UnixNano())
	if u.idle() {
		t.Fatal("только что правили задачу — программа занята")
	}
	lastEdit.Store(time.Now().Add(-idleAfter - time.Minute).UnixNano())
	if !u.idle() {
		t.Fatal("давно ничего не меняли и окон нет — можно обновляться")
	}
}
