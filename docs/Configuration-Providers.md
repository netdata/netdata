# LLM Providers

Configure LLM providers for AI Agent.

---

## Supported Providers

| Type | Provider | Description |
|------|----------|-------------|
| `openai` | OpenAI | GPT-4o, GPT-4o-mini, etc. |
| `anthropic` | Anthropic | Claude 3.5, Claude 3, etc. |
| `google` | Google | Gemini Pro, Gemini Flash, etc. |
| `openrouter` | OpenRouter | Multi-provider gateway |
| `ollama` | Ollama | Local models |
| `openai-compatible` | Custom | Any OpenAI-compatible API |

---

## OpenAI

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "https://api.openai.com/v1"
    }
  }
}
```

Usage in agent:
```yaml
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
```

---

## Anthropic

```json
{
  "providers": {
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}",
      "cacheStrategy": "full"
    }
  }
}
```

### Cache Strategy

- `full` (default): Apply ephemeral cache control
- `none`: Disable cache control

Usage:
```yaml
models:
  - anthropic/claude-sonnet-4-20250514
  - anthropic/claude-3-haiku-20240307
```

---

## Google

```json
{
  "providers": {
    "google": {
      "type": "google",
      "apiKey": "${GOOGLE_API_KEY}"
    }
  }
}
```

Usage:
```yaml
models:
  - google/gemini-1.5-pro
  - google/gemini-1.5-flash
```

---

## OpenRouter

```json
{
  "providers": {
    "openrouter": {
      "type": "openrouter",
      "apiKey": "${OPENROUTER_API_KEY}"
    }
  }
}
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `OPENROUTER_REFERER` | HTTP-Referer header | `https://ai-agent.local` |
| `OPENROUTER_TITLE` | X-OpenRouter-Title header | `ai-agent` |

Usage:
```yaml
models:
  - openrouter/openai/gpt-4o
  - openrouter/anthropic/claude-3-sonnet
  - openrouter/meta-llama/llama-3-70b
```

---

## Ollama

```json
{
  "providers": {
    "ollama": {
      "type": "ollama",
      "baseUrl": "http://localhost:11434"
    }
  }
}
```

Usage:
```yaml
models:
  - ollama/llama3
  - ollama/mixtral
```

---

## OpenAI-Compatible

For self-hosted or custom APIs:

```json
{
  "providers": {
    "nova": {
      "type": "openai-compatible",
      "apiKey": "${NOVA_API_KEY}",
      "baseUrl": "http://10.20.4.21:8090/v1"
    }
  }
}
```

Usage:
```yaml
models:
  - nova/gpt-oss-20b
```

---

## Per-Model Configuration

Override settings for specific models:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "models": {
        "gpt-4o": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o",
          "overrides": {
            "temperature": 0.5
          }
        },
        "gpt-4o-mini": {
          "contextWindow": 128000,
          "overrides": {
            "top_p": null
          }
        }
      }
    }
  }
}
```

### Model Settings

| Key | Description |
|-----|-------------|
| `contextWindow` | Maximum tokens |
| `tokenizer` | Tokenizer ID (e.g., `tiktoken:gpt-4o`) |
| `interleaved` | Enable interleaved reasoning replay |
| `overrides` | LLM parameter overrides |

### Overrides

Force specific values or disable parameters:

```json
{
  "overrides": {
    "temperature": 0.2,
    "top_p": null,
    "repeat_penalty": null
  }
}
```

Use `null` to omit a parameter entirely.

---

## Interleaved Reasoning

For models requiring reasoning replay:

```json
{
  "providers": {
    "nova": {
      "type": "openai-compatible",
      "models": {
        "thinking-model": {
          "interleaved": true
        }
      }
    }
  }
}
```

Options:
- `true`: Use `reasoning_content` field
- `"custom_field"`: Use specified field name

---

## Fallback Chains

Models are tried in order:

```yaml
# Agent frontmatter
models:
  - openai/gpt-4o          # Try first
  - anthropic/claude-3-haiku   # Fallback 1
  - ollama/llama3          # Fallback 2
```

On failure:
1. Same model, next provider (if multiple providers for same model)
2. Next model in list

---

## Custom Headers

Add custom headers to requests:

```json
{
  "providers": {
    "custom": {
      "type": "openai-compatible",
      "apiKey": "${API_KEY}",
      "baseUrl": "https://api.example.com/v1",
      "headers": {
        "X-Custom-Header": "value"
      }
    }
  }
}
```

---

## See Also

- [Configuration](Configuration) - Overview
- [Context Window](Configuration-Context-Window) - Token budgets
- [docs/specs/providers-*.md](../docs/specs/) - Provider specifications
