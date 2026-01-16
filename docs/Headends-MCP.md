# MCP Server Headend

Expose agents as MCP (Model Context Protocol) tools for AI-powered clients like Claude Code, Codex CLI, and VS Code extensions.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [CLI Options](#cli-options) - Command-line configuration
- [Transport Types](#transport-types) - stdio, HTTP, SSE, WebSocket
- [Tool Schema](#tool-schema) - How agents become MCP tools
- [Client Configuration](#client-configuration) - Setup for various clients
- [Tool Invocation](#tool-invocation) - Request and response format
- [Format Requirements](#format-requirements) - Format and schema handling
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The MCP headend makes your agents available as tools for MCP-aware clients:
- **Claude Code** - Desktop AI assistant
- **Codex CLI** - OpenAI's command-line tool
- **Gemini CLI** - Google's command-line tool
- **VS Code** - With MCP extensions
- **Any MCP-compatible client**

**Key features**:
- Multiple transport options (stdio, HTTP, SSE, WebSocket)
- Each agent exposed as a single MCP tool
- Automatic schema generation from agent configuration
- Concurrent session support for network transports

---

## Quick Start

### For Claude Code (stdio)

```bash
ai-agent --agent chat.ai --mcp stdio
```

### For Network Clients (HTTP)

```bash
ai-agent --agent chat.ai --mcp http:8081
```

---

## CLI Options

### --mcp

| Property | Value |
|----------|-------|
| Type | `string` |
| Format | `stdio` or `<transport>:<port>` |
| Required | Yes (to enable MCP headend) |
| Repeatable | Yes |

**Description**: MCP transport specification. Can be specified multiple times to enable multiple transports.

**Valid formats**:
- `stdio` - Standard input/output
- `http:<port>` - HTTP streaming transport
- `sse:<port>` - Server-Sent Events transport
- `ws:<port>` - WebSocket transport

**Example**:
```bash
# Single transport
ai-agent --agent chat.ai --mcp stdio

# Multiple transports
ai-agent --agent chat.ai --mcp stdio --mcp http:8081 --mcp sse:8082
```

---

## Transport Types

### stdio

| Property | Value |
|----------|-------|
| Protocol | Standard input/output pipes |
| Sessions | Single connection only |
| Concurrency | No limiting |
| Use case | CLI integration (Claude Code, etc.) |

Communicates via stdin/stdout. Best for local CLI tools.

```bash
ai-agent --agent chat.ai --mcp stdio
```

### HTTP (Streamable)

| Property | Value |
|----------|-------|
| Protocol | HTTP POST with streaming responses |
| Sessions | Multiple via `mcp-session-id` header |
| Concurrency | Default 10 concurrent sessions |
| Endpoint | `POST /mcp` |

```bash
ai-agent --agent chat.ai --mcp http:8081
```

### SSE (Server-Sent Events)

| Property | Value |
|----------|-------|
| Protocol | Server-Sent Events |
| Sessions | Multiple concurrent clients |
| Concurrency | Default 10 concurrent sessions |
| Endpoints | `GET /mcp/sse` (stream), `POST /mcp/sse/message` (send) |

Legacy SSE transport for backwards compatibility.

```bash
ai-agent --agent chat.ai --mcp sse:8082
```

### WebSocket

| Property | Value |
|----------|-------|
| Protocol | WebSocket (ws://) |
| Sessions | Per-connection |
| Concurrency | Default 10 concurrent sessions |
| Subprotocol | `mcp` |

Bi-directional streaming communication.

```bash
ai-agent --agent chat.ai --mcp ws:8083
```

---

## Tool Schema

Each agent becomes an MCP tool. The tool definition is derived from the agent configuration:

| Tool Field | Source |
|------------|--------|
| `name` | Filename (without `.ai`) or frontmatter `toolName` |
| `description` | Frontmatter `description` |
| `inputSchema` | Frontmatter `input` or default schema |

### Default Input Schema

When an agent has no custom input schema:

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

### Custom Input Schema

Define in agent frontmatter:

```yaml
---
toolName: analyze-text
description: Analyze text for sentiment and key topics
input:
  type: object
  properties:
    text:
      type: string
      description: The text to analyze
    detailed:
      type: boolean
      description: Include detailed analysis
      default: false
  required: [text]
---
```

---

## Client Configuration

### Claude Code / Claude Desktop

In your `claude_desktop_config.json` (or MCP settings):

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

**Config file locations**:
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/claude/claude_desktop_config.json`

### HTTP Client Configuration

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

### SSE Client Configuration

```json
{
  "mcpServers": {
    "remote-agents": {
      "type": "sse",
      "url": "http://localhost:8082/mcp/sse"
    }
  }
}
```

### WebSocket Client Configuration

```json
{
  "mcpServers": {
    "remote-agents": {
      "type": "ws",
      "url": "ws://localhost:8083/"
    }
  }
}
```

---

## Tool Invocation

### Request Format

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

### Response Format

```json
{
  "content": [
    {
      "type": "text",
      "text": "Hello! I'm doing well, thank you for asking. How can I help you today?"
    }
  ]
}
```

### JSON Format Request

```json
{
  "tool": "analyzer",
  "arguments": {
    "prompt": "Analyze this text for sentiment",
    "format": "json",
    "schema": {
      "type": "object",
      "properties": {
        "sentiment": { "type": "string", "enum": ["positive", "negative", "neutral"] },
        "confidence": { "type": "number" }
      },
      "required": ["sentiment", "confidence"]
    }
  }
}
```

---

## Format Requirements

**Important**: MCP tool calls **must** include a `format` argument.

| Format | Schema Required | Description |
|--------|-----------------|-------------|
| `markdown` | No | Rich text output (default) |
| `text` | No | Plain text output |
| `json` | Yes | Structured JSON output |

When `format=json`, you must also provide a `schema`:

```json
{
  "tool": "chat",
  "arguments": {
    "prompt": "Extract key points",
    "format": "json",
    "schema": {
      "type": "object",
      "properties": {
        "points": {
          "type": "array",
          "items": { "type": "string" }
        }
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

If `instructions` are configured, they appear in `getInstructions()` responses.

---

## Troubleshooting

### Tool not appearing in client

**Symptom**: Agent tools don't show up in Claude Code or other clients.

**Possible causes**:
1. MCP server not started
2. Client configuration incorrect
3. Agent not registered

**Solutions**:
1. Verify ai-agent is running with `--mcp` flag
2. Check client config file path and syntax
3. Verify `--agent` flag points to valid agent file

### Connection refused

**Symptom**: Client cannot connect to MCP server.

**Possible causes**:
1. Wrong port number
2. Server not running
3. Firewall blocking connection

**Solutions**:
1. Verify port matches between server and client config
2. Check ai-agent process is running
3. Check firewall rules for the port

### "format is required" error

**Symptom**: Tool calls fail with missing format error.

**Cause**: MCP tools require explicit `format` argument.

**Solution**: Always include `format` in tool arguments:
```json
{
  "prompt": "Hello",
  "format": "markdown"
}
```

### JSON format failing

**Symptom**: JSON format requests fail with schema error.

**Cause**: JSON format requires a schema.

**Solution**: Include `schema` when using `format: "json"`:
```json
{
  "prompt": "Extract data",
  "format": "json",
  "schema": { "type": "object", "properties": { "data": { "type": "string" } } }
}
```

### Session timeouts

**Symptom**: Long-running requests timeout.

**Possible causes**:
1. Concurrency limit reached
2. Agent execution taking too long

**Solutions**:
1. Check if other sessions are active
2. Review agent timeout settings
3. Consider increasing concurrency limit

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Configuration-MCP-Servers](Configuration-MCP-Servers) - Using MCP tools in agents
- [Agent-Files-Tools](Agent-Files-Tools) - Configuring agent tools
- [specs/headend-mcp.md](specs/headend-mcp.md) - Technical specification
