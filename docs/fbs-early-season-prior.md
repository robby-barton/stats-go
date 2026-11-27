# FBS early-season prior (opt-in, default off)

The scheduled updater can blend a reviewed preseason strength prior into
early-season FBS rankings. The feature is **off by default**: without the
flag, the updater publishes exact baseline rankings and never reads the
prior file. This runbook covers opting in, rotating, and disabling.

The blend implements the researched 2026 retrospective policy exactly, with
no operator-tunable parameters:

- Shrinkage weight per team: `lambda = max(0, 1 - (W+L+T)/4)` — the record
  includes FBS-FCS games.
- Prior contribution: `0.9 * prior z-score`, with no restandardization.
- Baseline z-scores use the population SD with a floor of `0.05`.
- Full calendar bypass 56 Eastern calendar days after the verified preseason
  anchor; at or after that instant the updater publishes the exact baseline,
  silently.
- Bypassed teams keep their exact baseline score; if every team bypasses,
  all baseline ranks are preserved exactly.

## Opting in

Prerequisites — both are operator responsibilities; the runtime verifies
bytes, not provenance:

1. A reviewed `ncaaf-shadow-prior/1` artifact frozen by the research
   `shadow-freeze` tool for the current season, stored on a local path the
   updater container can read (a regular file up to 1 MiB; mount it read-only,
   e.g. a Compose bind mount of a directory holding only the reviewed artifact).
   The default Compose
   file is unchanged; the mount is added only when opting in.
2. The SHA-256 digest of the exact artifact bytes, computed independently
   (e.g. `sha256sum ncaaf-shadow-prior.json`).

Run:

```sh
updater schedule \
  --fbs-early-season-prior \
  --fbs-early-season-prior-path /etc/stats-go/ncaaf-shadow-prior.json \
  --fbs-early-season-prior-sha256 "$REVIEWED_PRIOR_SHA256"
```

Scope:

- Only the **scheduled football** ranking worker uses the prior. Basketball,
  the one-shot `ncaaf ranking` command, `--all`/backfill recomputation, and
  historical rankings are always baseline.
- The artifact is read **once at startup** and pinned against the digest.
  Only the latest regular-season FBS window can be blended: preseason
  (stored phase `-1`) and postseason windows, and windows with no
  current-season game, are normal silent skips.
- A prior for the wrong season, a diverged FBS field (team IDs or names),
  inconsistent window clocks, or any blend calculation failure logs one
  warning and that update publishes the exact baseline instead. Ordinary
  updater operation (games fetches, deploys, other sports) is never blocked
  or retried differently.

## Warning vs. expiry

- **Startup warning**: a bad artifact or digest mismatch disables the blend
  for that updater process. The scheduler continues at baseline.
- **Update warning**: a wrong prior year, field divergence, inconsistent
  clocks (including a latest kickoff outside the anchor-to-observation
  interval), or blend failure leaves that update entirely at baseline.
- **Silent baseline**: the day-56 calendar expiry, and the normal skips
  listed above. No warning is logged.

## Disabling or rotating

- **Disable:** restart the updater without `--fbs-early-season-prior`
  (or with the default Compose command). The artifact file is never read.
- **Rotate:** review the new artifact, pin its digest, restart the updater
  with the new path/digest. There is no hot reload.

## Stored rows

Rankings are persisted per (team, year, week, phase, sport). The blend only
affects rows upserted while it is enabled:

- Disabling the prior does **not** retroactively restore blended rows to
  baseline values. The next scheduled update for the same keys overwrites
  them (the final/complete-season recomputation upserts the same keys with
  baseline values); earlier weeks' stored rows keep whatever was written.
- Historical backfills and `ranking --all` never apply the prior, so any
  keys they upsert are baseline.
