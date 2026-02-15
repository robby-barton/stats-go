# Architecture

## System Overview

stats-go is a set of Go services for ranking college sports teams (football and
basketball). Data flows in one direction: ESPN API -> parsing -> database ->
ranking algorithm -> output.

```
┌─────────────────────────────────────────────────────────┐
│                      ESPN HTTP API                       │
└──────────────────────────┬──────────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │     internal/espn       │  Pure HTTP client
              │  (schedules, stats,     │  No DB dependency
              │   team info)            │  Sport-parameterized
              └────────────┬────────────┘
                           │
              ┌────────────▼────────────┐
              │     internal/game       │  Parses ESPN responses
              │     internal/team       │  into DB model structs
              └────────────┬────────────┘
                           │
              ┌────────────▼────────────┐
              │   internal/database     │  GORM models (14 tables)
              │  (Postgres or SQLite)   │  Sport column on shared tables
              └────────────┬────────────┘
                           │
          ┌────────────────┼────────────────┐
          │                │                │
   ┌──────▼──────┐  ┌─────▼──────┐  ┌──────▼──────┐
   │  internal/   │  │ internal/  │  │  internal/  │
   │  ranking     │  │ updater    │  │  writer     │
   │  (algorithm) │  │ (orchestr) │  │ (interface) │
   └──────────────┘  └────────────┘  └─────────────┘
          │                │                │
          │         ┌──────┴──────┐         │
          │         │  Uses both  │─────────┘
          │         └─────────────┘
          │
   ┌──────┴──────────────────────────────────┐
   │              cmd/ entry points           │
   │  ranker     updater     migrate          │
   └──────────────────────────────────────────┘
```

## Multi-Sport Support

The system supports college football and basketball through a `Sport` type in the
ESPN package (`espn.CollegeFootball`, `espn.CollegeBasketball`). Each sport has:

- **ESPN client configuration:** Different API URLs, group IDs, season types
- **Database separation:** Shared tables use a `sport` column (`"cfb"` or `"cbb"`)
- **Ranking constants:** Sport-dependent `requiredGames`, `yearsBack`, and MOV caps
- **Division structure:** Football has FBS/FCS; basketball has D1 only
- **JSON output paths:** Sport-prefixed (`cfb/ranking/...`, `cbb/ranking/...`)

The `Updater` and `Ranker` structs each carry a sport identifier. The CLI
exposes sport subcommands (`football`, `basketball`). The `schedule` command runs
both sports in a single process with sport-appropriate cron schedules.

## Package Dependency Rules

Dependencies flow **downward only**. Packages must not import from peers at the
same level or from `cmd/`.

```
cmd/ranker   → config, database, ranking
cmd/updater  → config, database, logger, updater, writer, espn
cmd/migrate  → database

updater      → database, espn, game, ranking, team, writer
game         → database, espn
team         → espn
ranking      → database
writer       → (external: AWS SDK, DO API)
espn         → (external: net/http only)
config       → (external: godotenv)
logger       → (external: zap)
database     → (external: gorm)
```

## Key Abstractions

### Writer Interface

```go
type Writer interface {
    WriteData(ctx context.Context, fileName string, data any) error
    PurgeCache(ctx context.Context) error
}
```

Two implementations:
- **DigitalOceanWriter** — uploads gzipped JSON to DO Spaces, purges CDN cache
- **DefaultWriter** — writes JSON to local filesystem

### Updater Struct

Central orchestrator that ties together all internal packages:

```go
type Updater struct {
    DB     *gorm.DB
    Logger *zap.SugaredLogger
    Writer writer.Writer
    ESPN   *espn.Client
    Sport  espn.Sport
}
```

Responsible for: fetching games, updating the DB, computing rankings, and
exporting JSON. Used by `cmd/updater` in both scheduled and on-demand modes.
Each sport gets its own `Updater` instance with a sport-specific ESPN client.

### Ranker Struct

```go
type Ranker struct {
    DB    *gorm.DB
    Year  int64
    Week  int64
    Fcs   bool
    Sport string  // "cfb" or "cbb"
}
```

Executes the ranking pipeline: `setup → record → srs → sos → finalRanking`.
All computation happens in-memory after initial DB queries. Sport-dependent
constants (required games, years of history, MOV caps) are selected via
`sportConfig()`.

### ESPN Client

HTTP client backed by the `espn.Client` struct, which holds retry and
rate-limit configuration (`MaxRetries`, `InitialBackoff`, `RequestTimeout`,
`RateLimit`). Retries use exponential backoff capped at 30s.

Each client is bound to a sport via `NewClientForSport(sport)`, which sets
per-client URLs for that sport's ESPN endpoints. The `NewClient()` constructor
defaults to football and uses package-level URL vars (overridable in tests via
`SetTestURLs`).

## Database

14 GORM models covering teams, games, and player statistics. Supports both
PostgreSQL (production) and SQLite (local development). Connection is determined
by whether `DBParams` is nil (nil → SQLite).

All models use composite primary keys for multi-dimensional lookups
(team+year, game+team, etc.). Shared tables (`games`, `team_names`,
`team_seasons`, `team_week_results`) include a `sport` column with a default of
`"cfb"`. The `team_names`, `team_seasons`, and `team_week_results` primary keys
include `sport`. ESPN uses the same team IDs across sports for the same school,
so `team_names` requires `(team_id, sport)` to store per-sport team metadata.

## Deployment

- **Docker:** Multi-stage build (`golang:1.26-alpine` → `alpine:latest`)
- **Production:** `updater schedule` running in a container alongside PostgreSQL
- **CI/CD:** GitHub Actions — lint and test on PR, build+push on merge to master
- **Output:** JSON files served from DigitalOcean Spaces CDN
