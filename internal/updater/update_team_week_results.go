package updater

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
)

type yearInfo struct {
	Year       int64
	Weeks      int64
	Postseason int64
}

func (u *Updater) getYearInfo() ([]yearInfo, error) {
	var yearInfo []yearInfo
	if err := u.db.Model(database.Game{}).
		Select(`season as year, max(week) as weeks, max(postseason) as postseason`).
		Where("sport = ? and season >= ?", u.sportDB(), 1936). // first official year of AP poll
		Group("season").
		Order("season").Find(&yearInfo).Error; err != nil {
		return nil, err
	}

	return yearInfo, nil
}

// regularSeasonWeeks returns the distinct regular-season week numbers for a
// given year, in ascending order. This avoids iterating over week gaps (common
// in basketball) that would cause the ranker to fall through to the latest-game
// logic and produce duplicate entries.
func (u *Updater) regularSeasonWeeks(year int64) ([]int64, error) {
	var weeks []int64
	if err := u.db.Model(database.Game{}).
		Where("sport = ? and season = ? and postseason = 0", u.sportDB(), year).
		Distinct("week").
		Order("week").
		Pluck("week", &weeks).Error; err != nil {
		return nil, err
	}
	return weeks, nil
}

func (u *Updater) insertRankingsToDB(rankings []database.TeamWeekResult) error {
	return u.db.Transaction(func(tx *gorm.DB) error {
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

func teamListToTeamWeekResult(teamList ranking.TeamList, fbs bool, sport sport.Sport) []database.TeamWeekResult {
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

	newRanker := func(fcs bool) (*ranking.Ranker, error) {
		in, err := load.Input(load.Options{
			DB:    u.db,
			Sport: sport,
			Year:  year,
			Week:  week,
			Fcs:   fcs,
		})
		if err != nil {
			return nil, err
		}
		return ranking.NewRanker(in)
	}

	if u.espn.SportInfo() == espn.CollegeBasketball {
		// Basketball: single D1 ranking, no FBS/FCS split
		ranker, err := newRanker(false)
		if err != nil {
			return nil, err
		}
		teamList, err := ranker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(teamList, true, sport)...)
	} else {
		fbsRanker, err := newRanker(false)
		if err != nil {
			return nil, err
		}
		fbsRanking, err := fbsRanker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fbsRanking, true, sport)...)

		fcsRanker, err := newRanker(true)
		if err != nil {
			return nil, err
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
		weeks, err := u.regularSeasonWeeks(year.Year)
		if err != nil {
			return err
		}
		for _, week := range weeks {
			u.logger.Infof("%d/%d", year.Year, week)
			weekRankings, err := u.rankingForWeek(year.Year, week)
			if err != nil {
				return err
			}
			teamWeekResults = append(teamWeekResults, weekRankings...)
		}
		// postseason or current week
		if year.Postseason == 1 {
			u.logger.Infof("%d/Final", year.Year)
		} else {
			u.logger.Infof("%d/%d", year.Year, year.Weeks+1)
		}
		weekRankings, err := u.rankingForWeek(year.Year, 0)
		if err != nil {
			return err
		}
		teamWeekResults = append(teamWeekResults, weekRankings...)
	}

	return u.insertRankingsToDB(teamWeekResults)
}
