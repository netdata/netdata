# Prometheus profile format

Prometheus profiles ship curated charts for recognized exporters. Without a profile, the generic Prometheus collector
renders one chart per scraped metric (autogeneration). A profile replaces that flat list, for the metrics it knows, with
a designed dashboard menu: named sections, per-instance charts, meaningful dimensions, units, and heatmaps. You are not
limited to the stock library -- you can author a profile for your own application's metrics too.

This page documents the profile file format and the job-level [metric relabeling](#metric-relabeling) option that
reshapes scraped metrics before profiles and charts see them.

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

A profile is a YAML file with three top-level keys: `match` selects the profile by scraped metric names, `app` names the
application the charts belong to, and `template` defines the charts. Everything below `template` is Netdata's
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md); inline comments explain each field.

```yaml
# example.yaml -- the basename (before .yaml/.yml) is the profile name
match: 'example_*'          # REQUIRED. Netdata simple pattern over scraped metric
                            # family names; one hit selects the profile for the job.
app: example                # optional. Application identity: charts appear under
                            # the prometheus.<app>.* contexts and the app's own
                            # Applications section in the UI, unless the job
                            # config sets its own `app`.

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
   ([selector syntax](/src/go/pkg/prometheus/selector/README.md)), and [relabeling](#metric-relabeling) rewrites metric
   names and labels. Profiles and charts only ever see the result.
2. **Selection** -- once, at job autodetection, each profile's `match` is tested against the scraped metric family
   names, per the job's `profiles.mode` (see
   [Selecting profiles in job configuration](#selecting-profiles-in-job-configuration)). The selection is cached until
   the job restarts.
3. **App resolution** -- the job's `app` option wins; when unset, the first selected profile that declares an `app`
   provides it; the job name is the last resort. If selected profiles declare different apps, the first (in selection
   order) wins and the rest are logged -- set the job's `app` to disambiguate.
4. **Charts** -- the selected profiles' templates are merged on top of the autogeneration base. Series matched by a
   profile's selectors get the curated charts; every other scraped metric family keeps its generic autogen chart, so
   profile coverage never loses data.

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
  [Chart template rules](#chart-template-rules) and [the relabeling block `match`](#the-relabeling-block-match).)
- It sees the names **after** the job's `selector` and `relabeling` have been applied, so a rename can bring an
  endpoint's metrics into (or out of) a profile's match.
- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.

Keep the pattern narrow -- anchored to the exporter's metric prefix, such as `haproxy_*`. In the default `auto` mode
every catalog profile whose `match` hits at least one scraped metric family is selected, so an over-broad pattern
attaches the profile to unrelated jobs.

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
  for uncovered metrics stays enabled.
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
profile keep their generic autogen charts -- a profile cannot disable autogeneration; to hide metrics you do not chart,
drop them with the job's `selector` option or a `relabeling` drop rule.

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
   template's `family`, with `prometheus.<app>.*` contexts, and uncovered metrics remain as autogen charts.

If the profile is selected but a curated chart is missing or empty, the usual causes are: a dimension selector written
with the metric family name instead of the suffixed exposition name (`foo` instead of `foo_bucket`);
`instances.by_labels` or `name_from_label` naming a label the series does not carry; or the metric not being collected
at all (untyped without `_total` or `fallback_type`). The giveaway is the metric appearing as a generic autogen chart
instead of in your curated one. Selector matching is not validated -- neither the job check nor debug mode flags a
selector that matches nothing -- so re-run the commands from step 1 and compare the exact series names and label keys
against your `metrics` lists, selectors, and labels.

To contribute a profile to Netdata, add it under `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/`. Stock
profiles are held to a stricter standard than user profiles: a broken stock profile is not skipped -- an invalid header,
name, or duplicate fails the whole catalog, and an invalid template fails the check of every job that selects it. The
collector test suite validates every stock profile before merge:

```bash
cd src/go
go test -count=1 ./plugin/go.d/collector/prometheus/...
```

## Metric relabeling

Relabeling rewrites, adds, drops, or filters a scraped metric -- its name and its labels -- **before** profiles are
matched and charts are built. It is a job-level configuration option (`relabeling`), not a profile field: it works with
or without profiles, and profiles see the relabeled result. The rule shape is Prometheus-compatible: it mirrors
Prometheus
[`metric_relabel_configs`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs),
so rules you already use in Prometheus carry over -- with these deliberate exceptions: the `__name__` handling of the
label-name actions (see [Actions](#actions)), and the histogram/summary integrity protection, which rejects rules --
such as thinning buckets by `le` -- that Prometheus allows (see
[Histogram and summary safety](#histogram-and-summary-safety)). When porting, wrap your flat `metric_relabel_configs`
list in a `relabeling` block with a required `match` (use `'*'` for all metrics). The job's `selector` option runs
first, on the original scraped names -- relabeling sees only what the selector kept, so a rename cannot recover a series
the selector dropped under its old name.

Use it to keep noisy metrics out of your dashboards, fold high-cardinality labels away, derive new labels, rename
metrics, normalize label casing -- or rename an exporter's metrics so a stock profile recognizes them -- without
touching the application you are scraping.

> **Note:** The removed `label_prefix` option prefixed every label key. Relabeling covers its practical use case --
> avoiding collisions with the labels Netdata adds when re-exporting -- with a targeted `labelmap` rule (see the
> [example](#prefix-label-names-replaces-label_prefix)). A blanket all-labels rename has no direct equivalent:
> `labelmap` copies labels, it does not rename them.

### How a rule reads

Relabeling operates per scraped **series** (one sample line of the exposition), not per metric family. Each rule
transforms one series in a few steps:

1. **Join** the values of the metric's `source_labels` (with `separator`) into one input string. The metric's own name
   is available as a special label, `__name__`.
2. **Match** that input against `regex`. The `regex` is a regular expression that must match the **whole** input, not a
   substring (wrap it in `.*` to match a part).
3. **Act**: the `action` decides what happens -- set or rename a label, drop or keep the metric, and so on. If you omit
   `action`, it defaults to `replace`.
4. **Write** -- `replace` writes the expanded `replacement` (with `regex` capture groups: `$1`, or `${name}`) into
   `target_label`; use `__name__` to rename the metric. `lowercase`/`uppercase`/`hashmod` also write `target_label`, but
   without `replacement` expansion. The keep/drop actions write nothing, and the label-name actions operate on label
   names directly, never on `target_label` -- see [Actions](#actions).

Two different matchers are at play -- don't confuse them: a block's `match` is a **glob** (simple pattern) over the
metric name, while a rule's `regex` is an **anchored regular expression** over the joined label values.

### Quick start

Add a `relabeling` block to a job. This drops the `go_gc_duration_seconds` metric and derives an HTTP `code_class` label
(`2xx`, `4xx`, ...) on every `http_*` metric:

```yaml
jobs:
  - name: myapp
    url: http://127.0.0.1:9090/metrics
    relabeling:
      - match: 'go_gc_duration_seconds*'
        metric_relabel_configs:
          - action: drop
      - match: 'http_*'
        metric_relabel_configs:
          - source_labels: [code]
            regex: '(\d)\d\d'
            target_label: code_class
            replacement: '${1}xx'
```

The drop and label-deriving techniques are explained step by step under [Relabeling examples](#relabeling-examples).

### Relabeling configuration

`relabeling` is a **list of blocks**. Each block has a `match` and a list of rules; the rules apply only to metrics
whose name matches the block's `match`. Grouping rules under a block lets you scope a set of rules to a subset of
metrics by name, instead of repeating a name match in every rule. Relabeling is opt-in -- jobs without a `relabeling`
section are unaffected.

```yaml
jobs:
  - name: myapp
    url: http://127.0.0.1:9090/metrics
    relabeling:
      - match: '<simple-pattern>'           # REQUIRED
        metric_relabel_configs:
          - source_labels: [<label>, ...]
            separator: ';'
            regex: '<regexp>'
            modulus: <uint>
            target_label: '<label>'         # or __name__ to target the metric name
            replacement: '$1'
            action: <action>
```

#### The relabeling block `match`

`match` selects which metrics a block's rules apply to. It is **required**.

- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.
- It is matched against the **full metric name, including** the `_bucket`/`_sum`/`_count` suffixes of histograms and
  summaries. Prefer a glob like `app_lat*` over an exact `app_lat`, otherwise the histogram's `_bucket`/`_sum`/`_count`
  series will not match. (Profile [`match`](#match) works on family base names instead.)
- Use `*` to target **every** metric.

Blocks run in order, and `match` sees each metric's **current** name -- so a block can match the new name produced by an
earlier block's rename.

#### Rule fields

Inside a block, `metric_relabel_configs` is a list of rules applied **in order** (see
[How a rule reads](#how-a-rule-reads)). Every field is optional except where an action requires it; `action` defaults to
`replace`.

| Field           | Description                                                                                                                              | Default   |
|:----------------|:-----------------------------------------------------------------------------------------------------------------------------------------|:----------|
| `source_labels` | Label names whose values are joined (with `separator`) into the rule's input string. Use `__name__` for the metric name.                 | --        |
| `separator`     | String placed between joined `source_labels` values.                                                                                     | `;`       |
| `regex`         | Regular expression matched against the joined input. It is **fully anchored** (compiled as `^(?s:...)$`, so it must match the whole input and `.` also matches newlines); to match a substring, wrap it in `.*`. | `(.*)`    |
| `modulus`       | Positive integer, used only by the `hashmod` action.                                                                                     | --        |
| `target_label`  | The label the action writes. Use `__name__` to write the metric name. For `replace`, it also expands `regex` capture groups (`$1`, `${name}`), like `replacement`. | --        |
| `replacement`   | The value written, with regex capture-group expansion (`$1`, or `${name}` for named groups).                                             | `$1`      |
| `action`        | One of the [actions](#actions) below.                                                                                                    | `replace` |

#### Actions

| Action      | What it does                                                                                                                                                                                            |
|:------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `replace`   | If `regex` matches the joined input, set `target_label` to the expanded `replacement` (an empty result removes the label); a non-match leaves the label unchanged. With no `source_labels`, the default `regex` matches the empty input, so `replace` writes a constant `replacement` -- the standard add-a-static-label idiom. See [example](#add-a-derived-label). |
| `keep`      | Keep the series only if `regex` matches the joined input; otherwise drop it. See [example](#keep-only-specific-metrics).                                                                                |
| `drop`      | Drop the series if `regex` matches the joined input. See [example](#drop-metrics-you-dont-want).                                                                                                        |
| `keepequal` | Keep the series only if `target_label`'s value equals the joined input. Accepts only `source_labels` and `target_label`.                                                                                |
| `dropequal` | Drop the series if `target_label`'s value equals the joined input. Accepts only `source_labels` and `target_label`.                                                                                     |
| `hashmod`   | Set `target_label` to `md5(joined input) mod modulus` (Prometheus-compatible hashing) -- useful for sharding.                                                                                           |
| `labelmap`  | For every label whose **name** matches `regex`, copy its value to a new label whose name is `replacement` with capture groups from the label name expanded (e.g. `app_$1`). See [example](#prefix-label-names-replaces-label_prefix). |
| `labeldrop` | Remove every label whose name matches `regex`. See [example](#remove-a-high-cardinality-label).                                                                                                         |
| `labelkeep` | Remove every label whose name does **not** match `regex`. See [example](#keep-only-a-few-labels).                                                                                                       |
| `lowercase` | Set `target_label` to the lowercased joined input. See [example](#normalize-label-case).                                                                                                                |
| `uppercase` | Set `target_label` to the uppercased joined input. See [example](#normalize-label-case).                                                                                                                |

> **Differs from Prometheus.** The label-name actions `labelmap`, `labeldrop`, and `labelkeep` act on labels only --
> they never touch the metric name (`__name__`). In Prometheus, `__name__` is visible to these three actions; a ported
> rule that relied on that (a `labelkeep` whose regex omits `__name__`, a blanket `labeldrop`, a `labelmap` over the
> metric name) silently leaves the metric name untouched in Netdata. To rename a metric, use a `replace` rule with
> `target_label: __name__`.

### How relabeling runs

1. For each scraped metric, every block whose `match` matches the metric's **current** name runs, in order.
2. Within a block, rules run in order. Each rule joins `source_labels` into an input string and applies its `action`.
3. A rule can rename the metric (write `__name__`), so a later block or rule sees the new name.
4. If any rule drops the metric, the remaining rules and blocks are skipped for it.

#### Histogram and summary safety

A histogram or summary is assembled from several series -- `_bucket`/`_sum`/`_count`, or per-quantile series. Netdata
will **not** let relabeling silently corrupt one. A rule that would:

- split a histogram/summary across multiple metric names,
- drop only some of its components,
- change its `le` or `quantile` label,
- merge two metric families together, or
- duplicate a component,

is rejected. The job **fails at autodetection**, so a bad rule is caught immediately; if the exposition changes at
runtime, the affected metric family is **dropped** (rather than charted with wrong values) and the rest of the job keeps
working.

This also covers the common Prometheus technique of thinning histogram buckets: a `keep`/`drop` rule on the `le` (or
`quantile`) label is treated as corrupting the metric family, never as thinning it -- the job fails its check when the
check scrape shows the family partially dropped or mutated, and the whole metric family is dropped if the exposition
drifts into corruption later at runtime. Reduce histogram cardinality at the exporter or with the job's `selector`
option instead.

Plain gauges and counters have no such restriction -- you can rename, relabel, split, or drop them freely.

### Relabeling examples

#### Drop metrics you don't want

Drop everything named `go_*`:

```yaml
relabeling:
  - match: 'go_*'
    metric_relabel_configs:
      - action: drop
```

`match: 'go_*'` already selects the metrics, so the rule needs no `regex` -- it drops every metric the block sees.

#### Keep only specific metrics

Within a metric family, drop everything except a chosen series by matching a label. This keeps only
`node_systemd_unit_state` samples whose `state` is `active`, dropping the rest:

```yaml
relabeling:
  - match: 'node_systemd_unit_state'
    metric_relabel_configs:
      - source_labels: [state]
        regex: 'active'
        action: keep
```

`keep` drops any sample whose `state` label is not exactly `active` (the `regex` is fully anchored).

#### Add a derived label

Derive an HTTP `code_class` (`2xx`, `4xx`, ...) from a numeric `code` label:

```yaml
relabeling:
  - match: 'http_*'
    metric_relabel_configs:
      - source_labels: [code]
        regex: '(\d)\d\d'
        target_label: code_class
        replacement: '${1}xx'
```

For `code="404"`, the rule writes `code_class="4xx"`. The original `code` label is left untouched.

#### Rename a metric

Write `__name__` to rename the metric itself:

```yaml
relabeling:
  - match: 'legacy_app_requests_total'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'legacy_(.*)'
        target_label: __name__
        replacement: '$1'
```

`legacy_app_requests_total` becomes `app_requests_total`. (Renaming a histogram/summary is allowed only when all of its
components are renamed together -- see [Histogram and summary safety](#histogram-and-summary-safety).)

#### Remove a high-cardinality label

Drop a label that explodes cardinality and clutters charts:

```yaml
relabeling:
  - match: '*'
    metric_relabel_configs:
      - regex: 'pod_uid'
        action: labeldrop
```

Every `pod_uid` label is removed from all metrics. To drop several, use an alternation like
`regex: 'pod_uid|instance_id'`.

#### Keep only a few labels

Strip a metric down to a known set of labels, removing everything else:

```yaml
relabeling:
  - match: 'mysql_*'
    metric_relabel_configs:
      - regex: 'instance|job'
        action: labelkeep
```

`labelkeep` removes every label whose name is **not** in the regex. The metric name (`__name__`) is always kept, so you
do not list it.

#### Normalize label case

Lowercase a label value so `GET`, `Get`, and `get` collapse into one dimension:

```yaml
relabeling:
  - match: 'http_*'
    metric_relabel_configs:
      - source_labels: [method]
        target_label: method
        action: lowercase
```

Use `uppercase` for the opposite. These actions take `source_labels`, `separator`, and `target_label` (the target is
often the same label).

#### Prefix label names (replaces `label_prefix`)

When these metrics are re-exported in Prometheus format, Netdata adds its own `instance`, `family`, `chart`, and
`dimension` labels. If the scraped endpoint already uses one of those names, the re-export emits a duplicate label and a
downstream Prometheus rejects the scrape. Rename the colliding labels -- copy them to a prefixed name with `labelmap`,
then drop the originals with `labeldrop`:

```yaml
relabeling:
  - match: '*'
    metric_relabel_configs:
      - regex: '(instance|family)'
        action: labelmap
        replacement: 'app_$1'
      - regex: '(instance|family)'
        action: labeldrop
```

`labelmap` copies `instance` -> `app_instance` and `family` -> `app_family` (the `( )` capture feeds `$1` in
`replacement`); the `labeldrop` then removes the originals -- its anchored regex matches those names exactly, not the
new `app_` ones. Add more names to the alternation for other collisions; `__name__` is never affected. The removed
`label_prefix` option prefixed every label key; the re-export collision shown here is the problem it existed to solve.
Prefix only the labels that actually collide -- a generic all-labels rename is not expressible with these actions,
because `labelmap` copies labels and the originals cannot be dropped without naming them.

#### Scope rules to a subset of metrics

Each block's `match` already scopes its rules. Combine blocks to apply different rules to different metric sets:

```yaml
relabeling:
  - match: 'http_*'
    metric_relabel_configs:
      - regex: 'pod_uid'
        action: labeldrop
  - match: 'db_* cache_*'
    metric_relabel_configs:
      - source_labels: [shard]
        target_label: shard
        action: uppercase
```

The first block touches only `http_*` metrics; the second touches `db_*` and `cache_*`. A metric matched by no block is
left unchanged.
