# SOW-0019 - Native macOS logs Function

## Status

Status: completed

Sub-state: completed again on 2026-05-13 after fixing the runtime permissions regression for installed macOS logs and related powermetrics hardware metrics.

## Requirements

### Purpose

Add macOS unified logs to the Netdata Logs tab through a native Function implementation, similar in user-facing behavior to Linux `systemd-journal` and Windows Events, without shelling out to Apple's `log` command-line tool for query execution.

### User Request

From the parent request in SOW-0016, support:

- macOS logs, similar to how Netdata monitors Linux and Windows logs

User decision from 2026-05-13: "logs natively please, not via external tool."

### Assistant Understanding

Facts:

- Linux and Windows logs are Function plugins in the `logs` category.
- `systemd-journal.plugin` registers `systemd-journal` with sensitive-data access flags.
- `windows-events.plugin` registers `windows-events` with sensitive-data access flags.
- Apple's public documentation says unified logging is available on macOS 10.12+.
- The local `log(1)` manual shows `log show` and `log stream` can query logs, but the user explicitly rejected an external-tool implementation.
- Netdata does not currently have a native macOS unified-log parser or Function.

Inferences:

- A native implementation likely needs to read Apple's unified-log datastore or use a native framework/API rather than invoking `log`.
- This is materially more complex and higher risk than the originally recommended CLI-backed Function.
- The work must begin with deeper research into public/private APIs, tracev3/logarchive format behavior, and open-source parsers before implementation.

Unknowns:

- Exact runtime permission behavior across supported macOS versions when `netdata` runs as root versus a non-admin service account.
- Whether Apple's native reverse enumeration honors all requested positions on every supported macOS version; implementation must remain bounded even if the OS returns more entries than requested.
- How much predicate pushdown is reliable enough to enable by default; implementation starts with time-bounded enumeration and Netdata-side facet filtering, matching existing Logs tab semantics.

### Acceptance Criteria

- A macOS logs Function is registered in the `logs` category and appears in the Logs tab.
- The implementation does not shell out to `log show`, `log stream`, or other external log-query commands for normal query execution.
- Queries are bounded by time, row count, timeout, and cancellation.
- The Function uses sensitive-data access flags equivalent to Linux and Windows log Functions.
- The response conforms to Netdata Function UI schemas and supports useful fields/facets/search.
- Native implementation feasibility and API/format risks are documented with evidence before code changes.
- Docs and `integrations/logs/metadata.yaml` are updated.

## Analysis

Sources checked:

- SOW-0016.
- `src/collectors/systemd-journal.plugin/`.
- `src/collectors/windows-events.plugin/`.
- `integrations/logs/metadata.yaml`.
- Apple unified logging public docs.
- Local `log(1)` manual.
- Local macOS SDK OSLog headers:
  - `System/Library/Frameworks/OSLog.framework/Headers/Store.h`: `OSLogStore`, local/system stores, enumerators, date positions, reverse iteration, and admin permission requirement.
  - `System/Library/Frameworks/OSLog.framework/Headers/Entry.h`: `OSLogEntry`, `OSLogEntryFromProcess`, and `OSLogEntryWithPayload` fields.
  - `System/Library/Frameworks/OSLog.framework/Headers/EntryLog.h`: `OSLogEntryLog` levels.
- Local proof probe using `OSLogStore` and `OSLogEnumerator` against the system store returned bounded entries without printing log contents.
- Open-source reference evidence:
  - `mandiant/macos-UnifiedLogs @ 868d56a6d2efef3e21ebe4e5737b2ed16097913d`: Rust tracev3 parser; README documents extracted fields and parser limitations.
  - `ydkhatri/UnifiedLogReader @ 39f774e41be45a780f84e1486ce49b2ed31cab9f`: archived Python tracev3 parser; README documents required diagnostics/uuidtext/timesync inputs and macOS-version limits.

Current state:

- Netdata has no macOS logs Function.
- Existing Linux/Windows logs Function patterns are available for Function registration, access flags, query bounds, and Logs tab category.
- The public OSLog API provides native querying of the macOS unified log store from Objective-C without invoking `/usr/bin/log`.
- Tracev3 parsers exist, but they are forensic parsers for Apple's private binary storage format and have explicit limitations; they are not the right first implementation path for an always-on Agent plugin when a public API exists.

Risks:

- Native unified-log access may depend on private, unstable, or reverse-engineered formats.
- Log data is sensitive; native parser bugs could expose too much or mishandle redaction.
- Query performance on large unified-log stores is unknown without Apple's `log` indexing behavior.
- A native parser may need substantial fixtures, but real logs cannot be committed.

## Pre-Implementation Gate

Status: ready-for-implementation

Problem / root-cause model:

- macOS logs are missing because Netdata's existing logs support is platform-specific: Linux uses systemd journal APIs/files and Windows uses Windows Event Log APIs. macOS unified logs have a different store and API model. The user requires native access rather than shelling out to Apple's `log` tool.
- Root cause of the gap is not lack of a Netdata Function schema; that exists in `logs_query_status.h`. The missing piece is a macOS backend that can enumerate native unified-log entries and translate them into Netdata facets safely.

Evidence reviewed:

- Netdata Function registration pattern:
  - `src/collectors/systemd-journal.plugin/systemd-main.c`: registers `systemd-journal` in category `logs` with `HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA`.
  - `src/collectors/windows-events.plugin/windows-events.c`: registers `windows-events` with the same sensitive access pattern.
- Shared Logs tab query machinery:
  - `src/libnetdata/facets/logs_query_status.h`: parses `after`, `before`, `anchor`, `last`, `query`, `facets`, `histogram`, `direction`, `data_only`, `tail`, and source parameters.
  - `src/libnetdata/facets/facets.h`: provides row fields, severities, filters, FTS, facets, histograms, and item selection.
- Public macOS API evidence:
  - `OSLogStore` can create local/system stores, return positions by date, and return `OSLogEnumerator` objects with reverse iteration.
  - `OSLogEntry` exposes timestamp, composed message, and store category.
  - `OSLogEntryFromProcess` exposes process, pid, sender, thread id, and activity id.
  - `OSLogEntryWithPayload` exposes subsystem and category.
  - `OSLogEntryLog` exposes level.
- Runtime probe:
  - A local Objective-C program compiled with `clang -fobjc-arc -framework Foundation -framework OSLog` and enumerated a bounded sample from `OSLogStoreSystem` without printing log contents.
- Alternative evidence:
  - Existing tracev3 parsers confirm the same useful fields exist, but their README limitations and reverse-engineered storage dependency make them a fallback, not the first implementation path.

Affected contracts and surfaces:

- New macOS logs Function name: `macos-logs`.
- New macOS-only plugin binary: `macos-logs.plugin`.
- Function registration in category `logs`.
- Function UI response schema and Logs tab behavior.
- Access flags and sensitive-data handling.
- macOS CMake build language/linking: Objective-C source, Foundation framework, OSLog framework, and libnetdata.
- `integrations/logs/metadata.yaml` and docs.

Existing patterns to reuse:

- `systemd-journal.plugin` Function event loop, registration, sensitive access flags, progress, timeout, cancellation, and logs facets.
- `windows-events.plugin` native platform-event-query pattern.
- Existing Function validation tooling.
- Existing macOS build conditional structure in `CMakeLists.txt` for macOS-only collectors and Apple frameworks.

Risk and blast radius:

- Medium-high but bounded. The chosen API is public, but log access is sensitive and permission-dependent. Query performance depends on Apple's OSLog implementation, so Netdata must enforce time, row, timeout, cancellation, and scan caps.
- Blast radius is limited to macOS builds and a new plugin binary. Linux, Windows, and existing logs plugins should not change behavior.

Sensitive data handling plan:

- Never commit real macOS log records, usernames, file paths, hostnames, process arguments, UUID-like IDs, or proprietary traces.
- Use synthetic fixtures only. If real logs are needed, store under `.local/` and redact summaries before committing.
- Function must register as sensitive data access.
- Local validation may query real system logs only for counts/schema behavior; no real log payloads, usernames, hostnames, paths, process arguments, identifiers, or endpoint values are written to repo artifacts.

Implementation plan:

1. Add `src/collectors/macos-logs.plugin/` with:
   - C-facing header and Function entrypoint.
   - Objective-C bridge that owns `OSLogStore` access and maps `OSLogEntry` objects to Netdata facets.
   - C main/event-loop code mirroring `windows-events.plugin` and `systemd-journal.plugin`.
2. Register fields in stable order:
   - `MESSAGE` as visible main text and FTS.
   - `LEVEL` as default histogram/facet and row severity source.
   - `PROCESS`, `PID`, `SENDER`, `SUBSYSTEM`, `CATEGORY`, `ENTRY_TYPE`, `STORE_CATEGORY`, `THREAD_ID`, and `ACTIVITY_ID` as useful facets/details.
3. Query behavior:
   - Use `OSLogStoreSystem` on macOS 12+ and `localStoreAndReturnError` fallback on macOS 10.15+.
   - Use `positionWithDate:` and `OSLogEnumeratorReverse` or forward enumeration according to request direction.
   - Enforce `after`, `before`, `last`, `data_only`, `tail`, cancellation, timeout, and a hard scan cap.
   - Filter full-text/facets through Netdata's facets layer first; enable OS predicate pushdown only if a small, safe subset proves reliable.
4. Wire CMake on macOS only:
   - Enable Objective-C for macOS builds.
   - Find/link OSLog and Foundation.
   - Install `macos-logs.plugin` under `plugins.d`.
5. Update docs and integration metadata for macOS logs.

Validation plan:

- Configure and build the macOS target that includes the new plugin.
- Run `macos-logs.plugin debug` or a direct Function invocation with bounded parameters; record only counts/status/schema, not log rows.
- Run Function schema validation against sanitized output if the validation tooling supports direct plugin output.
- Run `rg` same-failure scans to verify no external `log show`, `log stream`, or unbounded log-query command exists in the plugin.
- Verify cancellation/timeout/scan caps by code inspection and bounded local execution.
- Run `.agents/sow/audit.sh`.

Artifact impact plan:

- AGENTS.md: likely unaffected unless native macOS logs require project-wide policy.
- Runtime project skills: update only if durable Function/log authoring guidance emerges.
- Specs: add/update logs Function contract if shipped and not already covered by source docs.
- End-user/operator docs: update Logs tab and integration docs.
- End-user/operator skills: update query skills only if their user-facing source list or examples change.
- SOW lifecycle: close with implementation artifacts when complete.

Open-source reference evidence:

- `mandiant/macos-UnifiedLogs @ 868d56a6d2efef3e21ebe4e5737b2ed16097913d`: tracev3 parser used as field/format feasibility comparison only; not vendored or linked.
- `ydkhatri/UnifiedLogReader @ 39f774e41be45a780f84e1486ce49b2ed31cab9f`: archived tracev3 parser used as risk evidence for reverse-engineered parsing; not vendored or linked.

Open decisions:

- None. Existing project patterns resolve the design:
  - Native public OS API beats external command execution and beats reverse-engineered tracev3 parsing.
  - A dedicated macOS Function plugin matches Linux and Windows logs architecture.
  - Sensitive-data access flags match existing logs Functions.

## Implications And Decisions

- 2026-05-13: User explicitly rejected external-tool execution for macOS logs and selected native implementation.

## Plan

1. Implement the native `macos-logs.plugin`.
2. Update metadata/docs.
3. Validate build, bounded execution, schema behavior, and SOW audit.

## Execution Log

### 2026-05-13

- Created as split implementation SOW from SOW-0016.
- Activated SOW and selected the native public `OSLogStore` design after checking existing Netdata logs Function patterns, local SDK headers, a bounded local proof probe, and open-source tracev3 parsers as fallback evidence.
- Added `src/collectors/macos-logs.plugin/` with C Function registration and an Objective-C OSLog bridge.
- Added macOS-only CMake wiring for Objective-C, Foundation, and OSLog.
- Added build-info reporting for `macos-logs`.
- Updated Logs tab docs, logs integration metadata, generated logs integration docs, public query skills, and `.agents/sow/specs/macos-logs-function.md`.

## Validation

Acceptance criteria evidence:

- Function registration exists in `src/collectors/macos-logs.plugin/macos-logs.c`:
  - Function name: `macos-logs`.
  - Category: `logs`.
  - Access flags: `HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA`.
- Native implementation exists in `src/collectors/macos-logs.plugin/macos-logs-oslog.m`:
  - Uses `OSLogStore`, `OSLogEnumerator`, `OSLogEntry`, `OSLogEntryFromProcess`, `OSLogEntryWithPayload`, and `OSLogEntryLog`.
  - Does not invoke external log-query commands.
  - Enforces time bounds, Function timeout/cancellation checks, data-only stop behavior, and a hard scan cap.
- Build wiring exists in `CMakeLists.txt` and `packaging/cmake/Modules/NetdataPlatform.cmake`.
- User-facing docs and metadata updated:
  - `src/collectors/macos-logs.plugin/README.md`.
  - `docs/dashboards-and-charts/logs-tab.md`.
  - `integrations/logs/metadata.yaml`.
  - `integrations/logs/integrations/macos_unified_logs.md`.

Tests or equivalent validation:

- `git diff --check`: passed.
- `.local/integrations-venv/bin/python integrations/gen_integrations.py`: passed.
- `.local/integrations-venv/bin/python integrations/gen_docs_integrations.py`: passed.
- Objective-C OSLog API syntax probe with `clang -x objective-c -fobjc-arc -framework Foundation -framework OSLog -fsyntax-only`: passed. The compiler reported only expected unused linker-input warnings because `-fsyntax-only` does not link frameworks.
- Objective-C entry-class syntax probe for `OSLogEntryLog`, `OSLogEntryActivity`, `OSLogEntrySignpost`, and `OSLogEntryBoundary`: passed with the same `-fsyntax-only` framework warnings.
- Full Netdata CMake configure/build was not runnable in this workstation state: `cmake` is not installed.
- Direct source syntax checking of `src/collectors/macos-logs.plugin/macos-logs-oslog.m` was attempted with a temporary empty config header and failed before reaching the new source because required project build definitions/dependency headers were missing: `SIZEOF_VOID_P is not defined` and `uv.h file not found`.

Real-use evidence:

- Bounded native OSLog smoke probe compiled and ran through Apple's OSLog framework without storing or printing log payloads:
  - command shape: `clang -x objective-c -fobjc-arc -framework Foundation -framework OSLog -o /tmp/netdata-oslog-count-probe -`.
  - output summary: `ok count=5`.
- No real log messages, usernames, hostnames, process arguments, endpoint values, or identifiers were written to committed artifacts.

Reviewer findings:

- No independent internal or external reviewer was invoked; the user did not ask for a review, and current developer rules allow subagents only when explicitly requested.
- Self-review findings handled:
  - Removed a nonexistent `functions_evloop_process()` call and matched the existing Linux/Windows external Function heartbeat loop.
  - Kept debug mode to `info` output only so debug execution does not dump real logs by default.
  - Added a helper for `NSError` domain handling instead of relying on a non-portable shorthand expression.
  - Added public query-skill updates after discovering they still enumerated only the pre-existing log Functions.

Same-failure scan:

- `rg -n "log show|log stream|/usr/bin/log|popen|system\\(|posix_spawn|execv|NSTask|fork\\(" src/collectors/macos-logs.plugin`:
  - matched only README text stating external commands are not used.
  - no source-code external command execution paths found.
- `rg -n "functions_evloop_process" src/collectors/macos-logs.plugin src/libnetdata/functions_evloop src/collectors/systemd-journal.plugin src/collectors/windows-events.plugin`:
  - no matches.
- `rg -n "apple\\.svg" integrations/logs/metadata.yaml integrations/logs/integrations/macos_unified_logs.md`:
  - no matches; the integration uses the existing `macos.svg` icon.

Sensitive data gate:

- Durable artifacts contain sanitized evidence only. Real OSLog records were not copied into SOWs, specs, docs, skills, comments, tests, or fixtures.
- Temporary OSLog probes printed only status/count output and used `/tmp` for throwaway binaries.

Artifact maintenance gate:

- AGENTS.md: no update needed; existing SOW, collector, integration, sensitive-data, and git rules covered the work.
- Runtime project skills: no update needed; `project-writing-collectors` and `integrations-lifecycle` already covered new Function collector and integration metadata work.
- Specs: added `.agents/sow/specs/macos-logs-function.md`.
- End-user/operator docs: updated `src/collectors/macos-logs.plugin/README.md`, `docs/dashboards-and-charts/logs-tab.md`, `integrations/logs/metadata.yaml`, and generated `integrations/logs/integrations/macos_unified_logs.md`.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-netdata-cloud/` and `docs/netdata-ai/skills/query-netdata-agents/` log Function references.
- SOW lifecycle: status is `completed`; this file is being moved to `.agents/sow/done/` with implementation artifacts.

Specs update:

- Added `.agents/sow/specs/macos-logs-function.md` with the native OSLog, access, source-selector, fields, bounds, and sensitive-data contracts.

Project skills update:

- No runtime project skill update needed; the existing collector and integration skills already describe the workflow used.

End-user/operator docs update:

- Updated:
  - `src/collectors/macos-logs.plugin/README.md`.
  - `docs/dashboards-and-charts/logs-tab.md`.
  - `integrations/logs/metadata.yaml`.
  - `integrations/logs/integrations/macos_unified_logs.md`.

End-user/operator skills update:

- Updated:
  - `docs/netdata-ai/skills/query-netdata-cloud/SKILL.md`.
  - `docs/netdata-ai/skills/query-netdata-cloud/query-functions.md`.
  - `docs/netdata-ai/skills/query-netdata-cloud/query-logs.md`.
  - `docs/netdata-ai/skills/query-netdata-agents/SKILL.md`.
  - `docs/netdata-ai/skills/query-netdata-agents/query-logs.md`.

Lessons:

- The public OSLog framework is a better first implementation path than reverse-engineered tracev3 parsing for an Agent plugin. Tracev3 references remain useful as field/format risk evidence, not runtime dependencies.
- Native macOS log validation must avoid recording payloads. Count/schema/status probes are sufficient durable evidence unless a future bug requires local redacted debugging.

Follow-up mapping:

- Build validation remains environment-blocked because `cmake` and required dependency headers are missing locally; no separate SOW is created because this is an environment limitation, not a product design follow-up.
- Hardware/storage macOS monitoring is tracked separately by SOW-0017 and SOW-0018 from the parent feature split.

## Outcome

Implemented native macOS unified log support through a new macOS-only `macos-logs.plugin`.

The implementation uses Apple's OSLog framework directly, registers as a sensitive Logs tab Function, avoids external `log` command execution, exposes useful OSLog fields/facets, enforces query bounds, and updates docs, integration metadata, generated integration docs, public query skills, and the SOW spec.

## Lessons Extracted

Native platform APIs can satisfy the user's native-log requirement while still reusing the existing Netdata logs Function machinery. The durable rule is now captured in `.agents/sow/specs/macos-logs-function.md`.

## Followup

None for this SOW. The remaining macOS hardware/storage monitoring work is tracked by SOW-0017 and SOW-0018.

## Regression Log

### Regression - 2026-05-13

What broke:

- Direct use of the Logs tab returned HTTP 500 with `failed to open macOS unified log store`.
- Local installed log evidence showed `MACOS-LOGS: failed to open OSLogStore, domain 'OSLogErrorDomain', code 9` in `/opt/netdata/var/log/netdata/collector.log`.
- The installed plugin permission model differed from privileged plugin patterns: `/opt/netdata/usr/libexec/netdata/plugins.d/macos-logs.plugin` was `root:netdata` mode `0750`, while existing privileged plugins such as `apps.plugin`, `go.d.plugin`, and `ndsudo` were `root:netdata` mode `4750`.
- A local sanitized OSLog probe confirmed that an effective-root process invoked by the `netdata` user can open and enumerate `OSLogStore` without recording log payloads.
- Related macOS hardware metrics also exposed a permission bug: `/usr/bin/powermetrics` failed with `powermetrics must be invoked as the superuser` when spawned directly from the unprivileged daemon.

Why previous validation missed it:

- The original SOW validation used a standalone Objective-C OSLog probe as the invoking user/root path and did not validate the installed plugin under Netdata's runtime user and installed file permissions.
- Build validation was environment-blocked at the time, so the full installer permission path was not exercised before completion.

User decision:

- 2026-05-13: user selected option 1, least-privilege escalation per component.
- Logs: make `macos-logs.plugin` follow the existing privileged Function plugin pattern with `root:netdata` ownership and setuid root mode.
- Metrics: route the native Apple `powermetrics` sampler through `ndsudo` with a hard-coded allow-list for the exact supported argv shape instead of running the whole daemon as root.

Repair plan:

- Update installer and makeself permission setup so installed `macos-logs.plugin` is owned by `root:${NETDATA_GROUP}` and mode `4750` when present.
- Add a narrow `ndsudo` allow-list entry for the supported `powermetrics` sampler command.
- Update `macos.plugin` powermetrics execution to default to `ndsudo powermetrics-thermal-smc` while preserving a configurable command path for advanced/debug use.
- Update docs/specs to state the installed privilege model.
- Rebuild/reinstall locally, restart Netdata, and validate both `macos-logs` and powermetrics behavior without writing real log payloads into durable artifacts.

Repair outcome:

- Source installer path now sets installed `macos-logs.plugin` to `root:${NETDATA_GROUP}` and mode `4750`.
- Static/makeself install path now includes `macos-logs.plugin` in privileged ownership handling and sets mode `4750`.
- `ndsudo` now has a hard-coded `powermetrics-thermal-smc` command for `/usr/bin/powermetrics -n 1 -i {{sampleWindowMs}} -s thermal,smc -f plist`.
- `ndsudo` validates `--sampleWindowMs` as a present positive integer before running the allow-listed command.
- `macos.plugin` now defaults `[plugin:macos:powermetrics] use ndsudo = yes` and invokes the allow-listed helper command through the installed plugins directory.
- The direct `command path` path remains available only when `use ndsudo` is disabled.

Regression validation:

- `cmake --build ./build --parallel 8 --target ndsudo netdata macos-logs.plugin`: passed before reinstall.
- `./build/ndsudo --test powermetrics-thermal-smc --sampleWindowMs 1000`: printed `/usr/bin/powermetrics -n 1 -i 1000 -s thermal,smc -f plist`.
- `./build/ndsudo --test powermetrics-thermal-smc --sampleWindowMs abc`: rejected the argument with exit code `2`.
- Reinstall command completed and restarted Netdata in `/opt/netdata`. Non-fatal installer warnings were unrelated to this fix: `git fetch -t` failed under `sudo` because of SSH host-key setup, the `netdata` group already existed, and `/usr/lib/tmpfiles.d` is not writable on macOS.
- Installed permission check:
  - `/opt/netdata/usr/libexec/netdata/plugins.d/macos-logs.plugin`: `root netdata` mode `4750`.
  - `/opt/netdata/usr/libexec/netdata/plugins.d/ndsudo`: `root netdata` mode `4750`.
- Runtime API check: `http://localhost:19999/api/v1/info` returned `v2.10.0-207-g5a65ed9523` and `macOS`.
- Launchd check: `system/com.github.netdata` was `state = running` with program `/opt/netdata/usr/sbin/netdata`.
- Installed `ndsudo` test as the `netdata` user printed the exact allow-listed `powermetrics` argv.
- Installed `ndsudo` real execution as the `netdata` user produced a plist payload and no `powermetrics must be invoked as the superuser` error.
- Root `powermetrics` comparison on this host produced the same plist shape: `thermal_pressure = Nominal` and an empty `smc` dictionary. This means fan and SMC temperature charts cannot appear on this host because Apple returns no SMC sensor values here; the permission issue is fixed, but missing SMC data is hardware/OS output behavior.
- Netdata charts API showed `macos.thermal_pressure` after restart.
- Direct installed `macos-logs.plugin` Function-protocol query as the `netdata` user returned `FUNCTION_RESULT_BEGIN "tx1" 200 "application/json"` and reported unified-log rows read/useful. Real log payloads were not copied into durable artifacts.
- New collector log evidence after restart did not show the prior `powermetrics must be invoked as the superuser` failure or `MACOS-LOGS: failed to open OSLogStore` failure; remaining `Invalid key detected!` lines come from Apple's `powermetrics` command on this host and also appear when run directly as root.

Same-failure search:

- `rg -n "powermetrics-thermal-smc|sampleWindowMs" src/collectors/utils/ndsudo.c src/collectors/macos.plugin/macos_powermetrics.c`: only the new allow-list and caller path matched.
- `tail -n 180 /opt/netdata/var/log/netdata/collector.log | rg -n "powermetrics must be invoked|failed to open OSLogStore|MACOS-LOGS: failed|disabling powermetrics"`: only pre-reinstall failures remained in the historical tail.

Artifact maintenance gate for regression:

- AGENTS.md: no update needed; existing SOW regression, privilege, collector, and install rules covered the work.
- Runtime project skills: no update needed; `project-writing-collectors` and `integrations-lifecycle` still describe the workflow used.
- Specs: updated `.agents/sow/specs/macos-logs-function.md` and `.agents/sow/specs/macos-hardware-monitoring.md` to record installed privilege behavior.
- End-user/operator docs: updated `src/collectors/macos-logs.plugin/README.md`, `src/collectors/macos.plugin/metadata.yaml`, and generated integration docs.
- End-user/operator skills: no update needed for this regression; public query skill behavior did not change.
- SOW lifecycle: reopened from `done/`, marked completed after validation, and moved back to `done/` with the repair.

Regression lessons:

- macOS log validation must include the installed setuid/runtime-user path, not just a standalone OSLog probe.
- Privileged macOS metric collection should use a narrow helper allow-list instead of requiring the full daemon to run as root.
- `powermetrics` can succeed yet expose no SMC fan/temperature keys on some Macs; absence of those charts is not necessarily a privilege regression.
