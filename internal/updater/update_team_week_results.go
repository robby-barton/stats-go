package updater

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/ranking"
)

type yearInfo struct {
	Year       int64
	Weeks      int64
	Postseason int64
}

func (u *Updater) getYearInfo() ([]yearInfo, error) {
	var yearInfo []yearInfo
	if err := u.DB.Model(database.Game{}).
		Select(`season as year, max(week) as weeks, max(postseason) as postseason`).
		Where("sport = ? and season >= ?", u.sportDB(), 1936). // first official year of AP poll
		Group("season").
		Order("season").Find(&yearInfo).Error; err != nil {
		return nil, err
	}

	return yearInfo, nil
}

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

func teamListToTeamWeekResult(teamList ranking.TeamList, fbs bool, sport string) []database.TeamWeekResult {
	var retTWR []database.TeamWeekResult

	for id, result := range teamList {
		retTWR = append(retTWR, database.TeamWeekResult{
			TeamID:     id,
			Name:       result.Name,
			Conf:       result.Conf,
			Year:       result.Year,
			Week:       result.Week,
			Postseason: result.Postseason,
			Sport:      sport,
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
	sport := u.sportDB()
	var teamWeekResults []database.TeamWeekResult

	if u.Sport == espn.CollegeBasketball {
		// Basketball: single D1 ranking, no FBS/FCS split
		ranker := ranking.Ranker{
			DB:    u.DB,
			Year:  year,
			Week:  week,
			Sport: sport,
		}
		teamList, err := ranker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(teamList, true, sport)...)
	} else {
		fbsRanker := ranking.Ranker{
			DB:    u.DB,
			Year:  year,
			Week:  week,
			Sport: sport,
		}
		fbsRanking, err := fbsRanker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fbsRanking, true, sport)...)

		fcsRanker := ranking.Ranker{
			DB:    u.DB,
			Year:  year,
			Week:  week,
			Fcs:   true,
			Sport: sport,
		}
		fcsRanking, err := fcsRanker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fcsRanking, false, sport)...)
	}

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

	yearInfo, err := u.getYearInfo()
	if err != nil {
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
