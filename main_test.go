package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNextDate(t *testing.T) {
	cases := []struct{ due, repeat, want string }{
		{"2026-10-25", "daily", "2026-10-26"},
		{"2026-10-23", "weekdays", "2026-10-26"}, // пятница → понедельник
		{"2026-10-25", "weekly", "2026-11-01"},
		{"2026-01-31", "monthly", "2026-02-28"}, // нет 31 февраля — последний день месяца
		{"2028-02-29", "yearly", "2029-02-28"},
		{"2026-10-25", "yearly", "2027-10-25"},
		{"2026-10-25", "", "2026-10-25"},
	}
	for _, c := range cases {
		if got := nextDate(c.due, c.repeat); got != c.want {
			t.Errorf("nextDate(%s, %s) = %s, want %s", c.due, c.repeat, got, c.want)
		}
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestToggleRepeatCreatesNext(t *testing.T) {
	s := newTestStore(t)
	task, _ := s.CreateTask(&Task{Title: "Зарядка", Due: "2026-10-25", Repeat: "daily", Subtasks: []Subtask{{Title: "a", Done: true}}})
	done, next, err := s.ToggleTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Done || done.Repeat != "" {
		t.Fatalf("исходная задача: done=%v repeat=%q", done.Done, done.Repeat)
	}
	if next == nil || next.Due != "2026-10-26" || next.Done || next.Repeat != "daily" || next.Subtasks[0].Done {
		t.Fatalf("неверная следующая копия: %+v", next)
	}
}

func TestTimerAccumulatesAndIsExclusive(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateTask(&Task{Title: "a"})
	b, _ := s.CreateTask(&Task{Title: "b"})
	if _, err := s.StartTimer(a.ID); err != nil {
		t.Fatal(err)
	}
	// сдвигаем старт в прошлое, чтобы не ждать
	past := time.Now().Add(-90 * time.Second)
	s.data.Tasks[0].TimerStart = &past
	changed, _ := s.StartTimer(b.ID)
	if len(changed) != 2 {
		t.Fatalf("ожидали остановку таймера a и запуск b, изменено: %d", len(changed))
	}
	if got := s.data.Tasks[0]; got.TimerStart != nil || got.Spent < 89 {
		t.Fatalf("таймер a: start=%v spent=%d", got.TimerStart, got.Spent)
	}
	if s.data.Tasks[1].Status != "doing" {
		t.Fatal("запуск таймера должен переводить задачу «в работу»")
	}
	// клиент не может подменить затраченное время через обычное сохранение
	upd := *s.data.Tasks[0]
	upd.Spent = 99999
	res, _ := s.UpdateTask(a.ID, &upd)
	if res.Spent == 99999 {
		t.Fatal("UpdateTask не должен менять Spent")
	}
}

func TestRolloverMoveAndShift(t *testing.T) {
	s := newTestStore(t)
	// воскресенье 25.10.2026; план: пн 1, вт 2, ср 2, чт 1, пт 1, сб 0, вс 1
	sr, _ := s.CreateSeries(&Series{Name: "Курс", PerDay: [7]int{1, 1, 2, 2, 1, 1, 0}})
	chore, _ := s.CreateTask(&Task{Title: "Полить цветы", Due: "2026-10-23", Carry: "move"})
	stay, _ := s.CreateTask(&Task{Title: "Просрочка", Due: "2026-10-23"})
	dues := []string{"2026-10-24", "2026-10-26", "2026-10-27", "2026-10-27", "2026-10-28"}
	var lessons []*Task
	for i, d := range dues {
		l, _ := s.CreateTask(&Task{Title: fmt.Sprint("Урок ", i), Due: d, Carry: "shift", Series: sr.ID})
		lessons = append(lessons, l)
	}
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local) // понедельник
	if _, err := s.Rollover(now); err != nil {
		t.Fatal(err)
	}
	get := func(id string) *Task { _, tk := s.findTask(id); return tk }
	if get(chore.ID).Due != "2026-10-26" {
		t.Errorf("move: срок %s, ожидали перенос на сегодня", get(chore.ID).Due)
	}
	if get(stay.ID).Due != "2026-10-23" {
		t.Error("задача без переноса не должна двигаться")
	}
	// пропущенный урок встаёт на сегодня, остальные сдвигаются по сетке, суббота пустая
	want := []string{"2026-10-26", "2026-10-27", "2026-10-27", "2026-10-28", "2026-10-28"}
	for i, l := range lessons {
		if got := get(l.ID).Due; got != want[i] {
			t.Errorf("урок %d: %s, ожидали %s", i, got, want[i])
		}
	}
}

func TestPomodoroCustomSettings(t *testing.T) {
	s := newTestStore(t)
	p := NewPomodoro(s)
	if st := p.State(); st.Left != 25*60 {
		t.Fatalf("по умолчанию 25 минут, получили %d с", st.Left)
	}
	if _, err := p.SetSettings(PomoSettings{Work: 50, Short: 10, Long: 30, LongEvery: 2, AutoBreak: false}); err != nil {
		t.Fatal(err)
	}
	if st := p.State(); st.Left != 50*60 || st.Durations["short"] != 600 {
		t.Fatalf("новые длительности не применились: %+v", st)
	}
	if _, err := p.SetSettings(PomoSettings{Work: 0, Short: 5, Long: 15, LongEvery: 4}); err == nil {
		t.Fatal("ноль минут фокуса должен отклоняться")
	}
	// ручная правка часов
	if st, _ := p.Do("set", "", "", 12.5); st.Left != 750 {
		t.Fatalf("set 12.5 мин: осталось %d с", st.Left)
	}
	// после двух фокусов — длинный перерыв, и без автозапуска
	p.Do("start", "", "", 0)
	p.finish(true)
	p.Do("mode", "work", "", 0)
	p.finish(true)
	if st := p.State(); st.Mode != "long" || st.Running {
		t.Fatalf("ожидали длинный перерыв на паузе, получили mode=%s running=%v", st.Mode, st.Running)
	}
	// настройки сохраняются в data.json
	s2, _ := NewStore(s.path)
	if got := NewPomodoro(s2).State().Settings.Work; got != 50 {
		t.Fatalf("после перезапуска фокус %d мин, ожидали 50", got)
	}
}

func TestGuardBlocksCrossSite(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := guard(ok)
	cases := []struct {
		name, host, origin, ctype string
		want                      int
	}{
		{"свой запрос", "127.0.0.1:8080", "http://127.0.0.1:8080", "application/json", 204},
		{"чужой сайт", "127.0.0.1:8080", "https://evil.example", "application/json", 403},
		{"text/plain без CORS", "127.0.0.1:8080", "", "text/plain", 415},
		{"DNS rebinding", "evil.example:8080", "", "application/json", 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/import", strings.NewReader(`{}`))
		r.Host = c.host
		r.Header.Set("Content-Type", c.ctype)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: код %d, ожидали %d", c.name, w.Code, c.want)
		}
	}
}
