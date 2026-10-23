# Zarba Nuqtasi — o'yin serveri

Brauzerdagi taktik otishma va uning onlayn serveri. Bitta Go dasturi o'yin sahifasini tarqatadi, `/ws` orqali o'yinchilarni xonalarga ulaydi, hisoblar va reytingni PostgreSQL'da saqlaydi. Yagona tashqi kutubxona — PostgreSQL drayveri `github.com/lib/pq`, uning o'zi ham boshqa kutubxonalarga bog'liq emas.

```
zarba-server/
├── main.go            HTTP server, sozlamalar, statik fayllar (embed)
├── hub.go             xonalar, o'yinchilar, holatni uzatish (relay)
├── match.go           o'yinni server tomonidan kuzatish (hisob, o'ldirishlar, natija)
├── rating.go          Elo reyting va darajalar
├── store.go           PostgreSQL: hisoblar, sessiyalar, o'yinlar tarixi, migratsiyalar
├── migrations/        SQL sxema (server ishga tushganda avtomatik qo'llanadi)
├── legacy.go          oldingi fayl bazasidan bir martalik import
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
# hammasi Docker'da (o'yin + PostgreSQL + zaxira):
echo "ZARBA_DB_PASSWORD=$(openssl rand -hex 24)" > .env
docker compose up -d --build

# yoki o'yinni Go bilan, bazani Docker'da:
docker run -d --name zarba-pg -e POSTGRES_USER=zarba -e POSTGRES_PASSWORD=zarba -e POSTGRES_DB=zarba -p 5432:5432 postgres:17-alpine
DATABASE_URL="postgres://zarba:zarba@localhost:5432/zarba?sslmode=disable" go run .   # http://localhost:8080

# testlar (alohida test bazasi; har test uni tozalaydi):
ZARBA_TEST_DATABASE_URL="postgres://zarba:zarba@localhost:5432/zarba_test?sslmode=disable" go test -race ./...
```

`DATABASE_URL` berilmasa ham o'yin ishlaydi, faqat hisoblar va reyting o'chiq bo'ladi.

Bir xil Wi-Fi'dagi do'stlar `http://<kompyuteringiz-IP>:8080` orqali kira oladi. Buning uchun compose faylida portni `"8080:8080"` qiling.

## Serverga deploy (Docker + Nginx + HTTPS)

1. Papkani serverga ko'chiring (`scp -r zarba-server user@server:~/`).
2. Konteynerni ishga tushiring:
   ```bash
   cd ~/zarba-server
   ./deploy/deploy.sh                  # .env (baza paroli) ni yaratadi va hammasini ishga tushiradi
   curl -s localhost:8080/healthz      # {"db":"ok","players":0,"rooms":0}
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

### Ma'lumotlar bazasi (PostgreSQL 17)

Jadvallar: `users`, `sessions` (tokenning o'zi emas, faqat sha256 xeshi saqlanadi), `matches`, `match_players`. Sxema `migrations/NNN_*.sql` fayllarida. Server ishga tushganda yangi migratsiyalar avtomatik qo'llanadi: har biri o'z tranzaksiyasida, `pg_advisory_lock` bilan himoyalangan. O'yin natijasi va barcha o'yinchilarning reytingi bitta tranzaksiyada yoziladi.

- **Baza paroli:** `.env` faylidagi `ZARBA_DB_PASSWORD`. Fayl git'ga tushmaydi. `deploy/deploy.sh` uni birinchi marta o'zi yaratadi. **`.env` ni o'chirmang:** baza shu parol bilan yaratilgan.
- **Port:** tashqariga ochilmaydi, bazaga faqat compose ichidagi xizmatlar ulanadi.
- **Zaxira:** `db-backup` xizmati har kuni `pg_dump` oladi va `zarba-backups` volume'ida 7 kunlik nusxani saqlaydi:
  ```bash
  docker compose exec db-backup ls -la /backups                                       # ro'yxat
  docker compose cp db-backup:/backups/zarba-$(date +%F).sql.gz .                      # ko'chirib olish
  gunzip -c zarba-2026-10-23.sql.gz | docker compose exec -T db psql -U zarba zarba    # tiklash (bo'sh bazaga)
  ```
- **Bazaga qo'lda kirish:** `docker compose exec db psql -U zarba zarba`.
- **Baza ishlamasa:** o'yin baribir ishlaydi (mehmon rejimida), `/api` `503` qaytaradi va server bazaga qayta ulanishga urinib turadi. Holatini `/healthz` dagi `db` maydoni ko'rsatadi.
- **Oldingi versiyadan ko'chish:** baza bo'sh bo'lsa, server `/data/snapshot.json` va `matches.jsonl` ni (fayl bazasini) avtomatik import qiladi. Hisoblar, parollar, sessiyalar, reyting va tarix saqlanib qoladi, fayllar `*.imported-<sana>` deb qayta nomlanadi.

API: `POST /api/register`, `POST /api/login`, `POST /api/logout`, `GET /api/me`, `GET /api/leaderboard`, `GET /api/players/{login}`.

## Sozlamalar (muhit o'zgaruvchilari)

| O'zgaruvchi | Standart | Ma'nosi |
|---|---|---|
| `ADDR` | `:8080` | Tinglanadigan manzil |
| `MAX_PLAYERS` | `16` | Bitta xonadagi o'yinchilar chegarasi |
| `ALLOWED_ORIGINS` | bo'sh | Bo'sh bo'lsa faqat o'z domeni. `*` bo'lsa hammasi. Aks holda vergul bilan ajratilgan hostlar |
| `DATABASE_URL` | bo'sh | PostgreSQL manzili. Bo'sh bo'lsa hisoblar o'chiq |
| `DATA_DIR` | bo'sh (Docker'da `/data`) | Oldingi fayl bazasi papkasi (faqat bir martalik import uchun) |
| `STUN_URLS` | `stun:stun.l.google.com:19302` | Ovozli chat uchun STUN serverlar (vergul bilan). `-` — o'chirish |
| `TURN_SECRET` | bo'sh (Docker'da `.env` dan) | coturn bilan bir xil kalit. Bo'sh bo'lsa TURN ishlatilmaydi |
| `TURN_HOST` | bo'sh | TURN domeni. Bo'sh bo'lsa o'yin ochilgan domen olinadi |
| `TURN_PORT` | `3478` | TURN porti |

Bitta IP manzildan bir vaqtda ko'pi bilan 8 ta ulanish qabul qilinadi. Bir o'yinchidan soniyasiga 60 tadan ortiq xabar kelsa, ortiqchasi tashlanadi. Bitta xabar 8 KB dan oshmaydi.

## Ovozli chat

Onlayn xonada o'yinchilar bir-biri bilan gaplashadi (faqat o'yinning o'z saytida; Claude ichidagi versiyada o'chiq). Sozlamalarda tanlanadi:

- **Ovozli chat:** O'chiq, Jamoa (faqat jamoadoshlar eshitadi) yoki Hamma (raqiblar bilan ham, agar ular ham «Hamma»ni tanlagan bo'lsa).
- **Mikrofon:** K tugmasini bosib gapirish (telefonda mikrofon tugmasi) yoki doim ochiq.
- **Suhbat ovozi:** boshqalarning ovozi qanchalik baland eshitilishi.

Gapirayotganlar radar ostida ko'rinadi. Ovoz brauzerlar o'rtasida to'g'ridan-to'g'ri (WebRTC) yuboriladi. Server faqat ulanish xabarlarini uzatadi va ovozni yozmaydi. Mikrofon faqat eshitishga ruxsati bor o'yinchilarga yuboriladi, shuning uchun raqib brauzerini o'zgartirib ham jamoa suhbatini eshita olmaydi.

Ba'zi tarmoqlarda (mobil internet, ofis NAT) brauzerlar to'g'ridan-to'g'ri ulana olmaydi. Bunda ovoz `docker-compose.yml` dagi `turn` xizmati (coturn) orqali o'tadi. Uning kaliti `.env` dagi `ZARBA_TURN_SECRET`, uni `deploy/deploy.sh` o'zi yaratadi. Firewall'da quyidagilarni oching:

```bash
sudo ufw allow 3478/udp
sudo ufw allow 3478/tcp
sudo ufw allow 49160:49200/udp
```

Hosting provayderida alohida firewall bo'lsa (masalan, bulut paneli), o'sha portlarni u yerda ham oching. Server ichki IP orqasida bo'lsa (NAT), `turn` xizmati buyrug'iga `--external-ip=<tashqi IP>` qo'shing.

## Qanday ishlaydi

- Har bir o'yinchi soniyasiga ~20 marta o'z holatini yuboradi: joylashuv, qarash yo'nalishi, jon, qurol, otilgan o'q va tekkan zarbalar. Server uni xonadagi qolganlarga uzatadi, yangi kelganga esa hozirgi holatni beradi.
- Raund mantig'ini (vaqt, hisob, bomba, g'olib) o'yinchilardan biri, ya'ni **xost** yuritadi. Xost tanlovi avtomatik. U chiqib ketsa, keyingisi o'sha joydan davom ettiradi.
- Zarbani otgan o'yinchining brauzeri hisoblaydi (client-side hit detection). Do'stlar orasida o'ynash uchun bu yetarli va tez. Ochiq turnir o'tkazmoqchi bo'lsangiz, firibgarlikka qarshi hisob-kitobni serverga o'tkazish kerak bo'ladi (ROADMAP, 3-bosqich).
- Server har bir xabardan faqat kerakli maydonlarni o'qiydi. Hisobga kirgan o'yinchining ismini va reytingini server o'zi yozadi, shuning uchun boshqa birovning nomi bilan o'ynab bo'lmaydi.
- Tekshiruv: `GET /healthz` hozirgi xonalar va o'yinchilar sonini qaytaradi.

## Boshqaruv

WASD — harakat, sichqoncha — nishonga olish va otish, C yoki Ctrl — cho'kkalab yurish, Shift — sekin yurish, Bo'shliq — sakrash, R — qayta o'qlash, B — xarid, E (bosib turing) — bomba o'rnatish yoki zararsizlantirish, Tab — hisob, M — jamoani almashtirish, K (bosib turing) — ovozli chatda gapirish.

Granatalar 4-slotda: 4 tugmasi granatani oladi, yana bosilsa keyingi turiga o'tadi. Chap tugma — uzoqqa, o'ng tugma — yaqinga otish; granata tugma qo'yib yuborilganda uchadi. Turlari: portlovchi ($300), ko'r qiluvchi ($200, ikkitagacha), tutun ($300, 18 soniya ko'rinishni to'sadi), yondiruvchi ($400, 7 soniya yonadi, tutun uni o'chiradi). Bir o'yinchida ko'pi bilan 4 ta granata bo'ladi. Himoyachilar zararsizlantirish to'plamini ($400) olsa, bomba 7 emas, 4 soniyada zararsizlanadi.

O'yin CS kabi ikki yarimdan iborat. Sozlamalarda uzunlikni tanlaysiz: to'liq (MR12, 13 g'alabagacha, 12 raunddan keyin tomonlar almashadi) yoki qisqa (9 g'alabagacha, 8 raunddan keyin almashadi). Hisob teng bo'lsa (12:12 yoki 8:8), qo'shimcha vaqt boshlanadi: 6 raund, 4 g'alaba kerak, har biriga $12 500 beriladi, 3 raunddan keyin tomonlar almashadi. Botlarga qarshi ikkinchi yarimda siz himoyada bo'lasiz: botlar bombani olib, A yoki B nuqtani egallaydi va o'rnatadi, siz esa E bilan zararsizlantirasiz. Onlayn xonada formatni xost (birinchi kirgan o'yinchi) sozlamasi belgilaydi.

Onlayn o'yinda granata otilganda faqat uning boshlang'ich holati yuboriladi, uchishini har bir brauzer bir xil qadam bilan o'zi hisoblaydi, shuning uchun tutun hammada bir joyga tushadi. Har bir o'yinchi o'ziga tekkan zarar va ko'r bo'lishni o'zi qo'llaydi. Jamoadoshning granatasi zarar bermaydi, o'zingizniki esa beradi.

O'yin boshlanganda brauzer to'liq ekranga o'tadi va o'yin tugmalarini qulflaydi. Shu tufayli Ctrl+W yorliqni yopmaydi va Ctrl bilan cho'kkalash xavfsiz. Faqat o'yin tugmalari qulflanadi: Alt+Tab va Win ishlayveradi, Esc bosilsa to'liq ekrandan chiqiladi.

## O'yinni yangilash

`web/index.html` ni almashtiring va `docker compose up -d --build` ni qayta ishga tushiring. three.js `web/vendor/` ichida turadi (MIT litsenziyasi, `THREE-LICENSE.txt`).
