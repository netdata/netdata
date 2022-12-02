<!--
title: "Metrics collection"
sidebar_label: "Metrics collection"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md"
sidebar_position: "900"
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "Concepts/Netdata agent"
learn_docs_purpose: "Explain how metrics are collected [Existing plugins, Custom plugins (plugins.d protocol, for C, Go, Java, Node.js, Python, etc), Statsd]. Auto detection"
-->

With zero configuration, Netdata auto-detects thousands of data sources upon starting and immediately collects
per-second metrics.

Netdata can immediately collect metrics from these endpoints thanks to 300+ **collectors**, which all come pre-installed
when you install Netdata.

Every collector has two primary jobs:

-   Look for exposed metrics at a pre- or user-defined endpoint.
-   Gather exposed metrics and use additional logic to build meaningful, interactive visualizations.

If the collector finds compatible metrics exposed on the configured endpoint, it begins a per-second collection job. The
Netdata Agent gathers these metrics, sends them to the [database engine for
storage](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md), and immediately [visualizes them
meaningfully](https://github.com/netdata/netdata/blob/master/docs/concepts/visualizations/from-raw-metrics-to-visualization.md) on dashboards.

Each collector comes with a pre-defined configuration that matches the default setup for that application. This endpoint
can be a URL and port, a socket, a file, a web page, and more.

For example, the Nginx collector searches
at `http://127.0.0.1/stub_status`, which is the default endpoint for exposing Nginx metrics. The web log collector for
Nginx or Apache searches at
`/var/log/nginx/access.log` and `/var/log/apache2/access.log`, respectively, both of which are standard locations for
access log files on Linux systems.

The endpoint is user-configurable, as are many other specifics of what a given collector does.

## What can Netdata collect?

<!--To quickly find your answer, see our [list of supported collectors](/collectors/COLLECTORS.md).-->

Generally, Netdata's collectors can be grouped into three types:

-   Systems: Monitor CPU, memory, disk, networking, systemd, eBPF, and much more.
    Every metric exposed by `/proc`, `/sys`, and other Linux kernel sources.
-   Containers: Gather metrics from container agents, like `dockerd` or `kubectl`,
    along with the resource usage of containers and the applications they run.
-   Applications: Collect per-second metrics from web servers, databases, logs,
    message brokers, APM tools, email servers, and much more.

## Collector architecture and terminology

**Collector** is a catch-all term for any Netdata process that gathers metrics from an endpoint. 

While we use _collector_ most often in documentation, release notes, and educational content, you may encounter other
terms related to collecting metrics.

-   **Modules** are a type of collector.
-   **Orchestrators** are external plugins that run and manage one or more modules. They run as independent processes.
    The Go orchestrator is in active development.
    -   go.d.plugin: An orchestrator for data
        collection modules written in `go`.
    -   python.d.plugin: An orchestrator for data collection modules written in
        `python` v2/v3.
    -   charts.d.plugin: An orchestrator for data collection modules written in
        `bash` v4+.
-   **External plugins** gather metrics from external processes, such as a webserver or database, and run as independent
    processes that communicate with the Netdata daemon via pipes.
-   **Internal plugins** gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C`,
    and run as threads within the Netdata daemon.

## Related Documentation

### Related Concepts

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [Registry](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/registry.md)
- [Metrics streaming/replication](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
- [Metrics exporting](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-exporting.md)
- [Metrics collection](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md)
- [Metrics storage](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md)

### Related Tasks

- [Claim existing Agent deployments](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
