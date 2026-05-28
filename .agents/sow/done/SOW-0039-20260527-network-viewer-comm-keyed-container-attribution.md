# SOW-0039 - network-viewer comm-keyed container attribution uplift

## Status

Status: closed

Sub-state: closed as obsolete by SOW-0036 regression repair on 2026-05-28.

## Requirements

### Purpose

Improve `topology:network-connections` container attribution in the default
`processes:by_name` view so same-name processes running in different containers
do not inherit only the first observed PID's container metadata.

Closure note:

- This purpose is no longer valid. The corrected SOW-0036 contract says
  `group_by:process_name` must not emit container metadata because the process
  actor can represent multiple processes across containers or nodes.
- Container identity is now exposed by `group_by:container`, not by enriching
  the grouped process-name actor.

### User Request

Track the SOW-0036 known limitation as real future work instead of leaving it as
untracked deferred work.

### Assistant Understanding

Facts:

- SOW-0036 enriches comm-keyed process actors from the first PID retained on
  `NV_PROCESS_ACTOR`.
- `processes:by_pid` is accurate because each actor carries one PID.
- The default view can misattribute container columns when multiple PIDs share a
  process name across containers.

Inferences:

- The likely implementation must retain more than one PID per comm-keyed actor
  or split comm-keyed actors by container affiliation.

Unknowns:

- The product behavior for same-comm, multi-container actors needs a fresh
  design decision before implementation.

### Acceptance Criteria

- Default `processes:by_name` topology output no longer silently uses only the
  first PID's container metadata when multiple container affiliations exist.
- `processes:by_pid` behavior remains unchanged.
- Tests cover same-process-name PIDs in at least two containers.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0035-20260526-network-viewer-apps-lookup-client.md`
- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`

Current state:

- The limitation is documented for operators in SOW-0036 outputs.

Risks:

- Changing the actor key model can affect link aggregation, modal tables, and
  search semantics.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The actor dictionary collapses multiple PIDs into one process-name actor, but
  container metadata is per-PID.

Evidence reviewed:

- SOW-0036 follow-up item 1.

Affected contracts and surfaces:

- `topology:network-connections` actor identity, merge identity, links,
  modal tables, and grouping metadata.

Existing patterns to reuse:

- Existing `processes:by_pid` actor path and SOW-0036 enrichment helpers.

Risk and blast radius:

- Medium. The work touches actor identity and relationship table references.

Sensitive data handling plan:

- Use synthetic process/container names only in fixtures and durable evidence.

Implementation plan:

1. Re-open design with concrete options for multi-PID comm-keyed actors.
2. Implement the selected model with tests and docs.

Validation plan:

- Unit/fixture tests for same-comm PIDs in distinct containers.
- Schema validation for topology payloads.

Artifact impact plan:

- AGENTS.md: update only if workflow guardrails change.
- Runtime project skills: update topology skill if a new actor model pattern is introduced.
- Specs: update topology Function schema spec if identity semantics change.
- End-user/operator docs: update network-viewer topology docs if behavior changes.
- End-user/operator skills: update query topology guides if output interpretation changes.
- SOW lifecycle: move to current before implementation and close with validation evidence.

Open-source reference evidence:

- Not checked yet; this SOW is only a tracker until activated.

Open decisions:

- Choose whether to split same-comm actors by container, aggregate container
  metadata as sets, or keep one actor with representative metadata plus warnings.

## Implications And Decisions

Pending activation.

## Plan

1. Analyze actor-key options with code evidence.
2. Present the product decision.
3. Implement and validate the selected behavior.

## Execution Log

No work yet.

## Validation

Pending activation.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
