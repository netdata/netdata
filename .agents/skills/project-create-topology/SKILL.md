---
name: project-create-topology
description: Developer workflow for creating or updating Netdata topology producers and topology Function payloads using the production netdata.topology.v1 schema. Use when adding or migrating topology:network-connections, topology:streaming, topology:snmp, vSphere topology, correlation rules, graph presentation, drilldowns, direction semantics, telemetry overlays, or Cloud topology aggregation fixtures.
type: project
---

# Create Netdata Topologies

## What This Skill Is

This is a developer skill for assistants working in this repository. It is not
an end-user/operator skill. Use it when changing topology producers, schema
fixtures, validation, topology developer documentation, or Cloud/frontend
handoff artifacts.

## Required References

Read these before designing or changing topology payloads:

| File | Purpose |
|---|---|
| `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` | JSON Schema for production topology payloads |
| `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` | Human-readable topology schema contract and producer guidance |
| `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` | Backend/frontend/aggregator migration scope |
| `.agents/sow/specs/topology-function-schema.md` | Durable project spec for topology semantics |
| `.agents/sow/specs/topology-modes-correlation-aggregation.md` | Mode, correlation, aggregation, and actor modal identification contract |
| `.agents/skills/project-writing-collectors/SKILL.md` | Collector quality, Function, validation, and cardinality rules |

For transport-level Function behavior, also read:

- `src/plugins.d/FUNCTION_UI_REFERENCE.md`
- `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`

## Developer How-Tos

The how-to catalog lives under [`how-tos/`](./how-tos/). These recipes are
developer-facing and must stay in this project skill, not under
`docs/netdata-ai/skills/`.

## Core Rules

- Production payloads carry canonical topology facts for the aggregator and UI.
- Test-only projection code may reconstruct compatibility payload shapes to
  prove parity.
- Never add compatibility reconstruction fields, old-schema adapter names, or
  duplicated display strings to production payloads.
- Keep display composition in type-level and graph-level presentation metadata,
  not in high-cardinality rows.
- Keep raw sensitive payload captures under `.local/` only.

## Workflow

1. Define the topology purpose and scale target.
   - Identify the graph users need: nodes, processes, containers, L2 devices,
     vSphere inventory, streaming parents, or another domain.
   - Estimate actor count, graph-link count, evidence-row count, and payload
     size on realistic data.

2. Pick actors.
   - Use stable identities.
   - Keep display names separate from identity.
   - Declare `identity`, `merge_identity`, and `parent_identity` in actor types.
   - Prepare aggregation scopes such as node, process name, PID, container,
     Kubernetes workload, SNMP device/interface, or vSphere object.

3. Pick graph links.
   - Graph links are renderable relationship groups.
   - Keep graph links compact.
   - Put one-to-many observation detail in evidence sections.
   - Define direction semantics in link types.
   - Use distinct semantic link types for ownership, local/resolved links,
     correlation links, inferred links, and partial links when their meaning or
     layout behavior differs.

4. Pick evidence rows.
   - Evidence is the lossless relationship proof.
   - For sockets, preserve the exact matching tuple.
   - For SNMP/L2, preserve LLDP/CDP/FDB/ARP/STP facts according to role.
   - For streaming, keep relationship facts separate from actor-owned path data.
   - For vSphere, preserve inventory/relationship facts using stable object IDs.

5. Classify detail tables.
   - `actor_detail`: custom actor state, not generally aggregatable.
   - `actor_inventory`: actor-owned inventory data.
   - `relationship_evidence`: exact relationship rows.
   - `relationship_summary`: derived summaries.
   - Use `json` columns only for custom actor/detail cells that must preserve
     nested producer-owned values; avoid them for high-cardinality evidence.
   - Use a compact actor-owned `actor_labels` table for modal labels:
     `actor`, `key`, `value`, optional `source`, optional `kind`, and optional
     `value_index`.
   - Expose complete host/node labels when available.
   - Expose useful non-node actor labels and metadata, while keeping identity,
     correlation, grouping, sorting, filtering, and aggregation facts as typed
     canonical columns.

6. Define telemetry overlays.
   - Use overlay templates once per payload or type.
   - Links and actors carry compact refs and parameters only.
   - Do not put full metric query payloads on every row.

7. Define correlation semantics when actors can be resolved across payloads.
   - Declare whether the topology needs loose-side resolution, actor
     replacement, actor enrichment, or visible correlation actors.
   - Do not hide correlation state as flags on real actors.
   - Define `data.correlation.rules` with declarative key templates,
     priorities, `class`, `absorb` or `link` actions, point actor types when
     visible correlation actors exist, optional claim actor types, correlation
     link types, and output link types.
   - Emit compact `data.correlation.points` rows for visible correlation actors
     when the input graph has them, and `data.correlation.claims` rows for real
     actors that can satisfy keys.
   - For high-cardinality exact observations, prefer loose relationship-side
     facts plus declared materialization policy over creating one actor per
     ephemeral endpoint.
   - Use `absorb` only for exact matches that should remove correlation actors
     or loose-side placeholders from the aggregated output.
   - Use `link` for broader or partial matches that should keep the correlation
     actor or materialized partial actor visible.
   - Use `replace_actor` semantics for weaker placeholder actors that should be
     replaced by stronger managed actors.
   - Use `merge_enrich_actor` semantics when multiple payloads provide
     complementary facts for the same actor identity.
   - Keep NAT or alias information as additional point/claim rows, not as
     mutation of the original observation.

8. Define graph presentation.
   - Put actor presentation in `types.actor_types.<id>.presentation`.
   - Put link presentation in `types.link_types.<id>.presentation`.
   - Put graph port-bullet presentation in `types.port_types.<id>.presentation`.
   - Put legend, actor-click highlight behavior, port fields, and scale keys in
     `data.presentation`.
   - Use `__topology_mode` for detailed vs aggregated topology requests when a
     producer has a real mode difference. Do not expose a mode selector for
     mode-invariant topologies.
   - Use UI-owned color/icon/line/width/opacity/layout tokens only.
   - Define `label_policy.columns` with safe scalar display columns; never let
     canonical identity arrays become actor names.
   - Define `ports.sources[]` whenever an actor type sets
     `ports.show_bullets: true`.
   - Use scalar display columns for `ports.sources[].name_column`; do not use
     refs, arrays, or JSON as graph bullet labels.
   - Use numeric `ports.sources[].value_column` when one compact row represents
     multiple observations and the UI should size or count bullets by the sum.
   - Use at most one variable visual channel per link type, keyed by
     `variable.scale_key` and sourced from one raw numeric `value_column`.
   - Use `presentation.layout.strength` tokens `weakest`, `weaker`, `normal`,
     `stronger`, `strongest`, and `presentation.layout.distance` tokens
     `closest`, `closer`, `normal`, `farther`, `farthest`; do not emit numeric
     force values.

9. Define modal/table composition.
   - Put actor modal recipes in
     `types.actor_types.<id>.presentation.modal`.
   - Put link modal recipes in `types.link_types.<id>.presentation.modal`.
   - Put reusable table defaults in `types.table_types.<id>.presentation`.
   - Use `modal.labels.identification.fields[]` to choose the small set of
     actor labels that should appear in the actor modal identification/header
     area. The full `actor_labels` table remains the Labels tab.
   - Modal sections must select from existing `actors`, `links`, `evidence`,
     `actor_table`, or `relationship_table` sources.
   - Do not duplicate evidence or actor metadata only to populate a modal.
   - Use projections for display: direct column, actor-ref label, opposite
     actor, formatted endpoint, selected-side endpoint, label lookup,
     coalesce, const, or explicit scalar JSON path.
   - For `selected_side_endpoint`, include source/destination actor-ref
     columns and both endpoint sides in the projection so the UI can choose the
     side from the selected actor without hardcoded table knowledge.
   - For `label_lookup`, provide `label_key`; provide `actor_column` only when
     the lookup should read labels for an actor referenced by the source row
     instead of the selected modal actor.
   - For `json_path`, provide both the JSON `column` and scalar `path`.
   - Use cell types: text, number, badge, actor_link, timestamp, duration,
     endpoint, array_count, or debug_json.
   - Use visibility values: table, expanded, hidden, or debug.
   - Raw `json` is debug-only unless a schema-declared scalar projection gives
     the UI/aggregator semantics.
   - Treat Function `info` responses as metadata only. Validate full topology
     responses against `FUNCTION_TOPOLOGY_SCHEMA.json`; do not require
     metadata-only `info` responses to carry `data`.

10. Encode large sections as compact tables.
   - Use `const` for constant columns.
   - Use `dict` for low/medium-cardinality repeated values.
   - Use `values` only when values are high-cardinality.
   - Prefer dictionary references for strings.
   - For Go producers, use `src/go/pkg/topology/v1` compact-table helpers
     instead of hand-building table JSON.

11. Validate and measure.
   - Validate JSON with `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
   - Add semantic validation fixtures.
   - Measure raw and gzip size on realistic data.
   - Fail explicitly on size/row limits; never silently truncate.

## Direction Rules

- `directed` + `flow`: sockets, traffic, request dependencies.
- `directed` + `dependency`: logical dependency direction.
- `hierarchical` + `ownership`: parent/child, host/VM, cluster/host.
- `undirected` + `none`: physical adjacency with no direction.
- `observed_bidirectional` + `observation`: discovery saw one or both sides,
  but direction is not user-facing dependency.

If direction is noise, mark it so the aggregator can merge independently of
direction.

## Network-Connections Correlation Shape

Network-connections uses three graph-link families:

- node-to-process ownership links;
- local/resolved process-to-process socket links;
- process-to-endpoint or loose-side socket relationships for unresolved or
  cross-node remote socket endpoints.

Use distinct presentation for each family:

- `endpoint_socket`: solid, colored, thin, weakest, normal-distance unresolved
  endpoint dependency links;
- `correlated_socket`: solid, colored, thin, weakest, farthest aggregator
  output links after exact endpoint absorption;
- `socket`: gray, thin, stronger, farther local process links, optionally
  variable by `socket_count`;
- `ownership`: dotted, faded/dim, thin, normal, normal graph-coherence links.

In aggregated mode, do not enable process port bullets from detailed socket
evidence. Emit a compact actor inventory table such as `socket_ports` with
`actor`, `port`, and numeric `socket_count`, point the process actor
`ports.sources[]` at it with `value_column: "socket_count"`, and size process
actors with `size.mode: "metric"` over actor row `socket_count`.

For network-connections actor modals:

- self/node actors show a `Processes` section from `links` filtered to
  `type == ownership`;
- process actors show one primary section: aggregated mode uses
  `tables.relationship.connections`, detailed mode uses `evidence.socket`;
- endpoint actors show a `Processes` section over the same mode-specific
  relationship/evidence source;
- `socket_ports` stays an actor inventory for graph port bullets, not a normal
  modal tab;
- secondary socket metrics belong in `visibility: "expanded"` columns instead
  of separate duplicate sections.

For socket correlation:

- process actors emit claim rows for locally owned socket tuples;
- visible endpoint/correlation actors emit point rows when the producer
  materializes them;
- detailed evidence may also preserve loose remote tuple facts that the
  aggregator or UI can materialize according to schema-declared policy;
- the `socket_exact` rule uses `class: resolve_loose_side` and
  `action: absorb`;
- the key is declarative, typically protocol + address space + IP + port;
- `endpoint_socket` links are weak/normal-distance visible links before
  aggregation;
- `correlated_socket` is the farthest output link type after exact absorption.

## SNMP/L2 Modal Rules

For SNMP/L2 managed device actor modals:

- Treat the device as a collection of ports. The primary section is `Ports`
  over `actor_ports`.
- Put important device facts in `modal.labels.identification.fields[]`, backed
  by `actor_labels`. Typical keys are display name, management IP, vendor,
  model, port counts, and LLDP/CDP neighbor counts.
- Expose real port identity as typed `actor_ports` columns: `if_index`,
  source `port_id`, display `name`, `if_name`, `if_descr`, `if_alias`, MAC,
  speed, status, mode, role, VLAN, FDB, link, and neighbor counts.
- Do not fabricate numeric port IDs. Show `if_index` only when it is the real
  SNMP interface index.
- Use an actor-owned `actor_port_links` modal index for `Port Neighbors` when
  the device modal needs remote actor, remote port, link type, evidence count,
  confidence, inference, attachment mode, or timestamps.
- `actor_port_links` may carry compact side-specific refs and scalar facts, but
  must not duplicate raw LLDP/CDP/FDB/ARP/STP evidence JSON.
- Keep generic graph-link `Links` sections only for endpoint, segment, or
  custom actors that do not own port inventory.
- Build link endpoint port labels only from real port fields: `port_name`,
  `if_name`, `if_descr`, or source `port_id`. Never use actor labels such as
  `display_name` or `sys_name` as port-name fallbacks.

## Validation Checklist

- JSON validates against the topology schema.
- Semantic validation covers references, compact-table row counts, dictionaries,
  correlation rules, layout tokens, and schema-token parity.
- Actor identities are documented and tested.
- Link direction policy is documented and tested.
- Correlation points, claims, rules, priorities, actions, and output link types
  are documented and tested when cross-payload resolution applies.
- Evidence rows can reproduce required drilldown tables.
- Custom actor tables have correct roles and aggregation policy.
- Actor labels are emitted through `actor_labels` when the producer has labels
  or actor metadata to show.
- `actor_labels.key`, `actor_labels.value`, `actor_labels.source`, and
  `actor_labels.kind` are logical string fields. Accept `string` and
  `string_ref` encodings as equivalent when validating, aggregating, or
  rendering topology payloads.
- Treat `actor_labels` as sensitive topology Function data. Preserve the source
  Function's access-control assumptions when forwarding, aggregating, testing,
  or documenting labels.
- Modal sections are recipes over existing facts and do not duplicate
  high-cardinality evidence rows.
- Raw JSON columns are hidden/debug-only unless a schema-declared projection
  renders a scalar value.
- Payload size is measured on realistic or captured data.
- Raw sensitive captures remain under `.local/`.

Before considering `cloud-topology-service` ready, verify service-level
fixtures for all topology kinds covered by the schema. `network-connections` is
the required high-cardinality benchmark, but it is not enough by itself.

## vSphere Coordination

The vSphere topology producer lives in a separate PR worktree. Do not edit that
worktree before telling the user, because another agent may be working there.
