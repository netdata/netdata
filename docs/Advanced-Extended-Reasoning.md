# Extended Reasoning

Configure thinking blocks and reasoning modes for models that support extended thinking (Claude, o1, etc.).

---

## Table of Contents

- [Overview](#overview) - What extended reasoning is and why it matters
- [Quick Start](#quick-start) - Minimal configuration to enable reasoning
- [Configuration Reference](#configuration-reference) - All reasoning options
- [Reasoning Levels](#reasoning-levels) - Available levels and token budgets
- [Provider-Specific Behavior](#provider-specific-behavior) - How each provider handles reasoning
- [Thinking Output](#thinking-output) - Where thinking appears in different contexts
- [Conversation Replay](#conversation-replay) - Multi-turn requirements
- [Troubleshooting](#troubleshooting) - Common issues and solutions
- [See Also](#see-also) - Related documentation

---

## Overview

**Extended reasoning** enables models to show their thinking process before providing answers. This is particularly useful for:

- Complex problem-solving tasks
- Multi-step reasoning chains
- Debugging model decision-making
- Tasks requiring careful analysis

Supported providers:

- **Anthropic**: Claude models with extended thinking (cryptographically signed thinking blocks)
- **OpenAI-compatible**: Models with reasoning capabilities (o1, etc.)

---

## Quick Start

Enable reasoning in your agent frontmatter:

```yaml
---
models:
  - anthropic/claude-sonnet-4
reasoning: medium
---
Analyze this complex problem step by step.
```

Or via CLI:

```bash
ai-agent --agent my-agent.ai --reasoning medium "complex query"
```

---

## Configuration Reference

### reasoning

| Property     | Value                                                                             |
| ------------ | --------------------------------------------------------------------------------- |
| Type         | `string`                                                                          |
| Default      | `unset` (provider decides)                                                        |
| Valid values | `none`, `minimal`, `low`, `medium`, `high`, `default`, `unset`, `inherit`         |
| Note         | `default`, `unset`, and `inherit` are treated as "use default" (same as omitting) |
| Required     | No                                                                                |

**Description**: Sets the reasoning effort level. Higher levels use more tokens for thinking. Use `default` to explicitly fall back to global/CLI defaults.

**Where to configure**:

| Location             | Syntax                               | Priority                                                  |
| -------------------- | ------------------------------------ | --------------------------------------------------------- |
| Global/CLI override  | `--override reasoning=medium`        | 1 (highest, stomps everything)                            |
| Frontmatter          | `reasoning: medium`                  | 2                                                         |
| CLI flag             | `--reasoning medium`                 | 3 (overrides frontmatter)                                 |
| CLI default flag     | `--default-reasoning medium`         | 4 (only when agent has `reasoning: default` or omits key) |
| Config file defaults | `options.defaultReasoning: "medium"` | 5 (lowest)                                                |

**Frontmatter example**:

```yaml
---
models:
  - anthropic/claude-sonnet-4
reasoning: high
---
```

**CLI example**:

```bash
# Set reasoning for this run
ai-agent --agent test.ai --reasoning medium "query"

# Set default reasoning for sub-agents (only applies when sub-agent has reasoning: default or omits the key)
ai-agent --agent test.ai --default-reasoning medium "query"

# Force reasoning on all agents (stomps frontmatter)
ai-agent --agent test.ai --override reasoning=high "query"
```

**Config file example**:

```json
{
  "options": {
    "reasoning": "medium",
    "defaultReasoning": "medium"
  }
}
```

> **Note**: In config files, use `options.reasoning` to set reasoning for master agent, and `options.defaultReasoning` as the fallback for sub-agents.

Note: `reasoningTokens` maps to `defaults.reasoningValue` in the config file. Use frontmatter (`reasoningTokens: 16000`), CLI flag (`--reasoning-tokens 16000`), or config file (`defaults.reasoningValue: 16000`).

---

### reasoningTokens

| Property     | Value                                       |
| ------------ | ------------------------------------------- |
| Type         | `number` or `string`                        |
| Default      | Computed from reasoning level               |
| Valid values | Positive integer, `disabled`, `off`, `none` |
| Required     | No                                          |

**Description**: Explicit token budget for reasoning. Overrides the automatic calculation from reasoning level.

**Frontmatter example**:

```yaml
---
models:
  - anthropic/claude-sonnet-4
reasoningTokens: 16000
---
```

**CLI example**:

```bash
# Set explicit token budget
ai-agent --agent test.ai --reasoning-tokens 16000 "query"

# Disable reasoning tokens
ai-agent --agent test.ai --override reasoningTokens=disabled "query"
```

---

**Understanding `--reasoning` vs `--default-reasoning`**:

- `--reasoning <level>`: Sets reasoning for the master agent. If sub-agents don't have explicit reasoning in their frontmatter, they inherit this value.
- `--default-reasoning <level>`: Sets fallback reasoning only for agents that don't specify `reasoning: default` or omit the key. Has lower priority than frontmatter and `--reasoning`.

Use `--override reasoning=<level>` when you want to force reasoning on every agent, ignoring frontmatter.

---

## Reasoning Levels

| Level     | Description                                 | Budget Calculation                          |
| --------- | ------------------------------------------- | ------------------------------------------- |
| `none`    | Disable reasoning entirely                  | 0                                           |
| `default` | Explicitly fall back to global/CLI defaults | Uses --default-reasoning or config defaults |
| `minimal` | Quick thinking, simple tasks                | Provider minimum (1024)                     |
| `low`     | 20% of max output tokens                    | 20% × maxOutputTokens                       |
| `medium`  | 50% of max output tokens                    | 50% × maxOutputTokens                       |
| `high`    | 80% of max output tokens                    | 80% × maxOutputTokens                       |

**Example token budgets** (assuming 4096 max output tokens, Anthropic provider):

| Level     | Calculated Budget   |
| --------- | ------------------- |
| `minimal` | 1,024 tokens        |
| `low`     | ~819 tokens (20%)   |
| `medium`  | ~2,048 tokens (50%) |
| `high`    | ~3,277 tokens (80%) |

> **Note**: Actual budgets are computed dynamically from `maxOutputTokens`. Providers have different limits (Anthropic: min 1024, max 128,000).

---

## Provider-Specific Behavior

### Anthropic

| Aspect        | Behavior                                                                 |
| ------------- | ------------------------------------------------------------------------ |
| Signatures    | Reasoning blocks have cryptographic signatures                           |
| Preservation  | Signatures must survive serialization for replay                         |
| First request | If no signatures returned, reasoning may be disabled on subsequent turns |
| Streaming     | Auto-enabled when reasoning active AND max output tokens >= 21,333       |

**Signature handling**:

- Anthropic signs thinking blocks cryptographically
- Invalid or missing signatures disable reasoning for that turn only (not session-wide)
- Subsequent turns check if prior assistant tool calls have valid signatures
- Tool-only turns without reasoning are allowed to continue without reasoning enabled

### OpenAI-Compatible

| Aspect        | Behavior                                             |
| ------------- | ---------------------------------------------------- |
| Injection     | Reasoning via `providerOptions.<providerId>.<field>` |
| Default field | `reasoningEffort`                                    |
| Interleaved   | Per-model `interleaved` config controls injection    |

**Provider configuration for interleaved reasoning**:

```json
{
  "providers": {
    "openrouter": {
      "type": "openai",
      "apiKey": "...",
      "models": {
        "deepseek/deepseek-r1": {
          "interleaved": "reasoning_content"
        }
      }
    }
  }
}
```

> **Note**: The `interleaved` config injects reasoning content into the assistant message. For OpenAI-compatible providers, reasoning is sent as `providerOptions.<providerId>.reasoningEffort`.

---

## Thinking Output

### CLI

Thinking is displayed with `THK` prefix:

```
THK: Let me analyze this problem...
THK: First, I need to understand the constraints...
THK: The key insight here is...
```

### Library Callbacks

```typescript
callbacks: {
  onEvent: (event, meta) => {
    if (event.type === "thinking") {
      console.log("Thinking:", event.text);
    }
  };
}
```

### Headend Output

| Headend              | Format                              |
| -------------------- | ----------------------------------- |
| OpenAI-compatible    | Thinking in SSE reasoning deltas    |
| Anthropic-compatible | Thinking as `thinking_delta` blocks |
| REST/CLI             | Thinking in logs and responses      |
| Slack                | Latest thinking in status block     |

---

## Conversation Replay

Multi-turn conversations require special handling for reasoning blocks.

### Anthropic Requirements

1. **Signatures must survive serialization**: Store complete reasoning metadata
2. **Missing signatures**: Reasoning disabled for that turn only (conversation continues without reasoning)
3. **Tool-only turns**: Do not require thinking blocks
4. **Signed segments**: Must remain intact across turns

### Implementation

- `ConversationMessage.reasoning` stores reasoning segments
- `normalizeReasoningSegments` validates signatures before turns
- Sessions with invalid signatures gracefully continue without reasoning

**Example conversation message with reasoning**:

```typescript
{
  role: 'assistant',
  content: 'The answer is 42.',
  reasoning: [
    {
      type: 'thinking',
      text: 'Let me work through this...',
      signature: 'abc123...'  // Anthropic signature
    }
  ]
}
```

---

## Sub-Agent Inheritance

| Setting                          | Inheritance                          |
| -------------------------------- | ------------------------------------ |
| Frontmatter `reasoning`          | NOT copied to sub-agents             |
| Frontmatter `reasoning: default` | Sub-agents use default flag (if set) |
| CLI `--reasoning`                | Propagates to sub-agents             |
| CLI `--default-reasoning`        | Propagates as fallback to sub-agents |
| `--override reasoning=X`         | Propagates to all sub-agents         |

**Why**: Prevents master agents from forcing expensive reasoning on every sub-agent. Sub-agents can override by explicitly setting `reasoning` in their frontmatter.

---

## Logging

Request/response logs include reasoning status:

```
reasoning=unset|disabled|minimal|low|medium|high
```

### Debug Mode

```bash
CONTEXT_DEBUG=true ai-agent --agent test.ai "query" 2>&1 | grep reasoning
```

---

## Troubleshooting

### Reasoning Disabled Unexpectedly

**Symptoms**: No thinking output despite `reasoning: medium`

**Check logs for**:

- `disableReasoningForTurn` - Invalid signatures found
- Missing `providerMetadata.anthropic.signature`

**Solutions**:

1. Verify provider supports reasoning
2. Check conversation history for corrupted signatures
3. Start fresh session if replay fails

---

### Conversation Replay Fails

**Symptoms**: Anthropic errors like `messages.1.content.0.type`

**Cause**: Thinking block required but missing/invalid

**Solutions**:

1. Check signature preservation in persistence layer
2. Verify serialization/deserialization preserves all metadata
3. Clear conversation history and start fresh

---

### Reasoning Not Appearing

**Diagnostic steps**:

1. **Verify provider supports reasoning**:

   ```bash
   # Anthropic Claude models support extended thinking
   ai-agent --agent test.ai --models anthropic/claude-sonnet-4 --reasoning high "test"
   ```

2. **Check reasoning level is not disabled**:

   ```yaml
   # This disables reasoning:
   reasoning: none
   ```

3. **Verify headend/output supports thinking**:
   - CLI shows `THK:` prefix
   - JSON output includes reasoning field

---

### Token Budget Issues

**Symptoms**: Reasoning truncated or model times out

**Solutions**:

1. Reduce reasoning level: `high` -> `medium` -> `low`
2. Set explicit budget: `reasoningTokens: 8000`
3. Increase LLM timeout: `--llm-timeout-ms 300000`

---

## Best Practices

1. **Match level to task complexity**:
   - `minimal`: Simple questions, quick lookups
   - `low`/`medium`: Multi-step analysis, code review
   - `high`: Complex reasoning, mathematical proofs

2. **Monitor costs**: Extended reasoning uses significantly more tokens

3. **Test replay**: Verify multi-turn conversations work before production

4. **Log reasoning**: Preserve thinking for debugging and audit trails

5. **Use appropriate models**: Not all models support extended reasoning

---

## See Also

- [Configuration-Providers](Configuration-Providers) - Provider configuration
- [Agent-Files-Models](Agent-Files-Models) - Model configuration in frontmatter
- [CLI-Overrides](CLI-Overrides) - Runtime override options
