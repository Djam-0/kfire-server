package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PubgMatch is one played match, reduced to facts about the member.
//
// GameMode and MapName are the only text fields and both are PUBG's own
// bounded identifiers, guarded by the table's CHECK. There is deliberately no
// field able to hold a player name: not the member's, and not the ninety-nine
// others the API names in the same response.
type PubgMatch struct {
	UserID        string    `json:"user_id"`
	GameID        string    `json:"game_id"`
	MatchID       string    `json:"match_id"`
	GameMode      string    `json:"game_mode"`
	MapName       string    `json:"map_name"`
	WinPlace      int       `json:"win_place"`
	Kills         int       `json:"kills"`
	Assists       int       `json:"assists"`
	HeadshotKills int       `json:"headshot_kills"`
	Revives       int       `json:"revives"`
	DamageDealt   float64   `json:"damage_dealt"`
	TimeSurvived  int       `json:"time_survived"`
	DurationSecs  int       `json:"duration_secs"`
	PlayedAt      time.Time `json:"played_at"`
}

// PubgMemberStats is a member's record for a game, aggregated by the database
// so a page never downloads every match.
type PubgMemberStats struct {
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	AvatarURL    *string   `json:"avatar_url"`
	Matches      int       `json:"matches"`
	Wins         int       `json:"wins"`
	TopTens      int       `json:"top_tens"`
	Kills        int       `json:"kills"`
	DamageDealt  float64   `json:"damage_dealt"`
	BestPlace    int       `json:"best_place"`
	TimeSurvived int       `json:"time_survived"` // seconds
	LastPlayedAt time.Time `json:"last_played_at"`
}

// PubgPlayer is a member with a linked PUBG account, with what the daily sync
// needs to query the API: the account id, and the shard that serves it.
type PubgPlayer struct {
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id"`
	Platform  string `json:"platform"`
}

// InsertPubgMatch writes a match. A match already stored is silently ignored,
// which is what makes a sync pass replayable without counting anything twice.
func (s *Store) InsertPubgMatch(ctx context.Context, m PubgMatch) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pubg_matches
			(user_id, game_id, match_id, game_mode, map_name, win_place,
			 kills, assists, headshot_kills, revives, damage_dealt,
			 time_survived, duration_secs, played_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (user_id, match_id) DO NOTHING`,
		m.UserID, m.GameID, m.MatchID, m.GameMode, m.MapName, m.WinPlace,
		m.Kills, m.Assists, m.HeadshotKills, m.Revives, m.DamageDealt,
		m.TimeSurvived, m.DurationSecs, m.PlayedAt)
	return err
}

// PubgStatsByGame aggregates every member's PUBG record for one game. Banned
// members are excluded, like every other aggregate here.
func (s *Store) PubgStatsByGame(ctx context.Context, gameID string) ([]PubgMemberStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.avatar_url,
		       count(*)                                       AS matches,
		       count(*) FILTER (WHERE m.win_place = 1)        AS wins,
		       count(*) FILTER (WHERE m.win_place <= 10)      AS top_tens,
		       coalesce(sum(m.kills), 0),
		       coalesce(sum(m.damage_dealt), 0)::float8,
		       min(m.win_place),
		       coalesce(sum(m.time_survived), 0),
		       max(m.played_at)
		  FROM pubg_matches m
		  JOIN users u ON u.id = m.user_id AND u.banned_at IS NULL
		 WHERE m.game_id = $1
		 GROUP BY u.id, u.username, u.avatar_url`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PubgMemberStats
	for rows.Next() {
		var st PubgMemberStats
		if err := rows.Scan(&st.UserID, &st.Username, &st.AvatarURL,
			&st.Matches, &st.Wins, &st.TopTens, &st.Kills, &st.DamageDealt,
			&st.BestPlace, &st.TimeSurvived, &st.LastPlayedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// PubgRecentFor returns a member's latest matches, newest first.
func (s *Store) PubgRecentFor(ctx context.Context, userID, gameID string, limit int) ([]PubgMatch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, game_id, match_id, game_mode, map_name, win_place,
		       kills, assists, headshot_kills, revives, damage_dealt::float8,
		       time_survived, duration_secs, played_at
		  FROM pubg_matches
		 WHERE user_id = $1 AND game_id = $2
		 ORDER BY played_at DESC
		 LIMIT $3`, userID, gameID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PubgMatch
	for rows.Next() {
		var m PubgMatch
		if err := rows.Scan(&m.UserID, &m.GameID, &m.MatchID, &m.GameMode, &m.MapName,
			&m.WinPlace, &m.Kills, &m.Assists, &m.HeadshotKills, &m.Revives,
			&m.DamageDealt, &m.TimeSurvived, &m.DurationSecs, &m.PlayedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PubgAccountFor returns a member's shard and account id, or ErrNotFound when
// they have not linked an account. Both halves are needed together: the API is
// sharded, so an account id alone queries nothing.
func (s *Store) PubgAccountFor(ctx context.Context, userID string) (platform, accountID string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT pa.platform, la.provider_user_id
		  FROM pubg_accounts pa
		  JOIN linked_accounts la
		    ON la.user_id = pa.user_id AND la.provider = 'pubg'
		 WHERE pa.user_id = $1`, userID).Scan(&platform, &accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return platform, accountID, err
}

// UpsertPubgAccount records which shard serves a member. The identity itself
// lives in linked_accounts, written by the caller in the same request.
func (s *Store) UpsertPubgAccount(ctx context.Context, userID, platform string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pubg_accounts (user_id, platform, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET
			platform   = EXCLUDED.platform,
			updated_at = now()`,
		userID, platform)
	return err
}

// DeletePubgAccount removes a member's shard. The stored matches are kept on
// purpose: the publisher deletes them after 14 days, so unlinking would
// otherwise destroy history nothing can ever fetch again.
func (s *Store) DeletePubgAccount(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM pubg_accounts WHERE user_id = $1`, userID)
	return err
}

// PubgLinkedMembers returns every member the daily sync should refresh.
func (s *Store) PubgLinkedMembers(ctx context.Context) ([]PubgPlayer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pa.user_id, la.provider_user_id, pa.platform
		  FROM pubg_accounts pa
		  JOIN linked_accounts la
		    ON la.user_id = pa.user_id AND la.provider = 'pubg'
		  JOIN users u ON u.id = pa.user_id AND u.banned_at IS NULL
		 ORDER BY pa.updated_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PubgPlayer
	for rows.Next() {
		var p PubgPlayer
		if err := rows.Scan(&p.UserID, &p.AccountID, &p.Platform); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// pubgKnownWindow bounds how far back known match ids are read.
//
// The API never lists a match older than 14 days, so anything older can never
// come back as a candidate. Reading the whole table instead would grow forever
// for no gain, and this table is meant to grow forever: it is the only copy.
const pubgKnownWindow = "30 days"

// PubgKnownMatchIDs returns the ids already stored for a member, so a sync
// pass can skip them. Reading a match is free against the quota but not free
// in time, and a stored match cannot have changed.
func (s *Store) PubgKnownMatchIDs(ctx context.Context, userID, gameID string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT match_id
		  FROM pubg_matches
		 WHERE user_id = $1 AND game_id = $2
		   AND played_at > now() - $3::interval`, userID, gameID, pubgKnownWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		known[id] = true
	}
	return known, rows.Err()
}
