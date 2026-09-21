package hearthstone

import (
	"encoding/json"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/livestate"
)

// SLUG is the catalog slug this package claims, for the plugin, the recorder
// and the live reporter alike.
const SLUG = "hearthstone"

// The two modes the client may report live. They are the same two the recorder
// accepts for a finished match: a live state that could not become a recorded
// match would describe a game we do not otherwise believe in.
const (
	modeBattlegrounds = "battlegrounds"
	modeConstructed   = "constructed"
)

// maxTurn bounds the turn counter. Hearthstone has no hard limit, but no real
// game reaches a hundred turns: Battlegrounds ends around twenty, and even a
// constructed game milled into fatigue finishes far below. The bound is not a
// game rule, it is what stops a hostile client from broadcasting an absurd
// figure to every browser in the guild.
const maxTurn = 100

// maxPlacement is the size of a Battlegrounds lobby, hence the worst place
// there is.
const maxPlacement = 8

// livePayload is the current state of a game, broadcast and NEVER written.
//
// It is deliberately this short. Hearthstone's log names the opponent and lists
// every card played by both sides; none of that has any business leaving the
// member's machine, not even for a display. A mode, a turn and a place are the
// whole of what other members get to see.
type livePayload struct {
	// Ended marks the end of the game. Every other field is then ignored.
	Ended bool   `json:"ended"`
	Mode  string `json:"mode"`
	Turn  int    `json:"turn"`
	// Placement is the current place in a Battlegrounds lobby. A pointer
	// because absent and first place are different facts, and constructed
	// games have no place at all.
	Placement *int `json:"placement"`
}

// valid reports whether the state deserves to be rebroadcast. It goes out to
// every client in the org, so it is validated with the same rigor as written
// data.
func (p livePayload) valid() bool {
	if p.Ended {
		return true
	}
	// The mode string is rebroadcast verbatim and every browser in the guild
	// renders it. Accepting only the two known values is what makes it
	// impossible for this field to carry anything else, whatever the client
	// believes it is sending.
	if p.Mode != modeBattlegrounds && p.Mode != modeConstructed {
		return false
	}
	if p.Turn < 1 || p.Turn > maxTurn {
		return false
	}
	if p.Placement != nil {
		// A place outside a lobby, or a place in a game that has no lobby, is
		// refused rather than dropped: it means the client does not know what
		// it is sending, and the rest of the same payload deserves no more
		// trust than that field.
		if p.Mode != modeBattlegrounds {
			return false
		}
		if *p.Placement < 1 || *p.Placement > maxPlacement {
			return false
		}
	}
	return true
}

// LiveReporter shapes and validates the live game state Hearthstone's desktop
// client pushes. It holds no state of its own: Shape is a pure function of its
// argument, as the livestate.Reporter contract requires.
type LiveReporter struct{}

// NewLiveReporter builds the reporter.
func NewLiveReporter() *LiveReporter { return &LiveReporter{} }

// Slug returns the claimed catalog slug.
func (r *LiveReporter) Slug() string { return SLUG }

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
	out := map[string]any{
		"mode": p.Mode,
		"turn": p.Turn,
	}
	// Absent stays absent: a constructed game must not carry a placement key
	// at all, so the card has nothing to render rather than a zero to hide.
	if p.Placement != nil {
		out["placement"] = *p.Placement
	}
	return out, nil
}

// liveTTL is how long a Hearthstone state stays trustworthy without a fresh
// sample.
//
// Generous on purpose. The desktop client re-reads the game's log every five
// seconds and only sends when something changed, and in Battlegrounds nothing
// changes during the shopping and combat phases: a turn can legitimately hold
// for a minute. With the package default, sized for a client pushing twice a
// second, the card vanished from the guild's live page mid-game and came back
// at the next turn. A member reported exactly that.
//
// Two minutes is longer than any silence a real game produces, and still short
// enough that a client which crashes or is closed stops being shown quickly.
const liveTTL = 2 * time.Minute

// TTL reports how long this game's state survives without a new sample.
func (r *LiveReporter) TTL() time.Duration { return liveTTL }
