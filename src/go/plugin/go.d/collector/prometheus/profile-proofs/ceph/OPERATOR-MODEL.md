<!-- markdownlint-disable MD013 MD043 MD060 -->

# Ceph Prometheus operator model

## Evidence boundary

- **Reef:** `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df` (18.2.8).
- **Squid:** `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a` (19.2.5).
- **Tentacle:** `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc` (20.2.2).
- **NVMe-oF gateway:** `ceph/ceph-nvmeof @ c79b6f44bd2288f7ec5c48e3cc47f6e566573d3f`.
- **Official surfaces:** cluster-wide MGR Prometheus, host-local ceph-exporter with priority-0 counters, and NVMe-oF.
- **Synthetic exposition:** the committed fixture is the structural union of mutually optional releases, daemon roles,
  modules, transports, and runtime configurations. It is intentionally not one realizable scrape.
- **Native collector boundary:** the Ceph Dashboard REST collector is a different source. Overlap with it does not remove any
  official exporter family from this profile.

## Entity and capability map

```text
Ceph cluster
  -> control plane: MON, MGR, cephadm, health
  -> storage topology: pools -> PGs -> OSDs -> BlueStore/BlueFS/devices
  -> client services:
       CephFS -> filesystem -> MDS -> clients/sessions/cache/journal/purge queue
       RBD -> pool/image -> image I/O -> persistent write log/object cache
       RBD Mirror -> peer/image replication
       RGW -> gateway -> user/bucket/topic -> multisite sync/cache/notifications
       SMB -> share metadata
       NVMe-oF -> gateway -> subsystem -> namespace/listener/host -> block device
  -> shared runtime: throttles, finishers, memory pools, messenger/RDMA/DPDK, process/exporter
```

## Operator owners and causal order

1. **Cluster health and capacity:** service impact, usable/raw capacity, OSD state, pools, PG state, recovery, and backfill.
2. **Control plane:** MON/MGR/cephadm state and work that can block convergence or orchestration.
3. **OSD client I/O:** client work, latency, queues, throttles, and failure/retry outcomes.
4. **OSD recovery and scrubbing:** repair/recovery work, reservations, outcomes, and release-specific scrub phases.
5. **Storage engine:** BlueStore, BlueFS, RocksDB, block devices, compaction, allocation, cache, I/O, and space pressure.
6. **CephFS/MDS:** requests, clients, sessions, metadata cache, journal, memory, and purge queue.
7. **RBD and RBD Mirror:** per-image I/O, persistent write log, object cache, and replication progress/outcomes.
8. **RGW:** operations, latency/bytes, cache, lifecycle, notification/topic queues, and multisite synchronization.
9. **NVMe-oF:** gateway/reactor state, host connectivity, subsystem inventory/limits, namespace/listener topology, and bdev I/O.
10. **Shared runtime:** mClock shards, messenger workers, RDMA/DPDK paths, finishers, throttles, memory pools, and exporters.

The dashboard starts with service impact and convergence, follows user-facing services, then descends into storage-engine and
runtime causes.

## Identity lattice

```text
cluster:             {}
daemon:              {ceph_daemon}
pool:                {pool_id or pool}
PG:                  {pool_id, pgid}
MDS client surface:  {ceph_daemon, mds_filesystem_key, remaining emitted labels}
RBD image:           {ceph_daemon, librbd_image_key, remaining emitted labels}
RBD PWL:             {ceph_daemon, librbd_pwl_key, remaining emitted labels}
runtime component:   {ceph_daemon, normalized component identity, remaining emitted labels}
RGW gateway/entity:  {instance_id, emitted user/bucket/topic/zone labels}
NVMe-oF entity:      {gateway labels, nqn/host/device labels as emitted}
```

- MGR and ceph-exporter label sets differ by family and remain extensible.
- Every source-defined identity axis is preserved. Pools, daemons, images, users, buckets, gateways, devices, workers, and
  runtime components are not silently merged.
- The recommended job preserves exporter labels named `instance`, `family`, `chart`, or `dimension` under the corresponding
  `ceph_` name before Netdata adds its own Prometheus re-export labels. This changes only the collision-prone label key, not
  the source identity value or entity boundary.
- Open-ended instance identity remains at compatibility boundaries where the official endpoints have no one fixed label
  contract. This can create many chart instances; it is preferable to combining unrelated entities.
- Metric-name identities are converted into labels before curation whenever the source grammar has an unambiguous suffix.

## Source-defined dynamic name grammars

The job policy turns embedded identities into stable labels and canonical metric names. The non-greedy identity capture is
required because suffixes overlap (`rd`/`hit_rd`/`part_hit_rd`, `get`/`get_sum`, and `lock`/`unlock`).

| Source surface | Source evidence (Tentacle commit above) | Normalized identity |
|---|---|---|
| CephFS client metrics | `src/mds/MetricAggregator.cc:164` | `mds_filesystem_key` |
| librbd ImageCtx | `src/librbd/ImageCtx.cc:198` | `librbd_image_key` |
| librbd persistent write log | `src/librbd/cache/pwl/AbstractWriteLog.cc:105-240,621` | `librbd_pwl_key` |
| ObjectCacher | `src/osdc/ObjectCacher.cc:725-751` | `objectcacher_key` |
| Objecter address group | `src/osdc/Objecter.cc` | `objecter_address` |
| RocksDB BinnedLRU cache | `src/kv/rocksdb_cache/BinnedLRUCache.{cc,h}` | `rocksdb_cache_key` |
| Finisher | `src/common/Finisher.cc:29-38` | `finisher_key` |
| Throttle | `src/common/Throttle.cc:56-72` | `throttle_key` |
| arbitrary KernelDevice name | `src/blk/kernel/KernelDevice.cc:101-110` | `kernel_device_key` |
| mClock shard | `src/osd/scheduler/mClockScheduler.cc:100-113` | `mclock_shard` |
| AsyncMessenger worker | `src/msg/async/Stack.h:261-291` | `messenger_worker` |
| RDMA worker | `src/msg/async/rdma/RDMAStack.cc:664-681` | `rdma_worker` |
| DPDK queue and port | `src/msg/async/dpdk/DPDK.cc:628-675`; `src/msg/async/dpdk/DPDK.h:817-835` | `dpdk_queue`, `dpdk_port` |
| configured service identity | `src/common/ceph_context.cc:979-987` | `service_unique_id` |

Three finite source surfaces do **not** use broad relabeling:

- Mempool names are the in-tree enum expanded to `_bytes` and `_items` at `src/common/ceph_context.cc:953-975`; all 60
  resulting families are curated directly.
- SimpleRADOSStriper accepts a logger name, but the shipped call site fixes it to `libcephsqlite_striper` at
  `src/libcephsqlite.cc:129`; its eight families are curated directly from `src/SimpleRADOSStriper.cc:68-78`.
- PriorityCache accepts constructor names, but the three target releases have only the `prioritycache` and
  `bluestore-pricache` in-tree manager call sites (`src/mon/OSDMonitor.cc:1034` and
  `src/os/bluestore/BlueStore.cc:5491`). Those finite families remain directly curated. A generic suffix relabel would
  incorrectly capture unrelated names such as `mempool_ec_extent_cache_bytes`.

## Release and endpoint contracts

- One profile covers Reef, Squid, and Tentacle MGR and ceph-exporter endpoints plus the official NVMe-oF gateway.
- Ceph 19 and Ceph 20 scrub families remain separate causal branches because their contracts differ materially.
- Daemon long-running-average `_sum` values are cumulative. MGR declares them as counters while ceph-exporter declares the
  same daemon-schema sums as gauges; the profile deliberately uses incremental semantics on both wire variants.
- Priority-0 daemon counters and optional ceph-exporter process metrics are in scope.
- CephFS/MDS, CephFS Mirror, RBD, RBD Mirror, RGW multisite/topic/cache and dmClock scheduling, SMB, RDMA,
  DPDK, RocksDB BinnedLRU, external block devices, Ceph client I/O, and NVMe-oF remain in scope even when absent from the
  lab deployments.
- The official NVMe-oF gateway uses `prometheus-client` 0.19.0. Its exporter explicitly unregisters Python GC, so the profile
  curates the remaining process runtime and does not fabricate GC families.
- The NVMe-oF producer's `ceph_nvmeof_subsystem_listener_iface_speed_bytes` suffix is misleading. The implementation reads
  `/sys/class/net/<device>/speed` and exports that integer without conversion, so the profile presents the actual `Mbps`
  source unit rather than inventing bytes per second.
- Untyped fallback is limited to the 24 exact official MGR families observed in these releases; it is not a broad `ceph_*`
  type guess.

## Population and unit rules

- **Source lifecycle owns the algorithm.** Registration type, Prometheus wire type, suffix, HELP text, and one scrape are
  supporting evidence; the initialization, every update callsite, reset, and destruction path establish whether the raw value
  is a current population, cumulative work, or a snapshot total.
- Source-cumulative work and long-running-average sums use incremental algorithms so the Agent performs rate calculation and
  reset detection. The profile never pre-differentiates or implements reset logic.
- Source-current state, capacity, queue depth, inventory, last-seen timestamps, and instantaneous resource values use absolute
  algorithms, even when Ceph registers the source as a counter.
- Source update behavior overrides misleading wire types: CephFS `last_synced_bytes` is declared as a counter but set to the
  last sync's size; BinnedLRU activity and client operations are declared as gauges but source-cumulative. Their chart
  algorithms follow those update sites.
- Same-unit families share a chart only when they answer one causal question for one owner and measure the same population.
- Operation counts do not share an axis with bytes or objects produced by those operations.
- `_count` and `_sum` pairs retain separate unit algebra.
- Client `mdsqsum`, `readsqsum`, and `writesqsum` are cumulative Welford sum-of-squares values in nanoseconds squared. They use
  incremental semantics and a `1e6` divisor, producing `microseconds²/s` as exact, portable floating-point dimensions. The
  chosen scale fits the profile's Go `int` divisor on supported 32-bit Agent builds.

## Source-lifecycle audit evidence

The complete chart inventory was audited against the bounded source revisions above. Representative same-class proofs show why
wire type alone cannot determine the Netdata algorithm or unit:

| Population | Registration and update evidence | Profile result |
|---|---|---|
| BlueFS zero-filled reads | Reef `src/os/bluestore/BlueFS.cc:402-404,839,851,905,916` registers and increments the totals; Squid and Tentacle preserve that lifecycle. | incremental `reads/s` |
| BlueStore OMAP iterators | Reef `src/os/bluestore/BlueStore.cc:4445,4455,5294`; Squid `:5518,5528,6456`; Tentacle `:6519` register a counter but increment on iterator construction and decrement on destruction. | absolute current `iterators` |
| BlueStore GC merged bytes | Reef `src/os/bluestore/BlueStore.cc:5288,16359`; Squid `:6450,17347`; Tentacle `:6513,17598` add each merged extent length. | incremental `bytes/s` |
| MDS expired inodes | Reef `src/mds/MDSRank.cc:3512` registers the value and `src/mds/MDCache.cc:7088` increments it; Squid and Tentacle preserve that cumulative lifecycle. | incremental `inodes/s` |
| MDS large journal events | Squid `src/mds/MDLog.cc:87,441`; Tentacle `:88,535` register and increment one event per enlargement. | incremental `events/s` |
| RDMA worker state | Reef `src/msg/async/rdma/RDMAStack.cc:47-50,68,254,292,311,340,354,584,602,634,648` sets, increments, and decrements active QPs, inflight sends, polling state, and receive buffers as current populations. | absolute exact state units |
| Ceph client sum of squares | Reef `src/client/Client.cc:634-640,771-783,791-802,810-821` reads the prior Welford accumulator and stores the updated cumulative nanoseconds² total. | incremental `microseconds²/s`, divisor `1e6` |

The same audit covered every multi-dimensional chart and every source/type mismatch across cluster, pool, BlueFS/BlueStore,
CephFS/MDS, OSD/recovery/scrubbing, messenger/RDMA/DPDK, RGW, Objecter, RBD/PWL, shared runtime, and NVMe-oF families. The
catalog regression locks critical counter-as-current, gauge-as-cumulative, snapshot-total, and unit-conversion cases.

## Binding exclusions

- Raw `ceph_data_sync_from_<zone>_*` MGR aliases: ceph-exporter normalizes the same source metrics to the stable
  `ceph_data_sync_from_zone_*` families (`src/exporter/DaemonMetricCollector.cc:467-470`). The recommended job excludes the
  nine stable names from the alias block and drops only the source-proven raw suffix grammar to prevent duplicate
  observations; the stable metrics remain charted and unknown suffixes remain forward-open.
- `process_start_time_seconds`: raw NVMe-oF process-start epoch cannot be transformed into age. Process CPU, memory, and file
  descriptors remain charted.
- Five information families are rejected by the writer contract: three NVMe-oF metadata families, `python_info`, and
  `ceph_osd_osd_pg_info`. Their labels and lost metadata questions remain recorded in the source ledger.
- Address-bearing objecter families are **not excluded**. Job relabeling moves the address into `objecter_address`, renames the
  finite operation suffix to a stable canonical family, and the profile charts the result with complete identity.
- No family is excluded because it overlaps the native Ceph collector or because of the meaning of an exported label.

## Forward compatibility

- The source-complete fixture has zero generic fallback and zero unmatched series.
- Exact known exclusions cover only source-proven duplicates or ineligible families.
- Unknown future `ceph_*` families remain generically visible until source evidence can assign identity, unit, population,
  ownership, and a curated destination.

## Reconciliation ledger

- `src/go/testdata/prometheus/profiles/ceph/SOURCE-INVENTORY.tsv` is the binding per-family and exact-selector semantic ledger.
- It accounts for **1,794 source families** in 1,794 rows: **1,779 charted source-family routes**, **10 recommended-job
  exclusions**, and **5 writer-ineligible information families**.
- The profile contains **547 authored charts** and **1,772 unique authored selectors**. The structural union materializes
  **981 chart instances** and **2,937 dimensions**.
- Every row records operator owner, entity identity, signal role, observation population, cross-family relationship, unit
  algebra, label roles and optionality, availability gate, evidence limitation, disposition, destination, and source path.
- Separate producer fixtures prove Reef/Squid/Tentacle MGR versus ceph-exporter contracts and the NVMe-oF registry without
  pretending that their union is one realizable endpoint.
- Unresolved source families and authored selectors: **0**.
