-- O'yin yozuvlari (demo): fayl DATA_DIR/demos da, bu yerda ro'yxati

CREATE TABLE demos (
    id          BIGSERIAL   PRIMARY KEY,
    file        TEXT        NOT NULL,
    room        TEXT        NOT NULL,
    map         TEXT        NOT NULL,
    mode        TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL,
    score_a     INT         NOT NULL,
    score_b     INT         NOT NULL,
    players     JSONB       NOT NULL DEFAULT '[]'::jsonb   -- [{name, uid, bot}]
);
