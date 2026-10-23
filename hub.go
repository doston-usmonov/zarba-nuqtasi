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
	accJSON json.RawMessage // boshqalarga ko'rsatiladigan hisob ma'lumoti (nom, reyting, daraja)
	info    presInfo
	deSeen  bool
	lastDeN int
}

type room struct {
	name    string
	mu      sync.Mutex
	clients map[string]*client
	tr      tracker
}

type hub struct {
	mu         sync.Mutex
	rooms      map[string]*room
	perIP      map[string]int
	maxPlayers int
	store      *Store   // nil yoki tayyor emas — hisoblar o'chirilgan
	voice      voiceCfg // ovozli chat uchun STUN/TURN
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

func accPublic(u User) json.RawMessage {
	rk := rankOf(u.Rating, u.Matches)
	return jsonMsg(map[string]any{"u": u.Username, "r": u.Rating, "rk": rk.Name, "rc": rk.Color, "cal": rk.Calibrating})
}

// serve runs one player's connection until it closes.
func (h *hub) serve(ws *wsConn, roomName, ip, host string, acct *account) {
	defer ws.close()
	var accJSON json.RawMessage
	if acct != nil && h.store.Ready() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		u, ok := h.store.User(ctx, acct.id)
		cancel()
		if ok {
			accJSON = accPublic(u)
		}
	}
	h.mu.Lock()
	if h.perIP[ip] >= perIPLimit {
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": "too_many_connections"}))
		return
	}
	r := h.rooms[roomName]
	if r == nil {
		r = &room{name: roomName, clients: map[string]*client{}}
		h.rooms[roomName] = r
	}
	r.mu.Lock()
	reject := ""
	if len(r.clients) >= h.maxPlayers {
		reject = "room_full"
	}
	for _, o := range r.clients {
		if acct != nil && o.acct != nil && o.acct.id == acct.id {
			reject = "already_in_room"
		}
	}
	if reject != "" {
		empty := len(r.clients) == 0
		r.mu.Unlock()
		if empty {
			delete(h.rooms, roomName)
		}
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": reject}))
		return
	}
	c := &client{id: newID(), ip: ip, room: r, ws: ws, send: make(chan []byte, sendQueue), acct: acct, accJSON: accJSON}
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
	delete(r.clients, c.id)
	empty := len(r.clients) == 0
	r.broadcast(jsonMsg(map[string]string{"t": "l", "id": c.id}), "")
	r.mu.Unlock()
	if empty {
		delete(h.rooms, r.name)
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
		if c.acct != nil {
			fields["nm"] = jsonMsg(c.acct.name)
		}
		info := parseInfo(fields)

		r := c.room
		r.mu.Lock()
		if c.accJSON != nil {
			fields["acc"] = c.accJSON
		}
		out, err := json.Marshal(fields)
		if err != nil {
			r.mu.Unlock()
			continue
		}
		c.pres, c.info = out, info
		res := r.trackLocked(c, info, now)
		var buf bytes.Buffer
		buf.WriteString(`{"t":"u","id":"`)
		buf.WriteString(c.id)
		buf.WriteString(`","p":`)
		buf.Write(out)
		buf.WriteByte('}')
		r.broadcast(buf.Bytes(), c.id)
		r.mu.Unlock()
		if res != nil && h.store.Ready() {
			go h.finishMatch(r, res)
		}
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
			fresh[id] = accPublic(u)
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
