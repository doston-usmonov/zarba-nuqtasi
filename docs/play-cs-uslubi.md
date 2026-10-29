# Topshiriq: Play-CS uslubidagi lobby va server havolalari

Namuna: play-cs.com (brauzerdagi CS 1.6). Undan o'yinni emas, **saytning lobby qismini** olamiz: har bir server uchun to'g'ridan-to'g'ri havola, xarita bo'yicha doimiy serverlar, server ro'yxati va tez kirish. Grafika, xaritalar, qurollar o'zimizniki bo'lib qoladi — Valve'ning hech qanday fayli, nomi yoki xaritasi (de_dust2 va h.k.) ishlatilmaydi.

Hozir bor narsalar (qayta yozilmaydi, ustiga quriladi): `GET /api/rooms` va bosh sahifadagi ro'yxat (`rooms.go: publicRooms`), ommaviy/yopiq xonalar, `parseRoomOpts` (map, mode, max, pub, create), `/ws?room=zn-<kod>`, ping o'lchash (`t:'ping'`/`pong`), do'stlar va partiya.

Qoidalar o'zgarmaydi: tashqi kutubxona qo'shilmaydi (faqat Go standart kutubxonasi va `lib/pq`), matnlar o'zbekcha, har bosqichga `server_test.go` da test, README va ROADMAP yangilanadi. Bosqichlar alohida commit/deploy qilinadi.

---

## 1-bosqich. Xona havolasi (deep link) — eng muhimi

Play-CS'da `/en/de_dust2/27062/ru` havolasini ochgan odam to'g'ri o'sha serverga tushadi. Bizda:

```
https://game.telegap.uz/x/<xarita>/<kod>          → shu xonaga kirish (bo'lmasa shu xaritada yaratiladi)
https://game.telegap.uz/x/<xarita>/<kod>?mode=dm  → yaratilsa, shu rejimda
```

Masalan: `/x/bekat/umumiy`, `/x/qumtepa/dostlar-42`.

Server (`main.go`):
- `/x/` bilan boshlanadigan yo'lga `index.html` ni qaytaring (SPA fallback). Xarita `validMap` dan o'tmasa yoki kod `roomName` regexiga (`zn-` prefiksi bilan) mos kelmasa — 404 emas, oddiy bosh sahifa.
- Telegram va boshqa messenjerlarda havola chiroyli chiqishi uchun `index.html` dagi `<title>` va `og:title` / `og:description` / `og:image` ni server shu so'rov uchun to'ldirib bersin: «Bekat — umumiy · 6/16 o'yinchi · Zarba Nuqtasi». Shablonni har so'rovda `strings.Replace` bilan to'ldirish yetarli (html/template shart emas, lekin qiymatlarni `html.EscapeString` qiling). `og:image` uchun har xaritaga bitta statik rasm `web/og/<xarita>.png` (1200×630).

Klient (`web/index.html`):
- Yuklanganda `location.pathname` `/x/<xarita>/<kod>` bo'lsa: ism kiritilgan bo'lsa darhol `joinOnline(false, kod)` (xarita va rejimni `map=`/`mode=` bilan uzatib), bo'lmasa ism maydoniga fokus va «Kirish» tugmasi.
- Xonaga kirgach manzil satri `history.replaceState` bilan `/x/<joriy-xarita>/<kod>` ga o'zgarsin; xona egasi xaritani almashtirsa ham yangilansin. Xonadan chiqqanda `/` ga qaytsin.
- Pauza menyusiga «Havolani ulashish» tugmasi: `navigator.share` bo'lsa u, bo'lmasa `navigator.clipboard.writeText` va «Nusxalandi» xabari. Yopiq xona havolasi ham ishlaydi — kodni bilgan kiradi, bu hozirgi mantiq bilan bir xil.

Test: `/x/bekat/umumiy` → 200 va javobda `og:title` ichida «Bekat»; noto'g'ri xarita → 200 va oddiy sarlavha; `/x/../../etc` kabi yo'llar statik fayl bermaydi.

## 2-bosqich. Doimiy («rasmiy») serverlar

Play-CS'da har xarita uchun 24/7 ishlaydigan serverlar bor, kirgan odam hech qachon bo'sh xonaga tushmaydi. Bizda xonalar faqat odam kirganda yaratiladi va ro'yxatda faqat `humans > 0` bo'lsa ko'rinadi.

- Yangi muhit o'zgaruvchisi `OFFICIAL_ROOMS`, masalan:
  `bekat:comp:16,qumtepa:dm:20,bozor:comp:10,saroy:gg:12,karvon:wm:4`
  (format: `xarita:rejim:chegara`). Kod avtomatik: `<xarita>-<rejim>` (`bekat-comp`). Standart qiymat — yuqoridagi; `-` bo'lsa o'chiq.
- Server ishga tushganda bu xonalar yaratiladi va odamlar chiqib ketsa ham o'chirilmaydi (hub'dagi bo'sh xonani tozalash mantig'iga istisno).
- Egasi yo'q: ularda xona egasi buyruqlari (`adminLocked`) ishlamaydi, ommaviylik va chegarani o'zgartirib bo'lmaydi. Xaritani faqat o'yin oxiridagi ovoz berish almashtiradi; almashtirilsa ham kod o'zgarmaydi.
- Botlar bilan to'ldirish: xonada kamida 1 odam bo'lsa, odam + bot soni `min(chegara, 10)` ga yetguncha o'rta darajadagi botlar qo'shiladi; har yangi odam kirganda bitta bot chiqadi (jamoalar muvozanatini saqlab). Odam qolmasa botlar ham o'chiriladi — bo'sh xonada CPU sarflanmasin.
- Reyting qoidasi o'zgarmaydi: bot qatnashgan o'yin reytingga yozilmaydi. Shuning uchun rasmiy xonalar asosan «tez o'ynash» uchun; reytingli o'yin — 4-bosqichdagi navbat orqali.
- `/api/rooms` javobiga `official: true` maydoni; rasmiy xonalar `humans == 0` bo'lsa ham ro'yxatda ko'rinadi.

Test: server ishga tushgach `/api/rooms` da rasmiy xonalar bor; hamma chiqqach xona qoladi, botlar yo'qoladi; rasmiy xonada `kick`/`set` buyrug'i hech narsa qilmaydi.

## 3-bosqich. Server ro'yxati (Play-CS'dagi asosiy sahifa)

Bosh sahifadagi ro'yxatni server brauzeriga aylantiring:

- Ustunlar: xarita (kichik rasm bilan), rejim, o'yinchilar `odam(+bot)/chegara`, raund va hisob, holat (isinish / o'yin / tanaffus). Rasmiy xonalar tepada, belgisi bilan.
- Filtrlar: xarita, rejim, «bo'sh joyi bor», «bo'sh xonalarni yashirish». Tanlov `SET` ga saqlansin.
- Xarita kartochkalari: har xarita uchun «hozir N kishi o'ynayapti» va «O'ynash» tugmasi (shu xaritaning eng ko'p odamli, lekin to'lmagan xonasiga kiradi). Buning uchun `/api/rooms` javobiga `byMap: {bekat: 12, ...}` qo'shing.
- Xona qatorini bosganda darhol kirish emas, kichik oyna: o'yinchilar ro'yxati (ism, klan tegi, hisob), joriy hisob va «Kirish» / «Havolani nusxalash». Yangi endpoint: `GET /api/rooms/{kod}` — ro'yxatdagi maydonlar + `players: [{name, tag, team, k, d, bot}]`. Yopiq xona uchun ham kod ma'lum bo'lsa ishlaydi.
- Ro'yxat hozirgidek 5 soniyada yangilanadi; telefonda ham (sensorli ekran) qulay bo'lsin — o'yinning telefonda ishlashi Play-CS'dan ustun tomonimiz.

Mintaqa/region tanlovi hozircha kerak emas — bitta server (Toshkent). Lekin `roomListing` ga bo'sh `region` maydonini qo'yib qo'ying, keyin ikkinchi server qo'shilsa kerak bo'ladi.

## 4-bosqich. «O'yin topish» (ROADMAP 4-bo'limdagi ochiq band)

- Bosh sahifada katta «O'yin topish» tugmasi: rejim tanlanadi (Raqobat / Wingman / Deathmatch).
- Deathmatch uchun — oddiy tez kirish: rasmiy yoki ommaviy xonalardan eng ko'p odamlisi.
- Raqobat va Wingman uchun — reytingli navbat: faqat hisobga kirganlar, reyting farqi boshida ±150, har 20 soniyada ±50 kengayadi. 10 (Wingman uchun 4) kishi yig'ilganda yangi yopiq xona (`mm-<tasodifiy>`) yaratilib, hammaga havola yuboriladi; botsiz, demak reytingga yoziladi. Partiya (do'stlar) navbatga birga turadi.
- Navbat holati server xotirasida, `/ws` emas, alohida `POST /api/queue` (kirish/chiqish) + mavjud menyu so'rovi (10 soniyalik `social` yangilanishi) orqali yoki qisqa polling.

Bu bosqich kattaroq — 1–3 bosqichlar chiqqandan keyin boshlanadi.

---

## Qilinmaydigan narsalar

- Xash3D, CS 1.6 fayllari, de_dust2 kabi nomlar, Valve ovozlari yoki modellari — hech qaysi biri repoga kirmaydi.
- Pullik «premium», tasodifiy sandiq — hozirgi siyosat (pullik narsa yo'q) o'zgarmaydi.
