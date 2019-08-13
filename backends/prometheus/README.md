# Using Netdata with Prometheus

> IMPORTANT: the format Netdata sends metrics to prometheus has changed since Netdata v1.7. The new prometheus backend for Netdata supports a lot more features and is aligned to the development of the rest of the Netdata backends.

Prometheus is a distributed monitoring system which offers a very simple setup along with a robust data model. Recently Netdata added support for Prometheus. I'm going to quickly show you how to install both Netdata and prometheus on the same server. We can then use grafana pointed at Prometheus to obtain long term metrics Netdata offers. I'm assuming we are starting at a fresh ubuntu shell (whether you'd like to follow along in a VM or a cloud instance is up to you).


## Installing Netdata and prometheus

### Installing Netdata

There are number of ways to install Netdata according to [Installation](../../packaging/installer/#installation)  
The suggested way of installing the latest Netdata and keep it upgrade automatically. Using one line installation:

```
bash <(curl -Ss https://my-netdata.io/kickstart.sh)
```

At this point we should have Netdata listening on port 19999. Attempt to take your browser here:

```
http://your.netdata.ip:19999
```

*(replace `your.netdata.ip` with the IP or hostname of the server running Netdata)*

### Installing Prometheus

In order to install prometheus we are going to introduce our own systemd startup script along with an example of prometheus.yaml configuration. Prometheus needs to be pointed to your server at a specific target url for it to scrape Netdata's api. Prometheus is always a pull model meaning Netdata is the passive client within this architecture. Prometheus always initiates the connection with Netdata.

#### Download Prometheus

```sh
wget -O /tmp/prometheus-2.3.2.linux-amd64.tar.gz https://github.com/prometheus/prometheus/releases/download/v2.3.2/prometheus-2.3.2.linux-amd64.tar.gz
```

#### Create prometheus system user

```sh
sudo useradd -r prometheus
```

#### Create prometheus directory

```sh
sudo mkdir /opt/prometheus
sudo chown prometheus:prometheus /opt/prometheus
```

#### Untar prometheus directory

```sh
sudo tar -xvf /tmp/prometheus-2.3.2.linux-amd64.tar.gz -C /opt/prometheus --strip=1
```

#### Install prometheus.yml

We will use the following `prometheus.yml` file. Save it at `/opt/prometheus/prometheus.yml`.

Make sure to replace `your.netdata.ip` with the IP or hostname of the host running Netdata. 

```yaml
# my global config
global:
  scrape_interval:     5s # Set the scrape interval to every 5 seconds. Default is every 1 minute.
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

#### Install nodes.yml

The following is completely optional, it will enable Prometheus to generate alerts from some NetData sources. Tweak the values to your own needs. We will use the following `nodes.yml` file below. Save it at `/opt/prometheus/nodes.yml`, and add a *- "nodes.yml"* entry under the *rule_files:* section in the example prometheus.yml file above.
```
groups:
- name: nodes

  rules:
  - alert: node_high_cpu_usage_70
    expr: avg(rate(netdata_cpu_cpu_percentage_average{dimension="idle"}[1m])) by (job) > 70
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

#### Install prometheus.service

Save this service file as `/etc/systemd/system/prometheus.service`:

```
[Unit]
Description=Prometheus Server
AssertPathExists=/opt/prometheus

[Service]
Type=simple
WorkingDirectory=/opt/prometheus
User=prometheus
Group=prometheus
ExecStart=/opt/prometheus/prometheus --config.file=/opt/prometheus/prometheus.yml --log.level=info
ExecReload=/bin/kill -SIGHUP $MAINPID
ExecStop=/bin/kill -SIGINT $MAINPID

[Install]
WantedBy=multi-user.target
```
##### Start Prometheus

```
sudo systemctl start prometheus
sudo systemctl enable prometheus
```

Prometheus should now start and listen on port 9090. Attempt to head there with your browser. 

If everything is working correctly when you fetch `http://your.prometheus.ip:9090` you will see a 'Status' tab. Click this and click on 'targets' We should see the Netdata host as a scraped target. 

---

## Netdata support for prometheus

> IMPORTANT: the format Netdata sends metrics to prometheus has changed since Netdata v1.6. The new format allows easier queries for metrics and supports both `as collected` and normalized metrics.

Before explaining the changes, we have to understand the key differences between Netdata and prometheus.

### understanding Netdata metrics

##### charts

Each chart in Netdata has several properties (common to all its metrics):

- `chart_id` - uniquely identifies a chart.

- `chart_name` - a more human friendly name for `chart_id`, also unique.

- `context` - this is the template of the chart. All disk I/O charts have the same context, all mysql requests charts have the same context, etc. This is used for alarm templates to match all the charts they should be attached to.

- `family` groups a set of charts together. It is used as the submenu of the dashboard.

- `units` is the units for all the metrics attached to the chart.

##### dimensions

Then each Netdata chart contains metrics called `dimensions`. All the dimensions of a chart have the same units of measurement, and are contextually in the same category (ie. the metrics for disk bandwidth are `read` and `write` and they are both in the same chart).

### Netdata data source

Netdata can send metrics to prometheus from 3 data sources:

- `as collected` or `raw` - this data source sends the metrics to prometheus as they are collected. No conversion is done by Netdata. The latest value for each metric is just given to prometheus. This is the most preferred method by prometheus, but it is also the harder to work with. To work with this data source, you will need to understand how to get meaningful values out of them.

   The format of the metrics is: `CONTEXT{chart="CHART",family="FAMILY",dimension="DIMENSION"}`.

   If the metric is a counter (`incremental` in Netdata lingo), `_total` is appended the context.

   Unlike prometheus, Netdata allows each dimension of a chart to have a different algorithm and conversion constants (`multiplier` and `divisor`). In this case, that the dimensions of a charts are heterogeneous, Netdata will use this format: `CONTEXT_DIMENSION{chart="CHART",family="FAMILY"}`

- `average` - this data source uses the Netdata database to send the metrics to prometheus as they are presented on the Netdata dashboard. So, all the metrics are sent as gauges, at the units they are presented in the Netdata dashboard charts. This is the easiest to work with.

   The format of the metrics is: `CONTEXT_UNITS_average{chart="CHART",family="FAMILY",dimension="DIMENSION"}`.

   When this source is used, Netdata keeps track of the last access time for each prometheus server fetching the metrics. This last access time is used at the subsequent queries of the same prometheus server to identify the time-frame the `average` will be calculated. So, no matter how frequently prometheus scrapes Netdata, it will get all the database data. To identify each prometheus server, Netdata uses by default the IP of the client fetching the metrics. If there are multiple prometheus servers fetching data from the same Netdata, using the same IP, each prometheus server can append `server=NAME` to the URL. Netdata will use this `NAME` to uniquely identify the prometheus server.

- `sum` or `volume`, is like `average` but instead of averaging the values, it sums them.

   The format of the metrics is: `CONTEXT_UNITS_sum{chart="CHART",family="FAMILY",dimension="DIMENSION"}`.
   All the other operations are the same with `average`. 

Keep in mind that early versions of Netdata were sending the metrics as: `CHART_DIMENSION{}`.

### Querying Metrics

Fetch with your web browser this URL:

`http://your.netdata.ip:19999/api/v1/allmetrics?format=prometheus&help=yes`

*(replace `your.netdata.ip` with the ip or hostname of your Netdata server)*

Netdata will respond with all the metrics it sends to prometheus.

If you search that page for `"system.cpu"` you will find all the metrics Netdata is exporting to prometheus for this chart.  `system.cpu` is the chart name on the Netdata dashboard (on the Netdata dashboard all charts have a text heading such as : `Total CPU utilization (system.cpu)`. What we are interested here in the chart name: `system.cpu`).

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
*(Netdata response for `system.cpu` with source=`average`)*

In `average` or `sum` data sources, all values are normalized and are reported to prometheus as gauges. Now, use the 'expression' text form in prometheus. Begin to type the metrics we are looking for: `netdata_system_cpu`. You should see that the text form begins to auto-fill as prometheus knows about this metric.

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

*(Netdata response for `system.cpu` with source=`as-collected`)*

For more information check prometheus documentation.

### Streaming data from upstream hosts

The `format=prometheus` parameter only exports the host's Netdata metrics.  If you are using the master/slave functionality of Netdata this ignores any upstream hosts - so you should consider using the below in your **prometheus.yml**:

```
    metrics_path: '/api/v1/allmetrics'
    params:
      format: [prometheus_all_hosts]
    honor_labels: true
```

This will report all upstream host data, and `honor_labels` will make Prometheus take note of the instance names provided.

### Timestamps

To pass the metrics through prometheus pushgateway, Netdata supports the option `&timestamps=no` to send the metrics without timestamps.

## Netdata host variables

Netdata collects various system configuration metrics, like the max number of TCP sockets supported, the max number of files allowed system-wide, various IPC sizes, etc. These metrics are not exposed to prometheus by default.

To expose them, append `variables=yes` to the Netdata URL.

### TYPE and HELP

To save bandwidth, and because prometheus does not use them anyway, `# TYPE` and `# HELP` lines are suppressed. If wanted they can be re-enabled via `types=yes` and `help=yes`, e.g. `/api/v1/allmetrics?format=prometheus&types=yes&help=yes`

### Names and IDs

Netdata supports names and IDs for charts and dimensions. Usually IDs are unique identifiers as read by the system and names are human friendly labels (also unique).

Most charts and metrics have the same ID and name, but in several cases they are different: disks with device-mapper, interrupts, QoS classes, statsd synthetic charts, etc.

The default is controlled in `netdata.conf`:

```
[backend]
	send names instead of ids = yes | no
```

You can overwrite it from prometheus, by appending to the URL:

* `&names=no` to get IDs (the old behaviour)
* `&names=yes` to get names

### Filtering metrics sent to prometheus

Netdata can filter the metrics it sends to prometheus with this setting:

```
[backend]
	send charts matching = *
```

This settings accepts a space separated list of patterns to match the **charts** to be sent to prometheus. Each pattern can use ` * ` as wildcard, any number of times (e.g `*a*b*c*` is valid). Patterns starting with ` ! ` give a negative match (e.g `!*.bad users.* groups.*` will send all the users and groups except `bad` user and `bad` group). The order is important: the first match (positive or negative) left to right, is used.

### Changing the prefix of Netdata metrics

Netdata sends all metrics prefixed with `netdata_`. You can change this in `netdata.conf`, like this:

```
[backend]
	prefix = netdata
```

It can also be changed from the URL, by appending `&prefix=netdata`.

### Metric Units

The default source `average` adds the unit of measurement to the name of each metric (e.g. `_KiB_persec`).
To hide the units and get the same metric names as with the other sources, append to the URL `&hideunits=yes`.

The units were standardized in v1.12, with the effect of changing the metric names. 
To get the metric names as they were before v1.12, append to the URL `&oldunits=yes`

### Accuracy of `average` and `sum` data sources

When the data source is set to `average` or `sum`, Netdata remembers the last access of each client accessing prometheus metrics and uses this last access time to respond with the `average` or `sum` of all the entries in the database since that. This means that prometheus servers are not losing data when they access Netdata with data source = `average` or `sum`.

To uniquely identify each prometheus server, Netdata uses the IP of the client accessing the metrics. If however the IP is not good enough for identifying a single prometheus server (e.g. when prometheus servers are accessing Netdata through a web proxy, or when multiple prometheus servers are NATed to a single IP), each prometheus may append `&server=NAME` to the URL. This `NAME` is used by Netdata to uniquely identify each prometheus server and keep track of its last access time.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fbackends%2Fprometheus%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
