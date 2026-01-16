# Anthropic-Compatible Headend

Expose agents via the Anthropic Messages API for drop-in compatibility with Anthropic SDKs and tools.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [CLI Options](#cli-options) - Command-line configuration
- [Endpoints](#endpoints) - API reference
- [Request Format](#request-format) - Messages request structure
- [Response Format](#response-format) - Non-streaming and streaming responses
- [Client Integration](#client-integration) - Python, JavaScript, curl examples
- [Model Selection](#model-selection) - How agents map to models
- [Differences from OpenAI Headend](#differences-from-openai-headend) - Key distinctions
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The Anthropic-compatible headend provides a drop-in replacement for Anthropic's Messages API. Use it when:

- You have existing code using Anthropic SDKs
- You want explicit thinking/text content blocks
- You need Anthropic-style streaming events
- You prefer the Anthropic API structure

**Key features**:

- Full `/v1/messages` compatibility
- Streaming via Server-Sent Events (SSE)
- Explicit `thinking` and `text` content blocks
- Token usage tracking
- Concurrent request limiting

---

## Quick Start

```bash
# Start server
ai-agent --agent chat.ai --anthropic-completions 8083

# Test with curl
curl http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello"}]}'
```

---

## CLI Options

### --anthropic-completions

| Property   | Value                        |
| ---------- | ---------------------------- |
| Type       | `number`                     |
| Required   | Yes (to enable this headend) |
| Repeatable | Yes                          |

**Description**: HTTP port for Anthropic-compatible API. Can be specified multiple times.

**Example**:

```bash
ai-agent --agent chat.ai --anthropic-completions 8083
```

### --anthropic-completions-concurrency

| Property     | Value         |
| ------------ | ------------- |
| Type         | `number`      |
| Default      | `10`          |
| Valid values | `1` or higher |

**Description**: Maximum concurrent message requests.

**Example**:

```bash
ai-agent --agent chat.ai --anthropic-completions 8083 --anthropic-completions-concurrency 10
```

---

## Endpoints

### GET /health

Health check endpoint.

**Response**: `200 OK`

```json
{
  "status": "ok"
}
```

### GET /v1/models

List available models (agents).

**Response**:

```json
{
  "data": [
    {
      "id": "chat",
      "type": "model",
      "display_name": "Chat assistant for general questions"
    }
  ]
}
```

### POST /v1/messages

Create a message.

---

## Request Format

```json
{
  "model": "chat",
  "messages": [{ "role": "user", "content": "Hello!" }],
  "system": "You are a helpful assistant.",
  "stream": false
}
```

### Request Fields

| Field      | Type                | Required | Description                                         |
| ---------- | ------------------- | -------- | --------------------------------------------------- |
| `model`    | `string`            | Yes      | Agent name (filename without `.ai`)                 |
| `messages` | `array`             | Yes      | Conversation messages                               |
| `system`   | `string` or `array` | No       | System prompt(s)                                    |
| `stream`   | `boolean`           | No       | Enable SSE streaming (default: `false`)             |
| `format`   | `string`            | No       | Output format override (`markdown`, `json`, `text`) |
| `payload`  | `object`            | No       | Additional parameters including `schema`            |

### Message Format

```json
{
  "role": "user",
  "content": "Hello!"
}
```

Or with content array:

```json
{
  "role": "user",
  "content": [{ "type": "text", "text": "Hello!" }]
}
```

| Field     | Type                | Values              | Description         |
| --------- | ------------------- | ------------------- | ------------------- |
| `role`    | `string`            | `user`, `assistant` | Message author role |
| `content` | `string` or `array` | Any                 | Message content     |

> **Note**: Messages with `tool_use` or `tool_result` blocks are rejected. Tool calling happens internally within agents.

### JSON Output Format

For structured JSON output:

```json
{
  "model": "analyzer",
  "messages": [{ "role": "user", "content": "Analyze this" }],
  "format": "json",
  "payload": {
    "schema": {
      "type": "object",
      "properties": {
        "sentiment": { "type": "string" },
        "score": { "type": "number" }
      }
    }
  }
}
```

---

## Response Format

### Non-Streaming Response

```json
{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Hello! How can I help you today?"
    }
  ],
  "model": "chat",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 10,
    "output_tokens": 15,
    "total_tokens": 25
  }
}
```

### With Thinking Content

When agents emit reasoning:

```json
{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "thinking",
      "thinking": "Let me analyze this step by step..."
    },
    {
      "type": "text",
      "text": "Based on my analysis, here's the answer..."
    }
  ],
  "model": "chat",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 10,
    "output_tokens": 50,
    "total_tokens": 60
  }
}
```

### Streaming Response (SSE)

When `stream: true`:

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_abc","role":"assistant"}}

event: content_block_start
data: {"type":"content_block_start","content_block":{"type":"thinking"}}

event: content_block_delta
data: {"type":"content_block_delta","content_block":{"type":"thinking","thinking_delta":"Let me think..."}}

event: content_block_stop
data: {"type":"content_block_stop"}

event: content_block_start
data: {"type":"content_block_start","content_block":{"type":"text"}}

event: content_block_delta
data: {"type":"content_block_delta","content_block":{"type":"text","text_delta":"Hello!"}}

event: content_block_stop
data: {"type":"content_block_stop"}

event: message_stop
data: {"type":"message_stop"}
```

---

## Client Integration

### Python (Anthropic SDK)

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:8083",
    api_key="not-needed"  # Any value works
)

# Non-streaming
message = client.messages.create(
    model="chat",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(message.content[0].text)

# Streaming
with client.messages.stream(
    model="chat",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Tell me a story"}]
) as stream:
    for text in stream.text_stream:
        print(text, end="")
```

### JavaScript (Anthropic SDK)

```javascript
import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  baseURL: "http://localhost:8083",
  apiKey: "not-needed",
});

// Non-streaming
const message = await client.messages.create({
  model: "chat",
  max_tokens: 1024,
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(message.content[0].text);

// Streaming
const stream = await client.messages.stream({
  model: "chat",
  max_tokens: 1024,
  messages: [{ role: "user", content: "Tell me a story" }],
});
for await (const event of stream) {
  if (
    event.type === "content_block_delta" &&
    event.delta.type === "text_delta"
  ) {
    process.stdout.write(event.delta.text);
  }
}
```

### curl

```bash
# Non-streaming
curl http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{
    "model": "chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Streaming
curl -N http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{
    "model": "chat",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'

# List models
curl http://localhost:8083/v1/models
```

---

## Model Selection

The `model` field maps to agent filenames:

| Agent File       | Model Name    |
| ---------------- | ------------- |
| `chat.ai`        | `chat`        |
| `researcher.ai`  | `researcher`  |
| `code_review.ai` | `code_review` |

If an agent has `toolName` in frontmatter, that name is used instead.

> **Note**: Anthropic model names use underscores for deduplication (`chat`, `chat_2`), unlike OpenAI which uses dashes.

---

## Differences from OpenAI Headend

| Feature             | OpenAI Headend                       | Anthropic Headend                |
| ------------------- | ------------------------------------ | -------------------------------- |
| Endpoint            | `/v1/chat/completions`               | `/v1/messages`                   |
| Model deduplication | Dash (`chat-2`)                      | Underscore (`chat_2`)            |
| System prompt       | In messages array                    | Separate `system` field          |
| Token fields        | `prompt_tokens`, `completion_tokens` | `input_tokens`, `output_tokens`  |
| Thinking content    | `reasoning_content` delta            | Explicit `thinking` block        |
| Stop reason         | `stop`, `error`                      | `end_turn`, `error`              |
| Streaming           | Delta-based chunks                   | Explicit block start/stop events |

---

## Troubleshooting

### Model not found

**Symptom**: `{"error": "unknown_model"}`

**Cause**: The `model` field doesn't match any registered agent.

**Solutions**:

1. List available models: `curl http://localhost:8083/v1/models`
2. Check agent filename (without `.ai` extension)
3. Note: uses underscores for deduplication (`chat_2` not `chat-2`)

### JSON format errors

**Symptom**: `{"error": "missing_schema", "message": "JSON format requires schema"}`

**Cause**: Using `format: "json"` without providing a schema.

**Solution**: Include schema in payload:

```json
{
  "format": "json",
  "payload": {
    "schema": {
      "type": "object",
      "properties": { "result": { "type": "string" } }
    }
  }
}
```

### Missing x-api-key header

**Symptom**: SDK requires API key.

**Solution**: Use any non-empty string:

```python
client = anthropic.Anthropic(base_url="...", api_key="not-needed")
```

The headend does not validate API keys.

### Streaming events out of order

**Symptom**: Content blocks appear mixed up.

**Cause**: Usually a client parsing issue.

**Solution**: Track `content_block_start` and `content_block_stop` events to maintain block state.

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Headends-OpenAI-Compatible](Headends-OpenAI-Compatible) - OpenAI API compatibility
- [Agent-Files-Contracts](Agent-Files-Contracts) - Output schemas for JSON format
- [specs/headend-anthropic.md](specs/headend-anthropic.md) - Technical specification
