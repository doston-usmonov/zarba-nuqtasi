package main

// Doimiy («rasmiy») xonalar — Play-CS'dagi 24/7 serverlar kabi.
//
// OFFICIAL_ROOMS="bekat:comp:16,qumtepa:dm:20,..." (xarita:rejim:chegara; "-" — o'chiq).
// Kodi avtomatik: <xarita>-<rejim> (bekat-comp). Server ishga tushganda yaratiladi va hamma
// chiqib ketsa ham yo'qolmaydi. Egasi yo'q: xona egasi buyruqlari ishlamaydi, ommaviylik va
// chegara o'zgarmaydi, xaritani faqat o'yin oxiridagi ovoz berish almashtiradi.
//
// Botlar bilan to'ldirish: xonada kamida bitta odam bo'lsa, odam + bot = min(chegara, 10) gacha
// o'rta darajadagi botlar qo'shiladi, har yangi odam bitta botning o'rnini oladi (jamoalar teng
// saqlanadi). Odam qolmasa botlar ham chiqadi va o'yin boshidan boshlanadi — bo'sh xona CPU yemaydi.
// Bot qatnashgan o'yin reytingga yozilmaydi (match.go), shuning uchun bular «tez o'ynash» uchun.

import (
	"log"
	"strconv"
	"strings"
	"time"
)

const defaultOfficialRooms = "bekat:comp:16,qumtepa:dm:20,bozor:comp:10,saroy:gg:12,karvon:wm:4,maydon:arena:10,zavod:comp:10,koshk:comp:10"

type officialRoom struct {
	gmap, mode string
	max        int
}

func (o officialRoom) code() string { return o.gmap + "-" + o.mode }

// parseOfficialRooms: noto'g'ri bandlar tashlab yuboriladi (log bilan).
func parseOfficialRooms(s string) []officialRoom {
	s = strings.TrimSpace(s)
	if s == "-" {
		return nil
	}
	if s == "" {
		s = defaultOfficialRooms
	}
	var out []officialRoom
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		f := strings.Split(strings.TrimSpace(part), ":")
		if len(f) != 3 {
			if strings.TrimSpace(part) != "" {
				log.Printf("OFFICIAL_ROOMS: %q tushunarsiz (kerak: xarita:rejim:chegara)", part)
			}
			continue
		}
		o := officialRoom{gmap: strings.ToLower(f[0]), mode: strings.ToLower(f[1])}
		if o.gmap == "registon" { // eski sozlamalar: Registon o'rniga Bekat
			o.gmap = "bekat"
		}
		o.max, _ = strconv.Atoi(f[2])
		if !validMap(o.gmap) || isCustomMapID(o.gmap) || !validMode(o.mode) || o.max < 2 || o.max > 64 || seen[o.code()] || isAimMap(o.gmap) != (o.mode == modeAim) {
			log.Printf("OFFICIAL_ROOMS: %q o'tkazib yuborildi (xarita, rejim yoki chegara noto'g'ri, yoki takror)", part)
			continue
		}
		if o.mode == modeWM && o.max > 4 {
			o.max = 4 // Wingman: 2 ga 2
		}
		seen[o.code()] = true
		out = append(out, o)
	}
	return out
}

// startOfficial: rasmiy xonalarni yaratadi (bor bo'lsa tegmaydi).
func (h *hub) startOfficial(list []officialRoom) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, o := range list {
		name := "zn-" + o.code()
		if h.rooms[name] != nil {
			continue
		}
		r := &room{name: name, clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), pub: true, max: o.max, bans: map[string]time.Time{}, official: &o}
		r.gs.M = o.gmap
		r.gs.setMode(o.mode)
		r.gs.setAtm(randomAtm())
		h.rooms[name] = r
		go r.run(h)
	}
	if len(list) > 0 {
		log.Printf("rasmiy xonalar: %d ta", len(list))
	}
}

// botTarget: shuncha odam bo'lganda xonada nechta bot bo'lishi kerak.
func (r *room) botTarget(humans int) int {
	if humans == 0 {
		return 0
	}
	cap := r.max
	if cap <= 0 || cap > maxBots {
		cap = maxBots
	}
	if n := cap - humans; n > 0 {
		return n
	}
	return 0
}

// fillBotsLocked: rasmiy xonada botlar sonini va jamoalar muvozanatini to'g'rilaydi.
func (r *room) fillBotsLocked(now time.Time) {
	if r.official == nil {
		return
	}
	for i := 0; i < 2*maxBots; i++ { // har qadamda bitta bot qo'shiladi yoki chiqadi
		bots, humans := r.botCountLocked()
		want := r.botTarget(humans)
		cnt, botsIn := map[string]int{}, map[string]int{}
		for _, c := range r.clients {
			if c.info.Sq == "A" || c.info.Sq == "B" {
				cnt[c.info.Sq]++
				if c.bot != nil {
					botsIn[c.info.Sq]++
				}
			}
		}
		big, small := "A", "B"
		if cnt["B"] > cnt["A"] || (cnt["B"] == cnt["A"] && botsIn["B"] > botsIn["A"]) {
			big, small = "B", "A"
		}
		switch {
		case bots > want:
			if botsIn[big] == 0 {
				big = small
			}
			r.dropBotLocked(big)
		case bots < want:
			r.addBotLocked(small, "normal", now)
		case cnt[big]-cnt[small] >= 2 && botsIn[big] > 0 && r.gs.Ph != "live":
			// odam jamoa tanlagach muvozanat buzildi: raundlar orasida botni boshqa jamoaga o'tkazamiz
			r.dropBotLocked(big)
			r.addBotLocked(small, "normal", now)
			return
		default:
			return
		}
	}
}

// dropBotLocked: shu jamoadan bitta botni chiqaradi — iloji bo'lsa o'liklaridan.
func (r *room) dropBotLocked(sq string) {
	var pick *client
	for _, c := range r.clients {
		if c.bot == nil || c.info.Sq != sq {
			continue
		}
		if pick == nil || (c.ps != nil && !c.ps.alive && (pick.ps == nil || pick.ps.alive)) {
			pick = c
		}
	}
	if pick == nil { // jamoasi yo'q bot
		for _, c := range r.clients {
			if c.bot != nil {
				pick = c
				break
			}
		}
	}
	if pick != nil {
		r.removeClientLocked(pick)
	}
}

// emptyOfficialLocked: oxirgi odam chiqdi — botlar ketadi, xona o'z xaritasida boshidan boshlanadi.
func (r *room) emptyOfficialLocked() {
	for _, c := range r.clients {
		if c.bot != nil {
			r.removeClientLocked(c)
		}
	}
	r.gs.M = r.official.gmap
	r.gs.At, r.gs.Wx = "", ""
	r.gs.setAtm(randomAtm())
	r.gs.setMode(r.official.mode)
	r.gs.reset()
	r.st, r.stE, r.planter, r.defuser = nil, "", "", ""
	r.nades, r.smokes, r.fires, r.nadeKey = nil, nil, nil, ""
	r.ground, r.groundR, r.groundE = nil, 0, ""
	r.loss, r.lossE, r.purses = nil, "", nil
	r.matchAt, r.gains = time.Time{}, nil
	r.bans = map[string]time.Time{}
}
