# AI Agent Wiki

Welcome to **ai-agent** - a framework for building autonomous AI agents. Write a prompt, add tools, and run it.

---

## What is ai-agent?

ai-agent is a TypeScript framework that turns simple text files (`.ai` files) into powerful autonomous agents. Each agent gets:

- A system prompt that defines its behavior
- Access to tools (MCP servers, REST APIs, other agents)
- Automatic orchestration with retries, context management, and error recovery

**No orchestration code required** - just configuration and prompts.

---

## Quick Start

1. **Install**: See [Installation](Getting-Started-Installation) (package is private, install from source)
2. **Configure**: Create `~/.ai-agent/ai-agent.json` with your API keys
3. **Run**: `ai-agent --agent my-agent.ai "Hello, world!"`

See [Getting Started](Getting-Started) for the complete walkthrough.

---

## Key Sections

### For Building Agents

| Section                          | What You'll Find                                           |
| -------------------------------- | ---------------------------------------------------------- |
| [Agent Files](Agent-Files)       | Configure `.ai` files - models, tools, sub-agents, schemas |
| [System Prompts](System-Prompts) | Write effective prompts with includes and variables        |
| [CLI Reference](CLI)             | Run agents, debug, override settings, script automation    |

### For Configuration

| Section                        | What You'll Find                                           |
| ------------------------------ | ---------------------------------------------------------- |
| [Configuration](Configuration) | Providers, MCP servers, REST tools, caching                |
| [Headends](Headends)           | Deploy as REST API, MCP server, Slack bot, embedded widget |

### For Operations

| Section                            | What You'll Find                             |
| ---------------------------------- | -------------------------------------------- |
| [Operations](Operations)           | Logging, debugging, snapshots, telemetry     |
| [Technical Specs](Technical-Specs) | Architecture, session lifecycle, tool system |

---

## Common Tasks

**I want to...**

- **Create my first agent** - [Quick Start](Getting-Started-Quick-Start)
- **Add tools to my agent** - [Agent Files: Tools](Agent-Files-Tools)
- **Use a different model** - [Agent Files: Models](Agent-Files-Models)
- **Debug why my agent isn't working** - [CLI: Debugging](CLI-Debugging)
- **Deploy as a REST API** - [Headends: REST](Headends-REST)
- **Run agents in Slack** - [Headends: Slack](Headends-Slack)
- **Understand session snapshots** - [Operations: Snapshots](Operations-Snapshots)

---

## Architecture at a Glance

- **Recursive Autonomous Agents**: Each agent plans, executes, observes, and adapts
- **100% Session Isolation**: Every agent run is independent with zero shared state
- **TypeScript + Vercel AI SDK 5**: Type safety with unified provider interface
- **Library-First Design**: Core is an embeddable library; CLI is a thin wrapper

---

## For AI Assistants

Building ai-agents programmatically? See the [AI Agent Configuration Guide](skills/ai-agent-configuration.md) - a single-page reference covering frontmatter schema, tool composition, patterns, and output contracts.

---

## See Also

- [Getting Started](Getting-Started) - Installation and first steps
- [Contributing](Contributing) - Help improve ai-agent
- [Advanced](Advanced) - Hidden options and internal API
