package main

// «Zombi» rejimi (zm): raundlar, qayta tug'ilish, yuqtirish.
//
// Raund boshida hamma odam (Himoya rangida), qurollar bepul. 12 soniyadan keyin server tasodifan
// bir nechta o'yinchini (har 5 kishiga bittasi, kamida bitta) zombiga aylantiradi. Zombi faqat tirnog'i
// (pichoq) bilan uradi: bitta tegishi odamni yiqitadi, yiqilgan odam zombi bo'lib qayta tug'iladi. Zombining
// joni 400, o'lsa 2 soniyada qayta tug'iladi. Hamma odam yuqsa — zombilar yutadi; vaqt tugaguncha kimdir
// omon qolsa — odamlar yutadi va har omon qolgan +3 ochko oladi. Har yuqtirish va har o'ldirilgan zombi +1.
// 6 raunddan keyin eng ko'p ochko yig'gan g'olib. Kim zombi ekanini faqat server biladi (g.Zm).

import (
	"math/rand/v2"
	"time"
)

const modeZM = "zm"

var (
	zmRounds    = 6
	zmPrep      = 12.0  // soniya: odamlar tarqalib, yashirinadi
	zmRoundTime = 120.0 // yuqtirishdan keyin
	zmPost      = 6.0
)

const (
	zmHP   = 400.0
	zmClaw = 200.0 // zombi tirnog'ining zarari (odamni bir urishda yiqitadi)
)

func (g *gameState) zombie(id string) bool { return g.Md == modeZM && g.Zm[id] }

// zmPlayers: o'ynayotganlar (jamoasi bor, kuzatuvchi emas).
func (r *room) zmPlayersLocked() []*client {
	var out []*client
	for _, c := range r.clients {
		if !c.spec && c.info.Sq != "" && c.ps != nil {
			out = append(out, c)
		}
	}
	return out
}

func (r *room) tickZombieLocked(now time.Time, dt float64) {
	g := r.gs
	players := r.zmPlayersLocked()
	ready := len(players) >= 2
	if g.Ph != "warmup" && g.Ph != "over" && !ready {
		g.reset()
	}
	switch g.Ph {
	case "warmup":
		if !ready {
			g.Cd, g.T = 6, 0
			break
		}
		g.Cd -= dt
		g.T = max(0, g.Cd)
		if g.Cd <= 0 {
			g.R, g.SA, g.SB, g.B, g.Bm, g.E = 0, 0, 0, nil, -1, randEpoch()
			g.Sc, g.Wn, g.Wnm = map[string]int{}, "", ""
			r.matchAt = now
			r.zmRoundLocked(now)
		}
	case "live":
		g.T -= dt
		if len(g.Zm) == 0 { // tayyorgarlik: hali hech kim yuqmagan
			if g.T <= zmRoundTime {
				r.zmPatientZeroLocked(players, now)
			}
			break
		}
		humans := 0
		for _, c := range players {
			if g.Zm[c.id] {
				continue
			}
			if !c.ps.alive { // shu raundda yiqildi yoki yuqtirish boshlangandan keyin keldi — endi zombi
				r.zmInfectLocked(c)
				continue
			}
			if c.ps.alive {
				humans++
			}
		}
		switch {
		case humans == 0:
			g.W, g.Ph, g.T = "T", "post", zmPost
		case g.T <= 0:
			if g.Sc == nil {
				g.Sc = map[string]int{}
			}
			for _, c := range players {
				if !g.Zm[c.id] && c.ps.alive {
					g.Sc[c.id] += 3
				}
			}
			g.W, g.Ph, g.T = "CT", "post", zmPost
		}
	case "post":
		g.T -= dt
		if g.T <= 0 {
			if g.R >= zmRounds {
				r.finishModeLocked(nil)
			} else {
				r.zmRoundLocked(now)
			}
		}
	case "over":
		g.T -= dt
		if g.T <= 0 {
			r.applyVoteLocked(now)
		}
	default:
		g.reset()
	}
}

// zmRoundLocked: yangi raund — hamma yana odam, yangi joyda tug'iladi.
func (r *room) zmRoundLocked(now time.Time) {
	g := r.gs
	g.R++
	g.Ph, g.T, g.W, g.Zm = "live", zmPrep+zmRoundTime, "", nil
	g.liveAt = now
	for _, c := range r.clients {
		if c.bot != nil && c.info.Sq != "" {
			r.spawnBotLocked(c, now)
		}
	}
}

// zmPatientZeroLocked: tayyorgarlik tugadi — birinchi zombilar.
func (r *room) zmPatientZeroLocked(players []*client, now time.Time) {
	var alive []*client
	for _, c := range players {
		if c.ps.alive {
			alive = append(alive, c)
		}
	}
	if len(alive) == 0 {
		alive = players
	}
	n := max(1, len(players)/5)
	rand.Shuffle(len(alive), func(i, j int) { alive[i], alive[j] = alive[j], alive[i] })
	for _, c := range alive[:min(n, len(alive))] {
		r.zmInfectLocked(c)
		if c.ps.alive { // tirikligicha aylanadi
			c.ps.hp, c.ps.armor = zmHP, 0
			if b := c.bot; b != nil {
				b.weapon, b.mag, b.target, b.reloadAt = "knife", 0, "", time.Time{}
				c.info.Tm = "T"
			}
		}
	}
}

func (r *room) zmInfectLocked(c *client) {
	g := r.gs
	if g.Zm == nil {
		g.Zm = map[string]bool{}
	}
	g.Zm[c.id] = true
	if c.bot != nil {
		c.info.Tm = "T"
	}
}

// zmSpawnLocked: botning yangi hayoti (spawnBotLocked chaqiradi).
func (r *room) zmBotLifeLocked(c *client) {
	if !r.gs.zombie(c.id) {
		return
	}
	c.bot.weapon, c.bot.mag = "knife", 0
	c.ps.hp, c.ps.armor, c.ps.helmet = zmHP, 0, false
}
