# Caching Configuration

Configure response caching for agents and tools.

---

## Overview

AI Agent supports caching at multiple levels:

| Level | Purpose | Configuration |
|-------|---------|---------------|
| Agent | Cache agent responses | Frontmatter `cache:` |
| Tool | Cache tool responses | Config `cache:` per server/tool |
| LLM | Provider-side caching | Anthropic `cacheStrategy` |

---

## Cache Backend

Configure the cache backend:

```json
{
  "cache": {
    "backend": "sqlite",
    "sqlite": {
      "path": "${HOME}/.ai-agent/cache.db"
    },
    "maxEntries": 5000
  }
}
```

### Options

| Backend | Description |
|---------|-------------|
| `sqlite` | SQLite database (default) |
| `redis` | Redis server |

### Redis Configuration

```json
{
  "cache": {
    "backend": "redis",
    "redis": {
      "url": "redis://localhost:6379"
    }
  }
}
```

---

## Agent Response Caching

Cache complete agent responses:

```yaml
---
models:
  - openai/gpt-4o
cache: 1h
---
```

### TTL Formats

| Format | Example | Description |
|--------|---------|-------------|
| `off` | `cache: off` | Disable caching |
| Milliseconds | `cache: 60000` | 60 seconds |
| Duration | `cache: 5m` | 5 minutes |
| Duration | `cache: 1h` | 1 hour |
| Duration | `cache: 1d` | 1 day |
| Duration | `cache: 1w` | 1 week |

### Cache Key

Agent cache key is computed from:
- Agent hash (expanded prompt + config)
- User prompt
- Expected output format/schema

---

## Tool Response Caching

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
      "description": "Get weather",
      "method": "GET",
      "url": "https://api.weather.com/current",
      "cache": "15m"
    }
  }
}
```

### Tool Cache Key

Tool cache key is computed from:
- Tool identity (namespace + name)
- Request payload

---

## Anthropic Cache Control

Special caching for Anthropic models:

```json
{
  "providers": {
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}",
      "cacheStrategy": "full"
    }
  }
}
```

### Strategies

| Strategy | Description |
|----------|-------------|
| `full` (default) | Apply ephemeral cache control to messages |
| `none` | Disable cache control |

### How It Works

When `cacheStrategy: "full"`:
- Applies `cacheControl: { type: 'ephemeral' }` to the last valid user message per turn
- Enables significant cost savings on repeated context patterns

### Cache Accounting

Cache tokens are tracked in accounting:
- `cacheReadInputTokens`: Tokens read from cache
- `cacheWriteInputTokens`: Tokens written to cache

Telemetry counters:
- `ai_agent_llm_cache_read_tokens_total`
- `ai_agent_llm_cache_write_tokens_total`

---

## Cache Behavior

### Cache Hits

- Logged at verbose level
- Response returned immediately
- No LLM/tool calls made

### Cache Misses

- Silent (no log)
- Normal execution proceeds
- Result stored in cache

### Cache Invalidation

Caches are invalidated when:
- TTL expires
- Agent hash changes (prompt modified)
- `maxEntries` exceeded (LRU eviction)

---

## Debugging Cache

### Check Cache Status

```bash
# View cache file
sqlite3 ~/.ai-agent/cache.db ".tables"
sqlite3 ~/.ai-agent/cache.db "SELECT COUNT(*) FROM cache"
```

### Disable Cache Temporarily

```yaml
---
cache: off
---
```

### Clear Cache

```bash
rm ~/.ai-agent/cache.db
```

---

## Best Practices

### Do Cache

- Static or slow-changing data (reference docs, config)
- Expensive API calls (web search results)
- Idempotent operations

### Don't Cache

- Real-time data (stock prices, live metrics)
- User-specific responses
- Operations with side effects

### TTL Guidelines

| Data Type | Suggested TTL |
|-----------|---------------|
| Static reference | 1d - 1w |
| API responses | 15m - 1h |
| Search results | 1h - 4h |
| Real-time data | off |

---

## See Also

- [Configuration](Configuration) - Overview
- [Pricing](Configuration-Pricing) - Cache cost implications
