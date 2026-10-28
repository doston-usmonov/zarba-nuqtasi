package main

import (
	"context"
	"testing"
	"time"
)

// TestRoomStats: qurol bo'yicha o'ldirish va zarar, raunddagi eng ko'p o'ldirish, «1 ga N», bomba — o'yin oxiridagi hisoblagichlarda.
func TestRoomStats(t *testing.T) {
	r, a, b := newRefRoom()
	mk := func(id string) *client {
		c := &client{id: id, room: r, send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
		r.clients[id] = c
		return c
	}
	b2, b3 := mk("b2"), mk("b3")
	*r.gs = gameState{R: 1, Ph: "live", Md: modeComp, Fm: 12, E: "e1", Bm: -1, T: 60, M: "qumtepa"}
	a.acct = &account{id: 1, name: "Ali"}
	a.info.Sq, a.info.Tm = "A", "T"
	for _, c := range []*client{b, b2, b3} {
		c.info.Sq, c.info.Tm = "B", "CT"
	}
	for _, c := range r.clients {
		c.ps.alive = true
	}
	// A yolg'iz, qarshisida 3 kishi — hammasini o'ldiradi (1 ga 3), biri boshga
	r.clutchCheckLocked()
	r.noteDamageLocked(a, 100, "rifle")
	b.ps.alive = false
	r.noteDeathLocked(b, a, true, "rifle")
	b2.ps.alive = false
	r.noteDeathLocked(b2, a, false, "knife")
	b3.ps.alive = false
	r.noteDeathLocked(b3, a, false, "he")
	r.noteBombLocked(a, false)
	r.endRoundLocked("T", "elim")
	s := r.st["a"]
	if s.k != 3 || s.wk["rifle"] != 1 || s.whs["rifle"] != 1 || s.wk["knife"] != 1 || s.wk["he"] != 1 || s.wdmg["rifle"] != 100 || s.bestRK != 3 || s.clutch != 3 || s.plant != 1 {
		t.Fatalf("statistika: %+v", s)
	}
	// o'yin tugadi: hisoblagichlar
	r.matchAt = time.Now().Add(-time.Hour)
	g := r.gainsLocked(time.Now())
	if len(g) != 1 {
		t.Fatalf("natijalar: %+v", g)
	}
	st := g[0].stats
	for k, want := range map[string]int{"k": 3, "wk:rifle": 1, "whs:rifle": 1, "wk:knife": 1, "wdmg:rifle": 100, "best_rk": 3, "best_mk": 3, "clutch": 3, "plant": 1, "mp:qumtepa": 1, "md:comp": 1, "matches": 1} {
		if st[k] != want {
			t.Fatalf("%s = %d, kerak %d (%v)", k, st[k], want, st)
		}
	}
	// yolg'iz qolib yutqazilgan raund hisobga olinmaydi
	r.gs.R, r.gs.Ph = 2, "live"
	for _, c := range r.clients {
		c.ps.alive = true
	}
	r.clutchCheckLocked()
	r.endRoundLocked("CT", "elim")
	if r.st["a"].clutch != 3 || r.st["a"].clR != 2 {
		t.Fatalf("yutqazilgan «1 ga N»: %+v", r.st["a"])
	}
}

// TestMedalsStore: hisoblagichlar qo'shiladi, eng yaxshilari saqlanadi, medal bir marta ochiladi va XP beradi.
func TestMedalsStore(t *testing.T) {
	s, ctx := testStore(t, ""), context.Background()
	u, _, err := s.Register(ctx, "Medalchi", "secret12")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	st := map[string]int{"k": 7, "hs": 2, "matches": 1, "best_rk": 5, "best_mk": 7, "wk:knife": 6, "wk:rifle": 1, "whs:rifle": 1, "wdmg:rifle": 300, "mp:qumtepa": 1, "mw:qumtepa": 1, "md:comp": 1, "mdw:comp": 1, "wins": 1}
	xp, _, medals, err := s.ApplyGain(ctx, matchGain{uid: u.ID, mode: modeComp, xp: 100, stats: st}, now)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range medals {
		got[m.ID] = true
	}
	if len(medals) != 2 || !got["birinchi"] || !got["beshlik"] || xp != 100+50+400+xpMissions(t, s, u.ID) {
		t.Fatalf("medallar: %v, xp %d", medals, xp)
	}
	// ikkinchi o'yin: pichoq 6+5=11 — «Pichoq ustasi»; best_rk kichikroq — saqlanmaydi
	st2 := map[string]int{"k": 5, "matches": 1, "best_rk": 2, "best_mk": 5, "wk:knife": 5, "mp:bozor": 1, "md:comp": 1}
	_, _, medals2, err := s.ApplyGain(ctx, matchGain{uid: u.ID, mode: modeComp, xp: 100, stats: st2}, now)
	if err != nil || len(medals2) != 1 || medals2[0].ID != "pichoq" {
		t.Fatalf("2-o'yin medallari: %v %v", medals2, err)
	}
	d, err := s.PlayerDetail(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Totals["k"] != 12 || d.Totals["matches"] != 2 || d.Totals["best_rk"] != 5 || d.Totals["best_mk"] != 7 {
		t.Fatalf("jami: %v", d.Totals)
	}
	if len(d.Weapons) != 2 || d.Weapons[0].W != "knife" || d.Weapons[0].K != 11 || d.Weapons[1].W != "rifle" || d.Weapons[1].HS != 1 || d.Weapons[1].Dmg != 300 {
		t.Fatalf("qurollar: %+v", d.Weapons)
	}
	if len(d.Maps) != 2 || d.Maps[0].Played != 1 || len(d.Modes) != 1 || d.Modes[0].Played != 2 || d.Modes[0].Won != 1 {
		t.Fatalf("xaritalar/rejimlar: %+v %+v", d.Maps, d.Modes)
	}
	open := 0
	for _, m := range d.Medals {
		if m.At > 0 {
			open++
		}
		if m.ID == "mingtalik" && m.Have != 12 {
			t.Fatalf("medal yo'li: %+v", m)
		}
	}
	if open != 3 || len(d.Medals) != len(medalDefs) {
		t.Fatalf("ochilgan medallar: %d", open)
	}
}

// xpMissions: shu o'yindan kunlik vazifalar bergan XP (vazifalar tasodifiy — metrics bo'sh, demak 0).
func xpMissions(t *testing.T, s *Store, uid int64) int64 { return 0 }
