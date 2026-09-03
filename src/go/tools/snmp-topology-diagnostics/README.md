# SNMP topology diagnostics

This source-only maintainer tool reads an SNMP topology diagnostic archive and exposes the same replay and inspection
paths used by the collector. It is run from the repository and is not installed with the Netdata Agent.

> Diagnostic archives can contain device addresses, hostnames, descriptions, inventory data, and topology evidence.
> Treat an archive and the tool output as sensitive support material.

## Usage

Run commands from `src/go`:

```text
go run ./tools/snmp-topology-diagnostics validate --archive /path/to/archive.zst
go run ./tools/snmp-topology-diagnostics summary --archive /path/to/archive.zst
go run ./tools/snmp-topology-diagnostics replay --archive /path/to/archive.zst
go run ./tools/snmp-topology-diagnostics inspect-device --archive /path/to/archive.zst --registration-id 7
go run ./tools/snmp-topology-diagnostics inspect-link --archive /path/to/archive.zst --link-index 12
go run ./tools/snmp-topology-diagnostics inspect-link --archive /path/to/archive.zst \
  --source-identity ip:192.0.2.10 \
  --destination-identity ip:192.0.2.20 \
  --family lldp \
  --direction bidirectional
```

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

All operations accept human-readable reader limits. Their generous defaults are intended for normal Agent-produced
archives and can be overridden for a particular invocation:

```text
--max-compressed-size 256MiB --max-decoded-size 1GiB
```

Exit codes are `0` for success, `1` for archive or operation failure, and `2` for invalid command-line usage.

## Collection cost

`inspect-device` includes `collection_contexts` under both `latest_attempt` and `retained_success`. Each profile has
aggregate `stats` and, when recorded, an `execution` block:

- `preparation` measures profile tags/metadata acquisition and cached-input copying. It includes elapsed nanoseconds,
  logical GET calls, requested OIDs, SNMP errors, missing OIDs, and processing errors. This work also appears in the
  profile totals; do not add the breakdown to those totals again.
- `walks` lists actual executed roots, elapsed nanoseconds, and whether the Handler call returned an error. Each
  duration includes client processing, pagination, and retries inside `WalkAll`/`BulkWalkAll`, but excludes subsequent
  local PDU-map construction and row processing. It is not network RTT or a per-packet measurement. `failed: false`
  does not prove complete table coverage; terminal SNMP response reasons are not recorded.
- Shared walks appear only under the profile charged for the actual operation, not under each consuming route.
  Cached or dormant consumers add no walk. Repeated roots in separate passes or VLAN contexts are separate executions.
- An absent `execution` block means **not recorded**, as in older archives. A present block with zero requests or an
  empty walk list means measured no such work. Older aggregate counters exclude preparation, and older scalar timing
  can omit failed or topology scalar work; reading an archive cannot recover those measurements.

Compare latest-attempt and retained-success accounting separately; `same_attempt` indicates when they alias the same
capture. Do not sum them when they alias. GET/walk counts are Handler calls, not network packets or retry counts.
`walk_pdus` counts varbinds returned by successful walk calls. Missing-OID counts include configured sources already
known unavailable as well as received missing-value exceptions. Processing errors can coexist with usable values or an
ignored metadata rule.

Profile phase totals are not complete device-refresh wall time: connection setup, discovery-only metadata, profile
selection, and sysUptime work are outside them. Aggregate table/BGP/licensing timing includes local processing, so it
can exceed the sum of walk durations. The existing `failure_phase: prepare` is a control-flow boundary that also covers
scalar acquisition; use the separate timing fields to distinguish preparation from scalars.
