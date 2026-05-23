# SOW-0022 - Topology table composition

## Status

Status: paused

Sub-state: paused while the function-specific network-connections modal product
composition work proceeds in SOW-0025. Agent producer implementation is in-tree,
external read-only review found no remaining actionable issues, and narrow
validation passed. Full integrated UI/aggregator validation is still pending
before close.

## Requirements

### Purpose

Make topology actor and link drilldowns useful, curated, compact, and domain-agnostic. The UI must not display raw producer JSON blobs, long nested arrays, unformatted endpoint structures, or internal matching identifiers as final modal content.

### User Request

The user observed that current topology actor modals and table cells can show raw JSON directly in the final UI, including large actor attribute objects, nested interface/status arrays, neighbor arrays, and endpoint objects. The user explicitly requested analysis of all actor modals and split this as the second remediation step after presentation:

- SOW-0021: fix topology presentation.
- SOW-0022: fix table composition.

### Assistant Understanding

Facts:

- The new compact schema separates actor/link/evidence/detail tables, but modal composition is not yet sufficiently specified.
- Some current UI paths render nested JSON structures directly instead of curated, typed fields.
- Actor modal content includes both relationship evidence and actor-owned custom data, and those need different composition and aggregation semantics.
- User-provided examples contain infrastructure-identifying values and must not be copied into durable artifacts.

Inferences:

- Modal composition needs an explicit schema/profile layer, not just raw table definitions.
- Detail tables need typed column presentation, formatters, visibility defaults, source/purpose, aggregation policy, and safe rendering rules.
- Relationship evidence should power drilldowns without duplicating every evidence row under every actor.

Unknowns:

- The full current cloud-frontend modal renderer shape and all topology-specific assumptions.
- The full set of existing actor/link modal tables for network-connections, streaming, SNMP/L2, and vSphere.

### Acceptance Criteria

- Inventory every current actor/link modal and table source for old and new topology payloads.
- Define table composition profiles for actor details, link details, relationship evidence, relationship summaries, inventory, endpoint summaries, custom actor data, and path tables.
- Define safe scalar, enum, reference, array, and nested object rendering rules so raw JSON blobs do not leak into final UI unless explicitly marked as raw/debug.
- Define actor label and table display-name behavior if not fully closed by SOW-0021.
- Update schema/docs/skill/specs and backend producers as needed.
- Create Cloud frontend and Cloud aggregator handoff requirements for modal/table composition.
- Validate with sanitized fixtures covering SNMP/L2, streaming, and network-connections modal examples.
- Ensure `data.correlation.points` and `data.correlation.claims` are not shown
  as raw actor modal tables unless a future schema explicitly exposes a curated
  debug/diagnostic view.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0021-20260509-topology-presentation-contract.md`
- `.agents/sow/done/SOW-0023-20260509-topology-cross-payload-matching.md`
- `src/plugins.d/FUNCTION_UI_SCHEMA.json:286-372`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:1211-1260`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md:456-462`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md:541-584`
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_schema.go:7-96`
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:635-713`

Current state:

- SOW-0021 restored graph-level presentation for actors, links, ports,
  legends, labels, and highlight paths.
- SOW-0023 added the correlation plane, semantic correlation link types, and
  link layout tokens.
- The old Function UI topology schema had actor-type `summary_fields`,
  `tables`, and `modal_tabs`, plus table column labels and cell types.
- The old SNMP topology producer used those fields to describe device summary
  fields, Ports and Links tables, column labels, badge/number/actor-link cell
  hints, and an Info tab.
- The v1 topology schema currently classifies table types by `role`, `owner`,
  `aggregation`, `source_evidence`, and raw `columns`, but does not define how
  actor/link modals compose summaries, tabs, table sections, visible columns,
  nested values, or relationship-derived rows.
- The v1 developer guide explicitly says full modal/table composition, column
  hiding, nested JSON rendering, and richer formatting belong to this SOW.
- SNMP v1 currently preserves actor attributes/labels as an `actor_metadata`
  actor-detail table with `json` cells, and converts actor-owned dynamic tables
  mechanically. This preserves facts but does not preserve curated modal
  behavior.

Risks:

- Uncurated modal tables can leak sensitive infrastructure details, overwhelm users, and make topology look unfinished.
- Over-modeling table UI can couple backend producers to frontend component internals.
- Under-modeling table UI forces the frontend to hardcode producer-specific modal logic.

### Per-Function Modal/Table Inventory

Design rule:

- Modal/table definitions should describe how to select, filter, join, format,
  and order existing topology facts. They must not duplicate high-cardinality
  rows only for UI display.
- If a value is a canonical actor, link, evidence, inventory, or relationship
  fact, it belongs in the relevant canonical row/table once. Modal composition
  may reference it many times.
- If old modal behavior depended on a value that v1 no longer emits anywhere,
  the fix is to restore that canonical value once in the appropriate actor,
  link, evidence, or detail table, not to create a duplicate modal-only table.
- Raw `json` columns are facts, not UI. They may be preserved for lossless
  debug or future structured expansion, but polished modals should only render
  curated scalar/array/reference projections from them when a schema declares
  that projection.

#### topology:network-connections

Legacy behavior reviewed:

- Presentation came from `topology_write_presentation()` in the legacy
  network viewer function.
- `self` actor summary showed hostname, local IP count, and observed sockets.
  Its `Connections` table used `source: links` and displayed the remote actor,
  protocol, and direction.
- `process` actor summary showed process display name, command, sockets, local
  IP, and user. Its `Sockets` table used actor-owned socket rows and displayed
  remote endpoint, protocol, direction, and state. Its `Connections` table used
  `source: links` and displayed remote actor, protocol, direction, and state.
- `endpoint` actor summary showed endpoint IP, socket count, and address
  space. Its `Connections` table used `source: links` and displayed remote
  actor, protocol, and direction.

Current v1 facts reviewed:

- Actor rows carry type, machine GUID, hostname, process, PID/PPID/UID when PID
  scope is selected, network namespace, local IP/address-space, endpoint
  IP/address-space, display name, and socket count.
- Graph links carry actor refs, link type, protocol, direction, state,
  evidence count, socket count, retransmissions, and RTT maxima.
- Detailed mode emits `socket` relationship evidence with actor refs, local and
  remote tuples, protocol family, direction, state, namespace, process, socket
  count, retransmissions, and RTT maxima.
- Aggregated mode emits compact actor-owned `socket_ports` inventory with actor
  ref, port, protocol, direction, and socket count. This is enough for process
  port bullets without reading detailed socket evidence.
- Correlation points and claims exist for matching, but must not become modal
  tables.

No-duplication reconstruction strategy:

- `self` and `endpoint` `Connections` tables should be generated by filtering
  graph links where `src_actor == actor` or `dst_actor == actor`, then
  deriving `remoteLabel` from the opposite actor ref. Protocol, direction, and
  state come from the link row.
- `process` `Connections` should use the same graph-link projection and can
  optionally filter or group by semantic link types so ownership links do not
  appear as network dependencies unless explicitly requested.
- `process` `Sockets` in detailed mode should be generated by filtering
  `evidence.socket` rows where `src_actor == actor` or `dst_actor == actor`.
  The old `remote` display is a formatted projection of the remote tuple, not
  a stored duplicate.
- `process` `Sockets` in aggregated mode should use graph links plus
  `socket_count`, and `socket_ports` for port bullets. It cannot show every
  socket row because those rows were intentionally not emitted in aggregated
  mode.
- Missing canonical fields: the current v1 implementation fills actor struct
  fields for `username` and `cmdline`, but the actor column list does not emit
  them. Old process summaries cannot be fully reconstructed until those values
  are restored as actor columns. Old `self.local_ip_count` is also not emitted
  as a v1 actor column; either restore it as a self actor metric or drop that
  legacy summary field deliberately.

#### topology:streaming

Legacy behavior reviewed:

- Parent actor summary showed name, node type, agent version, OS, architecture,
  CPU count, child count, critical alerts, and warning alerts.
- Child actor summary showed name, node type, agent version, OS, architecture,
  CPU count, critical alerts, and warning alerts.
- Virtual node summary showed name, node type, and ephemerality.
- Stale actor summary showed name, node type, agent version, OS, and
  architecture.
- Parent `Inbound` table showed node actor link, received-from actor link, node
  type badge, ingest badge, hops, collected metrics/instances/contexts,
  replication completion, ingest age, SSL badge, and alert counts.
- Parent `Outbound` table showed node actor link, streamed-to actor link, node
  type badge, stream status badge, hops, SSL badge, and compression badge.
- `Streaming Path` table showed agent, hops, since, and flags.
- `Retention` table showed actor link, database status, from/to timestamps,
  duration, metrics, instances, and contexts.

Current v1 facts reviewed:

- Actor rows carry type, machine GUID, node ID, hostname, display name,
  severity, ephemerality, ingest status, stream status, ML status, agent name,
  agent version, health status, child count, and health alert counts.
- Link and evidence rows carry streaming/virtual/stale relationship refs,
  state, port name, timestamps, hops, connection and replication metrics, and
  collected metric/instance/context counts.
- Detail tables already exist for `stream_path`, `retention`, `inbound`, and
  `outbound`, with actor refs instead of duplicated display strings.

No-duplication reconstruction strategy:

- Summary fields should read actor columns directly. `node_type` is the actor
  `type` column.
- `Inbound.name` is a projection of `child_actor` rendered through actor label
  policy. `Inbound.received_from` is a projection of nullable `source_actor`.
  `node_type` is derived by joining `child_actor` to the actors table and
  reading its `type`.
- `Outbound.name` is a projection of `actor`. `Outbound.streamed_to` is a
  projection of nullable `destination_actor`.
- `Retention.name` is a projection of `actor`; the label can change per modal
  profile without changing row data.
- `Streaming Path.Agent` is `path_actor` when present, otherwise `hostname`.
  This preserves streaming-path highlighting and avoids storing display strings
  twice.
- Missing canonical fields: old summaries included OS, architecture, and CPU
  count. Current v1 actor rows do not emit those fields. Reconstructing the old
  summary requires restoring them as actor columns if the product still wants
  them in the modal.

#### topology:snmp

Legacy behavior reviewed:

- Device summary showed type, vendor, model, description, location, contact,
  protocols, capabilities, total ports, VLAN count, FDB MAC count, LLDP/CDP
  neighbor counts, chart prefix, Netdata host, source, and layer.
- Device `Ports` table showed port name, operational/admin status, port type,
  link mode, topology role, STP state, VLAN count/list, FDB MAC count, link
  count, and neighbor count.
- Device `Links` table used `source: links` and showed local port, remote
  actor, remote port, protocol, and direction.
- Segment summary showed type, discovery sources, ports total, endpoints total,
  source, and layer.
- Endpoint summary showed type, vendor, discovery sources, source, and layer.

Current v1 facts reviewed:

- Actor rows carry identity and match-oriented fields: type, layer, source,
  display name, chassis IDs, MAC addresses, IP addresses, hostnames, DNS names,
  sysObjectID, sysName, and parent devices.
- Link rows carry source/destination actor refs, semantic link type, protocol,
  direction, state, evidence count, and timestamps.
- Evidence rows currently include `src_endpoint`, `dst_endpoint`, and `metrics`
  as `json` cells.
- Actor detail tables are built mechanically from old actor tables. The old
  device `ports` table becomes `actor_ports`/`actor_ports`-like actor detail,
  while `actor_metadata` preserves raw attributes and labels as JSON.

No-duplication reconstruction strategy:

- Device, segment, and endpoint summaries should read scalar projections from
  actor rows and curated actor-detail columns. They should not display the
  entire `actor_metadata.attributes` or `actor_metadata.labels` objects.
- Device `Ports` should be generated from the actor-owned port inventory table.
  Existing fields such as `name`, `oper_status`, `admin_status`, `port_type`,
  `link_mode`, `topology_role`, `stp_state`, `vlan_ids`, `fdb_mac_count`,
  `link_count`, and `neighbor_count` should be declared as visible columns
  where present. Nested `neighbors` must not render as raw JSON in the main
  ports grid; it needs either a compact count in the row or a nested explicit
  drilldown profile.
- Device `Links` should be generated by filtering graph links for the selected
  actor and joining link/evidence endpoint columns. `remoteLabel` is the
  opposite actor ref; `protocol` and `direction` come from the link row.
  `localPort` and `remotePort` should be projections from structured endpoint
  fields in evidence, not raw endpoint JSON objects.
- Missing canonical fields: many legacy SNMP summary fields live only inside
  `actor_metadata.attributes` today. To avoid raw JSON display, SOW-0022 must
  either move important scalar fields into typed actor columns/typed detail
  columns, or define a safe projection mechanism from JSON paths with strict
  scalar output and hidden-by-default raw JSON.

#### vSphere topology

- vSphere remains legacy in a separate PR worktree and is tracked as a later
  migration SOW. This SOW should define a generic modal/table contract that the
  vSphere migration can use, but it should not edit that worktree without user
  coordination.

### Schema Direction For SOW-0022

The schema needs compact composition definitions, not modal row duplication:

- Actor and link types define modal profiles in presentation metadata.
- Table types define display metadata for existing table rows: label, order,
  default visibility, column labels, column display types, sorting, grouping,
  empty-state behavior, and raw/debug visibility.
- Modal sections reference existing sources:
  `actors`, `links`, `evidence.<type>`, `tables.actor.<table>`,
  `tables.relationship.<table>`, or future typed detail tables.
- A section can declare owner filters such as `actor_ref == selected actor`,
  `link_ref == selected link`, or `src_actor/dst_actor contains selected actor`.
- A section can declare safe projections:
  direct column, actor label from actor ref, opposite actor label from a link,
  formatted endpoint from IP/port columns, scalar JSON-path extraction, array
  length/count, badge/number/timestamp/duration formatting, and nullable
  fallback.
- The UI must never infer producer domain names such as process, router,
  parent, child, client, server, or endpoint. It should execute schema-declared
  projections over topology tables.
- The aggregator should preserve composition definitions and merge compatible
  table metadata by namespace/dedup rules from SOW-0021/SOW-0023. It should
  not materialize modal rows during aggregation unless it is already merging
  the underlying canonical table.

User decisions recorded for this SOW:

- Host labels must be exposed in full, without topology-specific filtering,
  when available. They belong on host/node-level actors and must be shown in
  actor modals as actor labels.
- Non-node actors must also expose all known actor labels. For process actors
  this includes process metadata such as command line, user, group, namespace,
  and similar producer-known facts where available.
- `json` columns should be used only when the UI or aggregator has declared
  semantics for them. If the value is only intended for display or filtering,
  prefer typed scalar/array columns or a key/value label table.
- An actor modal has four top-level entities:
  actor name, actor labels, a depth-1 topology-map miniature, and tables.
- Table composition must support actor-reference cells and row expansion.
  Actor references are table cell projections such as `actor_link`; expandable
  rows are presentation annotations over hidden/detail columns, not separate
  duplicated row data.

Recommended representation for labels:

- Use a compact actor-owned label table rather than one raw JSON map per actor:
  `actor_labels(actor, key, value, source?, kind?, value_index?)`.
- For host/node actors, populate this table from the complete host label set.
- For non-node actors, populate it from producer-known label and metadata
  facts. Keep facts that the aggregator must group or correlate on as
  canonical typed actor columns too; the label table is for display/filter/
  drilldown, not a replacement for canonical identity/grouping columns.
- Repeated label values should use repeated rows with the same `actor` and
  `key`, ordered by `value_index`, rather than JSON arrays.
- Sensitive-data note: topology Functions are sensitive-data surfaces with
  admin-controlled access. This permits exposing host labels, command lines,
  users, and topology metadata in Function responses. Implementation must still
  avoid copying raw label captures into durable repository artifacts or logs.

### Mapping Validation Matrix

This section maps old modal content to the new compact schema model. The goal
is to prove whether information exists once in canonical tables and whether a
table recipe can reconstruct the old polished UI without duplicating rows.

#### Shared Actor Modal Model

| Modal entity | Source in v1 | Needed schema/table recipe |
|---|---|---|
| Actor name | `actors` row via actor type `presentation.label_policy` | Existing SOW-0021 label policy is enough. |
| Actor labels | New `tables.actor.actor_labels` table | Add table type and a default modal section that filters `actor_labels.actor == selected actor`. |
| Depth-1 topology miniature | Existing `actors` and `links` tables | UI can build from incident links and opposite actors; schema may allow optional link-type filters. No duplicated payload. |
| Tables | Existing `actors`, `links`, `evidence.*`, and `tables.actor.*` | Add modal/table composition recipes with source, owner filter, row filters, projections, cell types, visibility, sorting, and row expansion. |

Required generic table-recipe primitives:

- `source`: `actors`, `links`, `evidence.<type>`, `tables.actor.<table>`, or
  `tables.relationship.<table>`.
- `owner_filter`: selected actor/link relationship, for example
  `actor_ref == selected_actor`, `src_actor == selected_actor`,
  `dst_actor == selected_actor`, or either endpoint.
- `row_filters`: link type, evidence type, null/non-null, or value predicates.
- `projection`: direct column, actor label from actor ref, opposite actor label,
  conditional local/remote endpoint field, formatted endpoint, label-table
  lookup, scalar JSON-path only when explicitly declared.
- `cell`: `text`, `number`, `badge`, `actor_link`, `timestamp`, `duration`,
  `endpoint`, `array_count`, or `debug_json`.
- `visibility`: `table`, `expanded`, `hidden`, or `debug`.

#### topology:network-connections Mapping

| Old modal/table item | Current/new canonical source | Recipe/status |
|---|---|---|
| Actor name for self/process/endpoint | `actors.display_name` with label policy fallback to hostname/process/IP | Covered by current actor columns. |
| Self labels | New `actor_labels` rows from complete host labels plus topology facts such as hostname/local IP count/socket count | Add `actor_labels`; add `local_ip_count` as either actor metric column or actor label. |
| Process labels | New `actor_labels` rows from process metadata: process name, PID/PPID/UID when available, user, command line, namespace, local IP/address space, socket count | Add `actor_labels`; emit `username` and `cmdline` because the v1 struct fills them but actor columns do not currently expose them. Group name is not currently collected by network-viewer; add it only if a canonical source is introduced. |
| Endpoint labels | New `actor_labels` rows from IP, address space, socket counts, endpoint class | Add `actor_labels`; existing actor columns cover IP/address-space/socket count. |
| Self `Connections` table | `links` incident to selected self actor | Recipe filters incident links, hides or de-emphasizes `ownership` unless a modal asks for graph-coherence links, projects opposite actor as `actor_link`, plus protocol/direction/state/socket metrics. |
| Process `Connections` table | `links` incident to selected process actor | Recipe filters incident network links, projects opposite actor, protocol, direction, state, socket count, RTT/retransmit metrics. Ownership links should be a separate optional/expanded section. |
| Process `Sockets` table, detailed mode | `evidence.socket` rows where selected actor is `src_actor` or `dst_actor` | Recipe projects formatted remote endpoint from local/remote tuple based on selected side, protocol, direction, state, counts, RTT/retransmit metrics; row expansion can expose PID/UID/netns/process/address-space fields. |
| Process port bullets | `tables.actor.socket_ports` | Already canonical; recipe not needed for graph bullets, but modal can show the same table if useful. |
| Process `Sockets` table, aggregated mode | `links` plus `socket_count`; `socket_ports` for port-level rollup | Covered with aggregated summary table. Exact per-socket rows are intentionally unavailable in aggregated mode. |
| Endpoint `Connections` table | `links` incident to selected endpoint actor | Recipe projects opposite actor as `actor_link`, protocol, direction, state, socket count. |
| Depth-1 miniature | Incident `links` and opposite actors | No new data. The mini graph should probably default to network link types and omit ownership unless explicitly enabled. |

Network-connections validation result:

- Information is mostly present once.
- Required canonical additions: `actor_labels`, emitted `username`, emitted
  `cmdline`, and either emitted `local_ip_count` or a decision to drop that
  old self summary.
- Required schema additions: modal/table recipes, conditional endpoint
  projection, actor-link cell, row expansion visibility.

#### topology:streaming Mapping

| Old modal/table item | Current/new canonical source | Recipe/status |
|---|---|---|
| Actor name | `actors.display_name` / `actors.hostname` via label policy | Covered. |
| Host labels | New `actor_labels` rows from complete `host->rrdlabels` for every RRDHOST-backed actor | Add `actor_labels`; old code nested host labels under actor labels. |
| Streaming actor labels | New `actor_labels` rows from actor columns and host/system metadata: node type, severity, ephemerality, ingest/stream/ML status, agent name/version, health status, child count, alert counts | Add `actor_labels`; do not duplicate graph identity fields outside canonical actor columns. |
| Parent/child summaries: name, type, version, child count, health counts | `actors` columns and/or `actor_labels` | Covered for name/type/version/child/health. |
| Parent/child/stale summaries: OS, architecture, CPU count | Old code emitted system info from `rrdhost_system_info_to_json_object_fields`; current v1 actor columns do not expose it | Add to `actor_labels` from host system info; add typed actor columns only if aggregator/UI needs grouping or sorting by these fields. |
| Virtual node summary: ephemerality | `actors.ephemerality` | Covered. |
| Parent `Inbound` table | `tables.actor.inbound` | Covered by current typed table: parent, child, source actor refs; status, hops, metrics, replication, age, SSL, alert counts. Recipe maps `child_actor` to old `name`, `source_actor` to old `received_from`, and child actor type to old `node_type`. |
| Parent `Outbound` table | `tables.actor.outbound` | Covered by current typed table. Recipe maps `actor` to old `name`, nullable `destination_actor` to old `streamed_to`, actor type to old `node_type`, then status/hops/SSL/compression. |
| `Streaming Path` table | `tables.actor.stream_path` | Covered by current typed table. Recipe maps `path_actor` to actor link when present, fallback `hostname`; shows hops/since/flags; expanded rows may show host/node/claim IDs and capabilities. |
| `Retention` table | `tables.actor.retention` | Covered by current typed table. Recipe maps `actor` or `observer_actor` to actor link depending on selected actor type, then status/from/to/duration/metrics/instances/contexts. |
| Highlight path | Existing SOW-0021 `data.presentation.selection.highlight_path` using `stream_path` | Covered outside modal table composition. |
| Depth-1 miniature | Incident streaming/virtual/stale links and opposite actors | No new data. |

Streaming validation result:

- Relationship tables are already modeled correctly and compactly.
- Required canonical additions: `actor_labels`, host labels, and host/system
  metadata labels for OS/architecture/CPU parity.
- Required schema additions: recipes that project actor refs as old
  actor-link cells, fallback actor labels for path rows, and expanded-row
  visibility for host/node/claim/capability fields.

#### topology:snmp Mapping

| Old modal/table item | Current/new canonical source | Recipe/status |
|---|---|---|
| Actor name | `actors.display_name` / `actors.sys_name` via label policy | Covered by current actor columns and label policy. |
| Actor labels | New `actor_labels` rows from actor labels and scalar/array metadata currently stored in `actor_metadata.attributes` | Add `actor_labels`; repeated array values should be repeated rows. Do not render raw `actor_metadata` as the user-facing label view. |
| Device summary: type, source, layer | `actors.type`, `actors.source`, `actors.layer` | Covered. |
| Device summary: vendor/model/sys description/location/contact/protocols/capabilities/ports/VLAN/FDB/LLDP/CDP/chart/netdata host | Currently mostly inside raw `actor_metadata.attributes`; some identity fields are in `actors` | Move important scalar/count fields into typed actor columns or typed actor-detail columns; also expose as `actor_labels`. This avoids raw JSON and supports sorting/filtering. |
| Segment summary: type/source/layer | `actors` columns | Covered. |
| Segment summary: learned sources, ports total, endpoints total | Currently attributes/labels, not typed actor columns | Add typed fields or `actor_labels` rows; use typed columns if used for sorting/filtering. |
| Endpoint summary: type/source/layer | `actors` columns | Covered. |
| Endpoint summary: vendor, learned sources | Currently attributes/labels, not typed actor columns | Add typed fields or `actor_labels` rows. |
| Device `Ports` table | Actor-owned port table currently derived from old `ports` rows | Partly covered. Current required `actor_ports` type only declares `name`, `topology_role`, `oper_status`, and `link_mode`; dynamic actor tables can carry more, but schema/presentation is not curated. Need a stable `actor_ports` inventory/detail table with visible columns for name/status/admin/type/mode/role/STP/VLAN/FDB/link/neighbor counts and expanded columns for aliases, speeds, chart refs, and neighbor details. |
| Device `Links` table | `links` plus evidence endpoint fields | Partly covered. Current SNMP evidence stores `src_endpoint`, `dst_endpoint`, and `metrics` as JSON. Need structured endpoint columns such as source/destination port ID/name/if_index/if_name/display name/management IP and structured metric fields needed by link modals. Recipe uses conditional local/remote projection based on whether selected actor is `src_actor` or `dst_actor`. |
| Link protocol/direction/state | `links.protocol`, `links.direction`, `links.state` | Covered. |
| Inferred vs verified link distinction | `links.type` and link presentation from SOW-0021 | Covered for graph; modal/legend recipes should expose link type/status. |
| Raw neighbors/endpoint objects | Existing nested JSON values | Should not render raw in main tables. Map to counts in table rows, and expose detailed nested information only through explicit expanded sections or typed child tables. |
| Depth-1 miniature | Incident L2 links and opposite actors | No new data. Mini graph can use existing link presentation, including inferred/verified link styles. |

SNMP validation result:

- Current schema preserves most facts, but too many user-facing facts are only
  reachable through JSON.
- Required canonical additions: `actor_labels`; typed actor/detail columns for
  important summary fields; stable, richer `actor_ports` table; structured
  SNMP evidence endpoint/metric columns replacing user-facing dependence on
  `src_endpoint`, `dst_endpoint`, and `metrics` JSON.
- Required schema additions: conditional local/remote endpoint projections,
  expanded-row visibility, and explicit debug-only handling for any remaining
  raw JSON.

#### vSphere Mapping

- vSphere is still legacy and tracked by a later SOW. The SOW-0022 contract
  must be generic enough for vSphere inventory actors and relationship tables:
  actor labels, mini depth-1 topology, actor/link tables, actor-link cells, and
  expandable rows.
- No vSphere producer changes should happen in this worktree without user
  coordination.

### Validation Outcome Before Implementation

The mapping exercise shows that implementation can proceed after the schema
adds the following generic primitives:

1. `actor_labels` as a first-class actor-owned table type.
2. Modal/table composition recipes on actor/link types or table types.
3. Cell annotations including `actor_link`, badge, number, timestamp, duration,
   endpoint, and debug JSON.
4. Projection primitives for direct columns, actor-ref labels, opposite actor,
   conditional local/remote endpoint columns, formatted endpoints, label-table
   lookup, and explicitly declared scalar JSON paths.
5. Column/row visibility for table view, expanded row, hidden, and debug.
6. Mini topology composition from existing incident links and actors, with
   optional link-type filters.

The mapping also shows producer-specific canonical field work:

1. `topology:network-connections`: emit process `username`, process `cmdline`,
   `actor_labels`, and self `local_ip_count` if retained.
2. `topology:streaming`: emit `actor_labels`, complete host labels, and
   host/system metadata labels used by old summaries.
3. `topology:snmp`: replace user-facing JSON dependence with `actor_labels`,
   typed summary fields, richer stable port rows, and structured link endpoint
   evidence columns.

## Pre-Implementation Gate

Status at implementation start: ready (historical snapshot; current SOW state is recorded in the top-level Status section).

Problem / root-cause model:

- Modal/table composition is underspecified. Producers can preserve useful facts, but the UI lacks enough schema-level guidance to turn those facts into polished modal content without raw JSON fallback or producer-specific hardcoding.

Evidence reviewed:

- User-provided examples of raw JSON leaking into final UI. Raw examples are intentionally not copied into this durable artifact.
- Old schema support for modal composition is defined in
  `src/plugins.d/FUNCTION_UI_SCHEMA.json:286-372`.
- Old SNMP modal composition is defined in
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation_schema.go:7-96`.
- V1 table type metadata stops at structural table classification in
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:1211-1260`.
- V1 SNMP currently emits raw actor metadata JSON in
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:687-713`.

Affected contracts and surfaces:

- `netdata.topology.v1` table/detail schema.
- Cloud frontend actor/link modal renderer.
- Cloud topology aggregator table merge behavior.
- Backend topology producers.
- Developer guide, topology spec, and `project-create-topology` skill.

Existing patterns to reuse:

- SOW-0021 presentation profiles.
- SOW-0023 pure correlation actors, points, claims, and semantic correlation
  link types.
- Existing compact table roles and column metadata.
- Old topology presentation table and modal-tab metadata as inventory input.

Risk and blast radius:

- Applies to all topology producers and the Cloud UI.
- Can affect payload size if table presentation metadata is repeated per row.
- Raw data examples may contain sensitive information and must remain sanitized.

Sensitive data handling plan:

- Do not copy raw customer/infrastructure details into durable artifacts.
- Use sanitized fixtures and placeholders only.
- Keep any raw captures under `.local/`.

Implementation plan:

1. Record the chosen table/modal composition contract.
2. Inventory current modal/table behavior across producers and UI.
3. Define compact table-composition profiles without high-cardinality
   repetition.
4. Update schema/docs/skill/specs and backend producers.
5. Create Cloud frontend and Cloud aggregator handoff artifacts.
6. Validate with sanitized fixtures.

Validation plan:

- Pending SOW-0021 and SOW-0023 output.

Artifact impact plan:

- AGENTS.md: no expected update unless workflow rules change.
- Specs: likely update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: likely update `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- Runtime project skills: likely update `.agents/skills/project-create-topology/SKILL.md`.
- End-user/operator skills: likely unaffected unless public operator workflows change.
- SOW lifecycle: current/in-progress; close only after implementation,
  integrated validation, and commit.

Open-source reference evidence:

- No external OSS reference was used for this contract pass. The work is a
  Netdata-specific payload/UI contract derived from old Netdata topology modal
  behavior and current `netdata.topology.v1` producer facts.

Open decisions:

- Resolved: table/modal composition metadata lives in actor/link type
  `presentation.modal`, with reusable table defaults in
  `types.table_types.<id>.presentation`.
- Resolved: actor and link modal composition belong to the same contract so
  table recipes and cell/projection tokens stay consistent.
- Resolved: raw `json` cells are hidden/debug-only unless an explicit schema
  projection extracts a curated scalar value or a future structured child table
  is defined.

## Implications And Decisions

1. User decision: table and actor-modal composition is separate from topology presentation and should be handled as SOW-0022.
2. User decision: document the target contract first, then implement. Before
   Agent implementation starts, create separate Cloud aggregator and Cloud UI
   handoff artifacts.
3. Contract decision: modal sections are recipes over existing facts; they do
   not duplicate high-cardinality rows or raw actor metadata for display.
4. Contract decision: actor labels use a compact actor-owned
   `actor_labels(actor, key, value, source?, kind?, value_index?)` table.
5. Contract decision: the Cloud UI must reuse existing topology modal/table
   components where practical and must not reimplement the old table stack from
   scratch for v1.

## Plan

1. Update topology schema, developer guide, durable spec, and topology
   producer skill with the modal/table composition contract.
2. Create a pending Cloud topology service SOW for aggregation behavior.
3. Create a Cloud frontend TODO for UI behavior and component reuse.
4. Implement Agent producer changes for network-connections, streaming, and
   SNMP/L2 after the contract/handoffs are accepted.
5. Wait for Cloud UI and Cloud aggregator implementation slices, then run
   integrated QA before closing this SOW.

## Execution Log

### 2026-05-09

- Opened as pending follow-up from user direction.

### 2026-05-10

- Updated dependencies after SOW-0023 added the correlation plane and link
  layout contract. Actor modals must not expose correlation point/claim tables
  as raw user-facing content.
- Completed the per-function mapping exercise for network-connections,
  streaming, SNMP/L2, and future vSphere migration.
- Began contract-first update of schema/spec/developer-guide/skill before
  Agent producer implementation.
- Moved SOW-0022 to current/in-progress after the user approved Agent backend
  implementation. Cloud aggregator SOW-0009 remains pending in the service
  repo until the service worker finishes SOW-0008.
- Implemented shared Go `netdata.topology.v1` modal/table composition structs
  and semantic validation for modal labels, mini topology link filters,
  section sources, owner filters, projections, cell visibility, table type
  presentation, and sort columns.
- Implemented `topology:network-connections` producer additions:
  `actor_labels`, process `username`, process `cmdline`, self
  `local_ip_count`, modal recipes over graph links, socket evidence, and
  `socket_ports`.
- Implemented `topology:streaming` producer additions: complete host-label
  export into `actor_labels`, system metadata labels/actor columns for OS,
  architecture, and CPU count, graph-link `port_name`, and modal recipes over
  existing `stream_path`, `retention`, `inbound`, and `outbound` tables.
- Implemented `topology:snmp` producer additions: `actor_labels`, typed summary
  fields, stable `actor_ports`, structured evidence endpoint columns, and
  actor modal recipes that avoid raw JSON as the default user-facing view.
- Added shared schema tests for v1 modal recipes and SNMP producer assertions
  for `actor_labels` and modal presence.
- Ran read-only reviews with GLM, Kimi, MiMo, Qwen, and MiniMax against this
  SOW and the uncommitted implementation.
- Addressed reviewer findings:
  - required `value` for `const` modal projections in schema and semantic
    validation;
  - required a local and remote side for `selected_side_endpoint` projections;
  - preserved explicit SNMP zero-valued counts/indexes instead of treating
    them as missing;
  - removed the misleading SNMP `actor_ports` table-type overwrite pattern;
  - added SNMP evidence-column and zero-preservation assertions.
- Ran a second read-only review round after those fixes, then addressed the
  concrete findings:
  - normalized empty nullable SNMP `protocols` and `capabilities` arrays to
    `null`;
  - replaced fragile SNMP empty-array type assertions with a helper;
  - tightened `selected_side_endpoint` semantic validation so empty-string side
    columns do not satisfy the local/remote requirement;
  - made evidence-section validation return an error instead of panicking on a
    malformed section;
  - deduplicated the SNMP port modal column recipe shared by the device modal
    and `actor_ports` table presentation;
  - added tests for invalid evidence-section shape, empty selected-side column
    validation, and nullable SNMP array normalization.
- Ran a third read-only review round with the same scope. Concrete fixes from
  that round:
  - aligned JSON schema and semantic validation for `label_lookup`,
    `json_path`, `coalesce`, and row-filter `value`/`values` requirements;
  - added schema and semantic negative tests for invalid modal projections and
    row filters;
  - documented that `actor_labels` logical string fields may be encoded as
    `string` or `string_ref`, and that aggregators/UI adapters must normalize
    both encodings;
  - removed a stale modal-column validator parameter;
  - documented and tested the SNMP `protocols` fallback from legacy
    `learned_sources`;
  - extended the SNMP test table decoder to support `dict` encodings.
- Ran a fourth read-only review round with the same scope. Concrete fixes from
  that round:
  - required `formatted_endpoint` projections to name at least an IP or port
    column in JSON schema and Go semantic validation;
  - made semantic validation reject explicit non-array `modal.sections`;
  - made the modal projection switch fail closed if a future unsupported kind
    reaches the semantic validator;
  - documented that `actor_labels` inherits topology Function sensitive-data
    access-control assumptions;
  - added regression tests for empty `formatted_endpoint` and malformed
    `modal.sections`.

## Validation

Acceptance criteria evidence:

- Contract artifacts now define `actor_labels`, modal sections, source kinds,
  owner filters, projections, cell types, visibility, table presentation, and
  raw JSON/debug rules in:
  - `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
  - `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
  - `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`
  - `.agents/sow/specs/topology-function-schema.md`
  - `.agents/skills/project-create-topology/SKILL.md`
- Cloud frontend handoff created at
  `../../dashboard/cloud-frontend/TODO-topology-modal-composition-contract.md`.
- Cloud aggregator handoff SOW created at
  `../../netdata/cloud-topology-service/.agents/sow/done/SOW-0009-20260510-modal-composition-and-actor-labels.md`.

Tests or equivalent validation:

- `python -m json.tool src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `git diff --check`
- `cd src/go && go test ./pkg/topology/v1 ./plugin/go.d/collector/snmp_topology`
- C syntax/type checks were run manually with the exact include/define flags
  from `build/compile_commands.json`, replacing object output with
  `-fsyntax-only`, for:
  - `src/collectors/network-viewer.plugin/network-viewer.c`
  - `src/web/api/functions/function-topology-streaming.c`
- `git -C ../../dashboard/cloud-frontend diff --check TODO-topology-modal-composition-contract.md`
- `git -C ../../netdata/cloud-topology-service diff --check .agents/sow/done/SOW-0009-20260510-modal-composition-and-actor-labels.md`

Real-use evidence:

- Not run yet for the completed producer implementation. The local `build/`
  directory is root-owned, so targeted Ninja object rebuilds could not write
  `.ninja_lock` or `.ninja_log`; syntax/type checks were run instead with the
  exact compile flags. Full real-use evidence requires rebuilding/installing
  the Agent and validating with the updated Cloud UI/aggregator path.

Reviewer findings:

- GLM, Kimi, MiMo, Qwen, and MiniMax completed read-only reviews of
  `.agents/sow/current/SOW-0022-20260509-topology-table-composition.md` and
  the uncommitted implementation.
- Findings accepted and fixed:
  - `const` projections were schema-valid without a `value`;
  - `selected_side_endpoint` projections were schema-valid with no usable
    endpoint side columns;
  - SNMP nullable integer handling collapsed explicit zero values into missing
    values;
  - SNMP `actor_ports` table-type construction had a confusing overwrite;
  - SNMP structured evidence columns needed direct test coverage.
- Findings reviewed but not changed:
  - empty `actor_labels` tables are currently allowed for a stable table
    contract and tiny overhead;
  - raw/debug JSON remains available only through explicit debug visibility;
  - Cloud UI projection-engine coverage remains tracked in the Cloud frontend
    TODO, not in the Agent producer implementation;
  - modal recipes are intentionally type-level payload metadata; they are not
    repeated per high-cardinality row;
  - modal sources may reference declared table types even when a runtime table
    has zero rows or is omitted, allowing the UI to render stable empty
    sections.
- Round-2 findings accepted and fixed:
  - empty nullable SNMP `protocols`/`capabilities` arrays normalized to `null`;
  - fragile empty-array type assertions replaced with `isEmptyArrayCell`;
  - `selected_side_endpoint` semantic validation now rejects empty-string side
    columns;
  - malformed evidence section objects now return validation errors;
  - duplicated SNMP port modal column recipe collapsed into one helper.
- Round-3 findings accepted and fixed:
  - JSON schema now conditionally requires `label_key`, `path`, non-empty
    `columns`, and row-filter `value`/`values` in the same cases covered by
    semantic validation;
  - Go semantic validation now rejects row filters that omit required values;
  - `actor_labels` encoding compatibility is documented for direct strings and
    dictionary references;
  - SNMP `protocols` legacy fallback is explicit and covered by producer tests;
  - SNMP test helpers now decode `dict` compact-table columns.
- Round-3 findings reviewed but not changed:
  - streaming may expose both curated system labels and complete host labels;
    complete host labels are an explicit product requirement and UI grouping can
    decide how to present duplicates;
  - streaming `actor_labels` table type does not need a table-presentation
    fallback because actor modal labels are driven by `presentation.modal.labels`;
  - C producer modal helper duplication is acceptable for now because the two C
    producers do not share a common topology-emitter module;
  - SNMP `sys_contact` and `sys_location` remain visible because topology
    Functions are already marked sensitive and access-controlled by the admin.
- Round-4 findings accepted and fixed:
  - empty `formatted_endpoint` projections now fail schema and semantic
    validation;
  - explicit non-array `modal.sections` values now fail semantic validation;
  - semantic modal projection validation fails closed for unsupported kinds;
  - `actor_labels` sensitive-data inheritance is documented in the developer
    guide, durable spec, and topology producer skill.
- Round-4 findings reviewed but not changed:
  - C producer test coverage remains an integrated-QA risk because current local
    C Function testing is manual/syntax-level only;
  - C modal helper duplication remains accepted for this SOW;
  - plain `string` actor-label columns in C producers are allowed by the
    documented contract and avoid adding new C dictionary infrastructure here;
  - payload-size growth remains a pre-close measurement/integrated-QA item.
- Round-5 findings accepted and fixed:
  - `selected_side_endpoint` projections are now self-contained: schema,
    semantic validation, producer docs, durable spec, and topology producer
    skill require source/destination actor-ref columns plus both endpoint
    sides;
  - network-connections socket modal projections now emit those side actor
    columns;
  - SNMP device link modal columns now use selected-side projections for
    `Local Port` and `Remote Port` instead of source/destination labels;
  - semantic validation now rejects explicitly configured optional actor-label
    columns when the referenced column does not exist, while still allowing
    optional label columns to be omitted from a table type;
  - `actor_labels` column roles are aligned across producers by marking label
    key/value/source/kind/index as attributes and actor as the reference.
- Round-5 findings reviewed but not changed:
  - C producer modal helper duplication remains accepted for this SOW;
  - C producer modal JSON has syntax/type validation but not dedicated C unit
    tests yet;
  - payload-size growth remains a pre-close measurement/integrated-QA item
    because realistic local/Cloud payloads are needed to measure the final
    producer output.
- Round-6 findings accepted and fixed:
  - `ValidateDecodedResponse` now fails closed for malformed response
    envelopes, missing `data`, and wrong schema versions;
  - compact-table semantic validation now rejects empty column ids, duplicate
    column ids, missing column types, unsupported column types, and primitive
    values that do not match the declared column type;
  - modal section and modal column semantic validation now rejects missing or
    empty `id` and `label`;
  - JSON schema now mirrors modal source, owner-filter, and projection
    conditional requirements for `table`, `evidence`, actor/link side columns,
    and direct/actor/opposite projection columns;
  - SNMP actor-owned custom tables named `labels` or `metadata` can no longer
    overwrite the built-in `actor_labels` or `actor_metadata` tables;
  - streaming graph link rows now emit the metric columns declared by their
    link type aggregation policy;
  - streaming `info` requests now return metadata without building or emitting
    the full topology payload.
- Round-6 findings reviewed but not changed:
  - C helper duplication remains accepted until/unless a shared C topology
    emitter module is introduced;
  - payload-size growth remains a pre-close measurement item;
  - dedicated C producer unit/snapshot tests remain an integrated-QA risk
    unless a small Function fixture harness is added before close.
- Round-7 findings accepted and fixed:
  - JSON schema now mirrors semantic validation for `json_path` by requiring
    both `column` and `path`;
  - `modal_section.label`, `label_key`, and `json_path.path` now reject empty
    strings at schema level where applicable;
  - semantic validation now rejects modal sections with missing or empty
    `columns`, actor type presentation labels that are explicitly empty, and
    non-integer modal mini-topology `depth`;
  - semantic validation now reports empty `row_filters[].values` separately
    from wrong-type or missing `values`;
  - SNMP custom actor-detail table ids now reserve built-in replacement ids and
    generate unique ids, so `labels`, `metadata`, `custom_labels`, or similar
    table names cannot overwrite one another;
  - SNMP `actor_ports` now preserves unknown custom port fields in an `extra`
    debug JSON column instead of dropping them when normalizing the stable port
    table;
  - SNMP `vlan_ids` and similar array labels now stringify scalar typed arrays,
    including integer arrays, instead of accepting only `[]string`/`[]any`;
  - SNMP neighbor-count inference now runs only when the `neighbors` value is a
    valid array of objects, avoiding a misleading zero for malformed values;
  - docs and the topology producer skill now clarify that Function `info`
    responses are metadata-only and are not validated as full topology payloads;
  - docs now describe `empty_label`, `badge_map`, `align`, `sortable`, optional
    `label_lookup.actor_column`, and required `json_path.column`/`path`.
- Round-7 findings reviewed but not changed:
  - `label_lookup.actor_column` remains optional by design. When omitted, the UI
    should look up labels for the selected modal actor; producers provide
    `actor_column` only for source-row actor references.
- Round-8 findings accepted and fixed:
  - JSON schema now gives `modal_section.label`, `link_type_presentation.label`,
    `port_type_presentation.label`, and `table_type_presentation.label`
    `minLength: 1`, matching semantic validation for explicit empty labels;
  - semantic validation now rejects explicit empty labels in link type, port
    type, and table type presentation, in addition to actor type and modal
    section/column labels;
  - validation tests now cover explicit empty presentation labels for actor,
    link, port, and table type presentation;
  - the current SOW file was marked with git intent-to-add so `git diff` based
    reviewers see the pending-to-current SOW move before the final commit.
- Round-8 findings reviewed but not changed:
  - `presentation.modal.sections: []` remains valid. A modal may provide labels
    and/or a mini-topology without table sections, and producers with no curated
    tables should not be forced to invent empty sections;
  - SNMP `anyStringSlice` keeps the small reflection fallback to preserve scalar
    typed arrays from non-JSON producers without adding a long type-switch;
  - C producer unit/snapshot tests and shared C modal emitter refactoring remain
    integrated-QA or follow-up risks already tracked in this SOW;
  - table type presentation validation intentionally validates type-registry
    table definitions. Runtime actor tables are validated separately and
    `presentation.modal.labels` already falls back to actor table columns when
    resolving label tables.
- Round-9 findings accepted and fixed:
  - semantic validation now rejects duplicate modal section ids and duplicate
    modal column ids, matching the duplicate-column guard already used for
    compact tables;
  - the streaming inbound modal recipe now projects the nullable
    `source_actor` as `Received from`, matching the old inbound table behavior
    and the SOW mapping;
  - streaming inbound and outbound modal recipes now expose the already-existing
    node type, collected instance/context, ingest age, TLS, alert-count, and
    outbound node columns needed to preserve old visible table functionality
    without duplicating row data;
  - streaming link type aggregation now declares `replication_completion: avg`;
  - network-connections ownership links now declare `socket_count: sum`, and
    network-connections actor labels include the canonical actor `type`, aligned
    with streaming actor-label behavior;
  - the Go validator comment now states that `ValidateDecodedResponse` is for
    full topology responses, not metadata-only Function `info` responses;
  - validation tests now cover the non-object `presentation.modal` shape.
- Round-10 findings reviewed but not changed:
  - C modal helper duplication remains accepted and tracked for a future shared
    emitter/refactor because the current C producers do not share a topology
    JSON helper module;
  - C producer unit/snapshot tests, payload-size measurement, and integrated
    Agent/UI/aggregator QA remain pre-close or follow-up items tracked in this
    SOW;
  - SNMP `anyStringSlice` reflection fallback remains intentional for typed
    scalar arrays from non-JSON producer data;
  - `formatted_endpoint` remains permissive by design: IP-only or port-only
    endpoints are valid when a producer has only partial endpoint facts.
- Round-11 final external review:
  - GLM, Kimi, MiMo, MiniMax, and Qwen were rerun on the same full SOW-0022
    scope after the Round-10 fixes, with only short fix notes appended;
  - no reviewer reported a new actionable or blocking issue;
  - remaining reviewer notes were informational only and matched already
    tracked risks: C modal helper duplication, missing C producer
    unit/snapshot tests, payload-size measurement, integrated Agent/UI/
    aggregator QA, the intentional SNMP `anyStringSlice` reflection fallback,
    and intentionally permissive partial endpoint formatting;
  - the final Qwen review also verified that modal/table composition remains
    generic, avoids modal-only high-cardinality duplication, and keeps
    sensitive actor-label handling documented.

Same-failure scan:

- Searched updated contract artifacts for stale relationship-table naming,
  old public create-topology skill references, and unresolved
  pre-implementation decision markers.

Sensitive data gate:

- Raw user-provided examples are not copied into this SOW. This SOW uses sanitized summaries only.

Artifact maintenance gate:

- AGENTS.md: no workflow rule change in this step.
- Runtime project skills: updated `.agents/skills/project-create-topology/SKILL.md`.
- Specs: updated `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: no operator workflow change. Updated developer-facing
  Function docs under `src/plugins.d/`.
- End-user/operator skills: unaffected; this is developer topology work, not an
  operator skill change.
- SOW lifecycle: remains in `current/` with `Status: paused`; do not close
  until integrated QA and commit are complete.

Specs update:

- Updated `.agents/sow/specs/topology-function-schema.md`.

Project skills update:

- Updated `.agents/skills/project-create-topology/SKILL.md`.

End-user/operator docs update:

- Not affected. Developer docs updated:
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` and
  `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`.

End-user/operator skills update:

- Not affected.

Lessons:

- The old modal behavior can be reconstructed without duplicating modal-only
  rows only if producers restore missing canonical fields first.

Follow-up mapping:

- Cloud frontend implementation is tracked by
  `../../dashboard/cloud-frontend/TODO-topology-modal-composition-contract.md`.
- Cloud aggregator implementation is tracked by
  `../../netdata/cloud-topology-service/.agents/sow/done/SOW-0009-20260510-modal-composition-and-actor-labels.md`.
- Function-specific modal product composition is intentionally split out and
  tracked separately:
  - `.agents/sow/done/SOW-0025-20260511-network-connections-modal-product-composition.md`;
  - `.agents/sow/done/SOW-0026-20260511-snmp-modal-product-composition.md`;
  - `.agents/sow/current/SOW-0027-20260511-streaming-modal-product-composition.md`.
- Integrated Agent/UI/aggregator QA remains in this SOW after the other workers
  finish their implementation slices.

## Outcome

Contract documentation, Cloud handoff artifacts, shared Go schema validation,
and Agent producer implementation are prepared. Full integrated QA is still
pending before this SOW can close.

## Lessons Extracted

- See the Validation section's Lessons entry for the current extracted lesson.

## Followup

- See the Validation section's Follow-up mapping entry for the active tracker
  list. This SOW is paused until integrated Agent/UI/aggregator QA can close
  those mapped items.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
