# Handle Disconnected Devices and Parent Failover

Devices may enter tunnels, lose cellular coverage, sleep or power down. Decide which gaps are acceptable and which must be recovered before selecting a local storage mode or reducing liveness traffic.

## Understand three different buffers

| Mechanism | What it provides | Limitation |
|:--|:--|:--|
| Streaming send buffer | Absorbs transient send delay | Not a durable outage queue; reconnects flush it |
| Local metric history | Samples that can be replicated after reconnection | Limited by retention; RAM history is lost on restart |
| Parent history | Central history already received | Cannot reconstruct uncollected or expired Child samples |

Increasing a send buffer consumes RAM and does not replace historical replication. Use local retention and enabled replication when backfill is required. See the [Parent-Child reference](../../src/streaming/README.md) for replication periods and steps.

## Set an outage contract

For each device class, specify maximum recoverable outage, power-loss behavior, expected reconnect traffic and how missing telemetry appears to operators. A Child with no local history can resume live streaming, but it cannot fill the disconnected period. A coarse collection policy still has coarse data after backfill.

Test outages shorter and longer than retention, and test a device reboot while disconnected. Inspect actual oldest and newest timestamps at the Parent after reconnection. Size local disk or RAM from that evidence rather than the streaming buffer setting.

## Provide Parent alternatives

The Child destination supports a list of Parents and connects to the first available destination. Provision matching authorization and TLS trust on each intended Parent. Verify failover with the actual network routes and firewall policy.

Multiple destinations do not by themselves create identical historical databases on every Parent. Design Parent replication and routing according to [centralization architecture](../deployment-guides/deployment-with-centralization-points.md), and test what users see when a Parent becomes unavailable.

## Distinguish disconnected from unhealthy

An absent Child cannot execute forwarded Functions. Previously retained history remains queryable on the Parent, subject to retention. Plan local diagnostics for field service and a recovery path for devices that cannot reconnect.

Define separate expectations for devices that intentionally sleep and devices expected to remain online. Tune liveness with the [cellular guide](./minimize-cellular-traffic.md), but do not interpret a longer detection delay as improved reliability. Alert routing should reflect the device's expected operating schedule.
