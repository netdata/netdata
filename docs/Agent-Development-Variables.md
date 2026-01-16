# Prompt Variables

Runtime variable substitution in `.ai` files.

---

## Syntax

Both forms are supported:

```
${VARIABLE}
{{VARIABLE}}
```

Variables are substituted after include resolution and before sending to LLM.

---

## Available Variables

### Date/Time

| Variable | Description | Example |
|----------|-------------|---------|
| `DATETIME` | RFC 3339 with timezone | `2025-08-31T02:05:07+03:00` |
| `TIMESTAMP` | Unix epoch seconds | `1733437845` |
| `DAY` | Weekday name | `Monday` |
| `TIMEZONE` | IANA timezone or `TZ` env | `Europe/Athens` |

### System Information

| Variable | Description | Example |
|----------|-------------|---------|
| `OS` | Operating system with version | `Ubuntu 24.04.1 LTS (kernel 6.8.0)` |
| `ARCH` | CPU architecture | `x64`, `arm64` |
| `KERNEL` | Kernel string | `Linux 6.8.0-41-generic` |
| `HOSTNAME` | Host name | `workstation` |

### User Information

| Variable | Description | Example |
|----------|-------------|---------|
| `USER` | Username | `costa` |
| `CD` | Current working directory | `/home/costa/project` |

### Agent Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `MAX_TURNS` | Configured maximum turns | `10` |
| `MAX_TOOLS` | Max tool calls per turn | `20` |

---

## Usage Examples

### Time-Aware Agent

```yaml
---
models:
  - openai/gpt-4o
---
You are a scheduling assistant.

Current time: ${DATETIME}
Day of week: ${DAY}
Timezone: ${TIMEZONE}

Help users manage their calendar.
```

### System-Aware Agent

```yaml
---
models:
  - openai/gpt-4o
tools:
  - filesystem
---
You are a system administration assistant.

Operating system: ${OS}
Architecture: ${ARCH}
Current directory: ${CD}
Running as: ${USER}

Help with system tasks appropriate for this environment.
```

### Self-Documenting Agent

```yaml
---
models:
  - openai/gpt-4o
maxTurns: 15
maxToolCallsPerTurn: 10
---
You are an autonomous research agent.

## Your Limits

- Maximum turns: ${MAX_TURNS}
- Maximum tools per turn: ${MAX_TOOLS}

Plan your work within these constraints.
```

---

## Environment Variables

Environment variables are NOT automatically available as prompt variables.

For environment variables, use them in the config file:

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  }
}
```

---

## Unknown Variables

Unknown variables are left unchanged in the prompt:

```
Hello ${UNKNOWN_VAR}!
```

Becomes:
```
Hello ${UNKNOWN_VAR}!
```

This prevents accidental data leakage.

---

## Substitution Order

1. Include directives (`${include:...}`) resolved first
2. Built-in variables substituted
3. Result sent to LLM

---

## Best Practices

### Do: Provide context

```yaml
---
models:
  - openai/gpt-4o
---
Current date: ${DATETIME}
You are a news summarizer. Consider recency when evaluating sources.
```

### Do: Document constraints

```yaml
---
models:
  - openai/gpt-4o
maxTurns: 10
---
You have ${MAX_TURNS} turns to complete this task. Plan accordingly.
```

### Don't: Rely on variables for security

Variables are informational. Don't use them for access control:

```yaml
# BAD - user can override
User: ${USER}
Only allow admin access for user 'admin'
```

---

## See Also

- [Include Directives](Agent-Development-Includes)
- [Environment Variables](Getting-Started-Environment-Variables)
