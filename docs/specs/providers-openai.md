# OpenAI LLM Provider

## TL;DR
OpenAI provider supporting responses/chat modes, reasoning effort levels, and frequency penalty via @ai-sdk/openai.

## Source Files
- `src/llm-providers/openai.ts` - Full implementation (77 lines)
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `@ai-sdk/openai` - External AI SDK provider

## Provider Identity
- **Name**: `openai`
- **Kind**: LLM Provider
- **SDK**: @ai-sdk/openai

## Construction

**Location**: `src/llm-providers/openai.ts:13-36`

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
      high: 'high'
    }
  });
  this.config = config;
  const prov = createOpenAI({
    apiKey: config.apiKey,
    baseURL: config.baseUrl,
    fetch: tracedFetch
  });

  const mode = config.openaiMode ?? 'responses';
  if (mode === 'responses') {
    this.provider = (model) => prov.responses(model);
  } else {
    this.provider = (model) => prov.chat(model);
  }
}
```

## API Modes

### responses (default)
- Uses OpenAI responses API
- Modern interface
- Extended features

### chat
- Uses OpenAI chat completions API
- Legacy compatibility
- Standard chat interface

## Reasoning Support

### Effort Levels
**Location**: `src/llm-providers/openai.ts:16-21`

```typescript
reasoningDefaults: {
  minimal: 'minimal',
  low: 'low',
  medium: 'medium',
  high: 'high'
}
```

### Configuration
**Location**: `src/llm-providers/openai.ts:55-57`

```typescript
if (request.reasoningValue !== undefined && request.reasoningValue !== null) {
  o.reasoningEffort = typeof request.reasoningValue === 'string'
    ? request.reasoningValue
    : String(request.reasoningValue);
}
```

Accepted values: `minimal`, `low`, `medium`, `high`

## Provider Options

**Location**: `src/llm-providers/openai.ts:50-59`

```typescript
const providerOptions = {
  openai: {
    toolChoice: 'required',
    maxTokens: request.maxOutputTokens,
    frequencyPenalty: request.repeatPenalty,
    reasoningEffort: request.reasoningValue
  }
};
```

Fields:
- `toolChoice`: Force tool use (default: 'required')
- `maxTokens`: Maximum output tokens
- `frequencyPenalty`: Repeat penalty mapping
- `reasoningEffort`: Reasoning level string

## Turn Execution

**Location**: `src/llm-providers/openai.ts:38-69`

Flow:
1. Create model instance (responses or chat mode)
2. Filter tools for final turn
3. Convert tools and messages
4. Build final turn messages
5. Configure provider options
6. Execute via base class

## Response Conversion

**Location**: `src/llm-providers/openai.ts:72-75`

```typescript
convertResponseMessages(messages, provider, model, tokens) {
  return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
}
```

Uses generic conversion from base class.

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `apiKey` | OpenAI API authentication |
| `baseUrl` | Custom API endpoint |
| `openaiMode` | responses or chat API |
| `stringSchemaFormatsAllowed` | Schema format filtering |
| `stringSchemaFormatsDenied` | Schema format blocking |
| `reasoningValue` | Reasoning effort level |
| `maxOutputTokens` | Response length limit |
| `repeatPenalty` | Frequency penalty |

## Telemetry

**Via base class**:
- Token usage (input, output)
- Latency
- Stop reason
- Tool calls

## Logging

No provider-specific logging. Uses base class error handling.

## Events

**Handled**:
- Tool calls
- Streaming chunks
- Response completion

## Invariants

1. **Tool choice default**: Always 'required'
2. **Mode selection**: responses (default) or chat
3. **Reasoning effort**: String-based levels only
4. **Frequency penalty**: Direct mapping to OpenAI
5. **Max tokens**: Integer truncation

## Business Logic Coverage (Verified 2025-11-16)

- **Mode fallback**: `openaiMode` defaults to `responses`; when set to `chat` the provider switches to the legacy Chat Completions API, but all other behaviors (tool filtering, final-turn messages) stay identical (`src/llm-providers/openai.ts:24-40`).
- **Tool choice override**: `resolveToolChoice` always forces `'required'` unless the session explicitly disables tool choice; this matches ai-agent’s expectation that the LLM must either call tools or final_report every turn (`src/llm-providers/openai.ts:46-60`, `src/llm-providers/base.ts:180-225`).
- **Reasoning mapping**: Reasoning values pass through unchanged when configured per provider/model; omitted values fall back to `reasoningDefaults`, so frontmatter `reasoning: high` works even when CLI doesn’t specify `reasoningValue` (`src/llm-providers/base.ts:90-150`, `src/llm-providers/openai.ts:50-58`).
- **Repeat penalty mapping**: ai-agent’s `repeatPenalty` (Claude-style) maps to OpenAI’s `frequencyPenalty`, ensuring CLI/frontmatter values behave consistently across providers (`src/llm-providers/openai.ts:50-58`).

## Test Coverage

**Phase 2**:
- Mode selection
- Provider options building
- Response conversion
- Reasoning effort configuration

**Gaps**:
- Mode switching behavior
- Reasoning effort validation
- API compatibility testing
- Error recovery scenarios

## Troubleshooting

### Wrong API mode
- Check openaiMode configuration
- Verify API availability
- Review endpoint compatibility

### Reasoning not applied
- Check reasoningValue format
- Verify model supports reasoning
- Review effort level string

### Frequency penalty ignored
- Check repeatPenalty numeric
- Verify finite number
- Review provider options

### API errors
- Check apiKey validity
- Verify baseUrl correct
- Review rate limits and quotas
