package pubg

import (
	"errors"
	"net/url"
	"time"
)

// ErrNotPlayable means the response is a real answer about something that is
// not a match: a tutorial, a training session, a custom game. Callers skip it
// without treating it as a failure, and without retrying it either.
var ErrNotPlayable = errors.New("pubg: not a playable match")

// ErrNoParticipant means the match exists but does not list that account. It
// is a permanent answer, not a hiccup: the match will never start listing
// them, so retrying is pointless.
var ErrNoParticipant = errors.New("pubg: match has no such participant")

// playableMatchTypes is a list of what we KEEP, deliberately, and not a list
// of what we reject.
//
// That direction is not a style preference, it was proven: a survey of forty
// real matches on 2026-09-22 turned up trainingroom, a type absent from the
// first survey of six. A reject list would have stored training sessions as
// games, which is exactly what happened on Rocket League. An unknown type must
// therefore be dropped by default, because no sample is an exhaustive list of
// anything.
//
// airoyale is kept, decided on 2026-09-22. It is a real mode people play, and
// the publisher deletes everything after fourteen days, so excluding it would
// lose those games for good. The cost is accepted and worth knowing: its planes
// and its descent make it a different game, so it slightly distorts an average
// placement computed across every mode. Splitting by mode is the way out when
// that starts to matter.
var playableMatchTypes = map[string]bool{
	"official":    true,
	"competitive": true,
	"airoyale":    true,
}

// trainingMaps are not matches whatever the match type says.
var trainingMaps = map[string]bool{"Range_Main": true}

// MatchResult is one played match, reduced to facts about the member.
//
// A PUBG match has a hundred players and the response names all of them, in
// participants and in rosters. None of that is carried here: this struct has
// no field able to hold another player's name.
type MatchResult struct {
	MatchID       string
	GameMode      string
	MapName       string
	MatchType     string
	WinPlace      int
	Kills         int
	Assists       int
	DamageDealt   float64
	TimeSurvived  int
	HeadshotKills int
	Revives       int
	PlayedAt      time.Time
	DurationSecs  int
}

// matchEnvelope is the decoded shape of a match answer.
type matchEnvelope struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			CreatedAt     time.Time `json:"createdAt"`
			Duration      int       `json:"duration"`
			GameMode      string    `json:"gameMode"`
			MapName       string    `json:"mapName"`
			MatchType     string    `json:"matchType"`
			IsCustomMatch bool      `json:"isCustomMatch"`
		} `json:"attributes"`
	} `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		Attributes struct {
			Stats struct {
				// Verified on six real matches on 2026-09-22: damageDealt,
				// longestKill, rideDistance and swimDistance come back as an
				// integer on some matches and as a float on others, with no
				// pattern. Decoding them as int would work on some matches and
				// fail on others, which is worse than failing outright because
				// nobody would look here.
				DamageDealt  float64 `json:"damageDealt"`
				WalkDistance float64 `json:"walkDistance"`
				RideDistance float64 `json:"rideDistance"`
				SwimDistance float64 `json:"swimDistance"`
				LongestKill  float64 `json:"longestKill"`

				Kills         int    `json:"kills"`
				Assists       int    `json:"assists"`
				HeadshotKills int    `json:"headshotKills"`
				Revives       int    `json:"revives"`
				TimeSurvived  int    `json:"timeSurvived"`
				WinPlace      int    `json:"winPlace"`
				PlayerID      string `json:"playerId"`
			} `json:"stats"`
		} `json:"attributes"`
	} `json:"included"`
}

// playable reports whether the match is one worth recording.
func playable(matchType, mapName string, custom bool) bool {
	return playableMatchTypes[matchType] && !trainingMaps[mapName] && !custom
}

// MatchForPlayer reads one match and keeps only that account's participation.
// It returns ErrNotPlayable for a tutorial, a training session or a custom
// game, which callers skip without treating it as a failure.
//
// This call is free: the publisher's quota does not meter the match endpoint,
// which is what makes walking a whole history affordable.
func (c *Connector) MatchForPlayer(platform, matchID, accountID string) (MatchResult, error) {
	path := "/shards/" + url.PathEscape(platform) + "/matches/" + url.PathEscape(matchID)

	var env matchEnvelope
	if err := c.get(path, &env); err != nil {
		return MatchResult{}, err
	}

	a := env.Data.Attributes
	if !playable(a.MatchType, a.MapName, a.IsCustomMatch) {
		return MatchResult{}, ErrNotPlayable
	}

	for _, inc := range env.Included {
		// Rosters and assets share the array, and a roster carries a stats
		// block of its own, so the type has to be checked rather than assumed.
		if inc.Type != "participant" || inc.Attributes.Stats.PlayerID != accountID {
			continue
		}
		s := inc.Attributes.Stats
		return MatchResult{
			MatchID:       env.Data.ID,
			GameMode:      a.GameMode,
			MapName:       a.MapName,
			MatchType:     a.MatchType,
			WinPlace:      s.WinPlace,
			Kills:         s.Kills,
			Assists:       s.Assists,
			DamageDealt:   s.DamageDealt,
			TimeSurvived:  s.TimeSurvived,
			HeadshotKills: s.HeadshotKills,
			Revives:       s.Revives,
			PlayedAt:      a.CreatedAt,
			DurationSecs:  a.Duration,
		}, nil
	}
	return MatchResult{}, ErrNoParticipant
}
