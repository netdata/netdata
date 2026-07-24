# Prometheus profile format

Prometheus profiles ship curated charts for recognized exporters. Without a profile, the generic Prometheus collector
renders one chart per scraped metric (autogeneration). A profile replaces that flat list, for the metrics it knows, with
a designed dashboard menu: named sections, per-instance charts, meaningful dimensions, units, and heatmaps. You are not
limited to the stock library -- you can author a profile for your own application's metrics too.

This page documents the profile file format. The job-level
[metric relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md) option, which reshapes scraped metrics
before profiles and charts see them, is documented separately.

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

A profile is a YAML file with two required top-level keys (`match` and `template`) plus optional `app` and `autogen`
policy. `match` selects the profile by scraped metric names, `app` names the application the charts belong to,
`autogen.selector` controls fallback charts within the profile's match scope, and `template` defines the curated
charts. Everything below `template` is Netdata's
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md); inline comments explain each field.

```yaml
# example.yaml -- the basename (before .yaml/.yml) is the profile name
match: 'example_*'          # REQUIRED. Netdata simple pattern over scraped metric
                            # family names; one hit selects the profile for the job.
app: example                # optional. Application identity: charts appear under
                            # the prometheus.<app>.* contexts and the app's own
                            # Applications section in the UI, unless the job
                            # config sets its own `app`.

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

At runtime the collector uses profiles in four steps:

1. **Scrape** -- the job scrapes the endpoint. The `selector` job option filters unwanted series
   ([selector syntax](/src/go/pkg/prometheus/selector/README.md)), and
   [relabeling](/src/go/plugin/go.d/collector/prometheus/relabel/README.md) rewrites metric names and labels. Profiles
   and charts only ever see the result.
2. **Selection** -- once, at job autodetection, each profile's `match` is tested against the scraped metric family
   names, per the job's `profiles.mode` (see
   [Selecting profiles in job configuration](#selecting-profiles-in-job-configuration)). The selection is cached until
   the job restarts.
3. **App resolution** -- the job's `app` option wins; when unset, the first selected profile that declares an `app`
   provides it; the job name is the last resort. If selected profiles declare different apps, the first (in selection
   order) wins and the rest are logged -- set the job's `app` to disambiguate.
4. **Charts** -- the selected profiles' templates are merged on top of the autogeneration base. Every series first gets
   a chance to route to every authored template dimension. If no route matches, each selected profile's
   `autogen.selector` is evaluated only when that profile's `match` applies to the source family. Every applicable
   selector must accept the series; one rejection suppresses fallback, independent of profile order. If no selector
   applies, the series keeps its generic autogen chart. A selector never removes samples from the metric store.

Chart contexts compose as `prometheus.<app>.<template context_namespace>.<chart context>` -- in the example above,
`prometheus.example.example.http_requests`. When the resolved app equals the profile's `context_namespace` (the common
case: the job has no `app` set and the profile provides it), the redundant segment is dropped, so the emitted context is
`prometheus.example.http_requests`. Autogen charts for uncovered metrics keep their `prometheus.<app>.<metric>`
contexts.

## Top-level fields

| Field      | Required | Description                                                                                                                                            |
|:-----------|:--------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------|
| `match`    |   yes    | [Netdata simple pattern](/src/libnetdata/simple_pattern/README.md) tested against scraped metric family names. One hit makes the profile applicable.  |
| `app`      |    no    | Application identity used as the `app` segment of chart contexts when the job does not set one. Must match `^[a-z][a-z0-9_]*$`.                       |
| `autogen`  |    no    | Fallback-chart policy. `autogen.selector` constrains generic charts inside this profile's `match` scope while retaining samples.                    |
| `template` |   yes    | Chart template group defining the curated charts. At least one chart.                                                                                 |

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
- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.

Keep the pattern narrow -- anchored to the exporter's metric prefix, such as `haproxy_*`. In the default `auto` mode
every catalog profile whose `match` hits at least one scraped metric family is selected, so an over-broad pattern
attaches the profile to unrelated jobs.

### `autogen.selector`

`autogen.selector` uses the existing metric selector `allow`/`deny` shape to control fallback charts. It runs only
after no authored chart-template dimension matched the flattened series, and only when this profile's `match` applies
to the resolved source family.

- `allow` alone keeps fallback only for selected series.
- `deny` alone keeps fallback for everything except selected series.
- With both, the result is `allow AND NOT deny`.
- At least one non-empty `allow` or `deny` entry is required. Empty `autogen`, null or empty `selector`, both lists
  absent or empty, whitespace-only entries, and invalid selector expressions are rejected.
- Each entry accepts the same metric-name and label expression syntax as other metric selectors. The selector receives
  the source family as `__name__` plus all current series labels.
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
    allow:
      - 'example_*{environment="production"}'
    deny:
      - example_http_request_duration_seconds
      - 'example_debug_*'
template:
  # ...
```

### `app`

`app` is the application identity of the exporter the profile understands -- the `app` segment of `prometheus.<app>.*`
chart contexts, which the Netdata UI turns into an Applications dashboard section. It is used only when the job has no
`app` of its own (set by the user or by service discovery); a configured job `app` always wins, and the job name is the
last resort. Stock profiles set `app` and `template.context_namespace` to the profile name so contexts stay short and
aligned; follow that convention.

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
- **`algorithm` is optional.** When omitted, the engine infers it from the metric (see the Chart Template Format); the
  examples here set it explicitly for readability.
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
- **Only collected series can be charted.** `*_info` families are skipped. Untyped families are collected only when the
  name ends in `_total` (treated as a counter) or the job's `fallback_type` option maps them to a gauge or counter
  (`fallback_type.gauge` takes precedence, so a `_total`-suffixed name mapped to gauge is charted as a gauge). A profile
  cannot chart what the job does not collect.
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
   `name_from_label`, and selector label filters work on. If the endpoint prints no `# TYPE` lines, its metrics are
   untyped: only `_total`-suffixed names are collected (as counters) until the job's `fallback_type` option maps the
   rest:

   ```yaml
   fallback_type:
     gauge: [myapp_queue_size, 'myapp_temp_*']
     counter: ['myapp_events_*']
   ```

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
at all (untyped without `_total` or `fallback_type`). When fallback policy accepts the family, the giveaway is the
metric appearing as a generic autogen chart instead of in your curated one. Whether a syntactically valid selector
matches observed series is not validated -- neither the job check nor debug mode flags a selector that matches nothing
-- so re-run the commands from step 1 and compare the exact series names and label keys against your `metrics` lists,
selectors, and labels.

To contribute a profile to Netdata, add it under `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/`. Stock
profiles are held to a stricter standard than user profiles: a broken stock profile is not skipped -- an invalid header,
name, or duplicate fails the whole catalog, and an invalid template fails the check of every job that selects it. The
collector test suite validates every stock profile before merge:

```bash
cd src/go
go test -count=1 ./plugin/go.d/collector/prometheus/...
```
