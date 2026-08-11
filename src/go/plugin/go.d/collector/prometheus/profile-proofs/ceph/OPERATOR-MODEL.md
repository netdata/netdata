<!-- markdownlint-disable MD013 MD043 MD060 -->

# Ceph Prometheus operator model

## Responsibility

This document owns human semantic decisions: operator domains, entity identity, causal order, population lifecycle, unit
interpretation, and the reasons for exclusions. External `SOURCE-SEMANTICS.yaml` owns source provenance and semantic
declarations; the generated source registry owns the mechanical registration surface. `proof.yaml` and replay tests own
all machine-verifiable behavior and coverage.

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

1. Cluster health and capacity: service impact, raw/usable capacity, OSD state, pools, PG state, recovery, and backfill.
2. Control plane: MON, MGR, and cephadm work that can block convergence or orchestration.
3. OSD client I/O: workload, latency, queues, throttles, failures, and retries.
4. Recovery and scrubbing: repair work, reservations, outcomes, and release-specific scrub phases.
5. Storage engine: BlueStore, BlueFS, RocksDB, block devices, compaction, allocation, cache, I/O, and space pressure.
6. CephFS/MDS: requests, clients, sessions, metadata cache, journal, memory, and purge queue.
7. RBD and RBD Mirror: per-image I/O, persistent write log, object cache, and replication progress/outcomes.
8. RGW: operations, latency/bytes, cache, lifecycle, notifications, topics, and multisite synchronization.
9. NVMe-oF: gateway/reactor state, host connectivity, subsystem inventory, namespace/listener topology, and bdev I/O.
10. Shared runtime: schedulers, messenger workers, RDMA/DPDK, finishers, throttles, memory pools, and exporters.

Navigation starts with service impact and convergence, follows user-facing services, then descends into storage-engine and
runtime causes. Signals remain with the component that can explain them instead of being grouped globally by unit or type.

## Identity model

```text
cluster:             {}
daemon:              {ceph_daemon}
pool:                {pool_id or pool}
PG:                  {pool_id, pgid}
MDS client surface:  {ceph_daemon, mds_filesystem_key, remaining emitted labels}
RBD image:           {ceph_daemon, librbd_image_key, remaining emitted labels}
RBD PWL:             {ceph_daemon, librbd_pwl_key, remaining emitted labels}
runtime component:   {ceph_daemon, normalized component identity, remaining emitted labels}
RGW entity:          {instance_id, emitted user/bucket/topic/zone labels}
NVMe-oF entity:      {gateway labels, nqn/host/device labels as emitted}
```

- MGR and ceph-exporter label sets differ by family and remain extensible.
- Source-defined ownership axes are preserved; daemons, pools, images, gateways, users, devices, and workers are not silently
  merged.
- Collision-prone source label keys are preserved under Ceph-specific keys before Netdata adds its own re-export labels.
- Open-ended identity is retained where official endpoints have no fixed label contract. Higher chart cardinality is safer
  than combining unrelated entities.

## Profile-owned name normalization

Ceph embeds identity in several metric-family names. Profile relabeling, not the job, extracts that identity and maps only
source-proven finite suffix branches to stable canonical families before profile selection. Non-greedy identity capture is
required where operation suffixes overlap.

The normalized identity classes include CephFS clients, RBD images and persistent write logs, objecter addresses, cache and
device names, finishers, throttles, scheduler shards, messenger/RDMA workers, DPDK queues/ports, and configured service IDs.
Finite source registries such as mempools and fixed in-tree logger names remain directly curated; broad relabeling would risk
capturing unrelated families.

Raw RGW source-zone aliases are a special duplicate surface. The profile drops only the source-proven raw suffix grammar and
retains the stable normalized families. Unknown suffixes are not silently classified as known duplicates.

## Release and producer semantics

- MGR and ceph-exporter can expose the same daemon schema with different Prometheus wire types. Long-running-average sums are
  cumulative in source and therefore remain incremental in both variants.
- Release-specific scrub and optional-daemon surfaces remain distinct causal branches where their contracts differ.
- NVMe-oF process/platform metrics are part of the gateway surface; Python GC is not fabricated because that producer
  unregisters it.
- The NVMe-oF listener speed family has a misleading bytes suffix. Its implementation reads the kernel interface speed value,
  so the chart uses the source's Mbps meaning.
- Untyped fallback is restricted to exact official MGR families whose source lifecycle is known; it is not a namespace-wide
  type guess.

## Population and unit rules

- Source lifecycle owns the algorithm. Registration type, wire type, suffix, HELP text, and one scrape are supporting
  evidence; initialization, updates, resets, and destruction determine whether a value is current state, cumulative work, or
  a snapshot.
- Cumulative work uses incremental algorithms. Current state, capacity, inventory, queue depth, and instantaneous resources
  use absolute algorithms even when Ceph registers them as counters.
- Gauge-as-cumulative and counter-as-current exceptions remain intentional when update callsites prove those lifecycles.
- Same-unit families share a chart only when they answer one causal question for one owner and population.
- Operation counts do not share an axis with bytes or objects produced by those operations. Histogram/count/sum roles retain
  their distinct unit algebra.
- Client Welford sum-of-squares values are cumulative nanoseconds squared. They use incremental semantics and a portable
  conversion to `microseconds²/s`; the Agent performs rate calculation and reset handling.

## Exclusion semantics

- Raw process-start epoch cannot be transformed into age, so it is excluded while CPU, memory, and file descriptors remain.
- Writer-ineligible information families retain their lost metadata questions in the source contract rather than being
  presented as numeric state.
- Duplicate aliases are excluded only when source proves the canonical family represents the same observation.
- Address-bearing objecter families are not discarded: the profile extracts their address identity and charts the finite
  operation branches.
- No family is excluded merely because a native Ceph collector overlaps its subject.

## Forward compatibility

Exact exclusions and normalizations cover source-proven names or grammars only. Unknown future `ceph_*` families remain
eligible for generic fallback until evidence can assign ownership, identity, population, units, and a curated destination.
