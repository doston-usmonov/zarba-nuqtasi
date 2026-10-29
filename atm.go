package main

// Kun vaqti va ob-havo (xona sozlamasi; o'yin holatida "at" va "wx").
//
//	at: day (kunduz), eve (kechqurun), night (tun)
//	wx: clear (ochiq), rain (yomg'ir), snow (qor), dust (chang bo'roni)
//
// Xona ochilganda tanlanadi (?time=&wx=, "rand" — tasodifiy), xona egasi o'yin davomida almashtiradi.
// Rasmiy xonalarda har o'yin tasodifiy, reytingli navbat va turnirlarda doim kunduz va ochiq havo.
// Server uchun bu faqat ko'rinish: brauzer chiroqlar, tuman va yog'inni chizadi.

import "math/rand/v2"

var (
	atmTimes = map[string]bool{"day": true, "eve": true, "night": true}
	atmWx    = map[string]bool{"clear": true, "rain": true, "snow": true, "dust": true}
)

// randomAtm: ko'pincha kunduz va ochiq havo, ba'zan boshqasi.
func randomAtm() (string, string) {
	at, wx := "day", "clear"
	switch x := rand.Float64(); {
	case x < .2:
		at = "eve"
	case x < .35:
		at = "night"
	}
	switch x := rand.Float64(); {
	case x < .15:
		wx = "rain"
	case x < .25:
		wx = "snow"
	case x < .32:
		wx = "dust"
	}
	return at, wx
}

// atmOpt: so'rovdagi qiymat ("rand" — tasodifiy); noto'g'ri bo'lsa "".
func atmOpt(at, wx string) (string, string) {
	ra, rw := randomAtm()
	if at == "rand" {
		at = ra
	} else if !atmTimes[at] {
		at = ""
	}
	if wx == "rand" {
		wx = rw
	} else if !atmWx[wx] {
		wx = ""
	}
	return at, wx
}

func (g *gameState) setAtm(at, wx string) {
	if atmTimes[at] {
		g.At = at
	}
	if atmWx[wx] {
		g.Wx = wx
	}
	if g.At == "day" {
		g.At = "" // standart — kunduz, ochiq havo (holatda yozilmaydi)
	}
	if g.Wx == "clear" {
		g.Wx = ""
	}
}
