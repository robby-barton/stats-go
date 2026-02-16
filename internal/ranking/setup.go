package ranking

import (
	"time"

	"github.com/robby-barton/stats-go/internal/database"
)

func (r *Ranker) setup() (TeamList, error) {
	if err := r.setGlobals(); err != nil {
		return nil, err
	}

	var teamList TeamList
	var err error

	// FCS ranking only applies to football; basketball has no division split.
	if r.Fcs && r.Sport != sportBasketball {
		teamList, err = r.createTeamList(0)
	} else {
		teamList, err = r.createTeamList(1)
	}
	if err != nil {
		return nil, err
	}

	return teamList, nil
}

func (r *Ranker) setGlobals() error {
	sport := r.sportFilter()

	if r.Year == 0 {
		var year int64
		if err := r.DB.Model(database.TeamSeason{}).
			Where("sport = ?", sport).
			Select("max(year) as year").Pluck("year", &year).Error; err != nil {
			return err
		}
		r.Year = year
	}

	var game database.Game
	if r.Week > 0 {
		if err := r.DB.
			Where("sport = ? and season = ? and week = ? and postseason = 0", sport, r.Year, r.Week).
			Order("start_time asc").
			Limit(1).
			Find(&game).Error; err != nil {
			return err
		}
		if game != (database.Game{}) {
			y, m, d := game.StartTime.
				AddDate(0, 0, -int(game.StartTime.Weekday()-time.Tuesday)).Date()
			r.startTime = time.Date(y, m, d, 0, 0, 0, 0, time.Local)
		} else {
			r.Week = 0
		}
	}

	if game == (database.Game{}) {
		if err := r.DB.
			Where("sport = ? and season <= ?", sport, r.Year).
			Order("start_time desc").
			Limit(1).
			Find(&game).Error; err != nil {
			return err
		}
	}

	if game.Season < r.Year {
		r.Week = 1
	} else {
		if r.Week == 0 {
			r.Week = game.Week + 1
		}

		if game.Postseason > 0 {
			r.postseason = true
		}
	}

	if r.startTime.Equal((time.Time{})) {
		r.startTime = game.StartTime
	}

	return nil
}

func (r *Ranker) createTeamList(findFbs int64) (TeamList, error) {
	teams := []struct {
		TeamID int64
		Name   string
		Conf   string
	}{}

	if err := r.DB.Model(&database.TeamSeason{}).
		Select("team_names.team_id, team_names.name, team_seasons.conf").
		Joins("join team_names on team_seasons.team_id = team_names.team_id and team_seasons.sport = team_names.sport").
		Where("team_seasons.fbs = ? and team_seasons.year = ? and team_seasons.sport = ?",
			findFbs, r.Year, r.sportFilter()).
		Scan(&teams).Error; err != nil {
		return nil, err
	}

	teamList := TeamList{}
	for _, team := range teams {
		teamList[team.TeamID] = &Team{
			Name: team.Name,
			Conf: team.Conf,
			Year: r.Year,
			Week: r.Week,
		}
		if r.postseason {
			teamList[team.TeamID].Postseason = 1
		}
	}

	return teamList, nil
}
