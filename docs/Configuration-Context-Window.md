# Context Window Configuration

Configure token budgets and context guards.

---

## Overview

The context guard prevents token overflow by:
1. Tracking token usage per turn
2. Rejecting tool calls that would exceed the budget
3. Forcing early completion when limits approach

---

## Configuration

### Provider-Level

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "contextWindow": 128000,
      "tokenizer": "tiktoken:gpt-4o"
    }
  }
}
```

### Model-Level

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "models": {
        "gpt-4o": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o"
        },
        "gpt-4o-mini": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o"
        }
      }
    }
  }
}
```

### Default

If no context window is specified, the agent uses a fallback of **131,072 tokens**.

---

## Context Buffer

Safety margin for final messages:

```json
{
  "defaults": {
    "contextWindowBufferTokens": 256
  }
}
```

This ensures forced-final-turn messages fit within the remaining budget.

---

## Tokenizers

Supported tokenizer identifiers:

| Tokenizer | Models |
|-----------|--------|
| `tiktoken:gpt-4o` | OpenAI GPT-4o, GPT-4o-mini |
| `tiktoken:gpt-4` | OpenAI GPT-4 |
| `anthropic` | Anthropic Claude models |
| `gemini:google` | Google Gemini models |

Example:

```json
{
  "providers": {
    "openai": {
      "models": {
        "gpt-4o": {
          "tokenizer": "tiktoken:gpt-4o"
        }
      }
    }
  }
}
```

---

## Context Guard Behavior

### Before Each Turn

1. Estimate token footprint of conversation + pending tool results
2. Calculate remaining budget: `contextWindow - maxOutputTokens - buffer`
3. If projected usage exceeds budget, enter forced-final-turn

### Tool Response Handling

When a tool response would overflow:

1. Tool call rejected with: `(tool failed: context window budget exceeded)`
2. Session enters forced-final-turn flow
3. Agent produces final answer with available context

---

## Override via CLI

```bash
ai-agent --agent test.ai --override contextWindow=32000 "query"
```

---

## Override via Frontmatter

```yaml
---
models:
  - openai/gpt-4o
contextWindow: 64000
maxOutputTokens: 4096
---
```

---

## Telemetry

Context guard activations are exported as metrics:

| Metric | Description |
|--------|-------------|
| `ai_agent_context_guard_events_total` | Activation count by trigger/outcome |
| `ai_agent_context_guard_remaining_tokens` | Remaining budget at activation |

Labels:
- `provider`: LLM provider
- `model`: Model name
- `trigger`: What triggered the guard
- `outcome`: Result (forced_final, tool_rejected, etc.)

---

## Tool Output Storage

When tool responses are too large:

### Size-Based Storage

```yaml
---
toolResponseMaxBytes: 12288  # 12 KB default
---
```

Responses exceeding this are stored and replaced with handles.

### Token-Based Guard

Even if under byte limit, responses that would overflow tokens are stored.

### Handle System

Stored responses can be retrieved:

```json
{
  "tool": "tool_output",
  "arguments": {
    "handle": "session-abc123/file-xyz",
    "extract": "lines 1-100"
  }
}
```

---

## Debugging

### Enable Context Debug

```bash
CONTEXT_DEBUG=true ai-agent --agent test.ai "query"
```

Shows:
- Token estimates per turn
- Budget calculations
- Guard activation decisions

### Check Accounting

Tool accounting entries include:
- `projected_tokens`: Estimated tokens
- `limit_tokens`: Context limit
- `remaining_tokens`: Remaining budget

---

## Best Practices

### Set Appropriate Limits

```yaml
---
models:
  - openai/gpt-4o
maxOutputTokens: 4096      # Reserve for output
contextWindow: 100000      # Leave headroom
---
```

### Plan for Large Responses

```yaml
---
toolResponseMaxBytes: 25000  # Higher limit for research agents
maxTurns: 20
---
```

### Monitor Token Usage

Use telemetry to track:
- How often guards activate
- Which tools trigger guards
- Remaining budget patterns

---

## See Also

- [Configuration](Configuration) - Overview
- [Operations - Tool Output Handles](Operations-Tool-Output) - Handle system
- [docs/specs/context-management.md](../docs/specs/context-management.md) - Technical spec
