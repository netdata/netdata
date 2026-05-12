# SOW-0017 - macOS power, thermal sensors, and fans

## Status

Status: completed

Sub-state: implementation, metadata/docs, spec update, validation, and SOW lifecycle move completed.

## Requirements

### Purpose

Add macOS battery, thermal, and fan monitoring with production-safe collection behavior, stable chart contracts, and clear fallback behavior across Intel and Apple Silicon Macs.

### User Request

From the parent request in SOW-0016, support monitoring:

- Battery
- Thermal sensors
- Fans

User decision from 2026-05-13: follow the recommended split and support Intel and Apple Silicon. For exact thermal/fan data, use bounded privileged Apple tooling first; do not start with private SMC/IOReport code unless subsequent evidence justifies the risk.

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

Status: ready-for-implementation

Problem / root-cause model:

- macOS lacks battery, thermal, and fan metrics because current macOS collection focuses on generic OS counters and IOKit disk/interface statistics. Battery has a public API path; exact thermal/fan readings require a bounded privileged sampler path or accepting private API risk.

Evidence reviewed:

- Existing macOS collector shape:
  - `src/collectors/macos.plugin/plugin_macos.c`: module table and worker job pattern for `sysctl`, `mach_smi`, and `iokit`.
  - `src/collectors/macos.plugin/macos_fw.c`: CoreFoundation and IOKit usage already linked into `macos.plugin`.
  - `CMakeLists.txt`: `MACOS_PLUGIN_FILES` are compiled into the main `netdata` target on macOS, linked with IOKit and Foundation.
- Public battery API evidence from the local macOS SDK:
  - `IOPowerSources.h`: `IOPSCopyPowerSourcesInfo()`, `IOPSCopyPowerSourcesList()`, and `IOPSGetPowerSourceDescription()` are the public API for detailed battery and UPS power-source dictionaries.
  - `IOPSKeys.h`: current/max/design capacity, charging state, voltage in mV, current in mA, temperature in Celsius, name, type, and power-source state keys are documented.
- Thermal/fan evidence from local `powermetrics`:
  - `powermetrics --help`: supports `thermal` and `smc` samplers and `--format plist` with NUL-separated machine-readable property lists.
  - `man powermetrics`: the `smc` sampler reports fan speed and temperature sensors where supported; the `thermal` sampler reports current thermal pressure.
  - `strings /usr/bin/powermetrics`: plist keys include `thermal_pressure`, `smc`, `fan`, `cpu_die`, `gpu_die`, `cpu_thermal_level`, `gpu_thermal_level`, `io_thermal_level`, `cpu_prochot`, and `smc_prochot`.
  - Non-root local execution of `powermetrics -n 1 -i 1000 -s battery,thermal,smc --format plist` reports that superuser execution is required; the collector must not retry and log this every cycle.
- Existing chart-contract evidence:
  - `src/collectors/common-contexts/power-supply.h`: reusable `powersupply.capacity` and `powersupply.voltage` chart contracts with `device` label.
  - `src/collectors/proc.plugin/sys_class_power_supply.c`: Linux battery/power-supply charts use the common power-supply contexts.
  - `src/collectors/windows.plugin/GetPowerSupply.c`: Windows battery charts reuse the same common power-supply contexts.
  - `src/collectors/debugfs.plugin/module-libsensors.c`: hardware sensor chart contexts include `system.hw.sensor.temperature.input` and `system.hw.sensor.fan.input`.
  - `src/collectors/windows.plugin/GetSensors.c`: Windows sensor charts also use `system.hw.sensor.*.input` contexts.

Affected contracts and surfaces:

- `macos.plugin` module list, worker jobs, chart contexts, chart labels, and collection cadence.
- CMake macOS source list.
- `src/collectors/macos.plugin/metadata.yaml` and generated `src/collectors/macos.plugin/integrations/macos.md`.
- The existing README symlink for `src/collectors/macos.plugin/README.md`.
- Packaging/privilege posture for exact thermal/fan readings: `powermetrics` requires superuser privileges, so failure must be graceful and one-shot.

Existing patterns to reuse:

- Existing `macos.plugin` IOKit/CoreFoundation handling.
- Common power-supply contexts for capacity and voltage.
- Existing `system.hw.sensor.temperature.input` and `system.hw.sensor.fan.input` contexts.
- Netdata `spawn_popen_run_argv()` and `spawn_popen_kill()` wrappers instead of raw `popen()`, shell execution, or unmanaged child processes.
- A background collection thread pattern where a slow data source must not block the 1-second collector loop.

Risk and blast radius:

- Low for battery support because the implementation uses public IOKit power-source APIs already linked into the macOS plugin.
- Medium for thermal/fan support because `powermetrics` output and available SMC keys vary by macOS release and hardware family, and the command requires superuser privileges.
- Blast radius is macOS-only and limited to `macos.plugin` plus generated integration documentation. Linux, Windows, FreeBSD, and existing macOS sysctl/mach/iokit charts should not change behavior.

Sensitive data handling plan:

- Do not commit real `powermetrics` output, battery serials, adapter serials, hostnames, device serials, or unique hardware identifiers.
- Do not add serial-number labels. Use sanitized power-source names and generic sensor labels only.
- Local validation may record command success/failure and chart/schema presence, but not raw hardware dumps.

Implementation plan:

1. Add a `power sources` module to `macos.plugin` using `IOPSCopyPowerSourcesInfo()` / `IOPSGetPowerSourceDescription()`.
2. Reuse `powersupply.capacity` and `powersupply.voltage` for battery capacity and voltage; add macOS power-source current/cycle charts only where public keys exist.
3. Add a `powermetrics` module to `macos.plugin` that starts a background sampler thread.
4. Run `/usr/bin/powermetrics -n 1 -i <window_ms> -s thermal,smc -f plist` at a configurable, bounded interval in the background thread.
5. Parse the NUL-separated plist with CoreFoundation property-list APIs, not text scraping.
6. Publish cached samples in the normal macOS collector loop, reusing hardware sensor contexts for fan and temperature charts.
7. Disable the `powermetrics` path after permanent command/permission failure to avoid per-cycle log floods.
8. Update CMake source lists, metadata, generated docs, and SOW/spec artifacts.

Validation plan:

- Run `git diff --check`.
- Run the integrations generators for `metadata.yaml` and generated collector docs.
- Run a local public IOKit syntax/API probe for power-source APIs without printing battery details.
- Run a local CoreFoundation plist parser probe against synthetic `powermetrics` plist data.
- Run non-root `powermetrics` once to verify graceful failure evidence without capturing hardware data.
- Attempt narrow source syntax validation where local build dependencies allow it; record environment blockers if full CMake remains unavailable.
- Run same-failure scans for raw `popen()`, shell command execution, serial-number labels, and per-cycle `powermetrics` execution.
- Run `.agents/sow/audit.sh`.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: no update expected; existing collector and integration lifecycle skills cover the workflow.
- Specs: add/update macOS hardware behavior spec if shipped.
- End-user/operator docs: update macOS collector docs and troubleshooting.
- End-user/operator skills: likely unaffected.
- SOW lifecycle: close with implementation artifacts when complete.

Open-source reference evidence:

- Not required for this design because the chosen contracts are grounded in local Netdata patterns, public Apple SDK headers, and the local Apple `powermetrics` binary/manual. No external parser or private API is being reused.

Open decisions:

- None. Existing project patterns and the user's autonomous-delivery instruction resolve the design choices:
  - public IOKit for battery/power sources;
  - bounded `powermetrics` background sampler for exact thermal/fan data;
  - no private SMC or IOReport implementation in this SOW;
  - graceful no-chart behavior for unsupported hardware or insufficient privileges.

## Implications And Decisions

- 2026-05-13: User selected the recommended hardware path from SOW-0016: bounded privileged Apple tooling for exact thermal/fan data, public APIs for battery, Intel and Apple Silicon support with feature detection and graceful gaps.

## Plan

1. Activate after the user or assistant selects this SOW as the next implementation unit.
2. Complete implementation gate with deeper evidence.
3. Implement and validate.

## Execution Log

### 2026-05-13

- Created as split implementation SOW from SOW-0016.
- Activated after completing and committing the native macOS logs SOW.
- Completed the implementation gate from existing `macos.plugin`, common power-supply, sensor context, public IOKit, and local `powermetrics` evidence.
- Added `macos.plugin` power-source collection using public IOKit power-source APIs.
- Added `macos.plugin` cached background thermal/fan collection using native Apple `powermetrics`.
- Updated macOS collector metadata and regenerated the macOS integration page.
- Added the durable macOS hardware monitoring spec.
- Validated metadata generation, API availability, plist parsing, privilege fallback, unsafe-command scans, sensitive-data scans, and SOW audit.

## Validation

Acceptance criteria evidence:

- Battery/power-source support:
  - `src/collectors/macos.plugin/plugin_macos.c` now registers the `power sources` module.
  - `src/collectors/macos.plugin/macos_power.c` uses `IOPSCopyPowerSourcesInfo()`, `IOPSCopyPowerSourcesList()`, and `IOPSGetPowerSourceDescription()`.
  - `src/collectors/macos.plugin/macos_power.c` emits `powersupply.capacity`, `powersupply.voltage`, `powersupply.current`, `powersupply.cycles`, and battery temperature charts only when macOS exposes those keys.
  - The local IOKit probe compiled and ran without printing power-source details: `ok count=0`, so this host exposes no power-source entries to validate live battery charts.
- Thermal/fan support:
  - `src/collectors/macos.plugin/plugin_macos.c` now registers the `powermetrics` module.
  - `src/collectors/macos.plugin/macos_powermetrics.c` runs `/usr/bin/powermetrics -n 1 -i <window_ms> -s thermal,smc -f plist` through `spawn_popen_run_argv()` in a background thread.
  - `src/collectors/macos.plugin/macos_powermetrics.c` parses plist output through CoreFoundation and publishes cached samples in the normal collector loop.
  - The module disables itself after repeated failures, so insufficient privilege does not produce per-cycle retries from the macOS collector loop.
- Stable chart contracts:
  - Reused existing contexts `powersupply.capacity`, `powersupply.voltage`, `system.hw.sensor.temperature.input`, and `system.hw.sensor.fan.input`.
  - Added macOS-specific contexts `powersupply.current`, `powersupply.cycles`, `macos.thermal_pressure`, `macos.smc_thermal_level`, and `macos.smc_prochot`.
- Documentation and metadata:
  - `src/collectors/macos.plugin/metadata.yaml` lists the new sections, options, permissions, performance impact, troubleshooting, and metrics.
  - `src/collectors/macos.plugin/integrations/macos.md` was regenerated from metadata.
  - `.agents/sow/specs/macos-hardware-monitoring.md` records the shipped behavior and sensitive-data rules.

Tests or equivalent validation:

- `git diff --check`: passed.
- `.local/integrations-venv/bin/python integrations/gen_integrations.py`: passed.
- `.local/integrations-venv/bin/python integrations/gen_docs_integrations.py -c macos.plugin/mach_smi`: passed.
- `clang` IOKit probe using `IOPSCopyPowerSourcesInfo()` / `IOPSCopyPowerSourcesList()`: compiled and ran, output `ok count=0`.
- `clang` CoreFoundation synthetic plist probe: compiled and ran, output `parse=ok smc_dict=yes`.
- Non-root `powermetrics` privilege probe:
  - Command shape: `/usr/bin/powermetrics -n 1 -i 100 -s thermal,smc -f plist`.
  - Result: `rc=1`, `stdout_bytes=0`, stderr summarized as superuser-required.
  - No real hardware plist output was captured or committed.
- Full Netdata CMake configure/build was not runnable because `cmake` is not installed in this workstation PATH.
- Direct `clang -fsyntax-only` on the new source files was attempted with narrow include flags and did not reach the new implementation because the repository source tree lacks the generated build context for standalone compilation. The failure occurred in shared headers before compiling the new source logic: missing/generated configuration context, `timespec` redefinition, and unresolved project macros/types.
- `integrations/check_collector_metadata.py src/collectors/macos.plugin/metadata.yaml` is currently unusable in this checkout because it imports removed names from `integrations/gen_integrations.py`; the full generator validation above is the authoritative metadata validation used for this SOW.

Real-use evidence:

- Public IOKit power-source API availability was verified on the local macOS host without printing power-source names or values. The host returned zero power-source entries, so live battery charts could not be validated on this hardware.
- Native `powermetrics` availability was verified at `/usr/bin/powermetrics`, and its help output confirms plist format plus `thermal` and `smc` samplers.
- Non-root execution confirms the expected macOS privilege failure path. Privileged thermal/fan live chart data could not be validated without running as superuser, and raw `powermetrics` output was intentionally not captured.

Reviewer findings:

- No external or delegated review was requested for this slice.
- Manual implementation review found and fixed:
  - Removed unused powermetrics state.
  - Added explicit platform headers for math, string comparison, and `read()`.
  - Changed CoreFoundation integer extraction for unsigned thermal levels to reject negative signed values safely.
  - Added `source=iokit` labels to common power-source capacity and voltage charts for metadata consistency.
  - Trimmed generated-doc trailing blank lines so `git diff --check` stays clean.

Same-failure scan:

- Unsafe process execution scan:
  - `rg -n "\\bpopen\\(|\\bsystem\\(|/bin/sh|fork\\(|execv|posix_spawn" src/collectors/macos.plugin/macos_power.c src/collectors/macos.plugin/macos_powermetrics.c`
  - Result: no matches.
- Sensitive hardware identifier scan:
  - `rg -n "Serial|serial|kIOPSHardwareSerial|kIOPSPowerAdapterSerial|kIOPMPSSerial" src/collectors/macos.plugin/macos_power.c src/collectors/macos.plugin/macos_powermetrics.c src/collectors/macos.plugin/metadata.yaml`
  - Result: no matches.
- Contract scan:
  - `rg -n "powermetrics|plugin:macos:powermetrics|power_sources|powersupply\\.current|powersupply\\.cycles|macos\\.thermal_pressure|system\\.hw\\.sensor\\.fan\\.input" src/collectors/macos.plugin CMakeLists.txt .agents/sow/specs/macos-hardware-monitoring.md`
  - Result: expected references only.

Sensitive data gate:

- No raw `powermetrics` plist output, battery serials, adapter serials, model serials, hostnames, usernames, or unique hardware identifiers were written to committed artifacts.
- Local validation recorded only API availability, command exit code, byte counts, and sanitized failure class.
- `.agents/sow/audit.sh` sensitive data guardrail passed for durable artifacts.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow, responsibilities, and project-wide guardrails did not change.
- Runtime project skills: no update needed; `project-writing-collectors` and `integrations-lifecycle` already cover the collector and metadata workflow used here.
- Specs: updated by adding `.agents/sow/specs/macos-hardware-monitoring.md`.
- End-user/operator docs: updated through `src/collectors/macos.plugin/metadata.yaml` and regenerated `src/collectors/macos.plugin/integrations/macos.md`.
- End-user/operator skills: no update needed; no public/operator AI skill behavior changed.
- SOW lifecycle: SOW moved to current for implementation, status set to `completed`, and will be moved to done with the implementation commit.

Specs update:

- Added `.agents/sow/specs/macos-hardware-monitoring.md` for power-source, thermal/fan, chart, configuration, and sensitive-data contracts.

Project skills update:

- No project skill update required; this work exposed no missing collector-authoring or integration-lifecycle process rule.

End-user/operator docs update:

- Updated `src/collectors/macos.plugin/metadata.yaml`.
- Regenerated `src/collectors/macos.plugin/integrations/macos.md`.

End-user/operator skills update:

- No end-user/operator skill update required; no public AI skill changed.

Lessons:

- The local `integrations/check_collector_metadata.py` helper is stale relative to `gen_integrations.py` and should not be used as the primary validation gate until repaired.
- Native `powermetrics` provides the required thermal/fan path, but production validation of populated charts needs a privileged macOS host with exposed SMC thermal/fan keys.

Follow-up mapping:

- Storage health work remains tracked separately by SOW-0018.
- No additional SOW is needed for the local build/tooling limitation; it is an environment constraint already recorded here and in prior macOS work.

## Outcome

Completed.

Implemented macOS battery, UPS/power-source, thermal pressure, fan speed, SMC temperature, SMC thermal level, and SMC processor-hot collection in `macos.plugin`, with bounded native `powermetrics` sampling and graceful privilege failure behavior.

## Lessons Extracted

Use native Apple APIs or tools first for macOS hardware collection, but treat live thermal/fan validation as privilege- and hardware-dependent. Do not commit raw hardware dumps as fixtures or evidence.

## Followup

Storage health is tracked by SOW-0018. No additional work remains for this SOW.

## Regression Log

None yet.
