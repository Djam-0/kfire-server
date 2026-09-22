package api

import (
	"errors"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/knightsofeternity/kfire-server/internal/store"
)

// maxRecapWindow bounds how much of the history one call may read. Without it
// a year-long window would scan the whole match history on every request,
// while a week already covers an evening several times over.
const maxRecapWindow = 7 * 24 * time.Hour

// errRecapRange carries the reason a window was refused, so the caller gets a
// sentence it can show instead of an empty result.
var errRecapRange = errors.New("invalid recap range")

// recapWindow is the half-open interval [From, To) a recap covers.
type recapWindow struct {
	From time.Time
	To   time.Time
}

// parseRecapWindow validates the two query parameters. It is deliberately a
// pure function: this is the one place in the feature where an off-by-one is
// both easy to write and invisible in production.
func parseRecapWindow(from, to string) (recapWindow, string, error) {
	if from == "" || to == "" {
		return recapWindow{}, "from and to are required", errRecapRange
	}
	f, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return recapWindow{}, "from must be RFC 3339", errRecapRange
	}
	t, err := time.Parse(time.RFC3339, to)
	if err != nil {
		return recapWindow{}, "to must be RFC 3339", errRecapRange
	}
	if !t.After(f) {
		return recapWindow{}, "to must be strictly after from", errRecapRange
	}
	if t.Sub(f) > maxRecapWindow {
		return recapWindow{}, "the range cannot exceed 7 days", errRecapRange
	}
	return recapWindow{From: f.UTC(), To: t.UTC()}, "", nil
}

// recapMember is the identity half of a per-member line. It says which member
// a match CAME FROM and nothing else: who played with whom is not knowable
// here, and guessing it from two members playing at the same minute would be a
// supposition displayed as a fact.
type recapMember struct {
	UserID    string
	Username  string
	AvatarURL *string
}

// rocketLeagueRecapMember is one member's Rocket League record over a window.
type rocketLeagueRecapMember struct {
	recapMember
	Matches         int
	Wins            int
	Losses          int
	Draws           int
	MVPs            int
	Goals           int
	Assists         int
	Saves           int
	Shots           int
	Demos           int
	Score           int
	PlayTimeSeconds int
}

// hearthstoneRecapMember is one member's Hearthstone record over a window.
// Nothing is shared with the Rocket League record on purpose: one counts
// goals, the other a finishing position.
type hearthstoneRecapMember struct {
	recapMember
	Matches      int
	Wins         int
	Losses       int
	Draws        int
	Ranked       int // matches carrying a placement (Battlegrounds)
	PlacementSum int
	Top4         int
}

// recapGameBlock is one game's part of the summary. Members holds either
// rocketLeagueRecapMember or hearthstoneRecapMember values, already rendered.
type recapGameBlock struct {
	store.RecapGame
	Matches int
	Members []fiber.Map
}

// tallyResult adds one match result to a win/loss/draw triple.
func tallyResult(result string, wins, losses, draws *int) {
	switch result {
	case "win":
		*wins++
	case "loss":
		*losses++
	default:
		*draws++
	}
}

// aggregateRocketLeague folds matches into one block per game, and inside each
// block one line per member.
//
// Grouping is by game id rather than by slug because the same game can exist
// twice in the catalog (two stores, two rows) and a recap must not merge two
// distinct catalog entries behind one identifier.
func aggregateRocketLeague(matches []store.RecapRocketLeagueMatch) []recapGameBlock {
	type key struct{ game, user string }
	byMember := map[key]*rocketLeagueRecapMember{}
	games := map[string]*recapGameBlock{}
	var order []string

	for _, m := range matches {
		g, ok := games[m.GameID]
		if !ok {
			g = &recapGameBlock{RecapGame: m.RecapGame}
			games[m.GameID] = g
			order = append(order, m.GameID)
		}
		g.Matches++

		k := key{m.GameID, m.UserID}
		s, ok := byMember[k]
		if !ok {
			s = &rocketLeagueRecapMember{recapMember: recapMember{
				UserID: m.UserID, Username: m.Username, AvatarURL: m.AvatarURL}}
			byMember[k] = s
		}
		s.Matches++
		tallyResult(m.Result, &s.Wins, &s.Losses, &s.Draws)
		if m.MVP {
			s.MVPs++
		}
		s.Goals += m.Goals
		s.Assists += m.Assists
		s.Saves += m.Saves
		s.Shots += m.Shots
		s.Demos += m.Demos
		s.Score += m.Score
		s.PlayTimeSeconds += m.DurationSeconds
	}

	blocks := make([]recapGameBlock, 0, len(order))
	for _, id := range order {
		g := games[id]
		var members []*rocketLeagueRecapMember
		for k, s := range byMember {
			if k.game == id {
				members = append(members, s)
			}
		}
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].Matches != members[j].Matches {
				return members[i].Matches > members[j].Matches
			}
			return members[i].Username < members[j].Username
		})
		for _, s := range members {
			g.Members = append(g.Members, rocketLeagueMemberJSON(*s))
		}
		blocks = append(blocks, *g)
	}
	sortRecapBlocks(blocks)
	return blocks
}

// aggregateHearthstone is the Hearthstone counterpart of aggregateRocketLeague.
func aggregateHearthstone(matches []store.RecapHearthstoneMatch) []recapGameBlock {
	type key struct{ game, user string }
	byMember := map[key]*hearthstoneRecapMember{}
	games := map[string]*recapGameBlock{}
	var order []string

	for _, m := range matches {
		g, ok := games[m.GameID]
		if !ok {
			g = &recapGameBlock{RecapGame: m.RecapGame}
			games[m.GameID] = g
			order = append(order, m.GameID)
		}
		g.Matches++

		k := key{m.GameID, m.UserID}
		s, ok := byMember[k]
		if !ok {
			s = &hearthstoneRecapMember{recapMember: recapMember{
				UserID: m.UserID, Username: m.Username, AvatarURL: m.AvatarURL}}
			byMember[k] = s
		}
		s.Matches++
		tallyResult(m.Result, &s.Wins, &s.Losses, &s.Draws)
		// A match without a placement says nothing about how it went, so it
		// weighs on neither the average nor the top-4 count.
		if m.Placement != nil {
			s.Ranked++
			s.PlacementSum += *m.Placement
			if *m.Placement <= 4 {
				s.Top4++
			}
		}
	}

	blocks := make([]recapGameBlock, 0, len(order))
	for _, id := range order {
		g := games[id]
		var members []*hearthstoneRecapMember
		for k, s := range byMember {
			if k.game == id {
				members = append(members, s)
			}
		}
		// By match count then name, the order every other per-member aggregate
		// uses. It is NOT a ranking: the page sorts on what it displays.
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].Matches != members[j].Matches {
				return members[i].Matches > members[j].Matches
			}
			return members[i].Username < members[j].Username
		})
		for _, s := range members {
			g.Members = append(g.Members, hearthstoneMemberJSON(*s))
		}
		blocks = append(blocks, *g)
	}
	sortRecapBlocks(blocks)
	return blocks
}

// sortRecapBlocks orders games by match count then name, so the game that
// carried the evening comes first.
func sortRecapBlocks(blocks []recapGameBlock) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Matches != blocks[j].Matches {
			return blocks[i].Matches > blocks[j].Matches
		}
		return blocks[i].GameName < blocks[j].GameName
	})
}

func recapMemberJSON(m recapMember) fiber.Map {
	out := fiber.Map{"user_id": m.UserID, "username": m.Username}
	if m.AvatarURL != nil {
		out["avatar_url"] = *m.AvatarURL
	}
	return out
}

func rocketLeagueMemberJSON(s rocketLeagueRecapMember) fiber.Map {
	out := recapMemberJSON(s.recapMember)
	out["matches"] = s.Matches
	out["wins"] = s.Wins
	out["losses"] = s.Losses
	out["draws"] = s.Draws
	out["mvps"] = s.MVPs
	out["goals"] = s.Goals
	out["assists"] = s.Assists
	out["saves"] = s.Saves
	out["shots"] = s.Shots
	out["demos"] = s.Demos
	out["score"] = s.Score
	out["play_time_seconds"] = s.PlayTimeSeconds
	return out
}

func hearthstoneMemberJSON(s hearthstoneRecapMember) fiber.Map {
	out := recapMemberJSON(s.recapMember)
	out["matches"] = s.Matches
	out["wins"] = s.Wins
	out["losses"] = s.Losses
	out["draws"] = s.Draws
	out["ranked"] = s.Ranked
	out["top4"] = s.Top4
	// Null rather than zero when nothing carried a placement: an average of
	// nothing is not a first place.
	out["avg_placement"] = nil
	if s.Ranked > 0 {
		out["avg_placement"] = float64(s.PlacementSum) / float64(s.Ranked)
	}
	return out
}

// recapGameIcon adds a link to the image cache, and only when the catalog
// actually holds an icon for that game.
//
// Emitting the link unconditionally would be simpler and wrong: the cache
// answers 404 for a game with no source image, and the page would draw a
// broken image beside a name that is perfectly fine. Absence is the signal,
// so it is decided here rather than guessed by the browser.
func recapGameIcon(out fiber.Map, base string, game store.RecapGame) {
	if game.GameIcon != nil {
		out["icon_url"] = base + "/img/games/" + game.GameID + "/icon"
	}
}

// recapEntryJSON is the shared head of a timeline entry: when, which member it
// came from, and which game.
//
// base is the server's public URL, threaded down rather than read from a
// global so these builders stay plain functions the tests can call.
func recapEntryJSON(base string, owner store.RecapMatchOwner, game store.RecapGame, playedAt time.Time) fiber.Map {
	out := recapMemberJSON(recapMember{owner.UserID, owner.Username, owner.AvatarURL})
	out["played_at"] = playedAt.UTC()
	out["game_id"] = game.GameID
	out["game_slug"] = game.GameSlug
	out["game_name"] = game.GameName
	recapGameIcon(out, base, game)
	return out
}

func rocketLeagueEntryJSON(base string, m store.RecapRocketLeagueMatch) fiber.Map {
	out := recapEntryJSON(base, m.RecapMatchOwner, m.RecapGame, m.PlayedAt)
	out["result"] = m.Result
	out["playlist"] = m.Playlist
	out["team_size"] = m.TeamSize
	out["player_team"] = m.PlayerTeam
	out["team_blue_score"] = m.TeamBlueScore
	out["team_orange_score"] = m.TeamOrangeScore
	out["goals"] = m.Goals
	out["assists"] = m.Assists
	out["saves"] = m.Saves
	out["shots"] = m.Shots
	out["score"] = m.Score
	out["demos"] = m.Demos
	out["mvp"] = m.MVP
	out["duration_seconds"] = m.DurationSeconds
	return out
}

func hearthstoneEntryJSON(base string, m store.RecapHearthstoneMatch) fiber.Map {
	out := recapEntryJSON(base, m.RecapMatchOwner, m.RecapGame, m.PlayedAt)
	out["mode"] = m.Mode
	out["result"] = m.Result
	out["turns"] = m.Turns
	out["placement"] = m.Placement
	out["hero_card_id"] = m.HeroCardID
	return out
}

// mergeRecapTimeline interleaves the per-game lists into the single
// oldest-first order the evening actually happened in.
//
// Both inputs arrive sorted by the database. On an exact tie the Rocket League
// entry comes first, which is arbitrary but stable: two members playing at the
// same instant is a coincidence, never a shared match, and the order between
// them carries no meaning.
func mergeRecapTimeline(base string, rl []store.RecapRocketLeagueMatch, hs []store.RecapHearthstoneMatch) []fiber.Map {
	out := make([]fiber.Map, 0, len(rl)+len(hs))
	i, j := 0, 0
	for i < len(rl) && j < len(hs) {
		if hs[j].PlayedAt.Before(rl[i].PlayedAt) {
			out = append(out, hearthstoneEntryJSON(base, hs[j]))
			j++
			continue
		}
		out = append(out, rocketLeagueEntryJSON(base, rl[i]))
		i++
	}
	for ; i < len(rl); i++ {
		out = append(out, rocketLeagueEntryJSON(base, rl[i]))
	}
	for ; j < len(hs); j++ {
		out = append(out, hearthstoneEntryJSON(base, hs[j]))
	}
	return out
}

// GET /api/v1/recap?from=<RFC3339>&to=<RFC3339>  (authenticated)
//
// One call returns the whole evening: the per-game, per-member summary and the
// single all-games timeline. Splitting it per game would force the page to
// make three calls and stitch them back together for nothing.
//
// This is NOT /sessions, which serves presence sessions.
func (h *handlers) recap(c *fiber.Ctx) error {
	w, msg, err := parseRecapWindow(c.Query("from"), c.Query("to"))
	if err != nil {
		// An invalid window is answered with a sentence, not an empty recap:
		// an empty result is the normal answer to a badly chosen evening, and
		// the two must not look alike.
		return errorJSON(c, fiber.StatusBadRequest, "invalid_range", msg)
	}

	// Who is asking decides whose matches they may see: a recap is a listing of
	// recent sessions, so it honours the same privacy toggle they do.
	claims := mustClaims(c)
	viewer := store.RecapViewer{UserID: claims.UserID, IsAdmin: claims.Role == "admin"}

	rl, err := h.store.RocketLeagueMatchesBetween(c.Context(), w.From, w.To, viewer)
	if err != nil {
		return err
	}
	hs, err := h.store.HearthstoneMatchesBetween(c.Context(), w.From, w.To, viewer)
	if err != nil {
		return err
	}

	blocks := append(aggregateRocketLeague(rl), aggregateHearthstone(hs)...)
	sortRecapBlocks(blocks)
	games := make([]fiber.Map, 0, len(blocks))
	for _, b := range blocks {
		block := fiber.Map{
			"game_id":   b.GameID,
			"game_slug": b.GameSlug,
			"game_name": b.GameName,
			"matches":   b.Matches,
			"members":   b.Members,
		}
		recapGameIcon(block, h.cfg.PublicURL, b.RecapGame)
		games = append(games, block)
	}

	return c.JSON(fiber.Map{
		"from":          w.From,
		"to":            w.To,
		"total_matches": len(rl) + len(hs),
		"games":         games,
		"timeline":      mergeRecapTimeline(h.cfg.PublicURL, rl, hs),
	})
}
