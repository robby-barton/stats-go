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

// GetSeasonDatesForYear returns the list of game dates from the scoreboard
// calendar for the given season year. Each date is an ISO 8601 timestamp
// (e.g. "2025-11-03T08:00Z"). The calendar is only included in the payload
// when a `dates` query parameter is passed (verified 2026-08-29); the plain
// scoreboard response omits it entirely, so the year must be supplied.
func (c *Client) GetSeasonDatesForYear(ctx context.Context, year int64) ([]string, error) {
	url := c.ScoreboardURL() + fmt.Sprintf("?dates=%d", year)

	var res ScoreboardESPN
	if err := makeRequest(ctx, c, url, &res); err != nil {
		return nil, err
	}
	return res.Leagues[0].Calendar, nil
}

// GetGameStats fetches a single game's box score off the site.api summary
// endpoint (both sports since the basketball migration of 2026-08-30; see
// GameInfoESPN for the shape). The summary carries the game start time in
// header.competitions[0].date, which the game parser consumes alongside the
// scoreboard data.
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

// extractTeamConfs collects team ID → conference ID from site.api scoreboard
// events. Competitors without a conferenceId (transition D-II schools,
// non-D1 fill-ins — verified live 2026-08-30 on the basketball scoreboard:
// 1 of 290 competitors on 2026-01-17) decode to 0 and are skipped.
func extractTeamConfs(events []SiteEvent) map[int64]int64 {
	teamConfs := map[int64]int64{}

	for _, event := range events {
		for _, comp := range event.Competitions {
			for _, team := range comp.Competitors {
				if team.Team.ConferenceID != 0 {
					teamConfs[team.Team.ID] = int64(team.Team.ConferenceID)
				}
			}
		}
	}

	return teamConfs
}
