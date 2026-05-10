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

6. Define telemetry overlays.
   - Use overlay templates once per payload or type.
   - Links and actors carry compact refs and parameters only.
   - Do not put full metric query payloads on every row.

7. Define correlation semantics when actors can be resolved across payloads.
   - Emit pure correlation actors for unresolved peers; do not hide correlation
     state as flags on real actors.
   - Define `data.correlation.rules` with declarative key templates,
     priorities, `absorb` or `link` actions, point actor types, optional claim
     actor types, correlation link types, and output link types.
   - Emit compact `data.correlation.points` rows for correlation actors and
     `data.correlation.claims` rows for real actors that can satisfy keys.
   - Use `absorb` only for exact matches that should remove correlation actors
     from the aggregated output.
   - Use `link` for broader or partial matches that should keep the correlation
     actor visible.
   - Keep NAT or alias information as additional point/claim rows, not as
     mutation of the original observation.

8. Define graph presentation.
   - Put actor presentation in `types.actor_types.<id>.presentation`.
   - Put link presentation in `types.link_types.<id>.presentation`.
   - Put graph port-bullet presentation in `types.port_types.<id>.presentation`.
   - Put legend, actor-click highlight behavior, port fields, and scale keys in
     `data.presentation`.
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

9. Encode large sections as compact tables.
   - Use `const` for constant columns.
   - Use `dict` for low/medium-cardinality repeated values.
   - Use `values` only when values are high-cardinality.
   - Prefer dictionary references for strings.
   - For Go producers, use `src/go/pkg/topology/v1` compact-table helpers
     instead of hand-building table JSON.

10. Validate and measure.
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
- process-to-correlation-endpoint links for unresolved or cross-node remote
  socket endpoints.

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

For socket correlation:

- process actors emit claim rows for locally owned socket tuples;
- endpoint actors emit point rows for remote tuples;
- the `socket_exact` rule uses `action: absorb`;
- the key is declarative, typically protocol + address space + IP + port;
- `endpoint_socket` links are weak/normal-distance visible links before
  aggregation;
- `correlated_socket` is the farthest output link type after exact absorption.

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
- Payload size is measured on realistic or captured data.
- Raw sensitive captures remain under `.local/`.

Before considering `cloud-topology-service` ready, verify service-level
fixtures for all topology kinds covered by the schema. `network-connections` is
the required high-cardinality benchmark, but it is not enough by itself.

## vSphere Coordination

The vSphere topology producer lives in a separate PR worktree. Do not edit that
worktree before telling the user, because another agent may be working there.
