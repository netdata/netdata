# SOW-0025 - PromQL - `@` modifier

## Status

Status: completed

## Requirements

### Purpose

The `@` modifier evaluates a vector or matrix selector at a fixed
timestamp instead of the query's evaluation time. Two forms:

* `<selector> @ <unix_ts>` -- fixed Unix timestamp (float seconds).
* `<selector> @ start()` / `@ end()` -- the outer query range's
  start/end. For an instant query, both collapse to the query time.

This is the second-smallest of the four remaining bucket A items
(after count_values).

### User Request

"Let's continue with the `@` at-modifier."

### Assistant Understanding

Facts:

- The `promql_parser` represents `@` as
  `VectorSelector.at: Option<AtModifier>` and
  `MatrixSelector.at: Option<AtModifier>`.
- `AtModifier` has three variants: `At(SystemTime)`, `Start`, `End`.
- Our Plan IR currently ignores the `at` field at lowering.
- `EvalContext` carries `at_ms`; range evaluation in `lib.rs` updates
  `at_ms` per step but does not propagate the outer `[start_ms,
  end_ms]` bounds. `@ start()` / `@ end()` need those bounds.

Inferences:

- `Plan::VectorSelect` and `Plan::MatrixSelect` each gain an optional
  `at_ms: Option<AtMod>` field.
- `EvalContext` gains `outer_start_ms`/`outer_end_ms` so the selector
  evaluator can resolve `@ start()` / `@ end()`. For instant queries,
  both are set to the query time.
- When `at_ms` is `Some(value)`, the selector evaluator uses that
  value instead of `ctx.at_ms` for both lookback (vector) and window
  (matrix) calculations.

### Acceptance Criteria

1. New `AtMod` enum in plan/ir.rs: `AtTs(i64)`, `AtStart`, `AtEnd`.
2. `Plan::VectorSelect` and `Plan::MatrixSelect` gain
   `at: Option<AtMod>`. Lowering reads the parser's at field and
   maps it.
3. `EvalContext` gains `outer_start_ms` and `outer_end_ms`. lib.rs
   populates both for instant (= query time) and range
   (= [start_ms, end_ms]) calls.
4. `eval/select.rs` resolves `at` to an effective timestamp before
   computing the lookback / window. The offset modifier still
   applies on top.
5. Rust unit tests cover lowering, instant `@ ts`, instant
   `@ start()`/`@ end()`, matrix selector with `@`.
6. Smoke harness gains 3+ checks against the live daemon:
   - `system_cpu @ <now>` produces the same result as `system_cpu`.
   - `system_cpu @ <future>` produces an empty result (lookback
     window doesn't cover the future).
   - `system_cpu @ start()` in an instant query equals
     `system_cpu`.

7. Spec moves `@ modifier` from out-of-scope to supported.

Out of scope:

- `@` on subqueries (subqueries are still deferred).
- `keep_metric_names` and subqueries -- separate SOWs.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The promql-parser parses `@` correctly but our lowering drops it.
The evaluator uses a single `at_ms` from the context. Adding the
`AtMod` field plus two new EvalContext fields and a small effective-
timestamp computation in `eval/select.rs` is the whole change.

Affected contracts and surfaces:

- Modified: `plan/ir.rs` -- AtMod enum; at field on VectorSelect/
  MatrixSelect.
- Modified: `plan/lower.rs::lower_vector_select`/`lower_matrix_select`
  -- map the parser's at field.
- Modified: `eval/context.rs` (or wherever EvalContext lives) --
  outer_start_ms / outer_end_ms fields.
- Modified: `eval/select.rs` -- compute effective_at = at_modifier
  resolution + offset before lookback / window.
- Modified: `lib.rs::run_instant`/`run_range` -- populate the new
  context fields.
- Modified: `eval/dispatch.rs` -- destructure new fields.
- Modified: spec and smoke harness.

No FFI changes. No shim changes.

Existing patterns to reuse:

- The Phase 1 offset-modifier handling at the lookback computation.

Risk and blast radius:

- Plan IR field additions only; default to None preserves Phase 1
  behavior.
- EvalContext field additions are populated by both query paths;
  callers that don't care about @ never read them.

Sensitive data handling plan: no new data classes.

Implementation plan (single chunk):

1. Add AtMod enum to plan/ir.rs.
2. Add `at: Option<AtMod>` to VectorSelect and MatrixSelect.
3. Lowering: map the parser's at into AtMod.
4. EvalContext: add outer_start_ms, outer_end_ms.
5. lib.rs: populate the new context fields for instant and range
   queries.
6. eval/select.rs: compute effective_at_ms before lookback /
   window.
7. Rust unit tests + smoke checks + spec + close.

Validation plan:

- Rust unit tests: target 116+.
- Smoke harness: target 91+.

Artifact impact plan:

- Specs: updated.
- AGENTS.md, runtime project skills, end-user docs/skills: no change.
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence: Prometheus `@` semantics from
`promql/parser/ast.go`.

## Execution Log

### 2026-05-14

- SOW drafted.
- Promoted + shipped in a single commit:
  - `AtMod` enum (AtTs, Start, End) added to plan/ir.rs; re-
    exported from plan/mod.rs.
  - `Plan::VectorSelect` and `Plan::MatrixSelect` gain
    `at: Option<AtMod>`. Existing destructures use `..` so the
    addition is non-breaking; one test destructure was updated.
  - Lowering: `lower_at` helper maps the parser's
    `AtModifier::At(SystemTime)` to a Unix-ms i64, and Start/End
    pass through as enum variants. Pre-epoch timestamps encode as
    negative i64 ms.
  - `EvalContext` gains `outer_start_ms`/`outer_end_ms`. `lib.rs`
    populates them for both instant (= query time) and range
    (= the supplied [start, end]) queries.
  - `eval/select.rs`: new `resolve_at` helper resolves the
    AtMod against the context (AtTs -> ms; Start -> outer_start;
    End -> outer_end). Both `eval_vector_select` and
    `eval_matrix_select` use the resolved time as the lookback /
    window right edge; output samples still stamp at the natural
    query step time, matching Prometheus.
- 3 new Rust unit tests cover lowering of `@ <ts>`, `@ start()`,
  `@ end()`, and `@` on a matrix selector. 116/116 Rust tests
  pass (was 113).
- 4 new smoke checks under Phase 3g: `@ <now>` matches bare
  selector, `@ <future>` returns empty, `@ start()` on instant
  query returns all series, `@` on matrix selector through
  `rate()`. 92/92 total smoke checks pass (was 88).
- Spec extended: `@` modifier moves from out-of-scope to supported
  for vector/matrix selectors; `@` on subqueries notes it waits
  for subqueries themselves.
- SOW closed; file moves to done/ in the same commit.

## Validation

- ACs all evidenced above. 116/116 Rust tests; 92/92 smoke.
- Live daemon: each of the four AC#6 cases produces the expected
  result.
- Artifact gate: AGENTS.md / runtime project skills /
  end-user docs / end-user skills unchanged; spec updated;
  SOW lifecycle completed.
- Sensitive data: no new data classes.
- Same-failure scan: no `*_phase_2` test rebrand needed; the
  obsolete out-of-scope entry in the spec was removed and the
  feature added to supported.

## Outcome

The `@` modifier works on both vector and matrix selectors, with
fixed-timestamp, start(), and end() forms. The branch is 21
commits ahead of `origin/master`.

## Lessons

- `@` on selectors is small because the parser already does the
  heavy lifting. Lowering plus an effective-time computation is
  the whole change. A useful pattern for narrow PromQL extensions
  whose syntax is already in the parser.

## Followup

1. `@` on subqueries -- arrives with subqueries themselves.
2. `keep_metric_names` (next smallest remaining item).
3. Subqueries (largest remaining item).
4. CI verification on push (carry-over).

## Regression Log

None yet.
