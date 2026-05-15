# SOW-0016 - Windows Storage And Fan Health Monitoring

## Status

Status: completed

Sub-state: Completed again on 2026-05-15 after fixing the installed Windows NVMe runtime regression and validating native NVMe, smartctl, and fan charts through the local Agent API.

## Requirements

### Purpose

Support a PR that improves Netdata monitoring on Windows for SMART disk health, NVMe health including thermal throttling signals, and fan-related telemetry without overclaiming unsupported hardware data.

### User Request

Make a PR to support monitoring fan speeds, SMART disk health, and NVMe devices including thermal throttling with Netdata on Windows.

### Assistant Understanding

Facts:

- Netdata already has go.d `smartctl` and `nvme` collectors with storage health, NVMe critical-warning, temperature, and thermal-management metrics.
- The `smartctl` collector has a Windows direct-execution path for `smartctl.exe`, but its README/metadata are Linux/ndsudo-centric and the direct scan-open path currently builds conflicting scan arguments.
- The `nvme` collector already has a Windows direct-execution path for an external `nvme.exe`, so this PR should keep that path instead of adding a second native Windows backend.
- `windows.plugin` has a Sensor API collector and a thermal-zone collector, but the standard Sensor API headers used by this tree do not define a standard fan RPM data field.
- WMI has `Win32_Fan`, but it exposes platform-dependent fan device metadata and desired/requested speed when available; it is not a reliable cross-machine source of actual tachometer RPM.

Inferences:

- SMART disk health should reuse the existing `smartctl` collector instead of duplicating SMART parsing in `windows.plugin`.
- Windows NVMe support should reuse the existing `nvme` collector charts, dimensions, metadata, alert context, and direct `nvme.exe` execution path.
- Fan support needed a product decision: either ship best-effort Windows fan metadata/requested speed with explicit caveats, or leave true actual fan RPM out until there is a reliable source such as vendor, EC/Super I/O, IPMI, or a supported third-party provider.

Unknowns:

- No irreducible product unknowns remain. Platform-specific availability of fan and NVMe data will be handled as runtime best-effort discovery.

### Acceptance Criteria

- Windows SMART disk health support is documented and validated through the existing `smartctl` collector path.
- Windows NVMe devices can report health, temperature, and thermal throttling metrics through an installed `nvme.exe` on Windows.
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
- Local Windows Sensor API headers available in the build environment.

Current state:

- `windows.plugin` reports disk performance through perflib and CPU/thermal sensor data through existing Windows APIs, but not SMART disk health or NVMe SMART health.
- `smartctl` already has Windows executable discovery but needs a direct scan argument fix and Windows documentation/metadata alignment.
- `nvme` already has the required chart and metric vocabulary for thermal throttling, and its Windows path already executes an installed `nvme.exe` directly.
- Fan RPM is not represented in the existing Sensor API collector because the standard Windows sensor data fields available here do not include fan RPM.

Risks:

- Presenting WMI `Win32_Fan.DesiredSpeed` as actual fan speed would mislead users and create false monitoring confidence.
- Adding a second native NVMe backend would increase maintenance risk and duplicate the existing `nvme.exe` command contract.
- Duplicating SMART or NVMe parsing in `windows.plugin` would create parallel behavior and long-term maintenance risk.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Windows storage health support is incomplete because existing Netdata storage health collectors are present but their Windows support is only partial: `smartctl` can execute `smartctl.exe` directly but has a scan-open argument bug and Linux-focused docs, while `nvme` needs accurate Windows documentation around its existing direct `nvme.exe` path.
- Windows fan RPM support is not merely missing code; the standard APIs already used by `windows.plugin` do not expose reliable actual fan tachometer RPM. WMI can expose fan objects and desired/requested speed on some systems, but that does not satisfy a strict actual-speed contract.

Evidence reviewed:

- Existing SOW/spec review found no current SOW overlap with Windows storage/fan monitoring.
- `src/go/plugin/go.d/collector/smartctl/init.go` contains Windows `smartctl.exe` discovery.
- `src/go/plugin/go.d/collector/smartctl/exec.go` appends `--scan-open` to an argument list that already contains `--scan`.
- `src/go/plugin/go.d/collector/nvme/init.go` contains Windows `nvme.exe` discovery and should remain the Windows NVMe execution path.
- `src/go/plugin/go.d/collector/nvme/charts.go`, `collect.go`, and sample tests already model NVMe critical warnings, composite temperature, warning/critical temperature time, and thermal-management transition/time counters.
- `src/collectors/windows.plugin/GetSensors.c` supports Sensor API temperature and custom values but not a standard fan RPM field.
- Local Windows headers expose Sensor API mechanical categories but no standard fan RPM data type.
- The existing `nvme smart-log` parser already maps the thermal-throttling fields needed for this PR.

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
- Reuse existing go.d executable discovery and direct command runner patterns for Windows `nvme.exe`.
- Reuse Windows plugin WMI helper patterns only if fan telemetry is selected.

Risk and blast radius:

- SMART change is low risk if limited to direct-scan argument construction and documentation.
- NVMe risk is lower when limited to the existing command runner, but the PR must not overclaim support without an installed `nvme.exe`.
- Fan telemetry is product-risky: a best-effort metric may be sparse or semantically weaker than users expect. Naming and docs must make the limitation visible.
- No data loss or destructive storage operations are planned. Queries must be read-only.

Sensitive data handling plan:

- Durable artifacts will not include raw secrets, credentials, bearer tokens, SNMP communities, customer names, personal data, customer-identifying non-private IPs, private endpoints, account IDs, or proprietary incident details.
- SOW evidence uses repo-relative paths and sanitized local observations only.
- Code comments and docs will describe API behavior and configuration without embedding local machine identifiers or environment-specific data.

Implementation plan:

1. Fix and test the `smartctl` Windows direct scan-open path; update metadata/README wording to state Windows uses installed smartmontools directly.
2. Keep the existing Windows `nvme.exe` direct execution path and remove the duplicate native backend from the PR.
3. Keep any NVMe chart-label or metadata fixes that apply to the existing `nvme smart-log` data model.
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
- Specs: Update the Windows storage/fan monitoring spec to state that Windows NVMe uses `nvme.exe`, not a native backend.
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
2. Keep the existing Windows `nvme.exe` path for NVMe health-log collection and avoid adding a duplicate native backend.
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
- Initially added native Windows NVMe discovery and SMART / health log collection, then removed it on 2026-05-14 after maintainer/user review confirmed the existing `nvme.exe` path is the desired Windows implementation.
- Removed Windows physical-drive path handling and tests from the `nvme` collector with the native backend.
- Added `windows.plugin` `GetFans` WMI collection for requested fan speed and fan state, wired it into the Windows plugin module list, build file list, metadata, README, and generated integration docs.
- Updated NVMe and SMART collector metadata/generated docs for Windows support and updated NVMe config schema wording for the Windows `nvme.exe` path.
- Added `.agents/sow/specs/windows-storage-fan-monitoring.md` to record the durable Windows SMART/NVMe/fan contracts.
- Reopened after live service validation showed the Windows compile/run helper did not install `go.d.plugin.exe`, then enabled `ENABLE_PLUGIN_GO=On` in that helper so SMART/NVMe go.d collectors are reachable in Windows service installs.
- Reopened again after identifying that existing build directories would keep cached `ENABLE_PLUGIN_GO=Off`; changed the helper to always run the CMake configure step before building.

### 2026-05-14

- Removed the native Windows NVMe backend files from the PR.
- Restored the `nvme` collector initialization to use the existing Windows direct `nvme.exe` path and the existing non-Windows `ndsudo` path.
- Updated NVMe metadata, generated integration docs, config schema wording, and the Windows storage/fan spec to state that Windows NVMe requires an installed `nvme.exe`.

## Validation

Acceptance criteria evidence:

- Windows SMART disk health support is documented in `smartctl` metadata/generated docs, and the direct scan-open bug is covered by `TestDirectSmartctlScanArgs`.
- Windows NVMe devices are supported through the existing direct `nvme.exe` execution path, which maps `nvme smart-log` JSON into existing health, temperature, and thermal-management metrics.
- Fan telemetry is implemented as best-effort WMI `Win32_Fan` requested-speed/status telemetry and documented as not guaranteed actual tachometer RPM.
- Collector consistency artifacts were updated: code, metadata, config schema where affected, stock README pointers/generated integration docs, and Windows plugin README.

Tests or equivalent validation:

- `go test ./plugin/go.d/collector/nvme` from `src/go`: passed.
- `go test ./plugin/go.d/collector/smartctl` from `src/go`: passed.
- `go test ./plugin/go.d/collector/nvme` from `src/go` after removing the native backend: passed.
- `go test ./plugin/go.d/collector/smartctl` from `src/go` after removing the native backend: passed.
- Direct compile of `src/collectors/windows.plugin/GetFans.c` using its `build-cygwin-MSYS/compile_commands.json` command: passed.
- Direct compile of `src/collectors/windows.plugin/windows_plugin.c` using its `build-cygwin-MSYS/compile_commands.json` command: passed.
- `PYTHONUTF8=1 python integrations/gen_integrations.py`: passed.
- Targeted integration doc generation for `go.d.plugin/nvme`, `go.d.plugin/smartctl`, and `windows.plugin/GetFans`: passed.
- `git diff --check`: passed with only existing Windows line-ending conversion warnings.
- `bash -n packaging/utils/compile-and-run-windows.sh`: passed after enabling `ENABLE_PLUGIN_GO=On`.
- `C:\msys64\usr\bin\bash.exe -n packaging/utils/compile-and-run-windows.sh` after removing the native backend: passed.
- `rg 'native Windows|native backend|IOCTL|PhysicalDrive|StorageDeviceProtocolSpecificProperty|IOCTL_STORAGE|exec_windows|init_windows|init_nonwindows|optional Windows CLI fallback' src/go/plugin/go.d/collector/nvme .agents/sow/specs/windows-storage-fan-monitoring.md` after removing the native backend: no matches.
- `git diff --check` after removing the native backend: passed with only existing Windows line-ending conversion warnings.
- `MSYSTEM=MSYS ./packaging/utils/compile-and-run-windows.sh service`: reconfigured the existing `build-cygwin-MSYS` tree with `ENABLE_PLUGIN_GO=On`, built, and installed `go.d.plugin.exe` plus stock `nvme.conf` / `smartctl.conf`; the command exited non-zero at the pre-existing Windows Event Log manifest import step with `Access is denied`, before service restart.
- `.agents/sow/audit.sh`: this SOW passed pre-implementation gate, regression placement, and sensitive-data checks; the audit also reported pre-existing unrelated repository warnings for cross-tool bridges, legacy non-project skill classification, and `SOW-0015-20260508-snmp-bgp-typed-projection.md` already living in `done/` with `Status: in-progress`.
- `cmake --build build-cygwin-MSYS --target netdata.exe`: blocked before changed code by the existing build-tree issue `build-cygwin-MSYS/include/json-c` being a directory where the Ninja rule expects to create a symlink.

Real-use evidence:

- Read-only local CIM checks found no `Win32_Fan` instances and no local NVMe physical disks on this host, so runtime hardware-positive fan/NVMe evidence was not available here.
- The absence case matches the documented fan behavior: systems without `Win32_Fan` data do not create fan charts.
- Live Windows service validation showed the service was running the PR build and listening on port 19999. API requests were blocked by Netdata's authorization gate, which is unrelated to collector startup.
- The installed service tree initially lacked `go.d.plugin.exe`; this regression was repaired by enabling `ENABLE_PLUGIN_GO=On` in the Windows helper build configuration.
- After forcing helper reconfiguration for existing build trees, `/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin.exe` exists and reports `go.d.plugin, version: v2.10.0-206-g3372ad94cc`; stock `nvme.conf` and `smartctl.conf` are present under `/opt/netdata/usr/lib/netdata/conf.d/go.d`.

Reviewer findings:

- Self-review removed the duplicate native Windows NVMe backend so the PR keeps only the existing `nvme.exe` path.
- Self-review kept the existing NVMe temp2 chart title correction, 128-bit counter clamping before subsequent collector multipliers, and NVMe config schema wording where they apply to the existing parser.
- No external reviewer findings were available in this local pre-PR pass.

Same-failure scan:

- `rg -- '--scan-open|scan\(open bool\)|directSmartctlScanArgs' src/go/plugin/go.d/collector` found the intended `smartctl` scan-open paths and no other direct scan argument construction requiring the same fix.
- `rg 'PhysicalDrive|StorageDeviceProtocolSpecificProperty|IOCTL_STORAGE|exec_windows|init_windows|init_nonwindows' src/go/plugin/go.d/collector/nvme` found no remaining native Windows NVMe backend code.
- `rg 'Win32_Fan|fan_requested_speed|Thermal management temp[12]' ...` found only the intended fan path plus the corrected NVMe chart titles/metadata.

Sensitive data gate:

- Passed. Durable artifacts contain no raw secrets, credentials, customer data, private endpoints, account IDs, or local hardware identifiers.

Artifact maintenance gate:

- AGENTS.md: No update needed; repository workflow and guardrails did not change.
- Runtime project skills: No update needed; existing collector and integrations lifecycle skills already covered the workflow.
- Specs: Updated `.agents/sow/specs/windows-storage-fan-monitoring.md`.
- End-user/operator docs: Updated collector metadata/generated integration docs and `src/collectors/windows.plugin/README.md`.
- End-user/operator skills: No update needed; public/operator AI skills were not affected.
- SOW lifecycle: This SOW was completed, reopened for the live-service install-path regressions, repaired, and moved back to `done/` with implementation and artifact updates.

Specs update:

- Added `.agents/sow/specs/windows-storage-fan-monitoring.md` for the Windows SMART, `nvme.exe` NVMe, thermal-signal, and fan telemetry contracts.

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
- No open implementation items remain. The local build-tree JSON-C symlink failure is an environmental build-directory issue, not a code item for this SOW.

## Outcome

Implemented Windows SMART documentation/fix, Windows NVMe `nvme.exe` documentation/metadata alignment for health and thermal-signal collection, best-effort Windows WMI fan telemetry, Windows helper build enablement for `go.d.plugin.exe`, and helper reconfiguration for existing Windows build directories.

## Lessons Extracted

Record weaker hardware/API semantics explicitly in chart names, docs, and specs. In this case, "fan requested speed" is safe; "fan speed" or "actual RPM" would overclaim what Windows WMI guarantees.

## Followup

None.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and subsequent testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.

## Regression - 2026-05-13

What broke:

- The initial PR claimed Windows SMART/NVMe support through go.d collectors, but live service validation on the Windows VM showed `/opt/netdata/usr/libexec/netdata/plugins.d` contained no `go.d.plugin.exe`.
- The Windows compile/run helper configures CMake with `DEFAULT_FEATURE_STATE=Off` and explicitly enables only `ENABLE_PLUGIN_APPS=On`, so `ENABLE_PLUGIN_GO` remains off for that install path.

Evidence:

- Netdata service was running from `C:\msys64\opt\netdata\usr\bin\netdata.exe` at commit `2953301da7`.
- `C:\msys64\opt\netdata\usr\libexec\netdata\plugins.d` contained `windows-events.plugin.exe` but no `go.d.plugin.exe`.
- `CMakeLists.txt` already has Windows-aware `go.d.plugin.exe` build/install rules gated by `ENABLE_PLUGIN_GO`.
- `packaging/utils/compile-and-run-windows.sh` passes `-DDEFAULT_FEATURE_STATE=Off` and did not pass `-DENABLE_PLUGIN_GO=On`.

Why previous validation missed it:

- The first pass validated go.d collector packages with `go test` and validated C code compiles, but did not validate that the Windows service install path includes the Go plugin binary.
- The VM lacks fan/NVMe hardware, which is expected, but the missing `go.d.plugin.exe` is independent of hardware and blocks SMART/NVMe collection entirely.

Repair plan:

- Enable `ENABLE_PLUGIN_GO=On` in the Windows compile/run helper so Windows service builds install `go.d.plugin.exe` and the go.d stock configuration tree.
- Validate by checking the CMake/install rules and rerunning targeted Go tests.
- Record that hardware-positive fan/NVMe runtime validation remains unavailable on this VM, while install reachability is now addressed.

Validation:

- `go test ./plugin/go.d/collector/nvme`: passed.
- `go test ./plugin/go.d/collector/smartctl`: passed.
- `bash -n packaging/utils/compile-and-run-windows.sh`: passed.
- Source inspection confirmed `packaging/utils/compile-and-run-windows.sh` now passes `-DENABLE_PLUGIN_GO=On`, while `CMakeLists.txt` already maps that to Windows `go.d.plugin.exe` build/install rules.

Artifact updates:

- Updated SOW validation/outcome after repair.
- No new end-user docs were needed; this changes the Windows developer/helper build path so the already-documented SMART/NVMe support is reachable.

## Regression - 2026-05-13 Existing Build Reconfigure

What broke:

- Enabling `ENABLE_PLUGIN_GO=On` in `packaging/utils/compile-and-run-windows.sh` was not enough for existing Windows build directories because the helper only ran CMake configuration when the build directory did not exist.
- A developer running the documented service command against an existing `build-cygwin-MSYS` tree would still use the previous cached `ENABLE_PLUGIN_GO=Off` configuration and would not install `go.d.plugin.exe`.

Evidence:

- The helper wrapped the CMake configuration command in `if [ ! -d "${build}" ]; then ... fi`.
- The user-provided live install path is `./packaging/utils/compile-and-run-windows.sh service` under `MSYSTEM=MSYS` from the repository root, which commonly reuses an existing build directory.

Why previous validation missed it:

- The previous repair inspected and syntax-checked the helper but did not account for CMake cache persistence in existing build directories.

Repair plan:

- Always run the CMake configure step before building/installing so the helper refreshes `ENABLE_PLUGIN_GO=On` for both new and existing build directories.
- Run the service install path under `MSYSTEM=MSYS` from the repository root and verify `go.d.plugin.exe` is installed.

Validation:

- `bash -n packaging/utils/compile-and-run-windows.sh`: passed.
- `go test ./plugin/go.d/collector/nvme ./plugin/go.d/collector/smartctl`: passed.
- `MSYSTEM=MSYS ./packaging/utils/compile-and-run-windows.sh service`: reconfigured the existing build tree, built, and installed `go.d.plugin.exe` and go.d stock configs. The command exited non-zero at Windows Event Log manifest import with `Access is denied`, before service restart.
- `build-cygwin-MSYS/CMakeCache.txt` confirms `DEFAULT_FEATURE_STATE=Off` and `ENABLE_PLUGIN_GO=On`.
- Installed file checks confirm `/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin.exe`, `/opt/netdata/usr/lib/netdata/conf.d/go.d/nvme.conf`, and `/opt/netdata/usr/lib/netdata/conf.d/go.d/smartctl.conf` exist.

Artifact updates:

- Updated this SOW after the service install validation.
- No new end-user docs were needed; this changes the Windows developer/helper build path so already-documented SMART/NVMe support is reachable.

## Regression - 2026-05-14 Native NVMe Backend Removed

What changed:

- Maintainer/user review identified that the native Windows NVMe backend was unnecessary because the go.d `nvme` collector already supports Windows by executing an installed `nvme.exe`.
- The PR should keep the `nvme.exe` path and remove the duplicate native backend implementation.

Evidence:

- `src/go/plugin/go.d/collector/nvme/exec.go` already implements direct `nvme list --output-format=json` and `nvme smart-log <device> --output-format=json` execution.
- `src/go/plugin/go.d/collector/nvme/init.go` already discovers `nvme`/`nvme-cli` plus `Program Files\nvme-cli\nvme.exe` locations for Windows direct execution.
- The existing `nvme smart-log` parser already maps critical warnings, composite temperature, warning/critical temperature time, and thermal-management counters.

Why previous validation missed it:

- The initial root-cause pass focused on filling a perceived Windows-native gap, but did not weight the existing Windows `nvme.exe` command path as the intended maintainer-preferred implementation.

Repair plan:

- Delete native Windows NVMe backend files and tests.
- Restore unified `initNVMeCLIExec()` selection in `init.go`: Windows direct `nvme.exe`, non-Windows `ndsudo`.
- Remove Windows physical-drive path handling that only existed for the native backend.
- Update metadata, config schema, generated integration docs, and specs to describe the `nvme.exe` contract.

Validation:

- `go test ./plugin/go.d/collector/nvme`: passed.
- `go test ./plugin/go.d/collector/smartctl`: passed.
- `C:\msys64\usr\bin\bash.exe -n packaging/utils/compile-and-run-windows.sh`: passed.
- `git diff --check`: passed with only existing Windows line-ending conversion warnings.
- Native-backend same-failure search confirmed no `exec_windows`, `init_windows`, `init_nonwindows`, `IOCTL_STORAGE`, or `StorageDeviceProtocolSpecificProperty` references remain in the `nvme` collector.
- `.agents/sow/audit.sh` through an MSYS login shell: passed this SOW's gates; it continued to report pre-existing unrelated repository warnings.

Artifact updates:

- Updated `.agents/sow/specs/windows-storage-fan-monitoring.md`.
- Updated NVMe metadata, config schema, and generated integration docs.
- Updated this SOW to record the scope correction and removal.

## Regression - 2026-05-14 Fan Context Alignment And Runtime Visibility

What broke:

- Windows fan telemetry was added with new contexts `system.fan_requested_speed` and `system.fan_state` before checking existing Netdata fan/cooling contexts on other collectors.
- The running Windows service build at commit `d7887f5973` exposes no `fan`, `smartctl`, or `nvme` chart IDs through `/api/v1/charts`.
- The local host exposes two `Win32_Fan` instances with `ActiveCooling=True`, `Availability=3`, and `Status=OK`, but `DesiredSpeed` and `VariableSpeed` are null. A speed chart is expected to be absent, but a fan state chart should be possible if the collector maps the available state fields correctly.
- `smartctl.exe` and `nvme.exe` are not installed in PATH or default locations on this host, so SMART/NVMe runtime charts are blocked by documented external-tool prerequisites and are not the immediate defect.

Evidence:

- `src/go/plugin/go.d/collector/sensors` already uses `sensors.chip_sensor_fan` with dimension `input` and unit `RPM` for Linux lm-sensors fan readings, plus `sensors.chip_sensor_fan_alarm` with dimensions `clear` and `triggered`.
- `src/collectors/freeipmi.plugin` uses `ipmi.sensor_fan_speed` with dimension `rotations` and unit `RPM`, and `ipmi.sensor_state` for generic IPMI sensor state.
- The Windows fan chart contexts were introduced only in `src/collectors/windows.plugin/GetFans.c`, `metadata.yaml`, and generated `fans_win.md`.
- Local direct Agent API checks found no charts whose ID or context matches `fan`, `smartctl`, or `nvme`.
- Local `Get-CimInstance Win32_Fan` returned two WMI instances with active/online state data but no desired speed value.

Why previous validation missed it:

- The earlier pass validated compile and install reachability but did not verify chart contexts against existing cross-platform sensor collectors.
- Runtime validation focused on external SMART/NVMe tool absence and did not follow through on the fact that WMI fan state data exists locally and should produce a visible chart.

Repair plan:

- Reuse the existing Linux sensors fan speed context where the semantics match RPM-like fan readings, with compatible dimension names, labels, family, and priority.
- Map WMI `Availability` and `Status` to the existing hardware fan alarm context with `clear` and `fault`; keep WMI capability fields as labels instead of dimensions.
- Patch `GetFans`, metadata, generated integration docs, and the Windows storage/fan spec.
- Rebuild/install using `MSYSTEM=MSYS ./packaging/utils/compile-and-run-windows.sh service`.
- Verify with the direct Agent API using `/api/v1/charts` and `/api/v1/data`, and record the result.

Validation:

- `ninja -C build-cygwin-MSYS netdata`: passed after the fan context/state mapping changes.
- `PYTHONUTF8=1 C:\msys64\mingw64\bin\python.exe integrations\gen_integrations.py`: passed after installing the required MSYS2 MinGW Python packages. `ninja -C build-cygwin-MSYS render-docs` failed because Cygwin Python cannot build the `rpds-py` dependency, so `fans_win.md` was regenerated through `gen_docs_integrations.py` with an in-memory Windows path-normalization shim and unrelated generated deletions were restored.
- `MSYSTEM=MSYS ./packaging/utils/compile-and-run-windows.sh service`: reconfigured and built successfully, installed `/opt/netdata/usr/bin/netdata.exe`, then exited non-zero while trying to stop/delete the Windows service and import the Windows Event Log manifest because the shell lacks service-control privileges (`Access is denied`).
- `ninja -C build-cygwin-MSYS install`: passed after the final mapping tweak and installed the rebuilt `/opt/netdata/usr/bin/netdata.exe`.
- Installed binary evidence: `strings /opt/netdata/usr/bin/netdata.exe` contains `system.hw.sensor.fan.input` and `system.hw.sensor.fan.alarm`; it does not contain `system.fan_requested_speed` or `system.fan_state`.
- WMI evidence: local `Win32_Fan` exposes two fan instances with `Availability=3` and `Status=OK`, no `DesiredSpeed`, and no `VariableSpeed`; the expected live result on this host is two alarm charts and no speed chart.
- Isolated runtime evidence: a foreground Windows Agent built from the patched tree was run on `127.0.0.1:19998` with only `windows.plugin/GetFans` enabled. `/api/v1/charts` exposed `sensors.win32_fan_root/cimv2_0_alarm` and `sensors.win32_fan_root/cimv2_1_alarm` with context `system.hw.sensor.fan.alarm`, chart type `sensors`, family `Fan`, units `status`, and dimensions `clear,fault`.
- Isolated data evidence: `/api/v1/data` for both live fan alarm charts returned columns `time,clear,fault` with values `clear=1` and `fault=0`.
- Same-failure API evidence: the isolated `/api/v1/charts` response contained no `system.fan_requested_speed` or `system.fan_state` contexts.
- Direct Agent API evidence against the still-running service on port 19999 showed no fan contexts because the service did not restart into the installed binary; this is explained by the service-control `Access is denied` failure and is not used as final collector-runtime evidence.
- `git diff --check`: passed with only Windows line-ending conversion warnings.

Artifact updates:

- Updated `src/collectors/windows.plugin/GetFans.c` to emit `sensors` charts with contexts `system.hw.sensor.fan.input` and `system.hw.sensor.fan.alarm`, dimensions `input`, `clear`, and `fault`, and sensor-style `feature`/`label` labels.
- Updated `src/libnetdata/os/windows-wmi/windows-wmi.c` to treat `RPC_E_TOO_LATE` from `CoInitializeSecurity()` as non-fatal, matching COM's process-wide security initialization semantics.
- Updated `src/collectors/windows.plugin/metadata.yaml`, generated `src/collectors/windows.plugin/integrations/fans_win.md`, and `src/collectors/windows.plugin/README.md`.
- Updated `.agents/sow/specs/windows-storage-fan-monitoring.md` with the fan context contract.
- Added `docs/netdata-ai/skills/query-netdata-agents/how-tos/verify-local-collector-contexts.md` and indexed it because this task required direct-agent context/dimension verification analysis.

Artifact maintenance gate:

- AGENTS.md: No update needed; repository workflow and guardrails did not change.
- Runtime project skills: No update needed; existing collector and integrations lifecycle skills already covered the source/edit workflow.
- Specs: Updated `.agents/sow/specs/windows-storage-fan-monitoring.md`.
- End-user/operator docs: Updated Windows plugin metadata, generated integration docs, and README entries for the fan contexts and dimensions.
- End-user/operator skills: Updated the public `query-netdata-agents` how-to catalog with direct local collector context verification guidance.
- SOW lifecycle: Reopened this completed SOW for the regression, recorded the new root cause and validation, and will move it back to `done/` with this implementation commit.

## Regression - 2026-05-15 Native Windows NVMe Needed

What broke:

- Commit `d7887f5973` removed the native Windows NVMe backend and changed the contract to require an installed `nvme.exe` on Windows.
- That assumption was wrong for the PR goal: Windows does not provide a stock `nvme.exe`, so the Agent should not require an external Windows NVMe CLI for local NVMe health collection.

Evidence:

- `d7887f5973` deleted `src/go/plugin/go.d/collector/nvme/exec_windows.go`, `exec_windows_test.go`, `init_windows.go`, and `init_nonwindows.go`, and rewrote metadata/spec text to say Windows requires `nvme.exe`.
- The removed native code implemented the existing go.d `nvmeCli` interface by enumerating `\\.\PhysicalDrive*`, filtering `BusTypeNvme`, and querying the NVMe SMART / health log page through `IOCTL_STORAGE_QUERY_PROPERTY`.
- Microsoft documents `IOCTL_STORAGE_QUERY_PROPERTY` plus `STORAGE_PROTOCOL_SPECIFIC_DATA` as the Windows API for protocol-specific NVMe data. The documented request shape supports `ProtocolTypeNVMe`, `NVMeDataTypeLogPage`, and log pages including SMART/health data.
- The existing go.d `nvme` collector already owns the `nvme.*` contexts, dimensions, labels, health alert, and docs. Keeping native Windows querying inside go.d reuses those contracts; moving it to `windows.plugin` would duplicate the NVMe dashboard model and risk cross-platform aggregation drift.

Why previous validation missed it:

- The earlier correction treated the presence of a direct `nvme.exe` runner as sufficient Windows support, but did not distinguish "optional third-party executable" from "stock Windows capability".
- The local Windows host does not have `nvme.exe`, so the dependency was visible, but it was incorrectly recorded as a prerequisite instead of a regression against the intended Windows support.

Repair plan:

- Restore the native Windows NVMe backend in the go.d `nvme` collector, with `nvme.exe` only as an optional fallback.
- Restore Windows physical-drive path handling in collection helpers and tests.
- Update the Windows storage/fan spec and NVMe metadata/config schema/generated integration docs to describe native Windows querying as the primary path.
- Validate with go.d NVMe tests, integration metadata generation, same-failure searches, and local Windows API/runtime checks where hardware and permissions permit.

Validation:

- `go test -count=1 ./plugin/go.d/collector/nvme ./plugin/go.d/collector/smartctl` from `src/go`: passed.
- `GOOS=linux GOARCH=amd64 go test -c ./plugin/go.d/collector/nvme`: passed compile-only.
- `GOOS=freebsd GOARCH=amd64 go test -c ./plugin/go.d/collector/nvme`: passed compile-only.
- `go build -o ..\..\.local\go.d.plugin.exe ./cmd/godplugin` from `src/go`: passed.
- `MSYSTEM=MSYS ninja -C build-cygwin-MSYS go-plugin`: passed and rebuilt `go.d.plugin.exe` after CMake detected the restored Windows NVMe files.
- `MSYSTEM=MSYS ninja -C build-cygwin-MSYS install`: passed and installed `/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin.exe`.
- `PYTHONUTF8=1 C:\msys64\mingw64\bin\python.exe integrations\gen_integrations.py`: passed and regenerated `src/go/plugin/go.d/collector/nvme/integrations/nvme_devices.md`.
- `git diff --check`: passed with only repository-standard Windows line-ending conversion warnings.
- Same-failure search over the NVMe collector and Windows storage/fan spec found no remaining active text that says Windows requires `nvme.exe` as the primary NVMe collection path.
- Local Windows inventory evidence: `Get-PhysicalDisk` reports two healthy NVMe disks, `Samsung SSD 990 PRO 4TB` and `KXG60PNV2T04 NVMe KIOXIA 2048GB`.
- Direct installed-plugin smoke evidence: `/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin.exe -d -m nvme -j nvme 1` loads the `nvme` module and stock `nvme` job config from the installed tree.
- Direct native enumerator live evidence from a temporary package test: the non-elevated shell returns `Access is denied` when opening `\\.\PhysicalDrive*`. This confirms the local permission boundary; the Netdata Windows service path runs with service privileges, and the code path itself is covered by package tests, cross-builds, and installed-plugin loading.
- PR helper scripts from `.agents/skills/pr-reviews` could not run locally because `gh` is not installed; post-push CI and review state will be checked through the GitHub connector.

Artifact updates:

- Restored `src/go/plugin/go.d/collector/nvme/exec_windows.go`, `exec_windows_test.go`, `init_windows.go`, and `init_nonwindows.go`.
- Restored Windows `\\.\PhysicalDriveN` handling in `src/go/plugin/go.d/collector/nvme/collect.go` and `collector_test.go`.
- Kept native Windows querying in go.d rather than C `windows.plugin` so the collector reuses existing `nvme.*` contexts, dimensions, labels, metadata, and alert contracts across operating systems.
- Updated `src/go/plugin/go.d/collector/nvme/metadata.yaml`, `config_schema.json`, and generated `integrations/nvme_devices.md` to document native Windows collection as primary and `nvme.exe` as an optional fallback.
- Updated `.agents/sow/specs/windows-storage-fan-monitoring.md` with the native Windows NVMe contract.
- AGENTS.md: no update needed; repository workflow and guardrails did not change.
- Runtime project skills: no update needed; existing collector and integrations lifecycle skills covered the workflow.
- Specs: updated the Windows storage/fan monitoring spec.
- End-user/operator docs: updated the NVMe collector metadata and generated integration page.
- End-user/operator skills: no update needed; no public/operator skill behavior changed.
- SOW lifecycle: reopened this completed SOW for the regression and will move it back to `done/` with the implementation commit.

## Regression - 2026-05-15 Installed Windows NVMe Job Not Live

What broke:

- After native Windows NVMe support was restored, the installed Windows Agent exposed the fan charts but no live `nvme.*` charts.
- The go.d `nvme` collector was registered, but the native Windows enumerator could not discover local NVMe devices on this host.

Evidence:

- The running service exposed two `system.hw.sensor.fan.alarm` charts and no obsolete fan contexts.
- Local storage inventory reported two healthy NVMe disks.
- A temporary live probe showed `IOCTL_STORAGE_QUERY_PROPERTY` for the NVMe health log succeeds on both local physical drives when opened with zero desired access.
- The same probe showed `CreateFile()` with `GENERIC_READ` or `GENERIC_READ|GENERIC_WRITE` fails with `Access is denied` from a normal shell.
- The same probe showed the plain storage-device descriptor query fails with `ERROR_BAD_LENGTH` when the input buffer is only the 8-byte fixed header, and succeeds when using the ABI-sized `STORAGE_PROPERTY_QUERY` buffer.

Root cause:

- `openWindowsPhysicalDrive()` required read access before issuing query-only IOCTLs, even though the storage stack allows the relevant property queries through a zero-access handle.
- `queryWindowsStorageDeviceDescriptor()` passed only the 8-byte fixed query header. On this Windows storage stack, the descriptor query requires the padded `STORAGE_PROPERTY_QUERY` size that includes the `AdditionalParameters` field.

Repair:

- Open Windows physical drives with zero desired access first, retaining the read and read/write attempts as fallbacks for storage stacks that require them.
- Use a 12-byte `STORAGE_PROPERTY_QUERY` buffer for ordinary storage descriptor queries.
- Keep the protocol-specific NVMe health-log request payload at offset 8, matching the documented query header plus protocol-specific additional parameters layout.

Validation:

- Installed `smartctl.exe` from smartmontools 7.5 at `C:\Program Files\smartmontools\bin\smartctl.exe`.
- Local `smartctl --json --scan-open` found two NVMe devices, and detailed `smartctl --json --all` checks succeeded for both. Hardware identifiers are intentionally omitted from this SOW.
- Temporary native NVMe live test after the patch discovered both local Windows physical-drive NVMe devices and read their SMART / health logs.
- `go test -count=1 ./plugin/go.d/collector/nvme ./plugin/go.d/collector/smartctl` from `src/go`: passed.
- `GOOS=linux GOARCH=amd64 go test -c ./plugin/go.d/collector/nvme`: passed compile-only.
- `GOOS=freebsd GOARCH=amd64 go test -c ./plugin/go.d/collector/nvme`: passed compile-only.
- `GOOS=darwin GOARCH=amd64 go test -c ./plugin/go.d/collector/nvme`: passed compile-only.
- `MSYSTEM=MSYS ninja -C build-cygwin-MSYS go-plugin`: passed.
- `MSYSTEM=MSYS ninja -C build-cygwin-MSYS install`: passed and installed the rebuilt `go.d.plugin.exe`.
- The user restarted the Windows service from an elevated shell. The restarted Agent showed `go.d` collectors for both `nvme` and `smartctl`.
- Direct Agent API chart inventory on port 19999 showed 32 `nvme.*` charts across two `PhysicalDrive` devices, 8 `smartctl.*` charts across two NVMe devices, and 2 `system.hw.sensor.fan.alarm` charts.
- Direct Agent API data queries returned current native NVMe temperatures for both drives, current smartctl NVMe temperatures/status, and fan alarm values with `clear=1` and `fault=0`.

Artifact maintenance gate:

- AGENTS.md: no update needed; repository workflow and guardrails did not change.
- Runtime project skills: no update needed; the existing collector workflow skill covered the fix and validation path.
- Specs: no update needed; the existing spec already records native Windows NVMe as the primary Windows contract.
- End-user/operator docs: no update needed; chart contexts, dimensions, and user-visible collector contract did not change in this repair.
- End-user/operator skills: no update needed; no public/operator skill behavior changed.
- SOW lifecycle: reopened completed SOW for this regression, recorded root cause and installed-runtime validation, and moved the SOW back to `done/` with the repair commit.
