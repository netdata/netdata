# TODO: Fix Missing Final Turn When User Presses "Stop"

## TL;DR

When users press "Stop" on Slack, the session exits immediately without giving the model a final turn. We need to extend `stopRef` with a `reason` field and add a final turn for graceful stops.

**Key insight from code review:** AbortSignal-first approach won't work because once aborted, the signal stays aborted permanently and would block the final turn LLM call. We must keep the dual mechanism (stopRef + abortSignal) with extended stopRef interface.

---

## CURRENT STATE ANALYSIS

### Three Separate Mechanisms Today

| Mechanism | Type | Trigger | Behavior |
|-----------|------|---------|----------|
| `stopRef.stopping` | Polling-based | `SessionManager.stopRun()`, `ShutdownController` | Graceful exit, allows in-flight ops |
| `abortSignal` / `isCanceled()` | Event-based | `SessionManager.cancelRun()`, `ShutdownController` | Immediate cancel, stops everything |
| `ShutdownController` | Both | Process signals (SIGTERM, etc.) | Triggers BOTH mechanisms |

### Why AbortSignal-first Won't Work

| Issue | Evidence |
|-------|----------|
| LLM requests receive `abortSignal` and abort immediately when fired | `src/session-turn-runner.ts:3525`, `src/llm-providers/base.ts:1325-1331` |
| AbortSignal cannot be "un-aborted" - once aborted, stays aborted forever | Node.js API limitation |
| Queues/tools also abort immediately on signal | `src/tools/queue-manager.ts:86-118`, `src/headends/concurrency.ts:32-75` |
| Final turn would be blocked because it needs a non-aborted signal | Fundamental conflict |

### Current Check Locations

**stopRef.stopping checks (13 locations):**
| File | Line | Context |
|------|------|---------|
| `src/session-turn-runner.ts` | 312 | Turn loop start |
| `src/session-turn-runner.ts` | 409 | Retry loop |
| `src/session-turn-runner.ts` | 1685 | Rate limit check |
| `src/session-turn-runner.ts` | 2779, 2811 | Sleep polling |
| `src/session-tool-executor.ts` | 189 | Before tool execution |
| `src/ai-agent.ts` | 1777 | Sleep check |
| `src/ai-agent.ts` | 1814 | Sleep polling (100ms interval) |
| `src/headends/embed-headend.ts` | 376 | Before request processing |

**isCanceled() checks (4 locations):**
| File | Line | Context |
|------|------|---------|
| `src/session-turn-runner.ts` | 310 | Turn loop start |
| `src/session-turn-runner.ts` | 2777, 2808 | Sleep checks |
| `src/session-tool-executor.ts` | 192 | Before tool execution |

### Current Finalization Paths

```
stopRef.stopping = true  →  finalizeGracefulStopSession()  →  success: true (NO FINAL TURN - BUG)
isCanceled() = true      →  finalizeCanceledSession()      →  success: false, error: 'canceled'
```

---

## PROPOSED UNIFIED MODEL (stopRef with reason)

### New Interface

```typescript
type StopReason = 'stop' | 'abort' | 'shutdown';

interface StopRef {
  stopping: boolean;
  reason?: StopReason;
}
```

### Behavior Matrix

| Reason | Triggered By | Final Turn? | Exit Status | Triggers AbortSignal? |
|--------|--------------|-------------|-------------|----------------------|
| `stop` | User "Stop" button | YES | success: true | NO |
| `abort` | User "Abort" button | NO | success: false | YES |
| `shutdown` | Process signal | NO | success: false | YES |

### Key Design Decision

**Keep dual mechanism:**
- `stopRef` with `reason` field for all stop types
- `abortSignal` triggered ONLY for `abort` and `shutdown` (not for `stop`)
- This allows final turn to execute because abortSignal is not triggered for graceful stop

### Migration Path

| Old Mechanism | New Equivalent |
|---------------|----------------|
| `stopRef.stopping = true` (user stop) | `stopRef.stopping = true; stopRef.reason = 'stop'` |
| `isCanceled()` / `cancelRun()` | `stopRef.stopping = true; stopRef.reason = 'abort'` + abort signal |
| `ShutdownController.shutdown()` | `stopRef.stopping = true; stopRef.reason = 'shutdown'` + abort signal |

---

## FILES REQUIRING CHANGES

### Type Definitions (11 files)

| File | Line | Change |
|------|------|--------|
| `src/types.ts` | ~675 | Add `StopReason` type, update `stopRef` interface |
| `src/types.ts` | ~773 | Update SessionConfig stopRef type |
| `src/llm-messages-xml-next.ts` | 87 | Add `'user_stop'` to `FinalTurnReason` |
| `src/session-turn-runner.ts` | 90, 151, 158-166 | Update stopRef type, add `'user_stop'` to unions |
| `src/ai-agent.ts` | 132, 1990 | Update stopRef type, add `'user_stop'` to getter |
| `src/context-guard.ts` | 80, 546 | Add `'user_stop'` to unions |
| `src/xml-tools.ts` | 30 | Add `'user_stop'` to union |
| `src/xml-transport.ts` | 66 | Add `'user_stop'` to union |
| `src/telemetry/index.ts` | ~122 | Add `'user_stop'` to metrics |
| `src/headends/types.ts` | 28 | Update stopRef type |
| `src/headends/headend-manager.ts` | 17, 27, 35 | Update stopRef type |

### Stop Signal Setters (6 files)

| File | Line | Current | New |
|------|------|---------|-----|
| `src/server/session-manager.ts` | 92 | `ref.stopping = true` | `ref.stopping = true; ref.reason = 'stop'` (NO abort signal) |
| `src/server/session-manager.ts` | 78 | `aborters.get(runId)?.abort()` | `ref.stopping = true; ref.reason = 'abort'` + abort signal |
| `src/shutdown-controller.ts` | 43 | `stopRef.stopping = true` + abort | `stopRef.stopping = true; stopRef.reason = 'shutdown'` + abort |
| `src/headends/shutdown-utils.ts` | 7 | `stopRef.stopping = true` | Update to new interface with reason |
| Headends (5 files) | Various | Local stopRef creation | Update to new interface |

### Stop Signal Checkers (8 files)

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/session-turn-runner.ts` | 310-313, 409-410, 1703-1704, 2779, 2811 | Check `reason` to decide: final turn vs immediate exit |
| `src/session-tool-executor.ts` | 189, 192 | Block tools only for `abort`/`shutdown`, allow `final_report` for `stop` |
| `src/ai-agent.ts` | 535-543, 1777, 1814 | Update to new interface, check reason |
| `src/headends/embed-headend.ts` | 376, 415 | Update to new interface |
| `src/headends/rest-headend.ts` | 248, 251, 258, 385, 388, 395 | Update to new interface |
| `src/headends/openai-completions-headend.ts` | 261, 264 | Update to new interface |
| `src/headends/anthropic-completions-headend.ts` | 262, 265 | Update to new interface |
| `src/headends/mcp-headend.ts` | 365, 366 | Update to new interface |

### Orchestration / Child Session Propagation (5 files) - ADDED BY REVIEWERS

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/orchestration/spawn-child.ts` | 36-47, 120-173, 188-208 | Propagate stopRef reason to child sessions |
| `src/orchestration/handoff.ts` | 42-44 | Update stopRef interface |
| `src/orchestration/advisors.ts` | 49-51 | Update stopRef interface |
| `src/subagent-registry.ts` | 165-171, 308-309 | Propagate stopRef reason |
| `src/agent-registry.ts` | 35-36, 124-125 | Update stopRef interface |

### Provider/Tool Layers (4 files) - ADDED BY REVIEWERS

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/llm-providers/base.ts` | 1325-1331 | AbortSignal handling unchanged (only triggers for abort/shutdown) |
| `src/tools/queue-manager.ts` | 86-118 | AbortSignal handling unchanged (only triggers for abort/shutdown) |
| `src/tools/rest-provider.ts` | 237-253 | AbortSignal handling unchanged (only triggers for abort/shutdown) |
| `src/headends/concurrency.ts` | 32-75 | AbortSignal handling unchanged (only triggers for abort/shutdown) |

### Additional Files (3 files) - ADDED BY REVIEWERS

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/agent-loader.ts` | 798-800 | Forward stopRef with reason |
| `src/cli.ts` | 1528-1529 | Pass stopRef from shutdownController |
| `src/headends/slack-headend.ts` | 160-169, 172-174 | Store and propagate stopRef with reason |

### New Code Required

| File | What to Add |
|------|-------------|
| `src/llm-messages-xml-next.ts` | `final_turn_user_stop` slug with clear instructions |
| `src/context-guard.ts` | `setForcedFinalReason()` method (already exists, just add 'user_stop' to union) |
| `src/session-turn-runner.ts` | Logic to trigger final turn for `reason === 'stop'` |

---

## RISK ANALYSIS

### HIGH RISK

| Risk | Description | Mitigation |
|------|-------------|------------|
| **Final turn needs non-aborted signal** | For `stop`, we must NOT trigger abortSignal, only set stopRef | Clear separation: `stop` = stopRef only, `abort`/`shutdown` = stopRef + abortSignal |
| **Headend local stopRefs** | Each headend creates local stopRef. Need to ensure reason propagates correctly. | Trace all local stopRef creations, ensure they inherit reason from parent |
| **ShutdownController dual trigger** | Currently triggers BOTH mechanisms. Must continue doing so for `shutdown`. | Keep dual trigger for shutdown, single trigger for stop |

### MEDIUM RISK

| Risk | Description | Mitigation |
|------|-------------|------------|
| **Test breakage** | 7+ test suites check current behavior. All will need updates. | Update tests in same PR |
| **Race conditions** | Multiple stopRef instances (global + per-headend + per-request). Reason must be consistent. | Add validation, log warnings on inconsistency |
| **In-flight LLM calls** | Final turn for `stop` adds one more LLM call. If model is slow, user waits longer. | Accept waiting (user decision) |
| **Child session propagation** | Reason must propagate to all child sessions | Update all orchestration files |

### LOW RISK

| Risk | Description | Mitigation |
|------|-------------|------------|
| **Dead code** | `finalizeCanceledSession()` may become dead code | Consolidate into single method |
| **Documentation drift** | Docs reference old mechanisms | Update docs in same PR |

---

## TEST IMPACT

### Existing Tests That Need Updates

| Test File | Lines | Current Behavior | New Behavior |
|-----------|-------|------------------|--------------|
| `src/tests/unit/shutdown-utils.spec.ts` | All | Tests `stopping` field only | Test `stopping` + `reason` fields |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 1363 | Checks `stopRef.stopping` | Add `reason` check |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 3064-3066 | Abort signal tests | Verify abort triggers for `abort`/`shutdown` only |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 3141-3150 | Expects immediate exit on stopRef | Update: `stop` = final turn, `abort` = immediate |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 6150-6312 | Run-test-73: Stop reference | Update assertions for new behavior |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 9123-9150 | Stop reference polling | Update for new interface |
| `src/tests/smoke-rest-headend.ts` | 42, 61 | Creates stopRef, sets stopping | Update to new interface |

### New Tests Required

| Test | Description |
|------|-------------|
| `stop-reason-stop` | User "Stop" → final turn → graceful exit with summary |
| `stop-reason-abort` | User "Abort" → immediate cancel → no final turn |
| `stop-reason-shutdown` | Global shutdown → immediate cancel all → no final turn |
| `stop-during-tool-execution` | Stop while tool running → tool completes → final turn |
| `stop-during-llm-call` | Stop while LLM streaming → stream completes → final turn |
| `abort-during-tool-execution` | Abort while tool running → immediate stop (abortSignal fired) |
| `abort-during-llm-call` | Abort while LLM streaming → immediate stop (abortSignal fired) |
| `stop-reason-propagation` | Reason propagates from SessionManager to TurnRunner |
| `stop-child-session-propagation` | Reason propagates to child/sub-agent sessions |
| `headend-local-stopref` | Local headend stopRef inherits reason from global |

---

## IMPLEMENTATION PLAN (Revised)

### Phase 1: Type System (8 steps)

1. Add `StopReason` type to `src/types.ts`
2. Update `stopRef` interface in `src/types.ts` to include `reason?: StopReason`
3. Add `'user_stop'` to all `FinalTurnReason` / `ForcedFinalReason` unions (8 files)
4. Add `final_turn_user_stop` slug to `src/llm-messages-xml-next.ts`
5. Add `FINAL_TURN_SLUG_MAP` entry for `user_stop`
6. Add `'user_stop'` to `ForcedFinalReason` union in `src/context-guard.ts` (method `setForcedFinalReason()` already exists)
7. Update `FinalTurnLogReason` in `src/session-turn-runner.ts`
8. Build and fix any type errors

### Phase 2: Stop Signal Setters (5 steps)

1. Update `SessionManager.stopRun()` to set `reason: 'stop'` (NO abort signal)
2. Update `SessionManager.cancelRun()` to set `reason: 'abort'` AND trigger abort signal
3. Update `ShutdownController` to set `reason: 'shutdown'` AND trigger abort signal
4. Update all headend stopRef setters
5. Update orchestration files (`spawn-child.ts`, `handoff.ts`, `advisors.ts`)

### Phase 3: Stop Signal Checkers (4 steps)

1. Update `session-turn-runner.ts` to check `reason` and route to final turn or immediate exit
2. Update `session-tool-executor.ts`: block tools for `abort`/`shutdown`, allow `final_report` for `stop`
3. Update all headend stopRef checks
4. Update registry files (`agent-registry.ts`, `subagent-registry.ts`)

### Phase 4: Final Turn Logic (3 steps)

1. Implement final turn trigger when `reason === 'stop'`
2. Add exit code `EXIT-USER-STOP`
3. Add logging for `user_stop` reason

### Phase 5: Cleanup (2 steps)

1. Consolidate `finalizeCanceledSession()` and `finalizeGracefulStopSession()`
2. Remove any dead code paths

### Phase 6: Tests (2 steps)

1. Update all existing tests for new interface
2. Add new test scenarios (10 tests)

### Phase 7: Documentation (9 files)

1. Update `docs/specs/session-lifecycle.md`
2. Update `docs/Technical-Specs-Session-Lifecycle.md`
3. Update `docs/Headends-Library.md` (lines 346-349)
4. Update `docs/Advanced-Internal-API.md` (lines 213-214)
5. Update `docs/specs/architecture.md` (lines 174-210)
6. Update `docs/specs/headends-overview.md` (lines 56-84)
7. Update `docs/specs/headend-manager.md` (lines 18-266)
8. Update `docs/specs/library-api.md` (lines 194-195)
9. Update `docs/Technical-Specs-Architecture.md` (lines 418-419)

---

## DECISIONS MADE

1. **Keep dual mechanism (stopRef + abortSignal):**
   - stopRef extended with `reason` field
   - abortSignal triggered ONLY for `abort` and `shutdown` (not for `stop`)
   - This allows final turn to execute because abortSignal is not triggered for graceful stop

2. **Three stop reasons:**
   - `stop` → final turn, graceful exit (success=true), NO abortSignal
   - `abort` → immediate cancel (success=false), YES abortSignal
   - `shutdown` → immediate cancel all (success=false), YES abortSignal

3. **No timeout for final turn:**
   - "Stop" means "preserve your work, wrap up gracefully"
   - User accepts waiting for model to finalize
   - This differentiates "stop" from "abort"

4. **Keep 100ms polling interval:**
   - Imperceptible to humans
   - Already battle-tested
   - Low CPU overhead

---

## ESTIMATED EFFORT

| Phase | Files | Effort |
|-------|-------|--------|
| Phase 1: Types | 11 | Medium |
| Phase 2: Setters | 10 | Medium |
| Phase 3: Checkers | 12 | High |
| Phase 4: Final Turn | 3 | Medium |
| Phase 5: Cleanup | 2 | Low |
| Phase 6: Tests | 7 | High |
| Phase 7: Docs | 9 | Medium |
| **Total** | **~40 files** | **High** |

---

## REVIEWER FEEDBACK INCORPORATED

### From GLM-4.7:
- ✅ Added missing orchestration files (`spawn-child.ts`, `handoff.ts`, `advisors.ts`)
- ✅ Added type safety note for getStopReason helper
- ✅ Added sub-agent signal propagation section
- ✅ Added missing documentation files

### From Codex:
- ✅ Fixed architecture contradiction (AbortSignal-first → dual mechanism)
- ✅ Added missing files (`agent-registry.ts`, `subagent-registry.ts`, `agent-loader.ts`, `cli.ts`, `slack-headend.ts`)
- ✅ Added provider/tool layer files (`llm-providers/base.ts`, `queue-manager.ts`, `rest-provider.ts`, `concurrency.ts`)
- ✅ Expanded documentation list (9 files instead of 2)
- ✅ Clarified that AbortSignal is NOT triggered for `stop` reason

---

## REVIEW NOTES (2026-01-17)

### Missing files / updates

- `src/types.ts` (~612): `ForcedFinalReason` union still lacks `user_stop`.
- `src/ai-agent.ts` (~2070): `logEnteringFinalTurn` reason union does not include `user_stop` (if stop logging is added here).
- Docs referencing `stopRef` not in plan:  
  - `docs/specs/IMPLEMENTATION.md` (~306)  
  - `docs/specs/retry-strategy.md` (~182)  
  - `docs/specs/headend-rest.md` (~51, ~153, ~210)  
  - `docs/specs/headend-slack.md` (~376)
- Tests missing from plan: `src/tests/phase2-harness-scenarios/phase2-runner.ts` run-test-44 (~9695+) expects **no** final report on stop.

### Gaps / risks

- `stopRef.reason` can be **undefined** in existing local stopRefs (e.g., `src/headends/mcp-headend.ts` ~362-366, `src/headends/rest-headend.ts` ~248-259). New logic must define a default behavior when `stopping=true` and `reason` is missing.
- Final-turn timing for `stop` is not specified (convert **current** turn into final vs add **extra** final turn).
- `mcp-headend` creates per-request `stopRef` from `abortSignal` only; `globalStopRef` is not propagated, so user `stop` may not reach MCP tool sessions unless wired.

### Decisions Made (2026-01-17)

1) **Default for missing `stopRef.reason`**: **IGNORE**
   - If `reason` is undefined, maintain backward compatibility (old behavior)
   - Only the new explicit `reason: 'stop'` triggers final turn behavior
   - This is safest: old code paths continue working unchanged

2) **Stop timing**: **B) Finish current turn, then make NEXT turn final**
   - Current turn cannot be changed mid-flight (tools running, LLM responding)
   - When `reason === 'stop'`, set a flag to make the NEXT turn final
   - If LLM already finalized in current turn, don't add extra turn

---

## CONCRETE IMPLEMENTATIONS (Critical Issues)

### Implementation #1: Final Turn Trigger (session-turn-runner.ts:310-314)

**Current code (BUG):**
```typescript
if (this.ctx.isCanceled())
    return this.finalizeCanceledSession(conversation, logs, accounting);
if (this.ctx.stopRef?.stopping === true) {
    return this.finalizeGracefulStopSession(conversation, logs, accounting);  // ← IMMEDIATE RETURN
}
```

**Fixed code:**
```typescript
if (this.ctx.isCanceled())
    return this.finalizeCanceledSession(conversation, logs, accounting);
if (this.ctx.stopRef?.stopping === true) {
    const reason = this.ctx.stopRef.reason;
    if (reason === 'stop') {
        // Graceful stop: trigger final turn, don't exit yet
        if (this.ctx.contextGuard.getForcedFinalReason() === undefined) {
            this.ctx.contextGuard.setForcedFinalReason('user_stop');
        }
        // Continue to the turn loop - next turn will be final
    } else if (reason === 'abort' || reason === 'shutdown') {
        // abort or shutdown - immediate exit with failure
        return this.finalizeCanceledSession(conversation, logs, accounting);
    } else {
        // undefined (legacy) - immediate exit with success (backward compat)
        return this.finalizeGracefulStopSession(conversation, logs, accounting);
    }
}
```

**Key behavior:**
- `reason === 'stop'` → Set forced final reason, continue to turn loop
- `reason === 'abort'` or `'shutdown'` → Immediate exit via `finalizeCanceledSession` (success=false)
- `reason === undefined` → Immediate exit via `finalizeGracefulStopSession` (backward compat)

---

### Implementation #1b: Retry Loop Stop Check (session-turn-runner.ts:409-410)

**Current code:**
```typescript
if (Boolean(this.ctx.stopRef?.stopping)) {
    return this.finalizeGracefulStopSession(conversation, logs, accounting);
}
```

**Fixed code:**
```typescript
if (Boolean(this.ctx.stopRef?.stopping)) {
    const reason = this.ctx.stopRef?.reason;
    if (reason === 'stop') {
        // Graceful stop: set final turn flag and break retry loop
        if (this.ctx.contextGuard.getForcedFinalReason() === undefined) {
            this.ctx.contextGuard.setForcedFinalReason('user_stop');
        }
        break; // Exit retry loop, continue to finalization with final turn
    } else if (reason === 'abort' || reason === 'shutdown') {
        return this.finalizeCanceledSession(conversation, logs, accounting);
    } else {
        return this.finalizeGracefulStopSession(conversation, logs, accounting);
    }
}
```

---

### Implementation #1c: Rate Limit Sleep Path (session-turn-runner.ts:1699-1706)

**Current code:**
```typescript
const sleepResult = await this.sleepWithAbort(maxRateLimitWaitMs);
if (sleepResult === 'aborted_cancel')
    return this.finalizeCanceledSession(conversation, logs, accounting);
if (sleepResult === 'aborted_stop')
    return this.finalizeGracefulStopSession(conversation, logs, accounting);
```

**Fixed code:**
```typescript
const sleepResult = await this.sleepWithAbort(maxRateLimitWaitMs);
if (sleepResult === 'aborted_cancel')
    return this.finalizeCanceledSession(conversation, logs, accounting);
if (sleepResult === 'aborted_stop') {
    const reason = this.ctx.stopRef?.reason;
    if (reason === 'stop') {
        // Graceful stop: set final turn and continue
        if (this.ctx.contextGuard.getForcedFinalReason() === undefined) {
            this.ctx.contextGuard.setForcedFinalReason('user_stop');
        }
        break; // Exit rate-limit loop, continue to final turn
    } else if (reason === 'abort' || reason === 'shutdown') {
        return this.finalizeCanceledSession(conversation, logs, accounting);
    } else {
        return this.finalizeGracefulStopSession(conversation, logs, accounting);
    }
}
```

---

### Implementation #1d: sleepWithAbort Return Handling (session-turn-runner.ts:2774-2811)

**No change to sleepWithAbort itself** - it correctly returns `'aborted_stop'` when stopRef triggers.
The CALLERS of sleepWithAbort must handle the return value with reason-awareness (see #1c above).

**All sleepWithAbort call sites need the same pattern:**
- `session-turn-runner.ts:1699` - rate limit sleep (see #1c)
- `session-turn-runner.ts:1727` - backoff sleep (same pattern as #1c)

---

### Implementation #2: Tool Blocking Exception (session-tool-executor.ts)

**IMPORTANT:** The stopRef check must be MOVED to AFTER `effectiveToolName` is computed (line ~205+).

**Current code location (line 189) - WRONG POSITION:**
```typescript
if (this.sessionContext.stopRef?.stopping === true) {
    throw new Error('stop_requested');  // ← BLOCKS ALL TOOLS, before effectiveToolName exists
}
```

**Fixed code - MOVE to after effectiveToolName is computed (~line 210+):**
```typescript
// After effectiveToolName is computed:
if (this.sessionContext.stopRef?.stopping === true) {
    const reason = this.sessionContext.stopRef.reason;
    if (reason === 'stop') {
        // For 'stop' reason: allow only final_report tool
        const isFinalReport = effectiveToolName === 'agent__final_report'
            || effectiveToolName === 'final_report';
        if (!isFinalReport) {
            throw new Error('stop_requested');
        }
        // Allow final_report to proceed
    } else {
        // abort, shutdown, or undefined (legacy) - block all tools
        throw new Error('stop_requested');
    }
}
```

**Key changes:**
1. MOVE the check to AFTER `effectiveToolName` is defined
2. Check reason to allow `final_report` for graceful stop

---

### Implementation #3: Local StopRef Reason Inheritance (all headends)

**Pattern for all headends:**

```typescript
// BEFORE (all headends):
const stopRef = { stopping: this.globalStopRef?.stopping === true };

// AFTER (all headends):
const stopRef = {
    stopping: this.globalStopRef?.stopping === true,
    reason: this.globalStopRef?.reason
};
```

**Files requiring this pattern:**
- `src/headends/rest-headend.ts:248` and `:385`
- `src/headends/embed-headend.ts:415`
- `src/headends/openai-completions-headend.ts:261`
- `src/headends/anthropic-completions-headend.ts:262`
- `src/headends/mcp-headend.ts:365` (see #3b for special handling)

---

### Implementation #3b: MCP Headend Special Case (mcp-headend.ts:362-366)

**Current code (creates stopRef from abortSignal only):**
```typescript
const abortController = new AbortController();
const stopRef = { stopping: false };
extra.signal.addEventListener('abort', () => {
    stopRef.stopping = true;
    abortController.abort();
}, { once: true });
```

**Fixed code (poll globalStopRef instead of snapshot):**
```typescript
const abortController = new AbortController();
// Create a getter-based stopRef that ALWAYS reads from globalStopRef
const stopRef = {
    get stopping(): boolean {
        return this.globalStopRef?.stopping === true;
    },
    get reason(): StopReason | undefined {
        return this.globalStopRef?.reason;
    }
};
// Watch for abort signal (MCP client disconnect)
extra.signal.addEventListener('abort', () => {
    // When abortSignal fires, we need to set stopping=true on globalStopRef
    // if it's not already set (handles MCP client disconnect case)
    if (this.globalStopRef && !this.globalStopRef.stopping) {
        this.globalStopRef.stopping = true;
        this.globalStopRef.reason = 'abort';
    }
    abortController.abort();
}, { once: true });
```

**IMPORTANT:** The getter-based approach ensures stopRef ALWAYS reflects the current
globalStopRef state. A snapshot (copying values) would NOT see later updates.
If getters are not feasible, pass `this.globalStopRef` directly instead of creating local stopRef.
```

---

### Implementation #4: Context Guard Method (context-guard.ts)

**Add new method:**
```typescript
/**
 * Force final turn due to external trigger (user stop, etc).
 * Only sets if not already forced.
 */
setForcedFinalReason(reason: ForcedFinalReason): void {
    if (this.forcedFinalReason === undefined) {
        this.forcedFinalReason = reason;
    }
}
```

**Update ForcedFinalReason union (line ~80):**
```typescript
type ForcedFinalReason = 'context' | 'max_turns' | 'task_status_completed'
    | 'task_status_only' | 'retry_exhaustion' | 'user_stop';
```

---

### Implementation #5: EXIT-USER-STOP Emission (session-turn-runner.ts:2552-2560)

**Current finalization code maps forcedFinalReason to exit code:**
```typescript
const exitCode = forcedFinalReason === 'context' ? 'EXIT-TOKEN-LIMIT'
    : forcedFinalReason === 'retry_exhaustion' ? 'EXIT-MAX-RETRIES'
    : 'EXIT-FINAL-ANSWER';
```

**Fixed code (add user_stop mapping):**
```typescript
const exitCode = forcedFinalReason === 'context' ? 'EXIT-TOKEN-LIMIT'
    : forcedFinalReason === 'retry_exhaustion' ? 'EXIT-MAX-RETRIES'
    : forcedFinalReason === 'user_stop' ? 'EXIT-USER-STOP'
    : 'EXIT-FINAL-ANSWER';
```

---

## UPDATED FILE LIST (Consolidated)

### Type Definition Updates

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/types.ts` | ~612, ~675, ~773 | Add `StopReason`, update `ForcedFinalReason`, update stopRef interface |
| `src/headends/types.ts` | 28 | Update stopRef type |
| `src/headends/shutdown-utils.ts` | 1-3 | Update `StopRef` interface with `reason` field |
| `src/shutdown-controller.ts` | 16 | Update stopRef getter return type |
| `src/server/session-manager.ts` | 23, 26 | Update stopRefs map type |

### Stop Signal Setters

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/server/session-manager.ts` | 78, 92 | Set `reason: 'abort'` / `reason: 'stop'` |
| `src/shutdown-controller.ts` | 43 | Set `reason: 'shutdown'` |

### Stop Signal Checkers (ALL stop paths)

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/session-turn-runner.ts` | 310-314 | Implementation #1 |
| `src/session-turn-runner.ts` | 409-410 | Implementation #1b |
| `src/session-turn-runner.ts` | 1699-1706 | Implementation #1c |
| `src/session-turn-runner.ts` | 1727 | Same pattern as #1c |
| `src/session-turn-runner.ts` | 2552-2560 | Implementation #5 (EXIT-USER-STOP) |
| `src/session-tool-executor.ts` | 189→210+ | Implementation #2 (MOVE check) |

### Headends (local stopRef creation)

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/headends/rest-headend.ts` | 248, 385 | Implementation #3 |
| `src/headends/embed-headend.ts` | 415 | Implementation #3 |
| `src/headends/openai-completions-headend.ts` | 261 | Implementation #3 |
| `src/headends/anthropic-completions-headend.ts` | 262 | Implementation #3 |
| `src/headends/mcp-headend.ts` | 362-366 | Implementation #3b (special) |

### Headends (stop signal checking logic)

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/headends/embed-headend.ts` | 376 | Check stopRef reason before processing |
| `src/headends/rest-headend.ts` | 251, 258, 388, 395 | Check stopRef reason |
| `src/headends/openai-completions-headend.ts` | 264 | Check stopRef reason |
| `src/headends/anthropic-completions-headend.ts` | 265 | Check stopRef reason |
| `src/headends/mcp-headend.ts` | 366 | Check stopRef reason |
| `src/ai-agent.ts` | 535-543, 1777, 1814 | Update stopRef checks to include reason |

### Context Guard

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/context-guard.ts` | ~80, new method | Implementation #4 |

### ForcedFinalReason Union Updates

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/llm-messages-xml-next.ts` | 87 | Add `'user_stop'` to `FinalTurnReason` |
| `src/ai-agent.ts` | 132, 1990, 2070 | Update stopRef type, `'user_stop'` in getter/logging |
| `src/xml-tools.ts` | 30 | Add `'user_stop'` to union |
| `src/xml-transport.ts` | 66 | Add `'user_stop'` to union |
| `src/telemetry/index.ts` | ~122 | Add `'user_stop'` to metrics |
| `src/headends/headend-manager.ts` | 17, 27, 35 | Update stopRef type |

### Orchestration / Child Session Propagation

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/orchestration/spawn-child.ts` | 36-47, 120-173, 188-208 | Propagate stopRef reason to child sessions |
| `src/orchestration/handoff.ts` | 42-44 | Update stopRef interface |
| `src/orchestration/advisors.ts` | 49-51 | Update stopRef interface |
| `src/subagent-registry.ts` | 165-171, 308-309 | Propagate stopRef reason |
| `src/agent-registry.ts` | 35-36, 124-125 | Update stopRef interface |

### Additional Source Files

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/agent-loader.ts` | 798-800 | Forward stopRef with reason |
| `src/cli.ts` | 1528-1529 | Pass stopRef from shutdownController |
| `src/headends/slack-headend.ts` | 160-169, 172-174 | Store and propagate stopRef with reason |

### Provider/Tool Layers (NO CHANGES - AbortSignal unchanged)

| File | Lines | Note |
|------|-------|------|
| `src/llm-providers/base.ts` | 1325-1331 | AbortSignal handling unchanged (only triggers for abort/shutdown) |
| `src/tools/queue-manager.ts` | 86-118 | AbortSignal handling unchanged |
| `src/tools/rest-provider.ts` | 237-253 | AbortSignal handling unchanged |
| `src/headends/concurrency.ts` | 32-75 | AbortSignal handling unchanged |

### Tests

| File | Lines | Change Required |
|------|-------|-----------------|
| `src/tests/unit/headend-log-utils.spec.ts` | ~13 | Update stopRef creation |
| `src/tests/unit/shutdown-utils.spec.ts` | All | Test new StopRef interface |
| `src/tests/smoke-rest-headend.ts` | 42, 61 | Creates stopRef, sets stopping → update |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 1363 | Checks `stopRef.stopping` → add reason check |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 3064-3066 | Abort signal tests → verify abort triggers for abort/shutdown only |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 3141-3150 | Expects immediate exit on stopRef → update: `stop` = final turn, `abort` = immediate |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 6150-6312 | run-test-73: Stop reference → update assertions |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | 9123-9150 | Stop reference polling → update for new interface |
| `src/tests/phase2-harness-scenarios/phase2-runner.ts` | ~9695+ | run-test-44 → update (expects no final report, now depends on reason) |

### Documentation (Consolidated - ALL docs)

| File | Lines | Change Required |
|------|-------|-----------------|
| `docs/specs/session-lifecycle.md` | 236, 249 | Update stopRef docs |
| `docs/Technical-Specs-Session-Lifecycle.md` | 124, 516 | Update stopRef docs |
| `docs/Headends-Library.md` | 349 | Update stopRef type |
| `docs/Advanced-Internal-API.md` | 214, 575-583 | Update stopRef examples |
| `docs/Technical-Specs-Architecture.md` | 354, 419 | Update EXIT-USER-STOP, stopRef |
| `docs/specs/architecture.md` | 136, 174, 185, 210 | Update stopRef/EXIT-USER-STOP |
| `docs/specs/library-api.md` | 195 | Update stopRef type |
| `docs/specs/IMPLEMENTATION.md` | 306 | Update stopRef plumbing |
| `docs/specs/retry-strategy.md` | 182 | Update sleepWithAbort docs |
| `docs/specs/headend-rest.md` | 51, 153, 210 | Update stopRef references |
| `docs/specs/headend-slack.md` | 376 | Update stopRef propagation |
| `docs/Operations-Logging.md` | 309 | EXIT-USER-STOP already documented |
| `docs/skills/ai-agent-guide.md` | 989 | EXIT-USER-STOP already documented |

### New Test Cases Required

| Test | Description |
|------|-------------|
| `stop-reason-stop` | User "Stop" → final turn → graceful exit with summary |
| `stop-reason-abort` | User "Abort" → immediate cancel → success=false |
| `stop-reason-shutdown` | Global shutdown → immediate cancel → success=false |
| `stop-during-rate-limit` | Stop while rate-limited → final turn after sleep |
| `final-turn-after-stop-calls-final-report` | Final turn can call `final_report` tool |
| `mcp-headend-stop-propagation` | MCP headend inherits globalStopRef reason |
