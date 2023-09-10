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
	err := u.DB.Model(database.TeamSeason{}).Where("year = ?", year).Count(&count).Error
	if err != nil {
		u.Logger.Error(err)
		return false
	}
	return count > 0
}

func (u *Updater) UpdateTeamSeasons(force bool) (int, error) {
	currentSeason, err := espn.DefaultSeason()
	if err != nil {
		return 0, err
	}

	if !force && !u.seasonsExist(currentSeason) {
		u.Logger.Info("Not updating")
		return 0, nil
	}

	conferences, err := espn.ConferenceMap()
	if err != nil {
		return 0, err
	}
	fbs := conferences[espn.FBS].(map[int64]string)
	fbsfcs := maps.Clone(fbs)
	maps.Copy(fbsfcs, conferences[espn.FCS].(map[int64]string))

	teamConfs, err := espn.TeamConferencesByYear(currentSeason)
	if err != nil {
		return 0, err
	}

	teamSeasons := []database.TeamSeason{}
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
			FBS:    isFBS,
		})
	}

	if err := u.insertSeasonToDB(teamSeasons); err != nil {
		return 0, err
	}

	return len(teamSeasons), nil
}
