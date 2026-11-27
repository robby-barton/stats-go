# Architecture

## System Overview

stats-go is a set of Go services for ranking college sports teams (football,
men's basketball, and women's basketball). Data flows in one direction: ESPN API -> parsing -> database ->
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

The system supports college football and basketball (men's and women's)
through a `Sport` type in the ESPN package (`espn.CollegeFootball`,
`espn.CollegeBasketball`, `espn.CollegeWomensBasketball`). Each sport has:

- **ESPN client configuration:** Different API URLs, group IDs, season types
  — the two basketball sports share one client behavior and one division
  group (D1, group 50) on dedicated URL paths
- **Database separation:** Shared tables use a `sport` column (`"ncaaf"`,
  `"ncaam"`, `"ncaaw"`)
- **Ranking constants:** Sport-dependent `requiredGames`, `yearsBack`, and MOV
  caps — women's basketball reuses the men's basketball parameters unchanged
- **Division structure:** Football has FBS/FCS; both basketball sports are
  D-I only, with no FCS ranking path and no division filter

The `Updater` and `Ranker` structs each carry a typed sport identifier
(`sport.Sport`, converted to the ESPN slug via `espn.Sport`). The CLI exposes
sport subcommands (`ncaaf`, `ncaam`, `ncaaw`). The `schedule` command runs
football and men's basketball in a single process with sport-appropriate cron
schedules; women's basketball is one-shot only until its scheduled jobs land.

## Package Dependency Rules

Dependencies flow **downward only**. Packages must not import from peers at the
same level or from `cmd/`.

```
cmd/ranker   → config, database, ranking, ranking/load, sport
cmd/updater  → config, database, logger, updater, espn
cmd/migrate  → database
cmd/ncaaf-replay → config, database, ncaafreplay, ranking/load

ncaafreplay  → database, ranking, ranking/load, sport
updater      → database, espn, game, ranking, ranking/load, sport, team, updater/fbsprior
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
(`sport.Football` = `"ncaaf"`, `sport.Basketball` = `"ncaam"`,
`sport.WomensBasketball` = `"ncaaw"`) shared by the database models, ranking,
updater, and CLI wiring; `Sport.IsBasketball()` covers both basketball sports
so the single-division ranking and season paths share one branch. ESPN-specific slugs and
URLs stay inside `espn`, which converts to the persistence ID at its edge via
`Sport.DBSport()`.

## Key Abstractions

### Offline NCAAF Replay

`cmd/ncaaf-replay` is a developer tool, not an updater dependency. It
compares the normal unblended FBS baseline with bounded explicit-cutoff
inputs. Both read paths share ranking predicates, but the replay loads its
own data and snapshots older history separately. Capture runs in a verified
read-only PostgreSQL transaction; the replay is evaluated after rollback.
See [the replay guide](docs/ncaaf-readonly-replay.md) for scope and fixtures.

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

### FBS Early-Season Prior (opt-in, default off)

`internal/updater/fbsprior` holds the `ncaaf-season-prior/1` automatic-freeze
artifact contract plus the `ncaaf-season-prior/2` rebuild artifact (Build/
Parse with an exact-byte checksum, the fixed August 10
midnight Eastern anchor, and the derived day-56 expiry) and the researched
blend math (rho 0.9, 4-game expiry including FBS-FCS games, SD floor 0.05).
With the sole `--fbs-early-season-prior` flag, the updater itself computes
and persists the prior: the freeze task in `internal/updater` (triggered
after annual season initialization, by a daily August 10–16 Eastern retry
job, and at startup) loads a bounded two-season donor input, refuses
incomplete or unusable data, and inserts one immutable
`database.FBSSeasonPrior` row per season with conflict-do-nothing inside a
transaction serialized against game writes (table locks on PostgreSQL,
immediate write lock on SQLite). The scheduled football ranking worker
(`UpdateRecentRankingsWithPrior`) resolves the latest regular-season FBS
window and reads the same-season row on eligible runs before expiry;
missing or invalid rows fall back to the exact baseline with a warning.
The same flag also opts the one-shot football ranking command into the same
policy: `updater ncaaf ranking --all --fbs-early-season-prior`
(`UpdateAllRankingsWithPrior`) rebuilds every ranking window through the
same baseline loader and score calculation and reconstructs one prior
freshly in memory per season in scope (completed previous seasons and the
current Eastern season, the latter even before its first game when its
metadata exists) with exactly the scheduled freeze method — same anchor,
same bounded two-donor-season input, same strength math — inside one
consistent snapshot transaction, with a defensive recheck that no
target-season or future game leaks into the donor window. Stored prior
values are never used as inputs. As the authoritative explicit rerun, the
rebuild then publishes the rebuilt priors atomically with its ranking
upserts: seasons whose anchor is already reached are replaced (existing
current-season rows included, midseason allowed) with `ncaaf-season-prior/2`
artifacts recording the real rebuild instant and source, and a season whose
reconstruction fails has its stale persisted prior invalidated (that
season's row only); any prior or ranking write failure rolls the whole
publication back. The publication write phase and the opted-in scheduled
read/calc/write share one publication-serialized transaction (a PostgreSQL
advisory xact lock; the SQLite write lock acquired up front), so a scheduled
run can never read the old prior and commit after a corrected publication;
the rebuild holds the lock only over its writes, and nothing takes it with
the feature off. The rebuild never calls ESPN/AP and never fabricates
persisted capture metadata (automatic `ncaaf-season-prior/1` artifacts stay
readable). Its current/latest week-0 slot uses the actual observation clock;
explicit historical weeks and past-season final snapshots use their resolved
historical ranking cutoffs, so historical weeks are recomputed at their
cutoffs rather than replayed at the exact times the scheduler originally
published them, and the numbers can differ from any hypothetical historical
snapshot when data has been revised. Everything else
(basketball, one-shot commands without the flag, plain `UpdateAllRankings`,
historical backfills) stays exact baseline, and with the flag off the scheduler neither registers
a freeze job nor reads or writes stored prior rows. The blend only substitutes
`FinalRaw`/`FinalRank` on the staged FBS rows after success. See
docs/fbs-early-season-prior.md for the policy and one-time
enable/disable.

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

Endpoint ownership by host (since 2026-08-30): cdn.espn.com had been
intermittently serving empty-body 202 bot challenges to automated clients, so
ALL ESPN fetches now run on site.api.espn.com: the current-week games fetch
(scoreboard with the `groups` param, one request per ET day for basketball's
today+yesterday window), both sports' box-score fetch (summary), the season
metadata (`DefaultSeason`, `GetWeeksInSeason`, `HasPostseasonStarted`),
`ConferenceMap` (via the `scoreboard/conferences` endpoint), the season
backfills (`GetGamesBySeason` — per-day walks over the calendar spans for
football and the game-date list for basketball), and both sports'
`TeamConferencesByYear` (basketball per-day scoreboard walk; basketball
scoreboard competitors carry `team.conferenceId` like football's — verified
live 2026-08-30). The cdn.espn.com schedule/playbyplay endpoints and their
wire types (`GameScheduleESPN` and friends) were deleted. See
docs/tech-debt.md for the migration history.

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
