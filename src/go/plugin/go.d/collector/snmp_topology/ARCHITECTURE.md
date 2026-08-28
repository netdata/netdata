# SNMP Topology Architecture

This is a maintainer-oriented map of `snmp_topology`. It explains the main
runtime path and package boundaries. It intentionally avoids per-OID and
per-protocol details; those live in the profile definitions and focused tests.

## Short Version

`snmp_topology` is a single-instance go.d collector that periodically builds an
immutable topology generation from SNMP jobs registered by the SNMP collector.

It has three independent entry points:

- `Run(ctx)` refreshes topology in the background.
- `Collect(ctx)` only publishes internal collector metrics.
- `snmp:topology:snmp` serves the latest cached topology through a Function.

Function calls do not walk SNMP. They acquire one already-published generation,
build a graph, shape/enrich it, and render `netdata.topology.v1`.

## Runtime Order

```mermaid
flowchart LR
    Init["collector/init.go"]
    Store["ddsnmp.DeviceStore"]
    TrapHandle["TrapEnrichmentHandle"]
    ReverseDNS["reversedns.Resolver"]
    SNMP["snmp collector"]
    Topology["snmp_topology collector"]
    Traps["snmp_traps collector"]

    Init --> Store
    Init --> TrapHandle
    Init --> ReverseDNS
    Init --> SNMP
    Init --> Topology
    Init --> Traps
    ReverseDNS --> Topology
    ReverseDNS --> Traps
    SNMP -->|"committed lifecycle and device state"| Store
    Store -->|"device connection state"| Topology
    Topology -->|"trap topology enrichment"| TrapHandle
    TrapHandle -->|"interface/device context"| Traps
```

```text
collector/init.go
  creates shared SNMP-family state:
    ddsnmp.DeviceStore
    snmp_topology.TrapEnrichmentHandle
    pkg/reversedns.Resolver

  registers:
    snmp          -> writes device connection state
    snmp_topology -> reads device state and publishes topology
    snmp_traps    -> reads topology enrichment for trap logs
```

`snmp_topology` is registered with `InstancePolicySingle`, so go.d runs one
collector instance for the whole agent.

## Refresh Loop

`collector.go` owns lifecycle and scheduling.

```mermaid
flowchart TD
    Run["Run(ctx)"]
    TrapPublish["publish trap enrichment"]
    Tick["initial refresh, then every update_every"]
    Devices["read registered jobs from DeviceStore"]
    Fresh{"next retry/refresh due?"}
    Resolve["resolve DNS targets<br/>up to 8 workers, shared 5s budget"]
    Walk["SNMP walk topology profiles"]
    Next["build mutable device state off-registry"]
    Freeze["freeze immutable DeviceSnapshot"]
    Activate["activate snapshots at publication"]
    GenPublish["publish one TopologyGeneration"]
    Prune["prune unregistered job state"]
    Collect["Collect(ctx)"]
    Metrics["write internal metrics only"]

    Run --> TrapPublish --> Tick --> Devices --> Fresh
    Fresh -->|"yes"| Resolve --> Walk --> Next --> Freeze --> Prune --> Activate --> GenPublish --> Tick
    Fresh -->|"no"| Prune
    Collect --> Metrics
```

```text
Run(ctx)
  publish trap enrichment handle
  refreshTopologyRecovering(ctx)       # immediate first refresh
  every update_every:
    refreshTopologyRecovering(ctx)

refreshTopology(ctx)
  read SNMP job entries and store-owned registration IDs from ddsnmp.DeviceStore
  clone the current per-job refresh state for this sweep
  build a plan of jobs whose next retry/refresh is due
  resolve planned DNS targets with up to eight workers under one shared 5s budget
  for each planned job, in registration-ID order:
    assign the next per-incarnation attempt ordinal
    refreshDeviceTopology(ctx, attemptID, device, targetResolutionEvidence, perDeviceAcquisitionLimits)
    update lastAttempt, lastSuccess, nextRetry, outcome, and failure count
  prune state for jobs no longer registered
  activate successful snapshots with one publication-based freshness deadline
  atomically publish one immutable TopologyGeneration for the complete sweep

refreshDeviceTopology(ctx, attemptID, device, targetResolutionEvidence, acquisitionLimits)
  start one bounded acquisition-attempt envelope and its main collection context
  connect to the device with gosnmp
  select topology profiles
  collect topology ProfileMetrics with ddsnmpcollector and receive one terminal acquisition report per selected profile
  query sysUpTime
  build a fresh mutable per-device builder with private target candidates
  ingest topology metrics into the builder
  collect VTP VLAN contexts into distinct acquisition contexts when needed
  filter target and observed addresses with collected masks, select one management IP, and finalize diagnostics
  freeze one immutable DeviceSnapshot with its exact acquisition time and successful attempt evidence
```

The refresh loop checks devices every `update_every` seconds, but a device is
fully refreshed only when its normal refresh or failure retry is due. A failed
attempt retries after `update_every`, doubles the delay after each consecutive
failure, and caps at `refresh_every`. Success resets the failure count and
restores the normal interval. Retry timing is internal and has no public tuning
option. Retryable client-construction, connection, and collection warnings use
the logger's built-in hourly limiter keyed by registration ID and
bounded failure class; warning suppression does not change retry timing.

`DeviceStore` keeps each caller-owned key private and assigns a typed, monotonic
registration ID to each uninterrupted registration lifetime. Updating a live
registration retains its ID; unregistering and registering the same owner key
again receives a new ID. Refresh ownership uses that ID, not the owner key or a
rebuilt `hostname:port` key. Two SNMP jobs targeting the same endpoint therefore
keep independent refresh state and device generations, and a replacement job
cannot inherit the removed job's retry or warning-limiter state.

Job Manager projects a credential-free normal-SNMP baseline from each committed
running/failed configuration. The baseline has an unknown phase when vnode,
secret, or configuration application fails before a runtime collector exists.
When construction succeeds, the collector records lifecycle phase/outcome
locally from `Init`, before system information or profile selection can succeed,
and Job Manager uses that detached value instead. Publication occurs only after
the matching DynCfg graph row commits, or after a successful transaction confirms
that its fallback row already matches. Failed candidate cleanup therefore cannot
erase the incumbent or publish the candidate. Graph reconciliation owns
connection demotion, incarnation replacement, and full removal; managed
collector cleanup does not mutate the shared `DeviceStore`.

`LifecycleCut` provides a separately sequenced and timestamped snapshot of every
committed job's credential-free configured identity, last completed lifecycle
phase/outcome, and topology-readiness state. Connection state collected before
graph commit is held once by the collector. A successfully accepted runtime is
consulted transiently during graph reconciliation so that state is flushed
atomically with the lifecycle row; the detached snapshot never retains the
collector, its configuration, or its SNMP client. Later updates retain the same
registration ID. `Entries` remains limited to topology-ready jobs. Lifecycle
reporting is diagnostic-only: failures or panics are rate-limited and cannot
change collector or graph results, while a panic in `Init` or `Check` is recorded
as a failed phase before the framework recovers it.

Only due DNS targets enter the lookup phase; IP literals bypass the resolver.
The workers are joined before SNMP collection begins, stop with the refresh
context, and use a lookup-only child context, so expiry of the shared lookup
budget does not cancel the parent refresh or the subsequent serial SNMP walks.
All normalized DNS answers remain private refresh evidence until collection has
provided interface masks. Finalization rejects mask-proven network and broadcast
addresses, then uses one selector across the surviving targets, LLDP/CDP
addresses, and IP-MIB addresses. Only the selected target enters public identity
or trap matching; alternate DNS answers are never published as aliases.

The important safety property is two-level immutability. A device refresh builds
and freezes its next collected snapshot off-registry. At the completed sweep
boundary, successful snapshots are activated with one shared publication time,
then the collector publishes the complete device vector with one atomic pointer
update. Function, focus-option, availability, reverse-DNS warming, and trap
readers each acquire one generation and cannot combine devices from different
sweeps. A failed attempt retains the last successful device generation and its
original freshness deadline; a canceled or panicking sweep does not publish a
partial vector.

Diagnostics preserve two intentionally independent inventory cuts:

- the current `DeviceStore` lifecycle cut, which can advance when a normal SNMP
  job initializes, checks, collects, becomes topology-ready, or exits;
- the topology sweep cut attached to the same immutable generation as the
  Function-visible device vector.

The topology cut's ordered device rows are the exact start-of-sweep registration inventory. Each row separates whether
the job was selected, its committed outcome/retry state, its retained successful acquisition, its latest completed
attempt, and whether the retained generation is renderable or expired. The two acquisition pointers alias when the
latest attempt succeeded; a later failure or no-profile attempt replaces only the latest-attempt pointer. Registrations
removed since the preceding cut are recorded separately. A canceled or panicking sweep leaves both the published
topology generation and its cut unchanged and replaces only one bounded last-aborted marker. That marker records the
sweep phase and, during device refresh, the active registration ID.

Per-device acquisition limits are applied while an attempt is projected. One serial global latest-view
record/logical-byte counter then admits distinct retained captures in registration order. A retained successful
generation is admitted before a different latest failure or no-profile attempt, preserving replayable evidence when
only one fits. Aliased captures count once. Evidence that does not fit is replaced by an explicit limit marker;
collection and topology publication continue. Lifecycle rows are admitted independently against the remaining budget
when diagnostics are acquired. There are no reservations, sessions, queues, or attempt ledgers.

## Topology Profile Composition

Topology profile selection uses the shared SNMP profile catalog. Matching is
additive: a device receives the combined topology rows from every matching
selector and each profile's `extends` graph. The topology projection then keeps
only topology-consumer fields and typed `topology:` rows.

`ddsnmpcollector` exposes an optional synchronous acquisition observer. When enabled, it emits one terminal report for
each selected profile, including preparation or table failures. A stable profile ordinal and route digest identify the
configured logical acquisition units and distinguish processed, not-observed, empty, dependency-rejected,
tag-rejected, partial, and failed acquisition. Reports cover topology rows, BGP rows, profile tags, and metadata used by
the topology consumer; ordinary metric rows remain live collector inputs but are not diagnostic routes. Shared canonical
WALKs remain a transport optimization: each logical unit keeps its configured root and counts only varbinds below that
root. Compact route/row/value references join the
synchronously borrowed topology and BGP results to their configured producing unit. These reports do not claim to
reproduce the lower-level GET/WALK execution graph. Profile source paths, raw packets, copied decoded values, transform
definitions, and error text are excluded. With no observer, profile digests, route reports, and value references are not
built.

BGP evidence keeps one logical unit per configured BGP row definition. Its digest covers the main table name/root and
every configured identity, descriptor, signal, tag source, and cross-table dependency. `Missing` counts configured scalar
OIDs already classified unavailable by collector state. Table rows without required identity/signals are rejected.
Optional absent descriptors are not missing; cross-table failures that reject a row or tag use the dependency class.
Synthetic table-dependency units have no semantic rows, so they report zero rows and count only received varbinds below
their configured root as values.

The standard capabilities have separate owners:

- `generic-device.yaml` and `generic-ups.yaml` extend
  `_std-topology-ip-mib.yaml`. This baseline walks the legacy IPv4
  `ipAddrTable` and provides address, interface-index, and netmask facts for L3
  subnet enrichment. The address comes from the indexed row suffix; the `.2`
  ifIndex and `.3` netmask columns are the required readable PDUs.
- `_std-topology-interface-mib.yaml` owns interface identity and state.
- `_std-topology-bridge-base-mib.yaml` owns bridge identity and bridge-port to
  ifIndex mapping.
- Classic FDB, Q-BRIDGE, STP, ARP, and Cisco VTP each have independent mixins.
  Product profiles inherit only the capabilities justified by their role and
  available MIB rows. Selector-only `topology-role-*.yaml` profiles fill exact
  or qualified capability gaps without attaching unrelated vendor metrics.

This separation is why an L3-only device can participate in logical subnet
topology without being polled as a bridge. Conversely, FDB and ARP are not
attempted on every generic SNMP device: their tables can be high-cardinality and
their graph semantics are role-specific.

When `sysObjectID` is unavailable, profile resolution uses only the configured
manual profiles. Such jobs must list `generic-device` explicitly when they need
the baseline IPv4 topology rows, for example
`manual_profiles: [vendor-profile, generic-device]`.

For a topology table, the configured symbol is a structural row-presence
anchor. An existing PDU emits the tagged observation with an internal value of
zero, including when the PDU is an OctetString. Cache ingestion consumes the
tags; scalar topology values retain normal value semantics.

## Per-Device Builder And Generation

`topology_cache.go` defines the mutable, collection-only builder for one SNMP
job. It is never published to runtime readers and needs no synchronization.

The builder stores normalized intermediate facts collected from topology profile
metrics:

- local device identity and metadata;
- interfaces, interface status, IP-to-interface mappings;
- LLDP and CDP neighbors;
- FDB, bridge-port, VLAN, STP, ARP/ND data;
- L3 interface addresses;
- OSPF neighbors;
- BGP peers.

Ingestion is split by source area:

- `topology_cache_lldp.go`
- `topology_cache_cdp.go`
- `topology_cache_fdb.go`
- `topology_cache_interfaces.go`
- `topology_cache_stp_arp.go`
- `topology_l3_interfaces.go`
- `topology_ospf_neighbors.go`
- `topology_bgp_peers.go`
- `topology_vlan_context_*.go`

Every completed device attempt retains a bounded acquisition envelope when projection succeeds. The envelope records its
registration/attempt identity, target-resolution outcome and safe addresses, closed outcomes for the outer collection
phases, and ordered main/VLAN collection contexts. Each context contains the collector's terminal per-profile route
report and, for replayable profiles, one immutable copy of the topology-consumer values needed for replay. The report's
child references preserve
the producing route and local row/value position for each retained topology or BGP value. Failed and no-profile attempts
remain diagnostic; a successful attempt is also owned by the published device generation.

The live builder and the replay path share one ordered event dispatcher for system uptime, profile tags, topology rows,
BGP rows, and successful VLAN-context rows. The retained values use positive per-event/per-topology-kind field
allowlists and are copied synchronously from the collector's borrowed result. They keep only metadata, tags, topology
rows, and BGP fallback tags consumed by those builder operations. Non-VLAN rows in VLAN events, credentials, profile
source paths, metric names, ordinary metric values, transform definitions, raw packets, and error text are not copied.
Retained decoded strings are exact-sized copies so a small retained substring cannot keep a larger SNMP response buffer
alive outside the logical-byte limit. Stable schema/profile tag keys may remain shared because they are not decoded
response data and their owners outlive the capture.

Acquisition capture has direct per-device record and logical-byte limits. Values from failed profiles are not retained
because replay skips those profiles. Limit exhaustion, projection errors, or
projection panics mark the attempt unavailable and release partial evidence without changing collection, builder
ingestion, or topology publication. Replay validates the completed shape, reconstructs the allowlisted values once,
invokes the same event dispatcher, and ignores failed profiles and unsuccessful VLAN contexts.

The optional ddsnmp producer creates temporary route reports and value references alongside the live initial collection.
Their size is linear in the selected profile routes and collected topology/BGP values; they are delivered synchronously
to the topology recorder, which owns admission of the immutable evidence selected for retention. Acquisition reporting
is explicitly an initial-collection facility on a fresh ddsnmp collector. The observer is released when the initial
`Collect` call returns; later calls continue normal collection and live-cache reuse without emitting acquisition reports.
The old/new generation overlap remains governed by the topology refresh lifecycle.

`topology_cache_metric_dispatch.go` maps `ddsnmp.TopologyKind` values to the
right builder ingester. Profile tags and device metadata are applied separately
because they describe the device itself rather than one topology row.

Finalization converts the builder into an immutable `topologyDeviceGeneration`:

- a prepared `ObservationSnapshot` for Function, focus, availability, and
  reverse-DNS readers;
- immutable trap-match, interface-name, and neighbor indexes;
- collection and expiry timestamps;
- the typed DeviceStore registration ID;
- a generation-local evidence reference and successful acquisition capture.

The collector separately owns `deviceRefreshState` per registration ID. It
tracks `lastAttempt`, `lastSuccess`, `nextRetry`, the latest outcome,
consecutive failures, the monotonic attempt ordinal, the latest completed
attempt capture, and the last successful device generation.

The published `topologyGeneration` also owns the producer scope captured at the
same commit boundary. Graph readers therefore cannot combine an observation
vector from one sweep with a later registry scope.

## Registry And Snapshot

`topology_registry.go` owns one atomic pointer to the latest immutable
`topologyGeneration`. The generation contains the complete, registration-ID
ordered vector and producer scope produced by one refresh sweep.

```text
topologyRegistry.snapshotWithEnvironment(options, environment)
  normalize query options
  acquire one topology generation
  read the generation's fixed renderable device membership and producer scope
  aggregate per-device observations
  build a topology graph
```

Each device generation contributes:

- an `l2topology.L2Observation`, used by the generic L2 engine;
- typed SNMP-side observation rows for L3 interfaces, OSPF neighbors, and BGP
  peers;
- local device detail used to enrich the selected local actor.

Each direct SNMP observation gives the generic L2 builder one selected
`ManagementIP` plus vetted `ManagementAliases`. Raw typed SNMP management rows
remain diagnostic evidence; valid IP-family rows also remain trap-matching
evidence. Public match, focus, and collapse identity consume the reconciled L2
result.

The generic L2 builder resolves address authority before neighbor matching:

- a selected primary owns its address over another device's alias;
- an alias claimed by multiple direct devices is removed from their public
  identity, while selected-primary collisions keep the existing IP-collapse
  behavior;
- every direct primary and alias claim seeds immutable ownership before
  neighbor resolution, including claims removed from public identity;
- actors retain the complete reconciled alias set for match, focus, and
  collapse, while each repeated link endpoint carries only the selected primary
  or one numerically deterministic canonical alias as its IP identity hint;
- FDB ownership and L3 correlation use complete actor matches, then precompute
  bounded link-only match views once per endpoint or actor;
- addresses from inferred observations and LLDP/CDP neighbors are accumulated
  as claims and enter device identity only after the complete claim set proves
  exclusive ownership;
- adjacency `remote_management_ip` and `remote_address_raw` labels remain
  internal evidence and never bypass the reconciled device when projecting
  public match or `RemoteIP` fields.

Consequently, remote-only observations with different hostname or chassis
identities do not merge solely because they advertise the same IP. They still
correlate through matching strong identity or a uniquely owned direct-device
address.

IP collapse preserves complete actor aliases. Within each collision group, the
generic projector unions every union-merged list field once and the SNMP shaping
pass unions its match lists once; scalar, map, optional, attachment, and ordered
protocol detail precedence remains representative-first and actor-index ordered.
This keeps alias-rich shared-primary groups linear in their input plus the final
deduplication sort instead of rebuilding the growing union after every actor.

Public actor IDs retain the complete reconciled identity and remain unchanged.
Internal graph traversal and link ownership use opaque, nonzero actor handles
instead of copying or hashing those IDs per link:

- handles are generation-local, nonserialized, and unrelated to public actor
  IDs or rendered actor row references;
- the generic projector assigns final actor and link handles at its centralized
  identity boundary, while later local and L3 actors receive fresh handles from
  the same generation high-water mark;
- shaping, collapse, focus, L3/OSPF/BGP enrichment, and rendering use handles
  for equality and lookup, while public actor-ID ordering remains the
  deterministic presentation order;
- strict and probable graphs never compare raw handles across generations;
  probable-link marking interns their public actor IDs once into request-local
  comparison tokens;
- the renderer validates unique actors and resolved link handles, then maps
  handles to final actor rows without serializing the handles.

The aggregate also carries the producer scope id captured in its immutable
generation from the parent Agent registry id. L3 subnet segment actor ids use
that scope so identical private subnets observed by different Agents do not
collide after Cloud aggregation. If the registry id is unavailable, L3 subnet
segment actors are omitted; direct L3 subnet links, OSPF, and BGP enrichment
still run.

Renderable membership is fixed when a complete topology generation publishes;
Function, focus, availability, and reverse-DNS readers therefore see one stable
view until the next completed sweep. Newly successful device snapshots start
their display-freshness window at that publication boundary while preserving
their exact acquisition timestamp. A retained generation from a failed refresh
keeps its original deadline and is removed from renderable membership when a
later completed sweep observes it expired. Trap enrichment preserves the prior
behavior of using the last successfully published device generation even after
topology display freshness expires; unregistering the SNMP job removes it on the
next completed sweep.

## Graph Build Order

`topology_registry_build.go` is the main graph pipeline.

For normal map types:

```text
aggregate observations
  -> l2topology.BuildL2ResultFromObservations
  -> l2topology.ToGraph
  -> convert generic graph to topologymodel.Data
  -> augment local actors with SNMP device-generation detail
  -> topologyshape.ApplyPolicies
  -> topologyenrich.ApplyLayer3 (L3 subnet, OSPF, BGP)
  -> topologyshape.ApplyDepthFocusFilter
```

For the low-confidence map type, the builder creates a strict map and a
probable map, marks probable-only link deltas, then applies the same L3/OSPF/BGP
enrichment and depth/focus filtering to the probable map.

The default `managed_fabric` map keeps every monitored SNMP device, the legacy
direct LLDP/CDP discovery surface, direct STP adjacencies between monitored
devices, and bridge/FDB legs for broadcast-domain segments adjacent to at least
two distinct monitored devices. Multiple legs from one device do not satisfy
that threshold. Endpoint actors, endpoint-only or sparse segments, direct FDB
shortcuts, and segment-to-segment paths are excluded. The selectable legacy
`lldp_cdp_managed`, `high_confidence_inferred`, and
`all_devices_low_confidence` policies retain their existing semantics.

Map-type shaping applies to the Layer 2 graph. Logical Layer 3 enrichment runs
after shaping and is preserved for every map type: `/24` through `/29` subnet
segments, `/30` and `/31` direct subnet links, OSPF adjacencies, and BGP
adjacencies. Their topology presentation uses distinct dashed logical/control
links, so Layer 2 acceptance and statistics must be checked by link type rather
than by treating any non-empty graph as Layer 2 success.

Local actor augmentation indexes the pre-policy actor generation once by its
local identity subset: chassis id, system name, and selected management IP.
Policy shaping can collapse, remove, and reorder actors, so that index is
discarded before `ApplyPolicies`. `ApplyLayer3` then builds one post-policy
resolver from copied managed-actor references and shares it across L3 subnet,
OSPF, and BGP enrichment. BGP runs last because it extends the resolver with
BGP-local identifiers and interface addresses. This keeps actor-alias work
linear in the indexed identities instead of repeating the complete alias scan
for every device snapshot and logical L3 enricher.

L3 subnet enrichment has two grains:

- `/30` and `/31` shared subnets emit direct managed-device
  `l3_subnet` links.
- `/24` through `/29` shared subnets emit an `l3_subnet_segment` actor plus
  `l3_subnet_membership` links from each resolved managed SNMP device to that
  segment.

L3 subnet segments are logical shared-subnet evidence, not physical links. They
include only managed SNMP network devices that can be resolved to topology
actors. Depth/focus filtering may show a focused device and the subnet segment
without fanning out to every other device on that subnet.

Current L3 subnet segments are single-routing-context. `L3Interface` has no
VRF/routing-context field, so segment identity is producer scope plus
subnet/prefix. Identical subnet/prefix values in multiple VRFs inside the same
producer scope are therefore one logical segment until collection adds routing
context and segment identity includes it.

## Internal Packages

The root package owns collector lifecycle, builder ingestion, immutable generations, registry snapshots,
and adapters to shared SNMP-family state.

The internal packages are deliberately narrower:

- `internal/topologymodel`: typed internal graph model used by SNMP topology.
- `internal/topologyoptions`: comparable scalar Function/query option constants
  and normalization.
- `internal/topologyshape`: graph shaping and policy passes, such as collapse,
  map type filtering, probable-link marking, and depth/focus filtering.
- `internal/topologyenrich`: pure graph enrichment for L3 subnet, OSPF, and BGP
  logical links.
- `internal/topologyv1`: renderer from `topologymodel.Data` to
  `netdata.topology.v1`.
- `internal/topologyutil`: shared normalization helpers.

`snmptopologyfunc` owns the Function API surface:

- method/function IDs;
- accepted parameters;
- Function response handling;
- conversion of request params into `topologyoptions.QueryOptions`.

It does not own graph building or rendering.

### Dependency Direction

The internal packages form a one-way dependency DAG. `internal/topologyutil` is
the only leaf; the other packages point inward toward the model, and the root
package composes all of them.

```text
topologyutil       leaf (stdlib only)
topologymodel    -> topologyutil
topologyoptions  -> topologyutil
topologyshape    -> topologymodel, topologyoptions, topologyutil
topologyenrich   -> topologymodel, topologyutil
topologyv1       -> topologymodel, topologyoptions, topologyutil
snmptopologyfunc -> topologyoptions, topologyutil
root snmptopology-> all of the above
```

Invariants (enforced by `go list -deps` in the decomposition validation):

- No internal package imports the root `collector/snmp_topology` package. The
  root composes the internal packages; they never depend back on it.
- The sibling layers `topologyshape`, `topologyenrich`, and `topologyv1` do not
  import one another. Logic shared between them lives in `topologymodel` or
  `topologyutil`.
- `topologyutil` imports no sibling; `topologymodel` and `topologyoptions`
  import only `topologyutil`.
- `snmptopologyfunc` (Function transport) depends on `topologyoptions`;
  `topologyoptions` owns the canonical query-option vocabulary and never imports
  the Function package.

## Function Request Path

```mermaid
flowchart TD
    Request["snmp:topology:snmp request"]
    Handler["snmptopologyfunc.Handle"]
    Options["resolve QueryOptions"]
    Registry["topologyRegistry.snapshotWithEnvironment"]
    Snapshot["fresh device-generation snapshots"]
    Aggregate["aggregate observations"]
    L2["l2topology BuildL2Result -> ToGraph"]
    Shape["topologyshape policies and focus"]
    Enrich["topologyenrich L3, OSPF, BGP"]
    Render["topologyv1.Render"]
    Response["Function topology response"]

    Request --> Handler --> Options --> Registry --> Snapshot --> Aggregate
    Aggregate --> L2 --> Shape --> Enrich --> Render --> Response
```

```text
snmp:topology:snmp request
  -> snmptopologyfunc.Handle
  -> funcDepsAdapter.Snapshot(options)
  -> topologyRegistry.snapshotWithEnvironment(options, cache-only DNS)
  -> topologyv1.Render(data)
  -> Function response with type "topology"
```

The Function returns `503` while no usable topology snapshot exists yet.

Reverse DNS is cache-backed and non-blocking on the Function path:

- `collector/init.go` owns one bounded resolver shared by `snmp_topology` and `snmp_traps`; neither collector closes or
  sweeps it.
- Function rendering uses only the shared resolver's cache-only `Lookup` path.
- The same display-name code records IPs it tried to resolve.
- A topology-owned warmer uses blocking, coalesced `Resolve` calls with at most four local workers and 1,024 candidates
  after refresh snapshots and after Function requests enqueue newly observed candidates.
- DNS failures, timeouts, and cache misses fall through to the existing
  sysName, hostname, IP, and MAC display-name order.

Function requests must not perform live DNS I/O while serving a response.

## Offline Graph Replay

`topology_graph_replay.go` rebuilds a typed `netdata.topology.v1` payload from
the committed diagnostic cut without retaining Function requests or rendered
payloads:

```text
committed topology diagnostic cut
  -> replay each renderable device's acquisition evidence
  -> sort and aggregate observation snapshots
  -> apply caller-supplied scalar query options
  -> run the shared graph, enrichment, shaping, and topology-v1 renderer
```

The replay contract is hermetic:

- query options contain only comparable scalar selectors; replay callers supply
  the desired option set instead of selecting retained query history;
- collection time comes from acquisition evidence, and replay rejects missing
  collection timestamps instead of allowing the renderer's current-time
  fallback;
- producer scope comes from the same immutable generation as the diagnostic
  cut;
- the offline build environment has no reverse-DNS resolver, so PTR-derived DNS
  names and display choices deterministically fall back to collected identity;
- OUI enrichment uses the compiled-in lookup table and needs no captured
  environment revision;
- a renderable device without available acquisition evidence makes complete
  replay fail rather than silently emitting a partial graph.

Live Function and offline replay use the same graph and renderer. Their typed
topology structure is identical for the same scalar options; only PTR-derived
presentation fields may differ. This replay entry point consumes trusted,
already-bounded in-memory diagnostics. Validation and structural limits for
untrusted archive input belong to the later archive reader boundary.

## Trap Enrichment

`topology_trap_enrich.go` publishes a separate handle used by `snmp_traps`.

The topology collector publishes enrichment state when `Run(ctx)` starts and
unpublishes it on cleanup. Trap enrichment is not part of the topology Function
payload; it is a cross-collector lookup path for trap log rows.

## Metrics And Charts

The collector emits internal metrics only. These metrics describe refresh health
and retained device-generation state; they are not the topology payload.

- `Collect(ctx)` writes current internal metric values.
- `Run(ctx)` performs SNMP topology refresh.
- `charts.go` and `metrix.go` define this internal observability surface.

## Concurrency Rules

- `Collector.refreshMu` serializes topology refreshes and cleanup.
- Each due device is collected into a private `topologyBuilder`; builders have no
  locks because runtime readers never receive them.
- Finalization freezes the builder into one immutable
  `topologyDeviceSnapshot`. A completed sweep activates successful snapshots as
  `topologyDeviceGeneration` values at one shared publication time. A failed
  collection retains the prior successful device generation and deadline.
- A completed sweep fixes renderable membership and publishes one immutable
  `topologyGeneration` through an atomic pointer. Cancellation or panic leaves
  the previous generation visible.
- Function, focus, availability, reverse-DNS, diagnostics, and trap readers each
  load that pointer once and never block on collection or builder locks.
- The registry mutex only protects producer-scope discovery and the reverse-DNS
  warmer context; it does not protect topology generations.

When adding new collected state, add it to:

- `topologyBuilder` and `newTopologyBuilder`;
- the relevant ingestion and finalization path;
- the immutable observation or trap projection that owns the published value;
- tests proving the value is complete before publication and that a failed or
  canceled refresh cannot expose partial state.

## Where To Change Things

- Add or adjust SNMP topology profile rows:
  - profile YAML and `ddsnmp.TopologyKind`;
  - builder ingester in the root package;
  - snapshot conversion if the row contributes to graph facts.
- Add graph shaping policy:
  - `internal/topologyshape`.
- Add logical links from existing observations:
  - `internal/topologyenrich`.
- Add or change Function parameters:
  - `internal/topologyoptions`;
  - `snmptopologyfunc`;
  - Function tests.
- Add or change topology payload columns, tables, presentation, or legend:
  - `internal/topologyv1`;
  - normalized golden test;
  - topology schema validation tests.
- Add collector refresh or generation lifecycle behavior:
  - root package only.

## Validation Checklist

Useful focused checks after changes:

```text
cd src/go
env GOCACHE=/tmp/netdata-go-build-cache go test -count=1 ./plugin/go.d/collector/snmp_topology/... ./pkg/l2topology/... ./pkg/topology/...
env GOCACHE=/tmp/netdata-go-build-cache go test -tags=snmp_topology_fixtures -count=1 ./plugin/go.d/collector/snmp_topology/...
env GOCACHE=/tmp/netdata-go-build-cache go test -race -count=1 ./plugin/go.d/collector/snmp_topology/...
env GOCACHE=/tmp/netdata-go-build-cache go vet ./plugin/go.d/collector/snmp_topology/... ./pkg/l2topology/... ./pkg/topology/...
```

If topology output may have changed, also inspect:

```text
git diff -- src/go/plugin/go.d/collector/snmp_topology/testdata/topology_v1_normalized_golden.json
```

An unchanged golden is expected for internal ownership and generation refactors.
