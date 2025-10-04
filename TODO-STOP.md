# TODO: Implement "Enough" Stop Mode

1. **Agent Loop Stop Mode**  
   - In `src/ai-agent.ts`, replace the early graceful-stop return with logic that clamps `maxTurns` to the current turn + 1 (capped at the original limit) and records that we are in stop mode.  
   - Ensure this happens once per session and reuse it from any path (`executeAgentLoop`, `sleepWithAbort`) that detects `stopRef.stopping`.

2. **Flush Pending Tool Queue**  
   - Add a lightweight `requestStop()` hook to `ToolsOrchestrator` that marks `stopRequested` and rejects outstanding waiters so queued tool calls surface the "stop requested" error immediately.  
   - Call this hook from the agent when stop mode is activated.

3. **Allow Final Report After Stop**  
   - Keep rejecting MCP/REST/sub-agent tools once stop is active, but permit `agent__final_report` (and only that tool) to run so the final turn can conclude.  
   - Apply the allow-list both in `toolExecutor` and the orchestrator’s `executeWithManagement` path so nothing else slips through.

4. **Rate-Limit & Sleep Path Alignment**  
   - Update the `sleepWithAbort`/rate-limit retry branch to delegate to the new stop-mode helper instead of finalizing immediately, ensuring we still reach the final turn.

5. **Slack UX Tweaks**  
   - Rename the Slack action button from "Stop" to "Enough" (handler id unchanged).  
   - While a run is in `stopping` state, keep showing the live progress snapshot but prepend a "Stopping…" section and hide action buttons.

6. **Verification**  
   - After code changes, run `npm run lint` and `npm run build` to confirm nothing else regresses.
