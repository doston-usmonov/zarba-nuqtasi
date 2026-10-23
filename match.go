package main

// O'yin natijasini kuzatish (reyting uchun). Raundni server o'zi yuritadi (referee.go),
// bu yerda faqat kim qaysi jamoada nechta raund o'ynagani, o'limlar va server tasdiqlagan
// o'ldirishlar yig'iladi. Presence'dan o'qiladigan maydonlar:
//   tm — hozirgi tomon (T / CT), sq — jamoa (A / B; yarim vaqtda tomon almashadi, jamoa emas),
//   hc — to'liq o'yin mijozi, in — qaysi raundda tirilgan.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const otHalf = 3 // overtime: har yarimi 3 raund, 4 g'alaba kerak

// validFinal — hisob haqiqiy o'yin oxiriga mos keladimi (MR12/MR8 + overtime).
func validFinal(fm, a, b int) bool {
	if fm != 8 && fm != 12 {
		return false
	}
	w, l := max(a, b), min(a, b)
	if w == fm+1 && l <= fm-1 {
		return true // asosiy vaqtda yutildi
	}
	return w >= fm+otHalf+1 && (w-fm-otHalf-1)%otHalf == 0 && l >= w-otHalf-1 && l <= w-2
}

type presInfo struct {
	Tm string
	Sq string
	Nm string
	Hc int
	In int
}

func parseInfo(m map[string]json.RawMessage) presInfo {
	var p presInfo
	json.Unmarshal(m["tm"], &p.Tm)
	json.Unmarshal(m["sq"], &p.Sq)
	json.Unmarshal(m["nm"], &p.Nm)
	json.Unmarshal(m["hc"], &p.Hc)
	json.Unmarshal(m["in"], &p.In)
	if p.Tm != "T" && p.Tm != "CT" {
		p.Tm = ""
	}
	if p.Sq != "A" && p.Sq != "B" {
		p.Sq = ""
	}
	return p
}

type tplayer struct {
	uid          int64
	name         string
	rounds       map[string]int // jamoa (A/B) → o'ynagan raundlari
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

// presence: o'yinchi shu raundda qatnashganini hisobga oladi (qaysi jamoada).
func (t *tracker) presence(c *client, info presInfo) {
	if !t.active || info.Sq == "" || info.In <= 0 || info.In != t.round {
		return
	}
	if p := t.player(c); p.last != info.In {
		p.last = info.In
		p.rounds[info.Sq]++
	}
}

// death: o'lim va (server tasdiqlagan bo'lsa) o'ldirgan o'yinchi.
func (t *tracker) death(r *room, victim, killer *client, hs bool) {
	if !t.active || victim.info.Sq == "" {
		return
	}
	t.player(victim).death++
	if killer != nil && killer != victim && killer.info.Sq != "" && killer.info.Sq != victim.info.Sq {
		kp := t.player(killer)
		kp.kills++
		if hs {
			kp.hs++
		}
	}
}

// phase: server raund holatini yangilagandan keyin chaqiriladi. O'yin tugasa natijani qaytaradi.
func (t *tracker) phase(r *room, prev string, now time.Time) *matchResult {
	g := r.gs
	var res *matchResult
	switch {
	case g.Ph == "over" && prev != "over":
		if t.active && g.E == t.epoch {
			res = t.finish(r.name, g, now)
		}
		t.active = false
	case g.Ph == "warmup":
		t.active = false // o'yinchilar chiqib ketdi yoki o'yin tugadi
	case g.E != "" && g.E != t.epoch && (g.Ph == "freeze" || g.Ph == "live" || g.Ph == "post"):
		*t = tracker{epoch: g.E, active: true, started: now, players: map[string]*tplayer{}}
	}
	t.round = g.R
	return res
}

func (t *tracker) finish(room string, g *gameState, now time.Time) *matchResult {
	if !validFinal(g.Fm, g.SA, g.SB) {
		return nil
	}
	if now.Sub(t.started) < time.Duration(g.SA+g.SB)*5*time.Second {
		return nil // raundlar bunchalik tez tugamaydi
	}
	// bazada jamoa boshlagan tomoni bilan saqlanadi: A — hujumdan, B — himoyadan boshlagan
	res := &matchResult{Room: room, Started: t.started, Ended: now, ScoreT: g.SA, ScoreCT: g.SB, Winner: "CT"}
	if g.SA > g.SB {
		res.Winner = "T"
	}
	sides := map[string]int{}
	for _, p := range t.players {
		sq, best := "", 0
		for s, n := range p.rounds {
			if n > best || (n == best && s < sq) {
				sq, best = s, n
			}
		}
		if best == 0 {
			continue
		}
		team := "T"
		if sq == "B" {
			team = "CT"
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
