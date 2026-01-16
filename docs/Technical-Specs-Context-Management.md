# Context Management

Token budget tracking, context guard enforcement, and overflow handling to prevent LLM context window overflow.

---

## Table of Contents

- [TL;DR](#tldr) - Quick summary of context management
- [Why This Matters](#why-this-matters) - When context management affects you
- [Core Concepts](#core-concepts) - Key terms and counters
- [Context Guard Algorithm](#context-guard-algorithm) - How limits are enforced
- [Limit Calculation](#limit-calculation) - How budgets are computed
- [Token Estimation](#token-estimation) - How tokens are counted
- [Tool Budget Management](#tool-budget-management) - Tool output handling
- [Configuration Options](#configuration-options) - Settings that affect context
- [Debug Mode](#debug-mode) - Troubleshooting context issues
- [Troubleshooting](#troubleshooting) - Common problems and solutions
- [See Also](#see-also) - Related documentation

---

## TL;DR

ai-agent tracks token usage across the conversation and enforces limits before they cause LLM API errors. When approaching the context window limit, the guard forces a final turn (tools restricted to `final_report` only). Tool outputs that exceed size limits are stored on disk and replaced with handles.

---

## Why This Matters

Context management affects you when:

- **Session ends unexpectedly**: Context guard forced a final turn
- **Tools stop executing**: Tool budget exceeded
- **Large responses stored**: Tool output exceeded size limit
- **Provider errors**: Context window overflow at the API level

Understanding these mechanisms helps you:

- Configure appropriate limits for your use case
- Debug "unexpected final turn" scenarios
- Optimize token usage for longer conversations
- Handle large tool outputs gracefully

---

## Core Concepts

### Token Counters

The session tracks multiple token counters to project usage.

```mermaid
graph LR
    subgraph "Current State"
        Current[currentCtxTokens<br/>Committed conversation]
    end

    subgraph "Pending"
        Pending[pendingCtxTokens<br/>Uncommitted outputs]
        New[newCtxTokens<br/>New this turn]
    end

    subgraph "Overhead"
        Schema[schemaCtxTokens<br/>Tool schemas]
    end

    subgraph "Projection"
        Projected[projected tokens<br/>Sum of all]
    end

    Current --> Projected
    Pending --> Projected
    New --> Projected
    Schema --> Projected
```

| Counter                  | Description                                                   | Updated When         |
| ------------------------ | ------------------------------------------------------------- | -------------------- |
| `currentCtxTokens`       | Tokens in committed conversation                              | After turn completes |
| `pendingCtxTokens`       | Tokens pending commit (tool outputs, retry notices)           | During turn          |
| `newCtxTokens`           | New tokens added this turn                                    | During turn          |
| `schemaCtxTokens`        | Tool schema token estimate                                    | When tools change    |
| `currentReasoningTokens` | Reasoning/thinking budget tokens for extended thinking models | At turn start        |

### Guard Triggers

| Trigger          | When Checked                  | Action if Exceeded |
| ---------------- | ----------------------------- | ------------------ |
| `turn_preflight` | Before each LLM request       | Force final turn   |
| `tool_preflight` | Before committing tool output | Block tool output  |

---

## Context Guard Algorithm

### Pre-Turn Evaluation

Before each LLM request, the guard evaluates whether the context fits.

```mermaid
flowchart TD
    Start[Turn Start] --> Evaluate[Evaluate Context for Provider]
    Evaluate --> Status{Guard Status?}

    Status -->|ok| Proceed[Proceed with Turn]
    Status -->|skip| Skip[Skip Provider<br/>Try Next]
    Status -->|final| Force[Force Final Turn]

    Force --> Restrict[Restrict Tools to final_report]
    Restrict --> Inject[Inject Instruction Message]
    Inject --> Proceed
```

### Projection Formula

```
projected = currentCtxTokens + pendingCtxTokens + newCtxTokens + extraTokens
limit = contextWindow - bufferTokens - maxOutputTokens

if projected > limit:
    trigger guard enforcement
```

**Note**: `schemaCtxTokens` is tracked separately but is already included in `currentCtxTokens` after the first turn (via cache_write/cache_read). On the first turn, projection is slightly underestimated; subsequent turns are accurate.

### Enforcement Actions

When the guard triggers:

| Step | Action                                       |
| ---- | -------------------------------------------- |
| 1    | Set `forcedFinalTurnReason = 'context'`      |
| 2    | Log context guard enforcement                |
| 3    | Restrict tools to `agent__final_report` only |
| 4    | Emit telemetry event                         |
| 5    | Recompute schema tokens (only final_report)  |

---

## Limit Calculation

### Effective Limit Formula

```
effectiveLimit = contextWindow - bufferTokens - maxOutputTokens
```

**Example Calculation**:

| Setting            | Value       |
| ------------------ | ----------- |
| contextWindow      | 128,000     |
| bufferTokens       | 8,192       |
| maxOutputTokens    | 16,384      |
| **effectiveLimit** | **103,424** |

### Context Window Resolution

The context window value is resolved from multiple sources (first defined wins):

| Priority | Source                          | Example            |
| -------- | ------------------------------- | ------------------ |
| 1        | `modelConfig.contextWindow`     | Per-model override |
| 2        | `providerConfig.contextWindow`  | Provider default   |
| 3        | `DEFAULT_CONTEXT_WINDOW_TOKENS` | 131,072 (fallback) |

### Buffer Resolution

| Priority | Source                                     | Default |
| -------- | ------------------------------------------ | ------- |
| 1        | `modelConfig.contextWindowBufferTokens`    | -       |
| 2        | `providerConfig.contextWindowBufferTokens` | -       |
| 3        | `defaults.contextWindowBufferTokens`       | -       |
| 4        | `DEFAULT_CONTEXT_BUFFER_TOKENS`            | 8192    |

---

## Token Estimation

### Estimation Methods

| Method          | Accuracy | Speed  | Used When               |
| --------------- | -------- | ------ | ----------------------- |
| Tokenizer-based | High     | Slower | Tokenizer configured    |
| Character-based | Low      | Fast   | Computed for comparison |

**Combined approach**: Uses `Math.max(tokenizerResult, characterApproximation)` to get the higher of both estimates.

**Character-based approximation**: `tokens ≈ characters / 4`

### Tokenizer Configuration

```yaml
providers:
  openai:
    models:
      gpt-4:
        tokenizer: "cl100k_base"
```

### What Gets Estimated

| Content            | Estimation                               |
| ------------------ | ---------------------------------------- |
| System prompt      | At session start                         |
| User message       | At session start                         |
| Assistant messages | After LLM response                       |
| Tool results       | Before commit (max of two methods)       |
| Tool schemas       | When tools change (scaled × 2.09)        |
| Reasoning/thinking | Set at turn start (display/logging only) |

---

## Tool Budget Management

### Tool Output Size Handling

```mermaid
flowchart TD
    Output[Tool Output] --> SizeCheck{Exceeds<br/>toolResponseMaxBytes?}

    SizeCheck -->|No| BudgetCheck{Exceeds<br/>Token Budget?}
    SizeCheck -->|Yes| Store[Store to Disk]

    Store --> Handle[Replace with Handle]
    Handle --> Commit[Commit to Conversation]

    BudgetCheck -->|No| Commit
    BudgetCheck -->|Yes| Drop[Drop Output]

    Drop --> Fail[Return Failure Message]
    Fail --> SetFlag[Set toolBudgetExceeded]
```

### Size Cap Behavior

When output exceeds `toolResponseMaxBytes`:

| Step | Action                              |
| ---- | ----------------------------------- |
| 1    | Write output to disk                |
| 2    | Generate unique handle              |
| 3    | Replace output with handle message  |
| 4    | Log warning with bytes/lines/tokens |

**Handle Format**: `session-<uuid>/<file-uuid>`

**Storage Location**: `/tmp/ai-agent-<run-hash>/`

### Budget Callback Interface

```typescript
toolBudgetCallbacks = {
  reserveToolOutput: async (output) => {
    const tokens = estimateTokens(output);
    const guard = evaluateContextGuard(tokens);

    if (guard.blocked.length > 0) {
      this.toolBudgetExceeded = true;
      enforceContextFinalTurn();
      return {
        ok: false,
        tokens,
        reason: "token_budget_exceeded",
        availableTokens,
      };
    }

    // Accumulate tokens so subsequent tool reservations see the updated context
    this.newCtxTokens += tokens;
    return { ok: true, tokens };
  },

  previewToolOutput: async (output) => {
    const tokens = estimateTokens(output);
    const guard = evaluateContextGuard(tokens);
    if (guard.blocked.length > 0) {
      return {
        ok: false,
        tokens,
        reason: "token_budget_exceeded",
        availableTokens,
      };
    }
    return { ok: true, tokens };
  },

  canExecuteTool: () => !this.toolBudgetExceeded,
};
```

Once `toolBudgetExceeded` is set, `canExecuteTool()` returns `false` and the orchestrator skips remaining tool calls.

---

## Configuration Options

| Setting                     | Type   | Default | Effect                    |
| --------------------------- | ------ | ------- | ------------------------- |
| `contextWindow`             | number | 131,072 | Total token capacity      |
| `contextWindowBufferTokens` | number | 8,192   | Safety margin             |
| `maxOutputTokens`           | number | 16,384  | Reserved for LLM response |
| `tokenizer`                 | string | -       | Estimation accuracy       |
| `toolResponseMaxBytes`      | number | 12,288  | Triggers disk storage     |
| `reasoning`                 | enum   | -       | Enables extended thinking |

### Example Configuration

```yaml
providers:
  openai:
    contextWindow: 128000
    models:
      gpt-4-turbo:
        maxOutputTokens: 16384
        contextWindow: 128000

defaults:
  toolResponseMaxBytes: 12288
  maxOutputTokens: 16384
```

---

## Debug Mode

Enable detailed context guard logging with environment variable.

**Enable**: `CONTEXT_DEBUG=true`

### Debug Topics

| Topic                           | Information                            |
| ------------------------------- | -------------------------------------- |
| `context-guard/init-counters`   | Initial token counts                   |
| `context-guard/loop-init`       | Per-turn initialization                |
| `context-guard/schema-estimate` | Tool schema token estimate             |
| `context-guard/evaluate`        | Full projection with reasoning tokens  |
| `context-guard/evaluate-target` | Per-target limit calculation           |
| `context-guard/provider-eval`   | Per-provider evaluation                |
| `context-guard/build-metrics`   | Metrics for LLM request                |
| `context-guard/enforce`         | Guard enforcement events               |
| `context-guard/tool-eval`       | Tool output evaluation (via callbacks) |

### Example Debug Output

```
[context-guard/provider-eval] openai/gpt-4
  currentCtxTokens: 45000
  pendingCtxTokens: 2000
  schemaCtxTokens: 3000
  projected: 50000
  limit: 111360
  status: ok
```

**With reasoning enabled**:

```
[context-guard/build-metrics] openai/gpt-4
  ctxTokens: 45000
  pendingCtxTokens: 2000
  schemaCtxTokens: 3000
  expectedTokens: 47000
  reasoningTokens: 1000
  maxOutputTokens: 16384
```

---

## Troubleshooting

### Unexpected Final Turn

**Symptom**: Session ends with "context guard enforced" message.

**Causes**:

- Context window setting too low
- Buffer tokens too large
- Tool schemas consuming budget
- Large tool outputs accumulated

**Solutions**:

1. Check `contextWindow` matches your model's actual limit
2. Reduce `contextWindowBufferTokens` (carefully)
3. Use `toolsAllowed`/`toolsDenied` to limit tools
4. Configure `toolResponseMaxBytes` to store large outputs

### Token Count Mismatch

**Symptom**: Estimated tokens differ significantly from provider-reported.

**Causes**:

- No tokenizer configured (character approximation used)
- Wrong tokenizer for model
- Provider counts differently
- Estimation uses `Math.max(tokenizer, char_approx)` which may overestimate

**Solutions**:

1. Configure correct tokenizer for your model
2. Compare with actual provider token counts
3. Adjust buffer tokens to account for variance

### Tool Budget Exceeded

**Symptom**: Tools stop executing mid-turn.

**Causes**:

- Large tool outputs consuming context
- Many tool calls in single turn
- Accumulated conversation history

**Solutions**:

1. Lower `toolResponseMaxBytes` to store large outputs
2. Increase `contextWindow` if model supports it
3. Review tool output sizes
4. Consider breaking into multiple sessions

### Post-Shrink Still Over Limit

**Symptom**: "Still over limit after shrink" warning.

**Cause**: Even with only `final_report` tool, context exceeds limit.

**Behavior**: Session proceeds best-effort (may fail at API level).

**Solutions**:

1. Increase context window
2. Review conversation history size
3. Use more aggressive context management earlier

---

## Invariants

These rules MUST hold:

1. **Guarded overflow**: When projections exceed limit, guard forces final turn
2. **Buffer preserved**: `bufferTokens` always subtracted from capacity
3. **Output space reserved**: `maxOutputTokens` always carved out
4. **Monotonic growth**: Counters only increase until turn commits
5. **Final turn enforcement**: Triggered BEFORE next LLM/tool execution
6. **Reasoning included**: Reasoning/thinking tokens are INSIDE `maxOutputTokens`, not added on top

---

## See Also

- [Session Lifecycle](Technical-Specs-Session-Lifecycle) - When context is checked
- [Tool System](Technical-Specs-Tool-System) - Tool output handling
- [Agent-Files-Behavior](Agent-Files-Behavior) - Configuration options
- [specs/context-management.md](specs/context-management.md) - Full specification
