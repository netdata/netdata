# Parameters Reference

Complete reference for all configuration parameters.

---

## Table of Contents

- [Overview](#overview) - Configuration structure
- [Config File Structure](#config-file-structure) - Top-level sections
- [Providers](#providers) - LLM provider configuration
- [MCP Servers](#mcp-servers) - MCP server configuration
- [REST Tools](#rest-tools) - REST API tool configuration
- [Queues](#queues) - Concurrency queue configuration
- [Cache](#cache) - Cache backend configuration
- [Accounting](#accounting) - Cost tracking configuration
- [Pricing](#pricing) - Token pricing configuration
- [Defaults](#defaults) - Default parameter values
- [Tool Output](#tool-output) - Tool output handling configuration
- [Slack](#slack) - Slack integration configuration
- [CLI Parameters](#cli-parameters) - Command line options
- [Frontmatter Parameters](#frontmatter-parameters) - Agent configuration
- [Environment Variables](#environment-variables) - Environment configuration
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent configuration is organized into:

| Level       | Location          | Purpose            |
| ----------- | ----------------- | ------------------ |
| Config file | `.ai-agent.json`  | Global settings    |
| Frontmatter | Agent `.ai` files | Per-agent settings |
| CLI         | Command line      | Runtime overrides  |
| Environment | Shell variables   | Secrets and paths  |

---

## Config File Structure

```json
{
  "providers": {},
  "mcpServers": {},
  "restTools": {},
  "queues": {},
  "cache": {},
  "accounting": {},
  "pricing": {},
  "defaults": {},
  "slack": {},
  "toolOutput": {}
}
```

---

## Providers

LLM provider configuration.

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

### Provider Properties

| Property                     | Type       | Default          | Description                  |
| ---------------------------- | ---------- | ---------------- | ---------------------------- |
| `type`                       | `string`   | Required         | Provider type                |
| `apiKey`                     | `string`   | -                | API key (supports `${VAR}`)  |
| `baseUrl`                    | `string`   | Provider default | API base URL                 |
| `headers`                    | `object`   | `{}`             | Custom HTTP headers          |
| `cacheStrategy`              | `string`   | `"full"`         | Anthropic cache control      |
| `contextWindow`              | `number`   | Provider default | Token limit                  |
| `tokenizer`                  | `string`   | Auto-detect      | Token counting method        |
| `models`                     | `object`   | `{}`             | Per-model overrides          |
| `toolsAllowed`               | `string[]` | `[]`             | Tools to expose              |
| `toolsDenied`                | `string[]` | `[]`             | Tools to hide                |
| `stringSchemaFormatsAllowed` | `string[]` | `[]`             | JSON Schema formats to allow |
| `stringSchemaFormatsDenied`  | `string[]` | `[]`             | JSON Schema formats to strip |

### Model Properties

| Property                   | Type             | Default        | Description                |
| -------------------------- | ---------------- | -------------- | -------------------------- |
| `contextWindow`            | `number`         | Provider value | Model token limit          |
| `tokenizer`                | `string`         | Provider value | Model tokenizer            |
| `interleaved`              | `boolean/string` | `false`        | Interleaved reasoning mode |
| `overrides.temperature`    | `number/null`    | -              | Temperature override       |
| `overrides.top_p`          | `number/null`    | -              | Top-p override             |
| `overrides.top_k`          | `number/null`    | -              | Top-k override             |
| `overrides.repeat_penalty` | `number/null`    | -              | Repeat penalty override    |

---

## MCP Servers

MCP server configuration.

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

### MCP Server Properties

| Property           | Type       | Default     | Description                |
| ------------------ | ---------- | ----------- | -------------------------- |
| `type`             | `string`   | Required    | Transport type             |
| `command`          | `string`   | -           | Stdio command              |
| `args`             | `string[]` | `[]`        | Stdio command arguments    |
| `env`              | `object`   | `{}`        | Environment variables      |
| `url`              | `string`   | -           | HTTP/SSE/WebSocket URL     |
| `headers`          | `object`   | `{}`        | HTTP headers               |
| `queue`            | `string`   | `"default"` | Concurrency queue          |
| `cache`            | `string`   | `"off"`     | Response cache TTL         |
| `toolsCache`       | `object`   | `{}`        | Per-tool cache TTL         |
| `shared`           | `boolean`  | `true`      | Share across sessions      |
| `healthProbe`      | `string`   | `"ping"`    | Health check method        |
| `requestTimeoutMs` | `number`   | -           | Request timeout (optional) |
| `toolsAllowed`     | `string[]` | `[]`        | Tools to expose            |
| `toolsDenied`      | `string[]` | `[]`        | Tools to hide              |

---

## REST Tools

REST API tool configuration.

```json
{
  "restTools": {
    "<name>": {
      "description": "string",
      "method": "GET | POST | PUT | DELETE | PATCH",
      "url": "string",
      "headers": { "string": "string" },
      "parametersSchema": {},
      "bodyTemplate": {},
      "queue": "string",
      "cache": "string",
      "streaming": {
        "mode": "json-stream",
        "linePrefix": "string",
        "discriminatorField": "string",
        "doneValue": "string",
        "tokenField": "string",
        "tokenValue": "string",
        "answerField": "string",
        "timeoutMs": "number | string"
      },
      "hasComplexQueryParams": "boolean",
      "queryParamNames": ["string"]
    }
  }
}
```

### REST Tool Properties

| Property                | Type       | Default     | Description                   |
| ----------------------- | ---------- | ----------- | ----------------------------- |
| `description`           | `string`   | Required    | Tool description              |
| `method`                | `string`   | Required    | HTTP method                   |
| `url`                   | `string`   | Required    | URL template                  |
| `headers`               | `object`   | `{}`        | HTTP headers                  |
| `parametersSchema`      | `object`   | Required    | JSON Schema for inputs        |
| `bodyTemplate`          | `any`      | -           | Request body template         |
| `queue`                 | `string`   | `"default"` | Concurrency queue             |
| `cache`                 | `string`   | `"off"`     | Response cache TTL            |
| `streaming`             | `object`   | -           | Streaming configuration       |
| `hasComplexQueryParams` | `boolean`  | `false`     | Enable nested query params    |
| `queryParamNames`       | `string[]` | `[]`        | Params to serialize complexly |

### Streaming Properties

| Property             | Type            | Default     | Description                |
| -------------------- | --------------- | ----------- | -------------------------- |
| `mode`               | `string`        | Required    | Must be `"json-stream"`    |
| `linePrefix`         | `string`        | `""`        | Prefix to strip            |
| `discriminatorField` | `string`        | `"type"`    | Event type field           |
| `doneValue`          | `string`        | `"done"`    | End indicator              |
| `tokenField`         | `string`        | `"content"` | Token content field        |
| `tokenValue`         | `string`        | `"token"`   | Token event type           |
| `answerField`        | `string`        | `"answer"`  | Final answer field         |
| `timeoutMs`          | `number/string` | `60000`     | Stream timeout (REST only) |

---

## Queues

Concurrency queue configuration.

```json
{
  "queues": {
    "<name>": {
      "concurrent": "number"
    }
  }
}
```

### Queue Properties

| Property     | Type     | Default  | Description                 |
| ------------ | -------- | -------- | --------------------------- |
| `concurrent` | `number` | Required | Max simultaneous executions |

---

## Cache

Cache backend configuration.

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

### Cache Properties

| Property      | Type     | Default                  | Description       |
| ------------- | -------- | ------------------------ | ----------------- |
| `backend`     | `string` | `"sqlite"`               | Cache backend     |
| `sqlite.path` | `string` | `"~/.ai-agent/cache.db"` | SQLite path       |
| `redis.url`   | `string` | -                        | Redis URL         |
| `maxEntries`  | `number` | `5000`                   | Max cache entries |

---

## Accounting

Cost tracking configuration.

```json
{
  "accounting": {
    "file": "string"
  }
}
```

### Accounting Properties

| Property | Type     | Default | Description          |
| -------- | -------- | ------- | -------------------- |
| `file`   | `string` | -       | Accounting file path |

---

## Pricing

Token pricing configuration.

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

### Pricing Properties

| Property     | Type     | Default    | Description        |
| ------------ | -------- | ---------- | ------------------ |
| `unit`       | `string` | `"per_1m"` | Price unit         |
| `prompt`     | `number` | `0`        | Input token price  |
| `completion` | `number` | `0`        | Output token price |
| `cacheRead`  | `number` | `0`        | Cache read price   |
| `cacheWrite` | `number` | `0`        | Cache write price  |

---

## Defaults

Default parameter values.

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
    "stream": "boolean"
  }
}
```

### Defaults Properties

| Property                    | Type      | Default  | Description            |
| --------------------------- | --------- | -------- | ---------------------- |
| `temperature`               | `number`  | `0.0`    | LLM temperature        |
| `topP`                      | `number`  | `null`   | Top-p sampling         |
| `topK`                      | `number`  | `null`   | Top-k sampling         |
| `repeatPenalty`             | `number`  | -        | Repetition penalty     |
| `llmTimeout`                | `number`  | `600000` | LLM timeout (ms)       |
| `toolTimeout`               | `number`  | `300000` | Tool timeout (ms)      |
| `maxTurns`                  | `number`  | `10`     | Max conversation turns |
| `maxToolCallsPerTurn`       | `number`  | `10`     | Max tools per turn     |
| `maxRetries`                | `number`  | `5`      | Max retry attempts     |
| `maxOutputTokens`           | `number`  | `4096`   | Max output tokens      |
| `toolResponseMaxBytes`      | `number`  | `12288`  | Max tool response size |
| `contextWindowBufferTokens` | `number`  | -        | Context buffer         |
| `stream`                    | `boolean` | `true`   | Enable streaming       |

---

## Tool Output

Tool output handling configuration.

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

### Tool Output Properties

| Property                | Type       | Default | Description                     |
| ----------------------- | ---------- | ------- | ------------------------------- |
| `enabled`               | `boolean`  | `true`  | Enable tool output storage      |
| `maxChunks`             | `number`   | `1`     | Max stored chunks               |
| `overlapPercent`        | `number`   | `10`    | Chunk overlap percentage        |
| `avgLineBytesThreshold` | `number`   | `1000`  | Line size threshold             |
| `models`                | `string[]` | `[]`    | Models that support tool output |

---

## Slack

Slack integration configuration.

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

### Slack Properties

| Property        | Type      | Default | Description          |
| --------------- | --------- | ------- | -------------------- |
| `enabled`       | `boolean` | `false` | Enable Slack         |
| `appToken`      | `string`  | -       | Slack app token      |
| `botToken`      | `string`  | -       | Slack bot token      |
| `signingSecret` | `string`  | -       | Slack signing secret |
| `routing.rules` | `array`   | `[]`    | Routing rules        |

### Routing Rule Properties

| Property          | Type       | Default | Description      |
| ----------------- | ---------- | ------- | ---------------- |
| `enabled`         | `boolean`  | `true`  | Enable rule      |
| `channels`        | `string[]` | `[]`    | Channel IDs      |
| `agent`           | `string`   | -       | Agent path       |
| `engage`          | `string[]` | `[]`    | Engagement types |
| `promptTemplates` | `object`   | `{}`    | Custom prompts   |

---

## CLI Parameters

| Parameter                       | Config Path                     | Default          | Description        |
| ------------------------------- | ------------------------------- | ---------------- | ------------------ |
| `--temperature <n>`             | `defaults.temperature`          | `0.0`            | LLM temperature    |
| `--top-p <n>`                   | `defaults.topP`                 | `null`           | Top-p sampling     |
| `--llm-timeout <ms>`            | `defaults.llmTimeout`           | `600000`         | LLM timeout        |
| `--tool-timeout <ms>`           | `defaults.toolTimeout`          | `300000`         | Tool timeout       |
| `--max-output-tokens <n>`       | `defaults.maxOutputTokens`      | `4096`           | Max output         |
| `--tool-response-max-bytes <n>` | `defaults.toolResponseMaxBytes` | `12288`          | Max tool response  |
| `--stream / --no-stream`        | `defaults.stream`               | `true`           | Enable streaming   |
| `--accounting <path>`           | `accounting.file`               | -                | Accounting file    |
| `--config <path>`               | -                               | `.ai-agent.json` | Config file        |
| `--verbose`                     | -                               | `false`          | Verbose output     |
| `--dry-run`                     | -                               | `false`          | Don't execute      |
| `--trace-mcp`                   | -                               | `false`          | Trace MCP calls    |
| `--override <key>=<value>`      | -                               | -                | Override parameter |

---

## Frontmatter Parameters

| Parameter              | Type       | Default          | Description        |
| ---------------------- | ---------- | ---------------- | ------------------ |
| `description`          | `string`   | -                | Agent description  |
| `usage`                | `string`   | -                | Usage instructions |
| `models`               | `string[]` | **Required**     | Model list         |
| `tools`                | `string[]` | `[]`             | Tool sources       |
| `agents`               | `string[]` | `[]`             | Sub-agent access   |
| `toolsAllowed`         | `string[]` | -                | Tools to expose    |
| `toolsDenied`          | `string[]` | -                | Tools to hide      |
| `maxTurns`             | `number`   | `10`             | Max turns          |
| `maxToolCallsPerTurn`  | `number`   | `10`             | Max tools per turn |
| `maxRetries`           | `number`   | `5`              | Max retries        |
| `temperature`          | `number`   | `0.0`            | Temperature        |
| `topP`                 | `number`   | `null`           | Top-p              |
| `topK`                 | `number`   | -                | Top-k              |
| `maxOutputTokens`      | `number`   | `4096`           | Max output         |
| `reasoning`            | `string`   | `"none"`         | Reasoning mode     |
| `llmTimeout`           | `number`   | `600000`         | LLM timeout (ms)   |
| `toolTimeout`          | `number`   | `300000`         | Tool timeout (ms)  |
| `cache`                | `string`   | `"off"`          | Response cache TTL |
| `toolResponseMaxBytes` | `number`   | `12288`          | Max tool response  |
| `contextWindow`        | `number`   | Provider default | Token limit        |
| `advisors`             | `string[]` | -                | Advisor agents     |
| `router.destinations`  | `string[]` | -                | Router targets     |
| `handoff`              | `string`   | -                | Handoff agent      |
| `input`                | `object`   | -                | Input schema       |
| `output.format`        | `string`   | `"markdown"`     | Output format      |
| `output.schema`        | `object`   | -                | Output JSON schema |

---

## Environment Variables

| Variable                       | Purpose            | Example       |
| ------------------------------ | ------------------ | ------------- |
| `OPENAI_API_KEY`               | OpenAI API key     | `sk-...`      |
| `ANTHROPIC_API_KEY`            | Anthropic API key  | `sk-ant-...`  |
| `GOOGLE_GENERATIVE_AI_API_KEY` | Google API key     | `AI...`       |
| `OPENROUTER_API_KEY`           | OpenRouter API key | `sk-or-...`   |
| `AI_AGENT_HOME`                | Config directory   | `~/.ai-agent` |
| `DEBUG`                        | Debug logging      | `ai-agent:*`  |
| `CONTEXT_DEBUG`                | Context debugging  | `true`        |
| `DEBUG_REST_CALLS`             | REST call tracing  | `true`        |
| `TRACE_REST`                   | REST tracing       | `true`        |

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [CLI Reference](Getting-Started-CLI-Reference) - Command line options
- [Frontmatter Schema](Agent-Development-Frontmatter) - Agent configuration
- [Providers](Configuration-Providers) - Provider configuration
- [MCP Servers](Configuration-MCP-Servers) - MCP server configuration
- [REST Tools](Configuration-REST-Tools) - REST tool configuration
- [Caching](Configuration-Caching) - Cache configuration
- [Queues](Configuration-Queues) - Queue configuration
- [Pricing](Configuration-Pricing) - Pricing configuration
