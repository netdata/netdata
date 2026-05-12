# SOW-0017 - macOS power, thermal sensors, and fans

## Status

Status: open

Sub-state: split from SOW-0016 after user decisions on 2026-05-13; pending activation. Implementation has not started.

## Requirements

### Purpose

Add macOS battery, thermal, and fan monitoring with production-safe collection behavior, stable chart contracts, and clear fallback behavior across Intel and Apple Silicon Macs.

### User Request

From the parent request in SOW-0016, support monitoring:

- Battery
- Thermal sensors
- Fans

User decision from 2026-05-13: follow the recommended split and support Intel and Apple Silicon. For exact thermal/fan data, use bounded privileged Apple tooling first; do not start with private SMC/IOReport code unless later evidence justifies the risk.

### Assistant Understanding

Facts:

- `macos.plugin` exists and runs `sysctl`, `mach_smi`, and `iokit` modules (`src/collectors/macos.plugin/plugin_macos.c:15-18`).
- Existing `macos.plugin` code already uses CoreFoundation/IOKit patterns (`src/collectors/macos.plugin/macos_fw.c:5-13`).
- Apple's public IOKit power-source APIs expose readable power-source dictionaries through `IOPSCopyPowerSourcesInfo()` and `IOPSGetPowerSourceDescription()`.
- The local macOS `powermetrics --help` output lists `battery`, `thermal`, and `smc` samplers.
- The local macOS SDK scan did not expose public SMC or IOReport headers.

Inferences:

- Battery should be implemented through public IOKit power-source APIs in or adjacent to `macos.plugin`.
- Thermal/fan exact readings require a slower, bounded sampling path because `powermetrics` is a sampling command and must not be spawned per second.
- The collector must gracefully emit no charts for hardware that does not expose a source.

Unknowns:

- Exact `powermetrics` output shape across macOS versions and hardware families.
- Whether `powermetrics` can provide all requested fan and thermal readings on representative Intel and Apple Silicon machines.
- Whether Netdata installation packaging currently grants enough privilege for the selected `powermetrics` helper path on macOS.

### Acceptance Criteria

- Battery charts exist on Macs with battery/power-source data and are absent or marked unavailable on systems without battery data.
- Thermal/fan charts are populated from the selected bounded `powermetrics` path where available.
- Collection does not spawn unbounded processes, block the 1-second macOS collector loop, or log errors per cycle.
- Charts have stable contexts, units, labels, and dashboard families.
- Metadata, docs, stock configuration, and any alert templates are updated consistently.
- Validation includes macOS build evidence and real-use checks on representative hardware, or records why a hardware family could not be validated.

## Analysis

Sources checked:

- SOW-0016.
- `src/collectors/macos.plugin/plugin_macos.c`.
- `src/collectors/macos.plugin/macos_fw.c`.
- Local `powermetrics(1)` manual and `powermetrics --help`.
- Local macOS SDK header scan.

Current state:

- `macos.plugin` does not currently expose battery, thermal sensors, or fan metrics.
- Linux sensor patterns exist, but the go.d sensors collector is Linux-only and sysfs-based.

Risks:

- `powermetrics` may require elevated privileges and may be too expensive unless cached or run as a long-lived helper.
- Sensor names and availability may differ substantially across Intel and Apple Silicon.
- Exact readings may not be available for every hardware model.

## Pre-Implementation Gate

Status: blocked-on-activation

Problem / root-cause model:

- macOS lacks battery, thermal, and fan metrics because current macOS collection focuses on generic OS counters and IOKit disk/interface statistics. Battery has a public API path; exact thermal/fan readings require a bounded privileged sampler path or accepting private API risk.

Evidence reviewed:

- See `## Analysis` and SOW-0016.

Affected contracts and surfaces:

- `macos.plugin` module list, worker jobs, chart contexts, chart labels, and collection cadence.
- CMake/platform linking if new frameworks are needed.
- Metadata/config/docs/alerts for macOS hardware monitoring.
- Packaging/privilege posture if a helper path is required.

Existing patterns to reuse:

- Existing `macos.plugin` IOKit/CoreFoundation handling.
- Existing Linux `sensors` chart concepts where cross-platform context reuse is safe.
- Existing `ndsudo` hard-coded allow-list model if a privileged helper is needed.

Risk and blast radius:

- Medium to high for thermal/fan support due hardware variance and privilege requirements.
- Low to medium for battery support through public APIs.

Sensitive data handling plan:

- Do not commit real sensor dumps, hostnames, serials, or hardware-identifying raw output. Use synthetic fixtures and sanitized evidence.

Implementation plan:

1. Investigate `powermetrics` output and permissions on representative macOS hardware.
2. Inspect 2-3 open-source macOS monitoring implementations for battery/sensor/fan handling and record commit-pinned evidence.
3. Implement battery via public IOKit power-source APIs.
4. Implement bounded thermal/fan sampling via selected helper model.
5. Update metadata/docs/config/alerts.

Validation plan:

- Build on macOS.
- Run targeted real-use checks with sanitized summaries.
- Verify no per-cycle process spawning, log floods, fd leaks, or collection-loop blocking.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update only if a durable macOS sensor authoring rule emerges.
- Specs: add/update macOS hardware behavior spec if shipped.
- End-user/operator docs: update macOS collector docs and troubleshooting.
- End-user/operator skills: likely unaffected.
- SOW lifecycle: close with implementation artifacts when complete.

Open-source reference evidence:

- Pending implementation investigation.

Open decisions:

- None from the umbrella SOW. Implementation may reveal a user decision if `powermetrics` cannot provide exact data.

## Implications And Decisions

- 2026-05-13: User selected the recommended hardware path from SOW-0016: bounded privileged Apple tooling for exact thermal/fan data, public APIs for battery, Intel and Apple Silicon support with feature detection and graceful gaps.

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

