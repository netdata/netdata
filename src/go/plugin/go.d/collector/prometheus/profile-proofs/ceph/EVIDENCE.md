<!-- markdownlint-disable MD013 MD043 -->

# Ceph profile evidence manifest

## Supported source boundary

- Reef 18.2.8: `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df`.
- Squid 19.2.5: `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a`.
- Tentacle 20.2.2: `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc`.
- NVMe-oF gateway: `ceph/ceph-nvmeof @ c79b6f44bd2288f7ec5c48e3cc47f6e566573d3f`.
- NVMe-oF runtime registry: `prometheus/client_python 0.19.0 @ 2dcd17efd0ce2f0a1ad15cb3c150ffcdc42ced65`.
- Official producer contracts: MGR `src/pybind/mgr/prometheus/module.py`, ceph-exporter
  `src/exporter/DaemonMetricCollector.cc`, NVMe-oF `control/prometheus.py`, every daemon performance-schema registration and
  update callsite recorded in `SOURCE-INVENTORY.tsv`, and `doc/mgr/prometheus.rst` at each pinned Ceph revision.

## Feature, release, and producer gates

- One profile covers MGR, ceph-exporter, and NVMe-oF but does not claim their union is one endpoint. MGR cluster metadata and
  ceph-exporter daemon performance are separate producer populations.
- Priority-0 daemon families require `ceph-exporter` with `exporter_prio_limit=0`; default thresholds expose a subset.
- CephFS/MDS and mirror, RBD/PWL and mirror, RGW multisite/topic/cache/dmClock, SMB, messenger/RDMA/DPDK, RocksDB binned cache,
  external block devices, and NVMe-oF are module, daemon-role, build, or configuration gated.
- Tentacle-only BinnedLRU, external-block-device, client-write, and objecter replica-read families are absent from Reef/Squid.
- Source lifecycle, not Prometheus wire type, determines absolute versus incremental algorithms.

## Evidence classes and fixture provenance

- `src/go/plugin/go.d/collector/prometheus/testdata/ceph_all_metrics.prom` is the sanitized structural union of the three Ceph
  releases, official producers, daemon roles, modules, and optional transports. Values and identities are synthetic.
- The seven `src/go/plugin/go.d/collector/prometheus/testdata/ceph_*_all_metrics.prom` producer fixtures separately preserve
  the Reef/Squid/Tentacle MGR and ceph-exporter contracts plus NVMe-oF. Expected strict failures are only dead
  charts/dimensions from release/producer branches absent from that fixture; each has zero generic fallback and zero
  unmatched series.
- Dynamic raw family-name grammars are proven from pinned source. Relabeling retains their dynamic key as identity before
  mapping only finite source-known suffix branches onto stable canonical names.
- The native Ceph Dashboard REST collector is a different source and does not narrow or suppress official exporter coverage.
- `SOURCE-INVENTORY.tsv` binds every declared family/component to exact source, owner, identity, population, unit algebra,
  availability gate, disposition, and authored destination.

## Reproduction and integrity

- `VALIDATION-JOB.yaml` is the sanitized objective-validator input corresponding to the recommended metadata job policy.
- `VALIDATION.md` contains the authoritative union result, producer matrix, and exact commands.
- `SHA256SUMS.tsv` fingerprints every committed semantic and executable proof input.
- The union result requires zero generic fallback and zero unmatched series for all pinned sources. Unknown future `ceph_*`
  families remain eligible for generic fallback.

## Explicit limitations

- The structural union is not one realizable cluster or exporter endpoint.
- Source-derived optional shapes prove schema and routing, not local enablement, values, cadence, or cardinality.
- Live-Agent behavior is validated separately during an authorized lab rollout and cannot replace source-completeness evidence.
