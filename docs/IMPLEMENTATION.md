AI Agent Integration Plan: Vercel AI SDK + MCP TypeScript SDK

Overview

- Goal: Combine Vercel AI SDK v5 (in `libs/ai`) with the MCP TypeScript SDK (in `libs/typescript-sdk`) to build a robust, streaming, tool‑calling agent that discovers and executes MCP tools automatically while the LLM orchestrates tool selection.
- Approach: Use MCP SDK for protocol‑compliant tool discovery and invocation (stdio, websocket, streamable HTTP, or SSE). Convert discovered tool schemas into AI SDK tools; delegate tool call planning, sequencing, and parallelization to the AI SDK. Stream assistant tokens to stdout and perform structured accounting.

Why this design

- Let the LLM + AI SDK decide when/how to call tools. This maximizes provider‑native capabilities (e.g., OpenAI parallel function calls), minimizes bespoke orchestration, and reduces drift across providers.
- Use the official MCP SDK for complete protocol coverage (capabilities, transports, instructions, prompts, structured outputs, future extensions), rather than a thin/experimental client.
- Append only MCP instructions to the system prompt once; expose tool schemas via request tool definitions. This matches best practices documented in README.md and avoids confusing the LLM with pasted JSON schemas.

Architecture

1) Configuration and validation

- File: `.ai-agent.json` (resolved: `--config`, then local, then `~/.ai-agent.json`).
- Environment expansion: `${VAR}` expanded for all strings except under `mcpServers.*.(env|headers)` where literal placeholders are preserved for the server process; these are resolved at spawn/transport time instead.
- Schema (selected fields):
  - `providers[providerName]`: `{ apiKey?: string; baseUrl?: string }`
  - `providers[providerName].models[modelName]`: `{ contextWindow?: number; tokenizer?: string; contextWindowBufferTokens?: number; overrides?: ... }`
  - `mcpServers[name]`: `{ type: 'stdio'|'websocket'|'http'|'sse', command?: string|[string,...], args?: string[], url?: string, headers?: Record<string,string>, env?: Record<string,string>, enabled?: boolean }`
  - `defaults`: `{ llmTimeout?: number; toolTimeout?: number; temperature?: number; topP?: number; parallelToolCalls?: boolean; contextWindowBufferTokens?: number }`

2) MCP client layer (libs/typescript-sdk)

- Core types and methods used (from `@modelcontextprotocol/sdk`):
  - `new Client(clientInfo, { capabilities })`
    - `clientInfo`: `{ name: string; version: string; title?: string }`
    - `capabilities`: minimally `{ tools: {} }` to enable tools; prompts/resources are optional.
  - Transports:
    - `new StdioClientTransport({ command, args, env, stderr?: 'pipe'|'inherit' })`
    - `new SSEClientTransport(new URL(url), headers?)`
    - `new StreamableHTTPClientTransport(new URL(url), { requestInit: { headers } })`
    - Custom websocket via helper that wraps `WebSocket`.
  - Lifecycle:
    - `await client.connect(transport)`
    - `await client.close()`
  - Discovery:
    - `await client.listTools()` → `{ tools: Tool[] }`
      - Tool shape (`ToolSchema`): `{ name: string; title?: string; description?: string; inputSchema: { type: 'object', properties?: object, required?: string[] }, outputSchema?: {...}, annotations?: {...} }`
      - Compatibility: legacy servers may return `parameters` instead of `inputSchema`; we handle both.
    - `await client.listPrompts()` (optional) → prompts’ `name/description/arguments` used only as textual instructions.
    - Server‑provided instructions: some servers expose `getInstructions()` on the client instance; we read if present.
  - Invocation:
    - `await client.callTool({ name: string, arguments?: Record<string,unknown> })`
      - Result (`CallToolResultSchema` or compatibility variant):
        - `content: ContentBlock[]` (default: [])
          - Common `ContentBlock`: `{ type: 'text', text: string }` plus others like `image`, etc.
        - `structuredContent?: object` (when server defines `outputSchema`)
        - `isError?: boolean` (tool‑level failure for model visibility; protocol errors are separate)

- Implementation notes:
  - Headers/env resolution: `${VAR}` placeholders resolved from current process env; empty values are omitted.
  - Stdio server scoping: only explicitly configured `env` keys are passed to child process; no leakage across tools.
  - Tracing: a single `[mcp]` sink logs connect, stderr lines, tools/prompts requests and responses, and callTool requests/results.
  - Cleanup: close clients; terminate any spawned processes with SIGTERM then SIGKILL after grace period.

3) Tool exposure to the AI SDK (libs/ai)

- For each MCP tool we construct an AI SDK tool with a matching input schema and an execute function:
  - `inputSchema`: created by `jsonSchema(mcpTool.inputSchema)` from `@ai-sdk/provider-utils`.
  - `execute(parameters, options)`: calls `client.callTool({ name, arguments: parameters })` and returns a string (concatenated text parts from MCP result). The AI SDK will convert the return value into a model tool result.
  - We mark them as dynamic tools implicitly by using `jsonSchema(...)` and providing `execute` at runtime.

- Rationale:
  - No custom planner: the AI SDK already streams tool calls and results, with provider‑native semantics (e.g., OpenAI parallel function calls). We simply supply tool definitions and perform the actual execution when requested.
  - Validation "for free": the AI SDK uses the tool schema to coerce/validate model‑emitted arguments before executing.

4) LLM layer (Vercel AI SDK)

- Providers used:
  - OpenAI: `createOpenAI({ apiKey, baseURL?, headers?, name?, fetch? })`
  - Anthropic: `createAnthropic({ apiKey, baseURL?, headers?, fetch? })`
  - Google: `createGoogleGenerativeAI({ apiKey, baseURL?, headers?, fetch? })`
  - OpenRouter (OpenAI‑compatible): `createOpenAI({ ... headers, name: 'openrouter' })`, and use `.chat(model)` to force Chat Completions API for best tool support.
  - Ollama (OpenAI‑compatible): `createOpenAI({ apiKey: 'ollama', baseURL: 'http://localhost:11434/v1' })`

- Core call: `streamText({ model, tools, system, messages?, temperature, topP, providerOptions })`
  - Inputs:
    - `model`: from selected provider/model.
    - `tools`: map of tool name → { description?, inputSchema, execute }
    - `system`: system prompt with appended MCP instructions exactly once.
    - `messages`: conversation history + current user message.
    - `temperature`, `topP`: taken from config/CLI.
    - `providerOptions`:
      - `openai.parallelToolCalls?: boolean` — controls parallel function calls for OpenAI/OpenRouter providers. Default true in config; toggleable via CLI flags.
  - Outputs:
    - `textStream` (async iterable of stream parts): we stream only `text-delta` to stdout to honor I/O rules.
    - `usage` (Promise): token usage object; providers differ (`inputTokens/promptTokens`, `outputTokens/completionTokens`, `cachedTokens`, etc.).
    - `response` (Promise): final response, including `messages` with tool call/results reflected by the provider.

- Accounting:
  - On each successful step, compute `{ inputTokens, outputTokens, totalTokens, cachedTokens? }` and emit an `AccountingEntry` for `type: 'llm'`.
  - For each tool execution, emit `type: 'tool'` with `latency`, `charactersIn`, `charactersOut`, and `mcpServer`/`command`.
- Context budget guard:
  - Resolve `contextWindow`, `tokenizer`, and `contextWindowBufferTokens` for each target model during session creation. When no window is declared, the session falls back to an internal ceiling of 131072 tokens. Tokenizer identifiers support `tiktoken:*`, `anthropic`/`claude` prefixes, and `gemini`/`google:gemini` prefixes via local libraries, with an approximate heuristic as the last resort.
  - Before executing a tool, estimate the tokens contributed by the prospective tool message. If the projection exceeds `contextWindow - maxOutputTokens - buffer`, synthetic failure `(tool failed: context window budget exceeded)` is returned, the session logs `agent:context`, and `EXIT-TOKEN-LIMIT` is enforced.
  - Every guard activation emits telemetry: the counter `ai_agent_context_guard_events_total{provider,model,trigger,outcome}` and the observable gauge `ai_agent_context_guard_remaining_tokens{provider,model,trigger,outcome}` capture activation frequency and remaining token headroom.
  - Accounting entries for such failures include `details.projected_tokens`, `details.limit_tokens`, and `details.remaining_tokens` to support downstream audits.

Prompt construction and MCP instructions

- We append tool instructions once at the start using:
  - Header `## TOOLS' INSTRUCTIONS`
  - Per server: `## TOOL {serverName} INSTRUCTIONS` when available
  - Per tool: `## TOOL {toolName} INSTRUCTIONS` when available
- We do not paste JSON schemas into the prompt; schemas are provided through the `tools` option for the model request.

Provider fallback and streaming semantics

- Fallback order: model‑first, then providers per model, each receiving the exact same request (system, messages, tools, params) with no mutation.
- Streaming failure: if a provider/model fails mid‑stream, we discard partial assistant output from history; emit a warning and try the next provider/model. Tool calls are never retried across attempts.

Error handling and timeouts

- MCP tool timeouts: per‑call with `toolTimeout` (default 10s). Timeouts throw `Tool execution timed out`, counted as a failed tool result recorded in history/accounting; never retried.
- LLM timeout: provider/network level via fetch and AI SDK retry settings; we do not wrap with an extra timer to avoid interrupting internal stream machinery.
- Tool failure (`isError: true` from MCP): treated as a failed tool result but still added to message history for LLM self‑correction.

Security, tracing, and headers

- Redact `Authorization` headers in LLM traces; include `Accept: application/json` by default.
- OpenRouter: add attribution headers `HTTP-Referer`, `X-OpenRouter-Title`, and `User-Agent` to avoid non‑JSON responses and for proper attribution.
- MCP stdio servers: strict env scoping; only whitelisted variables (after `${VAR}` expansion) are passed to the child process.

API Inventory and Type Verification

MCP SDK (client)

- `new Client(info, { capabilities })`
  - `info`: `{ name: string; version: string; title?: string }`
  - `capabilities`: at minimum `{ tools: {} }` to enable `tools/*` methods.

- `connect(transport)` where transport is one of:
  - `new StdioClientTransport({ command: string, args?: string[], env?: Record<string,string>, stderr?: 'pipe'|'inherit' })`
  - `new SSEClientTransport(url: URL, headers?: Record<string,string>)`
  - `new StreamableHTTPClientTransport(url: URL, { requestInit?: { headers?: Record<string,string> } })`

- `listTools()` → `{ tools: Array<{ name: string; title?: string; description?: string; inputSchema: JSONSchemaObject; outputSchema?: JSONSchemaObject; annotations?: object }> }`
  - We accept both `inputSchema` and legacy `parameters`.

- `listPrompts()` (optional) → `{ prompts: Array<{ name: string; description?: string; arguments?: Array<{ name: string; description?: string; required?: boolean }> }> }`
  - We use only the names/descriptions as additional instructions.

- `callTool({ name, arguments })` → `CallToolResult`:
  - `content: ContentBlock[]` (default: []) — we concatenate all `text` blocks; non‑text types are tagged (e.g., `[Image]`).
  - `structuredContent?: object` — ignored unless/until we want structured tool outputs.
  - `isError?: boolean` — mapped to `success=false` and added to history.

- `close()` — ensure clean shutdown.

AI SDK (provider + stream)

- Providers
  - `createOpenAI({ apiKey, baseURL?, headers?, name?, fetch? })`
    - Returned function is callable with a model id; `.chat(model)` forces Chat Completions when needed (OpenRouter).
    - ProviderOptions for tool calls: `{ openai: { parallelToolCalls?: boolean } }`.
  - `createAnthropic({ apiKey, baseURL?, headers?, fetch? })`
  - `createGoogleGenerativeAI({ apiKey, baseURL?, headers?, fetch? })`

- Tools
  - `jsonSchema(schema)` converts JSON Schema (from MCP) into AI SDK `FlexibleSchema` for tool input.
  - Tool shape provided to `streamText`: `{ description?: string, inputSchema, execute(parameters, options) }`.

- `streamText({ model, tools, system, messages, temperature, topP, providerOptions })`
  - Returns `{ textStream, usage, response }`.
  - We stream only assistant `text-delta` to stdout; everything else stays internal.
  - After completion, we read `usage` and `response.messages` for accounting and history persistence.

Alternative designs considered

- Use AI SDK’s experimental MCP client instead of the MCP SDK
  - Pros: fewer dependencies, potential tighter integration
  - Cons: limited transport coverage and protocol depth; less mature. We chose the official MCP SDK for full spec coverage and stability.

- Orchestrate tool calls ourselves outside the AI SDK
  - Pros: explicit control over sequencing/parallelism
  - Cons: loses provider‑native scheduling (e.g., OpenAI parallel tool calls), increases complexity and maintenance. Letting the AI SDK orchestrate maximizes correctness and performance.

- Paste tool schemas into the prompt instead of using `tools`
  - Cons: increases prompt length, decreases clarity, and bypasses automatic argument validation. Provider tool interfaces are designed to receive schemas out‑of‑band.

- Always sequential tool calls
  - Cons: slower for providers/models that natively support parallel calls. We expose `parallelToolCalls` for OpenAI‑compatible providers and default to true.

## Headend Surfaces

The CLI’s headend mode is implemented as a set of focused classes managed by `HeadendManager`:

- **`RestHeadend`** – lightweight REST API on `/v1/:agent` with optional `format` query parameter and `/health` probe. Each request acquires a `ConcurrencyLimiter` slot before starting an agent session.
- **`McpHeadend`** – wraps the MCP TypeScript SDK server and supports `stdio`, streamable HTTP (`POST /mcp`), SSE (`GET /mcp/sse` + `POST /mcp/sse/message`), and WebSocket transports. Every tool invocation must include a `format`; when `format=json`, callers must also send a `schema` object so the agent can validate structured output.
- **`OpenAICompletionsHeadend`** – surfaces agents as OpenAI Chat Completions models (`/v1/models`, `/v1/chat/completions`) with optional SSE streaming.
- **`AnthropicCompletionsHeadend`** – provides Anthropic Messages compatibility (`/v1/models`, `/v1/messages`) and emits Anthropic-style SSE events.
- **`SlackHeadend`** – wraps the existing Slack bot logic behind the headend manager. Socket Mode traffic shares the same concurrency limiter, and slash commands mount on the first REST headend (fallback listener on `api.port` when no REST headend is configured).

Each headend is repeatable; the CLI can spin up multiple instances (e.g., multiple MCP ports plus REST). Concurrency guards are per-headend (`--api-concurrency`, etc.) and default to ten in-flight sessions per headend instance.

Edge cases and compatibility

- Legacy servers returning `parameters` instead of `inputSchema` are supported.
- Zero‑argument tools are supported (empty input schema object).
- Tool timeouts return a failed result; never retried.
- Client disconnects before a queued request acquires a slot are detected; the limiter removes them from the FIFO queue so abandoned work is never executed.
- Non-text content from tools is preserved in principle; we currently surface non-text as a tagged placeholder in the tool result text. If desired later, we can attach resource links or structured content.
- Streaming failure mid-response discards partial assistant text from the conversation history but continues fallback cleanly.

Implementation checklist (end‑to‑end)

- Load and validate config; expand env; normalize MCP servers (`local` → `stdio`, `remote` → `http|sse` by URL).
- Initialize MCP servers:
  - Build and connect proper transport per server type
  - Fetch `tools` and optional `prompts`; store per‑server instructions
  - Build combined instructions string for system prompt
  - Build tool→server mapping
- Build AI SDK tools: for each MCP tool, `jsonSchema(inputSchema)` + `execute` that delegates to MCP `callTool`.
- Enhance system prompt with instructions once.
- Execute `streamText` with model, tools, system, messages, temperature/topP, and OpenAI `parallelToolCalls` option when applicable.
- Stream `text-delta` to stdout; on finish, record usage and append response messages to conversation with metadata.
- Emit accounting entries for LLM and tools; write JSONL when configured.
- Cleanup MCP clients and spawned processes.

Notes for future improvements

- Structured tool outputs: When MCP tools define `outputSchema`, we can map it to AI SDK `outputSchema` and adapt `toModelOutput` to pass the structured JSON back to the model to improve downstream reasoning and reduce hallucinations.
- Resource links: Return `resource_link` content blocks as stream data for UIs and keep tool result text concise.
- Prompt completions: Integrate MCP prompt/resource completions into interactive CLIs.
- Telemetry: Wire AI SDK telemetry spans with MCP timing for correlated traces.

References (code entry points)

- CLI: `src/cli.ts` — argument parsing, prompt resolution, accounting sink, and agent execution.
- Agent core: `src/ai-agent.ts` — provider setup, `streamText` orchestration, usage accounting, prompt enhancement, fallback.
- MCP client: `src/mcp-client.ts` — transport selection, tool discovery, prompt discovery, tool execution, headers/env resolution, cleanup.

---

## OpenRouter implementation

https://www.npmjs.com/package/@openrouter/ai-sdk-provider/v/beta

This is compatible with AI SDK 5. The page examples. Read it.

---

## Ollama implementation

Provider

- Uses the community AI SDK 5 provider `ollama-ai-provider-v2` with the native REST API under `/api` (not the OpenAI compatibility layer).
- Base URL normalization: if a config `baseUrl` ends with `/v1`, it is rewritten to `/api`; if it lacks `/api`, it is appended.
- Tracing: provider is initialized with `fetch: tracedFetch` so `--trace-llm` logs HTTP requests and SSE headers.

Provider options (per README)

- Per-provider `custom.providerOptions.ollama` maps to the provider’s schema:
  - `think`: boolean (for thinking-capable models)
  - `options`: `{ num_ctx, num_predict, temperature, top_p, top_k, min_p, repeat_last_n, repeat_penalty, stop, seed }`

Streaming and timeouts

- Streaming path uses an inactivity timeout: an `AbortController` is armed for `llmTimeout` ms and reset on every streamed chunk. If the stream stalls for `llmTimeout`, the controller aborts.
- Non-streaming path uses a fixed per-call timeout with `AbortSignal.timeout(llmTimeout)`.
- The per-provider streaming switch is supported via `providers.ollama.custom.stream` (see README: Stream / No-Stream Options).

Tool-calling

- Full agentic loop: assistant tool_calls and tool messages are preserved and returned to Ollama in the next turn. No synthetic user messages are used for tool results.
- On the final allowed turn, tools are disabled for the request and a single user message instructs the model to conclude using existing tool outputs.
- Tool-choice overrides: `.ai-agent.json` supports `providers.<name>.toolChoice` and per-model `providers.<name>.models.<model>.toolChoice` (`"auto" | "required"`). The resolved value is injected into both the AI SDK request and provider-specific payloads (e.g., OpenRouter’s OpenAI shim) so routed vendors that reject `tool_choice="required"` stay compatible.
