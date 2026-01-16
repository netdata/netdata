# Configuration

Configure providers, tools, caching, and runtime behavior.

---

## Pages in This Section

| Page | Description |
|------|-------------|
| [Configuration Loading](Configuration-Loading) | Resolution order, file locations, merging |
| [LLM Providers](Configuration-Providers) | Provider setup and options |
| [MCP Tools](Configuration-MCP-Tools) | MCP server configuration |
| [REST Tools](Configuration-REST-Tools) | REST/OpenAPI tool setup |
| [Tool Filtering](Configuration-Tool-Filtering) | Allowlists and denylists |
| [Caching](Configuration-Caching) | Response and tool caching |
| [Context Window](Configuration-Context-Window) | Token budgets and guards |
| [Queues](Configuration-Queues) | Concurrency control |
| [Pricing](Configuration-Pricing) | Cost tracking configuration |
| [Parameters Reference](Configuration-Parameters) | All configuration keys |

---

## Quick Start

Create `.ai-agent.json` in your working directory:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {},
  "defaults": {
    "temperature": 0.7,
    "llmTimeout": 120000,
    "toolTimeout": 60000
  }
}
```

---

## Configuration Sections

### Providers

LLM provider credentials and settings:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    },
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  }
}
```

### MCP Servers

Tool servers using Model Context Protocol:

```json
{
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"]
    }
  }
}
```

### Queues

Concurrency control for tools:

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 }
  }
}
```

### Cache

Response caching:

```json
{
  "cache": {
    "backend": "sqlite",
    "sqlite": { "path": "${HOME}/.ai-agent/cache.db" },
    "maxEntries": 5000
  }
}
```

### Defaults

Global default settings:

```json
{
  "defaults": {
    "temperature": 0.7,
    "llmTimeout": 120000,
    "toolTimeout": 60000,
    "stream": true,
    "contextWindowBufferTokens": 256
  }
}
```

---

## Environment Variable Expansion

All string values support `${VAR}` expansion:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "${OPENAI_BASE_URL:-https://api.openai.com/v1}"
    }
  }
}
```

Default values with `:-`:
```
${VAR:-default}  → Uses 'default' if VAR is unset
```

---

## Configuration Hierarchy

Settings are merged in priority order (highest to lowest):

1. **CLI options**: `--temperature 0.2`
2. **Frontmatter**: `temperature: 0.3`
3. **Config defaults**: `defaults.temperature: 0.7`

---

## Complete Example

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "models": {
        "gpt-4o": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o"
        }
      }
    },
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}",
      "cacheStrategy": "full"
    }
  },
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-filesystem", "/tmp"],
      "queue": "default"
    },
    "brave": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-brave-search"],
      "env": {
        "BRAVE_API_KEY": "${BRAVE_API_KEY}"
      },
      "cache": "1h"
    }
  },
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 }
  },
  "cache": {
    "backend": "sqlite",
    "sqlite": { "path": "${HOME}/.ai-agent/cache.db" },
    "maxEntries": 5000
  },
  "defaults": {
    "temperature": 0.7,
    "llmTimeout": 120000,
    "toolTimeout": 60000,
    "toolResponseMaxBytes": 12288,
    "contextWindowBufferTokens": 256
  }
}
```

---

## See Also

- [Getting Started](Getting-Started) - First-time setup
- [docs/specs/configuration-loading.md](../docs/specs/configuration-loading.md) - Technical spec
