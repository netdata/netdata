<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/sizing-and-capacity.md"
sidebar_label: "Sizing and Capacity"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'sizing', 'capacity planning', 'storm control', 'udp buffer', 'rcvbuferrors']
endmeta-->

<!-- markdownlint-disable-file -->

# Sizing and Capacity Planning

A practical guide to where the trap receiver runs and how big it has to be. Read it before you point production devices at a listener.

The number that matters: **size for the storm, not the average.** A failure that generates traps — a flapping core link, a spanning-tree reconvergence, a power or environmental cascade — produces 10–100× your steady-state trap rate, and it does so at exactly the moment you need every trap. A receiver sized for a quiet Tuesday goes blind during the outage it exists to catch.

## What one listener is built for

The trap receiver is designed to receive, decode, enrich, and store traps from **one site** — the routers, switches, and devices that send to one Netdata Agent. That is the unit you size.

On a well-provisioned single Agent, the receiver sustains roughly **50,000 traps/s** to durable local storage. Treat that as a planning ceiling, not a guaranteed SLA — it moves with disk speed, packet size, SNMPv3 authentication, and host load.

Decode is not the bottleneck — the receiver decodes traps far faster than it persists them. The limit is the **durable write path**, which appends each trap to the journal and flushes to disk once per second. That has two consequences:

- **More CPU does not raise the ceiling.** The decode path has headroom to spare; the durable write path is the wall.
- **The way past the ceiling is more Agents, not a bigger box** — see [Distributed deployment](#distributed-deployment-is-the-scaling-answer).

If your worst-case sustained rate stays at or below ~50,000 traps/s, you have headroom on one Agent. If it does not, plan distributed before you plan bigger iron.

## When the receiver can't keep up, the kernel drops first — and Netdata sees it {#kernel-udp-buffer-drops}

Before a trap reaches the collector, it sits in the **kernel UDP receive buffer**. If that buffer fills faster than the receiver drains it, the kernel drops the datagram. The trap collector's `received` counter counts packets *after* the kernel buffer, so these kernel drops never appear in the receiver's own pipeline metrics — **but Netdata catches them at the system level.** The `ipv4.udperrors` chart records exactly these drops on its `RcvbufErrors` dimension, and Netdata ships the **`1m_ipv4_udp_receive_buffer_errors`** alert on it. (That alert is routed `to: silent` by default — it raises in the dashboard but sends no notification until you route it to a recipient.)

So during a storm, watch `ipv4.udperrors` alongside the trap pipeline: if `RcvbufErrors` climbs while traps are flowing, the kernel buffer — not the collector — is your bottleneck.

Two buffers are in play:

- **Netdata's request:** `listen.receive_buffer` defaults to **4 MiB** per bound endpoint (max 256 MiB) — what the listener *asks* the kernel for.
- **The kernel's ceiling:** the OS grants no more than `net.core.rmem_max`, whose Linux default is ~208 KiB. The kernel silently caps the request at this value, so on an untuned host the 4 MiB request becomes ~208 KiB.

Raise the kernel ceiling so the request can be honored, then size `listen.receive_buffer` for your burst:

```bash
# Allow larger UDP receive buffers; persist in /etc/sysctl.d/.
sudo sysctl -w net.core.rmem_max=33554432      # 32 MiB
sudo sysctl -w net.core.netdev_max_backlog=5000
```

A bigger buffer absorbs bursts but cannot fix sustained overload — for that, shed load with [storm controls](#storm-controls) or add Agents.

## Distributed deployment is the scaling answer

Aggregating every device's traps into one central receiver is rarely the right shape: you almost always investigate one site, one device, or one interface at a time, and a single receiver is both a bottleneck and a single point of failure. Netdata scales the other way — **one Agent per site (or per data center, or per branch), each its own SNMP hub**, federated by Netdata Cloud:

- Each Agent's load is bounded by **one site's** trap rate, not the whole estate's.
- No single host is the bottleneck for receive, decode, storage, or query.
- Losing one Agent loses one site's local history, not everything.
- You don't pay the WAN cost of shipping every trap datagram to a central collector.

Use a central relay only for sites too small to host their own Agent.

## Storage and retention

Local trap history is bounded by `retention.max_size` (default **10 GB per job**); when the journal reaches it, the oldest rows are evicted. Size that cap to your incident-review window, not to device count — a chatty 50-device site can outproduce a quiet 500-device one.

Per-trap journal cost depends on varbind count and enrichment, so the reliable way to size disk is to **measure your own rate**: run representative traffic, then watch the on-disk growth of `/var/log/netdata/traps/<job>/` over a known trap count and extrapolate to your retention window. Put the journal directory on **NVMe** — the same disk that bounds throughput also serves your queries.

A few fixed internal behaviors bound the write path; you cannot tune them and they need no action beyond watching `journal_write_failed`:

- Under sustained overload, when the write path cannot keep up, traps are rejected and counted as `write_failed`.
- Traps are flushed to disk once per second. An abrupt power loss or OS crash can therefore lose up to the last second of traps; a clean `netdata` restart loses nothing. (Forwarded OTLP records have the same one-second-window caveat — keep that in mind when the journal is your only local copy.)
- Job creation fails if direct-journal storage is enabled and the Netdata log directory is missing or not writable.

For the byte-unit details of `max_size` and rotation, see [Configuration](/docs/snmp-traps/configuration.md#direct-journal-retention).

## Shipped limits and defaults

Useful starting points and the fixed caps that bound capacity — not capacity promises. For the full configurable option list, see [Configuration](/docs/snmp-traps/configuration.md#option-map).

| Limit / default | Value | What it bounds |
|---|---|---|
| `listen.receive_buffer` | 4 MiB / endpoint (max 256 MiB) | Requested UDP buffer; capped by `net.core.rmem_max` |
| Oversized packets | 8 KiB datagram, 256 varbinds | Larger PDUs become decode-error rows, not trap rows |
| `rate_limit.per_source_pps` | 1000 (when enabled) | Per-source trap rate before drop/sample |
| Rate-limiter source tracking | 10,000 sources / job (fixed) | Distinct source IPs tracked when rate limiting |
| `dedup.window_sec` / `cache_max_entries` | 5 s / 100,000 (when enabled) | Dedup window and distinct fingerprints / job |
| `dynamic_engine_id_max_pairs` | 4096 / job | SNMPv3 `(engineID, username)` pairs under dynamic discovery |
| `retention.max_size` | 10 GB / job | Local journal disk before oldest rows are evicted |
| Receiver per-source charts | 2000 sources / job (fixed) | Per-source visibility; excess counted as `overflow_dropped` |
| `profile_metrics.limits.max_instances_per_job` | 50,000 | Profile-derived metric series / job |

Profile-defined metrics stay **disabled by default** and are the main cardinality risk — a rule with an unbounded label can create uncontrolled series. Enable selected rules only, and keep the caps as guardrails; for the cardinality model and limits see [Configuration](/docs/snmp-traps/configuration.md#profile-metrics).

## Storm controls

Storm controls trade completeness for survival, per source:

- **Rate limiting** (`rate_limit`, off by default) drops or samples a source above `per_source_pps`. Rate-limit the **storming source only** — never globally disable `linkUp`/`linkDown`, because a flap is a leading indicator of failing hardware.
- **Deduplication** (`dedup`, off by default) summarizes repeated matching traps inside a window. Add `dedup.key_varbinds` (for example `ifIndex`) when one trap OID covers distinct resources, so a line-card failure on 48 ports is not collapsed into one suppressed row.

For how each control reads out in the receiver counters, see [Metrics](/docs/snmp-traps/metrics.md#receiver-pipeline).

## Validate before you trust it

A receiver that has never been load-tested will fail during the next major outage. Before pointing production devices at it:

1. Build a lab job that matches the production config.
2. Replay representative traffic **at 10× your steady-state rate** for ~30 minutes — normal flow, maintenance bursts, relay behavior, SNMPv3 if used, unknown OIDs, and repeated traps.
3. Require: **no `dropped`/`write_failed` growth, no climbing `RcvbufErrors`, and host CPU/disk below saturation.** If any fails, tune buffers, shed load, or move to distributed before go-live.

For the full quality checklist, see [Validation and Data Quality](/docs/snmp-traps/validation-and-data-quality.md).

## What's next

- [Configuration](/docs/snmp-traps/configuration.md) — buffer, retention, storm-control, and OTLP options.
- [Metrics](/docs/snmp-traps/metrics.md) — the receiver counters to watch during a storm.
- [Alerts](/docs/snmp-traps/alerts.md) — the default storm and dedup alerts, plus the kernel UDP buffer-drop alert.
- [Troubleshooting](/docs/snmp-traps/troubleshooting.md) — missing traps, silent loss, and backend failures.
