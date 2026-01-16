# Pricing Configuration

Configure token pricing for cost tracking.

---

## Table of Contents

- [Overview](#overview) - Cost tracking purpose
- [Price Configuration](#price-configuration) - Setting token prices
- [Price Fields](#price-fields) - Token types and pricing
- [Accounting Output](#accounting-output) - Cost recording
- [Cost Calculation](#cost-calculation) - Formula and examples
- [Current Pricing](#current-pricing) - 2025 reference prices
- [Configuration Reference](#configuration-reference) - All pricing options
- [Analyzing Costs](#analyzing-costs) - Cost analysis queries
- [Telemetry](#telemetry) - Cost metrics
- [Best Practices](#best-practices) - Cost optimization
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent tracks costs by:

1. **Counting tokens**: Input, output, and cached tokens per LLM call
2. **Applying prices**: Using configured rates per provider/model
3. **Recording entries**: Writing to accounting file for analysis

Cost tracking enables:
- Budget monitoring
- Usage optimization
- Chargeback/allocation
- ROI analysis

---

## Price Configuration

Configure prices in `.ai-agent.json`:

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
      },
      "claude-3-haiku-20240307": {
        "unit": "per_1m",
        "prompt": 0.25,
        "completion": 1.25,
        "cacheRead": 0.03,
        "cacheWrite": 0.30
      }
    },
    "google": {
      "gemini-1.5-pro": {
        "unit": "per_1m",
        "prompt": 1.25,
        "completion": 5.00
      },
      "gemini-1.5-flash": {
        "unit": "per_1m",
        "prompt": 0.075,
        "completion": 0.30
      }
    }
  }
}
```

---

## Price Fields

| Field | Type | Description |
|-------|------|-------------|
| `unit` | `string` | Price unit: `per_1m` (per million tokens) |
| `prompt` | `number` | Input token price (USD) |
| `completion` | `number` | Output token price (USD) |
| `cacheRead` | `number` | Cached input token read price (USD) |
| `cacheWrite` | `number` | Cached input token write price (USD) |

### Token Types

| Token Type | Description | Pricing |
|------------|-------------|---------|
| Input | Tokens in the prompt | `prompt` rate |
| Output | Tokens in the response | `completion` rate |
| Cache Read | Tokens read from Anthropic cache | `cacheRead` rate |
| Cache Write | Tokens written to Anthropic cache | `cacheWrite` rate |

---

## Accounting Output

### Enable Accounting File

In config:

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

### LLM Entry Format

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
  "timestamp": "2025-01-15T10:30:00Z",
  "agentPath": "agents/research.ai",
  "sessionId": "abc123"
}
```

### Tool Entry Format

```json
{
  "type": "tool",
  "status": "ok",
  "server": "github",
  "tool": "search_code",
  "latencyMs": 523,
  "requestBytes": 45,
  "responseBytes": 12456,
  "timestamp": "2025-01-15T10:30:01Z",
  "agentPath": "agents/research.ai",
  "sessionId": "abc123"
}
```

### Entry Fields Reference

| Field | Type | Description |
|-------|------|-------------|
| `type` | `string` | Entry type: `llm` or `tool` |
| `status` | `string` | Result status: `ok` or `error` |
| `provider` | `string` | LLM provider name |
| `model` | `string` | Model identifier |
| `inputTokens` | `number` | Input token count |
| `outputTokens` | `number` | Output token count |
| `cacheReadTokens` | `number` | Cache read token count |
| `cacheWriteTokens` | `number` | Cache write token count |
| `cost` | `number` | Calculated cost in USD |
| `latencyMs` | `number` | Request duration |
| `timestamp` | `string` | ISO 8601 timestamp |
| `agentPath` | `string` | Agent file path |
| `sessionId` | `string` | Session identifier |
| `server` | `string` | MCP server name (tool entries) |
| `tool` | `string` | Tool name (tool entries) |
| `requestBytes` | `number` | Request size (tool entries) |
| `responseBytes` | `number` | Response size (tool entries) |

---

## Cost Calculation

### Formula

```
cost = (inputTokens * promptPrice / 1,000,000)
     + (outputTokens * completionPrice / 1,000,000)
     + (cacheReadTokens * cacheReadPrice / 1,000,000)
     + (cacheWriteTokens * cacheWritePrice / 1,000,000)
```

### Example: GPT-4o

Prices: $2.50/1M input, $10.00/1M output

| Tokens | Calculation | Cost |
|--------|-------------|------|
| 10,000 input | 10,000 * 2.50 / 1,000,000 | $0.025 |
| 2,000 output | 2,000 * 10.00 / 1,000,000 | $0.020 |
| **Total** | | **$0.045** |

### Example: Claude Sonnet with Cache

Prices: $3.00/1M input, $15.00/1M output, $0.30/1M cache read, $3.75/1M cache write

| Tokens | Calculation | Cost |
|--------|-------------|------|
| 5,000 input | 5,000 * 3.00 / 1,000,000 | $0.015 |
| 1,000 output | 1,000 * 15.00 / 1,000,000 | $0.015 |
| 8,000 cache read | 8,000 * 0.30 / 1,000,000 | $0.0024 |
| 5,000 cache write | 5,000 * 3.75 / 1,000,000 | $0.01875 |
| **Total** | | **$0.05115** |

---

## Current Pricing

Reference prices as of 2025. Update your config when prices change.

### OpenAI

| Model | Input ($/1M) | Output ($/1M) |
|-------|--------------|---------------|
| gpt-4o | $2.50 | $10.00 |
| gpt-4o-mini | $0.15 | $0.60 |
| o1 | $15.00 | $60.00 |
| o1-mini | $3.00 | $12.00 |

### Anthropic

| Model | Input ($/1M) | Output ($/1M) | Cache Read ($/1M) | Cache Write ($/1M) |
|-------|--------------|---------------|-------------------|--------------------|
| claude-sonnet-4 | $3.00 | $15.00 | $0.30 | $3.75 |
| claude-3-haiku | $0.25 | $1.25 | $0.03 | $0.30 |
| claude-3-opus | $15.00 | $75.00 | $1.50 | $18.75 |

### Google

| Model | Input ($/1M) | Output ($/1M) |
|-------|--------------|---------------|
| gemini-1.5-pro | $1.25 | $5.00 |
| gemini-1.5-flash | $0.075 | $0.30 |
| gemini-2.0-flash | $0.10 | $0.40 |

---

## Configuration Reference

### Pricing Schema

```json
{
  "pricing": {
    "<provider>": {
      "<model>": {
        "unit": "per_1m",
        "prompt": "number",
        "completion": "number",
        "cacheRead": "number",
        "cacheWrite": "number"
      }
    }
  }
}
```

### Accounting Schema

```json
{
  "accounting": {
    "file": "string"
  }
}
```

### All Pricing Properties

| Location | Property | Type | Default | Description |
|----------|----------|------|---------|-------------|
| Pricing | `unit` | `string` | `"per_1m"` | Price unit |
| Pricing | `prompt` | `number` | `0` | Input token price |
| Pricing | `completion` | `number` | `0` | Output token price |
| Pricing | `cacheRead` | `number` | `0` | Cache read price |
| Pricing | `cacheWrite` | `number` | `0` | Cache write price |
| Accounting | `file` | `string` | - | Accounting file path |

---

## Analyzing Costs

### By Model

```bash
cat accounting.jsonl | jq -s '
  [.[] | select(.type == "llm")] |
  group_by(.model) |
  map({
    model: .[0].model,
    totalCost: (map(.cost // 0) | add),
    requests: length,
    avgCost: ((map(.cost // 0) | add) / length)
  }) |
  sort_by(-.totalCost)
'
```

### By Agent

```bash
cat accounting.jsonl | jq -s '
  [.[] | select(.type == "llm")] |
  group_by(.agentPath) |
  map({
    agent: .[0].agentPath,
    totalCost: (map(.cost // 0) | add),
    requests: length
  }) |
  sort_by(-.totalCost)
'
```

### Daily Summary

```bash
cat accounting.jsonl | jq -s '
  [.[] | select(.type == "llm")] |
  group_by(.timestamp[:10]) |
  map({
    date: .[0].timestamp[:10],
    cost: (map(.cost // 0) | add),
    requests: length
  }) |
  sort_by(.date)
'
```

### Token Usage Summary

```bash
cat accounting.jsonl | jq -s '
  [.[] | select(.type == "llm")] |
  {
    totalInputTokens: (map(.inputTokens // 0) | add),
    totalOutputTokens: (map(.outputTokens // 0) | add),
    totalCacheReadTokens: (map(.cacheReadTokens // 0) | add),
    totalCacheWriteTokens: (map(.cacheWriteTokens // 0) | add),
    totalCost: (map(.cost // 0) | add)
  }
'
```

### Top Expensive Sessions

```bash
cat accounting.jsonl | jq -s '
  [.[] | select(.type == "llm")] |
  group_by(.sessionId) |
  map({
    sessionId: .[0].sessionId,
    agent: .[0].agentPath,
    cost: (map(.cost // 0) | add)
  }) |
  sort_by(-.cost) |
  .[0:10]
'
```

---

## Telemetry

Cost and token metrics are exported:

| Metric | Description |
|--------|-------------|
| `ai_agent_llm_input_tokens_total` | Total input tokens |
| `ai_agent_llm_output_tokens_total` | Total output tokens |
| `ai_agent_llm_cache_read_tokens_total` | Total cache read tokens |
| `ai_agent_llm_cache_write_tokens_total` | Total cache write tokens |
| `ai_agent_llm_cost_total` | Total cost in USD |

### Labels

| Label | Description |
|-------|-------------|
| `provider` | LLM provider |
| `model` | Model identifier |
| `agent` | Agent path |

### Example Queries

```promql
# Total cost by provider
sum(ai_agent_llm_cost_total) by (provider)

# Token usage rate
rate(ai_agent_llm_input_tokens_total[1h])

# Cache efficiency
sum(ai_agent_llm_cache_read_tokens_total) /
sum(ai_agent_llm_input_tokens_total)
```

---

## Best Practices

### Monitor Regularly

Set up alerts for:

| Condition | Action |
|-----------|--------|
| Daily cost > threshold | Review usage patterns |
| Single request cost > limit | Check for runaway sessions |
| Token usage spike | Investigate cause |

### Optimize Caching

Reduce costs with response caching:

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
cache: 1h
---
```

Anthropic cache savings can be significant for repeated prompts.

### Use Appropriate Models

| Task Type | Recommended Model | Cost Level |
|-----------|-------------------|------------|
| Simple classification | gpt-4o-mini | Low |
| Code generation | gpt-4o | Medium |
| Complex reasoning | claude-sonnet-4 | Medium |
| Research synthesis | o1 | High |

### Cost-Aware Model Selection

Use model fallbacks for cost optimization:

```yaml
---
models:
  - anthropic/claude-3-haiku-20240307  # Try cheap first
  - anthropic/claude-sonnet-4-20250514  # Fallback to expensive
---
```

### Budget Tracking

Track costs per agent/session:

```bash
# Set budget limits
export DAILY_BUDGET=100

# Check current spend
SPENT=$(cat accounting.jsonl | jq -s '[.[].cost // 0] | add')
echo "Today's spend: $SPENT"
```

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [Caching](Configuration-Caching) - Reduce costs with caching
- [Providers](Configuration-Providers) - Provider configuration
- [Parameters](Configuration-Parameters) - All parameters reference
