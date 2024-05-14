# Glossary

The Netdata community welcomes engineers, SREs, admins, etc. of all levels of expertise with engineering and the Netdata tool. And just as a journey of a thousand miles starts with one step, sometimes, the journey to mastery begins with understanding a single term.

As such, we want to provide a little Glossary as a reference starting point for new users who might be confused about the Netdata vernacular that more familiar users might take for granted.

If you're here looking for the definition of a term you heard elsewhere in our community or products, or if you just want to learn Netdata from the ground up, you've come to the right page.

Use the alphabatized list below to find the answer to your single-term questions, and click the bolded list items to explore more on the topics! We'll be sure to keep constantly updating this list, so if you hear a word that you would like for us to cover, just let us know or submit a request!

[A](#a) | [B](#b) | [C](#c) | [D](#d)| [E](#e) | [F](#f) | [G](#g) | [H](#h) | [I](#i) | [J](#j) | [K](#k) | [L](#l) | [M](#m) | [N](#n) | [O](#o) | [P](#p)
| [Q](#q) | [R](#r) | [S](#s) | [T](#t) | [U](#u) | [V](#v) | [W](#w) | [X](#x) | [Y](#y) | [Z](#z)

## A

- [**Agent** or **Netdata Agent**](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md): Netdata's distributed monitoring Agent collects thousands of metrics from systems, hardware, and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.

- [**Agent-cloud link** or **ACLK**](https://github.com/netdata/netdata/blob/master/src/aclk/README.md): The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser through Netdata Cloud.

- [**Aggregate Function**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#aggregate-functions-over-time): A function applied When the granularity of the data collected is higher than the plotted points on the chart.

- [**Alerts** (formerly **Alarms**)](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md): With the information that appears on Netdata Cloud and the local dashboard about active alerts, you can configure alerts to match your infrastructure's needs or your team's goals.

- [**Alarm Entity Type**](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#health-entity-reference): Entity types that are attached to specific charts and use the `alarm` label.

- [**Anomaly Advisor**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/anomaly-advisor-tab.md): A Netdata feature that lets you focus on potentially anomalous metrics and charts related to a particular highlight window of interest.

## B

- [**Bookmarks**](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#manage-spaces): Netdata Cloud's bookmarks put your tools in one accessible place. Bookmarks are shared between all War Rooms in a Space, so any users in your Space will be able to see and use them.

## C

- [**Child**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md#streaming-basics): A node, running Netdata, that streams metric data to one or more parent.

- [**Cloud** or **Netdata Cloud**](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/README.md): Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata Cloud, you can view key metrics, insightful charts, and active alerts from all your nodes in a single web interface.

- [**Collector**](https://github.com/netdata/netdata/blob/master/src/collectors/README.md#collector-architecture-and-terminology): A catch-all term for any Netdata process that gathers metrics from an endpoint.

- [**Community**](https://community.netdata.cloud/): As a company with a passion and genesis in open-source, we are not just very proud of our community, but we consider our users, fans, and chatters to be an imperative part of the Netdata experience and culture.

- [**Context**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#contexts): A way of grouping charts by the types of metrics collected and dimensions displayed. It's kind of like a machine-readable naming and organization scheme.

- [**Custom dashboards**](https://github.com/netdata/netdata/blob/master/src/web/gui/custom/README.md) A dashboard that you can create using simple HTML (no javascript is required for basic dashboards).

## D

- [**Dashboard**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md): Out-of-the-box visual representation of metrics to provide insight into your infrastructure, its health and performance.

- [**Definition Bar**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md): Bar within a composite chart that provides important information and options about the metrics within the chart.

- [**Dimension**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#dimensions): A dimension is a value that gets shown on a chart.

## E

- [**External Plugins**](https://github.com/netdata/netdata/blob/master/src/collectors/plugins.d/README.md): These gather metrics from external processes, such as a webserver or database, and run as independent processes that communicate with the Netdata daemon via pipes.

## F

- [**Family**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#families): 1. What we consider our Netdata community of users and engineers. 2. A single instance of a hardware or software resource that needs to be displayed separately from similar instances.

- [**Flood Protection**](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#flood-protection): If a node has too many state changes like firing too many alerts or going from reachable to unreachable, Netdata Cloud enables flood protection. As long as a node is in flood protection mode, Netdata Cloud does not send notifications about this node

- [**Functions** or **Netdata Functions**](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md): Routines exposed by a collector on the Netdata Agent that can  bring additional information to support troubleshooting or trigger some action to happen on the node itself.

## G

- [**Group by**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/netdata-charts.md#group-by-dimension-node-or-chart): The drop-down on the dimension bar of a composite chart that allows you to group metrics by dimension, node, or chart.

## H

- [**Headless Collector Streaming**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md#supported-streaming-configurations): Streaming configuration where child `A`, _without_ a database or web dashboard, streams metrics to parent `B`.

- [**Health Configuration Files**](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#edit-health-configuration-files): Files that you can edit to configure your Agent's health watchdog service.

- [**Health Entity Reference**](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#health-entity-reference):

- [**Home** tab](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/home-tab.md): Tab in Netdata Cloud that provides a predefined dashboard of relevant information about entities in the War Room.

## I

- [**Internal plugins**](https://github.com/netdata/netdata/blob/master/src/collectors/README.md#collector-architecture-and-terminology): These gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C` and run as threads within the Netdata daemon.

## K

- [**Kickstart** or **Kickstart Script**](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md): An automatic one-line installation script named 'kickstart.sh' that works on all Linux distributions and macOS.

- [**Kubernetes Dashboard** or **Kubernetes Tab**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/kubernetes-tab.md): Netdata Cloud features enhanced visualizations for the resource utilization of Kubernetes (k8s) clusters, embedded in the default Overview dashboard.

## M

- [**Metrics Collection**](https://github.com/netdata/netdata/blob/master/src/collectors/README.md):  With zero configuration, Netdata auto-detects thousands of data sources upon starting and immediately collects per-second metrics. Netdata can immediately collect metrics from these endpoints thanks to 300+ collectors, which all come pre-installed when you install Netdata.

- [**Metric Correlations**](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md): A Netdata feature that lets you quickly find metrics and charts related to a particular window of interest that you want to explore further.

- [**Metrics Exporting**](https://github.com/netdata/netdata/blob/master/docs/exporting-metrics/README.md): Netdata allows you to export metrics to external time-series databases with the exporting engine. This system uses a number of connectors to initiate connections to more than thirty supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much more.

- [**Metrics Storage**](https://github.com/netdata/netdata/blob/master/docs/store/change-metrics-storage.md): Upon collection the collected metrics need to be either forwarded, exported or just stored for further treatment. The Agent is capable to store metrics both short and long-term, with or without the usage of non-volatile storage.

- [**Metrics Streaming Replication**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md): Each node running Netdata can stream the metrics it collects, in real time, to another node. Metric streaming allows you to replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series database (TSDB).

- [**Module**](https://github.com/netdata/netdata/blob/master/src/collectors/REFERENCE.md#enable-and-disable-a-specific-collection-module): A type of collector.

## N

- [**Netdata**](https://www.netdata.cloud/): Netdata is a monitoring tool designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack.

- [**Netdata Agent** or **Agent**](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md): Netdata's distributed monitoring Agent collects thousands of metrics from systems, hardware, and applications with zero configuration. It runs permanently on all your physical/virtual servers, containers, cloud deployments, and edge/IoT devices.

- [**Netdata Cloud** or **Cloud**](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/README.md): Netdata Cloud is a web application that gives you real-time visibility for your entire infrastructure. With Netdata Cloud, you can view key metrics, insightful charts, and active alerts from all your nodes in a single web interface.

- [**Netdata Functions** or **Functions**](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md): Routines exposed by a collector on the Netdata Agent that can  bring additional information to support troubleshooting or trigger some action to happen on the node itself.

<!-- No link for this keyword - **Netdata Logs** https://github.com/netdata/netdata/blob/master/docs/tasks/miscellaneous/check-netdata-logs.md: The three log files - `error.log`, `access.log` and `debug.log` - used by Netdata -->

<!-- Here we need to explain Agent notifications and Cloud notifications, not just "notifications"

- **Notifications** https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/notifications.md: Netdata can send centralized alert notifications to your team whenever a node enters a warning, critical, or unreachable state. By enabling notifications, you ensure no alert, on any node in your infrastructure, goes unnoticed by you or your team. -->

## O

- [**Obsoletion**(of nodes)](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#obsoleting-offline-nodes-from-a-space): Removing nodes from a space.

- [**Orchestrators**](https://github.com/netdata/netdata/blob/master/src/collectors/README.md#collector-architecture-and-terminology): External plugins that run and manage one or more modules. They run as independent processes.

## P

- [**Parent**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md#streaming-basics): A node, running Netdata, that receives streamed metric data.

- [**Proxy**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md#streaming-basics): A node, running Netdata, that receives metric data from a child and "forwards" them on to a separate parent node.

- [**Proxy Streaming**](https://github.com/netdata/netdata/blob/master/docs/metrics-storage-management/enable-streaming.md#supported-streaming-configurations): Streaming configuration where child `A`, _with or without_ a database, sends metrics to proxy `C`, also _with or without_ a database. `C` sends metrics to parent `B`

## R

- [**Registry**](https://github.com/netdata/netdata/blob/master/src/registry/README.md): Registry that allows Netdata to provide unified cross-server dashboards.

- [**Replication Streaming**](https://github.com/netdata/netdata/blob/master/src/streaming/README.md): Streaming configuration where child `A`, _with_ a database and web dashboard, streams metrics to parent `B`.

- [**Room** or **War Room**](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms): War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

## S

- [**Single Node Dashboard**](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md): A dashboard pre-configured with every installation of the Netdata agent, with thousand of metrics and hundreds of interactive charts that requires no set up.

- [**Space**](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-spaces): A high-level container and virtual collaboration area where you can organize team members, access levels,and the nodes you want to monitor.

## T

- [**Template Entity Type**](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md#entity-types): Entity type that defines rules that apply to all charts of a specific context, and use the template label. Templates help you apply one entity to all disks, all network interfaces, all MySQL databases, and so on.

- [**Tiers**](https://github.com/netdata/netdata/blob/master/src/database/engine/README.md#tiers): Tiering is a mechanism of providing multiple tiers of data with different granularity of metrics (the frequency they are collected and stored, i.e. their resolution).

## U

- [**Unlimited Scalability**](https://www.netdata.cloud/#:~:text=love%20community%20contributions!-,Infinite%20Scalability,-By%20storing%20data): With Netdata's distributed architecture, you can seamless observe a couple, hundreds or
even thousands of nodes. There are no actual bottlenecks especially if you retain metrics locally in the Agents.

## V

- [**Visualizations**](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/visualizations-overview.md): Netdata uses dimensions, contexts, and families to sort your metric data into graphs, charts, and alerts that maximize your understand of your infrastructure and your ability to troubleshoot it, along or on a team.

## W

- [**War Room** or **Room**](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/organize-your-infrastrucutre-invite-your-team.md#netdata-cloud-war-rooms): War Rooms organize your connected nodes and provide infrastructure-wide dashboards using real-time metrics and visualizations.

## Z

- **Zero Configuration**: Netdata is preconfigured and capable to autodetect and monitor any well known application that runs on your system. You just deploy and claim Netdata Agents in your Netdata space, and monitor them in seconds.
