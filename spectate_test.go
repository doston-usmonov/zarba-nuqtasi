package main

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSpectators: kuzatuvchi xonani ko'radi, o'yinchilar uni ko'rmaydi; to'la xonani ham kuzatsa bo'ladi;
// uning chati faqat kuzatuvchilarga; o'yinchilar chiqsa xona yopiladi.
func TestSpectators(t *testing.T) {
	h := newHub(16, nil)
	srv, _ := newPlainServer(t, h)
	tc := newTestClient(srv.URL)
	// bo'lmagan xonani kuzatib bo'lmaydi
	x := dialWS2(t, tc, "zn-yoq&spec=1")
	if m := x.read(t); m["t"] != "err" || m["code"] != "no_room" {
		t.Fatalf("bo'lmagan xona: %v", m)
	}
	x.c.Close()
	a := dialWS(t, tc, "zn-kuz&create=1&max=2&map=bozor&mode=comp")
	defer a.c.Close()
	b := dialWS(t, tc, "zn-kuz")
	defer b.c.Close()
	a.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "nm": "Ali", "al": 1, "x": 10, "y": 0, "z": 70}})
	time.Sleep(100 * time.Millisecond)
	// xona to'la (2/2) — o'yinchi kira olmaydi, kuzatuvchi kira oladi
	c := dialWS2(t, tc, "zn-kuz")
	if m := c.read(t); m["code"] != "room_full" {
		t.Fatalf("to'la xonaga kirdi: %v", m)
	}
	c.c.Close()
	s := dialWS(t, tc, "zn-kuz&spec=1")
	defer s.c.Close()
	snap := s.readUntil(t, func(m map[string]any) bool { return m["t"] == "s" })
	if ps, _ := snap["peers"].([]any); len(ps) != 1 {
		t.Fatalf("kuzatuvchi o'yinchini ko'rmadi: %v", snap)
	}
	s.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "sq": "A", "tm": "T", "nm": "Ayg'oqchi", "al": 1}})
	s2 := dialWS(t, tc, "zn-kuz&spec=1")
	defer s2.c.Close()
	r := h.rooms["zn-kuz"]
	r.mu.Lock()
	bots, humans := r.botCountLocked()
	lst := r.listingLocked()
	var sc *client
	for _, cl := range r.clients {
		if cl.id == s.id {
			sc = cl
		}
	}
	r.mu.Unlock()
	if humans != 2 || bots != 0 || lst.Spec != 2 || sc == nil || sc.info.Sq != "" || sc.info.Nm != "Ayg'oqchi" || sc.ps.alive {
		t.Fatalf("kuzatuvchi o'yinchi bo'lib qoldi: odam %d, kuzatuvchi %d, %+v", humans, lst.Spec, sc)
	}
	// o'yinchilar kuzatuvchining holatini olmaydi
	s.send(map[string]any{"t": "chat", "d": map[string]any{"m": "B ga borishyapti"}})
	a.send(map[string]any{"t": "chat", "d": map[string]any{"m": "salom"}})
	got := s2.readUntil(t, func(m map[string]any) bool { return m["t"] == "chat" && m["m"] == "B ga borishyapti" })
	if got["spec"] != float64(1) {
		t.Fatalf("kuzatuvchi belgisi: %v", got)
	}
	s.readUntil(t, func(m map[string]any) bool { return m["t"] == "chat" && m["m"] == "salom" })
	b.c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for i := 0; i < 100; i++ {
		m, ok := tryRead(b)
		if !ok {
			break
		}
		if m["t"] == "chat" && m["m"] == "B ga borishyapti" {
			t.Fatal("kuzatuvchining xabari o'yinchiga bordi")
		}
		if m["t"] == "u" && m["id"] == s.id {
			t.Fatal("kuzatuvchining holati o'yinchiga bordi")
		}
	}
	// o'yinchilar chiqdi — xona yopiladi, kuzatuvchiga xabar
	a.c.Close()
	b.c.Close()
	s.readUntil(t, func(m map[string]any) bool { return m["t"] == "err" && m["code"] == "room_closed" })
	time.Sleep(500 * time.Millisecond)
	h.mu.Lock()
	_, still := h.rooms["zn-kuz"]
	h.mu.Unlock()
	if still {
		t.Fatal("xona yopilmadi")
	}
	// navbat o'yinini kuzatib bo'lmaydi
	code := h.openMM(modeWM, "bozor", map[int64]string{1: "A", 2: "B"})
	m := dialWS2(t, tc, "zn-"+code+"&spec=1")
	if r := m.read(t); r["code"] != "mm_spec" {
		t.Fatalf("navbat o'yinini kuzatdi: %v", r)
	}
	m.c.Close()
}

// tryRead: muddat tugaguncha xabar o'qiydi (yo'q bo'lsa false).
func tryRead(w *wsTest) (map[string]any, bool) {
	var h [2]byte
	if _, err := w.br.Read(h[:1]); err != nil {
		return nil, false
	}
	if _, err := w.br.Read(h[1:2]); err != nil {
		return nil, false
	}
	n := int(h[1] & 0x7f)
	if n == 126 {
		var b [2]byte
		w.br.Read(b[:1])
		w.br.Read(b[1:])
		n = int(b[0])<<8 | int(b[1])
	}
	p := make([]byte, n)
	for got := 0; got < n; {
		k, err := w.br.Read(p[got:])
		if err != nil {
			return nil, false
		}
		got += k
	}
	if h[0]&0x0f != opText {
		return map[string]any{}, true
	}
	var m map[string]any
	json.Unmarshal(p, &m)
	return m, true
}
