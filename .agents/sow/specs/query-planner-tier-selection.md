# Spec - Query planner tier selection

## Status

Active. Added by SOW-0017.

## Scope

This spec describes automatic storage-tier selection for metric data
queries when the caller does not explicitly request `tier=`.

It applies to the query-target planner used by `/api/v1/data`,
`/api/v2/data`, `/api/v3/data`, MCP metric queries, weights/value
helpers, and other callers that reach `rrd2rrdr()` without
`RRDR_OPTION_SELECTED_TIER`.

Explicit `tier=` requests are outside this automatic selection rule.
They keep the existing behavior: the requested tier is used when valid,
and automatic tier switching is disabled.

## Contract

Automatic tier selection is resolution-driven among tiers that overlap
the requested effective query window.

1. A tier with no overlap with the requested effective window is not a
   candidate.
2. A candidate tier is scored by point density as if it had full-window
   coverage:

   ```text
   candidate_points = effective_duration / tier_update_every
   ```

   The implementation uses fixed-point integer weights so sub-resolution
   windows retain fractional ordering instead of collapsing to zero.

3. The acceptable-density threshold is 50% of the requested output point
   count.
4. If one or more candidate tiers meet the 50% threshold, select the
   sparsest acceptable tier. This avoids reading much denser data when a
   coarser tier can satisfy the requested output density well enough.
5. If no candidate tier meets the 50% threshold, select the densest
   candidate tier. This handles short windows below every tier's
   resolution and preserves the best available fidelity.
6. After the initial tier is selected, the existing query planner may
   fill beginning/end coverage gaps with neighboring tiers. Tier
   switching is a coverage-gap mechanism, not a full per-segment
   resolution optimizer.

## Important Edge Cases

- A requested window shorter than a 10-second collector cadence must not
  make all overlapping tiers unusable. The densest overlapping tier wins
  when no tier reaches the 50% threshold.
- A non-overlapping tier must never win just because every tier has poor
  density.
- When all tiers cover a narrow sub-resolution window, automatic
  selection should choose the densest tier, not the highest-numbered
  tier.
- When a coarser tier can provide at least 50% of the requested point
  density, it may be selected over a much denser tier to reduce source
  reads while preserving acceptable output fidelity.

## Natural Points

The legacy automatic aggregate-tier helper is not part of this contract.
Natural-points update-every selection for an explicit selected tier uses
the minimum `db_update_every_s` for that selected tier across the query
target metrics.

## Code References

- `src/web/api/queries/query-plan.c` - automatic per-metric tier
  selection and planner gap-fill.
- `src/web/api/queries/query-window.c` - selected-tier natural-points
  update-every calculation.

