# AI Agent Guide for Autonomous Builders

> **Audience & Use**: This document is written exclusively for AI assistants and automation scripts. No human review is assumed. Consume it programmatically to generate, compose, and operate ai-agents. Every fact is validated against the current TypeScript implementation (commit-time truth source).

## 1. Operating Model
- **Control loop**: `AIAgentSession.executeAgentLoop` (src/ai-agent.ts) runs turns `1..maxToolTurns` (default 10). Each turn: (a) open opTree turn node, (b) run provider/model pairs via `LLMClient.executeTurn`, (c) stream assistant output, (d) execute tool calls through `ToolsOrchestrator`, (e) append tool results to conversation, (f) repeat until assistant emits final answer or `agent__final_report` call.
- **Tool failures**: `ToolsOrchestrator.executeWithManagement` never retries a tool. Failures are logged (`severity='ERR'`, `type='tool'`) and inserted into conversation as tool-result messages so the LLM can course-correct. Output > `toolResponseMaxBytes` (default 12 288 bytes) is truncated with `[TRUNCATED]` prefix before being returned and emits a warning that includes both the actual byte size and the limit (for example, `Tool response exceeded max size (actual 16384 B > limit 12288 B)`).
- **LLM failures & retry logic**: Per turn, the session cycles through configured provider/model pairs (`targets`) and retries up to `maxRetries` (default 3). `LLMClient` normalizes provider errors into statuses (`rate_limit`, `network_error`, `auth_error`, etc.). Retryable errors advance to the next provider; fatal ones log `EXIT-*` and end the loop.
- **Stop/cancel handling**: A `stopRef` (`{ stopping: boolean }`) can be threaded into `AIAgentSessionConfig`. The runner now polls this flag before every retry attempt, during rate-limit backoff sleeps, and once more before emitting a failure. When every provider in a cycle reports `rate_limit`, the session still enters the recommended backoff window even if no retries remain whenever a `stopRef` is present, ensuring late stop requests can resolve gracefully instead of bubbling a `rate_limit` error.
- **Reasoning management**: Frontmatter/CLI `reasoning` (`minimal|low|medium|high`) and `reasoningTokens` overrides feed `ProviderReasoningMapping` before each turn. Anthropic-style thinking streams arrive via `onThinking` and surface as `THK` log entries plus grey stderr text. Every LLM turn also triggers `onTurnStarted(turnIndex)` so headends can render numbered turn headers even when no reasoning/progress chunks arrive.
- **Context-window guard**: `AIAgentSession` tracks `currentCtxTokens`, `pendingCtxTokens`, `newCtxTokens`, and `schemaCtxTokens`. If projected totals exceed provider limits it enters “forced final turn” mode: tool calls disabled, final user nudge injected, and `logExit('EXIT-TOKEN-LIMIT', ...)` recorded.
- **System prompt composition**: `agent-loader` flattens `.ai` content (resolves `include:`), strips frontmatter, then `applyFormat` injects `${FORMAT}` description. During runtime, `AIAgentSession` appends tool instruction blocks exactly once (see docs/SPECS.md §System Prompt Integration). No schemas are pasted into the prompt; they are passed as JSON tool defs.
- **Injected user messages**:
  - If `maxToolTurns` is reached, the session appends the hardcoded “You are not allowed to run any more tools…” user message and disables tools for the concluding turn.
  - For forced-final context situations, a user message states “Maximum number of turns/steps reached…” before invoking `agent__final_report`.
- **Internal tools**: `agent__final_report` is always available; the LLM must call it exactly once to conclude. Sub-agents register as `agent__<toolName>`. REST imports register as `rest__<toolName>`.
- **Logging & tracing**:
  - `LogEntry.severity` set includes `VRB` (verbose flow), `WRN`, `ERR`, `TRC` (trace dumps), `THK` (thinking stream headers), `FIN` (summary). CLI renders them via `log-sink-tty` with ANSI colors; headends forward logs to their sinks.
  - `--trace-llm`, `--trace-mcp`, `--trace-sdk`, `--verbose` toggle streaming of payloads; `traceSlack` controls Slack bot verbosity.
  - Thinking chunks go to stderr with grey color; when `THK` entries close, CLI inserts newline to keep logs aligned.
  - **Verbose line anatomy** (`--verbose`): each `VRB` message looks like `[VRB] → [1.0] llm openai:gpt-4o: messages 3, 1247 bytes`.
    - Arrow: `→` request, `←` response.
    - `[turn.subturn]`: `1.0` = turn 1 LLM request; `1.1` first tool call, `1.2` second tool, `2.0` next LLM turn, etc. This exposes depth when sub-agents call their own tools (turn counters restart inside child sessions and their logs are prefixed with `agent:child` identifiers).
    - `type` (`llm`/`tool`) plus `remoteIdentifier` (`openai:gpt-4o`, `mcp:filesystem:read_file`, `agent:web.research`) shows which provider/tool executed.
    - Message tail encodes payload summary (message count, token stats, latency). Use these cues to trace hierarchical behavior without additional tooling.
- **Accounting**: Every LLM/tool call emits `AccountingEntry` (type `llm` or `tool`) with tokens, latency, and cost. CLI flushes them to `${HOME}/.ai-agent/accounting.jsonl` unless `--billing-file` overrides it.
- **Exit codes & summaries**: `AIAgentSession.logExit` emits standardized tags (`EXIT-FINAL-ANSWER`, `EXIT-TOOL-FAILURE`, `EXIT-TOKEN-LIMIT`, `EXIT-UNCAUGHT-EXCEPTION`, etc.) with `fatal=true`. `FIN` summaries follow even on failure, and telemetry spans capture `ai.agent.success` plus turn counts.

**Checklist**
- [ ] Declare at least one provider/model pair (`targets`).
- [ ] Confirm `maxToolTurns`/`maxRetries` satisfy task budget.
- [ ] Provide final-report instructions so the LLM knows when to call `agent__final_report`.
- [ ] Decide whether to expose tool instructions (default: appended automatically once).
- [ ] Enable tracing/verbose flags when debugging provider/tool failures.

## 2. `.ai` Files (Frontmatter + Prompt Body)
- **Structure**: optional shebang (`#!/usr/bin/env ai-agent`), YAML frontmatter between `---` fences, blank line, then prompt body. Unknown frontmatter keys raise errors (`parseFrontmatter` enforces allowed list from `OPTIONS_REGISTRY`).
- **Required keys**: `description` (string) and/or `usage` are optional but highly recommended. Prompt body can reference `${FORMAT}` and `${VAR}` placeholders.
- **Linking tools/agents**:
  - `models`: list of `provider/model` pairs applied before CLI overrides.
  - `tools`: MCP server names, REST tools (`rest__*`), or literal tool IDs.
  - `agents`: relative paths to child `.ai` files; `agent-loader` preloads them into the registry.
  - `toolName`: stable identifier when exposing the agent as a callable tool (`agent__toolName`).
- **Input/Output contracts**:
  - `input.format`: `text` (default) or `json`; provide `schema` or `schemaRef` (YAML/JSON) when set to `json`.
  - `output.format`: `markdown` (default), `text`, or `json`. JSON outputs should include `schema` or `schemaRef` so headends can attach `response_format`.
- **Operational knobs** (frontmatter keys → defaults):

| Key | Type | Default | Notes |
| --- | --- | --- | --- |
| `maxToolTurns` | int | `10` | Total LLM turns with tool access. |
| `maxToolCallsPerTurn` | int | `10` | Caps tool invocations per turn. |
| `maxRetries` | int | `3` | Provider/model attempts per turn. |
| `llmTimeout` | ms | `600000` | Reset per streamed chunk. |
| `toolTimeout` | ms | `300000` | Per tool call timeout. |
| `temperature` | number | `0.7` | Clamped `0-2`. |
| `topP` | number | `1.0` | `0-1`. |
| `maxOutputTokens` | int | `4096` | `undefined` disables explicit cap. |
| `repeatPenalty` | number | `1.1` | >=0. |
| `toolResponseMaxBytes` | bytes | `12288` | Over-limit responses are truncated. |
| `stream` | boolean | `false` | CLI default is streaming off; headends opt-in. |
| `reasoning` | enum | provider default | `minimal/low/medium/high`. |
| `reasoningTokens` | number or string | undefined | `0`/`disabled` disables provider reasoning cache. |
| `caching` | `full|none` | `full` | Anthropic cache control. |

- **Example**:
```yaml
#!/usr/bin/env ai-agent
---
description: Trend analyst
toolName: trend.analyzer
models:
  - openai/gpt-4o-mini
agents:
  - ./sub/researcher.ai
input:
  format: json
  schema:
    type: object
    required: [topic]
    properties:
      topic: { type: string }
output:
  format: markdown
maxToolTurns: 8
---
You synthesize research data into concise briefs. Answer in ${FORMAT}.
```
- **Tool references**: The `tools` array in frontmatter accepts **configuration keys**:
- MCP servers by name (e.g., `filesystem` when `.ai-agent.json` defines `"mcpServers": { "filesystem": { ... } }`). Listing the server exposes every tool the server returns, subject to `toolsAllowed/toolsDenied` filters.
- Server naming rules: `mcpServers.<name>` keys must match `[A-Za-z0-9_-]+`. The agent no longer auto-sanitizes names; invalid characters trigger a configuration error during MCP initialization so telemetry/logs always match your config exactly.
  - Individually named REST imports (prefixed `rest__` internally). Declare `rest__catalog_listItems` directly.
  - Literal tool IDs emitted by other entry points (rare; typically only `agent__*` or `rest__*`).
  - Sub-agents must live under `agents`, not `tools`; their callable handles (`agent__childName`) are inserted automatically.
- **Schema references**: `schemaRef` points to a JSON or YAML file relative to the prompt file (fallback: `process.cwd()`). Example: `schemaRef: ./schemas/report.json`. Files with other extensions are parsed as JSON first, YAML second; errors bubble as `invalid_parameters`.

**Checklist**
- [ ] Validate frontmatter keys against allowed list; remove unknown attributes.
- [ ] Define `toolName` when exposing the agent as a callable tool.
- [ ] Provide JSON schemas whenever `format: json` is declared.
- [ ] Reference `${FORMAT}` in the prompt body so headends can inject target instructions.

## 3. Prompt Variable Expansion & Includes
- **Include directives**: `${include:relative/path}` or `{{include:relative/path}}` inline files **before** frontmatter parsing (implementation: `src/include-resolver.ts`).
  - Base directory = the current file’s directory; nested includes re-root relative to the file that invoked them.
  - Maximum depth is 8. Exceeding the limit throws to prevent recursive loops.
  - `.env` and other hidden secrets cannot be included; attempting to do so aborts load.
  - Use includes for reusable instructions (e.g., safety preambles) so every agent shares the same canonical text.
- **Expansion phases**:
  1. Resolve every include until a single prompt body exists.
  2. Strip/parse frontmatter.
  3. Run `applyFormat` to inject `${FORMAT}` / `{{FORMAT}}` placeholders.
  4. Run `expandVars` with the active placeholder map. `${VAR}` and `{{VAR}}` are equivalent; casing must match.
- **`.ai` prompt placeholders** (from `buildPromptVars` + per-session extras):

| Placeholder | Description | Source |
| --- | --- | --- |
| `${DATETIME}` | Current local timestamp in RFC 3339 format. | `buildPromptVars()`
| `${DAY}` | Local weekday name (e.g., “Friday”). | `buildPromptVars()`
| `${TIMEZONE}` | Olson timezone ID (`America/Los_Angeles` fallback `TZ`/`UTC`). | `buildPromptVars()`
| `${MAX_TURNS}` | Effective `maxToolTurns` after overrides. | Injected in `AIAgentSession`
| `${FORMAT}` / `{{FORMAT}}` | Target output instructions (“Markdown with tables”, “JSON matching schema X”). | `applyFormat()`

- **CLI inline prompt placeholders** (`ai-agent "sys" "user"` mode only): same as above **plus** the host metadata shown below (see `buildPromptVariables` in `src/cli.ts`). These extras are *not* injected when running `.ai` files via headends.

| Placeholder | Description |
| --- | --- |
| `${OS}` | `/etc/os-release PRETTY_NAME` + kernel fallback `os.type() os.release()`. |
| `${ARCH}` | `process.arch`. |
| `${KERNEL}` | `os.type()` + `os.release()`. |
| `${CD}` | `process.cwd()` when the CLI command started. |
| `${HOSTNAME}` | `os.hostname()`. |
| `${USER}` | `os.userInfo().username` fallback `$USER`/`$USERNAME`. |

- **Why `${FORMAT}` matters**: Headends/CLI compute a final format string (Markdown vs JSON vs Slack Block Kit) and *replace* `${FORMAT}` with explicit instructions (“Respond in JSON that validates against …”). Omit the placeholder and downstream clients may answer in the wrong format.
- **Worked example**:
  ```markdown
  ## Operating Context
  - Today is ${DAY} (${DATETIME} in ${TIMEZONE}).
  - You have up to ${MAX_TURNS} tool turns; prioritize the highest-signal tools.
  - Deliver the final response in ${FORMAT}.
  ```
- **Best practices**:
  - Always mention `${MAX_TURNS}` so sub-agents understand remaining budget.
  - Keep include files small and composable; avoid bringing in secrets.
  - Prefer `${FORMAT}` over hard-coding “Answer in Markdown” so headends remain authoritative.

**Checklist**
- [ ] Resolve all include directives (no `${include:…}` remains in flattened prompt).
- [ ] Use only the placeholders listed above; casing matters and undefined names pass through literally.
- [ ] Add `${FORMAT}` in every prompt body unless the system intentionally lets the LLM choose the format.
- [ ] Keep include depth ≤ 8 and never target `.env` files.

## 4. Configuration Search vs Prompt Paths
- `discoverLayers` loads `.ai-agent.json/.ai-agent.env` in priority order (highest first): `--config` explicit path → current working directory → prompt file directory (if different) → binary directory → `~/.ai-agent/` → `/etc/ai-agent/`.
- Each layer contributes JSON + env variables; higher layers override lower ones field-by-field.
- Prompt-relative configs: when running `ai-agent ./agents/researcher.ai`, the loader automatically checks `./agents/.ai-agent.json` before falling back to binary/home/system defaults.
- `.ai-agent.env` sits next to each JSON file; values found there override process env **only** for that layer during placeholder expansion.
- Placeholder expansion walks every string via `expandPlaceholders`: `${NAME}` is replaced with the first value found in the layer’s `.ai-agent.env`, otherwise the value from `process.env`. If neither exists, `resolveProvider/resolveMCPServer` throws `Unresolved variable ${NAME} for <scope> '<id>' at <origin>` so missing secrets fail fast.
- Layers never merge `.ai-agent.env` contents. A provider defined in the home directory can only see `~/.ai-agent/ai-agent.env` (or OS env); it will not pick up secrets from `/etc/ai-agent/ai-agent.env`.
- CLI `--config` short-circuits discovery (only that file + its `.env` are considered before falling through to lower-priority shared layers).

**Checklist**
- [ ] Place project-specific configs in the prompt directory when agents live outside cwd.
- [ ] Use `--config` for ephemeral experiments so global defaults stay untouched.
- [ ] Keep `.ai-agent.env` git-ignored at every layer.

## 5. Building Agent Hierarchies
- **Registration**: In frontmatter, list child prompt paths under `agents`. During load, `agent-registry` preloads each child, validates frontmatter, and assigns a canonical `agent__<toolName>` entry.
- **toolName discipline**: Every sub-agent must set `toolName`; it becomes the callable identifier exposed to parent LLMs. Missing `toolName` defaults to sanitized filename but explicit names are safer.
- **Input validation**: Parents pass arguments that must satisfy the child’s `input` schema. Invalid payloads bubble up as `tool` errors.
- **Isolation guarantees**: Each sub-agent run gets a new `AIAgentSession` with its own MCP clients, accounting, and context counters. Environment overlays come from the same merged config but no mutable state is shared.
- **Recursion controls**: Parents inherit `maxToolTurns`, `maxRetries`, etc., to children unless the child frontmatter overrides them. The queue manager enforces process-wide limits across both MCP tools and sub-agents based on the queue each tool binds to.
- **Workflow**:
  1. Parent .ai file lists `agents` (paths) and adds child `toolName` instructions in the prompt.
  2. Run headend/CLI with `--agent parent.ai` so the registry knows about the tree.
  3. At runtime, the parent LLM calls `agent__child` with JSON/text payloads per child’s `input` contract.
- **Prompting pattern**: mention each child explicitly so the LLM knows when to call it, e.g.:
```markdown
## Available Specialists
- `agent__research.company` → structured company profile fetcher. Input JSON: `{ "company": "ACME" }`.
- `agent__web.sweeps` → investigative web searcher. Input JSON: `{ "query": "..." }`.

Use the research agent whenever you need canonical company data; use the sweeps agent to gather current intel before drafting conclusions.
```
- **Error propagation**: invalid child inputs raise `invalid_parameters: ...` errors, surfaced as tool results in the parent conversation. Timeouts and other failures likewise arrive as tool responses; parent prompts should instruct the LLM to recover (“if a tool fails, inspect the error text and retry with corrected inputs”).
- **Test-from-leaves workflow**:
  1. Run each leaf agent standalone with `ai-agent --agent ./agents/leaf.ai --verbose "System" "Task"` to verify its tools and prompts behave.
  2. Inspect verbose lines: `→ [1.0] llm ...` shows the leaf’s own turns; any `agent__` tool calls inside the leaf would indicate unintended recursion.
  3. Once leaves are stable, move one level up (e.g., `analyst.ai`) and repeat. This prevents chasing compounded failures in deep trees.
  4. When debugging, focus on mismatched turn indexes: if the parent stalls at `→ [2.0]` with no matching `←`, the issue is in the LLM/provider; if a child tool never logs `← [1.1]`, the failing MCP/REST call is at that layer.

**Checklist**
- [ ] Every sub-agent declares `toolName`, `input`, and `output` blocks.
- [ ] Parent prompt instructs when/how to call each child tool.
- [ ] Verify child frontmatter doesn’t reference tools the parent failed to register.
- [ ] Ensure `agents` paths are relative to the parent file or absolute.

## 6. `.ai-agent.json` Schema Essentials
- **Providers (`providers.<name>`)**:
  - Required `type`: one of `openai`, `anthropic`, `google`, `openrouter`, `ollama`, `test-llm`.
  - Optional `apiKey`, `baseUrl`, `headers`, `custom` provider-specific payloads, `toolsAllowed/Denied`, `reasoning` mapping (single value or tuple per reasoning level), `contextWindow`, `tokenizer`, `contextWindowBufferTokens`.
  - `models.<model>` supports `overrides.temperature/topP`, reasoning entries, `contextWindow`, `tokenizer`, `contextWindowBufferTokens`.
- **MCP servers (`mcpServers.<name>`)**:
  - Fields: `type` (`stdio|websocket|http|sse`), `command` + `args` (for stdio), `url`, `headers`, `env`, `enabled`, `toolSchemas`, `toolsAllowed/Denied`.
  - Legacy aliases `type=local/remote` auto-normalize.
- **REST tools (`restTools`)**: Each entry names a tool (prefixed `rest__` at runtime) with HTTP metadata imported directly or via `openapiSpecs`.
- **Defaults**: Under `defaults`, you may set the same knobs as frontmatter/CLI (timeouts, sampling, stream, tool limits, contextWindowBufferTokens, default output format, per-surface format preferences).
- **Queues**: Declare process-wide concurrency in `queues.{name}.concurrent`. Every MCP/REST tool binds to a queue via `queue: name` (falling back to `default`). Use small queues (e.g., `fetcher`) to throttle heavy MCP servers like Playwright; internal tools never consume slots so deadlocks are impossible.
- **Telemetry**: `telemetry` block controls OTLP exporters, Prometheus endpoint, trace sampler (`always_on|always_off|parent|ratio`), log formats (`journald|logfmt|json|none`), and extra sinks (`otlp`).
- **Slack/API**: `slack` toggles bot behavior (see Section 10). `api` controls built-in REST headend defaults (`enabled`, `port`, `bearerKeys`).
- **Pricing**: Optional cost tables keyed by provider/model with per-1k or per-1M token rates for prompt/completion/cache hits.

### MCP server example
```json
"mcpServers": {
  "codebase": {
    "type": "stdio",
    "command": "node",
    "args": ["./mcp/file-server.js"],
    "env": {
      "ROOT": "${REPO_ROOT}",
      "TOKEN": "${GIT_TOKEN}"
    },
    "headers": { "User-Agent": "ai-agent/1.0" },
    "toolsAllowed": ["list_files", "read_file"],
    "toolsDenied": ["write_file"]
  }
}
```
- `type` controls the transport; stdio servers require `command` (+ optional `args`).
- `env` only passes the listed keys to the child process; secrets should live in `.ai-agent.env`.
- `toolsAllowed`/`toolsDenied` accept arrays of case-insensitive names. `"*"`/`"any"` = wildcard. Filtering happens server-side before tools reach the LLM.
- `toolSchemas` exists for future manual overrides and is currently ignored—leave it unset.
- **Stdio watchdogs**: Every stdio MCP PID is tracked (Linux, macOS, Windows, FreeBSD). On tool timeouts we run a health probe (`ping` → `listTools`) and only recycle the process when the probe fails; if it succeeds we leave the server running. Remote transports follow the same probe/restart flow minus the PID kill step. User-driven aborts still just close the client.
- `shared` (default `true`) tells ai-agent to keep a single MCP client alive for all transports (stdio, websocket, http, sse). Set `shared: false` when you need a per-session instance (e.g., highly stateful servers or sensitive configs).
- **Shared restart policy**: When a shared server fails to initialize, times out, or fails its health probe, ai-agent immediately begins a restart loop with exponential backoff (0, 1, 2, 5, 10, 30, 60 s; the loop stays at 60 s thereafter). Every restart decision and failure is logged with `ERR` severity, and the loop never gives up—fix the server or disable it in config before restarting ai-agent if you need it to stop. Private (`shared: false`) servers keep the legacy single-shot restart behavior.
- **Exit detection**: If the underlying MCP transport closes (stdio child exits, websocket disconnects, HTTP/SSE link drops), ai-agent now treats that as a fatal condition and immediately schedules the same restart loop without waiting for the next tool timeout. The next caller either waits for the restart or receives `mcp_restart_in_progress` instead of timing out on a dead connection.
- `healthProbe` (default `"ping"`) controls the probe used after a timeout; set to `"listTools"` for servers that don’t implement `ping`.

Timeouts are not crashes. Many MCP servers run long-lived asynchronous work (SQL engines, crawlers, fetchers) and may legitimately exceed a single request’s SLA while still servicing other requests. ai-agent therefore refuses to restart a shared server unless the probe fails. For servers where the protocol-level `ping` is insufficient (it’s a lightweight heartbeat handled by the SDK), set `healthProbe: 'listTools'` or another work-specific probe so we only recycle the process when it truly stops responding.

Shared MCP servers are initialized once per ai-agent process. For stdio transports, the first session spawns the child process and subsequent sessions reuse the same pipes. Remote transports reuse a single persistent client connection. When a timeout occurs we run the probe (ping → listTools) and only recycle the PID/connection when it fails; if the restart itself fails we keep retrying forever using the backoff sequence described above. If you flip `shared` to `false`, the legacy per-session lifecycle (spawn on demand, stop at session end) remains in place.

To keep cancellation deterministic, `MCPProvider` pads each shared client’s `requestTimeoutMs` (≥150 % of the configured tool timeout, with at least +1 s) so the orchestrator’s watchdog fires first; this ensures the agent, not the SDK, controls whether a restart is attempted.

When a shared restart succeeds, queued sessions resume automatically. If the restart window expires or the child process fails to boot, subsequent tool calls now fail with explicit error codes (`mcp_restart_in_progress` while a restart is still running, `mcp_restart_failed` when recovery ultimately fails), making the failure mode visible to both logs and the LLM transcript.

#### AI-agent placeholders vs MCP runtime env
| Scope | Defined in | Resolution moment | Consumer | Notes |
| --- | --- | --- | --- | --- |
| Agent-level placeholders | `.ai-agent.env` next to each JSON layer or `process.env` | During config load via `expandPlaceholders` | Provider entries, MCP `command/args/url/headers/env`, REST/OpenAPI config, telemetry | Once replaced, values are plain strings; they are not re-resolved later. |
| MCP stdio runtime env | `mcpServers.<name>.env` after placeholder expansion | When `MCPProvider` launches the stdio binary | `child_process.spawn` environment | The map replaces the entire env for the child. Add `PATH`, `HOME`, etc., explicitly if the server needs them. |
| MCP remote auth headers | `mcpServers.<name>.headers` (placeholders allowed) | When HTTP/WS/SSE transports send requests | Remote MCP endpoint | Use headers (or query params) for API keys when the server lives elsewhere; the `env` block is ignored for remote transports. |

- Bridge secrets explicitly: declare them once in `.ai-agent.env`, reference them wherever needed in `.ai-agent.json`, and let placeholder expansion copy the value into both providers and MCP servers.
  ```env
  # .ai-agent.env
  OPENAI_API_KEY=sk-live...
  CODEBASE_TOKEN=ghp_...
  MCP_ROOT=/home/ai/repos/project
  ```
  ```json
  "providers": { "openai": { "type": "openai", "apiKey": "${OPENAI_API_KEY}" } },
  "mcpServers": {
    "codebase": {
      "type": "stdio",
      "command": "node",
      "args": ["./servers/codebase.js"],
      "env": {
        "TOKEN": "${CODEBASE_TOKEN}",
        "ROOT": "${MCP_ROOT}"
      }
    }
  }
  ```
- `MCP_ROOT` has a safety net: if the resolved value is empty/blank, the resolver swaps in `process.cwd()` (and emits a verbose log when enabled) so filesystem servers always receive a root path.
- MCP tool inputs remain pure JSON arguments supplied by the LLM. `.ai-agent.env` values are never injected into tool parameters automatically—encode defaults inside the MCP server or prompt the LLM to pass them explicitly.

### REST tool definitions & OpenAPI imports
Direct declaration:
```json
"restTools": {
  "catalog_listItems": {
    "description": "List catalog entries",
    "method": "GET",
    "url": "https://api.example.com/catalog",
    "headers": { "Authorization": "Bearer ${CATALOG_TOKEN}" },
    "parametersSchema": {
      "type": "object",
      "properties": {
        "query": { "type": "string" },
        "limit": { "type": "integer", "minimum": 1, "maximum": 100 }
      }
    }
  }
}
```
Body-bearing endpoints can add `"bodyTemplate": { "query": "${parameters.query}" }` to render payloads. Streaming outputs use the `streaming` sub-object.

OpenAPI import sample:
```json
"openapiSpecs": {
  "inventory": {
    "spec": "./specs/inventory.yaml",
    "baseUrl": "https://api.example.com",
    "headers": { "Authorization": "Bearer ${INVENTORY_TOKEN}" },
    "includeMethods": ["get", "post"],
    "tagFilter": ["catalog"]
  }
}
```
This generates `rest__inventory_*` tools automatically. Reference them from frontmatter (`tools: ["rest__inventory_getCatalog"]`).

### Tool filtering semantics
- Listing an MCP server in frontmatter exposes **all** surviving tools after `toolsAllowed/toolsDenied`. To expose just one tool, keep the server name in frontmatter but set `toolsAllowed` to `"tool_you_want"`.
- Manually listing individual tool IDs in frontmatter is only needed for REST imports or bespoke tool providers; MCP servers remain coarse-grained handles.

### Provider reasoning & custom fields
- `reasoning` accepts a single value (string or number) or a tuple of four entries `[minimal, low, medium, high]`. Example:
```json
"providers": {
  "anthropic": {
    "type": "anthropic",
    "apiKey": "${ANTHROPIC_API_KEY}",
    "reasoning": [null, "light", 2048, 8192]
  }
}
```
Values can be strings (Anthropic effort labels) or integers (token budgets). Use `null` to disable reasoning for a level.
- `custom.providerOptions` passes backend-specific knobs untouched to providers, e.g.:
```json
"providers": {
  "ollama": {
    "type": "ollama",
    "baseUrl": "http://localhost:11434",
    "custom": {
      "providerOptions": {
        "ollama": {
          "options": { "num_ctx": 8192, "temperature": 0.2 }
        }
      }
    }
  },
  "openrouter": {
    "type": "openrouter",
    "apiKey": "${OPENROUTER_API_KEY}",
    "custom": {
      "providerOptions": {
        "openrouter": { "provider": "meta" }
      }
    }
  }
}
```

### Context windows & tokenizer overrides
- `contextWindow`: explicit token limit for a provider/model; used for budgeting when vendors omit metadata.
- `tokenizer`: string identifier consumed by the tokenizer registry (e.g., `"gpt-4o"`) so context estimates use the right encoder.
- `contextWindowBufferTokens`: extra headroom reserved per request (e.g., `1024`). The context guard subtracts this buffer to avoid hard-overflow.

### Tool registration flow recap
1. Define providers/MCP servers/REST tools in `.ai-agent.json`.
2. Frontmatter `tools` lists **server keys** (for MCP) plus any `rest__*` entries you want.
3. `toolsAllowed/toolsDenied` limit what each MCP server exposes.
4. Sub-agents live under `agents`; their callable names automatically gain the `agent__` prefix and are available to the parent without extra frontmatter wiring.

**Checklist**
- [ ] Define every provider referenced in `models`/CLI `--models`.
- [ ] For stdio MCP servers, provide `command` array or `command` + `args` and set `env` with API keys.
- [ ] Keep `defaults` aligned with your latency/cost budgets.
- [ ] Store secrets via `${VAR}` placeholders to be resolved from `.ai-agent.env`.

## 7. `.ai-agent.env` Handling
- Plain text lines `KEY=value`. Optional `export KEY=value` works; wrapping quotes are stripped; `#` starts a comment. Blank lines are skipped.
- Scope is per-layer: the `.ai-agent.env` that sits next to a JSON file is the **only** overlay consulted when resolving placeholders inside that JSON. There is no cascading of env files between system/home/cwd layers.
- Resolution pipeline for every `${NAME}`:
  1. Look for `NAME` inside the layer’s `.ai-agent.env` map.
  2. Fallback to `process.env.NAME`.
  3. If still missing, throw `Unresolved variable ${NAME} for <scope> '<id>' at <origin>` and abort load (the lone exception is `MCP_ROOT`, which defaults to `process.cwd()` if blank—see Section 6).
- The resulting value is written into the merged configuration immediately; runtime components never “re-expand” placeholders later.
- `.ai-agent.env` values are distinct from MCP runtime env values. To hand secrets to a stdio MCP server you **must** reference them in `mcpServers.<name>.env` (or `headers` for remote servers) so the resolver copies them across.
- Example:
  ```env
  OPENAI_API_KEY=sk-live...
  FILESERVER_TOKEN=fs-secret
  ```
  ```json
  "providers": {
    "openai": { "type": "openai", "apiKey": "${OPENAI_API_KEY}" }
  },
  "mcpServers": {
    "files": {
      "type": "stdio",
      "command": "python",
      "args": ["mcp_files.py"],
      "env": { "TOKEN": "${FILESERVER_TOKEN}" }
    }
  }
  ```
- Security: keep each `.ai-agent.env` git-ignored and restrict permissions (`chmod 600`). `parseEnvFile` only reads sibling files, preventing accidental traversal.

**Checklist**
- [ ] Mirror every `${VAR}` referenced in `.ai-agent.json` with an entry in the same directory’s `.ai-agent.env` or a real OS env var.
- [ ] When passing secrets to MCP servers, explicitly include them inside the server’s `env` or `headers` block after placeholder substitution.
- [ ] Use quotes when values contain spaces or shell-sensitive characters.
- [ ] Lock down file permissions so secrets remain local.

## 8. Parameter Inheritance & Override Matrix
| Priority (high→low) | Source | Notes |
| --- | --- | --- |
| 1 | CLI flags / Commander options | Includes `--temperature`, `--max-tool-turns`, `--parallel-tool-calls`, `--trace-*`, `--agent`, headend flags. | 
| 2 | `globalOverrides` passed to registry/headends | Applies to every agent/sub-agent (headend manager uses same object). |
| 3 | Agent frontmatter | Agent-specific; overrides config defaults for that file + sub-agents loaded beneath it (unless child overrides). |
| 4 | `.ai-agent.json` defaults | Resolved per config layer (Section 4). |
| 5 | Internal hardcoded defaults | Temperature 0.7, topP 1.0, llmTimeout 600 000 ms, toolTimeout 300 000 ms, maxRetries 3, maxToolTurns 10, maxToolCallsPerTurn 10, toolResponseMaxBytes 12 288 bytes, stream false.

- **Sub-agent propagation**: Parent sessions pass effective options (post-override) when launching child sessions unless the child frontmatter/CLI overrides them. `defaultsForUndefined` ensures missing knobs inherit the parent’s resolved values.
- **Runtime overrides**: `--override key=value` (applies to all agents) and `--models`/`--tools` CLI args feed `globalOverrides`. Use sparingly.

**Checklist**
- [ ] Decide which layer owns each knob; avoid mixing frontmatter + CLI for the same setting unless necessary.
- [ ] Document overrides in prompts so other AIs understand runtime behavior.
- [ ] Remember that headend-specified `globalOverrides` affect every registered agent.

## 9. Headends (Server Modes)
- **REST API (`--api <port>`)**
  - Routes: `GET /health` → `{status:"ok"}`, `GET /v1/<agent>?q=...&format=...` → streams a new agent session. `format` defaults to `markdown` unless the agent declares `output.format=json` (then pass `format=json`).
  - Concurrency: default 10, override via `--api-concurrency`.
  - Logging: each request generates `headend:api` log entries with request IDs.
- **MCP headend (`--mcp stdio|http:PORT|sse:PORT|ws:PORT`)**
  - Exposes registered agents as MCP tools; caller must supply `format` and, when `format=json`, a `schema` for validation.
  - Each transport inherits the same concurrency limiter infrastructure.
- **OpenAI-compatible headend (`--openai-completions <port>`)**
  - Implements `/v1/models`, `/v1/chat/completions` + SSE streaming. Model IDs map to registered agents (prefixed `agent/` inside `modelIdMap`).
  - Supports `stream=true`, `response_format`, `payload` passthrough. Concurrency guard via `--openai-completions-concurrency` (default 10).
- **Anthropic-compatible headend (`--anthropic-completions <port>`)**
  - Provides `/v1/models`, `/v1/messages` with SSE event stream semantics. Mirrors OpenAI headend behaviors for hints/format.
- **Headend startup**: Always register at least one agent via repeating `--agent <path>`. All headends share the same `AgentRegistry` and telemetry context.

**Checklist**
- [ ] Register every `.ai` file you plan to expose before starting headends.
- [ ] Configure concurrency limits per headend to match infrastructure capacity.
- [ ] For JSON outputs, ensure headend clients pass `format=json` and schema objects.

## 10. Slack Bot (`--slack`)
- **Transport**: Socket Mode; no public HTTP required unless you add slash commands. Tokens: `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, optional `SLACK_SIGNING_SECRET` for slash commands.
- **Config fields (`slack` block)**: `enabled`, `mentions`, `dms`, `updateIntervalMs`, `historyLimit`, `historyCharsCap`, `botToken`, `appToken`, `signingSecret`, `routing` (default persona + per-channel overrides), deny rules.
- **Engagement modes**: Mentions, DMs, channel posts, message shortcuts. Each request spawns an isolated `AIAgentSession`; logs stream back via interactive Block Kit updates with stop/retry controls.
- **Telemetry**: Slack headend automatically injects `headend:slack` logs and inherits global telemetry labels.

**Checklist**
- [ ] Populate `.ai-agent.json` `slack` block with `${VAR}` placeholders and mirror them in `.ai-agent.env`.
- [ ] Grant Slack app scopes (`app_mentions:read`, `chat:write`, etc.) and run `ai-agent --agent ... --slack`.
- [ ] Configure routing so each workspace channel maps to the correct agent/tool tree.

## 11. CLI Direct Mode
- **Invocation patterns**:
  - `ai-agent "<system prompt>" "<user prompt>"` — fastest path when embedding prompts inline.
  - `ai-agent @system.txt @user.txt` — prefix `@` to load prompt text from files.
  - `ai-agent - "Summarize"` — a lone dash reads the system prompt from stdin (cannot use `-` for both system and user simultaneously; enforced by `validatePrompts`).
- **State management flags**:
  - `--load <file>` — JSON array of prior `ConversationMessage`s appended before the new user prompt.
  - `--save <file>` — writes master conversation JSON on success; `--save-all <dir>` stores master + each sub-agent conversation under `${originTxnId}` grouping.
  - `--sessions-dir <path>` — overrides the default `~/.ai-agent/sessions` snapshot directory used by persistence callbacks.
  - `--billing-file <path>` — overrides accounting ledger target (`~/.ai-agent/accounting.jsonl` by default).
- **Prompt formatting**: CLI builds `${FORMAT}` based on `--format` (choices: `markdown`, `markdown+mermaid`, `slack-block-kit`, `tty`, `pipe`, `json`, `sub-agent`). If omitted, it selects `tty` when stdout is a TTY and `markdown` otherwise, auto-switching to JSON when `output.format=json` is declared.
- **Persistence & tracing**: `--dry-run` validates config + MCP servers without contacting LLMs. `--trace-llm`, `--trace-mcp`, `--trace-sdk`, `--verbose`, `--show-tree` mirror the logging descriptions in Section 1.
- **Debug tip**: Always smoke-test sub-agents with `--verbose` before invoking higher-level orchestrators. The verbose stream shows per-turn indices and tool names so you can confirm the planner is calling the expected tools (e.g., look for `agent:web.sweeps` before `agent:analysis` proceeds).
- **Overrides**: Any CLI option seen in `OPTIONS_REGISTRY` (temperature, timeouts, tool limits, reasoning tokens, caching, etc.) takes top priority (Section 8). Use `--override key=value` for batch overrides across every agent in the run.

**Checklist**
- [ ] Supply prompts inline, via files, or stdin (`-`) while respecting the “only one stdin source” rule.
- [ ] Use `--load/--save/--save-all` to persist or restore conversations when chaining sessions.
- [ ] Run with `--dry-run` before long jobs to surface config or MCP issues fast.
- [ ] Pin `--format` when downstream automation expects a specific structure.

## 12. Snapshots & Ledger Files
- **When snapshots fire**: `AIAgentSession.persistSessionSnapshot` runs after each sub-agent completion (`reason='subagent_finish'`) and after the session ends (`reason='final'`). CLI persistence writes only if `sessionsDir` is configured (default `~/.ai-agent/sessions`).
- **File format**: `<originId>.json.gz` (stable UUID). Payload after gunzip:
```json
{
  "version": 1,
  "reason": "final",
  "opTree": { "turns": [...], "logs": [...], ... }
}
```
  - `version` currently `1`.
  - `opTree` matches `SessionTreeBuilder.getSession()`: includes turns, ops (LLM/tool/system), metrics, child sessions, and aggregated token counts.
- **Reading snapshots**: Decompress with `gzip -cd <file>`. The `opTree` object is self-contained; no external schema required. Use it to reconstruct progress, render execution trees, or restart headend visualizations.
- **Accounting flushes**: `onAccountingFlush` appends newline-delimited JSON entries to `billingFile`. Each entry includes `timestamp`, `status`, `latency`, and either LLM token counts or tool character counts. These files are append-only; truncate with care.

**Checklist**
- [ ] Set `--sessions-dir` (or `config.persistence.sessionsDir`) when you need historical replay.
- [ ] Use origin IDs to correlate master + sub-agent snapshots; filenames share the same origin UUID.
- [ ] Decompress with gzip before analysis; the files are not tarballs.
- [ ] Keep accounting ledgers synced with cost dashboards (JSONL format is append-only).

## 13. Appendices
- **Numeric defaults (code-level)**:

| Knob | Default |
| --- | --- |
| `temperature` | `0.7` |
| `topP` | `1.0` |
| `maxOutputTokens` | `4096` |
| `repeatPenalty` | `1.1` |
| `llmTimeout` | `600000` ms |
| `toolTimeout` | `300000` ms |
| `maxRetries` | `3` |
| `maxToolTurns` | `10` |
| `maxToolCallsPerTurn` | `10` |
| `toolResponseMaxBytes` | `12288` bytes |
| `stream` | `false` |

- **Troubleshooting cues**:
  - `EXIT-TOOL-FAILURE`: tool exception bubbled; check tool logs and MCP stderr.
  - `EXIT-LLM-TIMEOUT`: provider took longer than `llmTimeout`; consider lowering streaming inactivity or increasing limit.
  - `EXIT-TOKEN-LIMIT`: prompt/tool results exceeded context budget; reduce tool verbosity or increase context window buffer via `providers.<name>.contextWindowBufferTokens`.
  - `EXIT-DRY-RUN`: indicates a successful config validation run (no action needed).
- **Quickstart checklist**:
  1. Create `.ai-agent.json` + `.ai-agent.env` with providers + MCP servers (Section 6 & 7).
  2. Author `.ai` files with frontmatter keys (Section 2) and prompt variables (Section 3).
  3. Verify config discovery order (Section 4) and load any hierarchies (Section 5).
  4. Run `ai-agent --dry-run --agent ...` to ensure the stack loads.
  5. Launch headends/Slack/CLI as needed (Sections 9–11) and monitor logs/ledger files.

**Final Checklist Before Shipping Changes**
- [ ] Update this guide whenever behavior, defaults, or schemas change.
- [ ] Re-run `npm run lint` + `npm run build` to ensure documentation edits don’t break tooling (per repo policy).
- [ ] Double-check snapshots/accounting output expectations whenever persistence code is touched.
