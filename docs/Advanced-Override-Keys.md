# Override Keys

Runtime configuration overrides via `--override` for testing, debugging, and forced configuration.

---

## Table of Contents

- [Overview](#overview) - What override keys are and how they work
- [Quick Start](#quick-start) - Basic usage examples
- [Override Key Reference](#override-key-reference) - Complete list of all override keys
- [Multiple Overrides](#multiple-overrides) - Combining overrides
- [Priority](#priority) - How overrides interact with other configuration
- [Override vs Default](#override-vs-default) - When to use each mechanism
- [Discovery](#discovery) - Finding override handling in source
- [Stability](#stability) - What to expect
- [See Also](#see-also) - Related documentation

---

## Overview

The `--override` flag allows runtime configuration changes that:

- **Bypass normal config loading**: Override frontmatter, config files, CLI flags
- **Apply to all agents**: Including sub-agents (unlike frontmatter)
- **Take highest priority**: Nothing can override an override

**Primary use cases**:
- Testing and debugging
- Forcing specific behavior
- Temporary configuration changes

**Syntax**:
```bash
ai-agent --agent test.ai --override key=value "query"
```

---

## Quick Start

```bash
# Disable parallel tool execution
ai-agent --agent test.ai --override no-batch=true "query"

# Force context window size
ai-agent --agent test.ai --override contextWindow=65536 "query"

# Force reasoning level on all agents
ai-agent --agent test.ai --override reasoning=high "query"

# Multiple overrides
ai-agent --agent test.ai \
  --override no-batch=true \
  --override contextWindow=32768 \
  "query"
```

---

## Override Key Reference

### Model Configuration

#### models

| Property | Value |
|----------|-------|
| Type | `string` (comma-separated) |
| Default | From frontmatter/CLI |
| Example | `models=openai/gpt-4o,anthropic/claude-sonnet-4` |

**Description**: Override model selection for all agents.

```bash
ai-agent --agent test.ai --override models=openai/gpt-4o "query"
```

---

#### tools

| Property | Value |
|----------|-------|
| Type | `string` (comma-separated) |
| Default | From frontmatter |
| Example | `tools=github,slack` |

**Description**: Override available tools.

```bash
ai-agent --agent test.ai --override tools=github,filesystem "query"
```

---

#### agents

| Property | Value |
|----------|-------|
| Type | `string` (comma-separated paths) |
| Default | From frontmatter |
| Example | `agents=helper.ai,analyzer.ai` |

**Description**: Override sub-agent definitions.

```bash
ai-agent --agent test.ai --override agents=helper.ai "query"
```

---

### Sampling Parameters

#### temperature

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | `0.0` |
| Valid values | `0.0` to `2.0` |
| Example | `temperature=0.7` |

**Description**: Override response creativity/variance.

```bash
ai-agent --agent test.ai --override temperature=0.5 "query"
```

---

#### topP

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | Not sent (provider decides) |
| Valid values | `0.0` to `1.0` |
| Example | `topP=0.9` |

**Description**: Override token selection diversity.

```bash
ai-agent --agent test.ai --override topP=0.95 "query"
```

---

#### topK

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | Not sent (provider decides) |
| Valid values | `1` or greater |
| Example | `topK=40` |

**Description**: Limit token selection to K most probable.

```bash
ai-agent --agent test.ai --override topK=50 "query"
```

---

#### maxOutputTokens

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `4096` |
| Valid values | `1` or greater |
| Example | `maxOutputTokens=8192` |

**Description**: Override maximum response length per turn.

```bash
ai-agent --agent test.ai --override maxOutputTokens=16384 "query"
```

---

#### repeatPenalty

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | Not sent (provider decides) |
| Valid values | `0.0` or greater (`1.0` = off) |
| Example | `repeatPenalty=1.2` |

**Description**: Reduce repetitive text.

```bash
ai-agent --agent test.ai --override repeatPenalty=1.1 "query"
```

---

### Execution Limits

#### llmTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or duration string |
| Default | `600000` (10 minutes) |
| Valid values | Positive integer or duration (`5s`, `2m`) |
| Example | `llmTimeout=300000` |

**Description**: Override LLM response timeout.

```bash
ai-agent --agent test.ai --override llmTimeout=120000 "query"
```

---

#### toolTimeout

| Property | Value |
|----------|-------|
| Type | `number` (ms) or duration string |
| Default | `300000` (5 minutes) |
| Valid values | Positive integer or duration (`5s`, `2m`) |
| Example | `toolTimeout=60000` |

**Description**: Override tool execution timeout.

```bash
ai-agent --agent test.ai --override toolTimeout=60000 "query"
```

---

#### maxRetries

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `5` |
| Valid values | `0` or greater |
| Example | `maxRetries=3` |

**Description**: Override retry count on LLM failures.

```bash
ai-agent --agent test.ai --override maxRetries=10 "query"
```

---

#### maxTurns

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |
| Example | `maxTurns=25` |

**Description**: Override maximum tool-using turns.

```bash
ai-agent --agent test.ai --override maxTurns=50 "query"
```

---

#### maxToolCallsPerTurn

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `10` |
| Valid values | `1` or greater |
| Example | `maxToolCallsPerTurn=20` |

**Description**: Override maximum parallel tool calls per turn.

```bash
ai-agent --agent test.ai --override maxToolCallsPerTurn=5 "query"
```

---

#### toolResponseMaxBytes

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `12288` (12KB) |
| Valid values | `0` or greater |
| Example | `toolResponseMaxBytes=65536` |

**Description**: Override tool output size cap.

```bash
ai-agent --agent test.ai --override toolResponseMaxBytes=32768 "query"
```

---

### Context and Reasoning

#### contextWindow

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | From provider/model config |
| Valid values | Positive integer |
| Example | `contextWindow=65536` |

**Description**: Override context window size for all providers.

```bash
ai-agent --agent test.ai --override contextWindow=131072 "query"
```

**Use case**: Testing context guard behavior or working with custom models.

---

#### reasoning

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | `unset` |
| Valid values | `none`, `unset`, `inherit`, `minimal`, `low`, `medium`, `high` |
| Example | `reasoning=high` |

**Description**: Force reasoning level on all agents/sub-agents (ignores frontmatter).

```bash
ai-agent --agent test.ai --override reasoning=medium "query"
```

> **Note**: Use `--default-reasoning` instead if you only want to set fallback values.

---

#### reasoningTokens

| Property | Value |
|----------|-------|
| Type | `integer` or `string` |
| Default | Computed from reasoning level |
| Valid values | Positive integer, `disabled`, `off`, `none` |
| Example | `reasoningTokens=16000` |

**Description**: Explicit token budget for reasoning.

```bash
# Set explicit budget
ai-agent --agent test.ai --override reasoningTokens=8000 "query"

# Disable reasoning tokens
ai-agent --agent test.ai --override reasoningTokens=disabled "query"
```

---

#### interleaved

| Property | Value |
|----------|-------|
| Type | `boolean` or `string` |
| Default | From provider config |
| Valid values | `true`, `false`, or field name |
| Example | `interleaved=reasoning_content` |

**Description**: Control interleaved reasoning injection for OpenAI-compatible providers.

```bash
# Disable interleaved reasoning
ai-agent --agent test.ai --override interleaved=false "query"

# Enable with default field name
ai-agent --agent test.ai --override interleaved=true "query"

# Enable with custom field name
ai-agent --agent test.ai --override interleaved=reasoning_content "query"
```

---

### Caching and Streaming

#### cache

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | Off |
| Valid values | `off`, duration (`5m`, `1h`), milliseconds |
| Example | `cache=1h` |

**Description**: Override response cache TTL.

```bash
ai-agent --agent test.ai --override cache=30m "query"
```

---

#### caching

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | `full` |
| Valid values | `none`, `full` |
| Example | `caching=none` |

**Description**: Anthropic prompt caching mode.

```bash
ai-agent --agent test.ai --override caching=none "query"
```

---

#### stream

| Property | Value |
|----------|-------|
| Type | `boolean` |
| Default | `true` |
| Valid values | `true`, `false` |
| Example | `stream=false` |

**Description**: Override streaming mode.

```bash
ai-agent --agent test.ai --override stream=false "query"
```

---

### Concurrency

#### mcpInitConcurrency

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | Varies |
| Valid values | `1` or greater |
| Example | `mcpInitConcurrency=4` |

**Description**: Override MCP server initialization concurrency.

```bash
ai-agent --agent test.ai --override mcpInitConcurrency=2 "query"
```

---

### Debug Flags

#### no-batch

| Property | Value |
|----------|-------|
| Type | `boolean` |
| Default | `false` |
| Valid values | `true`, `false` |
| Example | `no-batch=true` |

**Description**: Disable `agent__batch` parallel tool execution.

```bash
ai-agent --agent test.ai --override no-batch=true "query"
```

**Effect**: Tools are executed sequentially instead of in parallel batches.

**Use case**: Debugging tool execution order or race conditions.

---

#### no-progress

| Property | Value |
|----------|-------|
| Type | `boolean` |
| Default | `false` |
| Valid values | `true`, `false` |
| Example | `no-progress=true` |

**Description**: Disable progress-only `task_status` tool behavior.

```bash
ai-agent --agent test.ai --override no-progress=true "query"
```

**Effect**: `task_status` tool may trigger final turn enforcement even in progress mode.

**Use case**: Testing final turn behavior.

---

## Multiple Overrides

Pass multiple `--override` flags:

```bash
ai-agent --agent test.ai \
  --override no-batch=true \
  --override contextWindow=32768 \
  --override temperature=0.5 \
  --override maxTurns=20 \
  "query"
```

---

## Priority

Override keys have highest priority in the configuration stack:

| Priority | Source |
|----------|--------|
| 1 (highest) | `--override key=value` |
| 2 | CLI flags (`--temperature`, etc.) |
| 3 | Frontmatter |
| 4 | Config file |
| 5 (lowest) | System defaults |

---

## Override vs Default

| Mechanism | Behavior | Use When |
|-----------|----------|----------|
| `--override key=value` | Forces value, ignores all other config | Testing/debugging, forced behavior |
| `--default-reasoning X` | Sets fallback when frontmatter omits | Operational defaults |
| `defaults.X` in config | Sets fallback from config file | System-wide defaults |

**Example comparison**:

```bash
# Force reasoning on ALL agents (stomps frontmatter)
ai-agent --agent test.ai --override reasoning=high "query"

# Set default reasoning (only when frontmatter omits it)
ai-agent --agent test.ai --default-reasoning high "query"
```

---

## Discovery

Override keys are processed in:

| File | Purpose |
|------|---------|
| `src/cli.ts` | CLI parsing and validation |
| `src/agent-loader.ts` | Application to loaded agents |

### Find All Override Keys

```bash
# In cli.ts, look for the switch statement handling override keys
grep -A2 "case '" src/cli.ts | grep -E "case '(models|tools|temperature)"
```

### Current Override Keys (Source of Truth)

From `src/options-registry.ts`:
```
models, tools, agents, temperature, topP, topK, maxOutputTokens,
repeatPenalty, llmTimeout, toolTimeout, maxRetries, maxTurns,
maxToolCallsPerTurn, toolResponseMaxBytes, mcpInitConcurrency,
stream, interleaved, reasoning, reasoningTokens, caching,
contextWindow, no-batch, no-progress
```

---

## Stability

| Aspect | Status |
|--------|--------|
| **Internal** | Primarily for development/testing |
| **Undocumented** | Not part of stable API |
| **May Change** | Without deprecation warnings |

> **Tip:** For production use, prefer frontmatter or config file settings over overrides.

---

## See Also

- [Configuration](Configuration) - Standard configuration
- [CLI-Overrides](CLI-Overrides) - CLI parameter reference
- [Agent-Files-Behavior](Agent-Files-Behavior) - Frontmatter behavior settings
- [Advanced-Extended-Reasoning](Advanced-Extended-Reasoning) - Reasoning configuration
