# Minimize CPU and Memory on Fleet Devices

Move long-term storage, alert evaluation and anomaly detection to Parents, then measure what remains on the Child. The package preparation script saves installed bytes; collector activity, metric cardinality and database configuration determine most runtime costs.

## Choose the Child's responsibilities

Use the [lightweight Child configuration guidance](../deployment-guides/deployment-strategies.md) as the baseline:

- Disable ML on the Child when the Parent supplies anomaly detection.
- Disable health evaluation on the Child when the Parent supplies alerts. Decide separately whether any local alerts are needed during disconnection.
- Keep only required collectors and integrations running. Built-in collectors remain compiled in after package pruning.
- Restrict the local web listener according to your access policy. If the dashboard files are omitted, use the Parent dashboard.
- Keep the information required to diagnose the device, including process or hardware collectors where those are operational requirements.

Apply these settings in the Child's `netdata.conf` when the corresponding Parent responsibility is confirmed:

| Section and setting | Effect |
|:--|:--|
| `[ml] enabled = no` | Stops local anomaly-detection work |
| `[health] enabled = no` | Stops local alert evaluation |
| `[web] bind to = localhost` | Restricts the local listener to loopback |
| `[db] db = ram` | Retains metric samples in RAM; requires a retention budget |
| `[db] db = none` | Removes local metric history and its backfill capability |

Use either `ram` or `none`, according to the outage contract. These are Child settings; do not copy disabled ML and health settings onto a Parent expected to provide those capabilities.

The automatic IoT profile is selected for constrained hardware, including one-CPU or low-memory systems. It is a default profile, not a complete fleet policy. Inspect the effective configuration rather than assuming it disables all storage or changes collection to five minutes. You can explicitly select `[global] profile = iot`; still configure collection, storage and health according to the fleet requirements. See [Agent configuration](../netdata-agent/configuration/README.md).

## Select local history deliberately

`ram` database mode retains recent metric samples in memory instead of DBengine metric files. `none` provides no local metric history. Neither setting means that Netdata performs no disk writes: metadata databases, logs and other state may still be persisted.

For non-DBengine memory-backed history, the configured `retention` value is used as a sample-entry count, despite being parsed with duration syntax. The retained time is approximately the entry count multiplied by that chart's collection interval, with allocation rounding. Therefore a value of 120 does not mean two minutes of history for charts collected every 300 seconds. For example, on a system with 4 KiB pages, RAM sample arrays round to multiples of 1,024 four-byte entries per dimension. A requested 120 entries can therefore allocate 1,024, representing roughly 85 hours at a 300-second cadence. Size the buffer against actual chart intervals and verify the resulting oldest timestamps and memory use on the device.

DBengine provides persistent metric history, with separately configured disk limits and storage tiers. Use it when outage recovery requires persistence through restarts, and budget for database space, I/O and memory. Consult [database configuration](../netdata-agent/configuration/README.md) and [Agent sizing](../netdata-agent/sizing-netdata-agents/README.md); package size is separate from database capacity.

Choose the maximum outage you intend to backfill. RAM history survives a network outage while the Agent remains running; it does not survive an Agent restart or device power loss. With `none`, there is no history to replicate after reconnection.

## Reduce collection work

Increase `[db] update every` only as far as the monitored behavior allows. A cadence of 300 or 600 seconds can reduce work and steady streaming traffic for slow status metrics. Set `[db] update every = 300` for five minutes or `600` for ten minutes only where that loss of resolution is acceptable. Collector-specific configuration can override the default; inspect actual chart cadence at the Parent, including Netdata's own monitoring charts.

Counter rates and gauge samples have different meanings at coarse cadence. A gauge can miss an entire event between samples. A long counter interval can average a burst into a low rate. Collectors may have independent event processing or maintenance work that continues between published samples; CPU reduction is not necessarily proportional to the interval.

Control metric cardinality before cutting intervals indiscriminately: unnecessary per-process groups, interfaces, container dimensions and application jobs can cost both RAM and CPU. Remove the unused collection job or configure its supported filters; streaming filters alone do not stop local collection.

## Measure a realistic budget

For each device class, record Agent and plugin RSS, CPU time, metric count, writes and startup time. Include first boot, steady operation, Parent disconnection, backfill, interactive Functions and update commissioning. Track plugin processes as well as the daemon. Verify Parent ML training and eligibility at the chosen cadence; moving ML does not make sparse samples equivalent to one-second data.

For logs, configure supported destinations, severity and rotation in the image. Confirm metadata and log writes on the filesystem; do not assume `ram` makes the device read-only. See the [logging reference](../../src/libnetdata/log/README.md).

Use an otherwise identical image to compare policies, with the same workload, kernel and observation period. On 32-bit systems, include address-space limits in your assessment; a package that fits on disk can still fail a runtime memory budget.

The general performance and sizing documents describe typical workloads, not a promised ARM footprint. No runtime CPU or memory benchmark accompanies the offline package measurements in this guide. Use [performance optimization](../netdata-agent/configuration/optimize-the-netdata-agents-performance.md) as a reference, and set acceptance thresholds from your measured hardware.
