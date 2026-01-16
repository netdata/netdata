# MCP Tools Configuration

Configure Model Context Protocol (MCP) servers as tools.

---

## Overview

MCP servers provide tools that agents can call. AI Agent supports all MCP transports:

| Transport | Description | Example |
|-----------|-------------|---------|
| `stdio` | Local process | `npx @anthropic/mcp-server-filesystem` |
| `http` | HTTP (streamable) | `https://mcp.example.com/v1` |
| `sse` | Server-sent events | `https://mcp.example.com/sse` |
| `websocket` | WebSocket | `wss://mcp.example.com/ws` |

---

## Stdio Transport

Run MCP server as local process:

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"],
      "env": {
        "NODE_OPTIONS": "--max-old-space-size=512"
      }
    }
  }
}
```

### Environment Scoping

Only explicitly configured environment variables are passed:

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

---

## HTTP Transport

Connect to HTTP MCP server:

```json
{
  "mcpServers": {
    "remote": {
      "type": "http",
      "url": "https://mcp.example.com/v1",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      }
    }
  }
}
```

---

## SSE Transport

Connect via Server-Sent Events:

```json
{
  "mcpServers": {
    "streaming": {
      "type": "sse",
      "url": "https://mcp.example.com/sse",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      }
    }
  }
}
```

---

## WebSocket Transport

Connect via WebSocket:

```json
{
  "mcpServers": {
    "realtime": {
      "type": "websocket",
      "url": "wss://mcp.example.com/ws",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      }
    }
  }
}
```

---

## Server Options

| Option | Type | Description |
|--------|------|-------------|
| `type` | `string` | Transport: `stdio`, `http`, `sse`, `websocket` |
| `command` | `string` | Command to run (stdio) |
| `args` | `string[]` | Command arguments (stdio) |
| `url` | `string` | Server URL (http/sse/websocket) |
| `headers` | `object` | HTTP headers |
| `env` | `object` | Environment variables (stdio) |
| `queue` | `string` | Concurrency queue name |
| `cache` | `string` | Response cache TTL |
| `shared` | `boolean` | Share server across sessions (default: true) |
| `healthProbe` | `string` | Health check method: `ping` or `listTools` |
| `requestTimeoutMs` | `number` | Per-request timeout |

---

## Tool Filtering

### Allow Specific Tools

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsAllowed": [
        "search_code",
        "get_file_contents",
        "list_issues"
      ]
    }
  }
}
```

### Deny Specific Tools

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsDenied": [
        "create_or_update_file",
        "push_files",
        "delete_branch"
      ]
    }
  }
}
```

---

## Caching

### Server-Wide Cache

```json
{
  "mcpServers": {
    "fetcher": {
      "type": "http",
      "url": "https://mcp.jina.ai/v1",
      "cache": "1h"
    }
  }
}
```

### Per-Tool Cache

```json
{
  "mcpServers": {
    "database": {
      "type": "stdio",
      "command": "./db-mcp",
      "cache": "5m",
      "toolsCache": {
        "slow_query": "1h",
        "fast_lookup": "1m"
      }
    }
  }
}
```

---

## Concurrency Queues

Limit concurrent calls to heavy servers:

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 }
  },
  "mcpServers": {
    "slow-api": {
      "type": "http",
      "url": "https://slow-api.example.com/mcp",
      "queue": "heavy"
    }
  }
}
```

---

## Shared vs Private Servers

### Shared (Default)

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"],
      "shared": true
    }
  }
}
```

- One instance shared across all sessions
- Automatic restart on failure (exponential backoff)
- More resource efficient

### Private

```json
{
  "mcpServers": {
    "per-session": {
      "type": "stdio",
      "command": "./session-specific-server",
      "shared": false
    }
  }
}
```

- New instance per session
- Single retry on failure
- Isolated state per session

---

## Restart Behavior

Shared servers restart automatically:

- Backoff: 0, 1, 2, 5, 10, 30, 60 seconds (60s repeats)
- Restarts on: process exit, connection close
- Logs every restart decision with `ERR`

To stop retries: disable the server and restart ai-agent.

---

## Using in Agents

Reference MCP server by name:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - filesystem
  - github
---
You have access to filesystem and GitHub tools.
```

Tool calls use namespace: `mcp__servername__toolname`

---

## See Also

- [Configuration](Configuration) - Overview
- [Tool Filtering](Configuration-Tool-Filtering) - Detailed filtering
- [Queues](Configuration-Queues) - Concurrency control
