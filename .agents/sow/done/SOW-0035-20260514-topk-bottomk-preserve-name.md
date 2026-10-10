# SOW-0035 - PromQL - Preserve __name__ in topk / bottomk output

## Status

Status: completed

Sub-state: `__name__` (and every other input label) now preserved on
`topk`/`bottomk` output. Compliance corpus 501 -> 511 (+10 cases on
aggregators.test, exactly the SOW estimate). 191/191 unit tests;
117/117 smoke (one smoke assertion that encoded the bug was inverted
to assert the new correct behaviour). Live verify confirms
`topk(3, system_cpu)` emits series with `__name__: "system_cpu"`.

## Requirements

### Purpose

`topk(k, v)` and `bottomk(k, v)` currently strip `__name__` from
their output series. Prometheus preserves it. This is a documented
real bug in `tests/compliance-data/EXPECTED_FAILS.md` and the
single largest remaining source of `aggregators.test` failures
after SOW-0034.

### User Request

User: "fix topk/bottomk __name__ preservation".

### Assistant Understanding

Facts:

- `apply_topk_bottomk` in `eval/aggregation.rs` builds
  `stripped_labels` by filtering out `__name__`, then uses the
  stripped form as both the output labels and the dedup key
  (`stripped_signatures`).
- Compliance failures of the form `topk(N, ...): no actual
  matches expected [("__name__", "..."), ...]` show the
  expected output preserves `__name__`; ours doesn't.
- The fix is a one-block change: use `s.labels` directly for
  the output and for the signature, instead of the stripped
  form.
- Per Prometheus' documentation: "topk and bottomk preserve
  the original metric name (`__name__`) and labels." The
  general aggregation convention (strip `__name__`) does not
  apply to these two.
- One existing unit test (`topk_returns_top_k`) asserts that
  `__name__` is stripped. It encodes the bug and must be
  inverted.

Inferences:

- Removing the strip means the dedup signature includes
  `__name__`. Multiple input series with the same value but
  different `__name__` would already have distinct signatures,
  so dedup remains correct.
- `count_values` (which also passes through this file)
  intentionally strips `__name__` because it constructs new
  bucket labels; that stays.

Unknowns: none.

### Acceptance Criteria

1. `topk` and `bottomk` output series carry the original input
   labels including `__name__` (when present on the input).
2. The bucket-grouping key still uses the projection (which
   includes the existing `__name__` strip via `project_labels`
   when no `by`/`without` clause is given) -- consistent with
   how Prometheus groups.
3. Compliance corpus: improves; aggregators.test gains at
   least 10 more passes (current 108, expected ≥ 118).
4. 191 unit tests pass after `topk_returns_top_k`'s assertion
   is inverted.
5. Live spot-check: `topk(3, system_cpu)` returns series whose
   metric label includes `__name__: "system_cpu"`.

Out of scope: any other aggregator's label projection.

## Pre-Implementation Gate

Status: ready. Problem: real bug. Plan: invert the strip in
`apply_topk_bottomk`; invert the corresponding unit test
assertion; verify compliance jumps.

Sensitive data handling plan: no sensitive data.

## Execution Log

### 2026-05-14

- SOW promoted.
- Fixed `apply_topk_bottomk`: output labels are `s.labels` not
  `stripped_labels`; output signature is `s.signature` not
  `stripped_signatures`. Removed the now-unused stripped_*
  precomputations.
- Inverted the `topk_returns_top_k` test assertion: it now
  asserts that `__name__` IS preserved.
- Updated EXPECTED_FAILS.md.

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) Output preserves `__name__`: **MET**.
- (2) Bucket-grouping key unchanged (still uses
  `project_labels`): **MET**.
- (3) Compliance ≥ +10 cases: **MET**, aggregators.test
  108 -> 118; total 501 -> 511.
- (4) 191 unit tests pass: **MET**.
- (5) Live spot-check: **MET** -- `topk(3, system_cpu)` returns
  series with `__name__: "system_cpu"` in the metric label set.

### Lessons

- One smoke assertion (`topk preserves dimension, strips
  __name__`) encoded the bug. It was the first thing to update
  when the fix landed; otherwise the smoke gate would
  permanently certify the wrong behaviour. Worth grepping
  smoke/unit tests for "strips __name__" patterns whenever a
  bug fix inverts a label-projection rule.
- The dedup signature for topk/bottomk previously used the
  stripped-labels signature. Switching to the input series'
  own `s.signature` is correct because different `__name__`s
  produce different signatures anyway, and same-`__name__`-same-
  rest-of-labels would already collide in the input.

### Followup

None.

## Regression Log

None yet.
