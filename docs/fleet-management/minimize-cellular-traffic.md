# Minimize Cellular Traffic from Fleet Devices

Budget the traffic over the entire device lifecycle. Steady metric streaming is only one part: TLS setup, reconnects, metadata, historical replication, remote Functions, downloads and other device applications also use the link. Carrier billing can include overhead that interface counters do not capture.

## Choose cadence and telemetry together

Start by choosing the required telemetry, then its fastest useful cadence. Set the Child's default collection interval and any collector-specific intervals accordingly. For slow status monitoring, test five-minute or ten-minute collection. Keep a faster interval wherever short faults matter.

A streaming chart filter can omit unwanted charts from the network, but does not stop their collection on the Child. To save both CPU and bandwidth, reduce unnecessary collection jobs as well. Keep enough operational telemetry to detect an unhealthy Agent or broken Parent connection.

## Use cadence-aware Parent liveness

The [sparse-streaming change](https://github.com/netdata/netdata/pull/23615), merged August 25, 2026, added cadence-aware liveness and TCP keepalive policy. Use a release containing this change on the Parent; a Child-only upgrade does not configure receiver sockets.

In the Parent's API-key receiver section, or its per-machine override section in `stream.conf`, `tcp keepalive idle = auto` derives idle time from half the fastest observed chart interval, bounded between 30 seconds and one hour. Five-minute charts therefore imply about 150 seconds; ten-minute charts imply about 300 seconds, if no faster chart has been observed on that connection. The fastest observed interval can lower the value until reconnection. Parent application idle limits also accommodate sparse collection.

An explicit idle interval overrides automatic selection. `tcp keepalive idle = off` disables TCP keepalives on that Parent receiver socket. This does not disable application liveness or every other packet on the connection, and it does not control keepalive policy on unrelated connections. See the [streaming configuration reference](../../src/streaming/README.md).

Automatic policy is a starting point. Cellular NATs and gateways may expire idle flows; fewer probes can save traffic but provoke expensive reconnects on some networks. Test each carrier and connectivity design before choosing an explicit interval or disabling probes.

## Balance compression against CPU

Leave stream compression enabled unless measurements show a better policy. The IoT profile uses lower default compression effort for the relevant algorithms than the ordinary Child profile. Explicit compression settings override profile defaults. The negotiated algorithm depends on both sides.

Measure compressed bytes and CPU together. Small sparse messages may have different trade-offs from large continuous workloads; compression cannot remove protocol overhead or replace a sensible collection policy.

## Interpret the published experiment correctly

The merged change includes a controlled experiment with 46 dimensions across 17 charts. It used IPv4, no TLS, replication disabled and no reconnects or retransmissions during the steady-state sample. Projected 31-day traffic was:

| Collection interval / receiver keepalive idle | Projected traffic |
|:--|--:|
| 600 seconds / 300 seconds | 5.224 MiB |
| 300 seconds / 150 seconds | 6.220 MiB |
| 120 seconds / 60 seconds | 15.419 MiB |

These are workload-specific projections from that experiment, not a per-device quota or encrypted-production estimate. Controlled Parent-restart reconnects cost approximately 7.0–7.5 kB each in the same plaintext setup. Metadata size, TLS, fleet topology and history can change those costs substantially.

## Verify cellular cost in production

Compare streaming counters at the Parent, device interface counters and carrier usage over the same period. Include a planned disconnection and reconnection, an on-demand Function call, first provisioning and an image update. Account separately for non-Netdata traffic on the cellular interface.

Set alerts for excessive traffic, frequent reconnects and unexpected faster collection. A single retained fast chart can defeat a sparse-connection assumption. Use the [fleet monitoring guidance](./monitor-the-fleet.md) to monitor both observability coverage and its cost.
