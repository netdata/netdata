# MCP Servers Configuration

Configure Model Context Protocol (MCP) servers as tool providers.

---

## Table of Contents

- [Overview](#overview) - What MCP servers provide
- [Transport Types](#transport-types) - stdio, HTTP, SSE, WebSocket
- [Stdio Transport](#stdio-transport) - Local process servers
- [HTTP Transport](#http-transport) - Streamable HTTP servers
- [SSE Transport](#sse-transport) - Server-sent events transport
- [WebSocket Transport](#websocket-transport) - WebSocket transport
- [Server Configuration Reference](#server-configuration-reference) - All options
- [Tool Filtering](#tool-filtering) - Allow and deny lists
- [Caching](#caching) - Response caching per server and tool
- [Concurrency Queues](#concurrency-queues) - Rate limiting servers
- [Shared vs Private Servers](#shared-vs-private-servers) - Connection sharing
- [Health Probing](#health-probing) - Server health checks
- [Restart Behavior](#restart-behavior) - Automatic recovery
- [Using in Agents](#using-in-agents) - Referencing servers in agents
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related documentation

---

## Overview

MCP servers provide tools that agents can call. AI Agent manages:

- **Connection lifecycle**: Startup, health checks, restarts
- **Tool discovery**: Automatic tool enumeration and schema extraction
- **Namespacing**: Tools are prefixed with server name (`mcp__server__tool`)
- **Concurrency**: Queue-based rate limiting
- **Caching**: Optional response caching

---

## Transport Types

| Transport   | Description                  | Use Case                        |
| ----------- | ---------------------------- | ------------------------------- |
| `stdio`     | Local process (stdin/stdout) | NPM packages, local scripts     |
| `http`      | HTTP (streamable)            | Remote HTTP MCP servers         |
| `sse`       | Server-sent events           | Legacy SSE MCP servers          |
| `websocket` | WebSocket                    | Real-time bidirectional servers |

---

## Stdio Transport

Run MCP server as a local process, communicating via stdin/stdout.

### Basic Configuration

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"]
    }
  }
}
```

### With Environment Variables

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

### Stdio Configuration Reference

| Property  | Type       | Default  | Description                       |
| --------- | ---------- | -------- | --------------------------------- |
| `type`    | `string`   | Required | Must be `"stdio"`                 |
| `command` | `string`   | Required | Command to run                    |
| `args`    | `string[]` | `[]`     | Command arguments                 |
| `env`     | `object`   | `{}`     | Environment variables for process |

### Environment Scoping

Only explicitly configured environment variables are passed to the process:

```json
{
  "mcpServers": {
    "database": {
      "type": "stdio",
      "command": "./db-mcp",
      "env": {
        "DB_HOST": "${DB_HOST}",
        "DB_PORT": "${DB_PORT}",
        "DB_PASSWORD": "${DB_PASSWORD}"
      }
    }
  }
}
```

> **Note:** `env` values are NOT expanded at config load time. They pass through to child processes for runtime resolution.

### Common Stdio Servers

**Filesystem:**

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/allowed/path"]
    }
  }
}
```

**GitHub:**

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

**Brave Search:**

```json
{
  "mcpServers": {
    "brave": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-brave-search"],
      "env": {
        "BRAVE_API_KEY": "${BRAVE_API_KEY}"
      },
      "cache": "1h"
    }
  }
}
```

---

## HTTP Transport

Connect to HTTP MCP server with streamable responses.

### Configuration

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

### HTTP Configuration Reference

| Property  | Type     | Default  | Description      |
| --------- | -------- | -------- | ---------------- |
| `type`    | `string` | Required | Must be `"http"` |
| `url`     | `string` | Required | Server URL       |
| `headers` | `object` | `{}`     | HTTP headers     |

### Example: Jina Fetcher

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

---

## SSE Transport

Connect via Server-Sent Events (legacy transport).

### Configuration

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

### SSE Configuration Reference

| Property  | Type     | Default  | Description      |
| --------- | -------- | -------- | ---------------- |
| `type`    | `string` | Required | Must be `"sse"`  |
| `url`     | `string` | Required | SSE endpoint URL |
| `headers` | `object` | `{}`     | HTTP headers     |

---

## WebSocket Transport

Connect via WebSocket for real-time bidirectional communication.

### Configuration

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

### WebSocket Configuration Reference

| Property  | Type     | Default  | Description                         |
| --------- | -------- | -------- | ----------------------------------- |
| `type`    | `string` | Required | Must be `"websocket"`               |
| `url`     | `string` | Required | WebSocket URL (`ws://` or `wss://`) |
| `headers` | `object` | `{}`     | HTTP headers for upgrade            |

---

## Server Configuration Reference

Complete MCP server configuration schema:

```json
{
  "mcpServers": {
    "<name>": {
      "type": "stdio | http | sse | websocket",
      "command": "string",
      "args": ["string"],
      "url": "string",
      "headers": { "string": "string" },
      "env": { "string": "string" },
      "queue": "string",
      "cache": "string | number",
      "toolsCache": { "<tool>": "string | number" },
      "shared": "boolean",
      "healthProbe": "ping | listTools",
      "requestTimeoutMs": "number | string",
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"]
    }
  }
}
```

### All Options

| Property           | Type               | Default     | Description                                   |
| ------------------ | ------------------ | ----------- | --------------------------------------------- |
| `type`             | `string`           | Required    | Transport type                                |
| `command`          | `string`           | -           | Command to run (stdio only)                   |
| `args`             | `string[]`         | `[]`        | Command arguments (stdio only)                |
| `url`              | `string`           | -           | Server URL (http/sse/websocket)               |
| `headers`          | `object`           | `{}`        | HTTP headers                                  |
| `env`              | `object`           | `{}`        | Environment variables (stdio only)            |
| `queue`            | `string`           | `"default"` | Concurrency queue name                        |
| `cache`            | `string \| number` | -           | Response cache TTL (no caching by default)    |
| `toolsCache`       | `object`           | `{}`        | Per-tool cache TTL overrides                  |
| `shared`           | `boolean`          | `true`      | Share server across sessions (default `true`) |
| `healthProbe`      | `string`           | `"ping"`    | Health check method                           |
| `requestTimeoutMs` | `number \| string` | -           | Per-request timeout                           |
| `toolsAllowed`     | `string[]`         | `["*"]`     | Tool allowlist (default `["*"]`)              |
| `toolsDenied`      | `string[]`         | `[]`        | Tool denylist                                 |

### Duration Formats

The following properties accept duration strings:

| Property           | Formats                       | Examples                         |
| ------------------ | ----------------------------- | -------------------------------- |
| `cache`            | `off`, milliseconds, duration | `"off"`, `60000`, `"5m"`, `"1h"` |
| `toolsCache.*`     | `off`, milliseconds, duration | `"off"`, `60000`, `"5m"`, `"1h"` |
| `requestTimeoutMs` | milliseconds, duration        | `60000`, `"30s"`, `"2m"`         |

Duration units: `ms`, `s`, `m`, `h`, `d`, `w`, `mo`, `y`

---

## Tool Filtering

### Allow Specific Tools

Only expose specific tools from a server:

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsAllowed": ["search_code", "get_file_contents", "list_issues"]
    }
  }
}
```

### Deny Specific Tools

Block specific tools (allow all others):

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsDenied": ["create_or_update_file", "push_files", "delete_branch"]
    }
  }
}
```

### Precedence

If both `toolsAllowed` and `toolsDenied` are specified:

1. Tool must be in `toolsAllowed`
2. Tool must NOT be in `toolsDenied`

See [Tool Filtering](Configuration-Tool-Filtering) for advanced patterns.

---

## Caching

### Server-Wide Cache

Cache all tool responses from a server:

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

Override cache TTL for specific tools:

```json
{
  "mcpServers": {
    "database": {
      "type": "stdio",
      "command": "./db-mcp",
      "cache": "5m",
      "toolsCache": {
        "slow_query": "1h",
        "fast_lookup": "1m",
        "realtime_data": "off"
      }
    }
  }
}
```

### Cache TTL Formats

| Format       | Example | Duration   |
| ------------ | ------- | ---------- |
| `"off"`      | `"off"` | Disabled   |
| Milliseconds | `60000` | 60 seconds |
| Seconds      | `"30s"` | 30 seconds |
| Minutes      | `"5m"`  | 5 minutes  |
| Hours        | `"1h"`  | 1 hour     |
| Days         | `"1d"`  | 1 day      |
| Weeks        | `"1w"`  | 1 week     |
| Months       | `"1mo"` | 1 month    |
| Years        | `"1y"`  | 1 year     |

---

## Concurrency Queues

Limit concurrent tool calls to a server:

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

See [Queues](Configuration-Queues) for complete queue configuration.

---

## Shared vs Private Servers

### Shared (Default)

One server instance shared across all sessions:

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

**Benefits:**

- More resource efficient
- Automatic restart on failure (exponential backoff)
- Shared state across sessions

### Private

New server instance per session:

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

**Benefits:**

- Isolated state per session
- Single retry on failure
- No cross-session interference

---

## Health Probing

### Probe Methods

| Method      | Description                       |
| ----------- | --------------------------------- |
| `ping`      | MCP ping request (default)        |
| `listTools` | List tools request (if supported) |

### Configuration

```json
{
  "mcpServers": {
    "custom": {
      "type": "stdio",
      "command": "./custom-mcp",
      "healthProbe": "ping"
    }
  }
}
```

### Probe Behavior

- **Timeout:** 3000ms
- **On failure:** Triggers restart (for shared servers)
- **On success:** Server marked healthy

---

## Restart Behavior

Shared servers restart automatically on failure.

### Backoff Schedule

Restart delays: `0, 1, 2, 5, 10, 30, 60` seconds (60s repeats)

### Triggers

- Process exit
- Transport connection close
- Health probe failure

### Logging

Restart attempts are logged with `WRN` severity. Failed restart attempts are logged with `ERR` severity:

```
[WRN] shared restart attempt 1 for 'github' (transport-exit)
[ERR] shared MCP server restart failed (attempt 1): connection refused [server='github', type=stdio]
```

### Stopping Restarts

To stop restart attempts:

1. Remove or disable the server in configuration
2. Restart ai-agent

---

## Using in Agents

Reference MCP servers by name in agent frontmatter:

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

### Tool Naming Convention

Tools are exposed with namespaced names:

| Format                  | Example                    |
| ----------------------- | -------------------------- |
| `mcp__<server>__<tool>` | `mcp__github__search_code` |

### Accessing Specific Tools

In agent frontmatter, reference tools by server name. All allowed tools from that server become available.

To restrict which tools are exposed, use `toolsAllowed` or `toolsDenied` in the MCP server configuration.

---

## Troubleshooting

### Server failed to initialize

```
Error: MCP server 'name' failed to initialize
```

**Causes:**

- Command not found
- Missing environment variables
- Network unreachable (for HTTP/WS)

**Solutions:**

1. Verify command exists: `which npx`
2. Check environment variables are set
3. Test command manually: `npx -y @anthropic/mcp-server-filesystem /tmp`
4. Use `--verbose` to see stderr output

### Tool not found

```
Error: Tool 'mcp__server__tool' not found
```

**Causes:**

- Server not initialized
- Tool filtered by allowlist/denylist
- Tool name typo

**Solutions:**

1. Check server initialized successfully
2. Verify tool is not in `toolsDenied`
3. Check `toolsAllowed` includes the tool
4. Use `--dry-run --verbose` to see available tools

### Initialization timeout (60s)

```
Error: MCP server 'name' initialization timed out after 60s
```

**Causes:**

- Server unavailable (404, network error)
- Slow server startup

**Solutions:**

1. Check server is reachable
2. Verify URL/command is correct
3. Check server logs
4. Background retries continue - server may recover

### Timeout during tool execution

```
Error: Tool execution timed out
```

**Causes:**

- `requestTimeoutMs` too low
- Server slow to respond
- Network issues

**Solutions:**

1. Increase `requestTimeoutMs`:

```json
{
  "mcpServers": {
    "slow-server": {
      "requestTimeoutMs": 120000
    }
  }
}
```

2. Check server health
3. Review server logs

### Restart loop

**Symptoms:** Server repeatedly restarting

**Causes:**

- Server crashes immediately after start
- Configuration error
- Resource exhaustion

**Solutions:**

1. Check server stderr for errors
2. Test server manually
3. Review backoff timing in logs
4. Check system resources

### Memory or process leak

**Symptoms:** Growing memory usage, orphan processes

**Solutions:**

1. Ensure `cleanup()` is called on shutdown
2. Check process tree termination
3. Review PID tracking logs
4. Set `shared: false` for problematic servers

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [Tool Filtering](Configuration-Tool-Filtering) - Detailed filtering patterns
- [Queues](Configuration-Queues) - Concurrency control
- [Caching](Configuration-Caching) - Cache configuration
- [specs/tools-mcp.md](specs/tools-mcp.md) - Technical specification
