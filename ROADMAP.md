# Zarba Nuqtasi — rivojlanish rejasi

Haqiqiy CS'dagi imkoniyatlar bosqichma-bosqich qo'shiladi. Bosqichlar ustuvorlik bo'yicha tartiblangan, har biri alohida chiqariladi. Bajarilganlari ✅ bilan belgilangan.

## 1. Asos ✅
- ✅ Bomba rejimi: o'rnatish, zararsizlantirish, 8 g'alabagacha
- ✅ Iqtisod va xarid menyusi, qurollar, tepki naqshlari, zarar tizimi, bronejilet va dubulg'a
- ✅ Botlar (3 daraja), radar, hisob jadvali, ovozlar
- ✅ Onlayn xonalar, Go server, Docker, avtomatik deploy
- ✅ Telefonda o'ynash (sensorli boshqaruv)
- ✅ Ro'yxatdan o'tish, Elo reyting, darajalar, reyting jadvali, o'yinlar tarixi

## 2. O'yin qoidalari CS'ga yaqinroq
- ✅ Granatalar: portlovchi, tutun, ko'r qiluvchi, yondiruvchi (xarid menyusida, 4-slot, onlayn sinxron)
- ✅ Yarim vaqtda tomonlarni almashtirish, MR12 (13 g'alabagacha) yoki qisqa o'yin (9 g'alabagacha), overtime (MR3)
- ✅ Botlarga qarshi ikkinchi yarimda himoyada o'ynash: botlar bombani olib, nuqtani egallab, o'rnatadi
- ✅ Zararsizlantirish to'plami (himoyachilar uchun: 7 o'rniga 4 soniya)
- ✅ Qurolni tashlash (G) va ko'tarish (E), o'lgan o'yinchining quroli yerda qoladi, yangi qurol olinganda eskisi tushadi
- ✅ Jamoadoshga qurol berish (sotib olib, G bilan tashlab berish)
- ✅ Jamoadoshga pul berish: xarid menyusida, server o'tkazadi va tekshiradi
- ✅ Botlar ham granata otishi: tayyor joydan tutun, nuqtaga kirishdan oldin ko'r qiluvchi, raqib ko'ringan joyga portlovchi yoki yondiruvchi (offlayn va server botlari)
- ✅ CS'dagi zarar: masofa birligi 12,7 m, qo'llar zonasi (×1, zirh himoya qiladi); orqadan pichoq bitta urishda
- ✅ Elektroshoker «Uchqun» ($200): 4,6 m, bitta urishda, bir martalik; server masofa va qo'ldaligini tekshiradi
- ✅ Yerdagi qurol: «+» belgisi, E — olish, G — almashtirish; botlarning quroli ham yerda qoladi
- ✅ Qaytarib sotish: shu raund xaridlari xarid vaqtida to'liq narxiga
- ✅ Nishon ko'rsatkichi sozlamalari: kattalashadi / faqat otganda / doim bir xil, uzunlik, qalinlik, oraliq, shaffoflik, nuqta, chegara, istalgan rang

## 3. Server hakamligi (anti-cheat)
- ✅ Zarba tekshiruvi serverda (zarar, otish tezligi, devor, kechikishni hisobga olib), jonni server hisoblaydi
- ✅ Harakat tezligi va pozitsiyani serverda tekshirish
- ✅ Raund mantig'i (vaqt, bomba, g'olib, yarim vaqt, overtime) serverda, bomba o'rnatish/zararsizlantirish tekshiriladi
- ✅ Reytingga faqat server tasdiqlagan o'ldirishlar yoziladi
- ✅ Granata va bomba portlashi zararini serverda hisoblash (granata yo'li brauzer bilan aynan bir xil hisoblanadi)
- ✅ Iqtisod serverda: pul, xaridlar, qo'ldagi qurollar; yerdagi qurolni faqat bir kishi oladi; qayta ulanganda pul saqlanadi

## 4. Matchmaking
- ✅ Ochiq o'yinlar ro'yxati, ommaviy/yopiq xonalar, o'yinchilar chegarasi
- ✅ Xona egasi: bot qo'shish/chiqarish, o'yinchini chiqarish
- ✅ Server brauzeri: xarita kartochkalari (hozir nechta odam, «O'ynash»), filtrlar, xona oynasi (o'yinchilar va hisob), `GET /api/rooms/{kod}` — [docs/play-cs-uslubi.md](docs/play-cs-uslubi.md) 3-bosqich
- ✅ Rasmiy xonalar (`OFFICIAL_ROOMS`): har doim ochiq, egasi yo'q, botlar odamlar soniga qarab qo'shiladi/chiqadi, bo'shasa xona qoladi — [docs/play-cs-uslubi.md](docs/play-cs-uslubi.md) 2-bosqich
- ✅ Xona havolasi `/x/<xarita>/<kod>`: ochgan odam shu xonaga tushadi, messenjerlarda rasm va o'yinchilar soni bilan chiqadi (og: teglari), pauza menyusida «Havolani ulashish» — [docs/play-cs-uslubi.md](docs/play-cs-uslubi.md) 1-bosqich
- ✅ Reytingga qarab navbat: «O'yin topish» (Raqobat 5×5, Wingman 2×2; ±150, har 20 s da +50; yopiq botsiz `mm-` xona) va Deathmatch'ga tez kirish — [docs/play-cs-uslubi.md](docs/play-cs-uslubi.md) 4-bosqich
- ✅ Deathmatch (har kim har kimga yoki jamoa jamoaga — xona yaratishda tanlanadi; qayta tug'ilish, 6 daqiqa) va Qurol poygasi (snayperdan boshlab har 2 o'ldirishda kuchsizroq qurol, oxiri pichoq); hisobni server yuritadi
- ✅ Kuzatuvchi rejimi: xonani o'yinchilarga ko'rinmasdan kuzatish (o'yinchilarni kuzatish yoki erkin kamera), chati faqat kuzatuvchilarga
- ✅ Wingman (2 ga 2, faqat A nuqta, MR8) va Retake (bomba raund boshidan o'rnatilgan, qurollar beriladi)
- ✅ Partiya: do'stlar bilan birga navbatga turish (bitta jamoaga tushadi)
- ✅ O'yinni tashlab ketganlik uchun jazo: 2,5 daqiqada qaytmasa navbat taqiqi (5 daqiqa → 30 daqiqa → 2 soat), menyuda «Qayta ulanish»

## 5. Xaritalar
- ✅ Ikki yangi ixcham xarita: Bozor va Karvonsaroy (Qumtepa bilan birga 3 ta); xona yaratishda va xona egasi o'yin davomida tanlaydi
- ✅ Bekat (eski temiryo'l bekati: perron, depo — A, yuk ombori — B; soat minorasi, relslar; avvalgi Registon o'rniga) va Saroy (tepalikdagi qal'a: ichki zallar, devor bo'yi uzun yo'lak, zinapoyali yo'l; qal'a tishlari, minoralar, bayroqlar) — jami 5 ta xarita, ovoz berish 1–5 tugmalari
- ✅ Zavod xaritasi: dastgohlar zali (A), konteynerli ombor (B), konveyer zali, qozonxona va ustaxona; mo'rilar, quvurlar; tayyor tutun joylari; rasmiy `zavod-comp` xonasi
- ✅ Xarita muharriri: o'z xaritangizni chizish (simmetriya, tasodifiy xarita, shablonlar), botlar bilan sinash, galereya (yangi/mashhur/meniki), onlayn xona va havola
- ✅ Aim xaritalari (Maydon, Chorraha, Hovli) va «Aim» rejimi: raundlar, bomba yo'q, faqat snayper va pichoq (server ham tekshiradi), botlar ov qiladi; rasmiy `maydon-arena` xonasi
- ✅ O'yin oxirida keyingi xarita uchun ovoz berish (1/2/3 tugmalari)

## 6. Ijtimoiy
- ✅ Xona ichida matnli chat (Y — hamma, U — jamoa)
- ✅ Ovozli chat (WebRTC): jamoa yoki hamma, K bosib gapirish yoki doim ochiq mikrofon, TURN (coturn)
- ✅ Do'stlar: login bo'yicha so'rov, kim onlayn (menyuda yoki qaysi xonada o'ynayapti), do'st xonasiga bitta tugma bilan qo'shilish
- ✅ Partiya: pauza menyusidan onlayn do'stni o'z xonangga chaqirish, chaqiriq menyuda yoki o'yin ichida keladi, qabul qilgan do'st sen bilan bitta jamoaga tushadi
- ✅ Klanlar turniri: 4 yoki 8 klan, setka (reyting bo'yicha), har o'yinga alohida xona (faqat ikki klan, boshqalar kuzatadi), natija o'zi yoziladi, texnik g'alaba
- ✅ Klanlar: nom va 2–5 belgili teg ([TEG] o'yinda ism oldida), do'stlarni taklif qilish, egasi a'zoni chiqaradi, klanlar reytingi (a'zolarning o'rtacha reytingi)
- ✅ Batafsil statistika: qurollar, xaritalar va rejimlar bo'yicha, eng yaxshi natijalar (profilda «Statistika»)
- ✅ Telegram orqali kirish, hisobni ulash, parolni tiklash (Telegram bilan kirib, eski parolsiz yangisini qo'yish)
- ✅ Telegram bot: /start (O'ynash tugmasi), /online (kim o'ynayapti, ochiq xonalar), /top (reyting), /me (o'z natijalari)

## 7. Kosmetika va yutuqlar
- ✅ To'rtta yangi qurol: Chaqmoq (avtomatik to'pponcha), Bo'ron (drobovik), Tog' (pulemyot), Sayyod (yengil snayper) — serverda tekshiriladi, botlar ham ishlatadi
- ✅ Askarlarda 3 xil bosh (soch, soqol, bosh kiyim) va 3 xil teri rangi
- ✅ Qurollar Blender'da qayta yasalgan (silliq qirralar, tepki, qo'riqlagich, portlar, relslar va boshqa detallar); bo'yoqlar avvalgidek ishlaydi
- ✅ Blender'da yasalgan 3D askar modellari (yaxlit silliq tana, skelet, kamuflyaj), o'yin harakatlariga bog'langan
- ✅ Personajlar odamga o'xshashroq: yuz (ko'z, qosh, burun, quloq, lab), soqol yoki dubulg'a va quloqchin, yumaloq tana va qo'l-oyoq, barmoqli qo'lqop, oyoq kiyimi yerga tekis turadi; har bir tana qismi bitta mesh qilib birlashtirilgan (kamroq draw call)
- ✅ Raund MVP (eng ko'p o'ldirgan, bombani o'rnatgan yoki zararsizlantirgan), o'yin oxirida natijalar jadvali va o'yin MVP'si, killcam
- ✅ XP va 40 ta daraja, har kuni 3 ta vazifa (Toshkent vaqti bilan yangilanadi)
- ✅ Qurol bo'yoqlari (10 ta o'zimizning naqsh: Qum, Tun, Zangori, Atlas, Lola, Gilam, Shoh, Olov, Muz, Afsona) daraja bilan ochiladi; tasodifiy sandiq va pullik narsa yo'q
- ✅ Yutuqlar va medallar: 18 ta (Beshlik — bir raundda 5 ta, Yolg'iz qahramon — 1 ga 3 va boshqalar), XP beradi, profilda «Medallar»

## Mashq
- ✅ Nishon mashqi: 60 soniya, qurolni o'zingiz tanlaysiz, nishonlar o'q uzmaydi, qiyinlikka qarab turadi, yuradi yoki cho'kkalaydi. Natija: ochko, aniqlik, boshga tekkani, o'rtacha reaksiya. Har qurol va qiyinlik uchun rekord brauzerda saqlanadi
- ✅ Granata mashqi: cheksiz granata, tugma bosib turilganda uchish yo'li va tushadigan joy ko'rinadi (haqiqiy otish bilan aynan bir xil fizika), R — oxirgi otgan joyga qaytish, tushgan joylar belgilanib qoladi, o'z granatangiz zarar bermaydi
- ✅ Tayyor tutun joylari: har xaritada A, B va o'rta uchun 3 tadan (jami 15), o'yin fizikasi bilan topilgan (`tools/lineups.py`); turish halqasi, nishon doirasi, L — keyingi joy
- ✅ Xarita bo'yicha tayyor granata otish joylari (smoke lineup) to'plami — yuqoridagi «Tayyor tutun joylari»

## Ovoz
- ✅ O'zbekcha diktor: raund boshi, bomba o'rnatildi/zararsizlantirildi, 10 soniya qoldi, g'olib tomon, ketma-ket o'ldirishlar, yarim vaqt, hal qiluvchi raund, o'yin tugadi (sozlamalarda o'chiriladi)

## Sozlamalar
- ✅ Tugmalarni qayta sozlash (21 ta amal, band tugma almashtiriladi), grafika sifati (Avto/Juda past/Past/O'rta/Yuqori), ko'rish burchagi

## Texnik
- ✅ PostgreSQL: hisoblar, reyting va o'yinlar tarixi, migratsiyalar, kunlik zaxira
- ✅ O'yin yozuvlari (demo) va qayta ko'rish: server raundli o'yinlarni yozadi, profilda «Yozuvlar» — pauza, tezlik, vaqt chizig'i, raundlar, o'yinchi ko'zi bilan
- ✅ Monitoring: /metrics (Prometheus, token bilan) — o'yinchilar, xonalar, navbat, xatolar, hakam, xona takti, baza; docker-compose'da ixtiyoriy Prometheus
