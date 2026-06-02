# SOW-0042 - network-viewer docker-images hide argument

## Status

Status: closed

Sub-state: closed as obsolete by SOW-0036 regression repair on 2026-05-28.

## Requirements

### Purpose

Add an operator-controlled `docker-images:<show|hide>` Function argument for
`topology:network-connections` so the `docker_image` process actor column can be
suppressed in stricter multi-tenant environments.

Closure note:

- This purpose is no longer valid as a direct follow-up to SOW-0036. The
  corrected contract removes the `cgroup-paths:<show|hide>` pattern and avoids
  adding per-field hide/show controls for grouped topology actors.
- Raw container metadata, including `docker_image`, is now limited to
  `group_by:pid`. Any future privacy policy for PID-mode raw fields needs a
  fresh product decision, not a mirrored hide knob.

### User Request

Track the `docker_image` sensitivity mitigation from SOW-0036 as real work.

### Assistant Understanding

Facts:

- SOW-0036 emits `docker_image` as a nullable process actor attribute when the
  APPS_LOOKUP cache has an `image` label.
- SOW-0036 regression repair removed the `cgroup-paths:hide` pattern.
- Image registry hostnames and repository paths can contain tenant-identifying
  strings in some environments.

Inferences:

- No implementation should mirror `cgroup-paths:<show|hide>`; that pattern is
  removed.

Unknowns:

- Product demand and default behavior should be re-confirmed before activation.

### Acceptance Criteria

- `docker-images:hide` emits JSON `null` for `docker_image`.
- Default behavior remains `docker-images:show`.
- Metadata and operator docs list the new argument.
- Tests cover show and hide behavior.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`

Current state:

- `docker_image` emits when present; no suppression argument exists.

Risks:

- Hiding `docker_image` can reduce troubleshooting context. Default show keeps
  current SOW-0036 behavior.

## Pre-Implementation Gate

Historical status at implementation start: blocked.

Problem / root-cause model:

- `docker_image` is useful but can carry sensitive registry or repository text.

Evidence reviewed:

- SOW-0036 Risk Register and follow-up item 4.

Affected contracts and surfaces:

- `topology:network-connections` Function arguments, metadata, docs, public
  skills, and actor table values.

Existing patterns to reuse:

- None. The prior `cgroup-paths:<show|hide>` parsing and column suppression
  pattern was removed by SOW-0036 regression repair.

Risk and blast radius:

- Low to medium. This is a narrow visibility knob over one nullable attribute.

Sensitive data handling plan:

- Tests use synthetic registry names under reserved/example domains.

Implementation plan:

1. Add option parsing and defaults.
2. Gate `docker_image` population.
3. Update metadata, docs, skills, and tests.

Validation plan:

- Focused unit or fixture tests for default show and explicit hide.
- Topology schema validation.

Artifact impact plan:

- AGENTS.md: no expected change unless workflow guardrails change.
- Runtime project skills: update topology skill if argument pattern is generalized.
- Specs: update topology Function schema spec.
- End-user/operator docs: update network-viewer docs.
- End-user/operator skills: update query topology guides and how-tos.
- SOW lifecycle: move to current before implementation and close with validation evidence.

Open-source reference evidence:

- Not checked yet; this SOW is only a tracker until activated.

Open decisions:

- Confirm default remains `show`.

## Implications And Decisions

Pending activation.

## Plan

1. Re-confirm product default.
2. Implement the visibility knob.
3. Validate and update docs.

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
