package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

type client struct {
	id   string
	ip   string
	room *room
	ws   *wsConn
	send chan []byte
	pres json.RawMessage // guarded by room.mu
}

type room struct {
	name    string
	mu      sync.Mutex
	clients map[string]*client
}

type hub struct {
	mu         sync.Mutex
	rooms      map[string]*room
	perIP      map[string]int
	maxPlayers int
}

func newHub(maxPlayers int) *hub {
	return &hub{rooms: map[string]*room{}, perIP: map[string]int{}, maxPlayers: maxPlayers}
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

// serve runs one player's connection until it closes.
func (h *hub) serve(ws *wsConn, roomName, ip string) {
	defer ws.close()
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
	if len(r.clients) >= h.maxPlayers {
		r.mu.Unlock()
		h.mu.Unlock()
		ws.write(opText, jsonMsg(map[string]string{"t": "err", "code": "room_full"}))
		return
	}
	c := &client{id: newID(), ip: ip, room: r, ws: ws, send: make(chan []byte, sendQueue)}
	// greeting + everyone already here, queued before anyone else can write to us
	c.send <- jsonMsg(map[string]string{"t": "hello", "id": c.id})
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
	c.reader()
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

func (c *client) reader() {
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
			T string          `json:"t"`
			D json.RawMessage `json:"d"`
		}
		if json.Unmarshal(data, &m) != nil || m.T != "p" {
			continue
		}
		d := bytes.TrimSpace(m.D)
		if len(d) == 0 || len(d) > maxPresence || d[0] != '{' || !json.Valid(d) {
			continue
		}
		var buf bytes.Buffer
		buf.WriteString(`{"t":"u","id":"`)
		buf.WriteString(c.id)
		buf.WriteString(`","p":`)
		buf.Write(d)
		buf.WriteByte('}')
		r := c.room
		r.mu.Lock()
		c.pres = append(json.RawMessage(nil), d...)
		r.broadcast(buf.Bytes(), c.id)
		r.mu.Unlock()
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
