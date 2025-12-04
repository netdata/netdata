# AI Agent Library Design

## Core Principle: Complete Session Autonomy

Each AI agent session is a **completely independent universe** with ZERO shared state. Agents running concurrently share absolutely nothing.

## Architecture Overview

### File Organization and Separation of Concerns

**100% SEPARATION OF CONCERNS** with clear responsibilities:

#### ai-agent.ts (High-Level Business Logic)
- AIAgent (Static factory for creating completely isolated sessions)
- AIAgentSession (Autonomous agent with complete isolated state)
- Main agent loop orchestrating turns, retries, and provider switching
- Session state management and immutable operations
- **Does NOT care about LLM details or tool execution mechanics**
- XML transport is now fixed to `xml-final` via `XmlToolTransport` (`src/xml-transport.ts`): XML-NEXT notices carry nonce, slots, and schemas; the model must emit `<ai-agent-NONCE-XXXX tool=\"...\">` tags for the final report while provider tool lists remain native (tool_calls). Progress follows the native path.

#### llm-client.ts (Single Turn Implementation)
- Complete implementation of single LLM turn execution
- Returns clear turn execution status for decision making
- Handles streaming, token counting, message formatting
- Provider-agnostic interface for all LLM operations
- **Does NOT care about multi-turn logic or retry strategies**

#### llm-providers/ (Provider-Specific Logic)
- Directory structure: `llm-providers/{provider}.ts`
- One file per provider: `openai.ts`, `anthropic.ts`, `openrouter.ts`, `ollama.ts`, etc.
- Provider-specific configuration, authentication, and API handling
- Unified interface implemented by all providers
- **Isolated provider logic with no cross-dependencies**

#### mcp-client.ts (Tool Execution Engine)
- Everything needed to execute MCP tools
- Clear tool execution status for each operation
- MCP server management and connection handling
- Tool result processing and error handling
- **Does NOT care about when or why tools are called**

#### cli.ts (Configuration and I/O Management)
- User of the library, not part of core logic
- Configuration file handling and validation
- Command line parameter parsing and environment variables
- Accounting file management and session persistence
- Loading/saving of conversation history
- Log filtering and colored output formatting
- **Pure I/O and configuration management**
- Headend manager (`HeadendManager`) coordinates REST/MCP/OpenAI/Anthropic headends; each headend implements `Headend` with `start/stop/describe`, runs on its own port/transport, and enforces per-headend concurrency via `ConcurrencyLimiter`. JSON formats (`format=json`) require callers to include a `schema` definition, ensuring tool results are validated before reaching clients.

### Clear Status Interfaces

#### Turn Execution Status
Each turn execution returns a clear status enabling decision making:

```typescript
type TurnStatus = 
  | { type: 'success'; hasToolCalls: boolean; finalAnswer: boolean }
  | { type: 'rate_limit'; retryAfterMs?: number }
  | { type: 'auth_error'; message: string }
  | { type: 'model_error'; message: string; retryable: boolean }
  | { type: 'network_error'; message: string; retryable: boolean }
  | { type: 'timeout'; message: string }
  | { type: 'invalid_response'; message: string }
  | { type: 'quota_exceeded'; message: string };

interface TurnResult {
  status: TurnStatus;
  response?: string;
  toolCalls?: ToolCall[];
  tokens?: TokenUsage;
  latencyMs: number;
}
```

#### Tool Execution Status
Each tool execution returns a clear status for error handling:

```typescript
type ToolStatus =
  | { type: 'success' }
  | { type: 'mcp_server_error'; serverName: string; message: string }
  | { type: 'tool_not_found'; toolName: string; serverName: string }
  | { type: 'invalid_parameters'; toolName: string; message: string }
  | { type: 'execution_error'; toolName: string; message: string }
  | { type: 'timeout'; toolName: string; timeoutMs: number }
  | { type: 'connection_error'; serverName: string; message: string };

interface ToolResult {
  toolCallId: string;
  status: ToolStatus;
  result: string;
  latencyMs: number;
  metadata: ToolMetadata;
}
```

### Component Interaction Flow

1. **ai-agent.ts** orchestrates the main loop:
   - Calls **llm-client.ts** for each turn
   - Receives `TurnResult` with clear status
   - Makes decisions on retries/provider switching based on status
   - Calls **mcp-client.ts** for tool execution when needed

2. **llm-client.ts** handles single turns:
   - Uses **llm-providers/** for provider-specific operations
   - Returns structured `TurnResult` with clear status
   - No knowledge of retry logic or session state

3. **mcp-client.ts** executes tools:
   - Returns structured `ToolResult` with clear status
   - Handles all MCP protocol details
   - No knowledge of when or why tools are called

4. **cli.ts** orchestrates everything:
   - Creates sessions with **ai-agent.ts**
   - Handles all I/O, configuration, and presentation
   - Never directly calls **llm-client.ts** or **mcp-client.ts**
   - Headend manager supervises network entry points (REST, MCP stdio/HTTP/SSE/WS, OpenAI tool/completions, Anthropic completions). Each headend registers with `HeadendManager`, shares the common agent registry, and uses per-headend concurrency guards so independent services cannot starve each other.

## Session Isolation Requirements

Each session MUST be completely autonomous:

- ❌ **No shared MCP clients** - Each session initializes its own MCP servers
- ❌ **No shared configurations** - Each session has its own config object
- ❌ **No shared provider instances** - Each session creates its own LLM providers
- ❌ **No shared conversation state** - Each session manages its own messages
- ❌ **No shared accounting/logging** - Each session tracks its own metrics
- ❌ **No shared retry state** - Each session has independent retry logic

## API Design

### Session Creation
```typescript
const session = await AIAgent.create({
  config: {
    providers: { 
      openai: { apiKey: "...", baseUrl: "..." },
      anthropic: { apiKey: "..." }
    },
    mcpServers: {
      filesystem: { type: 'stdio', command: 'mcp-fs' },
      web: { type: 'http', url: 'http://localhost:3000' }
    }
  },
  providers: ['openai', 'anthropic'],
  models: ['gpt-4o', 'claude-3-5-sonnet'],
  tools: ['filesystem', 'web'],
  systemPrompt: 'You are a helpful assistant',
  userPrompt: 'Help me with this task',
  temperature: 0.7,
  maxRetries: 3,
  maxTurns: 30
});
```

### Concurrent Usage
```typescript
const sessions = await Promise.all([
  AIAgent.create({
    config: { providers: { openai: {...} }, mcpServers: { fs: {...} } },
    providers: ['openai'],
    models: ['gpt-4o'],
    tools: ['fs'],
    systemPrompt: 'Code reviewer',
    temperature: 0.2
  }).run(),
  
  AIAgent.create({
    config: { providers: { anthropic: {...} }, mcpServers: { web: {...} } },
    providers: ['anthropic'], 
    models: ['claude-3-5'],
    tools: ['web'],
    systemPrompt: 'Researcher',
    temperature: 0.8
  }).run(),
  
  AIAgent.create({
    config: { providers: { ollama: {...} }, mcpServers: { db: {...} } },
    providers: ['ollama'],
    models: ['llama3.2'],
    tools: ['db'],
    systemPrompt: 'Data analyst',
    temperature: 0.1
  }).run()
]);
```

## Session State Management

### Immutable Sessions
Sessions are immutable after creation. Operations return new session objects:

```typescript
const initialSession = await AIAgent.create(config);
const result = await initialSession.run();

if (!result.success) {
  const retryResult = await result.retry(); // Returns new session
}
```

### Complete State Isolation
Each session contains:

```typescript
class AIAgentSession {
  readonly config: Configuration;           // Session-owned config
  readonly mcpClient: MCPClientManager;     // Session-owned MCP client
  readonly conversation: ConversationMessage[]; // Session conversation
  readonly accounting: AccountingEntry[];   // Session accounting
  readonly currentTurn: number;             // Session turn counter
  readonly success: boolean;                // Session result
  readonly error?: string;                  // Session error
  
  // Session-scoped retry with provider switching
  async retry(): Promise<AIAgentSession>;
}
```

## Retry Logic Design

### Status-Based Retry Logic
The main loop in **ai-agent.ts** uses clear status responses to make retry decisions:

```typescript
for turn in 0..maxTurns:
  for retry in 0..maxRetries:
    for providerModel in pairs:
      const turnResult = await llmClient.executeTurn({
        messages: currentMessages,
        provider: providerModel.provider,
        model: providerModel.model,
        tools: availableTools
      });
      
      switch (turnResult.status.type) {
        case 'success':
          if (turnResult.status.hasToolCalls) {
            // Execute tools and advance to next turn
            const toolResults = await mcpClient.executeTools(turnResult.toolCalls);
            return advanceToNextTurn(toolResults);
          }
          if (turnResult.status.finalAnswer) {
            return success();
          }
          break;
          
        case 'rate_limit':
        case 'network_error':
        case 'timeout':
          // Try next provider/model pair
          continue;
          
        case 'auth_error':
        case 'quota_exceeded':
          // Skip this provider entirely
          break;
          
        case 'model_error':
          if (turnResult.status.retryable) {
            continue; // Try next provider
          } else {
            return failure(turnResult.status.message);
          }
      }
```

### State Preservation Rules
- **Messages**: Accumulated across turns within same session
- **Provider failures**: Don't reset conversation state
- **Retry attempts**: Preserve all progress within the current turn
- **Tool execution**: Only successful tool execution advances turns
- **Status-driven**: All decisions based on clear status responses

### Provider Switching Strategy
1. **Per-turn basis**: Each turn attempts all provider/model pairs
2. **Status-based**: Switching logic based on specific error types
3. **Retry isolation**: Provider failures don't affect other providers
4. **Max attempts**: 3 retries per turn across all combinations
5. **Smart skipping**: Skip providers with auth/quota issues

## Session Lifecycle

1. **Creation**: `AIAgent.create(config)` returns configured session
2. **Execution**: `session.run()` executes with retry logic
3. **Retry**: `session.retry()` returns new session with next provider
4. **Result**: Session contains complete state (messages, accounting, logs)

## Error Handling

### Final Turn Behavior
- Tools disabled on final turn (turn >= maxTurns - 1) and `logEnteringFinalTurn` emits `agent:final-turn` (WRN) so telemetry/alerts can key off the last-turn window.
- Special "no more tools" message is appended and deduped through `pushSystemRetryMessage`.
- `pendingFinalReport` caches any report parsed from assistant text or tool output but defers acceptance until retries are exhausted **and** we are inside the forced final turn. Earlier turns keep retrying instead of short‑circuiting.
- `finalReportAttempts` increments in both the sanitizer (invalid payload dropped) and tool executor (valid call). When either `incompleteFinalReportDetected` or `finalReportAttempts > 0` fires, `maxTurns` collapses to `currentTurn + 1` and we log `agent:orchestrator` plus fire a telemetry counter (see below).
- Fallback acceptance is logged explicitly via `agent:fallback-report` (WRN) indicating whether the cache originated from text extraction or tool-message adoption.
- `logFinalReportAccepted` records the source (`tool-call`, `text-fallback`, `tool-message`, or `synthetic`) so downstream systems can alert on anything other than the happy path.
- If no assistant response arrives even after enforcing the final turn, `logFailureReport` synthesizes a structured `status: failure` report, marks `finalReportSource = 'synthetic'`, and ensures `(no output)` never escapes to headends.

### Retry Exhaustion
- After max retries: return session with error
- Error message indicates specific failure reason
- All progress preserved in returned session

## Output and Thinking Display Requirements

### Output Stream Handling
The agent MUST properly display both content output and thinking/reasoning streams from LLMs:

#### Standard Output (stdout)
- **Content**: All LLM response/content text
- **Formatting**: No color formatting applied
- **Streaming mode**: Real-time display via `onOutput` callback as chunks arrive
- **Non-streaming mode**: Complete response displayed after turn completion

#### Standard Error (stderr)  
- **Content**: All LLM thinking/reasoning text
- **Formatting**: Light gray color (`\x1b[90m`)
- **Streaming mode**: Real-time display via `onThinking` callback as chunks arrive
- **Turn notifications**: `onTurnStarted(turnIndex)` fires whenever the orchestrator begins a new LLM turn so downstream headends can annotate reasoning logs even if no thinking text streams.
- **Provider support**: Only with providers supporting reasoning (e.g., OpenAI o1, Anthropic Claude)

#### Implementation Details
1. **Streaming Response Handling**:
   - Text chunks are passed to callbacks immediately as they arrive
   - `onChunk(chunk, 'content')` → `onOutput(chunk)` → stdout
   - `onChunk(chunk, 'thinking')` → `onThinking(chunk)` → stderr (gray)

2. **Non-Streaming Response Handling**:
   - Complete response buffered during execution
   - Output displayed via `onOutput` callback after turn completion
   - Only if response contains actual content (not just tool calls)

3. **Tool Execution Context**:
   - Tool execution happens WITHIN turns (via AI SDK)
   - Responses after tool use should still be displayed
   - Final answer detection must check LAST assistant message only
   - Don't exit prematurely when tools are involved

## Agent Exit Tracking Requirements

### CRITICAL: Verbose Exit Logging
**Every agent exit MUST include clear verbose logging with unique exit codes for debugging:**

#### Exit Code Categories and Requirements

1. **SUCCESS**: Normal completion
   - `EXIT-FINAL-ANSWER`: Final answer received from LLM
   - `EXIT-MAX-TURNS-WITH-RESPONSE`: Max turns reached with final response
   - `EXIT-USER-STOP`: User-initiated stop signal

2. **LLM FAILURES**: LLM-related terminations
   - `EXIT-NO-LLM-RESPONSE`: No response from LLM after all retries
   - `EXIT-EMPTY-RESPONSE`: Invalid/empty response from all providers
   - `EXIT-AUTH-FAILURE`: All providers failed with auth errors
   - `EXIT-QUOTA-EXCEEDED`: All providers hit quota limits
   - `EXIT-MODEL-ERROR`: Persistent model errors across all providers

3. **TOOL FAILURES**: MCP/Tool-related terminations
   - `EXIT-TOOL-FAILURE`: Critical tool execution failure
   - `EXIT-MCP-CONNECTION-LOST`: MCP server connection lost
   - `EXIT-TOOL-NOT-AVAILABLE`: Required tool not available
   - `EXIT-TOOL-TIMEOUT`: Tool timeout exceeded max threshold

4. **CONFIGURATION**: Config/setup issues
   - `EXIT-NO-PROVIDERS`: No valid providers configured
   - `EXIT-INVALID-MODEL`: Invalid model configuration
   - `EXIT-MCP-INIT-FAILED`: MCP server initialization failed

5. **TIMEOUT/LIMITS**: Resource limits
   - `EXIT-INACTIVITY-TIMEOUT`: Inactivity timeout reached
   - `EXIT-MAX-RETRIES`: Max retries exhausted
   - `EXIT-TOKEN-LIMIT`: Token limit exceeded
   - `EXIT-MAX-TURNS-NO-RESPONSE`: Max turns reached without response

6. **UNEXPECTED**: Unexpected terminations
   - `EXIT-UNCAUGHT-EXCEPTION`: Uncaught exception
   - `EXIT-SIGNAL-RECEIVED`: Process signal received
   - `EXIT-UNKNOWN`: Unknown termination reason

#### Exit Logging Format
```typescript
// Verbose mode MUST log exit with:
[LEVEL] ← [X.X] agent {EXIT-CODE}: {detailed_reason} (fatal={true|false})

// Example outputs:
[VRB] ← [5.0] agent EXIT-FINAL-ANSWER: Final answer received, session complete (fatal=false)
[ERR] ← [3.0] agent EXIT-NO-LLM-RESPONSE: No LLM response after 3 retries across 2 providers (fatal=true)
[ERR] ← [10.0] agent EXIT-MAX-TURNS-NO-RESPONSE: Max turns (10) reached without final response (fatal=true)
[ERR] ← [2.0] agent EXIT-INACTIVITY-TIMEOUT: Inactivity timeout after 300000ms waiting for LLM (fatal=true)
```

#### Implementation Requirements
1. **Every exit path** in the code MUST have a unique exit code
2. **Verbose logging** MUST include the exit code and detailed reason
3. **Exit location** should be traceable from the code (file:line)
4. **Fatal flag** MUST reflect whether the session actually failed (`true` for failures, `false` for clean exits)
5. **Session state** should be preserved up to the exit point

### Final Report Telemetry & Observability
- `recordFinalReportMetrics` (src/telemetry/index.ts) emits `ai_agent_final_report_total`, `ai_agent_final_report_attempts_total`, and `ai_agent_final_report_turns` with labels `source`, `status`, `forced_final_reason`, `headend`, and `synthetic_reason`. Dashboards must alarm whenever `source != 'tool-call'` exceeds historical baselines.
- `recordRetryCollapseMetrics` feeds `ai_agent_retry_collapse_total` (`reason = 'final_report_attempt' | 'incomplete_final_report'`) whenever `maxTurns` collapses early. This was added so we can observe LLM compliance regressions before they impact `(no output)` rates.
- `agent:turn-start`, `agent:text-extraction`, `agent:fallback-report`, `agent:final-report-accepted`, and `agent:failure-report` logs are part of the lifecycle contract and must remain documented; follow-on tooling consumes them to annotate live headends and page SREs when synthetic failures spike.

## Implementation Notes

### Component Boundaries and Responsibilities

#### ai-agent.ts Implementation
- **AIAgent.create()**: Static factory creating isolated sessions
- **AIAgentSession**: Immutable session objects with state isolation
- **Main loop**: Status-based orchestration using clear interfaces
- **Session management**: Turn counting, retry logic, provider switching
- **No direct LLM/MCP calls**: All operations via **llm-client.ts** and **mcp-client.ts**

#### llm-client.ts Implementation  
- **Single interface**: `executeTurn(TurnRequest): Promise<TurnResult>`
- **Provider delegation**: Uses **llm-providers/** for actual API calls
- **Status mapping**: Converts provider errors to clear `TurnStatus` types
- **Token accounting**: Standardized token counting across all providers
- **Streaming support**: Handles streaming responses uniformly

#### llm-providers/ Implementation
- **Unified interface**: All providers implement same contract
- **Provider isolation**: No shared state between provider implementations
- **Configuration**: Each provider handles its own auth/config requirements
- **Error mapping**: Provider-specific error handling to standard status types
- **API compatibility**: Handles provider-specific API differences

#### mcp-client.ts Implementation
- **Per-session isolation**: Each session creates its own MCPClientManager
- **Clear status responses**: All operations return structured `ToolResult`
- **Connection lifecycle**: Independent MCP server connections per session
- **Error categorization**: Maps MCP errors to clear `ToolStatus` types
- **No tool logic**: Pure execution engine, no business logic

### Concurrency and Isolation
- **Zero shared state**: All components operate on isolated session data
- **No singletons**: Every resource is session-owned
- **Thread safety**: No locks needed due to isolation design
- **Resource cleanup**: Each session owns and cleans up its resources
- **Provider independence**: Concurrent sessions can use different providers

## Output and Logging Design

### Core Principle: Library is Silent
The library produces NO output:
- ❌ No console.log/error/warn
- ❌ No file writes  
- ❌ No stderr output
- ❌ No side effects
- ✅ Pure callback-driven communication

### Structured Logging System

All library activity is logged through structured log entries:

```typescript
interface LogEntry {
  timestamp: number;                    // Unix timestamp (ms)
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC'; // Log severity level
  turn: number;                         // Sequential turn ID  
  subturn: number;                      // Sequential tool ID within turn
  direction: 'request' | 'response';    // Request or response
  type: 'llm' | 'mcp';                 // Operation type
  remoteIdentifier: string;             // 'provider:model' (LLM) or 'protocol:namespace:tool'
  fatal: boolean;                       // True if this caused agent to stop
  message: string;                      // Human readable message
}
```

### Log Severity Rules

1. **VRB (Verbose)**: Exactly one per request/response pair
   - Request logged immediately when operation starts (gives user instant feedback)
   - Response logged when operation completes (provides results/timing)
   - Shows the flow of operations
   - Used by CLI only with `--verbose` flag
   - Displayed in dark grey to stderr

2. **WRN (Warning)**: Non-fatal issues
   - Always displayed by CLI in yellow
   - Retry attempts, recoverable errors

3. **ERR (Error)**: Fatal failures  
   - Always displayed by CLI in red
   - Causes agent to stop (fatal=true)

4. **TRC (Trace)**: Detailed debugging info
   - Always generated, filtered by CLI
   - Shown only with `--trace-llm` or `--trace-mcp`
   - Full request/response payloads

### Session Log Management

```typescript
class AIAgentSession {
  readonly logs: LogEntry[] = [];       // Complete chronological log
  private currentTurn = 0;
  private currentSubturn = 0;
  
  private log(
    severity: LogEntry['severity'],
    direction: LogEntry['direction'], 
    type: LogEntry['type'],
    remoteIdentifier: string,
    message: string,
    fatal = false
  ) {
    const entry: LogEntry = {
      timestamp: Date.now(),
      severity, turn: this.currentTurn, subturn: this.currentSubturn,
      direction, type, remoteIdentifier, fatal, message
    };
    
    this.logs.push(entry);              // Store in session
    this.callbacks.onLog?.(entry);      // Real-time callback
  }
}
```

### Real-Time Callbacks

```typescript
interface AIAgentCallbacks {
  onLog?: (entry: LogEntry) => void;    // Structured log streaming
}

const session = await AIAgent.create({
  // ... config
  callbacks: {
    onLog: (entry) => {
      // CLI filters and formats based on severity and flags
      if (entry.severity === 'ERR') {
        console.error(`[ERR] ${entry.direction} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`);
      }
      if (entry.severity === 'VRB' && options.verbose) {
        console.error(`[VRB] ${entry.direction} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`);
      }
    }
  }
});
```

### Verbose Message Format Requirements

**VRB (Verbose) messages must contain specific operational details:**

1. **LLM Requests**: Total payload size in bytes (not JSON)
   - Format: `messages {count}, {total_bytes} bytes`
   - Example: `[VRB] request [1.0] llm openai:gpt-4o: messages 3, 1247 bytes`

2. **LLM Responses**: Full accounting with tokens, latency, and size  
   - Format: `input {tokens}, output {tokens} tokens, {latency}ms, {bytes} bytes`
   - Example: `[VRB] response [1.0] llm openai:gpt-4o: input 128, output 45 tokens, 1834ms, 156 bytes`

3. **MCP Requests**: Compact parameter format
   - Format: `{tool_name}({field1}:{value1}, {field2}:{value2})`
   - Example: `[VRB] request [1.1] mcp filesystem:read_file: read_file(path:/etc/hosts)`

4. **MCP Responses**: Latency and result size
   - Format: `{latency}ms, {result_size} chars`
   - Example: `[VRB] response [1.1] mcp filesystem:read_file: 12ms, 340 chars`

### Example Log Flow

```typescript
// Turn 1: LLM call with tool execution
[VRB] → [1.0] llm openai:gpt-4o: messages 3, 1247 bytes          ← Immediate feedback
[TRC] → [1.0] llm openai:gpt-4o: request: {"messages":[...], "tools":[...]}
... (user waits, knowing LLM is being called) ...
[VRB] ← [1.0] llm openai:gpt-4o: input 128, output 45 tokens, 1834ms, 156 bytes
[TRC] ← [1.0] llm openai:gpt-4o: response: {"choices":[...]}

[VRB] → [1.1] mcp filesystem:read_file: read_file(path:/etc/hosts)           ← Immediate feedback
[TRC] → [1.1] mcp filesystem:read_file: params: {"path":"/etc/hosts"}
... (user waits, knowing tool is executing) ...
[VRB] ← [1.1] mcp filesystem:read_file: 12ms, 340 chars
[TRC] ← [1.1] mcp filesystem:read_file: result: "127.0.0.1 localhost..."

// Turn 2: Final response
[VRB] → [2.0] llm openai:gpt-4o: messages 5, 2089 bytes (final turn) ← Immediate feedback
... (user waits for final response) ...
[VRB] ← [2.0] llm openai:gpt-4o: input 246, output 67 tokens, 2156ms, 234 bytes

// Error example
[VRB] → [3.0] llm anthropic:claude-3-5-sonnet: messages 7, 3421 bytes ← Immediate feedback
... (user waits, then gets error) ...
[ERR] ← [3.0] llm anthropic:claude-3-5-sonnet: RateLimitError: rate limit exceeded (fatal=true)
```

### Log Analysis

Complete session logs enable post-execution analysis:

```typescript
const result = await session.run();

// Find all errors
const errors = result.logs.filter(l => l.severity === 'ERR');

// Count tool executions
const toolCount = result.logs.filter(l => l.type === 'mcp' && l.direction === 'request').length;

// Calculate total latency
const totalTime = result.logs[result.logs.length - 1].timestamp - result.logs[0].timestamp;

// Find retry patterns
const retries = result.logs.filter(l => l.severity === 'WRN').length;
```

## CLI Integration

### CLI Logging Color Requirements

**All logs must be output exclusively to stderr with mandatory TTY coloring:**

- **All logs MUST be colored when stderr is a TTY** - never output uncolored text to prevent confusion with LLM output
- **LLM output (stdout)**: White (default terminal color)
- **LLM thinking**: Light gray (`\x1b[37m`)
- **Default log color**: Dark gray (`\x1b[90m`) 
- **Warnings (WRN)**: Yellow (`\x1b[33m`)
- **Errors (ERR)**: Red (`\x1b[31m`)
- **Traces (TRC)**: Dark gray (`\x1b[90m`) - same as default
- **Verbose (VRB)**: Dark gray (`\x1b[90m`) - same as default

CLI creates single session per invocation with log filtering:

```typescript
const callbacks = {
  onLog: (entry: LogEntry) => {
    // Helper for consistent coloring - MANDATORY in TTY
    const colorize = (text: string, colorCode: string): string => {
      return process.stderr.isTTY ? `${colorCode}${text}\x1b[0m` : text;
    };
    
    // Helper for direction symbols to save space
    const dirSymbol = (direction: string): string => direction === 'request' ? '→' : '←';
    
    // Always show errors and warnings with colors
    if (entry.severity === 'ERR') {
      const formatted = colorize(`[ERR] ${dirSymbol(entry.direction)} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[31m');
      process.stderr.write(`${formatted}\n`);
    }
    
    if (entry.severity === 'WRN') {
      const formatted = colorize(`[WRN] ${dirSymbol(entry.direction)} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[33m');
      process.stderr.write(`${formatted}\n`);
    }
    
    // Show verbose only with --verbose flag (dark gray)
    if (entry.severity === 'VRB' && options.verbose) {
      const formatted = colorize(`[VRB] ${dirSymbol(entry.direction)} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[90m');
      process.stderr.write(`${formatted}\n`);
    }
    
    // Show trace only with specific flags (dark gray)
    if (entry.severity === 'TRC') {
      if ((entry.type === 'llm' && options.traceLlm) || 
          (entry.type === 'mcp' && options.traceMcp)) {
        const formatted = colorize(`[TRC] ${dirSymbol(entry.direction)} [${entry.turn}.${entry.subturn}] ${entry.type} ${entry.remoteIdentifier}: ${entry.message}`, '\x1b[90m');
        process.stderr.write(`${formatted}\n`);
      }
    }
  }
};

const session = await AIAgent.create({
  config: loadConfiguration(configPath),
  providers: parseProviders(args),
  models: parseModels(args), 
  tools: parseTools(args),
  systemPrompt: resolvedSystem,
  userPrompt: resolvedUser,
  callbacks
});

const result = await session.run();
// result.logs contains complete session log history
```

## Benefits

1. **True Concurrency**: Unlimited concurrent agents with zero interference
2. **Flexible Configuration**: Each session can have completely different settings
3. **State Isolation**: No shared state bugs or race conditions
4. **Independent Lifecycles**: Sessions don't affect each other
5. **Retry Resilience**: Provider failures don't impact other sessions
6. **Resource Management**: Each session owns its resources completely
7. **Complete Observability**: Structured logging with real-time callbacks
8. **Library Purity**: No side effects, all output via callbacks
