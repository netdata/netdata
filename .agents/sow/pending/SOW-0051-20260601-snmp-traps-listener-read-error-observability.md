# SOW-0051 - SNMP Traps Listener Read Error Observability

## Status

Status: open

Sub-state: created from the 2026-06-01 SOW-0039 close-gate review; no implementation started.

## Requirements

### Purpose

Make persistent UDP listener read errors visible to operators without flooding logs or adding avoidable hot-path cost.

### User Request

Track the close-gate reviewer finding that `listener.go` currently backs off and continues on UDP read errors without a log or metric.

### Acceptance Criteria

- Persistent listener read errors are observable through a bounded metric and/or rate-limited log.
- Closed-listener shutdown remains quiet.
- Hot-path behavior for normal trap receipt remains unchanged.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- `Listener.readLoop` handles `ReadFromUDP` errors by sleeping and continuing. This avoids noisy logs, but a persistent socket/kernel error can make trap receipt fail silently.

Evidence reviewed:

- SOW-0039 final `glm` review finding.
- `src/go/plugin/go.d/collector/snmp_traps/listener.go`.

Affected contracts and surfaces:

- SNMP trap error metrics, health alerts if a new dimension is added, logs, tests, docs if operator-visible behavior changes.

Existing patterns to reuse:

- Existing `snmp.trap.errors` chart dimensions and rate-limited logging discipline from collector guidance.

Risk and blast radius:

- Low. Adding a metric dimension affects charts/metadata/health docs and must follow collector consistency.

Sensitive data handling plan:

- Do not log packet contents, source communities, or device identifiers from read-error paths.

Implementation plan:

1. Decide metric-only vs metric plus rate-limited log.
2. Add bounded counter and tests.
3. Update charts, metadata, health/docs if a public metric changes.

Validation plan:

- Unit test read-error path with a fake listener or injected read error; run SNMP traps Go tests and collector consistency checks if charts/docs change.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: no expected change.
- Specs: update SNMP traps error metric spec if a dimension is added.
- End-user/operator docs: update generated docs/metadata if a public metric changes.
- End-user/operator skills: update only if query workflow changes.
- SOW lifecycle: close with implementation and validation.

Open-source reference evidence:

- None needed; this is local observability hardening.

Open decisions:

- Metric-only or metric plus rate-limited log.

## Followup

None yet.

## Regression Log

None yet.
