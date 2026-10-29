package main

// O'yin rejimlari.
//
//   comp — raqobatli: raundlar, bomba, iqtisod, yarim vaqt (referee.go).
//   dm   — deathmatch: har kim har kimga qarshi, o'lgan zahoti (1,6 s) qayta tug'iladi,
//          6 daqiqada eng ko'p o'ldirgan yutadi.
//   tdm  — jamoaviy deathmatch (ichkarida dm + g.Tm): ikki jamoa, faqat raqibni o'ldirish
//          mumkin, 6 daqiqada ko'proq o'ldirgan jamoa yutadi.
//   gg   — qurol poygasi: ikki jamoa, qayta tug'ilish; eng kuchli quroldan (snayper)
//          boshlab har 2 o'ldirishda kuchsizrog'iga o'tiladi, oxirgi bosqich — pichoq, pichoq bilan o'ldirgan birinchi o'yinchi yutadi.
//
// Ikkala yangi rejimda ham hisobni server yuritadi: o'ldirish faqat server tasdiqlagan
// tegishdan keyin sanaladi (g.sc), qurol poygasida o'q faqat o'yinchining hozirgi quroli
// (yoki pichoq) bilan qabul qilinadi. Bu rejimlar reytingga yozilmaydi.

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	modeComp = "comp"
	modeDM   = "dm"
	modeGG   = "gg"
	modeTDM  = "tdm"   // tashqi nomi: xona yaratish va admin; ichkarida Md=dm, Tm=true
	modeAim  = "arena" // aim xaritalarida (brauzerdagi "aim" — nishon mashqi, shuning uchun boshqa nom): raundlar, bomba yo'q, qurollar bepul

	dmTime       = 360.0 // soniya
	ggTime       = 900.0
	respawnDelay = 1600 * time.Millisecond
	spawnShield  = 1500 * time.Millisecond // qayta tug'ilgandan keyin o'q o'tmaydi
	ggPerLevel   = 2
)

// qurol poygasi zinapoyasi: eng kuchlisidan kuchsiziga (web/index.html dagi GG_LADDER bilan bir xil)
var ggLadder = []string{"sniper", "lmg", "rifle", "carbine", "shotgun", "smg", "deagle", "mpistol", "pistol", "knife"}

func validMode(m string) bool {
	return m == modeComp || m == modeDM || m == modeTDM || m == modeGG || m == modeWM || m == modeRT || m == modeAim || m == modeZM
}

// setMode: tashqi rejim nomi (tdm ham) → Md va Tm.
func (g *gameState) setMode(m string) {
	g.Md, g.Tm = m, false
	if m == modeTDM {
		g.Md, g.Tm = modeDM, true
	}
}

// modeID: rejimning tashqi nomi (ro'yxat va admin uchun).
func (g *gameState) modeID() string {
	if g.Md == modeDM && g.Tm {
		return modeTDM
	}
	return g.Md
}

func ggLevel(kills int) int     { return min(len(ggLadder)-1, max(0, kills)/ggPerLevel) }
func ggWeapon(kills int) string { return ggLadder[ggLevel(kills)] }
func ggWinKills() int           { return (len(ggLadder)-1)*ggPerLevel + 1 }

func (g *gameState) ffa() bool      { return g.Md == modeDM && !g.Tm }
func (g *gameState) respawns() bool { return g.Md == modeDM || g.Md == modeGG || g.Md == modeZM }

// foes: a va b bir-biriga raqibmi (deathmatch'da — har kim).
func (r *room) foes(a, b *client) bool {
	if a == nil || b == nil || a == b || a.info.Sq == "" || b.info.Sq == "" {
		return false
	}
	if r.gs.Md == modeZM { // zombilar odamlarga qarshi (jamoalar emas)
		return r.gs.Zm[a.id] != r.gs.Zm[b.id]
	}
	return r.gs.ffa() || a.info.Sq != b.info.Sq
}

// spawnOKLocked: yangi hayot boshlanadigan joy (deathmatch'da — istalgan ochiq katak).
func (r *room) spawnOKLocked(p vec3, side string) bool {
	if r.gs.Md == modeRT && side == "T" && (r.gs.Ph == "freeze" || r.gs.Ph == "live") {
		return r.retakeSpawnOK(p) && !r.gm().botSolid(wc(p.X), wc(p.Z))
	}
	if (r.gs.Md == modeDM || r.gs.Md == modeZM) && (r.gs.Ph == "live" || r.gs.Ph == "post") {
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
	if g.Tm { // jamoaviy deathmatch: jamoa hisobi
		if killer.info.Sq == "B" {
			g.SB++
		} else {
			g.SA++
		}
	}
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
	if g.Tm { // jamoaviy deathmatch: ko'proq o'ldirgan jamoa ("" — durang)
		g.Ph, g.T, g.B = "over", 10, nil
		r.finalStatsLocked()
		g.Wn, g.Wnm = "", ""
		if g.SA != g.SB {
			g.Wn = "A"
			if g.SB > g.SA {
				g.Wn = "B"
			}
		}
		return
	}
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
	if g.Md == modeZM {
		r.tickZombieLocked(now, dt)
		return
	}
	ready := nA+nB >= 2
	if g.Md == modeGG || g.Tm {
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
			r.applyVoteLocked(now)
		}
	default: // boshqa rejimdan qolgan bosqich
		g.reset()
	}
}

// changeModeLocked: xona boshqa rejimga o'tadi — o'yin isinishdan boshlanadi.
func (r *room) changeModeLocked(m string, now time.Time) {
	r.gs.M, _ = pairMapMode(r.gs.M, m, true)
	r.gs.setMode(m)
	if m == modeWM && (r.max == 0 || r.max > 4) {
		r.max = 4 // Wingman: 2 ga 2
	}
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

// ---- Wingman (2v2) va Retake ----
//
//   wm — Wingman: ikki kishilik jamoalar, bitta bomba nuqtasi (A), 9 g'alabagacha (MR8),
//        raund 1,5 daqiqa, iqtisod raqobatdagidek.
//   rt — Retake: har raund boshida bomba A yoki B da allaqachon o'rnatilgan; hujumchilar uni
//        nuqta yonida himoya qiladi, himoyachilar bazadan kelib zararsizlantiradi. Xarid yo'q —
//        qurollar har raundda beriladi. 7 g'alabagacha (har 6 raundda tomonlar almashadi).

const (
	modeWM = "wm"
	modeRT = "rt"
)

func (g *gameState) roundTime() float64 {
	switch g.Md {
	case modeWM:
		return 90
	case modeRT:
		return 40
	case modeAim:
		return 60
	}
	return 115
}

// freezeTime: raund oldidan kutish (aim'da qisqa — xarid yo'q, faqat qurol tanlash).
func (g *gameState) freezeTime(first, newHalf bool) float64 {
	switch {
	case g.Md == modeAim:
		return 3
	case first:
		return 6
	case newHalf:
		return 10
	}
	return 5
}

// aimTimeoutWinner: aim raundi vaqti tugadi — tirik qolganlar ko'p jamoa, keyin joni ko'p jamoa, teng bo'lsa himoya.
func (r *room) aimTimeoutWinner() string {
	n, hp := map[string]int{}, map[string]float64{}
	for _, c := range r.clients {
		if c.ps.alive && c.info.In == r.gs.R && (c.info.Tm == "T" || c.info.Tm == "CT") {
			n[c.info.Tm]++
			hp[c.info.Tm] += c.ps.hp
		}
	}
	switch {
	case n["T"] != n["CT"]:
		if n["T"] > n["CT"] {
			return "T"
		}
		return "CT"
	case hp["T"] > hp["CT"]:
		return "T"
	}
	return "CT"
}

// fixedFm: rejimning o'z formati (0 — o'yinchilar sozlamasi).
func (g *gameState) fixedFm() int {
	switch g.Md {
	case modeWM:
		return 8
	case modeRT:
		return 6
	case modeAim:
		return 8
	}
	return 0
}

// retakeRoundLocked: yangi raund — bomba tasodifiy nuqtada allaqachon o'rnatilgan.
func (r *room) retakeRoundLocked() {
	g, gm := r.gs, r.gm()
	site := []string{"A", "B"}[rand.IntN(2)]
	spots := gm.spots[site]
	p := cellCenter(spots[rand.IntN(len(spots))])
	g.Rs = site
	g.B = &bombState{X: math.Round(p.X*100) / 100, Z: math.Round(p.Z*100) / 100, S: site, T: 40}
}

// retakeSpawnOK: retake'da hujumchilar bomba yonida tug'iladi.
func (r *room) retakeSpawnOK(p vec3) bool {
	g := r.gs
	if g.B == nil {
		return false
	}
	return p.flat(vec3{g.B.X, 0, g.B.Z}) < 16 || r.gm().siteAt(p.X, p.Z) == g.Rs
}

// ---- keyingi xarita uchun ovoz berish (o'yin oxirida) ----

func (r *room) voteLocked(c *client, raw []byte) {
	g := r.gs
	if g.Ph != "over" || c.bot != nil || c.info.Sq == "" {
		return
	}
	var v struct {
		M string `json:"m"`
	}
	if json.Unmarshal(raw, &v) != nil || !validMap(v.M) || isAimMap(v.M) != isAimMap(g.M) { // ovoz faqat shu turdagi xaritalarga
		return
	}
	c.vote = v.M
	g.Vt = map[string]int{}
	for _, o := range r.clients {
		if o.vote != "" {
			g.Vt[o.vote]++
		}
	}
}

// applyVoteLocked: ovozlar bo'yicha keyingi xarita (teng bo'lsa — tasodifan). O'yin isinishdan boshlanadi.
func (r *room) applyVoteLocked(now time.Time) {
	g := r.gs
	best, bn := []string{}, 0
	for _, id := range mapOrder() {
		n := g.Vt[id]
		if n > bn {
			best, bn = []string{id}, n
		} else if n == bn && n > 0 {
			best = append(best, id)
		}
	}
	for _, c := range r.clients {
		c.vote = ""
	}
	if len(best) > 0 {
		if m := best[rand.IntN(len(best))]; m != g.M {
			r.changeMapLocked(m, now)
			return
		}
	}
	g.reset()
}

func mapOrder() []string {
	out := make([]string, 0, len(mapDefs))
	for _, d := range mapDefs {
		out = append(out, d.id)
	}
	return out
}
