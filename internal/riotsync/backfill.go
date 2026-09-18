package riotsync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/knightsofeternity/kfire-server/internal/connectors/riot"
	"github.com/knightsofeternity/kfire-server/internal/store"
)

// backfillPage is how many match ids are claimed per step. Riot's own ceiling
// on the ids route is 100, and each id then costs one detail call: a page is
// therefore about 101 calls, roughly two minutes of our quota.
const backfillPage = 100

// backfillEvery is how often a step runs.
//
// Deliberately slow. The backfill is the LOWEST priority consumer of a quota
// shared with the hourly refresh and the live poller, both of which serve
// someone who is waiting in front of a screen. Walking a long history takes
// hours, and that is fine: it runs once per member, forever.
const backfillEvery = 2 * time.Minute

// backfillMembers is how many members are advanced per step. One, on purpose:
// a step already costs a full page, and spreading it wider would only make
// every member finish later.
const backfillMembers = 1

// oldestPlayedAt returns the earliest instant in a page, which becomes the
// next cursor. The page is walked rather than trusting Riot's ordering: an
// assumption about ordering that turns out wrong would silently skip history.
func oldestPlayedAt(times []time.Time) time.Time {
	var oldest time.Time
	for _, t := range times {
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	return oldest
}

// backfillDone reports whether a page of this size ends the walk. Riot answers
// an empty page once there is nothing older left to give.
func backfillDone(idsInPage int) bool { return idsInPage == 0 }

// RunBackfill walks every linked member's League history into lol_matches,
// oldest-cursor-first, until Riot has nothing older to give.
//
// Interruptible by design: the cursor is stored after every page, so a restart
// resumes where it stopped. Without that, a server restarted nightly would
// never finish a long history.
func (s *Syncer) RunBackfill(ctx context.Context) {
	t := time.NewTicker(backfillEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.backfillStep(ctx)
		}
	}
}

func (s *Syncer) backfillStep(ctx context.Context) {
	if s.riot == nil || !s.riot.Enabled() || !s.pluginActive() {
		return
	}
	game, err := s.store.GetGameBySlug(ctx, liveSlug)
	if err != nil {
		// League missing from this instance's catalog is a legitimate, quiet
		// state. A database failure is not, and swallowing it here would hide
		// it every two minutes forever. Same reasoning as pollLive.
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("riotsync: get game", "slug", liveSlug, "err", err)
		}
		return
	}
	players, err := s.store.LolBackfillPending(ctx, backfillMembers)
	if err != nil {
		slog.Warn("riotsync: backfill pending", "err", err)
		return
	}
	for _, p := range players {
		s.backfillMember(ctx, p, game.ID)
	}
}

func (s *Syncer) backfillMember(ctx context.Context, p store.RiotPlayer, gameID string) {
	// pgx.ErrNoRows cannot happen here: p comes from LolBackfillPending, which
	// joins riot_accounts, so the row exists by construction. Any error is
	// therefore a real database failure and deserves to be seen.
	before, _, err := s.store.LolBackfillCursor(ctx, p.UserID)
	if err != nil {
		slog.Warn("riotsync: backfill cursor", "user_id", p.UserID, "err", err)
		return
	}
	var cursor time.Time
	if before != nil {
		cursor = *before
	}

	ids, err := s.riot.MatchIDsBefore(ctx, riot.MatchCluster(p.Platform), p.PUUID, cursor, backfillPage)
	if err != nil {
		slog.Warn("riotsync: backfill ids", "user_id", p.UserID, "err", err)
		return
	}
	if backfillDone(len(ids)) {
		// Nothing older left. From here on the hourly refresh keeps it current.
		if err := s.store.SetLolBackfillCursor(ctx, p.UserID, before, true); err != nil {
			slog.Warn("riotsync: backfill finish", "user_id", p.UserID, "err", err)
		}
		slog.Info("riotsync: backfill complete", "user_id", p.UserID)
		return
	}

	// Sequential on purpose. The limiter would serialise parallel calls anyway,
	// and going one at a time means an interrupted page still advances the
	// cursor honestly rather than leaving holes behind it.
	var seen []time.Time
	for _, id := range ids {
		m, err := s.riot.MatchDetail(ctx, riot.MatchCluster(p.Platform), id, p.PUUID)
		if riot.Transient(err) {
			// Riot is rate-limiting us or having a bad minute. Abandon the page
			// WITHOUT moving the cursor, so the next tick walks it again.
			//
			// Skipping instead would be silently destructive: the cursor would
			// move past these matches and never come back, so a passing 429
			// would carve a permanent hole in a history nobody can audit. The
			// whole point of this walk is that it is complete.
			slog.Warn("riotsync: backfill paused on a transient failure",
				"user_id", p.UserID, "err", err)
			return
		}
		if err != nil {
			// A permanent answer: a 404, or a match that does not list this
			// member. Retrying it forever would stall the walk on one bad id.
			continue
		}
		seen = append(seen, m.PlayedAt)
		if err := s.store.InsertLolMatch(ctx, store.LolMatch{
			UserID: p.UserID, GameID: gameID, MatchID: m.MatchID,
			Champion: m.Champion, Win: m.Win,
			Kills: m.Kills, Deaths: m.Deaths, Assists: m.Assists,
			QueueID: m.QueueID, DurationSeconds: m.DurationSeconds,
			PlayedAt: m.PlayedAt,
		}); err != nil {
			slog.Warn("riotsync: backfill store", "user_id", p.UserID, "match_id", m.MatchID, "err", err)
		}
	}

	next := oldestPlayedAt(seen)
	if next.IsZero() {
		// A whole page without a single usable detail. Do not move the cursor,
		// or the history behind it would be skipped for good.
		slog.Warn("riotsync: backfill page yielded nothing", "user_id", p.UserID)
		return
	}
	// One second earlier, and that subtraction is load-bearing: Riot's endTime
	// is INCLUSIVE, so without it the oldest match of this page comes back as
	// the newest of the next one, forever. Insertion is idempotent, so the
	// symptom would not be duplicate rows but a walk that never advances.
	//
	// Subtracting a whole second cannot skip a match: a member cannot be in two
	// League games at once, and a game lasts minutes, so no two of their matches
	// share an end second.
	next = next.Add(-time.Second)
	if err := s.store.SetLolBackfillCursor(ctx, p.UserID, &next, false); err != nil {
		slog.Warn("riotsync: backfill advance", "user_id", p.UserID, "err", err)
	}
}
