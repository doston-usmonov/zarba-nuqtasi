package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func mmTest() (*matchmaker, *fakeClock, *[]map[int64]string) {
	q, clk, opened := newMatchmaker(), &fakeClock{t: time.Unix(1_800_000_000, 0)}, &[]map[int64]string{}
	q.now = clk.now
	q.open = func(mode, gmap string, sq map[int64]string) string {
		*opened = append(*opened, sq)
		return fmt.Sprintf("mm-%d", len(*opened))
	}
	return q, clk, opened
}

func pl(id int64, rating int) qMember {
	return qMember{id: id, name: fmt.Sprintf("p%d", id), rating: rating}
}

// TestMatchmaker: reyting oralig'i, kengayishi, jamoalarni bo'lish, partiyalar, chiqib ketish.
func TestMatchmaker(t *testing.T) {
	// 4 ta yaqin reytingli o'yinchi — Wingman darhol topiladi, jamoalar teng
	q, clk, opened := mmTest()
	for i, r := range []int{1000, 1100, 1050, 1120} {
		if err := q.join(pl(int64(i+1), r), modeWM, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(*opened) != 1 {
		t.Fatalf("o'yin ochilmadi: %d", len(*opened))
	}
	sq := (*opened)[0]
	sum := map[string]int{}
	for id, s := range sq {
		sum[s] += []int{1000, 1100, 1050, 1120}[id-1]
	}
	if len(sq) != 4 || sum["A"]+sum["B"] != 4270 || abs(sum["A"]-sum["B"]) > 50 {
		t.Fatalf("jamoalar: %v %v", sq, sum)
	}
	if st := q.status(1); st.State != "found" || st.Found.Room != "mm-1" || st.Found.Sq != sq[1] || st.Found.Mode != modeWM {
		t.Fatalf("holat: %+v", st)
	}

	// reyting farqi katta — boshida kutadi, vaqt o'tgach farq kengayib topiladi
	q, clk, opened = mmTest()
	for i, r := range []int{1000, 1010, 1020, 1400} {
		q.join(pl(int64(i+1), r), modeWM, nil)
	}
	if len(*opened) != 0 {
		t.Fatal("±150 dan katta farq bilan o'yin ochildi")
	}
	st := q.status(4)
	if st.State != "searching" || st.Range != 150 || st.InQueue[modeWM] != 4 {
		t.Fatalf("navbat holati: %+v", st)
	}
	for i := 0; i < 16; i++ { // 160 soniya: farq 150 + 8×50 = 550 (har kim har 10 soniyada so'raydi)
		clk.add(10 * time.Second)
		for id := int64(1); id <= 4; id++ {
			q.status(id)
		}
	}
	if len(*opened) != 1 {
		t.Fatalf("farq kengaygach ham topilmadi (%d)", len(*opened))
	}

	// partiya: boshliq do'stini chaqiradi, qabul qilsa birga turadi va bitta jamoaga tushadi
	q, _, opened = mmTest()
	if err := q.join(pl(1, 1000), modeWM, []qMember{pl(2, 0)}); err != nil {
		t.Fatal(err)
	}
	if st := q.status(2); st.Invite == nil || st.Invite.From != "p1" {
		t.Fatalf("chaqiriq kelmadi: %+v", st)
	}
	if err := q.accept(pl(2, 1080)); err != nil {
		t.Fatal(err)
	}
	if st := q.status(2); st.State != "searching" || len(st.Party) != 2 || st.Leader {
		t.Fatalf("partiya: %+v", st)
	}
	if err := q.join(pl(5, 1000), modeWM, []qMember{pl(6, 0), pl(7, 0)}); err != errPartyBig {
		t.Fatalf("Wingman'da 3 kishilik partiya: %v", err)
	}
	q.join(pl(3, 1020), modeWM, nil)
	q.join(pl(4, 1060), modeWM, nil)
	if len(*opened) != 1 || (*opened)[0][1] != (*opened)[0][2] || (*opened)[0][3] != (*opened)[0][4] {
		t.Fatalf("partiya bitta jamoada emas: %v", *opened)
	}

	// partiyalarni buzib bo'lmasa (Raqobat: 4+4+2) — kutadi
	q, _, opened = mmTest()
	party := func(leader int64, n int) {
		var inv []qMember
		for i := 1; i < n; i++ {
			inv = append(inv, pl(leader+int64(i), 0))
		}
		q.join(pl(leader, 1000), modeComp, inv)
		for i := 1; i < n; i++ {
			q.accept(pl(leader+int64(i), 1000))
		}
	}
	party(10, 4)
	party(20, 4)
	party(30, 2)
	if len(*opened) != 0 {
		t.Fatal("4+4+2 ikki teng jamoaga bo'linmaydi, lekin o'yin ochildi")
	}
	party(40, 1)
	party(50, 1) // 4+4+1+1 = 5+5
	if len(*opened) != 1 {
		t.Fatalf("4+4+1+1 topilmadi: %d", len(*opened))
	}
	if st := q.status(30); st.State != "searching" {
		t.Fatalf("2 kishilik partiya navbatda qolishi kerak: %+v", st)
	}

	// boshliq chiqsa partiya chiqadi; so'ramay qo'ygan o'yinchi navbatdan tushadi
	q.leave(30)
	if st := q.status(31); st.State != "idle" {
		t.Fatalf("boshliq chiqdi, a'zo hali navbatda: %+v", st)
	}
	q, clk, _ = mmTest()
	q.join(pl(1, 1000), modeComp, nil)
	q.join(pl(2, 1000), modeComp, nil)
	clk.add(10 * time.Second)
	q.status(2)
	clk.add(10 * time.Second)
	if st := q.status(2); st.InQueue[modeComp] != 1 {
		t.Fatalf("yopilgan brauzer navbatda qoldi: %+v", st)
	}
	if err := q.join(pl(3, 1000), "dm", nil); err != errQueueMode {
		t.Fatalf("deathmatch navbati: %v", err)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestQueueAPI: to'rt hisob Wingman navbatiga turadi → yopiq mm xonasi, faqat ular kiradi, egasi yo'q.
func TestQueueAPI(t *testing.T) {
	srv, _, h := newTestServer(t)
	defer srv.Close()
	var cs []*testClient
	for i := 0; i < 5; i++ {
		c := newTestClient(srv.URL)
		if code, out := c.post(t, "/api/register", credentials{fmt.Sprintf("Navbat%d", i), "parol123"}); code != 201 {
			t.Fatalf("ro'yxat: %d %v", code, out)
		}
		cs = append(cs, c)
	}
	guest := newTestClient(srv.URL)
	if code, _ := guest.post(t, "/api/queue", map[string]any{"action": "join", "mode": "wm"}); code != 401 {
		t.Fatalf("mehmon navbatga turdi: %d", code)
	}
	if code, out := cs[0].post(t, "/api/queue", map[string]any{"action": "join", "mode": "dm"}); code != 400 {
		t.Fatalf("deathmatch navbati: %d %v", code, out)
	}
	for i := 0; i < 4; i++ {
		code, out := cs[i].post(t, "/api/queue", map[string]any{"action": "join", "mode": "wm"})
		if code != 200 {
			t.Fatalf("navbat %d: %d %v", i, code, out)
		}
	}
	_, st := cs[0].get(t, "/api/queue")
	f, _ := st["found"].(map[string]any)
	if st["state"] != "found" || f == nil {
		t.Fatalf("o'yin topilmadi: %v", st)
	}
	code := f["room"].(string)
	r := h.rooms["zn-"+code]
	if r == nil || r.pub || r.mm == nil || r.max != 4 || r.gs.modeID() != "wm" || r.gs.M != f["map"] {
		t.Fatalf("mm xonasi: %+v", r)
	}
	for _, l := range h.publicRooms() {
		if l.Code == code {
			t.Fatal("mm xonasi ommaviy ro'yxatda")
		}
	}
	// navbatga turmagan hisob kira olmaydi
	x := dialWS2(t, cs[4], "zn-"+code)
	if m := x.read(t); m["t"] != "err" || m["code"] != "mm_only" {
		t.Fatalf("begona kirdi: %v", m)
	}
	x.c.Close()
	// topilgan o'yinchi kiradi; egasi yo'q, bot qo'shib bo'lmaydi
	w := dialWS(t, cs[0], "zn-"+code)
	defer w.c.Close()
	w.send(map[string]any{"t": "adm", "d": map[string]any{"a": "bot", "sq": "A"}})
	time.Sleep(150 * time.Millisecond)
	r.mu.Lock()
	bots, humans := r.botCountLocked()
	owner := r.owner
	r.mu.Unlock()
	if bots != 0 || humans != 1 || owner != "" {
		t.Fatalf("mm xonasi: bot %d, odam %d, ega %q", bots, humans, owner)
	}
	// jamoani server belgilaydi
	want := f["sq"].(string)
	other := map[string]string{"A": "B", "B": "A"}[want]
	w.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "hc": 1, "sq": other, "tm": "T", "nm": "x"}})
	time.Sleep(150 * time.Millisecond)
	r.mu.Lock()
	var got string
	for _, c := range r.clients {
		got = c.info.Sq
	}
	r.mu.Unlock()
	if got != want {
		t.Fatalf("jamoa: %q, kerak %q", got, want)
	}
	// (o'yin tugadi deb hisoblaymiz: kelmagan o'yinchilar jazolanmaydi — bu TestLeaverPenalty'da)
	r.mu.Lock()
	r.mm.done, r.mm.absent = true, map[int64]time.Time{}
	r.mu.Unlock()
	// partiya chaqirig'i /api/social da ham keladi
	cs[4].post(t, "/api/friends", map[string]any{"action": "add", "name": "Navbat1"})
	cs[1].post(t, "/api/friends", map[string]any{"action": "add", "name": "Navbat4"})
	cs[1].post(t, "/api/queue", map[string]any{"action": "leave"})
	if code, out := cs[1].post(t, "/api/queue", map[string]any{"action": "join", "mode": "comp", "party": []string{"Navbat4"}}); code != 200 {
		t.Fatalf("partiya bilan navbat: %d %v", code, out)
	}
	_, soc := cs[4].get(t, "/api/social")
	if p, _ := soc["party"].(map[string]any); p == nil || p["from"] != "Navbat1" || p["mode"] != "comp" {
		t.Fatalf("partiya chaqirig'i: %v", soc["party"])
	}
	if code, out := cs[4].post(t, "/api/queue", map[string]any{"action": "accept"}); code != 200 || out["state"] != "searching" {
		t.Fatalf("qabul: %d %v", code, out)
	}
	if code, _ := cs[2].post(t, "/api/queue", map[string]any{"action": "join", "mode": "comp", "party": []string{"Navbat3"}}); code != 400 {
		t.Fatal("do'st bo'lmagan o'yinchini partiyaga chaqirib bo'ldi")
	}
}

// TestWingmanRated: Wingman o'yinlari ham reyting kuzatuvchisiga tushadi (navbatdagi Wingman reytingli).
func TestWingmanRated(t *testing.T) {
	r, _, _ := newRefRoom()
	r.gs.Md, r.gs.E, r.gs.Ph, r.gs.R = modeWM, "w1", "freeze", 1
	r.tr.phase(r, "warmup", time.Now())
	if !r.tr.active || r.tr.epoch != "w1" {
		t.Fatalf("Wingman o'yini kuzatilmayapti: %+v", r.tr)
	}
}

// TestLeaverPenalty: navbat o'yiniga kelmagan yoki chiqib qaytmagan o'yinchi navbatga vaqtincha kira olmaydi.
func TestLeaverPenalty(t *testing.T) {
	srv, st, h := newTestServer(t)
	defer srv.Close()
	h.leaverGrace = 400 * time.Millisecond
	var cs []*testClient
	for i := 0; i < 4; i++ {
		c := newTestClient(srv.URL)
		if code, out := c.post(t, "/api/register", credentials{fmt.Sprintf("Qochoq%d", i), "parol123"}); code != 201 {
			t.Fatalf("ro'yxat: %d %v", code, out)
		}
		c.post(t, "/api/queue", map[string]any{"action": "join", "mode": "wm"})
		cs = append(cs, c)
	}
	_, s0 := cs[0].get(t, "/api/queue")
	f, _ := s0["found"].(map[string]any)
	if f == nil {
		t.Fatalf("o'yin topilmadi: %v", s0)
	}
	room := "zn-" + f["room"].(string)
	// 0, 1, 2 kiradi; 3 kelmaydi
	var ws []*wsTest
	for i := 0; i < 3; i++ {
		ws = append(ws, dialWS(t, cs[i], room))
	}
	defer func() {
		for _, w := range ws {
			w.c.Close()
		}
	}()
	time.Sleep(900 * time.Millisecond)
	if code, out := cs[3].post(t, "/api/queue", map[string]any{"action": "join", "mode": "wm"}); code != 403 {
		t.Fatalf("kelmagan o'yinchi navbatga turdi: %d %v", code, out)
	}
	if _, q := cs[3].get(t, "/api/queue"); q["ban"] == nil || q["ban"].(float64) < 250 || q["ban"].(float64) > 301 {
		t.Fatalf("taqiq 5 daqiqa bo'lishi kerak: %v", q["ban"])
	}
	for i := 0; i < 3; i++ {
		if _, q := cs[i].get(t, "/api/queue"); q["ban"] != nil {
			t.Fatalf("xonadagi o'yinchi %d jazolandi: %v", i, q)
		}
	}
	// 0 chiqadi va vaqtida qaytadi — jazo yo'q
	ws[0].c.Close()
	time.Sleep(150 * time.Millisecond)
	_, q := cs[0].get(t, "/api/queue")
	rj, _ := q["rejoin"].(map[string]any)
	if rj == nil || rj["room"] != f["room"] || rj["sq"] != f["sq"] {
		t.Fatalf("qayta ulanish taklifi yo'q: %v", q)
	}
	if code, _ := cs[0].post(t, "/api/queue", map[string]any{"action": "join", "mode": "comp"}); code != 400 {
		t.Fatal("tashlab chiqqan o'yinchi yangi navbatga turdi")
	}
	ws[0] = dialWS(t, cs[0], room)
	time.Sleep(600 * time.Millisecond)
	if _, q := cs[0].get(t, "/api/queue"); q["ban"] != nil || q["rejoin"] != nil {
		t.Fatalf("qaytgan o'yinchi jazolandi: %v", q)
	}
	// 1 chiqib qaytmaydi — jazo
	ws[1].c.Close()
	time.Sleep(700 * time.Millisecond)
	if _, q := cs[1].get(t, "/api/queue"); q["ban"] == nil {
		t.Fatalf("qaytmagan o'yinchi jazolanmadi: %v", q)
	}
	// o'yin tugagach chiqish jazolanmaydi
	r := h.rooms[room]
	r.mu.Lock()
	r.gs.Ph = "over"
	r.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	ws[2].c.Close()
	time.Sleep(700 * time.Millisecond)
	if _, q := cs[2].get(t, "/api/queue"); q["ban"] != nil {
		t.Fatalf("o'yin tugagach chiqqan o'yinchi jazolandi: %v", q)
	}
	// zinapoya: 5 daqiqa, 30 daqiqa, 2 soat; 24 soatdan keyin boshidan
	_, out := cs[3].get(t, "/api/me")
	_ = out
	ctx := context.Background()
	id, _, _ := st.userID(ctx, "Qochoq2")
	now := time.Now()
	for i, want := range []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 2 * time.Hour} {
		until, n, err := st.AddLeave(ctx, id, now.Add(time.Duration(i)*time.Minute))
		if err != nil || n != i+1 || until.Sub(now.Add(time.Duration(i)*time.Minute)) != want {
			t.Fatalf("%d-marta: %v %d %v", i+1, until, n, err)
		}
	}
	if _, n, _ := st.AddLeave(ctx, id, now.Add(26*time.Hour)); n != 1 {
		t.Fatalf("24 soatdan keyin hisob boshidan boshlanmadi: %d", n)
	}
}

func TestQueueMaps(t *testing.T) {
	ids := queueMaps()
	if len(ids) < 5 {
		t.Fatalf("xaritalar: %v", ids)
	}
	for _, id := range ids {
		if isAimMap(id) {
			t.Fatalf("navbatda aim xaritasi: %s", id)
		}
	}
}
