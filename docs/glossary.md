
[A](#a) | [B](#b) | [C](#c) | [D](#d)| [E](#e) | [F](#f) | [G](#g) | [H](#h) | [I](#i) | [J](#j) | [K](#k) | [L](#l) | [M](#m) | [N](#n) | [O](#o) | [P](#p) 
| [Q](#q) | [R](#r) | [S](#s) | [T](#t) | [U](#u) | [V](#v) | [W](#w) | [X](#x) | [Y](#y) | [Z](#Z)

## A

- [**Agent-cloud link** or **ACLK**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/aclk.md): The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser through Netdata Cloud. 

- [**Alerts** (formerly **Alarms**](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md): With the information that appears on Netdata Cloud and the local dashboard about active alerts, you can configure alerts to match your infrastructure's needs or your team's goals.

- [Alarm Entity Type](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md#entity-types): Entity types that are attached to specific charts and use the `alarm` label.

- [**Anomaly Advisor**](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/machine-learning-powered-anomaly-advisor.md): A Netdata feature that lets you quickly surface potentially anomalous metrics and charts related to a particular highlight window of interest.

## C

- [**Child**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/netdata-agent/metrics-streaming-replication.md#streaming-basics): A node, running Netdata, that streams metric data to one or more parent.

- [**Collector**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md#collector-architecture-and-terminology): A catch-all term for any Netdata process that gathers metrics from an endpoint.

- [**Community**](https://github.com/netdata/netdata/blob/rework-learn/docs/getting-started/introduction.md#community): As a company with a passion and genesis in open-source, we are not just very proud of our community, but we consider our users, fans, and chatters to be an imperative part of the Netdata experience and culture.

## D

- [**Distributed Architecture**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/distributed-data-architecture.md): The data architecture mindset with which Netdata was built, where all data are collected and stored on the edge, whenever it's possible, creating countless benefits. 

## E 

- [**External Plugins**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md#collector-architecture-and-terminology): These gather metrics from external processes, such as a webserver or database, and run as independent processes that communicate with the Netdata daemon via pipes.

## G

- [**Guided Troubleshooting**](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/Overview.md): Troubleshooting with our Machine-Learning-powered tools designed to give you a cutting edge advantage in your troubleshooting battles.

## H

- [**Headless Collector Streaming**](https://github.com/netdata/netdata/edit/master/docs/concepts/netdata-agent/metrics-streaming-replication.md): Streaming configuration where child `A`, _without_ a database or web dashboard, streams metrics to parent `B`.

- [**Health Configuration Files**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/health-monitoring/alerts.md#health-configuration-files): Files that you can edit to configure your Agent's health watchdog service.

- [**Health Entity Reference**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/health-monitoring/alerts.md#health-entity-reference): 
-
- [**High Fidelity** or **High Fidelity Architecture**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/high-fidelity-monitoring.md): We consider Netdata's monitoring solution "high fidelity" because it provides real time metrics so you can view metrics/changes in seconds since their occur, the highest resolution of metrics to allow you to observe changes occur between seconds, gixed step metric collection to allow you to quantify your observation windows, and unlimited data to search for patterns in data that you don't even believe they are correlated.

## I

- [**Internal plugins**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md#collector-architecture-and-terminology): These gather metrics from `/proc`, `/sys`, and other Linux kernel sources. They are written in `C` and run as threads within the Netdata daemon.

## K

- [**Kickstart** or **Kickstart Script**](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/kickstart.md): An automatic one-line installation script named 'kickstart.sh' that works on all Linux distributions and macOS.

## M

- [**Metrics Collection**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md):  With zero configuration, Netdata auto-detects thousands of data sources upon starting and immediately collects per-second metrics. Netdata can immediately collect metrics from these endpoints thanks to 300+ collectors, which all come pre-installed when you install Netdata.

- [**Metric Correlations**](https://github.com/netdata/netdata/blob/master/docs/concepts/guided-troubleshooting/metric-correlations.md): A Netdata feature that lets you quickly find metrics and charts related to a particular window of interest that you want to explore further.

- [**Metrics Exporting**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-exporting.md): Netdata allows you to export metrics to external time-series databases with the exporting engine. This system uses a number of connectors to initiate connections to more than thirty supported databases, including InfluxDB, Prometheus, Graphite, ElasticSearch, and much more.

- [**Metrics Storage**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-storage.md): Upon collection the collected metrics need to be either forwarded, exported or just stored for further treatment. The Agent is capable to store metrics both short and long-term, with or without the usage of non-volatile storage.

- [**Metrics Streaming Replication**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md): Each node running Netdata can stream the metrics it collects, in real time, to another node. Metric streaming allows you to replicate metrics data across multiple nodes, or centralize all your metrics data into a single time-series database (TSDB).

- [**Module**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md#collector-architecture-and-terminology): A type of collector. 

## N

- [**Netdata**](https://github.com/netdata/netdata/blob/master/docs/getting-started/introduction.md): Netdata is a monitoring tool designed by system administrators, DevOps engineers, and developers to collect everything, help you visualize
metrics, troubleshoot complex performance problems, and make data interoperable with the rest of your monitoring stack. 

## O

- [**Orchestrators**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-collection.md#collector-architecture-and-terminology): External plugins that run and manage one or more modules. They run as independent processes.

## P

- [**Parent**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md#streaming-basics): A node, running Netdata, that receives streamed metric data.

- [**Proxy**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md#streaming-basics): A node, running Netdata, that receives metric data from a child and "forwards" them on to a separate parent node.

- [**Proxy Streaming**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-agent/metrics-streaming-replication.md#supported-streaming-configurations): Streaming configuration where child `A`, _with or without_ a database, sends metrics to proxy `C`, also _with or without_ a database. `C` sends metrics to parent `B`

## R

- [**Registry**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/netdata-agent/registry.md): Registry that allows Netdata to provide unified cross-server dashboards.

- [Replication Streaming](https://github.com/netdata/netdata/edit/rework-learn/docs/concepts/netdata-agent/metrics-streaming-replication.md): Streaming configuration where child `A`, _with_ a database and web dashboard, streams metrics to parent `B`. 

## T

- [**Template Entity Type**](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md#entity-types): Entity type that defines rules that apply to all charts of a specific context, and use the template label. Templates help you apply one entity to all disks, all network interfaces, all MySQL databases, and so on.

- [**Tiering**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/netdata-agent/metrics-storage.md#tiering): Tiering is a mechanism of providing multiple tiers of data with different granularity of metrics (the frequency they are collected and stored, i.e. their resolution).

## U

- [**Unlimited Scalability**](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-architecture/unlimited-scalability.md): With Netdata's distributed architecture, you can seamless observe a couple, hundreds or 
even thousands of nodes. There are no actual bottlenecks especially if you retain metrics locally in the Agents.

## Z

- [**Zero Configuration**](https://github.com/netdata/netdata/blob/rework-learn/docs/concepts/netdata-architecture/zero-configuration.md): Netdata is preconfigured and capable to autodetect and monitor any well known application that runs on your system. You just deploy and claim Netdata Agents in your Netdata space, and monitor them in seconds. 
