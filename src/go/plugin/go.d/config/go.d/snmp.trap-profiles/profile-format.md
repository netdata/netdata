# SNMP Trap Profile Format

## Overview

An **SNMP trap profile** defines _how a specific class of SNMP trap notifications is decoded, classified, and rendered_ by the Netdata SNMP trap subsystem.

:::info

Trap profiles are reusable and declarative — you never need to modify the
collector source code to support a new device or vendor.

:::

A profile tells the Netdata SNMP trap plugin:

- which **trap OIDs** carry well-known notifications
- which **varbinds** ship with each trap (their OIDs, MIB types, value enums)
- how to **classify** each trap (category + severity)
- how to **render** each trap into a human-readable journal `MESSAGE`

Trap profiles do **not** define journal field names (the plugin always captures
all varbinds in a fixed schema — see the SNMP trap subsystem design doc) and
do **not** define per-trap metric emission (operators opt-in OIDs to metric
emission in plugin configuration, not in profiles).

A profile is a single YAML file. One file per vendor by convention; stock
profiles ship under `default/` and are organized by inferred enterprise-PEN
vendor (`cisco.yaml`, `huawei.yaml`, `juniper.yaml`, …) plus the IETF-standard
file (`standard.yaml`) and the IEEE LLDP file (`ieee-lldp.yaml`).

### How profiles are loaded

The plugin loads stock profiles from `<stock>/go.d/snmp.trap-profiles/default/`
(typically `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/`) and
operator overrides from `<user>/go.d/snmp.trap-profiles/`
(typically `/etc/netdata/go.d/snmp.trap-profiles/`).

Profiles are loaded **only when the SNMP trap plugin is enabled** — Netdata
agents that do not receive traps never pay the memory footprint.

When the plugin is enabled, profile loading is lazy where practical: the
plugin pre-indexes trap OIDs at startup but defers full varbind materialisation
to first-use per vendor.

## File layout

```yaml
# Optional file-scope metadata
vendor: cisco
mib_count: 390
trap_count: 1584

# File-scoped table of varbind metadata, name-keyed.
# Defined once per file; every trap below references entries by name.
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifAdminStatus:
    oid: 1.3.6.1.2.1.2.2.1.7
    type: INTEGER
    enum:
      '1': up
      '2': down
      '3': testing
  cospfConfigErrorType:
    oid: 1.3.6.1.4.1.9.10.101.1.1.2
    type: INTEGER
    enum:
      '1': badVersion
      '2': areaMismatch
      '6': authFailure
      # …

# Trap definitions — each entry references varbinds by name.
traps:
  - oid: 1.3.6.1.4.1.9.10.101.0.1
    name: cospfIfConfigError
    category: state_change
    severity: warning
    description: |
      OSPF configuration mismatch on {SNMP_DEVICE_HOSTNAME}
        local router ID: {ospfRouterId}
        interface IP: {ospfIfIpAddress}
        source of mismatched packet: {cospfPacketSrc}
        error type: {cospfConfigErrorType}
        packet type: {cospfPacketType}
    mib: CISCO-OSPF-TRAP-MIB
    status: current
    varbinds:
      - ospfRouterId
      - ospfIfIpAddress
      - cospfPacketSrc
      - cospfConfigErrorType
      - cospfPacketType
```

## Schema reference

### File-level `varbinds:` table

Name-keyed map of varbind metadata shared across every trap in the file. Each
entry is the slim shipping form of a single MIB object:

| Field         | Required | Type   | Notes |
|---------------|----------|--------|-------|
| `oid`         | yes      | string | Numeric OID of the varbind |
| `type`        | yes      | string | MIB syntax (e.g., `INTEGER`, `OctetString`, `Counter32`, `IpAddress`, `DisplayString`, `ZeroBasedCounter32`) or a TEXTUAL-CONVENTION name from the source MIB |
| `enum`        | no       | map    | Numeric-string → label for `INTEGER` enumerations |
| `constraints` | no       | string | Size/range constraint expression (e.g., `SIZE(0..16)`, `(1..65535)`) |
| `display_hint`| no       | string | Render hint (e.g., `1x:` for MAC, `1d.1d.1d.1d` for IPv4) |

Plugin behaviour: varbinds that the plugin sees on a trap but cannot resolve
via the file table fall back to MIB-index lookup, then to raw OID-keyed
rendering. The varbind still lands in the `SNMP_TRAP_JSON` journal field with
its OID, ASN.1-decoded type, and value.

### Trap entries (`traps:` list)

Each list entry defines one trap notification.

| Field         | Required | Type    | Notes |
|---------------|----------|---------|-------|
| `oid`         | yes      | string  | Numeric OID of the trap |
| `name`        | rec.     | string  | Symbolic name (overrides MIB name when MIB isn't loaded) |
| `category`    | yes      | string  | One of the 8 canonical categories — see below |
| `severity`    | yes      | string  | One of the 8 syslog severities — see below |
| `description` | rec.     | string  | Template rendered into the journal `MESSAGE` field |
| `mib`         | no       | string  | Source MIB module name (informational only) |
| `status`      | no       | string  | MIB status: `current`, `deprecated`, `obsolete` |
| `varbinds`    | no       | list    | Names referencing the file-level table, or inline dicts (see below) |
| `labels`      | no       | map     | Operator-overridable: key → template producing a `TRAP_<KEY>` journal field |
| `dedup_key_varbinds` | no | list | Names of varbinds that participate in the deduplication fingerprint |

#### Varbind references

`varbinds:` on a trap entry is a list. Each element is either:

- **a string** — the varbind name, resolved via the file-level `varbinds:`
  table (the normal case for stock profiles), or
- **a dict** — a full inline `varbind` definition for varbinds that are
  not in the table (operator additions, vendor-specific one-offs):

  ```yaml
  varbinds:
    - ifIndex                      # reference into the file table
    - oid: 1.3.6.1.4.1.99.0.42     # inline definition
      name: customIndex
      type: Counter32
  ```

The order of `varbinds:` on a trap entry is not significant — the plugin
matches arriving varbinds by OID at runtime.

#### Description template

`description:` is a template substituted at render time. Recognized
placeholders:

| Reference                       | Resolved to |
|---------------------------------|-------------|
| `{<varbind_name>}`              | Varbind value, formatted per its `display_hint` / enum |
| `{<varbind_name>.raw}`          | Varbind raw value (numeric for enums, undecoded bytes for OctetString) |
| `{<numeric_oid>}`               | Varbind value by OID when no symbolic name is available |
| `{SNMP_DEVICE_HOSTNAME}`        | Resolved device hostname (sysName or DNS) |
| `{SNMP_SOURCE_IP}`              | UDP source address of the trap PDU |
| `{SNMP_TRAP_NAME}`              | The trap's symbolic name |
| `{SNMP_DEVICE_VENDOR}`          | Inferred device vendor slug |
| `{ND_TOPOLOGY_INTERFACE}`       | Topology-resolved interface, when topology is co-located |
| `{ND_TOPOLOGY_NEIGHBORS}`       | Topology-resolved upstream neighbours, when co-located |

Unresolved references render as `<missing>` for absent varbinds, or
`<unresolved:varname>` for unknown placeholder names. The latter also
increments the `snmp.trap.errors.template_unresolved` plugin metric so
operators notice and can correct the profile.

If `description:` is absent the plugin renders the default template
`"{SNMP_TRAP_NAME} from {SNMP_DEVICE_HOSTNAME} ({SNMP_SOURCE_IP})"`.

The rendered `MESSAGE` is capped at 512 bytes (longer values are truncated
with a marker). Multi-line MESSAGE values are written using systemd-journal's
binary field encoding so embedded newlines never inject other journal fields.

### Categories — closed set

`category` MUST be one of these 8 canonical slugs. Stock profiles use this set;
operator overrides cannot extend it. Cross-cutting concerns (compliance scope,
tenant, datacenter, change window…) are expressed as **labels**, not as new
categories.

| Slug            | Meaning |
|-----------------|---------|
| `state_change`  | Interface/port state, system lifecycle, routing protocol state, environmental state transitions (`linkDown`, `coldStart`, BGP transitions, fan/PSU/temp) |
| `config_change` | Configuration change audit (`ccmCLIRunningConfigChanged` and analogues — who / what / when / from-where) |
| `security`      | Security violations with per-event detail (port-security MAC violations, DHCP-snooping drops, DAI drops, ACL hits, IPS hits) |
| `auth`          | Authentication events with source identity (`authenticationFailure` with source IP, user attempt) |
| `license`       | License / compliance events (expired, violated, feature unlocked) |
| `mobility`      | MAC mobility / topology events with the actor (`macAddressMoved`, STP `newRoot`) |
| `diagnostic`    | Vendor diagnostic events with device-determined context (reboot reasons, module insertion, RAID array, optical transceiver) |
| `unknown`       | No profile coverage — default for OIDs not in the catalogue, or vendor user-defined trap slots whose semantics are operator-determined |

### Severities — closed set, mapped to syslog PRIORITY

`severity` MUST be one of these 8 syslog levels. The plugin maps each to a
numeric `PRIORITY` field on the journal entry:

| Slug      | PRIORITY | Use it for |
|-----------|----------|------------|
| `emerg`   | 0        | System is unusable — exceptional vendor catastrophe |
| `alert`   | 1        | Action must be taken immediately |
| `crit`    | 2        | Critical conditions: hardware failure, security breach in progress |
| `err`     | 3        | Error conditions: failure, fault, denial |
| `warning` | 4        | Warning conditions: threshold breach, degradation, recoverable error |
| `notice`  | 5        | Normal-but-significant: routine state changes, completed operations |
| `info`    | 6        | Informational: status updates, periodic events |
| `debug`   | 7        | Debug-level: rare; reserved for traps the MIB explicitly marks debug |

## Cardinality discipline

The trap subsystem enforces cardinality discipline at the metric / label
surface only — the journal (including the rendered MESSAGE) has no
cardinality restriction. See the trap subsystem design doc §4 for full
rationale.

In profile terms:

- `description:` and `varbinds:` references — **no restriction**. Free use of
  any varbind including high-cardinality data (MAC, IP, username, packet
  content). These land in MESSAGE / SNMP_TRAP_JSON.
- `labels:` — **bounded-cardinality only**. Templates that reference
  unbounded varbinds (MAC, IP, username) are rejected at profile load with
  a clear error. Labels become `TRAP_<KEY>=<VALUE>` journal fields and
  propagate to metric labels.

## Operator overrides

Operators do not copy entire stock profiles to make changes. Instead they
place small override files under `/etc/netdata/go.d/snmp.trap-profiles/` that
add or change entries by OID. The plugin merges overrides on top of stock
profiles; the operator file always wins for any field it specifies.

```yaml
# /etc/netdata/go.d/snmp.trap-profiles/site-additions.yaml
traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1   # Cisco config change
    labels:
      compliance: pci
      tenant: acme
      change_window: business_hours
```

Per-OID metric opt-in (turning a trap into a dedicated chart for alerting)
lives in plugin configuration (`go.d/snmp.trap.conf`), **not** in profiles.
This keeps profiles vendor-curated and installation-agnostic.

## Generated stock profiles

Stock profiles under `default/` are generated by `tools/snmp-traps-profile-gen/`
from public MIB sources and an LLM classification step. The same tools can
re-emit the pack when new MIBs are mirrored or the taxonomy changes. See
`tools/snmp-traps-profile-gen/README.md` for the regeneration recipe.

Edits to files under `default/` are overwritten on regeneration; operator
modifications belong in the user override directory, not in stock files.
