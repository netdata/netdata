<!-- markdownlint-disable MD013 MD043 MD060 -->

# Ceph Prometheus profile validation

## Bounded source scope

- Reef 18.2.8: `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df`.
- Squid 19.2.5: `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a`.
- Tentacle 20.2.2: `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc`.
- NVMe-oF gateway: `ceph/ceph-nvmeof @ c79b6f44bd2288f7ec5c48e3cc47f6e566573d3f`, with
  `prometheus/client_python` 0.19.0 @ `2dcd17efd0ce2f0a1ad15cb3c150ffcdc42ced65`.
- Official producer contracts: MGR Prometheus, ceph-exporter priority-0 daemon schemas, and NVMe-oF. The native Dashboard REST
  collector is a different source and does not narrow this profile.

## Authoritative structural-union result

- `proof.yaml` is the authoritative machine-checked PASS verdict and complete objective count record.
- The exclusion ledger dispositions are 10 recommended-job exclusions and 5 writer-ineligible information families.
- Job-relabel normalization is reconciled by logical identity to charted canonical writer families rather than counted as
  source loss.
- Generic fallback, unmatched series, dead charts/dimensions, materialization loss, and collisions: **0**.
- `src/go/testdata/prometheus/profiles/ceph/SOURCE-INVENTORY.tsv` maps all 1,779 charted source-family routes and 1,772
  unique authored selectors; unresolved families/selectors **0**.

## Source-derived producer matrix

Each producer fixture is intentionally a partial release/role surface. Strict validation therefore reports dead charts and
inactive dimensions belonging to other producers/releases. Every objective error is in those two expected classes; every
surface has zero autogen and zero unmatched series.

| Surface | Raw families | Writer series | Runtime charts | Dimensions | Autogen | Unmatched | Strict error classes |
|---|---:|---:|---:|---:|---:|---:|---|
| Reef MGR | 1,723 | 2,863 | 931 | 2,863 | 0 | 0 | dead chart/dimension only |
| Reef ceph-exporter priority 0 | 1,609 | 2,541 | 785 | 2,541 | 0 | 0 | dead chart/dimension only |
| Squid MGR | 1,597 | 2,737 | 886 | 2,737 | 0 | 0 | dead chart/dimension only |
| Squid ceph-exporter priority 0 | 1,483 | 2,415 | 740 | 2,415 | 0 | 0 | dead chart/dimension only |
| Tentacle MGR | 1,628 | 2,768 | 900 | 2,768 | 0 | 0 | dead chart/dimension only |
| Tentacle ceph-exporter priority 0 | 1,514 | 2,446 | 754 | 2,446 | 0 | 0 | dead chart/dimension only |
| NVMe-oF | 29 | 24 | 21 | 24 | 0 | 0 | dead chart/dimension only |

## Policy and producer checks

- MGR raw RGW source-zone aliases are removed by one source-proven bounded `drop` grammar; the nine stable normalized RGW
  families remain charted, and unknown future `ceph_data_sync_from_*` suffixes remain eligible for generic fallback.
- Official source labels named `instance`, `family`, `chart`, or `dimension` are preserved under a `ceph_` prefix before
  Netdata adds its own Prometheus re-export labels; source-faithful fixtures retain the original exporter label names.
- Address-bearing objecter and BinnedLRU cache names are normalized to stable families with explicit identity labels.
- MGR long-running-average sums remain counters; ceph-exporter exposes the same sums as gauges; both intentionally use
  incremental chart semantics because source update callsites prove cumulative values.
- Source lifecycle rather than Prometheus wire type selects every algorithm: increment/decrement populations such as
  BlueStore OMAP iterators and RDMA active state are absolute, while snapshot/cumulative gauges and counters are incremental.
- Client Welford sum-of-squares dimensions are incremental floating-point `microseconds²/s` after the exact, 32-bit-portable
  `1e6` nanoseconds² conversion.
- Tentacle-only BinnedLRU, external-block-device, client-write, and objecter replica-read families are absent from Reef/Squid
  producer fixtures and present in Tentacle fixtures.
- NVMe-oF retains process/platform runtime, excludes raw process-start epoch, and does not fabricate the GC collector that
  `control/prometheus.py` unregisters.

## Executable regression evidence

- `TestCollector_CephProfileAllMetrics` exercises the structural union through the production collector/relabeler/writer/store,
  catalog, chartengine, and public chart emitter.
- `TestCollector_CephProfileProducerVariants` exercises all seven producer/release fixtures and their wire-type distinctions.
- `TestDefaultCatalog_CephAlgorithmsFollowSourceLifecycle` locks representative source-proven lifecycle, population, algorithm,
  and unit conversions so a future wire-type heuristic cannot silently regress them.
- `TestCollector_CephProfileProducerVariants` is the committed executable proof for producer-specific partial fixtures; raw
  local JSON reports are transient and are not required to reproduce the result.

## Live validation boundary

Live-Agent results are a separate operational rollout check on authorized lab monitoring nodes. They do not replace or narrow
the source-completeness proof, and no Ceph daemon, exporter, or Ceph configuration is modified by this profile.

## Reproducible artifacts

- Profile: `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/ceph.yaml`.
- Union and producer fixtures: `src/go/testdata/prometheus/profiles/ceph/fixtures/ceph*_all_metrics.prom`.
- Job input: `src/go/plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION-JOB.yaml`.
- Semantic proof: `OPERATOR-MODEL.md` and
  `src/go/testdata/prometheus/profiles/ceph/SOURCE-INVENTORY.tsv`.
- Evidence provenance: `EVIDENCE.md`.
- External evidence manifest: `src/go/testdata/prometheus/profiles/ceph/manifest.yaml`.
- Machine descriptor and integrity metadata: `proof.yaml`.

From `src/go`, reproduce the authoritative union result with:

```sh
go run ./tools/prometheus-profile-validation \
  --profile plugin/go.d/config/go.d/prometheus.profiles/default/ceph.yaml \
  --dump testdata/prometheus/profiles/ceph/fixtures/ceph_all_metrics.prom \
  --job plugin/go.d/collector/prometheus/profile-proofs/ceph/VALIDATION-JOB.yaml \
  --output text
```

Reproduce every producer/release route with:

```sh
go test ./plugin/go.d/collector/prometheus -run TestCollector_CephProfileProducerVariants -count=1
```
