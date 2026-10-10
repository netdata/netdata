# SOW-0042 - Engine-driven tier planning + splicing for PromQL

## Status

Status: open

Sub-state: drafted after the SOW-0041 close conversation surfaced
that the auto-select policy I shipped duplicates infrastructure
already battle-tested in the v1/v2 data path. The right move is to
reuse the engine's `query_plan()` rather than maintain a parallel
implementation.

## Requirements

### Purpose

Replace the SOW-0041 auto-select policy with a thicker shim that
reuses the existing query-engine infrastructure for per-metric tier
selection and multi-tier splicing. The shim retains its
native-sample drain interface (Rust still performs all PromQL
operator semantics in column form); the only change is which tier(s)
the samples are read from.

The structural pieces SOW-0041 shipped — explicit `?tier=N`
override, plan-level tier-0 forcing via `Plan::requires_tier_zero`,
FFI plumbing for `tier_hint` and `points_wanted` — all stay. This
SOW touches only the shim-internal auto-select path and the
sample-iteration state machine.

### User Request

After investigating how `/api/v2/data` handles tier selection,
grouping, and heterogeneous metrics, the user asked:

> I'm curious if all these issues/complexity means that we should
> explore the idea of exporting the higher-level query logic that
> the agent uses for its existing HTTP API.

The investigation showed that the engine's tier-planning (concerns
1 and 2: tier selection + splicing) is a clean reuse target. The
engine's grouping/aggregation (concern 3) cannot reuse because
PromQL needs per-grid-point sliding-window rollups with
per-operator reductions — a different semantic from the engine's
non-overlapping bucket grouping. So the proposed split is:
**engine handles tier planning + sample iteration; Rust handles
grid alignment + operator semantics**.

Followed by: "Yes, draft this as a new SOW-0042."

### Assistant Understanding

Facts:

- `src/web/api/queries/query-plan.c` defines:
  - `query_plan_points_coverage_weight(...)` — the weight function.
  - `query_metric_best_tier_for_timeframe(qm, after, before, points_wanted)` —
    per-metric tier picker.
  - `rrddim_find_best_tier_for_timeframe(...)` — cross-metric picker.
  - `query_plan(ops, after, before, points_wanted)` — builds the
    multi-tier plan per `QUERY_METRIC` with splicing at retention
    boundaries; padded with `POINTS_TO_EXPAND_QUERY` at transitions.
  - `query_planer_initialize_plans`, `query_planer_activate_plan`,
    `query_planer_finalize_remaining_plans` — the plan iterator
    state machine.
- All these functions have external linkage but **no header
  declaration**. To use them from the PromQL shim, we either expose
  them via a new header or `extern`-declare them locally.
- `QUERY_METRIC` carries per-tier metadata
  (`db_first_time_s`, `db_last_time_s`, `db_update_every_s`,
  `smh`, `seb`, `weight`) and the plan array (`plan.array[]`,
  `plan.used`).
- The current PromQL shim (`promql-data-source.c`) builds its own
  `nd_pds_series_record` per series with cached
  `(algorithm, multiplier, divisor)`. It does **not** populate a
  `QUERY_METRIC`-shaped tier-metadata array per series.
- The SOW-0041 `shim_pick_tier` walks all series, aggregates
  per-tier metadata via MIN/MAX/MIN rule, picks one tier per
  query. Per-query, no splicing, log-distance not implemented (the
  user's pushback after the close).
- `nd_pds_samples` (the sample iterator) currently holds one
  `storage_engine_query_handle` plus cached metadata. It iterates a
  single tier from start to finish.
- The engine's plan iterator carries `expanded_after`,
  `expanded_before`, `current_plan`, plus a `handle` per plan
  entry. Plan transitions are driven by
  `query_planer_activate_plan`.

Inferences:

- Per-metric tier planning is the practical 90% case for
  heterogeneous deployments (different metrics with different
  retention, parent agents aggregating children). v2 has handled
  this for years.
- Splicing matters for forensic-zoom fidelity: the recent portion
  of a long query gets the finest-tier resolution that exists for
  that subwindow, rather than degrading to whatever tier covers the
  full window. SOW-0041 conversation flagged this as the real value
  of v2's approach.
- Reusing engine internals from the shim couples the shim's TU to
  the engine's evolution. We accept this coupling because the
  alternative — duplicating ~300 lines of weighting + planning +
  splicing logic — would diverge over time and become its own
  maintenance burden.

Unknowns:

- The engine's plan iterator depends on `QUERY_ENGINE_OPS` and
  `RRDR` types that carry orchestration state beyond what the shim
  needs (group method, view, time grouping, etc.). The shim must
  either construct a sufficient subset of these or extract the
  pure-plan path into a leaner helper. This is the largest
  unknown; it shapes the implementation plan and is the
  Pre-Implementation Gate's first investigation.

### Acceptance Criteria

- All 195/195 unit tests pass.
- Compliance corpus: same 545/255/212 split (the harness uses
  `MemBackend`, no tier semantics).
- Smoke harness: 117/117 pass.
- A heterogeneous-metric query (two metrics with different
  per-tier retention) on the live daemon serves each metric from
  its appropriate tier. Verifiable via per-series tier in the
  slow-query log.
- A long-range query whose window crosses tier-0's retention
  boundary stitches the older portion from tier 1+ and the
  recent portion from tier 0 for the same metric. Verifiable by
  inspecting the sample-by-sample timestamp density at the
  boundary.
- Auto-select for queries fully within tier-0 retention may still
  pick tier 0 (matches v2 exactly). The user-perceived
  improvement lands at retention boundaries and on heterogeneous
  metrics, both of which v2 already exhibits.
- A query containing `min_over_time` still forces tier 0 across
  every series (plan-level forcing unchanged).
- Explicit `?tier=N` still pins all series to that tier (clamped to
  configured count).

## Analysis

Sources checked:

- `src/web/api/queries/query-plan.c` — the full per-metric planner
  and splicing logic.
- `src/web/api/queries/query.c` — the eval driver that consumes
  the plan.
- `src/web/api/queries/query-internal.h` — the `QUERY_ENGINE_OPS`
  shape that drives plan iteration.
- `src/web/api/queries/query-window.c` — the points_wanted /
  group / update_every computation that feeds the planner.
- `src/database/contexts/query_target.c` — how `QUERY_TARGET`
  populates per-tier metadata per metric.
- `src/database/contexts/promql-data-source.c` — the current shim,
  to know what changes.
- `src/database/storage-engine.h` — the storage iterator interface
  (`storage_engine_query_init`, `_next_metric`, `_is_finished`,
  `_finalize`).

Current state:

- SOW-0041 shipped per-query tier selection with a faithful copy
  of the v2 weight function. The auto-select picks one tier per
  resolve call and applies it uniformly.
- The Pre-Implementation Gate of SOW-0041 explicitly chose
  per-query over per-metric (Option A on Open Decision 1),
  citing SOW-0032's shared-timestamps invariant. That citation
  was wrong — RangeVector results already carry per-series
  timestamps, and InstantVector results' shared grid is synthetic
  and tier-agnostic. Per-metric tier selection is fully compatible
  with the column shape.

Risks:

- **Coupling**: the shim's TU starts to depend on engine internals.
  Mitigation: limit the surface area to the few `query_plan*`
  symbols, exposed via a deliberate header file. Document the
  dependency.
- **State-machine complexity**: the plan iterator switches between
  tiers mid-stream and applies boundary padding. The shim's
  sample iterator must replicate this state. Mitigation: reuse
  the engine's `query_planer_*` functions wholesale rather than
  re-implementing.
- **Per-operator rollup-window constraint**: a query containing
  `rate(M[5m])` cannot tolerate a tier whose `update_every > 5m`
  (zero or one sample per window). The Rust plan walker must
  compute `min_rollup_window_ms` and pass it to the shim. The
  shim's plan selection must reject tiers with
  `update_every > rollup_window`.
- **The engine's plan iterator returns aggregated `STORAGE_POINT`
  values** with `(min, max, sum, count)`. The shim's
  `collapse_storage_point` already reduces this to one double;
  that semantic is unchanged.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

The SOW-0041 auto-select policy duplicates 25 lines of weighting
logic, picks one tier per query, and doesn't splice. The engine
already has the right code, hardened for years against v1/v2
workloads. Reuse beats duplication. The mechanical change is
plumbing the engine's planner into the shim's resolve path; the
semantic change is per-metric tier + splicing.

Evidence reviewed:

- All facts/sources above.
- SOW-0041 close conversation and the user pushback on per-query
  semantics.
- Walkthrough of `query_plan()` showing the splicing state
  machine and boundary padding logic.

Affected contracts and surfaces:

- `src/database/contexts/promql-data-source.c` —
  `shim_pick_tier` and `shim_tier_weight` removed. `nd_pds_query`
  series records gain per-series plan state. `nd_pds_open_samples`
  becomes plan-aware. `nd_pds_samples_next` iterates across plan
  entries with tier transitions.
- `src/database/contexts/promql-data-source.h` —
  `nd_pds_resolve` gains `int64_t rollup_window_ms`. Other
  signatures unchanged.
- `src/web/api/queries/query-plan-public.h` (new) — exposes the
  engine's tier-planning helpers (the exact set to be determined
  in Implementation chunk 1).
- `src/crates/netdata_promql/src/storage/{backend,ffi_backend,
  mem_backend,query}.rs` — `Backend::resolve` and `NdQuery::resolve`
  gain `rollup_window_ms`. `MemBackend` ignores it.
- `src/crates/netdata_promql/src/eval/context.rs` — no change
  (the `tier_hint` field already exists; rollup window is
  computed per call, not stored).
- `src/crates/netdata_promql/src/plan/ir.rs` — new
  `Plan::min_rollup_window_ms() -> Option<i64>` walks the IR and
  returns the smallest `range_ms` among MatrixSelect and
  Subquery nodes. `None` when no matrix selector is present
  (instant-vector-only query — no rollup constraint).
- `src/crates/netdata_promql/src/eval/{select,fused}.rs` —
  callers compute and pass `rollup_window_ms`.
- `src/crates/netdata_promql/src/lib.rs` — `run_*_inner` walks
  the plan once for `requires_tier_zero` AND `min_rollup_window_ms`,
  passes both into resolve calls.
- `src/web/api/v3/api_v3_promql.c` — no change (FFI signature
  unchanged on the C-to-Rust side; only the internal shim resolve
  signature gains the rollup window).
- No HTTP/JSON/wire change.

Existing patterns to reuse:

- `query_plan` in `query-plan.c` — the entire per-metric splicing
  planner. Reused verbatim where possible; adapted where the
  shim's data shape differs from `QUERY_METRIC`.
- `query_planer_initialize_plans`, `query_planer_activate_plan`,
  `query_planer_finalize_remaining_plans` — the iterator state
  machine. Same.
- `query_metric_best_tier_for_timeframe` — single-metric tier
  picker. Same.
- The boundary-padding constant `POINTS_TO_EXPAND_QUERY` —
  reused as-is.
- The v2-faithful weight function — reused, NOT replaced with
  the log-distance variant the SOW-0041 conversation discussed.
  Matching v2 keeps the cross-API contract consistent: a user
  expecting v2 behavior on v1/v2 endpoints gets the same tier
  choice on the PromQL endpoint. If the v2 weight function turns
  out to be wrong for PromQL workloads in practice, that is a
  separate calibration question affecting all three APIs and
  should be addressed at the engine level.

Risk and blast radius:

- Numerical-output risk: zero for queries that previously picked
  tier 0 and stay on tier 0. For queries that newly pick tier 1+
  (because per-metric or splicing applies), the storage values
  come from bucket aggregates. ABSOLUTE metrics return bucket
  averages (same as v2); INCREMENTAL counters return per-bucket
  deltas (same as v2). Compliance corpus uses `MemBackend` with
  no tiers, so no drift there.
- Smoke harness: extended with multi-tier checks. Existing
  checks unaffected.
- Performance risk: per-metric planning costs O(N_series × N_tiers)
  in the resolve path. For a typical 1000-series query with 3
  tiers, that's 3000 weight evaluations — negligible.
- Compilation risk: the new header brings the engine's TU into
  the shim's. Header hygiene is important; the header should
  expose only the functions the shim needs.

Sensitive data handling plan:

- Internal C and Rust changes. No secrets, credentials, customer
  data, or private endpoints. Benchmark queries reference stock
  metric names. The new header documents internal APIs only; no
  public ABI change.

Implementation plan:

1. **Investigation gate**: catalog the exact engine symbols the
   shim needs. Confirm `query_plan` can run against a
   shim-allocated `QUERY_METRIC` or whether the shim needs its
   own leaner planner that reuses only
   `query_plan_points_coverage_weight` and the boundary-padding
   helpers. Two sub-options:
   - **(a)** Allocate enough `QUERY_METRIC` per series record to
     drive `query_plan` directly. The shim populates per-tier
     metadata and lets the engine do everything.
   - **(b)** Extract the pure planner logic into a leaner helper
     (`query_plan_for_metric(per_tier_meta[], n_tiers, after,
     before, points_wanted, rollup_window_ms, out_plan)`) and
     have the shim call that. Avoids dragging `QUERY_TARGET` /
     `RRDR` into the shim's TU.
   Recommendation deferred to the gate.
2. **Header exposure**: create
   `src/web/api/queries/query-plan-public.h` declaring the chosen
   symbols. Update `query-plan.c` to include it (no behavior
   change to v1/v2). Update CMake if needed for include paths.
3. **Shim resolve**: change `nd_pds_resolve` signature to add
   `int64_t rollup_window_ms`. The shim builds per-series plan
   data (using sub-option from step 1) instead of one
   per-query `tier`. Plan-level tier-0 forcing comes through
   `tier_hint=0`.
4. **Shim sample iteration**: change `nd_pds_samples` to carry a
   per-series plan iterator. `nd_pds_open_samples` initializes
   the iterator at plan entry 0; `nd_pds_samples_next` walks
   samples within an entry and advances to the next when
   exhausted, switching `storage_engine_query_handle` per
   transition.
5. **Bindgen**: regenerates `raw.rs` automatically.
6. **Rust storage layer**: add `rollup_window_ms` to
   `Backend::resolve`, `BackendQuery::drain_samples` (no — drain
   doesn't change; the parameter lives on resolve only),
   `FfiBackend::resolve`, `MemBackend::resolve` (ignored), and
   `NdQuery::resolve`.
7. **Rust plan walker**: add `Plan::min_rollup_window_ms() ->
   Option<i64>` analogous to `requires_tier_zero`. Walks
   recursively, returns the MIN of `range_ms` across all
   MatrixSelect and Subquery nodes.
8. **Rust call-sites**: `eval/select.rs`, `eval/fused.rs`,
   `eval/subquery.rs`, and the test contexts pass through the
   new value.
9. **lib.rs**: `run_*_inner` calls `plan.min_rollup_window_ms()`
   and passes through. Defaults to `i64::MAX` (no constraint)
   when None.
10. **Regression gate**: cargo test, compliance, smoke.
11. **Performance gate**: measure heterogeneous-retention scenario
    (manually create a metric with restricted tier-0 backfill
    and verify the splice). Profile Q3 / Q6 to confirm no
    regression on the simple case (one tier covers, no splicing
    happens, fast path is hit).
12. **Slow-query log**: add per-series chosen-tier(s) to the log
    line so users can inspect the planner's decision. This is the
    user-visible debug surface.
13. **Docs**: brief note on the PromQL endpoint's tier behavior,
    pointing to the v2 contract for parity.
14. **Close**.

Validation plan:

- Unit tests + compliance corpus + smoke as usual.
- New unit tests for `Plan::min_rollup_window_ms()`:
  - Vector-only plan returns None.
  - `rate(M[5m])` returns 300_000.
  - `rate(M[5m]) + avg_over_time(M[1h])` returns 300_000 (MIN).
  - `avg(rate(M[5m]))` (fused) returns 300_000.
  - Subquery: `(M[1h:1m])` returns 3_600_000.
- New smoke checks: heterogeneous-tier scenario, splice scenario
  at retention boundary.
- Manual verification of the slow-query log's chosen-tier
  field on a few representative queries.

Artifact impact plan:

- AGENTS.md: no update.
- Runtime project skills: no update.
- Specs: a brief spec note at `.agents/sow/specs/` describing
  the shim's contract that "PromQL tier selection follows v2
  semantics" would be useful, in case the engine's weight
  function gets re-tuned later and we need a checkpoint of what
  PromQL expected. Add at close.
- End-user/operator docs: docs note as above.
- End-user/operator skills: no update.
- SOW lifecycle: standard close. Single commit.

Open-source reference evidence:

- None. All patterns are in-tree.

Open decisions:

1. **Sub-option for the planner integration.**
   - Option A: shim allocates per-series QUERY_METRIC-shaped
     state and calls `query_plan` unmodified.
   - Option B: extract a leaner `query_plan_for_metric(per_tier_meta[],
     ...)` helper from the engine and have the shim use that.
   - Recommendation: B if extraction is clean (the v2 code drops
     a few fields it doesn't need anyway); A if extraction
     fragments the engine code. Investigation in chunk 1
     determines.

2. **Weight-function calibration.**
   - Option A: keep the v2 weight function unchanged (this SOW).
   - Option B: replace with the log-distance heuristic discussed
     in the SOW-0041 conversation (would mean changes to v1/v2
     too, larger blast radius).
   - Recommendation: A. The log-distance idea is interesting but
     belongs in a separate SOW that addresses all three APIs.
     Don't fork PromQL's tier semantics from v2's without strong
     evidence.

3. **Rollup-window enforcement strictness.**
   - Option A: hard reject any tier whose `update_every >
     rollup_window`. Falls back to a finer tier.
   - Option B: soft preference — penalize but don't reject.
   - Recommendation: A. A `rate(M[5m])` at tier 1 with 60s
     update_every gives at most 5 samples per window; at tier 2
     with 1h update_every gives 0 or 1 sample (rate
     undefined). Hard reject is the correct semantic.

## Implications And Decisions

Pending user response on Open Decisions 1, 2, 3 (or acceptance of
the recommended path).

## Plan

1. Investigation gate (Open Decision 1).
2. Header exposure (chunk 2).
3. Shim resolve refactor (chunk 3).
4. Shim sample iteration refactor (chunk 4).
5. Bindgen + Rust storage (chunks 5-6).
6. Rust plan walker for `min_rollup_window_ms` (chunk 7).
7. Rust call-site updates (chunk 8).
8. lib.rs wiring (chunk 9).
9. Regression + performance gate (chunks 10-11).
10. Slow-query log + docs (chunks 12-13).
11. Close (chunk 14).

## Execution Log

### 2026-05-14

- SOW drafted following user direction to reuse the engine's
  tier-planning infrastructure rather than maintain a parallel
  implementation in the shim.

## Validation

Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

- Weight-function calibration across all three APIs (if PromQL
  workload evidence argues for it).
- SOW-0042's deferred predecessor (rich per-sample drain
  exposing bucket min/max/sum/count) was never opened; its goal
  is now partly subsumed because plan-level tier-0 forcing
  guarantees min/max/quantile operators always see raw samples.
  No re-tracking needed.

## Regression Log

None yet.
