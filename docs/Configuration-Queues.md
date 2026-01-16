# Queue Configuration

Control concurrency with tool queues.

---

## Table of Contents

- [Overview](#overview) - Purpose of concurrency queues
- [Defining Queues](#defining-queues) - Queue configuration syntax
- [Default Queue](#default-queue) - Automatic default queue
- [Assigning Tools to Queues](#assigning-tools-to-queues) - MCP and REST tool assignment
- [Queue Behavior](#queue-behavior) - Slot acquisition and waiting
- [Configuration Reference](#configuration-reference) - All queue options
- [Common Patterns](#common-patterns) - Production queue setups
- [Sizing Recommendations](#sizing-recommendations) - Concurrency guidelines
- [Telemetry](#telemetry) - Queue metrics
- [Debugging](#debugging) - Troubleshooting queue issues
- [See Also](#see-also) - Related documentation

---

## Overview

Queues limit concurrent tool executions:

| Purpose           | Benefit                                |
| ----------------- | -------------------------------------- |
| API rate limiting | Prevent overwhelming external services |
| Resource control  | Limit CPU/memory usage                 |
| Fair access       | Ensure equitable resource sharing      |
| Cost control      | Throttle expensive operations          |

Without queues, parallel tool calls may exceed API rate limits or exhaust system resources.

---

## Defining Queues

Configure queues in `.ai-agent.json`:

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 },
    "api": { "concurrent": 4 },
    "rate-limited": { "concurrent": 1 }
  }
}
```

### Queue Properties

| Property     | Type     | Default  | Description                     |
| ------------ | -------- | -------- | ------------------------------- |
| `concurrent` | `number` | Required | Maximum simultaneous executions |

---

## Default Queue

If no `default` queue is defined, one is injected automatically:

```json
{
  "queues": {
    "default": { "concurrent": 32 }
  }
}
```

### Default Queue Behavior

- All tools without explicit queue assignment use `default`
- Provides baseline concurrency control
- Override with custom `default` configuration

---

## Assigning Tools to Queues

### MCP Servers

Assign a queue to all tools from an MCP server:

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
    },
    "rate-limited-api": {
      "type": "stdio",
      "command": "./api-mcp",
      "queue": "rate-limited"
    }
  }
}
```

### REST Tools

Assign a queue to a REST tool:

```json
{
  "restTools": {
    "expensive-api": {
      "description": "Rate-limited external API",
      "method": "GET",
      "url": "https://api.example.com/data",
      "queue": "api",
      "parametersSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" }
        }
      }
    }
  }
}
```

### Assignment Reference

| Tool Source | Configuration                |
| ----------- | ---------------------------- |
| MCP Server  | `mcpServers.<name>.queue`    |
| REST Tool   | `restTools.<name>.queue`     |
| Default     | `"default"` if not specified |

---

## Queue Behavior

### Slot Acquisition

1. Tool execution requests a slot from its assigned queue
2. If slot available: execute immediately
3. If no slot: wait until one becomes available
4. After completion: release slot

### Waiting

When all slots are in use, new requests wait:

```
[tool] queued: mcp__slow-server__query (queue: heavy, waiting: 2)
[tool] acquired: mcp__slow-server__query (queue: heavy, wait: 1523ms)
```

### FIFO Order

Waiting requests are processed in order (first-in, first-out).

### Queue Bypass

The `agent__batch` tool bypasses queue concurrency to enable parallel tool execution within a batch. Other internal tools (`agent__final_report`, `agent__task_status`) use normal queue behavior.

| Tool           | Purpose                                                         |
| -------------- | --------------------------------------------------------------- |
| `agent__batch` | Batch tool execution (bypasses queues for parallel inner tools) |

---

## Configuration Reference

### Queue Schema

```json
{
  "queues": {
    "<name>": {
      "concurrent": "number"
    }
  }
}
```

### MCP Server Queue Assignment

```json
{
  "mcpServers": {
    "<name>": {
      "queue": "string"
    }
  }
}
```

### REST Tool Queue Assignment

```json
{
  "restTools": {
    "<name>": {
      "queue": "string"
    }
  }
}
```

### All Queue Properties

| Location   | Property     | Type     | Default     | Description                 |
| ---------- | ------------ | -------- | ----------- | --------------------------- |
| Queue      | `concurrent` | `number` | Required    | Max simultaneous executions |
| MCP Server | `queue`      | `string` | `"default"` | Queue assignment            |
| REST Tool  | `queue`      | `string` | `"default"` | Queue assignment            |

---

## Common Patterns

### Per-Integration Queues

Isolate different integrations:

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

For APIs with strict rate limits:

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
      "queue": "rate-limited",
      "parametersSchema": { "type": "object" }
    }
  }
}
```

### Sub-Agent Control

Limit concurrent sub-agent invocations:

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "llm-sub-agents": { "concurrent": 4 }
  }
}
```

### Production Setup

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

## Sizing Recommendations

| Use Case          | Concurrent | Rationale                |
| ----------------- | ---------- | ------------------------ |
| Local filesystem  | 32+        | Fast, no external limits |
| Database queries  | 10-20      | Connection pool limits   |
| Public APIs       | 2-5        | Rate limit compliance    |
| Rate-limited APIs | 1          | Strict rate limits       |
| Heavy computation | 2-4        | CPU/memory constraints   |
| Sub-agents        | 2-4        | LLM API limits           |

### Factors to Consider

| Factor                 | Lower Concurrency | Higher Concurrency |
| ---------------------- | ----------------- | ------------------ |
| API rate limits        | Yes               | No                 |
| Expensive operations   | Yes               | No                 |
| Shared resources       | Yes               | No                 |
| Fast local operations  | No                | Yes                |
| Independent operations | No                | Yes                |

---

## Telemetry

Queue metrics are exported for monitoring:

| Metric                            | Description                                                                      |
| --------------------------------- | -------------------------------------------------------------------------------- |
| `ai_agent_queue_depth`            | Number of queued tool executions awaiting a slot                                 |
| `ai_agent_queue_in_use`           | Number of active tool executions consuming queue capacity                        |
| `ai_agent_queue_wait_duration_ms` | Latency between enqueue and start time for queued tool executions (milliseconds) |
| `ai_agent_queue_last_wait_ms`     | Most recent observed wait duration per queue (milliseconds)                      |

### Labels

| Label   | Description |
| ------- | ----------- |
| `queue` | Queue name  |

### Example Query

```promql
# Average wait time per queue
avg(ai_agent_queue_wait_duration_ms) by (queue)

# Queue utilization (waiting + in-use) / capacity
(ai_agent_queue_depth + ai_agent_queue_in_use) / on(queue) group_left() (avg(ai_agent_queue_in_use) by (queue) / sum(quantile_over_time(ai_agent_queue_in_use[5m], 0.5)))
```

---

## Debugging

### Check Queue Status

With `--verbose`:

```bash
ai-agent --agent test.ai --verbose "query"
```

Output:

```
[tool] queued: mcp__api__query (queue: heavy, waiting: 2)
[tool] acquired: mcp__api__query (queue: heavy, wait: 1523ms)
[tool] completed: mcp__api__query (queue: heavy, duration: 3241ms)
```

### Monitor Metrics

```bash
curl localhost:9090/metrics | grep ai_agent_queue
```

### Common Issues

**Tools waiting too long:**

- Increase `concurrent` for the queue
- Check if one tool is blocking others
- Consider separate queues for slow tools

**Rate limit errors despite queue:**

- `concurrent` may still be too high
- API may have burst limits
- Consider adding delays between calls

**Deadlock symptoms:**

- Internal tools should bypass queues
- Check for circular dependencies
- Verify queue assignments

### Trace Queue Operations

```bash
DEBUG=queue ai-agent --agent test.ai "query"
```

Shows detailed queue acquisition/release operations.

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [MCP Servers](Configuration-MCP-Servers) - MCP server configuration
- [REST Tools](Configuration-REST-Tools) - REST tool configuration
- [Parameters](Configuration-Parameters) - All parameters reference
