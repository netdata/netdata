# SOW-0041 - Cloud topology group_by consumer

## Status

Status: open

Sub-state: tracked from SOW-0036 follow-up item 3. Not started.

## Requirements

### Purpose

Implement the Cloud-side consumer work needed for the
`topology:network-connections` `view.group_by` metadata emitted by the Agent to
become an operator-visible grouping control.

### User Request

Track the Cloud consumer half separately from the Agent producer work so the
Agent-side SOW can close without claiming Cloud UI grouping is already done.

### Assistant Understanding

Facts:

- SOW-0036 regression repair emits the Agent payload with three actor grouping
  choices: `process_name`, `pid`, and `container`.
- `group_by:pid` returns per-PID `process` actors with raw per-PID fields.
- `group_by:process_name` returns grouped `process` actors without variable
  per-process/container fields.
- `group_by:container` returns grouped `container` actors keyed by canonical
  `container_name`.
- SOW-0036 explicitly does not implement Cloud frontend or Cloud topology
  service selection UX.

Inferences:

- This SOW will likely touch Cloud repositories outside this Agent checkout.

Unknowns:

- Exact Cloud repo branch, ownership, and delivery sequencing must be confirmed
  before implementation.

### Acceptance Criteria

- Cloud frontend exposes a grouping selector for topology payloads that declare
  `view.group_by`.
- Cloud topology aggregation honors the selected actor grouping.
- Cloud grouping respects the Agent actor type and merge identity emitted for
  the selected mode.
- Agent payloads emitted by SOW-0036 require no further Agent-side changes.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`

Current state:

- Agent producer work is implemented in this repository; Cloud consumer work is
  tracked separately.

Risks:

- Cross-repository sequencing and API contract drift can produce UI controls
  that do not match Agent payload semantics.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The Agent emits grouping metadata, but Cloud consumers must turn that metadata
  into rebucketing behavior and UI controls.

Evidence reviewed:

- SOW-0036 Cloud-side consumer follow-up.

Affected contracts and surfaces:

- Cloud frontend topology adapter, Cloud topology aggregation service, and
  topology Function query flow.

Existing patterns to reuse:

- Existing topology v1 normalization and aggregation paths in the Cloud repos.

Risk and blast radius:

- High. This is cross-repository user-visible Cloud behavior.

Sensitive data handling plan:

- Use synthetic topology payloads and redacted Cloud test data only.

Implementation plan:

1. Confirm Cloud repositories, owners, and target branch.
2. Audit current Cloud normalization and aggregation behavior.
3. Implement group_by selection and sparse-scope identity preservation.

Validation plan:

- Synthetic SOW-0036 payloads through frontend and aggregation tests.
- Manual Cloud UI verification in a non-production environment.

Artifact impact plan:

- AGENTS.md: update only if cross-repo workflow guardrails change.
- Runtime project skills: update topology skill if Cloud consumer workflow is documented here.
- Specs: update if topology consumer semantics become part of Agent-side spec.
- End-user/operator docs: update Cloud topology operator docs.
- End-user/operator skills: update query topology skills if workflow changes.
- SOW lifecycle: move to current only after repository and ownership are confirmed.

Open-source reference evidence:

- Not checked yet; this SOW is only a tracker until activated.

Open decisions:

- Confirm whether Cloud rebucketing happens server-side, client-side, or both.

## Implications And Decisions

Pending activation.

## Plan

1. Confirm Cloud-side implementation location.
2. Audit and design grouping flow.
3. Implement, test, and document.

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
