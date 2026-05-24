# Netdata's Existing SNMP Support — Inventory and Analysis

Purpose: provide the working context for the SNMP-trap design discussion. What does Netdata already have? Which components are mature? Which would be reusable for trap support? Where would a trap subsystem plug in?

**Important boundary**: This is the *current state of polling-based SNMP* in this repo. Netdata has **no SNMP trap reception today** — `find src/go -iname '*trap*' -path '*snmp*'` returns zero hits, and `grep -r 'snmptrap\|InformRequest\|TrapPDU'` returns zero hits in Go source. This document inventories what exists so we can later judge what to reuse, what to clone-and-adapt, and what must be built fresh.

Repository: `netdata/netdata @ 6a515000ac` (branch `snmptraps`, this working tree).

---

## 1. Module Inventory

Two Go modules under `src/go/plugin/go.d/collector/` directly implement SNMP. Both are framework-V1 collectors registered through `collectorapi.Register`.

| Module | Path | Purpose | LOC focus | Update interval |
|---|---|---|---|---|
| `snmp` | `src/go/plugin/go.d/collector/snmp/` | Per-device metric collection from a single SNMP target | `collect_snmp.go`, `collector.go`, `ddsnmp/` subtree | 10s default |
| `snmp_topology` | `src/go/plugin/go.d/collector/snmp_topology/` | Network topology discovery (LLDP / CDP / FDB / ARP / STP / VLANs) | `topology_cache_*.go` (16 files), `func_topology*.go` (12 files) | 60s collect / 30m refresh |

Plus the discovery side:

| Discoverer | Path | Purpose |
|---|---|---|
| `snmpsd` | `src/go/plugin/go.d/discovery/sdext/discoverer/snmpsd/` | Scans IP ranges for SNMP-enabled devices and emits SD targets that the `snmp` collector consumes (defaults: 30-minute rescan, 32 parallel scans per network, 1s SNMP timeout) |

Shared utility package:

| Package | Path | Purpose |
|---|---|---|
| `snmputils` | `src/go/plugin/go.d/pkg/snmputils/` | `sysuptime.go`, `sysinfo.go`, `overrides.go`, `utils.go`, plus a static `enterprise-numbers.txt` of vendor enterprise OIDs |

The third major piece is **`ddsnmp/`** — a Netdata-internal fork/port of Datadog's SNMP profile engine. It lives inside `collector/snmp/ddsnmp/` but is shared between the `snmp` and `snmp_topology` collectors:

```
src/go/plugin/go.d/collector/snmp/ddsnmp/
├── ddprofiledefinition/       <- YAML schema for SNMP profiles (Datadog-compatible)
│   ├── profile_definition.go  <- root struct: Selector, Extends, Metadata, Metrics, Topology, Licensing, BGP, MetricTags, ...
│   ├── topology.go            <- 17 TopologyKind enums (lldp_rem, cdp_cache, fdb_entry, arp_entry, stp_port, ...)
│   ├── metrics.go, bgp.go, licensing.go, virtual_metrics.go, selector.go, ...
├── ddsnmpcollector/           <- the actual collector that walks a device using a profile
│   ├── collector.go, collector_scalar.go, collector_table.go, collector_topology.go, collector_bgp.go, ...
├── profile_catalog.go         <- discovers and indexes bundled profiles
├── load.go                    <- YAML parser, "extends" resolution
├── profile.go, profile_filter.go, profile_prepare.go, profile_merge_test.go
├── device_registry.go         <- maps sysObjectID and sysDescr to a profile
└── topology_kind.go, topology_provider.go, transform.go, ...
```

`ddsnmp` is the heart of the system. It is licensed Apache-2.0 because it derives from upstream Datadog (`profile_definition.go:3-4`).

---

## 2. SNMP Protocol Stack

- **Library**: `github.com/gosnmp/gosnmp v1.42.1` — Pure-Go SNMP v1/v2c/v3 client
- **Pinned to a fork**: `src/go/go.mod` line `replace github.com/gosnmp/gosnmp => github.com/ilyam8/gosnmp v0.0.0-20250912202722-388b2cb5192e`
- **Versions supported**: SNMPv1, v2c, v3 (USM with auth+priv). Config enum `"1" | "2c" | "3"` in `collector/snmp/config_schema.json`.
- **Transports**: UDP (gosnmp default). No TLSTM / DTLS support — confirmed by absence of references in `pkg/snmputils/` and the gosnmp fork README.
- **Identity**: `snmputils.GetSysInfo()` (`pkg/snmputils/sysinfo.go`) fetches `sysObjectID`, `sysDescr`, `sysName`, `sysContact`, `sysLocation`, `sysServices`, used as inputs to profile selection.
- **Concurrency**: per-device walks are serialized inside one job; multi-device parallelism is achieved by configuring multiple jobs. `errgroup.WithContext` parallelises SNMP vs ping fetches within a single device cycle (`collect_snmp.go`).

What's notable in the gosnmp fork: it's maintained by Ilya Mashchenko (Netdata engineer). The fork is referenced from a single `replace` line — would be worth confirming what patches the fork carries (out of scope for this doc).

---

## 3. Profile System (the load-bearing piece)

This is where Netdata's SNMP support outclasses most polling-only collectors and is the most directly reusable asset for trap support.

### 3.1 Bundled profile inventory

```
src/go/plugin/go.d/config/go.d/snmp.profiles/
├── default/                   <- 317 YAML files (find ... -name '*.yaml' | wc -l)
│   ├── 3com.yaml, _arista-bgp4-v2.yaml, _arista-metadata.yaml, arista-switch.yaml,
│   ├── aruba-*.yaml, cisco-asa.yaml, cisco-catalyst.yaml, cisco-nexus.yaml,
│   ├── fortinet-fortigate.yaml, juniper-junos.yaml, palo-alto.yaml, apc-ups.yaml,
│   └── ... 300+ vendor and model profiles, plus shared "_*" mixins
└── metadata/                  <- per-vendor metadata YAMLs (3com, cisco, dell, hp, juniper, ...)
```

Profiles use Datadog's YAML schema as defined in `ddprofiledefinition/profile_definition.go`:

```go
type ProfileDefinition struct {
    Selector            SelectorSpec                     // sysObjectID / sysDescr match
    Extends             []string                         // mixin composition
    Metadata            MetadataConfig                   // device-level static metadata
    SysobjectIDMetadata []SysobjectIDMetadataEntryConfig
    Metrics             []MetricsConfig                  // OIDs to walk, scalar or table
    Topology            []TopologyConfig                 // LLDP/CDP/FDB/ARP/STP table OIDs
    Licensing           []LicensingConfig                // license-state OIDs (Cisco, Fortinet, Sophos, ...)
    BGP                 []BGPConfig                      // BGP4-MIB typed projection
    MetricTags          []GlobalMetricTagConfig
    StaticTags          []StaticMetricTagConfig
    VirtualMetrics      []VirtualMetricConfig
    SysObjectIDs        StringArray                      // deprecated
}
```

### 3.2 What a profile encodes

For each device family, a profile lists:

- Which sysObjectIDs and sysDescr patterns identify it
- Which **scalar** and **table** OIDs to walk for metrics
- How to **tag** rows from index OIDs
- How to compute **virtual metrics** (derivations across raw OIDs)
- Which OIDs are **topology-relevant**, classified by `TopologyKind` (17 kinds, see §4)
- License-state and BGP-typed projections (Netdata extensions beyond stock Datadog)

### 3.3 Profile authoring rules

Project skill: `.agents/skills/project-snmp-profiles-authoring/` documents the rules. Crucial rule: MIB `MAX-ACCESS` `not-accessible` INDEX objects must be derived from the row index, not requested directly. This is captured because earlier work tripped on it.

Recent SOWs (`.agents/sow/done/SOW-0012-20260506-snmp-profile-projection.md`, `SOW-0013-20260507-snmp-licensing-projection.md`, `SOW-0015-20260508-snmp-bgp-typed-projection.md`) show active investment in profile-driven typed projections for licensing and BGP.

### 3.4 Profile-driven typing for traps?

Profiles already carry typed semantics for `Topology`, `Licensing`, `BGP`, and `VirtualMetrics`. They do not currently encode `Trap` semantics (no `Traps` field in `ProfileDefinition`). Adding a `Traps` config block would let trap OIDs be:

- Resolved to symbolic names (already half-done — profiles map OID names)
- Tagged by device profile (already done by `device_registry.go`)
- Routed into the same `ddsnmp.Profile` typed projection layer as licensing / BGP

This is a major reusable asset — but only if traps fit the profile model cleanly. Vendor traps may need MIB compilation, which the current profile system does NOT do (profiles are hand-maintained YAML, not auto-derived from MIB files).

---

## 4. Topology Engine

Distinct from the topology *cache* in `snmp_topology` — there is a generic topology engine at `src/go/pkg/topology/engine/`:

- `engine.go`, `node_topology.go`, `runtime_test.go`
- L2 pipeline: `l2_pipeline_adjacencies.go`, `l2_pipeline_fdb_test.go`, `l2_pipeline_normalization_test.go`
- Adapters: `topology_adapter_builder.go`, `topology_adapter_device_summary.go`, `topology_adapter_bridge_links.go`, `topology_adapter_bridge_ports_test.go`, `topology_adapter_display.go`, `topology_adapter_pruning.go`, `topology_adapter_segment_hints.go`, `topology_adapter_identity_assignment.go`, `topology_adapter_projection_pairs.go`, `topology_adapter_regression_test.go`
- MAC OUI lookup: `mac_oui_lookup.go`

This engine is fed by the per-device topology cache in `snmp_topology/topology_cache.go` and produces the `netdata.topology.v1` payload that the UI consumes. Fixture: `src/go/tools/functions-validation/fixtures/topology-v1/snmp-l2.json` shows the wire shape.

### 4.1 Per-device topology cache

`src/go/plugin/go.d/collector/snmp_topology/topology_cache.go:11-38` defines the state held per device:

```go
type topologyCache struct {
    mu           sync.RWMutex
    lastUpdate   time.Time
    updateTime   time.Time
    staleAfter   time.Duration

    agentID      string
    localDevice  topologyDevice

    lldpLocPorts map[string]*lldpLocPort     // local-side LLDP ports
    lldpRemotes  map[string]*lldpRemote      // remote neighbors via LLDP
    cdpRemotes   map[string]*cdpRemote       // remote neighbors via CDP

    ifNamesByIndex      map[string]string
    ifStatusByIndex     map[string]ifStatus  // admin/oper/type/mac/speed/lastChange/duplex
    ifIndexByIP         map[string]string
    ifNetmaskByIP       map[string]string
    bridgePortToIf      map[string]string
    fdbEntries          map[string]*fdbEntry
    fdbIDToVlanID       map[string]string
    vlanIDToName        map[string]string
    fdbRowsDroppedNoMAC int
    fdbRowsUnmappedPort int
    vtpVersion          string
    stpBaseBridgeAddress string
    stpDesignatedRoot   string
    stpPorts            map[string]*stpPortEntry
    arpEntries          map[string]*arpEntry
}
```

The cache is updated by `topology_cache_ingest.go:updateTopologyCacheEntry` based on the `TopologyKind` of each profile-tagged metric. Stale entries are pruned by `topology_cache_lifecycle.go`.

### 4.2 TopologyKind enumeration

17 kinds, in `ddprofiledefinition/topology.go:6-26`:

```
KindLldpLocPort          | KindLldpLocManAddr
KindLldpRem              | KindLldpRemManAddr | KindLldpRemManAddrCompat
KindCdpCache
KindIfName | KindIfStatus | KindIfDuplex
KindIpIfIndex
KindBridgePortIfIndex
KindFdbEntry | KindQbridgeFdbEntry | KindQbridgeVlanEntry
KindStpPort
KindVtpVlan
KindArpEntry | KindArpLegacyEntry
```

A trap subsystem that wanted to update topology on `linkDown` / `linkUp` would write into `ifStatusByIndex` and emit a topology delta event. This is reusable.

---

## 5. Configuration Model

### 5.1 Per-job device configuration (`snmp` module)

Stock config (`src/go/plugin/go.d/config/go.d/snmp.conf`) is minimal — a commented example:

```
#jobs:
#  - name: switch
#    update_every: 10
#    hostname: "192.0.2.1"
#    community: public
#    options:
#      version: 2
```

Full JSON Schema in `collector/snmp/config_schema.json` (383 lines). Top-level fields:

- `update_every` (default 10s)
- `hostname` (required IP or DNS name)
- `community` (default `public`) — v1/v2c
- `create_vnode` (default `true`) — see §6
- `vnode_device_down_threshold` (default 3 failed polls)
- `vnode.guid` / `vnode.hostname` / `vnode.labels`
- `options.version` (enum `1` | `2c` | `3`)
- `options.user`, `options.auth_protocol`, `options.auth_passphrase`, `options.priv_protocol`, `options.priv_passphrase`, `options.context_name` — for SNMPv3 USM
- `options.timeout`, `options.retries`, `options.port` (default 161), `options.max_repetitions` (BULK)
- Probably also `options.network_interface_filter` (per stock metadata.yaml keywords)
- `ping.enabled` / `ping.privileged` / `ping.packets` — ICMP RTT alongside SNMP

### 5.2 SD-driven config (`snmpsd`)

`snmpsd` scans CIDR ranges and emits SD targets. Each target eventually becomes an `snmp` job via the SD pipeline. Persisted via `filepersister`. Default rescan = 30 min, parallel scans per network = 32. Hash-tracked so unchanged config skips rescan.

### 5.3 Topology config (`snmp_topology`)

Stock config is a singleton (`snmp_topology.conf`):

```yaml
jobs:
  - name: snmp_topology
    update_every: 60
    refresh_every: 30m
```

Devices to discover are drawn automatically from running `snmp` jobs — no per-device topology config. Topology is on-by-default. This is the inverse of the data flow in most NMS: usually you declare devices to topology, here topology learns from the collectors.

---

## 6. Virtual Nodes (vnodes)

Every monitored SNMP device shows up in Netdata as a **virtual node** (`create_vnode: true` is the default).

- Framework lives at `src/go/plugin/framework/vnodes/`
- Schema: `vnodes/config_schema.json`
- Each vnode gets: `guid` (auto-derived from IP if not set), `hostname`, free-form `labels`
- The vnode is reported by the agent alongside its own host, and treated as a peer in the UI tree
- `vnode_device_down_threshold` controls when a device transitions to a "down" vnode state

**Implication for traps**: an SNMP trap arrives with a source IP and (in v2c+) a `snmpTrapOID.0`. Source-IP-to-vnode mapping is already implemented for the metric path (`collector/snmp/init.go` + `vnodes` package). A trap receiver could re-use this mapping to attribute incoming UDP/162 packets to the right vnode.

---

## 7. Functions (Interactive Operator Queries)

Netdata has a Functions framework — an operator can call a function on a node and get a structured response, used to drive interactive UI tabs.

The SNMP module exposes these functions today (file naming convention `func_*.go` in `collector/snmp/`):

| Function file | What it exposes |
|---|---|
| `func_router.go` | Per-device summary (router/switch facts) |
| `func_interfaces.go` (+ `_cache.go`) | Interface table with status, errors, speed, MAC, etc. |
| `func_bgp_peers.go` (+ `_cache.go`) | BGP peer table |
| `func_bgp_error_text.go` | Decoded BGP last-error text |
| `func_licenses.go` | License inventory per device (Cisco, Fortinet, Sophos, Check Point, Blue Coat, MikroTik) |

The `snmp_topology` module exposes more:

| Function file | What it exposes |
|---|---|
| `func_topology.go`, `func_topology_v1.go` | Topology payload in the `netdata.topology.v1` schema |
| `func_topology_handler.go` | Request router and entry point |
| `func_topology_depth.go` | Depth-bounded traversal |
| `func_topology_managed_focus.go` | Filter by managed-device focus |
| `func_topology_options.go`, `func_topology_presentation.go` (+ `_params.go`, `_schema.go`, `_types.go`) | Presentation contract for the UI |

**Implication for traps**: a `func_traps` / `func_trap_log` function would be the natural way to expose received traps to the UI without inventing a new transport. The pattern is established.

---

## 8. Storage / Persistence

**No relational database**. Netdata stores SNMP-related runtime state in in-memory caches:

- `device_registry` in `ddsnmp/device_registry.go` (per-device profile match cache)
- `topologyCache` (per-device, in `snmp_topology/topology_cache.go`)
- BGP peers cache (`func_bgp_peers_cache.go`)
- Interfaces cache (`func_interfaces_cache.go`)
- Per-device unsupported-table cache (work-in-progress: see `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md`)

Persisted state on disk:
- Service discovery: targets persisted via `filepersister` (`snmpsd/discoverer.go`)
- Metrics: stored in Netdata's native TSDB (dbengine), not SNMP-specific

Profiles are read-only files under `snmp.profiles/`, loaded at startup by `profile_catalog.go`.

**Implication for traps**: there is no event store, no relational schema for trap records, no on-disk index for trap history. A trap subsystem would need to invent its persistence model (or piggyback on the existing log indexing — see §10).

---

## 9. Integration With Other Signals

### 9.1 Metrics

SNMP metrics are first-class. Each profile metric becomes a Netdata chart on the device vnode. Charts auto-publish via `ChartTemplateYAML` mechanisms (`charts.go`, `charts_test.go`).

### 9.2 Alerts

`src/health/health.d/` ships these stock SNMP alert templates:
- `snmp.conf` — licensing alerts (`snmp.license.remaining_time`, `snmp.license.authorization_remaining_time`) with `class: Errors`, `type: NetworkDevice`, warn <30 days, crit <7 days
- `snmp_bgp.conf` — BGP peer state alerts
- `snmp_cisco_nexus.conf` — Nexus-specific alerts
- `snmp_fortigate.conf` — FortiGate-specific alerts

Pattern: alerts read from chart contexts produced by the SNMP module. They are template-driven, not trap-driven.

### 9.3 Topology

Topology integration is the strongest signal integration in the SNMP subsystem. See §4 — the `snmp_topology` collector feeds the topology engine which produces the `netdata.topology.v1` payload consumed by the UI's topology view.

### 9.4 Logs

No SNMP-to-log path currently. Netdata's log pipeline (`systemd-journal`, OTEL logs, Windows events) is separate; SNMP does not write events into it.

### 9.5 Northbound

No outbound SNMP. Netdata does not currently emit SNMP traps as alert transport. Compare with LibreNMS (which DOES emit traps northbound as an alert transport).

---

## 10. Tests and Fixtures

| Module | `_test.go` files |
|---|---|
| `collector/snmp/` | 77 test files |
| `collector/snmp_topology/` | 22 test files |
| `pkg/snmputils/` | small (sysuptime_test, overrides_test) |
| `ddsnmp/` and `ddsnmpcollector/` | dozens of `*_profile_test.go` and `*_fixture_test.go` (Alcatel/Arista/Cisco/Huawei/Juniper/Nokia BGP profile tests; Bluecoat/Sophos/Cisco licensing tests) |

Fixtures: the only on-disk fixture dataset is `ddsnmp/testdata/librenms/` — LibreNMS-derived per-device walk dumps used to reproduce real-device responses in unit tests. This is the kind of fixture model that would be ideal for trap testing — replay captured PDUs against the parser.

CI workflow: `.github/workflows/snmp-topology-tests.yml` — runs topology tests on PRs.

---

## 11. Documentation

- Generated integration doc: `collector/snmp/integrations/snmp_devices.md` — produced from `metadata.yaml` via the integrations pipeline (`integrations-lifecycle` project skill). It lists all bundled profiles, keywords, supported devices, and features.
- Topology integration doc: `collector/snmp_topology/integrations/snmp_devices.md` (much smaller).
- SD integration doc under `discovery/sdext/discoverer/snmpsd/integrations/`.

The generated doc claims "**Built-in vendor profiles** … no manual OID configuration needed", "**Automatic vendor/model detection**", "**ICMP ping**: Optional round-trip latency monitoring", "**SNMP v1, v2c, and v3 support**", "**Shared device-level licensing metrics**", "**Interactive licensing drill-down**" via the `snmp:licenses` function.

---

## 12. What is currently MISSING vs SNMP trap support

| Capability | Today | Required for traps |
|---|---|---|
| UDP/162 listener | None | Need a goroutine listening on UDP/162 (privileged port — same CAP_NET_BIND_SERVICE concern as listening on 161 outbound is not a concern) |
| ASN.1/BER trap PDU parser | gosnmp parses request/response PDUs but the trap-PDU paths (v1 TrapPDUv1, v2c/v3 NotificationPDU, InformRequest) are not exercised by Netdata today | Need either gosnmp's trap handler path enabled (gosnmp has a `Listen` API for traps) or a separate light parser |
| MIB compilation | Not present — profiles are hand-authored YAML, not derived from `.mib` files | Either keep the hand-curated approach (profile authors define trap OID → name mappings) or add MIB compilation (significant new dependency) |
| Trap OID → semantic mapping | Not present | Could extend `ProfileDefinition` with a `Traps` block similar to existing `Topology` / `Licensing` blocks |
| Source-IP → device mapping | EXISTS for metric path (`snmp` job → vnode) | Reusable for traps |
| Deduplication / storm handling | Not present | Need to invent (see foundational spec §6.6 "Hierarchical Deduplication") |
| Severity normalization | Not present for traps | Need a config surface (could piggyback on alert health templates) |
| Trap forwarding (northbound) | Not present | Out of scope for first cut |
| SNMPv3 USM key store | Used inbound for polling; same keys could be reused for trap auth | Reusable |
| TLSTM / DTLS transport | Not present in gosnmp or the fork | Not required for first cut; almost no real devices use it |
| Trap-as-event store | Not present | Needs design decision: log-pipeline ingestion vs new event store vs metric annotations |
| Trap-as-annotation on metrics | Not present | Would need a new UI surface — see what Datadog/Grafana do |
| Topology delta on `linkDown` / `linkUp` | Not present (topology refreshes on schedule) | Would benefit `snmp_topology` operationally — event-driven re-poll vs 30-min rescan |
| Alert routing | EXISTS for chart-based alerts | Reusable only if traps become metrics; otherwise traps need their own routing |

---

## 13. Reusable Assets (the bottom line)

If we decide to add SNMP trap support, the following existing parts are highly likely to be reused as-is or with small adaptation:

1. **`gosnmp` (ilyam8 fork) listen path** — gosnmp ships `GoSNMPServer`-style trap reception; reuse rather than write a new BER parser.
2. **Source IP → vnode mapping** — already done for outgoing polling; a trap receiver knows the source IP, and we can map to the configured SNMP job's vnode.
3. **`ddsnmp.Profile` + `ProfileDefinition`** — extend with a `Traps` config block; profile authors define `trapOID → handler`. Inherits "extends" mixin model and the existing 317-profile library.
4. **`device_registry.go`** — already does sysObjectID/sysDescr → profile matching; the same registry can answer "which profile applies to this trap source."
5. **`enterprise-numbers.txt`** — already covers vendor enterprise OIDs; useful for v1 traps where the enterprise field is structurally separate.
6. **`Functions` framework** — a `func_traps` / `func_trap_log` function is the natural UI surface, no new transport needed.
7. **Topology cache** — `linkDown`/`linkUp` traps could trigger immediate `ifStatusByIndex` updates and a topology delta.
8. **Vnodes** — every trap source is already a vnode (if it was polled).
9. **Health alerts** — if traps become chart metrics (counter: "linkDown traps per minute per device"), existing health template syntax handles them.
10. **LibreNMS fixture pattern** — capture real device trap PDUs once, replay forever.

What we would have to BUILD fresh:

- UDP/162 listener wiring into go.d plugin lifecycle (one collector or one daemon-level component?)
- Trap PDU dispatcher (parse → resolve → enrich → route)
- Severity normalization configuration
- Deduplication / storm handling (rate limit, suppression keys, window)
- Persistence model decision (log pipeline? new event store? metric counters only?)
- Trap-driven topology updates (cheap if we choose to keep them transient)
- UI surface decisions (Functions tab is established; metric annotations on charts would be new)
- MIB import workflow IF we decide profile-only is insufficient (significant)

---

## 14. Key constraints inherited from Netdata's architecture

- **Per-device collector model**: today, each SNMP device is one job in a per-device collector. Traps invert this — one listener serves many devices. The trap subsystem would not fit cleanly inside the existing per-job lifecycle. It is more naturally a singleton collector (like `snmp_topology`) or a plugin-level service.
- **No central RDBMS**: Netdata is agent-local and TSDB-backed. A trap subsystem cannot lean on a Postgres schema the way OpenNMS does. Persistence must use existing mechanisms (TSDB, log pipeline) or invent a small file-backed store.
- **Configuration is YAML files (+ DynCfg)**: Operator configuration must be expressible in flat YAML and a JSON Schema; DynCfg integration is preferred for runtime changes.
- **Distribution**: Netdata is installed everywhere (1.5M+ daily). The trap subsystem cannot ship with heavyweight dependencies (no JVM, no PostgreSQL, no MIB compiler binary by default). C, Go, and Rust are fair game.
- **UDP/162 privileged port**: Netdata generally runs as `netdata` user; will need `CAP_NET_BIND_SERVICE` or a port-mapper if we listen on 162 directly. There is precedent in the installer for setting capabilities.
- **Profiles are authored, not compiled**: The existing profile model is *human-curated YAML*, not auto-generated from MIBs. Trap support should likely match that — author trap mappings into profiles rather than introduce an MIB compiler.
- **Two parallel SOWs in flight**: `.agents/sow/current/SOW-0022-20260509-topology-table-composition.md`, `SOW-0030-20260517-network-connections-dependency-semantics.md`, plus several SNMP-related projection SOWs in `done/`. Trap work should not collide with active topology and projection work — coordinate.

---

## 15. Open design questions for the eventual Netdata trap-support discussion

(These are NOT decided. They are the shape of the questions Costa will need to answer once the comparative-analysis is in hand.)

1. **One trap listener for the whole agent, or one per SNMP job?** Industry pattern is one listener. Netdata's collector model favours per-job. Mixing creates a singleton problem.
2. **Profile-driven trap OID mapping, or MIB compilation?** Profile-driven is consistent with the rest of the codebase. MIB compilation handles long-tail vendors but introduces a heavy dependency.
3. **Persistence**: traps as metrics (counter increments)? as log lines? as a new event store? Each has UX consequences.
4. **Severity strategy**: trust vendor severity vs profile-defined normalization vs operator-defined override matrix? Foundational spec §6.3 argues for normalization; we already have a per-profile model that could carry this.
5. **Topology integration**: should `linkDown` / `linkUp` traps trigger immediate `snmp_topology` re-poll? Or only annotate?
6. **Alerting**: if traps become metrics, existing health templates handle them. If traps become events, we need a new alert path.
7. **UI**: Functions tab for trap log? Metric annotations on charts? New topology overlay? All three?
8. **InformRequest support**: yes/no? Most monitoring systems support it; gosnmp supports the protocol; complexity is low if we receive but don't necessarily emit informs from Netdata.
9. **Storm handling**: where does the dedup window live? In the listener? In a separate stage? What is the operator-tunable surface?
10. **Receiver as a separate `go.d` collector** (`snmp_traps`) or a separate plugin (`netdata.trapd`)? The former integrates with the existing lifecycle; the latter is closer to how OpenNMS / Zenoss split their trap daemon from the rest of the NMS.

---

## 16. Evidence trail

- Module survey: `find . -type d -iname '*snmp*'`, `find . -type f -iname '*snmp*'` from working tree
- Library version: `src/go/go.mod` lines for `gosnmp/gosnmp v1.42.1` and the `ilyam8/gosnmp` replace
- Profile count: `find src/go/plugin/go.d/config/go.d/snmp.profiles -name '*.yaml' | wc -l` → 317
- TopologyKind list: `src/go/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition/topology.go:6-26`
- Topology cache struct: `src/go/plugin/go.d/collector/snmp_topology/topology_cache.go:11-38`
- Config schema: `src/go/plugin/go.d/collector/snmp/config_schema.json` (383 lines)
- Health alerts: `src/health/health.d/snmp*.conf`
- Tests: `find src/go/plugin/go.d/collector/snmp -name '*_test.go' | wc -l` → 77 (snmp), 22 (snmp_topology)
- Trap absence: `find src/go -path '*snmp*' -type f \( -iname '*trap*' -o -iname '*notification*' -o -iname '*inform*' \)` → 0 hits; `grep -rl 'snmptrap\|InformRequest\|TrapPDU' src/go --include='*.go'` → 0 hits
- Profile schema root: `src/go/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition/profile_definition.go:13-26`
- Functions: `src/go/plugin/go.d/collector/snmp/func_*.go` (router, interfaces, bgp_peers, bgp_error_text, licenses), `snmp_topology/func_topology*.go`
- Generated docs: `src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md`

End of inventory.
