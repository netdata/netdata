# OpenRouter LLM Provider

## TL;DR
OpenRouter aggregator provider with provider routing metadata capture, reasoning effort/budget support, and cost tracking via @openrouter/ai-sdk-provider.

## Source Files
- `src/llm-providers/openrouter.ts` - Full implementation (463 lines)
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `@openrouter/ai-sdk-provider` - External AI SDK provider

## Provider Identity
- **Name**: `openrouter`
- **Kind**: LLM Provider
- **SDK**: @openrouter/ai-sdk-provider
- **Host**: `openrouter.ai`

## Construction

**Location**: `src/llm-providers/openrouter.ts:16-44`

```typescript
constructor(config: ProviderConfig, tracedFetch?) {
  super({
    formatPolicy: {
      allowed: config.stringSchemaFormatsAllowed,
      denied: config.stringSchemaFormatsDenied
    },
    reasoningDefaults: {
      minimal: 'minimal',
      low: 'low',
      medium: 'medium',
      high: 'high',
    },
    reasoningLimits: { min: 1024, max: 32_000 }
  });
  this.config = config;
  const prov = createOpenRouter({
    apiKey: config.apiKey,
    fetch: tracedFetch,
    headers: {
      'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
      'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent',
      'User-Agent': 'ai-agent/1.0',
      ...(config.headers ?? {}),
    },
  });
  this.provider = (model) => prov(model);
}
```

## Default Headers

**Location**: `src/llm-providers/openrouter.ts:46-54`

```typescript
prepareFetch(details): { headers?: Record<string, string> } | undefined {
  if (!details.url.includes(OpenRouterProvider.HOST)) return undefined;
  return {
    headers: {
      'HTTP-Referer': process.env.OPENROUTER_REFERER ?? 'https://ai-agent.local',
      'X-OpenRouter-Title': process.env.OPENROUTER_TITLE ?? 'ai-agent',
      'User-Agent': 'ai-agent/1.0',
    }
  };
}
```

Only applied to requests to `openrouter.ai` host.

## Reasoning Support

**Location**: `src/llm-providers/openrouter.ts:104-112`

```typescript
if (request.reasoningValue !== undefined) {
  if (request.reasoningValue === null) {
    delete routerOptions.reasoning;
  } else if (typeof request.reasoningValue === 'string') {
    routerOptions.reasoning = { effort: request.reasoningValue };
  } else if (typeof request.reasoningValue === 'number') {
    routerOptions.reasoning = { max_tokens: Math.trunc(request.reasoningValue) };
  }
}
```

Three modes:
- `null`: Disable reasoning (delete property)
- `string`: Effort level (minimal, low, medium, high)
- `number`: Token budget (sent as `max_tokens` after truncation)

Explicit numeric budgets are not clamped; they are only truncated to integers so harness scenarios can push higher budgets when OpenRouter models allow it. The `[1024, 32_000]` `reasoningLimits` configured on the base class apply only when a reasoning **level** is converted into a default budget.

## Provider Options

### Base Options
**Location**: `src/llm-providers/openrouter.ts:90-96`

```typescript
const openaiOpts = {
  toolChoice?: 'auto' | 'required',
  maxTokens?: number,
  frequencyPenalty?: number
};

const baseProviderOptions = {
  openrouter: { usage: { include: true } },
  openai: openaiOpts
};
```

### Custom Options
**Location**: `src/llm-providers/openrouter.ts:297-308`

```typescript
getProviderOptions(): Record<string, unknown> {
  const raw = this.config.custom;
  if (this.isPlainObject(raw)) {
    const providerOptions = raw.providerOptions;
    if (this.isPlainObject(providerOptions)) return providerOptions;
  }
  return {};
}
```

### Router Provider Config
**Location**: `src/llm-providers/openrouter.ts:284-293`

```typescript
getRouterProviderConfig(): Record<string, unknown> {
  const raw = this.config.custom;
  if (this.isPlainObject(raw)) {
    const providerConfig = raw.provider;
    if (this.isPlainObject(providerConfig)) return providerConfig;
  }
  return {};
}
```

Merged into `openrouter.provider` for backend preferences.

## Metadata Capture

### Response Collector
**Location**: `src/llm-providers/openrouter.ts:56-60`

```typescript
getResponseMetadataCollector() {
  return async ({ url, response }) => {
    return await this.handleMetadataCapture(url, response);
  };
}
```

### Capture Handler
**Location**: `src/llm-providers/openrouter.ts:233-263`

```typescript
handleMetadataCapture(url, response): Promise<ProviderTurnMetadata | undefined> {
  if (!url.includes(OpenRouterProvider.HOST)) return undefined;

  if (contentType.includes('application/json')) {
    const parsed = await this.parseJsonResponse(response.clone());
    metadata = this.metadataFromRecord(parsed);
  } else if (contentType.includes('text/event-stream')) {
    const partial = await this.parseOpenRouterSseStream(response.clone());
    metadata = partial;
  }

  if (metadata !== undefined) {
    this.enqueueProviderMetadata(metadata);
  }
}
```

Two parsing modes:
- JSON: Direct parse
- SSE: Line-by-line stream parsing

### Metadata Fields
**Location**: `src/llm-providers/openrouter.ts:265-277`

```typescript
metadataFromRecord(record): ProviderTurnMetadata {
  return {
    actualProvider: routing.provider,  // Actual backend used
    actualModel: routing.model,        // Actual model used
    reportedCostUsd: costs.costUsd,    // Total cost
    upstreamCostUsd: costs.upstreamCostUsd,  // Backend cost
    cacheWriteInputTokens: cacheWrite  // Cache tokens
  };
}

## Business Logic Coverage (Verified 2025-11-16)

- **Attribution headers**: Provider injects `HTTP-Referer`, `X-OpenRouter-Title`, and a deterministic `User-Agent`, unless overridden, to comply with OpenRouter attribution policies (`src/llm-providers/openrouter.ts:16-60`).
- **Router preferences**: `config.custom.provider` merges into `openrouter.provider` so operators can bias routing toward specific upstream vendors without editing code (`src/llm-providers/openrouter.ts:284-310`).
- **Metadata parsing**: SSE streams are parsed line-by-line to recover routing + cost metadata even when OpenRouter doesn’t send final JSON, ensuring accounting always knows which upstream provider ran the request (`src/llm-providers/openrouter.ts:233-370`).
- **Reasoning flexibility**: `reasoningValue` accepts either effort labels (string) or explicit token budgets (number); budgets are truncated but never clamped so higher experimental limits can be forwarded while still allowing CLI/frontmatter to disable reasoning via `null` (`src/llm-providers/openrouter.ts:90-125`).
```

## Routing Extraction

**Location**: `src/llm-providers/openrouter.ts:380-399`

```typescript
extractOpenRouterRouting(record): { provider?: string; model?: string } {
  // Try top-level or nested data
  const target = this.isPlainObject(record.data) ? record.data : record;
  let provider = target.provider;
  let model = target.model;

  // Also check choices array
  if (choices && choices.length > 0) {
    const choice = choices[0];
    if (choice.provider) provider = choice.provider;
    if (choice.model) model = choice.model;
  }
  return { provider, model };
}
```

Multiple extraction paths for compatibility.

## Cost Extraction

**Location**: `src/llm-providers/openrouter.ts:338-347`

```typescript
extractUsageCosts(usage): { costUsd?: number; upstreamCostUsd?: number } {
  const cost = this.toNumber(usage.cost);
  const details = usage.cost_details;
  const upstream = this.toNumber(details?.upstream_inference_cost);
  return { costUsd: cost, upstreamCostUsd: upstream };
}
```

## Cache Token Extraction

**Location**: `src/llm-providers/openrouter.ts:349-378`

Checks multiple key patterns:
- `cacheWriteInputTokens`
- `cacheCreationInputTokens`
- `cache_creation_input_tokens`
- `cache_write_input_tokens`
- Nested: `cacheCreation.ephemeral_5m_input_tokens`
- Nested: `cache_creation.ephemeral_5m_input_tokens`

## SSE Stream Parsing

**Location**: `src/llm-providers/openrouter.ts:420-451`

```typescript
parseOpenRouterSseStream(response): Promise<ProviderTurnMetadata | undefined> {
  const text = await response.text();
  const lines = text.split('\n');
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line.startsWith('data:')) continue;
    const payload = line.slice(5).trim();
    if (payload === '[DONE]') continue;
    const parsed = this.parseJsonString(payload);
    // Extract routing, usage, costs from each chunk
    metadata = this.mergeProviderMetadata(metadata, {
      actualProvider, actualModel,
      reportedCostUsd, upstreamCostUsd,
      cacheWriteInputTokens
    });
  }
}
```

Accumulates metadata across SSE chunks.

## Retry Directive

**Location**: `src/llm-providers/openrouter.ts:62-75`

```typescript
buildRetryDirective(request, status): TurnRetryDirective | undefined {
  if (status.type === 'rate_limit') {
    return {
      action: 'retry',
      backoffMs: status.retryAfterMs,
      logMessage: `OpenRouter rate limit; backing off...`,
      sources: status.sources,
    };
  }
  return super.buildRetryDirective(request, status);
}
```

Custom rate limit handling with provider hint; rate-limit retries are logged but not sent to the model.

## Response Conversion

**Location**: `src/llm-providers/openrouter.ts:125-206`

Handles:
1. **Tool role with array content**: Split bundled tool results
2. **Assistant role with tool-results**: Extract embedded results (defensive)
3. **Standard messages**: Parse via generic method

Tool result extraction:
```typescript
if (outObj.type === 'text' && typeof outObj.value === 'string')
  text = outObj.value;
else if (outObj.type === 'json')
  text = JSON.stringify(outObj.value);
else if (typeof outObj === 'string')
  text = outObj;
```

## Metadata Extraction

**Location**: `src/llm-providers/openrouter.ts:208-233`

```typescript
protected override extractTurnMetadata(request, context): ProviderTurnMetadata | undefined {
  let metadata = this.mergeProviderMetadata(
    this.deriveContextMetadata(request, context),
    this.consumeQueuedProviderMetadata()
  );

  const usageRecord = this.extractUsageRecord(context.usage);
  const cacheWrite = this.extractCacheWriteTokens(usageRecord);
  if (cacheWrite !== undefined && metadata?.cacheWriteInputTokens === undefined) {
    metadata = this.mergeProviderMetadata(metadata, { cacheWriteInputTokens: cacheWrite });
  }
  const costs = this.extractUsageCosts(usageRecord);
  if (costs.costUsd !== undefined && metadata?.reportedCostUsd === undefined) {
    metadata = this.mergeProviderMetadata(metadata, { reportedCostUsd: costs.costUsd });
  }
  if (costs.upstreamCostUsd !== undefined && metadata?.upstreamCostUsd === undefined) {
    metadata = this.mergeProviderMetadata(metadata, { upstreamCostUsd: costs.upstreamCostUsd });
  }
  return metadata;
}
```

Queued metadata captured from JSON/SSE responses remains authoritative. Cache write tokens and cost fields from the response `usage` payload only backfill values that were missing so streaming metadata is never overwritten.

## Turn Execution

**Location**: `src/llm-providers/openrouter.ts:77-122`

Flow:
1. Create model instance
2. Filter tools for final turn
3. Convert tools and messages
4. Build final turn messages
5. Resolve tool choice
6. Build OpenAI options (toolChoice, maxTokens, frequencyPenalty)
7. Build base provider options (openrouter.usage.include, openai opts)
8. Deep merge custom provider options
9. Merge router provider config
10. Apply reasoning settings
11. Execute via base class (streaming or non-streaming)

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `apiKey` | OpenRouter API key |
| `stringSchemaFormatsAllowed` | Schema format filtering |
| `stringSchemaFormatsDenied` | Schema format blocking |
| `headers` | Custom HTTP headers |
| `custom.providerOptions` | SDK provider options |
| `custom.provider` | Router backend preferences |
| `reasoningValue` | Effort string or token budget |
| `maxOutputTokens` | Response length limit |
| `repeatPenalty` | Frequency penalty |

## Environment Variables

| Variable | Default | Effect |
|----------|---------|--------|
| `OPENROUTER_REFERER` | `https://ai-agent.local` | HTTP-Referer header |
| `OPENROUTER_TITLE` | `ai-agent` | X-OpenRouter-Title header |

## Telemetry

**Enhanced metadata**:
- `actualProvider`: Backend provider used
- `actualModel`: Backend model used
- `reportedCostUsd`: Total cost charged
- `upstreamCostUsd`: Upstream inference cost
- `cacheWriteInputTokens`: Cache creation tokens

**Via base class**:
- Token usage (input, output)
- Latency
- Stop reason
- Tool calls

## Logging

**Warnings**:
- Metadata capture failures
- Provider cleanup failures

## Events

**Handled**:
- Tool calls
- Streaming chunks
- Response completion
- Metadata capture (JSON and SSE)

## Invariants

1. **Host check**: Metadata capture only for openrouter.ai
2. **Usage inclusion**: Always requests usage data
3. **Reasoning modes**: String (effort) or number (tokens)
4. **Deep merge**: Provider options merged recursively
5. **Response cloning**: Clones response for metadata parsing
6. **Queued metadata**: Captured metadata queued for turn extraction

## Test Coverage

**Phase 1**:
- Provider construction
- Header injection
- Reasoning configuration
- Metadata extraction
- Cost parsing
- Cache token extraction
- SSE stream parsing

**Gaps**:
- Multi-provider routing scenarios
- Rate limit retry behavior
- Cache write edge cases
- Upstream cost validation
- Error recovery in metadata capture

## Troubleshooting

### Metadata not captured
- Check URL includes openrouter.ai
- Verify content-type header
- Review parsing errors in logs

### Incorrect costs
- Check usage.cost field exists
- Verify cost_details.upstream_inference_cost
- Review toNumber conversion

### Reasoning not applied
- Check reasoningValue type (string or number)
- Verify within limits (1024-32000)
- Review openrouter.reasoning in options

### Wrong backend used
- Check custom.provider configuration
- Verify router preferences
- Review actualProvider in metadata

### Headers missing
- Check environment variables
- Verify prepareFetch called
- Review config.headers merge
