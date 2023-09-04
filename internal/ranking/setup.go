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
	if r.Fcs {
		if teamList, err = r.createTeamList(0); err != nil {
			return nil, err
		}
	} else {
		if teamList, err = r.createTeamList(1); err != nil {
			return nil, err
		}
	}

	return teamList, nil
}

func (r *Ranker) setGlobals() error {
	if r.Year == 0 {
		var year int64
		if err := r.DB.Model(database.TeamSeason{}).
			Select("max(year) as year").Pluck("year", &year).Error; err != nil {
			return err
		}
		r.Year = year
	}

	var game database.Game
	if r.Week > 0 {
		if err := r.DB.
			Where("season = ? and week = ? and postseason = 0", r.Year, r.Week).
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
			Where("season <= ?", r.Year).
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

	if r.startTime == (time.Time{}) {
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
		Joins("left join team_names on team_seasons.team_id = team_names.team_id").
		Where("team_seasons.fbs = ? and team_seasons.year = ?", findFbs, r.Year).
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
