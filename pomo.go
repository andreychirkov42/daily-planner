package main

import (
	"errors"
	"sync"
	"time"
)

// Pomodoro — общий таймер для основного окна, мини-панели и иконки в трее.
// После фокус-сессии сам запускается перерыв, после перерыва таймер ждёт нового старта.
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
	Count      int            `json:"count"`     // фокус-сессий сегодня
	FinishedAt int64          `json:"finishedAt"`
	Finished   string         `json:"finished"` // какой режим только что закончился
}

var pomoDurations = map[string]int{"work": 25 * 60, "short": 5 * 60, "long": 15 * 60}

func NewPomodoro(s *Store) *Pomodoro {
	p := &Pomodoro{store: s, st: PomoState{Mode: "work", Left: pomoDurations["work"], Durations: pomoDurations}}
	go func() {
		for range time.Tick(500 * time.Millisecond) {
			p.tick()
		}
	}()
	return p
}

func (p *Pomodoro) State() PomoState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st
	st.Count = p.store.PomoCount(today())
	return st
}

// Do выполняет команду: start, pause, reset, skip, mode.
func (p *Pomodoro) Do(action, mode, label string) (PomoState, error) {
	p.mu.Lock()
	now := time.Now()
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
		if _, ok := pomoDurations[mode]; !ok {
			p.mu.Unlock()
			return p.State(), errors.New("неизвестный режим")
		}
		p.setModeLocked(mode)
		if label != "" {
			p.st.Label = label
		}
	case "skip":
		p.mu.Unlock()
		p.finish(false)
		return p.State(), nil
	default:
		p.mu.Unlock()
		return p.State(), errors.New("неизвестное действие")
	}
	p.mu.Unlock()
	return p.State(), nil
}

func (p *Pomodoro) setModeLocked(mode string) {
	p.st.Mode = mode
	p.st.Running = false
	p.st.Left = pomoDurations[mode]
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
	finished := p.st.Mode
	next := "work"
	if finished == "work" {
		if counted {
			_ = p.store.AddPomo(today())
		}
		next = "short"
		if n := p.store.PomoCount(today()); n > 0 && n%4 == 0 {
			next = "long"
		}
	}
	label := p.st.Label
	p.setModeLocked(next)
	if next != "work" {
		// перерыв начинается сам
		p.st.Label = label
		p.st.Running = true
		p.st.EndAt = time.Now().Add(time.Duration(p.st.Left) * time.Second).UnixMilli()
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
