# MCP Server Headend

Expose agents as MCP tools for AI-powered clients.

---

## Overview

The MCP headend makes your agents available as tools for MCP-aware clients:
- Claude Code
- Codex CLI
- Gemini CLI
- VS Code with MCP extensions
- Any MCP-compatible client

---

## Start Server

### stdio Transport

```bash
ai-agent --agent agents/chat.ai --mcp stdio
```

### HTTP Transport

```bash
ai-agent --agent agents/chat.ai --mcp http:8081
```

### SSE Transport

```bash
ai-agent --agent agents/chat.ai --mcp sse:8082
```

### WebSocket Transport

```bash
ai-agent --agent agents/chat.ai --mcp ws:8083
```

---

## Multiple Transports

```bash
ai-agent --agent agents/chat.ai \
  --mcp stdio \
  --mcp http:8081 \
  --mcp sse:8082
```

---

## Transport Details

### stdio

Communicates via stdin/stdout:

```bash
ai-agent --agent chat.ai --mcp stdio
# Client connects to stdin/stdout
```

### HTTP (Streamable)

```
POST /mcp
```

### SSE (Server-Sent Events)

```
GET  /mcp/sse          # SSE stream
POST /mcp/sse/message  # Send messages
```

### WebSocket

```
ws://host:port/
# Subprotocol: mcp
```

---

## Tool Schema

Each agent becomes a tool with:

| Field | Source |
|-------|--------|
| `name` | Filename (without `.ai`) |
| `description` | Frontmatter `description` |
| `inputSchema` | Frontmatter `input` or default |

### Default Input Schema

```json
{
  "type": "object",
  "properties": {
    "prompt": {
      "type": "string",
      "description": "The user prompt"
    },
    "format": {
      "type": "string",
      "enum": ["markdown", "json", "text"],
      "default": "markdown"
    },
    "schema": {
      "type": "object",
      "description": "JSON Schema when format=json"
    }
  },
  "required": ["prompt", "format"]
}
```

---

## Client Configuration

### Claude Code

In your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ai-agent": {
      "command": "ai-agent",
      "args": ["--agent", "agents/chat.ai", "--mcp", "stdio"]
    }
  }
}
```

### HTTP Client

In your `.ai-agent.json`:

```json
{
  "mcpServers": {
    "remote-agents": {
      "type": "http",
      "url": "http://localhost:8081/mcp"
    }
  }
}
```

---

## Tool Invocation

When an MCP client calls your agent:

```json
{
  "tool": "chat",
  "arguments": {
    "prompt": "Hello, how are you?",
    "format": "markdown"
  }
}
```

Response:

```json
{
  "content": [
    {
      "type": "text",
      "text": "Hello! I'm doing well, thank you for asking..."
    }
  ]
}
```

---

## Format Requirements

**Important**: MCP tool calls **must** include a `format` argument.

When `format=json`, also provide `schema`:

```json
{
  "tool": "analyzer",
  "arguments": {
    "prompt": "Analyze this text",
    "format": "json",
    "schema": {
      "type": "object",
      "properties": {
        "sentiment": { "type": "string" }
      }
    }
  }
}
```

---

## Server Info

The MCP server exposes standard MCP capabilities:

```json
{
  "name": "ai-agent",
  "version": "1.0.0",
  "capabilities": {
    "tools": {}
  }
}
```

---

## See Also

- [Headends](Headends) - Overview
- [MCP Tools](Configuration-MCP-Tools) - Using MCP tools
- [docs/specs/headend-mcp.md](../docs/specs/headend-mcp.md) - Technical spec
