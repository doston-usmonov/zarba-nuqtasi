package main

// Telegram orqali kirish va parolni tiklash.
//
// TELEGRAM_BOT_TOKEN va TELEGRAM_BOT_NAME (botning @siz nomi) berilsa, kirish panelida Telegram Login
// tugmasi chiqadi (BotFather'da botga /setdomain bilan o'yin domeni ko'rsatilgan bo'lishi kerak).
// Telegram brauzerga imzolangan ma'lumot beradi (id, ism, auth_date, hash); server imzoni bot tokeni
// bilan tekshiradi (HMAC-SHA256, kalit = SHA256(token)) va 24 soatdan eski ma'lumotni qabul qilmaydi.
//
//   - Kirish: shu Telegram ulangan hisobga kiradi; ulangan hisob bo'lmasa — yangi hisob ochiladi
//     (login Telegram nomidan, band bo'lsa raqam qo'shiladi; tasodifiy parol).
//   - Ulash: hisobga kirgan o'yinchi Telegram'ini ulaydi — keyin parolni unutsa ham kira oladi.
//   - Parolni tiklash: Telegram bilan kirilgan sessiyada yangi parol eski parolsiz qo'yiladi.
//     Oddiy sessiyada eski parol so'raladi. Parol o'zgarganda boshqa sessiyalar yopiladi.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type tgConfig struct {
	token, bot string
}

func (c tgConfig) on() bool { return c.token != "" && c.bot != "" }

type tgUser struct {
	ID       int64
	Username string
	First    string
}

func (u tgUser) display() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return u.First
}

var (
	errTgSign   = errors.New("Telegram ma'lumoti tasdiqlanmadi")
	errTgOld    = errors.New("Telegram ma'lumoti eskirgan, qayta urinib ko'ring")
	errTgTaken  = errors.New("Bu Telegram boshqa hisobga ulangan")
	errTgOff    = errors.New("Telegram orqali kirish yoqilmagan")
	errPassOld  = errors.New("Joriy parol noto'g'ri")
	errTgNeedPw = errors.New("Telegram'ni uzish uchun parol bilan kiring (aks holda hisobga kira olmay qolasiz)")
)

// verifyTelegram: Telegram Login Widget ma'lumotining imzosini tekshiradi.
func verifyTelegram(data map[string]string, token string, now time.Time) (tgUser, error) {
	hash := data["hash"]
	keys := make([]string, 0, len(data))
	for k := range data {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + data[k]
	}
	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(lines, "\n")))
	want := hex.EncodeToString(mac.Sum(nil))
	if hash == "" || subtle.ConstantTimeCompare([]byte(want), []byte(strings.ToLower(hash))) != 1 {
		return tgUser{}, errTgSign
	}
	at, err := strconv.ParseInt(data["auth_date"], 10, 64)
	if err != nil {
		return tgUser{}, errTgSign
	}
	if d := now.Sub(time.Unix(at, 0)); d > 24*time.Hour || d < -5*time.Minute {
		return tgUser{}, errTgOld
	}
	id, err := strconv.ParseInt(data["id"], 10, 64)
	if err != nil || id <= 0 {
		return tgUser{}, errTgSign
	}
	return tgUser{ID: id, Username: data["username"], First: data["first_name"]}, nil
}

var nonLogin = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// tgLoginName: Telegram nomidan o'yin logini (3–16 belgi, lotin harflari, raqam, _).
func tgLoginName(u tgUser) string {
	n := nonLogin.ReplaceAllString(u.Username, "")
	if len(n) < 3 {
		n = nonLogin.ReplaceAllString(u.First, "")
	}
	if len(n) < 3 {
		n = "tg" + strconv.FormatInt(u.ID%1000000, 10)
	}
	if len(n) > 16 {
		n = n[:16]
	}
	return n
}

// ---- baza ----

// TelegramLogin: shu Telegram ulangan hisobga sessiya; bo'lmasa yangi hisob ochadi.
func (s *Store) TelegramLogin(ctx context.Context, tu tgUser) (User, string, bool, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE tg_id = $1`, tu.ID))
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		base := tgLoginName(tu)
		pass := hex.EncodeToString(randBytes(18)) // hech kim bilmaydi: kirish faqat Telegram bilan, keyin parol qo'yiladi
		for i := 0; i < 20; i++ {
			name := base
			if i > 0 {
				suf := strconv.Itoa(i + 1 + int(randBytes(1)[0])%90)
				if len(name)+len(suf) > 16 {
					name = name[:16-len(suf)]
				}
				name += suf
			}
			u, _, err = s.Register(ctx, name, pass)
			if !errors.Is(err, errTaken) {
				break
			}
		}
		if err != nil {
			return User{}, "", false, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET tg_id = $2, tg_name = $3 WHERE id = $1`, u.ID, tu.ID, tu.display()); err != nil {
			return User{}, "", false, err
		}
		s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, u.ID) // Register bergan parol sessiyasi kerak emas
		created = true
	} else if err != nil {
		return User{}, "", false, err
	} else {
		s.db.ExecContext(ctx, `UPDATE users SET tg_name = $2 WHERE id = $1`, u.ID, tu.display())
	}
	token := base64.RawURLEncoding.EncodeToString(randBytes(32))
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at, via) VALUES ($1, $2, $3, 'tg')`, tokenHash(token), u.ID, time.Now().Add(sessionTTL)); err != nil {
		return User{}, "", false, err
	}
	return u, token, created, nil
}

// LinkTelegram: hisobga Telegram ulaydi.
func (s *Store) LinkTelegram(ctx context.Context, uid int64, tu tgUser) error {
	var other int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE tg_id = $1`, tu.ID).Scan(&other)
	if err == nil && other != uid {
		return errTgTaken
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET tg_id = $2, tg_name = $3 WHERE id = $1`, uid, tu.ID, tu.display())
	return err
}

func (s *Store) UnlinkTelegram(ctx context.Context, uid int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET tg_id = NULL, tg_name = NULL WHERE id = $1`, uid)
	return err
}

// telegramOf: ulangan Telegram nomi ("" — ulanmagan).
func (s *Store) telegramOf(ctx context.Context, uid int64) string {
	var n sql.NullString
	s.db.QueryRowContext(ctx, `SELECT tg_name FROM users WHERE id = $1 AND tg_id IS NOT NULL`, uid).Scan(&n)
	return n.String
}

// sessionVia: sessiya qanday ochilgan (pass yoki tg).
func (s *Store) sessionVia(ctx context.Context, token string) string {
	var via string
	s.db.QueryRowContext(ctx, `SELECT via FROM sessions WHERE token_hash = $1`, tokenHash(token)).Scan(&via)
	return via
}

// SetPassword: yangi parol; oldOK bo'lmasa eski parol tekshiriladi. Boshqa sessiyalar yopiladi.
func (s *Store) SetPassword(ctx context.Context, uid int64, old, pass, keepToken string, oldOK bool) error {
	if !validPassword(pass) {
		return errBadPassword
	}
	u, ok := s.User(ctx, uid)
	if !ok {
		return errBadLogin
	}
	if !oldOK && subtle.ConstantTimeCompare(hashPassword(old, u.Salt, u.Iter), u.Hash) != 1 {
		return errPassOld
	}
	salt := randBytes(16)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET pass_salt = $2, pass_hash = $3, pass_iter = $4 WHERE id = $1`, uid, salt, hashPassword(pass, salt, pbkdfIter), pbkdfIter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, uid, tokenHash(keepToken)); err != nil {
		return err
	}
	// endi parolni biladi: bu sessiya ham oddiy bo'ladi
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET via = 'pass' WHERE token_hash = $1`, tokenHash(keepToken)); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- API ----

func (a *api) telegramRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/telegram", h(a.tgInfo))
	mux.HandleFunc("POST /api/telegram/login", h(a.tgLogin))
	mux.HandleFunc("POST /api/telegram/link", h(a.tgLink))
	mux.HandleFunc("POST /api/telegram/unlink", h(a.tgUnlink))
	mux.HandleFunc("POST /api/password", h(a.setPassword))
}

// tgInfo: bot nomi (tugma uchun), ulangan Telegram va parolsiz o'zgartirish mumkinmi.
func (a *api) tgInfo(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"bot": ""}
	if a.tg.on() {
		out["bot"] = a.tg.bot
	}
	if u, ok := a.currentUser(r); ok {
		out["linked"] = a.st.telegramOf(r.Context(), u.ID)
		out["noOld"] = a.st.sessionVia(r.Context(), sessionToken(r)) == "tg"
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) readTelegram(w http.ResponseWriter, r *http.Request) (tgUser, bool) {
	if !a.tg.on() {
		writeErr(w, http.StatusNotFound, errTgOff.Error())
		return tgUser{}, false
	}
	var raw map[string]any
	if !a.readJSON(w, r, &raw) {
		return tgUser{}, false
	}
	data := map[string]string{}
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			data[k] = x
		case float64:
			data[k] = strconv.FormatFloat(x, 'f', -1, 64)
		}
	}
	tu, err := verifyTelegram(data, a.tg.token, time.Now())
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return tgUser{}, false
	}
	return tu, true
}

func (a *api) tgLogin(w http.ResponseWriter, r *http.Request) {
	tu, ok := a.readTelegram(w, r)
	if !ok {
		return
	}
	if !a.login.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "Juda ko'p urinish. Birozdan keyin qayta urinib ko'ring")
		return
	}
	u, token, created, err := a.st.TelegramLogin(r.Context(), tu)
	if err != nil {
		serverErr(w, "telegram", err)
		return
	}
	if created {
		log.Printf("yangi o'yinchi (Telegram): %s (#%d)", u.Username, u.ID)
	}
	setSession(w, r, token, int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"user": toOut(u), "created": created})
}

func (a *api) tgLink(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	tu, ok := a.readTelegram(w, r)
	if !ok {
		return
	}
	if err := a.st.LinkTelegram(r.Context(), u.ID, tu); errors.Is(err, errTgTaken) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		serverErr(w, "telegram", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"linked": tu.display()})
}

func (a *api) tgUnlink(w http.ResponseWriter, r *http.Request) {
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	if a.st.sessionVia(r.Context(), sessionToken(r)) == "tg" {
		writeErr(w, http.StatusForbidden, errTgNeedPw.Error())
		return
	}
	if err := a.st.UnlinkTelegram(r.Context(), u.ID); err != nil {
		serverErr(w, "telegram", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"linked": ""})
}

func (a *api) setPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !a.readJSON(w, r, &req) {
		return
	}
	u, ok := a.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "Hisobga kirilmagan")
		return
	}
	if !a.login.allow("pw:" + strconv.FormatInt(u.ID, 10)) {
		writeErr(w, http.StatusTooManyRequests, "Juda ko'p urinish. Birozdan keyin qayta urinib ko'ring")
		return
	}
	token := sessionToken(r)
	err := a.st.SetPassword(r.Context(), u.ID, req.Old, req.New, token, a.st.sessionVia(r.Context(), token) == "tg")
	switch {
	case errors.Is(err, errBadPassword), errors.Is(err, errPassOld):
		writeErr(w, http.StatusBadRequest, err.Error())
	case err != nil:
		serverErr(w, "parol", err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
