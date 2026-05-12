# SOW-0018 - macOS S.M.A.R.T. and NVMe storage health

## Status

Status: open

Sub-state: split from SOW-0016 after user decisions on 2026-05-13; pending activation. Implementation has not started.

## Requirements

### Purpose

Add macOS S.M.A.R.T. and NVMe health monitoring while reusing existing Netdata storage-health chart contracts where practical.

### User Request

From the parent request in SOW-0016, support monitoring:

- S.M.A.R.T.
- NVMe disks

User decision from 2026-05-13: use the recommended hybrid strategy. Preserve/reuse `smartctl` where practical for S.M.A.R.T.; investigate native macOS IOKit/NVMe SMART support for NVMe.

### Assistant Understanding

Facts:

- `smartctl` and `nvme` go.d collectors already exist.
- `smartctl` platform metadata currently says Linux/BSD and non-Windows execution uses `ndsudo`.
- `nvme` platform metadata currently says Linux/BSD and non-Windows execution uses `ndsudo`.
- `ndsudo` already allow-lists `smartctl` and `nvme` commands used by these collectors.
- The local macOS SDK exposes public IOKit ATA SMART and NVMe SMART headers.

Inferences:

- Reusing existing chart contexts avoids needless dashboard/API churn.
- macOS NVMe should not assume Linux `nvme-cli` behavior before evidence; native IOKit/NVMe SMART may be the better macOS path.

Unknowns:

- Whether `smartctl --json --scan` and device polling behave consistently on macOS for SATA/USB/NVMe devices.
- Whether native IOKit NVMe SMART gives equivalent fields to the existing `nvme` collector contexts.
- Whether the storage-health implementation belongs in go.d, `macos.plugin`, or a shared helper.

### Acceptance Criteria

- macOS S.M.A.R.T. collection works for supported storage devices through the selected path.
- macOS NVMe health collection works for supported NVMe devices through the selected native/hybrid path.
- Existing `smartctl` and `nvme` chart contexts are reused where semantically compatible.
- Device discovery is bounded and does not cause per-cycle expensive scans.
- Serial numbers and real hardware identifiers are not committed in fixtures/docs/SOWs.
- Metadata, config schema, stock configs, health alerts, and docs are updated consistently.

## Analysis

Sources checked:

- SOW-0016.
- `src/go/plugin/go.d/collector/smartctl/`.
- `src/go/plugin/go.d/collector/nvme/`.
- `src/collectors/utils/ndsudo.c`.
- Local macOS SDK header scan for ATA SMART and NVMe SMART.

Current state:

- Existing storage-health collectors are not documented or initialized as macOS-ready.
- Native macOS storage SMART APIs exist in the local SDK, but are not used by Netdata today.

Risks:

- Duplicating storage-health chart logic between go.d and `macos.plugin`.
- Misaligning native IOKit fields with existing storage-health chart semantics.
- Exposing hardware serials in labels or fixtures.

## Pre-Implementation Gate

Status: blocked-on-activation

Problem / root-cause model:

- macOS storage health is missing because existing Netdata storage-health collectors target Linux/BSD-style tooling and privilege assumptions, while macOS also exposes native IOKit storage SMART interfaces that are currently unused.

Evidence reviewed:

- See `## Analysis` and SOW-0016.

Affected contracts and surfaces:

- go.d `smartctl` and `nvme` collector code, tests, metadata, config schemas, stock configs, and README/generated docs.
- `macos.plugin` if native storage-health metrics land there.
- `ndsudo` if new macOS allow-list entries or privilege behavior are required.
- Health alerts for storage-health states.

Existing patterns to reuse:

- Existing `smartctl` and `nvme` metric contexts.
- Existing go.d test fixture style with synthetic JSON.
- Existing `ndsudo` allow-list discipline.

Risk and blast radius:

- Medium, because storage-health data is operationally important and device behavior differs by transport.

Sensitive data handling plan:

- Use synthetic fixtures. Do not commit real serials, device UUIDs, host-specific paths beyond generic public examples, or raw hardware dumps.

Implementation plan:

1. Validate current `smartctl` behavior on macOS and inspect relevant upstream/open-source implementations.
2. Validate native IOKit ATA/NVMe SMART field coverage and compare to existing contexts.
3. Choose exact code location based on evidence without duplicating logic unnecessarily.
4. Implement macOS support and update artifacts.

Validation plan:

- Go tests for changed go.d collectors.
- macOS build and real-use checks with sanitized summaries.
- Same-failure search for storage-health platform assumptions.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update only if durable storage-health authoring guidance emerges.
- Specs: add/update macOS storage-health contract if shipped.
- End-user/operator docs: update storage collector docs.
- End-user/operator skills: likely unaffected.
- SOW lifecycle: close with implementation artifacts when complete.

Open-source reference evidence:

- Pending implementation investigation.

Open decisions:

- None from the umbrella SOW. Implementation may reveal a code-location decision if evidence shows native IOKit should not live in existing go.d collectors.

## Implications And Decisions

- 2026-05-13: User selected the recommended hybrid storage-health strategy from SOW-0016.

## Plan

1. Activate after the user or assistant selects this SOW as the next implementation unit.
2. Complete implementation gate with deeper evidence.
3. Implement and validate.

## Execution Log

### 2026-05-13

- Created as split implementation SOW from SOW-0016.

## Validation

Acceptance criteria evidence:

- Pending implementation.

Tests or equivalent validation:

- Pending implementation.

Real-use evidence:

- Pending implementation.

Reviewer findings:

- Pending implementation.

Same-failure scan:

- Pending implementation.

Sensitive data gate:

- Planning-only SOW uses sanitized evidence.

Artifact maintenance gate:

- AGENTS.md: pending implementation outcome.
- Runtime project skills: pending implementation outcome.
- Specs: pending implementation outcome.
- End-user/operator docs: pending implementation outcome.
- End-user/operator skills: pending implementation outcome.
- SOW lifecycle: open in pending, matching status.

Specs update:

- Pending implementation outcome.

Project skills update:

- Pending implementation outcome.

End-user/operator docs update:

- Pending implementation outcome.

End-user/operator skills update:

- Pending implementation outcome.

Lessons:

- None yet.

Follow-up mapping:

- None yet.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.

