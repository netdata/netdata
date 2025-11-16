# Google LLM Provider

## TL;DR
Google Generative AI provider supporting thinking budgets and frequency penalty via @ai-sdk/google.

## Source Files
- `src/llm-providers/google.ts` - Full implementation (76 lines)
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `@ai-sdk/google` - External AI SDK provider

## Provider Identity
- **Name**: `google`
- **Kind**: LLM Provider
- **SDK**: @ai-sdk/google

## Construction

**Location**: `src/llm-providers/google.ts:15-28`

```typescript
constructor(config: ProviderConfig, tracedFetch?) {
  super({
    formatPolicy: {
      allowed: config.stringSchemaFormatsAllowed,
      denied: config.stringSchemaFormatsDenied
    },
    reasoningLimits: { min: 1024, max: 32_768 }
  });
  this.config = config;
  const prov = createGoogleGenerativeAI({
    apiKey: config.apiKey,
    baseURL: config.baseUrl,
    fetch: tracedFetch
  });
  this.provider = (model) => prov(model);
}
```

## Reasoning Support

### Thinking Budget
**Location**: `src/llm-providers/google.ts:42-60`

```typescript
if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
  const budget = typeof request.reasoningValue === 'number'
    ? request.reasoningValue
    : Number(request.reasoningValue);
  if (Number.isFinite(budget)) {
    g.thinkingConfig = {
      thinkingBudget: Math.trunc(budget),
      includeThoughts: true
    };
  } else {
    warn(`google reasoning value '${String(request.reasoningValue)}' is not numeric; ignoring`);
  }
}
```

- `reasoningValue === null` disables thinking entirely (no `thinkingConfig`).
- Numeric/string inputs are truncated to integers but **not** clamped. The base class only uses the declared `reasoningLimits` (min 1,024 / max 32,768) when it needs to derive a default budget from a reasoning **level**.

## Provider Options

**Location**: `src/llm-providers/google.ts:42-60`

```typescript
const providerOptions = (() => {
  const base: Record<string, unknown> = { google: {} };
  const g = base.google as Record<string, unknown>;
  if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) {
    g.maxOutputTokens = Math.trunc(request.maxOutputTokens);
  }
  if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) {
    g.frequencyPenalty = request.repeatPenalty;
  }
  if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
    const budget = typeof request.reasoningValue === 'number' ? request.reasoningValue : Number(request.reasoningValue);
    if (Number.isFinite(budget)) {
      g.thinkingConfig = { thinkingBudget: Math.trunc(budget), includeThoughts: true };
    }
  }
  return base;
})();
```

Fields:
- `maxOutputTokens`: Added only when finite, truncated to an integer.
- `frequencyPenalty`: Direct mapping of cli/frontmatter `repeatPenalty` when numeric.
- `thinkingConfig`: Attached only when reasoning is active and numeric.

## Turn Execution

**Location**: `src/llm-providers/google.ts:30-68`

Flow:
1. Create model instance
2. Filter tools for final turn
3. Convert tools and messages
4. Build final turn messages
5. Configure provider options
6. Execute via base class

## Response Conversion

**Location**: `src/llm-providers/google.ts:71-74`

```typescript
convertResponseMessages(messages, provider, model, tokens) {
  return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
}
```

Uses generic conversion from base class.

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `apiKey` | Google AI API key |
| `baseUrl` | Custom API endpoint |
| `stringSchemaFormatsAllowed` | Schema format filtering |
| `stringSchemaFormatsDenied` | Schema format blocking |
| `reasoningValue` | Thinking budget tokens |
| `maxOutputTokens` | Response length limit |
| `repeatPenalty` | Frequency penalty |

## Telemetry

**Via base class**:
- Token usage (input, output)
- Latency
- Stop reason
- Tool calls

## Logging

**Warnings**:
- Non-numeric reasoning value

## Events

**Handled**:
- Tool calls
- Streaming chunks
- Response completion

## Invariants

1. **Budget parsing**: Values must parse to finite numbers; otherwise they are ignored with a warning.
2. **Include thoughts**: Always `true` when thinking is enabled so `onThinking` callbacks receive content.
3. **Reasoning defaults**: The base class only uses `[1024, 32768]` bounds when deriving budgets from reasoning levels (not for explicit overrides).
4. **Frequency penalty**: Direct mapping when finite.
5. **Max tokens**: Integer truncation before sending to Google.

## Business Logic Coverage (Verified 2025-11-16)

- **Thinking config parsing**: Budgets are truncated to integers only after they pass a finite-number check; there is intentionally no upper clamp beyond what callers provide so internal harnesses can test out-of-range budgets (`src/llm-providers/google.ts:42-60`).
- **Thought streaming**: When thinking is enabled `includeThoughts: true` guarantees reasoning text returns even if the request wasn’t explicitly streaming, matching ai-agent’s expectation for the `onThinking` callback (`src/llm-providers/google.ts:47-58`).
- **Repeat penalty mapping**: `repeatPenalty` is passed through directly as Google’s `frequencyPenalty`, aligning CLI/frontmatter semantics across providers (`src/llm-providers/google.ts:42-58`).

## Test Coverage

**Phase 1**:
- Provider construction
- Thinking config building
- Response conversion
- Error handling

**Gaps**:
- Thinking budget edge cases
- API compatibility testing
- Large response handling
- Error recovery scenarios

## Troubleshooting

### Thinking not enabled
- Ensure `reasoningValue` is a positive finite number (or a numeric string)
- Confirm it was not explicitly set to `null`/`disabled`
- Review parse warnings for rejected inputs

### Frequency penalty ignored
- Check repeatPenalty numeric
- Verify finite number
- Review provider options

### API errors
- Check apiKey validity
- Verify baseUrl correct
- Review rate limits and quotas
