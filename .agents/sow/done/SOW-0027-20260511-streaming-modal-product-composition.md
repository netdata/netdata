# SOW-0027 - Streaming Modal Product Composition

## Status

Status: completed

Sub-state: producer modal composition implemented and validated. This SOW is intentionally separate from network-connections and SNMP modal work.

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

Status: ready for implementation

Problem / root-cause model:

- The current streaming modal recipes expose local source tables but do not encode the operational roles of a selected actor.
- The retention table has the key `observer_actor` fact but the current modal recipe neither filters by it nor displays it, so parent responsibility is hidden.
- The inbound table already models descendants received through a parent; the current modal label `Inbound children` under-describes transit/descendant responsibility and makes the table look incomplete.

Evidence reviewed:

- Retention columns include `actor` and `observer_actor` in `src/web/api/functions/function-topology-streaming.c:1313-1323`.
- The modal retention section currently filters by `actor` in `src/web/api/functions/function-topology-streaming.c:1484-1495`.
- Inbound rows include `parent_actor`, `child_actor`, and `source_actor` in `src/web/api/functions/function-topology-streaming.c:1325-1340`.
- Outbound rows include only `actor` and `destination_actor` plus stream attributes in `src/web/api/functions/function-topology-streaming.c:1342-1349`.
- Descendant rows are populated into `parent_descendants` in `src/web/api/functions/function-topology-streaming.c:2546-2604` and then emitted as inbound rows in `src/web/api/functions/function-topology-streaming.c:1145-1192`.
- Actor labels already include display name, hostname, machine GUID, node ID, type, stream/ingest/health status, agent fields, system fields, child count, alert counts, and full host labels where available in `src/web/api/functions/function-topology-streaming.c:549-589`.

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

1. Keep one streaming modal recipe for all streaming actor types because the existing tables already use actor-ref owner filters and empty sections naturally disappear or show meaningful empty states per role.
2. Make the recipe role-aware through section labels and owner filters, not by duplicating table data.
3. Add or adjust modal sections for:
   - retention for selected node (`actor`);
   - retention maintained by selected node (`observer_actor`);
   - received/transit descendants (`parent_actor`);
   - upstream stream from selected node (`actor`).
4. Display `observer_actor` in the selected-node retention view and `actor` in the maintained-retention view.
5. Add important actor identification/header fields backed by `actor_labels`: health status and child count, while keeping full labels in the Labels tab.
6. Add missing columns/rows only if current canonical tables cannot answer the required operational questions.
7. Validate with local payload/schema checks and mark cloud/multi-parent validation as external if no aggregated streaming fixture is available in this repository.

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

- External open-source topology references were not used for implementation authority. This SOW changes Netdata-specific streaming semantics defined by local producer code and topology specs; local Netdata code is the authoritative source.

Open decisions:

- Resolved for this SOW: use one recipe with multiple well-labeled sections and owner filters. This avoids repeating modal metadata across actor types while preserving role-specific behavior through table filters.
- Resolved for this SOW: use concise operator-facing section names: `Stream path`, `Retention for node`, `Retained nodes`, `Received nodes`, and `Upstream stream`.

## Implications And Decisions

Decision recorded after the user asked to proceed to SOW-0027:

- Keep the payload single-source-of-truth: reuse existing `actor_labels`, `stream_path`, `retention`, `inbound`, and `outbound` tables.
- Do not create modal-only duplicate rows.
- Keep one shared streaming actor modal recipe unless implementation proves role-specific recipes are necessary.
- Rename/recompose sections so the modal answers operator questions directly:
  - `Retention for node`: who retains the selected node and for what range.
  - `Retained nodes`: which nodes' data the selected actor maintains.
  - `Received nodes`: which children/vnodes/stale descendants are received through the selected parent.
  - `Upstream stream`: where the selected actor itself sends data.

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
- Promoted SOW to current and recorded the implementation decision to keep one shared streaming actor modal recipe with role-aware sections over existing tables.
- Updated `topology:streaming` modal identification fields to include health status and child count.
- Split retention presentation into `Retention for node` and `Retained nodes` using the existing `retention` table and different owner filters.
- Renamed/recomposed relationship sections as `Received nodes` and `Upstream stream` so they match the underlying `inbound` and `outbound` table semantics.
- Updated topology specs and the project topology skill with streaming modal rules.

## Validation

Acceptance criteria evidence:

- Complete fact inventory recorded in `## Analysis` and `## Pre-Implementation Gate`.
- Actor role expectations recorded under `Target audience and questions`.
- `Retention for node` now filters `retention` rows by `actor` and displays `observer_actor`.
- `Retained nodes` now filters the same `retention` table by `observer_actor`, without duplicating retention rows.
- `Received nodes` now explains the existing `inbound` rows as children, virtual nodes, stale nodes, and descendants received through a parent.
- Actor modal identification now includes hostname, node type, stream status, ingest status, health status, child count, machine GUID, and Agent version.
- No missing producer rows were found for this SOW's modal fix. Aggregator preservation of multi-parent retention rows is already specified in `.agents/sow/specs/topology-modes-correlation-aggregation.md`.

Tests or equivalent validation:

- `git diff --check` passed.
- `(cd src/go && go test -count=1 ./pkg/topology/v1 ./tools/functions-validation/validate)` passed.
- `sudo -n cmake --build build --target netdata -- -j2` passed. The build emitted unrelated protobuf/stringop warnings during link, not streaming topology compile errors.
- `.agents/sow/audit.sh` passed with the pre-existing non-project skill classification warning.

Real-use evidence:

- Not performed against the live Agent in this SOW because the built binary was not installed/restarted. A live UI check before install would validate a different binary. The code path was validated by compiling the `netdata` target and by schema/fixture tests for the topology v1 contract.

Reviewer findings:

- No external reviewer run was requested for SOW-0027. The change is scoped to producer modal metadata and documentation/spec alignment.

Same-failure scan:

- `rg -n "Inbound children|Outbound stream|\"Retention\"|No inbound children|No outbound stream" src/web/api/functions .agents/sow/specs .agents/skills/project-create-topology/SKILL.md` found no remaining stale streaming modal labels.
- `rg -n "retention|observer_actor|retained_nodes|Received nodes|Upstream stream|child_count" ...` confirmed the new contract is present in the producer, specs, and project topology skill.

Sensitive data gate:

- This SOW uses only path/line evidence and synthetic descriptions. No raw sensitive payload data is included.

Artifact maintenance gate:

- AGENTS.md: unchanged. This SOW did not change project-wide workflow or guardrails.
- Runtime project skills: updated `.agents/skills/project-create-topology/SKILL.md` with streaming modal composition rules.
- Specs: updated `.agents/sow/specs/topology-function-schema.md` and `.agents/sow/specs/topology-modes-correlation-aggregation.md`.
- End-user/operator docs: unchanged. This is internal topology payload/modal composition, not a public user command or operator workflow.
- End-user/operator skills: unchanged. No public/operator skill behavior changed.
- SOW lifecycle: SOW promoted to current, completed, and will be moved to done with implementation in the same commit.

Specs update:

- Updated `.agents/sow/specs/topology-function-schema.md` with required streaming modal sections and identification labels.
- Updated `.agents/sow/specs/topology-modes-correlation-aggregation.md` with the streaming UI table semantics.

Project skills update:

- Updated `.agents/skills/project-create-topology/SKILL.md` with streaming modal rules for future topology producers.

End-user/operator docs update:

- Not needed. The Function remains sensitive/internal topology data; no public operator command, UI label documentation, or configuration guide changed.

End-user/operator skills update:

- Not needed. Public skills under `docs/netdata-ai/skills/` are for querying/operator workflows, not developer topology modal contracts.

Lessons:

- Streaming already had the canonical facts needed for the modal fix. The root problem was recipe composition and labels, not missing table data.

Follow-up mapping:

- No new follow-up SOW is required from this change. Live UI validation after installing the rebuilt Agent remains an execution check, not a new product requirement.

## Outcome

Implemented.

- `topology:streaming` actor modals now expose operator-relevant identity in the modal header.
- Retention is shown in both directions:
  - who maintains the selected node;
  - which nodes the selected actor maintains.
- Parent responsibility is clearer through `Received nodes`, backed by existing descendant rows.
- `Upstream stream` now clearly means the selected actor's own outbound stream.

## Lessons Extracted

- Reusing the same canonical table through different owner filters is the right pattern for modal composition when the relationship has two actor-ref sides.
- Section names must describe the selected actor's perspective; otherwise correct rows can still appear wrong to operators.

## Followup

None.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
