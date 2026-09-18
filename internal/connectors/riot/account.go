package riot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Account is one Riot account's identity.
type Account struct {
	PUUID    string `json:"puuid"`
	GameName string `json:"gameName"`
	TagLine  string `json:"tagLine"`
}

// RiotID renders the display form, "Name#TAG".
func (a Account) RiotID() string { return a.GameName + "#" + a.TagLine }

// APIError is a non-200 answer from Riot. Callers branch on Status: 404 is a
// normal, expected answer on several routes, 429 means back off.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("riot: %s: status %d: %s", e.Path, e.Status, e.Body)
}

// NotFound reports whether err is a Riot 404.
func NotFound(err error) bool {
	var e *APIError
	if ok := asAPIError(err, &e); !ok {
		return false
	}
	return e.Status == http.StatusNotFound
}

// maxRetries bounds how many times a 429 is retried before giving up. Three
// is enough to ride out a burst; beyond that the key is genuinely saturated
// and the caller must hear about it rather than block a request forever.
const maxRetries = 3

// retryAfter reads Riot's Retry-After header, in seconds.
//
// Riot does not always send it, so a missing or unreadable header falls back
// to a second rather than to zero: retrying immediately is what got us
// throttled.
func retryAfter(res *http.Response) time.Duration {
	if v := res.Header.Get("Retry-After"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Second
}

// get performs an authenticated GET against one Riot host and decodes the body
// into out. host is either a platform (euw1) or a cluster (europe).
//
// Every call waits for the connector's limiter first, so callers never have to
// think about the quota: RecentMatches fetching details in parallel and the
// backfill walking thousands of matches queue behind the same cursor.
func (c *Connector) get(ctx context.Context, host, path string, out any) error {
	endpoint := fmt.Sprintf(c.APIHostTmpl, host) + path
	var last error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.attempt(ctx, endpoint, path, out)
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

// attempt performs one request. Split out of get so that the deferred close of
// the response body is scoped to a single try: deferring inside the retry loop
// would be correct only as long as every branch returned, which is exactly the
// kind of invariant a later edit breaks silently.
func (c *Connector) attempt(ctx context.Context, endpoint, path string, out any) error {
	if c.limiter != nil {
		// Waiting here, on every attempt, is what keeps a retry from being a
		// burst: even a Retry-After of zero still queues behind the limiter's
		// own spacing.
		if err := c.limiter.wait(ctx); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Riot-Token", c.APIKey)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusTooManyRequests && c.limiter != nil {
		// Slow EVERY caller down, not just this goroutine: the quota belongs
		// to the key, so one rejected call is everyone's problem.
		c.limiter.penalise(retryAfter(res))
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return &APIError{Status: res.StatusCode, Path: path, Body: string(body)}
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// AccountByPUUID resolves a PUUID to its current Riot ID.
func (c *Connector) AccountByPUUID(ctx context.Context, puuid string) (Account, error) {
	var acc Account
	err := c.get(ctx, accountCluster, "/riot/account/v1/accounts/by-puuid/"+url.PathEscape(puuid), &acc)
	return acc, err
}

// AccountByRiotID resolves a Riot ID, "Name#TAG" split in two, to its account.
// A Riot ID nobody owns comes back as a 404, which callers surface as a typo
// rather than as a failure.
func (c *Connector) AccountByRiotID(ctx context.Context, gameName, tagLine string) (Account, error) {
	var acc Account
	path := "/riot/account/v1/accounts/by-riot-id/" +
		url.PathEscape(gameName) + "/" + url.PathEscape(tagLine)
	err := c.get(ctx, accountCluster, path, &acc)
	return acc, err
}

// ActiveRegion returns the member's active League platform, e.g. "euw1".
func (c *Connector) ActiveRegion(ctx context.Context, puuid string) (string, error) {
	var out struct {
		Region string `json:"region"`
	}
	path := "/riot/account/v1/region/by-game/lol/by-puuid/" + url.PathEscape(puuid)
	if err := c.get(ctx, accountCluster, path, &out); err != nil {
		return "", err
	}
	if out.Region == "" {
		return "", fmt.Errorf("riot: empty active region")
	}
	return out.Region, nil
}
