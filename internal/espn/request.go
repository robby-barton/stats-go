package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const maxBackoff = 30 * time.Second

// Client holds configuration for ESPN HTTP requests.
type Client struct {
	MaxRetries     int
	InitialBackoff time.Duration
	RequestTimeout time.Duration
	RateLimit      time.Duration // delay between batch API calls
	Sport          Sport         // sport this client fetches data for

	// Shared HTTP transport. Set by the constructors; callers that build a
	// Client directly (tests) fall back to a per-request client with the
	// configured timeout.
	httpClient *http.Client

	// Per-client URL overrides. When non-empty, these take precedence over
	// the package-level vars. This allows multiple clients (one per sport) to
	// coexist in the same process.
	scheduleURL   string
	gameStatsURL  string
	teamInfoURL   string
	scoreboardURL string
}

// NewClient returns a SportClient configured for college football with sensible defaults.
// Per-client URL overrides are NOT set, so this client falls back to the
// package-level vars (which can be overridden via SetTestURLs in tests).
func NewClient() SportClient {
	return &FootballClient{Client: &Client{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		RequestTimeout: 1 * time.Second,
		RateLimit:      500 * time.Millisecond,
		Sport:          CollegeFootball,
		httpClient:     &http.Client{Timeout: 1 * time.Second},
	}}
}

// NewClientForSport returns a SportClient configured for the given sport.
func NewClientForSport(sport Sport) SportClient {
	urls := SportURLs(sport)
	c := &Client{
		MaxRetries:     5,
		InitialBackoff: 1 * time.Second,
		RequestTimeout: 1 * time.Second,
		RateLimit:      500 * time.Millisecond,
		Sport:          sport,
		httpClient:     &http.Client{Timeout: 1 * time.Second},
		scheduleURL:    urls.Schedule,
		gameStatsURL:   urls.GameStats,
		teamInfoURL:    urls.TeamInfo,
		scoreboardURL:  urls.Scoreboard,
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

// WeekURL returns the schedule URL for this client.
func (c *Client) WeekURL() string {
	if c.scheduleURL != "" {
		return c.scheduleURL
	}
	return weekURL
}

// GameStatsURL returns the game stats URL template for this client.
func (c *Client) GameStatsURL() string {
	if c.gameStatsURL != "" {
		return c.gameStatsURL
	}
	return gameStatsURL
}

// TeamInfoURL returns the team info URL for this client.
func (c *Client) TeamInfoURL() string {
	if c.teamInfoURL != "" {
		return c.teamInfoURL
	}
	return teamInfoURL
}

// ScoreboardURL returns the scoreboard URL for this client.
func (c *Client) ScoreboardURL() string {
	if c.scoreboardURL != "" {
		return c.scoreboardURL
	}
	return scoreboardURL
}

type validatable interface {
	validate() error
}

// Responses constrains the response types makeRequest can decode. Every
// ESPN response implements validate, so decoded payloads are checked at the
// transport boundary.
type Responses interface {
	GameInfoESPN | GameScheduleESPN | TeamInfoESPN | ScoreboardESPN
	validatable
}

// makeRequest fetches endpoint and decodes the JSON body into out. It retries
// 5xx responses with exponential backoff and aborts as soon as ctx is
// cancelled. out is validated after decoding.
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
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/54.0.2840.90 Safari/537.36",
		"Accept": "application/json",
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var res *http.Response
	for attempt := range c.MaxRetries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("request cancelled for %q: %w", endpoint, err)
		}

		res, err = httpClient.Do(req)
		if err == nil {
			if res.StatusCode >= 500 {
				res.Body.Close()
				err = fmt.Errorf("unexpected status %d from %q", res.StatusCode, endpoint)
				if sleepErr := c.sleep(ctx, c.backoff(attempt)); sleepErr != nil {
					return fmt.Errorf("request cancelled for %q: %w", endpoint, sleepErr)
				}
				continue
			}
			break
		}
		if ctx.Err() != nil {
			return fmt.Errorf("request cancelled for %q: %w", endpoint, ctx.Err())
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
		return fmt.Errorf("decoding response from %q: %w", endpoint, err)
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
