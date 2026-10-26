package main

import (
	"testing"
	"time"
)

// TestStatsMVP: raund MVP va o'yin oxiridagi jadval faqat tasdiqlangan o'ldirishlardan.
func TestStatsMVP(t *testing.T) {
	r, a, b := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "live", Md: modeComp, Fm: 8, E: "s1", Bm: -1, T: 100, liveAt: time.Now()}
	pres := func(c *client, sq, tm string, z float64, extra map[string]any) {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "rifle"}
		for k, v := range extra {
			d[k] = v
		}
		refPres(r, c, d)
	}
	pres(a, "A", "T", 75, nil)
	pres(b, "B", "CT", 9, nil)
	r.mu.Lock()
	a.ps.pos, b.ps.pos = vec3{43, 0, 30}, vec3{43, 0, 34}
	a.ps.posOK, b.ps.posOK = true, true
	a.ps.inv.prim, b.ps.inv.prim = "rifle", "rifle"
	a.ps.push(sample{t: time.Now(), p: a.ps.pos})
	b.ps.push(sample{t: time.Now(), p: b.ps.pos})
	r.mu.Unlock()
	pres(a, "A", "T", 30, map[string]any{"hi": []map[string]any{{"i": 1, "t": "b", "d": 144, "pn": .7, "p": "h", "w": "rifle"}}})
	pres(b, "B", "CT", 34, map[string]any{"al": 0, "de": map[string]any{"n": 1, "by": "a", "w": "rifle", "hs": 1}})
	r.mu.Lock()
	r.tickLocked(time.Now(), .05)
	if r.gs.Ph != "post" || r.gs.Mv != "a" || r.gs.Mvr != "k1" {
		t.Fatalf("raund MVP: ph=%s mv=%q mvr=%q", r.gs.Ph, r.gs.Mv, r.gs.Mvr)
	}
	s := r.st["a"]
	if s == nil || s.k != 1 || s.hs != 1 || s.mvp != 1 || s.dmg < 99 || s.dmg > 101 || r.st["b"].d != 1 {
		t.Fatalf("statistika: %+v %+v", s, r.st["b"])
	}
	// o'yin tugadi — jadval
	r.gs.SA, r.gs.T = 9, 0
	r.tickLocked(time.Now(), .05)
	if r.gs.Ph != "over" || len(r.gs.St) != 2 || r.gs.St[0].ID != "a" || r.gs.St[0].Mvp != 1 || r.gs.St[0].Dmg != 100 {
		t.Fatalf("yakuniy jadval: %s %+v", r.gs.Ph, r.gs.St)
	}
	r.mu.Unlock()
}
