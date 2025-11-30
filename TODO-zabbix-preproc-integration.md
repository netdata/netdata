# TL;DR
- When a question or implementation detail is not explicitly answered in this TODO, default to the behavior of the respective upstream system: follow existing Nagios semantics when working on Nagios pieces and follow native Zabbix semantics when working on Zabbix pieces. Always prefer their behavior over inventing new rules unless documented otherwise below.
- We will integrate the standalone `github.com/netdata/zabbix-preproc` library into the scripts.d plugin so Zabbix-style jobs (collection + LLD + dependent preprocessing) can run natively inside Netdata without affecting other jobs.
- Nagios jobs will also be refactored to match go.d semantics: each job is standalone, and execution is coordinated by configurable schedulers (with a default) rather than monolithic shards.
- Zabbix jobs will encapsulate three layers—collection command, optional LLD pipeline, and one or more dependent pipelines—and the library will run preprocessing only for Zabbix jobs (`collect → preprocess → metrics/logs`).
- **High priority:** build a dedicated “Zabbix job executor” library (wraps zabbixpreproc, exposes `register → process → destroy`) plus ≥100 table-driven E2E tests so scripts.d simply maps returned metrics/flags to Netdata charts.
- **Gate for integration:** this job engine must also ship with ≥50 multi-collection/stateful scenarios (add/remove/failure permutations) that cover every LLD/preproc format we support (JSONPath, CSV, XML/XPath, SNMP walk, Prometheus multi, etc.) and finish in ≲2 seconds so regressions are obvious and cheap to run.

# Analysis
## Library capabilities (`~/src/zabbix-preproc`)
- Pure-Go module (Go 1.25.1) with no CGO; passes 362/362 official Zabbix preprocessing tests plus bespoke multi-metric/unit tests.
- Exposes `Preprocessor`, `Step`, `Value`, `Result`, `Metric`, validation helpers, timeouts, and a shard-scoped state machine. Supports Zabbix step IDs 1–30 plus Netdata extensions ≥60 for multi-metric extractions (Prometheus/JSONPath/SNMP-walk/CSV multi variants).
- Error handling mirrors Zabbix (`ErrorHandler` with default/discard/set-value/set-error). `ExecutePipeline(itemID, value, steps)` produces `Result{Metrics, Logs, Error, Discarded}` where `Metrics` may be a single scalar (Zabbix-compatible) or multiple entries (Netdata extension). State keys are `shardID:itemID:operation`.
- No persistence is required; state resets on restart. Logging hook exists but defaults to no-op.

## Current scripts.d state (`~/src/netdata-ktsaou.git`)
- Nagios module today treats each configuration file as a shard that owns a single `runtime.Scheduler` instance. All jobs inside the shard share workers/retry state, so editing one job forces a full shard restart (sections `modules/nagios`, `pkg/runtime`).
- Job specs (`pkg/spec`) lack preprocessing concepts (no step arrays, no LLD definitions, no dependent pipelines). Scheduler assumes each job emits Nagios status/perfdata text with no intermediate transformations.
- go.d modules already operate as standalone jobs; they rely on vnodes/labels for chart uniqueness. Scripts.d needs to align with that model.

## Target architecture
1. **Standalone jobs**: Both Nagios and Zabbix jobs become top-level go.d-like jobs (unique names). They can be added/removed/reloaded individually without restarting unrelated jobs.
2. **Configurable schedulers**: A new “virtual” module `scheduler` will expose scheduler definitions as jobs so dyncfg/IaC can manage worker pools, retry policies, OTLP logging, etc. Each Nagios/Zabbix job references a scheduler (default provided). Internally, schedulers replace the old shard concept but without bundling every job together.
3. **Zabbix job structure**: A single Zabbix job encapsulates:
   - **Collection**: command/snmp/http invocation that returns the raw payload (same as Zabbix master item). Runs on the schedule controlled by the assigned scheduler.
   - **LLD pipeline (optional)**: uses zabbix-preproc steps to emit discovery JSON arrays (`[{#MACRO}: value, ...]`). scripts.d consumes this to create/update per-instance dependent pipelines.
   - **Dependent pipelines**: per-metric preprocessing definitions. Each pipeline produces one scalar metric per instance (mirrors Zabbix dependent items). They can reference LLD macros (e.g., `{#FSNAME}`) and reuse the latest collection payload; no extra data fetches occur.
4. **Preprocessing scope**: Only Zabbix jobs run `collect → preprocess → metrics/logs`. Nagios jobs continue producing Nagios-style status/perfdata output with no zabbix-preproc involvement.
5. **Uniqueness constraints**: Job names must be globally unique across scripts.d. Using `{module}.{job}` (e.g., `zabbix.fs_usage`) ensures unique chart contexts per vnode and unique zabbix-preproc `itemID`s.
6. **Timeframes**: Netdata will not implement Zabbix time periods; all jobs run 24×7. Time period fields in existing configs will be ignored/removed for Nagios/Zabbix integrations.
7. **LLD orchestration**: The library already emits LLD-compatible JSON arrays; scripts.d must add orchestration to run LLD pipelines, materialize instance definitions, and attach dependent pipelines to those instances. This replaces Zabbix’s built-in LLD/dependent item management.

## Integration challenges to solve (detailed)
1. **Configuration schema**
   - **Modules**: Zabbix becomes a new module (e.g., `modules/zabbix`) separate from Nagios. Schedulers live in a dedicated “scheduler” module so dyncfg/IaC can manage them like jobs.
   - **Scheduler schema**: Each scheduler definition includes name (unique), worker count, queue size (if needed), jitter %, OTLP logging settings, and optional labels/metadata. CRUD operations on schedulers mirror job lifecycle (create/update/delete). Deleting a scheduler must either refuse if jobs reference it or force those jobs to rebind to `default`.
   - **Zabbix job schema**: Each job declares:
     - Collection method (matching the Zabbix item types we plan to support: agent/command execution, SNMP queries, HTTP checks, simple checks, scripts, etc.) with all parameters and macros Zabbix exposes. Item keys and macro placeholders ($USERn$, {HOST.*}, {ITEM.*}, {#MACRO}, etc.) must be honored exactly, and any Zabbix-standard variables need equivalent handling on Netdata. Unsupported item types must be rejected with explicit errors.
     - Initial implementation must cover at least command/script execution (Nagios-style), HTTP(S) requests (Go `net/http` with TLS/auth headers), and SNMP queries (reuse the existing go.d gosnmp wrapper). Additional Zabbix item types can be layered later but must be validated explicitly if unsupported.
     - LLD block: pipeline steps (Zabbix-like syntax), macro definitions (which keys become macros and which also become Netdata labels), instance identity template (which macro combination yields unique instance IDs, sanitized for chart IDs), discovery schedule (default = each collection unless overridden).
     - Dependent pipelines: list of metrics per instance. Each entry defines chart context, chart title template, family, chart type, target data type (`int64` vs `double`), dimension name, units, and preprocessing steps (Zabbix-like). Because Netdata stores integers, the config should specify the desired data type so the runtime can automatically add the appropriate multipliers/divisors to convert floating-point outputs into scaled integers. Pipelines can target the same context/dimension (different instances) or different contexts entirely.
    - Scheduler reference: defaults to `default` scheduler if unspecified.
     - Guardrails: no practical cap is desired; if limits are needed (e.g., `max_instances`, `max_pipelines`, `max_dimensions`), set them to very high defaults (≥10k) so real-world workloads never hit them unless explicitly configured.
   - Schema validation must highlight exact pipeline step and parameter errors via zabbix-preproc `ValidateStep`/`ValidatePipeline`.

2. **Scheduler implementation**
   - Implement a scheduler manager that loads all scheduler definitions (including the implicit `default`), instantiates `runtime.Scheduler` per definition, and tracks assigned jobs.
   - Schedulers expose charts similar to today’s Nagios scheduler: queue length, workers busy, started/finished counters, retries, pending jobs, etc.
   - Virtual module `scheduler` exposes each scheduler as a job for dyncfg/UI: users can list schedulers, adjust worker counts/jitter/logging, and stop/start schedulers. CRUD operations must include safety checks (e.g., deleting a scheduler requires reassigning jobs or defaults to `default`). Scheduler jobs primarily manage configuration/lifecycle; their charts/resets are separate from Nagios/Zabbix job charts.
   - Default scheduler config: use sane defaults (e.g., workers=50, jitter ~10%). Retry intervals remain per job (scheduler just enforces them).

3. **Preprocessor lifecycle & library placement**
   - Relocate the `zabbix-preproc` library into the monorepo under `src/go/pkg/zabbixpreproc/` so it is first-party code that both scripts.d and future go.d modules can reuse. Copy the entire source tree (including tests and `testdata/`) into that directory and update Netdata’s `go.mod` with all downstream dependencies (goja, gosnmp fork, etc.). A simple copy is acceptable; we do not need to preserve the original git history.
   - Add any missing APIs (e.g., `ClearState(itemID string)`) in the standalone repo before copying so the in-tree version already has the required functionality.
   - Maintain a single shared `zabbixpreproc.Preprocessor` instance per Netdata node; pass the Netdata host identifier (or literal `default`) as `shardID`. Instantiate it during scripts.d module initialization and keep it in a manager struct that job handlers can reference. Jobs remain isolated by unique `{module}.{job}` item IDs.
   - Provide helper functions to map job metadata to `Value`/`Step` structures and to clear state when a job or instance is obsoleted (call the new `ClearState` instead of relying solely on TTL). If TTL-based cleanup remains enabled, align its interval with the obsoletion delay so the two stay in sync.

4. **Execution flow for Zabbix jobs**
   - Scheduler triggers job → collection method runs (external command, HTTP, SNMP, etc.) using Zabbix-like macros/arguments.
   - LLD pipeline (if defined) runs every collection unless a slower interval is configured. Its JSON output drives instance management: new instances create chart templates + dependent pipeline bindings; missing instances are marked obsolete immediately (charts removed via Netdata’s obsoletion flow; data retained until DB rotation).
   - Dependent pipelines execute for each active instance: macros (e.g., `{#FSNAME}`) are expanded at runtime using the stored discovery data; pipeline runs through zabbix-preproc, yielding exactly one scalar per pipeline. Each result maps to a chart/dimension defined in the pipeline config.
   - Multi-metric step types (>=60) are reserved for LLD/discovery use cases only (per Costa). Standard pipelines always emit one metric = one dimension.

5. **Metric/charts mapping**
   - LLD configuration determines instance-level metadata: chart ID template, family, labels. Instances are not first-class structures in Netdata; the LLD acts as a multiplier, creating per-instance chart IDs and labels according to NIDL guidelines (see docs).
   - Chart IDs should follow `{module}.{job}.{instance_id}.{context_suffix}` so they remain unique and traceable (e.g., `zabbix.disk_monitoring.disk_sda.iops`). Context suffix can default to the last segment of the context string unless overridden.
   - Dependent pipeline → one dimension. Pipelines may share a context/dimension (across instances) or define distinct contexts. For example, with two pipelines (reads/writes) and five disks, we emit five charts per pipeline context (one per disk) with two dimensions per chart. Pipeline definitions include target data type so we can apply automatic multipliers/divisors internally when the user wants floating-point metrics scaled into Netdata’s integer storage (default precision 3 decimals unless overridden).
   - Contexts represent the metric name (Prometheus-style). Same context can aggregate across disks/nodes. Chart IDs remain instance-specific (e.g., `zabbix.disk.sda.iops`). Labels derived from macros (e.g., `{#DEVNAME}`) attach to dimensions so multi-node dashboards can filter by device.

6. **Error semantics**
   - `ErrorActionDefault`: mark the instance (and possibly job) iteration as failed. Scheduler’s retry logic (max attempts, soft/hard state) governs re-runs. For Zabbix-style state charts, adopt Zabbix semantics (OK/PROBLEM) while integrating with Netdata alerts.
   - `ErrorActionDiscard`: treat iteration as successful but skip chart updates for that pipeline/instance. No retry triggered.
   - `ErrorActionSetValue/SetError`: pipeline returns the provided value/error string; chart updates with that result, and state reflects the configured severity.
   - Partial failures: each instance maintains its own state chart. If 3 of 10 instances fail, only those three state charts enter PROBLEM while the job-level overview chart aggregates (e.g., counts of OK vs PROBLEM instances).
   - Scheduler itself exposes its own state/throughput charts (queue length, running jobs, successes/failures) similar to current Nagios plugin metrics.

6a. **State charts and alerts**
   - Every scheduler exposes scheduler-level charts (queue, running jobs, retries, etc.).
   - Every job (Nagios or Zabbix) exposes its own state chart reflecting the job’s definition (Nagios retains OK/WARNING/CRITICAL/UNKNOWN semantics; Zabbix uses OK/PROBLEM semantics). These charts enable Netdata alarms without relying on the scheduler.
   - Zabbix job state chart should break down failure causes (e.g., `collect_failure`, `lld_failure`, `extraction_failure`, `dimension_failure`) as separate dimensions so operators can see exactly what failed when the state enters PROBLEM.
   - Zabbix jobs with LLD additionally create one state chart per discovered instance so users can alert on instance-specific failures. Instance charts also track discrete failure causes (dimension-level failures vs collection/LLD issues) so partial failures are transparent.
   - State transitions are triggered by collection failures, LLD errors, or dependent pipeline failures according to the configured error handlers (default failure → PROBLEM, discard → stay OK, set-error → follow specified severity).
   - All state charts must be obsoleted/cleaned automatically when the corresponding job or instance disappears (either via LLD removal or dyncfg stop).

7. **LLD orchestration specifics**
   - LLD runs per collection by default; allow optional `discovery_interval` to run it less frequently.
   - Discovery objects must declare which macros become labels and which combination forms the unique instance key. Store discovered macro sets in memory (per job) and update dependent pipelines accordingly.
   - Removal: as soon as LLD output omits an instance, mark it obsolete (trigger chart removal and library state cleanup). Provide a configurable counter (e.g., obsolete after N consecutive misses/failed collections) if needed to avoid flapping, matching Costa’s “declare obsoletion and cleanup after X failed data collections” guidance.
   - Macro expansion: dependent pipelines reference macros in their step parameters (e.g., JSONPath filters). Expansion occurs at each execution using stored macro values (string substitution before passing to the library). Macro values also become Netdata labels automatically.
   - Multi-metric step types (≥60) are reserved for LLD/discovery transformations only. Standard dependent pipelines always emit exactly one metric/dimension.

8. **Testing and validation**
   - Schema unit tests verifying scheduler CRUD, Zabbix job definitions (collection, LLD, dependent pipelines), macro validation, and guardrail enforcement.
   - Functional tests for LLD/discovery lifecycle (add/remove instances, macro expansion, chart obsoletion, preprocessor cleanup).
   - Tests ensuring contexts/dimensions stay consistent across multiple instances/pipelines.
   - Scheduler tests covering queue metrics, worker utilization, retries, and default scheduler behavior.
   - Regression tests for existing Nagios code paths.
   - Optionally integrate upstream zabbix-preproc test suite runs when bumping dependency versions.

## Implementation snapshot (2025-11-16)
### What already exists
- **Single-job Nagios collector**: go.d now instantiates one scripts.d job per config entry. The collector registers with the scheduler registry (instead of spinning shard schedulers), builds per-job charts via `charts.JobIdentity`, validates vnode aliases up front, and filters shared scheduler metrics so it only emits its own chart series.
- **Scheduler registry owning workers**: the scheduler module exposes worker pools/OTLP settings, while `pkg/schedulers` keeps an in-memory registry + runtime hosts. Modules attach/detach jobs through `AttachJob`/`DetachJob`, so the runtime no longer needs shard-specific wiring.
- **In-tree zabbixpreproc library**: the standalone repo now lives under `src/go/pkg/zabbixpreproc/`, complete with docs/tests, and `src/go/go.mod` already pulls in the needed deps (`goja`, gosnmp fork, etc.).
- **Baseline scheduler plumbing**: the go.d `scheduler` module plus `pkg/runtime` executor still provide async execution, OTLP/log emitters, macro expansion, perfdata scaling, and chart wiring. Integration tests under `src/go/plugin/scripts.d/tests/mock_integration_test.go` cover these flows.
- **Zabbix job schema + validation**: `pkg/zabbix/config.go` covers collection/LLD/pipeline structs, and the go.d schema (`modules/zabbix/config_schema.json`) now mirrors these definitions so UI/dyncfg can build jobs with full metadata. The UI schema exposes four tabs (General, Collection, Discovery, Metrics Extraction) with nested editors for pipeline arrays/steps.
- **Nagios UI schema**: `modules/nagios/config_schema.json` now ships a tabbed UI layout (General, Arguments, Timing, Environment) so dyncfg users can edit command jobs without hand-writing YAML.
- **Zabbix runtime + schedulers**: `pkg/zabbix/runtime.go` groups jobs by scheduler, builds `spec.JobConfig` entries, and wires them into shared `runtime.Scheduler` instances. A custom `jobCollector` executes command, HTTP, and SNMP collections via `ndexec`/`net/http`/`gosnmp`, so collection is no longer synchronous or shard-bound.
- **LLD & pipeline orchestration**: `modules/zabbix/emitter.go` reconciles discovery output, instantiates per-instance charts, expands macros into pipeline steps/labels, and calls `Preprocessor.ClearState` when instances disappear. Metrics flush through `Collector.Collect`, so scheduler telemetry and dependent pipeline data already reach Netdata charts.
- **Shared preprocessor manager**: `modules/zabbix/preprocessor.go` now exposes `acquirePreprocessor()` so every `Collector` reuses the same `zpre.Preprocessor` instance keyed by host, keeping history across reloads.
- **Base labels & vnode metadata**: pipeline charts now include `zabbix_job`, `zabbix_scheduler`, `zabbix_vnode`, `zabbix_pipeline`, and the legacy `instance` label (plus macro-driven `zabbix_instance`), so dashboards/UI can filter just like Nagios charts even when jobs target virtual nodes.
- **Job engine package**: `src/go/plugin/scripts.d/pkg/zabbix/jobengine/` wraps the shared preprocessor, exposes `NewJob → Process → Destroy`, tracks discovery catalogs, and emits per-job/instance failure flags. It currently ships ~150 table-driven tests (≥100 single-shot cases plus 50 multi-collection runs) that stress macros, variable expansion, stateful discovery, and failure semantics across JSONPath, CSV, XML/XPath, SNMP, Prometheus, and CSV-multi preprocessors, so new preprocessing methods must be added to this matrix before integration.
- **Module integration**: `modules/zabbix/emitter.go` now delegates collection results to the job engine, so discovery/orchestration/state handling lives in one place. The emitter simply maps `jobengine.Result` objects to Netdata charts and dimensions (including multi-series pipelines via the new `MetricResult.Labels` data).
- **Plugin wiring**: `cmd/scriptsdplugin` now imports the Zabbix and Scheduler modules (alongside Nagios), registering their dyncfg schemas so the cloud UI can add/update scripts.d jobs for all three modules without manual YAML edits.
- **Default scheduler stock job**: a stock `scripts.d/scheduler.conf` ships the builtin `default` worker pool so dyncfg lists it alongside other scheduler jobs (users still override it through dyncfg or user configs).
- **Inline Zabbix job schema**: Zabbix configs now follow the go.d convention (one job per config), reusing the dyncfg job name instead of asking for it inside the schema; legacy `jobs:` arrays remain supported for handwritten configs, but the UI surfaces the streamlined single-job form with the Collection/Discovery/Metrics tabs Costa requested.

### UI / dyncfg schema reference (cloud-frontend @ 2025-11-16)
- The cloud UI renders module forms via `src/domains/configuration/components/configForm` (see `objectFieldTemplate` + `tabsLayout`). Setting `ui:flavour: "tabs"` on an object and providing `ui:options.tabs` (array of `{ "title": string, "fields": [] }`) arranges child properties into named tabs; `ui:options.rest` lists fields that should stay outside the tab control.
- Panels support collapsible headers, default expansion, and markdown-capable helper text using `ui:collapsible`, `ui:initiallyExpanded`, and `ui:help`. (Example: `go.d/collector/nvidia_smi/config_schema.json`).
- Array editors (`ArrayFieldTemplate`) automatically expose Tabs/List views, drag-and-drop reordering, copy/paste, and an optional `ui:openEmptyItem`. `ui:listFlavour: "list"` forces list mode. Each item renders a nested form, so pipeline arrays and preprocessing step arrays are first-class citizens.
- Tabbed arrays (`tabs.js`) currently label items as “Item N”, but they clone each child form so we can include explicit `name`/`title` fields inside the schema to make the content self-describing.
- Existing go.d schemas already follow the three-tab model we need (collection, selectors/LLD, auth/advanced). Zabbix can mirror this by building three parent sections: **Collection** (command/http/snmp), **Discovery** (LLD settings/steps), and **Metrics Extraction** (array of dependent pipelines). Each dependent pipeline object would itself expose metadata fields and an array of preprocessing steps.
- Step/pipeline definitions should document the available preprocessors (JSONPath, CSV, Prometheus, SNMP, etc.) inside `description`/`ui:help` so users know which macros (`{#MACRO}`, `{ITEM.VALUE}`, etc.) are valid. The UI simply renders what the schema describes; no extra front-end work is needed once the schema is in place.
- **State telemetry plumbing**: every job now emits a `zabbix.<job>.job.state` chart with the agreed dimensions (`collect_failure`, `lld_failure`, `extraction_failure`, `dimension_failure`, `ok`), and each discovered instance gets its own chart tracking the same signals so partial failures surface immediately.
- **Vnode registry enforcement**: the module loads `config/vnodes/`, resolves aliases into `runtime.VnodeInfo`, and hands a lookup to the scheduler so macros use the remote host metadata. Jobs referencing unknown vnodes now fail during init, while blank vnode fields fall back to the local host.
- **E2E acceptance tests (in progress)**: we’re committing to ~100 mock end-to-end scenarios ((collection payload → LLD → dependent pipeline → metrics/state) covering single/multi metric, multi-instance, macro expansion, and failure semantics) so every preprocessing feature has regression coverage.

- **Zabbix still shard-based**: only Nagios has been ported to single-job go.d configs. Zabbix configs/schemas/runtime still accept nested `jobs:` arrays, so dyncfg/UI can’t yet send/receive one-job payloads there.
- **Schemas & docs lag**: even though Nagios now runs per-job, the JSON schema, dyncfg helpers, and docs still describe shard-level defaults/directories/logging. They need to be rewritten to mirror go.d semantics (plus move plugin-wide knobs back to `scripts.d.conf`).
- **Directory auto-discovery**: Nagios configs still expose `directories` even though discovery should be a dedicated module later; keeping the field contradicts the new contract.
- **Virtual node routing in Zabbix**: Nagios validates vnode aliases up front, but Zabbix still lacks a `VirtualNode()` override, so remote charts continue to land on the parent host.
- **Testing harness**: `go test` for `pkg/runtime`/`pkg/schedulers` currently fails outside Netdata’s GOPATH; need a documented GOPATH/module workflow before we rely on those tests in CI.

### Immediate risks / cleanups
- Lack of module-level vnode routing means multi-tenant data lands on the parent host, which is misleading for users expecting per-vnode isolation.
- Dyncfg unusable until schemas/files/payloads align (current UI errors block Costa from testing).

### Decisions / clarifications needed from Costa
- Costa mandated (2025-11-16 follow-up): drop the “shard” concept entirely. Each module config mirrors go.d semantics (one job per file, `name`/`update_every`/`vnode` per job). Plugin-level knobs live in `/etc/netdata/scripts.d.conf`.
- Remove Nagios directory auto-discovery for now; reintroduce via `nagios_dir_watcher` when ready.
- Enums must be human-readable strings (e.g., `jsonpath`, `csv_to_json`) in both YAML and UI—no numeric codes exposed to users.

## Decisions (confirmed / outstanding)
1. **Config shape** → Use Zabbix-like step arrays and collection semantics (Costa: “new module, feel natural, human friendly when possible but align with Zabbix semantics”). Dependent pipelines must declare chart metadata explicitly.
2. **Scheduler model** → Option 2: configurable schedulers as virtual module jobs with a default scheduler (Costa).
3. **Timeframes** → No time periods; all jobs run 24×7 (Costa). Any legacy `CheckPeriod` fields are ignored/removed for Nagios/Zabbix integrations.
4. **Preprocessor ownership** → Single shared `Preprocessor` instance per Netdata node; use Netdata host identifier (or literal `default`) as shard ID. Cleanup occurs when collectors obsolete jobs/instances (Costa reaffirmed 2025-11-16).
5. **itemID mapping** → Use `{module}.{job}` to satisfy uniqueness for charts, scripts.d jobs, and library state (Costa allows passing full name even if job alone is unique).
6. **Execution stage** → Preprocessing applies only to Zabbix jobs after collection; Nagios jobs remain unchanged (Costa).
7. **LLD integration** → Zabbix jobs encapsulate collection + LLD + dependent pipelines. LLD results drive instance creation (chart ID, family, labels), macros become Netdata labels, and dependent pipelines define contexts/dimensions (Costa).
8. **Error semantics & retries** → Each instance needs its own state chart (Costa). Scheduler charts (queue, counts) remain separate. Need explicit mapping of error handlers to state transitions and retries (this doc now captures high-level expectation).
9. **Metric mapping** → Context = measurement type; chart = instance-specific; dimension = pipeline result. LLD determines instance metadata; dependent pipeline determines context/dimension. Documented in this TODO per Costa’s guidance.
10. **State persistence** → No persistence; state resets on restart (Costa).
11. **Nagios migration** → No migration tooling needed for existing Nagios configs (current scripts.d Nagios work is still in PR and not merged). Legacy behavior can remain untouched until new model lands (Costa).
12. **Scheduler jitter semantics** → Jitter remains a per-job control only; scheduler-level jitter settings are ignored (Costa prefers per-job only on 2025-11-16).
13. **Zabbix config schema exposure** → Flesh out the go.d schema now so dyncfg/UI tooling can validate collection/LLD/pipeline blocks before runtime support ships (decision taken 2025-11-16 after Costa delegated choice).
14. **Vnode assignment semantics** → Each job carries an optional `vnode` value; blank means “emit on the local Netdata host,” while specifying a vnode mandates that it already exists in the registry (unknown aliases are fatal during job init).
15. **Config file parity** → `/etc/netdata/scripts.d/*.conf` must match dyncfg payloads exactly. Each file describes a single job for its module (Nagios, Zabbix, Scheduler, future `nagios_dir_watcher`). Plugin-level settings (logging, watcher debounce, executor pools) belong in `/etc/netdata/scripts.d.conf` only.
16. **Config pattern parity (MANDATORY)** → Scripts.d configs (file layout, `jobs:` arrays, one module per file) must be identical to go.d configs. Use the same YAML schema, the same `jobs:` array wrapping, and the same semantics that dyncfg/go.d expect (see `src/go/plugin/go.d/config/go.d/nginx.conf`). Any shard-specific fields, wrappers, or directory loaders are forbidden—this pattern was wrong and is now removed.
17. **Virtual node handling (MANDATORY)** → Scripts.d follows go.d’s vnode contract exactly: modules implement `VirtualNode()` and rely on the go.d agent to route charts to the configured vnode (reference `src/go/plugin/go.d/collector/snmp/collector.go:175`). No alternative routing layers.
18. **Scheduler ownership (MANDATORY)** → The `scheduler` module is the sole owner of scheduler definitions. Nagios and Zabbix modules may only bind their jobs to schedulers already registered via that module; they must never create or mutate scheduler pools themselves. Sharing workers/logging happens through the scheduler registry.
18. **State persistence** → No on-disk persistence for LLD/state; restart resets counters (Netdata jobs are long-lived, so per-run state only).
19. **Delivery order** → Focus on a working happy path first, then expand tests, then documentation (Costa, 2025-11-16). Docs can lag until functionality + coverage are stable.

## Implementation audit (2025-11-16)
- Scheduler definitions already exist as go.d-style jobs: the module registers itself, exposes worker/queue/logging knobs, and applies definitions through the shared registry so schedulers can be CRUDed via configs or dyncfg (`src/go/plugin/scripts.d/modules/scheduler/module.go:22-185`). The registry keeps the builtin default and spins runtime hosts on demand (`src/go/plugin/scripts.d/pkg/schedulers/manager.go:33-199`).
- The runtime scheduler/executor stack supports standalone jobs with OTLP logging, macro expansion, vnode routing, and skip counters; it is exercised via the mock integration tests under `plugin/scripts.d/tests` (`src/go/plugin/scripts.d/pkg/runtime/scheduler.go:21-210`, `src/go/plugin/scripts.d/pkg/runtime/emitter_otel.go:1-120`, `src/go/plugin/scripts.d/tests/mock_integration_test.go:1-140`).
- Nagios jobs are already autonomous go.d jobs that validate vnode aliases, bind to schedulers, and register OTLP emitters/charts per job, replacing the shard model end-to-end (`src/go/plugin/scripts.d/modules/nagios/module.go:23-190`).
- The Zabbix stack (module + runtime + job engine + emitter) is in-tree: configs flatten `jobs:` arrays, vnode lookups happen before scheduling, and a shared `zabbixpreproc` instance drives LLD/dependent pipelines with per-job and per-instance state charts (`src/go/plugin/scripts.d/modules/zabbix/module.go:25-135`, `src/go/plugin/scripts.d/pkg/zabbix/runtime.go:25-205`, `src/go/plugin/scripts.d/pkg/zabbix/jobengine/job.go:209-337`, `src/go/plugin/scripts.d/modules/zabbix/emitter.go:61-200,240-360`).
- Test coverage matches the stated targets: `pkg/zabbix/jobengine/job_test.go` includes ≥50 stateful scenarios plus extensive single-shot cases (`lines 260-360`), `modules/zabbix/emitter_test.go` validates chart/state wiring, `pkg/zabbix/runtime_test.go` exercises spec generation, and the mock scheduler tests cover perfdata/macro behavior.
- Stock configs for Nagios and scheduler jobs already live under `src/go/plugin/scripts.d/config/scripts.d/`, so operators can define explicit jobs and workers today (`nagios.conf`, `scheduler.conf`).

## Issues (2025-11-17)

### Critical bugs (2025-11-17)
1. ~~Scheduler charts never registered~~ **(DONE)**
2. ~~Zabbix error_handler parsing unimplemented~~ **(DONE)**
3. ~~Zabbix step “type” uses numeric IDs~~ **(DONE)**
4. **Zabbix runtime bypasses shared scheduler registry** – jobs instantiate their own schedulers instead of using `schedulers.AttachJob`, so configured pools/metrics are ignored.
5. ~~VirtualNode() contract not implemented~~ **(DONE)**
6. ~~Scheduler ApplyDefinition ignores updates~~ **(DONE)**
7. ~~Zabbix `pipeline.unit` not validated in Go~~ **(DONE)**
8. ~~`update_every` validator allows zero~~ **(DONE)**
9. ~~Duplicate Zabbix job names overwrite each other~~ **(DONE)**
10. ~~LLD miss counter off-by-one~~ **(DONE)**
11. ~~Dependent pipelines accept multi-metric outputs~~ **(DONE)**

### Missing feature parity
1. **Collection macro substitution (host/item/user macros)**
   - TODO demands native Zabbix macros for commands/HTTP/SNMP. Currently, only LLD macros are expanded; collection payloads run literals.
2. **HTTP/SNMP config knobs ignored**
   - HTTP TLS flag, legacy method aliases, and SNMP context fields exist in the schema but are never used in runtime collectors.
3. **SNMP limited to single GET**
   - No SNMP walk/bulk support despite TODO highlighting multi-row discovery.
4. ~~VirtualNode lookup contract~~ **(DONE)**
5. **Schema completeness** – Nagios schema still exposes now-obsolete knobs (`time_periods`, per-job logging). We only need `user_macros` at the module level; time periods were a Nagios load-management hack that Netdata doesn’t need, and logging must stay global (one sink for the whole plugin). Zabbix UI help still references numeric IDs (needs string-enum wording) and dyncfg schemas must match the new macro layer.
6. **Documentation cleanup** – remove remaining shard/time_period references, describe scheduler telemetry, and document the new macro/TLS/SNMP behavior in dyncfg/UI help.
7. **Test coverage gaps** – add module lifecycle tests, HTTP/SNMP collector tests (including SNMP walk), scheduler chart tests, error handler parsing tests, vnode fallbacks.

### Plan to fix bugs (priority order)
1. Wire scheduler charts: call `charts.BuildSchedulerCharts` in `modules/scheduler.Init`, keep chart handles, and make `Collect()` feed real metrics.
2. Implement error_handler parsing:
   - Add custom type to `pkg/zabbix/config.go` to convert string enums into `zpre.ErrorHandler`.
   - Update schema to match enum values.
3. Convert Zabbix step `type` to human-readable enums:
   - Define string constants (jsonpath, csv_to_json, etc.), map them to zpre IDs inside config loader.
4. Update `JobConfig.Validate()` to reject `update_every <= 0` (when provided) and fix LLD `max_missing` comparison (`>=` instead of `>`).
5. ~~Detect duplicate Zabbix job names early~~ **(DONE)** – collector now errors when names repeat before runtime wiring.
6. ~~Enforce “one scalar per pipeline”~~ **(DONE)** – job engine rejects multi-metric outputs for dependent pipelines.
7. ~~Fix `PipelineConfig.validate()` to require `Unit`~~ **(DONE)** – validation now matches schema.
8. ~~Add explicit `VirtualNode()` implementations to both modules~~ **(DONE)** – go.d now drives vnode routing for Nagios/Zabbix.
9. ~~Allow scheduler definitions to reconfigure running hosts~~ **(DONE)** – scheduler manager rebuilds runtime hosts and preserves jobs on updates.
11. Ensure README/docs reflect the fixes (remove remaining shard references, describe scheduler charts, mention error_handler strings).

### Plan to deliver missing features (after bugfixes)
1. **Macro substitution layer** – (✅ commands/HTTP/SNMP now expand `{HOST.*}`, `{ITEM.*}`, `$USERn$`); remaining work: expose helper docs + tests for edge cases.
2. **HTTP/SNMP knob support** – (✅ HTTP TLS/method alias + SNMP context implemented); remaining work: document UI help + add collector tests.
3. **SNMP walk/bulk support** – (✅ runtime uses BulkWalk when pipelines reference walk steps); remaining work: add tests covering bulk paths.
4. **Schema completeness** – expose only the needed Nagios module fields: keep `user_macros`, drop `time_periods`, and ensure logging stays global (no per-job logging config). Keep scheduler/Zabbix schemas aligned with their structs and scrub the numeric ID help text.
5. **UI/doc alignment** – finish removing shard references, describe scheduler telemetry, document new step/error handler semantics.
6. **Test additions** – add module lifecycle tests, HTTP/SNMP collector tests, chart registration tests, scheduler chart tests, error handler parsing tests, vnode fallback coverage.

## External audit verification (Claude report, 2025-11-17)
- ✅ **UI hint still references numeric step IDs** – confirmed at `modules/zabbix/config_schema.json:630`; help copy must mention the enum names instead of numeric IDs.
- ✅ **`defaultInterval()` ignores `update_every`** – `pkg/zabbix/runtime.go:176-180` falls back to the collection timeout, violating the 60s cadence requirement.
- ✅ **Zabbix runtime bypasses the scheduler registry** – `pkg/zabbix/runtime.go:50-95` spins up private `runtime.Scheduler` instances instead of using `schedulers.AttachJob`, so jobs bound to the same scheduler name do not share worker pools or telemetry.
- ✅ **Collection macro substitution missing** – collectors (`pkg/zabbix/runtime.go:220-327`) execute commands/HTTP/SNMP with literal strings; `{HOST.*}`, `{ITEM.*}`, `$USERn$`, and `{#MACRO}` placeholders never expand outside dependent pipelines.
- ✅ **HTTP TLS knob ignored** – `HTTPConfig.TLS` is parsed in `pkg/zabbix/config.go` but `runHTTP()` never inspects or applies it, so per-job TLS requirements cannot be enforced.
- ✅ **SNMP limited to single GET** – `runSNMP()` always issues a single `Get` call; there is no Walk/BulkWalk implementation to feed multi-row preprocessing steps.
- ⚠️ **Scheduler metrics prefix already correct** – contrary to the report, the scheduler module filters on `"<name>.scheduler."` (module.go:175-193) and the runtime emits metrics with that prefix (`pkg/runtime/scheduler.go:538-544`), so charts update once jobs use the shared registry.
- ⚠️ **Per-job vnode routing works** – `c.vnodeLookup` returns a vnode per `spec.JobSpec` (`modules/zabbix/module.go:179-215`), and each registration copies it into `runtime.JobRegistration.Vnode` (`pkg/zabbix/runtime.go:75-85`), so data is tagged with the correct vnode. The `VirtualNode()` getter only exposes the latest vnode to go.d and is not used for routing.
- ✅ **SNMP context unused** – `snmpCfg.Context` exists in `pkg/zabbix/config.go:104-117` but is never set on the `gosnmp.GoSNMP` client.
- ✅ **`update_every` validator lets zero through** – `pkg/zabbix/config.go:342-347` only rejects negatives, so `0` incorrectly falls back to the timeout-derived schedule.
- ⚠️ **Remaining “shard” references exist only in this TODO** – repo-wide search shows no other live docs/configs mentioning shards; the report’s documentation concern is obsolete.
- ✅ **HTTP/SNMP collectors lack tests** – `pkg/zabbix/runtime_test.go` doesn’t cover `runHTTP()` or `runSNMP()`, leaving request/connection handling untested.
- ✅ **Nagios schema missing `name`** – `modules/nagios/config_schema.json` never defines the `name` property even though `spec.JobConfig` requires it, so dyncfg/UI cannot set explicit job names.
- ⚠️ **Scheduler metric naming mismatch** – duplicate of the earlier false alarm; metric IDs already align with `charts.SchedulerMetricKey`.
- ✅ **Error handler parsing lacks tests** – `parseErrorHandler()` has no dedicated tests despite its validation logic.
- ✅ **Dual input style still present** – `modules/zabbix/module.go:88-109` supports both inline single-job configs and `jobs:` arrays, conflicting with the “pure go.d pattern” directive and causing schema/UX ambiguity.

## Decisions (Costa, 2025-11-16)
- **Zabbix cadence** → Follow Netdata/go.d semantics: every job exposes `update_every`, and that interval drives scheduling. Action: add `update_every` to the Zabbix job schema/config and wire it through `buildJobSpecs`.
- **LLD persistence** → Keep discovery catalogs volatile (in-memory only). Action: ensure docs mention restart behavior and avoid designing on-disk caches.
- **Config cleanup** → Remove shard-specific fields (`defaults`, `directories`, watcher knobs) from `scripts.d.conf`, schemas, and docs immediately so configs mirror the actual implementation.
- **Job naming via dyncfg paths** → go.d/scripts.d job IDs originate from the dyncfg command arguments/paths rather than a `name` field embedded in the payload. Schemas should mirror go.d by keeping `name` implicit; `wrapJobPayloadIfNeeded`/`userConfigFromPayload` inject it only for round-trips. Action: document this behavior so new tooling doesn’t diverge.
- **Module-scope Nagios knobs** → Only `user_macros` needs to remain configurable at the module/file level. `time_periods` was a Nagios workload throttle and should be removed entirely (Netdata scheduling is already distributed), and logging must stay global—one sink per plugin, not per job. Action: clean up the schema/implementation accordingly so there’s no per-job logging block and no `time_periods` object.

## Plan (detailed)
1. **Update this TODO with final requirements** – keep all constraints explicit so there is no ambiguity during implementation. ✅
2. **Move the zabbix-preproc library in-tree** ✅ (complete)
   - Library copied into `src/go/pkg/zabbixpreproc/` with tests and docs. Dependencies (goja, gosnmp fork) added to `src/go/go.mod`.
   - `ClearState(itemID string)` + TTL cleanup shipped in `preproc.go` with `state_isolation_test.go`, so the library side of lifecycle management is done; runtime still has to call it.
3. **Introduce the scheduler module** ✅
   - Add `src/go/plugin/scripts.d/modules/scheduler/` with a config schema (workers, queue size, jitter, OTLP logging, labels) exposed as go.d jobs so dyncfg/IaC can CRUD schedulers.
   - Instantiate `runtime.Scheduler` per config, provide a default scheduler, and expose scheduler metrics (queue, running jobs, retries).
4. **Refactor scripts.d job handling** ✅
   - Both Nagios and Zabbix jobs now feed `runtime.Scheduler` instances and run independently of shard restarts. Remaining work for Zabbix lives in the follow-up steps (shared preprocessor, state telemetry, schema polish).
5. **Implement the Zabbix module** – ⏳ in progress
	- ✅ Non-command collection types now use synthetic plugin identifiers, so HTTP/SNMP jobs clear `spec.JobConfig` validation and reach the scheduler runtime.
	- ✅ Vnode awareness + labels land on every chart via the registry lookup; invalid vnode names now fail fast.
	- ✅ Job/instance state charts publish `collect_failure`, `lld_failure`, `extraction_failure`, `dimension_failure`, and `ok` signals each iteration.
	- ✅ `update_every` plumbed end-to-end (schema, config, runtime, docs) so cadence matches go.d semantics.
	- Harden LLD lifecycle (document volatile miss counters, ensure macro substitutions cover labels/context/unit templates).
6. **Extract the Zabbix job executor library** – ✅ complete
	- `pkg/zabbix/jobengine` now wraps `zabbixpreproc`, exposes `NewJob/Process/Destroy`, and manages the discovery catalog + per-job failure flags so modules simply map outputs to Netdata charts.
	- 150+ table-driven tests already exist (≥100 single-run cases plus 50 stateful runs) that stress macro expansion, state transitions, and variable substitution across JSONPath, CSV, XML/XPath, SNMP, Prometheus, and CSV-multi pipelines.
	- `modules/zabbix/emitter.go` has been refactored to call the job engine directly; scripts.d no longer reimplements discovery/pipeline orchestration and simply projects `MetricResult` data (including multi-metric outputs) into Netdata charts/dimensions.
7. **Wire the shared preprocessor** – ✅ (singleton landed)
	- `modules/zabbix/preprocessor.go` now exposes `acquirePreprocessor()` so every collector shares the same `zpre.Preprocessor` per host.
	- Job removal/path shutdown now calls into the emitter to clear all pipeline/LDD state and drop charts so the shared preprocessor doesn’t leak state across reloads.
8. **Chart + state mapping** – ✅ (state telemetry shipped)
   - Job-level and per-instance state charts now emit failure dimensions plus an `ok` flag; we can iterate later if alarm requirements evolve.
9. **Testing** – in progress
   - Unit tests for scheduler module, Zabbix schema parsing, LLD lifecycle, dependent pipelines, state chart logic, and error handler mapping.
   - Build a mock end-to-end suite (target ≥100 scenarios) that exercises collection payloads, LLD pipelines, dependent preprocessing (single/multi metric, macro expansion), and state/error propagation.
   - Library tests run in-tree; consider integrating upstream compatibility suites if updated.
10. **Documentation** – pending
   - Update docs to cover the scheduler module, Zabbix job configuration (collection methods/macros, LLD, dependent pipelines), chart/state conventions, and the “full feature” delivery requirement.
   - Provide migration guidance for Zabbix users (manual template translation, macro usage).
11. **Configuration realignment** – ⏳ NEW
   - ✅ Sample `scripts.d.conf` switched to go.d-style toggles (no shard defaults/directories/watchers). README reflects the simplified layout + cadence guidance.
   - Fix `userConfigFromPayload` + dyncfg update paths so a single job object (matching the job schema) is returned/accepted while the on-disk YAML remains multi-job. Add regression tests using real Nagios/Zabbix configs like Costa’s `ssl_certificates` example.
   - Rebuild Nagios/Zabbix/Scheduler JSON schemas to match the real file structures exactly (human-readable enums such as `jsonpath`, `csv_to_json` only).
   - Document the new layering (plugin conf vs module jobs vs scheduler jobs) and scrub any remaining “shard” terminology from the code/docs, including a note that LLD state is volatile by design.

## Implied decisions
- Default empty/whitespace `scheduler` fields in Zabbix jobs to the registered `default` scheduler (same behavior as Nagios jobs) so existing configs stay minimal but power users can bind to explicit schedulers.
- Use the fully-qualified `{module}.{job}` string for `itemID` propagation into the preprocessor and chart contexts; this satisfies uniqueness without inventing another naming scheme.
- Reuse the existing `runtime.Scheduler` machinery (per-job jitter, OTLP logging, retry counters) instead of creating a Zabbix-specific executor so operators see identical scheduler metrics across modules.
- Treat scheduler definitions as the single source of truth; the Zabbix runtime must refuse to auto-create worker pools if a referenced scheduler is missing and surface a configuration error instead.

## Testing requirements
- Job engine suite must keep ≥100 single-shot scenarios *plus* ≥50 multi-collection/stateful scenarios that cover every supported LLD/preprocessing methodology (JSONPath, CSV, XML/XPath, SNMP walk, Prometheus multi, etc.), include pass + failure modes (added/removed instances, macro expansion issues, dependent pipeline errors), and run within ≲2 seconds.
- Schema parsing tests for scheduler and Zabbix job definitions.
- Unit tests covering LLD → dependent pipeline orchestration and macro expansion.
- Scheduler integration tests verifying standalone job reloads without affecting others.
- Multi-metric pipeline tests ensuring contexts remain homogeneous.
- Regression tests for legacy Nagios workflows.
- (Optional) External verification by running `go test ./...` inside `~/src/zabbix-preproc` when updating dependency versions.

## Documentation updates
- Scripts.d module docs (and UI/dyncfg references) must describe the new scheduler abstraction, default scheduler behavior, and how to assign jobs to schedulers.
- Zabbix job documentation detailing collection commands, LLD configuration, dependent pipeline templates, metric metadata, and how to map to Netdata charts.
- Developer documentation describing the lifecycle: job creation, LLD updates, dependent pipeline management, error semantics, and how zabbix-preproc is invoked.
