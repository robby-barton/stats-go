package updater

import (
	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/ranking"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (u *Updater) insertRankingsToDB(rankings []database.TeamWeekResult) error {
	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			CreateInBatches(rankings, 1000).Error; err != nil {
			return err
		}

		return nil
	})
}

func teamListToTeamWeekResult(teamList ranking.TeamList, fbs bool) []database.TeamWeekResult {
	var retTWR []database.TeamWeekResult

	for id, result := range teamList {
		retTWR = append(retTWR, database.TeamWeekResult{
			TeamID:     id,
			Name:       result.Name,
			Conf:       result.Conf,
			Year:       result.Year,
			Week:       result.Week,
			Postseason: result.Postseason,
			FinalRank:  result.FinalRank,
			FinalRaw:   result.FinalRaw,
			Wins:       result.Record.Wins,
			Losses:     result.Record.Losses,
			Ties:       result.Record.Ties,
			SRSRank:    result.SRSRank,
			SOSRank:    result.SOSRank,
			Fbs:        fbs,
		})
	}

	return retTWR
}

func (u *Updater) rankingForWeek(year int64, week int64) ([]database.TeamWeekResult, error) {
	var teamWeekResults []database.TeamWeekResult

	fbsRanker := ranking.Ranker{
		DB:   u.DB,
		Year: year,
		Week: week,
	}
	fbsRanking, err := fbsRanker.CalculateRanking()
	if err != nil {
		return nil, err
	}
	teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fbsRanking, true)...)

	fcsRanker := ranking.Ranker{
		DB:   u.DB,
		Year: year,
		Week: week,
		Fcs:  true,
	}
	fcsRanking, err := fcsRanker.CalculateRanking()
	if err != nil {
		return nil, err
	}
	teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fcsRanking, false)...)

	return teamWeekResults, nil
}

func (u *Updater) UpdateRecentRankings() error {
	weekRankings, err := u.rankingForWeek(0, 0)
	if err != nil {
		return err
	}

	return u.insertRankingsToDB(weekRankings)
}

func (u *Updater) UpdateAllRankings() error {
	var teamWeekResults []database.TeamWeekResult

	var yearInfo []struct {
		Year       int64
		Weeks      int64
		Postseason int64
	}
	if err := u.DB.Model(database.Game{}).
		Select(`season as year, max(week) as weeks, max(postseason) as postseason`).
		Where("season >= ?", 1936). // first official year of AP poll
		Group("season").
		Order("season").Find(&yearInfo).Error; err != nil {
		return err
	}

	for _, year := range yearInfo {
		for week := int64(1); week <= year.Weeks; week++ {
			u.Logger.Infof("%d/%d", year.Year, week)
			weekRankings, err := u.rankingForWeek(year.Year, week)
			if err != nil {
				return err
			}
			teamWeekResults = append(teamWeekResults, weekRankings...)
		}
		// postseason or current week
		if year.Postseason == 1 {
			u.Logger.Infof("%d/Final", year.Year)
		} else {
			u.Logger.Infof("%d/%d", year.Year, year.Weeks+1)
		}
		weekRankings, err := u.rankingForWeek(year.Year, 0)
		if err != nil {
			return err
		}
		teamWeekResults = append(teamWeekResults, weekRankings...)
	}

	return u.insertRankingsToDB(teamWeekResults)
}
