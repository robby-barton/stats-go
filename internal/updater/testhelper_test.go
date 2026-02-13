//go:build integration

package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
)

// ---------------------------------------------------------------------------
// Test database
// ---------------------------------------------------------------------------

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&database.Game{},
		&database.TeamSeason{},
		&database.TeamName{},
		&database.TeamWeekResult{},
		&database.TeamGameStats{},
		&database.PassingStats{},
		&database.RushingStats{},
		&database.ReceivingStats{},
		&database.FumbleStats{},
		&database.DefensiveStats{},
		&database.InterceptionStats{},
		&database.ReturnStats{},
		&database.KickStats{},
		&database.PuntStats{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// ---------------------------------------------------------------------------
// Capturing writer
// ---------------------------------------------------------------------------

type capturingWriter struct {
	mu         sync.Mutex
	data       map[string]any
	purgeCount int
}

func newCapturingWriter() *capturingWriter {
	return &capturingWriter{data: map[string]any{}}
}

func (w *capturingWriter) WriteData(_ context.Context, fileName string, data any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data[fileName] = data
	return nil
}

func (w *capturingWriter) PurgeCache(_ context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.purgeCount++
	return nil
}

func (w *capturingWriter) hasFile(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.data[name]
	return ok
}

func (w *capturingWriter) fileCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.data)
}

// ---------------------------------------------------------------------------
// Fixture data — 4 teams, 2 conferences, 6 games across 2 weeks
// ---------------------------------------------------------------------------

// Team IDs:
//
//	1 = Alpha (SEC, FBS)
//	2 = Beta  (SEC, FBS)
//	3 = Gamma (Big Ten, FBS)
//	4 = Delta (Big Ten, FBS)
//
// Week 1 games (2023-09-02, Saturday = dow 6):
//
//	Game 401001: Alpha 28 – Beta 14  (conf game)
//	Game 401002: Gamma 21 – Delta 10 (conf game)
//	Game 401003: in-progress, should be filtered
//
// Week 2 games (2023-09-09, Saturday = dow 6):
//
//	Game 401004: Alpha 35 – Gamma 17
//	Game 401005: Beta 24 – Delta 21
//	Game 401006: in-progress, should be filtered

const (
	fixtureGameID1 int64 = 401001
	fixtureGameID2 int64 = 401002
	fixtureGameID3 int64 = 401003 // in-progress
	fixtureGameID4 int64 = 401004
	fixtureGameID5 int64 = 401005
	fixtureGameID6 int64 = 401006 // in-progress
)

func fixtureScheduleResponse() espn.GameScheduleESPN {
	return espn.GameScheduleESPN{
		Content: espn.Content{
			Schedule: map[string]espn.Day{
				"2023-09-02": {
					Games: []espn.Game{
						newFinalGame(fixtureGameID1, 1, 100, 28, 2, 100, 14),
						newFinalGame(fixtureGameID2, 3, 200, 21, 4, 200, 10),
						newInProgressGame(fixtureGameID3, 5, 100, 7, 6, 200, 3),
					},
				},
				"2023-09-09": {
					Games: []espn.Game{
						newFinalGame(fixtureGameID4, 1, 100, 35, 3, 200, 17),
						newFinalGame(fixtureGameID5, 2, 100, 24, 4, 200, 21),
						newInProgressGame(fixtureGameID6, 5, 100, 14, 6, 200, 10),
					},
				},
			},
			Parameters: espn.Parameters{Week: 1, Year: 2023, SeasonType: 2, Group: 80},
			Defaults:   espn.Parameters{Week: 1, Year: 2023, SeasonType: 2, Group: 80},
			Calendar: []espn.Calendar{
				{
					StartDate:  "2023-08-26T07:00Z",
					EndDate:    "2023-12-03T07:59Z",
					SeasonType: 2,
					Weeks: []espn.Week{
						{Num: 1, StartDate: "2023-09-04T07:00Z", EndDate: "2023-09-11T06:59Z"},
						{Num: 2, StartDate: "2023-09-11T07:00Z", EndDate: "2023-09-18T06:59Z"},
					},
				},
				{
					StartDate:  "2023-12-16T08:00Z",
					EndDate:    "2024-01-09T07:59Z",
					SeasonType: 3,
					Weeks: []espn.Week{
						{Num: 1, StartDate: "2023-12-16T08:00Z", EndDate: "2024-01-09T07:59Z"},
					},
				},
			},
			ConferenceAPI: espn.ConferenceAPI{
				Conferences: []espn.Conference{
					{GroupID: 100, Name: "Southeastern Conference", ShortName: "SEC", ParentGroupID: 80},
					{GroupID: 200, Name: "Big Ten Conference", ShortName: "Big Ten", ParentGroupID: 80},
					{GroupID: 300, Name: "Missouri Valley", ShortName: "MVFC", ParentGroupID: 81},
				},
			},
		},
	}
}

func newFinalGame(id int64, homeID, homeConf, homeScore, awayID, awayConf, awayScore int64) espn.Game {
	return espn.Game{
		ID: id,
		Status: espn.Status{StatusType: espn.StatusType{
			Name: "STATUS_FINAL", Completed: true,
		}},
		Competitions: []espn.Competition{{
			Competitors: []espn.Competitor{
				{ID: homeID, Team: espn.ScheduleTeam{ID: homeID, ConferenceID: homeConf}, Score: homeScore, HomeAway: "home"},
				{ID: awayID, Team: espn.ScheduleTeam{ID: awayID, ConferenceID: awayConf}, Score: awayScore, HomeAway: "away"},
			},
		}},
	}
}

func newInProgressGame(id int64, homeID, homeConf, homeScore, awayID, awayConf, awayScore int64) espn.Game {
	return espn.Game{
		ID: id,
		Status: espn.Status{StatusType: espn.StatusType{
			Name: "STATUS_IN_PROGRESS", Completed: false,
		}},
		Competitions: []espn.Competition{{
			Competitors: []espn.Competitor{
				{ID: homeID, Team: espn.ScheduleTeam{ID: homeID, ConferenceID: homeConf}, Score: homeScore, HomeAway: "home"},
				{ID: awayID, Team: espn.ScheduleTeam{ID: awayID, ConferenceID: awayConf}, Score: awayScore, HomeAway: "away"},
			},
		}},
	}
}

func fixtureGameInfoResponse(gameID int64) espn.GameInfoESPN {
	games := map[int64]espn.GameInfoESPN{
		fixtureGameID1: newGameInfo(fixtureGameID1, 1, 28, 2, 14, 2023, 1, true),
		fixtureGameID2: newGameInfo(fixtureGameID2, 3, 21, 4, 10, 2023, 1, true),
		fixtureGameID4: newGameInfo(fixtureGameID4, 1, 35, 3, 17, 2023, 2, false),
		fixtureGameID5: newGameInfo(fixtureGameID5, 2, 24, 4, 21, 2023, 2, false),
	}
	if g, ok := games[gameID]; ok {
		return g
	}
	// Fallback for unknown game IDs
	return newGameInfo(gameID, 1, 0, 2, 0, 2023, 1, false)
}

func newGameInfo(
	gameID, homeID, homeScore, awayID, awayScore, year, week int64,
	confGame bool,
) espn.GameInfoESPN {
	dateStr := "2023-09-02T23:00Z"
	if week == 2 {
		dateStr = "2023-09-09T23:00Z"
	}
	return espn.GameInfoESPN{
		GamePackage: espn.GamePackage{
			Header: espn.Header{
				ID: gameID,
				Competitions: []espn.Competitions{{
					ID:       gameID,
					Date:     dateStr,
					ConfGame: confGame,
					Neutral:  false,
					Competitors: []espn.Competitors{
						{HomeAway: "home", ID: homeID, Score: homeScore},
						{HomeAway: "away", ID: awayID, Score: awayScore},
					},
					Status: espn.Status{StatusType: espn.StatusType{
						Name: "STATUS_FINAL", Completed: true,
					}},
				}},
				Season: espn.Season{Year: year, Type: 2},
				Week:   week,
			},
			Boxscore: espn.Boxscore{
				Teams: []espn.Teams{
					{
						Team: espn.Team{ID: homeID},
						Statistics: []espn.TeamStatistics{
							{Name: "firstDowns", DisplayValue: "22"},
							{Name: "totalYards", DisplayValue: "450"},
							{Name: "netPassingYards", DisplayValue: "250"},
							{Name: "completionAttempts", DisplayValue: "20/30"},
							{Name: "rushingYards", DisplayValue: "200"},
							{Name: "rushingAttempts", DisplayValue: "35"},
							{Name: "totalPenaltiesYards", DisplayValue: "5-40"},
							{Name: "fumblesLost", DisplayValue: "1"},
							{Name: "interceptions", DisplayValue: "0"},
							{Name: "possessionTime", DisplayValue: "32:15"},
							{Name: "thirdDownEff", DisplayValue: "6-12"},
							{Name: "fourthDownEff", DisplayValue: "1-2"},
						},
					},
					{
						Team: espn.Team{ID: awayID},
						Statistics: []espn.TeamStatistics{
							{Name: "firstDowns", DisplayValue: "15"},
							{Name: "totalYards", DisplayValue: "300"},
							{Name: "netPassingYards", DisplayValue: "180"},
							{Name: "completionAttempts", DisplayValue: "15/25"},
							{Name: "rushingYards", DisplayValue: "120"},
							{Name: "rushingAttempts", DisplayValue: "25"},
							{Name: "totalPenaltiesYards", DisplayValue: "7-55"},
							{Name: "fumblesLost", DisplayValue: "2"},
							{Name: "interceptions", DisplayValue: "1"},
							{Name: "possessionTime", DisplayValue: "27:45"},
							{Name: "thirdDownEff", DisplayValue: "4-10"},
							{Name: "fourthDownEff", DisplayValue: "0-1"},
						},
					},
				},
				Players: []espn.Players{
					{
						Team: espn.Team{ID: homeID},
						Statistics: []espn.PlayerStatistics{
							{
								Name:   "passing",
								Labels: []string{"C/ATT", "YDS", "TD", "INT"},
								Totals: []string{"20/30", "250", "3", "0"},
								Athletes: []espn.AthleteStats{
									{
										Athlete: espn.Athlete{ID: homeID*100 + 1, FirstName: "QB", LastName: "Home"},
										Stats:   []string{"20/30", "250", "3", "0"},
									},
								},
							},
							{
								Name:   "rushing",
								Labels: []string{"CAR", "YDS", "TD", "LONG"},
								Totals: []string{"35", "200", "1", "45"},
								Athletes: []espn.AthleteStats{
									{
										Athlete: espn.Athlete{ID: homeID*100 + 2, FirstName: "RB", LastName: "Home"},
										Stats:   []string{"35", "200", "1", "45"},
									},
								},
							},
						},
					},
					{
						Team: espn.Team{ID: awayID},
						Statistics: []espn.PlayerStatistics{
							{
								Name:   "passing",
								Labels: []string{"C/ATT", "YDS", "TD", "INT"},
								Totals: []string{"15/25", "180", "1", "1"},
								Athletes: []espn.AthleteStats{
									{
										Athlete: espn.Athlete{ID: awayID*100 + 1, FirstName: "QB", LastName: "Away"},
										Stats:   []string{"15/25", "180", "1", "1"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func fixtureTeamInfoResponse() espn.TeamInfoESPN {
	return espn.TeamInfoESPN{
		Sports: []espn.Sport{{
			ID:   90,
			Name: "Football",
			Slug: "football",
			Leagues: []espn.League{{
				ID:           23,
				Name:         "National Collegiate Athletic Association",
				Abbreviation: "NCAAF",
				ShortName:    "NCAAF",
				Slug:         "college-football",
				Year:         2023,
				Teams: []espn.TeamWrap{
					{Team: espn.TeamInfo{
						ID: 1, Name: "Crimson Tide", DisplayName: "Alpha Crimson Tide",
						Abbreviation: "ALP", Location: "Alpha", Slug: "alpha",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 2, Name: "Tigers", DisplayName: "Beta Tigers",
						Abbreviation: "BET", Location: "Beta", Slug: "beta",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 3, Name: "Wildcats", DisplayName: "Gamma Wildcats",
						Abbreviation: "GAM", Location: "Gamma", Slug: "gamma",
						IsActive: true,
					}},
					{Team: espn.TeamInfo{
						ID: 4, Name: "Bulldogs", DisplayName: "Delta Bulldogs",
						Abbreviation: "DEL", Location: "Delta", Slug: "delta",
						IsActive: true,
					}},
				},
			}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Mock HTTP server
// ---------------------------------------------------------------------------

// setupTestServer creates an httptest.Server that serves fixture data for all
// ESPN endpoints. The optional scoreOverride lets tests swap in different scores
// for a specific game ID (used by the score-change test). The override applies
// to both the schedule and game info endpoints.
func setupTestServer(t *testing.T, scoreOverride map[int64][2]int64) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/core/college-football/schedule", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := fixtureScheduleResponse()
		if scoreOverride != nil {
			for date, day := range resp.Content.Schedule {
				for i, g := range day.Games {
					if scores, ok := scoreOverride[g.ID]; ok {
						for j := range g.Competitions[0].Competitors {
							if g.Competitions[0].Competitors[j].HomeAway == "home" {
								g.Competitions[0].Competitors[j].Score = scores[0]
							} else {
								g.Competitions[0].Competitors[j].Score = scores[1]
							}
						}
						day.Games[i] = g
					}
				}
				resp.Content.Schedule[date] = day
			}
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/core/college-football/playbyplay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gameIDStr := r.URL.Query().Get("gameId")
		var gameID int64
		fmt.Sscanf(gameIDStr, "%d", &gameID) //nolint:errcheck // test helper

		resp := fixtureGameInfoResponse(gameID)
		if scoreOverride != nil {
			if scores, ok := scoreOverride[gameID]; ok {
				comps := resp.GamePackage.Header.Competitions[0].Competitors
				for i := range comps {
					if comps[i].HomeAway == "home" {
						comps[i].Score = scores[0]
					} else {
						comps[i].Score = scores[1]
					}
				}
				resp.GamePackage.Header.Competitions[0].Competitors = comps
			}
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/apis/site/v2/sports/football/college-football/teams", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fixtureTeamInfoResponse()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Test Updater constructor
// ---------------------------------------------------------------------------

func newTestUpdater(t *testing.T, scoreOverride map[int64][2]int64) (*Updater, *capturingWriter) {
	t.Helper()

	db := setupTestDB(t)
	cw := newCapturingWriter()
	ts := setupTestServer(t, scoreOverride)

	restore := espn.SetTestURLs(
		ts.URL+"/core/college-football/schedule?xhr=1&render=false&userab=18",
		ts.URL+"/core/college-football/playbyplay?gameId=%d&xhr=1&render=false&userab=18",
		ts.URL+"/apis/site/v2/sports/football/college-football/teams?limit=1000",
	)
	t.Cleanup(restore)

	client := &espn.Client{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		RequestTimeout: 5 * time.Second,
		RateLimit:      0,
	}

	u := &Updater{
		DB:     db,
		Logger: zap.NewNop().Sugar(),
		Writer: cw,
		ESPN:   client,
	}

	return u, cw
}

// seedTeamsAndSeasons inserts teams and seasons into the test database
// so that ranking and JSON-export tests have team context available.
// Includes 4 FBS teams and 2 FCS teams to exercise both ranking paths.
func seedTeamsAndSeasons(t *testing.T, db *gorm.DB) {
	t.Helper()

	teamNames := []database.TeamName{
		{TeamID: 1, Name: "Alpha", DisplayName: "Alpha Crimson Tide", Abbreviation: "ALP", Location: "Alpha", Slug: "alpha", IsActive: true},
		{TeamID: 2, Name: "Beta", DisplayName: "Beta Tigers", Abbreviation: "BET", Location: "Beta", Slug: "beta", IsActive: true},
		{TeamID: 3, Name: "Gamma", DisplayName: "Gamma Wildcats", Abbreviation: "GAM", Location: "Gamma", Slug: "gamma", IsActive: true},
		{TeamID: 4, Name: "Delta", DisplayName: "Delta Bulldogs", Abbreviation: "DEL", Location: "Delta", Slug: "delta", IsActive: true},
		{TeamID: 5, Name: "Epsilon", DisplayName: "Epsilon Eagles", Abbreviation: "EPS", Location: "Epsilon", Slug: "epsilon", IsActive: true},
		{TeamID: 6, Name: "Zeta", DisplayName: "Zeta Falcons", Abbreviation: "ZET", Location: "Zeta", Slug: "zeta", IsActive: true},
	}
	if err := db.Create(&teamNames).Error; err != nil {
		t.Fatalf("seed team_names: %v", err)
	}

	teamSeasons := []database.TeamSeason{
		{TeamID: 1, Year: 2023, FBS: 1, Conf: "SEC"},
		{TeamID: 2, Year: 2023, FBS: 1, Conf: "SEC"},
		{TeamID: 3, Year: 2023, FBS: 1, Conf: "Big Ten"},
		{TeamID: 4, Year: 2023, FBS: 1, Conf: "Big Ten"},
		{TeamID: 5, Year: 2023, FBS: 0, Conf: "MVFC"},
		{TeamID: 6, Year: 2023, FBS: 0, Conf: "MVFC"},
	}
	if err := db.Create(&teamSeasons).Error; err != nil {
		t.Fatalf("seed team_seasons: %v", err)
	}
}

// seedGames inserts completed fixture games directly into the database.
// Includes 4 FBS games and 2 FCS games.
func seedGames(t *testing.T, db *gorm.DB) {
	t.Helper()

	games := []database.Game{
		// FBS games
		{
			GameID: fixtureGameID1, Season: 2023, Week: 1,
			HomeID: 1, AwayID: 2, HomeScore: 28, AwayScore: 14,
			ConfGame: true,
			StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
		},
		{
			GameID: fixtureGameID2, Season: 2023, Week: 1,
			HomeID: 3, AwayID: 4, HomeScore: 21, AwayScore: 10,
			ConfGame: true,
			StartTime: time.Date(2023, 9, 2, 23, 0, 0, 0, time.UTC),
		},
		{
			GameID: fixtureGameID4, Season: 2023, Week: 2,
			HomeID: 1, AwayID: 3, HomeScore: 35, AwayScore: 17,
			StartTime: time.Date(2023, 9, 9, 23, 0, 0, 0, time.UTC),
		},
		{
			GameID: fixtureGameID5, Season: 2023, Week: 2,
			HomeID: 2, AwayID: 4, HomeScore: 24, AwayScore: 21,
			StartTime: time.Date(2023, 9, 9, 23, 0, 0, 0, time.UTC),
		},
		// FCS games
		{
			GameID: 501001, Season: 2023, Week: 1,
			HomeID: 5, AwayID: 6, HomeScore: 17, AwayScore: 10,
			ConfGame: true,
			StartTime: time.Date(2023, 9, 2, 20, 0, 0, 0, time.UTC),
		},
		{
			GameID: 501002, Season: 2023, Week: 2,
			HomeID: 6, AwayID: 5, HomeScore: 14, AwayScore: 21,
			ConfGame: true,
			StartTime: time.Date(2023, 9, 9, 20, 0, 0, 0, time.UTC),
		},
	}
	if err := db.Create(&games).Error; err != nil {
		t.Fatalf("seed games: %v", err)
	}
}
