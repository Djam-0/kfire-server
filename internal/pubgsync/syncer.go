// Package pubgsync collects the guild's PUBG matches before the publisher
// forgets them.
//
// PUBG's API keeps a match for 14 days and then deletes it, publisher
// included. Every day without a pass is a day of history that nobody, ever,
// will be able to fetch again. That is why this loop exists, and why it is
// careful about the difference between "failed for now" and "will never
// work".
package pubgsync

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/connectors/pubg"
	"github.com/knightsofeternity/kfire-server/internal/store"
)

// syncEvery is how often each linked member is refreshed.
//
// PUBG keeps matches for 14 days, so a daily pass leaves thirteen days of
// slack if one fails. It costs one quota-counted request per member (the
// player lookup); the match reads that follow are free, which is what makes a
// full refresh cheap.
const syncEvery = 24 * time.Hour

// SLUG is the catalog slug this package claims, for both the daily sync and
// the plugin that displays what it collected.
//
// The two must never name different games: a sync filing matches under one
// slug while the plugin reads another would show empty pages with nothing
// logged and nothing failing. One constant is what makes that impossible.
//
// Recorded from production on 2026-09-22; the catalog holds several games
// whose name contains "battlegrounds", so this is the checked one and not a
// guess.
const SLUG = "pubg-battlegrounds"

// puller is the slice of the connector the walk actually uses. It is an
// interface so the walk over a member's matches can be tested against a fake,
// without an HTTP server and without a database.
type puller interface {
	MatchForPlayer(platform, matchID, accountID string) (pubg.MatchResult, error)
}

// Syncer records PUBG matches once a day.
type Syncer struct {
	store *store.Store
	pubg  *pubg.Connector
}

// New returns a syncer.
func New(st *store.Store, conn *pubg.Connector) *Syncer {
	return &Syncer{store: st, pubg: conn}
}

// Run passes over every linked member, now and then once a day, until ctx is
// cancelled.
//
// The first pass runs immediately rather than a day later: a restart must not
// cost a day of collection, and with a 14-day retention a missed day is not
// recoverable.
func (s *Syncer) Run(ctx context.Context) {
	ticker := time.NewTicker(syncEvery)
	defer ticker.Stop()
	for {
		s.SyncAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SyncAll refreshes every linked member once.
func (s *Syncer) SyncAll(ctx context.Context) {
	if s.pubg == nil || !s.pubg.Enabled() {
		return // connector not configured on this instance
	}
	members, err := s.store.PubgLinkedMembers(ctx)
	if err != nil {
		slog.Error("pubgsync: list linked members", "err", err)
		return
	}
	if len(members) == 0 {
		return // nobody linked: not worth resolving the game
	}
	game, err := s.store.GetGameBySlug(ctx, SLUG)
	if err != nil {
		// The catalog is imported in the background on a fresh instance, so
		// the game can legitimately be missing for a few minutes after boot.
		slog.Warn("pubgsync: PUBG is not in the games catalog yet", "slug", SLUG, "err", err)
		return
	}

	for _, m := range members {
		if ctx.Err() != nil {
			return
		}
		// One member's failure must not cost the others their pass: a single
		// broken link would otherwise cancel a day of collection for everyone.
		if err := s.syncMember(ctx, m, game.ID); err != nil {
			slog.Warn("pubgsync: member skipped", "user_id", m.UserID, "err", err)
		}
	}
}

// syncMember records whatever that member has played and we do not have yet.
//
// The returned error is the one that stopped the whole member, not one that
// merely skipped a match: an individual match never fails the pass.
func (s *Syncer) syncMember(ctx context.Context, m store.PubgPlayer, gameID string) error {
	known, err := s.store.PubgKnownMatchIDs(ctx, m.UserID, gameID)
	if err != nil {
		// Fail closed: without the known ids, every match would be read again
		// for nothing.
		return err
	}
	player, err := s.pubg.PlayerByID(m.Platform, m.AccountID)
	if err != nil {
		return err
	}

	res := walk(ctx, s.pubg, m, player.MatchIDs, known, func(r pubg.MatchResult) error {
		return s.store.InsertPubgMatch(ctx, toStore(m.UserID, gameID, r))
	})
	if res.stored > 0 || res.retryable > 0 {
		slog.Info("pubgsync: member refreshed", "user_id", m.UserID,
			"stored", res.stored, "known", res.known, "not_playable", res.notPlayable,
			"retryable", res.retryable, "permanent", res.permanent)
	}
	return nil
}

// outcome counts what a walk did, for the log line and for the tests.
type outcome struct {
	stored      int // written to the database
	known       int // already stored, not read again
	notPlayable int // a tutorial, the training range, a custom game
	retryable   int // failed for now, and the next pass will try again
	permanent   int // failed for good, and retrying would not help
}

// walk reads the matches that are not stored yet and saves them.
//
// Nothing here records that a match was handled. The only mark of "done" is
// the row in pubg_matches, which is why a match that fails for a passing
// reason simply comes back as a candidate tomorrow, while it is still inside
// the publisher's 14-day window. Writing a watermark instead would turn one
// timeout into a match lost forever.
func walk(ctx context.Context, p puller, m store.PubgPlayer, matchIDs []string,
	known map[string]bool, save func(pubg.MatchResult) error) outcome {

	var res outcome
	for _, id := range matchIDs {
		if ctx.Err() != nil {
			return res
		}
		if known[id] {
			// Free against the quota, but not free in time, and a stored match
			// cannot have changed.
			res.known++
			continue
		}
		r, err := p.MatchForPlayer(m.Platform, id, m.AccountID)
		switch {
		case errors.Is(err, pubg.ErrNotPlayable):
			// A tutorial or a training session. This is the ordinary case, not
			// a failure, and it must never be logged as one.
			res.notPlayable++
			continue
		case errors.Is(err, pubg.ErrNoParticipant):
			// The match exists and does not list this account. It never will,
			// so there is nothing to come back for.
			res.permanent++
			slog.Warn("pubgsync: account absent from its own match",
				"user_id", m.UserID, "match_id", id)
			continue
		case err != nil:
			if pubg.Transient(err) {
				// "Later", not "no". Leave it unstored and keep going: the
				// remaining matches are independent and cost no quota, and
				// this one is a candidate again tomorrow.
				res.retryable++
				slog.Warn("pubgsync: match read failed, will retry",
					"user_id", m.UserID, "match_id", id, "err", err)
			} else {
				res.permanent++
				slog.Warn("pubgsync: match unreadable, giving up on it",
					"user_id", m.UserID, "match_id", id, "err", err)
			}
			continue
		}
		if err := save(r); err != nil {
			// A CHECK violation is the one database failure that will NEVER
			// succeed: the match really is outside the bounds this table
			// declares. Treating it as passing would retry it every day for the
			// fourteen days the publisher keeps it, log an error each time, and
			// lose it anyway. Counting it as permanent stops the storm, and the
			// single loud line is what tells us a bound is wrong so we can widen
			// it while the match is still fetchable.
			if isCheckViolation(err) {
				res.permanent++
				slog.Error("pubgsync: a match falls outside what the table allows, and a bound is probably too tight",
					"user_id", m.UserID, "match_id", id, "err", err)
				continue
			}
			// Anything else is passing by nature, and the insert is idempotent,
			// so the next pass writes it.
			res.retryable++
			slog.Error("pubgsync: store match", "user_id", m.UserID, "match_id", id, "err", err)
			continue
		}
		res.stored++
	}
	return res
}

// toStore converts a connector result into a row.
//
// Damage stays a float all the way to the column, which is a numeric(8,2):
// the publisher sends it as an integer on some matches and as a float on
// others, and truncating it here would lose information for nothing.
func toStore(userID, gameID string, r pubg.MatchResult) store.PubgMatch {
	return store.PubgMatch{
		UserID: userID, GameID: gameID, MatchID: r.MatchID,
		GameMode: r.GameMode, MapName: r.MapName,
		WinPlace: r.WinPlace, Kills: r.Kills, Assists: r.Assists,
		HeadshotKills: r.HeadshotKills, Revives: r.Revives,
		DamageDealt: r.DamageDealt, TimeSurvived: r.TimeSurvived,
		DurationSecs: r.DurationSecs, PlayedAt: r.PlayedAt,
	}
}

// isCheckViolation reports whether an error is PostgreSQL refusing a row
// because it breaks a CHECK constraint.
//
// SQLSTATE 23514 is the only database failure here that retrying cannot fix,
// which is why it is worth telling apart from a connection that dropped.
func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}
