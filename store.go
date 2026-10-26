package main

// Hisoblar, sessiyalar va o'yinlar tarixi — PostgreSQL'da.
// Sxema migrations/*.sql fayllarida; server ishga tushganda yangi migratsiyalar avtomatik qo'llanadi.

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

const (
	pbkdfIter   = 600_000
	sessionTTL  = 30 * 24 * time.Hour
	historyKeep = 30 // profilda ko'rsatiladigan so'nggi o'yinlar
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

var (
	errBadUsername = errors.New("Login 3–16 belgidan iborat bo'lsin: lotin harflari, raqamlar va _")
	errBadPassword = errors.New("Parol 6 belgidan 72 belgigacha bo'lsin")
	errTaken       = errors.New("Bu login band, boshqasini tanlang")
	errBadLogin    = errors.New("Login yoki parol noto'g'ri")
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type User struct {
	ID       int64
	Username string
	Salt     []byte
	Hash     []byte
	Iter     int
	Rating   int
	Matches  int
	Wins     int
	Kills    int
	Deaths   int
	HS       int
	XP       int64
	Created  int64 // unix
}

// histRow — o'yinchining bitta o'yindagi natijasi (profil uchun).
type histRow struct {
	MatchID int64
	Ended   int64
	Room    string
	ScoreT  int
	ScoreCT int
	Team    string
	Won     bool
	Kills   int
	Deaths  int
	HS      int
	Before  int
	After   int
}

type ratingChange struct {
	Before, After, Delta int
	Won                  bool
	Matches              int
}

type Store struct {
	db    *sql.DB
	ready atomic.Bool // migratsiyalar qo'llangan va baza javob beryapti
	dummy User        // mavjud bo'lmagan login uchun ham parol tekshiruvi bir xil vaqt olsin
}

// openStore bazaga ulanadi. Baza hali tayyor bo'lmasa (masalan, konteyner endi ishga tushmoqda),
// fonda qayta urinadi; shu vaqtda Ready() false qaytaradi va o'yin mehmon rejimida ishlaydi.
func openStore(dsn, legacyDir string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	s := &Store{db: db}
	s.dummy = User{Salt: randBytes(16), Iter: pbkdfIter}
	s.dummy.Hash = hashPassword("x", s.dummy.Salt, s.dummy.Iter)
	go func() {
		for i := 0; ; i++ {
			if err := s.init(legacyDir); err != nil {
				if i%6 == 0 {
					log.Printf("baza hali tayyor emas: %v", err)
				}
				time.Sleep(5 * time.Second)
				continue
			}
			s.ready.Store(true)
			log.Printf("baza tayyor")
			s.maintenance()
			return
		}
	}()
	return s, nil
}

func (s *Store) Ready() bool { return s != nil && s.ready.Load() }

func (s *Store) init(legacyDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	if err := s.migrate(ctx); err != nil {
		return fmt.Errorf("migratsiya: %w", err)
	}
	if legacyDir != "" {
		if n, err := s.importLegacy(ctx, legacyDir); err != nil {
			return fmt.Errorf("eski ma'lumotlarni import qilish: %w", err)
		} else if n > 0 {
			log.Printf("eski fayl bazasidan %d ta hisob import qilindi", n)
		}
	}
	return nil
}

// migrate migrations/NNN_*.sql fayllarini tartib bilan, har birini o'z tranzaksiyasida qo'llaydi.
func (s *Store) migrate(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// bir nechta server nusxasi bir vaqtda migratsiya qilmasin
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(727274)`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(727274)`)
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		v, err := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("%s: fayl nomi NNN_ bilan boshlanishi kerak", base)
		}
		var done bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, v).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		body, _ := migrationFS.ReadFile(name)
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", base, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, v); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("migratsiya qo'llandi: %s", base)
	}
	return nil
}

// maintenance eskirgan sessiyalarni soatiga bir marta o'chiradi.
func (s *Store) maintenance() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
			log.Printf("sessiyalarni tozalashda xato: %v", err)
		}
		cancel()
		time.Sleep(time.Hour)
	}
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func hashPassword(pass string, salt []byte, iter int) []byte {
	h, err := pbkdf2.Key(sha256.New, pass, salt, iter, 32)
	if err != nil {
		panic(err)
	}
	return h
}

func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }

func validPassword(p string) bool { return len(p) >= 6 && len(p) <= 72 }

const userCols = `id, username, pass_salt, pass_hash, pass_iter, rating, matches, wins, kills, deaths, hs, xp, extract(epoch FROM created_at)::bigint`

type scanner interface{ Scan(...any) error }

func scanUser(r scanner) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Username, &u.Salt, &u.Hash, &u.Iter, &u.Rating, &u.Matches, &u.Wins, &u.Kills, &u.Deaths, &u.HS, &u.XP, &u.Created)
	return u, err
}

func newSession(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, uid int64) (string, error) {
	token := base64.RawURLEncoding.EncodeToString(randBytes(32))
	_, err := q.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash(token), uid, time.Now().Add(sessionTTL))
	return token, err
}

// Register yangi hisob ochadi va unga sessiya beradi.
func (s *Store) Register(ctx context.Context, username, password string) (User, string, error) {
	username = strings.TrimSpace(username)
	if !usernameRe.MatchString(username) {
		return User{}, "", errBadUsername
	}
	if !validPassword(password) {
		return User{}, "", errBadPassword
	}
	salt := randBytes(16)
	hash := hashPassword(password, salt, pbkdfIter)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	u, err := scanUser(tx.QueryRowContext(ctx, `INSERT INTO users (username, pass_salt, pass_hash, pass_iter, rating)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT ((lower(username))) DO NOTHING RETURNING `+userCols,
		username, salt, hash, pbkdfIter, startRating))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", errTaken
	} else if err != nil {
		return User{}, "", err
	}
	token, err := newSession(ctx, tx, u.ID)
	if err != nil {
		return User{}, "", err
	}
	return u, token, tx.Commit()
}

func (s *Store) Login(ctx context.Context, username, password string) (User, string, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE lower(username) = lower($1)`, strings.TrimSpace(username)))
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, "", err
	}
	ref := s.dummy
	if found {
		ref = u
	}
	ok := subtle.ConstantTimeCompare(hashPassword(password, ref.Salt, ref.Iter), ref.Hash) == 1
	if !found || !ok {
		return User{}, "", errBadLogin
	}
	token, err := newSession(ctx, s.db, u.ID)
	return u, token, err
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash(token))
	return err
}

// Session token bo'yicha foydalanuvchini qaytaradi.
func (s *Store) Session(ctx context.Context, token string) (User, bool) {
	if token == "" || len(token) > 100 {
		return User{}, false
	}
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+prefixed("u", userCols)+` FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash(token)))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("sessiya tekshirilmadi: %v", err)
		}
		return User{}, false
	}
	return u, true
}

func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		if strings.HasPrefix(p, "extract(epoch FROM ") {
			parts[i] = strings.Replace(p, "FROM ", "FROM "+alias+".", 1)
		} else {
			parts[i] = alias + "." + p
		}
	}
	return strings.Join(parts, ", ")
}

func (s *Store) User(ctx context.Context, id int64) (User, bool) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("foydalanuvchi o'qilmadi: %v", err)
		}
		return User{}, false
	}
	return u, true
}

// position: reyting jadvalidagi o'rin (0 — hali o'yin o'ynamagan).
func (s *Store) position(ctx context.Context, id int64) (int, error) {
	var pos int
	err := s.db.QueryRowContext(ctx, `SELECT pos FROM (
		SELECT id, row_number() OVER (ORDER BY rating DESC, wins DESC, id) AS pos FROM users WHERE matches > 0) r WHERE id = $1`, id).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return pos, err
}

// Leaderboard: eng yaxshi limit ta o'yinchi, jami soni va berilgan foydalanuvchining o'rni.
func (s *Store) Leaderboard(ctx context.Context, limit int, me int64) (top []User, total int, myPos int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users WHERE matches > 0 ORDER BY rating DESC, wins DESC, id LIMIT $1`, limit)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, 0, err
		}
		top = append(top, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE matches > 0`).Scan(&total); err != nil {
		return nil, 0, 0, err
	}
	if me != 0 {
		if myPos, err = s.position(ctx, me); err != nil {
			return nil, 0, 0, err
		}
	}
	return top, total, myPos, nil
}

// Profile: o'yinchi, uning o'rni va so'nggi o'yinlari.
func (s *Store) Profile(ctx context.Context, name string) (User, int, []histRow, bool, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE lower(username) = lower($1)`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, 0, nil, false, nil
	} else if err != nil {
		return User{}, 0, nil, false, err
	}
	pos, err := s.position(ctx, u.ID)
	if err != nil {
		return User{}, 0, nil, false, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.id, extract(epoch FROM m.ended_at)::bigint, m.room, m.score_t, m.score_ct,
			p.team, p.won, p.kills, p.deaths, p.hs, coalesce(p.rating_before, 0), coalesce(p.rating_after, 0)
		FROM match_players p JOIN matches m ON m.id = p.match_id
		WHERE p.user_id = $1 ORDER BY m.id DESC LIMIT $2`, u.ID, historyKeep)
	if err != nil {
		return User{}, 0, nil, false, err
	}
	defer rows.Close()
	hist := []histRow{}
	for rows.Next() {
		var h histRow
		if err := rows.Scan(&h.MatchID, &h.Ended, &h.Room, &h.ScoreT, &h.ScoreCT, &h.Team, &h.Won, &h.Kills, &h.Deaths, &h.HS, &h.Before, &h.After); err != nil {
			return User{}, 0, nil, false, err
		}
		hist = append(hist, h)
	}
	return u, pos, hist, true, rows.Err()
}

// ApplyMatch reytinglarni yangilaydi va o'yinni tarixga yozadi — hammasi bitta tranzaksiyada.
func (s *Store) ApplyMatch(ctx context.Context, res *matchResult) (map[int64]ratingChange, error) {
	var ids []int64
	for _, p := range res.Players {
		if p.UserID != 0 {
			ids = append(ids, p.UserID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	type cur struct {
		rating, matches int
		name            string
	}
	users := map[int64]cur{}
	rows, err := tx.QueryContext(ctx, `SELECT id, username, rating, matches FROM users WHERE id = ANY($1) ORDER BY id FOR UPDATE`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var c cur
		if err := rows.Scan(&id, &c.name, &c.rating, &c.matches); err != nil {
			rows.Close()
			return nil, err
		}
		users[id] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sum, cnt := map[string]float64{}, map[string]float64{}
	for _, p := range res.Players {
		r := float64(startRating)
		if c, ok := users[p.UserID]; ok {
			r = float64(c.rating)
		}
		sum[p.Team] += r
		cnt[p.Team]++
	}
	if cnt["T"] == 0 || cnt["CT"] == 0 {
		return nil, nil
	}
	avg := map[string]float64{"T": sum["T"] / cnt["T"], "CT": sum["CT"] / cnt["CT"]}
	opp := map[string]string{"T": "CT", "CT": "T"}

	var matchID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO matches (room, started_at, ended_at, score_t, score_ct, winner) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		res.Room, res.Started, res.Ended, res.ScoreT, res.ScoreCT, res.Winner).Scan(&matchID); err != nil {
		return nil, err
	}
	changes := map[int64]ratingChange{}
	for _, p := range res.Players {
		won := p.Team == res.Winner
		var uid, before, after any // NULL — mehmon
		name := p.Name
		if c, ok := users[p.UserID]; ok {
			d := eloDelta(avg[p.Team], avg[opp[p.Team]], won, c.matches, res.ScoreT-res.ScoreCT)
			newRating := max(0, c.rating+d)
			w := 0
			if won {
				w = 1
			}
			if _, err := tx.ExecContext(ctx, `UPDATE users SET rating = $2, matches = matches + 1, wins = wins + $3,
				kills = kills + $4, deaths = deaths + $5, hs = hs + $6 WHERE id = $1`, p.UserID, newRating, w, p.Kills, p.Deaths, p.HS); err != nil {
				return nil, err
			}
			changes[p.UserID] = ratingChange{Before: c.rating, After: newRating, Delta: newRating - c.rating, Won: won, Matches: c.matches + 1}
			uid, before, after, name = p.UserID, c.rating, newRating, c.name
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO match_players (match_id, user_id, name, team, kills, deaths, hs, won, rating_before, rating_after)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, matchID, uid, name, p.Team, p.Kills, p.Deaths, p.HS, won, before, after); err != nil {
			return nil, err
		}
	}
	return changes, tx.Commit()
}
