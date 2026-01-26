# Context Management

## TL;DR
Token budget tracking with `ContextGuard`. The guard projects token usage, enforces limits per provider/model, forces final-turn mode when approaching limits, and manages tool output sizes. Final-turn mode still requires finalization readiness (final report + required META when configured).

## Source Files
- `src/context-guard.ts` - TargetContextConfig, guard evaluation, enforcement, tool budget mutex
- `src/ai-agent.ts` - ContextGuard construction and callbacks wiring
- `src/session-turn-runner.ts` - Preflight evaluation, forced-final schema shrink, tool filtering
- `src/final-report-manager.ts` - Final report lock state that narrows final-turn tools during META-only retries
- `src/llm-messages-xml-next.ts` - Final-turn and META-only model guidance
- `src/tokenizer-registry.ts` - Token estimation

## Data Structures

### TargetContextConfig
**Location**: `src/context-guard.ts`

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
**Location**: `src/context-guard.ts`

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
**Location**: `src/context-guard.ts`

```typescript
interface ContextGuardEvaluation {
  blocked: ContextGuardBlockedEntry[];
  projectedTokens: number;
}
```

## Token Counters

**Session State** (`src/context-guard.ts`):
- `currentCtxTokens: number` - Tokens in committed conversation
- `pendingCtxTokens: number` - Tokens pending commit
- `newCtxTokens: number` - New tokens this turn
- `schemaCtxTokens: number` - Tool schema token estimate

## Context Guard Flow

### 1. Initialization
**Location**: `src/ai-agent.ts`

```typescript
this.targetContextConfigs = sessionTargets.map((target) => {
  const contextWindow = sessionConfig.contextWindow
    ?? modelConfig?.contextWindow
    ?? providerConfig.contextWindow
    ?? DEFAULT;
  const bufferTokens = modelConfig?.contextWindowBufferTokens
    ?? providerConfig.contextWindowBufferTokens
    ?? defaultBuffer;
  return { provider, model, contextWindow, tokenizerId, bufferTokens };
});
this.currentCtxTokens = this.estimateTokensForCounters(this.conversation);
```

### 2. Pre-Turn Evaluation
**Location**: `src/session-turn-runner.ts`

```typescript
const toolSelection = this.selectToolsForTurn(provider, isFinalTurn);
this.schemaCtxTokens = this.estimateToolSchemaTokens(toolSelection.toolsForTurn);

const providerContextStatus = this.evaluateContextForProvider(provider, model);
if (providerContextStatus === 'final') {
  const evaluation = this.evaluateContextGuard();
  this.enforceContextFinalTurn(evaluation.blocked, 'turn_preflight');
  this.schemaCtxTokens = this.computeForcedFinalSchemaTokens(provider);
  this.evaluateContextGuard(); // Post-shrink warning if still blocked
} else if (providerContextStatus === 'skip') {
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
**Function**: `ContextGuard.enforceFinalTurn(blocked, trigger)`

**Actions**:
1. Set `forcedFinalTurnReason = 'context'`
2. Emit callback side effects (logs + telemetry via `handleContextGuardForcedFinalTurn`)
3. SessionTurnRunner shrinks schema tokens for final-turn tools and re-evaluates the guard
4. Final-turn tool filtering still respects final report lock state (META-only retries remove extra tools), and finalization readiness remains required

### 5. Tool Budget Callback
**Location**: `src/context-guard.ts`

```typescript
toolBudgetCallbacks = {
  reserveToolOutput: async (output) => {
    return await this.mutex.runExclusive(() => {
      const tokens = this.estimateTokens([{ role: 'tool', content: output }]);
      const guard = this.evaluate(tokens);
      if (guard.blocked.length > 0) {
        if (!this.toolBudgetExceeded) {
          this.toolBudgetExceeded = true;
          this.enforceFinalTurn(guard.blocked, 'tool_preflight');
        }
        return { ok: false, tokens, reason: 'token_budget_exceeded' };
      }
      this.newCtxTokens += tokens;
      return { ok: true, tokens };
    });
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
- maxOutputTokens: 16384
- effectiveLimit: 111360

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

**Location**: `src/session-turn-runner.ts`

When forced final turn but still over limit:
1. Recompute schema tokens for the forced final-turn tool set (`agent__final_report` plus any allowed extra tools)
2. Re-evaluate guard
3. If still blocked, log warning
4. Proceed anyway (best effort)

If a final report is captured without required META, the final report becomes locked and extra tools are removed so retries focus only on META completion.

**Warning**: "Context limit exceeded; forcing final turn."

## Business Logic Coverage (Verified 2026-01-25)

- **Per-provider outcomes**: `ContextGuard.evaluateForProvider` returns `ok`, `skip`, or `final`, and `SessionTurnRunner` applies the result during turn preflight (`src/context-guard.ts`, `src/session-turn-runner.ts`).
- **Context telemetry**: Guard enforcement emits `recordContextGuardMetrics` with `{provider, model, trigger, outcome, remaining_tokens}` for observability (`src/session-turn-runner.ts`, `src/telemetry/index.ts`).
- **Tool budget mutex**: Tool output reservations run under a mutex so overlapping tool calls cannot race token projections or forced-final enforcement (`src/context-guard.ts`).
- **Schema shrink logic**: When forced-final mode is triggered, the runner recomputes schema tokens for the final-turn tool set and re-evaluates the guard before proceeding (`src/session-turn-runner.ts`).
- **Forced-final messaging + META-only gating**: The guard sets `forcedFinalTurnReason='context'`, XML-NEXT renders forced-final guidance, and final report lock state can further narrow tools and guidance to META-only retries (`src/context-guard.ts`, `src/llm-messages-xml-next.ts`, `src/final-report-manager.ts`).

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `contextWindow` | Total token capacity |
| `contextWindowBufferTokens` | Safety margin |
| `maxOutputTokens` | Reserved for response |
| `tokenizer` | Estimation accuracy |
| `toolResponseMaxBytes` | Triggers tool_output handle storage; handle text is what the guard budgets against |

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

**Phase 2**:
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
- Verify toolResponseMaxBytes and tool_output storage warnings
- Review accumulated context

### Provider skipped
- Check provider's context window
- Verify model configuration
- Review projected vs limit

### Context not growing
- Check pendingCtxTokens commit
- Verify conversation append
- Check counter synchronization
