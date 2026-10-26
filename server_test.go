package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEloDelta(t *testing.T) {
	if d := eloDelta(1000, 1000, true, 20, 1); d < 12 || d > 14 {
		t.Fatalf("teng raqiblar, g'alaba 8:7: %d", d)
	}
	if w, l := eloDelta(1000, 1000, true, 20, 1), eloDelta(1000, 1000, false, 20, 1); w != -l {
		t.Fatalf("simmetriya buzildi: %d / %d", w, l)
	}
	if a, b := eloDelta(1000, 1000, true, 20, 8), eloDelta(1000, 1000, true, 20, 1); a <= b {
		t.Fatalf("8:0 g'alaba 8:7 dan ko'p berishi kerak: %d <= %d", a, b)
	}
	if d := eloDelta(1600, 1000, true, 20, 1); d != 1 {
		t.Fatalf("kuchli jamoa kuchsizni yengsa ham kamida +1: %d", d)
	}
	if d := eloDelta(1000, 1600, true, 20, 1); d < 20 {
		t.Fatalf("kuchsiz jamoa g'alabasi katta bo'lishi kerak: %d", d)
	}
	if rankOf(1300, 2).Name != "Kalibrlash" || rankOf(1300, 5).Name != "Katta serjant" || rankOf(5000, 50).Name != "General" {
		t.Fatal("darajalar noto'g'ri")
	}
}

// ---- PostgreSQL bilan testlar: ZARBA_TEST_DATABASE_URL berilmasa o'tkazib yuboriladi ----

// testStore toza sxema bilan yangi Store ochadi (har test uchun baza tozalanadi).
func testStore(t *testing.T, legacyDir string) *Store {
	t.Helper()
	dsn := os.Getenv("ZARBA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ZARBA_TEST_DATABASE_URL berilmagan — PostgreSQL testlari o'tkazib yuborildi")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("test bazasi tozalanmadi: %v", err)
	}
	db.Close()
	st, err := openStore(dsn, legacyDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; !st.Ready(); i++ {
		if i > 200 {
			t.Fatal("baza 20 soniyada tayyor bo'lmadi")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Cleanup(func() { st.db.Close() })
	return st
}

func TestStore(t *testing.T) {
	s, ctx := testStore(t, ""), context.Background()
	if _, _, err := s.Register(ctx, "ab", "secret1"); err != errBadUsername {
		t.Fatalf("qisqa login qabul qilinmasligi kerak: %v", err)
	}
	if _, _, err := s.Register(ctx, "Ali", "123"); err != errBadPassword {
		t.Fatalf("qisqa parol qabul qilinmasligi kerak: %v", err)
	}
	u, tok, err := s.Register(ctx, "Ali_01", "secret1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Register(ctx, "ali_01", "boshqa1"); err != errTaken {
		t.Fatalf("katta-kichik harf farqi bilan ham band: %v", err)
	}
	if _, _, err := s.Login(ctx, "ALI_01", "wrong-pass"); err != errBadLogin {
		t.Fatalf("noto'g'ri parol: %v", err)
	}
	if _, _, err := s.Login(ctx, "nobody", "secret1"); err != errBadLogin {
		t.Fatalf("mavjud bo'lmagan login: %v", err)
	}
	if _, _, err := s.Login(ctx, "ali_01", "secret1"); err != nil {
		t.Fatalf("to'g'ri parol: %v", err)
	}
	res := &matchResult{Room: "zn-x", Started: time.Now().Add(-5 * time.Minute), Ended: time.Now(), ScoreT: 8, ScoreCT: 3, Winner: "T",
		Players: []resultPlayer{{UserID: u.ID, Name: "Ali_01", Team: "T", Kills: 9, Deaths: 4, HS: 3}, {Name: "Mehmon", Team: "CT", Kills: 4, Deaths: 9}}}
	ch, err := s.ApplyMatch(ctx, res)
	if err != nil || ch[u.ID].Delta <= 0 {
		t.Fatalf("g'olib reytingi oshishi kerak: %+v %v", ch, err)
	}
	got, ok := s.Session(ctx, tok)
	if !ok || got.Username != "Ali_01" || got.Rating != ch[u.ID].After || got.Kills != 9 || got.HS != 3 || got.Matches != 1 {
		t.Fatalf("bazadagi holat: %+v ok=%v", got, ok)
	}
	if _, pos, hist, ok, err := s.Profile(ctx, "ali_01"); err != nil || !ok || pos != 1 || len(hist) != 1 || !hist[0].Won || hist[0].After != got.Rating {
		t.Fatalf("profil/tarix: pos=%d hist=%+v err=%v", pos, hist, err)
	}
	var guests int
	s.db.QueryRow(`SELECT count(*) FROM match_players WHERE user_id IS NULL AND name = 'Mehmon'`).Scan(&guests)
	if guests != 1 {
		t.Fatalf("mehmon o'yin tarixiga yozilishi kerak: %d", guests)
	}
	if err := s.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Session(ctx, tok); ok {
		t.Fatal("chiqqandan keyin sessiya ishlamasligi kerak")
	}
}

// Oldingi versiyaning fayl bazasidan (snapshot.json + matches.jsonl) import.
func TestLegacyImport(t *testing.T) {
	dir := t.TempDir()
	salt := randBytes(16)
	hash := hashPassword("eski-parol", salt, 1000)
	token := "eski-sessiya-tokeni"
	snap := map[string]any{
		"version": 1, "next_user": 3, "next_match": 2,
		"users": []map[string]any{
			{"id": 1, "username": "Eski_Ali", "salt": salt, "hash": hash, "iter": 1000, "rating": 1042, "matches": 1, "wins": 1, "kills": 7, "deaths": 3, "hs": 2, "created": 1761100000},
			{"id": 2, "username": "Eski_Bek", "salt": salt, "hash": hash, "iter": 1000, "rating": 958, "matches": 1, "wins": 0, "kills": 3, "deaths": 7, "hs": 1, "created": 1761100000},
		},
		"sessions": map[string]any{hex.EncodeToString(tokenHash(token)): map[string]any{"u": 1, "exp": time.Now().Add(time.Hour).Unix()}},
	}
	b, _ := json.Marshal(snap)
	os.WriteFile(dir+"/snapshot.json", b, 0o600)
	m := `{"id":1,"room":"zn-eski","started":1761100000,"ended":1761101000,"sT":8,"sCT":2,"winner":"T","players":[` +
		`{"uid":1,"name":"Eski_Ali","team":"T","k":7,"d":3,"hs":2,"won":true,"before":1000,"after":1042},` +
		`{"uid":2,"name":"Eski_Bek","team":"CT","k":3,"d":7,"hs":1,"won":false,"before":1000,"after":958},` +
		`{"uid":0,"name":"Mehmon","team":"CT","k":0,"d":1,"hs":0,"won":false,"before":0,"after":0}]}` + "\n"
	os.WriteFile(dir+"/matches.jsonl", []byte(m), 0o600)

	s, ctx := testStore(t, dir), context.Background()
	u, ok := s.Session(ctx, token)
	if !ok || u.Username != "Eski_Ali" || u.Rating != 1042 || u.Kills != 7 {
		t.Fatalf("eski sessiya va hisob ko'chmadi: %+v %v", u, ok)
	}
	if _, _, err := s.Login(ctx, "eski_bek", "eski-parol"); err != nil {
		t.Fatalf("eski parol bilan kirish: %v", err)
	}
	if _, _, hist, _, _ := s.Profile(ctx, "Eski_Bek"); len(hist) != 1 || hist[0].Won || hist[0].After != 958 {
		t.Fatalf("eski o'yinlar tarixi: %+v", hist)
	}
	nu, _, err := s.Register(ctx, "Yangi", "parol123") // id ketma-ketligi import qilinganlardan davom etadi
	if err != nil || nu.ID != 3 {
		t.Fatalf("yangi hisob id: %d %v", nu.ID, err)
	}
	if _, err := os.Stat(dir + "/snapshot.json"); !os.IsNotExist(err) {
		t.Fatal("import qilingan fayl qayta nomlanishi kerak")
	}
}

// ---- to'liq oqim: HTTP API + WebSocket orqali o'yin ----

func newTestServer(t *testing.T) (*httptest.Server, *Store, *hub) {
	st := testStore(t, "")
	a := newAPI(st, nil)
	h := newHub(16, st)
	mux := http.NewServeMux()
	a.routes(mux)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		var acct *account
		if u, ok := a.currentUser(r); ok {
			acct = &account{id: u.ID, name: u.Username}
		}
		ws, err := upgrade(w, r)
		if err != nil {
			return
		}
		h.serve(ws, r.URL.Query().Get("room"), r.RemoteAddr+newID(), r.Host, acct, parseRoomOpts(r.URL.Query()))
	})
	return httptest.NewServer(mux), st, h
}

type testClient struct {
	http *http.Client
	base string
}

func (c *testClient) post(t *testing.T, path string, body any) (int, map[string]any) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *testClient) get(t *testing.T, path string) (int, map[string]any) {
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func newTestClient(base string) *testClient {
	jar, _ := cookiejar.New(nil)
	return &testClient{http: &http.Client{Jar: jar}, base: base}
}

// minimal WebSocket mijoz (testlar uchun)
type wsTest struct {
	c     net.Conn
	br    *bufio.Reader
	id    string
	hello map[string]any
}

func dialWS(t *testing.T, tc *testClient, room string) *wsTest {
	u, _ := url.Parse(tc.base)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	rand.Read(key)
	var cookies []string
	for _, ck := range tc.http.Jar.Cookies(u) {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	req := "GET /ws?room=" + room + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " +
		base64.StdEncoding.EncodeToString(key) + "\r\nCookie: " + strings.Join(cookies, "; ") + "\r\n\r\n"
	conn.Write([]byte(req))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatalf("websocket ulanmadi: %v %v", err, resp)
	}
	w := &wsTest{c: conn, br: br}
	for {
		m := w.read(t)
		if m["t"] == "hello" {
			w.id, w.hello = m["id"].(string), m
			return w
		}
	}
}

func (w *wsTest) send(v any) {
	p, _ := json.Marshal(v)
	hdr := []byte{0x81}
	if len(p) < 126 {
		hdr = append(hdr, 0x80|byte(len(p)))
	} else {
		hdr = append(hdr, 0x80|126, byte(len(p)>>8), byte(len(p)))
	}
	mask := []byte{1, 2, 3, 4}
	hdr = append(hdr, mask...)
	for i := range p {
		p[i] ^= mask[i&3]
	}
	w.c.Write(append(hdr, p...))
}

func (w *wsTest) read(t *testing.T) map[string]any {
	w.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var h [2]byte
	if _, err := io.ReadFull(w.br, h[:]); err != nil {
		t.Fatalf("o'qilmadi: %v", err)
	}
	n := int(h[1] & 0x7f)
	if n == 126 {
		var b [2]byte
		io.ReadFull(w.br, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	} else if n == 127 {
		var b [8]byte
		io.ReadFull(w.br, b[:])
		n = int(binary.BigEndian.Uint64(b[:]))
	}
	p := make([]byte, n)
	io.ReadFull(w.br, p)
	if h[0]&0x0f != opText {
		return map[string]any{}
	}
	var m map[string]any
	json.Unmarshal(p, &m)
	return m
}

// readUntil: shart bajarilguncha xabarlarni o'qiydi.
func (w *wsTest) readUntil(t *testing.T, ok func(map[string]any) bool) map[string]any {
	for i := 0; i < 200; i++ {
		if m := w.read(t); ok(m) {
			return m
		}
	}
	t.Fatal("kutilgan xabar kelmadi")
	return nil
}

func TestAccountsAndRatedMatch(t *testing.T) {
	srv, _, h := newTestServer(t)
	defer srv.Close()
	ali, bek, guest := newTestClient(srv.URL), newTestClient(srv.URL), newTestClient(srv.URL)

	if code, _ := ali.get(t, "/api/me"); code != 401 {
		t.Fatalf("kirmagan foydalanuvchi: %d", code)
	}
	if code, out := ali.post(t, "/api/register", credentials{"Ali", "parol123"}); code != 201 {
		t.Fatalf("ro'yxat: %d %v", code, out)
	}
	if code, _ := bek.post(t, "/api/register", credentials{"ali", "boshqa12"}); code != 409 {
		t.Fatalf("band login: %d", code)
	}
	bek.post(t, "/api/register", credentials{"Bek", "parol456"})
	if code, out := ali.get(t, "/api/me"); code != 200 || out["user"].(map[string]any)["rating"].(float64) != 1000 {
		t.Fatalf("me: %d %v", code, out)
	}
	// boshqa saytdan kelgan so'rov rad etiladi
	req, _ := http.NewRequest("POST", srv.URL+"/api/login", strings.NewReader(`{"username":"Ali","password":"parol123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://yomon.example")
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 403 {
		t.Fatalf("begona Origin: %d", resp.StatusCode)
	}

	room := "zn-test"
	wa, wb, wg := dialWS(t, ali, room), dialWS(t, bek, room), dialWS(t, guest, room)
	// bir hisob bilan bitta xonaga ikkinchi marta kirib bo'lmaydi
	dup := dialWS2(t, ali, room)
	if m := dup.read(t); m["code"] != "already_in_room" {
		t.Fatalf("takror ulanish: %v", m)
	}

	// raundni server yuritadi; test uni qo'lda boshqaradi (taymer o'chiriladi)
	h.mu.Lock()
	rm := h.rooms[room]
	h.mu.Unlock()
	rm.mu.Lock()
	rm.manual = true
	rm.mu.Unlock()
	tSpawn, ctSpawn := [3]float64{43, 0, 75}, [3]float64{43, 0, 9}
	pres := func(w *wsTest, nm, sq, tm string, in int, at [3]float64, extra map[string]any) {
		d := map[string]any{"v": 1, "hc": 1, "nm": nm, "sq": sq, "tm": tm, "in": in, "al": 1, "x": at[0], "y": at[1], "z": at[2], "pf": 8}
		for k, v := range extra {
			d[k] = v
		}
		w.send(map[string]any{"t": "p", "d": d})
	}
	all := func(in int) {
		tA, tB, sA, sB := "T", "CT", tSpawn, ctSpawn
		if in > 8 { // ikkinchi yarim: tomonlar almashadi
			tA, tB, sA, sB = "CT", "T", ctSpawn, tSpawn
		}
		pres(wa, "Yolg'onchi", "A", tA, in, sA, nil)
		pres(wb, "Bek", "B", tB, in, sB, nil)
		pres(wg, "Mehmon1", "B", tB, in, sB, nil)
		time.Sleep(40 * time.Millisecond)
	}
	place := func(w *wsTest, at vec3) {
		rm.mu.Lock()
		p := rm.clients[w.id].ps
		p.pos, p.posOK, p.moveAt = at, true, time.Now()
		p.push(sample{t: time.Now(), p: at})
		rm.mu.Unlock()
	}
	all(0)
	// boshqalar Ali'ning ismini server yozgan holda ko'radi, "acc" bilan
	upd := wb.readUntil(t, func(m map[string]any) bool { return m["t"] == "u" && m["id"] == wa.id })
	p := upd["p"].(map[string]any)
	if p["nm"] != "Ali" || p["acc"].(map[string]any)["r"].(float64) != 1000 {
		t.Fatalf("server belgilagan nom/reyting: %v", p)
	}
	rm.mu.Lock()
	rm.tickLocked(time.Now(), .1) // isinish: format birinchi o'yinchining sozlamasidan
	if rm.gs.Fm != 8 {
		t.Fatalf("format: %d", rm.gs.Fm)
	}
	*rm.gs = gameState{R: 1, Ph: "live", Fm: 8, E: "m1", Bm: -1, T: 100, liveAt: time.Now()}
	rm.tr.phase(rm, "warmup", time.Now())
	if !rm.tr.active || rm.tr.epoch != "m1" {
		t.Fatalf("o'yin boshlanishi kuzatilmadi: %+v", rm.tr)
	}
	// test 10 raundni bir zumda "o'ynaydi": boshlanish vaqtini orqaga surib, "juda tez o'yin" tekshiruvidan o'tkazamiz
	rm.tr.started = rm.tr.started.Add(-10 * time.Minute)
	rm.mu.Unlock()
	all(1)
	// Ali va Bek o'rtada bir-birini ko'radi, mehmon esa devor ortida
	place(wa, vec3{43, 0, 40})
	place(wb, vec3{43, 0, 30})
	place(wg, vec3{30, 0, 9})
	time.Sleep(700 * time.Millisecond) // kechikish oynasidan eski (bazadagi) holatlar chiqib ketsin
	place(wa, vec3{43, 0, 40})
	place(wb, vec3{43, 0, 30})
	place(wg, vec3{30, 0, 9})
	// mehmon devor ortidan Ali'ga "tegdi" — server bu tegishni boshqalarga yubormaydi
	pres(wg, "Mehmon1", "B", "CT", 1, [3]float64{30, 0, 9}, map[string]any{"mk": "devor", "w": "rifle", "hi": []any{map[string]any{"i": 1, "t": wa.id, "d": 144, "pn": .8, "p": "h", "w": "rifle"}}})
	marked := func(id, mk string) func(map[string]any) bool {
		return func(m map[string]any) bool {
			p, _ := m["p"].(map[string]any)
			return m["t"] == "u" && m["id"] == id && p != nil && p["mk"] == mk
		}
	}
	m := wa.readUntil(t, marked(wg.id, "devor"))
	if hi := m["p"].(map[string]any)["hi"]; hi != nil {
		t.Fatalf("devor ortidan tegish o'tib ketdi: %v", hi)
	}
	// Ali Bek'ning boshiga tegdi — bu haqiqiy
	pres(wa, "Yolg'onchi", "A", "T", 1, [3]float64{43, 0, 40}, map[string]any{"mk": "bosh", "w": "rifle", "hi": []any{map[string]any{"i": 1, "t": wb.id, "d": 144, "pn": .775, "p": "h", "w": "rifle"}}})
	m = wb.readUntil(t, marked(wa.id, "bosh"))
	if hi, _ := m["p"].(map[string]any)["hi"].([]any); len(hi) != 1 {
		t.Fatalf("haqiqiy tegish yetib bormadi: %v", m["p"])
	}
	// Bek o'ladi (Ali o'ldirdi), takror xabar bir marta hisoblanadi
	for i := 0; i < 2; i++ {
		pres(wb, "Bek", "B", "CT", 1, [3]float64{43, 0, 30}, map[string]any{"al": 0, "de": map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1}})
	}
	// mehmon "meni Ali o'ldirdi" deydi, lekin Ali unga tegmagan — o'ldirish Ali'ga yozilmaydi
	pres(wg, "Mehmon1", "B", "CT", 1, [3]float64{30, 0, 9}, map[string]any{"al": 0, "de": map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1}})
	m = wa.readUntil(t, func(m map[string]any) bool {
		return m["t"] == "u" && m["id"] == wg.id && m["p"].(map[string]any)["de"] != nil
	})
	if by := m["p"].(map[string]any)["de"].(map[string]any)["by"]; by != "" {
		t.Fatalf("tasdiqlanmagan o'ldirish: %v", by)
	}
	time.Sleep(50 * time.Millisecond)
	for r := 2; r <= 10; r++ {
		rm.mu.Lock()
		rm.gs.R, rm.gs.Ph = r, "live"
		rm.tr.phase(rm, "live", time.Now())
		rm.mu.Unlock()
		all(r)
	}
	rm.mu.Lock()
	rm.gs.R, rm.gs.Ph, rm.gs.T, rm.gs.SA, rm.gs.SB = 10, "post", 0, 9, 1
	res := rm.tickLocked(time.Now(), .1)
	rm.mu.Unlock()
	if res == nil {
		t.Fatal("o'yin natijasi chiqmadi")
	}
	h.finishMatch(rm, res)

	ra := wa.readUntil(t, func(m map[string]any) bool { return m["t"] == "rt" })
	rb := wb.readUntil(t, func(m map[string]any) bool { return m["t"] == "rt" })
	if ra["won"] != true || ra["delta"].(float64) <= 0 || rb["won"] != false || rb["delta"].(float64) >= 0 {
		t.Fatalf("reyting o'zgarishi: ali=%v bek=%v", ra, rb)
	}
	_, me := ali.get(t, "/api/me")
	u := me["user"].(map[string]any)
	if u["matches"].(float64) != 1 || u["wins"].(float64) != 1 || u["kills"].(float64) != 1 || u["hs"].(float64) != 1 {
		t.Fatalf("Ali statistikasi: %v", u)
	}
	_, lb := guest.get(t, "/api/leaderboard")
	players := lb["players"].([]any)
	if len(players) != 2 || players[0].(map[string]any)["username"] != "Ali" {
		t.Fatalf("reyting jadvali: %v", lb)
	}
	_, prof := guest.get(t, "/api/players/bek")
	if ms := prof["matches"].([]any); len(ms) != 1 || ms[0].(map[string]any)["won"] != false || ms[0].(map[string]any)["d"].(float64) != 1 {
		t.Fatalf("Bek tarixi: %v", prof)
	}
	if code, _ := ali.post(t, "/api/logout", map[string]any{}); code != 200 {
		t.Fatal("chiqish")
	}
	if code, _ := ali.get(t, "/api/me"); code != 401 {
		t.Fatal("chiqqandan keyin sessiya o'chishi kerak")
	}
}

// dialWS2: hello kutmasdan ulanadi (rad etilishini tekshirish uchun).
func dialWS2(t *testing.T, tc *testClient, room string) *wsTest {
	u, _ := url.Parse(tc.base)
	conn, _ := net.Dial("tcp", u.Host)
	var cookies []string
	for _, ck := range tc.http.Jar.Cookies(u) {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	conn.Write([]byte("GET /ws?room=" + room + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nCookie: " + strings.Join(cookies, "; ") + "\r\n\r\n"))
	br := bufio.NewReader(conn)
	http.ReadResponse(br, nil)
	return &wsTest{c: conn, br: br}
}

func TestValidFinal(t *testing.T) {
	ok := [][3]int{{12, 13, 0}, {12, 13, 11}, {12, 9, 13}, {12, 16, 12}, {12, 14, 16}, {12, 19, 17}, {12, 15, 19}, {8, 9, 7}, {8, 12, 10}}
	bad := [][3]int{{12, 13, 12}, {12, 12, 10}, {12, 15, 12}, {12, 16, 15}, {12, 17, 13}, {8, 13, 0}, {10, 11, 0}, {12, 0, 0}, {8, 9, 8}}
	for _, c := range ok {
		if !validFinal(c[0], c[1], c[2]) {
			t.Errorf("haqiqiy natija rad etildi: fm=%d %d:%d", c[0], c[1], c[2])
		}
	}
	for _, c := range bad {
		if validFinal(c[0], c[1], c[2]) {
			t.Errorf("noto'g'ri natija qabul qilindi: fm=%d %d:%d", c[0], c[1], c[2])
		}
	}
}

func TestVoiceSignal(t *testing.T) {
	srv, _, h := newTestServer(t)
	defer srv.Close()
	h.voice = voiceCfg{stun: []string{"stun:stun.example.uz:3478"}, turnSecret: "maxfiy", turnPort: "3478"}
	tc := newTestClient(srv.URL)
	a, b := dialWS(t, tc, "zn-ovoz"), dialWS(t, tc, "zn-ovoz")
	other := dialWS(t, tc, "zn-boshqa")

	// hello: STUN va vaqtinchalik TURN login-paroli
	ice := a.hello["ice"].([]any)
	if len(ice) != 2 {
		t.Fatalf("ice: %v", ice)
	}
	turn := ice[1].(map[string]any)
	user, cred := turn["username"].(string), turn["credential"].(string)
	if !strings.HasSuffix(user, ":"+a.id) || cred != turnCredential("maxfiy", user) || turn["urls"].([]any)[0] != "turn:127.0.0.1:3478?transport=udp" {
		t.Fatalf("turn: %v", turn)
	}

	// xabar faqat ko'rsatilgan o'yinchiga boradi, "from" ni server yozadi
	a.send(map[string]any{"t": "sig", "to": b.id, "from": "soxta", "d": map[string]any{"sdp": map[string]any{"type": "offer", "sdp": "v=0"}}})
	m := b.readUntil(t, func(m map[string]any) bool { return m["t"] == "sig" })
	if m["from"] != a.id || m["d"].(map[string]any)["sdp"].(map[string]any)["type"] != "offer" {
		t.Fatalf("sig: %v", m)
	}
	// boshqa xonadagi o'yinchiga uzatilmaydi
	a.send(map[string]any{"t": "sig", "to": other.id, "d": map[string]any{"c": "x"}})
	other.send(map[string]any{"t": "p", "d": map[string]any{"v": 1}})
	a.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "nm": "Ali"}})
	b.readUntil(t, func(m map[string]any) bool { return m["t"] == "u" && m["id"] == a.id })
	// "other" faqat o'z xonasining xabarlarini (s) olgan bo'lishi kerak
	for {
		other.c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		var hdr [2]byte
		if _, err := io.ReadFull(other.br, hdr[:]); err != nil {
			break
		}
		p := make([]byte, int(hdr[1]&0x7f))
		io.ReadFull(other.br, p)
		if bytes.Contains(p, []byte(`"sig"`)) {
			t.Fatalf("boshqa xonaga xabar o'tib ketdi: %s", p)
		}
	}
}

// ---- server hakami: bazasiz unit test ----

func newRefRoom() (*room, *client, *client) {
	r := &room{name: "zn-hakam", clients: map[string]*client{}, gs: newGameState(), stop: make(chan struct{}), manual: true}
	mk := func(id string) *client {
		c := &client{id: id, room: r, send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
		r.clients[id] = c
		return c
	}
	return r, mk("a"), mk("b")
}

func refPres(r *room, c *client, d map[string]any) map[string]json.RawMessage {
	raw, _ := json.Marshal(d)
	var f map[string]json.RawMessage
	json.Unmarshal(raw, &f)
	info := parseInfo(f)
	r.mu.Lock()
	defer r.mu.Unlock()
	c.info = info
	r.refereeLocked(c, f, info, time.Now())
	c.deSeen = true
	out, _ := json.Marshal(f)
	c.pres = out
	return f
}

func drain(c *client) []map[string]any {
	var out []map[string]any
	for {
		select {
		case m := <-c.send:
			var v map[string]any
			json.Unmarshal(m, &v)
			out = append(out, v)
		default:
			return out
		}
	}
}

func TestReferee(t *testing.T) {
	r, a, b := newRefRoom()
	base := func(c *client, sq, tm string, x, z float64, extra map[string]any) map[string]json.RawMessage {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 1, "al": 1, "x": x, "y": 0, "z": z, "w": "rifle"}
		for k, v := range extra {
			d[k] = v
		}
		return refPres(r, c, d)
	}
	*r.gs = gameState{R: 1, Ph: "live", Fm: 12, E: "x", Bm: -1, T: 100, liveAt: time.Now()}
	// yangi hayot faqat bazada: B yarim xaritadan "tug'ilsa", server uni bazaga qaytaradi
	f := base(b, "B", "CT", 43, 40, nil)
	fx, _ := num(f["x"])
	fz, _ := num(f["z"])
	if !mapByID("").inSpawn(vec3{fx, 0, fz}, "CT") {
		t.Fatalf("bazadan tashqarida tug'ilish qabul qilindi: z=%s", f["z"])
	}
	base(a, "A", "T", 43, 75, nil)
	base(b, "B", "CT", 43, 9, nil)
	// teleport: bir zumda 30 metr — rad etiladi, "fix" yuboriladi
	f = base(a, "A", "T", 43, 45, nil)
	if string(f["z"]) != "75" {
		t.Fatalf("teleport qabul qilindi: z=%s", f["z"])
	}
	if ms := drain(a); len(ms) == 0 || ms[len(ms)-1]["t"] != "fix" {
		t.Fatalf("fix yuborilmadi: %v", ms)
	}
	// ikkalasini o'rtaga qo'yamiz (oddiy yurib kelgandek)
	for _, c := range []*client{a, b} {
		c.ps.pos, c.ps.moveAt = vec3{43, 0, map[string]float64{"a": 40, "b": 30}[c.id]}, time.Now()
		c.ps.push(sample{t: time.Now(), p: c.ps.pos})
	}
	// juda katta zarar — rad etiladi
	f = base(a, "A", "T", 43, 40, map[string]any{"hi": []any{map[string]any{"i": 1, "t": "b", "d": 400, "p": "c", "w": "rifle"}}})
	if string(f["hi"]) != "null" {
		t.Fatalf("katta zarar o'tdi: %s", f["hi"])
	}
	// haqiqiy tegish: 2 ta o'q boshga — server hisobida B ning joni tugaydi
	f = base(a, "A", "T", 43, 40, map[string]any{"hi": []any{map[string]any{"i": 2, "t": "b", "d": 70, "p": "h", "w": "rifle"}, map[string]any{"i": 3, "t": "b", "d": 70, "p": "h", "w": "rifle"}}})
	var kept []any
	json.Unmarshal(f["hi"], &kept)
	if len(kept) != 2 || b.ps.hp > 0 {
		t.Fatalf("haqiqiy tegish: %s hp=%v", f["hi"], b.ps.hp)
	}
	// B o'lishni "unutadi" — server avval buyuradi, keyin boshqalarga o'lik ko'rsatadi
	r.mu.Lock()
	r.tickLocked(time.Now().Add(800*time.Millisecond), .1)
	r.mu.Unlock()
	if ms := drain(b); len(ms) == 0 || ms[0]["t"] != "kill" || ms[0]["by"] != "a" {
		t.Fatalf("kill buyrug'i: %v", ms)
	}
	r.mu.Lock()
	r.tickLocked(time.Now().Add(1600*time.Millisecond), .1)
	r.mu.Unlock()
	if b.ps.alive {
		t.Fatal("server B ni o'ldirmadi")
	}
	f = base(b, "B", "CT", 43, 30, nil)
	if string(f["al"]) != "0" {
		t.Fatalf("o'lgan o'yinchi tirik ko'rsatilmoqda: al=%s", f["al"])
	}
	// raund: CT tomonida tirik qolmadi — T yutadi
	r.mu.Lock()
	r.tickLocked(time.Now(), .1)
	r.mu.Unlock()
	if r.gs.Ph != "post" || r.gs.W != "T" || r.gs.SA != 1 {
		t.Fatalf("raund natijasi: %+v", r.gs)
	}
}

func TestRefereePlant(t *testing.T) {
	r, a, b := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "live", Fm: 12, E: "x", Bm: -1, T: 100, liveAt: time.Now()}
	refPres(r, b, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 1, "al": 1, "x": 43, "y": 0, "z": 9})
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 75})
	site := vec3{16, 0, 18} // A nuqta
	a.ps.pos = site
	a.ps.push(sample{t: time.Now(), p: site})
	// endigina kelib, darhol "o'rnatdim" — rad etiladi
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 16, "y": 0, "z": 18, "pl": map[string]any{"n": 1, "x": 16, "z": 18, "s": "A", "r": 1}})
	if r.gs.B != nil {
		t.Fatal("bomba bir zumda o'rnatildi")
	}
	// 3 soniya shu joyda turgan
	for i := 0; i < 40; i++ {
		a.ps.push(sample{t: time.Now().Add(time.Duration(-3200+i*80) * time.Millisecond), p: site})
	}
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 16, "y": 0, "z": 18, "pl": map[string]any{"n": 2, "x": 16, "z": 18, "s": "A", "r": 1}})
	if r.gs.B == nil || r.gs.B.S != "A" {
		t.Fatalf("bomba o'rnatilmadi: %+v", r.gs)
	}
	// noto'g'ri nuqta nomi bilan — rad etiladi (B nuqtada emas)
	r.gs.B = nil
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 16, "y": 0, "z": 18, "pl": map[string]any{"n": 3, "x": 16, "z": 18, "s": "B", "r": 1}})
	if r.gs.B != nil {
		t.Fatal("noto'g'ri nuqta qabul qilindi")
	}
}

func TestRefereeDefuse(t *testing.T) {
	r, a, b := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "live", Fm: 12, E: "x", Bm: -1, T: 100, liveAt: time.Now().Add(-time.Minute), B: &bombState{X: 70, Z: 18, S: "B", T: 30}}
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 75})
	refPres(r, b, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 1, "al": 1, "x": 43, "y": 0, "z": 9})
	at := vec3{70.5, 0, 18}
	stand := func(ms int) {
		now := time.Now()
		for i := ms; i >= 0; i -= 45 { // 22 Hz holatlar, eskisidan yangisiga
			b.ps.push(sample{t: now.Add(-time.Duration(i) * time.Millisecond), p: at})
		}
		b.ps.pos = at
	}
	// 5 soniya bomba yonida — to'plamsiz (7 s) yetmaydi
	stand(5000)
	refPres(r, b, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 1, "al": 1, "x": 70.5, "y": 0, "z": 18, "df": map[string]any{"n": 1, "r": 1}})
	if r.gs.Ph != "live" {
		t.Fatal("zararsizlantirish juda tez qabul qilindi")
	}
	// xarid vaqti o'tgach "to'plam" paydo bo'lmaydi
	refPres(r, b, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 1, "al": 1, "x": 70.5, "y": 0, "z": 18, "kt": 1, "df": map[string]any{"n": 2, "r": 1}})
	if r.gs.Ph != "live" {
		t.Fatal("xarid vaqtidan keyin paydo bo'lgan to'plam qabul qilindi")
	}
	// 7 soniya turgan bo'lsa — qabul qilinadi
	stand(7300)
	refPres(r, b, map[string]any{"v": 1, "sq": "B", "tm": "CT", "in": 1, "al": 1, "x": 70.5, "y": 0, "z": 18, "df": map[string]any{"n": 3, "r": 1}})
	if r.gs.Ph != "post" || r.gs.W != "CT" || r.gs.Y != "defuse" {
		t.Fatalf("zararsizlantirish qabul qilinmadi: %+v", r.gs)
	}
}

func TestAStar(t *testing.T) {
	// T bazasidan A nuqtaga yo'l bor va har bir qadam bo'sh katakda
	gm := mapByID("qumtepa")
	p := gm.astar(vec3{43, 0, 75}, cellCenter([2]int{9, 8}))
	if len(p) < 10 {
		t.Fatalf("yo'l topilmadi: %d", len(p))
	}
	for _, s := range p {
		if gm.botSolid(cellOf(s)) {
			t.Fatalf("yo'l devordan o'tadi: %+v", s)
		}
	}
	if gm.astar(vec3{43, 0, 75}, vec3{1, 0, 1}) != nil {
		t.Fatal("devor ichiga yo'l topildi")
	}
}

func TestBots(t *testing.T) {
	r, a, _ := newRefRoom()
	delete(r.clients, "b")
	r.owner = a.id
	r.bans = map[string]time.Time{}
	h := &hub{maxPlayers: 64}
	// faqat ega bot qo'sha oladi
	other := &client{id: "z", room: r, send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
	r.clients["z"] = other
	h.adminLocked(r, other, json.RawMessage(`{"a":"bot","sq":"B"}`), time.Now())
	if bots, _ := r.botCountLocked(); bots != 0 {
		t.Fatal("ega bo'lmagan o'yinchi bot qo'shdi")
	}
	delete(r.clients, "z")
	h.adminLocked(r, a, json.RawMessage(`{"a":"bot","sq":"B","lv":"hard"}`), time.Now())
	var bot *client
	for _, c := range r.clients {
		if c.bot != nil {
			bot = c
		}
	}
	if bot == nil || bot.info.Sq != "B" || bot.bot.lv != "hard" {
		t.Fatal("bot qo'shilmadi")
	}
	// isinish: bot o'z bazasida tug'iladi
	now := time.Now()
	r.botStepLocked(bot, now, .05)
	if !bot.ps.alive || !mapByID("").inSpawn(bot.ps.pos, "CT") {
		t.Fatalf("bot tug'ilmadi: %+v", bot.ps.pos)
	}
	// odam bot ko'radigan joyda turadi: bot otadi va server hisobida jon kamayadi
	refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 0, "al": 1, "x": 43, "y": 0, "z": 75})
	a.ps.pos = vec3{43, 0, 22}
	a.ps.push(sample{t: now, p: a.ps.pos})
	bot.ps.pos = vec3{43, 0, 12}
	bot.bot.yaw = 0 // shimolga (pastdan yuqoriga) emas: a z=22 da, bot z=12 da — yawTo hisoblaydi
	bot.bot.yaw = yawTo(bot.ps.pos, a.ps.pos)
	hp0 := a.ps.hp
	for i := 0; i < 200 && a.ps.hp == hp0; i++ {
		now = now.Add(50 * time.Millisecond)
		bot.ps.pos = vec3{43, 0, 12}
		r.botStepLocked(bot, now, .05)
	}
	if a.ps.hp >= hp0 || len(bot.bot.hits) == 0 || bot.bot.sh == nil {
		t.Fatalf("bot otmadi: hp=%v hits=%d", a.ps.hp, len(bot.bot.hits))
	}
	// odam botni o'ldiradi (tasdiqlangan tegish) — bot o'ladi va o'limi e'lon qilinadi
	f := refPres(r, a, map[string]any{"v": 1, "sq": "A", "tm": "T", "in": 0, "al": 1, "x": 43, "y": 0, "z": 22, "w": "deagle",
		"hi": []any{map[string]any{"i": 1, "t": bot.id, "d": 224, "p": "h", "w": "deagle"}}})
	if string(f["hi"]) == "null" || bot.ps.alive || bot.bot.de == nil || bot.bot.de.By != a.id {
		t.Fatalf("bot o'lmadi: hi=%s alive=%v de=%+v", f["hi"], bot.ps.alive, bot.bot.de)
	}
	// ega botni chiqaradi
	h.adminLocked(r, a, json.RawMessage(`{"a":"kick","id":"`+bot.id+`"}`), now)
	if r.clients[bot.id] != nil {
		t.Fatal("bot chiqarilmadi")
	}
}

func TestChat(t *testing.T) {
	r, a, b := newRefRoom()
	mate := &client{id: "m", room: r, send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
	r.clients["m"] = mate
	for c, sq := range map[*client]string{a: "A", b: "B", mate: "A"} {
		refPres(r, c, map[string]any{"v": 1, "sq": sq, "tm": map[string]string{"A": "T", "B": "CT"}[sq], "nm": "O'yinchi " + c.id, "in": 0, "al": 1, "x": 43, "y": 0, "z": 75})
		drain(c)
	}
	now := time.Now()
	r.chatLocked(a, json.RawMessage(`{"m":"  salom\nhammaga  ","tm":0}`), now)
	if ms := drain(b); len(ms) != 1 || ms[0]["m"] != "salom hammaga" || ms[0]["nm"] != "O'yinchi a" || ms[0]["side"] != "T" {
		t.Fatalf("hammaga xabar: %v", ms)
	}
	drain(mate)
	if len(drain(a)) != 0 {
		t.Fatal("o'ziga qaytib keldi")
	}
	r.chatLocked(a, json.RawMessage(`{"m":"B ga boramiz","tm":1}`), now)
	if len(drain(b)) != 0 {
		t.Fatal("jamoa xabari raqibga yetib bordi")
	}
	if ms := drain(mate); len(ms) != 1 || ms[0]["tm"].(float64) != 1 {
		t.Fatalf("jamoadoshga: %v", ms)
	}
	// spam: ketma-ket ko'p xabar — ortig'i rad etiladi
	for i := 0; i < 10; i++ {
		r.chatLocked(a, json.RawMessage(`{"m":"spam","tm":0}`), now)
	}
	if n := len(drain(b)); n > 3 {
		t.Fatalf("spam cheklanmadi: %d", n)
	}
}
