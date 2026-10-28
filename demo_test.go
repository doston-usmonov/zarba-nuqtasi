package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDemoRecording(t *testing.T) {
	srv, _, h := newTestServer(t)
	defer srv.Close()
	h.dataDir = t.TempDir()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/demos", h.demosHandler)
	mux.HandleFunc("GET /api/demos/{id}", h.demoFileHandler)
	r, a, b := newRefRoom()
	a.acct = &account{id: 1, name: "Ali"}
	pres := func(c *client, x float64, shot int) {
		p := map[string]any{"v": 1, "nm": c.id, "sq": "A", "tm": "T", "x": x, "y": 0, "z": 70, "yw": 0.5, "al": 1, "hp": 100, "w": "rifle", "acc": map[string]any{"u": "Ali"}}
		if c == b {
			p["sq"], p["tm"] = "B", "CT"
		}
		if shot > 0 {
			p["sh"] = map[string]any{"n": shot, "w": "rifle"}
		}
		c.pres = jsonMsg(p)
		c.info.Sq = p["sq"].(string)
	}
	*r.gs = gameState{R: 1, Ph: "freeze", Md: modeComp, Fm: 12, E: "ep1", Bm: -1, T: 5, M: "bozor"}
	now := time.Now()
	for i := 0; i < 30; i++ {
		now = now.Add(50 * time.Millisecond)
		shot := 0
		if i >= 10 {
			shot = 1 + i/10 // o'q raqami 2, 3 — har biri bir marta yozilishi kerak
		}
		pres(a, float64(10+i), shot)
		pres(b, 40, 0)
		if i == 5 {
			r.gs.Ph = "live"
		}
		r.demoTickLocked(h, now)
	}
	if r.demo == nil || r.demo.frames < 14 {
		t.Fatalf("kadrlar yozilmadi: %+v", r.demo)
	}
	// o'yin tugadi — yozuv yopiladi va ro'yxatga tushadi
	r.gs.Ph, r.gs.SA, r.gs.SB = "over", 13, 7
	r.demoTickLocked(h, now.Add(200*time.Millisecond))
	var list struct{ Demos []demoRow }
	for i := 0; i < 40 && len(list.Demos) == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		w := httptestGet(mux, "/api/demos?player=Ali")
		json.Unmarshal(w, &list)
	}
	if len(list.Demos) != 1 || list.Demos[0].SA != 13 || list.Demos[0].Map != "bozor" || len(list.Demos[0].Players) != 2 {
		t.Fatalf("ro'yxat: %+v", list)
	}
	files, _ := filepath.Glob(filepath.Join(h.dataDir, "demos", "*"))
	if len(files) != 1 || filepath.Ext(files[0]) != ".gz" {
		t.Fatalf("fayllar: %v", files)
	}
	// yuklab olish: gzip'li, 1-qator sarlavha, keyin kadrlar; o'q hodisasi faqat yangilanganda
	raw := httptestGet(mux, "/api/demos/"+itoa(int(list.Demos[0].ID)))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	sc.Scan()
	var head map[string]any
	json.Unmarshal(sc.Bytes(), &head)
	if head["map"] != "bozor" || head["mode"] != "comp" {
		t.Fatalf("sarlavha: %v", head)
	}
	frames, shots, names := 0, 0, 0
	var last map[string]any
	for sc.Scan() {
		var f struct {
			T int64
			G map[string]any
			P []map[string]any
		}
		if err := json.Unmarshal(sc.Bytes(), &f); err != nil {
			t.Fatal(err)
		}
		frames++
		for _, p := range f.P {
			if p["sh"] != nil {
				shots++
			}
			if p["nm"] != nil {
				names++
			}
		}
		last = f.G
	}
	if frames < 15 || shots != 2 || names != 2 || last["ph"] != "over" {
		t.Fatalf("kadrlar %d, o'qlar %d, ismlar %d, oxirgi %v", frames, shots, names, last)
	}
	// tugamagan o'yin yozuvi o'chiriladi
	*r.gs = gameState{R: 1, Ph: "live", Md: modeComp, Fm: 12, E: "ep2", Bm: -1, M: "bozor"}
	r.demo = nil
	r.demoTickLocked(h, now.Add(time.Second))
	part := r.demo.path + ".part"
	if _, err := os.Stat(part); err != nil {
		t.Fatal("yangi yozuv boshlanmadi")
	}
	r.gs.Ph = "warmup"
	r.demoTickLocked(h, now.Add(2*time.Second))
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("tugamagan yozuv o'chirilmadi")
	}
}

func httptestGet(h http.Handler, path string) []byte {
	req, _ := http.NewRequest("GET", path, nil)
	w := &respRec{h: http.Header{}}
	h.ServeHTTP(w, req)
	if w.h.Get("Content-Encoding") == "gzip" {
		zr, _ := gzip.NewReader(bytes.NewReader(w.b.Bytes()))
		out, _ := io.ReadAll(zr)
		return out
	}
	return w.b.Bytes()
}

type respRec struct {
	h    http.Header
	b    bytes.Buffer
	code int
}

func (r *respRec) Header() http.Header         { return r.h }
func (r *respRec) Write(p []byte) (int, error) { return r.b.Write(p) }
func (r *respRec) WriteHeader(c int)           { r.code = c }
