-- 0038: PUBG match history.
--
-- PUBG's API deletes matches after 14 days, publisher included. Nothing older
-- can ever be recovered, so this table is the only place that history survives.
-- That is the point of the feature, not a side effect.
--
-- linked_accounts carries the identity (provider_user_id = the PUBG account id,
-- display_name = the in-game name shown back to the member). Its provider CHECK
-- has to be widened: pubg was not one of the allowed values.
--
-- Routing lives apart, in pubg_accounts, exactly as riot_accounts does for
-- Riot: the API is sharded by platform, so a name alone cannot find anyone.
--
-- The privacy guarantee is STRUCTURAL. A PUBG match has a hundred players and
-- the response names every one of them, teammates included. This table has no
-- column able to hold a name: map and mode are bounded identifiers, everything
-- else is a number.

BEGIN;

ALTER TABLE linked_accounts DROP CONSTRAINT IF EXISTS linked_accounts_provider_check;
ALTER TABLE linked_accounts ADD CONSTRAINT linked_accounts_provider_check
    CHECK (provider IN ('steam', 'battlenet', 'riot', 'epic', 'xbox', 'psn', 'pubg'));

CREATE TABLE pubg_accounts (
    user_id    uuid        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    platform   text        NOT NULL CHECK (platform IN ('steam', 'xbox', 'psn', 'kakao', 'stadia')),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE pubg_matches (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id        uuid        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    match_id       text        NOT NULL CHECK (match_id ~ '^[A-Za-z0-9-]{1,64}$'),
    game_mode      text        NOT NULL CHECK (game_mode ~ '^[a-z0-9-]{1,32}$'),
    map_name       text        NOT NULL CHECK (map_name ~ '^[A-Za-z0-9_]{1,32}$'),
    win_place      int         NOT NULL CHECK (win_place BETWEEN 1 AND 100),
    kills          int         NOT NULL CHECK (kills >= 0),
    assists        int         NOT NULL CHECK (assists >= 0),
    headshot_kills int         NOT NULL CHECK (headshot_kills >= 0),
    revives        int         NOT NULL CHECK (revives >= 0),
    damage_dealt   numeric(8,2) NOT NULL CHECK (damage_dealt >= 0),
    time_survived  int         NOT NULL CHECK (time_survived BETWEEN 0 AND 7200),
    duration_secs  int         NOT NULL CHECK (duration_secs BETWEEN 0 AND 7200),
    played_at      timestamptz NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, match_id)
);

CREATE INDEX pubg_matches_game_idx ON pubg_matches (game_id, played_at DESC);
CREATE INDEX pubg_matches_user_idx ON pubg_matches (user_id, played_at DESC);

COMMIT;
