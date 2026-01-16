# Behavior Configuration

Configure agent execution limits and sampling parameters: turns, retries, timeouts, temperature, and token limits.

---

## Table of Contents

- [Overview](#overview) - What behavior configuration controls
- [Quick Example](#quick-example) - Basic behavior configuration
- [Execution Limits](#execution-limits) - maxTurns, maxRetries, maxToolCallsPerTurn
- [Timeouts](#timeouts) - llmTimeout, toolTimeout
- [Sampling Parameters](#sampling-parameters) - temperature, topP, topK
- [Output Limits](#output-limits) - maxOutputTokens, repeatPenalty
- [Response Caching](#response-caching) - cache TTL
- [Common Patterns](#common-patterns) - Typical behavior configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Behavior configuration controls:
- **Execution limits**: How many turns and retries before stopping
- **Timeouts**: How long to wait for LLM and tool responses
- **Sampling**: Creativity and randomness of responses
- **Output**: Response length and repetition handling
- **Caching**: Response cache for cost optimization

**User questions answered**: "How do I limit turns?" / "How do I adjust temperature?" / "How do I set timeouts?"

---

## Quick Example

Basic limits:

```yaml
---
description: Standard assistant
models:
  - openai/gpt-4o
maxTurns: 10
maxRetries: 3
temperature: 0.3
---
```

Production configuration:

```yaml
---
description: Production agent
models:
  - anthropic/claude-sonnet-4-20250514
  - openai/gpt-4o
maxTurns: 15
maxToolCallsPerTurn: 10
maxRetries: 5
llmTimeout: 5m
toolTimeout: 2m
temperature: 0.2
maxOutputTokens: 8192
cache: 1h
---
```

---

## Execution Limits

### maxTurns

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |

**Description**: Maximum number of LLM turns (request-response cycles) before the agent must complete.

**What it affects**:
- Prevents infinite loops in agentic behavior
- On the final turn, tools are disabled and agent is forced to provide a final answer
- Higher values allow more complex multi-step tasks
- Directly impacts cost and execution time

**Example**:
```yaml
---
maxTurns: 15    # Allow more tool-using turns
---

---
maxTurns: 5     # Limit for simple tasks
---

---
maxTurns: 25    # Complex multi-step workflows
---
```

**How Turns Work**:
1. Turn 1: LLM receives prompt, may call tools or respond
2. Turn 2-N: LLM receives tool results, continues work
3. Final turn: Tools disabled, LLM must produce final answer

**Notes**:
- If agent consistently hits `maxTurns` without completing:
  - Simplify the task
  - Improve the prompt
  - Increase the limit
  - Use sub-agents for complex sub-tasks

---

### maxToolCallsPerTurn

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |

**Description**: Maximum number of tools the agent can call in a single turn.

**What it affects**:
- Caps parallel tool usage per turn
- Prevents excessive tool calls from a single LLM response
- Affects throughput and cost

**Example**:
```yaml
---
maxToolCallsPerTurn: 20    # Allow more parallel tools
---

---
maxToolCallsPerTurn: 5     # Limit parallel execution
---
```

**Notes**:
- LLMs can request multiple tool calls in one response
- This setting limits how many are executed
- Excess calls are queued for next turn

---

### maxRetries

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `5` |
| Valid values | `0` or greater |

**Description**: How many times to retry when LLM calls fail. Goes through all fallback models before giving up.

**What it affects**:
- Resilience to transient failures
- Rate limit handling
- Fallback model utilization
- Total wait time on failures

**Example**:
```yaml
---
maxRetries: 3     # Standard retry count
---

---
maxRetries: 10    # More retries for critical tasks
---

---
maxRetries: 0     # No retries (fail immediately)
---
```

**Retry Behavior**:
1. Primary model fails
2. Try next model in fallback chain
3. If all models fail, wait and retry from primary
4. Repeat until `maxRetries` exhausted
5. Session fails with error

**Notes**:
- Retries cycle through all configured models
- Rate limit errors include backoff wait
- Network errors retry immediately

---

## Timeouts

### llmTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` (duration) |
| Default | `600000` (10 minutes) |
| Valid values | Positive integer or duration string |

**Description**: How long to wait for the LLM to respond before timing out.

**What it affects**:
- Maximum wait time per LLM request
- For streaming: timeout resets on each received chunk (inactivity timeout)
- Triggers retry/fallback on timeout

**Example**:
```yaml
---
llmTimeout: 120000    # 2 minutes in milliseconds
---

---
llmTimeout: 5m        # 5 minutes as duration string
---

---
llmTimeout: 30s       # 30 seconds for fast responses
---
```

**Duration String Format**:

| Unit | Suffix | Example |
|------|--------|---------|
| Milliseconds | `ms` | `500ms` |
| Seconds | `s` | `30s` |
| Minutes | `m` | `5m` |
| Hours | `h` | `1h` |
| Days | `d` | `1d` |

**Notes**:
- For streaming, timeout is an inactivity timeout (resets on each chunk)
- Very short timeouts may cause unnecessary retries
- Very long timeouts delay error detection

---

### toolTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` (duration) |
| Default | `300000` (5 minutes) |
| Valid values | Positive integer or duration string |

**Description**: How long to wait for each tool call to complete.

**What it affects**:
- Maximum execution time per tool
- Applies to MCP tools, REST tools, and sub-agent calls
- Tool times out with error result

**Example**:
```yaml
---
toolTimeout: 60000    # 1 minute in milliseconds
---

---
toolTimeout: 2m       # 2 minutes
---

---
toolTimeout: 30s      # 30 seconds for fast tools
---
```

**Notes**:
- Same duration format as `llmTimeout`
- Tool timeout returns error to LLM, which can retry or use alternatives
- Sub-agent calls inherit this timeout unless they specify their own

---

## Sampling Parameters

### temperature

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `0.0` |
| Valid values | `0.0` to `2.0`, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Controls response creativity/randomness. Lower values produce more focused, deterministic outputs.

**What it affects**:
- Response variability
- Creativity vs determinism
- Reproducibility of results

**Example**:
```yaml
---
temperature: 0        # Most deterministic
---

---
temperature: 0.3      # Focused, factual responses
---

---
temperature: 0.7      # Balanced
---

---
temperature: 1.2      # More creative
---

---
temperature: null     # Let provider decide (don't send parameter)
---
```

**Guidelines**:

| Temperature | Use Case |
|-------------|----------|
| `0 - 0.3` | Factual tasks, code, analysis |
| `0.3 - 0.7` | General conversation, balanced |
| `0.7 - 1.0` | Creative writing, brainstorming |
| `1.0 - 2.0` | Highly creative, experimental |

**Notes**:
- Set to `null`, `none`, `off`, `unset`, or `default` to not send the parameter
- Some models/features ignore temperature

---

### topP

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `null` (not sent) |
| Valid values | `0.0` to `1.0`, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Nucleus sampling - limits token selection to the smallest set of tokens whose cumulative probability exceeds this threshold.

**What it affects**:
- Token selection diversity
- Alternative to temperature for controlling randomness
- Affects response variation

**Example**:
```yaml
---
topP: 0.9     # Consider tokens in top 90% probability mass
---

---
topP: 0.5     # More focused selection
---

---
topP: null    # Don't send (use provider default)
---
```

**Notes**:
- Generally, use either `temperature` OR `topP`, not both
- `null` means the parameter is not sent to the provider
- Lower values = more focused, higher values = more diverse

---

### topK

| Property | Value |
|----------|-------|
| Type | `integer` or `null` |
| Default | `null` (not sent) |
| Valid values | `1` or greater, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Limits token selection to the K most probable tokens.

**What it affects**:
- Token vocabulary per generation step
- Response diversity
- Provider-dependent support

**Example**:
```yaml
---
topK: 40      # Consider only top 40 tokens
---

---
topK: 10      # Very focused selection
---

---
topK: null    # Don't send (use provider default)
---
```

**Notes**:
- Not all providers support `topK`
- `null` means the parameter is not sent
- Combine carefully with `temperature` and `topP`

---

## Output Limits

### maxOutputTokens

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `4096` |
| Valid values | `1` or greater |

**Description**: Maximum number of tokens the model can generate per turn.

**What it affects**:
- Maximum response length
- Cost (longer responses use more tokens)
- Truncation behavior

**Example**:
```yaml
---
maxOutputTokens: 8192     # Allow longer responses
---

---
maxOutputTokens: 1024     # Enforce shorter responses
---

---
maxOutputTokens: 16384    # Very long outputs
---
```

**Notes**:
- Actual limit depends on model capabilities
- Very long outputs may hit context window limits
- Doesn't affect tool call size

---

### repeatPenalty

| Property | Value |
|----------|-------|
| Type | `number` or `null` |
| Default | `null` (not sent) |
| Valid values | `0.0` or greater, or `null`/`none`/`off`/`unset`/`default` |

**Description**: Penalizes repetitive text. Higher values reduce repetition.

**What it affects**:
- Text repetition in outputs
- Response variety
- Provider-dependent implementation

**Example**:
```yaml
---
repeatPenalty: 1.0    # No penalty (default behavior)
---

---
repeatPenalty: 1.5    # Moderate penalty
---

---
repeatPenalty: null   # Don't send (use provider default)
---
```

**Notes**:
- Provider-dependent implementation
- Some providers call this "frequency penalty"
- `null` means the parameter is not sent

---

## Response Caching

### cache

| Property | Value |
|----------|-------|
| Type | `number` (ms) or `string` |
| Default | Undefined (no caching) |
| Valid values | `off`, milliseconds, or duration string |

**Description**: Response cache TTL. Caches agent responses to avoid redundant LLM calls.

**What it affects**:
- Whether responses are cached
- How long cached responses remain valid
- Cost savings for repeated identical requests
- Response latency for cache hits

**Example**:
```yaml
---
cache: off        # Disable caching
---

---
cache: 300000     # 5 minutes in milliseconds
---

---
cache: 1h         # 1 hour
---

---
cache: 1d         # 1 day
---

---
cache: 5m         # 5 minutes
---
```

**Duration Units**:

| Unit | Suffix | Example |
|------|--------|---------|
| Milliseconds | `ms` | `500ms` |
| Seconds | `s` | `30s` |
| Minutes | `m` | `5m` |
| Hours | `h` | `1h` |
| Days | `d` | `1d` |
| Weeks | `w` | `1w` |
| Months | `mo` | `1mo` |
| Years | `y` | `1y` |

**Notes**:
- Cache key includes: prompt content, model, parameters
- Requires cache backend configuration in `.ai-agent.json`
- Cache hits skip LLM calls entirely

---

## Common Patterns

### Fast Q&A Agent

```yaml
---
description: Quick Q&A
models:
  - openai/gpt-4o-mini
maxTurns: 3
maxRetries: 2
llmTimeout: 30s
temperature: 0.3
maxOutputTokens: 1024
---
```

### Thorough Research Agent

```yaml
---
description: Deep research
models:
  - anthropic/claude-sonnet-4-20250514
maxTurns: 20
maxToolCallsPerTurn: 15
maxRetries: 5
llmTimeout: 5m
toolTimeout: 3m
temperature: 0.2
maxOutputTokens: 8192
---
```

### Creative Writing Agent

```yaml
---
description: Creative writer
models:
  - anthropic/claude-sonnet-4-20250514
maxTurns: 5
temperature: 0.9
topP: 0.95
maxOutputTokens: 4096
repeatPenalty: 1.3
---
```

### Production Agent with Caching

```yaml
---
description: Production FAQ bot
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
maxTurns: 5
maxRetries: 10
llmTimeout: 2m
temperature: 0
cache: 1h
---
```

### Sub-Agent with Tight Limits

```yaml
---
description: Quick lookup sub-agent
toolName: quick_lookup
models:
  - openai/gpt-4o-mini
maxTurns: 3
maxRetries: 2
llmTimeout: 30s
toolTimeout: 15s
temperature: 0
---
```

---

## Troubleshooting

### Agent Hits maxTurns Without Completing

**Problem**: Session ends with incomplete results.

**Causes**:
- Task too complex for turn limit
- Agent stuck in loop
- Poor prompt design

**Solutions**:
1. Increase `maxTurns`:
   ```yaml
   maxTurns: 25
   ```
2. Simplify the task
3. Use sub-agents for complex sub-tasks
4. Improve prompt clarity

### Frequent Timeouts

**Problem**: LLM or tool calls timing out often.

**Causes**:
- Timeout too short
- Model/tool genuinely slow
- Network issues

**Solutions**:
1. Increase timeouts:
   ```yaml
   llmTimeout: 5m
   toolTimeout: 3m
   ```
2. Add faster fallback models
3. Check network connectivity

### Inconsistent Responses

**Problem**: Same prompt gives very different answers.

**Cause**: High temperature/topP settings.

**Solution**: Lower sampling parameters:
```yaml
temperature: 0.1
topP: null
```

### Responses Too Short

**Problem**: Agent truncates responses.

**Cause**: `maxOutputTokens` too low.

**Solution**: Increase output limit:
```yaml
maxOutputTokens: 8192
```

### Repetitive Output

**Problem**: Agent repeats phrases or patterns.

**Cause**: Model tendency, no penalty.

**Solution**: Add repeat penalty:
```yaml
repeatPenalty: 1.5
```

### Cache Not Working

**Problem**: Repeated calls still hit LLM.

**Causes**:
- Cache not configured
- TTL too short
- Different prompts/parameters

**Solutions**:
1. Verify `cache` setting:
   ```yaml
   cache: 1h
   ```
2. Check cache backend in `.ai-agent.json`
3. Ensure identical prompts

### All Retries Exhausted

**Problem**: Agent fails after `maxRetries`.

**Causes**:
- All providers down
- Authentication issues
- Persistent errors

**Solutions**:
1. Increase `maxRetries`:
   ```yaml
   maxRetries: 10
   ```
2. Add more fallback models
3. Check provider status and credentials

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Agent-Files-Models](Agent-Files-Models) - Model selection and fallbacks
- [Configuration-Providers](Configuration-Providers) - Provider timeout configuration
- [CLI-Overrides](CLI-Overrides) - Runtime overrides (`--temperature`, `--max-turns`)
