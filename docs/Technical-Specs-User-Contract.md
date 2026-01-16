# User Contract

End-user guarantees that MUST hold regardless of implementation.

---

## Overview

This document defines the **end-user contract** for ai-agent: guarantees that MUST hold under ALL conditions. Every condition in specs maps to a contract-level guarantee here.

---

## 1. Turn and Tool Limits

### maxTurns

- Application will **NEVER** exceed this number of turns
- 1 turn = 1 LLM request/response + tool execution phase
- Retries within same turn do NOT count toward limit
- When limit reached without final_report → synthetic failure generated

### maxToolCallsPerTurn

- LLM cannot request more tools than this limit per turn
- Excess tool calls are dropped (NOT executed)
- LLM receives error message about excess

### maxRetries

- Maximum total LLM attempts per turn (initial + retries)
- `maxRetries: 1` = 1 attempt (no retries)
- `maxRetries: 3` = 3 attempts (initial + 2 retries)
- Attempts cycle through all provider/model targets round-robin

---

## 2. Context Window Management

### Token Counters

- `currentCtxTokens`: Committed to conversation
- `pendingCtxTokens`: Pending commit (tool outputs, retry notices)
- `schemaCtxTokens`: Tool schema overhead
- `contextWindow`: Provider/model capacity

### LLM Request Metrics

Every `LLM request prepared` log MUST satisfy:
```
expectedTokens = ctxTokens + pendingCtxTokens + schemaCtxTokens
```

### Guard Projection

```
projected = currentCtxTokens + pendingCtxTokens + newCtxTokens + schemaCtxTokens
limit = contextWindow - contextWindowBufferTokens - maxOutputTokens
```

### Enforcement

- When `projected > limit` → forced final turn
- Tools restricted to `final_report` only
- Session proceeds best-effort if still over after shrink

---

## 3. Tool Response Handling

### toolResponseMaxBytes

- Responses exceeding size are **stored** on disk
- Replaced with `tool_output` handle message
- Handle includes: `handle`, `reason`, `bytes`, `lines`, `tokens`
- Original preserved under `/tmp/ai-agent-<run-hash>/`

### toolTimeout

- Tool execution aborted after this duration
- Timed-out tools marked `status: 'failed'`
- LLM receives: `(tool failed: timeout)`

---

## 4. Core Invariants

### Always Return Output

**Application NEVER crashes without returning `AIAgentResult`**:

- Model-provided report: `success: true`, `finalReport` populated
- Synthetic failure: `success: false`, `finalReport` with failure content
- Fatal errors: `success: false`, `error` field populated

### CLI Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success or informational output |
| 1 | Generic session failure |
| 3 | Missing/failed MCP servers |
| 4 | Invalid arguments or config |
| 5 | Schema validation failures |

### Empty or Invalid Responses

**Empty content without tool calls**:
- If only reasoning → preserve reasoning, retry with guidance
- If nothing → NOT added to conversation, synthetic retry
- Counts as one attempt toward `maxRetries`

**Invalid tool parameters**:
- Malformed calls dropped
- LLM receives validation error
- Valid calls still executed

---

## 5. Error Handling

### Fatal Errors (No Retry)

| Error | Behavior |
|-------|----------|
| Auth errors | Session fails immediately |
| Quota exceeded | Session fails immediately |

### Retryable Errors

| Error | Behavior |
|-------|----------|
| Rate limits | Honor Retry-After, exponential backoff |
| Network/timeout | Retry with next target immediately |
| Invalid responses | Synthetic retry with error message |

### Provider Cycling

```
Attempt 1: targets[0]
Attempt 2: targets[1]
Attempt N: targets[(N-1) % targets.length]
```

---

## 6. Final Report Contract

### Every Session Produces EITHER

1. **Tool-provided**: LLM calls `final_report` successfully
2. **Text extraction fallback**: Final turn text without `final_report`
3. **Synthetic failure**: Max turns exhausted

### Final Report Structure

**System-determined**:
- `status`: `'success'` | `'failure'` (by source, not model)
- `format`: Must match session expectation
- `ts`: Timestamp

**Model-provided**:
- `content_json`: For JSON format
- `content`: For text formats
- `metadata`: Optional

---

## 7. Accounting Contract

### LLM Entries

Every LLM request produces:
- `type`: `'llm'`
- `provider`, `model`
- `status`: `'ok'` | `'failed'`
- `latency`, `tokens`, `timestamp`
- Tracing: `agentId`, `callPath`, `txnId`

### Tool Entries

Every tool execution produces:
- `type`: `'tool'`
- `mcpServer`, `command`
- `status`: `'ok'` | `'failed'`
- `latency`, `timestamp`
- `charactersIn`, `charactersOut`

### Guarantees

- Provider fallback observable in accounting sequence
- All tool executions appear (success or failure)
- Timing available for performance analysis

---

## 8. Conversation Contract

### Message Order

1. System messages (initial)
2. User message (task prompt)
3. Alternating: assistant → tool → assistant → tool
4. Final: assistant with `final_report` or text

### Tool Messages

- Successful: `role: 'tool'` with content
- Failed: `(tool failed: <reason>)`
- Dropped: `(tool failed: context window budget exceeded)`

---

## 9. Configuration Loading

### Layer Precedence (highest to lowest)

1. `--config` explicit path
2. Current working directory
3. Prompt directory
4. Binary directory
5. `$HOME/.ai-agent/ai-agent.json`
6. `/etc/ai-agent/ai-agent.json`

### Placeholder Expansion

- `${VARIABLE}` resolves from `.ai-agent.env` then `process.env`
- Throws if unresolved (with layer origin)
- Protected: `${parameters.foo}` in REST tools not expanded

---

## 10. Contract Violations

Any deviation is a **critical bug**.

### Examples of Violations

| Violation | Description |
|-----------|-------------|
| Turn limit exceeded | 4 assistant messages when `maxTurns: 3` |
| Context window exceeded | Provider rejects due to overflow |
| Missing final report | `success: true` but `finalReport` undefined |
| Wrong exit code | CLI exits 0 when `success: false` |
| Config ignored | `temperature: 0.5` but provider receives 1.0 |

### Non-Violations (Implementation Details)

| Allowed Change | Description |
|----------------|-------------|
| Log message wording | Internal logs can change |
| State refactoring | Internal state can change |
| Estimation improvement | Token estimation can improve |

---

## See Also

- [Architecture](Technical-Specs-Architecture) - System architecture
- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - Session flow
- [docs/CONTRACT.md](../docs/CONTRACT.md) - Full contract

