package game

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/robby-barton/stats-go/internal/espn"
)

// siteSummaryGameJSON is a trimmed-but-real site.api.espn.com summary
// response (event 401864494: USC 42, San José State 26, 2026 week 1). It is
// the flat shape (header/boxscore at the top level) that
// espn.GameInfoESPN.UnmarshalJSON folds into the cdn-style GamePackage.
const siteSummaryGameJSON = `{
  "header": {
    "id": "401864494",
    "season": {"year": 2026, "type": 2},
    "week": 1,
    "competitions": [{
      "id": "401864494",
      "date": "2026-08-29T19:00Z",
      "conferenceCompetition": false,
      "neutralSite": false,
      "competitors": [
        {"id": "30", "homeAway": "home", "score": "42", "team": {"id": "30", "displayName": "USC Trojans"}},
        {"id": "23", "homeAway": "away", "score": "26", "team": {"id": "23", "displayName": "San José State Spartans"}}
      ]
    }]
  },
  "boxscore": {
    "teams": [
      {"homeAway": "away", "team": {"id": "23"}, "statistics": [
        {"name": "firstDowns", "displayValue": "19", "label": "1st Downs"},
        {"name": "netPassingYards", "displayValue": "234", "label": "Net Passing Yards"},
        {"name": "possessionTime", "displayValue": "22:45", "label": "Possession"}
      ]},
      {"homeAway": "home", "team": {"id": "30"}, "statistics": [
        {"name": "firstDowns", "displayValue": "29", "label": "1st Downs"},
        {"name": "netPassingYards", "displayValue": "341", "label": "Net Passing Yards"},
        {"name": "possessionTime", "displayValue": "37:15", "label": "Possession"}
      ]}
    ],
    "players": [
      {"team": {"id": "23"}, "statistics": [
        {"name": "passing", "labels": ["C/ATT", "YDS", "AVG", "TD", "INT"],
         "totals": ["21/32", "234", "7.3", "2", "0"],
         "athletes": [
           {"athlete": {"id": "5295238", "firstName": "Luke", "lastName": "Weaver"},
            "stats": ["21/32", "234", "7.3", "2", "0"]}]},
        {"name": "rushing", "labels": ["CAR", "YDS", "AVG", "TD", "LONG"],
         "totals": ["24", "102", "4.3", "1", "24"],
         "athletes": [
           {"athlete": {"id": "5295238", "firstName": "Luke", "lastName": "Weaver"},
            "stats": ["8", "42", "5.3", "1", "12"]}]}
      ]},
      {"team": {"id": "30"}, "statistics": [
        {"name": "passing", "labels": ["C/ATT", "YDS", "AVG", "TD", "INT"],
         "totals": ["30/36", "341", "9.5", "2", "1"],
         "athletes": [
           {"athlete": {"id": "4685454", "firstName": "Jayden", "lastName": "Maiava"},
            "stats": ["25/29", "286", "9.9", "2", "0"]}]}
      ]}
    ]
  }
}`

// TestParseSiteAPISummary runs a real site.api summary response through the
// same parse path the updater uses (parseGameInfo → parseTeamInfo →
// parsePlayerStats) to prove every consumed field is populated from the flat
// summary shape, not just the cdn gamepackageJSON shape.
func TestParseSiteAPISummary(t *testing.T) {
	var gameInfo espn.GameInfoESPN
	if err := json.Unmarshal([]byte(siteSummaryGameJSON), &gameInfo); err != nil {
		t.Fatalf("unmarshal site.api summary: %v", err)
	}
	// Cardinality is validated inside parseGameInfo; the parser must handle
	// the summary shape exactly like the cdn shape.

	var s ParsedGameInfo
	if err := s.parseGameInfo(&gameInfo); err != nil {
		t.Fatalf("parseGameInfo: %v", err)
	}
	log := zap.NewNop().Sugar()
	s.parseTeamInfo(&gameInfo, log)
	s.parsePlayerStats(&gameInfo, log)

	// Header → Info
	info := s.Info
	if info.GameID != 401864494 {
		t.Errorf("GameID = %d, want 401864494", info.GameID)
	}
	wantStart := time.Date(2026, time.August, 29, 19, 0, 0, 0, time.UTC)
	if !info.StartTime.Equal(wantStart) {
		t.Errorf("StartTime = %v, want %v", info.StartTime, wantStart)
	}
	if info.Week != 1 || info.Season != 2026 || info.Postseason != 0 {
		t.Errorf("Week/Season/Postseason = %d/%d/%d, want 1/2026/0", info.Week, info.Season, info.Postseason)
	}
	if info.ConfGame || info.Neutral {
		t.Error("ConfGame/Neutral = true, want false")
	}
	if info.HomeID != 30 || info.HomeScore != 42 || info.AwayID != 23 || info.AwayScore != 26 {
		t.Errorf("home/away = %d(%d)/%d(%d), want 30(42)/23(26)",
			info.HomeID, info.HomeScore, info.AwayID, info.AwayScore)
	}

	// Boxscore teams → TeamGameStats, ordered home/away from the header
	// (boxscore response order is irrelevant to the parser)
	if len(s.TeamStats) != 2 {
		t.Fatalf("len(TeamStats) = %d, want 2", len(s.TeamStats))
	}
	usc, sjsu := s.TeamStats[0], s.TeamStats[1]
	if sjsu.TeamID != 23 || sjsu.Score != 26 {
		t.Errorf("TeamStats[1] = %+v, want team 23 score 26", sjsu)
	}
	if sjsu.FirstDowns != 19 || sjsu.PassYards != 234 || sjsu.Possession != 1365 {
		t.Errorf("SJSU team stats = firstDowns %d passYards %d possession %d, want 19/234/1365",
			sjsu.FirstDowns, sjsu.PassYards, sjsu.Possession)
	}
	if usc.TeamID != 30 || usc.Score != 42 || usc.FirstDowns != 29 || usc.PassYards != 341 || usc.Possession != 2235 {
		t.Errorf("USC team stats = %+v", usc)
	}

	// Boxscore players → PassingStats (totals row has playerId -1)
	if len(s.PassingStats) != 4 {
		t.Fatalf("len(PassingStats) = %d, want 4 (one totals + one athlete row per team)", len(s.PassingStats))
	}
	totals23 := s.PassingStats[0]
	if totals23.TeamID != 23 || totals23.PlayerID != -1 || totals23.Yards != 234 ||
		totals23.Completions != 21 || totals23.Attempts != 32 || totals23.Touchdowns != 2 {
		t.Errorf("SJSU passing totals = %+v", totals23)
	}
	weaver := s.PassingStats[1]
	if weaver.PlayerID != 5295238 || weaver.Yards != 234 || weaver.Interceptions != 0 {
		t.Errorf("Weaver passing = %+v", weaver)
	}
	maiava := s.PassingStats[3]
	if maiava.TeamID != 30 || maiava.PlayerID != 4685454 || maiava.Yards != 286 {
		t.Errorf("Maiava passing = %+v", maiava)
	}

	// Rushing stats parse with the same label mapping as the cdn shape.
	if len(s.RushingStats) != 2 {
		t.Fatalf("len(RushingStats) = %d, want 2", len(s.RushingStats))
	}
	if s.RushingStats[0].TeamID != 23 || s.RushingStats[0].RushYards != 102 {
		t.Errorf("SJSU rushing totals = %+v", s.RushingStats[0])
	}
	if s.RushingStats[1].PlayerID != 5295238 || s.RushingStats[1].Carries != 8 || s.RushingStats[1].RushLong != 12 {
		t.Errorf("Weaver rushing = %+v", s.RushingStats[1])
	}
}
