# Metric relabeling

Relabeling rewrites, adds, drops, or filters a scraped metric -- its name and its labels -- before charts are built.
The same `relabeling` block format has two configuration levels:

- **Job relabeling** is operator policy. It runs after the job's `selector` and before
  [chart profiles](/src/go/plugin/go.d/collector/prometheus/profile-format.md) are selected, so it can bring an
  endpoint into a profile's expected namespace.
- **Profile relabeling** is exporter/profile policy. It is stored at the profile root, applies automatically only after
  that profile is selected, and normalizes the metrics its template sees. It cannot cause its own selection.

Both levels use the same validation and execution path. The rule shape is Prometheus-compatible: it mirrors
Prometheus
[`metric_relabel_configs`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs),
so rules you already use in Prometheus carry over -- with these deliberate exceptions: the `__name__` handling of the
label-name actions (see [Actions](#actions)), and the histogram/summary integrity protection, which rejects rules --
such as thinning buckets by `le` -- that Prometheus allows (see
[Histogram and summary safety](#histogram-and-summary-safety)). When porting, wrap your flat `metric_relabel_configs`
list in a `relabeling` block with a required `match` (use `'*'` for all metrics). The job's `selector` option runs
first, on the original scraped names -- neither relabeling stage can recover a series the selector dropped.

Use job relabeling to keep noisy metrics out of your dashboards, fold high-cardinality labels away, derive deployment
labels, or rename an endpoint so a profile recognizes it. Put stable exporter normalization required by a curated
profile in the profile itself, so every job that selects that profile gets the same metric contract without duplicating
an optional job policy.

> **Note:** The removed `label_prefix` option prefixed every label key. Relabeling covers its practical use case --
> avoiding collisions with the labels Netdata adds when re-exporting -- with a targeted `labelmap` rule (see the
> [example](#prefix-label-names-replaces-label_prefix)). A blanket all-labels rename has no direct equivalent:
> `labelmap` copies labels, it does not rename them.

## How a rule reads

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

## Quick start

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

The drop and label-deriving techniques are explained step by step under [Relabeling examples](#examples).

## Configuration

`relabeling` is a **list of blocks**. Each block has a `match` and a list of rules; the rules apply only to metrics
whose name matches the block's `match`. Grouping rules under a block lets you scope a set of rules to a subset of
metrics by name, instead of repeating a name match in every rule. Relabeling is opt-in at both levels -- a
job or profile without a `relabeling` section adds no rules at that stage.

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

The same field may appear at the root of a profile. The profile's root `match` controls selection and source-family
applicability; the relabeling block `match` controls which physical series names its rules process:

```yaml
match: 'myapp_*'
relabeling:
  - match: 'myapp_legacy_*'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'myapp_legacy_(.+)'
        target_label: __name__
        replacement: 'myapp_${1}'
template:
  # ...
```

### `match`

`match` selects which metrics a block's rules apply to. It is **required**.

- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.
- It is matched against the **full metric name, including** the `_bucket`/`_sum`/`_count` suffixes of histograms and
  summaries. Prefer a glob like `app_lat*` over an exact `app_lat`, otherwise the histogram's `_bucket`/`_sum`/`_count`
  series will not match. (Profile
  [`match`](/src/go/plugin/go.d/collector/prometheus/profile-format.md#match) works on family base names instead.)
- Use `*` to target **every** metric.

Blocks run in order, and `match` sees each metric's **current** name -- so a block can match the new name produced by an
earlier block's rename.

### Rule fields

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

### Actions

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

## How relabeling runs

The stage order is fixed:

1. The job `selector` filters original scraped series.
2. Job relabeling runs and its result is assembled and checked.
3. Untyped fallback classification and profile selection use the post-job, pre-profile names.
4. Selected profile relabeling runs and its result is assembled and checked independently.
5. Final size/writer checks run, then charts see the normalized result.

Inside either relabeling stage:

1. For each metric, every block whose `match` matches the metric's **current** name runs, in order.
2. Within a block, rules run in order. Each rule joins `source_labels` into an input string and applies its `action`.
3. A rule can rename the metric (write `__name__`), so a later block or rule sees the new name.
4. If any rule drops the metric, the remaining rules and blocks are skipped for it.

### Histogram and summary safety

A histogram or summary is assembled from several series -- `_bucket`/`_sum`/`_count`, or per-quantile series. Netdata
will **not** let relabeling silently corrupt one. A rule that would:

- split a histogram/summary across multiple metric names,
- drop only some of its components,
- change its `le` or `quantile` label,
- merge two metric families together, or
- duplicate a component,

is rejected independently at each stage. The job **fails at autodetection**, so a bad rule is caught immediately; if
the exposition changes at runtime, the affected metric family is **dropped** (rather than charted with wrong values)
and the rest of the job keeps working. A later profile stage cannot hide corruption introduced by the job stage.

This also covers the common Prometheus technique of thinning histogram buckets: a `keep`/`drop` rule on the `le` (or
`quantile`) label is treated as corrupting the metric family, never as thinning it -- the job fails its check when the
check scrape shows the family partially dropped or mutated, and the whole metric family is dropped if the exposition
drifts into corruption later at runtime. Reduce histogram cardinality at the exporter or with the job's `selector`
option instead.

Plain gauges and counters have no such restriction -- you can rename, relabel, split, or drop them freely.

### Profile precedence on the shared stream

Selected profiles share one final metric stream; the collector does not copy source metrics per profile. For each
original source family, it chooses the first selected profile whose root `match` covers the base family and whose
relabeling block matcher covers at least one original physical series. That profile's complete pipeline receives every
series in the family. Later profile pipelines do not process the family and do not see names produced by the first one.

Normalizer precedence is profile-name order in `auto`, configured entry order in `exact`, and configured entries first
followed by the remaining auto-selected profiles in profile-name order in `combined`. Block and rule order inside the
chosen profile remains the YAML order described above.

Dispatch does not predict rule results or output names. The root `match` constrains the original source family only, and
the chosen pipeline may produce a different namespace. Every selected chart template sees those final names and labels,
so profile authors must account for interactions between profiles selected for the same endpoint. See the profile
format's [`relabeling` section](/src/go/plugin/go.d/collector/prometheus/profile-format.md#relabeling) for authoring
guidance. A family renamed outside the profile's root `match` also leaves that profile's `autogen.selector` scope, so an
uncovered output keeps generic autogen unless another applicable profile scope rejects it.

Untyped fallback type is bound from the post-job, pre-profile name using declared, job, selected-profile, and implicit
`_total` precedence. Profile relabeling preserves that decision but cannot create one by adding `_total` or renaming a
final metric into a job/profile `fallback_type` pattern. See the profile format's
[`fallback_type` section](/src/go/plugin/go.d/collector/prometheus/profile-format.md#fallback_type) for the full order.

## Examples

### Drop metrics you don't want

Drop everything named `go_*`:

```yaml
relabeling:
  - match: 'go_*'
    metric_relabel_configs:
      - action: drop
```

`match: 'go_*'` already selects the metrics, so the rule needs no `regex` -- it drops every metric the block sees.

### Keep only specific metrics

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

### Add a derived label

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

### Rename a metric

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

### Remove a high-cardinality label

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

### Keep only a few labels

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

### Normalize label case

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

### Prefix label names (replaces `label_prefix`)

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

### Scope rules to a subset of metrics

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
