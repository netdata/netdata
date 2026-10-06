# Edge Device Monitoring

Monitor thousands of robots, digital signage players, gateways and autonomous vehicles from one place. Netdata gives your team fleet-wide visibility, alerts, anomaly detection and live troubleshooting while keeping monitoring lightweight on each device—even on 32-bit ARM hardware. Choose collection intervals from one second for fast-changing workloads to ten minutes for devices with tight power or cellular budgets.

A small Agent on each device collects system and application metrics and streams them to a Netdata Parent. The Parent provides the storage and analysis capacity. Your operators get the monitoring experience they need, while the devices stay focused on their workloads.

## Full fleet visibility with a small device footprint

You can tailor Netdata to the hardware, connectivity and operational needs of each device class:

- **Fit your device image.** Keep the collectors you need and remove debug symbols and unused plugins. The ARMv7 example in this guide reduces the package payload from 676 MiB to **41 MiB**, retaining system metrics, process monitoring and hardware sensors.
- **Keep CPU and memory available for your application.** Move historical storage, alert evaluation and machine learning to Parents. Keep local health enabled when you want [device-side automation](./monitor-the-fleet.md#automate-actions-on-the-device) to trigger custom actions on alert transitions. A Raspberry Pi 3 B+ example with five-minute system collection, one-minute process and sensor collection, and StatsD disabled used **under 1% of a CPU core and about 30 MiB of memory**.
- **Run from RAM.** Keep metric history in memory and turn off local Agent logs or keep them in RAM. The Child collects and streams without writing metric history to the SD card; the Parent stores long-term history.
- **Stay connected on cellular.** Sparse collection, compression and the Parent's default automatic keepalives keep devices available for live troubleshooting between updates. A compact metric workload streams for **about 4–7 MiB per device per month** with five- or ten-minute collection, including keepalive traffic.
- **Recover through coverage gaps.** Buffer history locally and send it to the Parent when connectivity returns. Configure alternative Parents for automatic connection failover.

The [disk](./minimize-disk-footprint.md), [CPU and memory](./minimize-cpu-and-memory.md) and [cellular](./minimize-cellular-traffic.md) guides show the configurations and example results behind these footprints.

## See the fleet the way your business operates

Attach customer, location, building, room, model and software-version labels to each device. Use them to filter and group thousands of devices in the Nodes Map, compare deployment groups and investigate a customer's fleet.

Bring your application's operating values into the same view: whether a robot is working, a vehicle's speed, a drone's altitude or a motor's RPM. Applications can send these metrics to the local Agent through StatsD. Use labels and metrics to group devices, choose the information shown in their inspectors, and color hexagons by operating values and alert status.

Parent-side anomaly detection learns patterns in application metrics as well as system metrics, helping your team spot unusual operating behavior while keeping machine-learning work off the devices.

When a device needs attention, open its charts or run live troubleshooting Functions through the Parent. Inspect processes, network connections, services and logs using the collectors included in that device's package. Your team can investigate centrally without maintaining a dashboard on every device.

## Deploy and operate your fleet

1. [Deploy and identify devices](./deployment-and-identity.md): connect devices to Parents, preserve their identities and attach business labels.
2. [Minimize disk footprint](./minimize-disk-footprint.md): prepare and install a smaller static package for 32-bit ARM devices.
3. [Minimize CPU and memory](./minimize-cpu-and-memory.md): configure a lightweight Child and protect local storage.
4. [Minimize cellular traffic](./minimize-cellular-traffic.md): keep devices connected with a small monthly data budget.
5. [Handle disconnections and failover](./disconnected-devices-and-failover.md): recover history after coverage gaps and provide alternative Parents.
6. [Monitor fleet health](./monitor-the-fleet.md): organize the fleet, add application metrics and troubleshoot from one interface.
7. [Update and troubleshoot devices](./updates-and-troubleshooting.md): preserve your package policy through image updates.

For automated provisioning, see [Fleet Deployment and Configuration Management](../fleet-configuration-management.md). For the central monitoring layer, see [deployment with centralization points](../deployment-guides/deployment-with-centralization-points.md) and [Parent best practices](../observability-centralization-points/best-practices.md).
