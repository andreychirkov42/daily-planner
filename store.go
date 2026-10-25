package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Subtask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Notes       string     `json:"notes"`
	Done        bool       `json:"done"`
	Priority    int        `json:"priority"` // 0 нет, 1 низкий, 2 средний, 3 высокий
	Due         string     `json:"due"`      // YYYY-MM-DD
	DueTime     string     `json:"dueTime"`  // HH:MM
	ListID      string     `json:"listId"`
	Tags        []string   `json:"tags"`
	Subtasks    []Subtask  `json:"subtasks"`
	Repeat      string     `json:"repeat"` // "", daily, weekdays, weekly, monthly, yearly
	Pinned      bool       `json:"pinned"`
	Order       int        `json:"order"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Канбан и учёт времени (идеи из Vikunja и Super Productivity)
	Status     string     `json:"status"`   // "" — к выполнению, "doing" — в работе
	Estimate   int        `json:"estimate"` // оценка, минуты
	Spent      int        `json:"spent"`    // затрачено, секунды
	TimerStart *time.Time `json:"timerStart,omitempty"`
}

// stopTimer прибавляет время идущего таймера к Spent.
func (t *Task) stopTimer(now time.Time) {
	if t.TimerStart != nil {
		t.Spent += int(now.Sub(*t.TimerStart).Seconds())
		t.TimerStart = nil
	}
}

type List struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type Habit struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Color string          `json:"color"`
	Done  map[string]bool `json:"done"` // дата -> отмечено
}

type Data struct {
	Tasks  []*Task           `json:"tasks"`
	Lists  []*List           `json:"lists"`
	Habits []*Habit          `json:"habits"`
	Notes  map[string]string `json:"notes"` // дата -> заметка дня
}

type Store struct {
	mu   sync.RWMutex
	path string
	data Data
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.data = defaultData()
		return s, s.saveLocked()
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	s.normalize()
	return s, nil
}

func defaultData() Data {
	return Data{
		Lists: []*List{
			{ID: newID(), Name: "Личное", Color: "#7c6cf2", Icon: "🏠"},
			{ID: newID(), Name: "Работа", Color: "#2fb59a", Icon: "💼"},
			{ID: newID(), Name: "Учёба", Color: "#f2a33c", Icon: "📚"},
		},
		Notes: map[string]string{},
	}
}

func (s *Store) normalize() {
	if s.data.Notes == nil {
		s.data.Notes = map[string]string{}
	}
	for _, h := range s.data.Habits {
		if h.Done == nil {
			h.Done = map[string]bool{}
		}
	}
	for _, t := range s.data.Tasks {
		if t.Tags == nil {
			t.Tags = []string{}
		}
		if t.Subtasks == nil {
			t.Subtasks = []Subtask{}
		}
	}
}

// saveLocked атомарно пишет данные на диск; вызывать под блокировкой.
func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- задачи ----

func (s *Store) Snapshot() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.Marshal(s.data)
	var d Data
	_ = json.Unmarshal(b, &d)
	return d
}

func (s *Store) Tasks() []*Task {
	return s.Snapshot().Tasks
}

func (s *Store) CreateTask(t *Task) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID = newID()
	t.CreatedAt = time.Now()
	t.Order = len(s.data.Tasks)
	t.TimerStart = nil // таймер запускается только через свой эндпоинт
	if t.Status == "doing" && t.Done {
		t.Status = ""
	}
	fixTask(t)
	s.data.Tasks = append(s.data.Tasks, t)
	return t, s.saveLocked()
}

func fixTask(t *Task) {
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.Subtasks == nil {
		t.Subtasks = []Subtask{}
	}
	for i := range t.Subtasks {
		if t.Subtasks[i].ID == "" {
			t.Subtasks[i].ID = newID()
		}
	}
	if t.Priority < 0 || t.Priority > 3 {
		t.Priority = 0
	}
}

func (s *Store) findTask(id string) (int, *Task) {
	for i, t := range s.data.Tasks {
		if t.ID == id {
			return i, t
		}
	}
	return -1, nil
}

func (s *Store) UpdateTask(id string, in *Task) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t := s.findTask(id)
	if t == nil {
		return nil, ErrNotFound
	}
	in.ID, in.CreatedAt, in.Order = t.ID, t.CreatedAt, t.Order
	// таймером управляют только отдельные эндпоинты, клиент мог прислать устаревшее значение
	in.Spent, in.TimerStart = t.Spent, t.TimerStart
	if in.Done {
		in.stopTimer(time.Now())
		in.Status = ""
	}
	if in.Done && t.CompletedAt != nil {
		in.CompletedAt = t.CompletedAt
	} else if in.Done {
		now := time.Now()
		in.CompletedAt = &now
	} else {
		in.CompletedAt = nil
	}
	fixTask(in)
	*t = *in
	return t, s.saveLocked()
}

func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, t := s.findTask(id)
	if t == nil {
		return ErrNotFound
	}
	s.data.Tasks = append(s.data.Tasks[:i], s.data.Tasks[i+1:]...)
	return s.saveLocked()
}

// ToggleTask переключает выполнение. Для повторяющейся задачи при выполнении
// создаётся следующая копия с новой датой, а текущая остаётся выполненной.
func (s *Store) ToggleTask(id string) (*Task, *Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t := s.findTask(id)
	if t == nil {
		return nil, nil, ErrNotFound
	}
	t.Done = !t.Done
	var next *Task
	if t.Done {
		now := time.Now()
		t.CompletedAt = &now
		t.stopTimer(now)
		t.Status = ""
		if t.Repeat != "" {
			cp := *t
			cp.ID = newID()
			cp.Done = false
			cp.CompletedAt = nil
			cp.Spent = 0
			cp.CreatedAt = now
			cp.Order = len(s.data.Tasks)
			cp.Due = nextDate(t.Due, t.Repeat)
			cp.Tags = append([]string{}, t.Tags...)
			cp.Subtasks = make([]Subtask, len(t.Subtasks))
			for i, st := range t.Subtasks {
				cp.Subtasks[i] = Subtask{ID: newID(), Title: st.Title}
			}
			t.Repeat = "" // повтор переходит к новой копии
			s.data.Tasks = append(s.data.Tasks, &cp)
			next = &cp
		}
	} else {
		t.CompletedAt = nil
	}
	return t, next, s.saveLocked()
}

// StartTimer запускает учёт времени по задаче и останавливает остальные таймеры.
// Возвращает все задачи, которые изменились.
func (s *Store) StartTimer(id string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t := s.findTask(id)
	if t == nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	changed := []*Task{}
	for _, o := range s.data.Tasks {
		if o.TimerStart != nil && o.ID != id {
			o.stopTimer(now)
			changed = append(changed, o)
		}
	}
	if t.TimerStart == nil {
		t.TimerStart = &now
	}
	if !t.Done {
		t.Status = "doing"
	}
	changed = append(changed, t)
	return changed, s.saveLocked()
}

func (s *Store) StopTimer(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t := s.findTask(id)
	if t == nil {
		return nil, ErrNotFound
	}
	t.stopTimer(time.Now())
	return t, s.saveLocked()
}

func nextDate(due, repeat string) string {
	base := time.Now()
	if d, err := time.Parse("2006-01-02", due); err == nil {
		base = d
	}
	var n time.Time
	switch repeat {
	case "daily":
		n = base.AddDate(0, 0, 1)
	case "weekdays":
		n = base.AddDate(0, 0, 1)
		for n.Weekday() == time.Saturday || n.Weekday() == time.Sunday {
			n = n.AddDate(0, 0, 1)
		}
	case "weekly":
		n = base.AddDate(0, 0, 7)
	case "monthly":
		n = addMonthsClamped(base, 1)
	case "yearly":
		n = addMonthsClamped(base, 12)
	default:
		return due
	}
	return n.Format("2006-01-02")
}

// addMonthsClamped сдвигает дату на n месяцев; 31 января + 1 месяц = 28/29 февраля, а не 3 марта.
func addMonthsClamped(d time.Time, n int) time.Time {
	first := time.Date(d.Year(), d.Month()+time.Month(n), 1, 0, 0, 0, 0, d.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(d.Day(), last)-1)
}

func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := make(map[string]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	for _, t := range s.data.Tasks {
		if p, ok := pos[t.ID]; ok {
			t.Order = p
		}
	}
	return s.saveLocked()
}

func (s *Store) ClearCompleted() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.Tasks[:0]
	removed := 0
	for _, t := range s.data.Tasks {
		if t.Done {
			removed++
			continue
		}
		kept = append(kept, t)
	}
	s.data.Tasks = kept
	return removed, s.saveLocked()
}

// ---- списки ----

func (s *Store) CreateList(l *List) (*List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l.ID = newID()
	s.data.Lists = append(s.data.Lists, l)
	return l, s.saveLocked()
}

func (s *Store) UpdateList(id string, in *List) (*List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.data.Lists {
		if l.ID == id {
			in.ID = id
			*l = *in
			return l, s.saveLocked()
		}
	}
	return nil, ErrNotFound
}

// DeleteList удаляет список; его задачи переезжают во «Входящие».
func (s *Store) DeleteList(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, l := range s.data.Lists {
		if l.ID == id {
			s.data.Lists = append(s.data.Lists[:i], s.data.Lists[i+1:]...)
			for _, t := range s.data.Tasks {
				if t.ListID == id {
					t.ListID = ""
				}
			}
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// ---- привычки ----

func (s *Store) CreateHabit(h *Habit) (*Habit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h.ID = newID()
	h.Done = map[string]bool{}
	s.data.Habits = append(s.data.Habits, h)
	return h, s.saveLocked()
}

func (s *Store) DeleteHabit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, h := range s.data.Habits {
		if h.ID == id {
			s.data.Habits = append(s.data.Habits[:i], s.data.Habits[i+1:]...)
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

func (s *Store) ToggleHabit(id, date string) (*Habit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.data.Habits {
		if h.ID == id {
			if h.Done[date] {
				delete(h.Done, date)
			} else {
				h.Done[date] = true
			}
			return h, s.saveLocked()
		}
	}
	return nil, ErrNotFound
}

// ---- заметки дня ----

func (s *Store) Note(date string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Notes[date]
}

func (s *Store) SetNote(date, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if text == "" {
		delete(s.data.Notes, date)
	} else {
		s.data.Notes[date] = text
	}
	return s.saveLocked()
}

// ---- импорт ----

func (s *Store) Replace(d Data) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = d
	s.normalize()
	return s.saveLocked()
}
