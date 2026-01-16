# REST Tools Configuration

Configure REST API endpoints as tools for agents.

---

## Table of Contents

- [Overview](#overview) - What REST tools provide
- [Basic Configuration](#basic-configuration) - Simple REST tool setup
- [URL Templating](#url-templating) - Dynamic URL parameters
- [Request Headers](#request-headers) - Authentication and custom headers
- [Request Body](#request-body) - POST/PUT/PATCH request bodies
- [Parameters Schema](#parameters-schema) - Input validation
- [Complex Query Parameters](#complex-query-parameters) - Nested query structures
- [JSON Streaming](#json-streaming) - Server-sent events and streaming
- [REST Tool Configuration Reference](#rest-tool-configuration-reference) - All options
- [Caching](#caching) - Response caching
- [Concurrency Queues](#concurrency-queues) - Rate limiting
- [Using in Agents](#using-in-agents) - Referencing tools
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related documentation

---

## Overview

REST tools expose HTTP API endpoints as agent tools. Features:

- **Dynamic URL/body templates**: `${parameters.name}` substitution
- **Input validation**: JSON Schema validation before requests
- **Streaming support**: Parse streaming JSON responses
- **Caching**: Response caching with configurable TTL
- **Queue integration**: Concurrency control

Tool naming: `rest__<name>`

---

## Basic Configuration

### GET Request

```json
{
  "restTools": {
    "weather": {
      "description": "Get current weather for a location",
      "method": "GET",
      "url": "https://api.weather.com/current?location=${parameters.location}",
      "parametersSchema": {
        "type": "object",
        "properties": {
          "location": {
            "type": "string",
            "description": "City name or coordinates"
          }
        },
        "required": ["location"]
      }
    }
  }
}
```

### POST Request

```json
{
  "restTools": {
    "create_ticket": {
      "description": "Create a support ticket",
      "method": "POST",
      "url": "https://api.helpdesk.com/tickets",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      },
      "bodyTemplate": {
        "subject": "${parameters.subject}",
        "description": "${parameters.description}",
        "priority": "${parameters.priority}"
      },
      "parametersSchema": {
        "type": "object",
        "properties": {
          "subject": { "type": "string" },
          "description": { "type": "string" },
          "priority": { "type": "string", "enum": ["low", "medium", "high"] }
        },
        "required": ["subject", "description"]
      }
    }
  }
}
```

---

## URL Templating

### Basic Substitution

Use `${parameters.name}` to insert parameter values:

```json
{
  "url": "https://api.example.com/users/${parameters.userId}"
}
```

URL values are **automatically URL-encoded** using `encodeURIComponent`.

### Query Parameters

```json
{
  "url": "https://api.example.com/search?q=${parameters.query}&limit=${parameters.limit}"
}
```

### Path Parameters

```json
{
  "url": "https://api.example.com/repos/${parameters.owner}/${parameters.repo}/issues"
}
```

### Combined Example

```json
{
  "url": "https://api.github.com/repos/${parameters.owner}/${parameters.repo}/issues?state=${parameters.state}&per_page=${parameters.limit}"
}
```

---

## Request Headers

### Static Headers

```json
{
  "headers": {
    "Content-Type": "application/json",
    "Accept": "application/json"
  }
}
```

### Dynamic Headers

Headers support `${parameters.name}` substitution:

```json
{
  "headers": {
    "Authorization": "Bearer ${parameters.token}",
    "X-Custom-ID": "${parameters.requestId}"
  }
}
```

### Environment Variables

Headers support `${VAR}` expansion from environment:

```json
{
  "headers": {
    "Authorization": "Bearer ${API_KEY}",
    "X-Org-ID": "${ORG_ID}"
  }
}
```

### Content-Type Default

When `bodyTemplate` is defined:
- If `Content-Type` not set, defaults to `application/json`
- To override, explicitly set `Content-Type` in headers

---

## Request Body

### Object Template

```json
{
  "bodyTemplate": {
    "name": "${parameters.name}",
    "email": "${parameters.email}",
    "settings": {
      "notifications": true,
      "theme": "${parameters.theme}"
    }
  }
}
```

**Result:** JSON-serialized object with substituted values.

### Single Parameter Body

For passing the entire parameter as the body:

```json
{
  "bodyTemplate": "${parameters.payload}"
}
```

When the template is exactly `${parameters.name}`, the raw parameter value is used directly (not string-interpolated).

### Nested Substitution

Templates support nested objects:

```json
{
  "bodyTemplate": {
    "user": {
      "firstName": "${parameters.firstName}",
      "lastName": "${parameters.lastName}"
    },
    "metadata": {
      "source": "ai-agent",
      "timestamp": "${parameters.timestamp}"
    }
  }
}
```

---

## Parameters Schema

JSON Schema for validating tool inputs.

### Basic Schema

```json
{
  "parametersSchema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Search query"
      },
      "limit": {
        "type": "integer",
        "minimum": 1,
        "maximum": 100,
        "default": 10
      }
    },
    "required": ["query"]
  }
}
```

### Type Reference

| Type | Description | Example |
|------|-------------|---------|
| `string` | Text value | `"hello"` |
| `integer` | Whole number | `42` |
| `number` | Any number | `3.14` |
| `boolean` | True/false | `true` |
| `array` | List of values | `[1, 2, 3]` |
| `object` | Nested object | `{"key": "value"}` |

### Enum Constraint

```json
{
  "properties": {
    "status": {
      "type": "string",
      "enum": ["open", "closed", "pending"],
      "description": "Filter by status"
    }
  }
}
```

### Array Parameters

```json
{
  "properties": {
    "tags": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Filter by tags"
    }
  }
}
```

### Nested Object Parameters

```json
{
  "properties": {
    "filter": {
      "type": "object",
      "properties": {
        "startDate": { "type": "string", "format": "date" },
        "endDate": { "type": "string", "format": "date" }
      }
    }
  }
}
```

---

## Complex Query Parameters

For APIs requiring nested query parameters (arrays, objects).

### Configuration

```json
{
  "restTools": {
    "search": {
      "method": "GET",
      "url": "https://api.example.com/search",
      "hasComplexQueryParams": true,
      "queryParamNames": ["filters", "sort"],
      "parametersSchema": {
        "type": "object",
        "properties": {
          "q": { "type": "string" },
          "filters": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "field": { "type": "string" },
                "value": { "type": "string" }
              }
            }
          },
          "sort": {
            "type": "object",
            "properties": {
              "field": { "type": "string" },
              "order": { "type": "string" }
            }
          }
        }
      }
    }
  }
}
```

### Serialization Format

| Input | Serialized |
|-------|------------|
| `filters: [{ field: "email", value: "test@example.com" }]` | `filters[0][field]=email&filters[0][value]=test%40example.com` |
| `sort: { field: "date", order: "desc" }` | `sort[field]=date&sort[order]=desc` |

### hasComplexQueryParams Behavior

| Setting | Behavior |
|---------|----------|
| `false` (default) | Standard URL encoding |
| `true` | Nested bracket serialization for `queryParamNames` only |

---

## JSON Streaming

Parse streaming JSON responses (SSE-style).

### Configuration

```json
{
  "restTools": {
    "chat": {
      "method": "POST",
      "url": "https://api.example.com/chat",
      "bodyTemplate": {
        "message": "${parameters.message}"
      },
      "streaming": {
        "mode": "json-stream",
        "linePrefix": "data: ",
        "discriminatorField": "type",
        "doneValue": "done",
        "tokenField": "content",
        "tokenValue": "token",
        "answerField": "answer",
        "timeoutMs": 60000
      },
      "parametersSchema": {
        "type": "object",
        "properties": {
          "message": { "type": "string" }
        },
        "required": ["message"]
      }
    }
  }
}
```

### Streaming Configuration Reference

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `mode` | `string` | Required | Must be `"json-stream"` |
| `linePrefix` | `string` | `""` | Prefix to strip from each line |
| `discriminatorField` | `string` | `"type"` | Field name for event type |
| `doneValue` | `string` | `"done"` | Value indicating stream end |
| `tokenField` | `string` | `"content"` | Field containing token content |
| `tokenValue` | `string` | `"token"` | Discriminator value for tokens |
| `answerField` | `string` | `"answer"` | Field containing final answer |
| `timeoutMs` | `number \| string` | `60000` | Stream read timeout |

### Stream Processing

Example stream input:
```
data: {"type": "token", "content": "Hello"}
data: {"type": "token", "content": " world"}
data: {"type": "done", "answer": "Hello world"}
```

1. Lines are buffered and split on newlines
2. `linePrefix` (`data: `) is stripped
3. JSON is parsed from each line
4. `discriminatorField` (`type`) determines handling
5. Tokens are accumulated from `tokenField`
6. Final answer from `answerField` on done event

---

## REST Tool Configuration Reference

Complete REST tool schema:

```json
{
  "restTools": {
    "<name>": {
      "description": "string",
      "method": "GET | POST | PUT | DELETE | PATCH",
      "url": "string",
      "headers": { "string": "string" },
      "parametersSchema": { "JSON Schema object" },
      "bodyTemplate": { "any JSON structure" },
      "queue": "string",
      "cache": "string | number",
      "streaming": {
        "mode": "json-stream",
        "linePrefix": "string",
        "discriminatorField": "string",
        "doneValue": "string",
        "answerField": "string",
        "tokenValue": "string",
        "tokenField": "string",
        "timeoutMs": "number | string"
      },
      "hasComplexQueryParams": "boolean",
      "queryParamNames": ["string"]
    }
  }
}
```

### All Options

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `description` | `string` | Required | Tool description for the LLM |
| `method` | `string` | Required | HTTP method |
| `url` | `string` | Required | URL template |
| `headers` | `object` | `{}` | HTTP headers |
| `parametersSchema` | `object` | Required | JSON Schema for inputs |
| `bodyTemplate` | `any` | - | Request body template |
| `queue` | `string` | `"default"` | Concurrency queue name |
| `cache` | `string \| number` | `"off"` | Response cache TTL |
| `streaming` | `object` | - | JSON streaming config |
| `hasComplexQueryParams` | `boolean` | `false` | Enable nested query params |
| `queryParamNames` | `string[]` | `[]` | Params to serialize complexly |

---

## Caching

### Basic Cache

```json
{
  "restTools": {
    "lookup": {
      "method": "GET",
      "url": "https://api.example.com/data/${parameters.id}",
      "cache": "1h",
      "parametersSchema": {
        "type": "object",
        "properties": {
          "id": { "type": "string" }
        },
        "required": ["id"]
      }
    }
  }
}
```

### Cache TTL Formats

| Format | Example | Duration |
|--------|---------|----------|
| `"off"` | `"off"` | Disabled |
| Milliseconds | `60000` | 60 seconds |
| Seconds | `"30s"` | 30 seconds |
| Minutes | `"5m"` | 5 minutes |
| Hours | `"1h"` | 1 hour |
| Days | `"1d"` | 1 day |

### Cache Key

Cache key is computed from:
- Tool name
- All parameter values

Same parameters = same cache key.

---

## Concurrency Queues

Limit concurrent requests:

```json
{
  "queues": {
    "rate-limited": { "concurrent": 2 }
  },
  "restTools": {
    "external-api": {
      "method": "GET",
      "url": "https://api.example.com/data",
      "queue": "rate-limited",
      "parametersSchema": { "type": "object" }
    }
  }
}
```

See [Queues](Configuration-Queues) for complete queue configuration.

---

## Using in Agents

Reference REST tools in agent frontmatter:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - restTools
---
You have access to the weather API.
```

Or reference specific tools:

```yaml
---
tools:
  - rest__weather
  - rest__create_ticket
---
```

### Tool Naming

| Config Key | Tool Name |
|------------|-----------|
| `weather` | `rest__weather` |
| `create_ticket` | `rest__create_ticket` |
| `search_api` | `rest__search_api` |

---

## Troubleshooting

### Invalid arguments error

```
Error: Invalid arguments: ...
```

**Causes:**
- Parameter doesn't match schema
- Missing required parameter
- Wrong type

**Solutions:**
1. Check `parametersSchema` definition
2. Verify required fields
3. Check type constraints (string vs number)

### URL substitution failure

```
Error: Missing parameter 'name' for URL substitution
```

**Causes:**
- Parameter referenced in URL but not in schema
- Typo in `${parameters.name}`

**Solutions:**
1. Add parameter to `parametersSchema`
2. Check placeholder spelling

### Request timeout

```
Error: Request timeout
```

**Causes:**
- API slow to respond
- Network issues
- Large response

**Solutions:**
1. Increase timeout via streaming config:
```json
{
  "streaming": {
    "timeoutMs": 120000
  }
}
```
2. Check API status
3. Use `--verbose` to see timing

### Streaming parse failure

```
Error: Failed to parse streaming response
```

**Causes:**
- Incorrect `linePrefix`
- Wrong `discriminatorField`
- Invalid JSON in stream

**Solutions:**
1. Verify line prefix matches exactly
2. Check discriminator field name
3. Test API response manually
4. Use debug tracing (see below)

### Missing body

**Symptoms:** API returns error about missing body

**Solutions:**
1. Define `bodyTemplate`
2. Verify template syntax
3. Check method is POST/PUT/PATCH

### Header not applied

**Symptoms:** API authentication fails

**Solutions:**
1. Check headers object syntax
2. Verify environment variables are set
3. Check header name spelling

### Debug Tracing

Enable REST call tracing:

```bash
export DEBUG_REST_CALLS=true
ai-agent --agent myagent.ai "query"
```

Or:

```bash
export TRACE_REST=true
```

Logs:
- Method and URL
- Headers (authorization redacted)
- Body (truncated to 500 chars)

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [MCP Servers](Configuration-MCP-Servers) - MCP tool servers
- [Tool Filtering](Configuration-Tool-Filtering) - Access control
- [Caching](Configuration-Caching) - Cache configuration
- [specs/tools-rest.md](specs/tools-rest.md) - Technical specification
