# SNMP Profile Projection

## Purpose

SNMP profiles are one catalog with explicit projections for their consumers.
Regular SNMP metric collection, SNMP topology, SNMP licensing, and SNMP BGP
monitoring use the same profile loading, matching, inheritance, metadata, and
tag machinery, but they consume different profile views.

## Consumers

The supported profile consumers are:

- `metrics` - regular SNMP charted metrics and virtual metrics.
- `topology` - SNMP topology observations.
- `licensing` - typed SNMP network-device license rows.
- `bgp` - typed SNMP BGP device, peer, and peer-family rows.

Profile metadata fields and top-level `metric_tags` default to all consumers.
They may narrow their visibility with:

```yaml
consumers: [metrics]
consumers: [topology]
consumers: [licensing]
consumers: [bgp]
```

Metadata resource `id_tags` do not carry `consumers` today. They inherit the
metadata defaults used by charted metrics and topology and are not included in a
licensing-only projection.

Metric rows under top-level `metrics:` are regular metric rows. They are
metrics-only.

Topology rows live under top-level `topology:` and must declare a closed
`kind`.

Licensing rows live under top-level `licensing:` and emit typed license rows,
not chart metrics and not hidden underscore-prefixed metrics.

BGP rows live under top-level `bgp:` and emit typed BGP rows, not chart metrics
and not hidden underscore-prefixed metrics.

## Topology Rows

Topology rows reuse the regular `MetricsConfig` scalar/table shape:

```yaml
topology:
  - kind: lldp_rem
    table:
      OID: 1.0.8802.1.1.2.1.4.1
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.6
        name: lldp_rem
    metric_tags:
      - tag: lldp_loc_port_num
        index: 2
```

Topology row symbol names must not be underscore-prefixed. The historical
`_topology_*` naming convention is not a classifier.

Topology rows must not set regular metric chart/export fields on the row value
symbol: `chart_meta`, `metric_type`, `mapping`, `transform`, `scale_factor`,
`format`, or `constant_value_one`.

`systemUptime` remains in `metrics:` for regular SNMP collection. It is not a
topology kind. Topology-specific uptime acquisition is collector code, not
profile topology schema.

## Topology Kinds

The closed topology kind set is:

- `lldp_loc_port`
- `lldp_loc_man_addr`
- `lldp_rem`
- `lldp_rem_man_addr`
- `lldp_rem_man_addr_compat`
- `cdp_cache`
- `if_name`
- `if_status`
- `if_duplex`
- `ip_if_index`
- `bridge_port_if_index`
- `fdb_entry`
- `qbridge_fdb_entry`
- `qbridge_vlan_entry`
- `stp_port`
- `vtp_vlan`
- `arp_entry`
- `arp_legacy_entry`

## Licensing Rows

Licensing rows are row-centric because one license row can aggregate identity,
descriptors, state, timers, and usage signals:

```yaml
licensing:
  - id: vendor-license-group
    table:
      OID: 1.3.6.1.4.1.example.1
      name: vendorLicenseTable
    identity:
      id: { index: 1 }
      name:
        symbol:
          OID: 1.3.6.1.4.1.example.1.2
          name: vendorLicenseName
    state:
      symbol:
        OID: 1.3.6.1.4.1.example.1.3
        name: vendorLicenseState
      mapping:
        1: "0"
        2: "1"
        3: "2"
    signals:
      expiry:
        timestamp:
          symbol:
            OID: 1.3.6.1.4.1.example.1.4
            name: vendorLicenseExpiry
          format: snmp_dateandtime
```

Scalar-only licensing rows are allowed. If a scalar row combines multiple
scalar signal OIDs into one license row, it must declare an explicit `id:` so
the grouping is stable. Otherwise scalar structural identity defaults to the
single scalar signal OID.

`from: <oid>` lets a typed licensing value read a sibling OID directly. For
table rows, `from` must be a peer column under the same table OID. For scalar
rows, schema validation only checks OID syntax and the row's explicit identity
rules; there is no cross-profile reference path in profile validation.

Supported licensing signal fields are:

- state severity: `state`
- timers: `expiry`, `authorization`, `certificate`, `grace`
- usage: `used`, `capacity`, `available`, `percent`

Timer signals may declare exactly one of the shorthand timer value,
`timestamp`, or `remaining`. Sentinel policies are closed names and are
evaluated by the typed licensing producer before runtime consumers see the row.

## BGP Rows

BGP rows are row-centric because one BGP row can aggregate identity,
descriptors, connection state, counters, timers, route counts, limits, and
error fields:

```yaml
bgp:
  - id: std-peer
    MIB: BGP4-MIB
    kind: peer
    table:
      OID: 1.3.6.1.2.1.15.3
      name: bgpPeerTable
    identity:
      neighbor:
        symbol:
          OID: 1.3.6.1.2.1.15.3.1.7
          name: bgpPeerRemoteAddr
          format: ip_address
      remote_as:
        symbol:
          OID: 1.3.6.1.2.1.15.3.1.9
          name: bgpPeerRemoteAs
          format: uint32
    state:
      symbol:
        OID: 1.3.6.1.2.1.15.3.1.2
        name: bgpPeerState
        mapping:
          items:
            1: idle
            2: connect
            3: active
            4: opensent
            5: openconfirm
            6: established
    connection:
      established_uptime:
        symbol:
          OID: 1.3.6.1.2.1.15.3.1.16
          name: bgpPeerFsmEstablishedTime
```

BGP row kinds are closed:

- `device`
- `peer`
- `peer_family`

Device rows are identity-free device summaries. Supported `device_counts`
fields are `peers`, `ibgp_peers`, `ebgp_peers`, and per-state fields under
`states`. `ibgp_peers` and `ebgp_peers` map to the public
`bgp.devices.peer_counts` dimensions `ibgp` and `ebgp`; `peers` maps to
`configured`.

Peer rows must declare stable `neighbor` and `remote_as` identity fields.
Peer-family rows must additionally declare canonical `address_family` and
`subsequent_address_family` identity fields.

The BGP state mapping is a closed RFC 4271 state contract. Rows that declare
state must map all six states by default:

- `idle`
- `connect`
- `active`
- `opensent`
- `openconfirm`
- `established`

Partial state coverage is allowed only when the row explicitly declares
`partial: true`. That escape hatch records that the profile author knowingly
accepts a partial source MIB. `partial_states: [...]` records the canonical
states that the source can represent.

AFI/SAFI values are normalized to closed canonical strings at profile load.
Known address families include `ipv4`, `ipv6`, and `l2vpn`. Known subsequent
address families include `unicast`, `multicast`, `labeled_unicast`, `mvpn`,
`vpls`, `evpn`, and `vpn`. Vendor-private values require an explicit
`allow_private: true` on that identity value.

BGP keeps normal ddsnmp table-row chart behavior. A typed BGP table row can
still produce one chart per row; the typed projection controls domain identity,
validation, function output, and old-protocol deletion, not a separate
BGP-specific chart cap.

BGP value fields can read from the current row, from row indexes, from literal
values, or from a related table in the same resolved profile. Row-index value
sources can use `index: N`, `index_transform:`, or `index_from_end: N`.
`index_from_end` selects one OID index component from the tail and is for MIBs
where the wanted trailing INDEX component follows a variable-length component
such as `InetAddress`. Profiles should declare only one row-index selector per
typed BGP value. Validation rejects configs that set more than one of
`index`, `index_from_end`, and `index_transform` on the same typed BGP value.

A cross-table typed value uses `table: <table_name>` with `index_transform:`
to derive the referenced row index:

```yaml
identity:
  remote_as:
    table: vendorBgpPeerTable
    index_transform:
      - start: 0
        drop_right: 2
    symbol:
      OID: 1.3.6.1.4.1.example.2.1
      name: vendorBgpPeerRemoteAs
```

If the referenced table is keyed differently, a typed BGP value may also use
`lookup_symbol:`. The collector first derives a lookup value from the current
row index with `index_transform:`, scans the referenced table for a row whose
`lookup_symbol` column has that value, and then reads the requested typed
`symbol` from the matched row. This is required for MIBs such as Juniper
BGP4-V2 where peer-family counters are indexed by peer ID, AFI, and SAFI while
peer identity is stored in a peer table keyed by routing-instance/local/remote
address components.

Cross-table typed values are internal BGP row sources. They do not require
synthetic metric labels and must not reintroduce underscore-prefixed side
protocols. Scalar BGP rows cannot use `table:` sources.

When a peer or peer-family row does not expose a routing instance, public chart
and function labels normalize the empty value to `default`.

BGP function cache freshness is tracked per typed source profile. A failed BGP
source preserves its stale rows during the bounded stale window without
blocking other BGP sources from refreshing. After the stale window expires, the
function omits expired stale rows and returns 503 when no usable rows remain.

## Resolve And Projection

`ddsnmp.Catalog.Resolve()` resolves profiles by `sysObjectID`, `sysDescr`, and
manual profile policy. The regular SNMP collector uses manual-profile fallback
semantics. The topology collector uses manual-profile augment semantics.

`ResolvedProfileSet.Project(metrics)` returns the regular metrics view:

- keeps `metrics`;
- keeps `virtual_metrics`;
- drops `topology`;
- filters metadata and top-level metric tags by `consumers`.

`ResolvedProfileSet.Project(topology)` returns the topology view:

- keeps `topology`;
- drops regular `metrics`;
- drops `virtual_metrics`;
- filters metadata and top-level metric tags by `consumers`.

`ResolvedProfileSet.Project(licensing)` returns the licensing view:

- keeps `licensing`;
- drops regular `metrics`;
- drops `topology`;
- drops `virtual_metrics`;
- filters metadata and top-level metric tags by `consumers`.

`ResolvedProfileSet.Project(bgp)` returns the BGP view:

- keeps `bgp`;
- drops regular `metrics`;
- drops `topology`;
- drops `licensing`;
- drops `virtual_metrics`;
- filters metadata and top-level metric tags by `consumers`.

The regular SNMP collector uses `Project(metrics, licensing)` so one SNMP pass
can produce charted metrics and typed license rows. Single-consumer projections
remain pure.

The regular SNMP collector can use `Project(metrics, licensing, bgp)` when BGP
typed rows are produced alongside ordinary charted metrics and licensing rows.
Single-consumer projections remain pure.

`ProjectedView.FilterByKind()` is a topology view filter. VLAN-context topology
uses it with the VLAN-scopable kind set instead of hardcoded topology mixin
filenames.

## Inheritance And Merge Rules

Profile inheritance must merge `topology:`, `licensing:`, and `bgp:` rows in
addition to `metrics:`, `virtual_metrics`, metadata, global metric tags, and
static tags.

Topology row identity is:

```text
kind + table identity + symbol name
```

The table identity is the table name when set, otherwise the table OID. Scalar
topology rows use kind plus scalar symbol name and OID.

When a derived topology row overrides an inherited row with the same identity,
the derived row wins. Conflicting topology kinds for the same table/symbol
identity are load errors.

Cross-profile deduplication runs after profile matching because it depends on
matched-profile specificity. It must deduplicate both regular metrics and
topology/licensing rows in the resolved matched set.

Runtime licensing row structural identity is:

```text
origin profile id + table OID + row index
origin profile id + scalar signal OID
origin profile id + explicit scalar group id
```

`origin profile id` is the logical profile file that declared the licensing row,
including mixin-origin rows after `extends:` merge. It is not the root matched
profile name and not an absolute workstation path. Repeated `(structural
identity, signal kind)` entries are load errors unless a valid inheritance
override handles them.

Profile inheritance merge identity is the pre-collection form of that identity:

```text
table OID
scalar signal OID
explicit scalar group id
```

Derived `licensing:` rows with the same merge identity replace inherited rows.
This keeps intentional `extends:` overrides valid while duplicate signal kinds
inside one resolved profile remain load errors.

Runtime BGP row structural identity is:

```text
origin profile id + row kind + typed config id + table OID + row index
origin profile id + row kind + scalar signal OID
origin profile id + row kind + explicit scalar group id
```

The typed config id is part of table-row structural identity because a single
SNMP table row can legitimately produce multiple typed BGP rows with different
semantics, such as separate peer-family views over one operational table. The
runtime key still length-prefixes every component, including an empty config id.

Within that structural identity, display fields such as peer description,
local address, and peer identifier never determine storage identity. During the
legacy-to-typed BGP migration, typed structural identity owns typed row and
function row identity; chart and alert identity remain bound to the existing
public chart context and label contract until the legacy BGP path is removed.

Profile inheritance merge identity is the pre-collection form of that identity:

```text
row kind + table OID + optional row id
row kind + scalar signal OID
row kind + explicit scalar group id
```

Derived `bgp:` rows with the same merge identity replace inherited rows. Repeated
`(structural identity, signal kind)` entries are load errors unless a valid
inheritance override handles them.

## Delivery

Regular metrics are emitted through `ProfileMetrics.Metrics`.

Topology rows are emitted through `ProfileMetrics.TopologyMetrics` and carry
`Metric.TopologyKind`.

Licensing rows are emitted through `ProfileMetrics.LicenseRows` and carry
typed grouped fields for identity, descriptors, state, timers, usage, tags,
origin profile id, table OID, row key, and structural id.

BGP rows are emitted through `ProfileMetrics.BGPRows` and carry typed grouped
fields for row kind, identity, descriptors, admin/state, connection, traffic,
transitions, timers, last-error, last-notification, reason, graceful-restart,
route, route-limit, device-count, tags, origin profile id, table OID, row key,
and structural id. BGP does not use a generic signal-kind map; each field has
its own numeric, boolean, text, or state shape.

`ProfileMetrics.HiddenMetrics` remains a generic delivery container for
underscore-prefixed non-topology and non-licensing metrics. SNMP topology and
SNMP licensing must not depend on hidden metrics. SNMP BGP must also not depend
on hidden metrics or underscore-prefixed tag protocols.

Licensing row counts are reported through `Stats.Metrics.Licensing`. Ordinary
`Stats.Metrics.Tables` and `Stats.Metrics.Rows` remain regular chart-metric
table counters. Licensing timing and processing failures use their own
licensing fields in timing and processing-error stats.

Top-level `metric_tags` on topology projections are profile/device labels. They
are applied through topology profile-tag ingestion and are not topology row
dispatch keys.

## Validation Guarantees

Profile validation rejects:

- unknown topology kinds;
- underscore-prefixed topology row value symbol names;
- regular metric chart/export-only fields on topology row value symbols;
- unknown licensing signal, sentinel, and state policy names;
- licensing rows without state or signals;
- scalar licensing rows that group multiple scalar signal OIDs without an
  explicit `id`;
- scalar licensing rows with only literal values and no explicit `id`;
- repeated licensing signal kinds for the same structural identity;
- licensing table `from` OIDs outside the row table;
- underscore-prefixed licensing value names;
- regular metric chart/export-only fields, transforms, scale factors, and
  constant-value hacks on licensing row value symbols;
- `extract_value`, `match_pattern`, or `match_value` on licensing row value
  symbols;
- timer slots that set both timestamp-style and remaining-style values;
- unsupported licensing value formats;
- unknown BGP row kinds;
- BGP peer rows without neighbor or remote AS identity;
- BGP peer-family rows without neighbor, remote AS, AFI, or SAFI identity;
- BGP state mappings that do not cover all six RFC 4271 states unless explicit
  partial coverage is declared;
- unknown BGP peer-state, AFI, or SAFI values;
- repeated BGP typed fields for the same structural identity;
- device-count fields on non-device BGP rows;
- route and route-limit fields on non-peer-family BGP rows;
- BGP table `from` OIDs outside the row table;
- BGP scalar values that use `table:` sources;
- BGP scalar values that use row-index sources such as `index:`,
  `index_from_end:`, or `index_transform:`;
- BGP values that set more than one row-index selector (`index`,
  `index_from_end`, or `index_transform`);
- BGP cross-table value sources without a source OID;
- BGP cross-table value sources whose source or lookup OID is outside the
  referenced table when that referenced table is declared by another BGP row in
  the resolved profile;
- BGP `lookup_symbol` value sources without a referenced table;
- underscore-prefixed BGP value names;
- regular metric chart/export-only fields, transforms, scale factors, and
  constant-value hacks on BGP row value symbols;
- invalid `consumers` values;
- virtual metrics whose sources resolve to topology rows.
