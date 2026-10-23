package main

import (
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
}

func newAPI(st *Store, allowed []string) *api {
	return &api{st: st, allowed: allowed,
		login: newLimiter(10, 30*time.Second), // 10 ta urinish, so'ng har 30 soniyada bittadan
		reg:   newLimiter(5, 10*time.Minute)}
}

func (a *api) routes(mux *http.ServeMux) {
	h := func(f http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if a.st == nil {
				writeErr(w, http.StatusServiceUnavailable, "Hisoblar vaqtincha ishlamayapti")
				return
			}
			f(w, r)
		}
	}
	mux.HandleFunc("GET /api/me", h(a.me))
	mux.HandleFunc("POST /api/register", h(a.register))
	mux.HandleFunc("POST /api/login", h(a.doLogin))
	mux.HandleFunc("POST /api/logout", h(a.logout))
	mux.HandleFunc("GET /api/leaderboard", h(a.leaderboard))
	mux.HandleFunc("GET /api/players/{name}", h(a.player))
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
}

func toOut(u User) userOut {
	return userOut{ID: u.ID, Username: u.Username, Rating: u.Rating, Rank: rankOf(u.Rating, u.Matches), Matches: u.Matches,
		Wins: u.Wins, Kills: u.Kills, Deaths: u.Deaths, HS: u.HS, Created: u.Created}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
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
	if a.st == nil {
		return User{}, false
	}
	return a.st.Session(sessionToken(r))
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
	_, _, pos := a.st.Leaderboard(0, u.ID)
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
	u, token, err := a.st.Register(c.Username, c.Password)
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
	u, token, err := a.st.Login(c.Username, c.Password)
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
		a.st.Logout(t)
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
	top, total, myPos := a.st.Leaderboard(limit, meID)
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
	u, pos, hist, ok := a.st.Profile(r.PathValue("name"))
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
	for _, m := range hist {
		for _, p := range m.Players {
			if p.UserID == u.ID {
				ms = append(ms, mOut{ID: m.ID, Ended: m.Ended, Room: m.Room, ScoreT: m.ScoreT, ScoreCT: m.ScoreCT, Team: p.Team, Won: p.Won,
					Kills: p.Kills, Deaths: p.Deaths, HS: p.HS, Before: p.Before, After: p.After})
			}
		}
	}
	out := toOut(u)
	out.Pos = pos
	writeJSON(w, http.StatusOK, map[string]any{"user": out, "stats": row(pos, u), "matches": ms})
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
