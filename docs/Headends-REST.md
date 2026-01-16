# REST API Headend

Expose agents as REST endpoints.

---

## Start Server

```bash
ai-agent --agent agents/chat.ai --api 8080
```

Multiple ports:

```bash
ai-agent --agent agents/chat.ai --api 8080 --api 8081
```

---

## Endpoints

### Health Check

```
GET /health
```

Response: `200 OK`

### Query Agent

```
GET /v1/<agent>?q=<prompt>&format=<format>
POST /v1/<agent>
```

Parameters:
- `q` (required): User prompt
- `format` (optional): `markdown` (default), `json`, `text`
- `schema` (optional): JSON Schema when format=json

---

## Examples

### GET Request

```bash
curl "http://localhost:8080/v1/chat?q=Hello"
```

### POST Request

```bash
curl -X POST "http://localhost:8080/v1/chat" \
  -H "Content-Type: application/json" \
  -d '{"q": "Hello", "format": "markdown"}'
```

### JSON Format

```bash
curl -X POST "http://localhost:8080/v1/analyzer" \
  -H "Content-Type: application/json" \
  -d '{
    "q": "Analyze this text",
    "format": "json",
    "schema": {
      "type": "object",
      "properties": {
        "summary": {"type": "string"},
        "sentiment": {"type": "string"}
      }
    }
  }'
```

---

## Streaming

Responses stream via Server-Sent Events:

```bash
curl -N "http://localhost:8080/v1/chat?q=Hello"
```

### SSE Events

```
data: {"type": "text", "content": "Hello"}
data: {"type": "text", "content": "! How"}
data: {"type": "text", "content": " can I help?"}
data: {"type": "done"}
```

---

## Concurrency

Limit concurrent requests:

```bash
ai-agent --agent chat.ai --api 8080 --api-concurrency 10
```

Default: 4 concurrent sessions

---

## Agent Names

Agents are accessed by filename (without `.ai`):

| File | Endpoint |
|------|----------|
| `chat.ai` | `/v1/chat` |
| `web-research.ai` | `/v1/web-research` |
| `agents/helper.ai` | `/v1/helper` |

---

## Error Responses

### 400 Bad Request

```json
{
  "error": "Missing required parameter: q"
}
```

### 404 Not Found

```json
{
  "error": "Agent not found: unknown"
}
```

### 500 Internal Error

```json
{
  "error": "LLM communication failed",
  "details": "Timeout after 120000ms"
}
```

### 503 Service Unavailable

```json
{
  "error": "No available slots",
  "retry_after": 5
}
```

---

## Integration Examples

### Python

```python
import requests

response = requests.get(
    "http://localhost:8080/v1/chat",
    params={"q": "Hello"}
)
print(response.text)
```

### JavaScript

```javascript
const response = await fetch(
  "http://localhost:8080/v1/chat?q=Hello"
);
const text = await response.text();
console.log(text);
```

### curl with Streaming

```bash
curl -N "http://localhost:8080/v1/chat?q=Hello" | while read line; do
  echo "$line"
done
```

---

## See Also

- [Headends](Headends) - Overview
- [docs/specs/headend-rest.md](../docs/specs/headend-rest.md) - Technical spec
