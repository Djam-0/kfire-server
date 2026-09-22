package pubg

import (
	"net/http"
	"net/url"
	"strings"
)

// KnownPlatforms returns every shard KFIRE accepts, for the account page's
// platform picker. The API is partitioned by shard: a name alone finds nobody,
// so the member has to say where they play.
func KnownPlatforms() []string {
	return []string{"steam", "xbox", "psn", "kakao", "stadia"}
}

// Player is a PUBG account, resolved from the in-game name the member typed.
//
// AccountID is what gets stored, never Name: a player can rename themself, and
// the account id cannot. Storing the name would break the link on the first
// rename, which is exactly the weak point Rocket League still has.
type Player struct {
	AccountID string
	Name      string
	MatchIDs  []string
}

// PlayerByName resolves an in-game name on one platform.
//
// This is one of the few calls the publisher actually meters, so it waits for
// the limiter. It is also the only per-member cost of a sync: everything that
// follows reads matches, which are free.
//
// A name nobody owns comes back as a 404, which callers surface as a typo
// rather than as a failure.
func (c *Connector) PlayerByName(platform, name string) (Player, error) {
	q := url.Values{}
	// The parameter is spelled with literal brackets, so it has to go through
	// Values rather than be pasted into the path.
	q.Set("filter[playerNames]", name)
	path := "/shards/" + url.PathEscape(platform) + "/players?" + q.Encode()

	var body struct {
		Data []playerEntry `json:"data"`
	}
	if err := c.get(path, &body); err != nil {
		// The searched name is in the path, and the path ends up in logs. The
		// failed lookup is worth recording; who was being looked up is not.
		var apiErr *APIError
		if asAPIError(err, &apiErr) {
			apiErr.Path = redactName(apiErr.Path, name)
		}
		return Player{}, err
	}
	if len(body.Data) == 0 {
		// The API normally 404s an unknown name, but an empty data array says
		// the same thing. Callers must not have to handle two shapes of "no
		// such player", and indexing it blindly would panic.
		return Player{}, &APIError{
			Status: http.StatusNotFound,
			Path:   redactName(path, name),
			Body:   "no player matching that name on this platform",
		}
	}

	return body.Data[0].player(), nil
}

// PlayerByID lists an account's recent matches, without going through its
// name.
//
// This is what the daily sync uses. The name is deliberately not involved: a
// member who renames themself in game would otherwise stop being collected
// silently, and with a 14-day retention nobody would notice in time to get
// those matches back.
//
// Like the name lookup, this one is metered, so it waits for the limiter. It
// is the only quota-counted call a sync pass makes per member.
func (c *Connector) PlayerByID(platform, accountID string) (Player, error) {
	path := "/shards/" + url.PathEscape(platform) + "/players/" + url.PathEscape(accountID)

	// This route answers with a single object where the name search answers
	// with an array, so the envelope differs even though the player inside is
	// the same shape.
	var body struct {
		Data playerEntry `json:"data"`
	}
	if err := c.get(path, &body); err != nil {
		return Player{}, err
	}
	return body.Data.player(), nil
}

// playerEntry is a player object as both player routes carry it.
type playerEntry struct {
	ID         string `json:"id"`
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
	Relationships struct {
		Matches struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"matches"`
	} `json:"relationships"`
}

// player converts a decoded entry, dropping everything the portal has no use
// for. MatchIDs keeps the API's order, newest first.
func (e playerEntry) player() Player {
	p := Player{AccountID: e.ID, Name: e.Attributes.Name}
	p.MatchIDs = make([]string, 0, len(e.Relationships.Matches.Data))
	for _, m := range e.Relationships.Matches.Data {
		p.MatchIDs = append(p.MatchIDs, m.ID)
	}
	return p
}

// redactName strips the searched name out of a path before it lands in an
// error, which is very likely to end up in a log line. The failing lookup is
// worth recording; who was being looked up is not.
func redactName(path, name string) string {
	if name == "" {
		return path
	}
	return strings.ReplaceAll(path, url.QueryEscape(name), "...")
}
