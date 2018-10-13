# Data Collection Plugins

netdata supports **internal** and **external** data collection plugins:

- **internal** plugins are written in `C` and run as threads inside the netdata daemon.

- **external** plugins may be written in any computer language and are spawn as independent long-running processes by the netdata daemon.
   They communicate with the netdata daemon via `pipes` (`stdout` communication).

> To minimize the number of processes spawn for data collection, netdata also supports **plugin orchestrators**.

- **plugin orchestrators** are external plugins that do not collect any data by themeselves.
   Instead they support data collection **modules** written in the language of the orchestrator.
   Usually the orchestrator provides a higher level abstraction, making it ideal for writing new
   data collection modules with the minimum of code.

## Internal Plugins

plugin|description
:---:|:---
[checks.plugin](checks.plugin/)|a debugging plugin (by default it is disabled)
[freebsd.plugin](freebsd.plugin/)|collects resource usage and performance data on FreeBSD systems
[idlejitter.plugin](idlejitter.plugin/)|measures CPU latency and jitter on all operating systems
[linux-cgroups.plugin](linux-cgroups.plugin/)|collects resource usage of Containers, VMs and systemd, on Linux systems
[linux-diskspace.plugin](linux-diskspace.plugin/)|collects disk space usage metrics on Linux mount points
[linux-nfacct.plugin](linux-nfacct.plugin/)|collects netfilter metrics using `libmnl` and `libnetfilter_acct`
[linux-proc.plugin](linux-proc.plugin/)|collects resource usage and performance data on Linux systems
[linux-tc.plugin](linux-tc.plugin/)|collects traffic QoS metrics of Linux network interfaces
[macos.plugin](macos.plugin/)|collects resource usage and performance data on MacOS systems
[plugins.d.plugin](plugins.d.plugin/)|implements the **external plugins** API and serves external plugins
[statsd.plugin](statsd.plugin/)|implements a high performance statsd server for netdata

## External Plugins and Plugin Orchestrators

Browse the [plugins.d.plugin](plugins.d.plugin/) directory.

## Internal Plugins API

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
        // this chart is already create it
        // let netdata know we start the next iteration on it
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