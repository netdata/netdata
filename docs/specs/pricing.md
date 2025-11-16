# Pricing System

## TL;DR
Token-based cost calculation with provider/model pricing tables, per-1k or per-1m units, cache read/write pricing, and provider-reported cost override support.

## Source Files
- `src/config.ts` - Pricing schema definition (lines 213-228)
- `src/ai-agent.ts` - Cost computation logic (lines 1777-1806)
- `src/llm-providers/openrouter.ts` - Provider-reported costs
- `src/session-tree.ts` - Cost aggregation
- `src/telemetry/index.ts` - Cost telemetry

## Configuration Schema

**Location**: `src/config.ts:213-228`

```typescript
const PricingSchema = z.record(
  z.string(),  // provider name
  z.record(
    z.string(),  // model name
    z.object({
      unit: z.enum(['per_1k', 'per_1m']).optional(),
      currency: z.literal('USD').optional(),
      prompt: z.number().nonnegative().optional(),
      completion: z.number().nonnegative().optional(),
      cacheRead: z.number().nonnegative().optional(),
      cacheWrite: z.number().nonnegative().optional(),
    })
  )
);
```

## Example Configuration

```json
{
  "pricing": {
    "anthropic": {
      "claude-sonnet-4-20250514": {
        "unit": "per_1m",
        "currency": "USD",
        "prompt": 3.0,
        "completion": 15.0,
        "cacheRead": 0.30,
        "cacheWrite": 3.75
      },
      "claude-3-5-haiku-20241022": {
        "unit": "per_1m",
        "prompt": 0.80,
        "completion": 4.0,
        "cacheRead": 0.08,
        "cacheWrite": 1.0
      }
    },
    "openai": {
      "gpt-4o": {
        "unit": "per_1m",
        "prompt": 2.50,
        "completion": 10.0
      },
      "gpt-4o-mini": {
        "unit": "per_1m",
        "prompt": 0.15,
        "completion": 0.60
      }
    },
    "openrouter": {
      "anthropic/claude-3.5-sonnet": {
        "unit": "per_1m",
        "prompt": 3.0,
        "completion": 15.0
      }
    }
  }
}
```

## Cost Computation

**Location**: `src/ai-agent.ts:1777-1793`

```typescript
const computeCost = (): { costUsd?: number } => {
  try {
    const pricing = config.pricing ?? {};
    const effectiveProvider = metadata?.actualProvider ?? provider;
    const effectiveModel = metadata?.actualModel ?? model;

    const provTable = pricing[effectiveProvider];
    const modelTable = provTable?.[effectiveModel];
    if (modelTable === undefined) return {};

    // Unit conversion
    const denom = modelTable.unit === 'per_1k' ? 1000 : 1_000_000;

    // Extract prices (default 0)
    const pIn = modelTable.prompt ?? 0;
    const pOut = modelTable.completion ?? 0;
    const pRead = modelTable.cacheRead ?? 0;
    const pWrite = modelTable.cacheWrite ?? 0;

    // Extract token counts
    const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;
    const w = tokens.cacheWriteInputTokens ?? 0;

    // Calculate total cost
    const cost = (pIn * tokens.inputTokens +
                  pOut * tokens.outputTokens +
                  pRead * r +
                  pWrite * w) / denom;

    return { costUsd: Number.isFinite(cost) ? cost : undefined };
  } catch { return {}; }
};
```

### Formula
```
Cost = (prompt_price × input_tokens +
        completion_price × output_tokens +
        cache_read_price × cache_read_tokens +
        cache_write_price × cache_write_tokens) / unit_divisor
```

Where:
- `unit_divisor` = 1000 for `per_1k`, 1,000,000 for `per_1m`
- All prices default to 0 if not specified
- Cache tokens default to 0 if not reported

## Cost Priority

**Location**: `src/ai-agent.ts:1806`

```typescript
costUsd: metadata?.reportedCostUsd ?? computed.costUsd
```

Priority:
1. **Provider-reported cost**: From provider metadata (e.g., OpenRouter)
2. **Computed cost**: From pricing table
3. **Undefined**: No cost information available

## Provider-Reported Costs

### OpenRouter
**Location**: `src/llm-providers/openrouter.ts:338-346`

```typescript
private extractUsageCosts(usage?: Record<string, unknown>): {
  costUsd?: number;
  upstreamCostUsd?: number
} {
  const cost = usage?.cost;                    // Router cost
  const upstream = usage?.upstream_inference_cost;  // Upstream provider cost
  return { costUsd: cost, upstreamCostUsd: upstream };
}
```

OpenRouter provides:
- `cost`: Total cost charged
- `upstream_inference_cost`: Actual provider cost

### Effective Provider/Model Resolution

For routers (OpenRouter), actual provider/model may differ from requested:

```typescript
const effectiveProvider = metadata?.actualProvider ?? provider;
const effectiveModel = metadata?.actualModel ?? model;
```

This ensures pricing lookup uses the real provider/model.

## Cost Aggregation

### Per-Session Totals
**Location**: `src/session-tree.ts:402-440`

```typescript
let costUsd = 0;
for (const entry of accounting) {
  if (entry.type === 'llm') {
    const c = entry.costUsd;
    if (typeof c === 'number') costUsd += c;
  }
}
const normalizedCost = Number(costUsd.toFixed(4));
return {
  costUsd: Number.isFinite(normalizedCost) ? normalizedCost : undefined,
};
```

- Sums all LLM entry costs
- Rounds to 4 decimal places
- Returns undefined if not finite

### Telemetry Recording
**Location**: `src/telemetry/index.ts`

```typescript
// Metric: ai_agent_llm_cost_usd_total
if (record.costUsd !== undefined) {
  llmCostCounter.add(record.costUsd, labels);
}
```

Cost tracked per provider/model with labels.

## Accounting Entry Structure

```typescript
interface AccountingEntry {
  type: 'llm' | 'tool';
  timestamp: number;
  status: 'ok' | 'failed';
  latency: number;
  provider: string;
  model: string;
  actualProvider?: string;  // From metadata
  actualModel?: string;     // From metadata
  costUsd?: number;         // Final computed cost
  upstreamInferenceCostUsd?: number;  // Router upstream cost
  tokens: {
    inputTokens: number;
    outputTokens: number;
    totalTokens: number;
    cacheReadInputTokens?: number;
    cacheWriteInputTokens?: number;
    cachedTokens?: number;  // Legacy
  };
  // ... other fields
}
```

## Token Counting

### Cache Token Variants

```typescript
// Cache read tokens (priority order)
const r = tokens.cacheReadInputTokens ?? tokens.cachedTokens ?? 0;

// Cache write tokens
const w = tokens.cacheWriteInputTokens ?? 0;
```

Supports both modern and legacy cache token field names.

### Total Token Computation

```typescript
const totalWithCache =
  tokens.inputTokens +
  tokens.outputTokens +
  r + w;
```

Total includes cache read/write for comprehensive cost accounting.

## Display Formatting

### Session Summary
**Location**: `src/session-tree.ts:289`

```
cost=$${(totals.costUsd ?? 0).toFixed(2)}
```

Formatted as USD with 2 decimal places.

### Headend Summary
**Location**: `src/headends/openai-completions-headend.ts:397`

```typescript
if (totals.costUsd > 0) parts.push(`cost $${totals.costUsd.toFixed(4)}`);
```

Formatted as USD with 4 decimal places (more precision for APIs).

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `pricing[provider][model].unit` | Divisor: 1000 or 1000000 |
| `pricing[provider][model].prompt` | Price per input token |
| `pricing[provider][model].completion` | Price per output token |
| `pricing[provider][model].cacheRead` | Price per cache hit token |
| `pricing[provider][model].cacheWrite` | Price per cache write token |
| `pricing[provider][model].currency` | Currency type (USD only) |

## Missing Price Handling

When pricing table is missing:
- **Provider not in table**: Cost undefined
- **Model not in table**: Cost undefined
- **Price field missing**: Defaults to 0
- **Unit missing**: Defaults to per_1m (1,000,000)

```typescript
const denom = modelTable.unit === 'per_1k' ? 1000 : 1_000_000;
```

## Error Handling

Cost computation wrapped in try-catch:

```typescript
const computeCost = (): { costUsd?: number } => {
  try {
    // ... computation
    return { costUsd: Number.isFinite(cost) ? cost : undefined };
  } catch { return {}; }
};
```

Silent failure returns empty object, allowing:
- Provider metadata to take precedence
- Graceful degradation when pricing unavailable
- No cost tracking without errors

## Business Logic Coverage (Verified 2025-11-16)

- **Pricing gap warnings**: When a provider/model lacks pricing and no cost metadata is included, the CLI logs `agent:pricing` warnings listing missing entries so operators can update the table (`src/ai-agent.ts:1037-1100`).
- **Cache token fallbacks**: Cache costs pull from `cacheReadInputTokens` / `cacheWriteInputTokens` but fall back to `cachedTokens` when providers report only pooled cache values, ensuring caching charges remain consistent (`src/ai-agent.ts:1785-1805`).
- **Router overrides**: Router metadata (actual provider/model and cost) overrides config pricing, preventing double-counting when routers like OpenRouter already report upstream costs (`src/llm-providers/openrouter.ts:233-370`).
- **Rounded totals**: Session totals are rounded to 4 decimals (e.g., `$0.1234`) before telemetry/logging to avoid floating-point drift across long sessions (`src/session-tree.ts:402-440`).

## Invariants

1. **USD only**: Currency always USD
2. **Non-negative**: All prices ≥ 0
3. **Finite check**: Undefined if not finite
4. **Priority**: Provider-reported > computed
5. **Rounding**: 4 decimal precision for totals
6. **Default unit**: per_1m if not specified
7. **Default price**: 0 if field missing

## Test Coverage

**Phase 1**:
- Schema validation
- Basic cost computation
- Unit conversion (per_1k vs per_1m)
- Cache token pricing
- Provider-reported cost priority

**Gaps**:
- Multi-currency support (future)
- Pricing table hot-reload
- Historical pricing (versioning)
- Cost alerts/budgets
- Batch pricing discounts

## Troubleshooting

### No cost appearing
- Check pricing table exists for provider/model
- Verify provider/model names match exactly
- Check actualProvider/actualModel for routers
- Review token counts are populated

### Incorrect costs
- Verify unit (per_1k vs per_1m)
- Check cache token fields populated
- Compare with provider-reported costs
- Review actualProvider resolution

### Router cost mismatch
- OpenRouter reports actual cost
- Pricing table may have different rates
- Provider-reported takes priority
- Check upstreamInferenceCostUsd for details

### High costs
- Verify cache pricing (write often expensive)
- Check completion vs prompt ratio
- Review token efficiency
- Monitor per-model breakdown
