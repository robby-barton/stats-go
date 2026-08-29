package updater

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/game"
	"github.com/robby-barton/stats-go/internal/sport"
)

func (u *Updater) checkGames(games []espn.Game) ([]espn.Game, error) {
	gameIDs := []int64{}
	for _, game := range games {
		gameIDs = append(gameIDs, game.ID)
	}
	var existing []database.Game
	if err := u.DB.Where("game_id in ? and sport = ?", gameIDs, u.sportDB()).Find(&existing).Error; err != nil {
		return nil, err
	}

	existsMap := map[int64]database.Game{}
	for _, x := range existing {
		existsMap[x.GameID] = x
	}

	var newGames []espn.Game
	for _, game := range games {
		existingGame, ok := existsMap[game.ID]
		if !ok {
			newGames = append(newGames, game)
			continue
		}

		// Malformed schedule entries cannot be score-compared; send them
		// through the full single-game path (which validates strictly) rather
		// than indexing blindly.
		if len(game.Competitions) == 0 || len(game.Competitions[0].Competitors) < 2 {
			newGames = append(newGames, game)
			continue
		}

		teams := game.Competitions[0]
		home := teams.Competitors[0]
		away := teams.Competitors[1]
		if home.HomeAway == "away" {
			home, away = away, home
		}
		if existingGame.Retry == 1 {
			// A previous fetch failed; always retry regardless of score match.
			newGames = append(newGames, game)
		} else if existingGame.HomeScore != home.Score || existingGame.AwayScore != away.Score {
			newGames = append(newGames, game)
		}
	}

	return newGames, nil
}

// The functions below map the game package's domain structs onto the GORM
// persistence models. The mapping lives here (not in game/) so the parser
// stays free of database concerns.

func gameInfoToDB(in game.Info, sp sport.Sport) database.Game {
	return database.Game{
		GameID:     in.GameID,
		StartTime:  in.StartTime,
		Sport:      sp,
		Neutral:    in.Neutral,
		ConfGame:   in.ConfGame,
		Season:     in.Season,
		Week:       in.Week,
		Postseason: in.Postseason,
		HomeID:     in.HomeID,
		HomeScore:  in.HomeScore,
		AwayID:     in.AwayID,
		AwayScore:  in.AwayScore,
	}
}

func teamStatsToDB(in []game.TeamGameStats) []database.TeamGameStats {
	out := make([]database.TeamGameStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.TeamGameStats{
			GameID:             s.GameID,
			TeamID:             s.TeamID,
			Score:              s.Score,
			Drives:             s.Drives,
			PassYards:          s.PassYards,
			Completions:        s.Completions,
			CompletionAttempts: s.CompletionAttempts,
			RushYards:          s.RushYards,
			RushAttempts:       s.RushAttempts,
			FirstDowns:         s.FirstDowns,
			ThirdDowns:         s.ThirdDowns,
			ThirdDownsConv:     s.ThirdDownsConv,
			FourthDowns:        s.FourthDowns,
			FourthDownsConv:    s.FourthDownsConv,
			Fumbles:            s.Fumbles,
			Interceptions:      s.Interceptions,
			Possession:         s.Possession,
			Penalties:          s.Penalties,
			PenaltyYards:       s.PenaltyYards,
		})
	}
	return out
}

func passingStatsToDB(in []game.PassingStats) []database.PassingStats {
	out := make([]database.PassingStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.PassingStats{
			PlayerID:      s.PlayerID,
			TeamID:        s.TeamID,
			GameID:        s.GameID,
			Completions:   s.Completions,
			Attempts:      s.Attempts,
			Yards:         s.Yards,
			Touchdowns:    s.Touchdowns,
			Interceptions: s.Interceptions,
		})
	}
	return out
}

func rushingStatsToDB(in []game.RushingStats) []database.RushingStats {
	out := make([]database.RushingStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.RushingStats{
			PlayerID:   s.PlayerID,
			TeamID:     s.TeamID,
			GameID:     s.GameID,
			Carries:    s.Carries,
			RushYards:  s.RushYards,
			RushLong:   s.RushLong,
			Touchdowns: s.Touchdowns,
		})
	}
	return out
}

func receivingStatsToDB(in []game.ReceivingStats) []database.ReceivingStats {
	out := make([]database.ReceivingStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.ReceivingStats{
			PlayerID:   s.PlayerID,
			TeamID:     s.TeamID,
			GameID:     s.GameID,
			Receptions: s.Receptions,
			RecYards:   s.RecYards,
			RecLong:    s.RecLong,
			Touchdowns: s.Touchdowns,
		})
	}
	return out
}

func fumbleStatsToDB(in []game.FumbleStats) []database.FumbleStats {
	out := make([]database.FumbleStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.FumbleStats{
			PlayerID:    s.PlayerID,
			TeamID:      s.TeamID,
			GameID:      s.GameID,
			Fumbles:     s.Fumbles,
			FumblesLost: s.FumblesLost,
			FumblesRec:  s.FumblesRec,
		})
	}
	return out
}

func defensiveStatsToDB(in []game.DefensiveStats) []database.DefensiveStats {
	out := make([]database.DefensiveStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.DefensiveStats{
			PlayerID:       s.PlayerID,
			TeamID:         s.TeamID,
			GameID:         s.GameID,
			PassesDef:      s.PassesDef,
			QBHurries:      s.QBHurries,
			Sacks:          s.Sacks,
			SoloTackles:    s.SoloTackles,
			Touchdowns:     s.Touchdowns,
			TacklesForLoss: s.TacklesForLoss,
			TotalTackles:   s.TotalTackles,
		})
	}
	return out
}

func interceptionStatsToDB(in []game.InterceptionStats) []database.InterceptionStats {
	out := make([]database.InterceptionStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.InterceptionStats{
			PlayerID:      s.PlayerID,
			TeamID:        s.TeamID,
			GameID:        s.GameID,
			Interceptions: s.Interceptions,
			Touchdowns:    s.Touchdowns,
			IntYards:      s.IntYards,
		})
	}
	return out
}

func returnStatsToDB(in []game.ReturnStats) []database.ReturnStats {
	out := make([]database.ReturnStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.ReturnStats{
			PlayerID:   s.PlayerID,
			TeamID:     s.TeamID,
			GameID:     s.GameID,
			PuntKick:   s.PuntKick,
			ReturnNo:   s.ReturnNo,
			Touchdowns: s.Touchdowns,
			RetYards:   s.RetYards,
			RetLong:    s.RetLong,
		})
	}
	return out
}

func kickStatsToDB(in []game.KickStats) []database.KickStats {
	out := make([]database.KickStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.KickStats{
			PlayerID: s.PlayerID,
			TeamID:   s.TeamID,
			GameID:   s.GameID,
			FGA:      s.FGA,
			FGM:      s.FGM,
			FGLong:   s.FGLong,
			XPA:      s.XPA,
			XPM:      s.XPM,
			Points:   s.Points,
		})
	}
	return out
}

func puntStatsToDB(in []game.PuntStats) []database.PuntStats {
	out := make([]database.PuntStats, 0, len(in))
	for _, s := range in {
		out = append(out, database.PuntStats{
			PlayerID:   s.PlayerID,
			TeamID:     s.TeamID,
			GameID:     s.GameID,
			PuntLong:   s.PuntLong,
			PuntNo:     s.PuntNo,
			PuntYards:  s.PuntYards,
			Touchbacks: s.Touchbacks,
			Inside20:   s.Inside20,
		})
	}
	return out
}

func (u *Updater) insertGameInfo(parsed *game.ParsedGameInfo) error {
	if parsed == nil {
		return errors.New("game nil")
	}

	dbGameInfo := gameInfoToDB(parsed.Info, u.sportDB())
	dbTeamStats := teamStatsToDB(parsed.TeamStats)
	dbPassingStats := passingStatsToDB(parsed.PassingStats)
	dbRushingStats := rushingStatsToDB(parsed.RushingStats)
	dbReceivingStats := receivingStatsToDB(parsed.ReceivingStats)
	dbFumbleStats := fumbleStatsToDB(parsed.FumbleStats)
	dbDefensiveStats := defensiveStatsToDB(parsed.DefensiveStats)
	dbInterceptionStats := interceptionStatsToDB(parsed.InterceptionStats)
	dbReturnStats := returnStatsToDB(parsed.ReturnStats)
	dbKickStats := kickStatsToDB(parsed.KickStats)
	dbPuntStats := puntStatsToDB(parsed.PuntStats)

	return u.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.OnConflict{
				UpdateAll: true, // upsert
			}).
			Create(&dbGameInfo).Error; err != nil {
			return err
		}

		if len(dbTeamStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbTeamStats).Error; err != nil {
				return err
			}
		}

		if len(dbPassingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbPassingStats).Error; err != nil {
				return err
			}
		}

		if len(dbRushingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbRushingStats).Error; err != nil {
				return err
			}
		}

		if len(dbReceivingStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbReceivingStats).Error; err != nil {
				return err
			}
		}

		if len(dbFumbleStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbFumbleStats).Error; err != nil {
				return err
			}
		}

		if len(dbDefensiveStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbDefensiveStats).Error; err != nil {
				return err
			}
		}

		if len(dbInterceptionStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbInterceptionStats).Error; err != nil {
				return err
			}
		}

		if len(dbReturnStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
					Columns: []clause.Column{
						{Name: "player_id"},
						{Name: "team_id"},
						{Name: "game_id"},
						{Name: "punt_kick"},
					},
				}).
				Create(&dbReturnStats).Error; err != nil {
				return err
			}
		}

		if len(dbKickStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbKickStats).Error; err != nil {
				return err
			}
		}

		if len(dbPuntStats) > 0 {
			if err := tx.
				Clauses(clause.OnConflict{
					UpdateAll: true, // upsert
				}).
				Create(&dbPuntStats).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

const gamesBatchSize = 100

// FailedGame records a game whose ESPN fetch failed.
type FailedGame struct {
	GameID int64
	Err    error
}

// GameUpdateResult separates successfully persisted games from per-game
// failures so callers can report partial results instead of silently
// dropping them. Failed games are also persisted with retry=1 (see
// markGameRetry) so the next update cycle re-fetches them.
type GameUpdateResult struct {
	Processed []int64
	Failed    []FailedGame
}

// FailedIDs returns the IDs of games that failed to fetch.
func (r *GameUpdateResult) FailedIDs() []int64 {
	ids := make([]int64, len(r.Failed))
	for i, f := range r.Failed {
		ids[i] = f.GameID
	}
	return ids
}

// markGameRetry durably records a failed fetch using the games.retry column
// so the game is re-fetched on the next update cycle. If the game already has
// a row, only the retry flag is updated; otherwise a minimal row is seeded
// from the schedule entry. A successful later fetch clears the flag because
// insertGameInfo upserts the full row (retry defaults to 0).
func (u *Updater) markGameRetry(g espn.Game) error {
	retry := &database.Game{GameID: g.ID, Sport: u.sportDB(), Retry: 1}

	if len(g.Competitions) > 0 && len(g.Competitions[0].Competitors) >= 2 {
		comp := g.Competitions[0]
		home, away := comp.Competitors[0], comp.Competitors[1]
		if home.HomeAway == "away" {
			home, away = away, home
		}
		retry.HomeID = home.ID
		retry.AwayID = away.ID
		retry.HomeScore = home.Score
		retry.AwayScore = away.Score
	}

	return u.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "game_id"}},
		DoUpdates: clause.Assignments(map[string]any{"retry": 1}),
	}).Create(retry).Error
}

func (u *Updater) processGames(ctx context.Context, games []espn.Game) (*GameUpdateResult, error) {
	result := &GameUpdateResult{}

	for start := 0; start < len(games); start += gamesBatchSize {
		end := min(start+gamesBatchSize, len(games))
		batch := games[start:end]

		for _, g := range batch {
			stats, err := game.GetSingleGame(ctx, u.ESPN, u.Logger, g.ID)
			if err != nil {
				u.Logger.Warnf("skipping game %d: %v", g.ID, err)
				result.Failed = append(result.Failed, FailedGame{GameID: g.ID, Err: err})
				if markErr := u.markGameRetry(g); markErr != nil {
					u.Logger.Errorf("failed to mark game %d for retry: %v", g.ID, markErr)
				}
				continue
			}
			if err := u.insertGameInfo(stats); err != nil {
				return result, fmt.Errorf("persisting game %d: %w", g.ID, err)
			}
			result.Processed = append(result.Processed, stats.Info.GameID)
			u.ESPN.Throttle(ctx)
		}

		u.Logger.Infof("processed %d/%d games", end, len(games))
	}

	return result, nil
}

func (u *Updater) UpdateCurrentWeek(ctx context.Context) (*GameUpdateResult, error) {
	games, err := game.GetCurrentWeekGames(ctx, u.ESPN)
	if err != nil {
		return nil, err
	}

	games, err = u.checkGames(games)
	if err != nil {
		return nil, err
	}

	return u.processGames(ctx, games)
}

func (u *Updater) UpdateGamesForYear(ctx context.Context, year int64) (*GameUpdateResult, error) {
	games, err := game.GetGamesForSeason(ctx, u.ESPN, year)
	if err != nil {
		return nil, err
	}

	games, err = u.checkGames(games)
	if err != nil {
		return nil, err
	}

	return u.processGames(ctx, games)
}

func (u *Updater) UpdateSingleGame(ctx context.Context, gameID int64) error {
	gameStats, err := game.GetSingleGame(ctx, u.ESPN, u.Logger, gameID)
	if err != nil {
		return err
	}

	return u.insertGameInfo(gameStats)
}
