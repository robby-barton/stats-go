package espn

import (
	"context"
	"fmt"
	"time"
)

// FootballClient wraps a shared *Client with football-specific season logic.
type FootballClient struct{ *Client }

// Compile-time interface check.
var _ SportClient = (*FootballClient)(nil)

// GetCurrentWeekGames fetches completed games for the current week from the
// site.api scoreboard, filtered server-side to the given division group via
// the `groups` query parameter (verified for FBS 80 / FCS 81; the cdn-style
// `group` parameter is indeed ignored by site.api).
func (fc *FootballClient) GetCurrentWeekGames(ctx context.Context, group Group) ([]Game, error) {
	url := fc.ScoreboardURL() + fmt.Sprintf("?groups=%d", group)

	var res SiteScoreboardESPN
	if err := makeRequest(ctx, fc.Client, url, &res); err != nil {
		return nil, err
	}

	return res.finalGames()
}

// DefaultSeason returns the current ESPN season year from the site.api
// scoreboard's leagues[0].season object.
func (fc *FootballClient) DefaultSeason(ctx context.Context) (int64, error) {
	var res SiteScoreboardESPN
	err := makeRequest(ctx, fc.Client, fc.ScoreboardURL(), &res)
	if err != nil {
		return 0, err
	}

	return res.Leagues[0].Season.Year, nil
}

// getCalendarForYear fetches the season calendar for a year off the site.api
// scoreboard. The calendar is only included when a `dates` parameter is
// passed (verified 2026-08-29); the plain scoreboard response omits it.
func (fc *FootballClient) getCalendarForYear(ctx context.Context, year int64) (*SiteScoreboardESPN, error) {
	url := fc.ScoreboardURL() + fmt.Sprintf("?dates=%d", year)

	var res SiteScoreboardESPN
	if err := makeRequest(ctx, fc.Client, url, &res); err != nil {
		return nil, err
	}
	if res.calendarType(Regular) == nil && res.calendarType(Postseason) == nil {
		return nil, fmt.Errorf("scoreboard response missing calendar for year %d", year)
	}
	return &res, nil
}

// GetWeeksInSeason returns the number of regular-season weeks from the
// scoreboard calendar. The Regular Season entry lists one entry per week,
// numbered 1..N (the postseason is a separate calendar entry).
func (fc *FootballClient) GetWeeksInSeason(ctx context.Context, year int64) (int64, error) {
	sb, err := fc.getCalendarForYear(ctx, year)
	if err != nil {
		return 0, err
	}

	regular := sb.calendarType(Regular)
	if len(regular.Entries) == 0 {
		return 0, fmt.Errorf("calendar has no regular-season week entries for year %d", year)
	}

	return int64(len(regular.Entries)), nil
}

// HasPostseasonStarted reports whether the postseason phase of the given
// season has begun by startTime. It compares against the Postseason calendar
// entry's start date (the cdn implementation used the same date via
// calendar[1]; matching by season-type value is equivalent but robust
// against entry reordering).
func (fc *FootballClient) HasPostseasonStarted(ctx context.Context, year int64, startTime time.Time) (bool, error) {
	sb, err := fc.getCalendarForYear(ctx, year)
	if err != nil {
		return false, err
	}

	postseason := sb.calendarType(Postseason)
	postSeasonStart, _ := time.Parse("2006-01-02T15:04Z", postseason.StartDate)
	return postSeasonStart.Before(startTime), nil
}

func (fc *FootballClient) GetGamesBySeason(ctx context.Context, year int64, group Group) ([]Game, error) {
	var allGames []Game

	numWeeks, err := fc.GetWeeksInSeason(ctx, year)
	if err != nil {
		return nil, err
	}

	// GetWeeksInSeason returns the number of regular-season weeks (calendar
	// entry 0 excludes the postseason), numbered 1..N. Fetch every one of them;
	// postseason week 1 is fetched separately below.
	for i := int64(1); i <= numWeeks; i++ {
		games, err := fc.GetCompletedGamesByWeek(ctx, year, i, group, Regular)
		if err != nil {
			return nil, err
		}

		allGames = append(allGames, games...)
		fc.Throttle(ctx)
	}

	games, err := fc.GetCompletedGamesByWeek(ctx, year, int64(1), group, Postseason)
	if err != nil {
		return nil, err
	}

	allGames = append(allGames, games...)

	return allGames, nil
}

// parseESPNTime parses scoreboard calendar timestamps, which omit seconds
// (e.g. "2026-08-22T07:00Z") unlike strict RFC 3339.
func parseESPNTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z", "2006-01-02T15:04:05Z0700", "2006-01-02"} {
		t, err := time.Parse(layout, v)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", v)
}

// TeamConferencesByYear builds team ID -> conference ID for every team that
// plays in the given season. The site.api scoreboard honours a `dates` range
// parameter (verified 2026-08-30; `week`/`year` are ignored) and each
// competitor carries team.conferenceId, so the season's calendar span is
// walked in weekly chunks per division group. Postseason is included: a few
// teams only appear there (e.g. bowl-only scheduling edge cases).
func (fc *FootballClient) TeamConferencesByYear(ctx context.Context, year int64) (map[int64]int64, error) {
	cal, err := fc.getCalendarForYear(ctx, year)
	if err != nil {
		return nil, err
	}

	teamConfs := map[int64]int64{}
	for _, seasonType := range []SeasonType{Regular, Postseason} {
		ct := cal.calendarType(seasonType)
		if ct == nil {
			continue
		}
		start, err := parseESPNTime(ct.StartDate)
		if err != nil {
			return nil, fmt.Errorf("calendar start date %q: %w", ct.StartDate, err)
		}
		end, err := parseESPNTime(ct.EndDate)
		if err != nil {
			return nil, fmt.Errorf("calendar end date %q: %w", ct.EndDate, err)
		}

		for _, group := range fc.Sport.Groups() {
			for cur := start; !cur.After(end); cur = cur.AddDate(0, 0, 7) {
				chunkEnd := cur.AddDate(0, 0, 6)
				if chunkEnd.After(end) {
					chunkEnd = end
				}
				url := fc.ScoreboardURL() + fmt.Sprintf("?dates=%s-%s&groups=%d",
					cur.Format("20060102"), chunkEnd.Format("20060102"), group)
				var res SiteScoreboardESPN
				if err := makeRequest(ctx, fc.Client, url, &res); err != nil {
					return nil, err
				}
				for _, ev := range res.Events {
					for _, comp := range ev.Competitions {
						for _, c := range comp.Competitors {
							if c.Team.ConferenceID != 0 {
								teamConfs[c.Team.ID] = int64(c.Team.ConferenceID)
							}
						}
					}
				}
				fc.Throttle(ctx)
			}
		}
	}

	if len(teamConfs) == 0 {
		return nil, fmt.Errorf("no teams found for the %d season", year)
	}
	return teamConfs, nil
}

// ConferenceMap fetches conference metadata off the site.api
// scoreboard/conferences endpoint, one request per division group (FBS, FCS,
// DII, DIII). The plain endpoint returns FBS only, so FCS/DII/DIII need an
// explicit `groups` value — verified live 2026-08-29. The DII/DIII
// "sub-groups" are the child conference group IDs of each division, matching
// what the cdn schedule's conferenceAPI subGroups arrays used to provide.
func (fc *FootballClient) ConferenceMap(ctx context.Context) (ConferenceMapResult, error) {
	fbsRes, err := fc.GetConferences(ctx, FBS)
	if err != nil {
		return ConferenceMapResult{}, err
	}
	fc.Throttle(ctx)

	fcsRes, err := fc.GetConferences(ctx, FCS)
	if err != nil {
		return ConferenceMapResult{}, err
	}
	fc.Throttle(ctx)

	diiRes, err := fc.GetConferences(ctx, DII)
	if err != nil {
		return ConferenceMapResult{}, err
	}
	fc.Throttle(ctx)

	diiiRes, err := fc.GetConferences(ctx, DIII)
	if err != nil {
		return ConferenceMapResult{}, err
	}

	return ConferenceMapResult{
		Conferences: map[Group]map[int64]string{ //nolint:exhaustive // football doesn't have D1Basketball
			FBS: fbsRes.conferenceShortNames(FBS),
			FCS: fcsRes.conferenceShortNames(FCS),
		},
		SubGroups: map[Group][]int64{ //nolint:exhaustive // only DII/DIII have sub-groups
			DII:  diiRes.conferenceSubGroups(DII),
			DIII: diiiRes.conferenceSubGroups(DIII),
		},
	}, nil
}
