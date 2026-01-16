# CLI: Runtime Overrides

How to override agent configuration at runtime. Covers model selection, sampling parameters, limits, and the `--override` mechanism.

---

## Table of Contents

- [Overview](#overview) - Override hierarchy and scopes
- [Model Selection](#model-selection) - The `--models` flag
- [Sampling Parameters](#sampling-parameters) - Temperature, top-p, top-k
- [Limits and Timeouts](#limits-and-timeouts) - Turns, retries, timeouts
- [Output Control](#output-control) - Tokens, format, streaming
- [Cache Settings](#cache-settings) - Response cache TTL
- [The Override Flag](#the-override-flag) - Universal `--override` mechanism
- [Configuration File](#configuration-file) - The `--config` flag
- [Quick Reference](#quick-reference) - All override flags in one table
- [See Also](#see-also) - Related pages

---

## Overview

CLI flags override agent frontmatter, which overrides configuration files.

**Precedence (highest to lowest):**

1. `--override key=value` (applies to all agents)
2. Direct CLI flags (e.g., `--temperature`)
3. Agent frontmatter
4. Configuration file defaults
5. Built-in defaults

**Override Scopes:**

| Scope          | Description                          | Example Flags                                    |
| -------------- | ------------------------------------ | ------------------------------------------------ |
| Master Only    | Affects only the top-level agent     | `--models`, `--tools`, `--agents`                |
| Master Default | Inherited by sub-agents when unset   | `--temperature`, `--max-turns`                   |
| All Agents     | Applies to master and all sub-agents | `--override`, `--verbose`, `--stream`, `--cache` |
| Global         | Application-level settings           | `--config`, `--dry-run`                          |

---

## Model Selection

### The `--models` Flag

| Property | Value                                  |
| -------- | -------------------------------------- |
| Flag     | `--models <list>`                      |
| Scope    | Master Only                            |
| Format   | Comma-separated `provider/model` pairs |

**Description**: Specify which LLM models to use. Multiple models create a fallback chain.

**Examples**:

```bash
# Single model
ai-agent --agent chat.ai --models openai/gpt-4o "Hello"

# Fallback chain (tries in order)
ai-agent --agent chat.ai --models "openai/gpt-4o,anthropic/claude-3-5-sonnet-20241022" "Hello"

# Local model
ai-agent --agent chat.ai --models ollama/llama3.2 "Hello"
```

**Provider format**: `provider/model-name`

- `openai/gpt-4o`
- `anthropic/claude-3-5-sonnet-20241022`
- `ollama/llama3.2`
- `openrouter/meta-llama/llama-3.1-70b-instruct`

---

## Sampling Parameters

### Temperature

| Property | Value               |
| -------- | ------------------- |
| Flag     | `--temperature <n>` |
| Scope    | Master Default      |
| Range    | `0.0` to `2.0`      |
| Default  | `0.0`               |

**Description**: Controls response creativity/variance.

- `0` = Deterministic, focused responses
- `0.7` = Balanced creativity
- `1.5+` = High creativity, may be incoherent

```bash
# Deterministic (good for code, facts)
ai-agent --agent code.ai --temperature 0 "Write a sort function"

# Creative (good for writing, brainstorming)
ai-agent --agent writer.ai --temperature 0.9 "Write a poem"

# Let provider decide (clear the override)
ai-agent --agent chat.ai --temperature default "Hello"
```

### Top-P (Nucleus Sampling)

| Property | Value                       |
| -------- | --------------------------- |
| Flag     | `--top-p <n>`               |
| Scope    | Master Default              |
| Range    | `0.0` to `1.0`              |
| Default  | Not sent (provider decides) |

**Description**: Token selection diversity. Lower values = more focused.

```bash
# Very focused
ai-agent --agent chat.ai --top-p 0.5 "Precise answer"

# More diverse
ai-agent --agent chat.ai --top-p 0.95 "Creative answer"
```

### Top-K

| Property | Value                       |
| -------- | --------------------------- |
| Flag     | `--top-k <n>`               |
| Scope    | Master Default              |
| Range    | `1` to unlimited (integer)  |
| Default  | Not sent (provider decides) |

**Description**: Limits token selection to K most probable tokens.

```bash
ai-agent --agent chat.ai --top-k 40 "Generate text"
```

### Repeat Penalty

| Property | Value                  |
| -------- | ---------------------- |
| Flag     | `--repeat-penalty <n>` |
| Scope    | Master Default         |
| Range    | `0.0` and above        |
| Default  | Not sent               |

**Description**: Reduces repetitive text. `1.0` = off, higher = stronger penalty.

```bash
ai-agent --agent writer.ai --repeat-penalty 1.2 "Write without repetition"
```

---

## Limits and Timeouts

### Max Turns

| Property | Value             |
| -------- | ----------------- |
| Flag     | `--max-turns <n>` |
| Scope    | Master Default    |
| Range    | `1` and above     |
| Default  | `10`              |

**Description**: Maximum tool-using turns before forcing a final answer.

```bash
# Quick task (few turns)
ai-agent --agent quick.ai --max-turns 3 "Simple query"

# Complex task (many turns)
ai-agent --agent research.ai --max-turns 25 "Deep research topic"
```

### Max Retries

| Property | Value               |
| -------- | ------------------- |
| Flag     | `--max-retries <n>` |
| Scope    | Master Default      |
| Range    | `0` and above       |
| Default  | `5`                 |

**Description**: Retry count when LLM calls fail. Cycles through fallback models.

```bash
# More resilient
ai-agent --agent api.ai --max-retries 10 "Critical task"

# Fail fast
ai-agent --agent test.ai --max-retries 1 "Quick test"
```

### Max Tool Calls Per Turn

| Property | Value                           |
| -------- | ------------------------------- |
| Flag     | `--max-tool-calls-per-turn <n>` |
| Scope    | Master Default                  |
| Range    | `1` and above                   |
| Default  | `10`                            |

**Description**: Maximum parallel tool calls in a single turn.

```bash
# Allow more parallel calls
ai-agent --agent batch.ai --max-tool-calls-per-turn 20 "Process many items"
```

### LLM Timeout

| Property | Value                                       |
| -------- | ------------------------------------------- |
| Flag     | `--llm-timeout-ms <ms>`                     |
| Scope    | Master Default                              |
| Format   | Milliseconds or duration (e.g., `5s`, `2m`) |
| Default  | `600000` (10 minutes)                       |

**Description**: How long to wait for LLM response.

```bash
# Quick timeout
ai-agent --agent fast.ai --llm-timeout-ms 30000 "Quick query"

# Long timeout for complex reasoning
ai-agent --agent reasoning.ai --llm-timeout-ms 5m "Complex problem"
```

### Tool Timeout

| Property | Value                    |
| -------- | ------------------------ |
| Flag     | `--tool-timeout-ms <ms>` |
| Scope    | Master Default           |
| Format   | Milliseconds or duration |
| Default  | `300000` (5 minutes)     |

**Description**: How long to wait for each tool execution.

```bash
# Long-running tools
ai-agent --agent compute.ai --tool-timeout-ms 10m "Run heavy computation"
```

---

## Output Control

### Max Output Tokens

| Property | Value                     |
| -------- | ------------------------- |
| Flag     | `--max-output-tokens <n>` |
| Scope    | Master Default            |
| Range    | `1` to model maximum      |
| Default  | `4096`                    |

**Description**: Maximum response length per turn.

```bash
# Longer responses
ai-agent --agent writer.ai --max-output-tokens 8192 "Write a long story"

# Shorter responses
ai-agent --agent concise.ai --max-output-tokens 500 "Brief answer"
```

### Tool Response Max Bytes

| Property | Value                           |
| -------- | ------------------------------- |
| Flag     | `--tool-response-max-bytes <n>` |
| Scope    | Master Default                  |
| Default  | `12288` (12KB)                  |

**Description**: Maximum tool output size kept in conversation. Larger outputs are stored externally.

```bash
# Allow larger inline responses
ai-agent --agent data.ai --tool-response-max-bytes 50000 "Fetch large dataset"
```

---

## Cache Settings

### Response Cache TTL

| Property | Value                |
| -------- | -------------------- |
| Flag     | `--cache <ttl>`      |
| Scope    | All Agents           |
| Default  | Not set (no caching) |

**Description**: Time-to-live for response caching (agent and tool responses).

**Examples**:

```bash
# Disable caching
ai-agent --agent research.ai --cache off "Query"

# Cache for 1 hour
ai-agent --agent research.ai --cache 1h "Query"

# Cache for 15 minutes
ai-agent --agent api.ai --cache 15m "API call"
```

**TTL format**: `off` | `<ms>` | `<N.Nu>` where u=`ms|s|m|h|d|w|mo|y`

---

## The Override Flag

### Universal Overrides

| Property   | Value                    |
| ---------- | ------------------------ |
| Flag       | `--override <key=value>` |
| Scope      | All Agents               |
| Repeatable | Yes                      |

**Description**: Override any setting for all agents including sub-agents. Takes precedence over other flags.

**Supported keys**:

- `models` - Model list
- `tools` - Tool access
- `agents` - Sub-agents
- `temperature`, `topP`, `topK` - Sampling
- `maxOutputTokens`, `repeatPenalty` - Output
- `llmTimeout`, `toolTimeout` - Timeouts
- `maxRetries`, `maxTurns`, `maxToolCallsPerTurn` - Limits
- `toolResponseMaxBytes` - Response size
- `mcpInitConcurrency` - MCP startup
- `cache` - Response cache TTL (time-to-live)
- `stream` - Streaming mode
- `interleaved` - Interleaved reasoning
- `reasoning`, `reasoningTokens` - Reasoning mode
- `caching` - Anthropic cache mode (provider-side prompt caching)
- `contextWindow` - Context size override
- `no-batch` - Disable batch tool execution
- `no-progress` - Disable progress-only task_status

**Examples**:

```bash
# Single override
ai-agent --agent chat.ai --override temperature=0.5 "Hello"

# Multiple overrides
ai-agent --agent chat.ai \
  --override temperature=0.5 \
  --override maxTurns=5 \
  --override contextWindow=128000 \
  "Complex query"

# Override for sub-agents too
ai-agent --agent orchestrator.ai \
  --agents "worker1.ai,worker2.ai" \
  --override temperature=0 \
  "All agents use temperature=0"
```

### Special Override Values

Some overrides accept special values:

```bash
# Disable batch tool execution
ai-agent --agent tools.ai --override no-batch=true "Sequential tools"

# Disable progress-only task_status
ai-agent --agent status.ai --override no-progress=true "Full status updates"

# Enable interleaved reasoning
ai-agent --agent reason.ai --override interleaved=true "Think step by step"
```

---

## Configuration File

### The `--config` Flag

| Property | Value             |
| -------- | ----------------- |
| Flag     | `--config <path>` |
| Scope    | Global            |

**Description**: Use a specific configuration file, overriding auto-discovery.

```bash
# Use specific config
ai-agent --config /path/to/config.json --agent chat.ai "Hello"

# Different configs for different environments
ai-agent --config ./config/production.json --agent api.ai --api 8080
ai-agent --config ./config/development.json --agent api.ai --api 8080
```

---

## Quick Reference

### Master Only (top-level agent)

| Flag              | Default | Description           |
| ----------------- | ------- | --------------------- |
| `--models <list>` | -       | LLM model(s) to use   |
| `--tools <list>`  | -       | MCP servers to enable |
| `--agents <list>` | -       | Sub-agent files       |
| `--schema <json>` | -       | JSON output schema    |

### Master Default (inherited by sub-agents)

| Flag                            | Default  | Description                                          |
| ------------------------------- | -------- | ---------------------------------------------------- |
| `--temperature <n>`             | `0`      | Response creativity                                  |
| `--top-p <n>`                   | -        | Nucleus sampling                                     |
| `--top-k <n>`                   | -        | Token selection limit                                |
| `--repeat-penalty <n>`          | -        | Repetition penalty                                   |
| `--max-turns <n>`               | `10`     | Maximum turns                                        |
| `--max-retries <n>`             | `5`      | Retry count                                          |
| `--max-tool-calls-per-turn <n>` | `10`     | Parallel tool calls                                  |
| `--max-output-tokens <n>`       | `4096`   | Response length                                      |
| `--llm-timeout-ms <ms>`         | `600000` | LLM timeout                                          |
| `--tool-timeout-ms <ms>`        | `300000` | Tool timeout                                         |
| `--tool-response-max-bytes <n>` | `12288`  | Max inline tool output                               |
| `--cache <ttl>`                 | -        | Response cache TTL                                   |
| `--reasoning <level>`           | -        | Reasoning effort                                     |
| `--default-reasoning <level>`   | -        | Default reasoning for agents                         |
| `--reasoning-tokens <n>`        | -        | Thinking token budget                                |
| `--caching <mode>`              | `full`   | Anthropic caching mode (full=enabled, none=disabled) |

### All Agents

| Flag                       | Default | Description             |
| -------------------------- | ------- | ----------------------- |
| `--override <key=value>`   | -       | Universal override      |
| `--stream` / `--no-stream` | `true`  | Streaming mode          |
| `--format <format>`        | -       | Output format           |
| `--verbose`                | `false` | Detailed logging        |
| `--trace-llm`              | `false` | LLM tracing             |
| `--trace-mcp`              | `false` | Tool tracing            |
| `--trace-slack`            | `false` | Slack bot communication |
| `--trace-sdk`              | `false` | AI SDK raw payloads     |

### Global

| Flag                    | Default                 | Description     |
| ----------------------- | ----------------------- | --------------- |
| `--config <path>`       | Auto-discovered         | Config file     |
| `--dry-run`             | `false`                 | Validation only |
| `--quiet`               | `false`                 | Suppress logs   |
| `--sessions-dir <path>` | `~/.ai-agent/sessions/` | Session storage |
| `--billing-file <path>` | -                       | Cost tracking   |
| `--resume <id>`         | -                       | Resume session  |

---

## See Also

- [CLI](CLI) - CLI overview and quick reference
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents
- [CLI-Debugging](CLI-Debugging) - Debugging execution
- [CLI-Scripting](CLI-Scripting) - Automation and scripting
- [Agent-Files-Behavior](Agent-Files-Behavior) - Frontmatter behavior options
- [Configuration](Configuration) - Configuration file reference
