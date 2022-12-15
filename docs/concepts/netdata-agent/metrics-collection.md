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
per-second metrics. **Collector** is a catch-all term for any Netdata process that gathers metrics from an endpoint.

Every collector has two primary jobs:

- Look for exposed metrics at a pre- or user-defined endpoint.
- Gather exposed metrics and use additional logic to build meaningful, interactive visualizations.

### Plugins & Orchestrators

Plugins are the fundamental building blocks which aggregate or collect metrics. A plugin is responsible for one or more
data collection process. We distinguish our plugins in:

- **Orchestrators** are external plugins that run and manage one or more modules. They run as independent processes.
    - go.d.plugin: An orchestrator for data collection modules written in `go`.
    - python.d.plugin: An orchestrator for data collection modules written in
      `python` v2/v3.
    - charts.d.plugin: An orchestrator for data collection modules written in
      `bash` v4+.
- **External plugins** gather metrics from external processes, such as a webserver or database, and run as independent
  processes that communicate with the Netdata daemon via pipes.
- **Internal plugins** gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C`, and
  run as threads within the Netdata daemon.

When you apply configuration changes in any plugin, it affects all the modules/processes it manages. For instance if you
update the `update_every = X` frequency of the `go.d.plugin` in the `netdata.conf` it will affect all the modules it
manages by c.

### Modules

Modules are entities that include one or more metric collection jobs for a single topic, for example, all the metric
collection jobs as far as an _Apache Webserver_ are managed and configured by a single module.

### Job

Every metric collection process is a job. For instance when you want to monitor N Nginx servers from a single Netdata
deployment, you don't have to configure N Nginx Netdata modules, you just have to specify N Jobs in the Nginx module.

### Auto detection

Netdata has reached and found out every default configuration option for any component it monitors. This give you the
ability to monitor any component that follows the <u>default setup configuration</u> out of the box. So if a collector
finds compatible metrics exposed on the configured endpoint, the Netdata Agent gathers these metrics and produces the
corresponding charts. Periodically Netdata checks for new endpoints or endpoints that wasn't ready in its start up.

For instance, the Nginx collector searches at `http://127.0.0.1/stub_status`, which is the default endpoint for exposing
Nginx metrics. The web log collector for
Nginx searches at
`/var/log/nginx/access.log` and `/var/log/apache2/access.log`, respectively, both of which are standard locations for
access log files on Linux systems.

The endpoint is user-configurable, as are many other specifics of what a given collector does.

### Related Documentation

#### Related Concepts

- [ACLK](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md)
- [Registry](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/registry.md)
- [Metrics streaming/replication](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md)
- [Metrics exporting](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-exporting.md)
- [Metrics collection](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md)
- [Metrics storage](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md)

#### Related Tasks

- [Claim existing Agent deployments](https://github.com/netdata/netdata/blob/master/docs/tasks/setup/claim-existing-agent-to-cloud.md)
