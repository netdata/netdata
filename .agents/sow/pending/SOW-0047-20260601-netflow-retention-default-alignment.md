# SOW-0047 - NetFlow Retention Default Alignment

## Status

Status: open

Sub-state: created as a post-SOW-0039 follow-up; no implementation started.

## Requirements

### Purpose

Align NetFlow journal retention defaults with the SNMP traps retention decision if product owners agree that both forensic journal producers should default to size-only eviction.

### User Request

Track SOW-0039 follow-up: NetFlow currently defaults journal file duration to `7d`; SNMP traps intentionally defaults to no time-based age limit.

### Acceptance Criteria

- Decide whether NetFlow should change from `7d` duration default to `null` / size-only eviction.
- If accepted, update NetFlow defaults, docs, tests, and specs consistently.
- If rejected, record why NetFlow should intentionally differ from SNMP traps.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- SNMP traps use size-only default retention because trap forensic data should not age out by time alone. NetFlow currently has a different time-based default.

Evidence reviewed:

- SOW-0039 follow-up mapping.
- SNMP traps spec retention section.

Affected contracts and surfaces:

- NetFlow plugin config defaults, operator docs, packaging examples, retention behavior.

Existing patterns to reuse:

- NetFlow retention config defaults in `src/crates/netflow-plugin/src/plugin_config/defaults.rs`.
- SNMP traps retention semantics and docs.

Risk and blast radius:

- Behavioral change for NetFlow users; may increase disk use unless size caps are clear.

Sensitive data handling plan:

- No sensitive data expected. Use only config names, defaults, and sanitized evidence.

Implementation plan:

1. Inspect NetFlow retention defaults, docs, and tests.
2. Present product decision if the tradeoff is not already settled.
3. Implement or reject with evidence.

Validation plan:

- Rust tests for NetFlow config defaults plus docs/spec checks.

Artifact impact plan:

- AGENTS.md: no expected change.
- Runtime project skills: no expected change.
- Specs: update retention spec if behavior changes.
- End-user/operator docs: update NetFlow docs if behavior changes.
- End-user/operator skills: update only if public query/operator workflow changes.
- SOW lifecycle: close only with implemented/rejected decision.

Open-source reference evidence:

- None checked yet.

Open decisions:

- Whether NetFlow should match SNMP traps size-only default.

## Followup

None yet.

## Regression Log

None yet.
