package store

import (
	"context"
	"time"
)

// LolMatch is one League match, reduced to facts about the member.
//
// Champion is the only text field and it is Riot's own champion identifier,
// bounded by the table's CHECK. There is deliberately no field for a summoner
// name: not the member's, and not the nine other players'.
type LolMatch struct {
	UserID          string    `json:"user_id"`
	GameID          string    `json:"game_id"`
	MatchID         string    `json:"match_id"`
	Champion        string    `json:"champion"`
	Win             bool      `json:"win"`
	Kills           int       `json:"kills"`
	Deaths          int       `json:"deaths"`
	Assists         int       `json:"assists"`
	QueueID         int       `json:"queue_id"`
	DurationSeconds int       `json:"duration_seconds"`
	PlayedAt        time.Time `json:"played_at"`
}

// LolMemberStats is a member's record for a game, aggregated by the database
// so a page never downloads every match.
type LolMemberStats struct {
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	AvatarURL    *string   `json:"avatar_url"`
	Matches      int       `json:"matches"`
	Wins         int       `json:"wins"`
	Kills        int       `json:"kills"`
	Deaths       int       `json:"deaths"`
	Assists      int       `json:"assists"`
	PlayTime     int       `json:"play_time"` // seconds
	LastPlayedAt time.Time `json:"last_played_at"`
}

// InsertLolMatch writes a match. A match already stored is silently ignored:
// the backfill is interruptible and replays pages it has already walked, and
// the uniqueness on (user_id, match_id) is what makes that free.
func (s *Store) InsertLolMatch(ctx context.Context, m LolMatch) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO lol_matches
			(user_id, game_id, match_id, champion, win,
			 kills, deaths, assists, queue_id, duration_seconds, played_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, match_id) DO NOTHING`,
		m.UserID, m.GameID, m.MatchID, m.Champion, m.Win,
		m.Kills, m.Deaths, m.Assists, m.QueueID, m.DurationSeconds, m.PlayedAt)
	return err
}

// LolStatsByGame aggregates every member's League record for one game.
func (s *Store) LolStatsByGame(ctx context.Context, gameID string) ([]LolMemberStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.avatar_url,
		       count(*)                          AS matches,
		       count(*) FILTER (WHERE m.win)     AS wins,
		       coalesce(sum(m.kills), 0),
		       coalesce(sum(m.deaths), 0),
		       coalesce(sum(m.assists), 0),
		       coalesce(sum(m.duration_seconds), 0),
		       max(m.played_at)
		  FROM lol_matches m
		  JOIN users u ON u.id = m.user_id AND u.banned_at IS NULL
		 WHERE m.game_id = $1
		 GROUP BY u.id, u.username, u.avatar_url`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LolMemberStats
	for rows.Next() {
		var st LolMemberStats
		if err := rows.Scan(&st.UserID, &st.Username, &st.AvatarURL,
			&st.Matches, &st.Wins, &st.Kills, &st.Deaths, &st.Assists,
			&st.PlayTime, &st.LastPlayedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// LolRecentFor returns a member's latest matches, newest first.
func (s *Store) LolRecentFor(ctx context.Context, userID, gameID string, limit int) ([]LolMatch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, game_id, match_id, champion, win,
		       kills, deaths, assists, queue_id, duration_seconds, played_at
		  FROM lol_matches
		 WHERE user_id = $1 AND game_id = $2
		 ORDER BY played_at DESC
		 LIMIT $3`, userID, gameID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LolMatch
	for rows.Next() {
		var m LolMatch
		if err := rows.Scan(&m.UserID, &m.GameID, &m.MatchID, &m.Champion, &m.Win,
			&m.Kills, &m.Deaths, &m.Assists, &m.QueueID, &m.DurationSeconds,
			&m.PlayedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LolBackfillCursor returns how far back a member's history has been walked.
// A nil instant means the backfill has not started.
func (s *Store) LolBackfillCursor(ctx context.Context, userID string) (*time.Time, bool, error) {
	var before *time.Time
	var done bool
	err := s.pool.QueryRow(ctx,
		`SELECT backfill_before, backfill_done FROM riot_accounts WHERE user_id = $1`,
		userID).Scan(&before, &done)
	if err != nil {
		return nil, false, err
	}
	return before, done, nil
}

// SetLolBackfillCursor records how far back the history has been walked, so an
// interrupted backfill resumes instead of starting over. Without this, a server
// restarted every night would never finish a member's history.
func (s *Store) SetLolBackfillCursor(ctx context.Context, userID string, before *time.Time, done bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE riot_accounts SET backfill_before = $2, backfill_done = $3 WHERE user_id = $1`,
		userID, before, done)
	return err
}

// LolBackfillPending returns the linked members whose history is not fully
// walked yet, oldest cursor first so nobody is starved.
func (s *Store) LolBackfillPending(ctx context.Context, limit int) ([]RiotPlayer, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT la.user_id, la.provider_user_id, ra.platform
		  FROM riot_accounts ra
		  JOIN linked_accounts la
		    ON la.user_id = ra.user_id AND la.provider = 'riot'
		  JOIN users u ON u.id = ra.user_id AND u.banned_at IS NULL
		 WHERE ra.backfill_done = false
		 ORDER BY ra.backfill_before ASC NULLS FIRST
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RiotPlayer
	for rows.Next() {
		var p RiotPlayer
		if err := rows.Scan(&p.UserID, &p.PUUID, &p.Platform); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
