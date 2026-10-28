package main

import (
	"context"
	"testing"
	"time"
)

func TestLevels(t *testing.T) {
	if lv, _, _ := levelOf(0); lv != 1 {
		t.Fatal(lv)
	}
	if lv, cur, need := levelOf(500); lv != 2 || cur != 0 || need != 600 {
		t.Fatalf("500 XP: %d %d %d", lv, cur, need)
	}
	prev := int64(-1)
	for l := 1; l <= maxLevel; l++ {
		if levelStart(l) <= prev {
			t.Fatal("darajalar o'smaydi")
		}
		prev = levelStart(l)
		if lv, _, _ := levelOf(levelStart(l)); lv != l {
			t.Fatalf("%d-daraja: %d", l, lv)
		}
	}
	if lv, _, need := levelOf(1 << 40); lv != maxLevel || need != 0 {
		t.Fatal("eng yuqori daraja")
	}
	a, b := dailyMissions(7, "2026-10-26"), dailyMissions(7, "2026-10-26")
	if a[0].ID != b[0].ID || a[1].ID == a[0].ID || a[2].ID == a[1].ID || a[2].ID == a[0].ID {
		t.Fatalf("kunlik vazifalar: %v %v", a, b)
	}
	diff := false
	for d := 1; d < 10; d++ {
		if dailyMissions(7, "2026-10-1"+string(rune('0'+d)))[0].ID != a[0].ID {
			diff = true
		}
	}
	if !diff {
		t.Fatal("vazifalar har kuni bir xil")
	}
}

func TestProgressStore(t *testing.T) {
	s, ctx := testStore(t, ""), context.Background()
	u, _, err := s.Register(ctx, "Ali", "secret12")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ms := dailyMissions(u.ID, dayOf(now))
	// bitta o'yinda hamma vazifa metrikasini to'ldiramiz
	big := map[string]int{"kills": 30, "hs": 10, "mvp": 5, "wins": 1, "dmkills": 20, "ggknife": 1, "matches": 1, "dmg": 3000}
	xp, done, _, err := s.ApplyGain(ctx, matchGain{uid: u.ID, mode: modeComp, xp: 400, metrics: big}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(400)
	for _, m := range ms {
		if m.metric == "matches" {
			continue // 3 ta o'yin kerak
		}
		want += int64(m.Reward)
	}
	if xp != want || len(done) == 0 {
		t.Fatalf("XP: %d, kutilgan %d, bajarilgan %v", xp, want, done)
	}
	// takror: bajarilgan vazifa ikkinchi marta mukofot bermaydi
	xp2, done2, _, _ := s.ApplyGain(ctx, matchGain{uid: u.ID, mode: modeComp, xp: 100, metrics: big}, now)
	for _, d := range done2 {
		if d.metric != "matches" {
			t.Fatalf("vazifa ikki marta: %v", d)
		}
	}
	if xp2 < xp+100 {
		t.Fatal("XP qo'shilmadi")
	}
	x, st, lo, err := s.Progress(ctx, u.ID, now)
	if err != nil || x != xp2 || len(st) != 3 || len(lo) != 0 {
		t.Fatalf("progress: %d %v %v %v", x, st, lo, err)
	}
	// bo'yoq: ochilmaganini tanlab bo'lmaydi
	if _, err := s.SetFinish(ctx, u.ID, "rifle", "afsona"); err != errLocked {
		t.Fatalf("yopiq bo'yoq tanlandi: %v", err)
	}
	lv, _, _ := levelOf(x)
	var open string
	for _, f := range finishes {
		if f.Level <= lv {
			open = f.ID
		}
	}
	if open == "" {
		t.Fatalf("%d-darajada ochiq bo'yoq yo'q", lv)
	}
	lo, err = s.SetFinish(ctx, u.ID, "rifle", open)
	if err != nil || lo["rifle"] != open {
		t.Fatalf("bo'yoq: %v %v", lo, err)
	}
	if l2, _ := s.Loadout(ctx, u.ID); l2["rifle"] != open {
		t.Fatal("bo'yoq saqlanmadi")
	}
}
