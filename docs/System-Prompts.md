# System Prompts

System prompts define how your agent behaves, responds, and interacts. This is where you shape your agent's personality, capabilities, and constraints.

---

## Table of Contents

- [What is a System Prompt?](#what-is-a-system-prompt) - The role of prompts in agent behavior
- [Prompt Anatomy](#prompt-anatomy) - Structure and key components
- [Quick Example](#quick-example) - A complete working prompt
- [Key Features](#key-features) - Variables, includes, and formatting
- [Chapter Pages](#chapter-pages) - Detailed documentation for each topic
- [See Also](#see-also) - Related documentation

---

## What is a System Prompt?

The system prompt is the instruction set sent to the LLM at the start of every conversation. It defines:

- **Who the agent is** - Role, personality, expertise
- **What it should do** - Tasks, workflows, behaviors
- **How it should respond** - Tone, format, structure
- **What it must avoid** - Constraints, safety rules, boundaries

The system prompt is everything after the frontmatter's closing `---` marker in a `.ai` file.

---

## Prompt Anatomy

A well-structured system prompt typically contains these sections:

```markdown
---
models:
  - openai/gpt-4o
maxTurns: 10
---
# 1. Role Definition
You are a [role] that [primary function].

# 2. Context
Current time: ${DATETIME}
User: ${USER}

# 3. Capabilities
You can:
- Do this
- Do that

# 4. Constraints
You must NOT:
- Do dangerous thing
- Share sensitive info

# 5. Output Format
Respond in ${FORMAT}.

# 6. Workflow (for complex agents)
Follow these steps:
1. First...
2. Then...
3. Finally...

# 7. Shared Content (optional)
${include:shared/guidelines.md}
```

**Key insight**: The prompt is regular Markdown text. Use headings, lists, and formatting to make it clear and scannable for the LLM.

---

## Quick Example

A simple research assistant:

```yaml
---
description: Research assistant with web search
models:
  - openai/gpt-4o
tools:
  - brave
maxTurns: 10
---
You are a research assistant that helps users find accurate information.

## Context
Current time: ${DATETIME}
Timezone: ${TIMEZONE}

## Your Approach
1. Search for relevant information using available tools
2. Verify facts from multiple sources when possible
3. Cite your sources clearly
4. Acknowledge uncertainty when appropriate

## Output Format
Respond in ${FORMAT}.

## Guidelines
- Be concise but thorough
- Prioritize accuracy over speed
- If unsure, say so rather than guessing
```

---

## Key Features

### Variable Substitution

Insert dynamic values into your prompt:

```markdown
Current time: ${DATETIME}
Running on: ${OS}
User: ${USER}
```

**Available variables**: `DATETIME`, `TIMESTAMP`, `DAY`, `TIMEZONE`, `OS`, `ARCH`, `KERNEL`, `HOSTNAME`, `USER`, `CD`, `MAX_TURNS`, `MAX_TOOLS`, `FORMAT`

See [System-Prompts-Variables](System-Prompts-Variables) for the complete reference.

### Include Directives

Reuse content across multiple agents:

```markdown
${include:shared/tone.md}
${include:shared/safety-rules.md}
```

Includes are resolved before variable substitution. Nested includes are supported.

See [System-Prompts-Includes](System-Prompts-Includes) for syntax and patterns.

### Output Format

The `${FORMAT}` variable contains output format instructions based on how the agent is invoked:

| Context | FORMAT value |
|---------|--------------|
| Terminal (TTY) | TTY-compatible plain text with ANSI colors |
| Piped output | Plain text without formatting |
| JSON output expected | `json` |
| Slack headend | Slack Block Kit instructions |

Always include `${FORMAT}` in your prompt to ensure consistent output across contexts.

---

## Chapter Pages

This chapter contains four pages:

| Page | Description |
|------|-------------|
| **[System-Prompts](System-Prompts)** | This page - overview and anatomy |
| **[System-Prompts-Writing](System-Prompts-Writing)** | Best practices, structure, patterns, dos and don'ts |
| **[System-Prompts-Includes](System-Prompts-Includes)** | `@include` syntax, resolution, nesting |
| **[System-Prompts-Variables](System-Prompts-Variables)** | All variables, when available, examples |

---

## Common Questions

### Where does the system prompt go?

After the frontmatter section. Everything after the closing `---` is the system prompt.

### How long should my prompt be?

As long as needed, but no longer. Most effective prompts are 100-500 words. Very long prompts (1000+ words) can dilute the model's attention.

### Should I use Markdown formatting?

Yes. Headings, lists, and bold text help the model understand structure. Avoid complex formatting like tables in the prompt itself.

### What's the difference between system prompt and user prompt?

- **System prompt**: Set once when the agent is created. Defines behavior.
- **User prompt**: Provided at runtime. Contains the actual task/question.

---

## See Also

- [Agent-Development-Agent-Files](Agent-Development-Agent-Files) - `.ai` file structure
- [Agent-Development-Frontmatter](Agent-Development-Frontmatter) - Configuration options
- [Agent-Development-Multi-Agent](Agent-Development-Multi-Agent) - Multi-agent workflows
