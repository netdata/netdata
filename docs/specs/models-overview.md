# Models Overview

## TL;DR
Unified provider abstraction via BaseLLMProvider. Six concrete providers: OpenAI, Anthropic, Google, OpenRouter, Ollama, TestLLM. Provider-specific config isolated.

## Source Files
- `src/llm-providers/base.ts` - Abstract base provider (~98KB)
- `src/llm-providers/openai.ts` - OpenAI implementation
- `src/llm-providers/anthropic.ts` - Anthropic Claude
- `src/llm-providers/google.ts` - Google Gemini
- `src/llm-providers/openrouter.ts` - OpenRouter proxy
- `src/llm-providers/ollama.ts` - Local Ollama
- `src/llm-providers/test-llm.ts` - Deterministic test provider
- `src/llm-client.ts` - LLMClient orchestrator
- `src/types.ts` - Core types

## Provider Architecture

### BaseLLMProvider (`src/llm-providers/base.ts`)
**Abstract base class with shared functionality**:

**Interface**:
```typescript
interface LLMProviderInterface {
  name: string;
  executeTurn(request: TurnRequest): Promise<TurnResult>;
}
```

**Core Methods**:
- `executeTurn(TurnRequest): Promise<TurnResult>` - Abstract, implemented by each provider
- `resolveReasoningValue(context)` - Map ReasoningLevel to provider-specific value
- `shouldAutoEnableReasoningStream(level, options)` - Auto-enable reasoning streams
- `shouldDisableReasoning(context)` - Check if reasoning should be disabled
- `mapError(error)` - Convert provider errors to TurnStatus
- `getResponseMetadataCollector()` - Optional metadata extraction
- `prepareFetch(options)` - Inject custom headers before fetch

**Protected Helpers**:
- `resolveToolChoice(request)` - Determine tool choice mode ('auto' | 'required')
- `traceSdkPayload(request, stage, payload)` - Trace SDK payloads
- `buildWireRequestSnapshot(options)` - Build request snapshot for tracing
- `normalizeResponseMessages(payload)` - Parse response into standard format
- `convertToConversationMessages(messages)` - Convert to ConversationMessage[]
- `buildToolSet(tools)` - Build AI SDK ToolSet from MCPTool[]
- `buildCoreModelMessage(message)` - Convert ConversationMessage to ModelMessage
- `estimateTokens(text)` - Simple token estimation
- `mapErrorToStatus(error)` - Map exceptions to TurnStatus

**Reasoning Support**:
- Levels: `minimal`, `low`, `medium`, `high`
- Provider-specific mapping via `reasoningDefaults`
- Limits enforcement via `reasoningLimits.min/max`
- Auto-enable for certain providers (e.g., Anthropic extended thinking)

**Format Policy**:
- String format validation (date-time, email, uuid, etc.)
- Allowed/denied lists with wildcard support
- Applied during tool schema generation

### Concrete Providers

#### OpenAI (`openai`)
- API: OpenAI Chat Completions (responses or chat mode)
- Auth: `Authorization: Bearer <apiKey>`
- Features: Streaming, tool calling, JSON mode
- Reasoning: Supported via `reasoningEffort` (minimal, low, medium, high)
- Cache: Not native (server-side)
- Modes: `responses` (default) or `chat` via `openaiMode` config

#### Anthropic (`anthropic`)
- API: Anthropic Messages API
- Auth: `x-api-key: <apiKey>`
- Features: Streaming, tool use, extended thinking
- Reasoning: Extended thinking with budget tokens
- Cache: Prompt caching with 5-minute ephemeral TTL
- Special: System prompt as separate parameter

#### Google (`google`)
- API: Google AI / Vertex AI
- Auth: `x-goog-api-key: <apiKey>`
- Features: Streaming, function calling
- Reasoning: Supported via `reasoningValue` → `thinkingConfig` (budget tokens + `includeThoughts: true`)
- Cache: Server-side context caching

#### OpenRouter (`openrouter`)
- API: OpenRouter (OpenAI-compatible proxy)
- Auth: `Authorization: Bearer <apiKey>`
- Headers: HTTP-Referer, X-OpenRouter-Title
- Features: Multi-provider routing, cost reporting
- Reasoning: Provider-dependent
- Metadata: Actual provider/model, cost, upstream cost

#### Ollama (`ollama`)
- API: Ollama OpenAI-compatible endpoint
- Auth: None (local service)
- BaseURL: Typically `http://localhost:11434`
- Features: Streaming, tool calling
- Reasoning: Not supported
- Cache: None

#### TestLLM (`test-llm`)
- Purpose: Deterministic testing
- Features: Scripted responses, predictable behavior
- No network calls
- Supports all turn scenarios

## Business Logic Coverage (Verified 2025-11-16)

- **Metadata queue**: Providers push per-turn metadata (`enqueueProviderMetadata`) so `LLMClient` can attach actual provider/model, pricing, cache tokens, and parameter warnings even when AI SDK streams chunked responses (`src/llm-providers/base.ts:346-369`, `src/llm-client.ts:420-520`).
- **Parameter warning tracking**: When providers reject sampling parameters (e.g., OpenAI rejects `top_p`), `recordParameterWarning()` logs the warning once and exposes it through metadata so CLI/headends can surface helpful hints (`src/llm-providers/base.ts:370-410`).
- **Tool result splitting**: Some providers bundle multiple tool results into one message; `convertResponseMessagesGeneric` splits them and normalizes IDs so downstream `ToolsOrchestrator` sees discrete calls (`src/llm-providers/base.ts:420-520`).
- **Reasoning policy**: `resolveReasoningValue` maps `reasoningLevel` to provider-specific knobs (OpenAI `reasoningEffort`, Anthropic `reasoning` tokens, Google `includeThoughts`), and providers can override auto-enable/disable logic via `shouldAutoEnableReasoningStream` / `shouldDisableReasoning` (`src/llm-providers/base.ts:60-170`, `src/llm-providers/anthropic.ts:80-150`).
- **String format enforcement**: Base provider enforces allowed/denied JSON string formats in tool schemas (GUID/date/etc.) preventing models from hallucinating invalid types (`src/llm-providers/base.ts:30-120`).
- **LLMClient retry hints**: Provider errors map to canonical `TurnStatus` codes with `retry` directives so `AIAgentSession` can differentiate retry vs abort conditions consistently across providers (`src/llm-providers/base.ts:500-620`, `src/llm-client.ts:250-360`).

## Data Structures

### TurnRequest
```typescript
interface TurnRequest {
  provider: string;
  model: string;
  messages: ConversationMessage[];
  tools?: MCPTool[];
  temperature?: number | null;
  topP?: number | null;
  topK?: number | null;
  maxOutputTokens?: number;
  repeatPenalty?: number | null;
  reasoningLevel?: ReasoningLevel;
  reasoningValue?: ProviderReasoningValue | null;
  stream?: boolean;
  toolChoice?: 'auto' | 'required';
  toolChoiceRequired?: boolean;
  isFinalTurn?: boolean;
  contextMetrics?: TurnRequestContextMetrics;
  sdkTrace?: boolean;
  sdkTraceLogger?: (event: { stage: string; provider: string; model: string; payload: unknown }) => void;
}
```

### TurnResult
```typescript
interface TurnResult {
  status: TurnStatus;
  response?: string;
  toolCalls?: ToolCall[];
  tokens?: TokenUsage;
  latencyMs: number;
  messages: ConversationMessage[];
  hasReasoning?: boolean;
  hasContent?: boolean;
  stopReason?: string;
  providerMetadata?: ProviderTurnMetadata;
  retry?: TurnRetryDirective;
  responseBytes?: number;
}
```

### TurnStatus
```typescript
type TurnStatus =
  | { type: 'success'; hasToolCalls: boolean; finalAnswer: boolean }
  | { type: 'rate_limit'; retryAfterMs?: number; sources?: string[] }
  | { type: 'auth_error'; message: string }
  | { type: 'model_error'; message: string; retryable: boolean }
  | { type: 'network_error'; message: string; retryable: boolean }
  | { type: 'timeout'; message: string }
  | { type: 'invalid_response'; message: string }
  | { type: 'quota_exceeded'; message: string };
```

### TokenUsage
```typescript
interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  cachedTokens?: number;
  cacheReadInputTokens?: number;
  cacheWriteInputTokens?: number;
  totalTokens: number;
}
```

### ProviderTurnMetadata
```typescript
interface ProviderTurnMetadata {
  actualProvider?: string;
  actualModel?: string;
  reportedCostUsd?: number;
  upstreamCostUsd?: number;
  cacheWriteInputTokens?: number;
  cacheReadInputTokens?: number;
  effectiveCostUsd?: number;
  reasoningState?: string;
  parameterWarnings?: ProviderParameterWarning[];
}
```

## LLMClient Orchestration

**Location**: `src/llm-client.ts`

**Responsibilities**:
1. Provider registry management
2. HTTP fetch tracing
3. Metadata collection (cost, routing)
4. Turn/subturn tracking
5. Pricing computation

**Key Methods**:
- `executeTurn(TurnRequest)` - Route to provider, capture metadata
- `setTurn(turn, subturn)` - Set current turn counters
- `waitForMetadataCapture()` - Await pending async operations
- `resolveReasoningValue(provider, context)` - Delegate to provider
- `shouldAutoEnableReasoningStream(provider, level, options)` - Delegate to provider
- `shouldDisableReasoning(provider, context)` - Delegate to provider
- `getLastActualRouting()` - Get last actual provider/model
- `getLastCostInfo()` - Get last cost information

**Traced Fetch**:
- Intercepts all HTTP requests
- Logs request/response when traceLLM enabled
- Redacts sensitive headers (Authorization, API keys)
- Captures response metadata asynchronously
- Handles SSE and JSON responses

## Configuration

### Provider Config (`ProviderConfig`)
```typescript
interface ProviderConfig {
  type: 'openai' | 'anthropic' | 'google' | 'openrouter' | 'ollama' | 'test-llm';
  baseUrl?: string;
  apiKey?: string;
  headers?: Record<string, string>;
  models?: Record<string, ProviderModelConfig>;
  contextWindow?: number;
  tokenizer?: string;
  contextWindowBufferTokens?: number;
}
```

### Model Config (`ProviderModelConfig`)
```typescript
interface ProviderModelConfig {
  overrides?: ProviderModelOverrides;
  reasoning?: ProviderReasoningMapping | null;
  toolChoice?: ToolChoiceMode;
  contextWindow?: number;
  tokenizer?: string;
  contextWindowBufferTokens?: number;
}
```

### Model Overrides
```typescript
interface ProviderModelOverrides {
  temperature?: number | null;
  topP?: number | null;
  top_p?: number | null;
  topK?: number | null;
  top_k?: number | null;
  repeatPenalty?: number | null;
  repeat_penalty?: number | null;
}
```

## Telemetry

**Per LLM Request**:
- Latency (ms)
- Input/output tokens
- Cache read/write tokens
- Cost (computed or reported)
- Stop reason
- Provider metadata

**Logged Details**:
- `request_bytes` - Request payload size
- `response_bytes` - Response payload size
- `context_pct` - Context utilization percentage
- `reasoning` - Reasoning state description

## Logging

**Request Logs**:
- Severity: VRB
- Direction: request
- Type: llm
- Remote: `provider:model`
- Details: message count, bytes, final turn flag, context metrics

**Response Logs**:
- Severity: VRB (success), WRN (transient error), ERR (fatal error)
- Direction: response
- Type: llm
- Remote: `provider/actualProvider:actualModel`
- Details: latency, tokens, cache stats, cost, stop reason

**Trace Logs** (when traceLLM enabled):
- Full HTTP request/response
- Headers (redacted)
- Body (pretty-printed JSON or raw SSE)

## Events

No direct events. Accounting entries emitted per LLM attempt.

## Undocumented Behaviors

### Base Provider Hooks
1. **prepareFetch(options)**:
   - Hook for modifying fetch requests before execution
   - Location: `src/llm-providers/base.ts:452-454`

2. **getResponseMetadataCollector()**:
   - Hook for capturing response metadata asynchronously
   - Location: `src/llm-providers/base.ts:456-458`

3. **buildRetryDirective(error)**:
   - Provider-specific retry logic generation
   - Location: `src/llm-providers/base.ts:460-462`

4. **Metadata queuing system**:
   - `enqueueProviderMetadata()` / `consumeQueuedProviderMetadata()`
   - Queues metadata for later consumption
   - Location: `src/llm-providers/base.ts:346-356`

5. **Parameter warning tracking**:
   - `recordParameterWarning()` system
   - Logs and tracks parameter validation issues
   - Location: `src/llm-providers/base.ts:387-396`

6. **Response message normalization**:
   - Complex parsing from various provider formats
   - Handles tool calls, reasoning, text content
   - Location: `src/llm-providers/base.ts:153-234`

7. **Timeout controller with idle reset**:
   - Resets timeout on activity (streaming)
   - Location: `src/llm-providers/base.ts:1107-1127`

### Anthropic-Specific
8. **Signature validation on turn > 1**:
   - Strips reasoning without signatures
   - `shouldDisableReasoning()` override
   - Location: `src/llm-providers/anthropic.ts:158-200`

9. **Tool choice restriction with reasoning**:
   - Cannot force `toolChoice: 'required'` when reasoning active
   - Location: `src/llm-providers/anthropic.ts:151-156`

10. **Auto-streaming threshold**:
    - Auto-enables at 21,333+ tokens output
    - Location: `src/llm-providers/anthropic.ts:138-149`

11. **sendReasoning flag**:
    - Defaults to true, controls reasoning in response
    - Location: `src/llm-providers/anthropic.ts:107-108`

### OpenRouter-Specific
12. **User-Agent header**:
    - Hardcoded to 'ai-agent/1.0'
    - Location: `src/llm-providers/openrouter.ts:37`

13. **SSE stream parsing for metadata**:
    - Custom parsing to capture cost and routing info
    - Location: `src/llm-providers/openrouter.ts:420-451`

14. **Tool result splitting**:
    - Splits bundled tool results from single message
    - Location: `src/llm-providers/openrouter.ts:125-206`

15. **Deep merge for provider options**:
    - Merges config.custom.provider options
    - Location: `src/llm-providers/openrouter.ts:284-323`

### TestLLM-Specific
16. **Attempt counter tracking**:
    - Per scenario:turn:provider key tracking
    - Location: `src/llm-providers/test-llm.ts:32, 150-154`

17. **Temperature/TopP validation**:
    - 1e-6 tolerance for floating point comparison
    - Location: `src/llm-providers/test-llm.ts:111-129`

## Test Coverage

**Phase 2**:
- Provider selection
- Turn execution
- Error mapping
- Token counting
- Metadata capture
- Retry directives

**Gaps**:
- Provider-specific edge cases (rate limit formats)
- Extended thinking scenarios
- Cache hit/miss patterns
- Signature validation edge cases

## Troubleshooting

### Authentication failures
- Check apiKey in provider config
- Check baseUrl correctness
- Check custom headers

### Rate limiting
- Check retryAfterMs in response
- Check request frequency
- Check quota limits

### Invalid responses
- Check model compatibility with tools
- Check message format requirements
- Check streaming support

### Token counting mismatch
- Check tokenizer configuration
- Check provider-specific counting rules
- Check cache token reporting

### Missing cost information
- Check pricing table completeness
- Check actual provider/model routing
- Check metadata collector function
