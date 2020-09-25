<!--
title: "How Netdata's metrics collectors work"
description: "When Netdata starts, and with zero configuration, it auto-detects thousands of data sources and immediately collects per-second metrics."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/collect/how-collectors-work.md
-->

# How Netdata's metrics collectors work

When Netdata starts, and with zero configuration, it auto-detects thousands of data sources and immediately collects
per-second metrics.

Netdata can immediately collect metrics from these endpoints thanks to 300+ **collectors**, which all come pre-installed
when you [install the Netdata Agent](/docs/get/README.md#install-the-netdata-agent).

Every collector has two primary jobs:

-   Look for exposed metrics at a pre- or user-defined endpoint.
-   Gather exposed metrics and use additional logic to build meaningful, interactive visualizations.

If the collector finds compatible metrics exposed on the configured endpoint, it begins a per-second collection job. The
Netdata Agent gathers these metrics, sends them to the [database engine for
storage](/docs/store/change-metrics-storage.md), and immediately [visualizes them
meaningfully](/docs/visualize/interact-dashboards-charts.md) on dashboards.

Each collector comes with a pre-defined configuration that matches the default setup for that application. This endpoint
can be a URL and port, a socket, a file, a web page, and more.

For example, the [Nginx collector](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/nginx) searches
at `http://127.0.0.1/stub_status`, which is the default endpoint for exposing Nginx metrics. The [web log collector for
Nginx or Apache](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/modules/weblog) searches at
`/var/log/nginx/access.log` and `/var/log/apache2/access.log`, respectively, both of which are standard locations for
access log files on Linux systems.

The endpoint is user-configurable, as are many other specifics of what a given collector does.

## What can Netdata collect?

To quickly find your answer, see our [list of supported collectors](/collectors/COLLECTORS.md).

Generally, Netdata's collectors can be grouped into three types:

-   [Systems](/docs/collect/system-metrics.md): Monitor CPU, memory, disk, networking, systemd, eBPF, and much more.
    Every metric exposed by `/proc`, `/sys`, and other Linux kernel sources.
-   [Containers](/docs/collect/container-metrics.md): Gather metrics from container agents, like `dockerd` or `kubectl`,
    along with the resource usage of containers and the applications they run.
-   [Applications](/docs/collect/application-metrics.md): Collect per-second metrics from web servers, databases, logs,
    message brokers, APM tools, email servers, and much more.

## Collector architecture and terminology

**Collector** is a catch-all term for any Netdata process that gathers metrics from an endpoint. 

While we use _collector_ most often in documentation, release notes, and educational content, you may encounter other
terms related to collecting metrics.

-   **Modules** are a type of collector.
-   **Orchestrators** are external plugins that run and manage one or more modules. They run as independent processes.
    The Go orchestator is in active development.
    -   [go.d.plugin](https://learn.netdata.cloud/docs/agent/collectors/go.d.plugin/): An orchestrator for data
        collection modules written in `go`.
    -   [python.d.plugin](/collectors/python.d.plugin/README.md): An orchestrator for data collection modules written in
        `python` v2/v3.
    -   [charts.d.plugin](/collectors/charts.d.plugin/README.md): An orchestrator for data collection modules written in
        `bash` v4+.
    -   [node.d.plugin](/collectors/node.d.plugin/README.md): An orchestrator for data collection modules written in
        `node.js`.
-   **External plugins** gather metrics from external processes, such as a webserver or database, and run as independent
    processes that communicate with the Netdata daemon via pipes.
-   **Internal plugins** gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C`,
    and run as threads within the Netdata daemon.

## What's next?

[Enable or configure a collector](/docs/collect/enable-configure.md) if the default settings are not compatible with
your infrastructure.

See our [collectors reference](/collectors/REFERENCE.md) for detailed information on Netdata's collector architecture,
troubleshooting a collector, developing a custom collector, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fcollect%2Fhow-collectors-work&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
