# Session Lifecycle

## TL;DR
Session created via static factory, executes main loop with turn-level retries, finalizes with cleanup and accounting flush.

## Source Files
- `src/ai-agent.ts:769-837` - `AIAgentSession.create()`
- `src/ai-agent.ts:1037-1388` - `AIAgentSession.run()`
- `src/ai-agent.ts:1390-2600+` - `executeAgentLoop()`

## Lifecycle Phases

### 1. Creation (`AIAgentSession.create`)
**Location**: `src/ai-agent.ts:769-837`

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
**Location**: `src/ai-agent.ts:410-767`

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
    - AgentProvider (subagent) - if subAgents present
13. Apply initial session title if provided

**Invariants**:
- Turn 0 is always system initialization
- ToolsOrchestrator mapping populated before first turn
- All providers registered synchronously

### 3. Execution (`run()`)
**Location**: `src/ai-agent.ts:1037-1388`

**Steps**:
1. Create OpenTelemetry span with session attributes
2. Emit `agent_started` progress event
3. Warm up tools orchestrator (MCP connection, tool discovery)
4. If verbose: log settings summary
5. Log tools banner (MCP, REST, subagent counts)
6. Check pricing coverage, warn on missing entries
7. Expand system prompt (apply format placeholder, variable expansion)
8. Expand user prompt
9. Build enhanced system prompt with tool instructions (native only; XML transport omits tool instructions)
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

### 4. Agent Loop (`executeAgentLoop`)
**Location**: `src/ai-agent.ts:1390-2600+`

**Structure**:
```
for turn = 1 to maxTurns:
  check cancellation/stop
  begin turn in opTree
  if transport=xml-final: rebuild XML-NEXT (nonce, slots, schemas, final-report slot) and XML-PAST (previous turn results); append to conversation. Provider tool lists remain native.

  attempts = 0
  while attempts < maxRetries and not successful:
    select provider/model from targets (round-robin)
    check context guard
    inject final turn instruction if needed

    execute single turn (LLM request)
    sanitize messages
    process tool calls

    handle status:
      - success with tools: execute tools, continue turn
      - success with final_report: finalize
      - success without tools: retry (synthetic failure)
      - rate_limit: backoff and retry
      - auth_error: fatal, abort
      - quota_exceeded: fatal, abort
      - model_error: skip provider or retry
      - network_error: retry
      - timeout: retry
      - invalid_response: retry

    record accounting
    update opTree

  end turn in opTree

  if final_report captured: break
  if max turns reached: finalize
```

**Retry Logic**:
- `maxRetries` is global cap per turn (default: 3)
- Provider cycling: round-robin through targets array
- Backoff for rate limits: exponential with max wait
- Synthetic retries: content without tools triggers retry
- Final turn enforcement: last turn only allows final_report

**Context Guard**:
- Before each LLM request: evaluate projected token usage
- If exceeds limit: enforce final turn or skip provider
- Post-shrink warning if still over limit after enforcement

### 5. Turn Execution (`executeSingleTurn`)
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

### 6. Tool Execution
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

### 7. Finalization
**Triggered by**:
- `agent__final_report` tool called
- Max turns reached
- Context guard enforced
- Error conditions

**Steps**:
1. Capture final report (status, format, content)
2. Validate JSON schema if applicable
3. Log finalization
4. End all open operations in opTree
5. Return AIAgentResult

### 8. Cleanup
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
- **Finalization**: Final report captured or max turns exhausted
- **Cleanup**: `finally` block closes the tool orchestrator (logs/FIN events already emitted before returning)

The flow is **procedural**, not state-machine driven. Error handling is inline via try/catch blocks, not state transitions.

## Cancellation Paths

### Abort Signal
- Checked at turn start
- Propagated to child operations
- Sets `this.canceled = true`
- Returns `finalizeCanceledSession()`

### Graceful Stop
- Checked via `stopRef.stopping`
- Allows current turn to complete
- Does not start new turns
- Returns `finalizeGracefulStopSession()`

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `maxTurns` | Loop iteration limit |
| `maxRetries` | Per-turn retry cap |
| `targets` | Provider/model cycling order |
| `abortSignal` | Cancellation trigger |
| `stopRef` | Graceful stop trigger |
| `conversationHistory` | Initial conversation state |
| `toolTimeout` | Per-tool execution timeout |
| `toolResponseMaxBytes` | Response size cap |

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
- `agent:text-extraction` - Final report payload parsed from assistant text/tool message (still pending)
- `agent:fallback-report` - Cached pending payload accepted on the forced final turn
- `agent:final-report-accepted` - Final report committed; `details.source` disambiguates tool-call vs fallback vs synthetic
- `agent:failure-report` - Synthetic failure final report synthesized (ERR)
- `agent:fin` - Session finalized
- `agent:error` - Uncaught exception
- Exit codes: `agent:EXIT-*`

## Events

- `agent_started` - Session began
- `agent_update` - Progress update
- `agent_finished` - Success completion
- `agent_failed` - Error completion

## Business Logic Coverage (Verified 2025-11-16)

- **Pending final report cache & fallback acceptance**: `tryAdoptFinalReportFromText` now returns a payload that is stored in `pendingFinalReport` (instead of fabricating a tool call). The orchestrator keeps retrying until the forced final turn, then optionally accepts the cached payload (logging `agent:fallback-report` and preserving `finalReportSource`). (`src/ai-agent.ts:1862-1960`, `src/ai-agent.ts:3337-3410`).
- **Context-forced fallback report**: When the context guard blocks further turns, the session synthesizes a `failure` final report, logs `agent:failure-report` + `EXIT-TOKEN-LIMIT`, and surfaces the reason in the FIN summary and telemetry (`src/ai-agent.ts:2593-2625`).
- **Incomplete final report detection**: If the assistant calls the final-report tool without required fields, the session shortens `maxTurns`, injects instructions, and retries within the same provider cycle (`src/ai-agent.ts:2252-2275`).
- **Tool failure overrides**: `toolFailureMessages` / `toolFailureFallbacks` let MCP/REST providers replace low-level errors with curated text so final answers consistently describe which tool failed (`src/ai-agent.ts:1671-1895`, `src/ai-agent.ts:3707-3952`).
- **sleepWithAbort + retry directives**: Backoff waits respect aborts, and `buildFallbackRetryDirective` crafts deterministic retry metadata when providers omit guidance (covers rate_limit, auth, quota, timeout, and network errors) (`src/ai-agent.ts:2470-2545`, `src/ai-agent.ts:3298-3338`).
- **Tool budget mutex**: Concurrent tool executions acquire a mutex before accounting for output bytes, ensuring the `(tool failed: response exceeded ...)` guard is deterministic (`src/ai-agent.ts:155-562`, `src/ai-agent.ts:516-562`).
- **Pending retry deduplication**: `pushSystemRetryMessage` keeps a deduped queue of system reminders so multi-provider retries don't spam instructions in conversation history (`src/ai-agent.ts:198, 2959-3042`).
- **Context limit warning gating**: `contextLimitWarningLogged` prevents repetitive warnings; once logged the session suppresses duplicate notices to keep logs clean (`src/ai-agent.ts:159, 2338-2368`).
- **Snapshot + accounting flush**: After `executeAgentLoop()` succeeds, `persistSessionSnapshot('final')` streams the opTree through `onSessionSnapshot` callbacks and `flushAccounting()` pushes accumulated entries to `onAccountingFlush`; on uncaught exceptions the catch path skips both calls (`src/ai-agent.ts:360-420`, `src/ai-agent.ts:1256-1310`).
- **Child session capture**: `childConversations` stores prompt paths, agent IDs, and conversations for every sub-agent call, and FIN summaries aggregate their accounting so parent sessions can render nested timelines (`src/ai-agent.ts:700-884`, `src/ai-agent.ts:3120-3220`).

10. **evaluateContextForProvider()**:
    - Returns: 'ok' | 'skip' | 'final'
    - Decision logic for provider context fit
    - Location: `src/ai-agent.ts:3164-3194`

## Test Coverage

**Phase 1 Tests**:
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
- Check tool call validation
- Check schema compliance
- Check format parameter matches expected
