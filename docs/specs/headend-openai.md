# OpenAI Completions Headend

## TL;DR
OpenAI-compatible chat completions API exposing agents as models with streaming SSE support, reasoning content rendering, and progress event tracking.

## Source Files
- `src/headends/openai-completions-headend.ts` - Full implementation (1039 lines)
- `src/headends/concurrency.ts` - ConcurrencyLimiter
- `src/headends/http-utils.ts` - HTTP/SSE utilities
- `src/headends/summary-utils.ts` - Markdown formatting
- `src/persistence.ts` - Callback persistence merging

## Headend Identity
- **ID**: `openai-completions:{port}`
- **Kind**: `openai-completions`
- **Label**: `OpenAI chat headend (port {port})`

## Configuration

```typescript
interface Options {
  port: number;              // HTTP server port
  concurrency?: number;      // Max concurrent requests (default: 10)
}
```

## Construction

**Location**: `src/headends/openai-completions-headend.ts:133-144`

```typescript
constructor(registry: AgentRegistry, options) {
  this.registry = registry;
  this.options = options;
  this.id = `openai-completions:${String(options.port)}`;
  this.label = `OpenAI chat headend (port ${port})`;
  const limit = options.concurrency > 0 ? Math.floor(options.concurrency) : 10;
  this.limiter = new ConcurrencyLimiter(limit);
  this.closed = this.closeDeferred.promise;
  this.refreshModelMap();  // Build agent → model mapping
}
```

## API Endpoints

| Method | Path | Handler |
|--------|------|---------|
| GET | `/health` | Health check |
| GET | `/v1/models` | List available models |
| POST | `/v1/chat/completions` | Chat completion |

## Model Mapping

**Location**: `src/headends/openai-completions-headend.ts:925-956`

```typescript
buildModelId(meta: AgentMetadata, seen: Set<string>): string {
  // Priority: toolName > promptPath basename > agent id basename
  const baseSources = [meta.toolName, path.basename(meta.promptPath), meta.id.split('/').pop()];
  let base = baseSources.find(s => s?.length > 0) ?? 'agent';
  base = base.replace(/\.ai$/i, '');  // Remove .ai suffix

  // Deduplicate with counter
  let candidate = base;
  let counter = 2;
  while (seen.has(candidate)) {
    candidate = `${base}-${counter}`;
    counter += 1;
  }
  seen.add(candidate);
  return candidate;
}
```

Agents exposed as OpenAI models.

## Chat Completion Request

**Location**: `src/headends/openai-completions-headend.ts:255-854`

### Request Schema
```typescript
interface OpenAIChatRequest {
  model: string;                          // Agent model ID
  messages: OpenAIChatRequestMessage[];   // Conversation
  stream?: boolean;                       // Enable SSE streaming
  format?: string;                        // Output format override
  response_format?: {
    type?: string;                        // 'json_object' for JSON
    json_schema?: unknown;                // JSON schema
  };
  payload?: Record<string, unknown>;      // Additional parameters
}

interface OpenAIChatRequestMessage {
  role: 'system' | 'user' | 'assistant';
  content: unknown;
  tool_calls?: unknown;  // Not supported - throws error
}
```

### Request Processing Flow

1. **Parse JSON body**:
   ```typescript
   const body = await readJson<OpenAIChatRequest>(req);
   ```

2. **Resolve agent**:
   ```typescript
   const agent = this.resolveAgent(body.model);
   if (agent === undefined) throw HttpError(404, 'unknown_model');
   ```

3. **Build prompt**:
   - Concatenate system messages with `System context:` prefix
   - Add conversation history with `User:`/`Assistant:` labels
   - Final user message with `User request:` prefix

4. **Resolve format**:
   - Explicit `format` field
   - `response_format.type === 'json_object'`
   - Agent default format
   - JSON requires schema

5. **Setup abort/cleanup**:
   - Client abort detection
   - Response close detection
   - Shutdown signal propagation

6. **Acquire concurrency slot**

7. **Initialize callbacks**:
   - `onOutput`: Accumulate and stream content
   - `onThinking`: Build reasoning structure
   - `onTurnStarted`: Track turn progression
   - `onProgress`: Handle progress events
   - `onLog`: Forward logs
   - `onAccounting`: Track token usage

8. **Stream response** (if `stream: true`):
   ```typescript
   res.writeHead(200, {
     'Content-Type': 'text/event-stream',
     'Cache-Control': 'no-cache',
     Connection: 'keep-alive',
   });
   ```

9. **Spawn and run session**

10. **Finalize response** (streaming or JSON)

## Streaming Response Format

### SSE Chunks
```typescript
// Role announcement
{
  id: responseId,
  object: 'chat.completion.chunk',
  created: timestamp,
  model: modelName,
  choices: [{
    index: 0,
    delta: { role: 'assistant' },
    finish_reason: null,
  }],
}

// Content delta
{
  id: responseId,
  object: 'chat.completion.chunk',
  created: timestamp,
  model: modelName,
  choices: [{
    index: 0,
    delta: { content: chunk },
    finish_reason: null,
  }],
}

// Reasoning delta
{
  id: responseId,
  object: 'chat.completion.chunk',
  created: timestamp,
  model: modelName,
  choices: [{
    index: 0,
    delta: { reasoning_content: text },
    finish_reason: null,
  }],
}

// Final chunk with usage
{
  id: responseId,
  object: 'chat.completion.chunk',
  created: timestamp,
  model: modelName,
  choices: [{
    index: 0,
    delta: {},
    finish_reason: 'stop' | 'error',
  }],
  usage: {
    prompt_tokens: number,
    completion_tokens: number,
    total_tokens: number,
  },
}
```

## Non-Streaming Response

```typescript
interface OpenAIChatResponse {
  id: string;
  object: 'chat.completion';
  created: number;
  model: string;
  choices: [{
    index: number;
    message: { role: 'assistant'; content: string };
    finish_reason: 'stop' | 'error';
  }];
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
}
```

Content includes rendered reasoning prefix + final output.

## Reasoning Rendering

**Location**: `src/headends/openai-completions-headend.ts:400-448`

```typescript
renderReasoning(): string {
  // Heading: ## <agent>: <txnId>
  // For each turn:
  //   ### Turn N
  //   (<formatTotals output>)
  //   \n\n---\n<real thinking text>
  //   \n\n---\n- <agent progress lines>
  // Summary appended via formatSummaryLine → "SUMMARY: <agent>, ..."
}
```

Structure:
- `## <agent>: <txnId>` plain-text header (Markdown H2, escaped; no bolding).
- Turns render as `### Turn N` with the per-turn metrics summary on the next line enclosed in parentheses.
- Real provider thinking is wrapped between blank-line hairlines (`\n\n---\n`) until an artificial message (progress line/summary) needs to print; additional thinking deltas keep appending inside the open block.
- Progress updates continue to render as `- **callPath** ...` bullets after the thinking block closes.
- Final line is `SUMMARY: <agent>, duration **…**, cost **$…**, agents N, tools M, tokens …` with no origin/txn id.

## Progress Event Handling

**Location**: `src/headends/openai-completions-headend.ts:550-656`

Events tracked:
- `agent_started`: Log start with reason
- `agent_update`: Log update message
- `agent_finished`: Log completion with metrics
- `agent_failed`: Log failure with error and metrics

Filtering:
- Only events matching root agent or callPath
- Ignores `tool_started` and `tool_finished`

## Token Usage Tracking

**Location**: `src/headends/openai-completions-headend.ts:96-106`

```typescript
collectUsage(entries: AccountingEntry[]): { prompt: number; completion: number; total: number } {
  return entries
    .filter((entry) => entry.type === 'llm')
    .reduce((acc, entry) => {
      acc.prompt += entry.tokens.inputTokens;
      acc.completion += entry.tokens.outputTokens;
      return acc;
    }, { prompt: 0, completion: 0 });
}
```

Aggregates all LLM accounting entries.

## Content Resolution

**Location**: `src/headends/openai-completions-headend.ts:998-1007`

```typescript
resolveContent(output: string, finalReport: unknown): string {
  if (finalReport?.format === 'json' && finalReport?.content_json) {
    return JSON.stringify(finalReport.content_json);
  }
  if (finalReport?.content?.length > 0) {
    return finalReport.content;
  }
  return output;
}
```

Priority: JSON content > text content > accumulated output.

## Shutdown Handling

Similar to REST headend:
- Socket tracking and cleanup
- Graceful end then destroy
- Abort signal propagation

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `port` | HTTP server listen port |
| `concurrency` | Max concurrent requests |
| `registry` | Available agents as models |

## Telemetry

**Labels**:
- `headend: openai-completions:{port}`
- All base telemetry labels

## Logging

**Severity levels**:
- `VRB`: Request received, completion finished
- `WRN`: Client abort
- `ERR`: Request failure, concurrency failure

**Details logged**:
- `port`, `concurrency_limit`
- `stream`, `model`, `status`
- `error`

## Events

**Request lifecycle**:
- Abort detection (client disconnect)
- Shutdown signal (server stop)
- Response completion/error

## Invariants

1. **Model uniqueness**: Deduplication with counter suffix
2. **JSON schema required**: JSON format needs schema
3. **Tool calls rejected**: Throws HttpError for tool_calls
4. **SSE format**: data: JSON\n\n for each chunk
5. **Usage aggregation**: Sum all LLM entries
6. **Turn tracking**: New turn on LLM accounting entry
7. **Unauthenticated surface**: No `Authorization` header validation; production deployments must use API gateway or network restrictions

## Business Logic Coverage (Verified 2025-11-16)

- **Reasoning stream parity**: SSE chunks emit `reasoning_content` deltas that map directly to Anthropic-style thinking logs, allowing UI clients to render thoughts inline with text (`src/headends/openai-completions-headend.ts:311-420`).
- **Transaction headers**: Each run starts with a synthetic “Transaction …” markdown header containing txnId/callPath so downstream clients can collapse/expand long responses and correlate to FIN summaries (`src/headends/openai-completions-headend.ts:311-560`).
- **Format negotiation**: Request `format`, `response_format.type`, and agent defaults merge deterministically; JSON output always validates that either `response_format.json_schema` or agent schema exists before execution (`src/headends/openai-completions-headend.ts:255-420`).
- **Tool call rejection**: Any request containing `messages[].tool_calls` is rejected with `HttpError(400, 'tool_calls_not_supported')`, keeping parity with OpenAI invariants documented in `SPECS.md` (`src/headends/openai-completions-headend.ts:360-380`).
- **Concurrency limiter + aborts**: Requests acquire limiter slots, and client aborts immediately cancel sessions, release slots, and emit SSE `error` events describing the cancel reason (`src/headends/openai-completions-headend.ts:188-250`, `src/headends/concurrency.ts`).

## Test Coverage

**Phase 1**:
- Model listing
- Chat completion (streaming/non-streaming)
- Prompt building
- Format resolution
- Reasoning rendering
- Progress event tracking

**Gaps**:
- Multi-agent call path filtering
- Large conversation handling
- Schema validation edge cases
- Concurrent request isolation

## Troubleshooting

### Model not found
- Check model ID in modelIdMap
- Verify agent registration
- Review buildModelId logic

### JSON format errors
- Check json_schema provided
- Verify agent.outputSchema exists
- Review response_format.type

### Streaming not working
- Check stream: true in request
- Verify SSE headers set
- Review writeSseChunk calls

### Reasoning not rendered
- Check transactionHeader initialized
- Verify turn structure built
- Review flushReasoning calls

### Usage counts incorrect
- Check accounting entries collected
- Verify LLM entry filtering
- Review collectUsage aggregation
