# Retry Strategy

## TL;DR
Per-turn retry with provider cycling, exponential backoff for rate limits, synthetic retries for invalid responses, context-aware final turn enforcement.

## Source Files
- `src/ai-agent.ts:1390-2600+` - executeAgentLoop retry logic
- `src/llm-providers/base.ts` - mapError(), retry directives
- `src/types.ts:57-63` - TurnRetryDirective

## Retry Architecture

### Retry Directive
**Location**: `src/types.ts:57-63`

```typescript
interface TurnRetryDirective {
  action: 'retry' | 'skip-provider' | 'abort';
  backoffMs?: number;
  logMessage?: string;
  systemMessage?: string;
  sources?: string[];
}
```

### Retry Loop Structure
**Location**: `src/session-turn-runner.ts:389-1552`

```
for turn = 1 to maxTurns:
  attempts = 0
  pairCursor = 0

  while attempts < maxRetries and not successful:
    # Reset per-attempt error state (line 390-393)
    lastError = undefined
    lastErrorType = undefined

    pair = targets[pairCursor % targets.length]
    pairCursor += 1
    attempts += 1

    result = executeSingleTurn(pair.provider, pair.model, ...)

    if success:
      if hasToolCalls or finalReport:
        turnSuccessful = true
      else:
        synthetic retry (content without tools)
    else:
      handle error based on status type
      maybe wait (backoff)
      continue to next attempt
```

### Per-Attempt Error State Reset
**Location**: `src/session-turn-runner.ts:390-393`

At the start of each attempt within a turn, `lastError` and `lastErrorType` are reset to `undefined`. This prevents a failed attempt from poisoning later successful attempts within the same turn.

**Example scenario**:
1. Attempt 1: Final report validation fails → `lastErrorType = 'invalid_response'`
2. Attempt 2: LLM runs tools successfully (no final report)
3. Without reset: Tools would be blocked from marking turn successful
4. With reset: Tools correctly mark turn successful

### Final Report Validation Retry
**Location**: `src/session-turn-runner.ts:1329-1358`

When a final report fails schema validation:
1. `validatePayload()` is called BEFORE `commitFinalReport()`
2. On failure: Sets retry flags and returns `false`
3. `lastErrorType = 'invalid_response'` triggers retry at line 1500-1507
4. Turn is NOT marked successful until validation passes

## Error Classification

### Fatal Errors (No Retry)
**Stop session immediately**:

| Status | Condition | Exit Code |
|--------|-----------|-----------|
| `auth_error` | Invalid API key | EXIT-AUTH-FAILURE |
| `quota_exceeded` | Account quota hit | EXIT-QUOTA-EXCEEDED |

### Retryable Errors
**Try next provider/model**:

| Status | Condition | Backoff |
|--------|-----------|---------|
| `rate_limit` | Too many requests | retryAfterMs or exponential |
| `model_error` | Model unavailable (retryable=true) | None |
| `network_error` | Connection failed (retryable=true) | None |
| `timeout` | Request timeout | None |
| `invalid_response` | Malformed response | None |

### Skip-Provider Errors
**Move to next provider in targets**:

| Condition | Action |
|-----------|--------|
| Model not supported | Skip to next target |
| Provider temporary issue | Skip to next target |

## Synthetic Retries

### Content Without Tools
**Location**: `src/ai-agent.ts:1973-2011`

**Trigger**: LLM returns text content but no tool calls and no final_report

**Behavior**:
1. Log warning: "Synthetic retry: assistant returned content without tool calls"
2. Do NOT add to conversation
3. Inject system message: "System notice: plain text responses are ignored..."
4. Continue to next attempt

**Purpose**: Force model to use tools or call final_report

### Invalid Tool Parameters
**Location**: `src/ai-agent.ts:1951-1970`

**Trigger**: Tool call has malformed JSON parameters

**Behavior**:
1. Drop invalid tool call
2. Log warning: "Invalid tool call dropped due to malformed payload"
3. Inject system message about JSON requirements
4. Continue to next attempt

## Provider Cycling

### Round-Robin Selection
**Location**: `src/ai-agent.ts:1484-1486`

```typescript
const pair = pairs[pairCursor % pairs.length];
pairCursor += 1;
const { provider, model } = pair;
```

**Behavior**:
- Cycle through targets array
- Wrap around when exhausted
- Each attempt increments cursor
- Continue until maxRetries exhausted

### Rate Limit Tracking
**Location**: `src/ai-agent.ts:1456-1457`

```typescript
let rateLimitedInCycle = 0;
let maxRateLimitWaitMs = 0;
```

**Behavior**:
- Track rate limits per cycle (full rotation through targets)
- If all providers rate-limited, apply max wait
- Reset counters after successful attempt

## Backoff Strategy

### Fallback Waits Per Attempt

```typescript
const RATE_LIMIT_MIN_WAIT_MS = 1_000;
const RATE_LIMIT_MAX_WAIT_MS = 60_000;
const fallbackWait = Math.min(Math.max(attempts * 1_000, RATE_LIMIT_MIN_WAIT_MS), RATE_LIMIT_MAX_WAIT_MS);
```

- `attempts` is the number of tries within the current turn.
- The loop clamps the fallback wait between 1s and 60s.

### Provider-Reported Delay / Retry-After

- Providers may return `retry.backoffMs` or `TurnStatus.rate_limit.retryAfterMs`.
- When present, those values override the fallback wait (still clamped to 1s–60s).
- The loop keeps the *maximum* suggested wait seen across the rotation in `maxRateLimitWaitMs`.

### Cycle-Level Sleep

- Sleeps only after a full rotation when **every** provider returned `rate_limit`.
- Logs `agent:retry` with the selected wait.
- Uses `sleepWithAbort(maxRateLimitWaitMs)` so `abortSignal`/`stopRef` can short-circuit before the next attempt.

## Final Turn Enforcement

### Context-Forced Final Turn
**Location**: `src/ai-agent.ts:1568-1597`

**Trigger**: Context guard detects token budget exceeded

**Behavior**:
1. Set `forcedFinalTurnReason = 'context'`
2. Inject instruction: "The conversation is at the context window limit..."
3. Restrict tools to only `agent__final_report`
4. Log warning about context enforcement
5. Retry if model doesn't call final_report

### Max Turns Final Turn
**Location**: `src/ai-agent.ts:1569-1574`

**Trigger**: `currentTurn === maxTurns`

**Behavior**:
1. Inject instruction: "Maximum number of turns/steps reached..."
2. Restrict tools to only `agent__final_report`
3. Log warning about final turn
4. Retry if model doesn't call final_report

### Final Report Attempt Tracking & Turn Collapse
**Location**: `src/ai-agent.ts:1671-1678` (flag setting), `src/ai-agent.ts:2213-2240` (collapse logic), `src/ai-agent.ts:3707` (sanitizer increment), `src/ai-agent.ts:3884` (executor increment)

- `finalReportAttempts` increments twice: when the sanitizer drops malformed `agent__final_report` calls and when the tool executor receives a legitimate call. Either event sets `finalReportAttempted = true` for the turn.
- If `incompleteFinalReportDetected === true` **or** the attempt flag is set, `maxTurns` collapses to `currentTurn + 1`. The orchestrator logs `agent:orchestrator` with the old/new limits so operators can trace why the session ended early.
- Every collapse emits `recordRetryCollapseMetrics({ reason, turn, previousMaxTurns, newMaxTurns })`, which backs the `ai_agent_retry_collapse_total` metric.

### Pending Final Report Cache
**Location**: `src/ai-agent.ts:1862-1990` (extraction/storage), `src/ai-agent.ts:2592-2599` (acceptance check), `src/ai-agent.ts:3421-3439` (commit/accept methods), `src/ai-agent.ts:3355-3370` (fallback logging)

- Text extraction and tool-message adoption no longer fabricate tool calls. Instead, valid payloads populate `pendingFinalReport = { source, payload }` and the retry loop proceeds as if no tool call existed.
- Cached payloads are only accepted via `acceptPendingFinalReport()` after the session enters the forced final turn (context guard or `currentTurn === maxTurns`). This guarantees retries always happen before fallbacks.
- Accepting a cache logs `agent:fallback-report` (including the source) and preserves `finalReportSource` so final telemetry/logs distinguish fallback exits from tool-call exits.

### Synthetic Failure Contract
**Location**: `src/ai-agent.ts:2593-2645`

- When max turns or context guard exhausts the run without a pending cache, the session synthesizes a failure final report, logs `agent:failure-report`, and commits it with metadata `{ reason, turns_completed, final_report_attempts, last_stop_reason }`.
- FIN + `agent:final-report-accepted` (with `source='synthetic'`) ensure headends render an explicit explanation instead of `(no output)`.
- `recordFinalReportMetrics` captures the synthetic reason so dashboards can alert whenever these spike.

### Telemetry Signals (Retry/Finalization)
- `ai_agent_final_report_total{source, status, forced_final_reason}` increments for every accepted final report. Alert when `source != 'tool-call'` or `status != 'success'` diverges from historical norms.
- `ai_agent_final_report_attempts_total` accumulates the session’s attempt count, helping spot regressions where the LLM repeatedly emits malformed payloads.
- `ai_agent_final_report_turns` histogram records the turn index that produced the final report, enabling SLA checks for prompt changes.
- `ai_agent_retry_collapse_total{reason}` counts how often the orchestrator shortens `maxTurns` so we can correlate collapses with prompt/policy updates.

## Conversation Mutation During Retry

### System Messages Injection
**Location**: `pushSystemRetryMessage()` helper

**Used for**:
- Final turn instructions
- Tool usage reminders
- JSON parameter guidance
- Final report reminders

## Business Logic Coverage (Verified 2025-11-16)

- **Cycle-scoped rate-limit waits**: `rateLimitedInCycle` counts per-target failures and waits only after *every* provider in the rotation returns `rate_limit`, preventing unnecessary sleeps when at least one path remains (`src/ai-agent.ts:1456-2511`).
- **Fallback retry directives**: When providers omit retry guidance, `buildFallbackRetryDirective` maps error types to deterministic actions (`retry`, `skip-provider`, `abort`) and optional backoff windows (`src/ai-agent.ts:3298-3338`).
- **Backoff cancellation**: All waits use `sleepWithAbort`, which returns `'aborted_stop'`/`'aborted_cancel'` markers so the loop can exit instead of sleeping when users stop sessions (`src/ai-agent.ts:2470-2545`).
- **Pending retry dedupe**: `pushSystemRetryMessage` ignores duplicate text, so repeated reminders (JSON schema, final report) appear once even when multiple providers fail in succession (`src/ai-agent.ts:2959-3042`).
- **Retry-after harmonization**: Provider metadata can supply `retryAfterMs`; the loop selects the maximum observed delay per cycle and logs it before waiting (`src/ai-agent.ts:2451-2493`).

**Messages are temporary**:
- Added to attempt conversation
- Not persisted to main conversation on retry
- Only persisted on success

### Metadata Tagging
```typescript
{
  role: 'user',
  content: 'System notice: ...',
  metadata: { retryMessage: 'retry-reason' }
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `maxRetries` | Global retry cap per turn (default: 3) |
| `maxTurns` | When to enforce final turn |
| `targets` | Provider/model cycling pool |
| `llmTimeout` | Request timeout threshold |

## Telemetry

**Per Retry Attempt**:
- Attempt number
- Provider/model used
- Status result
- Latency
- Error details
- Backoff duration

**Per Turn**:
- Total attempts
- Final status
- Time in retry loop

## Logging

**Retry Events**:
- `WRN`: "Synthetic retry: assistant returned content without tool calls"
- `WRN`: "Invalid tool call dropped due to malformed payload"
- `WRN`: "Final turn detected: restricting tools..."
- `WRN`: "Context guard enforced: restricting tools..."
- `ERR`: Auth/quota errors (fatal)

**Each Attempt**:
- Provider/model identifier
- Status result
- Token usage
- Cost information

## Invariants

1. **Retry cap honored**: Never exceed maxRetries per turn
2. **Provider cycling**: Round-robin through all targets
3. **Backoff respected**: Wait before retry when directed
4. **Fatal errors immediate**: No retry on auth_error/quota_exceeded
5. **Conversation integrity**: Failed attempts don't corrupt history
6. **Final turn enforcement**: Last turn restricted to final_report

## Undocumented Behaviors

1. **Empty response retry** (distinct from content-without-tools):
   - Triggers when response has no content AND no tools
   - Location: `src/ai-agent.ts:2280-2299`

2. **Final turn retry exhaustion fallback**:
   - Synthesizes failure report when final turn exhausts retries
   - Records last error in metadata
   - Location: `src/ai-agent.ts:2640-2665`

3. **Rate limit cycle backoff**:
   - Sleeps only after ALL providers in cycle are rate-limited
   - Uses `maxRateLimitWaitMs` across cycle
   - `rateLimitedInCycle` counter tracks this
   - Location: `src/ai-agent.ts:2487-2513`

4. **buildFallbackRetryDirective()**:
   - Generates retry directives when provider doesn't supply one
   - Includes exponential backoff calculation
   - Location: `src/ai-agent.ts:3298-3338`

5. **System message deduplication**:
   - `pendingRetryMessages` array accumulates messages
   - Deduplication via `pushSystemRetryMessage()` helper
   - Location: `src/ai-agent.ts:2956-2961`

6. **Invalid tool parameter retry**:
   - Logs "Invalid tool call dropped due to malformed payload"
   - Injects system message about JSON requirements
   - Continues to next attempt without adding invalid call
   - Location: `src/ai-agent.ts:1951-1970`

## Test Coverage

**Phase 1**:
- Retry on rate limit
- Retry on network error
- Synthetic retry on content-only
- Provider cycling
- Final turn enforcement
- Backoff delays

**Gaps**:
- Complex multi-cycle rate limiting
- Partial provider availability
- Retry message accumulation
- Empty response vs content-without-tools distinction

## Troubleshooting

### Retries exhausted quickly
- Check maxRetries setting
- Check provider error rates
- Verify targets array has multiple options

### Stuck in retry loop
- Check for synthetic retry conditions
- Verify model understands tool requirements
- Check final turn instructions

### Rate limit wait too long
- Check provider rate limit policies
- Verify backoff calculation
- Consider adding more providers to targets

### Auth error on specific provider
- Check API key validity
- Check provider configuration
- Verify credentials not expired

### Final turn not producing report
- Check final_report tool availability
- Verify instruction injection
- Check model comprehension
