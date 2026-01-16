# Anthropic-Compatible Headend

Expose agents via Anthropic Messages API.

---

## Start Server

```bash
ai-agent --agent agents/chat.ai --anthropic-completions 8083
```

---

## Endpoints

### List Models

```
GET /v1/models
```

### Create Message

```
POST /v1/messages
```

### Health Check

```
GET /health
```

---

## Request Format

```json
{
  "model": "chat",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "max_tokens": 1024,
  "stream": true
}
```

---

## Examples

### Non-Streaming

```bash
curl http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{
    "model": "chat",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "max_tokens": 1024
  }'
```

Response:

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
    "output_tokens": 15
  }
}
```

### Streaming

```bash
curl -N http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: not-needed" \
  -d '{
    "model": "chat",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "max_tokens": 1024,
    "stream": true
  }'
```

Response (SSE):

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_abc","role":"assistant"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"!"}}

event: message_stop
data: {"type":"message_stop"}
```

---

## Client Integration

### Anthropic Python SDK

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:8083",
    api_key="not-needed"
)

message = client.messages.create(
    model="chat",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(message.content[0].text)
```

### Anthropic JavaScript SDK

```javascript
import Anthropic from '@anthropic-ai/sdk';

const client = new Anthropic({
  baseURL: 'http://localhost:8083',
  apiKey: 'not-needed'
});

const message = await client.messages.create({
  model: 'chat',
  max_tokens: 1024,
  messages: [
    { role: 'user', content: 'Hello!' }
  ]
});
console.log(message.content[0].text);
```

---

## Concurrency

```bash
ai-agent --agent chat.ai --anthropic-completions 8083 --anthropic-completions-concurrency 10
```

Default: 4 concurrent sessions

---

## See Also

- [Headends](Headends) - Overview
- [OpenAI-Compatible](Headends-OpenAI) - OpenAI API
- [docs/specs/headend-anthropic.md](../docs/specs/headend-anthropic.md) - Technical spec
