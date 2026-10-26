package main

// Xonalar: ommaviy yoki yopiq, o'yinchilar chegarasi, xona egasi (xost) huquqlari.
//
// Xonani birinchi kirgan o'yinchi yaratadi va uning egasi bo'ladi (chiqib ketsa — eng oldin
// kirgan keyingi o'yinchiga o'tadi). Ega bot qo'sha oladi, bot yoki o'yinchini chiqarib yubora
// oladi (chiqarilgan o'yinchi shu xonaga 10 daqiqa kira olmaydi), xona sozlamalarini o'zgartiradi.
// Ommaviy xonalar /api/rooms ro'yxatida va bosh sahifada ko'rinadi, yopiqlariga faqat kodni
// bilganlar kiradi.

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const kickBan = 10 * time.Minute

var modeNames = map[string]string{modeComp: "Raqobat", modeDM: "Deathmatch", modeGG: "Qurol poygasi"}

type roomOpts struct {
	create bool // "yaratish" tugmasi: bunday xona bor bo'lsa — xato
	pub    bool
	max    int    // 0 — cheksiz
	gmap   string // xarita
	mode   string // o'yin rejimi
}

func parseRoomOpts(q map[string][]string) roomOpts {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	o := roomOpts{create: get("create") == "1", pub: get("pub") != "0", gmap: defaultMap}
	if validMap(get("map")) {
		o.gmap = get("map")
	}
	o.mode = modeComp
	if validMode(get("mode")) {
		o.mode = get("mode")
	}
	if m, err := strconv.Atoi(get("max")); err == nil && m >= 2 && m <= 64 {
		o.max = m
	}
	return o
}

func banKeys(c *client) []string {
	keys := []string{"ip:" + c.ip}
	if c.acct != nil {
		keys = append(keys, "u:"+strings.ToLower(c.acct.name))
	}
	return keys
}

func (r *room) bannedLocked(ip string, acct *account, now time.Time) bool {
	keys := []string{"ip:" + ip}
	if acct != nil {
		keys = append(keys, "u:"+strings.ToLower(acct.name))
	}
	for _, k := range keys {
		if t, ok := r.bans[k]; ok && now.Before(t) {
			return true
		}
	}
	return false
}

// pickOwnerLocked: egasi chiqib ketgan bo'lsa, eng oldin kirgan odamni ega qiladi.
func (r *room) pickOwnerLocked() bool {
	if o := r.clients[r.owner]; o != nil && o.bot == nil {
		return false
	}
	var best *client
	for _, c := range r.clients {
		if c.bot == nil && (best == nil || c.joined.Before(best.joined)) {
			best = c
		}
	}
	r.owner = ""
	if best != nil {
		r.owner = best.id
	}
	return true
}

func (r *room) infoMsgLocked() []byte {
	bots, humans := r.botCountLocked()
	return jsonMsg(map[string]any{"t": "room", "owner": r.owner, "pub": r.pub, "max": r.max, "bots": bots, "humans": humans})
}

// adminLocked: xona egasining buyruqlari.
func (h *hub) adminLocked(r *room, c *client, d json.RawMessage, now time.Time) {
	if c.id != r.owner {
		return
	}
	var m struct {
		A   string `json:"a"`
		ID  string `json:"id"`
		Sq  string `json:"sq"`
		Lv  string `json:"lv"`
		Pub *bool  `json:"pub"`
		Max *int   `json:"max"`
		M   string `json:"m"`
	}
	if json.Unmarshal(d, &m) != nil {
		return
	}
	switch m.A {
	case "bot":
		bots, humans := r.botCountLocked()
		if bots >= maxBots || (r.max > 0 && bots+humans >= r.max) || bots+humans >= h.maxPlayers {
			c.trySend(jsonMsg(map[string]string{"t": "note", "msg": "Xona to'la — bot qo'shib bo'lmaydi"}))
			return
		}
		r.addBotLocked(m.Sq, m.Lv, now)
	case "kick":
		t := r.clients[m.ID]
		if t == nil || t == c {
			return
		}
		if t.bot != nil {
			r.removeClientLocked(t)
			break
		}
		for _, k := range banKeys(t) {
			r.bans[k] = now.Add(kickBan)
		}
		t.trySend(jsonMsg(map[string]string{"t": "err", "code": "kicked"}))
		go func(t *client) { time.Sleep(200 * time.Millisecond); t.ws.close() }(t)
	case "mode":
		if !validMode(m.M) || m.M == r.gs.Md {
			return
		}
		r.changeModeLocked(m.M, now)
		r.broadcast(jsonMsg(map[string]string{"t": "note", "msg": "Rejim: " + modeNames[m.M] + " — o'yin isinishdan qayta boshlanadi"}), "")
	case "map":
		if !validMap(m.M) || m.M == r.gs.M {
			return
		}
		r.changeMapLocked(m.M, now)
		r.broadcast(jsonMsg(map[string]string{"t": "note", "msg": "Xarita: " + mapByID(m.M).name + " — o'yin isinishdan qayta boshlanadi"}), "")
	case "set":
		if m.Pub != nil {
			r.pub = *m.Pub
		}
		if m.Max != nil && (*m.Max == 0 || (*m.Max >= 2 && *m.Max <= 64)) {
			r.max = *m.Max
		}
	default:
		return
	}
	r.broadcast(r.infoMsgLocked(), "")
}

// changeMapLocked: xona boshqa xaritaga o'tadi — o'yin isinishdan boshlanadi, hamma yangi bazasida tug'iladi.
func (r *room) changeMapLocked(id string, now time.Time) {
	g := r.gs
	g.M = id
	g.reset()
	for _, c := range r.clients {
		if c.ps == nil {
			continue
		}
		if c.bot != nil {
			r.spawnBotLocked(c, now)
			continue
		}
		c.ps.posOK, c.ps.badSpawnAt = false, time.Time{}
	}
}

type roomListing struct {
	Code   string `json:"code"`
	Owner  string `json:"owner"`
	Humans int    `json:"humans"`
	Bots   int    `json:"bots"`
	Max    int    `json:"max"`
	Map    string `json:"map"`
	Mode   string `json:"mode"`
	Phase  string `json:"phase"`
	Round  int    `json:"round"`
	SA     int    `json:"sA"`
	SB     int    `json:"sB"`
	Fm     int    `json:"fm"`
}

// publicRooms: bosh sahifadagi ro'yxat uchun.
func (h *hub) publicRooms() []roomListing {
	h.mu.Lock()
	rooms := make([]*room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	h.mu.Unlock()
	out := []roomListing{}
	for _, r := range rooms {
		r.mu.Lock()
		if r.pub {
			bots, humans := r.botCountLocked()
			owner := ""
			if o := r.clients[r.owner]; o != nil {
				owner = cleanGuestName(o.info.Nm)
			}
			if humans > 0 {
				out = append(out, roomListing{Code: strings.TrimPrefix(r.name, "zn-"), Owner: owner, Humans: humans, Bots: bots, Max: r.max, Map: r.gs.M, Mode: r.gs.Md,
					Phase: r.gs.Ph, Round: r.gs.R, SA: r.gs.SA, SB: r.gs.SB, Fm: r.gs.Fm})
			}
		}
		r.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Humans != out[j].Humans {
			return out[i].Humans > out[j].Humans
		}
		return out[i].Code < out[j].Code
	})
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func (h *hub) roomsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"rooms": h.publicRooms()})
}
