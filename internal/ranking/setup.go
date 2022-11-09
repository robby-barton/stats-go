package ranking

import (
	"time"

	"github.com/robby-barton/stats-api/internal/database"
)

func (r *Ranker) setup(params CalculateRankingParams) (TeamList, error) {
	if err := r.setGlobals(params); err != nil {
		return nil, err
	}

	var teamList TeamList
	var err error
	if params.Fbs {
		if teamList, err = r.createTeamList(1); err != nil {
			return nil, err
		}
	} else {
		if teamList, err = r.createTeamList(0); err != nil {
			return nil, err
		}
	}

	return teamList, nil
}

func (r *Ranker) setGlobals(globals CalculateRankingParams) error {
	if globals.Year > 0 {
		year = globals.Year
	} else {
		currYear, currMonth, _ := time.Now().Date()
		if currMonth >= 8 {
			year = int64(currYear)
		} else {
			year = int64(currYear - 1)
		}
	}

	var game database.Game
	if globals.Week > 0 {
		if err := r.DB.
			Where("season = ? and week = ? and postseason = 0", year, globals.Week).
			Order("start_time asc").
			Limit(1).
			Find(&game).Error; err != nil {

			return err
		}
		if game != (database.Game{}) {
			y, m, d := game.StartTime.
				AddDate(0, 0, -int(game.StartTime.Weekday()-time.Tuesday)).Date()
			startTime = time.Date(y, m, d, 0, 0, 0, 0, time.Local)
			week = globals.Week
		}
	}

	if game == (database.Game{}) {
		if err := r.DB.
			Where("season <= ?", year).
			Order("start_time desc").
			Limit(1).
			Find(&game).Error; err != nil {

			return err
		}
	}

	if week == 0 {
		week = game.Week + 1
	}

	if startTime == (time.Time{}) {
		startTime = game.StartTime
	}

	if game.Postseason > 0 {
		postseason = true
	}

	return nil
}

func (r *Ranker) createTeamList(findFbs int64) (TeamList, error) {
	teams := []struct {
		TeamId int64
		Name   string
		Conf   string
	}{}

	if err := r.DB.Model(&database.TeamSeason{}).
		Select("team_names.team_id, team_names.name, team_seasons.conf").
		Joins("left join team_names on team_seasons.team_id = team_names.team_id").
		Where("team_seasons.fbs = ? and team_seasons.year = ?", findFbs, year).
		Scan(&teams).Error; err != nil {

		return nil, err
	}

	teamList := TeamList{}
	for _, team := range teams {
		teamList[team.TeamId] = &Team{
			Name: team.Name,
			Conf: team.Conf,
		}
	}

	return teamList, nil
}

func (r *Ranker) addGames(teamList TeamList) error {
	var games []database.Game
	if err := r.DB.
		Where(
			"season = ? and start_time <= ?",
			year,
			startTime,
		).
		Order("start_time asc").Find(&games).Error; err != nil {

		return err
	}

	var gameIds []int64
	for _, game := range games {
		gameIds = append(gameIds, game.GameId)
	}
	var tgsWinner []database.TeamGameStats
	if err := r.DB.Select("distinct on (game_id) game_id, team_id").
		Where(
			"game_id in (?)",
			gameIds,
		).
		Order("game_id").
		Order("score desc").Find(&tgsWinner).Error; err != nil {

		return err
	}
	winners := make(map[int64]int64)
	for _, winner := range tgsWinner {
		winners[winner.GameId] = winner.TeamId
	}

	for _, game := range games {
		if home, ok := teamList[game.HomeId]; ok {
			scheduleGame := ScheduleGame{
				GameId:   game.GameId,
				Opponent: game.AwayId,
			}
			if winners[game.GameId] == game.HomeId {
				home.Record.Wins++
				scheduleGame.Won = true
			} else {
				home.Record.Losses++
				scheduleGame.Won = false
			}
			home.Schedule = append(home.Schedule, scheduleGame)
			home.Record.Record =
				float64(home.Record.Wins) / float64(home.Record.Wins+home.Record.Losses)
		}

		if away, ok := teamList[game.AwayId]; ok {
			scheduleGame := ScheduleGame{
				GameId:   game.GameId,
				Opponent: game.HomeId,
			}
			if winners[game.GameId] == game.AwayId {
				away.Record.Wins++
				scheduleGame.Won = true
			} else {
				away.Record.Losses++
				scheduleGame.Won = false
			}
			away.Schedule = append(away.Schedule, scheduleGame)
			away.Record.Record =
				float64(away.Record.Wins) / float64(away.Record.Wins+away.Record.Losses)
		}
	}

	return nil
}
