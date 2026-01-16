# Internal API

Library embedding API for programmatic use of ai-agent in Node.js applications.

---

## Table of Contents

- [Overview](#overview) - What the internal API provides
- [Quick Start](#quick-start) - Minimal working example
- [Core Methods](#core-methods) - Main API entry points
- [Session Configuration](#session-configuration) - All configuration options
- [Event Callbacks](#event-callbacks) - Real-time event handling
- [Result Structure](#result-structure) - Understanding session results
- [Event Types Reference](#event-types-reference) - All event types
- [Accounting Records](#accounting-records) - Usage tracking
- [Output Format Contract](#output-format-contract) - Structured output
- [Library Guarantees](#library-guarantees) - What embedders can rely on
- [Notes for Embedders](#notes-for-embedders) - Important implementation details
- [See Also](#see-also) - Related documentation

---

## Overview

The AI Agent library can be embedded in Node.js applications without using the CLI. This enables:

- **No CLI dependency**: Direct programmatic control
- **Real-time events**: Callbacks for streaming, logs, and accounting
- **Custom UI integration**: Build web interfaces, bots, or custom applications
- **Batch processing**: Run agents programmatically at scale

**Entry Point**: `src/ai-agent.ts` (exported as `AIAgent`)

---

## Quick Start

```typescript
import { AIAgent } from "ai-agent";

// Configuration for providers
const config = {
  providers: {
    openai: {
      apiKey: process.env.OPENAI_API_KEY,
      type: "openai",
    },
  },
  mcpServers: {},
};

// Collect results
const logs = [];
const accounting = [];
let output = "";

// Session configuration
const sessionConfig = {
  config,
  targets: [{ provider: "openai", model: "gpt-4o-mini" }],
  tools: [],
  systemPrompt: "You are a helpful assistant.",
  userPrompt: "Say hello.",
  outputFormat: "markdown",
  maxRetries: 3,
  maxTurns: 5,
  callbacks: {
    onEvent: (event, meta) => {
      if (event.type === "log") logs.push(event.entry);
      if (event.type === "accounting") accounting.push(event.entry);
      if (event.type === "output" && meta.source !== "finalize") {
        output += event.text;
      }
    },
  },
};

// Run the agent
const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);

console.log("Success:", result.success);
console.log("Output:", output);
console.log("Total logs:", logs.length);
console.log("Total accounting entries:", accounting.length);
```

---

## Core Methods

### AIAgent.create()

| Property    | Value                                 |
| ----------- | ------------------------------------- |
| Parameters  | `sessionConfig: AIAgentSessionConfig` |
| Returns     | `AIAgentSession`                      |
| Description | Creates a new session instance        |

```typescript
const session = AIAgent.create(sessionConfig);
```

---

### AIAgent.run()

| Property    | Value                                                            |
| ----------- | ---------------------------------------------------------------- |
| Parameters  | `session: AIAgentSession`                                        |
| Returns     | `Promise<AIAgentResult>`                                         |
| Description | Runs session with full orchestration (advisors, router, handoff) |

```typescript
const result = await AIAgent.run(session);
```

---

### session.run()

| Property    | Value                                          |
| ----------- | ---------------------------------------------- |
| Parameters  | None                                           |
| Returns     | `Promise<AIAgentResult>`                       |
| Description | Runs inner session directly (no orchestration) |

```typescript
const result = await session.run();
```

---

## Session Configuration

### AIAgentSessionConfig

```typescript
interface AIAgentSessionConfig {
  // Required
  config: Configuration; // Provider and MCP server config
  targets: { provider: string; model: string }[]; // Provider/model pairs
  tools: string[]; // Tool names to expose (MCP server names and REST tool names; internal tools and sub-agents are not listed here)
  subAgents?: PreloadedSubAgent[]; // Optional preloaded sub-agent definitions
  systemPrompt: string; // System prompt text
  userPrompt: string; // User prompt text
  outputFormat: OutputFormatId; // Required output format
  renderTarget?: "cli" | "slack" | "api" | "web" | "embed" | "sub-agent"; // Rendering target

  // Output contract
  expectedOutput?: {
    format: "json" | "markdown" | "text";
    schema?: Record<string, unknown>; // JSON Schema for 'json' format
  };

  // Sampling parameters
  temperature?: number | null; // Default: 0.0
  topP?: number | null; // Default: not sent
  topK?: number | null; // Default: not sent
  maxOutputTokens?: number; // Computed as contextWindow/4 if not specified
  repeatPenalty?: number | null; // Default: not sent

  // Execution limits
  maxRetries?: number; // Default: 3
  maxTurns?: number; // Default: 10
  maxToolCallsPerTurn?: number; // Default: 10
  llmTimeout?: number; // Default: 600000 (10 min)
  toolTimeout?: number; // Default: 300000 (5 min)
  toolResponseMaxBytes?: number; // Default: 12288 (12KB)

  // Orchestration
  orchestration?: OrchestrationConfig;
  orchestrationRuntime?: OrchestrationRuntimeConfig;

  // Caching
  cacheTtlMs?: number; // Response cache TTL
  agentHash?: string; // Cache key component

  // Reasoning
  reasoning?: ReasoningLevel; // 'minimal' | 'low' | 'medium' | 'high'
  reasoningValue?: string | number | null; // Explicit token budget
  caching?: CachingMode; // 'none' | 'full'

  // Streaming and debugging
  stream?: boolean; // Default: true
  traceLLM?: boolean; // Default: false
  traceMCP?: boolean; // Default: false
  traceSdk?: boolean; // Default: false
  verbose?: boolean; // Default: false

  // Identity and tracing
  agentId?: string; // Agent identifier
  headendId?: string; // Headend identifier
  headendWantsProgressUpdates?: boolean; // Progress updates requested
  telemetryLabels?: Record<string, string>; // Telemetry labels
  isMaster?: boolean; // Is this the master agent?
  pendingHandoffCount?: number; // Pending handoffs
  trace?: {
    // Trace context propagation
    selfId?: string;
    originId?: string;
    parentId?: string;
    callPath?: string;
    agentPath?: string;
    turnPath?: string;
  };

  // Callbacks
  callbacks?: AIAgentEventCallbacks;

  // Control
  abortSignal?: AbortSignal; // External cancellation
  stopRef?: { stopping: boolean }; // Graceful stop

  // Context
  contextWindow?: number; // Override context window
  conversationHistory?: ConversationMessage[]; // Prior conversation
  ancestors?: string[]; // Recursion prevention
  agentPath?: string; // Agent path
  turnPathPrefix?: string; // Turn path prefix
  initialTitle?: string; // Pre-set session title
  toolOutput?: ToolOutputConfigInput; // Tool output config
}
```

---

## Event Callbacks

### AIAgentEventCallbacks

```typescript
interface AIAgentEventCallbacks {
  onEvent?: (event: AIAgentEvent, meta: AIAgentEventMeta) => void;
}
```

### AIAgentEventMeta

| Field                 | Type           | Description                           |
| --------------------- | -------------- | ------------------------------------- |
| `agentId`             | `string?`      | Agent identifier                      |
| `callPath`            | `string?`      | Hierarchical call path                |
| `sessionId`           | `string?`      | Session identifier                    |
| `parentId`            | `string?`      | Parent session ID                     |
| `originId`            | `string?`      | Root session ID                       |
| `headendId`           | `string?`      | Headend identifier                    |
| `renderTarget`        | `string?`      | Target renderer type                  |
| `isMaster`            | `boolean`      | Is this the master agent?             |
| `isFinal`             | `boolean`      | Authoritative only for `final_report` |
| `pendingHandoffCount` | `number`       | Pending handoffs                      |
| `handoffConfigured`   | `boolean`      | Handoff is configured                 |
| `sequence`            | `number`       | Event sequence number                 |
| `source`              | `EventSource?` | 'stream', 'replay', or 'finalize'     |

---

## Result Structure

### AIAgentResult

```typescript
interface AIAgentResult {
  success: boolean; // Run succeeded
  error?: string; // Error message if failed
  finalAgentId?: string; // Agent that produced final report
  conversation: ConversationMessage[]; // Full conversation
  logs: LogEntry[]; // All log entries
  accounting: AccountingEntry[]; // All accounting entries

  // Optional fields
  treeAscii?: string; // ASCII execution tree
  opTreeAscii?: string; // Hierarchical operation tree
  opTree?: SessionNode; // Operation tree structure
  childConversations?: {
    // Sub-agent conversations
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
  routerSelection?: RouterSelection;
  finalReport?: FinalReportPayload;
}
```

### FinalReportPayload

```typescript
interface FinalReportPayload {
  format:
    | "json"
    | "markdown"
    | "markdown+mermaid"
    | "slack-block-kit"
    | "tty"
    | "pipe"
    | "sub-agent"
    | "text";
  content?: string; // Text content
  content_json?: Record<string, unknown>; // JSON content
  metadata?: Record<string, unknown>; // Additional metadata
  ts: number; // Timestamp
}
```

---

## Event Types Reference

### AIAgentEvent

| Type               | Payload                               | Description             |
| ------------------ | ------------------------------------- | ----------------------- |
| `output`           | `{ text: string }`                    | Assistant output stream |
| `thinking`         | `{ text: string }`                    | Reasoning stream        |
| `turn_started`     | See below                             | LLM turn start          |
| `progress`         | `{ event: ProgressEvent }`            | Progress events         |
| `status`           | `{ event: ProgressEvent }`            | Mirror of progress      |
| `final_report`     | `{ report: FinalReportPayload }`      | Final report            |
| `handoff`          | `{ report: FinalReportPayload }`      | Handoff payload         |
| `log`              | `{ entry: LogEntry }`                 | Structured log          |
| `accounting`       | `{ entry: AccountingEntry }`          | Accounting entry        |
| `snapshot`         | `{ payload: SessionSnapshotPayload }` | Session snapshot        |
| `accounting_flush` | `{ payload: AccountingFlushPayload }` | Batched accounting      |
| `op_tree`          | `{ tree: SessionNode }`               | Operation tree snapshot |

### turn_started Event

```typescript
{
  type: 'turn_started',
  turn: number;           // Turn number
  attempt: number;        // Retry attempt
  isRetry: boolean;       // Is this a retry?
  isFinalTurn: boolean;   // Is this the final turn?
  forcedFinalReason?: ForcedFinalReason;
  retrySlugs?: string[];  // Retry reasons
}
```

### ProgressEvent Types

| Type             | Description                  |
| ---------------- | ---------------------------- |
| `agent_started`  | Agent execution started      |
| `agent_update`   | Agent status update          |
| `agent_finished` | Agent completed successfully |
| `agent_failed`   | Agent failed                 |
| `tool_started`   | Tool execution started       |
| `tool_finished`  | Tool execution completed     |

---

## Accounting Records

### LLMAccountingEntry

```typescript
interface LLMAccountingEntry {
  type: "llm";
  timestamp: number;
  provider: string;
  model: string;
  actualProvider?: string; // For routed providers
  actualModel?: string;
  status: "ok" | "failed";
  latency: number; // Milliseconds
  tokens: {
    inputTokens: number;
    outputTokens: number;
    cachedTokens?: number;
    cacheReadInputTokens?: number;
    cacheWriteInputTokens?: number;
    totalTokens: number;
  };
  costUsd?: number;
  upstreamInferenceCostUsd?: number;
  stopReason?: string;
  error?: string;

  // Tracing fields
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
}
```

### ToolAccountingEntry

```typescript
interface ToolAccountingEntry {
  type: "tool";
  timestamp: number;
  mcpServer: string; // 'agent' for internal tools
  command: string; // Tool name
  status: "ok" | "failed";
  latency: number; // Milliseconds
  charactersIn: number; // Input size
  charactersOut: number; // Output size
  error?: string;

  // Tracing fields
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  details?: Record<string, unknown>; // Additional details
}
```

---

## Output Format Contract

The `expectedOutput.format` controls final report structure:

| Format     | Tool Schema                                                        | Validation     |
| ---------- | ------------------------------------------------------------------ | -------------- |
| `json`     | `{ report_format: { const: 'json' }, content_json: <schema> }`     | AJV validation |
| `markdown` | `{ report_format: { const: 'markdown' }, report_content: string }` | None           |
| `text`     | `{ report_format: { const: 'text' }, report_content: string }`     | None           |

**Example JSON output configuration**:

```typescript
const sessionConfig = {
  // ...
  expectedOutput: {
    format: "json",
    schema: {
      type: "object",
      properties: {
        summary: { type: "string" },
        items: { type: "array", items: { type: "string" } },
      },
      required: ["summary"],
    },
  },
};
```

---

## Library Guarantees

| Guarantee                   | Description                            |
| --------------------------- | -------------------------------------- |
| **No Direct I/O**           | Never writes to stdout/stderr or files |
| **Real-time Callbacks**     | Events emitted as they occur           |
| **Always Returns Result**   | `run()` resolves even on errors        |
| **FIN Summaries**           | Always emitted at end of run           |
| **Error Transparency**      | Errors in logs and `result.error`      |
| **Accounting Completeness** | All entries recorded                   |

---

## Final Report Failure Semantics

### Run-level failure

Transport, config, or runtime errors:

```typescript
{
  success: false,
  error: 'Connection refused',
  // ...
}
```

### Task-level failure

Model-declared failure (task couldn't be completed):

```typescript
{
  success: true,  // Run completed successfully
  finalReport: {
    format: 'json',
    content_json: {
      status: 'failure',  // Task failed
      error: 'Unable to find the requested data'
    }
  }
}
```

---

## Notes for Embedders

### Frontmatter Parsing

- Frontmatter parsing is CLI responsibility
- Pass concrete values in `AIAgentSessionConfig`
- Don't expect library to parse `.ai` files

### Log Handling

- Handle `onEvent(type='log')` for redacted HTTP logs
- Persist `result.logs` and `result.accounting` even on failure
- Log entries include structured `details` for programmatic access

### Error Handling

```typescript
const result = await AIAgent.run(session);

if (!result.success) {
  // Run-level failure
  console.error("Run failed:", result.error);
  console.error("Logs:", result.logs);
  return;
}

if (result.finalReport?.content_json?.status === "failure") {
  // Task-level failure
  console.warn(
    "Task could not be completed:",
    result.finalReport.content_json.error,
  );
}
```

### Streaming Output

```typescript
let streamedOutput = "";

const sessionConfig = {
  // ...
  stream: true,
  callbacks: {
    onEvent: (event, meta) => {
      if (event.type === "output") {
        // Stream during execution
        if (meta.source !== "finalize") {
          process.stdout.write(event.text);
          streamedOutput += event.text;
        }
      }
    },
  },
};
```

### Cancellation

```typescript
const controller = new AbortController();

const sessionConfig = {
  // ...
  abortSignal: controller.signal,
};

// Later, cancel the session
controller.abort();
```

### Graceful Stop

```typescript
const stopRef = { stopping: false };

const sessionConfig = {
  // ...
  stopRef,
};

// Later, request graceful stop (agent finishes current work)
stopRef.stopping = true;
```

---

## See Also

- [docs/specs/AI-AGENT-INTERNAL-API.md](specs/AI-AGENT-INTERNAL-API.md) - Full API documentation
- [docs/specs/library-api.md](specs/library-api.md) - Library specification
- [docs/Headends-Library.md](Headends-Library) - Library headend integration
- [docs/Technical-Specs-Session-Lifecycle.md](Technical-Specs-Session-Lifecycle) - Session flow
