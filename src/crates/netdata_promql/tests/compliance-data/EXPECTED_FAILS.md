# Expected failures

Lines in this file are formatted as `<file>#<at_ms>#<query>` and skip a
specific compliance case from counting as a real failure. Comments
(lines starting with `#`) are ignored.

The first run of the compliance corpus (SOW-0030) produced **468 pass
/ 332 fail / 212 skip** across 1,012 cases drawn from 10 vendored
`.test` files. The failures fall into the following categories, in
rough order of impact. Each is a follow-up SOW.

## Category: missing aggregators

We don't yet implement these aggregation operators:

* `stddev` (aggregation of an instant vector -- distinct from
  `stddev_over_time` which we do have)
* `stdvar` (aggregation of an instant vector)
* `limitk(k, v)` (Prometheus 2.40+: limit-by-count)
* `limit_ratio(ratio, v)` (Prometheus 2.40+: limit-by-ratio)
* `group(v)` (returns 1 for every input series; used as a join key
  fabricator)

Adds ~5 variants to `AggrKind`. Trivial implementations; not in
SOW-0030 because the SOW is about the harness, not the gaps it
exposes.

## Category: `topk` / `bottomk` `__name__` preservation

Prometheus' `topk(k, v)` and `bottomk(k, v)` return the *original*
input series, so `__name__` is preserved. Our implementation
(SOW-0021) drops `__name__` to match the aggregation convention. The
correct Prometheus semantic is to preserve it; this is a real bug
and should be fixed in a follow-up.

## Category: counter-reset & staleness divergences

Several tests in `staleness.test` and counter-family tests in
`functions.test` exercise edge cases around stale markers, lookback
boundary samples, and counter-reset extrapolation. Our 5-minute
fixed lookback and shim-side reset handling diverge from
Prometheus' semantics by design (documented in the contract spec);
the harness surfaces these as failures so we can decide case by
case which to fix and which to accept.

## Category: result comparison artefacts

A few tests fail because our `same_labelset` comparator doesn't yet
handle:

* the `{} value` shape (labelless result series in
  `literals.test`) -- 0/25 pass rate on that file is almost
  certainly a comparator bug, not 25 real semantic deviations
* expected results with `__name__` in the labelset

These are bugs in the comparator, not in the evaluator.

## Category: range queries

`range_queries.test` is reported as 0/5/13 because SOW-0030 doesn't
implement the range-eval execution path. The harness parses the
`eval range from A to B step S <query>` form but the runner skips
those cases. Range eval will land in a separate SOW.

## Listed cases

(Empty for now. Once specific cases are reviewed and accepted as
divergent rather than buggy, add their `<file>#<at_ms>#<query>` key
on its own line below.)
