# PAN-OS go.d Collector

## Scope

The `panos` collector monitors Palo Alto Networks PAN-OS firewalls through the PAN-OS XML API.

Version 1 ships the read-only telemetry metricsets with fixture-backed XML response shapes:

- BGP
- system
- HA
- environment
- licenses
- IPsec

Future PAN-OS metricsets must stay in the same `panos` collector and use `panos.<area>.*` chart contexts.

The collector is a metrics collector, not a generic PAN-OS XML API passthrough. It must not expose arbitrary operator-supplied XML commands, configuration changes, commits, imports, exports, log retrieval, report retrieval, or user-id operations.

## go.d Runtime Contract

- The collector is a go.d CollectorV2 module.
- Runtime chart definitions live in `src/go/plugin/go.d/collector/panos/charts.yaml`.
- The chart template must compile with the go.d chart template engine.
- Dynamic per-entity chart templates use `expire_after_cycles: 3`, matching the collector's stale-entity grace period.
- Collection preserves partial-success semantics: when at least one enabled metricset emits metrics, CollectorV2 `Collect()` writes those metrics to the metrix store and returns success so the runtime does not abort the cycle. Full failures with no metrics return an error.

## Transport And Auth

- The collector uses `github.com/PaloAltoNetworks/pango` for PAN-OS XML API access.
- Supported auth modes:
  - `api_key`
  - `username` plus `password`, with pango key generation and in-memory key reuse
- When username/password are configured, the collector retries one failed op request after refreshing the API key if PAN-OS reports unauthorized/session-expired.
- The configured URL must point at the firewall management interface; accepted paths are empty, `/`, and `/api`.
- URLs with embedded credentials are rejected. Use `api_key`, or `username` plus `password`, instead.
- Panorama target proxy mode is not part of v1.
- The collector logs PAN-OS system information once after successful API initialization when the device reports it through pango.
- Supported metricsets can be enabled or disabled per job. All v1 metricsets are enabled by default.
- Enabled metricsets are collected serially. If one metricset fails, the collector logs/returns metricset and command context while preserving metrics from successful metricsets. A job check succeeds when at least one enabled metricset returns metrics.

## Metricsets And Commands

The collector uses these read-only operational commands:

| Metricset | XML API command |
|---|---|
| BGP legacy VR | `<show><routing><protocol><bgp><peer></peer></bgp></protocol></routing></show>` |
| BGP Advanced Routing Engine | `<show><advanced-routing><bgp><peer>...</peer></bgp></advanced-routing></show>` |
| System | `<show><system><info></info></system></show>` |
| HA | `<show><high-availability><state></state></high-availability></show>` |
| Environment | `<show><system><environmentals></environmentals></system></show>` |
| Licenses | `<request><license><info></info></license></request>` |
| IPsec | `<show><vpn><ipsec-sa></ipsec-sa></vpn></show>` |

## BGP Metricset

The BGP metricset supports legacy Virtual Router and Advanced Routing Engine deployments.

The collector probes the legacy command first:

```xml
<show><routing><protocol><bgp><peer></peer></bgp></protocol></routing></show>
```

If no legacy BGP peers are found, it probes Advanced Routing Engine peer commands under:

```xml
<show><advanced-routing><bgp><peer>...</peer></bgp></advanced-routing></show>
```

The successful command is cached for later collections. If the cached command fails, probing is retried.

If all legacy and Advanced Routing Engine probes return successful empty results, the collector records an explicit no-BGP state and suppresses full re-probing for 5 minutes. This avoids repeatedly sending all probe commands to firewalls that do not have BGP configured.

## Chart Contexts

### System

System contexts use device-level labels when known: `hostname`, `model`, `serial`, and `sw_version`.

- `panos.system.uptime`
- `panos.system.device_certificate_status`
- `panos.system.operational_mode`

### HA

HA contexts describe the local firewall and its HA peer as reported by PAN-OS.

- `panos.ha.enabled`
- `panos.ha.local.state`
- `panos.ha.peer.state`
- `panos.ha.peer.connection_status`
- `panos.ha.state_sync`
- `panos.ha.links_status`
- `panos.ha.priority`

### Environment

Environment contexts use labels `slot`, `sensor`, and `sensor_type`.

- `panos.environment.sensors.collection`
- `panos.environment.temperature`
- `panos.environment.fan_speed`
- `panos.environment.voltage`
- `panos.environment.sensor_alarm`
- `panos.environment.power_supply_status`

### Licenses

- `panos.license.count`
- `panos.license.collection`

Per-license contexts use labels `feature` and `description`. `panos.license.time_until_expiration` uses `-1` only for licenses where PAN-OS reports `Never`. Unrecognized expiration dates are reported as collection errors with the license name and raw value; the collector must not convert malformed dates into a fake never-expiring value.

- `panos.license.status`
- `panos.license.time_until_expiration`

### IPsec

- `panos.ipsec.tunnels`
- `panos.ipsec.tunnels.collection`

Per-tunnel contexts use labels `tunnel`, `gateway`, `remote`, `tunnel_id`, `protocol`, and `encryption`.

- `panos.ipsec.tunnel.sa_lifetime`

### BGP

Collection/cardinality contexts are global:

- `panos.bgp.peers.collection`
- `panos.bgp.prefix_families.collection`
- `panos.bgp.virtual_routers.collection`

Per-peer contexts use labels `vr`, `peer_address`, `local_address`, `remote_as`, and `peer_group`.

- `panos.bgp.peer.state`
- `panos.bgp.peer.uptime`
- `panos.bgp.peer.messages`
- `panos.bgp.peer.updates`
- `panos.bgp.peer.flaps`
- `panos.bgp.peer.established_transitions`

Per-peer-per-family contexts add labels `afi` and `safi`.

- `panos.bgp.peer.prefixes_received`
- `panos.bgp.peer.prefixes_advertised`

Per-virtual-router contexts use label `vr`.

- `panos.bgp.vr.peers_by_state`
- `panos.bgp.vr.peers_total`

## Defaults And Limits

- Default `update_every`: 60 seconds.
- Default timeout: 10 seconds.
- The v1 collector issues PAN-OS XML API requests serially and caps the SDK transport at 2 connections per firewall job.
- Default per-entity chart caps:
  - `max_bgp_peers`: 512
  - `max_bgp_prefix_families_per_peer`: 4
  - `max_bgp_virtual_routers`: 256
  - `max_environment_sensors`: 512
  - `max_licenses`: 64
  - `max_ipsec_tunnels`: 1024
- Every capped entity family has a matching selector:
  - `bgp_peers` matches `vr/peer_address`.
  - `bgp_prefix_families` matches `vr/peer_address/afi/safi`.
  - `bgp_virtual_routers` matches the virtual-router or logical-router name.
  - `environment_sensors` matches `sensor_type/slot/sensor`.
  - `licenses` matches the license feature name.
  - `ipsec_tunnels` matches `name/gateway/remote/tunnel_id`.
- Caps do not fail collection. The collector emits collection/cardinality metrics with `discovered`, `monitored`, `omitted_by_selector`, and `omitted_by_limit`; it logs cap hits once per changed condition.
- Summary metrics remain complete when possible: IPsec active tunnel count is emitted from `<ntun>` when PAN-OS reports it, license total/expired counts are computed from all parsed licenses, and per-entity charts are capped/selected.
- If PAN-OS reports an IPsec `<ntun>` count that differs from returned `<entry>` elements, the collector must keep the summary count and return/log an explicit mismatch error because per-tunnel lifetime charts may be incomplete.
- Obsoletion is independent of cardinality controls. Charts for entities that disappear, become excluded, or fall beyond a cap are removed only after the stale-chart grace period.
- Dynamic peer, prefix, and virtual-router charts are removed only after 3 consecutive missing collections to avoid chart churn during transient empty responses.
- Enabled metricsets must report unexpected empty/unknown successful XML responses with the metricset, command context, and expected XML section instead of failing silently.
- Missing or malformed numeric, decimal, duration, boolean/status, and license date values that back emitted charts must be reported with the field/entity name and raw value when present instead of being converted to zero or fake valid state.
- The collector intentionally does not emit charts for BGP data PAN-OS XML API does not expose, including separate withdraw/notification/keepalive/route-refresh counters, RPKI route correctness, EVPN VNI metrics, and detailed reset cause history.

## Out Of Scope For v1

- Panorama `target` proxy mode.
- GlobalProtect, certificate inventory, sessions, interfaces, dataplane CPU, and arbitrary XML command passthrough.
- PAN-OS config, commit, import, export, log, report, user-id, restart, and upgrade operations.
- Shared code or chart-context coupling with the generic `bgp` collector.
- SNMP profile changes.
