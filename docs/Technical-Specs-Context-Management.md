# Context Management

Token budget tracking and context guard enforcement.

---

## TL;DR

Projects token usage, enforces limits per model, forces final turn when approaching limit, manages tool output sizes.

---

## Token Counters

### Session State

| Counter | Description |
|---------|-------------|
| `currentCtxTokens` | Tokens in committed conversation |
| `pendingCtxTokens` | Tokens pending commit |
| `newCtxTokens` | New tokens this turn |
| `schemaCtxTokens` | Tool schema token estimate |

---

## Context Guard Flow

### 1. Initialization

At session start:
```typescript
targetContextConfigs = sessionTargets.map((target) => {
  return {
    provider, model,
    contextWindow,      // Model capacity
    bufferTokens,       // Safety margin
    tokenizerId
  };
});
currentCtxTokens = estimateTokensForCounters(conversation);
```

### 2. Pre-Turn Evaluation

Before each LLM request:
```typescript
const status = evaluateContextForProvider(provider, model);
// Returns: 'ok' | 'skip' | 'final'

if (status === 'final') {
  enforceContextFinalTurn();
}
if (status === 'skip') {
  // Log warning, provider cannot fit context
}
```

### 3. Guard Projection

```typescript
projected = currentCtxTokens + pendingCtxTokens + newCtxTokens + schemaCtxTokens;
limit = contextWindow - bufferTokens - maxOutputTokens;

if (projected > limit) {
  blocked.push({ provider, model, limit, projected });
}
```

### 4. Enforcement

When guard triggers:
1. Set `forcedFinalTurnReason = 'context'`
2. Log context guard enforcement
3. Restrict tools to `final_report` only
4. Emit telemetry event

---

## Limit Calculation

```typescript
effectiveLimit = contextWindow - bufferTokens - maxOutputTokens;
```

**Example**:
- contextWindow: 128,000
- bufferTokens: 256
- maxOutputTokens: 16,384
- **effectiveLimit**: 111,360

---

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

---

## Tool Budget Callback

```typescript
toolBudgetCallbacks = {
  reserveToolOutput: async (output) => {
    const tokens = estimateTokens(output);
    const guard = evaluateContextGuard(tokens);
    if (guard.blocked.length > 0) {
      this.toolBudgetExceeded = true;
      enforceContextFinalTurn();
      return { ok: false, reason: 'token_budget_exceeded' };
    }
    return { ok: true, tokens };
  },
  canExecuteTool: () => !this.toolBudgetExceeded,
};
```

Once a tool response is rejected, `canExecuteTool()` returns `false`, so the orchestrator skips additional tools.

---

## Context Window Hierarchy

**Resolution Order** (first defined wins):

1. `modelConfig.contextWindow` (model-specific)
2. `providerConfig.contextWindow` (provider default)
3. `DEFAULT_CONTEXT_WINDOW_TOKENS` (131,072)

**Buffer Resolution**:

1. `modelConfig.contextWindowBufferTokens`
2. `providerConfig.contextWindowBufferTokens`
3. `defaults.contextWindowBufferTokens`
4. `DEFAULT_CONTEXT_BUFFER_TOKENS` (256)

---

## Token Estimation

### Methods

- `estimateMessagesTokens(messages, tokenizerId)`
- `resolveTokenizer(tokenizerId)`

### Fallback

- Default estimation: characters / 4
- Per-model tokenizers when available

---

## Post-Shrink Behavior

When forced final but still over limit:
1. Recompute schema tokens (final_report only)
2. Re-evaluate guard
3. If still blocked → log warning
4. Proceed anyway (best effort)

---

## Debug Mode

**Environment**: `CONTEXT_DEBUG=true`

**Console topics**:
- `context-guard/init-counters`
- `context-guard/loop-init`
- `context-guard/schema-estimate`
- `context-guard/provider-eval`
- `context-guard/enforce`
- `context-guard/tool-eval`
- `context-guard/request-metrics`

---

## Configuration Effects

| Setting | Effect |
|---------|--------|
| `contextWindow` | Total token capacity |
| `contextWindowBufferTokens` | Safety margin |
| `maxOutputTokens` | Reserved for response |
| `tokenizer` | Estimation accuracy |
| `toolResponseMaxBytes` | Triggers tool_output storage |

---

## Telemetry

**Context Guard Events**:
- `trigger`: turn_preflight / tool_preflight
- `outcome`: forced_final / skipped_provider
- `limitTokens`: Computed limit
- `projectedTokens`: Estimated usage
- `remainingTokens`: Available headroom

---

## Invariants

1. **Guarded overflow**: When projections exceed limit, guard forces final turn
2. **Buffer preserved**: bufferTokens always subtracted
3. **Output space reserved**: maxOutputTokens always carved out
4. **Monotonic growth**: Counters only increase until turn commits
5. **Final turn enforcement**: Triggered before next LLM/tool execution

---

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

---

## See Also

- [Configuration-Context-Window](Configuration-Context-Window) - Configuration guide
- [docs/specs/context-management.md](../docs/specs/context-management.md) - Full spec

