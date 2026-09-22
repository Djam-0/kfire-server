// Package pubg reads PUBG match history with the server's API key.
//
// The point of this connector is retention, not statistics: PUBG's API keeps a
// match for 14 days and then deletes it, publisher included. Whatever is not
// collected inside that fortnight is gone for everyone, forever, so the
// portal's own table is the only place that history survives.
//
// The API carries no live data of any kind, so there is nothing here to poll
// for a match in progress. That is an impossibility, not a postponement.
//
// Docs: https://documentation.pubg.com/
package pubg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultAPIBase is the single, unsharded host. Unlike Riot, PUBG puts the
// shard in the path rather than in the hostname.
const defaultAPIBase = "https://api.pubg.com"

// defaultRate is how many calls per second the connector allows itself, for
// the calls that are actually metered.
//
// The published ceiling is 10 requests per minute, which is 0.167 per second.
// We aim under it rather than at it, as for Riot: a 429 costs more than being
// slow, and the metered work is tiny anyway (one player lookup per linked
// member per day).
const defaultRate = 0.125

// maxRetries bounds how many times a 429 is retried before giving up. The
// quota is small enough that a saturated key must be reported rather than
// retried forever behind a member's request.
const maxRetries = 3

// countsAgainstQuota reports whether a path is metered by the publisher.
//
// PUBG's rate limit covers the player, season and sample endpoints, but
// explicitly NOT the match and telemetry endpoints, and the responses confirm
// it: a match reply carries no X-RateLimit-* header at all, while a player
// reply does and decrements its remaining count.
//
// This predicate exists because pacing every call blindly would be the natural
// thing to write and would be badly wrong: a member's history is dozens of
// match reads for one player lookup, so spacing the free calls at the metered
// rate would make a sync roughly a hundred times slower for no reason.
func countsAgainstQuota(path string) bool {
	return !strings.Contains(path, "/matches") && !strings.Contains(path, "/telemetry")
}

// Connector talks to PUBG. APIBase is overridable for tests.
type Connector struct {
	APIKey  string
	APIBase string
	HTTP    *http.Client

	// limiter paces the metered calls. It lives on the connector, not on a
	// caller, because the quota belongs to the key: the daily sync and a
	// member linking their account from the account page share one budget.
	limiter *limiter
}

// New returns a connector. It is disabled until apiKey is set.
func New(apiKey string) *Connector {
	return &Connector{
		APIKey:  apiKey,
		APIBase: defaultAPIBase,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		limiter: newLimiter(defaultRate),
	}
}

// SetRate overrides the call rate. For tests only: production keeps the
// conservative default, because the quota is ten a minute and nothing else.
func (c *Connector) SetRate(perSecond float64) { c.limiter = newLimiter(perSecond) }

// Enabled reports whether the API key is configured. Without one the whole
// connector stays dark, and the account page hides its card.
func (c *Connector) Enabled() bool { return c.APIKey != "" }

// APIError is a non-200 answer from PUBG. Callers branch on Status: a 404 is a
// normal answer on the player route, where it simply means the name was typed
// wrong or belongs to another platform.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pubg: %s: status %d: %s", e.Path, e.Status, e.Body)
}

// asAPIError is errors.As specialised to *APIError.
func asAPIError(err error, target **APIError) bool { return errors.As(err, target) }

// NotFound reports whether err is a PUBG 404.
func NotFound(err error) bool {
	var e *APIError
	if !asAPIError(err, &e) {
		return false
	}
	return e.Status == http.StatusNotFound
}

// Transient reports whether an error is worth trying again later.
//
// A caller walking a member's history must tell the two apart: treating a rate
// limit like a deleted match would skip that match forever, and with a 14-day
// retention "later" does not exist. There is no second chance to fetch it.
func Transient(err error) bool {
	if err == nil {
		return false
	}
	var e *APIError
	if asAPIError(err, &e) {
		return e.Status == http.StatusTooManyRequests || e.Status >= 500
	}
	// Not an answer from PUBG at all: a dial failure, a timeout, a cancelled
	// context. None of those say anything about the match itself.
	return true
}

// resetAfter reads how long to wait after a 429.
//
// PUBG does not send Retry-After. It sends X-RateLimit-Reset, an absolute UNIX
// timestamp, so the delay has to be computed against the clock rather than
// read off. A missing or already-past value falls back to nothing extra: the
// limiter's own spacing still applies on the next attempt.
func resetAfter(res *http.Response, now time.Time) time.Duration {
	v := res.Header.Get("X-RateLimit-Reset")
	if v == "" {
		return time.Second
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Second
	}
	d := time.Unix(n, 0).Sub(now)
	if d < 0 {
		return 0
	}
	// A quota window is a minute; anything claiming much more than that is a
	// clock disagreement, not a real wait, and must not stall a sync for hours.
	if d > 2*time.Minute {
		return 2 * time.Minute
	}
	return d
}

// get performs an authenticated GET and decodes the body into out.
func (c *Connector) get(path string, out any) error {
	ctx := context.Background()
	var last error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.attempt(ctx, path, out)
		var apiErr *APIError
		if asAPIError(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests {
			// A 429 means "later", not "no", so it is retried.
			last = err
			continue
		}
		return err
	}
	return last
}

// attempt performs one request. It is split out of get so the deferred close
// of the body is scoped to a single try, rather than piling up until the retry
// loop happens to return.
func (c *Connector) attempt(ctx context.Context, path string, out any) error {
	if c.limiter != nil && countsAgainstQuota(path) {
		if err := c.limiter.wait(ctx); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	// PUBG answers 415 without this one on some routes.
	req.Header.Set("Accept", "application/vnd.api+json")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusTooManyRequests && c.limiter != nil {
		// Slow EVERY caller down, not just this goroutine: the quota belongs
		// to the key, so one rejected call is everyone's problem.
		c.limiter.penalise(resetAfter(res, time.Now()))
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return &APIError{Status: res.StatusCode, Path: path, Body: string(body)}
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// limiter paces the metered calls and absorbs a 429.
//
// Like Riot's, it spaces calls rather than handing out a bucket of tokens: a
// bucket lets a burst through, and a burst against a ten-a-minute quota is
// precisely what earns a 429.
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// newLimiter builds a limiter allowing rate calls per second.
func newLimiter(rate float64) *limiter {
	if rate <= 0 {
		rate = defaultRate
	}
	return &limiter{interval: time.Duration(float64(time.Second) / rate)}
}

// wait blocks until the caller may issue its request, or until ctx is done.
//
// The slot is reserved BEFORE sleeping, so concurrent callers queue up behind
// one another instead of all waking at the same instant.
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	slot := l.next
	if slot.Before(now) {
		slot = now
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	delay := time.Until(slot)
	if delay <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// penalise pushes every pending caller back by d, after PUBG answered 429.
//
// It moves the shared cursor instead of sleeping in the caller, so one
// rejected call slows everybody down. That is the intent: the quota is per
// key, so a single caller backing off politely while the others keep going
// would not help at all.
func (l *limiter) penalise(d time.Duration) {
	if d <= 0 {
		return
	}
	l.mu.Lock()
	until := time.Now().Add(d)
	if until.After(l.next) {
		l.next = until
	}
	l.mu.Unlock()
}
