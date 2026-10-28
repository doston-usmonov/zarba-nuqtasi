package main

// Iqtisod va qurollar serverda (raqobat rejimi, isinishdan tashqari).
//
// Server har bir o'yinchining pulini o'zi yuritadi (CS qoidalari bilan: boshida va yarim
// vaqtda $800, overtime'da $12 500, raund g'alabasi $3250/$3500, mag'lubiyat $1400 + $500
// har ketma-ket mag'lubiyat uchun, o'ldirish uchun qurolga qarab, bomba qo'yish $300) va
// pulni brauzerga {"t":"eco","m":...} bilan yuboradi.
//
// Xaridlar presence'dagi "bu" ro'yxati bilan keladi va tekshiriladi: xarid vaqti, o'z bazasi,
// pul yetarliligi, granata chegaralari. Rad etilgan xarid {"t":"eco","deny":n} bilan qaytariladi.
// Server har kimning qo'lidagi qurollarni ham biladi (boshlang'ich to'pponcha, sotib olingan,
// yerdan olingan, tashlangan): sizda yo'q qurol bilan otilgan o'q va sizda yo'q granata
// qabul qilinmaydi, yerdagi bitta qurolni ikki kishi ola olmaydi.

import (
	"encoding/json"
	"math"
	"time"
)

type buyItem struct {
	price int
	slot  int // 1 — asosiy, 2 — to'pponcha, 4 — granata, 0 — jihoz
	max   int // granatalar
	kill  int // o'ldirish mukofoti
}

// web/index.html dagi W va GEAR jadvallari bilan bir xil
var shop = map[string]buyItem{
	"pistol":  {price: 200, slot: 2, kill: 300},
	"deagle":  {price: 700, slot: 2, kill: 300},
	"smg":     {price: 1250, slot: 1, kill: 600},
	"carbine": {price: 2050, slot: 1, kill: 300},
	"rifle":   {price: 2700, slot: 1, kill: 300},
	"sniper":  {price: 4750, slot: 1, kill: 100},
	"mpistol": {price: 500, slot: 2, kill: 300},
	"shotgun": {price: 1200, slot: 1, kill: 900},
	"lmg":     {price: 5000, slot: 1, kill: 300},
	"scout":   {price: 1700, slot: 1, kill: 300},
	"he":      {price: 300, slot: 4, max: 1, kill: 300},
	"flash":   {price: 200, slot: 4, max: 2, kill: 300},
	"smoke":   {price: 300, slot: 4, max: 1, kill: 300},
	"molly":   {price: 400, slot: 4, max: 1, kill: 300},
	"kevlar":  {price: 650},
	"helmet":  {price: 1000},
	"kit":     {price: 400},
	"taser":   {price: 200, kill: 100}, // elektroshoker «Uchqun», bir martalik
}

const (
	nadeMax   = 4
	moneyMax  = 16000
	startCash = 800
	otCash    = 12500
)

func killReward(w string) int {
	if w == "knife" {
		return 1500
	}
	if it, ok := shop[w]; ok && it.kill > 0 {
		return it.kill
	}
	return 300
}

// inventory: server biladigan qurollar (bir hayot davomida).
type inventory struct {
	prim, sec string
	taser     bool
	nades     map[string]int
	gone      map[string]time.Time // yaqinda qo'ldan chiqqan qurollar (kechikkan o'qlar uchun)
}

func newInventory() inventory {
	return inventory{sec: "pistol", nades: map[string]int{}, gone: map[string]time.Time{}}
}

func (iv *inventory) has(w string, now time.Time) bool {
	if w == iv.prim || w == iv.sec || w == "knife" {
		return w != ""
	}
	if w == "taser" && iv.taser {
		return true
	}
	t, ok := iv.gone[w]
	return ok && now.Sub(t) < 1500*time.Millisecond
}

func (iv *inventory) remove(w string, now time.Time) bool {
	switch {
	case w == "taser" && iv.taser:
		iv.taser = false
	case w == "":
		return false
	case w == iv.prim:
		iv.prim = ""
	case w == iv.sec:
		iv.sec = ""
	default:
		return false
	}
	iv.gone[w] = now
	return true
}

func (iv *inventory) put(w string, now time.Time) {
	it := shop[w]
	if it.slot == 1 {
		if iv.prim != "" && iv.prim != w {
			iv.gone[iv.prim] = now
		}
		iv.prim = w
	} else if it.slot == 2 {
		if iv.sec != "" && iv.sec != w {
			iv.gone[iv.sec] = now
		}
		iv.sec = w
	}
}

func (iv *inventory) nadeCount() int {
	n := 0
	for _, c := range iv.nades {
		n += c
	}
	return n
}

// ecoOn: pul va qurollar tekshiriladimi.
func (r *room) ecoOn() bool {
	g := r.gs
	return !g.respawns() && g.Md != modeRT && g.Md != modeAim && g.E != "" && g.R > 0 && g.Ph != "warmup" && g.Ph != "over"
}

func (c *client) sendEco(extra map[string]any) {
	m := map[string]any{"t": "eco", "m": c.ps.money}
	for k, v := range extra {
		m[k] = v
	}
	c.trySend(jsonMsg(m))
}

func (r *room) addMoneyLocked(c *client, v int) {
	if c.bot != nil || !r.ecoOn() {
		return
	}
	c.ps.money = max(0, min(moneyMax, c.ps.money+v))
	c.sendEco(nil)
}

// purse: aloqa uzilib qayta ulangan o'yinchi puli, qurollari va joni yo'qolmasligi uchun.
type purse struct {
	e           string
	money       int
	inv         inventory
	armor, hp   float64
	helmet, kit bool
	life        int
	alive       bool
}

func purseKey(c *client) string {
	if c.acct != nil {
		return "u:" + c.acct.name
	}
	return "g:" + c.ip + "|" + cleanGuestName(c.info.Nm)
}

// savePurseLocked: o'yinchi chiqib ketdi (yoki aloqa uzildi).
func (r *room) savePurseLocked(c *client) {
	p := c.ps
	if c.bot != nil || p == nil || p.ecoE == "" || p.ecoE != r.gs.E {
		return
	}
	if r.purses == nil {
		r.purses = map[string]purse{}
	}
	r.purses[purseKey(c)] = purse{e: p.ecoE, money: p.money, inv: p.inv, armor: p.armor, hp: p.hp, helmet: p.helmet, kit: p.kit, life: p.life, alive: p.alive}
}

// ecoNewLifeLocked: raund boshida tirilish — pul va qurollar CS qoidalari bo'yicha.
func (r *room) ecoNewLifeLocked(c *client, survived bool, now time.Time) {
	g, p := r.gs, c.ps
	if !r.ecoOn() || c.bot != nil {
		return
	}
	newHalf := halfStart(g.R, g.Fm)
	if sv, ok := r.purses[purseKey(c)]; ok && p.ecoE != g.E && sv.e == g.E {
		delete(r.purses, purseKey(c))
		p.ecoE, p.money = g.E, sv.money
		if sv.alive && sv.life == g.R { // shu raund davom etmoqda: hammasi joyida qoladi
			p.inv, p.armor, p.helmet, p.kit, p.hp = sv.inv, sv.armor, sv.helmet, sv.kit, sv.hp
			c.sendEco(nil)
			return
		}
		survived = sv.alive && sv.life == g.R-1
		if survived {
			p.inv, p.armor, p.helmet = sv.inv, sv.armor, sv.helmet
		}
	}
	switch {
	case p.ecoE != g.E || g.R == 1:
		p.ecoE, p.money = g.E, startCash
	case newHalf:
		p.money = startCash
		if g.R > 2*g.Fm {
			p.money = otCash
		}
	}
	if !survived || newHalf || g.R == 1 {
		p.inv = newInventory()
		p.armor, p.helmet, p.kit = 0, false, false
	}
	p.kit = false
	c.sendEco(nil)
}

// roundAwardsLocked: raund oxiridagi pul (jamoaviy ketma-ket mag'lubiyat bilan).
func (r *room) roundAwardsLocked(side, why string, planted bool) {
	g := r.gs
	if r.loss == nil {
		r.loss = map[string]int{}
	}
	if r.lossE != g.E {
		r.loss, r.lossE = map[string]int{}, g.E
	}
	winSq := "A"
	if sideOfSquad("A", g.R, g.Fm) != side {
		winSq = "B"
	}
	loseSq := map[string]string{"A": "B", "B": "A"}[winSq]
	streak := r.loss[loseSq]
	for _, c := range r.clients {
		if c.bot != nil || c.info.Sq == "" || c.ps.ecoE != g.E {
			continue
		}
		if c.info.Sq == winSq {
			if why == "bomb" {
				r.addMoneyLocked(c, 3500)
			} else {
				r.addMoneyLocked(c, 3250)
			}
		} else {
			bonus := 1400 + 500*min(streak, 4)
			if sideOfSquad(c.info.Sq, g.R, g.Fm) == "T" && planted {
				bonus += 800
			}
			r.addMoneyLocked(c, bonus)
		}
	}
	r.loss[winSq] = 0
	r.loss[loseSq] = streak + 1
	// yarim vaqt oldidan: keyingi yarimda hisoblagich noldan
	if !matchWinner2(g) && halfStart(g.R+1, g.Fm) {
		r.loss = map[string]int{}
	}
}

func matchWinner2(g *gameState) bool { return matchWinner(g.SA, g.SB, g.R, g.Fm) != "" }

type buyEvent struct {
	N  int    `json:"n"`
	ID string `json:"id"`
	S  int    `json:"s,omitempty"` // 1 — qaytarib sotish, 2 — jamoadoshga pul berish
	T  string `json:"t,omitempty"` // pul kimga (o'yinchi id)
	A  int    `json:"a,omitempty"` // qancha
}

// boughtItem: xarid — shu raundning xarid vaqtida to'liq narxiga qaytarib sotsa bo'ladi.
type boughtItem struct {
	id      string
	price   int
	armor0  float64 // xariddan oldingi zirh (zirh qaytarilsa tiklanadi)
	helmet0 bool
	e       string
	r       int
}

// checkBuysLocked: presence'dagi yangi xaridlar.
func (r *room) checkBuysLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	raw := f["bu"]
	if len(raw) == 0 || raw[0] != '[' {
		return
	}
	var list []buyEvent
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	p := c.ps
	for _, b := range list {
		if b.N <= p.buyN {
			continue
		}
		p.buyN = b.N
		if !r.ecoOn() {
			continue // isinish va boshqa rejimlar: hamma narsa bepul
		}
		if b.S == 2 {
			if why := r.giveLocked(c, b.T, b.A, now); why != "" {
				c.sendEco(map[string]any{"deny": b.N, "id": "$", "why": why})
			}
			continue
		}
		what, do := "xarid", r.buyLocked
		if b.S == 1 {
			what, do = "qaytarib sotish", r.sellLocked
		}
		if why := do(c, b.ID, now); why != "" {
			c.warnLocked("rad etilgan "+what+": "+b.ID+" ("+why+")", now)
			c.sendEco(map[string]any{"deny": b.N, "id": b.ID})
		}
	}
	delete(f, "bu") // boshqalarga kerak emas
}

func (r *room) buyLocked(c *client, id string, now time.Time) string {
	g, p := r.gs, c.ps
	it, ok := shop[id]
	switch {
	case !ok:
		return "noma'lum narsa"
	case !p.alive:
		return "o'lik"
	case !inBuyTime(g, now):
		return "xarid vaqti tugagan"
	case !r.nearSpawnLocked(p.pos, c.info.Tm):
		return "bazadan tashqarida"
	}
	price := it.price
	if id == "helmet" && p.armor >= 100 && !p.helmet {
		price = 350
	}
	if price > p.money {
		return "pul yetmaydi"
	}
	a0, h0 := p.armor, p.helmet
	switch {
	case it.slot == 4:
		if p.inv.nades[id] >= it.max || p.inv.nadeCount() >= nadeMax {
			return "granata chegarasi"
		}
		p.inv.nades[id]++
	case it.slot == 1 || it.slot == 2:
		p.inv.put(id, now)
	case id == "kevlar":
		if p.armor >= 100 {
			return "zirh bor"
		}
		p.armor = 100
	case id == "helmet":
		if p.armor >= 100 && p.helmet {
			return "dubulg'a bor"
		}
		p.armor, p.helmet = 100, true
	case id == "taser":
		if p.inv.taser {
			return "uchqun bor"
		}
		p.inv.taser = true
	case id == "kit":
		if c.info.Tm != "CT" || p.kit {
			return "to'plam kerak emas"
		}
		p.kit = true
	}
	p.money -= price
	keep := p.bought[:0]
	for _, b := range p.bought { // faqat shu raunddagilar
		if b.e == g.E && b.r == g.R {
			keep = append(keep, b)
		}
	}
	p.bought = append(keep, boughtItem{id: id, price: price, armor0: a0, helmet0: h0, e: g.E, r: g.R})
	c.sendEco(nil)
	return ""
}

// giveLocked: jamoadoshga pul — xarid vaqtida, o'z bazasida, tirik odam jamoadoshga, $100 dan $5000 gacha
// (100 ga karrali), uning puli $16 000 dan oshmaydi. Bo'sh qator — bajarildi, aks holda sababi.
func (r *room) giveLocked(c *client, to string, a int, now time.Time) string {
	g, p := r.gs, c.ps
	t := r.clients[to]
	switch {
	case a < 100 || a > 5000 || a%100 != 0:
		return "noto'g'ri miqdor"
	case t == nil || t == c || t.bot != nil || t.info.Tm == "" || t.info.Tm != c.info.Tm || !t.ps.alive:
		return "jamoadosh topilmadi"
	case !p.alive:
		return "o'lik"
	case !inBuyTime(g, now):
		return "xarid vaqti tugagan"
	case !r.nearSpawnLocked(p.pos, c.info.Tm):
		return "bazadan tashqarida"
	case a > p.money:
		return "pul yetmaydi"
	case t.ps.money+a > moneyMax:
		return "jamoadoshning puli to'la"
	}
	p.money -= a
	t.ps.money += a
	c.sendEco(map[string]any{"gave": map[string]any{"to": clientName(t), "a": a}})
	t.sendEco(map[string]any{"gift": map[string]any{"from": clientName(c), "a": a}})
	return ""
}

// sellLocked: shu raundda sotib olingan narsani xarid vaqtida, bazada, hali qo'lda bo'lsa to'liq narxiga qaytarib oladi.
func (r *room) sellLocked(c *client, id string, now time.Time) string {
	g, p := r.gs, c.ps
	switch {
	case !p.alive:
		return "o'lik"
	case !inBuyTime(g, now):
		return "xarid vaqti tugagan"
	case !r.nearSpawnLocked(p.pos, c.info.Tm):
		return "bazadan tashqarida"
	}
	idx := -1
	for i := len(p.bought) - 1; i >= 0; i-- {
		b := p.bought[i]
		if b.e != g.E || b.r != g.R {
			continue
		}
		if id == "kevlar" || id == "helmet" { // zirh: faqat oxirgi zirh xaridi
			if b.id == "kevlar" || b.id == "helmet" {
				if b.id == id {
					idx = i
				}
				break
			}
			continue
		}
		if b.id == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "bu raundda sotib olinmagan"
	}
	b, it := p.bought[idx], shop[id]
	switch {
	case it.slot == 4:
		if p.inv.nades[id] <= 0 {
			return "granata yo'q"
		}
		p.inv.nades[id]--
	case it.slot == 1 || it.slot == 2:
		if !p.inv.remove(id, now) {
			return "qurol qo'lda emas"
		}
	case id == "kevlar" || id == "helmet":
		if p.armor < 100 {
			return "zirh shikastlangan"
		}
		p.armor, p.helmet = b.armor0, b.helmet0
	case id == "kit":
		if !p.kit {
			return "to'plam yo'q"
		}
		p.kit = false
	case id == "taser":
		if !p.inv.remove("taser", now) {
			return "uchqun yo'q"
		}
	default:
		return "noma'lum narsa"
	}
	p.bought = append(p.bought[:idx], p.bought[idx+1:]...)
	p.money = min(moneyMax, p.money+b.price)
	c.sendEco(nil)
	return ""
}

// forgetBought: qo'ldan chiqqan qurolni (tashlangan) endi qaytarib sotib bo'lmaydi.
func (p *pstate) forgetBought(w string) {
	for i := len(p.bought) - 1; i >= 0; i-- {
		if p.bought[i].id == w {
			p.bought = append(p.bought[:i], p.bought[i+1:]...)
			return
		}
	}
}

// groundLocked: yerdagi qurollar ro'yxati shu raund uchun.
func (r *room) groundLocked() {
	g := r.gs
	if r.ground == nil || r.groundR != g.R || r.groundE != g.E {
		r.ground, r.groundR, r.groundE = map[string]groundGun{}, g.R, g.E
	}
}

// nearSpawnLocked: o'z bazasida (bir katak chetlanish bilan — kechikish uchun).
func (r *room) nearSpawnLocked(pos vec3, side string) bool {
	gm := r.gm()
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			t := gm.cellType(wc(pos.X)+dx, wc(pos.Z)+dz)
			if (side == "T" && t == cTSpawn) || (side == "CT" && t == cCTSpawn) {
				return true
			}
		}
	}
	return false
}

// ---- yerdagi qurollar ----

type groundGun struct {
	w string
	r int // qaysi raundda tashlangan
}

type dropEvent struct {
	N  int    `json:"n"`
	ID string `json:"id"`
	W  string `json:"w"`
	R  int    `json:"r"`
}

// checkDropsLocked: tashlangan va olingan qurollar. Noto'g'rilari boshqalarga yuborilmaydi.
func (r *room) checkDropsLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	p, g := c.ps, r.gs
	r.groundLocked()
	if raw := f["dr"]; len(raw) > 0 && raw[0] == '[' {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			keep := list[:0]
			for _, e := range list {
				var d dropEvent
				if json.Unmarshal(e, &d) != nil {
					continue
				}
				if d.N <= p.dropN {
					if p.okDrops[d.N] {
						keep = append(keep, e)
					}
					continue
				}
				p.dropN = d.N
				if r.ecoOn() {
					if d.R != g.R || !p.inv.remove(d.W, now) {
						c.warnLocked("sizda yo'q qurol tashlandi: "+d.W, now)
						continue
					}
					p.forgetBought(d.W)
					r.ground[d.ID] = groundGun{w: d.W, r: g.R}
				}
				p.okDrops[d.N] = true
				keep = append(keep, e)
			}
			for n := range p.okDrops {
				if n < p.dropN-32 {
					delete(p.okDrops, n)
				}
			}
			f["dr"] = jsonMsg(keep)
		}
	}
	if raw := f["pk"]; len(raw) > 0 && raw[0] == '[' {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			keep := list[:0]
			for _, e := range list {
				var d dropEvent
				if json.Unmarshal(e, &d) != nil {
					continue
				}
				if d.N <= p.pickN {
					if p.okPicks[d.N] {
						keep = append(keep, e)
					}
					continue
				}
				p.pickN = d.N
				if r.ecoOn() {
					gg, ok := r.ground[d.ID]
					if !ok || !p.alive {
						// boshqa o'yinchi oldinroq olgan (yoki bunday qurol yo'q) — qurol qaytarib olinadi
						c.sendEco(map[string]any{"take": d.ID})
						continue
					}
					delete(r.ground, d.ID)
					p.inv.put(gg.w, now)
				}
				p.okPicks[d.N] = true
				keep = append(keep, e)
			}
			for n := range p.okPicks {
				if n < p.pickN-32 {
					delete(p.okPicks, n)
				}
			}
			f["pk"] = jsonMsg(keep)
		}
	}
}

// checkNadeThrowLocked: granata otildi — sizda bo'lsa server uni hisoblaydi, bo'lmasa boshqalarga ko'rsatilmaydi.
func (r *room) checkNadeThrowLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	raw := f["gn"]
	if len(raw) == 0 || raw[0] != '{' {
		return
	}
	var gn struct {
		N  int     `json:"n"`
		K  string  `json:"k"`
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
		Z  float64 `json:"z"`
		VX float64 `json:"vx"`
		VY float64 `json:"vy"`
		VZ float64 `json:"vz"`
	}
	p := c.ps
	if json.Unmarshal(raw, &gn) != nil || gn.N <= p.nadeN {
		if gn.N > 0 && gn.N == p.nadeN && !p.nadeOK {
			delete(f, "gn")
		}
		return
	}
	p.nadeN, p.nadeAt, p.nadeOK = gn.N, now, false
	start, vel := vec3{gn.X, gn.Y, gn.Z}, vec3{gn.VX, gn.VY, gn.VZ}
	eyeP := vec3{p.pos.X, p.pos.Y + 1.62, p.pos.Z}
	switch {
	case !p.alive || r.gs.respawns():
		c.warnLocked("granata otib bo'lmaydi", now)
	case start.sub(eyeP).len() > 3 || vel.len() > 30:
		c.warnLocked("granata noto'g'ri joydan otildi", now)
	case r.ecoOn() && p.inv.nades[gn.K] <= 0:
		c.warnLocked("sizda yo'q granata: "+gn.K, now)
	default:
		if r.ecoOn() {
			p.inv.nades[gn.K]--
		}
		p.nadeOK = true
		r.throwNadeLocked(c, gn.K, start, vel, now)
		return
	}
	delete(f, "gn")
}

// clampGearLocked: zirh, dubulg'a va to'plam faqat sotib olingan bo'lsa (raqobatda).
func (r *room) clampGearLocked(p *pstate, ar float64, hm, kt bool) {
	p.armor = math.Min(p.armor, ar)
	p.helmet = p.helmet && hm
	p.kit = p.kit && kt
}
