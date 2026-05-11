# SOW-0027 - Streaming Modal Product Composition

## Status

Status: open

Sub-state: pending product analysis and implementation. This SOW is intentionally separate from network-connections and SNMP modal work.

## Requirements

### Purpose

Make `topology:streaming` actor modals useful for operators of Netdata parent/child/vnode streaming trees, including path, retention, inbound, outbound, stale, and transit relationships.

### User Request

The user reported that streaming modals show wrong or incomplete table information. Example: a parent with many children/vnodes shows a `Retention` tab with one row and no clear node maintaining retention. The user expects the modal model to reflect stale nodes, children, parents, grandparents, great-grandparents, and parent transit responsibilities correctly.

### Assistant Understanding

Facts:

- Current streaming actor modals use one generic modal recipe for all streaming actor types.
- Current modal sections are `Stream path`, `Retention`, `Inbound children`, and `Outbound stream`.
- Current retention rows have both `actor` and `observer_actor`.
- Current retention modal filters by `actor`, so selecting a parent shows retention for that parent, not retention maintained by that parent for other nodes.
- Current outbound modal filters by `actor`, so it shows the selected actor's own outbound stream row, not necessarily children/vnodes passing through that parent.

Inferences:

- The current table data has some right primitives but the modal recipes are not role-aware enough.
- A parent actor needs at least two retention views:
  - retention for this actor;
  - retention this actor maintains for other actors.
- A parent actor likely needs a transit/children view that includes children/vnodes that pass through it, not only the parent's own outbound stream.
- Some rows may be missing for full cloud aggregation semantics, especially if querying several parents where many parents maintain retention for the same child.

Unknowns:

- Whether the current streaming Function sees enough local state to emit retention rows for every node whose data is retained by a parent, or only for actors known in the current topology.
- Whether cloud aggregation will merge retention rows from multiple parents without losing `observer_actor`.
- Whether parent transit relationships are fully represented by existing `inbound` rows, existing graph links/evidence, or require a new table/column.

### Acceptance Criteria

- A complete inventory exists for streaming modal facts: actors, actor labels, links/evidence, `stream_path`, `retention`, `inbound`, and `outbound`.
- The SOW defines what each streaming actor role should show: self/local parent, parent, child, virtual node, stale node, and inferred path actors.
- Retention tables distinguish "retention for this node" from "retention maintained by this node for others".
- Inbound/outbound/transit tables show children and descendants passing through a parent where the data exists.
- Actor identification/header labels expose important node identity/status fields, not only the generic `Labels` tab.
- Missing data needed for correct modal semantics is identified as producer work, aggregator work, or frontend work.

## Analysis

Sources checked:

- `src/web/api/functions/function-topology-streaming.c:320` stream path row type.
- `src/web/api/functions/function-topology-streaming.c:337` retention row type.
- `src/web/api/functions/function-topology-streaming.c:349` inbound row type.
- `src/web/api/functions/function-topology-streaming.c:367` outbound row type.
- `src/web/api/functions/function-topology-streaming.c:1130` retention row construction.
- `src/web/api/functions/function-topology-streaming.c:1167` inbound row construction.
- `src/web/api/functions/function-topology-streaming.c:1203` outbound row construction.
- `src/web/api/functions/function-topology-streaming.c:1296` stream path columns.
- `src/web/api/functions/function-topology-streaming.c:1313` retention columns.
- `src/web/api/functions/function-topology-streaming.c:1325` inbound columns.
- `src/web/api/functions/function-topology-streaming.c:1342` outbound columns.
- `src/web/api/functions/function-topology-streaming.c:1443` current modal recipe.
- `.agents/sow/specs/topology-function-schema.md:390` streaming modal composition notes.

Current state:

- The current modal recipe is generic across streaming actor types.
- `Retention` section filters `retention` rows by `actor`, which answers "what retention exists for the selected node", not "what retention this selected parent maintains for other nodes".
- The `Retention` section does not display `observer_actor`, even though the table has that column.
- `Outbound stream` filters by `actor`, which answers "where this actor sends", not "which children/vnodes are sent through this parent".
- `Inbound children` filters by `parent_actor`, which is closer to the parent view but may still not cover all transit/descendant questions.

Available facts to inventory:

- Actor labels:
  - display name, hostname, machine GUID, node ID, type, severity, ephemerality, ingest status, stream status, ML status, agent/version, health status, OS/architecture/CPU, child/alert counts, host labels.
- Stream path rows:
  - selected actor, path actor, path index, hostname, host ID, node ID, claim ID, hops, since/first-time, capabilities/flags.
- Retention rows:
  - actor whose data is retained, observer actor that maintains the data, DB status, time range, duration, metrics, instances, contexts.
- Inbound rows:
  - parent actor, child actor, optional source actor, received type, ingest status, hops, collected metrics/instances/contexts, replication completion, ingest age, TLS, alert counts.
- Outbound rows:
  - actor, destination actor, stream status, hops, TLS, compression.
- Links/evidence:
  - directed streaming relationships with port name and collected/replication metrics.

Target audience and questions:

- Netdata operator looking at a child/vnode:
  - What is this node?
  - What is its path to cloud/parents?
  - Which parent receives it?
  - Who retains its data and over what time range?
  - Is it stale/virtual/healthy?
- Netdata operator looking at a parent:
  - Which children/vnodes does this parent receive directly?
  - Which descendants pass through this parent?
  - Which nodes' data does this parent retain?
  - Where does this parent send data upstream?
  - What is the replication status and health of each stream?
- Netdata operator looking at stale or virtual actors:
  - Why is this actor present?
  - When was it first/last observed?
  - Which path or retention state still references it?

Risks:

- A retention table that hides `observer_actor` is actively misleading in aggregated/cloud views where multiple parents may retain the same node.
- A parent modal that only shows its own outbound stream misses its operational responsibility for children passing through it.
- A generic recipe for all roles may be too simple; role-specific sections may be required.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The current streaming modal recipes expose local source tables but do not encode the operational roles of a selected actor.
- The retention table has the key `observer_actor` fact but the current modal recipe neither filters by it nor displays it, so parent responsibility is hidden.

Evidence reviewed:

- Retention columns include `actor` and `observer_actor` in `src/web/api/functions/function-topology-streaming.c:1313-1323`.
- The modal retention section currently filters by `actor` in `src/web/api/functions/function-topology-streaming.c:1484-1495`.
- Inbound rows include `parent_actor`, `child_actor`, and `source_actor` in `src/web/api/functions/function-topology-streaming.c:1325-1340`.
- Outbound rows include only `actor` and `destination_actor` plus stream attributes in `src/web/api/functions/function-topology-streaming.c:1342-1349`.

Affected contracts and surfaces:

- Agent Function payload for `topology:streaming`.
- Streaming C topology Function row construction and modal recipes.
- Cloud aggregator retention/actor-table merge behavior.
- Cloud frontend actor modal rendering and identity/header display.
- Developer guide, topology spec, project topology skill.

Existing patterns to reuse:

- Actor-owned `stream_path`, `retention`, `inbound`, and `outbound` tables.
- `actor_ref_label` and `label_lookup` projections.
- Separate table sections filtered by different actor-ref columns.
- Actor labels for identity/status facts.

Risk and blast radius:

- User-facing streaming modal behavior changes.
- Aggregation semantics are important because a cloud topology can contain many parents reporting retention for the same node.
- Sensitive data risk includes host labels, node IDs, claim IDs, machine GUIDs, and private hostnames; durable artifacts must use synthetic examples only.

Sensitive data handling plan:

- Do not copy raw host labels, machine GUIDs, claim IDs, hostnames, customer names, private endpoints, or production topology payloads into durable artifacts.
- Store real payload captures only under `.local/`.
- Use synthetic parent/child/vnode examples in docs/tests.

Implementation plan:

1. Inventory current streaming rows and old modal behavior by actor role.
2. Define role-specific modal views:
   - child/vnode view;
   - parent view;
   - stale view;
   - self/local node view if different.
3. Add modal sections for:
   - retention for selected node (`actor`);
   - retention maintained by selected node (`observer_actor`);
   - inbound direct children (`parent_actor`);
   - transit/descendant rows if current data supports them;
   - outbound stream from selected node (`actor`).
4. Add missing columns/rows only if current canonical tables cannot answer the required operational questions.
5. Update aggregator and frontend handoffs if section filtering or role-specific actor modals need support beyond current schema.
6. Validate with local and multi-parent/cloud payloads where possible.

Validation plan:

- C syntax check for `function-topology-streaming.c`.
- Schema validation of generated streaming payloads.
- Local Function call on a parent with children/vnodes.
- Verify modal rows for:
  - selected child/vnode;
  - selected parent;
  - selected stale node if present.
- Aggregated/cloud payload check that retention rows preserve both retained actor and observer actor.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md` if streaming modal guidance changes.
- Specs: update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: likely unaffected unless Function examples are changed.
- End-user/operator skills: unaffected.
- SOW lifecycle: close only after local and, if available, cloud/aggregated streaming validation.

Open-source reference evidence:

- Not checked yet. This is Netdata-specific streaming semantics; external references are unlikely to help except for general tree/path modal UX.

Open decisions:

- Decide whether streaming actor types need distinct modal recipes by role, or whether one recipe with multiple well-labeled sections is enough.
- Decide what section names make the operational meaning clear, especially for retention and transit/forwarded children.

## Implications And Decisions

Pending. User decision is required after the field inventory and proposed modal layout are written.

## Plan

1. Inventory old/current streaming modal fields and role-specific facts.
2. Design role-aware streaming actor modals.
3. Identify missing canonical streaming rows/columns.
4. Implement only streaming producer changes after design acceptance.
5. Coordinate frontend/aggregator changes if required.
6. Validate locally and with aggregated/cloud payloads when available.

## Execution Log

### 2026-05-11

- Created SOW from user-reported streaming modal regressions and current code evidence.

## Validation

Acceptance criteria evidence:

- Pending.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending.

Reviewer findings:

- Pending.

Same-failure scan:

- Pending.

Sensitive data gate:

- This SOW uses only path/line evidence and synthetic descriptions. No raw sensitive payload data is included.

Artifact maintenance gate:

- Pending.

Specs update:

- Pending.

Project skills update:

- Pending.

End-user/operator docs update:

- Pending.

End-user/operator skills update:

- Pending.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
