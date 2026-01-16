# Model Configuration

Configure which LLMs your agent uses, including fallback chains, reasoning modes, and caching.

---

## Table of Contents

- [Overview](#overview) - What model configuration controls
- [Quick Example](#quick-example) - Basic model configuration
- [Configuration Reference](#configuration-reference) - All model-related keys
- [Fallback Chains](#fallback-chains) - Automatic model failover
- [Reasoning Configuration](#reasoning-configuration) - Extended thinking modes
- [Prompt Caching](#prompt-caching) - Anthropic prompt cache settings
- [Common Patterns](#common-patterns) - Typical model configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Model configuration controls:

- **Which LLMs** handle requests (`models`)
- **Fallback order** when a model fails
- **Extended reasoning** for complex problems (`reasoning`, `reasoningTokens`)
- **Prompt caching** for cost optimization (`caching`)

**User questions answered**: "How do I change the model?" / "How do I set up fallbacks?" / "How do I enable extended thinking?"

---

## Quick Example

Single model:

```yaml
---
description: Simple assistant
models: openai/gpt-4o
---
```

Fallback chain:

```yaml
---
description: Resilient assistant
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
  - openai/gpt-4o-mini
---
```

With reasoning enabled:

```yaml
---
description: Complex problem solver
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: high
reasoningTokens: 32000
---
```

---

## Configuration Reference

### models

| Property     | Value                  |
| ------------ | ---------------------- |
| Type         | `string` or `string[]` |
| Default      | None                   |
| Required     | **Yes**                |
| Valid values | `provider/model` pairs |

**Description**: List of provider/model pairs to use. The agent tries each in order if one fails (fallback chain).

**What it affects**:

- Which LLM(s) handle requests
- Fallback order on failures (rate limits, errors, timeouts)
- Cost and performance characteristics
- Available features (reasoning, tools, etc.)

**Format**: Always `provider/model` with exactly one `/`:

```yaml
# Single model (string)
models: openai/gpt-4o

# Multiple models (array) - fallback chain
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
  - openai/gpt-4o-mini
```

**Common Providers**:

| Provider     | Example Models                                           | Notes               |
| ------------ | -------------------------------------------------------- | ------------------- |
| `openai`     | `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`                   | Standard OpenAI API |
| `anthropic`  | `claude-sonnet-4-20250514`, `claude-3-5-sonnet-20241022` | Supports reasoning  |
| `google`     | `gemini-2.0-flash`, `gemini-1.5-pro`                     | Google AI           |
| `openrouter` | `anthropic/claude-3.5-sonnet`, `openai/gpt-4-turbo`      | Aggregator          |
| `ollama`     | `llama3.2:latest`, `mistral:latest`                      | Local models        |

**Notes**:

- Provider names must match entries in `.ai-agent.json` `providers` section
- First model is primary; others are fallbacks
- Models inherit capabilities from their provider configuration

---

### reasoning

| Property     | Value                                                                     |
| ------------ | ------------------------------------------------------------------------- |
| Type         | `string`                                                                  |
| Default      | Not sent (provider default)                                               |
| Valid values | `none`, `unset`, `default`, `inherit`, `minimal`, `low`, `medium`, `high` |

**Description**: Controls extended thinking/reasoning effort for models that support it.

**What it affects**:

- Whether the model uses extended reasoning (chain-of-thought)
- Reasoning effort level and token budget
- Cost and latency (higher reasoning = more expensive/slower)
- Quality of complex problem solving

**Reasoning Levels**:

| Level     | Description                | Use Case                     |
| --------- | -------------------------- | ---------------------------- |
| `none`    | Disable reasoning          | Fast responses, simple tasks |
| `unset`   | Same as `none`             | Explicitly disable           |
| `minimal` | Light reasoning            | Quick analysis               |
| `low`     | Basic reasoning            | Moderate complexity          |
| `medium`  | Moderate reasoning         | Complex analysis             |
| `high`    | Maximum reasoning          | Difficult problems           |
| `default` | Inherit from parent/global | Use configured default       |
| `inherit` | Same as `default`          | Use configured default       |

**Example**:

```yaml
---
reasoning: none # Disable - fast responses
---
---
reasoning: medium # Moderate reasoning effort
---
---
reasoning: high # Maximum reasoning for hard problems
---
```

**Notes**:

- `none` or `unset` explicitly disables reasoning
- `default` or `inherit` (or omitting the key) uses global fallback from `--default-reasoning` or `defaults.reasoning`
- Reasoning is model-dependent:
  - **Anthropic**: Uses thinking mode
  - **OpenAI**: Uses reasoning models (o1, o3, etc.)
  - **Others**: May not support reasoning

---

### reasoningTokens

| Property     | Value                                                     |
| ------------ | --------------------------------------------------------- |
| Type         | `number` or `string`                                      |
| Default      | Provider decides                                          |
| Valid values | Positive integer, `0`, `"disabled"`, `"off"`, or `"none"` |

**Description**: Token budget for extended reasoning (primarily Anthropic thinking mode).

**What it affects**:

- Maximum tokens the model can use for internal reasoning
- `0`, `"disabled"`, `"off"`, or `"none"` disable reasoning (set to `null` internally)
- Higher values allow more thorough analysis but cost more

**Example**:

```yaml
---
reasoningTokens: 16000 # Allow up to 16K reasoning tokens
---
---
reasoningTokens: 32000 # Maximum for complex problems
---
---
reasoningTokens: 0 # Disable reasoning
---
```

**Notes**:

- Only meaningful when `reasoning` is enabled
- Provider may have minimum/maximum limits
- More tokens = better reasoning but higher cost and latency

---

### caching

| Property     | Value          |
| ------------ | -------------- |
| Type         | `string`       |
| Default      | `full`         |
| Valid values | `none`, `full` |

**Description**: Controls Anthropic prompt caching behavior.

**What it affects**:

- Whether Anthropic's prompt cache is used
- Cost optimization for repeated prompts
- Only affects Anthropic provider

**Example**:

```yaml
---
caching: full # Enable prompt caching (default)
---
---
caching: none # Disable prompt caching
---
```

**How Prompt Caching Works**:

- Anthropic caches prompt prefixes that are reused
- Subsequent requests with the same prefix are cheaper
- Useful for agents with long system prompts called repeatedly

**Notes**:

- Only affects Anthropic provider
- `full` enables cache reuse for repeated prompt prefixes
- Other providers ignore this setting

---

## Fallback Chains

Fallback chains provide resilience against model failures. When the primary model fails (rate limit, error, timeout), the agent automatically tries the next model.

### How Fallback Works

```yaml
models:
  - anthropic/claude-sonnet-4-20250514 # First model in chain
  - openai/gpt-4o # Second model in chain
  - openai/gpt-4o-mini # Third model in chain
```

**Retry mechanism**:

- For each turn, system cycles through all configured provider/model pairs
- Up to `maxRetries` attempts are made per turn (default: 3)
- Each attempt tries the next model in the sequence (wrapping around if needed)
- Example: With models [A, B, C] and maxRetries=3, a single failing turn could try: A, B, C, A, B, C, A, B before giving up

**Fallback triggers**:

1. Rate limit errors (429)
2. Server errors (500+)
3. Timeout (no response within `llmTimeout`; default: 600000ms / 10 minutes)
4. Authentication errors
5. Model-specific errors

**Fallback does NOT trigger for**:

- Successful responses (even if low quality)
- Validation errors in your configuration
- Network issues that affect all providers

### Recommended Patterns

**High availability**:

```yaml
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
  - openai/gpt-4o-mini
```

**Cost optimization** (try cheap first):

```yaml
models:
  - openai/gpt-4o-mini
  - openai/gpt-4o
  - anthropic/claude-sonnet-4-20250514
```

**Quality optimization** (try best first):

```yaml
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
```

**Local with cloud fallback**:

```yaml
models:
  - ollama/llama3.2:latest
  - openai/gpt-4o-mini
```

---

## Reasoning Configuration

Extended reasoning enables models to "think" before responding, improving quality for complex problems.

### When to Use Reasoning

**Good for**:

- Complex analysis
- Multi-step problems
- Code generation and review
- Research synthesis
- Decision making

**Not needed for**:

- Simple Q&A
- Data extraction
- Formatting tasks
- Quick lookups

### Configuring Reasoning

**Basic reasoning**:

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: medium
---
```

**Maximum reasoning with budget**:

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: high
reasoningTokens: 32000
---
```

**Disable reasoning for speed**:

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: none
---
```

### Provider Support

| Provider  | Reasoning Support             |
| --------- | ----------------------------- |
| Anthropic | Full (thinking mode)          |
| OpenAI    | Via reasoning models (o1, o3) |
| Google    | Limited                       |
| Ollama    | Model-dependent               |

---

## Prompt Caching

Anthropic's prompt caching reduces costs for agents called repeatedly with the same system prompt.

### How It Works

1. First request: Full prompt sent, cached by Anthropic
2. Subsequent requests: Only new content sent, prefix retrieved from cache
3. Significant cost savings for agents with long system prompts

### Configuration

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
caching: full # Enable (default)
---
```

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
caching: none # Disable if needed
---
```

### When to Disable

Disable caching when:

- System prompt changes frequently
- Using dynamic prompt content
- Debugging prompt issues

---

## Common Patterns

### Simple Assistant

```yaml
---
description: General assistant
models:
  - openai/gpt-4o
---
```

### Resilient Production Agent

```yaml
---
description: Production agent with fallbacks
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
  - openai/gpt-4o-mini
maxRetries: 5
---
```

### Complex Problem Solver

```yaml
---
description: Solves complex problems with reasoning
models:
  - anthropic/claude-sonnet-4-20250514
reasoning: high
reasoningTokens: 32000
temperature: 0.1
---
```

### Cost-Optimized Agent

```yaml
---
description: Budget-friendly agent
models:
  - openai/gpt-4o-mini
  - openai/gpt-4o
caching: full
cache: 1h
---
```

### Local-First Agent

```yaml
---
description: Uses local model with cloud fallback
models:
  - ollama/llama3.2:latest
  - openai/gpt-4o-mini
---
```

---

## Troubleshooting

### "Invalid provider/model pair 'gpt-4o'"

**Problem**: Model specified without provider prefix.

**Cause**: Models must include provider prefix.

**Solution**: Use `provider/model` format:

```yaml
# Wrong
models: gpt-4o

# Correct
models: openai/gpt-4o
```

### "Provider 'xyz' not configured"

**Problem**: Using a provider not defined in `.ai-agent.json`.

**Cause**: Provider must exist in configuration.

**Solution**: Add provider to `.ai-agent.json`:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### "Invalid reasoning level"

**Problem**: Using an unsupported reasoning value.

**Cause**: Must use one of the valid values.

**Solution**: Use: `none`, `unset`, `default`, `inherit`, `minimal`, `low`, `medium`, `high`

### "Invalid caching mode"

**Problem**: Using unsupported caching value.

**Cause**: Must use `none` or `full`.

**Solution**:

```yaml
caching: full   # Enable
caching: none   # Disable
```

### Model Keeps Timing Out

**Problem**: LLM requests consistently time out.

**Cause**: Model too slow or timeout too short.

**Solutions**:

1. Increase timeout (default: 10 minutes / 600000ms):
   ```yaml
   llmTimeout: 5m
   ```
2. Add faster fallback:
   ```yaml
   models:
     - anthropic/claude-sonnet-4-20250514
     - openai/gpt-4o-mini # Faster fallback
   ```
3. Reduce reasoning if enabled:
   ```yaml
   reasoning: low
   reasoningTokens: 8000
   ```

### All Models Failing

**Problem**: Fallback chain exhausted without success.

**Cause**: All providers failing (network, auth, etc.).

**Solutions**:

1. Check API keys in environment
2. Verify provider configuration in `.ai-agent.json`
3. Increase `maxRetries`:
   ```yaml
   maxRetries: 10
   ```
4. Check network connectivity

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Agent-Files-Behavior](Agent-Files-Behavior) - Sampling parameters (temperature, topP)
- [Configuration-Providers](Configuration-Providers) - Provider setup in .ai-agent.json
- [CLI-Overrides](CLI-Overrides) - Runtime model overrides (`--model`)
