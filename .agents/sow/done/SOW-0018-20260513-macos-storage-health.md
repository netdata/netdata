# SOW-0018 - macOS S.M.A.R.T. and NVMe storage health

## Status

Status: completed

Sub-state: completed again on 2026-05-15 after Mac mini storage validation, native NVMe retry bounding, and storage documentation corrections.

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

Resolved implementation constraints:

- Generic S.M.A.R.T. collection is provided through the existing `smartctl` go.d collector and now documented as macOS-supported when `smartmontools` is installed.
- Native NVMe health collection belongs in `macos.plugin` because macOS exposes the relevant SMART data through IOKit, not through the Linux/BSD `nvme-cli` contract.
- Native IOKit exposes only a subset of the existing Linux/BSD `nvme.*` contexts by name; unsupported thermal-management contexts are intentionally not emitted.

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

Status: complete

Problem / root-cause model:

- macOS storage health is missing from the supported macOS contract for two different reasons.
- Generic S.M.A.R.T. support already exists in the `smartctl` go.d collector, but its metadata and operator guidance only advertise Linux/BSD support.
- NVMe health support already exists in the `nvme` go.d collector, but that implementation depends on `nvme-cli`, while macOS exposes native NVMe SMART data through IOKit.
- The right implementation shape is therefore hybrid: reuse the existing `smartctl` collector for generic S.M.A.R.T. where `smartmontools` is installed, and add native macOS NVMe SMART sampling to `macos.plugin` while reusing compatible `nvme.*` contexts.

Evidence reviewed:

- `src/go/plugin/go.d/collector/smartctl/collector.go:20` registers the `smartctl` collector with `update_every=10`; `collector.go:34` through `collector.go:39` set the default timeout, scan interval, device poll interval, low-power policy, selector, and sequential scan behavior.
- `src/go/plugin/go.d/collector/smartctl/init.go:44` through `init.go:49` already route all non-Windows platforms through `ndsudo`, so Darwin follows the existing privileged non-Windows execution path.
- `src/go/plugin/go.d/collector/smartctl/exec.go:40` through `exec.go:63` execute `smartctl-json-scan`, `smartctl-json-scan-open`, and `smartctl-json-device-info` through `ndsudo`; `exec.go:140` through `exec.go:143` keep SMART health-result exit bits while treating fatal command/open errors separately.
- `src/collectors/utils/ndsudo.c:103` through `ndsudo.c:124` already allow-list the required `smartctl` commands.
- `src/go/plugin/go.d/collector/nvme/collector.go:19` through `collector.go:24` register the existing NVMe collector at `update_every=10`; `collector.go:38` sets bounded device listing to every 10 minutes.
- `src/go/plugin/go.d/collector/nvme/init.go:14` through `init.go:19` route all non-Windows platforms through `ndsudo`, and `exec.go:150` through `exec.go:165` call `nvme-list` and `nvme-smart-log`.
- `src/collectors/utils/ndsudo.c:207` through `ndsudo.c:220` allow-list Linux-style `nvme list` and `nvme smart-log`, not native macOS IOKit calls.
- `src/go/plugin/go.d/collector/nvme/charts.go:51` through `charts.go:168` define the existing reusable `nvme.*` chart contexts for endurance, spare, temperature, transferred data, power cycles, power-on time, critical warnings, unsafe shutdowns, media errors, and error log entries.
- `src/go/plugin/go.d/collector/nvme/collect.go:52` through `collect.go:77` define the unit conversions and critical-warning bit mapping that native macOS NVMe sampling must match where fields are exposed by IOKit.
- The macOS SDK header `IOKit/storage/nvme/NVMeSMARTLibExternal.h` exposes `kIOPropertyNVMeSMARTCapableKey`, `IONVMeSMARTInterface`, `SMARTReadData`, `GetIdentifyData`, `NVMeSMARTData`, and `NVMeIdentifyControllerStruct`.
- The macOS SDK header `IOKit/storage/nvme/NVMeSMARTLibExternal.h` exposes NVMe fields equivalent to the existing Netdata contexts for critical warning, composite temperature, available spare, percent used, data units read/written, power cycles, power-on hours, unsafe shutdowns, media errors, and error-log entries.
- The same SDK header does not expose named fields for the Linux `nvme-cli` thermal-management transition/time contexts through `NVMeSMARTData`; those contexts must not be synthesized from unknown reserved bytes.
- Local CLI probe on 2026-05-13 found no `smartctl` binary and no `nvme` binary in `PATH`, so local real-use validation cannot rely on those external tools.
- Local compile probe on 2026-05-13 confirmed that macOS code can include the native NVMe SMART header and link against CoreFoundation and IOKit.
- Local IOKit registry probe on 2026-05-13 found no NVMe SMART-capable services on this host, so native NVMe real-use validation must be compile/API validation plus no-device runtime behavior, not a real NVMe health sample.

Affected contracts and surfaces:

- `macos.plugin` source, module list, cleanup path, build source list, metadata, generated integration documentation, and macOS hardware/storage specs.
- `go.d` `smartctl` metadata and generated documentation, to advertise macOS support and macOS installation guidance for `smartmontools`.
- Existing `nvme.*` metric contexts and `src/health/health.d/nvme.conf`; native macOS NVMe critical warnings should keep the `device` label so the existing alert remains meaningful.
- No `ndsudo` changes are needed for native macOS NVMe because IOKit calls are in-process and do not execute external commands.
- No `nvme` go.d collector code changes are needed because its `nvme-cli` dependency remains the right implementation for Linux/BSD tooling.

Existing patterns to reuse:

- Existing `smartctl` go.d scan/poll cadence and `ndsudo` allow-list discipline for generic S.M.A.R.T.
- Existing `nvme.*` contexts, labels, priorities, dimensions, and unit conversions where IOKit exposes equivalent fields.
- Existing `macos.plugin` module registration pattern in `src/collectors/macos.plugin/plugin_macos.c`.
- Existing `macos.plugin` chart creation and label style from `macos_power.c` and `macos_powermetrics.c`.
- Existing macOS collector documentation and generated integration artifact flow.

Risk and blast radius:

- Medium.
- The native NVMe code is macOS-specific and only compiled for macOS, limiting non-macOS blast radius.
- The largest behavioral risk is misrepresenting native IOKit fields as existing `nvme.*` contexts. The implementation must emit only fields with clear semantic equivalence and leave unsupported Linux-only thermal-management fields absent.
- The second risk is cardinality or hot-path cost. Discovery must be bounded and rate-limited; SMART reads must not traverse the IOKit registry in the one-second hot path.
- The third risk is exposing hardware identifiers. The implementation may use model number as the existing NVMe collector does, but must not publish serial numbers or commit raw device dumps.

Sensitive data handling plan:

- Do not commit real serial numbers, registry paths, UUIDs, hardware dumps, hostnames, usernames, or raw `ioreg` output.
- Native NVMe chart identity uses sanitized ordinal names such as `nvme0`, not IORegistry paths or serial numbers.
- Native NVMe labels may include `model_number` to match the existing NVMe collector contract, but must not include serial numbers.
- Validation evidence records sanitized command outcomes such as compile success, service count, and failure class only.
- SOWs, specs, documentation, project skills, agent instructions, code comments, and fixtures are treated as public artifacts.

Implementation plan:

1. Add a native `macos.plugin` NVMe SMART module using IOKit `IONVMeSMARTInterface`.
2. Discover NVMe SMART-capable services through the documented IORegistry property, cache retained services, cap devices, and rescan on a configurable interval.
3. Read native SMART data on a configurable sample interval, defaulting to the existing NVMe collector's 10-second cadence so the existing critical-warning alert window remains valid.
4. Emit compatible `nvme.*` charts for endurance, spare, composite temperature, transferred data, power cycles, power-on time, unsafe shutdowns, critical warnings, media errors, and error log entries.
5. Do not emit Linux `nvme-cli` contexts for warning/critical temperature time or thermal-management transition/time fields because the native Apple SMART struct does not expose those fields by name.
6. Update `smartctl` metadata and generated docs to include macOS support and macOS `smartmontools` prerequisites.
7. Update `macos.plugin` metadata, generated docs, and specs for native NVMe SMART behavior.

Validation plan:

- `git diff --check`.
- Compile and run sanitized local IOKit NVMe SMART API probes.
- Compile touched macOS C source as far as local headers/dependencies allow; record environment limitations if full CMake remains unavailable.
- Run narrow Go tests for `smartctl` and `nvme` collectors if Go source behavior changes; otherwise record why metadata-only changes do not need collector unit tests.
- Run integration metadata/doc generators for modified collector metadata.
- Search for raw serial/identifier leakage in new SOW/spec/docs/source changes.
- Search for same-failure patterns such as per-cycle registry scans, shell execution, and unsupported `nvme.*` context fabrication.
- Run `.agents/sow/audit.sh` and record pre-existing unrelated findings separately.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: likely unaffected because this follows existing collector and integration lifecycle rules without adding a new authoring workflow.
- Specs: add macOS storage-health contract for native NVMe SMART and smartctl macOS support.
- End-user/operator docs: update generated integration docs for `macos.plugin` and `smartctl`.
- End-user/operator skills: likely unaffected.
- SOW lifecycle: current SOW moved from open to in-progress before implementation.

Open-source reference evidence:

- Not needed for the core implementation. The authoritative API evidence is the local Apple SDK header shipped with Xcode/Command Line Tools; no third-party implementation is required.

Open decisions:

- None. The user requested autonomous delivery using existing project patterns; the code-location decision follows the evidence above.

## Implications And Decisions

- 2026-05-13: User selected the recommended hybrid storage-health strategy from SOW-0016.

## Plan

1. Implement native macOS NVMe SMART collection in `macos.plugin`.
2. Update `smartctl` macOS metadata and docs.
3. Update macOS metadata, generated docs, and storage-health spec.
4. Validate and close.

## Execution Log

### 2026-05-13

- Created as split implementation SOW from SOW-0016.
- Activated the SOW and completed the implementation gate before code changes.
- Added native macOS NVMe SMART collection to `macos.plugin`.
- Added `macos.plugin` module registration, cleanup, and build-file wiring for `nvme smart`.
- Updated `smartctl` metadata and generated documentation to include macOS support and macOS `smartmontools` guidance.
- Updated macOS collector metadata and generated documentation with native NVMe SMART behavior, configuration, troubleshooting, and metrics.
- Added `.agents/sow/specs/macos-storage-health.md`.

## Validation

Acceptance criteria evidence:

- macOS S.M.A.R.T. support is delivered through the existing `smartctl` go.d collector by documenting macOS as a supported platform when `smartmontools` 7.0+ is installed.
- macOS NVMe health support is delivered through native IOKit in `src/collectors/macos.plugin/macos_nvme.c`.
- Existing storage-health chart contracts are reused where semantically compatible: native macOS NVMe emits `nvme.device_estimated_endurance_perc`, `nvme.device_available_spare_perc`, `nvme.device_composite_temperature`, `nvme.device_io_transferred_count`, `nvme.device_power_cycles_count`, `nvme.device_power_on_time`, `nvme.device_unsafe_shutdowns_count`, `nvme.device_critical_warnings_state`, `nvme.device_media_errors_rate`, and `nvme.device_error_log_entries_rate`.
- Unsupported Linux `nvme-cli` thermal-management contexts are not emitted because the public Apple `NVMeSMARTData` struct does not expose equivalent named fields.
- Discovery is bounded by `MACOS_NVME_MAX_DEVICES=32`, rescan interval `discovery every=300s`, and sample interval `sample every=10s`.
- Native NVMe chart identity uses sanitized ordinal names such as `nvme0`; labels include `device`, `model_number`, and `source=iokit`, with no serial-number label.
- Existing `nvme_device_critical_warnings_state` alert remains applicable because the native module reuses `nvme.device_critical_warnings_state` and preserves the `device` label.

Tests or equivalent validation:

- `git diff --check`: passed.
- `.local/integrations-venv/bin/python integrations/gen_integrations.py`: passed.
- `.local/integrations-venv/bin/python integrations/gen_docs_integrations.py -c macos.plugin/mach_smi`: passed.
- `.local/integrations-venv/bin/python integrations/gen_docs_integrations.py -c go.d.plugin/smartctl`: passed.
- Isolated C syntax check for `macos_nvme.c` with Netdata stubs and native macOS headers: passed.
- Native IOKit registry probe compiled and ran: `ok nvme_smart_capable=0`.
- Full CMake build validation was not available because `cmake` is not installed in this environment.
- Go collector tests were not available because `go` is not installed in this environment.

Real-use evidence:

- Local host exposes no NVMe SMART-capable services through IOKit, so native NVMe real-use validation is limited to no-device behavior and API compilation.
- Local `smartctl` and `nvme` binaries are not installed, so real device polling through external storage tools could not be validated on this host.
- The native path does not depend on external tools; local IOKit probe confirmed the API is available and that the collector should create no NVMe health charts on this host.

Reviewer findings:

- No separate reviewer was requested for this SOW.
- Self-review checked the implementation against existing `smartctl`, `nvme`, and `macos.plugin` patterns; no unresolved implementation findings remain.

Same-failure scan:

- Searched new native NVMe code and related docs/spec/SOW for shell execution, `popen`, `spawn`, `system()`, external `nvme-cli` usage, raw serial handling, IORegistry path labels, and serial-number labels.
- Result: native NVMe code uses in-process IOKit only; no shell/external command path was added.
- Result: new native NVMe labels do not include serial numbers, IORegistry paths, UUIDs, or raw registry entry names.
- Result: `IORegistryCreateIterator` is only used behind the 300-second discovery interval, not in the one-second macOS collector hot path.
- Result: unsupported Linux `nvme-cli` thermal-management contexts are documented as intentionally absent from the native macOS module.

Sensitive data gate:

- New durable artifacts use sanitized evidence only.
- No raw `smartctl`, `ioreg`, NVMe SMART dumps, serial numbers, hostnames, usernames, UUIDs, or private paths were committed.
- `.agents/sow/audit.sh` sensitive-data guardrail passed for durable artifacts.

Artifact maintenance gate:

- AGENTS.md: not updated; no project-wide workflow or guardrail changed.
- Runtime project skills: not updated; this followed existing collector and integrations workflows without adding a new HOW-to-work-here rule.
- Specs: added `.agents/sow/specs/macos-storage-health.md`.
- End-user/operator docs: updated generated integration docs for `macos.plugin` and `smartctl`.
- End-user/operator skills: not updated; no public/operator AI skill behavior changed.
- SOW lifecycle: activated from open to in-progress before implementation; completed and will move to `done/` with the feature commit.

Specs update:

- Added `.agents/sow/specs/macos-storage-health.md` to record the macOS S.M.A.R.T. and native NVMe SMART contract.

Project skills update:

- No project skill update needed; existing `project-writing-collectors` and `integrations-lifecycle` skills covered the workflow.

End-user/operator docs update:

- Updated `src/collectors/macos.plugin/metadata.yaml` and regenerated `src/collectors/macos.plugin/integrations/macos.md`.
- Updated `src/go/plugin/go.d/collector/smartctl/metadata.yaml` and regenerated `src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md`.

End-user/operator skills update:

- No end-user/operator skill update needed; no public skill workflow, command, or schema changed.

Lessons:

- Reusing existing storage-health contexts is safe only for fields with direct API equivalence; native macOS NVMe must leave unsupported Linux `nvme-cli` contexts absent rather than fabricating them from reserved SMART bytes.

Follow-up mapping:

- No deferred items remain.

## Outcome

Completed.

- macOS generic S.M.A.R.T. support is documented through the existing `smartctl` collector.
- Native macOS NVMe SMART support is implemented in `macos.plugin` using IOKit and compatible `nvme.*` contexts.
- Metadata, generated docs, storage-health spec, and SOW lifecycle artifacts are updated.

## Lessons Extracted

- Public Apple NVMe SMART exposes enough fields for core NVMe health, but not the full Linux `nvme-cli` thermal-management metric surface. Context reuse must stay field-by-field, not name-by-name.

## Followup

None.

## Regression Log

### Regression - 2026-05-15 - Mac mini storage validation

What broke:

- The completed SOW claimed native macOS NVMe SMART support, but local installed validation on a Mac mini shows the internal Apple storage does not expose the expected native NVMe SMART user client to Netdata.
- The completed SOW also treated `smartctl` macOS support as operator-ready, but the installed `ndsudo` helper cannot find the Homebrew `smartctl` path on Apple Silicon.

Evidence:

- Installed collector logs report that native NVMe SMART data cannot be read through IOKit, and NVMe health charts will appear only when macOS exposes readable NVMe SMART data.
- Homebrew `smartctl` is installed and reports version 7.5.
- Installed `ndsudo --test smartctl-json-scan-open` as the `netdata` user fails with `smartctl : not available in PATH`.
- `smartctl --scan-open` sees an Apple internal NVMe path but fails to open it through the smartmontools path.
- IOKit registry evidence shows Apple internal NVMe controller classes on this host, but the native SMART collector does not produce NVMe health charts.

Why previous validation missed it:

- Earlier validation ran on a host without `smartctl` installed and without readable NVMe SMART-capable services, so the implementation was validated mainly by compile/API checks and no-device behavior.
- It did not validate Apple Silicon Homebrew command discovery through the installed setuid `ndsudo` path.
- It did not classify Apple internal ANS NVMe storage separately from generic NVMe devices that expose standard SMART paths.

User decision:

- 2026-05-15: user accepted fixing non-logs regressions first, then requested identification of the Mac mini storage type and how it should be monitored.

Repair and investigation plan:

- Identify the Mac mini storage stack using sanitized local system evidence.
- Determine whether the internal Apple storage should be monitored through native NVMe SMART, smartctl, generic disk I/O, powermetrics disk data, IOReport, or some combination.
- Fix only safe, production-quality issues found in the generic storage path; do not add unsafe setuid search paths for user-writable Homebrew binaries.
- Update specs/docs/SOW with the resulting storage monitoring contract and validation evidence.

Investigation results:

- Local storage identity:
  - Internal storage is an Apple internal SSD model `APPLE SSD AP0512Z`.
  - Capacity is about 500.3 GB.
  - Primary whole disk is `disk0`.
  - macOS reports protocol `Apple Fabric`, fixed internal solid-state media, 4096-byte device block size, and TRIM support.
- Local health API evidence:
  - `system_profiler SPNVMeDataType` reports the device SMART status as `Verified`.
  - `diskutil info disk0` reports SMART status as `Not Supported`.
  - IOKit exposes Apple internal NVMe classes including `AppleANS3CGv2Controller`, `IOEmbeddedNVMeBlockDevice`, and an `AppleNVMeSMARTUserClient` child.
  - IOKit also exposes an `NVMe SMART Capable` marker on the block device, but a scratch probe could not open the native SMART plugin interface for the service.
  - `smartctl --json --scan-open` finds one NVMe device through an IOService path, but `smartctl --json --all ... --device nvme` returns exit status `2` with `IOCreatePlugInInterfaceForService failed`.
  - `powermetrics -s disk -f plist` exposes aggregate read/write throughput counters, not storage-health data.
- Local Netdata evidence:
  - Generic disk I/O charts are present for `disk0`: bandwidth, operations, utilization, and total I/O time.
  - Filesystem space and inode charts are present for the mounted APFS volumes.
  - Per-application disk logical I/O charts are present through the apps collector.
  - Native NVMe health charts are absent, which is correct for this host because detailed public NVMe health fields are not readable through IOKit.
  - The Homebrew `smartctl` binary is installed, but the default Apple Silicon Homebrew prefix is not a safe setuid helper search path because it is user/admin-owned and group-writable on this host.

Repair outcome:

- Updated native macOS NVMe discovery to require a successful native SMART read before adding a device to the active NVMe health set.
- Added a runtime guard that removes NVMe devices from the active set after repeated read failures so an unreadable Apple internal device does not keep retrying every ten seconds.
- Kept detailed NVMe health charts absent on this Mac mini instead of fabricating metrics from unavailable fields.
- Did not add the Homebrew prefix to `ndsudo` search paths because that would allow a setuid-root helper to find binaries from a user-writable package-manager location.
- Updated macOS storage specs, macOS integration metadata/docs, and smartctl metadata/docs to state the readable-user-client requirement and the trusted-path requirement for `smartctl`.

How this Mac mini should be monitored:

- Monitor the internal Apple Fabric SSD with existing generic disk and filesystem charts:
  - `disk.disk0`
  - `disk_ops.disk0`
  - `disk_util.disk0`
  - `disk_iotime.disk0`
  - APFS disk space and inode charts
  - per-application disk logical I/O charts
- Do not expect detailed NVMe health charts on this host unless macOS exposes a readable native SMART user client in a future OS/hardware combination.
- Do not expect the `smartctl` collector to provide internal Apple SSD health on this host; smartmontools can see the IOService-backed NVMe device but cannot open it.
- A coarse `system_profiler` SMART-status chart is not implemented in this repair. It would be a separate product decision because it is an external command, exposes only a coarse health state, and conflicts with `diskutil` on this host.

Regression validation:

- `cmake --build ./build-macmini --parallel 10 --target netdata`: passed.
- Regenerated integration artifacts using a fresh arm64 local venv because the existing `.local/integrations-venv` contained an x86_64 wheel incompatible with this Mac mini.
- `git diff --check`: passed.
- Scratch native NVMe probe under `.local/macos-storage-probes/` produced sanitized JSON only: class names, IOReturn codes, counts, hashes, and booleans; no serials, raw SMART data, or IORegistry paths were written to durable artifacts.
- Installed changed `netdata` into `/opt/netdata` and restarted `system/com.github.netdata`.
- Runtime API check: `http://localhost:19999/api/v1/info` returned macOS from the local Agent.
- Runtime chart check after restart:
  - `disk.disk0`, `disk_ops.disk0`, `disk_util.disk0`, and `disk_iotime.disk0` are present.
  - NVMe health charts are absent on this host.
- Runtime collector-log check after waiting across several ten-second NVMe sample periods showed at most one bounded native NVMe read-failure class message for the unreadable Apple internal device, not continuous retries.
- Installed `ndsudo --test smartctl-json-scan-open` as the `netdata` user still reports `smartctl : not available in PATH`; this is intentionally not fixed by adding Homebrew paths because the local Homebrew prefix is not root-controlled.
- Direct `smartctl` validation with sanitized output:
  - `smartctl --json --scan-open` found one NVMe IOService-backed device.
  - `smartctl --json --all ... --device nvme` returned exit status `2` with `IOCreatePlugInInterfaceForService failed`.
- `.agents/sow/audit.sh`: this SOW is status/directory consistent. The audit still reports unrelated pre-existing repository hygiene issues: one older SOW status/directory mismatch and non-project skill classification warnings.

Same-failure search:

- `rg -n "IORegistryCreateIterator|IOCreatePlugInInterfaceForService|SMARTReadData|smartctl|/opt/homebrew" src/collectors/macos.plugin/macos_nvme.c src/collectors/utils/ndsudo.c src/go/plugin/go.d/collector/smartctl .agents/sow/specs/macos-storage-health.md`: verified that no Homebrew path was added to `ndsudo`, native NVMe still uses in-process IOKit, and docs/specs contain the trusted-path warning.
- `curl ... /api/v1/charts | jq ... nvme`: no NVMe health charts were emitted on this Mac mini after the patch.

Artifact maintenance gate for regression:

- AGENTS.md: no update needed; workflow, responsibilities, and project-wide guardrails did not change.
- Runtime project skills: no update needed; `project-writing-collectors` and `integrations-lifecycle` already cover the workflow used.
- Specs: updated `.agents/sow/specs/macos-storage-health.md`.
- End-user/operator docs: updated `src/collectors/macos.plugin/metadata.yaml`, generated `src/collectors/macos.plugin/integrations/macos.md`, `src/go/plugin/go.d/collector/smartctl/metadata.yaml`, and generated `src/go/plugin/go.d/collector/smartctl/integrations/s.m.a.r.t..md`.
- End-user/operator skills: no update needed; no public/operator AI skill behavior changed.
- SOW lifecycle: reopened from `done/`, marked completed after validation, and moved back to `done/` with the repair.

Regression lessons:

- Apple internal Apple Fabric storage is not equivalent to a generic NVMe device even when IORegistry contains NVMe class names and a SMART-capable marker.
- Detailed native NVMe health requires successful access to the native SMART user client, not just service discovery.
- The setuid `ndsudo` helper must not search user-writable package-manager prefixes to find tools such as Homebrew `smartctl`.
