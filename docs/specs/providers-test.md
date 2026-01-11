# Test LLM Provider

## TL;DR
Deterministic test harness provider that replays predefined scenarios for testing turn orchestration, tool execution, retry logic, and error handling.

## Source Files
- `src/llm-providers/test-llm.ts` - Full implementation (498 lines)
- `src/llm-providers/base.ts` - BaseLLMProvider parent
- `src/tests/fixtures/test-llm-scenarios.ts` - Scenario definitions

## Provider Identity
- **Name**: `test-llm`
- **Kind**: LLM Provider (Test Harness)
- **SDK**: None (custom implementation)

## Construction

**Location**: `src/llm-providers/test-llm.ts:30-36`

```typescript
constructor(_config: ProviderConfig) {
  super();
}
```

Minimal construction - no external SDK.

## Core Concepts

### Scenario Definition
```typescript
interface ScenarioDefinition {
  id: string;                              // Scenario identifier
  description: string;                     // Human-readable description
  turns: ScenarioTurn[];                   // Turn responses
  systemPromptMustInclude?: string[];      // Required system fragments
}
```

### Scenario Turn
```typescript
interface ScenarioTurn {
  turn: number;                            // Turn number (1-indexed)
  response: ScenarioStepResponse;          // Response to generate
  expectedTools?: string[];                // Tools that must be available
  allowMissingTools?: boolean;             // Skip tool validation
  expectedTemperature?: number;            // Verify temperature
  expectedTopP?: number;                   // Verify topP
  expectedReasoning?: 'enabled' | 'disabled';  // Verify reasoning state
  failuresBeforeSuccess?: number;          // Fail N times first
  failureThrows?: boolean;                 // Throw error vs return status
  failureStatus?: string;                  // Status type on failure
  failureMessage?: string;                 // Error message
  failureRetryable?: boolean;              // Allow retry
  failureRetryAfterMs?: number;            // Retry-After value
  failureError?: Record<string, unknown>;  // Custom error object
}
```

### Response Types
```typescript
type ScenarioStepResponse =
  | { kind: 'tool-call'; toolCalls: ToolCallSpec[]; ... }
  | { kind: 'final-report'; reportContent: string; ... }
  | { kind: 'text'; assistantText: string; ... };
```

## Scenario Execution Flow

**Location**: `src/llm-providers/test-llm.ts:85-232`

1. **Extract scenario ID** from first user message
2. **Load scenario** from registry
3. **Validate context** (system prompt fragments)
4. **Compute turn number** (count assistant messages + 1)
5. **Find matching step** or build fallback
6. **Validate tools** against expected
7. **Validate temperature** if specified
8. **Validate topP** if specified
9. **Validate reasoning** state if specified
10. **Handle failures** (retry simulation)
11. **Create scenario model** with step data
12. **Execute turn** via base class

## Scenario ID Extraction

**Location**: `src/llm-providers/test-llm.ts:234-240`

```typescript
extractScenarioId(messages: ConversationMessage[]): string | undefined {
  const userMessages = messages.filter((m) => m.role === 'user');
  const firstUser = userMessages.at(0);
  if (firstUser === undefined) return undefined;
  return firstUser.content.trim();
}
```

First user message content = scenario ID.

## Turn Number Computation

**Location**: `src/llm-providers/test-llm.ts:242-245`

```typescript
computeTurnNumber(messages: ConversationMessage[]): number {
  const assistantMessages = messages.filter((m) => m.role === 'assistant').length;
  return assistantMessages + 1;
}
```

Turn N = N assistant messages already in conversation.

## Validation

### System Prompt
**Location**: `src/llm-providers/test-llm.ts:247-255`

```typescript
validateScenarioContext(scenario, request): ErrorDescriptor | undefined {
  const systemPrompt = request.messages.find((m) => m.role === 'system');
  const requiredFragments = scenario.systemPromptMustInclude ?? [];
  const systemContent = systemPrompt?.content ?? '';
  const missing = requiredFragments.filter((f) => !systemContent.includes(f));
  if (missing.length === 0) return undefined;
  return { message: `System prompt missing: ${missing.join(', ')}` };
}
```

### Tools
**Location**: `src/llm-providers/test-llm.ts:270-281`

```typescript
validateTools(step, request): ErrorDescriptor | undefined {
  if (step.allowMissingTools) return undefined;
  const expected = step.expectedTools ?? [];
  if (expected.length === 0) return undefined;
  const available = new Set(request.tools.map((t) => sanitizeToolName(t.name)));
  const missing = expected
    .filter((t) => !t.startsWith('agent__'))  // Internal tools exempted
    .filter((t) => !available.has(sanitizeToolName(t)));
  if (missing.length === 0) return undefined;
  return { message: `Expected tools not available: ${missing.join(', ')}` };
}
```

### Temperature
**Location**: `src/llm-providers/test-llm.ts:111-119`

```typescript
if (expectedTemperature !== undefined) {
  if (Math.abs(actualTemperature - expectedTemperature) > 1e-6) {
    return createErrorTurn(`Expected temperature ${expectedTemperature} but received ${actualTemperature}`);
  }
}
```

### TopP
**Location**: `src/llm-providers/test-llm.ts:121-129`

```typescript
if (expectedTopP !== undefined) {
  if (Math.abs(actualTopP - expectedTopP) > 1e-6) {
    return createErrorTurn(`Expected topP ${expectedTopP} but received ${actualTopP}`);
  }
}
```

### Reasoning State
**Location**: `src/llm-providers/test-llm.ts:130-139`

```typescript
if (expectedReasoning !== undefined) {
  const reasoningEnabled = request.reasoningValue !== null && request.reasoningValue !== undefined;
  if (expectedReasoning === 'enabled' && !reasoningEnabled) {
    return createErrorTurn('Expected reasoning to remain enabled');
  }
  if (expectedReasoning === 'disabled' && reasoningEnabled) {
    return createErrorTurn('Expected reasoning to be disabled');
  }
}
```

## Failure Simulation

**Location**: `src/llm-providers/test-llm.ts:149-210`

```typescript
const attemptKey = `${scenarioId}:${turn}:${provider}`;
const attemptCount = this.attemptCounters.get(attemptKey) ?? 0;
const failuresAllowed = step.failuresBeforeSuccess ?? 0;

if (attemptCount < failuresAllowed) {
  this.attemptCounters.set(attemptKey, attemptCount + 1);

  if (step.failureThrows === true) {
    throw new Error(step.failureMessage ?? 'Simulated failure');
  }

  switch (step.failureStatus) {
    case 'timeout':
      return { status: { type: 'timeout', message }, ... };
    case 'invalid_response':
      return { status: { type: 'invalid_response', message }, ... };
    case 'network_error':
      return { status: { type: 'network_error', message, retryable }, ... };
    case 'rate_limit':
      return { status: { type: 'rate_limit', retryAfterMs }, ... };
    case 'model_error':
    default:
      return { status: { type: 'model_error', message, retryable }, ... };
  }
}
```

Attempt tracking per scenario:turn:provider key.

## Scenario Language Model

**Location**: `src/llm-providers/test-llm.ts:316-340`

```typescript
function createScenarioLanguageModel(context: StepContext): LanguageModelV2 {
  return {
    specificationVersion: 'v2',
    provider: PROVIDER_NAME,
    modelId: context.modelId,
    supportedUrls: {},
    doGenerate(_options) {
      return Promise.resolve(buildResponse(context.step.response));
    },
    doStream(_options) {
      const response = buildResponse(context.step.response);
      const parts = convertContentToStream(response.content, ...);
      const stream = new ReadableStream({
        start(controller) {
          parts.forEach((part) => controller.enqueue(part));
          controller.close();
        },
      });
      return Promise.resolve({ stream });
    },
  };
}
```

Returns LanguageModelV2 that generates predefined responses.

## Response Building

### Tool Call Response
**Location**: `src/llm-providers/test-llm.ts:381-399`

```typescript
function buildToolCallContent(response: ToolCallStep): LanguageModelV2Content[] {
  const items = [];
  if (response.assistantText) {
    items.push({ type: 'text', text: response.assistantText });
  }
  response.toolCalls.forEach((call, index) => {
    const callId = call.callId ?? `call-${index + 1}`;
    if (call.assistantText) {
      items.push({ type: 'text', text: call.assistantText });
    }
    items.push({
      type: 'tool-call',
      toolCallId: callId,
      toolName: call.toolName,
      input: JSON.stringify(call.arguments),
    });
  });
  return items;
}
```

### Final Report Response
**Location**: `src/llm-providers/test-llm.ts:401-418`

```typescript
function buildFinalReportContent(response: FinalReportStep): LanguageModelV2Content[] {
  const items = [];
  if (response.assistantText) {
    items.push({ type: 'text', text: response.assistantText });
  }
  items.push({
    type: 'tool-call',
    toolCallId: 'agent-final-report',
    toolName: 'agent__final_report',
    input: JSON.stringify({
      status: response.status ?? 'success',
      report_format: response.reportFormat,
      report_content: response.reportContent,
      content_json: response.reportContentJson,
    }),
  });
  return items;
}
```

### Reasoning Content
**Location**: `src/llm-providers/test-llm.ts:420-461`

```typescript
function appendReasoningParts(content: LanguageModelV2Content[], response): void {
  const segments = [];

  // From response.reasoning array
  if (Array.isArray(response.reasoning)) {
    response.reasoning.forEach((entry) => {
      segments.push({ text: entry.text, providerMetadata: entry.providerMetadata });
    });
  }

  // From response.reasoningContent (array, string, or object)
  if (response.reasoningContent !== undefined) {
    // Handle various formats
  }

  segments.forEach((segment) => {
    content.push({
      type: 'reasoning',
      text: segment.text,
      ...(segment.providerMetadata ? { providerMetadata: segment.providerMetadata } : {}),
    });
  });
}
```

## Reasoning Signature Validation

**Location**: `src/llm-providers/test-llm.ts:38-79`

```typescript
shouldDisableReasoning(context): { disable: boolean; normalized: ConversationMessage[] } {
  if (context.currentTurn <= 1 || !context.expectSignature) {
    return { disable: false, normalized: context.conversation };
  }

  let missing = false;
  const normalized = context.conversation.map((message) => {
    if (message.role !== 'assistant') return message;
    if (!message.toolCalls || message.toolCalls.length === 0) return message;

    const segments = message.reasoning ?? [];
    if (segments.length === 0) return message;

    const hasSignature = segments.some((s) => this.segmentHasSignature(s));
    if (hasSignature) return message;

    missing = true;
    const cloned = { ...message };
    delete cloned.reasoning;
    return cloned;
  });

  return { disable: missing, normalized };
}
```

Removes reasoning from messages without signatures.

### Signature Detection
**Location**: `src/llm-providers/test-llm.ts:66-79`

```typescript
segmentHasSignature(segment): boolean {
  const anthropic = segment.providerMetadata?.anthropic;
  const signature = anthropic?.signature ?? '';
  const redacted = anthropic?.redactedData ?? '';
  return signature.length > 0 || redacted.length > 0;
}
```

Checks for Anthropic signature or redactedData.

## Stream Conversion

**Location**: `src/llm-providers/test-llm.ts:463-497`

```typescript
function convertContentToStream(content, finishReason, usage): LanguageModelV2StreamPart[] {
  const parts = [{ type: 'stream-start', warnings: [] }];

  content.forEach((entry) => {
    if (entry.type === 'text') {
      parts.push({ type: 'text-start', id });
      parts.push({ type: 'text-delta', id, delta: entry.text });
      parts.push({ type: 'text-end', id });
    }
    if (entry.type === 'reasoning') {
      parts.push({ type: 'reasoning-start', id });
      parts.push({ type: 'reasoning-delta', id, delta: entry.text });
      parts.push({ type: 'reasoning-end', id });
    }
    if (entry.type === 'tool-call' || entry.type === 'tool-result') {
      parts.push(entry);
    }
  });

  parts.push({ type: 'finish', finishReason, usage });
  return parts;
}
```

Converts non-streaming response to streaming parts.

## Error Handling

### Error Turn Creation
**Location**: `src/llm-providers/test-llm.ts:283-313`

```typescript
createErrorTurn(request, message, scenarioId?): Promise<TurnResult> {
  const fallback = {
    turn: this.computeTurnNumber(request.messages),
    response: {
      kind: 'final-report',
      assistantText: 'Scenario validation failed.',
      reportContent: `# Test Harness Error\n\n${message}`,
      reportFormat: 'markdown',
      status: 'failure',
    },
  };

  // Execute as final report
  return super.executeNonStreamingTurn(model, messages, tools, request, start, undefined);
}
```

### Fallback Step
**Location**: `src/llm-providers/test-llm.ts:257-268`

```typescript
buildFallbackStep(turn, message): ScenarioTurn {
  return {
    turn,
    response: {
      kind: 'final-report',
      assistantText: 'Encountered scenario mismatch.',
      reportContent: `# Scenario Failure\n\n${message}`,
      reportFormat: 'markdown',
      status: 'failure',
    },
  };
}
```

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `config` | Ignored (no external configuration) |
| Scenario ID | First user message content |
| Turn number | Count of assistant messages + 1 |
| `expectedTools` | Tool availability validation |
| `expectedTemperature` | Temperature value verification |
| `expectedTopP` | TopP value verification |
| `expectedReasoning` | Reasoning state verification |
| `failuresBeforeSuccess` | Retry simulation count |

## Telemetry

**Scenario metadata**:
- Token usage from scenario definition (default: 64 input, 32 output, 96 total)
- Provider metadata from response definition

**Via base class**:
- Latency tracking
- Stop reason mapping

## Logging

**Implicit warnings**:
- Scenario not found
- System prompt missing fragments
- Tools not available
- Temperature mismatch
- TopP mismatch
- Reasoning state mismatch

## Events

**Handled**:
- Tool calls (deterministic)
- Streaming chunks (converted from non-streaming)
- Response completion
- Failure simulation

## Invariants

1. **Scenario ID required**: First user message = scenario identifier
2. **Turn computation**: assistantMessages.length + 1
3. **Deterministic responses**: Same scenario always produces same output
4. **Tool validation**: Expected tools must be present (unless allowMissingTools)
5. **Attempt tracking**: Per scenario:turn:provider key
6. **Signature preservation**: Removes reasoning without signatures on later turns
7. **Error recovery**: All errors produce final_report with failure content and mark the session unsuccessful

## Business Logic Coverage (Verified 2025-11-16)

- **Attempt counters**: `{ scenarioId, turn, provider }` keys ensure `failuresBeforeSuccess` runs exactly as many times as requested; once the scripted success path executes the counter remains at the max value, so future invocations skip the simulated failures for that provider/scenario combination (`src/llm-providers/test-llm.ts:130-212`).
- **Reasoning signature stripping**: The provider mirrors Anthropic’s behavior by stripping reasoning content without signatures on turns > 1, guaranteeing deterministic transcripts for harness assertions (`src/llm-providers/test-llm.ts:24-82`).
- **Phase 2 harness integration**: TestLLM drives every deterministic scenario in `src/tests/phase2-harness-scenarios/` to validate retry logic, tool orchestration, accounting, and pricing without live APIs (`src/tests/phase2-harness.ts`, `src/tests/fixtures/test-llm-scenarios.ts`).

## Test Coverage

**Phase 2**:
- Scenario loading
- Turn number computation
- Tool validation
- Temperature/TopP validation
- Reasoning state validation
- Failure simulation (all status types)
- Response building (tool-call, final-report)
- Stream conversion
- Signature validation

**Gaps**:
- Complex multi-turn scenarios
- Edge cases in reasoning content formats
- Large response handling
- Provider metadata preservation

## Troubleshooting

### Scenario not found
- Check first user message content
- Verify scenario registered in test-llm-scenarios.ts
- Review listScenarioIds() output

### Tool validation failure
- Check expectedTools list
- Verify tools provided in request
- Review allowMissingTools flag

### Turn mismatch
- Count assistant messages in conversation
- Verify scenario defines expected turn
- Check for missing turns in scenario

### Temperature/TopP mismatch
- Check expected values in scenario
- Verify request values
- Review floating point comparison (1e-6 tolerance)

### Retry not working
- Check failuresBeforeSuccess count
- Verify attemptCounters tracking
- Review attempt key format

### Reasoning validation failed
- Check expectedReasoning setting
- Verify reasoningValue in request
- Review null/undefined distinction
