package espn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robby-barton/stats-go/internal/sport"
)

// Sport identifies a college sport for ESPN API parameterization (the ESPN
// slug, e.g. "college-football"). The persistence identifier lives in the
// sport package; use DBSport to convert.
type Sport string

const (
	CollegeFootball   Sport = "college-football"
	CollegeBasketball Sport = "college-basketball"
)

// DBSport returns the persistence identifier ("ncaaf"/"ncaam") for the sport.
// Returns "" for unknown sports; clients built via NewClientForSport are
// always one of the known values.
func (s Sport) DBSport() sport.Sport {
	switch s {
	case CollegeBasketball:
		return sport.Basketball
	case CollegeFootball:
		return sport.Football
	default:
		return ""
	}
}

type Group int64
type SeasonType int64

const (
	FBS  Group = 80
	FCS  Group = 81
	DII  Group = 57
	DIII Group = 58

	// Basketball D1 group on ESPN.
	D1Basketball Group = 50

	Regular    SeasonType = 2
	Postseason SeasonType = 3
)

// Groups returns the division groups used for schedule fetching for a sport.
func (s Sport) Groups() []Group {
	switch s {
	case CollegeBasketball:
		return []Group{D1Basketball}
	case CollegeFootball:
		return []Group{FBS, FCS}
	default:
		panic(fmt.Sprintf("unknown sport: %q", s))
	}
}

// HasDivisionSplit returns true if the sport distinguishes divisions (e.g. FBS/FCS).
func (s Sport) HasDivisionSplit() bool {
	return s == CollegeFootball
}

func (c *Client) GetGamesByWeek(
	ctx context.Context, year int64, week int64, group Group, seasonType SeasonType,
) (*GameScheduleESPN, error) {
	url := c.WeekURL() +
		fmt.Sprintf("&year=%d&week=%d&group=%d&seasonType=%d", year, week, group, seasonType)

	var res GameScheduleESPN
	err := makeRequest(ctx, c, url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) GetCompletedGamesByWeek(
	ctx context.Context, year int64, week int64, group Group, seasonType SeasonType,
) ([]Game, error) {
	res, err := c.GetGamesByWeek(ctx, year, week, group, seasonType)
	if err != nil {
		return nil, err
	}

	return completedGames(res), nil
}

// GetGamesByDate fetches all games for a specific date (format YYYYMMDD).
// Used by basketball where the schedule endpoint is date-based, not week-based.
func (c *Client) GetGamesByDate(ctx context.Context, date string, group Group) (*GameScheduleESPN, error) {
	url := c.WeekURL() + fmt.Sprintf("&date=%s&group=%d", date, group)

	var res GameScheduleESPN
	if err := makeRequest(ctx, c, url, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// GetCompletedGamesByDate returns only completed (final) games for a date.
func (c *Client) GetCompletedGamesByDate(ctx context.Context, date string, group Group) ([]Game, error) {
	res, err := c.GetGamesByDate(ctx, date, group)
	if err != nil {
		return nil, err
	}
	return completedGames(res), nil
}

// GetSeasonDates returns the list of game dates from the scoreboard calendar.
// Each date is an ISO 8601 timestamp (e.g. "2025-11-03T08:00Z").
func (c *Client) GetSeasonDates(ctx context.Context) ([]string, error) {
	sb, err := c.GetScoreboard(ctx)
	if err != nil {
		return nil, err
	}
	return sb.Leagues[0].Calendar, nil
}

func completedGames(res *GameScheduleESPN) []Game {
	var games []Game
	for _, day := range res.Content.Schedule {
		for _, event := range day.Games {
			if event.Status.StatusType.Completed && event.Status.StatusType.Name == "STATUS_FINAL" {
				games = append(games, event)
			}
		}
	}
	return games
}

func (c *Client) GetGameStats(ctx context.Context, gameID int64) (*GameInfoESPN, error) {
	url := fmt.Sprintf(c.GameStatsURL(), gameID)

	var res GameInfoESPN
	err := makeRequest(ctx, c, url, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) GetTeamInfo(ctx context.Context) (*TeamInfoESPN, error) {
	var res TeamInfoESPN
	err := makeRequest(ctx, c, c.TeamInfoURL(), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

// dateToParam converts "2025-11-03T08:00Z" to "20251103".
func dateToParam(isoDate string) string {
	t, err := time.Parse("2006-01-02T15:04Z", isoDate)
	if err != nil {
		if len(isoDate) < 10 {
			return ""
		}
		// Best-effort: strip non-digits from the date portion.
		return strings.ReplaceAll(isoDate[:10], "-", "")
	}
	return t.Format("20060102")
}

func extractTeamConfs(games *GameScheduleESPN) map[int64]int64 {
	teamConfs := map[int64]int64{}

	for _, day := range games.Content.Schedule {
		for _, event := range day.Games {
			if len(event.Competitions) == 0 {
				continue
			}
			for _, team := range event.Competitions[0].Competitors {
				teamConfs[team.Team.ID] = team.Team.ConferenceID
			}
		}
	}

	return teamConfs
}
