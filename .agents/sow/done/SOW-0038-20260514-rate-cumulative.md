# SOW-0038 - PromQL - Rate on cumulative counters

## Status

Status: completed

Sub-state: tried two paired changes -- compliance harness load-time
differentiation, eval rate denominator switch -- both reverted.
The rate-on-cumulative-counters divergence is genuinely
architectural and can't be closed with surgical fixes. Net result:
no code change shipped; this SOW documents the failed attempt and
the path forward.

## Requirements

### Purpose

The Prometheus `.test` format loads counter samples as cumulative
values (`0+10x1000` expands to `[0, 10, 20, ..., 10000]`).
Prometheus computes `rate()` as `(last - first) / range`. Netdata
storage stores INCREMENTAL counter dimensions pre-differentiated
as deltas, and our `rate()` computes `sum(values) / span`. Both
shapes are equivalent **iff** the input format matches the rate
algorithm. Today they don't.

This causes 17 failures in `selectors.test`, plus cascading
failures in `functions.test`, `name_label_dropping.test`,
`operators.test`, and probably others. Fix is a paired change:

1. Compliance harness differentiates counter-shaped test data
   (`_total`/`_count`/`_sum` suffixed metric names) at load
   time, with counter-reset handling -- so the data reaches
   `MemBackend` as deltas, matching Netdata's storage
   convention.
2. The rate/increase denominator switches from the actual
   sample span to the matrix selector's `range_ms`. This
   matches Prometheus' "rate is increase over the requested
   range" semantic and accounts for the boundary gap that
   Prometheus' extrapolation handles. Production blast radius
   is small (rates on tight windows shift by a few percent).

### User Request

User selected `selectors.test` as the next compliance target.
Triage revealed all 17 failures cascade from this divergence,
which also blocks ~25-40 cases across other files.

### Acceptance Criteria

1. Compliance harness: a `differentiate_counter_samples` helper
   transforms cumulative samples to deltas before pushing into
   `MemBackend`. Applied to series whose `__name__` ends in
   `_total`, `_count`, or `_sum`. Handles counter resets
   (`v[i] < v[i-1]` -> `delta = v[i]`) and NaN samples
   (pass through unchanged).
2. Eval `compute_window_op` for `WindowOp::Rate` divides by
   `range_ms / 1000` (in seconds) instead of `last_ts -
   first_ts`. Falls back to sample span when `range_ms` is
   `None` (legacy/test-only single-window callers).
3. Compliance: `selectors.test` improves from 14 to ≥ 28
   passes (the 17 rate failures should clear). Total ≥ 559
   (currently 545; expect +14 to +40 depending on cascades).
4. 195 unit tests pass. `apply_window_op_legacy` keeps using
   span (it doesn't get a range_ms from the test caller); the
   rate-related unit tests assert the legacy-path numbers
   which stay the same.
5. 117 smoke pass. Live spot-check: `rate(system_cpu[1m])`
   still produces sensible values on the daemon (Netdata
   deltas + range_ms denominator).

Out of scope:
- `irate` (uses last two samples; different algorithm).
- `increase` reuses the same denominator/sum path; it'll
  trace through automatically.
- Prometheus' rate extrapolation for boundary cases (samples
  not exactly at the window edge). The current fix gets us
  Prometheus-equivalent on the common test cases; an exact
  match on extrapolation would need additional logic.

## Pre-Implementation Gate

Status: ready.

Risk: changing the rate denominator is a production-behaviour
change. Live spot-check on the daemon will confirm the
typical rate value stays in a reasonable range. The change is
small (a few percent on tight windows) and brings us closer
to Prometheus' semantic.

Sensitive data handling plan: no sensitive data.

## Execution Log

### 2026-05-14

- Investigated `selectors.test`: all 17 failures involve
  `rate(http_requests_total[...])`. Test data is cumulative
  `0+10x1000`; our rate sums-and-divides-by-span, giving
  values 957× the expected.
- Confirmed the same root cause cascades into `functions.test`
  (16 rate cases), `name_label_dropping.test` (9 rate cases),
  `operators.test` (2 rate cases), `subquery.test` (2 rate
  cases). Total estimated yield 30-40 cases.

## Validation / Outcome / Lessons / Followup

### Acceptance criteria status

All criteria **NOT MET**. Net code shipped: zero.

- (1) Harness differentiation: **TRIED, REVERTED**. Worked
  in isolation for rate-style queries but broke direct-
  selector queries on counter-named metrics (which expect
  cumulative values). The corpus mixes both patterns in the
  same files, so naming-based discrimination is not enough.
- (2) Eval denominator switch (`range_ms` instead of sample
  span): **TRIED, REVERTED**. By itself, neither helps
  the rate-on-cumulative case (still 957× off after the
  switch -- ratio nearly unchanged) nor produces a clear
  compliance win. Cost one unit-test breakage and a small
  functions.test regression (-1).
- (3) Compliance ≥ 559: **NOT MET**. Stayed at 545.

### Lessons

- **Triage before commitment.** I jumped from "selectors.test
  17 failures" to "rate-on-cumulative SOW" without first
  confirming that the supposed cascade across other files
  would unlock anything. In reality the cascade was an
  illusion: most rate failures in other files are
  independently rate-on-cumulative, with the same structural
  issue, not downstream of any harness-level fix.
- **Naming-based discrimination fails when the same metric
  is used both as counter (rate input) and as gauge
  (direct selector).** Prometheus' test corpus does both
  with `http_requests_total`. There's no clean per-load
  decision.
- **The proper fix is implementing Prometheus' rate algorithm
  end-to-end**: `(last - first) / range_ms` with counter-reset
  detection and extrapolation, accepting cumulative input.
  That's a substantial change to the rate semantic and conflicts
  with Netdata's INCREMENTAL-counter delta storage convention.
  A dual-mode rate (counter-shape detection at query time, or
  an EvalContext flag) is the only path that satisfies both
  audiences. Not in scope for a small compliance SOW.
- **Compliance corpus chasing has hit diminishing returns**
  for me. After SOW-0034 / 0035 / 0036 / 0037 jumped from
  468 -> 545 (+77 cases), the remaining ~250 failures cluster
  around three durable divergences -- rate-on-cumulative,
  Prometheus' delayed-name-drop in `or` expressions, native
  histograms -- each of which is its own significant feature.
  Tactical surgical fixes won't move the number.

### Followup

- **Don't pursue this SOW's design further.** The right
  follow-up is either (a) a full Prometheus-style rate
  implementation as a separate, larger SOW, or (b) accept
  the divergence as permanent and document the affected
  case count more concretely.
- **Update EXPECTED_FAILS.md** to record this attempt and
  the diagnosis so a future contributor doesn't repeat it.

## Regression Log

None yet.
