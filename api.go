package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const cookieName = "zn_sess"

type api struct {
	st      *Store
	allowed []string
	login   *limiter // noto'g'ri parolni taxmin qilishga qarshi
	reg     *limiter // ko'plab soxta hisob ochishga qarshi
	// onLoadout: o'yinchi qurol bo'yog'ini tanladi (xonadagi presence'ga darhol ta'sir qiladi)
	onLoadout func(uid int64, lo map[string]string)
	hub       *hub        // do'stlarning onlayn holati va xonaga chaqiriqlar (social.go); testlarda nil bo'lishi mumkin
	queue     *matchmaker // «O'yin topish» navbati (queue.go)
	tg        tgConfig    // Telegram orqali kirish (telegram.go); bo'sh — o'chiq
}

func newAPI(st *Store, allowed []string) *api {
	return &api{st: st, allowed: allowed,
		login: newLimiter(10, 30*time.Second), // 10 ta urinish, so'ng har 30 soniyada bittadan
		reg:   newLimiter(5, 10*time.Minute), queue: newMatchmaker()}
}

func (a *api) routes(mux *http.ServeMux) {
	h := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if !a.st.Ready() {
				writeErr(w, http.StatusServiceUnavailable, "Hisoblar vaqtincha ishlamayapti, birozdan keyin qayta urinib ko'ring")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			f(w, r.WithContext(ctx))
		}
	}
	mux.HandleFunc("GET /api/me", h(a.me))
	mux.HandleFunc("POST /api/register", h(a.register))
	mux.HandleFunc("POST /api/login", h(a.doLogin))
	mux.HandleFunc("POST /api/logout", h(a.logout))
	mux.HandleFunc("GET /api/leaderboard", h(a.leaderboard))
	mux.HandleFunc("GET /api/players/{name}", h(a.player))
	mux.HandleFunc("GET /api/progress", h(a.progress))
	mux.HandleFunc("POST /api/loadout", h(a.setFinish))
	mux.HandleFunc("GET /api/social", h(a.socialGet))
	mux.HandleFunc("POST /api/friends", h(a.friendPost))
	mux.HandleFunc("POST /api/clan", h(a.clanPost))
	mux.HandleFunc("GET /api/clans", h(a.clansGet))
	mux.HandleFunc("GET /api/queue", h(a.queueGet))
	mux.HandleFunc("POST /api/queue", h(a.queuePost))
	a.telegramRoutes(mux, h)
	a.trnRoutes(mux, h)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusNotFound, "Topilmadi") })
}

// ---- javob shakllari ----

type userOut struct {
	ID       int64    `json:"id"`
	Username string   `json:"username"`
	Rating   int      `json:"rating"`
	Rank     rankInfo `json:"rank"`
	Matches  int      `json:"matches"`
	Wins     int      `json:"wins"`
	Kills    int      `json:"kills"`
	Deaths   int      `json:"deaths"`
	HS       int      `json:"hs"`
	Created  int64    `json:"created"`
	Pos      int      `json:"pos,omitempty"`
	XP       int64    `json:"xp"`
	Level    int      `json:"level"`
	LvCur    int64    `json:"lvCur"`
	LvNeed   int64    `json:"lvNeed"`
}

func toOut(u User) userOut {
	lv, cur, need := levelOf(u.XP)
	return userOut{ID: u.ID, Username: u.Username, Rating: u.Rating, Rank: rankOf(u.Rating, u.Matches), Matches: u.Matches,
		Wins: u.Wins, Kills: u.Kills, Deaths: u.Deaths, HS: u.HS, Created: u.Created, XP: u.XP, Level: lv, LvCur: cur, LvNeed: need}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func serverErr(w http.ResponseWriter, what string, err error) {
	log.Printf("%s: %v", what, err)
	writeErr(w, http.StatusInternalServerError, "Server xatosi, keyinroq urinib ko'ring")
}

// readJSON: faqat o'z saytimizdan kelgan, kichik JSON so'rovlar (CSRF va katta so'rovlarga qarshi).
func (a *api) readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if !originOK(r, a.allowed) {
		writeErr(w, http.StatusForbidden, "So'rov boshqa saytdan keldi")
		return false
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "JSON kutilgan")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "So'rov noto'g'ri")
		return false
	}
	return true
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || strings.HasPrefix(r.Header.Get("Origin"), "https://")
}

func setSession(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", MaxAge: maxAge, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r)})
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

// currentUser: cookie bo'yicha foydalanuvchi (yo'q bo'lsa false).
func (a *api) currentUser(r *http.Request) (User, bool) {
	if !a.st.Ready() {
		return User{}, false
	}
	return a.st.Session(r.Context(), sessionToken(r))
}

// ---- endpointlar ----

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	pos, err := a.st.position(r.Context(), u.ID)
	if err != nil {
		serverErr(w, "o'rin", err)
		return
	}
	out := toOut(u)
	out.Pos = pos
	writeJSON(w, http.StatusOK, map[string]any{"user": out})
}

func (a *api) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !a.readJSON(w, r, &c) {
		return
	}
	if !a.reg.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Bu manzildan juda ko'p hisob ochildi. Keyinroq urinib ko'ring")
		return
	}
	u, token, err := a.st.Register(r.Context(), c.Username, c.Password)
	switch {
	case errors.Is(err, errBadUsername), errors.Is(err, errBadPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, errTaken):
		writeErr(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		log.Printf("ro'yxatdan o'tkazilmadi: %v", err)
		writeErr(w, http.StatusInternalServerError, "Server xatosi, keyinroq urinib ko'ring")
		return
	}
	log.Printf("yangi o'yinchi: %s (#%d)", u.Username, u.ID)
	setSession(w, r, token, int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusCreated, map[string]any{"user": toOut(u)})
}

func (a *api) doLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !a.readJSON(w, r, &c) {
		return
	}
	ip := clientIP(r)
	if !a.login.peek(ip) {
		writeErr(w, http.StatusTooManyRequests, "Juda ko'p noto'g'ri urinish. Bir daqiqadan keyin qayta urinib ko'ring")
		return
	}
	u, token, err := a.st.Login(r.Context(), c.Username, c.Password)
	if errors.Is(err, errBadLogin) {
		a.login.allow(ip) // faqat noto'g'ri urinishlar hisoblanadi
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	} else if err != nil {
		log.Printf("kirishda xato: %v", err)
		writeErr(w, http.StatusInternalServerError, "Server xatosi, keyinroq urinib ko'ring")
		return
	}
	setSession(w, r, token, int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"user": toOut(u)})
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if !originOK(r, a.allowed) {
		writeErr(w, http.StatusForbidden, "So'rov boshqa saytdan keldi")
		return
	}
	if t := sessionToken(r); t != "" {
		if err := a.st.Logout(r.Context(), t); err != nil {
			log.Printf("chiqishda xato: %v", err)
		}
	}
	setSession(w, r, "", -1)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type lbRow struct {
	Pos      int      `json:"pos"`
	Username string   `json:"username"`
	Rating   int      `json:"rating"`
	Rank     rankInfo `json:"rank"`
	Matches  int      `json:"matches"`
	WinPct   int      `json:"winPct"`
	KD       float64  `json:"kd"`
	HSPct    int      `json:"hsPct"`
}

func row(pos int, u User) lbRow {
	pct := func(a, b int) int {
		if b == 0 {
			return 0
		}
		return int(math.Round(float64(a) * 100 / float64(b)))
	}
	kd := float64(u.Kills)
	if u.Deaths > 0 {
		kd = float64(u.Kills) / float64(u.Deaths)
	}
	return lbRow{Pos: pos, Username: u.Username, Rating: u.Rating, Rank: rankOf(u.Rating, u.Matches), Matches: u.Matches,
		WinPct: pct(u.Wins, u.Matches), KD: math.Round(kd*100) / 100, HSPct: pct(u.HS, u.Kills)}
}

func (a *api) leaderboard(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var meID int64
	me, logged := a.currentUser(r)
	if logged {
		meID = me.ID
	}
	top, total, myPos, err := a.st.Leaderboard(r.Context(), limit, meID)
	if err != nil {
		serverErr(w, "reyting jadvali", err)
		return
	}
	rows := make([]lbRow, 0, len(top))
	for i, u := range top {
		rows = append(rows, row(i+1, u))
	}
	resp := map[string]any{"players": rows, "total": total, "tiers": rankTiers, "placement": placementMatches}
	if logged {
		mr := row(myPos, me)
		resp["me"] = mr
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) player(w http.ResponseWriter, r *http.Request) {
	u, pos, hist, ok, err := a.st.Profile(r.Context(), r.PathValue("name"))
	if err != nil {
		serverErr(w, "profil", err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "Bunday o'yinchi yo'q")
		return
	}
	type mOut struct {
		ID      int64  `json:"id"`
		Ended   int64  `json:"ended"`
		Room    string `json:"room"`
		ScoreT  int    `json:"sT"`
		ScoreCT int    `json:"sCT"`
		Team    string `json:"team"`
		Won     bool   `json:"won"`
		Kills   int    `json:"k"`
		Deaths  int    `json:"d"`
		HS      int    `json:"hs"`
		Before  int    `json:"before"`
		After   int    `json:"after"`
	}
	ms := []mOut{}
	for _, h := range hist {
		ms = append(ms, mOut{ID: h.MatchID, Ended: h.Ended, Room: h.Room, ScoreT: h.ScoreT, ScoreCT: h.ScoreCT, Team: h.Team, Won: h.Won,
			Kills: h.Kills, Deaths: h.Deaths, HS: h.HS, Before: h.Before, After: h.After})
	}
	out := toOut(u)
	out.Pos = pos
	det, err := a.st.PlayerDetail(r.Context(), u.ID)
	if err != nil {
		serverErr(w, "statistika", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": out, "stats": row(pos, u), "matches": ms, "detail": det})
}

// ---- oddiy "chelak" cheklagichi: har kalitga cap ta urinish, so'ng har every vaqtda bittadan ----

type limiter struct {
	mu    sync.Mutex
	cap   float64
	every time.Duration
	b     map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(capacity int, every time.Duration) *limiter {
	l := &limiter{cap: float64(capacity), every: every, b: map[string]*bucket{}}
	go func() {
		for range time.Tick(10 * time.Minute) {
			l.mu.Lock()
			for k, v := range l.b {
				if time.Since(v.last) > time.Duration(l.cap)*l.every {
					delete(l.b, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *limiter) refill(key string) *bucket {
	now := time.Now()
	b := l.b[key]
	if b == nil {
		b = &bucket{tokens: l.cap, last: now}
		l.b[key] = b
	}
	b.tokens = math.Min(l.cap, b.tokens+now.Sub(b.last).Seconds()/l.every.Seconds())
	b.last = now
	return b
}

// allow bitta urinishni sarflaydi.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refill(key)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// peek urinish qolganini tekshiradi, lekin sarflamaydi.
func (l *limiter) peek(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refill(key).tokens >= 1
}
