-- 0037: League of Legends matches, and the backfill cursor.
--
-- Until now the only League history was five matches inside the JSON blob
-- riot_game_profile.data, REWRITTEN WHOLE on every hourly refresh. Nothing
-- accumulated: a member playing twenty games in an evening kept five, and last
-- week was gone for good. Every League statistic was therefore computed over a
-- five-match sample, which measures nothing.
--
-- Unlike Hearthstone and Rocket League, this table is fed by the SERVER from
-- Riot's own API, not by the desktop client. The privacy guarantee is the same
-- and it is STRUCTURAL: there is no free-text column. champion is Riot's own
-- champion identifier, bounded by a regexp exactly like hero_card_id in 0034,
-- and there is no column capable of holding a summoner name -- neither the
-- member's nor the nine other players'.
--
-- The uniqueness is (user_id, match_id): Riot's match id is stable, so
-- re-inserting a match the backfill already walked is a no-op. That is what
-- makes the backfill safe to interrupt and replay.
--
-- backfill_before is the cursor: matches OLDER than this instant are still to
-- be fetched. NULL means the backfill has not started; backfill_done marks a
-- member whose history has been walked to its end, after which only new
-- matches are added.

BEGIN;

CREATE TABLE lol_matches (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id          uuid        NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    match_id         text        NOT NULL CHECK (match_id ~ '^[A-Z0-9]{2,8}_[0-9]{1,20}$'),
    champion         text        NOT NULL CHECK (champion ~ '^[A-Za-z0-9._''-]{1,64}$'),
    win              boolean     NOT NULL,
    kills            int         NOT NULL CHECK (kills >= 0),
    deaths           int         NOT NULL CHECK (deaths >= 0),
    assists          int         NOT NULL CHECK (assists >= 0),
    queue_id         int         NOT NULL CHECK (queue_id >= 0),
    duration_seconds int         NOT NULL CHECK (duration_seconds BETWEEN 0 AND 21600),
    played_at        timestamptz NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, match_id)
);

CREATE INDEX lol_matches_game_idx ON lol_matches (game_id, played_at DESC);
CREATE INDEX lol_matches_user_idx ON lol_matches (user_id, played_at DESC);

ALTER TABLE riot_accounts
    ADD COLUMN backfill_before timestamptz,
    ADD COLUMN backfill_done   boolean NOT NULL DEFAULT false;

COMMIT;
