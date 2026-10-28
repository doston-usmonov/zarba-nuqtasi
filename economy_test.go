package main

import (
	"encoding/json"
	"testing"
	"time"
)

// TestEconomy: pul, xarid, qurollar va yerdagi qurolni faqat bir kishi olishi.
func TestEconomy(t *testing.T) {
	r, a, b := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "freeze", Md: modeComp, Fm: 12, E: "e1", Bm: -1, T: 5}
	pres := func(c *client, sq, tm string, z float64, extra map[string]any) map[string]json.RawMessage {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "pistol"}
		for k, v := range extra {
			d[k] = v
		}
		return refPres(r, c, d)
	}
	pres(a, "A", "T", 75, nil)
	pres(b, "B", "CT", 9, nil)
	if a.ps.money != 800 || a.ps.inv.sec != "pistol" {
		t.Fatalf("boshlang'ich pul/qurol: %d %+v", a.ps.money, a.ps.inv)
	}
	// avtomatga pul yetmaydi, Burgut esa sotib olinadi
	pres(a, "A", "T", 75, map[string]any{"bu": []map[string]any{{"n": 1, "id": "rifle"}, {"n": 2, "id": "deagle"}}})
	if a.ps.money != 100 || a.ps.inv.sec != "deagle" || a.ps.inv.prim != "" {
		t.Fatalf("xarid: %d %+v", a.ps.money, a.ps.inv)
	}
	denied := false
	for _, m := range drain(a) {
		if m["t"] == "eco" && m["deny"] == float64(1) {
			denied = true
		}
	}
	if !denied {
		t.Fatal("rad etilgan xarid haqida xabar yo'q")
	}
	// bazadan tashqarida xarid yo'q
	pres(b, "B", "CT", 40, nil)
	r.mu.Lock()
	b.ps.pos = vec3{43, 0, 40}
	r.mu.Unlock()
	pres(b, "B", "CT", 40, map[string]any{"bu": []map[string]any{{"n": 1, "id": "kevlar"}}})
	if b.ps.armor != 0 || b.ps.money != 800 {
		t.Fatalf("bazadan tashqarida xarid o'tdi: %v %d", b.ps.armor, b.ps.money)
	}
	// zirhni sotib olmasdan ko'rsatib bo'lmaydi
	pres(b, "B", "CT", 40, map[string]any{"ar": 100, "hm": 1})
	if b.ps.armor != 0 {
		t.Fatalf("bepul zirh: %v", b.ps.armor)
	}
	// A Burgutni tashlaydi; B ham, A ham olmoqchi — faqat birinchisi oladi
	pres(a, "A", "T", 75, map[string]any{"dr": []map[string]any{{"n": 1, "id": "a:1", "w": "deagle", "r": 1}}})
	if a.ps.inv.sec != "" || r.ground["a:1"].w != "deagle" {
		t.Fatalf("tashlash: %+v %v", a.ps.inv, r.ground)
	}
	f := pres(b, "B", "CT", 40, map[string]any{"pk": []map[string]any{{"n": 1, "id": "a:1"}}})
	if b.ps.inv.sec != "deagle" || string(f["pk"]) == "[]" {
		t.Fatalf("olish: %+v %s", b.ps.inv, f["pk"])
	}
	drain(a)
	f = pres(a, "A", "T", 75, map[string]any{"pk": []map[string]any{{"n": 1, "id": "a:1"}}})
	if a.ps.inv.sec != "" || string(f["pk"]) != "[]" {
		t.Fatalf("bitta qurolni ikki kishi oldi: %+v %s", a.ps.inv, f["pk"])
	}
	took := false
	for _, m := range drain(a) {
		if m["t"] == "eco" && m["take"] == "a:1" {
			took = true
		}
	}
	if !took {
		t.Fatal("ikkinchi olgan o'yinchidan qurol qaytarib olinmadi")
	}
	// sizda yo'q qurol bilan otilgan o'q rad etiladi
	r.mu.Lock()
	r.gs.Ph, r.gs.liveAt = "live", time.Now()
	a.ps.pos, b.ps.pos = vec3{43, 0, 40}, vec3{43, 0, 30}
	a.ps.push(sample{t: time.Now(), p: a.ps.pos})
	b.ps.push(sample{t: time.Now(), p: b.ps.pos})
	r.mu.Unlock()
	f = pres(a, "A", "T", 40, map[string]any{"hi": []map[string]any{{"i": 1, "t": "b", "d": 30, "pn": .5, "p": "c", "w": "rifle"}}})
	if string(f["hi"]) != "null" {
		t.Fatal("sotib olinmagan avtomatdan o'q o'tdi")
	}
	f = pres(b, "B", "CT", 30, map[string]any{"hi": []map[string]any{{"i": 1, "t": "a", "d": 50, "pn": .9, "p": "c", "w": "deagle"}}})
	if string(f["hi"]) == "null" {
		t.Fatal("yerdan olingan Burgutdan o'q o'tmadi")
	}
	// raund oxiri: g'olib 3250, yutqazgan 1400; o'ldirish 300
	pres(a, "A", "T", 40, map[string]any{"al": 0, "de": map[string]any{"n": 1, "by": "b", "w": "deagle", "hs": 0}})
	r.mu.Lock()
	moneyB := b.ps.money
	r.tickLocked(time.Now(), .05)
	if r.gs.Ph != "post" || b.ps.money != moneyB+3250 || a.ps.money != 100+1400 {
		t.Fatalf("raund puli: ph=%s a=%d b=%d (avval %d)", r.gs.Ph, a.ps.money, b.ps.money, moneyB)
	}
	if moneyB != 800+300 {
		t.Fatalf("o'ldirish mukofoti: %d", moneyB)
	}
	r.mu.Unlock()
}

// TestServerGrenade: portlovchi granata zararini server o'zi qo'llaydi.
func TestServerGrenade(t *testing.T) {
	r, a, b := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "live", Md: modeComp, Fm: 12, E: "g1", Bm: -1, T: 100, liveAt: time.Now()}
	for _, x := range []struct {
		c      *client
		sq, tm string
		z      float64
	}{{a, "A", "T", 75}, {b, "B", "CT", 9}} {
		d := map[string]any{"v": 1, "hc": 1, "sq": x.sq, "tm": x.tm, "in": 1, "al": 1, "x": 43, "y": 0, "z": x.z}
		refPres(r, x.c, d)
	}
	r.mu.Lock()
	a.ps.pos, b.ps.pos = vec3{43, 0, 44}, vec3{43, 0, 30}
	a.ps.push(sample{t: time.Now(), p: a.ps.pos})
	b.ps.push(sample{t: time.Now(), p: b.ps.pos})
	a.ps.inv.nades["he"] = 1
	r.mu.Unlock()
	// A o'rta yo'lak bo'ylab B tomonga otadi
	refPres(r, a, map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 44,
		"gn": map[string]any{"n": 1, "k": "he", "x": 43, "y": 1.55, "z": 43.7, "vx": 0, "vy": 2, "vz": -8.5}})
	if len(r.nades) != 1 || a.ps.inv.nades["he"] != 0 {
		t.Fatalf("granata hisoblanmadi: %d %v", len(r.nades), a.ps.inv.nades)
	}
	land := r.nades[0].p
	r.mu.Lock()
	b.ps.pos = vec3{land.X, 0, land.Z + 1}
	b.ps.push(sample{t: time.Now(), p: b.ps.pos})
	r.tickLocked(time.Now().Add(1700*time.Millisecond), .05)
	r.mu.Unlock()
	if b.ps.hp > 40 || len(r.nades) != 0 {
		t.Fatalf("granata zarari: hp=%v (tushdi %.1f,%.1f)", b.ps.hp, land.X, land.Z)
	}
	// ikkinchi granata yo'q — otilgani boshqalarga yuborilmaydi
	f := refPres(r, a, map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 44,
		"gn": map[string]any{"n": 2, "k": "he", "x": 43, "y": 1.55, "z": 43.7, "vx": 0, "vy": 2, "vz": -8.5}})
	if _, ok := f["gn"]; ok || len(r.nades) != 0 {
		t.Fatal("sotib olinmagan granata otildi")
	}
}

// TestPurseReconnect: aloqa uzilib qayta ulangan o'yinchining puli va quroli saqlanadi.
func TestPurseReconnect(t *testing.T) {
	r, a, _ := newRefRoom()
	a.ip = "1.2.3.4"
	*r.gs = gameState{R: 3, Ph: "live", Md: modeComp, Fm: 12, E: "p1", Bm: -1, T: 80, liveAt: time.Now()}
	r.mu.Lock()
	a.info = presInfo{Sq: "A", Tm: "T", Nm: "Ali", In: 3}
	a.ps.ecoE, a.ps.money, a.ps.alive, a.ps.life, a.ps.hp = "p1", 4200, true, 3, 37
	a.ps.inv.prim = "rifle"
	r.savePurseLocked(a)
	delete(r.clients, a.id)
	a2 := &client{id: "a2", room: r, ip: "1.2.3.4", send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
	r.clients["a2"] = a2
	r.mu.Unlock()
	refPres(r, a2, map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "nm": "Ali", "in": 3, "al": 1, "x": 43, "y": 0, "z": 75})
	if a2.ps.money != 4200 || a2.ps.inv.prim != "rifle" || a2.ps.hp != 37 {
		t.Fatalf("qayta ulanish: pul=%d qurol=%q jon=%v", a2.ps.money, a2.ps.inv.prim, a2.ps.hp)
	}
}

// TestSellBack: shu raundda sotib olingan narsa xarid vaqtida to'liq narxiga qaytariladi.
func TestSellBack(t *testing.T) {
	r, a, _ := newRefRoom()
	*r.gs = gameState{R: 1, Ph: "freeze", Md: modeComp, Fm: 12, E: "e1", Bm: -1, T: 5}
	n := 0
	pres := func(extra map[string]any) {
		d := map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 1, "al": 1, "x": 43, "y": 0, "z": 75, "w": "pistol"}
		for k, v := range extra {
			d[k] = v
		}
		refPres(r, a, d)
	}
	ev := func(id string, sell bool) map[string]any {
		n++
		m := map[string]any{"n": n, "id": id}
		if sell {
			m["s"] = 1
		}
		return m
	}
	pres(nil)
	pres(map[string]any{"bu": []map[string]any{ev("deagle", false), ev("smoke", false), ev("taser", false)}})
	if a.ps.inv.sec != "deagle" || a.ps.money != 100 {
		t.Fatalf("xarid: %d %+v", a.ps.money, a.ps.inv)
	}
	pres(map[string]any{"bu": []map[string]any{ev("deagle", true)}})
	if a.ps.inv.sec != "" || a.ps.money != 800 {
		t.Fatalf("Burgut qaytarildi: %d %+v", a.ps.money, a.ps.inv)
	}
	pres(map[string]any{"bu": []map[string]any{ev("deagle", true)}}) // ikkinchi marta — yo'q
	if a.ps.money != 800 {
		t.Fatalf("ikki marta qaytarildi: %d", a.ps.money)
	}
	// uchqun va tutun
	pres(map[string]any{"bu": []map[string]any{ev("taser", false), ev("smoke", false)}})
	if !a.ps.inv.taser || a.ps.inv.nades["smoke"] != 1 || a.ps.money != 300 {
		t.Fatalf("uchqun+tutun: %v %v %d", a.ps.inv.taser, a.ps.inv.nades, a.ps.money)
	}
	pres(map[string]any{"bu": []map[string]any{ev("taser", true), ev("smoke", true)}})
	if a.ps.inv.taser || a.ps.inv.nades["smoke"] != 0 || a.ps.money != 800 {
		t.Fatalf("qaytarish: %v %v %d", a.ps.inv.taser, a.ps.inv.nades, a.ps.money)
	}
	// zirh: shikastlanmagan bo'lsa qaytadi, oldingi holatga
	pres(map[string]any{"bu": []map[string]any{ev("kevlar", false)}, "ar": 100})
	if a.ps.armor != 100 {
		t.Fatalf("zirh: %v", a.ps.armor)
	}
	pres(map[string]any{"bu": []map[string]any{ev("kevlar", true)}, "ar": 100})
	if a.ps.armor != 0 || a.ps.money != 800 {
		t.Fatalf("zirh qaytarildi: %v %d", a.ps.armor, a.ps.money)
	}
	pres(map[string]any{"bu": []map[string]any{ev("kevlar", false)}, "ar": 100})
	pres(map[string]any{"bu": []map[string]any{ev("kevlar", true)}, "ar": 92}) // o'q tekkan
	if a.ps.money != 150 {
		t.Fatalf("shikastlangan zirh qaytarilmasligi kerak: %d", a.ps.money)
	}
	// tashlangan qurol qaytarilmaydi; boshlang'ich to'pponcha ham (sotib olinmagan)
	r.mu.Lock()
	a.ps.money = 800
	r.mu.Unlock()
	pres(map[string]any{"bu": []map[string]any{ev("mpistol", false)}})
	pres(map[string]any{"dr": []map[string]any{{"n": 1, "id": "a:1", "w": "mpistol", "r": 1}}})
	pres(map[string]any{"bu": []map[string]any{ev("mpistol", true), ev("pistol", true)}})
	if a.ps.money != 300 {
		t.Fatalf("tashlangan qurol yoki sotib olinmagan to'pponcha qaytarildi: %d", a.ps.money)
	}
	// xarid vaqti tugagach — yo'q
	pres(map[string]any{"bu": []map[string]any{ev("flash", false)}})
	r.mu.Lock()
	r.gs.Ph, r.gs.liveAt = "live", time.Now().Add(-time.Minute)
	r.mu.Unlock()
	pres(map[string]any{"bu": []map[string]any{ev("flash", true)}})
	if a.ps.money != 100 || a.ps.inv.nades["flash"] != 1 {
		t.Fatalf("xarid vaqtidan keyin qaytarildi: %d %v", a.ps.money, a.ps.inv.nades)
	}
}

// TestBotDropsGun: raundli rejimda o'lgan botning quroli yerda qoladi va odam uni ola oladi.
func TestBotDropsGun(t *testing.T) {
	r, a, _ := newRefRoom()
	*r.gs = gameState{R: 2, Ph: "live", Md: modeComp, Fm: 12, E: "e1", Bm: -1, T: 90}
	r.gs.liveAt = time.Now()
	r.mu.Lock()
	bot := r.addBotLocked("B", "normal", time.Now())
	bot.bot.weapon, bot.bot.mag = "rifle", 17
	bot.ps.alive = true
	r.botDieLocked(bot, nil, "rifle", false, time.Now())
	r.publishBotLocked(bot, time.Now())
	var pres struct {
		Dr []struct {
			ID string `json:"id"`
			W  string `json:"w"`
			Mg int    `json:"mg"`
			R  int    `json:"r"`
		} `json:"dr"`
	}
	json.Unmarshal(bot.pres, &pres)
	r.mu.Unlock()
	if len(pres.Dr) != 1 || pres.Dr[0].W != "rifle" || pres.Dr[0].Mg != 17 || pres.Dr[0].R != 2 || r.ground[pres.Dr[0].ID].w != "rifle" {
		t.Fatalf("botning quroli: %+v %v", pres.Dr, r.ground)
	}
	// odam uni oladi
	refPres(r, a, map[string]any{"v": 1, "hc": 1, "sq": "A", "tm": "T", "in": 2, "al": 1, "x": 43, "y": 0, "z": 75, "w": "pistol",
		"pk": []map[string]any{{"n": 1, "id": pres.Dr[0].ID}}})
	if a.ps.inv.prim != "rifle" {
		t.Fatalf("odam botning qurolini ololmadi: %+v", a.ps.inv)
	}
	// qayta tug'iladigan rejimda qurol tashlanmaydi
	r.mu.Lock()
	r.gs.Md = modeDM
	b2 := r.addBotLocked("A", "normal", time.Now())
	b2.bot.weapon, b2.ps.alive = "smg", true
	r.botDieLocked(b2, nil, "smg", false, time.Now())
	r.mu.Unlock()
	if len(b2.bot.drops) != 0 {
		t.Fatal("deathmatch'da bot qurol tashlamasligi kerak")
	}
}

// TestGiveMoney: jamoadoshga pul — faqat o'z jamoasiga, xarid vaqtida, bazada, pul yetsa va $16 000 dan oshmasa.
func TestGiveMoney(t *testing.T) {
	r, a, b := newRefRoom()
	c := &client{id: "c", room: r, send: make(chan []byte, 64), ps: newPState(), joined: time.Now()}
	r.clients["c"] = c
	*r.gs = gameState{R: 1, Ph: "freeze", Md: modeComp, Fm: 12, E: "e1", Bm: -1, T: 5}
	pres := func(cl *client, sq, tm string, z float64, extra map[string]any) {
		d := map[string]any{"v": 1, "hc": 1, "sq": sq, "tm": tm, "in": 1, "al": 1, "x": 43, "y": 0, "z": z, "w": "pistol", "nm": cl.id}
		for k, v := range extra {
			d[k] = v
		}
		refPres(r, cl, d)
	}
	pres(a, "A", "T", 75, nil)
	pres(b, "A", "T", 75, nil)
	pres(c, "B", "CT", 9, nil)
	give := func(n int, to string, amt int) {
		pres(a, "A", "T", 75, map[string]any{"bu": []map[string]any{{"n": n, "id": "$", "s": 2, "t": to, "a": amt}}})
	}
	drain(a)
	drain(b)
	give(1, "b", 500)
	if a.ps.money != 300 || b.ps.money != 1300 {
		t.Fatalf("pul berilmadi: %d %d", a.ps.money, b.ps.money)
	}
	var gift bool
	for _, m := range drain(b) {
		if g, _ := m["gift"].(map[string]any); m["t"] == "eco" && g != nil && g["a"] == float64(500) && g["from"] == "a" && m["m"] == float64(1300) {
			gift = true
		}
	}
	if !gift {
		t.Fatal("oluvchiga xabar kelmadi")
	}
	denied := func(n int) bool {
		for _, m := range drain(a) {
			if m["t"] == "eco" && m["deny"] == float64(n) && m["id"] == "$" {
				return true
			}
		}
		return false
	}
	give(2, "c", 100) // raqibga
	if c.ps.money != 800 || !denied(2) {
		t.Fatal("raqibga pul berildi")
	}
	give(3, "b", 400) // pul yetmaydi
	if a.ps.money != 300 || !denied(3) {
		t.Fatal("pul yetmasa ham berildi")
	}
	give(4, "b", 150) // 100 ga karrali emas
	if !denied(4) {
		t.Fatal("noto'g'ri miqdor o'tdi")
	}
	b.ps.money = 15900
	give(5, "b", 200)
	if b.ps.money != 15900 || !denied(5) {
		t.Fatal("$16 000 dan oshib ketdi")
	}
	b.ps.money = 1000
	r.gs.Ph, r.gs.liveAt = "live", time.Now().Add(-30*time.Second) // xarid vaqti tugagan
	give(6, "b", 100)
	if b.ps.money != 1000 || !denied(6) {
		t.Fatal("xarid vaqtidan keyin berildi")
	}
	give(6, "b", 100) // takror raqam e'tiborga olinmaydi
	if a.ps.money != 300 {
		t.Fatal("takror hodisa")
	}
}
