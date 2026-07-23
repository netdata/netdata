# Query Contract Corpus

End-to-end correctness testing for the Netdata query engine, against a
completely **stock** `netdata` daemon: fixtures are ingested through the real
streaming protocol by a fake child (`pusher`), queries run over the normal
HTTP API, and assertions run on canonical query JSON. No test code inside the
daemon, no faked storage, no faked contexts.

## The layered ladder

The query pipeline composes through narrow interfaces: group-by consumes only
a per-metric series of `(value, flags, anomaly-rate)` and cannot see which
time-aggregation or tier produced it; a second group-by pass sees only
first-pass groups. Proving each layer independently therefore proves the
composition — no combinatorial multiplication across layers is needed,
provided every layer's fixtures cover the full value-shape palette of its
input interface.

- **Layer 0 — harness**: pusher/driver self-test; fixture data round-trips
  through the streaming protocol byte-exact; settle discipline.
- **Layer 1 — tier0 ingestion**: stored points equal pushed points, including
  gaps, resets, anomaly bits, negatives, zeros.
- **Layer 2 — tier rollups**: tier1+ points are the correct min/max/sum/count/
  anomaly-count derivation of tier0, including around gaps.
- **Layer 3 — time-aggregations**: every time-grouping function, one by one
  (average, sum, min, max, incremental-sum, median, trimmed-median, stddev,
  cv, ses, des, trimmed-mean, percentile+options, countif+options), with odd
  window/points alignments and per-function options.
- **Layer 4 — tier edges**: every time-aggregation across tier transitions —
  edges going up and going down, plan switching mid-window in both
  directions, overlapping tier retention, `selected-tier`.
- **Layer 5 — level-1 group-by**: every group-by key and aggregation, one by
  one, in BOTH contracts: non-raw and `raw` (raw is a second contract for
  group-by finalize — no AVERAGE division, no percentage conversion, no
  trimming, sums+counts+hidden emitted for Cloud). Metadata asserted inside
  every case: `summary.*`, `view.dimensions.sts`, `aggregated`, point
  annotations, anomaly rates.
- **Layer 6 — level-2 group-by**: every second-pass algorithm for each
  first-pass algorithm, non-raw and raw, same metadata assertions.
- **Layer 7 — formatters**: one pinned result rendered through every output
  format (json, json2, csv, ssv, csvjsonarray, datatable, html, value) and
  option sets — bugs here break before or after the engine runs.
- **Layer 8 — post-processing**: `options=percentage` (row-total), absolute,
  nonzero, null2zero, cardinality_limit (+`cardinality-limit-all`),
  partial-data trimming near now.
- **Layer 9 — window/API surface**: v1 vs v2/v3, aligned/relative windows,
  `points`/`gtime` resampling edge cases.
- **Cloud**: cloud-charts-service replays the raw halves of layers 5-6
  through the real `DataV2Aggregator` plus its merge semantics.

## The edge-data palette

"Representative data" is a named, fixed set of fixture shapes, reused across
layers; each layer declares which entries it consumes:

`complete`, `leading-gap`, `interior-gap`, `trailing-gap` (short retention),
`reset-flagged`, `anomalous`, `negative`, `all-zero`, `single-point`,
`mixed-update-every`, `two-children`.

## Rules

- Every layer's cases carry deterministic fixtures at a fixed 2023 epoch and
  hand-computable expectations (the driver computes them from the fixture
  definition — never from the engine under test).
- Every bug found while building a layer is proven RED here first, then fixed
  in its own focused branch/PR, then flipped GREEN here.
- The manifest tracks case -> proves-what -> agent status -> cloud status ->
  fixed-by.

The full developer contract — how to run the suite, author fixtures and
oracles, add red cases, flip them green, and change pins safely — is
`.agents/skills/project-query-corpus/SKILL.md`.
