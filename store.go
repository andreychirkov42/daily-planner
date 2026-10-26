package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
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

	// Что делать, если задача не выполнена к сроку
	Carry  string `json:"carry"`  // "" — остаётся просроченной, "move" — переезжает на сегодня, "shift" — сдвигается весь план
	Series string `json:"series"` // план, к которому относится задача (для "shift")

	UpdatedAt time.Time `json:"updatedAt"` // последняя правка — по ней синхронизация выбирает версию
}

// Series — план из последовательных задач (например, курс), разложенных по дням недели.
// Если задача плана не выполнена, оставшиеся задачи заново раскладываются начиная с сегодня.
type Series struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	PerDay [7]int `json:"perDay"` // сколько задач в день: вс, пн, вт, ср, чт, пт, сб

	UpdatedAt time.Time `json:"updatedAt"`
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

	UpdatedAt time.Time `json:"updatedAt"`
}

type Habit struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Color string          `json:"color"`
	Done  map[string]bool `json:"done"` // дата -> отмечено

	UpdatedAt time.Time `json:"updatedAt"`
}

type Data struct {
	Tasks    []*Task           `json:"tasks"`
	Lists    []*List           `json:"lists"`
	Habits   []*Habit          `json:"habits"`
	Notes    map[string]string `json:"notes"` // дата -> заметка дня
	Pomos    map[string]int    `json:"pomos"` // дата -> завершённых фокус-сессий
	Series   []*Series         `json:"series"`
	Pomo     *PomoSettings     `json:"pomoSettings,omitempty"`
	Sketches []*Sketch         `json:"sketches"`
	App      *AppSettings      `json:"app,omitempty"`

	// для синхронизации (sync.go): когда правили заметки дня и что удалено (id → когда)
	NoteTimes map[string]time.Time `json:"noteTimes,omitempty"`
	Deleted   map[string]time.Time `json:"deleted,omitempty"`
	Sync      *SyncSettings        `json:"sync,omitempty"`
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
		s.normalize() // все словари должны существовать и в новом файле
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
	if s.data.Pomos == nil {
		s.data.Pomos = map[string]int{}
	}
	if s.data.NoteTimes == nil {
		s.data.NoteTimes = map[string]time.Time{}
	}
	if s.data.Deleted == nil {
		s.data.Deleted = map[string]time.Time{}
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

// repeatID — id следующего повтора задачи. Он одинаковый на всех устройствах, поэтому
// отметка одной и той же задачи на телефоне и на ПК не даёт после синхронизации двух копий.
func repeatID(id, due string) string {
	h := sha256.Sum256([]byte(id + "/" + due))
	return hex.EncodeToString(h[:8])
}

// tombLocked запоминает удаление, чтобы синхронизация удалила запись и на другом устройстве.
func (s *Store) tombLocked(id string, now time.Time) { s.data.Deleted[id] = now }

// ---- задачи ----

func (s *Store) Snapshot() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.Marshal(s.data)
	var d Data
	_ = json.Unmarshal(b, &d)
	return d
}

// DueCounts — сколько невыполненных задач на день и сколько просрочено, без копирования данных.
func (s *Store) DueCounts(day string) (today, late int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tasks {
		switch {
		case t.Done || t.Due == "":
		case t.Due == day:
			today++
		case t.Due < day:
			late++
		}
	}
	return today, late
}

func (s *Store) Tasks() []*Task {
	return s.Snapshot().Tasks
}

func (s *Store) CreateTask(t *Task) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID = newID()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
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
	in.UpdatedAt = time.Now()
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
	s.tombLocked(id, time.Now())
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
	t.UpdatedAt = time.Now()
	var next *Task
	if t.Done {
		now := t.UpdatedAt
		t.CompletedAt = &now
		t.stopTimer(now)
		t.Status = ""
		if t.Repeat != "" {
			cp := *t
			cp.Due = nextDate(t.Due, t.Repeat)
			cp.ID = repeatID(t.ID, cp.Due)
			cp.Done = false
			cp.CompletedAt = nil
			cp.Spent = 0
			cp.CreatedAt = now
			cp.Order = len(s.data.Tasks)
			cp.Tags = append([]string{}, t.Tags...)
			cp.Subtasks = make([]Subtask, len(t.Subtasks))
			for i, st := range t.Subtasks {
				cp.Subtasks[i] = Subtask{ID: newID(), Title: st.Title}
			}
			t.Repeat = "" // повтор переходит к новой копии
			if _, old := s.findTask(cp.ID); old != nil {
				*old = cp // повтор уже пришёл с другого устройства — обновляем его, а не дублируем
				next = old
			} else {
				s.data.Tasks = append(s.data.Tasks, &cp)
				next = &cp
			}
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
			o.UpdatedAt = now
			changed = append(changed, o)
		}
	}
	if t.TimerStart == nil {
		t.TimerStart = &now
	}
	if !t.Done {
		t.Status = "doing"
	}
	t.UpdatedAt = now
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
	t.UpdatedAt = time.Now()
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
	now := time.Now()
	for _, t := range s.data.Tasks {
		if p, ok := pos[t.ID]; ok && t.Order != p {
			t.Order = p
			t.UpdatedAt = now
		}
	}
	return s.saveLocked()
}

func (s *Store) ClearCompleted() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.data.Tasks[:0]
	removed := 0
	now := time.Now()
	for _, t := range s.data.Tasks {
		if t.Done {
			removed++
			s.tombLocked(t.ID, now)
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
	l.UpdatedAt = time.Now()
	s.data.Lists = append(s.data.Lists, l)
	return l, s.saveLocked()
}

func (s *Store) UpdateList(id string, in *List) (*List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.data.Lists {
		if l.ID == id {
			in.ID = id
			in.UpdatedAt = time.Now()
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
			now := time.Now()
			s.tombLocked(id, now)
			for _, t := range s.data.Tasks {
				if t.ListID == id {
					t.ListID = ""
					t.UpdatedAt = now
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
	h.UpdatedAt = time.Now()
	s.data.Habits = append(s.data.Habits, h)
	return h, s.saveLocked()
}

func (s *Store) DeleteHabit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, h := range s.data.Habits {
		if h.ID == id {
			s.data.Habits = append(s.data.Habits[:i], s.data.Habits[i+1:]...)
			s.tombLocked(id, time.Now())
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
			h.UpdatedAt = time.Now()
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
	s.data.NoteTimes[date] = time.Now() // удаление заметки — тоже правка
	return s.saveLocked()
}

// ---- перенос невыполненного ----

func (s *Store) CreateSeries(sr *Series) (*Series, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sr.ID == "" {
		sr.ID = newID()
	}
	sr.UpdatedAt = time.Now()
	for i, old := range s.data.Series {
		if old.ID == sr.ID {
			s.data.Series[i] = sr
			return sr, s.saveLocked()
		}
	}
	s.data.Series = append(s.data.Series, sr)
	return sr, s.saveLocked()
}

// Rollover переносит просроченные невыполненные задачи согласно их Carry.
// Возвращает число изменённых задач.
func (s *Store) Rollover(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := now.Format("2006-01-02")
	changed := 0
	replan := map[string]bool{}
	for _, t := range s.data.Tasks {
		if t.Done || t.Due == "" || t.Due >= day {
			continue
		}
		switch {
		case t.Carry == "move" || (t.Carry == "shift" && s.seriesLocked(t.Series) == nil):
			t.Due = day
			t.DueTime = ""
			t.UpdatedAt = now
			changed++
		case t.Carry == "shift":
			replan[t.Series] = true
		}
	}
	for id := range replan {
		changed += s.replanLocked(s.seriesLocked(id), now)
	}
	if changed == 0 {
		return 0, nil
	}
	return changed, s.saveLocked()
}

func (s *Store) seriesLocked(id string) *Series {
	for _, sr := range s.data.Series {
		if sr.ID == id {
			return sr
		}
	}
	return nil
}

// replanLocked раскладывает невыполненные задачи плана по дням начиная с сегодня,
// сохраняя их порядок; задачи, уже сделанные сегодня, занимают сегодняшние места.
func (s *Store) replanLocked(sr *Series, now time.Time) int {
	day := now.Format("2006-01-02")
	var todo []*Task
	doneToday := 0
	for _, t := range s.data.Tasks {
		if t.Series != sr.ID {
			continue
		}
		if !t.Done {
			todo = append(todo, t)
		} else if t.CompletedAt != nil && t.CompletedAt.Format("2006-01-02") == day {
			doneToday++
		}
	}
	total := 0
	for _, n := range sr.PerDay {
		total += n
	}
	if len(todo) == 0 || total == 0 {
		return 0
	}
	sort.SliceStable(todo, func(i, j int) bool {
		if todo[i].Due != todo[j].Due {
			return todo[i].Due < todo[j].Due
		}
		return todo[i].Order < todo[j].Order
	})
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	free := sr.PerDay[d.Weekday()] - doneToday
	changed := 0
	for _, t := range todo {
		for free <= 0 {
			d = d.AddDate(0, 0, 1)
			free = sr.PerDay[d.Weekday()]
		}
		if due := d.Format("2006-01-02"); t.Due != due {
			t.Due = due
			t.DueTime = ""
			t.UpdatedAt = now
			changed++
		}
		free--
	}
	return changed
}

// ---- настройки программы ----

// AppSettings — общие для окна программы и браузера настройки.
type AppSettings struct {
	CloseAction string `json:"closeAction"` // "tray" — крестик сворачивает в трей, "quit" — закрывает программу
	DayEnd      string `json:"dayEnd"`      // конец рабочего дня ЧЧ:ММ

	NoAutoUpdate bool `json:"noAutoUpdate,omitempty"` // автообновление выключено (по умолчанию включено)
}

func (s *Store) AppSettings() AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := AppSettings{CloseAction: "tray", DayEnd: "22:00"}
	if s.data.App != nil {
		a.NoAutoUpdate = s.data.App.NoAutoUpdate
		if s.data.App.CloseAction != "" {
			a.CloseAction = s.data.App.CloseAction
		}
		if s.data.App.DayEnd != "" {
			a.DayEnd = s.data.App.DayEnd
		}
	}
	return a
}

func (s *Store) SetAppSettings(a AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.App = &a
	return s.saveLocked()
}

// ---- помодоро ----

func (s *Store) PomoCount(date string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Pomos[date]
}

func (s *Store) PomoSettings() *PomoSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Pomo == nil {
		return nil
	}
	cp := *s.data.Pomo
	return &cp
}

func (s *Store) SetPomoSettings(p PomoSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Pomo = &p
	return s.saveLocked()
}

func (s *Store) AddPomo(date string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Pomos[date]++
	return s.saveLocked()
}

// ---- импорт ----

func (s *Store) Replace(d Data) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d.Sync = s.data.Sync // подключение к телефону или ПК — свойство устройства, а не данных
	s.data = d
	s.normalize()
	now := time.Now() // импортированное должно победить при следующей синхронизации
	for _, t := range s.data.Tasks {
		t.UpdatedAt = now
	}
	for _, l := range s.data.Lists {
		l.UpdatedAt = now
	}
	for _, h := range s.data.Habits {
		h.UpdatedAt = now
	}
	for _, sr := range s.data.Series {
		sr.UpdatedAt = now
	}
	for date := range s.data.Notes {
		s.data.NoteTimes[date] = now
	}
	return s.saveLocked()
}
