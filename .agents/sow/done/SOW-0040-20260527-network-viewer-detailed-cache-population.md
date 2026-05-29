# SOW-0040 - network-viewer detailed-view cache population

## Status

Status: closed

Sub-state: superseded by SOW-0044, which implements detailed-view PID warming
as part of the broader topology enrichment consistency repair.

## Requirements

### Purpose

Populate the APPS_LOOKUP warm-cache path for PIDs that appear only in the
tabular `network-connections` Function's `sockets:detailed` mode.

### User Request

Track the detailed-view cache-population gap as real work instead of leaving it
inside the completed topology grouping SOW. This narrow tracker is now closed
because SOW-0044 absorbed and implements the work.

### Assistant Understanding

Facts:

- SOW-0035 warms APPS_LOOKUP for topology and tabular paths where PIDs are
  available before output emission.
- The detailed tabular path emits rows inline during local socket enumeration.
- Some detailed-only PIDs may not enter the warm-cache set before output.

Inferences:

- The fix likely needs either a small side-channel from the detailed callback or
  a two-pass detailed output model.

Unknowns:

- The least invasive implementation path needs fresh code analysis when this
  SOW is activated.

### Acceptance Criteria

- PIDs visible only in `sockets:detailed` are submitted to APPS_LOOKUP warming.
- Existing detailed tabular output remains byte-compatible except for intended
  timing/cache side effects.
- Tests cover detailed-only PIDs.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0035-20260526-network-viewer-apps-lookup-client.md`
- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`

Current state:

- The gap is tracked but not implemented.

Risks:

- Restructuring detailed output can affect memory use and Function latency.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The detailed callback emits rows while local socket data is still streaming,
  which leaves no stable PID set for APPS_LOOKUP warming after enumeration.

Evidence reviewed:

- SOW-0036 follow-up item 2 and SOW-0035 follow-up mapping.

Affected contracts and surfaces:

- `network-connections` tabular Function, APPS_LOOKUP warm-cache behavior, and
  network-viewer Function latency.

Existing patterns to reuse:

- SOW-0035 warm-PID batching and SOW-0036 cache accessor discipline.

Risk and blast radius:

- Medium. The tabular Function is operator-facing and performance-sensitive.

Sensitive data handling plan:

- Use synthetic PIDs and process names in tests; do not store raw socket rows
  from real systems in durable artifacts.

Implementation plan:

1. Re-read the detailed callback and local-sockets cleanup path.
2. Choose side-channel PID collection or two-pass detailed output.
3. Implement with focused tests.

Validation plan:

- Unit or integration test proving detailed-only PIDs are warmed.
- Existing network-viewer tests and schema checks.

Artifact impact plan:

- AGENTS.md: update only if workflow guardrails change.
- Runtime project skills: update collector/topology skills if a new pattern is introduced.
- Specs: update if tabular Function behavior changes.
- End-user/operator docs: update only if visible behavior changes.
- End-user/operator skills: update only if operator workflow changes.
- SOW lifecycle: move to current before implementation and close with validation evidence.

Open-source reference evidence:

- Not checked yet; this SOW is only a tracker until activated.

Open decisions:

- Side-channel PID collection versus two-pass detailed output.

## Implications And Decisions

Pending activation.

## Plan

1. Analyze the detailed output path.
2. Present the implementation decision if there are multiple safe options.
3. Implement and validate.

## Execution Log

2026-05-28: closed as superseded by
`.agents/sow/current/SOW-0044-20260528-topology-enrichment-consistency.md`.

## Validation

Closed without separate implementation. SOW-0044 owns validation.

## Outcome

Superseded by SOW-0044.

## Lessons Extracted

No separate lessons; see SOW-0044.

## Followup

None yet.

## Regression Log

None yet.
