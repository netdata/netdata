# Configuration Loading

How AI Agent finds and merges configuration files.

---

## Resolution Order

Configuration is searched in order (first found wins):

1. `--config <path>` CLI option
2. `.ai-agent.json` in current directory
3. `.ai-agent.json` in agent file's directory (for frontmatter agents)
4. `~/.ai-agent.json` in home directory
5. `/etc/ai-agent/.ai-agent.json` system-wide

If no configuration is found, the program fails with an error.

---

## Environment File

Environment variables are loaded from `.ai-agent.env` sidecar file:

1. `.ai-agent.env` in same directory as config
2. `.ai-agent.env` in current directory

Format:
```bash
# Comments supported
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
BRAVE_API_KEY=...
```

---

## Variable Expansion

All string values support `${VAR}` expansion:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### Default Values

Use `:-` for defaults:

```json
{
  "baseUrl": "${OPENAI_BASE_URL:-https://api.openai.com/v1}"
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

---

## Configuration Layers

Settings are merged from multiple sources:

### Priority (highest to lowest)

1. **CLI options**: `--temperature 0.2`
2. **Per-agent frontmatter**: `temperature: 0.3`
3. **Config file defaults**: `defaults.temperature: 0.7`
4. **Built-in defaults**: hardcoded fallbacks

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

Result: `temperature = 0.2` (CLI wins)

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

### Merge Strategy

Default merge is `overlay` (model settings extend provider settings).

Options:
- `overlay` (default): Model settings add to/override provider settings
- `replace`: Model settings completely replace provider settings

---

## Validating Configuration

### Dry Run

```bash
ai-agent --agent test.ai --dry-run
```

Validates config without calling LLM.

### Verbose Mode

```bash
ai-agent --agent test.ai --verbose
```

Shows configuration resolution in logs.

---

## Multiple Config Files

For different environments:

```
project/
├── .ai-agent.json          # Development
├── .ai-agent.prod.json     # Production
└── .ai-agent.test.json     # Testing
```

Use:
```bash
ai-agent --config .ai-agent.prod.json --agent myagent.ai
```

---

## Security Considerations

### Do NOT commit secrets

Add to `.gitignore`:
```
.ai-agent.env
```

### Use environment variables

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

### Restrict file permissions

```bash
chmod 600 .ai-agent.env
chmod 644 .ai-agent.json
```

---

## Troubleshooting

### Config not found

```
Error: Configuration file not found
```

Check:
1. File exists in expected location
2. File is valid JSON
3. File permissions allow reading

### Variable not expanded

```
Error: API key invalid: ${OPENAI_API_KEY}
```

Check:
1. Environment variable is set
2. `.ai-agent.env` is in correct location
3. Variable name matches exactly

---

## See Also

- [Configuration](Configuration) - Overview
- [docs/specs/configuration-loading.md](../docs/specs/configuration-loading.md) - Technical spec
