# Ollama LLM Provider

## TL;DR
Local Ollama inference provider with automatic URL normalization and custom options support via ollama-ai-provider-v2.

## Source Files
- `src/llm-providers/ollama.ts` - Full implementation (97 lines)
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `ollama-ai-provider-v2` - External AI SDK provider

## Provider Identity
- **Name**: `ollama`
- **Kind**: LLM Provider
- **SDK**: ollama-ai-provider-v2

## Construction

**Location**: `src/llm-providers/ollama.ts:15-25`

```typescript
constructor(config: ProviderConfig, tracedFetch?) {
  super({
    formatPolicy: {
      allowed: config.stringSchemaFormatsAllowed,
      denied: config.stringSchemaFormatsDenied
    }
  });
  this.config = config;
  const normalizedBaseUrl = this.normalizeBaseUrl(config.baseUrl);
  const prov = createOllama({
    baseURL: normalizedBaseUrl,
    fetch: tracedFetch
  });
  this.provider = (model) => prov(model);
}
```

## URL Normalization

**Location**: `src/llm-providers/ollama.ts:27-41`

```typescript
normalizeBaseUrl(url?: string): string {
  const def = 'http://localhost:11434/api';
  if (url === undefined || url.length === 0) return def;

  let v = url.replace(/\/$/, '');

  // Replace trailing /v1 with /api
  if (/\/v1\/?$/.test(v)) return v.replace(/\/v1\/?$/, '/api');

  // If already ends with /api, keep as-is
  if (/\/api\/?$/.test(v)) return v;

  // Otherwise, append /api
  return v + '/api';
}
```

Examples:
- `http://host:11434` → `http://host:11434/api`
- `http://host:11434/v1` → `http://host:11434/api`
- `http://host:11434/api` → `http://host:11434/api` (unchanged)
- Default: `http://localhost:11434/api`

## Provider Options

### Base Options
**Location**: `src/llm-providers/ollama.ts:82-89`

```typescript
getProviderOptions() {
  const custom = this.config.custom ?? {};
  return custom.providerOptions ?? undefined;
}
```

Loaded from config.custom.providerOptions.

### Dynamic Options
**Location**: `src/llm-providers/ollama.ts:56-70`

```typescript
const dyn: Record<string, unknown> = {};

if (typeof request.maxOutputTokens === 'number' && Number.isFinite(request.maxOutputTokens)) {
  const existing = (dyn.ollama as { options?: Record<string, unknown> } | undefined)?.options ?? {};
  dyn.ollama = { options: { ...existing, num_predict: Math.trunc(request.maxOutputTokens) } };
}

if (typeof request.repeatPenalty === 'number' && Number.isFinite(request.repeatPenalty)) {
  const existing = (dyn.ollama as { options?: Record<string, unknown> } | undefined)?.options ?? {};
  dyn.ollama = { options: { ...existing, repeat_penalty: request.repeatPenalty } };
}

if (Object.keys(dyn).length > 0) {
  providerOptions = { ...(providerOptions ?? {}), ...dyn };
}
```

Ollama-specific mappings:
- `maxOutputTokens` → `num_predict`
- `repeatPenalty` → `repeat_penalty`

## Turn Execution

**Location**: `src/llm-providers/ollama.ts:43-80`

Flow:
1. Create model instance
2. Filter tools for final turn
3. Convert tools and messages
4. Build final turn messages
5. Get base provider options
6. Overlay dynamic knobs
7. Execute via base class

## Response Conversion

**Location**: `src/llm-providers/ollama.ts:92-95`

```typescript
convertResponseMessages(messages, provider, model, tokens) {
  return this.convertResponseMessagesGeneric(messages, provider, model, tokens);
}
```

Uses generic conversion from base class.

## No Reasoning Support

Unlike Anthropic/Google/OpenAI, Ollama provider:
- Does not configure reasoning limits
- Does not map reasoningValue
- Uses base class defaults only

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `baseUrl` | Ollama server URL (auto-normalized) |
| `stringSchemaFormatsAllowed` | Schema format filtering |
| `stringSchemaFormatsDenied` | Schema format blocking |
| `custom.providerOptions` | Base Ollama options |
| `maxOutputTokens` | num_predict parameter |
| `repeatPenalty` | repeat_penalty parameter |

## Telemetry

**Via base class**:
- Token usage (input, output)
- Latency
- Stop reason
- Tool calls

## Logging

**Warnings**:
- Provider option cleanup failures

## Events

**Handled**:
- Tool calls
- Streaming chunks
- Response completion

## Invariants

1. **URL normalization**: Always produces /api endpoint
2. **Default URL**: localhost:11434/api
3. **Options merging**: Base + dynamic overlay
4. **No API key**: Local inference doesn't require key
5. **Integer tokens**: num_predict truncated to integer

## Business Logic Coverage (Verified 2025-11-16)

- **URL rewrite rules**: Base URLs ending with `/v1` are rewritten to `/api`, bare hostnames append `/api`, and trailing slashes are trimmed, matching Ollama’s native REST endpoints rather than the OpenAI shim (`src/llm-providers/ollama.ts:27-41`).
- **Option overlay order**: `custom.providerOptions` serves as the base object; per-request settings overlay on top so session-specific knobs (e.g., `maxOutputTokens`) override but never mutate the config defaults (`src/llm-providers/ollama.ts:56-89`).
- **Tool filtering**: Because Ollama lacks built-in tool calling, this provider relies on the base class’ internal tool orchestration, ensuring assistant tool calls still run through MCP/REST providers even on local models (`src/llm-providers/ollama.ts:43-75`).

## Test Coverage

**Phase 1**:
- URL normalization cases
- Provider options merging
- Response conversion
- Error handling

**Gaps**:
- Custom option validation
- Model availability checks
- Connection timeout handling
- Large model support

## Troubleshooting

### Connection refused
- Check Ollama service running
- Verify URL normalization
- Review port number

### Model not found
- Check model installed in Ollama
- Verify model name spelling
- Review available models

### Options not applied
- Check custom.providerOptions format
- Verify dynamic overlay
- Review merge warnings

### Slow response
- Check local hardware capabilities
- Verify model size vs resources
- Review num_predict setting
