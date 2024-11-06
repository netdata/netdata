# Optimize the Agent's performance

By default, Agents will automatically detect applications running on the node and start collecting metrics in real-time, have health monitoring enabled and train Machine Learning models for each metric to use in [Anomaly Detection](/src/ml/README.md).

> **Note**
>
> Check the [Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md) section to read more about the default requirements an Agent has.

This document describes the strategies to optimize Netdata in order to better suit your scenario.

## Summary of performance optimizations

The following table summarizes the effect of each optimization on the CPU, RAM and Disk IO utilization in production.

| Optimization                                                                                                                      | CPU                | RAM                | Disk IO            |
|-----------------------------------------------------------------------------------------------------------------------------------|--------------------|--------------------|--------------------|
| [Use Centralization Points](#use-centralization-points)                                                                           | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Disable Plugins or Collectors](#disable-plugins-or-collectors)                                                 | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Reduce data collection frequency](#reduce-data-collection-frequency)                                                                  | :heavy_check_mark: |                    | :heavy_check_mark: |
| [Lower memory usage for metric retention](#lower-memory-usage-for-metric-retention) |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Change Database mode](#change-database-mode)                                                                |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Disable ML on Children](#disable-machine-learning-on-children)                                                                   | :heavy_check_mark: |                    |                    |
| [Use a reverse proxy](#run-netdata-behind-a-proxy)                                                                                | :heavy_check_mark: |                    |                    |

## Use Centralization Points

For production environments, Parent nodes outside the production infrastructure should be receiving all collected data from Children running on the production infrastructure, using [Centralization Points](/docs/observability-centralization-points/README.md).

## Disable Plugins or Collectors

If you know that you don't need an [entire Plugin or a specific Collector](/src/collectors/README.md#collector-architecture-and-terminology), you can disable them.

> **Note**
>
> Keep in mind that if a Plugin or a Collector has nothing to collect, it simply shuts down and doesn’t consume system resources. You will only improve the Agent's performance by disabling Plugins/Collectors that are actively collecting metrics.

Check our documentation on [instructions for disabling Plugins and Collectors](/docs/netdata-agent/configuration/collectors/enable-or-disable-collectors-and-plugins.md)

## Reduce data collection frequency

The fastest way to improve the Agent's resource utilization is to reduce how often it collects metrics.

If you don't need per-second metrics, or if the Netdata Agent uses a lot of CPU even when no one is viewing that node's dashboard, [configure the Agent to collect metrics less often](/docs/netdata-agent/configuration/collectors/data-collection-frequency.md).

## Lower memory usage for metric retention

If you don't need to store metrics at high resolution for a long period of time, check our document on [changing how long Netdata stores metrics](/docs/netdata-agent/configuration/optimizing-metrics-database/change-metrics-storage.md).

## Change Database mode

You can use a different [Database mode](/src/database/README.md#select-database-mode) when running Netdata on IoT devices, and for Children in [Centralization Point setups](/docs/observability-centralization-points/README.md).

## Disable Machine Learning on Children

We recommend ML to only be enabled on Parents that sit outside your production infrastructure, or if you have cpu and memory to spare.

On less powerful systems, and Children, you can disable ML by opening `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) and setting:

```text
[ml]
   enabled = no
```

## Run Netdata behind a proxy

A dedicated web server like Nginx can handle more concurrent connections than the Agent's internal [web server](/src/web/README.md), reuse idle connections, and use fast gzip compression to reduce payloads.

For details on installing another web server as a proxy for the local Agent dashboard, see [reverse proxies](/docs/netdata-agent/configuration/running-the-netdata-agent-behind-a-reverse-proxy/README.md).
