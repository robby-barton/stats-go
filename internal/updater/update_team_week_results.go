package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/ranking"
	"github.com/robby-barton/stats-go/internal/ranking/load"
	"github.com/robby-barton/stats-go/internal/sport"
	"github.com/robby-barton/stats-go/internal/updater/fbsprior"
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

// rankingForWeek computes one ranking window. priorEnabled selects the
// scheduled football blend path: the FBS load resolves the window explicitly
// and the stored prior (if any) is read fresh for every run.
func (u *Updater) rankingForWeek(year int64, week int64, priorEnabled bool) ([]database.TeamWeekResult, error) {
	sportDB := u.sportDB()
	var teamWeekResults []database.TeamWeekResult

	// Sample the observed snapshot clock once, before the enabled FBS input
	// load, so the blend's window and expiry decisions cannot move mid-run.
	var now time.Time
	if u.priorEligible(priorEnabled, sportDB) {
		now = nowFunc()
	}

	newRanker := func(fcs bool) (*ranking.Ranker, ranking.Input, load.Window, error) {
		opts := load.Options{
			DB:    u.db,
			Sport: sportDB,
			Year:  year,
			Week:  week,
			Fcs:   fcs,
		}
		var (
			in     ranking.Input
			window load.Window
			err    error
		)
		if u.priorEligible(priorEnabled, sportDB) && !fcs {
			in, window, err = load.InputWindow(opts)
		} else {
			in, err = load.Input(opts)
		}
		if err != nil {
			return nil, ranking.Input{}, load.Window{}, err
		}
		ranker, err := ranking.NewRanker(in)
		return ranker, in, window, err
	}

	if u.espn.SportInfo() == espn.CollegeBasketball {
		// Basketball: single D1 ranking, no FBS/FCS split. The early-season
		// prior is football-only and is never passed to this path.
		ranker, _, _, err := newRanker(false)
		if err != nil {
			return nil, err
		}
		teamList, err := ranker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(teamList, true, sportDB)...)
	} else {
		fbsRanker, fbsIn, fbsWindow, err := newRanker(false)
		if err != nil {
			return nil, err
		}
		fbsRanking, err := fbsRanker.CalculateRanking()
		if err != nil {
			return nil, err
		}

		// Opt-in early-season blend: the stored prior is read per run, so a
		// row created after startup is usable without a restart. On any
		// problem this warns (or skips silently for normal skips) and leaves
		// every FBS row at baseline.
		if u.priorEligible(priorEnabled, sportDB) {
			if prior := u.storedPriorForWindow(fbsIn, fbsWindow, now); prior != nil {
				u.applyEarlySeasonPrior(fbsRanking, fbsIn, fbsWindow, prior, now)
			}
		}

		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fbsRanking, true, sportDB)...)

		fcsRanker, _, _, err := newRanker(true)
		if err != nil {
			return nil, err
		}
		fcsRanking, err := fcsRanker.CalculateRanking()
		if err != nil {
			return nil, err
		}
		teamWeekResults = append(teamWeekResults, teamListToTeamWeekResult(fcsRanking, false, sportDB)...)
	}

	return teamWeekResults, nil
}

// UpdateRecentRankings computes and upserts the latest rankings for the
// updater's sport, entirely at baseline.
func (u *Updater) UpdateRecentRankings() error {
	weekRankings, err := u.rankingForWeek(0, 0, false)
	if err != nil {
		return err
	}

	return u.insertRankingsToDB(weekRankings)
}

// UpdateRecentRankingsWithPrior is the scheduler-only football entry point.
// It behaves exactly like UpdateRecentRankings except that the opt-in FBS
// early-season blend can apply to the latest regular-season FBS window using
// the stored same-season prior read fresh from fbs_season_priors. With no
// stored row (or an unreadable one) it publishes the exact baseline; every
// other ranking path stays at baseline unconditionally.
func (u *Updater) UpdateRecentRankingsWithPrior() error {
	weekRankings, err := u.rankingForWeek(0, 0, true)
	if err != nil {
		return err
	}

	return u.insertRankingsToDB(weekRankings)
}

// priorEligible reports whether the opt-in blend can apply at all: enabled
// on the scheduled football ranking path only. Basketball and every other
// sport are unaffected by construction.
func (u *Updater) priorEligible(priorEnabled bool, sportDB sport.Sport) bool {
	return priorEnabled && sportDB == sport.Football
}

// storedPriorForWindow reads the stored prior for the resolved latest FBS
// window, validating the exact stored bytes against the stored checksum.
// It returns nil for the normal silent skips (no persisted window, a
// non-regular stored phase, a window from another season than the loaded
// one, or the fixed day-56 calendar expiry). An absent, unreadable, or
// invalid row warns once per run and stays on baseline; an existing row
// is never repaired or replaced.
func (u *Updater) storedPriorForWindow(
	in ranking.Input,
	window load.Window,
	now time.Time,
) *fbsprior.Prior {
	if !window.Found || window.Postseason != 0 || window.Season != in.Year {
		return nil
	}
	expiry, err := fbsprior.CalendarExpiryForSeason(window.Season)
	if err != nil {
		return nil
	}
	if !now.Before(expiry) {
		return nil
	}

	var row database.FBSSeasonPrior
	if err := u.db.Where("season = ?", window.Season).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			u.logger.Warnf("fbs early-season prior: no frozen prior for season %d; staying on baseline rankings", window.Season)
			return nil
		}
		u.logger.Warnf("fbs early-season prior: reading the stored prior: %v; staying on baseline rankings", err)
		return nil
	}

	data := []byte(row.Artifact)
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != row.Checksum {
		u.logger.Warnf("fbs early-season prior: stored prior for season %d fails its checksum; staying on baseline rankings",
			window.Season)
		return nil
	}
	prior, err := fbsprior.Parse(data)
	if err != nil {
		u.logger.Warnf("fbs early-season prior: stored prior for season %d is invalid: %v; staying on baseline rankings",
			window.Season, err)
		return nil
	}
	if prior.Season != window.Season {
		u.logger.Warnf(
			"fbs early-season prior: stored prior keyed by season %d contains season %d; staying on baseline rankings",
			window.Season, prior.Season)
		return nil
	}
	return prior
}

// applyEarlySeasonPrior blends the frozen preseason prior into the baseline
// FBS ranking, all or nothing. On success it substitutes FinalRaw and
// FinalRank on the staged FBS team list only; on any failure it warns and
// leaves every row at the exact baseline. Normal skips (no persisted game,
// no current-season game yet, a non-regular stored phase, or the fixed
// day-56 calendar expiry) stay at baseline without a warning.
func (u *Updater) applyEarlySeasonPrior(
	teams ranking.TeamList,
	in ranking.Input,
	window load.Window,
	prior *fbsprior.Prior,
	now time.Time,
) {
	stayBaseline := func(format string, args ...any) {
		u.logger.Warnf("fbs early-season prior: "+format+"; staying on baseline rankings", args...)
	}

	// The eligible window's clocks must be consistent with the prior.
	if window.Kickoff.Before(prior.AnchorInstant) {
		stayBaseline("latest game kickoff %s predates the preseason anchor %s",
			window.Kickoff.UTC().Format(time.RFC3339), prior.AnchorInstant.Format(time.RFC3339))
		return
	}
	if window.Kickoff.After(now) {
		stayBaseline("latest game kickoff %s is after the observed clock %s",
			window.Kickoff.UTC().Format(time.RFC3339), now.Format(time.RFC3339))
		return
	}
	if !window.Kickoff.Equal(in.StartTime) {
		stayBaseline("resolved cutoff %s does not match the latest game kickoff %s",
			in.StartTime.Format(time.RFC3339), window.Kickoff.UTC().Format(time.RFC3339))
		return
	}
	if in.Week != window.Week+1 {
		stayBaseline("resolved week %d does not follow the latest game week %d", in.Week, window.Week)
		return
	}
	if now.Before(prior.CapturedAt) {
		stayBaseline("observed clock %s precedes the prior capture %s",
			now.Format(time.RFC3339), prior.CapturedAt.Format(time.RFC3339))
		return
	}
	if prior.Season != in.Year {
		stayBaseline("prior season %d does not match the loaded season %d", prior.Season, in.Year)
		return
	}
	if !now.Before(prior.CalendarExpiry) {
		// Safe normal expiry: the fixed 56-Eastern-day boundary has passed.
		return
	}
	if len(teams) == 0 {
		return
	}
	if !prior.CoversField(teams) {
		stayBaseline("current FBS field diverged from the prior field")
		return
	}

	adjusted, err := fbsprior.Apply(teams, prior, now)
	if err != nil {
		stayBaseline("blend failed: %v", err)
		return
	}

	// Success: substitute only the blended scores and ranks.
	for id, adj := range adjusted {
		teams[id].FinalRaw = adj.FinalRaw
		teams[id].FinalRank = adj.FinalRank
	}
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
			weekRankings, err := u.rankingForWeek(year.Year, week, false)
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
		weekRankings, err := u.rankingForWeek(year.Year, 0, false)
		if err != nil {
			return err
		}
		teamWeekResults = append(teamWeekResults, weekRankings...)
	}

	return u.insertRankingsToDB(teamWeekResults)
}
