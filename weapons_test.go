package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestNewWeaponsMatchClient: yangi qurollar brauzerdagi W jadvali bilan bir xil (zarar, tezlik, o'qlar).
func TestNewWeaponsMatchClient(t *testing.T) {
	html := readIndex(t)
	for _, id := range []string{"mpistol", "shotgun", "lmg", "scout"} {
		spec, ok := weaponSpecs[id]
		it, ok2 := shop[id]
		if !ok || !ok2 || weaponMag[id] == 0 || weaponFall[id] == 0 || weaponAcc[id] == 0 {
			t.Fatalf("%s: serverda to'liq emas", id)
		}
		loc := regexp.MustCompile(`\n  ` + id + `:\s*\{ name:[^\n]*`).FindStringIndex(html)
		if loc == nil {
			t.Fatalf("%s brauzerda yo'q", id)
		}
		line := html[loc[0]:loc[1]]
		for _, want := range []string{"dmg:" + ftoa(spec.dmg), "rate:" + ftoa(spec.rate), "price:" + itoa(it.price), "mag:" + itoa(weaponMag[id])} {
			if !strings.Contains(line, want) {
				t.Fatalf("%s: brauzerda %q yo'q: %s", id, want, line)
			}
		}
	}
}

func ftoa(v float64) string { return strings.TrimPrefix(strconv.FormatFloat(v, 'f', -1, 64), "0") }

func readIndex(t *testing.T) string {
	b, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestShotgunClaims: drobovik tegishi bitta da'vo — chegara saochmalar soniga qarab.
func TestShotgunClaims(t *testing.T) {
	r, x, y := newRefRoom()
	r.gs.Md, r.gs.Ph, r.gs.R, r.gs.T, r.gs.Sc = modeDM, "live", 1, 500, map[string]int{}
	p := func(c *client, sq string, z float64, extra map[string]any) {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "shotgun"}
		for k, v := range extra {
			d[k] = v
		}
		refPres(r, c, d)
	}
	p(x, "A", 30, nil)
	p(y, "B", 34, nil)
	r.mu.Lock()
	x.ps.spawnAt, y.ps.spawnAt = time.Time{}, time.Time{}
	x.ps.pos, y.ps.pos = vec3{43, 0, 30}, vec3{43, 0, 34}
	for range 3 {
		x.ps.push(sample{t: time.Now(), p: x.ps.pos})
		y.ps.push(sample{t: time.Now(), p: y.ps.pos})
	}
	now := time.Now()
	if why := r.validHitLocked(x, hitClaim{I: 1, T: y.id, D: 150, Pn: .6, P: "c", W: "shotgun", Pc: 8}, now); why != "" {
		t.Fatalf("8 saochma 150 zarar rad etildi: %s", why)
	}
	if why := r.validHitLocked(x, hitClaim{I: 2, T: y.id, D: 150, Pn: .6, P: "c", W: "shotgun", Pc: 1}, now); why == "" {
		t.Fatal("bitta saochma 150 zarar qabul qilindi")
	}
	if why := r.validHitLocked(x, hitClaim{I: 3, T: y.id, D: 700, Pn: .6, P: "h", W: "shotgun", Pc: 20}, now); why == "" {
		t.Fatal("saochmalar soni cheklanmadi")
	}
	r.mu.Unlock()
}

// TestTaserAndDamageModel: elektroshoker (yaqin, bir martalik, zirhdan o'tadi), qo'llar zonasi, masofa birligi.
func TestTaserAndDamageModel(t *testing.T) {
	html := readIndex(t)
	line := regexp.MustCompile(`\n  taser:\s*\{ name:[^\n]*`).FindString(html)
	if line == "" || !strings.Contains(line, "dmg:"+ftoa(weaponSpecs["taser"].dmg)) || !strings.Contains(line, "price:"+itoa(shop["taser"].price)) || !strings.Contains(line, "range:4.6") {
		t.Fatalf("brauzerdagi taser: %q", line)
	}
	if !strings.Contains(html, "const RANGE_U = "+ftoa(rangeUnit)+";") || !strings.Contains(html, "arms:1") || !strings.Contains(html, "const base = player.zombie ? ZM.claw : behind ? 180 :") {
		t.Fatal("brauzer: masofa birligi, qo'llar zonasi yoki orqadan pichoq server bilan mos emas")
	}
	if !strings.Contains(html, "const ZM = { hp:"+ftoa(zmHP)+", claw:"+ftoa(zmClaw)+", netRound:"+ftoa(zmRoundTime)+",") {
		t.Fatal("brauzer: zombining joni, tirnog'i yoki raund vaqti server bilan mos emas (zombie.go)")
	}

	r, x, y := newRefRoom()
	r.gs.Md, r.gs.Ph, r.gs.R, r.gs.T, r.gs.Sc = modeDM, "live", 1, 500, map[string]int{}
	place := func(zx, zy float64) {
		for _, c := range []*client{x, y} {
			z, sq := zx, "A"
			if c == y {
				z, sq = zy, "B"
			}
			refPres(r, c, map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "taser"})
		}
		r.mu.Lock()
		x.ps.spawnAt, y.ps.spawnAt = time.Time{}, time.Time{}
		x.ps.pos, y.ps.pos = vec3{43, 0, zx}, vec3{43, 0, zy}
		x.ps.hn, y.ps.hn = 0, 0
		for range 3 {
			x.ps.push(sample{t: time.Now(), p: x.ps.pos})
			y.ps.push(sample{t: time.Now(), p: y.ps.pos})
		}
		r.mu.Unlock()
	}
	place(30, 34)
	r.mu.Lock()
	now := time.Now()
	if why := r.validHitLocked(x, hitClaim{I: 1, T: y.id, D: 500, Pn: 1, P: "c", W: "taser"}, now); why != "" {
		t.Fatalf("4 m dan elektroshoker rad etildi: %s", why)
	}
	if why := r.validHitLocked(x, hitClaim{I: 2, T: y.id, D: 36, Pn: .775, P: "a", W: "rifle"}, now); why != "" {
		t.Fatalf("qo'lga tekkan o'q rad etildi: %s", why)
	}
	if why := r.validHitLocked(x, hitClaim{I: 3, T: y.id, D: 60, Pn: .775, P: "a", W: "rifle"}, now); why == "" {
		t.Fatal("qo'lga 60 zarar qabul qilindi (chegara 36)")
	}
	// zirhli nishon ham bitta urishda o'ladi
	y.ps.armor, y.ps.helmet, y.ps.hp = 100, true, 100
	r.applyHitLocked(x, hitClaim{I: 1, T: y.id, D: 500, Pn: 1, P: "c", W: "taser"}, now)
	if y.ps.hp > 0 {
		t.Fatalf("elektroshokerdan keyin jon %v", y.ps.hp)
	}
	r.mu.Unlock()
	place(30, 39) // 9 m — yetmaydi
	r.mu.Lock()
	y.ps.alive = true
	if why := r.validHitLocked(x, hitClaim{I: 4, T: y.id, D: 500, Pn: 1, P: "c", W: "taser"}, time.Now()); why != "uchqun uzoqdan" {
		t.Fatalf("9 m: %q", why)
	}
	r.mu.Unlock()

	// do'kon va bir martalik zaryad
	iv := newInventory()
	now = time.Now()
	if iv.has("taser", now) {
		t.Fatal("boshida elektroshoker bo'lmasligi kerak")
	}
	iv.taser = true
	if !iv.has("taser", now) || !iv.remove("taser", now) || iv.taser {
		t.Fatal("elektroshoker olish/ishlatish")
	}
	if iv.has("taser", now.Add(3*time.Second)) {
		t.Fatal("ishlatilgan elektroshoker qayta otmasligi kerak")
	}
	if iv.sec != "pistol" || iv.remove("", now) {
		t.Fatal("bo'sh qurol nomi hech narsani olib tashlamasligi kerak")
	}
	if killReward("taser") != 100 {
		t.Fatalf("mukofot %d", killReward("taser"))
	}
}
