# Prompt Variables Reference

Reference for runtime values delivered via XML-NEXT, plus CLI-only `${VAR}` expansion.

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

Runtime values are provided via XML-NEXT each turn. Direct CLI prompts (`--system` / `--user`) still support `${VAR}` expansion.
Use these values for:

- **Context**: Provide current time, date, timezone
- **Environment**: Include OS, architecture, working directory
- **Self-documentation**: Show the agent its own limits (max turns, max tools)
- **Output control**: Ensure correct output format via `XML-NEXT.FORMAT`

**Important**:
- `.ai` system prompts are static per agent load (no `${VAR}` expansion).
- XML-NEXT carries per-session/per-turn values.
- CLI prompt files still support `${VAR}` expansion.

---

## Syntax

### XML-NEXT (for `.ai` prompts)

Reference runtime values as **static markers** in your prompt. The values arrive in the XML-NEXT block each turn:

```markdown
Current time: XML-NEXT.DATETIME
Output format: XML-NEXT.FORMAT
```

### CLI prompt expansion (only for `--system` / `--user`)

Two syntaxes are supported (equivalent) for CLI prompt files:

```markdown
${VARIABLE_NAME}
{{VARIABLE_NAME}}
```

Variables are case-sensitive and use `UPPER_SNAKE_CASE`.

**Example (system.md)**:

```markdown
Current time: ${DATETIME}
```

Run:

```
ai-agent --system system.md "hello"
```

---

## Variable Reference (XML-NEXT)

### Date and Time Variables (XML-NEXT)

| Variable    | Description                      | Example Value               |
| ----------- | -------------------------------- | --------------------------- |
| `DATETIME`  | RFC 3339 timestamp with timezone | `2025-08-31T14:30:00+03:00` |
| `TIMESTAMP` | Unix epoch (seconds)             | `1733437845`                |
| `DAY`       | Full weekday name                | `Monday`                    |
| `TIMEZONE`  | IANA timezone identifier         | `Europe/Athens`             |

**Notes**:

- `DATETIME` includes timezone offset for unambiguous time references
- `TIMEZONE` comes from system detection or `TZ` environment variable

### System Information Variables (CLI-only)

| Variable   | Description                   | Example Value                       |
| ---------- | ----------------------------- | ----------------------------------- |
| `OS`       | Operating system with version | `Ubuntu 24.04.1 LTS (kernel 6.8.0)` |
| `ARCH`     | CPU architecture              | `x64`, `arm64`                      |
| `KERNEL`   | Kernel type and version       | `Linux 6.8.0-41-generic`            |
| `HOSTNAME` | Machine hostname              | `workstation`                       |
| `USER`     | Current username              | `costa`                             |
| `CD`       | Current working directory     | `/home/costa/project`               |

**Notes**:

- These variables expand **only** for CLI prompt files.
- They do **not** apply to `.ai` system prompts.

### Agent Configuration Variables (XML-NEXT)

| Variable    | Description                 | Example Value |
| ----------- | --------------------------- | ------------- |
| `MAX_TURNS` | Configured maximum turns    | `10`          |
| `MAX_TOOLS` | Maximum tool calls per turn | `10`          |

**Notes**:

- Values come from frontmatter or defaults
- Useful for self-aware agents that plan within constraints

### Output Format Variable (XML-NEXT)

| Variable | Description                                        |
| -------- | -------------------------------------------------- |
| `FORMAT` | Output format instructions for the current context |

**FORMAT values by context**:

| Context          | XML-NEXT.FORMAT value                                                                                                                                                              |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terminal (TTY)   | `a TTY-compatible plain monospaced text response. Use literal "\\x1b[...m" sequences for ANSI colours and avoid decorative boxes. Do not output markdown. Do not wrap long lines.` |
| Piped output     | `Plain text without any formatting or markdown. Do not wrap long lines.`                                                                                                           |
| JSON expected    | `json`                                                                                                                                                                             |
| Slack headend    | `Slack Block Kit JSON array of messages (not raw text or GitHub markdown)`                                                                                                         |
| Markdown         | `GitHub Markdown`                                                                                                                                                                  |
| Markdown+Mermaid | `GiHub Markdown with Mermaid diagrams`                                                                                                                                             |
| Sub-agent        | `Internal agent-to-agent exchange format (not user-facing).`                                                                                                                       |

**Important**: Always include `XML-NEXT.FORMAT` in your prompts to ensure consistent output across all invocation contexts.

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
Current time: XML-NEXT.DATETIME
Day of week: XML-NEXT.DAY
Timezone: XML-NEXT.TIMEZONE

Help users manage their calendar, considering:
- Current day and time for availability
- Timezone differences for international meetings
- Business hours (9am-6pm local time)

Respond in XML-NEXT.FORMAT.
```

### System Administration Prompt (CLI)

Use CLI-only variables for environment-specific guidance:

```markdown
## System Context
Operating system: ${OS}
Architecture: ${ARCH}
Kernel: ${KERNEL}
Hostname: ${HOSTNAME}
Current directory: ${CD}
Running as user: ${USER}
```

Run:

```
ai-agent --system system.md "help me with disk cleanup"
```

**Note**: `${OS}`, `${ARCH}`, `${KERNEL}`, `${HOSTNAME}`, `${CD}`, and `${USER}` are **only available when running from CLI** (not via headends). They are not available in `.ai` prompts.

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
- Maximum turns available: XML-NEXT.MAX_TURNS
- Maximum tools per turn: XML-NEXT.MAX_TOOLS

Plan your research within these constraints:
- Prioritize the most important queries
- Batch related tool calls in the same turn
- Leave 2-3 turns for synthesis and reporting

Respond in XML-NEXT.FORMAT.
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
Current date and time: XML-NEXT.DATETIME
Unix timestamp: XML-NEXT.TIMESTAMP

When evaluating news sources:
- Prefer articles from the last 24 hours
- Note the publication date of all sources
- Flag if information might be outdated

Respond in XML-NEXT.FORMAT.
```

### Multi-Format Output Agent

Rely on `XML-NEXT.FORMAT` for context-appropriate output:

```yaml
---
models:
  - openai/gpt-4o
---
You are a report generator.

## Output Instructions
Format your response in XML-NEXT.FORMAT.

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

Processing differs by prompt type:

1. **`.ai` prompts (Liquid templates)**:
   - Includes are resolved at load-time.
   - Liquid variables must be defined; unknown variables fail agent load.
   - XML-NEXT markers are **static** and are not substituted.
2. **CLI prompt files** (`--system`, `--user`):
   - `${VAR}` / `{{VAR}}` placeholders are expanded at runtime.
   - Unknown placeholders remain unchanged.

---

## Unknown Variables

Behavior depends on the prompt type:

- **`.ai` prompts (Liquid templates)**: Unknown variables **fail agent load**.
- **CLI prompt files**: Unknown placeholders remain unchanged (literal `${UNKNOWN}`).

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

**Symptom**: A CLI placeholder remains literal (e.g., `${DATETIME}`).

**Causes and fixes**:

1. **Not using a CLI prompt file**: `${VAR}` only expands in `--system` / `--user` files.
2. **Typo in variable name**: Check spelling and case (must be `UPPER_SNAKE_CASE`).
3. **Unsupported variable**: Only the documented CLI variables exist.

### Getting current values for debugging

Use `--verbose` and `--trace-llm` to inspect the actual XML-NEXT block and CLI expansions:

```bash
ai-agent --agent my-agent.ai --verbose --trace-llm "test"
```

### FORMAT not working as expected

**Symptom**: Output format doesn't match expectations.

**Causes and fixes**:

1. **Missing XML-NEXT.FORMAT**: Add it to your prompt

   ```markdown
   Respond in XML-NEXT.FORMAT.
   ```

2. **Conflicting instructions**: Don't override FORMAT with specific format instructions

   ```markdown
   # Bad - conflicts with FORMAT

   Respond in XML-NEXT.FORMAT.
   Always use JSON format.

   # Good - relies on FORMAT

   Respond in XML-NEXT.FORMAT.
   ```

3. **Format determined by context**: FORMAT is set based on:
   - TTY vs piped output
   - `--format` CLI flag
   - Headend configuration
   - Output schema (JSON expected)

### Time showing wrong timezone

**Symptom**: `XML-NEXT.DATETIME` shows wrong timezone.

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
Respond in XML-NEXT.FORMAT.
```

### 2. Use Context Variables When Relevant

If the task benefits from time/environment awareness, include them:

```markdown
Current time: XML-NEXT.DATETIME
```

**Note**: Time variables (`XML-NEXT.DATETIME`, `XML-NEXT.TIMESTAMP`, `XML-NEXT.DAY`, `XML-NEXT.TIMEZONE`) are available in all contexts. System variables (`${OS}`, `${ARCH}`, `${KERNEL}`, `${HOSTNAME}`, `${CD}`, `${USER}`) are **only available when running from CLI** (not via headends).

### 3. Help Agents Self-Limit

For complex agents, expose their limits:

```markdown
You have XML-NEXT.MAX_TURNS turns and XML-NEXT.MAX_TOOLS tools per turn.
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
