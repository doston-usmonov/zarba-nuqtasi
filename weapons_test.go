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
