package main

// Автообновление (по образцу Tauri updater). В каждом релизе лежит манифест latest.json: версия,
// описание изменений и для каждой платформы ссылка на файл с его SHA-256. Манифест подписан
// ключом автора (latest.json.sig, ed25519). Clepsy читает его по постоянной ссылке
// releases/latest/download/latest.json — это обычное скачивание, без API GitHub и его лимитов.
// Windows: новая версия скачивается, сверяется и ставится подменой exe с перезапуском
// (update_windows.go), когда программой не пользуются. Android: то же делает оболочка на Java.

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
	manifestURL    = repoURL + "/releases/latest/download/latest.json"
	updateEvery    = 6 * time.Hour
	maxUpdateBytes = 200 << 20
)

// updatePubKey — открытый ключ подписи манифеста (tools/release). Закрытый ключ хранится только
// у автора (не на GitHub): даже со взломанным аккаунтом GitHub подменённый exe не поставится.
// Переменная — чтобы тесты подставили свой ключ.
var updatePubKey = "4c249d0e7f5811889d3a917287edb99ccb5591d92c1914d971679d2e31168ff4"

// updateManifestURL — адрес latest.json; в тестах подменяется на свой сервер.
func updateManifestURL() string {
	if u := os.Getenv("CLEPSY_UPDATE_URL"); u != "" {
		return u
	}
	return manifestURL
}

// Manifest — latest.json. Его же пишет tools/release при выпуске.
type Manifest struct {
	Version   string                   `json:"version"`
	PubDate   string                   `json:"pub_date"`
	Notes     string                   `json:"notes"` // что нового, Markdown
	Platforms map[string]PlatformBuild `json:"platforms"`
}

type PlatformBuild struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
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

var updateClient = &http.Client{Timeout: 5 * time.Minute}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Clepsy/"+appVersion)
	res, err := updateClient.Do(req)
	if err != nil {
		return nil, errors.New("нет связи с GitHub")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("в последнем релизе нет latest.json — обнови вручную со страницы релизов")
	}
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

// verifySig проверяет подпись автора (hex ed25519) над данными.
func verifySig(data, sigHex []byte) bool {
	pub, err1 := hex.DecodeString(updatePubKey)
	sig, err2 := hex.DecodeString(strings.TrimSpace(string(sigHex)))
	return err1 == nil && err2 == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(pub, data, sig)
}

// fetchManifest скачивает latest.json и его подпись; без верной подписи манифесту не верим.
func fetchManifest(ctx context.Context) (*Manifest, error) {
	url := updateManifestURL()
	data, err := httpGet(ctx, url, 1<<20)
	if err != nil {
		return nil, err
	}
	sig, err := httpGet(ctx, url+".sig", 1<<10)
	if err != nil {
		return nil, err
	}
	if !verifySig(data, sig) {
		return nil, errors.New("подпись релиза не сошлась — он не от автора Clepsy, не устанавливаю")
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil || m.Version == "" {
		return nil, errors.New("непонятный latest.json")
	}
	return &m, nil
}

// fetchBuild скачивает файл своей платформы и сверяет его SHA-256 с подписанным манифестом.
func fetchBuild(ctx context.Context, m *Manifest, platform string) ([]byte, error) {
	p, ok := m.Platforms[platform]
	if !ok || p.URL == "" || p.SHA256 == "" {
		return nil, fmt.Errorf("в версии %s нет сборки для этой системы", m.Version)
	}
	b, err := httpGet(ctx, p.URL, maxUpdateBytes)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(b)
	if hex.EncodeToString(got[:]) != strings.ToLower(p.SHA256) {
		return nil, errors.New("контрольная сумма не совпала — файл повреждён или подменён, не устанавливаю")
	}
	return b, nil
}

// UpdateState — что показывает раздел «Настройки → Обновления» и окно «Что нового».
type UpdateState struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest,omitempty"`
	Notes       string    `json:"notes,omitempty"` // что нового в Latest
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
	return &updater{store: s, pomo: p, st: UpdateState{Current: appVersion, UpdatedFrom: updatedFrom, CanInstall: updatePlatform != ""}}
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

// check читает манифест и, если версия новее и эта сборка умеет ставить, скачивает её.
func (u *updater) check(ctx context.Context) error {
	u.mu.Lock()
	if u.st.State == "checking" || u.st.State == "downloading" || u.st.State == "installing" {
		u.mu.Unlock()
		return nil
	}
	u.st.State, u.st.Err = "checking", ""
	u.mu.Unlock()

	err := func() error {
		m, err := fetchManifest(ctx)
		if err != nil {
			return err
		}
		avail := newerVersion(m.Version, appVersion)
		u.set(func(s *UpdateState) {
			s.Latest, s.Notes, s.Available, s.Checked = m.Version, m.Notes, avail, time.Now()
		})
		if !avail || updatePlatform == "" {
			return nil
		}
		u.mu.Lock()
		have := u.ready != nil && u.readyFor == m.Version // уже скачана именно эта версия, а не прошлая
		u.mu.Unlock()
		if have {
			return nil
		}
		u.set(func(s *UpdateState) { s.State = "downloading" })
		b, err := fetchBuild(ctx, m, updatePlatform)
		if err != nil {
			return err
		}
		u.mu.Lock()
		u.ready, u.readyFor = b, m.Version
		u.mu.Unlock()
		log.Printf("обновление %s скачано и проверено", m.Version)
		return nil
	}()
	u.set(func(s *UpdateState) {
		switch {
		case err != nil:
			s.State, s.Err = "error", err.Error()
		case s.Available && updatePlatform != "":
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

// idleAfter — сколько ничего не меняли, чтобы считать, что программой не пользуются
// (окно может быть закрыто, а Clepsy открыт во вкладке браузера).
const idleAfter = 15 * time.Minute

// idle — программой сейчас не пользуются: окна закрыты, таймер не идёт и давно ничего не меняли.
func (u *updater) idle() bool {
	if u.pomo != nil && u.pomo.State().Running {
		return false
	}
	if time.Since(time.Unix(0, lastEdit.Load())) < idleAfter {
		return false
	}
	return appBusy == nil || !appBusy()
}

// loop — фоновая проверка: вскоре после запуска, потом раз в 6 часов; скачанное ставится,
// как только программа освободится (проверяем каждые 5 минут). Сразу после обновления
// проверяем без ожидания — чтобы показать «Что нового» в только что поставленной версии.
func (u *updater) loop() {
	if u.state().UpdatedFrom == "" {
		time.Sleep(time.Minute)
	}
	for {
		if u.autoOn() || u.state().UpdatedFrom != "" && u.state().Checked.IsZero() {
			if st := u.state(); time.Since(st.Checked) > updateEvery || st.State == "error" && time.Since(st.Checked) > time.Hour {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				if err := u.check(ctx); err != nil {
					log.Printf("проверка обновлений: %v", err)
					u.set(func(s *UpdateState) { s.Checked = time.Now() }) // не долбим GitHub при ошибке
				}
				cancel()
			}
			if u.autoOn() && u.state().State == "ready" && u.idle() {
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
