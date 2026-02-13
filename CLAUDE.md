# stats-go

College football computer ranking system written in Go. Collects game data from
ESPN, computes rankings using a composite algorithm, and exports results to
DigitalOcean Spaces.

## Quick Reference

- **Language:** Go 1.26
- **Module:** `github.com/robby-barton/stats-go`
- **Build/Run:** `make ranker`, `make updater`
- **Test:** `go test ./...`
- **Lint:** `golangci-lint run --config=.golangci.yml ./cmd/... ./internal/...`
- **Format:** `go fmt ./...` (goimports enforced via golangci-lint)

## Repository Layout

```
cmd/
  ranker/     CLI tool to calculate and print rankings
  updater/    Service that fetches games and updates DB/JSON on a schedule
  migrate/    One-time migration from PostgreSQL to SQLite
internal/
  config/     Environment-based configuration (godotenv)
  database/   GORM models and DB initialization (Postgres + SQLite)
  espn/       ESPN API client (game schedules, stats, team info)
  game/       Game data parsing and stat extraction
  logger/     Structured logging (zap)
  ranking/    Ranking algorithm (SRS, SOS, composite scoring)
  team/       Team info parsing from ESPN
  updater/    Orchestration of DB updates and JSON export
  writer/     Output interface with DigitalOcean and local implementations
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full dependency graph and design
rationale.

Key patterns:
- **Three independent CLI entry points** in `cmd/` — each wires its own deps
- **Writer interface** (`internal/writer`) — pluggable output (DO Spaces vs local files)
- **Updater struct** receives DB, Logger, and Writer via dependency injection
- **ESPN package** is a pure HTTP client with no DB dependency
- **Ranking package** takes a `*gorm.DB` and computes everything in-memory

## Conventions

- All database access goes through GORM. Transactions are manually managed
  (`SkipDefaultTransaction: true`).
- Use `go.uber.org/zap` SugaredLogger for all logging. No `fmt.Println` outside
  of the ranker CLI output (enforced via `forbidigo` lint).
- Errors are propagated up — panics are only recovered at the scheduler level
  in `cmd/updater`.
- ESPN API calls use a 200ms sleep between requests for rate limiting (in `game/`).
- The HTTP client in `espn/request.go` retries up to 5 times with 1s backoff.
- Test files use `_test.go` suffix. ESPN tests use a mock HTTP server pattern
  with fixture data.
- `nolint` directives require both a specific linter and an explanation
  (`require-explanation: true`, `require-specific: true`).

## Golden Rule: Keep Documentation Up to Date

Any change that alters architecture, package dependencies, public interfaces,
ESPN API usage, conventions, or design rationale **must** include corresponding
updates to the relevant docs (`CLAUDE.md`, `ARCHITECTURE.md`, or files in
`docs/`). Documentation that contradicts the code is worse than no documentation
at all — it actively misleads future work.

When in doubt, update the docs. When adding a new package or changing how an
existing one works, update `ARCHITECTURE.md`. When making a deliberate tradeoff,
record it in `docs/design-decisions.md`. When discovering or resolving tech
debt, update `docs/tech-debt.md`.

## Deeper Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — package dependencies and layering
- [docs/design-decisions.md](docs/design-decisions.md) — rationale for key choices
- [docs/espn-api.md](docs/espn-api.md) — ESPN endpoints and data shapes
- [docs/tech-debt.md](docs/tech-debt.md) — known issues and improvement areas
