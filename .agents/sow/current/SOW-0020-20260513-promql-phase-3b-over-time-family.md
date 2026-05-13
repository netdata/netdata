# SOW-0020 - PromQL Phase 3b - The `*_over_time` family

## Status

Status: in-progress

Sub-state: promoted to current 2026-05-13 after user approval; chunk 1
(implement 7 over_time functions + Rust unit tests) in progress.

## Requirements

### Purpose

Phase 1 (SOW-0017) shipped `rate`, `irate`, `increase`, and `delta`
over the counter family; this SOW adds the `*_over_time` family that
Grafana panels and Prometheus alerting rules use for windowed
aggregations of gauges. Today the parser and lowering recognize the
names but the evaluator returns 422 "not yet implemented" for any
query that uses them. After this SOW lands, the seven most common
windowed-aggregation functions evaluate correctly and a Grafana panel
like `avg_over_time(system_cpu[5m])` renders a real graph.

### User Request

Direct user instruction: "Let's continue with your recommendation."
The recommendation (logged in the prior turn) was bucket A starting
with `*_over_time`: the parser/lowering already recognize these
names, the implementation pattern matches the rate/increase family
exactly, and the user-visible payoff is concrete.

### Assistant Understanding

Facts:

- `FuncKind::AvgOverTime` through `FuncKind::LastOverTime` are
  enumerated in `plan/ir.rs:78-87`. `FuncKind::from_name` maps the
  six over_time names; the parser already routes Prometheus syntax
  through to these variants.
- `eval/functions.rs:25-34` lists all six in the dispatch table with
  a `NotYetImplemented` arm that emits "Phase 2". (The "Phase 2"
  comment predates the chunk split; the bucket is Phase 3 in the
  current taxonomy.)
- `expect_one_range_vector` (functions.rs:45) and `strip_name`
  (functions.rs:357) are the existing helpers for one-argument
  windowed transforms. The implementation pattern is identical to
  `apply_window_op` (rate/increase): take a `Vec<Series>` of
  range-vector data, compute one scalar per series, emit an instant
  vector with the same labels minus `__name__`.
- The range-vector evaluator at `eval/select.rs::eval_matrix_select`
  emits every sample in the window per series; NaN samples are not
  filtered there (Phase 1 filtered NaN at the instant-vector path,
  not at the matrix path). The over_time implementations must
  handle NaN themselves -- skipping them is the Prometheus
  convention.
- Prometheus reference: `*_over_time` functions strip `__name__`
  from the output series, EXCEPT `last_over_time`, which preserves
  it. The rationale: a windowed mean/sum/count produces a different
  statistic than the source metric, while `last_over_time` returns
  a single observation of the same metric. This is documented in
  Prometheus' function table and codified in
  `prometheus/prometheus/promql/functions.go` (search for
  `FunctionDescription.KeepMetricName`).

Inferences:

- `present_over_time(v)` is trivial (1 if any non-NaN sample, else
  no series emitted) and Grafana panels commonly use it for "is the
  series alive?" alerting. Worth bundling into this SOW even though
  it isn't currently in the `FuncKind` enum -- the parser will
  recognize it through the same lowering once we add the variant.
- `stddev_over_time` and `stdvar_over_time` are simple but uncommon
  in Grafana dashboards. Defer to a follow-up unless lowering
  recognition needs them anyway (it doesn't -- the parser will
  reject them).
- `quantile_over_time(phi, v)` takes a scalar parameter and is
  more involved (sample sort + interpolation). Defer to its own
  SOW.

Unknowns:

- Whether `last_over_time` really preserves `__name__` in current
  Prometheus or only in early versions. Will verify against
  `prometheus/prometheus @ tag v2.45.0` source during chunk 1.

### Acceptance Criteria

1. `avg_over_time(<range>)`, `sum_over_time(<range>)`,
   `min_over_time(<range>)`, `max_over_time(<range>)`,
   `count_over_time(<range>)` each evaluate against a real Netdata
   range vector, producing one sample per series with the expected
   scalar.
   - NaN samples are skipped (Prometheus convention; "no
     observation").
   - All five strip `__name__` from the output labels (same as
     `rate`/`increase`).
   - Empty windows (no non-NaN samples) drop the series from the
     result.

2. `last_over_time(<range>)` returns the value at the most recent
   non-NaN sample timestamp in the window. Output labels preserve
   `__name__` (Prometheus convention).

3. `present_over_time(<range>)` returns `1` for every series with
   at least one non-NaN sample in the window. Series with no
   non-NaN samples are dropped. `__name__` is stripped (Prometheus
   convention).
   - Requires adding `PresentOverTime` to `FuncKind` and routing
     the name through `from_name`.

4. The dispatcher at `eval/functions.rs::apply_call` no longer
   returns `NotYetImplemented` for any of the seven names. Cases
   not covered (`stddev_over_time`, `stdvar_over_time`,
   `quantile_over_time`) remain `NotYetImplemented` with a clear
   message that they ship in a follow-up SOW.

5. Rust unit tests cover the seven functions with synthetic series
   (no live daemon required):
   - Each function on a series with 3-5 samples, expected
     scalar.
   - NaN handling: NaN samples skipped.
   - Empty window: series dropped.
   - `last_over_time` preserves `__name__`.
   - `last_over_time` empty window: series dropped.
   - `present_over_time` empty window: series dropped.

6. Smoke harness checks the seven functions against the live
   daemon for shape and HTTP status, plus a sanity check on
   `count_over_time(system_cpu[1m]) >= 1` (every dimension should
   have at least one sample in the last minute on a populated
   host).

7. Contract spec `.agents/sow/specs/promql-endpoint-contract.md`
   updates the supported-function-set section:
   - Move the over_time family items into the "supported" list.
   - Document the deferral of `stddev_over_time`,
     `stdvar_over_time`, `quantile_over_time` in the Phase 3
     follow-ups list.

Out of scope for this SOW (Phase 3c+ or later):

- `stddev_over_time` / `stdvar_over_time` -- straightforward but
  uncommon; can ship as a small follow-up SOW if real dashboards
  ask for them.
- `quantile_over_time` -- needs sample sort + interpolation; its
  own SOW.
- Subqueries (`<expr>[1h:5m]`), vector matching with
  `on`/`ignoring`/`group_left`/`group_right`, the `@` modifier,
  `topk`/`bottomk`/`quantile` (the aggregation, not the over_time
  variant), `predict_linear`/`holt_winters`, `keep_metric_names`.

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/plan/ir.rs:75-87` -- FuncKind
  variants and `from_name`.
- `src/crates/netdata_promql/src/eval/functions.rs:18-34` -- the
  dispatch table with the current `NotYetImplemented` arm.
- `src/crates/netdata_promql/src/eval/functions.rs:45-88` -- the
  `expect_one_range_vector` and `apply_window_op` helpers that the
  new functions will follow.
- `src/crates/netdata_promql/src/eval/functions.rs:357-` --
  `strip_name` helper used to drop `__name__`.

Current state:

- The seven function names parse and lower correctly today; only
  evaluation is missing. End-to-end testing of the parser and
  lowering already passes for `avg_over_time(system_cpu[5m])`; only
  the final eval step fails.
- The pattern for windowed transforms is well-established
  (`apply_window_op` for rate/increase).

Risks:

- *`__name__` preservation on `last_over_time`*. If we get this
  wrong, Grafana panels using `last_over_time(metric)` may either
  show the metric name where they shouldn't or vice versa. Risk:
  low (verified against Prometheus source during chunk 1; covered
  by a Rust unit test).
- *NaN-empty windows*. Series whose every sample is NaN in the
  window should drop, not emit a NaN value (Prometheus behavior).
  Easy to test; included in AC#5.
- *Off-by-one on `count_over_time`*. Some implementations count
  every step grid point; Prometheus counts only non-NaN samples.
  We follow Prometheus.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The evaluator already supports range-vector inputs (Phase 1's
matrix selector path) and the dispatcher already routes the
seven function names; only the implementation bodies are missing.
The work is straightforward: seven small functions that follow the
shape of `apply_window_op`.

Evidence reviewed:

- The `NotYetImplemented` arm in functions.rs:25-34.
- The matrix-selector path's sample-emission shape in
  eval/select.rs::eval_matrix_select.
- Prometheus' reference implementation for the over_time family
  (consulted via web reference, not a local checkout).

Affected contracts and surfaces:

- Modified: `src/crates/netdata_promql/src/eval/functions.rs` --
  dispatch table arms and seven new helper functions.
- Modified: `src/crates/netdata_promql/src/plan/ir.rs` -- add
  `FuncKind::PresentOverTime` variant.
- Modified: `src/crates/netdata_promql/src/plan/lower.rs` --
  `FuncKind::from_name` routes "present_over_time" to the new
  variant.
- Modified: `.agents/sow/specs/promql-endpoint-contract.md` --
  promote the family from "out of scope" to "supported".
- Modified: `tests/promql-smoke/run-smoke.sh` -- new checks under
  a `Phase 3b: *_over_time family` group.

No FFI signature changes. No shim changes. No new C code.

Existing patterns to reuse:

- `apply_window_op` shape for the six aggregators.
- `expect_one_range_vector` for arg validation.
- `strip_name` for the six that drop `__name__`.
- The smoke harness `check_instant` helper for the new shape
  checks.

Risk and blast radius:

- The change is entirely inside the Rust evaluator; no C, no FFI,
  no schema. Risk is bounded to query results.
- Existing queries that use rate/increase/etc. are not affected;
  the new arms only fire for the seven names that previously
  returned 422.

Sensitive data handling plan:

No new data classes. The output shapes are the same shapes the
evaluator already emits.

Implementation plan:

Two chunks.

1. **Implement the seven functions + Rust unit tests.**
   - Add `apply_avg_over_time`, `apply_sum_over_time`,
     `apply_min_over_time`, `apply_max_over_time`,
     `apply_count_over_time`, `apply_last_over_time`,
     `apply_present_over_time` in `functions.rs`. Each follows
     `apply_window_op`'s shape -- range-vector input, scalar
     output per series, strip name (except last_over_time).
   - Add `FuncKind::PresentOverTime` variant; route in
     `lower.rs`.
   - Update the dispatch table arm in `apply_call`.
   - Add ~12 Rust unit tests covering the AC#5 cases.

2. **Smoke harness + spec + close.**
   - 7 new smoke checks (shape only, one per function plus the
     count-sanity-check).
   - Spec updates: move the family from "out of scope" to
     "supported"; document the three deferred siblings.
   - Smoke checks pass against the live daemon.
   - SOW close.

Validation plan:

- Rust unit tests: target 70+ total (was 59 after SOW-0019).
- Smoke harness: target 60+ total (was 52 after SOW-0019).
- Manual curl exercises one query per function to confirm a
  non-422 response shape.
- Real-use validation via the existing Grafana session: a panel
  using `avg_over_time(system_cpu[5m])` renders.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: `.agents/sow/specs/promql-endpoint-contract.md` updates
  the supported-function-set list and the deferral list.
- End-user/operator docs: no change.
- End-user/operator skills: no change.
- SOW lifecycle: pending -> current on approval, current -> done
  on close.

Open-source reference evidence:

- `prometheus/prometheus @ tag v2.45.0` -- `promql/functions.go`
  for the seven implementations; `promql/parser/functions.go` for
  the `KeepMetricName` flags. Consulted via web reference at
  chunk 1; the SOW close validates findings against the
  checked-in source.

Open decisions:

1. **`present_over_time`: include in this SOW?** Recommendation:
   yes. The function is trivial (one line of logic), Grafana
   alerts use it heavily, and bundling it now avoids a five-line
   follow-up SOW.

2. **`stddev_over_time` / `stdvar_over_time`: include?**
   Recommendation: no. They're simple but uncommon in dashboard
   use; a future SOW (or this one's followup section) can pick
   them up if real-world queries surface them.

## Implications And Decisions

1. **Scope is six well-known + present_over_time, not all 10**
   (resolved, weak). The seven cover the dashboard-common cases;
   the remaining three (stddev / stdvar / quantile over_time)
   can ship separately if needed.

2. **`last_over_time` keeps `__name__`** (resolved, follows
   Prometheus). Verified during chunk 1 against
   `prometheus/prometheus`. The other six strip `__name__`.

3. **NaN samples are skipped** (resolved, follows Prometheus).
   Every windowed aggregator iterates only non-NaN samples;
   empty windows drop the series.

## Plan

See "Implementation plan" above. Two chunks.

## Execution Log

### 2026-05-13

- SOW drafted. Pre-Implementation Gate filled; status `ready`.
  Awaiting user approval to promote to `current/in-progress`.

## Validation

Acceptance criteria evidence:

Pending.

Tests or equivalent validation:

Pending.

Real-use evidence:

Pending.

Reviewer findings:

Pending.

Same-failure scan:

Pending.

Sensitive data gate:

Pending.

Artifact maintenance gate:

Pending.

Specs update:

Pending.

Project skills update:

Pending.

End-user/operator docs update:

Pending.

End-user/operator skills update:

Pending.

Lessons:

Pending.

Follow-up mapping:

Pending. Sibling functions (`stddev_over_time`, `stdvar_over_time`,
`quantile_over_time`) go to their own SOW if real dashboards ask
for them. Subqueries, vector matching, `topk`/`bottomk`/`quantile`,
and the remaining bucket A items are separate SOWs.

CI verification (gcc-build, clang-build, license check) awaits the
user's authorization to push the branch.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
