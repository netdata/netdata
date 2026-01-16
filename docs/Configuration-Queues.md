# Queue Configuration

Control concurrency with tool queues.

---

## Overview

Queues limit concurrent tool executions:
- Prevent overwhelming external APIs
- Control resource usage
- Ensure fair access across sessions

---

## Defining Queues

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 },
    "api": { "concurrent": 4 }
  }
}
```

### Default Queue

If no `default` queue is defined, one is injected automatically:

```json
{
  "queues": {
    "default": { "concurrent": 32 }
  }
}
```

---

## Assigning Tools to Queues

### MCP Servers

```json
{
  "mcpServers": {
    "fast-server": {
      "type": "stdio",
      "command": "fast-mcp",
      "queue": "default"
    },
    "slow-server": {
      "type": "http",
      "url": "https://slow.api.com/mcp",
      "queue": "heavy"
    }
  }
}
```

### REST Tools

```json
{
  "restTools": {
    "expensive-api": {
      "description": "Rate-limited API",
      "method": "GET",
      "url": "https://api.example.com/data",
      "queue": "api"
    }
  }
}
```

### Default Assignment

Tools without explicit queue assignment use the `default` queue.

---

## Queue Behavior

### Slot Acquisition

1. Tool execution requests a slot from its queue
2. If slot available → execute immediately
3. If no slot → wait until one is available

### Logging

When a tool waits for a slot:

```
[tool] queued: mcp__slow-server__query (queue: heavy, waiting: 2)
```

### Bypass

Internal tools bypass queues (never deadlock):
- `agent__final_report`
- `agent__task_status`
- `agent__batch`

---

## Telemetry

Queue metrics:

| Metric | Description |
|--------|-------------|
| `ai_agent_queue_depth` | Current in-use + waiting slots |
| `ai_agent_queue_wait_duration_ms` | Time spent waiting for slot |

Labels:
- `queue`: Queue name

---

## Queue Isolation Patterns

### Per-Integration Queues

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "github": { "concurrent": 5 },
    "database": { "concurrent": 10 },
    "external": { "concurrent": 2 }
  },
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "github-mcp",
      "queue": "github"
    },
    "postgres": {
      "type": "stdio",
      "command": "postgres-mcp",
      "queue": "database"
    },
    "third-party": {
      "type": "http",
      "url": "https://api.thirdparty.com/mcp",
      "queue": "external"
    }
  }
}
```

### Rate-Limited APIs

```json
{
  "queues": {
    "rate-limited": { "concurrent": 1 }
  },
  "restTools": {
    "limited-api": {
      "description": "1 req/sec API",
      "method": "GET",
      "url": "https://limited.api.com/data",
      "queue": "rate-limited"
    }
  }
}
```

---

## Sizing Recommendations

| Use Case | Concurrent |
|----------|------------|
| Local filesystem | 32+ |
| Database queries | 10-20 |
| Public APIs | 2-5 |
| Rate-limited APIs | 1 |
| Heavy computation | 2-4 |

---

## Debugging

### Check Queue Status

With `--verbose`:

```
[tool] queued: mcp__api__query (queue: heavy, waiting: 2)
[tool] acquired: mcp__api__query (queue: heavy, wait: 1523ms)
```

### Monitor Metrics

```bash
curl localhost:9090/metrics | grep ai_agent_queue
```

---

## Example: Production Setup

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "web": { "concurrent": 8 },
    "database": { "concurrent": 10 },
    "llm-sub-agents": { "concurrent": 4 }
  },
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "fs-mcp",
      "queue": "default"
    },
    "brave": {
      "type": "stdio",
      "command": "brave-mcp",
      "queue": "web"
    },
    "fetcher": {
      "type": "http",
      "url": "https://mcp.jina.ai/v1",
      "queue": "web"
    },
    "postgres": {
      "type": "stdio",
      "command": "pg-mcp",
      "queue": "database"
    }
  }
}
```

---

## See Also

- [Configuration](Configuration) - Overview
- [MCP Tools](Configuration-MCP-Tools) - Tool configuration
