# AI Agent Contract

**Version**: 1.0
**Last Updated**: 2025-11-19

This document defines the **end-user contract** for ai-agent: the guarantees that MUST hold regardless of implementation details. Every condition/rule in `docs/specs/*.md` maps to a contract-level guarantee here.

---

## 1. Configuration Guarantees

Every user-configurable setting has a guarantee that MUST be respected under ALL conditions.

### Turn and Tool Limits

**`maxTurns`**
- Application will NEVER exceed this number of conversation turns
- 1 turn = 1 LLM request/response cycle + tool execution phase
- Retries within the same turn do NOT count toward the limit
- When limit is reached WITHOUT a final_report → synthetic failure report generated

**`maxToolCallsPerTurn`**
- LLM cannot request more tools than this limit in a single turn
- Excess tool calls are dropped and NOT executed
- LLM receives error message about excess calls

**`maxRetries`**
- Maximum total LLM attempts per turn (includes initial request plus any retries)
- `maxRetries: 1` allows 1 attempt (no retries), `maxRetries: 3` allows 3 attempts (initial + 2 retries)
- Attempts cycle through all configured provider/model targets round-robin
- After exhausting all attempts → turn fails, session continues to next turn or exits

### Context Window Management

**`contextWindow` (per provider/model)**
- Conversation + tool schemas + pending outputs NEVER exceed this limit
- Context guard evaluates: `currentTokens + pendingTokens + schemaTokens + maxOutputTokens + bufferTokens ≤ contextWindow`
- When limit approached → forced final turn (tools restricted to `final_report` only)

**`contextWindowBufferTokens` (per provider/model)**
- Safety margin subtracted from context window
- Prevents provider rejections due to estimation drift

**`maxOutputTokens`**
- Requested token budget for LLM responses
- Passed to provider in every request
- Included in context guard calculations

### Tool Response Handling

**`toolResponseMaxBytes`**
- Tool responses exceeding this size are truncated
- Truncated responses include notice: `[TRUNCATED] Original size X bytes; truncated to Y bytes.` (prepended to content)
- Warning logged with metadata: tool name, actual bytes, limit bytes
- Original response only preserved in logs when `traceTools` mode enabled

**`toolTimeout`**
- Tool execution aborted after this duration (milliseconds)
- Timed-out tools marked as `status: 'failed'` in accounting
- LLM receives failure message: `(tool failed: timeout)`

### Provider Configuration

**`providers` (target list)**
- Requests cycle through targets in order when retries occur
- Each target has: provider name, model name, optional overrides

**Temperature, topP, repeatPenalty, etc.**
- Passed to provider on every LLM request
- Session-level defaults can be overridden per-provider or per-model

**Reasoning toggles**
- `extended_thinking`, `reasoning_effort` settings honored when provider supports them
- Passed to provider API when enabled

### MCP Tool Queues

**`queues.<name>.concurrent`**
- Maximum concurrent executions for tools bound to this queue
- Additional requests wait until slot available
- Queue depth tracked in telemetry

---

## 2. Core Invariants

These rules MUST hold in all cases, regardless of configuration.

### Always Return Output

**The application NEVER crashes without returning `AIAgentResult`**
- Success cases: `success: true`, `finalReport` with `status: 'success'`
- Failure cases: `success: false`, `finalReport` with `status: 'failure'` or `'partial'`
- Max turns exhausted: synthetic failure report with `metadata.reason: 'max_turns_exhausted'`
- Fatal errors (auth, quota): `success: false`, `error` field populated

### CLI Exit Codes

**Exit code 0**: Session succeeded or informational output completed
**Exit code 1**: Generic session failure or internal error
**Exit code 3**: Missing/failed MCP servers or initialization failures
**Exit code 4**: Invalid arguments, configuration errors, or validation failures
**Exit code 5**: Schema validation failures (e.g., tool schema validation)

Note: Exit codes map to failure categories, not specific error types. Check `result.error` and `result.finalReport.status` for detailed failure reasons.

### Context Guard Enforcement

**When context budget exceeded:**
1. **Tool responses** exceeding remaining budget are dropped
2. Dropped responses replaced with failure message visible to LLM: `(tool failed: context window budget exceeded)`
3. **Forced final turn** activated: only `final_report` tool available
4. Session continues with restricted tools until final turn completes

**Context guard triggers:**
- **Pre-turn preflight**: Before LLM request, if projected tokens exceed limit
- **Post-tool evaluation**: After tool execution, if response would exceed budget

### Empty or Invalid LLM Responses

**Empty content without tool calls:**
- Response NOT added to conversation
- Synthetic retry triggered with injected ephemeral message: `"System notice: plain text responses without tool calls are ignored. Use final_report to provide your answer."`
- Counts as one attempt toward `maxRetries` budget

**Invalid tool call parameters:**
- Malformed tool calls dropped
- LLM receives error message describing validation failure
- Valid tool calls in same response still executed

### Synthetic Failure Report

**When max turns exhausted without `final_report`:**
- Application generates synthetic failure report
- Status: `failure`
- Content describes reason: max turns reached
- Metadata includes: `{ reason: 'max_turns_exhausted' }`
- Result marked `success: false`

---

## 3. Error Handling Contract

### Fatal Errors (No Retry, Immediate Failure)

**Auth errors** (invalid API key)
- Session fails immediately
- No retry attempts
- Exit code 1 (generic failure)
- `result.success: false`
- `result.error` populated with auth failure reason

**Quota exceeded** (billing limit)
- Session fails immediately
- No retry attempts
- Exit code 1 (generic failure)
- `result.success: false`
- `result.error` populated with quota details

### Retryable Errors (Cycle Providers)

**Rate limits**
- Honor `Retry-After` header when provided
- Fallback: exponential backoff (1s → 60s max)
- Cycle to next provider/model target
- If ALL targets rate-limited in same cycle → wait max backoff

**Network/timeout/model errors**
- Retry with next provider/model target immediately
- No backoff delay
- Continue until `maxRetries` exhausted

**Invalid/malformed responses**
- Synthetic retry with error message to LLM
- Cycle to next provider if retries exhausted on current provider

### Provider Cycling

**Round-robin through targets:**
1. Attempt 1: `targets[0]`
2. Attempt 2: `targets[1]`
3. Attempt N: `targets[(N-1) % targets.length]`

**Continues until:**
- Success (tool calls or final_report received)
- `maxRetries` exhausted across all targets
- Fatal error encountered

---

## 4. Accounting Contract

The `accounting` array in `AIAgentResult` provides observable telemetry for verification.

### LLM Accounting Entries

**Every LLM request produces an entry:**
- `type`: `'llm'`
- `provider`: Provider name used
- `model`: Model name used
- `status`: `'ok'` or `'failed'`
- `latency`: Request duration (ms)
- `tokens`: `{ inputTokens, outputTokens, cachedTokens?, totalTokens }`
- `timestamp`: Unix timestamp (ms)
- `error?`: Error message if failed
- Optional: `costUsd`, `actualProvider`, `actualModel`, `stopReason`
- Multi-agent tracing: `agentId?`, `callPath?`, `txnId?`, `parentTxnId?`, `originTxnId?`

### Tool Accounting Entries

**Every tool execution produces an entry:**
- `type`: `'tool'`
- `mcpServer`: MCP server name
- `command`: Tool name executed
- `status`: `'ok'` or `'failed'`
- `latency`: Execution duration (ms)
- `timestamp`: Unix timestamp (ms)
- `charactersIn`: Input size
- `charactersOut`: Output size
- `error?`: Error message if failed
- Multi-agent tracing: `agentId?`, `callPath?`, `txnId?`, `parentTxnId?`, `originTxnId?`

### Accounting Guarantees

**Provider fallback observable:**
- Provider cycling detectable by examining `provider` field sequence in accounting entries
- Failed attempts followed by different provider indicate fallback occurred

**Tool execution observable:**
- All tool executions appear in accounting (success or failure)
- Tool failures include error message

**Timing observable:**
- Latency measurements available for performance analysis
- Timestamps allow sequencing of events

**Limitations:**
- Turn numbers NOT included in accounting entries (turn sequence inferred from timestamps)
- Retry attempt numbers NOT included (retry count = number of failed entries before success)

---

## 5. Final Report Contract

### Final Report Guarantees

**Every session produces EITHER:**
1. **Tool-provided report**: LLM calls `final_report` tool successfully
2. **Text extraction fallback**: Final turn contains text content without `final_report`
3. **Synthetic failure**: Max turns exhausted without final answer

### Final Report Structure

**Required fields:**
- `status`: `'success'` | `'failure'` | `'partial'`
- `format`: Output format (must match session `expectedOutputFormat`)
- Content field (varies by format):
  - JSON format: `content_json` object
  - Slack format: `messages` array
  - Other formats: `content` string

**Optional fields:**
- `metadata`: Additional structured data
- `ts`: Timestamp when report was captured

### Format Enforcement

**`expectedOutputFormat` (session config):**
- Tool schema enforces format via `const` keyword
- Mismatched format logged as warning, normalized before storage
- Final report format ALWAYS matches session expectation

---

## 6. Conversation Contract

The `conversation` array in `AIAgentResult` contains the full message history.

### Message Roles

**Order guarantee:**
1. System messages (initial)
2. User message (task prompt)
3. Alternating: assistant → tool → assistant → tool → ...
4. Final: assistant with `final_report` tool call OR text response

### Tool Message Handling

**Successful tools:**
- Tool response added to conversation as `role: 'tool'`
- Contains `toolCallId` linking to assistant's tool call

**Failed tools:**
- Error message added to conversation as `role: 'tool'`
- Format: `(tool failed: <reason>)`

**Dropped tools (context overflow):**
- Failure stub added to conversation: `(tool failed: context window budget exceeded)`
- Ensures LLM sees evidence of failure

### Ephemeral Messages

**Synthetic retry messages:**
- NOT added to permanent conversation
- Used only for single retry attempt
- Example: "System notice: plain text responses are ignored..."

---

## 7. Configuration Loading Contract

### Layer Precedence

**Configuration sources (highest to lowest priority):**
1. `--config` explicit path
2. Current working directory (`.ai-agent.json`)
3. Prompt directory (if different from cwd)
4. Binary directory
5. `$HOME/.ai-agent/ai-agent.json`
6. `/etc/ai-agent/ai-agent.json`

**Merge behavior:**
- Higher priority layers override lower layers for same keys
- Deep merge for objects (providers, mcpServers, queues)

### Placeholder Expansion

**`${VARIABLE}` expansion:**
- Resolves from layer's `.ai-agent.env` file first
- Falls back to `process.env`
- Throws `MissingVariableError` if unresolved
- Error message includes layer origin for debugging

**Protected placeholders:**
- `${parameters.foo}` in REST tool payloads NOT expanded (passed to runtime)
- `mcpServers.*.env` and `mcpServers.*.headers` NOT expanded (passed to child process)

### Queue Validation

**All MCP/REST tool queue bindings MUST reference existing queue:**
- Validation runs at startup
- Missing queue → startup failure with clear error
- Default queue added automatically if missing

---

## 8. Implementation Notes

### What This Contract Does NOT Specify

**Internal implementation details:**
- Logging format, log identifiers, log severities
- Internal state machine transitions
- Token estimation algorithms (only guarantee: context window respected)
- Provider-specific API details (only guarantee: config passed correctly)

**Observable only through accounting:**
- Exact retry timing (only guarantee: `maxRetries` honored)
- Exact provider sequence (only guarantee: round-robin cycling)
- Internal error recovery paths (only guarantee: final result correct)

### Testing Implications

**Contract verification requires:**
1. Manipulating: user prompt, test MCP tool, test LLM provider
2. Observing: `AIAgentResult` fields (`success`, `finalReport`, `conversation`, `accounting`)
3. Validating: configuration settings respected, invariants hold

**Contract violations detectable via:**
- Final report presence/absence and format
- Conversation message sequence and roles
- Accounting entries (provider fallback, tool executions, timing)
- Exit codes matching expected categories
- Error field populated correctly

**Limitations in current accounting:**
- Turn counts must be inferred from conversation length or timestamp sequence
- Retry counts must be calculated from consecutive failed entries with same provider
- Configuration compliance testing requires conversation/result analysis beyond accounting alone

**NOT detectable (and NOT part of contract):**
- Specific log messages
- Internal log identifiers
- Log severities
- Internal state transitions

---

## 9. Contract Violations

Any deviation from the guarantees above is a **contract violation** and must be treated as a critical bug.

### Examples of Violations

❌ **Turn limit exceeded**
- Conversation shows 4 assistant messages when `maxTurns: 3`

❌ **Context window exceeded**
- Provider rejects request due to token overflow (should be prevented by context guard)

❌ **Missing final report**
- Session returns `success: true` but `finalReport` is undefined

❌ **Wrong exit code**
- CLI exits 0 when `result.success: false`, or exits 1 for configuration error (should be 4)

❌ **Configuration ignored**
- `temperature: 0.5` configured but provider receives `temperature: 1.0`

❌ **Tool response overflow without handling**
- Tool returns 10KB when `toolResponseMaxBytes: 1024`, no truncation or notice in conversation

### Non-Violations (Implementation Details)

✅ **Log message changed**
- Contract: context guard forces final turn when limit approached
- Non-violation: Log message wording changed

✅ **Internal state refactored**
- Contract: max retries honored
- Non-violation: Retry loop restructured

✅ **Token estimation algorithm improved**
- Contract: context window never exceeded
- Non-violation: Estimation accuracy improved

---

## 10. Version History

**1.0** (2025-11-19)
- Initial contract definition
- Extracted from `docs/specs/*.md` implementation details
- Focused on user-observable guarantees
