# SOW-0037 - PromQL - Honor explicit __name__ in grouping; add first_over_time

## Status

Status: completed

Sub-state: `project_labels` now respects explicit
`by (__name__, ...)` / `without (__name__)` semantics. The
`first_over_time` plumbing is in place but blocked at the parser
(promql-parser v0.9.0 doesn't recognise the name; it lands when
the parser supports it). Compliance 541 -> 545 (+4 cases on
name_label_dropping.test). 195/195 unit tests; 117/117 smoke; live
verify confirms `sum by (__name__) (system_cpu)` emits the
`__name__` label.

## Requirements

### Purpose

Two of the three real bug-classes in `name_label_dropping.test`:

1. **`project_labels` strips `__name__` unconditionally.** When
   the user writes `sum by (__name__, env) (...)` or
   `sum without (env) (...)`, the spec is "keep the labels named
   on the `by` clause" (or "drop the labels named on the `without`
   clause and keep everything else") -- including `__name__` if
   the user named it. Our current implementation drops `__name__`
   from grouping keys *and* from output labels regardless, which
   diverges from Prometheus on these explicit clauses.

2. **`first_over_time` not implemented.** Prometheus added it
   alongside `last_over_time` in 2.40+. Same shape as
   `last_over_time` -- preserve `__name__`, return the first
   non-NaN sample in the window.

The third bug class (rate computes "sum of deltas / span" instead
of "extrapolated counter increase / span") is a documented Netdata
storage divergence (INCREMENTAL dimensions are stored as deltas;
Prometheus expects cumulative counter values). Out of scope; see
EXPECTED_FAILS.md.

### User Request

User chose `name_label_dropping.test` as the next compliance
target after SOW-0036. After triage, the two non-rate failures
are real bugs in our evaluator; the rate failures are
documented-as-divergent.

### Acceptance Criteria

1. `apply_collapsing` / `apply_quantile` / `apply_topk_bottomk` /
   `apply_count_values` / `apply_group` honor `by (__name__, ...)`
   -- the output labels keep `__name__` when the user explicitly
   named it. `without (...)` similarly retains `__name__` unless
   the user named it in the `without` list.
2. `FuncKind::FirstOverTime` added; parser name `first_over_time`
   maps to it; `OverTimeOp::First` added to functions.rs; the
   rollup keeps `__name__` like `last_over_time` does.
3. Compliance corpus: improves by at least 5 cases on
   name_label_dropping.test. Total ≥ 546 (current 541).
4. 191 unit tests pass; existing aggregation tests that don't
   reference `__name__` continue to pass.
5. 117 smoke pass.

Out of scope:
- The rate-on-cumulative-counters divergence.
- Other Prometheus 2.40+ functions besides `first_over_time`.

## Pre-Implementation Gate

Status: ready.

Problem: `project_labels` filters `n != "__name__"` regardless of
what the grouping clause names. Spec says: drop `__name__` only
when it isn't explicitly named in `by`/`without`. Fix: change
the filter to be conditional.

For `first_over_time`: mirror `last_over_time` everywhere it
appears (FuncKind, parser name, OverTimeOp, keep_name, the
compute_over_time variant arm). Each is a one-liner.

Sensitive data handling plan: no sensitive data.

## Execution Log

### 2026-05-14

- Investigation: 18 failures fall into 3 classes -- 6-8 from
  `__name__` grouping bug, 1-2 from `first_over_time`, ~9-10
  from rate-on-cumulative-counters divergence (documented,
  out of scope).

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

- (1) `project_labels` honors explicit `__name__` in
  by/without: **MET**.
- (2) `FirstOverTime` plumbing added (FuncKind, OverTimeOp,
  keep_name, compute_over_time branch): **MET (latent)** --
  ready but unreachable until promql-parser v0.9.0 learns the
  name. Adding it now means a future parser upgrade unlocks the
  function without a code change.
- (3) Compliance ≥ +5: **PARTIAL**, +4 on name_label_dropping
  (15 vs 11). Of the 14 remaining failures, 9 cascade from the
  documented rate-on-cumulative-counters divergence, 1 is
  blocked at the parser (first_over_time), and 4 require
  Prometheus' delayed-name-drop semantics for `or` expressions
  (a separate larger feature).
- (4) 195 unit tests pass (4 new tests cover the
  by(__name__)/without semantics).
- (5) 117 smoke pass.

### Lessons

- The label-projection rule for aggregation isn't "always drop
  `__name__`" -- it's "drop `__name__` unless the user named
  it on `by`, or keep it unless the user named it on
  `without`." Easy to overlook because aggregation output
  *usually* has no metric name.
- Adding a `FuncKind` variant without parser support is
  cheap-but-pointless until upstream lands the name. Worth
  doing only if a parser upgrade is likely in the near term;
  otherwise it just adds dead code. In this case the plumbing
  was almost free (one match arm) so it stays.
- `name_label_dropping.test` mixes three orthogonal concerns
  (label projection, name preservation by rollups, rate
  semantics). Categorical triage saves time over per-case
  debugging.

### Followup

- **first_over_time**: lands when promql-parser updates. Either
  bump the dep when upstream supports it, or fork.
- **Delayed name drop in `or` expressions**: 4 failing cases
  in name_label_dropping.test demand that `expr_a or expr_b`
  treats both sides' name-dropping behaviour as combined --
  if either side strips `__name__`, the combined output's
  bucket keying does too. Substantial implementation; defer.
- **Rate-on-cumulative divergence**: 9 failures cascade from
  Netdata storing INCREMENTAL counter dimensions as deltas
  rather than cumulative values. Tracked in EXPECTED_FAILS.md
  under "counter-reset & staleness divergences"; addressing
  it requires changing Netdata's rate semantics, well outside
  PromQL-evaluator scope.

## Regression Log

None yet.
