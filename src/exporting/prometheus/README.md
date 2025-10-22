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

## Configure Prometheus to Scrape Netdata Metrics

### Basic Configuration

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

### Kubernetes Service Discovery

Running Netdata in Kubernetes? Configure Prometheus to automatically discover and scrape all your Netdata pods.

#### Using Kubernetes SD Config

Add this job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'netdata-k8s'
    kubernetes_sd_configs:
      - role: pod
    
    relabel_configs:
      # Keep only pods with the Prometheus scrape annotation
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      
      # Use custom port if specified in annotations
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
      
      # Set instance label to pod name for clear identification
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: instance
      
      # Add namespace label for filtering
      - source_labels: [__meta_kubernetes_namespace]
        target_label: kubernetes_namespace
      
      # Add node label to identify which K8s node the pod runs on
      - source_labels: [__meta_kubernetes_pod_node_name]
        target_label: kubernetes_node
    
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus]
      source: [as-collected]
    
    scrape_interval: 15s
    scrape_timeout: 10s
```

#### Using Prometheus Operator ServiceMonitor

Create `netdata-servicemonitor.yaml`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: netdata
  namespace: monitoring
  labels:
    app: netdata
spec:
  selector:
    matchLabels:
      app: netdata
  namespaceSelector:
    matchNames:
      - default
      - kube-system
      - monitoring
  endpoints:
    - port: http
      path: /api/v1/allmetrics
      params:
        format: 
          - prometheus
        source: 
          - as-collected
      interval: 15s
      scrapeTimeout: 10s
      relabelings:
        - sourceLabels: [__meta_kubernetes_pod_name]
          targetLabel: instance
```

Apply the ServiceMonitor:

```bash
kubectl apply -f netdata-servicemonitor.yaml
```

#### Annotate Netdata Pods

Add these annotations to your Netdata Helm `values.yaml`:

```yaml
podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "19999"
  prometheus.io/path: "/api/v1/allmetrics"
```

Or add them directly to your Netdata DaemonSet:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: netdata
spec:
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "19999"
        prometheus.io/path: "/api/v1/allmetrics"
```

## Query Different Data Sources

Each data source requires different query approaches in Prometheus. Choose the method that matches your configured source.

### Query As-Collected (Counters)

As-collected metrics export raw counters. Use `rate()` or `increase()` functions:

```promql
# CPU usage rate over 5 minutes
rate(netdata_system_cpu_total{dimension="user"}[5m])

# Total bytes received in last hour
increase(netdata_net_kilobits_persec_total{dimension="received"}[1h])

# Network bandwidth in bits per second
rate(netdata_net_kilobits_persec_total[5m]) * 1000
```

**Advantages**: Most efficient, Prometheus-native, lowest memory usage  
**Considerations**: Requires understanding counter semantics and rate calculations

### Query Average (Gauges)

Average metrics provide pre-calculated values. Use them directly:

```promql
# Current CPU usage (no rate needed)
netdata_system_cpu_percentage_average{dimension="user"}

# Average memory usage over 5 minutes
avg_over_time(netdata_system_ram_MB_average{dimension="used"}[5m])

# Memory usage percentage
100 * (
  netdata_system_ram_MB_average{dimension="used"} /
  netdata_system_ram_MB_average
)
```

**Advantages**: Easiest to query, matches Netdata dashboard exactly  
**Considerations**: Higher memory usage in Prometheus

### Query Sum (Volume)

Sum metrics provide accumulated values over the scrape interval:

```promql
# Total volume of data received between scrapes
netdata_net_kilobits_persec_sum{dimension="received"}

# Total disk writes in last hour
sum_over_time(netdata_disk_io_KiB_persec_sum{dimension="writes"}[1h])
```

**Advantages**: Accurate volume tracking for billing and accounting  
**Considerations**: Depends on consistent scrape intervals

### Choose by Use Case

| Use Case | Recommended Source | Reason |
|----------|-------------------|--------|
| Production monitoring | `as-collected` | Lowest overhead, follows Prometheus best practices |
| Quick setup / testing | `average` | Simpler queries, matches Netdata UI |
| Billing / accounting | `sum` | Accurate volume tracking |
| Learning Prometheus | `as-collected` | Teaches proper counter handling |
| Grafana dashboards | `average` | Simpler queries for dashboard builders |

### Prometheus Alerts for Netdata Metrics

Create alert rules for proactive monitoring. Save these rules at `/opt/prometheus/nodes.yml` and add `- "nodes.yml"` under `rule_files:` in prometheus.yml:

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
      
      # Network alerts
      - alert: node_high_network_errors
        expr: rate(netdata_net_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High network errors on {{ $labels.instance }}"
          description: "Network interface {{ $labels.family }} has {{ $value }} errors/sec"
      
      # Disk I/O alerts
      - alert: node_high_disk_io_utilization
        expr: rate(netdata_disk_busy_time_percentage_average[5m]) > 80
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High disk I/O on {{ $labels.instance }}"
          description: "Disk {{ $labels.family }} is {{ $value }}% busy"
      
      # Process alerts
      - alert: node_high_process_count
        expr: netdata_system_active_processes_average > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High process count on {{ $labels.instance }}"
          description: "{{ $value }} active processes detected"
```

## Performance Tuning and Best Practices

Optimize your Netdata-Prometheus integration for scale, efficiency, and reliability.

### Choose the Right Scrape Interval

Match your scrape interval to your deployment scale and monitoring needs:

| Deployment Size | Node Count | Metrics/Second | Scrape Interval | Scrape Timeout |
|-----------------|------------|----------------|-----------------|----------------|
| Small | 1-10 | < 100K | 5-15s | 10s |
| Medium | 10-100 | 100K-1M | 15-30s | 20s |
| Large | 100-1000 | 1M-5M | 30-60s | 30s |
| Enterprise | 1000+ | 5M+ | Use Parents | 60s |

Set the interval in your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'netdata'
    scrape_interval: 30s
    scrape_timeout: 20s  # Must be less than scrape_interval
```

:::tip

Set `scrape_timeout` lower than `scrape_interval`. Prometheus fails the scrape if it exceeds the timeout.

:::

### Manage Metric Cardinality

High cardinality metrics increase Prometheus memory usage and slow down queries. Reduce cardinality to improve performance.

#### Filter Metrics in Netdata

Edit `/etc/netdata/exporting.conf`:

```ini
[prometheus:exporter]
    # Export only essential system and application metrics
    send charts matching = system.* disk.* net.* apps.cpu apps.mem !*tmp* !*cache* !*guest*
```

Restart Netdata:

```bash
sudo systemctl restart netdata
```

#### Use Metric Relabeling in Prometheus

```yaml
metric_relabel_configs:
  # Drop high-cardinality dimensions
  - source_labels: [dimension]
    regex: '(.*_tmp_.*|.*_cache_.*|.*_guest.*)'
    action: drop
```

#### Choose the Right Data Source

| Data Source | Prometheus Memory Impact | Best For |
|-------------|-------------------------|----------|
| `as-collected` | Lowest (counters) | Production, large scale |
| `average` | Medium (gauges) | Easier queries, medium scale |
| `sum` | Medium (gauges) | Volume tracking |

:::tip

Use `as-collected` for production deployments with more than 100 nodes. This source provides the lowest memory footprint and follows Prometheus best practices.

:::

### Use Parent-Child Architecture for Scale

For deployments with 100+ nodes, deploy Netdata Parents to aggregate metrics before Prometheus scrapes them:

```
[Agents] → [Parents] ← [Prometheus]
  1000        2-4          1
```

Benefits:
- Reduces Prometheus scrape targets from 1000 to 2-4
- Parents handle aggregation and deduplication
- Agents can run in minimal mode (RAM-only, no ML)
- Single Prometheus instance can handle 1000+ nodes

Configure Prometheus to scrape Parents:

```yaml
scrape_configs:
  - job_name: 'netdata-parents'
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus_all_hosts]  # Include all upstream hosts
    honor_labels: true
    static_configs:
      - targets: 
          - 'parent1.example.com:19999'
          - 'parent2.example.com:19999'
```

## Securing the Prometheus Endpoint

By default, Netdata's web server (port 19999) is unauthenticated. Anyone with network access can scrape all metrics, access the Netdata UI, and execute Netdata Functions (if enabled). Secure the endpoint for production environments.

### Use a Reverse Proxy with Authentication

Deploy nginx or HAProxy to add authentication and TLS to your Netdata metrics endpoint.

#### nginx Configuration

Install nginx and create a password file:

```bash
sudo apt-get install nginx apache2-utils
sudo htpasswd -c /etc/nginx/.htpasswd prometheus
```

Configure nginx at `/etc/nginx/sites-available/netdata`:

```nginx
server {
    listen 19999 ssl;
    server_name netdata.example.com;
    
    ssl_certificate /etc/ssl/certs/netdata.crt;
    ssl_certificate_key /etc/ssl/private/netdata.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    
    location /api/v1/allmetrics {
        auth_basic "Netdata Metrics";
        auth_basic_user_file /etc/nginx/.htpasswd;
        proxy_pass http://127.0.0.1:19998;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
    
    # Block other endpoints
    location / {
        deny all;
    }
}
```

Configure Netdata to bind to localhost only in `/etc/netdata/netdata.conf`:

```ini
[web]
    bind to = 127.0.0.1
```

Restart services:

```bash
sudo systemctl restart netdata
sudo systemctl restart nginx
```

Configure Prometheus with credentials in `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'netdata-secure'
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/ca.crt
      insecure_skip_verify: false
    basic_auth:
      username: prometheus
      password_file: /etc/prometheus/netdata_password
    static_configs:
      - targets: ['netdata.example.com:19999']
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus]
      source: [as-collected]
```

#### HAProxy Configuration

Configure HAProxy at `/etc/haproxy/haproxy.cfg`:

```haproxy
frontend netdata_metrics
    bind *:19999 ssl crt /etc/haproxy/certs/netdata.pem
    acl authorized http_auth(prometheus_users)
    http-request auth realm Netdata if !authorized
    default_backend netdata_backend

backend netdata_backend
    server netdata1 127.0.0.1:19998 check

userlist prometheus_users
    user prometheus password $6$rounds=50000$...
```

### Restrict Access by IP Address

Limit connections to specific IP addresses in `/etc/netdata/netdata.conf`:

```ini
[web]
    bind to = *
    allow connections from = localhost 10.0.0.0/8 192.168.1.100
    allow dashboard from = localhost
    allow badges from = *
    allow streaming from = *
    allow netdata.conf from = localhost
    allow management from = localhost
```

This configuration:
- Allows Prometheus (10.0.0.0/8 or specific IP) to scrape metrics
- Restricts dashboard access to localhost
- Permits badge access from any IP
- Controls streaming and management access

### Use Kubernetes Network Policies

Restrict access to Netdata pods using NetworkPolicies:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netdata-ingress
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: netdata
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: monitoring
        - podSelector:
            matchLabels:
              app: prometheus
      ports:
        - protocol: TCP
          port: 19999
```

Apply the policy:

```bash
kubectl apply -f netdata-network-policy.yaml
```

### Configure mTLS for Maximum Security

Use client certificates for the highest security level:

Generate a client certificate for Prometheus, then configure nginx to require client certificates:

```nginx
ssl_client_certificate /etc/nginx/ca.crt;
ssl_verify_client on;
```

Configure Prometheus with the client certificate:

```yaml
tls_config:
  cert_file: /etc/prometheus/client.crt
  key_file: /etc/prometheus/client.key
  ca_file: /etc/prometheus/ca.crt
```

### Security Best Practices

Follow these guidelines to secure your Netdata-Prometheus integration:

1. Never expose port 19999 to the public internet without authentication
2. Use TLS for all production deployments
3. Rotate credentials regularly
4. Monitor access logs for suspicious activity
5. Use network segmentation to isolate monitoring infrastructure
6. Disable unnecessary endpoints (dashboard, management API) for scrape-only nodes

### Verify Security Configuration

Test authentication:

```bash
# Should fail without credentials
curl https://netdata.example.com:19999/api/v1/allmetrics

# Should succeed with credentials
curl -u prometheus:password https://netdata.example.com:19999/api/v1/allmetrics
```

Check TLS configuration:

```bash
openssl s_client -connect netdata.example.com:19999 -showcerts
```

## Label Configuration and Management

Configure custom labels to organize and filter your metrics in Prometheus.

### Understanding Netdata Labels

Netdata supports two types of labels:

| Label Type | Source | Examples | Configuration |
|------------|--------|----------|---------------|
| Configured Labels | Defined in `/etc/netdata/netdata.conf` | `environment`, `region`, `team` | `send configured labels = yes` |
| Automatic Labels | Auto-generated by Netdata | `_os_name`, `_architecture`, `_container_id` | `send automatic labels = yes` |

### Configure Custom Labels

Add custom labels in `/etc/netdata/netdata.conf`:

```ini
[host labels]
    environment = production
    region = us-east-1
    datacenter = dc1
    team = platform
    cluster = k8s-prod-01
    cost_center = engineering
```

Enable label export in `/etc/netdata/exporting.conf`:

```ini
[prometheus:exporter]
    send configured labels = yes
    send automatic labels = yes
```

Restart Netdata:

```bash
sudo systemctl restart netdata
```

### Use Labels in Parent-Child Setups

Labels from child nodes propagate automatically when using `format=prometheus_all_hosts`:

```yaml
scrape_configs:
  - job_name: 'netdata-parents'
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus_all_hosts]
    honor_labels: true  # Preserves child labels
    static_configs:
      - targets: ['parent.example.com:19999']
```

Prometheus receives metrics with all labels:

```promql
netdata_system_cpu_percentage_average{
  instance="child-node-01",
  environment="production",
  region="us-east-1",
  team="platform",
  _os_name="linux",
  _architecture="x86_64"
} 5.2
```

### Implement Multi-Tenant Label Strategies

Segregate metrics by tenant using labels:

**Tenant A nodes** (`/etc/netdata/netdata.conf`):
```ini
[host labels]
    tenant = tenant-a
    cost_center = engineering
    sla = gold
```

**Tenant B nodes**:
```ini
[host labels]
    tenant = tenant-b
    cost_center = operations
    sla = silver
```

Query by tenant in Prometheus:

```promql
# Tenant A CPU usage
netdata_system_cpu_percentage_average{tenant="tenant-a"}

# All gold SLA nodes
netdata_system_cpu_percentage_average{sla="gold"}
```

### Follow Label Naming Best Practices

1. Use lowercase with underscores: `cost_center`, not `Cost-Center`
2. Avoid high cardinality: Don't use timestamps, UUIDs, or IP addresses as label values
3. Keep labels stable: Changing labels creates new time series
4. Prefix custom labels: Use descriptive prefixes to avoid conflicts
5. Document your label schema: Maintain a label inventory for your organization

### Combine with Prometheus External Labels

Add additional context at the Prometheus level:

```yaml
global:
  external_labels:
    prometheus_cluster: 'prod-us-east'
    datacenter: 'dc1'
    monitor: 'prometheus-01'
```

This creates a complete labeling hierarchy:

```
Netdata configured labels → Netdata automatic labels → Prometheus job labels → Prometheus external labels
```

### Verify Label Export

Check labels in Prometheus:

```promql
# Show all labels for a metric
netdata_system_cpu_percentage_average

# Count unique label values
count by (environment) (netdata_system_cpu_percentage_average)
```

Test label export from Netdata:

```bash
curl "http://localhost:19999/api/v1/allmetrics?format=prometheus" | grep "environment="
```

## Choosing Between Scraping and Remote Write

Netdata supports two methods for sending metrics to Prometheus. Choose based on your network topology and requirements.

### Compare the Methods

| Aspect | Scraping (Pull) | Remote Write (Push) |
|--------|----------------|---------------------|
| Setup Complexity | Simple (Prometheus config only) | Moderate (both sides) |
| Network Direction | Prometheus → Netdata | Netdata → Prometheus |
| Firewall Friendly | No (requires inbound access) | Yes (outbound only) |
| Buffering on Failure | No (Prometheus retries) | No (data loss possible) |
| Parent-Child Support | Excellent (`prometheus_all_hosts`) | Limited |
| Multi-Prometheus Support | Easy (multiple scrape configs) | Complex (multiple destinations) |
| Resource Usage | Lower (Prometheus controls rate) | Higher (Netdata pushes continuously) |
| Best For | Standard deployments, direct access | Firewalled/NAT environments, edge |

### Use Scraping (Recommended)

Choose scraping if:
- Prometheus can reach Netdata directly (same network, VPN, or cloud)
- You have a Parent-Child architecture (scrape Parents)
- You need to scrape from multiple Prometheus instances
- You want Prometheus to control scrape frequency
- You need reliable data delivery

Common scenarios: Standard Kubernetes deployments, datacenter monitoring, cloud VPC environments

### Use Remote Write

Choose remote write if:
- Netdata nodes are behind NAT or firewall
- Prometheus cannot initiate connections to Netdata
- You need to push to a remote Prometheus instance
- You're using Prometheus-compatible services (Cortex, Thanos, Mimir)

Common scenarios: IoT devices, edge locations, air-gapped networks

:::warning

Remote write has no buffering on failure. Data loss occurs if Prometheus is unavailable during the push.

:::

### Migrate from Remote Write to Scraping

Switch from remote write to scraping without data loss:

1. Add scraping configuration to Prometheus (keep remote write active)
2. Verify metrics appear correctly through scraping
3. Monitor for 24 hours to ensure no gaps
4. Disable remote write in Netdata configuration
5. Restart Netdata to free resources

### Configuration Examples

**Scraping** (recommended):
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'netdata'
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus]
    static_configs:
      - targets: ['netdata1:19999', 'netdata2:19999']
```

**Remote Write** (see [Remote Write documentation](/src/exporting/prometheus/remote_write/README.md)):
```ini
# /etc/netdata/exporting.conf
[prometheus_remote_write:my_instance]
    enabled = yes
    destination = prometheus.example.com:9090
    remote write URL path = /api/v1/write
```

:::tip

Use scraping for 95% of deployments. Only use remote write when network topology absolutely requires it.

:::

## Troubleshooting

Resolve common issues when integrating Netdata with Prometheus.

### Verify Metrics Export

Test the endpoint manually:

```bash
curl -s "http://localhost:19999/api/v1/allmetrics?format=prometheus&help=yes" | head -50
```

You should see output like:

```
# COMMENT netdata_system_cpu_percentage_average: dimension "user"...
netdata_system_cpu_percentage_average{chart="system.cpu",family="cpu",dimension="user"} 2.5
```

### Common Issues

<details>
<summary><strong>Prometheus Shows "Context Deadline Exceeded"</strong></summary>

**Cause**: Scrape timeout too short for large metric volumes.

**Solution**: Increase timeout in `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'netdata'
    scrape_timeout: 30s  # Increased from default 10s
    scrape_interval: 30s # Must be >= scrape_timeout
```

</details>

<details>
<summary><strong>Missing Metrics in Prometheus</strong></summary>

**Cause**: Metrics filtered by `send charts matching` configuration.

**Solution**: Check Netdata configuration:

```bash
grep "send charts matching" /etc/netdata/exporting.conf
```

Adjust the filter in `/etc/netdata/exporting.conf`:

```ini
[prometheus:exporter]
    send charts matching = *  # Export all charts
```

Restart Netdata:

```bash
sudo systemctl restart netdata
```

</details>

<details>
<summary><strong>High Memory Usage in Prometheus</strong></summary>

**Cause**: High cardinality from Netdata labels or too many dimensions.

**Solutions**:

1. Switch to `as-collected` source:
```yaml
params:
  source: [as-collected]
```

2. Filter metrics in Netdata:
```ini
[prometheus:exporter]
    send charts matching = system.* disk.io* net.* !*tmp* !*cache*
```

3. Use metric relabeling in Prometheus:
```yaml
metric_relabel_configs:
  - source_labels: [dimension]
    regex: '(.*_tmp_.*|.*_cache_.*)'
    action: drop
```

</details>

<details>
<summary><strong>Stale Metrics or Gaps in Data</strong></summary>

**Cause**: Netdata restart, network issues, or Prometheus scrape failures.

**Solution**: Check Netdata status:

```bash
sudo systemctl status netdata
tail -50 /var/log/netdata/error.log
```

Verify Prometheus scrape status in Prometheus UI at **Status → Targets**.

</details>

<details>
<summary><strong>Instance Labels Show IP Instead of Hostname</strong></summary>

**Cause**: Using `format=prometheus` instead of `format=prometheus_all_hosts` in Parent-Child setups.

**Solution**: Use the correct format:

```yaml
params:
  format: [prometheus_all_hosts]
honor_labels: true
```

</details>

### Enable Debug Logging

Edit `/etc/netdata/netdata.conf`:

```ini
[global]
    debug flags = 0x0000000000000001
```

Restart Netdata and monitor the debug log:

```bash
sudo systemctl restart netdata
tail -f /var/log/netdata/debug.log
```

:::warning

Debug logging generates large log files. Disable it after troubleshooting by setting `debug flags = 0x0000000000000000`.

:::

### Check Prometheus Scrape Details

Query Prometheus for scrape metrics:

```promql
# Scrape duration (should be < scrape_timeout)
scrape_duration_seconds{job="netdata"}

# Samples scraped per scrape
scrape_samples_scraped{job="netdata"}
```
