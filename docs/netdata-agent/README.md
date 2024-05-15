# Netdata Agent

The Netdata Agent is the main building block in a Netdata ecosystem. It is installed on all monitored systems to monitor system components, containers and applications.

The Netdata Agent is an **observability pipeline in a box** that can either operate standalone, or blend into a bigger pipeline made by more Netdata Agents (Children and Parents).

## Distributed Observability Pipeline

The Netdata observability pipeline looks like in the following graph.

The pipeline is extended by creating Metrics Observability Centralization Points that are linked all together (`from a remote Netdata`, `to a remote Netdata`), so that all Netdata installed become a vast integrated observability pipeline.

```mermaid
stateDiagram-v2
    classDef userFeature fill:#f00,color:white,font-weight:bold,stroke-width:2px,stroke:yellow
    classDef usedByNC fill:#090,color:white,font-weight:bold,stroke-width:2px,stroke:yellow
    Local --> Discover
    Local: Local Netdata
    [*] --> Detect: from a remote Netdata
    Others: 3rd party time-series DBs
    Detect: Detect Anomalies
    Dashboard:::userFeature
    Dashboard: Netdata Dashboards
    3rdDashboard:::userFeature
    3rdDashboard: 3rd party Dashboards
    Notifications:::userFeature
    Notifications: Alert Notifications
    Alerts: Alert Transitions
    Discover --> Collect
    Collect --> Detect
    Store: Store
    Store: Time-Series Database
    Detect --> Store
    Store --> Learn
    Store --> Check
    Store --> Query
    Store --> Score
    Store --> Stream
    Store --> Export
    Query --> Visualize
    Score --> Visualize
    Check --> Alerts
    Learn --> Detect: trained ML models
    Alerts --> Notifications
    Stream --> [*]: to a remote Netdata
    Export --> Others
    Others --> 3rdDashboard
    Visualize --> Dashboard
    Score:::usedByNC
    Query:::usedByNC
    Alerts:::usedByNC
```

1. **Discover**: auto-detect metric sources on localhost, auto-discover metric sources on Kubernetes.
2. **Collect**: query data sources to collect metric samples, using the optimal protocol for each data source. 800+ integrations supported, including dozens of native application protocols, OpenMetrics and StatsD.
3. **Detect Anomalies**: use the trained machine learning models for each metric, to detect in real-time if each sample collected is an outlier (an anomaly), or not.
4. **Store**: keep collected samples and their anomaly status, in the time-series database (database mode `dbengine`) or a ring buffer (database modes `ram` and `alloc`).
5. **Learn**: train multiple machine learning models for each metric collected, learning behaviors and patterns for detecting anomalies.
6. **Check**: a health engine, triggering alerts and sending notifications. Netdata comes with hundreds of alert configurations that are automatically attached to metrics when they get collected, detecting errors, common configuration errors and performance issues.
7. **Query**: a query engine for querying time-series data.
8. **Score**: a scoring engine for comparing and correlating metrics.
9. **Stream**: a mechanism to connect Netdata agents and build Metrics Centralization Points (Netdata Parents).
10. **Visualize**: Netdata's fully automated dashboards for all metrics.
11. **Export**: export metric samples to 3rd party time-series databases, enabling the use of 3rd party tools for visualization, like Grafana.

## Comparison to other observability solutions

1. **One moving part**: Other monitoring solution require maintaining metrics exporters, time-series databases, visualization engines. Netdata has everything integrated into one package, even when [Metrics Centralization Points](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/metrics-centralization-points/README.md) are required, making deployment and maintenance a lot simpler.

2. **Automation**: Netdata is designed to automate most of the process of setting up and running an observability solution. It is designed to instantly provide comprehensive dashboards and fully automated alerts, with zero configuration.

3. **High Fidelity Monitoring**: Netdata was born from our need to kill the console for observability. So, it provides metrics and logs in the same granularity and fidelity console tools do, but also comes with tools that go beyond metrics and logs, to provide a holistic view of the monitored infrastructure (e.g. check [Top Monitoring](https://github.com/netdata/netdata/blob/master/docs/top-monitoring-netdata-functions.md)).  

4. **Minimal impact on monitored systems and applications**: Netdata has been designed to have a minimal impact on the monitored systems and their applications. There are [independent studies](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf) reporting that Netdata excels in CPU usage, RAM utilization, Execution Time and the impact Netdata has on monitored applications and containers.

5. **Energy efficiency**: [University of Amsterdam did a research to find the energy efficiency of monitoring tools](https://twitter.com/IMalavolta/status/1734208439096676680). They tested Netdata, Prometheus, ELK, among other tools. The study concluded that **Netdata is the most energy efficient monitoring tool**.

## Dashboard Versions

The Netdata agents (Standalone, Children and Parents) **share the dashboard** of Netdata Cloud. However, when the user is logged-in and the Netdata agent is connected to Netdata Cloud, the following are enabled (which are otherwise disabled):

1. **Access to Sensitive Data**: Some data, like systemd-journal logs and several [Top Monitoring](https://github.com/netdata/netdata/blob/master/docs/top-monitoring-netdata-functions.md) features expose sensitive data, like IPs, ports, process command lines and more. To access all these when the dashboard is served directly from a Netdata agent, Netdata Cloud is required to verify that the user accessing the dashboard has the required permissions.

2. **Dynamic Configuration**: Netdata agents are configured via configuration files, manually or through some provisioning system. The latest Netdata includes a feature to allow users change some of the configuration (collectors, alerts) via the dashboard. This feature is only available to users of paid Netdata Cloud plan.
