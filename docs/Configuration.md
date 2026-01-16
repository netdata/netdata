# Configuration

System-wide configuration for providers, tools, caching, and runtime behavior.

---

## Table of Contents

- [Pages in This Section](#pages-in-this-section) - Navigate to specific configuration topics
- [Quick Start](#quick-start) - Minimal configuration to get started
- [Configuration Sections](#configuration-sections) - Overview of each config section
- [Environment Variable Expansion](#environment-variable-expansion) - Dynamic values in config
- [Configuration Hierarchy](#configuration-hierarchy) - Priority and merging rules
- [Complete Example](#complete-example) - Full production configuration
- [See Also](#see-also) - Related documentation

---

## Pages in This Section

| Page                                             | Description                                                 |
| ------------------------------------------------ | ----------------------------------------------------------- |
| [Configuration Files](Configuration-Files)       | File locations, resolution order, layer merging             |
| [LLM Providers](Configuration-Providers)         | OpenAI, Anthropic, Google, Ollama, OpenRouter setup         |
| [MCP Servers](Configuration-MCP-Servers)         | MCP tool server configuration (stdio, HTTP, SSE, WebSocket) |
| [REST Tools](Configuration-REST-Tools)           | REST/OpenAPI endpoint tools                                 |
| [Tool Filtering](Configuration-Tool-Filtering)   | Allowlists, denylists, and wildcard patterns                |
| [Caching](Configuration-Caching)                 | Response caching for agents and tools                       |
| [Context Window](Configuration-Context-Window)   | Token budgets, limits, and guards                           |
| [Queues](Configuration-Queues)                   | Concurrency control and rate limiting                       |
| [Pricing](Configuration-Pricing)                 | Cost tracking and token pricing                             |
| [Parameters Reference](Configuration-Parameters) | Complete reference of all configuration keys                |

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
    "temperature": 0.2,
    "llmTimeout": 600000,
    "toolTimeout": 300000
  }
}
```

Set your API key in the environment:

```bash
export OPENAI_API_KEY="sk-..."
```

Or create `.ai-agent.env` in the same directory:

```bash
OPENAI_API_KEY=sk-...
```

---

## Configuration Sections

### providers

LLM provider credentials and settings.

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

See [LLM Providers](Configuration-Providers) for complete provider configuration.

### mcpServers

Tool servers using Model Context Protocol.

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

See [MCP Servers](Configuration-MCP-Servers) for transport types and options.

### restTools

REST/OpenAPI endpoints exposed as tools.

```json
{
  "restTools": {
    "weather": {
      "description": "Get weather for a location",
      "method": "GET",
      "url": "https://api.weather.com/current?location=${parameters.location}",
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

See [REST Tools](Configuration-REST-Tools) for complete options.

### queues

Concurrency control for tools.

```json
{
  "queues": {
    "default": { "concurrent": 32 },
    "heavy": { "concurrent": 2 }
  }
}
```

See [Queues](Configuration-Queues) for queue assignment and patterns.

### cache

Response caching backend.

```json
{
  "cache": {
    "backend": "sqlite",
    "sqlite": { "path": "${HOME}/.ai-agent/cache.db" },
    "maxEntries": 5000
  }
}
```

See [Caching](Configuration-Caching) for backend options and TTL configuration.

### defaults

Global default settings for all agents.

```json
{
  "defaults": {
    "temperature": 0.2,
    "llmTimeout": 600000,
    "toolTimeout": 300000,
    "stream": false,
    "contextWindowBufferTokens": 8192
  }
}
```

See [Parameters Reference](Configuration-Parameters) for all available defaults.

### pricing

Token pricing for cost tracking.

```json
{
  "pricing": {
    "openai": {
      "gpt-4o": {
        "unit": "per_1m",
        "prompt": 2.5,
        "completion": 10.0
      }
    }
  }
}
```

See [Pricing](Configuration-Pricing) for cost calculation details.

### accounting

Output file for usage accounting.

```json
{
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
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
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "${OPENAI_BASE_URL}"
    }
  }
}
```

### Syntax

| Syntax   | Behavior                                   |
| -------- | ------------------------------------------ |
| `${VAR}` | Substitute with environment variable value |

### Variable Sources

Variables are resolved from:

1. **Layer-specific `.ai-agent.env`** file (same directory as config)
2. **Process environment** (`process.env`)

> **Note:** MCP server `env` and `headers` values are NOT expanded at config load time. They pass through to child processes for runtime resolution.

---

## Configuration Hierarchy

Settings merge from multiple sources with this priority (highest to lowest):

| Priority | Source            | Example                     |
| -------- | ----------------- | --------------------------- |
| 1        | CLI options       | `--temperature 0.2`         |
| 2        | Agent frontmatter | `temperature: 0.3`          |
| 3        | Config defaults   | `defaults.temperature: 0.7` |
| 4        | Built-in defaults | Hardcoded fallbacks         |

### Example

Config file:

```json
{
  "defaults": {
    "temperature": 0.7,
    "maxTurns": 10
  }
}
```

Agent frontmatter:

```yaml
---
temperature: 0.3
---
```

CLI:

```bash
ai-agent --agent test.ai --temperature 0.2
```

**Result:** `temperature = 0.2` (CLI wins)

---

## Complete Example

Production-ready configuration:

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
      "apiKey": "${ANTHROPIC_API_KEY}"
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
  "pricing": {
    "openai": {
      "gpt-4o": {
        "unit": "per_1m",
        "prompt": 2.5,
        "completion": 10.0
      }
    },
    "anthropic": {
      "claude-sonnet-4-20250514": {
        "unit": "per_1m",
        "prompt": 3.0,
        "completion": 15.0,
        "cacheRead": 0.3,
        "cacheWrite": 3.75
      }
    }
  },
  "defaults": {
    "temperature": 0.2,
    "llmTimeout": 600000,
    "toolTimeout": 300000,
    "toolResponseMaxBytes": 12288,
    "contextWindowBufferTokens": 8192
  },
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
  }
}
```

---

## See Also

- [Getting Started](Getting-Started) - First-time setup and installation
- [Agent Files](Agent-Files) - Per-agent configuration in frontmatter
- [CLI Reference](CLI) - Command-line options
- [specs/configuration-loading.md](specs/configuration-loading.md) - Technical specification
