# User Contract

End-user guarantees that MUST hold regardless of implementation details. These are the promises ai-agent makes to its users.

---

## Table of Contents

- [TL;DR](#tldr) - Quick summary of guaranteed behaviors
- [Why This Matters](#why-this-matters) - What the contract means for you
- [Turn and Tool Limits](#turn-and-tool-limits) - Guaranteed limit enforcement
- [Context Window Management](#context-window-management) - Token handling guarantees
- [Tool Response Handling](#tool-response-handling) - Tool output guarantees
- [Output Guarantees](#output-guarantees) - Session always produces output
- [Error Handling Contract](#error-handling-contract) - Error behavior guarantees
- [Final Report Contract](#final-report-contract) - Completion guarantees
- [Accounting Contract](#accounting-contract) - Cost tracking guarantees
- [Conversation Contract](#conversation-contract) - Message ordering guarantees
- [Configuration Contract](#configuration-contract) - Config loading guarantees
- [Contract Violations](#contract-violations) - What counts as a bug
- [See Also](#see-also) - Related documentation

---

## TL;DR

ai-agent guarantees: sessions never exceed `maxTurns`, context overflow is prevented before API errors, every session produces output (success or synthesized failure), and tool responses are handled consistently. These guarantees hold regardless of implementation changes.

---

## Why This Matters

The user contract defines what you can **rely on**:

- **Building integrations**: Know exactly what outputs to expect
- **Setting limits**: Trust that limits are enforced
- **Error handling**: Understand error behavior patterns
- **Cost control**: Rely on accounting records
- **Debugging**: Know what's guaranteed vs. implementation detail

**Contract vs. Implementation**:

- **Contract**: "Session never exceeds maxTurns" - guaranteed
- **Implementation**: "Retries use round-robin" - may change

---

## Turn and Tool Limits

### maxTurns Contract

| Guarantee                                          | Details                      |
| -------------------------------------------------- | ---------------------------- |
| Session NEVER exceeds `maxTurns`                   | Absolute limit on LLM turns  |
| 1 turn = 1 LLM request/response + tool phase       | Definition of a turn         |
| Retries don't count toward limit                   | Retries happen within a turn |
| Max turns without final_report → synthetic failure | Always produces output       |

**Behavior**:

```
Turn 1: LLM → Tools → (retry if needed)
Turn 2: LLM → Tools
...
Turn N (maxTurns): LLM → final_report ONLY
```

### maxToolCallsPerTurn Contract

| Guarantee                       | Details           |
| ------------------------------- | ----------------- |
| Excess tool calls are dropped   | Never executed    |
| LLM receives error about excess | Informed of limit |
| Valid calls still executed      | Up to limit       |

### maxRetries Contract

| Guarantee                                        | Details                                                                 |
| ------------------------------------------------ | ----------------------------------------------------------------------- |
| `maxRetries` = total attempts per turn           | Initial + retries                                                       |
| `maxRetries: 1` = 1 attempt                      | No retries                                                              |
| `maxRetries: 3` = 3 attempts                     | 2 retries                                                               |
| Retry loop continues until success or exhaustion | Cycle through targets round-robin until `maxRetries` attempts completed |

---

## Context Window Management

### Token Counter Guarantees

| Counter            | Guarantee                                  |
| ------------------ | ------------------------------------------ |
| `currentCtxTokens` | Committed conversation tokens              |
| `pendingCtxTokens` | Uncommitted tokens (tool outputs, notices) |
| `schemaCtxTokens`  | Tool schema overhead                       |
| `contextWindow`    | Model capacity                             |

### LLM Request Metrics

Every `LLM request prepared` log satisfies:

```
expectedTokens = ctxTokens + newTokens + schemaCtxTokens
```

**Note:** Schema tokens are tracked separately (`schemaCtxTokens`) and added to context via cache_write (first turn) and cache_read (subsequent turns). They are included in the `ctxTokens` value reported by providers (input + output + cache_read + cache_write), so `expectedTokens = ctxTokens + newTokens + schemaCtxTokens` accurately reflects total context.

### Guard Projection

```

projected = currentCtxTokens + pendingCtxTokens + newCtxTokens + schemaCtxTokens
limit = contextWindow - contextWindowBufferTokens - maxOutputTokens

```

### Enforcement Guarantees

| Condition               | Guarantee                             |
| ----------------------- | ------------------------------------- |
| `projected > limit`     | Forced final turn                     |
| Forced final turn       | Tools restricted to `final_report`    |
| Still over after shrink | Proceed best-effort (may fail at API) |

---

## Tool Response Handling

### toolResponseMaxBytes Contract

| Guarantee                           | Details                                                                                      |
| ----------------------------------- | -------------------------------------------------------------------------------------------- |
| Responses exceeding size are stored | Written to disk                                                                              |
| Replaced with `tool_output` handle  | LLM receives handle                                                                          |
| Handle includes metadata            | `handle`, `reason`, `bytes`, `lines`, `tokens`                                               |
| Original preserved                  | Stored under per-run directory (e.g., `/tmp/ai-agent-<run-hash>/session-<uuid>/<file-uuid>`) |

### toolTimeout Contract

| Guarantee                       | Details                  |
| ------------------------------- | ------------------------ |
| Tool aborted after timeout      | Execution stops          |
| Timed-out tools marked `failed` | Status recorded          |
| LLM receives failure message    | `(tool failed: timeout)` |

---

## Output Guarantees

### Always Return Output

**Guarantee**: ai-agent NEVER crashes without returning `AIAgentResult`.

| Scenario                       | Result                                     |
| ------------------------------ | ------------------------------------------ |
| Model provides final_report    | `success: true`, `finalReport` populated   |
| Max turns without final_report | `success: false`, synthetic failure report |
| Fatal error                    | `success: false`, `error` field populated  |

### CLI Exit Codes

| Code | Meaning         | When                               |
| ---- | --------------- | ---------------------------------- |
| 0    | Success         | Session completed successfully     |
| 1    | Generic failure | Session failed                     |
| 3    | MCP failure     | Missing/failed MCP servers         |
| 4    | Invalid config  | Invalid arguments or configuration |
| 5    | Schema error    | Schema validation failures         |

### Empty/Invalid Response Handling

**Empty content without tool calls**:

| Condition              | Behavior                                   |
| ---------------------- | ------------------------------------------ |
| Only reasoning present | Preserve reasoning, retry with guidance    |
| Nothing present        | NOT added to conversation, synthetic retry |
| Counts as attempt      | Toward `maxRetries`                        |

**Invalid tool parameters**:

| Condition           | Behavior                 |
| ------------------- | ------------------------ |
| Malformed JSON      | Call dropped             |
| Valid calls present | Still executed           |
| LLM receives error  | Validation error message |

---

## Error Handling Contract

### Fatal Errors (No Retry)

| Error          | Guarantee                 |
| -------------- | ------------------------- |
| Auth errors    | Session fails immediately |
| Quota exceeded | Session fails immediately |

### Retryable Errors

| Error             | Guarantee                              |
| ----------------- | -------------------------------------- |
| Rate limits       | Honor Retry-After, exponential backoff |
| Network/timeout   | Retry with next target immediately     |
| Invalid responses | Synthetic retry with error message     |

### Provider Cycling

Provider/model cycling occurs on every LLM attempt (including retries):

```

Attempt 1: targets[0]
Attempt 2: targets[1]
Attempt N: targets[(N-1) % targets.length]
```

**Cycle continues until:**

- Success (tool calls or final_report received)
- `maxRetries` exhausted across all targets
- Fatal error encountered

```

---

## Final Report Contract

### Every Session Produces EITHER

1. **Tool-provided**: LLM calls `final_report` successfully
2. **Text extraction**: Final turn text without `final_report` call
3. **Synthetic failure**: Max turns exhausted without valid report

### Final Report Structure

**Fields in FinalReportPayload**:

| Field          | Type                                                                                                                        | When Present                    |
| -------------- | --------------------------------------------------------------------------------------------------------------------------- | ------------------------------- |
| `format`       | `'json'` \| `'markdown'` \| `'markdown+mermaid'` \| `'slack-block-kit'` \| `'tty'` \| `'pipe'` \| `'sub-agent'` \| `'text'` | Always present                  |
| `content_json` | object                                                                                                                      | JSON format                     |
| `content`      | string                                                                                                                      | Text formats                    |
| `metadata`     | object                                                                                                                      | Optional                        |
| `ts`           | number                                                                                                                      | Unix timestamp (always present) |

**Session-level status**: The `success` field in `AIAgentResult` indicates overall session success/failure. This is distinct from the final report payload itself.

---

## Accounting Contract

### LLM Entry Guarantees

Every LLM request produces an accounting entry with:

| Field                          | Guarantee                   |
| ------------------------------ | --------------------------- |
| `type`                         | `'llm'`                     |
| `provider`, `model`            | Actual provider/model used  |
| `status`                       | `'ok'` or `'failed'`        |
| `latency`                      | Request duration            |
| `tokens`                       | Token counts (if available) |
| `timestamp`                    | When request was made       |
| `agentId`, `callPath`, `txnId` | Tracing context             |

### Tool Entry Guarantees

Every tool execution produces an accounting entry with:

| Field                           | Guarantee            |
| ------------------------------- | -------------------- |
| `type`                          | `'tool'`             |
| `mcpServer`, `command`          | Tool identification  |
| `status`                        | `'ok'` or `'failed'` |
| `latency`                       | Execution duration   |
| `timestamp`                     | When executed        |
| `charactersIn`, `charactersOut` | I/O sizes            |

### Observable Guarantees

| Guarantee                 | Details                         |
| ------------------------- | ------------------------------- |
| Provider fallback visible | Sequence shows provider changes |
| All tools recorded        | Success and failure             |
| Timing available          | Latency for all operations      |

---

## Conversation Contract

### Message Order Guarantees

1. System messages (initial)
2. User message (task prompt)
3. Alternating: assistant → tool → assistant → tool
4. Final: assistant with `final_report` or text

### Tool Message Formats

| Outcome          | Message Format                                  |
| ---------------- | ----------------------------------------------- |
| Success          | `role: 'tool'` with content                     |
| Failed           | `(tool failed: <reason>)`                       |
| Dropped (budget) | `(tool failed: context window budget exceeded)` |

---

## Configuration Contract

### Layer Precedence (Highest to Lowest)

| Priority | Source                          |
| -------- | ------------------------------- |
| 1        | `--config` explicit path        |
| 2        | Current working directory       |
| 3        | Prompt directory                |
| 4        | Binary directory                |
| 5        | `$HOME/.ai-agent/ai-agent.json` |
| 6        | `/etc/ai-agent/ai-agent.json`   |

### Placeholder Expansion

| Guarantee                     | Details                                 |
| ----------------------------- | --------------------------------------- |
| `${VARIABLE}` resolved        | From `.ai-agent.env` then `process.env` |
| Unresolved throws             | With layer origin information           |
| `${parameters.foo}` protected | Not expanded in REST tool URLs          |

---

## Contract Violations

**Any deviation from these contracts is a critical bug.**

### Examples of Violations

| Violation            | Description                                  |
| -------------------- | -------------------------------------------- |
| Turn limit exceeded  | 4 assistant messages when `maxTurns: 3`      |
| Context overflow     | Provider rejects due to context overflow     |
| Missing final report | `success: true` but `finalReport` undefined  |
| Wrong exit code      | CLI exits 0 when `success: false`            |
| Config ignored       | `temperature: 0.5` but provider receives 1.0 |
| Accounting missing   | LLM request without accounting entry         |

### Non-Violations (Implementation Details)

These may change without notice:

| Allowed Change        | Description                 |
| --------------------- | --------------------------- |
| Log message wording   | Internal log text           |
| Internal state names  | State refactoring           |
| Estimation algorithms | Token counting improvements |
| Retry timing          | Backoff adjustments         |

---

## Testing Contract Compliance

### Verification Approach

For each contract guarantee:

1. **Unit test**: Verify specific behavior
2. **Integration test**: End-to-end scenario
3. **Chaos test**: Behavior under failures

### Key Test Scenarios

| Scenario           | Contract Tested        |
| ------------------ | ---------------------- |
| Run to maxTurns    | Turn limit enforcement |
| Large context      | Context guard trigger  |
| Large tool output  | Size limit handling    |
| Provider failure   | Retry and cycling      |
| All providers fail | Error handling         |

---

## See Also

- [Architecture](Technical-Specs-Architecture) - System architecture
- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - Session execution flow
- [Retry Strategy](Technical-Specs-Retry-Strategy) - Error handling details
- [Context Management](Technical-Specs-Context-Management) - Token budget details
- [specs/CONTRACT.md](specs/CONTRACT.md) - Full contract specification

```

```

```
