# OpenAI-Compatible Headend

Expose agents via OpenAI Chat Completions API.

---

## Start Server

```bash
ai-agent --agent agents/chat.ai --openai-completions 8082
```

---

## Endpoints

### List Models

```
GET /v1/models
```

Returns registered agents as models.

### Chat Completions

```
POST /v1/chat/completions
```

Standard OpenAI Chat Completions format.

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
  "stream": true
}
```

---

## Examples

### Non-Streaming

```bash
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "stream": false
  }'
```

Response:

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

### Streaming

```bash
curl -N http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ],
    "stream": true
  }'
```

Response (SSE):

```
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","choices":[{"delta":{"content":"!"}}]}

data: [DONE]
```

---

## Client Integration

### OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8082/v1",
    api_key="not-needed"
)

response = client.chat.completions.create(
    model="chat",
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)
```

### OpenAI JavaScript SDK

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:8082/v1',
  apiKey: 'not-needed'
});

const response = await client.chat.completions.create({
  model: 'chat',
  messages: [
    { role: 'user', content: 'Hello!' }
  ]
});
console.log(response.choices[0].message.content);
```

### curl

```bash
curl http://localhost:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"chat","messages":[{"role":"user","content":"Hello!"}]}'
```

---

## Open WebUI

Connect Open WebUI to your agents:

1. Start ai-agent:
   ```bash
   ai-agent --agent chat.ai --openai-completions 8082
   ```

2. Configure Open WebUI:
   - API Base URL: `http://localhost:8082/v1`
   - API Key: (any value)

3. Select model: `chat`

---

## Concurrency

```bash
ai-agent --agent chat.ai --openai-completions 8082 --openai-completions-concurrency 10
```

Default: 4 concurrent sessions

---

## Model Selection

The `model` field maps to agent name:

| Model | Agent |
|-------|-------|
| `chat` | `chat.ai` |
| `researcher` | `researcher.ai` |
| `code-review` | `code-review.ai` |

---

## See Also

- [Headends](Headends) - Overview
- [Anthropic-Compatible](Headends-Anthropic) - Anthropic API
- [docs/specs/headend-openai.md](../docs/specs/headend-openai.md) - Technical spec
