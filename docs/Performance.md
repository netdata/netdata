# Performance

Netdata performance is affected by:

**Data collection**

-   the number of charts for which data are collected
-   the number of plugins running
-   the technology of the plugins (i.e. BASH plugins are slower than binary plugins)
-   the frequency of data collection

You can control all the above.

**Web clients accessing the data**

-   the duration of the charts in the dashboard
-   the number of charts refreshes requested
-   the compression level of the web responses

- - -

## Netdata Daemon

For most server systems, with a few hundred charts and a few thousand dimensions, the Netdata daemon, without any web clients accessing it, should not use more than 1% of a single core.

To prove Netdata scalability, check issue [#1323](https://github.com/netdata/netdata/issues/1323#issuecomment-265501668) where Netdata collects 95.000 metrics per second, with 12% CPU utilization of a single core!

In embedded systems, if the Netdata daemon is using a lot of CPU without any web clients accessing it, you should lower the data collection frequency. To set the data collection frequency, edit `/etc/netdata/netdata.conf` and set `update_every` to a higher number (this is the frequency in seconds data are collected for all charts: higher number of seconds = lower frequency, the default is 1 for per second data collection). You can also set this frequency per module or chart. Check the [daemon configuration](../daemon/config) for plugins and charts. For specific modules, the configuration needs to be changed in:

-   `python.d.conf` for [python](../collectors/python.d.plugin/#pythondplugin)
-   `node.d.conf` for [nodejs](../collectors/node.d.plugin/#nodedplugin)
-   `charts.d.conf` for [bash](../collectors/charts.d.plugin/#chartsdplugin)

## Plugins

If a plugin is using a lot of CPU, you should lower its update frequency, or if you wrote it, re-factor it to be more CPU efficient. Check [External Plugins](../collectors/plugins.d/) for more details on writing plugins.

## CPU consumption when web clients are accessing dashboards

Netdata is very efficient when servicing web clients. On most server platforms, Netdata should be able to serve **1800 web client requests per second per core** for auto-refreshing charts.

Normally, each user connected will request less than 10 chart refreshes per second (the page may have hundreds of charts, but only the visible are refreshed). So you can expect 180 users per CPU core accessing dashboards before having any delays.

Netdata runs with the lowest possible process priority, so even if 1000 users are accessing dashboards, it should not influence your applications. CPU utilization will reach 100%, but your applications should get all the CPU they need.

To lower the CPU utilization of Netdata when clients are accessing the dashboard, set `web compression level = 1`, or disable web compression completely by setting `enable web responses gzip compression = no`. Both settings are in the `[web]` section.

## Monitoring a heavily-loaded system

While running, Netdata does not depend much on disk I/O aside from writing to log files and the [database
engine](../database/engine/README.md) "spilling" historical metrics to disk when it uses all its available RAM.

Under a heavy system load, plugins that need disk may stop and show gaps during heavy system load, but the Netdata
daemon itself should be able to work and collect values from `/proc` and `/sys` and serve web clients accessing it.

Keep in mind that Netdata saves its database when it exits, and loads it up again when started.

## Netdata process priority

By default, Netdata runs with the `idle` process scheduler, which assigns CPU resources to Netdata, only when the system has such resources to spare.

The following `netdata.conf` settings control this:

```
[global]
    process scheduling policy = idle
    process scheduling priority = 0
    process nice level = 19
```

The policies supported by Netdata are `idle` (the Netdata default), `other` (also as `nice`), `batch`, `rr`, `fifo`. Netdata also recognizes `keep` and `none` to keep the current settings without changing them.

For `other`, `nice` and `batch`, the setting `process nice level = 19` is activated to configure the nice level of Netdata. Nice gets values -20 (highest) to 19 (lowest).

For `rr` and `fifo`, the setting `process scheduling priority = 0` is activated to configure the priority of the relative scheduling policy. Priority gets values 1 (lowest) to 99 (highest).

For the details of each scheduler, see `man sched_setscheduler` and `man sched`.

When Netdata is running under systemd, it can only lower its priority (the default is `other` with `nice level = 0`). If you want to make Netdata to get more CPU than that, you will need to set in `netdata.conf`:

```
[global]
    process scheduling policy = keep
```

and edit `/etc/systemd/system/netdata.service` and add:

```
CPUSchedulingPolicy=other | batch | idle | fifo | rr
CPUSchedulingPriority=99
Nice=-10
```

## Running Netdata in embedded devices

Embedded devices usually have very limited CPU resources available, and in most cases, just a single core.

> keep in mind that Netdata on RPi 2 and 3 does not require any tuning. The default settings will be good. The following tunables apply only when running Netdata on RPi 1 or other very weak IoT devices.

We suggest to do the following:

### 1. Disable External plugins

External plugins can consume more system resources than the Netdata server. Disable the ones you don't need. If you need them, increase their `update every` value (again in `/etc/netdata/netdata.conf`), so that they do not run that frequently.

Edit `/etc/netdata/netdata.conf`, find the `[plugins]` section:

```
[plugins]
	proc = yes

	tc = no
	idlejitter = no
	cgroups = no
	checks = no
	apps = no
	charts.d = no
	node.d = no
	python.d = no

	plugins directory = /usr/libexec/netdata/plugins.d
	enable running new plugins = no
	check for new plugins every = 60
```

In detail:

| plugin|description|
|:----:|:----------|
| `proc`|the internal plugin used to monitor the system. Normally, you don't want to disable this. You can disable individual functions of it at the next section.|
| `tc`|monitoring network interfaces QoS (tc classes)|
| `idlejitter`|internal plugin (written in C) that attempts show if the systems starved for CPU. Disabling it will eliminate a thread.|
| `cgroups`|monitoring linux containers. Most probably you are not going to need it. This will also eliminate another thread.|
| `checks`|a debugging plugin, which is disabled by default.|
| `apps`|a plugin that monitors system processes. It is very complex and heavy (consumes twice the CPU resources of the Netdata daemon), so if you don't need to monitor the process tree, you can disable it.|
| `charts.d`|BASH plugins (squid, nginx, mysql, etc). This is a heavy plugin, that consumes twice the CPU resources of the Netdata daemon.|
| `node.d`|node.js plugin, currently used for SNMP data collection and monitoring named (the name server).|
| `python.d`|has many modules and can use over 20MB of memory.|

For most IoT devices, you can disable all plugins except `proc`. For `proc` there is another section that controls which functions of it you need. Check the next section.

---

### 2. Disable internal plugins

In this section you can select which modules of the `proc` plugin you need. All these are run in a single thread, one after another. Still, each one needs some RAM and consumes some CPU cycles. With all the modules enabled, the `proc` plugin adds ~9 MiB on top of the 5 MiB required by the Netdata daemon. 

```
[plugin:proc]
	# /proc/net/dev = yes                       # network interfaces
	# /proc/diskstats = yes                     # disks
...
```

Refer to the [proc.plugins documentation](../collectors/proc.plugin/) for the list and description of all the proc plugin modules.

### 3. Lower internal plugin update frequency

If Netdata is still using a lot of CPU, lower its update frequency. Going from per second updates, to once every 2 seconds updates, will cut the CPU resources of all Netdata programs **in half**, and you will still have very frequent updates.

If the CPU of the embedded device is too weak, try setting even lower update frequency. Experiment with `update every = 5` or `update every = 10` (higher number = lower frequency) in `netdata.conf`, until you get acceptable results.

Keep in mind this will also force dashboard chart refreshes to happen at the same rate. So increasing this number actually lowers data collection frequency but also lowers dashboard chart refreshes frequency.

This is a dashboard on a device with `[global].update every = 5` (this device is a media player and is now playing a movie):		
		
![pi1](https://cloud.githubusercontent.com/assets/2662304/15338489/ca84baaa-1c88-11e6-9ab2-118208e11ce1.gif)

### 4. Disable logs

Normally, you will not need them. To disable them, set:

```
[global]
	debug log = none
	error log = none
	access log = none
```

### 5. Lower Netdata's memory usage

You can change the amount of RAM and disk the database engine uses for all charts and their dimensions with the
following settings in the `[global]` section of `netdata.conf`:

```conf
[global]
	# memory mode = dbengine
	# page cache size = 32
	# dbengine disk space = 256
```

See the [database engine documentation](../database/engine/README.md) or our [tutorial on metrics
retention](tutorials/longer-metrics-storage.md) for more details on lowering the database engine's memory requirements.

### 6. Disable gzip compression of responses

Gzip compression of the web responses is using more CPU that the rest of Netdata. You can lower the compression level or disable gzip compression completely. You can disable it, like this:

```
[web]
	enable gzip compression = no
```

To lower the compression level, do this:

```
[web]
	enable gzip compression = yes
	gzip compression level = 1
```

Finally, if no web server is installed on your device, you can use port tcp/80 for Netdata:

```
[web]
	port = 80
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FPerformance&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
