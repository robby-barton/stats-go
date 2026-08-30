package espn

import (
	"context"
	"time"
)

// ConferenceMapResult holds conference data returned by ConferenceMap.
type ConferenceMapResult struct {
	// Conferences maps group → (conference ID → short name).
	// Football populates FBS and FCS. Basketball populates D1Basketball.
	Conferences map[Group]map[int64]string

	// SubGroups maps group → sub-group IDs. Only used by football (DII, DIII).
	SubGroups map[Group][]int64
}

// SportClient is the interface for sport-specific ESPN API interactions.
// Both FootballClient and BasketballClient implement it. Every method takes a
// context so callers (e.g. the scheduler) can abort in-flight requests.
type SportClient interface {
	// Metadata
	SportInfo() Sport
	Throttle(ctx context.Context)

	// Game data (sport-agnostic)
	GetCurrentWeekGames(ctx context.Context, group Group) ([]Game, error)
	GetGameStats(ctx context.Context, gameID int64) (*GameInfoESPN, error)
	GetTeamInfo(ctx context.Context) (*TeamInfoESPN, error)

	// Season navigation (sport-specific)
	DefaultSeason(ctx context.Context) (int64, error)
	GetWeeksInSeason(ctx context.Context, year int64) (int64, error)
	HasPostseasonStarted(ctx context.Context, year int64, startTime time.Time) (bool, error)
	GetGamesBySeason(ctx context.Context, year int64, group Group) ([]Game, error)
	TeamConferencesByYear(ctx context.Context, year int64) (map[int64]int64, error)
	ConferenceMap(ctx context.Context) (ConferenceMapResult, error)
}

// SportInfo returns the sport this client is configured for.
func (c *Client) SportInfo() Sport {
	return c.Sport
}

// Throttle pauses between sequential ESPN API calls so every multi-request
// path (week loops, date loops, per-game fetches) shares one rate-limit
// policy. Call it after each request in a batch. The pause is cut short when
// ctx is cancelled.
func (c *Client) Throttle(ctx context.Context) {
	_ = c.sleep(ctx, c.RateLimit)
}
