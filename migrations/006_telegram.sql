-- Telegram orqali kirish va parolni tiklash

ALTER TABLE users ADD COLUMN tg_id BIGINT;               -- ulangan Telegram hisobi
ALTER TABLE users ADD COLUMN tg_name TEXT;               -- ko'rsatish uchun: @username yoki ism
CREATE UNIQUE INDEX users_tg ON users (tg_id) WHERE tg_id IS NOT NULL;

ALTER TABLE sessions ADD COLUMN via TEXT NOT NULL DEFAULT 'pass';   -- pass | tg: Telegram bilan kirgan eski parolsiz yangisini qo'ya oladi
