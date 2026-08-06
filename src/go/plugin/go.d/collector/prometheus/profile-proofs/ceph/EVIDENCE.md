<!-- markdownlint-disable MD013 MD043 -->

# Ceph profile evidence record

## Responsibility

This document records upstream provenance, fixture-construction boundaries, and evidence limitations. It does not own
validator verdicts, counts, findings, file paths, or integrity facts; those are declared in `proof.yaml` and checked by the
descriptor-backed replay tests.

## Supported source boundary

- Reef 18.2.8: `ceph/ceph @ efac5a54607c13fa50d4822e50242b86e6e446df`.
- Squid 19.2.5: `ceph/ceph @ abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a`.
- Tentacle 20.2.2: `ceph/ceph @ 0fcffee29411e3a38036764817b6e1afc59741cc`.
- NVMe-oF gateway: `ceph/ceph-nvmeof @ c79b6f44bd2288f7ec5c48e3cc47f6e566573d3f`.
- NVMe-oF runtime registry: `prometheus/client_python 0.19.0 @ 2dcd17efd0ce2f0a1ad15cb3c150ffcdc42ced65`.
- Producer contracts: MGR `src/pybind/mgr/prometheus/module.py`, ceph-exporter
  `src/exporter/DaemonMetricCollector.cc`, NVMe-oF `control/prometheus.py`, daemon performance-schema registrations and update
  callsites, and `doc/mgr/prometheus.rst` at the pinned revisions.

## Availability boundary

- The profile spans MGR, ceph-exporter, and NVMe-oF producers; it does not claim that their union appears on one endpoint.
- Priority-0 daemon families require ceph-exporter configured with `exporter_prio_limit=0`.
- Daemon role, module, build, release, and operator configuration gate CephFS, RBD, RGW, SMB, messenger, storage-engine, and
  NVMe-oF surfaces.
- Tentacle adds families not present in Reef or Squid. Producer-specific fixtures preserve those release distinctions.
- The Ceph Dashboard REST collector is a separate source and does not narrow the official Prometheus exporter contract.

## Fixture provenance

- The source-complete fixture is a sanitized structural union derived from the pinned registrations and update callsites.
- Supplemental fixtures preserve individual release/producer surfaces. They are partial evidence, not alternate completeness
  boundaries.
- Identities and values are synthetic and non-production. No private endpoint, credential, deployment label, or operating
  value is committed.
- The external source inventory is the exact source-to-disposition ledger. `proof.yaml` declares the consumed fixtures and
  verifies the latest external directory as one complete input set.

## Limitations

- The structural union is not one realizable cluster or exporter endpoint.
- Source-derived optional shapes prove schema and routing, not local enablement, value distribution, cadence, or cardinality.
- Live-Agent validation is a separate authorized rollout check and cannot replace or narrow source-completeness evidence.
