# TODO - Shared Tool Queue Manager

## TL;DR
We must replace the per-session concurrency guard in `ToolsOrchestrator` with a process-wide queue mechanism so MCP/REST tools honor shared capacity limits (e.g., Playwright). Queues are configured centrally in `.ai-agent.json` (`queues.{name}.concurrent`, with a mandatory `default`) and every MCP/REST tool binds to a named queue (falling back to `default`). Agents and internal tools no longer acquire slots; legacy knobs (`maxConcurrentTools`, `parallelToolCalls`) disappear from config, CLI, and frontmatter. The new queue manager always mediates tool calls, logs when work is queued, and emits telemetry for queue depth + wait duration.

## Current State Analysis
- **Existing limiter**: `src/tools/tools.ts:950-1045` keeps `slotsInUse` per session using `maxConcurrentTools` and `parallelToolCalls`. This only throttles tool calls within one agent turn, offering no protection when multiple sessions invoke the same heavy MCP server.
- **Configuration plumbing**:
  - `maxConcurrentTools`/`parallelToolCalls` appear throughout `src/options-registry.ts`, `src/options-resolver.ts`, `src/agent-loader.ts`, `src/types.ts`, `src/config.ts`, CLI option handling (`src/cli.ts`), frontmatter parser (`src/frontmatter.ts`), and docs.
  - `.ai-agent.json` currently has no `queues` block; MCP server definitions (`MCPServerConfig`) and REST tools (`RestToolConfig`) lack a `queue` field.
- **Tool providers**: `MCPProvider` and `RestProvider` expose tools without concurrency metadata. `ToolsOrchestrator.executeWithManagement` blindly executes once the per-session slot is available.
- **Logging/telemetry**: Tools log only `started`/`finished`. Telemetry exposes tool metrics but no queue stats, so operators cannot see backlog levels or wait durations.
- **Tests/docs**: Phase 1 harness scenarios (e.g., `src/tests/phase1-harness.ts:2290+`) and docs (README, AI-AGENT-GUIDE, DESIGN, MULTI-AGENT) reference the soon-to-be-retired knobs.

## Requirements Agreed With Costa
1. Only MCP and REST tools participate in queues. Agents, sub-agents, and internal tools (progress, batch, final report, etc.) bypass queues entirely.
2. `.ai-agent.json` defines queues in a map: `"queues": { "fetcher": { "concurrent": 4 }, "default": { "concurrent": 64 } }`. Every tool must reference a queue via `queue: "name"`; omit means `default`.
   - When several configs provide the same queue name, the resolver must merge them by keeping the maximum `concurrent` value per name before handing them to the runtime.
3. A queue is always used (default ensures this). No separate timeouts—the tool’s own timeout bounds slot occupancy.
4. Remove `maxConcurrentTools`, `parallelToolCalls`, and any related flags/options from config/CLI/frontmatter/runtime. Tools always run “in parallel” subject only to queue capacity.
5. Logging: emit a `queued` log entry **only** when acquisition actually waits (i.e., wait time > 0). Immediate acquisitions still log only `started`/`finished`.
6. Telemetry: record per-queue depth (gauge) and last wait duration (gauge or histogram) so we can observe pressure.
7. Implementation surface should stay minimal—introduce a simple queue manager that plugs into existing providers without large refactors.

## Implementation Plan
0. **Bootstrap & Migration**
   - When loading config layers, if `queues` is missing, inject `{ default: { concurrent: DEFAULT_QUEUE_CAPACITY } }` where `DEFAULT_QUEUE_CAPACITY = 64` so existing configs continue to work. Per Costa’s direction, we will not ship any additional backward-compat shims beyond this auto-injection and updating the Netdata configs.
   - Immediately after merging config layers (before provider initialization) validate every referenced queue name (MCP server, REST tool, OpenAPI-generated tool) and throw a descriptive error if any queue is missing.
   - Document migration guidance in README/docs (see Documentation section) and explicitly note that `maxConcurrentTools` / `parallelToolCalls` frontmatter/CLI keys were intentionally removed.

1. **Schema & Types**
   - Extend `Configuration` (`src/types.ts`) with `queues: Record<string, { concurrent: number }>` (resolver guarantees it exists post-merge).
   - Update Zod schema in `src/config.ts` to require `queues`, enforce at least `default`, and validate `concurrent >= 1`.
   - Add `queue?: string` to `MCPServerConfig` (provider-level inheritance) and `RestToolConfig`; extend `OpenAPISpecConfig` with `queue?: string` so generated REST tools inherit provider queues automatically.
   - Ensure config resolver (`src/config-resolver.ts`) preserves `queues` when merging layers.
2. **Option & Frontmatter Cleanup**
   - Remove `maxConcurrentTools` / `parallelToolCalls` from `src/options-registry.ts`, `src/options-resolver.ts`, `src/options-schema.ts`, `src/frontmatter.ts`, `src/agent-loader.ts`, `AIAgentSessionConfig` (`src/types.ts`), and CLI wiring (`src/cli.ts`).
   - Delete associated defaults in docs (`AI-AGENT-GUIDE`, README, etc.) and any sample `.ai` files that set them.
3. **Queue Manager Module**
   - Introduce `src/tools/queue-manager.ts` exporting a singleton instance (e.g., `export const queueManager = new QueueManager();`).
   - API: `configureQueues(map)`, `acquire(queueName, { signal, agentId, toolName })`, `release(queueName)`, `getQueueStatus(queueName)`.
   - Track waiters FIFO. If `signal` aborts before slot acquisition (or the wait times out), remove the waiter and reject with a `DOMException` named `'AbortError'` **without** adjusting slot counters.
   - Surface instrumentation hooks (callbacks or events) for logging + telemetry (queue depth, wait duration timestamps).
4. **Provider Wiring & Tool Registry**
   - During MCP/REST provider initialization, resolve each tool’s queue name (`config.queue ?? 'default'`) and store it alongside tool metadata.
   - Extend the orchestrator mapping to include queue info (e.g., `Map<toolName, { provider, kind, queueName }>`).
   - For REST tools generated from OpenAPI (`src/tools/openapi-importer.ts`), respect the provider-level `queue` in `OpenAPISpecConfig`.

5. **ToolsOrchestrator & Session Changes**
   - Remove `slotsInUse`, `waiters`, `acquireSlot`, `releaseSlot`, and related logging from both `ToolsOrchestrator` and `AIAgentSession`.
   - In `executeWithManagementInternal`, resolve the tool entry first, determine whether it’s MCP/REST, and only then call `queueManager.acquire(queueName, { signal })`. Internal/agent tools bypass queues entirely.
   - Provide an `AbortSignal` per tool execution (backed by an `AbortController` tied to session cancellation) so queued waiters drop immediately when the session aborts.
   - Always release the queue slot in a `finally` block.
6. **Logging**
   - Define a new `LogEntry` (severity `VRB`, message `queued`) emitted when `QueueManager` reports a wait. Include queue name, depth when enqueued, and reason.
   - Keep existing `started`/`finished` logs unchanged for immediate executions.
7. **Telemetry**
   - Extend `src/telemetry/index.ts` with queue metrics helpers (`recordQueueDepth`, `recordQueueWait`). Export metric names `ai_agent_queue_depth{name="queue"}` (gauge) and `ai_agent_queue_wait_duration_ms{name="queue"}` (last-wait gauge + histogram buckets) to align with existing OTEL conventions.
   - Queue manager should call these whenever depth changes or a waiter resumes; gauge updates are cheap, so emitting per change is acceptable.
8. **Configuration Bootstrap**
   - Ensure `queueManager.configureQueues` is called in the global bootstrap path (CLI entrypoint before any `LoadedAgent` creation). When multiple configs provide overlapping queue names, merge them by taking the **maximum** declared concurrency per queue name (never shrink). Repeat calls that only raise limits should succeed; smaller values are ignored.
   - Providers must see queue metadata eagerly (i.e., config ready before `ToolsOrchestrator.register`).
9. **Testing**
   - Update Phase 1 harness to drop references to removed options.
   - Add deterministic scenarios for:
     - Queue serialization (capacity 1) producing `queued` log and sequential execution.
     - Cancellation before acquisition (waiter removed, next waiter proceeds, queue depth correct).
     - Tool timing out while queued (treat like abort: waiter removed prior to slot acquisition and error bubbled up).
     - Multiple queues concurrently (A saturated, B free) to confirm isolation.
     - Default queue fallback when tool omits `queue`.
     - Telemetry assertions (depth updates, wait duration recorded).
   - Add targeted unit tests for `QueueManager` (depth accounting, release ordering, telemetry hook invocation, abort semantics, double-configure safety).
10. **Docs & Samples**
   - Update README, docs/SPECS.md, AI-AGENT-GUIDE, DESIGN, MULTI-AGENT to describe queue config/behavior and explicitly state that legacy knobs are gone.
   - Refresh `.ai` samples (`doxy.ai`, `neda/*.ai`, etc.) to remove deprecated fields.

## Telemetry & Logging Details
- **Queue depth gauge**: track current length per queue whenever acquire/release changes it.
- **Wait duration metric**: record the wait time (ms) for each tool that had to queue; can be gauge of “last wait” or histogram depending on existing telemetry helpers.
- **Log payload**: `details` should include `queue`, `queued_at_depth`, `max_concurrent`, and `wait_ms`.

## Testing Plan
- Phase 1 deterministic harness: new scenarios covering serialized execution (capacity 1), cancellation-before-acquire, timeout-while-waiting, multi-queue behavior, default queue fallback, and telemetry assertions.
- Unit tests for `QueueManager` validating concurrency limits, FIFO ordering, abort handling, telemetry hooks, and double-configure safety.
- Regression on REST tools/OpenAPI importers to ensure queue metadata attaches correctly.

## Documentation Updates
   - `.ai-agent.json` reference + README sections describing `queues` and tool `queue` bindings (provider-level inheritance for MCP + OpenAPI REST providers).
   - Add migration guide / release notes instructing users to add `queues.default` or rely on auto-injected default (documented behavior).
- Remove mentions of `maxConcurrentTools` / `parallelToolCalls` from README, docs/AI-AGENT-GUIDE.md, docs/DESIGN.md, docs/MULTI-AGENT.md, sample prompts.
- Add guidance to AI-AGENT-GUIDE on observing queue metrics/logs and how to size `default` vs specialized queues.
   - Explicitly note that per-tool queue overrides are out of scope for this release and may be revisited if needed later.

## Open Items
- Telemetry: queue depth as gauge; wait duration should update both a gauge (last wait) and a histogram for distribution analysis.
- Validate whether we still need to enforce any relationship between `default.concurrent` and agent depth since agents no longer queue (likely no additional guard beyond `>= 1`).
- Provider-level queue assignment only (per-tool overrides deferred to avoid config bloat). OpenAPI-imported REST tools inherit the provider-level queue.
- Confirm desired default queue capacity constant (now `DEFAULT_QUEUE_CAPACITY = 64`).
