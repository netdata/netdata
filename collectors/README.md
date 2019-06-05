# Data collection plugins

netdata supports **internal** and **external** data collection plugins:

- **internal** plugins are written in `C` and run as threads inside the netdata daemon.

- **external** plugins may be written in any computer language and are spawn as independent long-running processes by the netdata daemon.
   They communicate with the netdata daemon via `pipes` (`stdout` communication).

To minimize the number of processes spawn for data collection, netdata also supports **plugin orchestrators**.

- **plugin orchestrators** are external plugins that do not collect any data by themeselves.
   Instead they support data collection **modules** written in the language of the orchestrator.
   Usually the orchestrator provides a higher level abstraction, making it ideal for writing new
   data collection modules with the minimum of code.

   Currently netdata provides plugin orchestrators
   BASH v4+ [charts.d.plugin](charts.d.plugin/),
   node.js [node.d.plugin](node.d.plugin/) and
   python v2+ (including v3) [python.d.plugin](python.d.plugin/).

## Netdata Plugins

plugin|lang|O/S|runs as|modular|description
:---:|:---:|:---:|:---:|:---:|:---
[apps.plugin](apps.plugin/)|`C`|linux, freebsd|external|-|monitors the whole process tree on Linux and FreeBSD and breaks down system resource usage by **process**, **user** and **user group**.
[cgroups.plugin](cgroups.plugin/)|`C`|linux|internal|-|collects resource usage of **Containers**, libvirt **VMs** and **systemd services**, on Linux systems
[charts.d.plugin](charts.d.plugin/)|`BASH` v4+|any|external|yes|a **plugin orchestrator** for data collection modules written in `BASH` v4+.
[checks.plugin](checks.plugin/)|`C`|any|internal|-|a debugging plugin (by default it is disabled)
[cups.plugin](cups.plugin/)|`C`|any|external|-|monitors **CUPS**
[diskspace.plugin](diskspace.plugin/)|`C`|linux|internal|-|collects disk space usage metrics on Linux mount points
[fping.plugin](fping.plugin/)|`C`|any|external|-|measures network latency, jitter and packet loss between the monitored node and any number of remote network end points.
[ioping.plugin](ioping.plugin/)|`C`|any|external|-|measures disk read/write latency.
[freebsd.plugin](freebsd.plugin/)|`C`|freebsd|internal|yes|collects resource usage and performance data on FreeBSD systems
[freeipmi.plugin](freeipmi.plugin/)|`C`|linux, freebsd|external|-|collects metrics from enterprise hardware sensors, on Linux and FreeBSD servers.
[idlejitter.plugin](idlejitter.plugin/)|`C`|any|internal|-|measures CPU latency and jitter on all operating systems
[macos.plugin](macos.plugin/)|`C`|macos|internal|yes|collects resource usage and performance data on MacOS systems
[nfacct.plugin](nfacct.plugin/)|`C`|linux|external|-|collects netfilter firewall, connection tracker and accounting metrics using `libmnl` and `libnetfilter_acct`
[xenstat.plugin](xenstat.plugin/)|`C`|linux|external|-|collects XenServer and XCP-ng metrics using `libxenstat`
[perf.plugin](perf.plugin/)|`C`|linux|external|-|collects CPU performance metrics using performance monitoring units (PMU).
[node.d.plugin](node.d.plugin/)|`node.js`|any|external|yes|a **plugin orchestrator** for data collection modules written in `node.js`.
[plugins.d](plugins.d/)|`C`|any|internal|-|implements the **external plugins** API and serves external plugins
[proc.plugin](proc.plugin/)|`C`|linux|internal|yes|collects resource usage and performance data on Linux systems
[python.d.plugin](python.d.plugin/)|`python` v2+|any|external|yes|a **plugin orchestrator** for data collection modules written in `python` v2 or v3 (both are supported).
[statsd.plugin](statsd.plugin/)|`C`|any|internal|-|implements a high performance **statsd** server for netdata
[tc.plugin](tc.plugin/)|`C`|linux|internal|-|collects traffic QoS metrics (`tc`) of Linux network interfaces

## Enabling and Disabling plugins

Each plugin can be enabled or disabled via `netdata.conf`, section `[plugins]`.

At this section there a list of all the plugins with a boolean setting to enable them or disable them.

The exception is `statsd.plugin` that has its own `[statsd]` section.

Once a plugin is enabled, consult the page of each plugin for additional configuration options.

All **external plugins** are managed by [plugins.d](plugins.d/), which provides additional management options.

### Internal Plugins

Each of the internal plugins runs as a thread inside the netdata daemon.
Once this thread has started, the plugin may spawn additional threads according to its design.

#### Internal Plugins API

The internal data collection API consists of the following calls:

```c
collect_data() {
    // collect data here (one iteration)

    collected_number collected_value = collect_a_value();

    // give the metrics to netdata

    static RRDSET *st = NULL; // the chart
    static RRDDIM *rd = NULL; // a dimension attached to this chart

    if(unlikely(!st)) {
        // we haven't created this chart before
        // create it now
        st = rrdset_create_localhost(
                "type"
                , "id"
                , "name"
                , "family"
                , "context"
                , "Chart Title"
                , "units"
                , "plugin-name"
                , "module-name"
                , priority
                , update_every
                , chart_type
        );

        // attach a metric to it
        rd = rrddim_add(st, "id", "name", multiplier, divider, algorithm);
    }
    else {
        // this chart is already created
        // let netdata know we start a new iteration on it
        rrdset_next(st);
    }

    // give the collected value(s) to the chart
    rrddim_set_by_pointer(st, rd, collected_value);

    // signal netdata we are done with this iteration
    rrdset_done(st);
}
```

Of course netdata has a lot of libraries to help you also in collecting the metrics.
The best way to find your way through this, is to examine what other similar plugins do.


### External Plugins

**External plugins** use the API and are managed by [plugins.d](plugins.d/).


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
