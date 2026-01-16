# Extended Reasoning

Thinking blocks and reasoning configuration.

---

## Overview

Extended reasoning enables models to show their thinking process before providing answers. This is particularly important for Anthropic's Claude models with "extended thinking" and OpenAI-compatible models with reasoning capabilities.

---

## Configuration

### Frontmatter

```yaml
---
models:
  - anthropic/claude-sonnet-4
reasoning: medium
---
Your prompt here.
```

### CLI

```bash
# Set default reasoning for prompts that don't specify
ai-agent --agent test.ai --default-reasoning medium "query"

# Force reasoning on all agents (stomps frontmatter)
ai-agent --agent test.ai --override reasoning=high "query"
```

### Config File

```json
{
  "defaults": {
    "reasoning": "medium"
  }
}
```

---

## Reasoning Levels

| Level | Description |
|-------|-------------|
| `none` / `unset` | Disable reasoning entirely |
| `default` | Use configured defaults |
| `minimal` | Minimal effort (~1024 tokens) |
| `low` | 30% of max output tokens |
| `medium` | 60% of max output tokens |
| `high` | 80% to max output tokens |

---

## Provider-Specific Behavior

### Anthropic

- Reasoning blocks have cryptographic signatures
- Signatures must be preserved for conversation replay
- If first request doesn't return signatures → reasoning disabled
- `stream: true` required when max output tokens >= 21,333

### OpenAI-Compatible

- Reasoning injected via `providerOptions.openaiCompatible.<field>`
- Default field: `reasoning_content`
- Per-model `interleaved` configuration controls injection

---

## Thinking Output

### CLI

Thinking is displayed with `THK` prefix:
```
THK: Let me analyze this problem...
THK: First, I need to understand...
```

### Callbacks

```typescript
callbacks: {
  onEvent: (event, meta) => {
    if (event.type === 'thinking') {
      console.log('Thinking:', event.text);
    }
  }
}
```

### Headend Output

- **OpenAI-compatible**: Thinking in SSE reasoning deltas
- **Anthropic-compatible**: Thinking as `thinking_delta` blocks
- **REST/CLI**: Thinking in logs and responses
- **Slack**: Latest thinking in status block

---

## Conversation Replay

Reasoning blocks must be preserved correctly for multi-turn conversations:

### Anthropic Requirements

1. Signatures must survive serialization
2. Missing signatures → reasoning disabled for turn
3. Tool-only turns don't require thinking blocks
4. Signed segments must be intact across turns

### Implementation Details

- `ConversationMessage.reasoning` mirrors AI SDK `ReasoningOutput[]`
- `normalizeReasoningSegments` validates signatures before turns
- Sessions with invalid signatures continue without reasoning

---

## Sub-Agent Inheritance

- Frontmatter reasoning settings are **NOT** copied to sub-agents
- Only CLI/global overrides propagate
- Prevents master agents from forcing reasoning on all sub-agents

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

## Limitations

- Segments without signatures are dropped
- Redacted thinking preserved but not decrypted
- Token budgets not exposed in headend summaries
- Not all providers support extended reasoning

---

## Troubleshooting

### Reasoning Disabled Unexpectedly

Check logs for:
- `disableReasoningForTurn` - Invalid signatures found
- Missing `providerMetadata.anthropic.signature`

### Conversation Replay Fails

Anthropic errors like `messages.1.content.0.type`:
- Thinking block required but missing/invalid
- Check signature preservation in persistence

### Reasoning Not Appearing

1. Verify provider supports reasoning
2. Check reasoning level is not `none`/`unset`
3. Check headend/output format supports thinking

---

## Best Practices

1. **Use appropriate levels**: `minimal` for simple tasks, `high` for complex reasoning
2. **Monitor costs**: Extended reasoning uses more tokens
3. **Test replay**: Verify multi-turn conversations work
4. **Log reasoning**: Preserve thinking for debugging

---

## See Also

- [Configuration-Providers](Configuration-Providers) - Provider configuration
- [docs/REASONING.md](../docs/REASONING.md) - Full reasoning documentation

