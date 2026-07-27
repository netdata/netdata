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
  deny: ['*_created']
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
- every writer series is curated: zero autogen and zero unmatched series;
- every authored chart and every authored dimension materializes;
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
- every chart declares an explicit positive priority.

`pipeline_excluded` reports raw logical source series that are wholly or partly
absent after job policy and writer processing. Categories are deliberately
generic when selector or relabeling rules make the precise cause ambiguous.
Raw logical series and flattened writer series are different counts for
histograms and summaries.

Rendered chart IDs and dimension names can contain label values, so reports
fingerprint them instead of emitting the raw values. Public-emitter failures
are classified and fingerprinted rather than copied verbatim for the same
reason.

## Warnings preserve design judgment

Warnings identify designs that deserve explanation but can be correct:

- duplicate priorities or priority order that diverges from source order;
- open-ended `instances.by_labels: ['*']` identity;
- sibling family subtrees with no common explicit identity label;
- each allow list's observed exclusion of otherwise writer-capable families;
- each deny expression's observed impact on otherwise writer-capable families;
- metric declarations unused by any authored dimension in their scope;
- histogram bucket charts whose authored type differs from the compiler-forced
  heatmap;
- `area`/`stacked` charts with rate-like units, where volume semantics may or
  may not justify a filled visual;
- charts that mix distribution shape/count/sum roles under one unit;
- absolute chart dimensions whose observed non-zero magnitudes differ by at
  least 20x and may flatten the smaller signal;
- filled/stacked charts with non-volume units, including non-rate gauges.

The validator does not turn these heuristics into policy. For example, sibling
families can intentionally represent different entity levels, and tied
priorities can be deliberate. A bandwidth rate can justify area while an
ordinary request rate usually does not.

Hidden dimensions are excluded from the visible-scale comparison because they
do not control the chart's default visible axis. Their discoverability and the
choice to hide them remain semantic review questions.

## Evidence boundary

This is an objective correctness gate, not a dashboard designer:

- One dump cannot validate metrics, optional features, labels, or label values
  absent from it. Use a comprehensive representative dump.
- Observed ID collisions are checked; future unseen values can still normalize
  to the same ID.
- A lifecycle cap that accommodates this dump may still omit entities or
  dimensions in a larger configuration.
- The tool forces exact candidate selection. It does not prove that the
  profile's `match` expression uniquely auto-selects this exporter instead of
  an unrelated endpoint.
- Writer-rejected source series are reported but are not profile coverage gaps,
  because chartengine never receives them.
- A `PASS` does not establish that hierarchy, chart composition, titles, units,
  instance choices, or presentation order are useful to an operator.

Each process validates one candidate. Production catalog and plugin
configuration are process-global, so end-to-end test scenarios run in separate
subprocesses.
