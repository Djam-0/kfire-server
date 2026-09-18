package ws

import (
	"time"
)

// liveTTL is how long a match state with no new sample is considered
// finished. The client emits at 2 Hz, so 15 seconds leaves comfortable
// margin for a connection that hiccups.
const liveTTL = 15 * time.Second

// liveEntry is the state kept in memory for one member.
type liveEntry struct {
	// slug is the game it belongs to; the browser picks its rendering from it.
	slug      string
	match     map[string]any
	updatedAt time.Time
}

// expired reports whether the entry has gone without a new sample long
// enough to be considered finished.
func (e liveEntry) expired(now time.Time) bool {
	return now.Sub(e.updatedAt) > liveTTL
}
