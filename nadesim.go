package main

// Granatalar serverda.
//
// Brauzer granatani qat'iy qadam (1/120 s) bilan simulyatsiya qiladi: bir xil boshlang'ich
// joy va tezlik har doim bir xil sakrash va portlash nuqtasini beradi. Server ham xuddi
// shu hisobni qiladi (nadeSim — web/index.html dagi nadeMove/nadeStep'ning aynan nusxasi),
// portlash vaqtini va joyini biladi va zararni o'zi qo'llaydi:
//   - portlovchi (he): 6,5 m gacha, devor ortiga o'tmaydi, zirh 43% ni oladi;
//   - yondiruvchi (molly): 7 s yonadi, har 0,25 s 9 zarar, tutun o'chiradi;
//   - tutun: faqat olovni o'chirish uchun kuzatiladi; ko'r qiluvchi — faqat brauzerda.
// Bomba portlashi ham serverda: 25 m gacha, zirh 22% ni oladi.

import (
	"math"
	"time"
)

// nadeG va nadeH o'zgaruvchi: Go doimiylarni aniq hisoblaydi, brauzer esa float64'da — natija
// oxirgi bitgacha bir xil bo'lishi uchun ular ham ish vaqtida ko'paytiriladi.
var (
	nadeG = 14.0
	nadeH = 1.0 / 120
)

const (
	nadeR     = .045
	smokeR    = 4.2
	smokeLife = 18.0
	fireR     = 2.5
	fireLife  = 7.0
	heR       = 6.5
)

// rayGridN: rayGrid, lekin urilgan sirtning normali bilan (brauzerdagi rayGrid bilan bir xil).
func (m *gameMap) rayGridN(o, d vec3, maxD float64) (t float64, n vec3, hit bool) {
	cx, cz := wc(o.X), wc(o.Z)
	stepX, stepZ := -1, -1
	if d.X > 0 {
		stepX = 1
	}
	if d.Z > 0 {
		stepZ = 1
	}
	inf := math.Inf(1)
	tDX, tDZ := inf, inf
	if d.X != 0 {
		tDX = math.Abs(cellSize / d.X)
	}
	if d.Z != 0 {
		tDZ = math.Abs(cellSize / d.Z)
	}
	tMX, tMZ := inf, inf
	if d.X != 0 {
		if d.X > 0 {
			tMX = (float64(cx+1)*cellSize - o.X) / math.Abs(d.X)
		} else {
			tMX = (o.X - float64(cx)*cellSize) / math.Abs(d.X)
		}
	}
	if d.Z != 0 {
		if d.Z > 0 {
			tMZ = (float64(cz+1)*cellSize - o.Z) / math.Abs(d.Z)
		} else {
			tMZ = (o.Z - float64(cz)*cellSize) / math.Abs(d.Z)
		}
	}
	tE, side := 0.0, -1
	for i := 0; i < 400; i++ {
		tX := math.Min(math.Min(tMX, tMZ), maxD)
		h := m.cellH(cx, cz)
		if tE > 0 && o.Y+d.Y*tE < h {
			if side == 0 {
				return tE, vec3{float64(-stepX), 0, 0}, true
			}
			return tE, vec3{0, 0, float64(-stepZ)}, true
		}
		if d.Y < 0 {
			if tt := (h - o.Y) / d.Y; tt >= tE-1e-6 && tt <= tX {
				return math.Max(tt, 0), vec3{0, 1, 0}, true
			}
		}
		if tX >= maxD {
			return maxD, vec3{}, false
		}
		if tMX < tMZ {
			tE = tMX
			tMX += tDX
			cx += stepX
			side = 0
		} else {
			tE = tMZ
			tMZ += tDZ
			cz += stepZ
			side = 1
		}
		if cx < -2 || cz < -2 || cx > gridW+1 || cz > gridH+1 {
			return tE, vec3{}, false
		}
	}
	return maxD, vec3{}, false
}

func (a vec3) dot(b vec3) float64                { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (m *gameMap) surfaceY(x, z float64) float64 { return m.cellH(wc(x), wc(z)) }

// nadeResult: granata qachon (otilgandan necha soniyada) va qayerda ishga tushadi.
type nadeResult struct {
	t      float64
	p      vec3
	ground bool // yerda turibdi (yondiruvchi faqat yerda yonadi)
}

// nadeSim: brauzerdagi nadeStep/nadeMove ning aynan nusxasi.
func (m *gameMap) nadeSim(kind string, p, v vec3) nadeResult {
	fuse := 0.0
	switch kind {
	case "he":
		fuse = 1.6
	case "flash":
		fuse = 1.5
	case "molly":
		fuse = 2.0
	}
	t, restT := 0.0, 0.0
	ground, rest := false, false
	for step := 0; step < 120*8; step++ {
		t += nadeH
		if rest {
			restT += nadeH
		} else {
			// nadeMove
			if !ground {
				v.Y -= nadeG * nadeH
			}
			if p.Y > 7.6 && v.Y > 0 {
				v.Y = -v.Y * .3
			}
			sp := math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
			if sp > 1e-5 {
				d := v.scale(1 / sp)
				dist := sp * nadeH
				ht, n, hit := m.rayGridN(p, d, dist+nadeR)
				if hit && ht <= dist+nadeR {
					p = p.add(d.scale(math.Max(0, ht-nadeR)))
					vn := v.dot(n)
					impact := -vn
					if n.Y > .5 {
						if kind == "molly" && t > .12 {
							p.Y = m.surfaceY(p.X, p.Z) + nadeR
							return nadeResult{t: t, p: p, ground: true}
						}
						if impact < 2.2 {
							ground = true
							v.Y = 0
							p.Y = m.surfaceY(p.X, p.Z) + nadeR
						} else {
							v = v.add(n.scale(-vn * 1.45))
							v.X *= .7
							v.Z *= .7
						}
					} else {
						v = v.add(n.scale(-vn * 1.45))
						v = v.scale(.8)
					}
				} else {
					p = p.add(d.scale(dist))
				}
			}
			if ground {
				hs := math.Hypot(v.X, v.Z)
				drop := math.Max(0, hs-7*nadeH)
				if hs > 0 {
					v.X *= drop / hs
					v.Z *= drop / hs
				}
				if m.surfaceY(p.X, p.Z)+nadeR < p.Y-.02 {
					ground = false
				} else if drop < .15 {
					rest = true
					v = vec3{}
				}
			}
			if p.Y < nadeR {
				p.Y = nadeR
			}
		}
		if fuse > 0 && t >= fuse {
			return nadeResult{t: t, p: p, ground: ground}
		}
		if kind == "smoke" && ((rest && restT > .3) || t > 6) {
			return nadeResult{t: t, p: p, ground: true}
		}
	}
	return nadeResult{t: t, p: p, ground: ground}
}

// ---- xonadagi granatalar ----

type liveNade struct {
	kind    string
	at      time.Time // ishga tushish vaqti
	p       vec3
	ground  bool
	owner   string
	ownerSq string
}

type smokeCloud struct {
	p  vec3
	at time.Time
}

type fireArea struct {
	p       vec3
	h0      float64
	at      time.Time
	next    time.Time
	owner   string
	ownerSq string
}

func smokeDensity(t float64) float64 {
	grow := math.Max(0, math.Min(1, t/1.4))
	fade := math.Max(0, math.Min(1, (smokeLife-t)/3))
	return grow * fade
}

// throwNadeLocked: o'yinchi granata otdi — server uning yo'lini hisoblab, portlashni rejalashtiradi.
func (r *room) throwNadeLocked(c *client, kind string, p, v vec3, now time.Time) {
	if kind != "he" && kind != "molly" && kind != "smoke" {
		return // ko'r qiluvchi faqat brauzerda ta'sir qiladi
	}
	if key := r.gs.E + ":" + itoa(r.gs.R); key != r.nadeKey {
		r.clearNadesLocked()
		r.nadeKey = key
	}
	res := r.gm().nadeSim(kind, p, v)
	r.nades = append(r.nades, &liveNade{kind: kind, at: now.Add(time.Duration(res.t * float64(time.Second))), p: res.p, ground: res.ground, owner: c.id, ownerSq: c.info.Sq})
}

// canHurt: granata egasining o'zi va raqiblari zarar ko'radi, jamoadoshlari — yo'q.
func (r *room) canHurt(owner, ownerSq string, v *client) bool {
	return v.id == owner || r.gs.ffa() || (v.info.Sq != "" && v.info.Sq != ownerSq)
}

// nadesTickLocked: vaqti kelgan granatalar, yonayotgan olovlar va bomba.
func (r *room) nadesTickLocked(now time.Time) {
	gm := r.gm()
	keep := r.nades[:0]
	for _, n := range r.nades {
		if now.Before(n.at) {
			keep = append(keep, n)
			continue
		}
		switch n.kind {
		case "he":
			r.explodeHELocked(n, now)
		case "smoke":
			sp := vec3{n.p.X, gm.surfaceY(n.p.X, n.p.Z), n.p.Z}
			r.smokes = append(r.smokes, &smokeCloud{p: sp, at: n.at})
			fk := r.fires[:0]
			for _, f := range r.fires {
				if f.p.sub(sp).len() >= smokeR+fireR*.6 {
					fk = append(fk, f)
				}
			}
			r.fires = fk
		case "molly":
			if n.ground || n.p.Y-gm.surfaceY(n.p.X, n.p.Z) < .3 {
				fp := vec3{n.p.X, gm.surfaceY(n.p.X, n.p.Z), n.p.Z}
				if !r.smokeCoversLocked(fp, 1, now) {
					r.fires = append(r.fires, &fireArea{p: fp, h0: fp.Y, at: n.at, next: n.at.Add(100 * time.Millisecond), owner: n.owner, ownerSq: n.ownerSq})
				}
			}
		}
	}
	r.nades = keep
	sk := r.smokes[:0]
	for _, s := range r.smokes {
		if now.Sub(s.at).Seconds() < smokeLife {
			sk = append(sk, s)
		}
	}
	r.smokes = sk
	fk := r.fires[:0]
	for _, f := range r.fires {
		if now.Sub(f.at).Seconds() >= fireLife || r.smokeCoversLocked(f.p, 1, now) {
			continue
		}
		for !now.Before(f.next) {
			f.next = f.next.Add(250 * time.Millisecond)
			r.fireTickLocked(f, now)
		}
		fk = append(fk, f)
	}
	r.fires = fk
}

func (r *room) smokeCoversLocked(p vec3, pad float64, now time.Time) bool {
	for _, s := range r.smokes {
		if smokeDensity(now.Sub(s.at).Seconds()) > .3 && math.Hypot(p.X-s.p.X, p.Z-s.p.Z) < smokeR*.8+pad {
			return true
		}
	}
	return false
}

// inFire: brauzerdagi inFire bilan bir xil qoida.
func (m *gameMap) inFire(f *fireArea, pos vec3) bool {
	if math.Hypot(pos.X-f.p.X, pos.Z-f.p.Z) >= fireR*.9 || pos.Y >= f.h0+1 || pos.Y <= f.h0-.6 {
		return false
	}
	if math.Abs(m.surfaceY(pos.X, pos.Z)-f.h0) > .01 {
		return false
	}
	return m.lineOfSight(vec3{f.p.X, f.h0 + .3, f.p.Z}, vec3{pos.X, f.h0 + .3, pos.Z})
}

func (r *room) fireTickLocked(f *fireArea, now time.Time) {
	for _, v := range r.clients {
		if !v.ps.alive || !v.ps.posOK || !r.canHurt(f.owner, f.ownerSq, v) || !r.gm().inFire(f, v.ps.pos) {
			continue
		}
		dmg := 9.0
		if v.ps.armor > 0 {
			dmg *= .9
		}
		r.nadeDamageLocked(v, f.owner, "molly", dmg, now)
	}
}

func (r *room) explodeHELocked(n *liveNade, now time.Time) {
	gm := r.gm()
	c := vec3{n.p.X, n.p.Y + .25, n.p.Z}
	dmgAt := func(q vec3) float64 {
		d := q.sub(c).len()
		if d >= heR || !gm.lineOfSight(c, q) {
			return 0
		}
		return 98 * math.Pow(1-d/heR, 1.2)
	}
	for _, v := range r.clients {
		p := v.ps
		if !p.alive || !p.posOK || !r.canHurt(n.owner, n.ownerSq, v) {
			continue
		}
		var dmg float64
		if v.bot != nil {
			dmg = dmgAt(vec3{p.pos.X, p.pos.Y + 1.1 - v.bot.cr*.4, p.pos.Z})
		} else {
			cr := lastCrouch(p)
			body := .7
			if cr <= .5 {
				body = 1.05
			}
			dmg = math.Max(dmgAt(vec3{p.pos.X, p.pos.Y + body, p.pos.Z}), dmgAt(vec3{p.pos.X, p.pos.Y + 1.62 - cr*.48, p.pos.Z}))
		}
		if dmg <= 1 {
			continue
		}
		if p.armor > 0 {
			a := dmg * .43
			p.armor = math.Max(0, p.armor-a*.5)
			dmg -= a
		}
		r.nadeDamageLocked(v, n.owner, "he", dmg, now)
	}
}

func lastCrouch(p *pstate) float64 {
	if p.hn == 0 {
		return 0
	}
	return p.hist[(p.hn-1)%len(p.hist)].cr
}

// nadeDamageLocked: granata yoki olov zarari (egasi chiqib ketgan bo'lishi mumkin).
func (r *room) nadeDamageLocked(v *client, owner, w string, dmg float64, now time.Time) {
	p := v.ps
	killer := r.clients[owner]
	if killer != nil && killer != v {
		r.noteDamageLocked(killer, math.Min(dmg, math.Max(0, p.hp)))
	}
	p.hp -= dmg
	if p.hp > 0 {
		if v.bot != nil {
			v.bot.alertAt = now
		}
		return
	}
	by := owner
	if killer == nil || killer == v {
		killer, by = nil, ""
	}
	if v.bot != nil {
		r.botDieLocked(v, killer, w, false, now)
		return
	}
	if p.dueAt.IsZero() {
		p.dueAt, p.dueBy, p.dueW, p.dueHS = now, by, w, false
	}
}

// bombBlastLocked: bomba portladi — 25 m gacha hamma zarar ko'radi.
func (r *room) bombBlastLocked(b *bombState, now time.Time) {
	at := vec3{b.X, 0, b.Z}
	for _, v := range r.clients {
		p := v.ps
		if !p.alive || !p.posOK {
			continue
		}
		d := p.pos.sub(at).len()
		if d >= 25 {
			continue
		}
		dmg := 520 * math.Pow(1-d/25, 2.3)
		if p.armor > 0 {
			dmg *= .78
		}
		r.nadeDamageLocked(v, "", "bomb", dmg, now)
	}
}

// clearNadesLocked: yangi raund yoki o'yin — granatalar va olovlar yo'qoladi.
func (r *room) clearNadesLocked() { r.nades, r.smokes, r.fires = nil, nil, nil }
