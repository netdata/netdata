# SNMP diagnostics

This source-only maintainer tool reads built-in SNMP diagnostic files: topology, lifecycle, and normal per-device
metrics/BGP/licensing evidence. Topology replay and inspection use the collector's production paths. It is run from the repository and is not installed with the Netdata Agent.

**Place in the documentation set.** This README owns the maintainer CLI interface and output interpretation.
The developer skill `.agents/skills/triage-snmp-diagnostics/SKILL.md` links to its sections for the investigation
workflow.

> Diagnostic archives can contain device addresses, hostnames, descriptions, inventory data, and topology evidence.
> Treat an archive and the tool output as sensitive support material.

## Usage

Run commands from `src/go`. Every successful operation writes one JSON document to standard output; `validate` checks
the selected diagnostic document and reports its identity.

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

### Input selection

The input can be a single diagnostic `.zst` file, the `snmp/diagnostics` directory, an extracted support-bundle
root, or a support `.tar.zst`, `.tar.gz`, or `.zip` archive. Bundle inputs automatically use `06-state/snmp-diagnostics`.
Archives may contain that layout directly or under one wrapper directory; ambiguous roots, duplicate relevant members,
unsafe paths, and linked evidence are rejected. The tool does not extract files. Directory inputs confine reads to the
supplied directory tree; relative symlinks that stay within that tree are allowed.

Directory and bundle input selects the latest topology checkpoint by default. `--checkpoint N` selects a retained
sequence; `--lifecycle` selects lifecycle state; `--normal --registration-id N` selects a normal device. These evidence
selectors cannot be combined or used with an individual file. `--previous-run` requires `--normal`. Explicit lifecycle-file
input remains supported. Only the current diagnostic layout is supported.

```text
go run ./tools/snmp-diagnostics list --input /path/to/support-bundle.tar.zst
go run ./tools/snmp-diagnostics summary --input /path/to/support-bundle.tar.zst --checkpoint 3
go run ./tools/snmp-diagnostics summary --input /path/to/support-bundle.tar.zst --lifecycle
go run ./tools/snmp-diagnostics inspect-device --input /path/to/support-bundle.tar.zst --normal --registration-id 7
go run ./tools/snmp-diagnostics inspect-device --input /path/to/support-bundle.tar.zst --normal --previous-run --registration-id 7
go run ./tools/snmp-diagnostics replay --input /path/to/support-bundle.tar.zst
```

### Collection status and partial listings

`list` takes a directory or bundle without selectors and reports lifecycle availability, topology checkpoints, and indexed
normal device files without decoding evidence documents. Bundle listings also include `bundle.collection_status`: the
producer's original collection notes, including why evidence was not requested, unavailable, or partially copied. A
reported `complete` means the producer copied its selected files; it does not validate their contents or establish that
all files describe one simultaneous sample. Missing/unreadable status is `null`, with an explanation in `errors`.

Listing errors for one component appear in `errors` alongside the remaining inventory, with exit code zero. For example,
an invalid normal-run index does not hide topology or lifecycle evidence. Normal selection uses only the committed run
index; it never guesses current/previous from directory names. A missing index yields no indexed normal devices.
A selected missing or invalid document fails; the tool does not silently substitute another checkpoint or evidence kind.

### Normal device evidence

Normal files support `validate`, `summary`, and `inspect-device`; replay and link inspection require topology
evidence.
Normal `inspect-device` reports the latest attempt, retained last failure, metric samples, BGP/licensing cache state,
profile context, and linked source operations. Cached values may originate in an earlier attempt. Compare their source
and observation timestamps; publication time is not the time every value was collected. Device registration IDs belong to
a process run, so do not join current lifecycle rows to previous-run normal files or historical topology by ID alone.

A normal `summary` identifies one device and gives counts and failure indicators; use `inspect-device` for the
underlying attempts and evidence. The inspection body is `NormalDevice` from
`src/go/plugin/go.d/collector/snmp/diagnostics/normal.go`; its producer identity is available through `validate` or
`summary`. Keep that identity with the inspection report when comparing runs.

The attempt-level fields below live under `latest_attempt` or `last_failed_attempt`; `profile_context`,
`initialization_sources`, and `source_operations` are device-level fields.

| Evidence | Interpretation |
|---|---|
| `latest_attempt` / `last_failed_attempt` | Latest completed attempt and a retained failure; their timestamps qualify any comparison. |
| `profiles` / `profile_context` | Captured profile selection, acquisition results, processed metrics, and profile rows. |
| `metric_decisions` / `samples` | Decisions linking processed metrics to sample IDs, and the emitted sample values. |
| `bgp_cache` / `licensing_cache` | Captured normal-collector state, its update/normalization timestamps, and source references. |
| `cache_inputs` / `initialization_sources` | Retained inputs that may predate the latest attempt. |
| `source_operations` | The operation table used by the source references in this device cut. |

Match a source reference's `context_id` and `operation` to a source operation's `context_id` and `ordinal`. Operation
keys belong to one runtime; producer run ID and runtime ID qualify cross-file comparisons. Normal inspection does not
replay chart creation or reconstruct the Agent's time-series history.

#### Acquisition, processing, and samples

A normal attempt's `failed` flag includes source-operation and profile-processing failures, even when collection
returned usable samples from other work. It is not an all-or-nothing poll result. Inspect the recorded failures and
samples together. A `phase: check` attempt records initialization/check evidence but no sample map; absence of samples
there is expected. These boundaries are implemented by `finishNormalAttempt` and `Check` in the SNMP collector.

Acquisition `MetricValueReferences` describe values before normalization, duplicate selection, virtual-metric
derivation, and hidden-metric filtering. Their ordinals are not indexes into the final `profiles[].metrics` slice.
Profile metrics describe a later processing stage; `metric_decisions.sample_ids` links collector emission decisions to
the final `samples` map. Several decisions can target the same sample. See the collector architecture's [metric
emission](/src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#metric-emission) section for row identity and accumulation,
and the profile guide's [virtual metrics](/src/go/plugin/go.d/collector/snmp/profile-format.md#virtual-metrics) section
for derived values. The acquisition boundary is defined by `AcquisitionValueReference` in
`ddsnmp/ddsnmpcollector/acquisition_report.go` under the SNMP collector.

#### Cached inputs and earlier outcomes

A route's cache source does not imply old metric values: cached table structure can supply the OIDs for a fresh GET.
`Reused` instead marks an entire route outcome inherited from an earlier attempt. Cached profile tags and metadata can
therefore coexist with newly acquired metric values. Consult operation timestamps and bindings rather than dating
all fields from the route's source label. `AcquisitionRouteReport` defines this distinction.

`DiscardedSources` records rejected acquisition candidates, such as a cached GET replaced by a WALK; those operations
are not the accepted value source. `negative_causes` preserves original missing-OID evidence for configured sources
whose requests are subsequently suppressed. An absent request in the latest attempt alone does not mean the OID was
never tried. This is retained negative evidence, not a history of every disappearing table instance.

#### Cached state versus Function output

Normal inspection copies stored BGP and licensing state through `captureNormalBGP` and `captureNormalLicensing`; it
does not run their Function handlers. For licensing, `normalized_at` dates the stored normalized set. Interpret that
state using the collector architecture's consumer rules:

- [BGP collection and retained rows](/src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#bgp-collection-and-retained-rows)
- [Licensing normalization and collection results](/src/go/plugin/go.d/collector/snmp/ARCHITECTURE.md#licensing-normalization-and-collection-results)

### Topology replay and inspection

For topology, `summary` reports captured cuts and an ordered registration inventory. `replay` emits the production
topology-v1 payload. Inspection reports one device or link across captured evidence, graph, and rendered topology
stages. Link reports also include family-wide source context; that context is not causal provenance for the inspected
link. Stage availability and retained-success/latest-attempt interpretation are explained in
[Offline Diagnostic Inspection](/src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#offline-diagnostic-inspection).

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

### Read limits and container cost

Decode operations accept human-readable limits for the **selected diagnostic document** (128 MiB compressed and
512 MiB decoded JSON by default). They can be overridden for a particular invocation:

```text
--max-compressed-size 256MiB --max-decoded-size 1GiB
```

These limits do not cap the size or traversal cost of the enclosing support bundle. Compressed tar input requires one
complete streaming inventory pass, then at most one additional pass to reach the selected document. Even unrelated tar
payloads must pass through decompression; repeated CLI invocations repeat this work. ZIP input reads central-directory
metadata and opens relevant members directly. For repeated analysis of a large tar bundle, supplying an already extracted
bundle root avoids those scans.

The tool retains member metadata and collection-status/run-index contents, not diagnostic payloads. Only the selected
inner document is decoded. Tar inventory checks the outer compression stream; ZIP checks the members it reads. Neither
listing nor validating one selected document is a validation of every file in the support bundle.

### Exit codes

Exit codes are `0` for success (including a partial listing with component errors), `1` for evidence-selection, input, or
operation errors, and `2` for command/flag parsing and argument-validation errors. Evidence-selector conflicts such as
`--lifecycle --normal`, or `--previous-run` without `--normal`, return `1`.

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
