---
name: create-topology
description: Create or update Netdata topology producers and topology Function payloads using the production topology schema. Use when adding a topology map, migrating topology:network-connections / topology:streaming / topology:snmp / vSphere topology to the new schema, defining actor/link/evidence/table types, working on topology aggregation, topology drilldowns, direction semantics, or topology telemetry overlays.
---

# Create Netdata Topologies

Use this skill when creating or updating any Netdata topology producer,
topology Function, topology schema fixture, Cloud topology aggregator, or UI
decoder.

## Required References

Read these before designing payloads:

| File | Purpose |
|---|---|
| `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` | Human-readable topology schema contract and producer guidance |
| `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` | JSON Schema for production topology payloads |
| `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` | Backend/frontend/aggregator migration scope |
| `.agents/skills/project-writing-collectors/SKILL.md` | Collector quality, Function, validation, and cardinality rules |

For transport-level Function behavior, read:

- `src/plugins.d/FUNCTION_UI_REFERENCE.md`
- `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`

## Core Rule

Separate production payloads from compatibility tests.

- Production payloads carry canonical topology facts for the aggregator and UI.
- Test-only projection code may reconstruct compatibility payload shapes to
  prove parity.
- Never add compatibility reconstruction paths, adapter field names, or
  duplicated display strings to production payloads only to make compatibility
  tests easier.

## Workflow

1. Define the topology purpose and scale target.
   - What is the user's graph: nodes, processes, containers, L2 devices,
     vSphere inventory, streaming parents, or another domain?
   - What is the expected actor count, graph-link count, and evidence-row count?

2. Pick actors.
   - Choose stable identities.
   - Prefer durable IDs over display names.
   - Declare `identity`, `merge_identity`, and `parent_identity` in actor types.
   - Prepare aggregation scopes such as node, process name, container,
     Kubernetes workload, SNMP device/interface, or vSphere object.

3. Pick graph links.
   - Graph links are renderable relationship groups.
   - Keep graph links compact.
   - Put one-to-many observation details in evidence sections.
   - Define direction semantics in link types.

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

6. Define telemetry overlays.
   - Use overlay templates once per payload or type.
   - Links and actors carry compact refs and parameters only.
   - Do not put full metric query payloads on every row.

7. Define graph presentation.
   - Put actor presentation in `types.actor_types.<id>.presentation`.
   - Put link presentation in `types.link_types.<id>.presentation`.
   - Put graph port-bullet presentation in `types.port_types.<id>.presentation`.
   - Put legend, actor-click highlight behavior, port fields, and scale keys in
     `data.presentation`.
   - Use UI-owned color/icon/line/width/opacity tokens only.
   - Define `label_policy.columns` with safe scalar display columns; never let
     canonical identity arrays become actor names.
   - Define `ports.sources[]` whenever an actor type sets
     `ports.show_bullets: true`; port-bullet data must come from explicit link,
     evidence, or actor-table columns.
   - Use scalar display columns for `ports.sources[].name_column`; do not use
     actor/link/evidence refs, arrays, or JSON as graph bullet labels.
   - Use `selection.actor_click.mode: highlight_path` only with `path_table`,
     `path_actor_column`, and `path_order_column`; add `path_owner_column` when
     one table carries per-actor paths.
   - Use at most one variable visual channel per link type, keyed by
     `variable.scale_key` and sourced from one raw numeric `value_column`.

8. Encode large sections as compact tables.
   - Use `const` for constant columns.
   - Use `dict` for low/medium-cardinality repeated values.
   - Use `values` only when values are high-cardinality.
   - Prefer dictionary references for strings.
   - For Go producers, use `src/go/pkg/topology/v1` compact-table helpers
     instead of hand-building table JSON.

9. Validate and measure.
   - Validate against `FUNCTION_TOPOLOGY_SCHEMA.json`.
   - Add fixtures and semantic checks.
   - Measure raw and gzip size on realistic data.
   - Fail explicitly on size/row limits; never silently truncate.

## Direction Rules

Always define link direction semantics:

- `directed` + `flow`: sockets, traffic, request dependencies.
- `directed` + `dependency`: logical dependency direction.
- `hierarchical` + `ownership`: parent/child, host/VM, cluster/host.
- `undirected` + `none`: physical adjacency with no direction.
- `observed_bidirectional` + `observation`: discovery saw one or both sides,
  but direction is not user-facing dependency.

If direction is noise, mark it so the aggregator can merge independently of
direction.

## Payload Hygiene

Do not emit these per row:

- repeated display names or labels copied from actor/type metadata;
- repeated labels copied from actors;
- nested endpoint objects that repeat actor or evidence facts;
- `port_name` when the UI can derive it from port;
- implicit port-bullet sources when `ports.show_bullets` is true;
- modal table duplicates of relationship evidence;
- full metric queries when an overlay template plus params is enough.

Allowed once per type or column:

- small labels/descriptions for custom tables;
- semantic role metadata;
- aggregation policy;
- UI-owned presentation tokens and safe label policies;
- overlay template definitions.

Never emit raw SVG, raw CSS, coordinates, force-layout physics, viewport state,
or frontend component names. The UI owns rendering primitives; the backend
selects from the documented token vocabulary.

## Validation Checklist

Before considering a topology producer ready:

- JSON validates against the topology schema.
- Go producers use `src/go/pkg/topology/v1` and its tests cover compact-table
  row counts, encoding lengths, dictionary indexes, references, and JSON
  round trips.
- Actor type identities are documented and tested.
- Actor type presentation uses safe label policies and closed icon/color tokens.
- Actor port-bullet presentation has explicit sources and default port types.
- Link direction policy is documented and tested.
- Link type presentation defines direction arrows/curves only through closed
  tokens and scales at most one raw value per scale key.
- Evidence rows can reproduce required drilldown tables.
- Custom actor tables have correct roles and aggregation policy.
- Nested custom table cells, if any, use `json` columns and are not treated as
  generally aggregatable evidence.
- Overlay refs are compact and mergeable.
- Payload size is measured on realistic or captured data.
- Large payload behavior fails explicitly instead of truncating.
- Raw sensitive captures remain under `.local/`.

Before considering `cloud-topology-service` ready, verify service-level
fixtures for all topology kinds covered by the schema. `network-connections` is
the required high-cardinality benchmark, but it is not enough by itself.

## vSphere Coordination

The vSphere topology producer lives in a separate PR worktree. Do not edit that
worktree before telling the user, because another agent may be working there.
