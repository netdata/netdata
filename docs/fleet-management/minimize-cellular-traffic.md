# Minimize Cellular Traffic from Fleet Devices

Keep centralized device monitoring affordable on cellular links by sending only the metrics you need, at a suitable interval. Netdata supports five- and ten-minute collection, compressed streaming and cadence-aware connection keepalives, so you can reduce background traffic while keeping your fleet visible on Parents.

## Send fewer metric updates

Set the Child's `[db] update every` to `300` for five-minute collection or `600` for ten-minute collection. Use these intervals for slowly changing status and capacity metrics. Keep faster collection for conditions that require quick detection.

Apply the same policy to collector-specific intervals. The fastest chart on a connection influences keepalive behavior, so check actual chart intervals at the Parent, including the Agent's own monitoring charts.

Remove unused collection jobs to reduce both device work and traffic. Use streaming chart filters when you want to collect a metric locally without sending it to the Parent. See [CPU and memory settings](./minimize-cpu-and-memory.md) for the collection policy.

## Match keepalives to collection

Set `tcp keepalive idle = auto` in the Parent's API-key receiver section of `stream.conf`, or in the device's per-machine override. Netdata adjusts the idle time to half the fastest observed chart interval, between 30 seconds and one hour.

| Fastest chart interval | Automatic keepalive idle time |
|:--|:--|
| Five minutes | 150 seconds |
| Ten minutes | 300 seconds |

These values apply when **all streamed charts** use those intervals. In the pinned **v2.12.0-2-nightly** package, external plugins such as `apps` and `debugfs` need a shorter collection interval to avoid a two-minute plugin-read timeout. Follow the [per-plugin overrides](./minimize-cpu-and-memory.md#configure-a-lightweight-child); their one-minute charts keep automatic TCP keepalive idle at 30 seconds. Sparse built-in system collection remains available.

This reduces unnecessary TCP keepalive traffic on sparse connections. A faster chart reduces the automatic idle time for that connection; after changing the chart policy, reconnect the Child so the Parent starts with the new intervals.

For a fleet that needs a fixed policy, configure an explicit idle interval. `tcp keepalive idle = off` disables TCP keepalives on the Parent's receiving socket. Prefer automatic keepalives as a starting point: cellular gateways may expire idle connections, and frequent reconnects add traffic.

The Parent also accommodates sparse updates in its application-level liveness handling. Configure these settings on the Parent that receives the stream. See the [streaming configuration reference](../../src/streaming/README.md) for the full connection settings.

## Keep compression enabled

Stream compression reduces the bytes sent over the link. Use the IoT profile on constrained Children for lower compression effort, and retain streaming compression on both sides. Netdata negotiates a supported algorithm for the connection.

Review compression settings together with device CPU use when selecting the configuration for a device class.

## Plan for reconnects and updates

Include these activities in your cellular plan:

- Initial provisioning and Agent or image downloads.
- Reconnection and TLS setup.
- Historical data sent after a coverage gap.
- Live troubleshooting Functions and log access.

Set local retention to cover the outages you want to recover, using the [disconnection guide](./disconnected-devices-and-failover.md). Distribute large image updates through your fleet's preferred network or maintenance window.

## Track traffic centrally

Use per-Child streaming statistics on the Parent to track bytes transferred, connection changes and replication. Watch for unexpectedly frequent reconnects or faster chart intervals, and adjust the device class's settings accordingly.

Use device interface counters and the carrier's usage reports for the complete cellular bill, which includes other applications and network overhead. See [monitoring fleet health](./monitor-the-fleet.md) for the central operational view.
