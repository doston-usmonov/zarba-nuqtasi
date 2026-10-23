package main

// Hisoblar, sessiyalar va o'yinlar tarixi. Tashqi bazasiz: holat xotirada,
// diskka ikki faylda yoziladi —
//   snapshot.json  foydalanuvchilar va sessiyalar (har o'zgarishda atomik qayta yoziladi)
//   matches.jsonl  tugagan o'yinlar (faqat oxiriga qo'shiladi)
// Har kuni backups/ ichiga nusxa olinadi (oxirgi 7 kun saqlanadi).
// PostgreSQL'ga o'tish kerak bo'lsa, faqat shu fayl almashtiriladi.

import (
	"bufio"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	pbkdfIter   = 600_000
	sessionTTL  = 30 * 24 * time.Hour
	historyKeep = 30 // har bir o'yinchi uchun eslab qolinadigan o'yinlar
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

var (
	errBadUsername = errors.New("Login 3–16 belgidan iborat bo'lsin: lotin harflari, raqamlar va _")
	errBadPassword = errors.New("Parol 6 belgidan 72 belgigacha bo'lsin")
	errTaken       = errors.New("Bu login band, boshqasini tanlang")
	errBadLogin    = errors.New("Login yoki parol noto'g'ri")
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Salt     []byte `json:"salt"`
	Hash     []byte `json:"hash"`
	Iter     int    `json:"iter"`
	Rating   int    `json:"rating"`
	Matches  int    `json:"matches"`
	Wins     int    `json:"wins"`
	Kills    int    `json:"kills"`
	Deaths   int    `json:"deaths"`
	HS       int    `json:"hs"`
	Created  int64  `json:"created"`
}

type sessionRec struct {
	UserID  int64 `json:"u"`
	Expires int64 `json:"exp"`
}

type MatchPlayer struct {
	UserID int64  `json:"uid"` // 0 — mehmon
	Name   string `json:"name"`
	Team   string `json:"team"`
	Kills  int    `json:"k"`
	Deaths int    `json:"d"`
	HS     int    `json:"hs"`
	Won    bool   `json:"won"`
	Before int    `json:"before"`
	After  int    `json:"after"`
}

type MatchRec struct {
	ID      int64         `json:"id"`
	Room    string        `json:"room"`
	Started int64         `json:"started"`
	Ended   int64         `json:"ended"`
	ScoreT  int           `json:"sT"`
	ScoreCT int           `json:"sCT"`
	Winner  string        `json:"winner"`
	Players []MatchPlayer `json:"players"`
}

type ratingChange struct {
	Before, After, Delta int
	Won                  bool
	Matches              int
}

type snapshot struct {
	Version   int                   `json:"version"`
	NextUser  int64                 `json:"next_user"`
	NextMatch int64                 `json:"next_match"`
	Users     []*User               `json:"users"`
	Sessions  map[string]sessionRec `json:"sessions"`
}

type Store struct {
	mu        sync.Mutex
	dir       string
	users     map[int64]*User
	byName    map[string]*User // kichik harflarda
	sessions  map[string]sessionRec
	history   map[int64][]*MatchRec // eng yangisi oxirida
	nextUser  int64
	nextMatch int64
	matchLog  *os.File
	dummy     User // mavjud bo'lmagan login uchun ham parol tekshiruvi bir xil vaqt olsin
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, users: map[int64]*User{}, byName: map[string]*User{}, sessions: map[string]sessionRec{},
		history: map[int64][]*MatchRec{}, nextUser: 1, nextMatch: 1}
	s.dummy = User{Salt: randBytes(16), Iter: pbkdfIter}
	s.dummy.Hash = hashPassword("x", s.dummy.Salt, s.dummy.Iter)

	if b, err := os.ReadFile(s.path("snapshot.json")); err == nil {
		var snap snapshot
		if err := json.Unmarshal(b, &snap); err != nil {
			return nil, fmt.Errorf("snapshot.json o'qilmadi: %w", err)
		}
		for _, u := range snap.Users {
			s.users[u.ID] = u
			s.byName[strings.ToLower(u.Username)] = u
		}
		if snap.Sessions != nil {
			s.sessions = snap.Sessions
		}
		s.nextUser, s.nextMatch = max(snap.NextUser, 1), max(snap.NextMatch, 1)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := s.loadMatches(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.path("matches.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s.matchLog = f
	s.mu.Lock()
	s.pruneSessionsLocked()
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	go s.maintenance()
	return s, nil
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func (s *Store) loadMatches() error {
	f, err := os.Open(s.path("matches.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var m MatchRec
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue // yarim yozilgan oxirgi qator (masalan, elektr o'chganda) — tashlab ketiladi
		}
		s.remember(&m)
		if m.ID >= s.nextMatch {
			s.nextMatch = m.ID + 1
		}
	}
	return sc.Err()
}

func (s *Store) remember(m *MatchRec) {
	for _, p := range m.Players {
		if p.UserID == 0 {
			continue
		}
		h := append(s.history[p.UserID], m)
		if len(h) > historyKeep {
			h = h[len(h)-historyKeep:]
		}
		s.history[p.UserID] = h
	}
}

// saveLocked: snapshot'ni vaqtinchalik faylga yozib, so'ng almashtiradi — yarim yozilgan fayl qolmaydi.
func (s *Store) saveLocked() error {
	snap := snapshot{Version: 1, NextUser: s.nextUser, NextMatch: s.nextMatch, Sessions: s.sessions}
	for _, u := range s.users {
		snap.Users = append(snap.Users, u)
	}
	sort.Slice(snap.Users, func(i, j int) bool { return snap.Users[i].ID < snap.Users[j].ID })
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	tmp := s.path("snapshot.json.tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path("snapshot.json")); err != nil {
		return err
	}
	if d, err := os.Open(s.dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
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

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) newSessionLocked(uid int64) string {
	token := base64.RawURLEncoding.EncodeToString(randBytes(32))
	s.sessions[tokenKey(token)] = sessionRec{UserID: uid, Expires: time.Now().Add(sessionTTL).Unix()}
	return token
}

func (s *Store) pruneSessionsLocked() bool {
	now, changed := time.Now().Unix(), false
	for k, v := range s.sessions {
		if v.Expires < now || s.users[v.UserID] == nil {
			delete(s.sessions, k)
			changed = true
		}
	}
	return changed
}

func validPassword(p string) bool { return len(p) >= 6 && len(p) <= 72 }

// Register yangi hisob ochadi va unga sessiya beradi.
func (s *Store) Register(username, password string) (User, string, error) {
	username = strings.TrimSpace(username)
	if !usernameRe.MatchString(username) {
		return User{}, "", errBadUsername
	}
	if !validPassword(password) {
		return User{}, "", errBadPassword
	}
	salt := randBytes(16)
	hash := hashPassword(password, salt, pbkdfIter) // sekin amal — qulfdan tashqarida
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byName[strings.ToLower(username)] != nil {
		return User{}, "", errTaken
	}
	u := &User{ID: s.nextUser, Username: username, Salt: salt, Hash: hash, Iter: pbkdfIter, Rating: startRating, Created: time.Now().Unix()}
	s.nextUser++
	s.users[u.ID] = u
	s.byName[strings.ToLower(username)] = u
	token := s.newSessionLocked(u.ID)
	if err := s.saveLocked(); err != nil {
		delete(s.users, u.ID)
		delete(s.byName, strings.ToLower(username))
		return User{}, "", err
	}
	return *u, token, nil
}

func (s *Store) Login(username, password string) (User, string, error) {
	s.mu.Lock()
	u := s.byName[strings.ToLower(strings.TrimSpace(username))]
	ref := s.dummy
	if u != nil {
		ref = *u
	}
	s.mu.Unlock()
	ok := subtle.ConstantTimeCompare(hashPassword(password, ref.Salt, ref.Iter), ref.Hash) == 1
	if u == nil || !ok {
		return User{}, "", errBadLogin
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	token := s.newSessionLocked(u.ID)
	if err := s.saveLocked(); err != nil {
		return User{}, "", err
	}
	return *u, token, nil
}

func (s *Store) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[tokenKey(token)]; ok {
		delete(s.sessions, tokenKey(token))
		s.saveLocked()
	}
}

// Session token bo'yicha foydalanuvchini qaytaradi.
func (s *Store) Session(token string) (User, bool) {
	if token == "" || len(token) > 100 {
		return User{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.sessions[tokenKey(token)]
	if !ok || rec.Expires < time.Now().Unix() {
		return User{}, false
	}
	u := s.users[rec.UserID]
	if u == nil {
		return User{}, false
	}
	return *u, true
}

func (s *Store) User(id int64) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[id]
	if u == nil {
		return User{}, false
	}
	return *u, true
}

// ranked: kamida bitta o'yin o'ynaganlar, reyting bo'yicha kamayish tartibida.
func (s *Store) rankedLocked() []*User {
	var list []*User
	for _, u := range s.users {
		if u.Matches > 0 {
			list = append(list, u)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		if a.Wins != b.Wins {
			return a.Wins > b.Wins
		}
		return a.ID < b.ID
	})
	return list
}

// Leaderboard: eng yaxshi limit ta o'yinchi, jami soni va berilgan foydalanuvchining o'rni (0 — ro'yxatda yo'q).
func (s *Store) Leaderboard(limit int, me int64) (top []User, total int, myPos int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.rankedLocked()
	for i, u := range list {
		if i < limit {
			top = append(top, *u)
		}
		if u.ID == me {
			myPos = i + 1
		}
	}
	return top, len(list), myPos
}

func (s *Store) Profile(name string) (User, int, []MatchRec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.byName[strings.ToLower(name)]
	if u == nil {
		return User{}, 0, nil, false
	}
	pos := 0
	for i, x := range s.rankedLocked() {
		if x.ID == u.ID {
			pos = i + 1
			break
		}
	}
	h := s.history[u.ID]
	out := make([]MatchRec, 0, len(h))
	for i := len(h) - 1; i >= 0; i-- {
		out = append(out, *h[i])
	}
	return *u, pos, out, true
}

// ApplyMatch reytinglarni yangilaydi va o'yinni tarixga yozadi.
func (s *Store) ApplyMatch(res *matchResult) (map[int64]ratingChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registered := 0
	sum, cnt := map[string]float64{}, map[string]float64{}
	for _, p := range res.Players {
		r := float64(startRating)
		if u := s.users[p.UserID]; p.UserID != 0 && u != nil {
			r = float64(u.Rating)
			registered++
		}
		sum[p.Team] += r
		cnt[p.Team]++
	}
	if registered == 0 || cnt["T"] == 0 || cnt["CT"] == 0 {
		return nil, nil
	}
	avg := map[string]float64{"T": sum["T"] / cnt["T"], "CT": sum["CT"] / cnt["CT"]}
	opp := map[string]string{"T": "CT", "CT": "T"}
	m := &MatchRec{ID: s.nextMatch, Room: res.Room, Started: res.Started.Unix(), Ended: res.Ended.Unix(), ScoreT: res.ScoreT, ScoreCT: res.ScoreCT, Winner: res.Winner}
	changes := map[int64]ratingChange{}
	for _, p := range res.Players {
		mp := MatchPlayer{UserID: p.UserID, Name: p.Name, Team: p.Team, Kills: p.Kills, Deaths: p.Deaths, HS: p.HS, Won: p.Team == res.Winner}
		if u := s.users[p.UserID]; p.UserID != 0 && u != nil {
			d := eloDelta(avg[p.Team], avg[opp[p.Team]], mp.Won, u.Matches, res.ScoreT-res.ScoreCT)
			mp.Name, mp.Before, mp.After = u.Username, u.Rating, max(0, u.Rating+d)
			u.Rating = mp.After
			u.Matches++
			if mp.Won {
				u.Wins++
			}
			u.Kills += p.Kills
			u.Deaths += p.Deaths
			u.HS += p.HS
			changes[u.ID] = ratingChange{Before: mp.Before, After: mp.After, Delta: mp.After - mp.Before, Won: mp.Won, Matches: u.Matches}
		} else {
			mp.UserID, mp.Before, mp.After = 0, 0, 0
		}
		m.Players = append(m.Players, mp)
	}
	s.nextMatch++
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	line, _ := json.Marshal(m)
	if _, err := s.matchLog.Write(append(line, '\n')); err == nil {
		s.matchLog.Sync()
	} else {
		log.Printf("matches.jsonl ga yozilmadi: %v", err)
	}
	s.remember(m)
	return changes, nil
}

// maintenance: eskirgan sessiyalarni tozalaydi va kunlik zaxira nusxa oladi.
func (s *Store) maintenance() {
	for {
		s.mu.Lock()
		if s.pruneSessionsLocked() {
			s.saveLocked()
		}
		s.mu.Unlock()
		if err := s.backup(time.Now()); err != nil {
			log.Printf("zaxira nusxa olinmadi: %v", err)
		}
		time.Sleep(time.Hour)
	}
}

func (s *Store) backup(now time.Time) error {
	day := now.Format("2006-01-02")
	dst := filepath.Join(s.dir, "backups", day)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	s.mu.Lock()
	err := copyFile(s.path("snapshot.json"), filepath.Join(dst, "snapshot.json"))
	if err == nil {
		err = copyFile(s.path("matches.jsonl"), filepath.Join(dst, "matches.jsonl"))
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	entries, _ := os.ReadDir(filepath.Join(s.dir, "backups"))
	var days []string
	for _, e := range entries {
		if e.IsDir() {
			days = append(days, e.Name())
		}
	}
	sort.Strings(days)
	for len(days) > 7 {
		os.RemoveAll(filepath.Join(s.dir, "backups", days[0]))
		days = days[1:]
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
