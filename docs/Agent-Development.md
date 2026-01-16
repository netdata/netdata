# Agent Development

Learn how to build AI agents with the `.ai` file format.

---

## Pages in This Section

| Page | Description |
|------|-------------|
| [Agent Files](Agent-Development-Agent-Files) | The `.ai` file format and structure |
| [Frontmatter Schema](Agent-Development-Frontmatter) | All available frontmatter keys |
| [Include Directives](Agent-Development-Includes) | Code reuse with `${include:file}` |
| [Prompt Variables](Agent-Development-Variables) | Runtime variable substitution |
| [Input/Output Contracts](Agent-Development-Contracts) | JSON schemas for structured I/O |
| [Safety Gates](Agent-Development-Safety) | Prompt patterns for safe agents |
| [Multi-Agent Orchestration](Agent-Development-Multi-Agent) | Advisors, router, handoff, sub-agents |

---

## Complete Reference

**[AI Agent Configuration Guide](skills/ai-agent-configuration.md)** - The authoritative single-page reference for AI assistants building agents. Covers everything in depth.

---

## Quick Overview

### Agent File Structure

```yaml
#!/usr/bin/env ai-agent
---
# YAML frontmatter (configuration)
description: What this agent does
models:
  - openai/gpt-4o
tools:
  - mcp-server-name
maxTurns: 10
---
# System prompt (markdown)
You are a helpful assistant...
```

### Minimum Viable Agent

```yaml
---
models:
  - openai/gpt-4o-mini
---
You are a helpful assistant.
```

### Agent with Tools

```yaml
---
models:
  - openai/gpt-4o
tools:
  - filesystem
  - brave
maxTurns: 15
---
You are an assistant with access to the filesystem and web search.
```

### Multi-Agent with Sub-Agents

```yaml
---
models:
  - openai/gpt-4o
agents:
  - ./specialists/researcher.ai
  - ./specialists/writer.ai
maxTurns: 20
---
You are an orchestrator that delegates tasks to specialist agents.
```

---

## Key Concepts

### Models
- Specify as `provider/model` pairs
- Multiple models = fallback chain
- First model tried; on failure, next model

### Tools
- Reference MCP servers by name (from `.ai-agent.json`)
- Sub-agents automatically become tools
- Tools can be filtered with `toolsAllowed`/`toolsDenied`

### Turns
- Each LLM call + tool execution = one turn
- `maxTurns` prevents infinite loops
- Final turn forces completion

### Output
- `format: markdown | json | text`
- JSON requires `schema` definition
- Output validated against schema

---

## See Also

- [Configuration](Configuration) - Set up providers and tools
- [AI Agent Configuration Guide](skills/ai-agent-configuration.md) - Complete reference
