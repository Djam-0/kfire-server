package riotsync

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

// This is the SECOND source of a live League state. The first one, in live.go,
// is Riot's Spectator API pulled by the server; this one is Riot's local API on
// the member's own machine, pushed by the desktop client like Rocket League and
// Hearthstone. Same game, same slug, same card: only the richness differs, so
// the reporter lives next to the poller rather than in a package of its own.

// championName bounds the one string a client may push. It is rebroadcast
// verbatim to every browser in the guild, so bounding the alphabet AND the
// length is what makes a tag or a script structurally impossible here, exactly
// as heroCardID does for Hearthstone.
//
// The alphabet is what Riot's own roster needs and nothing more: letters and
// digits for names like "K'Sante" or "Renata Glasc", an apostrophe for
// "Kai'Sa", a period for "Dr. Mundo", an ampersand and spaces for
// "Nunu & Willump". The longest champion name today is fourteen characters;
// thirty-two leaves room for a roster that keeps growing without leaving room
// for a sentence.
var championName = regexp.MustCompile(`^[A-Za-z0-9 '.&-]{1,32}$`)

// maxChampionLevel is a real game rule rather than a guard rail: a champion
// caps at eighteen, and a client claiming more is not reporting League.
const maxChampionLevel = 18

// maxKDA bounds kills, deaths and assists. Not a game rule, a guard rail: the
// records sit in the low hundreds in the longest games that exist, so a
// thousand refuses only the absurd.
const maxKDA = 999

// maxCreepScore bounds the minions killed. A farming record over an hour-long
// game stays under a thousand; ten thousand refuses only the absurd.
const maxCreepScore = 9999

// maxGold bounds the gold figure. A whole team's earned gold over a very long
// game stays well under a hundred thousand, so this bounds one player's with
// an order of magnitude to spare.
const maxGold = 999999

// maxGameTimeSeconds bounds the in-game clock at ten hours. The longest
// professional game on record is a little over ninety minutes; this is not a
// prediction of how long a game can run, it is the point past which the number
// is certainly wrong.
const maxGameTimeSeconds = 36000

// livePayload is the current state of a League game as the member's own machine
// sees it, broadcast and NEVER written.
//
// Riot's local API names the TEN participants of the game, their summoner
// names, their runes and their items. None of that has any business leaving the
// member's machine: the nine other players never agreed to be broadcast on this
// portal, and the poller in live.go already refuses to relay them. This payload
// therefore carries facts about the member alone, and the test pins that list
// so it cannot quietly grow.
type livePayload struct {
	// Ended marks the end of the game. Every other field is then ignored.
	Ended           bool   `json:"ended"`
	Champion        string `json:"champion"`
	Level           int    `json:"level"`
	Kills           int    `json:"kills"`
	Deaths          int    `json:"deaths"`
	Assists         int    `json:"assists"`
	CreepScore      int    `json:"creep_score"`
	Gold            int    `json:"gold"`
	GameTimeSeconds int    `json:"game_time_seconds"`
}

// valid reports whether the state deserves to be rebroadcast. It goes out to
// every client in the org, so it is validated with the same rigor as written
// data.
func (p livePayload) valid() bool {
	if p.Ended {
		return true
	}
	if !championName.MatchString(p.Champion) {
		return false
	}
	if p.Level < 1 || p.Level > maxChampionLevel {
		return false
	}
	for _, v := range []struct {
		val, max int
	}{
		{p.Kills, maxKDA},
		{p.Deaths, maxKDA},
		{p.Assists, maxKDA},
		{p.CreepScore, maxCreepScore},
		{p.Gold, maxGold},
		{p.GameTimeSeconds, maxGameTimeSeconds},
	} {
		if v.val < 0 || v.val > v.max {
			return false
		}
	}
	return true
}

// LiveReporter shapes and validates the live game state League's desktop client
// reads off the local Riot API and pushes several times a second. It holds no
// state of its own: Shape is a pure function of its argument, as the
// livestate.Reporter contract requires.
type LiveReporter struct{}

// NewLiveReporter builds the reporter.
func NewLiveReporter() *LiveReporter { return &LiveReporter{} }

// Slug returns the claimed catalog slug. The same one the Spectator poller
// publishes under: one game, one slug, two sources.
func (r *LiveReporter) Slug() string { return liveSlug }

// Shape validates the payload and returns exactly the fields the card renders,
// and nothing else.
func (r *LiveReporter) Shape(raw json.RawMessage) (map[string]any, error) {
	var p livePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, livestate.ErrInvalidLive
	}
	if !p.valid() {
		return nil, livestate.ErrInvalidLive
	}
	return map[string]any{
		"champion":          p.Champion,
		"level":             p.Level,
		"kills":             p.Kills,
		"deaths":            p.Deaths,
		"assists":           p.Assists,
		"creep_score":       p.CreepScore,
		"gold":              p.Gold,
		"game_time_seconds": p.GameTimeSeconds,
	}, nil
}

// TTL uses the package default. Unlike the Spectator poller, which the server
// drives once a minute and which sets its own, this state is pushed by the
// member's client while the game runs.
func (r *LiveReporter) TTL() time.Duration { return 0 }
