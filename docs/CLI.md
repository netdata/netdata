# CLI Reference

Command line interface for ai-agent. Run agents, configure behavior, and integrate with automation.

---

## Table of Contents

- [Overview](#overview) - Basic concepts and usage patterns
- [Quick Reference](#quick-reference) - Common commands at a glance
- [Execution Modes](#execution-modes) - Direct vs headend mode
- [Getting Help](#getting-help) - Built-in help and version info
- [Sub-Pages](#sub-pages) - Detailed documentation by topic
- [See Also](#see-also) - Related documentation

---

## Overview

ai-agent runs in two primary modes:

1. **Direct Mode** - Run a single agent with a prompt, get a response, exit
2. **Headend Mode** - Start persistent servers (REST, MCP, Slack, etc.)

All CLI options follow these conventions:

- **Long flags** use `--kebab-case` (e.g., `--max-turns`)
- **Boolean flags** support negation with `--no-` prefix (e.g., `--no-stream`)
- **Repeatable flags** can be specified multiple times (e.g., `--agent a.ai --agent b.ai`)
- **Duration values** accept milliseconds or human-readable formats (e.g., `5000`, `5s`, `2m`)

---

## Quick Reference

### Run an Agent (Direct Mode)

```bash
# Using an agent file
ai-agent --agent chat.ai "What is the capital of France?"

# Using inline prompts (system + user)
ai-agent "You are a helpful assistant" "List files in current directory"

# Shorthand: .ai files are directly executable
./my-agent.ai "Hello, world!"
```

### Start a Server (Headend Mode)

```bash
# REST API server
ai-agent --agent api.ai --api 8080

# MCP server (stdio transport)
ai-agent --agent tools.ai --mcp stdio

# Multiple headends simultaneously
ai-agent --agent main.ai --api 8080 --mcp stdio --slack

# With concurrency limits
ai-agent --agent api.ai --api 8080 --api-concurrency 10
```

### Debug and Inspect

```bash
# Verbose logging
ai-agent --agent debug.ai --verbose "test query"

# Trace LLM calls
ai-agent --agent debug.ai --trace-llm "test query"

# Dry run (validate without executing)
ai-agent --agent test.ai --dry-run "test query"
```

### Override Settings

```bash
# Change model
ai-agent --agent chat.ai --models openai/gpt-4o "Hello"

# Adjust temperature
ai-agent --agent chat.ai --temperature 0.5 "Be creative"

# Multiple overrides
ai-agent --agent chat.ai --override temperature=0.8 --override maxTurns=5 "Query"
```

---

## Execution Modes

### Direct Mode

Runs one agent session, outputs the response, and exits. Use this for:

- Interactive CLI usage
- Script integration
- Testing agent configurations

```bash
ai-agent --agent assistant.ai "Your question here"
```

**Input sources:**

- Command line arguments
- Standard input (stdin) with `-` placeholder
- Piped content

### Headend Mode

Starts persistent servers that accept multiple requests. Use this for:

- Production deployments
- API integrations
- Bot platforms (Slack)

```bash
ai-agent --agent api.ai --api 8080
```

Triggered when any headend flag is present:

- `--api <port>` - REST API
- `--mcp <transport>` - Model Context Protocol transport (stdio|http:port|sse:port|ws:port)
- `--openai-completions <port>` - OpenAI-compatible API
- `--anthropic-completions <port>` - Anthropic-compatible API
- `--embed <port>` - Embeddable chat widget
- `--slack` - Slack Socket Mode

Concurrency controls (headend-specific):

- `--api-concurrency <n>` - Maximum concurrent REST API sessions
- `--openai-completions-concurrency <n>` - Maximum concurrent OpenAI chat sessions
- `--anthropic-completions-concurrency <n>` - Maximum concurrent Anthropic chat sessions
- `--embed-concurrency <n>` - Maximum concurrent embed sessions

---

## Getting Help

### Version Information

```bash
ai-agent --version
ai-agent -V
```

### Help Output

```bash
ai-agent --help
```

The help output includes:

- Frontmatter template for creating agents
- Configuration file resolution order
- All CLI options grouped by category
- Usage examples

### List Available Tools

```bash
# List all MCP server tools
ai-agent --list-tools all

# List tools from a specific server
ai-agent --list-tools github

# Validate tool schemas against JSON Schema drafts
ai-agent --list-tools all --schema-validate draft-07
```

---

## Sub-Pages

Detailed documentation organized by use case:

| Page                                     | Description                                                         |
| ---------------------------------------- | ------------------------------------------------------------------- |
| [CLI-Running-Agents](CLI-Running-Agents) | Running agents: `--agent`, prompts, stdin, piping, output formats   |
| [CLI-Debugging](CLI-Debugging)           | Debugging: `--verbose`, `--trace-*`, `--dry-run`, diagnostics       |
| [CLI-Overrides](CLI-Overrides)           | Runtime overrides: `--model`, `--temperature`, all override options |
| [CLI-Scripting](CLI-Scripting)           | Scripting: exit codes, JSON output, error handling, automation      |

---

## Option Categories

CLI options are grouped into four scopes:

### Master Agent Overrides

Apply only to the top-level agent (not inherited by sub-agents):

- `--models` - LLM model selection
- `--tools` - Tool access
- `--agents` - Sub-agent composition
- `--schema` - Structured output schema

### Master Defaults

Apply to master agent and inherited by sub-agents when unset:

- `--temperature` - Response creativity
- `--top-p` - Token selection diversity (0.0-1.0)
- `--top-k` - Limits token selection to K most probable tokens
- `--max-turns` - Turn limit
- `--max-retries` - Retry count
- `--max-output-tokens` - Maximum response length per turn
- `--repeat-penalty` - Reduces repetitive text (1.0=off, higher=stronger)
- `--max-tool-calls-per-turn` - Maximum parallel tool calls per turn
- `--reasoning` - Reasoning effort level (none, minimal, low, medium, high)
- `--default-reasoning` - Default reasoning for agents that omit it
- `--reasoning-tokens` - Reasoning token budget for Anthropic thinking mode
- `--caching` - Anthropic caching mode (full or none)
- `--cache` - Response cache TTL (off or duration like 5s/2m)
- Timeout settings

### All Models Overrides

Apply to every agent including sub-agents:

- `--override` - Key=value overrides
- `--verbose` - Logging level
- `--trace-*` - Tracing flags
- `--format` - Output format

### Global Controls

Application-level settings:

- `--config` - Configuration file path
- `--dry-run` - Validation mode
- `--quiet` - Suppress output
- `--sessions-dir` - Session save location
- `--billing-file` - Token usage log
- `--resume` - Resume interrupted session
- `--save-all <dir>` - Save all agent and sub-agent conversations to directory
- `--show-tree` - Dump full execution tree (ASCII) at the end
- `--mcp-init-concurrency <n>` - MCP server initialization concurrency
- Telemetry settings:
  - `--telemetry-enabled` - Enable OTLP metrics / Prometheus
  - `--telemetry-otlp-endpoint` - OTLP gRPC endpoint
  - `--telemetry-otlp-timeout-ms` - OTLP export timeout
  - `--telemetry-prometheus-enabled` - Expose Prometheus /metrics endpoint
  - `--telemetry-prometheus-host` - Prometheus /metrics host interface
  - `--telemetry-prometheus-port` - Prometheus /metrics port
  - `--telemetry-traces-enabled` - Enable OTLP tracing
  - `--telemetry-trace-sampler` - Tracing sampler (always_on, always_off, parent, ratio)
  - `--telemetry-trace-ratio` - Sampling ratio (0-1) when sampler=ratio
  - `--telemetry-label` - Additional telemetry labels (key=value)
  - `--telemetry-log-format` - Logging format (journald, logfmt, json, none)
  - `--telemetry-log-extra` - Additional log sinks (e.g., otlp)
  - `--telemetry-logging-otlp-endpoint` - OTLP endpoint for log exports
  - `--telemetry-logging-otlp-timeout-ms` - OTLP timeout for log exports

---

## See Also

- [Getting-Started-Quick-Start](Getting-Started-Quick-Start) - First steps with ai-agent
- [Agent-Files](Agent-Files) - Agent file structure and frontmatter
- [Configuration](Configuration) - Configuration file reference
- [Headends](Headends) - Headend deployment options
