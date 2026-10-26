package main

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestNadeSimMatchesClient: server granatani brauzer bilan aynan bir xil hisoblaydi
// (testdata/nades.json ni tools/nadefix.js brauzer kodidan yasaydi).
func TestNadeSimMatchesClient(t *testing.T) {
	b, err := os.ReadFile("testdata/nades.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Map string     `json:"map"`
		K   string     `json:"k"`
		P   [3]float64 `json:"p"`
		V   [3]float64 `json:"v"`
		Res struct {
			T, X, Y, Z float64
			Ground     bool
		} `json:"res"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	bad := 0
	for i, c := range cases {
		gm := gameMaps[c.Map]
		res := gm.nadeSim(c.K, vec3{c.P[0], c.P[1], c.P[2]}, vec3{c.V[0], c.V[1], c.V[2]})
		d := math.Abs(res.p.X-c.Res.X) + math.Abs(res.p.Y-c.Res.Y) + math.Abs(res.p.Z-c.Res.Z)
		if math.Abs(res.t-c.Res.T) > 1e-9 || d > 1e-6 {
			bad++
			t.Errorf("%d %s %s: server t=%.4f p=%.3f,%.3f,%.3f  brauzer t=%.4f p=%.3f,%.3f,%.3f", i, c.Map, c.K, res.t, res.p.X, res.p.Y, res.p.Z, c.Res.T, c.Res.X, c.Res.Y, c.Res.Z)
		}
	}
	if len(cases) < 100 || bad > 0 {
		t.Fatalf("%d / %d farq qildi", bad, len(cases))
	}
}
