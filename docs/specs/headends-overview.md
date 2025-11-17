# Headends Overview

## TL;DR
Network service endpoints exposing agent functionality. Six types: MCP, OpenAI-compatible, Anthropic-compatible, REST API, Slack, custom. HeadendManager orchestrates lifecycle.

## Source Files
- `src/headends/types.ts` - Core headend interfaces
- `src/headends/headend-manager.ts` - Lifecycle orchestration
- `src/headends/mcp-headend.ts` - MCP server mode (~39KB)
- `src/headends/openai-completions-headend.ts` - OpenAI API compat (~40KB)
- `src/headends/anthropic-completions-headend.ts` - Anthropic API compat (~37KB)
- `src/headends/rest-headend.ts` - REST API endpoint (~16KB)
- `src/headends/slack-headend.ts` - Slack Socket Mode (~33KB)
- `src/headends/concurrency.ts` - Request concurrency limiting
- `src/headends/http-utils.ts` - HTTP utilities
- `src/headends/summary-utils.ts` - Response summarization
- `src/headends/mcp-ws-transport.ts` - MCP WebSocket transport

## Headend Architecture

### Headend Interface
**Location**: `src/headends/types.ts:30-37`

```typescript
interface Headend {
  readonly id: string;
  readonly kind: HeadendKind;
  readonly closed: Promise<HeadendClosedEvent>;
  describe(): HeadendDescription;
  start(context: HeadendContext): Promise<void>;
  stop(): Promise<void>;
}
```

### HeadendKind
```typescript
type HeadendKind =
  | 'mcp'
  | 'openai-completions'
  | 'anthropic-completions'
  | 'api'
  | 'slack'
  | 'custom';
```

### HeadendContext
```typescript
interface HeadendContext {
  log: HeadendLogSink;
  shutdownSignal: AbortSignal;
  stopRef: { stopping: boolean };
}
```

### HeadendClosedEvent
```typescript
type HeadendClosedEvent =
  | { reason: 'stopped'; graceful: boolean }
  | { reason: 'error'; error: Error };
```

## HeadendManager

**Location**: `src/headends/headend-manager.ts`

**Responsibilities**:
1. Accept a fixed array of headends in the constructor
2. Start them sequentially via `startAll()` so port usage/log order is deterministic
3. Watch each headend’s `closed` promise and escalate the first fatal event via `onFatal`/`waitForFatal()`
4. Propagate shutdown signals and shared `stopRef` into every headend context
5. Stop all running headends concurrently and await watcher completion

**Lifecycle**:
- `startAll(context)` – sequentially invoke `headend.start(context)` and register watchers
- `waitForFatal()` – resolves with `{ headend, error }` when the first headend fails, or `undefined` after a clean stop
- `stopAll()` – marks `stopping=true`, calls `stop()` on each active headend, awaits watchers, then resolves `waitForFatal`

**Graceful Shutdown**:
- `stopRef.stopping = true` tells headends to reject new requests
- Active requests finish before `stop()` resolves
- All watcher promises settle to avoid dangling rejections

## Concrete Headends

### MCP Headend
**Kind**: `mcp`
**Protocol**: Model Context Protocol (stdio, HTTP/SSE, or WebSocket transports)

**Features**:
- Exposes each agent as a single MCP tool (name derived from prompt path/toolName) with a required `format` argument
- Supports stdio (single session) and network transports with per-session concurrency limits
- Requires `schema` when `format=json`, matching ai-agent’s internal contract for JSON outputs
- Streams assistant output via MCP `content` blocks; reasoning is included when providers emit `thinking`
- Shares session instructions (from `.ai-agent` metadata) through `getInstructions`

**Endpoints**:
- `tools/list` – returns the registered agent tool definitions
- `tools/call` – executes the selected agent

**Configuration**:
- CLI `--mcp stdio|http:PORT|sse:PORT|ws:PORT`
- Optional `instructions` string appended to MCP server metadata
- `concurrency` limit for non-stdio transports (default 10)

### OpenAI Completions Headend
**Kind**: `openai-completions`
**Protocol**: OpenAI Chat Completions API

**Features**:
- Drop-in `/v1/chat/completions` / `/v1/models` compatibility
- Streaming (SSE) support with `reasoning_content` deltas for thinking
- Rejects `tool_calls` payloads to match ai-agent’s agentic flow (tools are invoked internally)
- Enforces `format`/`json_schema` requirements before executing agents
- Emits transaction summary headers and cost/usage aggregates matching OpenAI’s schema

**Request Mapping**:
- `model` → Agent selection
- `messages` → Conversation history
- `tools` → Tool definitions (ignored, uses agent tools)
- `temperature`, `top_p`, `max_tokens`
- `stream` → Enable SSE

**Response Format**:
```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "agent-model",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "...",
        "tool_calls": [...]
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

### Anthropic Completions Headend
**Kind**: `anthropic-completions`
**Protocol**: Anthropic Messages API

**Features**:
- Drop-in `/v1/messages` compatibility with `/v1/models`
- Streaming (SSE) support with explicit `thinking` vs `text` content blocks
- Rejects Anthropic `tool_use` / `tool_result` payloads (tooling happens internally)
- Enforces `format` + schema requirements identical to OpenAI headend
- Includes cache/usage stats fields matching Anthropic responses

**Request Mapping**:
- `model` → Agent selection
- `messages` → Conversation
- `system` → System prompt
- `tools` → Tool definitions
- `max_tokens`
- `stream`

**Response Format**:
```json
{
  "id": "msg_...",
  "type": "message",
  "role": "assistant",
  "content": [...],
  "model": "agent-model",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 100,
    "output_tokens": 50,
    "cache_read_input_tokens": 20,
    "cache_creation_input_tokens": 0
  }
}
```

### REST Headend
**Kind**: `api`
**Protocol**: JSON over HTTP

**Features**:
- GET `/health` and GET `/v1/{agentId}?q=...&format=...` endpoints (GET-only surface)
- Requires `q` query parameter (escaped prompt)
- Optional `format` query parameter; JSON validation relies on the agent’s own schema (no query-level schema)
- Aggregates streamed `onOutput`/`onThinking` into `output` and `reasoning`, returning the final report alongside
- Supports extra routes registered by other modules (e.g., Slack slash commands)

**Response Fields**:
- `success` – boolean
- `output` – concatenated streamed text
- `reasoning` – aggregated thinking output (if any)
- `finalReport` – structured final_report payload
- `error` – string when session fails

### Slack Headend
**Kind**: `slack`
**Protocol**: Slack Socket Mode

**Features**:
- Socket Mode bot with mention, DM, and channel-post routing
- Routing rules driven by `slack.routing` (default, rules, deny) with channel glob patterns
- Prompt templates per engagement type plus configurable opener tones (`random`, `cheerful`, `formal`, `busy`)
- Slash-command webhook (REST extra route or fallback server) with signature verification
- Per-run concurrency limiting + progress updates streamed into Slack threads

**Events Handled**:
- `message` / `app_mention` – Primary engagement entry points (subject to routing rules)
- Slash commands – Routed through extra HTTP route (default `/ai-agent`)
- Progress updates – Posted as threaded messages with cost/usage summaries

**Message Flow**:
1. Resolve route (agent + prompt template) using channel patterns + `engage`
2. Acquire concurrency slot and start agent run
3. Post opener message using configured tone
4. Stream progress + reasoning into Slack thread while capturing transaction summaries
5. Post final report (markdown or Slack Block Kit)
6. Release run slot when done or on abort

**Configuration**:
- `slack.botToken`, `slack.appToken`, `slack.signingSecret`
- Optional `routing.default` / `routing.rules` / `routing.deny`
- `historyLimit`, `historyCharsCap`, `updateIntervalMs`, `mentions`, `dms`, `openerTone`

## Concurrency Management

**Location**: `src/headends/concurrency.ts`

**Features**:
- Concurrent session limiting via slot acquisition
- FIFO queue for pending requests (unbounded)
- AbortSignal support for cancellation
- Automatic release on completion

**Configuration**:
```typescript
{
  limit: number;  // Max concurrent requests (default: 10)
}
```

**Note**: The queue is unbounded - no maxQueueSize limit. Timeout enforcement is handled by callers, not the limiter itself.

**Key Methods**:
- `acquire(opts?)`: Returns release function when slot available
- `release()`: Releases slot (double-release protected)
- `maxConcurrency`: Returns configured limit

## Configuration

### Global Headend Config
```typescript
interface HeadendConfig {
  headends: HeadendDefinition[];
  gracefulShutdownTimeoutMs?: number;
  concurrency?: {
    maxConcurrentRequests: number;
    maxQueueSize: number;
  };
}
```

### Per-Headend Definition
```typescript
interface HeadendDefinition {
  id: string;
  kind: HeadendKind;
  enabled?: boolean;
  config: HeadendSpecificConfig;
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `headends[].enabled` | Enable/disable specific headend |
| `gracefulShutdownTimeoutMs` | Max time for graceful stop |
| `concurrency.maxConcurrentRequests` | Parallel request limit |
| `concurrency.maxQueueSize` | Pending request queue limit |

## Telemetry

**Per Request**:
- Request ID
- Headend ID
- Latency
- Status (success/error)
- Token usage (when applicable)

**Per Headend**:
- Active connections
- Request count
- Error count
- Uptime

## Logging

**Startup Logs**:
- Headend registration
- Transport binding (port, socket)
- Configuration summary

**Request Logs**:
- Request received
- Session created
- Progress updates
- Response sent
- Errors

**Lifecycle Logs**:
- Start/stop events
- Connection events
- Error conditions

## Events

Headends emit events through the context:
- Request started
- Request completed
- Error occurred
- Shutdown initiated

## Business Logic Coverage (Verified 2025-11-16)

- **Concurrency limiter**: All network headends share `ConcurrencyLimiter` (`src/headends/concurrency.ts`) which supports abortable acquire/release; CLI `--api-concurrency`, `--openai-completions-concurrency`, etc., feed those limits.
- **Shared telemetry labels**: `getTelemetryLabels()` tags each headend with `{ headend: id }` so OTLP metrics/traces can filter per surface (`src/headends/*-headend.ts:70-90`).
- **Progress propagation**: REST/OpenAI/Anthropic headends forward `onProgress` events to clients (Slack posts to threads), ensuring the `SUMMARY: <agent>, ...` line and the turn-by-turn Markdown transcript stay identical across surfaces (`src/headends/openai-completions-headend.ts:311-448`, `src/headends/anthropic-completions-headend.ts:300-360`, `src/headends/rest-headend.ts:242-358`).
- **Slash route reuse**: Slack headend registers a REST extra route when available; otherwise it spawns a tiny fallback HTTP server, so slash commands work even without REST headend present (`src/headends/slack-headend.ts:98-305`, `src/headends/rest-headend.ts:176-208`).

## Test Coverage

**Unit Tests**:
- Request/response mapping
- Protocol compliance
- Error handling
- Concurrency limits

**Integration Tests**:
- End-to-end API calls
- Streaming behavior
- Multi-headend scenarios

**Gaps**:
- Slack Socket Mode edge cases
- WebSocket reconnection
- Load testing scenarios

## Troubleshooting

### Headend fails to start
- Check port availability
- Check credentials (Slack tokens, etc.)
- Check network permissions

### Request timeout
- Check concurrency limits
- Check agent execution time
- Check graceful shutdown state

### Protocol mismatch
- Check request format (OpenAI vs Anthropic)
- Check required headers
- Check content-type

### Slack integration issues
- Verify bot token scopes
- Check Socket Mode enabled
- Verify event subscriptions

### Memory leaks
- Check session cleanup
- Check WebSocket closures
- Monitor concurrent request count

### Graceful shutdown hangs
- Check in-flight requests
- Verify stopRef propagation
- Check timeout settings
