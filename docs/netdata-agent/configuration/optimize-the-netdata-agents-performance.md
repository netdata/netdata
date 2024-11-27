# Agent Performance Optimization Guide

While Netdata Agents prioritize simplicity and out-of-the-box functionality, their default configuration focuses on comprehensive monitoring rather than performance optimization.

By default, Agents provide:

- **Automatic Application Discovery**: Continuously detects and monitors applications running on your node without manual configuration.
- **Real-time Metric Collection**: Collects metrics with one-second granularity.
- **Health Monitoring**: Actively tracks the health status of your applications and system components with built-in alerting.
- **Machine Learning**: Trains models for each metric to detect anomalies and unusual patterns in your system's behavior ([Anomaly Detection](/src/ml/README.md)).

> **Note**
>
> For details about Agent resource requirements, see [Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

This document describes various strategies to optimize Netdata's performance for your specific monitoring needs.

## Summary of performance optimizations

The following table summarizes the effect of each optimization on the CPU, RAM and Disk IO utilization in production.

| Optimization                                                              | CPU                | RAM                | Disk IO            |
|---------------------------------------------------------------------------|--------------------|--------------------|--------------------|
| [Implement Centralization Points](#implement-centralization-points)       | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Disable Plugins or Collectors](#disable-plugins-or-collectors)           | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| [Adjust data collection frequency](#adjust-data-collection-frequency)     | :heavy_check_mark: |                    | :heavy_check_mark: |
| [Optimize metric retention settings](#optimize-metric-retention-settings) |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Select appropriate Database Mode](#select-appropriate-database-mode)     |                    | :heavy_check_mark: | :heavy_check_mark: |
| [Disable ML on Children](#disable-machine-learning-on-children)           | :heavy_check_mark: |                    |                    |

## Implement Centralization Points

In production environments, use Parent nodes as centralization points to collect and aggregate data from Child nodes across your infrastructure. This architecture follows our recommended [Centralization Points](/docs/observability-centralization-points/README.md) pattern.

## Disable Plugins or Collectors

You can improve Agent performance by selectively disabling [Plugins or Collectors](/src/collectors/README.md) that you don't need for your monitoring requirements.

> **Note**
>
> Inactive Plugins and Collectors automatically shut down and don't consume system resources. Performance benefits come only from disabling those that are actively collecting metrics.

For detailed instructions on managing Plugins and Collectors, see [configuration guide](/src/collectors/REFERENCE.md).

## Adjust Data Collection Frequency

One of the most effective ways to reduce the Agent's resource consumption is to modify its data collection frequency.

If you don't require per-second precision, or if your Agent is consuming excessive CPU during periods of low dashboard activity, you can [reduce the collection frequency](/src/collectors/REFERENCE.md).
This adjustment can significantly improve CPU utilization while maintaining meaningful monitoring capabilities.

## Optimize Metric Retention Settings

You can reduce memory and disk usage by adjusting [how long the Agent stores metrics](/src/database/CONFIGURATION.md).

## Select Appropriate Database Mode

For IoT devices and Child nodes in [Centralization Point setups](/docs/observability-centralization-points/README.md), you can optimize performance by [switching to RAM mode](/src/database/CONFIGURATION.md). This can significantly reduce resource usage while maintaining essential monitoring capabilities.

## Disable Machine Learning on Children

For optimal resource allocation, we recommend running Machine Learning only on Parent nodes, or on systems with sufficient CPU and memory capacity.

To reduce resource usage on Child nodes or less powerful systems, you can disable ML by modifying `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config):

```text
[ml]
   enabled = no
```

This configuration is particularly beneficial for Child nodes since their primary role is to collect and stream metrics to Parent nodes, where ML analysis can be performed centrally.
