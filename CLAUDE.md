# stats-go

College sports computer ranking system written in Go. Collects game data from
ESPN for football and basketball, computes rankings using a composite algorithm,
and stores the results in PostgreSQL. The sibling `stats-web` site queries the
database directly at build time — there is no HTTP API in this repo.

## Repo Responsibilities

This repo is the **orchestration authority** for all cross-repo work. It owns:

- Docker Compose configuration (backend + Postgres)
- Database schema and all migrations
- The shared database contract with `stats-web` — table shapes, column names,
  and semantics (there is no HTTP API; `stats-web` reads the database directly)
- Runtime behavior and business logic

The sibling repo `stats-web` is a static site consumer. It has no authority
over the schema, contracts, or deployment orchestration. All schema and data
contract decisions are made here. See
[docs/multi-repo-workflow.md](docs/multi-repo-workflow.md).

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
  ranker/         CLI tool to calculate and print rankings
  updater/        Service that fetches games and updates DB on a schedule
  migrate/        One-time migration from PostgreSQL to SQLite
  migrate-schema/ One-shot schema initializer (runs before updater in Compose)
internal/
  config/     Environment-based configuration (godotenv)
  database/   GORM models and DB initialization (Postgres + SQLite)
  espn/       ESPN API client (game schedules, stats, team info)
  game/       Game data parsing and stat extraction
  logger/     Structured logging (zap)
  ranking/    Ranking algorithm (SRS, SOS, composite scoring)
  team/       Team info parsing from ESPN
  updater/    Orchestration of DB updates and ranking computation
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full dependency graph and design
rationale.

Key patterns:
- **Multi-sport support** — Football (`ncaaf`) and basketball (`ncaam`) via `espn.Sport` type
- **Three independent CLI entry points** in `cmd/` — each wires its own deps
- **Sport subcommands** — CLIs use `ncaaf`/`ncaam` subcommands; `schedule` runs both
- **Updater struct** receives DB, Logger, and ESPN client via dependency
  injection; construct it with `updater.NewUpdater`, which validates inputs
  (nil DB/logger/client and unknown sports return errors, mirroring
  `ranking.NewRanker`).
- **ESPN package** — per-client URLs via `NewClientForSport(sport)`, no DB dependency
- **Ranking package** takes a `*gorm.DB` and sport, computes everything in-memory
- **Sport column** on shared DB tables (`games`, `team_names`, `team_seasons`, `team_week_results`)

## Conventions

- All database access goes through GORM. Transactions are manually managed
  (`SkipDefaultTransaction: true`).
- Use `go.uber.org/zap` SugaredLogger for all logging. No `fmt.Println` outside
  of the ranker CLI output (enforced via `forbidigo` lint).
- Errors are propagated up — panics are only recovered at the scheduler level
  in `cmd/updater`.
- ESPN API calls are rate-limited via `espn.SportClient.Throttle()` (default
  500ms pause, `espn.Client.RateLimit`), which every multi-request path (week
  loops, date loops, per-game fetches) calls between sequential requests.
- The HTTP client in `espn/request.go` retries 5xx responses with exponential
  backoff (`InitialBackoff * 2^attempt`, capped at 30s). `MaxAttempts` is the
  total number of tries (default 5, i.e. 4 retries), and no backoff sleep
  happens after the final failed attempt.
- Test files use `_test.go` suffix. ESPN tests use a mock HTTP server pattern
  with fixture data. Updater tests are hermetic (mock ESPN HTTP server +
  in-memory SQLite) and run in the default `go test ./...` suite — use the
  `integration` build tag only for tests that genuinely need external
  resources.
- `nolint` directives require both a specific linter and an explanation
  (`require-explanation: true`, `require-specific: true`).

## Data Access Gotchas

Hard-won notes from debugging this schema (2026-08-30). Apply to every query,
including ad-hoc/psql sessions:

- **Every core table is keyed by `sport`** (`ncaaf`/`ncaam`): `games`,
  `team_names`, `team_seasons`, `team_week_results`. Always filter it. A query
  without the sport filter silently doubles rows for any school with both
  programs (272 schools have both a football and a basketball row in
  `team_names`).
- **`team_week_results` rows for a given school may exist under both sports**
  (the FBS and FCS rankings write separately). When correlating with
  `team_seasons`, join on `(team_id, sport, year)` — joining on `team_id`
  alone mixes divisions and fan-outs counts.
- **The `team_week_results` primary key includes `postseason`** — a regular
  week row and a Final row coexist for the same `(team_id, year, week, sport)`.
  Filter `postseason` to get the one you mean.
- **FCS-vs-lower-division games legitimately have competitors with no
  `team_seasons` row** (D-II/NAIA opponents are not ranked). LEFT JOIN from
  games; don't INNER JOIN `team_seasons` when auditing game coverage.
- **`team_week_results.final_raw` is `double precision`** — but if the column
  type drifts to `numeric`, `postgres.js` (stats-web) returns it as a string,
  which crashes frontend code doing numeric formatting. See
  [docs/multi-repo-workflow.md](docs/multi-repo-workflow.md) for the data
  type contract.

## Migration Rules

- **Additive first.** New columns must be nullable or carry a DEFAULT. Never
  add a NOT NULL column without a default to an existing table in a single
  migration.
- **No drops in the same migration that adds.** If replacing a column, the
  drop is a separate migration after `stats-web` has stopped reading the old field.
- **Destructive migrations require a rollback plan** documented in the PR body.
- **Migrations are forward-only.** Write them as if rollback is unavailable. A
  wrong migration is fixed by a new forward migration, not an edit.
- **Test against populated data**, not just an empty schema.

See [docs/migration-rules.md](docs/migration-rules.md) for full guidance.

## Branching & PR Rules

- **Never commit directly to `master`.** Always branch.
- Branch naming: `<type>/<short-description>` — e.g., `feat/add-basketball-rankings`,
  `fix/migration-null-column`.
- One PR per feature slice. Scope PRs tightly.
- PRs introducing a new or changed schema/data contract must include the
  contract shape in the PR description: table names, column names, and types.
- Do not merge if `docker compose up` fails or if tests/lint are red.
- Backend PR merges before the corresponding `stats-web` PR is opened.

## Anti-Patterns

- **Do not negotiate the database contract in `stats-web`.** All schema and
  data decisions are made here. If the frontend surfaces a problem, fix it here.
- **Do not add a frontend workaround for a backend bug.** Fix the backend.
- **Do not add NOT NULL columns without defaults in a single migration.**
- **Do not open a `stats-web` PR before the backend schema contract is finalized.**
- **Do not duplicate these rules in `stats-web`.** That repo defers to this one.
  Link to this file, do not copy it.
- **Do not skip `docker compose up` validation.** A green stack is the minimum
  bar for "done" on any backend feature.
- **Do not introduce schema hacks** (nullable columns used as booleans, magic
  sentinel strings) without immediately recording them in `docs/tech-debt.md`.

## Golden Rule: Keep Documentation Up to Date

Any change that alters architecture, package dependencies, public interfaces,
ESPN API usage, conventions, or design rationale **must** include corresponding
updates to the relevant docs (`README.md`, `CLAUDE.md`, `ARCHITECTURE.md`, or
files in `docs/`). Documentation that contradicts the code is worse than no documentation
at all — it actively misleads future work.

When in doubt, update the docs. `README.md` is written for a human audience —
keep it clear, practical, and free of agent-facing jargon. When adding a new
package or changing how an existing one works, update `ARCHITECTURE.md`. When
making a deliberate tradeoff, record it in `docs/design-decisions.md`. When
discovering or resolving tech debt, update `docs/tech-debt.md`.

## Deeper Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — package dependencies and layering
- [docs/design-decisions.md](docs/design-decisions.md) — rationale for key choices
- [docs/espn-api.md](docs/espn-api.md) — ESPN endpoints and data shapes
- [docs/tech-debt.md](docs/tech-debt.md) — known issues and improvement areas
- [docs/multi-repo-workflow.md](docs/multi-repo-workflow.md) — cross-repo feature process and `stats-web` coordination
- [docs/migration-rules.md](docs/migration-rules.md) — database migration safety rules
