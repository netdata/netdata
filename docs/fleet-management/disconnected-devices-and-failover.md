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

On devices with 4 KiB memory pages, `retention = 120` rounds up to 1,024 samples per dimension. At five-minute collection, this covers about 3.6 days; at ten-minute collection, about 7.1 days. Size this window for the outages your devices encounter; each chart's own interval determines its coverage, and larger memory pages retain more samples.

Enable replication in the Parent's API-key section of `stream.conf`:

```ini
[YOUR_STREAMING_API_KEY]
    enabled = yes
    enable replication = yes
    replication period = 4d
    replication step = 10m
```

Add these settings to your existing receiver configuration. After reconnection, the Parent requests available history within the replication period and fills the gap in steps. Four days covers the five-minute example above on 4 KiB-page devices; use `replication period = 8d` for ten-minute collection. Match the replication period to the history you want to recover, including any additional retention on devices with larger memory pages.

Choose the storage mode that matches the device's recovery needs:

| Child storage | Recovery capability |
|:--|:--|
| `ram` | Backfills network outages while the Agent stays running, with metric history kept off the SD card |
| `alloc` | Backfills from memory with a smaller allocation and shorter recovery window at low retention settings |
| `dbengine` | Backfills from persistent history that survives Agent restarts and device power loss |
| `none` | Resumes current monitoring on reconnection, with the smallest local history footprint |

Use local retention to cover outages. The streaming send buffer handles short sending delays. See [CPU and memory settings](./minimize-cpu-and-memory.md#choose-local-history) for retention choices and the [streaming reference](../../src/streaming/README.md) for replication settings.

## Connect automatically to another Parent

Give the Child multiple destinations in `stream.conf`:

```ini
[stream]
    destination = parent1.example:19999:SSL parent2.example:19999:SSL
    ssl skip certificate verification = no
```

The Child connects to an available Parent and tries alternative destinations when it loses the connection. Provision the streaming key, permitted source addresses and TLS certificates on each Parent. Configure the Child to trust both Parents using the [TLS setup guide](./deployment-and-identity.md#secure-the-connection-and-operator-access).

For shared historical coverage across Parents, configure their storage and replication topology using the [centralization architecture guide](../deployment-guides/deployment-with-centralization-points.md). Connection failover and Parent history work together to keep your monitoring service available.

## Distinguish sleeping devices from failures

Cloud [node states](../netdata-cloud/node-states-and-transitions.md#states-on-netdata-cloud) show when a device is **Live** or **Stale** with history still available through its Parent. Use device labels to group always-on devices and those that sleep or operate intermittently in your fleet views.

For devices expected to disconnect regularly, set `[global] is ephemeral node = yes` in their `netdata.conf`. This excludes planned disconnections from the Parent's built-in disconnection alerts, which track permanent devices in aggregate. Ephemeral nodes also follow their own cleanup policy; see [node lifecycle and notification settings](../nodes-ephemerality.md) to choose the policy for each device class. Use the Nodes Map and node states to identify individual devices that need attention.

When a Child reconnects, current monitoring resumes and live troubleshooting Functions become available again. Combine the recovery policy with [cellular traffic settings](./minimize-cellular-traffic.md) to budget for the history sent after an outage.
