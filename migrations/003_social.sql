-- Do'stlar va klanlar

CREATE TABLE friends (
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    friend_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    accepted    BOOLEAN     NOT NULL DEFAULT false,   -- false: user_id do'stlik so'radi, javob kutilmoqda
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, friend_id),
    CHECK (user_id <> friend_id)
);
CREATE INDEX friends_friend ON friends (friend_id);

CREATE TABLE clans (
    id          BIGSERIAL   PRIMARY KEY,
    name        TEXT        NOT NULL CHECK (length(name) BETWEEN 3 AND 24),
    tag         TEXT        NOT NULL CHECK (tag ~ '^[A-Za-z0-9]{2,5}$'),
    owner_id    BIGINT      NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX clans_name_lower ON clans (lower(name));
CREATE UNIQUE INDEX clans_tag_lower ON clans (lower(tag));

CREATE TABLE clan_members (
    user_id     BIGINT      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,  -- bir kishi — bitta klan
    clan_id     BIGINT      NOT NULL REFERENCES clans (id) ON DELETE CASCADE,
    owner       BOOLEAN     NOT NULL DEFAULT false,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX clan_members_clan ON clan_members (clan_id);

CREATE TABLE clan_invites (
    clan_id     BIGINT      NOT NULL REFERENCES clans (id) ON DELETE CASCADE,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    from_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (clan_id, user_id)
);
