package main

import (
	"testing"
	"time"
)

// TestAimPairing: aim xaritasi ↔ Aim rejimi doim birga.
func TestAimPairing(t *testing.T) {
	cases := []struct {
		q          map[string][]string
		gmap, mode string
	}{
		{map[string][]string{"mode": {"arena"}}, "maydon", "arena"},
		{map[string][]string{"map": {"hovli"}}, "hovli", "arena"},
		{map[string][]string{"map": {"chorraha"}, "mode": {"comp"}}, "chorraha", "arena"},
		{map[string][]string{"map": {"bozor"}, "mode": {"arena"}}, "bozor", "comp"},
		{map[string][]string{"map": {"bozor"}, "mode": {"dm"}}, "bozor", "dm"},
	}
	for _, c := range cases {
		o := parseRoomOpts(c.q)
		if o.gmap != c.gmap || o.mode != c.mode {
			t.Errorf("%v → %s/%s, kerak %s/%s", c.q, o.gmap, o.mode, c.gmap, c.mode)
		}
	}
	for _, id := range []string{"maydon", "chorraha", "hovli"} {
		if !isAimMap(id) || len(gameMaps[id].holds) < 4 {
			t.Errorf("%s: aim xaritasi to'liq emas", id)
		}
	}
	if isAimMap("qumtepa") {
		t.Error("qumtepa aim xaritasi emas")
	}
	r, a, _ := newRefRoom()
	r.gs.M, r.gs.Md = "qumtepa", modeComp
	now := time.Now()
	r.changeModeLocked(modeAim, now) // rejim aim → xarita ham aim
	if r.gs.M != defaultAimMap || r.gs.Md != modeAim {
		t.Fatalf("aim rejimiga o'tish: %s/%s", r.gs.M, r.gs.Md)
	}
	r.changeMapLocked("bekat", now) // oddiy xarita → Raqobat
	if r.gs.Md != modeComp {
		t.Fatalf("oddiy xaritada aim qoldi: %s", r.gs.Md)
	}
	r.changeMapLocked("hovli", now)
	if r.gs.Md != modeAim {
		t.Fatalf("aim xaritasida rejim: %s", r.gs.Md)
	}
	// ovoz faqat shu turdagi xaritalarga
	r.gs.Ph = "over"
	a.info.Sq = "A"
	r.voteLocked(a, []byte(`{"m":"qumtepa"}`))
	if a.vote != "" {
		t.Fatal("aim o'yinidan keyin oddiy xaritaga ovoz qabul qilindi")
	}
	r.voteLocked(a, []byte(`{"m":"chorraha"}`))
	if a.vote != "chorraha" {
		t.Fatal("aim xaritasiga ovoz qabul qilinmadi")
	}
	if l := parseOfficialRooms("maydon:comp:10,qumtepa:arena:10,hovli:arena:6"); len(l) != 1 || l[0].code() != "hovli-arena" {
		t.Fatalf("rasmiy aim xonalari: %+v", l)
	}
}

// TestAimRounds: botlar bilan aim o'yini — raundlar o'ynaladi, bomba yo'q, qurollar aim'niki, zirh bor.
func TestAimRounds(t *testing.T) {
	r := &room{name: "zn-aim", clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), manual: true}
	r.gs.M = "chorraha"
	r.gs.setMode(modeAim)
	if r.gs.roundTime() != 60 || r.gs.fixedFm() != 8 || r.gs.freezeTime(true, false) != 3 {
		t.Fatal("aim vaqtlari")
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for range 2 {
		r.addBotLocked("A", "hard", now)
		r.addBotLocked("B", "hard", now)
	}
	dt, rounds, sawLive, maxScore := .05, 0, false, 0
	aimW := map[string]bool{"sniper": true}
	for i := 0; i < 20*240; i++ {
		now = now.Add(50 * time.Millisecond)
		for _, c := range r.clients {
			r.botStepLocked(c, now, dt)
		}
		prev := r.gs.R
		r.tickLocked(now, dt)
		if r.gs.B != nil {
			t.Fatal("aim'da bomba paydo bo'ldi")
		}
		if r.gs.Ph == "live" && !sawLive {
			sawLive = true
			for _, c := range r.clients {
				if !aimW[c.bot.weapon] || c.ps.armor != 100 || !c.ps.helmet {
					t.Fatalf("aim boti: %s zirh %v dubulg'a %v", c.bot.weapon, c.ps.armor, c.ps.helmet)
				}
			}
		}
		if r.gs.R > prev && prev > 0 {
			rounds++
		}
		maxScore = max(maxScore, r.gs.SA+r.gs.SB)
	}
	if rounds < 2 || maxScore < 2 {
		t.Fatalf("4 daqiqada raundlar o'ynalmadi: %d raund, eng katta hisob %d, bosqich %s", rounds, maxScore, r.gs.Ph)
	}
	// vaqt tugasa: tirik qolganlar ko'p jamoa yutadi
	r.gs.Ph, r.gs.T, r.gs.R = "live", .01, 5
	alive := 0
	for _, c := range r.clients {
		c.info.In = 5
		c.ps.alive = c.info.Tm == "CT" || alive == 0
		if c.info.Tm == "T" && c.ps.alive {
			alive++
		}
		c.ps.hp = 100
	}
	sa, sb := r.gs.SA, r.gs.SB
	r.tickLocked(now, dt)
	if r.gs.W != "CT" || r.gs.Y != "time" || r.gs.SA+r.gs.SB != sa+sb+1 {
		t.Fatalf("vaqt tugaganda g'olib: %s %s", r.gs.W, r.gs.Y)
	}
}

// TestAimHits: Aim xonasida snayper o'qi qabul qilinadi (avval server qurolni «sizda yo'q» deb rad etardi),
// boshqa qurollar esa qabul qilinmaydi — faqat snayper va pichoq.
func TestAimHits(t *testing.T) {
	r, x, y := newRefRoom()
	*r.gs = gameState{R: 2, Ph: "live", Md: modeAim, Fm: 8, E: "a1", Bm: -1, T: 50, M: "maydon"}
	r.gs.liveAt = time.Now()
	pres := func(c *client, sq, tm string, z float64) {
		refPres(r, c, map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 2, "al": 1, "x": 31, "y": 0, "z": z, "w": "sniper", "ar": 100, "hm": 1})
	}
	pres(x, "A", "T", 40)
	pres(y, "B", "CT", 30)
	r.mu.Lock()
	defer r.mu.Unlock()
	x.ps.pos, y.ps.pos = vec3{31, 0, 40}, vec3{31, 0, 30}
	x.ps.alive, y.ps.alive = true, true
	for range 3 {
		x.ps.push(sample{t: time.Now(), p: x.ps.pos})
		y.ps.push(sample{t: time.Now(), p: y.ps.pos})
	}
	now := time.Now()
	if r.ecoOn() {
		t.Fatal("Aim rejimida iqtisod (va qurol inventari) bo'lmasligi kerak")
	}
	if why := r.validHitLocked(x, hitClaim{I: 1, T: y.id, D: 115, Pn: .97, P: "c", W: "sniper"}, now); why != "" {
		t.Fatalf("snayper o'qi rad etildi: %s", why)
	}
	if why := r.validHitLocked(x, hitClaim{I: 2, T: y.id, D: 36, Pn: .775, P: "c", W: "rifle"}, now); why == "" {
		t.Fatal("Aim'da avtomat o'qi qabul qilindi")
	}
	if why := r.validHitLocked(x, hitClaim{I: 3, T: y.id, D: 500, Pn: 1, P: "c", W: "taser"}, now); why == "" {
		t.Fatal("Aim'da elektroshoker qabul qilindi")
	}
	y.ps.hp = 100
	r.applyHitLocked(x, hitClaim{I: 1, T: y.id, D: 115, Pn: .97, P: "c", W: "sniper"}, now)
	if y.ps.hp > 0 {
		t.Fatalf("snayper ko'krakka: jon %v (zirh %v)", y.ps.hp, y.ps.armor)
	}
}
