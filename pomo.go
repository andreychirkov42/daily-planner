package main

import (
	"errors"
	"sync"
	"time"
)

// Pomodoro — общий таймер для основного окна, мини-панели и иконки в трее.
// После фокус-сессии сам запускается перерыв (если это не выключено),
// после перерыва таймер ждёт нового старта.
type Pomodoro struct {
	mu    sync.Mutex
	store *Store
	st    PomoState
	// onFinish вызывается при окончании сессии (трей показывает мини-панель)
	onFinish func(finished, next string)
}

type PomoState struct {
	Mode       string         `json:"mode"` // work, short, long
	Running    bool           `json:"running"`
	EndAt      int64          `json:"endAt"` // unix ms, когда идёт отсчёт
	Left       int            `json:"left"`  // секунд осталось, когда стоит на паузе
	Label      string         `json:"label"`
	Durations  map[string]int `json:"durations"` // секунды
	Settings   PomoSettings   `json:"settings"`
	Count      int            `json:"count"` // фокус-сессий сегодня
	FinishedAt int64          `json:"finishedAt"`
	Finished   string         `json:"finished"` // какой режим только что закончился
}

// PomoSettings — длительности в минутах и поведение таймера; хранятся в data.json.
type PomoSettings struct {
	Work      int  `json:"work"`
	Short     int  `json:"short"`
	Long      int  `json:"long"`
	LongEvery int  `json:"longEvery"` // длинный перерыв после каждых N фокусов
	AutoBreak bool `json:"autoBreak"` // перерыв запускается сам
}

var defaultPomoSettings = PomoSettings{Work: 25, Short: 5, Long: 15, LongEvery: 4, AutoBreak: true}

func (s PomoSettings) valid() bool {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	return in(s.Work, 1, 180) && in(s.Short, 1, 60) && in(s.Long, 1, 120) && in(s.LongEvery, 1, 12)
}

func (s PomoSettings) seconds(mode string) int {
	return map[string]int{"work": s.Work, "short": s.Short, "long": s.Long}[mode] * 60
}

func NewPomodoro(s *Store) *Pomodoro {
	p := &Pomodoro{store: s, st: PomoState{Mode: "work"}}
	p.st.Left = p.settings().seconds("work")
	go func() {
		for range time.Tick(500 * time.Millisecond) {
			p.tick()
		}
	}()
	return p
}

func (p *Pomodoro) settings() PomoSettings {
	if st := p.store.PomoSettings(); st != nil && st.valid() {
		return *st
	}
	return defaultPomoSettings
}

func (p *Pomodoro) State() PomoState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st
	set := p.settings()
	st.Settings = set
	st.Durations = map[string]int{"work": set.seconds("work"), "short": set.seconds("short"), "long": set.seconds("long")}
	st.Count = p.store.PomoCount(today())
	return st
}

// Do выполняет команду: start, pause, reset, skip, mode, set (оставшееся время в минутах).
func (p *Pomodoro) Do(action, mode, label string, minutes float64) (PomoState, error) {
	p.mu.Lock()
	now := time.Now()
	var err error
	switch action {
	case "start":
		if !p.st.Running {
			p.st.Running = true
			p.st.EndAt = now.Add(time.Duration(p.st.Left) * time.Second).UnixMilli()
		}
		if label != "" {
			p.st.Label = label
		}
	case "pause":
		if p.st.Running {
			p.st.Left = p.leftLocked(now)
			p.st.Running = false
		}
	case "reset":
		p.setModeLocked(p.st.Mode)
	case "mode":
		if p.settings().seconds(mode) == 0 {
			err = errors.New("неизвестный режим")
			break
		}
		p.setModeLocked(mode)
		if label != "" {
			p.st.Label = label
		}
	case "set":
		// ручная правка часов: сколько осталось в текущей сессии
		if minutes <= 0 || minutes > 600 {
			err = errors.New("время от 1 секунды до 10 часов")
			break
		}
		left := int(minutes * 60)
		p.st.Left = left
		if p.st.Running {
			p.st.EndAt = now.Add(time.Duration(left) * time.Second).UnixMilli()
		}
	case "add":
		// стрелки у часов: ±минуты к оставшемуся времени, не меньше минуты
		cur := p.st.Left
		if p.st.Running {
			cur = p.leftLocked(now)
		}
		left := min(max(cur+int(minutes*60), 60), 600*60)
		p.st.Left = left
		if p.st.Running {
			p.st.EndAt = now.Add(time.Duration(left) * time.Second).UnixMilli()
		}
	case "skip":
		p.mu.Unlock()
		p.finish(false)
		return p.State(), nil
	default:
		err = errors.New("неизвестное действие")
	}
	p.mu.Unlock()
	return p.State(), err
}

// SetSettings сохраняет длительности; таймер на паузе сразу подхватывает новую длину.
func (p *Pomodoro) SetSettings(s PomoSettings) (PomoState, error) {
	if !s.valid() {
		return p.State(), errors.New("фокус 1–180 мин, перерыв 1–60, длинный 1–120, длинный каждые 1–12 фокусов")
	}
	p.mu.Lock()
	old := p.settings()
	if err := p.store.SetPomoSettings(s); err != nil {
		p.mu.Unlock()
		return p.State(), err
	}
	if !p.st.Running && p.st.Left == old.seconds(p.st.Mode) {
		p.st.Left = s.seconds(p.st.Mode)
	}
	p.mu.Unlock()
	return p.State(), nil
}

func (p *Pomodoro) setModeLocked(mode string) {
	p.st.Mode = mode
	p.st.Running = false
	p.st.Left = p.settings().seconds(mode)
	if mode == "work" {
		p.st.Label = ""
	}
}

func (p *Pomodoro) leftLocked(now time.Time) int {
	return max(0, int((p.st.EndAt-now.UnixMilli()+999)/1000))
}

func (p *Pomodoro) tick() {
	p.mu.Lock()
	done := p.st.Running && p.leftLocked(time.Now()) == 0
	p.mu.Unlock()
	if done {
		p.finish(true)
	}
}

// finish закрывает текущую сессию; counted — засчитывать ли фокус в статистику.
func (p *Pomodoro) finish(counted bool) {
	p.mu.Lock()
	set := p.settings()
	finished := p.st.Mode
	next := "work"
	if finished == "work" {
		if counted {
			_ = p.store.AddPomo(today())
		}
		next = "short"
		if n := p.store.PomoCount(today()); n > 0 && n%set.LongEvery == 0 {
			next = "long"
		}
	}
	label := p.st.Label
	p.setModeLocked(next)
	if next != "work" {
		p.st.Label = label
		if set.AutoBreak {
			p.st.Running = true
			p.st.EndAt = time.Now().Add(time.Duration(p.st.Left) * time.Second).UnixMilli()
		}
	}
	p.st.Finished = finished
	p.st.FinishedAt = time.Now().UnixMilli()
	cb := p.onFinish
	p.mu.Unlock()
	if cb != nil {
		cb(finished, next)
	}
}

func today() string { return time.Now().Format("2006-01-02") }
