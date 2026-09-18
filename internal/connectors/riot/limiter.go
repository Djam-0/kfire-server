package riot

import (
	"context"
	"sync"
	"time"
)

// defaultRate is how many calls per second the connector allows itself.
//
// KFIRE holds a personal Riot key, whose published ceiling is 20 calls per
// second and 100 per two minutes. The two-minute window is the binding one:
// sustained, it works out below one call per second. We aim under it rather
// than at it, because being throttled by Riot costs far more than being slow:
// a 429 hits every League surface at once, including the live poller.
const defaultRate = 0.8

// limiter paces calls to Riot and absorbs a 429.
//
// It is deliberately a simple spacing limiter rather than a token bucket: a
// bucket lets a burst through, and a burst is exactly what gets a key
// throttled. Every caller waits its turn, including the backfill's parallel
// detail fetches.
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

// penalise pushes every pending caller back by d, after Riot answered 429.
//
// It moves the shared cursor rather than sleeping in the caller, so ONE
// rejected call slows everybody down. That is the point: the quota is per key,
// not per goroutine, so a single caller retrying politely while the others
// keep hammering would not help.
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
