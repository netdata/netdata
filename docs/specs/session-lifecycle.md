# Session Lifecycle

## TL;DR
Session is created via a static factory; `AIAgent.run()` wraps orchestration (advisors/router/handoff) around the inner `SessionTurnRunner` loop. A session is complete only when finalization readiness is achieved (final report + required META when configured) or a synthetic failure report is emitted.

## Source Files
- `src/ai-agent.ts` - `AIAgentSession.create()` and top-level `run()` wiring
- `src/session-turn-runner.ts` - Main agent loop (retries, final turns, finalization readiness)
- `src/final-report-manager.ts` - Final report + required META state
- `src/context-guard.ts` - Context guard budgets and forced-final enforcement
- `src/plugins/runtime.ts` - Final-report plugin validation and cache gating
- `src/xml-transport.ts` - XML FINAL/META transport pairing
- `src/orchestration/*` - advisors/router/handoff helpers
- `src/session-tool-executor.ts` - tool execution, failure overrides, router selection capture

## Lifecycle Phases

### 1. Creation (`AIAgentSession.create`)
**Location**: `src/ai-agent.ts`

**Steps**:
1. Validate config: `validateProviders()`, `validateMCPServers()`, `validatePrompts()`
2. Generate session transaction ID: `crypto.randomUUID()`
3. Infer agent path from config or trace context
4. Enrich session config with trace fields
5. Create LLMClient with provider configs and tracing options
6. Instantiate AIAgentSession with empty conversation, logs, accounting
7. Bind external log relay to session

**Invariants**:
- Config validation throws on invalid input
- Session always has unique txnId
- originTxnId defaults to selfId for root agents
- parentTxnId undefined for root agents

### 2. Initialization (Constructor)
**Location**: `src/ai-agent.ts`

**Steps**:
1. Store config references (defensive copies)
2. Set up abort signal listener
3. Initialize target context configs (context windows, tokenizers, buffers)
4. Compute initial context token count
5. Initialize SubAgentRegistry if subAgents provided
6. Set trace IDs (txnId, originTxnId, parentTxnId)
7. Initialize opTree (SessionTreeBuilder)
8. Initialize progressReporter
9. Initialize toolBudgetCallbacks
10. Begin system turn (turn 0)
11. Log session initialized
12. Initialize ToolsOrchestrator with:
    - MCPProvider (mcp)
    - RestProvider (rest) - if REST tools configured
    - InternalToolProvider (agent) - always registered
    - AgentProvider (agent) - if subAgents present
13. Apply initial session title if provided

**Invariants**:
- Turn 0 is always system initialization
- ToolsOrchestrator mapping populated before first turn
- All providers registered synchronously

### 3. Orchestration Wrapper (`AIAgent.run`)
**Location**: `src/ai-agent.ts`

**Steps**:
1. If no orchestration is configured, call `AIAgentSession.run()` directly.
2. **Advisors**: run advisor sessions in parallel with the original user prompt.
3. Build an enriched prompt containing `<original_user_request>` and `<advisory>` blocks (synthetic failures included).
4. Run the main session with the enriched prompt.
5. If the session selected a router destination, run the router target with an advisory block (optional router message).
6. If `handoff` is configured, run the handoff target with `<original_user_request>` and `<response>` blocks.
7. Merge results (parent conversation/logs + child accounting/logs; child final report becomes top-level).

### 4. Execution (`AIAgentSession.run`)
**Location**: `src/ai-agent.ts`

**Steps**:
1. Create OpenTelemetry span with session attributes
2. Emit `agent_started` progress event
3. Warm up tools orchestrator (MCP connection, tool discovery)
4. If verbose: log settings summary
5. Log tools banner (MCP, REST, subagent counts)
6. Check pricing coverage, warn on missing entries
7. Use static system prompt from agent load-time render (no runtime variable expansion)
8. Expand user prompt with runtime prompt variables (CLI-only placeholders)
9. Build enhanced system prompt by appending internal tools + MCP instructions (runtime sections)
10. Initialize conversation:
    - If conversationHistory: merge with enhanced system prompt
    - Else: create new with system + user messages
11. Execute agent loop (XML modes inject XML-PAST/XML-NEXT each turn and hide provider tool definitions)
12. Flatten opTree for logs/accounting
13. End system turn (turn 0)
14. End session in opTree
15. Emit agent completion event
16. Persist final session snapshot
17. Flush accounting entries
18. Return AIAgentResult

**Error Handling**:
- Uncaught exception: log EXIT-UNCAUGHT-EXCEPTION, emit FIN summary, return failure
- Always cleanup toolsOrchestrator in finally block

### 5. Agent Loop (`executeAgentLoop`)
**Location**: `src/session-turn-runner.ts`

**Structure**:
```
for turn = 1 to maxTurns:
  check cancellation/stop
  begin turn in opTree
  XML-final transport is always used: rebuild XML-NEXT (nonce, final slot, optional schema) and append to conversation. Provider tool lists remain native and XML-PAST is suppressed.

  attempts = 0
  while attempts < maxRetries and not successful:
    select provider/model from targets (round-robin)
    check context guard
    XML-NEXT carries final-turn instruction if needed (no other system notices)

    execute single turn (LLM request)
    sanitize messages
    process tool calls

    handle status:
      - success with tools: execute tools, continue turn
      - success with finalization readiness: finalize
      - success with final report but missing/invalid required META: retry (final report locked, META-only guidance)
      - success without tools: retry (synthetic failure)
      - rate_limit: skip provider/model pair on non-200; otherwise backoff and retry
      - auth_error: skip provider/model pair
      - quota_exceeded: skip provider/model pair
      - model_error: skip provider/model pair or retry
      - network_error: retry
      - timeout: retry
      - invalid_response: retry

    record accounting
    update opTree

  end turn in opTree

  if finalization readiness achieved: break
  if max turns reached: finalize
```

**Retry Logic**:
- `maxRetries` is global cap per turn (default: 3)
- Provider cycling: round-robin through targets array
- Backoff for rate limits: only when retrying (no non-200 skip)
- Synthetic retries: content without tools triggers retry
- Final turn enforcement: last turn allows final report and required META; extra tools (such as `router__handoff-to`) are allowed only when configured and the final report is not locked

**Context Guard**:
- Before each LLM request: evaluate projected token usage
- If exceeds limit: enforce final turn or skip provider/model pair
- Post-shrink warning if still over limit after enforcement

### 6. Turn Execution (`executeSingleTurn`)
**Location**: `src/ai-agent.ts:3521-3952`

**Steps**:
1. Begin LLM operation in opTree
2. Resolve reasoning value for provider
3. Build turn request with:
    - Messages
    - Tools (filtered for final turn)
    - Temperature, topP
    - Reasoning settings
    - Max output tokens
    - Caching mode
4. Call `llmClient.executeTurn()`
5. Process streaming response (if enabled)
6. Return turn result with messages, tokens, status

### 7. Tool Execution
**After successful LLM turn with tool calls**:

1. Log assistant message with tool calls
2. For each tool call:
    - Begin tool operation in opTree
    - Route to appropriate provider (MCP/REST/Internal/Agent)
    - Apply timeout wrapper
    - Apply response size cap
    - Reserve context budget via `ToolBudgetCallbacks.reserveToolOutput` (drops responses that would overflow)
    - Capture result
    - End tool operation
3. When reservation succeeds: add tool result messages to conversation
4. Continue to next turn (or retry if context guard enforced a final turn)

### 8. Finalization
**Triggered by**:
- Finalization readiness achieved (final report + required META when configured)
- Max turns reached without finalization readiness
- Context guard enforcement, retry exhaustion, or cancellation/stop handling
- Error conditions that require synthetic finalization

**Steps**:
1. Capture the final report and required plugin META blocks; if META is missing, lock the final report and retry with META-only guidance
2. Validate final report and required META schemas
3. When finalization readiness is achieved, run plugin `onComplete` hooks and emit final output once (locked-final retries suppress duplicate streaming)
4. Log finalization details (source, reasons, missing META diagnostics) and end all open operations in opTree
5. If finalization readiness cannot be achieved, synthesize a failure report with an explicit reason (for example, `final_meta_missing`)
5. Return AIAgentResult

### 9. Cleanup
**Finally block responsibilities**:

1. `toolsOrchestrator.cleanup()` - closes MCP connections

Final summaries (`agent:fin`, `emitAgentCompletion`) and snapshot/accounting callbacks run inside the `try`/`catch` branches before the method returns, so failures inside `finally` cannot suppress FIN logs.

## State Transitions

**Note**: The session does NOT track an explicit state field. The lifecycle phases are conceptual and implicit in the execution flow. There is no `this.state` property or state enum in the implementation.

```
Constructor → run() entry → executeAgentLoop() → finalization → cleanup
                                    ↓
                              error handling
```

**Implicit phases**:
- **Creation**: `AIAgentSession.create()` factory returns instance
- **Initialization**: Constructor configures tools, context, tracers
- **Execution**: `run()` warms tools, then enters `executeAgentLoop()`
- **Finalization**: Finalization readiness achieved (final report + required META) or a synthetic failure report is emitted on exhaustion
- **Cleanup**: `finally` block closes the tool orchestrator (logs/FIN events already emitted before returning)

The flow is **procedural**, not state-machine driven. Error handling is inline via try/catch blocks, not state transitions.

## Cancellation Paths

### Abort Signal
- Checked at turn start
- Propagated to child operations
- Sets `this.canceled = true`
- Returns `finalizeCanceledSession()`

### Stop Signal Handling
- Checked via `stopRef.stopping` and `stopRef.reason`
- Three stop reasons:
  - `'stop'`: Graceful stop - triggers final turn with `forcedFinalReason='user_stop'`, allows model to summarize, success=true
  - `'abort'`: Immediate cancel - no final turn, success=false, abortSignal fired
  - `'shutdown'`: Global shutdown - no final turn, success=false, abortSignal fired
- Tool execution during final turn: `final_report` is always allowed; `router__handoff-to` is allowed only when router destinations are configured and the final report is not locked
- Exit code: `EXIT-USER-STOP` when reason='stop' and finalization readiness is achieved

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `maxTurns` | Loop iteration limit |
| `maxRetries` | Per-turn retry cap |
| `targets` | Provider/model cycling order |
| `abortSignal` | Cancellation trigger |
| `stopRef` | Stop signal with reason: `{ stopping: boolean; reason?: 'stop' \| 'abort' \| 'shutdown' }` |
| `conversationHistory` | Initial conversation state |
| `toolTimeout` | Per-tool execution timeout |
| `toolResponseMaxBytes` | Oversize tool outputs stored + tool_output handle inserted |
| `toolOutput` | Overrides tool_output extraction/storage behavior (`storeDir` ignored; root is `/tmp/ai-agent-<run-hash>`) |

## Telemetry

**Spans**:
- `agent.session` - Root span for entire session
- Turn operations recorded in opTree

**Attributes**:
- `ai.agent.id`
- `ai.agent.call_path`
- `ai.session.txn_id`
- `ai.session.parent_txn_id`
- `ai.session.origin_txn_id`
- `ai.session.headend_id`
- `ai.agent.success`
- `ai.agent.turn_count`
- `ai.agent.final_report.status`

## Logging

**Key Log Events**:
- `agent:init` - Session initialized (turn 0)
- `agent:settings` - Configuration summary (verbose)
- `agent:tools` - Tool banner
- `agent:pricing` - Missing pricing warning
- `agent:start` - User prompt (verbose)
- `agent:turn-start` - Each LLM turn begins (VRB)
- `agent:final-turn` - Final turn detected (WRN)
- `agent:context` - Context guard events
- `agent:text-extraction` - Final report payload parsed from assistant text/tool message fallback
- `agent:fallback-report` - Final report synthesized from tool message fallback and accepted
- `agent:final-report-accepted` - Final report committed; `details.source` disambiguates tool-call vs fallback vs synthetic, but finalization readiness may still require META
- `agent:failure-report` - Synthetic failure final report synthesized when finalization readiness cannot be achieved (ERR)
- `agent:fin` - Session finalized
- `agent:error` - Uncaught exception
- Exit codes: `agent:EXIT-*`

## Events

- `agent_started` - Session began
- `agent_update` - Progress update (includes `taskStatus` when emitted via `agent__task_status`)
- `agent_finished` - Success completion
- `agent_failed` - Error completion

## Business Logic Coverage (Verified 2026-01-25)

- **Finalization readiness contract**: Session success requires both the final report and all required plugin META blocks. Missing/invalid META locks the final report, triggers META-only retries, and exhaustion can synthesize `reason: "final_meta_missing"` (`src/final-report-manager.ts`, `src/plugins/runtime.ts`, `src/session-turn-runner.ts`).
- **Cache gating requires META validation**: Cache hits and writes are accepted only when required META blocks validate; cache entries without META are treated as misses (`src/plugins/runtime.ts`, `src/ai-agent.ts`).
- **Final-turn tool filtering with locked-final gating**: Final turns keep `agent__final_report` and only allow extra tools (such as router handoff) when configured and the final report is not locked (`src/session-turn-runner.ts`, `src/ai-agent.ts`, `src/llm-messages-xml-next.ts`).
- **TURN-FAILED repair guidance**: Retry feedback is accumulated as TURN-FAILED events and flushed as a single user message before each attempt, keeping guidance specific while avoiding duplicate spam (`src/session-turn-runner.ts`, `src/llm-messages-turn-failed.ts`).
- **Context guard enforcement**: Per-provider evaluations can skip oversized targets or force final-turn mode; enforcement, counters, and telemetry live in `ContextGuard`, with runner-level preflight decisions (`src/context-guard.ts`, `src/session-turn-runner.ts`).
- **Rate-limit cycle gating + abort-aware sleep**: The runner backs off only when all provider/model pairs rate-limit within the same cycle, and every wait honors cancel/stop signals via `sleepWithAbort` (`src/session-turn-runner.ts`).
- **Tool budget mutex**: Tool budget reservations run under a mutex so concurrent tool executions cannot race token projections (`src/context-guard.ts`).
- **Tool failure overrides**: Tool failures can be replaced with curated fallback messages that are later surfaced to the model and final reports (`src/session-tool-executor.ts`, `src/session-turn-runner.ts`).
- **Snapshot + accounting flush**: Final snapshots and accounting flushes still emit deterministic events for headends and billing hooks (`src/ai-agent.ts`, `src/persistence.ts`).
- **Child session capture**: Sub-agent runs preserve ancestry (`originTxnId`, `parentTxnId`, `agentPath`) and capture nested conversations for auditing (`src/subagent-registry.ts`, `src/session-turn-runner.ts`).

10. **evaluateContextForProvider()**:
    - Returns: 'ok' | 'skip' | 'final'
    - Decision logic for provider context fit
    - Location: `src/ai-agent.ts:3164-3194`

## Test Coverage

**Phase 2 Tests**:
- Session creation validation
- Turn execution flow
- Retry logic scenarios
- Context guard enforcement
- Tool execution
- Final report capture
- Cancellation handling

**Gaps**:
- Edge cases in conversation history merging
- Complex multi-provider cycling scenarios
- Synthetic final report adoption scenarios
- Context-forced fallback report testing

## Troubleshooting

### Session creation fails
- Check provider validation errors
- Check MCP server config format
- Check prompt length/content

### Turn never completes
- Check llmTimeout setting
- Check provider availability
- Check abort signal not triggered

### Max retries exhausted
- Check provider error responses
- Check rate limit headers
- Check network connectivity

### Context guard fires unexpectedly
- Check contextWindow settings per model
- Check contextWindowBufferTokens
- Check tool output sizes

### Final report missing
- Check final report tool call validation
- Check required META blocks are present and schema-valid when plugins are configured
- Check XML wrappers use the session nonce and the correct FINAL/META structure
