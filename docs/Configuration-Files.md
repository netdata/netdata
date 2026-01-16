# Configuration Files

How AI Agent finds, loads, and merges configuration files.

---

## Table of Contents

- [Overview](#overview) - Configuration file concepts
- [Resolution Order](#resolution-order) - Where config files are searched
- [Environment Files](#environment-files) - Sidecar .env file loading
- [Variable Expansion](#variable-expansion) - Placeholder substitution
- [Configuration Layers](#configuration-layers) - Merging multiple sources
- [Provider Configuration Merging](#provider-configuration-merging) - Per-model overrides
- [Validating Configuration](#validating-configuration) - Debug and verify
- [Multiple Config Files](#multiple-config-files) - Environment-specific configs
- [Security Considerations](#security-considerations) - Protecting secrets
- [Troubleshooting](#troubleshooting) - Common issues and solutions
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent uses a layered configuration system:

- **Multiple config files** can exist at different locations
- Files are **discovered in priority order** (first found wins for most settings)
- **Environment variables** are expanded using layer-specific `.env` files
- **Validation** ensures all required values are present before execution

---

## Resolution Order

Configuration files are searched in this order:

| Priority | Location                      | Description                              |
| -------- | ----------------------------- | ---------------------------------------- |
| 1        | `--config <path>`             | Explicit CLI option                      |
| 2        | `./.ai-agent.json`            | Current working directory                |
| 3        | `<agent-dir>/.ai-agent.json`  | Agent file's directory                   |
| 4        | `<binary-dir>/.ai-agent.json` | Directory containing ai-agent executable |
| 5        | `~/.ai-agent/ai-agent.json`   | Home directory                           |
| 6        | `/etc/ai-agent/ai-agent.json` | System-wide                              |

### Resolution Behavior

- **First found wins** for most settings
- **On-demand resolution**: Only providers/tools actually used are resolved
- **Missing files**: Silently skipped (all layers are optional)
- **No config found**: All layers may be missing; built-in defaults apply

### Example

```bash
# Uses config from current directory
ai-agent --agent ./myagent.ai "hello"

# Explicit config path
ai-agent --config ./production.json --agent ./myagent.ai "hello"

# Uses config from agent's directory (if different from cwd)
ai-agent --agent /path/to/agents/myagent.ai "hello"
```

---

## Environment Files

Environment variables are loaded from `.ai-agent.env` sidecar files.

### Resolution

Each config layer has its own `.ai-agent.env` file in the same directory as that layer's `.ai-agent.json`. Environment variables are resolved from the layer's paired `.env` file first, then from `process.env`.

### Format

```bash
# Comments are supported
OPENAI_API_KEY=sk-proj-...
ANTHROPIC_API_KEY=sk-ant-...
BRAVE_API_KEY=BSA...

# export prefix is allowed
export GITHUB_TOKEN=ghp_...

# Quoted values are supported
DATABASE_URL="postgres://user:pass@host/db"
```

### Scoping

Each config layer has its own environment scope:

- Variables from a layer's `.env` file are used **only** for that layer's placeholder expansion
- Process environment (`process.env`) is checked after layer-specific variables
- This prevents secrets from leaking across unrelated projects

---

## Variable Expansion

All string values in configuration support `${VAR}` expansion.

### Basic Substitution

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### Home Directory

```json
{
  "cache": {
    "sqlite": {
      "path": "${HOME}/.ai-agent/cache.db"
    }
  }
}
```

### Special Handling

Environment variable expansion works differently for certain fields:

| Field                      | Special Behavior                                                  |
| -------------------------- | ----------------------------------------------------------------- |
| `restTools.*.bodyTemplate` | `${parameters.foo}` tokens are preserved for runtime substitution |
| `restTools.*.url`          | `${parameters.foo}` tokens are preserved for runtime substitution |
| `mcpServers.*.env`         | Environment variables are expanded at config load time            |
| `mcpServers.*.headers`     | Environment variables are expanded at config load time            |

### MCP_ROOT Handling

When `${MCP_ROOT}` resolves to blank, AI Agent falls back to `process.cwd()` and logs a verbose notice. This ensures stdio servers always have a working directory.

---

## Configuration Layers

Settings are merged from multiple sources with clear priority.

### Priority (highest to lowest)

| Priority | Source                | Example                          |
| -------- | --------------------- | -------------------------------- |
| 1        | CLI options           | `--temperature 0.2`              |
| 2        | Per-agent frontmatter | `temperature: 0.3` in `.ai` file |
| 3        | Config file defaults  | `defaults.temperature: 0.7`      |
| 4        | Built-in defaults     | Hardcoded fallbacks              |

### Merge Behavior

For most settings, **first defined wins** (higher priority overrides lower).

### Example

**Config file** (`~/.ai-agent/ai-agent.json`):

```json
{
  "defaults": {
    "temperature": 0.7,
    "maxTurns": 10
  }
}
```

**Agent frontmatter** (`myagent.ai`):

```yaml
---
temperature: 0.3
---
```

**CLI**:

```bash
ai-agent --agent myagent.ai --temperature 0.2
```

**Result**: `temperature = 0.2` (CLI wins), `maxTurns = 10` (config default)

---

## Provider Configuration Merging

Provider settings can be overridden per-model:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "contextWindow": 128000,
      "models": {
        "gpt-4o-mini": {
          "contextWindow": 128000
        },
        "gpt-4o": {
          "contextWindow": 128000,
          "overrides": {
            "temperature": 0.5
          }
        }
      }
    }
  }
}
```

### Resolution Order

For model-specific settings:

1. `models.<model>.overrides.<setting>` (model-specific)
2. `<setting>` at provider level
3. Built-in default

---

## Validating Configuration

### Dry Run

Validate configuration without calling the LLM:

```bash
ai-agent --agent test.ai --dry-run
```

**Shows**:

- Configuration resolution
- Provider validation
- MCP server discovery
- Tool availability

### Verbose Mode

See detailed configuration resolution:

```bash
ai-agent --agent test.ai --verbose
```

**Shows**:

- Which config files were loaded
- Environment variable sources
- Placeholder expansion results
- Queue bindings

### Validation Checks

AI Agent validates:

| Check                   | Error on Failure                                                     |
| ----------------------- | -------------------------------------------------------------------- |
| JSON syntax             | `Invalid JSON in config file`                                        |
| Unresolved placeholders | `Unresolved variable ${VAR} for provider 'X' at Y`                   |
| Unknown queues          | `X references unknown queue 'Y'. Add it under configuration.queues.` |

---

## Multiple Config Files

Use different configs for different environments:

```
project/
├── .ai-agent.json          # Development
├── .ai-agent.prod.json     # Production
├── .ai-agent.test.json     # Testing
├── .ai-agent.env           # Development secrets
├── .ai-agent.prod.env      # Production secrets
└── agents/
    └── myagent.ai
```

### Usage

```bash
# Development (uses .ai-agent.json)
ai-agent --agent agents/myagent.ai "query"

# Production
ai-agent --config .ai-agent.prod.json --agent agents/myagent.ai "query"

# Testing
ai-agent --config .ai-agent.test.json --agent agents/myagent.ai "query"
```

---

## Security Considerations

### Do NOT Commit Secrets

Add to `.gitignore`:

```gitignore
.ai-agent.env
.ai-agent.*.env
```

### Use Environment Variables

Store secrets in environment, not config files:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### Restrict File Permissions

```bash
# Secrets file - owner read/write only
chmod 600 .ai-agent.env

# Config file - owner read/write, group/others read
chmod 644 .ai-agent.json
```

### CI/CD Secrets

In CI/CD pipelines, inject secrets as environment variables:

```yaml
# GitHub Actions example
env:
  OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

---

## Troubleshooting

### Config not found

```
Error: Configuration file not found
```

**Causes**:

- No `.ai-agent.json` in any expected location
- File permissions prevent reading
- Typo in filename

**Solutions**:

1. Create `.ai-agent.json` in current directory
2. Use `--config <path>` to specify exact location
3. Check file permissions: `ls -la .ai-agent.json`

### Variable not expanded

```
Error: API key invalid: ${OPENAI_API_KEY}
```

**Causes**:

- Environment variable not set
- `.ai-agent.env` not in correct location
- Typo in variable name

**Solutions**:

1. Set variable: `export OPENAI_API_KEY=sk-...`
2. Check `.ai-agent.env` location (same directory as config)
3. Verify variable name matches exactly (case-sensitive)

### MissingVariableError

```
Unresolved variable ${OPENAI_API_KEY} for provider 'openai' at home. Define it in .ai-agent.env or environment.
```

**Meaning**: Variable `OPENAI_API_KEY` is missing in the config from `~/.ai-agent/ai-agent.json`.

**Solutions**:

1. Add to `~/.ai-agent/ai-agent.env`: `OPENAI_API_KEY=sk-...`
2. Export in shell: `export OPENAI_API_KEY=sk-...`

### Unknown queue error

```
mcp:myserver references unknown queue 'playwright'. Add it under configuration.queues.
```

**Causes**:

- MCP server or REST tool references undefined queue
- Typo in queue name

**Solutions**:

1. Define the queue in config:

```json
{
  "queues": {
    "playwright": { "concurrent": 4 }
  }
}
```

2. Change tool to use `"queue": "default"`

### Provider missing type warning

```
provider 'custom' at /path/to/.ai-agent.json missing "type"; defaulting to 'openai'. Update configuration to include "type" explicitly.
```

**Cause**: Legacy config omitted the `type` field.

**Solution**: Add explicit type:

```json
{
  "providers": {
    "custom": {
      "type": "openai-compatible",
      "apiKey": "..."
    }
  }
}
```

---

## See Also

- [Configuration.md](Configuration.md) - Configuration overview
- [Configuration-Providers.md](Configuration-Providers.md) - Provider setup
- [Configuration-Parameters.md](Configuration-Parameters.md) - All configuration keys
- [specs/configuration-loading.md](specs/configuration-loading.md) - Technical specification
