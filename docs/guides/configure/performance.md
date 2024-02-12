# How to optimize the Netdata Agent's performance

We designed the Netdata Agent to be incredibly lightweight, even when it's collecting a few thousand dimensions every
second and visualizing that data into hundreds of charts. However, the default settings of the Netdata Agent are not
optimized for performance, but for a simple, standalone setup. We want the first install to give you something you can
run without any configuration. Most of the settings and options are enabled, since we want you to experience the full
thing.

By default, Netdata will automatically detect applications running on the node it is installed to start collecting
metrics in real-time, has health monitoring enabled to evaluate alerts and trains Machine Learning (ML) models for each
metric, to detect anomalies.

This document describes the resources required for the various default capabilities and the strategies to optimize
Netdata for production use.

## Summary of performance optimizations

The following table summarizes the effect of each optimization on the CPU, RAM and Disk IO utilization in production.

| Optimization                                                                                                                  | CPU                | RAM                | Disk IO            |
|-------------------------------------------------------------------------------------------------------------------------------|--------------------|--------------------|--------------------|
| [Use streaming and replication](#use-streaming-and-replication)                                                               | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Disable unneeded plugins or collectors](#disable-unneeded-plugins-or-collectors)                                             | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Reduce data collection frequency](#reduce-collection-frequency)                                                              | :heavy_check_mark: |                    | :heavy_check_mark: |
| [Change how long Netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md) |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Use a different metric storage database](https://github.com/netdata/netdata/blob/master/src/database/README.md)              |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Disable machine learning](#disable-machine-learning)                                                                         | :heavy_check_mark: |                    |                    |
| [Use a reverse proxy](#run-netdata-behind-a-proxy)                                                                            | :heavy_check_mark: |                    |                    |
| [Disable/lower gzip compression for the agent dashboard](#disablelower-gzip-compression-for-the-dashboard)                    | :heavy_check_mark: |                    |                    |

## Resources required by a default Netdata installation

Netdata's performance is primarily affected by **data collection/retention** and **clients accessing data**.

You can configure almost all aspects of data collection/retention, and certain aspects of clients accessing data.

### CPU consumption

Expect about:

- 1-3% of a single core for the netdata core
- 1-3% of a single core for the various collectors (e.g. go.d.plugin, apps.plugin)
- 5-10% of a single core, when ML training runs

Your experience may vary depending on the number of metrics collected, the collectors enabled and the specific
environment they run on, i.e. the work they have to do to collect these metrics.

As a general rule, for modern hardware and VMs, the total CPU consumption of a standalone Netdata installation,
including all its components, should be below 5 - 15% of a single core. For example, on 8 core server it will use only
0.6% - 1.8% of a total CPU capacity, depending on the CPU characteristics.

The Netdata Agent runs with the lowest
possible [process scheduling policy](https://github.com/netdata/netdata/blob/master/src/daemon/README.md#netdata-process-scheduling-policy),
which is `nice 19`, and uses the `idle` process scheduler. Together, these settings ensure that the Agent only gets CPU
resources when the node has CPU resources to space. If the node reaches 100% CPU utilization, the Agent is stopped first
to ensure your applications get any available resources.

To reduce CPU usage you can (either one or a combination of the following actions):

1. [Disable machine learning](#disable-machine-learning),
2. [Use streaming and replication](#use-streaming-and-replication),
3. [Reduce the data collection frequency](#reduce-collection-frequency)
4. [Disable unneeded plugins or collectors](#disable-unneeded-plugins-or-collectors)
5. [Use a reverse proxy](#run-netdata-behind-a-proxy),
6. [Disable/lower gzip compression for the agent dashboard](#disablelower-gzip-compression-for-the-dashboard).

### Memory consumption

The memory footprint of Netdata is mainly influenced by the number of metrics concurrently being collected. Expect about
150MB of RAM for a typical 64-bit server collecting about 2000 to 3000 metrics.

To estimate and control memory consumption, you can (either one or a combination of the following actions):

1. [Disable unneeded plugins or collectors](#disable-unneeded-plugins-or-collectors)
2. [Change how long Netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md)
3. [Use a different metric storage database](https://github.com/netdata/netdata/blob/master/src/database/README.md).

### Disk footprint and I/O

By default, Netdata should not use more than 1GB of disk space, most of which is dedicated for storing metric data and
metadata. For typical installations collecting 2000 - 3000 metrics, this storage should provide a few days of
high-resolution retention (per second), about a month of mid-resolution retention (per minute) and more than a year of
low-resolution retention (per hour).

Netdata spreads I/O operations across time. For typical standalone installations there should be a few write operations
every 5-10 seconds of a few kilobytes each, occasionally up to 1MB. In addition, under heavy load, collectors that
require disk I/O may stop and show gaps in charts.

To optimize your disk footprint in any aspect described below you can:


To configure retention, you can: 

1. [Change how long Netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md).

To control disk I/O:

1. [Use a different metric storage database](https://github.com/netdata/netdata/blob/master/src/database/README.md),


Minimize deployment impact on the production system by optimizing disk footprint:

1. [Using streaming and replication](#use-streaming-and-replication)
2. [Reduce the data collection frequency](#reduce-collection-frequency)
3. [Disable unneeded plugins or collectors](#disable-unneeded-plugins-or-collectors).

## Use streaming and replication

For all production environments, parent Netdata nodes outside the production infrastructure should be receiving all
collected data from children Netdata nodes running on the production infrastructure,
using [streaming and replication](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md).

### Disable health checks on the child nodes

When you set up streaming, we recommend you run your health checks on the parent. This saves resources on the children
and makes it easier to configure or disable alerts and agent notifications.

The parents by default run health checks for each child, as long as the child is connected (the details are
in `stream.conf`). On the child nodes you should add to `netdata.conf` the following:

```conf
[health]
   enabled = no
```

### Use memory mode ram for the child nodes

See [using a different metric storage database](https://github.com/netdata/netdata/blob/master/src/database/README.md).

## Disable unneeded plugins or collectors

If you know that you don't need an [entire plugin or a specific
collector](https://github.com/netdata/netdata/blob/master/src/collectors/README.md#collector-architecture-and-terminology),
you can disable any of them. Keep in mind that if a plugin/collector has nothing to do, it simply shuts down and does
not consume system resources. You will only improve the Agent's performance by disabling plugins/collectors that are
actively collecting metrics.

Open `netdata.conf` and scroll down to the `[plugins]` section. To disable any plugin, uncomment it and set the value to
`no`. For example, to explicitly keep the `proc` and `go.d` plugins enabled while disabling `python.d` and `charts.d`.

```conf
[plugins]
    proc = yes
	python.d = no
	charts.d = no
	go.d = yes
```

Disable specific collectors by opening their respective plugin configuration files, uncommenting the line for the
collector, and setting its value to `no`.

```bash
sudo ./edit-config go.d.conf
sudo ./edit-config python.d.conf
sudo ./edit-config charts.d.conf
```

For example, to disable a few Python collectors:

```conf
modules:
  apache: no
	dockerd: no
	fail2ban: no
```

## Reduce collection frequency

The fastest way to improve the Agent's resource utilization is to reduce how often it collects metrics.

### Global

If you don't need per-second metrics, or if the Netdata Agent uses a lot of CPU even when no one is viewing that node's
dashboard, [configure the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) to collect
metrics less often.

Open `netdata.conf` and edit the `update every` setting. The default is `1`, meaning that the Agent collects metrics
every second.

If you change this to `2`, Netdata enforces a minimum `update every` setting of 2 seconds, and collects metrics every
other second, which will effectively halve CPU utilization. Set this to `5` or `10` to collect metrics every 5 or 10
seconds, respectively.

```conf
[global]
    update every = 5
```

### Specific plugin or collector

Every collector and plugin has its own `update every` setting, which you can also change in the `go.d.conf`,
`python.d.conf`, or `charts.d.conf` files, or in individual collector configuration files. If the `update
every` for an individual collector is less than the global, the Netdata Agent uses the global setting. See
the [collectors configuration reference](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md) for
details.

To reduce the frequency of
an [internal_plugin/collector](https://github.com/netdata/netdata/blob/master/src/collectors/README.md#collector-architecture-and-terminology),
open `netdata.conf` and find the appropriate section. For example, to reduce the frequency of the `apps` plugin, which
collects and visualizes metrics on application resource utilization:

```conf
[plugin:apps]
    update every = 5
```

To [configure an individual collector](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md#configure-a-collector),
open its specific configuration file with `edit-config` and look for the `update_every` setting. For example, to reduce
the frequency of the `nginx` collector, run `sudo ./edit-config go.d/nginx.conf`:

```conf
# [ GLOBAL ]
update_every: 10
```

## Lower memory usage for metrics retention

See how
to [change how long Netdata stores metrics](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md).

## Use a different metric storage database

Consider [using a different metric storage database](https://github.com/netdata/netdata/blob/master/src/database/README.md)
when running Netdata on IoT devices, and for children in a parent-child set up based
on [streaming and replication](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md).

## Disable machine learning

Automated anomaly detection may be a powerful tool, but we recommend it to only be enabled on Netdata parents that sit
outside your production infrastructure, or if you have cpu and memory to spare. You can disable ML with the following:

```conf
[ml]
   enabled = no
```

## Run Netdata behind a proxy

A dedicated web server like nginx provides more robustness than the Agent's
internal [web server](https://github.com/netdata/netdata/blob/master/src/web/README.md).
Nginx can handle more concurrent connections, reuse idle connections, and use fast gzip compression to reduce payloads.

For details on installing another web server as a proxy for the local Agent dashboard,
see [reverse proxies](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/reverse-proxies.md).

## Disable/lower gzip compression for the dashboard

If you choose not to run the Agent behind Nginx, you can disable or lower the Agent's web server's gzip compression.
While gzip compression does reduce the size of the HTML/CSS/JS payload, it does use additional CPU while a user is
looking at the local Agent dashboard.

To disable gzip compression, open `netdata.conf` and find the `[web]` section:

```conf
[web]
    enable gzip compression = no
```

Or to lower the default compression level:

```conf
[web]
    enable gzip compression = yes
    gzip compression level = 1
```

