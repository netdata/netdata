# Frontmatter Schema

Complete reference for all `.ai` frontmatter keys.

---

## Required Keys

| Key | Type | Description |
|-----|------|-------------|
| `models` | `string[]` | Provider/model pairs (e.g., `openai/gpt-4o`) |

---

## Identity Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `description` | `string` | - | Short description (shown in tool listings) |
| `usage` | `string` | - | Usage instructions for users |

---

## Model Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `models` | `string[]` | **required** | List of `provider/model` pairs (fallback order) |
| `temperature` | `number` | `0.7` | LLM temperature (0.0–2.0) |
| `topP` | `number` | `1.0` | Top-p sampling |
| `topK` | `number` | - | Top-k sampling (provider-dependent) |
| `maxOutputTokens` | `number` | Model default | Maximum output tokens |
| `reasoning` | `string` | `none` | Reasoning level: `none`, `minimal`, `low`, `medium`, `high` |

---

## Tool Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `tools` | `string[]` | `[]` | MCP server names to enable |
| `agents` | `string[]` | `[]` | Sub-agent `.ai` paths (become tools) |
| `toolsAllowed` | `string[]` | - | Whitelist of tool names |
| `toolsDenied` | `string[]` | - | Blacklist of tool names |

---

## Execution Limits

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `maxTurns` | `number` | `10` | Maximum LLM turns before forced completion |
| `maxToolCallsPerTurn` | `number` | `20` | Max parallel tool calls per turn |
| `maxRetries` | `number` | `3` | Provider retry attempts on failure |
| `llmTimeout` | `number` | `120000` | LLM inactivity timeout (ms) |
| `toolTimeout` | `number` | `60000` | Tool execution timeout (ms) |

---

## Output Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `output.format` | `string` | `markdown` | Output format: `markdown`, `json`, `text` |
| `output.schema` | `object` | - | JSON Schema for `format: json` |

Example:

```yaml
output:
  format: json
  schema:
    type: object
    properties:
      summary:
        type: string
      confidence:
        type: number
    required:
      - summary
```

---

## Input Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `input` | `object` | - | JSON Schema for agent input validation |

Example:

```yaml
input:
  type: object
  properties:
    query:
      type: string
    maxResults:
      type: number
  required:
    - query
```

---

## Caching

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cache` | `string` | `off` | Cache TTL: `off`, `<ms>`, `5m`, `1h`, `1d` |

---

## Tool Output Handling

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `toolResponseMaxBytes` | `number` | `12288` | Max bytes before tool output storage |
| `toolOutput.enabled` | `boolean` | `true` | Enable tool output chunking |
| `toolOutput.maxChunks` | `number` | - | Maximum chunks for large outputs |
| `toolOutput.overlapPercent` | `number` | - | Overlap between chunks (%) |

---

## Multi-Agent Orchestration

| Key | Type | Description |
|-----|------|-------------|
| `advisors` | `string[]` | Agents to run in parallel before main execution |
| `router.destinations` | `string[]` | Agents for router handoff |
| `handoff` | `string` | Agent to receive output after completion |

See [Multi-Agent Orchestration](Agent-Development-Multi-Agent).

---

## Context Window

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `contextWindow` | `number` | Provider default | Override context window size |
| `contextWindowBufferTokens` | `number` | `256` | Safety margin for final messages |

---

## Example: Complete Frontmatter

```yaml
---
description: Web research agent with structured output
usage: "Query: natural language research question"
models:
  - openai/gpt-4o
  - anthropic/claude-3-haiku
tools:
  - brave
  - fetcher
toolsDenied:
  - dangerous_tool
agents:
  - ./helpers/summarizer.ai
maxTurns: 15
maxToolCallsPerTurn: 10
maxRetries: 3
temperature: 0.3
reasoning: medium
llmTimeout: 180000
toolTimeout: 60000
cache: 1h
output:
  format: json
  schema:
    type: object
    properties:
      findings:
        type: array
        items:
          type: object
          properties:
            title:
              type: string
            url:
              type: string
            summary:
              type: string
      conclusion:
        type: string
    required:
      - findings
      - conclusion
---
```

---

## See Also

- [AI Agent Configuration Guide](skills/ai-agent-configuration.md) - Complete reference
- [specs/frontmatter.md](specs/frontmatter.md) - Technical specification
