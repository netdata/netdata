# Quick Start

**Goal**: Set up ai-agent from scratch to a working, deployed agent with tools.

**Time**: 15-20 minutes

---

## Table of Contents

- [Prerequisites](#prerequisites) - What you need
- [Phase 1: Configure Infrastructure](#phase-1-configure-infrastructure) - Providers and tools
- [Phase 2: Create Your Agent](#phase-2-create-your-agent) - Write your first .ai file
- [Phase 3: Test and Debug](#phase-3-test-and-debug) - CLI usage, output streams, exit codes
- [Phase 4: Deploy as a Service](#phase-4-deploy-as-a-service) - Systemd with headends
- [Phase 5: Integrate with Tools](#phase-5-integrate-with-tools) - Open-WebUI, Claude Code, VS Code
- [Common Issues](#common-issues) - Troubleshooting
- [Next Steps](#next-steps) - Where to go from here

---

## Prerequisites

Before starting:

- **ai-agent installed** - See [Installation](Getting-Started-Installation)
- **An LLM provider API key** - OpenAI (`sk-...`) or Anthropic (`sk-ant-...`)
- **A terminal** - Unix shell or Windows PowerShell

---

## Phase 1: Configure Infrastructure

Before creating any agent, you must configure the infrastructure: at least one LLM provider and optionally MCP tools.

### Step 1.1: Create Configuration Directory

```bash
mkdir -p ~/.ai-agent
```

### Step 1.2: Create Environment File

Store your API keys in `~/.ai-agent/ai-agent.env`:

```bash
cat > ~/.ai-agent/ai-agent.env << 'EOF'
OPENAI_API_KEY=sk-your-key-here
EOF
chmod 600 ~/.ai-agent/ai-agent.env
```

For Anthropic, use `ANTHROPIC_API_KEY=sk-ant-...` instead.

> **Security**: This file is `chmod 600` (owner-only). Keys are never stored in `.ai-agent.json`.

### Step 1.3: Create Configuration File

Create `~/.ai-agent/ai-agent.json`:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {
    "context7": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@context7/mcp-server"]
    }
  }
}
```

**What this configures**:

| Section               | Purpose                                                     |
| --------------------- | ----------------------------------------------------------- |
| `providers.openai`    | LLM provider with API key from environment                  |
| `mcpServers.context7` | Free MCP tool for library documentation (no API key needed) |

For Anthropic, replace the provider:

```json
{
  "providers": {
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  }
}
```

### Step 1.4: Verify Configuration

```bash
ai-agent --list-tools all
```

Expected output shows available tools for each MCP server. The exact tools depend on which MCP servers you have configured.

If you see errors, check:

- API key is set correctly in `ai-agent.env`
- `npx` is available (Node.js installed)
- JSON syntax is valid

---

## Phase 2: Create Your Agent

Now create an agent that uses the configured infrastructure.

### Step 2.1: Create Agent Directory

```bash
mkdir -p ~/agents
cd ~/agents
```

### Step 2.2: Create Your First Agent

Create `docs-helper.ai`:

```yaml
#!/usr/bin/env ai-agent
---
description: Answers questions using library documentation
models:
  - openai/gpt-4o-mini
tools:
  - context7
maxTurns: 10
---
You are a helpful programming assistant.

Use available MCP tools to help users with their questions.
```

For Anthropic, change `models:` to `- anthropic/claude-3.5-sonnet`.

**File structure explained**:

| Section                   | Purpose                               |
| ------------------------- | ------------------------------------- |
| `#!/usr/bin/env ai-agent` | Shebang - makes file executable       |
| `---`                     | YAML frontmatter delimiters           |
| `description`             | Human-readable description            |
| `models`                  | LLM to use (format: `provider/model`) |
| `tools`                   | MCP servers this agent can use        |
| `maxTurns`                | Maximum conversation turns            |
| Text after `---`          | System prompt - agent instructions    |

### Step 2.3: Make Executable

```bash
chmod +x docs-helper.ai
```

---

## Phase 3: Test and Debug

### Step 3.1: Basic Run

Two ways to run an agent:

```bash
# Direct execution (requires chmod +x and shebang)
./docs-helper.ai "How do I create a React component?"

# Explicit invocation (works without chmod)
ai-agent @docs-helper.ai "How do I create a React component?"
```

Both are equivalent. Direct execution is shorter; explicit invocation works on any `.ai` file.

**Output streams**:

| Stream   | Content                              |
| -------- | ------------------------------------ |
| `stdout` | Final agent response only            |
| `stderr` | Progress, tool calls, debugging info |

This separation allows piping: `./docs-helper.ai "query" > result.txt`

### Step 3.2: Verbose Mode

```bash
./docs-helper.ai --verbose "How do I use useState in React?"
```

Verbose output shows:

- Configuration resolution
- Model selection
- Tool calls and responses
- Token usage
- Timing information

### Step 3.3: Dry Run (Validation Only)

```bash
./docs-helper.ai --dry-run "test"
```

Validates configuration and agent file without calling the LLM. Use this to check for errors before real runs.

### Step 3.4: Understanding Exit Codes

The CLI uses specific exit codes to indicate different failure conditions:

| Code | Meaning                                 | Action                                                      |
| ---- | --------------------------------------- | ----------------------------------------------------------- |
| 0    | Success                                 | Agent completed successfully                                |
| 1    | Configuration or fatal error            | Check `.ai-agent.json`, agent file, or system configuration |
| 3    | MCP server errors (list-tools mode)     | Check MCP server configuration                              |
| 4    | CLI/headend errors or invalid arguments | Check command-line arguments, headend configuration         |
| 5    | Schema validation errors (list-tools)   | Check tool schemas                                          |

LLM-related errors and most runtime failures exit with code 1 with detailed error messages in stderr.

**Script usage**:

```bash
./docs-helper.ai "query"
if [ $? -eq 0 ]; then
  echo "Success"
else
  echo "Failed with exit code $?"
fi
```

---

## Phase 4: Deploy as a Service

ai-agent can run as a persistent service exposing multiple headends (APIs).

### Step 4.1: Create Service Configuration

Create `/etc/ai-agent/ai-agent.json` (or use `~/.ai-agent/ai-agent.json`):

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {
    "context7": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@context7/mcp-server"]
    }
  }
}
```

**Note**: The systemd service assumes the `ai-agent` user and `/opt/ai-agent` directory exist. Create them:

```bash
sudo useradd -r -s /bin/false ai-agent
sudo mkdir -p /opt/ai-agent/agents
sudo chown -R ai-agent:ai-agent /opt/ai-agent
sudo cp ~/agents/docs-helper.ai /opt/ai-agent/agents/
```

### Step 4.2: Create Systemd Service

Create `/etc/systemd/system/ai-agent.service`:

```ini
[Unit]
Description=AI Agent Service
After=network.target

[Service]
Type=simple
User=ai-agent
Group=ai-agent
WorkingDirectory=/opt/ai-agent
ExecStart=/opt/ai-agent/bin/ai-agent --openai-completions 3000 --agent /opt/ai-agent/agents/docs-helper.ai
Restart=always
RestartSec=5
EnvironmentFile=/etc/ai-agent/ai-agent.env

[Install]
WantedBy=multi-user.target
```

### Step 4.3: Start the Service

```bash
sudo systemctl daemon-reload
sudo systemctl enable ai-agent
sudo systemctl start ai-agent
sudo systemctl status ai-agent
```

### Step 4.4: Available Headends

| Headend              | Flag                             | Use Case                         |
| -------------------- | -------------------------------- | -------------------------------- |
| OpenAI-compatible    | `--openai-completions <port>`    | Open-WebUI, any OpenAI client    |
| Anthropic-compatible | `--anthropic-completions <port>` | Claude-compatible clients        |
| MCP                  | `--mcp stdio`                    | Claude Code, VS Code (stdio)     |
| MCP HTTP             | `--mcp http:<port>`              | HTTP-based MCP clients           |
| MCP SSE              | `--mcp sse:<port>`               | SSE-based MCP clients            |
| MCP WebSocket        | `--mcp ws:<port>`                | WebSocket-based MCP clients      |
| REST                 | `--api <port>`                   | Custom integrations              |
| Slack                | `--slack`                        | Slack bot (Socket Mode, no port) |

Multiple headends can run simultaneously on different ports:

```bash
ai-agent --openai-completions 3000 --api 3001 --mcp stdio --agent docs-helper.ai
```

---

## Phase 5: Integrate with Tools

### Open-WebUI (LLM Headend)

1. Start ai-agent with OpenAI-compatible headend:

   ```bash
   ai-agent --openai-completions 3000 --agent docs-helper.ai
   ```

2. In Open-WebUI settings, add connection:
   - **URL**: `http://localhost:3000/v1`
   - **API Key**: (any non-empty string)

3. Select your agent from the model dropdown (appears as `docs-helper`)

### Claude Code (MCP)

1. Add to Claude Code's MCP configuration (`~/.claude/claude_desktop_config.json`):

   ```json
   {
     "mcpServers": {
       "ai-agent": {
         "command": "ai-agent",
         "args": ["--mcp", "stdio", "--agent", "/path/to/docs-helper.ai"]
       }
     }
   }
   ```

2. Restart Claude Code. Your agent's tools appear as MCP tools.

### VS Code (MCP via Extension)

1. Install an MCP-compatible extension

2. Configure the extension to use:

   ```bash
   ai-agent --mcp stdio --agent /path/to/docs-helper.ai
   ```

3. Agent tools become available in VS Code

---

## Common Issues

### "No providers configured"

**Cause**: Configuration file not found or malformed.

**Solution**:

1. Check `~/.ai-agent/ai-agent.json` exists
2. Validate JSON syntax: `jq . ~/.ai-agent/ai-agent.json`
3. Run `ai-agent --dry-run @agent.ai "test"` to see errors

### "API key not set" / Authentication errors

**Cause**: Environment variable not loaded.

**Solution**:

1. Check `ai-agent.env` has the key: `cat ~/.ai-agent/ai-agent.env`
2. Verify key format (OpenAI: `sk-...`, Anthropic: `sk-ant-...`)
3. Test key directly with provider's API

### "Unknown model"

**Cause**: Model name doesn't match provider.

**Solution**: Use exact model names in the format `provider/model`:

- OpenAI: `openai/gpt-4o`, `openai/gpt-4o-mini`
- Anthropic: `anthropic/claude-3.5-sonnet`, `anthropic/claude-3-haiku`

Check your provider's documentation for the exact model identifier.

### MCP server fails to start

**Cause**: Missing dependencies or wrong command.

**Solution**:

1. Test manually: `npx -y @context7/mcp-server`
2. Check `node` and `npx` are installed
3. Check network access for npm packages

### Agent runs but no tool calls

**Cause**: Agent doesn't have tools configured or prompt doesn't trigger tool use.

**Solution**:

1. Verify `tools:` section lists the MCP server name
2. Check `--list-tools all` shows the tools
3. Make prompt explicitly request tool use

---

## What You Learned

After completing this guide:

- Configure providers and MCP servers in `.ai-agent.json`
- Store API keys securely in `.ai-agent.env`
- Create agents with tools using `.ai` files
- Test with `--verbose`, `--dry-run`, and understand exit codes
- Deploy as systemd service with headends
- Integrate with Open-WebUI, Claude Code, and VS Code

---

## Next Steps

| Goal                        | Page                                     |
| --------------------------- | ---------------------------------------- |
| Add more MCP tools          | [MCP Servers](Configuration-MCP-Servers) |
| Create multi-agent systems  | [Sub-Agents](Agent-Files-Sub-Agents)     |
| Build REST API integrations | [REST Tools](Configuration-REST-Tools)   |
| Advanced debugging          | [Debugging Guide](Operations-Debugging)  |
| All frontmatter options     | [Frontmatter Reference](Agent-Files)     |

---

## See Also

- [Installation](Getting-Started-Installation) - Detailed installation options
- [Environment Variables](Getting-Started-Environment-Variables) - All environment configuration
- [Configuration](Configuration) - Deep dive into configuration files
- [Headends](Headends) - All headend types and options
- [Exit Codes](Operations-Exit-Codes) - Complete exit code reference
