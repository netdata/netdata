# AWS CloudWatch profile format

CloudWatch profiles define which metrics the collector queries and how those metrics become Netdata charts. Each profile
covers one CloudWatch namespace and one exact resource-dimension grain.

| Location                                                    | Purpose                             |
|:------------------------------------------------------------|:------------------------------------|
| `/usr/lib/netdata/conf.d/go.d/cloudwatch.profiles/default/` | Stock profiles shipped with Netdata |
| `/etc/netdata/go.d/cloudwatch.profiles/`                    | User profiles                       |

To customize a stock profile, copy it to the user directory and keep the same filename -- a user profile fully replaces
the stock profile with that basename. Restart the go.d process after changing profiles because the catalog is cached
process-wide.

## How profiles work

A profile is a YAML file with five parts: identity fields (`version`, `display_name`, `namespace`), `query` timing
defaults, an `instance` dimension set, `metrics`, and a chart `template`. At runtime the collector uses them in four
steps:

1. **Selection** -- collection rules in `go.d/cloudwatch.conf` choose profiles. A rule that omits `profiles` selects
   every default-enabled profile; a profile marked `disabled: true` must be named explicitly in `profiles.include`.
2. **Discovery** -- the collector scans the profile's namespace with `ListMetrics` and keeps only metrics whose
   dimension names match `instance.dimensions` exactly. Each distinct combination of dimension values becomes one
   monitored instance. A profile whose dimensions are all constants skips discovery entirely: the instance is known
   statically.
3. **Query** -- every enabled metric/statistic pair is fetched with `GetMetricData`, using timing resolved from the job
   configuration and the profile's `query` defaults.
4. **Charts** -- the exported series feed the `template`, which renders one chart instance per monitored resource.

Naming flows from the filename. A file named `example.yaml` is selected as `example` and exports series beginning with
`example.`. Chart contexts come from the template: the final context is
`cloudwatch.<template context_namespace>.<chart context>` -- in the example below, `cloudwatch.example.requests`. Stock
profiles set `context_namespace` to the profile name so series and contexts stay aligned; follow that convention.

Job configuration can reshape a profile without editing it: rules select or drop metrics (`rules[].metrics`), change
statistics, and override query timing. Edit or author profile YAML only for new metrics, dimensions, or charts.

> **The loader ignores unknown YAML keys.** A misspelled field name does not produce an error -- it silently does
> nothing. When a profile change has no effect, re-check the field spelling first.

## Complete example

```yaml
version: v1
display_name: AWS Example Service
namespace: AWS/Example
supported_regions: [us-east-1]  # optional; omit when unrestricted
disabled: true                  # optional; demonstrates an opt-in profile -- omit for a normal one
query:
  period: 5m                    # required profile default
  lookback: 15m                 # optional retrieval horizon
  publication_delay: 10m        # optional settling delay

instance:
  dimensions:
    - name: ResourceId
      label: resource_id
    - name: Scope
      constant: Global

metrics:
  - id: requests
    metric_name: RequestCount
    statistics: [sum]
    rate: true
  - id: latency
    disabled: true               # opt-in; still declared and charted
    metric_name: Latency
    statistics: [average, p99]
    query:                      # optional field-by-field overrides
      period: 1m
      lookback: 5m
    nil_as_zero: false          # optional no-datapoint policy

template:
  family: Example Service
  context_namespace: example
  chart_defaults:
    instances:
      by_labels: [account_id, region, resource_id]
  charts:
    - id: aws_cloudwatch_example_requests
      context: requests
      title: Example Requests
      family: Traffic
      units: requests/s
      algorithm: absolute
      dimensions:
        - selector: requests_sum
          name: requests
    - id: aws_cloudwatch_example_latency
      context: latency
      title: Example Latency
      family: Latency
      units: milliseconds
      algorithm: absolute
      dimensions:
        - selector: latency_average
          name: average
        - selector: latency_p99
          name: p99
```

## Authoring workflow

1. Inspect the service's CloudWatch namespace, metric names, and exact dimension sets:

   ```bash
   aws cloudwatch list-metrics --namespace "AWS/Example" --region us-east-1 --output json \
     | jq -c '[.Metrics[] | {metric: .MetricName, dimensions: ([.Dimensions[].Name] | sort)}] | unique'
   ```

   ```json
   [
     {"metric":"RequestCount","dimensions":["ResourceId","Scope"]},
     {"metric":"Latency","dimensions":["ResourceId","Scope"]},
     {"metric":"RequestCount","dimensions":["Scope"]}
   ]
   ```

   Map the output onto profile fields:

   | `list-metrics` output      | Profile field                                                                                                                        |
   |:---------------------------|:-------------------------------------------------------------------------------------------------------------------------------------|
   | `Namespace`                | `namespace`                                                                                                                          |
   | one exact `dimensions` set | `instance.dimensions`. Pick ONE set: `["ResourceId","Scope"]` and `["Scope"]` above are different grains and need separate profiles. |
   | each `Dimensions[].Name`   | `instance.dimensions[].name`. Identifying values become `label`; fixed qualifiers become `constant`.                                 |
   | `MetricName`               | `metrics[].metric_name`                                                                                                              |

   `list-metrics` does not report statistics -- choose them from the AWS service's metric documentation (typically `sum`
   for counters, `average` or percentiles for latencies).

2. Start from the closest stock profile and choose one resource grain.
3. Keep the default metric set focused; CloudWatch bills `GetMetricData` per requested metric. Declare expensive or
   specialized additions with `disabled: true`, and keep their matching chart definitions active so an operator can
   enable them only through job configuration.
4. Run the collector tests and profile-catalog validation:

   ```bash
   cd src/go
   go test -count=1 ./plugin/go.d/collector/cloudwatch/...
   ```

5. Restart go.d and verify discovery, exported labels, chart identity, gaps, and the expected CloudWatch API volume in a
   test account.

The sections below are the field-by-field reference.

## Top-level fields

| Field               | Required | Description                                                                                                                                            |
|:--------------------|:--------:|:-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `version`           |   yes    | Profile schema version. The supported value is `v1`.                                                                                                   |
| `display_name`      |   yes    | Human-readable service name.                                                                                                                           |
| `namespace`         |   yes    | CloudWatch namespace, such as `AWS/EC2`. Allowed characters: letters, digits, `/`, `.`, `_`, `-`; matched verbatim, no leading or trailing whitespace. |
| `supported_regions` |    no    | Non-empty list of regions where this namespace publishes metrics. Omit for unrestricted regional services.                                             |
| `disabled`          |    no    | When `true`, collection rules do not select the profile through their default set. A rule can still name it explicitly in `profiles.include`.          |
| `query`             |   yes    | Profile query defaults. `query.period` is required; `lookback` and `publication_delay` are optional.                                                   |
| `instance`          |   yes    | Exact dimension set that identifies one collected resource. At least one dimension.                                                                    |
| `metrics`           |   yes    | Default-enabled and opt-in metrics available for every discovered instance. At least one metric.                                                       |
| `template`          |   yes    | Dynamic chart template populated from the exported series. At least one chart.                                                                         |

The filename must match `^[a-z][a-z0-9_]*$` (plus the `.yaml` or `.yml` extension): lowercase, starting with a letter,
using only letters, digits, and underscores. The basename is the profile name used everywhere else.

### Supported regions

Use `supported_regions` only when AWS publishes a service's CloudWatch metrics in a fixed subset of regions. CloudFront,
for example, is a global service whose metrics are published in `us-east-1`:

```yaml
supported_regions: [us-east-1]
```

Each entry must be a valid canonical lowercase AWS region code, with no duplicates. The collection-rule compiler
intersects `rules[].regions` with this list:

- A default-selected profile with no intersection is skipped and reported once during startup.
- A profile explicitly named in `profiles.include` with no intersection is a configuration error.
- An omitted `supported_regions` field means that every rule region is allowed.

An explicitly empty list is invalid; omit the field for unrestricted profiles.

## Query defaults

`query` defines the profile's CloudWatch timing defaults:

| Field               |       Required       | Description                                                                                                                                                                           |
|:--------------------|:--------------------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `period`            | yes at profile level | CloudWatch aggregation period. It must be from `1m` through `24h` and an exact multiple of `1m`.                                                                                      |
| `lookback`          |          no          | Rolling retrieval horizon. It must be positive, at least one period, an exact multiple of the effective period, and no more than 1,440 buckets. Omission follows the resolved period. |
| `publication_delay` |          no          | Delay before querying a closed bucket -- collector scheduling policy, not an AWS publication SLA. Default `10m`; `0` is valid.                                                        |

Durations accept duration strings or numeric seconds and must resolve to whole seconds. Stock profiles use canonical
strings such as `1m`, `10m`, and `24h`; custom profiles should do the same for readability.

Every metric may define an optional `query` object with the same fields. Job configuration resolves each field
independently in this order, from highest to lowest precedence:

1. `rules[].query` (job configuration)
2. `rule_defaults.query` (job configuration)
3. `metrics[].query` (profile)
4. profile `query`

After inheritance, the collector validates the complete policy. The combined `publication_delay + lookback + period`
horizon must not exceed 14 days.

## Instance dimensions

`instance.dimensions` is the exact CloudWatch dimension-name set accepted during discovery. Metrics with missing or
extra dimensions do not match. This prevents different CloudWatch aggregation grains from collapsing into one resource.

Each dimension sets exactly one of:

- `label`: emit the dimension value as a Netdata identity label. Labels use lowercase letters, digits, and underscores,
  start with a letter, and cannot be `account_id` or `region` -- the collector attaches those two identity labels to
  every instance automatically.
- `constant`: require an exact CloudWatch dimension value, include it in the query, but do not emit it as a label. Use
  this for a fixed qualifier such as CloudFront's `Region=Global`.

Dimension names and constant values are matched verbatim and must not have leading or trailing whitespace. Names and
emitted labels must be unique within the profile.

When every declared dimension is constant, the collector already knows the complete instance and queries it directly.
Such profiles do not call `ListMetrics`. Profiles with at least one identifying dimension use discovery normally.

### One profile per grain

When AWS publishes the same MetricNames at multiple exact dimension sets, write one profile per grain. Use separate
profile and context names so the identities cannot merge, and mark a higher-cardinality grain `disabled: true` when it
should be opt-in.

Stock examples:

- `privatelink_endpoint.yaml` and `privatelink_endpoint_subnet.yaml` are the parent/child pair: the child adds
  `Subnet Id`, while the collector's reviewed tag-association registry joins both profiles on their parent
  `VPC Endpoint Id`.
- The five `privatelink_service*.yaml` profiles are the multi-child example: all detailed Availability Zone,
  load-balancer, and consumer-endpoint grains remain separate, but every one joins tags through the parent `Service Id`
  and `ec2:vpc-endpoint-service` resource.

## Metrics

Every `metrics` entry supports:

| Field         | Required | Description                                                                                                                                                                                                                                          |
|:--------------|:--------:|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `id`          |   yes    | Stable lowercase identifier used in exported series names and chart selectors.                                                                                                                                                                       |
| `disabled`    |    no    | When `true`, the metric is opt-in: not queried by default, but still available to `rules[].metrics`, with its charts staying in the template.                                                                                                        |
| `metric_name` |   yes    | CloudWatch metric name, matched verbatim; no leading or trailing whitespace.                                                                                                                                                                         |
| `statistics`  |   yes    | One or more of `average`, `minimum`, `maximum`, `sum`, `sample_count`, or a percentile. Case-insensitive; duplicates within one metric are rejected.                                                                                                 |
| `rate`        |    no    | Divide each per-period `sum` or `sample_count` sibling by the effective period and present it per second. Other statistics on the same metric, such as `average`, remain unchanged. Requires at least one total statistic (`sum` or `sample_count`). |
| `query`       |    no    | Metric-specific `period`, `lookback`, and/or `publication_delay` overrides. Omitted fields inherit independently.                                                                                                                                    |
| `nil_as_zero` |    no    | Map a clean no-datapoint response to `0` (`true`) or a gap (`false`). Default: rate totals map to `0`, all other statistics to a gap.                                                                                                                |

**Percentiles** range from `p0` through `p100` with at most two decimal places (`p90`, `p99.9`, `p99.99`). **Metric
IDs** and CloudWatch metric names must be unique within a profile.

The exported series name is:

```text
<profile>.<metric_id>_<normalized_statistic>
```

Chart selectors may use the shorthand `<metric_id>_<statistic>` shown in the example; the loader qualifies it with the
profile name.

### How rules select profile metrics

Omitting `rules[].metrics` collects the default-enabled metrics from every selected profile. A metric group changes only
its named profile; selected profiles without a group keep their defaults. The group includes defaults when `defaults` is
omitted or `true`, so adding an opt-in metric is concise:

```yaml
profiles:
  include: [example]  # required because this example profile is disabled
metrics:
  - profile: example
    include:
      - name: Latency
```

Set `defaults: false` when the profile must use only the listed MetricNames. If both the group and metric omit
`statistics`, the collector uses every statistic declared for that metric in the profile.

## Chart template rules

The `template` uses Netdata's dynamic chart-template format. See the complete
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md) for every group, chart, instance, label-promotion,
lifecycle, dimension, presentation, and selector field. CloudWatch profiles add these restrictions:

- Set `context_namespace` at the template root to the profile name; the emitted context is
  `cloudwatch.<context_namespace>.<chart context>`.
- Every chart must set `algorithm: absolute`. CloudWatch returns per-period aggregates, not cumulative counters.
- Every chart's effective `instances.by_labels` must include `account_id`, `region`, and every identifying dimension
  `label`. Constant dimensions are not included. A `*` wildcard entry satisfies the requirement; a `!label` entry
  excludes that label, but excluding a required identity label fails validation.
- Do not author `template.metrics`; the collector derives the visible series from `metrics`.
- Every dimension selector must resolve to a series exported by the profile.
- Set an explicit `id` on every chart and keep it unique across the loaded profile catalog. A collision between two
  stock profiles fails loading; a collision involving a user profile logs a warning and drops the colliding chart. The
  uniqueness check covers only explicitly set IDs.

Unlike charttpl's strict standalone decoding, a CloudWatch profile's template is validated structurally but unknown keys
are ignored (see "How profiles work").

The collector divides rate totals by the effective query period before emission and marks all emitted CloudWatch metric
families as floating-point values.

