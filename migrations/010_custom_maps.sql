-- Foydalanuvchi xaritalari (xarita muharriri): 44×44 panjara, bir qator matn (custommaps.go)

CREATE TABLE custom_maps (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    style       TEXT        NOT NULL,
    rows        TEXT        NOT NULL,                 -- 1936 belgi: # . c s A B t k
    plays       INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX custom_maps_user ON custom_maps (user_id);
CREATE INDEX custom_maps_plays ON custom_maps (plays DESC, id DESC);
