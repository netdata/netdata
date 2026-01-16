# Pricing Configuration

Configure token pricing for cost tracking.

---

## Overview

AI Agent tracks costs by:
1. Counting tokens (input, output, cached)
2. Applying configured prices
3. Recording in accounting entries

---

## Price Configuration

```json
{
  "pricing": {
    "openai": {
      "gpt-4o": {
        "unit": "per_1m",
        "prompt": 2.50,
        "completion": 10.00
      },
      "gpt-4o-mini": {
        "unit": "per_1m",
        "prompt": 0.15,
        "completion": 0.60
      }
    },
    "anthropic": {
      "claude-sonnet-4-20250514": {
        "unit": "per_1m",
        "prompt": 3.00,
        "completion": 15.00,
        "cacheRead": 0.30,
        "cacheWrite": 3.75
      }
    }
  }
}
```

---

## Price Fields

| Field | Description |
|-------|-------------|
| `unit` | Price unit: `per_1m` (per million tokens) |
| `prompt` | Input token price |
| `completion` | Output token price |
| `cacheRead` | Cached input token read price |
| `cacheWrite` | Cached input token write price |

---

## Accounting Output

Enable accounting file:

```json
{
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
  }
}
```

Or via CLI:

```bash
ai-agent --agent test.ai --accounting ./accounting.jsonl "query"
```

### LLM Entry

```json
{
  "type": "llm",
  "status": "ok",
  "provider": "openai",
  "model": "gpt-4o",
  "inputTokens": 1523,
  "outputTokens": 456,
  "cacheReadTokens": 0,
  "cacheWriteTokens": 1523,
  "cost": 0.0084,
  "latencyMs": 2341,
  "timestamp": "2025-01-15T10:30:00Z"
}
```

### Tool Entry

```json
{
  "type": "tool",
  "status": "ok",
  "server": "github",
  "tool": "search_code",
  "latencyMs": 523,
  "requestBytes": 45,
  "responseBytes": 12456,
  "timestamp": "2025-01-15T10:30:01Z"
}
```

---

## Cost Calculation

### Formula

```
cost = (inputTokens * promptPrice / 1_000_000)
     + (outputTokens * completionPrice / 1_000_000)
     + (cacheReadTokens * cacheReadPrice / 1_000_000)
     + (cacheWriteTokens * cacheWritePrice / 1_000_000)
```

### Example

For GPT-4o with:
- 10,000 input tokens
- 2,000 output tokens
- Prices: $2.50/1M input, $10.00/1M output

```
cost = (10000 * 2.50 / 1000000) + (2000 * 10.00 / 1000000)
     = 0.025 + 0.02
     = $0.045
```

---

## Current Pricing (2025)

### OpenAI

| Model | Input | Output |
|-------|-------|--------|
| gpt-4o | $2.50/1M | $10.00/1M |
| gpt-4o-mini | $0.15/1M | $0.60/1M |

### Anthropic

| Model | Input | Output | Cache Read | Cache Write |
|-------|-------|--------|------------|-------------|
| claude-sonnet-4 | $3.00/1M | $15.00/1M | $0.30/1M | $3.75/1M |
| claude-3-haiku | $0.25/1M | $1.25/1M | $0.03/1M | $0.30/1M |

### Google

| Model | Input | Output |
|-------|-------|--------|
| gemini-1.5-pro | $1.25/1M | $5.00/1M |
| gemini-1.5-flash | $0.075/1M | $0.30/1M |

---

## Telemetry Metrics

| Metric | Description |
|--------|-------------|
| `ai_agent_llm_input_tokens_total` | Total input tokens |
| `ai_agent_llm_output_tokens_total` | Total output tokens |
| `ai_agent_llm_cache_read_tokens_total` | Cache read tokens |
| `ai_agent_llm_cache_write_tokens_total` | Cache write tokens |
| `ai_agent_llm_cost_total` | Total cost in USD |

---

## Analyzing Costs

### By Model

```bash
cat accounting.jsonl | jq -s '
  group_by(.model) |
  map({
    model: .[0].model,
    totalCost: (map(.cost // 0) | add),
    requests: length
  })
'
```

### By Agent

```bash
cat accounting.jsonl | jq -s '
  group_by(.agentPath) |
  map({
    agent: .[0].agentPath,
    totalCost: (map(.cost // 0) | add)
  })
'
```

### Daily Summary

```bash
cat accounting.jsonl | jq -s '
  group_by(.timestamp[:10]) |
  map({
    date: .[0].timestamp[:10],
    cost: (map(.cost // 0) | add)
  })
'
```

---

## Best Practices

### Monitor Regularly

Set up alerts for:
- Daily cost exceeds threshold
- Single request cost anomalies
- Token usage spikes

### Optimize Caching

Cache responses to reduce costs:

```yaml
---
cache: 1h  # Agent response caching
---
```

### Use Appropriate Models

- Complex reasoning: gpt-4o, claude-sonnet
- Simple tasks: gpt-4o-mini, claude-haiku
- Cost-sensitive: Use fallback chains

---

## See Also

- [Configuration](Configuration) - Overview
- [Caching](Configuration-Caching) - Reduce costs with caching
- [docs/specs/pricing.md](../docs/specs/pricing.md) - Technical spec
