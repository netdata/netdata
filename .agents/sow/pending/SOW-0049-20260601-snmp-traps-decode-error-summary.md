# SOW-0049 - SNMP Traps Decode Error Summary

## Status

Status: open

Sub-state: created as a post-SOW-0039 follow-up; no implementation started.

## Requirements

### Purpose

Decide and implement whether high-rate decode failures should produce batched summary journal entries analogous to deduplication summaries.

### User Request

Track SOW-0039 follow-up: `TRAP_REPORT_TYPE=decode_error_summary` is reserved, but not implemented in the first SNMP traps release.

### Acceptance Criteria

- Decide whether decode-error summary journal rows are worth the added complexity.
- If accepted, implement bounded summary aggregation and docs.
- If rejected, remove or document the reserved value clearly.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- Decode failures are visible as metrics, but not summarized as journal rows. The report type is reserved for a future operator-facing summary path.

Evidence reviewed:

- SOW-0039 follow-up mapping.
- SNMP traps serializer/report-type behavior.

Affected contracts and surfaces:

- Journal schema, health metrics, public query skill, docs, tests.

Existing patterns to reuse:

- Deduplication summary implementation and tests.

Risk and blast radius:

- Summary rows must not flood journals or confuse trap searches. They must omit device identity fields when not attributable.

Sensitive data handling plan:

- Use synthetic malformed packets and sanitized counts only.

Implementation plan:

1. Decide whether summary rows are needed.
2. Implement or reject with evidence.
3. Validate queryability and field semantics.

Validation plan:

- Unit tests for aggregation, bounded emission, journal fields, and public docs examples if implemented.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: no expected change.
- Specs: update journal schema/report type spec.
- End-user/operator docs: update trap docs if implemented.
- End-user/operator skills: update query-snmp-traps if implemented.
- SOW lifecycle: close with implemented or rejected outcome.

Open-source reference evidence:

- To be checked when implementation starts.

Open decisions:

- Whether decode-error summaries are required for first-class operator UX.

## Followup

None yet.

## Regression Log

None yet.
