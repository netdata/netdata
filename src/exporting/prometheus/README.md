# Using Netdata with Prometheus

Netdata supports exporting metrics to Prometheus in two ways:

 - You can [configure Prometheus to scrape Netdata metrics](#configure-prometheus-to-scrape-netdata-metrics).

 - You can [configure Netdata to push metrics to Prometheus](/src/exporting/prometheus/remote_write/README.md)
   , using the Prometheus remote write API.

## Netdata support for Prometheus

Regardless of the methodology, you first need to understand how Netdata structures the metrics it exports to Prometheus
and the capabilities it provides. The examples provided in this document assume that you will be using Netdata as 
a metrics endpoint, but the concepts apply as well to the remote write API method.

### Understanding Netdata metrics

#### Charts

Each chart in Netdata has several properties (common to all its metrics):

- `chart_id` - uniquely identifies a chart.

- `chart_name` - a more human friendly name for `chart_id`, also unique.

- `context` - this is the template of the chart. All disk I/O charts have the same context, all mysql requests charts
  have the same context, etc. This is used for alert templates to match all the charts they should be attached to.

- `family` groups a set of charts together. It is used as the submenu of the dashboard.

- `units` is the units for all the metrics attached to the chart.

#### Dimensions

Then each Netdata chart contains metrics called `dimensions`. All the dimensions of a chart have the same units of
measurement, and are contextually in the same category (ie. the metrics for disk bandwidth are `read` and `write` and
they are both in the same chart).

### Netdata data source

Netdata can send metrics to Prometheus from 3 data sources:

- `as collected` or `raw` - this data source sends the metrics to Prometheus as they are collected. No conversion is
  done by Netdata. The latest value for each metric is just given to Prometheus. This is the most preferred method by
  Prometheus, but it is also the harder to work with. To work with this data source, you will need to understand how
  to get meaningful values out of them.

  The format of the metrics is: `CONTEXT{chart="CHART",family="FAMILY",dimension="DIMENSION"}`.

  If the metric is a counter (`incremental` in Netdata lingo), `_total` is appended the context.

  Unlike Prometheus, Netdata allows each dimension of a chart to have a different algorithm and conversion constants
  (`multiplier` and `divisor`). In this case, that the dimensions of a charts are heterogeneous, Netdata will use this
  format: `CONTEXT_DIMENSION{chart="CHART",family="FAMILY"}`

- `average` - this data source uses the Netdata database to send the metrics to Prometheus as they are presented on
  the Netdata dashboard. So, all the metrics are sent as gauges, at the units they are presented in the Netdata
  dashboard charts. This is the easiest to work with.

  The format of the metrics is: `CONTEXT_UNITS_average{chart="CHART",family="FAMILY",dimension="DIMENSION"}`.

  When this source is used, Netdata keeps track of the last access time for each Prometheus server fetching the
  metrics. This last access time is used at the subsequent queries of the same Prometheus server to identify the
  time-frame the `average` will be calculated.

  So, no matter how frequently Prometheus scrapes Netdata, it will get all the database data.
  To identify each Prometheus server, Netdata uses by default the IP of the client fetching the metrics.

  If there are multiple Prometheus servers fetching data from the same Netdata, using the same IP, each Prometheus
  server can append `server=NAME` to the URL. Netdata will use this `NAME` to uniquely identify the Prometheus server.

- `sum` or `volume`, is like `average` but instead of averaging the values, it sums them.

  The format of the metrics is: `CONTEXT_UNITS_sum{chart="CHART",family="FAMILY",dimension="DIMENSION"}`. All the
  other operations are the same with `average`.

  To change the data source to `sum` or `as-collected` you need to provide the `source` parameter in the request URL.
  e.g.: `http://your.netdata.ip:19999/api/v1/allmetrics?format=prometheus&help=yes&source=as-collected`

  Keep in mind that early versions of Netdata were sending the metrics as: `CHART_DIMENSION{}`.

### Querying Metrics

Fetch with your web browser this URL:

`http://your.netdata.ip:19999/api/v1/allmetrics?format=prometheus&help=yes`

_(replace `your.netdata.ip` with the ip or hostname of your Netdata server)_

Netdata will respond with all the metrics it sends to Prometheus.

If you search that page for `"system.cpu"` you will find all the metrics Netdata is exporting to Prometheus for this
chart. `system.cpu` is the chart name on the Netdata dashboard (on the Netdata dashboard all charts have a text heading
such as : `Total CPU utilization (system.cpu)`. What we are interested here in the chart name: `system.cpu`).

Searching for `"system.cpu"` reveals:

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

_(Netdata response for `system.cpu` with source=`average`)_

In `average` or `sum` data sources, all values are normalized and are reported to Prometheus as gauges. Now, use the
'expression' text form in Prometheus. Begin to type the metrics we are looking for: `netdata_system_cpu`. You should see
that the text form begins to auto-fill as Prometheus knows about this metric.

If the data source was `as collected`, the response would be:

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

_(Netdata response for `system.cpu` with source=`as-collected`)_

For more information check Prometheus documentation.

### Streaming data from upstream hosts

The `format=prometheus` parameter only exports the host's Netdata metrics. If you are using the parent-child
functionality of Netdata this ignores any upstream hosts - so you should consider using the below in your
**prometheus.yml**:

```yaml
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [ prometheus_all_hosts ]
    honor_labels: true
```

This will report all upstream host data, and `honor_labels` will make Prometheus take note of the instance names
provided.

### Timestamps

To pass the metrics through Prometheus pushgateway, Netdata supports the option `&timestamps=no` to send the metrics
without timestamps.

## Netdata host variables

Netdata collects various system configuration metrics, like the max number of TCP sockets supported, the max number of
files allowed system-wide, various IPC sizes, etc. These metrics are not exposed to Prometheus by default.

To expose them, append `variables=yes` to the Netdata URL.

### TYPE and HELP

To save bandwidth, and because Prometheus does not use them anyway, `# TYPE` and `# HELP` lines are suppressed. If
wanted they can be re-enabled via `types=yes` and `help=yes`, e.g.
`/api/v1/allmetrics?format=prometheus&types=yes&help=yes`

Note that if enabled, the `# TYPE` and `# HELP` lines are repeated for every occurrence of a metric, which goes against
the Prometheus
documentation's [specification for these lines](https://github.com/prometheus/docs/blob/master/content/docs/instrumenting/exposition_formats.md#comments-help-text-and-type-information).

### Names and IDs

Netdata supports names and IDs for charts and dimensions. Usually IDs are unique identifiers as read by the system and
names are human friendly labels (also unique).

Most charts and metrics have the same ID and name, but in several cases they are different: disks with device-mapper,
interrupts, QoS classes, statsd synthetic charts, etc.

The default is controlled in `exporting.conf`:

```text
[prometheus:exporter]
	send names instead of ids = yes | no
```

You can overwrite it from Prometheus, by appending to the URL:

- `&names=no` to get IDs (the old behaviour)
- `&names=yes` to get names

### Filtering metrics sent to Prometheus

Netdata can filter the metrics it sends to Prometheus with this setting:

```text
[prometheus:exporter]
	send charts matching = *
```

This settings accepts a space separated list
of [simple patterns](/src/libnetdata/simple_pattern/README.md) to match the
**charts** to be sent to Prometheus. Each pattern can use `*` as wildcard, any number of times (e.g `*a*b*c*` is valid).
Patterns starting with `!` give a negative match (e.g `!*.bad users.* groups.*` will send all the users and groups
except `bad` user and `bad` group). The order is important: the first match (positive or negative) left to right, is
used.

### Changing the prefix of Netdata metrics

Netdata sends all metrics prefixed with `netdata_`. You can change this in `netdata.conf`, like this:

```text
[prometheus:exporter]
	prefix = netdata
```

It can also be changed from the URL, by appending `&prefix=netdata`.

### Metric Units

The default source `average` adds the unit of measurement to the name of each metric (e.g. `_KiB_persec`). To hide the
units and get the same metric names as with the other sources, append to the URL `&hideunits=yes`.

The units were standardized in v1.12, with the effect of changing the metric names. To get the metric names as they were
before v1.12, append to the URL `&oldunits=yes`

### Accuracy of `average` and `sum` data sources

When the data source is set to `average` or `sum`, Netdata remembers the last access of each client accessing Prometheus
metrics and uses this last access time to respond with the `average` or `sum` of all the entries in the database since
that. This means that Prometheus servers are not losing data when they access Netdata with data source = `average` or
`sum`.

To uniquely identify each Prometheus server, Netdata uses the IP of the client accessing the metrics. If however the IP
is not good enough for identifying a single Prometheus server (e.g. when Prometheus servers are accessing Netdata
through a web proxy, or when multiple Prometheus servers are NATed to a single IP), each Prometheus may append
`&server=NAME` to the URL. This `NAME` is used by Netdata to uniquely identify each Prometheus server and keep track of
its last access time.

## Configure Prometheus to scrape Netdata metrics

The following `prometheus.yml` file will scrape all netdata metrics "as collected". 

Make sure to replace `your.netdata.ip` with the IP or hostname of the host running Netdata.

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
      - targets: [ '0.0.0.0:9090' ]

  - job_name: 'netdata-scrape'

    metrics_path: '/api/v1/allmetrics'
    params:
      # format: prometheus | prometheus_all_hosts
      # You can use `prometheus_all_hosts` if you want Prometheus to set the `instance` to your hostname instead of IP 
      format: [ prometheus ]
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
      - targets: [ '{your.netdata.ip}:19999' ]
```

### Prometheus alerts for Netdata metrics

The following is an example of a `nodes.yml` file that will allow Prometheus to generate alerts from some Netdata sources. 
Save it at `/opt/prometheus/nodes.yml`, and add a _- "nodes.yml"_ entry under the _rule_files:_ section in the example prometheus.yml file above.

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
