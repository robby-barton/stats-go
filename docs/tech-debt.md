# Tech Debt Tracker

## Active

### ~~No integration tests~~ (resolved 2026-02-13)
Added integration tests in `internal/updater/` behind a `//go:build integration`
tag. Tests exercise the full pipeline (fetch → parse → store → rank → export)
against an in-memory SQLite database with a mock ESPN HTTP server and a
capturing writer. CI runs integration tests as a separate job.

### ~~ESPN API fragility~~ (resolved 2026-02-13)
Added HTTP status code validation and 5xx retry in `makeRequest`, wrapped JSON
decode errors with endpoint context, added `validate()` methods on all three
response types (`GameScheduleESPN`, `GameInfoESPN`, `TeamInfoESPN`), and guarded
remaining unprotected slice index accesses in `espn.go`.

### ~~Hard-coded rate limiting~~ (resolved 2026-02-13)
Introduced `espn.Client` struct with configurable `MaxRetries`,
`InitialBackoff`, `RequestTimeout`, and `RateLimit` fields. Retry logic now
uses exponential backoff (`initialBackoff * 2^attempt`, capped at 30s).

## Resolved

### Updater CLI flag surface area (resolved 2026-02-13)
Replaced 8 flat boolean flags with cobra subcommands (`schedule`, `games`,
`ranking`, `teams`, `season`, `json`). Each subcommand owns its own flags.

### Home field advantage constant unused (resolved 2026-02-13)
Removed the dead `hfa` constant from `internal/ranking/rating.go`.

### Dependency versions are dated (resolved 2026-02-13)
Upgraded Go 1.21 → 1.26, aws-sdk-go v1 → v2, gocron v1 → v2, GORM
drivers to pgx/v5, and zap to v1.27. All dependencies are now current.
