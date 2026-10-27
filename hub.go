package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"
	"time"
)

const (
	maxPresence = 8 << 10 // bitta o'yinchi holati, bayt
	sendQueue   = 256
	msgPerSec   = 60
	perIPLimit  = 8
	pingEvery   = 25 * time.Second
)

// account — /ws ulanishida cookie orqali aniqlangan hisob (nil — mehmon).
type account struct {
	id   int64
	name string
}

type client struct {
	id   string
	ip   string
	room *room
	ws   *wsConn
	send chan []byte
	acct *account
	// quyidagilar room.mu ostida
	pres    json.RawMessage
	accJSON json.RawMessage   // boshqalarga ko'rsatiladigan hisob ma'lumoti (nom, reyting, daraja)
	loadout map[string]string // qurol bo'yoqlari (room.mu ostida)
	vote    string            // o'yin oxirida tanlagan xaritasi
	info    presInfo
	deSeen  bool
	chatAt  time.Time // chat: oxirgi xabar va "chelak" (spamga qarshi)
	chatTok float64
	ps      *pstate   // server hakamining shu o'yinchi haqidagi ma'lumoti
	bot     *botBrain // nil — odam; aks holda server boshqaradigan bot
	prefFm  int       // o'yinchi sozlamasidagi format (8/12)
	joined  time.Time // xonaga kirgan vaqti
}

type room struct {
	name    string
	mu      sync.Mutex
	clients map[string]*client
	tr      tracker
	gs      *gameState    // server yuritadigan raund holati
	manual  bool          // testlar uchun: taymer raundni o'zi yuritmaydi
	stop    chan struct{} // xona yopilganda raund taymeri to'xtaydi
	owner   string        // xona egasi (bot qo'shadi, chiqaradi)
	pub     bool          // ommaviy — bosh sahifadagi ro'yxatda ko'rinadi
	max     int           // o'yinchilar (botlar bilan) chegarasi, 0 — cheksiz
	bans    map[string]time.Time
	// official: doimiy xona (officials.go) — egasi yo'q, bo'shasa ham yo'qolmaydi
	official *officialRoom
	// statistika (stats.go)
	st               map[string]*pstat
	stE              string
	planter, defuser string
	// granatalar va iqtisod (nadesim.go, economy.go)
	nades   []*liveNade
	smokes  []*smokeCloud
	fires   []*fireArea
	nadeKey string
	ground  map[string]groundGun
	groundR int
	groundE string
	loss    map[string]int
	lossE   string
	purses  map[string]purse // chiqib ketganlarning puli (qayta ulanish uchun)
	matchAt time.Time        // o'yin boshlangan vaqt (XP uchun)
	gains   []matchGain      // o'yin oxirida yoziladigan XP
}

type hub struct {
	mu         sync.Mutex
	rooms      map[string]*room
	perIP      map[string]int
	maxPlayers int
	store      *Store                      // nil yoki tayyor emas — hisoblar o'chirilgan
	voice      voiceCfg                    // ovozli chat uchun STUN/TURN
	debug      bool                        // ZARBA_DEBUG=1: e2e testlar raund holatini o'zgartira oladi (prod'da o'chiq)
	loadouts   map[int64]map[string]string // hisob → qurol bo'yoqlari (progress.go)
	soc        *social                     // kim qaysi xonada, xonaga chaqiriqlar (social.go)
}

func newHub(maxPlayers int, st *Store) *hub {
	return &hub{rooms: map[string]*room{}, perIP: map[string]int{}, maxPlayers: maxPlayers, store: st}
}

func newID() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (h *hub) stats() (rooms, players int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.rooms {
		r.mu.Lock()
		players += len(r.clients)
		r.mu.Unlock()
	}
	return len(h.rooms), players
}

func jsonMsg(v any) []byte { b, _ := json.Marshal(v); return b }

func accPublic(u User, clan string) json.RawMessage {
	rk := rankOf(u.Rating, u.Matches)
	lv, _, _ := levelOf(u.XP)
	m := map[string]any{"u": u.Username, "r": u.Rating, "rk": rk.Name, "rc": rk.Color, "cal": rk.Calibrating, "lv": lv}
	if clan != "" {
		m["cl"] = clan
	}
	return jsonMsg(m)
}

// serve runs one player's connection until it closes.
func (h *hub) serve(ws *wsConn, roomName, ip, host string, acct *account, opts roomOpts) {
	defer ws.close()
	var accJSON json.RawMessage
	var lo map[string]string
	if acct != nil && h.store.Ready() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		u, ok := h.store.User(ctx, acct.id)
		lo, _ = h.store.Loadout(ctx, acct.id)
		tag := h.store.ClanTag(ctx, acct.id)
		cancel()
		if ok {
			accJSON = accPublic(u, tag)
		}
		if lo != nil {
			h.mu.Lock()
			if h.loadouts == nil {
				h.loadouts = map[int64]map[string]string{}
			}
			h.loadouts[acct.id] = lo
			h.mu.Unlock()
		}
	}
	h.mu.Lock()
	if h.perIP[ip] >= perIPLimit {
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": "too_many_connections"}))
		return
	}
	r := h.rooms[roomName]
	if r != nil && opts.create {
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": "room_exists"}))
		return
	}
	if r == nil {
		r = &room{name: roomName, clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), pub: opts.pub, max: opts.max, bans: map[string]time.Time{}}
		if opts.gmap != "" {
			r.gs.M = opts.gmap
		}
		if opts.mode != "" {
			r.gs.setMode(opts.mode)
		}
		if r.gs.Md == modeWM && (r.max == 0 || r.max > 4) {
			r.max = 4 // Wingman: 2 ga 2
		}
		h.rooms[roomName] = r
		go r.run(h)
	}
	r.mu.Lock()
	reject := ""
	n := len(r.clients)
	if r.official != nil { // rasmiy xonada botlar odamga joy bo'shatadi
		_, n = r.botCountLocked()
	}
	if n >= h.maxPlayers || (r.max > 0 && n >= r.max) {
		reject = "room_full"
	}
	if r.bannedLocked(ip, acct, time.Now()) {
		reject = "kicked"
	}
	for _, o := range r.clients {
		if acct != nil && o.acct != nil && o.acct.id == acct.id {
			reject = "already_in_room"
		}
	}
	if reject != "" {
		empty := len(r.clients) == 0 && r.official == nil
		r.mu.Unlock()
		if empty {
			delete(h.rooms, roomName)
			close(r.stop)
		}
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": reject}))
		return
	}
	c := &client{id: newID(), ip: ip, room: r, ws: ws, send: make(chan []byte, sendQueue), acct: acct, accJSON: accJSON, loadout: lo, ps: newPState(), joined: time.Now()}
	// greeting + everyone already here, queued before anyone else can write to us
	c.send <- jsonMsg(map[string]any{"t": "hello", "id": c.id, "ice": h.voice.iceFor(host, c.id, time.Now())})
	type peer struct {
		ID string          `json:"id"`
		P  json.RawMessage `json:"p"`
	}
	snap := []peer{}
	for _, o := range r.clients {
		if o.pres != nil {
			snap = append(snap, peer{o.id, o.pres})
		}
	}
	c.send <- jsonMsg(map[string]any{"t": "s", "peers": snap})
	r.clients[c.id] = c
	if acct != nil {
		h.joinedLocked(acct.id, r)
	}
	if r.owner == "" && r.official == nil {
		r.owner = c.id
	}
	r.fillBotsLocked(time.Now())
	r.broadcast(r.infoMsgLocked(), "")
	r.mu.Unlock()
	h.perIP[ip]++
	h.mu.Unlock()

	done := make(chan struct{})
	go c.writer(done)
	h.reader(c)
	close(done)
	h.leave(c)
}

func (h *hub) leave(c *client) {
	r := c.room
	h.mu.Lock()
	r.mu.Lock()
	r.savePurseLocked(c)
	delete(r.clients, c.id)
	if c.acct != nil {
		h.leftLocked(c.acct.id, r)
	}
	_, humans := r.botCountLocked()
	empty := humans == 0 // botlar o'zi xonani ushlab turmaydi
	r.broadcast(jsonMsg(map[string]string{"t": "l", "id": c.id}), "")
	if r.official != nil {
		if empty {
			r.emptyOfficialLocked()
		} else {
			r.fillBotsLocked(time.Now())
			r.broadcast(r.infoMsgLocked(), "")
		}
		empty = false // rasmiy xona yo'qolmaydi
	} else if !empty && r.pickOwnerLocked() {
		r.broadcast(r.infoMsgLocked(), "")
	}
	r.mu.Unlock()
	if empty {
		delete(h.rooms, r.name)
		close(r.stop)
	}
	if h.perIP[c.ip]--; h.perIP[c.ip] <= 0 {
		delete(h.perIP, c.ip)
	}
	h.mu.Unlock()
}

// broadcast must be called with r.mu held. Slow receivers lose presence updates (newer ones follow).
func (r *room) broadcast(msg []byte, except string) {
	for id, o := range r.clients {
		if id == except {
			continue
		}
		select {
		case o.send <- msg:
		default:
		}
	}
}

func (h *hub) reader(c *client) {
	tokens, last := float64(msgPerSec), time.Now()
	for {
		data, err := c.ws.read(maxPresence + 256)
		if err != nil {
			return
		}
		now := time.Now()
		tokens += now.Sub(last).Seconds() * msgPerSec
		last = now
		if tokens > msgPerSec {
			tokens = msgPerSec
		}
		if tokens < 1 {
			continue // too chatty: drop
		}
		tokens--
		var m struct {
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
			To string          `json:"to"`
			C  json.Number     `json:"c"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.T == "ping" { // mijoz ping'ni o'lchaydi
			if f, err := m.C.Float64(); err == nil {
				select {
				case c.send <- jsonMsg(map[string]any{"t": "pong", "c": f}):
				default:
				}
			}
			continue
		}
		if m.T == "dbg" && h.debug {
			var g gameState
			if json.Unmarshal(m.D, &g) == nil {
				r := c.room
				r.mu.Lock()
				live := r.gs.liveAt
				*r.gs = g
				r.gs.liveAt = live
				if g.Ph == "live" && live.IsZero() {
					r.gs.liveAt = time.Now()
				}
				r.mu.Unlock()
			}
			continue
		}
		if m.T == "vote" { // o'yin oxirida keyingi xarita uchun ovoz
			r := c.room
			r.mu.Lock()
			r.voteLocked(c, m.D)
			r.mu.Unlock()
			continue
		}
		if m.T == "chat" { // matnli chat: hammaga yoki faqat jamoaga
			r := c.room
			r.mu.Lock()
			r.chatLocked(c, m.D, now)
			r.mu.Unlock()
			continue
		}
		if m.T == "adm" { // xona egasining buyrug'i
			r := c.room
			r.mu.Lock()
			h.adminLocked(r, c, m.D, now)
			r.mu.Unlock()
			continue
		}
		if m.T == "sig" { // ovozli chat ulanishi
			c.room.relaySignal(c, m.To, bytes.TrimSpace(m.D))
			continue
		}
		if m.T != "p" {
			continue
		}
		d := bytes.TrimSpace(m.D)
		if len(d) == 0 || len(d) > maxPresence || d[0] != '{' {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(d, &fields) != nil {
			continue
		}
		// ism va hisob ma'lumotini server belgilaydi: boshqa o'yinchi nomini olib bo'lmaydi
		delete(fields, "acc")
		delete(fields, "g") // raundni server yuritadi
		if c.acct != nil {
			fields["nm"] = jsonMsg(c.acct.name)
		}
		info := parseInfo(fields)

		r := c.room
		r.mu.Lock()
		c.info = info
		r.refereeLocked(c, fields, info, now)
		c.deSeen = true
		if c.accJSON != nil {
			fields["acc"] = c.accJSON
		}
		// qurol bo'yog'ini ham server yozadi (faqat ochilganlari)
		delete(fields, "fn")
		if c.loadout != nil {
			var w string
			json.Unmarshal(fields["w"], &w)
			if f := c.loadout[w]; f != "" {
				fields["fn"] = jsonMsg(f)
			}
		}
		out, err := json.Marshal(fields)
		if err != nil {
			r.mu.Unlock()
			continue
		}
		c.pres, c.info = out, info
		r.tr.presence(c, info)
		var buf bytes.Buffer
		buf.WriteString(`{"t":"u","id":"`)
		buf.WriteString(c.id)
		buf.WriteString(`","p":`)
		buf.Write(out)
		buf.WriteByte('}')
		r.broadcast(buf.Bytes(), c.id)
		r.mu.Unlock()
	}
}

// finishMatch reytinglarni yozadi va o'yinchilarga natijani yuboradi.
func (h *hub) finishMatch(r *room, res *matchResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	changes, err := h.store.ApplyMatch(ctx, res)
	if err != nil {
		log.Printf("o'yin natijasi saqlanmadi (%s): %v", r.name, err)
		return
	}
	if len(changes) == 0 {
		return
	}
	log.Printf("o'yin tugadi: xona=%s hisob=%d:%d g'olib=%s reytingli o'yinchilar=%d", r.name, res.ScoreT, res.ScoreCT, res.Winner, len(changes))
	fresh := map[int64]json.RawMessage{}
	for id := range changes {
		if u, ok := h.store.User(ctx, id); ok {
			fresh[id] = accPublic(u, h.store.ClanTag(ctx, id))
		}
	}
	r.mu.Lock() // bazaga murojaatlar qulfdan tashqarida
	defer r.mu.Unlock()
	for _, c := range r.clients {
		if c.acct == nil {
			continue
		}
		ch, ok := changes[c.acct.id]
		if !ok {
			continue
		}
		if a := fresh[c.acct.id]; a != nil {
			c.accJSON = a
		}
		msg := jsonMsg(map[string]any{"t": "rt", "before": ch.Before, "after": ch.After, "delta": ch.Delta, "won": ch.Won,
			"rank": rankOf(ch.After, ch.Matches), "matches": ch.Matches})
		select {
		case c.send <- msg:
		default:
		}
	}
}

func (c *client) writer(done chan struct{}) {
	t := time.NewTicker(pingEvery)
	defer t.Stop()
	for {
		select {
		case msg := <-c.send:
			if c.ws.write(opText, msg) != nil {
				c.ws.close()
				return
			}
		case <-t.C:
			if c.ws.write(opPing, nil) != nil {
				c.ws.close()
				return
			}
		case <-done:
			return
		}
	}
}
