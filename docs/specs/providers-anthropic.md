# Anthropic LLM Provider

## TL;DR
Claude-specific provider with cache control, reasoning signatures, and thinking budget support via @ai-sdk/anthropic.

## Source Files
- `src/llm-providers/anthropic.ts` - Full implementation
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `@ai-sdk/anthropic` - External AI SDK provider

## Provider Identity
- **Name**: `anthropic`
- **Kind**: LLM Provider
- **SDK**: @ai-sdk/anthropic

## Construction

**Location**: `src/llm-providers/anthropic.ts:21-34`

```typescript
constructor(config: ProviderConfig, tracedFetch?) {
  super({
    formatPolicy: {
      allowed: config.stringSchemaFormatsAllowed,
      denied: config.stringSchemaFormatsDenied
    },
    reasoningLimits: { min: 1024, max: 128_000 }
  });
  this.config = config;
  const prov = createAnthropic({
    apiKey: config.apiKey,
    baseURL: config.baseUrl,
    fetch: tracedFetch
  });
  this.provider = (model) => prov(model);
}
```

## Reasoning Support

### Budget Configuration
**Location**: `src/llm-providers/anthropic.ts:111-124`

```typescript
if (reasoningValue === null) {
  // Explicitly disabled
} else if (typeof reasoningValue === 'number') {
  a.thinking = { type: 'enabled', budgetTokens: Math.trunc(reasoningValue) };
} else if (typeof reasoningValue === 'string') {
  const parsed = Number(reasoningValue);
  a.thinking = { type: 'enabled', budgetTokens: Math.trunc(parsed) };
}
```

Limits: min=1024, max=128,000 tokens

### Auto-Enable Streaming
**Location**: `src/llm-providers/anthropic.ts:138-149`

```typescript
shouldAutoEnableReasoningStream(level, options) {
  const reasoningActive = options?.reasoningActive || level !== undefined;
  if (!reasoningActive) return false;
  if (options?.streamRequested) return false;
  if (maxOutputTokens < 21_333) return false;
  return true;
}
```

Threshold: 21,333 tokens

### Signature Validation
**Location**: `src/llm-providers/anthropic.ts:158-200`

```typescript
shouldDisableReasoning(context) {
  if (currentTurn <= 1 || !expectSignature) {
    return { disable: false, normalized: conversation };
  }
  const { normalized, missing } = this.stripReasoningWithoutSignature(conversation);
  return { disable: missing, normalized };
}
```

Signature presence check:
```typescript
segmentHasSignature(segment) {
  const anthropic = segment.providerMetadata?.anthropic;
  return anthropic?.signature || anthropic?.redactedData;
}
```

## Cache Control

**Location**: `src/llm-providers/anthropic.ts:45-102`

### Strategy
1. Strip cache control from all messages first
2. Find last non-system-notice message
3. Apply ephemeral cache control to that message

### Implementation
```typescript
// Strip existing cache controls
for (const msg of extendedMessages) {
  delete msg.providerOptions?.anthropic?.cacheControl;
}

// Apply to cache target (last valid message)
const cacheTargetIndex = cachingMode === 'none' ? -1 : findCacheTargetIndex();
if (cacheTargetIndex !== -1) {
  lastMessage.providerOptions = {
    anthropic: {
      cacheControl: { type: 'ephemeral' }
    }
  };
}
```

### Caching Modes
- `full`: Apply cache control (default)
- `none`: Skip cache control

## Provider Options

**Location**: `src/llm-providers/anthropic.ts:104-149`

```typescript
const providerOptions = (() => {
  const base: Record<string, unknown> = { anthropic: {} };
  const a = base.anthropic as Record<string, unknown>;
  a.sendReasoning = request.sendReasoning ?? true;
  if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) {
    a.maxTokens = Math.trunc(request.maxOutputTokens);
  }
  if (request.reasoningValue !== undefined) {
    if (request.reasoningValue === null) {
      // Explicit disable – leave `thinking` undefined
    } else if (typeof request.reasoningValue === 'number' && Number.isFinite(request.reasoningValue)) {
      a.thinking = { type: 'enabled', budgetTokens: Math.trunc(request.reasoningValue) };
    } else if (typeof request.reasoningValue === 'string' && request.reasoningValue.trim().length > 0) {
      const parsed = Number(request.reasoningValue);
      if (Number.isFinite(parsed)) {
        a.thinking = { type: 'enabled', budgetTokens: Math.trunc(parsed) };
      } else {
        warn(`anthropic reasoning value '${request.reasoningValue}' is not a number; ignoring`);
      }
    }
  }
  return base;
})();
```

Fields:
- `sendReasoning`: Include reasoning content (default `true`).
- `maxTokens`: Added only when finite, truncated to an integer.
- `thinking`: Attached only when reasoning is active; numbers/strings are truncated but **not** clamped beyond whatever the caller supplies (the context guard enforces practical limits).

## Tool Choice Restrictions

**Location**: `src/llm-providers/anthropic.ts:151-156`

```typescript
shouldForceToolChoice(request) {
  if (request.reasoningLevel !== undefined) {
    return false;  // Cannot force tool choice with reasoning
  }
  return super.shouldForceToolChoice(request);
}
```

## Turn Execution

**Location**: `src/llm-providers/anthropic.ts:36-136`

Flow:
1. Create model instance
2. Filter tools for final turn
3. Convert tools and messages
4. Apply cache control
5. Build provider options
6. Execute via base class (streaming or non-streaming)

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `apiKey` | Anthropic API authentication |
| `baseUrl` | Custom API endpoint |
| `stringSchemaFormatsAllowed` | Schema format filtering |
| `stringSchemaFormatsDenied` | Schema format blocking |
| `reasoningValue` | Thinking budget tokens |
| `caching` | Cache control mode |
| `maxOutputTokens` | Response length limit |

## Telemetry

**Via base class**:
- Token usage (input, output, cache read/write)
- Latency
- Stop reason
- Reasoning content

## Logging

**Warnings**:
- Invalid reasoning value string
- Cache control application failures

## Events

**Handled**:
- Reasoning chunks (via streaming)
- Cache control updates
- Tool choice relaxation when reasoning active

## Retry Handling

**Location**: `src/llm-providers/anthropic.ts:203-216`

Rate limits trigger a provider-specific retry directive:

```typescript
if (status.type === 'rate_limit') {
  const wait = Number.isFinite(status.retryAfterMs) ? status.retryAfterMs : undefined;
  const remoteId = `${request.provider}:${request.model}`;
  return {
    action: 'retry',
    backoffMs: wait,
    logMessage: `Anthropic rate limit; waiting ${wait ?? 'briefly'} before retry.`,
    systemMessage: `System notice: Anthropic (${remoteId}) rate-limited the prior request...`
  };
}
```

This surfaces a user-facing `system notice` explaining the throttle and mirrors the log message so operators know why execution paused.

## Business Logic Coverage (Verified 2025-11-16)

- **Cache target selection**: Cache control is applied only to the last user message that isn’t an internal system notice so Anthropic’s ephemeral cache is used exactly once per turn (`src/llm-providers/anthropic.ts:45-102`).
- **Reasoning signature enforcement**: Turns after the first must include Anthropic signatures on reasoning segments; otherwise the provider strips the reasoning content to avoid leaking unsigned internal thoughts (`src/llm-providers/anthropic.ts:158-214`).
- **Reasoning + tool choice**: When reasoning is active the provider refuses to force `tool_choice='required'` because Anthropic disallows simultaneous tool forcing and thinking. When reasoning is disabled the base class behavior applies (`src/llm-providers/anthropic.ts:151-156`).
- **Reasoning value handling**: Manual `reasoningValue` inputs are only truncated; the provider intentionally does **not** clamp them, relying on the context guard/options resolver to reject negatives so high budgets can flow through for Claude Thinking models (`src/llm-providers/anthropic.ts:104-149`, `src/options-resolver.ts:120-210`).
- **Rate limit messaging**: Custom retry directives inject a `system notice` plus consistent log text so operators and end users know Anthropic throttled the turn without having to inspect raw logs (`src/llm-providers/anthropic.ts:203-216`).

## Invariants

1. **Cache control placement**: Only on last valid message
2. **Reasoning signatures**: Required for turn > 1 when expected
3. **Thinking budget**: Defaults derived from reasoning levels land in 1,024–128,000 tokens; explicit overrides are only truncated.
4. **Tool choice**: Disabled when reasoning active
5. **Stream threshold**: Auto-enable at 21,333+ tokens

## Test Coverage

**Phase 1**:
- Cache control application
- Reasoning budget parsing
- Signature validation
- Tool filtering
- Provider options building

**Gaps**:
- Cache hit rate measurement
- Reasoning signature edge cases
- Large reasoning budget performance
- Error recovery scenarios

## Troubleshooting

### Cache not applied
- Check caching mode setting
- Verify non-empty message list
- Check system notice filtering

### Reasoning disabled unexpectedly
- Check signature presence
- Verify turn number
- Check expectSignature setting

### Invalid reasoning value
- Check numeric format
- Verify within limits (1024-128000)
- Review parse warnings

### Tool choice ignored
- Check if reasoning active
- Verify shouldForceToolChoice logic
- Review request configuration

### API errors
- Check apiKey validity
- Verify baseUrl correct
- Review rate limits
