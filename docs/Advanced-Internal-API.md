# Internal API

Library embedding API for programmatic use.

---

## Overview

The AI Agent library can be embedded in applications without the CLI. All output, logs, and accounting are emitted through callbacks and returned in a structured result.

**Entry Point**: `src/ai-agent.ts` (exported as `AIAgent`)

---

## Core Methods

```typescript
import { AIAgent } from 'ai-agent';

// Create a session
const session = AIAgent.create(sessionConfig);

// Run with orchestration (advisors/router/handoff)
const result = await AIAgent.run(session);

// Or run the inner session directly
const result = await session.run();
```

---

## Session Configuration

```typescript
interface AIAgentSessionConfig {
  // Required
  config: Configuration;          // providers + MCP servers
  targets: { provider, model }[]; // provider/model pairs
  tools: string[];               // MCP servers to expose
  systemPrompt: string;
  userPrompt: string;

  // Output contract
  expectedOutput?: {
    format: 'json' | 'markdown' | 'text';
    schema?: Record<string, unknown>;
  };

  // Execution controls
  temperature?: number;
  topP?: number;
  maxRetries?: number;
  maxTurns?: number;
  llmTimeout?: number;
  toolTimeout?: number;
  toolResponseMaxBytes?: number;
  stream?: boolean;

  // Orchestration
  orchestration?: OrchestrationConfig;
  orchestrationRuntime?: OrchestrationRuntimeConfig;

  // Caching
  cacheTtlMs?: number;
  agentHash?: string;

  // Debugging
  traceLLM?: boolean;
  traceMCP?: boolean;
  verbose?: boolean;

  // Callbacks
  callbacks?: AIAgentEventCallbacks;
}
```

---

## Event Callbacks

```typescript
interface AIAgentEventCallbacks {
  onEvent(event: AIAgentEvent, meta: AIAgentEventMeta): void;
}
```

### Event Types

| Type | Description | Payload |
|------|-------------|---------|
| `output` | Assistant output stream | `{ text }` |
| `thinking` | Reasoning stream | `{ text }` |
| `turn_started` | LLM attempt start | `{ turn, attempt, isRetry, isFinalTurn }` |
| `progress` | Progress events | `{ event: ProgressEvent }` |
| `status` | Mirror of progress | `{ event }` |
| `final_report` | Final report payload | `{ report: FinalReportPayload }` |
| `handoff` | Handoff payload | `{ report: FinalReportPayload }` |
| `log` | Structured logs | `{ entry: LogEntry }` |
| `accounting` | Accounting entries | `{ entry: AccountingEntry }` |
| `snapshot` | Session snapshot | `{ payload }` |
| `accounting_flush` | Batched accounting | `{ payload }` |
| `op_tree` | OpTree snapshot | `{ tree: SessionNode }` |

### Event Metadata

```typescript
interface AIAgentEventMeta {
  isMaster: boolean;        // Is this the master agent?
  pendingHandoffCount: number;
  handoffConfigured: boolean;
  isFinal: boolean;         // Authoritative only for final_report
  source?: 'stream' | 'replay' | 'finalize';
}
```

---

## Result Structure

```typescript
interface AIAgentResult {
  success: boolean;
  error?: string;
  finalAgentId?: string;
  conversation: ConversationMessage[];
  logs: LogEntry[];
  accounting: AccountingEntry[];
  finalReport?: {
    format: 'json' | 'markdown' | 'text' | '...';
    content?: string;
    content_json?: Record<string, unknown>;
    metadata?: Record<string, unknown>;
    ts: number;
  };
}
```

---

## Minimal Example

```typescript
import { AIAgent } from 'ai-agent';

const config = {
  providers: {
    openai: { apiKey: process.env.OPENAI_API_KEY, type: 'openai' }
  },
  mcpServers: {}
};

const logs = [];
const accounting = [];
let output = '';

const sessionConfig = {
  config,
  targets: [{ provider: 'openai', model: 'gpt-4o-mini' }],
  tools: [],
  systemPrompt: 'You are helpful.',
  userPrompt: 'Say hello.',
  maxRetries: 3,
  maxTurns: 5,
  callbacks: {
    onEvent: (event, meta) => {
      if (event.type === 'log') logs.push(event.entry);
      if (event.type === 'accounting') accounting.push(event.entry);
      if (event.type === 'output' && meta.source !== 'finalize') {
        output += event.text;
      }
    }
  }
};

const session = AIAgent.create(sessionConfig);
const result = await AIAgent.run(session);

console.log('success:', result.success);
console.log('output:', output);
```

---

## Library Guarantees

1. **No Direct I/O**: Never writes to stdout/stderr or files
2. **Real-time Callbacks**: Events emitted as they occur
3. **Always Returns Result**: `run()` resolves even on errors
4. **FIN Summaries**: Always emitted at end of run
5. **Error Transparency**: Errors in logs and result.error
6. **Accounting Completeness**: All entries recorded

---

## Accounting Records

### LLM Entries

```typescript
{
  type: 'llm',
  provider: string,
  model: string,
  actualProvider?: string,  // For routed providers
  actualModel?: string,
  status: 'ok' | 'failed',
  latency: number,
  tokens: { inputTokens, outputTokens, cachedTokens?, totalTokens },
  costUsd?: number,
  error?: string
}
```

### Tool Entries

```typescript
{
  type: 'tool',
  mcpServer: string,    // 'agent' for internal tools
  command: string,
  status: 'ok' | 'failed',
  latency: number,
  charactersIn: number,
  charactersOut: number,
  error?: string,
  details?: Record<string, unknown>  // e.g., context guard info
}
```

---

## Output Format Contract

The `expectedOutput.format` controls final report structure:

| Format | Tool Schema | Validation |
|--------|-------------|------------|
| `json` | `{ format: { const: 'json' }, content_json: <schema> }` | AJV validation |
| `markdown` | `{ format: { const: 'markdown' }, content: string }` | None |
| `text` | `{ format: { const: 'text' }, content: string }` | None |

---

## Final Report Failure Semantics

**Run-level failure** (transport/config/runtime):
- `result.success === false`
- `result.error` is set

**Task-level failure** (model-declared):
- `result.success === true`
- `result.finalReport?.status === 'failure'`
- Error description in `content` or `content_json`

---

## Notes for Embedders

- Frontmatter parsing is CLI responsibility
- Pass concrete values in `AIAgentSessionConfig`
- Handle `onEvent(type='log')` for redacted HTTP logs
- Persist `result.logs` and `result.accounting` even on failure

---

## See Also

- [docs/AI-AGENT-INTERNAL-API.md](../docs/AI-AGENT-INTERNAL-API.md) - Full API documentation
- [docs/specs/library-api.md](../docs/specs/library-api.md) - Library spec

