# Agent Development

This section covers everything you need to build and configure ai-agents. Start with the core chapters, then explore specialized topics.

---

## Table of Contents

- [Agent Files](#agent-files) - Configure `.ai` files (models, tools, orchestration)
- [System Prompts](#system-prompts) - Write effective prompts
- [Related Topics](#related-topics) - Safety patterns, multi-agent orchestration
- [See Also](#see-also) - CLI, configuration, deployment

---

## Agent Files

The most important chapter for building agents. Covers all `.ai` file configuration.

| Page | Description |
|------|-------------|
| [Overview](Agent-Files) | File structure and anatomy |
| [Identity](Agent-Files-Identity) | `description`, `usage`, `toolName` |
| [Models](Agent-Files-Models) | Model selection, fallbacks, caching |
| [Tools](Agent-Files-Tools) | Tool access and filtering |
| [Sub-Agents](Agent-Files-Sub-Agents) | Agent delegation and nesting |
| [Orchestration](Agent-Files-Orchestration) | Advisors, router, handoff |
| [Behavior](Agent-Files-Behavior) | Turns, retries, temperature |
| [Contracts](Agent-Files-Contracts) | Input/output schemas |

---

## System Prompts

Writing effective prompts for your agents.

| Page | Description |
|------|-------------|
| [Overview](System-Prompts) | Prompt structure and anatomy |
| [Writing Prompts](System-Prompts-Writing) | Best practices and patterns |
| [Include Directives](System-Prompts-Includes) | Reusing prompt content |
| [Variables](System-Prompts-Variables) | Available prompt variables |

---

## Related Topics

| Page | Description |
|------|-------------|
| [Safety Gates](Agent-Development-Safety) | Prompt patterns for safe, reliable agents |
| [Multi-Agent Orchestration](Agent-Development-Multi-Agent) | Advisors, routers, handoffs, sub-agents |

---

## See Also

- [CLI Reference](CLI) - Running agents from command line
- [Configuration](Configuration) - Providers and MCP servers setup
- [Headends](Headends) - Deploy agents via REST, Slack, MCP, etc.
