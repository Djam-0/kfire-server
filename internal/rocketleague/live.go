package rocketleague

import (
	"encoding/json"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

// SLUG is the catalog slug this package claims, for both the recorder and
// the live reporter.
const SLUG = "rocket-league"

// maxMatchScore bounds a team score. Rocket League has no hard limit, but a
// match with more than 99 goals does not exist: the bound stops a hostile
// client from broadcasting anything to the whole guild.
const maxMatchScore = 99

// maxSecondsRemaining bounds the displayed clock, overtime included.
const maxSecondsRemaining = 7200

// maxStat bounds the member's stats. Deliberately generous: this is not a
// game rule, it is a guard rail against an absurd value broadcast to
// everyone. The team scores and the clock were already bounded, these were
// not, for no reason.
const maxStat = 100000

// livePayload is the current state of a match, broadcast and NEVER written.
//
// Like the end-of-match summary, it names no one: two team scores, a clock,
// and the member's own stats. The game's own stream carries the name of
// every player; they never leave its machine, not even for a display.
type livePayload struct {
	// Ended marks the end of the match. Every other field is then ignored.
	Ended            bool `json:"ended"`
	TeamBlueScore    int  `json:"team_blue_score"`
	TeamOrangeScore  int  `json:"team_orange_score"`
	SecondsRemaining int  `json:"seconds_remaining"`
	Overtime         bool `json:"overtime"`
	Goals            int  `json:"goals"`
	Assists          int  `json:"assists"`
	Saves            int  `json:"saves"`
	Shots            int  `json:"shots"`
	Score            int  `json:"score"`
	Demos            int  `json:"demos"`
}

// valid reports whether the state deserves to be rebroadcast. It goes out to
// every client in the org, so it is validated with the same rigor as
// written data.
func (p livePayload) valid() bool {
	if p.Ended {
		return true
	}
	if p.TeamBlueScore < 0 || p.TeamBlueScore > maxMatchScore ||
		p.TeamOrangeScore < 0 || p.TeamOrangeScore > maxMatchScore {
		return false
	}
	if p.SecondsRemaining < 0 || p.SecondsRemaining > maxSecondsRemaining {
		return false
	}
	for _, v := range []int{p.Goals, p.Assists, p.Saves, p.Shots, p.Score, p.Demos} {
		if v < 0 || v > maxStat {
			return false
		}
	}
	return true
}

// LiveReporter shapes and validates the live match state Rocket League's
// desktop client pushes twice a second. It holds no state of its own: Shape
// is a pure function of its argument, as the livestate.Reporter contract
// requires.
type LiveReporter struct{}

// NewLiveReporter builds the reporter.
func NewLiveReporter() *LiveReporter { return &LiveReporter{} }

// Slug returns the claimed catalog slug.
func (r *LiveReporter) Slug() string { return SLUG }

// Shape validates the payload and returns exactly what liveEntryJSON used to
// build: the same fields, in the same shape, so nothing changes for the
// browsers that render it.
func (r *LiveReporter) Shape(raw json.RawMessage) (map[string]any, error) {
	var p livePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, livestate.ErrInvalidLive
	}
	if !p.valid() {
		return nil, livestate.ErrInvalidLive
	}
	return map[string]any{
		"team_blue_score":   p.TeamBlueScore,
		"team_orange_score": p.TeamOrangeScore,
		"seconds_remaining": p.SecondsRemaining,
		"overtime":          p.Overtime,
		"goals":             p.Goals,
		"assists":           p.Assists,
		"saves":             p.Saves,
		"shots":             p.Shots,
		"score":             p.Score,
		"demos":             p.Demos,
	}, nil
}

// TTL uses the package default: this client pushes twice a second, which is
// what that default was sized for.
func (r *LiveReporter) TTL() time.Duration { return 0 }
