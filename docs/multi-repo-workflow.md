# Multi-Repo Feature Workflow

This repo (`stats-go`) is the integration source of truth. The sibling repo
`stats-web` is a static frontend that reads the PostgreSQL database directly
at build time (via `postgres` queries during its Eleventy build). There is no
HTTP API between the repos and no hub repo.

## Ownership

| Concern | Owner |
|---|---|
| Docker Compose (backend + Postgres) | `stats-go` |
| Database schema and migrations | `stats-go` |
| Database contract (tables, columns, semantics) | `stats-go` |
| Frontend rendering and UX | `stats-web` |

When there is a conflict about who owns something, `stats-go` wins.

## Feature Workflow

All cross-repo features start here. This sequence is non-negotiable:

1. **Define the contract in `stats-go`.** Write the migration, update the GORM
   models, and verify the data lands correctly. Finalize table/column shapes
   before any frontend work begins.
2. **Validate the full stack locally.** `docker compose up` must be green, and
   the updated tables must contain the data `stats-web` will query.
3. **Open a `stats-go` PR.** Get it reviewed and merged (or at minimum freeze
   the contract) before touching `stats-web`.
4. **Then open a `stats-web` PR** that consumes the finalized contract.

One PR per repo per feature slice. Do not batch unrelated changes into a
cross-repo feature branch.

## Mixed-Version Deployment

During rollout, the backend (data in Postgres) and frontend (build-time
queries) will briefly be at different versions. Design for this:

- **New columns may be absent** on an older backend. The frontend must handle
  missing fields gracefully (don't crash on undefined).
- **Old columns must remain populated** until the frontend no longer reads
  them. Do not remove a column in the same deploy that the frontend stops
  using it — these are separate deploys.
- **Never make a breaking schema change** (removing a column, changing a
  column type, altering its meaning) without a plan documented in the PR.

## Validation Checklist

Before declaring a backend feature complete and unblocking `stats-web` work:

- [ ] `go test ./...` — no failures
- [ ] `golangci-lint run --config=.golangci.yml ./cmd/... ./internal/...` — clean
- [ ] `docker compose up` — stack starts and is healthy
- [ ] New or changed tables/columns contain correct data after an updater run
- [ ] Migrations run cleanly on a fresh database
- [ ] Migrations run cleanly on a populated database (no data loss)
- [ ] Schema contract documented in the PR description (tables, columns, types)
- [ ] `ARCHITECTURE.md` and `docs/` updated if interfaces or packages changed

## `stats-web` Coordination Rules

- All API questions are resolved in `stats-go`. Do not negotiate shape in the
  frontend repo — open a `stats-go` issue or PR and fix it at the source.
- Do not add a `stats-web` workaround for a `stats-go` bug. Fix the backend.
- Do not duplicate the rules in this file in `stats-web`. Point to this file.
- `stats-web` runs locally against this backend. If local setup is broken,
  the fix lives here.
