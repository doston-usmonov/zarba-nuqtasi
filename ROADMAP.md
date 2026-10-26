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
- [ ] Jamoadoshga pul berish
- [ ] Botlar ham granata otishi (tutun bilan yopish, ko'r qilib kirish)

## 3. Server hakamligi (anti-cheat)
- ✅ Zarba tekshiruvi serverda (zarar, otish tezligi, devor, kechikishni hisobga olib), jonni server hisoblaydi
- ✅ Harakat tezligi va pozitsiyani serverda tekshirish
- ✅ Raund mantig'i (vaqt, bomba, g'olib, yarim vaqt, overtime) serverda, bomba o'rnatish/zararsizlantirish tekshiriladi
- ✅ Reytingga faqat server tasdiqlagan o'ldirishlar yoziladi
- [ ] Granata va bomba portlashi zararini serverda hisoblash
- [ ] Iqtisodni (pul, xaridlar) serverda yuritish

## 4. Matchmaking
- ✅ Ochiq o'yinlar ro'yxati, ommaviy/yopiq xonalar, o'yinchilar chegarasi
- ✅ Xona egasi: bot qo'shish/chiqarish, o'yinchini chiqarish
- [ ] Reytingga qarab navbat: "O'yin topish" tugmasi
- [ ] 5v5 (Raqobatli) va 2v2 (Wingman) rejimlari
- [ ] Partiya: do'stlar bilan birga navbatga turish
- [ ] O'yinni tashlab ketganlik uchun jazo (vaqtincha navbat taqiqi)

## 5. Xaritalar
- [ ] 2–3 ta yangi xarita (o'zbek me'morchiligi ruhida: bozor, karvonsaroy, zavod)
- [ ] O'yin oldidan xarita tanlash uchun ovoz berish

## 6. Ijtimoiy
- ✅ Xona ichida matnli chat (Y — hamma, U — jamoa)
- ✅ Ovozli chat (WebRTC): jamoa yoki hamma, K bosib gapirish yoki doim ochiq mikrofon, TURN (coturn)
- [ ] Do'stlar ro'yxati, profil sahifasi, batafsil statistika (qurollar bo'yicha, xaritalar bo'yicha)
- [ ] Telegram orqali kirish, parolni tiklash

## 7. Kosmetika va yutuqlar
- [ ] Yutuqlar va medallar (masalan: "1 raundda 5 ta o'ldirish")
- [ ] Qurol ko'rinishlari (skinlar): yutuqlar va daraja orqali ochiladi, tasodifiy sandiq va pullik o'yin elementlari yo'q

## Texnik
- ✅ PostgreSQL: hisoblar, reyting va o'yinlar tarixi, migratsiyalar, kunlik zaxira
- [ ] O'yin yozuvlari (demo) va qayta ko'rish
- [ ] Monitoring: o'yinchilar soni, xonalar, xatolar (Prometheus)
