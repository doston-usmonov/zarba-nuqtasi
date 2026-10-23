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
		h.serve(ws, r.URL.Query().Get("room"), r.RemoteAddr+newID(), acct)
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
	c  net.Conn
	br *bufio.Reader
	id string
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
			w.id = m["id"].(string)
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

	// qisqa format (fm=8): 8 raunddan keyin tomonlar almashadi, 9 g'alaba kerak
	g := func(ph string, r, sA, sB int) map[string]any {
		return map[string]any{"r": r, "ph": ph, "sA": sA, "sB": sB, "fm": 8, "e": "m1"}
	}
	pres := func(w *wsTest, nm, sq, tm string, in int, gs map[string]any, de any) {
		w.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "hc": 1, "nm": nm, "sq": sq, "tm": tm, "in": in, "g": gs, "de": de}})
	}
	// uchala mijoz ham bir xil g yuboradi — kim xost bo'lishidan qat'i nazar
	all := func(gs map[string]any, in int, deB any) {
		tA, tB := "T", "CT"
		if in > 8 { // ikkinchi yarim
			tA, tB = "CT", "T"
		}
		pres(wa, "Yolg'onchi", "A", tA, in, gs, nil)
		pres(wb, "Bek", "B", tB, in, gs, deB)
		pres(wg, "Mehmon1", "B", tB, in, gs, nil)
		time.Sleep(30 * time.Millisecond)
	}
	all(g("warmup", 0, 0, 0), 0, nil)
	// boshqalar Ali'ning ismini server yozgan holda ko'radi, "acc" bilan
	upd := wb.readUntil(t, func(m map[string]any) bool { return m["t"] == "u" && m["id"] == wa.id })
	p := upd["p"].(map[string]any)
	if p["nm"] != "Ali" || p["acc"].(map[string]any)["r"].(float64) != 1000 {
		t.Fatalf("server belgilagan nom/reyting: %v", p)
	}
	all(g("freeze", 1, 0, 0), 1, nil)
	// test 9 raundni bir zumda "o'ynaydi": boshlanish vaqtini orqaga surib, "juda tez o'yin" tekshiruvidan o'tkazamiz
	h.mu.Lock()
	rm := h.rooms[room]
	h.mu.Unlock()
	rm.mu.Lock()
	if !rm.tr.active || rm.tr.epoch != "m1" {
		t.Fatalf("o'yin boshlanishi kuzatilmadi: %+v", rm.tr)
	}
	rm.tr.started = rm.tr.started.Add(-10 * time.Minute)
	rm.mu.Unlock()
	// Bek o'ladi: Ali o'ldirdi, boshga
	all(g("live", 1, 0, 0), 1, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1})
	all(g("live", 1, 0, 0), 1, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1}) // takror — bir marta hisoblanadi
	// mehmon o'zini "Ali'ni o'ldirdim" deb e'lon qiladi — hisobga olinmaydi (faqat qurbonning xabari ishonchli)
	for r := 2; r <= 10; r++ {
		all(g("live", r, r-1, min(r-1, 1)), r, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1})
	}
	all(g("over", 10, 9, 1), 10, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1})

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
