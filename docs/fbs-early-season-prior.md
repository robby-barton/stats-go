# FBS early-season prior (opt-in, default off)

The scheduled updater can automatically freeze one validated preseason
strength prior per football season and blend it into early-season FBS
rankings. The feature is **off by default** and controlled by a single flag:
without `--fbs-early-season-prior` the updater publishes exact baseline
rankings, never reads or writes the `fbs_season_priors` table, and registers
no prior-related job. This document covers the policy, the stored artifact,
and one-time enabling/disabling — there is no yearly runbook.

The blend uses the weights and game-based fade tested in the 2026
retrospective, with a fixed scheduler-based calendar anchor and no
operator-tunable parameters:

- Shrinkage weight per team: `lambda = max(0, 1 - (W+L+T)/4)` — the record
  includes FBS-FCS games.
- Prior contribution: `0.9 * prior z-score`, with no restandardization.
- Baseline z-scores use the population SD with a floor of `0.05`.
- Full calendar bypass **56 Eastern calendar days after the fixed August 10
  midnight Eastern anchor**; at or after that instant the updater publishes
  the exact baseline, silently.
- Bypassed teams keep their exact baseline score; if every team bypasses,
  all baseline ranks are preserved exactly.

## The fixed August 10 anchor (distinct from the former AP-date policy)

The anchor is **midnight Eastern on August 10 of the season year**, fixed.
The day-56 full-bypass instant (midnight Eastern on October 5) is always
derived from that fixed anchor. This is deliberately
different from the former research-shadow policy, which pinned the anchor to
a manually chosen AP-date artifact: here the anchor is a policy constant,
and only the observed **capture** date moves.

## Automatic freeze task (no yearly action)

With the flag enabled, the updater itself creates and persists the prior:

- **Triggers.** The task runs after successful annual football season
  initialization, at 06:07 Eastern each day of the retry window, and on
  updater startup. Every trigger checks the Eastern date and does nothing
  outside the window.
- **Window.** August 10–16 Eastern, inclusive. The target season is the
  Eastern calendar year. If the target year's football metadata is wholly
  absent, the task initializes it through the same annual season path; if
  metadata exists but fails validation or coverage, it is **never
  rewritten** — the attempt logs the reason and a later retry in the window
  picks it up.
- **Inputs.** From the two prior seasons only (year-2..year-1), bounded
  (512 FBS field rows, 8192 retained games), FBS-flagged teams, through the
  fixed anchor, between target-field teams, using the already tested SRS
  normalization with backfill disabled and exact-zero mean imputation for
  no-history newcomers. Incomplete or unusable field/history — missing
  metadata, an unsupported FBS departure, an empty donor season, a
  continuing FBS member without usable coverage, or a degenerate/nonfinite
  result — refuses the attempt and publishes no candidate. These checks
  catch common incomplete loads; they cannot independently prove complete
  coverage. If a real FBS departure looks like a missing team, the updater
  keeps the season on baseline.
- **Midseason guard.** The freeze refuses if any football game for the
  target season is already persisted (any phase, either division), or if an
  unclassified retry placeholder (a `season=0` games row) exists. The
  Eastern window is rechecked immediately before the insert.
- **Persistence.** One row per season in the additive `fbs_season_priors`
  table: `season` (PostgreSQL `bigint` primary key), `artifact` (`text`,
  version `ncaaf-season-prior/1`), and `checksum` (`text`, SHA-256 of the
  exact stored bytes). The updater reads donor data from one consistent
  transaction snapshot and inserts with conflict-do-nothing, serialized
  against game writes. Repeat attempts, races, and restarts
  never replace an existing row — a corrupt row is likewise never repaired
  or regenerated; rankings then report the validation failure and stay on
  baseline. The automatic freeze is immutable; the only writer that may
  replace or delete a row is the explicit rebuild command below.
- **Capture record.** The real observed freeze date (August 10–16) and
  capture instant are recorded inside the artifact separately from the
  anchor, so retry dates are visible in the stored artifact.

## Scope

Two paths use the opt-in prior; everything else stays at baseline.

- **Scheduled football worker** (`schedule --fbs-early-season-prior`). The
  latest regular-season FBS window is resolved first, and the row keyed by
  that resolved season is read fresh on every run — a row frozen after
  startup is usable without a restart. Preseason (stored phase `-1`),
  postseason, windows with no current-season game, and the at/after-expiry
  case are normal silent skips that never read the table. Missing,
  unreadable, checksum-mismatched, or season-mismatched rows log a warning
  and leave the computed baseline unchanged; a stored row is never repaired,
  replaced, or synthesized.
- **Inline rebuild** (`updater ncaaf ranking --all --fbs-early-season-prior`).
  This is the authoritative rebuild path. It **reconstructs one prior freshly
  in memory per season in scope** (completed previous seasons *and* the
  current Eastern season) using exactly the scheduled freeze method: the same
  fixed August 10 Eastern anchor, the same bounded two-donor-season input
  window, the same field validation and SRS strength algorithm. Stored prior
  values are never used as inputs — not even a valid row — so manual
  corrections to team or game data flow into the recomputed prior. It never
  calls ESPN/AP and never fabricates persisted capture metadata, and it
  publishes what it recomputed (see below).
- Basketball, one-shot `ncaaf ranking` without `--all`, plain
  `ranking --all`, and backfill recomputation never touch the prior. They
  write baseline rows and **can overwrite earlier blended rankings**.
  `ncaaf games --year` updates only games, not rankings.

### Rebuild publication (authoritative, midseason allowed)

Because a rerun of all rankings after some update would be wrong with stale
priors, the explicit rebuild also **publishes** what it recomputed:

- **Scope.** One fresh reconstruction attempt per season whose fixed
  August 10 Eastern anchor has already been reached — every season with
  persisted ranking windows, plus the current Eastern season when its
  football metadata exists even before its first game. Future years and a
  season whose anchor is not yet reached are never staged.
- **Replacement.** A successfully reconstructed season's prior is stored
  over the existing row — including the current season's row, midseason
  allowed — as a `ncaaf-season-prior/2` artifact.
- **Invalidation.** When a reconstruction is attempted but cannot yield a
  usable prior, the rebuild warns, publishes the exact baseline for the
  affected windows, and invalidates that season's stale persisted prior in
  the same transaction: only that season's row is deleted, never a broad
  delete, and the schema is unchanged. A database read or serialization
  failure aborts the rebuild instead of invalidating a season.
- **Atomicity.** The prior writes and the rebuilt ranking upserts commit in
  one transaction. If the prior table is unavailable or any ranking write
  fails, the whole publication rolls back — the season is never
  half-rebuilt.
- **Serialization.** The publication write phase and the opted-in scheduled
  read/calc/write share one PostgreSQL advisory xact lock (the SQLite write
  lock is acquired up front, since a deferred transaction that upgraded only
  after reading could otherwise overwrite). A scheduled run therefore can
  never read the old prior and commit after a corrected rebuild publication:
  it either commits before the rebuild or reads the corrected prior. The
  rebuild holds the lock only over its writes, never across its ranking
  reads, and nothing takes the lock with the feature off. The automatic
  freeze keeps its own table locks, and its conflict-do-nothing insert can
  never overwrite a manual result. Run the manual rebuild after your data
  corrections commit; don't overlap it with another manual rebuild.
- **Timestamps.** The rebuilt artifact records the start time of the rebuild
  (`rebuilt_at`) and source (`source`) in schema `ncaaf-season-prior/2`; the
  fixed anchor and day-56 expiry, the exact-byte checksum, and the
  field/coverage payload contract are unchanged. No August preseason capture
  is fabricated for a later rebuild. Automatic preseason artifacts
  (`ncaaf-season-prior/1`) remain fully readable. The scheduler checks its
  clock against that rebuild time, but inside the rebuild itself the
  timestamp is never an eligibility gate for historical snapshots (the
  in-memory reconstruction has no capture timestamp at all).

### Inline rebuild clocks and caveats

- The rebuild's **current/latest** week-0 slot uses the actual observation
  clock, so it agrees with the scheduled worker even when the last game is
  stale. **Explicit historical weeks and past-season final snapshots use
  their own resolved historical ranking cutoff** (the Tuesday Eastern
  pre-week boundary, or the season's final snapshot cutoff) — never the
  first kickoff of the requested week, and never today's clock.
- The reconstructed prior is computed from today's stored data: **it uses
  the same method as the scheduled freeze but does not promise the same
  numbers** as any hypothetical historical snapshot when team metadata or
  game data has been revised since the freeze. Historical weeks are
  recomputed at their resolved cutoffs, not replayed at the exact times the
  scheduler originally published them.
- A manual rebuild of the current season can blend even when the scheduler
  did not freeze (no stored row). The scheduler's midseason guard does not
  apply to the rebuild: the donor window is bounded to seasons year-2..year-1
  through the anchor and verified to contain no target-season or future
  game, so no target or future game leaks into the prior; if the inputs are
  unusable, the window stays at the exact baseline.

## Enabling and disabling (one-time)

Enable:

```sh
updater schedule --fbs-early-season-prior
```

The same flag opts the one-shot ranking command into the same policy:

```sh
updater ncaaf ranking --all --fbs-early-season-prior
```

The rebuild flag requires `--all` (plain `ncaaf ranking` and backfill stay
at baseline) and is registered on the football command only.

Disable: restart without the flag. The stored rows are left in place
(they are additive history); with the flag off they are never read and the
freeze jobs are never registered. There is no manual artifact to produce,
mount, pin, or rotate, and no yearly runbook step.

## Stored rows and provenance

- The stored artifact carries its own provenance: a deterministic
  fingerprint of the sorted frozen field and retained ranker inputs
  (`input_sha256`), separate from the checksum of the exact stored bytes
  verified on every read. A rebuilt artifact additionally records the real
  rebuild instant and source (`rebuilt_at`, `source`).
- Rankings are persisted per (team, year, week, phase, sport). Disabling the
  prior does not retroactively restore blended rows; the next scheduled
  update for the same keys overwrites them. Historical backfills and plain
  `ranking --all` never apply the prior, so keys they upsert are baseline.
  The opt-in rebuild computes every row through the same baseline loader and
  score calculation and substitutes only the blended FBS scores and ranks on
  early regular-season windows — but because it recomputes from current
  stored data, running it (or `--all` alone) can overwrite rows that the
  scheduler blended earlier with different (possibly baseline) values.
- The `fbs_season_priors` table is additive and never destructively
  migrated; `stats-web` has no dependency on it. With the flag off the table
  is never read or written and the rebuild works with it dropped; with the
  flag on the rebuild publishes into it, so an unavailable table aborts the
  whole publication (rollback, nothing half-rebuilt).
