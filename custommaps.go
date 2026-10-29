package main

// Foydalanuvchi xaritalari (xarita muharriri).
//
// O'yinchi brauzerda 44×44 panjarani chizadi (devor, yo'l, quti, baland quti, A va B nuqtalar,
// ikki jamoaning bazasi) va hisobiga saqlaydi. Saqlangan xarita galereyada hammaga ko'rinadi:
// botlar bilan o'ynash yoki u bilan onlayn xona ochish mumkin. Xona xaritasining nomi «k<id>»
// (masalan k12) — server xaritani bazadan o'qiydi va botlar uchun joylarni o'zi hisoblaydi (autoPlan).
//
// Tekshiruv (validateCustomRows): chegara — devor; A va B kamida 9 katak, har jamoa bazasi kamida
// 12 katak; hujumchilar bazasidan ikkala nuqtaga va himoyachilar bazasiga yo'l bor (quti to'siq hisoblanadi).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	maxUserMaps   = 30
	customMapsTTL = 30 * time.Minute
)

var (
	customIDRe  = regexp.MustCompile(`^k[1-9][0-9]{0,11}$`)
	mapStyles   = map[string]bool{"qum": true, "bozor": true, "karvon": true, "bekat": true, "saroy": true, "zavod": true}
	errMapName  = errors.New("Xarita nomi 3–32 harf bo'lsin")
	errMapStyle = errors.New("Bunday ko'rinish yo'q")
	errMapLimit = fmt.Errorf("Ko'pi bilan %d ta xarita saqlash mumkin — keraksizini o'chiring", maxUserMaps)
	errMapOwner = errors.New("Bu xarita sizniki emas")
	errMapNone  = errors.New("Xarita topilmadi")
)

func isCustomMapID(id string) bool { return customIDRe.MatchString(id) }

// ---- kesh: xonalar va havola sarlavhalari uchun ----

type customCache struct {
	mu sync.Mutex
	m  map[string]*gameMap
	at map[string]time.Time
}

var customMaps = &customCache{m: map[string]*gameMap{}, at: map[string]time.Time{}}

func (c *customCache) get(id string) *gameMap {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[id]
}

func (c *customCache) put(gm *gameMap) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.m[gm.id], c.at[gm.id] = gm, now
	if len(c.m) > 300 { // eskilarini tashlaymiz (xonalar o'z nusxasini r.cmap da saqlaydi)
		for k, t := range c.at {
			if now.Sub(t) > customMapsTTL {
				delete(c.m, k)
				delete(c.at, k)
			}
		}
	}
}

func (c *customCache) drop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, id)
	delete(c.at, id)
}

// ---- panjara: tekshirish va botlar uchun joylar ----

const mapChars = "#.csABtk"

func walkCell(ch byte) bool { return ch == '.' || ch == 'A' || ch == 'B' || ch == 't' || ch == 'k' }

// validateCustomRows: 44 qator × 44 belgi; chegara devorga aylantiriladi. Xato — o'zbekcha izoh.
func validateCustomRows(raw string) ([]string, error) {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r", ""), "\n", "")
	if len(raw) != gridW*gridH {
		return nil, errors.New("Xarita o'lchami noto'g'ri")
	}
	b := []byte(raw)
	count := map[byte]int{}
	for i, ch := range b {
		if !strings.ContainsRune(mapChars, rune(ch)) {
			return nil, errors.New("Xaritada noma'lum belgi bor")
		}
		x, z := i%gridW, i/gridW
		if x == 0 || z == 0 || x == gridW-1 || z == gridH-1 {
			b[i] = '#'
		}
		count[b[i]]++
	}
	switch {
	case count['A'] < 9:
		return nil, errors.New("A nuqta kamida 9 katak bo'lsin")
	case count['B'] < 9:
		return nil, errors.New("B nuqta kamida 9 katak bo'lsin")
	case count['t'] < 12:
		return nil, errors.New("Hujumchilar bazasi kamida 12 katak bo'lsin")
	case count['k'] < 12:
		return nil, errors.New("Himoyachilar bazasi kamida 12 katak bo'lsin")
	}
	rows := make([]string, gridH)
	for z := range rows {
		rows[z] = string(b[z*gridW : (z+1)*gridW])
	}
	// yo'llar: hujumchilar bazasidan hamma joyga
	start := -1
	for i, ch := range b {
		if ch == 't' {
			start = i
			break
		}
	}
	dist := bfsRows(rows, start%gridW, start/gridW)
	reach := map[byte]bool{}
	for i, d := range dist {
		if d >= 0 {
			reach[b[i]] = true
		}
	}
	switch {
	case !reach['A']:
		return nil, errors.New("Hujumchilar bazasidan A nuqtaga yo'l yo'q")
	case !reach['B']:
		return nil, errors.New("Hujumchilar bazasidan B nuqtaga yo'l yo'q")
	case !reach['k']:
		return nil, errors.New("Ikki jamoa bazasi orasida yo'l yo'q")
	}
	return rows, nil
}

// bfsRows: (x,z) dan yuriladigan kataklar bo'yicha masofa (-1 — yetib bo'lmaydi).
func bfsRows(rows []string, sx, sz int) []int {
	d := make([]int, gridW*gridH)
	for i := range d {
		d[i] = -1
	}
	if sx < 0 || sz < 0 || !walkCell(rows[sz][sx]) {
		return d
	}
	q := []int{sz*gridW + sx}
	d[q[0]] = 0
	for len(q) > 0 {
		i := q[0]
		q = q[1:]
		x, z := i%gridW, i/gridW
		for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, nz := x+o[0], z+o[1]
			if nx < 0 || nz < 0 || nx >= gridW || nz >= gridH || !walkCell(rows[nz][nx]) {
				continue
			}
			if j := nz*gridW + nx; d[j] < 0 {
				d[j] = d[i] + 1
				q = append(q, j)
			}
		}
	}
	return d
}

// zoneMid: shu belgili kataklarning o'rtasiga eng yaqini.
func zoneMid(rows []string, ch byte) [2]int {
	var cells [][2]int
	var sx, sz float64
	for z, row := range rows {
		for x := 0; x < gridW; x++ {
			if row[x] == ch {
				cells = append(cells, [2]int{x, z})
				sx, sz = sx+float64(x), sz+float64(z)
			}
		}
	}
	if len(cells) == 0 {
		return [2]int{gridW / 2, gridH / 2}
	}
	mx, mz := sx/float64(len(cells)), sz/float64(len(cells))
	sort.SliceStable(cells, func(i, j int) bool {
		return math.Hypot(float64(cells[i][0])-mx, float64(cells[i][1])-mz) < math.Hypot(float64(cells[j][0])-mx, float64(cells[j][1])-mz)
	})
	return cells[0]
}

// pathTo: bfs masofasi bo'yicha (tx,tz) dan boshlang'ich nuqtaga qaytish yo'li (boshidan oxirigacha).
func pathTo(rows []string, dist []int, tx, tz int) [][2]int {
	if dist[tz*gridW+tx] < 0 {
		return nil
	}
	p := [][2]int{{tx, tz}}
	x, z := tx, tz
	for dist[z*gridW+x] > 0 {
		for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, nz := x+o[0], z+o[1]
			if nx >= 0 && nz >= 0 && nx < gridW && nz < gridH && dist[nz*gridW+nx] == dist[z*gridW+x]-1 {
				x, z = nx, nz
				break
			}
		}
		p = append(p, [2]int{x, z})
	}
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	return p
}

type autoPlanOut struct {
	holds    map[string][][2][2]int
	spots    map[string][][2]int
	looks    map[string][][2]int
	stages   map[string][][2]int
	holdSite []string
}

// autoPlan: himoyachilar turadigan joylar, hujumchilar bomba qo'yadigan joylar va qaraydigan
// nuqtalar — qo'lda chizilgan xaritalardagi kabi, panjaradan hisoblanadi (brauzerda ham xuddi shunday).
func autoPlan(rows []string) autoPlanOut {
	out := autoPlanOut{holds: map[string][][2][2]int{}, spots: map[string][][2]int{}, looks: map[string][][2]int{}, stages: map[string][][2]int{}}
	tm := zoneMid(rows, 't')
	fromT := bfsRows(rows, tm[0], tm[1])
	for _, s := range []byte{'A', 'B'} {
		key := string(s)
		var cells, ent [][2]int
		for z, row := range rows {
			for x := 0; x < gridW; x++ {
				if row[x] != s {
					continue
				}
				cells = append(cells, [2]int{x, z})
				for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					if c := rows[z+o[1]][x+o[0]]; walkCell(c) && c != s {
						ent = append(ent, [2]int{x, z})
						break
					}
				}
			}
		}
		mid := zoneMid(rows, s)
		sort.SliceStable(cells, func(i, j int) bool { return cdist(cells[i], mid) < cdist(cells[j], mid) })
		n := len(cells)
		for _, k := range []int{0, n / 4, n / 2, 3 * n / 4} {
			if !containsCell(out.spots[key], cells[k]) {
				out.spots[key] = append(out.spots[key], cells[k])
			}
		}
		// kirish joylari: hujumchilarga eng yaqinidan boshlab, bir-biridan uzoq 3 tasi
		sort.SliceStable(ent, func(i, j int) bool { return fromT[ent[i][1]*gridW+ent[i][0]] < fromT[ent[j][1]*gridW+ent[j][0]] })
		var looks [][2]int
		for _, e := range ent {
			if fromT[e[1]*gridW+e[0]] < 0 {
				continue
			}
			far := true
			for _, l := range looks {
				if cdist(l, e) < 4 {
					far = false
				}
			}
			if far {
				looks = append(looks, e)
			}
			if len(looks) == 3 {
				break
			}
		}
		if len(looks) == 0 {
			looks = append(looks, mid)
		}
		out.looks[key] = looks
		// himoyachi: har kirishga qarab, undan eng uzoq katakda
		for _, l := range looks {
			best := cells[0]
			for _, c := range cells {
				if cdist(c, l) > cdist(best, l) && !holdTaken(out.holds[key], c) {
					best = c
				}
			}
			out.holds[key] = append(out.holds[key], [2][2]int{best, l})
		}
		if len(out.holds[key]) < 2 {
			out.holds[key] = append(out.holds[key], [2][2]int{mid, looks[0]})
		}
		// hujumdan oldin to'planish joyi: yo'lning 70% ida, nuqtaga kirmasdan
		if p := pathTo(rows, fromT, mid[0], mid[1]); len(p) > 4 {
			st := p[len(p)*7/10]
			for k := len(p) * 7 / 10; k > 0 && rows[st[1]][st[0]] == s; k-- {
				st = p[k]
			}
			out.stages[key] = [][2]int{st, p[len(p)/2]}
		} else {
			out.stages[key] = [][2]int{tm}
		}
	}
	// o'rta: ikki baza orasidagi eng qisqa yo'l
	cm := zoneMid(rows, 'k')
	if p := pathTo(rows, fromT, cm[0], cm[1]); len(p) > 6 {
		n := len(p)
		out.holds["M"] = [][2][2]int{{p[n*65/100], p[n*40/100]}, {p[n*55/100], p[n*30/100]}}
	} else {
		out.holds["M"] = [][2][2]int{{cm, tm}}
	}
	for _, k := range []string{"A", "B", "M"} {
		for range out.holds[k] {
			out.holdSite = append(out.holdSite, k)
		}
	}
	return out
}

func cdist(a, b [2]int) float64 { return math.Hypot(float64(a[0]-b[0]), float64(a[1]-b[1])) }
func containsCell(l [][2]int, c [2]int) bool {
	for _, x := range l {
		if x == c {
			return true
		}
	}
	return false
}
func holdTaken(l [][2][2]int, c [2]int) bool {
	for _, h := range l {
		if h[0] == c {
			return true
		}
	}
	return false
}

// newCustomGameMap: bazadagi xaritadan server xaritasi.
func newCustomGameMap(id, name string, rows []string) *gameMap {
	p := autoPlan(rows)
	d := mapDef{id: id, name: name, rows: rows, spots: p.spots, looks: p.looks}
	for _, k := range []string{"A", "B", "M"} {
		d.holds = append(d.holds, p.holds[k]...)
	}
	d.holdSite = p.holdSite
	return newGameMap(d)
}

// loadCustomMap: keshda bo'lmasa bazadan o'qiydi. Yo'q bo'lsa nil.
func loadCustomMap(ctx context.Context, st *Store, id string) *gameMap {
	if !isCustomMapID(id) {
		return nil
	}
	if gm := customMaps.get(id); gm != nil {
		return gm
	}
	if !st.Ready() {
		return nil
	}
	n, _ := strconv.ParseInt(id[1:], 10, 64)
	var name, raw string
	if err := st.db.QueryRowContext(ctx, `SELECT name, rows FROM custom_maps WHERE id = $1`, n).Scan(&name, &raw); err != nil {
		return nil
	}
	rows, err := validateCustomRows(raw)
	if err != nil {
		return nil
	}
	gm := newCustomGameMap(id, name, rows)
	customMaps.put(gm)
	return gm
}

// roomOptsFor: xona sozlamalari; foydalanuvchi xaritasi bo'lsa — avval bazadan keshga.
func roomOptsFor(r *http.Request, st *Store) roomOpts {
	if m := r.URL.Query().Get("map"); isCustomMapID(m) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		loadCustomMap(ctx, st, m)
		cancel()
	}
	return parseRoomOpts(r.URL.Query())
}

// ---- baza ----

type customMapRow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Author  string `json:"author"`
	Style   string `json:"style"`
	Plays   int    `json:"plays"`
	Created int64  `json:"created"`
	Rows    string `json:"rows,omitempty"`
}

func cleanMapName(s string) (string, error) {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" -_'ʻʼ.!", r) {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())
	if n := len([]rune(s)); n < 3 || n > 32 {
		return "", errMapName
	}
	return s, nil
}

// badMap: foydalanuvchiga ko'rsatiladigan xato (400).
type badMap struct{ error }

func (s *Store) SaveCustomMap(ctx context.Context, uid, id int64, name, style, raw string) (int64, error) {
	name, err := cleanMapName(name)
	if err != nil {
		return 0, badMap{err}
	}
	if !mapStyles[style] {
		return 0, badMap{errMapStyle}
	}
	rows, err := validateCustomRows(raw)
	if err != nil {
		return 0, badMap{err}
	}
	joined := strings.Join(rows, "")
	if id > 0 {
		res, err := s.db.ExecContext(ctx, `UPDATE custom_maps SET name = $3, style = $4, rows = $5, updated_at = now() WHERE id = $1 AND user_id = $2`, id, uid, name, style, joined)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return 0, errMapOwner
		}
		customMaps.drop("k" + strconv.FormatInt(id, 10)) // ochiq xonalar eski nusxada qoladi
		return id, nil
	}
	var cnt int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM custom_maps WHERE user_id = $1`, uid).Scan(&cnt)
	if cnt >= maxUserMaps {
		return 0, badMap{errMapLimit}
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO custom_maps (user_id, name, style, rows) VALUES ($1, $2, $3, $4) RETURNING id`, uid, name, style, joined).Scan(&id)
	return id, err
}

func (s *Store) DeleteCustomMap(ctx context.Context, uid, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM custom_maps WHERE id = $1 AND user_id = $2`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errMapOwner
	}
	customMaps.drop("k" + strconv.FormatInt(id, 10))
	return nil
}

func (s *Store) CustomMaps(ctx context.Context, sortBy, author string, limit int) ([]customMapRow, error) {
	q := `SELECT m.id, m.name, u.username, m.style, m.plays, extract(epoch FROM m.created_at)::bigint, m.rows FROM custom_maps m JOIN users u ON u.id = m.user_id`
	args := []any{}
	if author != "" {
		q += ` WHERE lower(u.username) = lower($1)`
		args = append(args, author)
	}
	if sortBy == "top" {
		q += ` ORDER BY m.plays DESC, m.id DESC`
	} else {
		q += ` ORDER BY m.id DESC`
	}
	q += fmt.Sprintf(` LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []customMapRow{}
	for rows.Next() {
		var m customMapRow
		if err := rows.Scan(&m.ID, &m.Name, &m.Author, &m.Style, &m.Plays, &m.Created, &m.Rows); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) CustomMap(ctx context.Context, id int64) (customMapRow, error) {
	var m customMapRow
	err := s.db.QueryRowContext(ctx, `SELECT m.id, m.name, u.username, m.style, m.plays, extract(epoch FROM m.created_at)::bigint, m.rows
		FROM custom_maps m JOIN users u ON u.id = m.user_id WHERE m.id = $1`, id).Scan(&m.ID, &m.Name, &m.Author, &m.Style, &m.Plays, &m.Created, &m.Rows)
	if errors.Is(err, sql.ErrNoRows) {
		return m, errMapNone
	}
	return m, err
}

func (s *Store) customMapPlayed(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.db.ExecContext(ctx, `UPDATE custom_maps SET plays = plays + 1 WHERE id = $1`, id)
}

// ---- API ----

// o'ynalganlar soni: bitta IP bitta xaritani 30 daqiqada bir marta qo'shadi
var mapPlays = struct {
	sync.Mutex
	seen map[string]time.Time
}{seen: map[string]time.Time{}}

func notePlay(st *Store, ip string, id int64) {
	k := ip + "|" + strconv.FormatInt(id, 10)
	now := time.Now()
	mapPlays.Lock()
	if t, ok := mapPlays.seen[k]; ok && now.Sub(t) < 30*time.Minute {
		mapPlays.Unlock()
		return
	}
	mapPlays.seen[k] = now
	if len(mapPlays.seen) > 20000 {
		for kk, t := range mapPlays.seen {
			if now.Sub(t) > 30*time.Minute {
				delete(mapPlays.seen, kk)
			}
		}
	}
	mapPlays.Unlock()
	if st.Ready() {
		go st.customMapPlayed(id)
	}
}

func (a *api) mapRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/maps", h(a.mapsList))
	mux.HandleFunc("GET /api/maps/{id}", h(a.mapGet))
	mux.HandleFunc("POST /api/maps", h(a.mapSave))
	mux.HandleFunc("POST /api/maps/{id}/play", h(a.mapPlay))
	mux.HandleFunc("DELETE /api/maps/{id}", h(a.mapDelete))
}

func (a *api) mapsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := a.st.CustomMaps(r.Context(), q.Get("sort"), strings.TrimSpace(q.Get("user")), 60)
	if err != nil {
		serverErr(w, "xaritalar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"maps": list, "limit": maxUserMaps})
}

func (a *api) mapGet(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	m, err := a.st.CustomMap(r.Context(), id)
	if err != nil {
		if errors.Is(err, errMapNone) {
			writeErr(w, http.StatusNotFound, err.Error())
		} else {
			serverErr(w, "xarita", err)
		}
		return
	}
	rows, err := validateCustomRows(m.Rows)
	if err != nil {
		writeErr(w, http.StatusNotFound, errMapNone.Error())
		return
	}
	p := autoPlan(rows)
	plan := map[string]any{}
	for _, k := range []string{"A", "B"} {
		plan[k] = map[string]any{"stages": p.stages[k], "spots": p.spots[k], "looks": p.looks[k]}
	}
	writeJSON(w, http.StatusOK, map[string]any{"map": m, "holds": p.holds, "plan": plan})
}

func (a *api) mapSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Style string `json:"style"`
		Rows  string `json:"rows"`
	}
	if !a.readJSONMax(w, r, &req, 8<<10) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Xaritani saqlash uchun hisobga kiring")
		return
	}
	id, err := a.st.SaveCustomMap(r.Context(), u.ID, req.ID, req.Name, req.Style, req.Rows)
	var bad badMap
	switch {
	case errors.As(err, &bad):
		writeErr(w, http.StatusBadRequest, bad.Error())
	case errors.Is(err, errMapOwner):
		writeErr(w, http.StatusForbidden, err.Error())
	case err != nil:
		serverErr(w, "xarita saqlash", err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	}
}

func (a *api) mapPlay(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id > 0 {
		notePlay(a.st, clientIP(r), id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *api) mapDelete(w http.ResponseWriter, r *http.Request) {
	if !originOK(r, a.allowed) {
		writeErr(w, http.StatusForbidden, "So'rov boshqa saytdan keldi")
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kiring")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := a.st.DeleteCustomMap(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, errMapOwner) {
			writeErr(w, http.StatusForbidden, err.Error())
		} else {
			serverErr(w, "xarita o'chirish", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
