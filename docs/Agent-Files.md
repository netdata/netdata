# Agent Files

Agent files (`.ai` files) are the building blocks of ai-agent. Each file defines a complete agent with its configuration and instructions.

---

## Table of Contents

- [What is an Agent File?](#what-is-an-agent-file) - Structure and purpose of .ai files
- [File Structure](#file-structure) - Frontmatter and prompt body
- [Quick Start](#quick-start) - Create your first agent file
- [Configuration Categories](#configuration-categories) - Overview of all configuration options
- [File Locations](#file-locations) - Where to put agent files
- [See Also](#see-also) - Related documentation

---

## What is an Agent File?

An agent file is a plain text file with a `.ai` extension that contains:

1. **Frontmatter** (YAML) - Configuration at the top, between `---` markers
2. **System Prompt** - Instructions for the agent after the frontmatter

When you run `ai-agent --agent my-agent.ai`, the system:

1. Parses the frontmatter for configuration
2. Extracts the system prompt
3. Initializes the agent with the specified model, tools, and behavior

---

## File Structure

Every agent file follows this structure:

```yaml
---
# FRONTMATTER: YAML configuration block
description: What this agent does
models:
  - openai/gpt-4o
tools:
  - filesystem
maxTurns: 10
---
# SYSTEM PROMPT: Instructions for the agent
You are a helpful assistant.

Your task is to help the user with their questions.

Respond in ${FORMAT}.
```

### Frontmatter Rules

- Must start on line 1 (or line 2 if a shebang `#!` is present)
- Opens with `---` on its own line
- Contains YAML configuration
- Closes with `---` on its own line
- Everything after the closing `---` is the system prompt

### System Prompt

- Plain text instructions for the agent
- Supports [variable substitution](System-Prompts-Variables) (e.g., `${FORMAT}`)
- Supports [@include directives](System-Prompts-Includes) for reusable content
- Can be as simple or complex as needed

---

## Quick Start

### Minimal Agent

The simplest working agent:

```yaml
---
description: A helpful assistant
models:
  - openai/gpt-4o
---
You are a helpful assistant. Answer questions clearly and concisely.

Respond in ${FORMAT}.
```

Save as `assistant.ai` and run:

```bash
ai-agent --agent assistant.ai "What is the capital of France?"
```

### Agent with Tools

An agent that can search the web:

```yaml
---
description: Research assistant with web search
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
  - fetcher
maxTurns: 15
temperature: 0.3
---
You are a research assistant with access to web search.

1. Search for relevant information
2. Verify facts from multiple sources
3. Provide a clear summary

Respond in ${FORMAT}.
```

### Agent with Structured Output

An agent that returns JSON:

```yaml
---
description: Company analyzer
models:
  - openai/gpt-4o
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      industry:
        type: string
      summary:
        type: string
    required:
      - name
      - industry
      - summary
---
Analyze the provided company and return structured data.
```

---

## Configuration Categories

Agent configuration is organized into these categories:

| Category          | Keys                                                                                                                                                     | Purpose                             | Documentation                                          |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- | ------------------------------------------------------ |
| **Identity**      | `description`, `usage`, `toolName`                                                                                                                       | Name and describe your agent        | [Agent-Files-Identity](Agent-Files-Identity)           |
| **Models**        | `models`, `reasoning`, `reasoningTokens`, `caching`                                                                                                      | Select LLMs and configure reasoning | [Agent-Files-Models](Agent-Files-Models)               |
| **Tools**         | `tools`, `toolResponseMaxBytes`, `toolOutput`                                                                                                            | Give your agent capabilities        | [Agent-Files-Tools](Agent-Files-Tools)                 |
| **Sub-Agents**    | `agents`                                                                                                                                                 | Delegate to other agents            | [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents)       |
| **Orchestration** | `advisors`, `router`, `handoff`                                                                                                                          | Multi-agent patterns                | [Agent-Files-Orchestration](Agent-Files-Orchestration) |
| **Behavior**      | `maxTurns`, `maxRetries`, `maxToolCallsPerTurn`, `temperature`, `topP`, `topK`, `llmTimeout`, `toolTimeout`, `maxOutputTokens`, `repeatPenalty`, `cache` | Limits and sampling                 | [Agent-Files-Behavior](Agent-Files-Behavior)           |
| **Contracts**     | `input`, `output`                                                                                                                                        | Structured I/O schemas              | [Agent-Files-Contracts](Agent-Files-Contracts)         |

---

## File Locations

### Finding Agent Files

Agent files can be anywhere on the filesystem. Reference them with:

```bash
# Relative path
ai-agent --agent ./agents/my-agent.ai

# Absolute path
ai-agent --agent /path/to/my-agent.ai
```

### Recommended Organization

```
project/
├── .ai-agent.json          # Configuration (providers, MCP servers)
├── agents/
│   ├── main.ai             # Primary agent
│   ├── helpers/
│   │   ├── researcher.ai   # Sub-agent
│   │   └── writer.ai       # Sub-agent
│   └── schemas/
│       └── report.json     # Shared JSON schemas
└── prompts/
    └── shared.md           # Shared prompt content (@include)
```

### Path Resolution

- **Relative paths** in frontmatter (`agents:`, `schemaRef:`, `@include`) resolve relative to the agent file's directory
- **Absolute paths** are used as-is

---

## Common Patterns

### Question-Answering Agent

```yaml
---
description: General Q&A assistant
models:
  - openai/gpt-4o
maxTurns: 5
temperature: 0.3
---
You are a helpful assistant. Answer questions clearly and concisely.
```

### Tool-Using Research Agent

```yaml
---
description: Research assistant with web access
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
  - fetcher
maxTurns: 15
maxToolCallsPerTurn: 10
cache: 1h
---
You are a research assistant. Search for information and synthesize findings.
```

### Multi-Agent Coordinator

```yaml
---
description: Research coordinator
models:
  - anthropic/claude-sonnet-4-20250514
agents:
  - ./helpers/researcher.ai
  - ./helpers/analyst.ai
  - ./helpers/writer.ai
maxTurns: 20
---
You coordinate research tasks. Delegate to specialists as needed.
```

---

## Troubleshooting

### "Unsupported frontmatter key(s)"

**Problem**: Unknown key in frontmatter.

**Solution**: Check spelling. Keys are case-sensitive. See this documentation for valid keys.

### "Invalid frontmatter key(s): stream, verbose"

**Problem**: Using runtime-only flags in frontmatter.

**Solution**: Remove these keys. Use CLI flags instead:

```bash
ai-agent --stream --verbose --agent my-agent.ai
```

### "Must have exactly one '/' in provider/model"

**Problem**: Model specified without provider prefix.

**Solution**: Use `provider/model` format:

```yaml
# Wrong
models: gpt-4o

# Correct
models: openai/gpt-4o
```

---

## See Also

- [Agent-Files-Identity](Agent-Files-Identity) - Configure agent name and description
- [Agent-Files-Models](Agent-Files-Models) - Model selection and fallbacks
- [Agent-Files-Tools](Agent-Files-Tools) - Tool configuration
- [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents) - Sub-agent delegation
- [Agent-Files-Orchestration](Agent-Files-Orchestration) - Multi-agent patterns
- [Agent-Files-Behavior](Agent-Files-Behavior) - Limits and sampling parameters
- [Agent-Files-Contracts](Agent-Files-Contracts) - Structured input/output
- [System-Prompts](System-Prompts) - Writing effective prompts
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents from command line
- [Configuration](Configuration) - Global configuration (`.ai-agent.json`)
