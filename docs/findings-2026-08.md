# Consolidated Review Findings — 2026-08-29

Findings from two independent AI design reviews (different model families) of the full
codebase, deduplicated and verified against source. Claims marked ✅ were confirmed by
direct inspection; the remainder were reported with file/line citations and are consistent
with verified adjacent code.

Suggested fix order: P0 (batch 1) → P1 (batch 2) → P2/P3 (batch 3+).

---

## P0 — Confirmed bugs

### 1. Exit codes swallowed ✅
`cmd/ranker/main.go:38`, `cmd/updater/main.go:312` — `rootCmd.Execute()` result is
discarded (`//nolint:errcheck`). Cobra validation and `RunE` failures do not produce a
non-zero exit status, so cron jobs and deploy scripts cannot distinguish success from
failure. Most updater one-shot commands compound this by logging errors and returning
`nil`.
**Fix:** return errors from each `RunE`; handle `Execute()` in `main` and `os.Exit(1)`
after logging once at the command boundary.

### 2. Deployer shutdown race ✅
`cmd/updater/main.go:83-96` — `deployer.stop()` calls `close(d.trigger)`, but
`Trigger()` still sends on that channel. A ranking worker finishing after shutdown
panics on send-to-closed-channel (`select/default` guards a *full* channel, not a
*closed* one). Stop functions also do not wait for workers to exit before closing.
**Fix:** introduce a process-level context; propagate it through ranking workers and the
deployer; use WaitGroup/errgroup to join workers; close the trigger channel only after
all producers have stopped.

### 3. Documented SQLite fallback is unreachable ✅
`internal/config/config.go:30` — `SetupConfig` always allocates a non-nil
`&database.DBParams{}`, even when every `PG_*` var is empty. `database.NewDatabase`
only opens SQLite when `params == nil` (`internal/database/database.go:20`), and only
`cmd/migrate` uses the nil path. README, `ARCHITECTURE.md`, and
`docs/design-decisions.md` all claim "omit PG vars → SQLite"; ranker/updater instead
attempt Postgres with empty settings.
**Fix:** construct `DBParams` only when the required Postgres vars are set; return a
validation error for partial configs (e.g., malformed port); or stop advertising
implicit SQLite.

### 4. Football season fetch skips the final regular-season week ✅
`internal/espn/football.go:67` — `for i := int64(1); i < numWeeks; i++` never requests
week N of the regular season (postseason week 1 is always fetched separately).
**Fix:** confirm `GetWeeksInSeason` semantics (whether the calendar includes the
postseason entry) and correct the loop bound; add a unit test for week coverage.

### 5. Ranking panics / NaN on degenerate input ✅
`internal/ranking/rating.go` — three related issues:
- Empty team set panics at `teamList[teamIDs[0]]` / `teamOrder[0]`.
- SRS normalization `(rating - minMOV) / (maxMOV - minMOV)` has no zero guard → NaN
  when all MOVs are equal.
- Tie ranking initializes `prev`/`prevRank` at zero, so the first team with an exact
  zero final score is assigned rank 0.
**Fix:** validate prerequisites up front; guard degenerate normalization ranges;
initialize tie tracking from the first sorted item; reject NaN/Inf values.

### 6. Unsafe ESPN parsing → cron panics or silent zero-value corruption ✅
`internal/game/parse_game_info.go:14`, `parse_player_stats.go`, `parse_team_stats.go`,
`internal/updater/update_games.go:31` — `checkGames` indexes
`game.Competitions[0]` / `Competitors[0|1]` without length checks (schedule
validation is weaker than box-score validation), numeric/date conversions ignore
errors, split results are indexed without length checks, and stat-map type assertions
are unchecked. Malformed upstream data either panics (recovered, but the cycle is
lost) or persists valid-looking zeros.
**Fix:** validate cardinality before indexing; return contextual parse errors;
distinguish missing/unsupported values from numeric zero; align schedule-game
validation with `GameInfoESPN.validate`.

---

## P1 — Data integrity & operations

### 7. Failed game fetches are dropped; partial results reported as success
`internal/updater/update_games.go:171-187` — `processGames` logs and `continue`s on
`GetSingleGame` errors, then returns nil. Historical backfills can announce completion
despite omitted games. `database.Game.Retry` (`internal/database/models.go:91`) exists
and is never read or written.
**Fix:** return a structured result (successes + per-game failures) or persist failures
for durable retry. Current-week polling may tolerate partial failure, but that policy
belongs at the scheduler boundary, not inside the core operation.

### 8. DB read failures masquerade as missing data ✅
`internal/updater/update_team_season.go:27` — `seasonsExist` logs query errors and
returns `false`, so a database outage is indistinguishable from "season not yet in
DB" and triggers expensive ESPN work before failing elsewhere.
**Fix:** return `(bool, error)` and propagate.

### 9. `sync.Once` permanently caches transient season lookup failures
`internal/espn/basketball.go:15` (`cachedSeasonOnce`) — a transient `DefaultSeason`
error is cached for the life of the process.
**Fix:** cache only successful results, or allow failed lookups to retry.

### 10. Fresh deployment has no schema initialization path
`docker-compose.yml:36` — no migration runner is wired into Compose or startup;
`cmd/migrate` copies data rather than managing the production schema. A fresh
`docker compose up` yields healthy containers and failing jobs.
**Fix:** make schema migrations a first-class one-shot service that must succeed
before the updater starts; prefer a versioned migration tool.

### 11. SQLite schema diverges from Postgres
`db/schema-sqlite.sql:38,53` — self-referencing `game_id` foreign key on `games`
(no Postgres equivalent, presumably accidental); SQLite connections do not enable
foreign-key enforcement, so declared relationships are ignored.
**Fix:** remove the self-reference; enable FKs on every SQLite connection; consider
generating both schemas from one migration history; add schema-level tests against
the checked-in SQL.

### 12. Rate limiting is not where the docs say, and is incomplete
`docs/design-decisions.md` says 500ms between batch calls lives in `game/`; it
actually lives in `updater.processGames` and basketball date loops.
`internal/espn/football.go:67` (week loop) and `GetCurrentWeekGames`/
`GetGamesForSeason` have no pause at all.
**Fix:** centralize on `SportClient` (or a small `Do` helper) so every multi-request
path shares one policy; update the docs to match.

---

## P2 — Architecture

### 13. Sport identity is three parallel vocabularies
`internal/espn/espn.go:19` (`Sport` slug + `SportDB()`), `internal/ranking/ranking.go:11,21`
(redefines DB strings, panics on unknown), CLI literals. Ranking cannot import espn
(by design), so the strings are duplicated and typo-prone.
**Fix:** put the persistence identifier (`ncaaf`/`ncaam`) in `database` or a tiny
`internal/sport` package; keep ESPN slugs/URLs inside `espn`; type the shared value.

### 14. Ranking algorithm is glued to GORM and mutates the Ranker
`internal/ranking/setup.go:27` (`setGlobals` overwrites caller's struct),
`rating.go`/`record.go`/`sos` re-query `*gorm.DB` mid-computation;
`startTime` derived with `time.Local` (`setup.go:55`) makes rankings
timezone-dependent. Not concurrent-safe; algorithm tests require a schema.
**Fix:** load games/teams once (or accept `[]database.Game` + team set), compute on a
value type, keep GORM at the updater/ranker-cmd edge, use UTC explicitly.

### 15. ESPN transport is not cancellable and is easy to misuse
`internal/espn/request.go:107` — new `http.Client` per call, `context.Background()`
everywhere, ignored `NewRequestWithContext` error, unchecked type assertion on
`data.(validatable)`, unused `Responses` type parameter. Scheduler shutdown cannot
abort in-flight retries/sleeps; basketball backfill issues ~160 sequential requests
with no parent context.
**Fix:** hold one `*http.Client` on `Client`; take `context.Context` through
`SportClient`; make `makeRequest` generic over `Responses`.

### 16. Parser/persistence boundary is inconsistent
`internal/game/game.go:9` maps ESPN JSON straight into `database.*` models, while
`team` uses `ParsedTeamInfo` DTOs. Unknown stat names `fmt.Printf` to stdout
(`parse_team_stats.go:65`), violating the system's logging convention.
**Fix:** ESPN DTOs in `espn`, domain structs in `game`/`team`, persistence mapping in
`updater`; log through the logger.

---

## P3 — Hygiene & docs drift

### 17. Documentation systematically wrong
- README/ARCHITECTURE.md/CLAUDE.md document `football`/`basketball` subcommands;
  Cobra uses `ncaaf`/`ncaam` (`cmd/ranker/main.go:54`, `cmd/updater/main.go:295`).
- `Makefile` still has `go run ./cmd/updater ranking --all` (invalid after sport nesting).
- ARCHITECTURE.md says "14 tables"; code has 19 GORM models.
- CLAUDE.md claims the system "serves results via an API"; there is no HTTP server.
- docs/tech-debt.md says basketball historical support is unimplemented, though
  `BasketballClient` now synthesizes historical date ranges.
**Fix:** sweep docs to match reality; the repo's own golden rule requires it.

### 18. Dead schema and unused fields
`Conference`, `Composite`, `Recruiting`, `Roster`, `Player` models are never used;
`ranking.Team` carries unused `Composite*`/`SRSHigh*`/`SRSLow*`/`SOV*`/`SOL*` fields
(`internal/ranking/ranking.go:70-88`); `team_week_results.sov_rank`/`sol_rank` are
never populated (`update_team_week_results.go:56`).
**Fix:** delete or document as owned by another consumer before removing.

### 19. Copy-paste persistence templates
`internal/updater/update_games.go:48` — `insertGameInfo` repeats ten identical
`OnConflict{UpdateAll: true}` blocks.
**Fix:** small helper or table-driven upserts so adding a stats table is one line.

### 20. Retry/cache semantics are brittle
`internal/espn/request.go:130` — `MaxRetries=5` means five *total attempts* while docs
describe five *retries*; code sleeps after the final failed attempt ✅.
**Fix:** define attempts-vs-retries unambiguously; skip backoff after the last attempt.

### 21. Tests excluded from default suite; global URL state
All updater tests carry the `integration` build tag (`internal/updater/updater_test.go:1`),
so `go test ./...` runs none of them. Leftover `NewClient`/package-level URL fallback
forces tests through global mutable state (`SetTestURLs`).
**Fix:** add inexpensive unit tests for command error propagation, partial failures,
shutdown coordination, and empty batches to the default suite; move tests to
per-client URL configuration.

### 22. No validation seam for the library surface
`predictor`-style note: `Updater`/`Ranker` accept nil DB/logger/client and expose
mutable fields; invalid sport panics deep inside computation.
**Fix:** validate dependencies up front in constructors; return errors for invalid
input; keep fields private.

---

## Review attribution

Two independent reviews were run in parallel with identical prompts on 2026-08-29
(details of the models intentionally irrelevant — findings stand on their own):

- **Reviewer A** (structural lens): found #4, #12, #16, #18, #19 uniquely; co-found
  #3, #6, #7, #13, #15, #17.
- **Reviewer B** (operational lens): found #1, #2, #8, #9, #10, #11, #20 uniquely;
  co-found #3, #5, #6, #7, #13, #15, #17.

Both converged on the SQLite fallback, unsafe parsing, dropped game fetches, docs
drift, sport-identity duplication, and missing context propagation — the highest-
confidence items in this document.
