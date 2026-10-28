package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testBotToken = "123456:TEST-token"

// signTelegram: Telegram Login Widget kabi imzolangan ma'lumot.
func signTelegram(token string, d map[string]string) map[string]any {
	keys := []string{}
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, k := range keys {
		lines = append(lines, k+"="+d[k])
	}
	sec := sha256.Sum256([]byte(token))
	m := hmac.New(sha256.New, sec[:])
	m.Write([]byte(strings.Join(lines, "\n")))
	out := map[string]any{"hash": hex.EncodeToString(m.Sum(nil))}
	for k, v := range d {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && (k == "id" || k == "auth_date") {
			out[k] = float64(n) // vidjet raqamlarni son qilib beradi
		} else {
			out[k] = v
		}
	}
	return out
}

func tgData(id int64, user string, at time.Time) map[string]string {
	return map[string]string{"id": strconv.FormatInt(id, 10), "first_name": "Ali", "username": user, "auth_date": strconv.FormatInt(at.Unix(), 10)}
}

func TestVerifyTelegram(t *testing.T) {
	now := time.Now()
	d := tgData(42, "zarba_fan", now)
	raw := signTelegram(testBotToken, d)
	s := map[string]string{}
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			s[k] = x
		case float64:
			s[k] = strconv.FormatFloat(x, 'f', -1, 64)
		}
	}
	u, err := verifyTelegram(s, testBotToken, now)
	if err != nil || u.ID != 42 || u.display() != "@zarba_fan" {
		t.Fatalf("to'g'ri imzo: %+v %v", u, err)
	}
	if _, err := verifyTelegram(s, "boshqa:token", now); err != errTgSign {
		t.Fatalf("boshqa token: %v", err)
	}
	s["first_name"] = "Vali"
	if _, err := verifyTelegram(s, testBotToken, now); err != errTgSign {
		t.Fatalf("o'zgartirilgan ma'lumot: %v", err)
	}
	old := signTelegram(testBotToken, tgData(42, "zarba_fan", now.Add(-25*time.Hour)))
	so := map[string]string{"hash": old["hash"].(string), "first_name": "Ali", "username": "zarba_fan", "id": "42", "auth_date": strconv.FormatInt(now.Add(-25*time.Hour).Unix(), 10)}
	if _, err := verifyTelegram(so, testBotToken, now); err != errTgOld {
		t.Fatalf("eski ma'lumot: %v", err)
	}
	for in, want := range map[tgUser]string{{ID: 1, Username: "Zarba_Fan"}: "Zarba_Fan", {ID: 2, First: "Олег"}: "tg2", {ID: 3, Username: "a", First: "Bekzod Qodirov"}: "BekzodQodirov",
		{ID: 4, Username: "very_long_telegram_name"}: "very_long_telegr"} {
		if got := tgLoginName(in); got != want {
			t.Errorf("tgLoginName(%+v) = %q, kerak %q", in, got, want)
		}
	}
}

func newTGServer(t *testing.T, tg tgConfig) (*httptest.Server, *Store) {
	st := testStore(t, "")
	a := newAPI(st, nil)
	a.tg = tg
	mux := http.NewServeMux()
	a.routes(mux)
	return httptest.NewServer(mux), st
}

func TestTelegramAPI(t *testing.T) {
	srv, _ := newTGServer(t, tgConfig{token: testBotToken, bot: "ZarbaBot"})
	defer srv.Close()
	now := time.Now()
	c := newTestClient(srv.URL)
	if _, info := c.get(t, "/api/telegram"); info["bot"] != "ZarbaBot" {
		t.Fatalf("bot nomi: %v", info)
	}
	// noto'g'ri imzo
	bad := signTelegram("boshqa:token", tgData(555, "zarba_fan", now))
	if code, _ := c.post(t, "/api/telegram/login", bad); code != 401 {
		t.Fatalf("soxta imzo: %d", code)
	}
	// birinchi kirish — yangi hisob
	code, out := c.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(555, "zarba_fan", now)))
	u, _ := out["user"].(map[string]any)
	if code != 200 || out["created"] != true || u["username"] != "zarba_fan" {
		t.Fatalf("telegram bilan kirish: %d %v", code, out)
	}
	if _, info := c.get(t, "/api/telegram"); info["linked"] != "@zarba_fan" || info["noOld"] != true {
		t.Fatalf("ulangan: %v", info)
	}
	// parolni tiklash: Telegram sessiyasida eski parolsiz
	if code, out := c.post(t, "/api/password", map[string]any{"new": "yangiparol1"}); code != 200 {
		t.Fatalf("yangi parol: %d %v", code, out)
	}
	p := newTestClient(srv.URL)
	if code, _ := p.post(t, "/api/login", credentials{"zarba_fan", "yangiparol1"}); code != 200 {
		t.Fatalf("yangi parol bilan kirish: %d", code)
	}
	// parol sessiyasida eski parol kerak
	if code, _ := p.post(t, "/api/password", map[string]any{"old": "xato-parol", "new": "boshqaparol"}); code != 400 {
		t.Fatalf("eski parolsiz o'zgardi: %d", code)
	}
	if code, _ := p.post(t, "/api/password", map[string]any{"old": "yangiparol1", "new": "boshqaparol"}); code != 200 {
		t.Fatalf("parol o'zgarmadi: %d", code)
	}
	if code, _ := c.get(t, "/api/me"); code != 401 {
		t.Fatalf("parol o'zgargach eski sessiya yopilmadi: %d", code)
	}
	// ikkinchi marta — o'sha hisob
	c2 := newTestClient(srv.URL)
	code, out = c2.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(555, "zarba_fan", now)))
	if u, _ := out["user"].(map[string]any); code != 200 || out["created"] != false || u["username"] != "zarba_fan" {
		t.Fatalf("qayta kirish: %d %v", code, out)
	}
	// Telegram sessiyasida uzib bo'lmaydi, parol sessiyasida bo'ladi
	if code, _ := c2.post(t, "/api/telegram/unlink", map[string]any{}); code != 403 {
		t.Fatalf("telegram sessiyasida uzildi: %d", code)
	}
	// boshqa o'yinchi: parol bilan ochilgan hisobga Telegram ulaydi; band Telegram ulanmaydi
	o := newTestClient(srv.URL)
	o.post(t, "/api/register", credentials{"Boshqa", "parol123"})
	if code, _ := o.post(t, "/api/telegram/link", signTelegram(testBotToken, tgData(555, "zarba_fan", now))); code != 409 {
		t.Fatalf("band telegram ulandi: %d", code)
	}
	if code, out := o.post(t, "/api/telegram/link", signTelegram(testBotToken, tgData(777, "", now))); code != 200 || out["linked"] != "Ali" {
		t.Fatalf("ulash: %d %v", code, out)
	}
	o3 := newTestClient(srv.URL)
	if code, out := o3.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(777, "", now))); code != 200 || out["user"].(map[string]any)["username"] != "Boshqa" {
		t.Fatalf("ulangan hisobga kirish: %d %v", code, out)
	}
	if code, _ := o.post(t, "/api/telegram/unlink", map[string]any{}); code != 200 {
		t.Fatalf("uzish: %d", code)
	}
	o4 := newTestClient(srv.URL)
	if code, out := o4.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(777, "", now))); code != 200 || out["created"] != true {
		t.Fatalf("uzilgandan keyin yangi hisob ochilishi kerak: %d %v", code, out)
	}
	// band login: raqam qo'shiladi
	o5 := newTestClient(srv.URL)
	if code, out := o5.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(888, "zarba_fan", now))); code != 200 || out["user"].(map[string]any)["username"] == "zarba_fan" {
		t.Fatalf("band login: %d %v", code, out)
	}
}

func TestTelegramOff(t *testing.T) {
	srv, _ := newTGServer(t, tgConfig{})
	defer srv.Close()
	c := newTestClient(srv.URL)
	if _, info := c.get(t, "/api/telegram"); info["bot"] != "" {
		t.Fatalf("o'chiq: %v", info)
	}
	if code, _ := c.post(t, "/api/telegram/login", signTelegram(testBotToken, tgData(1, "x", time.Now()))); code != 404 {
		t.Fatalf("o'chiq bo'lsa ham kirdi: %d", code)
	}
}
