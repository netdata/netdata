# Expected failures

Lines in this file are formatted as `<file>#<at_ms>#<query>` and skip a
specific compliance case from counting as a real failure. Comments
(lines starting with `#`) are ignored.

The first run of the compliance corpus (SOW-0030) produced **468 pass
/ 332 fail / 212 skip** across 1,012 cases drawn from 10 vendored
`.test` files. The failures fall into the following categories, in
rough order of impact. Each is a follow-up SOW.

SOW-0031/0032/0033 (whole-range orchestration, column-oriented
Series, aggr × rollup fusion) brought the baseline to 472. SOW-0034
(the five missing aggregators) brought it to **501 pass / 299 fail
/ 212 skip**. The aggregators category is closed; the rest stand.

## Category: missing aggregators (CLOSED, SOW-0034)

The five missing aggregation operators -- `stddev`, `stdvar`,
`limitk(k, v)`, `limit_ratio(ratio, v)`, `group(v)` -- now ship.
`aggregators.test` improved from 80 -> 108 passes; the overall
corpus from 472 -> 501.

## Category: `topk` / `bottomk` `__name__` preservation

Prometheus' `topk(k, v)` and `bottomk(k, v)` return the *original*
input series, so `__name__` is preserved. Our implementation
(SOW-0021) drops `__name__` to match the aggregation convention. The
correct Prometheus semantic is to preserve it; this is a real bug
and should be fixed in a follow-up.

## Category: rate-on-cumulative-counters (SOW-0038 attempted, reverted)

Prometheus `.test` fixtures load counter samples as cumulative
values (`0+10x1000` -> `[0, 10, 20, ..., 10000]`); Prometheus' rate
computes `(last - first) / range`. Netdata stores INCREMENTAL
counter dimensions pre-differentiated as deltas; our rate computes
`sum(values) / span`. The two are mathematically equivalent only
when the input format matches the algorithm.

This affects ~30-40 compliance cases across `selectors.test`,
`functions.test`, `operators.test`, `subquery.test`, and
`name_label_dropping.test` (every test that calls
`rate(metric_total[...])` on cumulative input).

SOW-0038 attempted to fix this by (a) differentiating
counter-suffixed series at load time in the compliance harness,
and (b) switching our rate denominator from sample span to the
matrix selector's `range_ms`. Both changes were reverted:

- Load-time differentiation broke direct-selector queries on
  counter-named metrics (which expect cumulative values, e.g.
  `metric_total + rate(metric_total[1m]) * 60` in
  `operators.test`).
- The denominator switch alone didn't help (still very wrong
  numbers on cumulative input) and regressed one functions.test
  case.

The proper fix is a full Prometheus-style rate algorithm
(`last - first / range_ms` with counter-reset detection and
extrapolation, accepting cumulative input) -- a substantial
change that conflicts with Netdata's delta-storage convention,
requiring dual-mode rate. Not yet pursued.

## Category: counter-reset & staleness divergences

Several tests in `staleness.test` and counter-family tests in
`functions.test` exercise edge cases around stale markers, lookback
boundary samples, and counter-reset extrapolation. Our 5-minute
fixed lookback and shim-side reset handling diverge from
Prometheus' semantics by design (documented in the contract spec);
the harness surfaces these as failures so we can decide case by
case which to fix and which to accept.

## Category: result comparison artefacts (PARTIALLY CLOSED, SOW-0036)

SOW-0036 fixed the bare-scalar expected-line parser; `literals.test`
went from 0/25 to 20/5. The remaining 5 `literals.test` failures
are top-level string literals (`"Foo"`, `""`) that our lowering
layer rejects -- tracked as a separate follow-up.

The original note attributed this to the `same_labelset`
comparator. The actual bug lived in `parse_expected_line`: it
returned `None` for inputs without `{` or whitespace (a bare
`12340000` token), so the expected list ended up empty and the
comparator reported "expected 0 labelled series" against the
correct scalar actual.

## Category: range queries

`range_queries.test` is reported as 0/5/13 because SOW-0030 doesn't
implement the range-eval execution path. The harness parses the
`eval range from A to B step S <query>` form but the runner skips
those cases. Range eval will land in a separate SOW.

## Listed cases

(Empty for now. Once specific cases are reviewed and accepted as
divergent rather than buggy, add their `<file>#<at_ms>#<query>` key
on its own line below.)
