# Environment Variables

**Overview**: All environment variables that configure ai-agent behavior.

---

## Table of Contents

- [Overview](#overview) - How environment variables work in ai-agent
- [API Keys](#api-keys) - LLM provider authentication
- [Environment File](#environment-file) - Persistent configuration
- [Debug Variables](#debug-variables) - Debugging and logging
- [Provider-Specific Variables](#provider-specific-variables) - Provider customization
- [Runtime Variables](#runtime-variables) - System information variables
- [Prompt Variables](#prompt-variables) - Variables available in .ai files
- [Variable Expansion in Config](#variable-expansion-in-config) - Using variables in configuration
- [Quick Reference](#quick-reference) - All variables in one table
- [See Also](#see-also) - Related documentation

---

## Overview

ai-agent uses environment variables for:

1. **Sensitive data** - API keys, tokens (never hardcode in config files)
2. **Environment-specific settings** - Debug flags, paths
3. **Prompt substitution** - Dynamic values in `.ai` system prompts

**Variable sources (in priority order):**

| Priority | Source                               | Description        |
| -------- | ------------------------------------ | ------------------ |
| 1        | Shell environment                    | `export VAR=value` |
| 2        | `.ai-agent.env` in current directory | Project-specific   |
| 3        | `~/.ai-agent/ai-agent.env`           | User-wide defaults |

---

## API Keys

Set provider API keys as environment variables. Never hardcode keys in configuration files.

### Provider Keys

| Variable             | Provider   | Format       | How to Get                                                           |
| -------------------- | ---------- | ------------ | -------------------------------------------------------------------- |
| `OPENAI_API_KEY`     | OpenAI     | `sk-...`     | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| `ANTHROPIC_API_KEY`  | Anthropic  | `sk-ant-...` | [console.anthropic.com](https://console.anthropic.com/)              |
| `GOOGLE_API_KEY`     | Google AI  | `AIza...`    | [aistudio.google.com](https://aistudio.google.com/)                  |
| `OPENROUTER_API_KEY` | OpenRouter | `sk-or-...`  | [openrouter.ai/keys](https://openrouter.ai/keys)                     |

### Setting API Keys

**In shell (temporary, current session only):**

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

**In shell profile (persistent):**

```bash
# Add to ~/.bashrc, ~/.zshrc, or ~/.profile
echo 'export OPENAI_API_KEY="sk-..."' >> ~/.bashrc
source ~/.bashrc
```

**Verification:**

```bash
echo $OPENAI_API_KEY
# Should output: sk-... (your key)
```

### Referencing in Configuration

Use `${VAR}` syntax in `.ai-agent.json`:

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

> **Security:** The `${VAR}` syntax means your actual key never appears in config files. Safe to commit config to version control.

---

## Environment File

Store environment variables in a file for persistent configuration.

### File Locations

| Location                   | Scope                       | Priority |
| -------------------------- | --------------------------- | -------- |
| `.ai-agent.env`            | Current directory (project) | Higher   |
| `~/.ai-agent/ai-agent.env` | User home (global)          | Lower    |

### File Format

Plain text, one variable per line:

```bash
# API Keys (required)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...

# MCP Server Keys (optional, for specific tools)
BRAVE_API_KEY=BSA...
GITHUB_TOKEN=ghp_...
SLACK_BOT_TOKEN=xoxb-...

# Debug settings (optional)
# DEBUG=true
# CONTEXT_DEBUG=true
```

### Creating the File

**For user-wide configuration:**

```bash
mkdir -p ~/.ai-agent
cat > ~/.ai-agent/ai-agent.env << 'EOF'
# LLM Provider API Keys
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
EOF
chmod 600 ~/.ai-agent/ai-agent.env
```

**For project-specific configuration:**

```bash
cat > .ai-agent.env << 'EOF'
# Project-specific keys
BRAVE_API_KEY=BSA...
EOF
chmod 600 .ai-agent.env
echo ".ai-agent.env" >> .gitignore
```

> **Security:** Use `chmod 600` to restrict file access. Add to `.gitignore` to prevent committing secrets.

---

## Debug Variables

Enable debugging output for troubleshooting.

| Variable        | Type    | Default | Description                                                   |
| --------------- | ------- | ------- | ------------------------------------------------------------- |
| `DEBUG`         | boolean | `false` | Enable AI SDK debug mode; dumps raw LLM payloads to stderr    |
| `CONTEXT_DEBUG` | boolean | `false` | Enable detailed context guard debugging; shows token counting |

### Usage

**Enable LLM payload debugging:**

```bash
DEBUG=true ai-agent --agent test.ai "Hello"
```

**Expected additional output:**

```
[DEBUG] LLM Request: {
  model: "openai/gpt-4o-mini",
  messages: [...],
  tools: [...]
}
[DEBUG] LLM Response: {
  content: "Hello! ...",
  usage: { promptTokens: 123, completionTokens: 45 }
}
```

**Enable context window debugging:**

```bash
CONTEXT_DEBUG=true ai-agent --agent test.ai "Hello"
```

**Expected additional output:**

```
[CONTEXT] Token count: 1234/128000 (1%)
[CONTEXT] Message compression: none needed
```

**Enable both:**

```bash
DEBUG=true CONTEXT_DEBUG=true ai-agent --agent test.ai "Hello"
```

---

## Provider-Specific Variables

Some providers support additional configuration via environment variables.

### OpenRouter

| Variable             | Type   | Default                  | Description                            |
| -------------------- | ------ | ------------------------ | -------------------------------------- |
| `OPENROUTER_REFERER` | string | `https://ai-agent.local` | HTTP-Referer header sent to OpenRouter |
| `OPENROUTER_TITLE`   | string | `ai-agent`               | X-OpenRouter-Title header              |

**Example:**

```bash
export OPENROUTER_REFERER="https://myapp.example.com"
export OPENROUTER_TITLE="My Application"
```

### Timezone

| Variable | Type   | Default        | Description                                                   |
| -------- | ------ | -------------- | ------------------------------------------------------------- |
| `TZ`     | string | System default | Timezone for `${DATETIME}` and `${TIMEZONE}` prompt variables |

**Example:**

```bash
export TZ="America/New_York"
ai-agent --agent my-agent.ai "What time is it?"
```

---

## Runtime Variables

System information variables used internally and available for prompt substitution.

| Variable      | Source           | Description               | Example Value         |
| ------------- | ---------------- | ------------------------- | --------------------- |
| `USER`        | System           | Current username          | `costa`               |
| `USERNAME`    | System (Windows) | Fallback for USER         | `costa`               |
| `HOME`        | System           | Home directory path       | `/home/costa`         |
| `USERPROFILE` | System (Windows) | Fallback for HOME         | `C:\Users\costa`      |
| `PWD`         | System           | Current working directory | `/home/costa/project` |

These are read-only and come from the operating system.

---

## Prompt Variables

These variables can be used in `.ai` file system prompts with `${VAR}` or `{{VAR}}` syntax.

### Available Variables

| Variable    | Description                  | Example Value                       |
| ----------- | ---------------------------- | ----------------------------------- |
| `DATETIME`  | Current date/time (RFC 3339) | `2025-08-31T02:05:07+03:00`         |
| `TIMESTAMP` | Unix epoch seconds           | `1733437845`                        |
| `DAY`       | Weekday name                 | `Monday`                            |
| `TIMEZONE`  | IANA timezone                | `Europe/Athens`                     |
| `MAX_TURNS` | Configured max turns         | `10`                                |
| `MAX_TOOLS` | Max tool calls per turn      | `10`                                |
| `OS`        | Operating system             | `Ubuntu 24.04.1 LTS (kernel 6.8.0)` |
| `ARCH`      | CPU architecture             | `x64`, `arm64`                      |
| `KERNEL`    | Kernel string                | `Linux 6.8.0-41-generic`            |
| `CD`        | Current working directory    | `/home/user/project`                |
| `HOSTNAME`  | Host name                    | `workstation`                       |
| `USER`      | Username                     | `costa`                             |

### Usage in .ai Files

```yaml
#!/usr/bin/env ai-agent
---
description: Time-aware assistant
models:
  - openai/gpt-4o-mini
maxTurns: 5
---
You are a helpful assistant.

## Current Context

- **Date/Time**: ${DATETIME}
- **Timezone**: ${TIMEZONE}
- **Day**: ${DAY}
- **User**: ${USER}
- **Working Directory**: ${CD}
- **System**: ${OS} (${ARCH})

## Limits

You have ${MAX_TURNS} turns and can make up to ${MAX_TOOLS} tool calls per turn.
```

**Rendered system prompt (example):**

```
You are a helpful assistant.

## Current Context

- **Date/Time**: 2025-01-16T14:30:00+02:00
- **Timezone**: Europe/Athens
- **Day**: Thursday
- **User**: costa
- **Working Directory**: /home/costa/project
- **System**: Ubuntu 24.04.1 LTS (kernel 6.8.0) (x64)

## Limits

You have 5 turns and can make up to 20 tool calls per turn.
```

---

## Variable Expansion in Config

All string values in `.ai-agent.json` support `${VAR}` expansion.

### Basic Expansion

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### Nested Expansion

Variables can reference other variables:

```json
{
  "cache": {
    "sqlite": {
      "path": "${HOME}/.ai-agent/cache.db"
    }
  }
}
```

### Behavior

- Variables are expanded from environment variables and the layer's `.ai-agent.env` file
- Missing variables cause a clear error at startup indicating which file needs the value
- Variables are expanded per layer, not globally

```

---

## Quick Reference

All environment variables in one table:

| Variable             | Category | Type    | Default                  | Description                     |
| -------------------- | -------- | ------- | ------------------------ | ------------------------------- |
| `OPENAI_API_KEY` | API Key | string | Required | OpenAI API key |
| `ANTHROPIC_API_KEY` | API Key | string | Required | Anthropic API key |
| `GOOGLE_API_KEY` | API Key | string | Required | Google AI API key |
| `OPENROUTER_API_KEY` | API Key | string | Required | OpenRouter API key |
| `DEBUG`              | Debug    | boolean | `false`                  | Enable LLM payload debugging    |
| `CONTEXT_DEBUG`      | Debug    | boolean | `false`                  | Enable context window debugging |
| `OPENROUTER_REFERER` | Provider | string  | `https://ai-agent.local` | OpenRouter HTTP-Referer header  |
| `OPENROUTER_TITLE`   | Provider | string  | `ai-agent`               | OpenRouter X-Title header       |
| `TZ`                 | System   | string  | System default           | Timezone override               |
| `USER`               | System   | string  | System user              | Current username                |
| `HOME`               | System   | string  | System home              | Home directory path             |

---

## See Also

- [Getting Started](Getting-Started) - Chapter overview
- [Installation](Getting-Started-Installation) - Installation and initial setup
- [Configuration](Configuration) - Deep dive into configuration options
- [Configuration Files](Configuration-Files) - File resolution and layering
- [Configuration Providers](Configuration-Providers) - Provider-specific configuration
- [System Prompts Variables](System-Prompts-Variables) - All prompt variables reference
```
