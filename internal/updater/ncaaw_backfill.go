package updater

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/robby-barton/stats-go/internal/database"
	"github.com/robby-barton/stats-go/internal/espn"
	"github.com/robby-barton/stats-go/internal/sport"
)

// ---------------------------------------------------------------------------
// Strict ncaaw backfill
// ---------------------------------------------------------------------------
//
// BackfillNCAAW is the strict women's basketball replacement for the generic
// `updater ncaaw backfill` path. For every requested season-ending year it
//
//  1. fetches and validates ESPN's year-specific flat calendar for the
//     season (fail closed on an invalid, empty, or wrong-year calendar —
//     the calendar is a validation artifact, never the game-day list),
//  2. walks every ET calendar day of the season against the D1 group-50
//     scoreboard exactly once (the "one daily walk" — the only
//     verified-complete discovery form), failing closed on any unusable or
//     wrong-season daily response,
//  3. imports D-I membership from that same walk (never re-walking guessed
//     dates), upserts team seasons, and backfills missing identities,
//  4. imports every observed completed final through the shared
//     checkGames/processGames path, then
//  5. audits every advertised final against the stored row (same event ID,
//     requested season, opponents, scores, retry=0, valid regular week >0,
//     missing postseason week tolerated), catches stale/orphan rows and
//     ambiguous duplicate matchups, classifies every team in a stored game
//     (legitimate lower-division opponents are stored but never ranked;
//     unclassified teams that are likely D-I fail the run), and validates
//     identity metadata — for EVERY requested season,
//
// and only then publishes ranking rows for the requested years, computed via
// the existing regularSeasonWeeks/rankingForWeek primitives and replaced
// in one atomic transaction. Any upstream, game,
// or metadata audit failure aborts the run before any ranking write, so old
// ranking rows are unchanged on failure.
//
// Known limitation: ESPN’s conference labels and bulk D-I team listing are
// current-only. A scoreboard conference that no longer exists can require
// manual membership reconciliation; corroborate historical membership before
// using these rows for release.

// ncaawSeasonWindow bounds a women's basketball season ending in year Y:
// no earlier than November 1 of Y-1 and no later than April 30 of Y (the
// live-checked 2026 and 2027 flat calendars both sit inside this window;
// the championship lands in early April). Dates are ET calendar dates.
func ncaawSeasonWindow(year int64) (time.Time, time.Time) {
	loc := ncaawETLoc()
	start := time.Date(int(year)-1, time.November, 1, 0, 0, 0, 0, loc)
	end := time.Date(int(year), time.April, 30, 23, 59, 59, 0, loc)
	return start, end
}

// ncaawETLoc returns the fixed Eastern location the week and day boundaries
// are computed in; it falls back to UTC when the zone database is missing.
func ncaawETLoc() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}

const ncaawFeedDateLayout = "2006-01-02T15:04Z"

// parseNCAAWDay validates ESPN's ISO calendar entry and returns its civil date.
func parseNCAAWDay(iso string) (time.Time, error) {
	day, err := time.Parse(ncaawFeedDateLayout, iso)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing calendar date %q: %w", iso, err)
	}
	return day.Truncate(24 * time.Hour), nil
}

// ncaawWalkDays lists every ET calendar day the season's daily walk must
// touch: November 1 of year-1 through the earlier of April 30 of year and
// today (Eastern). Future dates of the in-progress season are not walked —
// nothing final can exist there, and the scheduled poll owns them.
func ncaawWalkDays(year int64, now time.Time) ([]string, error) {
	start, end := ncaawSeasonWindow(year)
	now = now.In(start.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, start.Location())
	if today.Before(start) {
		return nil, fmt.Errorf("ncaaw backfill %d: the season has not started (walk begins %s)",
			year, start.Format("2006-01-02"))
	}
	if today.Before(end) {
		end = today
	}
	var days []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format("20060102"))
	}
	return days, nil
}

// ncaawSeasonFeed is the validated output of one season's daily walk: every
// observed completed final, the team → conference-ID observations collected
// from the same responses (all statuses; conference IDs are identical in
// every phase), and the ET days that advertised at least one event.
type ncaawSeasonFeed struct {
	finals      []espn.Game
	finalMeta   map[int64]ncaawFinalMetadata
	teamConfs   map[int64]int64
	rankedTeams map[int64]string
	gameDays    map[string]bool
}

type ncaawFinalMetadata struct {
	start      time.Time
	postseason int64
}

// BackfillNCAAW runs the strict ncaaw backfill over the requested
// season-ending years [fromYear, toYear]. publishFrom (0 means toYear) is
// the earliest year whose ranking rows are published; earlier years in the
// range are imported and audited as lookback input only. See the package
// comment above for the full contract.
func (u *Updater) BackfillNCAAW(ctx context.Context, fromYear, toYear, publishFrom int64) error {
	if u.sportDB() != sport.WomensBasketball {
		return fmt.Errorf("strict backfill is ncaaw-only; %s keeps the generic backfill", u.sportDB())
	}
	if fromYear <= 0 || toYear <= 0 || fromYear > toYear || toYear >= 10000 {
		return fmt.Errorf("ncaaw backfill: --from and --to must be positive with from <= to")
	}
	if publishFrom == 0 {
		publishFrom = toYear
	}
	if publishFrom < fromYear || publishFrom > toYear {
		return fmt.Errorf("ncaaw backfill: --publish-from %d must sit inside the import range [%d, %d]",
			publishFrom, fromYear, toYear)
	}
	// The ranking's one-season lookback (YearsBack = 1) means publishing a
	// season requires its prior season inside the audited import range: the
	// default `--publish-from = --to` therefore needs one lookback year
	// imported (and fully audited) below it, and explicitly exposing an
	// older year requires its own lookback too.
	if publishFrom-1 < fromYear {
		return fmt.Errorf(
			"ncaaw backfill: publishing %d requires the %d lookback season inside the audited "+
				"import range; lower --from to include it", publishFrom, publishFrom-1)
	}

	// Shared, current-only classification inputs: the D-I conference map
	// (conference ID → short name) and the bulk D-I team listing used to
	// detect unclassified likely D-I teams.
	d1Confs, err := u.ncaawD1Conferences(ctx)
	if err != nil {
		return err
	}
	bulkD1, err := u.ncaawBulkD1Teams(ctx)
	if err != nil {
		return err
	}

	for year := fromYear; year <= toYear; year++ {
		u.logger.Infof("ncaaw backfill %d: walking the season...", year)
		feed, err := u.ncaawWalkSeason(ctx, year)
		if err != nil {
			return err
		}
		u.logger.Infof("ncaaw backfill %d: %d finals on %d game days, %d conference observations",
			year, len(feed.finals), len(feed.gameDays), len(feed.teamConfs))

		if err := u.ncaawImportSeasonMetadata(ctx, year, feed, d1Confs); err != nil {
			return err
		}
		result, err := u.ncaawImportGames(ctx, year, feed)
		if err != nil {
			return err
		}
		// Fail closed before any later year is touched: a bad season never
		// reaches publication.
		if err := u.ncaawAuditYear(year, feed, result, bulkD1); err != nil {
			return err
		}
	}

	return u.ncaawPublish(publishFrom, toYear)
}

// ---------------------------------------------------------------------------
// Walk
// ---------------------------------------------------------------------------

// ncaawWalkSeason fetches the year's authoritative flat calendar, validates
// it, then walks every ET day of the season once against the D1 group-50
// scoreboard. Any unusable or wrong-season response fails the run.
func (u *Updater) ncaawWalkSeason(ctx context.Context, year int64) (*ncaawSeasonFeed, error) {
	// 1. The advertised flat calendar must be usable and belong to this
	// season-ending year.
	calendar, err := u.espn.GetSeasonDatesForYear(ctx, year)
	if err != nil {
		return nil, fmt.Errorf("ncaaw backfill %d: fetching the flat season calendar: %w", year, err)
	}
	if len(calendar) == 0 {
		return nil, fmt.Errorf("ncaaw backfill %d: the advertised flat calendar is empty; failing closed", year)
	}
	// Compare the calendar's date portion as a civil date against the season window.
	windowStart, windowEnd := ncaawSeasonWindow(year)
	for _, iso := range calendar {
		day, err := parseNCAAWDay(iso)
		if err != nil {
			return nil, fmt.Errorf("ncaaw backfill %d: %w; failing closed", year, err)
		}
		if day.Before(time.Date(windowStart.Year(), time.November, 1, 0, 0, 0, 0, time.UTC)) ||
			day.After(time.Date(windowEnd.Year(), time.April, 30, 0, 0, 0, 0, time.UTC)) {
			return nil, fmt.Errorf(
				"ncaaw backfill %d: advertised calendar date %s sits outside the %d season window (%s to %s); failing closed",
				year, day.Format("2006-01-02"), year,
				windowStart.Format("2006-01-02"), windowEnd.Format("2006-01-02"))
		}
	}

	// 2. One daily walk over the season's ET days, D1 group 50 only.
	days, err := ncaawWalkDays(year, nowFunc())
	if err != nil {
		return nil, err
	}
	feed := &ncaawSeasonFeed{
		finalMeta:   make(map[int64]ncaawFinalMetadata),
		teamConfs:   make(map[int64]int64),
		rankedTeams: make(map[int64]string),
		gameDays:    make(map[string]bool),
	}
	for _, day := range days {
		res, finals, err := u.espn.GetScoreboardDay(ctx, day, espn.D1Basketball)
		if err != nil {
			return nil, fmt.Errorf(
				"ncaaw backfill %d: the advertised feed is unusable on ET day %s: %w; failing closed",
				year, day, err)
		}
		if got := res.Leagues[0].Season.Year; got != year {
			return nil, fmt.Errorf(
				"ncaaw backfill %d: ET day %s answered with season year %d; failing closed", year, day, got)
		}
		// At the known scoreboard cap we cannot distinguish a complete slate
		// from a silently truncated response.
		const scoreboardCap = 200
		if len(res.Events) >= scoreboardCap {
			return nil, fmt.Errorf("ncaaw backfill %d: ET day %s reached the scoreboard cap; failing closed", year, day)
		}
		events := make(map[string]espn.SiteEvent, len(res.Events))
		for _, ev := range res.Events {
			if ev.Season.Year != year {
				return nil, fmt.Errorf(
					"ncaaw backfill %d: event %s on ET day %s carries season year %d; failing closed",
					year, ev.ID, day, ev.Season.Year)
			}
			if len(ev.Competitions) > 0 {
				feed.gameDays[day] = true
			}
			events[ev.ID] = ev
			for _, comp := range ev.Competitions {
				for _, competitor := range comp.Competitors {
					if competitor.Team.ConferenceID != 0 {
						feed.teamConfs[competitor.Team.ID] = int64(competitor.Team.ConferenceID)
					}
				}
			}
		}
		for _, g := range finals {
			if err := feed.addFinal(g, events[strconv.FormatInt(g.ID, 10)]); err != nil {
				return nil, fmt.Errorf("ncaaw backfill %d: %w", year, err)
			}
		}
		u.espn.Throttle(ctx)
	}
	if len(feed.finals) == 0 {
		return nil, fmt.Errorf("ncaaw backfill %d: daily walk returned no completed games; failing closed", year)
	}
	slices.SortFunc(feed.finals, func(a, b espn.Game) int { return cmp.Compare(a.ID, b.ID) })

	// The flat calendar is validated above but is deliberately NOT used as
	// the game-day list, and game days are deliberately not checked against
	// it: the live 2025-26 flat calendar lists 122 dates while the same
	// season's dated walk found 146 game days (verified 2026-11-27 — 24
	// game days with finals, e.g. 38 finals on 2025-11-08, are absent from
	// the advertised calendar), so a calendar-driven walk would silently
	// lose games. The full daily window walk above is the only
	// verified-complete discovery form, exactly like the football backfill.
	return feed, nil
}

func (f *ncaawSeasonFeed) addFinal(g espn.Game, ev espn.SiteEvent) error {
	if _, ok := f.finalMeta[g.ID]; ok {
		return fmt.Errorf("final %d advertised more than once; failing closed", g.ID)
	}
	if len(ev.Competitions) != 1 || len(ev.Competitions[0].Competitors) != 2 {
		return fmt.Errorf("advertised final %d is missing its competition pair; failing closed", g.ID)
	}
	start, err := time.Parse(ncaawFeedDateLayout, ev.Competitions[0].Date)
	if err != nil {
		return fmt.Errorf("advertised final %d has an invalid start time: %w", g.ID, err)
	}
	phase := ev.Season.Type - int64(espn.Regular)
	if phase != 0 && phase != 1 {
		return fmt.Errorf("advertised final %d has unexpected season type %d", g.ID, ev.Season.Type)
	}
	f.finals = append(f.finals, g)
	f.finalMeta[g.ID] = ncaawFinalMetadata{start: start, postseason: phase}
	return nil
}

// ncaawD1Conferences fetches and validates the D-I conference map.
func (u *Updater) ncaawD1Conferences(ctx context.Context) (map[int64]string, error) {
	confResult, err := u.espn.ConferenceMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("ncaaw backfill: fetching the D-I conference map: %w", err)
	}
	d1Confs := confResult.Conferences[espn.D1Basketball]
	if len(d1Confs) == 0 {
		return nil, fmt.Errorf("ncaaw backfill: the D-I conference map is empty; failing closed")
	}
	return d1Confs, nil
}

// ncaawBulkD1Teams fetches the bulk D-I team listing and returns its team-ID
// set. The listing is current-only; it is used solely to refuse unclassified
// likely D-I teams, never to classify historical membership.
func (u *Updater) ncaawBulkD1Teams(ctx context.Context) (map[int64]bool, error) {
	info, err := u.espn.GetTeamInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("ncaaw backfill: fetching the bulk D-I team listing: %w", err)
	}
	if len(info.Sports) == 0 || len(info.Sports[0].Leagues) == 0 || len(info.Sports[0].Leagues[0].Teams) == 0 {
		return nil, fmt.Errorf("ncaaw backfill: the bulk D-I team listing is unusable; failing closed")
	}
	teams := make(map[int64]bool, len(info.Sports[0].Leagues[0].Teams))
	for _, wrap := range info.Sports[0].Leagues[0].Teams {
		teams[wrap.Team.ID] = true
	}
	return teams, nil
}

// ncaawImportSeasonMetadata upserts the year's D-I team seasons from the
// walk's own conference observations (the guessed historical date walk in
// TeamConferencesByYear is deliberately not used) and backfills missing
// identity rows. Teams whose conference ID is missing or unknown to the
// current D-I map get no team_seasons row here; the metadata audit decides
// whether that is a legitimate lower-division opponent or a blocker.
//
// Conference names come from the current-only conference endpoint, so a
// historical season stores today's label for its conference.
func (u *Updater) ncaawImportSeasonMetadata(
	ctx context.Context, year int64, feed *ncaawSeasonFeed, d1Confs map[int64]string,
) error {
	seasons := make([]database.TeamSeason, 0, len(feed.teamConfs))
	for _, teamID := range slices.Sorted(maps.Keys(feed.teamConfs)) {
		confName, ok := d1Confs[feed.teamConfs[teamID]]
		if !ok {
			continue
		}
		feed.rankedTeams[teamID] = confName
		seasons = append(seasons, database.TeamSeason{
			TeamID: teamID,
			Conf:   confName,
			Year:   year,
			Sport:  u.sportDB(),
			FBS:    1, // all D-I basketball teams are top-division
		})
	}
	if len(seasons) == 0 {
		return fmt.Errorf("ncaaw backfill %d: no D-I membership observed on the walk; failing closed", year)
	}
	if err := u.insertSeasonToDB(seasons); err != nil {
		return fmt.Errorf("ncaaw backfill %d: storing team seasons: %w", year, err)
	}
	if _, err := u.backfillMissingTeamNames(ctx, year); err != nil {
		return fmt.Errorf("ncaaw backfill %d: backfilling team identities: %w", year, err)
	}
	return nil
}

// ncaawImportGames runs the walk's finals through the shared
// checkGames/processGames import path: already-stored score-matching games
// are skipped (rerun idempotence), retry-marked rows are re-fetched, and
// summary fetch failures are persisted as retry=1 and reported in the
// result for the audit to fail closed on.
func (u *Updater) ncaawImportGames(ctx context.Context, year int64, feed *ncaawSeasonFeed) (*GameUpdateResult, error) {
	games, err := u.checkGames(feed.finals)
	if err != nil {
		return nil, fmt.Errorf("ncaaw backfill %d: checking stored games: %w", year, err)
	}
	result, err := u.processGames(ctx, games)
	if err != nil {
		return nil, fmt.Errorf("ncaaw backfill %d: importing games: %w", year, err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

// ncaawAuditYear validates one imported season against the walk feed and the
// stored rows. It fails closed — with an error naming the first problem — on
// unresolved summary failures, advertised finals without a matching stored
// event (same ID, requested season, opponents, scores, retry=0, valid
// regular week >0; a missing postseason week is tolerated), stale/orphan
// stored rows, ambiguous duplicate matchups, missing identity metadata for
// D-I teams, and unclassified likely D-I teams. Legitimate lower-division
// opponents are stored but never ranked (they get no team_seasons row, so
// the ranking loader never selects them).
func (u *Updater) ncaawAuditYear(
	year int64,
	feed *ncaawSeasonFeed,
	result *GameUpdateResult,
	bulkD1 map[int64]bool,
) error {
	if len(result.Failed) > 0 {
		return fmt.Errorf("ncaaw backfill %d: %d game summary fetch(es) failed and are marked for retry (%v); failing closed",
			year, len(result.Failed), result.FailedIDs())
	}
	if len(result.Skipped) > 0 {
		u.logger.Warnf("ncaaw backfill %d: %d walk finals were refused by known-event protection: %v",
			year, len(result.Skipped), result.SkippedIDs())
	}

	var stored []database.Game
	if err := u.db.Where("sport = ? and season = ?", u.sportDB(), year).Find(&stored).Error; err != nil {
		return fmt.Errorf("ncaaw backfill %d: reading stored games: %w", year, err)
	}
	storedByID := make(map[int64]database.Game, len(stored))
	for _, row := range stored {
		storedByID[row.GameID] = row
	}

	// 1. Every observed completed final must match its stored event.
	type matchupKey struct {
		day       string
		lowID     int64
		highID    int64
		lowScore  int64
		highScore int64
	}
	matchups := make(map[matchupKey][]int64)
	for _, final := range feed.finals {
		row, ok := storedByID[final.ID]
		if !ok {
			return fmt.Errorf("ncaaw backfill %d: advertised final %d has no stored games row; failing closed",
				year, final.ID)
		}
		if row.Retry != 0 {
			return fmt.Errorf("ncaaw backfill %d: stored game %d is still marked for retry; failing closed",
				year, final.ID)
		}
		if len(final.Competitions) == 0 || len(final.Competitions[0].Competitors) < 2 {
			return fmt.Errorf("ncaaw backfill %d: advertised final %d has no competitor pair; failing closed",
				year, final.ID)
		}
		comp := final.Competitions[0]
		home, away := comp.Competitors[0], comp.Competitors[1]
		if home.HomeAway == scheduleAway {
			home, away = away, home
		}
		opponentsMatch := (row.HomeID == home.ID && row.AwayID == away.ID) ||
			(row.HomeID == away.ID && row.AwayID == home.ID)
		if !opponentsMatch {
			return fmt.Errorf(
				"ncaaw backfill %d: stored game %d opponents (%d, %d) do not match the advertised final (%d, %d); failing closed",
				year, final.ID, row.HomeID, row.AwayID, home.ID, away.ID)
		}
		var expectedHome, expectedAway int64
		if row.HomeID == home.ID {
			expectedHome, expectedAway = home.Score, away.Score
		} else {
			expectedHome, expectedAway = away.Score, home.Score
		}
		if row.HomeScore != expectedHome || row.AwayScore != expectedAway {
			return fmt.Errorf(
				"ncaaw backfill %d: stored game %d scores (%d-%d) do not match the advertised final (%d-%d); failing closed",
				year, final.ID, row.HomeScore, row.AwayScore, expectedHome, expectedAway)
		}
		meta := feed.finalMeta[final.ID]
		if row.Postseason != meta.postseason || !row.StartTime.Equal(meta.start) {
			return fmt.Errorf("ncaaw backfill %d: stored game %d phase or start time differs from the advertised final",
				year, final.ID)
		}
		if row.Postseason == 0 && row.Week <= 0 {
			return fmt.Errorf("ncaaw backfill %d: stored regular game %d has week %d; failing closed",
				year, final.ID, row.Week)
		}

		lowID, highID := row.HomeID, row.AwayID
		lowScore, highScore := row.HomeScore, row.AwayScore
		if lowID > highID {
			lowID, highID = highID, lowID
			lowScore, highScore = highScore, lowScore
		}
		key := matchupKey{day: row.StartTime.In(ncaawETLoc()).Format("2006-01-02"), lowID: lowID, highID: highID,
			lowScore: lowScore, highScore: highScore}
		matchups[key] = append(matchups[key], final.ID)
	}

	// 2. Stale/orphan rows: stored events for the requested season that the
	// feed no longer advertises (ESPN removed or recanted a previously
	// imported final).
	for _, row := range stored {
		if _, advertised := feed.finalMeta[row.GameID]; !advertised {
			return fmt.Errorf(
				"ncaaw backfill %d: stored game %d is not advertised by the feed (stale or orphan row); failing closed",
				year, row.GameID)
		}
	}

	// 3. Ambiguous duplicate matchups: two stored events with the same
	// opponents, scores, and day would double-count in the rankings.
	for _, ids := range matchups {
		if len(ids) > 1 {
			return fmt.Errorf("ncaaw backfill %d: ambiguous duplicate matchup stored under event IDs %v; failing closed",
				year, ids)
		}
	}

	return u.ncaawAuditTeams(year, feed.rankedTeams, bulkD1, stored)
}

func (u *Updater) ncaawAuditTeams(
	year int64, expected map[int64]string, bulkD1 map[int64]bool, games []database.Game,
) error {
	var teams []struct {
		TeamID int64
		FBS    int64
		Conf   string
		Name   *string
	}
	if err := u.db.Model(&database.TeamSeason{}).
		Select("team_seasons.team_id, team_seasons.fbs, team_seasons.conf, team_names.name").
		Joins("left join team_names on team_seasons.team_id = team_names.team_id and team_seasons.sport = team_names.sport").
		Where("team_seasons.sport = ? and team_seasons.year = ?", u.sportDB(), year).
		Order("team_seasons.team_id").Scan(&teams).Error; err != nil {
		return fmt.Errorf("ncaaw backfill %d: reading D-I membership and identities: %w", year, err)
	}
	for _, team := range teams {
		if conf, ok := expected[team.TeamID]; !ok || conf != team.Conf || team.FBS != 1 {
			return fmt.Errorf("ncaaw backfill %d: stored D-I membership for team %d is stale or invalid", year, team.TeamID)
		}
		if team.Name == nil || strings.TrimSpace(*team.Name) == "" {
			return fmt.Errorf("ncaaw backfill %d: D-I team %d has no usable team_names identity row; failing closed",
				year, team.TeamID)
		}
	}
	if len(teams) != len(expected) {
		return fmt.Errorf("ncaaw backfill %d: stored D-I membership has %d of %d expected teams",
			year, len(teams), len(expected))
	}
	for _, game := range games {
		_, homeD1 := expected[game.HomeID]
		_, awayD1 := expected[game.AwayID]
		for _, id := range []int64{game.HomeID, game.AwayID} {
			if _, ok := expected[id]; !ok && bulkD1[id] {
				return fmt.Errorf("ncaaw backfill %d: unclassified team %d is listed as current D-I by ESPN; failing closed",
					year, id)
			}
		}
		if !homeD1 && !awayD1 {
			return fmt.Errorf("ncaaw backfill %d: advertised final %d has no confirmed D-I team", year, game.GameID)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Publication
// ---------------------------------------------------------------------------

// ncaawPublish recomputes ranking rows for the requested verified years
// [publishFrom, toYear] — every explicit regular week via the existing
// regularSeasonWeeks/rankingForWeek primitives plus one Final if postseason
// games exist. Replaces the selected years in one atomic transaction. It only
// runs after every requested season passed its audit, so an upstream, game,
// or metadata audit failure never changes existing ranking rows.
func (u *Updater) ncaawPublish(publishFrom, toYear int64) error {
	var rows []database.TeamWeekResult
	for year := publishFrom; year <= toYear; year++ {
		weeks, err := u.regularSeasonWeeks(year)
		if err != nil {
			return fmt.Errorf("ncaaw backfill %d: resolving regular weeks: %w", year, err)
		}
		if len(weeks) == 0 {
			return fmt.Errorf("ncaaw backfill %d: no regular-season weeks stored; refusing to publish", year)
		}
		for _, week := range weeks {
			weekRows, err := u.rankingForWeek(year, week, nil)
			if err != nil {
				return fmt.Errorf("ncaaw backfill %d: ranking week %d: %w", year, week, err)
			}
			rows = append(rows, weekRows...)
		}
		finalRows, err := u.rankingForWeek(year, 0, nil)
		if err != nil {
			return fmt.Errorf("ncaaw backfill %d: ranking the week-0 slot: %w", year, err)
		}
		if len(finalRows) > 0 && finalRows[0].Postseason == 1 {
			rows = append(rows, finalRows...)
		}
		u.logger.Infof("ncaaw backfill %d: staged rankings for %d regular week(s)", year, len(weeks))
	}
	if len(rows) == 0 {
		return fmt.Errorf("ncaaw backfill: no ranking rows computed for years %d-%d; refusing to publish",
			publishFrom, toYear)
	}

	// Validate the staged rows before publishing anything: ncaaw rows for
	// the requested verified years only, D-I only, valid phases.
	for _, row := range rows {
		if row.Sport != sport.WomensBasketball {
			return fmt.Errorf("ncaaw backfill: staged ranking row for team %d carries sport %q; refusing to publish",
				row.TeamID, row.Sport)
		}
		if row.Year < publishFrom || row.Year > toYear {
			return fmt.Errorf("ncaaw backfill: staged ranking row for team %d carries out-of-range year %d; refusing to publish",
				row.TeamID, row.Year)
		}
		if !row.Fbs {
			return fmt.Errorf("ncaaw backfill: staged ranking row for team %d is not flagged D-I; refusing to publish",
				row.TeamID)
		}
		regular := row.Postseason == 0 && row.Week > 0
		final := row.Postseason == 1 && row.Week == 0
		if !regular && !final {
			return fmt.Errorf("ncaaw backfill: staged ranking row for team %d has invalid week/phase (%d, %d)",
				row.TeamID, row.Week, row.Postseason)
		}
	}

	if err := u.replaceNCAAWRankings(publishFrom, toYear, rows); err != nil {
		return fmt.Errorf("ncaaw backfill: publishing ranking rows: %w", err)
	}

	u.logger.Infof("ncaaw backfill complete: published %d ranking row(s) for years %d-%d; "+
		"years below %d remain lookback-only", len(rows), publishFrom, toYear, publishFrom)
	return nil
}

// replaceNCAAWRankings removes stale weeks, Final variants, and old team rows
// in the same transaction that installs the audited snapshots. A write error
// rolls back both operations; other sports and lookback years are untouched.
func (u *Updater) replaceNCAAWRankings(publishFrom, toYear int64, rows []database.TeamWeekResult) error {
	return u.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("sport = ? and year between ? and ?", sport.WomensBasketball, publishFrom, toYear).
			Delete(&database.TeamWeekResult{}).Error; err != nil {
			return err
		}
		return upsertTeamWeekResults(tx, rows)
	})
}
