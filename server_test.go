package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
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

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Register("ab", "secret1"); err != errBadUsername {
		t.Fatalf("qisqa login qabul qilinmasligi kerak: %v", err)
	}
	if _, _, err := s.Register("Ali", "123"); err != errBadPassword {
		t.Fatalf("qisqa parol qabul qilinmasligi kerak: %v", err)
	}
	u, tok, err := s.Register("Ali_01", "secret1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Register("ali_01", "boshqa1"); err != errTaken {
		t.Fatalf("katta-kichik harf farqi bilan ham band: %v", err)
	}
	if _, _, err := s.Login("ALI_01", "wrong-pass"); err != errBadLogin {
		t.Fatalf("noto'g'ri parol: %v", err)
	}
	if _, _, err := s.Login("ali_01", "secret1"); err != nil {
		t.Fatalf("to'g'ri parol: %v", err)
	}
	res := &matchResult{Room: "zn-x", Started: time.Now().Add(-5 * time.Minute), Ended: time.Now(), ScoreT: 8, ScoreCT: 3, Winner: "T",
		Players: []resultPlayer{{UserID: u.ID, Name: "Ali_01", Team: "T", Kills: 9, Deaths: 4, HS: 3}, {Name: "Mehmon", Team: "CT", Kills: 4, Deaths: 9}}}
	ch, err := s.ApplyMatch(res)
	if err != nil || ch[u.ID].Delta <= 0 {
		t.Fatalf("g'olib reytingi oshishi kerak: %+v %v", ch, err)
	}
	s2, err := openStore(dir) // qayta ochish: hammasi diskdan tiklanishi kerak
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Session(tok)
	if !ok || got.Username != "Ali_01" || got.Rating != ch[u.ID].After || got.Kills != 9 || got.HS != 3 || got.Matches != 1 {
		t.Fatalf("qayta ochilganda holat tiklanmadi: %+v ok=%v", got, ok)
	}
	if _, pos, hist, ok := s2.Profile("ali_01"); !ok || pos != 1 || len(hist) != 1 || hist[0].Players[1].UserID != 0 {
		t.Fatalf("profil/tarix: pos=%d hist=%d", pos, len(hist))
	}
}

// ---- to'liq oqim: HTTP API + WebSocket orqali o'yin ----

func newTestServer(t *testing.T) (*httptest.Server, *Store, *hub) {
	st, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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

	g := func(ph string, r, sT, sCT int) map[string]any {
		return map[string]any{"r": r, "ph": ph, "sT": sT, "sCT": sCT, "e": "m1"}
	}
	pres := func(w *wsTest, nm, tm string, in int, gs map[string]any, de any) {
		w.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "hc": 1, "nm": nm, "tm": tm, "in": in, "g": gs, "de": de}})
	}
	// uchala mijoz ham bir xil g yuboradi — kim xost bo'lishidan qat'i nazar
	all := func(gs map[string]any, in int, deB any) {
		pres(wa, "Yolg'onchi", "T", in, gs, nil)
		pres(wb, "Bek", "CT", in, gs, deB)
		pres(wg, "Mehmon1", "CT", in, gs, nil)
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
	for r := 2; r <= 9; r++ {
		all(g("live", r, r-1, 0), r, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1})
	}
	all(g("over", 9, 8, 1), 9, map[string]any{"n": 1, "by": wa.id, "w": "rifle", "hs": 1})

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
