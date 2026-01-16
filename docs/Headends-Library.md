# Library Embedding

Use AI Agent programmatically in your Node.js/TypeScript applications with the library-first API.

---

## Table of Contents

- [Overview](#overview) - What library embedding provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [Installation](#installation) - Package setup
- [Core API](#core-api) - AIAgent class and methods
- [Configuration Options](#configuration-options) - AIAgentSessionConfig reference
- [Event Callbacks](#event-callbacks) - Event handling system
- [Event Types](#event-types) - All event type definitions
- [Result Object](#result-object) - AIAgentResult structure
- [Running Agents](#running-agents) - Execution patterns
- [Advanced Usage](#advanced-usage) - Sub-agents, abort signals, tracing
- [Session Lifecycle](#session-lifecycle) - Creation, execution, cleanup
- [Silent Core](#silent-core) - I/O isolation principles
- [Error Handling](#error-handling) - Error patterns and recovery
- [Examples](#examples) - Complete integration examples
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

AI Agent is designed library-first. The core performs no I/O and communicates entirely via callbacks, giving you complete control over how your application handles output, logging, and errors.

**Key features**:

- Factory pattern with `AIAgent.create()` and `AIAgent.run()`
- Callback-driven execution with typed events
- No stdout/stderr writes - all output via callbacks
- Full conversation history and accounting
- Sub-agent orchestration support
- Abort signal integration
- Trace context propagation

**Use library embedding when**:

- Building custom headends
- Integrating into existing applications
- Need fine-grained control over I/O
- Building microservices or APIs
- Custom event processing requirements

---

## Quick Start

```typescript
import { AIAgent } from "ai-agent-claude";
import type { AIAgentSessionConfig, Configuration } from "ai-agent-claude";

// Load your configuration
const config: Configuration = {
  /* ... */
};

// Create session config
const sessionConfig: AIAgentSessionConfig = {
  config,
  targets: [{ provider: "anthropic", model: "claude-sonnet-4-20250514" }],
  tools: ["filesystem"],
  systemPrompt: "You are a helpful assistant.",
  userPrompt: "List files in the current directory.",
  outputFormat: "markdown",
  callbacks: {
    onEvent: (event) => {
      if (event.type === "output") {
        process.stdout.write(event.text);
      }
    },
  },
};

// Create and run
const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);

console.log(`Success: ${result.success}`);
```

---

## Installation

### From npm

```bash
npm install ai-agent-claude
```

### Package Entry Points

```json
{
  "main": "dist/index.js",
  "types": "dist/index.d.ts"
}
```

### Module Resolution

Requires `"module": "NodeNext"` in your tsconfig.json.

---

## Core API

### AIAgent.create()

Creates a new session instance with validated configuration.

```typescript
static create(config: AIAgentSessionConfig): AIAgentSession
```

| Parameter | Type                   | Description                |
| --------- | ---------------------- | -------------------------- |
| `config`  | `AIAgentSessionConfig` | Full session configuration |

**Returns**: `AIAgentSession` instance.

**Throws**: On validation errors (unknown providers, invalid tools, missing prompts).

### AIAgent.run()

Executes a session with full orchestration (advisors, router, handoff).

```typescript
static async run(session: AIAgentSession): Promise<AIAgentResult>
```

| Parameter | Type             | Description             |
| --------- | ---------------- | ----------------------- |
| `session` | `AIAgentSession` | Session from `create()` |

**Returns**: `AIAgentResult` with success status, conversation, logs, and accounting.

**Note**: Never throws for operational failures. Always resolves with `result.success = false` and `result.error` describing the failure.

### Exports

```typescript
// Classes
export { AIAgent, AIAgentSession } from "./ai-agent.js";
export { LLMClient } from "./llm-client.js";

// Types
export type {
  AIAgentSessionConfig,
  AIAgentResult,
  AIAgentEvent,
  AIAgentEventCallbacks,
  AIAgentEventMeta,
  Configuration,
  ConversationMessage,
  ToolCall,
  ToolResult,
  TokenUsage,
  LogEntry,
  AccountingEntry,
  // ... and more
};
```

---

## Configuration Options

`AIAgentSessionConfig` defines all session parameters.

### Required Fields

| Option         | Type                                    | Description                    |
| -------------- | --------------------------------------- | ------------------------------ |
| `config`       | `Configuration`                         | Full application configuration |
| `targets`      | `{ provider: string; model: string }[]` | LLM provider/model pairs       |
| `tools`        | `string[]`                              | MCP server names to enable     |
| `systemPrompt` | `string`                                | System prompt text             |
| `userPrompt`   | `string`                                | User prompt text               |
| `outputFormat` | `OutputFormatId`                        | Output format identifier       |

### Optional Identity

| Option            | Type                     | Default | Description                  |
| ----------------- | ------------------------ | ------- | ---------------------------- |
| `agentId`         | `string`                 | -       | Agent identifier for logging |
| `headendId`       | `string`                 | -       | Headend identifier           |
| `telemetryLabels` | `Record<string, string>` | -       | Custom telemetry labels      |

### Optional LLM Parameters

| Option            | Type                             | Default | Description                                          |
| ----------------- | -------------------------------- | ------- | ---------------------------------------------------- |
| `temperature`     | `number \| null`                 | -       | Sampling temperature                                 |
| `topP`            | `number \| null`                 | -       | Top-p sampling                                       |
| `topK`            | `number \| null`                 | -       | Top-k sampling                                       |
| `maxOutputTokens` | `number`                         | -       | Max output tokens                                    |
| `repeatPenalty`   | `number \| null`                 | -       | Frequency penalty                                    |
| `reasoning`       | `ReasoningLevel`                 | -       | Reasoning level (`minimal`, `low`, `medium`, `high`) |
| `reasoningValue`  | `ProviderReasoningValue \| null` | -       | Reasoning budget                                     |
| `caching`         | `CachingMode`                    | -       | Cache control mode                                   |

### Optional Limits

| Option                | Type     | Default | Description             |
| --------------------- | -------- | ------- | ----------------------- |
| `maxRetries`          | `number` | -       | Retry limit per turn    |
| `maxTurns`            | `number` | -       | Maximum LLM turns       |
| `maxToolCallsPerTurn` | `number` | -       | Max tool calls per turn |

### Optional Timeouts

| Option        | Type     | Default | Description                     |
| ------------- | -------- | ------- | ------------------------------- |
| `llmTimeout`  | `number` | -       | LLM timeout in milliseconds     |
| `toolTimeout` | `number` | -       | Tool timeout in milliseconds    |
| `cacheTtlMs`  | `number` | -       | Response cache TTL (0 disables) |

### Optional Behavior

| Option                        | Type      | Default | Description                 |
| ----------------------------- | --------- | ------- | --------------------------- |
| `stream`                      | `boolean` | -       | Enable streaming output     |
| `verbose`                     | `boolean` | -       | Verbose logging             |
| `traceLLM`                    | `boolean` | -       | Trace LLM calls             |
| `traceMCP`                    | `boolean` | -       | Trace MCP calls             |
| `traceSdk`                    | `boolean` | -       | Trace SDK calls             |
| `headendWantsProgressUpdates` | `boolean` | -       | Enable progress update tool |

### Optional Callbacks

| Option      | Type                    | Description              |
| ----------- | ----------------------- | ------------------------ |
| `callbacks` | `AIAgentEventCallbacks` | Event notification hooks |

### Optional Conversation

| Option                | Type                    | Description                     |
| --------------------- | ----------------------- | ------------------------------- |
| `conversationHistory` | `ConversationMessage[]` | Pre-existing conversation       |
| `subAgents`           | `PreloadedSubAgent[]`   | Preloaded sub-agent definitions |

### Optional Output Contract

| Option           | Type                  | Description                            |
| ---------------- | --------------------- | -------------------------------------- |
| `expectedOutput` | `{ format, schema? }` | Expected output format and JSON schema |

### Optional Trace Context

| Option            | Type     | Description         |
| ----------------- | -------- | ------------------- |
| `trace.originId`  | `string` | Root transaction ID |
| `trace.parentId`  | `string` | Parent agent ID     |
| `trace.selfId`    | `string` | This session's ID   |
| `trace.callPath`  | `string` | Call hierarchy path |
| `trace.agentPath` | `string` | Agent file path     |
| `trace.turnPath`  | `string` | Turn path prefix    |

### Optional Cancellation

| Option        | Type                    | Description                   |
| ------------- | ----------------------- | ----------------------------- |
| `abortSignal` | `AbortSignal`           | External cancellation control |
| `stopRef`     | `{ stopping: boolean }` | Graceful stop reference       |

### Example Configuration

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // Required
  config: loadConfiguration("./.ai-agent.json"),
  targets: [{ provider: "anthropic", model: "claude-sonnet-4-20250514" }],
  tools: ["filesystem", "git"],
  systemPrompt: "You are a helpful coding assistant.",
  userPrompt: "Review the code in src/main.ts",
  outputFormat: "markdown",

  // Optional limits
  maxTurns: 20,
  maxRetries: 3,
  maxToolCallsPerTurn: 10,

  // Optional timeouts
  llmTimeout: 120000,
  toolTimeout: 30000,

  // Optional behavior
  stream: true,
  temperature: 0.7,

  // Optional identity
  agentId: "code-reviewer",
  headendId: "my-app",
  telemetryLabels: {
    environment: "production",
    feature: "code-review",
  },

  // Optional callbacks
  callbacks: {
    onEvent: (event, meta) => {
      // Handle events
    },
  },
};
```

---

## Event Callbacks

### AIAgentEventCallbacks Interface

```typescript
interface AIAgentEventCallbacks {
  onEvent?: (event: AIAgentEvent, meta: AIAgentEventMeta) => void;
}
```

### AIAgentEventMeta Fields

| Field                 | Type                                 | Description                                      |
| --------------------- | ------------------------------------ | ------------------------------------------------ |
| `isMaster`            | `boolean`                            | True when this agent is the master               |
| `pendingHandoffCount` | `number`                             | Count of pending handoffs at session start       |
| `handoffConfigured`   | `boolean`                            | True when handoff target is configured           |
| `isFinal`             | `boolean`                            | **Authoritative only for `final_report` events** |
| `source`              | `'stream' \| 'replay' \| 'finalize'` | Stream source for output/thinking events         |

### Callback Isolation

Callback exceptions are caught and logged but do not crash the session. This prevents user-provided handlers from destabilizing the run.

---

## Event Types

| Event Type         | Payload Fields                                                             | Description                           |
| ------------------ | -------------------------------------------------------------------------- | ------------------------------------- |
| `output`           | `{ text }`                                                                 | Assistant output stream               |
| `thinking`         | `{ text }`                                                                 | Reasoning stream (extended thinking)  |
| `turn_started`     | `{ turn, attempt, isRetry, isFinalTurn, forcedFinalReason?, retrySlugs? }` | LLM attempt start notification        |
| `progress`         | `{ event }`                                                                | Progress events with `taskStatus`     |
| `status`           | `{ event }`                                                                | Mirror of progress for `agent_update` |
| `final_report`     | `{ report }`                                                               | Final report payload                  |
| `handoff`          | `{ report }`                                                               | Handoff payload when delegating       |
| `log`              | `{ entry }`                                                                | Structured log entry                  |
| `accounting`       | `{ entry }`                                                                | Accounting entry (LLM/tool)           |
| `snapshot`         | `{ payload }`                                                              | Session snapshot payload              |
| `accounting_flush` | `{ payload }`                                                              | Batched accounting payload            |
| `op_tree`          | `{ tree }`                                                                 | Operation tree snapshot               |

### Event Handling Example

```typescript
const callbacks: AIAgentEventCallbacks = {
  onEvent: (event, meta) => {
    switch (event.type) {
      case "output":
        // Real-time text output
        process.stdout.write(event.text);
        break;

      case "thinking":
        // Reasoning/thinking output
        console.log(`[Thinking] ${event.text}`);
        break;

      case "turn_started":
        // New turn notification
        const parts = [];
        if (Array.isArray(event.retrySlugs)) parts.push(...event.retrySlugs);
        if (event.forcedFinalReason) parts.push(event.forcedFinalReason);
        const reason = parts.length > 0 ? `, ${parts.join(", ")}` : "";
        console.log(
          `\n--- Turn ${event.turn}, Attempt ${event.attempt}${reason} ---`,
        );
        break;

      case "log":
        // Structured logging
        const { severity, message } = event.entry;
        if (severity === "ERR") {
          console.error(`[ERROR] ${message}`);
        } else if (severity === "WRN") {
          console.warn(`[WARN] ${message}`);
        }
        break;

      case "accounting":
        // Token/cost tracking
        if (event.entry.type === "llm") {
          const { inputTokens, outputTokens } = event.entry.tokens;
          console.log(`LLM: ${inputTokens}/${outputTokens} tokens`);
        }
        break;

      case "final_report":
        // Final response (authoritative when meta.isFinal)
        if (meta.isFinal) {
          console.log("Final report:", event.report.content);
        }
        break;

      case "handoff":
        // Agent delegation
        console.log(`Handing off to: ${event.report.targetAgent}`);
        break;

      case "snapshot":
        // Session snapshot (fires even on failure)
        saveSnapshot(event.payload);
        break;
    }
  },
};
```

---

## Result Object

### AIAgentResult Interface

```typescript
interface AIAgentResult {
  // Status
  success: boolean; // Session completed successfully
  error?: string; // Error message if failed

  // Conversation data
  conversation: ConversationMessage[]; // Full conversation history
  logs: LogEntry[]; // All log entries
  accounting: AccountingEntry[]; // All accounting entries

  // Execution tree (optional)
  treeAscii?: string; // ASCII representation
  opTreeAscii?: string; // ASCII of hierarchical ops
  opTree?: SessionNode; // Structured operation tree

  // Sub-agent results (optional)
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

  // Router selection (optional)
  routerSelection?: {
    agent: string;
    message?: string;
  };

  // The agent id that produced the final report/response (after orchestration)
  finalAgentId?: string;

  // Final report (optional)
  finalReport?: {
    format:
      | "json"
      | "markdown"
      | "markdown+mermaid"
      | "slack-block-kit"
      | "tty"
      | "pipe"
      | "sub-agent"
      | "text";
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}
```

### Result Processing Example

```typescript
const result = await AIAgent.run(session);

if (result.success) {
  console.log("Session completed successfully");

  // Access final report
  if (result.finalReport) {
    console.log(`Format: ${result.finalReport.format}`);
    console.log(`Content: ${result.finalReport.content}`);
  }

  // Calculate total tokens
  const totalTokens = result.accounting
    .filter((e) => e.type === "llm")
    .reduce((sum, e) => sum + e.tokens.totalTokens, 0);
  console.log(`Total tokens: ${totalTokens}`);

  // Check for warnings
  const warnings = result.logs.filter((l) => l.severity === "WRN");
  if (warnings.length > 0) {
    console.log(`Warnings: ${warnings.length}`);
  }
} else {
  console.error(`Session failed: ${result.error}`);

  // Review logs for details
  const errors = result.logs.filter((l) => l.severity === "ERR");
  errors.forEach((e) => console.error(e.message));
}
```

---

## Running Agents

### Basic Execution

```typescript
const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);
```

### With Conversation History

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  userPrompt: "Follow-up question",
  conversationHistory: previousResult.conversation,
};

const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);
```

### With Structured Input

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  userPrompt: JSON.stringify({
    text: "Content to analyze",
    options: {
      detailed: true,
      format: "json",
    },
  }),
};
```

---

## Advanced Usage

### With Sub-Agents

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  subAgents: [
    {
      toolName: "code_reviewer",
      description: "Reviews code for best practices",
      usage: "Review code for best practices",
      promptPath: "/path/to/reviewer.ai",
      inputFormat: "json",
      inputSchema: {
        type: "object",
        properties: {
          code: { type: "string" },
          guidelines: { type: "string" },
        },
      },
      hasExplicitInputSchema: true,
      systemTemplate: "You are a code reviewer...",
      loaded: {
        /* LoadedAgent object created by agent loader */
      },
    },
  ],
};
```

**Note**: `PreloadedSubAgent` objects are typically created by the agent loader when parsing `.ai` files. The `loaded` field is a runtime `LoadedAgent` object with `createSession` and `run` methods.

### With Abort Signal

```typescript
const controller = new AbortController();

const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  abortSignal: controller.signal,
};

// Cancel after 5 minutes
setTimeout(() => controller.abort(), 300000);

const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);

if (!result.success && result.error?.includes("abort")) {
  console.log("Session was cancelled");
}
```

### With Trace Context

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  trace: {
    originId: "request-12345",
    parentId: "parent-agent-uuid",
    selfId: crypto.randomUUID(),
    callPath: "root>analyzer>reviewer",
    agentPath: "reviewer.ai",
  },
};
```

### With Custom Telemetry

```typescript
const sessionConfig: AIAgentSessionConfig = {
  // ... base config
  headendId: "my-custom-headend",
  telemetryLabels: {
    environment: "production",
    customer: "acme-corp",
    feature: "document-analysis",
    version: "2.0.0",
  },
};
```

---

## Session Lifecycle

### 1. Creation Phase

`AIAgent.create(config)`:

- Validates providers exist in configuration
- Validates MCP server names
- Validates prompts (both can't use stdin)
- Generates transaction ID if not provided
- Creates LLM client with pricing configuration
- Initializes session state

### 2. Execution Phase

`AIAgent.run(session)`:

- Prepares system and user prompts
- Initializes MCP servers (with concurrency limit)
- Enters turn loop
- Processes tool calls
- Manages context window
- Invokes callbacks for each event
- Handles retries and errors
- Manages handoffs to other agents

### 3. Cleanup Phase

After execution:

- MCP servers disconnected
- Resources released
- Final accounting computed
- Snapshot persisted (via callback)

---

## Silent Core

The core library enforces complete I/O isolation:

- **Never writes to files** - Use `snapshot` callback for persistence
- **Never writes to stdout** - Use `output` callback
- **Never writes to stderr** - Use `log` callback
- **All output via callbacks** - Complete control in your application

This design enables:

- Custom logging frameworks
- Structured output collection
- Multi-tenant isolation
- Testing without I/O mocking

---

## Error Handling

### Creation Errors

Thrown by `AIAgent.create()`:

- Unknown provider names
- Unknown MCP server names
- Invalid prompt configuration (both stdin)
- Missing required fields

```typescript
try {
  const session = AIAgent.create(sessionConfig);
} catch (error) {
  console.error("Configuration error:", error.message);
}
```

### Runtime Errors

Returned in `result.error` (never thrown):

- LLM API failures
- Tool execution errors
- Context window exceeded
- Timeout reached
- Abort signal triggered

```typescript
const result = await AIAgent.run(session);

if (!result.success) {
  // Always check result.success
  console.error("Runtime error:", result.error);

  // Review logs for details
  const errorLogs = result.logs.filter((l) => l.severity === "ERR");
  errorLogs.forEach((log) => {
    console.error(`  ${log.ts}: ${log.message}`);
  });
}
```

### Error Categories

| Category   | Description             | Recovery                 |
| ---------- | ----------------------- | ------------------------ |
| Validation | Configuration errors    | Fix configuration        |
| Network    | API connectivity        | Retry with backoff       |
| Timeout    | LLM/tool timeout        | Increase timeout         |
| Context    | Context window exceeded | Reduce history           |
| Abort      | External cancellation   | Expected termination     |
| Tool       | Tool execution failure  | Check tool configuration |

---

## Examples

### REST API Server

```typescript
import express from "express";
import { AIAgent } from "ai-agent-claude";
import type { AIAgentSessionConfig, Configuration } from "ai-agent-claude";

const app = express();
app.use(express.json());

const config: Configuration = loadConfiguration("./.ai-agent.json");

app.post("/chat", async (req, res) => {
  const chunks: string[] = [];

  const sessionConfig: AIAgentSessionConfig = {
    config,
    targets: [{ provider: "anthropic", model: "claude-sonnet-4-20250514" }],
    tools: ["filesystem"],
    systemPrompt: "You are a helpful assistant.",
    userPrompt: req.body.message,
    outputFormat: "markdown",
    stream: true,
    callbacks: {
      onEvent: (event) => {
        if (event.type === "output") {
          chunks.push(event.text);
        }
      },
    },
  };

  const session = AIAgent.create(sessionConfig);
  const result = await AIAgent.run(session);

  res.json({
    success: result.success,
    response: chunks.join(""),
    tokens: result.accounting
      .filter((e) => e.type === "llm")
      .reduce((sum, e) => sum + e.tokens.totalTokens, 0),
  });
});

app.listen(3000, () => {
  console.log("Server running on port 3000");
});
```

### SSE Streaming Server

```typescript
import express from "express";
import { AIAgent } from "ai-agent-claude";

const app = express();
app.use(express.json());

app.post("/stream", async (req, res) => {
  res.setHeader("Content-Type", "text/event-stream");
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("Connection", "keep-alive");

  const sessionConfig = {
    // ... config
    callbacks: {
      onEvent: (event, meta) => {
        if (event.type === "output") {
          res.write(
            `event: text\ndata: ${JSON.stringify({ text: event.text })}\n\n`,
          );
        }
        if (event.type === "final_report" && meta.isFinal) {
          res.write(
            `event: done\ndata: ${JSON.stringify({ success: true })}\n\n`,
          );
        }
      },
    },
  };

  const session = AIAgent.create(sessionConfig);
  const result = await AIAgent.run(session);

  if (!result.success) {
    res.write(
      `event: error\ndata: ${JSON.stringify({ error: result.error })}\n\n`,
    );
  }

  res.end();
});
```

### Multi-Turn Conversation

```typescript
import { AIAgent } from "ai-agent-claude";
import type { ConversationMessage } from "ai-agent-claude";

async function chat(
  message: string,
  history: ConversationMessage[] = [],
): Promise<{ response: string; history: ConversationMessage[] }> {
  let response = "";

  const sessionConfig = {
    // ... base config
    userPrompt: message,
    conversationHistory: history,
    callbacks: {
      onEvent: (event) => {
        if (event.type === "output") {
          response += event.text;
        }
      },
    },
  };

  const session = AIAgent.create(sessionConfig);
  const result = await AIAgent.run(session);

  return {
    response,
    history: result.conversation,
  };
}

// Usage
let history: ConversationMessage[] = [];

const first = await chat("What is TypeScript?", history);
console.log(first.response);
history = first.history;

const second = await chat("How do I use generics?", history);
console.log(second.response);
history = second.history;
```

### With Progress Tracking

```typescript
const sessionConfig = {
  // ... base config
  headendWantsProgressUpdates: true,
  callbacks: {
    onEvent: (event) => {
      if (event.type === "progress" && event.event.type === "agent_update") {
        const status = event.event.taskStatus;
        console.log(`[${status.status}] ${status.message}`);
        if (status.done) console.log(`  Done: ${status.done}`);
        if (status.pending) console.log(`  Pending: ${status.pending}`);
        if (status.now) console.log(`  Current: ${status.now}`);
      }
    },
  },
};
```

---

## Troubleshooting

### Import errors

**Symptom**: `Cannot find module 'ai-agent'` or type errors.

**Causes**:

1. Package not installed
2. Module resolution mismatch
3. Build output missing

**Solutions**:

1. Run `npm install ai-agent-claude`
2. Set `"module": "NodeNext"` in tsconfig.json
3. Verify `dist/` directory exists in package

### Callbacks not firing

**Symptom**: `onEvent` never called.

**Causes**:

1. Callback not registered before `create()`
2. Property name misspelled
3. Exception in callback swallowed

**Solutions**:

1. Pass callbacks in `AIAgentSessionConfig`
2. Verify property is `callbacks.onEvent`
3. Add try/catch inside callback to debug

### Session hangs

**Symptom**: `run()` never resolves.

**Causes**:

1. Tool blocking indefinitely
2. LLM not responding
3. Abort signal not wired

**Solutions**:

1. Set `toolTimeout` (e.g., `30000`)
2. Set `llmTimeout` (e.g., `120000`)
3. Set `maxTurns` limit
4. Use `abortSignal` for external control

### Missing final report

**Symptom**: `result.finalReport` is undefined.

**Causes**:

1. Session failed
2. Agent didn't call `final_report` tool
3. Handoff configured but not completed

**Solutions**:

1. Check `result.success` and `result.error`
2. Review `result.logs` for details
3. Check `result.conversation` for agent output

### Type mismatches

**Symptom**: TypeScript compilation errors.

**Causes**:

1. Outdated type definitions
2. Version mismatch
3. Incorrect import paths

**Solutions**:

1. Run `npm run build` in ai-agent
2. Match package versions
3. Use `import type` for type-only imports

### Context window exceeded

**Symptom**: Error about context/token limits.

**Causes**:

1. Conversation history too long
2. Tool responses too large
3. System prompt too large

**Solutions**:

1. Limit `conversationHistory` length
2. Set `toolResponseMaxBytes`
3. Reduce system prompt size
4. Use conversation summarization

---

## See Also

- [Headends](Headends) - Built-in headend overview
- [Headends-REST](Headends-REST) - REST API headend
- [Headends-Embed](Headends-Embed) - Embed headend (similar SSE patterns)
- [Agent Files](Agent-Files) - Agent configuration reference
- [Configuration](Configuration) - Full configuration guide
- [specs/library-api.md](specs/library-api.md) - Technical specification
