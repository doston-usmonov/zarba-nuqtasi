package main

import (
	"context"
	"testing"
	"time"
)

func TestFriendsAndClans(t *testing.T) {
	s, ctx := testStore(t, ""), context.Background()
	ali, _, _ := s.Register(ctx, "Ali", "secret12")
	vali, _, _ := s.Register(ctx, "Vali", "secret12")
	guli, _, _ := s.Register(ctx, "Guli", "secret12")

	if err := s.FriendAction(ctx, ali.ID, "add", "ali"); err != errSelf {
		t.Fatal("o'zini do'st qilish:", err)
	}
	if err := s.FriendAction(ctx, ali.ID, "add", "yo'q"); err != errNoUser {
		t.Fatal(err)
	}
	if err := s.FriendAction(ctx, ali.ID, "add", "vali"); err != nil {
		t.Fatal(err)
	}
	so, _ := s.Social(ctx, vali.ID)
	if len(so.Incoming) != 1 || so.Incoming[0] != "Ali" || len(so.Friends) != 0 {
		t.Fatalf("kelgan so'rov: %+v", so)
	}
	so, _ = s.Social(ctx, ali.ID)
	if len(so.Outgoing) != 1 {
		t.Fatalf("yuborilgan so'rov: %+v", so)
	}
	// qarshi so'rov = qabul qilish
	if err := s.FriendAction(ctx, vali.ID, "add", "Ali"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{ali.ID, vali.ID} {
		so, _ = s.Social(ctx, id)
		if len(so.Friends) != 1 || len(so.Incoming)+len(so.Outgoing) != 0 {
			t.Fatalf("do'stlik ikki tomonlama emas: %+v", so)
		}
	}
	// Guli so'raydi, Ali rad etadi
	s.FriendAction(ctx, guli.ID, "add", "Ali")
	if err := s.FriendAction(ctx, ali.ID, "decline", "Guli"); err != nil {
		t.Fatal(err)
	}
	if so, _ = s.Social(ctx, ali.ID); len(so.Incoming) != 0 {
		t.Fatal("rad etilgan so'rov qoldi")
	}

	// klan
	if err := s.ClanAction(ctx, ali.ID, "create", "x", "AB", 0); err != errClanName {
		t.Fatal("qisqa nom:", err)
	}
	if err := s.ClanAction(ctx, ali.ID, "create", "Burgut  Ko'zi", "brk", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.ClanAction(ctx, guli.ID, "create", "burgut ko'zi", "ZZ", 0); err != errClanTaken {
		t.Fatal("band nom:", err)
	}
	if err := s.ClanAction(ctx, guli.ID, "create", "Boshqa", "BRK", 0); err != errClanTaken {
		t.Fatal("band teg:", err)
	}
	if tag := s.ClanTag(ctx, ali.ID); tag != "BRK" {
		t.Fatal("teg:", tag)
	}
	if err := s.ClanAction(ctx, ali.ID, "invite", "Guli", "", 0); err != errNotFriends {
		t.Fatal("do'st bo'lmaganni taklif:", err)
	}
	if err := s.ClanAction(ctx, ali.ID, "invite", "Vali", "", 0); err != nil {
		t.Fatal(err)
	}
	so, _ = s.Social(ctx, vali.ID)
	if len(so.ClanInvites) != 1 || so.ClanInvites[0].Tag != "BRK" || so.ClanInvites[0].From != "Ali" {
		t.Fatalf("klan taklifi: %+v", so.ClanInvites)
	}
	if err := s.ClanAction(ctx, vali.ID, "accept", "", "", so.ClanInvites[0].ID); err != nil {
		t.Fatal(err)
	}
	so, _ = s.Social(ctx, ali.ID)
	if so.Clan == nil || len(so.Clan.Members) != 2 || !so.Clan.Owner || so.Clan.Name != "Burgut Ko'zi" {
		t.Fatalf("klan: %+v", so.Clan)
	}
	if err := s.ClanAction(ctx, vali.ID, "kick", "Ali", "", 0); err != errNotOwner {
		t.Fatal("oddiy a'zo chiqara olmaydi:", err)
	}
	list, _ := s.Clans(ctx, 10)
	if len(list) != 1 || list[0].Members != 2 || list[0].Pos != 1 {
		t.Fatalf("klanlar reytingi: %+v", list)
	}
	// egasi ketsa — Vali ega bo'ladi; Vali ham ketsa — klan o'chadi
	if err := s.ClanAction(ctx, ali.ID, "leave", "", "", 0); err != nil {
		t.Fatal(err)
	}
	if so, _ = s.Social(ctx, vali.ID); so.Clan == nil || !so.Clan.Owner || len(so.Clan.Members) != 1 {
		t.Fatalf("egalik o'tmadi: %+v", so.Clan)
	}
	s.ClanAction(ctx, vali.ID, "leave", "", "", 0)
	if list, _ = s.Clans(ctx, 10); len(list) != 0 {
		t.Fatal("bo'sh klan qoldi")
	}
	// do'stlikni bekor qilish
	s.FriendAction(ctx, vali.ID, "remove", "Ali")
	if so, _ = s.Social(ctx, ali.ID); len(so.Friends) != 0 {
		t.Fatal("do'st o'chmadi")
	}
}

// Ikki do'st: biri xonada, ikkinchisi menyuda — onlayn holati, xonaga chaqiriq, klan tegi.
func TestSocialOnline(t *testing.T) {
	srv, st, _ := newTestServer(t)
	defer srv.Close()
	a, b := newTestClient(srv.URL), newTestClient(srv.URL)
	a.post(t, "/api/register", map[string]string{"username": "Ali", "password": "secret12"})
	b.post(t, "/api/register", map[string]string{"username": "Vali", "password": "secret12"})
	if code, _ := a.post(t, "/api/friends", map[string]string{"action": "add", "name": "Vali"}); code != 200 {
		t.Fatal(code)
	}
	if code, _ := b.post(t, "/api/friends", map[string]string{"action": "accept", "name": "Ali"}); code != 200 {
		t.Fatal(code)
	}
	if code, out := a.post(t, "/api/clan", map[string]string{"action": "create", "name": "Lochin", "tag": "LCH"}); code != 200 {
		t.Fatal(code, out)
	}
	// Vali menyuda (so'rov yuboradi) → Ali uni "menyuda" ko'radi
	b.get(t, "/api/social")
	_, out := a.get(t, "/api/social")
	fr := out["friends"].([]any)[0].(map[string]any)
	if fr["online"] != "menu" || fr["name"] != "Vali" {
		t.Fatalf("menyudagi do'st: %v", fr)
	}
	// xonaga kirmasdan chaqirib bo'lmaydi
	if code, _ := a.post(t, "/api/friends", map[string]string{"action": "invite", "name": "Vali"}); code != 400 {
		t.Fatal("xonasiz chaqiriq:", code)
	}
	wa := dialWS(t, a, "zn-dost1")
	defer wa.c.Close()
	// Ali xonada — Vali uni ko'radi
	_, out = b.get(t, "/api/social")
	fr = out["friends"].([]any)[0].(map[string]any)
	if fr["online"] != "game" || fr["room"] != "dost1" || fr["clan"] != "LCH" {
		t.Fatalf("o'ynayotgan do'st: %v", fr)
	}
	if code, out := a.post(t, "/api/friends", map[string]string{"action": "invite", "name": "Vali"}); code != 200 {
		t.Fatal("chaqiriq:", code, out)
	}
	_, out = b.get(t, "/api/social")
	inv := out["invites"].([]any)
	if len(inv) != 1 || inv[0].(map[string]any)["room"] != "dost1" || inv[0].(map[string]any)["from"] != "Ali" {
		t.Fatalf("chaqiriq kelmadi: %v", out["invites"])
	}
	// Vali qo'shiladi — chaqiriq bajarildi; xonada Alining hisobida klan tegi bor
	wb := dialWS(t, b, "zn-dost1")
	defer wb.c.Close()
	_, out = b.get(t, "/api/social")
	if len(out["invites"].([]any)) != 0 {
		t.Fatal("bajarilgan chaqiriq qoldi")
	}
	wa.send(map[string]any{"t": "p", "d": map[string]any{"v": 1, "tm": "T", "nm": "Ali"}})
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for !found && time.Now().Before(deadline) {
		m := wb.read(t)
		if m["t"] == "u" {
			if p, ok := m["p"].(map[string]any); ok {
				if acc, ok := p["acc"].(map[string]any); ok && acc["cl"] == "LCH" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("klan tegi presence'da yo'q")
	}
	// Ali chiqdi — endi menyuda
	wa.c.Close()
	time.Sleep(200 * time.Millisecond)
	_, out = b.get(t, "/api/social")
	if fr = out["friends"].([]any)[0].(map[string]any); fr["online"] != "menu" {
		t.Fatalf("chiqqan do'st: %v", fr)
	}
	_ = st
}
