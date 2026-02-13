# Design Decisions

## Ranking Algorithm: 60/30/10 Composite

The final ranking uses a weighted composite score:

```
FinalRaw = (Record * 0.60) + (SRS_normalized * 0.30) + (SOS_normalized * 0.10)
```

- **60% Win-Loss Record** — The most important factor. Teams that win more
  games should rank higher.
- **30% SRS (Simple Rating System)** — An iterative margin-of-victory rating
  adjusted for opponent strength. Run with two margin-of-victory caps (1 and 30)
  and averaged. The dual-MOV approach balances "did you win?" (MOV=1) with
  "did you dominate?" (MOV=30) while preventing blowouts from distorting ratings.
  Iterates up to 10,000 times or until convergence.
- **10% SOS (Strength of Schedule)** — Solved via Cholesky decomposition of a
  system of linear equations. Measures quality of opponents independent of the
  team's own performance.

All component scores are min-max normalized to [0,1] before weighting.

## SRS Backfill: The James Madison Problem

When a team transitions divisions (e.g., JMU moving to FBS in 2022), they may
have very few intra-division games early in the season. The SRS algorithm
requires a minimum of 12 games (`requiredGames`) per team. If a team has fewer,
games from up to 2 previous seasons (`yearsBack`) are included. If still short,
a targeted backfill query fetches additional historical games. This prevents
small sample sizes from distorting the entire rating scale.

See `internal/ranking/rating.go:199-228`.

## Dual Database Support (PostgreSQL + SQLite)

- **PostgreSQL** is used in production (DigitalOcean managed database).
- **SQLite** is used for local development and the ranker CLI, avoiding the need
  for a running Postgres instance.
- The choice is implicit: if `DBParams` is nil (no PG env vars), SQLite is used.
- Both use `SkipDefaultTransaction: true` because all transactions are managed
  explicitly where needed.

## ESPN API as Data Source

ESPN provides unofficial JSON endpoints that return game schedules, box scores,
and team metadata. These are undocumented public APIs used by ESPN's own
frontend. See [espn-api.md](espn-api.md) for endpoint details.

Key design choices:
- **Filter on `STATUS_FINAL` only** — Only completed games are ingested to
  avoid partial data.
- **200ms rate limiting** between sequential API calls (in `game/` package) to
  avoid being blocked.
- **5 retries with 1s backoff** on HTTP failures (in `espn/request.go`).
- **URL vars instead of constants** — ESPN endpoint URLs are `var` not `const`
  so tests can override them with a mock HTTP server.

## Writer Interface for Output

Rather than hardcoding DigitalOcean Spaces, output goes through a `Writer`
interface. This allows:
- Production: gzipped JSON uploaded to DO Spaces with CDN cache purging
- Local dev: plain JSON files written to disk
- Testing: mock writers

## Scheduled Updates During Football Season

The updater runs on cron schedules scoped to football season months (Aug-Jan):
- **Every 5 minutes:** Check for newly completed games
- **Sundays at 5am:** Refresh team metadata
- **August 10th at 6am:** Initialize the new season

This avoids wasting resources during the offseason while ensuring near-real-time
updates during games.

## Panic Recovery in Scheduler

Each scheduled task is wrapped in `defer recover()`. A panic in one update cycle
(e.g., ESPN returning unexpected data) should not crash the long-running
scheduler process. Errors are logged via zap and the next cycle runs normally.
