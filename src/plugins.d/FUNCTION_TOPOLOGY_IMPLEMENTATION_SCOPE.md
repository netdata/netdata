# Topology Schema Implementation Scope

This document scopes the work needed to move Netdata topology producers,
Cloud aggregation, and the Cloud UI to the production topology schema defined
in [FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md](/src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md)
and [FUNCTION_TOPOLOGY_SCHEMA.json](/src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json).

It is not an implementation plan for one commit. It is the work map for the
backend, frontend, producer, and aggregator changes.

## Ground Rules

- New topology producers emit only the new schema.
- Superseded topology schema support is removed from Agent/backend contracts and docs.
- Temporary compatibility support may exist only as an isolated Cloud frontend
  adapter during Agent rollout.
- Production payloads carry canonical topology facts, not reconstruction
  instructions for compatibility payloads.
- Test-only reconstruction/projection code may derive older shapes to prove
  information parity, but that code must not affect production payloads.
- Raw payload captures from real systems stay under `.local/` and are never
  committed.

## Shared Backend Work

### Function Contract

Required changes:

- add topology validation against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`;
- update Function validator tooling to recognize the new topology schema;
- make topology Function examples and tests use the new contract;
- remove superseded topology-schema references from Agent/backend docs once producer
  migration lands.

Likely files:

- `src/go/tools/functions-validation/`
- `src/plugins.d/FUNCTION_UI_REFERENCE.md`
- `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`

### Shared Encoding Helpers

The schema uses compact columnar tables. Producers should not hand-roll table
encoding repeatedly.

Required helpers:

- table builder for `rows` / `columns` / `values`;
- codecs for `const`, `values`, and `dict`;
- string dictionary builder;
- validation checks for column/value length;
- deterministic sorting helpers for actors, links, and evidence rows;
- size measurement hooks for tests.

Likely homes:

- Go: `src/go/pkg/topology/` or `src/go/pkg/funcapi/`
- C: small helper module for network-viewer, or a local builder until a shared
  C helper is justified
- Rust: SDK helper if a Rust topology producer is added

## Current Migration Inventory

### Agent Producers

`topology:network-connections`:

- producer path: `src/collectors/network-viewer.plugin/network-viewer.c`;
- the Function now emits `netdata.topology.v1` at
  `src/collectors/network-viewer.plugin/network-viewer.c:2535`;
- the Function parses `aggregated` / `mode:aggregated` and `detailed` /
  `mode:detailed`, with aggregated as the default, at
  `src/collectors/network-viewer.plugin/network-viewer.c:272`;
- response metadata exposes the `mode` selector at
  `src/collectors/network-viewer.plugin/network-viewer.c:1451`;
- actors, graph links, and optional socket evidence rows are emitted as compact
  columnar tables at `src/collectors/network-viewer.plugin/network-viewer.c:2568`;
- socket evidence is emitted only in detailed mode at
  `src/collectors/network-viewer.plugin/network-viewer.c:2571`;
- repeated string columns use automatic dictionary encoding when it is smaller
  than plain values at `src/collectors/network-viewer.plugin/network-viewer.c:2041`;
- old-schema presentation metadata and actor-nested socket tables have been
  removed from the Agent producer. The v1 producer now emits compact
  graph-presentation metadata inside type definitions plus `data.presentation`.
  Actor modal socket lists must be derived from evidence by the Cloud
  frontend/aggregator during rollout.

`topology:streaming`:

- producer paths: `src/web/api/functions/function-topology-streaming.c` and
  `src/streaming/stream-path.c`;
- the Function now emits `netdata.topology.v1` directly at
  `src/web/api/functions/function-topology-streaming.c:1870`;
- actors, graph links, link evidence, and actor-detail tables are emitted as
  compact tables at `src/web/api/functions/function-topology-streaming.c:1912`;
- streaming path rows are preserved as an `actor_detail` table at
  `src/web/api/functions/function-topology-streaming.c:1255`;
- inbound and outbound drilldown rows are declared as relationship summaries at
  `src/web/api/functions/function-topology-streaming.c:1259`;
- streaming, virtual, and stale links have explicit directed link-type metadata
  and separate evidence type ids at
  `src/web/api/functions/function-topology-streaming.c:1226`;
- remaining streaming work is parity/UX validation with the Cloud frontend and
  Cloud aggregator once those parallel workers are ready.

`topology:snmp`:

- producer paths: `src/go/plugin/go.d/collector/snmp_topology/` and
  `src/go/pkg/topology/`;
- the Function handler now adapts the current SNMP topology snapshot to
  `netdata.topology.v1` through
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`, while the
  deeper engine still builds the older shared Go topology model;
- the older shared Go topology data model stores top-level actor/link arrays at
  `src/go/pkg/topology/types.go:167`;
- older actor tables are nested under actor rows at
  `src/go/pkg/topology/types.go:24`;
- the older link model carries `Direction`, `Src`, `Dst`, and metrics at
  `src/go/pkg/topology/types.go:42`;
- the older presentation table metadata still uses `Source` only at
  `src/go/pkg/topology/types.go:72`, with SNMP ports/links examples at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_schema.go:29`;
- current L2 emission uses directions such as `bidirectional` and
  `unidirectional` at `src/go/pkg/topology/engine/topology_adapter_segments_builder_emit.go:60`
  and `src/go/pkg/topology/engine/topology_adapter_projection_pairs.go:230`;
- migration target: use `observed_bidirectional` or unordered aggregation policy
  where discovery direction is noise, preserve LLDP/CDP/FDB/ARP/STP evidence,
  keep interface inventory as actor detail/inventory, and move metric query
  definitions to overlay templates/refs.

vSphere:

- producer path is in a separate PR worktree and must be updated in place only
  after telling the user;
- migration target: use stable managed object ids for actor identity, model
  containment as hierarchical/ownership links, model VM-host/network/datastore
  relationships as graph links plus evidence where needed, and expose
  utilization/state through overlay templates/refs.

### Cloud Frontend

The Cloud frontend compatibility work is outside this repository, but the
schema rollout depends on it:

- current topology fetch normalizer decodes every topology payload through
  `normalizeTopologyPayload(response?.data || {})` and then computes render-time
  aggregated links at `${CLOUD_FRONTEND_REPO}/src/domains/functions/useFetch/normalizers/topology/index.js:9`;
- current frontend graph aggregation groups by source, target, and link type,
  canonicalizing reverse links if already seen, at
  `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/graphAggregation.js:58`;
- current actor modal code still branches on presentation table `source` values,
  including `source === "links"`, at
  `${CLOUD_FRONTEND_REPO}/src/domains/functions/components/topology/actorModal/index.js:286`;
- migration target: add a new-schema decoder for compact tables, keep old-schema
  support isolated in one temporary adapter, derive actor drilldown relationship
  tables from evidence rows, render actor custom tables from typed actor-detail
  tables, and use link-type direction metadata instead of guessing from raw link
  direction strings.

## Producer Migration Scope

### `topology:network-connections`

Producer path:

- `src/collectors/network-viewer.plugin/network-viewer.c`

Required behavior:

- emit actors as compact actor table rows;
- emit graph links as grouped process/endpoints relationships;
- emit one socket evidence row per socket tuple needed for cross-node matching;
- default to aggregated graph projection while preserving detailed evidence;
- support aggregation scopes prepared for node, process name, PID, container,
  and Kubernetes workload labels as enrichment becomes available;
- omit compatibility per-row display strings, duplicated labels, and actor modal socket
  tables from production payload;
- keep current metrics optional and separate from topology identity.

Validation:

- compare against captured corpus under `.local/`;
- prove no truncation on large socket counts;
- assert payload size at corpus scale;
- assert exact reverse-tuple matching inputs remain present.

Current state:

- `src/collectors/network-viewer.plugin/network-viewer.c` now emits compact
  actor rows, graph-link rows, and optional socket evidence rows directly in
  `netdata.topology.v1`;
- aggregated mode is the default and omits socket evidence from the response;
- detailed mode keeps socket evidence as a shared relationship-evidence table,
  not as actor-owned duplicated modal data;
- link and evidence string columns choose dictionary encoding only when it
  reduces raw payload size;
- remaining network-connections work is corpus-scale validation with captured
  Cloud payloads and Cloud/frontend integration once the parallel workers are
  ready.

### `topology:streaming`

Producer paths:

- `src/web/api/functions/function-topology-streaming.c`
- `src/streaming/stream-path.c`

Required behavior:

- emit streaming agents as actors;
- emit parent/child streaming relationships as directed dependency links;
- classify `stream_path` as actor detail, not relationship evidence;
- keep retention and relationship summaries as typed detail tables;
- make direction semantics explicit through link type definitions.

Validation:

- preserve current actor modal data through new actor-detail tables;
- prove graph links and custom actor tables are not conflated;
- use fixtures from current streaming topology tests where possible.

Current state:

- `src/web/api/functions/function-topology-streaming.c` now emits
  `netdata.topology.v1` directly from the C Function;
- actor rows, link rows, relationship evidence, `stream_path`, `retention`,
  `inbound`, and `outbound` tables are compact columnar sections;
- stale stream-path hops remain signed values instead of being coerced to
  unsigned values;
- streaming, virtual, and stale graph-link types have separate evidence types
  so link-type metadata and evidence metadata agree.
- streaming graph-presentation metadata is emitted inside type definitions plus
  `data.presentation`, including highlight-path selection, legend, link styles,
  and graph port-bullet tokens.

### `topology:snmp`

Producer paths:

- `src/go/plugin/go.d/collector/snmp_topology/`
- `src/go/pkg/topology/engine/`

Required behavior:

- emit devices, interfaces, bridge domains, VLANs, and endpoints as actor rows;
- emit L2 adjacencies with direction policy `canonicalize_unordered` when
  direction is discovery noise;
- preserve LLDP/CDP/FDB/ARP/STP facts as evidence or actor inventory depending
  on role;
- move interface traffic/errors/state metric pointers to overlay templates and
  overlay refs;
- avoid copying metric query fragments on every link.

Validation:

- reuse existing SNMP topology golden fixtures;
- add schema-level golden fixtures for devices, interfaces, ports, and
  bidirectional adjacency merge;
- verify overlay refs can query interface metrics without recomputing topology.

Current state:

- initial Function payload migration is implemented through a v1 adapter in
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`;
- the adapter emits compact actor, link, evidence, actor metadata, and
  actor-detail tables and preserves nested custom actor cells with `json`
  columns where needed;
- remaining SNMP work is to migrate metric lookup fragments into first-class
  overlay templates/refs instead of only preserving them in actor/detail data.

### vSphere Topology

Producer worktree:

- the separate vSphere topology worktree used for that PR

Required behavior:

- update the vSphere topology producer to the new schema in place;
- use stable vSphere managed object ids as actor identity where available;
- model inventory containment with hierarchical ownership links;
- represent VM-to-host, cluster-to-host, datastore, and network relationships
  as graph links plus typed evidence where needed;
- use overlay templates for refreshable utilization/state metrics.

Coordination constraint:

- do not edit the vSphere worktree before telling the user, because another
  agent is working in that directory.

## Cloud Frontend Scope

Required changes:

- add a decoder for the compact table schema;
- build graph nodes from the actors table;
- build graph edges from the links table;
- derive actor drilldown relationship tables from evidence rows;
- render actor custom tables from typed actor-detail tables;
- use link type direction metadata to decide whether links are directed,
  undirected, hierarchical, or observation-only;
- use overlay templates and refs for metric refreshes;
- isolate compatibility support in one temporary adapter;
- delete the temporary adapter after Agent rollout.

Likely frontend areas:

- topology payload normalizer;
- graph aggregation layer;
- actor modal tables;
- link details;
- telemetry overlay query layer;
- Function response version detection.

Frontend risks:

- decoding large columnar sections synchronously can still block the main
  thread; use streaming, workers, or chunked decode if needed;
- mixed Agent versions need clear adapter selection;
- actor modal tables must not duplicate evidence in memory unnecessarily.

## Cloud Aggregator Scope

The aggregator should be implemented in Go as a separate Cloud component or
service, not inside charts-service request routing.

The MVP aggregator must support all topology kinds covered by the production
schema contract. `topology:network-connections` remains the required
high-cardinality benchmark, but it is not an acceptable production boundary by
itself. The Cloud UI should not need separate aggregation paths for different
topology kinds.

### Inputs

- one or more `netdata.topology.v1` payloads;
- requested aggregation scope, such as node, process name, container,
  Kubernetes workload labels, vSphere object type, or SNMP device/interface;
- optional filters such as layer, link type, actor type, room, or node set.

### Outputs

- a `netdata.topology.v1` payload with:
  - merged actor rows;
  - merged graph links;
  - preserved or counted evidence rows according to schema policy;
  - merged detail tables according to table type policy;
  - merged overlay refs according to overlay template policy;
  - stats describing input rows, output rows, evidence rows, and drops/errors.

### Core Packages

Suggested package split:

- `schema`: generated or hand-written Go structs for the topology schema;
- `codec`: compact table decode/encode helpers;
- `model`: canonical in-memory actors, links, evidence, tables, overlays;
- `aggregate`: scope-based actor/link/evidence merge logic;
- `match`: identity normalization and exact tuple matching;
- `validate`: schema and semantic validation;
- `fixtures`: sanitized corpus and synthetic scale fixtures.

### Aggregation Logic

Required behavior:

- merge actors by the requested scope and actor type identity;
- preserve evidence rows when evidence policy is `preserve`;
- count evidence rows when evidence policy is `count`;
- never silently truncate evidence;
- fail explicitly when a requested payload would exceed configured limits;
- canonicalize undirected links only when link type policy allows it;
- preserve directed links when direction is flow, dependency, or ownership;
- merge overlay refs with `set` or `append` semantics defined by templates.

Network socket matching:

- exact reverse-tuple matching is in scope;
- NAT, load balancer, and proxy inference are out of scope for the first
  aggregator;
- unresolved endpoints can aggregate by visible endpoint identity, but the
  evidence row must remain available when the requested mode preserves it.

### Limits And Failure Behavior

The aggregator must have explicit limits:

- maximum decoded bytes;
- maximum actor rows;
- maximum graph links;
- maximum evidence rows;
- maximum output bytes;
- maximum CPU time per request.

If a limit is exceeded:

- return a structured error;
- include stats showing which limit was exceeded;
- do not return a truncated topology as if it were complete.

Paged or chunked evidence transport remains a phase-2 option. Phase 1 should
make payloads small enough that this is rarely needed.

### Tests

Required test classes:

- schema decode/encode round-trip;
- semantic validation failures;
- actor identity merge by scope;
- directed vs undirected link aggregation;
- relationship evidence preservation;
- actor-detail table aggregation;
- overlay ref merge;
- network socket exact reverse-tuple matching;
- streaming hierarchy and actor-detail custom tables;
- SNMP/L2 unordered adjacency and observation evidence;
- vSphere ownership/dependency topology;
- generic schema-conformant custom topology passthrough;
- synthetic scale benchmark near and above current corpus scale;
- sanitized real-corpus replay from `.local/` promoted only as non-sensitive
  fixtures when safe.

## Rollout Plan

1. Land schema docs, public skill, and implementation scope.
2. Add validator support and compact-table helpers.
3. Add Cloud frontend new-schema decoder and temporary compatibility adapter so
   mixed Agent rollout is safe before producers emit the new schema broadly.
4. Migrate producers behind tests. `topology:network-connections` should be the
   first high-cardinality producer exercised internally, but it is not the
   production boundary for Cloud aggregation.
5. Migrate the streaming producer and complete SNMP overlay-template
   refinement.
6. Coordinate and migrate the vSphere topology producer.
7. Build `cloud-topology-service` in parallel against fixtures and
   new-schema payloads. Its MVP is complete only when all topology kinds covered
   by this contract pass service-level aggregation tests.
8. Hand final service ownership, environment-specific Helm values, deployment
   targets, and production node-instance routing strategy to Cloud backend and
   DevOps once the service is otherwise ready for operational integration.
9. Remove compatibility support from Cloud frontend after supported Agent rollout.

## Resolved Phase-1 Defaults

- Cloud aggregator service repository: `cloud-topology-service`.
- Cloud aggregated topology route: `POST /api/v3/spaces/{spaceID}/rooms/{roomID}/topology`.
- Cloud service contract: accepts and emits only `netdata.topology.v1`.
- Phase-1 topology service MVP: all topology kinds covered by this contract,
  not only `topology:network-connections`.
- Network socket snapshot metrics such as RTT and retransmissions: opt-in, not
  default core topology columns.
- Cloud-side topology payload cache: no payload cache in phase 1; aggregate on
  demand and collect request-cost metrics first.
- Service-local validation package is sufficient for the Cloud service MVP;
  producer CI may still add a separate validator binary later if needed.

## External Integration Gates

These items cannot be safely invented from this repository and must be handed
to Cloud backend and DevOps when `cloud-topology-service` is otherwise ready for
operational integration:

- final service owner and CODEOWNERS entries;
- environment-specific Helm values and deployment targets;
- approved production node-instance routing strategy.
