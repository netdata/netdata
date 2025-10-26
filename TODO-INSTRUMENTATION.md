# Instrumentation Roadmap

## Objective & Scope
Deliver a single, coherent observability story for `ai-agent`—regardless of whether it runs under systemd, as a CLI tool, or inside headends—by:

- Emitting **structured logs**, **OTLP metrics**, and **OTLP traces** that share common resource attributes and correlation identifiers.
- Supporting both long-lived services and short-lived shell invocations without blocking or adding noticeable latency.
- Making every signal machine-parseable (journald/logfmt/json) so downstream systems can automate ingestion, alerting, and auditing.
- Preserving human readability (logfmt/journald) while guaranteeing lossless structure for collectors (json/OTLP).
- Ensuring telemetry is **opt-in** (explicit flags/config required) with safe timeouts and drop-on-backpressure semantics.
- Consolidating log identifiers (`log_uid`, curated `MESSAGE_ID`s) and low-cardinality metric labels (`agent_id`, `call_path`, `tool`, `provider`, `headend`, etc.) so dashboards, traces, and log searches align.
- Providing shims/adapters for third-party libraries so **all** emitted logs flow through the same structured sink.
- Allowing operators to define custom labels (e.g., `environment=production`, `cluster=staging`) via `ai-agent.json` or CLI flags; all three signal types must carry the same resource attributes so correlation is trivial.

## Current State (October 25, 2025)
- Structured telemetry (logs, metrics, traces) is shipping and enabled by default for journald/logfmt, with OTLP export opt-in via config/CLI.
- Console logging flows through `makeTTYLogCallbacks` → `formatLog` (see `src/log-sink-tty.ts`, `src/log-formatter.ts`) and falls back to ANSI-coloured logfmt when journald is unavailable or the `systemd-cat-native` helper cannot be executed.
- `LogEntry` metadata is preserved end-to-end; sinks emit structured key/value fields rather than flattened strings.
- Warning/error paths route through the shared logger (`warn()` delegates to structured sinks), so journald/logfmt/json/OTLP receive identical context.
- Headends reuse the same callback interface (`onLog`) and therefore inherit the structured pipeline without adapter-specific formatting.
- Structured logging publishes dedicated tool metadata (`tool_provider`, `tool`) alongside provider/model; the contract lives in `docs/LOGS.md` and is referenced by the harness assertions.
- Journald detection verifies `$JOURNAL_STREAM` device/inode and honours `AI_FORCE_JOURNAL` / `AI_DISABLE_JOURNAL` overrides for manual control. When journald is active and `systemd-cat-native` is present, the sink streams Journal Export Format lines via the helper; otherwise the agent warns once and downgrades to logfmt.
- OpenTelemetry exporters for metrics/traces (and optional logs) initialise lazily when `telemetry.enabled` is set; default harness runs keep telemetry disabled to preserve deterministic output.
- Phase1 harness checks implied fields on key warnings/errors so regressions surface immediately in CI.
- Functional behaviour remains unchanged: instrumentation only adds telemetry and does not alter agent logic or tool execution flow.

## Detecting systemd journal context
- systemd exports `$JOURNAL_STREAM` when stdout/stderr are connected to journald; the value encodes the device/inode pair that should match the current stream descriptors.
- Recommendation: treat journald mode as active when *all* conditions hold:
  1. `process.env.JOURNAL_STREAM` exists.
  2. `process.stdout` (or `stderr`) is a stream and its device/inode match the env pair (compare via `fs.fstatSync`).
- Fallback markers (for user services or container orchestration):
  - Presence of `INVOCATION_ID` (also set by systemd) as secondary hint.
  - Allow manual override via CLI/env (`AI_FORCE_JOURNAL=1` / `AI_DISABLE_JOURNAL=1`).

## Logging (Structured) Architecture
1. **Central Log Event Builder**
   - Introduce a `StructuredLogEvent` abstraction mirroring `LogEntry` but normalised (field names + optional key remapping map).
   - Extend `LogEntry` creation sites to supply a stable journald-compatible `MESSAGE_ID`, registered centrally (e.g., `src/log-message-ids.ts`).
   - Ensure opTree instrumentation (progress/status updates) emits structured objects too; replace raw status strings with typed payloads so both journald and console sinks can render them consistently.
   - Attach a globally unique `log_uid` (monotonic counter or UUID) to **every** emitted log for traceability, inject custom operator labels, and map fields to journald-style uppercase keys while providing lowercase aliases for logfmt/json.
2. **Sink Selection**
   - During startup, evaluate detection logic and choose between:
     - `JournalSink`: emits via native journald API (preferred library: `systemd-journald` or minimal custom implementation that writes `KEY=value\0` records to `/run/systemd/journal/socket`).
     - `ConsoleSink`: emits logfmt by default for human readability; allow opt-in JSON using CLI flag/env (`AI_LOG_FORMAT=json`) for tooling that prefers JSON. Both share the same structured field set.
   - Support dual-output when `traceLLM` or `DEBUG=true` requests raw console mirroring.
3. **Field Mapping**
   - Journald requires uppercase keys; reuse its schema where possible (`MESSAGE_ID`, `PRIORITY`, etc.) and emit Netdata extras under `AI_*` prefixes (`AI_AGENT`, `AI_TOOL`, `AI_PROVIDER`, `AI_MODEL`, `AI_HEADEND`, `AI_REASONING_LEVEL`, `AI_LABEL_*`).
   - Console formatter uses lowercase equivalents (`message_id`, `agent`, `tool`, `provider`, `model`, `reasoning_level`, `msg`) plus user labels for readability.
   - Transaction/session IDs remain in logs (and traces) for correlation, not in metrics.
4. **Message ID Registry**
   - Maintain `docs/LOG-MESSAGE-IDS.md` describing each journald `MESSAGE_ID`, severity, and trigger.
   - Provide helper that rejects unregistered IDs so new events require explicit catalog updates.
5. **Testing + Validation**
   - Add unit tests ensuring detection toggles sinks correctly (mock `fs.fstatSync`).
   - Extend deterministic harness to capture structured output and confirm fields remain stable.
   - Manual integration checklist: run under `systemd-run --unit=ai-agent-test --property=StandardOutput=journal` and confirm `journalctl -o json-pretty` shows expected keys.
6. **Library/External Log Capture**
   - Patch `console.log/warn/error` at bootstrap to route through the structured sink with `source=console` metadata.
   - Offer adapters for common loggers (Winston transport, Pino destination, `debug` hook) so libraries configured by the agent emit structured events natively.
   - Set OpenTelemetry diagnostics logger via `diag.setLogger()` to reuse the same sink.
   - For stubborn dependencies writing directly to `process.stderr`, wrap the stream with a capturing writable that tags lines (`source=stderr`, `library=unknown`) and forwards them.
   - Child processes: read stdout/stderr pipes, truncate if necessary, and emit structured entries with `source=child::<command>`.
7. **Known Challenges**
   - Existing formatters (CLI, headends) expect strings; we must keep human-readable rendering without losing structure.
   - Session persistence pathways store string logs; update payloads or versioning to accommodate structured entries.
   - Warning sink (`warn()`) currently stringifies; adapt to structured logging or ensure warnings are injected via the new sink.
   - Capture of third-party `console.*` must happen before any library logs; bootstrap order is critical.
   - OTLP log export stays **manual opt-in**; defaults remain journald (under systemd) or logfmt.

## Challenges & Considerations
- **Journald API dependency**: pulling an external module may add native build steps; evaluate `systemd-journald` (C binding) vs pure JS (crafting datagrams). If dependency risk is high, fall back to invoking `sd_journal_send` via optional addon.
- **Performance**: structured logging generates objects per event; ensure reuse of buffers (e.g., precompute base metadata).
- **Backward compatibility**: CLI users piping to files expect plain text. Provide opt-in flag to preserve legacy formatting until the new sink stabilises.
- **Headends**: confirm Slack/OpenAI/Anthropic headends linking to `onLog` still forward structured metadata without reformatting.
- **Message size limits**: journald caps individual fields to 2^64-1 bytes but ships in RAM; avoid dumping huge tool outputs as structured fields—store references (hashes, truncated preview).
- **Logfmt nuances**: ensure quoting rules (space/newline handling) are consistent across transports; consider reusing battle-tested encoders (e.g., `pino-std-serializers` or `logfmt` npm) to avoid edge-case bugs.
- **JSON option**: formatting must stay single-line to keep downstream parsers efficient; re-use same structured payload (just `JSON.stringify(event)`).

## Metrics Strategy (OTLP-first)
- Adopt OpenTelemetry Metrics SDK with the OTLP gRPC exporter targeting a single configurable endpoint (default: Netdata’s OTLP receiver on localhost). Metrics and traces share this endpoint to simplify deployment.
- Configure a periodic reader with short export intervals (e.g., 5–10 s) so long-running agents stream metrics continuously.
- For short-lived shell executions, call the meter provider/exporter `shutdown()` in a `finally` block to flush any remaining batches before exit; set exporter timeouts low (≤2 s) and disable unbounded retries so shutdown stays fast.
- Keep metric instruments lightweight (counters/histograms for tool timings, gauges for queue depth). Avoid StatsD—pure OTLP is sufficient per decision.
- Label metrics with low-cardinality attributes only: `agent_id`, `call_path`, `tool_name`, `provider`, `model`, `headend`, `reasoning_level`, and operator-defined labels. **Do not include transaction/session IDs** to prevent cardinality explosions.
- Exporters should request **delta temporality** so multiple agent instances can push increments without double counting; the collector can convert to cumulative if a backend requires it.
- Merge operator-specified custom labels into the metric resource/attributes set.
- Document environment overrides for operators: `AI_OTLP_METRICS_ENDPOINT`, `AI_OTLP_METRICS_TIMEOUT`, etc., and allow disabling metrics entirely via `AI_OTLP_DISABLE=1`.
- When running in server/headend mode, expose an optional `/metrics` endpoint using the Prometheus exporter (disabled by default for CLI mode). Register both readers simultaneously (OTLP delta + Prometheus cumulative) so push and pull pipelines stay in sync.
- Add integration tests that verify: (a) OTLP exports deltas, (b) `/metrics` serves cumulative counters concurrently, (c) custom labels appear in both outputs.
- Risks/Decisions:
  - Metric taxonomy is fixed: **LLM** metrics must include bytes-in/out, tokens (prompt, completion, cache read, cache write), latency, error counts by type, retry counts, and cost. **Tool** metrics include bytes-in/out, latency, error counts, and run counts (no tokens/cost/retries). Document cardinality budget per label.
  - Ensure exporter shutdown does not block CLI exit; set ≤2 s timeout and log (rather than throw) on failure.
  - Provide toggle to disable telemetry in tests (`AI_TELEMETRY_DISABLE=1`).

## Tracing Strategy
- Use OpenTelemetry tracing with OTLP gRPC exporter pointed at the same endpoint used for metrics.
- Emit spans for **every agent**, **every LLM call**, **every tool call**, and **every sub-agent** (turn-level spans optional but allowed). Provide config knobs for sampling (default always-on for CLI; rate-limited/percentage for server mode).
- Ensure tracer provider shares the same `Resource` attributes as metrics/logs (`service.name=ai-agent`, version, deployment env) plus custom labels.
- Maintain parent/child relationships that mirror the opTree so sub-agent spans nest correctly.
- Flush the tracer (`shutdown()`) during graceful exit; guard with low timeout and catch errors to avoid impacting job completion.
- Add tests ensuring tracer shutdown respects timeouts, spans carry custom labels, and sampling knobs behave as expected.

## Logging + OTLP Alignment
- If OTLP log export is enabled, reuse the structured event builder and send to the configured OTLP endpoint while still writing to journald/logfmt/json locally. Multiple log sinks may co-exist (e.g., journald + OTLP).
- Provide configuration switches (in `ai-agent.json` and CLI flags) so operators can choose any combination of {journald/logfmt/json} + {OTLP logs on/off}; OTLP remains **disabled by default** and requires explicit enablement.
- Journald writes remain synchronous (best-effort, immediate visibility) while OTLP log export runs asynchronously; ensure back-pressure on OTLP does not block journald emission.
- Ensure buffering strategy drops gracefully under pressure (bounded queue with drop + warning). In CLI mode we log-and-drop; in server mode we retry with backoff before dropping.

## Operational Flow for Standalone Agents
1. On startup, detect journald vs console and initialise the structured log sink(s).
2. Create OTLP metric and trace providers (sharing resource attributes) pointing to the configured OTLP gRPC endpoint.
3. Run the agent; exporters push asynchronously in the background.
4. On shutdown (normal or interrupt), invoke `shutdown()` on metric + trace providers and the log sink (if OTLP logs are enabled), each with short timeouts; ignore failures after logging a warning.
- Tests should be able to bypass telemetry initialization entirely to keep harness deterministic.

## Configuration Surface (Draft)
- `telemetry.enabled` (boolean; defaults false so telemetry is opt-in)
- `telemetry.otlp.endpoint` (URL, shared by metrics & traces; default `grpc://localhost:4317`; used only when telemetry enabled)
- `telemetry.otlp.timeout_ms`, `telemetry.otlp.disable`
- `telemetry.logging.formats` (array subset of `["journald","logfmt","json","none"]`; default driven by journald detection: `journald` when available, otherwise `logfmt`)
- `telemetry.logging.extra` (array subset of `["otlp"]`; opt-in parallel sinks layered on top of the default)
- `telemetry.logging.otlp.endpoint` (defaults to `telemetry.otlp.endpoint` but can diverge)
- `telemetry.logging.message_ids.strict` (boolean to require pre-registered IDs)
- `telemetry.prometheus.enabled` (default false; when true, exposes `/metrics` on configurable host/port)
- `telemetry.prometheus.host`, `telemetry.prometheus.port`
- `telemetry.labels` (map of custom key/value labels merged into log/metric/trace resources)
- All keys should have matching CLI overrides (`--telemetry-otlp-endpoint`, `--telemetry-label env=prod`, etc.) and env vars (`AI_TELEMETRY_OTLP_ENDPOINT`).

## Next Steps
### Phase 0 – Design Review & Foundations
1. Finalize structured log schema, journald field mapping, `MESSAGE_ID` catalogue, and metric/span taxonomies (already outlined above). Resolve any outstanding questions (sampling defaults, label naming).
2. Confirm configuration schema (`telemetry.enabled`, OTLP endpoints, formats, Prometheus host/port, labels) and CLI/env overrides.
3. Document test matrix (unit, harness, integration) and toggles (telemetry opt-in/off for tests).
4. Keep `docs/LOGS.md` aligned with planned changes; treat it as the contract when reviewing new logging work.

### Phase 0.1 – Dependency & Bootstrap Assessment
1. Audit build pipeline for impact of new OpenTelemetry packages (tree-shaking, bundle size). Plan lazy-load strategy for CLI to keep startup fast.
2. Prototype minimal import guard to ensure telemetry modules initialize only when `telemetry.enabled` is true.
3. Decide package set to add (`@opentelemetry/api`, `@opentelemetry/sdk-node`, `@opentelemetry/sdk-metrics`, `@opentelemetry/sdk-trace-node`, `@opentelemetry/sdk-logs`, `@opentelemetry/exporter-metrics-otlp-grpc`, `@opentelemetry/exporter-trace-otlp-grpc`, `@opentelemetry/exporter-logs-otlp-grpc`, `@opentelemetry/resources`, `@opentelemetry/semantic-conventions`), keeping log exporter behind opt-in.
4. Telemetry manager should live in `src/telemetry/` and expose `initTelemetry(config)` / `shutdownTelemetry()`; dynamic `import()` inside to avoid loading OTEL libs when telemetry disabled.
5. Ensure tests/CLI default to telemetry disabled; provide helper to short-circuit initialization when env `AI_TELEMETRY_DISABLE=1` is set.

### Phase 1 – Structured Logging Infrastructure _(Status: Completed — October 24, 2025)_
1. Implement shared structured logging core:
   - `StructuredLogEvent` abstraction, journald/logfmt/json sinks, `log_uid`, custom label injection, `MESSAGE_ID` registry helper.
   - Emit dedicated tool metadata (`tool_provider`, `tool`) alongside provider/model, keeping backwards compatibility.
   - Bootstrap detection (journald vs console) and configuration toggles (`telemetry.logging.formats`, `telemetry.enabled`).
2. Refactor existing log emitters (`AIAgentSession`, `LLMClient`, `ToolsOrchestrator`, headends, CLI) to populate structured events (no metrics/traces yet).
3. Update persistence (session snapshots/accounting) and headend renderers to consume structured logs.
4. Add unit/integration tests for journald detection, structured output integrity, opt-in behaviour (telemetry disabled by default), and existing CLI snapshots.
5. Harden journald detection (device/inode verification + `AI_FORCE_JOURNAL` / `AI_DISABLE_JOURNAL` overrides) before enabling auto-selection in production.

### Phase 2 – Metrics Instrumentation _(Status: Completed — October 24, 2025)_
1. Introduce OTLP metric exporter (delta temporality) with opt-in configuration; add Prometheus exporter guarded by `telemetry.prometheus.*`.
2. Instrument LLM/tool operations using the required metric sets (bytes/tokens/latency/errors/retries/cost for LLM; bytes/latency/errors/count for tools) with low-cardinality labels + custom labels.
3. Ensure exporter shutdown obeys CLI vs server policies (log/drop vs retry/backoff). Add tests covering delta exports, Prometheus `/metrics`, and label propagation.

### Phase 3 – Tracing Instrumentation _(Status: Completed — October 24, 2025)_
1. Configure tracer provider/exporter at the shared OTLP endpoint with resource labels.
2. Emit spans for every agent, LLM call, tool call, and sub-agent; add optional turn spans if warranted. Implement sampling knobs (default on for CLI, configurable for server).
3. Wire span context propagation through sub-agents and opTree; integrate with existing callbacks/logging for correlation.
4. Add tests for span coverage, sampling behaviour, shutdown timeouts, and custom labels.

### Phase 4 – OTLP Log Export (Optional, Opt-in) _(Status: Completed — October 24, 2025)_
1. Add OTLP log exporter behind manual enable flag (`telemetry.logging.extra` includes `otlp`). Ensure buffering with drop policies mirrors CLI/server behaviour.
2. Validate coexistence of journald/logfmt and OTLP sinks; ensure enabling OTLP doesn’t impact default behaviour.

### Phase 5 – Documentation & Operational Guides _(In Progress)_
1. Update README / docs with configuration examples (opt-in telemetry, OTLP endpoint, Prometheus, custom labels, enabling OTLP logs).
2. Provide operational runbooks for CLI vs server modes, failure behaviour, and integration with Netdata collector.
3. Outline migration/compatibility notes (legacy logs vs structured, telemetry opt-in expectations).
4. Keep `docs/LOGS.md` and related references up to date as sinks/fields evolve (e.g., json option, journald overrides, `bold` deprecation path).

### Phase 6 – Final Hardening _(Pending)_
1. Benchmark CLI startup/perf impact; adjust lazy-loading if needed.
2. Run extended phase1 harness with telemetry toggled on/off; ensure deterministic logs when disabled.
3. Validate end-to-end with Netdata OTLP receiver and journald in real environment; smoke test `/metrics` endpoint and span exports.

## Research Notes (OTel SDK Best Practices)
- Use the latest OpenTelemetry JS SDK (0.203+); traces/metrics are GA, logs remain experimental but usable via the Logs SDK.
- OTLP push is the default; export deltas for metrics to avoid double counting across multiple agents, letting the collector convert when needed.
- Prometheus exporter (pull) can coexist with OTLP push by registering concurrent metric readers; each maintains independent aggregation state.
- Node auto-instrumentations (Winston/Pino) inject `trace_id`/`span_id`; integrate with our structured sink via custom transports.
- Journald detection relies on `$JOURNAL_STREAM` device/inode comparison; falls back to console formats when absent.

## Remaining Follow-Ups (Paused)
- **Documentation**: Finalise README and operational guides covering telemetry configuration, deployment recipes, and migration notes (Phase 5). *Deferred while instrumentation work is paused.*
- **Hardening**: Benchmark startup/perf, validate in-system environments, and capture findings (Phase 6). *Deferred until instrumentation work resumes.*

## Decisions (October 25, 2025)
- Sink selection rules are locked in:
- When running under systemd (journald detected) **and** `systemd-cat-native` is available, `journald` is the default sink.
  - When journald is unavailable, `logfmt` on `stderr` is the default.
  - The default sink is configurable to one of `none`, `journald`, `logfmt`, or `json` via both `ai-agent.json` and CLI flags.
  - OTLP log export is an optional parallel sink layered on top of the default and remains opt-in (no automatic enablement).
- Structured logging contexts are sourced directly from runtime owners:
  - Headend-level logs inherit identifiers such as `mode` and `headend` from the active headend/session manager.
  - Agent/session-level logs pull `agent`, `call_path`, `turn`, `subturn`, and reasoning metadata from `AIAgentSession` state.
  - LLM logs reuse the session’s current `provider`, `model`, token counts, and cache metrics without duplicating them in message bodies.
  - Tool logs reuse tool metadata (`tool`, `tool_kind`, provider) captured by the orchestrator.
- Log messages must focus on incremental information; implied context fields live exclusively in structured metadata.
- Stack traces are emitted only for warnings/errors in journald (`AI_STACKTRACE`), while logfmt/JSON sinks omit stack traces to stay concise.
- Logfmt/JSON writers render `message` last to improve readability when scanning lines.

## OpenTelemetry Integration (Logs, Metrics, Traces)
- **Log correlation**: Winston/Pino instrumentation packages inject `trace_id`, `span_id`, `trace_flags` into structured log records, enabling cross-signal correlation. Determine whether to standardise on Winston (`@opentelemetry/instrumentation-winston` + `@opentelemetry/winston-transport`) or expose hook points for alternative loggers (Pino).
- **Transport strategy**:
  - Journald mode: continue sending the same structured payload; if OTLP export is enabled, journald also receives full context.
  - Console mode: wrap the logfmt/JSON sink with an optional OpenTelemetry LogRecord exporter (likely via OTLP in-process exporter). Guard behind config to avoid double shipping.
- **Initialization order**: OTel instrumentation must load before logger libraries (via preload script or early bootstrap) to ensure span context injection works. Document this in README and CLI instructions.
- **Metrics/traces alignment**:
  - Use shared `Resource` attributes (`service.name`, `service.version`, `deployment.environment`) across logger, tracer, and meter providers so downstream collectors can correlate data.
  - Provide helper to register log attributes (e.g., `ai_agent.session_id`) and ensure message IDs map to semantic conventions (consider adopting ECS compatibility for journald keys).
- **Collector pipeline**: investigate bundling a reference OpenTelemetry Collector config that accepts OTLP logs/metrics/traces and can forward to journald if needed (for operators who prefer tailing local logs).

## Additional Investigations
- Evaluate whether to depend on Winston (already heavy) or build minimal custom emitter + optional OTEL hook (reduces dependency surface).
- Assess performance impact of synchronous journald writes; consider buffering with `node:worker_threads` or `async_hooks` context to avoid blocking the event loop under high load.
- Plan migration path for existing CLI ANSI output; provide compatibility flag (e.g., `AI_LEGACY_LOGS=1`) until users fully adopt structured modes.
- **Message ID ownership**: maintain `docs/LOG-MESSAGE-IDS.md` enumerating which log categories warrant a stable `MESSAGE_ID`; new IDs require review to avoid collisions and overuse. Use a helper (`registerMessageId`) that throws if an unapproved ID is requested.

## Work Status
- Instrumentation work is currently paused. Logs, metrics, and traces are fully implemented; revisit the deferred follow-ups above when the next instrumentation cycle begins.
