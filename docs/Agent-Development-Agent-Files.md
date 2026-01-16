# Agent Files (.ai Format)

The `.ai` file format is the core building block of AI Agent.

---

## File Structure

Every `.ai` file has three parts:

```yaml
#!/usr/bin/env ai-agent        # Optional shebang (makes file executable)
---
# YAML frontmatter              # Configuration section
description: Agent description
models:
  - provider/model
---
# System prompt                 # Markdown prompt body
You are a helpful assistant...
```

---

## Shebang (Optional)

Add to make the file directly executable:

```bash
#!/usr/bin/env ai-agent
```

Then run:

```bash
chmod +x my-agent.ai
./my-agent.ai "Hello"
```

---

## Frontmatter Section

YAML configuration between `---` markers. See [Frontmatter Schema](Agent-Development-Frontmatter) for all keys.

**Required:**
- `models` - At least one `provider/model` pair

**Common:**
```yaml
---
description: Short description of the agent
models:
  - openai/gpt-4o
  - anthropic/claude-3-haiku    # Fallback
tools:
  - mcp-server-name
maxTurns: 10
temperature: 0.7
---
```

---

## Prompt Section

Everything after the second `---` is the system prompt. Supports:

- **Markdown formatting**
- **Variable substitution**: `${VAR}` or `{{VAR}}`
- **Include directives**: `${include:path/to/file.md}`

Example:

```markdown
---
models:
  - openai/gpt-4o
---
You are a helpful assistant.

Current time: ${DATETIME}

${include:shared/guidelines.md}

## Your Capabilities

- Answer questions
- Provide explanations
- Help with tasks
```

---

## File Organization

Typical project structure:

```
my-project/
├── .ai-agent.json          # Configuration
├── .ai-agent.env           # Secrets (gitignored)
├── agents/
│   ├── main.ai             # Main orchestrator
│   ├── researcher.ai       # Sub-agent
│   └── writer.ai           # Sub-agent
└── shared/
    ├── guidelines.md       # Shared prompt content
    └── tone.md             # Voice/tone guidelines
```

---

## Auto-Loading Sub-Agents

When you reference agents in frontmatter:

```yaml
---
agents:
  - ./specialists/researcher.ai
  - ./specialists/writer.ai
---
```

They are automatically loaded when the parent is registered. Paths are relative to the parent file.

---

## Naming Conventions

- Use descriptive names: `web-researcher.ai`, `code-reviewer.ai`
- Group related agents in directories
- Main orchestrator often named `main.ai` or after the project

---

## Examples

### Minimal Agent

```yaml
---
models:
  - openai/gpt-4o-mini
---
You are a helpful assistant.
```

### Research Agent

```yaml
#!/usr/bin/env ai-agent
---
description: Web research specialist
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
maxTurns: 15
temperature: 0.3
cache: 1h
---
You are a thorough web researcher.

${include:shared/research-guidelines.md}
```

### Orchestrator Agent

```yaml
#!/usr/bin/env ai-agent
---
description: Task orchestrator
models:
  - openai/gpt-4o
agents:
  - ./researcher.ai
  - ./writer.ai
  - ./reviewer.ai
maxTurns: 20
---
You coordinate a team of specialists.

## Your Team

- **researcher**: Gathers information
- **writer**: Creates content
- **reviewer**: Reviews and improves

Delegate tasks appropriately.
```

---

## See Also

- [Frontmatter Schema](Agent-Development-Frontmatter)
- [Include Directives](Agent-Development-Includes)
- [AI Agent Configuration Guide](skills/ai-agent-configuration.md)
