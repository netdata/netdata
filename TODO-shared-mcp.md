# TODO - Shared MCP lifecycle

## TL;DR
- Shared stdio MCP pooling (`MCPSharedRegistry`) is live and already the default; per-session providers simply acquire the shared handle unless `shared: false` is set.
- Costa has now required that the shared registry cover **all** MCP transports (stdio, websocket, http, sse); per-config entries are unique so there is no need for hashing/dup detection—every server name maps to exactly one config, and all call sites must share it.
- New directive (2025-11-12 15:10 UTC): **Any shared MCP server failure must trigger relentless restart attempts with exponential backoff [0, 1, 2, 5, 10, 30, 60 s] and ERR-level logs.** Private (`shared: false`) servers stay on the legacy single-retry behavior. The only way to stop shared retries is to disable the server in config and restart ai-agent. (Implemented in `src/tools/mcp-provider.ts`: restart loop now retries forever and Phase 1 scenario `run-test-73-shared-restart-backoff` proves the behavior.)
- Tool timeouts always run the health probe (`ping` → `listTools`) and only restart the shared server when the probe fails. A single slow tool call must never kill a healthy async server; we treat it as a per-request SLA miss, not a crash. MCP client `requestTimeoutMs` stays padded (>150 % of the tool timeout) so the agent’s watchdog fires first and can make this decision deliberately.
- Structured restart errors bubble through `ToolsOrchestrator`; Phase 1 scenario `run-test-72-shared-restart-error` now proves that logs/accounting contain `mcp_restart_failed` only when the probe fails and a restart actually happens.
- **New directive (2025-11-13 16:05 UTC)**: Detect shared MCP exits even when idle. Costa approved only the transport/child `onclose` → restart loop path for now; JSON-RPC error heuristics remain under discussion. Implementation must watch stdio child `close`/`exit` events (and remote transport `onclose`) and immediately schedule the shared restart loop so the next caller doesn’t pay a timeout penalty.

## Analysis (status as of 2025-11-12)
- **Shared registry / lifecycle** – `src/tools/mcp-provider.ts` owns `MCPSharedRegistry`; shared stdio servers stay resident across sessions, private servers still close on provider cleanup. Ref counts remain diagnostic-only. Need to add restart scheduler so shared entries never give up after a failure.
- **Transport scope** – (DONE) `shouldShareServer` now shares every transport and `initializeEntry` builds stdio/websocket/http/sse clients via a single factory.
- **Persistent retry requirement** – Shared initialization + restart paths now use the perpetual backoff loop; every attempt logs `ERR` on decision + failure, and deterministic coverage verifies multiple attempts. Private servers remain single-retry by design.
- **Restart / cancel path** – Tool timeouts now behave per spec: run the probe, and restart only when it fails. Healthy async servers that simply ran a slow query remain alive so other agents don’t lose in-flight work. Request-level timeouts remain padded (≥ toolTimeout * 1.5) so the orchestrator’s watchdog leads. User aborts still bypass restarts; private servers keep the legacy teardown.
- **Structured restart errors** – Timeout callbacks propagate `MCPRestartFailedError` / `MCPRestartInProgressError` through `ToolsOrchestrator`, so tool logs, accounting, and final reports surface the exact code whenever a restart actually happens.
- **Concurrent warmup gap (patched 2025-11-12 18:45 UTC)** – added a per-server `initializing` promise map inside `MCPSharedRegistry.acquire` so only the first caller actually runs `initializeEntry`; all concurrent requests await the same promise, and the entry is stored before any ref-counting. This stops the duplicate `npx fetcher-mcp` launches that showed up in `ps fax` and `journalctl`.
- **Deterministic fixtures / tests** – Fixtures cover hang → recovery (`run-test-71`), restart failure (`run-test-72-restart-failure`), shared timeout w/ probe failure (`run-test-72-shared-restart-error`), and shared timeout w/ probe success (new scenario). Both paths stay under regression coverage.
- **Harness plumbing** – `TEST_SCENARIOS` still derives from `BASE_TEST_SCENARIOS`. Need to ensure CI ignores stale `PHASE1_ONLY_SCENARIO` leftovers.
- **Docs** – `docs/AI-AGENT-GUIDE.md` and `docs/IMPLEMENTATION.md` highlight that restarts require probe failure; keep them synced if timeout heuristics or idle teardown semantics change.
- **Idle exit detection (NEW)** – No watcher currently observes stdio child exits or remote transport closes. `SharedServerHandle.callTool` only reacts to error strings containing “request timed out.” As a result, an MCP process that dies while idle is only restarted when the next tool call hangs long enough to trigger the watchdog; non-timeout errors (broken pipe, EOF) simply bubble out with no recovery. Need to hook transport-level `onclose` and schedule the restart loop with reason `transport-exit` even when no call is active.

## Decisions (Costa)
1. **Telemetry expansion** – Declined; stay with existing `[mcp]` logs for now.
2. **Config hashing / duplicate detection** – Declined; registry keys stay as declared server names, and conflicting configs are considered a user error.
3. **Structured restart signaling** – Required; we must propagate precise `mcp_restart_failed` / `mcp_restart_in_progress` errors back to callers and Phase 1 expectations.
4. **Deterministic restart fixture coverage** – Required; restart success/failure + concurrent waiter cases must live in the harness.
5. **Out-of-scope cases** – Ignore exotic scenarios (e.g., two sessions intentionally reusing a name with divergent configs); no extra handling planned.
6. **Transport pooling scope** – Required; *all* MCP transports must use the shared registry so long as `shared !== false`. No additional config hashing or validation beyond “one name = one config.” *(Done.)*
7. **Relentless restart scheduler (shared only)** – Required; every shared failure (startup, timeout probe failure, restart exception) must trigger ERR logs and exponential-backoff retries (0,1,2,5,10,30,60 s repeating). Only disabling the server in config + restarting the agent should stop the loop. Private servers stay as-is. *(Done – `run-test-73-shared-restart-backoff` exercises the new path.)*
8. **Idle-exit detection scope (NEW)** – Approved approach: only detect transport/child exits for now. Do *not* attempt JSON-RPC error heuristics until we revisit the design. Restart reason should log as `transport-exit` (or similar) and feed the same backoff loop. Remote transports (ws/http/sse) must fire the same code path when their underlying socket closes unexpectedly.

## Plan (updated)
## Plan (updated)
1. **Harness plumbing & coverage** *(next session)*
   - ✅ Reconcile `TEST_SCENARIOS` vs `BASE_TEST_SCENARIOS`: `PHASE1_ONLY_SCENARIO` is now ignored when `CI` is set, preventing filtered runs in automation.
   - ✅ Capture the fixture env knobs (`MCP_FIXTURE_BLOCK_EVENT_LOOP`, `MCP_FIXTURE_SKIP_EXIT`, `MCP_FIXTURE_HANG_MS`, `MCP_FIXTURE_EXIT_DELAY_MS`, `MCP_FIXTURE_FAIL_INIT_ATTEMPTS`, `MCP_FIXTURE_FAIL_RESTART_ATTEMPTS`) in docs (`docs/TESTING.md`).
2. **Idle exit detection implementation** *(complete)*
   - ✅ Wire shared entries to the MCP SDK's `onclose` hook so every transport (stdio, websocket, http, sse) funnels into `startRestartLoop(..., 'transport-exit')` without waiting for a tool timeout.
   - ✅ Guard planned restarts by toggling `transportClosing` so agent-initiated shutdowns don’t spawn redundant loops, while unexpected exits still log and restart immediately.
   - ✅ Extend deterministic coverage with an idle-exit fixture (`run-test-74-idle-exit-restart`) so Phase 1 guards against regressions.
3. **Doc / ops sync**
   - Keep `docs/AI-AGENT-GUIDE.md`, `docs/IMPLEMENTATION.md`, and `docs/SPECS.md` aligned whenever restart semantics or naming guidance change. (Current change updated all three to describe the relentless backoff.)
4. **Validation cadence**
   - Continue running `npm run lint`, `npm run test:phase1`, and `npm run build` after any shared-registry change. Update or add deterministic scenarios if future behavior (e.g., idle teardown, HTTP pooling) shifts.

## Implied Decisions / Follow-ups
- Pooling now must apply to all transports; any per-transport quirks (e.g., SSE subscription drift, HTTP rate limits) need to be handled in the shared path instead of bespoke per-session fixes.
- Shared entries never auto-shutdown when refCount reaches zero. Decide whether that is acceptable for long-lived daemons or if we want an idle timeout.
- No new telemetry/log schema was added beyond existing `[mcp]` logs; any production dashboards will need explicit counters if required.
- `restartPromise` guards duplicate restarts but there is no queue for callers waiting on a restart. If that’s desirable, we must design the waiting semantics (bounded await vs. fast error).
- Phase 1 deterministic fixtures now serve as the canonical behavioral contract; any runtime change to restart semantics must update both fixture instructions and doc guidance simultaneously.
- JSON-RPC error-based health detection is deferred; revisit after we gain telemetry on how often transport exits vs. upstream dependency failures occur.
- Never treat a single tool timeout as proof the MCP server is dead; rely on the probe. If probes become noisy/slow, adjust probe behavior instead of skipping it.

## Testing Requirements
- Keep `npm run lint`, `npm run build`, and full `npm run test:phase1` green; regression focus is on shared restart scenarios (`run-test-66/67/71/72`) plus context guard scenario.
- Deterministic restart fixture must exercise hang → timeout → probe failure → restart success and restart failure flows; both must assert the final report contents.
- `run-test-74-idle-exit-restart` now ensures a stdio transport exit between calls triggers the shared restart loop without manual probes; extend to remote transports later if needed.
- If restart semantics change again, clone/update the deterministic fixture instructions rather than relying on mocks.

## Documentation Updates
- `docs/AI-AGENT-GUIDE.md` and `docs/IMPLEMENTATION.md` already describe `shared` + `healthProbe`; only update them if future changes alter defaults, telemetry, or config validation.
- Docs updated to mention `transport-exit` restarts so operators know unexpected process exits now trigger recovery immediately.
- If we introduce telemetry or idle-teardown semantics, mirror the behavior in `docs/SPECS.md` and mention operational guidance (what logs/metrics to watch when a restart fails).
