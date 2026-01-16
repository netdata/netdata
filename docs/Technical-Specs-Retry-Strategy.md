# Retry Strategy

Error handling, provider cycling, and recovery mechanisms.

---

## TL;DR

Per-turn retry with provider cycling, exponential backoff for rate limits, synthetic retries for invalid responses, context-aware final turn enforcement.

---

## Retry Architecture

### Retry Directive

```typescript
interface TurnRetryDirective {
  action: 'retry' | 'skip-provider' | 'abort';
  backoffMs?: number;
  logMessage?: string;
  sources?: string[];
}
```

### Retry Loop Structure

```
for turn = 1 to maxTurns:
  attempts = 0
  pairCursor = 0

  while attempts < maxRetries and not successful:
    # Reset per-attempt error state
    lastError = undefined

    pair = targets[pairCursor % targets.length]
    pairCursor += 1
    attempts += 1

    result = executeSingleTurn(pair.provider, pair.model)

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

---

## Error Classification

### Fatal Errors (No Retry)

| Status | Condition | Exit Code |
|--------|-----------|-----------|
| `auth_error` | Invalid API key | EXIT-AUTH-FAILURE |
| `quota_exceeded` | Account quota hit | EXIT-QUOTA-EXCEEDED |

### Retryable Errors

| Status | Condition | Backoff |
|--------|-----------|---------|
| `rate_limit` | Too many requests | retryAfterMs or exponential |
| `model_error` | Model unavailable | None |
| `network_error` | Connection failed | None |
| `timeout` | Request timeout | None |
| `invalid_response` | Malformed response | None |

### Skip-Provider Errors

| Condition | Action |
|-----------|--------|
| Model not supported | Skip to next target |
| Provider temporary issue | Skip to next target |

---

## Synthetic Retries

### Content Without Tools

**Trigger**: LLM returns text but no tool calls and no final_report

**Behavior**:
1. Log warning
2. Add TURN-FAILED guidance
3. Continue to next attempt

### Invalid Tool Parameters

**Trigger**: Tool call has malformed JSON parameters

**Behavior**:
1. Preserve tool call with sanitization marker
2. Log ERR with full original payload
3. Return tool failure response
4. Continue to next attempt (no TURN-FAILED)

---

## Provider Cycling

### Round-Robin Selection

```typescript
const pair = pairs[pairCursor % pairs.length];
pairCursor += 1;
```

**Behavior**:
- Cycle through targets array
- Wrap around when exhausted
- Each attempt increments cursor
- Continue until maxRetries exhausted

### Rate Limit Tracking

```typescript
let rateLimitedInCycle = 0;
let maxRateLimitWaitMs = 0;
```

- Track rate limits per cycle
- If all providers rate-limited → apply max wait
- Reset counters after success

---

## Backoff Strategy

### Fallback Waits

```typescript
const RATE_LIMIT_MIN_WAIT_MS = 1_000;
const RATE_LIMIT_MAX_WAIT_MS = 60_000;
const fallbackWait = Math.min(
  Math.max(attempts * 1_000, RATE_LIMIT_MIN_WAIT_MS),
  RATE_LIMIT_MAX_WAIT_MS
);
```

### Provider-Reported Delay

- Providers may return `retry.backoffMs`
- Overrides fallback wait (clamped 1s-60s)
- Loop keeps maximum observed wait

### Cycle-Level Sleep

- Sleeps only after full rotation when ALL providers rate-limited
- Uses `sleepWithAbort()` for cancellation
- Logs `agent:retry` with selected wait

---

## Final Turn Enforcement

### Context-Forced Final Turn

**Trigger**: Context guard detects token budget exceeded

**Actions**:
1. Set `forcedFinalTurnReason = 'context'`
2. Inject instruction message
3. Restrict tools to `agent__final_report`
4. Log warning
5. Retry if model doesn't call final_report

### Max Turns Final Turn

**Trigger**: `currentTurn === maxTurns`

**Actions**:
1. Inject instruction message
2. Restrict tools to `agent__final_report`
3. Log warning
4. Retry if model doesn't call final_report

---

## Final Report Handling

### Attempt Tracking

- `finalReportAttempts` increments when:
  - Sanitizer drops malformed calls
  - Tool executor receives legitimate call

### Turn Collapse

When final report attempted or incomplete detected:
- `maxTurns` collapses to `currentTurn + 1`
- Logs `agent:orchestrator` with old/new limits
- Emits `ai_agent_retry_collapse_total` metric

### Pending Cache

- Valid payloads stored in `pendingFinalReport`
- Accepted only after forced final turn
- Guarantees retries happen before fallbacks

### Synthetic Failure

When exhausted without pending cache:
- Synthesizes failure report
- Logs `agent:failure-report`
- Sets metadata with reason and context

---

## Conversation Mutation

- Retry guidance persisted as TURN-FAILED user messages
- Provider throttling/transport errors NOT surfaced to model
- Malformed tool payloads returned as tool failures

---

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `maxRetries` | Global retry cap per turn (default: 3) |
| `maxTurns` | When to enforce final turn |
| `targets` | Provider/model cycling pool |
| `llmTimeout` | Request timeout threshold |

---

## Telemetry

### Per Retry Attempt
- Attempt number
- Provider/model used
- Status result
- Latency
- Error details
- Backoff duration

### Per Turn
- Total attempts
- Final status
- Time in retry loop

---

## Key Log Events

- `WRN`: "Synthetic retry: assistant returned content without tool calls"
- `WRN`: "Invalid tool call dropped due to malformed payload"
- `WRN`: "Final turn detected: restricting tools..."
- `WRN`: "Context guard enforced: restricting tools..."
- `ERR`: Auth/quota errors (fatal)

---

## Invariants

1. **Retry cap honored**: Never exceed maxRetries per turn
2. **Provider cycling**: Round-robin through all targets
3. **Backoff respected**: Wait before retry when directed
4. **Fatal errors immediate**: No retry on auth_error/quota_exceeded
5. **Conversation integrity**: Failed attempts don't corrupt history
6. **Final turn enforcement**: Last turn restricted to final_report

---

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
- Consider adding more providers

---

## See Also

- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - Turn execution
- [docs/specs/retry-strategy.md](../docs/specs/retry-strategy.md) - Full spec

