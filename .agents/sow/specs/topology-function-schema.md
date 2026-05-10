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
- presentation: backend-selected UI-token composition for labels, colors,
  icons, legends, highlighting, link styles, scale keys, and graph port
  bullets;
- overlay refs: compact references for refreshable metrics or Function-backed
  snapshots.

Graph links are projections. Evidence rows are the facts used by Cloud
aggregation, matching, and detailed drilldowns.

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
- optional `presentation` with UI-owned tokens, safe label policy, size policy,
  and graph port-bullet policy.

Link types declare:

- `orientation`: `directed`, `undirected`, `hierarchical`, or
  `observed_bidirectional`;
- `direction_role`: `none`, `flow`, `dependency`, `ownership`, or
  `observation`;
- aggregation policy for direction, evidence, metrics, tables, and overlays.
- optional `presentation` with UI-owned color, line style, width, curve, arrow,
  and one variable visual channel.

Port types are optional and declare graph port-bullet presentation only. Full
modal/table composition is separate from this graph presentation contract.

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

## Presentation

Presentation belongs in the production `netdata.topology.v1` payload, not in
Function `info`, except for old-schema compatibility during rollout.

Type definitions carry type-local presentation:

- `types.actor_types.<id>.presentation`
- `types.link_types.<id>.presentation`
- `types.port_types.<id>.presentation`

Graph-level `data.presentation` carries legend order, actor-click highlight
behavior, port tooltip field labels, and scale-key definitions.

Presentation uses closed UI-owned tokens. Producers must not emit raw SVG, raw
CSS, coordinates, component names, force-layout physics, or viewport state.

Closed token values are part of the schema contract:

- color slots: `primary`, `secondary`, `accent`, `self`, `neutral`, `muted`,
  `dim`, `derived`, `info`, `structural`, `warning`, `success`, `danger`,
  `blue`, `green`, `orange`, `purple`, `cyan`, `yellow`, `teal`, `gray`;
- opacity tokens: `normal`, `muted`, `faded`;
- width tokens: `thin`, `normal`, `thick`, `emphasis`;
- icons: `router`, `switch`, `firewall`, `access_point`, `server`, `storage`,
  `load_balancer`, `printer`, `phone`, `ups`, `camera`, `process`, `agent`,
  `netdata-agent`, `parent`, `remote-endpoint`, `local-endpoint`, `segment`,
  `self`, `ip`, `cloud`, `container`, `vm`, `database`, `service`,
  `datacenter`, `cluster`, `host`, `network`, `datastore`,
  `datastore_cluster`, `resource_pool`.

The UI owns token rendering and must treat producer labels as plain text. If a
new producer emits a schema-valid token that an older UI does not know, the UI
must use a safe fallback and record diagnostics.

Actor `label_policy` is the only approved way to choose display labels from
actor rows. Canonical identity is not display text. The UI must reject array
values by default so aggregated identities do not become long actor names.
Label-policy columns must be safe scalar actor-table columns. Producers must
not reference secrets, tokens, customer-identifying fields, or unbounded arrays
from `label_policy.columns`.

Link types may define one variable visual channel using `variable.channel`,
`variable.scale_key`, and `variable.value_column`. Producers emit raw domain
values, such as socket counts or traffic bytes. Cloud or the UI scales values
per `scale_key` across the visible graph. `variable.min` and `variable.max`
are visual tokens for the chosen channel: width variables use width tokens,
opacity variables use opacity tokens.

Actor port bullets require explicit `ports.sources[]` when
`show_bullets: true`. The source may be `links`, `evidence`, or an
`actor_table`. Source column names are table-local and must remain unchanged
during Cloud aggregation. Type ids inside row values and `default_type` values
are rewritten only when they refer to type registries. `ports.sources[].evidence`
is an evidence type id. `name_column` must reference a scalar display column,
not a raw actor/link/evidence reference, array, or JSON cell.

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
actor and graph-link tables only. Detailed mode adds a shared socket
relationship-evidence table for exact tuple matching and drilldowns. The
producer no longer emits the superseded old-schema presentation object or
duplicated actor-nested socket modal tables. It now emits compact graph
presentation metadata in type definitions plus `data.presentation`, and
repeated string columns use dictionary encoding when it reduces raw payload
size.

`topology:snmp` now emits `netdata.topology.v1` from the Function handler
through an adapter over the existing SNMP topology engine output. The adapter
preserves actors, links, L2 observation evidence, actor metadata, and actor
custom detail tables. Remaining SNMP refinement is to promote interface metric
lookup fragments into first-class overlay templates/refs.

`topology:streaming` now emits `netdata.topology.v1` directly from the C
Function. It models streaming agents as compact actor rows, streaming/virtual/
stale relationships as directed graph links with matching evidence types, and
keeps `stream_path`, retention, inbound, and outbound modal data as typed
actor-detail or relationship-summary tables. Streaming also emits graph
presentation metadata for highlight-path behavior, legend, link styles, and
port bullets. Streaming hops remain signed so stale path values are not
corrupted.
