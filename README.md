# Zarba Nuqtasi — o'yin serveri

Brauzerdagi taktik otishma va uning onlayn serveri. Bitta Go dasturi o'yin sahifasini tarqatadi va `/ws` orqali o'yinchilarni xonalarga ulaydi. Tashqi Go kutubxonasi yo'q, faqat standart kutubxona ishlatilgan.

```
zarba-server/
├── main.go            HTTP server, sozlamalar, statik fayllar (embed)
├── hub.go             xonalar, o'yinchilar, holatni uzatish (relay)
├── ws.go              WebSocket (RFC 6455) — serverga kerakli qismi
├── web/index.html     o'yinning o'zi
├── web/vendor/        three.js r128 (CDN'ga bog'liq emas)
├── Dockerfile
├── docker-compose.yml
└── deploy/nginx.conf  domen + HTTPS + WebSocket proksi namunasi
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

## Sozlamalar (muhit o'zgaruvchilari)

| O'zgaruvchi | Standart | Ma'nosi |
|---|---|---|
| `ADDR` | `:8080` | Tinglanadigan manzil |
| `MAX_PLAYERS` | `16` | Bitta xonadagi o'yinchilar chegarasi |
| `ALLOWED_ORIGINS` | bo'sh | Bo'sh bo'lsa faqat o'z domeni. `*` bo'lsa hammasi. Aks holda vergul bilan ajratilgan hostlar |

Bitta IP manzildan bir vaqtda ko'pi bilan 8 ta ulanish qabul qilinadi. Bir o'yinchidan soniyasiga 60 tadan ortiq xabar kelsa, ortiqchasi tashlanadi. Bitta xabar 8 KB dan oshmaydi.

## Qanday ishlaydi

- Har bir o'yinchi soniyasiga ~20 marta o'z holatini yuboradi: joylashuv, qarash yo'nalishi, jon, qurol, otilgan o'q va tekkan zarbalar. Server uni xonadagi qolganlarga uzatadi, yangi kelganga esa hozirgi holatni beradi.
- Raund mantig'ini (vaqt, hisob, bomba, g'olib) o'yinchilardan biri, ya'ni **xost** yuritadi. Xost tanlovi avtomatik. U chiqib ketsa, keyingisi o'sha joydan davom ettiradi.
- Zarbani otgan o'yinchining brauzeri hisoblaydi (client-side hit detection). Do'stlar orasida o'ynash uchun bu yetarli va tez. Ochiq turnir o'tkazmoqchi bo'lsangiz, firibgarlikka qarshi hisob-kitobni serverga o'tkazish kerak bo'ladi.
- Tekshiruv: `GET /healthz` hozirgi xonalar va o'yinchilar sonini qaytaradi.

## Boshqaruv

WASD — harakat, sichqoncha — nishonga olish va otish, C yoki Ctrl — cho'kkalab yurish, Shift — sekin yurish, Bo'shliq — sakrash, R — qayta o'qlash, B — xarid, E (bosib turing) — bomba o'rnatish yoki zararsizlantirish, Tab — hisob, M — jamoani almashtirish.

O'yin boshlanganda brauzer to'liq ekranga o'tadi va o'yin tugmalarini qulflaydi. Shu tufayli Ctrl+W yorliqni yopmaydi va Ctrl bilan cho'kkalash xavfsiz. Faqat o'yin tugmalari qulflanadi: Alt+Tab va Win ishlayveradi, Esc bosilsa to'liq ekrandan chiqiladi.

## O'yinni yangilash

`web/index.html` ni almashtiring va `docker compose up -d --build` ni qayta ishga tushiring. three.js `web/vendor/` ichida turadi (MIT litsenziyasi, `THREE-LICENSE.txt`).
