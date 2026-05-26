# Spec - Topology Function Schema

## Status

Active for new topology Function work. Existing deployed topology producers are
to be migrated to this contract.

## Contract

Topology Functions return normal Function envelopes with `type: "topology"` and
`data.schema_version: "netdata.topology.v1"`.

The production schema is defined by:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`

The schema is generic across topology domains. It applies to network
connections, streaming, SNMP/L2, vSphere, and future topology producers.

## Planes

Topology payloads separate these planes:

- actors: observed entities such as nodes, processes, containers, ports,
  devices, streaming agents, or virtualization objects;
- graph links: renderable relationship groups between actors;
- evidence: canonical relationship facts behind graph links;
- detail tables: actor-owned or relationship-owned drilldown data;
- actor labels: actor-owned key/value rows for modal labels and filters, not a
  replacement for canonical identity or grouping columns;
- presentation: backend-selected UI-token composition for labels, colors,
  icons, legends, highlighting, link styles, scale keys, and graph port
  bullets;
- modal composition: actor/link modal recipes over existing actors, links,
  evidence, labels, and detail tables;
- correlation: producer-visible rules, loose-side resolution, replacement,
  enrichment, visible correlation points, and claims used by an aggregator to
  correlate independently produced topology maps without exposing aggregator
  internal states;
- overlay refs: compact references for refreshable metrics or Function-backed
  snapshots.

Graph links are projections. Evidence rows are the facts used by Cloud
aggregation, matching, and detailed drilldowns.

## Mode Requests

Mode requests use `__topology_mode` when a producer has a real detailed vs
aggregated output difference. Valid values are `detailed` and `aggregated`.
Mode-invariant topologies should not expose a selector only to return identical
payloads. Mode-capable producers declare `data.view.supported_modes`; absent
or single-value `supported_modes` means the topology is mode-invariant for UI
control purposes. The Cloud topology aggregator consumes detailed payloads for
mode-capable producers before returning an aggregated view, so producers must
preserve correlation-grade evidence in detailed mode.

## Compact Tables

Large sections use compact columnar tables with:

- `rows`
- `columns`
- `values`

Supported column codecs are:

- `const`
- `values`
- `dict`

Every decoded column must produce exactly `rows` values. Producers should use
shared helpers for table building, dictionary encoding, validation, and
deterministic sorting rather than hand-rolling encoders in each plugin.

Column type `json` is allowed only for actor-owned or custom detail cells that
must preserve nested producer-owned values. It is not the default for
relationship evidence, because high-cardinality evidence needs typed scalar,
reference, array, or dictionary-backed columns for compactness and aggregation
semantics.

## Identity And Aggregation

Actor types declare:

- source-local `identity`;
- cross-payload `merge_identity`;
- optional `parent_identity`;
- supported `aggregation_scopes`.
- optional `search` policy over actor table columns and actor label keys.
- optional `presentation` with UI-owned tokens, safe label policy, size policy,
  actor repulsion policy, and graph port-bullet policy.

Link types declare:

- `orientation`: `directed`, `undirected`, `hierarchical`, or
  `observed_bidirectional`;
- `direction_role`: `none`, `flow`, `dependency`, `ownership`, or
  `observation`;
- optional `semantic_role`: `normal`, `discovery`, `ownership`, `traffic`,
  `correlation`, or `control`;
- aggregation policy for direction, evidence, metrics, tables, and overlays.
- optional `presentation` with UI-owned color, line style, width, curve, arrow,
  tokenized layout strength/distance, and one variable visual channel.

Port types are optional and declare graph port-bullet presentation only.
Actor/link modal composition is declared separately under actor/link type
presentation and table type presentation. Modal recipes are selectors and
projections over existing facts; they are not duplicate row stores.

Cloud aggregation may only canonicalize endpoint order when link type policy
explicitly allows unordered aggregation. Direction-significant links must
preserve direction.

## Evidence And Tables

Relationship evidence is not actor custom data.

Evidence tables carry matchable relationship facts such as socket tuples,
streaming hops, LLDP/CDP observations, or vSphere inventory edges. Actor-detail
tables carry actor-owned data such as streaming path inventory or local status
tables. Table metadata must state role and aggregation policy.

Production payloads must not duplicate the same evidence under every actor only
to populate drilldown modals. The UI and aggregator derive drilldown views from
shared evidence and typed table references.

Actor labels use a compact actor-owned table, normally
`tables.actor.actor_labels`, with rows shaped as:

```text
actor, key, value, source?, kind?, value_index?
```

`key`, `value`, `source`, and `kind` are logical strings and may be encoded as
`string` or `string_ref`. Producers should prefer `string_ref` when they already
maintain a local dictionary; aggregators and UI adapters must normalize both
encodings as equivalent label strings.

Host/node actors should expose the complete host label set when available.
Non-node actors should expose useful producer-known labels and metadata, such
as process command line, user, group, namespace, interface role, or
virtualization object properties. Repeated values are repeated rows ordered by
`value_index`, not JSON arrays. Facts needed for identity, correlation,
grouping, sorting, filtering, or aggregation must remain typed canonical
columns too.

`actor_labels` inherits the topology Function sensitive-data classification.
Consumers must preserve the same access-control assumptions as the source
Function because labels may include command lines, users, host labels, system
contact/location fields, or other operator-controlled metadata.

Actor modal identification is producer-selected through
`presentation.modal.labels.identification.fields[]`. Each field references an
existing `actor_labels.key`, provides the label to render near the actor title,
and may limit the number of displayed values with `max_values`. The full Labels
tab remains complete; identification is a curated header projection, not a
second table.

## Presentation

Presentation belongs in the production `netdata.topology.v1` payload, not in
Function `info`, except for old-schema compatibility during rollout.

Function `info` responses are metadata responses. They may omit `data` and
advertise only parameters, help text, and timing. The topology JSON schema
applies to full topology responses, not metadata-only `info` responses.

Type definitions carry type-local presentation:

- `types.actor_types.<id>.presentation`
- `types.link_types.<id>.presentation`
- `types.port_types.<id>.presentation`

Graph-level `data.presentation` carries legend order, actor-click highlight
behavior, port tooltip field labels, and scale-key definitions.

Actor and link type presentation may also carry modal composition:

- `presentation.modal.labels` describes the actor label table, usually
  `actor_labels`;
- `presentation.modal.mini_topology` describes a depth-1 modal graph preview
  built from incident links and opposite actors;
- `presentation.modal.sections[]` describes table sections over existing
  actors, links, evidence, actor detail tables, or relationship tables.

Table types may carry `presentation` defaults for table label, order, default
visibility, and column display metadata. Table type presentation does not
replace actor/link modal sections; it gives reusable defaults for existing
table rows.

Modal sections require non-empty `id`, non-empty `label`, a source, and at
least one column. `empty_label` is section-only empty-state text. Modal columns
require non-empty `id`, non-empty `label`, and a projection. `badge_map`,
`align`, and `sortable` are presentation hints over projected values and must
not introduce new facts.

`label_lookup` uses `label_key` and optionally an `actor_column`; omitted
`actor_column` means the selected modal actor. `json_path` always requires both
the JSON `column` and the scalar `path` to extract.

Presentation uses closed UI-owned tokens. Producers must not emit raw SVG, raw
CSS, coordinates, component names, raw force-layout physics, or viewport state.

Closed token values are part of the schema contract:

- color slots: `primary`, `secondary`, `accent`, `self`, `neutral`, `muted`,
  `dim`, `derived`, `info`, `structural`, `warning`, `success`, `danger`,
  `blue`, `green`, `orange`, `purple`, `cyan`, `yellow`, `teal`, `gray`;
- opacity tokens: `normal`, `muted`, `faded`;
- width tokens: `thin`, `normal`, `thick`, `emphasis`;
- link layout strength tokens: `weakest`, `weaker`, `normal`, `stronger`,
  `strongest`;
- link layout distance tokens: `closest`, `closer`, `normal`, `farther`,
  `farthest`;
- actor layout repulsion tokens: `weakest`, `weaker`, `normal`, `stronger`,
  `strongest`;
- actor size scale tokens: `compact`, `normal`, `emphasized`;
- link semantic roles: `normal`, `discovery`, `ownership`, `traffic`,
  `correlation`, `control`;
- icons: `router`, `switch`, `firewall`, `access_point`, `server`, `storage`,
  `load_balancer`, `printer`, `phone`, `ups`, `camera`, `process`, `agent`,
  `netdata-agent`, `parent`, `remote-endpoint`, `local-endpoint`, `segment`,
  `self`, `ip`, `cloud`, `container`, `vm`, `database`, `service`,
  `datacenter`, `cluster`, `host`, `network`, `datastore`,
  `datastore_cluster`, `resource_pool`, `device`, `endpoint`, `correlation`,
  `interface`, `group`, `unknown`.

The UI owns token rendering and must treat producer labels as plain text. If a
new producer emits a schema-valid token that an older UI does not know, the UI
must use a safe fallback and record diagnostics.

Actor `label_policy` is the only approved way to choose display labels from
actor rows. Canonical identity is not display text. The UI must reject array
values by default so aggregated identities do not become long actor names.
Label-policy columns must be safe scalar actor-table columns. Producers must
not reference secrets, tokens, customer-identifying fields, or unbounded arrays
from `label_policy.columns`.

Actor `search` is the only approved way to choose graph search content for v1.
`search.columns[]` references scalar actor-table columns. `search.label_keys[]`
references values in the actor label table, normally `actor_labels.key`. Set
`search.enabled: false` for helper actors that should not be searchable. The UI
must not traverse producer-specific `details`, `match`, `attributes`, or label
paths when rendering v1.

Link types may define one variable visual channel using `variable.channel`,
`variable.scale_key`, and `variable.value_column`. Producers emit raw domain
values, such as socket counts or traffic bytes. Cloud or the UI scales values
per `scale_key` across the visible graph. `variable.min` and `variable.max`
are visual tokens for the chosen channel: width variables use width tokens,
opacity variables use opacity tokens.

Link types may also define tokenized force-layout hints using
`presentation.layout.strength` and `presentation.layout.distance`. These are
relative UI-owned tokens, not numeric physics. Current producer tuning keeps
`strength` at `normal` and varies only `distance` where the topology needs
semantic separation. Do not reintroduce non-normal `strength` tokens for graph
polish unless a later product decision explicitly re-enables force-strength
tuning.

Actor types may define tokenized force-layout hints using
`presentation.layout.repulsion`. Repulsion is separate from link strength:
repulsion pushes actors apart, while link strength pulls endpoints together.
Producers must not emit raw charge values. Actor size may define a type-level
`size.scale` token for deliberate fixed emphasis, such as current/self actors;
the UI must not infer this from actor type names or labels.

`link_types.<id>.semantic_role` is behavior metadata, not visual styling. It
drives UI behavior such as discovery-link filtering, ownership/coherence
treatment, traffic emphasis, and correlation treatment without hardcoded
protocol or type-name checks. Link appearance still comes from
`presentation`.

When link `presentation.arrow` is `auto` or omitted, the UI derives arrows from
`orientation` and `direction_role`:

- `undirected` -> no arrow;
- `observed_bidirectional` -> no arrow;
- `direction_role: none` -> no arrow;
- `direction_role: observation` -> no arrow;
- `directed` with `flow` or `dependency` -> forward from `src_actor` to
  `dst_actor`;
- `hierarchical` with `ownership` -> forward from `src_actor` to `dst_actor`;
- all other combinations -> no arrow and a diagnostic if the combination is
  schema-valid but semantically unusual.

`observed_bidirectional` means observation completeness, not "draw both
arrows". Producers that need reverse or both arrows must set
`presentation.arrow` explicitly.

`direction_role` is required by the v1 schema. Missing `direction_role` is
schema-invalid and must not produce an inferred arrow from `orientation` alone.
The UI should render `auto` as no arrow and emit the normal missing-field
diagnostic for that invalid input.

For schema-valid values, the `auto` semantic diagnostic boundary is:

- no diagnostic for `directed+flow`, `directed+dependency`,
  `hierarchical+ownership`, `undirected+none`, `undirected+observation`,
  `observed_bidirectional+none`, or `observed_bidirectional+observation`;
- diagnostic for `directed+none`, `directed+observation`,
  `directed+ownership`, `hierarchical+none`, `hierarchical+flow`,
  `hierarchical+dependency`, `hierarchical+observation`, `undirected+flow`,
  `undirected+dependency`, `undirected+ownership`,
  `observed_bidirectional+flow`, `observed_bidirectional+dependency`, or
  `observed_bidirectional+ownership`.

Initial UI-owned mappings are:

- `size.scale`: `compact=0.85`, `normal=1.0`, `emphasized=1.18`;
- `layout.repulsion`: `weakest=-200`, `weaker=-300`, `normal=-450`,
  `stronger=-700`, `strongest=-1000`.

These numeric values are not schema and may be tuned after visual QA.
`size.scale` composes with `size.mode`; it does not override data-driven
sizing. Missing optional fields use neutral defaults (`scale: normal`,
`repulsion: normal`) and must not trigger v1 UI fallback heuristics.

Actor port bullets require explicit `ports.sources[]` when
`show_bullets: true`. The source may be `links`, `evidence`, or an
`actor_table`. Source column names are table-local and must remain unchanged
during Cloud aggregation. Type ids inside row values and `default_type` values
are rewritten only when they refer to type registries. `ports.sources[].evidence`
is an evidence type id. `name_column` must reference a scalar display column,
not a raw actor/link/evidence reference, array, or JSON cell.
`ports.sources[].value_column` is optional and must reference a numeric source
column. When present, the UI sums values for matching bullet keys and uses the
sum for bullet multiplicity, overflow, and sizing. Aggregated producers should
use this instead of sending repeated rows only to drive presentation.

Modal/table composition uses closed source, projection, cell, and visibility
tokens. Supported source kinds are `actors`, `links`, `evidence`,
`actor_table`, and `relationship_table`. Supported projections include direct
column values, actor-ref labels, opposite actor labels, formatted endpoints,
selected-side endpoints, label-table lookups, coalesced columns, constants,
and explicitly declared scalar JSON paths. Supported cell types include text,
number, badge, actor link, timestamp, duration, endpoint, array count, and
debug JSON. Raw JSON belongs behind `debug` visibility or an explicit scalar
projection; it must not be the default polished actor modal rendering.
Selected-side endpoint projections must be self-contained: the projection
names source and destination actor-ref columns, plus at least one source-side
endpoint column and one destination-side endpoint column.

`selection.actor_click.mode: highlight_path` requires `path_table`,
`path_actor_column`, and `path_order_column`. `path_actor_column` identifies
path members. When one table contains different paths for different clicked
actors, `path_owner_column` identifies the actor that owns each path row.
`highlight_connections` requires no path table.

Presentation conflict policy:

- producer-local type ids, port ids, scale keys, evidence ids, table type ids,
  and overlay template ids are namespaced before aggregation;
- identical definitions are deduplicated after canonicalization;
- conflicting local definitions are preserved as distinct canonical ids rather
  than hard-failing aggregation;
- `label_policy` belongs to the actor type presentation and follows the same
  namespace/deduplicate rule;
- `profile_version` is diagnostic. It may help choose a preferred display
  profile later, but it is not a comparable semantic-version contract and must
  not be used to drop facts or rows.

## Correlation Contract

Correlation is producer-visible graph semantics, not aggregator state.
Producers must not encode correlation as hidden flags on real actors, and must
not expose aggregator internal states such as absorbed, candidate, equivalence
class, or rewrite plan. The final aggregated output is always a normal topology
payload.

Correlation can resolve several shapes:

- loose relationship sides, where one side of a detailed row has endpoint facts
  but no known actor;
- visible correlation actors, where the input graph intentionally materializes
  unresolved peers;
- weaker placeholder actors that should be replaced by stronger managed actors;
- equivalent actors that should be merged and enriched with facts from multiple
  payloads.

`data.correlation.rules` defines how independent payloads of the same topology
kind can be correlated. Each rule defines:

- optional `class`: `resolve_loose_side`, `replace_actor`, or
  `merge_enrich_actor`;
- `action`: `absorb` for exact matches that remove visible correlation actors
  or consume loose-side placeholders and rewrite incident correlation
  relationships, or `link` for partial/broader matches that keep the visible
  correlation/materialized actor and add a weak correlation link;
- `priority`: lower numbers run first;
- `key_space`: namespace for exact string-key matching;
- `key`: a declarative template built from point/claim table columns and
  literals;
- `point_actor_types`: actor types that are visible correlation points when the
  input graph materializes points;
- optional `claim_actor_types`: actor types that may satisfy the point;
- optional `correlation_link_types`: link types that connect real actors to
  correlation actors and may be consumed/replaced by the rule;
- `output_link_type`: link type emitted for rewritten absorb links or visible
  partial correlation links.

`data.correlation.points` is a compact table of visible correlation actors and keys.
`data.correlation.claims` is a compact table of real actors and keys they can
satisfy. Both tables require `actor`, `rule`, and the key columns referenced by
their rules.

The aggregator is intentionally agnostic. It builds normalized keys from
declared columns and literals, applies rule priority, and handles ambiguity
conservatively. It must not need new code to understand every future IP, port,
MAC, chassis id, object id, label, or topology-domain key.

No match keeps the visible correlation actor or loose-side materialization
visible. Ambiguous matches remain unresolved and produce diagnostics. NAT or
other alias evidence can be modeled by adding extra point/claim rows for the
same actor and rule; aliases add facts without mutating the original
observation.

Correlation links must be semantic link types even for single-node payloads.
The legend must include visible correlation actors and links when they are
visible, so users can distinguish unresolved, partial, inferred, and resolved
graph relationships.

## Telemetry Overlays

Refreshable traffic, state, error, packet, or utilization data is represented by
overlay templates and per-actor/per-link refs.

Templates define the query mechanism once. Refs provide only template ids and
parameters. Aggregated links merge refs according to the template merge policy.

## Compatibility

Production payloads carry canonical topology facts, not compatibility
reconstruction instructions. Projection code for parity with deployed
compatibility consumers is test or rollout code only.

Agent/backend contracts and docs should point new work to
`netdata.topology.v1`. Temporary compatibility handling belongs in isolated
Cloud frontend adapters during rollout.

## Validation

Topology producer changes must include:

- JSON Schema validation against `FUNCTION_TOPOLOGY_SCHEMA.json`;
- semantic validation for table column lengths and reference bounds;
- fixture or corpus tests for payload size and evidence preservation;
- tests proving direction and aggregation policy are honored;
- checks that evidence is not silently truncated.

Cloud topology aggregation service readiness also requires service-level
fixtures for every topology kind covered by this contract. `network-connections`
is the high-cardinality benchmark, but the service is not considered ready if
the UI can use it for only some topology kinds while bypassing it for others.

## Migration Notes

`topology:network-connections` now emits `netdata.topology.v1` directly from
the C network-viewer Function. Aggregated mode is the default and emits compact
actor, graph-link, and actor-owned `socket_ports` tables. Detailed mode adds a
shared socket relationship-evidence table for exact tuple matching and
drilldowns. Process actor size uses the actor row `socket_count` metric, while
process port bullets read the `socket_ports.socket_count` value column so an
aggregated port row can represent several sockets. The producer no longer emits
the superseded old-schema presentation object or duplicated actor-nested socket
modal tables. It now emits compact graph presentation metadata in type
definitions plus `data.presentation`, and repeated string columns use
dictionary encoding when it reduces raw payload size.

Network-connections distinguishes unresolved endpoint links from aggregator
correlation output. `endpoint_socket` connects a process to a visible unresolved
endpoint actor and must not use the farthest layout distance because that makes
single-node maps zoom out unnecessarily. `correlated_socket` is the output link
type after exact endpoint absorption by an aggregator and may use farthest
distance to keep independent topology clusters from blending.

Network-connections modal composition is producer-declared. Self/node actors use
a `Processes` section over `ownership` graph links filtered by link type.
Network-connections socket link types use `direction_role: "dependency"` and
are client-to-server: `src_actor` is the client/dependant and `dst_actor` is the
server/dependency target. Non-node actors therefore use two primary sections in
both aggregated and detailed mode:
`Dependencies`, filtered to rows where the selected actor is `src_actor`, and
`Dependants`, filtered to rows where the selected actor is `dst_actor`.
Aggregated mode reads these sections from `tables.relationship.connections`;
detailed mode reads them from `evidence.socket`. `socket_ports` is an actor
inventory for process port bullets only; it is not a standalone modal tab for
network-connections.

`topology:snmp` now emits `netdata.topology.v1` from the Function handler
through an adapter over the existing SNMP topology engine output. The adapter
preserves actors, links, L2 observation evidence, actor metadata, and actor
custom detail tables. Remaining SNMP refinement is to promote interface metric
lookup fragments into first-class overlay templates/refs.

SNMP modal composition must be port-centric for managed device actors. A managed
device modal uses actor-label identification for important device facts, a
primary `Ports` section over `actor_ports`, and a `Port Neighbors` section over
`actor_port_links`. Generic graph-link `Links` sections are reserved for
endpoint, segment, or custom actors that do not own port inventory.

SNMP `actor_ports` exposes real port identity and status as typed columns:
SNMP `if_index` as the visible numeric port ID when known, source `port_id`,
display `name`, `if_name`, `if_descr`, `if_alias`, MAC, speed, status, mode,
role, VLAN, FDB, link, and neighbor counts. It must never fabricate numeric
port IDs; row order and generated sequences must not be used. `if_index` must
come from the device/SNMP facts and must align with `actor_port_links.if_index`.

SNMP `actor_ports` may also carry compact expanded-row neighbor columns such as
nullable `neighbor_actor` and `neighbor_port_name`, derived from graph-link
endpoint facts. These columns make the port row clickable without duplicating
raw LLDP/CDP/FDB/ARP/STP evidence.

SNMP `actor_port_links` is a compact actor-owned modal index over existing graph
links and evidence. It has one row per incident actor side and carries the local
`if_index`/port name, remote actor, remote port facts, protocol, link type,
state, evidence count, confidence, inference, attachment mode, and timestamps.
It exists so device modals can align neighbor rows with the same port identity
shown in `actor_ports`; it is not a second copy of raw evidence.

SNMP polished UI must not depend on raw `actor_metadata` and endpoint JSON.
Important scalar/count summary values live in typed actor or actor-detail
columns and are also available through `actor_labels`. Nested neighbors, VLANs,
unknown custom port attributes, and endpoint objects stay in expanded or debug
sections unless a structured child table is defined. Link endpoint port labels
must come only from real port fields such as `port_name`, `if_name`,
`if_descr`, or source `port_id`; actor labels such as `display_name` or
`sys_name` must not be used as port-name fallbacks.

`topology:streaming` now emits `netdata.topology.v1` directly from the C
Function. It models streaming agents as compact actor rows, streaming/virtual/
stale relationships as directed graph links with matching evidence types, and
keeps `stream_path`, retention, inbound, and outbound modal data as typed
actor-detail or relationship-summary tables. Streaming also emits graph
presentation metadata for highlight-path behavior, legend, link styles, and
port bullets. Streaming hops remain signed so stale path values are not
corrupted.

Streaming parent actor size is data-driven by the actor row
`retained_node_count` metric, not by graph degree or direct child count. This
count represents nodes for which the parent has retained DB data, including
self, virtual nodes, stale nodes, and transit descendants when they have
retention state. The parent actor type must declare `presentation.size:
{"mode":"metric","metric_column":"retained_node_count"}`. Parent port bullets
represent child or vnode streams attached to the parent side of streaming graph
links, so the parent `ports.sources[]` entry over `links` must use
`actor_column: "dst_actor"` with a scalar child/node display column such as
`port_name`.

Streaming modal composition emits `actor_labels`, complete host labels where
available, host/system metadata labels needed by old summaries, and typed
OS/architecture/CPU columns. The actor modal header must select important
identity/status labels from `actor_labels`, including hostname, node type,
stream status, ingest status, health status, retained-node count where
applicable, direct-child count where applicable, OS/platform labels, and Agent
version. Existing `stream_path`, `retention`, and `inbound` tables have the
right actor-ref shape for recipe-based table rendering. The `outbound` table
must use the parent-owned shape described below; a table that only records the
selected actor's own upstream destination is insufficient for parent operator
workflows.

Streaming actor modal identification is role-specific. Host-like actors
(`parent`, `child`, and `stale`) should expose operational status plus
OS/hardware/platform labels such as OS, OS version, kernel, architecture, CPU
model, cores, RAM, virtualization, container, cloud provider/type/region, and
Agent version. Parents additionally expose `retained_node_count` and
`child_count` so the visual size and direct attachments are both explainable.
Vnode actors should expose inventory/device labels such as vnode type, vendor,
model, address, location, sys object id, LLDP system name, and status. Long
stable identifiers such as `machine_guid` and `node_id` remain in the full
Labels tab by default unless a future product decision explicitly promotes them.

Streaming actor modals must keep those tables as the single source of truth and
must not duplicate rows only for modal display. The default visible sections are:

- `Stream path`: rows from `stream_path` filtered by `actor`, ordered by
  `path_index`. The table shows the selected actor's own path only; virtual
  nodes and children have their own actors and therefore their own path rows.
  `since` and `first_time` must be populated from the best canonical source
  available for every path row. Synthetic path rows added only for rendering or
  highlighting must still carry timestamps when the producer can derive them
  from the adjacent path edge, the selected actor's ingest status, or database
  first-time status. They may be null only when the Agent genuinely does not
  know the value.
- `Retained nodes`: rows from the same `retention` table filtered by
  `observer_actor`; this view answers which nodes' data the selected actor
  maintains. The table must include self, virtual nodes, direct children,
  transit descendants, and stale/archived hosts when those hosts are present in
  the Agent root index and have retention state. It must show retained node,
  node type, retention status, from/to timestamps, duration, metrics, instances,
  and contexts. `db_from` and `db_to` may be null only when the database status
  genuinely has no time range.
- `Received nodes`: rows from `inbound` filtered by `parent_actor`; this view
  represents children, virtual nodes, stale nodes, and descendants received or
  transiting through the selected parent. `source_actor` is the immediate actor
  from which the selected parent receives the row. For direct local receipt,
  `source_actor` should be the child/vnode actor itself; it should be null only
  when the immediate source is genuinely unknown.
- `Outbound streams`: rows from `outbound` filtered by the sending parent
  actor, not by the streamed node actor. This view answers which node payloads
  the selected parent currently streams, and where it streams each one. Each row
  must include the streamed node actor, destination actor when known, status,
  age, hops, TLS, compression, and useful stream/replication/count metrics when
  available. In clustered-parent setups, the selected parent must list self,
  virtual nodes, direct children, and transit descendants that are sent to each
  upstream destination.

The old `Retention for node` default section is not part of the current
streaming modal contract. The canonical `retention` table still keeps both
`actor` and `observer_actor` so Cloud aggregation can preserve multiple
retaining parents for the same node. If a future modal needs a selected-node
"who retains me" view, it must be explicitly named `Retained by` and must not
replace the parent-owned `Retained nodes` view.
