# AWS CloudWatch profile format

CloudWatch profiles define which metrics the collector queries and how those
metrics become Netdata charts. Each profile covers one CloudWatch namespace and
one exact resource-dimension grain.

Stock profiles are installed under
`/usr/lib/netdata/conf.d/go.d/cloudwatch.profiles/default/`. To customize one,
copy it to `/etc/netdata/go.d/cloudwatch.profiles/` and keep the same filename.
A user profile fully replaces the stock profile with that basename. Restart the
go.d process after changing profiles because the catalog is cached process-wide.

## Complete example

```yaml
version: v1
display_name: AWS Example Service
namespace: AWS/Example
supported_regions: [us-east-1]  # optional; omit when unrestricted
disabled: true                  # optional; exclude from the default-enabled set
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

The filename without `.yaml` is the profile name. In the example, a file named
`example.yaml` is selected as `example`, exports series beginning with
`example.`, and contributes chart contexts under `cloudwatch.example.*`.

## Top-level fields

| Field | Required | Description |
|:------|:--------:|:------------|
| `version` | yes | Profile schema version. The supported value is `v1`. |
| `display_name` | yes | Human-readable service name. |
| `namespace` | yes | CloudWatch namespace, such as `AWS/EC2`. |
| `supported_regions` | no | Non-empty list of canonical lowercase region codes where this namespace publishes metrics. Omit for unrestricted regional services. |
| `disabled` | no | When `true`, collection rules do not select the profile through their default set. A rule can still name it explicitly in `profiles.include`. |
| `query` | yes | Profile query defaults. `query.period` is required; `lookback` and `publication_delay` are optional. |
| `instance` | yes | Exact dimension set that identifies one collected resource. |
| `metrics` | yes | Metrics and statistics queried for every discovered instance. |
| `template` | yes | Dynamic chart template populated from the exported series. |

### Supported regions

Use `supported_regions` only when AWS publishes a service's CloudWatch metrics
in a fixed subset of regions. CloudFront, for example, is a global service whose
metrics are published in `us-east-1`:

```yaml
supported_regions: [us-east-1]
```

The collection-rule compiler intersects `rules[].regions` with this list:

- A default-selected profile with no intersection is skipped and reported once
  during startup.
- A profile explicitly named in `profiles.include` with no intersection is a
  configuration error.
- An omitted `supported_regions` field means that every rule region is allowed.

An explicitly empty list is invalid; omit the field for unrestricted profiles.

## Query defaults

`query` defines the profile's CloudWatch timing defaults:

| Field | Required | Description |
|:------|:--------:|:------------|
| `period` | yes at profile level | CloudWatch aggregation period. It must be from `1m` through `24h` and an exact multiple of one minute. |
| `lookback` | no | Rolling retrieval horizon. It must be at least one period, an exact multiple of the effective period, and no more than 1,440 buckets. Omission follows the resolved period. |
| `publication_delay` | no | How long the collector waits after a bucket closes before querying it. This is collector scheduling policy, not an AWS publication SLA. Omission uses the collector's built-in `10m` fallback. Zero is valid. |

Durations accept duration strings or numeric seconds. Stock profiles use
canonical strings such as `1m`, `10m`, and `24h`; custom profiles should do the
same for readability.

Every metric may define an optional `query` object with the same fields. Job
configuration resolves each field independently in this order, from highest to
lowest precedence:

1. `rules[].query`
2. `rule_defaults.query`
3. `metrics[].query`
4. profile `query`

After inheritance, the collector validates the complete policy. The combined
`publication_delay + lookback + period` horizon must not exceed 14 days.

## Instance dimensions

`instance.dimensions` is the exact CloudWatch dimension-name set accepted during
discovery. Metrics with missing or extra dimensions do not match. This prevents
different CloudWatch aggregation grains from collapsing into one resource.

Each dimension sets exactly one of:

- `label`: emit the dimension value as a Netdata identity label. Labels use
  lowercase letters, digits, and underscores, start with a letter, and cannot be
  `account_id` or `region`.
- `constant`: require an exact CloudWatch dimension value, include it in the
  query, but do not emit it as a label. Use this for a fixed qualifier such as
  CloudFront's `Region=Global`.

Dimension names and constant values are matched verbatim and must not have
leading or trailing whitespace. Names and emitted labels must be unique within
the profile.

When every declared dimension is constant, the collector already knows the
complete instance and queries it directly. Such profiles do not call
`ListMetrics`. Profiles with at least one identifying dimension use discovery
normally.

## Metrics

Every `metrics` entry supports:

| Field | Required | Description |
|:------|:--------:|:------------|
| `id` | yes | Stable lowercase identifier used in exported series names and chart selectors. |
| `metric_name` | yes | CloudWatch metric name, matched verbatim. |
| `statistics` | yes | One or more of `average`, `minimum`, `maximum`, `sum`, `sample_count`, or a percentile such as `p90` or `p99.9`. |
| `rate` | no | Divide a per-period `sum` or `sample_count` by the effective period and present it per second. Requires one of those total statistics. |
| `query` | no | Metric-specific `period`, `lookback`, and/or `publication_delay` overrides. Omitted fields inherit independently. |
| `nil_as_zero` | no | Record a clean no-datapoint response as `0` (`true`) or a gap (`false`). When omitted, rate totals default to zero and other statistics default to gaps. |

Metric IDs and CloudWatch metric names must be unique within a profile. The
exported series name is:

```text
<profile>.<metric_id>_<normalized_statistic>
```

Chart selectors may use the shorthand `<metric_id>_<statistic>` shown in the
example; the loader qualifies it with the profile name.

## Chart template rules

The `template` uses Netdata's dynamic chart-template format. See the complete
[Chart Template Format](/src/go/plugin/framework/charttpl/README.md) for every
group, chart, instance, label-promotion, lifecycle, dimension, presentation,
and selector field. CloudWatch profiles add these restrictions:

- Every chart must set `algorithm: absolute`. CloudWatch returns per-period
  aggregates, not cumulative counters.
- Every chart's effective `instances.by_labels` must include `account_id`,
  `region`, and every identifying dimension `label`. Constant dimensions are not
  included.
- Do not author `template.metrics`; the collector derives the visible series
  from `metrics`.
- Every dimension selector must resolve to a series exported by the profile.
- Chart IDs must be unique across the complete loaded profile catalog.

The collector divides rate totals by the effective query period before emission
and marks all emitted CloudWatch metric families as floating-point values.

## Authoring workflow

1. Inspect the service's CloudWatch namespace, metric names, and exact dimension
   sets with `aws cloudwatch list-metrics`.
2. Start from the closest stock profile and choose one resource grain.
3. Keep the default metric set focused; CloudWatch bills `GetMetricData` per
   requested metric. Leave expensive or high-cardinality additions commented
   with matching chart definitions when appropriate.
4. Run the collector tests and profile-catalog validation:

   ```bash
   cd src/go
   go test -count=1 ./plugin/go.d/collector/cloudwatch/...
   ```

5. Restart go.d and verify discovery, exported labels, chart identity, gaps, and
   the expected CloudWatch API volume in a test account.
