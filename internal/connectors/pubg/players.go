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
		Data []struct {
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
		} `json:"data"`
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

	e := body.Data[0]
	p := Player{AccountID: e.ID, Name: e.Attributes.Name}
	p.MatchIDs = make([]string, 0, len(e.Relationships.Matches.Data))
	for _, m := range e.Relationships.Matches.Data {
		p.MatchIDs = append(p.MatchIDs, m.ID)
	}
	return p, nil
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
