# TODO – Graceful Shutdown Across Headends & MCP Registry

## TL;DR
- Add a coordinated shutdown path so `ai-agent` exits cleanly on SIGINT/SIGTERM, even with active headend clients and MCP restarts in flight.

## Analysis
- Current signal handler in `src/cli.ts` just logs and calls `process.exit()`, so sockets/headends keep running until the OS sends SIGKILL.
- Headends (`Rest`, `OpenAI`, `Anthropic`, `MCP`, `Slack`) expose `stop()` but we never await them when quitting; long-lived clients keep the event loop busy.
- `MCPProvider`’s shared registry continues to restart servers after SIGTERM; there’s no stop flag so it assumes the agent is healthy.
- Agent sessions already have `stopRef`, but headends don’t pass a shared ref that ties into the process-level stop signal.

## Decisions
- Use a global `ShutdownController` (wraps `AbortController` + `stopRef`) initialized in CLI. On SIGINT/SIGTERM it:
  1. Sets `stopRef.stopping = true` so active sessions finish current turn without starting new ones.
  2. Calls `abortController.abort()` so background awaits can bail out.
  3. Invokes `await headendManager.stopAll()` which awaits every headend’s `stop()`.
  4. Tells `MCPProvider` to `shutdown()` so restart loops exit.
  5. Sets a watchdog timer (e.g., 30 s) to force `process.exit(1)` if cleanup hangs.
- Headend manager exposes a `stopAll()` that stops listeners and resolves when all connections close (REST/SSE/WS/Slack included).
- MCP shared registry gains a stop hook; restarts check `shutdownRequested` before spawning new transports.

## Plan
1. **Signal orchestration** (`src/cli.ts`)
   - Replace the simple `registeredSignals` handler with a helper that tracks `isStopping`, prints a single log, calls the new `gracefulShutdown()` promise (headends + MCP + telemetry), and sets a watchdog.
   - Ensure second SIGINT forces immediate exit (`process.exit(1)`).
2. **Headend manager stop API** (`src/headends/headend-manager.ts` + headends)
   - Implement `stopAll()` that iterates existing headends and awaits their `stop()`; each headend should close sockets/listeners and resolve after pending connections drain (REST server `close()`, WebSocket `terminate`, etc.).
   - Pass shared `stopRef`/`abortSignal` down to spawned agent sessions.
3. **MCP shutdown awareness** (`src/tools/mcp-provider.ts`)
   - Add `shutdown()` that sets `this.stopping = true`, aborts outstanding restart timers, and closes shared transports without restarting.
   - Restart logic checks `this.stopping` before invoking `restartShared()`.
4. **Session wiring**
   - Ensure headends supply the process-level `stopRef` to `registry.spawnSession` so agents stop launching new turns once shutdown begins.

## Implied Decisions
- We’ll treat shutdown as “stop accepting new work and finish the current turn,” not “kill immediately,” matching systemd expectations.
- Watchdog keeps us from hanging forever if a misbehaving headend refuses to close.

## Testing Requirements
- Unit/manual: run headend mode (`ai-agent --api 8123 --mcp stdio ...`), connect via curl/SSE, send SIGINT; process should exit without SIGKILL.
- Automated idea: add a smoke test spawning the CLI in headend mode, issue SIGTERM via child process, assert exit code 0 and that MCP restart logs stop.

## Documentation Updates Required
- Add a short note to README (headend section) describing graceful shutdown behavior / how SIGINT/SIGTERM are handled.
