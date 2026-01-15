# AI Agent - Universal LLM Tool Calling Interface

A TypeScript-based command-line tool and library for interacting with Large Language Models (LLMs) with Model Context Protocol (MCP) tool integration.

## Overview

The AI Agent provides a unified interface to interact with multiple LLM providers while seamlessly integrating with MCP (Model Context Protocol) tools. It supports streaming responses, parallel tool execution, and comprehensive accounting. Configuration lives in a JSON file; by default the local filename is `.ai-agent.json`.

### Principle: Follow the AI SDK Contract

Our conversation schemas, reasoning payloads, tool definitions, and streaming expectations MUST stay aligned with the current Vercel AI SDK contract. Track SDK releases, update the agent promptly when upstream schemas evolve, and avoid introducing local abstractions that drift from the SDK’s data model.

## High-Level Operation

1. **Configuration**: All settings are stored in a JSON config (default `.ai-agent.json`) including provider API keys and MCP server definitions
2. **Execution**: Command-line interface accepts providers, models, MCP tools, and prompts (positionals), plus optional flags
3. **Bootstrap**: Validates config and initializes MCP servers. Initialization is non-fatal: failures are logged and the agent can still proceed to the LLM. Use `--trace-mcp` to inspect initialization details. In `--dry-run`, both MCP spawn and LLM calls are skipped. Shared MCP servers retry initialization indefinitely using an exponential backoff (0, 1, 2, 5, 10, 30, 60 seconds; 60 s repeats) and log every restart decision/failure with `ERR`. The only way to stop retries is to disable the server and restart the agent. Private (`shared: false`) servers retain the single-retry behavior. Shared transports now also monitor `onclose` signals (stdio child exit, websocket close, streamable HTTP disconnect) and immediately enqueue the same restart loop, so a dead MCP process is recovered before the next request times out.
4. **Processing**: Sends requests to LLMs with available tools (schemas). The LLM orchestrates repeated tool calls. The agent preserves assistant tool_calls and tool results in history and loops until completion (see Agentic Behavior)
5. **Output**: Streams LLM responses to stdout in real time, logs errors to stderr. The transport is fixed to XML-final: normal tools are called natively (tool_calls) and the final report must be returned inside the XML tag using the session nonce; progress remains native.

## Agentic Behavior

- The agent is fully agentic: the LLM decides when to call tools, with repeated invocations across multiple turns.
- Tool calls are preserved as assistant messages with `tool_calls` (id, name, arguments). Tool results are preserved as `tool` role messages with `tool_call_id`. When the model calls an unqualified tool name that exactly matches a tool suffix and the payload validates against that tool’s schema, the agent auto-corrects the tool name, logs a warning, and records the corrected name in history. Missing tool results injected by the SDK are tagged with `metadata.injectedToolResult=true` and `metadata.injectedReason`; auto-correction only re-executes when the reason is `tool_missing`.
- The next turn always includes all prior assistant/user/tool messages in order, giving the LLM full transparency into requests and responses.
- A maximum tool-turns cap (`defaults.maxTurns`) is enforced. On the final allowed turn, tools are disabled and XML-NEXT carries the final-turn instruction; no additional system notices are injected outside XML-NEXT and TURN-FAILED. This guarantees a final answer without an error.
- Stop-reason handling: if the upstream stop reason is exactly `refusal` or `content-filter`, the turn is treated as a hard failure (`invalid_response`); the truncated response is discarded, and the retry/fallback flow takes over.

## Orchestration Patterns (Advisors, Router, Handoff)

Orchestration is frontmatter-driven and executed by `AIAgent.run()` around the inner `AIAgentSession.run()` loop.

- **Advisors (pre-run)**: each advisor runs in parallel with the original user prompt; their outputs are injected into the main user prompt as `<advisory>` blocks. Failures produce a synthetic advisory block describing the error so the main agent can continue.
- **Router (post-run)**: when `router.destinations` is configured, the session exposes the internal tool `router__handoff-to` with parameters `{ agent, message? }`. If the router calls this tool, the request is delegated to the selected destination and the router’s optional message becomes an advisory block for that destination.
- **Handoff (post-run)**: when `handoff` is configured, the agent’s final response is passed to the handoff target as `<response agent="...">` alongside the original user request.
- **Precedence**: if both router and handoff are configured, the router chain completes first and the parent handoff runs last.

## Hardcoded Strings (LLM-facing)

LLM-facing strings are centralized in `src/llm-messages.ts`, `src/llm-messages-xml-next.ts`, and `src/llm-messages-turn-failed.ts` (plus XML templates they render). This includes:
- XML-NEXT notices (per-turn instructions + final-turn guidance)
- TURN-FAILED guidance
- Tool-result failure text for malformed tool calls
- XML protocol notices and examples

There is no separate “short list” of hardcoded strings; treat these files as the canonical source of truth.

## Command Line Interface

The CLI supports two operating modes:

1. **Headend mode** – register one or more `.ai` prompts as services via repeatable `--agent` flags, then start the desired headends.
2. **Direct invocation** – run a single agent immediately by providing system and user prompts (legacy CLI flow).

### Headend Mode

```bash
ai-agent \
  --agent agents/master.ai \
  --agent agents/sub/task.ai \
  --api 8123 \
  --mcp stdio \
  --mcp http:8124 \
  --openai-completions 8082 \
  --anthropic-completions 8083 \
  --embed 8090
```

Every headend flag is repeatable. The headend manager instantiates each endpoint independently while sharing the same agent registry.

| Flag | Purpose | Notes |
|------|---------|-------|
| `--agent <path>` | Register an `.ai` file (repeat as needed) | Sub-agents referenced in frontmatter are auto-loaded. |
| `--api <port>` | REST API headend | `GET /health`, `GET /v1/:agent?q=...&format=...`. `format` defaults to `markdown`; when you request `format=json` ensure the agent advertises a JSON schema. |
| `--mcp <transport>` | MCP headend | Accepted values: `stdio`, `http:PORT`, `sse:PORT`, `ws:PORT`. Tool calls **must** include a `format` argument; when `format=json` the payload must also provide a `schema` object. |
| `--openai-completions <port>` | OpenAI Chat Completions compatibility | `/v1/models`, `/v1/chat/completions` (supports SSE streaming). |
| `--anthropic-completions <port>` | Anthropic Messages compatibility | `/v1/models`, `/v1/messages` (streams via SSE). |
| `--embed <port>` | Public embed headend | `GET /ai-agent-public.js`, `POST /v1/chat` (SSE), `GET /health`. |

Optional per-headend concurrency guards are available: `--api-concurrency <n>`, `--openai-completions-concurrency <n>`, `--anthropic-completions-concurrency <n>`, `--embed-concurrency <n>`. Each incoming request acquires a slot before spawning an agent session.

### Direct Invocation

When no headends are requested, the CLI expects system and user prompts:

```bash
ai-agent --agent prompt.ai @system.txt "Summarize the attached notes"
```

System prompts can be passed inline, via `@file`, or `-` (stdin). The final positional argument is the user prompt. Provider/model/tool selections are supplied with the existing options registry (`--models`, `--tools`, etc.).

## Configuration File (.ai-agent.json)

The configuration file must contain providers and MCP tools. Models are specified at runtime. The default local filename is `.ai-agent.json`.
Provider `type` values: `openai`, `openai-compatible`, `anthropic`, `google`, `openrouter`, `ollama`, `test-llm`.

### Configuration File Resolution
The configuration file is resolved in the following order:
1. `--config <filename>` command line option
2. `.ai-agent.json` in current directory  
3. `~/.ai-agent.json` in home directory
4. If none found, the program fails with error

### Environment Variable Expansion
All string values in the configuration support environment variable expansion using `${VARIABLE_NAME}` syntax:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "https://api.openai.com/v1"
    },
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}",
      "baseUrl": "https://api.anthropic.com/v1"
    },
    "ollama": {
      "baseUrl": "http://localhost:11434"
    }
  },
  "mcpServers": {
    "file-operations": {
      "type": "stdio",
      "command": "/usr/local/bin/file-mcp-server",
      "args": []
    },
    "netdata-tools": {
      "type": "websocket",
      "url": "ws://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer ${NETDATA_TOKEN}"
      }
    },
    "remote-api": {
      "type": "http",
      "url": "https://api.example.com/mcp",
      "headers": {
        "X-API-Key": "${REMOTE_API_KEY}"
      },
      "queue": "fetcher"
    }
  },
  "queues": {
    "default": { "concurrent": 32 },
    "fetcher": { "concurrent": 4 }
  },
  "cache": {
    "backend": "sqlite",
    "sqlite": { "path": "${HOME}/.ai-agent/cache.db" },
    "maxEntries": 5000
  },
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
  },
  "defaults": {
    "llmTimeout": 120000,
    "toolTimeout": 60000,
    "temperature": 0.2,
    "topP": null,
    "topK": null,
    "repeatPenalty": null,
    "stream": true
  }
}
```

### Tool Queue Configuration

- Define process-wide concurrency by declaring `queues` in `.ai-agent.json`. Each queue specifies a `concurrent` slot count and the agent automatically injects a `default` queue if omitted.
- MCP servers, REST tools, and OpenAPI generated operations can bind to a queue by setting `queue: "name"`. When omitted they fall back to `default`.
- Only external tools participate in queueing; internal agent helpers (`agent__final_report`, `agent__task_status`, `agent__batch`) bypass queues so they never deadlock against their parents.
- Every tool execution is routed through the queue manager. When a tool must wait for a slot the agent logs a `queued` entry and emits telemetry via `ai_agent_queue_depth` (gauge of in-use/waiting slots) and `ai_agent_queue_wait_duration_ms` (histogram + last-wait gauge).

### Response Caching (Global)

- Optional global cache config lives at top-level `cache` in `.ai-agent.json`. If omitted, enabling a cache TTL uses the default SQLite backend at `${HOME}/.ai-agent/cache.db` with `maxEntries=5000` (when SQLite is available).
- Per-agent TTLs come from frontmatter/CLI `cache` (`off` | `<ms>` | `<N.Nu>` where `u ∈ { ms, s, m, h, d, w, mo, y }`).
- Per-tool TTLs are configured via `mcpServers.<name>.cache`, `mcpServers.<name>.toolsCache.<tool>`, and `restTools.<tool>.cache`.
- Cache keys are SHA hashes of:
  - Agents: `agentHash + userPrompt + expected format/schema`.
  - Tools: `tool identity (namespace + name) + request payload`.
  Cache hits are logged; misses are silent.
- `agentHash` is computed by the registry from the fully expanded prompt (after `${include:...}` resolution, before other `${VAR}` expansion) plus the resolved agent config. Paths and agent names are not part of the identity.
- If the SQLite backend cannot be loaded, caching is disabled unless Redis is configured.

### Context Window Configuration

- Each provider/model pair should declare a `contextWindow` (maximum tokens permitted by the upstream model) and may optionally specify a canonical `tokenizer` identifier (for example `tiktoken:gpt-4o`). When no explicit window is supplied, the agent falls back to an internal ceiling of **131072 tokens** so the guard remains active even with sparse configuration.
- The agent reserves a safety margin (`contextWindowBufferTokens`, default 256) to ensure forced-final-turn messages fit inside the remaining budget. This value can be set globally under `defaults` or overridden per provider/model.
- Before every tool result is appended and before every LLM turn, the agent estimates the token footprint using the configured tokenizer. If the projected usage would exceed `contextWindow - maxOutputTokens - buffer`, the tool call is rejected with `(tool failed: context window budget exceeded)` and the session is forced into a final-turn flow.
- Context guard activations are exported as telemetry: `ai_agent_context_guard_events_total{provider,model,trigger,outcome}` counts each activation, and `ai_agent_context_guard_remaining_tokens{provider,model,trigger,outcome}` reports the most recent remaining budget, enabling dashboards to track proximity to limits.
- When the guard fires, the session emits `EXIT-TOKEN-LIMIT` and augments tool accounting entries with `projected_tokens`, `limit_tokens`, and `remaining_tokens` so downstream systems can audit the decision.

### Per-Model Parameter Overrides

Some providers require bespoke sampling controls for specific models. Add a `models`
block under the provider entry to override `temperature`, `top_p`/`topP`, `top_k`/`topK`,
or `repeat_penalty`/`repeatPenalty` values that will be sent to the LLM. Overrides
take precedence over CLI options, frontmatter, and config defaults. Use `null` to
omit a parameter entirely.

```json
{
  "providers": {
    "openrouter": {
      "apiKey": "${OPENROUTER_KEY}",
      "models": {
        "meta-llama/llama-3": {
          "overrides": {
            "top_p": null,
            "repeat_penalty": null
          }
        },
        "openai/gpt-4o-mini": {
          "overrides": {
            "temperature": 0.2,
            "topP": 1.0
          }
        }
      }
    }
  }
}
```

In this example `meta-llama/llama-3` never receives a `top_p` value, while
`openai/gpt-4o-mini` always uses the specified temperature and top-p values.

#### Interleaved reasoning replay (per-model)

Some models require their previous reasoning to be replayed using provider-native fields.
Use `models.<model>.interleaved` to enable this replay. It accepts:
- `true` → inject `reasoning_content`
- `"reasoning_content"` or any other string → inject that field name

When enabled, reasoning segments are removed from the assistant content and the merged
reasoning text is injected into `providerOptions.openaiCompatible.<field>` for **each**
assistant message that has reasoning (unless that reasoning text already appears verbatim in the
assistant message content for the same turn).

### Command Line vs Configuration Priority
- **Flags override config**: Command line options override configuration file settings
- **Defaults in config**: All timeout, limit, and model parameters can be set in the config file under `"defaults"`
- **Agent registration vs. direct prompts**: Headend mode discovers agents through repeatable `--agent` flags. Direct invocation still accepts positional system/user prompts, while providers/models/tools are driven by CLI options (e.g., `--models`, `--tools`).

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--agent <path>` | Register an agent file for headend mode (repeatable) | - |
| `--api <port>` | Start REST API headend on `<port>` (repeatable) | 4 concurrent requests (configurable via `--api-concurrency`) |
| `--mcp <transport>` | Start MCP headend (`stdio`, `http:PORT`, `sse:PORT`, `ws:PORT`) | HTTP uses `POST /mcp`; SSE pair (`GET /mcp/sse`, `POST /mcp/sse/message`); WebSocket speaks the `mcp` subprotocol. |
| `--openai-completions <port>` | Start OpenAI Chat Completions compatible headend | 4 concurrent requests (configurable via `--openai-completions-concurrency`) |
| `--anthropic-completions <port>` | Start Anthropic Messages compatible headend | 4 concurrent requests (configurable via `--anthropic-completions-concurrency`) |
| `--embed <port>` | Start public embed headend | Configurable via `--embed-concurrency` |
| `--api-concurrency <n>` | Max concurrent REST sessions per headend | 4 |
| `--openai-completions-concurrency <n>` | Max concurrent OpenAI chat sessions | 4 |
| `--anthropic-completions-concurrency <n>` | Max concurrent Anthropic chat sessions | 4 |
| `--embed-concurrency <n>` | Max concurrent embed sessions | 10 |
| `--llm-timeout <ms>` | Inactivity timeout per LLM call (resets on stream) | 120000 |
| `--tool-timeout <ms>` | Timeout for tool execution | 60000 |
| `--trace-llm` | Trace LLM HTTP requests and responses (Authorization redacted) | off |
| `--trace-mcp` | Trace MCP connect/list operations and callTool payloads | off |
| `--parallel-tool-calls` | Enable provider-native parallel tool calls | true |
| `--no-parallel-tool-calls` | Disable provider parallelism | - |
| `--temperature <n>` | LLM temperature (0.0–2.0) | 0.7 |
| `--top-p <n>` | LLM top-p sampling (0.0–1.0) | 1.0 |
| `--stream` | Force streaming responses | on |
| `--no-stream` | Disable streaming | - |
| `--save <filename>` | Save conversation transcript (direct mode) | - |
| `--load <filename>` | Load conversation history (direct mode) | - |
| `--config <filename>` | Explicit configuration file | See resolution order |
| `--accounting <filename>` | Override accounting file path | - |
| `--dry-run` | Validate inputs only; skip MCP spawn and LLM work | - |

Notes:
- `--save/--load`: When loading a conversation, the system prompt is replaced by the provided system prompt (or loaded one if none provided) and the new user prompt is appended. MCP tool instructions are appended to the system prompt at runtime (once) and are not stored as separate messages.
- MCP and OpenAI tool calls that declare `format=json` must also provide a `schema` object so the agent can validate the response.

## Main Processing Loop

0. **MCP Bootstrap**: Validate MCP servers, connect, fetch tool schemas and any instructions. Initialization errors are non‑fatal: they are logged and the agent may proceed to the LLM without those tools.
1. **Validation**: Validate configuration parameters - exit on any error
2. **LLM Request**: Send request to LLM with available tools (schemas) and conversation history
3. **Response Handling**:
   - If no tools requested: Stream output to stdout in real time and exit with code 0
   - If tools requested: Stream any assistant text to stdout, then proceed to tool execution
4. **Tool Execution**:
- Tool selection and execution are handled by the provider/AI SDK; the application does not impose a tool count limit. Concurrency is enforced by the queue manager (`queues` + `queue` bindings) rather than per-session flags, so heavy MCP servers can be throttled globally regardless of how many agents spawn them.
  - Never retry a tool call
  - Wait for all tools to complete (successful or failed)
  - Record each tool result in message history in the exact order specified by the LLM
5. **Loop**: Return to step 2 with updated conversation history

## Prompt Variable Substitutions

You can reference environment/system/context variables directly in your system and user prompts. Both `${VAR}` and `{{VAR}}` forms are supported.

Supported variables:

- `DATETIME`: Current local date/time in RFC 3339 with numeric offset, e.g. `2025-08-31T02:05:07+03:00`.
- `TIMESTAMP`: Current Unix epoch timestamp in seconds, e.g. `1733437845`.
- `DAY`: Weekday name, e.g. `Monday`.
- `TIMEZONE`: IANA time zone when available (e.g. `Europe/Athens`), else `TZ` env or `UTC`.
- `MAX_TURNS`: The configured maximum tool turns for the agent loop.
- `MAX_TOOLS`: The configured maximum tool calls per turn (after overrides), minimum 1.
- `OS`: Operating system name. On Linux, uses `/etc/os-release` PRETTY_NAME if available (e.g. `Ubuntu 24.04.1 LTS`) and appends kernel (e.g. `(kernel 6.8.0-41-generic)`). Otherwise falls back to `os.type() os.release()`.
- `ARCH`: CPU architecture, e.g. `x64`, `arm64`.
- `KERNEL`: Kernel string as `os.type() os.release()`, e.g. `Linux 6.8.0-41-generic`.
- `CD`: Current working directory (`process.cwd()`).
- `HOSTNAME`: Host name of the running machine.
- `USER`: Username of the invoking user (from `os.userInfo()` or `USER`/`USERNAME` env fallback).

Notes:

- Unknown variables are left unchanged in the prompt.
- Substitutions occur after reading prompts from `@file` or stdin and before appending tools' instructions.

## Core Requirements

### Tool Execution Rules
- **MANDATORY**: Every tool call must have a corresponding response in message history
- **Order Preservation**: Tool responses must maintain the exact order they were received from the LLM
- **Failure Handling**: Failed tools are included in history with error details; tools are never retried
- **Error Messages**: If a tool can't be executed and no response message is received, generate an explanatory error message
- **Parallel Execution**: Tools run concurrently but results are ordered correctly
- **Performance Tracking**: Include latency and request/response size accounting for each tool execution
- Task Status (`agent__task_status`) and batch wrapper (`agent__batch`) always return a response but do **not** satisfy turn success criteria.
- Turn success requires either (a) an accepted/valid final report, or (b) at least one executed non-progress/batch tool call. Turns advance only when successful.
- Failed turns emit exactly one WRN log with slugged reasons and the full LLM response (capped ~128 KB). Retry exhaustion fails the session immediately with a single ERR log and a synthetic session report. Collapse logic may only shrink `maxTurns`, never increase it.
<!-- Tool limits are now handled by the provider/SDK; no app-level limit option. -->

#### Tool Response Size Cap
- A configurable maximum size (bytes) determines when a tool result is **stored** and replaced with a `tool_output` handle message.
- If a tool returns content larger than the limit **or** would overflow the context budget, the agent:
  - Stores the sanitized output in the per-session `tool_output` store.
  - Replaces the tool result with a handle message instructing the model to call `tool_output(handle=..., extract=...)`.
  - Logs a warning that includes `handle`, `reason` (`size_cap|token_budget|reserve_failed`), `bytes`, `lines`, and `tokens`.
- Truncation happens **only inside the `tool_output` module** as a fallback when extraction fails.
- Configuration surfaces (highest precedence first):
  - CLI: `--tool-response-max-bytes <n>`
  - Frontmatter: `toolResponseMaxBytes: <number>`
  - Config defaults: `.ai-agent.json` → `defaults.toolResponseMaxBytes`
- Tool output module overrides (optional):
  - Frontmatter: `toolOutput: { enabled, storeDir, maxChunks, overlapPercent, avgLineBytesThreshold, models }`
  - Config: `.ai-agent.json` → `toolOutput`
- Default when unspecified: `12288` (12 KB).

### Provider Fallback
- Models are tried sequentially on failure (model-first), and for each model all providers are attempted in order
- Each model/provider attempt receives the **exact same request** without modifications
- Streaming behavior: assistant tokens are streamed to stdout in real time; if a provider/model fails mid-stream, the partial assistant text is discarded from conversation history (not persisted) and the next attempt proceeds. A warning is written to stderr.
- No conversation history is lost between model attempts; MCP tool calls are never retried across attempts

### MCP Server Integration
- Validates MCP server connectivity and retrieves tool schemas before the first LLM request. Initialization errors are non‑fatal: they are logged and the agent may continue to the LLM without those tools.
- Supports local (`stdio`) and remote (`http` for streamable HTTP, `websocket`, and `sse`) MCP servers
- Full MCP protocol compliance for tool discovery and execution
- **Schemas and Instructions**: MCP tools provide schemas and optional instructions
- **System Prompt Integration**: At program start, append only tool instructions (if any) to the system prompt once, using the following headers:
  - `## TOOLS' INSTRUCTIONS`
  - `## TOOL {name} INSTRUCTIONS`
  Schemas are NOT appended to the system prompt; they are exposed to the LLM via the request's tool definitions.
- **Server Name Constraints**: MCP server keys must match the regex `[A-Za-z0-9_-]+`. Names containing any other character (spaces, punctuation, etc.) are rejected during initialization so misconfigured servers fail loudly instead of being silently renamed.

#### Per‑Tool Environment Scoping
For `stdio` servers, only the environment variables explicitly configured for that MCP tool are passed to the spawned process. `${VAR}` placeholders are resolved from the current process environment. Variables are not leaked across tools.

### Accounting System
- **JSONL Logging**: All accounting data logged to JSONL file specified in config or command line
- **Entry Types**: Each entry has `"type": "llm"` or `"type": "tool"` and `"status": "ok"` or `"failed"`
- **LLM Accounting**: Track tokens per provider/model (input, completion, cached tokens, etc.) with latency
- **Tool Accounting**: Track MCP server (as named in config), tool command, latency, and character counts (in/out)
- **Privacy**: Never log prompt or completion content. Tool parameters are not logged; only tool name/command and metadata are recorded
- **Callbacks vs Files**: The core library never writes files or stdout/stderr; accounting is emitted via callbacks. The CLI may write JSONL when no custom accounting callback is supplied. If a callback is provided, file writing is skipped.

### Streaming Support
- LLM responses stream to stdout in real time
- If a stream fails mid-response, partial content is discarded from conversation history and a retry may proceed with the next provider/model
- Tools do not stream (their output is for the LLM, not user)
- Vercel AI SDK handles tool call detection automatically during streaming

#### Inactivity Timeout (LLM)
- The `llmTimeout` is an inactivity timeout during streaming: it resets on each received chunk and only aborts if no data arrives for the configured duration.
- In non-streaming mode, `llmTimeout` is a fixed per-call timeout.

### Verbose Logging (--verbose)
- For each request/response, prints a single concise line to stderr with key numbers:
  - `[llm] req: {provider}, {model}, messages N, X chars`
  - `[llm] res: input A, output B, cached C tokens, tools T, latency L ms, size S chars`
  - `[mcp] initializing {server}` / `[mcp] initialized {server}, {N} tools (...), latency Z ms`
  - `[mcp] req: {id} {server}, {tool}` / `[mcp] res: {id} {server}, {tool}, latency X ms, size Y chars`
  - `[fin] finally: llm requests R (tokens: A in, B out, C cached, tool-calls T, output-size S chars, latency-sum L ms), mcp requests M (serverA X, ...)`

### Stream / No-Stream Options
- Global default via `defaults.stream` (true/false)
- Per-provider override: `providers.<name>.custom.stream`
- CLI override: `--stream` / `--no-stream`
- Streaming recommended for interactivity; non-streaming can be used for providers or queries sensitive to streaming edge cases.

#### Tracing
- `--trace-llm`: Logs full request headers/body (Authorization redacted) and pretty JSON responses. For SSE responses, the raw SSE is logged after the stream completes.
- `--trace-mcp`: Logs MCP connect/start, tools/list and prompts/list requests and responses, server stderr lines, callTool requests, and results using a single `[mcp]` sink.

#### OpenRouter Notes
- Uses OpenAI‑compatible Chat Completions endpoints for best tool‑calling support.
- Adds attribution headers: `HTTP-Referer` and `X-OpenRouter-Title` (configurable via env `OPENROUTER_REFERER`, `OPENROUTER_TITLE`), and `User-Agent`.

## Library Architecture

The system is designed as a library that can be embedded in larger applications:

```typescript
import { AIAgent, AIAgentEventCallbacks } from './ai-agent';

// Optional callbacks for complete control over I/O
const callbacks: AIAgentEventCallbacks = {
  onEvent: (event) => {
    if (event.type === 'log') console.error(`[${event.entry.severity}] ${event.entry.message}`);
    if (event.type === 'output') process.stdout.write(event.text);
    if (event.type === 'accounting') { /* custom accounting logic */ }
  }
};

const agent = new AIAgent({
  configPath: '.ai-agent.json',
  llmTimeout: 30000,
  toolTimeout: 10000,
  callbacks, // Optional: if not provided, uses default I/O
  // ... other options
});

const result = await agent.run({
  providers: ['openai'],
  models: ['gpt-4o'],
  tools: ['file-operations'],
  systemPrompt: 'You are a helpful assistant',
  userPrompt: 'List current directory files',
  conversationHistory: [] // Optional: continue existing conversation
});
```

### Library Callbacks
The library accepts optional callbacks for complete control and performs no I/O itself:
- **onEvent**: Unified event handler (log, output, accounting, progress, status, final_report, handoff, etc.)
- **Silent Core**: The core library writes nothing (no files, no stdout/stderr). The CLI wires callbacks to provide default behavior.

**Finality note (important)**:
- `AIAgentEventMeta.isFinal` is **authoritative only for `final_report` events**. For all other event types treat it as informational.
- When a handoff is configured or a router handoff is selected, the upstream payload is emitted as `onEvent(type='handoff')` (not `final_report`). Headends should use `handoff` plus `pendingHandoffCount` to decide what to display.

## Input/Output Specifications

### Standard Streams
- **stdout**: LLM responses only - nothing else ever goes to stdout
- **stderr**: All errors, warnings, and debug information
- **stdin**: Supported via `-` parameter for prompts
- **Validation**: Cannot use stdin (`-`) for both system and user prompts simultaneously

### Exit Codes
- **0**: Successful completion
- **1**: Configuration error
- **2**: LLM communication error
- **3**: Tool execution error
- **4**: Invalid command line arguments

### File Formats
- **Configuration**: JSON with full schema validation and environment variable expansion
- **Conversation Save/Load**: JSON format preserving full message history with metadata
- **Prompt Files**: Plain text files (UTF-8)
- **Accounting**: JSONL format with structured entries for LLM and tool usage

## Technology & Packaging

- **Runtime**: Node.js 20+
- **Package Manager**: npm
- **LLM Communication**: Vercel AI SDK 5 (TypeScript support, streaming, unified providers)
- **MCP Integration**: Official `@modelcontextprotocol/sdk` (1.17.4)
- **CLI Framework**: Commander.js
- **Validation**: Zod for type-safe configuration
- **Language**: TypeScript with full type safety throughout
- **Binary Name**: The CLI is exposed as `ai-agent` via `package.json#bin`, so local installs and `npx ai-agent` work as expected

## Development Principles

1. **Library First**: Core functionality as embeddable library, CLI as thin wrapper
2. **Type Safety**: Full TypeScript coverage with runtime validation
3. **Streaming Native**: Built-in support for streaming responses
4. **Error Resilience**: Graceful handling of provider and tool failures
5. **Protocol Compliance**: Full MCP specification support
6. **Token Transparency**: Detailed token accounting and reporting

## Future Extensibility

The architecture supports:
- Additional LLM providers through Vercel AI SDK
- New MCP transport mechanisms
- Custom tool execution strategies
- Enhanced conversation management
- Integration with larger AI systems
