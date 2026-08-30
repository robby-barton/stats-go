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

### Historical week/date schedule fetches and TeamConferencesByYear still on cdn.espn.com

The site.api migration (2026-09) moved the current-week games fetch
(`GetCurrentWeekGames` → scoreboard with the `groups` param), the football
box-score fetch (`GetGameStats` → summary), the football season metadata
(`DefaultSeason` / `GetWeeksInSeason` / `HasPostseasonStarted` → scoreboard
with the `dates` param) and `ConferenceMap` (both sports →
`scoreboard/conferences?groups=N`; the plain endpoint returns FBS only, FCS
needs `groups=81`) to site.api.espn.com. Still on cdn.espn.com, which remains
prone to empty-body 202 bot challenges:

- `GetGamesByWeek` / `GetCompletedGamesByWeek` / `GetGamesBySeason`
  (historical week fetching). The site.api scoreboard cannot serve this: its
  responses cap events (~25) and only expand a single date per `dates`
  request, so a week would need 4+ requests instead of one.
- `TeamConferencesByYear` (both sports — team→conference extraction over all
  weeks/dates of a season). No site.api endpoint covers this in bulk: the
  `/teams?limit=1000` rows carry no `conferenceId`, and the scoreboard event
  caps apply here too. `sports.core.api` was probed (2026-08-29) and its
  season/teams list endpoints return application errors, so the cdn schedule
  remains the only known source. The move is not trivially shared either:
  `GetGamesByWeek` returns `*GameScheduleESPN` whose calendar/conferenceAPI
  blocks the scoreboard doesn't provide.
- All basketball schedule and box-score fetches (date-based schedule,
  playbyplay box scores).

Also verified live 2026-08-29: the site.api scoreboard omits
`leagues[0].calendar` entirely unless a `dates` query parameter is passed
(even mid-season), which is why basketball current-season date navigation now
uses the dates-parameterized scoreboard (`GetSeasonDatesForYear`). Some
football competitors (transition D-II→D-I schools) carry no
`team.conferenceId`.

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
