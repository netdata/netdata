# AI Agent Wiki

**Build powerful AI agents in minutes, not months. Write a prompt, add tools, run it.**

---

## Quick Navigation

| Section | Description |
|---------|-------------|
| [Getting Started](Getting-Started) | Installation, first agent, CLI basics |
| [Agent Development](Agent-Development) | `.ai` files, frontmatter, multi-agent patterns |
| [Configuration](Configuration) | Providers, tools, caching, context windows |
| [Headends](Headends) | Deployment modes (CLI, REST, MCP, Slack, etc.) |
| [Operations](Operations) | Debugging, logging, snapshots, telemetry |
| [Technical Specs](Technical-Specs) | Architecture, design, behavior contracts |
| [Advanced](Advanced) | Hidden options, overrides, internal API |
| [Contributing](Contributing) | Testing, code style, documentation |

---

## Core Architecture

- **Recursive Autonomous Agents**: Every agent continuously plans, executes, observes, and adapts
- **100% Session Isolation**: Each agent run is a completely independent universe with ZERO shared state
- **TypeScript + Vercel AI SDK 5**: Full type safety with unified provider interface
- **Library-First Design**: Core functionality as embeddable library; CLI is thin wrapper

---

## Key Capabilities

### LLM Provider Support
- **Commercial**: OpenAI, Anthropic, Google
- **Open-Source**: OpenRouter, Ollama, self-hosted (vLLM, llama.cpp)
- Provider/model fallback chains
- Per-model context window and tokenizer configuration

### Multi-Agent Orchestration
- **Advisors**: Parallel pre-run agents
- **Router**: Delegation via `router__handoff-to` tool
- **Handoff**: Post-run execution chains
- **Agent-as-Tool**: Any `.ai` file becomes callable

### Tool System
- MCP tools auto-converted to SDK tools
- REST/OpenAPI tool support
- Queue-based concurrency control
- Tool output storage for oversized responses

---

## Headend Modes

| Headend | Purpose |
|---------|---------|
| `--cli` | Direct agent execution |
| `--api <port>` | REST API |
| `--mcp <transport>` | MCP server |
| `--openai-completions <port>` | OpenAI-compatible API |
| `--anthropic-completions <port>` | Anthropic-compatible API |
| `--embed <port>` | Public embeddable chat |
| `--slack` | Slack Socket Mode app |

---

## For AI Assistants

**[AI Agent Configuration Guide](skills/ai-agent-configuration.md)** - Single-page reference for LLM agents building ai-agents. Covers frontmatter schema, tool composition, patterns, and output contracts.

---

## Real Impact

- **Neda CRM**: 22 specialized agents, ~2,000 lines of prompts
- **Traditional equivalent**: 20,000+ lines of orchestration code
- **Development time**: weeks vs months
