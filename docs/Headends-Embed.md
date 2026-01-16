# Embed Headend

Public embeddable chat for websites.

---

## Start Server

```bash
ai-agent --agent chat.ai --embed 8090
```

---

## Endpoints

### JavaScript Client

```
GET /ai-agent-public.js
```

### Chat API

```
POST /v1/chat
```

Streams responses via SSE.

### Health Check

```
GET /health
```

---

## Client Integration

### Basic Embed

```html
<script src="http://localhost:8090/ai-agent-public.js"></script>
<script>
  AIAgent.init({
    endpoint: 'http://localhost:8090',
    agent: 'chat'
  });
</script>
```

### With Custom UI

```html
<div id="chat-container"></div>
<script src="http://localhost:8090/ai-agent-public.js"></script>
<script>
  const chat = AIAgent.create({
    endpoint: 'http://localhost:8090',
    agent: 'chat',
    container: '#chat-container'
  });
</script>
```

---

## API Usage

### Direct API Call

```bash
curl -X POST http://localhost:8090/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent": "chat",
    "message": "Hello!"
  }'
```

### SSE Response

```
event: text
data: {"content": "Hello"}

event: text
data: {"content": "!"}

event: done
data: {}
```

---

## JavaScript Example

```javascript
async function chat(message) {
  const response = await fetch('http://localhost:8090/v1/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      agent: 'chat',
      message: message
    })
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    const text = decoder.decode(value);
    // Parse SSE events
    console.log(text);
  }
}
```

---

## Concurrency

```bash
ai-agent --agent chat.ai --embed 8090 --embed-concurrency 20
```

Default: 10 concurrent sessions

---

## CORS

The embed headend includes CORS headers for cross-origin requests.

---

## Security Considerations

- Rate limit at proxy/load balancer level
- Consider authentication for production
- Limit agent capabilities for public access

---

## See Also

- [Headends](Headends) - Overview
- [docs/specs/headend-embed.md](../docs/specs/headend-embed.md) - Technical spec
