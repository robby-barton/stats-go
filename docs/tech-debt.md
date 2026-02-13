# Tech Debt Tracker

## Active

### No integration tests
Unit tests cover ESPN parsing, ranking math, and record formatting. There are no
integration tests that exercise the full pipeline (fetch → parse → store →
rank → export) against a real or in-memory database.

### ESPN API fragility
The ESPN endpoints are undocumented and could change at any time. There is no
contract validation or schema checking on API responses — if ESPN changes a
field name or nests data differently, it will fail at JSON decode time with a
potentially unclear error.

### Updater CLI flag surface area
`cmd/updater/main.go` has grown to 8 boolean flags and a mix of scheduling and
one-shot modes in a single `main()`. This could be clearer with subcommands
(e.g., `updater schedule`, `updater games --all`).

### Hard-coded rate limiting
The 200ms sleep in `game/` and 1s retry backoff in `espn/request.go` are
hard-coded. These could be configurable or use exponential backoff.

### Home field advantage constant unused in final ranking
`rating.go` defines `hfa = 3` (home field advantage) but it is not currently
used in any calculation. Unclear if this is intentional or vestigial.

## Resolved

### Dependency versions are dated (resolved 2026-02-13)
Upgraded Go 1.21 → 1.26, aws-sdk-go v1 → v2, gocron v1 → v2, GORM
drivers to pgx/v5, and zap to v1.27. All dependencies are now current.
