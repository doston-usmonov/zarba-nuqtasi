# Zarba Nuqtasi — o'yin serveri

Brauzerdagi taktik otishma va uning onlayn serveri. Bitta Go dasturi o'yin sahifasini tarqatadi va `/ws` orqali o'yinchilarni xonalarga ulaydi. Tashqi Go kutubxonasi yo'q, faqat standart kutubxona ishlatilgan.

```
zarba-server/
├── main.go            HTTP server, sozlamalar, statik fayllar (embed)
├── hub.go             xonalar, o'yinchilar, holatni uzatish (relay)
├── match.go           o'yinni server tomonidan kuzatish (hisob, o'ldirishlar, natija)
├── rating.go          Elo reyting va darajalar
├── store.go           hisoblar, sessiyalar, o'yinlar tarixi, kunlik zaxira
├── api.go             /api: ro'yxatdan o'tish, kirish, reyting jadvali, profil
├── ws.go              WebSocket (RFC 6455) — serverga kerakli qismi
├── server_test.go     testlar (go test ./...)
├── web/index.html     o'yinning o'zi
├── web/vendor/        three.js r128 (CDN'ga bog'liq emas)
├── Dockerfile
├── docker-compose.yml
├── deploy/            nginx namunasi, deploy va avtomatik yangilanish skriptlari
└── ROADMAP.md         keyingi bosqichlar
```

## Tez ishga tushirish (lokal)

```bash
go run .                     # http://localhost:8080
# yoki
docker compose up -d --build
```

Bir xil Wi-Fi'dagi do'stlar `http://<kompyuteringiz-IP>:8080` orqali kira oladi. Buning uchun compose faylida portni `"8080:8080"` qiling.

## Serverga deploy (Docker + Nginx + HTTPS)

1. Papkani serverga ko'chiring (`scp -r zarba-server user@server:~/`).
2. Konteynerni ishga tushiring:
   ```bash
   cd ~/zarba-server
   docker compose up -d --build
   curl -s localhost:8080/healthz      # {"players":0,"rooms":0}
   ```
3. Domenning A yozuvini server IP manziliga yo'naltiring.
4. Nginx sozlamasini qo'ying va sertifikat oling:
   ```bash
   sudo cp deploy/nginx.conf /etc/nginx/sites-available/zarba
   sudo sed -i 's/oyin.example.uz/SIZNING-DOMENINGIZ/g' /etc/nginx/sites-available/zarba
   sudo ln -s /etc/nginx/sites-available/zarba /etc/nginx/sites-enabled/
   sudo certbot --nginx -d SIZNING-DOMENINGIZ
   sudo nginx -t && sudo systemctl reload nginx
   ```
5. `https://SIZNING-DOMENINGIZ` ni oching. **Onlayn o'yin** bo'limida ism va xona kodini kiriting. Do'stlaringiz o'sha havolani ochib, xuddi shu kodni kiritsa, bitta xonaga tushasiz.

HTTPS shart: `wss://` va to'liq ekran rejimi faqat xavfsiz ulanishda ishonchli ishlaydi.

## GitHub orqali yangilash

Har `main` ga push qilinganda GitHub Actions kodni tekshiradi va `ghcr.io/doston-usmonov/zarba-nuqtasi:latest` image'ini yig'adi. Serverda yangilash shunday:

```bash
cd ~/zarba-nuqtasi
git pull                       # compose va nginx sozlamalari yangilangan bo'lsa
docker compose pull && docker compose up -d
```

Package birinchi marta yig'ilgach, GitHub'da uni ochiq qiling: profil → Packages → zarba-nuqtasi → Package settings → Change visibility → Public. Yopiq qoldirsangiz, serverda `docker login ghcr.io` qilish kerak bo'ladi.

## Avtomatik deploy

`main` ga push qilinganda GitHub Actions image'ni yig'adi. Server esa har daqiqada GitHub'ni tekshiradi: yangi commit bo'lsa va uning image'i tayyor bo'lsa, `deploy/deploy.sh` ni ishga tushiradi. Skript `git pull` qiladi, yangi image'ni yuklab, konteynerni qayta ishga tushiradi va `/healthz` ni tekshiradi. Server GitHub'ga o'zi murojaat qilgani uchun SSH porti, GitHub secret'lari va kalitlar kerak emas. Push'dan keyin yangilanish taxminan 2–3 daqiqa oladi.

**Bir marta yoqish** (loyiha papkasida, papka egasi bo'lgan foydalanuvchi nomidan):

```bash
cd /var/www/zarba-nuqtasi && git pull
chmod +x deploy/*.sh && ./deploy/install-autodeploy.sh
```

Holat: `systemctl list-timers zarba-autodeploy.timer`. Jurnal: `journalctl -u zarba-autodeploy -f`. Qo'lda yangilash: `./deploy/deploy.sh`. O'chirish: `sudo systemctl disable --now zarba-autodeploy.timer`.

**Muqobil yo'l: Actions'dan SSH orqali.** Server GitHub runner'lari uchun SSH orqali ochiq bo'lsa, Actions'dagi `deploy` bosqichi push'dan so'ng darhol yangilaydi. Buning uchun repo secret'larini qo'shing: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY` va ixtiyoriy `DEPLOY_PORT`, `DEPLOY_KNOWN_HOSTS`. Serverdagi `~/.ssh/authorized_keys` ga kalitni `command="…/deploy/deploy.sh",no-pty,no-port-forwarding` cheklovi bilan qo'shing. `DEPLOY_HOST` secret'i bo'lmasa, bu bosqich o'tkazib yuboriladi.

## Hisoblar va reyting

O'yinchilar login va parol bilan ro'yxatdan o'tadi. Parol PBKDF2-SHA256 bilan (600 000 iteratsiya, tasodifiy tuz) shifrlanib saqlanadi. Sessiya 30 kun amal qiladi va `HttpOnly` cookie'da turadi. Kirishda noto'g'ri parol 10 martadan ko'p terilsa, keyingi urinishlar sekinlashtiriladi. Bitta IP manzildan 10 daqiqada ko'pi bilan bitta yangi hisob ochish mumkin, boshida esa 5 tagacha.

**Reyting** faqat onlayn o'yinlarda, hisobga kirgan o'yinchilarga yoziladi. Boshlang'ich reyting 1000. Hisoblash usuli: Elo, jamoalarning o'rtacha reytingiga qarab. Mehmonlar hisob-kitobda 1000 deb olinadi. G'alaba reytingni oshiradi, mag'lubiyat kamaytiradi, hisobdagi katta farq (masalan 8:0) o'zgarishni 1,5 baravargacha kattalashtiradi. Dastlabki 5 ta o'yin kalibrlash, shundan keyin daraja ochiladi: Oddiy askar → Askar → Kichik serjant → Serjant → Katta serjant → Leytenant → Kapitan → Mayor → Podpolkovnik → Polkovnik → General.

Natijani server o'zi hisoblaydi:
- O'yin xostning holatidan kuzatiladi.
- O'ldirish faqat **o'lgan o'yinchining o'z xabari** bilan yoziladi.
- O'yin kamida 8 g'alabagacha borishi kerak, ikkala jamoada ham o'yinchi bo'lishi shart va raundlar haddan tashqari tez tugamasligi kerak.
- Bitta hisob bilan bitta xonaga ikki marta kirib bo'lmaydi.

Zarbaning tekkani hali ham o'yinchi brauzerida hisoblanadi. Uni serverga o'tkazish ROADMAP'dagi keyingi bosqichlardan biri.

**Ma'lumotlar** `zarba-data` volume'ida (`/data`) saqlanadi:
- `snapshot.json`: hisoblar va sessiyalar, har o'zgarishda atomik qayta yoziladi.
- `matches.jsonl`: tugagan o'yinlar, faqat oxiriga qo'shiladi.
- `backups/YYYY-MM-DD/`: kunlik nusxa, oxirgi 7 kun saqlanadi.

Ma'lumotlar papkasini ochib bo'lmasa, server baribir ishga tushadi: o'yin mehmon rejimida ishlaydi, `/api` esa `503` qaytaradi.

Zaxira nusxani serverdan ko'chirib olish:
```bash
docker cp zarba-nuqtasi:/data ./zarba-data-$(date +%F)
```

API: `POST /api/register`, `POST /api/login`, `POST /api/logout`, `GET /api/me`, `GET /api/leaderboard`, `GET /api/players/{login}`.

## Sozlamalar (muhit o'zgaruvchilari)

| O'zgaruvchi | Standart | Ma'nosi |
|---|---|---|
| `ADDR` | `:8080` | Tinglanadigan manzil |
| `MAX_PLAYERS` | `16` | Bitta xonadagi o'yinchilar chegarasi |
| `ALLOWED_ORIGINS` | bo'sh | Bo'sh bo'lsa faqat o'z domeni. `*` bo'lsa hammasi. Aks holda vergul bilan ajratilgan hostlar |
| `DATA_DIR` | `data` (Docker'da `/data`) | Hisoblar, reyting va o'yinlar tarixi papkasi |

Bitta IP manzildan bir vaqtda ko'pi bilan 8 ta ulanish qabul qilinadi. Bir o'yinchidan soniyasiga 60 tadan ortiq xabar kelsa, ortiqchasi tashlanadi. Bitta xabar 8 KB dan oshmaydi.

## Qanday ishlaydi

- Har bir o'yinchi soniyasiga ~20 marta o'z holatini yuboradi: joylashuv, qarash yo'nalishi, jon, qurol, otilgan o'q va tekkan zarbalar. Server uni xonadagi qolganlarga uzatadi, yangi kelganga esa hozirgi holatni beradi.
- Raund mantig'ini (vaqt, hisob, bomba, g'olib) o'yinchilardan biri, ya'ni **xost** yuritadi. Xost tanlovi avtomatik. U chiqib ketsa, keyingisi o'sha joydan davom ettiradi.
- Zarbani otgan o'yinchining brauzeri hisoblaydi (client-side hit detection). Do'stlar orasida o'ynash uchun bu yetarli va tez. Ochiq turnir o'tkazmoqchi bo'lsangiz, firibgarlikka qarshi hisob-kitobni serverga o'tkazish kerak bo'ladi (ROADMAP, 3-bosqich).
- Server har bir xabardan faqat kerakli maydonlarni o'qiydi. Hisobga kirgan o'yinchining ismini va reytingini server o'zi yozadi, shuning uchun boshqa birovning nomi bilan o'ynab bo'lmaydi.
- Tekshiruv: `GET /healthz` hozirgi xonalar va o'yinchilar sonini qaytaradi.

## Boshqaruv

WASD — harakat, sichqoncha — nishonga olish va otish, C yoki Ctrl — cho'kkalab yurish, Shift — sekin yurish, Bo'shliq — sakrash, R — qayta o'qlash, B — xarid, E (bosib turing) — bomba o'rnatish yoki zararsizlantirish, Tab — hisob, M — jamoani almashtirish.

O'yin boshlanganda brauzer to'liq ekranga o'tadi va o'yin tugmalarini qulflaydi. Shu tufayli Ctrl+W yorliqni yopmaydi va Ctrl bilan cho'kkalash xavfsiz. Faqat o'yin tugmalari qulflanadi: Alt+Tab va Win ishlayveradi, Esc bosilsa to'liq ekrandan chiqiladi.

## O'yinni yangilash

`web/index.html` ni almashtiring va `docker compose up -d --build` ni qayta ishga tushiring. three.js `web/vendor/` ichida turadi (MIT litsenziyasi, `THREE-LICENSE.txt`).
