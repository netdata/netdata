# Prompt Variables Reference

Complete reference for all runtime variables available in system prompts.

---

## Table of Contents

- [Overview](#overview) - What variables are and how they work
- [Syntax](#syntax) - How to use variables
- [Variable Reference](#variable-reference) - All available variables
- [Usage Examples](#usage-examples) - Practical patterns
- [Substitution Order](#substitution-order) - How processing happens
- [Troubleshooting](#troubleshooting) - Common issues and fixes
- [See Also](#see-also) - Related documentation

---

## Overview

Prompt variables let you inject dynamic values into your system prompt at runtime. They're useful for:

- **Context**: Provide current time, date, timezone
- **Environment**: Include OS, architecture, working directory
- **Self-documentation**: Show the agent its own limits (max turns, max tools)
- **Output control**: Ensure correct output format via `${FORMAT}`

Variables are substituted after include resolution, right before the prompt is sent to the LLM.

---

## Syntax

Two syntaxes are supported (equivalent):

```markdown
${VARIABLE_NAME}
{{VARIABLE_NAME}}
```

Variables are case-sensitive and use `UPPER_SNAKE_CASE`.

**Example**:

```markdown
Current time: ${DATETIME}
Current time: {{DATETIME}}
```

Both produce:

```markdown
Current time: 2025-08-31T14:30:00+03:00
```

---

## Variable Reference

### Date and Time Variables

| Variable    | Description                      | Example Value               |
| ----------- | -------------------------------- | --------------------------- |
| `DATETIME`  | RFC 3339 timestamp with timezone | `2025-08-31T14:30:00+03:00` |
| `TIMESTAMP` | Unix epoch (seconds)             | `1733437845`                |
| `DAY`       | Full weekday name                | `Monday`                    |
| `TIMEZONE`  | IANA timezone identifier         | `Europe/Athens`             |

**Notes**:

- `DATETIME` includes timezone offset for unambiguous time references
- `TIMEZONE` comes from system detection or `TZ` environment variable
- All time values are computed at prompt load time

### System Information Variables

| Variable   | Description                   | Example Value                       |
| ---------- | ----------------------------- | ----------------------------------- |
| `OS`       | Operating system with version | `Ubuntu 24.04.1 LTS (kernel 6.8.0)` |
| `ARCH`     | CPU architecture              | `x64`, `arm64`                      |
| `KERNEL`   | Kernel type and version       | `Linux 6.8.0-41-generic`            |
| `HOSTNAME` | Machine hostname              | `workstation`                       |
| `USER`     | Current username              | `costa`                             |
| `CD`       | Current working directory     | `/home/costa/project`               |

**Notes**:

- These variables are **only available in CLI inline prompt mode** (`ai-agent "sys" "user"`)
- They are **not injected when running `.ai` files via headends** or the agent registry
- `OS` attempts to read `/etc/os-release` on Linux for a friendly name
- `USER` falls back to `USER` or `USERNAME` environment variables if `os.userInfo()` fails

### Agent Configuration Variables

| Variable    | Description                 | Example Value |
| ----------- | --------------------------- | ------------- |
| `MAX_TURNS` | Configured maximum turns    | `10`          |
| `MAX_TOOLS` | Maximum tool calls per turn | `10`          |

**Notes**:

- Values come from frontmatter or defaults
- Useful for self-aware agents that plan within constraints

### Output Format Variable

| Variable | Description                                        |
| -------- | -------------------------------------------------- |
| `FORMAT` | Output format instructions for the current context |

**FORMAT values by context**:

| Context          | `${FORMAT}` expands to                                                                                    |
| ---------------- | --------------------------------------------------------------------------------------------------------- |
| Terminal (TTY)   | `a TTY-compatible plain monospaced text response. Use literal "\\x1b[...m" sequences for ANSI colours...` |
| Piped output     | `Plain text without any formatting or markdown. Do not wrap long lines.`                                  |
| JSON expected    | `json`                                                                                                    |
| Slack headend    | `Slack Block Kit JSON array of messages (not raw text or GitHub markdown)`                                |
| Markdown         | `GitHub Markdown`                                                                                         |
| Markdown+Mermaid | `GiHub Markdown with Mermaid diagrams`                                                                    |
| Sub-agent        | `Internal agent-to-agent exchange format (not user-facing).`                                              |

**Important**: Always include `${FORMAT}` in your prompts to ensure consistent output across all invocation contexts.

---

## Usage Examples

### Time-Aware Agent

Include time context for scheduling, recency evaluation, or logging:

```yaml
---
models:
  - openai/gpt-4o
---
You are a scheduling assistant.

## Context
Current time: ${DATETIME}
Day of week: ${DAY}
Timezone: ${TIMEZONE}

Help users manage their calendar, considering:
- Current day and time for availability
- Timezone differences for international meetings
- Business hours (9am-6pm local time)

Respond in ${FORMAT}.
```

### System Administration Agent

Include system context for environment-appropriate commands:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - filesystem
---
You are a system administration assistant.

## System Context
Operating system: ${OS}
Architecture: ${ARCH}
Kernel: ${KERNEL}
Hostname: ${HOSTNAME}
Current directory: ${CD}
Running as user: ${USER}

Provide commands appropriate for this specific environment.
Do not assume sudo access unless the user confirms.

Respond in ${FORMAT}.
```

**Note**: `${OS}`, `${ARCH}`, `${KERNEL}`, `${HOSTNAME}`, `${CD}`, and `${USER}` variables are **only available when running via CLI inline mode** (`ai-agent "sys" "user"`). They are not available when running `.ai` files via headends.

### Self-Limiting Agent

Help the agent plan within its constraints:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - brave
  - filesystem
maxTurns: 15
maxToolCallsPerTurn: 10
---
You are an autonomous research agent.

## Your Operational Limits
- Maximum turns available: ${MAX_TURNS}
- Maximum tools per turn: ${MAX_TOOLS}

Plan your research within these constraints:
- Prioritize the most important queries
- Batch related tool calls in the same turn
- Leave 2-3 turns for synthesis and reporting

Respond in ${FORMAT}.
```

### News and Current Events Agent

Use timestamp for recency awareness:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - brave
---
You are a news analyst.

## Context
Current date and time: ${DATETIME}
Unix timestamp: ${TIMESTAMP}

When evaluating news sources:
- Prefer articles from the last 24 hours
- Note the publication date of all sources
- Flag if information might be outdated

Respond in ${FORMAT}.
```

### Multi-Format Output Agent

Rely on `${FORMAT}` for context-appropriate output:

```yaml
---
models:
  - openai/gpt-4o
---
You are a report generator.

## Output Instructions
Format your response in ${FORMAT}.

Structure all reports as:
1. Executive Summary
2. Key Findings
3. Detailed Analysis
4. Recommendations
5. Sources

Adjust formatting (markdown headers, plain text sections, etc.)
based on the required output format.
```

---

## Substitution Order

Processing happens in this order:

1. **Include resolution**: `${include:...}` directives are replaced with file contents
2. **Variable substitution**: `${VAR}` and `{{VAR}}` are replaced with values
3. **Prompt sent to LLM**: Final text is used as the system prompt

This means:

- Included files can contain variables (they'll be substituted)
- Variable names cannot be dynamic (no `${${OTHER_VAR}}`)

**Example**:

`shared/context.md`:

```markdown
## Context

Time: ${DATETIME}
User: ${USER}
```

`agent.ai`:

```yaml
---
models:
  - openai/gpt-4o
---
You are an assistant.

${include:shared/context.md}
```

**Step 1** - Include resolved:

```markdown
You are an assistant.

## Context

Time: ${DATETIME}
User: ${USER}
```

**Step 2** - Variables substituted:

```markdown
You are an assistant.

## Context

Time: 2025-08-31T14:30:00+03:00
User: costa
```

---

## Unknown Variables

Unknown variables are left unchanged in the prompt:

```markdown
Hello ${UNKNOWN_VARIABLE}!
```

Becomes:

```markdown
Hello ${UNKNOWN_VARIABLE}!
```

This behavior:

- Prevents accidental data leakage from environment
- Lets you catch typos (variable appears literally in output)
- Allows literal `${}` syntax if needed (though rare)

---

## Environment Variables

**Environment variables are NOT automatically available as prompt variables.**

The available variables are a fixed set (listed above). If you need environment values:

1. **For API keys and secrets**: Use `.ai-agent.json` configuration:

   ```json
   {
     "providers": {
       "openai": {
         "apiKey": "${OPENAI_API_KEY}"
       }
     }
   }
   ```

2. **For custom values**: There's currently no mechanism to inject arbitrary environment variables into prompts. This is intentional for security.

---

## Troubleshooting

### Variable not substituted

**Symptom**: `${DATETIME}` appears literally in the LLM's context.

**Causes and fixes**:

1. **Typo in variable name**: Check spelling and case

   ```markdown
   ${DATETIME}     # Correct
   ${datetime} # Wrong - lowercase
   ${DATE_TIME} # Wrong - extra underscore
   ```

2. **Syntax error**: Check for spaces or typos

   ```markdown
   ${DATETIME}     # Correct
   ${ DATETIME} # Wrong - space after {
   ${DATETIME } # Wrong - space before }
   ```

3. **Variable doesn't exist**: Only the documented variables are available

### Getting current values for debugging

Use `--verbose` and `--trace-llm` flags to see variable expansion in action:

```bash
ai-agent --agent my-agent.ai --verbose --trace-llm "test"
```

This shows verbose logs including the expanded system prompt sent to the LLM.

### FORMAT not working as expected

**Symptom**: Output format doesn't match expectations.

**Causes and fixes**:

1. **Missing ${FORMAT}**: Add it to your prompt

   ```markdown
   Respond in ${FORMAT}.
   ```

2. **Conflicting instructions**: Don't override FORMAT with specific format instructions

   ```markdown
   # Bad - conflicts with FORMAT

   Respond in ${FORMAT}.
   Always use JSON format.

   # Good - relies on FORMAT

   Respond in ${FORMAT}.
   ```

3. **Format determined by context**: FORMAT is set based on:
   - TTY vs piped output
   - `--format` CLI flag
   - Headend configuration
   - Output schema (JSON expected)

### Time showing wrong timezone

**Symptom**: `${DATETIME}` shows wrong timezone.

**Fix**: Set the `TZ` environment variable:

```bash
TZ=America/New_York ai-agent --agent my-agent.ai "What time is it?"
```

Or ensure your system timezone is configured correctly.

---

## Best Practices

### 1. Always Include FORMAT

Every prompt should specify output format:

```markdown
Respond in ${FORMAT}.
```

### 2. Use Context Variables When Relevant

If the task benefits from time/environment awareness, include them:

```markdown
Current time: ${DATETIME}
```

**Note**: Time variables (`${DATETIME}`, `${TIMESTAMP}`, `${DAY}`, `${TIMEZONE}`) are available in all contexts. System variables (`${OS}`, `${ARCH}`, `${KERNEL}`, `${HOSTNAME}`, `${CD}`, `${USER}`) are **only available in CLI inline mode**.

### 3. Help Agents Self-Limit

For complex agents, expose their limits:

```markdown
You have ${MAX_TURNS} turns and ${MAX_TOOLS} tools per turn.
Plan accordingly.
```

### 4. Don't Rely on Variables for Security

Variables are informational. Don't use them for access control:

```markdown
# Bad - user could be spoofed

User: ${USER}
Only allow admin if user is 'admin'
```

### 5. Test with Different Contexts

Test your agent in different contexts (TTY, piped, JSON) to verify FORMAT handling works correctly.

---

## See Also

- [System-Prompts](System-Prompts) - Chapter overview
- [System-Prompts-Writing](System-Prompts-Writing) - Best practices for prompts
- [System-Prompts-Includes](System-Prompts-Includes) - Include directive reference
- [Agent-Development-Frontmatter](Agent-Development-Frontmatter) - Configuration options
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents and format options
