# Retry Strategy

Error handling, provider cycling, backoff algorithms, and recovery mechanisms for resilient session execution.

---

## Table of Contents

- [TL;DR](#tldr) - Quick summary of retry behavior
- [Why This Matters](#why-this-matters) - When retry strategy affects you
- [Retry Architecture](#retry-architecture) - Core retry concepts
- [Error Classification](#error-classification) - How errors are handled
- [Provider Cycling](#provider-cycling) - Multi-provider failover
- [Backoff Strategy](#backoff-strategy) - Rate limit handling
- [Synthetic Retries](#synthetic-retries) - Model-level recovery
- [Final Turn Enforcement](#final-turn-enforcement) - Forced completion
- [Final Report Handling](#final-report-handling) - Report capture and fallback
- [Configuration](#configuration) - Settings that affect retries
- [Troubleshooting](#troubleshooting) - Common problems and solutions
- [See Also](#see-also) - Related documentation

---

## TL;DR

ai-agent implements per-turn retry with round-robin provider cycling, exponential backoff for rate limits, and synthetic retries for invalid LLM responses. When retries are exhausted or limits are reached, the session forces a final turn with tools restricted to `final_report` only.

---

## Why This Matters

Retry strategy affects you when:

- **Provider flaky**: Rate limits, temporary outages
- **Model misbehaves**: Empty responses, no tool calls
- **Session stuck**: Retries exhausted without progress
- **Unexpected completion**: Forced final turn triggered

Understanding retries helps you:

- Configure appropriate retry limits
- Set up effective provider fallbacks
- Debug "retry exhausted" failures
- Optimize for cost vs. reliability

---

## Retry Architecture

### Retry Directive

Each LLM turn result includes a retry directive.

```typescript
interface TurnRetryDirective {
  action: "retry" | "skip-provider" | "abort";
  backoffMs?: number; // Optional wait time
  logMessage?: string; // Human-readable reason
  sources?: string[]; // Error sources
}
```

### Retry Loop Structure

```mermaid
flowchart TD
    Start[Turn Start] --> Init[attempts = 0<br/>pairCursor = 0]
    Init --> Check{attempts < maxRetries<br/>AND not successful?}

    Check -->|Yes| Select[Select Provider<br/>targets[pairCursor % length]]
    Check -->|No| Exit[Exit Retry Loop]

    Select --> Increment[pairCursor++<br/>attempts++]
    Increment --> Execute[Execute LLM Turn]

    Execute --> Result{Result Status?}

    Result -->|Success + Tools| Done[turnSuccessful = true]
    Result -->|Success + Final| Done
    Result -->|Success, No Tools| Synthetic[Synthetic Retry]
    Result -->|Rate Limit| Backoff[Wait + Continue]
    Result -->|Auth Error| Abort[Abort Session]
    Result -->|Model Error| Skip[Skip to Next Provider]

    Synthetic --> Check
    Backoff --> Check
    Skip --> Check
    Done --> Exit
    Abort --> Exit
```

---

## Error Classification

### Retryable Errors

These errors trigger retry with backoff.

| Status             | Condition          | Backoff                                 | Behavior        |
| ------------------ | ------------------ | --------------------------------------- | --------------- |
| `rate_limit`       | Too many requests  | Yes (provider-specified or exponential) | Wait then retry |
| `network_error`    | Connection failed  | Yes (graduated, max 30s)                | Wait then retry |
| `timeout`          | Request timeout    | Yes (graduated, max 30s)                | Wait then retry |
| `invalid_response` | Malformed response | No                                      | Immediate retry |

---

## Provider Cycling

### Skip-Provider Errors

These errors skip the current provider but continue with others.

| Status           | Condition                         | Action                |
| ---------------- | --------------------------------- | --------------------- |
| `auth_error`     | Invalid API key                   | Skip to next provider |
| `quota_exceeded` | Account quota hit                 | Skip to next provider |
| `model_error`    | Model unavailable (non-retryable) | Skip to next provider |

Note: `model_error` with `retryable: true` continues with same provider (immediate retry).

### Round-Robin Selection

```mermaid
graph LR
    subgraph "Targets Array"
        T1[Target 0<br/>openai/gpt-4]
        T2[Target 1<br/>anthropic/claude-3]
        T3[Target 2<br/>openrouter/llama-3]
    end

    subgraph "Attempts"
        A1[Attempt 1 → Target 0]
        A2[Attempt 2 → Target 1]
        A3[Attempt 3 → Target 2]
        A4[Attempt 4 → Target 0]
    end

    T1 -.-> A1
    T2 -.-> A2
    T3 -.-> A3
    T1 -.-> A4
```

**Selection Formula**:

```
target = targets[pairCursor % targets.length]
pairCursor++
```

**Behavior**:

- Cycles through all configured targets
- Wraps around when array exhausted
- Each attempt increments cursor
- Continues until `maxRetries` exhausted

### Rate Limit Tracking

```typescript
let rateLimitedInCycle = 0;
let maxRateLimitWaitMs = 0;
```

| Scenario                   | Behavior               |
| -------------------------- | ---------------------- |
| One provider rate-limited  | Continue to next       |
| All providers rate-limited | Wait max observed time |
| After successful turn      | Reset counters         |

---

## Backoff Strategy

### Backoff Calculation

```mermaid
flowchart TD
    RateLimit[Rate Limit Hit] --> HasHeader{Provider Returned<br/>retry_after?}

    HasHeader -->|Yes| UseProvided[Use Provider Value<br/>Clamped 1s-60s]
    HasHeader -->|No| Calculate[Calculate Fallback]

    Calculate --> Formula[wait = min(attempts * 1000, 60000)<br/>Minimum 1000ms]

    UseProvided --> Track[Track Max Wait]
    Formula --> Track

    Track --> AllLimited{All Providers<br/>Rate Limited?}
    AllLimited -->|Yes| Sleep[Sleep Max Wait]
    AllLimited -->|No| Continue[Continue Immediately]

    Sleep --> NextAttempt[Next Attempt]
    Continue --> NextAttempt
```

### Backoff Values

| Constant                 | Value  | Purpose           |
| ------------------------ | ------ | ----------------- |
| `RATE_LIMIT_MIN_WAIT_MS` | 1,000  | Minimum wait time |
| `RATE_LIMIT_MAX_WAIT_MS` | 60,000 | Maximum wait time |

**Fallback Formula**:

```
fallbackWait = min(max(attempts * 1000, 1000), 60000)
```

### Sleep Behavior

- Uses `sleepWithAbort()` for cancellation support
- Sleeps only after full rotation when ALL providers rate-limited
- Logs `agent:retry` event with selected wait time

---

## Synthetic Retries

### Content Without Tools

**Trigger**: LLM returns text content but no tool calls and no `final_report`.

```mermaid
flowchart TD
    Response[LLM Response] --> HasTools{Has Tool Calls?}
    HasTools -->|Yes| Execute[Execute Tools]
    HasTools -->|No| HasFinal{Has Final Report?}

    HasFinal -->|Yes| Finalize[Finalize Session]
    HasFinal -->|No| Synthetic[Synthetic Retry]

    Synthetic --> Log[Log Warning]
    Log --> Guidance[Add TURN-FAILED Guidance]
    Guidance --> Continue[Continue to Next Attempt]
```

**Behavior**:

1. Log warning: "Synthetic retry: assistant returned content without tool calls"
2. Add TURN-FAILED guidance message to conversation
3. Increment attempt counter
4. Continue to next retry attempt

### Invalid Tool Parameters

**Trigger**: Tool call has malformed JSON parameters.

**Behavior**:

1. Preserve tool call with sanitization marker
2. Log ERR with full original payload
3. Return tool failure response: `(tool failed: invalid JSON parameters)`
4. Continue to next attempt within same turn (no TURN-FAILED added)

---

## Final Turn Enforcement

### Context-Forced Final Turn

**Trigger**: Context guard detects token budget exceeded.

```mermaid
stateDiagram-v2
    [*] --> Normal: Turn executing
    Normal --> ContextExceeded: projected > limit
    ContextExceeded --> ForcedFinal: Set reason='context'
    ForcedFinal --> Restricted: Tools = [final_report]
    Restricted --> Instruction: Inject guidance
    Instruction --> Execute: Continue turn
    Execute --> Retry: Model doesn't call final_report
    Retry --> Execute: Retry with guidance
    Execute --> Complete: final_report called
    Complete --> [*]
```

**Actions**:

1. Set `forcedFinalTurnReason = 'context'`
2. Inject instruction message
3. Restrict tools to `agent__final_report`
4. Log warning
5. Retry if model doesn't call `final_report`

### Max Turns Final Turn

**Trigger**: `currentTurn === maxTurns`

**Actions**:

1. Set `forcedFinalTurnReason = 'maxTurns'`
2. Inject instruction message
3. Restrict tools to `agent__final_report`
4. Log warning
5. Retry if model doesn't call `final_report`

---

## Final Report Handling

### Attempt Tracking

`finalReportAttempts` increments when:

- Sanitizer drops malformed `final_report` calls
- Tool executor receives legitimate `final_report` call

### Turn Collapse

When final report is attempted or incomplete detection triggers:

| Step | Action                                       |
| ---- | -------------------------------------------- |
| 1    | Set `maxTurns = currentTurn + 1`             |
| 2    | Log `agent:orchestrator` with old/new limits |
| 3    | Emit `ai_agent_retry_collapse_total` metric  |

### Pending Cache

```mermaid
flowchart TD
    FinalReport[final_report Call] --> Validate{Valid Payload?}

    Validate -->|Yes| Cache[Store in pendingFinalReport]
    Validate -->|No| Reject[Reject + Retry]

    Cache --> Forced{Forced Final Turn?}
    Forced -->|Yes| Accept[Accept Report]
    Forced -->|No| Continue[Continue Session<br/>Cache Preserved]

    Accept --> Finalize[Finalize Session]
```

**Purpose**: Ensures retries happen before fallbacks.

### Synthetic Failure

When exhausted without valid final report:

1. Synthesize failure report
2. Log `agent:failure-report`
3. Set metadata with reason and context
4. Return with `success: false`

---

## Configuration

### Key Settings

| Setting      | Type   | Default | Effect                                      |
| ------------ | ------ | ------- | ------------------------------------------- |
| `maxRetries` | number | 3       | Total attempts per turn (initial + retries) |
| `maxTurns`   | number | 10      | When to enforce final turn                  |
| `targets`    | array  | -       | Provider/model cycling pool                 |
| `llmTimeout` | number | 600000  | Request timeout threshold (10 minutes)      |

### Retry vs. Attempts

> **Note**: `maxRetries: 3` means **3 total attempts**, not "initial + 3 retries".

| maxRetries | Total Attempts | Retries After Initial |
| ---------- | -------------- | --------------------- |
| 1          | 1              | 0                     |
| 2          | 2              | 1                     |
| 3          | 3              | 2                     |

### Example Configuration

```yaml
---
maxRetries: 5
maxTurns: 30
models:
  - openai/gpt-4-turbo
  - anthropic/claude-3-sonnet
  - openrouter/meta-llama/llama-3-70b
---
```

---

## Telemetry

### Per Retry Attempt

| Metric         | Description                        |
| -------------- | ---------------------------------- |
| `attempt`      | Attempt number (1-based)           |
| `provider`     | Provider name                      |
| `model`        | Model name                         |
| `status`       | Result status                      |
| `latencyMs`    | Request latency                    |
| `errorDetails` | Error information (if failed)      |
| `backoffMs`    | Backoff duration (if rate-limited) |

### Per Turn

| Metric                | Description             |
| --------------------- | ----------------------- |
| `totalAttempts`       | Attempts used this turn |
| `finalStatus`         | Turn outcome            |
| `retryLoopDurationMs` | Time in retry loop      |

---

## Troubleshooting

### Retries Exhausted Quickly

**Symptom**: Session fails after few attempts.

**Causes**:

- `maxRetries` too low
- All providers returning same error
- Only one target configured

**Solutions**:

1. Increase `maxRetries` (consider cost implications)
2. Add more diverse providers to `targets`
3. Check provider error logs for patterns

### Stuck in Retry Loop

**Symptom**: Many retries but no progress.

**Causes**:

- Model consistently returning content without tools
- Final turn instructions unclear
- Tool schema issues

**Solutions**:

1. Review system prompt for clear tool usage instructions
2. Check model's tool-calling capabilities
3. Simplify available tools
4. Try different model

### Rate Limit Wait Too Long

**Symptom**: Session pauses for extended periods.

**Causes**:

- Provider rate limits aggressive
- Single provider in targets
- High request volume

**Solutions**:

1. Add more providers to distribute load
2. Implement request queuing
3. Upgrade provider plan
4. Reduce session concurrency

### Unexpected Synthetic Retry

**Symptom**: "Synthetic retry" warnings when expecting response.

**Causes**:

- Model misunderstanding task
- Missing tool schemas
- Prompt unclear about tool requirement

**Solutions**:

1. Review prompt for clarity
2. Verify tools are correctly registered
3. Check for tool schema issues
4. Try more capable model

---

## Invariants

These rules MUST hold:

1. **Retry cap honored**: Never exceed `maxRetries` per turn
2. **Provider cycling**: Round-robin through all targets
3. **Backoff respected**: Wait before retry when directed
4. **Skip-provider errors**: `auth_error`/`quota_exceeded`/non-retryable `model_error` skip to next provider
5. **Conversation integrity**: Failed attempts don't corrupt history
6. **Final turn enforcement**: Last turn restricted to `final_report`

---

## See Also

- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - Turn execution flow
- [Context Management](Technical-Specs-Context-Management) - Context-forced final turn
- [User Contract](Technical-Specs-User-Contract) - Guaranteed behaviors
- [specs/retry-strategy.md](specs/retry-strategy.md) - Full specification
