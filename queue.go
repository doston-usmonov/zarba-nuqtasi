package main

// «O'yin topish» — reytingli navbat (Raqobat 5×5, Wingman 2×2).
//
// Faqat hisobga kirganlar. Navbatga bitta o'yinchi yoki partiya (do'stlar) bo'lib turiladi: partiya
// boshlig'i onlayn do'stlarini chaqiradi, ular qabul qilsa navbatda birga turadi va bitta jamoaga
// tushadi. Reyting farqi boshida ±150, har 20 soniyada ±50 kengayadi; yetarli odam (10 yoki 4)
// yig'ilganda yangi yopiq xona `mm-<tasodifiy>` ochiladi, o'yinchilar reyting bo'yicha teng ikki
// jamoaga bo'linadi (partiyalar ajratilmaydi). Xonaga faqat topilgan o'yinchilar kira oladi, bot
// qo'shib bo'lmaydi — demak o'yin reytingga yoziladi.
//
// Holat server xotirasida: POST /api/queue (join / leave / accept / decline), GET /api/queue —
// brauzer navbatda turganda har 2 soniyada so'raydi (shu so'rov «hali shu yerdaman» belgisi ham:
// 15 soniya so'ramagan o'yinchi navbatdan chiqariladi). Partiya chaqiriqlari /api/social
// javobida ham keladi.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	queueRange0    = 150              // boshlang'ich reyting farqi
	queueRangeStep = 50               // har queueWiden da shuncha kengayadi
	queueWiden     = 20 * time.Second //
	queueStale     = 15 * time.Second // shuncha vaqt so'ramagan o'yinchi navbatdan chiqadi
	queueFoundTTL  = 3 * time.Minute  // topilgan o'yin haqida xabar shuncha turadi
	mmEmptyTTL     = 3 * time.Minute  // hech kim kirmagan mm xonasi o'chiriladi
	partyInviteTTL = 2 * time.Minute
)

// queueSize: rejimda nechta o'yinchi kerak.
var queueSize = map[string]int{modeComp: 10, modeWM: 4}

type qMember struct {
	id     int64
	name   string
	rating int
	seen   time.Time
}

// qTicket: navbatdagi bitta o'yinchi yoki partiya.
type qTicket struct {
	mode    string
	leader  int64
	members []*qMember
	since   time.Time
}

func (t *qTicket) rating() float64 {
	s := 0
	for _, m := range t.members {
		s += m.rating
	}
	return float64(s) / float64(len(t.members))
}

// spread: shu chiptaning hozirgi ruxsat etilgan reyting farqi.
func (t *qTicket) spread(now time.Time) float64 {
	return queueRange0 + queueRangeStep*math.Floor(now.Sub(t.since).Seconds()/queueWiden.Seconds())
}

type qFound struct {
	Room string `json:"room"` // kod (mm-...)
	Map  string `json:"map"`
	Mode string `json:"mode"`
	Sq   string `json:"sq"` // qaysi jamoa
	at   time.Time
}

type partyInvite struct {
	From string `json:"from"`
	Mode string `json:"mode"`
	from int64
	at   time.Time
}

// mmRoom: navbatdan ochilgan xona — kim kira oladi va qaysi jamoada.
type mmRoom struct {
	sq map[int64]string
}

type matchmaker struct {
	mu      sync.Mutex
	tickets []*qTicket
	byUser  map[int64]*qTicket
	found   map[int64]qFound
	invites map[int64]partyInvite
	now     func() time.Time
	open    func(mode, gmap string, sq map[int64]string) string // xona ochadi, kodini qaytaradi
}

func newMatchmaker() *matchmaker {
	return &matchmaker{byUser: map[int64]*qTicket{}, found: map[int64]qFound{}, invites: map[int64]partyInvite{}, now: time.Now}
}

var (
	errQueueMode  = errors.New("Bu rejimda navbat yo'q")
	errInQueue    = errors.New("Siz allaqachon navbatdasiz")
	errPartyBig   = errors.New("Partiya bu rejim uchun juda katta")
	errNoInvite   = errors.New("Chaqiriq topilmadi yoki eskirgan")
	errPartyGone  = errors.New("Partiya navbatdan chiqib ketgan")
	errNotInQueue = errors.New("Siz navbatda emassiz")
)

// join: o'yinchi (partiya boshlig'i) navbatga turadi. Chaqirilgan do'stlarga taklif yuboriladi.
func (q *matchmaker) join(u qMember, mode string, invite []qMember) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	need := queueSize[mode]
	if need == 0 {
		return errQueueMode
	}
	if q.byUser[u.id] != nil {
		return errInQueue
	}
	if 1+len(invite) > need/2 {
		return errPartyBig
	}
	now := q.now()
	u.seen = now
	t := &qTicket{mode: mode, leader: u.id, members: []*qMember{&u}, since: now}
	q.tickets = append(q.tickets, t)
	q.byUser[u.id] = t
	delete(q.found, u.id)
	for _, f := range invite {
		if f.id != u.id && q.byUser[f.id] == nil {
			q.invites[f.id] = partyInvite{From: u.name, Mode: mode, from: u.id, at: now}
		}
	}
	q.matchLocked()
	return nil
}

// accept: chaqirilgan do'st partiyaga qo'shiladi (boshliq hali navbatda bo'lsa).
func (q *matchmaker) accept(u qMember) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	inv, ok := q.invites[u.id]
	now := q.now()
	if !ok || now.Sub(inv.at) > partyInviteTTL {
		return errNoInvite
	}
	delete(q.invites, u.id)
	t := q.byUser[inv.from]
	if t == nil || t.leader != inv.from {
		return errPartyGone
	}
	if q.byUser[u.id] != nil {
		return errInQueue
	}
	if len(t.members)+1 > queueSize[t.mode]/2 {
		return errPartyBig
	}
	u.seen = now
	t.members = append(t.members, &u)
	q.byUser[u.id] = t
	delete(q.found, u.id)
	q.matchLocked()
	return nil
}

func (q *matchmaker) decline(uid int64) {
	q.mu.Lock()
	delete(q.invites, uid)
	q.mu.Unlock()
}

// leave: o'yinchi navbatdan chiqadi; boshliq chiqsa butun partiya chiqadi.
func (q *matchmaker) leave(uid int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.leaveLocked(uid)
}

func (q *matchmaker) leaveLocked(uid int64) {
	t := q.byUser[uid]
	if t == nil {
		return
	}
	if t.leader == uid || len(t.members) == 1 {
		q.dropLocked(t)
		return
	}
	keep := t.members[:0]
	for _, m := range t.members {
		if m.id != uid {
			keep = append(keep, m)
		}
	}
	t.members = keep
	delete(q.byUser, uid)
}

func (q *matchmaker) dropLocked(t *qTicket) {
	for _, m := range t.members {
		delete(q.byUser, m.id)
	}
	for i, x := range q.tickets {
		if x == t {
			q.tickets = append(q.tickets[:i], q.tickets[i+1:]...)
			break
		}
	}
}

type queueStatus struct {
	State   string         `json:"state"` // idle, searching, found
	Mode    string         `json:"mode,omitempty"`
	Wait    int            `json:"wait,omitempty"`   // soniya
	Range   int            `json:"range,omitempty"`  // hozirgi reyting farqi
	Party   []string       `json:"party,omitempty"`  // partiya a'zolari
	Leader  bool           `json:"leader,omitempty"` //
	Found   *qFound        `json:"found,omitempty"`
	Invite  *partyInvite   `json:"invite,omitempty"`
	InQueue map[string]int `json:"inQueue"` // har rejimda navbatdagi o'yinchilar
}

// status: o'yinchining holati. Bu so'rov «hali shu yerdaman» belgisi ham.
func (q *matchmaker) status(uid int64) queueStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	q.sweepLocked(now)
	st := queueStatus{State: "idle", InQueue: map[string]int{}}
	for _, t := range q.tickets {
		st.InQueue[t.mode] += len(t.members)
	}
	if inv, ok := q.invites[uid]; ok && now.Sub(inv.at) <= partyInviteTTL {
		st.Invite = &inv
	}
	if f, ok := q.found[uid]; ok {
		st.State, st.Found, st.Mode = "found", &f, f.Mode
		return st
	}
	t := q.byUser[uid]
	if t == nil {
		return st
	}
	for _, m := range t.members {
		if m.id == uid {
			m.seen = now
		}
		st.Party = append(st.Party, m.name)
	}
	st.State, st.Mode, st.Leader = "searching", t.mode, t.leader == uid
	st.Wait, st.Range = int(now.Sub(t.since).Seconds()), int(t.spread(now))
	q.matchLocked() // vaqt o'tgani sari farq kengayadi
	if f, ok := q.found[uid]; ok {
		st.State, st.Found = "found", &f
	}
	return st
}

// sweepLocked: yopilgan brauzerlar, eskirgan xabarlar.
func (q *matchmaker) sweepLocked(now time.Time) {
	for _, t := range append([]*qTicket(nil), q.tickets...) {
		for _, m := range append([]*qMember(nil), t.members...) {
			if now.Sub(m.seen) > queueStale {
				q.leaveLocked(m.id)
			}
		}
	}
	for id, f := range q.found {
		if now.Sub(f.at) > queueFoundTTL {
			delete(q.found, id)
		}
	}
	for id, inv := range q.invites {
		if now.Sub(inv.at) > partyInviteTTL {
			delete(q.invites, id)
		}
	}
}

// matchLocked: har rejimda eng uzoq kutayotgandan boshlab mos guruh yig'adi.
func (q *matchmaker) matchLocked() {
	now := q.now()
	for {
		made := false
		sort.SliceStable(q.tickets, func(i, j int) bool { return q.tickets[i].since.Before(q.tickets[j].since) })
		for _, t0 := range q.tickets {
			if g := q.groupFor(t0, now); g != nil {
				q.startLocked(g, now)
				made = true
				break
			}
		}
		if !made {
			return
		}
	}
}

// groupFor: t0 bilan birga o'ynay oladigan chiptalar (har juftlik bir-birining farqiga sig'adi,
// jami aynan kerakli son, ikki teng jamoaga partiyalarni buzmasdan bo'linadi).
func (q *matchmaker) groupFor(t0 *qTicket, now time.Time) []*qTicket {
	need := queueSize[t0.mode]
	group, n := []*qTicket{t0}, len(t0.members)
	fits := func(t *qTicket) bool {
		for _, g := range group {
			d := math.Abs(t.rating() - g.rating())
			if d > t.spread(now) || d > g.spread(now) {
				return false
			}
		}
		return true
	}
	for _, t := range q.tickets {
		if t == t0 || t.mode != t0.mode || n+len(t.members) > need || !fits(t) {
			continue
		}
		group, n = append(group, t), n+len(t.members)
		if n == need {
			if _, ok := splitTeams(group, need/2); ok {
				return group
			}
			group, n = group[:len(group)-1], n-len(t.members)
		}
	}
	return nil
}

// splitTeams: partiyalarni buzmasdan ikki jamoaga, reytinglar yig'indisi iloji boricha teng.
func splitTeams(group []*qTicket, half int) (a []bool, ok bool) {
	best, bestD := []bool(nil), math.Inf(1)
	total := 0.0
	for _, t := range group {
		total += t.rating() * float64(len(t.members))
	}
	k := len(group)
	for mask := 0; mask < 1<<k; mask++ {
		if mask&1 == 0 { // birinchi chipta doim A da (simmetriya)
			continue
		}
		n, s := 0, 0.0
		for i, t := range group {
			if mask&(1<<i) != 0 {
				n += len(t.members)
				s += t.rating() * float64(len(t.members))
			}
		}
		if n != half {
			continue
		}
		if d := math.Abs(2*s - total); d < bestD {
			bestD, best = d, make([]bool, k)
			for i := range group {
				best[i] = mask&(1<<i) != 0
			}
		}
	}
	return best, best != nil
}

func (q *matchmaker) startLocked(group []*qTicket, now time.Time) {
	inA, _ := splitTeams(group, queueSize[group[0].mode]/2)
	sq := map[int64]string{}
	for i, t := range group {
		side := "B"
		if inA[i] {
			side = "A"
		}
		for _, m := range t.members {
			sq[m.id] = side
		}
	}
	mode, ids := group[0].mode, mapOrder()
	gmap := ids[randIndex(len(ids))]
	code := "mm-" + randHex(4)
	if q.open != nil {
		code = q.open(mode, gmap, sq)
	}
	for _, t := range group {
		for _, m := range t.members {
			q.found[m.id] = qFound{Room: code, Map: gmap, Mode: mode, Sq: sq[m.id], at: now}
		}
		q.dropLocked(t)
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func randIndex(n int) int {
	var b [1]byte
	rand.Read(b[:])
	return int(b[0]) % n
}

// ---- xona ----

// openMM: navbatdan topilgan o'yin uchun yopiq, egasiz, botsiz xona.
func (h *hub) openMM(mode, gmap string, sq map[int64]string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	code := "mm-" + randHex(4)
	for h.rooms["zn-"+code] != nil {
		code = "mm-" + randHex(4)
	}
	r := &room{name: "zn-" + code, clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), pub: false, max: queueSize[mode], bans: map[string]time.Time{},
		mm: &mmRoom{sq: sq}}
	r.gs.M = gmap
	r.gs.setMode(mode)
	h.rooms[r.name] = r
	go r.run(h)
	go h.reapMM(r) // hech kim kelmasa — o'chiriladi
	return code
}

// reapMM: navbat xonasi bir necha daqiqa bo'sh tursa o'chiriladi (qayta ulanish uchun vaqt).
func (h *hub) reapMM(r *room) {
	time.Sleep(mmEmptyTTL)
	h.mu.Lock()
	defer h.mu.Unlock()
	r.mu.Lock()
	empty := len(r.clients) == 0
	r.mu.Unlock()
	if empty && h.rooms[r.name] == r {
		delete(h.rooms, r.name)
		close(r.stop)
	}
}

// ---- API ----

func (a *api) queueGet(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Reytingli navbat uchun hisobga kiring")
		return
	}
	writeJSON(w, http.StatusOK, a.queue.status(u.ID))
}

func (a *api) queuePost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string   `json:"action"`
		Mode   string   `json:"mode"`
		Party  []string `json:"party"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Reytingli navbat uchun hisobga kiring")
		return
	}
	me := qMember{id: u.ID, name: u.Username, rating: u.Rating}
	var err error
	switch req.Action {
	case "join":
		if a.hub != nil {
			a.hub.mu.Lock()
			inGame := a.hub.socialLocked().inRoom[u.ID] != nil
			a.hub.mu.Unlock()
			if inGame {
				err = errors.New("Avval o'yindan chiqing")
				break
			}
		}
		var inv []qMember
		for _, name := range req.Party {
			if len(inv) >= 4 {
				break
			}
			id, real, e := a.st.userID(r.Context(), strings.TrimSpace(name))
			if e != nil || !a.st.areFriends(r.Context(), u.ID, id) {
				err = errNotFriends
				break
			}
			inv = append(inv, qMember{id: id, name: real})
		}
		if err == nil {
			err = a.queue.join(me, req.Mode, inv)
		}
	case "accept":
		err = a.queue.accept(me)
	case "decline":
		a.queue.decline(u.ID)
	case "leave":
		a.queue.leave(u.ID)
	default:
		err = errors.New("Noma'lum amal")
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.queue.status(u.ID))
}
