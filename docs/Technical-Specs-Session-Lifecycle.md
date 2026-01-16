# Session Lifecycle

Session creation, execution, and finalization flow.

---

## TL;DR

Session created via static factory; `AIAgent.run()` wraps orchestration around the inner `AIAgentSession.run()` loop.

---

## Lifecycle Phases

### 1. Creation

**Method**: `AIAgentSession.create(config)`

**Steps**:
1. Validate config (providers, MCP servers, prompts)
2. Generate session transaction ID
3. Infer agent path from config
4. Enrich config with trace fields
5. Create LLMClient with provider configs
6. Instantiate AIAgentSession
7. Bind external log relay

**Invariants**:
- Config validation throws on invalid input
- Session always has unique txnId
- originTxnId defaults to selfId for root agents

---

### 2. Initialization (Constructor)

**Steps**:
1. Store config references
2. Set up abort signal listener
3. Initialize target context configs
4. Compute initial context token count
5. Initialize SubAgentRegistry if subAgents provided
6. Set trace IDs
7. Initialize opTree (SessionTreeBuilder)
8. Initialize progressReporter
9. Begin system turn (turn 0)
10. Initialize ToolsOrchestrator
11. Apply initial session title

---

### 3. Orchestration Wrapper

**Method**: `AIAgent.run(session)`

**Steps**:
1. If no orchestration → call `AIAgentSession.run()` directly
2. **Advisors**: run advisor sessions in parallel
3. Build enriched prompt with advisory blocks
4. Run main session with enriched prompt
5. If router selection → run router target
6. If `handoff` configured → run handoff target
7. Merge results

---

### 4. Execution

**Method**: `AIAgentSession.run()`

**Steps**:
1. Create OpenTelemetry span
2. Emit `agent_started` event
3. Warm up tools orchestrator
4. Log settings summary (if verbose)
5. Log tools banner
6. Check pricing coverage
7. Expand system and user prompts
8. Build enhanced system prompt
9. Initialize conversation
10. Execute agent loop
11. Flatten opTree
12. End system turn
13. End session in opTree
14. Emit completion event
15. Persist final snapshot
16. Flush accounting
17. Return AIAgentResult

---

### 5. Agent Loop

**Method**: `executeAgentLoop()`

```
for turn = 1 to maxTurns:
  check cancellation/stop
  begin turn in opTree

  attempts = 0
  while attempts < maxRetries and not successful:
    select provider/model (round-robin)
    check context guard
    execute single turn (LLM request)
    sanitize messages
    process tool calls

    handle status:
      - success with tools: execute tools
      - success with final_report: finalize
      - success without tools: retry
      - rate_limit: backoff and retry
      - auth_error: abort
      - model_error: retry or skip

    record accounting
    update opTree

  end turn in opTree
  if final_report captured: break
```

---

### 6. Turn Execution

**Method**: `executeSingleTurn()`

**Steps**:
1. Begin LLM operation in opTree
2. Resolve reasoning value for provider
3. Build turn request (messages, tools, params)
4. Call `llmClient.executeTurn()`
5. Process streaming response (if enabled)
6. Return turn result

---

### 7. Tool Execution

**After successful LLM turn with tool calls**:

1. Log assistant message with tool calls
2. For each tool call:
   - Begin tool operation in opTree
   - Route to appropriate provider
   - Apply timeout wrapper
   - Apply response size cap
   - Reserve context budget
   - Capture result
   - End tool operation
3. Add tool result messages to conversation
4. Continue to next turn

---

### 8. Finalization

**Triggers**:
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

---

### 9. Cleanup

**Finally block**:
- `toolsOrchestrator.cleanup()` - closes MCP connections

---

## Cancellation Paths

### Abort Signal
- Checked at turn start
- Propagated to child operations
- Returns `finalizeCanceledSession()`

### Graceful Stop
- Checked via `stopRef.stopping`
- Allows current turn to complete
- Returns `finalizeGracefulStopSession()`

---

## State Transitions

```
Constructor → run() entry → executeAgentLoop() → finalization → cleanup
                                  ↓
                            error handling
```

**Note**: The session does NOT track an explicit state field. Lifecycle phases are implicit in execution flow.

---

## Events

- `agent_started` - Session began
- `agent_update` - Progress update
- `agent_finished` - Success completion
- `agent_failed` - Error completion

---

## Key Log Events

- `agent:init` - Session initialized
- `agent:settings` - Configuration summary
- `agent:tools` - Tool banner
- `agent:turn-start` - Turn begins
- `agent:final-turn` - Final turn detected
- `agent:context` - Context guard events
- `agent:final-report-accepted` - Final report committed
- `agent:fin` - Session finalized

---

## See Also

- [Architecture](Technical-Specs-Architecture) - Component overview
- [Context Management](Technical-Specs-Context-Management) - Token budgets
- [docs/specs/session-lifecycle.md](../docs/specs/session-lifecycle.md) - Full spec

