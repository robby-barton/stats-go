# Architecture

## System Overview

stats-go is a set of Go services for ranking college football teams. Data flows
in one direction: ESPN API -> parsing -> database -> ranking algorithm -> output.

```
┌─────────────────────────────────────────────────────────┐
│                      ESPN HTTP API                       │
└──────────────────────────┬──────────────────────────────┘
                           │
              ┌────────────▼────────────┐
              │     internal/espn       │  Pure HTTP client
              │  (schedules, stats,     │  No DB dependency
              │   team info)            │
              └────────────┬────────────┘
                           │
              ┌────────────▼────────────┐
              │     internal/game       │  Parses ESPN responses
              │     internal/team       │  into DB model structs
              └────────────┬────────────┘
                           │
              ┌────────────▼────────────┐
              │   internal/database     │  GORM models (14 tables)
              │  (Postgres or SQLite)   │  Shared by all services
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

## Package Dependency Rules

Dependencies flow **downward only**. Packages must not import from peers at the
same level or from `cmd/`.

```
cmd/ranker   → config, database, ranking
cmd/updater  → config, database, logger, updater, writer
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
}
```

Responsible for: fetching games, updating the DB, computing rankings, and
exporting JSON. Used by `cmd/updater` in both scheduled and on-demand modes.

### Ranker Struct

```go
type Ranker struct {
    DB   *gorm.DB
    Year int64
    Week int64
    Fcs  bool
}
```

Executes the ranking pipeline: `setup → record → srs → sos → finalRanking`.
All computation happens in-memory after initial DB queries.

### ESPN Client

Stateless HTTP client. All functions are package-level (no struct). URLs are
package-level vars so tests can override them with a mock HTTP server.

## Database

14 GORM models covering teams, games, and player statistics. Supports both
PostgreSQL (production) and SQLite (local development). Connection is determined
by whether `DBParams` is nil (nil → SQLite).

All models use composite primary keys for multi-dimensional lookups
(team+year, game+team, etc.).

## Deployment

- **Docker:** Multi-stage build (`golang:1.21-alpine` → `alpine:latest`)
- **Production:** `updater -schedule` running in a container alongside PostgreSQL
- **CI/CD:** GitHub Actions — lint and test on PR, build+push on merge to master
- **Output:** JSON files served from DigitalOcean Spaces CDN
