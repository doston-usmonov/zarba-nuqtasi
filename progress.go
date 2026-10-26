package main

// Tajriba (XP), darajalar, kunlik vazifalar va qurol bo'yoqlari.
//
// XP faqat hisobga kirgan o'yinchilarga, onlayn o'yin oxirigacha o'ynalganda beriladi.
// Hisob server tasdiqlagan statistikadan (stats.go) olinadi: o'ldirishlar, MVP, g'alaba.
// Botlar qatnashgan o'yinda XP ikki baravar kam. Har kuni (Toshkent vaqti bilan) har bir
// o'yinchiga 3 ta vazifa beriladi; bajarilsa qo'shimcha XP. Daraja oshgan sari yangi qurol
// bo'yoqlari ochiladi — hammasi o'zimizning dizayn, sotilmaydi, tasodifiy sandiq yo'q.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"hash/fnv"
	"log"
	"net/http"
	"strconv"
	"time"
)

const maxLevel = 40

// levelStart: shu darajaga yetish uchun jami XP (1-daraja — 0).
func levelStart(l int) int64 {
	if l <= 1 {
		return 0
	}
	n := int64(l - 1)
	return 400*n + 50*n*(n+1)
}

// levelOf: daraja, shu darajadagi XP va keyingi darajagacha kerak bo'lgani.
func levelOf(xp int64) (lv int, cur, need int64) {
	lv = 1
	for lv < maxLevel && xp >= levelStart(lv+1) {
		lv++
	}
	if lv == maxLevel {
		return lv, xp - levelStart(lv), 0
	}
	return lv, xp - levelStart(lv), levelStart(lv+1) - levelStart(lv)
}

// ---- qurol bo'yoqlari (web/index.html dagi FINISHES bilan bir xil nomlar) ----

type finishDef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

var finishes = []finishDef{
	{"qum", "Qum", 2}, {"tun", "Tun", 4}, {"zangori", "Zangori", 6}, {"atlas", "Atlas", 8}, {"lola", "Lola", 11},
	{"gilam", "Gilam", 14}, {"shoh", "Shoh", 18}, {"olov", "Olov", 23}, {"muz", "Muz", 28}, {"afsona", "Afsona", 35},
}

var finishWeapons = []string{"rifle", "carbine", "smg", "sniper", "deagle", "pistol", "knife"}

func finishLevel(id string) int {
	for _, f := range finishes {
		if f.ID == id {
			return f.Level
		}
	}
	return 0
}

// ---- kunlik vazifalar ----

type missionDef struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Target int    `json:"target"`
	Reward int    `json:"reward"`
	metric string
	best   bool // bir o'yindagi eng yaxshi natija (yig'ilmaydi)
}

var missionPool = []missionDef{
	{ID: "k10", Text: "10 ta raqibni o'ldiring", Target: 10, Reward: 150, metric: "kills"},
	{ID: "k25", Text: "25 ta raqibni o'ldiring", Target: 25, Reward: 300, metric: "kills"},
	{ID: "hs5", Text: "Boshga 5 marta tegib o'ldiring", Target: 5, Reward: 200, metric: "hs"},
	{ID: "win1", Text: "1 ta o'yinda g'alaba qozoning", Target: 1, Reward: 250, metric: "wins"},
	{ID: "mvp2", Text: "2 marta raund MVP bo'ling", Target: 2, Reward: 200, metric: "mvp"},
	{ID: "dm15", Text: "Deathmatch'da bitta o'yinda 15 ta o'ldiring", Target: 15, Reward: 250, metric: "dmkills", best: true},
	{ID: "gg1", Text: "Qurol poygasida pichoq bosqichiga yeting", Target: 1, Reward: 250, metric: "ggknife"},
	{ID: "play3", Text: "3 ta o'yinni oxirigacha o'ynang", Target: 3, Reward: 200, metric: "matches"},
	{ID: "dmg2k", Text: "Jami 2000 zarar yetkazing", Target: 2000, Reward: 200, metric: "dmg"},
}

var tashkent = time.FixedZone("UZT", 5*3600)

func dayOf(t time.Time) string { return t.In(tashkent).Format("2006-01-02") }

// dailyMissions: o'yinchining bugungi 3 ta vazifasi (har kim uchun boshqacha, lekin kun davomida o'zgarmaydi).
func dailyMissions(uid int64, day string) []missionDef {
	h := fnv.New64a()
	h.Write([]byte(strconv.FormatInt(uid, 10) + "|" + day))
	seed := h.Sum64()
	idx := make([]int, len(missionPool))
	for i := range idx {
		idx[i] = i
	}
	for i := len(idx) - 1; i > 0; i-- {
		seed = seed*6364136223846793005 + 1442695040888963407
		j := int((seed >> 33) % uint64(i+1))
		idx[i], idx[j] = idx[j], idx[i]
	}
	out := make([]missionDef, 3)
	for i := range out {
		out[i] = missionPool[idx[i]]
	}
	return out
}

// matchGain: bitta o'yinchining bitta o'yindan olgani.
type matchGain struct {
	uid     int64
	mode    string
	xp      int
	metrics map[string]int
}

// gainsLocked: o'yin tugadi — hisobga kirgan har bir o'yinchiga XP va vazifa natijalari.
func (r *room) gainsLocked(now time.Time) []matchGain {
	g := r.gs
	if r.stE != g.E || g.E == "" || now.Sub(r.matchAt) < minMatchTime(g) {
		return nil
	}
	hasBot := false
	for _, c := range r.clients {
		if c.bot != nil {
			hasBot = true
		}
	}
	var out []matchGain
	for _, c := range r.clients {
		s := r.st[c.id]
		if c.acct == nil || s == nil || c.info.Sq == "" {
			continue
		}
		won := false
		switch g.Md {
		case modeDM, modeGG:
			won = g.Wn == c.id || (g.Tm && g.Wn == s.sq)
		default:
			won = (g.SA > g.SB) == (s.sq == "A")
		}
		xp := 0
		switch g.Md {
		case modeDM, modeGG:
			xp = 50 + 5*s.k
			if won {
				xp += 75
			}
		default:
			xp = 100 + 10*s.k + 15*s.mvp
			if won {
				xp += 100
			}
		}
		if hasBot {
			xp /= 2
		}
		m := map[string]int{"kills": s.k, "hs": s.hs, "mvp": s.mvp, "matches": 1, "dmg": int(s.dmg)}
		if won {
			m["wins"] = 1
		}
		if g.Md == modeDM {
			m["dmkills"] = s.k
		}
		if g.Md == modeGG && ggLevel(g.Sc[c.id]) == len(ggLadder)-1 {
			m["ggknife"] = 1
		}
		out = append(out, matchGain{uid: c.acct.id, mode: g.Md, xp: xp, metrics: m})
	}
	return out
}

func minMatchTime(g *gameState) time.Duration {
	switch g.Md {
	case modeDM:
		return 4 * time.Minute
	case modeGG:
		return 90 * time.Second
	}
	return time.Duration(g.SA+g.SB) * 20 * time.Second
}

// ---- baza ----

type missionState struct {
	missionDef
	Progress int  `json:"progress"`
	Done     bool `json:"done"`
}

// ApplyGain: XP va vazifalarni bitta tranzaksiyada yozadi. Bajarilgan vazifalarni qaytaradi.
func (s *Store) ApplyGain(ctx context.Context, g matchGain, now time.Time) (xp int64, done []missionDef, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	day := dayOf(now)
	total := g.xp
	for _, m := range dailyMissions(g.uid, day) {
		v := g.metrics[m.metric]
		if v <= 0 {
			continue
		}
		upd := `progress = daily_progress.progress + EXCLUDED.progress`
		if m.best {
			upd = `progress = GREATEST(daily_progress.progress, EXCLUDED.progress)`
		}
		var prog int
		var was bool
		if err := tx.QueryRowContext(ctx, `INSERT INTO daily_progress (user_id, day, mission, progress) VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, day, mission) DO UPDATE SET `+upd+` RETURNING progress, done`, g.uid, day, m.ID, v).Scan(&prog, &was); err != nil {
			return 0, nil, err
		}
		if !was && prog >= m.Target {
			if _, err := tx.ExecContext(ctx, `UPDATE daily_progress SET done = true WHERE user_id = $1 AND day = $2 AND mission = $3`, g.uid, day, m.ID); err != nil {
				return 0, nil, err
			}
			total += m.Reward
			done = append(done, m)
		}
	}
	if err := tx.QueryRowContext(ctx, `UPDATE users SET xp = xp + $2 WHERE id = $1 RETURNING xp`, g.uid, total).Scan(&xp); err != nil {
		return 0, nil, err
	}
	return xp, done, tx.Commit()
}

// Progress: o'yinchining XP'si, bugungi vazifalari va tanlangan bo'yoqlari.
func (s *Store) Progress(ctx context.Context, uid int64, now time.Time) (int64, []missionState, map[string]string, error) {
	var xp int64
	var lo []byte
	if err := s.db.QueryRowContext(ctx, `SELECT xp, loadout FROM users WHERE id = $1`, uid).Scan(&xp, &lo); err != nil {
		return 0, nil, nil, err
	}
	loadout := map[string]string{}
	json.Unmarshal(lo, &loadout)
	day := dayOf(now)
	var ms []missionState
	for _, m := range dailyMissions(uid, day) {
		st := missionState{missionDef: m}
		err := s.db.QueryRowContext(ctx, `SELECT progress, done FROM daily_progress WHERE user_id = $1 AND day = $2 AND mission = $3`, uid, day, m.ID).Scan(&st.Progress, &st.Done)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, nil, nil, err
		}
		ms = append(ms, st)
	}
	return xp, ms, loadout, nil
}

// SetFinish: qurolga bo'yoq tanlash (daraja yetgan bo'lsa). f == "" — oddiy ko'rinish.
func (s *Store) SetFinish(ctx context.Context, uid int64, w, f string) (map[string]string, error) {
	var xp int64
	var lo []byte
	if err := s.db.QueryRowContext(ctx, `SELECT xp, loadout FROM users WHERE id = $1`, uid).Scan(&xp, &lo); err != nil {
		return nil, err
	}
	loadout := map[string]string{}
	json.Unmarshal(lo, &loadout)
	lv, _, _ := levelOf(xp)
	if f == "" {
		delete(loadout, w)
	} else if need := finishLevel(f); need == 0 || need > lv {
		return nil, errLocked
	} else {
		loadout[w] = f
	}
	b, _ := json.Marshal(loadout)
	_, err := s.db.ExecContext(ctx, `UPDATE users SET loadout = $2 WHERE id = $1`, uid, b)
	return loadout, err
}

var errLocked = errors.New("Bu bo'yoq hali ochilmagan")

// ---- API ----

type progressOut struct {
	XP       int64             `json:"xp"`
	Level    int               `json:"level"`
	Cur      int64             `json:"cur"`
	Need     int64             `json:"need"`
	Missions []missionState    `json:"missions"`
	Finishes []finishDef       `json:"finishes"`
	Loadout  map[string]string `json:"loadout"`
	Weapons  []string          `json:"weapons"`
	Resets   int64             `json:"resets"` // vazifalar yangilanishigacha (soniya)
}

func (a *api) progress(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	now := time.Now()
	xp, ms, lo, err := a.st.Progress(r.Context(), u.ID, now)
	if err != nil {
		serverErr(w, "progress", err)
		return
	}
	lv, cur, need := levelOf(xp)
	t := now.In(tashkent)
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, tashkent)
	writeJSON(w, http.StatusOK, progressOut{XP: xp, Level: lv, Cur: cur, Need: need, Missions: ms, Finishes: finishes, Loadout: lo, Weapons: finishWeapons, Resets: int64(next.Sub(now).Seconds())})
}

func (a *api) setFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		W string `json:"w"`
		F string `json:"f"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	known := false
	for _, x := range finishWeapons {
		known = known || x == req.W
	}
	if !known {
		writeErr(w, http.StatusBadRequest, "Noma'lum qurol")
		return
	}
	lo, err := a.st.SetFinish(r.Context(), u.ID, req.W, req.F)
	if errors.Is(err, errLocked) {
		writeErr(w, http.StatusForbidden, err.Error())
		return
	} else if err != nil {
		serverErr(w, "bo'yoq", err)
		return
	}
	if a.onLoadout != nil {
		a.onLoadout(u.ID, lo)
	}
	writeJSON(w, http.StatusOK, map[string]any{"loadout": lo})
}

// ---- xonada: o'yin oxirida XP, o'yin davomida bo'yoqlar ----

// awardProgress: bazaga yozadi va o'yinchilarga {"t":"xp"} yuboradi.
func (h *hub) awardProgress(r *room, gains []matchGain) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	now := time.Now()
	type res struct {
		xp   int64
		gain int
		done []missionDef
	}
	results := map[int64]res{}
	for _, g := range gains {
		xp, done, err := h.store.ApplyGain(ctx, g, now)
		if err != nil {
			log.Printf("XP yozilmadi (#%d): %v", g.uid, err)
			continue
		}
		gain := g.xp
		for _, d := range done {
			gain += d.Reward
		}
		results[g.uid] = res{xp, gain, done}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.clients {
		if c.acct == nil {
			continue
		}
		x, ok := results[c.acct.id]
		if !ok {
			continue
		}
		lv, cur, need := levelOf(x.xp)
		before, _, _ := levelOf(x.xp - int64(x.gain))
		c.trySend(jsonMsg(map[string]any{"t": "xp", "gain": x.gain, "xp": x.xp, "level": lv, "cur": cur, "need": need, "up": lv > before, "done": x.done}))
	}
}

// loadoutCache: hisob → qurol bo'yoqlari (presence'ga server yozadi, o'zgartirib bo'lmaydi).
func (h *hub) setLoadout(uid int64, lo map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loadouts == nil {
		h.loadouts = map[int64]map[string]string{}
	}
	h.loadouts[uid] = lo
	for _, r := range h.rooms { // tartib: h.mu → r.mu (hub.leave bilan bir xil)
		r.mu.Lock()
		for _, c := range r.clients {
			if c.acct != nil && c.acct.id == uid {
				c.loadout = lo
			}
		}
		r.mu.Unlock()
	}
}

// Loadout: hisobning qurol bo'yoqlari.
func (s *Store) Loadout(ctx context.Context, uid int64) (map[string]string, error) {
	var lo []byte
	if err := s.db.QueryRowContext(ctx, `SELECT loadout FROM users WHERE id = $1`, uid).Scan(&lo); err != nil {
		return nil, err
	}
	out := map[string]string{}
	json.Unmarshal(lo, &out)
	return out, nil
}
