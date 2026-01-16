# Context Window Configuration

Configure token budgets and context guards.

---

## Table of Contents

- [Overview](#overview) - Context management purpose
- [Context Window Configuration](#context-window-configuration) - Setting token limits
- [Tokenizers](#tokenizers) - Token counting configuration
- [Context Buffer](#context-buffer) - Safety margin for final messages
- [Context Guard Behavior](#context-guard-behavior) - Overflow protection
- [Tool Output Storage](#tool-output-storage) - Handling large responses
- [Configuration Reference](#configuration-reference) - All context options
- [Debugging](#debugging) - Token tracking and troubleshooting
- [Best Practices](#best-practices) - Optimization guidelines
- [See Also](#see-also) - Related documentation

---

## Overview

The context guard prevents token overflow by:

1. **Tracking tokens**: Counting usage per turn
2. **Projecting overflow**: Estimating if next action exceeds budget
3. **Forcing completion**: Triggering final response when limits approach
4. **Rejecting tools**: Blocking tool calls that would overflow

Without context management, sessions fail when the context window is exceeded.

---

## Context Window Configuration

### Provider-Level Default

Set context window for all models from a provider:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "contextWindow": 128000
    }
  }
}
```

### Model-Level Override

Override for specific models:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "contextWindow": 128000,
      "models": {
        "gpt-4o": {
          "contextWindow": 128000
        },
        "gpt-4o-mini": {
          "contextWindow": 128000
        },
        "o1": {
          "contextWindow": 200000
        }
      }
    }
  }
}
```

### Agent-Level Override

Override in agent frontmatter:

```yaml
---
models:
  - openai/gpt-4o
contextWindow: 64000
maxOutputTokens: 4096
---
```

### CLI Override

Override at runtime:

```bash
ai-agent --agent test.ai --override contextWindow=32000 "query"
```

### Default Value

If no context window is specified, the agent uses a fallback of **131,072 tokens**.

---

## Tokenizers

Configure how tokens are counted for each provider/model.

### Tokenizer Configuration

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "tokenizer": "tiktoken:gpt-4o",
      "models": {
        "gpt-4o": {
          "tokenizer": "tiktoken:gpt-4o"
        },
        "gpt-4": {
          "tokenizer": "tiktoken:gpt-4"
        }
      }
    },
    "anthropic": {
      "type": "anthropic",
      "tokenizer": "anthropic"
    },
    "google": {
      "type": "google",
      "tokenizer": "google:gemini"
    }
  }
}
```

### Supported Tokenizers

| Tokenizer         | Provider  | Models              |
| ----------------- | --------- | ------------------- |
| `tiktoken:gpt-4o` | OpenAI    | GPT-4o, GPT-4o-mini |
| `tiktoken:gpt-4`  | OpenAI    | GPT-4, GPT-4-turbo  |
| `anthropic`       | Anthropic | All Claude models   |
| `google:gemini`   | Google    | All Gemini models   |

### Tokenizer Selection

- Use provider-specific tokenizers for accurate counts
- Mismatched tokenizers cause under/over-estimation
- Default tokenizer may be inaccurate for some models

---

## Context Buffer

Safety margin reserved for final messages.

```json
{
  "defaults": {
    "contextWindowBufferTokens": 8192
  }
}
```

### Purpose

The buffer ensures:

- Forced-final-turn messages fit within budget
- System messages for error handling have space
- Small variations in token counting don't cause failures

### How It Works

```
available_budget = contextWindow - maxOutputTokens - buffer
```

### Buffer Reference

| Property                    | Type     | Default | Description           |
| --------------------------- | -------- | ------- | --------------------- |
| `contextWindowBufferTokens` | `number` | `8192`  | Reserved token buffer |

---

## Context Guard Behavior

### Before Each Turn

1. **Estimate footprint**: Count conversation tokens + pending tool results
2. **Calculate budget**: `contextWindow - maxOutputTokens - buffer`
3. **Check projection**: If projected usage exceeds budget, trigger guard

### Guard Triggers

| Trigger              | Action                       |
| -------------------- | ---------------------------- |
| Projected overflow   | Enter forced-final-turn mode |
| Tool result overflow | Reject tool call with error  |
| Max turns reached    | Force final response         |

### Forced-Final-Turn Flow

When the guard activates:

1. Agent receives notification that context is nearly exhausted
2. Agent must produce a final response with available context
3. No further tool calls are allowed
4. Session completes gracefully

### Tool Response Handling

When a tool response would overflow:

```
(tool failed: context window budget exceeded)
```

The tool call is rejected, and the session enters forced-final-turn.

---

## Tool Output Storage

Handle large tool responses that would overflow context.

### Size-Based Storage

```yaml
---
toolResponseMaxBytes: 12288 # 12 KB default
---
```

Responses exceeding this size are stored and replaced with handles.

### Configuration

| Property               | Type     | Default | Description                |
| ---------------------- | -------- | ------- | -------------------------- |
| `toolResponseMaxBytes` | `number` | `12288` | Max response size in bytes |

### How Storage Works

1. Tool returns large response
2. Response stored in session storage
3. Handle returned to LLM instead:

```
Tool output is too large (45678 bytes, 1200 lines, 11445 tokens).
Call tool_output(handle = "session-abc123/file-uuid", extract = "what to extract").
The handle is a relative path under tool_output root.
Provide precise and detailed instructions in `extract` about what you are looking for.
```

### Retrieving Stored Output

Use the `tool_output` tool to retrieve stored responses:

```json
{
  "tool": "tool_output",
  "arguments": {
    "handle": "session-abc123/file-uuid",
    "extract": "Extract all lines containing 'error' or provide a summary of the output"
  }
}
```

The `extract` parameter accepts natural language instructions describing what you need from the stored output. Be specific and include keys, fields, or sections if known.

### Token-Based Guard

Even if under byte limit, responses that would overflow tokens are stored.

---

## Configuration Reference

### Provider Context Options

```json
{
  "providers": {
    "<name>": {
      "contextWindow": "number",
      "tokenizer": "string",
      "models": {
        "<model>": {
          "contextWindow": "number",
          "tokenizer": "string"
        }
      }
    }
  }
}
```

### Default Context Options

```json
{
  "defaults": {
    "contextWindowBufferTokens": "number",
    "maxOutputTokens": "number",
    "toolResponseMaxBytes": "number"
  }
}
```

### Frontmatter Context Options

```yaml
---
contextWindow: "number"
maxOutputTokens: "number"
toolResponseMaxBytes: "number"
---
```

### All Context Properties

| Location    | Property                    | Type     | Default        | Description                 |
| ----------- | --------------------------- | -------- | -------------- | --------------------------- |
| Provider    | `contextWindow`             | `number` | `131072`       | Provider-wide context limit |
| Provider    | `tokenizer`                 | `string` | approximate    | Token counting method       |
| Model       | `contextWindow`             | `number` | Provider value | Model-specific limit        |
| Model       | `tokenizer`                 | `string` | Provider value | Model-specific tokenizer    |
| Defaults    | `contextWindowBufferTokens` | `number` | `8192`         | Safety buffer               |
| Defaults    | `maxOutputTokens`           | `number` | `4096`         | Max output tokens           |
| Defaults    | `toolResponseMaxBytes`      | `number` | `12288`        | Max tool response size      |
| Frontmatter | `contextWindow`             | `number` | Provider value | Agent-specific limit        |
| Frontmatter | `maxOutputTokens`           | `number` | Defaults value | Agent output limit          |
| Frontmatter | `toolResponseMaxBytes`      | `number` | Defaults value | Agent tool response limit   |

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

### Verbose Mode

```bash
ai-agent --agent test.ai --verbose "query"
```

Logs context-related decisions.

### Check Accounting

Accounting entries include context metrics:

```json
{
  "type": "llm",
  "inputTokens": 15230,
  "outputTokens": 1456,
  "contextWindow": 128000,
  "remainingTokens": 111314
}
```

### Telemetry Metrics

| Metric                                    | Description                    |
| ----------------------------------------- | ------------------------------ |
| `ai_agent_context_guard_events_total`     | Guard activation count         |
| `ai_agent_context_guard_remaining_tokens` | Remaining budget at activation |

Labels:

- `provider`: LLM provider
- `model`: Model name
- `trigger`: What triggered the guard
- `outcome`: Result (forced_final, tool_rejected)

### Common Issues

**Session ends abruptly:**

- Check if context guard triggered
- Review token usage in accounting
- Consider increasing `contextWindow` or reducing tool responses

**Tool calls rejected:**

- Tool response would overflow context
- Use `toolResponseMaxBytes` to store large responses
- Consider chunked/paginated tool responses

**Inaccurate token counts:**

- Verify tokenizer matches model
- Different tokenizers produce different counts

---

## Best Practices

### Set Appropriate Limits

```yaml
---
models:
  - openai/gpt-4o
contextWindow: 100000 # Leave 28K headroom from 128K
maxOutputTokens: 4096 # Reserve for response
toolResponseMaxBytes: 25000 # Allow larger tool responses
---
```

### Plan for Large Responses

For research agents that process large documents:

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
toolResponseMaxBytes: 50000
maxTurns: 20
contextWindow: 180000
---
```

### Monitor Token Usage

Use telemetry to track:

- How often guards activate
- Which tools trigger guards
- Remaining budget patterns

### Optimize for Long Sessions

1. **Reduce system prompt size**: Concise prompts leave more room
2. **Use tool output storage**: Store large responses externally
3. **Paginate tool results**: Request data in chunks
4. **Monitor turn count**: More turns = more context consumed

### Model Selection by Context Need

| Use Case            | Recommended Model | Context Window |
| ------------------- | ----------------- | -------------- |
| Simple queries      | gpt-4o-mini       | 128K           |
| Code analysis       | claude-sonnet-4   | 200K           |
| Large document      | claude-sonnet-4   | 200K           |
| Multi-turn research | gpt-4o            | 128K           |

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [Providers](Configuration-Providers) - Provider configuration
- [Parameters](Configuration-Parameters) - All parameters reference
- [Operations - Tool Output](Operations-Tool-Output) - Handle system documentation
