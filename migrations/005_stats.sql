-- Batafsil statistika (qurollar, xaritalar, rejimlar) va medallar

CREATE TABLE player_stats (
    user_id   BIGINT  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key       TEXT    NOT NULL,      -- k, hs, wk:rifle, mp:qumtepa, best_rk, ... (medals.go)
    value     BIGINT  NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, key)
);

CREATE TABLE achievements (
    user_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id        TEXT        NOT NULL,  -- medalDefs dagi id
    at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);
