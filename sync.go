package main

// Синхронизация телефона с ПК. Телефон отправляет ПК все свои данные, ПК сливает их
// со своими и возвращает результат, телефон сливает его с тем, что успело измениться у него.
// Правило слияния одно на обеих сторонах: побеждает последняя правка записи (UpdatedAt),
// удаление (Deleted) побеждает правки, сделанные раньше него.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	syncPort     = "8092"
	tombstoneTTL = 90 * 24 * time.Hour // столько помним удаления; устройство, не синхронизированное дольше, может вернуть удалённое
)

// SyncSettings — подключение этого устройства. Не синхронизируется: у ПК и телефона свои.
type SyncSettings struct {
	ID       string    `json:"id,omitempty"`       // ПК: свой идентификатор, чтобы телефон не смешал данные двух компьютеров
	PeerID   string    `json:"peerId,omitempty"`   // телефон: идентификатор ПК, с которым уже синхронизировался
	Listen   bool      `json:"listen,omitempty"`   // ПК: принимать синхронизацию из сети
	Code     string    `json:"code,omitempty"`     // ПК: код, который вводят на телефоне
	Peer     string    `json:"peer,omitempty"`     // телефон: адрес ПК
	PeerCode string    `json:"peerCode,omitempty"` // телефон: код ПК
	Last     time.Time `json:"last,omitempty"`     // последняя удачная синхронизация
	LastErr  string    `json:"lastErr,omitempty"`
}

// syncPart — только то, что синхронизируется: задачи, списки, привычки, планы, заметки, фокус-сессии.
func syncPart(d *Data) Data {
	return Data{Tasks: d.Tasks, Lists: d.Lists, Habits: d.Habits, Series: d.Series,
		Notes: d.Notes, NoteTimes: d.NoteTimes, Pomos: d.Pomos, Deleted: d.Deleted}
}

// ---- слияние ----

// mergeByID сливает записи двух устройств: из пары берётся более свежая (при равенстве — своя),
// удалённые раньше своей последней правки записи выбрасываются. Порядок — свой, потом новые чужие.
func mergeByID[T any](mine, theirs []*T, id func(*T) string, upd func(*T) time.Time, deleted map[string]time.Time) []*T {
	other := make(map[string]*T, len(theirs))
	for _, x := range theirs {
		other[id(x)] = x
	}
	alive := func(x *T) bool {
		d, ok := deleted[id(x)]
		return !ok || upd(x).After(d)
	}
	out := make([]*T, 0, len(mine)+len(theirs))
	seen := map[string]bool{}
	for _, x := range mine {
		seen[id(x)] = true
		if o := other[id(x)]; o != nil && upd(o).After(upd(x)) {
			x = o
		}
		if alive(x) {
			out = append(out, x)
		}
	}
	for _, x := range theirs {
		if !seen[id(x)] && alive(x) {
			out = append(out, x)
		}
	}
	return out
}

// mergeData сливает данные другого устройства (theirs) со своими (mine) и возвращает результат.
func mergeData(mine, theirs Data, now time.Time) Data {
	deleted := map[string]time.Time{}
	for _, m := range []map[string]time.Time{mine.Deleted, theirs.Deleted} {
		for id, t := range m {
			if now.Sub(t) < tombstoneTTL && t.After(deleted[id]) {
				deleted[id] = t
			}
		}
	}
	out := Data{Deleted: deleted, Notes: map[string]string{}, NoteTimes: map[string]time.Time{}, Pomos: map[string]int{}}
	out.Tasks = mergeByID(mine.Tasks, theirs.Tasks, func(t *Task) string { return t.ID }, func(t *Task) time.Time { return t.UpdatedAt }, deleted)
	out.Lists = mergeByID(mine.Lists, theirs.Lists, func(l *List) string { return l.ID }, func(l *List) time.Time { return l.UpdatedAt }, deleted)
	out.Habits = mergeByID(mine.Habits, theirs.Habits, func(h *Habit) string { return h.ID }, func(h *Habit) time.Time { return h.UpdatedAt }, deleted)
	out.Series = mergeByID(mine.Series, theirs.Series, func(s *Series) string { return s.ID }, func(s *Series) time.Time { return s.UpdatedAt }, deleted)

	// заметки дня — по времени правки даты; пустой текст значит «заметку удалили»
	for _, d := range []Data{mine, theirs} {
		for date, t := range d.NoteTimes {
			if t.After(out.NoteTimes[date]) {
				out.NoteTimes[date] = t
			}
		}
	}
	for _, d := range []Data{mine, theirs} {
		for date, text := range d.Notes {
			if _, ok := out.Notes[date]; !ok && !d.NoteTimes[date].Before(out.NoteTimes[date]) {
				out.Notes[date] = text
			}
		}
	}
	// фокус-сессии только прибавляются — берём большее
	for _, d := range []Data{mine, theirs} {
		for date, n := range d.Pomos {
			out.Pomos[date] = max(out.Pomos[date], n)
		}
	}
	return out
}

// MergeSync сливает пришедшие данные с данными хранилища и возвращает общий результат.
func (s *Store) MergeSync(in Data) (Data, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, _ := json.Marshal(syncPart(&s.data))
	m := mergeData(syncPart(&s.data), in, time.Now())
	s.data.Tasks, s.data.Lists, s.data.Habits, s.data.Series = m.Tasks, m.Lists, m.Habits, m.Series
	s.data.Notes, s.data.NoteTimes, s.data.Pomos, s.data.Deleted = m.Notes, m.NoteTimes, m.Pomos, m.Deleted
	s.normalize()
	after, _ := json.Marshal(syncPart(&s.data))
	changed := !bytes.Equal(before, after)
	var out Data
	_ = json.Unmarshal(after, &out) // копия: дальше её сериализуют уже без блокировки
	if !changed {
		return out, false, nil
	}
	return out, true, s.saveLocked()
}

// dropUntouchedLists удаляет списки, созданные при первом запуске и ни разу не использованные.
// Так при первой синхронизации телефон не добавит на ПК «Личное» и «Работу», которые там удалили.
func (s *Store) dropUntouchedLists() {
	s.mu.Lock()
	defer s.mu.Unlock()
	used := map[string]bool{}
	for _, t := range s.data.Tasks {
		used[t.ListID] = true
	}
	kept := s.data.Lists[:0]
	now := time.Now()
	for _, l := range s.data.Lists {
		if l.UpdatedAt.IsZero() && !used[l.ID] {
			s.tombLocked(l.ID, now)
			continue
		}
		kept = append(kept, l)
	}
	s.data.Lists = kept
	_ = s.saveLocked()
}

func (s *Store) SyncSettings() SyncSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Sync == nil {
		return SyncSettings{}
	}
	return *s.data.Sync
}

func (s *Store) SetSyncSettings(set SyncSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sync = &set
	return s.saveLocked()
}

// ---- ПК: приём синхронизации из сети ----

// codeAlphabet без похожих символов (0/O, 1/I/L), чтобы код легко было набрать на телефоне.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newSyncCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b[:4]) + "-" + string(b[4:])
}

func normCode(c string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(c)))
}

// syncServer — отдельный сервер в локальной сети: только POST /sync с кодом.
// Основной интерфейс по-прежнему доступен только с этого компьютера.
type syncServer struct {
	store *Store
	addr  string

	mu     sync.Mutex
	srv    *http.Server
	fails  int
	lockTo time.Time
}

func (ss *syncServer) apply() {
	set := ss.store.SyncSettings()
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if set.Listen && ss.srv == nil {
		ln, err := net.Listen("tcp", ss.addr)
		if err != nil {
			log.Printf("синхронизация: не удалось открыть %s: %v", ss.addr, err)
			return
		}
		ss.srv = &http.Server{Handler: ss.handler(), ReadHeaderTimeout: 10 * time.Second}
		go func(srv *http.Server) { _ = srv.Serve(ln) }(ss.srv)
		log.Printf("синхронизация с телефоном: слушаю %s", ss.addr)
	} else if !set.Listen && ss.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = ss.srv.Shutdown(ctx)
		ss.srv = nil
		log.Printf("синхронизация с телефоном выключена")
	}
}

func (ss *syncServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sync", ss.handle)
	return mux
}

// checkCode сверяет код; после серии ошибок на время перестаёт принимать любые коды (перебор).
func (ss *syncServer) checkCode(got string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if time.Now().Before(ss.lockTo) {
		return false
	}
	want := normCode(ss.store.SyncSettings().Code)
	if want != "" && subtle.ConstantTimeCompare([]byte(normCode(got)), []byte(want)) == 1 {
		ss.fails = 0
		return true
	}
	ss.fails++
	if ss.fails >= 10 {
		ss.fails = 0
		ss.lockTo = time.Now().Add(10 * time.Minute)
		log.Printf("синхронизация: много неверных кодов, приём остановлен на 10 минут")
	}
	return false
}

func (ss *syncServer) handle(w http.ResponseWriter, r *http.Request) {
	if !ss.checkCode(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
		time.Sleep(time.Second)
		httpError(w, http.StatusForbidden, "неверный код")
		return
	}
	set := ss.store.SyncSettings()
	if set.ID == "" {
		set.ID = newID()
		_ = ss.store.SetSyncSettings(set)
	}
	// телефон уже связан с другим компьютером — не смешиваем задачи двух ПК
	if p := r.Header.Get("X-Clepsy-Peer"); p != "" && p != set.ID {
		httpError(w, http.StatusConflict, "другой компьютер")
		return
	}
	var in Data
	if !readJSON(w, r, &in) {
		return
	}
	out, changed, err := ss.store.MergeSync(in)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	set = ss.store.SyncSettings()
	set.Last, set.LastErr = time.Now(), ""
	_ = ss.store.SetSyncSettings(set)
	w.Header().Set("X-Clepsy-Id", set.ID)
	if changed {
		log.Printf("синхронизация с телефоном (%s): данные обновлены", r.RemoteAddr)
	}
	writeJSON(w, http.StatusOK, out)
}

// lanAddrs — адреса этого компьютера в локальных сетях (и в Tailscale, 100.x), для подсказки на экране.
// Домашний Wi-Fi (192.168…) первым; 198.18… — адаптеры VPN и прокси, с телефона недоступны.
func lanAddrs() []string {
	rank := func(ip string) int {
		switch {
		case strings.HasPrefix(ip, "192.168."):
			return 0
		case strings.HasPrefix(ip, "10."):
			return 1
		case strings.HasPrefix(ip, "100."):
			return 2
		}
		return 3
	}
	var out []string
	defer func() { sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) }) }()
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() && !(ip4[0] == 198 && ip4[1]&0xfe == 18) {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

// ---- телефон: синхронизация с ПК ----

// peerURL превращает «192.168.1.5» или «http://192.168.1.5:8092/» в адрес запроса.
func peerURL(peer string) (string, error) {
	p := strings.TrimSpace(peer)
	p = strings.TrimPrefix(strings.TrimPrefix(p, "http://"), "https://")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", errors.New("не указан адрес компьютера")
	}
	host, _, err := net.SplitHostPort(p)
	if err != nil {
		host, p = p, net.JoinHostPort(p, syncPort)
	}
	// у сервера на телефоне нет системного DNS (на Android нет /etc/resolv.conf) — только IP
	if net.ParseIP(host) == nil {
		return "", errors.New("укажи IP-адрес компьютера цифрами, например 192.168.0.10")
	}
	return "http://" + p + "/sync", nil
}

var syncMu sync.Mutex // одна синхронизация за раз

// runSync отправляет данные на ПК и сливает ответ со своими. Возвращает, изменилось ли что-то здесь.
func runSync(s *Store) (bool, error) {
	syncMu.Lock()
	defer syncMu.Unlock()
	set := s.SyncSettings()
	url, err := peerURL(set.Peer)
	if err == nil && set.Last.IsZero() {
		s.dropUntouchedLists() // первая синхронизация: не тащим на ПК пустые списки по умолчанию
	}
	var changed bool
	var peerID string
	if err == nil {
		changed, peerID, err = exchange(s, url, set.PeerCode, set.PeerID)
	}
	set = s.SyncSettings()
	if err != nil {
		set.LastErr = err.Error()
	} else {
		set.Last, set.LastErr = time.Now(), ""
		if peerID != "" {
			set.PeerID = peerID
		}
	}
	if serr := s.SetSyncSettings(set); err == nil {
		err = serr
	}
	return changed, err
}

// exchange — один обмен с ПК; возвращает, изменились ли данные здесь, и идентификатор ПК.
func exchange(s *Store, url, code, peerID string) (bool, string, error) {
	snap := s.Snapshot()
	body, err := json.Marshal(syncPart(&snap))
	if err != nil {
		return false, "", err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+code)
	req.Header.Set("X-Clepsy-Peer", peerID)
	c := http.Client{Timeout: 20 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return false, "", errors.New("компьютер не отвечает: проверь адрес, что Clepsy на ПК запущен и телефон в той же сети")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusForbidden:
		return false, "", errors.New("неверный код — посмотри его в настройках Clepsy на компьютере")
	case res.StatusCode == http.StatusConflict:
		return false, "", errors.New("телефон уже синхронизирован с другим компьютером — чтобы не смешать задачи, синхронизация отменена")
	case res.StatusCode != http.StatusOK:
		return false, "", fmt.Errorf("компьютер ответил ошибкой %d", res.StatusCode)
	}
	var merged Data
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 50<<20)).Decode(&merged); err != nil {
		return false, "", errors.New("непонятный ответ компьютера")
	}
	_, changed, err := s.MergeSync(merged)
	return changed, res.Header.Get("X-Clepsy-Id"), err
}

// ---- API для интерфейса ----

func registerSyncAPI(mux *http.ServeMux, s *Store, ss *syncServer) {
	view := func() map[string]any {
		set := s.SyncSettings()
		v := map[string]any{
			"listen": set.Listen, "code": set.Code, "addrs": lanAddrs(), "port": syncPort,
			"peer": set.Peer, "hasPeerCode": set.PeerCode != "", "lastErr": set.LastErr,
		}
		if !set.Last.IsZero() {
			v["last"] = set.Last
		}
		return v
	}
	mux.HandleFunc("GET /api/sync", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, view()) })
	mux.HandleFunc("PUT /api/sync", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Listen   *bool   `json:"listen"`
			NewCode  bool    `json:"newCode"`
			Peer     *string `json:"peer"`
			PeerCode *string `json:"peerCode"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		set := s.SyncSettings()
		if body.Listen != nil {
			set.Listen = *body.Listen
		}
		if body.NewCode || (set.Listen && set.Code == "") {
			set.Code = newSyncCode()
		}
		if body.Peer != nil {
			// Last не сбрасываем: смена адреса (Wi-Fi → Tailscale) — тот же компьютер, а «первая
			// синхронизация» удалила бы его пустые списки, пришедшие на телефон раньше
			set.Peer = strings.TrimSpace(*body.Peer)
		}
		if body.PeerCode != nil {
			set.PeerCode = strings.TrimSpace(*body.PeerCode)
		}
		set.LastErr = ""
		if err := s.SetSyncSettings(set); err != nil {
			respond(w, 0, nil, err)
			return
		}
		if ss != nil {
			ss.apply()
		}
		writeJSON(w, http.StatusOK, view())
	})
	mux.HandleFunc("POST /api/sync/run", func(w http.ResponseWriter, r *http.Request) {
		changed, err := runSync(s)
		if err != nil {
			httpError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "sync": view()})
	})
}
