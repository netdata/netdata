# Using Netdata with Prometheus

Netdata exports metrics to Prometheus through two methods:

- **[Configure Prometheus to scrape Netdata metrics](#configure-prometheus-to-scrape-netdata-metrics)** - Pull metrics from Netdata
- **[Configure Netdata to push metrics to Prometheus](/src/exporting/prometheus/remote_write/README.md)** - Push using remote write API

## Netdata Support for Prometheus

Before configuring either method, understand how Netdata structures its exported metrics and available capabilities. These concepts apply to both scraping and remote write methods.

### Understanding Netdata Metrics

#### Charts

Each Netdata chart has several properties common to all its metrics:

| Property     | Description                                                                                                                                    |
|:-------------|:-----------------------------------------------------------------------------------------------------------------------------------------------|
| `chart_id`   | Uniquely identifies a chart                                                                                                                    |
| `chart_name` | Human-friendly name for `chart_id`, also unique                                                                                                |
| `context`    | Chart template - all disk I/O charts share the same context, all MySQL request charts share the same context. Used for alert template matching |
| `family`     | Groups charts together as dashboard submenus                                                                                                   |
| `units`      | Units for all metrics in the chart                                                                                                             |

#### Dimensions

Each Netdata chart contains metrics called `dimensions`. All dimensions in a chart:

- Share the same units of measurement
- Belong to the same contextual category (e.g., disk bandwidth contains `read` and `write` dimensions)

### Netdata Data Source

Netdata sends metrics to Prometheus from 3 data sources:

#### 1. As-Collected (Raw)

Sends metrics exactly as collected without conversion. Prometheus prefers this method, but it requires understanding how to extract meaningful values.

**Metric formats:**

- Standard: `CONTEXT{chart="CHART",family="FAMILY",dimension="DIMENSION"}`
- Counters: `CONTEXT_total{chart="CHART",family="FAMILY",dimension="DIMENSION"}`
- Heterogeneous dimensions: `CONTEXT_DIMENSION{chart="CHART",family="FAMILY"}`

:::info

Unlike Prometheus, Netdata allows each dimension to have different algorithms and conversion constants (`multiplier` and `divisor`). When dimensions are heterogeneous, Netdata uses the `CONTEXT_DIMENSION` format.

:::

#### 2. Average

Sends metrics as they appear on the dashboard. All metrics become gauges in their dashboard units. This is the easiest to work with.

**Format:** `CONTEXT_UNITS_average{chart="CHART",family="FAMILY",dimension="DIMENSION"}`

Netdata tracks each Prometheus server's last access time to calculate averages for the time-frame between queries. This ensures no data loss regardless of scrape frequency. By default, Netdata identifies servers by client IP. For multiple servers using the same IP, append `server=NAME` to the URL for unique identification.

#### 3. Sum (Volume)

Like `average` but sums values instead of averaging them.

**Format:** `CONTEXT_UNITS_sum{chart="CHART",family="FAMILY",dimension="DIMENSION"}`

To change the data source, add the `source` parameter to the URL:

```
http://your.netdata.ip:19999/api/v1/allmetrics?format=prometheus&source=as-collected
```

:::info

Early Netdata versions sent metrics as `CHART_DIMENSION{}`.

:::

### Querying Metrics

Test the metrics endpoint in your browser:

```
http://your.netdata.ip:19999/api/v1/allmetrics?format=prometheus&help=yes
```

Replace `your.netdata.ip` with your Netdata server's IP or hostname.

Netdata responds with all metrics it sends to Prometheus. Search for `"system.cpu"` to find all CPU metrics (the chart name from the dashboard heading "Total CPU utilization (system.cpu)").

<details>
<summary><strong>Example: system.cpu with average source</strong></summary>

```sh
# COMMENT homogeneous chart "system.cpu", context "system.cpu", family "cpu", units "percentage"
# COMMENT netdata_system_cpu_percentage_average: dimension "guest_nice", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="guest_nice"} 0.0000000 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "guest", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="guest"} 1.7837326 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "steal", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="steal"} 0.0000000 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "softirq", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="softirq"} 0.5275442 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "irq", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="irq"} 0.2260836 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "user", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="user"} 2.3362762 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "system", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="system"} 1.7961062 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "nice", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="nice"} 0.0000000 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "iowait", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="iowait"} 0.9671802 1500066662000
# COMMENT netdata_system_cpu_percentage_average: dimension "idle", value is percentage, gauge, dt 1500066653 to 1500066662 inclusive
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="idle"} 92.3630770 1500066662000
```

</details>

In `average` or `sum` sources, all values are normalized and reported as gauges. Type `netdata_system_cpu` in the Prometheus expression field - it auto-completes as Prometheus recognizes the metric.

<details>
<summary><strong>Example: system.cpu with as-collected source</strong></summary>

```sh
# COMMENT homogeneous chart "system.cpu", context "system.cpu", family "cpu", units "percentage"
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "guest_nice", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="guest_nice"} 0 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "guest", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="guest"} 63945 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "steal", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="steal"} 0 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "softirq", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="softirq"} 8295 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "irq", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="irq"} 4079 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "user", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="user"} 116488 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "system", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="system"} 35084 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "nice", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="nice"} 505 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "iowait", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="iowait"} 23314 1500066716438
# COMMENT netdata_system_cpu_total: chart "system.cpu", context "system.cpu", family "cpu", dimension "idle", value * 1 / 1 delta gives percentage (counter)
netdata_system_cpu_total{chart="system.cpu",family="cpu",dimension="idle"} 918470 1500066716438
```

</details>

For more information, check Prometheus documentation.

### Streaming Data from Upstream Hosts

The `format=prometheus` parameter only exports the local host's metrics. For parent-child Netdata setups, use this configuration in **prometheus.yml**:

```yaml
metrics_path: '/api/v1/allmetrics'
params:
  format: [prometheus_all_hosts]
honor_labels: true
```

This reports all upstream host data with proper instance names.

### Timestamps

To pass metrics through Prometheus pushgateway, use `&timestamps=no` to send metrics without timestamps.

## Netdata Host Variables

Netdata collects system configuration metrics (max TCP sockets, system-wide file limits, IPC sizes, etc.) not exposed to Prometheus by default.

To expose them, append `variables=yes` to the URL.

### TYPE and HELP

`# TYPE` and `# HELP` lines are suppressed by default to save bandwidth (Prometheus doesn't use them). Re-enable with:

```
/api/v1/allmetrics?format=prometheus&types=yes&help=yes
```

:::warning

When enabled, `# TYPE` and `# HELP` lines repeat for every metric occurrence, violating [Prometheus specifications](https://github.com/prometheus/docs/blob/master/content/docs/instrumenting/exposition_formats.md#comments-help-text-and-type-information).

:::

### Names and IDs

Netdata supports both names and IDs for charts and dimensions. IDs are unique system identifiers; names are human-friendly labels (also unique). Most charts have identical ID and name, but some differ (device-mapper disks, interrupts, QoS classes, statsd synthetic charts).

Control the default in `exporting.conf`:

```text
[prometheus:exporter]
	send names instead of ids = yes | no
```

Override via URL:

- `&names=no` for IDs (old behavior)
- `&names=yes` for names

### Filtering Metrics Sent to Prometheus

Filter metrics with this setting:

```text
[prometheus:exporter]
	send charts matching = *
```

This accepts space-separated [simple patterns](/src/libnetdata/simple_pattern/README.md) to match charts. Pattern rules:

- `*` as wildcard (e.g., `*a*b*c*` is valid)
- `!` prefix for negative match
- First match (positive or negative) wins
- Example: `!*.bad users.* groups.*` sends all users and groups except `bad` ones

### Changing the Prefix of Netdata Metrics

Netdata prefixes all metrics with `netdata_`. Change in `netdata.conf`:

```text
[prometheus:exporter]
	prefix = netdata
```

Or append `&prefix=netdata` to the URL.

### Metric Units

| Source              | Unit Behavior                             | Control                             |
|:--------------------|:------------------------------------------|:------------------------------------|
| `average` (default) | Adds units to names (e.g., `_KiB_persec`) | `&hideunits=yes` to hide            |
| `as-collected`      | No units in names                         | N/A                                 |
| All sources         | v1.12+ standardized units                 | `&oldunits=yes` for pre-v1.12 names |

### Accuracy of Average and Sum Data Sources

When using `average` or `sum` sources, Netdata remembers each client's last access time to calculate values for the period since last access. This prevents data loss regardless of scrape frequency.

Server identification:
| Scenario | Method |
|:---------|:-------|
| Direct connection | Client IP (default) |
| Behind proxy or NAT | Append `&server=NAME` to URL |
| Multiple servers, same IP | Each uses unique `&server=NAME` |

### Host Labels

Netdata supports custom host labels that are exported to Prometheus. Configure labels in `/etc/netdata/netdata.conf`:

```ini
[host labels]
    environment = production
    region = us-east-1
    datacenter = dc1
```

After defining them, the labels will appear in the `netdata_info` metric, for example:

```text
netdata_info{
    instance="some-server",
    application="netdata",
    version="v2.7.2",
    datacenter="dc1",
    region="us-east-1",
    environment="production",
    _is_ephemeral="false"
} 1 1761148307085
```

## Configure Prometheus to Scrape Netdata Metrics

The following `prometheus.yml` scrapes all Netdata metrics "as collected":

```yaml
# my global config
global:
  scrape_interval: 5s # Set the scrape interval to every 5 seconds. Default is every 1 minute.
  evaluation_interval: 5s # Evaluate rules every 5 seconds. The default is every 1 minute.
  # scrape_timeout is set to the global default (10s).

  # Attach these labels to any time series or alerts when communicating with
  # external systems (federation, remote storage, Alertmanager).
  external_labels:
    monitor: 'codelab-monitor'

# Load rules once and periodically evaluate them according to the global 'evaluation_interval'.
rule_files:
# - "first.rules"
# - "second.rules"

# A scrape configuration containing exactly one endpoint to scrape:
# Here it's Prometheus itself.
scrape_configs:
  # The job name is added as a label `job=<job_name>` to any timeseries scraped from this config.
  - job_name: 'prometheus'

    # metrics_path defaults to '/metrics'
    # scheme defaults to 'http'.

    static_configs:
      - targets: ['0.0.0.0:9090']

  - job_name: 'netdata-scrape'

    metrics_path: '/api/v1/allmetrics'
    params:
      # format: prometheus | prometheus_all_hosts
      # You can use `prometheus_all_hosts` if you want Prometheus to set the `instance` to your hostname instead of IP 
      format: [prometheus]
      #
      # sources: as-collected | raw | average | sum | volume
      # default is: average
      #source: [as-collected]
      #
      # server name for this prometheus - the default is the client IP
      # for Netdata to uniquely identify it
      #server: ['prometheus1']
    honor_labels: true

    static_configs:
      - targets: ['{your.netdata.ip}:19999']
```

Replace `{your.netdata.ip}` with your Netdata host's IP or hostname.

### Prometheus Alerts for Netdata Metrics

Example `nodes.yml` file for generating alerts from Netdata metrics. Save at `/opt/prometheus/nodes.yml` and add `- "nodes.yml"` under `rule_files:` in prometheus.yml:

```yaml
groups:
  - name: nodes

    rules:
      - alert: node_high_cpu_usage_70
        expr: sum(sum_over_time(netdata_system_cpu_percentage_average{dimension=~"(user|system|softirq|irq|guest)"}[10m])) by (job) / sum(count_over_time(netdata_system_cpu_percentage_average{dimension="idle"}[10m])) by (job) > 70
        for: 1m
        annotations:
          description: '{{ $labels.job }} on ''{{ $labels.job }}'' CPU usage is at {{ humanize $value }}%.'
          summary: CPU alert for container node '{{ $labels.job }}'

      - alert: node_high_memory_usage_70
        expr: 100 / sum(netdata_system_ram_MB_average) by (job)
          * sum(netdata_system_ram_MB_average{dimension=~"free|cached"}) by (job) < 30
        for: 1m
        annotations:
          description: '{{ $labels.job }} memory usage is {{ humanize $value}}%.'
          summary: Memory alert for container node '{{ $labels.job }}'

      - alert: node_low_root_filesystem_space_20
        expr: 100 / sum(netdata_disk_space_GB_average{family="/"}) by (job)
          * sum(netdata_disk_space_GB_average{family="/",dimension=~"avail|cached"}) by (job) < 20
        for: 1m
        annotations:
          description: '{{ $labels.job }} root filesystem space is {{ humanize $value}}%.'
          summary: Root filesystem alert for container node '{{ $labels.job }}'

      - alert: node_root_filesystem_fill_rate_6h
        expr: predict_linear(netdata_disk_space_GB_average{family="/",dimension=~"avail|cached"}[1h], 6 * 3600) < 0
        for: 1h
        labels:
          severity: critical
        annotations:
          description: Container node {{ $labels.job }} root filesystem is going to fill up in 6h.
          summary: Disk fill alert for Swarm node '{{ $labels.job }}'
```
