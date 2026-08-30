# Architecture

## System Overview

stats-go is a set of Go services for ranking college sports teams (football and
basketball). Data flows in one direction: ESPN API -> parsing -> database ->
ranking algorithm -> database. The `stats-web` site consumes the rankings by
querying the database directly at build time; there is no HTTP API in this
repo.

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
          ┌────────────────┼────────────┐
          │                │            │
   ┌──────▼──────┐  ┌─────▼──────┐     │
   │  internal/   │  │ internal/  │     │
   │  ranking     │  │ updater    │     │
   │  (algorithm) │  │ (orchestr) │     │
   └──────────────┘  └────────────┘     │
          │                │            │
          │                │            │
   ┌──────┴─────────────────────────────┘
   │              cmd/ entry points
   │  ranker     updater     migrate
   └────────────────────────────────────┘
```

## Multi-Sport Support

The system supports college football and basketball through a `Sport` type in the
ESPN package (`espn.CollegeFootball`, `espn.CollegeBasketball`). Each sport has:

- **ESPN client configuration:** Different API URLs, group IDs, season types
- **Database separation:** Shared tables use a `sport` column (`"ncaaf"` or `"ncaam"`)
- **Ranking constants:** Sport-dependent `requiredGames`, `yearsBack`, and MOV caps
- **Division structure:** Football has FBS/FCS; basketball has D1 only

The `Updater` and `Ranker` structs each carry a typed sport identifier
(`sport.Sport`, converted to the ESPN slug via `espn.Sport`). The CLI exposes
sport subcommands (`ncaaf`, `ncaam`). The `schedule` command runs both sports
in a single process with sport-appropriate cron schedules.

## Package Dependency Rules

Dependencies flow **downward only**. Packages must not import from peers at the
same level or from `cmd/`.

```
cmd/ranker   → config, database, ranking, ranking/load, sport
cmd/updater  → config, database, logger, updater, espn
cmd/migrate  → database

updater      → database, espn, game, ranking, ranking/load, sport, team
game         → espn
team         → espn
ranking      → sport (no database or espn imports)
ranking/load → database, ranking, sport
espn         → sport (URLs/slugs only; persistence IDs via DBSport)
config       → (external: godotenv)
logger       → (external: zap)
database     → gorm, sport
sport        → (leaf: no internal dependencies)
```

The `internal/sport` package owns the persistence identifiers
(`sport.Football` = `"ncaaf"`, `sport.Basketball` = `"ncaam"`) shared by the
database models, ranking, updater, and CLI wiring. ESPN-specific slugs and
URLs stay inside `espn`, which converts to the persistence ID at its edge via
`Sport.DBSport()`.

## Key Abstractions

### Updater Struct

Central orchestrator that ties together all internal packages:

```go
u, err := updater.NewUpdater(db, log, espn.NewClientForSport(sport))
```

`NewUpdater` validates its inputs (nil DB, logger, or ESPN client, and unknown
sports return errors — mirroring `ranking.NewRanker`); the Updater's fields are
unexported so all wiring goes through the constructor.

Responsible for: fetching games, updating the DB, and computing rankings.
Used by `cmd/updater` in both scheduled and on-demand modes.
Each sport gets its own `Updater` instance with a sport-specific ESPN client.
The sport is derived from the ESPN client's `SportInfo()` method.

### Ranker Struct

```go
type Ranker struct {
    in Input  // Year, Week, Sport (sport.Sport), Teams, Games, Backfill
}
```

Constructed via `NewRanker(Input)`, which validates the sport (unknown sports
return an error instead of panicking mid-computation). Teams, games, and the
resolved ranking window are loaded once at the persistence edge —
`internal/ranking/load.Input` queries the database for them up front — so the
ranking pipeline is GORM-free and computes entirely on plain values:
`setup → record → srs → sos → finalRanking`.

One part of the input is deliberately not loaded eagerly: pre-window game
backfill (the "James Madison problem" search in `srs`). Eagerly resolving it
for every team would over-fetch, since only teams short on division games
need it. Instead, the loader injects a `ranking.BackfillFunc` — a query seam
implemented in `internal/ranking/load` — and the SRS stage invokes it on
demand for those teams. The seam keeps the pipeline GORM-free: the ranking
package sees a plain function returning games, never a database handle.
Sport-dependent constants
(required games, years of history, MOV caps) are selected via
`sportConfig()`. Algorithm tests construct `Input` values directly; the
load package's DB-backed window resolution is tested in
`internal/ranking/load`.

### ESPN Client

HTTP client backed by the `espn.Client` struct, which holds retry and
rate-limit configuration (`MaxAttempts`, `InitialBackoff`, `RequestTimeout`,
`RateLimit`). `MaxAttempts` is the total number of request tries; retries use
exponential backoff capped at 30s, and no backoff sleep happens after the
final failed attempt.

Every ESPN request takes a `context.Context` so callers can cancel in-flight
work (the scheduler threads the process context through updater operations;
one-shot CLI commands use `context.Background()`). Every multi-request path
(season week loops, basketball date loops, per-game fetches in `game/` and
`updater/`) calls `SportClient.Throttle(ctx)` after each sequential request,
so the full API surface shares one rate-limit policy.

Each client is bound to a sport via `NewClientForSport(sport)`, which sets
per-client URLs for that sport's ESPN endpoints. Every client carries its own
URL set (no package-level vars); tests point a single client at a mock HTTP
server via `Client.SetURLs`.

Endpoint ownership by host (since 2026-09): cdn.espn.com has been
intermittently serving empty-body 202 bot challenges to automated clients, so
the current-week games fetch, the football box-score fetch, the football
season metadata (`DefaultSeason`, `GetWeeksInSeason`, `HasPostseasonStarted`
via the scoreboard's object-shaped calendar), `ConferenceMap` (both
sports, via the `scoreboard/conferences` endpoint), and the football season
backfill (`GetGamesBySeason` — a per-day walk over the scoreboard calendar
spans) have all moved to site.api.espn.com. cdn.espn.com still serves the
basketball schedule and box-score fetches plus basketball
`TeamConferencesByYear` — no site.api endpoint provides a bulk team→
conference mapping (teams rows lack `conferenceId`; scoreboard responses
cap events and, for date ranges, are lossy against single-day fetches) —
plus all remaining basketball schedule and box-score fetches. See
docs/tech-debt.md for the remaining cdn dependencies.

## Database

14 GORM models covering teams, games, and player statistics. Supports both
PostgreSQL (production) and SQLite (local development). Connection is determined
by whether `DBParams` is nil (nil → SQLite).

All models use composite primary keys for multi-dimensional lookups
(team+year, game+team, etc.). Shared tables (`games`, `team_names`,
`team_seasons`, `team_week_results`) include a `sport` column with a default of
`"ncaaf"`. The `team_names`, `team_seasons`, and `team_week_results` primary keys
include `sport`. ESPN uses the same team IDs across sports for the same school,
so `team_names` requires `(team_id, sport)` to store per-sport team metadata.

## Deployment

- **Docker:** Multi-stage build (`golang:1.26-alpine` → `alpine:latest`)
- **Schema migrations:** A one-shot `migrate-schema` Compose service applies
  the GORM schema (additive, idempotent) and must complete successfully before
  the `updater` service starts (`depends_on:
  service_completed_successfully`)
- **Production:** `updater schedule` running in a container alongside PostgreSQL
- **CI/CD:** GitHub Actions — lint and test on PR, build+push on merge to master
