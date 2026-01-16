# Architecture

Layered architecture with strict separation of concerns.

---

## TL;DR

CLI → Session → LLM Client → Providers. Session orchestrates turns, LLM Client executes single requests, Providers handle protocol.

---

## Core Components

### 0. AIAgent (Orchestration Wrapper)

**Responsibility**: Runs advisors/router/handoff around the inner session loop.

**File**: `src/ai-agent.ts`

**Lifecycle**:
1. `AIAgent.run(session)` executes orchestration if configured
2. Calls `AIAgentSession.run()` for the main loop
3. Applies router delegation and handoff after the main session

---

### 1. AIAgentSession

**Responsibility**: Multi-turn orchestration, retry logic, context management

**File**: `src/ai-agent.ts`

**Key State**:
- `conversation` - Full conversation history
- `logs` - Structured log entries
- `accounting` - Token/cost tracking
- `currentTurn` - Current turn index (1-based for action turns)
- `opTree` - Hierarchical operation tracking
- `toolsOrchestrator` - Tool execution engine
- `llmClient` - LLM request executor
- `finalReport` - Captured final_report tool result

---

### 2. LLMClient

**Responsibility**: Single LLM request execution, response parsing, tracing

**File**: `src/llm-client.ts`

**Key Operations**:
- `executeTurn(TurnRequest)` → `TurnResult`
- Provider selection by name
- HTTP fetch tracing
- Metadata collection (cost, routing, cache stats)
- Pricing computation

**Registered Providers**:
- `openai` → OpenAIProvider
- `anthropic` → AnthropicProvider
- `google` → GoogleProvider
- `openrouter` → OpenRouterProvider
- `ollama` → OllamaProvider

---

### 3. ToolsOrchestrator

**Responsibility**: Tool discovery, schema management, execution routing

**File**: `src/tools/tools.ts`

**Providers**:
- MCPProvider - MCP protocol tools
- RestProvider - REST/OpenAPI tools
- InternalToolProvider - Built-in tools
- AgentProvider - Sub-agent invocation
- RouterToolProvider - Router delegation

---

### 4. SessionTreeBuilder

**Responsibility**: Hierarchical operation tracking (opTree)

**File**: `src/session-tree.ts`

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

---

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

---

## Exit Codes

### Success
- `EXIT-FINAL-ANSWER` - Agent called final_report
- `EXIT-MAX-TURNS-WITH-RESPONSE` - Max turns reached with response
- `EXIT-USER-STOP` - User-initiated graceful stop

### LLM Failures
- `EXIT-NO-LLM-RESPONSE` - No response from LLM
- `EXIT-EMPTY-RESPONSE` - Empty response
- `EXIT-AUTH-FAILURE` - Authentication failed
- `EXIT-QUOTA-EXCEEDED` - Quota exceeded
- `EXIT-MODEL-ERROR` - Model error

### Tool Failures
- `EXIT-TOOL-FAILURE` - Tool execution failed
- `EXIT-MCP-CONNECTION-LOST` - MCP server disconnected
- `EXIT-TOOL-NOT-AVAILABLE` - Requested tool not found
- `EXIT-TOOL-TIMEOUT` - Tool execution timeout

### Configuration
- `EXIT-NO-PROVIDERS` - No providers configured
- `EXIT-INVALID-MODEL` - Invalid model specified
- `EXIT-MCP-INIT-FAILED` - MCP initialization failed

### Timeout/Limits
- `EXIT-INACTIVITY-TIMEOUT` - Inactivity timeout
- `EXIT-MAX-RETRIES` - Max retries exhausted
- `EXIT-TOKEN-LIMIT` - Token limit exceeded
- `EXIT-MAX-TURNS-NO-RESPONSE` - Max turns without response

---

## Session Configuration

Key configuration options:

| Setting | Description |
|---------|-------------|
| `targets` | Model fallback chain |
| `systemPrompt` | System prompt template |
| `maxTurns` | Maximum action turns |
| `maxRetries` | Max retries per turn |
| `toolTimeout` | Tool execution timeout |
| `toolResponseMaxBytes` | Max tool response size |
| `abortSignal` | Cancellation signal |
| `callbacks` | Event callbacks |

---

## See Also

- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - Detailed session flow
- [Tool System](Technical-Specs-Tool-System) - Tool internals
- [docs/specs/architecture.md](../docs/specs/architecture.md) - Full spec

