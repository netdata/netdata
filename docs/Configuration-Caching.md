# Caching Configuration

Configure response caching for agents and tools.

---

## Table of Contents

- [Overview](#overview) - Caching layers and purposes
- [Cache Backend](#cache-backend) - SQLite and Redis configuration
- [Agent Response Caching](#agent-response-caching) - Cache complete agent responses
- [Tool Response Caching](#tool-response-caching) - Cache MCP and REST tool responses
- [Anthropic Cache Control](#anthropic-cache-control) - Provider-side prompt caching
- [TTL Formats](#ttl-formats) - Duration syntax reference
- [Cache Keys](#cache-keys) - How cache keys are computed
- [Cache Behavior](#cache-behavior) - Hits, misses, and invalidation
- [Configuration Reference](#configuration-reference) - All caching options
- [Debugging](#debugging) - Inspect and manage cache
- [Best Practices](#best-practices) - When to cache and TTL guidelines
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent supports caching at multiple levels:

| Level | Purpose                         | Configuration                   |
| ----- | ------------------------------- | ------------------------------- |
| Agent | Cache complete agent responses  | Frontmatter `cache:`            |
| Tool  | Cache individual tool responses | Config `cache:` per server/tool |
| LLM   | Provider-side prompt caching    | Frontmatter/CLI `caching:`      |

Caching reduces costs, improves latency, and prevents redundant API calls.

---

## Cache Backend

Configure where cached data is stored.

### SQLite Backend (Default)

```json
{
  "cache": {
    "backend": "sqlite",
    "sqlite": {
      "path": "~/.ai-agent/cache.db"
    },
    "maxEntries": 5000
  }
}
```

### Redis Backend

```json
{
  "cache": {
    "backend": "redis",
    "redis": {
      "url": "redis://localhost:6379"
    },
    "maxEntries": 10000
  }
}
```

### Backend Configuration Reference

| Property          | Type     | Default                  | Description                                                    |
| ----------------- | -------- | ------------------------ | -------------------------------------------------------------- |
| `backend`         | `string` | `"sqlite"`               | Cache backend: `sqlite` or `redis`                             |
| `sqlite.path`     | `string` | `"~/.ai-agent/cache.db"` | SQLite database path                                           |
| `redis.url`       | `string` | -                        | Redis connection URL                                           |
| `redis.username`  | `string` | -                        | Redis username                                                 |
| `redis.password`  | `string` | -                        | Redis password                                                 |
| `redis.database`  | `number` | -                        | Redis database number                                          |
| `redis.keyPrefix` | `string` | `"ai-agent:cache:"`      | Redis key prefix                                               |
| `maxEntries`      | `number` | `5000`                   | Maximum cache entries (SQLite: LRU eviction, Redis: TTL-based) |

---

## Agent Response Caching

Cache complete agent responses to avoid redundant LLM calls.

### Basic Usage

```yaml
---
models:
  - openai/gpt-4o
cache: 1h
---
Answer questions about our documentation.
```

### Cache Key Components

Agent cache keys are computed from:

- **Agent hash**: Expanded system prompt + configuration
- **User prompt**: The input query
- **Output format**: Expected format and schema (if any)

Same inputs produce the same cache key.

### When Agent Caching Helps

- FAQ-style agents with common questions
- Documentation lookup agents
- Reference data queries
- Repeated identical requests

### When to Avoid Agent Caching

- Agents with external tool calls (results may change)
- Time-sensitive queries
- User-specific responses
- Agents that modify state

---

## Tool Response Caching

Cache responses from MCP servers and REST tools.

### Server-Wide Cache

Apply cache TTL to all tools from a server:

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
    "api": {
      "type": "stdio",
      "command": "./api-server",
      "cache": "5m",
      "toolsCache": {
        "slow_query": "1h",
        "fast_lookup": "30s",
        "realtime_data": "off"
      }
    }
  }
}
```

### REST Tool Cache

```json
{
  "restTools": {
    "weather": {
      "description": "Get current weather",
      "method": "GET",
      "url": "https://api.weather.com/current?location=${parameters.location}",
      "cache": "15m",
      "parametersSchema": {
        "type": "object",
        "properties": {
          "location": { "type": "string" }
        },
        "required": ["location"]
      }
    }
  }
}
```

### Tool Cache Key Components

Tool cache keys are computed from:

- **Tool identity**: Namespace + tool name
- **Request payload**: All parameters serialized

---

## Anthropic Cache Control

Anthropic models support server-side prompt caching for cost savings.

### Enable Cache Mode

The cache mode is controlled via frontmatter or CLI option, not provider configuration:

```yaml
---
caching: full
---
```

Or via CLI:

```bash
ai-agent --caching full --agent my.ai "query"
```

### Cache Modes

| Mode   | Description                                         |
| ------ | --------------------------------------------------- |
| `full` | Apply ephemeral cache control to messages (default) |
| `none` | Disable cache control                               |

### How It Works

When `caching: "full"`:

1. Applies `cacheControl: { type: 'ephemeral' }` to the last valid user message per turn
2. Anthropic caches the prompt prefix server-side
3. Subsequent requests with the same prefix use cached tokens

### Cost Implications

| Token Type  | Price Impact                          |
| ----------- | ------------------------------------- |
| Cache write | Slightly higher than normal input     |
| Cache read  | Significantly lower than normal input |
| Cache hit   | Major cost savings on long prompts    |

### Cache Token Accounting

```json
{
  "type": "llm",
  "inputTokens": 1523,
  "outputTokens": 456,
  "cacheReadInputTokens": 1200,
  "cacheWriteInputTokens": 323
}
```

Telemetry metrics:

- `ai_agent_llm_cache_read_tokens_total`
- `ai_agent_llm_cache_write_tokens_total`

---

## TTL Formats

Cache TTL (Time To Live) can be specified in multiple formats.

| Format       | Example | Duration   |
| ------------ | ------- | ---------- |
| Disabled     | `off`   | No caching |
| Milliseconds | `60000` | 60 seconds |
| Seconds      | `30s`   | 30 seconds |
| Minutes      | `5m`    | 5 minutes  |
| Hours        | `1h`    | 1 hour     |
| Days         | `1d`    | 1 day      |
| Weeks        | `1w`    | 1 week     |
| Months       | `1mo`   | 30 days    |
| Years        | `1y`    | 365 days   |

### Examples

```yaml
cache: off      # Disabled
cache: 30s      # 30 seconds
cache: 5m       # 5 minutes
cache: 1h       # 1 hour
cache: 1d       # 1 day
cache: 60000    # 60 seconds (milliseconds)
```

---

## Cache Keys

### Agent Cache Key

```
hash(agent_hash + user_prompt + output_format)
```

Where `agent_hash` is computed from:

- Expanded system prompt (after variable substitution)
- All frontmatter configuration
- Model selection

### Tool Cache Key

```
hash(tool_namespace + tool_name + serialized_parameters)
```

All parameters are included, so different parameter values produce different keys.

### Key Implications

- Same query to same agent = cache hit
- Modified agent prompt = cache miss (new hash)
- Same tool call with same parameters = cache hit
- Tool call with different parameters = cache miss

---

## Cache Behavior

### Cache Hits

When a cache hit occurs:

- Response returned immediately
- No LLM or tool calls made
- Logged at verbose level: `cache hit: <agent_name>` or `cache hit: <tool_name>`

### Cache Misses

When a cache miss occurs:

- Normal execution proceeds
- Result stored in cache with TTL
- Silent (no log by default)

### Cache Invalidation

Caches are invalidated when:

- **TTL expires**: Expired entries skipped on access and removed during writes
- **Agent modified**: New agent hash = new cache key
- **Max entries exceeded** (SQLite only): LRU eviction removes oldest entries
- **Manual clear**: Database deleted or entry removed

### Stale Data

Cached responses may become stale:

- External data changes after caching
- Tool responses reflect old state
- Use appropriate TTLs for data volatility

---

## Configuration Reference

### Cache Backend Schema

```json
{
  "cache": {
    "backend": "sqlite | redis",
    "sqlite": {
      "path": "string"
    },
    "redis": {
      "url": "string",
      "username": "string",
      "password": "string",
      "database": "number",
      "keyPrefix": "string"
    },
    "maxEntries": "number"
  }
}
```

### Provider Cache Options

Cache mode is controlled via frontmatter or CLI, not provider configuration:

```yaml
---
caching: full | none
---
```

Or via CLI:

```bash
ai-agent --caching full --agent my.ai "query"
```

### MCP Server Cache Options

```json
{
  "mcpServers": {
    "<name>": {
      "cache": "string | number",
      "toolsCache": {
        "<tool>": "number"
      }
    }
  }
}
```

### REST Tool Cache Options

```json
{
  "restTools": {
    "<name>": {
      "cache": "string | number"
    }
  }
}
```

### Frontmatter Cache Option

```yaml
---
cache: string | number # string values (e.g., "5m", "1h") are parsed to milliseconds
---
```

### All Cache Properties

| Location    | Property                | Type     | Default                  | Description                                            |
| ----------- | ----------------------- | -------- | ------------------------ | ------------------------------------------------------ |
| Global      | `cache.backend`         | `string` | `"sqlite"`               | Backend type                                           |
| Global      | `cache.sqlite.path`     | `string` | `"~/.ai-agent/cache.db"` | SQLite path                                            |
| Global      | `cache.redis.url`       | `string` | -                        | Redis URL                                              |
| Global      | `cache.redis.username`  | `string` | -                        | Redis username                                         |
| Global      | `cache.redis.password`  | `string` | -                        | Redis password                                         |
| Global      | `cache.redis.database`  | `number` | -                        | Redis database number                                  |
| Global      | `cache.redis.keyPrefix` | `string` | `"ai-agent:cache:"`      | Redis key prefix                                       |
| Global      | `cache.maxEntries`      | `number` | `5000`                   | Max entries (SQLite LRU eviction, Redis uses TTL only) |
| Frontmatter | `caching`               | `string` | `"full"`                 | Anthropic cache mode                                   |
| MCP Server  | `cache`                 | `number` | `"off"`                  | Server-wide TTL                                        |
| MCP Server  | `toolsCache.<tool>`     | `number` | Server default           | Per-tool TTL                                           |
| REST Tool   | `cache`                 | `number` | `"off"`                  | Tool TTL                                               |
| Frontmatter | `cache`                 | `number` | `"off"`                  | Agent response TTL                                     |

---

## Debugging

### Check Cache Database

```bash
# View tables
sqlite3 ~/.ai-agent/cache.db ".tables"

# Count entries
sqlite3 ~/.ai-agent/cache.db "SELECT COUNT(*) FROM cache_entries"

# View recent entries
sqlite3 ~/.ai-agent/cache.db "SELECT key_hash, created_at FROM cache_entries ORDER BY created_at DESC LIMIT 10"
```

### Disable Cache Temporarily

```yaml
---
cache: off
---
```

Or clear environment:

```bash
rm ~/.ai-agent/cache.db
```

### Verbose Cache Logging

```bash
ai-agent --agent test.ai --verbose "query"
```

Shows cache hit/miss decisions.

### Force Cache Miss

Modify the query slightly to generate a new cache key:

```bash
# Original
ai-agent --agent test.ai "What is the weather?"

# Force miss
ai-agent --agent test.ai "What is the weather? (fresh)"
```

---

## Best Practices

### Do Cache

| Use Case               | Suggested TTL |
| ---------------------- | ------------- |
| Static reference data  | `1d` - `1w`   |
| Documentation lookups  | `1h` - `4h`   |
| API responses (stable) | `15m` - `1h`  |
| Search results         | `1h` - `4h`   |
| Expensive computations | `1h`          |

### Don't Cache

| Use Case                     | Reason                          |
| ---------------------------- | ------------------------------- |
| Real-time data               | Stale immediately               |
| User-specific responses      | Wrong data for other users      |
| Operations with side effects | Cached response != action taken |
| Security-sensitive queries   | Cache may leak data             |

### TTL Guidelines

| Data Volatility  | TTL            |
| ---------------- | -------------- |
| Static/immutable | `1w` or longer |
| Daily updates    | `1h` - `4h`    |
| Hourly updates   | `15m` - `30m`  |
| Real-time        | `off`          |

### Production Recommendations

1. **Start conservative**: Use shorter TTLs initially
2. **Monitor cache hit rates**: Increase TTLs for frequently hit entries
3. **Set appropriate maxEntries**: Based on memory/disk constraints
4. **Use Redis for multi-instance**: SQLite is single-process only
5. **Clear cache on deployments**: Prevents stale responses after updates

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [Pricing](Configuration-Pricing) - Cache cost implications
- [Providers](Configuration-Providers) - Provider cache strategies
- [MCP Servers](Configuration-MCP-Servers) - Server cache configuration
- [REST Tools](Configuration-REST-Tools) - REST tool cache configuration
