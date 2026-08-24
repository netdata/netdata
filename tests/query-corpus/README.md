# Query Contract Corpus

End-to-end correctness testing for the Netdata query engine against a
completely **stock** `netdata` daemon. A fake child ingests fixtures through
the real streaming protocol, queries use the normal HTTP API, and structured
results pass through strict typed decoders before semantic assertions.
Explicit formatter stability cases use labeled byte or shape pins. The daemon
contains no test hooks; storage and contexts are real.

## The layered ladder

The ladder isolates narrow query stages to control combinatorial growth.
Isolation does not prove every composition: dedicated cross-layer cases cover
tier seams, cadence changes, re-delivery, grouping options, and multipass
state where one stage changes another stage's inputs.

- **Layer 0 — harness**: exact typed fixture readback through live burst,
  legacy paced and replication ingestion, plus labels, independent children
  and journal-replay restart; retention barriers and deliberate disconnect
  regressions exercise delivery and teardown.
- **Layer 1 — tier-0 storage**: storage-number quantization, timestamps, gaps,
  reset/anomaly flags, incremental-rate collection, precision, and
  restart-sensitive gap states.
- **Layer 2 — higher-tier rollups**: aligned sum/min/max/count/anomaly-count
  windows, empty and absent windows, float32 page rounding, multiple
  collection intervals, and tier 2. Higher tiers aggregate the original
  collected `STORAGE_POINT`, not the tier-0 packed value.
- **Layer 3 — time groupings**: source-derived arithmetic for the registered
  stateless and stateful families and aliases, including `latest`, option
  handling, sparse buckets, and smoothing state. CASE-023 separately owns
  strict condition-expression and fleet-grouping contracts.
- **Layer 4 — tier selection and joins**: forced-tier oracles, automatic
  selection, overlapping retention, two-tier plan switching, and a
  three-tier join, with explicit seam controls.
- **Layer 5 — first-pass group-by**: the declared key/aggregation matrix,
  percentage behavior, multi-key grouping, metadata/statistics, and both
  non-raw and aggregatable raw responses.
- **Layer 6 — multipass group-by**: selected key/aggregation chains plus
  explicit first-pass-average and second-pass-percentage contracts, in
  non-raw and raw modes, including contributor-weighted anomaly metadata.
- **Layer 7 — formatters**: explicit stability and validity checks across
  classic v1 formats and relevant option sets.
- **Layer 8 — post-processing**: percentage, absolute, nonzero, null-to-zero,
  cardinality limits, and partial-data trimming.
- **Layer 9 — view/window surfaces**: bounded virtual-point interpolation
  checks, natural-points pins, relative/default/live windows, resampling, and
  v2/v3 parity.
- **Layer 10 — grouping invariants**: roster-driven sweeps over every
  requestable grouping declared by the paired source tree, across the finite
  tier/resolution matrices named by each case.
- **Layer 11 — slicing laws**: deterministic pairwise and seeded randomized
  additivity/conservation checks with explicit approximation bounds at
  stored-record edges.
- **Cross-cutting surfaces**: CASE regressions plus selector, option,
  anomaly-bit, rate, reset, update-every, and weights contracts complement
  the numbered layers.
- **Cloud boundary**: this repository does not run `cloud-charts-service` or
  `DataV2Aggregator`; the manifest's Cloud column is reserved for external
  replay and status tracking.

## The edge-data palette

"Representative data" is a named, fixed set of fixture shapes, reused across
layers; each layer declares which entries it consumes:

`complete`, `leading-gap`, `interior-gap`, `trailing-gap` (short retention),
`reset-flagged`, `anomalous`, `negative`, `all-zero`, `single-point`,
`mixed-update-every`, `two-children`.

## Rules

- **One green/red verdict represents one semantic invariant.** A manifest
  contract may exercise several inputs that all prove the same claim, but it
  must not combine independently actionable claims such as numeric values and
  units. Give independent claims separate contract keys and register them in
  separate test or subtest scopes. Otherwise a known failure in one claim can
  hide a new regression in another while the broken-contract set appears
  unchanged.
- Class A fixture-derived expectations are the default. Deterministic fixtures
  normally use `fixture.T0`; cases that must touch wall-clock time assert
  bounded envelopes.
- Engine-design transforms may use source-revision-cited Class B ports.
  Explicit Class C byte/parity pins prove stability or coherence, not
  first-principles correctness.
- A new engine defect is first stated as the correct contract in a failing
  corpus case, then fixed in a separate focused branch/PR.
- **A broken contract fails. Always.** On master, on a feature branch, whether
  or not the break is already known. There is no recorded "expected failure"
  anywhere in this suite: a corpus that reports success on a broken engine is
  worse than no corpus, because it is the thing you would trust. An unfiltered
  run ends with the complete, deduplicated list of broken contracts and fails
  if any manifest contract or required test component did not run. A filtered
  daemon-backed run reports its partial coverage and never claims the complete
  corpus holds. The explicitly named daemon-free harness/unit fast path prints
  only its Go test result and no query-contract verdict.
- The manifest tracks case -> proves-what -> cloud status -> fixed-by. It
  records no expected outcome; only running a case can say whether it holds.

The full developer contract — how to run the suite, author fixtures and
oracles, add a case for a new bug, and change pins safely — is
`.agents/skills/project-query-corpus/SKILL.md`.
