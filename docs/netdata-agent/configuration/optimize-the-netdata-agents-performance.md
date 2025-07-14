# Agent Performance Optimization Guide

While Netdata Agents work seamlessly out-of-the-box with comprehensive monitoring, you can tune their configuration for better performance when needed.

## Why optimize your Agent

By default, your Netdata Agent provides:

- **Automatic Application Discovery**: Continuously detects and monitors applications on your node
- **Real-time Metric Collection**: Collects metrics every second  
- **Health Monitoring**: Actively tracks health status with built-in alerting
- **Machine Learning**: Trains anomaly detection models for each metric ([Anomaly Detection](/src/ml/README.md))

These features deliver comprehensive monitoring but consume system resources. You might need to optimize when running Agents on resource-constrained systems or when scaling your monitoring infrastructure.

:::note

See [Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md) for detailed Agent resource requirements.

:::

## How to optimize performance

Here's how each optimization strategy reduces resource usage:

| Optimization Strategy | Reduces CPU | Reduces RAM | Reduces Disk IO |
|----------------------|-------------|-------------|-----------------|
| [Set up Parent-Child architecture](#set-up-parent-child-architecture) | ✓ | ✓ | ✓ |
| [Disable unneeded collectors](#disable-unneeded-collectors) | ✓ | ✓ | ✓ |
| [Reduce collection frequency](#reduce-collection-frequency) | ✓ | | ✓ |
| [Adjust metric retention](#adjust-metric-retention) | | ✓ | ✓ |
| [Switch to RAM mode](#switch-to-ram-mode) | | ✓ | ✓ |
| [Turn off ML on Children](#turn-off-ml-on-children) | ✓ | | |

## Set up Parent-Child architecture

Transform your monitoring by using Parent nodes as centralization points. Parents collect and aggregate data from multiple Child nodes, significantly reducing the load on individual systems.

In this setup:
- **Children** stream their metrics to Parents instead of storing everything locally
- **Parents** handle data aggregation, storage, and dashboard queries
- **You** access all metrics through the Parent nodes

:::tip
This architecture works especially well in production environments where you monitor many systems. Learn more in our [Centralization Points documentation](/docs/observability-centralization-points/README.md).
:::

## Disable unneeded collectors

Reduce resource usage by turning off [Plugins or Collectors](/src/collectors/README.md) you don't need.

:::warning Important
Only active collectors consume resources. Inactive plugins and collectors shut down automatically, so you only save resources by disabling those currently running and collecting metrics.
:::

Follow our [configuration guide](/src/collectors/REFERENCE.md) to identify and disable specific collectors.

## Reduce collection frequency

Save CPU and disk IO by collecting metrics less frequently. If you don't need per-second precision, or if your Agent consumes too much CPU during periods of low dashboard activity, increase the collection interval.

This change:
- Significantly reduces CPU usage
- Decreases disk write operations  
- Maintains meaningful monitoring capabilities

Learn how to adjust collection frequency in our [configuration guide](/src/collectors/REFERENCE.md).

## Adjust metric retention

Control memory and disk usage by changing how long your Agent stores historical data. Shorter retention periods mean:
- Less RAM needed for in-memory metrics
- Reduced disk space requirements
- Faster Agent startup times

Configure retention settings using our [database configuration guide](/src/database/CONFIGURATION.md).

## Switch to RAM mode

For IoT devices and Child nodes in [Parent-Child setups](/docs/observability-centralization-points/README.md), switch to RAM mode to eliminate disk operations entirely. This mode:
- Stores all metrics in memory only
- Eliminates disk IO for metric storage
- Significantly reduces overall resource usage

:::tip

Since Child nodes stream metrics to Parents, they don't need persistent local storage. RAM mode is ideal for this use case.

:::

Set up RAM mode following our [database configuration guide](/src/database/CONFIGURATION.md).

## Turn off ML on Children

Optimize resource allocation by running Machine Learning only where it matters most. We recommend:
- **Enable ML on Parents**: They have the complete data picture and typically more resources
- **Disable ML on Children**: They focus on collecting and streaming metrics

To disable ML, edit your configuration using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config):

```text
[ml]
   enabled = no
```

:::tip

This configuration particularly benefits Child nodes, allowing them to focus on their primary role of collecting and streaming metrics to Parent nodes where ML analysis happens centrally.

:::

## Next steps

1. **Identify your needs**: Determine whether you need optimization for resource constraints or architectural efficiency
2. **Start with architecture**: If monitoring multiple systems, implement Parent-Child setup first
3. **Fine-tune individual Agents**: Apply specific optimizations based on each system's role and resources
4. **Monitor the impact**: Use Netdata dashboards to verify your optimizations maintain the monitoring coverage you need