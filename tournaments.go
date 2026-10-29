package main

// Klanlar turniri: bitta yutqazishda chiqib ketish, 4 yoki 8 jamoa.
//
// Istalgan o'yinchi turnir ochadi (nomi, hajmi, xaritasi) — u tashkilotchi. Klan egalari o'z
// klanini yozdiradi. Tashkilotchi boshlaganda klanlar o'rtacha reyting bo'yicha saralanadi
// (1-o'rin eng kuchli) va odatdagi setka tuziladi (1–8, 4–5, 2–7, 3–6); jamoa yetmasa, kuchlilar
// keyingi bosqichga o'z-o'zidan o'tadi. Har o'yinning o'z xonasi bor: t<turnir>-<bosqich>-<o'rin>
// (Raqobat, 9 g'alabagacha, botsiz). Unga faqat shu ikki klan a'zolari kiradi — yuqoridagi klan A
// jamoasida, pastdagisi B da — boshqalar faqat kuzata oladi. O'yin tugagach (server tasdiqlagan
// natija) g'olib keyingi bosqichga o'tadi; final g'olibi — turnir g'olibi. Jamoa kelmasa yoki
// o'yin buzilsa, tashkilotchi g'olibni qo'lda belgilaydi (texnik g'alaba).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	errTrnName     = errors.New("Turnir nomi 3–40 belgidan iborat bo'lsin")
	errTrnSize     = errors.New("Turnir 4 yoki 8 jamoalik bo'ladi")
	errTrnMap      = errors.New("Bu xarita turnir uchun emas")
	errTrnNotFound = errors.New("Turnir topilmadi")
	errTrnNotOwner = errors.New("Buni faqat turnir tashkilotchisi qila oladi")
	errTrnState    = errors.New("Turnir bu bosqichda emas")
	errTrnClan     = errors.New("Klanni faqat uning egasi yozdira oladi")
	errTrnFull     = errors.New("Turnirda joy qolmagan")
	errTrnFew      = errors.New("Boshlash uchun kamida 2 ta klan kerak")
	errTrnMatch    = errors.New("Bu o'yinning natijasini belgilab bo'lmaydi")
	errTrnBusy     = errors.New("Sizda tugamagan turnir bor")
)

const trnFm = 8 // turnir o'yinlari: 9 g'alabagacha

// ---- setka ----

type tMatch struct {
	R, S   int
	A, B   int64 // klan (0 — hali yo'q)
	EA, EB bool  // bu tomonga hech kim kelmaydi
	W      int64
	SA, SB sql.NullInt64
}

func trnRounds(size int) int {
	n := 0
	for s := size; s > 1; s /= 2 {
		n++
	}
	return n
}

// seedOrder: 1-bosqichdagi o'rinlar bo'yicha urug'lar (1–8, 4–5, 2–7, 3–6).
func seedOrder(size int) []int {
	o := []int{1, 2}
	for len(o) < size {
		n := len(o)*2 + 1
		next := make([]int, 0, len(o)*2)
		for _, s := range o {
			next = append(next, s, n-s)
		}
		o = next
	}
	return o
}

func key(r, s int) [2]int { return [2]int{r, s} }

// newBracket: urug'langan klanlar (1-o'rin birinchi) bo'yicha bo'sh setka.
func newBracket(size int, seeded []int64) map[[2]int]*tMatch {
	ms := map[[2]int]*tMatch{}
	rounds := trnRounds(size)
	for r := 1; r <= rounds; r++ {
		for s := 0; s < size>>r; s++ {
			ms[key(r, s)] = &tMatch{R: r, S: s}
		}
	}
	ord := seedOrder(size)
	for i := 0; i < size/2; i++ {
		m := ms[key(1, i)]
		if a := ord[2*i]; a <= len(seeded) {
			m.A = seeded[a-1]
		} else {
			m.EA = true
		}
		if b := ord[2*i+1]; b <= len(seeded) {
			m.B = seeded[b-1]
		} else {
			m.EB = true
		}
	}
	return ms
}

// settle: bo'sh o'rinlar tufayli o'z-o'zidan hal bo'ladigan o'yinlar va keyingi bosqichga o'tish. Chempion (0 — hali yo'q).
func settle(ms map[[2]int]*tMatch, size int) int64 {
	rounds := trnRounds(size)
	for r := 1; r <= rounds; r++ {
		for s := 0; s < size>>r; s++ {
			m := ms[key(r, s)]
			if m.W == 0 {
				switch {
				case m.A != 0 && m.EB:
					m.W = m.A
				case m.B != 0 && m.EA:
					m.W = m.B
				}
			}
			if r == rounds {
				continue
			}
			n := ms[key(r+1, s/2)]
			out, empty := m.W, m.W == 0 && m.EA && m.EB
			if s%2 == 0 {
				n.A, n.EA = out, empty
			} else {
				n.B, n.EB = out, empty
			}
		}
	}
	return ms[key(rounds, 0)].W
}

// ---- baza ----

type trnRow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Size    int    `json:"size"`
	Map     string `json:"map"`
	State   string `json:"state"`
	Teams   int    `json:"teams"`
	Winner  string `json:"winner,omitempty"`
	ownerID int64
}

type trnTeam struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	Seed   int    `json:"seed,omitempty"`
	Rating int    `json:"rating"`
}

type trnSide struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type trnMatchOut struct {
	Round  int      `json:"round"`
	Slot   int      `json:"slot"`
	A      *trnSide `json:"a"`
	B      *trnSide `json:"b"`
	EA     bool     `json:"ea,omitempty"`
	EB     bool     `json:"eb,omitempty"`
	Winner int64    `json:"winner,omitempty"`
	SA     *int64   `json:"sA,omitempty"`
	SB     *int64   `json:"sB,omitempty"`
	Room   string   `json:"room,omitempty"` // o'ynaladigan xona kodi (ikkala jamoa ma'lum, natija yo'q)
}

func trnRoomCode(tid int64, r, s int) string { return fmt.Sprintf("t%d-%d-%d", tid, r, s) }

var trnCodeRe = regexp.MustCompile(`^zn-t(\d+)-(\d)-(\d)$`)

func parseTrnRoom(name string) (tid int64, r, s int, ok bool) {
	m := trnCodeRe.FindStringSubmatch(name)
	if m == nil {
		return 0, 0, 0, false
	}
	tid, _ = strconv.ParseInt(m[1], 10, 64)
	r, _ = strconv.Atoi(m[2])
	s, _ = strconv.Atoi(m[3])
	return tid, r, s, true
}

func (s *Store) trnGet(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64, lock bool) (trnRow, error) {
	var t trnRow
	var win sql.NullString
	sfx := ""
	if lock {
		sfx = " FOR UPDATE OF t"
	}
	err := q.QueryRowContext(ctx, `SELECT t.id, t.name, u.username, t.owner_id, t.size, t.map, t.state, c.name,
			(SELECT count(*) FROM tournament_teams x WHERE x.tournament_id = t.id)
		FROM tournaments t JOIN users u ON u.id = t.owner_id LEFT JOIN clans c ON c.id = t.winner_clan WHERE t.id = $1`+sfx, id).
		Scan(&t.ID, &t.Name, &t.Owner, &t.ownerID, &t.Size, &t.Map, &t.State, &win, &t.Teams)
	if errors.Is(err, sql.ErrNoRows) {
		return t, errTrnNotFound
	}
	t.Winner = win.String
	return t, err
}

// Tournaments: so'nggi turnirlar.
func (s *Store) Tournaments(ctx context.Context, limit int) ([]trnRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.name, u.username, t.size, t.map, t.state, coalesce(c.name, ''),
			(SELECT count(*) FROM tournament_teams x WHERE x.tournament_id = t.id)
		FROM tournaments t JOIN users u ON u.id = t.owner_id LEFT JOIN clans c ON c.id = t.winner_clan
		ORDER BY (t.state = 'done'), t.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []trnRow{}
	for rows.Next() {
		var t trnRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Owner, &t.Size, &t.Map, &t.State, &t.Winner, &t.Teams); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TrnCreate(ctx context.Context, owner int64, name string, size int, gmap string) (int64, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 3 || n > 40 {
		return 0, errTrnName
	}
	if size != 4 && size != 8 {
		return 0, errTrnSize
	}
	if !validMap(gmap) || isAimMap(gmap) || isCustomMapID(gmap) {
		return 0, errTrnMap
	}
	var busy int
	s.db.QueryRowContext(ctx, `SELECT count(*) FROM tournaments WHERE owner_id = $1 AND state <> 'done'`, owner).Scan(&busy)
	if busy > 0 {
		return 0, errTrnBusy
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO tournaments (name, owner_id, size, map) VALUES ($1, $2, $3, $4) RETURNING id`, name, owner, size, gmap).Scan(&id)
	return id, err
}

// TrnJoin / TrnLeave: klan egasi o'z klanini yozdiradi yoki chiqaradi (ochiq turnirda).
func (s *Store) TrnJoin(ctx context.Context, uid, tid int64, join bool) error {
	ci, err := s.clanOf(ctx, uid)
	if err != nil {
		return err
	}
	if ci == nil || !ci.Owner {
		return errTrnClan
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := s.trnGet(ctx, tx, tid, true)
	if err != nil {
		return err
	}
	if t.State != "open" {
		return errTrnState
	}
	if join {
		if t.Teams >= t.Size {
			return errTrnFull
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tournament_teams (tournament_id, clan_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, tid, ci.ID); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM tournament_teams WHERE tournament_id = $1 AND clan_id = $2`, tid, ci.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) TrnDelete(ctx context.Context, uid, tid int64) error {
	t, err := s.trnGet(ctx, s.db, tid, false)
	if err != nil {
		return err
	}
	if t.ownerID != uid {
		return errTrnNotOwner
	}
	if t.State == "running" {
		return errTrnState
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM tournaments WHERE id = $1`, tid)
	return err
}

func (s *Store) loadMatches(ctx context.Context, tx *sql.Tx, tid int64) (map[[2]int]*tMatch, error) {
	rows, err := tx.QueryContext(ctx, `SELECT round, slot, coalesce(clan_a, 0), coalesce(clan_b, 0), empty_a, empty_b, coalesce(winner, 0), score_a, score_b
		FROM tournament_matches WHERE tournament_id = $1 FOR UPDATE`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ms := map[[2]int]*tMatch{}
	for rows.Next() {
		m := &tMatch{}
		if err := rows.Scan(&m.R, &m.S, &m.A, &m.B, &m.EA, &m.EB, &m.W, &m.SA, &m.SB); err != nil {
			return nil, err
		}
		ms[key(m.R, m.S)] = m
	}
	return ms, rows.Err()
}

func nz(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func (s *Store) saveMatches(ctx context.Context, tx *sql.Tx, tid int64, size int, ms map[[2]int]*tMatch) error {
	champ := settle(ms, size)
	for _, m := range ms {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tournament_matches (tournament_id, round, slot, clan_a, clan_b, empty_a, empty_b, winner, score_a, score_b)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT (tournament_id, round, slot) DO UPDATE SET clan_a = $4, clan_b = $5, empty_a = $6, empty_b = $7, winner = $8, score_a = $9, score_b = $10`,
			tid, m.R, m.S, nz(m.A), nz(m.B), m.EA, m.EB, nz(m.W), m.SA, m.SB); err != nil {
			return err
		}
	}
	if champ != 0 {
		_, err := tx.ExecContext(ctx, `UPDATE tournaments SET state = 'done', winner_clan = $2, ended_at = now() WHERE id = $1`, tid, champ)
		return err
	}
	return nil
}

// TrnStart: urug'lash (o'rtacha reyting) va setka.
func (s *Store) TrnStart(ctx context.Context, uid, tid int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := s.trnGet(ctx, tx, tid, true)
	if err != nil {
		return err
	}
	if t.ownerID != uid {
		return errTrnNotOwner
	}
	if t.State != "open" {
		return errTrnState
	}
	if t.Teams < 2 {
		return errTrnFew
	}
	rows, err := tx.QueryContext(ctx, `SELECT x.clan_id FROM tournament_teams x JOIN clan_members m ON m.clan_id = x.clan_id JOIN users u ON u.id = m.user_id
		WHERE x.tournament_id = $1 GROUP BY x.clan_id ORDER BY avg(u.rating) DESC, x.clan_id`, tid)
	if err != nil {
		return err
	}
	var seeded []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		seeded = append(seeded, id)
	}
	rows.Close()
	if len(seeded) < 2 {
		return errTrnFew
	}
	for i, id := range seeded {
		if _, err := tx.ExecContext(ctx, `UPDATE tournament_teams SET seed = $3 WHERE tournament_id = $1 AND clan_id = $2`, tid, id, i+1); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tournaments SET state = 'running', started_at = now() WHERE id = $1`, tid); err != nil {
		return err
	}
	if err := s.saveMatches(ctx, tx, tid, t.Size, newBracket(t.Size, seeded)); err != nil {
		return err
	}
	return tx.Commit()
}

// TrnResult: o'yin natijasi — o'yindan (byOwner=0) yoki tashkilotchidan (texnik g'alaba).
func (s *Store) TrnResult(ctx context.Context, byOwner, tid int64, round, slot int, winner int64, sa, sb int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := s.trnGet(ctx, tx, tid, true)
	if err != nil {
		return err
	}
	if byOwner != 0 && t.ownerID != byOwner {
		return errTrnNotOwner
	}
	if t.State != "running" {
		return errTrnState
	}
	ms, err := s.loadMatches(ctx, tx, tid)
	if err != nil {
		return err
	}
	m := ms[key(round, slot)]
	if m == nil || m.A == 0 || m.B == 0 || m.W != 0 || (winner != m.A && winner != m.B) {
		return errTrnMatch
	}
	m.W = winner
	if sa >= 0 && sb >= 0 {
		m.SA, m.SB = sql.NullInt64{Int64: int64(sa), Valid: true}, sql.NullInt64{Int64: int64(sb), Valid: true}
	}
	if err := s.saveMatches(ctx, tx, tid, t.Size, ms); err != nil {
		return err
	}
	return tx.Commit()
}

// trnMatchFor: o'yin xonasi uchun — ikki klan va xarita (o'ynasa bo'ladigan bo'lsa).
func (s *Store) trnMatchFor(ctx context.Context, tid int64, round, slot int) (a, b int64, gmap string, ok bool) {
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(m.clan_a, 0), coalesce(m.clan_b, 0), t.map FROM tournament_matches m JOIN tournaments t ON t.id = m.tournament_id
		WHERE m.tournament_id = $1 AND m.round = $2 AND m.slot = $3 AND m.winner IS NULL AND t.state = 'running'`, tid, round, slot).Scan(&a, &b, &gmap)
	return a, b, gmap, err == nil && a != 0 && b != 0
}

func (s *Store) clanIDOf(ctx context.Context, uid int64) int64 {
	var id int64
	s.db.QueryRowContext(ctx, `SELECT clan_id FROM clan_members WHERE user_id = $1`, uid).Scan(&id)
	return id
}

type trnDetail struct {
	trnRow
	Teams   []trnTeam     `json:"teamList"`
	Matches []trnMatchOut `json:"matches"`
	Rounds  int           `json:"rounds"`
	Me      struct {
		Owner     bool   `json:"owner"`
		Clan      int64  `json:"clan,omitempty"`
		ClanOwner bool   `json:"clanOwner,omitempty"`
		ClanName  string `json:"clanName,omitempty"`
	} `json:"me"`
}

func (s *Store) TrnDetail(ctx context.Context, tid, me int64) (trnDetail, error) {
	var d trnDetail
	t, err := s.trnGet(ctx, s.db, tid, false)
	if err != nil {
		return d, err
	}
	d.trnRow, d.Rounds = t, trnRounds(t.Size)
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.tag, coalesce(x.seed, 0), coalesce(round(avg(u.rating)), 0)::int
		FROM tournament_teams x JOIN clans c ON c.id = x.clan_id LEFT JOIN clan_members m ON m.clan_id = c.id LEFT JOIN users u ON u.id = m.user_id
		WHERE x.tournament_id = $1 GROUP BY c.id, x.seed ORDER BY coalesce(x.seed, 99), avg(u.rating) DESC, c.id`, tid)
	if err != nil {
		return d, err
	}
	sides := map[int64]*trnSide{}
	d.Teams = []trnTeam{}
	for rows.Next() {
		var tm trnTeam
		if err := rows.Scan(&tm.ID, &tm.Name, &tm.Tag, &tm.Seed, &tm.Rating); err != nil {
			rows.Close()
			return d, err
		}
		d.Teams = append(d.Teams, tm)
		sides[tm.ID] = &trnSide{tm.ID, tm.Name, tm.Tag}
	}
	rows.Close()
	mr, err := s.db.QueryContext(ctx, `SELECT round, slot, coalesce(clan_a, 0), coalesce(clan_b, 0), empty_a, empty_b, coalesce(winner, 0), score_a, score_b
		FROM tournament_matches WHERE tournament_id = $1 ORDER BY round, slot`, tid)
	if err != nil {
		return d, err
	}
	defer mr.Close()
	d.Matches = []trnMatchOut{}
	for mr.Next() {
		var m tMatch
		if err := mr.Scan(&m.R, &m.S, &m.A, &m.B, &m.EA, &m.EB, &m.W, &m.SA, &m.SB); err != nil {
			return d, err
		}
		o := trnMatchOut{Round: m.R, Slot: m.S, A: sides[m.A], B: sides[m.B], EA: m.EA, EB: m.EB, Winner: m.W}
		if m.SA.Valid {
			o.SA, o.SB = &m.SA.Int64, &m.SB.Int64
		}
		if m.A != 0 && m.B != 0 && m.W == 0 && t.State == "running" {
			o.Room = trnRoomCode(tid, m.R, m.S)
		}
		d.Matches = append(d.Matches, o)
	}
	d.Me.Owner = me != 0 && me == t.ownerID
	if me != 0 {
		if ci, _ := s.clanOf(ctx, me); ci != nil {
			d.Me.Clan, d.Me.ClanOwner, d.Me.ClanName = ci.ID, ci.Owner, ci.Name
		}
	}
	return d, mr.Err()
}

// ---- xonalar ----

// trnRoom: turnir o'yini xonasi — kim qaysi jamoada (klan bo'yicha).
type trnRoom struct {
	tid          int64
	round, slot  int
	clanA, clanB int64
	reported     bool
}

func (t *trnRoom) side(clan int64) string {
	switch clan {
	case t.clanA:
		return "A"
	case t.clanB:
		return "B"
	}
	return ""
}

// trnResult: o'yin tugadi — g'olib setkada keyingi bosqichga o'tadi.
func (h *hub) trnResult(t *trnRoom, res *matchResult) {
	if !h.store.Ready() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, sa, sb := t.clanB, res.ScoreT, res.ScoreCT // A — hujumdan boshlagan jamoa (match.go)
	if res.ScoreT > res.ScoreCT {
		w = t.clanA
	}
	if err := h.store.TrnResult(ctx, 0, t.tid, t.round, t.slot, w, sa, sb); err != nil {
		log.Printf("turnir #%d natijasi yozilmadi: %v", t.tid, err)
		return
	}
	log.Printf("turnir #%d: %d-bosqich %d-o'yin, g'olib klan #%d (%d:%d)", t.tid, t.round, t.slot, w, sa, sb)
}

// ---- API ----

func (a *api) trnRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/tournaments", h(a.trnList))
	mux.HandleFunc("GET /api/tournaments/{id}", h(a.trnGetAPI))
	mux.HandleFunc("POST /api/tournaments", h(a.trnPost))
}

func (a *api) trnList(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.Tournaments(r.Context(), 30)
	if err != nil {
		serverErr(w, "turnirlar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tournaments": list})
}

func (a *api) trnGetAPI(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var me int64
	if u, ok := a.currentUser(r); ok {
		me = u.ID
	}
	d, err := a.st.TrnDetail(r.Context(), id, me)
	if errors.Is(err, errTrnNotFound) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		serverErr(w, "turnir", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) trnPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Size   int    `json:"size"`
		Map    string `json:"map"`
		Round  int    `json:"round"`
		Slot   int    `json:"slot"`
		Winner int64  `json:"winner"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Turnir uchun hisobga kiring")
		return
	}
	ctx := r.Context()
	var err error
	id := req.ID
	switch req.Action {
	case "create":
		id, err = a.st.TrnCreate(ctx, u.ID, req.Name, req.Size, req.Map)
	case "join", "leave":
		err = a.st.TrnJoin(ctx, u.ID, req.ID, req.Action == "join")
	case "start":
		err = a.st.TrnStart(ctx, u.ID, req.ID)
	case "result":
		err = a.st.TrnResult(ctx, u.ID, req.ID, req.Round, req.Slot, req.Winner, -1, -1)
	case "delete":
		err = a.st.TrnDelete(ctx, u.ID, req.ID)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
			return
		}
	default:
		err = errors.New("Noma'lum amal")
	}
	switch {
	case errors.Is(err, errTrnNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, errTrnNotOwner), errors.Is(err, errTrnClan):
		writeErr(w, http.StatusForbidden, err.Error())
		return
	case err != nil && isUserErr(err):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		serverErr(w, "turnir", err)
		return
	}
	d, err := a.st.TrnDetail(ctx, id, u.ID)
	if err != nil {
		serverErr(w, "turnir", err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func isUserErr(err error) bool {
	for _, e := range []error{errTrnName, errTrnSize, errTrnMap, errTrnState, errTrnFull, errTrnFew, errTrnMatch, errTrnBusy} {
		if errors.Is(err, e) {
			return true
		}
	}
	return err.Error() == "Noma'lum amal"
}
