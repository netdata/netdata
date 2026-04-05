# Chart Template Format

## Overview

A **chart template** defines _how a collector's metrics are organized into charts_ in the Netdata dashboard.

> [!NOTE]
> Chart templates are declarative YAML files. You describe **what** to chart, and the engine handles creating, updating, and removing chart instances at runtime.

It tells the chart engine:

- which **metrics** to include
- how to **group** them into charts and families
- how to **name** and **scale** dimensions
- how to create **per-instance charts** (e.g., one chart per host, per disk, per user)

Each collector has a single `charts.yaml` file that describes all its charts.

> [!TIP]
> Groups can be nested to **any depth**. Family paths, context namespaces, and metric scopes compose automatically as you nest — no need to repeat prefixes. See [groups](#4-groups) for the full composition rules and examples.

### How Chart Templates Work

When a collector runs, the chart engine:

1. Reads the collector's `charts.yaml` file.
2. Compiles it into an immutable program (validates, resolves defaults, infers algorithms).
3. On each collection cycle, matches incoming metrics against dimension selectors.
4. Creates chart instances dynamically based on instance identity labels.
5. Updates dimension values every cycle; removes stale instances based on lifecycle policy.

**Template Lifecycle**

```text
                     charts.yaml
                         |
                         v
              ┌─────────────────────┐
              │  Decode & Validate  │  strict YAML parse + semantic checks
              └──────────┬──────────┘
                         v
              ┌─────────────────────┐
              │  Compile (engine)   │  selector parsing, algorithm inference,
              │                     │  context/family/ID composition
              └──────────┬──────────┘
                         v
              ┌─────────────────────┐
              │  Runtime (per cycle)│  match series → create/update/remove
              │                     │  charts and dimensions
              └─────────────────────┘
```

### Example: Complete Chart Template

The example below shows a single template that covers all common metric kinds:
gauge, counter, histogram, summary, and stateset. Inline comments explain each field.

```yaml
version: v1                           # schema version (only "v1" supported)
context_namespace: myapp              # prefix for all chart contexts → myapp.<group context>.<chart context>

groups:
  # ── Gauge: point-in-time values (algorithm: absolute) ──────────────
  - family: Resources
    metrics: # metrics visible to dimension selectors in this group
      - memory_used_bytes
      - memory_total_bytes
      - cpu_usage_percent
    charts:
      - id: memory_usage
        title: Memory Usage
        context: memory_usage         # final context: myapp.memory_usage
        units: bytes
        type: stacked                 # line (default), area, stacked, heatmap
        algorithm: absolute           # absolute = raw value; incremental = rate (value - prev) / interval
        instances:
          by_labels: [host]           # one chart per unique "host" label value
        dimensions:
          - selector: memory_used_bytes
            name: used                # static dimension name
          - selector: memory_total_bytes
            name: total
            options:
              hidden: true            # collected but not drawn (useful for % calculations)

      - id: cpu_usage
        title: CPU Usage
        context: cpu_usage
        units: percentage
        instances:
          by_labels: [host]
        dimensions:
          - selector: cpu_usage_percent
            name: used
            options:
              divisor: 100            # raw value in basis points → divide by 100 for percent
              float: true             # use floating-point precision

  # ── Counter: monotonically increasing values (algorithm: incremental) ──
  - family: Traffic
    metrics:
      - http_requests_total
      - bytes_received
      - bytes_sent
    charts:
      - id: http_requests
        title: HTTP Requests
        context: http_requests
        units: requests/s
        algorithm: incremental        # engine computes rate: (current - previous) / interval
        instances:
          by_labels: [host]
        dimensions:
          - selector: http_requests_total
            name_from_label: method   # dynamic name: each unique label value becomes a dimension
            # e.g., method="GET" → dim "GET", method="POST" → dim "POST"

      - id: bandwidth
        title: Network Bandwidth
        context: bandwidth
        units: kilobits/s
        type: area
        algorithm: incremental
        instances:
          by_labels: [host]
        dimensions:
          - selector: bytes_received
            name: in
            options:
              multiplier: 8           # bytes → bits
              divisor: 1000           # bits → kilobits
          - selector: bytes_sent
            name: out
            options:
              multiplier: -8          # negative = drawn below zero line (bidirectional chart)
              divisor: 1000

  # ── Histogram: bucketed distribution (flattened into _bucket, _count, _sum) ──
  - family: Latency
    metrics:
      - request_duration_seconds_bucket
      - request_duration_seconds_count
      - request_duration_seconds_sum
    charts:
      - id: request_duration_buckets
        title: Request Duration Buckets
        context: request_duration_buckets
        units: observations/s
        type: stacked
        algorithm: incremental        # histogram buckets are counters
        instances:
          by_labels: [host]
        dimensions:
          - selector: request_duration_seconds_bucket
            # no name, no name_from_label → engine infers dimension names
            # from the "le" (less-than-or-equal) label automatically:
            # le="0.005" → dim "0.005", le="0.01" → dim "0.01", etc.

      - id: request_rate
        title: Request Rate
        context: request_rate
        units: requests/s
        algorithm: incremental
        instances:
          by_labels: [host]
        dimensions:
          - selector: request_duration_seconds_count
            name: requests

  # ── Summary: quantile distribution (flattened into quantile values, _count, _sum) ──
  - family: Response Time
    metrics:
      - response_time_seconds
      - response_time_seconds_count
      - response_time_seconds_sum
    charts:
      - id: response_time_quantiles
        title: Response Time Quantiles
        context: response_time_quantiles
        units: seconds
        algorithm: absolute           # quantile values are gauges, not counters
        instances:
          by_labels: [host]
        dimensions:
          - selector: response_time_seconds
            # no name, no name_from_label → engine infers dimension names
            # from the "quantile" label automatically:
            # quantile="0.5" → dim "0.5", quantile="0.99" → dim "0.99", etc.
            options:
              float: true

  # ── StateSet: named boolean states (exactly one active at a time) ──
  - family: Health
    metrics:
      - service_status
    charts:
      - id: service_health
        title: Service Health Status
        context: service_health
        units: state
        instances:
          by_labels: [host]
        dimensions:
          - selector: service_status
            # no name, no name_from_label → engine infers dimension names
            # from the metric-name label (service_status=<value>) automatically:
            # service_status="ready" → dim "ready", service_status="degraded" → dim "degraded", etc.
```

**What this template produces** — if the collector reports metrics for 2 hosts (`host="web-1"`, `host="web-2"`), the engine creates **2 instances of every chart** (one per host). The histogram bucket chart gets one dimension per `le` boundary, the summary chart gets one per quantile, and the stateset chart gets one per state — all named automatically by the engine.

## Template Structure

Every `charts.yaml` follows this structure:

```yaml
version: <schema version>
context_namespace: <context prefix>
engine: <engine policy>
groups:
  - family: <family name>
    context_namespace: <context segment>
    metrics: <available metrics>
    chart_defaults: <inheritable defaults>
    charts: <chart definitions>
    groups: <nested groups>
```

| Section                                       | Purpose                                            |
|-----------------------------------------------|----------------------------------------------------|
| [**version**](#1-version)                     | Schema version (must be `v1`).                     |
| [**context_namespace**](#2-context_namespace) | Top-level prefix for chart context paths.          |
| [**engine**](#3-engine)                       | Engine-level policy (selectors, autogeneration).   |
| [**groups**](#4-groups)                       | Recursive chart groups — the core of the template. |

---

## Field Reference

### 1. version

Schema version. Currently only `v1` is supported. Defaults to `v1` if omitted.

```yaml
version: v1
```

### 2. context_namespace

Top-level prefix for all chart context paths in the template. Combined with group-level `context_namespace` and chart `context` to form the final context.

```yaml
context_namespace: mysql
```

**Context composition** — the final chart context is built by joining all context parts with `.`:

```
<top context_namespace>.<group context_namespace>...<chart.context>
```

For example:

| Level                         | Value               |
|-------------------------------|---------------------|
| Top-level `context_namespace` | `mysql`             |
| Group `context_namespace`     | _(empty)_           |
| Chart `context`               | `queries`           |
| **Resulting context**         | **`mysql.queries`** |

### 3. engine

Template-level policy that controls metric filtering and autogeneration.

```yaml
engine:
  selector:
    allow: ["cpu_*", "memory_*"]
    deny: ["cpu_guest_*"]
  autogen:
    enabled: true
    expire_after_success_cycles: 50
```

| Field                                 | Type          | Default     | Description                                                                            |
|---------------------------------------|---------------|-------------|----------------------------------------------------------------------------------------|
| `selector.allow`                      | array[string] | _(empty)_   | Include only metrics matching these patterns (simple patterns: `*` and `?` wildcards). |
| `selector.deny`                       | array[string] | _(empty)_   | Exclude metrics matching these patterns (simple patterns: `*` and `?` wildcards).      |
| `autogen.enabled`                     | bool          | `false`     | Create charts for metrics not matched by any template dimension.                       |
| `autogen.max_type_id_len`             | int           | `0` (=1200) | Max full `type.id` length. Must be `0` or `>= 4`.                                      |
| `autogen.expire_after_success_cycles` | uint64        | `0`         | Remove autogenerated charts not seen for N successful cycles (`0` = never).            |

**When to use autogen**: For collectors like Nagios plugins where the set of metrics is unpredictable and user-defined. The engine creates a chart for every unmatched metric automatically.

**Example: Nagios collector with autogeneration**

```yaml
version: v1
context_namespace: nagios
engine:
  autogen:
    enabled: true
    expire_after_success_cycles: 50
groups:
  - family: Job
    context_namespace: job
    groups:
      - family: Execution
        metrics:
          - nagios.job.execution_state
          - nagios.job.execution_duration
        charts:
          - id: job_execution_state
            title: Job Execution State
            context: execution_state
            units: state
            instances:
              by_labels: [nagios_job]
            dimensions:
              - selector: nagios.job.execution_state
          - id: job_execution_duration
            title: Execution Duration
            context: execution_duration
            units: seconds
            instances:
              by_labels: [nagios_job]
            dimensions:
              - selector: nagios.job.execution_duration
                name: duration
                options:
                  float: true
```

Explicitly defined charts (like `execution_state`) use the template. Any _other_ metrics the Nagios plugin emits get auto-charted by the engine.

### 4. groups

Groups organize charts into a hierarchy that can be nested to **any depth**. Each group defines a **family** segment, can declare **metrics** in scope, and contains **charts** and/or nested **groups**.

Nesting serves three purposes:

1. **Family composition** — each level's `family` is joined with `/`, producing Netdata's hierarchical family structure automatically (the UI renders `/`-separated families as navigable levels).
2. **Context composition** — each level's `context_namespace` is joined with `.`, so you write short context leaves instead of long prefixed strings.
3. **Metric scoping** — metrics declared in a group are inherited by all descendants, so you declare once at the appropriate level.

```yaml
groups:
  - family: <family name>
    context_namespace: <optional context segment>
    metrics:
      - <metric_name>
    chart_defaults:
      label_promotion: [<label>, ...]
      instances:
        by_labels: [<label>, ...]
    charts:
      - <chart definition>
    groups:
      - <nested group>
```

| Field               | Type          | Required | Description                                                                         |
|---------------------|---------------|----------|-------------------------------------------------------------------------------------|
| `family`            | string        | **yes**  | Family segment. Groups compose the chart family hierarchy.                          |
| `context_namespace` | string        | no       | Context segment appended to inherited context namespace.                            |
| `metrics`           | array[string] | no       | Metrics visible to dimension selectors in this group and descendants.               |
| `chart_defaults`    | object        | no       | Inheritable defaults for descendant charts (see [chart_defaults](#chart_defaults)). |
| `charts`            | array         | no       | Chart definitions (see [charts](#5-charts)).                                        |
| `groups`            | array         | no       | Nested groups (recursive).                                                          |

**Family composition** — group families compose hierarchically. The final chart family is built by joining all group `family` segments and the chart's own `family` (if set) with `/`:

| Level                | Family value                            |
|----------------------|-----------------------------------------|
| Root group           | `Storage Engine`                        |
| Nested group         | `InnoDB`                                |
| Nested group         | `Buffer Pool`                           |
| **Resulting family** | **`Storage Engine/InnoDB/Buffer Pool`** |

Here is a real-world nesting example showing how family and context compose at each level:

```yaml
# context_namespace: mysql (set at top level)
groups: # family                                context
  - family: Storage Engine                # Storage Engine                        (inherited)
    groups:
      - family: InnoDB                    # Storage Engine/InnoDB                 (inherited)
        groups:
          - family: Buffer Pool           # Storage Engine/InnoDB/Buffer Pool
            charts:
              - context: pages            #                                       → mysql.pages
          - family: I/O                   # Storage Engine/InnoDB/I/O
            charts:
              - context: bandwidth        #                                       → mysql.bandwidth
      - family: MyISAM                    # Storage Engine/MyISAM
        charts:
          - context: key_blocks           #                                       → mysql.key_blocks
```

Without nesting, you would repeat `Storage Engine/InnoDB/` in every chart's family and `mysql.` in every context. Nesting eliminates that repetition and makes the structure self-documenting.

> [!WARNING]
> Dimensions can only reference metrics declared in their group or any ancestor group. Referencing a metric not in scope produces a validation error.

**Metric scoping** — this prevents accidental cross-references and keeps templates self-documenting:

```yaml
groups:
  - family: Database
    metrics:
      - queries_total        # visible to all charts in this group and nested groups
    groups:
      - family: Cache
        metrics:
          - cache_hits        # visible only in this group and its descendants
        charts:
          - title: Cache Performance
            context: cache
            units: hits/s
            dimensions:
              - selector: cache_hits      # OK — declared in this group
                name: hits
              - selector: queries_total   # OK — inherited from parent group
                name: queries
```

#### chart_defaults

Inheritable chart configuration applied to all descendant charts in the group subtree. Useful when many charts share the same instance identity or label promotion policy.

| Field             | Type          | Description                              |
|-------------------|---------------|------------------------------------------|
| `label_promotion` | array[string] | Default labels to promote on all charts. |
| `instances`       | object        | Default instance identity policy.        |

> [!NOTE]
> **Inheritance rules**: nearest group default wins (child overrides parent), chart-local field overrides inherited default, and list/object fields replace the inherited field wholesale — there is no deep merge or append.

**Example: Azure Monitor — all charts share the same instance identity**

```yaml
groups:
  - family: Azure Key Vault
    context_namespace: key_vault
    chart_defaults:
      label_promotion: [resource_name, resource_group, region]
      instances:
        by_labels: [resource_uid]
    charts:
      # Every chart below inherits instances and label_promotion
      # without repeating them.
      - id: availability
        title: Azure Key Vault Availability
        context: availability
        units: percentage
        dimensions:
          - selector: key_vault.availability_average
            name: average
      - id: api_latency
        title: Azure Key Vault API Latency
        context: api_latency
        units: milliseconds
        dimensions:
          - selector: key_vault.service_api_latency_average
            name: average
```

Without `chart_defaults`, you would need to repeat `instances` and `label_promotion` on every chart.

### 5. charts

A chart defines a single visualization in the Netdata dashboard.

```yaml
charts:
  - id: <chart ID>
    title: <chart title>
    family: <optional family leaf>
    context: <chart context>
    units: <units string>
    algorithm: <absolute|incremental>
    type: <line|area|stacked|heatmap>
    priority: <int>
    label_promotion: [<label>, ...]
    instances:
      by_labels: [<label>, ...]
    lifecycle:
      max_instances: <int>
      expire_after_cycles: <int>
      dimensions:
        max_dims: <int>
        expire_after_cycles: <int>
    dimensions:
      - <dimension definition>
```

| Field             | Type          | Required | Default                | Description                                                                  |
|-------------------|---------------|----------|------------------------|------------------------------------------------------------------------------|
| `id`              | string        | no       | derived from `context` | Base chart ID. If omitted, derived by replacing `.` with `_` in `context`.   |
| `title`           | string        | **yes**  |                        | Chart title shown in the dashboard.                                          |
| `family`          | string        | no       |                        | Optional chart-level family leaf, appended to the group family.              |
| `context`         | string        | **yes**  |                        | Chart context leaf. Combined with context namespaces.                        |
| `units`           | string        | **yes**  |                        | Chart units (e.g., `queries/s`, `bytes`, `percentage`).                      |
| `algorithm`       | string        | no       | inferred from metrics  | `absolute` or `incremental`. If omitted, inferred from metric suffixes.      |
| `type`            | string        | no       | `line`                 | `line`, `area`, `stacked`, or `heatmap`.                                     |
| `priority`        | int           | no       | `70000`                | Chart ordering priority in the dashboard (`0` = use engine default `70000`). |
| `label_promotion` | array[string] | no       | from `chart_defaults`  | Labels to promote as chart labels (for filtering/grouping in UI). Entries must be non-empty label keys. |
| `instances`       | object        | no       | from `chart_defaults`  | Instance identity policy (see [instances](#instances)).                      |
| `lifecycle`       | object        | no       |                        | Instance/dimension cap and expiry (see [lifecycle](#lifecycle)).             |
| `dimensions`      | array         | **yes**  |                        | At least one dimension required (see [dimensions](#6-dimensions)).           |

> [!TIP]
> When `algorithm` is omitted, the engine infers it from metric name suffixes. You only need to set it explicitly when the suffix doesn't match the intended behavior (e.g., a gauge metric named `*_total`).

| Suffix                                    | Inferred algorithm |
|-------------------------------------------|--------------------|
| `*_total`, `*_count`, `*_sum`, `*_bucket` | `incremental`      |
| Everything else                           | `absolute`         |

> [!WARNING]
> If a chart's dimensions mix counter-like metrics (e.g., `requests_total`) with gauge-like metrics (e.g., `temperature`) and `algorithm` is omitted, the engine fails with a compile error: _"algorithm inference is ambiguous for mixed metric kinds; set algorithm explicitly"_. Set `algorithm` on the chart to resolve this.

**Example: MySQL queries — incremental counters displayed as rates**

```yaml
charts:
  - id: queries
    title: Queries
    context: queries
    units: queries/s
    algorithm: incremental
    dimensions:
      - selector: queries
        name: queries
      - selector: questions
        name: questions
      - selector: slow_queries
        name: slow_queries
```

**Example: MySQL bandwidth — bidirectional area chart with unit conversion**

```yaml
charts:
  - id: net
    title: Bandwidth
    context: net
    units: kilobits/s
    type: area
    algorithm: incremental
    dimensions:
      - selector: bytes_received
        name: in
        options:
          multiplier: 8
          divisor: 1000
      - selector: bytes_sent
        name: out
        options:
          multiplier: -8       # negative = below zero line
          divisor: 1000
```

#### instances

Instance identity determines how series are grouped into chart instances. When multiple series share the same instance identity label values, they appear as dimensions on the same chart instance.

> [!TIP]
> Without `instances`, there is one chart instance (all matching series land on the same chart). With `instances`, the engine creates one chart instance per unique combination of the specified label values.

```yaml
instances:
  by_labels: [host]
```

| Token        | Meaning                                                       |
|--------------|---------------------------------------------------------------|
| `label_key`  | Include this label in instance identity.                      |
| `*`          | Include all labels.                                           |
| `!label_key` | Exclude this label (use with `*` to include all _except_...). |

Excludes are order-independent and always win. For example, both `["host", "!host"]` and `["!host", "host"]` exclude `host`.
When `instances` is set, `by_labels` must include at least one positive selector: `*` or `label_key`. Exclude tokens use strict `!label_key` syntax; `! host` is invalid.

**Example: One chart per host**

```yaml
instances:
  by_labels: [host]
```

If the collector reports metrics for hosts `server-1`, `server-2`, `server-3`, the engine creates 3 separate chart instances — each showing only that host's dimensions.

**Example: One chart per unique (job, instance) combination**

```yaml
instances:
  by_labels: [nagios_job, perfdata_value]
```

**Example: All labels except one**

```yaml
instances:
  by_labels: ["*", "!_collect_job"]
```

#### lifecycle

Controls cardinality limits and expiry for chart instances and dimensions.

| Field                            | Type | Default        | Description                                                                          |
|----------------------------------|------|----------------|--------------------------------------------------------------------------------------|
| `max_instances`                  | int  | `0` (disabled) | Best-effort cap on chart instances per template. Active instances are never evicted. |
| `expire_after_cycles`            | int  | `5`            | Remove chart instances not seen for N successful collection cycles.                  |
| `dimensions.max_dims`            | int  | `0` (disabled) | Best-effort cap on dimensions per chart instance.                                    |
| `dimensions.expire_after_cycles` | int  | `0` (disabled) | Remove dimensions not seen for N successful collection cycles.                       |

**How lifecycle caps work**:

- Caps are **best-effort** — instances/dimensions actively seen in the current cycle are never evicted.
- Oldest inactive entries are evicted first (by last-seen time).
- Expiry counters only advance on **successful** collection cycles.

### 6. dimensions

A dimension binds a metric from the collector's metric store to a line on the chart.

```yaml
dimensions:
  - selector: <metric selector>
    name: <static name>
    name_from_label: <label key>
    options:
      multiplier: <int>
      divisor: <int>
      hidden: <bool>
      float: <bool>
```

| Field                | Type   | Required | Default | Description                                                      |
|----------------------|--------|----------|---------|------------------------------------------------------------------|
| `selector`           | string | **yes**  |         | Metric selector expression (see [selectors](#selectors) below).  |
| `name`               | string | no       |         | Static dimension name shown in the chart.                        |
| `name_from_label`    | string | no       |         | Dynamic name: use the value of this label as the dimension name. |
| `options.multiplier` | int    | no       | `1`     | Multiply the raw value by this factor.                           |
| `options.divisor`    | int    | no       | `1`     | Divide the raw value by this factor.                             |
| `options.hidden`     | bool   | no       | `false` | Hide this dimension in the chart (still collected).              |
| `options.float`      | bool   | no       | `false` | Use floating-point precision for this dimension.                 |

> [!IMPORTANT]
> There are three ways to name a dimension — pick **exactly one**:
> - `name` — static name you choose (e.g., `name: read`).
> - `name_from_label` — dynamic name from a label value (e.g., `name_from_label: method` → dimensions "GET", "POST", ...).
> - **Omit both** — the engine infers the name automatically for histogram buckets (`le`), summary quantiles (`quantile`), and statesets.
>
> `name` and `name_from_label` are mutually exclusive. Duplicate static `name` values within the same chart are rejected.

#### selectors

A selector specifies which metric(s) a dimension should match.

**Syntax:**

```
metric_name
metric_name{label_key=label_value, ...}
```

- The metric name prefix is **required** — label-only selectors like `{label=value}` are rejected.
- The metric must be declared in the current group's `metrics` list (or inherited from an ancestor group).
- Label filters narrow which series match. Without labels, all series of that metric match.

**Examples:**

```yaml
# Match all series of the "queries" metric
- selector: queries

# Match only series where method="GET"
- selector: http_requests_total{method="GET"}

# Match a specific histogram bucket
- selector: request_duration_seconds_bucket{le="0.5"}
```

#### Common dimension patterns

**Unit conversion** — convert bytes to kilobits per second:

```yaml
dimensions:
  - selector: bytes_received
    name: in
    options:
      multiplier: 8
      divisor: 1000
```

**Bidirectional charts** — use a negative multiplier to display below zero:

```yaml
dimensions:
  - selector: bytes_received
    name: in
    options:
      multiplier: 8
      divisor: 1000
  - selector: bytes_sent
    name: out
    options:
      multiplier: -8
      divisor: 1000
```

**Float precision** — for ratios or small decimal values:

```yaml
dimensions:
  - selector: efficiency_ratio
    name: efficiency
    options:
      float: true
```

**Dynamic naming from labels** — each unique label value becomes a separate dimension:

```yaml
dimensions:
  - selector: http_requests_total
    name_from_label: method
```

If the metric has series with `method="GET"`, `method="POST"`, etc., each becomes its own dimension on the chart.

## Examples

### Simple: static metrics, no instances

A collector that monitors a single MySQL server. Each chart has a fixed set of dimensions.

```yaml
version: v1
context_namespace: mysql
groups:
  - family: Queries
    groups:
      - family: Statistics
        metrics:
          - queries
          - questions
          - slow_queries
          - com_delete
          - com_insert
          - com_select
          - com_update
        charts:
          - id: queries
            title: Queries
            context: queries
            units: queries/s
            algorithm: incremental
            dimensions:
              - selector: queries
                name: queries
              - selector: questions
                name: questions
              - selector: slow_queries
                name: slow_queries
          - id: queries_type
            title: Queries By Type
            context: queries_type
            units: queries/s
            type: stacked
            algorithm: incremental
            dimensions:
              - selector: com_delete
                name: delete
              - selector: com_insert
                name: insert
              - selector: com_select
                name: select
              - selector: com_update
                name: update
```

### Per-instance: one chart per host

A ping collector that monitors multiple hosts. Each host gets its own set of charts.

```yaml
version: v1
context_namespace: ping
groups:
  - family: latency
    metrics:
      - min_rtt
      - max_rtt
      - avg_rtt
    charts:
      - id: host_rtt
        title: Ping round-trip time
        context: host_rtt
        units: milliseconds
        type: area
        instances:
          by_labels: [host]
        dimensions:
          - selector: min_rtt
            name: min
            options:
              divisor: 1000
          - selector: max_rtt
            name: max
            options:
              divisor: 1000
          - selector: avg_rtt
            name: avg
            options:
              divisor: 1000
```

### Per-instance with multiple labels

MySQL replication monitoring creates one chart per replication connection.

```yaml
groups:
  - family: Replication
    groups:
      - family: Slave Status
        metrics:
          - seconds_behind_master
          - slave_io_running
          - slave_sql_running
        charts:
          - id: slave_behind
            title: Slave Behind Seconds
            context: slave_behind
            units: seconds
            instances:
              by_labels: [connection]
            dimensions:
              - selector: seconds_behind_master
                name: seconds
          - id: slave_thread_running
            title: I/O / SQL Thread Running State
            context: slave_status
            units: boolean
            instances:
              by_labels: [connection]
            dimensions:
              - selector: slave_io_running
                name: io_running
              - selector: slave_sql_running
                name: sql_running
```

### Deeply nested groups

MySQL's InnoDB storage engine metrics organized in a deep hierarchy.

```yaml
groups:
  - family: Storage Engine
    groups:
      - family: InnoDB
        groups:
          - family: Buffer Pool
            metrics:
              - innodb_buffer_pool_pages_data
              - innodb_buffer_pool_pages_dirty
              - innodb_buffer_pool_pages_free
              - innodb_buffer_pool_pages_misc
              - innodb_buffer_pool_pages_total
            charts:
              - id: innodb_buffer_pool_pages
                title: InnoDB Buffer Pool Pages
                context: innodb_buffer_pool_pages
                units: pages
                dimensions:
                  - selector: innodb_buffer_pool_pages_data
                    name: data
                  - selector: innodb_buffer_pool_pages_dirty
                    name: dirty
                    options:
                      multiplier: -1
                  - selector: innodb_buffer_pool_pages_free
                    name: free
                  - selector: innodb_buffer_pool_pages_misc
                    name: misc
                    options:
                      multiplier: -1
                  - selector: innodb_buffer_pool_pages_total
                    name: total
          - family: I/O
            metrics:
              - innodb_data_read
              - innodb_data_written
            charts:
              - id: innodb_io
                title: InnoDB I/O Bandwidth
                context: innodb_io
                units: KiB/s
                type: area
                algorithm: incremental
                dimensions:
                  - selector: innodb_data_read
                    name: read
                    options:
                      divisor: 1024
                  - selector: innodb_data_written
                    name: write
                    options:
                      divisor: 1024
```

The resulting chart families are `Storage Engine/InnoDB/Buffer Pool` and `Storage Engine/InnoDB/I/O`.

### chart_defaults: reducing repetition

When monitoring a cloud resource that has many charts, all sharing the same instance identity.

```yaml
groups:
  - family: Azure PostgreSQL
    context_namespace: postgres_flexible
    chart_defaults:
      label_promotion: [resource_name, resource_group, region]
      instances:
        by_labels: [resource_uid]
    charts:
      - title: CPU Percent
        context: cpu_percent
        units: percentage
        dimensions:
          - selector: postgres_flexible.cpu_percent_average
            name: average
      - title: Memory Percent
        context: memory_percent
        units: percentage
        dimensions:
          - selector: postgres_flexible.memory_percent_average
            name: average
      - title: Storage Percent
        context: storage_percent
        units: percentage
        dimensions:
          - selector: postgres_flexible.storage_percent_average
            name: average
```

All three charts inherit `instances` and `label_promotion` from `chart_defaults` — no repetition needed.

### Autogeneration: handling unpredictable metrics

For collectors where the metric set is user-defined or discovered at runtime, use autogen to catch metrics that don't match any explicit chart template.

```yaml
version: v1
context_namespace: prometheus_scraper
engine:
  autogen:
    enabled: true
    expire_after_success_cycles: 30
  selector:
    deny: ["go_*", "promhttp_*"]      # exclude internal Go/Prometheus metrics
groups:
  - family: Application
    metrics:
      - app_http_requests_total
      - app_http_response_time_seconds
    charts:
      - id: app_requests
        title: Application HTTP Requests
        context: http_requests
        units: requests/s
        algorithm: incremental
        instances:
          by_labels: [instance]
        dimensions:
          - selector: app_http_requests_total
            name_from_label: status_code
      - id: app_response_time
        title: Application Response Time
        context: response_time
        units: seconds
        instances:
          by_labels: [instance]
        dimensions:
          - selector: app_http_response_time_seconds
            name: p99
            options:
              float: true
```

The explicitly defined charts handle `app_http_requests_total` and `app_http_response_time_seconds`. Any _other_ application metrics the scraper discovers are automatically charted by the engine, and removed after 30 cycles of inactivity. The `selector.deny` filter excludes noisy internal metrics from autogeneration.

## Validation Rules

> [!CAUTION]
> Unknown YAML fields cause an immediate decode error (strict unmarshal). Double-check field names for typos — a misspelled field like `demensions` will be caught at parse time with an unmarshal error, not at runtime with a descriptive message pointing to the affected chart.

All rules below produce semantic validation errors unless noted:

| Rule                                                                                    | Error type                      |
|-----------------------------------------------------------------------------------------|---------------------------------|
| `version` must be `v1`                                                                  | semantic                        |
| `groups[]` must be non-empty                                                            | semantic                        |
| `group.family` must not be empty or whitespace-only                                     | semantic                        |
| `group.metrics[]` entries must not be empty; no duplicates within same group            | semantic                        |
| `chart.title`, `chart.context`, `chart.units` must be non-empty                         | semantic                        |
| `chart.algorithm` must be `absolute` or `incremental` (when specified)                  | semantic                        |
| `chart.type` must be `line`, `area`, `stacked`, or `heatmap` (when specified)           | semantic                        |
| `dimension.selector` must include explicit metric name (prefix before `{`)              | semantic                        |
| Selector metric must be visible in current group metric scope                           | semantic                        |
| `name` and `name_from_label` are mutually exclusive                                     | semantic                        |
| `name` and `name_from_label` must not be whitespace-only                                | semantic                        |
| Duplicate dimension `name` values within the same chart are rejected                    | semantic                        |
| `instances.by_labels` must contain at least one token when `instances` is set           | semantic                        |
| `instances.by_labels` exclude token must use `!label_key` syntax                         | semantic                        |
| `instances.by_labels` must include at least one positive selector (`*` or `label_key`)   | semantic                        |
| `instances.by_labels` tokens must not be duplicated                                     | semantic                        |
| `label_promotion[]` entries must not be empty or whitespace-only                        | semantic                        |
| Lifecycle numeric fields must be `>= 0`                                                 | semantic                        |
| `engine.autogen.max_type_id_len` must be `0` or `>= 4`                                  | semantic                        |
| Unknown YAML fields                                                                     | decode error (strict unmarshal) |

## Compiler-Derived Behavior

> [!NOTE]
> These behaviors are applied by `chartengine` during compilation, not by the template parser. You don't need to configure them — they happen automatically, but knowing about them helps you write simpler templates.

| Input                                   | Derived behavior                                                                         |
|-----------------------------------------|------------------------------------------------------------------------------------------|
| Missing `chart.id`                      | `id` derived from `context` (`.` replaced with `_`).                                     |
| Missing `chart.algorithm`               | Inferred from metric suffixes (`*_total`, `*_count`, `*_sum`, `*_bucket` = incremental). |
| `chart.priority = 0`                    | Treated as `70000` (engine default).                                                     |
| Group family hierarchy + `chart.family` | Composed into `/`-separated chart family.                                                |
| `options.multiplier = 0`                | Treated as `1`.                                                                          |
| `options.divisor = 0`                   | Treated as `1`.                                                                          |
