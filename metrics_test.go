package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetrics(t *testing.T) {
	h := newHub(16, nil)
	r, a, _ := newRefRoom()
	r.official = &officialRoom{gmap: "bozor", mode: modeComp, max: 10}
	h.rooms[r.name] = r
	h.adminLocked(r, a, json.RawMessage(`{"a":"bot","sq":"A"}`), time.Now()) // rasmiy xonada ega yo'q — bot qo'shilmaydi
	r.addBotLocked("B", "normal", time.Now())
	q := newMatchmaker()
	q.join(qMember{id: 1, name: "a", rating: 1000}, modeComp, nil)
	mtr.apiErrors.Add(2)
	noteTick(7 * time.Millisecond)
	get := func(m metricsSrc, hdr string) (int, string) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		if hdr != "" {
			req.Header.Set("Authorization", "Bearer "+hdr)
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	if code, _ := get(metricsSrc{h: h, queue: q}, ""); code != http.StatusNotFound {
		t.Fatalf("tokensiz yoqilmagan bo'lishi kerak: %d", code)
	}
	m := metricsSrc{h: h, queue: q, token: "sir"}
	if code, _ := get(m, "xato"); code != http.StatusUnauthorized {
		t.Fatalf("noto'g'ri token: %d", code)
	}
	code, body := get(m, "sir")
	if code != 200 {
		t.Fatalf("token bilan: %d", code)
	}
	for _, want := range []string{"zarba_rooms 1\n", `zarba_rooms_by_kind{kind="official"} 1`, "zarba_players 2\n", "zarba_bots 1\n",
		`zarba_queue_players{mode="comp"} 1`, "# TYPE zarba_api_errors_total counter", "zarba_db_up 0\n", "zarba_tick_max_seconds 0.007"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%q yo'q:\n%s", want, body)
		}
	}
	if _, body := get(m, "sir"); !strings.Contains(body, "zarba_tick_max_seconds 0.000000") {
		t.Fatal("eng uzun takt so'rovdan keyin nolga tushmadi")
	}
}
