package main

import (
	"testing"
	"time"
)

func TestZombieMode(t *testing.T) {
	oldPrep, oldRound, oldPost, oldRounds := zmPrep, zmRoundTime, zmPost, zmRounds
	zmPrep, zmRoundTime, zmPost, zmRounds = 1, 6, 1, 2
	defer func() { zmPrep, zmRoundTime, zmPost, zmRounds = oldPrep, oldRound, oldPost, oldRounds }()
	srv, _, h := newTestServer(t)
	defer srv.Close()
	tc := newTestClient(srv.URL)
	w := dialWS(t, tc, "zn-zombi&create=1&mode=zm&map=bozor")
	defer w.c.Close()
	for i := 0; i < 5; i++ {
		w.send(map[string]any{"t": "adm", "d": map[string]any{"a": "bot", "sq": []string{"A", "B"}[i%2]}})
	}
	var rm *room
	for end := time.Now().Add(3 * time.Second); rm == nil && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		h.mu.Lock()
		rm = h.rooms["zn-zombi"]
		h.mu.Unlock()
	}
	if rm == nil {
		t.Fatal("xona yo'q")
	}
	type snap struct {
		ph, w         string
		r, zombies    int
		knife, sideOK bool
		hpOK          bool
		sc            int
	}
	look := func() snap {
		rm.mu.Lock()
		defer rm.mu.Unlock()
		g := rm.gs
		s := snap{ph: g.Ph, r: g.R, w: g.W, zombies: len(g.Zm), knife: true, sideOK: true, hpOK: true}
		for _, c := range rm.clients {
			if c.bot == nil {
				continue
			}
			if g.Zm[c.id] {
				if c.ps.alive && c.bot.weapon != "knife" {
					s.knife = false
				}
				if c.ps.alive && c.info.Tm != "T" {
					s.sideOK = false
				}
			} else if g.Ph == "live" && c.ps.alive && c.info.Tm != "CT" {
				s.sideOK = false
			}
		}
		for _, v := range g.Sc {
			s.sc += v
		}
		return s
	}
	wait := func(what string, sec float64, f func(snap) bool) snap {
		t.Helper()
		var s snap
		for end := time.Now().Add(time.Duration(sec * float64(time.Second))); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
			if s = look(); f(s) {
				return s
			}
		}
		t.Fatalf("%s: %+v", what, s)
		return s
	}
	if rm.gs.Md != modeZM {
		t.Fatalf("rejim: %s", rm.gs.Md)
	}
	wait("1-raund boshlandi (hali zombi yo'q)", 10, func(s snap) bool { return s.ph == "live" && s.r == 1 && s.zombies == 0 })
	s := wait("birinchi zombi", 3, func(s snap) bool { return s.zombies >= 1 })
	if !s.knife || !s.sideOK {
		t.Fatalf("zombi botlar pichoqda va Hujum rangida bo'lishi kerak: %+v", s)
	}
	s = wait("raund tugadi", 12, func(s snap) bool { return s.ph == "post" && s.r == 1 })
	if s.w != "T" && s.w != "CT" {
		t.Fatalf("raund g'olibi: %+v", s)
	}
	wait("2-raundda hamma yana odam", 4, func(s snap) bool { return s.ph == "live" && s.r == 2 && s.zombies == 0 && s.sideOK })
	s = wait("o'yin tugadi", 16, func(s snap) bool { return s.ph == "over" })
	if s.sc == 0 {
		t.Fatalf("ochkolar yozilmadi: %+v", s)
	}
	rm.mu.Lock()
	wn := rm.gs.Wnm
	rm.mu.Unlock()
	if wn == "" {
		t.Fatal("g'olib yo'q")
	}
}

func TestZombieReferee(t *testing.T) {
	r, a, b := newRefRoom()
	r.gs.setMode(modeZM)
	r.gs.Ph, r.gs.R = "live", 1
	a.info.Sq, b.info.Sq = "A", "A" // bir xil guruhda bo'lsa ham — zombi va odam raqib
	if r.foes(a, b) {
		t.Fatal("ikkala odam raqib bo'lmasligi kerak")
	}
	r.gs.Zm = map[string]bool{a.id: true}
	if !r.foes(a, b) || !r.gs.zombie(a.id) || r.gs.zombie(b.id) {
		t.Fatal("zombi va odam raqib")
	}
	if side := r.botSideLocked(a); side != "T" {
		t.Fatalf("zombi tomoni: %s", side)
	}
}
