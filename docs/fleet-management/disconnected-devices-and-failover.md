# Handle Disconnected Devices and Parent Failover

Keep your fleet's monitoring history through cellular coverage gaps, tunnels and network changes. A Netdata Child continues collecting while disconnected, then sends its retained history to the Parent when the connection returns. Operators can still explore the history already on the Parent while a device is offline.

## Recover history after a coverage gap

For lightweight devices, keep a recovery buffer in RAM. Set these values in the Child's `netdata.conf`:

```ini
[db]
    db = ram
    update every = 300
    retention = 120
```

At five-minute collection, 120 samples cover roughly ten hours per dimension. At ten-minute collection, the same entry count covers roughly twenty hours. Size this window for the outages your devices encounter; each chart's own interval determines its coverage.

Enable replication in the Parent's API-key section of `stream.conf`:

```ini
[YOUR_STREAMING_API_KEY]
    enabled = yes
    enable replication = yes
    replication period = 1d
    replication step = 10m
```

Add these settings to your existing receiver configuration. After reconnection, the Parent requests available history within the replication period and fills the gap in steps. A one-day replication period accommodates the example RAM history above.

Choose the storage mode that matches the device's recovery needs:

| Child storage | Recovery capability |
|:--|:--|
| `ram` | Backfills network outages while the Agent stays running, with metric history kept off the SD card |
| `dbengine` | Backfills from persistent history that survives Agent restarts and device power loss |
| `none` | Resumes current monitoring on reconnection, with the smallest local history footprint |

Use local retention to cover outages. The streaming send buffer handles short sending delays. See [CPU and memory settings](./minimize-cpu-and-memory.md#choose-local-history) for retention choices and the [streaming reference](../../src/streaming/README.md) for replication settings.

## Connect automatically to another Parent

Give the Child multiple destinations in `stream.conf`:

```ini
[stream]
    destination = parent1.example:19999:SSL parent2.example:19999:SSL
```

The Child connects to an available Parent and tries alternative destinations when it loses the connection. Provision the streaming key, permitted source addresses and TLS trust on each Parent.

For shared historical coverage across Parents, configure their storage and replication topology using the [centralization architecture guide](../deployment-guides/deployment-with-centralization-points.md). Connection failover and Parent history work together to keep your monitoring service available.

## Distinguish sleeping devices from failures

Cloud [node states](../netdata-cloud/node-states-and-transitions.md#states-on-netdata-cloud) show when a device is **Live** or **Stale** with history still available through its Parent. Use device labels to separate always-on devices from those that sleep or operate intermittently. Apply connectivity alerts and routing to those groups so your team responds to unexpected outages while allowing planned offline periods.

When a Child reconnects, current monitoring resumes and live troubleshooting Functions become available again. Combine the recovery policy with [cellular traffic settings](./minimize-cellular-traffic.md) to budget for the history sent after an outage.
