package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTelegram: Bot API o'rniga — getUpdates bir marta berilgan xabarlarni qaytaradi, yuborilganlarni yozib oladi.
type fakeTelegram struct {
	mu      sync.Mutex
	updates []map[string]any
	sent    []map[string]any
	calls   map[string]int
	status  int
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	method := parts[len(parts)-1]
	if !strings.HasPrefix(parts[1], "bot"+testBotToken) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "Unauthorized"})
		return
	}
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.calls[method]++
	if f.status != 0 {
		st := f.status
		f.mu.Unlock()
		w.WriteHeader(st)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "err"})
		return
	}
	var result any = true
	switch method {
	case "getUpdates":
		result = f.updates
		f.updates = []map[string]any{}
		f.mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
		return
	case "sendMessage":
		f.sent = append(f.sent, body)
	}
	f.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func tgMsg(id int64, chat int64, typ string, from int64, text string) map[string]any {
	return map[string]any{"update_id": id, "message": map[string]any{"text": text, "chat": map[string]any{"id": chat, "type": typ}, "from": map[string]any{"id": from}}}
}

func runFakeBot(t *testing.T, st *Store, h *hub, ups []map[string]any, want int) (*fakeTelegram, []map[string]any) {
	t.Helper()
	f := &fakeTelegram{updates: ups, calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	b := newTgBot(tgConfig{token: testBotToken, bot: "ZarbaBot"}, "https://o'yin.test/", h, st)
	b.api, b.poll = srv.URL, time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.sent)
		f.mu.Unlock()
		if n >= want || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond) // ortiqcha javob yo'qligini ko'rish uchun
	cancel()
	<-done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f, append([]map[string]any(nil), f.sent...)
}

func TestTgBotCommand(t *testing.T) {
	b := &tgBot{name: "ZarbaBot"}
	for in, want := range map[string]string{"/start": "start", "/TOP": "top", "/top@ZarbaBot": "top", "/top@zarbabot x": "top", "/top@BoshqaBot": "", "salom": "", "/start abc": "start"} {
		if got := b.command(in); got != want {
			t.Errorf("command(%q) = %q, kerak %q", in, got, want)
		}
	}
}

func TestTgBotReplies(t *testing.T) {
	h := newHub(16, nil)
	ups := []map[string]any{
		tgMsg(10, 1, "private", 501, "/start"),
		tgMsg(11, -100, "group", 502, "salom hammaga"),  // guruhda oddiy matn — javobsiz
		tgMsg(12, -100, "group", 502, "/top@BoshqaBot"), // boshqa botga
		tgMsg(13, -200, "supergroup", 503, "/online@ZarbaBot"),
		tgMsg(14, 2, "private", 504, "nima gap"),
		tgMsg(15, 2, "private", 504, "/help"), // 1 soniya ichida ikkinchi xabar — spam himoyasi
	}
	f, sent := runFakeBot(t, nil, h, ups, 3)
	if f.calls["deleteWebhook"] == 0 || f.calls["setMyCommands"] == 0 {
		t.Fatalf("sozlash chaqirilmadi: %v", f.calls)
	}
	if len(sent) != 3 {
		t.Fatalf("3 ta javob kerak edi, %d: %v", len(sent), sent)
	}
	start := sent[0]
	if start["chat_id"].(float64) != 1 || !strings.Contains(start["text"].(string), "Zarba Nuqtasi") || start["parse_mode"] != "HTML" {
		t.Fatalf("/start: %v", start)
	}
	kb, _ := json.Marshal(start["reply_markup"])
	if !strings.Contains(string(kb), `"url":"https://o'yin.test"`) {
		t.Fatalf("O'ynash tugmasi: %s", kb)
	}
	if sent[1]["chat_id"].(float64) != -200 || !strings.Contains(sent[1]["text"].(string), "hech kim o'ynamayapti") {
		t.Fatalf("/online: %v", sent[1])
	}
	if sent[2]["chat_id"].(float64) != 2 || !strings.Contains(sent[2]["text"].(string), "/help") {
		t.Fatalf("oddiy matn: %v", sent[2])
	}
}

func TestTgBotBadToken(t *testing.T) {
	f := &fakeTelegram{calls: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	b := newTgBot(tgConfig{token: "999:WRONG", bot: "ZarbaBot"}, "https://x.test", newHub(4, nil), nil)
	b.api = srv.URL
	done := make(chan struct{})
	go func() { b.run(context.Background()); close(done) }()
	select {
	case <-done: // noto'g'ri token — bot to'xtaydi, qayta-qayta urinmaydi
	case <-time.After(3 * time.Second):
		t.Fatal("noto'g'ri token bilan bot to'xtamadi")
	}
}

func TestTgBotAccount(t *testing.T) {
	st := testStore(t, "")
	ctx := context.Background()
	u, _, err := st.Register(ctx, "Botir", "parol123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE users SET rating = 1234, matches = 20, wins = 12, kills = 300, deaths = 200, hs = 90 WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkTelegram(ctx, u.ID, tgUser{ID: 777, Username: "botir_tg"}); err != nil {
		t.Fatal(err)
	}
	h := newHub(16, st)
	ups := []map[string]any{
		tgMsg(1, 777, "private", 777, "/me"),
		tgMsg(2, 888, "private", 888, "/me"),
		tgMsg(3, 999, "private", 999, "/top"),
		tgMsg(4, 7777, "private", 777, "/start"),
	}
	_, sent := runFakeBot(t, st, h, ups, 4)
	if len(sent) != 4 {
		t.Fatalf("4 ta javob kerak edi: %v", sent)
	}
	me := sent[0]["text"].(string)
	if !strings.Contains(me, "Botir") || !strings.Contains(me, "reyting 1234") || !strings.Contains(me, "1-o'rin") || !strings.Contains(me, "K/D 1.50") || !strings.Contains(me, "30%") {
		t.Fatalf("/me: %s", me)
	}
	kb, _ := json.Marshal(sent[0]["reply_markup"])
	if !strings.Contains(string(kb), "#p=Botir") {
		t.Fatalf("profil tugmasi: %s", kb)
	}
	if !strings.Contains(sent[1]["text"].(string), "ulanmagan") {
		t.Fatalf("ulanmagan /me: %v", sent[1]["text"])
	}
	if top := sent[2]["text"].(string); !strings.Contains(top, "🥇 Botir — 1234") || !strings.Contains(top, "Jami reytingda: 1") {
		t.Fatalf("/top: %s", top)
	}
	if s := sent[3]["text"].(string); !strings.Contains(s, "Salom, <b>Botir</b>") {
		t.Fatalf("/start (ulangan): %s", s)
	}
}
