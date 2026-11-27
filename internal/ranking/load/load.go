// Package load builds ranking inputs from the database. It is the persistence
// edge of the ranking pipeline: everything the algorithm needs (teams, games,
// and the resolved ranking window) is loaded here once, so the ranking
// package itself stays free of GORM.
package load

import (
	"errors"
	"fmt"
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
//
//nolint:gochecknoglobals // immutable, computed once; loading per call adds no value
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

// Window describes the persisted game row that resolved the ranking window.
// It exposes values Input already resolves, through a narrow return path, so
// callers can gate on the actual loaded window without issuing another query.
type Window struct {
	// Found reports whether any persisted game resolved the window.
	Found bool
	// Season, Week, and Postseason are the cutoff row's raw column values.
	// Postseason is the raw phase: 0 regular season, >0 postseason, and -1
	// for a stored preseason game.
	Season     int64
	Week       int64
	Postseason int64
	// Kickoff is the cutoff row's raw start_time.
	Kickoff time.Time
	// StartTime is the resolved window start (Input.StartTime).
	StartTime time.Time
	// ResolvedWeek is the resolved Input.Week.
	ResolvedWeek int64
}

// Input resolves the ranking window and loads the teams and games the
// ranking pipeline computes over, replicating the window semantics of the
// former Ranker.setGlobals.
func Input(opts Options) (ranking.Input, error) {
	in, _, err := resolve(opts)
	return in, err
}

// InputWindow behaves exactly like Input and additionally reports the
// persisted cutoff row that resolved the window. It issues no query beyond
// the ones Input already runs.
func InputWindow(opts Options) (ranking.Input, Window, error) {
	return resolve(opts)
}

func resolve(opts Options) (ranking.Input, Window, error) {
	if opts.DB == nil {
		return ranking.Input{}, Window{}, errors.New("load: nil DB")
	}
	if err := opts.Sport.Validate(); err != nil {
		return ranking.Input{}, Window{}, err
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
			return ranking.Input{}, Window{}, err
		}
	}
	in.Year = year

	// Resolve the week and the window start time.
	var game database.Game
	if opts.Week > 0 {
		q := opts.DB.Where("sport = ? and season = ? and week = ? and postseason = 0", opts.Sport, year, opts.Week)
		if opts.Sport == sport.WomensBasketball {
			// ESPN sometimes starts Week 1 on the Sunday before the first
			// Monday slate. Anchor on the latest game, not that opener.
			q = q.Where("retry = 0").Order("start_time desc")
		} else {
			q = q.Order("start_time asc")
		}
		if err := q.Limit(1).Find(&game).Error; err != nil {
			return ranking.Input{}, Window{}, err
		}
		if game != (database.Game{}) {
			// The boundary date is a local-timezone concept: derive the
			// cutoff day in the fixed boundary zone (a late game can land
			// on a different UTC calendar day), then express midnight of
			// that date as a UTC instant for comparisons.
			in.StartTime = weekCutoff(opts.Sport, game.StartTime)
		} else {
			in.Week = 0
			if opts.Sport == sport.WomensBasketball {
				// An explicit women's regular week with no stored games must
				// not silently resolve to the latest window: the strict
				// backfill enumerates the stored regular weeks, so an absent
				// week (or season) is a data problem, not a latest request.
				return ranking.Input{}, Window{}, fmt.Errorf(
					"load: no %s regular games for season %d week %d",
					opts.Sport, year, opts.Week)
			}
		}
	}

	if game == (database.Game{}) {
		if opts.Sport == sport.WomensBasketball {
			// The women's latest window resolves from the selected season's
			// actual rankable games only: never a phantom preview built from
			// a previous season, an unparsed retry placeholder, or a
			// preseason exhibition.
			if err := opts.DB.
				Where("sport = ? and season = ? and retry = 0", opts.Sport, year).
				Where("postseason = 1 or (postseason = 0 and week > 0)").
				Order("start_time desc, postseason desc").
				Limit(1).
				Find(&game).Error; err != nil {
				return ranking.Input{}, Window{}, err
			}
			if game == (database.Game{}) {
				return ranking.Input{}, Window{}, fmt.Errorf(
					"load: no %s rankable games for season %d", opts.Sport, year)
			}
		} else if err := opts.DB.
			Where("sport = ? and season <= ?", opts.Sport, year).
			Order("start_time desc").
			Limit(1).
			Find(&game).Error; err != nil {
			return ranking.Input{}, Window{}, err
		}
	}

	if game.Season < year {
		in.Week = 1
	} else {
		if in.Week == 0 {
			in.Week = game.Week + 1
			if opts.Sport == sport.WomensBasketball {
				// Women's regular rankings are labeled by the week they
				// cover. Finals always use week zero, regardless of which
				// week (if any) ESPN assigns a postseason event.
				in.Week = game.Week
				if game.Postseason == 1 {
					in.Week = 0
				}
			}
		}

		if game.Postseason > 0 {
			in.Postseason = true
		}
	}

	if in.StartTime.IsZero() {
		in.StartTime = game.StartTime
	}

	window := Window{
		Found:        game != (database.Game{}),
		Season:       game.Season,
		Week:         game.Week,
		Postseason:   game.Postseason,
		Kickoff:      game.StartTime,
		StartTime:    in.StartTime,
		ResolvedWeek: in.Week,
	}

	teams, err := loadTeams(opts.DB, opts.Sport, year)
	if err != nil {
		return ranking.Input{}, Window{}, err
	}
	in.Teams = teams

	q := baseGameQuery(opts.DB, opts.Sport, year, in.StartTime)
	if opts.Sport == sport.WomensBasketball && opts.Week > 0 {
		q = q.Where("start_time < ?", in.StartTime).
			Where("season < ? or (postseason = 0 and week <= ?)", year, opts.Week)
	}
	games, err := loadGameRows(q)
	if err != nil {
		return ranking.Input{}, Window{}, err
	}
	in.Games = games

	// Women's publication audits the configured one-season lookback. Do not
	// supplement those inputs with earlier, unaudited database rows.
	if opts.Sport != sport.WomensBasketball {
		in.Backfill = backfillFunc(opts.DB, opts.Sport)
	}

	return in, window, nil
}

// weekCutoff resolves a regular week's ranking cutoff from its anchor game.
//
// Football and men's basketball keep the historical Tuesday derivation: the
// cutoff is midnight of the Tuesday of the game's calendar week, so the
// snapshot covers that calendar week up to the Tuesday midnight.
//
// Women's basketball instead opens each week at Monday 00:00 Eastern (live
// ESPN summary checks: Sundays 2026-11-08/15/22 are weeks 1/2/3 while the
// adjacent Mondays are weeks 2/3/4, and a Tuesday game still belongs to the
// Monday's week). The cutoff is therefore the Monday 00:00 Eastern that opens
// the calendar week after the game's week. That keeps a late Sunday tip in its
// own week, keeps the following Monday's games out of it, and survives DST
// because the boundary is built as local midnight. Using the latest game as
// the anchor also handles ESPN's occasional Sunday Week 1 opener.
func weekCutoff(sp sport.Sport, startTime time.Time) time.Time {
	et := startTime.In(boundaryLoc)
	if sp != sport.WomensBasketball {
		y, m, d := et.
			AddDate(0, 0, -int(et.Weekday()-time.Tuesday)).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, boundaryLoc).UTC()
	}
	weekOpen := time.Date(et.Year(), et.Month(), et.Day(), 0, 0, 0, 0, boundaryLoc).
		AddDate(0, 0, -(int(et.Weekday())+6)%7) // Monday 00:00 Eastern of the game's week
	return weekOpen.AddDate(0, 0, 7).UTC() // Monday 00:00 Eastern opening the next week
}

// loadTeams loads every team for the sport/year (team_names ⋈ team_seasons),
// including the division flag so the ranking can select FBS or FCS.
func loadTeams(db *gorm.DB, sp sport.Sport, year int64) ([]ranking.TeamInfo, error) {
	return loadTeamRows(teamQuery(db, sp, year))
}

// teamQuery returns the shared production team predicate: the sport/year
// team_names ⋈ team_seasons join. Replay loading reuses this exact query so
// explicit-cutoff snapshots cannot drift from production resolution.
func teamQuery(db *gorm.DB, sp sport.Sport, year int64) *gorm.DB {
	return db.Model(&database.TeamSeason{}).
		Select("team_names.team_id as team_id, team_names.name as name, team_seasons.conf as conf, team_seasons.fbs as fbs").
		Joins("join team_names on team_seasons.team_id = team_names.team_id and team_seasons.sport = team_names.sport").
		Where("team_seasons.year = ? and team_seasons.sport = ?", year, sp)
}

func loadTeamRows(query *gorm.DB) ([]ranking.TeamInfo, error) {
	var rows []struct {
		TeamID int64
		Name   string
		Conf   string
		FBS    int64
	}

	if err := query.Scan(&rows).Error; err != nil {
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

// baseGameQuery returns games in the sport's season window through the cutoff,
// in descending order (SRS game selection depends on this order).
// Only women’s basketball pins both season bounds; the historical loaders and
// FBS replay retain the legacy predicate.
func baseGameQuery(db *gorm.DB, sp sport.Sport, year int64, startTime time.Time) *gorm.DB {
	q := db.Where("sport = ? and season >= ? and start_time <= ?", sp, year-ranking.YearsBack(sp), startTime)
	if sp == sport.WomensBasketball {
		// Women's rankable games exclude unparsed retry placeholders and
		// preseason exhibitions; the other sports keep the unfiltered
		// historical predicate.
		q = q.Where("season <= ? and retry = 0", year).
			Where("postseason = 1 or (postseason = 0 and week > 0)")
	}
	return q.Order("start_time desc")
}

// olderGameQuery returns the shared production backfill predicate: prior-season
// games for a team against any of its opponents, most recent first, bounded by
// limit. There is deliberately no historical-year floor.
func olderGameQuery(db *gorm.DB, sp sport.Sport, teamID int64, opponents []int64,
	beforeSeason int64, limit int,
) *gorm.DB {
	return db.Where(
		"sport = ? and season < ? and ((home_id = ? and away_id in (?)) or "+
			"(away_id = ? and home_id in (?)))",
		sp, beforeSeason, teamID, opponents, teamID, opponents,
	).Limit(limit).Order("start_time desc")
}

func loadGameRows(query *gorm.DB) ([]ranking.Game, error) {
	var rows []database.Game
	if err := query.Find(&rows).Error; err != nil {
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
		return loadGameRows(olderGameQuery(db, sp, teamID, opponents, beforeSeason, limit))
	}
}
