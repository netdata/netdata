<!-- markdownlint-disable-file MD043 -->

# Prometheus profile validation

This developer tool validates one candidate `go.d/prometheus.profiles` file,
with optional supporting profiles, against one captured Prometheus exposition
dump. It exercises the production
profile catalog, Prometheus collector, writer, metrix store, chart-template
compiler, chartengine planner, and public chart emitter instead of
reimplementing those contracts.

The tool runs with `go run`; it is not installed and is not a `go.d.plugin`
mode.

See the [canonical framework architecture](../../internal/promprofile/README.md) for the relationship between standalone
validation, stock semantic proofs, production diagnostics, and external testdata. This document owns the standalone CLI,
job policy, findings, and extension details.

## Run

From `src/go`:

```text
go run ./tools/prometheus-profile-validation \
  --profile /path/to/exporter.yaml \
  --support-profile /path/to/runtime.yaml \
  --dump /path/to/metrics.txt \
  --job /path/to/job-policy.yaml \
  --output text
```

`--support-profile` is optional and repeatable; it supplies profiles expected to compose with the candidate on the same
endpoint. `--output` accepts `text` (default) or `json`. `--job` is optional for
exploration, but a deliverable profile needs the intended job policy; omitting
it produces a warning because collector defaults may not match deployment.

Stock-profile fixtures and strict source-semantic contracts live in
[`netdata/testdata`](https://github.com/netdata/testdata). Clone its latest
`master` into the ignored `src/go/testdata` directory, or set
`NETDATA_TESTDATA_DIR` to another checkout. Each profile uses one stable external
directory; update latest testdata and the corresponding Netdata proof expectations
together.
For an existing default checkout, shallow-fetch `origin master` and detach at
the fetched tip before replay. Local feature branches remain unchanged.

```text
git -C src/go/testdata fetch --depth=1 origin master
git -C src/go/testdata switch --detach FETCH_HEAD
```

The dedicated replay checks every declared outcome and reconciles source semantics, normalization, routes, chart plans,
observations, wire identities, and aggregate semantic coverage. Required mode fails when
external evidence is unavailable:

```text
NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1 go test -count=1 \
  ./internal/promprofile/validation -run TestStockProfileProofsReplay
```

`go run ./tools/prometheus-profile-proof verify --repo-root ../..` verifies the complete stock-proof catalog, exact local
and external layouts, strict source/profile contracts, and executable replay expectations.

Exit codes:

- `0`: objective validation passed; warnings may still require review.
- `1`: validation completed and found an objective failure.
- `2`: command-line usage or report-output failure.

## Architecture and extension points

The executable is a flag/exit-code adapter over `promvalidation.Validate` in
`internal/promprofile/validation`. Contributor policy, deterministic findings,
and report rendering live in that internal library. Runtime semantics remain
with their production owners:

- `pkg/prometheus` parses/assembles evidence and defines physical/logical series
  identities plus strict duplicate detection;
- the Prometheus collector performs selector, relabel, writer, exact-profile,
  and store processing and emits opt-in structured pipeline facts;
- `pkg/matcher` and collector `relabel` own bounded, cancelable witness/name-flow
  mechanisms used by contributor policy;
- chartengine emits opt-in route facts from its one production plan; and
- chartemit exposes structured public-ID/context/dimension inspection from the
  same preparation path used by application.

Diagnostics are constructor options, not collector configuration. Disabled
production paths do not allocate diagnostic records. Enabled validation bypasses
the chart route cache so every series produces an attempt-local fact; observers
stream or aggregate within one run and retain no process-global state.

Add a new semantic fact at the production owner and expose a small structured
diagnostic/mechanism. Add contributor severity, evidence obligations, wording,
and report presentation in `internal/promprofile/validation`. Do not reconstruct
runtime acceptance, identity, naming, or wire behavior in the validator.

After isolated inputs and profiles are loaded, scraping, collector execution,
future-input replay, matcher/relabel analysis, and provenance finalization use a
30-second deadline bounded by the caller context. Input staging, profile decode,
authored-template traversal, and the chartengine plan call are synchronous and
are not covered by that internal timeout. Matcher/relabel analyzers also have
deterministic state/transition/value limits and fail closed when those limits or
context cancellation are reached.

## Safe job policy

The job file accepts only settings that shape the captured dump:

```yaml
name: exporter
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
future_inputs:
  - name: exporter_future_requests_total
    type: counter
    labels:
      instance: future-instance
```

The schema has no URL, authentication, TLS, or profile-selection fields. The
tool snapshots the dump, forces an isolated `file://` endpoint, empties ambient
profile catalogs, and selects exactly the supplied candidate/supporting set. Unknown job keys and multiple job documents
fail.

`app` remains an accepted deployment-shaped field for an intentional override or for disambiguating profiles that declare
different apps. Omit it when automatic profile selection already supplies the intended identity. Stock proof composition is
declared by `PROFILE-DESIGN.yaml` and resolved through production automatic selection; it is never copied into job policy.

`future_inputs` are validator-only raw samples. `name` is required; `type` is
optional (`gauge` by default, `counter`, or `untyped`); and `labels` is an
optional string map that cannot contain `__name__`. Names and label names use
the production Prometheus UTF-8 rules. A job may declare at most 256 unique raw
identities and one type per family. Use invented, non-sensitive labels. These
samples enter before job selector processing and both job and profile
relabeling, so do not declare post-relabel names.

The validator derives bounded raw witnesses when metric-name identity is
preserved. If a reachable job or profile relabel rule can write metric names,
explicit `future_inputs` are required because arbitrary relabeling cannot be
inverted soundly.

## What `PASS` establishes

For the supplied dump and job policy, a pass establishes that:

- strict catalog/profile/template decoding succeeds;
- the real collector completes `Init`, `Check`, and a committed `Collect`;
- one production chart plan emits complete structured route diagnostics for
  every scanned writer series;
- every current writer series is accounted for with zero generic fallback and
  zero unmatched series;
- a second isolated real collector sequence introduces declared or derived raw
  future probes before the job selector and job/profile relabeling; future
  results cannot satisfy current coverage;
- every positive wildcard term and future-relevant relabel scope/rule in the candidate and each supplied support profile
  has a probe that actually traverses the production selector, relabeler,
  assembler, writer, exact profile selection, and planner;
- future probes neither overwrite current writer identities nor collapse with
  each other, and reach generic fallback unless a bounded contributor-approved
  alias normalization legitimately maps them to an authored family;
- any non-empty job allow list structurally covers each positive wildcard term
  in the profile scope, and wildcard relabel discard has a bounded,
  source-fixture-evidenced metric-name grammar without prior label-derived
  metric-name mutation; wildcard metric-name rewrites are derived only from the
  original name and use a bounded, source-fixture-evidenced input grammar;
- every supplied profile has no fallback allowlist or open-ended fallback deny, and neither
  job nor profile relabeling has a sample-filtering `keep` or `keepequal`
  action; exact profile/job/relabel exclusions have the required
  source-complete evidence; a selector containing `{...}` is label-constrained
  policy rather than an exact metric name;
- the complete job-then-profile relabel pipeline preserves every observed
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
- explicit chart priorities, when present, are preserved and verified; charts without priority use the common runtime default;
- every chart has at least one visible dimension;
- histogram bucket charts use observation-rate units and incremental semantics.

Explicit `area` and `stacked` are advisory rather than objective failures.
Units cannot prove the relationship: `area` needs deliberate filled-magnitude
intent, while `stacked` needs an exact disjoint, exhaustive, additive partition
of one whole. The strict stock proof records and verifies that semantic intent.

`instance_identity_label_unavailable` reports the direct cause when a chart's
effective inherited/explicit `instances.by_labels` requires keys absent from
writer series selected by one of its dimensions. It reports only the chart,
selector, missing label keys, and affected count—not observed label values.
Changing the entity boundary or overriding identity can be correct; inventing a
label that the series does not carry is not.

`pipeline_excluded` records raw source families that are wholly or partly absent
after job/profile policy and writer processing. Each record carries the raw and
materialized logical-series counts plus every matched mutation/drop rule in
`policy_paths`; `counts.pipeline_excluded` is the number of
family records, not the sum of lost logical identities. Relabel-renamed families
are reconciled by logical identity against their final writer names, so
successful normalization is not reported as loss. Categories are deliberately
generic when selector or relabeling rules make the precise cause ambiguous. Raw
logical series and flattened writer series are different counts for histograms
and summaries.

`pipeline_renamed` reports successful raw-to-final family-name lineage separately,
including the raw logical-series count and how many renamed identities reached
the writer. Its `policy_paths` identify the exact job/profile rules that changed
the source identities. A family can appear in both sections when only part of its source
identity set survives normalization.

`profiles.candidate`, `profiles.supports`, and `profiles.selected` distinguish the
authored input roles from the profiles production selection actually activated.
When validation composes profiles, profile-owned chart and relabel paths use
`profiles[<name>].template...` and `profiles[<name>].relabeling...`; candidate-only
validation retains its existing concise paths.

The strict current-source requirement is zero autogen and zero unmatched
series. One explicit profile policy can pass with a warning:

- `profile_suppressed_series` means production route facts show every unmatched
  series was suppressed by selected profiles' explicit fallback selectors.
  Each contributing profile must have an exact deny list. Owner-qualified
  finding paths identify the candidate and/or supporting selectors involved.
  This proves the reported gap is an intentional deny policy; it does not prove
  that suppressing the operator question is a good design.

Forward compatibility combines bounded structural analysis with the separate
raw future-input run. Wildcard name-discard and name-rewrite exceptions also
require the current source-complete dump to exercise every bounded grammar
branch. The `profiles.candidate.future_raw_probe` and
`profiles.supports[].future_raw_probe` fields record each profile's first open raw probe that actually covers one of its
wildcard scopes or relabel rules. Declared input order does not assign ownership. Shared job policy and all profile-local
requirements are exercised by one composed future collector/planner run. Current physical sample names and logical
typed-family base names are excluded from accepted and derived probes. These objective errors reject closed contribution
policy even when the current fixture has no coverage gap:

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
- `future_metric_witness_unavailable`: wildcard profile scope cannot produce a
  valid deterministic Prometheus-family probe;
- `future_relabel_witness_unavailable`: one positive wildcard term in an
  overlapping relabel block can discard or rename metrics, but its exclusions
  hide every deterministic in-scope probe or ordered relabeling prevents the
  block from processing those probes;
- `future_inputs_required`: reachable relabeling can change metric names but no
  explicit raw probes were declared;
- `future_input_not_future`: a declared probe name already occurs in current
  evidence;
- `future_profile_term_uncovered`, `future_relabel_scope_uncovered`, and
  `future_relabel_branch_uncovered`: the actual future run did not cover one
  required positive wildcard term, relabel scope, or rename/drop rule;
- `future_metric_blocked_by_profile`: a future writer series is unmatched by
  chart routing;
- `future_metric_blocked_by_job_selector`: the recommended job rejects it;
- `future_metric_blocked_by_job_relabel`: an otherwise applicable relabel rule
  drops it;
- `future_metric_blocked_by_profile_relabel`: a selected candidate or supporting profile's relabeling drops it;
- `future_metric_rejected_by_writer`: the production writer rejects its final
  family or series identity;
- `future_metric_routed_to_authored_metric`: relabeling maps a primary future
  probe onto a metric selected by an authored dimension without a recognized
  bounded alias contract;
- `future_metric_identity_collapse`: relabeling maps distinct primary future
  probes onto the same writer identity, or a future probe overwrites a current
  identity, which can merge unrelated families or types;
- `future_run_changed_current_evidence`: adding raw future probes changes or
  removes a current writer identity/value;
- `observed_relabel_identity_collapse`: after normal histogram/summary
  component assembly, the complete job-then-profile relabel pipeline maps multiple
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

`authored_mapping` reports charts from the selected candidate/support
composition in source order:

- the source profile-local chart path for composed validation;
- composed displayed family;
- title, context, units, authored algorithm intent, and type;
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
- every sample-discarding job or profile relabel rule, including its observed
  logical-identity impact or the absence of matching evidence; `keep` and
  `keepequal` additionally fail contributor validation because they are closed
  inverse filters;
- metric declarations unused by any authored dimension in their scope;
- histogram bucket charts whose authored type differs from the compiler-forced
  heatmap;
- every explicit `area` chart, so deliberate filled-magnitude intent is reviewed;
- every explicit `stacked` chart, so the exact disjoint/exhaustive/additive
  relationship is reviewed;
- charts that mix distribution shape/count/sum roles under one unit;
- compiler-resolved incremental charts whose rendered units omit the
  per-second denominator and are not a recognized derived equivalent such as
  cores, concurrency, or utilization;
- absolute chart dimensions whose observed non-zero magnitudes differ by at
  least 20x and may flatten the smaller signal.

The validator does not turn these heuristics into policy. For example, sibling
families can intentionally represent different entity levels, and a leaf can
mark an intentional filter boundary.
Identity findings report effective label keys only; they cannot determine the
application's entity types or invent a missing label. A label can be
intentionally aggregated, but its lost comparison/filtering and expected
cardinality still need explanation. Presentation likewise depends on the
operator question and source relationships, not a unit-word allow/deny list.

The exclusion summary prevents a mechanically clean report from hiding a
shrinking denominator among many per-rule warnings. It remains advisory because
the tool cannot prove why the user chose a collection boundary. Dashboard focus
alone is not a collection boundary: use hierarchy and coherent chart composition rather than
discarding distinct writer-capable diagnostics.

Relabel discard warnings consume facts emitted while the production processor
runs the validated ordered blocks and attribute observed drops to the exact
rule. A rule with no observed drop still warns because one dump cannot prove its
future exclusion surface.
Under an overlapping wildcard stock-policy scope, an otherwise permitted
`__name__` drop or rewrite also fails unless every bounded grammar branch has
evidence. The warning never reports observed label values.

Incremental-unit warnings use chartengine's runtime-kind-resolved algorithm
rather than inferring an algorithm from metric-name suffixes. Compound observed units such as
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
  absent from it. Use a comprehensive representative dump. Raw future probes
  prove only that declared/derived identities traverse current policy; they do
  not invent or validate a future metric's semantics, cardinality, or real
  labels.
- Observed ID collisions are checked; future unseen values can still normalize
  to the same ID.
- A lifecycle cap that accommodates this dump may still omit entities or
  dimensions in a larger configuration.
- The tool forces exact selection of the supplied candidate/supporting set. It does not prove that their `match`
  expressions uniquely auto-select this exporter instead of an unrelated endpoint. The generic-match warning catches
  common unsafe classes, not every possible false-positive signature.
- Writer-rejected source series are reported but are not profile coverage gaps,
  because chartengine never receives them.
- A `PASS` enforces only the units and presentation cases listed above. It does
  not establish that the remaining hierarchy, chart composition, titles, units,
  instance choices, or presentation order are useful to an operator.

Each validation call constructs isolated current and future collectors with an
explicit candidate catalog. It does not mutate production catalog or plugin
configuration globals, so library tests run in-process.
