# REST Tools Configuration

Configure REST/OpenAPI endpoints as callable tools.

---

## Overview

REST tools let you expose any HTTP API as an agent tool without running an MCP server.

```json
{
  "restTools": {
    "weather": {
      "description": "Get current weather for a location",
      "method": "GET",
      "url": "https://api.weather.com/v1/current",
      "headers": {
        "Authorization": "Bearer ${WEATHER_API_KEY}"
      },
      "parametersSchema": {
        "type": "object",
        "properties": {
          "location": { "type": "string" }
        },
        "required": ["location"]
      }
    }
  }
}
```

---

## Basic Structure

```json
{
  "restTools": {
    "tool-name": {
      "description": "What this tool does",
      "method": "GET | POST | PUT | DELETE",
      "url": "https://api.example.com/endpoint",
      "headers": { ... },
      "parametersSchema": { ... },
      "bodyTemplate": { ... }
    }
  }
}
```

---

## GET Request

```json
{
  "restTools": {
    "search": {
      "description": "Search for items",
      "method": "GET",
      "url": "https://api.example.com/search?q=${parameters.query}&limit=${parameters.limit}",
      "parametersSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" },
          "limit": { "type": "number", "default": 10 }
        },
        "required": ["query"]
      }
    }
  }
}
```

---

## POST Request with Body

```json
{
  "restTools": {
    "create-item": {
      "description": "Create a new item",
      "method": "POST",
      "url": "https://api.example.com/items",
      "headers": {
        "Content-Type": "application/json",
        "Authorization": "Bearer ${API_KEY}"
      },
      "parametersSchema": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["name"]
      },
      "bodyTemplate": {
        "item": {
          "name": "${parameters.name}",
          "description": "${parameters.description}"
        }
      }
    }
  }
}
```

---

## Real-World Example: PostHog Query

```json
{
  "restTools": {
    "posthog-query": {
      "description": "Run a HogQL query via PostHog Query API",
      "method": "POST",
      "url": "https://app.posthog.com/api/projects/${POSTHOG_PROJECT}/query",
      "headers": {
        "Authorization": "Bearer ${POSTHOG_API_KEY}",
        "Content-Type": "application/json"
      },
      "parametersSchema": {
        "type": "object",
        "properties": {
          "hogql": {
            "type": "string",
            "description": "HogQL query to execute"
          }
        },
        "required": ["hogql"]
      },
      "bodyTemplate": {
        "query": {
          "kind": "HogQLQuery",
          "query": "${parameters.hogql}"
        }
      },
      "queue": "posthog"
    }
  }
}
```

---

## Configuration Options

| Option | Type | Description |
|--------|------|-------------|
| `description` | `string` | Tool description for LLM |
| `method` | `string` | HTTP method |
| `url` | `string` | Endpoint URL (supports variable substitution) |
| `headers` | `object` | HTTP headers |
| `parametersSchema` | `object` | JSON Schema for tool parameters |
| `bodyTemplate` | `object` | Request body template |
| `queue` | `string` | Concurrency queue name |
| `cache` | `string` | Response cache TTL |

---

## Variable Substitution

### In URL

```json
{
  "url": "https://api.example.com/users/${parameters.userId}"
}
```

### In Headers

```json
{
  "headers": {
    "Authorization": "Bearer ${API_KEY}"
  }
}
```

### In Body

```json
{
  "bodyTemplate": {
    "query": "${parameters.query}",
    "filters": {
      "date": "${parameters.date}"
    }
  }
}
```

---

## Caching

```json
{
  "restTools": {
    "expensive-query": {
      "description": "Run expensive query",
      "method": "POST",
      "url": "https://api.example.com/query",
      "cache": "1h"
    }
  }
}
```

Cache TTL formats:
- `5m` - 5 minutes
- `1h` - 1 hour
- `1d` - 1 day
- `12345` - milliseconds

---

## Concurrency Control

```json
{
  "queues": {
    "api-heavy": { "concurrent": 2 }
  },
  "restTools": {
    "slow-api": {
      "description": "Calls slow API",
      "method": "GET",
      "url": "https://slow.api.com/data",
      "queue": "api-heavy"
    }
  }
}
```

---

## Using in Agents

Reference REST tools in frontmatter:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - posthog-query
  - weather
---
You can query PostHog and get weather data.
```

Tool calls use namespace: `rest__toolname`

---

## Error Handling

REST tool errors are returned to the LLM:

```json
{
  "error": "HTTP 401 Unauthorized",
  "status": 401,
  "body": "Invalid API key"
}
```

The LLM can then decide how to proceed.

---

## See Also

- [Configuration](Configuration) - Overview
- [MCP Tools](Configuration-MCP-Tools) - MCP server tools
- [Queues](Configuration-Queues) - Concurrency control
