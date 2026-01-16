# Headends (Deployment Modes)

Deploy agents through multiple network interfaces. Headends are the entry points that expose your agents to external clients.

---

## Table of Contents

- [Overview](#overview) - Quick summary of available deployment modes
- [Available Headends](#available-headends) - Table of all headend types with links
- [Running Multiple Headends](#running-multiple-headends) - How to combine deployment modes
- [Concurrency Control](#concurrency-control) - Managing concurrent requests
- [Agent Registration](#agent-registration) - How agents are exposed
- [Health Checks](#health-checks) - Monitoring endpoints
- [Quick Examples](#quick-examples) - Copy-pasteable startup commands
- [See Also](#see-also) - Related documentation

---

## Overview

A **headend** is a network service that exposes your agents to external clients. Each headend type serves different use cases:

| Use Case | Recommended Headend |
|----------|---------------------|
| Direct execution from scripts | CLI (default) |
| Building web applications | [REST API](Headends-REST) |
| Integrating with AI tools (Claude Code, Codex) | [MCP Server](Headends-MCP) |
| Drop-in replacement for OpenAI API | [OpenAI-Compatible](Headends-OpenAI-Compatible) |
| Drop-in replacement for Anthropic API | [Anthropic-Compatible](Headends-Anthropic-Compatible) |
| Team collaboration via Slack | [Slack](Headends-Slack) |
| Public website chat widget | [Embed](Headends-Embed) |
| Node.js application integration | [Library](Headends-Library) |

---

## Available Headends

| Page | CLI Flag | Description |
|------|----------|-------------|
| CLI | (default) | Direct command-line agent execution |
| [REST API](Headends-REST) | `--api <port>` | HTTP REST endpoints for agents |
| [MCP Server](Headends-MCP) | `--mcp <transport>` | Model Context Protocol server |
| [OpenAI-Compatible](Headends-OpenAI-Compatible) | `--openai-completions <port>` | OpenAI Chat Completions API |
| [Anthropic-Compatible](Headends-Anthropic-Compatible) | `--anthropic-completions <port>` | Anthropic Messages API |
| [Slack](Headends-Slack) | `--slack` | Slack Socket Mode integration |
| [Embed](Headends-Embed) | `--embed <port>` | Public embeddable chat widget |
| [Library](Headends-Library) | N/A | Programmatic Node.js embedding |

---

## Running Multiple Headends

All headend flags are **repeatable**. You can run multiple headends simultaneously:

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

**Key behaviors**:
- All headends share the same agent registry
- Each headend maintains its own concurrency limits
- Startup is sequential (deterministic port binding order)
- Shutdown waits for all active requests to complete

---

## Concurrency Control

Each headend type has its own concurrency limit to prevent resource exhaustion:

| CLI Option | Default | Description |
|------------|---------|-------------|
| `--api-concurrency <n>` | 4 | REST API concurrent sessions |
| `--openai-completions-concurrency <n>` | 4 | OpenAI headend sessions |
| `--anthropic-completions-concurrency <n>` | 4 | Anthropic headend sessions |
| `--embed-concurrency <n>` | 10 | Embed headend sessions |

MCP and Slack headends use internal limits (10 concurrent sessions by default).

**What happens when limit is reached**:
- REST/OpenAI/Anthropic: Returns `503 Service Unavailable` with `retry_after` header
- MCP: Blocks until a slot becomes available
- Slack: Queues the request

---

## Agent Registration

Agents are registered with headends using the `--agent` flag:

```bash
ai-agent --agent agents/main.ai --agent agents/helper.ai --api 8080
```

**How agents are exposed**:

| Headend | Agent Identifier |
|---------|------------------|
| REST API | Filename without `.ai` extension → `/v1/chat` |
| MCP | Filename or `toolName` from frontmatter |
| OpenAI | Filename or `toolName` as model name |
| Anthropic | Filename or `toolName` as model name |
| Slack | Configured via routing rules |
| Embed | Filename or default from config |

**Sub-agent loading**: Agents referenced in frontmatter (via `agents:` or `handoff:`) are auto-loaded.

---

## Health Checks

All network headends expose health endpoints for load balancer integration:

| Headend | Endpoint | Response |
|---------|----------|----------|
| REST API | `GET /health` | `{"status":"ok"}` |
| OpenAI | `GET /health` | `{"status":"ok"}` |
| Anthropic | `GET /health` | `{"status":"ok"}` |
| Embed | `GET /health` | `{"status":"ok"}` |

---

## Quick Examples

### REST API

```bash
# Start server
ai-agent --agent chat.ai --api 8080

# Query agent
curl "http://localhost:8080/v1/chat?q=Hello"
```

### MCP Server (stdio)

```bash
# Start MCP server for Claude Code integration
ai-agent --agent chat.ai --mcp stdio
```

### OpenAI-Compatible

```bash
# Start server
ai-agent --agent chat.ai --openai-completions 8082

# Use with OpenAI SDK
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello"}]}'
```

### Anthropic-Compatible

```bash
# Start server
ai-agent --agent chat.ai --anthropic-completions 8083

# Use with Anthropic SDK
curl http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}'
```

### Slack Bot

```bash
# Start Slack integration (requires tokens in config)
ai-agent --agent chat.ai --slack
```

### Embed Widget

```bash
# Start public chat endpoint
ai-agent --agent chat.ai --embed 8090

# Embed in website
<script src="http://localhost:8090/ai-agent-public.js"></script>
```

---

## See Also

- [Configuration](Configuration) - Provider and tool setup
- [CLI Reference](CLI) - Command-line options
- [Agent Files](Agent-Files) - Agent configuration
- [specs/headends-overview.md](specs/headends-overview.md) - Technical specification
