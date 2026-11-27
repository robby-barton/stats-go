package updater

import (
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
		return upsertTeamWeekResults(tx, rankings)
	})
}

// upsertTeamWeekResults upserts one batch of ranking rows on the given
// handle: the updater's database, or the publication-serialized transaction
// shared with the rebuilt-prior publication.
func upsertTeamWeekResults(tx *gorm.DB, rankings []database.TeamWeekResult) error {
	if len(rankings) == 0 {
		return nil
	}
	return tx.
		Clauses(clause.OnConflict{
			UpdateAll: true, // upsert
		}).
		CreateInBatches(rankings, 1000).Error
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

// rankingForWeek computes one ranking window. run selects the opt-in
// football blend path (nil computes the exact baseline everywhere): the
// scheduler's latest window and the inline `ranking --all` rebuild both
// resolve the FBS window explicitly and go through the one shared
// application path with a run-local prior memo.
func (u *Updater) rankingForWeek(year int64, week int64, run *priorRun) ([]database.TeamWeekResult, error) {
	sportDB := u.sportDB()
	var teamWeekResults []database.TeamWeekResult

	newRanker := func(fcs bool) (*ranking.Ranker, ranking.Input, load.Window, error) {
		opts := load.Options{
			DB:    runDB(u, run),
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
		if run != nil && !fcs {
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

	if u.espn.SportInfo().IsBasketball() {
		// Basketball (men's and women's): single D1 ranking, no FBS/FCS split.
		// The early-season prior is football-only and is never passed here.
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

		// Opt-in early-season blend: the prior is loaded through the run-local
		// memo, so a row created after startup is usable without a restart and
		// one load or reconstruction is shared by every window of a season. On
		// any problem this warns (or skips silently for normal skips) and
		// leaves every FBS row at baseline.
		if run != nil {
			// The scheduler's latest slot (year 0, week 0) and the rebuild's
			// current-season week-0 slot use the actual observation clock so
			// they agree with the normal scheduler even when the last game is
			// stale; every explicit historical week and past-season final
			// regular snapshot uses its own resolved ranking cutoff instead.
			latest := week == 0 && (year == 0 || year == run.currentSeason)
			clock := run.now
			if !latest {
				clock = fbsIn.StartTime
			}
			if prior := u.priorForWindow(fbsIn, fbsWindow, clock, latest, run); prior != nil {
				u.applyPriorWindow(fbsRanking, fbsIn, fbsWindow, prior, clock, latest)
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
	weekRankings, err := u.rankingForWeek(0, 0, nil)
	if err != nil {
		return err
	}

	return u.insertRankingsToDB(weekRankings)
}

// UpdateRecentRankingsWithPrior is the scheduled football entry point.
// It behaves exactly like UpdateRecentRankings except that the opt-in FBS
// early-season blend can apply to the latest regular-season FBS window using
// the stored same-season prior read fresh from fbs_season_priors. With no
// stored row (or an unreadable one) it publishes the exact baseline.
//
// The whole read/calc/write runs inside one publication-serialized
// transaction (the shared advisory xact lock on PostgreSQL, the up-front
// write lock on SQLite), so an opted-in scheduled run can never read the old
// prior and commit after a corrected rebuild publication. With the feature
// off nothing takes the lock.
func (u *Updater) UpdateRecentRankingsWithPrior() error {
	if u.espn.SportInfo() != espn.CollegeFootball {
		// Basketball never blends: plain baseline upsert, no prior locking.
		return u.UpdateRecentRankings()
	}
	return publicationTransaction(u.db, func(tx *gorm.DB) error {
		run := newPriorRun(nowFunc(), false)
		run.tx = tx
		weekRankings, err := u.rankingForWeek(0, 0, run)
		if err != nil {
			return err
		}
		return upsertTeamWeekResults(tx, weekRankings)
	})
}

// applyPriorWindow is the one shared application path for the opt-in FBS
// early-season blend, used by the scheduled worker (latest window, observed
// clock) and the inline `ranking --all` rebuild (explicit historical windows
// and past-season final snapshots, resolved cutoffs) after each path's own
// window eligibility. It blends the frozen preseason prior into the baseline
// FBS ranking, all or nothing: on success it substitutes FinalRaw and
// FinalRank on the staged FBS team list only; on any failure it warns and
// leaves every row at the exact baseline. Normal skips (no persisted game,
// no current-season game yet, a non-regular stored phase, or the fixed
// day-56 calendar expiry) stay at baseline without a warning.
func (u *Updater) applyPriorWindow(
	teams ranking.TeamList,
	in ranking.Input,
	window load.Window,
	prior *fbsprior.Prior,
	clock time.Time,
	latest bool,
) {
	stayBaseline := func(format string, args ...any) {
		u.logger.Warnf("fbs early-season prior: "+format+"; staying on baseline rankings", args...)
	}

	// The eligible window's clocks must be consistent with the prior.
	if latest {
		// Scheduler-only consistency checks for the latest window: the
		// cutoff must be the latest game's kickoff, and that kickoff must
		// follow the season and precede the observed clock.
		if window.Kickoff.Before(prior.AnchorInstant) {
			stayBaseline("latest game kickoff %s predates the preseason anchor %s",
				window.Kickoff.UTC().Format(time.RFC3339), prior.AnchorInstant.Format(time.RFC3339))
			return
		}
		if window.Kickoff.After(clock) {
			stayBaseline("latest game kickoff %s is after the observed clock %s",
				window.Kickoff.UTC().Format(time.RFC3339), clock.Format(time.RFC3339))
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
	} else if clock.Before(prior.AnchorInstant) {
		stayBaseline("resolved cutoff %s predates the preseason anchor %s",
			clock.Format(time.RFC3339), prior.AnchorInstant.Format(time.RFC3339))
		return
	}
	if !prior.CapturedAt.IsZero() && clock.Before(prior.CapturedAt) {
		stayBaseline("snapshot clock %s precedes the prior capture %s",
			clock.Format(time.RFC3339), prior.CapturedAt.Format(time.RFC3339))
		return
	}
	if prior.Season != in.Year {
		stayBaseline("prior season %d does not match the loaded season %d", prior.Season, in.Year)
		return
	}
	if !clock.Before(prior.CalendarExpiry) {
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

	adjusted, err := fbsprior.Apply(teams, prior, clock)
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
	return u.updateAllRankings(nil)
}

// UpdateAllRankingsWithPrior is the opt-in inline rebuild behind
// `updater ncaaf ranking --all --fbs-early-season-prior`. It computes every
// ranking window exactly like UpdateAllRankings — the baseline loader and
// score calculation run for every row — and additionally applies the
// scheduled FBS early-season policy to eligible regular-season windows.
// It always reconstructs the prior freshly in memory once per season in
// scope, including the current Eastern season, never using stored values as
// inputs; as the authoritative explicit rebuild it then publishes the
// rebuilt priors — replacing existing rows, including the current season's,
// midseason allowed — atomically with the rebuilt ranking upserts, and
// invalidates the stale persisted prior of any season whose reconstruction
// could not yield a usable prior. Any prior or ranking write failure rolls
// the whole publication back.
func (u *Updater) UpdateAllRankingsWithPrior() error {
	if u.espn.SportInfo() != espn.CollegeFootball {
		return errors.New("fbs early-season prior: football updater required")
	}
	run := newPriorRun(nowFunc(), true)
	run.publish = true
	return u.updateAllRankings(run)
}

// updateAllRankings iterates every persisted season's explicit regular weeks
// plus each season's week-0 slot, upserting the computed rows in one
// transaction. run selects the opt-in prior path (nil is the exact baseline);
// a publishing run stages one fresh reconstruction per season in scope
// before the ranking loop and then publishes the rebuilt priors atomically
// with the ranking upserts.
func (u *Updater) updateAllRankings(run *priorRun) error {
	var teamWeekResults []database.TeamWeekResult

	yearInfo, err := u.getYearInfo()
	if err != nil {
		return err
	}

	// The rebuild publication stages one fresh reconstruction attempt per
	// season in scope before any lock is taken: the ranking math below
	// consumes only these in-memory priors, and the locked write phase stays
	// minimal.
	var seasons []int64
	if run != nil && run.publish {
		seasons, err = u.rebuildPriorSeasons(run, yearInfo)
		if err != nil {
			return err
		}
		for _, season := range seasons {
			u.priorForSeason(run, season)
			if run.readErr != nil {
				return run.readErr
			}
		}
	}

	for _, year := range yearInfo {
		weeks, err := u.regularSeasonWeeks(year.Year)
		if err != nil {
			return err
		}
		for _, week := range weeks {
			u.logger.Infof("%d/%d", year.Year, week)
			weekRankings, err := u.rankingForWeek(year.Year, week, run)
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
		weekRankings, err := u.rankingForWeek(year.Year, 0, run)
		if err != nil {
			return err
		}
		teamWeekResults = append(teamWeekResults, weekRankings...)
	}

	if run != nil && run.publish {
		return u.publishRebuiltRankings(run, seasons, teamWeekResults)
	}
	return u.insertRankingsToDB(teamWeekResults)
}
