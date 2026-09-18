package ws

import (
	"time"
)

// liveTTL is how long a match state with no new sample is considered
// finished. The client emits at 2 Hz, so 15 seconds leaves comfortable
// margin for a connection that hiccups.
const liveTTL = 15 * time.Second

// liveSource says which of the two kinds of source produced an entry.
//
// A game can now be reported by both at once: League of Legends is pulled from
// Riot's Spectator API by a server-side poller AND pushed by the member's own
// machine, which reads Riot's local API. They describe the same game with very
// different richness and at very different rhythms, one sample a minute against
// several a second, so the hub has to know which one an entry came from in
// order to arbitrate between them. A bool would have answered today's question;
// a named type says what the two values mean.
type liveSource uint8

const (
	// sourceClient is a state PUSHED by the member's own machine over the
	// socket. It is the richer and the faster of the two, and it is the zero
	// value only because a typed constant must have one: every call site names
	// its source explicitly.
	sourceClient liveSource = iota
	// sourceServer is a state PULLED by one of our pollers from a third-party
	// API. It is the fallback: it covers a member who has no KFIRE client
	// running, or one whose client is too old to report this game.
	sourceServer
)

// liveEntry is the state kept in memory for one member.
type liveEntry struct {
	// slug is the game it belongs to; the browser picks its rendering from it.
	slug      string
	match     map[string]any
	updatedAt time.Time
	// source is where this state came from. See setLive for the rule it
	// arbitrates.
	source liveSource
	// ttl is how long this entry survives without a fresh sample. Zero means
	// liveTTL: the source sets it, because only the source knows its own
	// rhythm.
	ttl time.Duration
}

// expired reports whether the entry has gone without a new sample long
// enough to be considered finished.
func (e liveEntry) expired(now time.Time) bool {
	ttl := e.ttl
	if ttl <= 0 {
		ttl = liveTTL
	}
	return now.Sub(e.updatedAt) > ttl
}
