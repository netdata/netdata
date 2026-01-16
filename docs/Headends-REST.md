# REST API Headend

Expose agents as HTTP REST endpoints for web applications and scripts.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [CLI Options](#cli-options) - Command-line configuration
- [Endpoints](#endpoints) - API reference
- [Request Parameters](#request-parameters) - Query and body parameters
- [Response Format](#response-format) - Response structure
- [Error Responses](#error-responses) - Error handling
- [Integration Examples](#integration-examples) - Python, JavaScript, curl
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The REST API headend provides a simple HTTP interface for executing agents. Use it when:

- Building web applications that call agents
- Integrating agents into existing services
- Testing agents programmatically
- Creating webhooks or automation

**Key features**:

- GET-based API (easy to test with browser/curl)
- Concurrency limiting
- JSON and Markdown output formats

---

## Quick Start

```bash
# Start server on port 8080
ai-agent --agent chat.ai --api 8080

# Query the agent
curl "http://localhost:8080/v1/chat?q=Hello"
```

---

## CLI Options

### --api

| Property   | Value                        |
| ---------- | ---------------------------- |
| Type       | `number`                     |
| Required   | Yes (to enable REST headend) |
| Repeatable | Yes                          |

**Description**: HTTP port to listen on. Can be specified multiple times to listen on multiple ports.

**Example**:

```bash
ai-agent --agent chat.ai --api 8080 --api 8081
```

### --api-concurrency

| Property | Value    |
| -------- | -------- |
| Type     | `number` |
| Default  | `10`     |

**Description**: Maximum number of concurrent agent sessions. Additional requests receive `503 Service Unavailable`.

**Example**:

```bash
ai-agent --agent chat.ai --api 8080 --api-concurrency 10
```

---

## Endpoints

### GET /health

Health check for load balancer integration.

**Response**: `200 OK`

```json
{
  "status": "ok"
}
```

### GET /v1/{agentId}

Execute an agent and return the response.

| Parameter | Location | Required | Description                                                                                                  |
| --------- | -------- | -------- | ------------------------------------------------------------------------------------------------------------ |
| `agentId` | Path     | Yes      | Agent filename without `.ai` extension                                                                       |
| `q`       | Query    | Yes      | User prompt (URL-encoded)                                                                                    |
| `format`  | Query    | No       | Output format: `json`, `markdown`, `markdown+mermaid`, `pipe`, `slack-block-kit`, `tty`, `text`, `sub-agent` |

**Example**:

```bash
curl "http://localhost:8080/v1/chat?q=What%20is%202%2B2"
```

---

## Request Parameters

### q (required)

| Property | Value           |
| -------- | --------------- |
| Type     | `string`        |
| Location | Query parameter |
| Required | Yes             |

**Description**: The user prompt to send to the agent. Must be URL-encoded.

**Example**:

```bash
# Simple prompt
curl "http://localhost:8080/v1/chat?q=Hello"

# Complex prompt with special characters
curl "http://localhost:8080/v1/chat?q=What%20is%20the%20weather%20in%20Paris%3F"
```

### format (optional)

| Property     | Value                                                                                         |
| ------------ | --------------------------------------------------------------------------------------------- |
| Type         | `string`                                                                                      |
| Default      | `markdown`                                                                                    |
| Valid values | `json`, `markdown`, `markdown+mermaid`, `pipe`, `slack-block-kit`, `tty`, `text`, `sub-agent` |

**Description**: Desired output format. When `json` is specified, the agent must have an output schema defined in frontmatter, or validation will fail.

**Example**:

```bash
# Markdown output (default)
curl "http://localhost:8080/v1/chat?q=Hello"

# JSON output (requires agent with output schema)
curl "http://localhost:8080/v1/analyzer?q=Analyze%20this&format=json"

# Plain text output
curl "http://localhost:8080/v1/chat?q=Hello&format=pipe"
```

---

## Response Format

### Success Response

```json
{
  "success": true,
  "output": "Hello! How can I help you today?",
  "finalReport": {
    "content": "Hello! How can I help you today?",
    "format": "markdown",
    "ts": 1736976960123
  },
  "reasoning": "The user said hello, so I responded with a friendly greeting."
}
```

| Field         | Type      | Description                                     |
| ------------- | --------- | ----------------------------------------------- |
| `success`     | `boolean` | Always `true` for successful responses          |
| `output`      | `string`  | Concatenated streamed text output               |
| `finalReport` | `object`  | Structured final report from agent              |
| `reasoning`   | `string`  | (Optional) Aggregated thinking/reasoning output |

### JSON Format Response

When `format=json` is specified, the final report contains structured JSON data in `content_json`:

```json
{
  "success": true,
  "output": "Agent's text output...",
  "finalReport": {
    "content_json": {
      "summary": "Text analysis",
      "sentiment": "positive"
    },
    "format": "json",
    "ts": 1736976960123
  }
}
```

---

## Error Responses

### 400 Bad Request

Missing or invalid parameters.

```json
{
  "success": false,
  "output": "",
  "error": "Query parameter q is required"
}
```

### 404 Not Found

Agent not registered.

```json
{
  "success": false,
  "output": "",
  "error": "Agent 'chat' not registered"
}
```

### 405 Method Not Allowed

Using POST instead of GET for built-in routes.

```json
{
  "success": false,
  "output": "",
  "error": "method_not_allowed"
}
```

### 500 Internal Server Error

Agent execution failed. When the session completes but with errors, includes any partial output generated before failure.

```json
{
  "success": false,
  "output": "Partial output before failure...",
  "error": "LLM communication failed",
  "finalReport": {
    "format": "markdown",
    "content": "...partial response...",
    "ts": 1736976960123
  },
  "reasoning": "Agent attempted to call LLM..."
}
```

When an unexpected error occurs, output is empty:

```json
{
  "success": false,
  "output": "",
  "error": "Unexpected error message"
}
```

### 503 Service Unavailable

Concurrency limit reached.

```json
{
  "success": false,
  "output": "",
  "error": "concurrency_unavailable"
}
```

---

## Integration Examples

### Python

```python
import requests

# Simple GET request
response = requests.get(
    "http://localhost:8080/v1/chat",
    params={"q": "Hello, how are you?"}
)
data = response.json()
print(data["output"])

# With format parameter
response = requests.get(
    "http://localhost:8080/v1/analyzer",
    params={"q": "Analyze this text", "format": "json"}
)
data = response.json()
print(data["finalReport"]["content_json"])
```

### JavaScript (Node.js)

```javascript
// Using fetch
const response = await fetch(
  "http://localhost:8080/v1/chat?q=" + encodeURIComponent("Hello!"),
);
const data = await response.json();
console.log(data.output);
```

### JavaScript (Browser)

```javascript
// Simple fetch
async function askAgent(question) {
  const response = await fetch(
    `http://localhost:8080/v1/chat?q=${encodeURIComponent(question)}`,
  );
  const data = await response.json();
  return data.output;
}
```

### curl

```bash
# Basic request
curl "http://localhost:8080/v1/chat?q=Hello"

# URL-encoded complex prompt
curl "http://localhost:8080/v1/chat?q=$(echo 'What is 2+2?' | jq -sRr @uri)"

# JSON format
curl "http://localhost:8080/v1/analyzer?q=Analyze%20this&format=json"
```

---

## Troubleshooting

### Port already in use

**Symptom**: Server fails to start with "EADDRINUSE" error.

**Solution**: Choose a different port or stop the process using that port:

```bash
ai-agent --agent chat.ai --api 8081
```

### Agent not found (404)

**Symptom**: `{"success": false, "output": "", "error": "Agent 'xxx' not registered"}`

**Cause**: The agent ID in the URL doesn't match any registered agent.

**Solution**:

1. Check the agent filename (without `.ai` extension)
2. Verify the agent was loaded with `--agent`
3. For `chat.ai`, the endpoint is `/v1/chat`

### Requests timing out

**Symptom**: Requests hang or timeout without response.

**Possible causes**:

1. Agent execution taking too long
2. LLM provider not responding
3. Tool execution stuck

**Solutions**:

1. Check agent timeout settings
2. Verify LLM provider connectivity
3. Review tool configurations

### JSON format failing

**Symptom**: Session fails with an error related to JSON output schema.

**Cause**: Using `format=json` with an agent that has no output schema defined.

**Solution**: Add an output schema to the agent frontmatter:

```yaml
---
output:
  format: json
  schema:
    type: object
    properties:
      result: { type: string }
---
```

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Agent Files](Agent-Files) - Agent configuration
- [Agent-Files-Contracts](Agent-Files-Contracts) - Output schemas for JSON format
- [specs/headend-rest.md](specs/headend-rest.md) - Technical specification
