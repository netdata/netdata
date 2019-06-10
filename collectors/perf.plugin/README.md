# perf.plugin

`perf.plugin` collects system-wide CPU performance statistics using Performance Monitoring Units (PMU) through
the `perf_event_open()` system call.

## Security notes

Keep in mind that accessing hardware PMUs requires root permissions, so the plugin is setuid to root.

## Charts

The plugin provides statistics for general hardware and software performance monitoring events:

Hardware events:
1.  CPU cycles.
2.  BUS cycles.
3.  Instructions

Software events:
1.  Context switches

## Configuration

The plugin is disabled by default because the number of PMUs is usually quite limited and it is not desired to
allow Netdata silently struggle for PMUs, interfering with other performance monitoring software. If you need to
enable perf plugin, edit /etc/netdata/netdata.conf and set:

```raw
[plugins]
    perf = yes
```

## Debugging

You can run the plugin by hand:

```raw
sudo /usr/libexec/netdata/plugins.d/perf.plugin 1 debug
```

You will get verbose output on what the plugin does.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fperf.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
