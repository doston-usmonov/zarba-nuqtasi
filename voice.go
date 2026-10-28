package main

// Ovozli chat. Brauzerlar ovozni bir-biriga WebRTC orqali to'g'ridan-to'g'ri yuboradi;
// server faqat ulanish xabarlarini (SDP, ICE nomzodlari) shu xonadagi kerakli o'yinchiga
// uzatadi va STUN/TURN manzillarini beradi. Kim kimni eshitishini brauzerlar o'zi
// boshqaradi: ovoz faqat ruxsat etilgan o'yinchiga jo'natiladi (raqib jamoaga emas).
//
// TURN (coturn) — o'yinchilar to'g'ridan-to'g'ri ulana olmasa (mobil internet, NAT),
// ovozni server orqali o'tkazadi. TURN_SECRET berilsa, har bir ulanishga vaqtinchalik
// login-parol beriladi (coturn'ning use-auth-secret rejimi):
//   username = "<amal qilish muddati, unix>:<o'yinchi id>", parol = base64(HMAC-SHA1(secret, username)).

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

const maxSignal = 6 << 10 // bitta SDP/ICE xabari, bayt

type iceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type voiceCfg struct {
	stun       []string
	turnSecret string
	turnHost   string // bo'sh — o'yin ochilgan domen
	turnPort   string
}

func voiceFromEnv() voiceCfg {
	return voiceCfg{
		stun:       parseList(env("STUN_URLS", "stun:stun.l.google.com:19302")),
		turnSecret: env("TURN_SECRET", ""),
		turnHost:   env("TURN_HOST", ""),
		turnPort:   env("TURN_PORT", "3478"),
	}
}

func parseList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && p != "-" {
			out = append(out, p)
		}
	}
	return out
}

var hostRe = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,253}$`)

func hostOnly(h string) string {
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	if !hostRe.MatchString(h) {
		return ""
	}
	return strings.ToLower(h)
}

func turnCredential(secret, user string) string {
	m := hmac.New(sha1.New, []byte(secret))
	m.Write([]byte(user))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// iceFor — bitta o'yinchi uchun STUN/TURN ro'yxati (host — o'yin ochilgan domen).
func (v voiceCfg) iceFor(host, id string, now time.Time) []iceServer {
	out := []iceServer{}
	if len(v.stun) > 0 {
		out = append(out, iceServer{URLs: v.stun})
	}
	if v.turnSecret == "" {
		return out
	}
	h := v.turnHost
	if h == "" {
		h = hostOnly(host)
	}
	if h == "" {
		return out
	}
	user := fmt.Sprintf("%d:%s", now.Add(12*time.Hour).Unix(), id)
	base := "turn:" + h + ":" + v.turnPort
	return append(out, iceServer{URLs: []string{base + "?transport=udp", base + "?transport=tcp"}, Username: user, Credential: turnCredential(v.turnSecret, user)})
}

// relaySignal uzatadi: {"t":"sig","to":id,"d":{...}} → qabul qiluvchiga {"t":"sig","from":id,"d":{...}}.
func (r *room) relaySignal(from *client, to string, d json.RawMessage) {
	if len(d) == 0 || len(d) > maxSignal || d[0] != '{' || to == from.id {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.clients[to]
	if t == nil || t.spec || from.spec {
		return
	}
	msg := jsonMsg(map[string]any{"t": "sig", "from": from.id, "d": d})
	select {
	case t.send <- msg:
	default:
	}
}
