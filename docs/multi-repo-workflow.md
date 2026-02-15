# Multi-Repo Feature Workflow

This repo (`stats-go`) is the integration source of truth. The sibling repo
`stats-web` is a static frontend consumer. There is no hub repo.

## Ownership

| Concern | Owner |
|---|---|
| Docker Compose (backend + Postgres) | `stats-go` |
| Database schema and migrations | `stats-go` |
| API contracts (paths, fields, HTTP semantics) | `stats-go` |
| Frontend rendering and UX | `stats-web` |

When there is a conflict about who owns something, `stats-go` wins.

## Feature Workflow

All cross-repo features start here. This sequence is non-negotiable:

1. **Define the contract in `stats-go`.** Write the migration, update models,
   implement the endpoint. Finalize field names and response shapes before any
   frontend work begins.
2. **Validate the full stack locally.** `docker compose up` must be green.
   New or changed endpoints must return correct responses.
3. **Open a `stats-go` PR.** Get it reviewed and merged (or at minimum freeze
   the contract) before touching `stats-web`.
4. **Then open a `stats-web` PR** that consumes the finalized contract.

One PR per repo per feature slice. Do not batch unrelated changes into a
cross-repo feature branch.

## Mixed-Version Deployment

During rollout, the backend and frontend will briefly be at different versions.
Design for this:

- **New fields may be absent** on an older backend. The frontend must handle
  missing fields gracefully (don't crash on undefined).
- **Old fields must remain present** until the frontend no longer reads them.
  Do not remove a response field in the same deploy that the frontend stops
  using it — these are separate deploys.
- **Never make a breaking API change** (removing a field, changing a field type,
  altering HTTP semantics) without a versioning plan documented in the PR.

## Validation Checklist

Before declaring a backend feature complete and unblocking `stats-web` work:

- [ ] `go test ./...` — no failures
- [ ] `golangci-lint run --config=.golangci.yml ./cmd/... ./internal/...` — clean
- [ ] `docker compose up` — stack starts and is healthy
- [ ] New or changed endpoints return correct responses
- [ ] Migrations run cleanly on a fresh database
- [ ] Migrations run cleanly on a populated database (no data loss)
- [ ] API contract documented in the PR description (fields, types, method, path)
- [ ] `ARCHITECTURE.md` and `docs/` updated if interfaces or packages changed

## `stats-web` Coordination Rules

- All API questions are resolved in `stats-go`. Do not negotiate shape in the
  frontend repo — open a `stats-go` issue or PR and fix it at the source.
- Do not add a `stats-web` workaround for a `stats-go` bug. Fix the backend.
- Do not duplicate the rules in this file in `stats-web`. Point to this file.
- `stats-web` runs locally against this backend. If local setup is broken,
  the fix lives here.
