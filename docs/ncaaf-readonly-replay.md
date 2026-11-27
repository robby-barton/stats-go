# NCAAF read-only replay and parity tool

A narrow developer tool that checks one thing: for explicitly listed
regular-season windows, the production ranking baseline and an independently
loaded explicit-cutoff replay produce identical rankings.

## What it does

For each window in a manifest (`year`, `week`, optional RFC3339 `cutoff`
assertion):

1. **Reference (production path).** `load.InputWindow` resolves the window and
   `ranking.NewRanker(...).CalculateRanking()` runs the *unblended, default*
   ranking — the exact baseline `internal/updater` uses before the opt-in
   #108 prior blend. Every actual older-backfill request and response is
   captured as a trace.
2. **Replay (independent path).** `load.MaterializeAt` loads the same window
   with an explicit cutoff (`window.StartTime`), the resolved week, and
   bounded row limits, using the *same shared production SQL predicates*
   (extracted in `internal/ranking/load`) rather than any value derived from
   the reference. The snapshot owns plain values only.
3. **Comparison (after rollback).** The replay ranking is recomputed from the
   rolled-back snapshot and compared to the reference under the parity
   contract below.

## Guarantees

- The tool runs entirely inside a PostgreSQL transaction opened with
  `ReadOnly: true` and `Isolation: repeatable read`. It verifies both modes
  server-side (`SHOW transaction_read_only` / `SHOW transaction_isolation`)
  before any application read, sets `SET LOCAL statement_timeout = '10s'`,
  and **always rolls back — there is no commit path**. The report records
  `verified_read_only`, `verified_isolation`, and `rollback_completed`.
- No ESPN or AP network access, no updater commands, no schema changes, no
  writes of any kind.
- Errors never carry connection details; the DSN is never serialized.
- The CLI takes explicit `PG_*` values only (it does not load `.env`) and
  refuses non-loopback hosts. Use a disposable local database, never a
  production connection or a local tunnel to one.

## Parity contract

Exact equality of: team set, per-team `Record{Wins,Losses,Ties}`,
`FinalRank`, base game sequence and values, and the
ordered older-backfill request/response traces (including the selected
traversal/deficit audit). Numeric fields (`Record.Record`, `FinalRaw`, `SRS`,
`SRSNorm`, `SOS`, `SOSNorm`) use a fixed absolute tolerance of `1e-9`;
nonfinite values fail.

Fail closed:

- **Every** final-rank mismatch fails, including within-tolerance near ties.
- Component rank (`srs`/`sos`) differences are diagnostic only when a pairwise
  near-tie proof exists in **both** runs; otherwise they fail.
- A missing requested week cannot be replaced by the latest available week.
  A non-regular-season window or a cutoff that contradicts the manifest's
  optional assertion also fails the run.
- A run that never issued an actual older-backfill request fails as a vacuous
  pass (`missing_backfill_coverage`).
- On any failure the tool writes `parity.json` (status `failed`, failures,
  counts) and exits non-zero. It never writes rankings, metrics, polls, or
  any efficacy/coverage report — on success or failure, `parity.json` is the
  only artifact.

## Baseline scope

The baseline is the **unblended, default ranking path**. The opt-in early-season
FBS prior blend (#108) is applied only *after* ranking in the updater; this
tool does not model it and reports do not include it.

## Coverage scope

Only explicitly listed manifest windows are compared. This tool makes **no
full-study coverage claim**; the archived full-study evidence
(`fix/zero-game-record`, commit `6f264f9`) is unchanged and not weakened.
Manifests are bounded to 60 windows.

## Usage

```sh
# 1. Provision a disposable PostgreSQL (never production):
docker run --rm -d --name stats-replay-pg \
  -e POSTGRES_USER=stats -e POSTGRES_PASSWORD=stats -e POSTGRES_DB=stats \
  -p 127.0.0.1:55432:5432 postgres:16-alpine

# 2. Migrate the schema and seed synthetic data:
PG_HOST=127.0.0.1 PG_PORT=55432 PG_USER=stats PG_PASSWORD=stats \
  PG_DBNAME=stats PG_SSLMODE=disable go run ./cmd/migrate-schema
psql 'host=127.0.0.1 port=55432 user=stats password=stats dbname=stats sslmode=disable' \
  -f internal/ncaafreplay/testdata/seed.sql

# 3. Run the replay (new-only output directory; exits non-zero on failure):
PG_HOST=127.0.0.1 PG_PORT=55432 PG_USER=stats PG_PASSWORD=stats \
  PG_DBNAME=stats PG_SSLMODE=disable \
  go run ./cmd/ncaaf-replay \
    --manifest internal/ncaafreplay/testdata/manifest.json \
    --output-dir /tmp/ncaaf-replay-out
```

Flags: `--max-team-rows` (≤512), `--max-game-rows` (≤8192),
`--max-older-rows` (≤6144) optionally tighten the loader bounds.

## Optional AP comparison (deferred, not wired)

The archived branch implements AP-poll comparison (CSV manifests + HTML
snapshots, `scripts/import_ap_polls.py` on `fix/zero-game-record`). This slice
does not import, parse, or fetch AP data at runtime. To compare against poll
dates later, provision fixtures as checked-in CSV/manifest files under
`internal/ncaafreplay/testdata/` with per-window explicit cutoffs (poll dates
are Eastern midnight, not production Tuesday boundaries), extend the manifest
schema, and compare the independent inputs without assuming equivalence. Do
not fetch from ESPN or AP at runtime.

## Tests

- Hermetic (default): `go test ./...` — loader/parser/parity/transaction tests
  use in-memory SQLite and a fake SQL driver.
- Optional disposable PostgreSQL: `STATS_TEST_PG_DSN='host=127.0.0.1
  port=55432 user=stats password=stats dbname=stats sslmode=disable'
  go test -tags=integration -count=1 ./internal/ncaafreplay/` — skipped when
  the DSN is unset. Use a fresh database, not the one seeded in the example
  above. The test seeds its own data and checks parity, the transaction
  proof, and unchanged table counts.

## Known divergences (documented, fail-closed)

1. Map iteration and float accumulation order make bit-identical runs
   impossible; the `1e-9` tolerance exists for this, and near-tie rank flips
   fail closed.
2. Equal `start_time` ordering is unspecified in SQL; replay preserves
   returned order and validates non-increasing times only. No ID tie-break was
   added; equal-time prefix differences fail parity.
3. The production base query has no upper season bound; replay rejects
   `season > year` rows. Corrupt future rows fail replay but not production.
4. Duplicate joined team rows: production fans out; replay rejects.
5. A current-season game appearing after a prior-season row in start-time-desc
   order (e.g. a January bowl labeled with the prior season) rejects the
   window — legitimate season-boundary data can block parity deliberately.
6. Older backfill prefix equivalence relies on identical `ORDER BY start_time
   desc` with no tie-break (see 2).
7. No historical-year floor in older backfill; only the top-quota prefix is
   used, and `oldest_returned_season` may reach far back.
8. Week labels: production supports `Week 0` and can fall back to the
   latest game. The tool requires a positive week and rejects fallback.
9. Postseason windows cannot be expressed by replay and are rejected.
10. `record` includes all teams' games while `srs` restricts to the division;
    both sides use the same ranker, so parity is unaffected.

## Deferred (preserved on the archive branch, not ported)

- AP import/parsing/alias mapping, CSV manifests, HTML snapshots, poll
  comparison.
- Frozen preseason prior + modifier grid experiments (`internal/backtest`
  on the archive).
- Full-season study driver, Python checkers/contract, evidence documents.
- Repair command, updater duplicate guard, seasonal prior writers, and other
  updater changes — explicitly not ported.
- Bounded production-reference reads (`InputBounded` row budgets on the
  reference path) — the normal `InputWindow` query is unchanged. The
  reference row count is checked after loading it and before replay
  materialization.
