# OpenAI-Compatible Headend

Expose agents via the OpenAI Chat Completions API for drop-in compatibility with OpenAI SDKs and tools.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [CLI Options](#cli-options) - Command-line configuration
- [Endpoints](#endpoints) - API reference
- [Request Format](#request-format) - Chat completions request structure
- [Response Format](#response-format) - Non-streaming and streaming responses
- [Client Integration](#client-integration) - Python, JavaScript, curl examples
- [Open WebUI Integration](#open-webui-integration) - Use with Open WebUI
- [Model Selection](#model-selection) - How agents map to models
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The OpenAI-compatible headend provides a drop-in replacement for OpenAI's Chat Completions API. Use it when:
- You have existing code using OpenAI SDKs
- You want to use tools like Open WebUI with your agents
- You need streaming chat completions
- You want reasoning/thinking content in responses

**Key features**:
- Full `/v1/chat/completions` compatibility
- Streaming via Server-Sent Events (SSE)
- `reasoning_content` field for thinking output
- Token usage tracking
- Concurrent request limiting

---

## Quick Start

```bash
# Start server
ai-agent --agent chat.ai --openai-completions 8082

# Test with curl
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello"}]}'
```

---

## CLI Options

### --openai-completions

| Property | Value |
|----------|-------|
| Type | `number` |
| Required | Yes (to enable this headend) |
| Repeatable | Yes |

**Description**: HTTP port for OpenAI-compatible API. Can be specified multiple times.

**Example**:
```bash
ai-agent --agent chat.ai --openai-completions 8082
```

### --openai-completions-concurrency

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | `4` |
| Valid values | `1` to `100` |

**Description**: Maximum concurrent chat completion requests.

**Example**:
```bash
ai-agent --agent chat.ai --openai-completions 8082 --openai-completions-concurrency 10
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
  "object": "list",
  "data": [
    {
      "id": "chat",
      "object": "model",
      "owned_by": "ai-agent"
    },
    {
      "id": "analyzer",
      "object": "model",
      "owned_by": "ai-agent"
    }
  ]
}
```

### POST /v1/chat/completions

Create a chat completion.

---

## Request Format

```json
{
  "model": "chat",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "stream": false,
  "temperature": 0.7,
  "max_tokens": 1024
}
```

### Request Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | `string` | Yes | Agent name (filename without `.ai`) |
| `messages` | `array` | Yes | Conversation messages |
| `stream` | `boolean` | No | Enable SSE streaming (default: `false`) |
| `temperature` | `number` | No | Sampling temperature (0-2) |
| `top_p` | `number` | No | Nucleus sampling |
| `max_tokens` | `number` | No | Maximum tokens to generate |
| `format` | `string` | No | Output format override (`markdown`, `json`, `text`) |
| `response_format` | `object` | No | Structured output format |

### Message Format

```json
{
  "role": "user",
  "content": "Hello!"
}
```

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `role` | `string` | `system`, `user`, `assistant` | Message author role |
| `content` | `string` | Any | Message content |

> **Note**: Messages with `tool_calls` are rejected. Tool calling happens internally within agents.

### JSON Output Format

For structured JSON output:

```json
{
  "model": "analyzer",
  "messages": [{"role": "user", "content": "Analyze this"}],
  "response_format": {
    "type": "json_object",
    "json_schema": {
      "type": "object",
      "properties": {
        "sentiment": {"type": "string"},
        "score": {"type": "number"}
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
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1705432891,
  "model": "chat",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! How can I help you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 15,
    "total_tokens": 25
  }
}
```

### Streaming Response (SSE)

When `stream: true`:

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{"content":"!"}}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{}}],"finish_reason":"stop","usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}

data: [DONE]
```

### Reasoning Content

When agents emit thinking/reasoning, it appears in `reasoning_content`:

```json
{
  "choices": [
    {
      "delta": {
        "reasoning_content": "Let me think about this step by step..."
      }
    }
  ]
}
```

---

## Client Integration

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8082/v1",
    api_key="not-needed"  # Any value works
)

# Non-streaming
response = client.chat.completions.create(
    model="chat",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="chat",
    messages=[{"role": "user", "content": "Tell me a story"}],
    stream=True
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### JavaScript (OpenAI SDK)

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:8082/v1',
  apiKey: 'not-needed'
});

// Non-streaming
const response = await client.chat.completions.create({
  model: 'chat',
  messages: [
    { role: 'user', content: 'Hello!' }
  ]
});
console.log(response.choices[0].message.content);

// Streaming
const stream = await client.chat.completions.create({
  model: 'chat',
  messages: [{ role: 'user', content: 'Tell me a story' }],
  stream: true
});
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta?.content || '');
}
```

### curl

```bash
# Non-streaming
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Streaming
curl -N http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'

# List models
curl http://localhost:8082/v1/models
```

---

## Open WebUI Integration

Connect [Open WebUI](https://github.com/open-webui/open-webui) to your agents:

1. **Start ai-agent**:
   ```bash
   ai-agent --agent chat.ai --agent researcher.ai --openai-completions 8082
   ```

2. **Configure Open WebUI**:
   - Go to Settings → Connections
   - Add OpenAI-compatible API:
     - API Base URL: `http://localhost:8082/v1`
     - API Key: `any-value` (not validated)

3. **Select model**: Your agents appear as models (`chat`, `researcher`, etc.)

---

## Model Selection

The `model` field maps to agent filenames:

| Agent File | Model Name |
|------------|------------|
| `chat.ai` | `chat` |
| `researcher.ai` | `researcher` |
| `code-review.ai` | `code-review` |

If an agent has `toolName` in frontmatter, that name is used instead:

```yaml
---
toolName: my-custom-name
---
```

Duplicate names get numeric suffixes: `chat`, `chat-2`, `chat-3`.

---

## Troubleshooting

### Model not found

**Symptom**: `{"error": "unknown_model"}`

**Cause**: The `model` field doesn't match any registered agent.

**Solutions**:
1. List available models: `curl http://localhost:8082/v1/models`
2. Check agent filename (without `.ai` extension)
3. Verify agent was loaded with `--agent`

### JSON format errors

**Symptom**: `{"error": "missing_schema"}`

**Cause**: Using `response_format.type: "json_object"` without a schema.

**Solution**: Include `json_schema`:
```json
{
  "response_format": {
    "type": "json_object",
    "json_schema": {
      "type": "object",
      "properties": {"result": {"type": "string"}}
    }
  }
}
```

### tool_calls not supported

**Symptom**: `{"error": "tool_calls_not_supported"}`

**Cause**: Messages contain `tool_calls` field.

**Solution**: Remove `tool_calls` from messages. Tool calling is handled internally by agents.

### Streaming not working

**Symptom**: Response comes all at once.

**Cause**: `stream` not set to `true`.

**Solution**: Add `"stream": true` to request body.

### API key errors with SDKs

**Symptom**: SDK requires API key but you don't have one.

**Solution**: Use any non-empty string:
```python
client = OpenAI(base_url="...", api_key="not-needed")
```

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Headends-Anthropic-Compatible](Headends-Anthropic-Compatible) - Anthropic API compatibility
- [Agent-Files-Contracts](Agent-Files-Contracts) - Output schemas for JSON format
- [specs/headend-openai.md](specs/headend-openai.md) - Technical specification
