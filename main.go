package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "адрес сервера")
	dataPath := flag.String("data", defaultDataPath(), "файл с данными")
	noOpen := flag.Bool("no-open", false, "не открывать браузер при старте")
	noTray := flag.Bool("no-tray", false, "без иконки в трее, только веб-сервер")
	autostart := flag.Bool("autostart", false, "запуск вместе с Windows: тихо, только иконка в трее")
	showPopup := flag.Bool("popup", false, "сразу показать мини-панель из трея")
	syncAddr := flag.String("sync-addr", ":"+syncPort, "адрес приёма синхронизации с телефона (включается в настройках)")
	flag.Parse()
	setupLog(filepath.Join(filepath.Dir(*dataPath), "planner.log"))

	url := "http://" + *addr
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		// порт занят — скорее всего ежедневник уже запущен: просим его показать своё окно
		log.Printf("порт %s занят (%v), Clepsy уже запущен", *addr, err)
		if !*autostart && !askRunningToOpen(url) {
			openBrowser(url)
		}
		return
	}

	store, err := NewStore(*dataPath)
	if err != nil {
		log.Fatalf("не удалось открыть %s: %v", *dataPath, err)
	}
	pomo := NewPomodoro(store)
	syncSrv = &syncServer{store: store, addr: *syncAddr}
	syncSrv.apply() // если в настройках включена синхронизация с телефоном
	// раз в минуту переносим невыполненное (и сразу после полуночи, и при старте)
	go func() {
		for {
			if n, err := store.Rollover(time.Now()); err != nil {
				log.Printf("перенос задач: %v", err)
			} else if n > 0 {
				log.Printf("перенесено задач: %d", n)
			}
			time.Sleep(time.Minute)
		}
	}()

	static, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	registerAPI(mux, store, pomo)

	log.Printf("Clepsy запущен: %s, данные: %s", url, *dataPath)
	go func() { log.Fatal(http.Serve(ln, guard(logRequests(mux)))) }()
	if *noTray {
		if !*noOpen {
			go func() { time.Sleep(300 * time.Millisecond); openBrowser(url) }()
		}
		select {}
	}
	runApp(appOptions{url: url, store: store, pomo: pomo, showPopup: *showPopup, showMain: !*noOpen && !*autostart})
}

type appOptions struct {
	url                 string
	store               *Store
	pomo                *Pomodoro
	showPopup, showMain bool
}

// openAppWindow показывает окно ежедневника (задаёт оболочка с иконкой в трее).
// Возвращает false, если своего окна нет и нужно открыть браузер.
var openAppWindow func(view string) bool

// setAppTheme красит заголовок окна под тему страницы (задаёт оболочка Windows).
var setAppTheme func(dark bool)

const (
	appVersion = "1.1.2"
	repoURL    = "https://github.com/andreychirkov42/daily-planner"
)

// Возможности оболочки Windows для раздела «Настройки»; nil — на этой системе недоступно.
var (
	getAutostart func() bool
	putAutostart func(on bool) error
	revealFile   func(path string)
)

// askRunningToOpen просит уже запущенную копию показать окно вместо новой вкладки.
func askRunningToOpen(url string) bool {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Post(url+"/api/app/open", "application/json", strings.NewReader(`{"view":""}`))
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == http.StatusNoContent
}

// defaultDataPath кладёт data.json рядом с exe, чтобы данные не зависели от папки запуска.
func defaultDataPath() string {
	exe, err := os.Executable()
	if err != nil || strings.HasPrefix(exe, os.TempDir()) { // go run собирает во временную папку
		return "data.json"
	}
	return filepath.Join(filepath.Dir(exe), "data.json")
}

// setupLog пишет лог в файл: у exe без консоли stderr никуда не ведёт.
func setupLog(path string) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}

// syncSrv принимает синхронизацию с телефона; nil в тестах.
var syncSrv *syncServer

func registerAPI(mux *http.ServeMux, s *Store, pomo *Pomodoro) {
	registerSketchAPI(mux, s)
	registerSyncAPI(mux, s, syncSrv)
	mux.HandleFunc("GET /api/pomodoro", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, pomo.State())
	})
	mux.HandleFunc("POST /api/pomodoro", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action, Mode, Label string
			Minutes             float64
		}
		if !readJSON(w, r, &body) {
			return
		}
		st, err := pomo.Do(body.Action, body.Mode, body.Label, body.Minutes)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("PUT /api/pomodoro/settings", func(w http.ResponseWriter, r *http.Request) {
		var set PomoSettings
		if !readJSON(w, r, &set) {
			return
		}
		st, err := pomo.SetSettings(set)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /api/series", func(w http.ResponseWriter, r *http.Request) {
		var sr Series
		if !readJSON(w, r, &sr) {
			return
		}
		if sr.Name == "" {
			httpError(w, http.StatusBadRequest, "нужно название плана")
			return
		}
		res, err := s.CreateSeries(&sr)
		respond(w, http.StatusOK, res, err)
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, settingsView(s))
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CloseAction *string `json:"closeAction"`
			DayEnd      *string `json:"dayEnd"`
			Autostart   *bool   `json:"autostart"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		a := s.AppSettings()
		if body.CloseAction != nil {
			if *body.CloseAction != "tray" && *body.CloseAction != "quit" {
				httpError(w, http.StatusBadRequest, "closeAction: tray или quit")
				return
			}
			a.CloseAction = *body.CloseAction
		}
		if body.DayEnd != nil {
			if !timeRe.MatchString(*body.DayEnd) {
				httpError(w, http.StatusBadRequest, "конец дня в формате ЧЧ:ММ")
				return
			}
			a.DayEnd = *body.DayEnd
		}
		if body.Autostart != nil {
			if putAutostart == nil {
				httpError(w, http.StatusBadRequest, "автозапуск доступен только в Windows")
				return
			}
			if err := putAutostart(*body.Autostart); err != nil {
				httpError(w, http.StatusInternalServerError, "автозапуск: "+err.Error())
				return
			}
		}
		if err := s.SetAppSettings(a); err != nil {
			respond(w, 0, nil, err)
			return
		}
		writeJSON(w, http.StatusOK, settingsView(s))
	})
	mux.HandleFunc("POST /api/app/reveal", func(w http.ResponseWriter, r *http.Request) {
		if revealFile == nil {
			httpError(w, http.StatusNotFound, "недоступно")
			return
		}
		revealFile(s.path)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/app/github", func(w http.ResponseWriter, r *http.Request) {
		openBrowser(repoURL) // только этот адрес — чтобы API не открывал произвольные ссылки
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/app/theme", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Dark bool }
		if !readJSON(w, r, &body) {
			return
		}
		if setAppTheme != nil {
			setAppTheme(body.Dark)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/app/open", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ View string }
		if !readJSON(w, r, &body) {
			return
		}
		if openAppWindow == nil || !openAppWindow(body.View) {
			httpError(w, http.StatusNotFound, "окно недоступно")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// всё состояние одним запросом — фронтенду так проще
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, publicSnapshot(s))
	})

	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Tasks())
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		var t Task
		if !readJSON(w, r, &t) || !validTask(w, &t) {
			return
		}
		res, err := s.CreateTask(&t)
		respond(w, http.StatusCreated, res, err)
	})
	mux.HandleFunc("PUT /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var t Task
		if !readJSON(w, r, &t) || !validTask(w, &t) {
			return
		}
		res, err := s.UpdateTask(r.PathValue("id"), &t)
		respond(w, http.StatusOK, res, err)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, s.DeleteTask(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		t, next, err := s.ToggleTask(r.PathValue("id"))
		respond(w, http.StatusOK, map[string]*Task{"task": t, "next": next}, err)
	})
	mux.HandleFunc("POST /api/tasks/{id}/timer/start", func(w http.ResponseWriter, r *http.Request) {
		changed, err := s.StartTimer(r.PathValue("id"))
		respond(w, http.StatusOK, changed, err)
	})
	mux.HandleFunc("POST /api/tasks/{id}/timer/stop", func(w http.ResponseWriter, r *http.Request) {
		t, err := s.StopTimer(r.PathValue("id"))
		respond(w, http.StatusOK, t, err)
	})
	mux.HandleFunc("POST /api/tasks/reorder", func(w http.ResponseWriter, r *http.Request) {
		var ids []string
		if !readJSON(w, r, &ids) {
			return
		}
		respond(w, http.StatusNoContent, nil, s.Reorder(ids))
	})
	mux.HandleFunc("POST /api/tasks/clear-completed", func(w http.ResponseWriter, r *http.Request) {
		n, err := s.ClearCompleted()
		respond(w, http.StatusOK, map[string]int{"removed": n}, err)
	})

	mux.HandleFunc("POST /api/lists", func(w http.ResponseWriter, r *http.Request) {
		var l List
		if !readJSON(w, r, &l) {
			return
		}
		if l.Name == "" {
			httpError(w, http.StatusBadRequest, "нужно название списка")
			return
		}
		res, err := s.CreateList(&l)
		respond(w, http.StatusCreated, res, err)
	})
	mux.HandleFunc("PUT /api/lists/{id}", func(w http.ResponseWriter, r *http.Request) {
		var l List
		if !readJSON(w, r, &l) {
			return
		}
		res, err := s.UpdateList(r.PathValue("id"), &l)
		respond(w, http.StatusOK, res, err)
	})
	mux.HandleFunc("DELETE /api/lists/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, s.DeleteList(r.PathValue("id")))
	})

	mux.HandleFunc("POST /api/habits", func(w http.ResponseWriter, r *http.Request) {
		var h Habit
		if !readJSON(w, r, &h) {
			return
		}
		if h.Name == "" {
			httpError(w, http.StatusBadRequest, "нужно название привычки")
			return
		}
		res, err := s.CreateHabit(&h)
		respond(w, http.StatusCreated, res, err)
	})
	mux.HandleFunc("DELETE /api/habits/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, s.DeleteHabit(r.PathValue("id")))
	})
	mux.HandleFunc("POST /api/habits/{id}/toggle/{date}", func(w http.ResponseWriter, r *http.Request) {
		date := r.PathValue("date")
		if !dateRe.MatchString(date) {
			httpError(w, http.StatusBadRequest, "дата в формате ГГГГ-ММ-ДД")
			return
		}
		res, err := s.ToggleHabit(r.PathValue("id"), date)
		respond(w, http.StatusOK, res, err)
	})

	mux.HandleFunc("GET /api/notes/{date}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"text": s.Note(r.PathValue("date"))})
	})
	mux.HandleFunc("PUT /api/notes/{date}", func(w http.ResponseWriter, r *http.Request) {
		date := r.PathValue("date")
		var body struct {
			Text string `json:"text"`
		}
		if !dateRe.MatchString(date) {
			httpError(w, http.StatusBadRequest, "дата в формате ГГГГ-ММ-ДД")
			return
		}
		if !readJSON(w, r, &body) {
			return
		}
		respond(w, http.StatusNoContent, nil, s.SetNote(date, body.Text))
	})

	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="planner-backup-`+time.Now().Format("2006-01-02")+`.json"`)
		writeJSON(w, http.StatusOK, publicSnapshot(s))
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var d Data
		if !readJSON(w, r, &d) {
			return
		}
		respond(w, http.StatusNoContent, nil, s.Replace(d))
	})
}

// publicSnapshot — данные для страницы и резервной копии: без кодов синхронизации,
// чтобы они не уходили вместе с файлом экспорта.
func publicSnapshot(s *Store) Data {
	d := s.Snapshot()
	d.Sync = nil
	return d
}

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// settingsView — всё, что показывает раздел «Настройки».
func settingsView(s *Store) map[string]any {
	a := s.AppSettings()
	v := map[string]any{
		"closeAction": a.CloseAction,
		"dayEnd":      a.DayEnd,
		"version":     appVersion,
		"repo":        repoURL,
		"dataPath":    s.path,
		"canReveal":   revealFile != nil,
		"desktop":     putAutostart != nil, // есть ли оболочка Windows с треем
		"phone":       apiToken != "",      // запущен внутри приложения Android
	}
	if getAutostart != nil {
		v["autostart"] = getAutostart()
	}
	return v
}

func validTask(w http.ResponseWriter, t *Task) bool {
	switch {
	case t.Title == "":
		httpError(w, http.StatusBadRequest, "нужно название задачи")
	case t.Due != "" && !dateRe.MatchString(t.Due):
		httpError(w, http.StatusBadRequest, "дедлайн в формате ГГГГ-ММ-ДД")
	default:
		return true
	}
	return false
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		httpError(w, http.StatusBadRequest, "некорректный JSON: "+err.Error())
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, v any, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpError(w, http.StatusNotFound, "не найдено")
	case err != nil:
		log.Printf("ошибка: %v", err)
		httpError(w, http.StatusInternalServerError, "ошибка сервера")
	case status == http.StatusNoContent:
		w.WriteHeader(status)
	default:
		writeJSON(w, status, v)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// apiToken задаёт оболочка Android: на телефоне к 127.0.0.1 могут обратиться и другие
// приложения, поэтому API отвечает только WebView, которому выдана cookie с этим кодом.
var apiToken = os.Getenv("CLEPSY_TOKEN")

// guard не даёт чужим сайтам, открытым в браузере, менять данные через локальный API:
// проверяем Host (защита от DNS rebinding), Origin и тип тела запроса.
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			httpError(w, http.StatusForbidden, "доступ только с этого компьютера")
			return
		}
		if apiToken != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			c, err := r.Cookie("clepsy")
			if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(apiToken)) != 1 {
				httpError(w, http.StatusForbidden, "нет доступа")
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				httpError(w, http.StatusForbidden, "запрос с чужого сайта")
				return
			}
			if r.ContentLength != 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				httpError(w, http.StatusUnsupportedMediaType, "нужен application/json")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		if len(r.URL.Path) > 4 && r.URL.Path[:5] == "/api/" {
			log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
		}
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
