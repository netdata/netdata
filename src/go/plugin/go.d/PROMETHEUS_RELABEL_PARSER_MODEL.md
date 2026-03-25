# Prometheus Relabel-Capable Parser Model

## TL;DR

- Facts:
  - The current normalized parser drops information that Prometheus metric relabeling can legitimately use.
  - A clean Prometheus v2 design therefore needs a relabel-capable parsed sample model before typed `metrix` assembly.
- Specification:
  - The first parser scope is classic text exposition only.
  - The parser must preserve enough structure for upstream-style `metric_relabel_configs`, then feed a strict post-relabel typed-family assembly step with explicit failure and performance contracts.

## Status

- Synchronized revision after round-3 adversarial review.
- Purpose of this revision:
  - align this parser/relabel note with the locked decisions in `TODO-PROMETHEUS-V2-MIGRATION.md`
  - remove stale “still open” wording for decisions that are already locked
  - make the collector-to-`metrix` bridge explicit enough for implementation review
- Current alignment:
  - first successful schema for an exported histogram/summary metric name is the schema lock
  - later schema drift is invalid in the initial implementation
  - for Prometheus phase 1, summary quantile schema is established from the first grouped summary observation before the first quantile-bearing write

## Document Authority

- This file is a derived implementation-contract note.
- The authoritative source of truth is:
  - `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/plugin/go.d/TODO-PROMETHEUS-V2-MIGRATION.md`
- If this file and the TODO disagree:
  - the TODO wins
- This file exists to restate the parser/relabel/assembly/bridge contract in one focused place.

## Facts And Constraints

### Facts from the current codebase

- The current parser normalizes classic histogram and summary subseries back to a base metric family name:
  - `_sum` -> base name in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
  - `_count` -> base name in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
  - `_bucket` -> base name in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
- The current parser removes structural relabel-relevant labels from the ordinary label set:
  - `quantile` is removed in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
  - `le` is removed in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
- Upstream Prometheus metric relabeling runs after target-label merge and conflict resolution in `/Users/ilyam/Projects/prometheus/scrape/scrape.go`.
- Upstream Prometheus defaults `HonorLabels` to `false` in `/Users/ilyam/Projects/prometheus/config/config.go`.
- `metrix` already imposes hard structural constraints:
  - histogram base labels must not contain `le` in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/metrix/histogram.go`
  - summary base labels must not contain `quantile` in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/metrix/summary.go`
  - summary quantile schema is fixed through descriptor options in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/metrix/options.go`
  - flattened histogram/summary output names are fixed in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/metrix/reader.go`
- `charttpl` requires visible metric names to be known during validation in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/plugin/framework/charttpl/validate.go`.
- The current parser already follows a reuse-oriented style:
  - parser state is reused across scrapes in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`
  - label slices are reset and reused in the hot path in `/Users/ilyam/Projects/github/ilyam8/netdata/src/go/pkg/prometheus/parse.go`

### User decisions already locked

- Relabel stays Prometheus-specific and collector-local.
- First implementation direction is a Netdata-owned relabel engine, not another Prometheus dependency.
- Relabel syntax should stay as close as practical to upstream `metric_relabel_configs`.
- Clean end design is preferred over short-term churn reduction.
- If the current parser blocks correct relabel behavior, design a new relabel-capable parser model.
- The first parser scope is classic text exposition only.
- The parser must prioritize performance over cleverness or pretty internals.
- The initial relabel-visible label set is intentionally narrower than full Prometheus scrape target-label semantics:
  - scraped sample labels
  - synthetic `__name__`
  - no collector-injected target labels
- Scoped relabel-block selectors are allow/deny lists of Prometheus-profile-specific selector expressions.
- The post-relabel typed-family contract must be defined before implementation.
- Ownership, lifetime, copy boundaries, and reuse guarantees must be explicit before implementation.

## Scope

### In scope for the first parser design

- Classic Prometheus text exposition.
- Float samples for:
  - gauge
  - counter
  - untyped
  - classic histogram bucket/sum/count
  - classic summary quantile/sum/count
- A parsed sample model that preserves everything needed for collector-local metric relabeling.
- A strict post-relabel assembly contract that feeds typed `metrix` writes.

### Out of scope for the first parser design

- Native histograms.
- Gauge histograms.
- Exemplars.
- Created/start timestamps as first-class assembly inputs.
- A generic parser abstraction for every Prometheus ingestion use case in the repo.

## Design Goal

- Facts:
  - `charttpl` and `chartengine` should only see post-relabel stored metrics.
  - Relabeling must therefore happen before typed `metrix` write decisions are finalized.
- Specification:
  - The collector pipeline should be:
    1. parse scrape text into a relabel-capable sample stream
    2. construct the relabel-visible label set
    3. apply Prometheus-style metric relabel rules
    4. assemble surviving samples into typed families
    5. write typed metrics to `metrix`
    6. let `charttpl` validate and materialize against the flattened post-store namespace

## Parsed Sample Model

### Purpose

- The parser model is not the final storage model.
- It exists to preserve raw scrape semantics needed by relabeling and by later typed-family assembly.

### Required sample fields

- `Name`
  - The exact scraped metric name for the current sample.
  - Facts:
    - this must preserve `_bucket`, `_sum`, `_count`, and base summary names as scraped
    - this field is the source of synthetic relabel label `__name__`
- `Labels`
  - The exact scraped sample labels, excluding `__name__`
  - Facts:
    - `le` and `quantile` must remain present when they were present in the scrape
    - label ordering/canonicalization rules must be explicit
- `Value`
  - Float sample value
- `Kind`
  - A parser-level role, not a final `metrix` kind
  - Allowed first-scope roles:
    - `scalar`
    - `histogram_bucket`
    - `histogram_sum`
    - `histogram_count`
    - `summary_quantile`
    - `summary_sum`
    - `summary_count`
- `FamilyType`
  - Best-effort family type metadata as observed from `# TYPE`
  - This is advisory input for later typed-family selection

### Deliberately excluded from the hot-path sample object

- `Help`
- `Unit`
- comment text
- any fully materialized per-family aggregation state

Reason:

- Facts:
  - these are not needed for relabel execution on a per-sample basis
  - carrying them on every sample would add hot-path cost with no relabel value
- Specification:
  - family metadata may be tracked out of band if needed later, but it is not part of the required per-sample relabel contract

## Parser Contract

### Ownership and lifetime

- Specification:
  - The parser is single-pass.
  - Samples are exposed in scrape order exactly once.
  - Parser-owned backing memory is scratch memory.
  - Sample name bytes, label values, and any parser-owned scratch are only guaranteed until the next parser advance.
  - Downstream code must copy only when it needs to retain data beyond the current iteration.
- Facts:
  - This matches the reuse-oriented style in the current Netdata parser.
  - This also mirrors the lifetime style of upstream `textparse.Parser`, which invalidates returned byte slices after the next `Next()`.

### Allocation and reuse

- Specification:
  - No mandatory per-sample heap allocation in steady state.
  - The parser may grow slices or symbol tables when capacity is insufficient, but steady-state processing should reuse already allocated backing storage.
  - The parser may use internal object reuse aggressively even if the internals are not pretty.
- Facts:
  - This matches the user’s stated performance requirement.

### Label rules

- Specification:
  - The parser must expose the full scraped sample label set except `__name__`, which is carried separately as `Name`.
  - `le` and `quantile` must remain visible in `Labels`.
  - The parser must not normalize `_bucket`, `_sum`, `_count`, or summary quantile samples into base-family form before relabeling.
  - The parser must not drop structural information that would change relabel-visible semantics.

### Error handling

- Specification:
  - Invalid exposition input may be skipped or rejected according to parser policy, but the parser contract must not silently rewrite valid relabel-relevant sample identity into a different identity before relabel.

## Relabel-Visible Label Set Contract

### Facts

- Upstream metric relabeling can see sample labels after target-label merge and conflict resolution.
- The current Prometheus v2 collector design in this task does **not** adopt that target-label merge model for the initial implementation.

### Specification

- The relabel engine input for each sample is:
  - parsed sample labels
  - plus synthetic `__name__ = sample.Name`
- The relabel engine runs on that label set directly.
- If relabel drops the sample, it is discarded before typed-family assembly.
- If relabel rewrites `__name__` to empty or invalid output, the sample is discarded.

### Boundary of this contract

- Specification:
  - The initial implementation does not inject collector-owned target labels such as `job` or `instance` into the metric-relabel input.
  - The initial implementation does not provide Prometheus-style `exported_*` collision-preservation behavior because there is no pre-relabel target-label merge step.
- Rationale:
  - this keeps relabel scoped to scraped sample labels plus synthetic `__name__`

## Relabel Rule Semantics

- Specification:
  - The collector-owned relabel engine should accept upstream-style `metric_relabel_configs`.
  - Ordered rule execution matters.
  - `__name__` must be supported as a synthetic relabel label.
  - The engine contract must distinguish:
    - final keep/drop decision
    - final metric name
    - final label set
- Constraint:
  - This document does not yet define the engine implementation details.
  - It defines the data contract the engine must consume and produce.

## Scoped Relabel Selector Contract

- Specification:
  - each Prometheus relabel block is scoped by a selector with:
    - `allow`
    - `deny`
  - each selector entry is a Prometheus-profile-specific sample selector expression
  - selector grammar and evaluation semantics reuse the existing Prometheus selector package:
    - `allow` entries are ORed
    - `deny` entries are ORed and override allow
    - empty selector means no selector restriction
    - invalid selector strings fail at init-time compilation/validation
  - selector matching happens before relabel over:
    - raw parsed sample name
    - raw scraped labels
  - raw classic histogram/summary subseries are matched by their raw names and structural labels:
    - `*_bucket`
    - `*_sum`
    - `*_count`
    - `le`
    - `quantile`
  - selector lists are compiled and validated at profile/job init
- Boundary:
  - this selector layer is Prometheus-profile-specific
  - it is not the same as charttpl/chartengine selector policy over post-store metrics

## Post-Relabel Typed-Family Assembly Contract

### Goal

- Facts:
  - `metrix` does not consume raw Prometheus sample roles directly.
  - It consumes typed metric families with explicit histogram/summary invariants.
- Specification:
  - Typed-family assembly is the boundary between Prometheus scrape/relabel semantics and Netdata `metrix` semantics.

### Global rules

- The assembly stage operates on relabeled samples only.
- Raw samples that match no scoped relabel block pass through unchanged into the post-parse pipeline.
- Identity keys are computed from the final post-relabel metric name and final post-relabel labels.
- Duplicate/conflicting final identities must not silently overwrite each other.
- The assembly stage must be deterministic for a given scrape input.

### Scalar families

- A scalar sample is a post-relabel sample with role `scalar`.
- Final emitted metric name is the final relabeled `__name__`.
- Final emitted label set is the final relabeled label set without `__name__`.
- Final metric kind selection is collector policy, using available family type metadata and compatibility rules such as existing `fallback_type`.
- Conflict rule:
  - if two scalar samples collapse to the same final metric name plus final labels within one scrape, that final identity is conflicted for the scrape and must not be emitted as last-write-wins

### Histogram families

- Histogram assembly is driven by post-relabel sample roles:
  - `histogram_bucket`
  - `histogram_sum`
  - `histogram_count`
- Structural validity rules:
  - bucket samples must retain a final metric name ending in `_bucket`
  - bucket samples must retain exactly one `le` label
  - sum samples must retain a final metric name ending in `_sum`
  - count samples must retain a final metric name ending in `_count`
- Base identity derivation:
  - base name for buckets is `trim_suffix(final_name, "_bucket")`
  - base name for sum is `trim_suffix(final_name, "_sum")`
  - base name for count is `trim_suffix(final_name, "_count")`
  - base labels are final labels without `le`
  - group key is `(base_name, base_labels)`
- Valid assembled histogram for one scrape requires:
  - exactly one count sample
  - exactly one sum sample
  - at least one bucket sample
  - unique bucket bounds after numeric parse
  - strictly increasing finite bucket bounds
  - if explicit `+Inf` is present, its cumulative count must equal count
  - if explicit `+Inf` is absent, count is the implicit `+Inf` bucket value
  - the last finite cumulative count must not exceed count
- Schema rules:
  - the first successful scrape/write for an exported histogram metric name establishes its bounds schema
  - later scrapes/writes for the same exported histogram metric name must match that schema
  - schema drift invalidates that histogram identity for the current scrape
  - schema lock scope is per exported metric name, not per exported metric name plus labels
- Failure rule:
  - malformed or conflicting histogram groups are dropped as a whole for the current scrape
  - they do not degrade to scalar output

### Summary families

- Summary assembly is driven by post-relabel sample roles:
  - `summary_quantile`
  - `summary_sum`
  - `summary_count`
- Structural validity rules:
  - quantile samples must retain exactly one `quantile` label
  - summary sum samples must retain a final metric name ending in `_sum`
  - summary count samples must retain a final metric name ending in `_count`
- Base identity derivation:
  - base name for quantile samples is the final sample name
  - base name for sum is `trim_suffix(final_name, "_sum")`
  - base name for count is `trim_suffix(final_name, "_count")`
  - base labels are final labels without `quantile`
  - group key is `(base_name, base_labels)`
- Valid assembled summary for one scrape requires:
  - exactly one count sample
  - exactly one sum sample
  - one or more quantile samples in the current supported scope
  - unique quantile values after numeric parse
- Schema rules:
  - the first successful scrape/write for an exported summary metric name establishes its quantile schema
  - later scrapes/writes for the same exported summary metric name must match that schema
  - schema drift invalidates that summary identity for the current scrape
  - schema lock scope is per exported metric name, not per exported metric name plus labels
- Failure rule:
  - malformed or conflicting summary groups are dropped as a whole for the current scrape
  - they do not degrade to scalar output
- Initial collector scope:
  - quantile-bearing summaries are supported
  - count/sum-only Prometheus summaries are intentionally ignored in the initial implementation

### Duplicate and overlap policy at assembly time

- Facts:
  - the initial collector design rejects overlapping relabel outputs and fails fast on config conflicts.
- Specification:
  - typed-family assembly sees one unified stream of post-relabel samples
  - duplicate final identities across different relabel scopes are handled by the same conflict rules as duplicates within one scope
  - the assembly layer must never silently accept collisions by overwrite

## Exported Metric Namespace Contract

### Facts

- `charttpl` validation requires explicit visible metric names.
- `metrix` flattening already defines how histogram and summary families appear to readers.

### Specification

- The namespace visible to `charttpl` is the flattened post-store namespace, not the raw sample-role namespace.
- Visible names are:
  - scalar: final scalar metric name
  - histogram: `<base>_bucket`, `<base>_count`, `<base>_sum`
  - summary: `<base>`, `<base>_count`, `<base>_sum`
- Consequence:
  - curated profile/template validation requires the collector to know the exported metric names visible to the template at init time
  - arbitrary dynamic name rewrites that cannot be enumerated at init are not compatible with strict curated template validation

## Performance Contract

### Hard requirements

- Single-pass parser.
- Reuse parser-owned memory aggressively.
- No required per-sample heap allocation in steady state.
- No full materialization of raw scrape input solely to support relabel.
- The parser may be internally ugly if that is what the hot path needs.

### Assembly and relabel implications

- The relabel engine should reuse builder scratch and temporary label buffers across samples.
- Numeric parsing scratch for `le` and `quantile` should be reused.
- Typed-family assembly should retain only:
  - surviving in-progress grouped state for the current scrape
  - persistent schema state needed across scrapes
- The design should avoid keeping a second full copy of the scrape in memory.

## Collector Integration Contract

- Specification:
  - collector flow is:
  1. scrape endpoint text
  2. iterate parser sample stream
  3. apply `metric_relabel_configs` to scraped labels plus synthetic `__name__`
  4. validate post-relabel output and drop malformed samples before assembly
  5. hand surviving samples to typed-family assembler
  6. emit typed metrics into `metrix`
  7. let chartengine operate only on flattened post-store series

- Boundary:
  - the initial implementation does not merge collector-owned target labels before relabel
  - the initial implementation allows `__name__` rewrites only when the resulting output remains structurally valid and, in curated mode, init-enumerable

## Collector-To-`metrix` Bridge Contract

- Shared pre-write validation:
  - exported metric name must be present and valid
  - final labels must canonicalize successfully
  - empty, duplicate, or otherwise invalid label keys are rejected before any `metrix` write call
  - metric values must be finite before any `metrix` write call
  - histogram/summary structural validity must already be established before any `ObservePoint(...)`
  - registration mismatch or schema-drift outcomes are treated as drop/observe events, not as fallback-to-another-kind behavior

- Curated mode:
  - one or more curated profiles may be merged into one generated job-level charttpl spec
  - profiles contribute nested `groups`; the collector owns the top-level spec
  - exported metric names and kinds are predeclared at init
  - curated templates validate against that init-known exported namespace
  - histogram/summary descriptors may be registered before schema is known
  - merge-time validation fails fast on:
    - duplicate selected profile IDs
    - duplicate chart IDs across merged curated groups
    - curated exported-metric collisions

- Generic/autogen mode:
  - instruments are registered on first successful assembled metric group
  - the init template is intentionally minimal:
    - autogen enabled
    - one placeholder root group
    - no curated metric-visibility contract

- Job-level engine policy:
  - `engine.autogen` is set by the collector per data-collection job when generating the top-level charttpl spec
  - profiles do not own top-level engine policy
  - curated groups may coexist with autogen-enabled unmatched fallback in the same job when that job-level option is enabled

- Scalar write path:
  - resolve final exported metric name and final labels
  - resolve final metric kind using family metadata and collector compatibility rules such as `fallback_type`
  - register or reuse the scalar instrument for that exported metric name
  - write one validated scalar point

- Histogram write path:
  - assemble one validated histogram group keyed by exported base metric name plus base labels
  - register or reuse the snapshot histogram instrument for the exported base metric name
  - the first successful histogram commit in `metrix` captures bounds for that exported metric name
  - call `ObservePoint(HistogramPoint{...})` only after structural validation has already passed

- Summary write path:
  - assemble one validated summary group keyed by exported base metric name plus base labels
  - register or reuse the snapshot summary instrument for the exported base metric name
  - register with quantiles derived from the first grouped summary observation before the first quantile-bearing write
  - curated mode may still predeclare the same quantile set when the profile already fixes it
  - call `ObservePoint(SummaryPoint{...})` only after structural validation has already passed
  - the initial Prometheus collector still drops count/sum-only summaries instead of writing them

## `18.B` Shared-Package Follow-Up

- `18.B` is no longer a prerequisite for the initial Prometheus v2 implementation.
- If revisited later as shared-package cleanup, the intended contract would be:
  - snapshot summary descriptors may exist without quantile schema
  - the first successful quantile-bearing snapshot summary write for an exported metric name captures the sorted quantile schema in `metrix`
  - later quantile-bearing writes for that exported metric name must match the locked schema
  - flattening and `Reader.Summary(...)` use the locked schema once captured
