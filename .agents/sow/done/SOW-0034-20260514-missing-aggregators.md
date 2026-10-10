# SOW-0034 - PromQL - Five missing aggregators

## Status

Status: completed

Sub-state: five aggregators (stddev, stdvar, limitk, limit_ratio,
group) implemented and wired through lowering + dispatch. Compliance
jumped from 472 -> 501 (+29 cases); aggregators.test from 80 -> 108
passes. 191/191 unit tests; 117/117 smoke. Live spot-check confirms
all five evaluate against the daemon as expected.

## Requirements

### Purpose

Implement the five aggregation operators that the SOW-0030 compliance
corpus flagged as missing: `stddev`, `stdvar`, `limitk`, `limit_ratio`,
`group`. These are the largest single category of compliance failures
documented in `tests/compliance-data/EXPECTED_FAILS.md`. Each is small
and self-contained; together they should add ~15-25 compliance passes.

### User Request

User asked, after the three-SOW arc closed: "Add the five missing
aggregators."

### Assistant Understanding

Facts:

- The five missing operators are documented in
  `tests/compliance-data/EXPECTED_FAILS.md`:
  - `stddev` (aggregation of instant vector, distinct from the
    `stddev_over_time` rollup we have)
  - `stdvar` (aggregation of instant vector, distinct from
    `stdvar_over_time`)
  - `limitk(k, v)` (Prometheus 2.40+: limit-by-count per group)
  - `limit_ratio(ratio, v)` (Prometheus 2.40+: limit-by-ratio per
    group; deterministic-random selection)
  - `group(v)` (returns 1 for every input series; used as a join
    key fabricator)
- Compliance failures for these show as "unsupported in Phase 1:
  aggregation operator token NN is unrecognized" -- the parser
  surfaces the operator, the lowering layer rejects it.
- `AggrKind` lives in `plan/ir.rs`; `apply_aggregate` dispatches in
  `eval/aggregation.rs`. SOW-0031 made the aggregations grid-aware
  (iterate positions). SOW-0032 made them column-shaped.

Inferences:

- `stddev` and `stdvar` need a 3-component accumulator `(sum,
  count, sum_of_squares)` per bucket per grid position. They're
  the closest cousins of the existing sum/avg path; `Bucket`
  gains two fields.
- `limitk(k, v)` is parametrized like `topk` but the selection
  rule is "first k series in some stable order" rather than
  "top k by value". Stable order = signature (the FNV-1a hash
  we already compute), so the implementation is `sort_by(|a, b|
  a.signature.cmp(&b.signature))` then `truncate(k)` per
  bucket. Per-position iteration is identical to topk's.
- `limit_ratio(ratio, v)` is parametrized with a float. The
  selection rule: compute a deterministic per-series "hash in
  [0, 1)" from the signature; keep the series when
  `hash < ratio` (positive) or `hash >= 1 + ratio` (negative).
  `ratio` NaN or zero -> empty. `ratio` ≥ 1 -> all. `ratio` ≤
  -1 -> all (negative complement).
- `group(v)` is the simplest: per bucket per position, emit
  1.0 if any non-NaN input was seen, NaN otherwise. Output
  series carry the grouping labels (or empty labelset for
  no-grouping case).
- The lowering layer currently maps `promql_parser`'s
  `AggregateOp` enum to our `AggrKind`. Adding five entries to
  that match arm plus five `AggrKind` variants gets the parser
  surface wired.

Unknowns:

- The exact token numbers (53/54/57/58) come from
  `promql_parser`'s internal aggregator-op enum. Need to look at
  the parser's public surface to map them by name; this is a
  trivial inspection.
- Prometheus' exact `limit_ratio` hashing function. Their
  implementation hashes a stable per-series fingerprint into a
  u64 then converts to a [0, 1) float; we have `signature: u64`
  already which is FNV-1a over the sorted label set. Using
  signature directly is deterministic, stable across
  evaluations, and avoids a second hash pass. The selection
  output won't be bit-identical to Prometheus' (different hash
  algorithm) but it satisfies the spec ("deterministic-random
  selection of a fraction of input series").

### Acceptance Criteria

1. Five new `AggrKind` variants: `Stddev`, `Stdvar`, `LimitK`,
   `LimitRatio`, `Group`. `takes_param()` returns true for
   `LimitK` and `LimitRatio`.
2. Lowering layer maps the parser's aggregator names ("stddev",
   "stdvar", "limitk", "limit_ratio", "group") to the new
   variants. Tests verify each lowers without "unrecognized
   operator" errors.
3. `apply_aggregate` dispatches the five new variants to their
   per-position implementations.
4. `stddev` / `stdvar` use a 3-component bucket accumulator
   (sum, count, sum_of_squares) and produce grid-aligned output
   series. Same shape as `apply_collapsing`; same grouping
   semantics; output labels follow the same rule (`__name__`
   stripped; grouping projection retained).
5. `limitk(k, v)` per position per bucket sorts the bucket's
   input series by signature ascending, truncates to k,
   emits the selected series with their original labels minus
   `__name__`. NaN values within a kept series remain NaN; not
   dropped (consistent with `topk`).
6. `limit_ratio(ratio, v)` per position per bucket selects
   input series whose per-series hash maps below `ratio`
   (positive) or into the complement (negative). NaN ratio ->
   empty result. ratio in [-1, 1] selects accordingly; outside
   the range clamps. Output labels: original minus `__name__`.
7. `group(v)` per position per bucket emits 1.0 if any
   non-NaN input was observed, NaN otherwise. Output labels:
   the grouping projection (same rule as sum/avg).
8. Compliance baseline: at minimum 472 preserved; expect an
   improvement of 15-25 cases from the new aggregators (most
   live in `aggregators.test`).
9. 179 existing unit tests pass; add at least one unit test per
   new variant covering: instant input, NaN handling, grouping
   (by/without).
10. 117 smoke checks pass.

Out of scope:

- `limitk` / `limit_ratio` selection-stability across queries
  with different label-sort orders (the underlying signature is
  stable, so this is implicitly handled).
- Histogram-aware aggregators.
- Spec / docs updates beyond the trivial mention in the
  contract spec that these five aggregators are now supported.

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/plan/ir.rs` (`AggrKind`).
- `src/crates/netdata_promql/src/plan/lower.rs` (the
  `AggregateOp` -> `AggrKind` match arm).
- `src/crates/netdata_promql/src/eval/aggregation.rs`
  (`apply_aggregate`, `apply_collapsing`, `apply_topk_bottomk`,
  `Bucket`).
- `tests/compliance-data/EXPECTED_FAILS.md`.
- `tests/compliance-data/aggregators.test` (the cases that
  exercise the new operators).
- promql-parser docs for the `AggregateOp` enum surface.

Current state:

- `Bucket` carries `(labels, sum, count, min, max)`. Adding
  `sum_sq` is one field.
- `apply_aggregate` has 4 dispatch arms (Sum/Avg/Min/Max/Count
  collapsing, TopK/BottomK, Quantile, CountValues). Five new
  variants add one or two more arms (stddev+stdvar share a
  variance-family path; limitk shares topk's
  per-position-truncate pattern; limit_ratio is its own;
  group is its own; the latter two are small).

Risks:

- Lowering tests need new cases verifying each operator name
  lowers cleanly. Small.
- The variance arithmetic (`E[X²] - (E[X])²`) is identical to
  what `compute_over_time::Stddev/Stdvar` already does.
  Reusing the formula keeps consistency.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The compliance corpus surfaces five aggregator names the parser
recognises but our lowering layer rejects with "unsupported
operator token NN". The implementation barrier is purely
absent code; the operators are well-specified and the
infrastructure (per-position aggregation, parametrized
dispatch, grouping projection) is all in place after SOW-0031.

Evidence reviewed:

- `EXPECTED_FAILS.md` (the bug catalogue).
- The Prometheus docs for each operator.
- The existing `apply_topk_bottomk` and `apply_collapsing`
  paths in aggregation.rs.

Affected contracts and surfaces:

- Modified: `plan/ir.rs` (`AggrKind` enum + `takes_param`).
- Modified: `plan/lower.rs` (aggregator name mapping).
- Modified: `eval/aggregation.rs` (dispatch + new
  implementations).
- New: unit tests in `eval/aggregation.rs` test module.
- The compliance corpus, smoke tests, FFI, JSON output --
  unchanged.

Existing patterns to reuse:

- `apply_collapsing`'s per-position bucket loop maps directly
  for stddev/stdvar/group. The accumulator extension and the
  per-bucket finalize logic is local.
- `apply_topk_bottomk`'s per-position bucket+sort+truncate
  pattern maps directly for limitk; just swap the sort key.
- `Bucket::finalize_value` already pattern-matches on
  `AggrKind` -- adding stddev/stdvar arms there keeps the
  trait clean.

Risk and blast radius:

- Five small, well-bounded additions. The dispatch surface
  grows by one or two arms; the implementations are 20-40
  lines each.
- The `LimitRatio` selection uses our existing `signature`
  (FNV-1a over labels) as the per-series hash. Not bit-
  identical to Prometheus' selection, but the spec is
  satisfied (deterministic selection of a ratio of series).
  Compliance tests assert series-count, not series-identity,
  for `limit_ratio`.

Sensitive data handling plan: no sensitive data.

Implementation plan:

1. **IR + lowering.** Add five `AggrKind` variants. Extend
   `AggrKind::takes_param` for `LimitK` and `LimitRatio`. Map
   the parser's aggregator-op enum names to the new variants in
   `lower.rs`. Lowering unit tests pin each name.
2. **Stddev / Stdvar.** Extend `Bucket` with `sum_sq`.
   `accumulate` adds `value * value` to it. `finalize_value`
   gains two new arms using the existing variance formula
   (population variance = E[X²] - (E[X])²; std = sqrt).
   `apply_collapsing` dispatches them naturally.
3. **Group.** New `apply_group` function, mirrors
   `apply_collapsing` but with a 1-or-NaN accumulator (any
   non-NaN seen -> 1; otherwise NaN). Grouping projection
   identical.
4. **LimitK.** New `apply_limitk` function. Per position, per
   bucket, sort series by signature, truncate to k. Output
   carries the original labels minus `__name__` -- same as
   topk's output convention.
5. **LimitRatio.** New `apply_limit_ratio` function. Per
   bucket per series, compute `hash01 = (signature as f64) /
   (u64::MAX as f64)`. Select if `(ratio >= 0.0 && hash01 <
   ratio) || (ratio < 0.0 && hash01 >= 1.0 + ratio)`. NaN
   ratio -> empty. The selection is the *same series across
   all grid positions* (the hash is a series-property, not a
   position-property), which matches Prometheus.
6. **Unit tests.** One per variant: NaN handling, grouping,
   parametrized-param edge cases (k=0/negative for limitk;
   ratio out of range for limit_ratio).
7. **Compliance + smoke.** Re-run both gates. Update
   `EXPECTED_FAILS.md` to remove the "missing aggregators"
   category once the new aggregators are landed.
8. **Commit + close.**

Validation plan:

- Compliance corpus: expect 472 -> ~490 (15-25 case improvement).
- 179 -> ~185 unit tests pass.
- 117 smoke checks pass.
- Spot-check via the daemon: `stddev(system_cpu)`,
  `limitk(3, system_cpu)`, `group by (instance) (system_cpu)`.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: no change to the contract spec (these are
  Prometheus-spec operators; their inclusion is just catching
  up).
- `EXPECTED_FAILS.md`: remove the "missing aggregators"
  category; remaining categories stay.
- End-user docs / skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- Prometheus' `promql/aggregations.go` documents each
  operator's exact semantics; not directly mirrored but
  consulted for `limit_ratio`'s ratio clamping rules.

Open decisions:

- None blocking. Implementation can start.

## Execution Log

### 2026-05-14

- SOW drafted; promoted to current.
- Step 1: added five `AggrKind` variants (`Stddev`, `Stdvar`,
  `LimitK`, `LimitRatio`, `Group`). Extended `takes_param`
  to cover `LimitK` and `LimitRatio`. `aggr_from_token` in
  `lower.rs` maps `T_STDDEV`/`T_STDVAR`/`T_LIMITK`/
  `T_LIMIT_RATIO`/`T_GROUP` from `promql_parser`.
- Step 2: `Bucket` gained `sum_sq`. `Bucket::accumulate`
  adds `value²` per non-NaN observation. `finalize_value`
  gained `Stdvar` and `Stddev` arms using the same
  population-variance formula already used by the
  `stdvar_over_time` / `stddev_over_time` rollups
  (`var = sum_sq/n - (sum/n)²`; clamped to ≥ 0 for
  round-off).
- Step 3: `apply_group` -- per bucket per grid position, emit
  1.0 if any input series had a non-NaN at that position, NaN
  otherwise. All-NaN buckets dropped.
- Step 4: `apply_limitk` sorts each bucket's input-series
  indices by their stripped signature ascending and truncates
  to `k`; selection is position-independent. `apply_limit_ratio`
  computes a deterministic per-series `hash01 = (sig as f64) /
  (u64::MAX as f64)` and keeps series where `hash01 < ratio`
  (positive) or `hash01 >= 1 + ratio` (negative). NaN or zero
  ratio -> empty. The selection is "by series identity"; output
  preserves the original labels minus `__name__`.
- Step 5: 12 new unit tests (one+ per variant covering NaN
  handling, grouping, parametrized edge cases like k=0 and
  ratio out-of-range). Compliance run: aggregators.test
  80 -> 108 passes; total 472 -> 501. EXPECTED_FAILS.md marked
  the missing-aggregators category as closed.
- Live verification on the daemon:
  - `stddev(system_cpu)` -> 1 collapsed series.
  - `stdvar by (dimension) (system_cpu)` -> 10 series.
  - `group by (dimension) (system_cpu)` -> 10 series each
    valued 1.
  - `limitk(3, system_cpu)` -> exactly 3 series.
  - `limit_ratio(0.5, system_cpu)` -> 5 of 10 series.

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) Five new `AggrKind` variants + `takes_param` updated:
  **MET**.
- (2) Lowering maps the parser token IDs: **MET**.
- (3) `apply_aggregate` dispatches the five new variants:
  **MET**.
- (4) `stddev` / `stdvar` via 3-component bucket accumulator:
  **MET**.
- (5) `limitk(k, v)` selection: **MET**.
- (6) `limit_ratio(ratio, v)` hash01-based selection: **MET**.
- (7) `group(v)`: **MET**.
- (8) Compliance ≥ 472 preserved: **EXCEEDED**, 501/299/212
  (+29 cases vs target of 15-25).
- (9) 179 unit tests + new tests pass: **EXCEEDED**, 191/191.
- (10) 117 smoke: **MET**.

### Lessons

- The compliance-driven approach pays off: a one-line catalog
  in EXPECTED_FAILS.md plus 60 minutes of implementation
  yields 29 additional passes. Triaging compliance failures
  by category and addressing them in single-focus SOWs is
  the right cadence.
- Reusing the existing `Bucket` + `apply_collapsing` plumbing
  for stddev/stdvar (vs. building a separate accumulator type)
  kept the code surface tight. The `sum_sq` field is paid for
  by every collapsing-path bucket, but the cost is one f64 add
  per accumulated value -- negligible.
- `limitk` and `limit_ratio` share the same "filter input
  series indices by a per-series predicate, then carry them
  through with original labels minus `__name__`" pattern. The
  predicate is the only difference; selection is
  position-independent for both.

### Followup

- The hash function in `limit_ratio` is our FNV-1a signature
  cast to `[0, 1)`. Prometheus uses a different hash, so the
  exact selected subset differs from a like-for-like
  Prometheus run. Compliance tests assert series-count, not
  series-identity, so this satisfies the spec. If a future
  user query explicitly depends on the exact selected subset
  matching Prometheus, swap the hash for a port of theirs.

## Regression Log

None yet.
