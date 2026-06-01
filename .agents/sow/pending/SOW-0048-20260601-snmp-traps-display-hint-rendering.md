# SOW-0048 - SNMP Traps Display-Hint Rendering

## Status

Status: open

Sub-state: created as a post-SOW-0039 follow-up; no implementation started.

## Requirements

### Purpose

Render trap varbind values with profile `display_hint` semantics when doing so materially improves operator readability without breaking raw journal queryability.

### User Request

Track SOW-0039 follow-up: `display_hint` is documented as reserved-future in the trap profile format.

### Acceptance Criteria

- Define supported display hints and fallback behavior.
- Extract `DISPLAY-HINT` from MIB TEXTUAL-CONVENTION definitions during profile generation when applicable.
- Render journal/display values consistently while preserving raw values for query and troubleshooting.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- Current trap profiles can reserve `display_hint`, but the runtime renderer does not apply it yet.

Evidence reviewed:

- SOW-0039 follow-up mapping.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.

Affected contracts and surfaces:

- Trap profile schema, profile generator, journal fields, docs, tests.

Existing patterns to reuse:

- Current varbind rendering and `TRAP_JSON` tests.
- SNMP profile authoring rules for MIB-derived metadata.

Risk and blast radius:

- Incorrect rendering could hide raw values or break searches. Raw values must remain available.

Sensitive data handling plan:

- Use public MIB fixtures and synthetic values only.

Implementation plan:

1. Define supported hint subset.
2. Update generator extraction and runtime rendering.
3. Add fixtures/tests for MAC, IPv4, and textual convention cases.

Validation plan:

- Generator tests, runtime serializer tests, and profile-format docs validation.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: update SNMP trap profile authoring skill if authoring rules change.
- Specs: update trap profile spec.
- End-user/operator docs: update profile-format docs.
- End-user/operator skills: update query docs only if field/query behavior changes.
- SOW lifecycle: close only after raw-value preservation is validated.

Open-source reference evidence:

- To be checked when implementation starts.

Open decisions:

- Supported display-hint subset and raw/display field policy.

## Followup

None yet.

## Regression Log

None yet.
