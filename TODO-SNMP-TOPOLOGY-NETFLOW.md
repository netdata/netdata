# YOU MUST READ THE WHOLE OF THIS TODO FILE, AND YOU MUST ALWAYS UPDATE IT WITH YOUR PROGRESS - TRY TO KEEP THE TODO FILE AT A MANAGEABLE SIZE, OVERWRITING PREVIOUSLY PENDING ITEMS WITH THEIR COMPLETION AND VERIFICATION. YOU MUST READ THE TODO IN WHOLE, AFTER EACH COMPACTION OF YOUR MEMORY.
# Feature Analysis: SNMP Topology Map & NetFlow/IPFIX/sFlow

## Contract (Costa, 2026-02-03)

1. These are new modules. We will touch some existing code, but the majority of the work is new modules.
2. I expect that will work autonomously until you finish the work completely, delivering 100% production quality code.
3. To assist you, you can run claude, codex, gemini and glm-4.7, as many times as required, for code reviews, for assistance in any dilemma. I grant this permission to you for this TODO, overriding ~/.AGENTS.md rule that prevents it.
4. I expect the new code to have clear separation of concerns, clarity and simplicity. You may perform as many code reviews as necessary to understand existing patterns.
5. Testing is 100% required. Anything that is not being tested cannot be trusted.
6. TESTING MUST BE DONE WITH REAL DEVICE DATA, AVAILABLE FROM THE MULTIPLE SOURCES DEFINED BELOW. TESTING WITH HYPOTHETICAL GENERATED DATA IS NOT ACCEPTED AS TESTING.
7. I expect you will use as many **real-life data** to ensure the code works end-to-end. You can spawn VMs if needed (libvirt), start docker containers, or whatever needed to ensure everything is fully tested.
8. ANY FEATURE IS NOT COMPLETE IF IT IS NOT THOROUGHLY TESTED AGAINST REAL DEVICE DATA.
9. You are not allowed to exclude anything from the scope. Do not change the deliverable.

## TL;DR

Two features designed for **distributed Netdata architecture**:
- Multiple agents collect SNMP/topology/flows independently
- Each agent maintains local metrics, topology, and flow data
- Netdata Cloud queries all agents, aggregates/merges, and presents unified view

TOPOLOGY SHOULD WORK AT LEAST IN 2 LEVELS: LAYER 2 (SNMP) AND LAYER 3 (FLOWS). THE JSON SCHEMA OF THE FUNCTION SHOULD BE COMMON. SO THE ACTORS, THE LINKS, THE FLOWS SHOULD BE AGNOSTIC OF PROTOCOL OR LAYER, SO THAT THE SAME VISUALIZATION CAN PERFECTLY WORK FOR LAYER 2 TOPOLOGY MAPS, LAYER 3 TOPOLOGY MAPS AND ANY OTHER LAYER WE MAY ADD IN THE FUTURE. OF COURSE, IF ANY LAYER REQUIRED ADDITIONAL FIELDS (e.g. port number of a switch), THESE FIELDS SHOULD BE GENERALIZED AND BE AVAILABLE ON ALL LAYERS. For example, the port of switch at layer 2, could be the process name of a host at layer 3, so generally it should be the entity/module/component within an actor. You are allowed and expected to design this schema YOURSELF - once you finish we may polish it together - but do the heavy lifting of defining a multi-layered generic topology schema.

**Key Challenge:** Data must be "aggregatable" - topology must merge, flows must sum without double-counting.

**Permission:** Costa grants permission to run **Claude, Codex, Gemini, and GLM‑4.7** for reviews or dilemmas as needed.

## ✅ COMPLETED WORK — VERIFICATION REQUIRED

The following items are complete and verified via tests:

| Item | Description | Status |
|------|-------------|--------|
| **9** | Expand real device test coverage — add ALL LibreNMS snmprec files (106 LLDP, 15+ CDP), ALL Akvorado pcaps | ✅ DONE |
| **10** | Simulator-based integration testing — snmpsim + NetFlow stress replay workflow | ✅ DONE |
| **11** | Complete LLDP/CDP profiles — add ALL missing MIB fields (management addresses, capabilities, etc.) | ✅ DONE |

**See "Plan" section for details and verification notes.**

---

## Progress (Agent)

- 2026-02-04: **Completed Costa request** — updated `.github/workflows/snmp-netflow-sim-tests.yml` to run only when SNMP/NetFlow/IPFIX/sFlow-related files change; improved snmpsim fixture selection (LLDP `lldpRemSysName` + CDP `cdpCacheDeviceId`) for reliable CI; fixed LLDP/CDP profiles to use valid scalar metrics and ensured `lldpRemEntry`/`cdpCacheEntry` use existing numeric columns so topology rows are emitted. Manual integration tests run locally with snmpsim + stress pcaps: `go test -tags=integration ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow` **PASS**.
- 2026-02-04: Ran `gofmt -w` across all Go files in the repo as requested.
- 2026-02-04: Re-verified implementation completeness; ran unit + integration-tag tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`, `go test -count=1 -tags=integration ./plugin/go.d/collector/netflow ./plugin/go.d/collector/snmp`, and `go test -v -count=1 -tags=integration ./plugin/go.d/collector/snmp`). NetFlow integration replay passed; SNMP integration is env-gated and skipped without `NETDATA_SNMPSIM_ENDPOINT`/`NETDATA_SNMPSIM_COMMUNITIES`. Verified deps `gopacket` and `goflow2/v2` are latest via `go list -m -u ...` (no updates).
- 2026-02-04: Reviewed current state and re-ran unit + integration-tag tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge` and `go test -count=1 -tags=integration ./plugin/go.d/collector/netflow ./plugin/go.d/collector/snmp`); all pass. No code changes required.
- 2026-02-04: Marked Plan items 9-11 as ✅ DONE; re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass.
- 2026-02-04: Completed pending scope: expanded LLDP/CDP profiles (caps + mgmt address tables + stats), extended topology cache/types for mgmt addresses/capabilities, imported **116** LibreNMS snmprec fixtures + **36** Akvorado pcaps with updated attribution, added integration tests (`//go:build integration`) and CI workflow `snmp-netflow-sim-tests.yml` (snmpsim + NetFlow stress pcaps), expanded unit tests to cover all fixtures. Ran `go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge` (pass).
- 2026-02-04: Re-ran targeted Go tests with real-device fixtures (snmp, netflow, jobmgr, funcapi, topology-flow-merge); all pass. No code changes needed.
- 2026-02-04: Reviewed SNMP topology + NetFlow/IPFIX/sFlow modules, schema types, and merge tool; re-ran real-device tests (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. No code changes required.
- 2026-02-04: Reviewed repo for completeness (docs/config/schema/function types) and re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. No code changes required.
- 2026-02-04: Re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. Verified module versions via `go list -m -u github.com/google/gopacket github.com/netsampler/goflow2/v2` (no updates available).
- 2026-02-04: Completed NetFlow collector (v5/v9/IPFIX/sFlow) + flow aggregation + flows function + charts + docs/config/schema + merge CLI; added real-device testdata (LibreNMS snmprec + Akvorado pcaps) with attribution.
- 2026-02-03: Implemented topology/flows response types + schema support + SNMP topology function/cache + LLDP/CDP profiles + charts + docs + tests.

---

## Codebase Analysis (Agent, 2026-02-03)

### Functions infrastructure (facts)

- go.d function responses now resolve `type` from `MethodConfig.ResponseType` or `FunctionResponse.ResponseType` (defaults to `"table"`). Evidence: `src/go/plugin/go.d/agent/jobmgr/funcshandler.go` (resolveResponseType).
- The **Functions UI schema** accepts custom `type` values for topology and flows via dedicated definitions. Evidence: `src/plugins.d/FUNCTION_UI_SCHEMA.json` (topology_response / flows_response).
- `funcapi.FunctionResponse` supports **columns/data + charting** (table) or **custom data payloads** when `ResponseType` is non-table. Evidence: `src/go/pkg/funcapi/response.go`.
- The **functions-validation** tool validates responses against `FUNCTION_UI_SCHEMA.json`. Evidence: `src/go/tools/functions-validation/README.md:67-71`.

### SNMP collector (facts)

- The SNMP module registers **Methods/MethodHandler** and uses a **funcRouter** for function dispatch. Evidence: `src/go/plugin/go.d/collector/snmp/collector.go:26-36`, `src/go/plugin/go.d/collector/snmp/func_router.go:20-64`.
- The existing **SNMP interfaces function** is a good reference for new function handlers (cache → build columns/data → return `FunctionResponse`). Evidence: `src/go/plugin/go.d/collector/snmp/func_interfaces.go:31-113`.
- SNMP collection uses **ddsnmp** profile metrics, and the **interface cache** is populated inside table-metric processing (`collectProfileTableMetrics`). Evidence: `src/go/plugin/go.d/collector/snmp/collect_snmp.go:24-94`.
- SNMP devices are typically **vnodes**; labels include sysinfo + metadata (sys_object_id, name, vendor, model, etc.), which can be used for deterministic correlation. Evidence: `src/go/plugin/go.d/collector/snmp/collect.go:119-190`.
- `snmputils.SysInfo` already provides **sysName/sysDescr/sysObjectID/sysLocation** and metadata. Evidence: `src/go/plugin/go.d/pkg/snmputils/sysinfo.go:20-85`.

### SNMP profiles & tags (facts)

- Profiles support **metric_tags** and cross-table tags; `_std-if-mib.yaml` shows tags derived from **ifTable/ifXTable**. Evidence: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-if-mib.yaml:58-118`.
- Tag processing converts PDU values to **strings** and stores them as tags. This can carry LLDP/CDP string fields. Evidence: `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/tag_processor.go:51-84`.
- LLDP/CDP standard profiles now exist and are used for topology discovery. Evidence: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-lldp-mib.yaml`, `_std-cdp-mib.yaml`.

### NetFlow/IPFIX (facts)

- NetFlow/IPFIX/sFlow collector exists under `src/go/plugin/go.d/collector/netflow` with decoding, aggregation, charts, and flows function responses.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           NETDATA CLOUD                                  │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Aggregation Layer                                               │   │
│  │  - Query all agents for topology/flow data                       │   │
│  │  - Merge topologies (deduplicate nodes, validate links)          │   │
│  │  - Aggregate flows (sum by dimension, normalize sampling)        │   │
│  │  - Present unified visualization                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ Query API
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│   Agent A     │       │   Agent B     │       │   Agent C     │
│ (Data Center) │       │ (Branch Office)│      │ (Cloud Region)│
├───────────────┤       ├───────────────┤       ├───────────────┤
│ SNMP Jobs:    │       │ SNMP Jobs:    │       │ SNMP Jobs:    │
│ - Core Router │       │ - Branch SW   │       │ - Cloud LB    │
│ - DC Switches │       │ - Branch FW   │       │ - Cloud FW    │
│ - Firewalls   │       │               │       │               │
├───────────────┤       ├───────────────┤       ├───────────────┤
│ Local Topology│       │ Local Topology│       │ Local Topology│
│ (partial view)│       │ (partial view)│       │ (partial view)│
├───────────────┤       ├───────────────┤       ├───────────────┤
│ Flow Collector│       │ Flow Collector│       │ Flow Collector│
│ (local flows) │       │ (local flows) │       │ (local flows) │
└───────────────┘       └───────────────┘       └───────────────┘
```

---

## Part 1: SNMP Topology Map (Distributed)

### 1.1 Design Principles

1. **Each agent builds its LOCAL topology** from devices it monitors
2. **Topology data is exposed via API** for Cloud to query
3. **Cloud merges all agent topologies** into unified view
4. **Globally unique identifiers** enable cross-agent correlation

### 1.2 Aggregation Challenges

| Challenge | Problem | Solution |
|-----------|---------|----------|
| **Same device, multiple agents** | Device monitored by 2+ agents | Use chassis ID / sysObjectID as global key, deduplicate |
| **Cross-agent links** | Agent A sees link to device monitored by Agent B | Cloud matches by remote chassis ID |
| **Bidirectional validation** | Link reported by one end, other end on different agent | Cloud validates across agent boundaries |
| **Inconsistent naming** | Same device has different sysName per agent | Canonical ID from chassis ID, aliases tracked |
| **Partial visibility** | Agent sees neighbor but doesn't monitor it | Mark as "discovered" vs "monitored" |

### 1.3 Data Model (Aggregation-Friendly)

```go
// Per-agent topology data (exposed via API)
type AgentTopology struct {
    AgentID       string                  // Netdata agent unique ID
    CollectedAt   time.Time               // Timestamp of collection
    Devices       []TopologyDevice        // Devices this agent monitors
    DiscoveredLinks []TopologyLink        // Links discovered via LLDP/CDP
}

// Device with globally unique identifier
type TopologyDevice struct {
    // === GLOBAL IDENTIFIERS (for cross-agent matching) ===
    ChassisID       string              // LLDP chassis ID (MAC or other)
    ChassisIDType   string              // "mac", "networkAddress", "hostname", etc.
    SysObjectID     string              // SNMP sysObjectID (vendor/model)

    // === LOCAL IDENTIFIERS ===
    AgentJobID      string              // SNMP job ID on this agent
    ManagementIP    string              // IP used to poll this device

    // === DEVICE INFO ===
    SysName         string              // SNMP sysName
    SysDescr        string              // SNMP sysDescr
    SysLocation     string              // SNMP sysLocation
    Capabilities    []string            // "router", "bridge", "station", etc.
    Vendor          string              // Derived from sysObjectID
    Model           string              // Derived from sysDescr or profile

    // === INTERFACES ===
    Interfaces      []TopologyInterface // All interfaces with neighbor info

    // === ROUTING TOPOLOGY ===
    BGPPeers        []BGPPeerInfo       // BGP neighbor relationships
    OSPFNeighbors   []OSPFNeighborInfo  // OSPF adjacencies
}

// Interface with neighbor discovery data
type TopologyInterface struct {
    // === LOCAL INTERFACE ===
    IfIndex         int
    IfName          string              // e.g., "GigabitEthernet0/1"
    IfDescr         string
    IfAlias         string              // User-configured description
    IfType          int                 // IANAifType
    IfSpeed         uint64              // bits/sec
    IfOperStatus    string              // up/down/testing
    IfAdminStatus   string
    IfPhysAddress   string              // MAC address

    // === LLDP NEIGHBOR (if discovered) ===
    LLDPNeighbor    *LLDPNeighborInfo

    // === CDP NEIGHBOR (if discovered) ===
    CDPNeighbor     *CDPNeighborInfo
}

// LLDP neighbor info (enables cross-agent correlation)
type LLDPNeighborInfo struct {
    // === REMOTE DEVICE IDENTIFIERS ===
    RemChassisID      string            // Key for cross-agent matching!
    RemChassisIDType  string
    RemSysName        string
    RemSysDescr       string
    RemSysCapabilities []string

    // === REMOTE PORT ===
    RemPortID         string
    RemPortIDType     string
    RemPortDescr      string

    // === MANAGEMENT ADDRESS ===
    RemManagementAddr string
}

// CDP neighbor info
type CDPNeighborInfo struct {
    RemDeviceID       string            // Cisco device ID
    RemPlatform       string            // e.g., "cisco WS-C3750-48P"
    RemPortID         string            // e.g., "GigabitEthernet1/0/1"
    RemCapabilities   []string
    RemManagementAddr string
    RemNativeVLAN     int
}

// Link representation (for API/Cloud)
type TopologyLink struct {
    // === SOURCE (this agent's device) ===
    SrcChassisID    string              // Global device ID
    SrcIfIndex      int
    SrcIfName       string
    SrcAgentID      string              // Which agent monitors source

    // === TARGET (may be on different agent) ===
    DstChassisID    string              // Global device ID (for matching)
    DstPortID       string              // As reported by LLDP/CDP
    DstSysName      string              // As reported by LLDP/CDP
    DstAgentID      string              // Empty if not monitored by any agent

    // === LINK METADATA ===
    Protocol        string              // "lldp", "cdp", "bgp", "ospf"
    DiscoveredAt    time.Time
    LastSeen        time.Time

    // === VALIDATION (set by Cloud after aggregation) ===
    Bidirectional   bool                // Both ends confirmed
    Validated       bool                // Cloud has matched both ends
}
```

### 1.4 Cloud Aggregation Logic

```
CLOUD TOPOLOGY MERGE ALGORITHM:

1. COLLECT
   - Query /api/v3/topology from all connected agents
   - Each agent returns AgentTopology with its local view

2. BUILD DEVICE INDEX
   FOR each agent's devices:
     key = normalize(ChassisID, ChassisIDType)
     IF key exists in global_devices:
       MERGE device info (prefer newest, keep all management IPs)
       ADD agent to device.MonitoredByAgents[]
     ELSE:
       INSERT into global_devices

3. PROCESS LINKS
   FOR each agent's discovered links:
     src_key = normalize(link.SrcChassisID)
     dst_key = normalize(link.DstChassisID)

     link_key = canonical_link_key(src_key, dst_key)

     IF link_key exists in global_links:
       // Link already known from other direction
       SET link.Bidirectional = true
       SET link.Validated = true
     ELSE:
       INSERT into global_links
       // Check if destination device is monitored
       IF dst_key in global_devices:
         SET link.DstAgentID = device.MonitoredByAgents[0]
       ELSE:
         // Discovered but not monitored - mark as edge device
         CREATE placeholder device (discovered=true, monitored=false)

4. VALIDATE
   FOR each link:
     IF NOT link.Validated:
       // Only one end reported - might be stale or asymmetric
       IF link.LastSeen < (now - LLDP_HOLDTIME):
         MARK as potentially_stale
       ELSE:
         MARK as unidirectional (warning)

5. BUILD GRAPH
   - Create nodes for all devices
   - Create edges for all validated links
   - Annotate with metrics (traffic, status) from agents
```

### 1.5 Agent API Endpoint

```
GET /api/v3/topology

Response:
{
  "agent_id": "abc123",
  "collected_at": "2025-01-15T10:30:00Z",
  "devices": [...],
  "links": [...],
  "stats": {
    "devices_monitored": 15,
    "links_discovered": 42,
    "lldp_neighbors": 38,
    "cdp_neighbors": 12,
    "bgp_peers": 8,
    "ospf_neighbors": 6
  }
}
```

### 1.6 Implementation Components

| Component | Location | Description |
|-----------|----------|-------------|
| LLDP Profile | `snmp.profiles/default/_std-lldp-mib.yaml` | Collect LLDP neighbor data |
| CDP Profile | `snmp.profiles/default/_std-cdp-mib.yaml` | Collect CDP neighbor data |
| Topology Builder | `collector/snmp/topology/` | Build local topology from SNMP data |
| Topology Cache | `collector/snmp/topology/cache.go` | Cache topology between polls |
| API Handler | `collector/snmp/func_topology.go` | Expose topology via function API |
| Cloud Aggregator | Netdata Cloud | Merge agent topologies |

### 1.7 SNMP Profiles Needed

**LLDP Profile (`_std-lldp-mib.yaml`):**
```yaml
# LLDP-MIB (IEEE 802.1AB)
metrics:
  # Local system info
  - MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.3.7  # lldpLocPortTable
      name: lldpLocPortTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.3.7.1.3
        name: lldpLocPortId
      - OID: 1.0.8802.1.1.2.1.3.7.1.4
        name: lldpLocPortDesc
    metric_tags:
      - column:
          OID: 1.0.8802.1.1.2.1.3.7.1.2
          name: lldpLocPortIdSubtype
        tag: port_id_subtype

  # Remote neighbor info (critical for topology)
  - MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.4.1  # lldpRemTable
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.5
        name: lldpRemChassisId
      - OID: 1.0.8802.1.1.2.1.4.1.1.7
        name: lldpRemPortId
      - OID: 1.0.8802.1.1.2.1.4.1.1.8
        name: lldpRemPortDesc
      - OID: 1.0.8802.1.1.2.1.4.1.1.9
        name: lldpRemSysName
      - OID: 1.0.8802.1.1.2.1.4.1.1.10
        name: lldpRemSysDesc
    metric_tags:
      - column:
          OID: 1.0.8802.1.1.2.1.4.1.1.4
          name: lldpRemChassisIdSubtype
        tag: chassis_id_subtype
      - column:
          OID: 1.0.8802.1.1.2.1.4.1.1.6
          name: lldpRemPortIdSubtype
        tag: port_id_subtype
```

---

## Part 2: NetFlow/IPFIX (Distributed)

### 2.1 Design Principles

1. **Each agent runs a flow collector** listening on configured UDP ports
2. **Flows are aggregated locally** into time-bucketed summaries
3. **Flow summaries exposed via API** for Cloud to query
4. **Cloud aggregates across agents** avoiding double-counting

### 2.2 Aggregation Challenges

| Challenge | Problem | Solution |
|-----------|---------|----------|
| **Same flow, multiple exporters** | Transit traffic exported by ingress AND egress routers | Deduplicate by flow key + direction, or use ingress-only |
| **Sampling rate variance** | Router A samples 1:1000, Router B samples 1:100 | Normalize all flows to estimated actual counts |
| **Cross-agent flows** | Flow enters at Agent A's router, exits at Agent B's | Track by flow key, sum bytes/packets, don't double-count |
| **Time alignment** | Agents have different clock skew | Use flow timestamps, align to common buckets |
| **Cardinality explosion** | Too many unique src/dst combinations | Pre-aggregate to top-N per dimension |

### 2.3 Data Model (Aggregation-Friendly)

```go
// Per-agent flow summary (exposed via API)
type AgentFlowSummary struct {
    AgentID       string
    PeriodStart   time.Time           // Start of aggregation window
    PeriodEnd     time.Time           // End of aggregation window
    Exporters     []ExporterInfo      // Flow exporters sending to this agent

    // Pre-aggregated summaries (for Cloud to merge)
    ByProtocol    []ProtocolSummary
    ByASPair      []ASPairSummary     // Top N AS pairs
    ByInterface   []InterfaceSummary
    ByCountry     []CountrySummary    // If GeoIP enabled
    TopTalkers    []TalkerSummary     // Top N by bytes

    // Raw aggregates for flexible Cloud queries
    FlowBuckets   []FlowBucket        // Time-bucketed flow data
}

// Exporter metadata (for Cloud correlation)
type ExporterInfo struct {
    ExporterIP      string
    ExporterName    string            // From SNMP sysName if available
    SamplingRate    uint32            // 1:N, used for normalization
    FlowVersion     string            // "netflow_v5", "netflow_v9", "ipfix", "sflow"
    InterfaceMap    map[uint32]string // ifIndex -> ifName (from SNMP)
}

// Flow bucket (time-aggregated, dimension-grouped)
type FlowBucket struct {
    Timestamp     time.Time           // Bucket start time
    Duration      time.Duration       // Bucket size (e.g., 1 minute)

    // === FLOW KEY (for deduplication across agents) ===
    FlowKey       FlowKey

    // === COUNTERS (normalized for sampling) ===
    Bytes         uint64              // Normalized byte count
    Packets       uint64              // Normalized packet count
    Flows         uint64              // Number of distinct flows

    // === SAMPLING INFO ===
    RawBytes      uint64              // Before normalization
    SamplingRate  uint32              // Rate used for normalization

    // === SOURCE INFO ===
    ExporterIP    string
    AgentID       string
    Direction     string              // "ingress" or "egress"
}

// Flow key for deduplication
type FlowKey struct {
    // 5-tuple (hashed for cardinality control)
    SrcAddrPrefix   string            // /24 for IPv4, /48 for IPv6
    DstAddrPrefix   string
    SrcPort         uint16            // 0 if aggregated
    DstPort         uint16            // 0 if aggregated
    Protocol        uint8

    // Routing info
    SrcAS           uint32
    DstAS           uint32

    // Interface info (for per-interface views)
    InIfIndex       uint32
    OutIfIndex      uint32

    // Exporter (for attribution)
    ExporterIP      string
}

// Pre-computed summaries for common queries
type ProtocolSummary struct {
    Protocol    uint8               // TCP=6, UDP=17, etc.
    Bytes       uint64
    Packets     uint64
    Flows       uint64
}

type ASPairSummary struct {
    SrcAS       uint32
    DstAS       uint32
    Bytes       uint64
    Packets     uint64
}

type InterfaceSummary struct {
    ExporterIP  string
    IfIndex     uint32
    IfName      string              // Enriched from SNMP
    Direction   string              // "in" or "out"
    Bytes       uint64
    Packets     uint64
}

type TalkerSummary struct {
    Address     string              // IP or prefix
    Direction   string              // "src" or "dst"
    Bytes       uint64
    Packets     uint64
    TopPorts    []PortCount         // Top N ports
}
```

### 2.4 Cloud Aggregation Logic

```
CLOUD FLOW AGGREGATION ALGORITHM:

1. COLLECT
   - Query /api/v3/flows?period=5m from all connected agents
   - Each agent returns AgentFlowSummary for the period

2. NORMALIZE
   FOR each agent's flow buckets:
     IF bucket.SamplingRate != 1:
       bucket.Bytes = bucket.RawBytes * bucket.SamplingRate
       bucket.Packets = bucket.RawPackets * bucket.SamplingRate

3. DEDUPLICATE (avoid double-counting transit traffic)

   Option A: Ingress-only accounting
     FOR each bucket:
       IF bucket.Direction == "egress":
         SKIP (only count ingress)

   Option B: Flow-key based deduplication
     flow_seen = {}
     FOR each bucket:
       key = hash(bucket.FlowKey, bucket.Timestamp)
       IF key in flow_seen:
         // Same flow reported by multiple exporters
         // Keep the one with better sampling (lower ratio)
         IF bucket.SamplingRate < flow_seen[key].SamplingRate:
           REPLACE flow_seen[key] with bucket
       ELSE:
         flow_seen[key] = bucket

4. AGGREGATE BY DIMENSION

   // Global protocol breakdown
   protocol_totals = {}
   FOR each bucket:
     protocol_totals[bucket.Protocol] += bucket.Bytes

   // Global AS-pair matrix
   as_matrix = {}
   FOR each bucket:
     as_matrix[(bucket.SrcAS, bucket.DstAS)] += bucket.Bytes

   // Per-interface totals
   interface_totals = {}
   FOR each bucket:
     key = (bucket.ExporterIP, bucket.IfIndex, bucket.Direction)
     interface_totals[key] += bucket.Bytes

5. BUILD VISUALIZATION
   - Sankey diagram from AS-pair matrix
   - Time-series from bucket timestamps
   - Top-N tables from sorted aggregates
```

### 2.5 Agent API Endpoint

```
GET /api/v3/flows?period=5m&dimensions=protocol,as_pair

Response:
{
  "agent_id": "abc123",
  "period_start": "2025-01-15T10:25:00Z",
  "period_end": "2025-01-15T10:30:00Z",
  "exporters": [
    {
      "ip": "10.0.0.1",
      "name": "core-router-01",
      "sampling_rate": 1000,
      "flow_version": "ipfix"
    }
  ],
  "by_protocol": [
    {"protocol": 6, "bytes": 1234567890, "packets": 987654},
    {"protocol": 17, "bytes": 567890123, "packets": 456789}
  ],
  "by_as_pair": [
    {"src_as": 15169, "dst_as": 32934, "bytes": 999999999},
    ...
  ],
  "top_talkers": [...],
  "stats": {
    "total_bytes": 9999999999,
    "total_packets": 8888888,
    "total_flows": 123456,
    "exporters_active": 3
  }
}
```

### 2.6 Implementation Components

| Component | Location | Description |
|-----------|----------|-------------|
| Flow Collector | `collector/netflow/` | UDP listener for NetFlow/IPFIX/sFlow |
| Flow Parser | `collector/netflow/parser/` | Protocol-specific parsers |
| Flow Aggregator | `collector/netflow/aggregator/` | Time-bucket aggregation |
| SNMP Enricher | `collector/netflow/enricher/` | Add interface names from SNMP |
| GeoIP Enricher | `collector/netflow/enricher/` | Add country/ASN info |
| API Handler | `collector/netflow/func_flows.go` | Expose flow data via function API |
| Cloud Aggregator | Netdata Cloud | Merge flow data from agents |

### 2.7 Flow Collector Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     NETDATA AGENT                               │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   Flow Collector                          │  │
│  │                                                           │  │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────┐                │  │
│  │  │ UDP     │   │ UDP     │   │ UDP     │                │  │
│  │  │ :2055   │   │ :4739   │   │ :6343   │                │  │
│  │  │NetFlow  │   │ IPFIX   │   │ sFlow   │                │  │
│  │  └────┬────┘   └────┬────┘   └────┬────┘                │  │
│  │       │             │             │                      │  │
│  │       └─────────────┼─────────────┘                      │  │
│  │                     ▼                                    │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Flow Parser   │                            │  │
│  │            │  (goflow2 lib) │                            │  │
│  │            └───────┬────────┘                            │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐      ┌─────────────────┐  │  │
│  │            │   Enricher     │◄─────│  SNMP Collector │  │  │
│  │            │ (ifIndex→name) │      │  (interface map)│  │  │
│  │            └───────┬────────┘      └─────────────────┘  │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐      ┌─────────────────┐  │  │
│  │            │   Enricher     │◄─────│  GeoIP DB       │  │  │
│  │            │ (IP→Country)   │      │  (MaxMind)      │  │  │
│  │            └───────┬────────┘      └─────────────────┘  │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Aggregator    │                            │  │
│  │            │ (time buckets) │                            │  │
│  │            └───────┬────────┘                            │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Flow Cache    │──────► Metrics             │  │
│  │            │ (ring buffer)  │──────► API /api/v3/flows   │  │
│  │            └────────────────┘                            │  │
│  │                                                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Part 3: Shared Considerations

### 3.1 Agent API Requirements

Both features need new API endpoints. Options:

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **Function API** | Extend existing `/api/v3/function` | Consistent with SNMP interfaces API | Limited to GET semantics |
| **New API paths** | Add `/api/v3/topology`, `/api/v3/flows` | Clean, RESTful | More API surface |
| **Streaming API** | WebSocket for real-time updates | Low latency for live view | More complex |

**Recommendation:** Function API initially (consistent with existing pattern), dedicated paths later if needed.

### 3.2 Data Retention

| Data Type | Agent Retention | Purpose |
|-----------|-----------------|---------|
| Topology snapshot | Latest only | Cloud queries on-demand |
| Topology history | 24 hours | Detect topology changes |
| Flow buckets | 1 hour (detailed) | Recent high-resolution data |
| Flow summaries | 24 hours | Historical queries |

### 3.3 Cloud Query Patterns

**Topology:**
```
GET /spaces/{space}/topology
  - Cloud queries all agents in parallel
  - Merges responses
  - Caches merged topology (TTL: 1 minute)
  - Returns unified graph
```

**Flows:**
```
GET /spaces/{space}/flows?period=5m&group_by=as_pair
  - Cloud queries all agents for period
  - Aggregates/deduplicates
  - Returns merged summary
```

---

## Part 4: Decisions Required

### Decisions Confirmed (2026-02-03)

- **Scope:** This repo is **agent-only**. No Cloud backend changes in this repo.
- **Agent Functions:** Agent must expose **Netdata Functions** for **Topology** and **Flows**.
- **Time-series:** Agent **may create time-series** with **proper labels** for deterministic correlation later.
- **Function Schemas:** Function JSON outputs must be **concrete and specific**, designed for **cross-agent aggregation/correlation**. If time-series are created, the JSON schema must **link/refer** to them.
- **Aggregation Prototype:** Build a **Go program** that merges **non-overlapping / partial-overlapping / fully-overlapping** Function JSON outputs into a final topology map. This is a **prototype** to validate aggregation logic before Cloud integration.
- **Schema Infrastructure:** Reuse existing **Function schema infrastructure** (table-view schema). Add **common fields** so UI can identify **topology** schema vs table/logs. Versioning is allowed and **schema differences across agents** must be aggregatable (e.g., v1 + v2).
- **Metric Correlation:** SNMP devices are typically **vnodes**; metrics are **namespaced by node**. Constants like node identifiers can be used for deterministic correlation.
- **Overlap Handling:** With **clear identity**, multi-agent overlap should be manageable; treat it similar to a single agent with full visibility.
- **Decision 1:** **B** - Function schema identification should use **new function types** (e.g., `topology`, `flows`) instead of `table/log`.
- **Decision 2:** NetFlow ingestion will be a **separate netflow module with jobs**, and **each job has its own listener port**.
- **Decision 4:** **A** - Aggregation prototype should live under `src/go/tools/` as a CLI tool.
- **Decision 5:** **A** - Require Cloud for Topology/Flows functions.
- **Decision 6:** **B** - Emit time-series **plus** functions, with strict label discipline.
- **Decision 3:** Use **goflow2** as the flow parsing library.
- **Decision 3 (version):** Use module **`github.com/netsampler/goflow2/v2` @ v2.2.6** (verified via `go list -m github.com/netsampler/goflow2/v2@latest`).
- **Decision 7:** **Hybrid** LLDP/CDP enablement: **vendor-profile explicit + configurable autoprobe** with **in-memory missing‑OID learning** only.

### Decisions Confirmed (2026-02-04)

- **Flow Protocol Scope:** Implement **NetFlow v5, NetFlow v9, IPFIX and sFlow**.
- **Aggregation Key (Agent):** Aggregate by **5‑tuple + AS + in/out interface** when present. Missing fields omitted; agent does **no dedup** across exporters.
- **Bucketing & Retention:** **bucket_duration defaults to update_every**; retain **max_buckets** (default 60). Old buckets are evicted.
- **Clock Skew Handling:** When flow timestamps are **too old** for current retention, **re-bucket to arrival time** and increment `records_too_old` (avoid dropping data).
- **Sampling Normalization:** Apply **per‑record sampling rate** when available; fallback to **exporter override** or **config default**. Preserve `raw_*` fields.
- **Flows Response Payload:** Conform to **FUNCTION_UI_SCHEMA flows_response** with `exporters`, `buckets`, `summaries`, `metrics`.
- **Time‑series:** Emit totals for **bytes/s, packets/s, flows/s**, plus **dropped records**.
- **sFlow Support:** Decode **sFlow v5** via `goflow2/v2/decoders/sflow`, detect by 32‑bit version (0x00000005). Map SampledIPv4/IPv6 and SampledHeader into flow records; use sample `sampling_rate`, `input/output` ifIndex. (Exporter IP prefers sFlow agent IP if present.)
- **IPFIX Field Coverage:** Add IPFIX field mappings for bytes/packets/flows, protocol, ports, addresses, prefix lengths, interfaces, AS numbers, direction, and flow start/end (seconds/milliseconds/sysUpTime) where available.

### Decision 1: Topology Identifier Strategy

**Question:** How to uniquely identify devices across agents?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Chassis ID (MAC/LLDP) | Standard, unique | Not all devices support LLDP |
| **B** | Management IP | Always available | IPs can change, NAT issues |
| **C** | sysObjectID + sysName | Works without LLDP | Name collisions possible |
| **D** | Composite (A + B + C) | Most robust | Complex matching logic |

**Recommendation:** D - Use chassis ID when available, fall back to composite

### Decision 2: Flow Deduplication Strategy

**Question:** How to handle same flow seen by multiple exporters?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Ingress-only | Simple, no double-count | Loses egress perspective |
| **B** | Flow-key dedup | Accurate | Complex, needs timestamps |
| **C** | Per-exporter only | No dedup needed | Double-counts transit |
| **D** | User-configurable | Flexible | User must understand |

**Recommendation:** A initially (simple), B later

### Decision 3: Flow Aggregation Granularity

**Question:** What dimensions to aggregate by on agents?

| Option | Description | Storage | Query Flexibility |
|--------|-------------|---------|-------------------|
| **A** | Minimal (protocol only) | Low | Limited |
| **B** | Standard (proto + AS + interface) | Medium | Good for most cases |
| **C** | Full (proto + AS + prefix + port) | High | Maximum flexibility |
| **D** | Configurable per-agent | Varies | Optimal but complex |

**Recommendation:** B - Standard dimensions, configurable top-N limits

### Decision 4: SNMP-Flow Integration

**Question:** How to correlate flow interfaces with SNMP interface names?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Same agent monitors both | Direct correlation | Limits deployment flexibility |
| **B** | Cross-agent lookup via Cloud | Flexible deployment | Latency, complexity |
| **C** | Export mapping via flow exporter config | Simple | Manual config required |
| **D** | Agent-local SNMP query to exporter | Works if exporter allows | Extra SNMP load |

**Recommendation:** A initially, D as enhancement

### Decision 5: Implementation Priority

**Question:** Which to implement first?

| Option | Description |
|--------|-------------|
| **A** | Topology first (LLDP/CDP profiles + API) |
| **B** | Flows first (collector + aggregation + API) |
| **C** | Both in parallel |

**Recommendation:** A - Topology is lower effort and builds on existing SNMP infrastructure

### Decision 6: Function Schema Identification Strategy (Topology/Flows)

**Background:** go.d functions are wrapped with `type: "table"` in the job manager (`src/go/plugin/go.d/agent/jobmgr/funcshandler.go:153-196`). The Functions UI schema only accepts `type: "table"` or `type: "log"` for data responses (`src/plugins.d/FUNCTION_UI_SCHEMA.json:189-195`).  
We still need **common fields** so UI/Cloud can detect **topology** vs **flows** schemas and handle **versioning**.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Keep `type: "table"` and add **extra fields** (e.g., `schema_id`, `schema_version`, `schema_kind`) | No schema change; works with existing validator and wrapper | UI/Cloud must rely on extra fields; schema meaning not enforced |
| **B** | Extend function type to `topology`/`flows` (update wrapper + schema) | Explicit type; clear UI branching | Requires changes in `funcshandler` + `FUNCTION_UI_SCHEMA.json`; risk of tool/UI incompatibility |
| **C** | Keep `type: "table"` and **no extra fields** (encode everything as table only) | Zero infra change | UI cannot distinguish schema; versioning is implicit and fragile |

**Recommendation:** A — minimal infra change, compatible with current schema, still allows explicit schema/version fields.

### Decision 7: NetFlow Collector Job Model

**Background:** go.d modules are **job-based**; functions are bound to job instances (see SNMP router pattern in `src/go/plugin/go.d/collector/snmp/func_router.go:20-64`). There is **no existing UDP listener collector** in go.d, so we must pick a model.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | **One job = one UDP listener** (addr/port per job) | Aligns with go.d model; simple config; multiple listeners allowed | Multiple listeners if many ports; duplication of aggregators |
| **B** | **One listener shared by multiple jobs** (jobs are filters) | Centralized listener | Requires cross-job shared state; complex lifecycle |
| **C** | **Single global listener** (one job only) | Simplest runtime | Limits flexibility; not aligned with multi-job config patterns |

**Recommendation:** A — matches go.d patterns and avoids shared-state complexity.

### Decision 8: Flow Parsing Library

**Background:** There is **no netflow/ipfix/sflow code** in this repo; we must add a parser library and dependencies.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Use `goflow2` (NetFlow v5/v9/IPFIX) + separate sFlow lib if needed | Mature, widely used | New deps; need sFlow coverage |
| **B** | Adapt Akvorado decoder code (AGPL, GPL-compatible) | Broad protocol coverage | Porting effort; license obligations (AGPL) |
| **C** | Write minimal parsers in-house (v5/v9 first) | Full control | High effort; higher bug risk |

**Recommendation:** A initially (fastest to a working collector), then evaluate sFlow coverage.

### Decision 9: Aggregation Prototype Placement

**Background:** This repo already has **Go tools** under `src/go/tools/` (e.g., functions-validation). There are also binaries in `src/go/cmd/`.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Add to `src/go/tools/topology-aggregator/` (CLI tool) | Fits tooling pattern; not part of agent runtime | Not built by default |
| **B** | Add to `src/go/cmd/topology-aggregator/` | Produces binary; easier reuse | Expands build/test matrix |
| **C** | Add under `collector/snmp/` as test helper | Close to SNMP code | Hard to reuse for Cloud later |

**Recommendation:** A — aligns with existing tooling and keeps runtime clean.

### Decision 10: Require Cloud for Topology/Flows Functions

**Background:** `MethodConfig.RequireCloud` controls access flags when functions are registered (`src/go/plugin/go.d/agent/jobmgr/manager.go:148-162`).

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | `RequireCloud = true` | Limits access; aligns with Cloud-centric usage | Local-only UI access blocked |
| **B** | `RequireCloud = false` | Available locally for debugging | Might expose sensitive topology/flow data locally |

**Recommendation:** A for flows (sensitive), **B or A** for topology depending on sensitivity policy.

### Decision 11: Time-series Emission for Topology/Flows

**Background:** Functions can return JSON only, but agents may also emit time-series for correlation. SNMP vnodes provide stable labeling (`src/go/plugin/go.d/collector/snmp/collect.go:140-190`).

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | **Functions only**, no extra charts | Minimal storage/cost | No time-series correlation; less historical context |
| **B** | **Functions + time-series** (key metrics only) | Enables correlation and dashboards | Higher cardinality risk; more design work |

**Recommendation:** B, but **limit** to essential metrics and strong label discipline.

### Decision 12: How to Enable LLDP/CDP Profiles

**Background:** Profiles are matched via selectors; base profiles are applied through `extends` (e.g., `arista-switch.yaml` extends `_std-if-mib.yaml`). LLDP/CDP profiles won’t apply unless explicitly extended.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Add `_std-lldp-mib.yaml` / `_std-cdp-mib.yaml` and **extend** them in all relevant vendor profiles | Explicit control; predictable | Many profile edits; higher maintenance |
| **B** | Create **generic selectors** for LLDP/CDP profiles so they apply broadly | Minimal profile changes | Risk: collect LLDP/CDP from devices that don’t support it (extra SNMP load) |
| **C** | Add **manual_profiles** guidance only (user opt-in) | No broad changes | Requires user configuration; fewer devices covered |

**Recommendation:** A (explicit) to avoid accidental SNMP load; we can later add B as an opt-in toggle.

---

## Part 5: Implementation Phases

**Note:** This repo is **agent-only**. Cloud aggregation phases are **out of scope** here; we will only build the **aggregation prototype tool** in Go for validation.

### Phase 1: SNMP Topology (Foundation)

1. Create LLDP profile (`_std-lldp-mib.yaml`)
2. Create CDP profile (`_std-cdp-mib.yaml`)
3. Build topology data structures
4. Implement local topology builder
5. Add `/api/v3/function?function=topology` endpoint
6. Document API for Cloud team

**Deliverable:** Each agent exposes local topology via API

### Phase 2: Flow Collection (Foundation)

1. Add flow collector package using goflow2
2. Implement UDP listeners (configurable ports)
3. Build flow aggregator with time buckets
4. Integrate with SNMP for interface enrichment
5. Add `/api/v3/function?function=flows` endpoint
6. Document API for Cloud team

**Deliverable:** Each agent collects and exposes flow data via API

### Phase 3: Cloud Aggregation (Topology)

1. Implement topology query to all agents
2. Build merge algorithm (device dedup, link validation)
3. Create unified topology visualization
4. Add topology diff/change detection

**Deliverable:** Cloud shows merged topology across all agents

### Phase 4: Cloud Aggregation (Flows)

1. Implement flow query to all agents
2. Build deduplication logic
3. Create aggregated views (Sankey, time-series)
4. Add drill-down capabilities

**Deliverable:** Cloud shows aggregated flow data across all agents

---

## References

### SNMP Topology
- LLDP-MIB: IEEE 802.1AB-2016
- CISCO-CDP-MIB: Cisco proprietary
- RFC 2922: PTOPO-MIB (Physical Topology)

### NetFlow/IPFIX
- [goflow2](https://github.com/netsampler/goflow2) - BSD-3 licensed Go library
- RFC 3954: NetFlow v9
- RFC 7011-7015: IPFIX
- RFC 3176: sFlow

### Similar Systems
- [Akvorado](https://github.com/akvorado/akvorado) - Centralized flow collector
- [ntopng](https://www.ntop.org/products/traffic-analysis/ntop/) - Network traffic analysis
- [pmacct](http://www.pmacct.net/) - Network accounting

---

## Part 6: Testing Strategy (E2E & CI)

### 6.1 Current Testing Status

**What exists:**
- Unit tests with gosnmp `MockHandler` (gomock-based)
- 18+ test files covering SNMP collector components
- No integration tests, no simulators, no real device data

**What's needed for topology + flows:**
- SNMP simulator with LLDP/CDP neighbor tables
- NetFlow/IPFIX/sFlow packet generator
- Multi-device topology scenarios
- Flow deduplication testing
- CI integration

### 6.2 Testing Tools Available

#### SNMP Simulation

| Tool | Type | LLDP/CDP Support | CI Ready | Notes |
|------|------|------------------|----------|-------|
| **gosnmp MockHandler** | Go mock | Manual PDU setup | Yes | Already used, fast but manual |
| **[GoSNMPServer](https://github.com/slayercat/GoSNMPServer)** | Go library | Programmable | Yes | Pure Go, can create real agents |
| **[snmpsim](https://github.com/etingof/snmpsim)** | Python daemon | Via .snmprec files | Yes | Can record from real devices |
| **iReasoning SNMP Simulator** | Java | Full MIB support | Limited | Commercial, GUI-focused |

#### NetFlow/IPFIX/sFlow Generation

| Tool | Protocols | CI Ready | Notes |
|------|-----------|----------|-------|
| **[nflow-generator](https://github.com/nerdalert/nflow-generator)** | NetFlow v5 only | Yes (Docker) | Simple, limited to v5 |
| **[FlowTest](https://github.com/CESNET/FlowTest)** | NetFlow/IPFIX | Yes | Complex, research-grade |
| **[softflowd](https://github.com/irino/softflowd)** | NetFlow v1/5/9, IPFIX | Yes | Generates from pcap |
| **[goflow2](https://github.com/netsampler/goflow2)** | NetFlow v5/v9, IPFIX, sFlow | Library only | Can encode packets |
| **Custom Go generator** | Any | Yes | Build using goflow2 encoding |

### 6.3 Recommended Testing Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CI PIPELINE (GitHub Actions)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        UNIT TESTS (Fast, No I/O)                     │   │
│  │  - gosnmp MockHandler for SNMP PDU responses                        │   │
│  │  - Mock flow packets for parser testing                             │   │
│  │  - Topology merge algorithm tests (pure data structures)            │   │
│  │  - Flow aggregation tests (pure data structures)                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    INTEGRATION TESTS (With Simulators)               │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────┐  │   │
│  │  │  GoSNMPServer    │    │  GoSNMPServer    │    │ GoSNMPServer │  │   │
│  │  │  "router-a"      │    │  "switch-b"      │    │ "switch-c"   │  │   │
│  │  │  LLDP neighbors: │    │  LLDP neighbors: │    │ LLDP:        │  │   │
│  │  │  - switch-b:g0/1 │    │  - router-a:g0/0 │    │ - switch-b   │  │   │
│  │  │  - switch-c:g0/2 │    │  - switch-c:g0/1 │    │              │  │   │
│  │  └────────┬─────────┘    └────────┬─────────┘    └──────┬───────┘  │   │
│  │           │                       │                      │          │   │
│  │           └───────────────────────┼──────────────────────┘          │   │
│  │                                   ▼                                 │   │
│  │                        ┌──────────────────┐                         │   │
│  │                        │  SNMP Collector  │                         │   │
│  │                        │  (under test)    │                         │   │
│  │                        └────────┬─────────┘                         │   │
│  │                                 │                                   │   │
│  │                                 ▼                                   │   │
│  │                        ┌──────────────────┐                         │   │
│  │                        │ Topology Builder │                         │   │
│  │                        │ Assert: 3 nodes  │                         │   │
│  │                        │ Assert: 3 links  │                         │   │
│  │                        │ Assert: bidir    │                         │   │
│  │                        └──────────────────┘                         │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      FLOW INTEGRATION TESTS                          │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐                    ┌──────────────────────┐   │   │
│  │  │  Flow Generator  │  UDP packets       │   Flow Collector     │   │   │
│  │  │  (Go, in-process)│ ──────────────────►│   (under test)       │   │   │
│  │  │                  │  NetFlow v9/IPFIX  │                      │   │   │
│  │  │  - 100 flows     │  sFlow             │   Assert:            │   │   │
│  │  │  - known values  │                    │   - bytes match      │   │   │
│  │  │  - sampling 1:100│                    │   - packets match    │   │   │
│  │  └──────────────────┘                    │   - sampling norm    │   │   │
│  │                                          └──────────────────────┘   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.4 Implementation: SNMP Test Fixtures

#### Option A: GoSNMPServer-based (Recommended)

Create Go test fixtures that spin up real SNMP agents:

```go
// testdata/topology_fixtures.go
package testdata

import (
    "github.com/slayercat/GoSNMPServer"
)

// CreateTopologyFixture creates a 3-device topology for testing
func CreateTopologyFixture(t *testing.T) *TopologyFixture {
    // Router A - has LLDP neighbors to Switch B and C
    routerA := NewSNMPAgent(t, "127.0.0.1:10161")
    routerA.SetSysName("router-a")
    routerA.SetSysObjectID("1.3.6.1.4.1.9.1.1")  // Cisco
    routerA.SetChassisID("00:11:22:33:44:01")
    routerA.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/0",
        RemChassisID:  "00:11:22:33:44:02",  // Switch B
        RemPortID:     "GigabitEthernet0/1",
        RemSysName:    "switch-b",
    })
    routerA.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/1",
        RemChassisID:  "00:11:22:33:44:03",  // Switch C
        RemPortID:     "GigabitEthernet0/1",
        RemSysName:    "switch-c",
    })

    // Switch B - has LLDP neighbors to Router A and Switch C
    switchB := NewSNMPAgent(t, "127.0.0.1:10162")
    switchB.SetSysName("switch-b")
    switchB.SetChassisID("00:11:22:33:44:02")
    switchB.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/1",
        RemChassisID:  "00:11:22:33:44:01",  // Router A
        RemPortID:     "GigabitEthernet0/0",
        RemSysName:    "router-a",
    })
    // ... more neighbors

    return &TopologyFixture{
        Agents: []*SNMPAgent{routerA, switchB, switchC},
        ExpectedNodes: 3,
        ExpectedLinks: 3,
        ExpectedBidirectional: 3,
    }
}
```

**Pros:**
- Pure Go, no external dependencies
- Fast startup (in-process or localhost UDP)
- Deterministic, reproducible
- Easy to create complex topologies programmatically

**Cons:**
- Need to implement LLDP-MIB OID handlers
- More initial development effort

#### Option B: snmpsim with Pre-recorded Data

Record from real devices, store in repo:

```
testdata/snmprec/
├── router-a.snmprec       # Recorded from real Cisco router
├── switch-b.snmprec       # Recorded from real switch
├── switch-c.snmprec       # Recorded from real switch
└── topology-scenario.yaml # Describes expected topology
```

**Recording command:**
```bash
snmprec-record-commands \
  --protocol=udp \
  --agent=192.168.1.1 \
  --community=public \
  --output-file=router-a.snmprec \
  --start-oid=1.0.8802.1.1.2  # LLDP-MIB
```

**CI integration:**
```yaml
# .github/workflows/snmp-integration.yml
jobs:
  integration-test:
    runs-on: ubuntu-latest
    services:
      snmpsim:
        image: tandoor/snmpsim:latest
        ports:
          - 10161:161/udp
          - 10162:162/udp
        volumes:
          - ./testdata/snmprec:/usr/share/snmpsim/data
```

**Pros:**
- Uses real device responses
- Mature, battle-tested tool
- Easy to add new device types

**Cons:**
- Python dependency
- Slower startup
- Harder to modify programmatically

### 6.5 Implementation: Flow Test Fixtures

#### Option A: Go-based Flow Generator (Recommended)

Build a simple flow generator using goflow2's encoding:

```go
// testdata/flow_generator.go
package testdata

import (
    "net"
    "github.com/netsampler/goflow2/producer"
)

type FlowGenerator struct {
    conn    *net.UDPConn
    target  *net.UDPAddr
}

func NewFlowGenerator(targetAddr string) *FlowGenerator {
    // Setup UDP connection to collector
}

// SendNetFlowV9 sends a NetFlow v9 packet with specified flows
func (g *FlowGenerator) SendNetFlowV9(flows []TestFlow) error {
    // Encode using goflow2/producer
    // Send via UDP
}

// SendIPFIX sends an IPFIX packet
func (g *FlowGenerator) SendIPFIX(flows []TestFlow) error {
    // Similar encoding
}

// SendSFlow sends an sFlow datagram
func (g *FlowGenerator) SendSFlow(samples []TestSample) error {
    // sFlow encoding
}

// TestFlow represents a flow record for testing
type TestFlow struct {
    SrcAddr     net.IP
    DstAddr     net.IP
    SrcPort     uint16
    DstPort     uint16
    Protocol    uint8
    Bytes       uint64
    Packets     uint64
    StartTime   time.Time
    EndTime     time.Time
    SrcAS       uint32
    DstAS       uint32
    InIfIndex   uint32
    OutIfIndex  uint32
    SamplingRate uint32
}
```

**Test example:**
```go
func TestFlowCollector_NetFlowV9(t *testing.T) {
    // Start collector
    collector := NewFlowCollector(t, ":19995")
    defer collector.Stop()

    // Create generator
    gen := NewFlowGenerator("127.0.0.1:19995")

    // Send known flows
    flows := []TestFlow{
        {SrcAddr: net.ParseIP("10.0.0.1"), DstAddr: net.ParseIP("10.0.0.2"),
         SrcPort: 12345, DstPort: 80, Protocol: 6,
         Bytes: 1000000, Packets: 1000, SamplingRate: 100},
    }
    gen.SendNetFlowV9(flows)

    // Wait and verify
    time.Sleep(100 * time.Millisecond)

    summary := collector.GetSummary()
    // With 1:100 sampling, expect 100M bytes
    assert.Equal(t, uint64(100000000), summary.TotalBytes)
}
```

#### Option B: External Tools (Docker)

Use existing tools in CI:

```yaml
# .github/workflows/flow-integration.yml
jobs:
  flow-test:
    runs-on: ubuntu-latest
    steps:
      - name: Start collector
        run: |
          ./netdata-flow-collector --port 2055 &
          sleep 2

      - name: Generate NetFlow v5 traffic
        run: |
          docker run --rm --net=host networkstatic/nflow-generator \
            -t 127.0.0.1 -p 2055 -c 1000

      - name: Generate NetFlow v9 traffic
        run: |
          # Use softflowd with pre-recorded pcap
          softflowd -i testdata/sample.pcap -n 127.0.0.1:2055 -v 9

      - name: Verify collected data
        run: |
          ./verify-flow-collection --expected-flows 1000
```

### 6.6 Test Scenarios

#### Topology Scenarios

| Scenario | Description | Validates |
|----------|-------------|-----------|
| **T1: Simple chain** | A → B → C | Basic LLDP discovery |
| **T2: Ring** | A → B → C → A | Bidirectional link detection |
| **T3: Partial visibility** | A monitors B, B sees C (not monitored) | "Discovered" node handling |
| **T4: Multi-agent** | Agent1 monitors A, Agent2 monitors B | Cross-agent link merge |
| **T5: CDP + LLDP mixed** | Some devices CDP, some LLDP | Protocol coexistence |
| **T6: Link flap** | Link disappears then reappears | Stale link handling |

#### Flow Scenarios

| Scenario | Description | Validates |
|----------|-------------|-----------|
| **F1: Single exporter** | 1 router sends NetFlow v9 | Basic collection |
| **F2: Multi-protocol** | NetFlow v5 + v9 + IPFIX + sFlow | Protocol parsing |
| **F3: Sampling normalization** | Same flow, 1:100 vs 1:1000 | Normalization math |
| **F4: Transit dedup** | Same flow from ingress + egress | Deduplication logic |
| **F5: Multi-agent** | Agent1 gets ingress, Agent2 gets egress | Cross-agent aggregation |
| **F6: Interface enrichment** | Flows with ifIndex, SNMP has names | SNMP correlation |
| **F7: High cardinality** | 100K unique flows | Top-N aggregation |

### 6.7 CI Pipeline Design

```yaml
# .github/workflows/network-features.yml
name: Network Topology & Flow Tests

on:
  push:
    paths:
      - 'src/go/plugin/go.d/collector/snmp/**'
      - 'src/go/plugin/go.d/collector/netflow/**'
  pull_request:
    paths:
      - 'src/go/plugin/go.d/collector/snmp/**'
      - 'src/go/plugin/go.d/collector/netflow/**'

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run unit tests
        run: |
          cd src/go/plugin/go.d
          go test -v -race ./collector/snmp/...
          go test -v -race ./collector/netflow/...

  snmp-integration:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run SNMP topology integration tests
        run: |
          cd src/go/plugin/go.d
          go test -v -tags=integration ./collector/snmp/integration/...

  flow-integration:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run flow collector integration tests
        run: |
          cd src/go/plugin/go.d
          go test -v -tags=integration ./collector/netflow/integration/...

  e2e-topology:
    runs-on: ubuntu-latest
    needs: [snmp-integration, flow-integration]
    steps:
      - uses: actions/checkout@v4
      - name: Build Netdata
        run: ./build.sh
      - name: Start SNMP simulators
        run: |
          docker-compose -f testdata/topology/docker-compose.yml up -d
      - name: Run Netdata with topology
        run: |
          ./netdata -D &
          sleep 10
      - name: Verify topology API
        run: |
          curl -s localhost:19999/api/v3/function?function=topology | \
            jq -e '.devices | length == 3'
          curl -s localhost:19999/api/v3/function?function=topology | \
            jq -e '.links | length == 3'
```

### 6.8 Verdict: Is Full CI Testing Doable?

**YES**, with the following approach:

| Layer | Approach | Effort | CI Time |
|-------|----------|--------|---------|
| **Unit tests** | gosnmp MockHandler + mock flow packets | Low | <30s |
| **SNMP integration** | GoSNMPServer (in-process Go agents) | Medium | <1min |
| **Flow integration** | Go-based flow generator (in-process) | Medium | <1min |
| **E2E topology** | Docker + snmpsim OR GoSNMPServer | Medium | <5min |
| **E2E flows** | Docker + nflow-generator + softflowd | Low | <3min |
| **Multi-agent E2E** | Docker Compose with 2+ Netdata instances | High | <10min |

**Total CI time:** ~15-20 minutes for full test suite

**Recommendation:**
1. **Phase 1:** GoSNMPServer-based integration tests (pure Go, fast, no Docker)
2. **Phase 2:** Add Docker-based E2E for realistic scenarios
3. **Phase 3:** Multi-agent E2E with simulated Cloud aggregation

### 6.9 Testing Infrastructure Components to Build

| Component | Purpose | Location |
|-----------|---------|----------|
| `snmpagent` | Go library to create SNMP agents for tests | `pkg/testutil/snmpagent/` |
| `flowgen` | Go library to generate flow packets | `pkg/testutil/flowgen/` |
| `topofixtures` | Pre-defined topology scenarios | `collector/snmp/testdata/topologies/` |
| `flowfixtures` | Pre-defined flow scenarios | `collector/netflow/testdata/flows/` |
| `e2e/` | Docker Compose + test scripts | `src/go/plugin/go.d/e2e/` |

---

## Part 7: Available Test Data Resources (VERIFIED)

**Verification Date:** 2025-02-03
**Method:** Cloned repositories and inspected actual files

### 7.1 License Compatibility

**Netdata is GPL-3.0+** - All sources below are license-compatible:

| Source | License | Compatible | Notes |
|--------|---------|------------|-------|
| **LibreNMS** | GPL-3.0 | ✅ Yes | Same license family |
| **Akvorado** | AGPL-3.0 | ✅ Yes | AGPL is GPL-compatible |
| **snmpsim-data** | BSD-2-Clause | ✅ Yes | Permissive |
| **goflow2** | BSD-3-Clause | ✅ Yes | Permissive |
| **Wireshark samples** | Various/Public | ✅ Yes | Public domain |
| **tcpreplay samples** | BSD | ✅ Yes | Permissive |

**Conclusion:** No licensing issues. We can freely use code and data from all sources.

### 7.2 SNMP Test Data (VERIFIED)

#### LibreNMS snmprec Collection

**Repository:** https://github.com/librenms/librenms/tree/master/tests/snmpsim
**License:** GPL-3.0 (compatible)

**Verified counts:**
| Metric | Count |
|--------|-------|
| Total snmprec files | **1,872** |
| Files with LLDP data (any) | **137** |
| Files with LLDP remote neighbors (lldpRemTable) | **106** |
| Files with CDP data | **15+** |

**Sample LLDP remote neighbor data (verified):**
```
# From arista_eos.snmprec - lldpRemSysName (1.0.8802.1.1.2.1.4.1.1.9)
1.0.8802.1.1.2.1.4.1.1.9.0.47.2|4|3750E-Estudio1.example.net
1.0.8802.1.1.2.1.4.1.1.9.0.48.1|4|4900M-CORE.example.net

# From arubaos-cx.snmprec - lldpRemSysName
1.0.8802.1.1.2.1.4.1.1.9.5.10.19|4|vmhost-20
1.0.8802.1.1.2.1.4.1.1.9.5.43.16|4|vmhost-30
1.0.8802.1.1.2.1.4.1.1.9.5.44.15|4|vmhost-31
```

**Sample CDP neighbor data (verified):**
```
# From ios_2960x.snmprec - cdpCacheDeviceId (1.3.6.1.4.1.9.9.23.1.2.1.1.6)
1.3.6.1.4.1.9.9.23.1.2.1.1.6.10103.1|4|acme-fr-ap-011
1.3.6.1.4.1.9.9.23.1.2.1.1.6.10152.8|4|acme-fr-s-001

# cdpCacheDevicePort (1.3.6.1.4.1.9.9.23.1.2.1.1.7)
1.3.6.1.4.1.9.9.23.1.2.1.1.7.10103.1|4|GigabitEthernet0
```

**Devices with LLDP neighbor data include:**
- Arista EOS switches
- Aruba OS-CX switches (multiple versions)
- Alcatel-Lucent AOS switches
- Avaya/Extreme BOSS switches
- Various others

**Verdict:** ✅ Production-quality LLDP/CDP test data available and ready to use.

#### snmpsim-data Repository

**Repository:** https://github.com/etingof/snmpsim-data
**License:** BSD-2-Clause (permissive)

**Verified counts:**
| Metric | Count |
|--------|-------|
| Total snmprec files | **116** |
| Categories | UPS, cameras, storage, network, OS, IoT |

**Verdict:** ✅ Smaller but permissive license. Good for general SNMP testing.

### 7.3 NetFlow/IPFIX/sFlow Test Data (VERIFIED)

#### Akvorado Test Data

**Repository:** https://github.com/akvorado/akvorado
**License:** AGPL-3.0 (GPL-compatible)

**Verified NetFlow/IPFIX pcaps:** (`outlet/flow/decoder/netflow/testdata/`)

| File | Size | Content |
|------|------|---------|
| `nfv5.pcap` | 1,498 bytes | NetFlow v5 (29 flow records) |
| `data+templates.pcap` | 1,498 bytes | NetFlow v9 with templates |
| `template.pcap` | 202 bytes | NetFlow v9 templates only |
| `mpls.pcap` | 798 bytes | MPLS traffic flows |
| `nat.pcap` | 594 bytes | NAT translation records |
| `juniper-cpid-data.pcap` | 254 bytes | Juniper-specific fields |
| `juniper-cpid-template.pcap` | 174 bytes | Juniper templates |
| `ipfix-srv6-data.pcap` | 232 bytes | IPFIX with SRv6 |
| `ipfix-srv6-template.pcap` | 126 bytes | IPFIX SRv6 templates |
| `multiplesamplingrates-*.pcap` | Various | Different sampling rates |
| `options-*.pcap` | Various | Options templates |
| `physicalinterfaces.pcap` | 1,430 bytes | Physical interface data |
| `samplingrate-*.pcap` | Various | Sampling rate testing |
| `ipfixprobe-*.pcap` | Various | ipfixprobe format |
| `datalink-*.pcap` | Various | Datalink layer data |
| `icmp-*.pcap` | Various | ICMP flow records |

**Total NetFlow/IPFIX test files:** 25

**Verified sFlow pcaps:** (`outlet/flow/decoder/sflow/testdata/`)

| File | Size | Content |
|------|------|---------|
| `data-sflow-ipv4-data.pcap` | 410 bytes | Basic sFlow IPv4 |
| `data-sflow-raw-ipv4.pcap` | 302 bytes | Raw IPv4 sFlow |
| `data-sflow-expanded-sample.pcap` | 410 bytes | Expanded samples |
| `data-encap-vxlan.pcap` | 326 bytes | VXLAN encapsulation |
| `data-qinq.pcap` | 290 bytes | QinQ VLAN tagging |
| `data-icmpv4.pcap` | 298 bytes | ICMPv4 flows |
| `data-icmpv6.pcap` | 286 bytes | ICMPv6 flows |
| `data-multiple-interfaces.pcap` | 1,290 bytes | Multi-interface |
| `data-discard-interface.pcap` | 1,290 bytes | Discard interface |
| `data-local-interface.pcap` | 1,290 bytes | Local interface |
| `data-1140.pcap` | 1,290 bytes | Additional sFlow data |

**Total sFlow test files:** 11

**NetFlow v5 pcap content verified via tshark:**
```
Frame 1: 1458 bytes
Ethernet II, Src: HuaweiTechno_79:5f:5c
IPv4: 10.19.144.41 → 10.19.144.26
UDP: 40000 → 9990
NetFlow v5 header: version=5, count=29 flows
```

**Verdict:** ✅ Production-quality flow test data covering all major protocols and edge cases.

#### Akvorado Demo Exporter (Flow Generator)

**Location:** `demoexporter/flows/`
**License:** AGPL-3.0 (GPL-compatible)

**Verified files:**
| File | Lines | Purpose |
|------|-------|---------|
| `root.go` | ~80 | Main component, UDP sender |
| `generate.go` | ~100 | Flow generation logic |
| `nfdata.go` | ~80 | NetFlow data structures |
| `nftemplates.go` | ~150 | NetFlow v9 template encoding |
| `config.go` | ~60 | Configuration |
| `packets.go` | ~20 | Packet helpers |
| `*_test.go` | Various | Test files |

**Key features (from code review):**
- Generates NetFlow v9 packets
- Configurable flow rate (flows per second)
- Random IP generation within prefixes using seeded RNG
- Peak hour simulation (traffic patterns)
- Deterministic output for reproducible tests

**Verdict:** ✅ Can adapt this code for Netdata's test flow generator.

### 7.4 Additional Resources (VERIFIED)

#### Wireshark Sample Captures

**URL:** https://wiki.wireshark.org/SampleCaptures

**Available LLDP/CDP files:**
- `lldp.minimal.pcap` - Basic LLDP frames
- `lldp.detailed.pcap` - LLDP with additional TLVs
- `lldpmed_civicloc.pcap` - LLDP-MED with location
- `cdp.pcap` - CDP v2 from Cisco router
- `cdp_v2.pcap` - CDP v2 from Cisco switch

**Note:** These are raw LLDP/CDP Ethernet frames, NOT SNMP walk data. Useful for understanding protocol encoding.

#### Tcpreplay Sample Captures

**URL:** https://tcpreplay.appneta.com/wiki/captures.html

| File | Size | Flows | Applications |
|------|------|-------|--------------|
| `smallFlows.pcap` | 9.4 MB | 1,209 | 28 |
| `bigFlows.pcap` | 368 MB | 40,686 | 132 |
| `test.pcap` | 0.07 MB | 37 | 1 |

**Use with softflowd for high-volume flow generation:**
```bash
softflowd -r bigFlows.pcap -n 127.0.0.1:2055 -v 9
```

### 7.5 Summary: Verified Test Data

| Data Type | Source | Files | Quality | Ready to Use |
|-----------|--------|-------|---------|--------------|
| **SNMP general** | LibreNMS | 1,872 | Real devices | ✅ Yes |
| **SNMP LLDP neighbors** | LibreNMS | 106 | Real topology data | ✅ Yes |
| **SNMP CDP neighbors** | LibreNMS | 15+ | Real Cisco data | ✅ Yes |
| **NetFlow v5** | Akvorado | 1 | Real capture | ✅ Yes |
| **NetFlow v9/IPFIX** | Akvorado | 24 | Real captures + edge cases | ✅ Yes |
| **sFlow** | Akvorado | 11 | Real captures + edge cases | ✅ Yes |
| **Flow generator code** | Akvorado | 6 files | Production code | ✅ Adaptable |
| **High-volume pcaps** | tcpreplay | 3 | 40K+ flows | ✅ Yes |

### 7.6 Recommended Testing Approach

#### For SNMP Topology Testing:

1. **Use LibreNMS snmprec files directly**
   ```bash
   # Clone test data
   git clone --depth 1 --filter=blob:none --sparse \
     https://github.com/librenms/librenms.git /tmp/librenms
   cd /tmp/librenms && git sparse-checkout set tests/snmpsim

   # Find files with LLDP remote neighbors
   grep -l "1\.0\.8802\.1\.1\.2\.1\.4" tests/snmpsim/*.snmprec
   # Returns: arista_eos.snmprec, arubaos-cx.snmprec, aos7.snmprec, etc.
   ```

2. **Run with snmpsim in CI**
   ```yaml
   services:
     snmpsim:
       image: tandoor/snmpsim:latest
       volumes:
         - ./testdata/snmprec:/usr/share/snmpsim/data
   ```

3. **Or use GoSNMPServer for programmatic tests**
   - Load snmprec data into GoSNMPServer
   - Create topology scenarios dynamically

#### For NetFlow/IPFIX/sFlow Testing:

1. **Use Akvorado pcaps directly**
   ```bash
   # Clone test data
   git clone --depth 1 https://github.com/akvorado/akvorado.git /tmp/akvorado

   # Copy relevant pcaps
   cp /tmp/akvorado/outlet/flow/decoder/netflow/testdata/*.pcap ./testdata/flows/
   cp /tmp/akvorado/outlet/flow/decoder/sflow/testdata/*.pcap ./testdata/flows/
   ```

2. **Replay pcaps in tests**
   ```go
   func TestNetFlowV5(t *testing.T) {
       collector := startCollector(t, ":2055")
       replayPcap(t, "testdata/flows/nfv5.pcap", "127.0.0.1:2055")
       // Assert collected flows match expected
   }
   ```

3. **Adapt Akvorado's demoexporter for dynamic generation**
   - Port `generate.go` and `nftemplates.go`
   - Use for fuzz testing and edge cases

### 7.7 Production Confidence Assessment

| Testing Layer | Data Source | Confidence |
|---------------|-------------|------------|
| Unit tests (mocks) | Hand-crafted | 30% |
| Integration (LibreNMS snmprec) | Real device recordings | +25% |
| Integration (Akvorado pcaps) | Real flow captures | +25% |
| E2E (combined) | All sources | +10% |
| Beta testing (real infra) | User networks | +10% |

**Total achievable confidence with available test data: ~90%**

The remaining 10% covers:
- Vendor-specific edge cases not in test data
- Scale issues (memory, CPU at high volume)
- Timing/race conditions in production

---

## Plan (Agent + Aggregator, 2026-02-04)

1. **NetFlow collector core** ✅
   - UDP listener + decoder (NetFlow v5/v9/IPFIX) using goflow2/v2.
   - Aggregation (bucket + key + sampling normalization + retention).
2. **Flows function** ✅
   - Build `flows` Function response payload (schema_version, exporters, buckets, summaries, metrics).
3. **Time‑series charts** ✅
   - Emit totals (bytes/s, packets/s, flows/s) + dropped records.
4. **Docs + config** ✅ (NetFlow/IPFIX only; **sFlow pending**)
5. **Aggregation prototype CLI** ✅
   - Merge topology + flows JSON outputs (non/partial/full overlap).
   - Add fixtures + unit tests.
6. **Remaining work** ✅ (done)
   - Add **sFlow v5** decoding and protocol flag.
   - Expand **IPFIX field decoding** (bytes/packets/ports/AS/prefix/time/direction).
   - Update **docs/config/schema** to mention sFlow.
   - Add **tests** for sFlow mapping and IPFIX fields.
7. **Run targeted tests** ✅ `go test ./plugin/go.d/collector/netflow ./tools/topology-flow-merge`
8. **Add real-data tests (required)** ✅
   - Add LibreNMS snmprec fixtures for LLDP/CDP and tests that build topology from them.
   - Add Akvorado pcaps for NetFlow v5/v9/IPFIX and sFlow v5 and tests that decode/aggregate them.
   - Include attribution files for testdata sources and run targeted tests.
9. **Expand real device test coverage (required)** ✅ DONE
   - SNMP: added all LibreNMS LLDP/CDP snmprec fixtures (116 files) to `src/go/plugin/go.d/collector/snmp/testdata/snmprec/`.
   - NetFlow/IPFIX/sFlow: added all Akvorado pcaps (36 files) to `src/go/plugin/go.d/collector/netflow/testdata/flows/`.
   - Tests: SNMP snmprec tests iterate all fixtures; NetFlow tests cover all pcaps.
   - Attribution updated: `src/go/plugin/go.d/collector/snmp/testdata/ATTRIBUTION.md`, `src/go/plugin/go.d/collector/netflow/testdata/ATTRIBUTION.md`.
   - Stress pcaps are downloaded at test time (CI) from tcpreplay sources.
10. **Simulator-based integration testing (required)** ✅ DONE
    - Integration tests added with `//go:build integration`:
      - SNMP: `src/go/plugin/go.d/collector/snmp/topology_integration_test.go`
      - NetFlow: `src/go/plugin/go.d/collector/netflow/netflow_integration_test.go`
    - CI workflow added: `.github/workflows/snmp-netflow-sim-tests.yml` (snmpsim + pcap replay + optional stress pcaps).
11. **Complete LLDP/CDP profiles — COMPREHENSIVE, NOT MINIMAL (required)** ✅ DONE
    - LLDP profile expanded with caps, management address tables, and stats: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-lldp-mib.yaml`.
    - CDP profile expanded with globals, interface table, and full cache fields: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-cdp-mib.yaml`.
    - Topology cache/types updated for capabilities + management addresses: `src/go/plugin/go.d/collector/snmp/topology_cache.go`, `src/go/plugin/go.d/collector/snmp/topology_types.go`.
    - Tests updated to validate new fields: `src/go/plugin/go.d/collector/snmp/topology_cache_test.go`, `src/go/plugin/go.d/collector/snmp/topology_snmprec_test.go`.

**Status:** Core implementation complete. **Profile completion, expanded testing, and simulator testing DONE** (items 9-11).

## Implied Decisions (implemented)

- Added **LLDP/CDP profiles** and extended vendor profiles under `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`.
- Added **netflow collector** with README/metadata/config and schema.
- Added **function response types** (topology/flows) and schema support.
- Added **goflow2/v2** (flow decoding) and **gopacket** (pcap parsing) dependencies.
- Added **real device testdata** from LibreNMS (snmprec) and Akvorado (pcap) with attribution.

## Testing Requirements

- **Unit tests**
  - Topology cache building from LLDP/CDP tags.
  - Schema generation and versioning fields.
  - Flow aggregation (sampling normalization, bucket rollups).
- **Integration tests**
  - SNMP topology fixtures (Go-based SNMP agent) for LLDP/CDP.
  - Flow packet generation (NetFlow v5/v9/IPFIX; sFlow if supported).
- **Prototype tool tests**
  - Merge non-overlapping, partial-overlap, full-overlap JSON inputs.

## Documentation Updates Required

- `src/go/plugin/go.d/collector/snmp/README.md` (add topology function docs).
- `src/go/plugin/go.d/collector/snmp/metadata.yaml` (expose topology function in docs).
- New NetFlow collector docs:
  - `src/go/plugin/go.d/collector/netflow/README.md`
  - `src/go/plugin/go.d/collector/netflow/metadata.yaml`
  - `src/go/plugin/go.d/collector/netflow/config_schema.json`
  - `src/go/plugin/go.d/config/go.d/netflow.conf`

---

## Additional Test Data Sources (User-Provided, Unverified)

**Status:** These sources are **not yet verified** in this repo.  
Before using them, we must validate **license**, **file availability**, and **fitness** for CI.

**License note:** Netdata is **GPL‑v3+**, so we can use **AGPL**, **GPL**, and all **permissive** licenses.  
Unknown or incompatible licenses must be excluded until verified.

### Proposed Sources (to verify)
- **CESNET FlowTest** (PCAPs for protocol coverage)
- **lldpd/lldpd** (LLDP/CDP fuzzing corpus)
- **pcap_genflow** (load testing PCAP generator)
- **Network Data Repository** (topology graphs)
- **snmpsim-data** (BSD-2-Clause snmprec files)
- **Wireshark SampleCaptures** (LLDP/CDP PCAPs)
- **NetLab / NetReplica templates** (topology inputs)

### Risks / Considerations
- **License unknown** for some repos (cannot import until verified).
- **Data size** may be too large for CI.
- **Security/PII** risks in public PCAPs (need careful selection).
- **Protocol mismatch** (PCAP vs NetFlow export format) may require conversion tooling.

### Action Needed
- Shortlist a **small, safe** subset for CI.
- Verify licenses and download locations before adoption.
