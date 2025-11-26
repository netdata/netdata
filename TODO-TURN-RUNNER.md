# Turn Runner Extraction

## TL;DR
Extract `executeAgentLoop` (~1500 lines) and related helper methods from `AIAgentSession` into a new `TurnRunner` class. This separates turn orchestration concerns from session lifecycle management.

## Analysis

### Current State
- `ai-agent.ts`: 4204 lines
- `executeAgentLoop`: lines 1404-2900 (~1500 lines)
- Helper methods used by executeAgentLoop: lines 2902-3760 (~850 lines)

### Methods to Extract
1. **Core loop**: `executeAgentLoop` (1404)
2. **Turn execution**: `executeSingleTurn` (3762), `executeSingleTurnInternal` (3805)
3. **Finalization**: `finalizeCanceledSession` (2902), `finalizeGracefulStopSession` (2920), `finalizeWithCurrentFinalReport` (3405)
4. **Sleep/timing**: `sleepWithAbort` (2937)
5. **Logging helpers**: `emitFinalSummary` (2988), `logTurnStart` (3284), `logEnteringFinalTurn` (3299), `logFinalReportAccepted` (3319), `logFallbackAcceptance` (3341), `logFailureReport` (3358)
6. **Context guard**: `reportContextGuardEvent` (3094), `handleContextGuardForcedFinalTurn` (3221), `evaluateContextGuard` (3199), `evaluateContextForProvider` (3207), `enforceContextFinalTurn` (3211), `buildContextMetrics` (3203)
7. **Tool selection**: `selectToolsForTurn` (3374), `consumePendingToolSelection` (3388), `filterToolsForProvider` (4092), `computeForcedFinalSchemaTokens` (4109)
8. **Final report**: `commitFinalReport` (3394), `acceptPendingFinalReport` (3398), `getFinalReportStatus` (3483), `tryAdoptFinalReportFromText` (3553)
9. **Message processing**: `sanitizeTurnMessages` (3563), `mergePendingRetryMessages` (3161), `pushSystemRetryMessage` (3087), `buildFinalReportReminder` (3141), `expandNestedLLMToolCalls` (4166)
10. **Retry logic**: `buildFallbackRetryDirective` (3511), `addTurnFailure` (4154)
11. **Utilities**: `composeRemoteIdentifier` (3491), `debugLogRawConversation` (3499), `emitReasoningChunks` (3730), `resolveReasoningMapping` (4123), `resolveReasoningValue` (4149), `resolveToolChoice` (4132), `resolveModelOverrides` (193)

### Session Fields Accessed
- **Read-only config**: `sessionConfig`, `config`, `callPath`, `agentPath`, `txnId`, `parentTxnId`, `originTxnId`, `headendId`, `telemetryLabels`
- **Collaborators**: `llmClient`, `toolsOrchestrator`, `contextGuard`, `finalReportManager`, `opTree`, `progressReporter`, `sessionExecutor`, `xmlTransport`, `subAgents`
- **Mutable state**: `_currentTurn`, `canceled`, `turnFailureReasons`, `masterLlmStartLogged`, `pendingToolSelection`, `pendingRetryMessages`, `toolFailureMessages`, `toolFailureFallbacks`, `trimmedToolCallIds`, `plannedSubturns`, `finalTurnEntryLogged`, `currentLlmOpId`, `systemTurnBegan`, `centralSizeCapHits`, `llmAttempts`, `llmSyntheticFailures`
- **Callbacks**: `log()`, various session callbacks

## Decisions

### Interface Design
1. **TurnRunnerContext**: Immutable configuration and collaborators passed at construction
2. **TurnRunnerCallbacks**: Side-effect handlers (logging, telemetry, opTree updates)
3. **TurnRunnerState**: Mutable state managed internally by TurnRunner

### What Stays in AIAgentSession
- Session lifecycle (`run()`, `create()`, constructor)
- Session-level state (conversation history, logs, accounting arrays)
- Collaborator instantiation (LLMClient, ToolsOrchestrator, etc.)
- Result composition and cleanup

### What Moves to TurnRunner
- Turn iteration logic
- Retry/backoff logic
- Context guard evaluation per turn
- LLM call orchestration
- Message sanitization
- Final report extraction/adoption

## Plan

1. ✅ Create `src/session-turn-runner.ts`:
   - Define `TurnRunnerContext` interface
   - Define `TurnRunnerCallbacks` interface
   - Define `TurnRunner` class with `execute()` method
   - Move all helper methods

2. ⏳ Update `src/ai-agent.ts`:
   - Remove extracted methods
   - Create `TurnRunnerContext` in `run()`
   - Instantiate `TurnRunner` and delegate

3. Update imports in `ai-agent.ts`

4. ✅ Run build and lint

## Current Progress

### Phase 1 - COMPLETED ✅
- Created `session-turn-runner.ts` with ~2100 lines
- TurnRunner class with `execute()` method
- All helper methods extracted
- **Fixed all critical gaps identified by Gemini/Codex reviews:**
  - ✅ Rate limit cycle handling (`rateLimitedInCycle`, `maxRateLimitWaitMs`, `sleepWithAbort`)
  - ✅ `finalReportToolFailed` tracking with proper error differentiation
  - ✅ `turnHadFinalReportAttempt` + `collapseRemainingTurns()` logic
  - ✅ Complete success path (reasoning-only, empty response, `getFinalReportStatus()`, final/non-final turns)
  - ✅ `childConversations` parameter in `execute()`
  - ✅ Helper methods: `getFinalReportStatus()`, `buildFinalReportReminder()`
  - ✅ `plannedSubturns` instrumentation for log enrichment
- Build passes
- Lint passes with zero warnings

### Phase 2 - INTEGRATION COMPLETE ✅
- ✅ Updated `AIAgentSession.run()` to instantiate and use TurnRunner
- ✅ Build passes
- ✅ Lint passes (zero warnings)
- ⏳ Dead code removal (optional cleanup)
- ⏳ Manual smoke test

### Dead Code to Remove (optional, ~2400 lines)
After testing, the following methods can be removed from `ai-agent.ts`:
- `executeAgentLoop` (line 1457)
- `finalizeCanceledSession`, `finalizeGracefulStopSession`, `finalizeWithCurrentFinalReport`
- `sleepWithAbort`
- `emitFinalSummary`, `logTurnStart`, `logEnteringFinalTurn`, `logFinalReportAccepted`, `logFallbackAcceptance`, `logFailureReport`
- `executeSingleTurn`, `executeSingleTurnInternal`
- All helper methods moved to TurnRunner

## Review Validation (Gemini + Codex)

Reviews by Gemini and Codex identified critical gaps. **Validation status:**

### ✅ CONFIRMED: Rate Limit Handling (CRITICAL - Safety Issue)
- **Original**: `rateLimitedInCycle`, `maxRateLimitWaitMs`, `cycleComplete` track rate limits; `sleepWithAbort` called when all providers rate-limited (ai-agent.ts:2594-2620)
- **TurnRunner**: Has `sleepWithAbort` but removed cycle tracking
- **Impact**: Agent will spin through retries immediately without backoff when all providers rate-limit
- **Fix**: Reinstate `rateLimitedInCycle`, `maxRateLimitWaitMs`, `cycleIndex`, `cycleComplete`; add logic to check `allRateLimited` and call `sleepWithAbort`

### ✅ CONFIRMED: `finalReportToolFailed` (IMPORTANT)
- **Original**: Used at line 2753 to handle case where final_report tool itself fails
- **TurnRunner**: Removed
- **Impact**: Incorrect failure reason; may exit with "Max turns exceeded" instead of proper failure
- **Fix**: Add `finalReportToolFailed` to state; set it when detecting failed final_report tool

### ✅ CONFIRMED: `turnHadFinalReportAttempt` (IMPORTANT)
- **Original**: Used at lines 2350, 2425 for `finalReportAttemptFlag`; triggers turn collapsing
- **TurnRunner**: Removed
- **Impact**: Turn collapsing won't trigger after failed final report attempts
- **Fix**: Reinstate variable and logic in success path handling

### ⚠️ CONFIRMED: `childConversations` (IMPORTANT)
- **Original**: Populated via AgentProvider callback in AIAgentSession (line 712)
- **TurnRunner**: Creates fresh empty array; population happens in AIAgentSession
- **Impact**: Child conversation data lost if not passed properly
- **Fix**: Pass `childConversations` array reference from AIAgentSession to `execute()`

### ✅ CONFIRMED: Success Path Logic Missing (CRITICAL)
- **TurnRunner line 1019**: "For brevity, marking turn as successful..."
- **Missing from original (lines 2361-2491)**:
  - `hasReasoning` check for reasoning-only responses (continue without penalty)
  - Empty response retry handling with specific messaging
  - `getFinalReportStatus()` check for success/failure/partial/missing
  - Different handling for final vs non-final turns
  - `retryFlags` / `finalReportAttemptFlag` computation
  - Turn collapsing based on `incompleteFinalReportDetected`
- **Impact**: Multiple edge cases not handled; wrong retry behavior
- **Fix**: Port complete success path logic from ai-agent.ts:2349-2491

### ❌ FALSE: `incompleteFinalReportDetected` missing
- **TurnRunner HAS this** at lines 1913-1924, 2146
- **Issue**: The USAGE (collapse logic) is missing (see turnHadFinalReportAttempt above)

### Partially Confirmed: State variables
- `plannedSubturns`, `trimmedToolCallIds`, `centralSizeCapHits` - exist in TurnRunner but usages need verification

## Updated Plan

### Phase 1: Fix Critical Gaps (BEFORE integration)
1. **Rate limit cycle handling**
   - Reinstate `rateLimitedInCycle`, `maxRateLimitWaitMs`, `cycleIndex`, `cycleComplete`
   - Add `allRateLimited` check after rate_limit status handling
   - Call `sleepWithAbort` when all providers rate-limited

2. **Final report failure tracking**
   - Add `finalReportToolFailed` to TurnRunnerState
   - Set flag when detecting `(tool failed:` for FINAL_REPORT_TOOL
   - Use flag in finalization logic

3. **Turn collapsing logic**
   - Reinstate `turnHadFinalReportAttempt`
   - Add `retryFlags` / `finalReportAttemptFlag` computation
   - Trigger `collapseRemainingTurns` based on flags

4. **Complete success path**
   - Add `hasReasoning` check for reasoning-only responses
   - Add `getFinalReportStatus()` call and status-based handling
   - Add empty response retry with messaging
   - Add final vs non-final turn differentiation

5. **childConversations**
   - Change `execute()` to accept `childConversations` as parameter
   - Use passed reference instead of creating new array

### Phase 2: Integration
1. Update AIAgentSession to use TurnRunner
2. Run build and lint
3. Test with harness
4. Manual smoke test

### Remaining Work (after Phase 1)
1. Update `AIAgentSession.run()` to use TurnRunner
2. Verify existing test harness passes
3. Manual smoke test with `./run.sh`

## Implied Decisions

- Token counter accessors (`currentCtxTokens`, etc.) delegate to `contextGuard` - keep delegation pattern
- Static constants (`FINAL_REPORT_TOOL`, etc.) - duplicate in TurnRunner or pass via context
- `childConversations` array - passed in result, managed by TurnRunner

## Testing Requirements

- Existing test harness should pass unchanged
- Manual smoke test with `./run.sh`
- Build with zero warnings/errors

## Documentation Updates

- Update `docs/DESIGN.md` to mention TurnRunner extraction
- Update `docs/IMPLEMENTATION.md` if it references executeAgentLoop
