# SOW-0022 - Topology table composition

## Status

Status: open

Sub-state: pending follow-up after SOW-0021 restores topology presentation profiles.

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

## Analysis

Sources checked:

- Pending.

Current state:

- Pending SOW-0021 inventory.

Risks:

- Uncurated modal tables can leak sensitive infrastructure details, overwhelm users, and make topology look unfinished.
- Over-modeling table UI can couple backend producers to frontend component internals.
- Under-modeling table UI forces the frontend to hardcode producer-specific modal logic.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- Modal/table composition is underspecified. Producers can preserve useful facts, but the UI lacks enough schema-level guidance to turn those facts into polished modal content without raw JSON fallback or producer-specific hardcoding.

Evidence reviewed:

- User-provided examples of raw JSON leaking into final UI. Raw examples are intentionally not copied into this durable artifact.

Affected contracts and surfaces:

- `netdata.topology.v1` table/detail schema.
- Cloud frontend actor/link modal renderer.
- Cloud topology aggregator table merge behavior.
- Backend topology producers.
- Developer guide, topology spec, and create-topology skill.

Existing patterns to reuse:

- SOW-0021 presentation profiles.
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

1. Wait for SOW-0021 presentation profile inventory and schema direction.
2. Inventory current modal/table behavior across producers and UI.
3. Define compact table-composition profiles.
4. Update schema/docs/skill/specs and backend producers.
5. Validate with sanitized fixtures.

Validation plan:

- Pending SOW-0021 output.

Artifact impact plan:

- AGENTS.md: no expected update unless workflow rules change.
- Runtime project skills: likely no update except collector/topology workflow references if needed.
- Specs: likely update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: likely update `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- End-user/operator skills: likely update `docs/netdata-ai/skills/create-topology/SKILL.md`.
- SOW lifecycle: remains pending until SOW-0021 is complete or explicitly paused.

Open-source reference evidence:

- Not checked yet.

Open decisions:

- Pending SOW-0021 output.

## Implications And Decisions

1. User decision: table and actor-modal composition is separate from topology presentation and should be handled as SOW-0022.

## Plan

1. Wait for SOW-0021.
2. Inventory modals and tables.
3. Define composition schema.
4. Implement and validate.

## Execution Log

### 2026-05-09

- Opened as pending follow-up from user direction.

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

- Raw user-provided examples are not copied into this SOW. This SOW uses sanitized summaries only.

Artifact maintenance gate:

- AGENTS.md: pending close review.
- Runtime project skills: pending close review.
- Specs: pending close review.
- End-user/operator docs: pending close review.
- End-user/operator skills: pending close review.
- SOW lifecycle: pending.

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
