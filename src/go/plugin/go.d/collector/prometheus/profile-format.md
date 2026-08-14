# Prometheus profile format

Prometheus profiles ship curated charts for recognized exporters. Without a profile, the generic Prometheus collector
renders one chart per scraped metric (autogeneration). A profile replaces that flat list, for the metrics it knows, with
a designed dashboard menu: named sections, per-instance charts, meaningful dimensions, units, and heatmaps. You are not
limited to the stock library -- you can author a profile for your own application's metrics too.

This page documents the profile file format. Profiles may also classify an exporter's untyped scalar metrics and
provide [metric relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md) that normalizes its metrics
after the profile is selected and before its chart template sees them. Jobs retain higher-precedence type and
relabeling policy for operator-specific overrides before profile processing.

Profiles reshape metrics an existing job already scrapes --
[set up the Prometheus collector job](/src/go/plugin/go.d/collector/prometheus/integrations/prometheus_endpoint.md)
first.

| Location                                                    | Purpose                             |
|:------------------------------------------------------------|:------------------------------------|
| `/usr/lib/netdata/conf.d/go.d/prometheus.profiles/default/` | Stock profiles shipped with Netdata |
| `/etc/netdata/go.d/prometheus.profiles/`                    | User profiles                       |

To customize a stock profile, copy it to the user directory and keep the same filename -- a user profile fully replaces
the stock profile with that basename. Restart the Netdata Agent (which restarts the go.d plugin) after changing profiles
because the catalog is cached for the plugin's lifetime.

> **Profiles are strictly validated.** A misspelled or unknown field is an error, not silently ignored. A broken user
> profile is skipped with a warning in the log (the stock profile of the same name, if any, stays in effect), and a
> descriptive validation error is reported -- unknown or misspelled keys are named explicitly.

## Complete example

A profile is a YAML file with two required top-level keys (`match` and `template`) plus optional `app`,
`fallback_type`, `relabeling`, and `autogen` policy. `match` selects the profile by scraped metric names, `app` names
the application the charts belong to, `fallback_type` classifies untyped scalar metrics owned by the exporter,
`relabeling` normalizes matching metrics after selection, `autogen.selector` controls fallback charts within the
profile's match scope, and `template` defines the curated charts. Everything below `template` is Netdata's
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md); inline comments explain each field.

```yaml
# example.yaml -- the basename (before .yaml/.yml) is the profile name
match: 'example_*'          # REQUIRED. Netdata simple pattern over scraped metric
                            # family names; one hit selects the profile for the job.
app: example                # optional. Application identity: charts appear under
                            # the prometheus.<app>.* contexts and the app's own
                            # Applications section in the UI, unless the job
                            # config sets its own `app`.

fallback_type:              # optional. Exporter-owned types for untyped scalar
  gauge:                    # metrics, matched on the pre-profile metric name.
    - example_open_connections
    - example_memory_bytes

relabeling:                 # optional. Normalize this exporter's metrics after
                            # profile selection and before template routing.
  - match: example_legacy_http_requests_total
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: example_legacy_http_requests_total
        target_label: __name__
        replacement: example_http_requests_total

autogen:                    # optional. Controls fallback charts inside this
  selector:                 # profile's match scope after authored routing fails.
    deny:
      - example_http_request_duration_seconds

template:                   # REQUIRED. One chart-template group; at least one chart.
  family: Example           # top-level dashboard menu section
  context_namespace: example  # context prefix; by convention the profile name
  groups:
    - family: Requests      # nested menu section: Example -> Requests
      metrics:              # metrics visible to this group's dimension selectors
        - example_http_requests_total
        - example_http_request_duration_seconds_bucket
      charts:
        - title: HTTP Requests
          context: http_requests        # emitted as prometheus.example.http_requests
                                        # (app == context_namespace, so the duplicate
                                        # segment is dropped -- see "How profiles work")
          units: requests/s
          algorithm: incremental        # counters are charted as rates
          aggregation: sum              # omitted labels are additive for this chart
          instances:
            by_labels: [listener]       # one chart instance per "listener" label value
          dimensions:
            - selector: example_http_requests_total
              name_from_label: method   # one dimension per "method" label value
        - title: HTTP Request Duration
          context: http_request_duration
          units: observations/s         # bucket dimensions count observations
          type: heatmap                 # optional: bucket charts are forced to heatmap
          algorithm: incremental
          instances:
            by_labels: [listener]
          dimensions:
            - selector: example_http_request_duration_seconds_bucket
              # no name -> bucket dimensions are derived from the "le" label

    - family: Resources
      metrics:
        - example_open_connections
        - example_memory_bytes
      charts:
        - title: Open Connections
          context: open_connections
          units: connections
          algorithm: absolute           # gauges are charted as-is
          dimensions:
            - selector: example_open_connections
              name: open
        - title: Memory Used
          context: memory_bytes
          units: MiB
          algorithm: absolute
          dimensions:
            - selector: example_memory_bytes
              name: used
              options:
                divisor: 1048576        # bytes -> MiB
                float: true
```

A minimal profile needs only `match` and a one-chart `template`:

```yaml
# myapp.yaml
match: 'myapp_*'
template:
  family: MyApp
  context_namespace: myapp
  groups:
    - family: Requests
      metrics: [myapp_requests_total]
      charts:
        - title: Requests
          context: requests
          units: requests/s
          algorithm: incremental
          dimensions:
            - selector: myapp_requests_total
              name: requests
```

## How profiles work

At runtime the collector uses profiles in seven ordered steps:

1. **Scrape** -- the job scrapes the endpoint. The `selector` job option filters unwanted series
   ([selector syntax](/src/go/pkg/prometheus/selector/README.md)).
2. **Job normalization** -- the job's
   [relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md) rewrites names and labels. Histogram and
   summary integrity is checked before continuing.
3. **Selection** -- once, at job autodetection, each profile's `match` is tested against the post-job metric family
   names, per the job's `profiles.mode` (see
   [Selecting profiles in job configuration](#selecting-profiles-in-job-configuration)). The selection is cached until
   the job restarts. Profile-owned relabeling cannot select its own profile because it runs after this step.
4. **Type classification** -- declared Prometheus types are authoritative. For an untyped scalar, the job's
   `fallback_type` is tried first, followed by selected profiles in normalization order, then the implicit `_total`
   counter rule. Classification uses the post-job, pre-profile name and is repeated on every scrape, so matching
   families that appear after autodetection are handled too.
5. **Profile normalization** -- each source family is processed by the first applicable selected profile's `relabeling`.
   Later profiles do not process that family. The resulting typed-family integrity is checked again.
6. **App resolution** -- the job's `app` option wins; when unset, the first selected profile that declares an `app`
   provides it; the job name is the last resort. If selected profiles declare different apps, the first (in selection
   order) wins and the rest are logged -- set the job's `app` to disambiguate.
7. **Charts** -- the selected profiles' templates are merged on top of the autogeneration base. Every series first gets
   a chance to route to every authored template dimension. If no route matches, each selected profile's
   `autogen.selector` is evaluated only when that profile's `match` applies to the final post-profile family. Every
   applicable selector must accept the series; one rejection suppresses fallback, independent of profile order. If no
   selector applies, the series keeps its generic autogen chart. A selector never removes samples from the metric store.

Chart contexts compose as `prometheus.<app>.<template context_namespace>.<chart context>` -- in the example above,
`prometheus.example.example.http_requests`. When the resolved app equals the profile's `context_namespace` (the common
case: the job has no `app` set and the profile provides it), the redundant segment is dropped, so the emitted context is
`prometheus.example.http_requests`. Autogen charts for uncovered metrics keep their `prometheus.<app>.<metric>`
contexts.

## Top-level fields

| Field           | Required | Description                                                                                                                                           |
|:----------------|:--------:|:------------------------------------------------------------------------------------------------------------------------------------------------------|
| `match`         |   yes    | [Netdata simple pattern](/src/libnetdata/simple_pattern/README.md) tested against post-job metric family names. One hit makes the profile applicable. |
| `app`           |    no    | Application identity used as the `app` segment of chart contexts when the job does not set one. Must match `^[a-z][a-z0-9_]*$`.                      |
| `fallback_type` |    no    | Exporter-owned gauge/counter classification for untyped scalar metrics inside this profile's `match` scope.                                           |
| `relabeling`    |    no    | Profile-owned metric normalization, applied automatically after selection and before charts.                                                         |
| `autogen`       |    no    | Fallback-chart policy. `autogen.selector` constrains generic charts inside this profile's `match` scope while retaining samples.                     |
| `template`      |   yes    | Chart template group defining the curated charts. At least one chart.                                                                                |

The filename must match `^[a-z][a-z0-9_]*$` (plus the `.yaml` or `.yml` extension): lowercase, starting with a letter,
using only letters, digits, and underscores -- `my_app.yaml`, not `my-app.yaml` or `MyApp.yaml`. The basename is the
profile name used everywhere else -- in `profiles.mode_exact`/`mode_combined` entries and in log messages.

### `match`

`match` decides whether the profile applies to a job:

- It is tested against the **metric family names** of the scraped metrics -- the series name with the
  `_bucket`/`_sum`/`_count` suffix removed for histograms and summaries (`example_http_request_duration_seconds`, not
  `example_http_request_duration_seconds_bucket`). Counters keep their `_total` suffix in the family name -- Netdata
  does not strip it, so match `foo_total` or a glob, never a bare `foo`. (This is the opposite of the template's
  dimension selectors and the relabeling block `match`, which both work on the full suffixed names -- see
  [Chart template rules](#chart-template-rules) and [the relabeling block `match`](/src/go/plugin/go.d/collector/prometheus/relabel/README.md#match).)
- It sees the names **after** the job's `selector` and `relabeling` have been applied, so a rename can bring an
  endpoint's metrics into (or out of) a profile's match.
- It sees names **before** this profile's own `relabeling`. A profile cannot rename an otherwise non-matching metric
  into its own selection scope; use job relabeling when normalization is required to select the profile itself.
- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.

Keep the pattern narrow -- anchored to the exporter's metric prefix, such as `haproxy_*`. In the default `auto` mode
every catalog profile whose `match` hits at least one scraped metric family is selected, so an over-broad pattern
attaches the profile to unrelated jobs.

### `fallback_type`

`fallback_type` makes a selected profile self-contained when its exporter omits Prometheus `TYPE` metadata for scalar
metrics. Declared gauges, counters, histograms, and summaries retain their declared type. Unsupported declared types,
such as OpenMetrics info and state-set families, remain unwritable; fallback policy cannot convert them. A chart's
`algorithm` cannot replace classification: the collector must accept and type the sample before the chart engine can
route it.

```yaml
match: 'example_*'
fallback_type:
  gauge:
    - example_open_connections
    - 'example_*_bytes'
  counter:
    - 'example_events_*'
template:
  # ...
```

- Each item is a Go shell-style glob matched against the exact post-job, pre-profile scalar family name. Patterns must
  not be blank or have leading or trailing whitespace. Both lists are optional, but at least one pattern is required
  when `fallback_type` is present.
- The profile's root `match` is an additional scope guard. A broad fallback glob such as `*` never classifies a family
  outside that selected profile's `match`.
- Precedence is: declared Prometheus type; job `fallback_type.gauge`; job `fallback_type.counter`; the first selected
  profile that classifies the family; implicit counter for an `_total` suffix. Within one profile, `gauge` wins over
  `counter`.
- Profile order is the same order used for normalization: profile-name order in `auto`, configured entry order in
  `exact`, and configured entries followed by remaining name-ordered auto profiles in `combined`.
- Classification is bound before profile relabeling. A rename preserves the chosen type, but a rename cannot make an
  otherwise ineligible sample match `fallback_type` or become a counter merely by adding `_total`.
- Use profile-owned rules for stable exporter behavior. Use job-owned `fallback_type` only for deployment-specific
  overrides; job rules deliberately take precedence over every profile. Keep job patterns narrow: a broad rule such as
  `gauge: ['*']` overrides every profile counter classification in its scope.

### `relabeling`

`relabeling` stores normalization required by the exporter profile itself. It uses the exact same ordered block and
rule format as job-level [metric relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md), including the
full action set. Use it when the profile's template needs a stable metric or label shape across exporter versions. Use
job-level relabeling for deployment-specific policy, filtering, or normalization needed before profile selection.

```yaml
match: 'example_*'
relabeling:
  - match: example_requests
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: example_requests
        target_label: __name__
        replacement: example_http_requests_total
template:
  # Select example_http_requests_total here.
  # ...
```

Profile relabeling uses first-applicable precedence over the shared metric stream:

- It runs only for selected profiles and cannot affect profile selection.
- A profile is applicable to an original source family when its root `match` covers the base family name and at least
  one of its relabeling blocks matches an original physical series name. Rule results and output names do not affect
  this decision.
- The first applicable profile processes every series in that source family through its complete pipeline. Later
  profile pipelines do not see the family, including names produced by the first pipeline.
- Normalizer precedence is profile-name order in `auto`; configured entry order in `exact`; and configured entries
  first followed by remaining auto-selected profiles in profile-name order in `combined`.
- All selected templates consume the same final stream. A first profile's name or label changes are therefore visible
  to every selected profile; profiles do not receive private copies of the source metrics.
- The root `match` constrains source applicability, not output names. Profile authors are responsible for making the
  produced names and labels agree with every selected template that consumes them.
- If a produced family no longer matches the profile's root `match`, that profile's `autogen.selector` no longer applies
  to it either. An uncovered output family therefore keeps generic autogen unless another applicable profile scope
  rejects it.
- Histogram and summary integrity is validated independently after job and profile relabeling. Partial component
  renames/drops, structural `le`/`quantile` changes, splits, and merges are rejected during checking and contained by
  dropping the corrupted family if they first appear at runtime.

### `autogen.selector`

`autogen.selector` uses the existing metric selector `allow`/`deny` shape to control fallback charts. It runs only
after no authored chart-template dimension matched the flattened series, and only when this profile's `match` applies
to the final post-profile family that reaches the chart engine.

- `allow` alone keeps fallback only for selected series.
- `deny` alone keeps fallback for everything except selected series.
- With both, the result is `allow AND NOT deny`.
- At least one non-empty `allow` or `deny` entry is required. Empty `autogen`, null or empty `selector`, both lists
  absent or empty, whitespace-only entries, and invalid selector expressions are rejected.
- Each entry accepts the same metric-name and label expression syntax as other metric selectors. The selector receives
  the final post-profile family as `__name__` plus the relabeled series' current labels.
- When multiple selected profile scopes apply, every applicable selector must accept the series. One rejection
  suppresses fallback, and profile order cannot change the result. Profiles without `autogen.selector` add no rule.
- Histograms and summaries use their base family as `__name__`: `foo_bucket`, `foo_sum`, and `foo_count` all evaluate
  as `foo`. Structural labels remain visible, including `le` and `quantile`. StateSet state labels and MeasureSet
  `measure_field` are also available.
- Authored routing wins. A profile can create a heatmap from `foo_bucket` and deny `foo`; the bucket series still
  reaches the heatmap, while unmatched fallback charts for `foo_sum` and `foo_count` are suppressed.
- This is chart suppression, not ingestion filtering. Use the job `selector` or a
  [relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md) `drop` rule when the
  matching samples must be discarded.

Example:

```yaml
match: 'example_*'
autogen:
  selector:
    deny:
      - example_http_request_duration_seconds
template:
  # ...
```

> **Stock contribution policy:** the runtime syntax above remains available to
> user-owned profiles, including `allow` and wildcard selectors. Profiles
> contributed to Netdata MUST NOT use `autogen.selector.allow` or open-ended
> `deny` entries: unknown future families matching `match` must remain eligible
> for generic fallback. Contributed denies name exact family base names present
> in the source-complete fixture only. A selector containing `{...}` is
> label-constrained policy, not an exact family name. The objective profile
> validator enforces this policy separately from its strict zero-fallback check
> over current source-complete evidence.

Stock profile and recommended-job relabeling follow the same forward-open rule.
Under a wildcard relabel block, a sample-discarding rule may use only a
`__name__` `drop` that enumerates finite exact names or one non-empty internal
entity key between finite exporter prefixes and finite terminal metric
suffixes. Every finite exact name or prefix/suffix branch must be exercised by
the source-complete fixture. Open-ended terminal regexes, wildcard
`dropequal`, inverse `keep`/`keepequal`, and application-label-dependent
discard are not accepted stock-authoring patterns; runtime support remains
available for user-owned jobs.

Exact recommended-job selector denies and exact profile/job relabel-block metric
names must also be present in the source-complete fixture. Every exact-scope
discard rule must drop
at least one fixture sample at its real ordered pipeline position. A wildcard
name-derived rewrite follows the bounded drop grammar and fixture-evidence rule;
an internal-key rewrite cannot reference its dynamic capture, and every finite
output must be an authored canonical metric. Before either dynamic form, the
same block must copy unchanged from `__name__` the capture that encloses the
entire dynamic entity region into a reserved static non-`__name__` label; a
nested capture covering only one alternative is incomplete. That target must be
absent from source-fixture block inputs and preserved through every reachable
later rule or block. A later label write sourced only from `__name__` is
harmless when its regex is disjoint from every possible current name. Canonical
outputs must preserve every finite prefix/suffix branch distinction.
Capture-bearing replacement reachability also preserves literal prefixes and
suffixes around the captures. The rewrite may additionally normalize a
source-proven `<canonical_name>_<non-empty-identity>` family exactly to
`<canonical_name>`.
This rewrite-only canonical form does not permit unrelated terminal catches.
Across exact and wildcard blocks, the complete recommended relabel pipeline
must also preserve every observed writer-admissible logical identity after
normal histogram/summary assembly. Distinct source identities may not converge
on the same final metric name and labels: metric relabeling does not aggregate
values when a name rewrite or label removal collapses identities.
Writer-rejected samples such as non-finite scalars do not participate in this
collision proof.
Every source-fixture sample reached by a metric-name rewrite must also retain a
valid non-empty name. Use an explicit bounded, source-evidenced `drop` rule for
an intentional exclusion instead of relying on invalid replacement output to
discard the sample.

### `app`

`app` is the application identity of the exporter the profile understands -- the `app` segment of `prometheus.<app>.*`
chart contexts, which the Netdata UI turns into an Applications dashboard section. It is used only when the job has no
`app` of its own (set by the user or by service discovery); a configured job `app` always wins, and the job name is the
last resort. Stock profiles set `app` and `template.context_namespace` to the profile name so contexts stay short and
aligned; follow that convention.

Stock application metadata examples omit the job `app` when their automatically selected profiles provide one
unambiguous application identity. Set a job `app` only for an intentional override, to disambiguate selected profiles
that declare different apps, or when no selected profile supplies an app.

## Chart template rules

The `template` value is one group of Netdata's dynamic chart-template format. See the complete
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md) for every group, chart, instance, label, lifecycle,
dimension, presentation, and selector field. Prometheus profiles add these rules:

- **The template is a group, not a full spec.** The linked reference's examples show complete `charts.yaml` specs (a
  `version` plus a top-level `groups` list); a profile's `template` is one item of that `groups` list, written without
  the leading dash -- you author the *content of one group*: `family`, `context_namespace`, `metrics`, `charts`, nested
  `groups`, and `chart_defaults`. `instances`, `lifecycle`, and `label_promotion` are per-chart fields, not group fields
  (`instances` and `label_promotion` can also be set once for a whole group via `chart_defaults`). The spec-level
  `version` and `engine` fields are rejected. The collector wraps your group into its per-job spec, where autogeneration
  for uncovered metrics stays enabled. Configure conditional fallback through the profile-root `autogen.selector`, not
  a nested `engine`.
- **Set `context_namespace` at the template root to the profile name.** This is the group-level `context_namespace`
  field -- a profile has no separate top-level one; the collector supplies the `prometheus.<app>` prefix. The emitted
  context is `prometheus.<app>.<context_namespace>.<chart context>`, with the namespace segment dropped when it equals
  the resolved app.
- **Leave chart `id` unset.** The format allows an explicit `id`, but when omitted the ID is derived from the chart's
  composed context, keeping chart identities aligned with contexts -- every stock profile relies on that. Keep every
  chart's `context` unique within the profile -- when the same measurement exists at several scopes, prefix the context
  with the scope (`process_requests`, `frontend_requests`, `backend_requests`).
- **`algorithm` is optional.** When omitted, the engine uses the collected series type for each rendered dimension:
  counters are incremental and gauges are absolute, regardless of metric name. The examples here set it explicitly for
  readability. Use an explicit chart algorithm for an intentional type override or when differently typed series are
  deliberately aggregated into the same rendered dimension.
- **Dimension selectors address scraped series by their exposition names**, after `selector` filtering and relabeling.
  This is the opposite surface from the profile's `match`: a histogram dimension selector must use the suffixed name
  (`foo_bucket`), and one written with the family name (`foo`) matches nothing -- the curated chart silently never
  appears. The names:
  - a gauge or counter `foo` is the series `foo`;
  - a histogram `foo` is selectable as `foo_bucket` (per-`le` bucket dimensions, rendered as a heatmap), `foo_count`,
    and `foo_sum`;
  - a summary `bar` is selectable as `bar` (per-`quantile` dimensions), `bar_count`, and `bar_sum`;
  - series keep their scraped labels, which is what `instances.by_labels`, `name_from_label`, and label selectors work
    on.
- **Choose `aggregation` when an instance identity omits labels.** Multiple full-label series can then map to the same
  rendered chart and dimension. Set it on the chart; it applies to all dimensions in that chart. When omitted, the reducer
  is `sum`. Use separate charts when metrics require different reducers. Use `min`, `max`, or `avg` when the metric is not
  additive:
  - `sum`: additive counters/gauges and histogram buckets, counts, or sums;
  - `max`: latest timestamp or "any source active" for a 0/1 state;
  - `min`: oldest timestamp, lowest limit, or "all sources active" for a 0/1 state;
  - `avg`: unweighted arithmetic mean or fraction active for a 0/1 state; it emits floating-point values.

  Metric type does not determine the right reducer: Prometheus gauges can be stocks, states, timestamps, limits, or
  averages. Summary quantiles cannot be merged into a global quantile; `avg` is not a weighted mean; and non-sum
  reduction of cumulative counters can produce misleading rates when source membership changes. `instances.by_labels`
  lowers emitted chart cardinality; `aggregation` only selects the value for resulting collisions. Every scraped series is
  still processed and retained in the collector's metric store. This chart reduction is separate from Prometheus
  relabeling: it does not remove or rewrite stored series labels.
- **Only collected series can be charted.** `*_info` families are skipped. Untyped scalar families are collected only
  when the selected profile or job `fallback_type` maps them to a gauge or counter, or when the name ends in `_total`
  (the last-resort implicit counter rule). Declared type wins; job policy wins over profile policy; and an explicit
  gauge match wins over counter classification within the same policy layer. A chart's `algorithm` acts later and
  cannot make an unclassified sample collectible.
- **Every group that contains charts must list the metrics its selectors reference in its `metrics` list** (or inherit
  them from an ancestor group). A selector on a metric outside the group's declared scope fails validation.

## Selecting profiles in job configuration

The job's `profiles.mode` controls which catalog profiles apply:

- `auto` (default): every profile whose `match` hits at least one scraped metric family.
- `exact`: only the profiles named in `mode_exact.entries`; each must match the scraped metrics, or the job fails its
  check.
- `combined`: `auto` plus the profiles named in `mode_combined.entries`, deduplicated. The named entries follow the same
  rule as `exact`: each must match, or the job fails its check.
- `none`: no profiles -- generic autogen charts only.

```yaml
jobs:
  - name: myapp
    url: http://127.0.0.1:9090/metrics
    profiles:
      mode: exact
      mode_exact:
        entries:
          - name: example
```

Only the block matching the selected mode (`mode_exact` or `mode_combined`) is read, and a name that exists in neither
the stock nor the user catalog is a configuration error in both modes. Scraped metrics not charted by any selected
profile keep their generic autogen charts unless an applicable profile `autogen.selector` rejects them. Use
`autogen.selector` to constrain fallback charts while retaining samples; use the job's `selector` or a `relabeling`
drop rule to discard samples.

Stock application examples rely on the default `auto` mode and omit `profiles`. Their application and reusable
support profiles must select from their own exporter signatures. Copying the proof bundle's exact
candidate-plus-support list into the job would make every named optional support namespace mandatory. Exact selection
remains useful while developing a user profile or when an operator deliberately wants to pin deployment policy.

When selected profiles contain fallback classification or relabeling, the mode also determines policy precedence:
`auto` uses profile-name order, `exact` uses entry order, and `combined` tries its configured entries first and then the
remaining auto-selected profiles in profile-name order. Profile fallback uses the first matching classification;
profile relabeling uses the first applicable normalizer. Profile selection, app fallback, and template composition
retain their existing behavior.

`profiles`, `relabeling`, `selector`, `fallback_type`, and `app` are all job options in `go.d/prometheus.conf` -- edit
it with `sudo ./edit-config go.d/prometheus.conf` from your
[Netdata config directory](/docs/netdata-agent/configuration/README.md).

## Authoring workflow

1. Inspect what the endpoint actually exposes -- family names, types, and labels are exactly what `match` and the
   template's selectors work on:

   ```bash
   curl -s http://127.0.0.1:9090/metrics | grep '# TYPE'
   curl -s http://127.0.0.1:9090/metrics | grep -v '^#' | head -20
   ```

   The second command shows the series themselves -- the `{label="value"}` pairs are what `instances.by_labels`,
   `name_from_label`, and selector label filters work on. If the exporter omits `# TYPE` for scalar metrics, declare
   their stable types in the profile so the profile works without special job configuration:

   ```yaml
   match: 'myapp_*'
   fallback_type:
     gauge: [myapp_queue_size, 'myapp_temp_*']
     counter: ['myapp_events_*']
   ```

   Keep job-level `fallback_type` for deployment-specific overrides. It has higher precedence than profile policy.

2. Start from the [Complete example](#complete-example) above or from a stock profile (for example
   [`haproxy.yaml`](/src/go/plugin/go.d/config/go.d/prometheus.profiles/default/haproxy.yaml), installed under
   `/usr/lib/netdata/conf.d/go.d/prometheus.profiles/default/`). Name the file after the exporter.
3. Save it under `prometheus.profiles/` in your Netdata user config directory (typically `/etc/netdata`); the file must
   be readable by the `netdata` user. Then restart the Netdata Agent (the profile catalog is cached for the plugin's
   lifetime):

   ```bash
   sudo mkdir -p /etc/netdata/go.d/prometheus.profiles/
   sudo cp myapp.yaml /etc/netdata/go.d/prometheus.profiles/
   sudo systemctl restart netdata
   ```

4. While iterating, pin the job to the profile with `profiles.mode: exact` -- a profile that fails to match then fails
   the job check loudly instead of silently falling back to autogen charts. Check the log:

   ```bash
   journalctl -u netdata --no-pager | grep -E 'profile|prometheus'
   ```

   On success the collector logs `profiles: mode "exact" selected 1 profile(s): example`. Failures do not carry the
   `profiles:` prefix: a non-matching exact profile fails the job check with
   `profile "example" matches no scraped metric (pattern ...)`, and a broken user profile is skipped once at catalog
   load with `ignoring invalid user profile ...` followed by the validation error.

   For a faster loop than restarting the Agent, run the plugin in debug mode. It loads user profiles from the same
   directories as the Agent and prints selection and validation messages at startup, then keeps collecting until you
   press Ctrl+C. The plugin path varies by install type -- static installs keep it under `/opt/netdata`:

   ```bash
   sudo -u netdata /usr/libexec/netdata/plugins.d/go.d.plugin -d -m prometheus
   ```

5. Verify the dashboard: the curated charts appear in the node's left-hand menu under the section named by the
   template's `family`, with `prometheus.<app>.*` contexts. Uncovered metrics remain as autogen charts unless an
   applicable profile `autogen.selector` rejects them.

If the profile is selected but a curated chart is missing or empty, the usual causes are: a dimension selector written
with the metric family name instead of the suffixed exposition name (`foo` instead of `foo_bucket`);
`instances.by_labels` or `name_from_label` naming a label the series does not carry; or the metric not being collected
at all (untyped without `_total` or a profile/job `fallback_type`). When fallback policy accepts the family, the
giveaway is the metric appearing as a generic autogen chart instead of in your curated one. Whether a syntactically
valid selector matches observed series is not validated -- neither the job check nor debug mode flags a selector that
matches nothing -- so re-run the commands from step 1 and compare the exact series names and label keys against your
`metrics` lists, selectors, and labels.

To contribute a profile to Netdata, add it under `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/`. Stock
profiles are held to a stricter standard than user profiles. They require source-complete, sanitized evidence; a structured
job policy; an operator model and source-family reconciliation; and an objective validator `PASS` with zero current-source
fallback and zero unmatched series. A broken stock profile is not skipped -- an invalid header, name, or duplicate fails
the whole catalog, and an invalid template fails every job that selects it.

Compact proof documents live under
`src/go/plugin/go.d/collector/prometheus/profile-proofs/<profile>/`. Bulky generated evidence lives in
[`netdata/testdata`](https://github.com/netdata/testdata) under `prometheus/profiles/<profile>/`: source semantics,
optional generated source registries, and sanitized exposition fixtures. Netdata tests clone latest testdata `master`;
update the profile-owned external evidence and its main-tree semantic design together. Historical Netdata checkouts are
not guaranteed to validate against later testdata content.

To run the complete stock-profile gate locally, clone testdata into the ignored location, then require external evidence:

```bash
git clone --depth=1 --branch master https://github.com/netdata/testdata.git src/go/testdata
cd src/go
NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1 go test -count=1 \
  ./internal/promprofile/testutil \
  ./internal/promprofile/validation \
  ./tools/prometheus-profile-validation \
  ./plugin/go.d/collector/prometheus/...
```

For an existing checkout, shallow-fetch `origin master` and detach at the fetched tip before replay. This preserves any local
feature branch while making the tested tree exactly the fetched `master`:

```bash
git -C src/go/testdata fetch --depth=1 origin master
git -C src/go/testdata switch --detach FETCH_HEAD
```

Set `NETDATA_TESTDATA_DIR` if the checkout lives elsewhere. Ordinary tests never fetch testdata and skip only the
external-dependent cases when the checkout root is absent. A present but incomplete or unreadable checkout fails. The
dedicated CI workflow requires external evidence, verifies exact external layout and generated-registry reproducibility,
and replays the objective validator and semantic contract for every stock proof.
