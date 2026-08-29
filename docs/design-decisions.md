# Design Decisions

## Ranking Algorithm: Sport-Specific Composite

The final ranking uses a weighted composite score:

```
FinalRaw = (Record * RecordWeight) + (SRS_normalized * SRSWeight) + (SOS_normalized * SOSWeight)
```

- **Win-Loss Record** — Winning more games is the primary signal for football;
  less dominant for basketball where margin matters more.
- **SRS (Simple Rating System)** — An iterative margin-of-victory rating
  adjusted for opponent strength. Run with two margin-of-victory caps and
  averaged. The dual-MOV approach balances "did you win?" with "did you
  dominate?" while preventing blowouts from distorting ratings. Iterates up to
  10,000 times or until convergence.
- **SOS (Strength of Schedule)** — Solved via Cholesky decomposition of a
  system of linear equations. Measures quality of opponents independent of the
  team's own performance.

All component scores are min-max normalized to [0,1] before weighting. Weights
were tuned independently per sport via exhaustive grid search.

### Sport-Specific Constants

The same algorithm is used for both football and basketball, but with different
tuning parameters:

| Parameter | Football | Basketball | Rationale |
|-----------|----------|------------|-----------|
| `RecordWeight` | 0.45 | 0.25 | Football results are more outcome-driven; basketball rewards margin |
| `SRSWeight` | 0.40 | 0.60 | SRS is more predictive in basketball's higher-volume schedule |
| `SOSWeight` | 0.15 | 0.15 | Equal schedule-strength influence across sports |
| `requiredGames` | 12 | 25 | Basketball plays ~30 games/season vs ~12 for football |
| `yearsBack` | 2 | 1 | Basketball has more games, less need for historical backfill |
| MOV caps | [1, 30] | [1, 20] | Basketball has narrower score variance |

## SRS Backfill: The James Madison Problem

When a team transitions divisions (e.g., JMU moving to FBS in 2022), they may
have very few intra-division games early in the season. The SRS algorithm
requires a minimum of `requiredGames` per team. If a team has fewer,
games from previous seasons (`yearsBack`) are included. If still short,
a targeted backfill query fetches additional historical games. This prevents
small sample sizes from distorting the entire rating scale.

See `internal/ranking/rating.go`.

## Multi-Sport via Sport Column (Not Separate Tables)

Football and basketball data share the same tables (`games`, `team_names`,
`team_seasons`, `team_week_results`) differentiated by a `sport` column
(`"ncaaf"` or `"ncaam"`). This avoids duplicating schema definitions and keeps
queries simple — just add `WHERE sport = ?`. The alternative (separate tables
per sport) was rejected because the data models are identical and separate
tables would mean duplicating every query and migration.

The `sport` column defaults to `"ncaaf"` for backward compatibility with existing
football-only data. ESPN uses the same team IDs for a school across sports
(e.g., Alabama's team_id is identical in football and basketball), so
`team_names` requires `(team_id, sport)` as its primary key to store per-sport
metadata without overwriting.

## Per-Client ESPN URLs (No Package-Level Vars)

When supporting multiple sports in a single process (the `schedule` command),
each sport needs its own ESPN API URLs. Each `espn.Client` therefore carries
its own URL set, configured by `NewClientForSport(sport)`. There are no
package-level URL vars; tests point a single client at a mock HTTP server via
`Client.SetURLs`, so parallel clients (one per sport) never interfere.

## Basketball: D1 Only, No Division Split

Unlike football (which has FBS and FCS divisions requiring separate rankings),
basketball only ranks D1 teams as a single group. The `FBS` column in
`team_seasons` is reused to mean "top division" — all D1 basketball teams get
`FBS=1`. The ranking code skips the FCS path for basketball.

## Dual Database Support (PostgreSQL + SQLite)

- **PostgreSQL** is used in production (DigitalOcean managed database).
- **SQLite** is used for local development and the ranker CLI, avoiding the need
  for a running Postgres instance. SQLite connections enable foreign-key
  enforcement via the `_foreign_keys=on` DSN parameter (the PRAGMA is
  per-connection, so a DSN is the only reliable way to set it).
- The choice is implicit: if no `PG_*` environment variable is set at all,
  `DBParams` is nil and SQLite is used. A *partial* Postgres configuration
  (some `PG_*` vars set but required ones missing, or a malformed `PG_PORT`)
  is a configuration error — it never falls back to SQLite silently.
- Both use `SkipDefaultTransaction: true` because all transactions are managed
  explicitly where needed.

## ESPN API as Data Source

ESPN provides unofficial JSON endpoints that return game schedules, box scores,
and team metadata. These are undocumented public APIs used by ESPN's own
frontend. See [espn-api.md](espn-api.md) for endpoint details.

Key design choices:
- **Filter on `STATUS_FINAL` only** — Only completed games are ingested to
  avoid partial data.
- **Centralized rate limiting** — 500ms pause (`espn.Client.RateLimit`) between
  sequential API calls, applied via `espn.SportClient.Throttle()`. Every
  multi-request path (football week loops, basketball date loops, per-game
  fetches) calls it, so no request loop can bypass the shared policy.
- **Up to 5 total attempts (4 retries) with 1s initial exponential backoff**
  on HTTP failures (in `espn/request.go`); no backoff sleep after the final
  attempt.
- **Per-client URLs** — each `espn.Client` carries its own endpoint URL set
  (`NewClientForSport`); tests override them per client via `SetURLs`.

## Failed Game Fetches: Durable Retry via `games.retry`

When a single-game ESPN fetch fails during `processGames`, the failure is not
silently dropped: the game is recorded in `games` with `retry = 1` (upserting
only the flag if the row already exists, otherwise seeding a minimal row from
the schedule entry). `checkGames` always re-queues `retry = 1` games on the
next cycle, and a successful fetch clears the flag via the normal full-row
upsert.

`processGames` returns a structured `GameUpdateResult` (processed game IDs +
per-game failures) so callers — the scheduler, one-shot CLI commands, and
backfills — can report partial results explicitly. Whether partial failure is
tolerable is decided at the caller boundary, not inside the core operation.

## Schema Initialization: One-Shot Compose Service

`docker-compose.yml` runs a `migrate-schema` service (built from the same
image, entrypoint overridden to `cmd/migrate-schema`) that applies the GORM
schema via `database.MigrateSchema` before the updater starts. It is additive
and idempotent — it creates missing tables/columns/indexes and never drops —
so it is safe on every `docker compose up`. A fresh deployment therefore
cannot reach a state where the updater starts against an empty or partial
schema. The updater only starts after this service exits successfully
(`depends_on: service_completed_successfully`).

## Post-Rankings Deploy Hook

After each ranking update, the updater optionally triggers a deploy script
configured via the `DEPLOY_SCRIPT` environment variable. The deployer runs in a
background goroutine with a buffered channel of size 1. Multiple ranking updates
in quick succession coalesce into a single deploy — if one is already queued,
extra triggers are dropped. This prevents deploy storms during rapid back-to-back
ranking runs. If `DEPLOY_SCRIPT` is empty, the hook is a no-op.

On shutdown, the ranking workers are cancelled and joined (via a process-level
context and WaitGroup) *before* the deployer's trigger channel is closed, so a
worker can never send on a closed channel.

## Scheduled Updates During Season

The updater runs on cron schedules scoped to each sport's season months:

**Football (Aug–Jan):**
- **Every 5 minutes:** Check for newly completed games
- **Sundays at 5am:** Refresh team metadata
- **August 10th at 6am:** Initialize the new season

**Basketball (Nov–Apr):**
- **Every 5 minutes:** Check for newly completed games
- **Sundays at 5am:** Refresh team metadata
- **November 1st at 6am:** Initialize the new season

Both sports run in a single process via the `schedule` command, each with its
own update channel and goroutine.

## Panic Recovery in Scheduler

Each scheduled task is wrapped in `defer recover()`. A panic in one update cycle
(e.g., ESPN returning unexpected data) should not crash the long-running
scheduler process. Errors are logged via zap and the next cycle runs normally.
