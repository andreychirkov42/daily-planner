package main

// Автообновление. Раз в 6 часов Clepsy спрашивает у GitHub последний релиз; если версия новее,
// скачивает файл для своей системы, сверяет его с SHA256SUMS.txt того же релиза и ставит:
// на Windows — подменяет exe и перезапускается (update_windows.go), когда программой не пользуются.
// На Android проверку и установку делает оболочка на Java: у Go-сервера на телефоне нет DNS.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	releasesAPI    = "https://api.github.com/repos/andreychirkov42/daily-planner/releases/latest"
	updateEvery    = 6 * time.Hour
	maxUpdateBytes = 200 << 20
)

// updatePubKey — открытый ключ подписи релизов (tools/signsums). SHA256SUMS.txt должен быть подписан
// закрытым ключом, который хранится только у автора (не на GitHub): даже со взломанным аккаунтом GitHub
// подменённый exe не поставится. Переменная — чтобы тесты подставили свой ключ.
var updatePubKey = "4c249d0e7f5811889d3a917287edb99ccb5591d92c1914d971679d2e31168ff4"

// updateAPI — адрес «последнего релиза»; в тестах подменяется на свой сервер.
func updateAPI() string {
	if u := os.Getenv("CLEPSY_UPDATE_URL"); u != "" {
		return u
	}
	return releasesAPI
}

type release struct {
	Tag     string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func (r *release) asset(name string) (string, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

// parseVersion разбирает «v1.2.0-beta.1» на числа и метку предварительной версии.
func parseVersion(v string) (nums [3]int, pre string, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}

// newerVersion — новее ли a, чем b. Обычная версия новее своей беты: 1.2.0 > 1.2.0-beta.1.
func newerVersion(a, b string) bool {
	na, pa, okA := parseVersion(a)
	nb, pb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range na {
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	switch {
	case pa == pb:
		return false
	case pa == "":
		return true
	case pb == "":
		return false
	}
	return pa > pb
}

// sumFor находит хэш файла в SHA256SUMS.txt («<hex>  <имя>»).
func sumFor(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

var updateClient = &http.Client{Timeout: 5 * time.Minute}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Clepsy/"+appVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := updateClient.Do(req)
	if err != nil {
		return nil, errors.New("нет связи с GitHub")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub ответил %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, errors.New("загрузка прервалась")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("файл слишком большой")
	}
	return b, nil
}

func fetchLatest(ctx context.Context) (*release, error) {
	b, err := httpGet(ctx, updateAPI(), 1<<20)
	if err != nil {
		return nil, err
	}
	var r release
	if err := json.Unmarshal(b, &r); err != nil || r.Tag == "" {
		return nil, errors.New("непонятный ответ GitHub")
	}
	return &r, nil
}

// verifySums проверяет подпись SHA256SUMS.txt ключом автора.
func verifySums(sums, sigHex []byte) bool {
	pub, err1 := hex.DecodeString(updatePubKey)
	sig, err2 := hex.DecodeString(strings.TrimSpace(string(sigHex)))
	return err1 == nil && err2 == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, sums, sig)
}

// fetchVerified скачивает файл релиза и проверяет его по SHA256SUMS.txt того же релиза,
// а сам SHA256SUMS.txt — по подписи автора (SHA256SUMS.txt.sig).
func fetchVerified(ctx context.Context, r *release, name string) ([]byte, error) {
	sumsURL, ok1 := r.asset("SHA256SUMS.txt")
	sigURL, ok2 := r.asset("SHA256SUMS.txt.sig")
	fileURL, ok3 := r.asset(name)
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("в релизе %s нет %s, SHA256SUMS.txt или подписи — обнови вручную со страницы релизов", r.Tag, name)
	}
	sums, err := httpGet(ctx, sumsURL, 64<<10)
	if err != nil {
		return nil, err
	}
	sig, err := httpGet(ctx, sigURL, 1<<10)
	if err != nil {
		return nil, err
	}
	if !verifySums(sums, sig) {
		return nil, errors.New("подпись релиза не сошлась — файл не от автора Clepsy, не устанавливаю")
	}
	want, ok := sumFor(string(sums), name)
	if !ok {
		return nil, fmt.Errorf("в SHA256SUMS.txt нет %s", name)
	}
	b, err := httpGet(ctx, fileURL, maxUpdateBytes)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(b)
	if hex.EncodeToString(got[:]) != want {
		return nil, errors.New("контрольная сумма не совпала — файл повреждён или подменён, не устанавливаю")
	}
	return b, nil
}

// UpdateState — что показывает раздел «Настройки → Обновления».
type UpdateState struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest,omitempty"`
	Available   bool      `json:"available"`
	State       string    `json:"state"` // "", checking, downloading, ready, installing, error
	Err         string    `json:"error,omitempty"`
	Checked     time.Time `json:"checked,omitzero"`
	UpdatedFrom string    `json:"updatedFrom,omitempty"` // с какой версии только что обновились
	CanInstall  bool      `json:"canInstall"`            // умеет ли эта сборка ставить обновление сама
}

type updater struct {
	store *Store
	pomo  *Pomodoro

	mu       sync.Mutex
	st       UpdateState
	ready    []byte // скачанный и проверенный файл новой версии
	readyFor string // какой версии этот файл
}

func newUpdater(s *Store, p *Pomodoro, updatedFrom string) *updater {
	return &updater{store: s, pomo: p, st: UpdateState{Current: appVersion, UpdatedFrom: updatedFrom, CanInstall: updateAsset != ""}}
}

func (u *updater) state() UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

func (u *updater) set(f func(*UpdateState)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	f(&u.st)
}

// check узнаёт последнюю версию и, если она новее и эта сборка умеет ставить, скачивает её.
func (u *updater) check(ctx context.Context) error {
	u.mu.Lock()
	if u.st.State == "checking" || u.st.State == "downloading" || u.st.State == "installing" {
		u.mu.Unlock()
		return nil
	}
	u.st.State, u.st.Err = "checking", ""
	u.mu.Unlock()

	err := func() error {
		r, err := fetchLatest(ctx)
		if err != nil {
			return err
		}
		avail := newerVersion(r.Tag, appVersion)
		u.set(func(s *UpdateState) {
			s.Latest, s.Available, s.Checked = strings.TrimPrefix(r.Tag, "v"), avail, time.Now()
		})
		if !avail || updateAsset == "" {
			return nil
		}
		u.mu.Lock()
		have := u.ready != nil && u.readyFor == r.Tag // уже скачана именно эта версия, а не прошлая
		u.mu.Unlock()
		if have {
			return nil
		}
		u.set(func(s *UpdateState) { s.State = "downloading" })
		b, err := fetchVerified(ctx, r, updateAsset)
		if err != nil {
			return err
		}
		u.mu.Lock()
		u.ready, u.readyFor = b, r.Tag
		u.mu.Unlock()
		log.Printf("обновление %s скачано и проверено", r.Tag)
		return nil
	}()
	u.set(func(s *UpdateState) {
		switch {
		case err != nil:
			s.State, s.Err = "error", err.Error()
		case s.Available && updateAsset != "":
			s.State = "ready"
		default:
			s.State = ""
		}
	})
	return err
}

// install ставит скачанную версию и перезапускает программу.
func (u *updater) install() error {
	u.mu.Lock()
	b := u.ready
	if b == nil {
		u.mu.Unlock()
		return errors.New("обновление ещё не скачано")
	}
	u.st.State = "installing"
	u.mu.Unlock()
	log.Printf("ставлю обновление %s → %s", appVersion, u.state().Latest)
	if err := installUpdate(b); err != nil {
		u.set(func(s *UpdateState) { s.State, s.Err = "error", err.Error() })
		return err
	}
	restartAfterUpdate()
	return nil
}

func (u *updater) autoOn() bool { return !u.store.AppSettings().NoAutoUpdate }

// idle — программой сейчас не пользуются: окна закрыты и таймер не идёт.
func (u *updater) idle() bool {
	if u.pomo != nil && u.pomo.State().Running {
		return false
	}
	return appBusy == nil || !appBusy()
}

// loop — фоновая проверка: вскоре после запуска, потом раз в 6 часов; скачанное ставится,
// как только программа освободится (проверяем каждые 5 минут).
func (u *updater) loop() {
	time.Sleep(time.Minute)
	for {
		if u.autoOn() {
			if st := u.state(); time.Since(st.Checked) > updateEvery || st.State == "error" && time.Since(st.Checked) > time.Hour {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				if err := u.check(ctx); err != nil {
					log.Printf("проверка обновлений: %v", err)
					u.set(func(s *UpdateState) { s.Checked = time.Now() }) // не долбим GitHub при ошибке
				}
				cancel()
			}
			if u.state().State == "ready" && u.idle() {
				if err := u.install(); err != nil {
					log.Printf("обновление: %v", err)
				}
			}
		}
		time.Sleep(5 * time.Minute)
	}
}

// appBusy — открыто ли окно или мини-панель (задаёт оболочка Windows); nil — не знаем, считаем свободной.
var appBusy func() bool

func registerUpdateAPI(mux *http.ServeMux, u *updater) {
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, u.state())
	})
	mux.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		_ = u.check(ctx) // ошибка — в состоянии
		writeJSON(w, http.StatusOK, u.state())
	})
	mux.HandleFunc("POST /api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if u.state().State != "ready" {
			httpError(w, http.StatusConflict, "обновление ещё не скачано")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		go func() { time.Sleep(300 * time.Millisecond); _ = u.install() }() // сначала ответить странице
	})
}
