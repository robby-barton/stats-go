package game

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

const playerID = "playerId"
const yds = "YDS"
const long = "LONG"

func createStatMaps(stats espn.PlayerStatistics) []map[string]interface{} {
	var statMaps []map[string]interface{}

	keys := stats.Labels

	if len(stats.Totals) > 0 {
		totals := make(map[string]interface{})
		for i, key := range keys {
			totals[key] = stats.Totals[i]
		}
		totals[playerID] = int64(-1)

		statMaps = append(statMaps, totals)
	}

	for _, athlete := range stats.Athletes {
		playerStats := make(map[string]interface{})
		for i, key := range keys {
			playerStats[key] = athlete.Stats[i]
		}
		playerStats[playerID] = athlete.Athlete.ID

		statMaps = append(statMaps, playerStats)
	}

	return statMaps
}

func parsePassingStats(
	gameID int64,
	teamID int64,
	passStats espn.PlayerStatistics,
) []database.PassingStats {
	var retStats []database.PassingStats

	statMaps := createStatMaps(passStats)
	for _, statMap := range statMaps {
		player := database.PassingStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "C/ATT":
				compSplit := strings.Split(value.(string), "/")
				player.Completions, _ = strconv.ParseInt(compSplit[0], 10, 64)
				player.Attempts, _ = strconv.ParseInt(compSplit[1], 10, 64)
			case yds:
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
	gameID int64,
	teamID int64,
	rushStats espn.PlayerStatistics,
) []database.RushingStats {
	var retStats []database.RushingStats

	statMaps := createStatMaps(rushStats)
	for _, statMap := range statMaps {
		player := database.RushingStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "CAR":
				player.Carries, _ = strconv.ParseInt(value.(string), 10, 64)
			case yds:
				player.RushYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			case long:
				player.RushLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseReceivingStats(
	gameID int64,
	teamID int64,
	recStats espn.PlayerStatistics,
) []database.ReceivingStats {
	var retStats []database.ReceivingStats

	statMaps := createStatMaps(recStats)
	for _, statMap := range statMaps {
		player := database.ReceivingStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "REC":
				player.Receptions, _ = strconv.ParseInt(value.(string), 10, 64)
			case yds:
				player.RecYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TD":
				player.Touchdowns, _ = strconv.ParseInt(value.(string), 10, 64)
			case long:
				player.RecLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func parseFumbleStats(
	gameID int64,
	teamID int64,
	fumbleStats espn.PlayerStatistics,
) []database.FumbleStats {
	var retStats []database.FumbleStats

	statMaps := createStatMaps(fumbleStats)
	for _, statMap := range statMaps {
		player := database.FumbleStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
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
	gameID int64,
	teamID int64,
	defStats espn.PlayerStatistics,
) []database.DefensiveStats {
	var retStats []database.DefensiveStats

	statMaps := createStatMaps(defStats)
	for _, statMap := range statMaps {
		player := database.DefensiveStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
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
	gameID int64,
	teamID int64,
	intStats espn.PlayerStatistics,
) []database.InterceptionStats {
	var retStats []database.InterceptionStats

	statMaps := createStatMaps(intStats)
	for _, statMap := range statMaps {
		player := database.InterceptionStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "INT":
				player.Interceptions, _ = strconv.ParseInt(value.(string), 10, 64)
			case yds:
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
	gameID int64,
	teamID int64,
	returnStats espn.PlayerStatistics,
	returnType string,
) []database.ReturnStats {
	var retStats []database.ReturnStats

	statMaps := createStatMaps(returnStats)
	for _, statMap := range statMaps {
		player := database.ReturnStats{
			TeamID:   teamID,
			GameID:   gameID,
			PuntKick: returnType,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "NO":
				player.ReturnNo, _ = strconv.ParseInt(value.(string), 10, 64)
			case yds:
				player.RetYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case long:
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
	gameID int64,
	teamID int64,
	kickStats espn.PlayerStatistics,
) []database.KickStats {
	var retStats []database.KickStats

	statMaps := createStatMaps(kickStats)
	for _, statMap := range statMaps {
		player := database.KickStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "FG":
				compSplit := strings.Split(value.(string), "/")
				player.FGM, _ = strconv.ParseInt(compSplit[0], 10, 64)
				player.FGA, _ = strconv.ParseInt(compSplit[1], 10, 64)
			case long:
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
	gameID int64,
	teamID int64,
	puntStats espn.PlayerStatistics,
) []database.PuntStats {
	var retStats []database.PuntStats

	statMaps := createStatMaps(puntStats)
	for _, statMap := range statMaps {
		player := database.PuntStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "NO":
				player.PuntNo, _ = strconv.ParseInt(value.(string), 10, 64)
			case yds:
				player.PuntYards, _ = strconv.ParseInt(value.(string), 10, 64)
			case "TB":
				player.Touchbacks, _ = strconv.ParseInt(value.(string), 10, 64)
			case "In 20":
				player.Inside20, _ = strconv.ParseInt(value.(string), 10, 64)
			case long:
				player.PuntLong, _ = strconv.ParseInt(value.(string), 10, 64)
			}
		}

		retStats = append(retStats, player)
	}

	return retStats
}

func (s *ParsedGameInfo) parsePlayerStats(gameInfo *espn.GameInfoESPN) {
	gameID := gameInfo.GamePackage.Header.ID
	players := gameInfo.GamePackage.Boxscore.Players
	for _, playerStats := range players {
		teamID := playerStats.Team.ID
		for _, stat := range playerStats.Statistics {
			switch stat.Name {
			case "passing":
				s.PassingStats =
					append(s.PassingStats, parsePassingStats(gameID, teamID, stat)...)
			case "rushing":
				s.RushingStats =
					append(s.RushingStats, parseRushingStats(gameID, teamID, stat)...)
			case "receiving":
				s.ReceivingStats =
					append(s.ReceivingStats, parseReceivingStats(gameID, teamID, stat)...)
			case "fumbles":
				s.FumbleStats =
					append(s.FumbleStats, parseFumbleStats(gameID, teamID, stat)...)
			case "defensive":
				s.DefensiveStats =
					append(s.DefensiveStats, parseDefensiveStats(gameID, teamID, stat)...)
			case "interceptions":
				s.InterceptionStats =
					append(s.InterceptionStats, parseInterceptionStats(gameID, teamID, stat)...)
			case "kickReturns":
				s.ReturnStats =
					append(s.ReturnStats, parseReturnStats(gameID, teamID, stat, "kick")...)
			case "puntReturns":
				s.ReturnStats =
					append(s.ReturnStats, parseReturnStats(gameID, teamID, stat, "punt")...)
			case "kicking":
				s.KickStats =
					append(s.KickStats, parseKickStats(gameID, teamID, stat)...)
			case "punting":
				s.PuntStats =
					append(s.PuntStats, parsePuntStats(gameID, teamID, stat)...)
			default:
				fmt.Printf("Not found {%s}\n", stat.Name) //nolint:forbidigo // allow for this case
			}
		}
	}
}
