---
name: collectors-snmp-profiles
description: Author or review Netdata SNMP polling profiles, ddsnmp parsing, topology/licensing/BGP rows and profile-format documentation. Check source MIB accessibility and row-index extraction for affected symbols. Trap profiles and topology payload design use their separate skills.
---

# SNMP Profile Authoring

Use this skill when authoring or reviewing the profile contracts in:

- `src/go/plugin/go.d/config/go.d/snmp.profiles/`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/`
- `src/go/plugin/go.d/collector/snmp/profile-format.md`
- `src/go/plugin/go.d/collector/snmp_topology/`

Boundary: this skill owns the profile `topology:` rows, which OIDs feed the SNMP topology producer and how they are
extracted. What the producer emits from those observations (actors, links, evidence, modals) is the payload contract
in `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`; the developer workflow for changing it is
`.agents/skills/topology-authoring/SKILL.md`.

## Select The Affected Contract

Apply `AGENTS.md#skill-selection`. Review uses the checks below and existing source/validation evidence; it does not
create an implementation SOW or require a live device capture. Live access remains within the task's authorization.
Verify every added or changed acquisition symbol against its source MIB. A prose-only change does not require a new
MIB audit of untouched symbols; parser changes still need the relevant input and consumer contracts.

| Affected behavior | Read/check |
|---|---|
| Acquired OIDs, index extraction or a device deviation | Required Checks, Device-Scoped MIB Deviations and Index Rules below; `src/go/plugin/go.d/collector/snmp/profile-format.md#field-accessibility` |
| Profile syntax, inheritance or parser behavior | Canonical Profile Syntax, full profile-format owner and parser/consumer tests |
| Topology, licensing or BGP rows | Their Required Checks and Audit Recipe items; follow the typed producer/consumer paths |
| Emitted topology payload | `topology-authoring`; this skill still covers affected profile acquisition |
| Received trap profiles or trap logs | `collectors-snmp-trap-profiles` or `query-snmp-traps` |

## Required Checks

1. Identify the source MIB object for every profile field being added or changed, and confirm the configured OID is
   that object's OID: a column `<entry>.<column>` (the entry is `<table>.1`) under its row's `table.OID`, or a scalar
   with its `.0` instance. Profiles with these mistakes still load (an OID outside `table.OID` only logs a warning),
   and a row whose columns double the entry arc still parses, so run the opt-in MIB check under Validation
   (`src/go/plugin/go.d/collector/snmp/profile-format.md#table-metrics-multiple-rows`).
2. Check the object's `MAX-ACCESS` (SMIv2) or `ACCESS` (SMIv1).
3. Polling inputs MUST be readable: `not-accessible`, `accessible-for-notify`, and `write-only` are not polling
   access. Notification-only and write-only fields MUST NOT supply GET/WALK inputs. A device-scoped deviation for
   a `not-accessible` source MUST satisfy every requirement below and the exact source-role guard; a vendor name
   or OID alone never authorizes an exception.
4. If a non-pollable object appears in the table `INDEX`, derive it from the row OID index using `index`,
   `index_transform`, or `index_decode`. A table root itself need not be readable; check the actual acquired column.
5. Keep index extraction and value formatting separate:
   - use `index` for one raw index arc and `index_transform` for arc slices;
   - use `index_decode` for a full logical INDEX layout with variable or fixed-width octet strings;
   - use `symbol.format` for final formatting; text from decimal index octets requires `octet_string`, while
     `string` preserves its existing dotted-decimal behavior. See the Index Rules owner below.
6. Resolve a cross-table tag through a row-index component (for example `ifXTable` via `index_transform`) only when
   the MIB defines that component as the target table's index, such as an `InterfaceIndex`. A port number scoped to
   a group, module or stack member (for example POWER-ETHERNET-MIB `pethPsePortIndex`) is not an ifIndex: identify
   those rows by their own index tags and leave interface labels absent unless a MIB mapping such as ENTITY-MIB
   `entAliasMappingTable` can be followed.
7. Put SNMP topology rows under top-level `topology:` with a required closed
   `kind`. Do not mark topology rows by naming metrics `_topology_*`.
8. Do not use chart/export-only value fields on topology row anchor symbols:
   `chart_meta`, `metric_type`, `mapping`, `transform`, `scale_factor`,
   or `format`.
9. Keep regular `systemUptime` rows under `metrics:`. Do not model uptime as a
   topology row kind; topology-specific uptime acquisition belongs in collector
   code, not profile topology schema.
10. Put SNMP licensing rows under top-level `licensing:`. Do not model licensing
   telemetry as underscore-prefixed hidden metrics or `_license_*` tag
   protocols.
11. Licensing row value symbols may use `format` and exact `mapping`, but must
   not use chart/export fields, transforms, scale factors, or
   underscore-prefixed generated names.
12. For scalar licensing rows that combine multiple scalar signal OIDs into one
   license row, declare an explicit stable `id:`. For table licensing rows,
   keep every effective OID source (`symbol.OID`, `from`, or legacy `OID`) inside the same table OID and derive
   `not-accessible` INDEX values from the row index.
13. Put SNMP BGP rows under top-level `bgp:`. Do not model BGP telemetry as
   vendor-specific raw metrics, `virtual_metrics`, or underscore-prefixed tag
   protocols when adding or migrating BGP coverage.
14. BGP peer-state mappings must use the six RFC 4271 canonical states
   (`idle`, `connect`, `active`, `opensent`, `openconfirm`, `established`).
   Use `partial: true` plus `partial_states` only when the source MIB is
   intentionally partial.
15. Typed BGP value fields derive `not-accessible` INDEX objects with exactly one selector: `index`, `index_from_end`,
   or `index_transform`. Use `index_from_end` for trailing AFI/SAFI-like components after variable-length indexes
   such as `InetAddress`. BGP row `metric_tags` use `index`, `index_transform`, or `index_decode` for row-index
   identity; follow the Index Rules below.
16. To constrain a topology table to a fixed structural index prefix, use one
    readable anchor symbol and set `table.OID` to `<anchor-column>.<prefix>`.
    A simple same-index cross-table dependency inherits that prefix and is
    anchor-gated only when its target table has no symbol-bearing producer.
    A target route with a real symbol owner remains eager. Do not rely on
    propagation for ordinary metrics, multiple anchors, `lookup_symbol`,
    `index_transform`, `index_decode`, `index_from`, or conflicting dependency scopes.

## Device-Scoped MIB Deviations

A vendor profile MAY poll an object that the source MIB marks `not-accessible` only when all of these requirements hold:

- **Scope:** The polling row is declared in a selector-matched device or topology-role profile. A generic standards
  profile MUST NOT carry the deviation.
- **Proof:** A checked repository fixture or repeatable live capture proves that the matched device returns the object as
  a readable column and omits the standards-readable anchor needed to obtain the row otherwise.
- **Isolation:** The device row overrides the generic row identity instead of adding a second generic fallback or a second
  topology kind. Every effective consumer, including direct and transitive inheritors, MUST retain the exact
  capture-proven selector; checking only the profile that declares the source does not establish polling scope.
- **Documentation:** A short profile comment explains the vendor deviation and why index-only decoding cannot be used.
- **Regression:** Tests pin both the selector-scoped override and real fixture collection through the profile parser and
  consumer path.

Do not generalize a device deviation to related models or vendors without equivalent evidence.

## Canonical Profile Syntax

Stock profiles MUST use symbol objects for OID/name pairs, including metric tags and typed licensing/BGP values.
Put metric type overrides on each affected `symbol` or `symbols` entry instead of the deprecated row-level `metric_type`.
Legacy custom profile aliases remain supported; typed OID precedence is `symbol.OID`, then `from`, then legacy `OID`.
Collection, boundary validation and inheritance identity use that same source. Prefer one OID source form per value.
Metric and topology column rows require `table.OID`; `table.name` is optional for those rows.
The retired `constant_value_one` key is ignored. Ordinary metric symbols still require OIDs; do not use the key to generate or suppress metrics.

## Index Rules

- `index` is 1-based.
- `index_transform.start` and `index_transform.end` are 0-based and inclusive.
- `index_transform: [{start: N}]` keeps index component `N` through the last component when `N > 0`.
- `index_transform: [{start: 0, end: 0}]` keeps only the first index component.
- `drop_right` can be used when the right side has fixed trailing components.
- For `index_decode`, declare every logical component, including unselected components, and select with one-based
  `component`. An `octet_string` without `length` is length-prefixed; a positive fixed `length` omits the prefix.
  Layout, extraction and final formatting failures MUST reject the entire identity row and keep it out of caches.
- Cross-table joins require authoritative equivalence of their keys, including every ownership axis. Equal-looking
  numeric indexes do not prove equal meaning; process-less metadata cannot be attributed to process-scoped rows
  without a source contract. Omit unproved enrichment rather than manufacture identity or drop valid counters.

Compound syntax and validation live in
`src/go/plugin/go.d/collector/snmp/profile-format.md#compound-index-decoding-index_decode`.
For compound identity changes, test final chart IDs with every required axis, first WALK, cached GET and profile
clone/reload. Include punctuation, empty components, malformed unselected components and downstream formatting errors,
not only decoder failures. Preserve exact semantic identity through acquisition and caches; defer wire sanitation to
the renderer. Exercise the full path with names that normalize to the same display label, not just prebuilt metric
maps. Strict row rejection must retain bounded error summaries for ordinary and typed tables. Machine identity and
display labels have separate contracts; see
`src/go/plugin/go.d/collector/snmp/profile-format.md#5-metric_tags`.
V1 `Dim.ID` is a job-scoped sample lookup key; `DIMENSION` and `SET` use `Dim.Name` when set. Do not infer
wire identity or name validation from an internal key, or replace chart-scoped names with job-scoped keys.
Typed BGP identity and optional-value availability are defined in
`src/go/plugin/go.d/collector/snmp/profile-format.md#bgp-rows`. Preserve configured process identity through all
consumers, including Functions, topology inventory and diagnostic replay. In topology, preserve exact nonblank process and
routing-instance identifiers through grouping and output; absence/default handling must not rewrite those tokens. Missing optional values and failed
SNMP dependency walks are different cases and MUST be tested separately.

## Common Patterns

Interface alias label:

- Every `interface`/`_interface` tag read from `ifName` MUST have an `_if_alias` sibling: a copy of that tag with only
  `tag`, `symbol.OID` and `symbol.name` changed; keep the table and complete lookup chain identical.
  `TestStockProfiles_IfNameInterfaceTagsCarryIfAlias` enforces it.
- The underscore keeps the alias out of chart IDs; the collector emits it as the `if_alias` chart label.

Value-keyed table joins use
`src/go/plugin/go.d/collector/snmp/profile-format.md#chained-value-keys-index_from`. Verify each intermediate column
against its source MIB, including compound target indexes and RowPointer type. Validate missing, malformed and
recovered links through collection and final chart-label output, not only the resolver. Dependency tests MUST cover
all hops, shared routes and cache invalidation; never propagate a source index prefix through a value-derived key.

```yaml
- tag: interface
  table: ifXTable
  MIB: IF-MIB
  symbol:
    OID: 1.3.6.1.2.1.31.1.1.1.1
    name: ifName
- tag: _if_alias
  table: ifXTable
  MIB: IF-MIB
  symbol:
    OID: 1.3.6.1.2.1.31.1.1.1.18
    name: ifAlias
```

Q-BRIDGE learned FDB MAC:

```yaml
- tag: dot1q_fdb_mac
  symbol:
    format: mac_address
  index_transform:
    - start: 1
```

IP-MIB `ipNetToPhysicalTable` address:

```yaml
- tag: arp_ip
  symbol:
    format: ip_address
  index_transform:
    - start: 3
```

The `start: 3` skips `ifIndex`, address type, and the InetAddress length byte.

IP-MIB `ipAddressTable` IPv4 address:

```yaml
table:
  OID: 1.3.6.1.2.1.4.34.1.3.1.4
  name: ipAddressTableIPv4
symbols:
  - OID: 1.3.6.1.2.1.4.34.1.3
    name: ip_if_index
metric_tags:
  - tag: topo_ip_addr
    symbol:
      name: ipAddressAddr
    index_transform:
      - start: 2
```

The table root anchors on readable `ipAddressIfIndex` and appends index prefix
`1.4` for IPv4 type and its required four-octet length. The address is the
not-accessible `ipAddressAddr` index component; `start: 2` skips address type
and length. The open-ended slice is intentional: a fixed `end` would hide
malformed trailing components instead of letting the strict consumer reject
the row. Keep the transformed components raw and require the topology
consumer to decode exactly four decimal octets in `0..255`; structural walk
scope alone does not validate the remaining instance suffix. A malformed
descendant can activate dependency walks, but it MUST NOT emit topology.

LLDP-MIB local management address:

```yaml
- tag: lldp_loc_mgmt_addr
  symbol:
    name: lldpLocManAddr
    format: hex
  index_transform:
    - start: 2
```

The `start: 2` skips management-address subtype and length. Use `hex`, not
`ip_address`, because LLDP management addresses can carry non-IP subtypes; the
topology runtime normalizes IP-compatible bytes later.

## Audit Recipe

When a profile reads a table column, verify that the MIB object is readable:

```bash
rg -n -C 4 'OBJECT-TYPE|MAX-ACCESS[[:space:]]+(not-accessible|accessible-for-notify)|ACCESS[[:space:]]+(not-accessible|write-only)' path/to/MIB
```

For known topology-sensitive symbols, scan profile YAMLs before committing:

```bash
rg -n 'name:[[:space:]]*(dot1qTpFdbAddress|ipAddressAddr|ipNetToPhysicalIfIndex|ipNetToPhysicalNetAddressType|ipNetToPhysicalNetAddress|lldpLocManAddrSubtype|lldpLocManAddr|lldpRemManAddrSubtype|lldpRemManAddr)\b' src/go/plugin/go.d/config/go.d/snmp.profiles
```

Any hit must be reviewed. It is valid only when the tag is index-derived without a `symbol.OID`, or when it satisfies every
device-scoped MIB-deviation requirement above.

When adding a new topology kind, update all three parts together:

- profile YAML using `topology: - kind: <kind>`;
- the Go `TopologyKind` enum and validation;
- the topology cache handler registry and tests.

Verify that topology rows are delivered through `ProfileMetrics.TopologyMetrics`,
not through underscore-prefixed `HiddenMetrics`.

Several kinds can feed one record: `if_name`, `if_status` and `if_duplex` rows all update one interface per ifIndex and
arrive separately. A handler MUST store only the fields its row carries. Derive a value that combines columns from
different rows (port speed from `ifHighSpeed` and `ifSpeed`) after ingest, so row order cannot change it.

When adding or migrating licensing profile coverage, update all related parts
together:

- profile YAML using `licensing:`;
- the closed licensing signal/state/sentinel enums and validation when adding
  new policy names;
- typed `ProfileMetrics.LicenseRows` producer/consumer tests;
- MIB evidence for every OID and every `not-accessible` index-derived field.

Verify that licensing rows are delivered through `ProfileMetrics.LicenseRows`,
not through underscore-prefixed `HiddenMetrics`.

When adding or migrating BGP profile coverage, update all related parts
together:

- profile YAML using `bgp:`;
- closed BGP row kind, peer-state, AFI/SAFI, and typed field validation when
  adding new policy names or value domains;
- typed `ProfileMetrics.BGPRows` producer/consumer tests;
- MIB evidence for every OID and every `not-accessible` index-derived field;
- SNMP integration metadata and generated docs when public BGP capability
  claims change.

Verify that BGP rows are delivered through `ProfileMetrics.BGPRows`, not
through `metrics:`, `virtual_metrics:`, or underscore-prefixed hidden metrics.

When adding or refactoring SNMP profile, parser, or topology tests, prefer
table-driven cases using `map[string]struct{}` keyed by test-case name when
the cases share setup and assertion shape. Use separate test functions only for
materially different setup or assertions.

## Validation

Select the suites for the affected behavior and its consumers. Existing relevant results count as review evidence;
run local checks when needed to verify a claim, without treating review as a request to change profiles. From `src/go`:

```bash
# Profile schema, parsing and validation
go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition
# Stock profile loading and stock-wide rules (table scope, interface aliases)
go test ./plugin/go.d/collector/snmp/ddsnmp
# Acquisition, index decoding and typed row delivery
go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector
# Topology consumption of profile observations
go test ./plugin/go.d/collector/snmp_topology
```

`TestStockProfiles_PollingAccessibility` is always on. Its exact module/object/access evidence and source-file hashes
are pinned in `src/go/plugin/go.d/collector/snmp/ddsnmp/testdata/profile-mib-access.json`. New unknown polling sources
fail; unresolved existing source roles remain an explicit bounded baseline, never proof of readability.
Only the exact RB750Gr3 selector/row/source deviations pinned by the guard and checked capture are allowed.

When changing acquisition inputs, verify the manifest against its recorded MIB corpus revision. The refresh command
and exception evidence are owned by
`src/go/plugin/go.d/collector/snmp/profile-format.md#stock-polling-source-guard`. Refresh MUST NOT silently expand the
unresolved baseline or accept changed pinned source hashes. The opt-in OID-resolution check remains useful for
structural errors and reports absent MIB coverage; it is not a substitute for access evidence.

```bash
NETDATA_SNMP_MIB_DIRS=<pinned-mibs> go test -count=1 ./plugin/go.d/collector/snmp/ddsnmp \
  -run 'TestStockProfiles_(PollingAccessibility|AccessManifestAgainstMIBs|OIDsResolveAgainstMIBs)'
```
