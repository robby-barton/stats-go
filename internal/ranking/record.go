package ranking

import (
	"github.com/robby-barton/stats-go/internal/database"
)

func (r *Ranker) record(teamList TeamList) error {
	var games []database.Game
	if err := r.DB.Where("season = ? and start_time <= ?", r.Year, r.startTime).
		Find(&games).Error; err != nil {

		return err
	}

	var allTeams []int64
	if err := r.DB.Model(database.TeamSeason{}).Where("year = ?", r.Year).
		Pluck("team_id", &allTeams).Error; err != nil {

		return err
	}

	allowedTeam := map[int64]bool{}
	teamRecords := make(map[int64]*Record)
	for _, team := range allTeams {
		allowedTeam[team] = true
		teamRecords[team] = &Record{}
	}

	for _, game := range games {
		if allowedTeam[game.HomeId] {
			homeRecord := teamRecords[game.HomeId]
			if game.HomeScore > game.AwayScore {
				homeRecord.Wins++
			} else if game.AwayScore > game.HomeScore {
				homeRecord.Losses++
			} else {
				homeRecord.Ties++
			}
			homeRecord.Record = (1 + float64(homeRecord.Wins) + 0.5*float64(homeRecord.Ties)) /
				(2 + float64(homeRecord.Wins+homeRecord.Losses+homeRecord.Ties))
		}
		if allowedTeam[game.AwayId] {
			awayRecord := teamRecords[game.AwayId]
			if game.HomeScore > game.AwayScore {
				awayRecord.Losses++
			} else if game.AwayScore > game.HomeScore {
				awayRecord.Wins++
			} else {
				awayRecord.Ties++
			}
			awayRecord.Record = (1 + float64(awayRecord.Wins) + 0.5*float64(awayRecord.Ties)) /
				(2 + float64(awayRecord.Wins+awayRecord.Losses+awayRecord.Ties))
		}
	}

	for id, team := range teamList {
		if record, ok := teamRecords[id]; ok {
			team.Record = *record
		}
	}

	return nil
}
