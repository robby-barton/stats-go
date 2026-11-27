# NCAAF Army–Temple reimport guard

The reviewed repair of the 2012 Army–Temple game is **already applied**:
canonical NCAAF event `323220349` holds the reconciled stats and retired
duplicate event `400368353` is gone. This change does not re-run, reproduce, or
document a repair procedure. It only stops the updater from bringing the retired
duplicate back. It is not a fuzzy deduplicator, schema constraint, data-repair
tool, or ranking change.

## Scope and decisions

Only NCAAF duplicate event `400368353`, judged against canonical `323220349`, is
protected. Basketball events with the same numeric ID and every unrelated game
are untouched.

| Entry point | Behavior on the protected ID |
| --- | --- |
| `UpdateCurrentWeek` / `UpdateGamesForYear` discovery | Skip before any summary fetch; unrelated games still import |
| `processGames` mixed batch | Same skip; batch continues, no error |
| `UpdateSingleGame` | Return an explicit `known-event protection` error |
| `markGameRetry` | Return the same error; no `retry=1` row |
| `insertGameInfo` stat write | Return the same error; no upsert/DML |

A batch skip is visible, not silent:

- a structured warning `known-event import rejected` carries `game_id`,
  `canonical_game_id`, and `reason`;
- `GameUpdateResult.Skipped` holds a `ProtectedGame` per refusal (with
  `SkippedIDs()` for callers);
- `cmd/updater` logs skipped IDs after the existing processed/failed summaries.

The guard never fetches the duplicate's summary, creates no retry placeholder,
neither redirects nor rewrites canonical stats, and performs no write against
either event. Already-existing duplicate rows are left for separate operator
remediation.

## Fail-closed rules

The refusal is keyed on the exact ID and sport, so it cannot be disabled by
canonical metadata:

- canonical row missing, wrong sport, or any reviewed identity field drifted;
- source schedule header missing/malformed or not matching the reviewed scores
  and competitors;
- parsed-header mismatch on a direct stat write.

Those conditions change only the diagnostic `reason`; the protected game is
still skipped/refused, even if it already exists with matching scores. An
error reading the canonical row is logged with the refusal but does not stop
unrelated games. Errors from normal game reads or writes still follow their
existing handling.

The reviewed canonical identity is: NCAAF, season 2012, week 12, start
`2012-11-17T17:00:00Z`, home 349 score 32, away 218 score 63, postseason 0,
neutral false, conference false, retry 0.

## Verification

Hermetic tests use mock ESPN HTTP and in-memory SQLite; no credentials, live
ESPN, or production access are involved.

```sh
go test -count=1 ./internal/updater ./cmd/updater
go test -race -count=1 ./internal/updater
golangci-lint run --config=.golangci.yml ./internal/updater/... ./cmd/updater/...
go build ./...
```

Coverage includes a mixed `processGames` batch, current-week and
historical-season discovery, explicit single-game refusal, direct stat-write and
retry defense, canonical/source metadata drift, canonical and unrelated normal
imports, and basketball with the same numeric ID.

## Boundaries

This guard does not remove an existing duplicate, discover other duplicate IDs,
freeze canonical-ID imports, add a database constraint, or change deployment,
schema, ranking formulas/order, or published data.
