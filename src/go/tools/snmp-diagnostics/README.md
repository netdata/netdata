# SNMP diagnostics

This source-only maintainer tool reads built-in SNMP diagnostic files: topology, lifecycle, and normal per-device
metrics/BGP/licensing evidence. Topology replay and inspection use the collector's production paths. It is run from the repository and is not installed with the Netdata Agent.

> Diagnostic archives can contain device addresses, hostnames, descriptions, inventory data, and topology evidence.
> Treat an archive and the tool output as sensitive support material.

## Usage

Run commands from `src/go`:

```text
go run ./tools/snmp-diagnostics validate --input /path/to/archive.zst
go run ./tools/snmp-diagnostics summary --input /path/to/archive.zst
go run ./tools/snmp-diagnostics replay --input /path/to/archive.zst
go run ./tools/snmp-diagnostics inspect-device --input /path/to/archive.zst --registration-id 7
go run ./tools/snmp-diagnostics inspect-link --input /path/to/archive.zst --link-index 12
go run ./tools/snmp-diagnostics inspect-link --input /path/to/archive.zst \
  --source-identity ip:192.0.2.10 \
  --destination-identity ip:192.0.2.20 \
  --family lldp \
  --direction bidirectional
```

The input can be a single `.zst` file or the `snmp/diagnostics` directory (including the copy under
`06-state/snmp-diagnostics` in a support bundle). Directory input selects the latest topology checkpoint by default;
`--checkpoint N` selects a retained sequence. Select `lifecycle.zst` explicitly to inspect lifecycle state.
Only the current diagnostic layout is supported.

```text
go run ./tools/snmp-diagnostics list --input /path/to/snmp/diagnostics
go run ./tools/snmp-diagnostics summary --input /path/to/snmp/diagnostics --checkpoint 3
go run ./tools/snmp-diagnostics summary --input /path/to/snmp/diagnostics/lifecycle.zst
go run ./tools/snmp-diagnostics inspect-device --input /path/to/snmp/diagnostics --normal --registration-id 7
go run ./tools/snmp-diagnostics inspect-device --input /path/to/snmp/diagnostics --normal --previous-run --registration-id 7
```

`list` reports topology checkpoints and normal device files without decoding them. It takes a directory without selectors.
Normal files support `validate`, `summary`, and `inspect-device`; replay and link inspection require topology evidence.
Normal `inspect-device` reports the latest attempt, retained last failure, metric samples, BGP/licensing cache state,
profile context, and linked source operations. Cached values may originate in an earlier attempt. Compare their source
and observation timestamps; publication time is not the time every value was collected. Device registration IDs belong to
a process run, so do not join current lifecycle rows to previous-run normal files or historical topology by ID alone.

Every successful operation writes one JSON document to standard output. `validate` verifies the complete archive and
reports its identity. `summary` reports the captured cuts and an ordered registration inventory. `replay` emits the
production topology-v1 payload. The inspection operations report one device or link across captured evidence, graph, and
rendered topology stages. Link reports also include family-wide source context; that context is not causal provenance for
the inspected link.

Use `summary` to find a device registration ID. Use `--link-index` to inspect one existing link by its zero-based row in
the `links` table emitted by `replay`. The index belongs to that archive and query option set; do not reuse it with a
different archive or query. Exact selection also works when an actor identity cannot be carried in a command-line
argument or when the identity selector would match multiple parallel links.

The identity-based link selector remains useful for investigating a link that may be absent. A device inspection reports
its graph identity keys; one of those keys can be supplied as a link endpoint identity. The exact and identity-based
selector modes cannot be combined. Link families are `lldp`, `cdp`, `bridge`, `fdb`, `stp`, `arp`, `l3_subnet`,
`l3_subnet_membership`, `ospf_adjacency`, and `bgp_adjacency`. Link direction is required for identity selection and
accepts `observed`, `unidirectional`, or `bidirectional`.

Replay and inspection accept the production scalar query options:

```text
--collapse-actors-by-ip=true|false
--eliminate-non-ip-inferred=true|false
--map-type managed_fabric|lldp_cdp_managed|high_confidence_inferred|all_devices_low_confidence
--inference-strategy fdb_minimum_knowledge|stp_parent_tree|fdb_pairwise_minimum_knowledge|stp_fdb_correlated|cdp_fdb_hybrid
--managed-device-focus all_devices|ip:ADDRESS[,ip:ADDRESS...]
--depth all|0..10
```

Decode operations accept human-readable reader limits (128 MiB compressed and 512 MiB decoded by default). Their generous defaults are intended for normal Agent-produced
archives and can be overridden for a particular invocation:

```text
--max-compressed-size 256MiB --max-decoded-size 1GiB
```

Exit codes are `0` for success, `1` for archive or operation failure, and `2` for invalid command-line usage.

## Collection cost

Topology `inspect-device` includes `collection_contexts` under both `latest_attempt` and `retained_success`. Each profile has
aggregate `stats` and, when recorded, an `execution` block:

- `preparation` measures profile tags/metadata acquisition and cached-input copying. It includes elapsed nanoseconds,
  logical GET calls, requested OIDs, SNMP errors, missing OIDs, and processing errors. This work also appears in the
  profile totals; do not add the breakdown to those totals again.
- `walk_operations` references executed source operations in the collection context. Those operations record roots,
  returned OIDs and typed values, elapsed time, and structured failure outcomes. Duration includes client processing,
  pagination, and retries inside `WalkAll`/`BulkWalkAll`; it is not network RTT or a per-packet measurement.
  A successful Handler return does not prove complete table coverage: terminal SNMP response reasons are not yet recorded.
- Shared walks appear once, with bindings from consuming routes. Cached or dormant consumers do not cause another walk.
  Repeated roots in separate passes or VLAN contexts remain separate executions.
- An absent execution block means not recorded; a present block with zero requests means measured no such work.

Compare latest-attempt and retained-success accounting separately; `same_attempt` indicates when they alias the same
capture. Do not sum them when they alias. GET/walk counts are Handler calls, not network packets or retry counts.
`walk_pdus` counts varbinds returned by successful walk calls. Missing-OID counts include configured sources already
known unavailable as well as received missing-value exceptions. Processing errors can coexist with usable values or an
ignored metadata rule.

Profile phase totals are not complete device-refresh wall time: connection setup, discovery-only metadata, profile
selection, and sysUptime work are outside them. Aggregate table/BGP/licensing timing includes local processing, so it
can exceed the sum of walk durations. The existing `failure_phase: prepare` is a control-flow boundary that also covers
scalar acquisition; use the separate timing fields to distinguish preparation from scalars.
