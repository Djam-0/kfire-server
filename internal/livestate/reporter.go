// Package livestate routes a live game state, reported by a desktop client, to
// the code that knows how to read that game.
//
// It is to the live state what internal/matchrecord is to a finished match: the
// hub resolves a slug and delegates, and knows no game itself. The two are kept
// apart on purpose. A match result is history, queued, replayed and written; a
// live state is broadcast and forgotten, so the guarantees they need are not the
// same.
package livestate

import (
	"encoding/json"
	"errors"
	"regexp"
)

// ErrUnknownGame is returned when no reporter claims the slug.
var ErrUnknownGame = errors.New("no live reporter for this game")

// ErrInvalidLive is returned for anything not worth broadcasting.
var ErrInvalidLive = errors.New("invalid live state")

// MaxPayload bounds the raw JSON of one live state.
//
// This is the ONLY message in the feature whose content is relayed to other
// members, and payloads got richer when three games joined. The largest of them,
// League of Legends, fits in a few hundred bytes; this leaves ten times the room
// while making it impossible for a hostile or broken client to flood the guild's
// browsers.
const MaxPayload = 4 << 10

// slugPattern is the shape of a catalog slug. Bounding the length AND the
// alphabet is what makes an HTML tag structurally impossible in the one string
// that reaches other people's screens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Reporter validates and shapes one game's live state.
type Reporter interface {
	// Slug is the catalog slug this reporter claims.
	Slug() string

	// Shape validates the game-specific payload and returns what will be
	// broadcast, or ErrInvalidLive. What it returns IS what browsers see: a
	// reporter MUST broadcast no more than it validated. The registry does
	// not take that on trust: it measures what comes back and enforces the
	// same MaxPayload bound on it.
	//
	// MUST be safe for concurrent use, and is best written as a pure function
	// of raw. It is called from every member's connection at once, several
	// times a second. A reporter holding mutable state shared between calls
	// could let one member's fields land in another member's broadcast, and
	// nothing here would catch it: the result would still be short, valid JSON
	// under a slug the registry itself controls. That is the one hole this
	// package cannot close for you, so it is stated rather than assumed.
	Shape(raw json.RawMessage) (map[string]any, error)
}

// State is one validated live state, ready to broadcast.
type State struct {
	// Slug is the game it belongs to; the browser picks its rendering from it.
	Slug string
	// Ended marks the end of a match. Match is then nil.
	Ended bool
	// Match is what the reporter validated, or nil when Ended.
	Match map[string]any
}

// envelope is all the registry itself needs to understand.
type envelope struct {
	GameSlug string `json:"game_slug"`
	Ended    bool   `json:"ended"`
}

// Registry resolves a slug to its reporter.
//
// Built once at startup and read afterwards, so it needs no lock: same contract
// as the recorder registry, and for the same reason.
type Registry struct {
	bySlug map[string]Reporter
}

// NewRegistry builds a registry from the given reporters.
func NewRegistry(reporters ...Reporter) *Registry {
	m := make(map[string]Reporter, len(reporters))
	for _, r := range reporters {
		m[r.Slug()] = r
	}
	return &Registry{bySlug: m}
}

// Shape validates the common envelope, then hands the payload to the reporter
// claiming its slug.
func (r *Registry) Shape(raw json.RawMessage) (State, error) {
	if len(raw) > MaxPayload {
		return State{}, ErrInvalidLive
	}
	var e envelope
	if err := json.Unmarshal(raw, &e); err != nil {
		return State{}, ErrInvalidLive
	}
	if !slugPattern.MatchString(e.GameSlug) {
		return State{}, ErrInvalidLive
	}

	rep, ok := r.bySlug[e.GameSlug]
	if !ok {
		return State{}, ErrUnknownGame
	}
	if e.Ended {
		// The reporter is resolved but never consulted: a match ending is the
		// same fact in every game, and letting each one re-implement it would
		// be a way for one of them to get it wrong. Resolving it anyway keeps
		// the two paths consistent, and a game nobody claims can have no live
		// entry to end.
		return State{Slug: e.GameSlug, Ended: true}, nil
	}
	match, err := rep.Shape(raw)
	if err != nil {
		return State{}, err
	}
	// The cap is applied to the reporter's OUTPUT too, not only to the client's
	// input. What comes back is broadcast verbatim to every browser in the org,
	// and a reporter that amplifies its input, keeps state across calls or
	// simply has a bug would otherwise decide that size on its own. Measuring
	// costs a marshal of a few hundred bytes twice a second; trusting every
	// future reporter costs a guarantee.
	out, err := json.Marshal(match)
	if err != nil || len(out) > MaxPayload {
		return State{}, ErrInvalidLive
	}
	return State{Slug: e.GameSlug, Match: match}, nil
}
