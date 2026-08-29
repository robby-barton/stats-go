// Package load builds ranking inputs from the database. It is the persistence
// edge of the ranking pipeline: everything the algorithm needs (teams, games,
// and the resolved ranking window) is loaded here once, so the ranking
// package itself stays free of GORM.
package load

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/sport"
)

// boundaryLoc is the fixed timezone in which the week boundary DATE is
// derived. The cutoff has always been midnight Eastern (the pre-refactor
// code used the server's local zone, America/New_York in production); a
// midnight-UTC cutoff silently drops games played 7pm-midnight Eastern on
// the boundary day, which matters for basketball's late tips. Pinning the
// zone keeps rankings deterministic across servers while matching the old
// behavior. Falls back to UTC if the zone database is unavailable.
var boundaryLoc = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// Options configures what Input loads.
type Options struct {
	DB    *gorm.DB
	Sport sport.Sport

	// Year/Week identify the ranking window. Year 0 resolves to the latest
	// season in team_seasons; Week 0 resolves to the latest available week.
	Year int64
	Week int64

	// Fcs selects the lower-division ranking (football only).
	Fcs bool
}

// Input resolves the ranking window and loads the teams and games the
// ranking pipeline computes over, replicating the window semantics of the
// former Ranker.setGlobals.
func Input(opts Options) (ranking.Input, error) {
	if opts.DB == nil {
		return ranking.Input{}, errors.New("load: nil DB")
	}
	if err := opts.Sport.Validate(); err != nil {
		return ranking.Input{}, err
	}

	in := ranking.Input{
		Sport: opts.Sport,
		Fcs:   opts.Fcs,
		Week:  opts.Week,
	}

	// Resolve the season when not given explicitly.
	year := opts.Year
	if year == 0 {
		if err := opts.DB.Model(database.TeamSeason{}).
			Where("sport = ?", opts.Sport).
			Select("max(year) as year").Pluck("year", &year).Error; err != nil {
			return ranking.Input{}, err
		}
	}
	in.Year = year

	// Resolve the week and the window start time.
	var game database.Game
	if opts.Week > 0 {
		if err := opts.DB.
			Where("sport = ? and season = ? and week = ? and postseason = 0", opts.Sport, year, opts.Week).
			Order("start_time asc").
			Limit(1).
			Find(&game).Error; err != nil {
			return ranking.Input{}, err
		}
		if game != (database.Game{}) {
			// The boundary date is a local-timezone concept: derive the
			// Tuesday of the week in the fixed boundary zone (a late game
			// can land on a different UTC calendar day), then express
			// midnight of that date as a UTC instant for comparisons.
			et := game.StartTime.In(boundaryLoc)
			y, m, d := et.
				AddDate(0, 0, -int(et.Weekday()-time.Tuesday)).Date()
			in.StartTime = time.Date(y, m, d, 0, 0, 0, 0, boundaryLoc).UTC()
		} else {
			in.Week = 0
		}
	}

	if game == (database.Game{}) {
		if err := opts.DB.
			Where("sport = ? and season <= ?", opts.Sport, year).
			Order("start_time desc").
			Limit(1).
			Find(&game).Error; err != nil {
			return ranking.Input{}, err
		}
	}

	if game.Season < year {
		in.Week = 1
	} else {
		if in.Week == 0 {
			in.Week = game.Week + 1
		}

		if game.Postseason > 0 {
			in.Postseason = true
		}
	}

	if in.StartTime.IsZero() {
		in.StartTime = game.StartTime
	}

	teams, err := loadTeams(opts.DB, opts.Sport, year)
	if err != nil {
		return ranking.Input{}, err
	}
	in.Teams = teams

	games, err := loadGames(opts.DB, opts.Sport, year, in.StartTime)
	if err != nil {
		return ranking.Input{}, err
	}
	in.Games = games

	in.Backfill = backfillFunc(opts.DB, opts.Sport)

	return in, nil
}

// loadTeams loads every team for the sport/year (team_names ⋈ team_seasons),
// including the division flag so the ranking can select FBS or FCS.
func loadTeams(db *gorm.DB, sp sport.Sport, year int64) ([]ranking.TeamInfo, error) {
	var rows []struct {
		TeamID int64
		Name   string
		Conf   string
		FBS    int64
	}

	if err := db.Model(&database.TeamSeason{}).
		Select("team_names.team_id as team_id, team_names.name as name, team_seasons.conf as conf, team_seasons.fbs as fbs").
		Joins("join team_names on team_seasons.team_id = team_names.team_id and team_seasons.sport = team_names.sport").
		Where("team_seasons.year = ? and team_seasons.sport = ?", year, sp).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	teams := make([]ranking.TeamInfo, 0, len(rows))
	for _, row := range rows {
		teams = append(teams, ranking.TeamInfo{
			ID:   row.TeamID,
			Name: row.Name,
			Conf: row.Conf,
			FBS:  row.FBS != 0,
		})
	}
	return teams, nil
}

// loadGames loads the games available for computation: the sport's season
// window (as far back as the ranking reaches, YearsBack) up to the window
// start time, ordered by start time descending (the srs game selection
// depends on this order).
func loadGames(db *gorm.DB, sp sport.Sport, year int64, startTime time.Time) ([]ranking.Game, error) {
	var rows []database.Game
	if err := db.
		Where("sport = ? and season >= ? and start_time <= ?", sp, year-ranking.YearsBack(sp), startTime).
		Order("start_time desc").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	games := make([]ranking.Game, 0, len(rows))
	for _, row := range rows {
		games = append(games, ranking.Game{
			GameID:    row.GameID,
			Season:    row.Season,
			Week:      row.Week,
			StartTime: row.StartTime,
			HomeID:    row.HomeID,
			HomeScore: row.HomeScore,
			AwayID:    row.AwayID,
			AwayScore: row.AwayScore,
		})
	}
	return games, nil
}

// backfillFunc returns the database-backed implementation of the ranking
// input's pre-window game backfill (the "James Madison problem" search).
func backfillFunc(db *gorm.DB, sp sport.Sport) ranking.BackfillFunc {
	return func(teamID int64, opponents []int64, beforeSeason int64, limit int) ([]ranking.Game, error) {
		var rows []database.Game
		if err := db.
			Where(
				"sport = ? and season < ? and ((home_id = ? and away_id in (?)) or "+
					"(away_id = ? and home_id in (?)))",
				sp,
				beforeSeason,
				teamID,
				opponents,
				teamID,
				opponents,
			).Limit(limit).Order("start_time desc").
			Find(&rows).Error; err != nil {
			return nil, err
		}

		games := make([]ranking.Game, 0, len(rows))
		for _, row := range rows {
			games = append(games, ranking.Game{
				GameID:    row.GameID,
				Season:    row.Season,
				Week:      row.Week,
				StartTime: row.StartTime,
				HomeID:    row.HomeID,
				HomeScore: row.HomeScore,
				AwayID:    row.AwayID,
				AwayScore: row.AwayScore,
			})
		}
		return games, nil
	}
}
