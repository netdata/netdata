# Metric relabeling

Relabeling rewrites, adds, drops, or filters a scraped metric — its name and its labels — **before** Netdata builds
charts from it. The rule shape is Prometheus-compatible: it mirrors Prometheus
[`metric_relabel_configs`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs),
so rules you already use in Prometheus carry over directly.

Use it to keep noisy metrics out of your dashboards, fold high-cardinality labels away, derive new labels, rename
metrics, or normalize label casing — without touching the application you are scraping.

> **Note**
> Relabeling replaces the removed `label_prefix` option. If you used `label_prefix` to prefix label keys, use a
> `labelmap` rule instead (see the [example](#prefix-label-names-replaces-label_prefix)).

## How a rule reads

Each rule transforms one scraped metric in a few steps:

1. **Join** the values of the metric's `source_labels` (with `separator`) into one input string. The metric's own name
   is available as a special label, `__name__`.
2. **Match** that input against `regex`. The `regex` is a regular expression that must match the **whole** input, not a
   substring (wrap it in `.*` to match a part).
3. **Act**: the `action` decides what happens — set or rename a label, drop or keep the metric, and so on. If you omit
   `action`, it defaults to `replace`.
4. **Write** the result into `target_label` (use `__name__` to rename the metric), expanding any `regex` capture groups
   in `replacement` (`$1`, or `${name}`).

Two different matchers are at play — don't confuse them: a block's `match` is a **glob** (simple pattern) over the
metric name, while a rule's `regex` is an **anchored regular expression** over the joined label values.

## Quick start

Add a `relabeling` block to a job. This drops the `go_gc_duration_seconds` metric and derives an HTTP `code_class`
label (`2xx`, `4xx`, …) on every `http_*` metric:

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

## Configuration

`relabeling` is a **list of blocks**. Each block has a `match` and a list of rules; the rules apply only to metrics
whose name matches the block's `match`. Grouping rules under a block lets you scope a set of rules to a subset of
metrics by name, instead of repeating a name match in every rule. Relabeling is opt-in — jobs without a `relabeling`
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

### `match`

`match` selects which metrics a block's rules apply to. It is **required**.

- Syntax is a Netdata [simple pattern](/src/libnetdata/simple_pattern/README.md): a space-separated list of globs where
  `*` matches any sequence, `?` matches any single character, and a leading `!` negates a term.
- It is matched against the **full metric name, including** the `_bucket`/`_sum`/`_count` suffixes of histograms and
  summaries. Prefer a glob like `app_lat*` over an exact `app_lat`, otherwise the histogram's `_bucket`/`_sum`/`_count`
  series will not match.
- Use `*` to target **every** metric.

Blocks run in order, and `match` sees each metric's **current** name — so a block can match the new name produced by an
earlier block's rename.

### Rule fields

Inside a block, `metric_relabel_configs` is a list of rules applied **in order** (see [How a rule reads](#how-a-rule-reads)).
Every field is optional except where an action requires it; `action` defaults to `replace`.

| Field           | Description                                                                                                                                                                | Default |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| `source_labels` | Label names whose values are joined (with `separator`) into the rule's input string. Use `__name__` for the metric name.                                                  | —       |
| `separator`     | String placed between joined `source_labels` values.                                                                                                                       | `;`     |
| `regex`         | Regular expression matched against the joined input. It is **fully anchored** (implicitly `^…$`); to match a substring, wrap it in `.*`.                                    | `(.*)`  |
| `modulus`       | Positive integer, used only by the `hashmod` action.                                                                                                                       | —       |
| `target_label`  | The label the action writes. Use `__name__` to write the metric name.                                                                                                      | —       |
| `replacement`   | The value written, with regex capture-group expansion (`$1`, or `${name}` for named groups).                                                                                | `$1`    |
| `action`        | One of the [actions](#actions) below.                                                                                                                                       | `replace` |

### Actions

| Action      | What it does                                                                                                                                                                                   |
|-------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `replace`   | If `regex` matches the joined input, set `target_label` to the expanded `replacement` (an empty result removes the label); a non-match leaves the label unchanged. See [example](#add-a-derived-label). |
| `keep`      | Keep the metric only if `regex` matches the joined input; otherwise drop it. See [example](#keep-only-specific-metrics).                                                                       |
| `drop`      | Drop the metric if `regex` matches the joined input. See [example](#drop-metrics-you-dont-want).                                                                                               |
| `keepequal` | Keep the metric only if `target_label`'s value equals the joined input.                                                                                                                       |
| `dropequal` | Drop the metric if `target_label`'s value equals the joined input.                                                                                                                            |
| `hashmod`   | Set `target_label` to `md5(joined input) mod modulus` — useful for sharding.                                                                                                                  |
| `labelmap`  | For every label whose **name** matches `regex`, copy its value to a new label named by `replacement`. See [example](#prefix-label-names-replaces-label_prefix).                                |
| `labeldrop` | Remove every label whose name matches `regex`. See [example](#remove-a-high-cardinality-label).                                                                                               |
| `labelkeep` | Remove every label whose name does **not** match `regex`. See [example](#keep-only-a-few-labels).                                                                                             |
| `lowercase` | Set `target_label` to the lowercased joined input. See [example](#normalize-label-case).                                                                                                      |
| `uppercase` | Set `target_label` to the uppercased joined input. See [example](#normalize-label-case).                                                                                                      |

> The label-name actions `labelmap`, `labeldrop`, and `labelkeep` act on labels only — they never touch the metric name
> (`__name__`). To rename a metric, use a `replace` rule with `target_label: __name__`.

## How it works

1. For each scraped metric, every block whose `match` matches the metric's **current** name runs, in order.
2. Within a block, rules run in order. Each rule joins `source_labels` into an input string and applies its `action`.
3. A rule can rename the metric (write `__name__`), so a later block or rule sees the new name.
4. If any rule drops the metric, the remaining rules and blocks are skipped for it.

### Histogram and summary safety

A histogram or summary is assembled from several series — `_bucket`/`_sum`/`_count`, or per-quantile series. Netdata
will **not** let relabeling silently corrupt one. A rule that would:

- split a histogram/summary across multiple metric names,
- drop only some of its components,
- change its `le` or `quantile` label,
- merge two families together, or
- duplicate a component,

is rejected. The job **fails at autodetection**, so a bad rule is caught immediately; if the exposition changes at
runtime, the affected family is **dropped** (rather than charted with wrong values) and the rest of the job keeps
working.

Plain gauges and counters have no such restriction — you can rename, relabel, split, or drop them freely.

## Examples

### Drop metrics you don't want

Drop everything named `go_*`:

```yaml
relabeling:
  - match: 'go_*'
    metric_relabel_configs:
      - action: drop
```

`match: 'go_*'` already selects the metrics, so the rule needs no `regex` — it drops every metric the block sees.

### Keep only specific metrics

Within a family, drop everything except a chosen series by matching a label. This keeps only `node_systemd_unit_state`
samples whose `state` is `active`, dropping the rest:

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

Derive an HTTP `code_class` (`2xx`, `4xx`, …) from a numeric `code` label:

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
components are renamed together — see [Histogram and summary safety](#histogram-and-summary-safety).)

### Remove a high-cardinality label

Drop a label that explodes cardinality and clutters charts:

```yaml
relabeling:
  - match: '*'
    metric_relabel_configs:
      - regex: 'pod_uid'
        action: labeldrop
```

Every `pod_uid` label is removed from all metrics. To drop several, use an alternation like `regex: 'pod_uid|instance_id'`.

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

Use `uppercase` for the opposite. These actions take only `source_labels` and `target_label` (often the same label).

### Prefix label names (replaces `label_prefix`)

Copy every label to a prefixed name using `labelmap`:

```yaml
relabeling:
  - match: '*'
    metric_relabel_configs:
      - regex: '(.*)'
        action: labelmap
        replacement: 'app_$1'
```

A label `region` becomes `app_region`; the metric name (`__name__`) is left untouched. `labelmap` matches against label
**names** and writes the new name from `replacement`; the original labels remain, so add a [`labeldrop`](#remove-a-high-cardinality-label)
rule afterwards if you want them removed.

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
