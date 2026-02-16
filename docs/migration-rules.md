# Migration Rules

Database migrations in this repo are managed via GORM `AutoMigrate` in
`internal/database/`. Migrations affect the shared SQLite (local/ranker) and
PostgreSQL (production) databases.

## Core Rules

### Additive first

New columns must be nullable or carry a server-side DEFAULT. Never add a NOT
NULL column without a default to an existing table in a single migration — this
breaks any running instance that tries to insert a row before deploying the new
code.

```sql
-- Wrong: breaks existing inserts immediately
ALTER TABLE games ADD COLUMN source TEXT NOT NULL;

-- Right: nullable first, tighten later if needed
ALTER TABLE games ADD COLUMN source TEXT;
```

### Separate add from drop

If you are replacing a column, the removal is a separate migration that lands
after `stats-web` has been deployed and is no longer reading the old field. The
sequence is:

1. Migration: add new column.
2. Backend code: write to both columns during transition.
3. `stats-web` deploy: switch reads to new column.
4. Migration: drop old column.

Never add and drop in the same migration.

### Destructive migrations require a rollback plan

Dropping a table, column, or index requires a written rollback plan in the PR
body. "Delete the data" is not a plan. Describe what a revert deploy looks like
and whether data recovery is possible.

### Migrations are forward-only

Write every migration as if you cannot roll it back. If a migration turns out
to be wrong, the fix is a new forward migration — not an edit to an already-run
one. Editing a migration that has run in production produces divergence between
the recorded schema and actual state.

### Test against populated data

Before merging, run the migration against a database that already has rows in
the affected tables. An empty-schema test is not sufficient — migrations that
assume columns are empty or tables have specific row counts will fail in
production.

## Schema Design Constraints

- Shared tables (`games`, `team_names`, `team_seasons`, `team_week_results`)
  carry a `sport` column (`"ncaaf"` or `"ncaam"`). New columns on these tables
  must be sport-agnostic or include sport-specific defaults.
- `team_names` uses `(team_id, sport)` as its composite primary key. ESPN
  reuses team IDs across sports. Do not collapse this back to a single-column
  primary key.
- The `fbs` column in `team_seasons` means "top division" for both sports
  (`fbs=1` for FBS football, `fbs=1` for D1 basketball). This is acknowledged
  tech debt — do not rename it without a migration and a corresponding update
  to every query that references it.

## When Schema Hacks Are Unavoidable

If a schema compromise is necessary (e.g., reusing a column for a second
meaning, storing structured data as a string), it must be recorded immediately
in `docs/tech-debt.md` with a description of the intended clean-up path. Silent
hacks compound over time and become load-bearing.
