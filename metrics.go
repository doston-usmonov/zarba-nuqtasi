package main

// Monitoring: GET /metrics — Prometheus matn formatida (tashqi kutubxonasiz).
//
// METRICS_TOKEN berilmasa, manzil yopiq (404). Berilsa, so'rovda "Authorization: Bearer <token>"
// (yoki ?token=) bo'lishi kerak — nginx orqasida /metrics ham ochiq bo'lgani uchun.
// docker-compose'dagi «monitoring» profili Prometheus'ni shu token bilan ishga tushiradi (README).
//
// Ko'rsatkichlar: xonalar (rejim bo'yicha, rasmiy, navbat, turnir), o'yinchilar, botlar,
// kuzatuvchilar, navbatdagilar; ulanishlar, tugagan va reytingli o'yinlar, server xatolari (5xx),
// hakam ogohlantirishlari, sekin mijozlarga yetmay qolgan xabarlar; xona taktining eng uzun vaqti;
// baza holati; Go: goroutine, xotira, ishlash vaqti.

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var mtr struct {
	conns, matches, rated, apiErrors, refWarns, dropped atomic.Int64
	tickMaxNs                                           atomic.Int64 // oxirgi so'rovdan beri eng uzun xona takti
}

var startedAt = time.Now()

// noteTick: xona taktining davomiyligi (eng kattasi saqlanadi).
func noteTick(d time.Duration) {
	for {
		cur := mtr.tickMaxNs.Load()
		if int64(d) <= cur || mtr.tickMaxNs.CompareAndSwap(cur, int64(d)) {
			return
		}
	}
}

type metricsSrc struct {
	h     *hub
	st    *Store
	queue *matchmaker
	token string
}

func (m metricsSrc) authorized(r *http.Request) bool {
	if m.token == "" {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(m.token)) == 1
}

func (m metricsSrc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.token == "" {
		http.NotFound(w, r)
		return
	}
	if !m.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		http.Error(w, "token kerak", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	var b strings.Builder
	gauge := func(name, help string) { fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name) }
	counter := func(name, help string, v int64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}

	// xonalar va o'yinchilar
	type agg struct{ rooms, humans, bots, spec int }
	byMode := map[string]*agg{}
	var total agg
	kinds := map[string]int{"official": 0, "mm": 0, "tournament": 0, "custom": 0}
	for _, r := range m.h.roomList() {
		r.mu.Lock()
		bots, humans := r.botCountLocked()
		spec := r.specCountLocked()
		mode := r.gs.modeID()
		switch {
		case r.official != nil:
			kinds["official"]++
		case r.mm != nil:
			kinds["mm"]++
		case r.trn != nil:
			kinds["tournament"]++
		default:
			kinds["custom"]++
		}
		r.mu.Unlock()
		a := byMode[mode]
		if a == nil {
			a = &agg{}
			byMode[mode] = a
		}
		a.rooms, a.humans, a.bots, a.spec = a.rooms+1, a.humans+humans, a.bots+bots, a.spec+spec
		total.rooms, total.humans, total.bots, total.spec = total.rooms+1, total.humans+humans, total.bots+bots, total.spec+spec
	}
	modes := make([]string, 0, len(byMode))
	for k := range byMode {
		modes = append(modes, k)
	}
	sort.Strings(modes)
	gauge("zarba_rooms", "Ochiq xonalar soni")
	fmt.Fprintf(&b, "zarba_rooms %d\n", total.rooms)
	gauge("zarba_rooms_by_kind", "Xonalar turi bo'yicha")
	for _, k := range []string{"custom", "mm", "official", "tournament"} {
		fmt.Fprintf(&b, "zarba_rooms_by_kind{kind=%q} %d\n", k, kinds[k])
	}
	gauge("zarba_rooms_by_mode", "Xonalar rejim bo'yicha")
	for _, k := range modes {
		fmt.Fprintf(&b, "zarba_rooms_by_mode{mode=%q} %d\n", k, byMode[k].rooms)
	}
	gauge("zarba_players", "Xonalardagi odam o'yinchilar")
	fmt.Fprintf(&b, "zarba_players %d\n", total.humans)
	gauge("zarba_players_by_mode", "Odam o'yinchilar rejim bo'yicha")
	for _, k := range modes {
		fmt.Fprintf(&b, "zarba_players_by_mode{mode=%q} %d\n", k, byMode[k].humans)
	}
	gauge("zarba_bots", "Server botlari")
	fmt.Fprintf(&b, "zarba_bots %d\n", total.bots)
	gauge("zarba_spectators", "Kuzatuvchilar")
	fmt.Fprintf(&b, "zarba_spectators %d\n", total.spec)
	if m.queue != nil {
		st := m.queue.status(0)
		gauge("zarba_queue_players", "Reytingli navbatdagi o'yinchilar")
		for _, k := range []string{modeComp, modeWM} {
			fmt.Fprintf(&b, "zarba_queue_players{mode=%q} %d\n", k, st.InQueue[k])
		}
	}
	counter("zarba_connections_total", "Qabul qilingan o'yin ulanishlari", mtr.conns.Load())
	counter("zarba_matches_total", "Bazaga yozilgan tugagan o'yinlar", mtr.matches.Load())
	counter("zarba_rated_players_total", "Reytingi o'zgargan o'yinchilar (o'yinlar bo'yicha jami)", mtr.rated.Load())
	counter("zarba_api_errors_total", "Server xatolari (HTTP 500)", mtr.apiErrors.Load())
	counter("zarba_referee_warnings_total", "Hakam rad etgan harakatlar (o'q, siljish, xarid va hokazo)", mtr.refWarns.Load())
	counter("zarba_dropped_messages_total", "Sekin mijozga yetmay qolgan xabarlar", mtr.dropped.Load())
	gauge("zarba_tick_max_seconds", "Oxirgi so'rovdan beri eng uzun xona takti (50 ms dan oshmasligi kerak)")
	fmt.Fprintf(&b, "zarba_tick_max_seconds %.6f\n", float64(mtr.tickMaxNs.Swap(0))/1e9)
	gauge("zarba_db_up", "Baza ishlayaptimi (1/0)")
	db := 0
	if m.st.Ready() {
		db = 1
	}
	fmt.Fprintf(&b, "zarba_db_up %d\n", db)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge("zarba_goroutines", "Goroutine'lar")
	fmt.Fprintf(&b, "zarba_goroutines %d\n", runtime.NumGoroutine())
	gauge("zarba_heap_bytes", "Ishlatilayotgan xotira (heap)")
	fmt.Fprintf(&b, "zarba_heap_bytes %d\n", ms.HeapAlloc)
	gauge("zarba_uptime_seconds", "Server ishlab turgan vaqt")
	fmt.Fprintf(&b, "zarba_uptime_seconds %.0f\n", time.Since(startedAt).Seconds())
	v := version
	if len(v) > 7 {
		v = v[:7]
	}
	gauge("zarba_build_info", "Versiya")
	fmt.Fprintf(&b, "zarba_build_info{version=%q} 1\n", v)
	w.Write([]byte(b.String()))
}
