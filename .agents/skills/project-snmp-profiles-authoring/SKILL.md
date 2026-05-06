---
name: project-snmp-profiles-authoring
description: Use when editing Netdata SNMP profile YAMLs, topology SNMP profiles, ddsnmp profile parsing, or profile-format documentation. Requires checking source MIB field accessibility, especially MAX-ACCESS not-accessible INDEX objects, before adding or changing profile symbols.
---

# SNMP Profile Authoring

Use this skill before editing files under:

- `src/go/plugin/go.d/config/go.d/snmp.profiles/`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/`
- `src/go/plugin/go.d/collector/snmp/profile-format.md`
- `src/go/plugin/go.d/collector/snmp_topology/`

## Required Checks

1. Identify the source MIB object for every profile field being added or changed.
2. Check the object's `MAX-ACCESS`.
3. If the object is `not-accessible`, do not configure it as a readable `symbol.OID`.
4. If a `not-accessible` object appears in the table `INDEX`, derive it from the row OID index using `index` or `index_transform`.
5. Keep index extraction and value formatting separate:
   - use `index` for one index component;
   - use `index_transform` for multiple components;
   - use `symbol.format` only for final formatting such as `ip_address`, `mac_address`, or `hex`.

## Index Rules

- `index` is 1-based.
- `index_transform.start` and `index_transform.end` are 0-based and inclusive.
- `index_transform: [{start: N}]` keeps index component `N` through the last component when `N > 0`.
- `index_transform: [{start: 0, end: 0}]` keeps only the first index component.
- `drop_right` can be used when the right side has fixed trailing components.

## Common Patterns

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
rg -n -C 4 'OBJECT-TYPE|MAX-ACCESS[[:space:]]+not-accessible|ACCESS[[:space:]]+not-accessible' path/to/MIB
```

For known topology-sensitive symbols, scan profile YAMLs before committing:

```bash
rg -n 'name:[[:space:]]*(dot1qTpFdbAddress|ipNetToPhysicalIfIndex|ipNetToPhysicalNetAddressType|ipNetToPhysicalNetAddress|lldpLocManAddrSubtype|lldpLocManAddr)\b' src/go/plugin/go.d/config/go.d/snmp.profiles
```

Any hit must be reviewed. It is valid only when the tag is index-derived and does not declare `symbol.OID` for a `not-accessible` object.

## Validation

Run the narrow suites for the changed area:

```bash
cd src/go
go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition
go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector
go test ./plugin/go.d/collector/snmp_topology
```

See `src/go/plugin/go.d/collector/snmp/profile-format.md` for the full profile syntax and the "Field accessibility" section.
