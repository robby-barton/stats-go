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

### `FBS` column overloaded as "top division" flag

The `fbs` column in `team_seasons` means "FBS" for football but "D1" for
basketball. All D1 basketball teams are stored with `fbs=1`. This works but the
column name is misleading when reading basketball queries. Renaming to something
like `top_division` would require a migration and touching every query that
references it.

See `internal/updater/update_team_season.go:83` and `docs/design-decisions.md`.

### Package-level ESPN URL vars exist only as test fallback

Three package-level `var` declarations (`weekURL`, `gameStatsURL`, `teamInfoURL`)
exist solely so the legacy `NewClient()` constructor — used only in two test
files — can have its URLs overridden via `SetTestURLs()`. Production code uses
`NewClientForSport()` which sets per-client URLs directly. The fallback chain
(`Client.WeekURL()` etc.) adds indirection for no production benefit.

Eliminating `NewClient()` in favor of `NewClientForSport()` everywhere (including
tests) would allow removing the package-level vars, the `SetTestURLs` function,
and the fallback methods.

See `internal/espn/request.go:58-79`.

### `fmt.Println` in ranker CLI for error and duration output

`cmd/ranker/main.go` prints `err` (which may be `nil`) and `duration` via
`fmt.Println` with `//nolint:forbidigo` suppression. Printing a nil error
produces a confusing `<nil>` line. The error should be checked and the duration
should use structured output or be omitted.

## Resolved

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
tag. Tests exercise the full pipeline (fetch → parse → store → rank → export)
against an in-memory SQLite database with a mock ESPN HTTP server and a
capturing writer. CI runs integration tests as a separate job.

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
