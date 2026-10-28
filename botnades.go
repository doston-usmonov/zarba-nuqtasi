package main

// Server botlarining granatalari (rasmiy xonalar). Brauzerdagi botlar bilan bir xil fikr:
// hujumchi tayyor tutun joyidan (lineup) o'z nuqtasiga tutun otadi, nuqtaga kirishdan oldin ko'r
// qiluvchi otadi; raqibni hozirgina ko'rgan bo'lsa — o'sha joyga portlovchi (himoyachi — yondiruvchi).
// Otish o'yinchinikiday hisoblanadi (nadeLaunch → nadeSim): bot kerakli joyga tushadigan burchakni
// qidiradi. Granata bot holatida "gn" bo'lib boradi — brauzerlar uni o'zlari uchiradi, server esa
// portlovchi, tutun va olovni o'zi hisoblaydi (throwNadeLocked).

import (
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

var botNadeOdds = map[string]map[string]float64{
	"T":  {"flash": .6, "he": .3},
	"CT": {"molly": .4, "he": .3, "flash": .3},
}

// jsRound: brauzerdagi Math.round(v*100)/100 (yarimlar yuqoriga) — ikkala tomon bir xil son bilan hisoblaydi.
func jsRound(v float64) float64 { return math.Floor(v*100+.5) / 100 }

// botLaunch: joyida turgan bot otgan granata qayerdan va qanday tezlikda uchadi (brauzerdagi nadeLaunch).
func botLaunch(at vec3, yaw, pitch float64, soft bool) (pos, vel vec3) {
	pt := math.Min(1.45, pitch+.1)
	d := vec3{-math.Sin(yaw) * math.Cos(pt), math.Sin(pt), -math.Cos(yaw) * math.Cos(pt)}
	pos = vec3{at.X, at.Y + 1.62, at.Z}.add(d.scale(.25))
	pos.Y -= .1
	sp := 17.5
	if soft {
		sp = 7.5
	}
	vel = d.scale(sp)
	return vec3{jsRound(pos.X), jsRound(pos.Y), jsRound(pos.Z)}, vec3{jsRound(vel.X), jsRound(vel.Y), jsRound(vel.Z)}
}

type nadeAim struct {
	yaw, pitch float64
	soft       bool
	err        float64
}

// botAimNade: target ga eng yaqin tushadigan otish (ko'r qiluvchi — biroz tepasida portlaydi); topilmasa ok=false.
func (m *gameMap) botAimNade(at vec3, kind string, target vec3) (nadeAim, bool) {
	yaw := yawTo(at, target)
	gy := m.surfaceY(target.X, target.Z) + 1.8
	best, found := nadeAim{}, false
	try := func(pitch float64, soft bool) {
		pos, vel := botLaunch(at, yaw, pitch, soft)
		res := m.nadeSim(kind, pos, vel)
		if kind == "molly" && !res.ground {
			return // havoda yonib ketadi
		}
		e := math.Hypot(res.p.X-target.X, res.p.Z-target.Z)
		if kind == "flash" {
			e += math.Abs(res.p.Y-gy) * .5
		}
		if !found || e < best.err {
			best, found = nadeAim{yaw, pitch, soft, e}, true
		}
	}
	for _, soft := range []bool{false, true} {
		for pitch := -.15; pitch < 1.2; pitch += .1 {
			try(pitch, soft)
		}
	}
	if found {
		p0, sf := best.pitch, best.soft
		for _, dp := range []float64{-.06, -.03, .03, .06} {
			try(p0+dp, sf)
		}
	}
	limit := 3.2
	if kind == "flash" {
		limit = 5
	}
	return best, found && best.err < limit
}

// botGiveNadesLocked: raund boshida botga granatalar (raundli rejimlarda).
func (r *room) botGiveNadesLocked(c *client, rih int, now time.Time) {
	b, g := c.bot, r.gs
	b.gren, b.lu, b.flashed = nil, nil, false
	b.nadeAt = now.Add(time.Duration(3000+rand.IntN(4000)) * time.Millisecond)
	if g.respawns() || g.Md == modeAim || g.Ph == "warmup" || g.R <= 0 {
		return
	}
	k := 1.0
	if rih <= 1 {
		k = .4
	}
	switch b.lv {
	case "easy":
		k *= .5
	case "hard":
		k *= 1.25
	}
	for _, kind := range []string{"smoke", "flash", "he", "molly"} {
		if rand.Float64() < botNadeOdds[c.info.Tm][kind]*k {
			b.gren = append(b.gren, kind)
		}
	}
	// hujumchi: ba'zan o'z nuqtasining tayyor tutun joyiga borib, jamoa kirganda tutun otadi
	if c.info.Tm == "T" && g.Md != modeRT && rand.Float64() < .35*k {
		for i := range r.gm().lineups {
			if l := &r.gm().lineups[i]; l.site == b.site {
				b.lu, b.mode = l, "lu"
				b.gren = append(b.gren, "smoke")
				break
			}
		}
	}
}

// botLineupLocked: tutunchi bot tayyor joyga boradi va jamoa kirganda otadi. true — hali shu ish bilan band.
func (r *room) botLineupLocked(c *client, now time.Time, dt float64) bool {
	b, p, g := c.bot, c.ps, r.gs
	if b.mode != "lu" || b.lu == nil {
		return false
	}
	spot := vec3{b.lu.x, 0, b.lu.z}
	d := p.pos.flat(spot)
	switch {
	case d > .6:
		r.botGo(c, spot, 4.1, dt, now)
	case d > .03:
		st := math.Min(d, 2.2*dt)
		p.pos.X += (spot.X - p.pos.X) / d * st
		p.pos.Z += (spot.Z - p.pos.Z) / d * st
		b.moving = true
	default:
		p.pos.X, p.pos.Z = spot.X, spot.Z
		b.turn(b.lu.yaw, 4, dt)
	}
	there := p.pos.flat(spot) < .05
	if now.After(b.execAt) || (g.Ph == "live" && g.T < 50) {
		if there || d > 22 || g.T < 40 {
			if there && g.Ph == "live" {
				r.botThrowLocked(c, "smoke", nadeAim{yaw: b.lu.yaw, pitch: b.lu.pitch}, now)
			}
			b.mode, b.path = "go", nil
		}
	}
	return true
}

func (r *room) botThrowLocked(c *client, kind string, a nadeAim, now time.Time) bool {
	b, p := c.bot, c.ps
	i := slices.Index(b.gren, kind)
	if i < 0 {
		return false
	}
	b.gren = slices.Delete(b.gren, i, i+1)
	pos, vel := botLaunch(p.pos, a.yaw, a.pitch, a.soft)
	b.nadeN++
	b.gn = map[string]any{"n": b.nadeN, "k": kind, "x": pos.X, "y": pos.Y, "z": pos.Z, "vx": vel.X, "vy": vel.Y, "vz": vel.Z}
	b.yaw = a.yaw
	if t := now.Add(500 * time.Millisecond); b.fireAt.Before(t) {
		b.fireAt = t
	}
	b.nadeAt = now.Add(time.Duration(4000+rand.IntN(3000)) * time.Millisecond)
	r.throwNadeLocked(c, kind, pos, vel, now)
	return true
}

// botNadeLocked: raqibni ko'rmay turganda granata otish kerakmi.
func (r *room) botNadeLocked(c *client, now time.Time) {
	b, p, g := c.bot, c.ps, r.gs
	if len(b.gren) == 0 || now.Before(b.nadeAt) || g.Ph != "live" || g.respawns() || g.Md == modeAim || b.actT > 0 || b.mode == "lu" {
		return
	}
	gm := r.gm()
	has := func(k string) bool { return slices.Contains(b.gren, k) }
	try := func(kind string, at vec3, cd time.Duration) bool {
		b.nadeAt = now.Add(cd)
		if kind == "flash" && r.mateNearLocked(c, at, 9) {
			return false // jamoadoshni ko'r qilmaslik uchun
		}
		a, ok := gm.botAimNade(p.pos, kind, at)
		return ok && r.botThrowLocked(c, kind, a, now)
	}
	// hujumchi: nuqtaga kirishdan oldin ko'r qiluvchi
	if c.info.Tm == "T" && b.mode == "go" && g.B == nil && has("flash") && !b.flashed {
		if d := p.pos.flat(b.goal); d > 6 && d < 16 {
			b.flashed = true
			if try("flash", b.goal, 1200*time.Millisecond) {
				return
			}
		}
	}
	// raqib hozirgina shu yerda edi
	if !b.knownAt.IsZero() && now.Sub(b.knownAt) > 800*time.Millisecond && now.Sub(b.knownAt) < 6*time.Second {
		if d := p.pos.flat(b.lastKnown); d > 6 && d < 24 {
			kind := ""
			switch {
			case c.info.Tm == "CT" && has("molly"):
				kind = "molly"
			case has("he"):
				kind = "he"
			case has("flash") && d < 18:
				kind = "flash"
			case has("molly"):
				kind = "molly"
			}
			if kind != "" {
				try(kind, b.lastKnown, 1500*time.Millisecond)
			}
		}
	}
}

// mateNearLocked: c ning tirik jamoadoshi at ga shuncha yaqinmi.
func (r *room) mateNearLocked(c *client, at vec3, dist float64) bool {
	for _, o := range r.clients {
		if o != c && o.ps.alive && o.info.Sq != "" && o.info.Sq == c.info.Sq && o.ps.pos.flat(at) < dist {
			return true
		}
	}
	return false
}
