# SOW-0051 - SNMP Traps Listener Read Error Observability

## Status

Status: completed

Sub-state: metric, chart, health, docs, and unit-test coverage were already
implemented in earlier branch commits; user approved adding bounded log
visibility for operator troubleshooting. The bounded log path is implemented
and validated.

## Requirements

### Purpose

Make persistent UDP listener read errors visible to operators without flooding logs or adding avoidable hot-path cost.

### User Request

Track the close-gate reviewer finding that `listener.go` currently backs off and continues on UDP read errors without a log or metric.

### Acceptance Criteria

- Persistent listener read errors are observable through a bounded metric and a rate-limited log.
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
- Existing go.d logger pattern: `c.Limit(key, 1, window).Warningf(...)`.

Risk and blast radius:

- Low. Adding a metric dimension affects charts/metadata/health docs and must follow collector consistency.

Sensitive data handling plan:

- Do not log packet contents, source communities, or device identifiers from read-error paths.

Implementation plan:

1. Keep the existing `listener_read_failed` metric, chart, metadata, health,
   generated docs, and unit tests.
2. Add a listener read-error callback so socket code reports unexpected read
   failures without depending directly on the collector logger.
3. Add a collector-side rate-limited warning that logs operation, listener
   endpoint, and sanitized OS error, without packet contents or peer data.
4. Add tests proving unexpected read errors invoke the callback, clean shutdown
   remains quiet, and collector logging is rate-limited.

Validation plan:

- Unit test read-error path with an injected read error; run SNMP traps Go tests.
- No chart, metadata, health, or generated-doc changes are expected for the log
  addition because the public metric surface already exists.

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

- User chose metric plus rate-limited log on 2026-06-10. Rationale: operators
  troubleshooting traps expect trap-related errors to be findable in logs; the
  metric/health path shows persistence while the log carries the concrete OS
  read error.

## Followup

None yet.

## Regression Log

None yet.

## Validation

- `go test -count=1 ./plugin/go.d/collector/snmp_traps` passed from `src/go`.
- `git diff --check` passed.

## Outcome

- Unexpected UDP listener read errors now increment the existing
  `listener_read_failed` metric and emit a rate-limited warning with the
  listener endpoint and OS read error.
- Closed-listener shutdown remains quiet.
- The log path does not write packet contents, peer data, SNMP communities, or
  trap varbinds.
