package main

// Telegram bot: xabarlarga javob beradi (kirish tugmasidan tashqari).
//
// TELEGRAM_BOT_TOKEN berilgan bo'lsa server Telegram'dan yangi xabarlarni o'zi so'rab turadi
// (getUpdates, long polling) — webhook, ochiq port yoki nginx sozlamasi kerak emas.
// Buyruqlar:
//
//	/start  — salom, «O'ynash» tugmasi; Telegram ulangan bo'lsa — reyting va daraja
//	/top    — reytingdagi eng yaxshi 10 o'yinchi
//	/online — hozir nechta odam o'ynayapti va ochiq xonalar (havolasi bilan)
//	/me     — o'z natijalari (Telegram hisobga ulangan bo'lsa)
//	/help   — buyruqlar ro'yxati
//
// O'yin manzili PUBLIC_URL dan olinadi (standart — https://game.telegap.uz).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const tgAPIBase = "https://api.telegram.org"

type tgBot struct {
	token, name, site, api string
	h                      *hub
	st                     *Store
	client                 *http.Client
	poll                   time.Duration // getUpdates kutish vaqti

	mu   sync.Mutex
	last map[int64]time.Time // chat → oxirgi javob (spamga qarshi)
}

func newTgBot(cfg tgConfig, site string, h *hub, st *Store) *tgBot {
	return &tgBot{token: cfg.token, name: cfg.bot, site: strings.TrimRight(site, "/"), api: tgAPIBase, h: h, st: st,
		client: &http.Client{Timeout: 70 * time.Second}, poll: 50 * time.Second, last: map[int64]time.Time{}}
}

type tgUpdate struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		Text string `json:"text"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
	} `json:"message"`
}

var errTgUnauthorized = errors.New("bot tokeni noto'g'ri (Telegram 401)")

// call: Bot API usuli; javobning "result" qismi out ga yoziladi.
func (b *tgBot) call(ctx context.Context, method string, params any, out any) error {
	body, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.api+"/bot"+b.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var r struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	json.NewDecoder(res.Body).Decode(&r)
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusNotFound {
		return errTgUnauthorized
	}
	if !r.OK {
		return fmt.Errorf("%s: %d %s", method, res.StatusCode, r.Description)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

var tgCommands = []map[string]string{
	{"command": "start", "description": "O'yinni ochish"},
	{"command": "online", "description": "Hozir kim o'ynayapti"},
	{"command": "top", "description": "Reytingdagi eng yaxshi 10 o'yinchi"},
	{"command": "me", "description": "Mening natijalarim"},
	{"command": "help", "description": "Yordam"},
}

// run: bot tsikli (alohida goroutine'da). Token noto'g'ri bo'lsa to'xtaydi.
func (b *tgBot) run(ctx context.Context) {
	setup := func() error {
		// webhook o'rnatilgan bo'lsa getUpdates ishlamaydi
		if err := b.call(ctx, "deleteWebhook", map[string]any{}, nil); err != nil {
			return err
		}
		b.call(ctx, "setMyCommands", map[string]any{"commands": tgCommands}, nil)
		b.call(ctx, "setMyShortDescription", map[string]any{"short_description": "Zarba Nuqtasi — brauzerdagi taktik otishma. " + b.site}, nil)
		b.call(ctx, "setMyDescription", map[string]any{"description": "Zarba Nuqtasi — brauzerdagi taktik otishma: botlarga qarshi yoki do'stlar bilan onlayn, reyting, klanlar va turnirlar.\n\nBu bot orqali o'yinga kirasiz, reytingni va hozir kim o'ynayotganini ko'rasiz."}, nil)
		return nil
	}
	wait := 5 * time.Second
	for {
		err := setup()
		if err == nil {
			break
		}
		if errors.Is(err, errTgUnauthorized) {
			log.Printf("Telegram bot: %v — bot javob bermaydi (TELEGRAM_BOT_TOKEN ni tekshiring)", err)
			return
		}
		log.Printf("Telegram bot: ulanib bo'lmadi: %v (%v dan keyin qayta)", err, wait)
		if !sleepCtx(ctx, wait) {
			return
		}
		wait = min(wait*2, 5*time.Minute)
	}
	log.Printf("Telegram bot @%s xabarlarga javob beryapti", b.name)
	var offset int64
	wait = 5 * time.Second
	for ctx.Err() == nil {
		var ups []tgUpdate
		err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": int(b.poll / time.Second), "allowed_updates": []string{"message"}}, &ups)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, errTgUnauthorized) {
				log.Printf("Telegram bot: %v — to'xtadi", err)
				return
			}
			log.Printf("Telegram bot: %v (%v dan keyin qayta)", err, wait)
			if !sleepCtx(ctx, wait) {
				return
			}
			wait = min(wait*2, 2*time.Minute)
			if strings.Contains(err.Error(), "409") { // boshqa joyda webhook yoki ikkinchi nusxa
				b.call(ctx, "deleteWebhook", map[string]any{}, nil)
			}
			continue
		}
		wait = 5 * time.Second
		for _, u := range ups {
			offset = max(offset, u.ID+1)
			b.handle(ctx, u)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// command: "/top@zarba_bot arg" → "top" (boshqa botga atalgan bo'lsa "").
func (b *tgBot) command(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	cmd, _, _ := strings.Cut(strings.TrimPrefix(text, "/"), " ")
	cmd, to, found := strings.Cut(cmd, "@")
	if found && !strings.EqualFold(to, b.name) {
		return ""
	}
	return strings.ToLower(cmd)
}

func (b *tgBot) handle(ctx context.Context, u tgUpdate) {
	m := u.Message
	if m == nil || m.Chat.ID == 0 {
		return
	}
	private := m.Chat.Type == "private"
	cmd := b.command(strings.TrimSpace(m.Text))
	if cmd == "" && !private { // guruhda faqat buyruqlarga
		return
	}
	b.mu.Lock()
	now := time.Now()
	if t, ok := b.last[m.Chat.ID]; ok && now.Sub(t) < time.Second {
		b.mu.Unlock()
		return
	}
	b.last[m.Chat.ID] = now
	if len(b.last) > 5000 {
		for k, t := range b.last {
			if now.Sub(t) > time.Minute {
				delete(b.last, k)
			}
		}
	}
	b.mu.Unlock()
	var from int64
	if m.From != nil {
		from = m.From.ID
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	text, kb := b.reply(ctx, cmd, from)
	msg := map[string]any{"chat_id": m.Chat.ID, "text": text, "parse_mode": "HTML", "link_preview_options": map[string]any{"is_disabled": true}}
	if kb != nil {
		msg["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	if err := b.call(ctx, "sendMessage", msg, nil); err != nil {
		log.Printf("Telegram bot: javob yuborilmadi: %v", err)
	}
}

type tgButton = map[string]string

// reply: buyruqqa javob matni (HTML) va tugmalar.
func (b *tgBot) reply(ctx context.Context, cmd string, from int64) (string, [][]tgButton) {
	play := [][]tgButton{{{"text": "🎮 O'ynash", "url": b.site}}}
	switch cmd {
	case "start":
		s := "<b>Zarba Nuqtasi</b> — brauzerdagi taktik otishma. Hech narsa o'rnatish shart emas: tugmani bosing va o'ynang — botlarga qarshi yoki do'stlar bilan onlayn.\n\n"
		if u, ok := b.userByTelegram(ctx, from); ok {
			s += "Salom, <b>" + html.EscapeString(u.Username) + "</b>! " + b.userLine(ctx, u) + "\n\n"
		} else {
			s += "Telegram'ingizni o'yindagi hisobga ulasangiz (Hisob → Telegram'ni ulash), parolsiz kirasiz va natijalaringizni shu yerda ko'rasiz.\n\n"
		}
		return s + "/online — hozir kim o'ynayapti\n/top — reyting\n/me — mening natijalarim", play
	case "top":
		return b.topText(ctx), play
	case "online":
		return b.onlineText(), play
	case "me":
		u, ok := b.userByTelegram(ctx, from)
		if !ok {
			return "Telegram'ingiz hali hech qaysi hisobga ulanmagan. O'yinda: Hisob → «Telegram'ni ulash» (yoki kirish panelida «Log in with Telegram»).", play
		}
		kd := float64(u.Kills)
		if u.Deaths > 0 {
			kd = float64(u.Kills) / float64(u.Deaths)
		}
		s := "<b>" + html.EscapeString(u.Username) + "</b>\n" + b.userLine(ctx, u) + "\n\n" +
			fmt.Sprintf("O'yinlar: %d · g'alaba: %d\nO'ldirish: %d · o'lim: %d · K/D %.2f · boshga: %d%%", u.Matches, u.Wins, u.Kills, u.Deaths, kd, pctOf(u.HS, u.Kills))
		return s, [][]tgButton{{{"text": "🎮 O'ynash", "url": b.site}, {"text": "📊 Profil", "url": b.site + "/#p=" + url.PathEscape(u.Username)}}}
	case "help":
		return "Buyruqlar:\n/start — o'yinni ochish\n/online — hozir kim o'ynayapti, ochiq xonalar\n/top — reytingdagi eng yaxshi 10 o'yinchi\n/me — mening natijalarim (Telegram hisobga ulangan bo'lsa)", play
	}
	return "O'ynash uchun pastdagi tugmani bosing. Buyruqlar: /help", play
}

func pctOf(a, b int) int {
	if b <= 0 {
		return 0
	}
	return a * 100 / b
}

func (b *tgBot) userByTelegram(ctx context.Context, tgID int64) (User, bool) {
	if tgID == 0 || !b.st.Ready() {
		return User{}, false
	}
	u, err := scanUser(b.st.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE tg_id = $1`, tgID))
	return u, err == nil
}

// userLine: "Reyting 1520 · Oltin · 12-daraja · 5-o'rin".
func (b *tgBot) userLine(ctx context.Context, u User) string {
	rk := rankOf(u.Rating, u.Matches)
	lv, _, _ := levelOf(u.XP)
	s := fmt.Sprintf("%d-daraja · ", lv)
	if rk.Calibrating {
		return s + fmt.Sprintf("kalibrlash %d/%d o'yin", rk.Played, rk.Need)
	}
	s += fmt.Sprintf("reyting %d (%s)", u.Rating, rk.Name)
	if pos, err := b.st.position(ctx, u.ID); err == nil && pos > 0 {
		s += fmt.Sprintf(" · %d-o'rin", pos)
	}
	return s
}

func (b *tgBot) topText(ctx context.Context) string {
	if !b.st.Ready() {
		return "Reyting vaqtincha ishlamayapti, birozdan keyin urinib ko'ring."
	}
	top, total, _, err := b.st.Leaderboard(ctx, 10, 0)
	if err != nil {
		log.Printf("Telegram bot: reyting: %v", err)
		return "Reyting vaqtincha ishlamayapti, birozdan keyin urinib ko'ring."
	}
	if len(top) == 0 {
		return "Reyting hali bo'sh — birinchi bo'ling!"
	}
	var s strings.Builder
	s.WriteString("🏆 <b>Reyting</b>\n")
	medals := []string{"🥇", "🥈", "🥉"}
	for i, u := range top {
		p := strconv.Itoa(i+1) + "."
		if i < len(medals) {
			p = medals[i]
		}
		rk := rankOf(u.Rating, u.Matches)
		name := rk.Name
		if rk.Calibrating {
			name = "kalibrlash"
		}
		fmt.Fprintf(&s, "%s %s — %d · %s\n", p, html.EscapeString(u.Username), u.Rating, html.EscapeString(name))
	}
	fmt.Fprintf(&s, "\nJami reytingda: %d o'yinchi", total)
	return s.String()
}

func (b *tgBot) onlineText() string {
	humans, rooms := 0, 0
	for _, r := range b.h.roomList() {
		r.mu.Lock()
		_, n := r.botCountLocked()
		r.mu.Unlock()
		if n > 0 {
			humans += n
			rooms++
		}
	}
	var s strings.Builder
	if humans == 0 {
		s.WriteString("Hozir hech kim o'ynamayapti — birinchi bo'ling! Botlar bilan ham o'ynasa bo'ladi.")
	} else {
		fmt.Fprintf(&s, "🟢 Hozir <b>%d</b> kishi o'ynayapti (%d xonada).", humans, rooms)
	}
	list := []roomListing{}
	for _, l := range b.h.publicRooms() {
		if l.Humans > 0 && !l.MM {
			list = append(list, l)
		}
	}
	if len(list) > 0 {
		s.WriteString("\n\nOchiq xonalar:")
		for i, l := range list {
			if i == 8 {
				fmt.Fprintf(&s, "\n… va yana %d ta", len(list)-i)
				break
			}
			link := b.site + "/x/" + url.PathEscape(l.Map) + "/" + url.PathEscape(l.Code)
			line := fmt.Sprintf("\n• <a href=\"%s\">%s</a> — %s · %d/%d", html.EscapeString(link), html.EscapeString(mapDisplayName(l.Map)), html.EscapeString(modeNames[l.Mode]), l.Humans+l.Bots, l.Max)
			if l.Phase != "warmup" && l.Round > 0 && l.Mode != modeDM && l.Mode != modeGG {
				line += fmt.Sprintf(" · %d:%d", l.SA, l.SB)
			}
			s.WriteString(line)
		}
	}
	return s.String()
}
