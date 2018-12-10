# Performance

netdata performance is affected by:

**Data collection**
- the number of charts for which data are collected
- the number of plugins running
- the technology of the plugins (i.e. BASH plugins are slower than binary plugins)
- the frequency of data collection

You can control all the above.

**Web clients accessing the data**
- the duration of the charts in the dashboard
- the number of charts refreshes requested
- the compression level of the web responses

---

## Netdata Daemon

For most server systems, with a few hundred charts and a few thousand dimensions, the netdata daemon, without any web clients accessing it, should not use more than 1% of a single core.

To prove netdata scalability, check issue [#1323](https://github.com/netdata/netdata/issues/1323#issuecomment-265501668) where netdata collects 95.000 metrics per second, with 12% CPU utilization of a single core!

In embedded systems, if the netdata daemon is using a lot of CPU without any web clients accessing it, you should lower the data collection frequency. To set the data collection frequency, edit `/etc/netdata/netdata.conf` and set `update_every` to a higher number (this is the frequency in seconds data are collected for all charts: higher number of seconds = lower frequency, the default is 1 for per second data collection). You can also set this frequency per module or chart. Check the [daemon configuration](../daemon/config) for plugins and charts. For specific modules, the configuration needs to be changed in:
- `python.d.conf` for [python](../collectors/python.d.plugin/#pythondplugin)
- `node.d.conf` for [nodejs](../collectors/node.d.plugin/#nodedplugin)
- `charts.d.conf` for [bash](../collectors/charts.d.plugin/#chartsdplugin)

## Plugins

If a plugin is using a lot of CPU, you should lower its update frequency, or if you wrote it, re-factor it to be more CPU efficient. Check [External Plugins](../collectors/plugins.d/) for more details on writing plugins.

## CPU consumption when web clients are accessing dashboards

Netdata is very efficient when servicing web clients. On most server platforms, netdata should be able to serve **1800 web client requests per second per core** for auto-refreshing charts.

Normally, each user connected will request less than 10 chart refreshes per second (the page may have hundreds of charts, but only the visible are refreshed). So you can expect 180 users per CPU core accessing dashboards before having any delays.

Netdata runs with the lowest possible process priority, so even if 1000 users are accessing dashboards, it should not influence your applications. CPU utilization will reach 100%, but your applications should get all the CPU they need.

To lower the CPU utilization of netdata when clients are accessing the dashboard, set `web compression level = 1`, or disable web compression completely by setting `enable web responses gzip compression = no`. Both settings are in the `[web]` section.


## Monitoring a heavy loaded system

Netdata, while running, does not depend on disk I/O (apart its log files and `access.log` is written with buffering enabled and can be disabled). Some plugins that need disk may stop and show gaps during heavy system load, but the netdata daemon itself should be able to work and collect values from `/proc` and `/sys` and serve web clients accessing it.

Keep in mind that netdata saves its database when it exits and loads it back when restarted. While it is running though, its DB is only stored in RAM and no I/O takes place for it.

## Netdata process priority

By default, netdata runs with the `idle` process scheduler, which assigns CPU resources to netdata, only when the system has such resources to spare.

The following `netdata.conf` settings control this:

```
[global]
    process scheduling policy = idle
    process scheduling priority = 0
    process nice level = 19
```

The policies supported by netdata are `idle` (the netdata default), `other` (also as `nice`), `batch`, `rr`, `fifo`. netdata also recognizes `keep` and `none` to keep the current settings without changing them.

For `other`, `nice` and `batch`, the setting `process nice level = 19` is activated to configure the nice level of netdata. Nice gets values -20 (highest) to 19 (lowest).

For `rr` and `fifo`, the setting `process scheduling priority = 0` is activated to configure the priority of the relative scheduling policy. Priority gets values 1 (lowest) to 99 (highest).

For the details of each scheduler, see `man sched_setscheduler` and `man sched`.

When netdata is running under systemd, it can only lower its priority (the default is `other` with `nice level = 0`). If you want to make netdata to get more CPU than that, you will need to set in `netdata.conf`:

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

## Running netdata in embedded devices

Embedded devices usually have very limited CPU resources available, and in most cases, just a single core.

> keep in mind that netdata on RPi 2 and 3 does not require any tuning. The default settings will be good. The following tunables apply only when running netdata on RPi 1 or other very weak IoT devices.

We suggest to do the following:

### 1. Disable External plugins

External plugins can consume more system resources than the netdata server. Disable the ones you don't need. If you need them, increase their `update every` value (again in `/etc/netdata/netdata.conf`), so that they do not run that frequently.

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

	plugins directory = /usr/libexec/netdata/plugins.d
	enable running new plugins = no
	check for new plugins every = 60
```

In detail:

plugin|description
:---:|:---------
`proc`|the internal plugin used to monitor the system. Normally, you don't want to disable this. You can disable individual functions of it at the next section.
`tc`|monitoring network interfaces QoS (tc classes)
`idlejitter`|internal plugin (written in C) that attempts show if the systems starved for CPU. Disabling it will eliminate a thread.
`cgroups`|monitoring linux containers. Most probably you are not going to need it. This will also eliminate another thread.
`checks`|a debugging plugin, which is disabled by default.
`apps`|a plugin that monitors system processes. It is very complex and heavy (consumes twice the CPU resources of the netdata daemon), so if you don't need to monitor the process tree, you can disable it.
`charts.d`|BASH plugins (squid, nginx, mysql, etc). This is a heavy plugin, that consumes twice the CPU resources of the netdata daemon.
`node.d`|node.js plugin, currently used for SNMP data collection and monitoring named (the name server).

For most IoT devices, you can disable all plugins except `proc`. For `proc` there is another section that controls which functions of it you need. Check the next section.

---

### 2. Disable internal plugins

In this section you can select which modules of the `proc` plugin you need. All these are run in a single thread, one after another. Still, each one needs some RAM and consumes some CPU cycles.

```
[plugin:proc]
	# /proc/net/dev = yes                       # network interfaces
	# /proc/diskstats = yes                     # disks
	# /proc/net/snmp = yes                      # generic IPv4
	# /proc/net/snmp6 = yes                     # generic IPv6
	# /proc/net/netstat = yes                   # TCP and UDP
	# /proc/net/stat/conntrack = yes            # firewall
	# /proc/net/ip_vs/stats = yes               # IP load balancer
	# /proc/net/stat/synproxy = yes             # Anti-DDoS
	# /proc/stat = yes                          # CPU, context switches
	# /proc/meminfo = yes                       # Memory
	# /proc/vmstat = yes                        # Memory operations
	# /proc/net/rpc/nfsd = yes                  # NFS Server
	# /proc/sys/kernel/random/entropy_avail = yes # Cryptography
	# /proc/interrupts = yes                    # Interrupts
	# /proc/softirqs = yes                      # SoftIRQs
	# /proc/loadavg = yes                       # Load Average
	# /sys/kernel/mm/ksm = yes                  # Memory deduper
	# netdata server resources = yes            # netdata charts
```

### 3. Lower internal plugin update frequency

If netdata is still using a lot of CPU, lower its update frequency. Going from per second updates, to once every 2 seconds updates, will cut the CPU resources of all netdata programs **in half**, and you will still have very frequent updates.

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
### 5. Set memory mode to RAM

Setting the memory mode to `ram` will disable loading and saving the round robin database. This will not affect anything while running netdata, but it might be required if you have very limited storage available.

```
[global]
	memory mode = ram
```

### 6. Use the single threaded web server

Normally, netdata spawns a thread for each web client. This allows netdata to utilize all the available cores for servicing chart refreshes. You can however disable this feature and serve all charts one after another, using a single thread / core. This will might lower the CPU pressure on the embedded device. To enable the single threaded web server, edit `/etc/netdata/netdata.conf` and set `mode = single-threaded` in the `[web]` section.

### 7. Lower memory requirements

You can set the default size of the round robin database for all charts, using:

```
[global]
      history = 600
```

The units for history is `[global].update every` seconds. So if `[global].update every = 6` and `[global].history = 600`, you will have an hour of data ( 6 x 600 = 3.600 ), which will store 600 points per dimension, one every 6 seconds.

Check also [Database](../database) for directions on calculating the size of the round robin database.


### 8. Disable gzip compression of responses

Gzip compression of the web responses is using more CPU that the rest of netdata. You can lower the compression level or disable gzip compression completely. You can disable it, like this:

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

Finally, if no web server is installed on your device, you can use port tcp/80 for netdata:

```
[web]
	port = 80
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FPerformance&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
