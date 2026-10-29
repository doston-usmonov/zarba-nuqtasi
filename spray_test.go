package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSprayReferee(t *testing.T) {
	r, a, _ := newRefRoom()
	m := r.gm()
	// devori shimolda bo'lgan bo'sh katak
	var cx, cz int
	for z := 2; z < gridH-2 && cx == 0; z++ {
		for x := 2; x < gridW-2; x++ {
			if m.cellType(x, z) == cFloor && m.cellType(x, z-1) == cWall && m.cellType(x, z+1) == cFloor {
				cx, cz = x, z
				break
			}
		}
	}
	at := cellCenter([2]int{cx, cz})
	p := a.ps
	p.alive, p.posOK, p.pos = true, true, at
	pat := strings.Repeat("0123456789abc", 20)[:256]
	try := func(s sprayClaim, now time.Time) bool {
		raw, _ := json.Marshal(s)
		f := map[string]json.RawMessage{"sp": raw}
		r.mu.Lock()
		r.sprayLocked(a, f, now)
		r.mu.Unlock()
		_, ok := f["sp"]
		return ok
	}
	now := time.Now()
	good := sprayClaim{N: 1, X: at.X, Y: 1.6, Z: float64(cz) * cellSize, NX: 0, NZ: 1, P: pat}
	if !try(good, now) {
		t.Fatal("to'g'ri graffiti rad etildi")
	}
	if !try(good, now.Add(time.Second)) { // xuddi shu (holatda turibdi)
		t.Fatal("qabul qilingan graffiti keyingi holatda yo'qoldi")
	}
	second := good
	second.N = 2
	if try(second, now.Add(3*time.Second)) {
		t.Fatal("10 soniya o'tmasdan ikkinchi graffiti")
	}
	for name, s := range map[string]sprayClaim{
		"uzoq":           {N: 3, X: at.X + 9, Y: 1.6, Z: float64(cz) * cellSize, NZ: 1, P: pat},
		"havoda":         {N: 3, X: at.X, Y: 1.6, Z: at.Z, NZ: 1, P: pat},
		"teskari":        {N: 3, X: at.X, Y: 1.6, Z: float64(cz) * cellSize, NZ: -1, P: pat},
		"qiya":           {N: 3, X: at.X, Y: 1.6, Z: float64(cz) * cellSize, NX: .7071, NZ: .7071, P: pat},
		"rasm noto'g'ri": {N: 3, X: at.X, Y: 1.6, Z: float64(cz) * cellSize, NZ: 1, P: strings.Repeat("z", 256)},
		"juda past":      {N: 3, X: at.X, Y: .1, Z: float64(cz) * cellSize, NZ: 1, P: pat},
	} {
		if try(s, now.Add(20*time.Second)) {
			t.Errorf("%s: qabul qilindi", name)
		}
	}
	second.N = 3
	if !try(second, now.Add(20*time.Second)) {
		t.Fatal("10 soniyadan keyin graffiti qabul qilinmadi")
	}
	p.alive = false
	second.N = 4
	if try(second, now.Add(40*time.Second)) {
		t.Fatal("o'lgan o'yinchi graffiti chizdi")
	}
}
