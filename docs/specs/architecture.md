# Architecture

## TL;DR
Layered architecture with strict separation: CLI → Session wrapper → Session turn runner → LLM client → Providers. `SessionTurnRunner` owns retries, context guard enforcement, finalization readiness (final report + required META), and router short-circuit.

## Source Files
- `src/ai-agent.ts` - Orchestration wrapper, session factory, top-level run wiring
- `src/session-turn-runner.ts` - Core turn loop (retries, final turns, finalization readiness, router short-circuit)
- `src/final-report-manager.ts` - Final report + plugin META state (lock state and finalization readiness)
- `src/plugins/runtime.ts` - Final-report plugins, META validation, cache gating
- `src/context-guard.ts` - Token budgets, forced-final enforcement, tool budget mutex
- `src/xml-transport.ts` - xml-final transport (FINAL/META extraction, XML-NEXT pairing)
- `src/llm-messages-xml-next.ts` - Model-facing XML-NEXT guidance (FINAL + META, META-only mode)
- `src/llm-messages-turn-failed.ts` - Persistent TURN-FAILED feedback with detailed repair instructions
- `src/llm-client.ts` - Single turn execution (LLMClient)
- `src/cli.ts` - CLI entry point
- `src/index.ts` - Library exports
- `src/types.ts` - Core type definitions

## Core Components

### 0. AIAgent (Orchestration Wrapper)
**Responsibility**: Runs advisors/router/handoff around the inner session loop.

**Lifecycle**:
1. `AIAgent.run(session)` executes orchestration if configured.
2. Calls `AIAgentSession.run()` for the main loop.
3. Applies router delegation and handoff after the main session.

### 1. AIAgentSession (`src/ai-agent.ts`)
**Responsibility**: Multi-turn orchestration, retry logic, context management

**Key State**:
- `conversation: ConversationMessage[]` - Full conversation history
- `logs: LogEntry[]` - Structured log entries
- `accounting: AccountingEntry[]` - Token/cost tracking
- `currentTurn: number` - Current turn index (1-based for action turns)
- `opTree: SessionTreeBuilder` - Hierarchical operation tracking
- `toolsOrchestrator: ToolsOrchestrator` - Tool execution engine
- `llmClient: LLMClient` - LLM request executor
- `finalReportManager: FinalReportManager` - Tracks final report, required plugin META blocks, lock state, and finalization readiness
- `targetContextConfigs` - Per-model context window limits
- `forcedFinalTurnReason` - Context guard enforcement state

**Lifecycle**:
1. `AIAgentSession.create(config)` → Validates config, creates session
2. `session.run()` → Executes agent loop, returns AIAgentResult
3. Turn 0 = system initialization
4. Turns 1..maxTurns = action turns

### 2. LLMClient (`src/llm-client.ts`)
**Responsibility**: Single LLM request execution, response parsing, tracing

**Key Operations**:
- `executeTurn(TurnRequest)` → `TurnResult`
- Provider selection by name
- HTTP fetch tracing (request/response logging)
- Metadata collection (cost, routing, cache stats)
- Pricing computation

**Providers Registered**:
- `openai` → OpenAIProvider
- `anthropic` → AnthropicProvider
- `google` → GoogleProvider
- `openrouter` → OpenRouterProvider
- `ollama` → OllamaProvider
- `test-llm` → TestLLMProvider

### 3. ToolsOrchestrator (`src/tools/tools.ts`)
**Responsibility**: Tool discovery, schema management, execution routing

**Registered Providers**:
- MCPProvider - MCP protocol tools (stdio/websocket/http/sse)
- RestProvider - REST/OpenAPI tools
- InternalToolProvider - Built-in tools (task_status, final_report, batch)
- AgentProvider - Sub-agent invocation
- RouterToolProvider - `router__handoff-to` (registered only when router destinations are configured)

### 4. SessionTreeBuilder (`src/session-tree.ts`)
**Responsibility**: Hierarchical operation tracking (opTree)

**Structure**:
```
Session
├─ Turn 0 (system)
│  ├─ Op: init
│  └─ Op: fin
├─ Turn 1
│  ├─ Op: llm (attempt 1)
│  ├─ Op: tool (call 1)
│  └─ Op: tool (call 2)
└─ Turn N
```

## Data Flow

### Request Flow
```
User Prompt
    ↓
AIAgent.run()
    ↓
(advisors pre-run, if configured)
    ↓
AIAgentSession.run()
    ↓
executeAgentLoop()
    ↓
[Per Turn]
    ↓
LLMClient.executeTurn()
    ↓
Provider.executeTurn()
    ↓
HTTP Request to LLM API
```

### Response Flow
```
LLM API Response
    ↓
Provider parses response
    ↓
TurnResult (status, messages, toolCalls, tokens)
    ↓
AIAgentSession processes result
    ↓
[If toolCalls]
    ↓
ToolsOrchestrator.executeWithManagement()
    ↓
Tool results added to conversation
    ↓
[Next turn or finalize]
```

## Exit Codes
Defined in `src/ai-agent.ts` and emitted by `src/session-turn-runner.ts`.

**Success**:
- `EXIT-FINAL-ANSWER` - Finalization readiness achieved (final report + required META when configured)
- `EXIT-MAX-TURNS-WITH-RESPONSE` - Max turns reached with response
- `EXIT-USER-STOP` - User-initiated graceful stop

**LLM Failures**:
- `EXIT-NO-LLM-RESPONSE` - No response from LLM
- `EXIT-EMPTY-RESPONSE` - Empty response
- `EXIT-AUTH-FAILURE` - Authentication failed
- `EXIT-QUOTA-EXCEEDED` - Quota exceeded
- `EXIT-MODEL-ERROR` - Model error

**Tool Failures**:
- `EXIT-TOOL-FAILURE` - Tool execution failed
- `EXIT-MCP-CONNECTION-LOST` - MCP server disconnected
- `EXIT-TOOL-NOT-AVAILABLE` - Requested tool not found
- `EXIT-TOOL-TIMEOUT` - Tool execution timeout

**Configuration**:
- `EXIT-NO-PROVIDERS` - No providers configured
- `EXIT-INVALID-MODEL` - Invalid model specified
- `EXIT-MCP-INIT-FAILED` - MCP initialization failed

**Timeout/Limits**:
- `EXIT-INACTIVITY-TIMEOUT` - Inactivity timeout
- `EXIT-MAX-RETRIES` - Max retries exhausted
- `EXIT-TOKEN-LIMIT` - Token limit exceeded
- `EXIT-MAX-TURNS-NO-RESPONSE` - Max turns without response

**Unexpected**:
- `EXIT-UNCAUGHT-EXCEPTION` - Uncaught exception
- `EXIT-SIGNAL-RECEIVED` - Signal received
- `EXIT-UNKNOWN` - Unknown error

## Invariants

1. **Session Immutability**: Once created, session config is immutable
2. **Turn Ordering**: Turn 0 is always system init, action turns are 1-based
3. **Provider Isolation**: Each provider handles its own auth/protocol
4. **Context Guard**: Token budget checked before each LLM request
5. **Tool Budget**: Tool output size checked before committing to conversation
6. **Stop Signal Propagation**: `stopRef` with reason ('stop'/'abort'/'shutdown') propagates to every wait loop; 'stop' triggers final turn, 'abort'/'shutdown' trigger AbortSignal
7. **OpTree Consistency**: Every operation has begin/end lifecycle calls

## Business Logic Coverage (Verified 2026-01-25)

- **Finalization readiness (FINAL + required META)**: Success requires both the final report and all required plugin META blocks; missing META locks the final report, triggers META-only retries, and exhaustion can synthesize `reason: "final_meta_missing"` (`src/final-report-manager.ts`, `src/plugins/runtime.ts`, `src/session-turn-runner.ts`).
- **Final-turn tool filtering with router gating**: Final turns keep `agent__final_report` and only allow extra tools (such as router handoff) when configured and the final report is not locked (`src/ai-agent.ts`, `src/session-turn-runner.ts`, `src/llm-messages-xml-next.ts`).
- **TURN-FAILED repair guidance**: Retry feedback is accumulated as TURN-FAILED events and injected as a single user message before each attempt, so the model gets specific repair instructions without duplicate spam (`src/session-turn-runner.ts`, `src/llm-messages-turn-failed.ts`).
- **Context guard + forced final turn**: Per-provider guard evaluation can skip oversized targets or force final turns; enforcement and telemetry live in `ContextGuard`, with runner-level preflight decisions (`src/context-guard.ts`, `src/session-turn-runner.ts`).
- **Rate-limit cycle gating + abort-aware sleep**: The runner sleeps only when all provider/model pairs rate-limit in the same cycle, and every wait uses `sleepWithAbort` to honor cancel/stop signals (`src/session-turn-runner.ts`).
- **Tool-budget mutex**: Tool budget reservations run under a mutex to prevent concurrent tool executions from racing on token projections (`src/context-guard.ts`).
- **Progress + opTree sync**: Structured progress events and opTree snapshots are emitted via `SessionProgressReporter` and `SessionTreeBuilder` (`src/session-progress-reporter.ts`, `src/session-tree.ts`).
- **Sub-agent isolation**: `SubAgentRegistry` preserves trace ancestry and conversation capture for nested sessions (`src/subagent-registry.ts`, `src/session-turn-runner.ts`).
- **Tool failure overrides**: Tool failures can be replaced with curated fallback messages that are later surfaced to the model and final reports (`src/session-tool-executor.ts`, `src/session-turn-runner.ts`).

## Configuration

### Session Config (`AIAgentSessionConfig`)
- `config: Configuration` - Global config (providers, mcpServers, etc.)
- `targets: Array<{provider, model}>` - Model fallback chain
- `systemPrompt: string` - System prompt template
- `userPrompt: string` - User prompt
- `tools: string[]` - Enabled tool names
- `maxTurns: number` - Maximum action turns (default: 10)
- `maxRetries: number` - Max retries per turn (default: 3)
- `temperature?: number` - Sampling temperature
- `topP?: number` - Top-p sampling
- `topK?: number` - Top-k sampling
- `reasoning?: ReasoningLevel` - Extended thinking level
- `reasoningValue?: ProviderReasoningValue | null` - Direct reasoning value
- `stream?: boolean` - Enable streaming
- `llmTimeout?: number` - LLM request timeout (ms)
- `toolTimeout?: number` - Tool execution timeout (ms)
- `toolResponseMaxBytes?: number` - Max tool response size (oversize stored + tool_output handle)
- `toolOutput?: ToolOutputConfigInput` - tool_output module overrides (storeDir ignored; root is `/tmp/ai-agent-<run-hash>`)
- `abortSignal?: AbortSignal` - Cancellation signal
- `stopRef?: { stopping: boolean; reason?: 'stop' | 'abort' | 'shutdown' }` - Stop signal with reason
- `callbacks?: AIAgentEventCallbacks` - Event callbacks
- `subAgents?: PreloadedSubAgent[]` - Preloaded sub-agent definitions
- `agentId?: string` - Agent identifier
- `headendId?: string` - Headend identifier
- `headendWantsProgressUpdates?: boolean` - Enable progress updates
- `telemetryLabels?: Record<string, string>` - Custom telemetry labels
- `outputFormat: OutputFormatId` - Output format identifier
- `renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'sub-agent'` - Render target
- `conversationHistory?: ConversationMessage[]` - Initial conversation
- `expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> }` - Expected output spec
- `maxOutputTokens?: number` - Max output tokens
- `repeatPenalty?: number` - Repeat penalty
- `maxToolCallsPerTurn?: number` - Max tool calls per turn
- `traceLLM?: boolean` - Enable LLM tracing
- `traceMCP?: boolean` - Enable MCP tracing
- `traceSdk?: boolean` - Enable SDK tracing
- `verbose?: boolean` - Verbose logging
- `caching?: CachingMode` - Cache mode
- `initialTitle?: string` - Initial session title
- `mcpInitConcurrency?: number` - MCP init concurrency
- `trace?: { selfId?: string; originId?: string; parentId?: string; callPath?: string; agentPath?: string; turnPath?: string }` - Trace context
- `agentPath?: string` - Agent path in hierarchy
- `turnPathPrefix?: string` - Turn path prefix
- `ancestors?: string[]` - Ancestor agent IDs

### Callbacks (`AIAgentEventCallbacks`)
- `onEvent(type='log')` - Log entry emitted
- `onEvent(type='accounting')` - Accounting entry emitted
- `onEvent(type='progress')` - Progress event emitted
- `onEvent(type='status')` - Progress mirror for `agent_update`
- `onEvent(type='op_tree')` - OpTree updated
- `onEvent(type='turn_started')` - LLM attempt began (includes `turn`, `attempt`, retry/final annotations)
- `onEvent(type='snapshot')` - Session snapshot available
- `onEvent(type='accounting_flush')` - Accounting batch ready
- `onEvent(type='output')` - Text output emitted
- `onEvent(type='thinking')` - Thinking/reasoning text emitted
- `onEvent(type='final_report')` - Final report payload
- `onEvent(type='handoff')` - Handoff payload (non-terminal)

## Telemetry
- OpenTelemetry spans: `agent.session`, turn operations
- Attributes: agent ID, call path, txn IDs, headend ID
- Metrics: turn count, success status, final report status

## Logging
- Severity levels: VRB, WRN, ERR, TRC, THK, FIN
- Direction: request/response
- Type: llm/tool
- Remote identifier format: `agent:<action>` for internal lifecycle logs, `${provider[/actual]}:${model}` for LLM attempts, `kind:namespace:tool` for tool execution (e.g., `mcp:docs:search`), plus `queue:<name>` for queueing diagnostics
- Trace fields: agentId, callPath, txnId, parentTxnId, originTxnId

## Events
- `agent_started` - Session began
- `agent_update` - Progress update
- `agent_finished` - Session completed successfully
- `agent_failed` - Session failed
- `tool_started` - Tool execution began
- `tool_finished` - Tool execution completed

## Test Coverage
- Phase 2 harness: `src/tests/phase2-harness-scenarios/*.ts` - Deterministic scenarios
- Covered: Session creation, turn execution, retry logic, context guard, tool execution
- Gaps: None identified in core orchestration

## Troubleshooting

### Session fails to start
- Check: Provider config validation (`validateProviders`)
- Check: MCP server config validation (`validateMCPServers`)
- Check: Prompt validation (`validatePrompts`)

### Turn retries exhausted
- Check: `maxRetries` setting
- Check: Provider-specific errors in logs
- Check: Rate limit headers in response

### Context limit reached
- Check: `targetContextConfigs` buffer settings
- Check: `contextWindowBufferTokens` per model
- Check: Tool output size caps

### Tools not available
- Check: Tool warmup logs
- Check: MCP server connection status
- Check: Tool allow/deny lists
