# Library API

## TL;DR
Programmatic API for embedding ai-agent in applications with session factory pattern, callback-driven execution, and typed configuration/result contracts.

## Source Files
- `src/index.ts` - Library entry point (33 lines)
- `src/ai-agent.ts` - AIAgentSession class (4440+ lines)
- `src/types.ts` - Type definitions
- `src/llm-client.ts` - LLM client
- `package.json` - Entry point configuration

## Package Entry

**package.json**:
```json
{
  "main": "dist/index.js",
  "types": "dist/index.d.ts"
}
```

## Exports

**Location**: `src/index.ts`

### Class Exports
```typescript
export { AIAgent, AIAgentSession } from './ai-agent.js';
export { LLMClient } from './llm-client.js';
```

`AIAgent` is a wrapper class that runs orchestration (advisors/router/handoff) around the inner session loop.

### Type Exports
```typescript
export type {
  AIAgentSessionConfig,
  AIAgentResult,
  AIAgentCallbacks,
  AIAgentOptions,
  AIAgentRunOptions,
  Configuration,
  ProviderConfig,
  MCPServerConfig,
  MCPTool,
  MCPServer,
  ConversationMessage,
  ToolCall,
  ToolResult,
  TokenUsage,
  TurnRequest,
  TurnResult,
  TurnStatus,
  ToolStatus,
  LogEntry,
  AccountingEntry,
  LLMAccountingEntry,
  ToolAccountingEntry,
  LLMProvider,
  ToolChoiceMode,
};
```

## Core API

### Session Creation

**Location**: `src/ai-agent.ts:770-837`

```typescript
import { AIAgent, type AIAgentSessionConfig } from 'ai-agent-claude';

const session = AIAgent.create(config);
const result = await AIAgent.run(session);
```

Static factory method validates and enriches configuration.

### Factory Flow
```typescript
static create(sessionConfig: AIAgentSessionConfig): AIAgentSession {
  // 1. Validate configuration
  validateProviders(sessionConfig.config, sessionConfig.targets.map(t => t.provider));
  validateMCPServers(sessionConfig.config, sessionConfig.tools);
  validatePrompts(sessionConfig.systemPrompt, sessionConfig.userPrompt);

  // 2. Generate trace context
  const sessionTxnId = crypto.randomUUID();
  const enrichedSessionConfig = {
    ...sessionConfig,
    trace: {
      selfId: sessionTxnId,
      originId: sessionConfig.trace?.originId ?? sessionTxnId,
      parentId: sessionConfig.trace?.parentId,
      // ...
    },
  };

  // 3. Create LLM client
  const llmClient = new LLMClient(enrichedSessionConfig.config.providers, {
    traceLLM: enrichedSessionConfig.traceLLM,
    onLog: wrapLog(enrichedSessionConfig.callbacks?.onLog),
    pricing: enrichedSessionConfig.config.pricing,
  });

  // 4. Instantiate session
  return new AIAgentSession(/* ... */);
}
```

## AIAgentSessionConfig

**Location**: `src/types.ts:543-597`

```typescript
interface AIAgentSessionConfig {
  // Required
  config: Configuration;                    // Full app configuration
  targets: { provider: string; model: string }[];  // LLM targets
  tools: string[];                          // Tool providers to use
  systemPrompt: string;                     // System prompt
  userPrompt: string;                       // User prompt
  cacheTtlMs?: number;                      // Response cache TTL (ms; 0/off disables)
  agentHash?: string;                       // Stable agent hash for cache keys
  outputFormat: OutputFormatId;             // Output format

  // Optional identity
  agentId?: string;                         // Agent identifier
  headendId?: string;                       // Headend identifier
  telemetryLabels?: Record<string, string>; // Custom telemetry labels

  // Optional sub-agents
  subAgents?: PreloadedSubAgent[];          // Preloaded sub-agent definitions

  // Optional conversation
  conversationHistory?: ConversationMessage[];  // Pre-existing conversation

  // Optional output contract
  expectedOutput?: {
    format: 'json' | 'markdown' | 'text';
    schema?: Record<string, unknown>;
  };

  // Optional LLM parameters
  temperature?: number | null;              // Sampling temperature
  topP?: number | null;                     // Top-p sampling
  topK?: number | null;                     // Top-k sampling
  maxOutputTokens?: number;                 // Max output tokens
  repeatPenalty?: number | null;            // Frequency penalty

  // Optional limits
  maxRetries?: number;                      // Retry limit
  maxTurns?: number;                        // Max LLM turns
  maxToolCallsPerTurn?: number;             // Max tool calls per turn

  // Optional timeouts
  llmTimeout?: number;                      // LLM timeout (ms; frontmatter/CLI/config also accept duration strings)
  toolTimeout?: number;                     // Tool timeout (ms; frontmatter/CLI/config also accept duration strings)

  // Optional behavior
  stream?: boolean;                         // Enable streaming
  reasoning?: ReasoningLevel;               // Reasoning level
  reasoningValue?: ProviderReasoningValue | null;  // Reasoning budget
  caching?: CachingMode;                    // Cache control

  // Optional tracing
  traceLLM?: boolean;                       // Trace LLM calls
  traceMCP?: boolean;                       // Trace MCP calls
  traceSdk?: boolean;                       // Trace SDK calls
  verbose?: boolean;                        // Verbose logging

  // Optional callbacks
  callbacks?: AIAgentCallbacks;             // Event callbacks

  // Optional trace context
  trace?: {
    selfId?: string;
    originId?: string;
    parentId?: string;
    callPath?: string;
    agentPath?: string;
    turnPath?: string;
  };

  // Optional cancellation
  abortSignal?: AbortSignal;                // Abort signal
  stopRef?: { stopping: boolean };          // Graceful stop reference

  // Optional miscellaneous
  headendWantsProgressUpdates?: boolean;    // Enable progress updates
  renderTarget?: 'cli' | 'slack' | 'api' | 'web' | 'embed' | 'sub-agent';
  initialTitle?: string;                    // Pre-set session title
  toolResponseMaxBytes?: number;            // Max tool response size
  mcpInitConcurrency?: number;              // MCP init concurrency
  ancestors?: string[];                     // Ancestor chain (recursion prevention)
  agentPath?: string;                       // Agent path override
  turnPathPrefix?: string;                  // Turn path prefix
  stream?: boolean;                         // Override stream mode for this run
}
```

## AIAgentCallbacks

**Location**: `src/types.ts:530-541`

```typescript
interface AIAgentCallbacks {
  onLog?: (entry: LogEntry) => void;
  onOutput?: (text: string) => void;
  onThinking?: (text: string) => void;
  onTurnStarted?: (turn: number) => void;
  onAccounting?: (entry: AccountingEntry) => void;
  onSessionSnapshot?: (payload: SessionSnapshotPayload) => void | Promise<void>;
  onAccountingFlush?: (payload: AccountingFlushPayload) => void | Promise<void>;
  onProgress?: (event: ProgressEvent) => void;
  onOpTree?: (tree: unknown) => void;
}
```

### Callback Purposes

| Callback | Purpose |
|----------|---------|
| `onLog` | Receive all log entries |
| `onOutput` | Stream final output text |
| `onThinking` | Stream reasoning content |
| `onTurnStarted` | Notification of new LLM turn |
| `onAccounting` | Receive accounting entries |
| `onSessionSnapshot` | Full session state snapshots |
| `onAccountingFlush` | Batch accounting entries |
| `onProgress` | Progress events; `agent_update` includes structured `taskStatus` when emitted via `agent__task_status` |
| `onOpTree` | Hierarchical operation tree updates |

## AIAgentResult

**Location**: `src/types.ts:600-631`

```typescript
interface AIAgentResult {
  success: boolean;                         // Session completed successfully
  error?: string;                           // Error message if failed
  conversation: ConversationMessage[];      // Full conversation history
  logs: LogEntry[];                         // All log entries
  accounting: AccountingEntry[];            // All accounting entries

  // Optional execution tree
  treeAscii?: string;                       // ASCII representation of tree
  opTreeAscii?: string;                     // ASCII of hierarchical ops
  opTree?: SessionNode;                     // Structured operation tree

  // Optional sub-agent results
  childConversations?: {
    agentId?: string;
    toolName: string;
    promptPath: string;
    trace?: {
      originId?: string;
      parentId?: string;
      selfId?: string;
      callPath?: string;
    };
    conversation: ConversationMessage[];
  }[];

  // Optional final report
  finalReport?: {
    format: 'json' | 'markdown' | 'markdown+mermaid' | 'slack-block-kit' | 'tty' | 'pipe' | 'sub-agent' | 'text';
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}
```

## Usage Example

```typescript
import { AIAgent } from 'ai-agent-claude';
import type { AIAgentSessionConfig, Configuration } from 'ai-agent-claude';
import { loadConfiguration } from './config.js';

async function main() {
  // Load configuration
  const config: Configuration = loadConfiguration('./.ai-agent.json');

  // Create session config
  const sessionConfig: AIAgentSessionConfig = {
    config,
    targets: [{ provider: 'anthropic', model: 'claude-sonnet-4-20250514' }],
    tools: ['filesystem', 'git'],
    systemPrompt: 'You are a helpful coding assistant.',
    userPrompt: 'List all TypeScript files in the src directory.',
    outputFormat: 'markdown',
    maxTurns: 20,
    llmTimeout: 120000,
    toolTimeout: 30000,
    stream: true,
    callbacks: {
      onOutput: (text) => process.stdout.write(text),
      onThinking: (text) => console.log(`[Thinking] ${text}`),
      onTurnStarted: (turn) => console.log(`\n--- Turn ${turn} ---`),
      onLog: (entry) => {
        if (entry.severity === 'ERR') console.error(entry.message);
      },
      onAccounting: (entry) => {
        if (entry.type === 'llm') {
          console.log(`LLM: ${entry.tokens.inputTokens}/${entry.tokens.outputTokens} tokens`);
        }
      },
    },
  };

  // Create and run session
  const session = AIAgent.create(sessionConfig);
  const result = await AIAgent.run(session);

  // Process result
  if (result.success) {
    console.log('\nSession completed successfully');
    if (result.finalReport) {
      console.log(`Final report content: ${result.finalReport?.content ?? ''}`);
    }
  } else {
    console.error(`Session failed: ${result.error}`);
  }

  // Token usage summary
  const totalTokens = result.accounting
    .filter(e => e.type === 'llm')
    .reduce((sum, e) => sum + e.tokens.totalTokens, 0);
  console.log(`Total tokens used: ${totalTokens}`);
}

main().catch(console.error);
```

## Advanced Usage

### With Sub-Agents

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  subAgents: [
    {
      toolName: 'code_reviewer',
      description: 'Reviews code for best practices',
      promptPath: '/path/to/reviewer.ai',
      systemPrompt: '...',
      expectedOutput: { format: 'json', schema: { /* ... */ } },
      // ... other options
    },
  ],
};
```

### With Abort Signal

```typescript
const controller = new AbortController();

const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  abortSignal: controller.signal,
};

// Cancel session after 5 minutes
setTimeout(() => controller.abort(), 300000);

const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);
```

### With Trace Context

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  trace: {
    originId: 'root-transaction-id',
    parentId: 'parent-agent-id',
    selfId: crypto.randomUUID(),
    callPath: 'root>child',
    agentPath: 'child-agent',
  },
};
```

### With Custom Telemetry Labels

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  headendId: 'slack-bot',
  telemetryLabels: {
    environment: 'production',
    customer: 'acme-corp',
    feature: 'code-review',
  },
};
```

## Session Lifecycle

1. **Creation**: `AIAgent.create(config)`
   - Validates providers, MCP servers, prompts
   - Generates transaction ID
   - Creates LLM client
   - Initializes session state

2. **Execution**: `AIAgent.run(session)` (applies orchestration when configured)
   - Prepares system/user prompts
   - Initializes MCP servers
   - Enters turn loop
   - Processes tool calls
   - Manages context window
   - Invokes callbacks
   - Returns result

3. **Cleanup**:
   - MCP servers disconnected
   - Resources released
   - Final accounting computed

## Read-Only Properties

After creation, session exposes read-only state:

```typescript
class AIAgentSession {
  readonly config: Configuration;
  readonly conversation: ConversationMessage[];
  readonly logs: LogEntry[];
  readonly accounting: AccountingEntry[];
  readonly success: boolean;
  readonly error?: string;
  get currentTurn(): number;
}
```

## Helper Methods

### getExecutionSnapshot

```typescript
session.getExecutionSnapshot(): { logs: number; accounting: number }
```

Returns current log and accounting entry counts.

## Validation

Factory method performs validation:

```typescript
validateProviders(config, targets.map(t => t.provider));
validateMCPServers(config, tools);
validatePrompts(systemPrompt, userPrompt);
```

Throws on:
- Unknown provider names
- Unknown MCP server names
- Both prompts using stdin (-)

## Error Handling

### Creation Errors
- Configuration validation failures
- Missing required fields
- Invalid provider/tool references

### Runtime Errors
- LLM API failures
- Tool execution errors
- Context window exceeded
- Timeout reached
- Abort signal triggered

All errors populate `result.error` and set `result.success = false`.

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `config` | Base configuration (providers, servers, pricing) |
| `targets` | LLM provider/model selection |
| `tools` | Available tool providers |
| `callbacks` | Event notification hooks |
| `maxTurns` | Turn limit before forced stop |
| `stream` | Enable incremental output |
| `abortSignal` | External cancellation control |

## Business Logic Coverage (Verified 2025-11-16)

- **Result contract**: `run()` never throws for operational failures; it always resolves with `AIAgentResult` where `success=false` and `error` describes the failure, so embedders can rely on promise resolution semantics (`src/ai-agent.ts:1037-1388`).
- **Callback isolation**: Callback exceptions (onLog/onOutput/onThinking/onAccounting/onOpTree/onSessionSnapshot) are caught and logged but do not crash the session, preventing user-provided handlers from destabilizing the run (`src/ai-agent.ts:330-410`).
- **Final report exposure**: Regardless of streaming output, the result always includes `finalReport` with status/format/content so embedders can display the canonical answer without parsing streamed text (`src/ai-agent.ts:189-210`).
- **Snapshot hooks**: `callbacks.onSessionSnapshot` and `onAccountingFlush` fire even on failure, enabling embedders to persist diagnostics or billing for crashed sessions (`src/ai-agent.ts:368-408`).
## Invariants

1. **Factory pattern**: Private constructor, use `create()`
2. **Immutable result**: Result is defensive copy
3. **Single run**: Each session runs once (use `AIAgent.run`, not `session.run`, unless you intentionally want to skip orchestration)
4. **Validation first**: All validation before session creation
5. **Trace enrichment**: Transaction IDs auto-generated if missing
6. **Callback isolation**: Callback errors don't crash session

## Test Coverage

**Phase 2**:
- Session creation and validation
- Configuration passing
- Callback invocation
- Result structure
- Error propagation

**Gaps**:
- Memory leak detection
- Long-running session stability
- Concurrent session limits
- Callback exception handling
- Resource cleanup verification

## Troubleshooting

### Import errors
- Check package.json main/types paths
- Verify build output in dist/
- Ensure module resolution matches ("module": "NodeNext")

### Type mismatches
- Regenerate types with `npm run build`
- Check TypeScript version compatibility
- Verify Configuration matches schema

### Callback not firing
- Check callback property name spelling
- Verify not swallowed by try/catch
- Review callback registration before create()

### Session hangs
- Check abortSignal wiring
- Verify maxTurns limit
- Review tool timeouts
- Monitor LLM response times

### Missing results
- Check result.success flag
- Review result.error message
- Examine result.logs for details
- Check accounting entries
