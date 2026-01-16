# CLI Reference

Complete command line options for ai-agent.

---

## Basic Usage

```bash
# Headend mode (recommended)
ai-agent --agent <path.ai> [--api <port>] [--mcp <transport>] ...

# Direct invocation (legacy)
ai-agent --agent <path.ai> "user prompt"
```

---

## Agent Registration

| Option | Description |
|--------|-------------|
| `--agent <path>` | Register an `.ai` file (repeatable) |

Sub-agents referenced in frontmatter are auto-loaded.

---

## Headend Options

| Option | Description | Default |
|--------|-------------|---------|
| `--api <port>` | Start REST API headend | - |
| `--mcp <transport>` | Start MCP headend (`stdio`, `http:PORT`, `sse:PORT`, `ws:PORT`) | - |
| `--openai-completions <port>` | Start OpenAI-compatible API | - |
| `--anthropic-completions <port>` | Start Anthropic-compatible API | - |
| `--embed <port>` | Start public embed headend | - |
| `--slack` | Start Slack Socket Mode app | - |

### Concurrency Limits

| Option | Description | Default |
|--------|-------------|---------|
| `--api-concurrency <n>` | Max concurrent REST sessions | 4 |
| `--openai-completions-concurrency <n>` | Max concurrent OpenAI sessions | 4 |
| `--anthropic-completions-concurrency <n>` | Max concurrent Anthropic sessions | 4 |
| `--embed-concurrency <n>` | Max concurrent embed sessions | 10 |

---

## LLM Options

| Option | Description | Default |
|--------|-------------|---------|
| `--models <list>` | Comma-separated provider/model pairs | - |
| `--temperature <n>` | LLM temperature (0.0–2.0) | 0.7 |
| `--top-p <n>` | Top-p sampling (0.0–1.0) | 1.0 |
| `--max-output-tokens <n>` | Maximum output tokens | Model default |
| `--stream` | Force streaming responses | on |
| `--no-stream` | Disable streaming | - |
| `--parallel-tool-calls` | Enable parallel tool calls | true |
| `--no-parallel-tool-calls` | Disable parallel tool calls | - |

---

## Timeout Options

| Option | Description | Default |
|--------|-------------|---------|
| `--llm-timeout <ms>` | Inactivity timeout per LLM call | 120000 |
| `--tool-timeout <ms>` | Timeout for tool execution | 60000 |

---

## Tool Options

| Option | Description | Default |
|--------|-------------|---------|
| `--tools <list>` | Comma-separated MCP server names | - |
| `--tool-response-max-bytes <n>` | Max tool response size before storage | 12288 |

---

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `--config <path>` | Configuration file path | `.ai-agent.json` |
| `--accounting <path>` | Accounting file path | From config |

---

## Debugging Options

| Option | Description |
|--------|-------------|
| `--verbose` | Enable verbose logging |
| `--trace-llm` | Trace LLM HTTP requests/responses |
| `--trace-mcp` | Trace MCP operations |
| `--dry-run` | Validate inputs only; skip LLM work |

---

## Session Options

| Option | Description |
|--------|-------------|
| `--save <path>` | Save conversation transcript |
| `--load <path>` | Load conversation history |
| `--resume <id>` | Resume from session ID |
| `--sessions-dir <path>` | Session snapshots directory |

---

## Telemetry Options

| Option | Description |
|--------|-------------|
| `--telemetry-prometheus-host <host>` | Prometheus metrics host |
| `--telemetry-prometheus-port <port>` | Prometheus metrics port |
| `--telemetry-otlp-endpoint <url>` | OTLP endpoint for traces/metrics |
| `--telemetry-logging-otlp-endpoint <url>` | OTLP endpoint for logs |

---

## Override Keys

Use `--override key=value` for runtime overrides:

| Key | Description |
|-----|-------------|
| `no-batch` | Disable `agent__batch` parallel tool execution |
| `no-progress` | Disable progress-only `task_status` behavior |
| `contextWindow` | Override context window size |
| `interleaved` | Enable/configure interleaved reasoning |

---

## Examples

```bash
# Run single agent
ai-agent --agent chat.ai "Hello"

# Run as REST API
ai-agent --agent chat.ai --api 8080

# Run multiple headends
ai-agent --agent chat.ai --api 8080 --mcp stdio --slack

# Debug mode
ai-agent --agent chat.ai --verbose --trace-llm "Test"

# Override settings
ai-agent --agent chat.ai --temperature 0.2 --override contextWindow=32000 "Query"
```

---

## See Also

- [Environment Variables](Getting-Started-Environment-Variables)
- [Configuration](Configuration)
- [Parameters Reference](Configuration-Parameters)
