# Minimize Cellular Traffic from Fleet Devices

Keep centralized device monitoring affordable on cellular links by sending only the metrics you need, at a suitable interval. Netdata supports five- and ten-minute collection, compressed streaming and cadence-aware connection keepalives, so you can reduce background traffic while keeping your fleet visible on Parents.

## Stay connected with a small monthly data budget

Your Child keeps its TCP connection to the Parent open between metric updates. Your team can continue to see the device's connection status and request live troubleshooting Functions through that connection. Five- or ten-minute collection reduces routine telemetry; it does not require disconnecting the device between updates.

Three controls work together:

- **Select the metrics** your team needs on the cellular link.
- **Set their collection intervals** to match how quickly you need new values.
- **Reduce unnecessary TCP keepalives** while preserving the connection on your modem and carrier network.

For a small operational metric set, connected streaming can use only a few MiB per month. The following example gives you a starting budget for a device class.

### Monthly traffic example

A packet-capture test streamed **46 metric dimensions** over TLS with ZSTD level 1. The Child used Netdata **v2.10.4**; the test Parent included the cadence-aware receiver timeout and keepalive changes. The measurements counted IPv4 bytes in **both directions**, including IP/TCP headers, TLS, acknowledgements and Netdata control traffic.

| Collection interval | Parent TCP keepalive idle | Projected streaming traffic per 31-day month |
|:--|:--|--:|
| Ten minutes | 30 seconds | 11.56 MiB |
| Ten minutes | 60 seconds | 6.87 MiB |
| Ten minutes | 120 seconds | 4.78 MiB |
| Ten minutes | 300 seconds | **4.27 MiB** |
| Five minutes | 150 seconds | **6.72 MiB** |

At ten-minute collection, changing only the Parent keepalive idle from 30 to 300 seconds reduced total measured traffic by **63%**. The TCP connections stayed open throughout the measured steady windows.

These are monthly projections from packet captures covering two complete collection intervals after warmup, rather than month-long carrier measurements. The tests had no reconnects or retransmissions, excluded connection setup and had historical replication disabled. Use them to size a starting configuration, then measure your device class on its actual cellular network. The table is a selected operational workload, not the bandwidth of every built-in metric or of the reduced packages' default configuration.

Metric count, metric changes and compression affect the result. Live Functions, log inspection, reconnects, retransmissions and historical replication add traffic. Other applications on the device also consume the SIM's allowance. Compare measured interface traffic with carrier usage when commissioning the fleet.

A connected socket provides a path for live requests between collections. Detection of a broken link still depends on TCP keepalive and application-idle timeouts; five-minute data collection does not imply immediate detection of every failure.

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

The Parent also accommodates sparse updates in its application-level liveness handling: its application-idle timeout is the greater of ten minutes and twice the fastest observed chart interval. Configure these settings on the Parent that receives the stream. See the [streaming configuration reference](../../src/streaming/README.md) for the full connection settings.

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
