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

// GetGamesBySeason fetches every completed game of a season for one division
// group off the site.api scoreboard: the season calendar (?dates=<year>) is
// fetched once, then the Regular and Postseason calendar spans are walked
// ONE DAY AT A TIME with scoreboard?dates=YYYYMMDD&groups=N (the endpoint
// honors `groups`; verified for FBS 80 / FCS 81), collecting STATUS_FINAL
// games via finalGames. Callers dedupe across division groups.
//
// Why per-day and not the weekly date-RANGE chunks TeamConferencesByYear
// uses: live parity-check 2026-08-30 (2025 season, both groups) showed the
// range endpoint ?dates=start-end is lossy against single-day fetches in
// ways the 200-event cap does not explain — a 7-day range dropped the
// tail-day evening games (e.g. Friday-night games) and sometimes lost whole
// days mid-range (Aug30-Sep05 range returned nothing after Sep02, and
// Aug29-30 returned only the Aug29 games while dropping all of Aug30). A
// full range-walk found 1602 of the 1697 cdn-ingested games (95 lost,
// ~5.6%); a per-day walk found 1698 — superset of the cdn data minus one
// out-of-group D-II matchup the scoreboard never surfaces under groups
// 80/81. See docs/espn-api.md (Scoreboard section) for the characterization.
func (fc *FootballClient) GetGamesBySeason(ctx context.Context, year int64, group Group) ([]Game, error) {
	cal, err := fc.getCalendarForYear(ctx, year)
	if err != nil {
		return nil, err
	}

	var allGames []Game
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

		// Walk the span one calendar day at a time (inclusive); single-day
		// fetches cover the full ET day, so the union is complete. Both bounds
		// are truncated to UTC midnight: the calendar timestamps carry a
		// time-of-day (spans start ~07:00Z/08:00Z and end ~07:59Z), and
		// stepping from the raw start would drift every probe past the end
		// bound and silently drop the final day.
		startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
			url := fc.ScoreboardURL() + fmt.Sprintf("?dates=%s&groups=%d",
				day.Format("20060102"), group)
			var res SiteScoreboardESPN
			if err := makeRequest(ctx, fc.Client, url, &res); err != nil {
				return nil, err
			}
			games, err := res.finalGames()
			if err != nil {
				return nil, err
			}
			allGames = append(allGames, games...)
			fc.Throttle(ctx)
		}
	}

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
//
// Known hazards of the scoreboard ?dates=start-end range endpoint (verified
// live 2026-08-30 across seasons 2021-2025):
//
//  1. Sunday-start ranges are degenerate: any range that STARTS on a Sunday
//     returns 0-10 events instead of the 53-89 expected, deterministically.
//     weekChunks therefore shifts Sunday chunk starts to Monday. Regular-
//     season calendar starts are Saturday-anchored, but the postseason
//     calendar starts on a Sunday in 2022, 2024, and 2026, which is why the
//     shift matters for bowl-only team collection.
//  2. Responses are hard-capped at 200 events with silent chronological
//     truncation. Weekly chunks keep each request well under the cap; do
//     not widen them into multi-week ranges.
//
// Degenerate-chunk detection is NOT implemented: Client carries no logger,
// so a chunk that unexpectedly returns 0 events (e.g. if ESPN shifts the
// Sunday hazard to other start days) cannot be surfaced from here. If this
// walk ever yields an implausibly low team count for a season, re-verify the
// range endpoint behavior with curl before trusting the results.
//
// Other verified quirks: ranges crossing the regular/postseason boundary
// return both season types cleanly (harmless here — conference IDs are
// identical), and single-day fetches are not supersets of ranges.
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
			for _, chunk := range weekChunks(start, end) {
				cur, chunkEnd := chunk[0], chunk[1]
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

// weekChunks splits the inclusive [start, end] span into scoreboard date-
// range chunks of at most 7 days, covering end inclusive. A chunk start that
// lands on a Sunday is shifted forward one day (to Monday) because the
// scoreboard ?dates= range endpoint returns degenerate subsets (0-10 events
// instead of 53-89) for any range starting on a Sunday — verified live
// 2026-08-30 across all seasons 2021-2025. Because 7-day stepping preserves
// the weekday, a Sunday can only surface as the span start itself (shifted
// here) or as a chunk end; a span consisting solely of one Sunday therefore
// yields no chunks — there is no non-degenerate way to fetch it. Every other
// span covers end inclusive. See TeamConferencesByYear for the full hazard
// list.
func weekChunks(start, end time.Time) [][2]time.Time {
	var chunks [][2]time.Time
	for cur := start; !cur.After(end); {
		if cur.Weekday() == time.Sunday {
			cur = cur.AddDate(0, 0, 1)
			if cur.After(end) {
				break
			}
		}
		chunkEnd := cur.AddDate(0, 0, 6)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		chunks = append(chunks, [2]time.Time{cur, chunkEnd})
		cur = chunkEnd.AddDate(0, 0, 1)
	}
	return chunks
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
