# SOW-0029 - Network Connections Detailed Loose Sides

## Status

Status: open

Sub-state: pending follow-up split from SOW-0028.

## Requirements

### Purpose

Make detailed `topology:network-connections` preserve exact socket endpoints
without forcing every remote `IP:PORT` tuple to become a graph actor, while
keeping direct-Agent views readable and Cloud aggregation correlation-safe.

### User Request

The user asked to complete SOW-0028. The remaining true loose-side detailed
graph work was split here because it requires a separate Agent/UI/aggregator
execution pass.

### Assistant Understanding

Facts:

- SOW-0028 implemented the schema compatibility layer, modal identification,
  mode metadata, correlation rule classes, and relationship-summary rows.
- The current Agent producer still emits endpoint actors in detailed
  network-connections views.
- The SOW-0028 spec describes a stronger target: detailed rows may have one
  known actor side and one loose endpoint side, with exact tuple facts preserved
  for correlation and UI materialization.

Inferences:

- Implementing true loose sides is not a small closure task. It affects compact
  table shape, schema validation, UI materialization, and aggregator
  correlation execution together.
- Keeping it separate reduces risk of breaking current working direct-Agent
  topology views.

Unknowns:

- The exact compact schema shape for one-sided graph/evidence rows must be
  finalized against the existing `netdata.topology.v1` table model before code
  changes.

### Acceptance Criteria

- The schema/docs/specs define the exact compact representation for one-sided
  detailed network-connections rows and materialization policy.
- The Agent detailed network-connections producer emits known actors plus
  one-sided loose endpoint facts where appropriate, without actor-per-ephemeral
  `IP:PORT` graph explosion.
- The UI materializes loose sides only for direct-Agent detailed views and only
  according to producer-declared policy.
- The Cloud topology service resolves exact loose-side matches before returning
  aggregated output and keeps unresolved/partial cases visible without exposing
  aggregator-internal state.
- Installed Function checks and focused tests prove detailed and aggregated
  network-connections still render and correlate correctly.

## Analysis

Sources checked:

- `.agents/sow/specs/topology-modes-correlation-aggregation.md`
- `.agents/sow/done/SOW-0028-20260511-topology-mode-correlation-aggregation.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- Cloud frontend `TODO-topology-mode-correlation-aggregation.md`
- Cloud topology service SOW-0011

Current state:

- Detailed network-connections preserves exact socket evidence and correlation
  metadata.
- Detailed network-connections still uses endpoint actors in the renderable
  graph.
- UI loose-side materialization is documented in the frontend TODO but not yet
  implemented because producers do not emit one-sided detailed rows.

Risks:

- Actor-per-`IP:PORT` remains a graph cardinality risk if detailed views grow.
- A careless loose-side implementation can become a hidden frontend aggregator
  or hide unresolved dependencies.
- Changing detailed graph rows can break existing modal recipes if source
  columns and owner filters are not updated together.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The current v1 payload supports exact socket evidence, but graph links still
  require materialized endpoint actors for remote peers. The desired detailed
  model needs one-sided rows and a declared materialization policy, so exact
  evidence can be preserved without graph actor explosion.

Evidence reviewed:

- SOW-0028 installed checks show detailed network-connections returns socket
  evidence and correlation metadata, but still returns endpoint actors and
  normal graph links.
- `src/collectors/network-viewer.plugin/network-viewer.c` currently emits
  non-null `src_actor` and `dst_actor` columns for graph links and socket
  evidence table declarations.

Affected contracts and surfaces:

- Agent topology JSON schema, developer guide, and network-viewer producer.
- Cloud frontend v1 normalizer, graph model, actor modal source selection, and
  diagnostics.
- Cloud topology service correlation and aggregation core.
- Topology specs and project topology skill.

Existing patterns to reuse:

- Compact tables with nullable reference columns.
- Existing modal `selected_side_endpoint` projection.
- Existing correlation `resolve_loose_side` rule class.
- Existing UI diagnostics for unsupported metadata.

Risk and blast radius:

- High semantic risk for network dependency correctness.
- Medium UI risk because one-sided rows need deterministic render-only actors.
- Medium aggregator risk because exact match, partial match, and no-match cases
  must remain visible and truthful.

Sensitive data handling plan:

- Use synthetic socket fixtures and summaries only.
- Keep any live Function captures under `.local/`; do not commit raw process
  names, command lines, bearer tokens, private endpoints, raw node IDs, raw
  machine GUIDs, or customer-identifying public endpoints.

Implementation plan:

1. Finalize the schema shape and materialization policy with synthetic
   examples.
2. Update Agent producer and schema validation.
3. Update UI normalizer/materialization and modal behavior.
4. Update Cloud topology service correlation handling for one-sided rows.
5. Validate with synthetic tests and installed Function checks.

Validation plan:

- Agent schema validation and network-viewer build.
- Focused UI tests for direct-Agent detailed loose-side materialization.
- Cloud topology service tests for exact, partial, ambiguous, and no-match
  loose-side cases.
- Installed `topology:network-connections` detailed/aggregated checks.

Artifact impact plan:

- AGENTS.md: not expected unless workflow changes.
- Runtime project skills: update `project-create-topology` if the final
  materialization policy adds a recurring rule.
- Specs: update topology schema specs.
- End-user/operator docs: not expected.
- End-user/operator skills: not expected.
- SOW lifecycle: start only after SOW-0028 is completed and the user approves
  this follow-up priority.

Open-source reference evidence:

- Not checked. This is a Netdata topology schema contract, not an external
  protocol behavior.

Open decisions:

- Blocked until this SOW is selected as the next active topology task.

## Implications And Decisions

No user decision has been requested yet.

## Plan

1. Re-open the detailed network-connections schema section and decide the exact
   compact table representation.
2. Patch Agent, UI, and Cloud topology service in one coordinated pass.
3. Validate with installed Function output and focused tests.

## Execution Log

### 2026-05-11

- Created as a pending follow-up split from SOW-0028 closure.

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

- Pending.

Artifact maintenance gate:

- AGENTS.md: not changed.
- Runtime project skills: pending if materialization policy changes.
- Specs: pending.
- End-user/operator docs: not expected.
- End-user/operator skills: not expected.
- SOW lifecycle: pending/open.

Specs update:

- Pending.

Project skills update:

- Pending.

End-user/operator docs update:

- Not expected.

End-user/operator skills update:

- Not expected.

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
