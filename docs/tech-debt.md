# Tech Debt Tracker

<!--
Format rules:
- Active items go under ## Active with a ### heading describing the issue.
- When an item is resolved, move it to ## Resolved (at the bottom).
  Add "(resolved YYYY-MM-DD)" to the heading and write a brief note on
  what was done. Do NOT use strikethrough or leave resolved items under Active.
- Keep items in reverse-chronological order within each section (newest first).
-->

## Active

### Basketball season navigation is current-season only

Basketball historical support is now largely implemented:
`BasketballClient.getSeasonDates` synthesizes the full date range for any
historical season (Nov 1 of Y-1 through Apr 10 of Y), so `GetGamesBySeason` and
`TeamConferencesByYear` support historical years. What remains current-season
only are `GetWeeksInSeason` and `HasPostseasonStarted` (guarded by
`validateCurrentSeason`), which are used only by the current-season scheduler.
Extending them would require a year-parameterized ESPN calendar endpoint or a
season-date archive.

### `games` PK missing `sport` — cross-sport collision risk

`games` table uses `game_id` alone as PK. Unlike `team_names`, `team_seasons`,
and `team_week_results` (which all include `sport`), `games` relies on ESPN
using separate ID spaces per sport. If that assumption breaks,
`OnConflict{UpdateAll: true}` silently overwrites data. Fix requires a
multi-step migration touching FK constraints — separate PR.

### ESPN teams endpoint missing some D1 basketball teams

The `/teams?limit=1000` endpoint doesn't return all D1 basketball teams. At
least 3 teams (IDs 2511, 88, 2815) appear in schedule data with D1 conference
IDs but are absent from the teams response. These teams get `team_seasons` rows
but no `team_names` entry, so they show up in rankings with empty names/logos.

May need to increase the limit, paginate, or backfill missing teams from game
data.

### `FBS` column overloaded as "top division" flag

The `fbs` column in `team_seasons` means "FBS" for football but "D1" for
basketball. All D1 basketball teams are stored with `fbs=1`. This works but the
column name is misleading when reading basketball queries. Renaming to something
like `top_division` would require a migration and touching every query that
references it.

See `internal/updater/update_team_season.go:83` and `docs/design-decisions.md`.

## Resolved

### All ESPN fetches migrated to site.api.espn.com (resolved 2026-08-30)

Basketball was the last cdn.espn.com consumer; with its migration every ESPN
fetch runs on site.api.espn.com and the cdn host is no longer contacted at
all (motivated by cdn's intermittent empty-body 202 bot challenges — the
2026-08-29 incident — and observed live again 2026-08-30 while capturing
comparison data: repeated cdn playbyplay requests returned empty-body 202s).
Live-verified basketball facts:

- The basketball scoreboard honors `?dates=YYYYMMDD&groups=50` (single-day
  D1 walk; without `groups` the plain date fetch returned a 21-event subset
  of a 145-game January Saturday, so every basketball request carries
  `groups=50`).
- Basketball scoreboard competitors carry `team.conferenceId` (289 of 290
  competitors on 2026-01-17; the one miss was non-D1 fill-in Bethesda
  Flames), so `TeamConferencesByYear` moved to a per-day scoreboard walk
  alongside the games backfill — the earlier "no site.api source for
  team→conference" premise was wrong.
- The basketball calendar is a flat list of date strings and is present even
  without a `dates` parameter (unlike football); `SiteScoreboardLeague`'s
  SiteCalendar decode now tolerates both shapes (flat list → nil).
- The plain basketball scoreboard's season can already advertise the NEXT
  season before it starts (2026-08-30 capture advertised season 2027), so
  "current season" follows ESPN's flip.
- The basketball summary (site.api) carries every field the game parsers
  consume: header id/season(own year)/week, competition date/conf/neutral,
  competitor ids/scores, and boxscore team statistics. Player-stat category
  `name` is JSON null on basketball payloads (labels carry the columns),
  which is harmless — `parsePlayerStats` is football-only. A direct live
  diff against the cdn playbyplay shape was impossible (cdn 202-challenged
  every request); the football summary equivalence had already been
  verified on its migration.

Deleted with the migration: `GetGamesByDate`, `GetCompletedGamesByDate`, the
cdn `completedGames` helper, `GameScheduleESPN` and its wire-type tree
(Content/Day/Parameters/Calendar/Week/ConferenceAPI/Conference), the cdn
schedule/playbyplay URL plumbing (`WeekURL`, `scheduleURL`,
`SportURLConfig.Schedule`), and the cdn half of `GameInfoESPN`'s dual-shape
unmarshalling (a custom `MarshalJSON` re-emits the summary shape so test
fixtures round-trip). Football notes preserved from the earlier batches:

- Football historical backfill (`GetGamesBySeason`) runs a per-day walk over
  the calendar spans (`scoreboard?dates=YYYYMMDD&groups=N`); live parity
  2026-08-30: the per-day walk found 1698 games for the 2025 season — a
  superset of the cdn data minus one out-of-group D-II matchup, plus 13
  games cdn had missed. The per-day form is deliberate — see hazard 3 below.
- Football `TeamConferencesByYear` walks weekly date ranges per division
  group. Live-verified 2026-08-30: the ncaaf season one-shot produced 266
  team_seasons rows (138 FBS / 128 FCS). Follow-up candidate: the range
  endpoint's lossiness (hazard 3 below) means this walk under-collects a
  few percent of games; harmless for conference extraction (teams appear in
  many games) but it should migrate to the per-day walk for consistency.

Site.api scoreboard `?dates=start-end` range characterization (curl,
2026-08-30, seasons 2021-2025; supersedes the earlier "inconsistent
expansion" watch item — the two observations below fully explain it):

1. **Sunday-start hazard.** Any range that starts on a Sunday returns a
   degenerate subset (0-10 events instead of the 53-89 expected),
   deterministically across all five seasons. Regular-season calendar starts
   are Saturday-anchored, so the weekly walk was never bitten — but the
   postseason calendar starts on a Sunday in 2022, 2024, and 2026, which
   under-collected bowl-only teams for the 2026 postseason. **Mitigated in
   code:** `weekChunks` (`internal/espn/football.go`) shifts any Sunday
   chunk start forward to Monday; unit tests cover the Saturday-anchored,
   Sunday-start, sub-week, and tail cases.
2. **200-event response cap.** Range responses are hard-capped at 200
   events with silent chronological truncation — no error, no indicator.
   Weekly chunks (55-90 games) stay well under the cap; do not widen them
   into multi-week ranges.
3. **Range responses are lossy against single-day fetches (discovered
   2026-08-30 during the football backfill parity check, 2025 season).
   Even far below the 200-event cap, multi-day ranges drop games that the
   single-day fetch for the same date returns: a 7-day range lost the
   tail-day evening games (Friday-night games dated 22:00Z–02:30Z), and
   some ranges silently dropped whole tail days (an Aug30–Sep05 range
   returned nothing dated after Sep02; Aug29–30 returned the 11 Aug29 games
   but none of Aug30's 62). A full range-walk of the 2025 season found only
   1602 of the 1697 cdn-ingested games (~5.6% silently lost). **Single-day
   fetches (`dates=YYYYMMDD`) are the only verified-complete form** — each
   covers exactly one ET calendar day — and are what both `GetGamesBySeason`
   implementations now use. Consequence for any range consumer: compare
   against single-day fetches before trusting completeness.

Also verified during characterization: ranges crossing the
regular/postseason boundary return both season types cleanly, and
single-day fetches are not supersets of ranges (the two forms genuinely
differ — see hazard 3). Degenerate-chunk detection (logging a 0-event
response for a chunk that should contain games) is not implemented because
`espn.Client` carries no logger; the limitation is documented on
`TeamConferencesByYear`.

Also verified live: some football competitors (transition D-II→D-I schools)
carry no `team.conferenceId`.

### Package-level ESPN URL vars exist only as test fallback (resolved 2026-08-30)

Eliminated. All clients are built with `NewClientForSport` (per-client URLs);
tests point a single client at a mock server via `Client.SetURLs`. The
package-level URL vars (`weekURL`, `gameStatsURL`, `teamInfoURL`, and the dead
`scoreboardURL`), `SetTestURLs`, `SetTestScoreboardURL`, the legacy
`NewClient()` constructor, and the URL fallback chain have all been removed.

See `internal/espn/request.go` and `internal/espn/testing_helpers.go`.

### `stats-web` header doesn't fit on mobile viewports (resolved 2026-02-16)

Added a hamburger menu to `stats-web` that collapses nav links behind a toggle
on viewports under 48rem. Removed the `min-width: 45rem` rule that forced a
720px minimum width. Implemented with CSS media queries and a small inline
script — no new React island needed.

### `fmt.Println` in ranker CLI for error and duration output (resolved 2026-02-16)

Changed duration output from `fmt.Println` to `fmt.Fprintf(os.Stderr, ...)` so
it doesn't mix with ranking table output on stdout and no longer needs a
`forbidigo` nolint directive. The nil-error print was already fixed in a prior
change (error is checked and returned before reaching the print).

### Remove JSON export from the updater (resolved 2026-02-16)

Removed the entire JSON export pipeline: `internal/writer/` package,
`update_json.go`, writer field from `Updater` struct, `DOConfig`/`Local` from
config, `json` CLI subcommand, and all related test infrastructure. The
`stats-web` frontend consumes the PostgreSQL database directly at build time.

### Updater tests run in the default suite (resolved 2026-08-30)

The hermetic updater tests (mock ESPN HTTP server + in-memory SQLite) no
longer carry the `integration` build tag, so `go test ./...` runs them. CI's
separate integration job is now redundant with the default test job.

### `team_names` primary key missing `sport` (resolved 2026-02-14)

ESPN uses the same team IDs across sports for the same school. The `team_names`
table had `team_id` as its sole primary key, so `UpdateTeamInfo` for one sport
would overwrite the other sport's row. Fixed by adding `sport` to the
`team_names` primary key and updating the join in `createTeamList`.

### Silent football fallback on unknown sport (resolved 2026-02-14)

Multiple switch statements (`SportDB`, `Groups`, `SportURLs`, `sportConfig`,
`sportFilter`) silently defaulted to football for unrecognized sport values.
Fixed by panicking on unknown sport.

### Unnamed multi-value returns in sport config (resolved 2026-02-14)

`sportConfig()` returned `(int, int64, []int64)` and `SportURLs()` returned
three unnamed strings. Replaced with `sportParams` and `SportURLConfig` structs.

### Schedule command cron boilerplate duplication (resolved 2026-02-14)

The `scheduleCommand` function duplicated ~130 lines of goroutine/channel/cron
registration for each sport. Extracted `sportSchedule.registerJobs` method.

### No integration tests (resolved 2026-02-13)

Added integration tests in `internal/updater/` behind a `//go:build integration`
tag. Tests exercise the full pipeline (fetch → parse → store → rank) against an
in-memory SQLite database with a mock ESPN HTTP server. (Later moved into the
default `go test ./...` suite — see the Resolved entry above.)

### ESPN API fragility (resolved 2026-02-13)

Added HTTP status code validation and 5xx retry in `makeRequest`, wrapped JSON
decode errors with endpoint context, added `validate()` methods on all three
response types (`GameScheduleESPN`, `GameInfoESPN`, `TeamInfoESPN`), and guarded
remaining unprotected slice index accesses in `espn.go`.

### Hard-coded rate limiting (resolved 2026-02-13)

Introduced `espn.Client` struct with configurable `MaxRetries`,
`InitialBackoff`, `RequestTimeout`, and `RateLimit` fields. Retry logic now
uses exponential backoff (`initialBackoff * 2^attempt`, capped at 30s).

### Updater CLI flag surface area (resolved 2026-02-13)

Replaced 8 flat boolean flags with cobra subcommands (`schedule`, `games`,
`ranking`, `teams`, `season`, `json`). Each subcommand owns its own flags.

### Home field advantage constant unused (resolved 2026-02-13)

Removed the dead `hfa` constant from `internal/ranking/rating.go`.

### Dependency versions are dated (resolved 2026-02-13)

Upgraded Go 1.21 → 1.26, aws-sdk-go v1 → v2, gocron v1 → v2, GORM
drivers to pgx/v5, and zap to v1.27. All dependencies are now current.
