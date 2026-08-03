<!-- markdownlint-disable-file MD043 -->

# Prometheus profile validation

This developer tool validates one candidate `go.d/prometheus.profiles` file
against one captured Prometheus exposition dump. It exercises the production
profile catalog, Prometheus collector, writer, metrix store, chart-template
compiler, chartengine planner, and public chart emitter instead of
reimplementing those contracts.

The tool runs with `go run`; it is not installed and is not a `go.d.plugin`
mode.

## Run

From `src/go`:

```text
go run ./tools/prometheus-profile-validation \
  --profile /path/to/exporter.yaml \
  --dump /path/to/metrics.txt \
  --job /path/to/job-policy.yaml \
  --output text
```

`--output` accepts `text` (default) or `json`. `--job` is optional for
exploration, but a deliverable profile needs the intended job policy; omitting
it produces a warning because collector defaults may not match deployment.

Exit codes:

- `0`: objective validation passed; warnings may still require review.
- `1`: validation completed and found an objective failure.
- `2`: command-line usage or report-output failure.

## Safe job policy

The job file accepts only settings that shape the captured dump:

```yaml
name: exporter
app: exporter
selector:
  allow: ['exporter_*']
  deny: [exporter_requests_created]
relabeling:
  - match: exporter_*
    metric_relabel_configs:
      - source_labels: [old_label]
        target_label: instance
        action: replace
fallback_type:
  gauge: ['exporter_*']
  counter: ['*_total']
expected_prefix: exporter_
max_time_series: 2000
max_time_series_per_metric: 200
```

The schema has no URL, authentication, TLS, or profile-selection fields. The
tool snapshots the dump, forces an isolated `file://` endpoint, empties ambient
profile catalogs, and selects only the supplied candidate. Unknown job keys and
multiple job documents fail.

## What `PASS` establishes

For the supplied dump and job policy, a pass establishes that:

- strict catalog/profile/template decoding succeeds;
- the real collector completes `Init`, `Check`, and a committed `Collect`;
- required chartengine runtime coverage counters exist and are valid;
- every current writer series is accounted for with zero generic fallback and
  zero unmatched series;
- a deterministic synthetic future family derived from wildcard profile
  `match` scope is admitted by both profile fallback and the structured job
  selector/relabel policy;
- any non-empty job allow list structurally covers each positive wildcard term
  in the profile scope, and wildcard relabel discard has a bounded,
  source-fixture-evidenced metric-name grammar without prior label-derived
  metric-name mutation; wildcard metric-name rewrites are derived only from the
  original name and use a bounded, source-fixture-evidenced input grammar;
- the profile has no fallback allowlist or open-ended fallback deny, and the
  job has no sample-filtering `keep` or `keepequal` relabel action; exact
  profile/job/relabel exclusions have the required source-complete evidence; a
  selector containing `{...}` is label-constrained policy rather than an exact
  metric name;
- the complete recommended relabel pipeline preserves every observed
  writer-admissible logical identity after normal histogram/summary component
  assembly; distinct source identities do not converge on the same final
  metric name and labels;
- every source-fixture sample reached by a metric-name rewrite retains a valid
  non-empty name rather than being implicitly discarded;
- every authored chart and every authored dimension materializes;
- every selected writer series carries every label required by its chart's
  effective explicit instance identity;
- observed cross-template rendered-ID collisions are absent;
- observed same-template instance identities do not collapse into fewer chart
  IDs;
- observed per-instance dimension identities are not discarded by lifecycle
  caps or planner normalization;
- planner-distinct chart IDs remain non-empty and unique after public wire
  normalization;
- distinct effective contexts do not collapse into one public wire context,
  and effective contexts do not normalize to empty;
- observed dynamic dimension names do not collapse into duplicate emitted wire
  IDs or sanitize to an omitted empty ID;
- every chart declares an explicit positive priority and priorities do not
  decrease in source order;
- every chart has at least one visible dimension;
- histogram bucket charts use observation-rate units and incremental semantics;
  and
- charts with unambiguously non-volume units do not use filled presentation.

`instance_identity_label_unavailable` reports the direct cause when a chart's
effective inherited/explicit `instances.by_labels` requires keys absent from
writer series selected by one of its dimensions. It reports only the chart,
selector, missing label keys, and affected count—not observed label values.
Changing the entity boundary or overriding identity can be correct; inventing a
label that the series does not carry is not.

`pipeline_excluded` reports raw logical source series that are wholly or partly
absent after job policy and writer processing. Categories are deliberately
generic when selector or relabeling rules make the precise cause ambiguous.
Raw logical series and flattened writer series are different counts for
histograms and summaries.

The strict current-source requirement is zero autogen and zero unmatched
series. One explicit profile policy can pass with a warning:

- `profile_suppressed_series` means a counterfactual planner run removed the
  candidate profile's exact fallback rule, converted every formerly unmatched
  series into generic fallback, and left zero unmatched series. The profile
  must have an exact deny list. This proves the reported gap is an intentional
  deny policy rather than a coincidental count; it does not prove that
  suppressing the operator question is a good design.

Forward compatibility uses structural policy checks plus synthetic names that
are independent from the dump. Wildcard name-discard and name-rewrite
exceptions additionally require the source-complete dump to exercise every
bounded grammar branch.
The report's `profile.future_metric_canary` records the primary synthesized
name. These objective errors reject closed contribution policy even when the
current fixture has no coverage gap:

- `closed_profile_fallback`: non-empty `autogen.selector.allow`;
- `open_ended_profile_fallback_deny`: wildcard, label-only, or otherwise
  non-exact profile deny;
- `unproven_profile_fallback_deny`: an exact profile deny names a family absent
  from the source-complete family inventory;
- `open_ended_job_selector_deny`: wildcard, label-only, or otherwise non-exact
  recommended-job deny;
- `unproven_job_selector_deny`: an exact recommended-job deny names an
  exposition metric absent from the source-complete sample inventory;
- `closed_job_selector_allow`: a non-empty job allow list does not contain each
  positive wildcard profile term unchanged in an unconstrained expression, the
  complete profile match expression, or `*`;
- `future_metric_canary_unavailable`: wildcard profile scope cannot produce a
  valid deterministic Prometheus-family probe;
- `future_relabel_canary_unavailable`: one positive wildcard term in an
  overlapping relabel block can discard or rename metrics, but its exclusions
  hide every deterministic in-scope probe or ordered relabeling prevents the
  block from processing those probes;
- `future_metric_blocked_by_profile`: the canary cannot reach profile fallback;
- `future_metric_blocked_by_job_selector`: the recommended job rejects it;
- `future_metric_blocked_by_job_relabel`: an otherwise applicable relabel rule
  drops it;
- `future_metric_routed_to_authored_metric`: relabeling maps a primary future
  canary onto a metric selected by an authored dimension instead of leaving it
  generic;
- `future_metric_identity_collapse`: relabeling maps distinct primary future
  canaries onto the same generic metric name, which can merge unrelated
  families or types;
- `observed_relabel_identity_collapse`: after normal histogram/summary
  component assembly, the complete recommended relabel pipeline maps multiple
  observed writer-admissible source identities onto the same final metric name
  and labels. Relabeling retains one value for that final identity; it does not
  aggregate the collapsed sources. Writer-rejected samples such as non-finite
  scalars do not participate;
- `invalid_relabel_metric_name_discard`: a source-fixture sample is implicitly
  dropped because recommended relabeling produces an empty or invalid metric
  name. Intentional exclusions must use explicit bounded, source-evidenced
  `drop` rules;
- `closed_relabel_filter`: a relabel rule uses inverse `keep` or `keepequal`;
- `unbounded_relabel_discard`: a wildcard relabel block uses application labels
  to discard samples instead of binding the exclusion to `__name__`; label-based
  discard requires an exact known metric block;
- `open_ended_relabel_name_discard`: a wildcard `__name__` discard is not a
  finite exact-name set or one non-empty internal entity key between finite
  exporter prefixes and finite terminal metric suffixes. Wildcard `dropequal`
  is also rejected because it cannot regex-bound the discarded names;
- `unproven_relabel_name_discard`: at least one finite exact name or dynamic
  alias prefix/suffix branch has no dropped sample in the source-complete
  fixture;
- `unproven_exact_relabel_scope`: an exact positive relabel-block metric name
  is absent from the source-complete exposition;
- `unproven_exact_relabel_discard`: an exact-scope `drop` or `dropequal` rule
  drops no fixture sample at its ordered pipeline position;
- `tainted_relabel_name_discard`: an earlier ordered relabel rule may derive
  `__name__` from application labels before a later sample-discard rule reads
  it. Name provenance is tracked across relabel blocks;
- `unbounded_metric_name_rewrite`: a wildcard relabel block may write
  `__name__` from application labels, including through a `labelmap` replacement
  that can create `__name__`. The rewrite itself can rename or invalidate an
  unknown future family. Netdata's relabel runtime ignores configured
  `labelmap.source_labels`, so naming only `__name__` there does not establish
  name-only provenance. An exact block is bounded only while the name is still
  the original metric name; earlier renaming can route future families into
  that block. Provenance follows reachable output scopes using exact
  glob-language intersection, so a disjoint exact or character-class rename
  does not poison a later block. Capture-bearing replacements retain both
  literal prefixes and suffixes in that reachability proof. A name-only rewrite
  inherits any application-label provenance in its input `__name__`. Dynamic destination
  capability is evaluated from the rule regex and replacement together, so a
  finite output set that cannot produce `__name__` remains valid label-only
  policy;
- `open_ended_relabel_name_rewrite`: a wildcard name-derived replace does not
  enumerate finite exact inputs, one internal dynamic entity key with finite
  terminal suffixes, or the rewrite-only canonical dynamic-tail form
  `<canonical_name>_<non-empty-identity> -> <canonical_name>`. An internal-key
  rewrite also fails when its output references the dynamic capture, a finite
  capture nested inside that identity, or any finite output is not an authored
  canonical metric;
- `unpreserved_relabel_name_identity`: an internal-key or canonical-tail
  rewrite removes the dynamic identity from `__name__` without an earlier rule
  in the same block copying unchanged the capture that encloses the entire
  dynamic entity region into a reserved stable non-name label. A nested capture
  covering only one alternative is incomplete. The target must be absent from
  fixture inputs at block entry and survive every reachable later rule/block.
  The canonical output must keep every finite prefix/suffix branch distinct. A
  later label write sourced only from `__name__` is ignored when its regex is
  disjoint from every possible current canonical name. Distinct raw entities
  could otherwise collapse into one writer series;
- `unproven_relabel_name_rewrite`: at least one finite name, prefix/suffix
  branch, or canonical dynamic-tail prefix does not reach the rewrite in the
  source-complete fixture.

`authored_mapping` reports the effective profile in source order:

- composed displayed family;
- title, context, units, authored algorithm intent, type, and priority;
- effective `instances.by_labels` after inheritance; and
- exact dimension selectors, static/dynamic naming mechanisms, and visibility.

This mapping is deterministic evidence for semantic review. It lets the author
reconcile the emitted selector-to-display design against a source-backed
operator model without claiming that the validator understands the
application's causal structure. Materialized chart records separately report
the compiler-resolved runtime algorithm.

Rendered chart IDs and dimension names can contain label values, so reports
fingerprint them instead of emitting the raw values. Public-emitter failures
are classified and fingerprinted rather than copied verbatim for the same
reason.

## Warnings preserve design judgment

Warnings identify designs that deserve explanation but can be correct:

- exact explicitly suppressed fallback series, whose source boundary and
  operator trade-off require review;
- profile `match` expressions that also accept common `go_*`, `http_*`,
  `process_*`, or `python_*` families and can therefore auto-select on an
  unrelated endpoint;
- duplicate priorities, whose runtime placement falls back to chart-ID order;
- open-ended `instances.by_labels: ['*']` identity;
- repeated non-empty sibling family names, which compose the same displayed
  navigation path instead of distinct semantic branches;
- displayed leaf families whose charts use different effective entity
  identities;
- child group/chart identities that drop labels from an explicitly declared
  parent `chart_defaults.instances.by_labels` identity;
- sibling family subtrees with no common explicit identity label;
- observed label keys that selected chart series carry but the chart neither
  uses nor explicitly excludes for identity, dimensions, promoted metadata, or
  selector routing;
- one union summary of the observed writer-capable evidence removed by the
  effective job selector before profile coverage is measured;
- each allow list's observed exclusion of otherwise writer-capable families;
- each deny expression's observed impact on otherwise writer-capable families;
- every sample-discarding relabel rule, including its observed
  logical-identity impact or the absence of matching evidence; `keep` and
  `keepequal` additionally fail contributor validation because they are closed
  inverse filters;
- metric declarations unused by any authored dimension in their scope;
- histogram bucket charts whose authored type differs from the compiler-forced
  heatmap;
- `area`/`stacked` charts with physical-rate or ambiguous units, where volume
  semantics may or may not justify a filled visual;
- charts that mix distribution shape/count/sum roles under one unit;
- compiler-resolved incremental charts whose rendered units omit the
  per-second denominator and are not a recognized derived equivalent such as
  cores, concurrency, or utilization;
- absolute chart dimensions whose observed non-zero magnitudes differ by at
  least 20x and may flatten the smaller signal.

The validator does not turn these heuristics into policy. For example, sibling
families can intentionally represent different entity levels, a leaf can mark
an intentional filter boundary, and tied priorities can be deliberate.
Identity findings report effective label keys only; they cannot determine the
application's entity types or invent a missing label. A label can be
intentionally aggregated, but its lost comparison/filtering and expected
cardinality still need explanation. A bandwidth rate can justify area while an
ordinary request rate usually does not.

The exclusion summary prevents a mechanically clean report from hiding a
shrinking denominator among many per-rule warnings. It remains advisory because
the tool cannot prove why the user chose a collection boundary. Dashboard focus
alone is not a collection boundary: use hierarchy and priority rather than
discarding distinct writer-capable diagnostics.

Relabel discard warnings replay the validated ordered blocks over the captured
samples and attribute observed drops to the exact rule. A rule with no observed
drop still warns because one dump cannot prove its future exclusion surface.
Under an overlapping wildcard stock-policy scope, an otherwise permitted
`__name__` drop or rewrite also fails unless every bounded grammar branch has
evidence. The warning never reports observed label values.

Incremental-unit warnings use chartengine's compiler-resolved algorithm rather
than duplicating suffix inference. Compound observed units such as
`duration/item/s` retain rate semantics and are not rejected merely for having
more than one denominator.

Hidden dimensions are excluded from the visible-scale comparison because they
do not control the chart's default visible axis. A chart may hide supporting
dimensions, but hiding every dimension is an objective failure because the
chart has no visible answer. The discoverability of each remaining hidden
dimension is still a semantic review question.

## Evidence boundary

This is an objective correctness gate, not a dashboard designer:

- One dump cannot validate metrics, optional features, labels, or label values
  absent from it. Use a comprehensive representative dump. The synthetic
  future-family canary proves only that policy remains open to a new matching
  name; it does not invent or validate that future metric's semantics.
- Observed ID collisions are checked; future unseen values can still normalize
  to the same ID.
- A lifecycle cap that accommodates this dump may still omit entities or
  dimensions in a larger configuration.
- The tool forces exact candidate selection. It does not prove that the
  profile's `match` expression uniquely auto-selects this exporter instead of
  an unrelated endpoint. The generic-match warning catches common unsafe
  classes, not every possible false-positive signature.
- Writer-rejected source series are reported but are not profile coverage gaps,
  because chartengine never receives them.
- A `PASS` enforces only the units and presentation cases listed above. It does
  not establish that the remaining hierarchy, chart composition, titles, units,
  instance choices, or presentation order are useful to an operator.

Each process validates one candidate. Production catalog and plugin
configuration are process-global, so end-to-end test scenarios run in separate
subprocesses.
