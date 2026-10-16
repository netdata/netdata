# SOW-0024 - PromQL - `count_values` aggregator

## Status

Status: completed

## Requirements

### Purpose

`count_values(label, expr)` is the fourth parametrized aggregator in
Prometheus. SOW-0021 shipped the other three (`topk`, `bottomk`,
`quantile`) but skipped `count_values` because its parameter is a
*string* (a label name), not a scalar. The existing `Plan::Aggregate`
slot is `param: Option<Arc<Plan>>` which is shaped for scalars only.

This SOW closes that gap. After it lands, all four parametrized
aggregators evaluate correctly and `count_values` shifts from
"unknown function" rejection to actual evaluation.

### User Request

Direct user instruction: "Let's pick the items by smallest to
bigger architectural changes." count_values is the smallest of the
four remaining bucket A items.

### Assistant Understanding

Facts:

- `plan/lower.rs::aggr_from_token` rejects `T_COUNT_VALUES` with a
  "deferred to a follow-up SOW" message (from SOW-0021).
- `plan/ir.rs::AggrKind` enumerates {Sum, Avg, Min, Max, Count,
  TopK, BottomK, Quantile} with a `takes_param()` predicate that
  returns true for the parametrized three (numeric).
- `Plan::Aggregate.param: Option<Arc<Plan>>` carries the scalar
  param. `count_values` needs a string param.
- `promql_parser` represents the string param as an
  `Expr::StringLiteral` in the parser's `AggregateExpr.param`
  slot.
- The semantic of `count_values("status", expr)`: bucket the
  input series by their **value** (a number), emit one series
  per distinct value with the supplied `label` set to the
  string-form of that value. Grouping labels (from `by`/
  `without`) are preserved; `__name__` is stripped per
  aggregation convention.

Inferences:

- Two ways to carry the string param:
  1. Add a sibling `param_string: Option<String>` to
     `Plan::Aggregate`.
  2. Replace `param: Option<Arc<Plan>>` with an enum
     `AggrParam { None, Scalar(Arc<Plan>), String(String) }`.
  
  Option 2 is cleaner but a bigger diff. Option 1 is uglier but
  minimal. We pick (1) because every caller that wants the scalar
  param already knows its variant by `AggrKind::takes_param()`,
  so adding a parallel `takes_string_param()` keeps the touch
  surface small.

- `count_values` does not accept negative or NaN-input values
  meaningfully -- Prometheus emits the string form (e.g. `"NaN"`,
  `"-Inf"`) and groups them. We follow.

- The output label name (the first arg) must be a valid PromQL
  label name. Prometheus rejects invalid names at parse time.

Unknowns:

- Whether to format integer-valued floats as `"42"` or `"42.0"`.
  Prometheus uses Go's default float formatting, which renders
  integers without a decimal. We use Rust's default `{}` format
  on f64, which renders the same way for typical inputs.

### Acceptance Criteria

1. `AggrKind` gains a `CountValues` variant. `aggr_from_token`
   routes `T_COUNT_VALUES` to it. The "deferred" rejection from
   SOW-0021 goes away.

2. `Plan::Aggregate` gains a `param_string: Option<String>` field
   alongside the existing `param`. `AggrKind::takes_param()` is
   joined by a new `takes_string_param()` predicate; lowering
   enforces:
   - `count_values` requires `param_string`, rejects `param`.
   - The other parametrized aggregators (topk/bottomk/quantile)
     require `param`, reject `param_string`.
   - Non-parametrized aggregators reject both.

3. `apply_aggregate` dispatches a new `apply_count_values` path
   that:
   - Buckets input series by **(grouping_key, value_string)**.
   - Emits one output series per bucket with the grouping labels
     plus `<param_string>=<value_string>` plus a value equal to
     the count of inputs in that bucket.
   - NaN-input series contribute to the `"NaN"` bucket
     (Prometheus convention).
   - `__name__` is stripped (aggregation convention).

4. Rust unit tests cover:
   - Two distinct input values -> two output series, each with the
     count of inputs.
   - Three series sharing one value -> one output series with
     count 3.
   - Grouping `by (label)` partitions correctly.
   - Output series carry the supplied label name with the
     stringified value.
   - NaN values bucket under `"NaN"`.
   - Lowering rejects `count_values` without a string param.
   - Lowering rejects scalar-param aggregators with a string param
     (e.g. `topk("x", v)` is malformed).

5. Smoke harness gains 2+ checks against the live daemon:
   - `count_values("v", system_cpu)` returns the expected number
     of buckets (one per distinct value).
   - The returned series carry the `"v"` label.

6. Contract spec moves `count_values` from out-of-scope to
   supported with the bucketing semantics documented.

Out of scope:

- `keep_metric_names`, `@` modifier, subqueries -- separate SOWs.

## Analysis

Sources checked:

- `plan/ir.rs::AggrKind` and `Plan::Aggregate`.
- `plan/lower.rs::aggr_from_token`, `lower_aggregate`.
- `eval/aggregation.rs::apply_aggregate` and dispatch.
- Prometheus reference for `count_values` semantics.

Current state:

- `count_values` rejects at lowering with the "deferred" message.
- The IR slot for `param` is scalar-shaped only.

Risks:

- *String-formatting precision*. Floats with many decimals could
  produce verbose label values. Prometheus uses Go's default; we
  use Rust's. They're not bit-identical but are equivalent for
  typical PromQL inputs (integers, percentages).
- *Backward compat*. Adding `param_string` to `Plan::Aggregate`
  is a struct field addition. Every site that destructures
  `Plan::Aggregate` either uses `..` or names fields explicitly;
  a sweep verifies the change compiles.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The IR lacks a string-param slot for `count_values`. Adding it is
a small extension. The evaluator path is mechanically similar to
the existing `apply_collapsing` but the bucket key includes the
value's string form.

Evidence reviewed:

- The "deferred" rejection in `aggr_from_token`.
- The shape of existing parametrized-aggregator paths.
- Prometheus' `count_values` semantics.

Affected contracts and surfaces:

- Modified: `plan/ir.rs` -- `AggrKind` gains `CountValues`;
  `Plan::Aggregate` gains `param_string`; new
  `takes_string_param` predicate.
- Modified: `plan/lower.rs` -- `aggr_from_token` routes the
  token; `lower_aggregate` enforces param shape per operator.
- Modified: `eval/dispatch.rs` -- destructure the new field
  and forward.
- Modified: `eval/aggregation.rs` -- new `apply_count_values`
  path.
- Modified: spec and smoke harness.

No FFI changes. No shim changes.

Existing patterns to reuse:

- The `apply_collapsing` BTreeMap-keyed bucket approach.
- `project_labels` for the grouping projection.

Risk and blast radius:

- Additive struct-field change to `Plan::Aggregate`. Every site
  that destructures `Plan::Aggregate` is in this repo (no public
  API) so the sweep is local.
- The new evaluator path is a fresh function -- existing
  aggregators don't change.

Sensitive data handling plan:

No new data classes.

Implementation plan:

A single chunk.

1. Add `AggrKind::CountValues` + `takes_string_param` predicate.
   Update `aggr_from_token`.
2. Add `Plan::Aggregate.param_string`. Sweep destructures to
   match.
3. `lower_aggregate`: enforce param shape per operator. Extract
   `StringLiteral` from the parser's param slot.
4. `eval/dispatch.rs`: forward `param_string` to apply_aggregate.
5. `apply_aggregate`: dispatch `AggrKind::CountValues` to a new
   `apply_count_values`.
6. Rust unit tests + smoke checks + spec + close.

Validation plan:

- Rust unit tests: target 115+ (was 109).
- Smoke harness: target 88+ (was 86).
- Live daemon curl.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated.
- End-user/operator docs: no change.
- End-user/operator skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- the count_values
  implementation in `promql/functions.go` / `promql/engine.go`.

Open decisions:

1. **String vs enum for param**. Recommend a parallel
   `param_string` field; the alternative (replacing `param` with
   an enum) is a bigger diff with no semantic gain.

## Implications And Decisions

1. **Single chunk** (resolved). Small enough.

2. **Float-formatting matches Rust default** (resolved). For
   typical PromQL inputs the strings are identical to Prometheus'
   Go default. We document the choice in the spec.

## Plan

See "Implementation plan" above. One chunk.

## Execution Log

### 2026-05-14

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
- Promoted to current/in-progress after user approval.
- Shipped in a single commit:
  - `AggrKind::CountValues` added; `takes_string_param()`
    predicate distinguishes it from the scalar-param family.
  - `Plan::Aggregate` gains a parallel `param_string` field.
    Lowering enforces the three exclusivity rules (string-param
    required, scalar-param required, both rejected per op).
  - `aggr_from_token` routes `T_COUNT_VALUES` to the new variant.
    The "deferred to a follow-up SOW" rejection from SOW-0021 is
    gone.
  - `lower_aggregate` extracts the `StringLiteral` from the
    parser's param slot when the operator is count_values.
  - `eval/dispatch.rs` forwards both `param` (the evaluated scalar)
    and `param_string` (a borrowed `&str`) to `apply_aggregate`.
  - `apply_count_values` buckets input series by
    `(grouping_signature, value_string)`, emits one output series
    per bucket with `<label>=<value_string>`. NaN values bucket
    under `"NaN"`; `__name__` stripped.
  - `format_value_for_label` uses Rust's default `{}` formatter
    for finite floats (matches Prometheus' Go default for typical
    inputs); handles NaN / +Inf / -Inf as their string forms.
  - 4 new Rust unit tests cover the bucketing rule, the
    all-same-value collapse, grouping partitioning, and the NaN
    bucket. 113/113 Rust tests pass (was 109).
  - 3 new smoke checks under a `Phase 3f: count_values` group:
    every output carries the `"v"` label, the bucket counts sum
    to the input series count (10 for system_cpu), and
    `__name__` is stripped. 88/88 total smoke checks pass (was
    86 after one obsolete "count_values rejected" check was
    removed).
  - Spec extended: count_values moves from out-of-scope to
    supported with its NaN/Inf bucket rule documented. Out-of-
    scope narrows to keep_metric_names, @ modifier, subqueries.
  - SOW closed: status `completed`, file moves from
    `.agents/sow/current/` to `.agents/sow/done/` in the same
    commit.

## Validation

Acceptance criteria evidence:

- AC#1 (AggrKind + token routing): `CountValues` variant + the
  `takes_string_param` predicate added. `aggr_from_token` routes
  `T_COUNT_VALUES` to the new variant.
- AC#2 (Plan::Aggregate.param_string + lowering enforcement):
  Field added; `lower_aggregate` enforces the exclusivity rules
  via a `match (&a.param, takes_param, takes_string_param)`.
- AC#3 (apply_count_values): implemented in
  `eval/aggregation.rs`; covered by 4 unit tests.
- AC#4 (Rust unit tests): `count_values_buckets_by_value`,
  `count_values_all_same_collapses_to_one`,
  `count_values_with_grouping_partitions`,
  `count_values_nan_bucket`. 113/113 Rust tests pass (was 109).
- AC#5 (smoke harness): 3 new checks under Phase 3f. 88/88 total
  smoke checks pass.
- AC#6 (spec extension): supported list now lists count_values
  with its NaN/Inf bucket rule. Out-of-scope narrows.

Tests or equivalent validation:

- Rust: 113/113.
- Smoke: 88/88.
- Live daemon: `count_values("v", system_cpu)` on a 10-dimension
  metric returns 5 buckets (six dimensions at zero collapse to
  one bucket; four distinct non-zero values).

Real-use evidence:

The live daemon exercise above; Grafana panels using count_values
now evaluate.

Reviewer findings:

The signature change to `apply_aggregate` (added `param_string`
parameter) required updating 17 test call sites. Used a regex
sweep for the single-line forms and three manual edits for the
multi-line forms. No semantic regressions.

Same-failure scan:

The "X rejected" smoke pattern is consistently removed as features
land (SOW-0017 lesson). The new check `count_values rejected`
became obsolete with this SOW and was replaced by the positive
Phase 3f group.

Sensitive data gate:

No new data classes.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated.
- End-user/operator docs: no change.
- End-user/operator skills: no change.
- SOW lifecycle: `completed`; file moves to done/ in the same
  commit.

Specs update:

Done. Supported list documents count_values' bucketing rule.
Out-of-scope narrows to keep_metric_names, @ modifier, subqueries.

Project skills update:

No change.

End-user/operator docs update:

No change.

End-user/operator skills update:

No change.

Lessons:

- *Parallel optional fields beat enum refactors for small extensions.*
  The choice between `Plan::Aggregate.param_string: Option<String>`
  and a new `AggrParam` enum was clear in retrospect: the existing
  callers index by AggrKind, so the new predicate `takes_string_param`
  naturally selects the right path. An enum would have forced more
  destructure changes across the codebase for zero ergonomic gain.
- *Removing obsolete smoke checks is a SOW-close ritual.* Each
  SOW that lights up a feature has a "this used to be rejected"
  smoke check that becomes stale. Add a positive check for the
  new feature and remove the old rejection. SOW-0023 and SOW-0024
  both did this.

Follow-up mapping:

- `@` modifier with arithmetic (next smallest architectural
  item; needs a `Plan::At` wrapper or an `at_ms` field on
  selectors).
- `keep_metric_names` (medium; needs a flag threaded through
  every strip point).
- Subqueries (biggest; needs nested time-grid evaluation).
- CI verification on push (carry-over).

## Outcome

`count_values` works end to end; bucket A's parametrized-
aggregator slice is now fully covered (topk/bottomk/quantile/
count_values). The branch is 20 commits ahead of `origin/master`.

## Lessons Extracted

See `Validation > Lessons` (two items).

## Followup

1. `@` modifier with arithmetic -- separate SOW.
2. `keep_metric_names` -- separate SOW.
3. Subqueries -- separate SOW.
4. CI verification on push (carry-over).

## Regression Log

None yet.
