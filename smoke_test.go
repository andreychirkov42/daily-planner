package main

import (
	"context"
	"html"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// TestSmokePages открывает основную страницу и мини-панель в браузере без окна
// (Edge или Chrome) и проверяет, что они загрузились без ошибок JavaScript.
// Страницы с ?smoke сами обходят все разделы, панель задачи и окна.
func TestSmokePages(t *testing.T) {
	browser := findBrowser()
	if browser == "" {
		t.Skip("не найден Edge/Chrome; путь можно указать в DAILYPLANNER_BROWSER")
	}

	s := newTestStore(t)
	seedSmokeData(t, s)
	static, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	registerAPI(mux, s, NewPomodoro(s))
	srv := httptest.NewServer(guard(mux))
	defer srv.Close()

	for _, page := range []string{"/?smoke=1", "/popup.html?smoke=1"} {
		t.Run(page, func(t *testing.T) {
			dom := dumpDOM(t, browser, srv.URL+page)
			htmlTag := regexp.MustCompile(`(?s)<html[^>]*>`).FindString(dom)
			if m := regexp.MustCompile(`data-errors="([^"]*)"`).FindStringSubmatch(htmlTag); m != nil {
				t.Fatalf("ошибки JavaScript: %s", html.UnescapeString(m[1]))
			}
			if !regexp.MustCompile(`data-ready="1"`).MatchString(htmlTag) {
				t.Fatalf("страница не дошла до конца загрузки; тег: %s", htmlTag)
			}
		})
	}
}

func findBrowser() string {
	if p := os.Getenv("DAILYPLANNER_BROWSER"); p != "" {
		return p
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"msedge", "google-chrome", "chromium", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func dumpDOM(t *testing.T, browser, url string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser,
		"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--user-data-dir="+t.TempDir(),
		"--virtual-time-budget=15000", "--dump-dom", url)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("браузер не отработал: %v", err)
	}
	return string(out)
}

// seedSmokeData кладёт задачи во всех состояниях, чтобы отрисовка прошла по всем веткам.
func seedSmokeData(t *testing.T, s *Store) {
	t.Helper()
	day := time.Now()
	d := func(n int) string { return day.AddDate(0, 0, n).Format("2006-01-02") }
	sr, _ := s.CreateSeries(&Series{Name: "Курс", PerDay: [7]int{1, 1, 2, 2, 1, 1, 0}})
	listID := s.Snapshot().Lists[0].ID
	tasks := []*Task{
		{Title: "Просроченная", Due: d(-2), Priority: 3, Carry: "move"},
		{Title: "Сегодня со временем", Due: d(0), DueTime: "18:00", Priority: 2, Estimate: 45, ListID: listID, Tags: []string{"работа"}},
		{Title: "С подзадачами", Due: d(0), Subtasks: []Subtask{{Title: "раз", Done: true}, {Title: "два"}}, Notes: "заметка"},
		{Title: "Урок плана", Due: d(1), Carry: "shift", Series: sr.ID, Estimate: 90},
		{Title: "Повторяется", Due: d(3), Repeat: "weekly", Pinned: true},
		{Title: "Без срока"},
	}
	for _, tk := range tasks {
		if _, err := s.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
	}
	done, _ := s.CreateTask(&Task{Title: "Уже сделана", Due: d(0)})
	_, _, _ = s.ToggleTask(done.ID)
	_, _ = s.StartTimer(tasks[1].ID)
	h, _ := s.CreateHabit(&Habit{Name: "Чтение", Color: "#2f6bff"})
	_, _ = s.ToggleHabit(h.ID, d(0))
	_ = s.SetNote(d(0), "заметка дня")
}
