package main

// Onlayn xonadagi botlar. Bot — serverning o'zidagi "virtual o'yinchi": u xonadagi boshqa
// o'yinchilar kabi holat (presence) yuboradi, shuning uchun brauzerlar uni oddiy raqib yoki
// jamoadosh sifatida ko'radi, unga otadi, o'ldiradi. Botning o'qlari, joni, bomba o'rnatishi
// va zararsizlantirishi to'g'ridan-to'g'ri server hakami orqali o'tadi.

import (
	"container/heap"
	"encoding/json"
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

const maxBots = 10

var botNames = []string{"Sardor", "Jasur", "Diyor", "Bekzod", "Otabek", "Sherzod", "Anvar", "Rustam", "Temur", "Nodir", "Farrux", "Akmal"}

type botLevel struct {
	react, turn, acc, head float64
}

var botLevels = map[string]botLevel{
	"easy":   {react: .75, turn: 4, acc: .28, head: .07},
	"normal": {react: .45, turn: 7, acc: .42, head: .14},
	"hard":   {react: .25, turn: 11, acc: .58, head: .24},
}

var weaponFall = map[string]float64{"pistol": .85, "deagle": .81, "smg": .85, "carbine": .97, "rifle": .98, "sniper": .99, "knife": 1,
	"mpistol": .8, "shotgun": .55, "lmg": .97, "scout": .99}
var weaponMag = map[string]int{"pistol": 20, "deagle": 7, "smg": 30, "carbine": 25, "rifle": 30, "sniper": 5,
	"mpistol": 18, "shotgun": 7, "lmg": 100, "scout": 10}
var weaponAcc = map[string]float64{"pistol": .85, "deagle": .85, "smg": .8, "carbine": 1, "rifle": 1, "sniper": 1.6,
	"mpistol": .8, "shotgun": 1.2, "lmg": .9, "scout": 1.4}

func cellCenter(c [2]int) vec3 {
	return vec3{(float64(c[0]) + .5) * cellSize, 0, (float64(c[1]) + .5) * cellSize}
}

type botBrain struct {
	name, lv  string
	weapon    string
	mag       int
	reloadAt  time.Time
	yaw       float64
	cr        float64
	path      []vec3
	pi        int
	pathGoal  vec3
	planAt    time.Time
	mode      string
	goal      vec3
	look      vec3
	target    string
	reactAt   time.Time
	fireAt    time.Time
	burst     int
	seenSince time.Time
	lastKnown vec3
	knownAt   time.Time
	alertAt   time.Time
	actT      float64 // bomba o'rnatish / zararsizlantirish vaqti
	execAt    time.Time
	site      string
	shotN     int
	sh        map[string]any
	hiSeq     int
	hits      []hitClaim
	drops     []map[string]any // o'lganda tashlangan qurollar ("dr" hodisalari)
	dropN     int
	deN       int
	de        *deathClaim
	kills     int
	deaths    int
	life      int
	deadAt    time.Time
	moving    bool
}

func (r *room) botCountLocked() (bots, humans int) {
	for _, c := range r.clients {
		if c.bot != nil {
			bots++
		} else {
			humans++
		}
	}
	return
}

// addBotLocked: jamoaga (A/B) yangi bot qo'shadi.
func (r *room) addBotLocked(sq, lv string, now time.Time) *client {
	if sq != "A" && sq != "B" {
		return nil
	}
	if _, ok := botLevels[lv]; !ok {
		lv = "normal"
	}
	used := map[string]bool{}
	for _, c := range r.clients {
		if c.bot != nil {
			used[c.bot.name] = true
		}
	}
	name := "Bot"
	for _, n := range botNames {
		if !used["BOT "+n] {
			name = "BOT " + n
			break
		}
	}
	c := &client{id: "b" + newID(), room: r, ps: newPState(), joined: now, bot: &botBrain{name: name, lv: lv, life: -9, deadAt: now.Add(-time.Hour)}}
	c.info = presInfo{Sq: sq, Nm: name, Hc: 1}
	c.info.Tm = r.botSideLocked(c)
	r.clients[c.id] = c
	r.publishBotLocked(c, now)
	return c
}

func (r *room) removeClientLocked(c *client) {
	delete(r.clients, c.id)
	r.broadcast(jsonMsg(map[string]string{"t": "l", "id": c.id}), "")
}

func (r *room) botSideLocked(c *client) string {
	g := r.gs
	if g.R <= 0 || g.Ph == "warmup" {
		if c.info.Sq == "B" {
			return "CT"
		}
		return "T"
	}
	return sideOfSquad(c.info.Sq, g.R, g.Fm)
}

func roundInHalf(r, h int) int {
	if r <= 2*h {
		return (r-1)%h + 1
	}
	return (r-2*h-1)%otHalf + 1
}

// pickAimWeapon: aim xaritasida bot quroli (hammasi bepul).
func pickAimWeapon() string {
	switch x := rand.Float64(); {
	case x < .35:
		return "rifle"
	case x < .55:
		return "carbine"
	case x < .7:
		return "sniper"
	case x < .8:
		return "scout"
	case x < .9:
		return "deagle"
	}
	return "smg"
}

func pickBotWeapon(rih int) string {
	switch {
	case rih <= 1:
		switch x := rand.Float64(); {
		case x < .3:
			return "deagle"
		case x < .45:
			return "mpistol"
		}
		return "pistol"
	case rih == 2:
		if rand.Float64() < .3 {
			return "shotgun"
		}
		return "smg"
	}
	x := rand.Float64()
	switch {
	case x < .1:
		return "sniper"
	case x < .15:
		return "scout"
	case x < .55:
		return "rifle"
	case x < .75:
		return "carbine"
	case x < .82:
		return "shotgun"
	case x < .9:
		return "lmg"
	}
	return "smg"
}

func (m *gameMap) randomCellIn(t uint8) vec3 {
	var cells [][2]int
	for z := 0; z < gridH; z++ {
		for x := 0; x < gridW; x++ {
			if m.grid[z*gridW+x] == t {
				cells = append(cells, [2]int{x, z})
			}
		}
	}
	c := cellCenter(cells[rand.IntN(len(cells))])
	return vec3{c.X + (rand.Float64()-.5)*.8, 0, c.Z + (rand.Float64()-.5)*.8}
}

func (r *room) spawnBotLocked(c *client, now time.Time) {
	b, p, g := c.bot, c.ps, r.gs
	c.info.Tm = r.botSideLocked(c)
	zone := uint8(cTSpawn)
	if c.info.Tm == "CT" {
		zone = cCTSpawn
	}
	pos := r.gm().randomCellIn(zone)
	if g.Md == modeRT && c.info.Tm == "T" && g.Rs != "" && g.Ph != "warmup" { // retake: bomba yonida
		spots := r.gm().spots[g.Rs]
		pos = cellCenter(spots[rand.IntN(len(spots))])
		pos.X += (rand.Float64() - .5) * 1.2
		pos.Z += (rand.Float64() - .5) * 1.2
	}
	modeLive := g.respawns() && g.Ph == "live"
	if modeLive && g.Md == modeDM {
		pos = r.dmSpawnLocked(c)
	}
	rih := 1
	if g.Ph == "warmup" || g.R <= 0 {
		b.life, c.info.In, rih = -1, 0, 3
	} else {
		b.life, c.info.In = g.R, g.R
		rih = roundInHalf(g.R, g.Fm)
		if g.R > 2*g.Fm || g.respawns() {
			rih = 3
		}
	}
	b.weapon = pickBotWeapon(rih)
	if g.Md == modeAim {
		b.weapon, rih = pickAimWeapon(), 3
	}
	if modeLive && g.Md == modeGG {
		b.weapon = ggWeapon(g.Sc[c.id])
	}
	b.mag = weaponMag[b.weapon]
	b.reloadAt, b.target, b.path, b.actT, b.cr, b.burst = time.Time{}, "", nil, 0, 0, 0
	b.knownAt, b.alertAt = time.Time{}, time.Time{}
	p.alive, p.hp, p.pos, p.posOK, p.moveAt = true, 100, pos, true, now
	p.armor, p.helmet = 0, false
	if rih > 1 {
		p.armor, p.helmet = 100, rih > 2
	}
	p.dueAt, p.spawnAt = time.Time{}, now
	p.push(sample{t: now, p: pos})
	if modeLive || g.Md == modeAim {
		b.yaw = rand.Float64() * 2 * math.Pi
		b.mode, b.goal, b.path = "roam", r.gm().roamPoint(), nil
	} else if c.info.Tm == "T" {
		b.yaw = 0
		b.site = []string{"A", "B"}[rand.IntN(2)]
		if g.Md == modeWM {
			b.site = "A"
		}
		spots := r.gm().spots[b.site]
		b.goal = cellCenter(spots[rand.IntN(len(spots))])
		looks := r.gm().looks[b.site]
		b.look = cellCenter(looks[rand.IntN(len(looks))])
		b.mode = "wait"
		b.execAt = now.Add(time.Duration(6+rand.IntN(14)) * time.Second)
	} else {
		b.yaw = math.Pi
		gm := r.gm()
		var hs [][2][2]int
		for i, h := range gm.holds {
			if g.Md != modeWM || gm.holdSite[i] != "B" { // Wingman: faqat A va o'rta
				hs = append(hs, h)
			}
		}
		h := hs[rand.IntN(len(hs))]
		b.goal, b.look = cellCenter(h[0]), cellCenter(h[1])
		b.goal.X += (rand.Float64() - .5) * 1.2
		b.goal.Z += (rand.Float64() - .5) * 1.2
		b.mode = "hold"
	}
}

// botDieLocked: bot halok bo'ldi (killer nil bo'lishi mumkin).
func (r *room) botDieLocked(v, killer *client, w string, hs bool, now time.Time) {
	b := v.bot
	if !v.ps.alive {
		return
	}
	v.ps.alive, v.ps.dueAt, v.ps.deadAt = false, time.Time{}, now
	b.deadAt, b.deaths, b.target, b.actT = now, b.deaths+1, "", 0
	b.deN++
	by := ""
	if killer != nil {
		by = killer.id
		if killer.bot != nil && killer.info.Sq != v.info.Sq {
			killer.bot.kills++
		}
	}
	b.de = &deathClaim{N: b.deN, By: by, W: w, HS: map[bool]int{true: 1}[hs]}
	if !r.gs.respawns() && weaponMag[b.weapon] > 0 {
		r.botDropLocked(v)
	}
	r.tr.death(r, v, killer, hs)
	r.noteDeathLocked(v, killer, hs, w)
	if killer != nil {
		r.creditKillLocked(v, killer, now)
	}
}

// botDropLocked: o'lgan botning quroli yerda qoladi — odamlar uni olishi mumkin (brauzer odamnikidek
// "dr" hodisasini ko'radi, server esa uni yerdagi qurollar ro'yxatiga yozadi).
func (r *room) botDropLocked(c *client) {
	b, p, g := c.bot, c.ps, r.gs
	tag := g.R
	if g.Ph == "warmup" {
		tag = 0
	}
	b.dropN++
	id := c.id + ":" + strconv.Itoa(b.dropN)
	if r.ecoOn() {
		r.groundLocked()
		r.ground[id] = groundGun{w: b.weapon, r: g.R}
	}
	a := rand.Float64() * 2 * math.Pi
	r2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	b.drops = append(b.drops, map[string]any{"n": b.dropN, "id": id, "w": b.weapon, "mg": max(0, b.mag), "rs": weaponMag[b.weapon] * 3, "r": tag,
		"x": r2(p.pos.X), "y": r2(p.pos.Y + .9), "z": r2(p.pos.Z), "vx": r2(math.Cos(a) * 1.2), "vy": 1.6, "vz": r2(math.Sin(a) * 1.2)})
	if len(b.drops) > 6 {
		b.drops = b.drops[1:]
	}
}

// ---- yo'l topish (A*) ----

func (m *gameMap) botSolid(cx, cz int) bool {
	t := m.cellType(cx, cz)
	return t == cWall || t == cCrate || t == cStack
}

type pqItem struct {
	cell int
	f    float64
	idx  int
}
type pqueue []*pqItem

func (q pqueue) Len() int           { return len(q) }
func (q pqueue) Less(i, j int) bool { return q[i].f < q[j].f }
func (q pqueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].idx = i; q[j].idx = j }
func (q *pqueue) Push(x any)        { it := x.(*pqItem); it.idx = len(*q); *q = append(*q, it) }
func (q *pqueue) Pop() any          { old := *q; it := old[len(old)-1]; *q = old[:len(old)-1]; return it }
func cellOf(p vec3) (int, int)      { return wc(p.X), wc(p.Z) }
func cellIdx(x, z int) int          { return z*gridW + x }
func idxCell(i int) (int, int)      { return i % gridW, i / gridW }
func octile(ax, az, bx, bz int) float64 {
	dx, dz := math.Abs(float64(ax-bx)), math.Abs(float64(az-bz))
	return math.Max(dx, dz) + (math.Sqrt2-1)*math.Min(dx, dz)
}

func (m *gameMap) astar(from, to vec3) []vec3 {
	sx, sz := cellOf(from)
	gx, gz := cellOf(to)
	if m.botSolid(gx, gz) {
		return nil
	}
	const n = gridW * gridH
	var gs [n]float64
	var came [n]int
	var closed [n]bool
	for i := range gs {
		gs[i], came[i] = math.Inf(1), -1
	}
	start, goal := cellIdx(sx, sz), cellIdx(gx, gz)
	gs[start] = 0
	open := &pqueue{}
	heap.Push(open, &pqItem{cell: start, f: octile(sx, sz, gx, gz)})
	for open.Len() > 0 {
		cur := heap.Pop(open).(*pqItem).cell
		if cur == goal {
			var cells []int
			for c := goal; c != -1; c = came[c] {
				cells = append([]int{c}, cells...)
			}
			out := make([]vec3, 0, len(cells))
			for _, c := range cells[1:] {
				x, z := idxCell(c)
				out = append(out, cellCenter([2]int{x, z}))
			}
			out = append(out, to)
			return out
		}
		if closed[cur] {
			continue
		}
		closed[cur] = true
		cx, cz := idxCell(cur)
		for dz := -1; dz <= 1; dz++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dz == 0 {
					continue
				}
				nx, nz := cx+dx, cz+dz
				if nx < 0 || nz < 0 || nx >= gridW || nz >= gridH || m.botSolid(nx, nz) {
					continue
				}
				if dx != 0 && dz != 0 && (m.botSolid(cx+dx, cz) || m.botSolid(cx, cz+dz)) {
					continue // burchakni kesib o'tmaslik
				}
				step := 1.0
				if dx != 0 && dz != 0 {
					step = math.Sqrt2
				}
				ni := cellIdx(nx, nz)
				if ng := gs[cur] + step; ng < gs[ni] {
					gs[ni], came[ni] = ng, cur
					heap.Push(open, &pqItem{cell: ni, f: ng + octile(nx, nz, gx, gz)})
				}
			}
		}
	}
	return nil
}

// ---- bir qadam ----

func wrapAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// yawTo: o'yindagi burilish burchagi (oldinga = (-sin, -cos)).
func yawTo(from, to vec3) float64 { return math.Atan2(-(to.X - from.X), -(to.Z - from.Z)) }

func (b *botBrain) turn(target, rate, dt float64) float64 {
	d := wrapAngle(target - b.yaw)
	step := math.Max(-rate*dt, math.Min(rate*dt, d))
	b.yaw = wrapAngle(b.yaw + step)
	return math.Abs(d - step)
}

func (r *room) botGo(c *client, goal vec3, speed, dt float64, now time.Time) bool {
	b, p := c.bot, c.ps
	if b.path == nil || now.After(b.planAt) || b.pathGoal.flat(goal) > 1.2 {
		b.path, b.pi, b.pathGoal, b.planAt = r.gm().astar(p.pos, goal), 0, goal, now.Add(3*time.Second)
		if b.path == nil {
			return false
		}
	}
	for b.pi < len(b.path) && b.path[b.pi].flat(p.pos) < .3 {
		b.pi++
	}
	if b.pi >= len(b.path) {
		return true
	}
	wp := b.path[b.pi]
	d := wp.flat(p.pos)
	st := math.Min(d, speed*dt)
	p.pos.X += (wp.X - p.pos.X) / d * st
	p.pos.Z += (wp.Z - p.pos.Z) / d * st
	b.turn(yawTo(p.pos, wp), 8, dt)
	b.moving = true
	return false
}

// botSees: eng yaqin ko'rinib turgan raqib.
func (r *room) botSees(c *client, now time.Time) (*client, float64) {
	b, p := c.bot, c.ps
	eyeP := vec3{p.pos.X, p.pos.Y + 1.6 - b.cr*.45, p.pos.Z}
	alert := now.Sub(b.alertAt) < 4*time.Second
	var best *client
	bd := 80.0
	for _, o := range r.clients {
		if o == c || !o.ps.alive || !o.ps.posOK || o.info.Tm == "" || !r.foes(c, o) {
			continue
		}
		d := eyeP.sub(o.ps.pos).len()
		if d > bd {
			continue
		}
		ang := math.Abs(wrapAngle(yawTo(p.pos, o.ps.pos) - b.yaw))
		if ang > 1.3 && !alert && o.id != b.target {
			continue
		}
		head := vec3{o.ps.pos.X, o.ps.pos.Y + 1.5, o.ps.pos.Z}
		chest := vec3{o.ps.pos.X, o.ps.pos.Y + 1.05, o.ps.pos.Z}
		if r.gm().lineOfSight(eyeP, chest) || r.gm().lineOfSight(eyeP, head) {
			best, bd = o, d
		}
	}
	return best, bd
}

func (r *room) botFire(c *client, t *client, dist float64, now time.Time) {
	b, p := c.bot, c.ps
	spec := weaponSpecs[b.weapon]
	lv := botLevels[b.lv]
	if b.weapon == "knife" { // pichoq: yaqin kelib uradi
		b.fireAt = now.Add(time.Duration(700+rand.IntN(300)) * time.Millisecond)
		b.shotN++
		b.sh = map[string]any{"n": b.shotN, "w": "knife", "sx": 0, "sy": 0, "sz": 0, "ex": 0, "ey": 0, "ez": 0}
		if dist > 2 || rand.Float64() > .8 {
			return
		}
		b.hiSeq++
		h := hitClaim{I: b.hiSeq, T: t.id, D: 55, Pn: spec.pen, P: "c", W: "knife"}
		if t.bot == nil {
			b.hits = append(b.hits, h)
			if len(b.hits) > 8 {
				b.hits = b.hits[1:]
			}
		}
		r.applyHitLocked(c, h, now)
		return
	}
	b.mag--
	if b.mag <= 0 {
		b.reloadAt = now.Add(2600 * time.Millisecond)
		b.mag = weaponMag[b.weapon]
	}
	b.fireAt = now.Add(time.Duration(spec.rate*(1.25+rand.Float64()*.35)*1000) * time.Millisecond)
	b.burst++
	if (b.weapon == "rifle" || b.weapon == "carbine" || b.weapon == "smg" || b.weapon == "lmg" || b.weapon == "mpistol") && b.burst >= 3+rand.IntN(4) {
		b.burst = 0
		b.fireAt = b.fireAt.Add(time.Duration(250+rand.IntN(400)) * time.Millisecond)
	}
	acc := lv.acc * weaponAcc[b.weapon] * math.Max(.25, math.Min(1.1, 1.15-dist/55))
	if n := min(t.ps.hn, len(t.ps.hist)); n >= 2 {
		a, z := t.ps.hist[(t.ps.hn-1)%len(t.ps.hist)], t.ps.hist[(t.ps.hn-2)%len(t.ps.hist)]
		if dt := a.t.Sub(z.t).Seconds(); dt > 0 && a.p.flat(z.p)/dt > 2.5 {
			acc *= .7
		}
	}
	if now.Sub(b.seenSince) < 500*time.Millisecond {
		acc *= .8
	}
	if b.weapon == "sniper" && now.Sub(b.seenSince) > 900*time.Millisecond {
		acc = math.Min(.95, acc*1.3)
	}
	muzzle := vec3{p.pos.X - math.Sin(b.yaw)*.7, p.pos.Y + 1.35 - b.cr*.4, p.pos.Z - math.Cos(b.yaw)*.7}
	hit := rand.Float64() < acc
	part := "c"
	end := vec3{t.ps.pos.X, t.ps.pos.Y + 1.05, t.ps.pos.Z}
	if hit {
		x := rand.Float64()
		switch {
		case x < lv.head:
			part, end.Y = "h", t.ps.pos.Y+1.55
		case x < lv.head+.45:
			part = "c"
		case x < lv.head+.7:
			part, end.Y = "s", t.ps.pos.Y+.95
		default:
			part, end.Y = "l", t.ps.pos.Y+.5
		}
	} else {
		end = end.add(vec3{(rand.Float64() - .5) * 2.4, (rand.Float64() - .3) * 1.4, (rand.Float64() - .5) * 2.4})
	}
	b.shotN++
	r2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	b.sh = map[string]any{"n": b.shotN, "w": b.weapon, "sx": r2(muzzle.X), "sy": r2(muzzle.Y), "sz": r2(muzzle.Z), "ex": r2(end.X), "ey": r2(end.Y), "ez": r2(end.Z)}
	if !hit {
		return
	}
	dmg := spec.dmg * partMult[part] * math.Pow(weaponFall[b.weapon], dist/rangeUnit)
	pc := 0
	if spec.pellets > 1 { // drobovik: masofa oshgan sari kamroq saochma yetib boradi
		pc = max(1, int(math.Round(float64(spec.pellets)*math.Max(.1, math.Min(1, 1.3-dist/14)))))
		dmg *= float64(pc)
	}
	b.hiSeq++
	h := hitClaim{I: b.hiSeq, T: t.id, D: math.Round(dmg), Pn: spec.pen, P: part, W: b.weapon, Pc: pc}
	if t.bot == nil { // odam: uning brauzeri zararni o'zi qo'llaydi, server ham hisoblaydi
		b.hits = append(b.hits, h)
		if len(b.hits) > 8 {
			b.hits = b.hits[1:]
		}
	}
	r.applyHitLocked(c, h, now)
	if t.bot != nil {
		t.bot.alertAt = now
	}
}

// botStepLocked: botning bir qadami (soniyasiga 20 marta).
func (r *room) botStepLocked(c *client, now time.Time, dt float64) {
	b, p, g := c.bot, c.ps, r.gs
	b.moving = false
	// tirilish
	switch {
	case g.Ph == "warmup":
		if !p.alive && now.Sub(b.deadAt) > 3*time.Second {
			r.spawnBotLocked(c, now)
		} else if p.alive && b.life != -1 {
			r.spawnBotLocked(c, now)
		}
	case g.Ph == "freeze" && g.R > 0 && b.life != g.R:
		r.spawnBotLocked(c, now)
	case g.respawns() && g.Ph == "live" && (b.life != g.R || (!p.alive && now.Sub(b.deadAt) > 2*time.Second)):
		r.spawnBotLocked(c, now)
	}
	if !p.alive || g.Ph == "freeze" || g.Ph == "over" {
		r.publishBotLocked(c, now)
		return
	}
	lv := botLevels[b.lv]
	t, dist := r.botSees(c, now)
	if t != nil {
		if b.target != t.id {
			b.target, b.seenSince = t.id, now
			b.reactAt = now.Add(time.Duration((lv.react*(.8+rand.Float64()*.5)+dist/150)*1000) * time.Millisecond)
		}
		b.lastKnown, b.knownAt = t.ps.pos, now
		b.actT = 0
		off := b.turn(yawTo(p.pos, t.ps.pos), lv.turn, dt)
		if b.weapon == "knife" && dist > 1.6 {
			r.botGo(c, t.ps.pos, 5, dt, now)
			b.turn(yawTo(p.pos, t.ps.pos), lv.turn, dt)
		} else if now.After(b.reactAt) && now.After(b.fireAt) && now.After(b.reloadAt) && off < .15 {
			r.botFire(c, t, dist, now)
		}
	} else {
		b.target = ""
		r.botObjectiveLocked(c, now, dt)
	}
	b.cr += ((map[bool]float64{true: 1}[b.actT > 0]) - b.cr) * math.Min(1, dt*8)
	if b.moving {
		p.moveAt = now
	}
	p.push(sample{t: now, p: p.pos, cr: b.cr})
	r.publishBotLocked(c, now)
}

func (r *room) botObjectiveLocked(c *client, now time.Time, dt float64) {
	b, p, g := c.bot, c.ps, r.gs
	if (g.respawns() || g.Md == modeAim) && g.Ph == "live" { // aim'da ham — raqibni qidirib yuradi
		r.botRoamLocked(c, now, dt)
		return
	}
	// yaqinda raqibni ko'rgan bo'lsa — o'sha joyga boradi
	chase := !b.knownAt.IsZero() && now.Sub(b.knownAt) < 5*time.Second
	if g.B != nil {
		bomb := vec3{g.B.X, 0, g.B.Z}
		if c.info.Tm == "CT" {
			if p.pos.flat(bomb) > 1.1 {
				b.actT = 0
				r.botGo(c, bomb, 4.4, dt, now)
				return
			}
			b.actT += dt
			b.turn(yawTo(p.pos, bomb), 6, dt)
			if b.actT >= 7 && g.Ph == "live" {
				b.actT = 0
				r.defuser = c.id
				r.endRoundLocked("CT", "defuse")
			}
			return
		}
		// hujumchi: bomba atrofida qo'riqlaydi
		if b.mode != "guard" {
			b.mode = "guard"
			a := rand.Float64() * 2 * math.Pi
			b.goal = vec3{bomb.X + math.Sin(a)*4, 0, bomb.Z + math.Cos(a)*4}
			if r.gm().botSolid(cellOf(b.goal)) {
				b.goal = bomb
			}
		}
		if p.pos.flat(b.goal) > .7 {
			r.botGo(c, b.goal, 3.8, dt, now)
		} else {
			b.turn(yawTo(p.pos, b.look)+math.Sin(float64(now.UnixMilli())/1500)*.4, 3, dt)
		}
		return
	}
	if chase && b.mode != "plant" {
		if r.botGo(c, b.lastKnown, 3.6, dt, now) || p.pos.flat(b.lastKnown) < 1.2 {
			b.knownAt = time.Time{}
		}
		return
	}
	if c.info.Tm == "T" && g.Ph != "warmup" {
		if b.mode == "wait" {
			if now.After(b.execAt) || (g.Ph == "live" && g.T < 50) {
				b.mode = "go"
			} else {
				b.turn(b.yaw+.4, .6, dt)
				return
			}
		}
		if p.pos.flat(b.goal) > .8 {
			b.actT = 0
			r.botGo(c, b.goal, 4.2, dt, now)
			return
		}
		if r.gm().siteAt(p.pos.X, p.pos.Z) != "" && g.Ph == "live" {
			b.mode = "plant"
			b.actT += dt
			if b.actT >= 3.2 && g.B == nil {
				g.B = &bombState{X: math.Round(p.pos.X*100) / 100, Z: math.Round(p.pos.Z*100) / 100, S: r.gm().siteAt(p.pos.X, p.pos.Z), T: 40}
				r.planter = c.id
				b.actT = 0
				b.mode = "guard"
			}
		}
		return
	}
	// himoyachi yoki isinish: o'z joyida turib qaraydi
	if p.pos.flat(b.goal) > .7 {
		r.botGo(c, b.goal, 3.8, dt, now)
	} else {
		look := b.look
		if now.Sub(b.alertAt) < 4*time.Second && !b.lastKnown.isZero() {
			look = b.lastKnown
		}
		b.turn(yawTo(p.pos, look)+math.Sin(float64(now.UnixMilli())/1800+float64(len(b.name)))*.3, 3, dt)
	}
}

func (v vec3) isZero() bool { return v.X == 0 && v.Y == 0 && v.Z == 0 }

// publishBotLocked: botning holatini hammaga yuboradi (odam o'yinchilarniki bilan bir xil shaklda).
func (r *room) publishBotLocked(c *client, now time.Time) {
	b, p := c.bot, c.ps
	r2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	al := 0
	if p.alive {
		al = 1
	}
	mv := 0
	if b.moving {
		mv = 1
	}
	f := map[string]any{"v": 1, "hc": 1, "bot": 1, "lv": b.lv, "nm": b.name, "sq": c.info.Sq, "tm": c.info.Tm,
		"x": r2(p.pos.X), "y": r2(p.pos.Y), "z": r2(p.pos.Z), "yw": r2(b.yaw), "pt": 0, "cr": r2(b.cr),
		"al": al, "hp": math.Max(0, math.Round(p.hp)), "w": b.weapon, "mv": mv, "k": b.kills, "d": b.deaths, "in": c.info.In,
		"sh": b.sh, "de": b.de, "ar": math.Round(p.armor)}
	if len(b.hits) > 0 {
		f["hi"] = b.hits
	}
	if len(b.drops) > 0 {
		f["dr"] = b.drops
	}
	out, err := json.Marshal(f)
	if err != nil {
		return
	}
	c.pres = out
	c.info.Nm = b.name
	r.tr.presence(c, c.info)
	r.broadcast(append(append([]byte(`{"t":"u","id":"`+c.id+`","p":`), out...), '}'), "")
}
