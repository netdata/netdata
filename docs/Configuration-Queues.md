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
    "default": { "concurrent": <computed> }
  }
}
```

The injected `default` queue's `concurrent` value is computed from hardware (see [Default Concurrency](#default-concurrency) below).

### Default Queue Behavior

- All tools without explicit queue assignment use `default`
- Provides baseline concurrency control
- Override with custom `default` configuration

### Default Concurrency

The injected default queue computes concurrency based on hardware:

- Formula: `coreCount * 2` (from `os.availableParallelism()` or `os.cpus().length`)
- Clamped between 1 and 64
- Example values: 4 cores → 8, 8 cores → 16, 16 cores → 32

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
ts=2025-01-16T12:34:56.789Z level=vrb type=tool direction=request turn=1 subturn=2 remote=queue:heavy message=queued queue=heavy wait_ms=1523 queued_depth=2 queue_capacity=2
```

### FIFO Order

Waiting requests are processed in order (first-in, first-out).

### Queue Bypass

Internal tools do not use queue concurrency, while MCP and REST tools always acquire queue slots.

| Tool           | Purpose                                     |
| -------------- | ------------------------------------------- |
| `agent__batch` | Batch tool execution (internal, no queuing) |

### Internal Tool Queue Behavior

Internal tools (`agent__batch`, `agent__final_report`, `agent__task_status`) do not acquire queue slots. They execute immediately regardless of queue capacity.

Tools invoked inside `agent__batch` (MCP and REST tools) follow normal queue behavior and must acquire slots.

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
rate(ai_agent_queue_wait_duration_ms_sum[5m]) / rate(ai_agent_queue_wait_duration_ms_count[5m]) by (queue)
```

---

## Debugging

### Check Queue Status

With `--verbose`:

```bash
ai-agent --agent test.ai --verbose "query"
```

Output (logfmt format):

```
ts=2025-01-16T12:34:56.789Z level=vrb type=tool direction=request turn=1 subturn=2 remote=queue:heavy message=queued queue=heavy wait_ms=1523 queued_depth=2 queue_capacity=2
```

A `queued` log entry is emitted only when a tool waits for a slot. The log includes:

- `queue`: Queue name
- `wait_ms`: Time spent waiting (milliseconds)
- `queued_depth`: Position in wait queue when acquired
- `queue_capacity`: Maximum concurrent executions for this queue

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

- Internal tools do not use queues (this is expected behavior)
- Check for circular dependencies between tools
- Verify queue assignments are valid

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [MCP Servers](Configuration-MCP-Servers) - MCP server configuration
- [REST Tools](Configuration-REST-Tools) - REST tool configuration
- [Parameters](Configuration-Parameters) - All parameters reference
