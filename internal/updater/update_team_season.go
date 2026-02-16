package updater

import (
	"maps"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func (u *Updater) insertSeasonToDB(seasons []database.TeamSeason) error {
	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			CreateInBatches(seasons, 1000).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *Updater) seasonsExist(year int64) bool {
	var count int64
	err := u.DB.Model(database.TeamSeason{}).Where("sport = ? and year = ?", u.sportDB(), year).Count(&count).Error
	if err != nil {
		u.Logger.Error(err)
		return false
	}
	return count > 0
}

func (u *Updater) UpdateTeamSeasons(force bool) (int, error) {
	currentSeason, err := u.ESPN.DefaultSeason()
	if err != nil {
		return 0, err
	}

	if !force && u.seasonsExist(currentSeason) {
		u.Logger.Info("Not updating")
		return 0, nil
	}

	sport := u.sportDB()

	teamConfs, err := u.ESPN.TeamConferencesByYear(currentSeason)
	if err != nil {
		return 0, err
	}

	var teamSeasons []database.TeamSeason

	confResult, err := u.ESPN.ConferenceMap()
	if err != nil {
		return 0, err
	}

	if u.ESPN.SportInfo() == espn.CollegeBasketball {
		// Basketball: all D1 teams are top-division (FBS=1). Conference names
		// come from the conference API but there's no FBS/FCS split.
		d1Confs := confResult.Conferences[espn.D1Basketball]

		for team, conf := range teamConfs {
			confName, ok := d1Confs[conf]
			if !ok {
				continue // skip non-D1 teams (e.g. D2/D3/NAIA opponents)
			}
			teamSeasons = append(teamSeasons, database.TeamSeason{
				TeamID: team,
				Conf:   confName,
				Year:   currentSeason,
				Sport:  sport,
				FBS:    1, // all D1 basketball teams treated as top-division
			})
		}
	} else {
		fbs := confResult.Conferences[espn.FBS]
		fbsfcs := maps.Clone(fbs)
		maps.Copy(fbsfcs, confResult.Conferences[espn.FCS])

		for team, conf := range teamConfs {
			confName, ok := fbsfcs[conf]
			if !ok {
				continue
			}
			var isFBS int64
			if _, ok := fbs[conf]; ok {
				isFBS = 1
			}
			teamSeasons = append(teamSeasons, database.TeamSeason{
				TeamID: team,
				Conf:   confName,
				Year:   currentSeason,
				Sport:  sport,
				FBS:    isFBS,
			})
		}
	}

	if err := u.insertSeasonToDB(teamSeasons); err != nil {
		return 0, err
	}

	return len(teamSeasons), nil
}
