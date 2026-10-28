-- Reytingli (navbat) o'yinni tashlab ketganlar: vaqtincha navbat taqiqi

CREATE TABLE leaver_bans (
    user_id     BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    strikes     INT         NOT NULL DEFAULT 0,      -- oxirgi 24 soatdagi tashlab ketishlar
    until_at    TIMESTAMPTZ NOT NULL,                -- shu vaqtgacha navbatga kira olmaydi
    last_at     TIMESTAMPTZ NOT NULL                 -- oxirgi tashlab ketish
);
