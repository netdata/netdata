# Fleet Management of Robots and Edge Devices

Netdata can monitor fleets of Linux robots, digital signage players, gateways and autonomous vehicles while keeping most storage and analysis on Netdata Parents. This guide focuses on constrained devices, especially 32-bit ARM systems, and on deployments managed through an image or firmware pipeline.

A small Child collects telemetry on each device and streams it to one or more Parents. Parents provide historical queries, dashboards, alerts and anomaly detection. Device Functions can be invoked through the Parent while the device is connected. Collection and the implementation of those Functions still run on the device: removing a collector removes its telemetry and Functions.

## Design the fleet before shrinking it

Define what operators need to detect, how quickly they must detect it, how long devices can be disconnected, and how much CPU, RAM, disk and cellular traffic each device can spend. Treat these as separate budgets.

| Budget | Main controls | Consequence to assess |
|:--|:--|:--|
| Installed files | Strip debugging information; omit optional plugins, dashboard and data files | Omitted capabilities cannot run on the Child |
| Local disk writes | Local database mode and retention; logging policy | Less local history to recover after an outage |
| Memory | Metric count, retention, enabled plugins, ML, buffers | Smaller buffers tolerate less interruption |
| CPU | Collection interval, enabled collectors, metric count, compression | Slower detection or less telemetry |
| Cellular traffic | Collection interval, chart selection, liveness policy, reconnects | Coarser data and potentially slower disconnect detection |

Stripping debugging information primarily saves disk space. It does not remove collection work. Conversely, disabling a plugin does not remove its installed binary. Use both package preparation and runtime configuration.

## Follow the deployment sequence

1. [Define capabilities and provision devices](./deployment-and-identity.md), including unique identities and secure Parent connections.
2. [Prepare a smaller static package](./minimize-disk-footprint.md) in your build pipeline.
3. [Set CPU and memory budgets](./minimize-cpu-and-memory.md), and verify them on representative hardware.
4. [Tune cellular traffic](./minimize-cellular-traffic.md) using a cadence appropriate to the workload.
5. [Plan disconnection and failover](./disconnected-devices-and-failover.md).
6. [Monitor the fleet and its observability cost](./monitor-the-fleet.md).
7. [Roll out updates and recover safely](./updates-and-troubleshooting.md).

## What a Parent preserves

Centralization preserves the ability to analyze retained telemetry, alert on it and view it centrally. It does not preserve every capability after arbitrary pruning. Keep `apps` for process telemetry and process Functions, `network` for network inspection, `journal` for journal Functions, and the appropriate application or custom collectors for device services. Remote Functions require a connected Child and can consume additional CPU and bandwidth when called.

Five- or ten-minute collection can suit status and capacity monitoring. It can miss short CPU spikes, brief temperature excursions and intermittent faults. For safety-critical control, keep the required sensing and decision loop on the device independently of this monitoring design.

For general deployment tooling and configuration distribution, see [Fleet Deployment and Configuration Management](../fleet-configuration-management.md). For Parent architecture and capacity, see [deployment with centralization points](../deployment-guides/deployment-with-centralization-points.md) and [Parent best practices](../observability-centralization-points/best-practices.md).
