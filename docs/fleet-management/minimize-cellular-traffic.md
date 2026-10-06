# Minimize Cellular Traffic from Fleet Devices

Keep devices connected to their Parent and available for live troubleshooting with a small cellular data budget. Netdata combines sparse metric collection, compressed streaming and collection-aware TCP keepalives to reduce routine traffic between updates. The Parent adjusts keepalives automatically by default.

With five- or ten-minute collection, Netdata streams a compact metric workload for **about 4–7 MiB per device per month**, including the default automatic keepalives. Your team retains the connection for device status and on-demand troubleshooting, while routine telemetry follows the interval you choose.

## Keep the connection, reduce the background traffic

A Child keeps its TCP connection to the Parent open between collections. Operators can request live Functions through that connection to investigate a device whenever they need to.

Three settings determine routine traffic:

- **Metric selection:** collect and stream the information your team uses.
- **Collection interval:** send updates as often as the workload needs them.
- **TCP keepalives:** the Parent automatically matches their cadence to those updates; override it when your fleet needs a fixed policy.

## Set a monthly streaming budget

This example streams 46 metric dimensions over compressed TLS. It includes traffic in both directions and shows the monthly traffic for each keepalive setting:

| Collection interval | Parent keepalive idle | Monthly streaming budget |
|:--|:--|--:|
| Ten minutes | 30 seconds | 11.56 MiB |
| Ten minutes | 60 seconds | 6.87 MiB |
| Ten minutes | 120 seconds | 4.78 MiB |
| Ten minutes | 300 seconds | **4.27 MiB** |
| Five minutes | 150 seconds | **6.72 MiB** |

At ten-minute collection, extending keepalive idle from 30 to 300 seconds reduced total traffic by **63% while keeping the connection open**.

These are 31-day projections for steady connected streaming with this compact workload. Budget additional traffic for reconnects, history recovery and live troubleshooting; your metric set and cellular network determine actual usage.

## Choose the collection interval

Set the Child's default in `netdata.conf`:

```ini
[db]
    update every = 600
```

Use `300` for five-minute collection. These intervals suit slowly changing status and capacity metrics. Keep faster collection for rapid workload changes or failures that need quick detection.

Set collector-specific intervals to match your policy. For the **v2.12.0-2-nightly** package used in the preparation example, use the [one-minute apps and debugfs overrides](./minimize-cpu-and-memory.md#collect-every-five-or-ten-minutes) to retain those plugins. An Agent with cadence-aware plugin read timeouts supports longer intervals for them.

Remove unused collector jobs to reduce device work and traffic together. Streaming chart filters let you keep a metric locally while excluding it from the Parent stream; see the [streaming reference](../../src/streaming/README.md).

## Match keepalives to collection

The Parent automatically adjusts TCP keepalives to collection cadence. No additional configuration is required. Leaving `tcp keepalive idle` unset is equivalent to this explicit default in the receiver's API-key section of `stream.conf`:

```ini
[YOUR_STREAMING_API_KEY]
    tcp keepalive idle = auto
```

Automatic keepalives use half the fastest streamed chart interval, bounded between 30 seconds and one hour:

| Fastest streamed interval | Automatic keepalive idle |
|:--|:--|
| One minute | 30 seconds |
| Five minutes | 150 seconds |
| Ten minutes | 300 seconds |

These cadence-matched values apply to Linux Parents. Parents on platforms without per-socket keepalive tuning use their operating system's default keepalive timing.

For the five- and ten-minute savings above, apply that interval to all streamed charts, including collector overrides and Agent monitoring charts. A one-minute chart keeps automatic keepalive idle at 30 seconds. Reconnect the Child after changing the interval policy so the Parent starts with the new cadence.

The default automatic keepalives maintain idle connections through gateways while avoiding unnecessary packets. To override the default, set `tcp keepalive idle` to a fixed duration. Positive values below 30 seconds become 30 seconds; values above one hour become one hour. To opt out, set it to `off`, `0` or `never`, which disables TCP keepalives on the Parent's receiving socket. A per-device `[MACHINE_GUID]` setting takes precedence over the API-key setting.

The Parent also adjusts its application-idle timeout for sparse updates: it waits at least ten minutes, or twice the fastest chart interval when that is longer. Together, these settings support sparse telemetry while detecting lost connections.

## Keep compression enabled

Use `enable compression = yes` on both sides of the stream. Netdata negotiates a supported compression algorithm. The Child's `iot` profile selects lower compression effort for constrained hardware, helping keep both cellular traffic and CPU use small.

## Budget for recovery and maintenance

Include history sent after coverage gaps, live Functions and log inspection, and Agent or image downloads in the device's cellular allowance. Use [local retention and replication](./disconnected-devices-and-failover.md) to recover the gaps that matter, and schedule large image updates on the fleet's preferred network.

Follow per-Child streaming statistics on the Parent to see transferred bytes, reconnects and replication. Repeated reconnects or an unexpectedly fast chart can explain rising traffic. Use interface counters and carrier reports to follow the SIM's total usage, including your applications.

The [fleet monitoring guide](./monitor-the-fleet.md) brings connectivity, traffic and device health into the central operating view.
