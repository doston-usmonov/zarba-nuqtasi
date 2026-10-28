package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestBracket(t *testing.T) {
	if o := seedOrder(4); !reflect.DeepEqual(o, []int{1, 4, 2, 3}) {
		t.Fatalf("4: %v", o)
	}
	if o := seedOrder(8); !reflect.DeepEqual(o, []int{1, 8, 4, 5, 2, 7, 3, 6}) {
		t.Fatalf("8: %v", o)
	}
	// 4 lik turnir, 3 jamoa: 1-o'rin finalga o'zi chiqadi
	ms := newBracket(4, []int64{11, 22, 33})
	if ch := settle(ms, 4); ch != 0 {
		t.Fatal("chempion juda erta")
	}
	if m := ms[key(1, 0)]; m.W != 11 || !m.EB {
		t.Fatalf("bo'sh o'rin: %+v", m)
	}
	if m := ms[key(1, 1)]; m.A != 22 || m.B != 33 || m.W != 0 {
		t.Fatalf("2-3: %+v", m)
	}
	if f := ms[key(2, 0)]; f.A != 11 || f.B != 0 || f.W != 0 {
		t.Fatalf("final: %+v", f)
	}
	ms[key(1, 1)].W = 33
	settle(ms, 4)
	ms[key(2, 0)].W = 33
	if ch := settle(ms, 4); ch != 33 || ms[key(2, 0)].B != 33 {
		t.Fatalf("chempion: %d %+v", ch, ms[key(2, 0)])
	}
	// 8 lik, 3 jamoa: 4–5 juftligi bo'sh — 1-o'rin finalgacha o'z-o'zidan o'tadi
	ms = newBracket(8, []int64{1, 2, 3})
	settle(ms, 8)
	if m := ms[key(1, 1)]; !m.EA || !m.EB || m.W != 0 {
		t.Fatalf("bo'sh juftlik: %+v", m)
	}
	if m := ms[key(2, 0)]; m.W != 1 {
		t.Fatalf("1-o'rin yarim finalda o'tmadi: %+v", m)
	}
	if m := ms[key(2, 1)]; m.A != 2 || m.B != 3 || m.W != 0 {
		t.Fatalf("2-3: %+v", m)
	}
	if f := ms[key(3, 0)]; f.A != 1 || f.B != 0 {
		t.Fatalf("final: %+v", f)
	}
	// ikkita jamoa 8 lik turnirda — darhol final
	ms = newBracket(8, []int64{5, 6})
	settle(ms, 8)
	if f := ms[key(3, 0)]; f.A != 5 || f.B != 6 || f.W != 0 {
		t.Fatalf("2 jamoa: %+v", f)
	}
}

func TestTournamentFlow(t *testing.T) {
	srv, st, h := newTestServer(t)
	defer srv.Close()
	ctx := context.Background()
	var cs []*testClient
	var clans []int64
	for i := 0; i < 4; i++ {
		c := newTestClient(srv.URL)
		if code, _ := c.post(t, "/api/register", credentials{fmt.Sprintf("Sardor%d", i), "parol123"}); code != 201 {
			t.Fatal("ro'yxat")
		}
		cs = append(cs, c)
	}
	// 3 ta klan (egalari 0, 1, 2); 3-o'yinchi 0-klanga a'zo
	for i := 0; i < 3; i++ {
		if code, out := cs[i].post(t, "/api/clan", map[string]any{"action": "create", "name": fmt.Sprintf("Burgutlar %d", i), "tag": fmt.Sprintf("BG%d", i)}); code != 200 {
			t.Fatalf("klan: %d %v", code, out)
		}
		id, _, _ := st.userID(ctx, fmt.Sprintf("Sardor%d", i))
		clans = append(clans, st.clanIDOf(ctx, id))
	}
	cs[0].post(t, "/api/clan", map[string]any{"action": "invite", "name": "Sardor3"})
	_, soc := cs[3].get(t, "/api/social")
	inv, _ := soc["clanInvites"].([]any)
	if len(inv) > 0 {
		cs[3].post(t, "/api/clan", map[string]any{"action": "accept", "id": inv[0].(map[string]any)["id"]})
	}
	// turnir: 4 lik, Bozor
	code, d := cs[0].post(t, "/api/tournaments", map[string]any{"action": "create", "name": "Kuzgi kubok", "size": 4, "map": "bozor"})
	if code != 200 || d["state"] != "open" {
		t.Fatalf("turnir ochilmadi: %d %v", code, d)
	}
	tid := int64(d["id"].(float64))
	if code, _ := cs[0].post(t, "/api/tournaments", map[string]any{"action": "create", "name": "Ikkinchi", "size": 4, "map": "bozor"}); code != 400 {
		t.Fatal("tashkilotchi ikkinchi turnir ochdi")
	}
	if code, _ := cs[0].post(t, "/api/tournaments", map[string]any{"action": "create", "name": "Aim", "size": 4, "map": "maydon"}); code != 400 {
		t.Fatal("aim xaritasida turnir")
	}
	for i := 0; i < 3; i++ {
		if code, out := cs[i].post(t, "/api/tournaments", map[string]any{"action": "join", "id": tid}); code != 200 {
			t.Fatalf("yozilish %d: %d %v", i, code, out)
		}
	}
	if code, _ := cs[3].post(t, "/api/tournaments", map[string]any{"action": "join", "id": tid}); code != 403 {
		t.Fatal("klan egasi bo'lmagan yozildi")
	}
	if code, _ := cs[1].post(t, "/api/tournaments", map[string]any{"action": "start", "id": tid}); code != 403 {
		t.Fatal("tashkilotchi bo'lmagan boshladi")
	}
	code, d = cs[0].post(t, "/api/tournaments", map[string]any{"action": "start", "id": tid})
	if code != 200 || d["state"] != "running" {
		t.Fatalf("boshlanmadi: %d %v", code, d)
	}
	// 3 jamoa: bittasi finalga o'zi chiqadi, qolgan ikkitasi yarim finalda
	var semi map[string]any
	for _, m := range d["matches"].([]any) {
		mm := m.(map[string]any)
		if mm["room"] != nil {
			semi = mm
		}
	}
	if semi == nil || semi["round"] != float64(1) {
		t.Fatalf("o'ynaladigan o'yin: %v", d["matches"])
	}
	room := "zn-" + semi["room"].(string)
	sa, sb := int64(semi["a"].(map[string]any)["id"].(float64)), int64(semi["b"].(map[string]any)["id"].(float64))
	// xonaga: shu klanlar a'zolari o'z jamoasida, boshqalar — yo'q (kuzatsa bo'ladi)
	who := map[int64]*testClient{}
	for i := 0; i < 3; i++ {
		who[clans[i]] = cs[i]
	}
	wa := dialWS(t, who[sa], room)
	defer wa.c.Close()
	if m := wa.readUntil(t, func(m map[string]any) bool { return m["t"] == "side" }); m["sq"] != "A" {
		t.Fatalf("jamoa: %v", m)
	}
	outsider := cs[3]
	if sa == clans[0] || sb == clans[0] {
		outsider = newTestClient(srv.URL)
	}
	x := dialWS2(t, outsider, room)
	if m := x.read(t); m["code"] != "trn_only" && m["code"] != "mm_only" {
		t.Fatalf("begona kirdi: %v", m)
	}
	x.c.Close()
	sp := dialWS(t, outsider, room+"&spec=1")
	sp.c.Close()
	r := h.rooms[room]
	if r == nil || r.trn == nil || r.gs.M != "bozor" || !r.ownerless() {
		t.Fatalf("turnir xonasi: %+v", r)
	}
	// o'yin tugadi: server natijasi — A yutdi 9:4
	h.trnResult(r.trn, &matchResult{ScoreT: 9, ScoreCT: 4})
	_, d = cs[0].get(t, fmt.Sprintf("/api/tournaments/%d", tid))
	var final map[string]any
	for _, m := range d["matches"].([]any) {
		mm := m.(map[string]any)
		if mm["round"] == float64(2) {
			final = mm
		}
		if mm["round"] == float64(1) && mm["slot"] == semi["slot"] && (mm["winner"] != float64(sa) || mm["sA"] != float64(9)) {
			t.Fatalf("yarim final natijasi: %v", mm)
		}
	}
	if final == nil || final["room"] == nil || (final["a"].(map[string]any)["id"] != float64(sa) && final["b"].(map[string]any)["id"] != float64(sa)) {
		t.Fatalf("finalga o'tmadi: %v", final)
	}
	// tashkilotchi finalda texnik g'alaba beradi
	fb := int64(final["b"].(map[string]any)["id"].(float64))
	if code, _ := cs[1].post(t, "/api/tournaments", map[string]any{"action": "result", "id": tid, "round": 2, "slot": 0, "winner": fb}); code != 403 {
		t.Fatal("begona natija belgiladi")
	}
	if code, d := cs[0].post(t, "/api/tournaments", map[string]any{"action": "result", "id": tid, "round": 2, "slot": 0, "winner": fb}); code != 200 || d["state"] != "done" || d["winner"] == "" {
		t.Fatalf("final: %d %v", code, d)
	}
	_, list := cs[2].get(t, "/api/tournaments")
	if ts := list["tournaments"].([]any); len(ts) != 1 || ts[0].(map[string]any)["state"] != "done" {
		t.Fatalf("ro'yxat: %v", list)
	}
	// tugagan o'yin xonasiga yangi xona ochilmaydi
	y := dialWS2(t, who[sa], "zn-"+trnRoomCode(tid, 2, 0))
	if m := y.read(t); m["code"] != "trn_closed" {
		t.Fatalf("tugagan o'yin: %v", m)
	}
	y.c.Close()
	_ = time.Second
}
