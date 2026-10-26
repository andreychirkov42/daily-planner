package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMergeLastEditWins(t *testing.T) {
	t0 := time.Now().Add(-time.Hour)
	t1 := t0.Add(time.Minute)
	pc := Data{Tasks: []*Task{{ID: "a", Title: "ПК новее", UpdatedAt: t1}, {ID: "b", Title: "старое", UpdatedAt: t0}}}
	phone := Data{Tasks: []*Task{{ID: "a", Title: "телефон", UpdatedAt: t0}, {ID: "b", Title: "телефон новее", UpdatedAt: t1}, {ID: "c", Title: "только на телефоне", UpdatedAt: t0}}}
	m := mergeData(pc, phone, time.Now())
	got := map[string]string{}
	for _, x := range m.Tasks {
		got[x.ID] = x.Title
	}
	want := map[string]string{"a": "ПК новее", "b": "телефон новее", "c": "только на телефоне"}
	for id, title := range want {
		if got[id] != title {
			t.Errorf("%s: %q, ожидали %q", id, got[id], title)
		}
	}
	if len(m.Tasks) != 3 || m.Tasks[0].ID != "a" || m.Tasks[2].ID != "c" {
		t.Errorf("порядок: свои, потом новые чужие; получили %v", got)
	}
}

func TestMergeDeletes(t *testing.T) {
	t0 := time.Now().Add(-time.Hour)
	pc := Data{
		Tasks:   []*Task{{ID: "edited-later", Title: "правка после удаления", UpdatedAt: t0.Add(2 * time.Minute)}},
		Deleted: map[string]time.Time{"gone": t0.Add(time.Minute)},
	}
	phone := Data{
		Tasks:   []*Task{{ID: "gone", Title: "удалили на ПК", UpdatedAt: t0}},
		Deleted: map[string]time.Time{"edited-later": t0.Add(time.Minute), "ancient": time.Now().Add(-100 * 24 * time.Hour)},
	}
	m := mergeData(pc, phone, time.Now())
	if len(m.Tasks) != 1 || m.Tasks[0].ID != "edited-later" {
		t.Fatalf("удалённая задача должна исчезнуть, а исправленная после удаления — остаться: %+v", m.Tasks)
	}
	if _, ok := m.Deleted["ancient"]; ok {
		t.Error("старые надгробия должны забываться")
	}
}

func TestMergeNotesAndPomos(t *testing.T) {
	t0 := time.Now().Add(-time.Hour)
	pc := Data{
		Notes:     map[string]string{"2026-10-26": "старая", "2026-10-25": "есть только на ПК"},
		NoteTimes: map[string]time.Time{"2026-10-26": t0, "2026-10-25": t0},
		Pomos:     map[string]int{"2026-10-26": 3},
	}
	phone := Data{
		Notes:     map[string]string{},
		NoteTimes: map[string]time.Time{"2026-10-26": t0.Add(time.Minute)}, // на телефоне заметку стёрли позже
		Pomos:     map[string]int{"2026-10-26": 1, "2026-10-24": 2},
	}
	m := mergeData(pc, phone, time.Now())
	if _, ok := m.Notes["2026-10-26"]; ok {
		t.Error("удаление заметки на телефоне должно дойти до ПК")
	}
	if m.Notes["2026-10-25"] != "есть только на ПК" {
		t.Error("заметка, которой нет на телефоне, должна остаться")
	}
	if m.Pomos["2026-10-26"] != 3 || m.Pomos["2026-10-24"] != 2 {
		t.Errorf("помидоры — по большему: %v", m.Pomos)
	}
}

// Одну и ту же повторяющуюся задачу отметили и на ПК, и на телефоне — копия должна быть одна.
func TestRepeatOnTwoDevicesMakesOneCopy(t *testing.T) {
	pc, phone := newTestStore(t), newTestStore(t)
	task, _ := pc.CreateTask(&Task{Title: "Зарядка", Due: "2026-10-26", Repeat: "daily"})
	if _, _, err := phone.MergeSync(syncPart(ptr(pc.Snapshot()))); err != nil {
		t.Fatal(err)
	}
	_, n1, _ := pc.ToggleTask(task.ID)
	_, n2, _ := phone.ToggleTask(task.ID)
	if n1.ID != n2.ID {
		t.Fatalf("id повтора должен совпадать: %s и %s", n1.ID, n2.ID)
	}
	m, _, _ := pc.MergeSync(syncPart(ptr(phone.Snapshot())))
	copies := 0
	for _, x := range m.Tasks {
		if x.Title == "Зарядка" && !x.Done {
			copies++
		}
	}
	if copies != 1 {
		t.Fatalf("невыполненных копий: %d", copies)
	}
}

func ptr(d Data) *Data { return &d }

// Телефон и ПК через настоящий HTTP: первая синхронизация, правки с обеих сторон, удаление, неверный код.
func TestSyncPhoneWithPC(t *testing.T) {
	pc, phone := newTestStore(t), newTestStore(t)
	ss := &syncServer{store: pc}
	srv := httptest.NewServer(ss.handler())
	defer srv.Close()
	_ = pc.SetSyncSettings(SyncSettings{Listen: true, Code: "ABCD-EFGH"})

	// на ПК пользователь удалил «Личное» и «Работу», на телефоне — свежая установка со списками по умолчанию
	for _, l := range pc.Snapshot().Lists[:2] {
		_ = pc.DeleteList(l.ID)
	}
	keep, _ := pc.CreateTask(&Task{Title: "с ПК", Due: "2026-10-26"})
	gone, _ := pc.CreateTask(&Task{Title: "удалят на телефоне"})

	_ = phone.SetSyncSettings(SyncSettings{Peer: strings.TrimPrefix(srv.URL, "http://"), PeerCode: "abcd efgh"})
	if _, err := runSync(phone); err != nil {
		t.Fatal(err)
	}
	if n := len(phone.Snapshot().Lists); n != 1 {
		t.Errorf("после первой синхронизации на телефоне должен быть один список (как на ПК), а их %d", n)
	}
	if n := len(pc.Snapshot().Lists); n != 1 {
		t.Errorf("списки телефона по умолчанию не должны попасть на ПК: на ПК %d", n)
	}

	time.Sleep(10 * time.Millisecond)
	_, _ = phone.CreateTask(&Task{Title: "с телефона"})
	_ = phone.DeleteTask(gone.ID)
	edit := pc.Snapshot().Tasks[0]
	edit.Title = "с ПК, исправлено"
	_, _ = pc.UpdateTask(keep.ID, edit)
	changed, err := runSync(phone)
	if err != nil || !changed {
		t.Fatalf("вторая синхронизация: changed=%v err=%v", changed, err)
	}
	titles := func(s *Store) string {
		var out []string
		for _, x := range s.Snapshot().Tasks {
			out = append(out, x.Title)
		}
		return strings.Join(out, ", ")
	}
	want := "с ПК, исправлено, с телефона"
	if titles(pc) != want || titles(phone) != want {
		t.Fatalf("данные разошлись:\nПК:      %s\nтелефон: %s", titles(pc), titles(phone))
	}

	_ = phone.SetSyncSettings(SyncSettings{Peer: srv.URL, PeerCode: "WRONG-CODE", Last: time.Now()})
	if _, err := runSync(phone); err == nil || !strings.Contains(err.Error(), "код") {
		t.Fatalf("с неверным кодом должна быть понятная ошибка, а не %v", err)
	}
}
