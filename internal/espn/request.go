package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	maxBackoff = 30 * time.Second

	// defaultMaxAttempts is the total number of times a request is tried when
	// a constructor does not override it. Backoff is skipped after the final
	// attempt, so MaxAttempts=5 means at most 4 backoff sleeps.
	defaultMaxAttempts = 5
)

// Client holds configuration for ESPN HTTP requests.
type Client struct {
	// MaxAttempts is the total number of times a request is tried before
	// giving up (MaxAttempts-1 retries). Backoff is skipped after the final
	// attempt.
	MaxAttempts    int
	InitialBackoff time.Duration
	RequestTimeout time.Duration
	RateLimit      time.Duration // delay between batch API calls
	Sport          Sport         // sport this client fetches data for

	// Shared HTTP transport. Set by the constructors; callers that build a
	// Client directly (tests) fall back to a per-request client with the
	// configured timeout.
	httpClient *http.Client

	// Per-client endpoint URLs. Set by NewClientForSport; tests point a
	// single client at a mock server via SetURLs.
	gameStatsURL  string
	teamInfoURL   string
	scoreboardURL string
	conferenceURL string
}

// NewClientForSport returns a SportClient configured for the given sport.
func NewClientForSport(sport Sport) SportClient {
	urls := SportURLs(sport)
	c := &Client{
		MaxAttempts:    defaultMaxAttempts,
		InitialBackoff: 1 * time.Second,
		RequestTimeout: 1 * time.Second,
		RateLimit:      500 * time.Millisecond,
		Sport:          sport,
		httpClient:     &http.Client{Timeout: 1 * time.Second},
		gameStatsURL:   urls.GameStats,
		teamInfoURL:    urls.TeamInfo,
		scoreboardURL:  urls.Scoreboard,
		conferenceURL:  urls.Conferences,
	}
	return wrapClient(c)
}

// wrapClient wraps a *Client in the appropriate sport-specific struct.
func wrapClient(c *Client) SportClient {
	switch c.Sport {
	case CollegeBasketball:
		return &BasketballClient{Client: c}
	case CollegeFootball:
		return &FootballClient{Client: c}
	default:
		panic(fmt.Sprintf("unsupported sport: %s", c.Sport))
	}
}

// GameStatsURL returns the game stats URL template for this client.
func (c *Client) GameStatsURL() string {
	return c.gameStatsURL
}

// TeamInfoURL returns the team info URL for this client.
func (c *Client) TeamInfoURL() string {
	return c.teamInfoURL
}

// ScoreboardURL returns the scoreboard URL for this client.
func (c *Client) ScoreboardURL() string {
	return c.scoreboardURL
}

// ConferencesURL returns the scoreboard/conferences URL for this client.
func (c *Client) ConferencesURL() string {
	return c.conferenceURL
}

type validatable interface {
	validate() error
}

// userAgentFor returns the User-Agent to send to the given endpoint URL.
// See the header comment in makeRequest for the per-host rationale.
func userAgentFor(endpoint string) string {
	if strings.Contains(endpoint, "site.api.espn.com") {
		return "curl/8.5.0"
	}
	return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/131.0.0.0 Safari/537.36"
}

// Responses constrains the response types makeRequest can decode. Every
// ESPN response implements validate, so decoded payloads are checked at the
// transport boundary.
type Responses interface {
	GameInfoESPN | TeamInfoESPN | TeamDetailESPN | ScoreboardESPN | SiteScoreboardESPN | ConferencesESPN
	validatable
}

// makeRequest fetches endpoint and decodes the JSON body into out. It retries
// 5xx responses with exponential backoff and aborts as soon as ctx is
// cancelled. out is validated after decoding. c.MaxAttempts is the total
// number of tries (including the first); no backoff sleep happens after the
// final failed attempt because the loop ends there either way.
func makeRequest[T Responses](ctx context.Context, c *Client, endpoint string, out *T) error {
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: c.RequestTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building request for %q: %w", endpoint, err)
	}

	headers := map[string]string{
		// Per-host User-Agent policy (both determined empirically 2026-08-29):
		//
		// - cdn.espn.com serves empty-body HTTP 202 challenges for old browser
		//   User-Agents, so it needs a current browser UA.
		// - site.api.espn.com 403s requests that CLAIM to be a browser (its
		//   protection flags the TLS-fingerprint/UA mismatch) but allows
		//   plain client UAs such as curl's. Verified by header matrix:
		//   curl-like UA 200, custom and Chrome UAs 403.
		"User-Agent": userAgentFor(endpoint),
		"Accept":     "application/json",
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	maxAttempts := c.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	var res *http.Response
	for attempt := range maxAttempts {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("request cancelled for %q: %w", endpoint, err)
		}

		res, err = httpClient.Do(req)
		if err == nil {
			// 202 is retried like a 5xx: ESPN's edge intermittently answers
			// automated clients with an empty-body 202 bot challenge. A short
			// backoff outlasts the window, whereas passing the empty body to
			// the decoder fails the whole poll (observed 2026-08-29).
			if res.StatusCode < 500 && res.StatusCode != http.StatusAccepted {
				break
			}
			res.Body.Close()
			err = fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
		} else if ctx.Err() != nil {
			return fmt.Errorf("request cancelled for %q: %w", endpoint, ctx.Err())
		}

		// Back off before retrying — except after the final attempt, where
		// sleeping would only delay the (already certain) error return.
		if attempt == maxAttempts-1 {
			continue
		}
		if sleepErr := c.sleep(ctx, c.backoff(attempt)); sleepErr != nil {
			return fmt.Errorf("request cancelled for %q: %w", endpoint, sleepErr)
		}
	}
	if err != nil {
		return fmt.Errorf("error from %q: %w", endpoint, err)
	}

	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
	}

	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response from %q (HTTP %d): %w", endpoint, res.StatusCode, err)
	}

	return (*out).validate()
}

// sleep waits for d, returning early if ctx is cancelled.
func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.InitialBackoff << attempt
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
