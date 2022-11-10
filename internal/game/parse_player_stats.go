package game

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func createStatMaps(stats espn.PlayerStatistics) []map[string]interface{} {
	var statMaps []map[string]interface{}

	keys := stats.Labels

	if len(stats.Totals) > 0 {
		totals := make(map[string]interface{})
		for i, key := range keys {
			totals[key] = stats.Totals[i]
		}
		totals["playerId"] = int64(-1)

		statMaps = append(statMaps, totals)
	}

	for _, athlete := range stats.Athletes {
		playerStats := make(map[string]interface{})
		for i, key := range keys {
			playerStats[key] = athlete.Stats[i]
		}
		playerStats["playerId"] = athlete.Athlete.Id

		statMaps = append(statMaps, playerStats)
	}

	return statMaps
}

func parsePassingStats(
	gameId int64,
	teamId int64,
	passStats espn.PlayerStatistics,
) []database.PassingStats {
	var retStats []database.PassingStats

	statMaps := createStatMaps(passStats)
	for _, statMap := range statMaps {
		player := database.PassingStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "C/ATT":
				compSplit := strings.Split(value.(string), "/")
				player.Completions, _ = strconv.ParseInt(compSplit[0], 10, 64)
				player.Attempts, _ = strconv.ParseInt(compSplit[1], 10, 64)
			case "YDS":
				player.Yards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			case "INT":
				player.Interceptions, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseRushingStats(
	gameId int64,
	teamId int64,
	rushStats espn.PlayerStatistics,
) []database.RushingStats {
	var retStats []database.RushingStats

	statMaps := createStatMaps(rushStats)
	for _, statMap := range statMaps {
		player := database.RushingStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "CAR":
				player.Carries, _ = strconv.ParseInt(value.(string), 10, 64)
			case "YDS":
				player.RushYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			case "LONG":
				player.RushLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseReceivingStats(
	gameId int64,
	teamId int64,
	recStats espn.PlayerStatistics,
) []database.ReceivingStats {
	var retStats []database.ReceivingStats

	statMaps := createStatMaps(recStats)
	for _, statMap := range statMaps {
		player := database.ReceivingStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "REC":
				player.Receptions, _ = strconv.ParseInt(value.(string), 10, 64)
			case "YDS":
				player.RecYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			case "LONG":
				player.RecLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseFumbleStats(
	gameId int64,
	teamId int64,
	fumbleStats espn.PlayerStatistics,
) []database.FumbleStats {
	var retStats []database.FumbleStats

	statMaps := createStatMaps(fumbleStats)
	for _, statMap := range statMaps {
		player := database.FumbleStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "FUM":
				player.Fumbles, _ = strconv.ParseInt(value.(string), 10, 64)
			case "LOST":
				player.FumblesLost, _ = strconv.ParseInt(value.(string), 10, 64)
			case "REC":
				player.FumblesRec, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseDefensiveStats(
	gameId int64,
	teamId int64,
	defStats espn.PlayerStatistics,
) []database.DefensiveStats {
	var retStats []database.DefensiveStats

	statMaps := createStatMaps(defStats)
	for _, statMap := range statMaps {
		player := database.DefensiveStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "TOT":
				player.TotalTackles, _ = strconv.ParseFloat(value.(string), 64)
			case "SOLO":
				player.SoloTackles, _ = strconv.ParseInt(value.(string), 10, 64)
			case "SACKS":
				player.Sacks, _ = strconv.ParseFloat(value.(string), 64)
			case "TFL":
				player.TacklesForLoss, _ = strconv.ParseFloat(value.(string), 64)
			case "PD":
				player.PassesDef, _ = strconv.ParseInt(value.(string), 10, 64)
			case "QB HUR":
				player.QBHurries, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseInterceptionStats(
	gameId int64,
	teamId int64,
	intStats espn.PlayerStatistics,
) []database.InterceptionStats {
	var retStats []database.InterceptionStats

	statMaps := createStatMaps(intStats)
	for _, statMap := range statMaps {
		player := database.InterceptionStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "INT":
				player.Interceptions, _ = strconv.ParseInt(value.(string), 10, 64)
			case "YDS":
				player.IntYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseReturnStats(
	gameId int64,
	teamId int64,
	returnStats espn.PlayerStatistics,
	returnType string,
) []database.ReturnStats {
	var retStats []database.ReturnStats

	statMaps := createStatMaps(returnStats)
	for _, statMap := range statMaps {
		player := database.ReturnStats{
			TeamId:   teamId,
			GameId:   gameId,
			PuntKick: returnType,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "NO":
				player.ReturnNo, _ = strconv.ParseInt(value.(string), 10, 64)
			case "YDS":
				player.RetYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "LONG":
				player.RetLong, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseKickStats(
	gameId int64,
	teamId int64,
	kickStats espn.PlayerStatistics,
) []database.KickStats {
	var retStats []database.KickStats

	statMaps := createStatMaps(kickStats)
	for _, statMap := range statMaps {
		player := database.KickStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "FG":
				compSplit := strings.Split(value.(string), "/")
				player.FGM, _ = strconv.ParseInt(compSplit[0], 10, 64)
				player.FGA, _ = strconv.ParseInt(compSplit[1], 10, 64)
			case "LONG":
				player.FGLong, _ = strconv.ParseInt(value.(string), 10, 64)
			case "XP":
				compSplit := strings.Split(value.(string), "/")
				player.XPM, _ = strconv.ParseInt(compSplit[0], 10, 64)
				player.XPA, _ = strconv.ParseInt(compSplit[1], 10, 64)
			case "PTS":
				player.Points, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parsePuntStats(
	gameId int64,
	teamId int64,
	puntStats espn.PlayerStatistics,
) []database.PuntStats {
	var retStats []database.PuntStats

	statMaps := createStatMaps(puntStats)
	for _, statMap := range statMaps {
		player := database.PuntStats{
			TeamId: teamId,
			GameId: gameId,
		}

		for key, value := range statMap {
			switch key {
			case "playerId":
				player.PlayerId = value.(int64)
			case "NO":
				player.PuntNo, _ = strconv.ParseInt(value.(string), 10, 64)
			case "YDS":
				player.PuntYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TB":
				player.Touchbacks, _ = strconv.ParseInt(value.(string), 10, 64)
			case "In 20":
				player.Inside20, _ = strconv.ParseInt(value.(string), 10, 64)
			case "LONG":
				player.PuntLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func (t *ParsedGameInfo) parsePlayerStats(gameInfo *espn.GameInfoESPN) {
	gameId := gameInfo.GamePackage.Header.Id
	players := gameInfo.GamePackage.Boxscore.Players
	for _, playerStats := range players {
		teamId := playerStats.Team.Id
		for _, stat := range playerStats.Statistics {
			switch stat.Name {
			case "passing":
				t.PassingStats =
					append(t.PassingStats, parsePassingStats(gameId, teamId, stat)...)
			case "rushing":
				t.RushingStats =
					append(t.RushingStats, parseRushingStats(gameId, teamId, stat)...)
			case "receiving":
				t.ReceivingStats =
					append(t.ReceivingStats, parseReceivingStats(gameId, teamId, stat)...)
			case "fumbles":
				t.FumbleStats =
					append(t.FumbleStats, parseFumbleStats(gameId, teamId, stat)...)
			case "defensive":
				t.DefensiveStats =
					append(t.DefensiveStats, parseDefensiveStats(gameId, teamId, stat)...)
			case "interceptions":
				t.InterceptionStats =
					append(t.InterceptionStats, parseInterceptionStats(gameId, teamId, stat)...)
			case "kickReturns":
				t.ReturnStats =
					append(t.ReturnStats, parseReturnStats(gameId, teamId, stat, "kick")...)
			case "puntReturns":
				t.ReturnStats =
					append(t.ReturnStats, parseReturnStats(gameId, teamId, stat, "punt")...)
			case "kicking":
				t.KickStats =
					append(t.KickStats, parseKickStats(gameId, teamId, stat)...)
			case "punting":
				t.PuntStats =
					append(t.PuntStats, parsePuntStats(gameId, teamId, stat)...)
			default:
				fmt.Printf("Not found {%s}\n", stat.Name)
			}
		}
	}
}
