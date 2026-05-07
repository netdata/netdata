# SNMP Profile Projection

## Purpose

SNMP profiles are one catalog with explicit projections for their consumers.
Regular SNMP metric collection and SNMP topology use the same profile loading,
matching, inheritance, metadata, and tag machinery, but they consume different
profile views.

## Consumers

The supported profile consumers are:

- `metrics` - regular SNMP charted metrics and virtual metrics.
- `topology` - SNMP topology observations.

Profile metadata fields and top-level `metric_tags` default to both consumers.
They may narrow their visibility with:

```yaml
consumers: [metrics]
consumers: [topology]
```

Metric rows under top-level `metrics:` are regular metric rows. They are
metrics-only.

Topology rows live under top-level `topology:` and must declare a closed
`kind`.

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

`ProjectedView.FilterByKind()` is a topology view filter. VLAN-context topology
uses it with the VLAN-scopable kind set instead of hardcoded topology mixin
filenames.

## Inheritance And Merge Rules

Profile inheritance must merge `topology:` rows in addition to `metrics:`,
`virtual_metrics`, metadata, global metric tags, and static tags.

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
topology rows in the resolved matched set.

## Delivery

Regular metrics are emitted through `ProfileMetrics.Metrics`.

Topology rows are emitted through `ProfileMetrics.TopologyMetrics` and carry
`Metric.TopologyKind`.

`ProfileMetrics.HiddenMetrics` remains a generic delivery container for
underscore-prefixed non-topology metrics. SNMP topology must not depend on
hidden metrics.

Top-level `metric_tags` on topology projections are profile/device labels. They
are applied through topology profile-tag ingestion and are not topology row
dispatch keys.

## Validation Guarantees

Profile validation rejects:

- unknown topology kinds;
- underscore-prefixed topology row value symbol names;
- regular metric chart/export-only fields on topology row value symbols;
- invalid `consumers` values;
- virtual metrics whose sources resolve to topology rows.
