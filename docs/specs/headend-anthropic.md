# Anthropic Completions Headend

## TL;DR
Anthropic-compatible messages API exposing agents as models with streaming SSE support, thinking/text content blocks, and progress event rendering.

## Source Files
- `src/headends/anthropic-completions-headend.ts` - Full implementation (932 lines)
- `src/headends/concurrency.ts` - ConcurrencyLimiter
- `src/headends/http-utils.ts` - HTTP/SSE utilities
- `src/headends/summary-utils.ts` - Markdown formatting

## Headend Identity
- **ID**: `anthropic-completions:{port}`
- **Kind**: `anthropic-completions`
- **Label**: `Anthropic chat headend (port {port})`

## Configuration

```typescript
interface Options {
  port: number;              // HTTP server port
  concurrency?: number;      // Max concurrent requests (default: 10)
}
```

## Construction

**Location**: `src/headends/anthropic-completions-headend.ts:124-135`

```typescript
constructor(registry: AgentRegistry, options) {
  this.registry = registry;
  this.options = options;
  this.id = `anthropic-completions:${String(options.port)}`;
  this.label = `Anthropic chat headend (port ${port})`;
  const limit = options.concurrency > 0 ? Math.floor(options.concurrency) : 10;
  this.limiter = new ConcurrencyLimiter(limit);
  this.closed = this.closeDeferred.promise;
  this.refreshModelMap();
}
```

## API Endpoints

| Method | Path | Handler |
|--------|------|---------|
| GET | `/health` | Health check |
| GET | `/v1/models` | List available models |
| POST | `/v1/messages` | Create message |

## Model Mapping

**Location**: `src/headends/anthropic-completions-headend.ts:903-924`

```typescript
buildModelId(meta: AgentMetadata, seen: Set<string>): string {
  const baseSources = [meta.toolName, path.basename(meta.promptPath), meta.id.split('/').pop()];
  let root = baseSources.find(s => s?.length > 0) ?? 'agent';
  root = root.replace(/\.ai$/i, '') || 'agent';

  let candidate = root;
  let counter = 2;
  while (seen.has(candidate)) {
    candidate = `${root}_${counter}`;  // Underscore separator
    counter += 1;
  }
  seen.add(candidate);
  return candidate;
}
```

Model IDs use underscore separator (vs dash in OpenAI headend).

## Models List Response

```typescript
{
  data: [
    {
      id: modelId,
      type: 'model',
      display_name: agent.description ?? modelId,
    }
  ]
}
```

## Messages Request

**Location**: `src/headends/anthropic-completions-headend.ts:248-797`

### Request Schema
```typescript
interface AnthropicRequestBody {
  model: string;                          // Agent model ID
  messages: AnthropicRequestMessage[];    // Conversation
  system?: string | string[];             // System prompt(s)
  stream?: boolean;                       // Enable SSE streaming
  format?: string;                        // Output format override
  payload?: Record<string, unknown>;      // Additional parameters
}

interface AnthropicRequestMessage {
  role: 'user' | 'assistant';
  content: string | AnthropicMessageContent[];
}

interface AnthropicMessageContent {
  type?: string;
  text?: string;
}
```

### Prompt Composition

**Location**: `src/headends/anthropic-completions-headend.ts:812-840`

```typescript
composePrompt(system, messages): string {
  // System context from system field
  // Conversation history with User:/Assistant: labels
  // Final user message with User request: prefix
}
```

Three sections joined by `\n\n`:
1. `System context:\n{system}`
2. `Conversation so far:\nUser: ...\nAssistant: ...`
3. `User request:\n{finalUser}`

## Streaming Response Format

### SSE Event Types

```typescript
// Message start
{
  type: 'message_start',
  message: {
    id: requestId,
    type: 'message',
    role: 'assistant',
    model: modelName,
    content: [],
  },
}

// Content block start (thinking)
{
  type: 'content_block_start',
  content_block: { type: 'thinking' },
}

// Content block start (text)
{
  type: 'content_block_start',
  content_block: { type: 'text' },
}

// Thinking delta
{
  type: 'content_block_delta',
  content_block: {
    type: 'thinking',
    thinking_delta: text,
  },
}

// Text delta
{
  type: 'content_block_delta',
  content_block: {
    type: 'text',
    text_delta: chunk,
  },
}

// Content block stop
{
  type: 'content_block_stop',
}

// Message stop
{
  type: 'message_stop',
}
```

### Block Management

```typescript
openTextBlock(): void {
  if (!textBlockOpen) {
    writeSseChunk(res, { type: 'content_block_start', content_block: { type: 'text' } });
    textBlockOpen = true;
  }
}

closeTextBlock(): void {
  if (textBlockOpen) {
    writeSseChunk(res, { type: 'content_block_stop' });
    textBlockOpen = false;
  }
}

openThinkingBlock(): void {
  if (!thinkingBlockOpen) {
    writeSseChunk(res, { type: 'content_block_start', content_block: { type: 'thinking' } });
    thinkingBlockOpen = true;
  }
}

closeThinkingBlock(): void {
  if (thinkingBlockOpen) {
    writeSseChunk(res, { type: 'content_block_stop' });
    thinkingBlockOpen = false;
  }
}
```

Blocks closed before opening different type.

## Non-Streaming Response

```typescript
interface AnthropicResponseBody {
  id: string;
  type: 'message';
  role: 'assistant';
  model: string;
  content: [
    { type: 'thinking'; thinking: string },  // Optional
    { type: 'text'; text: string }
  ];
  stop_reason: 'end_turn' | 'error';
  usage: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  };
}
```

Thinking block included if reasoning content exists.

## Reasoning Rendering

- `## <agent>: <txnId>` header (plain text, escaped) aligns with the OpenAI-compatible headend.
- Each turn logs as `### Turn N` with the full metrics snapshot shown on the next line inside parentheses.
- Real Anthropic thinking deltas flow between blank-line hairlines (`\n\n---\n`). The block stays open while new thinking arrives and only closes when the renderer needs to output a synthetic line (progress entry, next heading, or the final summary).
- Progress bullets remain `- **callPath**: ...` just like before.
- The final line reads `SUMMARY: <agent>, duration **…**, cost **$…**, agents N, tools M, tokens …` without origin/txn metadata; both streaming and non-streaming consumers receive the same text.

## Progress Event Handling

**Location**: `src/headends/anthropic-completions-headend.ts:485-567`

Only handles events where `event.agentId === agent.id`.

Events:
- `agent_started`: `{callPath}: started {reason}`
- `agent_update`: `{callPath}: update {message}`
- `agent_finished`: `{callPath}: finished {metrics}`
- `agent_failed`: `{callPath}: failed: {error}, {metrics}`

## Token Usage Tracking

**Location**: `src/headends/anthropic-completions-headend.ts:92-102`

```typescript
collectUsage(entries): { input: number; output: number; total: number } {
  const usage = entries
    .filter((entry) => entry.type === 'llm')
    .reduce((acc, entry) => {
      acc.input += entry.tokens.inputTokens;
      acc.output += entry.tokens.outputTokens;
      return acc;
    }, { input: 0, output: 0 });
  return { ...usage, total: usage.input + usage.output };
}
```

Uses `input`/`output` field names (vs `prompt`/`completion` in OpenAI).

## Format Resolution

**Location**: `src/headends/anthropic-completions-headend.ts:261-265`

```typescript
const format = body.format ?? agent.expectedOutput?.format ?? 'markdown';
const schema = format === 'json' ? (agent.outputSchema ?? extractSchema(body.payload)) : undefined;
if (format === 'json' && schema === undefined) {
  throw HttpError(400, 'missing_schema', 'JSON format requires schema');
}
```

Priority: explicit format > agent default > 'markdown'.

## Schema Extraction

**Location**: `src/headends/anthropic-completions-headend.ts:850-857`

```typescript
extractSchema(payload?: Record<string, unknown>): Record<string, unknown> | undefined {
  if (payload === undefined) return undefined;
  const val = payload.schema;
  if (val !== null && typeof val === 'object' && !Array.isArray(val)) {
    return val;
  }
  return undefined;
}
```

## Transaction Summary

Computed from accounting entries if not provided by progress events:
- Duration (max - min timestamp)
- Token counts (in, out, cache read, cache write)
- Tools run count
- Agents run count
- Total cost USD

Appended to final text block with origin txnId.

## Error Handling

### Streaming Errors
```typescript
const errorEvent = { type: 'error', message };
writeSseChunk(res, errorEvent);
writeSseDone(res);
```

### Non-Streaming Errors
```typescript
writeJson(res, status, { error: code, message });
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `port` | HTTP server listen port |
| `concurrency` | Max concurrent requests |
| `registry` | Available agents as models |

## Telemetry

**Labels**:
- `headend: anthropic-completions:{port}`
- All base telemetry labels

## Logging

**Severity levels**:
- `VRB`: Starting, listening, started, completion finished
- `WRN`: Client abort
- `ERR`: Request failure, concurrency failure

## Events

**Request lifecycle**:
- Abort detection
- Shutdown signal
- Block open/close state
- Response completion

## Invariants

1. **Model ID separator**: Underscore (not dash)
2. **JSON schema required**: JSON format needs schema
3. **Block exclusivity**: Only one block type open at a time
4. **System separate**: System prompt in dedicated field
5. **Content array**: Multiple text items joined with newlines
6. **Turn tracking**: New turn on root agent LLM entry
7. **Unauthenticated surface**: No `Authorization` header validation; production deployments must use API gateway or network restrictions

## Business Logic Coverage (Verified 2025-11-16)

- **Format enforcement**: Request `format` field mirrors agent defaults, and JSON output requires a schema before execution just like the OpenAI headend, ensuring identical error semantics (`src/headends/anthropic-completions-headend.ts:248-420`).
- **Thinking/text block parity**: SSE events emit explicit `content_block_start` / `content_block_delta` events for `thinking` vs `text`, preserving Claude’s block semantics so frontends can draw dual-stream UI (`src/headends/anthropic-completions-headend.ts:350-520`).
- **Transaction summaries**: Each response prepends a Markdown header summarizing txnId/callPath, matching Slack + REST progress logs (`src/headends/anthropic-completions-headend.ts:300-360, 550-612`).
- **Tool call rejection**: Requests containing `messages[].tool_calls` are rejected early, matching Anthropic’s Messages contract (no function calling) and preventing inconsistent histories (`src/headends/anthropic-completions-headend.ts:270-310`).
- **Concurrency limiter/abort hooks**: Requests acquire limiter slots and propagate client aborts through `AbortController`, guaranteeing immediate session cancellation and slot release when callers disconnect (`src/headends/anthropic-completions-headend.ts:188-240`).

## Key Differences from OpenAI Headend

| Feature | OpenAI | Anthropic |
|---------|--------|-----------|
| Endpoint | `/v1/chat/completions` | `/v1/messages` |
| Model ID separator | Dash (`-`) | Underscore (`_`) |
| System prompt | In messages array | Separate field |
| Token fields | `prompt`/`completion` | `input`/`output` |
| Reasoning field | `reasoning_content` | `thinking_delta` |
| Stop reason | `stop`/`error` | `end_turn`/`error` |
| Block structure | Delta-based | Explicit start/stop |

## Test Coverage

**Phase 1**:
- Model listing with display names
- Message creation (streaming/non-streaming)
- Prompt composition
- Content block management
- Thinking block rendering
- Progress event handling

**Gaps**:
- Nested content arrays
- Multi-system prompt merging
- Large thinking buffer handling
- Block state consistency

## Troubleshooting

### Model not found
- Check model ID in modelIdMap
- Verify agent registration
- Note underscore separator

### Thinking not appearing
- Check thinkingBlockOpen state
- Verify openThinkingBlock called
- Review closeTextBlock before thinking

### Content blocks malformed
- Check block open/close pairing
- Verify state tracking variables
- Review SSE chunk format

### System prompt ignored
- Check system field (string or array)
- Verify composePrompt concatenation
- Review SYSTEM_PREFIX constant

### Usage counts wrong
- Check accounting entries
- Verify LLM entry filtering
- Review input/output field names
