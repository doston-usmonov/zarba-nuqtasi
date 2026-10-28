package main

// Matnli chat (CS kabi): Y — hammaga, U — faqat o'z jamoasiga. Jamoa xabari faqat
// jamoadoshlarga yuboriladi (raqib uni hech qanday yo'l bilan ko'rmaydi). Ismni server
// o'zi qo'yadi, matn tozalanadi va uzunligi cheklanadi, ko'p yozishga chek qo'yilgan.

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"
)

const chatMaxRunes = 120

func cleanChat(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '​' || r == '‮' || r == '‭' {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > chatMaxRunes {
		s = string(r[:chatMaxRunes])
	}
	return s
}

func (r *room) chatLocked(c *client, d json.RawMessage, now time.Time) {
	var m struct {
		M  string `json:"m"`
		Tm int    `json:"tm"`
	}
	if json.Unmarshal(d, &m) != nil {
		return
	}
	text := cleanChat(m.M)
	if text == "" || (c.info.Sq == "" && !c.spec) {
		return
	}
	// chelak: 4 tagacha xabar ketma-ket, keyin har 1,5 soniyada bittadan
	c.chatTok = min(4, c.chatTok+now.Sub(c.chatAt).Seconds()/1.5)
	c.chatAt = now
	if c.chatTok < 1 {
		c.trySend(jsonMsg(map[string]string{"t": "note", "msg": "Juda tez yozyapsiz — biroz kuting"}))
		return
	}
	c.chatTok--
	name := cleanGuestName(c.info.Nm)
	if c.acct != nil {
		name = c.acct.name
	}
	team := m.Tm == 1 && !c.spec
	msg := jsonMsg(map[string]any{"t": "chat", "from": c.id, "nm": name, "side": c.info.Tm, "dead": !c.ps.alive, "tm": map[bool]int{true: 1}[team], "m": text, "spec": map[bool]int{true: 1}[c.spec]})
	for _, o := range r.clients {
		// kuzatuvchining xabari faqat kuzatuvchilarga (o'yinchilarga raqib haqida aytib bo'lmasin)
		if o == c || o.bot != nil || (team && o.info.Sq != c.info.Sq) || (c.spec && !o.spec) {
			continue
		}
		o.trySend(msg)
	}
}
