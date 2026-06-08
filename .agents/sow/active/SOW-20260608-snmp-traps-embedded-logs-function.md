# SOW-20260608-snmp-traps-embedded-logs-function - SNMP Traps Embedded Logs Function

## Status

Status: in-progress

Sub-state: implementation complete; local focused validation passed; external
review completed with no blocking findings.

## Requirements

### Purpose

Make SNMP trap reception fit production operator workflows: users can run a
trap listener that writes Netdata-native direct journal files by default, can
choose OTEL-only delivery without creating direct journal files, and see an
embedded SNMP trap logs viewer only when direct journal files exist.

### User Request

Implement the go.d Function framework extension needed to expose an SNMP traps
logs viewer backed by the journal SDK Netdata Function API.

Additional constraints from the user:

- The whole `snmp_traps` module must be disabled by default.
- A job must enable SNMP traps explicitly.
- Direct journal output must be enabled by default for enabled jobs.
- Users must be able to disable direct journal creation and keep only OTEL.
- Direct-journal jobs must appear as selectable log sources in one SNMP traps
  logs Function.
- OTEL-only jobs must not create direct journal sources. Until the dedicated
  Function deletion protocol PR lands, the single logs Function may remain
  registered and return no sources or an unavailable response when no direct
  journal source exists.

### Assistant Understanding

Facts:

- The local journal SDK exposes a Netdata-compatible logs Function API with
  raw request bytes and complete response maps.
- The current branch always creates a journal writer for SNMP traps and treats
  OTLP as an optional second backend.
- The current `snmp_traps` collector exposes `snmp_traps:reload-profiles` as a
  module-wide Function.
- Current go.d method handlers receive resolved params, not the raw payload
  needed by the SDK logs Function.
- Current go.d method response code wraps a `FunctionResponse`; the SDK returns
  a complete Function response envelope that must be sent unchanged.
- The dedicated Function deletion protocol PR is responsible for correct
  Function removal. This SOW must not add local plugins.d removal commands.

Inferences:

- The logs viewer should be one module-wide Function, because the Logs UI
  already supports source selection through `__logs_sources`.
- The reload Function should remain module-wide to preserve the existing
  `snmp_traps:reload-profiles` contract.
- The clean implementation needs a small framework extension, not a
  collector-local workaround.
- The per-job direct journal switch should be `journal.enabled`, defaulting to
  `true`, while existing `retention` config remains top-level for
  compatibility.
- Each direct-journal job keeps its own listener endpoints, journal directory,
  and retention policy. The single Function queries the shared traps root and
  maps each job directory to one `__logs_sources` option.

Unknowns:

- None currently blocking.

### Acceptance Criteria

- `snmp_traps` is disabled by default at module registration and still works
  when explicitly enabled/configured.
- SNMP trap jobs support `journal.enabled`, defaulting to `true`.
- `journal.enabled: false` with `otlp.enabled: true` creates an OTEL-only job:
  no direct journal directory/writer is created, and direct-journal creation
  failures are not preflighted for that job.
- A job with both `journal.enabled: false` and `otlp.enabled: false` fails at
  job creation with a clear coded error.
- Existing journal-direct jobs still preflight direct journal creation at job
  creation and surface failures through DynCfg apply.
- Existing `snmp_traps:reload-profiles` remains registered and behaves as
  before.
- Direct-journal jobs appear as `__logs_sources` options in one
  `snmp_traps:logs` Function using the SDK Netdata Function API.
- OTEL-only jobs create no direct journal source and are not listed in
  `__logs_sources`.
- When no direct journal root/source exists, the temporary behavior is an
  unavailable/no-sources logs Function response. Correct dynamic Function
  deletion is deferred to the dedicated Function deletion protocol PR.
- SDK `info` and query responses are returned as full Function envelopes,
  without double-wrapping.
- Cancellation and timeout behavior are wired so logs queries do not keep
  scanning after request cancellation.
- Focused framework tests, SNMP trap collector tests, and function query tests
  pass.

## Analysis

Sources checked:

- `src/go/AGENTS.md`
- `src/go/plugin/framework/docs/changing-framework-code.md`
- `src/go/plugin/framework/collectorapi/registry.go`
- `src/go/plugin/agent/jobmgr/funcctl/controller.go`
- `src/go/plugin/agent/jobmgr/funcctl/dispatch.go`
- `src/go/plugin/agent/jobmgr/funcctl/response.go`
- `src/go/plugin/framework/functions/parser.go`
- `src/go/plugin/framework/functions/manager.go`
- `src/go/plugin/framework/functions/manager_worker.go`
- `src/go/pkg/funcapi/handler.go`
- `src/go/pkg/funcapi/response.go`
- `src/go/pkg/netdataapi/api.go`
- `src/database/rrdfunctions.c`
- `src/plugins.d/pluginsd_functions.c`
- `src/go/plugin/go.d/collector/sql/func_table.go`
- `src/go/plugin/go.d/collector/snmp_traps/collector.go`
- `src/go/plugin/go.d/collector/snmp_traps/reload.go`
- `src/go/plugin/go.d/collector/snmp_traps/config.go`
- `src/go/plugin/go.d/collector/snmp_traps/config_schema.json`
- `src/go/plugin/go.d/config/go.d/snmp_traps.conf`
- `.agents/sow/specs/snmp-traps/netdata.md`
- `~/Documents/systemd-journal-sdk/go/API.md`
- `~/Documents/systemd-journal-sdk/go/journal/netdata.go`

Current state:

- `collectorapi.Register("snmp_traps", ...)` has `Methods:
  snmpTrapsMethods`, `MethodHandler: snmpTrapsMethodHandler`, and no
  `Disabled: true`.
- `Collector.Init()` creates a journal writer before listener creation and
  before optional OTLP writer creation.
- `config_schema.json` documents OTLP as "in addition to the journal-direct
  path".
- `funcctl` currently handles `info` before invoking job handlers.
- `funcapi.MethodHandler.Handle()` receives only method ID and resolved params,
  so it cannot faithfully pass raw JSON logs requests to the SDK.
- `funcctl.respondMethodDataWithParams()` builds a new response envelope and
  sends errors via `respondError`, so it cannot preserve SDK response maps such
  as `status: 304` or `status: 499`.
- `netdataapi.FUNCTIONREMOVE()` remains a no-op until the dedicated Function
  deletion protocol PR handles removal correctly.

Risks:

- Framework Function routing changes affect all go.d collector Functions.
- Raw response passthrough can bypass existing response normalization; it must
  be explicit and limited to handlers that ask for it.
- Dynamic Function disappearance is not relied on in this SOW. Until the
  dedicated deletion protocol PR lands, the Function may remain registered and
  return no sources or unavailable when direct journal output is absent.
- OTEL-only mode changes backend assumptions in the SNMP trap pipeline and
  docs.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- SNMP trap logs are already stored in SDK-created journal files, and the SDK
  can query journal directories using the Netdata logs Function API.
- The existing go.d Function method abstraction is suitable for simple table
  Functions, but not for a logs Function that needs raw payloads, SDK-owned
  `info` responses, cancellation, and full-envelope response passthrough.
- The current SNMP trap backend model has no OTEL-only mode because the direct
  journal writer is always created.

Evidence reviewed:

- `funcctl/dispatch.go` intercepts `info` and calls `handler.Handle()` only
  after parsing payload and resolving params.
- `funcctl/response.go` always constructs a wrapper envelope for
  `FunctionResponse`.
- `functions/parser.go` carries raw payload bytes, request args, timeout, and
  transaction ID at the low-level Function boundary.
- `functions/manager.go` creates a cancelable context for queued/running
  Function invocations, but the registered handler currently receives only
  `functions.Function`, not that context.
- SDK `RunDirectoryRequestBytesWithOptions()` accepts raw request bytes and
  returns the complete response map needed by the Logs UI.
- SDK `NetdataFunctionRunOptions` supports timeout and cancellation callbacks.

Affected contracts and surfaces:

- Shared go.d Function framework: collector registration, dispatch, response
  serialization, raw request access, cancellation.
- Existing go.d collector Functions, especially `snmp_traps:reload-profiles`
  and SQL per-job Functions.
- SNMP traps collector config schema, stock config, metadata, generated docs,
  public operator skill, and specs.
- DynCfg job creation and update behavior.
- Netdata core/plugin Function deletion protocol is intentionally excluded from
  this SOW and handled by the dedicated protocol PR.

Clean-end-state target:

- go.d Function framework supports:
  - an explicit raw/full-envelope response path;
  - a handler path that can receive raw Function requests and handle `info`;
  - cancellation visibility for long-running Function handlers.
- `snmp_traps` keeps module-wide reload and adds one module-wide
  `snmp_traps:logs` Function.
- `snmp_traps:logs` queries the shared traps journal root and uses SDK
  `NetdataFunctionState.FileMetadata()` to expose direct-journal job names as
  `__logs_sources`.
- `snmp_traps` supports direct-journal, OTEL-only, and combined backends using
  one TrapWriter fanout model.
- Removed as redundant (i): any collector-local double-wrap or fake
  unavailable per-job logs Function, and the local `FUNCTION_REMOVE` protocol
  implementation attempted in this branch.
- Excluded coupled items (ii): systemd-journal.plugin virtual functions are
  not part of this SOW; this SOW embeds the logs viewer in go.d.plugin.
- Excluded coupled items (ii): dynamic Function deletion/removal is handled by
  the dedicated Function deletion protocol PR, not by this SOW.
- Reference search: `rg "FunctionResponse|RawMethod|FUNCTIONREMOVE|FUNCTION_REMOVE|snmp_traps:.*:logs|__logs_sources|journal"`
  mapped the relevant existing paths and was rerun during final local
  validation.

Existing patterns to reuse:

- Existing `snmp_traps:reload-profiles` module-wide Function.
- Existing `dyncfgCodedError` pattern for job creation failures.
- Existing `TrapWriter` and fanout writer abstraction for multiple backends.
- Existing SDK-backed `JournalWriter.JournalDirectory()` effective directory.
- SDK `__logs_sources` source-selection pattern.

Risk and blast radius:

- Framework risk is moderate/high because Function routing is shared.
- Collector risk is moderate because backend selection changes job creation,
  cleanup, metrics, and docs.
- Compatibility risk exists if `reload-profiles` name changes; it must not.
- Operational risk exists if OTEL-only silently disables forensic local
  storage; docs and config text must make that explicit.
- Security risk is low if raw Function passthrough is limited to JSON maps from
  trusted handlers and existing permission/access registration is preserved.

Sensitive data handling plan:

- Use only synthetic trap data and temporary directories in tests.
- Do not write SNMP communities, device IPs, customer names, private endpoints,
  credentials, bearer tokens, or production trap payloads to SOWs, specs, docs,
  skills, code comments, or tests.
- Any examples use placeholders or loopback/private documentation values only.

Implementation plan:

1. Extend go.d Function framework:
   - add an explicit raw/full-envelope handler or response path;
   - ensure raw `info` and query payloads can reach handlers;
   - expose cancellation/timeout safely to raw handlers.
2. Add SNMP traps backend selection:
   - add `journal.enabled` defaulting true;
   - only create `JournalWriter` when enabled;
   - require at least one backend;
   - keep direct-journal creation failures as creation-time failures;
   - preserve OTLP preflight failures as creation-time failures when enabled.
3. Add SNMP traps logs Function:
   - register one module-wide `snmp_traps:logs` Function;
   - use SDK `NewNetdataJournalFunction()` with trap-specific defaults;
   - call `RunDirectoryRequestBytesWithOptions()` on the shared traps journal
     root;
   - expose job names as `__logs_sources` using SDK file metadata callbacks;
   - wire timeout and cancellation.
4. Remove local Function removal support:
   - remove local plugins.d `FUNCTION_REMOVE` command wiring and Go emission;
   - document the temporary unavailable/no-sources behavior until the dedicated
     Function deletion protocol PR lands.
5. Update docs/specs/skills:
   - config schema and stock config;
   - metadata and generated integration docs;
   - SNMP traps spec;
   - public `query-snmp-traps` skill if the recommended query Function changes.

Validation plan:

- Framework unit tests for:
  - raw/full-envelope response passthrough;
  - raw `info` request dispatch;
  - cancellation/timeout callback behavior.
- SNMP traps tests for:
  - module disabled by default;
  - `journal.enabled` default true;
  - OTEL-only no journal creation;
  - no-backend config rejected;
  - direct-journal jobs appear in `__logs_sources`;
  - OTEL-only jobs do not create direct journal sources;
  - default source selection queries all direct-journal jobs;
  - explicit `__logs_sources` selection filters by job;
  - direct-journal `info` and query through SDK against temp journals.
- Function deletion validation:
  - no local `FUNCTION_REMOVE` protocol references remain in this branch.
- Focused commands:
  - `GOTOOLCHAIN=go1.26.0 go test ./pkg/netdataapi ./plugin/framework/collectorapi ./plugin/agent/jobmgr/funcctl ./plugin/framework/functions ./pkg/funcapi`
  - `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s`
  - config/schema JSON validation and generated docs regeneration if metadata
    changes.

Artifact impact plan:

- AGENTS.md: no expected update; workflow rules unchanged.
- Runtime project skills: no expected update unless framework Function workflow
  gains a reusable rule.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` for OTEL-only mode,
  `journal.enabled`, and embedded logs Function.
- End-user/operator docs: update SNMP traps metadata/README/generated
  integration docs and stock config.
- End-user/operator skills: update `docs/netdata-ai/skills/query-snmp-traps`
  if the primary Function name changes from `systemd-journal` to the embedded
  SNMP traps Function.
- SOW lifecycle: branch-local active SOW; delete before merge after durable
  knowledge is transferred to specs/docs/tests.

Open-source reference evidence:

- None added for this SOW. This is a Netdata framework integration change and
  uses the local SDK API specified by the user.

Open decisions:

- None blocking.

## Implications And Decisions

1. User decision: the `snmp_traps` module is disabled by default.
   - Implication: stock install does not start a listener or allocate trap
     profile/runtime resources unless explicitly enabled.
   - Risk: operators must explicitly enable/configure a job.

2. User decision: direct journal output is enabled by default for enabled jobs.
   - Implication: existing and typical jobs keep local forensic logs and expose
     the embedded logs viewer.
   - Risk: direct journal file creation remains a job-creation preflight and
     can fail DynCfg apply if paths/permissions are wrong.

3. User decision: OTEL-only jobs are supported.
   - Implication: `journal.enabled: false` with `otlp.enabled: true` does not
     create direct journal files and does not expose the embedded logs viewer.
   - Risk: OTEL-only users lose local journal-backed forensic querying unless
     the OTEL receiver writes logs elsewhere.

4. User decision: implement the needed Function framework extension.
   - Implication: shared go.d Function code changes are in scope.
   - Risk: this requires framework tests and careful compatibility checks.

5. Implementation decision: use `journal.enabled` as the direct-journal switch.
   - Reasoning: it is short, explicit, and leaves existing top-level
     `retention` config backward-compatible.
   - Risk: direct-journal config is split between `journal.enabled` and
     `retention`; moving retention under `journal` would be cleaner but
     breaking.

6. User decision: do not implement local plugins.d Function removal protocol in
   this SOW.
   - Implication: remove the local `FUNCTION_REMOVE` command wiring and Go
     emission from this branch.
   - Reasoning: Function removal affects streaming and other Netdata core
     surfaces and is handled by the dedicated protocol PR.
   - Risk: until that protocol PR is merged/rebased, this branch must not rely
     on dynamic removal commands.

7. User decision: expose one SNMP traps logs Function and use
   `__logs_sources` for job selection.
   - Implication: the public Function is `snmp_traps:logs`; direct-journal jobs
     are selected through the SDK logs source selector.
   - Implication: all direct-journal sources are selected by default by the SDK.
   - Risk: until Function deletion lands, the module-wide Function can remain
     visible even when no direct-journal source exists; it must return no
     sources or unavailable rather than pretend a job is queryable.

## Plan

1. Framework batch: add raw Function support with tests.
2. Collector backend batch: add module default-disabled behavior and
   `journal.enabled` backend selection with tests.
3. Logs Function batch: register and handle one SDK-backed
   `snmp_traps:logs` Function that lists direct-journal job directories through
   `__logs_sources`.
4. Documentation/spec batch: update config schema, stock config, generated
   integration docs, spec, and public trap-query skill as needed.
5. Protocol cleanup batch: remove local Function removal code and leave
   dynamic deletion to the dedicated protocol PR.
6. Validation/review batch: run focused tests, same-failure searches, and
   external reviewers after the whole SOW implementation batch is complete.

## Execution Log

### 2026-06-08

- Created SOW and recorded user decisions before implementation.
- Implemented raw Function request handling, raw/full-envelope response
  passthrough, and cancellation context propagation.
- Implemented `journal.enabled` backend selection with direct-journal default,
  OTEL-only support, and no-backend creation-time failure.
- Added SDK-backed per-job `snmp_traps:<job>:logs` Function for direct-journal
  jobs only. This was superseded by the later approved one-Function
  `__logs_sources` design and is being reworked.
- Added explicit SDK error-path coverage for malformed raw logs requests.
- Made dedup summary write-failure metric selection an explicit constructor
  input instead of mutable setup after construction.
- Added plugins.d `FUNCTION_REMOVE` support and Go API emission for dynamic
  Function removal. This was superseded by the later user decision to leave
  deletion to the dedicated protocol PR and has been removed from the branch.
- Updated SNMP trap config schema, stock config, metadata, generated
  integration docs, SNMP traps spec, public trap-query skill, and plugins.d
  Function protocol docs.

### 2026-06-09

- Recorded the user decision to use one `snmp_traps:logs` Function with
  `__logs_sources` job selection.
- Removed local `FUNCTION_REMOVE` protocol and Go emission from this branch.
- Reworked the logs Function from per-job Functions to one source-selecting
  module Function.
- Updated SNMP traps metadata, generated integration docs, durable spec, and
  public trap-query skill/how-tos for `snmp_traps:logs` and
  `__logs_sources`.

## Validation

Current validation state:

- Local focused validation passed after the one-Function `__logs_sources`
  rework.
- External reviewer pass completed after the one-Function `__logs_sources`
  rework; no blocking production issue was reported.

Acceptance criteria evidence:

- Module disabled by default: `src/go/plugin/go.d/collector/snmp_traps/collector.go`
  sets `Defaults.Disabled: true`; covered by
  `TestCollectorRegistrationDisabledByDefault`.
- `journal.enabled` default true and OTEL-only mode: implemented in
  `config.go` / `collector.go`; covered by
  `TestJournalBackendConfigEnabledDefault` and
  `TestCollectorInit_OTELOnlySkipsJournalCreation`.
- No-backend config rejected at creation time: covered by
  `TestCollectorInit_NoOutputBackendIsCodedError`.
- Direct-journal jobs appear in `__logs_sources`, default source selection
  queries all direct-journal jobs, and explicit source selection filters by job:
  covered by `TestSNMPTrapsLogsFunctionInfoAndQuery`.
- Missing direct-journal root returns an unavailable/no-sources response:
  covered by `TestSNMPTrapsLogsFunctionUnavailableWithoutJournal`.
- SDK raw info/query responses are passed through: covered by
  `TestSNMPTrapsLogsFunctionInfoAndQuery`.
- SDK malformed raw payload errors are converted to a clear 400 response:
  covered by `TestSNMPTrapsLogsFunctionRejectsInvalidJSON`.
- Dynamic Function removal: local `FUNCTION_REMOVE` implementation was removed
  from this branch by user decision; stale-string scan found no local
  `FUNCTION_REMOVE` protocol references.

Tests or equivalent validation:

- PASS: `GOTOOLCHAIN=go1.26.0 go test ./pkg/netdataapi ./plugin/framework/collectorapi ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr ./plugin/framework/functions ./pkg/funcapi -count=1`
- PASS: `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s`
- PASS: `python3 -m json.tool src/go/plugin/go.d/collector/snmp_traps/config_schema.json >/dev/null`
- PASS: `python3 integrations/gen_taxonomy.py --check-only`
- PASS: `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
- PASS: `git diff --check`

Real-use evidence:

- Synthetic SDK-backed logs query test writes trap entries to a temp journal
  directory and queries them through the new raw Function handler.
- No live device trap test was run in this batch.

Reviewer findings:

- `qwen`: no blocking issue. Suggested documenting raw Function context
  propagation, documenting compact journal format, and removing an unnecessary
  `journalBaseRoot()` fallback in the logs handler.
  - Action: accepted and implemented all three.
- `glm`: no critical or security issue. Reported one commit-hygiene item:
  `func_logs.go` and `func_logs_test.go` are untracked before commit.
  - Action: accepted; final commit must explicitly add those two files.
  - Low-severity observations about SDK error-code mapping and dispatch-level
    handler integration are tracked as non-blocking follow-up candidates.
- `minimax`: no blocking issue. Confirmed raw `info` routing to the SDK envelope,
  cancellation wiring, `__logs_sources` default/all behavior, OTEL-only cleanup,
  `reload-profiles` preservation, and absence of local `FUNCTION_REMOVE`
  protocol implementation.
  - Action: no code change required beyond the `qwen` cleanup above.
- `kimi`: unavailable due to quota; no review produced.

Same-failure scan:

- PASS: searched for `FUNCTION_REMOVE`, `PLUGINSD_KEYWORD_FUNCTION_REMOVE`,
  and `pluginsd_function_remove`; no local protocol implementation remains.
- PASS: searched for old per-job `snmp_traps:<job>:logs` / constructed
  `snmp_traps:${...}:logs` docs and code references; replaced the SNMP trap
  surface with `snmp_traps:logs` plus `__logs_sources`.

Sensitive data gate:

- SOW uses only repo-relative paths, SDK-relative `~` paths, and no secrets or
  production trap data.

## Artifact Maintenance Gate

- AGENTS.md: no update needed; repository workflow and guardrails unchanged.
- Runtime project skills: no update needed; collector/framework implementation
  rules did not change.
- Specs: updated `.agents/sow/specs/snmp-traps/netdata.md`.
- End-user/operator docs: updated SNMP trap metadata/generated docs/stock
  config; no local plugins.d Function protocol docs changes remain.
- End-user/operator skills: updated `docs/netdata-ai/skills/query-snmp-traps`
  and how-tos for the embedded `snmp_traps:logs` Function.
- SOW lifecycle: active branch-local file; delete before merge after durable
  artifacts are updated.

Specs update:

- Updated `.agents/sow/specs/snmp-traps/netdata.md`.

Project skills update:

- No project skill update needed; no reusable authoring workflow changed.

End-user/operator docs update:

- Updated SNMP trap integration metadata/generated docs/stock config for
  `snmp_traps:logs` and `__logs_sources`.

End-user/operator skills update:

- Updated public `query-snmp-traps` skill and how-tos for `snmp_traps:logs`.

Lessons:

- Dynamic Function availability depends on core protocol support and is handled
  by the dedicated Function deletion protocol PR.

Follow-up mapping:

- Dedicated Function deletion protocol PR must provide correct removal; this
  SOW intentionally does not implement it.
- Commit hygiene: `src/go/plugin/go.d/collector/snmp_traps/func_logs.go` and
  `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go` are new files
  and must be explicitly added before commit.

## Outcome

Implementation is complete, locally validated, and externally reviewed with no
blocking findings. Final merge readiness depends on normal PR review/CI and the
separate Function deletion protocol PR for dynamic removal behavior.

## Lessons Extracted

- Job-scoped Function removal is not part of this SOW; the dedicated Function
  deletion protocol PR owns that behavior.
- For raw logs-style Functions, tests must cover both SDK-owned success
  envelopes and SDK error conversion; normal table Function tests do not prove
  that path.

## Follow-up Issues

None currently known.
