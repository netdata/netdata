# Netdata Data Collection Plugins

netdata supports **internal** and **external** data collection plugins:

- **internal** plugins are written in `C` and run as threads inside the netdata daemon.

- **external** plugins may be written in any computer language and are spawn as independent long-running processes by the netdata daemon.
   They communicate with the netdata daemon via `pipes` (`stdout` communication). The list of netdata external plugins at [plugins.d](plugins.d/).

> To minimize the number of processes spawn for data collection, netdata also supports **plugin orchestrators**.

- **plugin orchestrators** are external plugins that do not collect any data by themeselves.
   Instead they support data collection **modules** written in the language of the orchestrator.
   Usually the orchestrator provides a higher level abstraction, making it ideal for writing new
   data collection modules with the minimum of code. Currently netdata provides plugin orchestrators
   [BASH](plugins.d/charts.d.plugin) v4+, [node.js](plugins.d/node.d.plugin) and
   [python](plugins.d/python.d.plugin) v2+ (including v3).

## Netdata Internal Plugins

plugin|O/S|description
:---:|:---:|:---
[checks.plugin](checks.plugin/)|all|a debugging plugin (by default it is disabled)
[freebsd.plugin](freebsd.plugin/)|freebsd|collects resource usage and performance data on FreeBSD systems
[idlejitter.plugin](idlejitter.plugin/)|all|measures CPU latency and jitter on all operating systems
[linux-cgroups.plugin](linux-cgroups.plugin/)|linux|collects resource usage of **Containers**, libvirt **VMs** and **systemd services**, on Linux systems
[linux-diskspace.plugin](linux-diskspace.plugin/)|linux|collects disk space usage metrics on Linux mount points
[linux-nfacct.plugin](linux-nfacct.plugin/)|linux|collects netfilter firewall, connection tracker and accounting metrics using `libmnl` and `libnetfilter_acct`
[linux-proc.plugin](linux-proc.plugin/)|linux|collects resource usage and performance data on Linux systems
[linux-tc.plugin](linux-tc.plugin/)|linux|collects traffic QoS metrics (`tc`) of Linux network interfaces
[macos.plugin](macos.plugin/)|macos|collects resource usage and performance data on MacOS systems
[plugins.d](plugins.d/)|all|implements the **external plugins** API and serves external plugins
[statsd.plugin](statsd.plugin/)|all|implements a high performance **statsd** server for netdata

## Netdata External Plugins and Plugin Orchestrators

Browse the [plugins.d](plugins.d/) directory.

## Internal Plugins Configuration

Each plugin can be enabled or disabled via `netdata.conf`, section `[plugins]`.

At this section there a list of all the plugins with a boolean setting to enable them or not. 

Example:

```
[plugins]
	# cgroups = yes
	# diskspace = yes
	# idlejitter = yes
	# proc = yes
	# tc = yes
```

The exception is statsd.plugin that has its own `[statsd]` section.

Once a plugin is enabled, consult the page of each plugin for additional configuration options.

## Operation of Internal Plugins

Each of these plugins runs as a thread inside the netdata daemon.
Once this thread has started, the plugin may spawn additional threads according to its design.

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