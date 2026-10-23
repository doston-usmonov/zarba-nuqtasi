package main

// O'yinni server tomonidan kuzatish. Server o'yinchilar yuborgan holatlardan
// (presence) faqat kerakli maydonlarni o'qiydi:
//   tm  — jamoa (T / CT), hc — to'liq o'yin mijozi, in — qaysi raundda tirilgan,
//   g   — xostning o'yin holati (raund, faza, hisob, o'yin identifikatori e),
//   de  — o'lim hodisasi (kim o'ldirdi, boshgami).
// Xost — xuddi mijozlardagidek: hc=1 va jamoasi bor mijozlar ichida eng kichik id.
// O'ldirish faqat o'lgan o'yinchining o'z xabari bilan hisoblanadi: o'zini
// "o'ldirdim" deb e'lon qilish hech narsa bermaydi.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type gState struct {
	R   int    `json:"r"`
	Ph  string `json:"ph"`
	ST  int    `json:"sT"`
	SCT int    `json:"sCT"`
	E   string `json:"e"`
}

type deathEv struct {
	N  int    `json:"n"`
	By string `json:"by"`
	HS int    `json:"hs"`
}

type presInfo struct {
	Tm string
	Nm string
	Hc int
	In int
	G  *gState
	De *deathEv
}

func parseInfo(m map[string]json.RawMessage) presInfo {
	var p presInfo
	json.Unmarshal(m["tm"], &p.Tm)
	json.Unmarshal(m["nm"], &p.Nm)
	json.Unmarshal(m["hc"], &p.Hc)
	json.Unmarshal(m["in"], &p.In)
	if raw, ok := m["g"]; ok && len(raw) > 0 && raw[0] == '{' {
		var g gState
		if json.Unmarshal(raw, &g) == nil {
			p.G = &g
		}
	}
	if raw, ok := m["de"]; ok && len(raw) > 0 && raw[0] == '{' {
		var d deathEv
		if json.Unmarshal(raw, &d) == nil {
			p.De = &d
		}
	}
	if p.Tm != "T" && p.Tm != "CT" {
		p.Tm = ""
	}
	return p
}

type tplayer struct {
	uid          int64
	name         string
	rounds       map[string]int // jamoa → o'ynagan raundlari
	last         int
	kills, death int
	hs           int
}

type tracker struct {
	epoch   string
	active  bool
	started time.Time
	round   int
	players map[string]*tplayer
}

type resultPlayer struct {
	UserID            int64
	Name, Team        string
	Kills, Deaths, HS int
}

type matchResult struct {
	Room            string
	Started, Ended  time.Time
	ScoreT, ScoreCT int
	Winner          string
	Players         []resultPlayer
}

// hostIDLocked — mijozlar xostni qanday tanlasa, server ham shunday tanlaydi.
func (r *room) hostIDLocked() string {
	host := ""
	for id, c := range r.clients {
		if c.info.Hc == 1 && c.info.Tm != "" && (host == "" || id < host) {
			host = id
		}
	}
	return host
}

func (t *tracker) player(c *client) *tplayer {
	key := "p" + c.id
	if c.acct != nil {
		key = "u" + c.acct.name
	}
	p := t.players[key]
	if p == nil {
		p = &tplayer{rounds: map[string]int{}}
		if c.acct != nil {
			p.uid, p.name = c.acct.id, c.acct.name
		} else {
			p.name = cleanGuestName(c.info.Nm)
		}
		t.players[key] = p
	}
	return p
}

func cleanGuestName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 16 {
		s = string(r[:16])
	}
	if s == "" {
		s = "Mehmon"
	}
	return s
}

// trackLocked c mijozning yangi holatini hisobga oladi. O'yin tugaganda natijani qaytaradi.
func (r *room) trackLocked(c *client, info presInfo, now time.Time) *matchResult {
	t := &r.tr
	var res *matchResult
	if g := info.G; g != nil && c.id == r.hostIDLocked() {
		switch {
		case g.Ph == "over":
			if t.active && g.E == t.epoch {
				res = t.finish(r.name, g, now)
			}
			t.active = false
		case g.Ph == "warmup":
			t.active = false // o'yinchilar chiqib ketdi — o'yin hisobga olinmaydi
		case g.E != "" && g.E != t.epoch && (g.Ph == "freeze" || g.Ph == "live" || g.Ph == "post"):
			*t = tracker{epoch: g.E, active: true, started: now, players: map[string]*tplayer{}}
		}
		t.round = g.R
	}

	died := false
	if de := info.De; de != nil {
		died = c.deSeen && de.N > c.lastDeN
		c.lastDeN = max(c.lastDeN, de.N)
	}
	c.deSeen = true
	if !t.active || info.Tm == "" {
		return res
	}
	if info.In > 0 && info.In == t.round {
		if p := t.player(c); p.last != info.In {
			p.last = info.In
			p.rounds[info.Tm]++
		}
	}
	if died {
		t.player(c).death++
		if k := r.clients[info.De.By]; k != nil && k != c && k.info.Tm != "" && k.info.Tm != info.Tm {
			kp := t.player(k)
			kp.kills++
			if info.De.HS == 1 {
				kp.hs++
			}
		}
	}
	return res
}

func (t *tracker) finish(room string, g *gState, now time.Time) *matchResult {
	if max(g.ST, g.SCT) < 8 || g.ST == g.SCT {
		return nil
	}
	if now.Sub(t.started) < time.Duration(g.ST+g.SCT)*5*time.Second {
		return nil // raundlar bunchalik tez tugamaydi
	}
	res := &matchResult{Room: room, Started: t.started, Ended: now, ScoreT: g.ST, ScoreCT: g.SCT, Winner: "CT"}
	if g.ST > g.SCT {
		res.Winner = "T"
	}
	sides := map[string]int{}
	for _, p := range t.players {
		team, best := "", 0
		for tm, n := range p.rounds {
			if n > best || (n == best && tm < team) {
				team, best = tm, n
			}
		}
		if best == 0 {
			continue
		}
		res.Players = append(res.Players, resultPlayer{UserID: p.uid, Name: p.name, Team: team, Kills: p.kills, Deaths: p.death, HS: p.hs})
		sides[team]++
	}
	if sides["T"] == 0 || sides["CT"] == 0 {
		return nil
	}
	sort.Slice(res.Players, func(i, j int) bool { return res.Players[i].Name < res.Players[j].Name })
	return res
}
