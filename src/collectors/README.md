# Collectors

When Netdata starts, and with zero configuration, it auto-detects thousands of data sources and immediately collects
per-second metrics.

Netdata can immediately collect metrics from these endpoints thanks to 300+ **collectors**, which all come pre-installed
when you [install Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).

All collectors are **installed by default** with every installation of Netdata. You do not need to install
collectors manually to collect metrics from new sources. 
See how you can [monitor anything with Netdata](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md).

Upon startup, Netdata will **auto-detect** any application or service that has a collector, as long as both the collector
and the app/service are configured correctly. If you don't see charts for your application, see
our [collectors' configuration reference](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md).

## How Netdata's metrics collectors work

Every collector has two primary jobs:

-   Look for exposed metrics at a pre- or user-defined endpoint.
-   Gather exposed metrics and use additional logic to build meaningful, interactive visualizations.

If the collector finds compatible metrics exposed on the configured endpoint, it begins a per-second collection job. The
Netdata Agent gathers these metrics, sends them to the 
[database engine for storage](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md)
, and immediately 
[visualizes them meaningfully](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md) 
on dashboards.

Each collector comes with a pre-defined configuration that matches the default setup for that application. This endpoint
can be a URL and port, a socket, a file, a web page, and more. The endpoint is user-configurable, as are many other 
specifics of what a given collector does.

## Collector architecture and terminology

-   **Collectors** are the processes/programs that actually gather metrics from various sources. 

-   **Plugins** help manage all the independent data collection processes in a variety of programming languages, based on 
    their purpose  and performance requirements. There are three types of plugins:

    -   **Internal** plugins organize collectors that gather metrics from `/proc`, `/sys` and other Linux kernel sources.
        They are written in `C`, and run as threads within the Netdata daemon.

    -   **External** plugins organize collectors that gather metrics from external processes, such as a MySQL database or
        Nginx web server. They can be written in any language, and the `netdata` daemon spawns them as long-running
        independent processes. They communicate with the daemon via pipes. All external plugins are managed by
        [plugins.d](https://github.com/netdata/netdata/blob/master/src/collectors/plugins.d/README.md), which provides additional management options.

-   **Orchestrators** are external plugins that run and manage one or more modules. They run as independent processes.
    The Go orchestrator is in active development.

    -   [go.d.plugin](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/README.md): An orchestrator for data
        collection modules written in `go`.

    -   [python.d.plugin](https://github.com/netdata/netdata/blob/master/src/collectors/python.d.plugin/README.md): 
        An orchestrator for data collection modules written in `python` v2/v3.

    -   [charts.d.plugin](https://github.com/netdata/netdata/blob/master/src/collectors/charts.d.plugin/README.md): 
        An orchestrator for data collection modules written in`bash` v4+.

-   **Modules** are the individual programs controlled by an orchestrator to collect data from a specific application, or type of endpoint.
