# CLI: Running Agents

How to execute agents from the command line. Covers agent selection, prompt input, stdin handling, and output control.

---

## Table of Contents

- [Basic Execution](#basic-execution) - Running agents with prompts
- [Agent Selection](#agent-selection) - The `--agent` flag
- [Prompt Input](#prompt-input) - Command line, stdin, and piping
- [Output Control](#output-control) - Formatting and destination
- [Headend Options](#headend-options) - Server mode flags
- [Tool Selection](#tool-selection) - MCP server access
- [Sub-Agent Composition](#sub-agent-composition) - Multi-agent setups
- [See Also](#see-also) - Related pages

---

## Basic Execution

### Using an Agent File

The most common pattern: specify an agent and a prompt.

```bash
ai-agent --agent myagent.ai "Your question or task here"
```

The agent file contains the system prompt and configuration in frontmatter. The user prompt is your command-line argument.

### Direct Execution

Agent files can be made executable and run directly:

```bash
# Make executable
chmod +x myagent.ai

# Run with prompt
./myagent.ai "Hello, agent!"
```

### Inline Prompts

Without `--agent`, provide system and user prompts as arguments:

```bash
ai-agent "You are a helpful coding assistant" "Write a hello world in Python"
```

First argument is the system prompt; second is the user prompt.

---

## Agent Selection

### The `--agent` Flag

| Property | Value |
|----------|-------|
| Flag | `--agent <path>` |
| Repeatable | Yes |
| Required | Yes (for headend mode), optional for inline prompts |

**Description**: Register an agent file (`.ai`) for execution. In headend mode, multiple agents can be registered.

**Examples**:

```bash
# Single agent
ai-agent --agent chat.ai "Hello"

# Multiple agents (headend mode)
ai-agent --agent chat.ai --agent research.ai --api 8080

# Glob patterns
ai-agent --agent agents/*.ai --api 8080
```

**Resolution**:
- Paths are relative to current working directory
- Sub-agents referenced in frontmatter are auto-loaded
- Missing files cause immediate exit with error

---

## Prompt Input

### Command Line Arguments

Prompts provided directly as arguments:

```bash
# With agent file (user prompt only)
ai-agent --agent chat.ai "What time is it?"

# Without agent file (system + user prompts)
ai-agent "You are helpful" "What time is it?"
```

### Standard Input (stdin)

Use `-` as placeholder to read from stdin:

```bash
# User prompt from stdin
echo "Summarize this" | ai-agent --agent chat.ai -

# System prompt from stdin (rare)
echo "You are a pirate" | ai-agent - "Tell me about treasure"
```

### Piping Content

Pipe file contents or command output:

```bash
# Pipe file content
cat document.txt | ai-agent --agent summarizer.ai -

# Pipe command output
ls -la | ai-agent --agent analyst.ai "Analyze this directory listing"

# Combine with heredoc
ai-agent --agent coder.ai "Fix this code" <<EOF
function broken() {
  return undefined
}
EOF
```

### Multi-line Prompts

Use shell quoting or heredocs:

```bash
# Shell quoting
ai-agent --agent chat.ai "First line
Second line
Third line"

# Heredoc (better for complex prompts)
ai-agent --agent chat.ai - <<'EOF'
Please analyze the following requirements:
1. Must be fast
2. Must be reliable
3. Must be cheap

Provide recommendations.
EOF
```

---

## Output Control

### Output Format

| Property | Value |
|----------|-------|
| Flag | `--format <format>` |
| Default | None (uses agent default or markdown) |
| Values | `markdown`, `json`, `slack-block-kit`, `text`, custom |

**Description**: Controls how the agent formats its response. This is passed to the agent via the `${FORMAT}` variable.

```bash
# Request JSON output
ai-agent --agent api.ai --format json "Get user data"

# Request plain text
ai-agent --agent chat.ai --format text "Simple answer please"
```

### Structured Output (JSON Schema)

| Property | Value |
|----------|-------|
| Flag | `--schema <schema>` |
| Format | Inline JSON or `@path` to file |

**Description**: Forces JSON output with validation against a schema.

```bash
# Inline schema
ai-agent --agent extractor.ai --schema '{"type":"object","properties":{"name":{"type":"string"}}}' "Extract name from: John Smith"

# Schema from file
ai-agent --agent extractor.ai --schema @schemas/person.json "Extract person data"
```

### Streaming Control

| Property | Value |
|----------|-------|
| Flag | `--stream` / `--no-stream` |
| Default | `true` (streaming enabled) |

**Description**: Controls whether output is streamed as generated or delivered complete.

```bash
# Disable streaming (wait for complete response)
ai-agent --agent chat.ai --no-stream "Generate a long story"

# Force streaming (explicit)
ai-agent --agent chat.ai --stream "Hello"
```

### Quiet Mode

| Property | Value |
|----------|-------|
| Flag | `--quiet` |
| Default | `false` |

**Description**: Suppress all log output except critical errors. Only the agent's response is printed.

```bash
ai-agent --agent chat.ai --quiet "Hello" > response.txt
```

---

## Headend Options

When headend flags are present, ai-agent starts persistent servers instead of running a single query.

### REST API Headend

| Property | Value |
|----------|-------|
| Flag | `--api <port>` |
| Repeatable | Yes |
| Concurrency | `--api-concurrency <n>` (default: 4) |

```bash
# Single port
ai-agent --agent api.ai --api 8080

# Multiple ports
ai-agent --agent api.ai --api 8080 --api 8081
```

### MCP Headend

| Property | Value |
|----------|-------|
| Flag | `--mcp <transport>` |
| Transports | `stdio`, `http:<port>`, `sse:<port>`, `ws:<port>` |

```bash
# Stdio transport (for Claude Desktop, etc.)
ai-agent --agent tools.ai --mcp stdio

# HTTP transport
ai-agent --agent tools.ai --mcp http:3000

# SSE transport
ai-agent --agent tools.ai --mcp sse:3000
```

### OpenAI-Compatible API

| Property | Value |
|----------|-------|
| Flag | `--openai-completions <port>` |
| Concurrency | `--openai-completions-concurrency <n>` (default: 4) |

```bash
ai-agent --agent chat.ai --openai-completions 8080
```

### Anthropic-Compatible API

| Property | Value |
|----------|-------|
| Flag | `--anthropic-completions <port>` |
| Concurrency | `--anthropic-completions-concurrency <n>` (default: 4) |

```bash
ai-agent --agent chat.ai --anthropic-completions 8080
```

### Embed Headend

| Property | Value |
|----------|-------|
| Flag | `--embed <port>` |
| Concurrency | `--embed-concurrency <n>` (default: 10) |

```bash
ai-agent --agent widget.ai --embed 8080
```

### Slack Headend

| Property | Value |
|----------|-------|
| Flag | `--slack` |

```bash
ai-agent --agent slackbot.ai --slack
```

Requires Slack environment variables. See [Headends-Slack](Headends-Slack).

---

## Tool Selection

### The `--tools` Flag

| Property | Value |
|----------|-------|
| Flag | `--tools <list>` |
| Aliases | `--tool`, `--mcp`, `--mcp-tool`, `--mcp-tools` |
| Format | Comma-separated MCP server names |

**Description**: Specify which MCP servers the agent can access.

```bash
# Single server
ai-agent --agent code.ai --tools github "Create a PR"

# Multiple servers
ai-agent --agent research.ai --tools "github,slack,web" "Research topic"
```

### Listing Available Tools

```bash
# List all configured MCP servers and their tools
ai-agent --list-tools all

# List specific server
ai-agent --list-tools github
```

---

## Sub-Agent Composition

### The `--agents` Flag

| Property | Value |
|----------|-------|
| Flag | `--agents <list>` |
| Format | Comma-separated paths to .ai files |

**Description**: Load sub-agents as tools for the master agent.

```bash
# Compose agents
ai-agent --agent orchestrator.ai --agents "researcher.ai,writer.ai" "Write a blog post"
```

Sub-agents become callable tools. The master agent decides when to delegate.

---

## See Also

- [CLI](CLI) - CLI overview and quick reference
- [CLI-Debugging](CLI-Debugging) - Debugging agent execution
- [CLI-Overrides](CLI-Overrides) - Runtime configuration overrides
- [CLI-Scripting](CLI-Scripting) - Using ai-agent in scripts
- [Agent-Files](Agent-Files) - Agent file structure
- [Headends](Headends) - Headend deployment details
