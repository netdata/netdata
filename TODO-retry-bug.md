# TODO: Fix Retry Logic Bugs in Final Report Handling

## TL;DR (Nov 19, 2025)

- ✅ **Runtime + docs aligned**: `pendingFinalReport`, truthful logging helpers, and synthetic failure enforcement remain stable (`src/ai-agent.ts:1468-2645`, `src/telemetry/index.ts:688-742`). Docs already describe the flow across `docs/DESIGN.md`, `docs/specs/retry-strategy.md`, `docs/specs/session-lifecycle.md`, `docs/specs/logging-overview.md`, `docs/AI-AGENT-GUIDE.md`, and `docs/TESTING.md`.
- 📊 **Observability next**: Counters `ai_agent_final_report_*` and `ai_agent_retry_collapse_total` exist, but dashboards/alerts + runbook references are still missing. We must verify label coverage (source/status/reason/headend) before claiming production readiness.
- 🧪 **Deterministic harness gap**: 24 / 41 planned Phase 1 scenarios are implemented. Categories 7 (logging) and 8 ("never return empty") remain uncovered, and Category 1–6 each have at least one missing fixture (details below). Full-suite runs are still pending for the latest branch.
- 🗂️ **Single source of truth**: `TODO-retry-testing-plan.md`, `TODO-review-retry-fixes.md`, and `REVIEW-codex-retry-fixes.md` were retired on Nov 19 2025. This file now owns the plan, status, and review notes.

## Consolidation Note (Nov 19, 2025)
- Removed `TODO-retry-testing-plan.md`, `TODO-review-retry-fixes.md`, and `REVIEW-codex-retry-fixes.md`; their analysis and matrices are merged into the sections below.
- Any future scope/plan/test changes **must** update this file directly; no parallel TODO/review docs allowed for the retry workstream.

## Directive Update (Nov 19, 2025)
- **Standing order (Costa)**: finish documentation + telemetry/observability wiring **before** expanding the automated test suite.
- **Doc status**: Runtime design updates already landed across DESIGN/specs/AI-AGENT-GUIDE/TESTING/session-lifecycle/logging. Need one more editorial pass tying each section to the production dashboards/runbooks once those exist.
- **Implication**: Harness additions (Categories 1–8 completion + full Phase 1 runs) remain blocked on observability deliverables.

## Execution Order & Plan
1. **Observability & Runbooks (highest priority)**
   - Wire `ai_agent_final_report_*` + `ai_agent_retry_collapse_total` into dashboards with alerts on `source!='tool-call'`, rising synthetic counts, or repeated collapses.
   - Capture expected label cardinality (source, status, forced_final_reason, headend, synthetic_reason) and document alarm thresholds in `docs/TESTING.md` + `docs/DESIGN.md`.
   - Add a verification checklist to the release runbook ensuring `(no output)` responses correlate with `agent:failure-report` logs.
2. **Documentation cross-check (fast follow once dashboards exist)**
   - Update the doc sections listed above with links/screenshots of the dashboards/alerts plus the new log identifiers (`agent:turn-start`, `agent:final-turn`, `agent:text-extraction`, `agent:fallback-report`, `agent:final-report-accepted`, `agent:failure-report`).
   - Confirm telemetry names and field semantics match the shipped instrumentation (no drift between `src/telemetry/index.ts` and docs).
   - Reflect harness expectations in `docs/TESTING.md`: five control surfaces (user prompt, test MCP tool, test LLM provider, final response, telemetry) and contract-first assertions. ✅ Updated Nov 19, 2025.
3. **Deterministic harness + regression tests (unblock after #1+#2)**
   - Implement the missing fixtures per the Testing Matrix Snapshot below (Category counts 1.7, 2.5, 3.context-limit, 4.log-assertions, 5.provider-timeouts, all Category 7 + 8 cases).
   - Run **full** Phase 1 suite locally and capture logs, then stage whichever targeted Phase 2 scenarios Costa requests.
   - Ensure every new scenario asserts telemetry/log identifiers where applicable (especially `finalReportSource` + `agent:final-report-accepted`).

## Testing Matrix Snapshot (Nov 19, 2025)
- **Category 1 – Final-report tool call variations (6/7)**: Missing `final-report-missing-status` fixture that proves we retry + collapse without adopting text fallback.
- **Category 2 – Text extraction fallback (4/5)**: Need `text-extraction-pending-no-finalization` to show cached payload survives intermediate turns until a real tool call appears.
- **Category 3 – Forced-final-turn triggers (3/4)**: Context-limit scenario lacks assertions for `agent:final-report-accepted` source + `agent:EXIT-TOKEN-LIMIT` telemetry labels.
- **Category 4 – Turn collapse auditing (4/5)**: Must add a log-focused test asserting `recordRetryCollapseMetrics` + `agent:orchestrator` output when collapse shortens `maxTurns`.
- **Category 5 – Provider failure exhaustion (4/5)**: Need deterministic "provider errors every attempt" case yielding synthetic failure reason `llm_error` with no Phase 2 usage.
- **Category 6 – Tool mixing edge cases (complete for behavior)**: Additional log assertions optional but not urgent once Categories 7–8 exist.
- **Category 7 – Logging guarantees (0/6)**: No automated coverage yet for lifecycle log stack (`agent:turn-start`, `agent:final-turn`, `agent:text-extraction`, `agent:fallback-report`, `agent:final-report-accepted`, `agent:failure-report`).
- **Category 8 – "Never return empty" guarantees (1/5)**: Only `max-retries-exhausted-must-generate-report` exists. Still need fixtures for `all-tools-dropped-no-text`, `context-limit-no-final-report`, `llm-empty-string`, and `llm-timeout` to assert synthetic report generation.

## Harness Test Strategy Findings (Nov 19, 2025)
- **Contract vs. internals gap**: The harness currently inspects `result.logs` roughly 200 times (`rg "result\\.logs" src/tests/phase1/runner.ts`) and relies on helpers like `expectLogIncludes`/`expectLogDetailNumber` defined at `src/tests/phase1/runner.ts:331-333` and `821-825`. These checks lock in exact log identifiers (`agent:text-extraction`, `agent:final-report-accepted`, `agent:failure-report`, etc.) and detail keys (`ctx_tokens`, `schema_tokens`, `expected_tokens`). Any harmless wording/schema change breaks the suite even if the end-user contract (final report + MCP/tool outputs) is satisfied.
- **Documentation reinforces the pattern**: `docs/TESTING.md` currently tells contributors to assert telemetry and log identifiers for new retry/finalization cases (lines 62-66), so the log dependency is treated as a requirement rather than an anti-pattern.
- **Scenario expectations skewed toward internals**: Recent fixtures such as `run-test-final-report-retry-text` (`src/tests/phase1/runner.ts:9887-9950`) and `run-test-final-report-tool-message-fallback` (`~10100-10220`) assert sanitizer logs, collapse logs, fallback logs, and `agent:final-report-accepted` sources while barely examining the MCP tool transcript or final user-visible payload. Similar log-heavy assertions exist across the context-guard, telemetry, and restart suites.
- **Fragility already observed**: Refactors to logging helpers (e.g., splitting `logFinalTurnWarning`) required editing dozens of log assertions around lines 10185-11120, showing that implementation refactors—even when contract behavior is identical—cause large swaths of tests to fail.
- **Required adjustment**: Before adding more coverage, we need to redesign Phase 1 expectations to validate behavior via the three permitted levers (user input, deterministic LLM provider output, deterministic MCP tool behavior). Only rely on logs when no contract-level signal exists, and consider emitting explicit `events` in `AIAgentResult` if we truly must capture internal sequencing.

## Status Snapshot

### Verified fixes
- `pendingFinalReport` introduced to decouple text extraction from tool-call bookkeeping (`src/ai-agent.ts:210`, `1907-1920`, `3400-3421`).
- Sanitizer increments `finalReportAttempts` whenever it drops malformed `agent__final_report` payloads; tool executor increments on valid invocations (`src/ai-agent.ts:3579-3635`, `3680-3860`).
- Retries now fire because `sanitizedHasToolCalls` stays `false` when every tool call was dropped; fallback acceptance happens only inside `acceptPendingFinalReport`.
- Failure-mode enforcement synthesizes structured reports when `maxTurns`, `maxRetries`, or context guards stop progress (`src/ai-agent.ts:2579-2645`).
- Deterministic harness coverage now includes tool-message fallback (`run-test-final-report-tool-message-fallback`) and repeated invalid final-report attempts (`run-test-final-report-max-retries-synthetic`).

### Open items
1. **Test coverage breadth** – run the entire Phase 1 harness (and any Phase 2 scenarios Costa needs) after every major edit; keep adding cases from `TODO-retry-testing-plan.md` until all categories are represented.
2. **Observability wiring** – hook the new metrics (`ai_agent_final_report_*`, `ai_agent_retry_collapse_total`) into dashboards/alerts so `source!='tool-call'` and synthetic failures trigger paging in production.

> The sections below capture the historical analysis that led to the fix. They remain for context but no longer describe the current runtime behavior verbatim.

## Historical Analysis (Pre-Fix Reference)

### Current Control-Flow When a Final Report Tool Call is Dropped (Turn 4 example)

```
1. LLM sends response with:
   - Invalid agent__final_report tool call (malformed parameters)
   - Text content containing the same final report as JSON

2. Sanitizer drops invalid tool call
   → droppedInvalidToolCalls = 1
   → sanitizedHasToolCalls = false
   → Logs: "ERR agent/sanitizer Dropped 1 invalid tool call(s)"

3. `tryAdoptFinalReportFromText` executes (line 1871-1874)
   → Extracts final report from text (requires `sanitizedHasText = true`)
   → Sets `this.finalReport = finalReportPayload` (line 3414)
   → Returns true

4. BUG #2 triggers (line 1873)
   → `sanitizedHasToolCalls = true`

5. Retry guard bypassed (line 1937)
   → Condition: `droppedInvalidToolCalls > 0 && !sanitizedHasToolCalls`
   → Evaluates to `true && false = false`
   → **No retry executed** even though every tool call was dropped

6. Even if we forced a retry, BUG #3 (premature finalization) would still fire because `this.finalReport` is already populated; the next time we append sanitized messages we would hit line 2005 and exit immediately unless we reset that fallback payload

7. Code continues to line 2003
   → Pushes sanitized messages to conversation

8. Exit check triggers (line 2005)
   → Condition: `this.finalReport !== undefined`
   → Evaluates to `true`
   → Exits with EXIT-FINAL-ANSWER

9. BUG #4 triggers (line 2081)
   → Logs: "Final turn detected: restricting tools to `agent__final_report`"
   → Misleading because we were not at the final-turn limit—we simply short-circuited due to fallback adoption

Result: Turn 4 of 20, no retries, confusing logs, "(no output)" to user
```

### Bug #1: Dead Retry Flag - `finalReportAttempted` (✅ FIXED IN CODE)

- **Verification**: `finalReportAttempted` is now toggled in two places: the sanitizer (src/ai-agent.ts:3577-3593) bumps the counter whenever it drops an `agent__final_report` payload, and the tool executor (src/ai-agent.ts:3713-3722) increments it for every legitimate call. We also forward the flag into the retry logic (src/ai-agent.ts:2181-2186).
- **Result**: The "collapse maxTurns to currentTurn + 1" path is reachable again; the flag is no longer dead.
- **Action**: No code change required here. This section remains for historical context only; do **not** re-implement the same fix.

---

### Bug #2: Synthetic Tool Call Injection (CURRENT)

**Location**: `tryAdoptFinalReportFromText` (src/ai-agent.ts:3523-3546)

**What the code does today**:
```typescript
if (assistantMessage !== undefined) {
  const syntheticCall: ToolCall = {
    id: 'synthetic-final-report-' + Date.now().toString(),
    name: AIAgentSession.FINAL_REPORT_TOOL,
    parameters,
  };
  assistantMessage.toolCalls = [syntheticCall];
}
```

**Why this is fatal**:
- The sanitizer rightfully drops the malformed tool call. Immediately afterwards, `tryAdoptFinalReportFromText` injects a *fake* `agent__final_report` call with the extracted payload.
- When `turnResult.status.finalAnswer === true`, `adoptFromToolCall()` sees this fabricated entry, concludes “great, we got a final report”, and calls `commitFinalReport(...)` with the fallback content.
- Once `commitFinalReport` fires, we log `agent:final-report-accepted Final report accepted from tool call` and mark the turn successful. The retry branch (`if (droppedInvalidToolCalls > 0 && !sanitizedHasToolCalls)`) never executes because `sanitizedHasToolCalls` flips to `true` inside `adoptFromToolCall()`.

**Observed symptoms (matches production logs)**:
1. The sanitizer still logs `call 0: parameters not object`, but the very next log is `agent:final-report-accepted Final report accepted from tool call.`
2. Sessions stop immediately after the invalid payload (Costa’s “why did we stop at turn 4 with 20 configured?”).
3. The CLI/headend reports `(no tool retries)` even though we clearly rejected the call.
4. Users receive the fallback text while logs insist it came from the tool call (“why did we say successful final-report while we rejected it?”).

**Required fix**: Remove the mutation entirely. `tryAdoptFinalReportFromText` should *only* return a `PendingFinalReportPayload` and leave `assistantMessage.toolCalls` undefined. That keeps `sanitizedHasToolCalls` false, triggers the retry guard, and preserves the pending fallback for acceptance on the actual final turn.

### Bug #3: Fallback Adoption Commits `this.finalReport` Immediately (✅ FIXED VIA `pendingFinalReport`)

- **Current behavior**: Both text and tool-message extraction now populate `this.pendingFinalReport` (src/ai-agent.ts:1885, 1953) instead of `this.finalReport`. The actual acceptance happens later inside `acceptPendingFinalReport()` once we enter the final turn (src/ai-agent.ts:2545-2551).
- **Impact**: The premature-finalization issue described in the original TODO is no longer present; retries remain possible after we capture a fallback payload.
- **Action**: Keep this section as historical record only.

---

### Bug #4: Misleading "Final Turn Detected" Log (✅ FIXED VIA NEW HELPERS)

- **Status**: Replaced with explicit helpers (`logTurnStart`, `logEnteringFinalTurn`, `logFinalReportAccepted`, `logFallbackAcceptance`, `logFailureReport`) at src/ai-agent.ts:3204-3350.
- **Behavior now**:
  - Entering a forced final turn logs `agent:final-turn` once (line 1590).
  - Accepting a report logs `agent:final-report-accepted` with `details.source` so we can distinguish tool-call/text/tool-message/synthetic.
  - Fallback acceptance and synthetic failures have their own identifiers.
- **Action**: No additional work needed for this bug; keep monitoring to ensure new identifiers stay wired into the structured log pipeline.

---

## Historical Decisions (Resolved)
*For reference only; these decisions already landed with the current implementation.*

### Decision 1: How to track final-report attempts?

**Options**:

A. **Add flag in turn result** (recommended)
   - When sanitizer drops a tool call with name `agent__final_report`, set a flag on the turn result
   - Pass this flag through to the retry check at line 2240
   - Pros: Minimal change, follows existing pattern
   - Cons: Need to thread flag through turn result structure

B. **Track in session state**
   - Add `this.finalReportAttemptedThisTurn` boolean to session
   - Set it when sanitizer drops final-report call
   - Reset it at start of each turn
   - Pros: Simple, no need to modify turn result
   - Cons: More stateful, harder to reason about

C. **Detect from conversation history**
   - Check if previous turn had a dropped final-report call
   - Pros: No new state needed
   - Cons: Fragile, hard to implement correctly

**Recommendation**: Option A - add `finalReportAttempted: boolean` to turn result, set it in sanitizer when dropping `agent__final_report` calls

---

### Decision 2: What to do when extracting final report from text?

**Options**:

A. **Don't set `sanitizedHasToolCalls = true`** (recommended for correctness)
   - Remove line 1873 entirely
   - Let retry guards work based on actual tool call status
   - If we want to exit immediately with the extracted report, do it explicitly
   - Pros: Semantically correct, retry guards work as designed
   - Cons: Need to decide if/when to exit when extracting from text

B. **Keep current behavior but fix retry guard**
   - Keep `sanitizedHasToolCalls = true` at line 1873
   - Change retry guard at line 1937 to also check if final report was extracted from text
   - Pros: Minimal change
   - Cons: Semantic confusion persists, harder to reason about

C. **Remove text extraction entirely**
   - Only accept final reports via proper tool calls
   - Retry if tool call is malformed
   - Pros: Clearest semantics, forces LLMs to use proper tool calls
   - Cons: May reduce robustness with models that struggle with tool calls

**Recommendation**: Option A - remove line 1873, make retry decision explicit based on what actually happened

---

### Decision 3: Should text extraction immediately exit or allow retry?

**Context**: When LLM sends both a malformed tool call AND text with the report, what should we do?

**Options**:

A. **Immediately exit with text extraction** (current behavior)
   - Accept the extracted report as valid
   - Exit without retry
   - Pros: More forgiving to LLM mistakes
   - Cons: May accept low-quality reports, doesn't teach LLM to use proper tool calls

B. **Retry with text extraction as fallback**
   - First try to get LLM to send proper tool call
   - After max retries, fall back to text extraction if available
   - Pros: Encourages proper tool use, better quality reports
   - Cons: More LLM calls, may waste tokens

C. **Never extract from text, always retry**
   - Reject malformed tool calls even if text has the report
   - Force LLM to use proper tool calls
   - Pros: Cleanest semantics, best for model training
   - Cons: May fail with models that struggle with tool calls

**Recommendation**: Option B - retry first, extract from text only as last resort (after max retries on final turn)

---

### Decision 4: How to fix the misleading logs?

**Options**:

A. **Split the log function** (recommended)
   - Create separate functions:
     - `logEnteringFinalTurn()` - called at line 1582
     - `logFinalReportAccepted()` - called at line 2081
     - `logFinalReportExtractedFromText()` - called at line 1872
   - Each has distinct message
   - Pros: Clear, unambiguous logs
   - Cons: More functions to maintain

B. **Add context parameter**
   - Extend `logFinalTurnWarning` to take a context parameter
   - Different messages based on context
   - Pros: Single function
   - Cons: Function does too much, harder to read

C. **Just fix the message**
   - Change message at line 2081 to "Final report received"
   - Keep "Final turn detected" only at line 1582
   - Pros: Minimal change
   - Cons: Doesn't address full clarity issue

**Recommendation**: Option A - split into distinct log functions with clear, accurate messages

---

## Historical Implementation Plan (Pre-Fix)

### 1. Fix Bug #1: Track final-report attempts

**Files to modify**: src/ai-agent.ts, src/types.ts

**Changes**:

1. Add `finalReportAttempted?: boolean` to `TurnStatus` type (src/types.ts)

2. In `sanitizeTurnMessages` (src/ai-agent.ts:3418-3496):
   ```typescript
   let finalReportAttempted = false;

   rawToolCalls.forEach((tcRaw, callIndex) => {
     // ... existing validation ...

     if (paramsCandidate === undefined) {
       const preview = this.previewRawParameter(rawParameters);
       droppedReasons.push(`call ${callIndexStr}: parameters not object (raw preview: ${preview})`);

       // NEW: Track if this was a final-report attempt
       if (sanitizeToolName(rawName) === AIAgentSession.FINAL_REPORT_TOOL) {
         finalReportAttempted = true;
       }

       return;
     }
     // ...
   });

   return { messages: sanitized, dropped, finalReportAttempted };
   ```

3. Thread the flag through to turn result:
   ```typescript
   const { messages: sanitizedMessages, dropped: droppedInvalidToolCalls, finalReportAttempted } = this.sanitizeTurnMessages(
     turnResult.messages,
     { turn: currentTurn, provider, model }
   );

   // Add to turn result
   if (finalReportAttempted) {
     (turnResult.status as { finalReportAttempted?: boolean }).finalReportAttempted = true;
   }
   ```

4. The existing check at line 2240-2253 will now work correctly

---

### 2. Fix Bug #2: Don't corrupt `sanitizedHasToolCalls`

**Files to modify**: src/ai-agent.ts

**Changes**:

1. Remove line 1873:
   ```typescript
   if (this.finalReport === undefined && !sanitizedHasToolCalls && sanitizedHasText) {
     if (this.tryAdoptFinalReportFromText(assistantForAdoption, textContent)) {
       // DELETE THIS LINE:
       // sanitizedHasToolCalls = true;

       // INSTEAD: Add explicit handling
       // (see Decision 3 for what to do here)
     }
   }
   ```

2. Change `tryAdoptFinalReportFromText` (and `adoptFromToolMessage`) so they **return** the parsed payload instead of mutating `this.finalReport`. Store that payload in a new field such as `this.pendingFinalReportFromText`.

3. Implement Decision 3 strategy (retry with text extraction as fallback):
   ```typescript
   if (this.finalReport === undefined && !sanitizedHasToolCalls && sanitizedHasText) {
     const extracted = this.tryAdoptFinalReportFromText(assistantForAdoption, textContent);
     if (extracted !== undefined) {
       this.pendingFinalReportFromText = extracted;

       // Log that we extracted from text after tool call failure
       const warnEntry: LogEntry = {
         timestamp: Date.now(),
         severity: 'WRN',
         turn: currentTurn,
         subturn: 0,
         direction: 'response',
         type: 'llm',
         remoteIdentifier: 'agent:text-extraction',
         fatal: false,
         message: 'Final report extracted from text content after tool call rejection. Retrying for proper tool call.',
       };
       this.log(warnEntry);
     }
   }
   ```

4. Only accept text extraction as final resort on final turn:
   ```typescript
   // At the end of turn loop, after all retries exhausted
   if (isFinalTurn && this.finalReport === undefined && this.pendingFinalReportFromText !== undefined) {
     // Accept the text extraction as final report
     this.finalReport = { ...this.pendingFinalReportFromText, ts: Date.now() };
     // Log this clearly
     const acceptEntry: LogEntry = {
       timestamp: Date.now(),
       severity: 'WRN',
       turn: currentTurn,
       subturn: 0,
       direction: 'response',
       type: 'llm',
       remoteIdentifier: 'agent:text-extraction',
       fatal: false,
       message: 'Accepting final report from text extraction as last resort (final turn, no valid tool call).',
     };
     this.log(acceptEntry);
   }
   ```

---

### 3. Fix Bug #4: Split misleading log functions

**Files to modify**: src/ai-agent.ts

**Changes**:

1. Create three distinct log functions:
   ```typescript
   private logEnteringFinalTurn(reason: 'context' | 'max_turns', turn: number): void {
     if (this.finalTurnEntryLogged) return;
     const message = reason === 'context'
       ? `Context guard enforced: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`
       : `Final turn (${turn}) detected: restricting tools to \`${AIAgentSession.FINAL_REPORT_TOOL}\` and injecting finalization instruction.`;
     const entry: LogEntry = {
       timestamp: Date.now(),
       severity: 'WRN',
       turn,
       subturn: 0,
       direction: 'request',
       type: 'llm',
       remoteIdentifier: 'agent:final-turn',
       fatal: false,
       message,
     };
     this.log(entry);
     this.finalTurnEntryLogged = true;
   }

   private logFinalReportAccepted(turn: number, source: 'tool-call' | 'text-extraction'): void {
     const message = source === 'tool-call'
       ? `Final report accepted from tool call (${AIAgentSession.FINAL_REPORT_TOOL}), session complete.`
       : `Final report accepted from text extraction after tool call failure, session complete.`;
     const entry: LogEntry = {
       timestamp: Date.now(),
       severity: 'VRB',
       turn,
       subturn: 0,
       direction: 'response',
       type: 'llm',
       remoteIdentifier: 'agent:final-report-accepted',
       fatal: false,
       message,
     };
     this.log(entry);
   }
   ```

2. Update call sites:
   - Line 1582: `this.logEnteringFinalTurn(forcedFinalTurn ? 'context' : 'max_turns', currentTurn);`
   - Line 2081: DELETE (don't log "final turn detected" when exiting)
   - Line 2088: `this.logFinalReportAccepted(currentTurn, 'tool-call');` (track source)

3. Remove old `logFinalTurnWarning` function (line 3258-3275)

---

## Historical Implied Decisions

1. **Session state additions**:
   - Add `this.pendingFinalReportFromText?: { status: ... }` (and similar for tool-message fallback) instead of mutating `this.finalReport`
   - Add `this.finalTurnEntryLogged: boolean` to prevent duplicate logs

2. **Turn result structure**:
   - Add `finalReportAttempted?: boolean` to `TurnStatus` type
   - Pass this flag through sanitizer → turn result → retry check

3. **Sanitizer return type**:
   - Change from `{ messages, dropped }` to `{ messages, dropped, finalReportAttempted }`

4. **Log entry identifiers**:
   - New: `agent:text-extraction` for text extraction logs
   - New: `agent:final-report-accepted` for acceptance logs
   - Change: `agent:final-turn` only for actual final turn entry

---

## Testing Requirements

### Unit Tests

1. **Test final-report attempt tracking**:
   - Given: Sanitizer receives malformed `agent__final_report` tool call
   - When: `sanitizeTurnMessages` is called
   - Then: Returns `{ finalReportAttempted: true }`

2. **Test retry collapse with finalReportAttempted**:
   - Given: Turn result with `finalReportAttempted: true` and `currentTurn < maxTurns`
   - When: Retry check at line 2240 executes
   - Then: `maxTurns` is collapsed to `currentTurn + 1`

3. **Test sanitizedHasToolCalls remains false after text extraction**:
   - Given: Invalid tool call dropped, text contains valid final report
   - When: `tryAdoptFinalReportFromText` succeeds
   - Then: `sanitizedHasToolCalls` remains `false`

4. **Test retry guard triggers after text extraction**:
   - Given: Invalid tool call dropped, text extraction succeeded, `sanitizedHasToolCalls = false`
   - When: Retry guard check executes
   - Then: Retry is triggered with system message

### Integration Tests (Phase 1 Harness)

1. **Scenario: Malformed final-report tool call with valid text**:
   ```typescript
   {
     name: 'malformed-final-report-with-text',
     turns: [
       {
         request: { tools: ['agent__final_report'], prompt: 'Complete the task' },
         response: {
           toolCalls: [
             {
               id: 'call_1',
               name: 'agent__final_report',
               parameters: '{"status":"success","report_content":"Result"}',  // String instead of object
             }
           ],
           content: '{"status":"success","report_format":"text","report_content":"Result"}',  // Valid in text
         },
       },
     ],
     expectedBehavior: {
       finalReportAccepted: false,  // Should NOT accept on first turn
       maxTurnsCollapsed: true,      // Should collapse to currentTurn + 1
       retryTriggered: true,         // Should retry for proper tool call
       logsContain: [
         'agent/sanitizer Dropped 1 invalid tool call',
         'agent:text-extraction Final report extracted from text',
         'agent:orchestrator Collapsing remaining turns',
       ],
     },
   }
   ```

2. **Scenario: Retry exhaustion with text fallback**:
   ```typescript
   {
     name: 'retry-exhaustion-text-fallback',
     turns: [
       // Turn 1-5: Invalid tool calls, text extraction available
       // ...
       // Final turn: Accept text extraction
     ],
     expectedBehavior: {
       finalReportAccepted: true,
       finalReportSource: 'text-extraction',
       logsContain: [
         'agent:text-extraction Accepting final report from text extraction as last resort',
       ],
     },
   }
   ```

3. **Scenario: Proper tool call after invalid attempt**:
   ```typescript
   {
     name: 'proper-call-after-invalid',
     turns: [
       {
         // Turn 1: Invalid tool call
         response: {
           toolCalls: [{ parameters: 'invalid-string' }],
         },
       },
       {
         // Turn 2: Proper tool call
         response: {
           toolCalls: [
             {
               id: 'call_2',
               name: 'agent__final_report',
               parameters: { status: 'success', report_format: 'text', report_content: 'Result' },
             }
           ],
         },
       },
     ],
     expectedBehavior: {
       finalReportAccepted: true,
       finalReportSource: 'tool-call',
       turnCount: 2,
       logsContain: [
         'agent:final-report-accepted Final report accepted from tool call',
       ],
     },
   }
   ```

---

## Documentation Updates Required

### Files to update:

1. **docs/DESIGN.md**:
   - Section on final-report retry logic
   - Explain `finalReportAttempted` flag
   - Clarify text extraction as fallback strategy
   - Update exit conditions and logging behavior

2. **docs/specs/retry-strategy.md**:
   - Add final-report attempt tracking
   - Explain turn collapse when final-report is attempted but fails
   - Document text extraction fallback behavior

3. **docs/AI-AGENT-GUIDE.md**:
   - Update final_report tool documentation
   - Explain what happens when tool call is malformed
   - Document retry behavior and text extraction fallback
   - Show example log sequences for different scenarios

---

## Next Steps

### Phase 1: Fix core bugs (no behavior changes yet)

1. Add `finalReportAttempted` tracking in sanitizer
2. Thread flag through to retry check
3. Verify retry collapse triggers correctly
4. **DO NOT change text extraction behavior yet**

### Phase 2: Fix semantic bug (behavior change)

1. Remove `sanitizedHasToolCalls = true` at line 1873
2. Implement retry-first, text-extraction-fallback strategy
3. Add clear logging for text extraction
4. Verify retry guards work correctly

### Phase 3: Fix misleading logs

1. Split `logFinalTurnWarning` into distinct functions
2. Update all call sites
3. Verify log clarity in real sessions

### Phase 4: Testing

1. Add unit tests for all new behaviors
2. Add Phase 1 harness scenarios
3. Run against real LLMs (nova, gpt-4, claude-3.5)
4. Verify logs are clear and accurate

### Phase 5: Documentation

1. Update DESIGN.md with new retry logic
2. Update retry-strategy.md with final-report handling
3. Update AI-AGENT-GUIDE.md with examples
4. Update CHANGELOG.md

---

## Open Questions

1. Should we auto-repair simple JSON violations (escaped newlines) in final-report payloads, or keep strict validation?
   - Recommendation: Keep strict for now, revisit if this becomes common

2. What's the max number of retries we should allow specifically for final-report attempts?
   - Current: Uses existing `maxRetries` (5)
   - Recommendation: Keep same, but make it explicit in logging

3. Should text extraction be disabled entirely with a config flag for strict mode?
   - Recommendation: Yes, add `strictFinalReport: boolean` to session config

4. Should we emit the "(no output)" placeholder to headend when a turn is dropped?
   - Recommendation: Yes, add system message: "(Turn dropped due to invalid tool call, retrying...)"
