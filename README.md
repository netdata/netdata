# AI Agent - Composable Autonomous AI Agents Framework

**Build powerful AI agents in minutes, not months. Write a prompt, add tools, run it.**

---

## Core Architecture

- **Recursive Autonomous Agents**: Every agent continuously plans, executes, observes, and adapts
- **100% Session Isolation**: Each agent run is a completely independent universe with ZERO shared state (MCP clients, providers, conversation, accounting)
- **TypeScript + Vercel AI SDK 5**: Full type safety with unified provider interface
- **Library-First Design**: Core functionality as embeddable library; CLI is thin wrapper

---

## Key Capabilities by Feature Area

### LLM Provider Support

- **Commercial Models**: OpenAI, Anthropic, Google
- **Open-Source Models**: OpenRouter, Ollama, self-hosted (vLLM, llama.cpp, nova)
- **Special Open-Source Reliability**: Graceful degradation, retry strategies, and fallback chains ensure valid answers even when models are unreliable
- Provider/model fallback chain (model-first, then providers per model)
- Per-model context window and tokenizer configuration
- Interleaved reasoning replay support
- Streaming and non-streaming modes

### Reasoning Levels

- **Variable Reasoning Control**: Models that support it (OpenAI, Anthropic, Google, open-source) can be configured via `reasoning` frontmatter key
- Per-agent reasoning levels: `none`, `minimal`, `low`, `medium`, `high`
- Default reasoning can be set globally; per-agent overrides available

### Multi-Agent Orchestration

- **Advisors**: Parallel pre-run agents; outputs inject as `<advisory>` blocks
- **Router**: `router__handoff-to` tool for delegation to sub-agents
- **Handoff**: Post-run execution chain for multi-stage workflows
- **Agent-as-a-Tool**: Any `.ai` file instantly becomes a callable tool

### Input/Output Contracts

- **Structured I/O**: Define JSON schemas for agent inputs and outputs
- **Free-Form I/O**: Support for plain text/markdown inputs and outputs
- Per-agent format selection: `format: json | markdown | text`
- Schema validation ensures type-safe composition

### Tool System

- AI SDK-compatible virtual tools interface
- MCP tools auto-converted to SDK tools
- REST/OpenAPI tool support with queue binding
- Tool output storage (`tool_output` handles) for oversized responses

### MCP (Model Context Protocol)

- Full MCP SDK compliance (`@modelcontextprotocol/sdk` 1.17.4)
- Transports: `stdio`, `http`, `sse`, `websocket`
- Shared server registry with automatic restart (exponential backoff: 0, 1, 2, 5, 10, 30, 60s)
- Per-tool environment scoping (no leakage between tools)
- Queue-based concurrency control for heavy MCP servers

### Configuration System

- Multi-level resolution: CLI → config file → prompt dir → user home → system
- YAML frontmatter in `.ai` files drives all agent behavior
- Environment variable expansion (`${VAR}`) with `.ai-agent.env` sidecar
- Per-agent overrides: temperature, maxTurns, timeout, reasoning level

---

## Caching

### Cache Accounting (LLM Responses)

- **Provider Cache Tokens**: All providers report `cacheReadInputTokens` and `cacheWriteInputTokens`
- **Accounting Integration**: Cache tokens included in cost calculations and LLM accounting entries
- **Pricing Tables**: Optional per-model pricing for cache reads/writes (different from regular token pricing)
- **Telemetry**: `ai_agent_llm_cache_read_tokens_total`, `ai_agent_llm_cache_write_tokens_total` counters

### Anthropic Cache Control

- **Strategies**:
  - `full` (default): Apply ephemeral cache control to the last valid user message per turn
  - `none`: Skip cache control entirely
- **Implementation**: Applies `cacheControl: { type: 'ephemeral' }` to the last non-XML-notice message
- **Benefit**: Significant cost savings on repeated context patterns

### Agent Response Caching

- **Backend Options**: SQLite (default) or Redis
- **Per-Agent TTL**: `cache: off | <ms> | <N.Nu>` (e.g., `5m`, `1h`)
- **Key Components**: Hash of agent hash + user prompt + expected format/schema
- **Logging**: Cache hits logged; misses silent

### Tool Response Caching

- **Per-Tool TTL Configuration**:
  - `mcpServers.<name>.cache`: Entire server cache TTL
  - `mcpServers.<name>.toolsCache.<tool>`: Per-tool override
  - `restTools.<tool>.cache`: REST tool cache TTL
- **Key Components**: Tool namespace + name + request payload
- **Same Backend**: Uses same SQLite/Redis backend as agent caching

---

## Headend Modes (Multi-Protocol Deployment)

| Headend                              | Purpose                                                       | Who Can Use It                                                    |
| ------------------------------------ | ------------------------------------------------------------- | ----------------------------------------------------------------- |
| **`--cli`**                          | Direct agent execution; `.ai` files become executable scripts | Bash orchestration, cron jobs, CI/CD pipelines                    |
| **`--api <port>`**                   | REST API; agents as endpoints                                 | Any REST client (curl, scripts, web apps, custom backends)        |
| **`--mcp <transport>`**              | MCP server; agents become tools for MCP-aware clients         | Claude Code, Codex, Gemini, OpenCode, VS Code, any MCP-enabled AI |
| **`--openai-completions <port>`**    | OpenAI-compatible Chat Completions                            | Open WebUI, LM Studio, any OpenAI API client                      |
| **`--anthropic-completions <port>`** | Anthropic Messages API                                        | Any Anthropic API client                                          |
| **`--embed <port>`**                 | Public embeddable chat endpoint                               | Any webpage via `/ai-agent-public.js` + SSE chat                  |
| **`--slack`**                        | Slack Socket Mode app with Block Kit UI                       | Slack users, channels, slash commands                             |

Each headend flag is repeatable. All headends share the same agent registry and respect per-agent limits.

---

## Production Safeguards

- **Context Window Guard**: Token-based budget enforcement with early final-turn
- **Timeouts**: LLM inactivity (`llmTimeout`), tool execution (`toolTimeout`), per-call
- **Concurrency Limits**: Per-headend and global queue slots
- **Retry Logic**: Status-based (rate_limit, network_error, timeout → retry; auth_error, quota_exceeded → skip)
- **Loop Prevention**: Configurable recursion depth and maxTurns caps
- **Exit Codes**: 20+ distinct exit codes for precise debugging

---

## Testing & Quality Assurance

- **Phase 1**: Parallel unit tests (~200 tests, ~300ms)
- **Phase 2**: End-to-end deterministic harness (~260 scenarios)
- **Phase 3**: Real LLM integration tests (nova models)
- Built-in accounting and session snapshots for debugging

---

## Observability & Telemetry

- **Structured Logging**: VRB/WRN/ERR/TRC severity levels with TTY coloring
- **Accounting**: JSONL logging for LLM (tokens, cost, latency) and tool calls (bytes, latency)
- **Metrics**: `ai_agent_queue_depth`, `ai_agent_context_guard_*`, `ai_agent_final_report_*`
- **OTLP Export**: Traces, metrics, and logs to collectors
- **Prometheus Endpoint**: `/metrics` at configurable host:port
- **Session Snapshots**: `~/.ai-agent/sessions/{originId}.json.gz` for debugging

---

## Developer Experience

- **Single Prompt File**: `.ai` files with YAML frontmatter + system prompt
- **Zero Boilerplate**: No orchestration code, state management, or error handling
- **CI/CD Friendly**: Test each agent in isolation; version control as text files
- **Library API**: `AIAgent.create()` and `AIAgent.run()` for embedding
- **Callbacks**: `onEvent` for complete control over I/O

---

## Documentation for AI Assistants

**See [`docs/AI-AGENT-GUIDE.md`](docs/AI-AGENT-GUIDE.md)**: Single-page reference for LLM agents designing ai-agents. Covers:

- Frontmatter schema and all available keys
- Tool composition patterns
- Provider/tool selection guidance
- Common patterns and anti-patterns
- Prompt variable substitutions
- Output format contracts

---

## Real Impact

- Neda CRM: 22 specialized agents, ~2,000 lines of prompts total
- Traditional equivalent: 20,000+ lines of orchestration code
- Development: weeks vs months; text file edits vs code refactoring

---

## Support

- GitHub Issues: https://github.com/netdata/ai-agent/issues
- Discussions: https://github.com/netdata/ai-agent/discussions
- Documentation: docs/ (see the list in the repo)
