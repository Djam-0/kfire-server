// Package riot links Riot Games accounts by a typed Riot ID and reads League
// of Legends data with the server's API key.
//
// The Riot product KFIRE holds only carries an API key, with a personal key's
// rate limits; it has no RSO application, so there is no OAuth flow to prove
// account ownership. Linking instead trusts the Riot ID the member types in,
// the same way public stats sites do, and every later read is authenticated
// by the server-side API key alone.
//
// Docs: https://developer.riotgames.com/apis
package riot

import (
	"errors"
	"net/http"
	"sort"
	"time"
)

const (
	// defaultAPIHostTmpl takes the platform (euw1) or the cluster (europe).
	defaultAPIHostTmpl = "https://%s.api.riotgames.com"
	// accountCluster serves Account-V1. Riot lets any cluster answer for any
	// account and recommends the nearest one; KFIRE is hosted in Europe.
	accountCluster = "europe"
)

// platformToMatchCluster maps a League platform to its Match-V5 regional route.
// Note Match-V5 knows a fourth route, "sea", that Account-V1 does not.
var platformToMatchCluster = map[string]string{
	"euw1": "europe", "eun1": "europe", "tr1": "europe", "ru": "europe", "me1": "europe",
	"na1": "americas", "br1": "americas", "la1": "americas", "la2": "americas",
	"kr": "asia", "jp1": "asia",
	"oc1": "sea", "ph2": "sea", "sg2": "sea", "th2": "sea", "tw2": "sea", "vn2": "sea",
}

// MatchCluster returns the Match-V5 regional route for a platform. An unknown
// platform falls back to europe rather than failing: a wrong cluster yields an
// empty match list, which the caller already handles, while an error would lose
// the rank and mastery blocks too.
func MatchCluster(platform string) string {
	if c, ok := platformToMatchCluster[platform]; ok {
		return c
	}
	return "europe"
}

// KnownPlatforms returns every platform KFIRE accepts, sorted, for the SPA's
// region picker.
func KnownPlatforms() []string {
	out := make([]string, 0, len(platformToMatchCluster))
	for p := range platformToMatchCluster {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Connector talks to Riot. APIHostTmpl is overridable for tests.
type Connector struct {
	APIKey      string
	APIHostTmpl string
	HTTP        *http.Client

	// limiter paces every call. It lives on the connector, not on a caller, so
	// the hourly refresh, the live poller and the backfill share ONE quota,
	// which is the only thing Riot actually meters.
	limiter *limiter
}

// New returns a connector. It is disabled until apiKey is set.
func New(apiKey string) *Connector {
	return &Connector{
		APIKey:      apiKey,
		APIHostTmpl: defaultAPIHostTmpl,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
		limiter:     newLimiter(defaultRate),
	}
}

// SetRate overrides the call rate. For tests only: production keeps the
// conservative default, because a throttled key breaks every League surface.
func (c *Connector) SetRate(perSecond float64) { c.limiter = newLimiter(perSecond) }

// Enabled reports whether the API key is configured.
func (c *Connector) Enabled() bool {
	return c.APIKey != ""
}

// asAPIError is errors.As specialised to *APIError.
func asAPIError(err error, target **APIError) bool { return errors.As(err, target) }

// ErrNoParticipant means a match exists but does not list the member. It is a
// permanent answer, not a hiccup: the match will never start listing them.
var ErrNoParticipant = errors.New("riot: match has no such participant")

// Transient reports whether an error is worth trying again later.
//
// The distinction matters to anything walking a history: a caller that treats
// a rate limit like a missing match skips it FOREVER, leaving a hole nobody
// can see afterwards, because a cursor that moved past it never comes back.
//
// Transient: a rate limit that survived the retries in get, a server-side
// failure, a network or context error. Permanent: any other 4xx, and a match
// that does not list the member.
func Transient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNoParticipant) {
		return false
	}
	var e *APIError
	if asAPIError(err, &e) {
		return e.Status == http.StatusTooManyRequests || e.Status >= 500
	}
	// Not an answer from Riot at all: a dial failure, a timeout, a cancelled
	// context. None of those say anything about the match itself.
	return true
}
