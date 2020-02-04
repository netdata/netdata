# Collecting metrics

Netdata can collect metrics from hundreds of different sources, be they internal data created by the system itself, or
external data created by services or applications. To see _all_ of the sources Netdata collects from, view our [list of
supported collectors](COLLECTORS.md).

There are two essential points to understand about how collecting metrics works in Netdata:

-   All collectors are **installed by default** with every installation of Netdata. You do not need to install
    collectors manually to collect metrics from new sources.
-   Upon startup, Netdata will **auto-detect** any service and application that has a [collector](COLLECTORS.md), as
    long as both the collector and the service/application are configured correctly. If Netdata fails to show charts for
    a service that's running on your system, it's due to a misconfiguration.



There are also different types of collectors:

-   **Internal** collectors gather metrics from `/proc`, `/sys` and other Linux kernel sources. They are written in `C`,
    and run as threads within the Netdata daemon.
-   **External** collectors gather metrics from external processes, such as a MySQL database or Nginx web server. They
    can be written in any language, and the `netdata` daemon spawns them as long-running independent processes. They
    communicate with the daemon via pipes.
-   **Plugin orchestrators** 


Netdata supports **internal** and **external** data collection plugins:

-   **Internal** plugins are written in `C` and run as threads inside the `netdata` daemon.
-   **External** plugins may be written in any computer language and are spawn as independent long-running processes by
     the `netdata` daemon. They communicate with the `netdata` daemon via `pipes` (`stdout` communication). Some 

To minimize the number of processes spawn for data collection, Netdata also supports **plugin orchestrators**.

-   **plugin orchestrators** are external plugins that do not collect any data by themeselves.
     Instead they support data collection **modules** written in the language of the orchestrator.
     Usually the orchestrator provides a higher level abstraction, making it ideal for writing new
     data collection modules with the minimum of code.

     Currently Netdata provides plugin orchestrators
     BASH v4+ [charts.d.plugin](charts.d.plugin/),
     node.js [node.d.plugin](node.d.plugin/) and
     python v2+ (including v3) [python.d.plugin](python.d.plugin/).


## Enabling and Disabling plugins

Each plugin can be enabled or disabled via `netdata.conf`, section `[plugins]`.

At this section there a list of all the plugins with a boolean setting to enable them or disable them.

The exception is `statsd.plugin` that has its own `[statsd]` section.

Once a plugin is enabled, consult the page of each plugin for additional configuration options.

All **external plugins** are managed by [plugins.d](plugins.d/), which provides additional management options.

### Internal Plugins

Each of the internal plugins runs as a thread inside the `netdata` daemon.
Once this thread has started, the plugin may spawn additional threads according to its design.

#### Internal Plugins API

The internal data collection API consists of the following calls:

```c
collect_data() {
    // collect data here (one iteration)

    collected_number collected_value = collect_a_value();

    // give the metrics to Netdata

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
        // let Netdata know we start a new iteration on it
        rrdset_next(st);
    }

    // give the collected value(s) to the chart
    rrddim_set_by_pointer(st, rd, collected_value);

    // signal Netdata we are done with this iteration
    rrdset_done(st);
}
```

Of course, Netdata has a lot of libraries to help you also in collecting the metrics. The best way to find your way through this, is to examine what other similar plugins do.

### External Plugins

**External plugins** use the API and are managed by [plugins.d](plugins.d/).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
