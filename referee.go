package main

// Server hakamligi (3-bosqich).
//
// Avval raundni o'yinchilardan biri (xost) yuritardi, o'q tekkanini esa otgan o'yinchining
// o'zi e'lon qilardi. Endi server:
//   - raundni o'zi yuritadi (vaqt, bomba, hisob, yarim vaqt, overtime) va holatni
//     {"t":"g"} xabari bilan soniyasiga 10 marta yuboradi;
//   - har bir o'q tegishini tekshiradi: otgan va tekkan tirikmi, raqibmi, zarar qurolga
//     mosmi, o'q tezligi qurolnikidan oshmaydimi, orada devor yo'qmi (kechikishni hisobga
//     olib, oxirgi ~0,5 soniyadagi holatlar bo'yicha). Noto'g'ri tegishlar boshqalarga
//     yuborilmaydi;
//   - har bir o'yinchining jonini o'zi hisoblaydi: joni tugagan, lekin o'lmagan o'yinchiga
//     {"t":"kill"} yuboriladi, bo'ysunmasa boshqalarga o'lik ko'rsatiladi;
//   - harakatni tekshiradi: juda tez siljish yoki devor ichiga kirish qabul qilinmaydi,
//     o'yinchi oxirgi to'g'ri joyiga qaytariladi ({"t":"fix"});
//   - bomba faqat A/B nuqtada, 3 soniya qimirlamay turilganda o'rnatiladi, zararsizlantirish
//     ham bomba yonida yetarli vaqt turilgandagina qabul qilinadi;
//   - o'ldirish reytingga faqat server tasdiqlagan tegishdan keyin yoziladi.
//
// Granata va bomba portlashi zararini ham server hisoblaydi (nadesim.go), pul, xaridlar va
// qo'ldagi qurollar ham serverda (economy.go).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"
)

// ---- qurollar (web/index.html dagi W jadvali bilan bir xil) ----

type wspec struct {
	dmg, pen, rate float64
	pellets        int // drobovik: bitta o'qda nechta saochma (tegishi bitta da'vo bo'lib keladi)
	melee, nade    bool
}

var weaponSpecs = map[string]wspec{
	"knife":   {dmg: 180, pen: .85, rate: .45, melee: true},
	"pistol":  {dmg: 30, pen: .47, rate: .15},
	"deagle":  {dmg: 56, pen: .93, rate: .27},
	"smg":     {dmg: 26, pen: .6, rate: .075},
	"carbine": {dmg: 31, pen: .75, rate: .09},
	"rifle":   {dmg: 36, pen: .775, rate: .1},
	"sniper":  {dmg: 115, pen: .97, rate: 1.45},
	"mpistol": {dmg: 24, pen: .5, rate: .065},
	"shotgun": {dmg: 20, pen: .6, rate: .85, pellets: 8},
	"lmg":     {dmg: 32, pen: .8, rate: .08},
	"scout":   {dmg: 88, pen: .85, rate: 1.2},
	"he":      {nade: true, pen: .57},
	"flash":   {nade: true},
	"smoke":   {nade: true},
	"molly":   {nade: true, pen: 1},
}

var partMult = map[string]float64{"h": 4, "c": 1, "s": 1.25, "l": .75}

// ---- o'yin holati ----

type bombState struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
	S string  `json:"s"`
	T float64 `json:"t"`
}

type gameState struct {
	R  int        `json:"r"`
	Ph string     `json:"ph"`
	M  string     `json:"m"`            // xarita
	Md string     `json:"md"`           // rejim: comp | dm | gg | wm | rt (modes.go)
	Tm bool       `json:"tm,omitempty"` // deathmatch jamoaviy: faqat raqib jamoani o'ldirish mumkin
	Rs string     `json:"rs"`           // retake: shu raunddagi bomba nuqtasi
	T  float64    `json:"t"`
	SA int        `json:"sA"`
	SB int        `json:"sB"`
	Fm int        `json:"fm"`
	B  *bombState `json:"b"`
	W  string     `json:"w"`
	Y  string     `json:"y"`
	Bm int        `json:"bm"`
	E  string     `json:"e"`
	Cd float64    `json:"cd"`
	// deathmatch / qurol poygasi
	Sc  map[string]int `json:"sc,omitempty"`  // o'yinchi id → server tasdiqlagan o'ldirishlar
	Wn  string         `json:"wn,omitempty"`  // g'olib id
	Wnm string         `json:"wnm,omitempty"` // g'olib ismi
	// statistika (stats.go)
	Mv  string         `json:"mv,omitempty"`  // raund MVP id
	Mvn string         `json:"mvn,omitempty"` // raund MVP ismi
	Mvr string         `json:"mvr,omitempty"` // sabab: "k3" (3 o'ldirish), "bomb", "defuse"
	St  []statRow      `json:"st,omitempty"`  // o'yin oxiridagi jadval
	Vt  map[string]int `json:"vt,omitempty"`  // keyingi xarita uchun ovozlar

	liveAt time.Time // jonli faza boshlangan vaqt (xarid vaqtini bilish uchun)
}

func newGameState() *gameState {
	return &gameState{Ph: "warmup", M: defaultMap, Md: modeComp, Fm: 12, Bm: -1, Cd: 6}
}

func randEpoch() string {
	var b [3]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// format qoidalari — brauzerdagi halfOf/sideOfSquad/matchWinner bilan bir xil
func halfOf(r, h int) int {
	if r < 1 {
		r = 1
	}
	if r <= 2*h {
		return (r - 1) / h
	}
	return 2 + (r-2*h-1)/otHalf
}

func sideOfSquad(sq string, r, h int) string {
	t := "T"
	for x := 1; x <= halfOf(r, h); x++ {
		if x == 1 || x%2 == 1 {
			t = other(t)
		}
	}
	if sq == "B" {
		return other(t)
	}
	return t
}

func other(side string) string {
	if side == "T" {
		return "CT"
	}
	return "T"
}

func halfStart(r, h int) bool { return r > 1 && halfOf(r, h) != halfOf(r-1, h) }

func matchWinner(a, b, r, h int) string {
	if a == b {
		return ""
	}
	need := h + 1
	if r > 2*h {
		need = h + (r-2*h-1)/(2*otHalf)*otHalf + otHalf + 1
	}
	if max(a, b) < need {
		return ""
	}
	if a > b {
		return "A"
	}
	return "B"
}

// ---- har bir o'yinchining server tomonidagi holati ----

type sample struct {
	t  time.Time
	p  vec3
	cr float64
}

type pstate struct {
	hist       [200]sample // ~9 soniyalik harakat tarixi (soniyasiga ~22 holat)
	hn         int
	pos        vec3
	posOK      bool
	moveAt     time.Time
	fixAt      time.Time
	badSpawnAt time.Time
	alive      bool
	life       int // qaysi raundda tirilgan (warmup'da -1)
	hp         float64
	armor      float64
	helmet     bool
	kit        bool
	tm         string
	hitSeen    int
	okHits     map[int]bool
	hitAt      []time.Time
	dealt      map[string]time.Time // kimga oxirgi marta tasdiqlangan o'q tekkan
	nadeN      int
	nadeAt     time.Time
	plN        int
	dfN        int
	deN        int
	deadAt     time.Time // oxirgi o'lim (qayta tug'ilish vaqti uchun)
	spawnAt    time.Time // oxirgi tirilish (tug'ilgandagi himoya)
	lvAt       time.Time // qurol poygasida oxirgi qurol almashgan vaqt
	// iqtisod (economy.go)
	money            int
	ecoE             string // qaysi o'yinda pul berilgan
	inv              inventory
	buyN             int
	dropN, pickN     int
	okDrops, okPicks map[int]bool
	nadeOK           bool
	// server jonini nolga tushirgan, lekin mijoz hali o'lmagan
	dueAt      time.Time
	dueBy      string
	dueW       string
	dueHS      bool
	killSent   bool
	forced     bool
	warn       int
	lastWarnAt time.Time
}

func newPState() *pstate {
	return &pstate{okHits: map[int]bool{}, dealt: map[string]time.Time{}, life: -2, hp: 100, inv: newInventory(), okDrops: map[int]bool{}, okPicks: map[int]bool{}}
}

func (p *pstate) push(s sample) { p.hist[p.hn%len(p.hist)] = s; p.hn++ }

// recent: oxirgi `win` ichidagi holatlar (eng yangisi birinchi), kamida bittasi.
func (p *pstate) recent(now time.Time, win time.Duration) []sample {
	out := []sample{}
	n := min(p.hn, len(p.hist))
	for i := 0; i < n; i++ {
		s := p.hist[(p.hn-1-i)%len(p.hist)]
		if i > 0 && now.Sub(s.t) > win {
			break
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, sample{t: now, p: p.pos})
	}
	return out
}

func (c *client) warnLocked(what string, now time.Time) {
	p := c.ps
	p.warn++
	if now.Sub(p.lastWarnAt) > 10*time.Second {
		p.lastWarnAt = now
		name := cleanGuestName(c.info.Nm)
		log.Printf("hakam: xona=%s o'yinchi=%s %s (jami %d)", c.room.name, name, what, p.warn)
	}
}

// ---- mijoz holatini tekshirish ----

type hitClaim struct {
	I  int     `json:"i"`
	T  string  `json:"t"`
	D  float64 `json:"d"`
	Pn float64 `json:"pn"`
	P  string  `json:"p"`
	W  string  `json:"w"`
	Pc int     `json:"pc,omitempty"` // drobovik: shu nishonga tekkan saochmalar soni
}

type deathClaim struct {
	N  int    `json:"n"`
	By string `json:"by"`
	W  string `json:"w"`
	HS int    `json:"hs"`
}

func num(raw json.RawMessage) (float64, bool) {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func inBuyTime(g *gameState, now time.Time) bool {
	return g.Ph == "warmup" || g.Ph == "freeze" || (g.Ph == "live" && now.Sub(g.liveAt) < 21*time.Second)
}

// refereeLocked mijozning yangi holatini tekshiradi va kerak bo'lsa tuzatadi (fields o'zgaradi).
// r.mu ushlangan bo'lishi kerak.
func (r *room) refereeLocked(c *client, f map[string]json.RawMessage, info presInfo, now time.Time) {
	p, g := c.ps, r.gs
	if fm, ok := num(f["pf"]); ok && (fm == 8 || fm == 12) {
		c.prefFm = int(fm)
	}
	al, _ := num(f["al"])
	claimAlive := al == 1

	// tirilish: isinishda yoki yangi raund boshida
	newLife := false
	survived := p.alive && g.R > 1 && p.life == g.R-1 // oldingi raunddan tirik qolgan (qurollari qoladi)
	if claimAlive && info.Sq != "" {
		switch {
		case g.Ph == "warmup" && (!p.alive || p.life != -1):
			newLife, p.life = true, -1
		case g.respawns() && g.Ph == "live" && info.In == g.R && g.R > 0 && (p.life != g.R || (!p.alive && now.Sub(p.deadAt) >= respawnDelay)):
			newLife, p.life = true, g.R // qayta tug'iladigan rejimlar
		case !g.respawns() && (g.Ph == "freeze" || g.Ph == "live") && info.In == g.R && g.R > 0 && p.life != g.R:
			newLife, p.life = true, g.R
		}
	}
	if newLife {
		p.alive, p.hp, p.posOK, p.forced, p.killSent, p.dueAt, p.spawnAt = true, 100, false, false, false, time.Time{}, now
		if !g.respawns() && g.Ph != "warmup" {
			r.ecoNewLifeLocked(c, survived, now)
		}
	}
	if info.Tm != p.tm {
		p.tm, p.posOK = info.Tm, false // jamoa almashdi: boshqa bazaga o'tadi
	}
	if !claimAlive {
		if p.alive {
			p.deadAt = now
		}
		p.alive = false
	} else if !p.alive {
		f["al"] = jsonMsg(0) // raund o'rtasida o'z-o'zidan tirilib bo'lmaydi
	}

	// zirh va to'plam faqat xarid vaqtida ko'payishi mumkin
	ar, _ := num(f["ar"])
	hm, _ := num(f["hm"])
	kt, _ := num(f["kt"])
	ar = math.Max(0, math.Min(100, ar))
	if r.ecoOn() {
		r.clampGearLocked(p, ar, hm == 1, kt == 1) // raqobatda zirh faqat sotib olinadi
	} else if newLife || inBuyTime(g, now) {
		p.armor, p.helmet, p.kit = ar, hm == 1, kt == 1
	} else {
		p.armor = math.Min(p.armor, ar)
		p.helmet = p.helmet && hm == 1
		p.kit = p.kit && kt == 1
	}

	// harakat
	if p.alive {
		x, okx := num(f["x"])
		y, oky := num(f["y"])
		z, okz := num(f["z"])
		cr, _ := num(f["cr"])
		if okx && oky && okz {
			np := vec3{x, y, z}
			dt := now.Sub(p.moveAt).Seconds()
			ok := true
			if p.posOK {
				lim := 8*math.Min(dt, 3) + 1.6
				// isinishda va raund boshida o'z bazasi ichida joy almashish (qayta tug'ilish) mumkin
				respawn := (g.Ph == "warmup" || g.Ph == "freeze") && info.Tm != "" && r.gm().inSpawn(np, info.Tm)
				if (np.flat(p.pos) > lim && !respawn) || r.gm().insideSolid(np) || (np.Y-p.pos.Y > 2.8 && !respawn) || np.X < 0 || np.Z < 0 || np.X > gridW*cellSize || np.Z > gridH*cellSize {
					ok = false
				}
			} else if r.gm().insideSolid(np) || !r.spawnOKLocked(np, info.Tm) {
				// yangi hayot faqat o'z bazasida boshlanadi (mijoz bazaga o'tguncha biroz kutamiz)
				ok = false
				if p.badSpawnAt.IsZero() {
					p.badSpawnAt = now
				}
				p.pos = r.gm().spawnCenter(info.Tm)
				if now.Sub(p.badSpawnAt) > 1200*time.Millisecond {
					p.posOK, p.moveAt = true, now
				}
			}
			if ok {
				p.pos, p.posOK, p.moveAt, p.badSpawnAt = np, true, now, time.Time{}
				p.push(sample{t: now, p: np, cr: cr})
			} else {
				if p.badSpawnAt.IsZero() {
					c.warnLocked(fmt.Sprintf("ruxsatsiz siljish: %.1f m / %.2f s (%.0f,%.0f → %.0f,%.0f)", np.flat(p.pos), dt, p.pos.X, p.pos.Z, np.X, np.Z), now)
				}
				f["x"], f["y"], f["z"] = jsonMsg(p.pos.X), jsonMsg(p.pos.Y), jsonMsg(p.pos.Z)
				if p.posOK && now.Sub(p.fixAt) > 700*time.Millisecond {
					p.fixAt = now
					c.trySend(jsonMsg(map[string]any{"t": "fix", "x": p.pos.X, "y": p.pos.Y, "z": p.pos.Z}))
				}
			}
		}
	}

	r.checkDropsLocked(c, f, now)     // tashlangan va olingan qurollar
	r.checkBuysLocked(c, f, now)      // xaridlar
	r.checkNadeThrowLocked(c, f, now) // granata: server o'zi hisoblaydi
	r.checkHitsLocked(c, f, now)
	r.checkDeathLocked(c, f, now)
	r.checkPlantLocked(c, f, now)
}

func (c *client) trySend(msg []byte) {
	select {
	case c.send <- msg:
	default:
	}
}

func (r *room) checkHitsLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	p := c.ps
	raw := f["hi"]
	if len(raw) == 0 || raw[0] != '[' {
		return
	}
	var hits []hitClaim
	if json.Unmarshal(raw, &hits) != nil {
		delete(f, "hi")
		return
	}
	keep := hits[:0]
	maxSeen := p.hitSeen
	for _, h := range hits {
		if h.I <= p.hitSeen {
			if p.okHits[h.I] {
				keep = append(keep, h)
			}
			continue
		}
		maxSeen = max(maxSeen, h.I)
		if why := r.validHitLocked(c, h, now); why != "" {
			if why[0] != '~' { // "~" — qoidaga mos holat (masalan, tug'ilgandagi himoya), ogohlantirish emas
				c.warnLocked("rad etilgan o'q: "+why, now)
			}
			continue
		}
		p.okHits[h.I] = true
		keep = append(keep, h)
		r.applyHitLocked(c, h, now)
	}
	p.hitSeen = maxSeen
	for i := range p.okHits { // eskilarini tozalash
		if i < p.hitSeen-64 {
			delete(p.okHits, i)
		}
	}
	if len(keep) == 0 {
		f["hi"] = json.RawMessage("null")
	} else {
		f["hi"] = jsonMsg(keep)
	}
}

func eye(s sample) vec3 { return vec3{s.p.X, s.p.Y + 1.62 - s.cr*.48, s.p.Z} }

func (r *room) validHitLocked(c *client, h hitClaim, now time.Time) string {
	g, p := r.gs, c.ps
	if g.Ph != "live" && g.Ph != "warmup" && g.Ph != "post" {
		return "raund jonli emas"
	}
	if !p.alive {
		return "otgan o'yinchi o'lik"
	}
	v := r.clients[h.T]
	if v == nil || v == c || !v.ps.alive {
		return "nishon yo'q yoki o'lik"
	}
	if c.info.Tm == "" || v.info.Tm == "" || !r.foes(c, v) {
		return "o'z jamoadoshi"
	}
	spec, ok := weaponSpecs[h.W]
	mult, okp := partMult[h.P]
	if g.respawns() {
		if !v.ps.spawnAt.IsZero() && now.Sub(v.ps.spawnAt) < spawnShield {
			return "~tug'ilgandagi himoya"
		}
		if spec.nade {
			return "bu rejimda granata yo'q"
		}
		if g.Md == modeGG && h.W != "knife" {
			k := g.Sc[c.id]
			if h.W != ggWeapon(k) && !(k > 0 && h.W == ggWeapon(k-1) && now.Sub(p.lvAt) < 2*time.Second) {
				return "qurol poygasida boshqa qurol"
			}
		}
	}
	if ok && spec.nade {
		return "~granata zararini server o'zi hisoblaydi"
	}
	if !ok || !okp {
		return "noma'lum qurol"
	}
	if r.ecoOn() && !p.inv.has(h.W, now) {
		return "sizda yo'q qurol: " + h.W
	}
	limit := spec.dmg * mult
	if spec.melee {
		limit = spec.dmg
	}
	if spec.pellets > 1 { // saochmalar har xil joyga tegadi: eng yomon holat — hammasi qism ko'paytmasi bilan
		limit = spec.dmg * math.Max(mult, partMult["s"]) * float64(min(max(h.Pc, 1), spec.pellets))
	}
	if h.D < 0 || h.D > limit*1.02+1 {
		return "zarar juda katta"
	}
	// o'q tezligi: bir soniyada qurol imkonidan ko'p tegmaydi
	cut := now.Add(-time.Second)
	kept := p.hitAt[:0]
	for _, t := range p.hitAt {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	p.hitAt = kept
	if float64(len(p.hitAt)) >= 1/spec.rate+2 {
		return "juda tez otish"
	}
	// masofa va devor (kechikish uchun oxirgi holatlar bo'yicha)
	shooter := p.recent(now, 350*time.Millisecond)
	target := v.ps.recent(now, 600*time.Millisecond)
	best := math.Inf(1)
	for _, s := range shooter {
		for _, t := range target {
			best = math.Min(best, eye(s).sub(t.p).len())
		}
	}
	if spec.melee {
		if best > 4.5 {
			return "pichoq uzoqdan"
		}
		return ""
	}
	if best > 180 {
		return "juda uzoq"
	}
	for _, s := range shooter {
		e := eye(s)
		for _, t := range target {
			// tananing o'rtasi va ikki cheti (burchakdan yelka chiqib turganda ham tegadi)
			dx, dz := t.p.X-e.X, t.p.Z-e.Z
			l := math.Hypot(dx, dz)
			if l < .01 {
				return ""
			}
			px, pz := -dz/l*.24, dx/l*.24
			for _, hgt := range []float64{1.55 - t.cr*.45, 1.1 - t.cr*.4, .45} {
				for _, k := range []float64{0, 1, -1} {
					if r.gm().lineOfSight(e, vec3{t.p.X + px*k, t.p.Y + hgt, t.p.Z + pz*k}) {
						return ""
					}
				}
			}
		}
	}
	return "devor ortidan"
}

func (r *room) applyHitLocked(c *client, h hitClaim, now time.Time) {
	v := r.clients[h.T]
	spec := weaponSpecs[h.W]
	vp := v.ps
	c.ps.hitAt = append(c.ps.hitAt, now)
	c.ps.dealt[v.id] = now
	dmg := h.D
	prot := vp.armor > 0 && h.P != "l" && (h.P != "h" || vp.helmet)
	if prot {
		red := dmg * spec.pen
		vp.armor = math.Max(0, vp.armor-(dmg-red)*.5)
		dmg = red
	}
	r.noteDamageLocked(c, math.Min(dmg, math.Max(0, vp.hp)))
	vp.hp -= dmg
	if v.bot != nil {
		v.bot.alertAt = now
		if vp.hp <= 0 {
			r.botDieLocked(v, c, h.W, h.P == "h", now)
		}
		return
	}
	if vp.hp <= 0 && vp.dueAt.IsZero() {
		vp.dueAt, vp.dueBy, vp.dueW, vp.dueHS = now, c.id, h.W, h.P == "h"
	}
}

// checkDeathLocked: o'lgan o'yinchining "meni X o'ldirdi" xabari faqat server tasdiqlagan
// tegishdan keyin (yoki X yaqinda granata otgan bo'lsa) qabul qilinadi.
func (r *room) checkDeathLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	raw := f["de"]
	if len(raw) == 0 || raw[0] != '{' {
		return
	}
	var d deathClaim
	if json.Unmarshal(raw, &d) != nil || d.N <= c.ps.deN {
		return
	}
	c.ps.deN = d.N
	if !c.deSeen {
		return // ulanishning birinchi xabaridagi eski o'lim
	}
	if c.ps.alive {
		c.ps.deadAt = now
	}
	c.ps.alive = false
	c.ps.dueAt = time.Time{}
	killer := r.clients[d.By]
	if d.By != "" && !r.creditOK(killer, c, d.W, now) {
		c.warnLocked("tasdiqlanmagan o'ldirish: "+d.By, now)
		d.By, killer = "", nil
		f["de"] = jsonMsg(d)
	}
	r.tr.death(r, c, killer, d.HS == 1)
	r.noteDeathLocked(c, killer, d.HS == 1, d.W)
	if killer != nil {
		r.creditKillLocked(c, killer, now)
	}
}

func (r *room) creditOK(k, victim *client, w string, now time.Time) bool {
	if !r.foes(k, victim) {
		return false
	}
	if t, ok := k.ps.dealt[victim.id]; ok && now.Sub(t) < 6*time.Second {
		return true
	}
	if spec := weaponSpecs[w]; spec.nade && !k.ps.nadeAt.IsZero() && now.Sub(k.ps.nadeAt) < 12*time.Second {
		return true
	}
	return false
}

// checkPlantLocked: bomba o'rnatish va zararsizlantirish.
func (r *room) checkPlantLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	g, p := r.gs, c.ps
	if raw := f["pl"]; len(raw) > 0 && raw[0] == '{' {
		var pl struct {
			N int     `json:"n"`
			X float64 `json:"x"`
			Z float64 `json:"z"`
			S string  `json:"s"`
			R int     `json:"r"`
		}
		if json.Unmarshal(raw, &pl) == nil && pl.N > p.plN {
			p.plN = pl.N
			if g.Ph == "live" && g.B == nil && pl.R == g.R && c.info.Tm == "T" && p.alive && c.info.In == g.R {
				at := vec3{pl.X, 0, pl.Z}
				site := r.gm().siteAt(pl.X, pl.Z)
				if g.Md == modeWM && site != "A" {
					site = "" // Wingman: faqat A nuqta
				}
				if site != "" && site == pl.S && at.flat(p.pos) < 2.5 && r.stoodLocked(p, now, 2.6, func(s sample) bool { return r.gm().siteAt(s.p.X, s.p.Z) == site && s.p.flat(at) < 2.5 }) {
					g.B = &bombState{X: pl.X, Z: pl.Z, S: site, T: 40}
					r.planter = c.id
					r.addMoneyLocked(c, 300)
				} else {
					c.warnLocked("noto'g'ri bomba o'rnatish", now)
				}
			}
		}
	}
	if raw := f["df"]; len(raw) > 0 && raw[0] == '{' {
		var df struct {
			N int `json:"n"`
			R int `json:"r"`
		}
		if json.Unmarshal(raw, &df) == nil && df.N > p.dfN {
			p.dfN = df.N
			if g.Ph == "live" && g.B != nil && df.R == g.R && c.info.Tm == "CT" && p.alive {
				b := vec3{g.B.X, 0, g.B.Z}
				need := 6.3
				if p.kit {
					need = 3.6
				}
				if p.pos.flat(b) < 2.5 && r.stoodLocked(p, now, need, func(s sample) bool { return s.p.flat(b) < 2.5 }) {
					r.defuser = c.id
					r.endRoundLocked("CT", "defuse")
				} else {
					c.warnLocked("noto'g'ri zararsizlantirish", now)
				}
			}
		}
	}
}

// stoodLocked: o'yinchi oxirgi `secs` soniya davomida shart bajariladigan joyda turdimi.
func (r *room) stoodLocked(p *pstate, now time.Time, secs float64, ok func(sample) bool) bool {
	n := min(p.hn, len(p.hist))
	covered := 0.0
	for i := 0; i < n; i++ {
		s := p.hist[(p.hn-1-i)%len(p.hist)]
		if !ok(s) {
			return false
		}
		covered = now.Sub(s.t).Seconds()
		if covered >= secs {
			return true
		}
	}
	// tarix qisqa (sekin ulanish): bor holatlar hammasi mos va yetarli vaqtni qamrasa
	return covered >= secs*.85
}

// ---- raund (avval xost brauzerida ishlardi) ----

func (r *room) endRoundLocked(side, why string) {
	g := r.gs
	g.Ph, g.T, g.W, g.Y = "post", 6, side, why
	if sideOfSquad("A", g.R, g.Fm) == side {
		g.SA++
	} else {
		g.SB++
	}
	r.roundMVPLocked(side, why)
	r.roundAwardsLocked(side, why, g.B != nil)
}

func (r *room) tickLocked(now time.Time, dt float64) *matchResult {
	g := r.gs
	if key := g.E + ":" + itoa(g.R); key != r.nadeKey { // yangi raund: granatalar va olovlar yo'qoladi
		r.clearNadesLocked()
		r.nadeKey = key
	}
	r.nadesTickLocked(now)
	nA, nB := 0, 0
	var first *client
	for _, c := range r.clients {
		if c.info.Sq == "A" {
			nA++
		} else if c.info.Sq == "B" {
			nB++
		}
		if c.info.Sq != "" && (first == nil || c.joined.Before(first.joined)) {
			first = c
		}
	}
	alive := func(side string) (part, live int) {
		for _, c := range r.clients {
			if c.info.Tm == side && c.info.In == g.R && g.R > 0 {
				part++
				if c.ps.alive {
					live++
				}
			}
		}
		return
	}
	r.forceDeathsLocked(now)
	prev := g.Ph
	if g.respawns() {
		r.tickModeLocked(now, dt, nA, nB)
		return r.tr.phase(r, prev, now)
	}
	if g.Ph != "warmup" && g.Ph != "over" && (nA == 0 || nB == 0) {
		g.reset()
	}
	switch g.Ph {
	case "warmup":
		if first != nil && first.prefFm != 0 {
			g.Fm = first.prefFm
		}
		if f := g.fixedFm(); f != 0 {
			g.Fm = f
		}
		if nA > 0 && nB > 0 {
			g.Cd -= dt
			g.T = math.Max(0, g.Cd)
			if g.Cd <= 0 {
				g.R, g.Ph, g.T, g.SA, g.SB, g.B, g.Bm, g.E = 1, "freeze", 6, 0, 0, nil, -1, randEpoch()
				g.Mv, g.Mvn, g.Mvr, g.St, r.planter, r.defuser = "", "", "", nil, "", ""
				r.purses, r.matchAt = nil, now
				if g.Md == modeRT {
					r.retakeRoundLocked()
				}
			}
		} else {
			g.Cd, g.T = 6, 0
		}
	case "freeze":
		g.T -= dt
		if g.T <= 0 {
			g.Ph, g.T, g.liveAt = "live", g.roundTime(), now
		}
	case "live":
		if g.B != nil {
			g.B.T -= dt
		} else {
			g.T -= dt
		}
		pCT, aCT := alive("CT")
		pT, aT := alive("T")
		switch {
		case g.B != nil && g.B.T <= 0:
			g.Bm = g.R
			r.bombBlastLocked(g.B, now)
			r.endRoundLocked("T", "bomb")
		case pCT > 0 && aCT == 0:
			r.endRoundLocked("T", "kill")
		case pT > 0 && aT == 0 && g.B == nil:
			r.endRoundLocked("CT", "kill")
		case g.B == nil && g.T <= 0:
			r.endRoundLocked("CT", "time")
		}
	case "post":
		g.T -= dt
		if g.T <= 0 {
			if matchWinner(g.SA, g.SB, g.R, g.Fm) != "" {
				g.Ph, g.T = "over", 10
				r.finalStatsLocked()
			} else {
				g.R++
				g.Ph, g.T, g.B = "freeze", 5, nil
				g.Mv, g.Mvn, g.Mvr, r.planter, r.defuser = "", "", "", "", ""
				if halfStart(g.R, g.Fm) {
					g.T = 10
				}
				if g.Md == modeRT {
					r.retakeRoundLocked()
				}
			}
		}
	case "over":
		g.T -= dt
		if g.T <= 0 {
			r.applyVoteLocked(now)
		}
	}
	return r.tr.phase(r, prev, now)
}

// forceDeathsLocked: server hisobida joni tugagan o'yinchi 0,7 soniyada o'lmasa —
// unga o'limni buyuramiz; 1,5 soniyada ham bo'ysunmasa, boshqalarga o'lik ko'rsatamiz.
func (r *room) forceDeathsLocked(now time.Time) {
	for _, c := range r.clients {
		p := c.ps
		if !p.alive || p.dueAt.IsZero() {
			continue
		}
		age := now.Sub(p.dueAt)
		if age > 700*time.Millisecond && !p.killSent {
			p.killSent = true
			c.trySend(jsonMsg(map[string]any{"t": "kill", "by": p.dueBy, "w": p.dueW, "hs": p.dueHS}))
		}
		if age > 1500*time.Millisecond {
			c.warnLocked("server o'ldirganini qabul qilmadi", now)
			p.alive, p.forced, p.dueAt, p.deadAt = false, true, time.Time{}, now
			killer := r.clients[p.dueBy]
			r.tr.death(r, c, killer, p.dueHS)
			r.noteDeathLocked(c, killer, p.dueHS, p.dueW)
			if killer != nil {
				r.creditKillLocked(c, killer, now)
			}
			// boshqalarga: o'lik va kim o'ldirgani
			var f map[string]json.RawMessage
			if json.Unmarshal(c.pres, &f) == nil {
				p.deN += 1000
				f["al"] = jsonMsg(0)
				f["de"] = jsonMsg(deathClaim{N: p.deN, By: p.dueBy, W: p.dueW, HS: map[bool]int{true: 1}[p.dueHS]})
				if out, err := json.Marshal(f); err == nil {
					c.pres = out
					r.broadcast(append(append([]byte(`{"t":"u","id":"`+c.id+`","p":`), out...), '}'), c.id)
				}
			}
		}
	}
}

// run: xona yashar ekan, soniyasiga 10 marta raundni yuritadi va holatni yuboradi.
func (r *room) run(h *hub) {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	last := time.Now()
	n := 0
	for {
		select {
		case <-r.stop:
			return
		case now := <-t.C:
			dt := math.Min(now.Sub(last).Seconds(), 1)
			last = now
			r.mu.Lock()
			if r.manual || len(r.clients) == 0 { // bo'sh (rasmiy) xona hech narsa hisoblamaydi
				r.mu.Unlock()
				continue
			}
			if n%20 == 0 {
				r.fillBotsLocked(now) // rasmiy xona: soniyasiga bir marta botlar soni va jamoalar
			}
			for _, c := range r.clients {
				if c.bot != nil {
					r.botStepLocked(c, now, dt)
				}
			}
			res := r.tickLocked(now, dt)
			if n++; n%2 == 0 { // holat soniyasiga 10 marta
				r.broadcast(jsonMsg(map[string]any{"t": "g", "g": r.gs}), "")
			}
			gains := r.gains
			r.gains = nil
			r.mu.Unlock()
			if res != nil && h.store.Ready() {
				go h.finishMatch(r, res)
			}
			if len(gains) > 0 && h.store.Ready() {
				go h.awardProgress(r, gains)
			}
		}
	}
}

func (m *gameMap) spawnCenter(side string) vec3 {
	if side == "CT" {
		return m.ctCenter
	}
	return m.tCenter
}

// gm: xonaning hozirgi xaritasi.
func (r *room) gm() *gameMap { return mapByID(r.gs.M) }

// reset: yangi o'yin (xarita saqlanadi).
func (g *gameState) reset() {
	m, md, tm := g.M, g.Md, g.Tm
	*g = *newGameState()
	g.M, g.Md, g.Tm = m, md, tm
}
