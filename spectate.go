package main

// Kuzatuvchilar: xonaga «?spec=1» bilan kiriladi (server brauzeridagi «Kuzatish» tugmasi).
// Kuzatuvchi o'ynamaydi va o'yinchilarga ko'rinmaydi: uning holati yuborilmaydi, jamoasi yo'q,
// o'yinchilar chegarasiga kirmaydi (xona to'la bo'lsa ham kuzatish mumkin, ko'pi bilan
// maxSpectators ta). Ovoz berish, xona buyruqlari va ovozli chat yo'q; chatdagi yozuvi faqat
// boshqa kuzatuvchilarga boradi (o'yinchilarga raqib qayerdaligini aytib bo'lmasin). Reytingli
// navbat o'yinini kuzatib bo'lmaydi. O'yinchisi qolmagan xona yopiladi va kuzatuvchilar chiqadi.

import "time"

const maxSpectators = 16

func (r *room) specCountLocked() int {
	n := 0
	for _, c := range r.clients {
		if c.spec {
			n++
		}
	}
	return n
}

// closeSpectatorsLocked: xona yopilyapti — kuzatuvchilarga xabar va ulanishni uzish.
func (r *room) closeSpectatorsLocked() {
	for _, c := range r.clients {
		if c.spec {
			c.trySend(jsonMsg(map[string]string{"t": "err", "code": "room_closed"}))
			go func(c *client) { // xabar yetib borsin
				time.Sleep(300 * time.Millisecond)
				c.ws.close()
			}(c)
		}
	}
}
