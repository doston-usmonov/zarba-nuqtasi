package main

// Graffiti: o'yinchi o'zi chizgan 16×16 rasmni devorga chizadi (T tugmasi).
//
// Mijoz holatida bir necha soniya "sp": {n, x, y, z, nx, nz, p} turadi: n — tartib raqami, (x,y,z) — devordagi
// nuqta, (nx,nz) — devordan tashqariga qaragan yo'nalish, p — 256 ta belgi (har katak palitradagi rang: 0 —
// shaffof, 1..c). Server tekshiradi: tirik o'yinchi, o'zidan 5 m ichida, haqiqatan devor yuzasida, 10 soniyada
// bir martadan ko'p emas. Mos kelmasa maydon boshqalarga yuborilmaydi.

import (
	"encoding/json"
	"math"
	"regexp"
	"time"
)

const sprayEvery = 10 * time.Second

var sprayRe = regexp.MustCompile(`^[0-9a-c]{256}$`)

type sprayClaim struct {
	N  int     `json:"n"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"`
	NX float64 `json:"nx"`
	NZ float64 `json:"nz"`
	P  string  `json:"p"`
}

func (r *room) sprayLocked(c *client, f map[string]json.RawMessage, now time.Time) {
	raw, ok := f["sp"]
	if !ok || string(raw) == "null" {
		return
	}
	var s sprayClaim
	p := c.ps
	if json.Unmarshal(raw, &s) != nil || p == nil {
		delete(f, "sp")
		return
	}
	if s.N == p.spN && s.N > 0 { // allaqachon qabul qilingan, holatda hali turibdi
		f["sp"] = p.spJSON
		return
	}
	if s.N <= p.spN || !p.alive || !p.posOK || !sprayRe.MatchString(s.P) || now.Sub(p.spAt) < sprayEvery ||
		math.Abs(math.Hypot(s.NX, s.NZ)-1) > .05 || math.Abs(s.NX*s.NZ) > .01 || // faqat to'g'ri devorlar
		math.Hypot(s.X-p.pos.X, s.Z-p.pos.Z) > 5 || s.Y < .5 || s.Y > 6.5 {
		delete(f, "sp")
		return
	}
	// nuqtaning orqasida devor (yoki baland quti) bo'lishi kerak, oldida — bo'sh joy
	m := r.gm()
	back := m.cellH(wc(s.X-s.NX*.3), wc(s.Z-s.NZ*.3))
	front := m.cellH(wc(s.X+s.NX*.3), wc(s.Z+s.NZ*.3))
	if back < s.Y+.6 || front > s.Y-.6 {
		delete(f, "sp")
		return
	}
	p.spN, p.spAt = s.N, now
	p.spJSON = jsonMsg(s)
	f["sp"] = p.spJSON
}
