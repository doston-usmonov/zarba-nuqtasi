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

func main() {
	addr := env("ADDR", ":8080")
	maxPlayers, _ := strconv.Atoi(env("MAX_PLAYERS", "16"))
	if maxPlayers < 2 {
		maxPlayers = 16
	}
	allowed := parseOrigins(env("ALLOWED_ORIGINS", ""))
	h := newHub(maxPlayers)

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	files := http.FileServer(http.FS(sub))

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
		ws, err := upgrade(w, r)
		if err != nil {
			return
		}
		h.serve(ws, room, clientIP(r))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		rooms, players := h.stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"rooms": rooms, "players": players})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	})

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
