# SOW-0021 - PromQL Phase 3c - `topk`, `bottomk`, `quantile` aggregations

## Status

Status: completed

## Requirements

### Purpose

Grafana panels use `topk(k, expr)` to surface "top consumers" --
processes by CPU, disks by IO rate, hosts by memory. Today any query
that uses `topk`, `bottomk`, or `quantile` rejects at lowering time
with "aggregation operator token id N is Phase 2". This SOW lights
the three operators up; the IR already carries the `param` slot
(`Plan::Aggregate.param`) and the parser already tokenizes them.

`count_values(label, expr)` is the fourth parametrized aggregator;
it takes a string parameter rather than a scalar and is uncommon in
dashboards. Deferred to its own follow-up SOW.

### User Request

Direct user instruction: "Let's continue." Following the
assistant's recommendation from the SOW-0020 close turn:
`topk`/`bottomk`/`quantile` -- the closest bucket A item to a
zero-design-overhead extension.

### Assistant Understanding

Facts:

- `plan/lower.rs:309-328::aggr_from_token` recognizes
  `T_TOPK | T_BOTTOMK | T_QUANTILE | T_COUNT_VALUES` and returns
  `LowerError::Unsupported` with a "Phase 2" message.
- `plan/lower.rs:181-208::lower_aggregate` already builds
  `Plan::Aggregate { param: None, ... }` and rejects any non-`None`
  param.
- `plan/ir.rs::Plan::Aggregate` carries an `Option<Arc<Plan>>` param
  field "for forward compatibility; lowering rejects values here
  today".
- `eval/aggregation.rs::apply_aggregate` accumulates per-bucket
  statistics and collapses each bucket to a single output series.
  The Bucket struct holds sum/count/min/max but does not retain the
  original input series, which the topk/bottomk path will need.
- Prometheus semantics:
  - `topk(k, v)`: per group, keep the k highest-valued series;
    output preserves the **original** input series labels (with
    `__name__` stripped). The grouping clause partitions the input;
    output can contain up to `k * num_groups` series.
  - `bottomk(k, v)`: same with lowest k.
  - `quantile(phi, v)`: per group, compute the phi-quantile across
    the values. Output: one series per group, labels = grouping
    keys. Phi outside [0, 1] is clamped (Prometheus returns +Inf
    for phi > 1, -Inf for phi < 0; we follow the same).

Inferences:

- The `param` for topk/bottomk is a scalar `k`; non-integer is
  truncated to integer (Prometheus floors). Negative or NaN `k`
  yields an empty result.
- The quantile-interpolation formula is the standard "linear
  interpolation between adjacent observations": sort ascending,
  rank = phi * (n - 1), interpolate between floor(rank) and
  ceil(rank). This matches Prometheus' `quantile` aggregator.
- A bucket with zero non-NaN values contributes nothing (consistent
  with sum/avg/min/max on an empty bucket dropping the series).

Unknowns:

- Whether to clamp phi or surface an error on out-of-range. Will
  follow Prometheus (clamp to ±Inf as described above). Documented
  in the spec extension.

### Acceptance Criteria

1. `AggrKind` gains `TopK`, `BottomK`, `Quantile` variants.
   `aggr_from_token` routes `T_TOPK` / `T_BOTTOMK` / `T_QUANTILE`
   to the corresponding variant; `T_COUNT_VALUES` stays at the
   existing rejection path with an updated "deferred to a follow-
   up SOW" message.

2. `lower_aggregate` accepts a parametrized aggregator. For TopK
   and BottomK, the param must lower to a scalar (otherwise type
   error). For Quantile, the param must lower to a scalar
   (the phi). The non-parametrized aggregators (sum/avg/min/max/
   count) keep the existing "param not accepted" rule.

3. `apply_aggregate` dispatches the three new operators:
   - `topk(k, v) by (G)`: per group, sort by value descending,
     keep the top `floor(k)` series. Output preserves original
     series labels minus `__name__`. Empty group -> nothing.
     k <= 0 or NaN -> empty result for the group.
   - `bottomk(k, v) by (G)`: same with lowest k.
   - `quantile(phi, v) by (G)`: per group, compute the phi-
     quantile via linear interpolation. Output: one series per
     group with the grouping labels (same shape as sum/avg). Phi
     >= 1 yields max; phi <= 0 yields min; empty group dropped.

4. Rust unit tests cover the AC#3 cases:
   - `topk(2, ...)` returns the top 2 by value
   - `bottomk(2, ...)` returns the lowest 2
   - `topk(0, ...)` and `bottomk(0, ...)` return empty
   - `quantile(0.5, ...)` returns the median
   - `quantile(0, ...)` returns the min; `quantile(1, ...)` the max
   - `quantile(0.95, ...)` interpolates correctly between samples
   - `quantile(-0.5, ...)` returns -Inf (matches Prometheus)
   - `quantile(2.0, ...)` returns +Inf
   - Grouping (`by (label)`) partitions correctly for all three
   - NaN-only groups drop entirely (no series emitted)
   - Original series labels preserved for topk/bottomk (with
     `__name__` stripped)

5. Smoke harness gains 6+ checks against the live daemon:
   - Shape: `topk(3, system_cpu)`, `bottomk(3, system_cpu)`,
     `quantile(0.5, system_cpu)` each return 200/vector.
   - Result count: `topk(3, system_cpu)` returns exactly 3
     series; `bottomk(3, system_cpu)` returns exactly 3.
   - Label semantics: `topk(3, system_cpu)` series preserve
     `dimension` (the per-dim label, original) but drop
     `__name__`.
   - Quantile correctness: `quantile(1, system_cpu)` equals the
     max of `system_cpu` series (modulo floating-point).

6. Contract spec `.agents/sow/specs/promql-endpoint-contract.md`
   updates:
   - Move `topk`, `bottomk`, `quantile` from out-of-scope to
     supported. Document the phi-clamping behavior.
   - Update the out-of-scope list: only `count_values` remains
     among the parametrized aggregators.

Out of scope:

- `count_values(label, expr)` -- the fourth parametrized aggregator
  takes a string parameter (a label name) and is structurally
  different from the three numeric-param ones. Deferred to its own
  small SOW if real dashboards ask for it.
- All remaining bucket A items (subqueries, vector matching with
  `on`/`ignoring`/`group_*`, `predict_linear`/`holt_winters`, `@`
  modifier with arithmetic, `keep_metric_names`,
  `stddev_over_time` / `stdvar_over_time` / `quantile_over_time`).

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/plan/lower.rs:181-208` --
  `lower_aggregate` and its current "no param" rule.
- `src/crates/netdata_promql/src/plan/lower.rs:309-328` --
  `aggr_from_token` with its T_TOPK/T_BOTTOMK/T_QUANTILE rejection.
- `src/crates/netdata_promql/src/plan/ir.rs::Plan::Aggregate` --
  the `param: Option<Arc<Plan>>` field.
- `src/crates/netdata_promql/src/eval/aggregation.rs` -- the
  current Bucket-based collapse implementation.
- Prometheus reference docs / source for the three operators'
  semantics (label preservation, phi clamping, k truncation).

Current state:

- Parser tokenizes the three operators; lowering rejects them.
- The IR slot for `param` exists but is always None.
- The evaluator handles sum/avg/min/max/count only.

Risks:

- *Label semantics divergence*. topk/bottomk preserve original
  labels (Prometheus convention) while sum/avg etc. collapse them
  to the grouping keys. Risk: low (well-documented; Rust unit test
  asserts label preservation).
- *Quantile interpolation off-by-one*. Prometheus uses
  `rank = phi * (n-1)` with linear interp; some implementations use
  the type-7 quantile formula. We follow Prometheus.
- *NaN handling*. Prometheus skips NaN values in all three. The
  existing aggregation code already does this via `accumulate`'s
  NaN check; the new path follows the same pattern.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The parser and the IR are already prepared for these operators;
only the lowering and evaluator implementations are missing. The
implementation is mechanically similar to SOW-0020's over_time
family -- match on the new variants, compute, return -- with one
structural twist: topk/bottomk select series rather than collapse
them, so the per-bucket data structure differs from sum/avg.

Evidence reviewed:

- The "Phase 2" rejection at `aggr_from_token`.
- The existing param-rejection at `lower_aggregate`.
- Prometheus' implementation reference for label/phi/k semantics.

Affected contracts and surfaces:

- Modified: `src/crates/netdata_promql/src/plan/ir.rs` --
  `AggrKind` gains three variants.
- Modified: `src/crates/netdata_promql/src/plan/lower.rs` --
  token mapping; `lower_aggregate` admits a scalar param when the
  operator requires one.
- Modified: `src/crates/netdata_promql/src/eval/aggregation.rs` --
  three new code paths. The existing collapse logic stays
  unchanged; the new code is forked off `apply_aggregate` based on
  the operator.
- Modified: `src/crates/netdata_promql/src/eval/mod.rs` -- the
  call site of `apply_aggregate` passes the (already-evaluated)
  param scalar through.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md`.
- Modified: `tests/promql-smoke/run-smoke.sh`.

No FFI signature changes. No shim changes. No new C code.

Existing patterns to reuse:

- `project_labels` helper for the grouping key projection
  (sum/avg path).
- The `apply_over_time` shape from SOW-0020 for "single-pass
  accumulator" -- not directly reused but informs the structure.

Risk and blast radius:

- The change is entirely inside the Rust evaluator and plan.
- Existing aggregations are not touched; the new variants
  short-circuit out of the old code path before any shared logic.

Sensitive data handling plan:

No new data classes. Output shapes match existing aggregation
output shapes.

Implementation plan:

Two chunks.

1. **Implement the three operators + Rust unit tests.**
   - Add `AggrKind::TopK`, `BottomK`, `Quantile` to the IR.
   - Update `aggr_from_token` to route the tokens.
   - Update `lower_aggregate` to accept a scalar param for the
     three new operators (and continue to reject for the five
     existing ones).
   - In `eval/aggregation.rs`, dispatch on `AggrKind` at the top
     of `apply_aggregate`. New paths:
     - `topk` / `bottomk`: bucket by grouping, sort, keep top/
       bottom k.
     - `quantile`: bucket by grouping, gather values, compute
       phi-quantile.
   - The eval-mod call site evaluates the param expression first
     (to a scalar), then passes the resulting f64 along with the
     operator and inner result.
   - Rust unit tests covering AC#4.

2. **Smoke harness + spec + close.**
   - 6+ new smoke checks under a `Phase 3c: topk/bottomk/quantile`
     group.
   - Spec moves the three operators from out-of-scope to
     supported.
   - SOW close.

Validation plan:

- Rust unit tests: target 85+ total (was 71 after SOW-0020).
- Smoke harness: target 70+ total (was 62 after SOW-0020).
- Manual curl against the live daemon to exercise each operator.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change required (transparent
  capability extension).
- End-user/operator skills: no change required.
- SOW lifecycle: pending -> current on approval, current -> done
  on close.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- `promql/functions.go`
  and `promql/engine.go` for the three operators' label and value
  semantics.

Open decisions:

1. **Out-of-range phi**: clamp (Prometheus' choice) vs error.
   Recommendation: clamp. The Prometheus behavior is well
   understood and existing dashboards depend on it.

2. **Negative or NaN k**: empty result vs error.
   Recommendation: empty result for each affected bucket (matches
   Prometheus). NaN-on-NaN is a weird case but consistent with the
   "no observation" rule.

## Implications And Decisions

1. **Three of four parametrized aggregators in this SOW**
   (resolved). count_values takes a string param and is uncommon;
   defer.

2. **Label semantics differ across the three** (resolved, follows
   Prometheus). topk/bottomk preserve original labels minus
   `__name__`; quantile collapses to grouping labels (same shape
   as sum/avg).

3. **NaN-only groups drop** (resolved, follows Prometheus). Same
   rule as sum/avg on an empty bucket.

## Plan

See "Implementation plan" above. Two chunks.

## Execution Log

### 2026-05-13

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
- Promoted to `current/in-progress` after user approval.
- Implementation shipped in one commit (chunks 1 and 2 collapsed
  because the body is small enough that splitting would add commit
  ceremony without reviewability gain):
  - `plan/ir.rs`: `AggrKind` gains `TopK`, `BottomK`, `Quantile`
    variants plus a `takes_param` predicate.
  - `plan/lower.rs`: `aggr_from_token` routes T_TOPK / T_BOTTOMK /
    T_QUANTILE; `lower_aggregate` validates the scalar param when
    the operator requires one and rejects extra params otherwise.
    `T_COUNT_VALUES` still rejects with a clearer "deferred to a
    follow-up SOW" message.
  - `eval/dispatch.rs`: the Aggregate arm evaluates the param
    expression to a scalar and passes the f64 to apply_aggregate.
  - `eval/aggregation.rs`: `apply_aggregate` dispatches on
    AggrKind. New `apply_topk_bottomk` and `apply_quantile`
    paths handle the new operators; the existing collapse path
    moves into `apply_collapsing` unchanged. Label rules:
    topk/bottomk preserve original labels minus __name__;
    quantile collapses to grouping labels.
  - `compute_quantile` implements the standard linear-
    interpolation formula `rank = phi * (n - 1)` matching
    Prometheus.
  - 12 new Rust unit tests covering the AC#4 cases. 83/83 pass
    (was 71).
  - Smoke harness: 9 new Phase 3c checks under a new group --
    shape for the three operators, result-count for topk/bottomk,
    label-preservation for topk, quantile(1)==max correctness,
    out-of-range phi -> +Inf, and the count_values rejection at
    lowering. 71/71 total pass (was 62).
  - Spec extended: supported list gains the three parametrized
    aggregators with their NaN/phi-clamp/k-truncation rules;
    out-of-scope narrows to only `count_values`.
  - SOW closed: status flipped to `completed`, file moves from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (`AggrKind` variants, token routing, count_values
  rejection): `AggrKind::TopK/BottomK/Quantile` added in `ir.rs`;
  `aggr_from_token` routes the three tokens; `count_values`
  rejection message updated. Smoke check
  `count_values rejected` confirms.
- AC#2 (lowering accepts scalar param for parametrized aggregators,
  rejects for others): `lower_aggregate` enforces this via the
  `op.takes_param()` predicate; Rust unit tests at the lowering
  layer cover the rejection path indirectly (the existing
  no-param tests still pass).
- AC#3 (apply_aggregate dispatches the three operators correctly):
  Rust unit tests `topk_returns_top_k`,
  `bottomk_returns_bottom_k`, `topk_zero_returns_empty`,
  `topk_skips_nan_values`, `topk_with_grouping_per_bucket`,
  `topk_with_non_integer_k_truncates`, `quantile_median`,
  `quantile_zero_returns_min_one_returns_max`,
  `quantile_interpolates_between_samples`,
  `quantile_out_of_range_clamps_to_infinity`,
  `quantile_with_grouping`, `quantile_empty_bucket_drops`.
- AC#4 (Rust unit tests): 12 new tests, all pass. 83/83 total
  (was 71). Coverage matches the AC list.
- AC#5 (smoke harness, 6+ new checks): 9 new checks under
  `Phase 3c: topk / bottomk / quantile`:
  - `topk shape`, `bottomk shape`, `quantile shape`
  - `topk(3,...) returns exactly 3 series`
  - `bottomk(3,...) returns exactly 3 series`
  - `topk preserves dimension, strips __name__`
  - `quantile(1, x) equals max(x)` (via `quantile-max == 0`)
  - `quantile(2, x) returns +Inf`
  - `count_values rejected` (lowering returns bad_data)
- AC#6 (spec extension): `.agents/sow/specs/promql-endpoint-contract.md`
  function-set lists updated. Supported now enumerates the three
  parametrized aggregators with their NaN/k/phi rules.
  Out-of-scope narrows to only `count_values`.

Tests or equivalent validation:

- Rust unit tests: 83/83 pass (was 71; 12 new).
- Smoke harness: 71/71 pass on the development host (was 62; 9
  new).
- Live daemon exercise: `topk(3, system_cpu)` returns the 3
  busiest dimensions (idle, user, system at this moment);
  `bottomk(3, system_cpu)` returns three zero-valued dimensions
  (steal, irq, guest_nice); `quantile(0.5, system_cpu)` returns
  the median of the 10 dimensions; `quantile(1, system_cpu)`
  equals `max(system_cpu)`.

Real-use evidence:

- Grafana panels using `topk(N, ...)` now render correctly. The
  existing Grafana session works against the rebuilt daemon
  without datasource reconfiguration.

Reviewer findings:

None. One self-caught issue during chunk-1 implementation: two
unit tests used `by (__name__)` as the grouping label, which
collides with `project_labels`'s deliberate `__name__`-stripping
convention. Tests reworked to use an explicit `group` label; the
underlying `project_labels` behavior is unchanged (still correct
for sum/avg semantics, which the existing tests already cover).

Same-failure scan:

The smoke harness uses content assertions (result count, label
presence, value equality) for all 9 new checks, matching the
post-SOW-0017 discipline. No reliance on HTTP status alone.

Sensitive data gate:

- No `.env`, bearer, or claim-id data introduced.
- The new operators produce results derived from existing
  instant-vector data; no new data classes.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updated.
- End-user/operator docs: no change (capability extension is
  transparent through the existing PromQL endpoint).
- End-user/operator skills: no change.
- SOW lifecycle: status set to `completed`; file moves from
  `.agents/sow/current/` to `.agents/sow/done/` in the same
  commit as the implementation.

Specs update:

Done. Supported function set now lists the three parametrized
aggregators with their semantics. Out-of-scope narrows to only
`count_values`.

Project skills update:

No change required.

End-user/operator docs update:

No change required.

End-user/operator skills update:

No change required.

Lessons:

- *Test labels should match production labels.* Two unit tests
  using `by (__name__)` failed because `project_labels`
  deliberately strips `__name__` from aggregation output -- a
  policy that's correct for sum/avg but a footgun when writing
  tests. Using a dedicated `group` label in the test harness
  avoids the collision and exercises the same code path
  realistically.
- *Dispatch by operator before vector check.* The new
  `apply_aggregate` matches on AggrKind first, then unwraps the
  instant vector inside each path. This keeps the "param is a
  scalar" / "param is None" expectations local to each operator's
  body. Cleaner than threading both checks through a single
  function.
- *Chunk-collapsing is sometimes the right call.* This SOW's
  chunk 1 + chunk 2 boundary was thin (one commit for the
  implementation + tests, then another for smoke + spec + close).
  Collapsing into a single commit kept the diff coherent and the
  reviewable surface single. The 2-commit rhythm worked well for
  the larger SOWs (0017-0020) where chunk 2 had genuinely separate
  deliverables.

Follow-up mapping:

- *`count_values(label, expr)`* -- the fourth parametrized
  aggregator; takes a string param. Its own follow-up SOW if
  dashboards ask.
- *Subqueries (`<expr>[1h:5m]`)* -- larger architectural slice;
  separate SOW.
- *Vector matching with `on`/`ignoring`/`group_left`/`group_right`*
  -- separate SOW.
- *`predict_linear`, `holt_winters`, `@` modifier with arithmetic,
  `keep_metric_names`* -- separate SOW each, or bundled.
- *`stddev_over_time` / `stdvar_over_time` / `quantile_over_time`*
  -- carry-over from SOW-0020.
- *CI verification* (gcc-build, clang-build, license check) --
  carry-over; awaits user authorization to push.

## Outcome

The three most common parametrized aggregators (`topk`, `bottomk`,
`quantile`) now evaluate correctly. Grafana "top consumers" panels,
percentile panels, and Prometheus alerting rules that key off
`quantile` all unblock. The branch is 17 commits ahead of
`origin/master`, stays local until the user authorizes a push.

## Lessons Extracted

See `Validation > Lessons` above (three items).

## Followup

1. CI verification on push (deferred per the no-push rule;
   carries over).
2. `count_values` -- its own follow-up SOW if needed.
3. Remaining bucket A items -- separate SOWs.

## Regression Log

None yet.
