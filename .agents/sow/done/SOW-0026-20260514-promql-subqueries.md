# SOW-0026 - PromQL - Subqueries

## Status

Status: completed

Sub-state: shipped 2026-05-14; range vector via nested step-grid
evaluation; cap reused at lowering time; 4 live smoke checks passing.

## Requirements

### Purpose

A subquery `<expr>[range:step]` turns an instant-vector expression
into a range vector by evaluating `expr` at every step grid point in
`[t - range, t]`. Used heavily in alerting rules: e.g.
`max_over_time(rate(http_requests[5m])[1h:1m])` -- the max of
1-minute-stepped rates over the last hour. The lowering currently
rejects subqueries with `Unsupported("subquery [...] is Phase 2")`.

This is the last remaining bucket A item that is parser-feasible
(keep_metric_names is upstream-blocked).

### User Request

"Let's continue." Following the smallest-to-biggest order set by
the user; subqueries is the biggest remaining.

### Assistant Understanding

Facts:

- The promql-parser exposes `Expr::Subquery(SubqueryExpr)` with
  fields `expr: Box<Expr>`, `range: Duration`, `step:
  Option<Duration>`, `offset: Option<Offset>`, `at:
  Option<AtModifier>`.
- Our `plan/lower.rs::lower` currently rejects `Expr::Subquery`
  with "subquery [...] is Phase 2".
- The IR has `ValueType::RangeVector`; subqueries should produce
  `RangeVector` so functions like `rate`, `*_over_time` can
  consume them.
- Step is optional; when omitted, Prometheus defaults to the
  global evaluation interval. We adopt 1 second as the default
  (the minimum sensible) and document it.

Inferences:

- Plan IR adds `Plan::Subquery { expr: Arc<Plan>, range_ms: i64,
  step_ms: i64, offset_ms: i64, at: Option<AtMod> }`.
- Evaluator iterates `t' = (effective_t - range_ms)..=effective_t`
  at `step_ms` stride, evaluates `expr` in a nested EvalContext
  with `at_ms = t'`, collects each step's instant-vector result
  into a per-series range vector keyed by signature. The
  resulting `Vec<Series>` becomes the subquery's output range
  vector.
- A subquery whose inner `expr` itself evaluates to a range
  vector is a type error (Prometheus rejects too).
- Cap on points: same `MAX_POINTS_PER_TIMESERIES = 11_000` cap
  already enforced for range queries applies to subqueries; reject
  at lowering time when `range/step > 11000`.

Unknowns:

- Whether to recompute the host_machine_guid lookup per step. We
  reuse the same ctx so the host scope is fixed -- that's the
  right Prometheus semantic.

### Acceptance Criteria

1. `Plan::Subquery` variant added; `lower` translates
   `Expr::Subquery`. Default `step_ms = 1000` when the parser
   leaves step as `None`.
2. `Plan::Subquery.value_type()` returns `RangeVector`. Inner
   expression must lower to `InstantVector`; rejected at lowering
   if it's a `RangeVector`.
3. Evaluator path: nested loop over the step grid; per-step
   results merged by signature into per-series sample sequences.
4. Subquery + `rate(...)`-family composition works:
   `rate(metric[1m])[10m:1m]` -> per-step rates over 10 minutes.
5. `*_over_time` consumes a subquery range vector:
   `max_over_time((sum(metric)) [5m:1m])` -> max of the per-step
   sums.
6. Cap enforcement: a subquery with `range/step > 11000` rejects
   at lowering with `bad_data`.
7. The `@` modifier on subqueries works: `expr[5m:1m] @ <ts>`
   evaluates the subquery as if the outer time were `<ts>`.
8. Rust unit tests cover IR + lowering shapes, value_type
   propagation, the points-cap rejection, and the inner-must-be-
   instant rejection.
9. Smoke harness gains 4+ checks against the live daemon:
   - `system_cpu[1m:30s]` returns a range vector with the
     expected step count.
   - `max_over_time((sum(system_cpu)) [1m:30s])` returns a
     non-empty result.
   - `rate(disk_io[2m])[5m:1m]` returns a range vector.
   - `system_cpu[1m:1ms]` (10000-points cap blown) rejects with
     400.
10. Contract spec moves subqueries from out-of-scope to
    supported.

Out of scope:

- Sample-aligned step grids matching Prometheus' exact rounding
  rules. We use straightforward integer-arithmetic step iteration
  which matches Prometheus for typical inputs but may differ in
  the last fractional step.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The IR lacks a subquery variant. Adding one is mechanical; the
evaluator path mirrors the existing range-query loop in
`lib.rs::run_range` but at a nested level inside `eval()`.

Affected contracts and surfaces:

- Modified: `plan/ir.rs` -- Plan::Subquery; value_type.
- Modified: `plan/lower.rs::lower` -- replace the rejection.
- Modified: `eval/dispatch.rs` -- new arm.
- New: `eval/subquery.rs` -- the nested-eval loop.
- Modified: `eval/mod.rs` -- expose new module.
- Modified: spec and smoke harness.

No FFI, no shim changes.

Existing patterns to reuse:

- `lib.rs::run_range` -- the step-grid loop shape; adapt for
  nested evaluation.
- `Plan::MatrixSelect`'s offset/at handling.

Risk and blast radius:

- Performance: nested evaluation is O(steps * inner_cost). The
  outer query's range query path can multiply this by yet
  another factor. The point cap bounds the worst case.
- Correctness: per-step series may have different label sets
  (e.g. a series disappears mid-window). The merge-by-signature
  approach drops series for steps where they're absent, leaving
  NaN holes; this matches Prometheus.

Implementation plan (single chunk):

1. Add `Plan::Subquery` to ir.rs.
2. Lowering: replace rejection; default step to 1s; enforce
   point cap.
3. Add eval/subquery.rs: nested loop; merge per-step results
   by signature.
4. Wire dispatch.
5. Rust unit tests + smoke + spec + close.

Validation plan:

- Rust unit tests: target 120+.
- Smoke harness: target 96+.
- Live daemon: 4 curl exercises.

Artifact impact plan:

- AGENTS.md, runtime project skills, end-user docs/skills: no
  change.
- Spec: updated.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence: Prometheus' subquery semantics
from `promql/engine.go`.

## Execution Log

### 2026-05-14

- SOW drafted.
- Promoted to current; implemented single chunk:
  - `Plan::Subquery { expr, range_ms, step_ms, offset_ms, at }`
    added to `plan/ir.rs`; `value_type()` returns `RangeVector`.
  - `lower_subquery` in `plan/lower.rs` replaces the Phase 2
    rejection. Defaults `step_ms = 1000` when the parser leaves
    `step` as `None`. Type-checks inner is `InstantVector`
    (defense-in-depth -- promql-parser pre-rejects the matrix
    inner case at the AST stage). Reuses
    `crate::MAX_POINTS_PER_TIMESERIES = 11_000` cap at lowering
    time with a `bad_data` rejection.
  - `eval/subquery.rs` implements `eval_subquery`: nested loop
    over `(window_start + step ..= effective_t]` at `step_ms`
    stride; per-step inner evaluation in a derived
    `EvalContext` whose `at_ms = t'` and whose
    `outer_start_ms`/`outer_end_ms` stay fixed at the outer
    query's bounds; per-signature merge into a stable-sorted
    `Vec<Series>`.
  - `eval/mod.rs` exposes the new module; `eval/dispatch.rs`
    routes `Plan::Subquery`.
  - `lib.rs::MAX_POINTS_PER_TIMESERIES` made `pub(crate)` so
    the lowering layer can reuse it.

## Validation

Acceptance criteria coverage:

1. `Plan::Subquery` variant added; lowering translates
   `Expr::Subquery`; default `step_ms = 1000` --
   `plan/ir.rs` + `plan/lower.rs::lower_subquery` +
   `subquery_defaults_step_to_one_second` unit test.
2. `Plan::Subquery.value_type()` returns `RangeVector`;
   inner must lower to `InstantVector` -- IR `value_type()`
   match arm + `subquery_lowers_to_range_vector` and
   `subquery_rejects_range_vector_inner` unit tests.
3. Evaluator nested loop with per-signature merge --
   `eval/subquery.rs::eval_subquery`.
4. Subquery + `rate`-family composition --
   `subquery_over_rate_lowers` unit test +
   `rate(disk_io[2m])[5m:1m] returns a matrix` smoke check.
5. `*_over_time` consumes subquery range vector --
   `max_over_time_consumes_subquery` unit test +
   `max_over_time over subquery returns a vector` smoke check.
6. Cap enforcement: `range/step > 11000` rejects with
   `bad_data` -- `subquery_rejects_oversized_point_count` unit
   test + `system_cpu[1m:1ms] rejects with bad_data` smoke check.
7. `@` modifier on subqueries --
   `subquery_carries_at_modifier` and
   `subquery_at_start_and_end_lower` unit tests.
8. Unit tests cover IR + lowering shapes, value_type
   propagation, points-cap rejection, inner-must-be-instant
   rejection -- nine new tests in `plan/lower.rs::tests`, plus
   `inner_scalar_is_rejected_at_eval` in `eval/subquery.rs`.
9. Smoke harness gains 4+ live checks -- the Phase 3h block
   in `tests/promql-smoke/run-smoke.sh` adds exactly the four
   checks listed in the SOW.
10. Contract spec moves subqueries from out-of-scope to
    supported -- `.agents/sow/specs/promql-endpoint-contract.md`.

Test posture:

- Rust unit tests: 125/125 (was 116; +9 new across
  `plan/lower.rs` and `eval/subquery.rs`).
- Smoke harness: 95/95 against the live daemon (was 92;
  +4 new Phase 3h checks, -1 obsolete `subquery rejected`
  rejection check).
- Live daemon: the four curl exercises in the smoke
  harness produce the expected matrix resultType for the
  range-yielding queries, the expected vector resultType
  for the `max_over_time` composition, and HTTP 400 for
  the over-cap query.

Reviewer findings: none yet (no PR pushed; per the
no-push-without-instruction memory the branch stays local).

Same-failure search: the `subquery rejected` smoke check
that asserted the Phase 2 rejection at HTTP 400 was removed
and replaced by the cap-rejection check; no other tests
reference the old "subquery [...] is Phase 2" string.

Artifact maintenance gate:

- `AGENTS.md`: no change (subqueries are a normal
  PromQL surface, not a workflow change).
- Runtime project skills: no change (the SOW touches the
  PromQL evaluator only; no project skill covers PromQL).
- Spec: updated -- subqueries moved from out-of-scope to
  supported, with the cap and step-default rules; the
  "Open items deferred" tail is brought up to date.
- End-user docs / skills: no change (PromQL endpoint is
  still phase-gated; no public skill consumes the surface
  yet).
- SOW lifecycle: pending -> current -> done; status
  `completed`.

Spec update: yes (see Artifact maintenance gate).

Project skill update: not required (none cover PromQL).

End-user docs update: not required (PromQL endpoint is
still phase-gated and not yet announced).

End-user skill update: not required (same reason).

## Outcome

Subqueries ship as a single chunk. The IR + lowering path
took ~60 lines; the evaluator took ~80 lines; the tests
account for the rest. Composition with `rate` and
`*_over_time` works exactly as in Prometheus because the
inner is just a normal instant-vector evaluation at a
different `at_ms`. The cap reuse means a pathological
`[1m:1ms]` rejects at lowering time rather than running
60,000 inner evaluations.

## Lessons

- `promql-parser` rejects range-vector subquery inners at
  the AST stage with a `Parse` error, so the lowering's
  type check is only reachable via direct Plan
  construction. Kept as defense in depth.
- `EvalContext::outer_start_ms`/`outer_end_ms` plumb
  through unchanged from outer to inner, so `@ start()`
  inside a subquery resolves against the outer query's
  range -- matches Prometheus.
- Reusing the existing `MAX_POINTS_PER_TIMESERIES`
  constant at lowering time avoids divergence with the
  range-query path; the same cap, the same error message
  shape.

## Followup

- Tier selection beyond tier 0 remains deferred to a future
  SOW. Subqueries do not change that surface.
- `keep_metric_names` remains upstream-blocked and is the
  only PromQL surface in the original Phase 3 plan that
  this branch does not implement.
- CI verification on push (gcc-build, clang-build, license
  check) remains gated on user authorization to push.

## Regression Log

None yet.
