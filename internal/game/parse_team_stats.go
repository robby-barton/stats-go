package game

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

func parseTeamStats(stats []espn.TeamStatistics, tgs *database.TeamGameStats) {

	// ESPN occasionally throws in extra dashes into stats
	var re = regexp.MustCompile(`\-+`)

	for _, stat := range stats {
		statValue := stat.DisplayValue
		switch statName := stat.Name; statName {
		case "firstDowns":
			tgs.FirstDowns, _ = strconv.ParseInt(statValue, 10, 64)
		case "thirdDownEff":
			cleaned := re.ReplaceAllString(statValue, "-")
			downsSplit := strings.Split(cleaned, "-")
			tgs.ThirdDownsConv, _ = strconv.ParseInt(downsSplit[0], 10, 64)
			tgs.ThirdDowns, _ = strconv.ParseInt(downsSplit[1], 10, 64)
		case "fourthDownEff":
			cleaned := re.ReplaceAllString(statValue, "-")
			downsSplit := strings.Split(cleaned, "-")
			tgs.FourthDownsConv, _ = strconv.ParseInt(downsSplit[0], 10, 64)
			tgs.FourthDowns, _ = strconv.ParseInt(downsSplit[1], 10, 64)
		case "netPassingYards":
			tgs.PassYards, _ = strconv.ParseInt(statValue, 10, 64)
		case "completionAttempts":
			cleaned := re.ReplaceAllString(statValue, "-")
			downsSplit := strings.Split(cleaned, "-")
			tgs.Completions, _ = strconv.ParseInt(downsSplit[0], 10, 64)
			tgs.CompletionAttempts, _ = strconv.ParseInt(downsSplit[1], 10, 64)
		case "rushingYards":
			tgs.RushYards, _ = strconv.ParseInt(statValue, 10, 64)
		case "rushingAttempts":
			tgs.RushAttempts, _ = strconv.ParseInt(statValue, 10, 64)
		case "totalPenaltiesYards":
			cleaned := re.ReplaceAllString(statValue, "-")
			downsSplit := strings.Split(cleaned, "-")
			tgs.Penalties, _ = strconv.ParseInt(downsSplit[0], 10, 64)
			tgs.PenaltyYards, _ = strconv.ParseInt(downsSplit[1], 10, 64)
		case "fumblesLost":
			tgs.Fumbles, _ = strconv.ParseInt(statValue, 10, 64)
		case "interceptions":
			tgs.Interceptions, _ = strconv.ParseInt(statValue, 10, 64)
		case "possessionTime":
			splitTime := strings.Split(statValue, ":")
			seconds, _ := time.ParseDuration(fmt.Sprintf("%sm%ss", splitTime[0], splitTime[1]))
			tgs.Possession = int64(seconds.Seconds())

		// These are stats from the API that can be derived
		case "totalYards":
		case "yardsPerPass":
		case "yardsPerRushAttempt":
		case "turnovers":

		default:
			fmt.Printf("Not found {%s}\n", statName)
		}
	}
}

func (s *ParsedGameInfo) parseTeamInfo(gameInfo *espn.GameInfoESPN) {
	homeTeam := database.TeamGameStats{
		GameId: gameInfo.GamePackage.Header.Id,
	}
	awayTeam := database.TeamGameStats{
		GameId: gameInfo.GamePackage.Header.Id,
	}

	for _, team := range gameInfo.GamePackage.Header.Competitions[0].Competitors {
		if team.HomeAway == "home" {
			homeTeam.TeamId = team.Id
			homeTeam.Score = team.Score
		} else if team.HomeAway == "away" {
			awayTeam.TeamId = team.Id
			awayTeam.Score = team.Score
		}
	}

	for _, teamStats := range gameInfo.GamePackage.Boxscore.Teams {
		if len(teamStats.Statistics) == 0 {
			continue
		}

		if teamStats.Team.Id == homeTeam.TeamId {
			parseTeamStats(teamStats.Statistics, &homeTeam)
		} else if teamStats.Team.Id == awayTeam.TeamId {
			parseTeamStats(teamStats.Statistics, &awayTeam)
		} else {
			continue
		}
	}

	s.TeamStats = append(s.TeamStats, homeTeam)
	s.TeamStats = append(s.TeamStats, awayTeam)
}
