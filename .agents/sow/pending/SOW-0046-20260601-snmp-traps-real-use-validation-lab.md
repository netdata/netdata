# SOW-0046 - SNMP Traps Real-Use Validation Lab

## Status

Status: open

Sub-state: created as a post-SOW-0039 follow-up; no implementation started.

## Requirements

### Purpose

Prove the SNMP traps implementation against real or lab-equivalent runtime paths that need external environment state and were not practical to complete inside the SOW-0039 merge gate.

### User Request

Track the remaining SNMP traps real-use validation gaps after the SOW-0039 close gate.

### Assistant Understanding

Facts:

- SOW-0036 covered SNMPv3 INFORM semantics with unit tests, including authPriv response generation and local engine ID persistence.
- SOW-0037 covered enrichment priority with focused unit tests.
- SOW-0039 installed-Agent validation covered SNMPv2c trap receipt on UDP/162, journal persistence, `journalctl --directory`, and metrics.

Inferences:

- The remaining validation needs either a real device, a protocol simulator, a signed-in Logs UI session, or a co-located SNMP polling/topology setup.

Unknowns:

- Which lab device/simulator and signed-in validation path will be available when this SOW starts.

### Acceptance Criteria

- Validate SNMPv3 INFORM persistence and response behavior across plugin restart with a real device or protocol simulator.
- Validate SNMP trap facets in the signed-in Logs UI or through an equivalent authorized Agent/Cloud Function path.
- Validate co-located SNMP collector/topology enrichment on a live or lab node.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- Current automated tests cover the protocol and enrichment logic, but external integration paths need environment state not owned by the unit-test harness.

Evidence reviewed:

- SOW-0036 real-use evidence section.
- SOW-0037 real-use evidence section.
- SOW-0039 installed-Agent validation and final reviewer findings.

Affected contracts and surfaces:

- SNMPv3 INFORM behavior, journal entries, Logs UI facets, SNMP/topology enrichment, operator validation docs.

Existing patterns to reuse:

- SNMP traps installed-Agent validation from SOW-0039.
- Existing `query-snmp-traps` public skill examples.

Risk and blast radius:

- Validation-only SOW; production code changes should be limited to fixes for issues found during validation.

Sensitive data handling plan:

- Use lab devices or redacted outputs. Do not write raw communities, USM keys, device identifiers, live public IPs, or customer-identifying data to durable artifacts.

Implementation plan:

1. Select lab/simulator and authorized Logs UI path.
2. Run real-use validation and record sanitized evidence.
3. Fix any discovered issue under this SOW or create a narrower follow-up if needed.

Validation plan:

- Real-use commands/API/UI evidence plus focused regression tests if issues are fixed.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: no expected change unless validation reveals a reusable developer workflow.
- Specs: update SNMP traps spec if real-use behavior differs from current contract.
- End-user/operator docs: update if validation changes documented operator workflow.
- End-user/operator skills: update `query-snmp-traps` if query workflow changes.
- SOW lifecycle: keep open until validation evidence is recorded or explicitly superseded.

Open-source reference evidence:

- None needed yet; this is lab validation of already implemented behavior.

Open decisions:

- Lab device/simulator and signed-in validation path.

## Followup

None yet.

## Regression Log

None yet.
