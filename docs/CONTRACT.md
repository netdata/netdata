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

See Section 2 “Context Management Contract” for the complete definition of counters, guard calculations, and enforcement rules. This configuration section only names the tunables (`contextWindow`, `contextWindowBufferTokens`, `maxOutputTokens`); their behaviour is governed entirely by that contract.

### Tool Response Handling

**`toolResponseMaxBytes`**
- Tool responses exceeding this size are truncated
- Truncated responses include notice: `[TRUNCATED] Original size X bytes; truncated to Y bytes.` (prepended to content)
- Warning logged with metadata: tool name, actual bytes, limit bytes
- Original response only preserved in logs when `traceTools` mode enabled
- **Exception:** `agent__batch` meta-tool output NOT truncated as a whole; individual tools within the batch ARE subject to truncation

**`toolTimeout`**
- Tool execution aborted after this duration (milliseconds)
- Timed-out tools marked as `status: 'failed'` in accounting
- LLM receives failure message: `(tool failed: timeout)`

### Tool Request Parsing and Repair

- Tool arguments are parsed with a deterministic two-step pipeline: native `JSON.parse`, then `jsonrepair` + re-parse when the first parse fails. Additional guarded fixes may close dangling braces or extract the first JSON object when needed.
- When repair succeeds, a WARN log records the exact repair steps and includes both original and repaired payloads for visibility.
- When parsing fails, the tool call is dropped, an ERR log records the full raw payload (not truncated), and the LLM is told to retry; valid calls in the same message still proceed.
- `agent__final_report` adds an `encoding` field (`raw` | `base64`); when `base64` is provided, `report_content` is decoded before use. `content_json` undergoes schema validation with a small iterative repair loop that attempts to parse stringified nested JSON fields before final rejection.
- If the `agent__final_report` tool fails for any reason (validation, decode, execution error), remaining turns are collapsed immediately and an ERR log is emitted containing the full JSON payload that was provided to the tool.

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
- Model-provided final report: `success: true`, `finalReport` with `status: 'success'`
- Synthetic failure (max turns, context overflow): `success: false`, `finalReport` with `status: 'failure'`
- Fatal errors (auth, quota): `success: false`, `error` field populated

**Note**: The `status` field is determined by the source of the final report, not by model input. Models do not provide a status field; the system sets it based on whether the report was model-provided (success) or synthetic (failure).

### CLI Exit Codes

**Exit code 0**: Session succeeded or informational output completed
**Exit code 1**: Generic session failure or internal error
**Exit code 3**: Missing/failed MCP servers or initialization failures
**Exit code 4**: Invalid arguments, configuration errors, or validation failures
**Exit code 5**: Schema validation failures (e.g., tool schema validation)

Note: Exit codes map to failure categories, not specific error types. Check `result.error` and `result.finalReport.status` for detailed failure reasons.

### Context Management Contract

#### Counters and Inputs
- `currentCtxTokens`: Tokens already committed to the conversation by the previous successful LLM response. By contract this equals `inputTokens + outputTokens + cacheReadTokens` reported by the provider. Turn 1 MUST log `ctx 0` because no request has been sent yet.
- `pendingCtxTokens`: Estimated token cost of every message appended since the last LLM request (tool outputs, retry notices, new user input). These tokens will enter the next prompt and therefore appear as `new` in the request log.
- `newCtxTokens`: Scratch bucket for tool outputs generated during the current turn before the guard flushes them into `pendingCtxTokens`. Every guard evaluation MUST move `newCtxTokens` into `pendingCtxTokens` before calculating projections, ensuring nothing is lost during retries.
- `schemaCtxTokens`: Token estimate for the tool schema exposed on the upcoming request. When tools are restricted (e.g., forced final turn), this value MUST be recomputed for the reduced tool set.
- `contextWindow`: Provider/model capacity. `contextWindowBufferTokens` subtracts a safety margin, and `maxOutputTokens` reserves space for the model’s reply. These three values define the hard ceiling for projections.

#### LLM Request Metrics
- Every `LLM request prepared` log MUST satisfy `expectedTokens = ctxTokens + pendingCtxTokens + schemaCtxTokens`.
- `ctxTokens` MUST match the provider-reported `ctx` from the previous response (0 on the very first turn).
- `new tokens` shown in the log equal `pendingCtxTokens` at the moment of logging and therefore represent the prompt delta the model is about to receive.
- Any retry-only system/user nudges included via `pendingRetryMessages` count toward `pendingCtxTokens` before the log is emitted.

#### Guard Projection
- Projection formula: `projected = currentCtxTokens + pendingCtxTokens + newCtxTokens + schemaCtxTokens`.
- Per target limit: `limit = contextWindow - contextWindowBufferTokens - maxOutputTokens`.
- Guard triggers in two places:
  - **`tool_preflight`**: before committing a tool response.
  - **`turn_preflight`**: before issuing the next LLM request.
- If `projected > limit`, the guard MUST record the blocked provider/model pair and move the session into forced-final handling.

#### Tool Overflow Handling
- Tool outputs whose estimated tokens would overflow the projection are dropped immediately. The LLM receives the deterministic stub `(tool failed: context window budget exceeded)` and the accounting entry captures both the original estimate and the replacement token count.
- Once a drop occurs, the orchestrator sets its `toolBudgetExceeded` flag, which forbids scheduling any NEW user-facing tools. Parallel executions already in flight may finish, but no additional tools may start until the session concludes.
- Overflow drops are logged with `projected_tokens`, `limit_tokens`, and `remaining_tokens` (if positive) so operators can audit exactly why the response was rejected.

#### Forced Final Turn
- When the guard fires (either from a tool overflow or a turn preflight), the session immediately injects the final-instruction system message, restricts the tool list to `agent__final_report`, and logs the `forcedFinalTurnReason = 'context'`.
- Schema tokens are recomputed for the reduced toolset and the guard is re-evaluated. If projections still exceed the limit, the session proceeds best-effort but MUST log the post-shrink warning.
- From this point forward no new tools (including progress_report) may be executed; only the final report tool is allowed, and the agent must use already gathered information to respond.

### Empty or Invalid LLM Responses

**Empty content without tool calls:**
- If the assistant returns only reasoning (non-empty `reasoning` field) and no tool calls or text content, the turn is allowed to proceed; the reasoning is preserved in the conversation for the next turn.
- Otherwise (no content, no tools, no reasoning), the response is NOT added to the conversation, and a synthetic retry is triggered with the ephemeral message: `"System notice: plain text responses without tool calls are ignored. Use final_report to provide your answer."`
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

**System-determined fields:**
- `status`: `'success'` | `'failure'` — determined by source, not model input:
  - `'success'`: Model provided a final report via tool call or text fallback
  - `'failure'`: System generated a synthetic failure report (max turns, context overflow)
- `format`: Output format (must match session `expectedOutputFormat`)
- `ts`: Timestamp when report was captured

**Model-provided fields:**
- Content field (varies by format):
  - JSON format: `content_json` object
  - Slack format: `messages` array
  - Other formats: `content` string
- `metadata`: Additional structured data (optional)

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

## 11. XML Tool Transport Contract (XML-PAST / XML-NEXT)

**Modes**
- `xml-final` (default): provider tools stay native (tool_calls), but the final report must be emitted via XML. Progress follows tools transport (native). Tool choice set to `auto`; provider tool definitions remain visible to the LLM for native calls.
- `native`: unchanged tool-call behavior, tool choice may be `required`.
- `xml`: all tools may be invoked via XML tags; tool choice set to `auto`, and provider tool definitions are withheld (XML is the only invocation path). Progress uses the XML channel when enabled.

**Session nonce and slots**
- Each session defines a fixed nonce (e.g., `<8hex>`) that remains constant across all turns.
- Numbered invocation slots increment across turns: turn 1 uses `NONCE-0001`, `NONCE-0002`, etc.; turn 2 continues from where turn 1 left off (e.g., `NONCE-0004`, `NONCE-0005`).
- Special slots: `NONCE-FINAL` for final report; `NONCE-PROGRESS` for progress (xml mode only, not xml-final).
- Tags are detected by substring only (no XML parser): `<ai-agent-NONCE-000X tool="...">payload</ai-agent-NONCE-000X>`.

**Messages**
- XML-PAST (permanent): a user message containing prior turn tool results (slot id, tool, status, duration, request, response). Suppressed in `xml-final` mode. Intended for the model’s context; may be capped to last turn.
- XML-NEXT (ephemeral, not stored/cached): per-turn instructions with the current nonce, available tools and their JSON schemas, slot templates, progress slot (optional), and final-report slot. In forced-final turns it only advertises the final-report slot (and optional progress).

**Final-report via XML**
- Final-report uses a reserved slot in XML-NEXT (`tool="agent__final_report"`, format attribute, raw content).
- Tag attributes: `format="<expected-format>"`.
- Status is not provided by the model; system determines status based on source (model-provided = success, synthetic = failure).
- Processing uses 3-layer architecture:
  1. **Layer 1 (Transport)**: Extract `format` from XML tag attributes; extract raw payload from tag content.
  2. **Layer 2 (Format Processing)**: Process payload based on format—`sub-agent` is opaque passthrough (no validation), `json` parses and validates, `slack-block-kit` expects messages array, text formats use raw content.
  3. **Layer 3 (Final Report)**: Construct clean final report object with status='success'. Wrapper fields never pollute payload content.
- This separation ensures user schema fields are never overwritten by transport metadata.

**Unclosed final-report tag handling**
- When a valid final-report opening tag is detected (`<ai-agent-NONCE-FINAL tool="agent__final_report">`) with content following it, the closing tag requirement depends on the LLM's `stopReason`:
  - `stop`, `end_turn`, `end`, `eos` (normal completion): Accept content even without closing tag.
  - `length`, `max_tokens` (truncation): Treat as incomplete, trigger retry.
  - Unknown/undefined: Log warning but accept content.
- This applies ONLY to `agent__final_report`; other tools always require complete tags.
- Rationale: Large models often stop generating after completing their final answer without emitting closing tags. The stop reason is the authoritative signal for completion.

**Tool execution and responses**
- Tags from the assistant are parsed by nonce and unused slot id; invalid/mismatched tags are ignored (do not count as attempts).
- Each valid tag is executed through the existing tool orchestrator (same budgets, maxToolCallsPerTurn, accounting, retries). Tools are “emulated” — transport differs, execution identical.
- Tool responses are returned as user messages: `System Notice: tool response for NONCE-000X (toolName)` containing request/response blocks. This avoids provider role-validation issues. Final-report produces no response message; it finalizes the session.

**Ordering and selection**
- Structured outputs: last valid matching tag for a given tool is used when a single result is expected (e.g., final-report). Unstructured outputs may be combined per tool rules; if not specified, each slot maps to one tool execution.

**Retries and missing outputs**
- Missing or invalid tags follow the existing retry logic for missing final-report/tool calls; empty/reasoning-only outputs are treated as missing. Retry budgets and provider cycling are unchanged.

**Prompting and schemas**
- The system prompt contains no tool/final-report text in XML modes; all instructions/schemas live in XML-NEXT. This allows forced-final turns to hide non-final tools by omitting them from XML-NEXT.

**Accounting/telemetry**
- Accounting entries use the real tool name with `source: xml`. Final-report emits `command: agent__final_report_xml` with status from the tag. Metrics/histograms remain, now distinguishable by source.

**Streaming**
- Streaming output is buffered once an opening-tag fragment for the current nonce appears and stops at the matching closing tag. Content outside matched tags is ignored.

**Unchanged**
- Context guard logic and limits are unchanged. Tool truncation rules remain; no size guard is applied to final outputs.

---

## 12. Version History

**1.0** (2025-11-19)
- Initial contract definition
- Extracted from `docs/specs/*.md` implementation details
- Focused on user-observable guarantees
