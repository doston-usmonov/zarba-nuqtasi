package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testMapRows: oddiy, to'g'ri xarita — katta maydon, ikki baza, ikki nuqta, bir necha quti.
func testMapRows() string {
	g := make([][]byte, gridH)
	for z := range g {
		g[z] = []byte(strings.Repeat("#", gridW))
	}
	rect := func(x0, z0, x1, z1 int, ch byte) {
		for z := z0; z <= z1; z++ {
			for x := x0; x <= x1; x++ {
				g[z][x] = ch
			}
		}
	}
	rect(2, 2, 41, 41, '.')
	rect(15, 38, 28, 40, 't')
	rect(15, 3, 28, 5, 'k')
	rect(4, 8, 10, 14, 'A')
	rect(33, 8, 39, 14, 'B')
	rect(18, 18, 25, 25, '#') // o'rtada bino
	g[20][10], g[30][30], g[12][7] = 'c', 's', 'c'
	var b strings.Builder
	for _, r := range g {
		b.Write(r)
	}
	return b.String()
}

func TestValidateCustomRows(t *testing.T) {
	ok := testMapRows()
	rows, err := validateCustomRows(ok)
	if err != nil || len(rows) != gridH {
		t.Fatalf("to'g'ri xarita rad etildi: %v", err)
	}
	// chegara devorga aylanadi
	edge := []byte(ok)
	edge[0], edge[5] = '.', 'A'
	if rows, err := validateCustomRows(string(edge)); err != nil || rows[0][0] != '#' || rows[0][5] != '#' {
		t.Fatalf("chegara: %v %q", err, rows[0][:8])
	}
	bad := func(name string, f func(b []byte) string, want string) {
		t.Helper()
		_, err := validateCustomRows(f([]byte(ok)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (kutilgan «%s»)", name, err, want)
		}
	}
	bad("o'lcham", func(b []byte) string { return string(b[:100]) }, "o'lchami")
	bad("belgi", func(b []byte) string { b[100] = 'x'; return string(b) }, "noma'lum")
	bad("A yo'q", func(b []byte) string { return strings.ReplaceAll(string(b), "A", ".") }, "A nuqta")
	bad("kichik baza", func(b []byte) string {
		s := strings.Replace(string(b), "t", ".", 40) // 42 tadan 2 tasi qoladi
		return s
	}, "Hujumchilar bazasi")
	bad("B ga yo'l yo'q", func(b []byte) string { // B ni devor bilan o'raymiz
		for z := 7; z <= 15; z++ {
			b[z*gridW+32], b[z*gridW+40] = '#', '#'
		}
		for x := 32; x <= 40; x++ {
			b[7*gridW+x], b[15*gridW+x] = '#', '#'
		}
		return string(b)
	}, "B nuqtaga yo'l yo'q")
}

func TestAutoPlan(t *testing.T) {
	rows, _ := validateCustomRows(testMapRows())
	p := autoPlan(rows)
	for _, s := range []string{"A", "B"} {
		if len(p.spots[s]) == 0 || len(p.looks[s]) == 0 || len(p.holds[s]) < 2 || len(p.stages[s]) == 0 {
			t.Fatalf("%s: %+v", s, p)
		}
		for _, c := range p.spots[s] {
			if rows[c[1]][c[0]] != s[0] {
				t.Fatalf("%s bomba joyi nuqtada emas: %v", s, c)
			}
		}
		for _, h := range p.holds[s] {
			if !walkCell(rows[h[0][1]][h[0][0]]) {
				t.Fatalf("%s himoya joyi yurib bo'lmas katakda: %v", s, h)
			}
		}
	}
	if len(p.holds["M"]) == 0 || len(p.holdSite) != len(p.holds["A"])+len(p.holds["B"])+len(p.holds["M"]) {
		t.Fatalf("o'rta: %+v", p)
	}
	gm := newCustomGameMap("k1", "Sinov", rows)
	if gm.siteAt(cellCenter([2]int{6, 10}).X, cellCenter([2]int{6, 10}).Z) != "A" || len(gm.open) == 0 || gm.tCenter.Z < gm.ctCenter.Z {
		t.Fatalf("server xaritasi: %+v", gm.tCenter)
	}
}

func TestCleanMapName(t *testing.T) {
	for in, want := range map[string]string{"  Mening   xaritam ": "Mening xaritam", "O'rmon-2": "O'rmon-2", "<b>Zo'r</b>": "bZo'rb"} {
		if got, err := cleanMapName(in); err != nil || got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
	for _, in := range []string{"ab", strings.Repeat("x", 33), "<<>>"} {
		if _, err := cleanMapName(in); err == nil {
			t.Errorf("%q qabul qilindi", in)
		}
	}
}

func TestCustomMapsAPI(t *testing.T) {
	srv, st, h := newTestServer(t)
	defer srv.Close()
	c := newTestClient(srv.URL)
	if code, _ := c.post(t, "/api/maps", map[string]any{"name": "Mehmon", "style": "qum", "rows": testMapRows()}); code != http.StatusUnauthorized {
		t.Fatalf("mehmon saqlay olmasligi kerak: %d", code)
	}
	c.post(t, "/api/register", map[string]any{"username": "Chizuvchi", "password": "parol123"})
	if code, d := c.post(t, "/api/maps", map[string]any{"name": "Yomon", "style": "qum", "rows": strings.ReplaceAll(testMapRows(), "B", ".")}); code != 400 || !strings.Contains(d["error"].(string), "B nuqta") {
		t.Fatalf("noto'g'ri xarita: %d %v", code, d)
	}
	if code, d := c.post(t, "/api/maps", map[string]any{"name": "X", "style": "qum", "rows": testMapRows()}); code != 400 || !strings.Contains(d["error"].(string), "nomi") {
		t.Fatalf("qisqa nom: %d %v", code, d)
	}
	code, d := c.post(t, "/api/maps", map[string]any{"name": "Birinchi xarita", "style": "bekat", "rows": testMapRows()})
	if code != 200 {
		t.Fatalf("saqlash: %d %v", code, d)
	}
	id := int64(d["id"].(float64))
	ids := strconv.FormatInt(id, 10)
	if code, d := c.get(t, "/api/maps"); code != 200 || len(d["maps"].([]any)) != 1 || d["maps"].([]any)[0].(map[string]any)["author"] != "Chizuvchi" {
		t.Fatalf("ro'yxat: %d %v", code, d)
	}
	code, d = c.get(t, "/api/maps/"+ids)
	m, _ := d["map"].(map[string]any)
	if code != 200 || m["name"] != "Birinchi xarita" || len(m["rows"].(string)) != gridW*gridH || d["holds"] == nil || d["plan"] == nil {
		t.Fatalf("xarita: %d %v", code, d)
	}
	// boshqa o'yinchi o'zgartira olmaydi
	c2 := newTestClient(srv.URL)
	c2.post(t, "/api/register", map[string]any{"username": "Boshqa", "password": "parol123"})
	if code, _ := c2.post(t, "/api/maps", map[string]any{"id": id, "name": "Buzildi", "style": "qum", "rows": testMapRows()}); code != http.StatusForbidden {
		t.Fatalf("begona xaritani o'zgartirish: %d", code)
	}
	// onlayn xona: server xaritani bazadan o'qiydi va botlar shu xaritada yuradi
	w := dialWS(t, c, "zn-meniki&create=1&map=k"+ids+"&mode=comp")
	defer w.c.Close()
	var rm *room
	waitFor := func(what string, f func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			if f() {
				return
			}
		}
		t.Fatal(what)
	}
	waitFor("xona", func() bool { h.mu.Lock(); rm = h.rooms["zn-meniki"]; h.mu.Unlock(); return rm != nil })
	rm.mu.Lock()
	gm, name := rm.gm(), rm.listingLocked().MapName
	rm.mu.Unlock()
	if gm.id != "k"+ids || name != "Birinchi xarita" || gm.siteAt(cellCenter([2]int{36, 10}).X, cellCenter([2]int{36, 10}).Z) != "B" {
		t.Fatalf("xona xaritasi: %s %q", gm.id, name)
	}
	waitFor("o'ynalganlar soni", func() bool {
		var n int
		st.db.QueryRow(`SELECT plays FROM custom_maps WHERE id = $1`, id).Scan(&n)
		return n == 1
	})
	// statistika: foydalanuvchi xaritasi xaritalar bo'limiga yozilmaydi — progress.go
	// o'chirish
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/maps/"+ids, nil)
	if res, err := c2.do(req, srv.URL); err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("begona o'chirish: %v %v", err, res)
	}
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/maps/"+ids, nil)
	if res, err := c.do(req, srv.URL); err != nil || res.StatusCode != 200 {
		t.Fatalf("o'chirish: %v %v", err, res)
	}
	if code, _ := c.get(t, "/api/maps/"+ids); code != 404 {
		t.Fatalf("o'chirilgan xarita: %d", code)
	}
	if loadCustomMap(context.Background(), st, "k"+ids) != nil {
		t.Fatal("o'chirilgan xarita keshda qoldi")
	}
}

func (c *testClient) do(req *http.Request, origin string) (*http.Response, error) {
	req.Header.Set("Origin", origin)
	res, err := c.http.Do(req)
	if err == nil {
		res.Body.Close()
	}
	return res, err
}
