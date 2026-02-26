# TODO: Topology Actor Enrichment for Network Engineering UI

## 1. TL;DR

Enrich topology actor attributes and per-port `if_statuses` entries so the frontend can render a complete network-engineer device detail view. Two categories of enrichment:

1. **Static structural data** — device identity (vendor, model, serial, firmware, sys_descr, sys_location) and per-port metadata (speed, neighbors, VLANs, STP state, descriptions) that the topology carries directly.
2. **Chart reference mapping** — per-actor and per-port references to live Netdata charts so the frontend can query real-time metrics (traffic, errors, packets) at full collection granularity instead of stale 30-min topology snapshots.

The frontend is at `~/src/dashboard/netdata-cloud-frontend.git` on branch `feature/topology-flows-view`.

**Frontend consumer**: `~/src/dashboard/netdata-cloud-frontend.git/TODO-TOPOLOGY-ACTOR-MODAL.md` — the actor detail modal that will use these enriched attributes.

## 2. Motivation

The frontend topology actor detail modal needs to answer the network engineer's primary question: **"which cable do I plug/unplug to connect/disconnect X?"**

This requires:
- Device identity: what is this device? (vendor, model, serial, firmware)
- Per-port detail: every port's status, speed, mode, role, and what's connected to it
- Live metrics: real-time traffic/errors per port via chart queries (not stale snapshots)

## 3. Current State (fact-based)

### 3a. Actor attributes currently exposed

File: `src/go/pkg/topology/engine/topology_adapter.go`, function `deviceToTopologyActor()` (lines 4894-4993).

Currently exposed in actor `attributes`:
- `device_id`, `discovered`, `inferred`
- `management_ip`, `management_addresses`
- `protocols`, `protocols_collected`
- `capabilities`, `capabilities_supported`, `capabilities_enabled`
- `ports_total`
- `if_indexes`, `if_names`
- `if_admin_status_counts`, `if_oper_status_counts`
- `if_link_mode_counts`, `if_topology_role_counts`
- `if_statuses` (array of per-port objects)

Currently exposed in actor `match`:
- `sys_name`, `sys_object_id`
- `ip_addresses`, `mac_addresses`, `chassis_ids`
- `hostnames`, `netdata_machine_guid`, `netdata_node_id` exist in schema but are **not** populated by the current `deviceToTopologyActor()` path

### 3b. Per-port `if_statuses` entries currently contain

- `if_name`, `if_index`, `if_type`
- `admin_status`, `oper_status`
- `link_mode`, `link_mode_confidence`, `link_mode_sources`
- `topology_role`, `topology_role_confidence`, `topology_role_sources`
- `vlan_ids`

### 3c. Data collected but NOT exposed in actor attributes

From `topology_cache.go`, `collect.go`, and `snmputils`:
- `sys_descr` and `sys_location` are already injected into the **local** actor by `augmentLocalActorFromCache()`, not by generic actor building
- `sys_contact` is collected in `snmputils.SysInfo.Contact` and available as vnode label `contact`, but not carried in topology actor attributes
- `vendor` and `model` exist on `topologyDevice` for the local device, but are not currently projected into actor attributes
- `serial_number` / `firmware_version` / `hardware_version` may exist in SNMP profile metadata labels, but are not projected into actor attributes
- Per-port LLDP/CDP remote data is collected for link building but not summarized per-port in `if_statuses`
- Per-port FDB entries and STP entries are collected but not aggregated per-port in `if_statuses`
- Per-port speed/alias/last_change/duplex are not currently carried in topology cache interface status path

### 3d. Chart ID patterns in the SNMP collector

File: `src/go/plugin/go.d/collector/snmp/charts.go` (line 237+).

- Scalar chart ID: `snmp_device_prof_{metricName}`
- Table chart ID: `snmp_device_prof_{metricName}_{tagValues}` where `tagValues` are built from all non-empty, non-internal tags (sorted by key) via `tableMetricKey()` in `collect_snmp.go:147`
- Per-interface: tag key is `interface`; value is profile/tag-resolution dependent (often `ifName`, but can fall back to `ifDescr`)
- Example: `snmp_device_prof_ifTraffic_eth0` with dimensions `in`, `out`
- Chart ID cleaning only replaces `.` and space with `_`; `/` is not rewritten by `cleanMetricName`
- Context: `snmp.device_prof_{metricName}`

File: `src/go/plugin/go.d/collector/snmp/collect.go` (line 140+), vnode setup:
- GUID: `uuid.NewSHA1(uuid.NameSpaceDNS, []byte(hostname))` — deterministic
- Not currently propagated into `match.netdata_machine_guid` in topology actors

File: `src/go/plugin/go.d/collector/snmp/func_interfaces_cache.go` (line 14+):
- Tracked interface metrics: `ifTraffic`, `ifPacketsUcast`, `ifPacketsBroadcast`, `ifPacketsMulticast`, `ifErrors`, `ifDiscards`, `ifAdminStatus`, `ifOperStatus`

## 4. Implementation Plan

### Priority 0 — Wire already-collected data into actor attributes

P0 is split into:
- **P0-A (true wiring):** fields already present in topology observations/summary payloads
- **P0-B (light data-path extensions):** fields available elsewhere in collector state but not yet in topology observation structs

#### P0-1. Device identity attributes

Add to actor `attributes`:

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `sys_descr` | string | local cache (`topologyDevice.SysDescr`) | Already injected for local actor; make exposure rules explicit/consistent. |
| `sys_location` | string | local cache (`topologyDevice.SysLocation`) | Already injected for local actor; make exposure rules explicit/consistent. |
| `sys_contact` | string | `snmputils.SysInfo.Contact` / vnode label `contact` | Requires adding to topology cache device struct and actor projection. |
| `vendor` | string | `topologyDevice.Vendor` | Available on local topology device, not currently in actor attributes. |
| `model` | string | `topologyDevice.Model` | Available on local topology device, not currently in actor attributes. |

`sys_uptime` is moved to **P1** because it is not currently part of `snmputils.SysInfo`/topology-cache actor path.

#### P0-2. Per-port speed (moved to P1)

Add to each `if_statuses` entry:

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `speed` | integer (bps) | ifSpeed (1.3.6.1.2.1.2.2.1.5) or ifHighSpeed (1.3.6.1.2.1.31.1.1.1.15) for >1Gbps | Not currently carried in topology cache/engine interface structs; requires collector+engine data path extension. |

#### P0-3. Per-port LLDP/CDP neighbor summary

Add a `neighbors` array to each `if_statuses` entry. This data is already collected and used for link building. Expose it per-port so the frontend can show "what is connected to this specific port" without cross-referencing links.

Each neighbor object:

| Field | Type | Source |
|-------|------|--------|
| `protocol` | string | "lldp" or "cdp" |
| `remote_device` | string | lldpRemSysName / cdpCacheDeviceId |
| `remote_port` | string | lldpRemPortId+lldpRemPortDesc / cdpCacheDevicePort |
| `remote_ip` | string | lldpRemMgmtAddr / cdpCacheAddress |
| `remote_chassis_id` | string | lldpRemChassisId |
| `remote_sys_descr` | string | lldpRemSysDesc |
| `remote_capabilities` | []string | lldpRemSysCapSupported/Enabled |

Source: `topology_cache.go` LLDP remote observations (lines 1878-1917) and CDP observations (lines 1919-1958).

#### P0-4. Per-port FDB MAC count

Add to each `if_statuses` entry:

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `fdb_mac_count` | integer | Count of FDB entries per port from dot1dTpFdbPort | Already collected in `FDBObservation`. Group by interface index. |

#### P0-5. Per-port STP state

Add to each `if_statuses` entry:

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `stp_state` | string | dot1dStpPortState (1.3.6.1.2.1.17.2.15.1.3) | disabled/blocking/listening/learning/forwarding. Already in `STPPortObservation`. Summarize across VLANs (worst state wins). |
| `stp_role` | string | rstpPortRole if available | root/designated/alternate/backup. |

#### P0-6. Device-level aggregate statistics

Add to actor `attributes`:

| Field | Type | Description |
|-------|------|-------------|
| `total_bandwidth_bps` | integer | Sum of ifHighSpeed for all oper_status=up interfaces |
| `ports_up` | integer | Count of ports with oper_status=up |
| `ports_down` | integer | Count of ports with oper_status=down |
| `ports_admin_down` | integer | Count of ports with admin_status=down |
| `fdb_total_macs` | integer | Total MAC addresses in FDB |
| `vlan_count` | integer | Number of VLANs configured |
| `lldp_neighbor_count` | integer | Number of LLDP neighbors discovered |
| `cdp_neighbor_count` | integer | Number of CDP neighbors discovered |

### Priority 0 — Chart reference mapping

#### P0-7. Actor-level chart references

Add to actor `attributes`:

| Field | Type | Description |
|-------|------|-------------|
| `netdata_host_id` | string | The vnode GUID for this device. Do not assume `match.netdata_machine_guid` is populated today. |
| `chart_id_prefix` | string | The chart ID prefix for constructing chart queries. Currently `"snmp_device_prof_"`. |
| `chart_context_prefix` | string | The chart context prefix. Currently `"snmp.device_prof_"`. Frontend uses this to discover all charts via `/api/v2/contexts`. |
| `device_charts` | map[string]string | Map of device-level chart IDs that actually exist. Keys are semantic names (`ping_rtt`, `topology_devices`, etc.), values are chart IDs. Only include charts that actually exist for this device. |

#### P0-8. Per-port chart references

Add to each `if_statuses` entry:

| Field | Type | Description |
|-------|------|-------------|
| `chart_id_suffix` | string | The exact `interface` tag value used when chart keys are generated. This is profile/tag-resolution dependent (often `ifName`, fallback may be `ifDescr`). Do not assume `/` rewriting. |
| `available_metrics` | []string | List of metric names that have charts for this interface. Example: `["ifTraffic", "ifErrors", "ifDiscards", "ifPacketsUcast", "ifAdminStatus", "ifOperStatus"]`. Only include metrics where the collector actually created a chart. |

With these fields, the frontend constructs per-interface chart IDs as:
```
{chart_id_prefix}{metricName}_{chart_id_suffix}
```
Example: `snmp_device_prof_ifTraffic_eth0` with dimensions `in`, `out`.

### Priority 1 — Add collection + wire

These require new SNMP walks or new parsing, then wiring into attributes.

#### P1-1. Additional device identity

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `sys_uptime` | integer | system uptime metric / sysUpTime where available | Add explicit collection-to-topology mapping for actor attributes. |
| `serial_number` | string | entPhysicalSerialNum (1.3.6.1.2.1.47.1.1.1.1.11) or device labels from SNMP profiles | Extract from device labels if collected via profiles. |
| `firmware_version` | string | entPhysicalSoftwareRev (1.3.6.1.2.1.47.1.1.1.1.10) or sysDescr parsing or device labels | Extract from device labels if collected via profiles. |
| `hardware_version` | string | entPhysicalHardwareRev (1.3.6.1.2.1.47.1.1.1.1.8) or device labels | Extract from device labels if collected via profiles. |

#### P1-2. Additional per-port enrichment

Add to each `if_statuses` entry:

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `speed` | integer (bps) | ifSpeed/ifHighSpeed | Requires extending topology cache interface status + engine interface labels/fields. |
| `if_descr` | string | ifDescr (1.3.6.1.2.1.2.2.1.2) | Interface description (hardware-level). |
| `if_alias` | string | ifAlias (1.3.6.1.2.1.31.1.1.1.18) | User-configurable port label. Critical — network engineers use this to document what's connected. |
| `mac` | string | ifPhysAddress (1.3.6.1.2.1.2.2.1.6) | The interface's own MAC address. |
| `last_change` | integer | ifLastChange (1.3.6.1.2.1.2.2.1.9) | When the port last changed state (timeticks). Key for "when did this go down?" |
| `duplex` | string | dot3StatsDuplexStatus (1.3.6.1.2.1.10.7.2.1.19) | half/full/unknown. |
| `vlans` | []object | Derived from FDB/STP VLAN data per port | Each: `{ vlan_id, vlan_name, tagged }`. For trunk ports = allowed VLANs. For access ports = single access VLAN. |

#### P1-3. Vendor inference for non-SNMP/unmanaged actors using MAC prefix (OUI)

Goal:
- Infer `vendor` for actors/endpoints that do not have SNMP identity metadata, using normalized MAC address prefix lookup.

Scope:
- Apply only when `vendor` is missing.
- Use canonical MAC identities already available in topology (`match.mac_addresses`, chassis MAC, endpoint MAC).
- Mark inferred provenance explicitly (e.g., `vendor_source: "mac_oui"` and `vendor_confidence: "low"` or similar).

Constraints:
- OUI-based vendor is heuristic only; resold NICs/virtual interfaces can produce wrong vendor.
- Keep SNMP-derived vendor as authoritative when present; OUI must never override explicit SNMP vendor.

Data source options are documented in **D4**.

### Priority 2 — Nice to have

#### P2-1. PoE data per port

| Field | Type | Source |
|-------|------|--------|
| `poe_status` | string | pethPsePortDetectionStatus (1.3.6.1.2.1.105.1.1.1.6) |
| `poe_power_mw` | integer | pethPsePortActualPower (1.3.6.1.2.1.105.1.1.1.7) |

## 5. Implementation Notes

1. **All new fields are optional.** If a device doesn't support the relevant MIBs, omit the field entirely (don't send empty/zero). The frontend handles missing fields gracefully.

2. **Use snake_case consistently.** The frontend reads both `snake_case` and `camelCase` variants of every field (responses get camelized in some code paths). Backend should use `snake_case` — the frontend handles both.

3. **Per-port data goes into the existing `if_statuses` array entries.** Device-level data goes into the actor `attributes` object.

4. **Chart reference `chart_id_suffix` must match exactly** the collector tag value used in `tableMetricKey()` for `interface` (profile-dependent). It is often `ifName`, but can be `ifDescr` fallback. Do not assume slash sanitization.

5. **`device_charts` should be populated by checking which charts the collector actually created** for this device. Don't assume all devices have ping, topology, CPU, etc.

6. **`available_metrics` per port should be populated from the collector's actual interface metric tracking**, not assumed. The collector tracks `ifTraffic`, `ifErrors`, `ifDiscards`, `ifPacketsUcast`, `ifPacketsBroadcast`, `ifPacketsMulticast`, `ifAdminStatus`, `ifOperStatus` — but only for interfaces it actually creates charts for.

## 6. Why chart references instead of metric snapshots

The topology refreshes every ~30 minutes (discovery cycle). Carrying metric values (CPU%, traffic bps, temperature) in topology attributes means the frontend shows 30-minute-stale data.

With chart references, the frontend:
- Embeds live sparkline charts in the port table (real-time, 1-10s granularity)
- Queries historical data for any time range
- Gets alerting integration (charts have alarm contexts)
- Shows real-time traffic/errors per port
- Links directly to full Netdata dashboard charts for deep-dive
- Wastes no bandwidth carrying metric values in topology payloads

The topology carries **structure** (what exists, how it's connected). Metrics come from the Netdata monitoring pipeline at full fidelity.

## 7. Frontend usage

The frontend will use these fields to build an actor detail modal with:

1. **Device summary card** — vendor, model, serial, firmware, sys_descr, location, capabilities, protocols, management IPs
2. **Per-port table** — every port with status, speed, mode, role, neighbor, STP state, VLANs, FDB count
3. **Expandable port rows** — clicking a port row expands to show all links on that port + live charts (traffic, errors, packets) queried via chart references
4. **Mini topology graph** — the actor in isolation with depth=1 neighbors, for spatial context
5. **Flows drilldown tab** — embedded flows function filtered to the actor's IPs

## 8. Testing requirements

- Unit tests for each new attribute added to `deviceToTopologyActor()`
- Unit tests for per-port neighbor summary construction
- Unit tests for chart reference mapping (verify `chart_id_suffix` matches actual collector `interface` tag resolution, including fallback behavior)
- Unit tests for OUI vendor inference (hit/miss/invalid MAC/no-override when SNMP vendor exists)
- Integration tests with real SNMP device data (use existing LibreNMS snmprec fixtures)
- Verify that existing topology tests still pass after enrichment
- Verify that topology payload size remains reasonable (no accidental explosion from per-port arrays on 96-port switches)

## 9. Priority order

1. **P0 first**: wire fields already available in topology observations + add chart references with exact collector key semantics.
2. **P1 second**: extend collector/topology data path for `speed`, `if_alias`, `if_descr`, `sys_uptime`, `serial_number`, `firmware_version`, `vlans`, `duplex`, `last_change`, `mac`, and OUI vendor inference for unmanaged actors.
3. **P2 later**: PoE.

## 10. Key source files

| File | Purpose |
|------|---------|
| `src/go/pkg/topology/engine/topology_adapter.go` | Actor attribute building (`deviceToTopologyActor()` at line 4894) |
| `src/go/pkg/topology/types.go` | Actor/Link/Observation type definitions |
| `src/go/plugin/go.d/collector/snmp/topology_cache.go` | SNMP data collection (LLDP, CDP, STP, FDB, ARP) |
| `src/go/plugin/go.d/collector/snmp/collect.go` | Vnode setup, GUID generation (line 140) |
| `src/go/plugin/go.d/collector/snmp/charts.go` | Chart ID creation patterns (line 237) |
| `src/go/plugin/go.d/collector/snmp/collect_snmp.go` | `tableMetricKey()` function (line 147) |
| `src/go/plugin/go.d/collector/snmp/func_interfaces_cache.go` | Interface metric name tracking (line 14) |
| `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/collector_device_meta.go` | Device metadata from SNMP profiles |
| `src/go/plugin/go.d/pkg/snmputils/sysinfo.go` | Existing vendor derivation path from sysObjectID/PEN (authoritative SNMP identity) |
| `src/go/plugin/go.d/pkg/snmputils/overrides.go` | Existing override loading framework, candidate place for configurable OUI aliasing |
| `src/go/pkg/topology/engine/types.go` | L2Observation, ObservedInterface, LLDP/CDP/STP/FDB types |

## 11. Pending decisions (must be confirmed before implementation)

### D1. Device metadata normalization for `serial_number` / `firmware_version` / `hardware_version`

Context:
- SNMP profile metadata keys are arbitrary field names from profile definitions and are merged as labels.
- There is no single enforced key set for firmware/hardware naming across all profiles.

Evidence:
- `collector_device_meta.go` processes metadata as `map[string]MetadataField` keys (`name` is the emitted key).
- `collect.go` merges all metadata keys into vnode labels.

Options:
- **A**: Strict keys only (`serial_number`, `firmware_version`, `hardware_version`)
  - Pros: predictable schema
  - Cons: misses devices using different key names
  - Risk: low coverage in real fleets
- **B**: Alias normalization map (preferred)
  - Example aliases: `firmware`, `software_version`, `sw_version` -> `firmware_version`
  - Pros: better real-world coverage, stable output schema
  - Cons: requires maintaining alias list
  - Risk: potential ambiguous alias mapping in edge profiles
- **C**: No normalization; expose only raw labels
  - Pros: no assumptions
  - Cons: frontend complexity and inconsistent UX
  - Risk: weak contract

Recommendation: **B**

### D2. Scope of enriched identity fields (`sys_descr`, `sys_location`, `sys_contact`, `vendor`, `model`)

Context:
- Current enrichment path injects local-device fields into the local actor after topology generation.
- Remote/synthetic actors frequently lack these fields.

Evidence:
- `augmentLocalActorFromCache()` enriches only the matched local actor.

Options:
- **A**: Local actor only (preferred for now)
  - Pros: accurate provenance, no synthetic data
  - Cons: heterogeneous actor richness
  - Risk: frontend must handle missing fields for non-local actors
- **B**: Attempt to project these fields onto all actors when available in labels
  - Pros: potentially richer graph
  - Cons: sparse/inconsistent; may project stale or inferred data
  - Risk: data quality ambiguity

Recommendation: **A**

### D3. `netdata_host_id` and `device_charts` semantics for non-local actors

Context:
- Vnode GUID is deterministic for the local collector target.
- Topology actor `match.netdata_machine_guid` is not currently populated in this path.
- Exact "charts that actually exist" inventory is not currently carried by topology snapshot.

Evidence:
- `collect.go` computes vnode GUID.
- `topology_adapter.go` does not populate `match.netdata_machine_guid`.
- Topology snapshot path does not expose a chart inventory structure.

Options:
- **A**: Populate `netdata_host_id` and `device_charts` only when explicitly known (preferred)
  - Pros: correctness-first, no fake IDs
  - Cons: partial coverage across actors
  - Risk: frontend must handle missing mappings
- **B**: Derive synthetic IDs/charts for non-local actors
  - Pros: more complete-looking payload
  - Cons: can be wrong
  - Risk: broken chart links / misleading UX
- **C**: Add collector-side chart inventory cache and propagate it into topology payload
  - Pros: highest fidelity
  - Cons: larger implementation scope (collector + topology cache + adapter)
  - Risk: delay and payload growth

Recommendation: **A** for P0; consider **C** as follow-up if needed.

### D4. Source of truth for MAC OUI -> vendor lookup

Context:
- There is no existing OUI lookup path in topology/snmp code.
- Existing `enterprise-numbers.txt` is IANA PEN mapping for `sysObjectID`, not IEEE OUI mapping.

Evidence:
- Vendor derivation today uses `lookupEnterpriseNumber(sysObject)` in `snmputils/sysinfo.go`.
- No OUI/manuf dataset is currently present in repository code paths for topology enrichment.

Options:
- **A**: Small built-in OUI table for known environments
  - Pros: fast implementation, minimal dependency surface
  - Cons: low coverage, manual maintenance
  - Risk: frequent unknowns
- **B**: Versioned embedded OUI dataset (recommended)
  - Pros: high coverage, deterministic runtime behavior, no network dependency
  - Cons: larger repo footprint, needs periodic refresh process
  - Risk: stale data over time
- **C**: Runtime external OUI service/file
  - Pros: freshest data
  - Cons: runtime dependency, offline fragility, harder reproducibility
  - Risk: operational failures break enrichment

Recommendation: **B**

## 12. Decisions made by user

Date: 2026-02-26

1. D1 metadata normalization: **B** (alias normalization map)
2. D2 identity scope: **A** (local actor only for now)
3. D3 host/charts semantics: **A** (only when explicitly known)
4. D4 OUI vendor source: **B** (embedded versioned OUI dataset)

## 13. Implementation progress

Date: 2026-02-26

- [x] P0-3 per-port LLDP/CDP neighbor summary in `if_statuses[].neighbors` (protocol/device/port/ip/chassis/capabilities)
- [x] P0-4 per-port `fdb_mac_count`
- [x] P0-5 per-port `stp_state` (worst-state summarization across observed STP rows)
- [x] P0-6 aggregate counters in actor attributes:
  - `ports_up`, `ports_down`, `ports_admin_down`
  - `fdb_total_macs`, `vlan_count`
  - `lldp_neighbor_count`, `cdp_neighbor_count`
- [ ] P0-6 `total_bandwidth_bps` (depends on speed wiring from P1-2)
- [ ] P0-1 identity enrichment (`sys_contact`, `vendor`, `model` local exposure path)
- [ ] P0-7 / P0-8 chart reference mapping
- [ ] P1-1 / P1-2 additional identity and per-port collector-path extensions
- [ ] P1-3 OUI vendor inference (D4=B source decision finalized)
