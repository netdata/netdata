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
period: 300

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
    period: 60                  # optional metric-level override
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
| `supported_regions` | no | Non-empty list of regions where this namespace publishes metrics. Omit for unrestricted regional services. |
| `disabled` | no | When `true`, collection rules do not select the profile through their default set. A rule can still name it explicitly in `profiles.include`. |
| `period` | yes | Default CloudWatch period in seconds. It must be a positive multiple of 60, up to 86400. |
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

## Metrics

Every `metrics` entry supports:

| Field | Required | Description |
|:------|:--------:|:------------|
| `id` | yes | Stable lowercase identifier used in exported series names and chart selectors. |
| `metric_name` | yes | CloudWatch metric name, matched verbatim. |
| `statistics` | yes | One or more of `average`, `minimum`, `maximum`, `sum`, `sample_count`, or a percentile such as `p90` or `p99.9`. |
| `rate` | no | Divide a per-period `sum` or `sample_count` by the effective period and present it per second. Requires one of those total statistics. |
| `period` | no | Metric-specific period override, with the same validation as the profile period. |
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

The collector injects the period divisor for rate metrics and marks all emitted
CloudWatch metric families as floating-point values.

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
