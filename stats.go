package main

// O'yin statistikasi va MVP.
//
// Server har bir o'yin (epoch) davomida har bir o'yinchi uchun o'ldirish, o'lim, boshga
// tegishlar, yetkazilgan zarar va raund MVP'larini yig'adi — faqat o'zi tasdiqlagan
// tegishlar va o'limlar bo'yicha. Raund oxirida g.mv (MVP), o'yin oxirida g.st (jadval)
// yuboriladi; brauzer ularni raund xabari va yakuniy natijalar oynasida ko'rsatadi.

import (
	"math"
	"sort"
	"time"
)

type pstat struct {
	name   string
	sq     string
	k, d   int
	hs     int
	dmg    float64
	mvp    int
	rk, rR int // shu raunddagi o'ldirishlar va qaysi raund
	// batafsil statistika va medallar uchun (medals.go)
	wk, whs       map[string]int     // qurol bo'yicha o'ldirish, boshga
	wdmg          map[string]float64 // qurol bo'yicha zarar
	bestRK        int                // bitta raunddagi eng ko'p o'ldirish
	clutch        int                // yutilgan eng og'ir «1 ga N»
	clR, clVs     int                // shu raundda yolg'iz qoldi (raund, nechtaga qarshi)
	plant, defuse int
}

// statRow — o'yin oxiridagi jadvalning bir qatori.
type statRow struct {
	ID  string `json:"id"`
	Nm  string `json:"nm"`
	Sq  string `json:"sq"`
	K   int    `json:"k"`
	D   int    `json:"d"`
	HS  int    `json:"hs"`
	Dmg int    `json:"dmg"`
	Mvp int    `json:"mvp"`
}

// statsOn: hozir hisob yuritiladimi (isinishda — yo'q).
func (r *room) statsOn() bool {
	g := r.gs
	return g.E != "" && g.R > 0 && (g.Ph == "freeze" || g.Ph == "live" || g.Ph == "post")
}

func (r *room) statLocked(c *client) *pstat {
	if r.stE != r.gs.E || r.st == nil {
		r.st, r.stE = map[string]*pstat{}, r.gs.E
	}
	s := r.st[c.id]
	if s == nil {
		s = &pstat{wk: map[string]int{}, whs: map[string]int{}, wdmg: map[string]float64{}}
		r.st[c.id] = s
	}
	s.name, s.sq = clientName(c), c.info.Sq
	return s
}

// noteDeathLocked: server tasdiqlagan o'lim (killer nil bo'lishi mumkin).
func (r *room) noteDeathLocked(victim, killer *client, hs bool, w string) {
	if !r.statsOn() {
		return
	}
	r.statLocked(victim).d++
	r.clutchCheckLocked()
	if killer == nil || !r.foes(killer, victim) {
		return
	}
	r.addMoneyLocked(killer, killReward(w))
	s := r.statLocked(killer)
	s.k++
	if w != "" {
		s.wk[w]++
	}
	if hs {
		s.hs++
		if w != "" {
			s.whs[w]++
		}
	}
	if s.rR != r.gs.R {
		s.rk, s.rR = 0, r.gs.R
	}
	s.rk++
	s.bestRK = max(s.bestRK, s.rk)
}

// clutchCheckLocked: kimdir jamoasida yolg'iz qoldi, raqib esa 2 va undan ko'p — «1 ga N» boshlandi.
func (r *room) clutchCheckLocked() {
	g := r.gs
	if g.respawns() || g.Ph != "live" {
		return
	}
	alive := map[string][]*client{}
	for _, c := range r.clients {
		if c.ps != nil && c.ps.alive && (c.info.Sq == "A" || c.info.Sq == "B") {
			alive[c.info.Sq] = append(alive[c.info.Sq], c)
		}
	}
	for sq, foe := range map[string]string{"A": "B", "B": "A"} {
		if len(alive[sq]) == 1 && len(alive[foe]) >= 2 {
			s := r.statLocked(alive[sq][0])
			if s.clR != g.R {
				s.clR, s.clVs = g.R, len(alive[foe])
			}
		}
	}
}

// clutchWonLocked: raund tugadi — g'olib tomonda yolg'iz qolgan bo'lsa, «1 ga N» yutildi.
func (r *room) clutchWonLocked(side string) {
	g := r.gs
	for _, s := range r.st {
		if s.clR == g.R && sideOfSquad(s.sq, g.R, g.Fm) == side {
			s.clutch = max(s.clutch, s.clVs)
		}
	}
}

// noteBombLocked: bomba o'rnatildi yoki zararsizlantirildi.
func (r *room) noteBombLocked(c *client, defuse bool) {
	if !r.statsOn() || c == nil {
		return
	}
	if s := r.statLocked(c); defuse {
		s.defuse++
	} else {
		s.plant++
	}
}

// noteDamageLocked: tasdiqlangan tegish (jondan ortig'i hisobga olinmaydi).
func (r *room) noteDamageLocked(c *client, dmg float64, w string) {
	if r.statsOn() && dmg > 0 {
		s := r.statLocked(c)
		s.dmg += dmg
		if w != "" {
			s.wdmg[w] += dmg
		}
	}
}

// roundMVPLocked: raund g'olib tomonidan eng foydali o'yinchi.
func (r *room) roundMVPLocked(side, why string) {
	g := r.gs
	g.Mv, g.Mvn, g.Mvr = "", "", ""
	pick := ""
	switch {
	case why == "defuse" && r.defuser != "":
		pick, g.Mvr = r.defuser, "defuse"
	case why == "bomb" && r.planter != "":
		pick, g.Mvr = r.planter, "bomb"
	default:
		best := 0
		ids := make([]string, 0, len(r.st))
		for id := range r.st {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			s := r.st[id]
			if s.rR == g.R && s.rk > best && sideOfSquad(s.sq, g.R, g.Fm) == side {
				pick, best = id, s.rk
			}
		}
		if pick != "" {
			g.Mvr = "k" + itoa(best)
		}
	}
	if pick == "" {
		return
	}
	s := r.st[pick]
	if s == nil {
		if c := r.clients[pick]; c != nil && r.statsOn() {
			s = r.statLocked(c)
		}
	}
	if s == nil {
		g.Mvr = ""
		return
	}
	s.mvp++
	g.Mv, g.Mvn = pick, s.name
}

// finalStatsLocked: o'yin oxiridagi jadval (o'ldirish bo'yicha).
func (r *room) finalStatsLocked() {
	g := r.gs
	g.St = nil
	if g.E == "" {
		return
	}
	for _, c := range r.clients { // o'ldirmagan va o'lmagan o'yinchilar ham jadvalda bo'lsin
		if c.info.Sq != "" {
			r.statLocked(c)
		}
	}
	for id, s := range r.st {
		g.St = append(g.St, statRow{ID: id, Nm: s.name, Sq: s.sq, K: s.k, D: s.d, HS: s.hs, Dmg: int(math.Round(s.dmg)), Mvp: s.mvp})
	}
	sort.Slice(g.St, func(i, j int) bool {
		a, b := g.St[i], g.St[j]
		if a.K != b.K {
			return a.K > b.K
		}
		if a.D != b.D {
			return a.D < b.D
		}
		return a.ID < b.ID
	})
	if len(g.St) > 32 {
		g.St = g.St[:32]
	}
	r.gains = r.gainsLocked(time.Now()) // XP va kunlik vazifalar (progress.go)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
