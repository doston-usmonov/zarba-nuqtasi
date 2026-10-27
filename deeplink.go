package main

// Xona havolalari: https://game.telegap.uz/x/<xarita>/<kod>
//
// Havolani ochgan odam to'g'ri o'sha xonaga tushadi (bo'lmasa shu xaritada yaratiladi) — buni
// brauzer qiladi (web/index.html: location.pathname). Server esa /x/ yo'llariga o'sha index.html ni
// qaytaradi, faqat <title> va og: teglarini shu xona uchun to'ldirib: Telegram va boshqa
// messenjerlarda havola «Registon — umumiy · 6/16 o'yinchi» bo'lib chiqadi.

import (
	"html"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

const (
	siteName  = "Zarba Nuqtasi"
	siteTitle = "<title>" + siteName + "</title>"
	siteDesc  = "Brauzerdagi taktik otishma: botlarga qarshi yoki do'stlar bilan onlayn."
)

// roomPeek: havola sarlavhasi uchun xonaning joriy holati (xona bo'lmasa ok=false).
func (h *hub) roomPeek(name string) (gmap, mode string, humans, max int, ok bool) {
	h.mu.Lock()
	r := h.rooms[name]
	h.mu.Unlock()
	if r == nil {
		return "", "", 0, 0, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, humans = r.botCountLocked()
	return r.gs.M, r.gs.modeID(), humans, r.max, true
}

// parseDeepLink: "/x/<xarita>/<kod>" → xarita va kod. Noto'g'ri bo'lsa ok=false (oddiy bosh sahifa).
func parseDeepLink(path string) (gmap, code string, ok bool) {
	rest, found := strings.CutPrefix(path, "/x/")
	if !found {
		return "", "", false
	}
	rest = strings.TrimSuffix(rest, "/")
	gmap, code, found = strings.Cut(rest, "/")
	if !found || !validMap(gmap) || !roomName.MatchString("zn-"+code) || strings.Contains(code, "/") {
		return "", "", false
	}
	return gmap, code, true
}

// mapDisplayName: "registon" → "Registon".
func mapDisplayName(id string) string {
	if m := gameMaps[id]; m != nil {
		return m.name
	}
	return id
}

// linkMeta: havola uchun sarlavha va tavsif.
func (h *hub) linkMeta(gmap, code, qmode string) (title, desc, img string) {
	mode := modeComp
	if validMode(qmode) {
		mode = qmode
	}
	players := ""
	if m, md, humans, max, ok := h.roomPeek("zn-" + code); ok {
		gmap, mode = m, md
		if humans > 0 {
			players = strconv.Itoa(humans)
			if max > 0 {
				players += "/" + strconv.Itoa(max)
			}
			players += " o'yinchi · "
		}
	}
	title = mapDisplayName(gmap) + " — " + code + " · " + players + siteName
	desc = modeNames[mode] + " · " + mapDisplayName(gmap) + " xaritasi. Havolani oching — brauzerda, yuklab olmasdan o'ynaysiz."
	if players == "" {
		desc = "Xona hozircha bo'sh — birinchi bo'lib kiring. " + desc
	}
	return title, desc, "/og/" + gmap + ".png"
}

// fillMeta: index.html dagi sarlavha va og: teglarini almashtiradi (qiymatlar html.EscapeString dan o'tadi).
func fillMeta(page []byte, title, desc, img, pageURL string) []byte {
	s := string(page)
	e := html.EscapeString
	s = strings.Replace(s, siteTitle, "<title>"+e(title)+"</title>", 1)
	s = strings.Replace(s, `<meta property="og:title" content="`+siteName+`">`, `<meta property="og:title" content="`+e(title)+`">`, 1)
	s = strings.Replace(s, `<meta name="description" content="`+siteDesc+`">`, `<meta name="description" content="`+e(desc)+`">`, 1)
	s = strings.Replace(s, `<meta property="og:description" content="`+siteDesc+`">`, `<meta property="og:description" content="`+e(desc)+`">`, 1)
	s = strings.Replace(s, `<meta property="og:image" content="/og/qumtepa.png">`, `<meta property="og:image" content="`+e(img)+`">`, 1)
	s = strings.Replace(s, `<meta property="og:url" content="/">`, `<meta property="og:url" content="`+e(pageURL)+`">`, 1)
	return []byte(s)
}

// absURL: og:image va og:url to'liq manzil bo'lishi kerak (nginx X-Forwarded-Proto beradi).
func absURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

// webHandler: statik fayllar + /x/ havolalari uchun index.html.
func webHandler(sub fs.FS, h *hub) http.Handler {
	files := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || strings.HasPrefix(r.URL.Path, "/x/") {
			// /x/ yo'llari hech qachon fayl tizimiga tushmaydi: noto'g'ri havola — oddiy bosh sahifa
			title, desc, img := siteName, siteDesc, "/og/qumtepa.png"
			if gmap, code, ok := parseDeepLink(r.URL.Path); ok {
				title, desc, img = h.linkMeta(gmap, code, r.URL.Query().Get("mode"))
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(fillMeta(index, title, desc, absURL(r, img), absURL(r, r.URL.Path)))
			return
		}
		files.ServeHTTP(w, r)
	})
}
