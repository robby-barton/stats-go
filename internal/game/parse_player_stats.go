package game

import (
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/espn"
)

const playerID = "playerId"
const yds = "YDS"
const long = "LONG"

// splitPair parses "a<sep>b" stat values (e.g. "20/30", "5-12") into two
// integers. Malformed or missing values (ESPN occasionally sends "--", "N/A",
// or truncated strings) return zeros instead of panicking on a short split.
func splitPair(value, sep string) (int64, int64) {
	parts := strings.Split(value, sep)
	if len(parts) != 2 {
		return 0, 0
	}
	a, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	b, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	return a, b
}

func createStatMaps(stats espn.PlayerStatistics) []map[string]interface{} {
	var statMaps []map[string]interface{}

	keys := stats.Labels

	if len(stats.Totals) > 0 {
		totals := make(map[string]interface{})
		for i, key := range keys {
			if i >= len(stats.Totals) {
				break // labels/totals length mismatch: skip missing values
			}
			totals[key] = stats.Totals[i]
		}
		totals[playerID] = int64(-1)

		statMaps = append(statMaps, totals)
	}

	for _, athlete := range stats.Athletes {
		playerStats := make(map[string]interface{})
		for i, key := range keys {
			if i >= len(athlete.Stats) {
				break // labels/stats length mismatch: skip missing values
			}
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
) []PassingStats {
	var retStats []PassingStats

	statMaps := createStatMaps(passStats)
	for _, statMap := range statMaps {
		player := PassingStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "C/ATT":
				player.Completions, player.Attempts = splitPair(value.(string), "/")
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
) []RushingStats {
	var retStats []RushingStats

	statMaps := createStatMaps(rushStats)
	for _, statMap := range statMaps {
		player := RushingStats{
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
) []ReceivingStats {
	var retStats []ReceivingStats

	statMaps := createStatMaps(recStats)
	for _, statMap := range statMaps {
		player := ReceivingStats{
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
) []FumbleStats {
	var retStats []FumbleStats

	statMaps := createStatMaps(fumbleStats)
	for _, statMap := range statMaps {
		player := FumbleStats{
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
) []DefensiveStats {
	var retStats []DefensiveStats

	statMaps := createStatMaps(defStats)
	for _, statMap := range statMaps {
		player := DefensiveStats{
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
) []InterceptionStats {
	var retStats []InterceptionStats

	statMaps := createStatMaps(intStats)
	for _, statMap := range statMaps {
		player := InterceptionStats{
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
) []ReturnStats {
	var retStats []ReturnStats

	statMaps := createStatMaps(returnStats)
	for _, statMap := range statMaps {
		player := ReturnStats{
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
) []KickStats {
	var retStats []KickStats

	statMaps := createStatMaps(kickStats)
	for _, statMap := range statMaps {
		player := KickStats{
			TeamID: teamID,
			GameID: gameID,
		}

		for key, value := range statMap {
			switch key {
			case playerID:
				player.PlayerID = value.(int64)
			case "FG":
				player.FGM, player.FGA = splitPair(value.(string), "/")
			case long:
				player.FGLong, _ = strconv.ParseInt(value.(string), 10, 64)
			case "XP":
				player.XPM, player.XPA = splitPair(value.(string), "/")
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
) []PuntStats {
	var retStats []PuntStats

	statMaps := createStatMaps(puntStats)
	for _, statMap := range statMaps {
		player := PuntStats{
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

func (s *ParsedGameInfo) parsePlayerStats(gameInfo *espn.GameInfoESPN, log *zap.SugaredLogger) {
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
				log.Warnf("game %d: unrecognized player stat category %q", gameID, stat.Name)
			}
		}
	}
}
