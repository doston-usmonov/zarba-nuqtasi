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
├── economy.go         pul, xaridlar va qo'ldagi qurollar (raqobat rejimi)
├── nadesim.go         granatalar va bomba portlashi serverda
├── progress.go        XP, darajalar, kunlik vazifalar, qurol bo'yoqlari (/api/progress, /api/loadout)
├── medals.go          batafsil statistika (qurollar, xaritalar, rejimlar) va medallar
├── stats.go           o'yin statistikasi: raund MVP va yakuniy jadval
├── social.go          do'stlar, xonaga chaqiriqlar, klanlar (/api/social, /api/friends, /api/clan, /api/clans)
├── queue.go           «O'yin topish»: reytingli navbat, partiyalar, mm- xonalar (/api/queue)
├── leaver.go          reytingli o'yinni tashlab ketganlik uchun navbat taqiqi, qayta ulanish
├── botnades.go        botlarning granatalari: tayyor tutun joylari, ko'r qiluvchi, portlovchi, yondiruvchi
├── officials.go       doimiy (rasmiy) xonalar va ularni botlar bilan to'ldirish
├── deeplink.go        /x/<xarita>/<kod> xona havolalari va og: teglari
├── modes.go           o'yin rejimlari: deathmatch (hamma hammaga yoki jamoaviy), qurol poygasi, Wingman, Retake
├── gamemap.go         xaritalar panjarasi: ko'rinish, devorlar, bazalar, nuqtalar
├── mapsdata.go        xaritalar (web/index.html dagi MAPS dan tools/genmaps.py yasaydi)
├── server_test.go     testlar (go test ./...)
├── web/index.html     o'yinning o'zi
├── web/vo/           o'zbekcha diktor ovozlari (ElevenLabs bilan yozilgan, 13 ta qisqa mp3)
├── web/models/        askarlar va qurollar 3D modellari (.glb, Blender'da yasalgan: tools/soldier.py, tools/weapons.py)
├── web/vendor/        three.js r128, GLTFLoader va SkeletonUtils (CDN'ga bog'liq emas)
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
- Raundni server o'zi yuritadi (pastdagi "Server hakamligi" bo'limi).
- O'ldirish faqat server tasdiqlagan o'q tegishidan keyin (yoki o'ldirgan o'yinchi shu paytda granata otgan bo'lsa) yoziladi.
- Natija format qoidasiga mos bo'lishi kerak (9 yoki 13 g'alaba, overtime). Ikkala jamoada ham o'yinchi bo'lishi shart va raundlar haddan tashqari tez tugamasligi kerak.
- Bitta hisob bilan bitta xonaga ikki marta kirib bo'lmaydi.

O'q qayerga tekkanini otgan o'yinchining brauzeri aniqlaydi, server esa har bir tegishni tekshiradi (zarar, otish tezligi, devor, qurol rostdan qo'lida bormi). Granata va bomba zararini, pul va xaridlarni server o'zi hisoblaydi.

### Ma'lumotlar bazasi (PostgreSQL 17)

Jadvallar: `users`, `sessions` (tokenning o'zi emas, faqat sha256 xeshi saqlanadi), `matches`, `match_players`, `daily_progress`, `friends`, `clans`, `clan_members`, `clan_invites`, `leaver_bans`, `player_stats`, `achievements`. Sxema `migrations/NNN_*.sql` fayllarida. Server ishga tushganda yangi migratsiyalar avtomatik qo'llanadi: har biri o'z tranzaksiyasida, `pg_advisory_lock` bilan himoyalangan. O'yin natijasi va barcha o'yinchilarning reytingi bitta tranzaksiyada yoziladi.

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

API: `POST /api/register`, `POST /api/login`, `POST /api/logout`, `GET /api/me`, `GET /api/leaderboard`, `GET /api/players/{login}`, `GET /api/progress`, `POST /api/loadout`, `GET /api/social`, `POST /api/friends`, `POST /api/clan`, `GET /api/clans`.

### Medallar va batafsil statistika

Har onlayn o'yin oxirida (XP bilan birga, hisobga kirganlarga) server tasdiqlagan natijalar jamlanadi (`medals.go`, `player_stats` jadvali): o'ldirish, o'lim, boshga, zarar, MVP, g'alaba, bomba o'rnatish va zararsizlantirish; qurollar bo'yicha (o'ldirish, boshga, zarar), xaritalar va rejimlar bo'yicha (o'ynagan, yutgan); eng yaxshilari — bitta raunddagi va bitta o'yindagi eng ko'p o'ldirish, yutilgan eng og'ir «1 ga N» (jamoada yolg'iz qolib 2 va undan ko'p raqibga qarshi raundni yutish).

**18 ta medal** (o'zimizniki): Birinchi qadam, Beshlik (bir raundda 5 ta), Yolg'iz qahramon (1 ga 3), Ayovsiz (bir o'yinda 30 ta), Pichoq ustasi, Uchqun, Qirg'iy ko'zi, Aniq nishon, Portlatuvchi, Olovli, Sapyor, Qutqaruvchi, Yulduz, G'olib, Faxriy, Mingtalik, Poyga sardori, Aim ustasi. Shart bajarilgan zahoti medal ochiladi (`achievements`), qo'shimcha XP beradi va o'yin oxiridagi oynada «Yangi medal» deb chiqadi.

Profilda (reyting jadvalida istalgan o'yinchi) uch bo'lim: «O'yinlar» (so'nggi reytingli o'yinlar), «Medallar» (ochilganlari sanasi bilan, qolganlari qancha qolgani bilan), «Statistika» (jami, qurollar, xaritalar, rejimlar). `GET /api/players/{login}` → `detail`.

### Do'stlar, partiya va klanlar

- **Do'stlar.** Hisob panelidagi «Do'stlar va klan» tugmasi. Do'stni login bo'yicha qo'shasiz, u qabul qilgach ro'yxatda holati ko'rinadi: oflayn, menyuda yoki qaysi xarita va rejimda o'ynayapti. O'ynayotgan do'st xonasiga «Qo'shilish» bilan kirasiz. Menyu har 10 soniyada yangilanadi.
- **Partiya.** O'yin ichida pauza menyusida (Esc) onlayn do'stlar ro'yxati chiqadi, «Chaqirish» bosilsa do'stga chaqiriq boradi: menyuda bo'lsa pastda xabar, boshqa xonada o'ynayotgan bo'lsa o'yin ichida. Chaqiriq 2 daqiqa amal qiladi. Qabul qilgan do'st sizning jamoangizga tushadi.
- **Klanlar.** Klan nomi 3–24 belgi, tegi 2–5 ta lotin harfi yoki raqam (katta harfga o'tkaziladi, nom ham, teg ham takrorlanmaydi). Bir o'yinchi faqat bitta klanda bo'ladi, klanda 30 tagacha a'zo. A'zolar klansiz do'stlarini taklif qiladi, egasi a'zoni chiqaradi. Egasi ketsa egalik eng eski a'zoga o'tadi, oxirgi a'zo ketsa klan o'chadi. Klan tegi o'yinda ism ustida va hisob jadvalida ko'rinadi. Klanlar reytingi a'zolarning o'rtacha reytingi bo'yicha.
- Jadvallar: `friends`, `clans`, `clan_members`, `clan_invites` (`migrations/003_social.sql`). Kim qaysi xonada ekani va chaqiriqlar server xotirasida turadi.

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
| `OFFICIAL_ROOMS` | `registon:comp:16,qumtepa:dm:20,bozor:comp:10,saroy:gg:12,karvon:wm:4,maydon:arena:10` | Doimiy (rasmiy) xonalar: `xarita:rejim:chegara`, vergul bilan. Kodi — `<xarita>-<rejim>`. `-` — o'chirish |

Bitta IP manzildan bir vaqtda ko'pi bilan 8 ta ulanish qabul qilinadi. Bir o'yinchidan soniyasiga 60 tadan ortiq xabar kelsa, ortiqchasi tashlanadi. Bitta xabar 8 KB dan oshmaydi.

## Xonalar va botlar

Bosh sahifadagi **server brauzeri** (har 5 soniyada yangilanadi, telefonda ham qulay):
- **Xarita kartochkalari** — har xaritada hozir nechta odam o'ynayapti va «O'ynash» tugmasi: shu xaritadagi eng ko'p odamli, lekin to'lmagan xonaga kiradi (xarita bo'yicha hech qanday xona bo'lmasa — `<xarita>-ochiq` yaratiladi).
- **Serverlar ro'yxati** — xarita rasmi, kod, rejim, o'yinchilar `odam+bot/chegara`, raund va hisob, holat (isinish / o'yin / tanaffus / to'la). Rasmiy xonalar tepada.
- **Filtrlar** — xarita, rejim, «Bo'sh joyi bor», «Bo'sh xonalarni yashirish»; tanlov brauzerda saqlanadi.
- Qatorni bossangiz darhol kirmaysiz — **xona oynasi** ochiladi: kim o'ynayapti (ism, klan tegi, jamoa, o'ldirish/o'lim), joriy hisob, «Kirish» va «Havolani nusxalash». Esc yoki «Yopish» — orqaga.

API: `GET /api/rooms` → `{rooms:[…], byMap:{qumtepa:3, …}}` (har qatorda `official` va hozircha bo'sh `region` maydoni ham bor); `GET /api/rooms/{kod}` → ro'yxatdagi maydonlar + `players:[{name, tag, team, k, d, bot}]` — yopiq xona uchun ham ishlaydi (kodni bilgan ko'radi), xona bo'lmasa 404.

**O'yin topish** — onlayn panelning tepasidagi katta tugma. Rejimni tanlaysiz:
- *Deathmatch* — darhol eng ko'p odam o'ynayotgan ochiq deathmatch xonasiga (rasmiy xonalar ham) kirasiz; hisob shart emas.
- *Raqobat* (5 ga 5) va *Wingman* (2 ga 2) — **reytingli navbat**, faqat hisobga kirganlar. Reytingingizga yaqin o'yinchilar qidiriladi: farq boshida ±150, har 20 soniyada ±50 kengayadi. Yetarli odam (10 yoki 4) yig'ilganda yangi yopiq xona `mm-<tasodifiy>` ochiladi, o'yinchilar reyting yig'indisi iloji boricha teng bo'lgan ikki jamoaga bo'linadi va hamma avtomatik shu xonaga tushadi. Xonaga faqat topilgan o'yinchilar kira oladi, egasi yo'q, bot qo'shib bo'lmaydi, jamoani server belgilaydi (M ishlamaydi) — shuning uchun natija reytingga yoziladi. Ekranning tepasida qidiruv holati: kutish vaqti, reyting oralig'i, navbatdagi o'yinchilar soni, «Bekor qilish».
- **Partiya:** navbatga turishdan oldin menyudagi onlayn do'stlaringizni belgilaysiz (Raqobatda 4 tagacha, Wingman'da 1). Ularga chaqiriq keladi; «Qo'shilish»ni bosganlar siz bilan navbatda turadi va doim bitta jamoaga tushadi (partiya buzilmaydi). Boshliq chiqsa partiya ham navbatdan chiqadi.
- Navbat server xotirasida: `POST /api/queue` (`join` + `mode` va `party`, `leave`, `accept`, `decline`), `GET /api/queue` — holat; brauzer navbatda turganda har 2 soniyada so'raydi, 15 soniya so'ramagan o'yinchi navbatdan chiqariladi. Partiya chaqirig'i `/api/social` javobida ham keladi. Hech kim kirmagan yoki hamma chiqib ketgan navbat xonasi 3 daqiqadan keyin o'chiriladi (shu vaqt ichida qayta ulanish mumkin). Reytingli o'yinlar faqat Raqobat xaritalarida (aim xaritalari navbatga tushmaydi).
- **Tashlab ketganlik uchun jazo** (`leaver.go`): topilgan o'yinga kelmagan yoki o'yin tugamasdan chiqib 2,5 daqiqa ichida qaytmagan o'yinchi navbatga vaqtincha kira olmaydi — oxirgi 24 soatda 1-marta 5 daqiqa, 2-marta 30 daqiqa, keyin har safar 2 soat (`leaver_bans` jadvali). Pauzadagi «Bosh menyu» birinchi bosilganda ogohlantiradi; chiqib ketgan o'yinchiga menyuda «Qayta ulanish» tugmasi va qolgan vaqt ko'rinadi (`GET /api/queue` → `rejoin`), taqiq vaqtida «O'yin topish» o'chiq va qancha qolgani yoziladi (`ban`). Taqiq vaqtida yoki qaytilmagan o'yin turganda navbatga turish va partiyaga qo'shilish rad etiladi. O'yin tugagach chiqish jazolanmaydi.

Wingman o'yinlari ham (botsiz bo'lsa) endi Raqobat kabi reytingga yoziladi.

**Rasmiy xonalar** (`OFFICIAL_ROOMS`) — har doim ochiq, ro'yxatning tepasida «RASMIY» belgisi bilan, odam bo'lmasa ham ko'rinadi: `registon-comp`, `qumtepa-dm`, `bozor-comp`, `saroy-gg`, `karvon-wm`, `maydon-arena`. Kirgan odam hech qachon bo'sh xonaga tushmaydi: odam + bot soni `min(chegara, 10)` ga yetguncha o'rta darajadagi botlar qo'shiladi, har yangi odam kirganda bitta bot chiqadi, jamoalar teng saqlanadi (odam jamoa tanlagach, raundlar orasida bot boshqa jamoaga o'tadi). Hamma chiqib ketsa botlar ham chiqadi, xona o'z xaritasida boshidan boshlanadi va bo'sh turganda hech narsa hisoblamaydi. Egasi yo'q: bot qo'shish/chiqarish, chegara, ommaviylik va rejimni hech kim o'zgartira olmaydi; xaritani faqat o'yin oxiridagi ovoz almashtiradi (kod o'zgarmaydi). Bot qatnashgani uchun bu o'yinlar reytingga yozilmaydi — ular tez o'ynash uchun. Chegara botlarni emas, odamlarni sanaydi.

**O'yin yaratish:** kod, kimga (ommaviy yoki yopiq) va maksimal o'yinchilar soni (standart: ommaviy, cheksiz). Yopiq o'yin ro'yxatda ko'rinmaydi, unga faqat kodni bilganlar kiradi. Bunday kodli o'yin allaqachon bo'lsa, yaratib bo'lmaydi. Kod bilan kirganda o'yin hali bo'lmasa, standart sozlamalar bilan yaratiladi.

**Xona havolasi:** har bir xonaning to'g'ridan-to'g'ri havolasi bor — `https://game.telegap.uz/x/<xarita>/<kod>`, masalan `/x/registon/umumiy`. Havolani ochgan odam ismi saqlangan bo'lsa darhol o'sha xonaga tushadi, bo'lmasa ismini yozib «Kirish»ni bosadi. Xona hali yo'q bo'lsa, havoladagi xaritada (va `?mode=dm` kabi berilgan rejimda) ommaviy xona bo'lib yaratiladi. Xonada turganingizda manzil satri shu havolani ko'rsatib turadi (xona egasi xaritani almashtirsa — yangilanadi), chiqqaningizda `/` ga qaytadi. Pauza menyusidagi **«Havolani ulashish»** telefonda ulashish oynasini ochadi, kompyuterda havolani nusxalaydi. Yopiq xona havolasi ham ishlaydi — kodni bilgan kiradi.

Telegram va boshqa messenjerlarda havola rasm va sarlavha bilan chiqadi: server `/x/...` so'rovi uchun `index.html` dagi `<title>` va `og:` teglarini to'ldiradi — «Registon — umumiy · 6/16 o'yinchi · Zarba Nuqtasi», rasm `web/og/<xarita>.png` (1200×630, o'yinning o'zidan olingan). Noto'g'ri havola (`/x/yoq-xarita/...`) oddiy bosh sahifani ochadi; `/x/` ostidan hech qanday fayl berilmaydi.

**Rejimlar** (o'yin yaratishda tanlanadi, xona egasi o'yin davomida almashtira oladi):
- *Raqobat*, *Wingman* (2 ga 2), *Retake* — raundlar va bomba.
- *Deathmatch — hamma hammaga*: har kim har kimga qarshi, 6 daqiqada eng ko'p o'ldirgan yutadi.
- *Deathmatch — jamoa jamoaga*: ikki jamoa, faqat raqib jamoani o'ldirish mumkin (jamoadoshga o'q, granata zarar bermaydi), ekranning tepasida ikkala jamoaning hisobi. 6 daqiqada ko'proq o'ldirgan jamoa yutadi. O'yin har ikki jamoada kamida bittadan o'yinchi (yoki bot) bo'lganda boshlanadi. Botlarga qarshi yakka o'yinda deathmatch doim «hamma hammaga».
- *Qurol poygasi*: eng kuchli quroldan boshlanib, har 2 o'ldirishda kuchsizrog'iga o'tiladi: Qirg'iy-50 (snayper) → Tog' (pulemyot) → Burkut → Lochin-K → Bo'ron (drobovik) → Shamol → Burgut .50 → Chaqmoq → P-18 → pichoq (19 o'ldirish). Pichoq bilan birinchi o'ldirgan yutadi.

**Xaritalar** (hammasi o'zimizniki, 44×44 katak): Qumtepa, Bozor, Karvonsaroy, Registon (Samarqand uslubidagi maydon: o'rtada favvora va rastalar, g'arbiy madrasa hovlisi — A, sharqiy yopiq bozor — B; koshinli peshtoqlar, minoralar, moviy gumbazlar), Saroy (tepalikdagi qal'a: o'rtada ustunli ichki zallar, sharqda devor bo'yi uzun yo'lak, g'arbda zinapoyali yo'l; qal'a tishlari, dumaloq minoralar, bayroqlar). Xarita sxemasi `web/index.html` dagi `MAPS` da, server nusxasi `tools/genmaps.py` bilan yasaladi.

**Aim xaritalari va «Aim» rejimi** — faqat nishon olish uchun kichik, simmetrik maydonlar (ikki tomon aynan bir xil sharoitda, `tools/mkaim.py` chizadi va tekshiradi):
- *Maydon* — ochiq maydon, o'rtada baland yashiklardan siniq devor va past yashik qatorlari: uzoq masofa, snayper va avtomat.
- *Chorraha* — xoch shaklidagi ko'chalar: uzun sharq–g'arb ko'chasi asosiy duel chizig'i, shimol va janub yo'llari — aylanib o'tish.
- *Hovli* — kvadrat hovli, o'rtasida to'rt eshikli kichik uy (eshiklar bir chiziqda emas): yaqin jang, burchakdan chiqish.

«Aim» rejimi (`arena`): jamoa jamoaga raundlar, bomba yo'q — raund raqib jamoa yo'q qilinganda tugaydi; 60 soniyada tugamasa, tirik qolganlari ko'p jamoa (teng bo'lsa — joni ko'p jamoa) yutadi. Raund oldidan 3 soniya. Faqat snayper va pichoq: har raundda zirh, dubulg'a, Qirg'iy-50 snayper miltig'i va pichoq beriladi (botlarga ham). Xarid menyusi, to'pponcha, granata, bomba, pul yo'q; o'lganlarning quroli yerga tushmaydi. Server ham shuni tekshiradi: aim'da snayper va pichoqdan boshqa qurolning o'qi qabul qilinmaydi. 9 g'alabagacha (8 raunddan keyin tomonlar almashadi). Aim xaritalari faqat shu rejimda, rejim faqat shu xaritalarda o'ynaladi — biri tanlansa ikkinchisi o'zi moslashadi (menyuda, xona yaratishda, xona egasi almashtirganda, ovoz berishda). Botlarga qarshi: siz va 2 ta bot; botlar raqib qidirib yuradi. Rasmiy xona: `maydon-arena` (10 kishilik, botlar bilan to'ladi). Reytingga yozilmaydi.

**Qurollar** (nomlari va dizayni o'zimizniki):

| Qurol | Turi | Narxi | Xususiyati |
|---|---|---|---|
| P-18 | to'pponcha | $200 | 20 o'q |
| Chaqmoq | avtomatik to'pponcha | $500 | 18 o'q, tez otadi |
| Burgut .50 | og'ir to'pponcha | $700 | boshga bitta o'q |
| Shamol | pistolet-pulemyot | $1250 | harakatda aniq, o'ldirish uchun $600 |
| Bo'ron | drobovik | $1200 | 8 ta saochma, yaqindan bir o'qda, uzoqda kuchsiz; o'ldirish uchun $900 |
| Lochin-K | avtomat | $2050 | yengil tepki |
| Burkut | avtomat | $2700 | kuchli, tepkisi katta |
| Tog' | pulemyot | $5000 | 100 o'q, og'ir — sekin yuradi, uzoq qayta o'qlanadi |
| Sayyod | yengil snayper | $1700 | 10 o'q, harakatda ham aniq, boshga bir o'qda |
| Qirg'iy-50 | snayper | $4750 | tanaga bir o'qda |
| Uchqun | elektroshoker (Jihozlar bo'limida) | $200 | 4,6 m gacha bitta urishda o'ldiradi, zirhdan o'tadi; bir martalik. 3 tugmasi bilan pichoq ⇄ Uchqun |

Drobovik o'qi serverga bitta tegish sifatida yuboriladi (nishonga tekkan saochmalar soni bilan), server zarar chegarasini saochmalar soniga qarab tekshiradi.

**Zarar** CS'dagi kabi hisoblanadi: bosh ×4, ko'krak va qo'llar ×1, qorin ×1,25, oyoq ×0,75. Zirh ko'krak, qorin va qo'llarni, dubulg'a boshni himoya qiladi; qurolning zirh teshish ko'rsatkichi qancha zarar o'tishini belgilaydi. Zarar har 12,7 metrda qurolning masofa ko'paytmasiga kamayadi (to'pponcha va pistolet-pulemyotda ko'proq, avtomat va snayperda deyarli kamaymaydi). Masalan, Burkut zirhsiz nishonni ko'krakka 3 o'qda, zirhlisini 4 o'qda o'ldiradi. Pichoq: oldidan 40 (og'ir zarba — o'ng tugma — 65), **orqadan esa har qanday zarba bitta urishda o'ldiradi**.

**Yerdagi qurollar.** O'lgan o'yinchining (va raundli rejimlarda botning) eng yaxshi quroli yerda qoladi. Unga qarasangiz, nishon ostida «+» belgisi, qurol nomi va o'qlari chiqadi: **E** — olish, **G** — qo'lingizdagi shu turdagi qurolni tashlab, yerdagisini olish. Bo'sh slotga qurol ustidan yurib o'tganda o'zi olinadi.

**Qaytarib sotish.** Shu raundda sotib olgan narsangizni xarid vaqti tugaguncha, bazada turgan holda, to'liq narxiga qaytarib sotishingiz mumkin: xarid menyusida «↩ Qaytarish» belgisini (yoki o'ng tugmani) bosing. Qurol qo'lingizda bo'lishi, zirh esa shikastlanmagan bo'lishi kerak; yerga tashlangan yoki boshqadan olingan qurol qaytarilmaydi. Server ham buni tekshiradi.

**Jamoadoshga pul.** Onlayn Raqobat va Wingman o'yinlarida xarid menyusining pastida «Jamoadoshga pul» bo'limi bor: tirik jamoadoshlaringiz (odamlar) ro'yxati va +$100 / +$500 / +$1000 / +$2000 tugmalari. Pul faqat xarid vaqtida, bazada turib beriladi, bir martada $5000 gacha, oluvchining puli $16 000 dan oshmaydi. Oluvchiga «… sizga $500 berdi» xabari chiqadi. Pulni server o'tkazadi (`bu` hodisasi, `s: 2`) va hamma shartni o'zi tekshiradi; rad etilsa pul qaytadi va sababi yoziladi.

**Askarlar modeli.** Hujumchi va himoyachi Blender'da yasalgan (`tools/soldier.py`, `pip install bpy` bilan ishlaydi): bitta yaxlit, silliq tana, 17 suyakli skelet, kamuflyaj teksturasi; 12,7 va 14,8 ming uchburchak. Qayta yasash: `python3 tools/soldier.py T web/models/soldier_T.glb preview.png` (CT uchun ham shunday). O'yinda modelning skeleti o'yin harakatlariga bog'lanadi: yurish, cho'kkalash, bosh va gavda harakati, qo'llar esa IK bilan qurol dastasiga. O'q tegadigan zonalar o'zgarmagan. Har bir askarga uchta bosh variantidan biri (hujumchi: to'qima shapka va to'liq soqol / salla va qisqa soqol / qisqa soch va mo'ylov; himoyachi: ko'zoynakli dubulg'a / dubulg'a va mo'ylov / kepka, quloqchin va qisqa soqol) va uchta teri rangidan biri tushadi. Sozlamalarda grafika «past» bo'lsa, oddiy (yengil) modellar ishlatiladi.

**Xona egasi** — o'yinni yaratgan o'yinchi. U chiqib ketsa, egalik eng oldin kirgan keyingi o'yinchiga o'tadi. Ega o'yin ichida Esc (telefonda «II») menyusidagi «Xona» bo'limida:
- bot qo'shadi: hujumga yoki himoyaga, oson / o'rta / qiyin (xonada ko'pi bilan 10 ta bot);
- bot yoki o'yinchini chiqaradi. O'yinchi uchun tugmani ikki marta bosish kerak. Chiqarilgan o'yinchi shu xonaga 10 daqiqa kira olmaydi;
- xonani ommaviy yoki yopiq qiladi, chegarani o'zgartiradi.

Botlar serverda ishlaydi: yo'l topadi, himoyachilar o'z joylarini qo'riqlaydi, hujumchilar nuqtaga borib bomba qo'yadi, himoyachilar uni zararsizlantiradi, raqibni ko'rsa otadi. Ular boshqa o'yinchilar kabi server hakami orqali o'ynaydi. Bot qatnashgan o'yin reytingga yozilmaydi.

**Botlarning granatalari** (`botnades.go`, brauzerdagi botlarda ham xuddi shunday): raundli rejimlarda raund boshida har botga tasodifiy granatalar beriladi (hujumchilarga ko'r qiluvchi va portlovchi, himoyachilarga yondiruvchi, portlovchi va ko'r qiluvchi; to'pponcha raundida kamroq, oson botlarda kamroq, qiyinlarida ko'proq). Hujumchilardan biri ko'pincha o'z nuqtasining tayyor tutun joyiga (granata mashqidagi o'sha joylar) boradi va jamoa kirganda tutun otadi — tutun aynan mashqdagi joyga tushadi. Nuqtaga kirishdan oldin hujumchi nuqta ustiga ko'r qiluvchi otadi (bot jamoadoshini ko'r qilmaydi). Raqibni hozirgina ko'rgan bot o'sha joyga portlovchi otadi (himoyachi — yondiruvchi); brauzerdagi himoyachi bot qadam tovushini eshitsa, o'sha yerga yondiruvchi otadi. Otish o'yinchinikiday hisoblanadi: bot kerakli joyga tushadigan burchakni o'yin fizikasi bilan qidiradi. Server botining granatasi uning holatida (`gn`) brauzerlarga boradi, portlash, tutun va olovni server o'zi hisoblaydi. Aim, deathmatch va qurol poygasida botlarda granata yo'q. `MAX_PLAYERS` (standart 64) — bitta xonadagi o'yinchilar va botlar uchun umumiy yuqori chegara.

## Matnli chat

Onlayn o'yinda CS'dagidek yoziladi: **Y** — hammaga (ikkala jamoa ham ko'radi), **U** — faqat o'z jamoasiga. Enter yuboradi, Esc bekor qiladi, telefonda yuqoridagi «Chat» tugmasi bor. O'lgan o'yinchi xabari `*O'LIK*` belgisi bilan chiqadi. Saytda xabar server orqali o'tadi. Jamoa xabari faqat jamoadoshlarga yuboriladi, ismni server qo'yadi, ketma-ket ko'p yozishga chek bor.

## Ovozli chat

Onlayn xonada o'yinchilar bir-biri bilan gaplashadi (faqat o'yinning o'z saytida; Claude ichidagi versiyada o'chiq). Sozlamalarda tanlanadi:

- **Ovozli chat:** O'chiq, Jamoa (faqat jamoadoshlar eshitadi) yoki Hamma (raqiblar bilan ham, agar ular ham «Hamma»ni tanlagan bo'lsa).
- **Mikrofon:** tugmani bosib gapirish yoki doim ochiq. Gapirish tugmasi standart K, sozlamalarda istalgan tugmaga yoki sichqonchaning qo'shimcha tugmasiga (yon tugmalar 4/5, g'ildirak) o'zgartiriladi. Telefonda mikrofon tugmasi bor.
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
- Raund mantig'ini (vaqt, hisob, bomba, g'olib, yarim vaqt, overtime) **server** yuritadi va holatni soniyasiga 10 marta yuboradi. (Claude ichidagi versiyada serverning o'rnini o'yinchilardan biri — xost bajaradi.)
- O'q tegishini otgan o'yinchining brauzeri aniqlaydi, chunki shunda o'yin tez seziladi. Lekin har bir tegishni server tekshiradi (pastda).

### Server hakamligi

- **O'q:** otgan va tekkan o'yinchi tirikmi, raqibmi, zarar qurolnikidan oshmaydimi, qurol imkonidan tez otilmaganmi, orada devor yo'qmi. Kechikishni hisobga olish uchun so'nggi ~0,5 soniyadagi holatlar va tananing chetlari tekshiriladi. Noto'g'ri tegish boshqalarga yuborilmaydi.
- **Jon:** server har bir o'yinchining jonini o'zi hisoblaydi. Joni tugagan, lekin o'lmagan o'yinchiga o'lim buyuriladi. Bo'ysunmasa, boshqalarga o'lik ko'rsatiladi va keyingi raundgacha tirilmaydi.
- **Harakat:** juda tez siljish (teleport, tezlik cheati) yoki devor ichiga kirish qabul qilinmaydi, o'yinchi oxirgi to'g'ri joyiga qaytariladi. Yangi raund faqat o'z bazasida boshlanadi.
- **Bomba:** faqat A/B nuqtada, 3 soniya qimirlamay turilganda o'rnatiladi. Zararsizlantirish bomba yonida 7 soniya (to'plam bilan 4) turilgandagina qabul qilinadi. To'plam va zirh faqat xarid vaqtida paydo bo'lishi mumkin.
- Shubhali harakatlar server logiga yoziladi (`docker compose logs zarba | grep hakam`).
- Hali o'yinchining o'zida qoladiganlari: granata va bomba portlashi zarari, pul (iqtisod). Ular keyingi qadamlarda serverga o'tadi.
- `ZARBA_DEBUG=1` faqat avtomatik testlar uchun (raund holatini o'zgartirishga ruxsat beradi). Serverda yoqmang.
- Server har bir xabardan faqat kerakli maydonlarni o'qiydi. Hisobga kirgan o'yinchining ismini va reytingini server o'zi yozadi, shuning uchun boshqa birovning nomi bilan o'ynab bo'lmaydi.
- Tekshiruv: `GET /healthz` hozirgi xonalar va o'yinchilar sonini qaytaradi.

## Boshqaruv

WASD — harakat, sichqoncha — nishonga olish va otish, C yoki Ctrl — cho'kkalab yurish, Shift — sekin yurish, Bo'shliq — sakrash, R — qayta o'qlash, B — xarid, E (bosib turing) — bomba o'rnatish yoki zararsizlantirish, Tab — hisob, G — qurolni tashlash (yerdagi qurolga qarab turilsa — almashtirish), E (qurolga qarab) — yerdagi qurolni olish, 3 (yana bosilsa) — pichoq ⇄ Uchqun, M — jamoani almashtirish, K (bosib turing) — ovozli chatda gapirish.

**Tugmalarni sozlash.** Sozlamalardagi «Tugmalarni sozlash» bo'limida 21 ta amalning (yurish, sakrash, cho'kkalash, sekin yurish, qayta o'qlash, olish, tashlash, xarid, qurollar 1–5 va oldingi qurol, hisob jadvali, jamoa, chat) tugmasini o'zgartirasiz: amalni bosing, keyin yangi tugmani. Band tugmani tanlasangiz, ikki amal tugmalarini almashtiradi; eski tugma endi hech narsa qilmaydi. Gapirish tugmasi bilan to'qnashmaydi. Yordam ro'yxatidagi tugmalar ham yangilanadi, sozlama brauzerda saqlanadi, «Asl holiga qaytarish» bor. **Grafika** (Avto, Juda past, Past, O'rta, Yuqori — Avto kadrlar soni past bo'lsa o'zi pasaytiradi) va **ko'rish burchagi** (70°–110°) ham sozlamalarda.

**Nishon ko'rsatkichi** sozlamalarda (bosh menyu yoki Esc menyusi) o'zingizga moslanadi, yonida jonli namuna turadi: yurganda va otganda *kattalashadi* (standart, aniqlik qancha tushganini ko'rsatadi), *faqat otganda* kattalashadi yoki *doim bir xil* turadi; uzunligi, qalinligi, oralig'i, shaffofligi, markaz nuqta, qora chegara va rangi (tayyor ranglar yoki istalgan rang). «Asl holiga qaytarish» standartga qaytaradi. Tanlov brauzerda saqlanadi.

Granatalar 4-slotda: 4 tugmasi granatani oladi, yana bosilsa keyingi turiga o'tadi. Chap tugma — uzoqqa, o'ng tugma — yaqinga otish; granata tugma qo'yib yuborilganda uchadi. Turlari: portlovchi ($300), ko'r qiluvchi ($200, ikkitagacha), tutun ($300, 18 soniya ko'rinishni to'sadi), yondiruvchi ($400, 7 soniya yonadi, tutun uni o'chiradi). Bir o'yinchida ko'pi bilan 4 ta granata bo'ladi. Himoyachilar zararsizlantirish to'plamini ($400) olsa, bomba 7 emas, 4 soniyada zararsizlanadi.

O'yin CS kabi ikki yarimdan iborat. Sozlamalarda uzunlikni tanlaysiz: to'liq (MR12, 13 g'alabagacha, 12 raunddan keyin tomonlar almashadi) yoki qisqa (9 g'alabagacha, 8 raunddan keyin almashadi). Hisob teng bo'lsa (12:12 yoki 8:8), qo'shimcha vaqt boshlanadi: 6 raund, 4 g'alaba kerak, har biriga $12 500 beriladi, 3 raunddan keyin tomonlar almashadi. Botlarga qarshi ikkinchi yarimda siz himoyada bo'lasiz: botlar bombani olib, A yoki B nuqtani egallaydi va o'rnatadi, siz esa E bilan zararsizlantirasiz. Onlayn xonada formatni xonaga birinchi kirgan o'yinchining sozlamasi belgilaydi.

Onlayn o'yinda granata otilganda faqat uning boshlang'ich holati yuboriladi, uchishini har bir brauzer bir xil qadam bilan o'zi hisoblaydi, shuning uchun tutun hammada bir joyga tushadi. Har bir o'yinchi o'ziga tekkan zarar va ko'r bo'lishni o'zi qo'llaydi. Jamoadoshning granatasi zarar bermaydi, o'zingizniki esa beradi.

**Mashq** (bosh menyudagi «Mashq» paneli, onlayn hisobga ta'sir qilmaydi, Claude versiyasida ham ishlaydi):
- *Nishon mashqi* — 60 soniya. Qurolni panelda tanlaysiz, qiyinlikni «Botlarga qarshi» panelida. Oson bo'lsa nishonlar turadi, o'rta bo'lsa yon tomonga yuradi, qiyin bo'lsa tez yuradi va cho'kkalaydi. Nishonlar o'q uzmaydi, o'qlar tugamaydi. Bir vaqtda 3 ta nishon bo'ladi, otib tushirilgani boshqa joyda yana chiqadi. Ochko = nishon × 100 + boshga tekkani uchun +50, so'ng aniqlik bilan ko'paytiriladi. Natijalar: aniqlik, boshga tekkan foiz, o'rtacha reaksiya (ms). Rekord har qurol va qiyinlik uchun brauzerda saqlanadi.
- *Granata mashqi* — vaqt cheklanmagan, granatalar tugamaydi, 4 tugmasi turini almashtiradi. Tugmani bosib tursangiz, granata qayerdan uchib, qayerga tushishi ko'rinadi: hisob haqiqiy otish bilan aynan bir xil. R — oxirgi otgan joyingizga o'sha nishon burchagi bilan qaytasiz. Oxirgi 8 ta granata tushgan joy belgilanib turadi. Har xaritada A, B va o'rta uchun tayyor tutun joylari bor: yerdagi havorang halqa — turish joyi, yonidagi halqa — tutun tushadigan joy. Halqaga kirsangiz, qayerga nishon olish oq doira bilan ko'rsatiladi; aniq tursangiz doira yashil bo'ladi. L — keyingi tayyor joyga o'tkazadi (tutun qo'lda, nishon to'g'ri) — faqat otish qoladi. O'z granatangiz zarar bermaydi, ko'r qiluvchi esa ko'r qiladi.

O'yin boshlanganda brauzer to'liq ekranga o'tadi va o'yin tugmalarini qulflaydi. Shu tufayli Ctrl+W yorliqni yopmaydi va Ctrl bilan cho'kkalash xavfsiz. Faqat o'yin tugmalari qulflanadi: Alt+Tab va Win ishlayveradi, Esc bosilsa to'liq ekrandan chiqiladi.

## O'yinni yangilash

`web/index.html` ni almashtiring va `docker compose up -d --build` ni qayta ishga tushiring. three.js `web/vendor/` ichida turadi (MIT litsenziyasi, `THREE-LICENSE.txt`).
