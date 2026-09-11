# Dashboard-shaping mechanisms

Reference for `collectors-authoring` §3. Each mechanism feeds the NIDL model described in the skill. Open
the subsection that matches the ingestion path you are working on.

## SNMP profiles — declarative spec → NIDL

SNMP collection is profile-driven. A profile is a YAML document declaring OIDs, metric definitions, table indexing,
units, chart families, and selectors. Stock profiles ship from `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`;
spec at `src/go/plugin/go.d/collector/snmp/profile-format.md` (~2000 lines).

Adding or extending SNMP coverage means writing or extending a profile, not adding code. The SNMP topology collector
(`snmp_topology`) builds on top of profiles — extending profiles is usually the right starting point for topology work
too.

Past pain: pre-profile SNMP code required per-vendor branches that became unmaintainable. Don't hardcode OID-to-metric
mappings inside a custom collector or vendor branch.

## statsd `synthetic_charts` — operator-curated dashboards

The statsd plugin lets the operator group raw statsd metrics into curated charts via INI configs at
`/etc/netdata/statsd.d/*.conf`. Each config defines:

- `[app]` — match raw metrics by pattern, group them under an application name
- `[dictionary]` — rename raw metric names to display names
- chart sections — declare a chart with `title`, `family`, `context`, `units`, `type`, and explicit `dimension =` lines
  mapping source metrics to display dimensions

Wildcard patterns extract dimension names from the matched portion: `dimension = pattern 'myapp.api.*.200' '' last 1 1`
creates dimensions named after the wildcard match. Three-layer dimension lookup (dimension name in dictionary → metric
name in dictionary → fallback to original). Stock examples: `src/collectors/statsd.plugin/k6.conf`,
`src/collectors/statsd.plugin/asterisk.conf`. Full spec: `src/collectors/statsd.plugin/README.md` lines 397-639.

This is the most operator-controllable shaping mechanism — the dashboard is whatever the operator declares.

## OTEL mappings — per-metric YAML routing

Netdata's OTEL plugin (`src/crates/otel-plugin/`) accepts any OTLP gRPC metric. Mapping is **generic by default** — all
resource attributes, scope attributes, and data point attributes become chart labels — but the operator controls routing
via per-metric YAML files at `/etc/netdata/otel.d/v1/metrics/*.yaml`. Key knobs:

- `instrumentation_scope.name` / `version` — regex match to scope an entry to a specific OTel instrumentation
- `dimension_attribute_key` — which data point attribute becomes the dimension name (default: `"value"`); other
  attributes become chart labels
- `interval_secs`, `grace_period_secs` — per-metric timing overrides

Aggregation temporality drives the chart algorithm: Gauge → absolute, Sum delta → DeltaSum, Sum cumulative monotonic →
CumulativeSum, Sum cumulative non-monotonic → treated as Gauge (`src/crates/otel-ingestor/src/chart.rs`).

The plugin does **not** recognize OTel semantic conventions specifically (`host.name`, `service.name`,
`deployment.environment`) — they pass through as labels. Cardinality control is `metrics.max_new_charts_per_request` in
`otel.yaml`. Stock examples: `src/crates/otel-ingestor/configs/otel.d/v1/metrics/`.

## Prometheus — deterministic mapping; shape with relabeling and profiles

The generic Prometheus scraper (`src/go/plugin/go.d/collector/prometheus/`) auto-maps from the exposition format:

- metric name → chart ID + dimension ID
- Prometheus labels → Netdata chart labels
- type (`counter`, `gauge`, `histogram`, `summary`) → chart type and dimension algorithm
- histograms produce bucket, `_sum`, and `_count` charts; summaries always produce `_sum` and `_count`, plus a
  quantile chart when quantiles exist
- recognized suffixes: `_total` (counter), `_bucket` + `le` label (histogram), `_sum`, `_count`, `quantile` label
  (summary), `_info` (skipped)
- unit suffixes drive the units string: `_seconds`, `_bytes`, `_hertz`

Operator controls (profiles documented in `src/go/plugin/go.d/collector/prometheus/profile-format.md`, relabeling in
`src/go/plugin/go.d/collector/prometheus/relabel/README.md`):

- **Scoping**: the time-series `selector` job option (allow/deny on metric name and label values, syntax in
  `src/go/pkg/prometheus/selector/README.md`) and `fallback_type` glob patterns for untyped metrics.
- **Shaping**: job-level `relabeling` is operator policy and runs before profile selection. Profile-root `relabeling`
  uses the same Prometheus-compatible block/rule format after selection and owns stable exporter normalization required
  by that profile's charts. Do not duplicate profile-required normalization as an optional job recipe.
- **Ordering**: the fixed namespace lifecycle is `selector -> job relabeling/safety -> fallback type + profile
  selection -> selected profile relabeling/safety -> final gates -> charts`. A profile cannot normalize itself into
  selection. Untyped classification is bound before profile relabeling, so a final rename cannot create or change it.
- **Profile precedence**: selected profiles share one final metric stream. For each original source family, only the
  first applicable selected profile's complete pipeline runs; later profiles do not see that family or names produced by
  the first pipeline. Precedence is profile-name order in `auto`, configured entry order in `exact`, and configured
  entries followed by the remaining name-ordered auto profiles in `combined`. Root `match` and block matchers classify
  original source names; rule results and output names do not affect dispatch. All selected templates consume the final
  names and labels, so authors must account for cross-profile interactions.
- **Charts**: chart profiles (`match`/`app`/`relabeling`/`autogen.selector`/`template` YAMLs, stock under
  `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/`, user under
  `/etc/netdata/go.d/prometheus.profiles/`) ship curated per-exporter dashboards — the Prometheus analog of
  statsd `synthetic_charts`. Metrics not covered by an authored profile chart keep their autogen charts unless an
  applicable profile selector rejects them. Each selector is limited to its profile's `match` scope; when scopes
  overlap, every applicable selector must accept the series. This changes fallback charts only; use the job selector
  or a relabeling `drop` action to discard samples.

Use `.agents/skills/collectors-prometheus-profiles/SKILL.md` when creating or materially
reviewing a profile. It teaches the dashboard-design reasoning and the
real-pipeline validation boundary; schema validity alone is not semantic
dashboard approval.
