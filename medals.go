package main

// Batafsil statistika va medallar (yutuqlar).
//
// O'yin oxirida (XP bilan birga, progress.go) hisobga kirgan har bir o'yinchining server tasdiqlagan
// natijalari player_stats jadvalidagi hisoblagichlarga qo'shiladi: jami (k, d, hs, dmg, mvp, matches,
// wins, plant, defuse), qurollar bo'yicha (wk:<qurol> o'ldirish, whs:<qurol> boshga, wdmg:<qurol>
// zarar), xaritalar (mp:/mw:<xarita> o'ynagan/yutgan) va rejimlar (md:/mdw:<rejim>), eng yaxshilari
// (best_rk — bitta raunddagi, best_mk — bitta o'yindagi o'ldirish, clutch — yutilgan eng og'ir «1 ga N»).
// Hisoblagich chegaraga yetganda medal ochiladi (achievements jadvali) va qo'shimcha XP beriladi.
// Hammasi bitta tranzaksiyada (ApplyGain).

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

type medalDef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Text   string `json:"text"`
	Key    string `json:"key"`
	Target int64  `json:"target"`
	XP     int    `json:"xp"`
}

// medalDefs: o'zimizning medallar (nomlari va shartlari o'yinniki).
var medalDefs = []medalDef{
	{"birinchi", "Birinchi qadam", "Birinchi o'yinni oxirigacha o'ynang", "matches", 1, 50},
	{"beshlik", "Beshlik", "Bitta raundda 5 ta raqibni o'ldiring", "best_rk", 5, 400},
	{"yolgiz", "Yolg'iz qahramon", "Bir o'zingiz 3 (yoki ko'proq) raqibga qarshi qolib raundni yuting", "clutch", 3, 400},
	{"ayovsiz", "Ayovsiz", "Bitta o'yinda 30 ta raqibni o'ldiring", "best_mk", 30, 300},
	{"pichoq", "Pichoq ustasi", "Pichoq bilan 10 ta raqibni o'ldiring", "wk:knife", 10, 250},
	{"uchqun", "Uchqun", "«Uchqun» elektroshokeri bilan 5 ta raqibni o'ldiring", "wk:taser", 5, 250},
	{"qirgiy", "Qirg'iy ko'zi", "Qirg'iy-50 snayper miltig'i bilan 100 ta raqibni o'ldiring", "wk:sniper", 100, 300},
	{"aniq", "Aniq nishon", "Boshga tegib 250 ta raqibni o'ldiring", "hs", 250, 300},
	{"portlash", "Portlatuvchi", "Portlovchi granata bilan 20 ta raqibni o'ldiring", "wk:he", 20, 250},
	{"olov", "Olovli", "Yondiruvchi granata bilan 10 ta raqibni o'ldiring", "wk:molly", 10, 250},
	{"sapyor", "Sapyor", "25 marta bomba o'rnating", "plant", 25, 250},
	{"qutqaruvchi", "Qutqaruvchi", "25 marta bombani zararsizlantiring", "defuse", 25, 250},
	{"yulduz", "Yulduz", "50 marta raund MVP bo'ling", "mvp", 50, 300},
	{"golib", "G'olib", "50 ta o'yinda g'alaba qozoning", "wins", 50, 300},
	{"faxriy", "Faxriy", "100 ta o'yinni oxirigacha o'ynang", "matches", 100, 300},
	{"mingtalik", "Mingtalik", "Jami 1000 ta raqibni o'ldiring", "k", 1000, 500},
	{"poyga", "Poyga sardori", "Qurol poygasida 5 marta g'alaba qozoning", "mdw:gg", 5, 250},
	{"aimchi", "Aim ustasi", "Aim rejimida 10 ta o'yinda g'alaba qozoning", "mdw:arena", 10, 250},
}

// bestKeys: bu hisoblagichlar qo'shilmaydi — eng kattasi saqlanadi.
var bestKeys = map[string]bool{"best_rk": true, "best_mk": true, "clutch": true}

// applyStatsTx: o'yin natijalarini hisoblagichlarga qo'shadi va yangi ochilgan medallarni qaytaradi.
func applyStatsTx(ctx context.Context, tx *sql.Tx, uid int64, stats map[string]int, now time.Time) ([]medalDef, error) {
	if len(stats) == 0 {
		return nil, nil
	}
	var addK, bestK []string
	var addV, bestV []int64
	for k, v := range stats {
		if v <= 0 {
			continue
		}
		if bestKeys[k] {
			bestK, bestV = append(bestK, k), append(bestV, int64(v))
		} else {
			addK, addV = append(addK, k), append(addV, int64(v))
		}
	}
	if len(addK) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO player_stats (user_id, key, value) SELECT $1, k, v FROM unnest($2::text[], $3::bigint[]) AS t(k, v)
			ON CONFLICT (user_id, key) DO UPDATE SET value = player_stats.value + EXCLUDED.value`, uid, pq.Array(addK), pq.Array(addV)); err != nil {
			return nil, err
		}
	}
	if len(bestK) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO player_stats (user_id, key, value) SELECT $1, k, v FROM unnest($2::text[], $3::bigint[]) AS t(k, v)
			ON CONFLICT (user_id, key) DO UPDATE SET value = GREATEST(player_stats.value, EXCLUDED.value)`, uid, pq.Array(bestK), pq.Array(bestV)); err != nil {
			return nil, err
		}
	}
	// medallar: shartdagi hisoblagichlar
	keys := make([]string, 0, len(medalDefs))
	for _, m := range medalDefs {
		keys = append(keys, m.Key)
	}
	vals, err := statValues(ctx, tx, uid, keys)
	if err != nil {
		return nil, err
	}
	var won []medalDef
	for _, m := range medalDefs {
		if vals[m.Key] < m.Target {
			continue
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO achievements (user_id, id, at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, uid, m.ID, now)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			won = append(won, m)
		}
	}
	return won, nil
}

type queryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func statValues(ctx context.Context, q queryer, uid int64, keys []string) (map[string]int64, error) {
	var rows *sql.Rows
	var err error
	if keys == nil {
		rows, err = q.QueryContext(ctx, `SELECT key, value FROM player_stats WHERE user_id = $1`, uid)
	} else {
		rows, err = q.QueryContext(ctx, `SELECT key, value FROM player_stats WHERE user_id = $1 AND key = ANY($2)`, uid, pq.Array(keys))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ---- profil ----

type medalOut struct {
	medalDef
	Have int64 `json:"have"`         // hozirgi qiymat (chegaraga qadar)
	At   int64 `json:"at,omitempty"` // ochilgan vaqt (unix), 0 — hali yo'q
}

type weaponStat struct {
	W   string `json:"w"`
	K   int64  `json:"k"`
	HS  int64  `json:"hs"`
	Dmg int64  `json:"dmg"`
}

type mapStat struct {
	M      string `json:"m"`
	Played int64  `json:"p"`
	Won    int64  `json:"w"`
}

type playerDetail struct {
	Totals  map[string]int64 `json:"totals"` // k, d, hs, dmg, mvp, matches, wins, plant, defuse, best_rk, best_mk, clutch
	Weapons []weaponStat     `json:"weapons"`
	Maps    []mapStat        `json:"maps"`
	Modes   []mapStat        `json:"modes"`
	Medals  []medalOut       `json:"medals"`
}

// PlayerDetail: profil uchun batafsil statistika va medallar.
func (s *Store) PlayerDetail(ctx context.Context, uid int64) (playerDetail, error) {
	out := playerDetail{Totals: map[string]int64{}, Weapons: []weaponStat{}, Maps: []mapStat{}, Modes: []mapStat{}, Medals: []medalOut{}}
	vals, err := statValues(ctx, s.db, uid, nil)
	if err != nil {
		return out, err
	}
	ws, ms, ds := map[string]*weaponStat{}, map[string]*mapStat{}, map[string]*mapStat{}
	for k, v := range vals {
		pre, id, ok := strings.Cut(k, ":")
		if !ok {
			out.Totals[k] = v
			continue
		}
		switch pre {
		case "wk", "whs", "wdmg":
			w := ws[id]
			if w == nil {
				w = &weaponStat{W: id}
				ws[id] = w
			}
			switch pre {
			case "wk":
				w.K = v
			case "whs":
				w.HS = v
			default:
				w.Dmg = v
			}
		case "mp", "mw":
			m := ms[id]
			if m == nil {
				m = &mapStat{M: id}
				ms[id] = m
			}
			if pre == "mp" {
				m.Played = v
			} else {
				m.Won = v
			}
		case "md", "mdw":
			m := ds[id]
			if m == nil {
				m = &mapStat{M: id}
				ds[id] = m
			}
			if pre == "md" {
				m.Played = v
			} else {
				m.Won = v
			}
		}
	}
	for _, w := range ws {
		out.Weapons = append(out.Weapons, *w)
	}
	for _, m := range ms {
		out.Maps = append(out.Maps, *m)
	}
	for _, m := range ds {
		out.Modes = append(out.Modes, *m)
	}
	byPlayed := func(a []mapStat) func(i, j int) bool {
		return func(i, j int) bool {
			return a[i].Played > a[j].Played || (a[i].Played == a[j].Played && a[i].M < a[j].M)
		}
	}
	sort.Slice(out.Maps, byPlayed(out.Maps))
	sort.Slice(out.Modes, byPlayed(out.Modes))
	sort.Slice(out.Weapons, func(i, j int) bool {
		a, b := out.Weapons[i], out.Weapons[j]
		return a.K > b.K || (a.K == b.K && a.W < b.W)
	})
	got := map[string]int64{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, extract(epoch FROM at)::bigint FROM achievements WHERE user_id = $1`, uid)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return out, err
		}
		got[id] = at
	}
	for _, m := range medalDefs {
		out.Medals = append(out.Medals, medalOut{medalDef: m, Have: min(vals[m.Key], m.Target), At: got[m.ID]})
	}
	return out, rows.Err()
}
