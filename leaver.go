package main

// Reytingli (navbat) o'yinni tashlab ketganlik uchun jazo.
//
// Navbatdan topilgan o'yinchi xonaga kelmasa yoki o'yin tugamasdan chiqib ketib leaverGrace ichida
// qaytmasa — «tashlab ketdi» deb yoziladi va navbatga vaqtincha kira olmaydi: oxirgi 24 soatdagi
// 1-marta 5 daqiqa, 2-marta 30 daqiqa, 3-marta va undan ko'p — 2 soat. Shu vaqt ichida menyuda
// «Qayta ulanish» tugmasi chiqadi (GET /api/queue → rejoin). O'yin tugagach (Ph == "over")
// hech kim jazolanmaydi.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"time"
)

const leaverGrace = 150 * time.Second // qayta ulanish uchun vaqt (mm xonasi undan keyin o'chadi: mmEmptyTTL)

// grace: hub'ning qayta ulanish vaqti (testlarda qisqa).
func (h *hub) grace() time.Duration {
	if h.leaverGrace > 0 {
		return h.leaverGrace
	}
	return leaverGrace
}

var leaverSteps = []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

const leaverWindow = 24 * time.Hour

// leaverBan: shuncha marta tashlab ketgan bo'lsa, qancha taqiq.
func leaverBan(strikes int) time.Duration {
	if strikes < 1 {
		return 0
	}
	return leaverSteps[min(strikes, len(leaverSteps))-1]
}

// mmPresentLocked: o'yinchi mm xonasiga kirdi (yoki qaytdi).
func (r *room) mmPresentLocked(uid int64) {
	if r.mm != nil {
		delete(r.mm.absent, uid)
	}
}

// mmAbsentLocked: o'yinchi mm xonasidan chiqdi — qaytish uchun vaqt hisoblanadi.
func (r *room) mmAbsentLocked(uid int64, now time.Time) {
	if r.mm != nil && !r.mm.done && r.mm.sq[uid] != "" {
		r.mm.absent[uid] = now
	}
}

// mmLeaversLocked: qaytish vaqti tugagan o'yinchilar (har biri bir marta).
func (r *room) mmLeaversLocked(now time.Time, grace time.Duration) []int64 {
	m := r.mm
	if m == nil || m.done {
		return nil
	}
	if r.gs.Ph == "over" {
		m.done, m.absent = true, map[int64]time.Time{}
		return nil
	}
	var out []int64
	for uid, at := range m.absent {
		if now.Sub(at) >= grace && !m.punished[uid] {
			m.punished[uid] = true
			delete(m.absent, uid)
			out = append(out, uid)
		}
	}
	return out
}

func (h *hub) punishLeavers(code string, uids []int64) {
	if !h.store.Ready() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, uid := range uids {
		until, n, err := h.store.AddLeave(ctx, uid, time.Now())
		if err != nil {
			log.Printf("tashlab ketish (%d): %v", uid, err)
			continue
		}
		log.Printf("%s: o'yinchi %d o'yinni tashlab ketdi (%d-marta), navbat taqiqi %s gacha", code, uid, n, until.Format("15:04"))
	}
}

// mmRejoin: o'yinchi tashlab chiqqan, hali qaytsa bo'ladigan navbat o'yini.
type mmRejoin struct {
	Room string `json:"room"`
	Map  string `json:"map"`
	Mode string `json:"mode"`
	Sq   string `json:"sq"`
	Left int    `json:"left"` // qaytish uchun qolgan soniya
}

func (h *hub) mmRejoinFor(uid int64, now time.Time) *mmRejoin {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.rooms {
		if r.mm == nil {
			continue
		}
		r.mu.Lock()
		at, ok := r.mm.absent[uid]
		var out *mmRejoin
		if ok && !r.mm.done {
			left := int(math.Ceil((h.grace() - now.Sub(at)).Seconds()))
			if left > 0 {
				out = &mmRejoin{Room: r.name[len("zn-"):], Map: r.gs.M, Mode: r.gs.modeID(), Sq: r.mm.sq[uid], Left: left}
			}
		}
		r.mu.Unlock()
		if out != nil {
			return out
		}
	}
	return nil
}

// ---- baza ----

// AddLeave: tashlab ketishni yozadi; yangi taqiq tugash vaqti va 24 soatdagi soni.
func (s *Store) AddLeave(ctx context.Context, uid int64, now time.Time) (time.Time, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer tx.Rollback()
	var strikes int
	var last time.Time
	err = tx.QueryRowContext(ctx, `SELECT strikes, last_at FROM leaver_bans WHERE user_id = $1 FOR UPDATE`, uid).Scan(&strikes, &last)
	if errors.Is(err, sql.ErrNoRows) {
		strikes = 0
	} else if err != nil {
		return time.Time{}, 0, err
	}
	if now.Sub(last) > leaverWindow {
		strikes = 0
	}
	strikes++
	until := now.Add(leaverBan(strikes))
	if _, err := tx.ExecContext(ctx, `INSERT INTO leaver_bans (user_id, strikes, until_at, last_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET strikes = $2, until_at = $3, last_at = $4`, uid, strikes, until, now); err != nil {
		return time.Time{}, 0, err
	}
	return until, strikes, tx.Commit()
}

// QueueBan: navbat taqiqi qachon tugaydi (o'tgan bo'lsa — nol vaqt).
func (s *Store) QueueBan(ctx context.Context, uid int64, now time.Time) time.Time {
	if !s.Ready() {
		return time.Time{}
	}
	var until time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT until_at FROM leaver_bans WHERE user_id = $1`, uid).Scan(&until); err != nil || !until.After(now) {
		return time.Time{}
	}
	return until
}

func banError(until, now time.Time) error {
	m := int(until.Sub(now).Minutes()) + 1
	return fmt.Errorf("Siz yaqinda reytingli o'yinni tashlab ketdingiz: navbatga %d daqiqadan keyin kira olasiz", m)
}
