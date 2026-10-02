# Edge Device Monitoring

Monitor robots, digital signage, gateways and autonomous vehicles from one place, with a small Netdata Agent on each device. Stream their metrics to Netdata Parents to bring fleet-wide dashboards, historical data, alerts, anomaly detection and live troubleshooting together while reducing work on the devices.

Netdata separates collection from analysis. The Agent on an edge device is a **Child**: it collects system and application metrics and sends them to a **Parent**. The Parent stores history and provides the monitoring experience for your fleet. You can also run live troubleshooting Functions through the Parent to inspect a connected device's processes, network connections and logs.

## Fit monitoring to your devices

Use the same architecture across a fleet while adapting each device's Agent to its hardware and workload. For constrained Linux systems, including 32-bit ARM devices, you can:

- **Reduce installed size** by removing debugging symbols and packaging only the plugins your devices use.
- **Reduce CPU and memory use** by moving storage, alerting and anomaly detection to Parents and collecting at an appropriate interval.
- **Reduce cellular traffic** with less frequent metric updates, compression and keepalive settings suited to intermittent connections.
- **Recover data after an outage** by keeping local history and replicating it when the device reconnects.
- **Troubleshoot centrally** with dashboards and live Functions, without installing a dashboard on every device.

Keep the collectors that supply your operational metrics and live Functions. For example, retain the apps plugin for process monitoring, the network viewer for network inspection, and the journal plugin for journal access. Your Parent brings these capabilities into one interface.

## Set up edge monitoring

Follow these guides to prepare and operate your fleet:

1. [Deploy and identify devices](./deployment-and-identity.md): provision unique identities, labels and secure Parent connections.
2. [Minimize disk footprint](./minimize-disk-footprint.md): prepare a smaller static package for your device image.
3. [Minimize CPU and memory](./minimize-cpu-and-memory.md): choose the Child's collection interval and local storage.
4. [Minimize cellular traffic](./minimize-cellular-traffic.md): configure sparse updates, compression and connection keepalives.
5. [Handle disconnections and failover](./disconnected-devices-and-failover.md): retain data through coverage gaps and provide alternative Parents.
6. [Monitor fleet health](./monitor-the-fleet.md): track device health, connectivity and monitoring resource use.
7. [Update and troubleshoot devices](./updates-and-troubleshooting.md): deliver reduced packages through your image pipeline and recover quickly.

For deployment automation, see [Fleet Deployment and Configuration Management](../fleet-configuration-management.md). For the central monitoring layer, see [deployment with centralization points](../deployment-guides/deployment-with-centralization-points.md) and [Parent best practices](../observability-centralization-points/best-practices.md).
