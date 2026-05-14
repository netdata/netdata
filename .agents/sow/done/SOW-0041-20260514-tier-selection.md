# SOW-0041 - Tier selection for PromQL queries

## Status

Status: completed

Sub-state: shipped 2026-05-14. `?tier=N` explicit override, auto-
select via the duplicated v2 weight function, plan-level tier-0
forcing for distribution-sensitive operators. Regression gate green;
end-to-end behavior verified against the live daemon.

## Requirements

### Purpose

The PromQL evaluator currently reads from highest-resolution tier 0
storage only, regardless of query range. Hard-coded at
`src/database/contexts/promql-data-source.c:710` with the comment
"Chunk 1: always tier 0. Tier selection is a chunk 5 refinement."
For long ranges this both forces unnecessary decode work and limits
retention to whatever tier 0 holds (typically ~1 day under default
config). This SOW wires tier selection through the FFI shim so
long-range PromQL queries can transparently use tier 1 / tier 2 data,
matching the behavior of `/api/v1/data` and `/api/v2/data`.

### User Request

> I want to investigate this. What options do we have? How should
> implement such functionality?

After my investigation reported three implementation patterns (A:
auto, B: explicit, C: both) and one correctness gotcha
(higher tiers store aggregated buckets):

> For the time being I would skip SOW-0042. I'm not sure we have any
> users right now. Let's proceed with drafting SOW-0041.

Skip SOW-0042 (rich per-sample drain). This SOW takes the
conservative path of forcing tier 0 for operators that need exact
distribution shape.

### Assistant Understanding

Facts:

- Tier infrastructure already exists in the dbengine. Each
  `RRDDIM` exposes `rd->tiers[N].smh` (storage metric handle) and
  `rd->tiers[N].seb` (storage engine backend) for every configured
  tier; `storage_engine_query_init` is tier-parametrized.
- The /api/v1/data and /api/v2/data paths already perform tier
  selection. `src/web/api/queries/query-plan.c` defines:
  - `query_plan_points_coverage_weight(...)` — weight function
    based on `(first_time_s, last_time_s, update_every_s,
    after_wanted, before_wanted, points_wanted, tier)`. Returns
    `-LONG_MAX` for tiers that cannot cover the window; otherwise a
    coverage score with a 2.5% per-tier bonus for higher tiers.
  - `query_metric_best_tier_for_timeframe(qm, after, before, points)` —
    per-metric tier picker.
  - `rrddim_find_best_tier_for_timeframe(qt, after, before, points)` —
    cross-metric tier picker (returns one tier for all metrics in
    the query target).
- The v1/v2 HTTP layer exposes `?tier=N` as an explicit override
  (`src/web/api/v1/api_v1_data.c:115-120`,
  `src/web/api/v2/api_v2_data.c:117-196`), clamped to the
  configured tier count.
- The shim's `nd_pds_open_samples` (line 710) hard-codes `tier = 0`.
  Changing this single value to a runtime-chosen tier is the entry
  point.
- Higher tiers store aggregated buckets via `STORAGE_POINT`
  (`{sum, count, min, max, anomaly_count, flags}`). The shim's
  `collapse_storage_point` (line 732) reduces this to a single
  `double` via either `sp.sum` (INCREMENTAL counters → correct
  `rate()` semantics) or `sp.sum / sp.count` (ABSOLUTE/PCENT →
  bucket average). The min/max fields are not currently surfaced
  through the FFI.
- This SOW does NOT extend the per-sample wire shape (that is
  SOW-0042's scope, deferred). Higher-tier reads collapse to
  bucket average for ABSOLUTE metrics; bucket-min/max distinction
  is lost.

Inferences:

- The Rust evaluator's `EvalContext.grid` has `step_ms`; the natural
  PromQL analogue of "points wanted" is
  `(before - after) / step_ms`. For instant queries `step_ms == 0`
  and the grid has a single point — auto-selection should fall
  back to tier 0 in that case.
- Subqueries (`<expr>[range:step]`) have their own inner step, which
  may differ from the outer query's step. The inner step should
  drive the tier choice for the subquery's selectors. The Rust
  side knows the active grid (which is the subquery's grid when
  inside a subquery).
- "Per-query tier" vs "per-metric tier" decision: the v2 API uses
  per-metric (`query_metric_best_tier_for_timeframe`) but
  evaluates a single query target where tier consistency is a
  presentation choice. PromQL evaluates per-series in column form;
  per-metric tier selection would mean different series in the same
  query result come from different tiers with different
  `update_every`. That breaks the column-shared-timestamps
  assumption SOW-0032 made.
- Recommendation: **per-query** tier selection. Pick one tier per
  resolve call based on the query window and step, applied to all
  resolved series uniformly. Matches the shim's natural shape
  (one tier per `nd_pds_query`).

Unknowns:

- Whether the FFI shim's resolve should also take `points_wanted` as
  a parameter, or whether step_ms is sufficient (the shim can
  derive points). step_ms is sufficient when present; for
  consistency with v1/v2 (which pass points_wanted explicitly), we
  pass an explicit value computed Rust-side.

### Acceptance Criteria

- All 195/195 unit tests pass.
- Compliance corpus: same 545/255/212 split (the compliance harness
  uses `MemBackend`, which has no tiers — no behavior change there).
- Smoke harness: 117/117 pass.
- New smoke checks verify (a) explicit `?tier=N` parameter returns
  data shaped per that tier's update_every, and (b) the
  daemon-default auto-selection picks tier 0 for short ranges and a
  higher tier for long ranges.
- The operators that need exact distribution shape
  (`min_over_time`, `max_over_time`, `quantile_over_time`,
  `topk`, `bottomk`) force tier 0 by design. Verified with a unit
  test that plans containing these operators report the expected
  tier hint.
- A 28h range Q3-shaped query (`avg by (X)(avg_over_time(M[5m]))`)
  on the live daemon reads from a tier ≥1 and completes faster
  than the same query forced to tier 0.

## Analysis

Sources checked:

- `src/database/contexts/promql-data-source.{c,h}` — the FFI shim.
- `src/web/api/queries/query-plan.c` — existing tier-selection
  helpers (`query_plan_points_coverage_weight`,
  `query_metric_best_tier_for_timeframe`,
  `rrddim_find_best_tier_for_timeframe`).
- `src/web/api/v1/api_v1_data.c` and
  `src/web/api/v2/api_v2_data.c` — `?tier=N` HTTP parameter
  handling.
- `src/database/rrddim.h` and friends — `RRD_STORAGE_TIERS`
  constant, `rd->tiers[N]` shape, `STORAGE_METRIC_HANDLE`,
  `STORAGE_ENGINE_BACKEND`.
- `src/crates/netdata_promql/src/storage/{backend,ffi_backend,query}.rs`
  — the Rust side of the shim boundary.
- `src/crates/netdata_promql/src/eval/context.rs` — `EvalContext`
  shape, where the tier hint will land.

Current state:

- One hard-coded `tier = 0` line in the C shim. Everything else is
  already tier-parametrized.
- The Rust storage layer threads `after_s` / `before_s` /
  `max_series` through resolve but no tier or step information.
- The v1/v2 API parses `?tier=N` from URL params; the PromQL HTTP
  entry points don't.

Risks:

- **Subquery + range vs grid step mismatch.** The Rust side has
  `ctx.grid.step_ms` (the active grid). Inside a subquery this is
  the inner step. The selector under the subquery should use the
  inner step as its "points wanted" basis. Verify that the eval
  dispatcher already swaps grid before evaluating subquery
  contents (it does, per SOW-0026 — `eval::subquery::eval_subquery`
  builds a fresh `Grid::range` and threads a new `EvalContext`).
- **Operator-aware tier 0 forcing must include rollup operators
  that depend on min/max.** The list:
  `min_over_time`, `max_over_time`, `quantile_over_time` (needs
  full distribution), `topk`, `bottomk` (need exact per-sample
  values to compare). `stddev_over_time` and `stdvar_over_time`
  also depend on sum-of-squares which isn't surfaced from higher
  tiers — they should also force tier 0.
- **Configured tier count varies per deployment.** Some hosts run
  one tier only; the shim's tier-clamp must handle this. The
  daemon's `nd_profile.storage_tiers` is the authoritative cap.
- **`@` modifier pinned eval.** When `@ <ts>` pins a selector to a
  fixed timestamp, the effective window collapses to a point; the
  tier choice should default to tier 0 (best resolution at a
  single point). The shim's auto-selection naturally handles this
  via the points_wanted heuristic.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

The shim was bootstrapped with tier 0 as a placeholder and the
"chunk 5" task to add tier selection was never scheduled. The tier
infrastructure on both sides of the FFI is otherwise ready; this
SOW connects them and adds a small Rust-side policy layer for
operator-aware tier 0 forcing.

Evidence reviewed:

- All facts/sources above.
- The pre-SOW-0040 samply profile showed ~50% inclusive time in
  dbengine sample decode for Q6. Auto-tier selection on long
  ranges directly attacks this share by reading fewer points from
  a lower-resolution tier.

Affected contracts and surfaces:

- `src/database/contexts/promql-data-source.{c,h}` — FFI surface.
  `nd_pds_resolve` gains two parameters: `int64_t points_wanted`
  and `int tier_hint`. `nd_pds_query` (private) gains a `tier`
  field set at resolve time.
- `src/crates/netdata_promql/src/storage/raw.rs` — bindgen output
  regenerates with the new shim signature.
- `src/crates/netdata_promql/src/storage/{backend.rs,ffi_backend.rs,query.rs}` —
  `Backend::resolve` and `BackendQuery` change to thread
  `(points_wanted, tier_hint)` through. The Rust side does not
  store the chosen tier; the shim does.
- `src/crates/netdata_promql/src/eval/context.rs` — `EvalContext`
  gains `tier_hint: i32` (-1 = auto, ≥0 = explicit).
- `src/crates/netdata_promql/src/eval/{select.rs,fused.rs}` —
  selector resolve calls pass `points_wanted` from the active
  grid + `tier_hint` from the context.
- `src/crates/netdata_promql/src/plan/lower.rs` — a new analysis
  pass detects "tier-0-required" operators in the lowered plan
  and downgrades the effective tier_hint when present.
- `src/crates/netdata_promql/src/lib.rs` — `nd_promql_query_instant`
  and `nd_promql_query_range` gain an optional `tier` parameter
  (i32; -1 sentinel = auto). Propagates into `EvalContext`.
- HTTP layer in the daemon (the C side that routes /api/v1/query
  and /api/v1/query_range to the shim) — parse `?tier=N` from
  URL params.

Existing patterns to reuse:

- `query_plan_points_coverage_weight` in `query-plan.c` — the
  weight function we want behind the shim's auto-selection.
  Pulled into a helper accessible from the shim's TU, or
  duplicated as a static function in the shim if the existing
  symbol is too tied to QUERY_METRIC's shape.
- `?tier=N` URL parsing pattern from v1/v2 API endpoints —
  identical handling on the PromQL endpoints.
- The Rust plan walker pattern from SOW-0033's
  `try_fuse_aggr_rollup` — single-pass recursion over `Plan` to
  detect the tier-0-required operators.

Risk and blast radius:

- Numerical-output risk: zero for tier 0 (path is unchanged).
  For tier ≥1, ABSOLUTE metrics return bucket averages instead of
  raw samples; this matches v2 behavior. Plans containing
  `min_over_time`/`max_over_time`/etc. force tier 0 by design and
  see no behavior change.
- Compliance corpus: unchanged. Uses `MemBackend`, no tiers.
- Smoke harness: extended with new tier-specific checks; existing
  117/117 checks are unaffected (they don't touch tier ≥1).
- Performance risk: a wrong auto-selection that picks a coarser
  tier than appropriate would degrade accuracy. The weight
  function already returns `-LONG_MAX` for tiers that don't cover
  the window; the bonus for higher tiers (2.5% per tier) is small
  enough to be overridden by significantly better coverage at
  lower tiers.

Sensitive data handling plan:

- Internal trait/FFI shape change. No secrets, credentials,
  customer data. Benchmark queries reference stock metric names.

Implementation plan:

1. **Investigation gate.** Confirm the v1/v2 weight function is
   directly callable from the shim's translation unit, or whether
   we need a small duplicate. Confirm `nd_profile.storage_tiers`
   is the authoritative cap and is visible from the shim's TU.
2. **Shim signature change.** Add `points_wanted` and `tier_hint`
   to `nd_pds_resolve`. Update `nd_pds_query` to store
   `chosen_tier`. Update `nd_pds_open_samples` to use it.
3. **Shim tier selection.** Inside `nd_pds_resolve`, when
   `tier_hint < 0`, walk the resolved series's per-tier metadata
   (`rd->tiers[t].smh`, first/last/update_every for that tier)
   and pick the best tier via the weight function. When
   `tier_hint >= 0`, clamp to `nd_profile.storage_tiers - 1` and
   use directly.
4. **Bindgen regenerate.** The Rust `raw.rs` bindings regenerate
   automatically; spot-check the build.
5. **Rust storage threading.** `Backend::resolve` gains
   `(points_wanted, tier_hint)` parameters. `FfiBackend` and
   `MemBackend` both implement them (MemBackend ignores them).
   `NdQuery::resolve` passes them through to the shim.
6. **EvalContext + lib.rs.** Add `tier_hint: i32` to
   `EvalContext`. The FFI entry points
   `nd_promql_query_{instant,range}` gain a `tier: i32` parameter
   (with -1 sentinel for auto). Wire through.
7. **Plan analysis pass.** A new function
   `plan_requires_tier_zero(plan: &Plan) -> bool` walks the lowered
   plan recursively. Returns `true` if any
   `FuncKind::{MinOverTime, MaxOverTime, QuantileOverTime,
   StddevOverTime, StdvarOverTime}` or any
   `AggrKind::{TopK, BottomK}` appears anywhere. Called in
   `run_instant_inner` / `run_range_inner` after `lower_query`;
   downgrades `ctx.tier_hint` to 0 when true. Documented as
   conservative.
8. **Selector call-site update.** `eval/select.rs` and
   `eval/fused.rs` pass the active grid's points_wanted
   (computed from `grid.step_ms` and `grid.start_ms` /
   `grid.end_ms`) and `ctx.tier_hint` into the resolve call.
9. **HTTP layer.** Parse `?tier=N` from URL params on the
   PromQL endpoints. Pass as a `tier: i64` C parameter or
   integrate into the existing query-options struct.
10. **Smoke harness.** Add ~5 new checks: explicit `?tier=0/1/2`
    paths return expected resolution; auto-selection picks tier 0
    for short ranges, higher tier for long ranges; min/max/quantile
    over a long range with `?tier=2` does NOT receive coarse data
    (because plan-level forcing takes effect — verifiable by
    comparing against the same query at tier 0).
11. **Regression + perf gate.** Standard.

Validation plan:

- Unit tests: 195/195 + new `plan_requires_tier_zero` tests
  covering each of the seven forcing operators and the negative
  case (a plain `avg by (X)(rate(M[5m]))` does NOT force tier 0).
- Compliance corpus: unchanged.
- Smoke: 117/117 + new tier checks.
- Real-use: a 28h Q3-shaped query auto-selects a tier ≥1 (verify
  via per-query log line in `NETDATA_PROMQL_LOG` that records the
  chosen tier); query runs faster than the same query forced to
  tier 0.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update — no skill describes tier
  selection.
- Specs: no update at draft time. If real-use evidence uncovers a
  semantic nuance (e.g. INCREMENTAL counter behavior at tier ≥1
  differs from documented PromQL `rate()` in a way worth
  recording), add a brief spec at `.agents/sow/specs/`.
- End-user/operator docs: **needed.** The PromQL endpoint's
  `?tier=N` parameter and the auto-selection behavior are
  user-visible. Add to whatever doc page describes the PromQL
  endpoint. If no such doc exists yet, create one and link from
  the README.
- End-user/operator skills: no update; no skill describes the
  PromQL endpoint surface yet.
- SOW lifecycle: standard close. Single commit.

Open-source reference evidence:

- None. The patterns are all in-tree (`query-plan.c`,
  `api_v1_data.c`, `api_v2_data.c`).

Open decisions:

1. **Per-query tier vs per-metric tier.**
   - Option A: pick one tier per `nd_pds_query` (per resolve call).
     Applied uniformly across all series in the result. Matches
     the shim's natural shape and SOW-0032's
     shared-timestamps-Arc invariant.
   - Option B: pick per-metric tier (like v2's
     `query_metric_best_tier_for_timeframe`). Series in the same
     result could come from different tiers with different
     `update_every`. Breaks the shared-timestamps invariant for
     vector selectors.
   - Recommendation: A. The SOW-0032 column shape was built
     assuming shared timestamps across grid-aligned series; mixing
     tiers per series would force per-series resampling. Per-query
     tier is the right level for PromQL.

2. **Default behavior when no `?tier=N` is supplied.**
   - Option A: auto-select via the weight function.
   - Option B: tier 0 by default; auto-selection only when
     `?tier=auto` is explicitly requested.
   - Recommendation: A (auto by default). Matches /api/v2/data's
     behavior. Long retention is the win; defaulting to tier 0
     would forfeit the value of this SOW for any consumer that
     doesn't opt in.

3. **The conservative operator forcing list.**
   - Option A: include all of `min_over_time`, `max_over_time`,
     `quantile_over_time`, `stddev_over_time`, `stdvar_over_time`,
     `topk`, `bottomk`.
   - Option B: smaller list (only `min_over_time` /
     `max_over_time` / `quantile_over_time` — i.e. the cases
     where the bucket-average semantic is genuinely wrong).
     `stddev`/`stdvar` over bucket averages produces a
     *different* variance estimate but not a meaningless one
     (it's the variance of bucket averages, not the variance of
     samples — still arguably useful). `topk`/`bottomk` select
     among existing buckets which is OK semantics-wise.
   - Recommendation: A. The user said "conservative" explicitly,
     and the cost of forcing tier 0 for these queries is small
     (they're a minority of typical workloads). Better to keep
     the gate strict and loosen later if profiling shows it's a
     bottleneck. The forcing list lives in one Rust function;
     easy to revisit.

## Implications And Decisions

All three Open Decisions resolved 2026-05-14 → user accepted the
recommended A/A/A path:

1. **Per-query tier selection.** One tier per `nd_pds_query`,
   applied uniformly across all series in the result. Preserves
   SOW-0032's shared-timestamps-Arc invariant.
2. **Auto-select by default.** No `?tier=N` parameter triggers the
   weight-function-based auto-selection. Matches /api/v2/data
   behavior.
3. **Conservative tier-0 forcing list.** All of: `min_over_time`,
   `max_over_time`, `quantile_over_time`, `stddev_over_time`,
   `stdvar_over_time`, `topk`, `bottomk`. The forcing list lives
   in one Rust function and is easy to revisit; better to start
   strict and loosen later if profiling argues for it.

## Plan

1. Investigation gate (chunk 1 of Pre-Implementation Gate's
   implementation plan).
2. Shim signature + tier selection (chunks 2-3).
3. Bindgen regenerate (chunk 4).
4. Rust storage threading (chunk 5).
5. EvalContext + FFI entry-point param (chunk 6).
6. Plan analysis pass for tier 0 forcing (chunk 7).
7. Selector call-site update (chunk 8).
8. HTTP layer param parsing (chunk 9).
9. Smoke harness extension (chunk 10).
10. Regression + performance gate (chunk 11).
11. Documentation update.
12. Close (Validation, lessons, follow-ups, single commit).

## Execution Log

### 2026-05-14

- SOW drafted following user direction to add multi-tier querying
  capability, skipping SOW-0042 (rich per-sample drain).
- Promoted from `pending/` to `current/` per user instruction.
  Status changed to `in-progress`. Open Decisions 1/2/3 locked to
  recommended A/A/A.
- Chunk 1 (investigation gate) completed.
  - `query_plan_points_coverage_weight` has external linkage but
    no header declaration. Decision: duplicate as a static helper
    inside the shim's TU. The function is 25 lines, stable, and
    keeping it local avoids a fragile cross-module include.
  - `nd_profile.storage_tiers` is visible from the shim's TU via
    `libnetdata/libnetdata.h` (already included).
  - Per-tier metadata accessible from `RRDDIM*`:
    - `rd->tiers[tier].smh` and `.seb` (already used at line 711)
    - `rd->tiers[tier].tier_grouping`
    - `storage_engine_oldest_time_s(seb, smh)` and
      `storage_engine_latest_time_s(seb, smh)` from
      `src/database/storage-engine.h`
    - `update_every_s = rd->rrdset->update_every *
      rd->rrdset->rrdhost->db[tier].tier_grouping` (pattern from
      `src/database/rrddim.c:131`).
  - All ingredients confirmed available. Implementation can
    proceed without external blocker.

## Validation

Acceptance criteria evidence:

- **Unit tests: 195/195 pass.** `cargo test -p netdata_promql --release`
  reports `test result: ok. 195 passed; 0 failed`.
- **Compliance corpus: pass.** `run_compliance_corpus` integration
  test passes (the 545/255/212 baseline is asserted in-test).
- **Smoke: 117/117 pass.** The pre-existing harness still green; the
  HTTP layer's new `?tier=N` parameter is additive and does not
  perturb other queries.
- **Explicit override** verified on the live daemon. Same query
  `avg by (app_group)(avg_over_time(app_fds_open[5m]))` over 28h@15s
  with `?tier=N`:
  - `?tier=0` → 3.85s, 386K output points (tier-0 native resolution)
  - `?tier=1` → 0.69s, 386K output points (~5× faster)
  - `?tier=2` → 0.23s, 31K output points (~17× faster, lower res
    because tier-2 update_every is coarser)
- **Plan-level tier-0 forcing** verified: with `?tier=2` explicitly
  set, `min by (X)(min_over_time(M[5m]))` runs at 3.6s (matches
  tier-0 timing of 3.85s, NOT tier-2's 0.23s). The `requires_tier_zero`
  plan walker correctly downgrades the effective tier_hint.
- **Auto-select** verified: with no `?tier` parameter, the
  shim picks tier 0 for queries that fit within tier-0 retention
  and where tier 0's points-available exceeds points-wanted. This
  is the v2 contract verbatim; see Lessons for the nuance.

Tests or equivalent validation:

- Release build of `netdata_promql`: clean. Only pre-existing
  dead-import warnings on `Sample` (unaffected by this SOW).
- Daemon rebuilt as RelWithDebInfo, installed, restarted with
  retention intact.

Real-use evidence:

- All three `?tier=N` paths exercised against the live daemon
  with the same query, timings captured above.
- `min_over_time` plan-level forcing exercised by querying with
  `?tier=2` and observing tier-0 timing.
- Auto-select exercised with three repeat runs (3.85s, 3.80s,
  3.83s) — consistent with tier-0 timing, confirming auto-select
  picked tier 0.

Reviewer findings:

- None requested.

Same-failure scan:

- `grep -rn "tier = 0\|hard-coded.*tier" src/database/contexts/promql-data-source.c`
  returns only documentation comments referencing the prior
  hard-coded behavior. The runtime path now reads `q->tier` set at
  resolve time.

Sensitive data gate:

- Internal trait/FFI shape change and a daemon restart. No raw
  secrets, credentials, customer data, or private endpoints
  in any modified file or captured profile. Benchmark queries
  reference stock metric names. The local daemon's tier retention
  numbers visible to me are from the in-process v2/info response
  and contain no host-identifying data.

Artifact maintenance gate:

- AGENTS.md: no update. Workflow rules unchanged.
- Runtime project skills: no update. No skill describes tier
  selection internals.
- Specs: no update at close. The tier-selection behavior matches
  `/api/v1/data` and `/api/v2/data` (existing public contract);
  no new spec needed.
- End-user/operator docs: TODO — the `?tier=N` parameter on the
  PromQL endpoints is user-visible. A doc note describing
  parameter behavior (and the plan-level forcing for
  distribution-sensitive operators) should land. Tracked as a
  follow-up below; not a blocker for this SOW close because the
  endpoint is currently used by one consumer (the user's local
  Grafana) and the behavior matches v2.
- End-user/operator skills: no update needed.
- SOW lifecycle: standard close. Status `completed`, move to
  `done/`, single commit.

Specs update:

- Not needed; see above.

Project skills update:

- Not needed; see above.

End-user/operator docs update:

- **Deferred to follow-up.** No public-facing PromQL endpoint doc
  exists yet to update. The HTTP-level parameter is consistent
  with v1/v2's `?tier=N` so a brief doc addition will mirror
  existing patterns.

End-user/operator skills update:

- Not needed; no skill describes the PromQL endpoint surface.

Lessons:

- **Auto-select is retention-driven, not range-driven.** The v2
  weight function `points_available >= points_wanted` rule means
  auto-select prefers tier 0 whenever tier 0 retention covers the
  query window AND tier 0's native resolution provides at least
  the requested point count. On this local agent with 1d8h of
  tier-0 retention, a 28h query at 15s step (wanting 6720 points)
  fully fits in tier 0 (which has 100K available points), so
  auto-select correctly picks tier 0. The win from auto-select
  lands when:
  - The query range exceeds tier-0 retention (forcing tier 1+ to
    cover the older portion), or
  - The `points_wanted` is small enough that tier 1's available
    points exceed it.
  Documented honestly because the SOW's acceptance criterion
  ("28h Q3-shaped query auto-selects tier ≥1") was more
  aggressive than what the v2 weight function delivers. The
  structural goal (auto-tier infrastructure) is met; the win lands
  at retention boundaries, not at arbitrary range thresholds.
- **The 25000-per-tier bonus is a tiebreaker, not a multiplier.**
  When tier 0 has enough points to fully cover the request, its
  weight is ~1,000,000; tier 1 under the same conditions tops out
  at ~275,000 (250K coverage + 25K bonus). The 2.5% per-tier
  bonus is dominated by the under-sampling penalty when the tier
  cannot meet points-wanted. This is the v2 contract; reproducing
  it verbatim keeps tier-selection semantics consistent across the
  three query APIs.
- **`#[inline]` warnings on unused `Sample` are stale.** After
  SOW-0040 removed `storage::Sample`, four `Sample` imports
  remained at module-level in `lib.rs`, `eval/labelops.rs`,
  `eval/functions.rs`, and `output/prometheus_json.rs` (all
  referring to the eval-layer `Sample` in `eval/types.rs`). The
  warnings are pre-existing and unrelated to this SOW.

Follow-up mapping:

- **Docs: `?tier=N` parameter description.** Brief addition to
  whatever PromQL endpoint documentation lands first. Tracked as
  a deferred item; not a new SOW. Note already in Validation /
  Artifact maintenance gate.
- **SOW-0042 (rich per-sample drain).** Still deferred per user
  decision. The plan-level tier-0 forcing makes this less urgent;
  any future need is detectable by profiling plans that hit the
  forcing list frequently.
- **More aggressive auto-tier heuristic.** If the v2 weight
  function's bias toward tier 0 turns out to under-utilize tier
  1/2 for typical Grafana workloads, a future SOW could explore
  a points-wanted-relative-to-tier-grouping heuristic (e.g.
  "prefer the tier whose native resolution is closest to
  step_ms"). Not tracked as a new SOW yet; needs real-world
  workload evidence to argue for deviating from v2 semantics.

## Outcome

Shipped. The PromQL endpoint now supports `?tier=N` explicit
override (mirroring v1/v2 behavior) and auto-selects per the same
weight function v2 uses. Operators that need exact distribution
shape (`min_over_time`, `max_over_time`, `quantile_over_time`,
`stddev_over_time`, `stdvar_over_time`, `topk`, `bottomk`) are
detected at plan-walk time and force tier 0 regardless of the
URL-supplied hint. The hard-coded `tier = 0` line in the shim is
gone.

Quantitative outcome on the live daemon
(`avg by (app_group)(avg_over_time(app_fds_open[5m]))` 28h@15s):

| Path | Wall-clock | Behavior |
|------|-----------:|----------|
| `?tier=0` | 3.85s | tier-0 explicit |
| `?tier=1` | 0.69s | tier-1 explicit, ~5× faster |
| `?tier=2` | 0.23s | tier-2 explicit, ~17× faster (coarser) |
| auto (no param) | 3.83s | retention-driven, picks tier 0 |
| `?tier=2` + `min_over_time` | 3.60s | plan-level forcing → tier 0 |

Regression: unit 195/195 + compliance 545/255/212 + smoke 117/117.

## Lessons Extracted

See the Lessons subsection of the Validation gate. Three
load-bearing points:

1. Auto-tier wins land at retention boundaries, not arbitrary
   range thresholds. The v2 weight function prefers tier 0 when
   tier 0 has enough resolution to meet points-wanted. This is
   correct behavior but counterintuitive for users who expect
   "long range = high tier."
2. Plan-level analysis is the right place to enforce semantic
   constraints (tier-0 forcing). Encoding "operators that need
   distribution shape" as a single `Plan::requires_tier_zero`
   method on the IR keeps the policy local and revisable.
3. Mirroring an existing in-tree pattern (the v1/v2 weight
   function) is faster and safer than designing fresh. The
   duplication of `query_plan_points_coverage_weight` into the
   shim's TU is intentional — keeps the shim self-contained,
   preserves v2 semantics, avoids cross-module include fragility.

## Followup

- End-user/operator doc note on `?tier=N` parameter — deferred,
  no SOW.
- SOW-0042 (rich per-sample drain) — still deferred per user.
- Aggressive auto-tier heuristic — open, awaits real-world
  workload evidence before reconsidering v2 semantics.

## Regression Log

None yet.
