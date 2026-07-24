<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/network-flows/sizing-capacity.md"
sidebar_label: "Sizing and Capacity Planning"
learn_status: "Published"
learn_rel_path: "Network Flows"
keywords: ['sizing', 'capacity', 'planning', 'storage', 'memory', 'scaling', 'distributed']
endmeta-->

<!-- markdownlint-disable-file -->

# Sizing and Capacity Planning

A practical guide to choosing the host, the storage, and the deployment shape for the netflow plugin. Read this before you decide where the plugin runs and how big it has to be.

## What this plugin is built for

The netflow plugin is designed to receive, decode, and store flow records (NetFlow / IPFIX / sFlow) from one network — typically the routers and switches at one site — directly on a Netdata Agent.

There is no unconditional “50k–100k flows/s” collector limit. The sustainable rate depends first on how many records each UDP packet carries, then on cardinality, enrichment, and storage contention. A sender that emits one record per packet exercises a much higher packet rate than a sender that fills normal near-MTU export packets.

## Measured full-pipeline capacity

The current release benchmark was run on 2026-07-24 with a release build on a `12th Gen Intel(R) Core(TM) i9-12900K`, 125.6 GiB RAM, and an ext4 filesystem on a Seagate FireCuda 530 NVMe. The collector used loopback UDP, a quiet-start target drive, and the host's `net.core.rmem_max = 7 MiB` cap. Every case ran for 30 seconds after a 4,096-record warmup, followed by a one-second post-send drain before shutdown.

The test starts an isolated collector and a separate sender, sends real privacy-safe UDP packets, shuts the collector down cleanly, and independently reads the final journals. A pass requires all of the following:

- the sender sustained at least 99% of the requested record rate;
- every UDP datagram reached the collector;
- the collector reported no decode or journal errors; and
- the final raw journal had the exact expected rows, bytes, packets, and ordinary-flow identity count.

The matrix covered NetFlow v5, NetFlow v9, IPFIX, sFlow, and Cisco NSEL; one-record UDP datagrams and near-MTU packed datagrams; repeating 256 identities, repeating 4,096 identities, and duration-bounded all-unique identities. “All unique” is a stress profile for the measured interval, not a claim that every deployment has permanently unique flows.

| Wire format | Packet layout | Profiles verified at 50k exporter records/s | Profiles verified at 100k exporter records/s |
|---|---|---|---|
| NetFlow v5 | one record per UDP packet | 256, 4,096 | none |
| NetFlow v5 | near-MTU packed | 256, 4,096, all unique | 256, 4,096 |
| NetFlow v9 | one record per UDP packet | 256 | none |
| NetFlow v9 | near-MTU packed | 256, 4,096, all unique | 256, 4,096, all unique |
| IPFIX | one record per UDP packet | 256 | none |
| IPFIX | near-MTU packed | 256, 4,096, all unique | 256, 4,096 |
| sFlow | one record per UDP packet | 256, 4,096 | 256 |
| sFlow | near-MTU packed | 256, 4,096, all unique | 256, 4,096 |
| Cisco NSEL v9 update events | one record per UDP packet | 256 | none |
| Cisco NSEL v9 update events | near-MTU packed | 256, 4,096, all unique | none |

For Cisco NSEL, the rate is firewall update events. Every accepted update in this benchmark emitted two directional traffic rows, so the verified packed 50k-event/s result is 100k journal rows/s. The test also verified zero create, deny, malformed, counterless, partial-counter, zero-responder, or unsupported-event outcomes for its synthetic update traffic.

Two bounded peak probes found a passing 100k ordinary-flow baseline and a first capacity failure at 125k records/s: one used sFlow with 256 repeating identities and one-record packets; the other used packed NetFlow v9 with all-unique identities. These are brackets for those selected cases, not a global 100k–125k ceiling.

### Planning from the measurement

- For ordinary flow exports that batch records into near-MTU UDP packets, **50k records/s is the verified baseline** on this host. Treat 100k records/s as a deployment-specific result to validate, not a default promise.
- For Cisco NSEL, **50k packed update events/s** is the verified baseline on this host. It writes two directional rows per accepted update. This benchmark did not verify 100k NSEL update events/s.
- Do not use one-record-per-datagram traffic as a generic 50k baseline. Its result varies materially by protocol and cardinality.
- GeoIP, classifiers, concurrent queries, periodic fsync, a slower disk, and a different UDP receive-buffer limit can all lower these results. Validate the exact exporter and configuration before sizing a production boundary.
- The collector requests a 64 MiB UDP receive buffer, but the operating system caps that request at `net.core.rmem_max`. Follow [Configuration](/docs/npm/network-flows/configuration.md#udp-listeners) before testing or operating at high packet rates.
- The ingest receive path is single-threaded. To exceed the verified envelope, distribute exporters across agents rather than assuming additional CPU cores raise the per-agent packet-rate ceiling.

## Distributed deployment is the scaling answer

Aggregation across many routers is rarely operationally meaningful for flow data — you almost always investigate one site, one router, or one interface at a time. So instead of pushing every flow to a single central collector, **deploy one Netdata Agent next to each router (or each site, or each branch office)**.

This pattern is how Netdata is built to scale: each agent owns its own flow journal, its own enrichment, and its own dashboard view; Netdata Cloud federates queries across them. The benefits compound:

- **Each agent's load is bounded by one router's flow rate**, not the whole network's. A 10 000 flow records/s router stays comfortably under the per-agent ceiling.
- **No single host becomes the bottleneck** for ingest, storage, or query latency.
- **Failure of one agent loses one router's history**, not the whole network's.
- **You don't pay the bandwidth cost** of moving every flow datagram to a central collector across WAN links.

For a multi-site / multi-data-centre / multi-branch deployment, this is the recommended shape: one Netdata Agent per router, federated through Netdata Cloud. Use the central collector pattern only if your sites are too small to host an agent each.

## Storage

Storage cost scales with received exporter records/events, cardinality, and the retention configured independently for each tier. The old single raw-tier estimate is not sufficient: it misses the physical allocation of the three rollup tiers and is wrong for traffic that cannot aggregate.

### Measured four-tier allocation

The storage benchmark modeled 50k and 100k input records/s in completed production-format archives for raw, 1-minute, 5-minute, and 1-hour tiers. It measured allocated disk space with `st_blocks * 512` for the archived `.journal` files and their journal-specific facet sidecars. It excluded active successor files and the shared `facet-state.bin`, because neither is a stable per-flow cost.

| Received source item | Repeating 256 identities | Repeating 4,096 identities | Duration-bounded all-unique stress |
|---|---:|---:|---:|
| Ordinary mixed NetFlow v5/v9, IPFIX, and sFlow record | 279 B | 294 B | 2.00 KiB |
| Cisco NSEL v9 update event (two directional flow rows) | 498 B | 527 B | 4.09 KiB |

These are **combined physical allocations for all four tiers per received source item**, not the apparent file length and not only the raw tier. The all-unique profile is intentionally an upper stress case: none of the tiers can combine its rows, so each tier retains one row per ordinary input or two rows per NSEL update.

The benchmark's scalable component model was checked against literal all-tier archives for each profile and traffic type. Combined allocation differed by at most 1.33%; the largest individual-tier difference was 3.88%, below the 5% acceptance limit.

For a daily write-allocation estimate, multiply the received event rate by 86,400 and by the applicable table value. At 50k received items/s, that is roughly:

- 1.20 TB/day for ordinary traffic with 256 repeating identities, or 8.84 TB/day for all-unique ordinary traffic;
- 2.15 TB/day for NSEL updates with 256 repeating identities, or 18.08 TB/day for all-unique NSEL updates.

Double those figures at 100k received items/s. They describe the archives generated from one day's input across all four tiers. **They are not the final retained-disk total** when tiers have different retention windows: calculate and provision each tier using its own retention period, then add the results.

### Size every tier from observed traffic

The raw tier always has a row for each decoded flow row, but it does not always dominate total disk use. Repeated traffic collapses strongly in the rollups; all-unique traffic does not. Use the table above as the starting range, measure the actual `raw`, `1m`, `5m`, and `1h` directories after a representative period, and then set each tier's size and duration limit.

Whichever limit is hit first rotates a journal. Size the per-tier file limit with a safety margin so the intended duration normally fires first and the size cap remains protection against bursts or a cardinality change.

### Use fast NVMe for the journal directory

The raw tier is queried directly for any IP-level investigation, full-text search, city / latitude / longitude maps, and anything that filters on a raw-only field (see [Field Reference](/docs/npm/network-flows/field-reference.md) for which fields survive into rollups). The collector also writes the raw stream continuously, so a busy or slow device directly reduces sustainable UDP intake.

Use fast NVMe for the journal directory when operating near the measured throughput envelope. A slower or contended device can make the collector lose UDP datagrams below the figures above. Put all four tiers on the same fast device unless you have measured a separate layout under representative write and query load.

## Memory

The journal backend uses **free system memory as page cache** — the bigger the database on disk, the more free RAM you want to keep available so the kernel can keep the working set hot.

Concrete guidance:

- For the agent process itself, expect **a few hundred MB to ~1 GB of RSS** at typical loads across the envelope. Enrichment, classifiers, accumulators, and routing tries add to the base process footprint. BMP / BioRIS full-table feeds can add a few hundred MB per peer, depending on table count and prefix mix.
- For the kernel page cache, aim to **leave at least the size of the recently-queried working set free** — practically, plan a few GB of free RAM on a busy agent so query I/O lands in cache instead of hitting NVMe each time.
- Watch default lightweight state-cardinality charts such as `netflow.open_tiers`, `netflow.facet_values`, and `netflow.tier_index_entries` for the agent's internal growth drivers. Enable `charts.memory_diagnostics` only when you need byte-level process attribution. Watch the system's overall free memory for the page-cache headroom.

The plugin does **not** preload the journal into RAM. Memory consumption is driven by active accumulators (during ingestion) and routing tries (when configured). Storage growth pressures memory only via the page cache, which the kernel manages.

## Querying — what's fast and what isn't

The journal is **fully indexed**: every field is indexed, exact-match selections (`SRC_AS_NAME = "AS15169 GOOGLE"`, `PROTOCOL = 6`) hit the index and return quickly regardless of how much data the tier holds.

The exception is the **full-text search box** in the dashboard. FTS runs as a regex against the raw journal payload bytes — it is a **full scan** of the matching tier. Any non-empty FTS query also forces the query to the raw tier (FTS is meaningless on aggregated rollup rows). That means:

- A full-text search over a wide raw-tier window can scan terabytes of indexed journal data. It runs but it is not fast.
- For fast queries, use **filters on indexed fields** (the filter ribbon, exact selections). Reserve full-text for the cases where you don't have an indexed handle.
- The 30-second hard query timeout is a real ceiling for FTS over wide windows. Narrow the time range, add an indexed filter, or switch to a rollup tier (which means dropping the FTS).

## Practical checklist before you deploy

1. **Capture the actual exporter packet shape and record rate.** Treat 50k packed ordinary records/s as the measured starting point, not a generic promise for one-record packets or NSEL.
2. **Pick retention independently for raw, 1m, 5m, and 1h.** The raw forensic window is often 24 hours, but it is not the only disk cost.
3. **Calculate storage per tier** from observed traffic and the measured four-tier range above; add a safety margin for bursts and changing cardinality.
4. **Use NVMe** for the journal directory. Slower or contended storage reduces UDP headroom.
5. **Leave RAM headroom** for the page cache and monitor the collector's state-cardinality charts.
6. **Tune kernel UDP buffers** for burst headroom (see [Troubleshooting](/docs/npm/network-flows/troubleshooting.md)).
7. **For multi-site deployments**, run one Netdata Agent per router or per site rather than central aggregation.

## What's next

- [Configuration](/docs/npm/network-flows/configuration.md#per-tier-retention) — Per-tier retention schema.
- [Retention and Querying](/docs/npm/network-flows/retention-querying.md) — How tiers map to queries and the auto-tier-pick rules.
- [Field Reference](/docs/npm/network-flows/field-reference.md) — Which fields survive into rollups and which are raw-only.
- [Troubleshooting](/docs/npm/network-flows/troubleshooting.md) — UDP buffer tuning, query timeout, and disk write pressure.
