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
- **Namespacing**: Tools are prefixed with server name (`server__tool`)
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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/tmp"]
    }
  }
}
```

> **Note:** ai-agent ships with a high-performance read-only filesystem MCP server. No external packages needed.

### With Environment Variables

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_PERSONAL_ACCESS_TOKEN}"
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

> **Note:** `env` values are expanded at config load time from the layer's environment file or process environment.

### Common Stdio Servers

**Filesystem (built-in):**

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/allowed/path"]
    }
  }
}
```

> ai-agent includes a high-performance read-only filesystem MCP server at `/opt/ai-agent/mcp/fs/fs-mcp-server.js`.

**Multiple Filesystem Instances:**

You can configure multiple filesystem instances with different root directories by giving each a unique name:

```json
{
  "mcpServers": {
    "fs-data": {
      "type": "stdio",
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/data"]
    },
    "fs-logs": {
      "type": "stdio",
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/var/log"]
    },
    "fs-workspace": {
      "type": "stdio",
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "${MCP_ROOT}"]
    }
  }
}
```

> **Note:** `${MCP_ROOT}` is a special variable that defaults to the current working directory when empty or unset. This is useful for workspace-relative file access.

**Multiple Namespaces for Same Server:**

A powerful pattern is configuring the same MCP server multiple times with different namespaces and tool filters. Each configuration entry name becomes the namespace prefix for its tools.

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}" },
      "toolsAllowed": ["*"],
      "toolsDenied": ["create_or_update_file", "push_files", "delete_branch"]
    },
    "github-readonly": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}" },
      "toolsAllowed": ["search_code", "get_file_contents", "list_issues", "get_issue"]
    }
  }
}
```

This creates two namespaces from the same GitHub MCP server:
- `github__*` tools - All tools except dangerous write operations
- `github-readonly__*` tools - Only read operations

Agents can then reference specific namespaces:

```yaml
---
tools:
  - github          # Full access (minus denied tools)
  - github-readonly # Read-only access
---
```

**GitHub:**

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_PERSONAL_ACCESS_TOKEN}"
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
      "args": ["-y", "@brave/brave-search-mcp-server", "--transport", "stdio"],
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
    "jina": {
      "type": "http",
      "url": "https://mcp.jina.ai/v1",
      "headers": {
        "Authorization": "Bearer ${JINA_API_KEY}"
      },
      "cache": "1d"
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
      "enabled": "boolean",
      "queue": "string",
      "cache": "string | number",
      "toolSchemas": { "<tool>": "object" },
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

| Property           | Type               | Default     | Description                                           |
| ------------------ | ------------------ | ----------- | ----------------------------------------------------- |
| `type`             | `string`           | Required    | Transport type                                        |
| `command`          | `string`           | -           | Command to run (stdio only)                           |
| `args`             | `string[]`         | `[]`        | Command arguments (stdio only)                        |
| `url`              | `string`           | -           | Server URL (http/sse/websocket)                       |
| `headers`          | `object`           | `{}`        | HTTP headers                                          |
| `env`              | `object`           | `{}`        | Environment variables (stdio only)                    |
| `enabled`          | `boolean`          | -           | Enable or disable the server                          |
| `queue`            | `string`           | `"default"` | Concurrency queue name                                |
| `cache`            | `string \| number` | -           | Response cache TTL (no caching by default)            |
| `toolSchemas`      | `object`           | `{}`        | Static tool schemas (for servers without `listTools`) |
| `toolsCache`       | `object`           | `{}`        | Per-tool cache TTL overrides                          |
| `shared`           | `boolean`          | `true`      | Share server across sessions (default `true`)         |
| `healthProbe`      | `string`           | `"ping"`    | Health check method                                   |
| `requestTimeoutMs` | `number \| string` | -           | Request timeout (provider-level setting)              |
| `toolsAllowed`     | `string[]`         | `["*"]`     | Tool allowlist (default `["*"]`)                      |
| `toolsDenied`      | `string[]`         | `[]`        | Tool denylist                                         |

### Duration Formats

The following properties accept duration strings:

| Property       | Formats                       | Examples                         |
| -------------- | ----------------------------- | -------------------------------- |
| `cache`        | `off`, milliseconds, duration | `"off"`, `60000`, `"5m"`, `"1h"` |
| `toolsCache.*` | `off`, milliseconds, duration | `"off"`, `60000`, `"5m"`, `"1h"` |

Duration units: `ms`, `s`, `m`, `h`, `d`, `w`, `mo`, `y`

**Note:** `requestTimeoutMs` is a provider-level setting configured via defaults, not per-server.

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
      "args": ["-y", "@modelcontextprotocol/server-github"],
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
      "args": ["-y", "@modelcontextprotocol/server-github"],
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

Cache TTL controls how long tool responses are cached.

### Server-Wide Cache

Cache all tool responses from a server:

```json
{
  "mcpServers": {
    "jina": {
      "type": "http",
      "url": "https://mcp.jina.ai/v1",
      "headers": {
        "Authorization": "Bearer ${JINA_API_KEY}"
      },
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

Per-tool TTL takes precedence over server-level TTL when configured. Use `"off"` to disable caching for a specific tool.

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

All tools from a server inherit the server's queue assignment.

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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/tmp"],
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
- Restart on timeout (stdio only)
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

| Format             | Example               |
| ------------------ | --------------------- |
| `<server>__<tool>` | `github__search_code` |

### Accessing Specific Tools

In agent frontmatter, reference tools by server name (namespace). All allowed tools from that server become available.

To restrict which tools are exposed, use `toolsAllowed` or `toolsDenied` in the MCP server configuration.

### Verify Available Tools

Use `--list-tools` with the namespace (as used in frontmatter) to see tools visible to the model:

```bash
ai-agent --list-tools github           # Tools from github namespace
ai-agent --list-tools github-readonly  # Tools from github-readonly namespace
ai-agent --list-tools all              # All tools from all servers
```

### Verify Agent Configuration

Run an agent with `--help` to see its flattened frontmatter (resolved values from frontmatter + config defaults) and config files resolution order:

```bash
./myagent.ai --help
```

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
3. Test command manually: `node /opt/ai-agent/mcp/fs/fs-mcp-server.js /tmp`
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

- Server slow to respond
- Network issues

**Solutions:**

1. Check server health
2. Review server logs
3. Increase `toolTimeout` in defaults:

```json
{
  "defaults": {
    "toolTimeout": 120000
  }
}
```

**Note:** Tool timeout is controlled via `defaults.toolTimeout`, not per-server `requestTimeoutMs`. The timeout is scaled by 1.5x with a 1s buffer for robustness.

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
