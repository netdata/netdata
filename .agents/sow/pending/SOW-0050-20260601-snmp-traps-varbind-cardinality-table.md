# SOW-0050 - SNMP Traps Varbind Cardinality Table

## Status

Status: open

Sub-state: created as a post-SOW-0039 follow-up; no implementation started.

## Requirements

### Purpose

Define an authoritative bounded/unbounded cardinality table for trap varbinds used by `dimension_from_varbind` so operator metrics stay safe and predictable.

### User Request

Track SOW-0039 follow-up: SOW-0037 rejects unbounded-cardinality varbinds at config load, but the reference table needs a durable source.

### Acceptance Criteria

- Define durable source of truth for varbind cardinality classification.
- Validate `dimension_from_varbind` against that source.
- Document authoring and operator behavior.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- Operator metrics can explode if an unbounded varbind is used as a metric dimension. Current validation exists, but the durable classification source should be explicit.

Evidence reviewed:

- SOW-0037 M4 operator metrics validation.
- SOW-0039 follow-up mapping.

Affected contracts and surfaces:

- Trap profile schema/generator, runtime config validation, docs, project authoring skill.

Existing patterns to reuse:

- Project SNMP trap profile authoring skill cardinality discipline.
- Existing profile validation paths.

Risk and blast radius:

- Incorrect classification can either block valid useful metrics or allow high-cardinality series.

Sensitive data handling plan:

- Use public MIB/profile metadata only.

Implementation plan:

1. Decide whether the table lives in profile schema, generated metadata, or separate YAML.
2. Implement validation against the chosen source.
3. Update docs and profile authoring guidance.

Validation plan:

- Unit tests for accepted/rejected varbind classifications and generated profile metadata.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: update SNMP trap profile authoring skill.
- Specs: update trap profile/operator metric spec.
- End-user/operator docs: update profile-format/config docs.
- End-user/operator skills: update only if query/operator workflow changes.
- SOW lifecycle: close after validation and docs.

Open-source reference evidence:

- To be checked when implementation starts.

Open decisions:

- Durable source location and classification taxonomy.

## Followup

None yet.

## Regression Log

None yet.
