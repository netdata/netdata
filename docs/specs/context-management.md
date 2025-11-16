# Context Management

## TL;DR
Token budget tracking with context guard. Projects token usage, enforces limits per model, forces final turn when approaching limit, manages tool output sizes.

## Source Files
- `src/ai-agent.ts:94-115` - TargetContextConfig, ContextGuardBlockedEntry
- `src/ai-agent.ts:441-458` - targetContextConfigs initialization
- `src/ai-agent.ts:516-530` - toolBudgetCallbacks
- `src/ai-agent.ts` - evaluateContextGuard(), enforceContextFinalTurn()
- `src/tokenizer-registry.ts` - Token estimation

## Data Structures

### TargetContextConfig
**Location**: `src/ai-agent.ts:94-100`

```typescript
interface TargetContextConfig {
  provider: string;
  model: string;
  contextWindow: number;
  tokenizerId?: string;
  bufferTokens: number;
}
```

### ContextGuardBlockedEntry
**Location**: `src/ai-agent.ts:102-110`

```typescript
interface ContextGuardBlockedEntry {
  provider: string;
  model: string;
  contextWindow: number;
  bufferTokens: number;
  maxOutputTokens: number;
  limit: number;
  projected: number;
}
```

### ContextGuardEvaluation
**Location**: `src/ai-agent.ts:112-115`

```typescript
interface ContextGuardEvaluation {
  blocked: ContextGuardBlockedEntry[];
  projectedTokens: number;
}
```

## Token Counters

**Session State** (`src/ai-agent.ts:151-154`):
- `currentCtxTokens: number` - Tokens in committed conversation
- `pendingCtxTokens: number` - Tokens pending commit
- `newCtxTokens: number` - New tokens this turn
- `schemaCtxTokens: number` - Tool schema token estimate

## Context Guard Flow

### 1. Initialization
**Location**: `src/ai-agent.ts:441-458`

```typescript
this.targetContextConfigs = sessionTargets.map((target) => {
  const contextWindow = modelConfig?.contextWindow ?? providerConfig.contextWindow ?? DEFAULT;
  const bufferTokens = modelConfig?.contextWindowBufferTokens ?? providerConfig.contextWindowBufferTokens ?? defaultBuffer;
  return { provider, model, contextWindow, tokenizerId, bufferTokens };
});
this.currentCtxTokens = this.estimateTokensForCounters(this.conversation);
```

### 2. Pre-Turn Evaluation
**Location**: `src/ai-agent.ts:1490-1553`

```typescript
const providerContextStatus = this.evaluateContextForProvider(provider, model);
if (providerContextStatus === 'final') {
  this.enforceContextFinalTurn(evaluation.blocked, 'turn_preflight');
}
if (providerContextStatus === 'skip') {
  // Log warning, provider cannot fit context
}
```

### 3. Guard Evaluation
**Function**: `evaluateContextGuard(schemaTokens)`

**Logic**:
```typescript
projectedTokens = currentCtxTokens + pendingCtxTokens + newCtxTokens + schemaTokens;
blocked = [];
for each targetConfig:
  limit = contextWindow - bufferTokens - maxOutputTokens;
  if (projectedTokens > limit):
    blocked.push({ provider, model, limit, projected });
return { blocked, projectedTokens };
```

### 4. Enforcement
**Function**: `enforceContextFinalTurn(blocked, trigger)`

**Actions**:
1. Set `forcedFinalTurnReason = 'context'`
2. Log context guard enforcement
3. Restrict tools to final_report only
4. Emit telemetry event

### 5. Tool Budget Callback
**Location**: `src/ai-agent.ts:516-530`

```typescript
toolBudgetCallbacks = {
  reserveToolOutput: async (output) => {
    const tokens = estimateTokens(output);
    const guard = evaluateContextGuard(tokens);
    if (guard.blocked.length > 0) {
      this.toolBudgetExceeded = true;
      enforceContextFinalTurn(guard.blocked, 'tool_preflight');
      return { ok: false, tokens, reason: 'token_budget_exceeded' };
    }
    return { ok: true, tokens };
  },
  canExecuteTool: () => !this.toolBudgetExceeded,
};
```

Once a tool response is rejected, `canExecuteTool()` flips to `false`, so the orchestrator skips additional non-internal tool executions and the session pivots straight to the forced-final flow.

## Token Estimation

**Location**: `src/tokenizer-registry.ts`

### Methods
- `estimateMessagesTokens(messages, tokenizerId)` - Estimate tokens for messages
- `resolveTokenizer(tokenizerId)` - Get tokenizer for model

### Algorithm
```typescript
function estimateTokensForCounters(messages: ConversationMessage[]): number {
  // Sum token estimates for each message
  // Accounts for role, content, tool calls
  // Adds overhead for message structure
}
```

### Fallback
- Default estimation: characters / 4
- Per-model tokenizers when available

## Context Window Hierarchy

**Resolution Order** (first defined wins):
1. `modelConfig.contextWindow` (model-specific)
2. `providerConfig.contextWindow` (provider default)
3. `DEFAULT_CONTEXT_WINDOW_TOKENS` (131072)

**Buffer Resolution**:
1. `modelConfig.contextWindowBufferTokens`
2. `providerConfig.contextWindowBufferTokens`
3. `defaults.contextWindowBufferTokens`
4. `DEFAULT_CONTEXT_BUFFER_TOKENS` (256)

## Limit Calculation

```typescript
effectiveLimit = contextWindow - bufferTokens - maxOutputTokens;
```

**Example**:
- contextWindow: 128000
- bufferTokens: 256
- maxOutputTokens: 4096
- effectiveLimit: 123648

## Guard Triggers

### turn_preflight
Before each LLM request:
- Evaluate projected tokens
- Force final turn if exceeded
- Skip provider if cannot fit

### tool_preflight
Before committing tool output:
- Estimate output tokens
- Block if budget exceeded
- Set toolBudgetExceeded flag

## Post-Shrink Behavior

**Location**: `src/ai-agent.ts:1493-1516`

When forced final turn but still over limit:
1. Recompute schema tokens (final_report only)
2. Re-evaluate guard
3. If still blocked, log warning
4. Proceed anyway (best effort)

**Warning**: "Context guard post-shrink still over projected limit"

## Business Logic Coverage (Verified 2025-11-16)

- **Per-provider outcomes**: `evaluateContextForProvider` returns `ok`, `skip`, or `final` so large-context providers can be skipped without forcing the entire session into final-turn mode (`src/ai-agent.ts:1386-1516`).
- **Context telemetry**: Every enforcement emits `recordContextGuardMetrics` with `{provider, model, trigger, outcome, remaining_tokens}` so dashboards can alert on chronic guard hits (`src/ai-agent.ts:2323-2368`).
- **Tool budget mutex**: Tool output reservations run inside a mutex so overlapping tool calls cannot simultaneously exceed the byte cap or race on `toolBudgetExceeded` (`src/ai-agent.ts:516-562`).
- **Schema shrink logic**: When the guard fires, the session recomputes schema tokens for the remaining tools (usually just `agent__final_report`) and logs explicit WARN entries if still over limit before proceeding (`src/ai-agent.ts:2345-2405`).
- **Forced-final messaging**: Guard activations push a deterministic system message plus `forcedFinalTurnReason='context'`, which later controls exit code selection and FIN summaries (`src/ai-agent.ts:2370-2650`).

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `contextWindow` | Total token capacity |
| `contextWindowBufferTokens` | Safety margin |
| `maxOutputTokens` | Reserved for response |
| `tokenizer` | Estimation accuracy |
| `toolResponseMaxBytes` | Indirect token impact |

## Telemetry

**Context Guard Events**:
- `trigger`: turn_preflight / tool_preflight
- `outcome`: forced_final / skipped_provider
- `limitTokens`: Computed limit
- `projectedTokens`: Estimated usage
- `remainingTokens`: Available headroom

**Function**: `reportContextGuardEvent()`

## Logging

**Key Log Events**:
- `agent:context` - Guard evaluation/enforcement
- Details: projected_tokens, limit_tokens, remaining_tokens
- Warning: Over budget situations
- Info: Context usage percentage

**Request Logs Include**:
- `ctx_tokens` - Current context size
- `new_tokens` - New content added
- `schema_tokens` - Tool schema overhead
- `expected_tokens` - Projected total
- `context_pct` - Utilization percentage

## Debug Mode

**Environment**: `CONTEXT_DEBUG=true`

**Console topics**:
- `context-guard/init-counters` – initial session counters
- `context-guard/loop-init` – turn-level counters before each attempt
- `context-guard/schema-estimate` / `schema-tokens` – schema token budgets and forced-final shrink logs
- `context-guard/provider-eval` – blocked provider/model pairs
- `context-guard/enforce` / `log-entry` – forced-final emission payloads
- `context-guard/tool-eval` – per-tool reservation diagnostics
- `context-guard/request-metrics` – metrics attached to the next LLM call

## Invariants

1. **Guarded overflow**: When projections exceed the limit, the guard forces a final turn and logs WARN entries; if shrinkage still leaves it over limit, execution continues best-effort under the forced-final flag.
2. **Buffer preserved**: bufferTokens are subtracted from every limit calculation.
3. **Output space reserved**: maxOutputTokens is always carved out before evaluating projections.
4. **Monotonic growth per turn**: Counters only increase until the turn commits; shrinkage happens between turns, never mid-turn.
5. **Final turn enforcement**: Triggered before the next LLM/tool execution so the model is instructed to finish rather than gather more context.

## Test Coverage

**Phase 1**:
- Context estimation accuracy
- Guard evaluation logic
- Final turn enforcement
- Tool budget blocking
- Multi-target evaluation

**Gaps**:
- Edge cases near limits
- Tokenizer accuracy validation
- Long conversation shrinking

## Troubleshooting

### Unexpected final turn
- Check context window settings
- Verify buffer tokens configuration
- Check tool schema size
- Review maxOutputTokens

### Token count mismatch
- Check tokenizer configuration
- Verify estimation algorithm
- Compare with actual provider counts

### Tool budget exceeded
- Check tool output sizes
- Verify toolResponseMaxBytes
- Review accumulated context

### Provider skipped
- Check provider's context window
- Verify model configuration
- Review projected vs limit

### Context not growing
- Check pendingCtxTokens commit
- Verify conversation append
- Check counter synchronization
