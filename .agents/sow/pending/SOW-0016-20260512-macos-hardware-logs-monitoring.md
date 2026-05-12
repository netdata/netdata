# SOW-0016 - macOS hardware and logs monitoring

## Status

Status: open

Sub-state: user decisions recorded on 2026-05-13; implementation split into dedicated pending SOWs for macOS power/sensors/fans, macOS storage health, and native macOS logs. This umbrella SOW remains planning-only.

## Requirements

### Purpose

Add macOS monitoring coverage for hardware health and logs so macOS nodes expose useful battery, thermal, fan, storage-health, NVMe, and log-exploration data with the same production-quality expectations as existing Linux and Windows collectors.

### User Request

The user wants Netdata to support monitoring:

- Battery
- Thermal sensors
- Fans
- S.M.A.R.T.
- NVMe disks
- macOS logs, similar to how Netdata monitors Linux and Windows logs

### Assistant Understanding

Facts:

- `macos.plugin` already exists as an internal C collector with three modules: `sysctl`, `mach_smi`, and `iokit` (`src/collectors/macos.plugin/plugin_macos.c:15-18`).
- The existing IOKit module already links against CoreFoundation/IOKit and reads disk I/O, mounted filesystems, inodes, and network interfaces through IOKit, `getmntinfo()`, and `getifaddrs()` (`src/collectors/macos.plugin/macos_fw.c:5-13`, `src/collectors/macos.plugin/macos_fw.c:24-36`, `src/collectors/macos.plugin/macos_fw.c:106-116`).
- The existing Linux sensors collector is explicitly Linux-only via `//go:build linux` (`src/go/plugin/go.d/collector/sensors/collector.go:1-4`), so it cannot cover macOS thermal sensors or fans without a separate macOS implementation.
- `smartctl` already exists as a go.d collector, but its current platform metadata says Linux/BSD (`src/go/plugin/go.d/collector/smartctl/metadata.yaml:36-37`) and its non-Windows path uses `ndsudo` (`src/go/plugin/go.d/collector/smartctl/init.go:44-53`).
- `nvme` already exists as a go.d collector, but its current platform metadata says Linux/BSD (`src/go/plugin/go.d/collector/nvme/metadata.yaml:28-29`) and its non-Windows path uses `ndsudo` (`src/go/plugin/go.d/collector/nvme/init.go:14-23`).
- `ndsudo` already has allow-listed commands for `smartctl --json --scan`, `smartctl --json --scan-open`, `smartctl --json --all ...`, `nvme list --output-format=json`, and `nvme smart-log ... --output-format=json` (`src/collectors/utils/ndsudo.c:103-124`, `src/collectors/utils/ndsudo.c:207-220`).
- Linux and Windows logs are exposed through Function plugins in the `logs` category, not as metrics. `systemd-journal.plugin` registers `systemd-journal` as a logs Function (`src/collectors/systemd-journal.plugin/systemd-main.c:75-95`); `windows-events.plugin` registers `windows-events` similarly (`src/collectors/windows-events.plugin/windows-events.c:1357-1374`).
- Apple's public logging documentation states unified logging is available on macOS 10.12+ and is viewed with Console, the `log` command-line tool, or Xcode. The local `log(1)` manual confirms `log show` supports JSON and NDJSON output and predicate filtering.
- Apple's public IOKit power-source APIs expose `IOPSCopyPowerSourcesInfo()` and `IOPSGetPowerSourceDescription()` for readable power-source dictionaries.
- The local macOS `powermetrics --help` output lists samplers for `battery`, `thermal`, `smc`, `nvme_ssd`, and `io_throttle_ssd`. The `powermetrics(1)` manual describes the tool as a sampler for CPU, wakeups, power, thermal, and platform-specific hardware data.
- The local macOS SDK exposes public ATA SMART and NVMe SMART headers under IOKit storage headers. It does not expose public SMC or IOReport headers in the SDK scan performed for this SOW.

Inferences:

- Battery monitoring is likely the lowest-risk hardware item because Apple's public power-source APIs exist and can be used inside or adjacent to `macos.plugin`.
- Exact temperature sensor and fan RPM support is the highest-risk hardware item. macOS exposes coarse thermal pressure through supported tools/APIs, but exact fan and thermal sensor readings typically require either `powermetrics` sampling or private AppleSMC/IOReport mechanisms.
- S.M.A.R.T. and NVMe support should not start from scratch without a decision. Netdata already has `smartctl` and `nvme` collectors, but macOS may be better served by either direct CLI execution, `ndsudo`, or native IOKit storage SMART APIs depending on dependency and permission policy.
- macOS logs should be modeled as a Function plugin, not a metric collector, because the Linux and Windows precedents are interactive log explorers and because logs are high-cardinality sensitive data.

Unknowns:

- Final implementation order across the three split SOWs.
- Whether exact sensor support through `powermetrics` is enough for the first hardware SOW, or whether a later private SMC/IOReport path is worth the maintenance risk.
- Whether requiring `smartmontools` is acceptable for macOS S.M.A.R.T. after the storage SOW validates real macOS behavior.
- Whether macOS logs should expose all unified-log fields or a curated cross-platform subset aligned with systemd-journal and Windows Events.

### Acceptance Criteria

- Battery metrics exist on macOS nodes where battery/power-source data is available, with stable chart contexts, units, labels, and documented missing-data behavior on desktops without batteries.
- Thermal and fan monitoring follows the selected sensor API policy and records clearly whether it provides exact readings, thermal pressure only, or an opt-in privileged path.
- S.M.A.R.T. monitoring works on supported macOS storage devices through the selected strategy, with existing `smartctl` metric contexts reused where practical.
- NVMe health monitoring works on supported macOS NVMe devices through the selected strategy, with existing `nvme` metric contexts reused where practical.
- macOS logs are available from the Netdata Logs tab through a Function registered in the `logs` category, with bounded query size, predicate/search support, sensitive-data access flags, cancellation/timeout handling, and no metrics derived from logs in the collection loop.
- Integration metadata, config schemas, stock configs, health alerts, README/generated docs, and logs catalog metadata are updated consistently for every shipped collector or Function.
- Validation includes macOS build/test evidence, Go collector tests where go.d code changes, Function schema validation for logs, and real-use checks on representative macOS hardware.

## Analysis

Sources checked:

- `AGENTS.md` SOW/collector requirements.
- `.agents/skills/project-writing-collectors/SKILL.md`.
- `.agents/skills/integrations-lifecycle/SKILL.md`.
- `.agents/sow/specs/sensitive-data-discipline.md`.
- Existing SOWs under `.agents/sow/current/` and `.agents/sow/pending/`.
- `src/collectors/macos.plugin/plugin_macos.c`.
- `src/collectors/macos.plugin/macos_fw.c`.
- `src/collectors/macos.plugin/macos_sysctl.c`.
- `src/collectors/macos.plugin/macos_mach_smi.c`.
- `src/collectors/macos.plugin/metadata.yaml`.
- `src/go/plugin/go.d/collector/sensors/`.
- `src/go/plugin/go.d/collector/smartctl/`.
- `src/go/plugin/go.d/collector/nvme/`.
- `src/collectors/utils/ndsudo.c`.
- `src/collectors/systemd-journal.plugin/`.
- `src/collectors/windows-events.plugin/`.
- `integrations/logs/metadata.yaml`.
- Local `log(1)` and `powermetrics(1)` manuals.
- Apple Developer documentation for unified logging and IOKit power sources.
- Local macOS SDK header scan for IOKit power, ATA SMART, NVMe SMART, SMC, and IOReport interfaces.

Current state:

- macOS has a native internal metrics collector, but it does not currently expose battery, exact thermal sensors, fans, SMART health, or NVMe health.
- Linux has power-supply and sensor coverage through Linux-specific `/sys` sources and the Linux-only `sensors` go.d collector.
- Storage health coverage exists in go.d (`smartctl`, `nvme`), but the implementation and metadata are not currently macOS-ready.
- Log exploration exists for Linux systemd journal, Windows Events, and OpenTelemetry logs, but there is no macOS unified log Function or logs catalog entry.

Risks:

- Exact macOS thermal/fan readings are not a clean public-API problem. A private AppleSMC implementation may break across Intel/Apple Silicon and macOS releases.
- Running `powermetrics` repeatedly can be expensive and often requires elevated privileges. A naive implementation would violate hot-path and process-management expectations.
- Log data is sensitive by design. macOS unified logs may include usernames, file paths, hostnames, process arguments, and private redactions. Function access must remain behind sensitive-data access flags.
- Storage health may expose serial numbers, model identifiers, firmware versions, and device names. Tests and durable artifacts must use synthetic or redacted fixtures.
- Chart contexts and log Function names are public contracts once shipped.
- Adding all six requested areas in one implementation batch risks an unreviewable diff and weak validation.

## Pre-Implementation Gate

Status: split-for-implementation

Problem / root-cause model:

- macOS monitoring coverage is incomplete because the current `macos.plugin` focuses on core OS metrics and IOKit disk/interface statistics, while Netdata's richer hardware health and logs support evolved mostly through Linux-specific sources, go.d storage-health collectors, and platform-specific log Function plugins. The work spans three distinct surfaces: macOS OS/hardware metrics, storage-health collectors, and interactive log Functions. The user selected a split implementation on 2026-05-13, so this umbrella SOW now tracks planning and references the dedicated implementation SOWs.

Evidence reviewed:

- Existing macOS module list and loop: `src/collectors/macos.plugin/plugin_macos.c:15-18`, `src/collectors/macos.plugin/plugin_macos.c:56-75`.
- Existing IOKit disk/interface pattern: `src/collectors/macos.plugin/macos_fw.c:5-13`, `src/collectors/macos.plugin/macos_fw.c:24-36`, `src/collectors/macos.plugin/macos_fw.c:106-116`.
- Linux-only sensors collector: `src/go/plugin/go.d/collector/sensors/collector.go:1-4`.
- Existing storage-health collectors and platform/exec gaps: `src/go/plugin/go.d/collector/smartctl/init.go:44-53`, `src/go/plugin/go.d/collector/nvme/init.go:14-23`, `src/go/plugin/go.d/collector/smartctl/metadata.yaml:36-37`, `src/go/plugin/go.d/collector/nvme/metadata.yaml:28-29`.
- Existing `ndsudo` allow-list for smartctl/nvme commands: `src/collectors/utils/ndsudo.c:103-124`, `src/collectors/utils/ndsudo.c:207-220`.
- Existing log Function registration patterns: `src/collectors/systemd-journal.plugin/systemd-main.c:75-95`, `src/collectors/windows-events.plugin/windows-events.c:1357-1374`.
- Apple public docs and local manuals for power sources, unified logging, `log show`, and `powermetrics`.

Affected contracts and surfaces:

- `macos.plugin` module names, chart contexts, dimensions, labels, and update cadence.
- go.d `smartctl` and `nvme` collectors, config schemas, stock configs, metadata, tests, and docs if reused.
- `ndsudo` command allow-list and privilege boundary if new privileged commands are needed.
- CMake/platform linking if new macOS frameworks are used.
- Logs Function protocol, `FUNCTION_UI_SCHEMA.json` compatibility, Logs tab behavior, access flags, query language, facets, and tail/play behavior.
- `integrations/logs/metadata.yaml` for macOS logs.
- `src/collectors/COLLECTORS.md` and generated per-integration docs if metadata changes are regenerated.
- Health alerts for battery/storage/sensor states if alertable metrics are shipped.
- End-user docs and troubleshooting guidance for macOS permissions and dependencies.

Existing patterns to reuse:

- Internal C OS collector pattern from `macos.plugin` for host-local OS/hardware metrics.
- Existing IOKit enumeration and CoreFoundation cleanup pattern from `macos_fw.c`.
- Existing `sensors` chart contexts and labels where a cross-platform sensor context can be preserved without breaking Linux semantics.
- Existing `smartctl` and `nvme` go.d metric contexts where macOS data maps cleanly.
- Existing `ndsudo` allow-list model for privileged command execution.
- Existing `systemd-journal` and `windows-events` Function plugin registration, query/facet, timeout, progress, and sensitive-data access patterns.

Risk and blast radius:

- Private API risk: direct AppleSMC or IOReport access may break silently on future macOS or Apple Silicon variants.
- Privilege risk: `powermetrics`, storage SMART, and logs may require elevated access; any expansion of `ndsudo` must remain hard-coded and narrow.
- Performance risk: CLI sampling inside a 1-second loop would be unacceptable. Hardware probes must use slower polling/caching or a helper model.
- Cardinality risk: per-sensor and per-log-field labels can explode; selectors, limits, and curated facets are required.
- Privacy risk: macOS logs and storage/device metadata can contain sensitive identifiers. Durable fixtures and docs must be synthetic or redacted.
- Compatibility risk: Apple Silicon and Intel expose different sensor/storage surfaces.

Sensitive data handling plan:

- Do not write real macOS log entries, hardware serials, storage serials, usernames, hostnames, file paths from real logs, private endpoints, account IDs, UUID-like device IDs, or customer-identifying values to SOWs, specs, docs, skills, code comments, tests, or fixtures.
- Use synthetic fixtures for `smartctl`, NVMe, `powermetrics`, and `log show` outputs.
- If real command output is needed for debugging, keep it under `.local/` only and redact before summarizing in any committed artifact.
- Code comments should cite API names, field names, and behavior, not real machine values.
- macOS logs Function must use sensitive-data access flags like the Linux and Windows log Functions.

Implementation plan:

1. Record user decisions below and split the selected scopes into dedicated pending SOWs.
2. For battery, implement or extend a macOS hardware module using public IOKit power-source APIs first unless the user selects a broader private/privileged policy.
3. For thermal/fans, implement the selected source: thermal pressure only, `powermetrics` sampler, private SMC/IOReport reader, or external-command bridge.
4. For S.M.A.R.T. and NVMe, reuse existing go.d collectors where possible or add native macOS IOKit backends if selected.
5. For logs, add a native macOS logs Function plugin without delegating query execution to `log show` / `log stream`, and validate it against the existing Function UI schema and Logs tab expectations.
6. Update metadata, config schemas, stock configs, health alerts, logs catalog metadata, README/generated integration artifacts, and docs in the same implementation scope.

Validation plan:

- Build the affected macOS targets locally.
- Run focused C tests if the touched subsystem has tests; otherwise use compile plus real-use checks with sanitized summaries.
- Run go.d unit tests for `smartctl`/`nvme` if those collectors change.
- Run Function schema validation for macOS logs if a Function is added.
- Manually validate on at least one Apple Silicon and one Intel macOS host if exact sensor/fan support is selected.
- Verify no per-cycle log floods, fd leaks, or runaway process spawning.
- Run same-failure searches for similar collector/platform-specific patterns before close.
- Run the sensitive-data gate before commit if any durable artifacts are staged.

Artifact impact plan:

- AGENTS.md: likely unaffected unless a new macOS collector policy becomes project-wide.
- Runtime project skills: update `project-writing-collectors` only if this work creates a durable macOS hardware/log collector rule future assistants need.
- Specs: add a spec for macOS hardware/log contracts if shipped behavior includes private API policy, log query semantics, or cross-platform sensor context decisions.
- End-user/operator docs: update collector docs, logs docs, and troubleshooting guidance for macOS permissions/dependencies.
- End-user/operator skills: likely unaffected unless public log-query skills need to include macOS logs.
- SOW lifecycle: this planning SOW should be split into implementation SOWs if the user selects a split delivery plan; every deferred item must be implemented, rejected, or tracked before close.

Open-source reference evidence:

- No external open-source implementations were inspected yet. Reason: this umbrella SOW stops at planning. The split implementation SOWs must inspect relevant open-source macOS monitoring implementations before implementation where the selected path depends on `powermetrics`, native unified-log parsing, private SMC/IOReport behavior, or native IOKit storage SMART behavior.

Open decisions:

- None for the umbrella SOW. Split implementation SOWs may identify new design choices after deeper source-specific investigation.

## Implications And Decisions

### Decision 1 - Delivery split

Context and evidence:

- This request spans three different implementation surfaces: `macos.plugin` metrics, go.d storage-health collectors, and logs Function plugins.
- Existing logs are Function plugins (`src/collectors/systemd-journal.plugin/systemd-main.c:75-95`, `src/collectors/windows-events.plugin/windows-events.c:1357-1374`), while existing macOS OS metrics are internal C collector modules (`src/collectors/macos.plugin/plugin_macos.c:15-18`).

Options:

- **A. One large SOW for all six items.** Pros: one umbrella outcome. Cons: high review risk, high regression risk, weak validation focus, and too many unrelated decisions in one diff.
- **B. Two SOWs: hardware/storage metrics and logs.** Pros: separates metrics from Functions. Cons: still mixes low-risk battery work with high-risk thermal/fan/private API and storage-health work.
- **C. Three SOWs: power/sensors/fans, storage health, macOS logs.** Pros: clean surfaces, smaller diffs, focused validation, easier rollback. Cons: more lifecycle overhead.
- **D. Start with only macOS logs.** Pros: delivers one user-visible gap quickly. Cons: delays requested hardware support.

Recommendation: **C**. The surfaces, risks, dependencies, and validation paths are materially different.

Decision: **C**, selected by the user on 2026-05-13 as "As you recommend". Split into:

- `SOW-0017-20260513-macos-power-sensors-fans.md`
- `SOW-0018-20260513-macos-storage-health.md`
- `SOW-0019-20260513-native-macos-logs.md`

### Decision 2 - Thermal sensor and fan source policy

Context and evidence:

- Battery has public IOKit power-source APIs.
- Exact thermal sensors and fan RPMs are not represented by the current `macos.plugin`.
- The local SDK scan found public IOKit storage SMART headers but no public SMC/IOReport headers.
- `powermetrics --help` lists `thermal` and `smc` samplers, but `powermetrics` is a sampler CLI and must not be run naively in a 1-second collection loop.

Options:

- **A. Public APIs only.** Battery plus thermal pressure/state where public. No exact fan RPM or per-sensor temperatures unless a public source is found. Pros: stable, low support risk. Cons: does not fully satisfy "thermal sensors" and "fans" as exact metrics.
- **B. Apple command-line tool path.** Use `powermetrics` through a bounded, privileged, cached helper path for thermal/fan data. Pros: uses Apple's shipped tool and can expose exact-ish data where supported. Cons: privilege, parsing, overhead, and output compatibility risks.
- **C. Direct private SMC/IOReport implementation.** Pros: fastest exact readings and no repeated CLI spawn if implemented correctly. Cons: undocumented/private API, high breakage risk across Intel/Apple Silicon/macOS versions, harder to support.
- **D. Configurable external command bridge.** Let operators provide a command that emits normalized sensor JSON. Pros: avoids owning private API behavior. Cons: weaker out-of-box product, harder support story, security review needed.

Recommendation: **B** if exact thermal/fan metrics are mandatory; otherwise **A** for the first shippable baseline. Do not choose **C** without accepting private API maintenance risk.

Decision: **B**, selected by the user on 2026-05-13 as part of "As you recommend". Use bounded privileged Apple tooling where exact thermal/fan metrics are needed, with public IOKit power-source APIs for battery. Do not start with direct private SMC/IOReport code unless later evidence proves Apple tooling cannot satisfy the requirement and the user accepts the maintenance risk.

### Decision 3 - S.M.A.R.T. and NVMe strategy

Context and evidence:

- Existing `smartctl` and `nvme` collectors already define storage-health charts and docs.
- Existing platform metadata excludes macOS (`src/go/plugin/go.d/collector/smartctl/metadata.yaml:36-37`, `src/go/plugin/go.d/collector/nvme/metadata.yaml:28-29`).
- Existing non-Windows execution uses `ndsudo` (`src/go/plugin/go.d/collector/smartctl/init.go:44-53`, `src/go/plugin/go.d/collector/nvme/init.go:14-23`).
- The local macOS SDK exposes IOKit ATA SMART and NVMe SMART headers, which suggests a native macOS backend is possible.

Options:

- **A. Extend existing go.d collectors to macOS using CLI execution.** Pros: reuses existing contexts/tests/docs. Cons: depends on installed CLI tooling and may not solve macOS NVMe cleanly.
- **B. Add native IOKit storage-health collection in `macos.plugin`.** Pros: no external CLI dependency and better macOS integration. Cons: duplicates existing go.d storage-health logic and requires new parsers/chart mapping.
- **C. Hybrid: `smartctl` for S.M.A.R.T., native IOKit/NVMe SMART backend for macOS NVMe.** Pros: pragmatic reuse for SMART and native path where macOS CLI support is weak. Cons: more integration work than one path.
- **D. Do not change storage-health collectors; document that current support remains Linux/BSD only.** Pros: no risk. Cons: does not satisfy the request.

Recommendation: **C**. It preserves existing `smartctl` value while avoiding a weak assumption that Linux-oriented `nvme-cli` is the right macOS NVMe path.

Decision: **C**, selected by the user on 2026-05-13 as part of "As you recommend". Use a hybrid storage-health strategy: preserve/reuse `smartctl` where practical for S.M.A.R.T. and investigate native macOS IOKit/NVMe SMART support for NVMe.

### Decision 4 - macOS logs implementation strategy

Context and evidence:

- Linux and Windows logs are interactive Function plugins in the `logs` category.
- The `log(1)` manual supports historical queries through `log show`, predicates, and JSON/NDJSON output.
- Native macOS unified log storage uses Apple tracev3/logarchive formats and is not already parsed in this repository.

Options:

- **A. New `macos-logs.plugin` Function using `log show --style ndjson` and optional `log stream` for tail/play.** Pros: fastest path, uses Apple's supported CLI, matches Function-plugin pattern. Cons: process execution, predicate escaping, timeout/cancellation, and output compatibility need careful handling.
- **B. Native tracev3/logarchive parser.** Pros: avoids CLI spawning and may allow deeper indexing. Cons: high complexity, proprietary format risk, large implementation and validation burden.
- **C. Route macOS logs into OpenTelemetry logs and use existing OTEL log viewer.** Pros: reuses existing viewer. Cons: requires a separate ingestion pipeline, weaker "like Linux/Windows logs" parity, not native to macOS nodes.

Recommendation: **A** for initial support, with strict query bounds, cancellation, sanitized errors, and sensitive-data access flags.

Decision: **B**, explicitly selected by the user on 2026-05-13: "logs natively please, not via external tool." Implement native macOS unified-log access; do not build the macOS logs Function by shelling out to `log show` or `log stream`.

### Decision 5 - macOS hardware/version support target

Context and evidence:

- Existing `macos.plugin` is compiled for macOS generally, but exact hardware sensor and storage-health support may differ across Intel and Apple Silicon.
- `powermetrics` and AppleSMC/IOKit surfaces are hardware-dependent.

Options:

- **A. Support both Intel and Apple Silicon, with feature detection and graceful gaps.** Pros: correct product expectation for macOS. Cons: requires broader validation and may ship different metric subsets per hardware family.
- **B. Apple Silicon first.** Pros: targets current hardware. Cons: leaves older Intel Macs with incomplete support.
- **C. Intel first.** Pros: older SMC sensor examples are more common. Cons: misses current hardware and weakens product value.

Recommendation: **A**, but record that exact fan/thermal support is "best effort by detected source" unless Decision 2 accepts a stronger private/privileged dependency.

Decision: **A**, selected by the user on 2026-05-13 as part of "As you recommend". Support Intel and Apple Silicon through feature detection and graceful gaps.

## Plan

1. Record the user's decisions.
2. Split this planning SOW into the selected implementation SOWs if Decision 1 selects a split.
3. Activate exactly one implementation SOW and fill its final pre-implementation gate with the chosen design.
4. Implement the selected scope.
5. Validate with focused tests, macOS build/run checks, reviewer iteration, same-failure search, and artifact maintenance.
6. Close only when every deferred item is implemented, rejected with evidence, or tracked by a real pending/current SOW.

## Execution Log

### 2026-05-12

- Created pending planning SOW after inspecting current SOWs/specs, collector-writing and integrations lifecycle skills, existing macOS collector code, Linux sensors collector, SMART/NVMe go.d collectors, `ndsudo`, Linux/Windows log Function plugins, local macOS manuals, Apple public docs, and local SDK headers.

### 2026-05-13

- Recorded user decisions:
  - split into three implementation SOWs;
  - use bounded privileged Apple tooling for exact thermal/fan data where needed;
  - use a hybrid S.M.A.R.T./NVMe storage-health strategy;
  - implement macOS logs natively, not by shelling out to Apple's `log` tool;
  - support Intel and Apple Silicon with feature detection and graceful gaps.
- Split implementation tracking into pending SOWs:
  - `.agents/sow/pending/SOW-0017-20260513-macos-power-sensors-fans.md`
  - `.agents/sow/pending/SOW-0018-20260513-macos-storage-health.md`
  - `.agents/sow/pending/SOW-0019-20260513-native-macos-logs.md`

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

- SOW uses repo-relative paths, API names, and sanitized command/API evidence. No raw secrets, bearer tokens, customer data, macOS log entries, hardware serials, storage serials, host-specific device values, usernames, or personal data are intentionally included.

Artifact maintenance gate:

- AGENTS.md: no update needed for planning-only SOW creation.
- Runtime project skills: no update needed yet; implementation may update `project-writing-collectors` if new durable macOS guidance emerges.
- Specs: no update needed yet; implementation may add a macOS hardware/log contract spec.
- End-user/operator docs: no update needed yet; implementation SOWs must update docs.
- End-user/operator skills: no update needed yet; implementation may update query skills if macOS logs become queryable.
- SOW lifecycle: file is open in `.agents/sow/pending/`, matching status.

Specs update:

- No spec update needed for planning-only SOW creation.

Project skills update:

- No runtime project skill update needed for planning-only SOW creation.

End-user/operator docs update:

- No docs update needed for planning-only SOW creation.

End-user/operator skills update:

- No operator skill update needed for planning-only SOW creation.

Lessons:

- None yet.

Follow-up mapping:

- Implementation is tracked by the three pending split SOWs listed in the execution log. This umbrella SOW contains no implementation work.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
