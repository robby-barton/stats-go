package espn

import "time"

// ConferenceMapResult holds conference data returned by ConferenceMap.
type ConferenceMapResult struct {
	// Conferences maps group → (conference ID → short name).
	// Football populates FBS and FCS. Basketball populates D1Basketball.
	Conferences map[Group]map[int64]string

	// SubGroups maps group → sub-group IDs. Only used by football (DII, DIII).
	SubGroups map[Group][]int64
}

// SportClient is the interface for sport-specific ESPN API interactions.
// Both FootballClient and BasketballClient implement it.
type SportClient interface {
	// Metadata
	SportInfo() Sport
	RateLimitDuration() time.Duration

	// Game data (sport-agnostic)
	GetCurrentWeekGames(group Group) ([]Game, error)
	GetGameStats(gameID int64) (*GameInfoESPN, error)
	GetTeamInfo() (*TeamInfoESPN, error)

	// Season navigation (sport-specific)
	DefaultSeason() (int64, error)
	GetWeeksInSeason(year int64) (int64, error)
	HasPostseasonStarted(year int64, startTime time.Time) (bool, error)
	GetGamesBySeason(year int64, group Group) ([]Game, error)
	TeamConferencesByYear(year int64) (map[int64]int64, error)
	ConferenceMap() (ConferenceMapResult, error)
}

// SportInfo returns the sport this client is configured for.
func (c *Client) SportInfo() Sport {
	return c.Sport
}

// RateLimitDuration returns the delay between batch API calls.
func (c *Client) RateLimitDuration() time.Duration {
	return c.RateLimit
}
