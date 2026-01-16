# Environment Variables

Environment variables for configuration and debugging.

---

## API Keys

Set provider API keys as environment variables:

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."

# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."

# Google
export GOOGLE_API_KEY="..."

# OpenRouter
export OPENROUTER_API_KEY="sk-or-..."
```

Reference them in `.ai-agent.json`:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

---

## Environment File

Create `.ai-agent.env` in your working directory (loaded automatically):

```bash
# API Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# MCP Server Keys
BRAVE_API_KEY=...
GITHUB_TOKEN=ghp_...
```

---

## Debug Variables

| Variable | Purpose | Values |
|----------|---------|--------|
| `DEBUG` | Enable AI SDK debug mode; dumps raw LLM payloads | `true`, `1` |
| `CONTEXT_DEBUG` | Enable detailed context guard debugging | `true`, `1` |

Example:

```bash
DEBUG=true ai-agent --agent test.ai "Hello"
```

---

## Provider-Specific Variables

### OpenRouter

| Variable | Purpose | Default |
|----------|---------|---------|
| `OPENROUTER_REFERER` | HTTP-Referer header | `https://ai-agent.local` |
| `OPENROUTER_TITLE` | X-OpenRouter-Title header | `ai-agent` |

### Timezone

| Variable | Purpose |
|----------|---------|
| `TZ` | Timezone override for `${DATETIME}` and `${TIMEZONE}` variables |

---

## User/System Variables

Used for prompt variable substitution:

| Variable | Purpose |
|----------|---------|
| `USER` / `USERNAME` | Username (fallback for `${USER}` prompt variable) |
| `HOME` / `USERPROFILE` | Home directory |

---

## Variable Expansion in Config

All string values in `.ai-agent.json` support `${VAR}` expansion:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "${OPENAI_BASE_URL:-https://api.openai.com/v1}"
    }
  },
  "cache": {
    "sqlite": {
      "path": "${HOME}/.ai-agent/cache.db"
    }
  }
}
```

---

## Prompt Variables

These variables can be used in `.ai` files with `${VAR}` or `{{VAR}}` syntax:

| Variable | Description | Example |
|----------|-------------|---------|
| `DATETIME` | Current date/time (RFC 3339) | `2025-08-31T02:05:07+03:00` |
| `TIMESTAMP` | Unix epoch seconds | `1733437845` |
| `DAY` | Weekday name | `Monday` |
| `TIMEZONE` | IANA timezone | `Europe/Athens` |
| `MAX_TURNS` | Configured max turns | `10` |
| `MAX_TOOLS` | Max tool calls per turn | `20` |
| `OS` | Operating system | `Ubuntu 24.04.1 LTS (kernel 6.8.0)` |
| `ARCH` | CPU architecture | `x64`, `arm64` |
| `KERNEL` | Kernel string | `Linux 6.8.0-41-generic` |
| `CD` | Current working directory | `/home/user/project` |
| `HOSTNAME` | Host name | `workstation` |
| `USER` | Username | `costa` |

Example in `.ai` file:

```yaml
---
description: Time-aware assistant
---
You are a helpful assistant.

Current time: ${DATETIME}
Timezone: ${TIMEZONE}
Running on: ${OS}
```

---

## See Also

- [Configuration](Configuration)
- [CLI Reference](Getting-Started-CLI-Reference)
