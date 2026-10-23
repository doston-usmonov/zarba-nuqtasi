-- Hisoblar, sessiyalar, o'yinlar va ularning ishtirokchilari

CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    username    TEXT        NOT NULL CHECK (username ~ '^[A-Za-z0-9_]{3,16}$'),
    pass_salt   BYTEA       NOT NULL,
    pass_hash   BYTEA       NOT NULL,
    pass_iter   INT         NOT NULL,
    rating      INT         NOT NULL DEFAULT 1000 CHECK (rating >= 0),
    matches     INT         NOT NULL DEFAULT 0,
    wins        INT         NOT NULL DEFAULT 0,
    kills       INT         NOT NULL DEFAULT 0,
    deaths      INT         NOT NULL DEFAULT 0,
    hs          INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_username_lower ON users (lower(username));
CREATE INDEX users_leaderboard ON users (rating DESC, wins DESC, id) WHERE matches > 0;

CREATE TABLE sessions (
    token_hash  BYTEA       PRIMARY KEY,           -- sha256(token); tokenning o'zi saqlanmaydi
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE matches (
    id          BIGSERIAL PRIMARY KEY,
    room        TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL,
    score_t     INT         NOT NULL,
    score_ct    INT         NOT NULL,
    winner      TEXT        NOT NULL CHECK (winner IN ('T', 'CT'))
);
CREATE INDEX matches_ended ON matches (ended_at DESC);

CREATE TABLE match_players (
    match_id      BIGINT  NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    user_id       BIGINT  REFERENCES users (id) ON DELETE SET NULL,  -- NULL: mehmon
    name          TEXT    NOT NULL,
    team          TEXT    NOT NULL CHECK (team IN ('T', 'CT')),
    kills         INT     NOT NULL DEFAULT 0,
    deaths        INT     NOT NULL DEFAULT 0,
    hs            INT     NOT NULL DEFAULT 0,
    won           BOOLEAN NOT NULL,
    rating_before INT,
    rating_after  INT
);
CREATE INDEX match_players_match ON match_players (match_id);
CREATE INDEX match_players_user ON match_players (user_id, match_id DESC) WHERE user_id IS NOT NULL;
