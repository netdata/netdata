<!--
title: How to optimize the Netdata Agent's performance
description: "While the Netdata Agent is designed to monitor a system with only 1% CPU, you can optimize its performance for low-resource systems."
image: /img/seo/guides/configure/performance.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/configure/performance.md
-->

# How to optimize the Netdata Agent's performance

We designed the Netdata Agent to be incredibly lightweight, even when it's collecting a few thousand dimensions every
second and visualizing that data into hundreds of charts. The Agent itself should never use more than 1% of a single CPU
core, roughly 100 MiB of RAM, and minimal disk I/O to collect, store, and visualize all this data.
  
We take this scalability seriously. We have one user [running
Netdata](https://github.com/netdata/netdata/issues/1323#issuecomment-266427841) on a system with 144 cores and 288
threads. Despite collecting 100,000 metrics every second, the Agent still only uses 9% CPU utilization on a
single core.

But not everyone has such powerful systems at their disposal. For example, you might run the Agent on a cloud VM with
only 512 MiB of RAM, or an IoT device like a [Raspberry Pi](/docs/guides/monitor/pi-hole-raspberry-pi.md). In these
cases, reducing Netdata's footprint beyond its already diminuitive size can pay big dividends, giving your services more
horsepower while still monitoring the health and the performance of the node, OS, hardware, and applications.

## Prerequisites

-   A node running the Netdata Agent.
-   Familiarity with configuring the Netdata Agent with `edit-config`.

If you're not familiar with how to configure the Netdata Agent, read our [node configuration
doc](/docs/configure/nodes.md) before continuing with this guide. This guide assumes familiarity with the Netdata config
directory, using `edit-config`, and the process of uncommenting/editing various settings in `netdata.conf` and other
configuration files.

## What affects Netdata's performance?

Netdata's performance is primarily affected by **data collection/retention** and **clients accessing data**. 

You can configure almost all aspects of data collection/retention, and certain aspects of clients accessing data. For
example, you can't control how many users might be viewing a local Agent dashboard, [viewing an
infrastructure](/docs/visualize/overview-infrastructure.md) in real-time with Netdata Cloud, or running [Metric
Correlations](https://learn.netdata.cloud/docs/cloud/insights/metric-correlations).

The Netdata Agent runs with the lowest possible [process scheduling
policy](/daemon/README.md#netdata-process-scheduling-policy), which is `nice 19`, and uses the `idle` process scheduler.
Together, these settings ensure that the Agent only gets CPU resources when the node has CPU resources to space. If the
node reaches 100% CPU utilization, the Agent is stopped first to ensure your applications get any available resources.
In addition, under heavy load, collectors that require disk I/O may stop and show gaps in charts.

Let's walk through the best ways to improve the Netdata Agent's performance.

## Reduce collection frequency

The fastest way to improve the Agent's resource utilization is to reduce how often it collects metrics.

## Global

If you don't need per-second metrics, or if the Agent uses a lot of CPU even when no one is viewing that node's
dashboard, configure the Agent to collect metrics less often.

Open `netdata.conf` and edit the `update every` setting. The default is `1`, meaning that the Agent updates every
second.

If you change this to `2`, Netdata collects metrics every other second, which will effectively halve the CPU utilization
dedicated for metrics collection. Set this to `5` or `10` to collect metrics every 5 or 10 seconds, respectively.

```conf
[global]
  update every: 5
```

## Specific plugin or collector

If you did not [reduce the global collection frequency](#global) but find that a specific plugin/collector uses too many
resources, you can reduce its frequency. You configure [internal
collectors](/docs/collect/how-collectors-work.md#collector-architecture-and-terminolog) in `netdata.conf` and external
collectors in their individual `.conf` files.

To reduce the frequency of an [internal
plugin/collector](/docs/collect/how-collectors-work.md#collector-architecture-and-terminology), open `netdata.conf` and
find the appropriate section. For example, to reduce the frequency of the `apps` plugin, which collects and visualizes
metrics on application resource utilization:

```conf
[plugin:apps]
    update every = 5
```

To [configure an individual collector](/docs/collect/enable-configure.md), open its specific configuration file with
`edit-config` and look for the `update_every` setting. For example, to reduce the frequency of the `nginx` collector,
run `sudo ./edit-config go.d/nginx.conf`:

```conf
# [ GLOBAL ]
update_every: 10
```

## Disable unneeded plugins or collectors

If you know that you don't need an [entire plugin or a specific
collector](/docs/collect/how-collectors-work.md#collector-architecture-and-terminology), you can disable any of them.
Keep in mind that if a plugin/collector has nothing to do, it simply shuts down and does not consume system resources.
You will only improve the Agent's performance by disabling plugins/collectors that are actively collecting metrics.

Open `netdata.conf` and scroll down to the `[plugins]` section. To disable any plugin, uncomment it and set the value to
`no`. For example, to explicitly keep the `proc` and `go.d` plugins enabled while disabling `python.d`, `charts.d`, and
`node.d`.

```conf
[plugins]
    proc = yes
		python.d = no
		charts.d = no
		node.d = no
		go.d = yes
```

Disable specific collectors by opening their respective plugin configuration files, uncommenting the line for the
collector, and setting its value to `no`.

```bash
sudo ./edit-config go.d.conf
sudo ./edit-config python.d.conf
sudo ./edit-config node.d.conf
sudo ./edit-config charts.d.conf
```

For example, to disable a few Python collectors:

```conf
  apache: no
	dockerd: no
	fail2ban: no
```

## Lower memory usage for metrics retention

Reduce the disk space that the [database engine](/database/engine/README.md) uses to retain metrics by editing
the `dbengine multihost disk space` option in `netdata.conf`. The default value is `256`, but can be set to a minimum of
`64`. By reducing the disk space allocation, Netdata also needs to store less metadata in the node's memory.

The `page cache size` option also directly impacts Netdata's memory usage, but has a minimum value of `32`.

Reducing the value of `dbengine multihost disk space` does slim down Netdata's resource usage, but it also reduces how
long Netdata retains metrics. Find the right balance of performance and metrics retention by using the [dbengine
calculator](/docs/store/change-metrics-storage.md#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics).

All the settings are found in the `[global]` section of `netdata.conf`:

```conf
[global]
	memory mode = dbengine
	page cache size = 32
  dbengine multihost disk space = 256
```

## Run Netdata behind Nginx

A dedicated web server like Nginx provides far more robustness than the Agent's internal [web server](/web/README.md).
Nginx can handle more concurrent connections, reuse idle connections, and use fast gzip compression to reduce payloads.

For details on installing Nginx as a proxy for the local Agent dashboard, see our [Nginx
doc](/docs/Running-behind-nginx.md).

After you complete Nginx setup according to the doc linked above, we recommend setting `keepalive` to `1024`, and using
gzip compression with the following options in the `location /` block:

```conf
  location / {
		...
		gzip on;
		gzip_proxied any;
		gzip_types *;
	}
```

Finally, edit `netdata.conf` with the following settings:

```conf
[global]
    bind socket to IP = 127.0.0.1
    access log = none
    disconnect idle web clients after seconds = 3600
    enable web responses gzip compression = no
```

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

## Disable logs

If you installation is working correctly, and you're not actively auditing Netdata's logs, disable them in
`netdata.conf`.

```conf
[global]
	debug log = none
	error log = none
	access log = none
```

## What's next?

We hope this guide helped you better understand how to optimize the performance of the Netdata Agent.

Now that your Agent is running smoothly, we recommend you [secure your nodes](/docs/configure/nodes.md) if you haven't
already.

Next, dive into some of Netdata's more complex features, such as configuring its health watchdog or exporting metrics to
an external time-series database.

-   [Interact with dashboards and charts](/docs/visualize/interact-dashboards-charts.md)
-   [Configure health alarms](/docs/monitor/configure-alarms.md)
-   [Export metrics to external time-series databases](/docs/export/external-databases.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fconfigure%2Fperformance.md&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
