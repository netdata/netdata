<!--
title: "Monitor CPU performance statistics (perf.plugin)"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/perf.plugin/README.md"
sidebar_label: "CPU performance statistics (perf.plugin)"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/System metrics"
-->

# Monitor CPU performance statistics (perf.plugin)

`perf.plugin` collects system-wide CPU performance statistics from Performance Monitoring Units (PMU) using
the `perf_event_open()` system call.

## Important Notes

If you are using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), you will need to install
the `netdata-plugin-perf` package using your system package manager.

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
following keywords: `cycles`, `instructions`, `branch`, `cache`, `bus`, `stalled`, `migrations`, `alignment`,
`emulation`, `L1D`, `L1D-prefetch`, `L1I`, `LL`, `DTLB`, `ITLB`, `PBU`.

## Debugging

You can run the plugin by hand:

```raw
sudo /usr/libexec/netdata/plugins.d/perf.plugin 1 all debug
```

You will get verbose output on what the plugin does.


