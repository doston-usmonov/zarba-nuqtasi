package main

import (
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMapsMatchClient: serverdagi xaritalar brauzerdagi MAPS bilan aynan bir xil.
func TestMapsMatchClient(t *testing.T) {
	b, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i, j := strings.Index(src, "const MAPS = {"), strings.Index(src, "const MAP_CH")
	if i < 0 || j < i {
		t.Fatal("web/index.html: MAPS topilmadi")
	}
	blk := src[i:j]
	head := regexp.MustCompile(`\n  (\w+):\{ name:`)
	row := regexp.MustCompile(`'([#.cskABtk]{44})'`)
	locs := head.FindAllStringSubmatchIndex(blk, -1)
	if len(locs) != len(mapDefs) {
		t.Fatalf("brauzerda %d xarita, serverda %d", len(locs), len(mapDefs))
	}
	for n, l := range locs {
		id := blk[l[2]:l[3]]
		end := len(blk)
		if n+1 < len(locs) {
			end = locs[n+1][0]
		}
		var rows []string
		for _, m := range row.FindAllStringSubmatch(blk[l[1]:end], -1) {
			rows = append(rows, m[1])
		}
		gm := gameMaps[id]
		if gm == nil {
			t.Fatalf("serverda %q xaritasi yo'q", id)
		}
		if strings.Join(rows, "\n") != strings.Join(gm.rows, "\n") {
			t.Fatalf("%s: brauzer va server xaritasi farq qiladi — mapsdata.go ni qayta yasang", id)
		}
	}
}

// TestMapsPlayable: har xaritada bazalar, nuqtalar bor va botlar hamma joyga yetib boradi.
func TestMapsPlayable(t *testing.T) {
	for id, gm := range gameMaps {
		if len(gm.grid) != gridW*gridH || len(gm.rows) != gridH {
			t.Fatalf("%s: o'lcham xato", id)
		}
		if !gm.inSpawn(gm.spawnCenter("T"), "T") || !gm.inSpawn(gm.spawnCenter("CT"), "CT") {
			t.Fatalf("%s: baza markazi bazada emas", id)
		}
		var goals []vec3
		for _, h := range gm.holds {
			goals = append(goals, cellCenter(h[0]))
		}
		for _, s := range []string{"A", "B"} {
			for _, c := range gm.spots[s] {
				p := cellCenter(c)
				if gm.siteAt(p.X, p.Z) != s {
					t.Fatalf("%s: %v nuqta %s ichida emas", id, c, s)
				}
				goals = append(goals, p)
			}
		}
		for _, from := range []vec3{gm.spawnCenter("T"), gm.spawnCenter("CT")} {
			for _, g := range goals {
				if gm.astar(from, g) == nil {
					t.Fatalf("%s: %v dan %v ga yo'l yo'q", id, from, g)
				}
			}
		}
	}
}

func TestRoomMap(t *testing.T) {
	o := parseRoomOpts(map[string][]string{"create": {"1"}, "map": {"bozor"}})
	if o.gmap != "bozor" {
		t.Fatalf("map=%q", o.gmap)
	}
	if parseRoomOpts(map[string][]string{"map": {"yo'q"}}).gmap != defaultMap {
		t.Fatal("noma'lum xarita standartga tushishi kerak")
	}
	r := &room{name: "zn-x", clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), manual: true}
	r.gs.Ph, r.gs.R, r.gs.SA = "live", 5, 3
	r.changeMapLocked("karvon", time.Now())
	if r.gs.M != "karvon" || r.gs.Ph != "warmup" || r.gs.R != 0 || r.gs.SA != 0 || r.gm().name != "Karvonsaroy" {
		t.Fatalf("xarita almashmadi: %+v", r.gs)
	}
	r.gs.Ph = "over"
	r.gs.reset()
	if r.gs.M != "karvon" {
		t.Fatal("reset xaritani saqlashi kerak")
	}
}

// TestModes: deathmatch (har kim har kimga) va qurol poygasi qoidalari.
func TestModes(t *testing.T) {
	r, a, b := newRefRoom()
	r.gs.Md = modeDM
	// ikkalasi bir jamoada (A), lekin deathmatch'da baribir raqib
	pres := func(c *client, x, z float64, extra map[string]any) map[string]json.RawMessage {
		d := map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": x, "y": 0, "z": z, "w": "rifle"}
		for k, v := range extra {
			d[k] = v
		}
		return refPres(r, c, d)
	}
	r.mu.Lock()
	r.tickLocked(time.Now(), .05)
	if r.gs.Ph != "warmup" {
		t.Fatal("bitta jamoada ikki kishi: deathmatch isinishda boshlanishi kerak")
	}
	r.mu.Unlock()
	pres(a, 43, 75, nil)
	pres(b, 43, 70, nil)
	r.mu.Lock()
	for i := 0; i < 130; i++ {
		r.tickLocked(time.Now(), .05)
	}
	if r.gs.Ph != "live" || r.gs.T < 300 {
		t.Fatalf("deathmatch boshlanmadi: %+v", r.gs)
	}
	r.mu.Unlock()
	// birinchi hayot istalgan ochiq joyda boshlanadi (bazada emas)
	gm := mapByID("")
	open := cellCenter(gm.open[len(gm.open)/2])
	f := pres(a, open.X, open.Z, nil)
	if x, _ := num(f["x"]); math.Abs(x-open.X) > .01 {
		t.Fatalf("deathmatch'da ochiq joyda tug'ilish rad etildi: %s", f["x"])
	}
	pres(b, open.X+3, open.Z, nil)
	// tug'ilgandagi himoya: darhol otilgan o'q o'tmaydi
	f = pres(a, open.X, open.Z, map[string]any{"hi": []map[string]any{{"i": 1, "t": "b", "d": 30, "pn": .7, "p": "c", "w": "rifle"}}})
	if string(f["hi"]) != "null" {
		t.Fatalf("himoyadagi o'yinchiga o'q o'tdi: %s", f["hi"])
	}
	r.mu.Lock()
	a.ps.spawnAt, b.ps.spawnAt = time.Now().Add(-3*time.Second), time.Now().Add(-3*time.Second)
	r.mu.Unlock()
	// bir jamoada bo'lsa ham o'q tegadi va o'ldirish sanaladi
	f = pres(a, open.X, open.Z, map[string]any{"hi": []map[string]any{{"i": 2, "t": "b", "d": 36, "pn": .7, "p": "c", "w": "rifle"}}})
	if string(f["hi"]) == "null" {
		t.Fatal("deathmatch'da o'q rad etildi")
	}
	pres(b, open.X+3, open.Z, map[string]any{"al": 0, "de": map[string]any{"n": 1, "by": "a", "w": "rifle", "hs": 0}})
	if r.gs.Sc["a"] != 1 {
		t.Fatalf("o'ldirish sanalmadi: %v", r.gs.Sc)
	}
	// qayta tug'ilish: darhol emas, 1,6 soniyadan keyin
	f = pres(b, open.X+3, open.Z, nil)
	if string(f["al"]) != "0" {
		t.Fatal("o'lgan zahoti tirilish qabul qilindi")
	}
	r.mu.Lock()
	b.ps.deadAt = time.Now().Add(-2 * time.Second)
	r.mu.Unlock()
	f = pres(b, open.X+3, open.Z, nil)
	if string(f["al"]) == "0" || !b.ps.alive {
		t.Fatal("qayta tug'ilish rad etildi")
	}
	// vaqt tugadi — eng ko'p o'ldirgan yutadi
	r.mu.Lock()
	r.gs.T = .01
	r.tickLocked(time.Now(), .05)
	if r.gs.Ph != "over" || r.gs.Wn != "a" {
		t.Fatalf("g'olib noto'g'ri: %+v", r.gs)
	}
	r.mu.Unlock()

	// qurol poygasi: ikki jamoa, faqat hozirgi qurol bilan
	r2, x, y := newRefRoom()
	r2.gs.Md, r2.gs.Ph, r2.gs.R, r2.gs.T, r2.gs.Sc = modeGG, "live", 1, 500, map[string]int{}
	p2 := func(c *client, sq, tm string, z float64, extra map[string]any) map[string]json.RawMessage {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "rifle"}
		for k, v := range extra {
			d[k] = v
		}
		return refPres(r2, c, d)
	}
	p2(x, "A", "T", 75, nil)
	p2(y, "B", "CT", 9, nil)
	r2.mu.Lock()
	x.ps.spawnAt, y.ps.spawnAt = time.Time{}, time.Time{}
	x.ps.pos, y.ps.pos = vec3{43, 0, 30}, vec3{43, 0, 34} // ochiq o'rta yo'lak
	for i := range 3 {
		x.ps.push(sample{t: time.Now(), p: x.ps.pos})
		y.ps.push(sample{t: time.Now(), p: y.ps.pos})
		_ = i
	}
	r2.mu.Unlock()
	f = refPres(r2, x, map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 30,
		"hi": []map[string]any{{"i": 1, "t": y.id, "d": 30, "pn": .7, "p": "c", "w": "sniper"}}})
	if string(f["hi"]) != "null" {
		t.Fatal("qurol poygasida navbatdagi bo'lmagan qurol bilan o'q o'tdi")
	}
	r2.mu.Lock()
	for i := 0; i < ggWinKills()-1; i++ {
		r2.creditKillLocked(y, x, time.Now())
	}
	if ggWeapon(r2.gs.Sc[x.id]) != "knife" || r2.gs.Ph != "live" {
		t.Fatalf("pichoq bosqichi: %v %s", r2.gs.Sc, r2.gs.Ph)
	}
	r2.creditKillLocked(y, x, time.Now())
	if r2.gs.Ph != "over" || r2.gs.Wn != x.id {
		t.Fatalf("pichoq bilan o'ldirgan yutishi kerak: %+v", r2.gs)
	}
	r2.mu.Unlock()
}

// TestModeBots: deathmatch'da botlar tug'iladi, aylanadi va bir-birini o'ldiradi.
func TestModeBots(t *testing.T) {
	r := &room{name: "zn-dm", clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), manual: true}
	r.gs.Md = modeDM
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for range 4 {
		r.addBotLocked("A", "hard", now)
	}
	dt := .05
	for i := 0; i < 20*120; i++ { // 2 daqiqa
		now = now.Add(50 * time.Millisecond)
		for _, c := range r.clients {
			r.botStepLocked(c, now, dt)
		}
		r.tickLocked(now, dt)
	}
	total := 0
	for _, k := range r.gs.Sc {
		total += k
	}
	if r.gs.Ph != "live" || total < 3 {
		t.Fatalf("deathmatch'da botlar o'ynamadi: ph=%s sc=%v", r.gs.Ph, r.gs.Sc)
	}
	for _, c := range r.clients {
		if c.ps.alive && r.gm().botSolid(wc(c.ps.pos.X), wc(c.ps.pos.Z)) {
			t.Fatalf("bot devor ichida: %v", c.ps.pos)
		}
	}
}

// TestGGLadderMatchesClient: qurol poygasi zinapoyasi brauzerdagi bilan bir xil.
func TestGGLadderMatchesClient(t *testing.T) {
	b, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	want := "const GG_LADDER = ['" + strings.Join(ggLadder, "','") + "'], GG_PER = " + strconv.Itoa(ggPerLevel) + ","
	if !strings.Contains(string(b), want) {
		t.Fatalf("web/index.html da %q topilmadi", want)
	}
}
