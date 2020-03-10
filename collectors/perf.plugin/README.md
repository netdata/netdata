<!--
---
title: "perf.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/perf.plugin/README.md
---
-->

# perf.plugin

`perf.plugin` collects system-wide CPU performance statistics from Performance Monitoring Units (PMU) using
the `perf_event_open()` system call.

## Important Notes

Accessing hardware PMUs requires root permissions, so the plugin is setuid to root.

Keep in mind that the number of PMUs in a system is usually quite limited and every hardware monitoring
event for every CPU core needs a separate file descriptor to be opened.

## Charts

The plugin provides statistics for general hardware and software performance monitoring events:

Hardware events:

1.  CPU cycles
2.  Instructions
3.  Branch instructions
4.  Cache operations
5.  BUS cycles
6.  Stalled frontend and backend cycles

Software events:

1.  CPU migrations
2.  Alignment faults
3.  Emulation faults

Hardware cache events:

1.  L1D cache operations
2.  L1D prefetch cache operations
3.  L1I cache operations
4.  LL cache operations
5.  DTLB cache operations
6.  ITLB cache operations
7.  PBU cache operations

## Configuration

The plugin is disabled by default because the number of PMUs is usually quite limited and it is not desired to
allow Netdata to struggle silently for PMUs, interfering with other performance monitoring software. If you need to
enable the perf plugin, edit /etc/netdata/netdata.conf and set:

```raw
[plugins]
    perf = yes
```

```raw
[plugin:perf]
    update every = 1
    command options = all
```

You can use the `command options` parameter to pick what data should be collected and which charts should be
displayed. If `all` is used, all general performance monitoring counters are probed and corresponding charts
are enabled for the available counters. You can also define a particular set of enabled charts using the
following keywords: `cycles`, `instructions`, `branch`, `cache`, `bus`, `stalled`, `migrations`, `alighnment`,
`emulation`, `L1D`, `L1D-prefetch`, `L1I`, `LL`, `DTLB`, `ITLB`, `PBU`.

## Debugging

You can run the plugin by hand:

```raw
sudo /usr/libexec/netdata/plugins.d/perf.plugin 1 all debug
```

You will get verbose output on what the plugin does.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fperf.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
