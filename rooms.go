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

var modeNames = map[string]string{modeComp: "Raqobat", modeDM: "Deathmatch", modeTDM: "Jamoaviy deathmatch", modeGG: "Qurol poygasi", modeWM: "Wingman", modeRT: "Retake", modeAim: "Aim"}

type roomOpts struct {
	create bool // "yaratish" tugmasi: bunday xona bor bo'lsa — xato
	pub    bool
	max    int    // 0 — cheksiz
	gmap   string // xarita
	mode   string // o'yin rejimi
	spec   bool   // kuzatuvchi bo'lib kirish (spectate.go)
	at, wx string // kun vaqti va ob-havo (atm.go)
}

func parseRoomOpts(q map[string][]string) roomOpts {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	o := roomOpts{create: get("create") == "1", pub: get("pub") != "0", gmap: defaultMap, spec: get("spec") == "1"}
	if validMap(get("map")) {
		o.gmap = get("map")
	}
	o.mode = modeComp
	if validMode(get("mode")) {
		o.mode = get("mode")
	}
	o.gmap, o.mode = pairMapMode(o.gmap, o.mode, validMode(get("mode")) && !validMap(get("map")))
	if m, err := strconv.Atoi(get("max")); err == nil && m >= 2 && m <= 64 {
		o.max = m
	}
	o.at, o.wx = atmOpt(get("time"), get("wx"))
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
// ownerless: rasmiy va navbat xonalarida ega yo'q — xona buyruqlari ishlamaydi.
func (r *room) ownerless() bool { return r.official != nil || r.mm != nil || r.trn != nil }

func (r *room) pickOwnerLocked() bool {
	if r.ownerless() {
		return false
	}
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
	return jsonMsg(map[string]any{"t": "room", "owner": r.owner, "pub": r.pub, "max": r.max, "bots": bots, "humans": humans, "official": r.official != nil, "mm": r.mm != nil})
}

// adminLocked: xona egasining buyruqlari.
func (h *hub) adminLocked(r *room, c *client, d json.RawMessage, now time.Time) {
	if r.ownerless() || c.id != r.owner { // rasmiy va navbat xonalarida egasi yo'q — buyruqlar ishlamaydi
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
		At  string `json:"at"`
		Wx  string `json:"wx"`
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
		if !validMode(m.M) || m.M == r.gs.modeID() {
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
	case "atm": // kun vaqti va ob-havo: o'yin to'xtamaydi
		at, wx := atmOpt(m.At, m.Wx)
		r.gs.setAtm(at, wx) // holat 100 ms ichida hammaga boradi
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
	if _, m := pairMapMode(id, g.modeID(), false); m != g.modeID() { // aim xaritasiga o'tsa — Aim rejimi va aksincha
		g.setMode(m)
	}
	g.M = id
	if r.official != nil { // rasmiy xonalarda har o'yin — yangi havo
		g.At, g.Wx = "", ""
		g.setAtm(randomAtm())
	}
	if isCustomMapID(id) {
		r.cmap = customMaps.get(id)
	}
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
	// Official: doimiy xona — odam bo'lmasa ham ro'yxatda
	Official bool   `json:"official"`
	Region   string `json:"region"`            // hozircha bitta server; keyin ikkinchisi qo'shilsa kerak bo'ladi
	Spec     int    `json:"spec"`              // kuzatuvchilar
	MM       bool   `json:"mm"`                // reytingli navbat o'yini (kuzatib bo'lmaydi)
	MapName  string `json:"mapName,omitempty"` // foydalanuvchi xaritasining nomi
	At       string `json:"at,omitempty"`      // kun vaqti va ob-havo (atm.go)
	Wx       string `json:"wx,omitempty"`
}

func (r *room) customNameLocked() string {
	if isCustomMapID(r.gs.M) {
		return r.gm().name
	}
	return ""
}

// listingLocked: ro'yxatdagi bitta qator (r.mu ushlangan holda).
func (r *room) listingLocked() roomListing {
	bots, humans := r.botCountLocked()
	owner := ""
	if o := r.clients[r.owner]; o != nil {
		owner = cleanGuestName(o.info.Nm)
	}
	return roomListing{Code: strings.TrimPrefix(r.name, "zn-"), Owner: owner, Humans: humans, Bots: bots, Max: r.max, Map: r.gs.M, Mode: r.gs.modeID(),
		Phase: r.gs.Ph, Round: r.gs.R, SA: r.gs.SA, SB: r.gs.SB, Fm: r.gs.Fm, Official: r.official != nil, Spec: r.specCountLocked(), MM: r.mm != nil, MapName: r.customNameLocked(), At: r.gs.At, Wx: r.gs.Wx}
}

func (h *hub) roomList() []*room {
	h.mu.Lock()
	defer h.mu.Unlock()
	rooms := make([]*room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	return rooms
}

// publicRooms: bosh sahifadagi ro'yxat uchun.
func (h *hub) publicRooms() []roomListing {
	out := []roomListing{}
	for _, r := range h.roomList() {
		r.mu.Lock()
		if r.pub {
			if l := r.listingLocked(); l.Humans > 0 || l.Official {
				out = append(out, l)
			}
		}
		r.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Official != out[j].Official { // rasmiy xonalar tepada
			return out[i].Official
		}
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

// mapCounts: har xaritada hozir nechta odam o'ynayapti (yopiq xonalar ham — faqat son).
func (h *hub) mapCounts() map[string]int {
	out := map[string]int{}
	for _, m := range mapOrder() {
		out[m] = 0
	}
	for _, r := range h.roomList() {
		r.mu.Lock()
		_, humans := r.botCountLocked()
		out[r.gs.M] += humans
		r.mu.Unlock()
	}
	return out
}

func (h *hub) roomsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"rooms": h.publicRooms(), "byMap": h.mapCounts()})
}

// roomPlayer: xona oynasidagi o'yinchi.
type roomPlayer struct {
	Name string `json:"name"`
	Tag  string `json:"tag,omitempty"` // klan tegi
	Team string `json:"team"`          // T, CT yoki "" (hali tanlamagan)
	K    int    `json:"k"`
	D    int    `json:"d"`
	Bot  bool   `json:"bot"`
}

type roomDetail struct {
	roomListing
	Players []roomPlayer `json:"players"`
}

// roomDetailLocked: ro'yxatdagi maydonlar + o'yinchilar (hisob joriy o'yindan).
func (r *room) roomDetailLocked() roomDetail {
	d := roomDetail{roomListing: r.listingLocked(), Players: []roomPlayer{}}
	for _, c := range r.clients {
		if c.spec {
			continue
		}
		p := roomPlayer{Name: clientName(c), Team: c.info.Tm, Bot: c.bot != nil}
		if s := r.st[c.id]; s != nil && r.stE == r.gs.E {
			p.K, p.D = s.k, s.d
		}
		if len(c.accJSON) > 0 {
			var a struct {
				Cl string `json:"cl"`
			}
			if json.Unmarshal(c.accJSON, &a) == nil {
				p.Tag = a.Cl
			}
		}
		d.Players = append(d.Players, p)
	}
	sort.Slice(d.Players, func(i, j int) bool {
		a, b := d.Players[i], d.Players[j]
		if a.Team != b.Team {
			return a.Team > b.Team // T, keyin CT, keyin tanlamaganlar
		}
		if a.K != b.K {
			return a.K > b.K
		}
		return a.Name < b.Name
	})
	return d
}

// roomHandler: GET /api/rooms/{kod} — yopiq xona ham, kodi ma'lum bo'lsa.
func (h *hub) roomHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	code := req.PathValue("code")
	if !roomName.MatchString("zn-" + code) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
		return
	}
	h.mu.Lock()
	r := h.rooms["zn-"+code]
	h.mu.Unlock()
	if r == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
		return
	}
	r.mu.Lock()
	d := r.roomDetailLocked()
	r.mu.Unlock()
	json.NewEncoder(w).Encode(d)
}
