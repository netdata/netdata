# Glossary

Welcome to Netdata's terminology guide! Whether you're a seasoned engineer or just starting your journey in system monitoring, this glossary helps demystify the terms used throughout our documentation and community.

This glossary serves as a living document, continuously updated to help you understand Netdata concepts, features, and terminology. Each term links to detailed documentation for deeper exploration.

Missing a term? Let us know or submit a request to expand our glossary. Together, we can make Netdata more accessible to everyone.

[A](#a) | [B](#b) | [C](#c) | [D](#d)| [E](#e) | [F](#f) | [G](#g) | [H](#h) | [I](#i) | [J](#j) | [K](#k) | [L](#l) | [M](#m) | [N](#n) | [O](#o) | [P](#p)
| [Q](#q) | [R](#r) | [S](#s) | [T](#t) | [U](#u) | [V](#v) | [W](#w) | [X](#x) | [Y](#y) | [Z](#z)

## A

- [**Agent** or **Netdata Agent**](/packaging/installer/README.md): Netdata's distributed monitoring Agent collects thousands of metrics from systems, hardware, and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.

- [**Agent-cloud link** or **ACLK**](/src/aclk/README.md): The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser through Netdata Cloud.

- [**Aggregate Function**](/docs/dashboards-and-charts/netdata-charts.md#aggregate-functions-over-time): A function applied When the granularity of the data collected is higher than the plotted points on the chart.

- [**Alerts** (formerly **Alarms**)](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md): With the information that appears on Netdata Cloud and the local dashboard about active alerts, you can configure alerts to match your infrastructure's needs or your team's goals.

- [**Alarm Entity Type**](/src/health/REFERENCE.md#entity-types-overview): Entity types that are attached to specific charts and use the `alarm` label.

- [**Anomaly Advisor**](/docs/dashboards-and-charts/anomaly-advisor-tab.md): A Netdata feature that lets you focus on potentially anomalous metrics and charts related to a particular highlight window of interest.

## C

- [**Child**](/docs/observability-centralization-points/metrics-centralization-points/README.md): A node, running Netdata, that streams metric data to one or more Parents.

- [**Cloud** or **Netdata Cloud**](/docs/netdata-cloud/README.md): Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata Cloud, you can view key metrics, insightful charts, and active alerts from all your nodes in a single web interface.

- [**Collector**](/src/collectors/README.md): A catch-all term for any Netdata process that gathers metrics from an endpoint.

- [**Community**](https://community.netdata.cloud/): As a company with a passion and genesis in open-source, we are not just very proud of our community, but we consider our users, fans, and chatters to be an imperative part of the Netdata experience and culture.

- [**Context**](/docs/dashboards-and-charts/netdata-charts.md#contexts): A way of grouping charts by the types of metrics collected and dimensions displayed. It's kind of like a machine-readable naming and organization scheme.

## D

- [**Dashboard**](/docs/dashboards-and-charts/README.md): Out-of-the-box visual representation of metrics to provide insight into your infrastructure, its health and performance.

- [**Definition Bar**](/docs/dashboards-and-charts/netdata-charts.md): Bar within a composite chart that provides important information and options about the metrics within the chart.

- [**Dimension**](/docs/dashboards-and-charts/netdata-charts.md#dimensions): A dimension is a value that gets shown on a chart.

## E

- [**External Plugins**](/src/plugins.d/README.md): These gather metrics from external processes, such as a webserver or database, and run as independent processes that communicate with the Netdata daemon via pipes.

## F

- [**Family**](/docs/dashboards-and-charts/netdata-charts.md#families): 1. What we consider our Netdata community of users and engineers. 2. A single instance of a hardware or software resource that needs to be displayed separately from similar instances.

- [**Flood Protection**](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#flood-protection): If a node has too many state changes like firing too many alerts or going from reachable to unreachable, Netdata Cloud enables flood protection. As long as a node is in flood protection mode, Netdata Cloud doesn’t send notifications about this node

- [**Functions** or **Netdata Functions**](/docs/top-monitoring-netdata-functions.md): Routines exposed by a collector on the Netdata Agent that can bring additional information to support troubleshooting or trigger some action to happen on the node itself.

## G

- [**Group by**](/docs/dashboards-and-charts/netdata-charts.md#group-by-dropdown): The drop-down on the dimension bar of a composite chart that allows you to group metrics by dimension, node, or chart.

- [**Health Configuration Files**](/src/health/REFERENCE.md#edit-health-configuration-files): Files that you can edit to configure your Agent's health watchdog service.

- [**Home** tab](/docs/dashboards-and-charts/home-tab.md): Tab in Netdata Cloud that provides a predefined dashboard of relevant information about entities in the Room.

## I

- [**Internal plugins**](/src/collectors/README.md): These gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C` and run as threads within the Netdata daemon.

## K

- [**Kickstart** or **Kickstart Script**](/packaging/installer/methods/kickstart.md): An automatic one-line installation script named 'kickstart.sh' that works on all Linux distributions and macOS.

- [**Kubernetes Dashboard** or **Kubernetes Tab**](/docs/dashboards-and-charts/kubernetes-tab.md): Netdata Cloud features enhanced visualizations for the resource utilization of Kubernetes (k8s) clusters, embedded in the default Overview dashboard.

## M

- [**Metrics Collection**](/src/collectors/README.md): With zero configuration, Netdata auto-detects thousands of data sources upon starting and immediately collects per-second metrics. Netdata can immediately collect metrics from these endpoints thanks to 300+ collectors, which all come pre-installed when you install Netdata.

- [**Metric Correlations**](/docs/metric-correlations.md): A Netdata feature that lets you quickly find metrics and charts related to a particular window of interest that you want to explore further.

- [**Metrics Exporting**](/docs/exporting-metrics/README.md): Netdata allows you to export metrics to external time-series databases with the exporting engine. This system uses a number of connectors to initiate connections to more than thirty supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much more.

- [**Metrics Storage**](/src/database/README.md#modes): Upon collection the collected metrics need to be either forwarded, exported or just stored for further treatment. The Agent is capable to store metrics both short and long-term, with or without the usage of non-volatile storage.

- [**Metrics Streaming Replication**](/docs/observability-centralization-points/README.md): Each node running Netdata can stream the metrics it collects, in real time, to another node. Metric streaming allows you to replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series database (TSDB).

## N

- [**Netdata**](https://www.netdata.cloud/): Netdata is a monitoring tool designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
  metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

- [**Netdata Agent** or **Agent**](/packaging/installer/README.md): Netdata's distributed monitoring Agent collects thousands of metrics from systems, hardware, and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.

- [**Netdata Cloud** or **Cloud**](/docs/netdata-cloud/README.md): Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata Cloud, you can view key metrics, insightful charts, and active alerts from all your nodes in a single web interface.

- [**Netdata Functions** or **Functions**](/docs/top-monitoring-netdata-functions.md): Routines exposed by a collector on the Netdata Agent that can bring additional information to support troubleshooting or trigger some action to happen on the node itself.

## O

- [**Obsoletion**(of nodes)](/docs/dashboards-and-charts/nodes-tab.md): Removing nodes from a space.

- [**Orchestrators**](/src/collectors/README.md): External plugins that run and manage one or more modules. They run as independent processes.

## P

- [**Parent**](/docs/observability-centralization-points/metrics-centralization-points/README.md): A node, running Netdata, that receives streamed metric data.

## R

- [**Registry**](/src/registry/README.md): Registry that allows Netdata to provide unified cross-server dashboards.

- [**Replication Streaming**](/src/streaming/README.md): Streaming configuration where child `A`, _with_ a database and web dashboard, streams metrics to parent `B`.

- [**Room**](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#rooms): Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

## S

- [**Single Node Dashboard**](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md): A dashboard pre-configured with every installation of the Netdata Agent, with thousands of metrics and hundreds of interactive charts that requires no setup.

- [**Space**](/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md#spaces): A high-level container and virtual collaboration area where you can organize team members, access levels, and the nodes you want to monitor.

## T

- [**Template Entity Type**](/src/health/REFERENCE.md#entity-types): Entity type that defines rules that apply to all charts of a specific context, and use the template label. Templates help you apply one entity to all disks, all network interfaces, all MySQL databases, and so on.

- [**Tiers**](/src/database/engine/README.md#tiers): Tiering is a mechanism of providing multiple tiers of data with different granularity of metrics (the frequency they are collected and stored, i.e., their resolution).

## U

- [**Unlimited Scalability**](https://www.netdata.cloud/#:~:text=love%20community%20contributions!-,Infinite%20Scalability,-By%20storing%20data): With Netdata's distributed architecture, you can seamlessly observe a couple, hundreds or
  even thousands of nodes. There are no actual bottlenecks especially if you retain metrics locally in the Agents.

## V

- [**Visualizations**](/docs/dashboards-and-charts/README.md): Netdata uses dimensions, contexts, and families to sort your metric data into graphs, charts, and alerts that maximize your understanding of your infrastructure and your ability to troubleshoot it, along or on a team.

## Z

- **Zero Configuration**: Netdata is pre-configured and capable of autodetect and monitor any well-known application that runs on your system. You deploy and connect Netdata Agents in your Netdata space, and monitor them in seconds.
