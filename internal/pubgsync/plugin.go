package pubgsync

import (
	"context"

	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
	"github.com/knightsofeternity/kfire-server/internal/store"
)

// recentMatches is the number of matches the member page lists.
//
// Ten, like the League and Hearthstone pages already show: past that a list
// reads as a wall of rows rather than as recent form, and the table holds far
// more than anyone wants to scroll.
const recentMatches = 10

// Plugin shows the PUBG history the daily sync collected.
type Plugin struct {
	st   *store.Store
	conn *pubg.Connector
}

// NewPlugin builds the plugin.
func NewPlugin(st *store.Store, conn *pubg.Connector) *Plugin {
	return &Plugin{st: st, conn: conn}
}

func (p *Plugin) ID() string        { return "pubg" }
func (p *Plugin) Name() string      { return "PUBG: BATTLEGROUNDS" }
func (p *Plugin) Connector() string { return "pubg" }
func (p *Plugin) Slugs() []string   { return []string{SLUG} }

// Available follows the key: without it the daily sync never runs, so the
// pages would show a history frozen at whatever the last configured instance
// collected. Same rule as League.
func (p *Plugin) Available() bool { return p.conn != nil && p.conn.Enabled() }

// Refresh does nothing on purpose, and this is not a gap to fill later.
//
// Refresh is called while a member's page is being built, and resolving a
// player is the one PUBG call that counts against a quota of ten requests per
// minute. Wiring it here would let a handful of page views exhaust the day's
// budget and starve the daily pass, which is the only thing standing between
// the guild and the publisher's 14-day deletion.
func (p *Plugin) Refresh(ctx context.Context, userID, gameSlug string) {}

// GameDetail returns the guild's record, one entry per member.
func (p *Plugin) GameDetail(ctx context.Context, _ string, g store.Game) (map[string]any, error) {
	stats, err := p.st.PubgStatsByGame(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	cards := make([]map[string]any, len(stats))
	for i, s := range stats {
		m := map[string]any{
			"user_id": s.UserID, "username": s.Username,
			"matches": s.Matches, "wins": s.Wins, "top_tens": s.TopTens,
			"kills": s.Kills, "damage_dealt": s.DamageDealt,
			"best_place": s.BestPlace, "time_survived": s.TimeSurvived,
			"last_played_at": s.LastPlayedAt,
		}
		if s.AvatarURL != nil {
			m["avatar_url"] = *s.AvatarURL
		}
		cards[i] = m
	}
	return map[string]any{"pubg_players": cards}, nil
}

// UserGameDetail returns a member's block: their latest matches.
//
// The map and the mode travel as PUBG's own identifiers, untranslated. The
// browser turns them into labels, because the identifier is the fact and the
// label is presentation, different per language. Same choice as the Rocket
// League playlist and the Hearthstone hero.
//
// user_id and game_id are left out of each row: the caller already knows both,
// and repeating them ten times would only make the payload bigger.
func (p *Plugin) UserGameDetail(ctx context.Context, targetUserID string, g store.Game) (map[string]any, error) {
	matches, err := p.st.PubgRecentFor(ctx, targetUserID, g.ID, recentMatches)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(matches))
	for i, m := range matches {
		out[i] = map[string]any{
			"match_id": m.MatchID, "game_mode": m.GameMode, "map_name": m.MapName,
			"win_place": m.WinPlace, "kills": m.Kills, "assists": m.Assists,
			"headshot_kills": m.HeadshotKills, "revives": m.Revives,
			"damage_dealt": m.DamageDealt, "time_survived": m.TimeSurvived,
			"duration_secs": m.DurationSecs, "played_at": m.PlayedAt,
		}
	}
	return map[string]any{"pubg_matches": out}, nil
}
