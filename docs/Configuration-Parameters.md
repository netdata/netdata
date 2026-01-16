# Parameters Reference

Complete reference for all configuration parameters.

---

## Config File Structure

```json
{
  "providers": { ... },
  "mcpServers": { ... },
  "restTools": { ... },
  "queues": { ... },
  "cache": { ... },
  "accounting": { ... },
  "pricing": { ... },
  "defaults": { ... },
  "slack": { ... },
  "toolOutput": { ... }
}
```

---

## Providers

```json
{
  "providers": {
    "<name>": {
      "type": "openai | anthropic | google | openrouter | ollama | openai-compatible | test-llm",
      "apiKey": "string",
      "baseUrl": "string",
      "headers": { "string": "string" },
      "cacheStrategy": "full | none",
      "contextWindow": "number",
      "tokenizer": "string",
      "models": {
        "<model>": {
          "contextWindow": "number",
          "tokenizer": "string",
          "interleaved": "boolean | string",
          "overrides": {
            "temperature": "number | null",
            "top_p": "number | null",
            "top_k": "number | null",
            "repeat_penalty": "number | null"
          }
        }
      },
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"],
      "stringSchemaFormatsAllowed": ["string"],
      "stringSchemaFormatsDenied": ["string"]
    }
  }
}
```

---

## MCP Servers

```json
{
  "mcpServers": {
    "<name>": {
      "type": "stdio | http | sse | websocket",
      "command": "string",
      "args": ["string"],
      "env": { "string": "string" },
      "url": "string",
      "headers": { "string": "string" },
      "queue": "string",
      "cache": "string",
      "toolsCache": { "<tool>": "string" },
      "shared": "boolean",
      "healthProbe": "ping | listTools",
      "requestTimeoutMs": "number",
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"]
    }
  }
}
```

---

## REST Tools

```json
{
  "restTools": {
    "<name>": {
      "description": "string",
      "method": "GET | POST | PUT | DELETE",
      "url": "string",
      "headers": { "string": "string" },
      "parametersSchema": { ... },
      "bodyTemplate": { ... },
      "queue": "string",
      "cache": "string"
    }
  }
}
```

---

## Queues

```json
{
  "queues": {
    "<name>": {
      "concurrent": "number"
    }
  }
}
```

---

## Cache

```json
{
  "cache": {
    "backend": "sqlite | redis",
    "sqlite": {
      "path": "string"
    },
    "redis": {
      "url": "string"
    },
    "maxEntries": "number"
  }
}
```

---

## Accounting

```json
{
  "accounting": {
    "file": "string"
  }
}
```

---

## Pricing

```json
{
  "pricing": {
    "<provider>": {
      "<model>": {
        "unit": "per_1m",
        "prompt": "number",
        "completion": "number",
        "cacheRead": "number",
        "cacheWrite": "number"
      }
    }
  }
}
```

---

## Defaults

```json
{
  "defaults": {
    "temperature": "number",
    "topP": "number",
    "topK": "number",
    "repeatPenalty": "number",
    "llmTimeout": "number",
    "toolTimeout": "number",
    "maxTurns": "number",
    "maxToolCallsPerTurn": "number",
    "maxRetries": "number",
    "maxOutputTokens": "number",
    "toolResponseMaxBytes": "number",
    "contextWindowBufferTokens": "number",
    "stream": "boolean",
    "parallelToolCalls": "boolean"
  }
}
```

---

## Tool Output

```json
{
  "toolOutput": {
    "enabled": "boolean",
    "maxChunks": "number",
    "overlapPercent": "number",
    "avgLineBytesThreshold": "number",
    "models": ["string"]
  }
}
```

---

## Slack

```json
{
  "slack": {
    "enabled": "boolean",
    "appToken": "string",
    "botToken": "string",
    "signingSecret": "string",
    "routing": {
      "rules": [
        {
          "enabled": "boolean",
          "channels": ["string"],
          "agent": "string",
          "engage": ["channel-posts", "direct-messages", "mentions", "threads"],
          "promptTemplates": {
            "channelPost": "string",
            "directMessage": "string",
            "mention": "string",
            "threadReply": "string"
          }
        }
      ]
    }
  }
}
```

---

## CLI Parameters

| Parameter | Config Path | Default |
|-----------|-------------|---------|
| `--temperature <n>` | `defaults.temperature` | 0.7 |
| `--top-p <n>` | `defaults.topP` | 1.0 |
| `--llm-timeout <ms>` | `defaults.llmTimeout` | 120000 |
| `--tool-timeout <ms>` | `defaults.toolTimeout` | 60000 |
| `--max-output-tokens <n>` | `defaults.maxOutputTokens` | Model default |
| `--tool-response-max-bytes <n>` | `defaults.toolResponseMaxBytes` | 12288 |
| `--stream / --no-stream` | `defaults.stream` | true |
| `--parallel-tool-calls / --no-parallel-tool-calls` | `defaults.parallelToolCalls` | true |
| `--accounting <path>` | `accounting.file` | - |
| `--config <path>` | - | `.ai-agent.json` |

---

## Frontmatter Parameters

| Parameter | Type | Default |
|-----------|------|---------|
| `description` | string | - |
| `usage` | string | - |
| `models` | string[] | **required** |
| `tools` | string[] | [] |
| `agents` | string[] | [] |
| `toolsAllowed` | string[] | - |
| `toolsDenied` | string[] | - |
| `maxTurns` | number | 10 |
| `maxToolCallsPerTurn` | number | 20 |
| `maxRetries` | number | 3 |
| `temperature` | number | 0.7 |
| `topP` | number | 1.0 |
| `topK` | number | - |
| `maxOutputTokens` | number | Model default |
| `reasoning` | string | none |
| `llmTimeout` | number | 120000 |
| `toolTimeout` | number | 60000 |
| `cache` | string | off |
| `toolResponseMaxBytes` | number | 12288 |
| `contextWindow` | number | Provider default |
| `advisors` | string[] | - |
| `router.destinations` | string[] | - |
| `handoff` | string | - |
| `input` | object | - |
| `output.format` | string | markdown |
| `output.schema` | object | - |

---

## See Also

- [Configuration](Configuration) - Overview
- [CLI Reference](Getting-Started-CLI-Reference) - Command line options
- [Frontmatter Schema](Agent-Development-Frontmatter) - Agent configuration
