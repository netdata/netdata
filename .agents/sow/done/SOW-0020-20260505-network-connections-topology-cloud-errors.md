# SOW-0020 - Network-connections topology Cloud errors

## Status

Status: completed

Sub-state: completed after the compact topology schema migration, producer migrations, validation fixtures, and the targeted network-connections graph-invariant repair for floating endpoint actors.

## Requirements

### Purpose

Make `topology:network-connections` reliable enough for production Cloud topology use in the target Cloud space. The Function must not crash or disconnect mid-response, must report missing/unavailable cases clearly, and must not produce disconnected floating endpoint actors unless the graph semantics explicitly allow them.

The expanded purpose is to define and implement a lossless compact `detailed` topology payload contract and an `aggregated` topology view path so large topologies can be used by the UI and by Cloud cross-node correlation without exceeding practical payload limits.

### User Request

The user reported three Cloud network-connections topology issues:

- One node returns HTTP 503 while data appears to start arriving; browser console shows `AgentError` with message `The plugin that was servicing this request, exited before responding.`
- Another node returns HTTP 404 for the same `topology:network-connections` Function request.
- A previously observed UI state showed orphan endpoint actors floating in the topology, but the user cannot reproduce it now.

The user then expanded the required solution:

- Update the Function schema for the new topology payload contract.
- Add/declare an `aggregated` vs `detailed` request mode for `topology:network-connections`; default behavior is `aggregated`.
- Update all topology Functions to emit the new schema: `topology:network-connections`, `topology:streaming`, and `topology:snmp`.
- Update the Cloud frontend to support deployed compatibility payloads and the new schema. Compatibility aggregation must stay isolated so it is easy to delete after supported Agents emit the new schema.
- Add a separate Cloud topology aggregation microservice. It consumes detailed topology payloads, aggregates topologies, and returns the aggregation requested by the user.

### Assistant Understanding

Facts:

- The failing Function name is `topology:network-connections`.
- The first observed Cloud request returned HTTP 503 and the browser console reported an agent-side plugin exit before response completion.
- The second observed Cloud request returned HTTP 404 for the same Function name and selections.
- `topology:network-connections` is registered by the C network-viewer plugin at `src/collectors/network-viewer.plugin/network-viewer.c`.
- Raw browser cookies and session tokens were present in the chat. They must not be copied into durable artifacts.

Inferences:

- The 503 is likely not a pure frontend cancellation because the console includes an explicit agent/plugin error. The early `Request was cancelled` stack may be a frontend request lifecycle side effect, but it is not the strongest failure signal.
- The 404 may mean the target node does not expose the Function, the node is stale/unreachable through Cloud, the function list differs by agent/plugin version, or Cloud maps an agent-side function-missing condition to 404.
- The orphan endpoint symptom may be a graph-construction invariant violation: actors emitted without at least one emitted link, or links filtered/dropped after actor creation.

Unknowns:

- Whether the network-viewer plugin process exits due to a crash, fatal(), timeout/cancellation path, memory pressure, malformed topology parameters, or Cloud transport disconnect.
- Whether the 404 node lacks the Function, is stale, runs an older agent, or is behind a Cloud routing/state issue.
- Whether the orphan endpoints were produced by agent data, Cloud/frontend filtering, or a race between partial data and final error handling.

### Acceptance Criteria

- The 503 path is reproduced or falsified through token-safe Cloud or direct-agent wrappers, with status code, response class, and sanitized failure evidence recorded.
- The 404 path is explained with node state/function availability/version evidence.
- The serving code path and any crash-prone branches for the reported selections are traced with file:line evidence.
- If a code fix is needed, it has focused tests or an equivalent validation path, and the validation includes a same-failure search for orphan actors.
- The new topology schema preserves the canonical information needed by Cloud aggregation and UI rendering. Compatibility projection for deployed payload parity is test/transition code only and is not part of the production payload.
- The new aggregated topology schema/view is explicitly derived from detailed topology data and is not treated as a correlation source of truth.
- `topology:network-connections`, `topology:streaming`, and `topology:snmp` all emit the new topology schema.
- `topology:network-connections` exposes an `aggregated`/`detailed` request mode in the Function schema, with `aggregated` as the default.
- The Cloud frontend supports deployed compatibility payloads and the new schema, with compatibility aggregation isolated in removable code.
- The Cloud topology aggregation microservice has a defined API contract and validation showing it can consume detailed topology payloads and return requested aggregated views.
- The detailed payload contract supports multiple actor scopes for Cloud aggregation, including node-level, process-name-level, PID-level, container/application-level, and Kubernetes label/workload-level grouping when the required enrichment evidence is available.
- Topology actor drilldown modals must continue to support per-actor tables listing exact dependencies and supporting details. These tables must avoid duplicating the same socket/link evidence under multiple actors where compact references or derived views can provide the same drilldown.
- Topology actor tables may also carry actor-owned custom data that is not relationship evidence and is not generally aggregatable, such as streaming topology `streaming_path` and retention/status tables. The schema must distinguish relationship/evidence tables from actor-detail/custom tables.
- Link direction semantics must be explicit. Some topology link types are direction-significant and must aggregate/render as directed relationships, while others are undirected adjacencies where any `direction` field describes observation completeness or evidence state rather than graph direction.
- Topology links must be able to expose refreshable traffic overlays without recomputing the topology. The schema must support compact link-level telemetry query templates, per-link template parameters, and merge rules for aggregated links.
- Before freezing the schema or updating producers/UI, build an emulation and benchmarking harness that can model required topology use cases at multiple scales, compare alternative encodings, run a hypothetical aggregation service, and produce repeatable payload-size and correctness evidence.

## Analysis

Sources checked:

- `docs/netdata-ai/skills/query-netdata-cloud/SKILL.md`
- `docs/netdata-ai/skills/query-netdata-agents/SKILL.md`
- `.agents/skills/query-agent-events/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/sow/pending/SOW-0002-20260501-unified-multi-layered-topology-schema.md`
- `.agents/sow/current/SOW-0012-20260505-streaming-topology-classification-bugs.md`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/plugins.d/FUNCTION_UI_REFERENCE.md`

Current state:

- Existing SOW-0002 covers future unified topology merge semantics; it is broader and blocked on design decisions. This incident is narrower and should remain separate unless it turns into schema-level work.
- This SOW has now turned into schema-level work. Before implementation, its relationship with SOW-0002 must be resolved by either merging scope, superseding the relevant part of SOW-0002, or narrowing this SOW back to `topology:network-connections` only. The current user direction points toward this SOW owning the concrete detailed/aggregated schema migration.
- Existing SOW-0012 covers `topology:streaming`; it is also in `.agents/sow/current/` and its current branch state already has the streaming Function split into `src/web/api/functions/function-topology-streaming.c` and `src/web/api/functions/function-netdata-streaming.c`. On 2026-05-06 the user stated SOW-0012 is done. For SOW-0020 sequencing, this unblocks shared schema work against the streaming topology producer, but the SOW-0012 lifecycle file still needs separate close/move handling if that has not already happened elsewhere.
- `project-writing-collectors` classifies topology Functions as live snapshots that must not block collection loops and should be validated with the Function protocol tooling.
- Token-safe Cloud probes using the repository `.env` token path returned HTTP 200 for all checked aliases at investigation time. This means the reported 503 and 404 were not stable, always-reproducible failures from the same API surface.
- Successful `topology:network-connections` responses were very large:
  - `node-503`: 134,180,370 bytes, 48 actors, 77,797 links, no orphan actors in the completed response.
  - `node-404-original`: 127,847,820 bytes, 47 actors, 73,989 links, no orphan actors in the completed response.
  - `node-404-latest`: 110,366,885 bytes, 47 actors, 63,895 links, no orphan actors in the completed response.
- The graph has low visual actor-pair cardinality despite very high link cardinality. For the largest response, 77,797 links collapse to 65 actor pairs, with the hottest actor pair carrying 38,275 separate socket links.
- `pluginsd` has a hard deferred Function response cap: `PLUGINSD_MAX_DEFERRED_SIZE` is `100 * 1024 * 1024` at `src/plugins.d/pluginsd_parser.h:18`, and the parser stops the plugin when a deferred response exceeds it at `src/plugins.d/pluginsd_parser.h:195-207`.
- The exact browser console message `The plugin that was servicing this request, exited before responding.` is emitted when an in-flight Function is deleted with HTTP 503 and no response body at `src/plugins.d/pluginsd_functions.c:96-97`.
- The `network-viewer` topology Function currently receives cancellation and timeout state but marks both unused at `src/collectors/network-viewer.plugin/network-viewer.c:2703-2705`; it builds the full response in memory and sends it only at the end at `src/collectors/network-viewer.plugin/network-viewer.c:2713-2727`.
- The `processes:by_name` option collapses process actors by command name at `src/collectors/network-viewer.plugin/network-viewer.c:1242-1253`, but the link key still includes pid, uid, namespace, local IP, remote IP, protocol, direction, state, local port, and remote endpoint port at `src/collectors/network-viewer.plugin/network-viewer.c:1359-1370`. This is the immediate source of many parallel socket links between the same collapsed actor pairs.
- The latest browser 404 maps to Cloud error key `ErrNodeInstanceNotFound` / message `could not find node instance`. At probe time, the same node alias was present in the room inventory, reachable, running the same nightly version as `node-503`, and advertised `topology:network-connections` through the room Functions endpoint. This points to a Cloud node-instance routing/state race or stale node-instance lookup, not a stable missing Function.
- A sibling `cloud-charts-service` checkout shows the Cloud Function route (`/api/v2/nodes/{nodeID}/function`) is handled by `nodePathProxy` (`cloud-charts-service/http/http.go:145-147`), which calls `DirectNodeRequest` (`cloud-charts-service/http/http.go:264-270`). `DirectNodeRequest` selects a node instance before proxying the request (`cloud-charts-service/internal/service/agent_data.go:565-585`). `ErrNodeInstanceNotFound` is the 404 error key and message in `cloud-charts-service/internal/model/errors.go:15`, and it is returned when node-instance routing filters leave no candidate (`cloud-charts-service/internal/routing/node_instance_filter.go:35-37`).
- A sibling `cloud-frontend` checkout has prior topology TODOs documenting the same class of payload issue: a previous 89 MB response with 42,609 links collapsed to 116 actor-pair tuples. Current source has render-time link aggregation, but the useFetch topology normalizer still synchronously normalizes every source link and then computes aggregated links for graph rendering: `src/domains/functions/useFetch/normalizers/topology/index.js:5-17`.
- The frontend normalizer copies full actor tables into graph node details and stores raw actors/links in table rows: `src/domains/functions/topology/payload.js:276-311,436-460`. It also copies full link `labels`, `metrics`, `src`, and `dst` into every graph link detail object: `src/domains/functions/topology/payload.js:462-487`.
- The topology actor modal uses actor-type table definitions from presentation metadata and renders either link-derived tables or data tables. Data tables read rows from `node.details.tables[tableKey]`, while link tables are derived from the actor's incident graph links: `cloud-frontend/src/domains/functions/components/topology/actorModal/index.js:122-130,284-309` and `cloud-frontend/src/domains/functions/components/topology/actorModal/dataTable.js:21-24`.
- Current `topology:network-connections` presentation defines modal table metadata for actors, including `source: "data"` sockets tables and `source: "links"` connection tables: `src/collectors/network-viewer.plugin/network-viewer.c:1869-1938`.
- Current `topology:network-connections` embeds per-process `tables.sockets` rows directly under each actor: `src/collectors/network-viewer.plugin/network-viewer.c:2209-2226`. This satisfies the modal drilldown UX but contributes to duplication and payload size.
- Current `topology:streaming` defines `streaming_path`, `retention`, `inbound`, and `outbound` actor tables with `source: "data"` even though they have different semantics: some describe streaming relationships, while `streaming_path` is actor-specific path metadata. Evidence: `src/web/api/functions/function-topology-streaming.c:303-318,321-341,360-410`.
- Current `topology:streaming` emits each actor's `streaming_path` table through `rrdhost_stream_path_to_json()`, whose rows contain path metadata such as hostname, host ID, node ID, claim ID, hops, timestamps, capabilities, and flags. Evidence: `src/web/api/functions/function-topology-streaming.c:1583-1584` and `src/streaming/stream-path.c:79-95`.
- The current presentation table schema only distinguishes `source: "data"` from `source: "links"`: `src/plugins.d/FUNCTION_UI_SCHEMA.json:349-357` and `src/go/pkg/topology/types.go:72-78`. This is insufficient to communicate aggregation semantics.
- Current shared `topology_link` has a free-form optional `direction` string, and `topology_presentation_link_type` has visual fields such as label/color/width/dash but no direction semantics or aggregation policy: `src/plugins.d/FUNCTION_UI_SCHEMA.json:304-315,479-497` and `src/go/pkg/topology/types.go:42-55,100-106`.
- Current `topology:network-connections` includes socket direction in the link key and emits it as link `direction` and labels, so direction is part of socket identity and aggregation: `src/collectors/network-viewer.plugin/network-viewer.c:1359-1370,2532-2541,2630-2647`.
- Current `topology:streaming` emits links from child/agent actor to parent/target actor based on stream path, so source/destination order is semantically directed even though the link object does not emit a separate `direction` field for those links: `src/web/api/functions/function-topology-streaming.c:1654-1738`.
- Current SNMP/L2 topology projects discovery adjacencies as `unidirectional` until a reverse pair is merged into `bidirectional`; this is observation/completeness metadata for an L2 adjacency, not application traffic direction: `src/go/pkg/topology/engine/topology_adapter_projection_pairs.go:62-69,230-235,301-315`.
- Current SNMP topology already attaches metric lookup fragments to actors and interface rows. Device actors include `chart_id_prefix`, `chart_context_prefix`, and `device_charts`, while port/status rows include `chart_id_suffix` and `available_metrics`: `src/go/plugin/go.d/collector/snmp_topology/topology_local_actor_attrs.go:61-68` and `src/go/plugin/go.d/collector/snmp_topology/topology_local_actor_charts.go:10-28,65-67,97-115`.
- Current Cloud metric query guidance requires explicit `scope.contexts` to avoid metadata explosion and supports node/context/label/dimension filtering in request scope/selectors. This makes repeated full query payloads on every topology link a payload risk; topology should carry compact query references instead: `docs/netdata-ai/skills/query-netdata-cloud/query-metrics.md:9-13,182-219`.
- Existing function validation tooling already validates Function output against `src/plugins.d/FUNCTION_UI_SCHEMA.json` and provides an E2E pattern that can be extended or mirrored for topology schema tests: `src/go/tools/functions-validation/README.md:1-42`.
- Existing SNMP topology code already uses manifest/golden fixture tests for topology parity and real device scenarios, which is the right pattern for repeatable topology schema experiments: `src/go/pkg/topology/engine/parity/golden_fixture_test.go:1-40` and `src/go/pkg/topology/engine/parity/node_topology_parity_test.go:1-80`.
- Frontend aggregation groups links after normalization by source, target, and link type: `src/domains/functions/topology/graphAggregation.js:58-125`. This reduces render complexity but does not reduce Cloud transfer size, JSON parse cost, or normalization memory cost.
- Payload size analysis of the largest captured response shows the current response is minified but structurally verbose:
  - Full response: 134,180,370 bytes.
  - `data.links`: 126,858,432 bytes for 77,797 links.
  - `data.actors`: 7,316,909 bytes for 48 actors.
  - Link `src`/`dst` blocks alone account for 61,303,293 bytes.
  - Link `labels` blocks alone account for 32,556,526 bytes.
  - Link `metrics` blocks alone account for 11,191,014 bytes.
  - Link object key names repeat about 152 bytes per link, about 11,825,144 bytes total.
  - Actor `tables` account for 7,291,350 of 7,316,909 actor bytes; one process actor table accounts for 7,286,291 bytes.
- Compact encoding estimates on the same captured response:
  - Current core link objects `{src_actor_id,dst_actor_id,link_type,direction,state,protocol,layer}`: 16,050,624 bytes.
  - The same core as arrays with string actor IDs: 9,749,067 bytes.
  - The same core as arrays with actor indexes: 4,006,272 bytes.
  - Actor indexes without repeating the constant layer: 3,617,287 bytes.
  - Actor indexes plus direction/protocol/state only: 2,917,072 bytes.
  - Backend graph aggregation by current UI dimensions collapses 77,797 raw links to 69 link groups; the grouped array representation is 3,794 bytes before actor data.
- Link cardinality evidence: the largest response has only two protocol values, two state values, three direction values, two link type values, and one layer value, but `labels` and `metrics` objects are effectively per-link unique in the current contract. This means enum dictionary encoding is a clear win, while full per-socket details need a columnar/table form or a separate detail plane to reduce raw uncompressed size.
- A 20,000 actor / 300,000 link target appears realistic under 100 MB only with an array-first, dictionary/columnar raw contract:
  - Measured on the largest captured response, core link rows encoded as arrays with enum indexes cost about 17 bytes/link before adjusting for larger actor-index digit width.
  - Link rows with core fields plus timestamps cost about 21 bytes/link before larger actor-index adjustment.
  - A full columnar/dictionary link row including endpoint, label, and metric fields cost about 222 bytes/link on the captured sample. Projected to 300,000 links this is about 66.7 MB, plus about 1.6 MB extra for wider actor indexes when actor indexes grow from two digits to up to five digits.
  - Sample actor rows without embedded tables cost about 180 bytes/actor with dictionary/columnar encoding, but actor richness is the main uncertainty. At 20,000 actors, 500 bytes/actor is about 10 MB, 1,000 bytes/actor is about 20 MB, and 1,500 bytes/actor is about 30 MB.
  - Therefore, a full-detail compact raw payload is expected to stay under 100 MB if average compact actor rows remain below roughly 1.5 KB and actor tables are not embedded in the graph actor list.
- Array-first encoding alone is insufficient if it still carries object-shaped per-link endpoint/detail payloads. The captured response dropped from 126.9 MB links to 110.2 MB links when only top-level link keys were removed but `src`, `dst`, `labels`, and `metrics` remained object-shaped. The large reduction appears only when link details are represented as columnar arrays and string/enum dictionaries.
- Socket-count scale evidence:
  - Linux sockets are file descriptors from the application point of view. `socket()` returns a file descriptor, and `accept()` creates a new connected socket with a new file descriptor. Therefore established TCP connection count is bounded in practice by per-process file descriptor limits, system-wide file-handle limits, TCP memory, socket buffers, and application architecture.
  - Linux kernel documentation defines `ip_local_port_range` as the automatic local port range for TCP/UDP. The documented default is `32768 60999`, which is 28,232 ports. A single client local IP talking to one server IP:port is therefore normally limited to about 28k simultaneous outbound TCP connections before local ephemeral-port exhaustion, unless the range, source IPs, namespaces, or explicit binding strategy differ.
  - A listening server is not bounded to 28k total inbound connections on one listening port. Each accepted TCP socket is identified by the full tuple. The practical upper bound becomes roughly client-source-IP fanout times client ephemeral-port availability, then file descriptors and memory. Many client IPs can therefore produce hundreds of thousands or millions of accepted sockets on one server.
  - Linux kernel documentation also exposes `tcp_max_tw_buckets`, `tcp_mem`, `tcp_max_orphans`, and TCP hash-bucket controls. These make TIME_WAIT/orphan/memory pressure part of worst-case socket inventory, not just established application-owned sockets.
  - The current workstation values at investigation time were `ip_local_port_range=32768 60999`, `tcp_max_tw_buckets=262144`, `somaxconn=4096`, `tcp_max_syn_backlog=4096`, `ulimit -n=524288`, `fs.nr_open=2147483584`, and effectively unlimited `fs.file-max`. These values are local evidence only, not product defaults.
  - `network-viewer` configures local-sockets collection with namespaces enabled, inbound/outbound enabled by default, all IPv4/IPv6 TCP/UDP protocols enabled by default, and local/listen graph output disabled by default. The helper reads `/proc/net/tcp`, `/proc/net/udp`, `/proc/net/tcp6`, and `/proc/net/udp6`, and the netlink path requests all socket states. Therefore the topology payload can reflect total socket inventory across host and container network namespaces, not only one process or one namespace.
  - Using the largest captured response as a sizing baseline, the old detailed payload costs about 1,725 bytes per emitted link including actor data and about 1,631 bytes per link for `data.links` alone. The 100 MiB Function cap is therefore crossed around 60k-64k links with the old format, matching the observed failure class.
  - The local lossless prototype costs about 522 bytes per socket-equivalent row on the captured response and would cross 100 MiB around 200k rows if the same actor/table shape repeats. The target columnar/dictionary estimate of about 222 bytes per full socket row supports about 236k rows in 50 MiB, 378k rows in 80 MiB, and 472k rows in 100 MiB before actor/dictionary overhead.
  - Phase 1 should therefore be engineered for hundreds of thousands of socket evidence rows per node in one compact response, while phase 2 chunking is required for honest million-socket scale.

Risks:

- Network-viewer is a C plugin; malformed topology building can cause a hard process exit if memory ownership, iterator invalidation, or JSON building is wrong.
- Cloud-proxied Function errors may hide whether the agent, plugin, or Cloud bridge failed. Direct-agent validation may be needed, but only through token-safe bearer handling.
- Raw topology payloads may contain process names, private IPs, hostnames, container identifiers, and other customer- or infrastructure-identifying data. They must stay in `.local/`.
- Raising parser limits alone would move the failure boundary but would not fix the root problem: topology is shipping tens of thousands of near-duplicate graph links and 100+ MB JSON for fewer than 50 actors.

## Pre-Implementation Gate

Status: open for schema documentation, lab work, validator/helper rails, and first Go producer migration; remaining producer/UI/aggregator implementation remains gated by the schema contract and validation evidence.

Problem / root-cause model:

- A Cloud-proxied `topology:network-connections` request can begin producing data but then fail with HTTP 503 and an agent-side `AgentError` saying the plugin exited before responding. Current evidence points to an oversized Function response operational cliff: the reported nodes can produce 110-134 MB topology JSON responses, while the Agent `pluginsd` parser has a 100 MiB deferred Function response cap. The completed responses contain few actors but tens of thousands of socket links because `processes:by_name` and `endpoints:by_ip` collapse actors but the backend still emits per-socket/per-port links. Root cause is not yet closed because the same token-safe API path returned HTTP 200 during investigation, so the failure is intermittent or path-dependent rather than deterministic.

Evidence reviewed:

- User-provided browser console evidence: HTTP 503 on `topology:network-connections` plus `AgentError` message `The plugin that was servicing this request, exited before responding.`
- User-provided second request: HTTP 404 for the same Function on a different node.
- Code search evidence: `src/collectors/network-viewer.plugin/network-viewer.c` defines `NETWORK_TOPOLOGY_VIEWER_FUNCTION` as `topology:network-connections`.
- Project skill evidence: topology Functions are live Function snapshots and should not block collection loops.
- Cloud probe evidence: raw bodies stored only under `.local/audits/network-connections-topology/`; durable artifacts record only aliases and aggregate counts.
- Agent Function protocol evidence: `src/plugins.d/pluginsd_parser.h:18,195-207` enforces the 100 MiB deferred response cap, and `src/plugins.d/pluginsd_functions.c:96-97` emits the exact 503 error message observed by the browser when an in-flight Function has no body.
- Network-viewer Function evidence: `src/collectors/network-viewer.plugin/network-viewer.c:1502-1557` prepares dictionaries and calls `local_sockets_process()`, `src/collectors/network-viewer.plugin/network-viewer.c:2703-2727` ignores cancellation state and sends the result only after full JSON generation, `src/collectors/network-viewer.plugin/network-viewer.c:1242-1253` collapses process actors by name, `src/collectors/network-viewer.plugin/network-viewer.c:1359-1370` keeps per-socket/per-port link keys, and the completed response stats show no orphan actors for captured successful payloads.
- Cloud routing evidence: `cloud-charts-service/http/http.go:145-147,264-270`, `cloud-charts-service/internal/service/agent_data.go:565-585`, `cloud-charts-service/internal/model/errors.go:15`, and `cloud-charts-service/internal/routing/node_instance_filter.go:35-37` show the browser 404 is produced before or during Cloud node-instance selection, not by the network-viewer plugin itself.

Affected contracts and surfaces:

- `topology:network-connections` Function output.
- `topology:streaming` Function output.
- `topology:snmp` Function output.
- Shared topology detailed/aggregated schema versioning and compatibility contract.
- Network-viewer plugin process stability.
- Cloud `/api/v2/nodes/{node}/function` proxy behavior as observed by users.
- Cloud topology UI rendering and error handling.
- Cloud topology aggregation microservice API and data model.
- Function protocol compliance for topology responses.

Existing patterns to reuse:

- Network-viewer's existing `network-connections` table Function and `topology:network-connections` registration path.
- `src/plugins.d/FUNCTION_UI_SCHEMA.json` and Function UI reference for validating envelopes.
- Token-safe query wrappers from `query-netdata-agents/scripts/_lib.sh`.
- `.local/audits/` for raw, gitignored Cloud responses and sanitized summaries.

Risk and blast radius:

- Any fix in `network-viewer.c` can affect both topology and table `network-connections` Functions.
- A fix that suppresses crashes by dropping data may hide real topology evidence and cause orphan or missing actors.
- Direct production querying can expose sensitive data. Only sanitized aggregates and aliases are allowed in durable artifacts.

Sensitive data handling plan:

- Do not copy browser cookies, session tokens, Cloud tokens, bearer tokens, claim IDs, raw node UUIDs, raw machine GUIDs, raw hostnames, raw process command lines, raw IP addresses, or raw topology payloads into SOWs, specs, docs, skills, code comments, commits, or PR text.
- Store raw API responses only under `.local/audits/network-connections-topology/`, which is gitignored.
- Use aliases such as `node-503`, `node-404`, `endpoint-a`, and `process-a` in durable artifacts.
- If a fixture is needed, derive a sanitized minimal reproducer with placeholder UUIDs and documentation that it is sanitized.

Implementation plan:

1. Keep SOW-0002 as the future unified merge/correlation SOW while this SOW owns the immediate topology Function payload migration.
2. Define the new shared topology schema, including `detailed`, `aggregated`, compatibility versioning, extension handling, typed sentinels, actor/link/table columnar sections, actor-scope/grouping metadata, and canonical round-trip rules.
3. Update Function schema/reference artifacts for the new topology payload contract.
4. Add the `aggregated`/`detailed` request mode to `topology:network-connections`, with `aggregated` as the default. The exact parameter name remains to be finalized in the schema, but accepted mode values are fixed by user decision.
5. Update `topology:network-connections`, `topology:streaming`, and `topology:snmp` to emit the new schema.
6. Update the Cloud frontend to support both old and new schemas; isolate old-schema detailed normalization and frontend aggregation in compatibility modules so removal is straightforward.
7. Design and implement a separate Cloud topology aggregation microservice that consumes detailed topology payloads and returns requested aggregated views.
8. Preserve the incident-investigation validation for 503/404/orphan actors as regression coverage.

Validation plan:

- Token-safe Cloud Function checks for `node-503` and `node-404`.
- Function discovery check for both nodes.
- Source-level same-failure scan for topology actors emitted without corresponding links.
- Compile or targeted test path for network-viewer/plugin Function code if a patch is made.
- Schema validation of any captured successful topology response when feasible.
- Golden old-detailed <-> new-detailed round-trip tests.
- Synthetic edge-case detailed payload tests for sentinels, dynamic fields, sparse columns, duplicate-looking links, large indexes, and table-heavy actors.
- Fuzz/property tests for lossless detailed conversion.
- Cross-function schema tests for `topology:network-connections`, `topology:streaming`, and `topology:snmp`.
- Cloud frontend compatibility tests for old schema path, new detailed path, and new aggregated path.
- Cloud topology aggregation microservice tests proving requested aggregations are derived from detailed payloads and do not mutate the detailed source of truth.

Artifact impact plan:

- AGENTS.md: likely unaffected unless the project-wide topology schema migration workflow needs a new guardrail.
- Runtime project skills: update `project-writing-collectors` with any reusable topology Function schema/validation rules exposed by the migration.
- Specs: add/update a topology detailed/aggregated schema spec and record the lossless detailed invariant.
- End-user/operator docs: update only if the public Function/query contract changes for users or downstream integrations.
- End-user/operator skills: update `query-netdata-cloud` how-to coverage for `topology:network-connections` and the new detailed/aggregated request mode.
- SOW lifecycle: SOW-0002 overlap is resolved by Decision 5; SOW-0012 implementation sequencing is unblocked by the user's 2026-05-06 statement that SOW-0012 is done. The SOW-0012 lifecycle file is still physically under `.agents/sow/current/` in this working tree and should be closed separately if needed.

Open-source reference evidence:

- Linux kernel documentation:
  - `https://docs.kernel.org/networking/ip-sysctl.html` documents `somaxconn`, `ip_local_port_range`, `tcp_max_tw_buckets`, `tcp_mem`, `tcp_max_orphans`, and TCP socket hash-bucket controls.
  - `https://docs.kernel.org/admin-guide/sysctl/fs.html` documents `file-max`, `file-nr`, and `nr_open`.
- Linux man-pages:
  - `https://man7.org/linux/man-pages/man2/socket.2.html` documents that `socket()` returns a file descriptor.
  - `https://man7.org/linux/man-pages/man2/accept.2.html` documents that `accept()` creates a new connected socket and returns a new file descriptor, and that `EMFILE`, `ENFILE`, and `ENOBUFS`/`ENOMEM` are practical limit failures.
- Apache Arrow official format docs (`https://arrow.apache.org/docs/format/Columnar.html`) describe a language-independent columnar format with data adjacency, random access, typed arrays, nested layouts, and dictionary encoding for repeated values. This supports the columnar/dictionary direction, but Netdata should keep JSON compatibility for Function transport unless a separate binary transport is explicitly chosen.
- open-telemetry/otel-arrow @ 78856dcb2ecd93270265296c7c279cd9ab877e24:
  - `docs/otap_basics.md:53-68` splits one semantic telemetry signal across multiple normalized tables with foreign keys so it can be reconstructed.
  - `docs/otap_basics.md:136-149` records that dictionary encoding is an encoding choice and can vary by data characteristics.
- grafana/grafana @ 1a416ef1a8724349c2c16b37f64bcf194a3d9b68:
  - `public/app/plugins/panel/nodeGraph/utils.ts:65-128` reads node and edge data as typed fields.
  - `public/app/plugins/panel/nodeGraph/utils.ts:134-185` transforms node/edge frames into layout objects for rendering.
  - `public/app/plugins/datasource/tempo/graphTransform.ts:76-137` builds separate node and edge data frames for a service-map graph.

Open decisions:

- Parameter naming for the `aggregated`/`detailed` request mode remains to be finalized.
- Cloud topology aggregation microservice repository, ownership, API route shape, deployment model, and persistence/caching behavior remain to be finalized.

## Implications And Decisions

### Decision 1 - Detailed vs Aggregated Contract Terminology

Date: 2026-05-06

Decision:

- Use the existing `network-connections` terminology: `aggregated` and `detailed`.
- `aggregated` is a UI-oriented view only.
- `detailed` is the canonical full-fidelity view for both UI drilldown and cross-node Cloud correlation.
- Any new compact `detailed` topology payload must preserve every piece of information currently available in the old detailed payload.
- A converter must be able to transform old detailed payloads to new detailed payloads and new detailed payloads back to old detailed payloads without information loss.

Implications:

- Compact detailed encoding may change representation, but not semantics.
- Array/dictionary/columnar encoding is acceptable only if it preserves field names, field values, missing-vs-null-vs-empty distinctions where meaningful, row order where meaningful, dynamic fields, actor/link tables, endpoint details, labels, metrics, timestamps, and all data needed for future aggregation or correlation.
- Aggregation must be derived from detailed data. It must not become the source of truth for correlation.
- Any backend aggregation for UI must remain separate from detailed raw/correlation payloads.

Risks:

- A compact schema with fixed columns only would silently lose future or function-specific fields unless it includes an extension path.
- Treating `aggregated` as suitable for Cloud correlation would destroy per-link/per-socket evidence and reduce future correlation options.

Validation requirement:

- Add round-trip compatibility tests when implementing the schema: old detailed -> new detailed -> old detailed must be canonically equivalent, and new detailed -> old detailed -> new detailed must preserve the same information model.

### Decision 2 - Lossless Detailed Payload Validation Strategy

Date: 2026-05-06

Decision:

- Losslessness must be validated through a canonical information model, not byte-for-byte JSON equality.
- Tests must include both directions:
  - old detailed -> canonical -> new detailed -> canonical -> old detailed -> canonical
  - new detailed -> canonical -> old detailed -> canonical -> new detailed -> canonical
- Every canonical representation in each chain must compare equal.

Required test properties:

- Preserve all known fields.
- Preserve unknown/dynamic fields through extension storage.
- Preserve missing, null, empty string, empty array, empty object, zero, and false distinctly where the old payload can distinguish them.
- Preserve actor IDs, link IDs or reconstructable link identity, endpoints, labels, metrics, timestamps, actor tables, link tables, row ordering where semantically meaningful, and all cross-reference integrity.
- Preserve enough information that all old aggregations and future cross-node correlation can be recomputed from the new detailed payload.

Required test classes:

- Golden fixture round-trip tests using sanitized old detailed payloads.
- Synthetic edge-case fixtures for null/missing/empty, unknown fields, sparse columns, duplicate-looking links, large actor indexes, high cardinality labels, and table-heavy actors.
- Property/fuzz tests that generate detailed payloads, round-trip them both ways, and assert canonical equality.
- Field coverage tests that recursively enumerate old detailed JSON paths and fail when a path is not mapped, explicitly preserved as extension data, or intentionally rejected by a recorded decision.
- Differential behavior tests that run the old aggregation/correlation logic on old detailed and on reconstructed old detailed from new detailed, then compare results.

Implications:

- The new compact detailed schema needs a typed extension path. Fixed arrays alone are not sufficient unless unknown fields have a lossless place to live.
- The converter must be part of the contract and must be tested in CI alongside topology Functions and UI normalizers.
- Aggregated payloads are not eligible for this lossless guarantee; only detailed payloads are.

### Decision 3 - Topology Schema Migration Scope

Date: 2026-05-06

Decision:

- The Function schema must be updated for the new topology payload contract.
- `topology:network-connections` must expose an `aggregated`/`detailed` request mode using the existing `network-connections` terminology.
- `aggregated` is the default mode for `topology:network-connections`.
- `detailed` remains the full-fidelity lossless payload used by UI drilldown and Cloud cross-node correlation.
- All topology Functions must emit the new schema:
  - `topology:network-connections`
  - `topology:streaming`
  - `topology:snmp`
- The Cloud frontend must support both old and new schemas.
- Frontend aggregation must be maintained only for the old-schema compatibility path.
- Old-schema detailed normalization and aggregation code must be isolated so it is easy to remove after the compatibility window.
- Cloud topology aggregation must be implemented as a separate microservice. It consumes detailed topology payloads, aggregates topologies, and outputs the aggregation requested by the user.

Implications:

- The new schema is not a `topology:network-connections`-only change. It becomes a shared topology contract across network connections, streaming, and SNMP.
- The UI should not pay the old raw detailed payload cost for normal graph rendering once the new aggregated path is available.
- Cloud correlation must use detailed payloads, not aggregated views.
- The Cloud topology aggregation microservice must be able to recompute requested views from detailed topology data without mutating or weakening the detailed source of truth.
- Backward compatibility in the Cloud frontend is required during rollout because old Agents and new Agents will coexist.

Risks:

- This is now a cross-repository migration touching Agent Functions, Function schemas, Cloud frontend, and a new Cloud aggregation service.
- Supporting both old and new schemas can add long-lived compatibility code unless the old path is explicitly isolated and scheduled for removal.
- If all topology Functions are not migrated consistently, Cloud aggregation will need per-function adapters and will become harder to reason about.

Validation requirement:

- Schema conformance tests must cover all topology Functions.
- Frontend tests must prove old-schema aggregation remains only in the old compatibility path.
- Cloud aggregation service tests must show requested aggregated outputs are reproducible from detailed inputs.
- Rollout tests must include mixed old/new Agent responses.

### Decision 4 - Cloud Aggregation Location

Date: 2026-05-06

Decision:

- Cloud topology aggregation will be implemented as a separate Cloud microservice.
- It will not be implemented inside charts-service.

Context:

- charts-service ownership was considered and recorded earlier on 2026-05-06.
- The backend team changed direction and now prefers a separate service boundary for this capability.
- charts-service still participates in Cloud Function routing for `/api/v2/nodes/{nodeID}/function` and node-instance selection, as recorded in the investigation evidence, so service integration with charts-service remains part of the design.

Implications:

- The SOW now includes a new Cloud service boundary, with repository, ownership, deployment, API, authentication/authorization, observability, and scaling decisions.
- The aggregation service API, caching, request fan-out, and error handling must integrate cleanly with existing Cloud services, including charts-service where routing/proxy context is needed.
- charts-service is no longer the implementation home for aggregation logic, but its integration path remains in scope.

Risks:

- A new service boundary adds deployment, operational, authentication, authorization, latency, retry, and observability work.
- The service must bound CPU/memory use for detailed payload fetch, decoding, aggregation, and caching.
- The integration path must avoid blocking or degrading existing charts/function proxy traffic.

Validation requirement:

- Add service-level tests for requested topology aggregations.
- Add integration tests for charts-service/service interaction where charts-service participates in request routing.
- Add resource-bound tests or benchmarks for large detailed inputs.
- Validate that service errors preserve enough detail to distinguish node-instance routing failures, Agent Function failures, and aggregation failures.

### Decision 5 - SOW-0002 / SOW-0020 Scope Boundary

Date: 2026-05-06

Decision:

- SOW-0020 owns the immediate topology Function payload migration:
  - detailed and aggregated payload contract;
  - lossless compact detailed representation;
  - old/new schema compatibility;
  - `topology:network-connections`, `topology:streaming`, and `topology:snmp` emission requirements;
  - Cloud frontend compatibility requirements;
  - the separate Cloud topology aggregation microservice contract.
- SOW-0002 remains pending and owns future unified topology merge semantics:
  - same-kind and cross-kind merge algorithms;
  - identity matching;
  - conflict resolution;
  - storage/indexing;
  - unified cross-layer view behavior.
- SOW-0002 will consume SOW-0020 detailed payloads as source evidence when it eventually implements merge/correlation. It does not block SOW-0020 unless SOW-0020 would remove information needed for SOW-0002.

Evidence:

- SOW-0002 is still `open` and explicitly blocked on merge semantics, identity matching, conflict resolution, storage model, scale targets, and L7 process granularity.
- SOW-0020 has concrete incident evidence that current topology payloads can exceed practical Function limits and needs a compact, lossless detailed contract now.
- User decisions already recorded in this SOW require detailed payloads to remain full-fidelity and require aggregation to be derived from detailed data.

Implications:

- This SOW can proceed with the payload schema/spec and network-connections incident fixes without solving cross-layer merge semantics.
- The detailed schema must keep enough extension capacity and source fidelity that SOW-0002 can later implement merge/correlation without another immediate payload rewrite.
- SOW-0002 acceptance criteria that mention a unified schema depend on the SOW-0020 schema once this migration ships.

Risks:

- If SOW-0020 over-specializes the detailed schema for current UI rendering, SOW-0002 may need a schema vNext before merge work can proceed.
- If SOW-0002 later needs correlation-specific provenance that SOW-0020 did not preserve, detailed payload compatibility tests will pass but future merge quality will suffer.

Mitigation:

- Treat `detailed` as the canonical evidence plane, not a render model.
- Preserve unknown/dynamic fields and table data through typed extension storage.
- Keep `aggregated` view-only and explicitly non-authoritative for correlation.

### Decision 6 - Detailed Payload Socket Evidence And Footprint

Date: 2026-05-08

Decision:

- The most important design goal for the new topology payload is minimizing footprint without losing evidence needed for Cloud-side cross-node matching.
- `detailed` does not mean "one rendered graph edge per socket".
- `detailed` may safely aggregate the graph projection when the aggregation is lossless with respect to reconstructing or correlating the underlying sockets.
- `detailed` must preserve per-socket evidence one by one because Cloud needs socket tuples from multiple nodes to match both sides of a connection.
- Per-socket evidence should move out of repeated object-shaped graph links and into a compact detail plane, such as a columnar `socket_rows` / `socket_evidence` table keyed back to the aggregated graph edge.
- `aggregated` remains the view-oriented payload and does not carry the full per-socket evidence table.

Evidence:

- Current `network-viewer` topology link keys include process identity, local endpoint, remote endpoint, protocol, direction, state, and ports. This preserves correlation evidence but explodes link cardinality when actors are grouped.
- Client-side outbound sockets can share the same server endpoint and can be graph-aggregated for rendering, but their local endpoint still matters for matching against server-side inbound observations.
- Server-side inbound sockets see remote endpoint tuples; without the matching client-side local endpoint tuples, Cloud cannot prove which remote process owns each inbound socket.

Implications:

- The detailed payload should be structured as:
  - compact actor records grouped by selected actor scope;
  - compact graph links aggregated by actor pair / endpoint / protocol / direction / state as appropriate;
  - compact per-socket evidence rows that retain local endpoint, remote endpoint, protocol, direction, state, owner identity, namespace/container identity when available, and metric summaries needed for correlation.
- The UI can render from the aggregated graph projection without inflating links to one row per socket.
- Cloud correlation can use the per-socket evidence table to match observations from different nodes.
- Future actor scopes such as container and Kubernetes labels require annotations/enrichment, but the payload contract should already allow those scope keys.

Risks:

- If `detailed` drops local client endpoint data during outbound aggregation, Cloud cannot reliably match those sockets to server-side inbound rows.
- If per-socket evidence remains embedded in every graph link as repeated JSON objects, the payload-size problem remains.
- If actor scope keys are not explicit, grouping by process name, PID, container, or Kubernetes labels will be hard to compare across agents and Cloud services.

Validation requirement:

- Add fixtures where many outbound sockets from one actor share one server endpoint and prove the rendered graph collapses while the socket evidence still preserves every local endpoint tuple.
- Add fixtures where server inbound observations are matched to client outbound observations through the preserved socket evidence.
- Add payload-size checks that fail if detailed output regresses toward one object-shaped graph link per socket.

### Decision 7 - Phase 1 Payload Budget And Phase 2 Chunking

Date: 2026-05-08

Decision:

- Paged/chunked socket evidence is the correct long-term answer when a lossless detailed topology response still cannot fit safely in one Function response.
- Paged/chunked socket evidence is phase 2, not the immediate phase 1 implementation.
- Phase 1 must do everything practical to prevent reaching the single-response limit:
  - keep `aggregated` as the default view;
  - keep graph links aggregated for rendering;
  - move per-socket evidence into compact columnar/dictionary tables;
  - avoid repeated object-shaped per-socket graph links;
  - avoid repeated strings where indexes/enums are sufficient;
  - keep actor tables and socket-evidence tables separate from the graph projection;
  - add payload-size regression tests and size budgets.
- If phase 1 still cannot produce a complete lossless detailed payload within the safe response budget, it must fail explicitly rather than silently truncate.
- Truncating per-socket evidence is not allowed because it breaks Cloud cross-node matching and can create false topology conclusions.

Implications:

- Phase 1 remains compatible with the current Function response model while reducing the probability of hitting the hard response cap.
- The phase 1 schema must be designed so phase 2 chunking can add cursors/pages for socket evidence without another semantic rewrite.
- The UI must not depend on receiving all per-socket rows for normal graph rendering; rendering should use the compact graph projection.
- Cloud correlation can consume phase 1 detailed payloads where they fit, and phase 2 chunking will provide the scale-out path for larger estates.

Risks:

- Very large servers or very large Cloud selections may still exceed the safe response budget even after compact encoding.
- If phase 1 schema couples graph rows and socket evidence too tightly, phase 2 chunking will require another disruptive schema migration.

Validation requirement:

- Add large synthetic payload tests that estimate serialized size for high socket counts and fail if the compact detailed format regresses.
- Add an explicit over-budget behavior test proving the producer fails clearly and does not return partial socket evidence as complete data.
- Add schema compatibility checks that reserve the phase 2 extension path for socket-evidence pagination/chunking.

### Decision 8 - NAT/LB Matching Boundary

Date: 2026-05-08

Decision:

- NAT, load-balancer, proxy, and masquerade correlation are out of scope for the current phase.
- Cloud-side socket matching must use only exact observable socket evidence available from the endpoints.
- When observable tuples cannot prove both sides of a connection, the topology must leave the edge unresolved rather than infer a process-to-process match.
- NAT/LB/proxy-aware matching requires future evidence sources such as flow data, proxy telemetry, conntrack/NAT metadata, or explicit service annotations.

Implications:

- The current implementation should be truthful before it is complete.
- Exact tuple matching can build reliable process-to-process edges where both sides expose compatible local/remote endpoint evidence.
- NAT/LB/proxy paths may remain as endpoint or infrastructure edges until a future SOW adds the required evidence.

Risks:

- Users may see incomplete cross-node process maps behind NAT, load balancers, proxies, service meshes, or SNAT-heavy Kubernetes paths.
- Heuristic matching would make the graph look more complete but would introduce false positives during incident analysis, so it is explicitly rejected for this phase.

Validation requirement:

- Add matching tests proving exact observable tuple matches are accepted.
- Add NAT/LB-like fixtures proving non-proven edges remain unresolved and are not heuristically matched.

### Decision 9 - Multi-Level Actor Scopes For Infrastructure Topologies

Date: 2026-05-09

Decision:

- The detailed topology payload must separate evidence from presentation/grouping.
- Cloud aggregation must be able to materialize different actor scopes from the same detailed evidence, without requiring a different Agent payload for each view.
- Required actor scopes include:
  - node-level infrastructure dependency maps, for large pet-style fleets where users need to see which nodes depend on which nodes;
  - container/application-level dependency maps, for large Kubernetes clusters where users need to see which applications or container names depend on which other applications or container names, not individual container instances;
  - process-name-level dependency maps, for large HPC nodes where users need to see which process names depend on which other process names, not individual PIDs;
  - PID-level drilldown, where exact process-instance evidence is needed.
- The schema must preserve raw evidence needed to derive those scopes:
  - Netdata node identity and host/network identity;
  - process name, PID, UID, command line where available, and network namespace identity;
  - container identity, container name, image, pod, namespace, workload, and Kubernetes labels when enrichment is available;
  - local/remote socket endpoint tuples and direction/state/protocol evidence.
- Actor scopes must be explicit metadata in the payload or aggregation request. They must not be inferred only from actor ID string formats.
- Container and Kubernetes label support is an enrichment requirement. The current network-viewer producer does not yet have enough evidence for those scopes, but the schema must be ready for them.

Evidence:

- Current `topology:network-connections` supports process grouping by process name by default and PID when requested.
- Current network-viewer classifies socket namespace type as `system`, `unknown`, or `container` from network namespace identity, but does not yet expose container name, pod, workload, or Kubernetes labels.
- Current shared topology `match` schema already has fields for node, container, pod, and namespace identities, and allows additional match properties.

Implications:

- The Agent detailed payload should behave like an evidence plane. It should expose compact rows with stable dimensions and enrichment columns, not only pre-rendered actor/link objects.
- The Cloud aggregator should own materialized views such as `group_by=node`, `group_by=process.name`, `group_by=process.pid`, `group_by=container.name`, `group_by=k8s.workload`, or selected Kubernetes labels.
- Aggregated payloads should state the actor scope used to produce them, so users and downstream systems know what each node in the graph represents.
- The same per-socket evidence can support infrastructure maps, application maps, and process maps when the required identity columns are present.

Risks:

- If the schema bakes in only process actors, it will not serve infrastructure-level or Kubernetes-level topology without another migration.
- If grouping scope is inferred from display labels, Cloud can merge unrelated actors that happen to share a name.
- Container-name grouping can intentionally merge many instances, so the payload must preserve instance-level evidence for drilldown and cross-checking.

Validation requirement:

- Add schema fixtures proving the same detailed socket evidence can produce node-level, process-name-level, and PID-level aggregations.
- Add synthetic enriched fixtures proving container-name and Kubernetes-label grouping can be derived without changing the detailed evidence contract.
- Add collision tests where two different actor instances share the same display name but must remain distinguishable in detailed evidence.

### Decision 10 - Actor Drilldown Tables Without Duplicating Evidence

Date: 2026-05-09

Decision:

- Actor drilldown tables are a first-class topology requirement.
- Every materialized actor scope must be able to expose per-actor drilldown tables that list exact dependencies and supporting evidence.
- Drilldown tables must support both:
  - aggregated rows, such as one row per peer actor, peer node, peer container, endpoint, protocol, direction, or state with counts and summaries;
  - exact rows, such as socket evidence rows, when the user needs to inspect the underlying connections.
- The schema must avoid duplicating the same detailed socket/link evidence inside every actor.
- Actor tables should be materialized views over shared compact evidence, using table definitions, row references, row ranges, filters, or indexes where possible.
- When an aggregated actor table row summarizes many sockets, it must carry enough references or drilldown keys to retrieve or reconstruct the exact rows from the detailed evidence plane.

Evidence:

- The current shared topology schema already allows `actor.tables`, so actor-specific drilldown data is part of the existing contract.
- The Cloud frontend actor modal reads presentation table definitions and renders `source: "data"` tables from `node.details.tables[tableKey]` or `source: "links"` tables from incident graph links.
- Current `topology:network-connections` emits per-process `tables.sockets` rows directly under each process actor, which satisfies the drilldown UX but duplicates information already present in link/socket evidence.

Implications:

- The new compact schema should keep the modal/table capability, but move repeated row payload out of actor objects.
- Presentation metadata should define table columns and sources, while data sections should provide compact shared table storage and per-actor indexes into it.
- Aggregated topology responses can expose aggregated drilldown rows, but detailed responses must preserve the exact evidence needed to expand them.
- Frontend compatibility code can reconstruct old `actor.details.tables` for old UI paths, but new-schema UI should prefer shared table sections and references.

Risks:

- If actor tables remain embedded as full object arrays, large actors will recreate the same payload-size problem even after graph links are compacted.
- If aggregated table rows do not reference the underlying evidence, users will lose trust because they cannot drill from summary to exact dependency rows.
- If table definitions are tied to one actor scope, Cloud will need separate payload shapes for node, container, process, and PID views.

Validation requirement:

- Add fixtures where a high-cardinality actor has many socket rows and prove modal drilldown works without duplicating full rows under the actor.
- Add tests proving aggregated table rows preserve counts/summaries and can drill down to exact socket evidence rows.
- Add compatibility tests proving old-schema `actor.tables` can still be normalized while new-schema shared table references render the same user-visible rows.

### Decision 11 - Actor Custom Tables vs Relationship Evidence Tables

Date: 2026-05-09

Decision:

- The topology schema must differentiate table semantics, not only table rendering source.
- `source: "data"` and `source: "links"` remain presentation/rendering hints, but they are not sufficient as aggregation semantics.
- Every table definition or table section should declare a semantic role, such as:
  - `relationship_evidence`: exact rows that describe dependencies or links and can be grouped/rolled up, such as socket evidence;
  - `relationship_summary`: aggregated dependency rows derived from evidence, such as peer actor summaries with counts and drilldown keys;
  - `actor_detail`: actor-owned custom data that describes the actor and is not generally aggregatable, such as streaming path, retention, status, capabilities, or local inventory;
  - `actor_inventory`: actor-owned lists that may be searchable/displayable but should not be treated as dependency evidence unless explicitly referenced by a relationship table.
- Every table should also declare an aggregation policy:
  - `none`: preserve per actor, do not merge across aggregated actors unless explicitly requested;
  - `derive`: recompute from detailed relationship evidence for the requested actor scope;
  - `rollup`: combine rows with declared group keys and measures;
  - `reference`: display rows by references/ranges/filters into a shared table section.
- Custom actor tables may still be stored compactly in shared columnar sections with an owner actor column, but their semantic role remains actor-owned detail. Storing them compactly must not imply they are aggregatable relationship evidence.
- When Cloud aggregates actors, relationship tables may be recomputed for the aggregated actor. Actor-detail/custom tables must either remain attached to their original member actors, be exposed as drilldown/member details, or use an explicit table-specific rollup policy.

Evidence:

- Streaming topology `streaming_path` is declared as a data table for the actor modal, but its rows describe the actor's path metadata, not an aggregatable dependency list.
- Streaming topology retention/status tables are actor details or operational summaries, not always link evidence.
- Network-connections socket rows are dependency evidence and can support exact drilldown plus aggregated summaries.
- The current schema's `source` enum only says where the frontend reads rows from, not whether the rows are actor-owned custom data or relationship evidence.

Implications:

- The new schema needs table metadata beyond `source`, for example `role`, `aggregation_policy`, `owner_scope`, `evidence_ref`, `group_keys`, and `measures`.
- The Cloud aggregator must not blindly merge custom actor tables when materializing node/container/process-level views.
- UI can still render all table roles in the same modal, but Cloud services need the role/policy fields to avoid incorrect aggregation.
- Compatibility adapters can map old `source: "data"` tables to conservative defaults:
  - network-connections `sockets` -> `relationship_evidence`;
  - streaming `streaming_path` -> `actor_detail`;
  - streaming `retention` -> `actor_detail` unless a future explicit rollup policy is added;
  - streaming `inbound` / `outbound` -> relationship or operational summary based on their declared policy.

Risks:

- If table semantics are not explicit, Cloud may aggregate custom actor metadata incorrectly and present false information.
- If all custom data remains embedded as actor-local object arrays, very large custom tables can still inflate payloads.
- If custom tables are treated as non-aggregatable without a drilldown/member path, users may lose useful details when viewing high-level aggregated actors.

Validation requirement:

- Add streaming topology fixtures with `streaming_path` and retention tables proving actor-detail tables survive schema conversion and are not merged as dependency evidence.
- Add network-connections fixtures proving socket tables are relationship evidence and can produce both aggregated dependency summaries and exact drilldown rows.
- Add mixed aggregation tests where multiple actors are grouped and custom actor-detail tables remain accessible as member details rather than incorrectly merged.

### Decision 12 - Link Direction Semantics And Aggregation Policy

Date: 2026-05-09

Decision:

- The schema must separate the link's endpoint order from the meaning of its `direction` value.
- Each link type must declare direction semantics and aggregation policy. A free-form link-level `direction` string is not enough.
- Required link-type metadata includes:
  - `orientation`: whether the graph relationship is `directed`, `undirected`, or `hierarchical`;
  - `direction_role`: what the link's direction value means, such as `flow`, `dependency`, `containment`, `observation_completeness`, or `none`;
  - `aggregation_direction_policy`: whether Cloud must `preserve` direction in aggregation keys, `ignore` direction and canonicalize endpoint pairs, or `retain_as_attribute` while aggregating independently of direction;
  - optional render hints such as whether arrows or curved parallel links are appropriate. The exact UI implementation is out of scope for this SOW, but the schema must carry the information.
- Direction-significant link types, such as network socket flows and streaming parent/child paths, must preserve direction during aggregation.
- Undirected adjacency link types, such as most L2 links, must allow Cloud to aggregate independently of direction while retaining observation metadata such as `unidirectional` or `bidirectional` when useful.
- Backward compatibility may keep the existing `direction` field, but new producers and aggregators must interpret it through the declared link-type semantics.

Examples:

- Network socket link type:
  - `orientation: directed`
  - `direction_role: flow`
  - `aggregation_direction_policy: preserve`
  - Rationale: inbound and outbound sockets have different source/destination meaning and different cross-node matching behavior.
- Streaming link type:
  - `orientation: directed`
  - `direction_role: dependency`
  - `aggregation_direction_policy: preserve`
  - Rationale: child-to-parent streaming direction is the topology relation.
- Ownership/containment link type:
  - `orientation: hierarchical`
  - `direction_role: containment`
  - `aggregation_direction_policy: preserve`
  - Rationale: parent contains child; reversing endpoints changes meaning.
- L2 discovery/adjacency link types:
  - `orientation: undirected`
  - `direction_role: observation_completeness`
  - `aggregation_direction_policy: retain_as_attribute`
  - Rationale: `unidirectional` and `bidirectional` describe whether one side or both sides reported evidence, not traffic direction. Cloud can aggregate endpoint pairs independently of direction, while preserving the observation status for details.

Evidence:

- Shared schema has an optional `direction` string on links but no declared semantics.
- Network-viewer uses socket direction as part of link identity and emits it into links and labels.
- Streaming links are direction-significant through source/destination order even without a separate `direction` field.
- SNMP/L2 projection uses `unidirectional`/`bidirectional` to describe observation completeness and merges reverse evidence into one bidirectional link.

Implications:

- The Cloud aggregator must read link-type direction metadata before deciding its aggregation key.
- UI rendering can use the same metadata to decide whether arrows/curves are meaningful, but rendering behavior will be implemented separately.
- Tables and drilldown should expose observation completeness separately from graph direction when `direction_role` is not `flow` or `dependency`.

Risks:

- If direction semantics stay implicit, Cloud may incorrectly merge directional dependencies or incorrectly split undirected L2 adjacencies.
- If the UI uses the raw `direction` field alone, it may draw arrows for L2 `unidirectional` evidence even though the user should read it as discovery completeness, not traffic direction.

Validation requirement:

- Add schema fixtures for directed socket links proving opposite directions do not collapse unless a requested aggregation explicitly allows it.
- Add streaming fixtures proving child-to-parent direction survives aggregation.
- Add L2 fixtures proving reverse discovery evidence can aggregate to one undirected adjacency while retaining `unidirectional`/`bidirectional` observation metadata.

### Decision 13 - Refreshable Link Telemetry Overlays

Date: 2026-05-09

Decision:

- The topology schema must support link-level telemetry overlays that can be refreshed without recomputing the topology graph.
- Topology responses should define telemetry overlay templates once per response, view, or link type. Links should carry only a template identifier plus compact parameters.
- Overlay templates must describe:
  - the provider kind, such as Cloud time-series metrics, direct Agent metrics, or Function-backed current snapshots;
  - the metric families exposed by the overlay, such as traffic, packets, errors, state, utilization, or future plugin-specific measurements;
  - required parameter names and their meaning, such as node IDs, host selectors, chart/context prefixes, chart suffixes, interface labels, socket tuple IDs, actor IDs, or compact dictionary references;
  - query construction rules, including required contexts, dimensions, labels/selectors, and target nodes;
  - merge rules for aggregated links, including whether parameters can be unioned, summed, counted, deduplicated, or must remain as separate query references;
  - coverage semantics, such as exact per-link, exact actor-scope, approximate actor-scope, unsupported, or snapshot-only.
- Link records should reference overlay templates through compact telemetry refs rather than embedding full query definitions.
- Aggregated links must merge telemetry refs according to template-defined merge policy. The aggregator must not infer merge behavior by string concatenating query fragments.
- If multiple telemetry refs cannot be merged safely, the aggregated link may carry multiple refs under the same overlay metric, and the UI or Cloud overlay layer can query and combine them according to the template.
- Topology payloads should distinguish static/snapshot metrics already present in the topology response from refreshable overlay definitions.

Examples:

- SNMP/L2:
  - template kind: time-series metric query;
  - parameters: monitored node or vnode, chart/context prefix, local interface chart suffix or interface labels;
  - metric families: traffic, packets, errors, operational state;
  - merge policy: traffic/packets/errors can usually sum across interfaces; state must use a state-specific rule such as worst-state, count-by-state, or separate member details, not sum;
  - coverage: exact for the selected interface rows when the SNMP collector emits the required chart/label references.
- Network-viewer process aggregation:
  - template kind: none today, future Function-backed current snapshot;
  - parameters could be actor scope plus compact socket/link evidence IDs;
  - coverage must be declared as unsupported until the plugin can provide exact per-link current traffic snapshots.
- Network-viewer container aggregation:
  - cgroup network metrics may provide container-interface time series, but they do not identify the remote peer link;
  - coverage must therefore be exact actor-scope or approximate actor-scope, not exact per-link, unless future evidence adds peer-aware counters.

Evidence:

- SNMP topology already exposes chart lookup fragments on devices and interface rows.
- Cloud metric queries must be tightly scoped by context/selector to avoid metadata explosion.
- Current topology links have only a generic `metrics` object and no refreshable query contract.
- Network-viewer currently collects socket identity and TCP summaries, but not per-link traffic time series.
- cgroup network metrics are container/interface scoped, not dependency-link scoped. The cgroups integration documents per-cgroup and per-k8s-cgroup network-device contexts such as `cgroup.net_net`, `cgroup.net_packets`, `cgroup.net_errors`, `k8s.cgroup.net_net`, and `k8s.cgroup.net_packets`: `src/collectors/cgroups.plugin/integrations/containers.md:145-172` and `src/collectors/cgroups.plugin/integrations/kubernetes_containers.md:160-194`.

Implications:

- The schema needs a dedicated overlay/telemetry contract, separate from actor tables, relationship evidence tables, and rendered graph links.
- The UI can refresh traffic by issuing targeted metric or Function queries using stable overlay refs, without calling the topology Function again.
- Cloud aggregation can merge telemetry refs at the same time it merges graph links, preserving enough information to calculate aggregate bandwidth for the aggregated edge.
- Overlay refs must be compact and dictionary-friendly because they may appear on many links.
- Overlay refs should carry coverage/confidence so the UI does not present container-level interface traffic as exact peer-to-peer link bandwidth.

Risks:

- If the schema stores full query JSON per link, the payload may grow as badly as today's repeated link objects.
- If overlay coverage is not explicit, users may see bandwidth on a link and assume exact peer traffic when the source is actually actor- or interface-level.
- If merge policies are not defined per metric family, Cloud may sum values that should use state-aware or non-additive aggregation.
- If overlay refs are tied to unstable chart IDs without stable labels or repair metadata, topology links may refresh incorrectly after interface rename, container restart, or vnode reassignment.

Validation requirement:

- Add fixtures proving many SNMP/L2 links can reference one overlay template with only per-link parameters.
- Add aggregation fixtures proving merged links union member telemetry refs and produce correct additive traffic queries.
- Add state/error fixtures proving non-additive metrics do not use additive merge rules.
- Add network-viewer fixtures proving unsupported overlays are represented explicitly and do not imply exact link traffic.

### Decision 14 - Schema Emulation Lab Before Producer/UI Migration

Date: 2026-05-09

Decision:

- Do not freeze the topology schema based only on hand-written examples.
- Build a schema emulation and benchmarking harness before updating all topology Functions and the UI.
- The harness must model required use cases at multiple scales, generate candidate payloads, run a prototype aggregator, and produce repeatable evidence for:
  - serialized payload size;
  - compressed payload size where relevant;
  - decode/encode CPU time;
  - peak memory where measurable;
  - actor/link/socket/relationship evidence counts;
  - lossless detailed round-trip correctness;
  - aggregation correctness for each required actor scope;
  - drilldown table correctness;
  - direction-preservation/canonicalization behavior;
  - telemetry overlay ref merging behavior.
- The harness output must be durable enough for review and CI regression checks, but raw captured production payloads must stay under `.local/` and must not be committed.
- Synthetic scenarios can be committed when they contain no sensitive data and no customer-identifying identifiers.
- Use read-only captures from internal Netdata-owned Kubernetes infrastructure as a real-world corpus when explicitly authorized for a capture run. Raw payloads, endpoint details, node names, hostnames, process names, IPs, cluster labels, and IDs must stay under `.local/`; only sanitized shape statistics, generated synthetic fixtures, and redacted/canonicalized summaries may be committed.
- For the current real-corpus capture, scope Cloud queries only to the user-authorized space named `Netdata Cloud`. Other spaces visible to the token are out of scope and must not be used for topology payload capture.

Required modeled scenarios:

- Real captured topology Function payloads from internal Netdata-owned Kubernetes infrastructure, used as local-only corpus inputs for size and schema-shape analysis.
- Network-viewer process-name aggregation on one node with many sockets to common destination endpoints.
- Network-viewer cross-node matching with client outbound and server inbound evidence preserved one by one.
- Network-viewer node-level infrastructure map across many nodes.
- Network-viewer enriched container/application and Kubernetes-label grouping, using synthetic enrichment until real enrichment exists.
- SNMP/L2 adjacency with unidirectional and bidirectional evidence, many devices, many ports, and metric overlay refs for traffic/packets/errors/state.
- Streaming topology with actor-owned custom tables such as `streaming_path`, plus relationship tables and directed child-to-parent links.
- Telemetry overlay aggregation where additive metrics, state metrics, unsupported overlays, and incompatible refs all behave differently.

Scale points:

- Small: developer-readable fixtures for schema review and golden tests.
- Medium: tens to hundreds of actors and thousands of links/evidence rows.
- Large: thousands of actors and hundreds of thousands of socket/interface evidence rows.
- Stress: one million socket evidence rows, expected to demonstrate whether phase 1 fits or must require phase 2 paging/chunking.

Candidate strategies to compare:

- Current object-shaped topology payload as the baseline.
- Array-of-objects with dictionaries.
- Columnar tables with dictionaries.
- Split graph projection plus evidence planes.
- Template-based telemetry overlay refs.
- Optional compression measurements, while treating compression as transport relief rather than a schema substitute.

Aggregator prototype requirements:

- Consume the same detailed candidate payload the Agent would emit.
- Produce requested views by actor scope: node, process name, PID, container/application, Kubernetes labels/workload.
- Preserve socket evidence for cross-node matching and actor drilldown.
- Apply link-type direction policy and overlay merge policy from schema metadata.
- Produce deterministic output for golden tests.

Evidence:

- Existing payload evidence already shows object-shaped per-link JSON is the primary size problem.
- Existing function validation and SNMP topology parity tests provide local patterns for schema validation, E2E checks, and golden fixtures.
- External topology systems such as Kiali, Pixie, and Coroot compute traffic/dependency views from metric or event queries, but their implementation patterns also show that aggregation semantics are source-specific and cannot be inferred from a generic link alone.
- The user has access to internal Netdata-owned Kubernetes infrastructure where existing topology Function payloads can be captured and analyzed locally. This gives the schema lab real-world payload shapes in addition to synthetic scale models, but it requires strict raw-data isolation and sanitization.

Implications:

- The schema design work becomes experiment-driven: candidate schema changes must be backed by size and correctness results before producer/UI migration starts.
- The first implementation artifact should be a topology schema lab/test package and prototype aggregator, not changes to all producers.
- The lab should support both synthetic scenario generation and importing raw local Function payloads from `.local/` so the same candidate schema and aggregator can be tested against real and synthetic data.
- Once the schema wins against the scenarios, producer and UI changes can proceed with much lower risk.

Risks:

- If the lab becomes too detached from real producer payloads, it may optimize synthetic data and miss real-world fields.
- If the lab only tests size and not semantics, it may choose a compact format that cannot support drilldown, cross-node matching, direction semantics, or overlays.
- If raw captured payloads are committed, sensitive infrastructure data may leak.
- If captures are taken from live infrastructure without clear read-only scope, rate limits, and local-only storage, the schema lab could create operational risk or expose internal topology details.

Validation requirement:

- CI-friendly tests must fail on payload-size regressions for representative scenarios.
- Golden tests must prove aggregation output is stable for each modeled use case.
- Round-trip tests must prove detailed schema alternatives preserve the canonical information model.
- Real-corpus import tests must operate on `.local/` payloads when available, but CI must use sanitized synthetic fixtures only.
- Stress tests that are too expensive for default CI may run behind an explicit build tag or local benchmark command, but their command and expected budget must be documented.

### Decision 15 - Producer Encoding Helper Strategy

Date: 2026-05-09

Decision:

- The user classified the exact helper split as an implementation detail and authorized using the safest path.
- Implement the Go topology v1 model and compact-table helper first.
- Defer the C helper API shape until the first C producer migration, so the C helper is informed by the actual `topology:network-connections` and `topology:streaming` write paths instead of guessed in advance.

Evidence:

- Go already has a central topology package, but it currently models the old object-shaped schema in `src/go/pkg/topology/types.go`.
- The SNMP topology Function returns Go topology data directly from `src/go/plugin/go.d/collector/snmp_topology/func_topology_handler.go`.
- The Network Viewer and Streaming producers write JSON manually with `BUFFER` helpers in C, and they have different table and link construction patterns.

Implications:

- Go producers and Go-side tooling can share typed compact-table construction and validation immediately.
- The first C producer migration must include the C helper decision instead of duplicating compact-table encoding by hand.
- This avoids freezing a C API before validating it against the high-cardinality Network Viewer socket evidence path and the Streaming custom-table path.

Risks:

- Until the C helper is added, C producer migration work remains blocked on a follow-up helper design step.
- If Go helper types drift from the JSON Schema, tests must catch the mismatch before producers rely on the helper.

Validation requirement:

- Add Go tests for compact-table row counts, encoding length checks, dictionary index bounds, actor/link reference columns, and JSON round-trip behavior.
- Validate committed topology fixtures against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` and the semantic checks in `src/go/tools/functions-validation/validate`.

### Decision 16 - Nested Custom Detail Table Cells

Date: 2026-05-09

Decision:

- Add an explicit `json` column type for actor/custom detail table cells that must preserve nested producer-owned data.
- Keep high-cardinality relationship evidence typed as scalar/reference/array columns whenever possible.
- Treat `json` cells as not generally aggregatable unless a table type explicitly defines a safe aggregation policy for that table.

Evidence:

- Current SNMP/L2 actor port details can include nested producer data. `src/go/pkg/topology/engine/topology_adapter_device_summary_render.go:35` emits `vlans` from `[]map[string]any`, and `src/go/pkg/topology/engine/topology_adapter_device_summary_render.go:44` builds `neighbors` as `[]map[string]any`.
- The user requirement says actor custom tables, such as streaming paths and topology-specific per-actor data, must remain supported and must be differentiated from aggregatable relationship evidence.

Implications:

- The schema can preserve existing custom actor detail tables without flattening away topology-specific structure.
- `json` columns are a compactness escape hatch for low-cardinality custom detail data, not the default representation for sockets, L2 observations, or other high-cardinality evidence.
- Aggregators may append or retain `json` detail rows according to table policy, but should not infer generic merge semantics from arbitrary nested values.

Risks:

- Overusing `json` columns in evidence tables would recreate object-shaped payload bloat and weaken aggregation semantics.
- Producers must still prefer typed scalar/reference columns when a value participates in identity, matching, grouping, or metric aggregation.

Validation requirement:

- Schema tests must include a typed topology v1 payload with a `json` custom detail column containing nested arrays/objects.
- Producer docs and skills must warn against using `json` for high-cardinality evidence when typed columns are possible.

## Plan

1. Keep SOW-0002 as the future unified merge-semantics SOW while this SOW owns the immediate shared topology payload schema migration.
2. Build the topology schema emulation lab and prototype aggregator with representative scenarios, scale profiles, and size/correctness reports.
3. Use the lab to compare candidate schemas and freeze the detailed/aggregated topology contract with measured evidence.
4. Draft the new detailed/aggregated topology schema, including compatibility versioning, typed sentinels, dictionaries, columnar sections, extension preservation, table handling, and refreshable telemetry overlay templates.
5. Promote the local lossless converter prototype into production-grade tests and reusable conversion code.
6. Update Function schema/reference artifacts for topology detailed/aggregated payloads.
7. Update Agent topology Functions:
   - `topology:network-connections`
   - `topology:streaming`
   - `topology:snmp`
8. Update the Cloud frontend compatibility architecture:
   - old schema adapter path
   - old frontend aggregation isolated in old path only
   - new detailed adapter path
   - new aggregated adapter path
9. Design and implement the separate Cloud topology aggregation microservice.
10. Validate losslessness, aggregation equivalence, frontend compatibility, mixed-version rollout, and the original 503/404/orphan-actor failure classes.

## Execution Log

### 2026-05-05

- Created the SOW.
- Loaded query and collector project skills.
- Confirmed the reported Function is served by `src/collectors/network-viewer.plugin/network-viewer.c`.
- Reproduced successful token-safe Cloud calls for all target aliases, saving raw responses under `.local/audits/network-connections-topology/`.
- Summarized response sizes and graph cardinality without storing sensitive values in the SOW.
- Traced the Agent-side 503 message and deferred response size cap.
- Confirmed the latest 404 alias was reachable and advertised `topology:network-connections` at probe time, so the browser 404 is not explained by stable Function absence.
- Traced the 404 error key through the sibling `cloud-charts-service` checkout and confirmed it is a Cloud node-instance routing/selection error.
- Measured payload-size waste in the largest captured response and confirmed the main waste is structural repetition: per-link `src`/`dst`, `labels`, `metrics`, repeated object keys, and actor `tables`.
- Built a local lossless detailed-payload prototype at `.local/audits/network-connections-topology/lossless-detailed-prototype.mjs`.
- Ran the prototype against the largest captured old detailed payload:
  - old detailed size: 134,180,370 bytes.
  - compact detailed size: 40,639,488 bytes.
  - compact ratio: 30.29% of old detailed.
  - actors: 48.
  - links: 77,797.
  - old canonical paths: 4,279,591.
  - reconstructed canonical paths: 4,279,591.
  - actor columns: 48.
  - actor table sections: 14.
  - maximum actor table columns: 4.
  - link columns: 69.
  - old detailed -> compact detailed -> old detailed canonical comparison: pass.
  - compact detailed -> old detailed -> compact detailed canonical comparison: pass.
- The first generic prototype pass exposed a real validation bug: internal empty-array/empty-object markers collided with real numeric values. The encoder was corrected to use private internal markers before encoding cells. This reinforces the requirement for explicit sentinel tests in production CI.
- A second table-aware prototype pass split actor tables into owner-indexed table sections. This reduced the compact detailed size from 76,910,648 bytes to 40,639,488 bytes while preserving canonical equality.

### 2026-05-06

- Recorded the expanded schema migration requirements:
  - Function schema update.
  - `topology:network-connections` `aggregated`/`detailed` request mode with `aggregated` default.
  - New schema for `topology:network-connections`, `topology:streaming`, and `topology:snmp`.
  - Cloud frontend support for both schemas, with old-schema frontend aggregation isolated for easy removal.
  - Separate Cloud topology aggregation microservice that consumes detailed payloads and returns requested aggregated views.
- Continuation pass: corrected the SOW-0012 relationship. SOW-0012 is also current and already owns active `topology:streaming` implementation work in this branch. SOW-0020 can continue with shared topology payload schema/spec work, but must not edit the streaming topology producer until SOW-0012 is closed or explicitly merged into this SOW.
- User update: the user stated SOW-0012 is done. For SOW-0020 sequencing, this unblocks shared schema work against the streaming topology producer. The SOW-0012 file still physically lives under `.agents/sow/current/` with `Status: in-progress` in this working tree, so closing or moving that SOW remains separate lifecycle work if needed.

### 2026-05-08

- Renumbered this SOW from `SOW-0013` to `SOW-0020` after rebasing onto `upstream/master`. Upstream already contains completed `SOW-0013` and `SOW-0014`; the user reserved `SOW-0015` and later reported that another worktree added more SOWs, so this work uses `SOW-0020`.
- Recorded the user's footprint decision: minimize detailed payload size first, while preserving per-socket evidence one by one for Cloud-side cross-node matching. Detailed graph links may be aggregated, but socket evidence must remain lossless in a compact detail plane.
- Recorded the user's payload-budget and matching decisions: paged/chunked socket evidence is phase 2, phase 1 must minimize footprint aggressively and fail explicitly rather than truncate if still over budget, and NAT/LB/proxy matching is out of scope for the current phase.

### 2026-05-09

- Rebased the worktree onto latest `upstream/master` at `79a23ebd9e`. The rebase completed with no conflicts, the autostash reapplied cleanly, and post-rebase checks `git diff --check` plus `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- Recorded the user's multi-level topology requirement: the same detailed evidence must support Cloud aggregation at node, container/application, Kubernetes label/workload, process-name, and PID scopes. Current network-viewer evidence supports node/process/PID and network namespace classification, but container and Kubernetes scopes require enrichment that the schema must be ready to carry.
- Recorded the user's actor drilldown table requirement: topology actor modals must continue to list exact dependencies, but new schema tables should be compact materialized views over shared evidence rather than duplicated object arrays under every actor.
- Recorded the user's custom actor table requirement: actor-owned tables such as streaming topology `streaming_path` must remain supported and must be distinguished from aggregatable relationship/evidence tables through explicit table role and aggregation-policy metadata.
- Recorded the user's direction semantics requirement: schema metadata must tell Cloud and UI whether a link type is directed, undirected, or hierarchical, and whether `direction` is flow/dependency meaning or observation metadata that can be ignored for aggregation identity.
- Recorded the user's refreshable link-telemetry overlay requirement: topology links need compact template references for bandwidth/packets/errors/state overlays so the UI can refresh traffic without recomputing topology, and Cloud can merge overlay refs when aggregating links.
- Recorded the user's schema-emulation requirement: before freezing the schema and migrating producers/UI, build a topology schema lab with modeled use cases, scale profiles, candidate encoding comparisons, and a prototype aggregator that produces payload-size and correctness evidence.
- Recorded the user's real-corpus requirement: the schema lab should be able to import read-only local captures of existing topology Function payloads from internal Netdata-owned Kubernetes infrastructure, with raw payloads kept under `.local/` and only sanitized summaries or generated fixtures committed.
- Recorded the user's Cloud corpus scope correction: only the `Netdata Cloud` space is in scope for topology payload capture; other visible spaces are irrelevant and must be ignored.
- Captured the scoped Cloud topology corpus using token-safe wrappers. Raw payloads are local-only under `.local/audits/topology-schema-lab/20260509T053114Z/`. Sanitized shape summary:
  - `topology:network-connections`: 12 successful payloads, 0 failures, 597,525,255 total raw bytes, 15,335,804 total gzip bytes, largest raw payload 151,865,348 bytes, largest graph 331 actors / 87,775 links / 87,894 sockets.
  - `topology:streaming`: 14 successful payloads, 0 failures, 1,732,039 total raw bytes, 101,389 total gzip bytes, largest raw payload 1,202,302 bytes, largest graph 120 actors / 121 links.
- Built and ran a local schema-lab prototype under `.local/audits/topology-schema-lab/scripts/` against the scoped corpus. The lab compared current object-shaped payloads, recursive string dictionaries, columnar lossless encoding, table-split columnar lossless encoding, graph/evidence-split lossless encoding, table-split graph/evidence lossless encoding, and a lossy aggregated-view estimate. Lossless candidates reconstruct the original payload and are checked with deep equality.
- Schema-lab result for `topology:network-connections` corpus:
  - Current baseline: 597,525,255 raw bytes / 15,335,804 gzip bytes.
  - Recursive string dictionary only: 319,794,282 raw bytes (53.52%) / 12,728,643 gzip bytes (83.00%).
  - Columnar lossless: 190,662,647 raw bytes (31.91%) / 10,004,844 gzip bytes (65.24%).
  - Table-split columnar lossless: 188,163,449 raw bytes (31.49%) / 9,997,988 gzip bytes (65.19%).
  - Graph/evidence split lossless: 171,733,199 raw bytes (28.74%) / 9,907,243 gzip bytes (64.60%).
  - Table-split graph/evidence split lossless: 168,277,837 raw bytes (28.16%) / 9,888,322 gzip bytes (64.48%).
  - Aggregated-view estimate without per-socket evidence: 25,258,107 raw bytes (4.23%) / 1,503,266 gzip bytes (9.80%).
  - Largest lossless table-split graph/evidence payload: 44,996,216 raw bytes for the largest 87,775-link capture.
  - Largest aggregated-view estimate: 6,284,950 raw bytes for the same largest capture class.
- Additional lossless scope measurement for `topology:network-connections` corpus:
  - Per-node best raw encoding, choosing the smallest verified-lossless candidate per node: 168,277,025 raw bytes / 9,888,011 gzip bytes summed across 12 node responses.
  - Largest per-node best raw response: 44,996,216 raw bytes / 2,748,857 gzip bytes for the largest 87,894-socket capture.
  - Per-node best gzip encoding, choosing the smallest verified-lossless gzip candidate per node: 170,026,315 raw bytes / 9,878,994 gzip bytes summed across 12 node responses.
  - Single combined current JSON-array response: 597,525,268 raw bytes / 15,342,691 gzip bytes.
  - Best measured single combined raw response: `table-split-graph-evidence-shared-dict-by-node-lossless`, 168,807,003 raw bytes / 9,882,897 gzip bytes.
  - Best measured single combined gzip response: `table-split-graph-evidence-global-table-lossless`, 176,133,862 raw bytes / 9,792,662 gzip bytes.
  - Raw combined lossless encoding is not better than independent per-node lossless encoding in this prototype. The best single combined raw response is 529,978 bytes larger than the per-node best raw sum. The best single combined gzip response saves only 86,332 gzip bytes compared with the per-node best gzip sum, at a 6,107,547 raw-byte cost.
- Follow-up measurement rejected the earlier "best lossless" interpretation:
  - The earlier 44,996,216-byte largest-node result was only the best among generic table transforms tested at that point; it was not proven best.
  - A per-column codec variant that still reconstructs the current old payload exactly reduced the largest 87,894-socket node to 13,672,282 raw bytes / 2,362,223 gzip bytes.
  - Across the 12-node `topology:network-connections` corpus, this exact-old-payload-lossless codec measured 52,398,087 raw bytes / 8,532,821 gzip bytes.
  - Largest-node section split for the exact-old-payload-lossless codec: string dictionary 3,693,814 bytes, evidence table 8,909,152 bytes, actor/table section 1,062,652 bytes, graph table 1,788 bytes.
  - The largest-node evidence table previously had 30,676,393 bytes of plain value arrays across 40 evidence columns; per-column codecs reduced those same evidence columns to 8,908,635 bytes. This proves the 44,996,216-byte result was not a schema lower bound.
  - Largest-node exact-old-payload-lossless record-size split: 13,672,282 total raw bytes over 87,894 sockets, about 155.6 bytes/socket. Of that, the evidence table is 8,909,152 bytes (101.4 bytes/socket), the global string dictionary is 3,693,814 bytes (42.0 bytes/socket), the actor/table section is 1,062,652 bytes (12.1 bytes/socket), and graph/envelope/metadata are negligible.
  - The largest string-dictionary consumers are legacy display/presentation strings: the shared `labels.display_name` / `metrics.display_name` value set has 72,526 unique strings and about 2,688,446 JSON string bytes before dictionary array overhead; remote `port_name` values add about 708,892 JSON string bytes; local/label port-name strings add about 157,331 JSON string bytes. These are not canonical topology identity requirements.
  - The largest evidence-table column costs are legacy display/port-name fields and current snapshot metrics: `labels.display_name` 972,404 bytes, `dst.attributes.port_name` 706,476 bytes, `labels.port_name` 600,121 bytes, `metrics.rtt_ms_max` 578,083 bytes, `dst.attributes.port` 476,926 bytes, `src.attributes.port` 459,193 bytes, `metrics.recv_rtt_ms_max` 453,373 bytes, plus many low-cardinality legacy label columns that each still cost about 2 bytes/socket as JSON index arrays.
  - A separate canonical socket-tuple experiment, not old-payload-lossless, preserved actor graph identity plus per-socket local bind IP/port, remote IP/port, namespace/address-space/family, direction/protocol/state, and ownership edges. It measured 2,015,773 raw bytes / 443,953 gzip bytes for the largest node, or 3,229,251 raw bytes / 897,960 gzip bytes when carrying current RTT/retransmission/socket-count snapshot metrics.
  - The same canonical socket-tuple experiment across all 12 nodes measured 7,864,151 raw bytes / 1,599,786 gzip bytes without current socket metrics, or 11,861,840 raw bytes / 3,039,411 gzip bytes with current socket metrics.
  - The canonical socket-tuple experiment is not yet a schema decision; it is evidence that the final schema should be purpose-built around socket evidence, not around exact reconstruction of the legacy object-shaped response.
- User correction on production payload vs test reconstruction:
  - Production payload must be optimized for the Cloud aggregator and UI, not for reconstructing the legacy payload.
  - Reconstruction details for old-payload parity are test harness code/fixtures only. They must not be shipped in production payloads.
  - The new production payload must preserve canonical information needed by aggregator/UI; it should not carry legacy presentation/reconstruction paths, old field names, display-string derivations, or redundant data solely to make old JSON byte/object reconstruction easier.
  - Losslessness for schema design should therefore be measured against a canonical information model, with separate test-side projection code proving that the old payload can be derived where compatibility/parity requires it.
- Production-only canonical socket payload rerun after separating reconstruction from payload:
  - Corpus scale: 323,077 socket evidence rows, 324,177 reported sockets, 1,839 graph links, and 259 ownership links across 12 captured `topology:network-connections` responses.
  - Current legacy corpus size: 597,525,255 raw bytes / 15,335,804 gzip bytes.
  - Production core payload as independent per-node responses: 7,280,783 raw bytes / 1,568,720 gzip bytes, 22.536 raw bytes per socket evidence row.
  - Production core payload as one combined response with shared string dictionary: 7,250,808 raw bytes / 1,556,744 gzip bytes, 22.443 raw bytes per socket evidence row.
  - Production core plus current RTT/retransmission/socket-count metrics as independent per-node responses: 11,278,208 raw bytes / 3,007,786 gzip bytes, 34.909 raw bytes per socket evidence row.
  - Production core plus current metrics as one combined response with shared string dictionary: 11,248,200 raw bytes / 2,992,573 gzip bytes, 34.816 raw bytes per socket evidence row.
  - Largest captured node under production core: 1,996,265 raw bytes / 443,069 gzip bytes for 87,761 socket evidence rows and 52 graph links.
  - Largest captured node under production core plus current metrics: 3,209,721 raw bytes / 896,764 gzip bytes.
  - Production core column costs across the corpus: graph index 689,510 bytes (2.134 bytes/socket row), local IP 651,101 (2.015), local port 1,359,664 (4.208), remote IP 692,081 (2.142), remote port 1,799,512 (5.570), namespace 646,151 (2.000), protocol family 646,152 (2.000), local address space 96, remote address space 646,305 (2.000). The remaining bytes are graph rows, ownership rows, per-node metadata, actors, JSON separators, and the string dictionary.
  - Current metrics add: `rtt_ms_max` 1,794,102 bytes (5.553 bytes/socket row), `recv_rtt_ms_max` 1,541,595 (4.772), retransmissions 661,560 (2.048), and `socket_count` 84 bytes because it is constant per node in this corpus.
- New documentation and implementation direction:
  - Document the new topology schema in detail, including the full JSON schema/contract, developer documentation, and an AI skill for creating topology producers.
  - Treat the superseded topology schema as removed from Agent/backend contracts and docs. The only temporary compatibility support should be isolated in Cloud frontend code until all supported Agents emit the new schema; that compatibility path is temporary and should be deleted later.
  - The new schema must be generic across topology types, not network sockets only. It must support network-connections, streaming, SNMP/L2, vSphere topology, and future topology producers.
  - Scope backend and frontend implementation changes after documenting the schema.
  - Build a Cloud topology aggregator as a separate Go service/component after the schema contract is documented and implementation scope is clear.
  - The vSphere topology work in the separate PR worktree must be updated in place, but no edits should be made there before telling the user because another agent is working in that directory.
- Drafted the production topology schema artifacts:
  - Added `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` as the JSON Schema for `netdata.topology.v1` payloads.
  - Added `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` documenting actors, links, evidence tables, actor/custom tables, direction semantics, aggregation policy, telemetry overlays, and producer examples for network-connections, streaming, SNMP/L2, and vSphere.
  - Added `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` scoping backend producer migration, frontend decoder/compatibility migration, and the Go aggregation component.
  - Added the public `docs/netdata-ai/skills/create-topology/` skill and `.agents/skills/create-topology` symlink so future assistants follow this topology contract.
  - Updated Function reference/developer docs and topology query skills to point to the production schema instead of documenting compatibility payload details.
- Schema-lab result for `topology:streaming` corpus:
  - Current baseline: 1,732,039 raw bytes / 101,389 gzip bytes.
  - Table-split columnar lossless is the best measured lossless candidate: 511,821 raw bytes (29.55%) / 81,484 gzip bytes (80.37%).
  - Graph/evidence split adds no value for streaming where each link already has one evidence row; table-splitting actor custom tables matters more.
- Prototype aggregator result for `topology:network-connections` corpus:
  - Extracted 323,077 socket evidence rows: 214,546 inbound and 108,531 outbound.
  - Exact reverse-tuple matching found 105,158 matched outbound rows, a 96.89% match ratio among outbound rows in this corpus.
  - After exact matching and unresolved-endpoint aggregation by IP, node-level projection produced 1,093 graph edges from 217,919 dependency evidence rows; process-name projection produced 1,027 graph edges; actor-identity projection produced 1,762 graph edges.
  - The largest node-level edge carried 43,196 evidence rows, proving that graph projection and evidence storage must be separate.
- User implementation clarification:
  - The Go topology aggregator should be implemented as a Cloud-style microservice similar to `${NETDATA_REPOS_DIR}/cloud-charts-service`, not primarily as an Agent-side helper package.
  - Before coding the aggregator, inspect the Cloud service layout, configuration, HTTP handler, test, and deployment conventions and mirror those patterns.
  - The new microservice name/repository should be `cloud-topology-service`.
  - Hard implementation requirement: `cloud-topology-service` must follow Cloud backend service coding, operational, configuration, testing, deployment, observability, and repository conventions exactly. Implementation must not begin until those conventions are extracted from existing Cloud service repositories with concrete file/line evidence and turned into a compliance checklist.
- Cloud backend service convention evidence collected so far:
  - `${NETDATA_REPOS_DIR}/cloud-service-builder/README.md:1` describes the skeleton as a microservice based on Netdata standards, but the generated template must be cross-checked against newer services before use.
  - `${NETDATA_REPOS_DIR}/cloud-service-builder/templates/cmd/app/main.go.tmpl:32`, `:41`, `:57`, `:129`, `:155`, and `:192` show the expected generated service-name constant, signal trap, config load, Prometheus/infra server, API server, and instance-name pattern.
  - `${NETDATA_REPOS_DIR}/cloud-service-builder/templates/internal/config/config.go.tmpl:54`, `:63`, and `:136` show the flag groups, log subset, and env parser prefix/set-separator pattern.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/cmd/service/main.go:62`, `:100`, `:125`, `:177`, `:208`, `:214`, `:245`, `:258`, `:272`, `:280`, `:361`, and `:380` show the current production service-name, logger, config, env prefix, Prometheus, infra server, ADC instrumentation, Pulsar, spaceroom gRPC client, repositories/services, API server, and route registration pattern.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/http/http.go:86`, `:107`, and `:144` show HTTP route registration, node/room auth wrapping, and the existing Cloud Function execution route pattern.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/http/auth_middleware.go:64` and `:134` show the Cloud spaceroom authorization pattern for node and room scoped endpoints.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/http/utils.go:21` and `:57` show the response and error-response helpers, including request id handling and `ckerrors` mapping.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/internal/model/errors.go:13` shows the service error taxonomy style.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/internal/service/agent_data.go:565` and `:1233` show direct single-node Function passthrough and concurrent multi-node Function passthrough through ADC after node-instance routing.
  - `${NETDATA_REPOS_DIR}/cloud-agent-data-ctrl-service/internal/config/config.go:21`, `:49`, and `:78` show an ADC-facing service configuration pattern with `ADC_` env prefix.
  - `${NETDATA_REPOS_DIR}/cloud-agent-data-ctrl-service/cmd/service/main.go:138`, `:218`, `:225`, `:235`, and `:242` show errgroup lifecycle, Prometheus, infra server, API server, and OpenTelemetry handler wiring.
  - `${NETDATA_REPOS_DIR}/cloud-agent-data-ctrl-service/transport/http.go:68`, `:95`, `:107`, `:132`, and `:196` show the internal agent API path map, handler construction, middleware, account validation, and internal proxy pattern.
  - `${NETDATA_REPOS_DIR}/cloud-custom-dashboard-service/cmd/customdashboardsvc/main.go:39`, `:71`, `:82`, `:122`, `:136`, `:157`, `:179`, `:185`, and `:192` show another current HTTP service pattern with signal handling, flags, env parser, Prometheus, spaceroom gRPC client, infra/API servers, and OpenTelemetry middleware.
  - `${NETDATA_REPOS_DIR}/cloud-custom-dashboard-service/internal/dashboard/transport_http.go:33` and `:54` show route construction and spaceroom authorization for room-scoped resources.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/Makefile:4`, `:18`, `:23`, `:28`, `:43`, and `:49` show expected tools, unit, integration, coverage, generate, and lint targets.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/Dockerfile:2`, `:4`, `:18`, `:29`, and `:42` show the Go base image, service env, build ldflags, Alpine production image, and entrypoint pattern.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/deployments/helm/values.yaml:2`, `:19`, `:30`, `:38`, `:44`, `:58`, `:64`, `:119`, and `:122` show the microservice anchor, Go memory limit env, probes, resource defaults, service env, security context, Prometheus annotations, service, and ingress-route shape.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/.github/workflows/main.yml:3`, `:40`, `:103`, `:128`, `:133`, `:137`, `:142`, and `:190` show PR/push triggers, permissions, Go setup, module verification, lint, integration test, coverage, and deployment workflow handoff.
  - `${NETDATA_REPOS_DIR}/cloud-charts-service/.github/CODEOWNERS:1` and `:8` show ownership separation for deployment files and Go service code.
- Cloud backend service compliance plan:
  - Treat existing Cloud service code as the source of truth. The service-builder template may bootstrap files, but every generated file must be reconciled against the current `cloud-charts-service`, `cloud-agent-data-ctrl-service`, `cloud-custom-dashboard-service`, and `cloud-spaceroom-service` patterns before implementation.
  - Produce a pre-code compliance matrix covering repository layout, Go module/dependency versions, Makefile targets, Dockerfile, GitHub workflows, CODEOWNERS, Helm values, environment variable prefix, configuration flags, logging, signal handling, Prometheus/OpenTelemetry, infra health/readiness, HTTP routing, CORS, authorization, error responses, ADC access, spaceroom access, tests, generated mocks, and deployment annotations.
  - Use `cloud-charts-service` as the primary behavioral reference for Function passthrough, node-instance routing, spaceroom authorization, ADC client instrumentation, request metadata, and Function-specific permissions.
  - Use `cloud-agent-data-ctrl-service` as the primary reference for ADC proxy lifecycle, agent request timeout handling, internal path validation, and service shutdown behavior.
  - Use `cloud-custom-dashboard-service` as the primary reference for compact room-scoped HTTP CRUD-style route construction and spaceroom auth middleware.
  - Use `cloud-service-builder` only for baseline repository shape after checking whether any generated defaults are stale compared to current services.
  - Implementation remains blocked until the compliance matrix identifies the exact file pattern to copy or adapt for each surface and records any gaps that need a user or Cloud-backend decision.
- Parallel microservice handoff:
  - Created `${NETDATA_REPOS_DIR}/cloud-topology-service/REQUIREMENTS.md` as a standalone handoff contract for a parallel worker to build the Cloud microservice while this SOW continues with schema/frontend/producer tasks.
  - The handoff requires the parallel worker to build the pre-code Cloud service compliance matrix before writing service behavior.
  - The handoff points back to this SOW and the topology schema, developer guide, implementation scope, and `create-topology` skill as the topology contract sources.
  - Reviewed the parallel worker's `${NETDATA_REPOS_DIR}/cloud-topology-service/QUESTIONS1.md` and answered it in `${NETDATA_REPOS_DIR}/cloud-topology-service/ANSWERS1.md`, setting phase-1 defaults for API ownership, route shape, new-schema-only behavior, source scope, partial/error semantics, caching, fixtures, optional metrics, validation location, deployment ownership handling, and compliance matrix location.
  - Kept three true external decisions open in the answer handoff: final service ownership/CODEOWNERS, environment-specific Helm values/deployment targets, and the exact Cloud-approved node-instance routing implementation strategy.
  - User clarified that these three open items are not user decisions; they should be handed to Cloud backend and DevOps once the service is otherwise ready for operational integration. Updated the microservice requirements and answers handoff so the parallel worker proceeds with compliance, schema/codec, aggregation, tests, HTTP scaffolding, and isolated fetcher interfaces without inventing ownership, environment values, or node-routing strategy.
  - User clarified that the phase-1 microservice MVP must support all topology kinds covered by the schema contract. It is not acceptable for the UI to use the service for only some topologies while bypassing it for others. Updated the microservice requirements, answers handoff, and topology implementation scope so `network-connections` remains the high-cardinality benchmark but not the MVP boundary.
  - Aligned the local topology implementation scope, topology schema spec, and `create-topology` skill with the all-topology MVP rule. Replaced stale Cloud aggregator open questions with resolved phase-1 defaults and Cloud backend/DevOps integration gates.
  - Added a current migration inventory to `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`, covering network-viewer, streaming, SNMP/L2, vSphere, and Cloud frontend surfaces with concrete file/line evidence and target migration behavior.
- Parallel Cloud frontend handoff:
  - Created sibling `cloud-frontend/TODO-topology-schema-new.md` as a standalone handoff contract for a parallel worker to implement frontend support for `netdata.topology.v1`.
  - The handoff records current frontend file/line evidence, requires old-schema support and the old link dedupper to be isolated under a removable legacy adapter, and scopes the new v1 path around compact table decoding, actor/link/evidence indexes, actor-detail vs relationship table separation, explicit direction semantics, overlay refs, worker decoding, and all-topology fixtures.
  - The handoff intentionally keeps Cloud service integration separate from the v1 decoder so per-node Function responses can migrate by schema version before the service route is ready.
  - Removed a later appended open-questions section from the handoff. The useful items were resolved into implementation defaults and concrete decoder clarifications so the frontend worker is not blocked on questions that can be answered from the schema, current frontend code, or the Cloud service handoff.
- Agent-side validation and fixture rail:
  - Extended `src/go/tools/functions-validation/validate` so the existing Function validation CLI now performs topology v1 semantic checks after JSON Schema validation. The checks validate compact table decoded lengths, inline dictionary indexes, global dictionary references, actor/link references, evidence references when a detail table declares `source_evidence`, and array column values.
  - Added schema-level topology v1 fixtures under `src/go/tools/functions-validation/fixtures/topology-v1/` for `network-connections`, `streaming`, `snmp-l2`, and `vsphere`.
  - The fixtures cover directed socket evidence, streaming actor-detail `stream_path`, SNMP/L2 unordered observation direction plus actor inventory and overlay refs, and vSphere hierarchy/dependency links plus actor detail.
  - Updated `src/go/tools/functions-validation/README.md` with a topology v1 validation command and documented the additional compact-table semantic checks.
- Producer helper rail:
  - Added `src/go/pkg/topology/v1` as the Go producer-side model and compact-table helper package for `netdata.topology.v1` payloads.
  - The helper provides response/data/type/table structs, compact-table constructors, row-count validation, parallel column/value validation, dictionary-index validation, and a shared decoded-payload semantic validator.
  - Refactored `src/go/tools/functions-validation/validate` to call the shared topology v1 semantic validator instead of keeping topology-specific validation private to the CLI.
  - Updated the topology developer guide, `create-topology` skill, and `project-writing-collectors` skill so future Go topology producers use the helper instead of hand-building compact-table JSON.
- SNMP/L2 producer migration investigation:
  - Investigated `topology:snmp` as the first Go producer candidate for migration to the helper.
  - Found a schema gap before producer migration: existing SNMP actor port details may include nested custom data such as `vlans` and `neighbors`, which cannot be represented by scalar-only compact table cells without losing information or flattening producer-specific structure.
  - Updated `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` to add a `json` column type for nested custom detail cells.
  - Updated the topology developer guide and `create-topology` skill to restrict `json` columns to actor/custom detail data and warn against using them for high-cardinality relationship evidence.
  - Extended the Go helper schema round-trip test with a nested `json` actor detail column so this requirement stays covered by tests.
- SNMP/L2 producer migration implementation:
  - Updated `src/go/plugin/go.d/collector/snmp_topology/func_topology_handler.go` so `topology:snmp` returns `netdata.topology.v1` data through a dedicated adapter before sending the Function response.
  - Added `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` to map the current SNMP topology snapshot into compact actor, link, L2 observation evidence, actor metadata, and actor-detail tables using `src/go/pkg/topology/v1`.
  - Preserved nested SNMP custom actor detail cells such as `neighbors` and `vlans` with `json` columns while keeping graph links and L2 evidence typed.
  - Updated SNMP topology Function tests to validate produced payloads against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` and the shared topology v1 semantic validator.
  - Updated the topology implementation scope and spec to record that SNMP now emits v1 through an adapter, while interface metric overlay-template/ref migration remains a refinement.
- Streaming producer migration implementation:
  - Replaced the superseded object-shaped `topology:streaming` payload in `src/web/api/functions/function-topology-streaming.c` with a direct `netdata.topology.v1` emitter.
  - Preserved streaming agents as compact actor rows, directed streaming/virtual/stale graph links, link evidence rows, and actor/modal tables for `stream_path`, retention, inbound, and outbound data.
  - Classified `stream_path` and retention as actor-detail tables and inbound/outbound rows as relationship summaries, so actor-owned custom data is not conflated with relationship evidence.
  - Kept stale stream-path hops as signed values and split streaming, virtual, and stale link evidence into separate evidence type ids so link-type metadata and evidence metadata agree.
  - Used a local C compact-table emitter for this low-cardinality producer. The shared C helper decision remains tied to the `topology:network-connections` migration, where high-cardinality socket evidence will determine the helper API.
  - Updated the topology implementation scope and topology schema spec to record that streaming now emits v1 directly.
- Network-viewer producer migration implementation:
  - Replaced the superseded object-shaped `topology:network-connections` payload in `src/collectors/network-viewer.plugin/network-viewer.c` with a direct `netdata.topology.v1` emitter.
  - Added `aggregated` / `mode:aggregated` and `detailed` / `mode:detailed` request handling, with aggregated as the default mode.
  - Preserved process grouping options and current socket filters while moving graph output to compact actor and graph-link tables.
  - Detailed mode emits socket relationship evidence as a shared compact table for exact tuple matching and drilldowns; aggregated mode omits the evidence table.
  - Removed superseded topology presentation emission and actor-nested socket tables from the Agent producer so the old schema is no longer present in this code path.
  - Added automatic string-column encoding for link/evidence columns: the writer uses inline dictionaries only when estimated raw JSON size is smaller than plain values.
  - Updated the topology implementation scope and topology schema spec to record that network-viewer now emits v1 directly.
- Orphan endpoint actor repair:
  - Investigated the user's reproduced floating endpoint actor with a non-zero
    socket count and no incident graph links.
  - Root cause: `local_sockets_cb_to_topology()` can create a remote endpoint
    actor while scanning sockets before all local IPs are known, but
    `topology_v1_collect_links()` resolves socket destinations later using the
    final local-IP set. If the same IP is learned as local later in the scan,
    link resolution treats it as self while the earlier remote endpoint actor
    remains in the actor table.
  - Fixed `topology_v1_collect_actors()` so endpoint actor emission rechecks
    `topology_ip_belongs_to_self()` against the final local-IP set. This makes
    endpoint actor emission and link destination resolution use the same self
    classification.

## Validation

Acceptance criteria evidence:

- 503 path: not deterministically reproduced through the token-safe path; evidence points to oversized intermittent/path-dependent Function failure. Supporting evidence: 110-134 MB successful bodies, 100 MiB parser cap, and exact 503 message source.
- 404 path: latest browser 404 was not reproduced through the token-safe path; function discovery and room inventory showed the node alias reachable and exposing `topology:network-connections` at probe time. This supports a Cloud node-instance routing/state race or stale browser/request context rather than a stable missing Function.
- Orphan endpoint path: reproduced by the user after the initial completed
  captures. Source analysis found an order-dependent actor/link classification
  mismatch in the producer; the targeted repair now aligns endpoint actor
  emission with final link destination classification.

Tests or equivalent validation:

- Schema-lab prototype transforms under `.local/audits/topology-schema-lab/scripts/` were run against the scoped Cloud corpus and produced the sanitized size/correctness measurements recorded in the execution log.
- `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- A minimal `netdata.topology.v1` sample payload was validated with Ajv 2020 against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- `git diff --check` passed.
- Stale-shape scan over the new topology docs/skills found no references to compatibility object fields such as `src_actor_id`, `dst_actor_id`, `data.actors[]`, `actors[]`, or `links[]`. The remaining `src_actor_id`/`dst_actor_id` references are in `src/plugins.d/FUNCTION_UI_SCHEMA.json`, which is the still-deployed Function UI schema and will be handled during implementation migration.
- Relative link target checks passed for the new query-topology references and the `.agents/skills/create-topology` symlink.
- Sensitive-path scan over the touched topology docs, skill, spec, and SOW found no per-user filesystem paths.
- Sensitive-path scan over the sibling Cloud frontend handoff found no per-user filesystem paths, personal names, or raw production identifiers.
- `jq empty` passed for all `src/go/tools/functions-validation/fixtures/topology-v1/*.json` fixtures.
- `go test ./tools/functions-validation/validate` passed from `src/go`.
- The validation CLI passed for all topology v1 fixtures with `--schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- `go test ./pkg/topology/v1 ./tools/functions-validation/validate` passed from `src/go`.
- `go test ./pkg/topology/... ./tools/functions-validation/validate` passed from `src/go`.
- The topology v1 helper JSON round-trip test marshaled a typed `netdata.topology.v1` response with a nested `json` actor detail column, validated it against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`, and then ran the shared semantic validator.
- `go test ./plugin/go.d/collector/snmp_topology ./pkg/topology/v1 ./tools/functions-validation/validate` passed from `src/go`.
- `go test ./pkg/topology/... ./plugin/go.d/collector/snmp_topology ./tools/functions-validation/validate` passed from `src/go`.
- SNMP topology Function tests now validate actual handler responses against both the production topology JSON Schema and the shared semantic validator.
- `TestSNMPTopologyToV1_PreservesActorCustomTables` validates that nested SNMP actor detail cells are preserved as `json` columns without weakening the typed graph/evidence path.
- Local CMake validation build configured with `cmake -S . -B .local/build-topology -DCMAKE_BUILD_TYPE=Debug -DENABLE_PLUGIN_XENSTAT=OFF -DENABLE_PLUGIN_DEBUGFS=OFF`. `ENABLE_PLUGIN_XENSTAT=OFF` avoids a missing local `xenstat` package, and `ENABLE_PLUGIN_DEBUGFS=OFF` avoids an uninitialized optional libsensors subtree in this local worktree.
- Initialized only the declared `src/aclk/aclk-schemas` submodule so the normal Agent target can generate protobuf sources for the local validation build.
- `cmake --build .local/build-topology --target netdata -j2` passed after the streaming migration, including compilation of `src/web/api/functions/function-topology-streaming.c` and linking the `netdata` executable.
- After tightening signed hops and evidence type ids, the incremental `cmake --build .local/build-topology --target netdata -j2` passed again, rebuilding `function-topology-streaming.c` and linking `netdata`.
- `git diff --check -- src/web/api/functions/function-topology-streaming.c` passed.
- Stale-shape scan over `src/web/api/functions/function-topology-streaming.c` found no superseded presentation helpers, `schema_version: "2.0"`, or old `src_actor_id` / `dst_actor_id` payload fields.
- `git diff --check -- src/collectors/network-viewer.plugin/network-viewer.c` passed after the network-viewer migration.
- `cmake --build .local/build-topology --target network-viewer.plugin -j2` passed after the final network-viewer migration, rebuilding and linking `network-viewer.plugin`.
- Stale-shape scan over `src/collectors/network-viewer.plugin/network-viewer.c` found no superseded topology presentation helpers, `schema_version: "2.0"`, or old `l7` topology layer values.
- Local `network-viewer.plugin debug` aggregated sample was captured under `.local/audits/topology-network-viewer-v1/` and validated against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` with the Function validator. Sanitized shape: 74 actors, 86 graph links, 0 socket-evidence rows, 20,132 raw bytes.
- Local `network-viewer.plugin debug` detailed sample was captured under `.local/audits/topology-network-viewer-v1/` and validated against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` with the Function validator. Sanitized shape: 91 actors, 102 graph links, 173 socket-evidence rows, 39,865 raw bytes.
- Local network-viewer sample codec check confirmed graph-link string columns use `dict` where beneficial and detailed socket-evidence string columns use a mix of `dict` and `values` based on estimated raw JSON size.
- Final validation pass after the network-viewer migration: `cmake --build .local/build-topology --target netdata -j2` passed.
- Final validation pass after the network-viewer migration: `go test ./pkg/topology/... ./plugin/go.d/collector/snmp_topology ./tools/functions-validation/validate` passed from `src/go`.
- Final validation pass after the network-viewer migration: the Function validator passed for all `src/go/tools/functions-validation/fixtures/topology-v1/*.json` fixtures and for the local aggregated/detailed network-viewer samples.
- Re-ran `go test ./pkg/topology/... ./plugin/go.d/collector/snmp_topology ./tools/functions-validation/validate` from `src/go`; it passed.
- Re-ran the Function validator over all `src/go/tools/functions-validation/fixtures/topology-v1/*.json` fixtures with `--schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`; all fixtures passed.
- Sensitive-data scan over the new topology fixtures and validator changes found no personal paths, personal names, tokens, cookies, API keys, UUID-shaped identifiers, or pushback wording.
- After the orphan endpoint actor repair:
  - C syntax validation passed for `src/collectors/network-viewer.plugin/network-viewer.c` using the compile command from `build/compile_commands.json` with `-fsyntax-only`.
  - `cmake --build .local/build-topology --target network-viewer.plugin -j2` passed.
  - A local `network-viewer.plugin debug` topology sample was captured under
    `.local/audits/topology-network-viewer-v1/` and validated with
    `go run ./tools/functions-validation/validate --schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
  - The local sample had zero endpoint actors without incident links after
    compact-table decoding.
  - The specific reproduced self-classified endpoint address was not emitted
    as an endpoint actor in the local sample after the repair.

Real-use evidence:

- Cloud API calls through the token-safe path returned HTTP 200 for all checked aliases during investigation. Browser evidence still shows real 503 and 404 failures, so the failures are intermittent/path-dependent.

Reviewer findings:

- No unresolved reviewer findings remain for this SOW. The final orphan-endpoint repair was narrow and was validated with source-path analysis, C syntax validation, a local plugin build, a runtime Function sample, schema validation, and a same-failure orphan scan. The broader topology presentation review was split into SOW-0021 and completed there with multi-reviewer findings and follow-up mapping.

Same-failure scan:

- Completed captured payloads were scanned for actors with no incident links and links referencing missing actors; no orphan actors or broken link references were found in those completed responses.

Sensitive data gate:

- Durable artifacts created so far use aliases and do not include browser cookies, raw tokens, raw node UUIDs, raw machine GUIDs, raw process details, or raw topology payloads.

Artifact maintenance gate:

- AGENTS.md: updated to register the public `create-topology` skill and symlink.
- Runtime project skills: `.agents/skills/project-writing-collectors/SKILL.md` updated to point topology producers at the new topology schema, guide, and implementation scope.
- Specs: `.agents/sow/specs/topology-function-schema.md` added as durable project memory for the production topology Function contract and updated with `json` detail-column, SNMP adapter migration notes, direct streaming v1 emission, signed stale-hop handling, and direct network-viewer v1 emission.
- End-user/operator docs: Function UI developer/reference docs updated to reference the topology schema; no external Learn docs were changed in this pass.
- End-user/operator skills: `docs/netdata-ai/skills/create-topology/`, `docs/netdata-ai/skills/query-netdata-cloud/`, and `docs/netdata-ai/skills/query-netdata-agents/` updated for topology schema/query guidance.
- SOW lifecycle: this SOW is marked `completed` and is moved to `.agents/sow/done/` with the implementation commit.

Specs update:

- `.agents/sow/specs/topology-function-schema.md` added and updated with `json` detail-column, SNMP adapter migration notes, direct streaming v1 migration notes, and direct network-viewer v1 migration notes.

Project skills update:

- `.agents/skills/project-writing-collectors/SKILL.md` updated.

End-user/operator docs update:

- `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` and `src/plugins.d/FUNCTION_UI_REFERENCE.md` updated.

End-user/operator skills update:

- `docs/netdata-ai/skills/create-topology/`, `docs/netdata-ai/skills/query-netdata-cloud/`, and `docs/netdata-ai/skills/query-netdata-agents/` updated.

Lessons:

- Actor emission and link emission must use the same final self/remote classification inputs. The network-viewer bug came from discovering remote endpoint actors during socket scanning before the final local-IP set was complete, while link emission later used the completed local-IP set.
- Compact topology validation must include graph invariants, not only schema validity. A payload can be valid JSON and still be unusable if it contains actors that no emitted link can reach.
- Large topology payload work needs both size tests and semantic round-trip tests. Size reduction alone is not enough if actor/link identity or drilldown evidence is weakened.

Follow-up mapping:

- SOW-0021 completed the topology presentation contract and backend producer presentation updates.
- SOW-0022 tracks actor modal and table composition.
- SOW-0023 tracks cross-payload actor identity, reconciliation, and matching strategies.
- SOW-0024 tracks vSphere topology migration to `netdata.topology.v1`.
- The Cloud topology service and Cloud frontend implementation handoffs are owned by their respective repositories and workers; this netdata commit does not include those repositories.

## Outcome

`topology:network-connections`, `topology:streaming`, and `topology:snmp` now have a compact `netdata.topology.v1` schema path, shared validation, producer guidance, and realistic fixtures. The network-connections orphan endpoint repair aligns remote actor emission with final link destination classification, so self-classified endpoints are no longer emitted as floating graph actors.

## Lessons Extracted

See `## Validation` lessons above. The durable schema, docs, and `create-topology` skill were updated so future topology producers preserve compactness, graph invariants, evidence, and presentation separation.

## Followup

- SOW-0021: topology presentation contract.
- SOW-0022: actor modal/table composition.
- SOW-0023: cross-payload actor matching and reconciliation.
- SOW-0024: vSphere topology v1 migration.

## Regression Log

None yet.
