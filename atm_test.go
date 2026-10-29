package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAtmOptions(t *testing.T) {
	o := parseRoomOpts(map[string][]string{"time": {"night"}, "wx": {"rain"}})
	if o.at != "night" || o.wx != "rain" {
		t.Fatalf("%+v", o)
	}
	if o := parseRoomOpts(map[string][]string{"time": {"tushlik"}, "wx": {"<b>"}}); o.at != "" || o.wx != "" {
		t.Fatalf("noto'g'ri qiymat: %+v", o)
	}
	seen := map[string]bool{}
	for i := 0; i < 400; i++ {
		o := parseRoomOpts(map[string][]string{"time": {"rand"}, "wx": {"rand"}})
		if !atmTimes[o.at] || !atmWx[o.wx] {
			t.Fatalf("tasodifiy: %+v", o)
		}
		seen[o.at+"/"+o.wx] = true
	}
	if len(seen) < 6 {
		t.Fatalf("tasodifiy havo xilma-xil emas: %v", seen)
	}
	g := newGameState()
	g.setAtm("night", "snow")
	g.M = "zavod"
	g.reset()
	b, _ := json.Marshal(g)
	var out map[string]any
	json.Unmarshal(b, &out)
	if out["at"] != "night" || out["wx"] != "snow" {
		t.Fatalf("yangi o'yinda havo saqlanmadi: %s", b)
	}
	g.setAtm("day", "clear")
	b, _ = json.Marshal(g)
	out = map[string]any{}
	json.Unmarshal(b, &out)
	if _, ok := out["at"]; ok {
		t.Fatalf("kunduz holatda yozilmasligi kerak: %s", b)
	}
}

func TestAtmAdmin(t *testing.T) {
	r, a, b := newRefRoom()
	r.manual = false
	r.owner = a.id
	h := &hub{rooms: map[string]*room{}}
	r.mu.Lock()
	defer r.mu.Unlock()
	h.adminLocked(r, b, json.RawMessage(`{"a":"atm","at":"night","wx":"dust"}`), time.Now())
	if r.gs.At != "" { // egasi emas
		t.Fatal("egasi bo'lmagan o'yinchi havoni o'zgartirdi")
	}
	h.adminLocked(r, a, json.RawMessage(`{"a":"atm","at":"night","wx":"dust"}`), time.Now())
	if r.gs.At != "night" || r.gs.Wx != "dust" {
		t.Fatalf("egasi o'zgartira olmadi: %q %q", r.gs.At, r.gs.Wx)
	}
}
