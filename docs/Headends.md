# Headends (Deployment Modes)

Deploy agents through multiple network interfaces.

---

## Pages in This Section

| Page | Description |
|------|-------------|
| [CLI](Headends-CLI) | Direct command-line execution |
| [REST API](Headends-REST) | HTTP REST endpoints |
| [MCP Server](Headends-MCP) | Model Context Protocol server |
| [OpenAI-Compatible](Headends-OpenAI) | OpenAI Chat Completions API |
| [Anthropic-Compatible](Headends-Anthropic) | Anthropic Messages API |
| [Slack](Headends-Slack) | Slack Socket Mode integration |
| [Embed](Headends-Embed) | Public embeddable chat |
| [Library](Headends-Library) | Programmatic embedding |

---

## Overview

| Headend | Flag | Purpose |
|---------|------|---------|
| CLI | (default) | Direct agent execution |
| REST API | `--api <port>` | REST endpoints for agents |
| MCP | `--mcp <transport>` | MCP server (stdio/http/sse/ws) |
| OpenAI | `--openai-completions <port>` | OpenAI-compatible API |
| Anthropic | `--anthropic-completions <port>` | Anthropic-compatible API |
| Slack | `--slack` | Slack Socket Mode app |
| Embed | `--embed <port>` | Public chat endpoint |

---

## Multiple Headends

All headend flags are repeatable. Run multiple simultaneously:

```bash
ai-agent \
  --agent agents/main.ai \
  --api 8080 \
  --mcp stdio \
  --mcp http:8081 \
  --openai-completions 8082 \
  --anthropic-completions 8083 \
  --embed 8090 \
  --slack
```

All headends share the same agent registry.

---

## Concurrency Control

Per-headend concurrency limits:

| Option | Default | Description |
|--------|---------|-------------|
| `--api-concurrency <n>` | 4 | REST API concurrent sessions |
| `--openai-completions-concurrency <n>` | 4 | OpenAI headend sessions |
| `--anthropic-completions-concurrency <n>` | 4 | Anthropic headend sessions |
| `--embed-concurrency <n>` | 10 | Embed headend sessions |

Each incoming request acquires a slot before spawning an agent session.

---

## Agent Registration

Register agents for headends:

```bash
ai-agent --agent agents/main.ai --agent agents/helper.ai --api 8080
```

Sub-agents referenced in frontmatter are auto-loaded.

---

## Health Checks

All headends expose health endpoints:

| Headend | Endpoint |
|---------|----------|
| REST API | `GET /health` |
| OpenAI | `GET /health` |
| Anthropic | `GET /health` |
| Embed | `GET /health` |

---

## Quick Examples

### REST API

```bash
ai-agent --agent chat.ai --api 8080
curl "http://localhost:8080/v1/chat?q=Hello"
```

### MCP Server (stdio)

```bash
ai-agent --agent chat.ai --mcp stdio
# Connect MCP client to stdin/stdout
```

### OpenAI-Compatible

```bash
ai-agent --agent chat.ai --openai-completions 8082
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello"}]}'
```

---

## See Also

- [Configuration](Configuration) - Provider and tool setup
- [docs/specs/headends-overview.md](../docs/specs/headends-overview.md) - Technical spec
