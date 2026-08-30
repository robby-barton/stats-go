package espn

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

// FootballClient wraps a shared *Client with football-specific season logic.
type FootballClient struct{ *Client }

// Compile-time interface check.
var _ SportClient = (*FootballClient)(nil)

func (fc *FootballClient) DefaultSeason(ctx context.Context) (int64, error) {
	var res GameScheduleESPN
	err := makeRequest(ctx, fc.Client, fc.WeekURL(), &res)
	if err != nil {
		return 0, err
	}

	return res.Content.Defaults.Year, nil
}

func (fc *FootballClient) GetWeeksInSeason(ctx context.Context, year int64) (int64, error) {
	url := fc.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := makeRequest(ctx, fc.Client, url, &res)
	if err != nil {
		return 0, err
	}

	if len(res.Content.Calendar) == 0 || len(res.Content.Calendar[0].Weeks) == 0 {
		return 0, fmt.Errorf("schedule response missing calendar/weeks for year %d", year)
	}

	return int64(len(res.Content.Calendar[0].Weeks)), nil
}

func (fc *FootballClient) HasPostseasonStarted(ctx context.Context, year int64, startTime time.Time) (bool, error) {
	url := fc.WeekURL() + fmt.Sprintf("&year=%d", year)

	var res GameScheduleESPN
	err := makeRequest(ctx, fc.Client, url, &res)
	if err != nil {
		return false, err
	}

	if len(res.Content.Calendar) < 2 {
		return false, fmt.Errorf("schedule response has %d calendar entries, need at least 2 for postseason",
			len(res.Content.Calendar))
	}

	postSeasonStart, _ := time.Parse("2006-01-02T15:04Z",
		res.Content.Calendar[1].StartDate)
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

func (fc *FootballClient) TeamConferencesByYear(ctx context.Context, year int64) (map[int64]int64, error) {
	teamConfs := map[int64]int64{}

	numWeeks, err := fc.GetWeeksInSeason(ctx, year)
	if err != nil {
		return nil, err
	}

	for _, group := range fc.Sport.Groups() {
		for i := int64(1); i <= numWeeks; i++ {
			games, err := fc.GetGamesByWeek(ctx, year, i, group, Regular)
			if err != nil {
				return nil, err
			}
			maps.Copy(teamConfs, extractTeamConfs(games))
			fc.Throttle(ctx)
		}

		games, err := fc.GetGamesByWeek(ctx, year, int64(1), group, Postseason)
		if err != nil {
			return nil, err
		}
		maps.Copy(teamConfs, extractTeamConfs(games))
	}

	return teamConfs, nil
}

func (fc *FootballClient) ConferenceMap(ctx context.Context) (ConferenceMapResult, error) {
	var res GameScheduleESPN
	err := makeRequest(ctx, fc.Client, fc.WeekURL(), &res)
	if err != nil {
		return ConferenceMapResult{}, err
	}

	conferences := res.Content.ConferenceAPI.Conferences

	fbs := map[int64]string{}
	fcs := map[int64]string{}
	dii := []int64{}
	diii := []int64{}

	for _, conference := range conferences {
		switch int64(conference.ParentGroupID) {
		case int64(FBS):
			fbs[conference.GroupID] = conference.ShortName
		case int64(FCS):
			fcs[conference.GroupID] = conference.ShortName
		default:
			if slices.Contains([]int64{int64(DII), int64(DIII)}, conference.GroupID) {
				for _, conf := range conference.SubGroups {
					group, _ := strconv.ParseInt(conf, 10, 64)
					switch conference.GroupID {
					case int64(DII):
						dii = append(dii, group)
					case int64(DIII):
						diii = append(diii, group)
					}
				}
			}
		}
	}

	return ConferenceMapResult{
		Conferences: map[Group]map[int64]string{ //nolint:exhaustive // football doesn't have D1Basketball
			FBS: fbs,
			FCS: fcs,
		},
		SubGroups: map[Group][]int64{ //nolint:exhaustive // only DII/DIII have sub-groups
			DII:  dii,
			DIII: diii,
		},
	}, nil
}
