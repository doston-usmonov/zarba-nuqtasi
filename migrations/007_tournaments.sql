-- Klanlar turniri: bitta yutqazishda chiqib ketish (4 yoki 8 jamoa)

CREATE TABLE tournaments (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL CHECK (length(name) BETWEEN 3 AND 40),
    owner_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    size        INT         NOT NULL CHECK (size IN (4, 8)),
    map         TEXT        NOT NULL,
    state       TEXT        NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'running', 'done')),
    winner_clan BIGINT      REFERENCES clans (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    ended_at    TIMESTAMPTZ
);
CREATE INDEX tournaments_recent ON tournaments (id DESC);

CREATE TABLE tournament_teams (
    tournament_id BIGINT NOT NULL REFERENCES tournaments (id) ON DELETE CASCADE,
    clan_id       BIGINT NOT NULL REFERENCES clans (id) ON DELETE CASCADE,
    seed          INT,
    PRIMARY KEY (tournament_id, clan_id)
);

CREATE TABLE tournament_matches (
    tournament_id BIGINT  NOT NULL REFERENCES tournaments (id) ON DELETE CASCADE,
    round         INT     NOT NULL,          -- 1 — birinchi bosqich
    slot          INT     NOT NULL,          -- bosqich ichidagi o'rni (0 dan)
    clan_a        BIGINT  REFERENCES clans (id) ON DELETE SET NULL,
    clan_b        BIGINT  REFERENCES clans (id) ON DELETE SET NULL,
    empty_a       BOOLEAN NOT NULL DEFAULT false,   -- bu tomonga hech kim kelmaydi (bo'sh o'rin)
    empty_b       BOOLEAN NOT NULL DEFAULT false,
    winner        BIGINT  REFERENCES clans (id) ON DELETE SET NULL,
    score_a       INT,
    score_b       INT,
    PRIMARY KEY (tournament_id, round, slot)
);
