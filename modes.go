package main

// O'yin rejimlari.
//
//   comp — raqobatli: raundlar, bomba, iqtisod, yarim vaqt (referee.go).
//   dm   — deathmatch: har kim har kimga qarshi, o'lgan zahoti (1,6 s) qayta tug'iladi,
//          6 daqiqada eng ko'p o'ldirgan yutadi.
//   gg   — qurol poygasi: ikki jamoa, qayta tug'ilish; har 2 o'ldirishda keyingi qurol,
//          oxirgi bosqich — pichoq, pichoq bilan o'ldirgan birinchi o'yinchi yutadi.
//
// Ikkala yangi rejimda ham hisobni server yuritadi: o'ldirish faqat server tasdiqlagan
// tegishdan keyin sanaladi (g.sc), qurol poygasida o'q faqat o'yinchining hozirgi quroli
// (yoki pichoq) bilan qabul qilinadi. Bu rejimlar reytingga yozilmaydi.

import (
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	modeComp = "comp"
	modeDM   = "dm"
	modeGG   = "gg"

	dmTime       = 360.0 // soniya
	ggTime       = 900.0
	respawnDelay = 1600 * time.Millisecond
	spawnShield  = 1500 * time.Millisecond // qayta tug'ilgandan keyin o'q o'tmaydi
	ggPerLevel   = 2
)

// qurol poygasi zinapoyasi (web/index.html dagi GG_LADDER bilan bir xil)
var ggLadder = []string{"rifle", "carbine", "smg", "sniper", "deagle", "pistol", "knife"}

func validMode(m string) bool { return m == modeComp || m == modeDM || m == modeGG }

func ggLevel(kills int) int     { return min(len(ggLadder)-1, max(0, kills)/ggPerLevel) }
func ggWeapon(kills int) string { return ggLadder[ggLevel(kills)] }
func ggWinKills() int           { return (len(ggLadder)-1)*ggPerLevel + 1 }

func (g *gameState) ffa() bool      { return g.Md == modeDM }
func (g *gameState) respawns() bool { return g.Md == modeDM || g.Md == modeGG }

// foes: a va b bir-biriga raqibmi (deathmatch'da — har kim).
func (r *room) foes(a, b *client) bool {
	if a == nil || b == nil || a == b || a.info.Sq == "" || b.info.Sq == "" {
		return false
	}
	return r.gs.ffa() || a.info.Sq != b.info.Sq
}

// spawnOKLocked: yangi hayot boshlanadigan joy (deathmatch'da — istalgan ochiq katak).
func (r *room) spawnOKLocked(p vec3, side string) bool {
	if r.gs.ffa() && r.gs.Ph == "live" {
		return !r.gm().botSolid(wc(p.X), wc(p.Z))
	}
	return side != "" && r.gm().inSpawn(p, side)
}

// dmSpawnLocked: raqiblardan iloji boricha uzoq ochiq joy.
func (r *room) dmSpawnLocked(c *client) vec3 {
	gm := r.gm()
	best, bd := vec3{}, -1.0
	for i := 0; i < 16; i++ {
		cell := gm.open[rand.IntN(len(gm.open))]
		p := cellCenter(cell)
		d := 1e9
		for _, o := range r.clients {
			if o != c && o.ps.alive && o.ps.posOK && r.foes(c, o) {
				d = math.Min(d, p.flat(o.ps.pos))
			}
		}
		d += rand.Float64() * 6 // bir xil joyda qayta-qayta tug'ilmaslik uchun
		if d > bd {
			best, bd = p, d
		}
	}
	best.X += (rand.Float64() - .5) * .6
	best.Z += (rand.Float64() - .5) * .6
	return best
}

// creditKillLocked: deathmatch / qurol poygasida server tasdiqlagan o'ldirish.
func (r *room) creditKillLocked(victim, killer *client, now time.Time) {
	g := r.gs
	if !g.respawns() || g.Ph != "live" || !r.foes(killer, victim) {
		return
	}
	if g.Sc == nil {
		g.Sc = map[string]int{}
	}
	g.Sc[killer.id]++
	k := g.Sc[killer.id]
	if g.Md != modeGG {
		return
	}
	if ggLevel(k) != ggLevel(k-1) {
		killer.ps.lvAt = now
		if b := killer.bot; b != nil {
			b.weapon = ggWeapon(k)
			b.mag = weaponMag[b.weapon]
			b.reloadAt = now.Add(400 * time.Millisecond) // qurol almashtirish
		}
	}
	if k >= ggWinKills() {
		r.finishModeLocked(killer)
	}
}

func clientName(c *client) string {
	if c.bot != nil {
		return c.bot.name
	}
	if c.acct != nil {
		return c.acct.name
	}
	return cleanGuestName(c.info.Nm)
}

// finishModeLocked: o'yin tugadi (g'olib nil bo'lsa — eng ko'p o'ldirgan).
func (r *room) finishModeLocked(w *client) {
	g := r.gs
	if w == nil {
		ids := make([]string, 0, len(g.Sc))
		for id := range g.Sc {
			if r.clients[id] != nil {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			if g.Sc[ids[i]] != g.Sc[ids[j]] {
				return g.Sc[ids[i]] > g.Sc[ids[j]]
			}
			return ids[i] < ids[j]
		})
		if len(ids) > 0 {
			w = r.clients[ids[0]]
		}
	}
	g.Ph, g.T, g.B = "over", 10, nil
	r.finalStatsLocked()
	g.Wn, g.Wnm = "", ""
	if w != nil {
		g.Wn, g.Wnm = w.id, clientName(w)
	}
}

// tickModeLocked: deathmatch va qurol poygasining vaqti va bosqichlari.
func (r *room) tickModeLocked(now time.Time, dt float64, nA, nB int) {
	g := r.gs
	ready := nA+nB >= 2
	if g.Md == modeGG {
		ready = nA > 0 && nB > 0
	}
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
		g.T = math.Max(0, g.Cd)
		if g.Cd <= 0 {
			g.R, g.Ph, g.SA, g.SB, g.B, g.Bm, g.E = 1, "live", 0, 0, nil, -1, randEpoch()
			g.T = dmTime
			if g.Md == modeGG {
				g.T = ggTime
			}
			g.Sc, g.Wn, g.Wnm = map[string]int{}, "", ""
			r.matchAt = now
		}
	case "live":
		g.T -= dt
		if g.T <= 0 {
			r.finishModeLocked(nil)
		}
	case "over":
		g.T -= dt
		if g.T <= 0 {
			g.reset()
		}
	default: // boshqa rejimdan qolgan bosqich
		g.reset()
	}
}

// changeModeLocked: xona boshqa rejimga o'tadi — o'yin isinishdan boshlanadi.
func (r *room) changeModeLocked(m string, now time.Time) {
	r.gs.Md = m
	r.gs.reset()
	for _, c := range r.clients {
		if c.bot != nil {
			r.spawnBotLocked(c, now)
		} else if c.ps != nil {
			c.ps.posOK, c.ps.badSpawnAt = false, time.Time{}
		}
	}
}

// botRoamLocked: qayta tug'iladigan rejimlarda bot xaritani aylanib raqib qidiradi.
func (r *room) botRoamLocked(c *client, now time.Time, dt float64) {
	b, p := c.bot, c.ps
	if !b.knownAt.IsZero() && now.Sub(b.knownAt) < 4*time.Second {
		if r.botGo(c, b.lastKnown, 4.4, dt, now) || p.pos.flat(b.lastKnown) < 1.2 {
			b.knownAt = time.Time{}
		}
		return
	}
	if b.mode != "roam" || b.goal.isZero() || p.pos.flat(b.goal) < 1.2 || (b.path == nil && now.After(b.planAt)) {
		b.mode = "roam"
		b.goal = r.gm().roamPoint()
		b.path = nil
	}
	r.botGo(c, b.goal, 4.2, dt, now)
}

// roamPoint: botlar boradigan joy — nuqtalar, himoya joylari yoki tasodifiy ochiq katak.
func (m *gameMap) roamPoint() vec3 {
	if rand.IntN(3) > 0 && len(m.holds) > 0 {
		return cellCenter(m.holds[rand.IntN(len(m.holds))][0])
	}
	return cellCenter(m.open[rand.IntN(len(m.open))])
}
