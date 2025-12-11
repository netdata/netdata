# AI Agent Library API (Internal)

This document describes the programmatic interface of the AI Agent library, separate from the CLI driver. Third parties can embed the agent by importing and calling the library directly.

The library performs no direct I/O (no stdout/stderr/file writes). All output, logs, and accounting are emitted through callbacks and also returned in a structured result.

## Overview

- Library entry: `src/ai-agent.ts` (exported name: `AIAgent`)
- Primary method: `AIAgent.create(sessionConfig)` → returns a session
- Execute: `await session.run()` → returns `AIAgentResult`
- All diagnostics are available via:
  - Callbacks you provide (real-time stream)
  - The returned `AIAgentResult` (post-run snapshot)

## Types (from `src/types.ts`)

- `AIAgentSessionConfig`
  - `config: Configuration` – providers + MCP servers
  - `targets: { provider: string; model: string }[]` – provider/model pairs
  - `tools: string[]` – MCP servers to expose
  - `headendId?: string` – optional identifier propagated to logging when the session is created from a headend (REST/MCP/OpenAI/Anthropic)
  - `systemPrompt: string`
  - `userPrompt: string`
  - `conversationHistory?: ConversationMessage[]`
  - Output contract: `expectedOutput?: { format: 'json' | 'markdown' | 'text'; schema?: Record<string, unknown> }`
  - Execution controls (optional): `temperature`, `topP`, `maxRetries`, `maxTurns`, `llmTimeout`, `toolTimeout`, `stream`
  - UX flags (optional): `traceLLM`, `traceMCP`, `verbose`
  - Tool response cap (optional): `toolResponseMaxBytes` (bytes)
  - `callbacks?: AIAgentCallbacks`
- `AIAgentCallbacks`
  - `onLog(entry: LogEntry)` – structured logs (see below)
  - `onOutput(text: string)` – assistant output stream
  - `onThinking(text: string)` – “thinking”/reasoning stream
  - `onTurnStarted(turn: number)` – fired at the beginning of every LLM turn (1-indexed) before any reasoning/output
  - `onAccounting(entry: AccountingEntry)` – accounting events
- `AIAgentResult`
  - `success: boolean`
  - `error?: string`
  - `conversation: ConversationMessage[]`
  - `logs: LogEntry[]`
  - `accounting: AccountingEntry[]`
  - `finalReport?: { format: 'json'|'markdown'|'text'|'markdown+mermaid'|'slack-block-kit'|'tty'|'pipe'|'sub-agent'; content?: string; content_json?: Record<string, unknown>; metadata?: Record<string, unknown>; ts: number }` – isolated final report returned by the model via `agent_final_report`
- `ConversationMessage` – messages in conversation, including tool calls/results
- `LogEntry` – structured logs
  - `severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN'`
  - `direction: 'request' | 'response'`
  - `type: 'llm' | 'tool'`
  - `remoteIdentifier: string` – `provider:model` for LLM events, `protocol:namespace:tool` for tool events
  - `message: string`
  - `headendId?: string` – populated when invoked via a headend so consumers can correlate activity with the entry point
- `AccountingEntry`
  - `type: 'llm'` (with tokens, cost, latency) or `type: 'tool'` (with characters in/out, latency)

## Library Guarantees

- No direct I/O: The library never writes to stdout/stderr or files.
- Real-time callbacks: If provided, callbacks receive logs/output/accounting as events occur.
- Always returns a result: `run()` resolves with `AIAgentResult` even on errors; it does not throw for normal failures.
- Unconditional FIN summaries: The library emits `FIN` summary logs for LLM and MCP at the end of every run, including when execution fails or ends early. These are delivered through `onLog` and included in `result.logs`.
- Error transparency: Errors are represented both in logs (`ERR`/`WRN` and `agent:EXIT-...` markers) and in the `AIAgentResult.error` string.
- Accounting completeness: All accounting entries (LLM/tool; ok/failed) are recorded and returned.

## Accounting Records: Structure and Emission Semantics

The library emits two accounting record types via `onAccounting(entry)` and also accumulates them in `AIAgentResult.accounting`.

Common base fields (both types):
- `timestamp: number` – Unix epoch (ms) when the operation completed
- `status: 'ok' | 'failed'` – outcome of the operation
- `latency: number` – end-to-end latency in milliseconds

Type `llm` (LLM calls):
- `type: 'llm'`
- `provider: string` – configured provider key
- `model: string` – configured model name
- `actualProvider?: string` – populated for routed providers (e.g., OpenRouter)
- `actualModel?: string` – populated for routed providers when available
- `costUsd?: number` – total reported cost (provider specific; OpenRouter)
- `upstreamInferenceCostUsd?: number` – upstream model cost (OpenRouter)
- `tokens: { inputTokens: number; outputTokens: number; cachedTokens?: number; totalTokens: number }`
- `error?: string` – only when `status: 'failed'` (mapped error message)

Emission timing (llm): After every provider/model attempt in a turn, regardless of success. If the provider doesn’t return usage, token counts default to zeros.

Type `tool` (MCP tool calls and internal tools):
- `type: 'tool'`
- `mcpServer: string` – the MCP server identifier; `'agent'` for internal tools; `'unknown'` when a failure occurs before server resolution
- `command: string` – tool name (namespaced for internal tools: `agent_*`)
- `charactersIn: number` – length of JSON-serialized input parameters
- `charactersOut: number` – length of returned text (after any truncation)
- `error?: string` – only when `status: 'failed'`
- `details?: Record<string, string | number | boolean>` – optional structured metadata (e.g., `projected_tokens`, `limit_tokens`, `remaining_tokens` for context guard failures)

Emission timing (tool): Immediately after each tool execution completes or fails:
- Internal tools `agent__progress_report`, `agent__final_report`: `status: 'ok'`, `mcpServer: 'agent'`, fixed `charactersOut` (15 and 12 respectively), `charactersIn` reflects parameter size.
- External MCP tools: `status: 'ok'` with the resolved `mcpServer` on success; `status: 'failed'`, `mcpServer: 'unknown'`, and `charactersOut: 0` on error.
- Truncation: If the tool response exceeds `toolResponseMaxBytes`, the library prefixes a truncation notice and trims the content to the limit; `charactersOut` reflects the returned (prefixed + truncated) content length. The emitted warning always includes the actual byte size and the configured limit (e.g., `Tool response exceeded max size (actual 16384 B > limit 12288 B)`).
- Context overflow: If projecting a tool result would overflow the configured `contextWindow`, the agent injects `(tool failed: context window budget exceeded)`, records an accounting entry with `error: 'context_budget_exceeded'`, and populates `details` with `projected_tokens`, `limit_tokens`, and `remaining_tokens`.
- Telemetry: every guard activation increments `ai_agent_context_guard_events_total{provider,model,trigger,outcome}` and updates the observable gauge `ai_agent_context_guard_remaining_tokens{provider,model,trigger,outcome}` so integrators can monitor how close sessions are to exhausting their budgets.

Persistence guidance: The CLI demonstrates persisting accounting to JSONL. As a library user, consume `onAccounting` in real time and/or persist `result.accounting` after `run()`.

## Lifecycle (what `run()` does)

1. Validate configuration and prompts.
2. Initialize MCP servers (non-fatal; initialization failures logged as `WRN`).
3. Build the effective system prompt (adds tools’ instructions; schemas are passed as tool defs, not appended to prompt).
4. Execute multi-turn loop with fallback across `targets`:
   - Streams assistant text via `onOutput`.
   - Emits instrumentation via `onLog` and `onAccounting`.
   - Preserves message history (assistant/tool/tool-result order).
5. Completion conditions:
   - Success: model calls internal `agent_final_report` tool → `EXIT-FINAL-ANSWER` logged, `FIN` summaries emitted, returns `success: true`.
   - Context guard: projected token use exceeds `contextWindow` → tool call rejected, `EXIT-TOKEN-LIMIT` logged, forced final turn instructs the model to answer with existing information.
   - Max turns exceeded: `EXIT-MAX-TURNS-NO-RESPONSE` logged, `FIN` summaries emitted, returns failure.
   - Other failures (auth/quota/timeouts/model errors or uncaught exception): logs error, logs an `agent:EXIT-...` reason, emits `FIN` summaries, returns failure.
6. Cleanup: MCP transports closed gracefully.

## Output Format Contract (`expectedOutput.format`)

Purpose: Tells the agent how to finalize and how to structure the final report when the model calls the internal `agent_final_report` tool.

Accepted values:
- `json`: Final report must be JSON; if a `schema` is provided, the content is validated with AJV.
- `markdown`: Final report is Markdown (plain string).
- `text`: Final report is plain text.

How it is enforced:
- Tool definition: The library exposes an internal tool `agent_final_report` to the LLM with an input schema tailored to the requested format:
  - `json`: `{ format: { const: 'json' }, content_json: <schema> }`
  - `markdown`: `{ format: { const: 'markdown' }, content: string }`
  - `text`: `{ format: { const: 'text' }, content: string }`
- Final turn restriction: On the final turn, all providers filter tools down to only `agent_final_report`, and an instructional message is appended forcing a single call to it.
- Synthetic retry: If the assistant returns plain content without tool calls and without a `final_report`, the library logs a warning and retries with the next provider/model within the turn.
- JSON validation: In `json` mode, if `content_json` does not match the schema, the library logs a `WRN` with AJV error details but does not abort; it still completes.

Important nuance – “Can I specify one format and get another?”
- The tool schema advertises a strict `format` via `const`, guiding the model to the correct format and parameters, and most SDKs/providers adhere to this when generating tool calls.
- Runtime behavior trusts the `format` parameter received in the `agent_final_report` call to decide how to emit the final output (JSON/markdown/text). There is no hard runtime rejection if the model sets a different `format` than requested; instead, the primary guardrail is the tool schema plus final-turn tool filtering and instructions.
- Practical effect: a mismatch is unlikely; if it happens in JSON mode, the library still validates and logs a warning. If strict enforcement is required (reject/massage mismatches), add a small guard in `agent_final_report` handling to assert `format === expectedOutput.format` and either normalize or fail.

## Final Report Delivery (Isolated)

- Real-time stream (optional): The library calls `onOutput(...)` as it streams assistant content and again when emitting the final report.
- Isolated final report: Regardless of any prior streamed output, the final report the model returns via `agent_final_report` is available as `result.finalReport` in `AIAgentResult`.
  - Contains status, format, content (string for markdown/text), or `content_json` (for json), optional metadata, and timestamp.
  - This lets embedders persist or present the definitive answer without including any intermediate assistant output.

## Final Report Failure Semantics

There are two distinct notions of “failure”:

- Run-level failure (transport/config/runtime):
  - `AIAgentResult.success === false` and `AIAgentResult.error` is set.
  - Use this for operational failures (auth/quota/network/timeout, etc.).

- Task-level failure (model-declared):
  - `AIAgentResult.success === true` (the session completed), and `result.finalReport?.status === 'failure'`.
  - This indicates the model could not complete the task logically, not that the agent crashed.
  - Error description source:
    - `format: 'markdown' | 'text'`: The explanation is in `result.finalReport.content`.
    - `format: 'json'`: The explanation is in `result.finalReport.content_json` according to your provided schema. The library does not impose a fixed structure; you should include a field like `error` / `message` / `reasons` in your schema to capture it.
  - The library does not derive or inject an error description for task-level failure; it relies on the model to populate the relevant fields per the output contract.

Recommendation for JSON output: include an explicit error structure in the schema, for example:

```json
{
  "type": "object",
  "required": ["status", "error"],
  "properties": {
    "status": {"enum": ["success", "partial", "failure"]},
    "error": {
      "type": "object",
      "required": ["message"],
      "properties": {
        "code": {"type": "string"},
        "message": {"type": "string"},
        "details": {"type": "array", "items": {"type": "string"}}
      }
    }
  },
  "additionalProperties": true
}
```


## Error Handling and Exit Markers

- The library logs a VRB `agent:EXIT-...` line with a reason string for termination causes.
- The CLI converts these into process exit codes; as a library user, rely on:
  - `result.success` and `result.error`,
  - `result.logs` including `agent:EXIT-...` and `FIN` lines,
  - `result.accounting` for a full activity record.

## MCP Tool Response Size Cap (Truncation)

- Configurable via `sessionConfig.toolResponseMaxBytes`.
- When an MCP tool response exceeds the cap:
  - Emits a `WRN` log (response, mcp) with full request formatting + actual size and limit, tagged "(truncated)".
  - Returns truncated content using 50/50 split (first half + last half) with marker at truncation point:
    - Marker format: `[···TRUNCATED {omitted} bytes···]` (appears in the middle of content)
  - Counts occurrences; `FIN` MCP summary includes `capped=<count>`.
- Headend surfaces (REST/MCP/OpenAI/Anthropic) apply the same cap via their session configuration and surface errors through HTTP/SSE/WebSocket semantics. Their incoming request payloads must include `format`, and when `format` is `json`, a `schema` object is required so the library can validate structured content.

## Using the Library (minimal example)

```ts
import { AIAgent as Agent } from '../claude/src/ai-agent.js';
import type { AIAgentSessionConfig, Configuration, LogEntry, AccountingEntry } from '../claude/src/types.js';

const config: Configuration = {
  providers: {
    openai: { apiKey: process.env.OPENAI_API_KEY, type: 'openai' }
  },
  mcpServers: {
    // Define servers as needed
  },
  defaults: {}
};

const logs: LogEntry[] = [];
const accounting: AccountingEntry[] = [];
let capturedOutput = '';

const sessionConfig: AIAgentSessionConfig = {
  config,
  targets: [{ provider: 'openai', model: 'gpt-4o-mini' }],
  tools: [],
  systemPrompt: 'You are helpful.',
  userPrompt: 'Say hello.',
  // Optional controls
  maxRetries: 3,
  maxTurns: 5,
  toolResponseMaxBytes: 12 * 1024,
  // Callbacks: route anywhere you wish
  callbacks: {
    onLog: (e) => { logs.push(e); },
    onOutput: (t) => { capturedOutput += t; },
    onThinking: (_t) => {},
    onAccounting: (a) => { accounting.push(a); }
  }
};

const session = Agent.create(sessionConfig);
const result = await session.run();

// Result is always provided (success or failure)
console.log('success:', result.success);
console.log('error:', result.error);
console.log('output:', capturedOutput);
console.log('logs:', result.logs[result.logs.length - 1]); // includes FIN lines
console.log('accounting entries:', result.accounting.length);
```

## Notes for Embedders

- Frontmatter parsing, file I/O, and command-line option resolution are CLI responsibilities; when embedding, pass plain strings and concrete values in `AIAgentSessionConfig`.
- If you need redacted HTTP header logging, set `traceLLM/traceMCP` and handle `onLog` entries (headers are redacted in logs by the library). Bodies are not redacted by the library.
- For durability, persist `result.logs` and `result.accounting` even when `result.success === false`.

## File Map

- Library core: `claude/src/ai-agent.ts`
- Types: `claude/src/types.ts`
- LLM client/providers: `claude/src/llm-client.ts`, `claude/src/llm-providers/*`
- MCP client: `claude/src/mcp-client.ts`
- CLI (reference implementation): `claude/src/cli.ts`
