# Embed Headend

Deploy agents as embeddable chat widgets for public websites with SSE streaming and transcript persistence.

---

## Table of Contents

- [Overview](#overview) - What this headend provides
- [Quick Start](#quick-start) - Get running in 30 seconds
- [CLI Options](#cli-options) - Command-line configuration
- [Endpoints](#endpoints) - API reference
- [Request Format](#request-format) - Chat request structure
- [SSE Events](#sse-events) - Server-Sent Events format
- [JavaScript Client](#javascript-client) - Using the public client library
- [Configuration](#configuration) - Full configuration options
- [Authentication](#authentication) - Rate limiting and access control
- [Metrics](#metrics) - Prometheus metrics export
- [Transcript Persistence](#transcript-persistence) - Conversation storage
- [CORS Configuration](#cors-configuration) - Cross-origin access
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related pages

---

## Overview

The Embed headend provides a public HTTP API designed for website chat widgets. Use it when:

- Building customer support chat on your website
- Creating public-facing Q&A systems
- Embedding AI assistance in web applications
- Need stateless multi-turn conversations

**Key features**:

- Server-Sent Events (SSE) streaming
- Public JavaScript client library
- Stable client UUIDs for follow-up questions
- Transcript persistence
- Rate limiting and authentication tiers
- Prometheus metrics

---

## Quick Start

```bash
# Start server
ai-agent --agent chat.ai --embed 8090

# Test the endpoint
curl -X POST http://localhost:8090/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"Hello!","agentId":"chat"}'
```

### Embed in Website

```html
<script src="http://localhost:8090/ai-agent-public.js"></script>
<script>
  AIAgent.init({
    endpoint: "http://localhost:8090",
    agent: "chat",
  });
</script>
```

---

## CLI Options

### --embed

| Property   | Value                        |
| ---------- | ---------------------------- |
| Type       | `number`                     |
| Required   | Yes (to enable this headend) |
| Repeatable | Yes                          |

**Description**: HTTP port for embed API. Can be specified multiple times.

**Example**:

```bash
ai-agent --agent chat.ai --embed 8090
```

### --embed-concurrency

| Property     | Value        |
| ------------ | ------------ |
| Type         | `number`     |
| Default      | `10`         |
| Valid values | `1` to `100` |

**Description**: Maximum concurrent chat sessions.

**Example**:

```bash
ai-agent --agent chat.ai --embed 8090 --embed-concurrency 20
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

### GET /ai-agent-public.js

Serves the public JavaScript client library. Cacheable.

### POST /v1/chat

Start or continue a chat session. Returns SSE stream.

### GET /metrics (optional)

Prometheus-format metrics when enabled.

---

## Request Format

```json
{
  "message": "How do I configure alerts?",
  "agentId": "support",
  "clientId": "optional-uuid",
  "history": [
    { "role": "user", "content": "Previous question" },
    { "role": "assistant", "content": "Previous answer" }
  ],
  "format": "markdown"
}
```

### Request Fields

| Field      | Type     | Required | Description                                                                                          |
| ---------- | -------- | -------- | ---------------------------------------------------------------------------------------------------- |
| `message`  | `string` | Yes      | User's message                                                                                       |
| `agentId`  | `string` | No       | Agent to use (default from config)                                                                   |
| `clientId` | `string` | No       | Stable client identifier                                                                             |
| `history`  | `array`  | No       | Previous conversation turns                                                                          |
| `format`   | `string` | No       | Output format: `markdown`, `markdown+mermaid`, `slack-block-kit`, `json`, `tty`, `pipe`, `sub-agent` |

### History Format

```json
[
  { "role": "user", "content": "What is X?" },
  { "role": "assistant", "content": "X is..." },
  { "role": "user", "content": "Tell me more" }
]
```

**Note**: Server is stateless. History is used only for prompt context.

---

## SSE Events

The endpoint returns Server-Sent Events:

### client

Client identification (sent first).

```
event: client
data: {"clientId":"abc-123-uuid","isNew":true}
```

### meta

Session metadata.

```
event: meta
data: {"sessionId":"session-uuid","turn":1,"agentId":"support"}
```

### status

Agent status updates.

```
event: status
data: {"agent":"support","status":"in-progress","message":"Searching...","done":"","pending":"3 tools","now":"web-search","timestamp":"2026-01-16T12:34:56.789Z"}
```

### report

Final report content chunks.

```
event: report
data: {"chunk":"Here is your answer...","index":0}
```

### done

Completion event.

```
event: done
data: {"success":true,"metrics":{"tokens":1234,"cost":0.01},"reportLength":456}
```

### error

Error event.

```
event: error
data: {"code":"rate_limited","message":"Too many requests","recoverable":true}
```

---

## JavaScript Client

The public client is served at `/ai-agent-public.js`.

### Basic Usage

```html
<script src="http://localhost:8090/ai-agent-public.js"></script>
<script>
  const chat = AIAgent.init({
    endpoint: 'http://localhost:8090',
    agent: 'support'
  });

  // Send message
  const response = await chat.send('How do I configure alerts?');
  console.log(response.content);
</script>
```

### With Custom Container

```html
<div id="chat-widget"></div>
<script src="http://localhost:8090/ai-agent-public.js"></script>
<script>
  AIAgent.create({
    endpoint: "http://localhost:8090",
    agent: "support",
    container: "#chat-widget",
  });
</script>
```

### Event Handling

```javascript
const chat = AIAgent.init({
  endpoint: "http://localhost:8090",
  agent: "support",
  onStatus: (status) => {
    console.log("Status:", status.message);
  },
  onChunk: (chunk) => {
    document.getElementById("output").append(chunk);
  },
  onDone: (result) => {
    console.log("Complete:", result);
  },
  onError: (error) => {
    console.error("Error:", error.message);
  },
});
```

### Manual SSE Handling

```javascript
async function chat(message) {
  const response = await fetch("http://localhost:8090/v1/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      agentId: "support",
      message: message,
    }),
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n\n");
    buffer = lines.pop() || "";

    for (const line of lines) {
      if (line.startsWith("event:")) {
        const eventType = line.split(":")[1].trim();
        const dataLine = lines.find((l) => l.startsWith("data:"));
        if (dataLine) {
          const data = JSON.parse(dataLine.substring(5));
          console.log(eventType, data);
        }
      }
    }
  }
}
```

---

## Configuration

In `.ai-agent.json`:

```json
{
  "embed": {
    "defaultAgent": "support.ai",
    "corsOrigins": ["*.example.com", "localhost:*"],
    "rateLimit": {
      "enabled": true,
      "requestsPerMinute": 10,
      "burstSize": 5
    },
    "auth": {
      "tiers": {
        "netdataProperties": {
          "origins": ["*.netdata.cloud"],
          "rateLimit": { "requestsPerMinute": 60 }
        },
        "agentDashboards": {
          "requireGuid": true,
          "verifyGuidInCloud": false,
          "rateLimit": { "requestsPerMinutePerGuid": 10 }
        },
        "unknown": {
          "allow": false,
          "rateLimit": { "requestsPerMinutePerIp": 2 }
        }
      },
      "signedTokens": {
        "enabled": false,
        "secret": "shared-secret",
        "ttlSeconds": 3600
      }
    },
    "metrics": {
      "enabled": true,
      "path": "/metrics"
    }
  }
}
```

### Configuration Options

| Option                        | Type      | Default                       | Description                                         |
| ----------------------------- | --------- | ----------------------------- | --------------------------------------------------- |
| `defaultAgent`                | `string`  | First registered              | Default agent when not specified                    |
| `corsOrigins`                 | `array`   | `[]`                          | Allowed origins (glob patterns)                     |
| `rateLimit.enabled`           | `boolean` | `false` (disabled by default) | Enable rate limiting (requires `requestsPerMinute`) |
| `rateLimit.requestsPerMinute` | `number`  | `10`                          | Default rate limit                                  |
| `rateLimit.burstSize`         | `number`  | `0`                           | Burst allowance                                     |
| `metrics.enabled`             | `boolean` | `true`                        | Enable metrics endpoint                             |
| `metrics.path`                | `string`  | `/metrics`                    | Metrics endpoint path                               |

---

## Authentication

### Authentication Tiers

Configure different rate limits for different origins:

```json
{
  "auth": {
    "tiers": {
      "netdataProperties": {
        "origins": ["*.netdata.cloud"],
        "rateLimit": { "requestsPerMinute": 60 }
      },
      "agentDashboards": {
        "requireGuid": true,
        "rateLimit": { "requestsPerMinutePerGuid": 10 }
      },
      "unknown": {
        "allow": false
      }
    }
  }
}
```

### GUID-Based Authentication

For agent dashboards:

```json
{
  "auth": {
    "tiers": {
      "agentDashboards": {
        "requireGuid": true,
        "verifyGuidInCloud": false,
        "rateLimit": { "requestsPerMinutePerGuid": 10 }
      }
    }
  }
}
```

Client must include `X-Netdata-Agent-GUID` header.

### Signed Token Authentication

For secure access:

```json
{
  "auth": {
    "signedTokens": {
      "enabled": true,
      "secret": "your-shared-secret",
      "ttlSeconds": 3600
    }
  }
}
```

Client must include `Authorization: Bearer <token>` header.

---

## Metrics

When enabled, `/metrics` returns Prometheus-format metrics:

```
# HELP embed_requests_total Total embed chat requests
# TYPE embed_requests_total counter
embed_requests_total{agent="support",origin="example.com",status="success"} 42

# HELP embed_sessions_active Active embed sessions
# TYPE embed_sessions_active gauge
embed_sessions_active 5

# HELP embed_session_duration_seconds Session duration
# TYPE embed_session_duration_seconds summary
embed_session_duration_seconds_sum 123.45
embed_session_duration_seconds_count 42

# HELP embed_report_chunks_total Report chunks sent
# TYPE embed_report_chunks_total counter
embed_report_chunks_total 1234

# HELP embed_errors_total Errors by code
# TYPE embed_errors_total counter
embed_errors_total{code="rate_limited"} 3
```

---

## Transcript Persistence

Conversations are persisted under `sessionsDir/embed-conversations/`:

```
~/.ai-agent/sessions/embed-conversations/{clientId}.json.gz
```

### Transcript Format

```json
{
  "version": 1,
  "clientId": "abc-123-uuid",
  "origin": "embed",
  "updatedAt": "2026-01-16T12:34:56.789Z",
  "turns": [
    {
      "turn": 1,
      "ts": "2026-01-16T12:30:00.000Z",
      "entries": [
        { "role": "user", "content": "How do I..." },
        { "role": "status", "content": "in-progress | Searching..." },
        { "role": "assistant", "content": "Here is how..." }
      ]
    }
  ]
}
```

---

## CORS Configuration

Control which domains can access the embed API:

```json
{
  "embed": {
    "corsOrigins": ["*.example.com", "localhost:*", "https://specific-site.com"]
  }
}
```

### Origin Patterns

- `*.example.com` - Any subdomain
- `localhost:*` - Any localhost port
- `https://example.com` - Exact match

### Headers Set

```
Access-Control-Allow-Origin: <origin>
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization, X-Netdata-Agent-GUID
```

---

## Troubleshooting

### CORS errors

**Symptom**: Browser blocks requests with CORS error.

**Cause**: Origin not in `corsOrigins` list.

**Solution**: Add your domain to `embed.corsOrigins`:

```json
{
  "embed": {
    "corsOrigins": ["*.yourdomain.com"]
  }
}
```

### Rate limited

**Symptom**: `event: error` with `code: "rate_limited"`

**Cause**: Too many requests from same origin/IP/GUID.

**Solutions**:

1. Increase rate limits in config
2. Configure higher tier for your origin
3. Add authentication tokens for higher limits

### Agent not found

**Symptom**: `event: error` with `code: "agent_not_found"`

**Cause**: `agentId` doesn't match registered agent.

**Solutions**:

1. Check agent filename (without `.ai`)
2. Configure `defaultAgent` in embed config
3. Verify agent was loaded with `--agent`

### SSE connection closing

**Symptom**: Stream ends unexpectedly.

**Possible causes**:

1. Proxy/load balancer timeout
2. Client disconnect
3. Server error

**Solutions**:

1. Configure proxy for long-lived connections
2. Check browser network tab for details
3. Review server logs for errors

### JavaScript client not loading

**Symptom**: `AIAgent is not defined`

**Cause**: Script not loaded.

**Solution**: Verify script URL is correct and accessible:

```html
<script src="http://your-server:8090/ai-agent-public.js"></script>
```

---

## See Also

- [Headends](Headends) - Overview of all deployment modes
- [Headends-REST](Headends-REST) - REST API for backend integration
- [Agent Files](Agent-Files) - Agent configuration
- [specs/headend-embed.md](specs/headend-embed.md) - Technical specification
