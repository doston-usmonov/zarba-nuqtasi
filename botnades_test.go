package main

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// TestBotLaunchMatchesLineups: server botining otishi brauzerdagi tayyor tutun joylari bilan aynan bir xil tushadi.
func TestBotLaunchMatchesLineups(t *testing.T) {
	n := 0
	for _, d := range mapDefs {
		m := gameMaps[d.id]
		for _, l := range m.lineups {
			pos, vel := botLaunch(vec3{l.x, 0, l.z}, l.yaw, l.pitch, false)
			res := m.nadeSim("smoke", pos, vel)
			if e := math.Hypot(res.p.X-l.land[0], res.p.Z-l.land[1]); e > .3 {
				t.Errorf("%s %s: tutun %.2f m chetga tushdi (%v, kerak %v)", d.id, l.site, e, res.p, l.land)
			}
			n++
		}
	}
	if n < 15 {
		t.Fatalf("tayyor joylar: %d", n)
	}
}

// TestBotAimNade: topilgan otish nishonga yaqin tushadi.
func TestBotAimNade(t *testing.T) {
	m := gameMaps["qumtepa"]
	rng := rand.New(rand.NewPCG(1, 2))
	found := 0
	for i := 0; i < 60; i++ {
		c := m.open[rng.IntN(len(m.open))]
		at := cellCenter(c)
		a := rng.Float64() * 2 * math.Pi
		target := vec3{at.X + math.Sin(a)*(8+rng.Float64()*12), 0, at.Z + math.Cos(a)*(8+rng.Float64()*12)}
		for _, kind := range []string{"he", "molly", "flash"} {
			aim, ok := m.botAimNade(at, kind, target)
			if !ok {
				continue
			}
			found++
			pos, vel := botLaunch(at, aim.yaw, aim.pitch, aim.soft)
			res := m.nadeSim(kind, pos, vel)
			lim := 3.2
			if kind == "flash" {
				lim = 5
			}
			if e := math.Hypot(res.p.X-target.X, res.p.Z-target.Z); e > lim || (kind == "molly" && !res.ground) {
				t.Fatalf("%s: %.2f m (%v → %v)", kind, e, target, res.p)
			}
		}
	}
	if found < 40 {
		t.Fatalf("juda kam otish topildi: %d", found)
	}
}

// TestBotNadesInRoom: hujumchi bot tayyor joydan tutun otadi (serverda tutun paydo bo'ladi, holatida "gn" boradi),
// raqibni yo'qotgan bot o'sha joyga granata otadi.
func TestBotNadesInRoom(t *testing.T) {
	r, a, _ := newRefRoom()
	delete(r.clients, "b")
	r.owner = a.id
	r.bans = map[string]time.Time{}
	r.gs.M = "qumtepa"
	h := &hub{maxPlayers: 64}
	h.adminLocked(r, a, json.RawMessage(`{"a":"bot","sq":"A","lv":"hard"}`), time.Now())
	var bot *client
	for _, c := range r.clients {
		if c.bot != nil {
			bot = c
		}
	}
	refPres(r, a, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 0, "al": 1, "x": 43, "y": 0, "z": 10})
	now := time.Now()
	// 2-raund, hujum: botni tutunchi qilamiz
	r.gs.E, r.gs.R, r.gs.Fm, r.gs.Ph, r.gs.T = "e1", 2, 12, "freeze", 115
	bot.info.Tm = "T"
	var got bool
	for k := 0; k < 40 && !got; k++ {
		r.spawnBotLocked(bot, now)
		got = bot.bot.lu != nil
	}
	if !got {
		t.Fatal("bot tutunchi bo'lmadi")
	}
	lu := *bot.bot.lu
	bot.bot.gren = []string{"smoke"} // faqat tutun (tasodifiy ko'r qiluvchi tekshiruvga aralashmasin)
	bot.bot.execAt = now.Add(20 * time.Second)
	r.gs.Ph = "live"
	a.ps.pos, a.ps.alive = vec3{3, 0, 3}, true // bot ko'rmaydigan joyda
	var smoked bool
	for i := 0; i < 1200 && !smoked; i++ {
		now = now.Add(50 * time.Millisecond)
		r.gs.T = 110
		a.ps.pos = vec3{3, 0, 3}
		r.botStepLocked(bot, now, .05)
		r.nadesTickLocked(now)
		smoked = len(r.smokes) > 0
	}
	if !smoked || bot.bot.gn == nil || bot.bot.gn["k"] != "smoke" {
		t.Fatalf("tutun otilmadi: mode=%s pos=%v lu=%v gn=%v", bot.bot.mode, bot.ps.pos, lu, bot.bot.gn)
	}
	if s := r.smokes[0].p; math.Hypot(s.X-lu.land[0], s.Z-lu.land[1]) > .3 {
		t.Fatalf("tutun %v ga tushdi, kerak %v", s, lu.land)
	}
	var pres map[string]any
	json.Unmarshal(bot.pres, &pres)
	if gn, _ := pres["gn"].(map[string]any); gn == nil || gn["k"] != "smoke" || gn["n"].(float64) < 1 {
		t.Fatalf("bot holatida granata yo'q: %v", pres["gn"])
	}
	// raqibni yo'qotdi — o'sha joyga portlovchi
	bot.bot.gren = []string{"he"}
	bot.bot.nadeAt = time.Time{}
	bot.bot.mode, bot.bot.actT = "go", 0 // bomba qo'ymayapti
	var target vec3
	found := false
	for k := 0; k < 2000 && !found; k++ {
		c := gameMaps["qumtepa"].open[rand.IntN(len(gameMaps["qumtepa"].open))]
		target = cellCenter(c)
		if d := bot.ps.pos.flat(target); d > 8 && d < 20 {
			_, found = gameMaps["qumtepa"].botAimNade(bot.ps.pos, "he", target)
		}
	}
	if !found {
		t.Skip("bu joydan portlovchi uchun nishon topilmadi")
	}
	bot.bot.lastKnown, bot.bot.knownAt = target, now.Add(-2*time.Second)
	n0 := len(r.nades)
	r.botNadeLocked(bot, now)
	if len(r.nades) != n0+1 || bot.bot.gn["k"] != "he" || len(bot.bot.gren) != 0 {
		t.Fatalf("portlovchi otilmadi: %v %v", bot.bot.gn, bot.bot.gren)
	}
	// aim va deathmatch'da granata yo'q
	r.gs.Md = modeDM
	r.spawnBotLocked(bot, now)
	if len(bot.bot.gren) != 0 {
		t.Fatalf("deathmatch'da granata: %v", bot.bot.gren)
	}
}
