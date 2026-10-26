-- Tajriba (XP), darajalar, kunlik vazifalar va qurol bo'yoqlari

ALTER TABLE users ADD COLUMN xp BIGINT NOT NULL DEFAULT 0 CHECK (xp >= 0);
ALTER TABLE users ADD COLUMN loadout JSONB NOT NULL DEFAULT '{}'::jsonb;  -- qurol → tanlangan bo'yoq

CREATE TABLE daily_progress (
    user_id   BIGINT  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day       DATE    NOT NULL,          -- Toshkent vaqti bo'yicha kun
    mission   TEXT    NOT NULL,
    progress  INT     NOT NULL DEFAULT 0,
    done      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, day, mission)
);
CREATE INDEX daily_progress_day ON daily_progress (day);
