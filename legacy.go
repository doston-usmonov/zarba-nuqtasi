package main

// Bir martalik ko'chirish: PostgreSQL'dan oldingi versiya hisoblarni /data/snapshot.json va
// /data/matches.jsonl fayllarida saqlagan. Baza bo'sh bo'lsa, ular shu yerdan import qilinadi
// (hisoblar, parollar, sessiyalar, reyting va o'yinlar tarixi) va fayl "*.imported" deb qayta nomlanadi.

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type legacyUser struct {
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

type legacySnapshot struct {
	Users    []legacyUser `json:"users"`
	Sessions map[string]struct {
		UserID  int64 `json:"u"`
		Expires int64 `json:"exp"`
	} `json:"sessions"`
}

type legacyMatch struct {
	ID      int64  `json:"id"`
	Room    string `json:"room"`
	Started int64  `json:"started"`
	Ended   int64  `json:"ended"`
	ScoreT  int    `json:"sT"`
	ScoreCT int    `json:"sCT"`
	Winner  string `json:"winner"`
	Players []struct {
		UserID int64  `json:"uid"`
		Name   string `json:"name"`
		Team   string `json:"team"`
		Kills  int    `json:"k"`
		Deaths int    `json:"d"`
		HS     int    `json:"hs"`
		Won    bool   `json:"won"`
		Before int    `json:"before"`
		After  int    `json:"after"`
	} `json:"players"`
}

func (s *Store) importLegacy(ctx context.Context, dir string) (int, error) {
	snapPath := filepath.Join(dir, "snapshot.json")
	b, err := os.ReadFile(snapPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil // baza allaqachon ishlatilmoqda — import qilinmaydi
	}
	var snap legacySnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	known := map[int64]bool{}
	for _, u := range snap.Users {
		if !usernameRe.MatchString(u.Username) || len(u.Salt) == 0 || len(u.Hash) == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, pass_salt, pass_hash, pass_iter, rating, matches, wins, kills, deaths, hs, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT DO NOTHING`,
			u.ID, u.Username, u.Salt, u.Hash, u.Iter, max(0, u.Rating), u.Matches, u.Wins, u.Kills, u.Deaths, u.HS, time.Unix(u.Created, 0)); err != nil {
			return 0, err
		}
		known[u.ID] = true
	}
	for key, sess := range snap.Sessions {
		h, err := hex.DecodeString(key)
		if err != nil || !known[sess.UserID] || sess.Expires < time.Now().Unix() {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			h, sess.UserID, time.Unix(sess.Expires, 0)); err != nil {
			return 0, err
		}
	}
	if f, err := os.Open(filepath.Join(dir, "matches.jsonl")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			var m legacyMatch
			if json.Unmarshal(sc.Bytes(), &m) != nil || (m.Winner != "T" && m.Winner != "CT") {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO matches (id, room, started_at, ended_at, score_t, score_ct, winner) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				m.ID, m.Room, time.Unix(m.Started, 0), time.Unix(m.Ended, 0), m.ScoreT, m.ScoreCT, m.Winner); err != nil {
				f.Close()
				return 0, err
			}
			for _, p := range m.Players {
				var uid, before, after any
				if p.UserID != 0 && known[p.UserID] {
					uid, before, after = p.UserID, p.Before, p.After
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO match_players (match_id, user_id, name, team, kills, deaths, hs, won, rating_before, rating_after)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, m.ID, uid, p.Name, p.Team, p.Kills, p.Deaths, p.HS, p.Won, before, after); err != nil {
					f.Close()
					return 0, err
				}
			}
		}
		f.Close()
	}
	// avtomatik id'lar import qilinganlardan keyin davom etsin
	for _, q := range []string{
		`SELECT setval(pg_get_serial_sequence('users', 'id'), (SELECT coalesce(max(id), 0) + 1 FROM users), false)`,
		`SELECT setval(pg_get_serial_sequence('matches', 'id'), (SELECT coalesce(max(id), 0) + 1 FROM matches), false)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	stamp := time.Now().Format("20060102-150405")
	os.Rename(snapPath, snapPath+".imported-"+stamp)
	os.Rename(filepath.Join(dir, "matches.jsonl"), filepath.Join(dir, "matches.jsonl.imported-"+stamp))
	return len(known), nil
}
