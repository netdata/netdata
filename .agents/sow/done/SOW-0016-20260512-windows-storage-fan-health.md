# SOW-0016 - Windows Storage And Fan Health Monitoring

## Status

Status: completed

Sub-state: Implementation, documentation, targeted validation, spec update, and artifact gates are complete.

## Requirements

### Purpose

Support a PR that improves Netdata monitoring on Windows for SMART disk health, NVMe health including thermal throttling signals, and fan-related telemetry without overclaiming unsupported hardware data.

### User Request

Make a PR to support monitoring fan speeds, SMART disk health, and NVMe devices including thermal throttling with Netdata on Windows.

### Assistant Understanding

Facts:

- Netdata already has go.d `smartctl` and `nvme` collectors with storage health, NVMe critical-warning, temperature, and thermal-management metrics.
- The `smartctl` collector has a Windows direct-execution path for `smartctl.exe`, but its README/metadata are Linux/ndsudo-centric and the direct scan-open path currently builds conflicting scan arguments.
- The `nvme` collector has a Windows direct-execution path for an external `nvme.exe`, but Windows has native storage protocol APIs that can query NVMe health logs without requiring Linux `nvme-cli` semantics.
- `windows.plugin` has a Sensor API collector and a thermal-zone collector, but the standard Sensor API headers used by this tree do not define a standard fan RPM data field.
- WMI has `Win32_Fan`, but it exposes platform-dependent fan device metadata and desired/requested speed when available; it is not a reliable cross-machine source of actual tachometer RPM.

Inferences:

- SMART disk health should reuse the existing `smartctl` collector instead of duplicating SMART parsing in `windows.plugin`.
- Native Windows NVMe support should reuse the existing `nvme` collector charts, dimensions, metadata, and alert context, adding a Windows backend that maps Windows NVMe health log data into the existing model.
- Fan support needed a product decision: either ship best-effort Windows fan metadata/requested speed with explicit caveats, or leave true actual fan RPM out until there is a reliable source such as vendor, EC/Super I/O, IPMI, or a supported third-party provider.

Unknowns:

- No irreducible product unknowns remain. Platform-specific availability of fan and NVMe data will be handled as runtime best-effort discovery.

### Acceptance Criteria

- Windows SMART disk health support is documented and validated through the existing `smartctl` collector path.
- Windows NVMe devices can be discovered and report health, temperature, and thermal throttling metrics through a native Windows backend or a documented fallback.
- Fan telemetry behavior is implemented according to the explicit fan semantics decision.
- Integration metadata, README/config/alert surfaces, and SOW artifacts are consistent with the implementation.
- Targeted tests or equivalent validation pass for every changed collector path.

## Analysis

Sources checked:

- SOW status directories and `.agents/sow/specs/`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/skills/integrations-lifecycle/SKILL.md`
- `src/collectors/windows.plugin/GetSensors.c`
- `src/collectors/windows.plugin/perflib-thermalzone.c`
- `src/collectors/windows.plugin/perflib-storage.c`
- `src/collectors/windows.plugin/windows_plugin.c`
- `src/collectors/windows.plugin/metadata.yaml`
- `src/go/plugin/go.d/collector/smartctl/`
- `src/go/plugin/go.d/collector/nvme/`
- `src/health/health.d/nvme.conf`
- Local Windows Sensor API and NVMe/storage protocol headers available in the build environment.

Current state:

- `windows.plugin` reports disk performance through perflib and CPU/thermal sensor data through existing Windows APIs, but not SMART disk health or NVMe SMART health.
- `smartctl` already has Windows executable discovery but needs a direct scan argument fix and Windows documentation/metadata alignment.
- `nvme` already has the required chart and metric vocabulary for thermal throttling, but its Windows path depends on an external CLI and does not handle native Windows physical-drive paths.
- Fan RPM is not represented in the existing Sensor API collector because the standard Windows sensor data fields available here do not include fan RPM.

Risks:

- Presenting WMI `Win32_Fan.DesiredSpeed` as actual fan speed would mislead users and create false monitoring confidence.
- Native NVMe querying touches Windows storage IOCTL code, so struct layout, permissions, and device-access handling must be conservative and well tested.
- Duplicating SMART or NVMe parsing in `windows.plugin` would create parallel behavior and long-term maintenance risk.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Windows storage health support is incomplete because existing Netdata storage health collectors are present but their Windows support is only partial: `smartctl` can execute `smartctl.exe` directly but has a scan-open argument bug and Linux-focused docs, while `nvme` depends on an external CLI rather than the native Windows NVMe health-log path.
- Windows fan RPM support is not merely missing code; the standard APIs already used by `windows.plugin` do not expose reliable actual fan tachometer RPM. WMI can expose fan objects and desired/requested speed on some systems, but that does not satisfy a strict actual-speed contract.

Evidence reviewed:

- Existing SOW/spec review found no current SOW overlap with Windows storage/fan monitoring.
- `src/go/plugin/go.d/collector/smartctl/init.go` contains Windows `smartctl.exe` discovery.
- `src/go/plugin/go.d/collector/smartctl/exec.go` appends `--scan-open` to an argument list that already contains `--scan`.
- `src/go/plugin/go.d/collector/nvme/init.go` contains Windows `nvme.exe` discovery but no native Windows storage backend.
- `src/go/plugin/go.d/collector/nvme/charts.go`, `collect.go`, and sample tests already model NVMe critical warnings, composite temperature, warning/critical temperature time, and thermal-management transition/time counters.
- `src/collectors/windows.plugin/GetSensors.c` supports Sensor API temperature and custom values but not a standard fan RPM field.
- Local Windows headers expose Sensor API mechanical categories but no standard fan RPM data type.
- Local Windows storage classes expose generic physical-disk health surfaces; native NVMe health-log querying is the more precise match for thermal throttling metrics.

Affected contracts and surfaces:

- go.d collector runtime behavior for `smartctl` and `nvme`.
- go.d collector tests and fixtures for Windows path handling and NVMe health-log mapping.
- `smartctl` and `nvme` collector metadata, README, and integration documentation source surfaces.
- `src/health/health.d/nvme.conf` if alert semantics need adjustment.
- Potential `windows.plugin` metadata/README only if best-effort WMI fan telemetry is selected.
- SOW lifecycle and final PR notes.

Existing patterns to reuse:

- Reuse `smartctl` for SMART health instead of adding SMART parsing to `windows.plugin`.
- Reuse `nvme` collector chart IDs, contexts, metric names, health alert, and sample-driven tests.
- Reuse existing go.d executable discovery and direct command runner patterns where fallback CLI behavior remains useful.
- Reuse Windows plugin WMI helper patterns only if fan telemetry is selected.

Risk and blast radius:

- SMART change is low risk if limited to direct-scan argument construction and documentation.
- Native NVMe backend is moderate risk because Windows storage IOCTL layouts and permissions vary by platform; implementation should fail closed and fall back gracefully.
- Fan telemetry is product-risky: a best-effort metric may be sparse or semantically weaker than users expect. Naming and docs must make the limitation visible.
- No data loss or destructive storage operations are planned. Queries must be read-only.

Sensitive data handling plan:

- Durable artifacts will not include raw secrets, credentials, bearer tokens, SNMP communities, customer names, personal data, customer-identifying non-private IPs, private endpoints, account IDs, or proprietary incident details.
- SOW evidence uses repo-relative paths and sanitized local observations only.
- Code comments and docs will describe API behavior and configuration without embedding local machine identifiers or environment-specific data.

Implementation plan:

1. Fix and test the `smartctl` Windows direct scan-open path; update metadata/README wording to state Windows uses installed smartmontools directly.
2. Add a native Windows backend for the `nvme` collector that discovers physical NVMe drives, queries the NVMe SMART / health information log, and maps temperature, warning, critical, and thermal-management fields into existing metrics.
3. Fix any discovered NVMe Windows path handling or chart-label issues needed for native physical-drive paths.
4. Apply the fan decision: either add explicitly named best-effort WMI fan requested-speed/status telemetry, or track true fan RPM source support separately and keep this PR honest.
5. Update docs, metadata, alerts/config surfaces, specs or project skills only where changed behavior requires it.

Validation plan:

- Run targeted Go tests for `src/go/plugin/go.d/collector/smartctl` and `src/go/plugin/go.d/collector/nvme`.
- Add unit tests for SMART scan argument construction and Windows NVMe health-log mapping where practical.
- Run same-failure searches for other direct scan argument construction and Windows physical-drive path assumptions.
- Perform read-only local Windows checks where available, without recording sensitive local identifiers in durable artifacts.
- Review generated or source documentation consistency for changed collectors.

Artifact impact plan:

- AGENTS.md: No expected update; repository-wide workflow does not change.
- Runtime project skills: No expected update unless this work exposes a reusable collector-authoring rule not already covered.
- Specs: Expected update only if native Windows NVMe behavior establishes a durable collector contract beyond docs/metadata.
- End-user/operator docs: Expected updates for collector README/metadata and generated integration docs if regeneration is performed.
- End-user/operator skills: No expected update unless public/operator AI skill docs are affected, which is unlikely.
- SOW lifecycle: This SOW remains in `current/` while in progress and will move to `done/` with `Status: completed` only in the same commit as implementation and artifact updates.

Open-source reference evidence:

- No external mirrored open-source repositories were checked; the implementation can be grounded in this repository and official Windows API surfaces.

Open decisions:

- Resolved autonomously per user instruction and project patterns: include best-effort WMI `Win32_Fan` telemetry in this PR, explicitly named as desired/requested fan speed and status, not guaranteed actual tachometer RPM. This keeps the PR useful while avoiding false monitoring claims.

## Implications And Decisions

1. Selected best-effort WMI `Win32_Fan` telemetry for fan support.
2. Rejected representing WMI requested speed as guaranteed actual RPM because the Windows API contract does not support that claim.

## Plan

1. Implement the SMART scan fix and documentation alignment.
2. Implement native Windows NVMe health-log discovery and parsing.
3. Implement best-effort WMI fan requested-speed/status telemetry with conservative names and docs.
4. Update collector metadata/docs and any affected alerts/config surfaces.
5. Run targeted validation, review same-failure searches, and close artifact gates.

## Execution Log

### 2026-05-12

- Created feature branch `codex-windows-storage-fan-health`.
- Reviewed open/current SOWs, relevant specs, project collector skill, integrations lifecycle skill, existing Windows plugin collectors, and existing go.d `smartctl` / `nvme` collectors.
- Added this SOW and pre-implementation gate before code changes.
- Resolved fan semantics autonomously after user requested no further questions: implement best-effort WMI requested-speed/status fan telemetry and do not claim universal actual RPM.

### 2026-05-13

- Fixed direct `smartctl.exe` scan-open argument construction so Windows direct execution uses either `--scan` or `--scan-open`, not both.
- Added native Windows NVMe discovery and SMART / health log collection through Windows storage protocol IOCTLs, with optional `nvme.exe` fallback.
- Added Windows physical-drive path handling and tests to the `nvme` collector.
- Added `windows.plugin` `GetFans` WMI collection for requested fan speed and fan state, wired it into the Windows plugin module list, build file list, metadata, README, and generated integration docs.
- Updated NVMe and SMART collector metadata/generated docs for Windows support and updated NVMe config schema wording for the Windows native path.
- Added `.agents/sow/specs/windows-storage-fan-monitoring.md` to record the durable Windows SMART/NVMe/fan contracts.

## Validation

Acceptance criteria evidence:

- Windows SMART disk health support is documented in `smartctl` metadata/generated docs, and the direct scan-open bug is covered by `TestDirectSmartctlScanArgs`.
- Windows NVMe devices are supported by a native backend that enumerates `\\.\PhysicalDriveN`, filters NVMe drives, and maps NVMe health-log fields into existing health, temperature, and thermal-management metrics.
- Fan telemetry is implemented as best-effort WMI `Win32_Fan` requested-speed/status telemetry and documented as not guaranteed actual tachometer RPM.
- Collector consistency artifacts were updated: code, metadata, config schema where affected, stock README pointers/generated integration docs, and Windows plugin README.

Tests or equivalent validation:

- `go test ./plugin/go.d/collector/nvme` from `src/go`: passed.
- `go test ./plugin/go.d/collector/smartctl` from `src/go`: passed.
- Direct compile of `src/collectors/windows.plugin/GetFans.c` using its `build-cygwin-MSYS/compile_commands.json` command: passed.
- Direct compile of `src/collectors/windows.plugin/windows_plugin.c` using its `build-cygwin-MSYS/compile_commands.json` command: passed.
- `PYTHONUTF8=1 python integrations/gen_integrations.py`: passed.
- Targeted integration doc generation for `go.d.plugin/nvme`, `go.d.plugin/smartctl`, and `windows.plugin/GetFans`: passed.
- `git diff --check`: passed with only existing Windows line-ending conversion warnings.
- `.agents/sow/audit.sh`: this SOW passed status/directory and sensitive-data checks; the audit also reported pre-existing unrelated repository warnings for cross-tool bridges, legacy non-project skill classification, and `SOW-0015-20260508-snmp-bgp-typed-projection.md` already living in `done/` with `Status: in-progress`.
- `cmake --build build-cygwin-MSYS --target netdata.exe`: blocked before changed code by the existing build-tree issue `build-cygwin-MSYS/include/json-c` being a directory where the Ninja rule expects to create a symlink.

Real-use evidence:

- Read-only local CIM checks found no `Win32_Fan` instances and no local NVMe physical disks on this host, so runtime hardware-positive fan/NVMe evidence was not available here.
- The absence case matches the documented fan behavior: systems without `Win32_Fan` data do not create fan charts.

Reviewer findings:

- Self-review fixed the Windows NVMe device open order to try read-only access before read/write fallback.
- Self-review fixed native NVMe open error propagation, 128-bit counter clamping before later collector multipliers, the existing NVMe temp2 chart title typo, and NVMe config schema wording.
- No external reviewer findings were available in this local pre-PR pass.

Same-failure scan:

- `rg -- '--scan-open|scan\(open bool\)|directSmartctlScanArgs' src/go/plugin/go.d/collector` found the intended `smartctl` scan-open paths and no other direct scan argument construction requiring the same fix.
- `rg 'PhysicalDrive|StorageDeviceProtocolSpecificProperty|Win32_Fan|fan_requested_speed|Thermal management temp[12]' ...` found only the intended new Windows NVMe/fan paths plus the corrected NVMe chart titles/metadata.

Sensitive data gate:

- Passed. Durable artifacts contain no raw secrets, credentials, customer data, private endpoints, account IDs, or local hardware identifiers.

Artifact maintenance gate:

- AGENTS.md: No update needed; repository workflow and guardrails did not change.
- Runtime project skills: No update needed; existing collector and integrations lifecycle skills already covered the workflow.
- Specs: Updated `.agents/sow/specs/windows-storage-fan-monitoring.md`.
- End-user/operator docs: Updated collector metadata/generated integration docs and `src/collectors/windows.plugin/README.md`.
- End-user/operator skills: No update needed; public/operator AI skills were not affected.
- SOW lifecycle: This SOW is completed and moved from `current/` to `done/`; it will be committed with implementation and artifact updates.

Specs update:

- Added `.agents/sow/specs/windows-storage-fan-monitoring.md` for the Windows SMART, native NVMe, thermal-signal, and fan telemetry contracts.

Project skills update:

- No project skill update needed; no reusable HOW-to-work rule changed beyond existing collector consistency and integrations lifecycle guidance.

End-user/operator docs update:

- Updated `src/go/plugin/go.d/collector/nvme/metadata.yaml`, `src/go/plugin/go.d/collector/nvme/integrations/nvme_devices.md`, and `src/go/plugin/go.d/collector/nvme/config_schema.json`.
- Updated `src/go/plugin/go.d/collector/smartctl/metadata.yaml` and `src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md`.
- Updated `src/collectors/windows.plugin/metadata.yaml`, `src/collectors/windows.plugin/integrations/fans_win.md`, and `src/collectors/windows.plugin/README.md`.

End-user/operator skills update:

- No end-user/operator skill update needed; this work did not change AI skill workflows or published skill commands.

Lessons:

- Windows fan telemetry needs explicit requested-speed wording because `Win32_Fan.DesiredSpeed` is not a universal actual tachometer RPM source.
- Running integration doc generation on Windows can expose path separator problems in ignored `integrations.js`; targeted generation used normalized edit links to avoid unrelated generated-doc churn.

Follow-up mapping:

- True cross-vendor Windows tachometer RPM support is rejected for this SOW because no reliable standard source exists in the investigated project/API surfaces. The implemented WMI path is the supported best-effort Windows interface for this PR.
- No deferred implementation items remain. The local build-tree JSON-C symlink failure is an environmental build-directory issue, not a code follow-up for this SOW.

## Outcome

Implemented Windows SMART documentation/fix, native Windows NVMe health and thermal-signal collection, and best-effort Windows WMI fan telemetry. The branch is ready for commit/PR creation after staging the completed SOW move with the code and documentation changes.

## Lessons Extracted

Record weaker hardware/API semantics explicitly in chart names, docs, and specs. In this case, "fan requested speed" is safe; "fan speed" or "actual RPM" would overclaim what Windows WMI guarantees.

## Followup

None.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and subsequent testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
