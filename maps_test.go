package main

import (
	"os"
	"regexp"
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
