// Zarba Nuqtasi — o'yin serveri.
//
// Ikki vazifa: o'yinning o'zini (web/index.html) tarqatadi va /ws manzilida
// o'yinchilarni xonalarga yig'ib, har birining holatini (presence) qolganlarga
// uzatadi. Raund mantig'i o'yinchilardan birida (xost) ishlaydi, server faqat
// relay: kod tashqi kutubxonasiz, faqat Go standart kutubxonasi.
package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

var roomName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,47}$`)

// version: build vaqtida commit SHA (Dockerfile: -ldflags -X main.version=...), /healthz va menyuda ko'rinadi
var version = "dev"

func main() {
	addr := env("ADDR", ":8080")
	// bitta xonadagi o'yinchilar (botlar bilan) uchun umumiy yuqori chegara; xona egasi undan pastroq chegara qo'yishi mumkin
	maxPlayers, _ := strconv.Atoi(env("MAX_PLAYERS", "64"))
	if maxPlayers < 2 {
		maxPlayers = 64
	}
	allowed := parseOrigins(env("ALLOWED_ORIGINS", ""))

	// Hisoblar va reyting — PostgreSQL. Baza bo'lmasa yoki hali tayyor bo'lmasa ham o'yin ishlayveradi (mehmon rejimida).
	var st *Store
	if dsn := os.Getenv("DATABASE_URL"); dsn == "" {
		log.Printf("DIQQAT: DATABASE_URL berilmagan — hisoblar va reyting o'chirilgan")
	} else if s, err := openStore(dsn, os.Getenv("DATA_DIR")); err != nil {
		log.Printf("DIQQAT: DATABASE_URL noto'g'ri: %v", err)
	} else {
		st = s
	}
	a := newAPI(st, allowed)
	h := newHub(maxPlayers, st)
	a.onLoadout = h.setLoadout
	a.hub = h
	a.queue.open = h.openMM
	h.voice = voiceFromEnv()
	h.debug = os.Getenv("ZARBA_DEBUG") == "1"
	h.startOfficial(parseOfficialRooms(os.Getenv("OFFICIAL_ROOMS")))
	if h.debug {
		log.Printf("DIQQAT: ZARBA_DEBUG=1 — raund holatini mijoz o'zgartira oladi, faqat test uchun")
	}
	if h.voice.turnSecret != "" {
		log.Printf("ovozli chat: TURN yoqilgan (port %s)", h.voice.turnPort)
	} else {
		log.Printf("ovozli chat: faqat STUN (TURN_SECRET berilmagan — ba'zi tarmoqlarda ovoz o'tmasligi mumkin)")
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !originOK(r, allowed) {
			http.Error(w, "origin ruxsat etilmagan", http.StatusForbidden)
			return
		}
		room := r.URL.Query().Get("room")
		if !roomName.MatchString(room) {
			http.Error(w, "xona nomi noto'g'ri", http.StatusBadRequest)
			return
		}
		var acct *account
		if u, ok := a.currentUser(r); ok {
			acct = &account{id: u.ID, name: u.Username}
		}
		ws, err := upgrade(w, r)
		if err != nil {
			return
		}
		h.serve(ws, room, clientIP(r), r.Host, acct, parseRoomOpts(r.URL.Query()))
	})
	a.routes(mux)
	mux.HandleFunc("/api/rooms", h.roomsHandler)
	mux.HandleFunc("GET /api/rooms/{code}", h.roomHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		rooms, players := h.stats()
		w.Header().Set("Content-Type", "application/json")
		db := "off"
		if st != nil {
			db = "starting"
			if st.Ready() {
				db = "ok"
			}
		}
		v := version
		if len(v) > 7 {
			v = v[:7]
		}
		json.NewEncoder(w).Encode(map[string]any{"rooms": rooms, "players": players, "db": db, "version": v})
	})
	mux.Handle("/", webHandler(sub, h))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("Zarba Nuqtasi serveri %s manzilida ishlayapti (xonada ko'pi bilan %d o'yinchi)", addr, maxPlayers)
	log.Fatal(srv.ListenAndServe())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ALLOWED_ORIGINS bo'sh bo'lsa faqat o'z domeni; "*" — hammasi; aks holda vergul bilan hostlar ro'yxati.
func parseOrigins(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

func originOK(r *http.Request, allowed []string) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if strings.EqualFold(host, r.Host) {
		return true
	}
	for _, a := range allowed {
		if a == "*" || a == host {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}
