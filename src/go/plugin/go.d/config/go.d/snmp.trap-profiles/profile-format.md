<!-- markdownlint-disable MD013 MD043 -->

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

Trap profiles do **not** define journal field names manually. The plugin derives
indexed `TRAP_VAR_*` journal fields from received non-sensitive, non-redundant
event varbinds, and also keeps the complete structured audit copy in
`TRAP_JSON` (see the SNMP trap subsystem design doc). Profiles also do **not**
define per-trap metric emission; operators opt-in OIDs to metric emission in
plugin configuration, not in profiles.

A profile is a single YAML file. One file per vendor by convention; stock
profiles ship under `default/` and are organized by inferred enterprise-PEN
vendor (`ciscosystems.yaml`, `huawei-technology-co-ltd.yaml`,
`juniper-networks-inc.yaml`, …) plus the IETF-standard file (`standard.yaml`)
and the IEEE LLDP file (`ieee-lldp.yaml`).

### How profiles are loaded

The plugin reads stock profiles from the go.d stock config directory
`snmp.trap-profiles/default/` subdirectory (typically
`/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/`) and operator
profiles from the go.d user config directory `snmp.trap-profiles/` subdirectory
(typically `/etc/netdata/go.d/snmp.trap-profiles/`). Stock files are plain
`.yaml` in the source repository for reviewability; installed packages ship
them as `.yaml.zst`. The loader accepts `.yaml`, `.yml`, `.yaml.zst`, and
`.yml.zst`; draft-era `.yaml.gz` and `.yml.gz` files are also accepted as a
compatibility fallback.

Profiles are loaded **only when the first runnable SNMP trap job is created** —
Netdata agents that do not receive traps never pay the memory footprint.

Profile loading is shared across trap jobs: the first runnable job eagerly
loads operator profiles, validates stock profiles, builds a stock OID route
table, and later jobs reuse the same cache. Stock profile definitions are loaded
into memory only when the first matching trap OID needs that stock file. Failed
profile validation is a creation-time job failure surfaced to DynCfg; it does
not leave a permanently poisoned cache. If all trap jobs are removed, the cache
is released and the next trap job creation validates profiles again.

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
    name: CISCO-OSPF-TRAP-MIB::cospfIfConfigError       # MIB-qualified, globally unique
    category: state_change
    severity: warning
    description: |
      OSPF configuration mismatch on {_HOSTNAME}
        local router ID: {ospfRouterId}
        interface IP: {ospfIfIpAddress}
        source of mismatched packet: {cospfPacketSrc}
        error type: {cospfConfigErrorType}
        packet type: {cospfPacketType}
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

> **Future field**: a `display_hint` member (render hint such as `1x:` for MAC
> or `1d.1d.1d.1d` for IPv4) will be added when the plugin's renderer needs
> it. The current extractor keeps DISPLAY-HINT data in intermediate JSONL when
> `gomib` exposes it, but the stock profile emitter does not write
> `display_hint`; the field is reserved for the renderer work that will consume
> it.

Plugin behaviour: varbinds that the plugin sees on a trap but cannot resolve
via the file table fall back to raw OID-keyed rendering — there is **no
runtime MIB compilation** in the plugin. Operators convert custom MIBs to
profile YAMLs offline with the installed helper
`/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` and drop the generated
YAML files under `/etc/netdata/go.d/snmp.trap-profiles/`. The varbind still
lands in the `TRAP_JSON` journal field with its OID, ASN.1-decoded type, and
value. If it is not a sensitive or protocol-control varbind, the plugin also
emits an indexed `TRAP_VAR_OID_<NUMERIC_OID>` journal field.

### Duplicate symbolic name handling in `TRAP_JSON` and `TRAP_VAR_*`

`TRAP_JSON` is an object keyed by varbind symbolic name. In the rare case
where two varbinds in a single trap share the same symbolic name (e.g.
two vendor MIBs each import a varbind that resolved to the same display
name), later entries use deterministic suffixes (`#2`, `#3`, ...) to avoid
duplicate JSON object keys. Example:

```json
{
  "ifDescr":            {"oid": "1.3.6.1.2.1.31.1.1.1.1",     "type": "OctetString", "value": "Gi0/1"},
  "ifDescr#2":          {"oid": "1.3.6.1.4.1.99.1.1",          "type": "OctetString", "value": "Gi0/2"}
}
```

The first occurrence wins the symbolic name; subsequent duplicates use
suffixed keys. Profile authors should avoid this by giving the conflicting
varbinds distinct symbolic names in the file-scoped `varbinds:` table; the
fallback exists only for cases where no such authoring control is possible at
extraction time or where malformed senders repeat OIDs.

Indexed journal fields use a separate collision-safe namespace. A received
symbolic varbind `ifOperStatus` emits `TRAP_VAR_IFOPERSTATUS`; if its value has
an enum label, the label is written to `TRAP_VAR_IFOPERSTATUS` and the numeric
value is written to `TRAP_VAR_IFOPERSTATUS_RAW`. Duplicate/sanitized field-name
collisions receive deterministic suffixes (`_2`, `_3`, ...). Protocol-control
varbinds that duplicate first-class fields (`sysUpTime.0`, `snmpTrapOID.0`,
`snmpTrapAddress.0`, `snmpTrapEnterprise.0`) are kept in `TRAP_JSON` but not
emitted as `TRAP_VAR_*`; the sensitive SNMP community varbind is omitted.

Generator rule: a varbind record produced by the MIB extractor that does
NOT have both a resolvable `oid` AND a `type` (MIB syntax) is dropped at
emit time — it never enters the `varbinds:` table, and any reference to it
from a trap entry's `varbinds:` list is removed in lockstep. This keeps the
shipped pack free of dangling references; description templates can only
reference varbinds that survive to disk.

### Trap entries (`traps:` list)

Each list entry defines one trap notification.

| Field         | Required | Type    | Notes |
|---------------|----------|---------|-------|
| `oid`         | yes      | string  | Numeric OID of the trap. Use the canonical OID form produced by the source MIB/tooling; the receiver tolerates the SMIv1 / SMIv2 `.0.` trap-OID ambiguity described below. |
| `name`        | yes      | string  | **MIB-qualified canonical form** `<MIB-MODULE>::<symbol>` (e.g. `IF-MIB::linkDown`, `CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged`). Globally unique — different OIDs MUST have different names. Mirrors the canonical SMI form produced by `snmptranslate`/`snmptrapd`/MIB browsers. The plugin writes this exact string to the `TRAP_NAME` journal field. |
| `category`    | yes      | string  | One of the 8 canonical categories — see below |
| `severity`    | yes      | string  | One of the 8 syslog severities — see below |
| `description` | rec.     | string  | Template rendered into the journal `MESSAGE` field |
| `status`      | no       | string  | MIB status: `current`, `deprecated`, `mandatory`, `obsolete`, or `optional`; unknown values are profile validation errors |
| `varbinds`    | no       | list    | Names referencing the file-level table, or inline dicts (see below) |
| `labels`      | no       | map     | Operator-overridable: key → template producing a `TRAP_TAG_<KEY>` journal field. Dynamic references must be bounded-cardinality. |
| `dedup_key_varbinds` | no | list | Names of varbinds that participate in the deduplication fingerprint; every name must resolve to the file-scoped `varbinds:` table |

> Note: the `name:` field encodes the source MIB module already. There is no
> separate `mib:` field on a trap entry — that would be redundant.

#### Trap OID `.0.` tolerance

SMIv1 `TRAP-TYPE` notifications use the RFC 3584 `enterprise.0.specific`
notification OID form. SMIv2 `NOTIFICATION-TYPE` notifications often use
`parent.specific` without the inserted `.0.` segment. Some MIB conversion
tools can emit either form for the same trap family.

The plugin matches trap profile entries exact-first. If exact lookup misses, it
tries one alternate trap OID by adding or removing a single `.0.` immediately
before the final OID arc. For example, a received
`1.3.6.1.4.1.14179.2.6.3.0.24` can match a profile `oid:` of
`1.3.6.1.4.1.14179.2.6.3.24`, and the reverse is also true. If both forms are
present as separate profile entries, the exact match wins.

This tolerance applies only to trap OID lookup.

Varbind OID resolution is exact-match-first. If a profile varbind OID does not
exactly match a received PDU varbind OID, it also matches received varbind OIDs
under `profile_oid + "."`. This covers SMI table cells such as `ifIndex.1` for
a profile column OID `ifIndex`, and scalar `.0` instances for profiles that use
the base scalar OID. When mapping a received PDU varbind OID back to profile
metadata, exact match wins; otherwise the longest matching profile varbind OID
prefix wins. The trap-OID `.0.` alternate spelling rule is not applied to
varbind OIDs.

When deduplication is enabled in a later SOW and a configured
`dedup_key_varbinds` varbind is absent from a received PDU, the fingerprint
uses a missing-value sentinel distinct from the empty string. Profile authors
should list only varbinds normally present on every PDU for that trap OID.

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

`description:` is a restricted Go `text/template` string substituted at render
time. Supported functions:

| Reference | Resolved to |
|---|---|
| `{{hostname}}` | Resolved device hostname from enrichment, or source IP fallback; the writer does not perform DNS lookup |
| `{{source_ip}}` | UDP source address of the trap PDU |
| `{{trap_name}}` | The trap's symbolic name |
| `{{vendor}}` | Inferred device vendor slug |
| `{{trap_interface}}` | Topology-resolved interface, when topology is co-located |
| `{{trap_neighbors}}` | Topology-resolved upstream neighbours, when topology is co-located |
| `{{value "varbindName"}}` | Varbind value, formatted per its enum (and future `display_hint`) |
| `{{raw "varbindName"}}` | Varbind raw value (numeric for enums, undecoded bytes for OctetString) |
| `{{first ...}}` | First non-empty argument, for optional-varbind fallback |

Supported control flow is limited to `{{with ...}}{{else}}{{end}}`, using the
same restricted function calls allowed for plain actions.
Known-but-absent varbinds render as an empty string, not `<missing>`, so use
`with` or `first` when optional context is included:

```yaml
description: '{{with first (value "ifDescr") (value "ifName") (value "ifIndex")}}Interface {{.}} went down{{else}}Interface went down{{end}} on {{hostname}}.'
```

Unknown functions, unknown varbind names, malformed templates, variables,
assignments, `if`, `range`, arbitrary pipelines, and template inclusion actions
fail at profile load so configuration errors are visible at job creation time.

Legacy single-brace templates from early development builds still render during
transition, but regenerated stock profiles and new operator profiles should use
the restricted Go-template syntax.

If `description:` is absent the plugin renders the default template
`"{{trap_name}} on {{hostname}}."`.

The rendered `MESSAGE` is capped at 512 bytes, including the ASCII `...`
truncation marker when truncation is needed. Multi-line MESSAGE values are
written using systemd-journal's binary field encoding so embedded newlines never
inject other journal fields.

The `status` field is informational in the MVP. The plugin does not filter,
drop, or warn on `deprecated` / `mandatory` / `obsolete` / `optional` traps in
SOW-0035; future UI or validation work may surface it.
Known `status` values are still validated so typos fail at profile load instead
of silently entering the shipped pack.

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
  content). These land in MESSAGE, indexed `TRAP_VAR_*` journal fields, and
  `TRAP_JSON` unless the varbind is sensitive or a protocol-control duplicate.
- `labels:` — **bounded-cardinality only**. Templates that reference
  unbounded varbinds (MAC, IP, username) are rejected at profile load with
  a clear error. Labels become `TRAP_TAG_<KEY>=<VALUE>` journal fields and
  propagate to metric labels.

## Operator overrides

Operators do not copy entire stock profiles to make changes. Instead they
place small override files under `/etc/netdata/go.d/snmp.trap-profiles/`.
The plugin loader mirrors the SNMP polling plugin's multipath pattern
(`src/go/plugin/go.d/collector/snmp/ddsnmp/load.go`):

1. **Same filename in higher-priority directory replaces the lower-priority
   one entirely.** Operator `ciscosystems.yaml` fully replaces stock
   `ciscosystems.yaml` or installed `ciscosystems.yaml.zst` — copy + edit the
   whole file to customize one vendor.
2. **Different filename adds entries.** Operator `site-additions.yaml`
   (different filename) merges its `traps:` into the loaded set without
   touching stock files.
3. **`extends:` chain field-merge** (when an override profile lists
   `extends: [_base.yaml, other-base.yaml]`): entries must be YAML filenames
   only, not paths. The loader merges trap
   entries by OID; fields specified in the override file win over fields
   from the extended bases; later entries in `extends:` override earlier
   ones for the same field.

### `TRAP_TAG_*` label namespace and collision-free design

Operator labels — `labels: { tenant: acme, oper_status: "{ifOperStatus}" }` —
always emit as `TRAP_TAG_<KEY_UPPERCASE>` journal fields (e.g.
`TRAP_TAG_TENANT=acme`, `TRAP_TAG_OPER_STATUS=down`). The
dedicated `TRAP_TAG_*` namespace structurally prevents collisions with
plugin-controlled `TRAP_*` fields, even when an operator label key
happens to match a plugin field name. For example, a profile with
`labels: { interface_state: "{ifOperStatus}" }` and a co-located topology plugin
both populate journal fields, but in different namespaces: the operator
label becomes `TRAP_TAG_INTERFACE_STATE`, the topology field becomes
`TRAP_INTERFACE`. Both can co-exist on the same trap entry without
conflict.

Dynamic label references must be bounded-cardinality at profile-load time. The
MVP accepts static strings, `TRAP_NAME`, `TRAP_DEVICE_VENDOR`, enum-backed
varbinds, booleans, and small numeric ranges. It rejects unbounded values such
as hostnames, source IPs, interface descriptions, MAC addresses, usernames,
packet contents, and raw numeric OID references without profile metadata.

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

Stock profiles under `default/` are generated by
`src/go/cmd/snmptrapprofilegen`, shipped as
`/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen`, from public MIB sources
and an LLM classification step. The helper can also convert operator MIBs
offline:

```sh
/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate \
  --source-dir ./mibs \
  --all \
  --out-dir ./snmp-trap-profile-gen-output
```

By default the helper reads the bundled IANA PEN snapshot from
`/usr/lib/netdata/conf.d/go.d/snmp.profiles/metadata/iana-enterprise-numbers.txt`.
Pass `--refresh-pen` to fetch the current IANA registry before emission.
The run also writes review artifacts under `--out-dir`: `traps.jsonl`,
`extraction-report.json`, `conflicts.json` for duplicate trap OIDs, and
`source-conflicts.json` when multiple MIB files define the same module name.

Edits to files under `default/` are overwritten on regeneration; operator
modifications belong in the user override directory, not in stock files.
