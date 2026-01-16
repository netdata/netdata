# Tool Configuration

Configure which tools your agent can use, including MCP servers, REST tools, and OpenAPI endpoints.

---

## Table of Contents

- [Overview](#overview) - What tool configuration controls
- [Quick Example](#quick-example) - Basic tool configuration
- [Configuration Reference](#configuration-reference) - All tool-related keys
- [Tool Types](#tool-types) - MCP servers, REST tools, OpenAPI
- [Tool Filtering](#tool-filtering) - Allowing and denying specific tools
- [Common Patterns](#common-patterns) - Typical tool configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Tool configuration controls:
- **Which tools** the agent can use (`tools`)
- **Which specific tools** are allowed from a source (`toolsAllowed`)
- **Which specific tools** are denied from a source (`toolsDenied`)
- **Tool output handling** (`toolResponseMaxBytes`, `toolOutput`)

**User questions answered**: "How do I give my agent tools?" / "How do I restrict tools?"

---

## Quick Example

Single MCP server:

```yaml
---
description: Agent with filesystem access
models:
  - openai/gpt-4o
tools:
  - filesystem
---
```

Multiple tool sources:

```yaml
---
description: Research agent
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave          # MCP server: web search
  - fetcher        # MCP server: URL fetching
  - rest__catalog  # REST tool: product catalog
---
```

---

## Configuration Reference

### tools

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | `[]` (no tools) |
| Valid values | MCP server names, REST tool names, `openapi:` prefixes |

**Description**: Which tools the agent can use. Lists MCP server names (from `.ai-agent.json`), REST tools, or OpenAPI providers.

**What it affects**:
- Available tools for the agent
- Which MCP servers are initialized at startup
- Which REST/OpenAPI tools are available
- Tool-related costs and latency

**Example**:
```yaml
---
# Single tool source
tools: filesystem

# Multiple tool sources
tools:
  - brave           # MCP server
  - fetcher         # MCP server
  - rest__catalog   # REST tool
  - openapi:github  # All tools from OpenAPI spec
---
```

**Tool Naming**:

| Source Type | Name Format | Example |
|-------------|-------------|---------|
| MCP Server | Server name from `.ai-agent.json` | `brave`, `filesystem` |
| REST Tool | `rest__<tool-name>` | `rest__catalog` |
| OpenAPI | `openapi:<provider-name>` | `openapi:github` |

**Notes**:
- MCP server names must match entries in `.ai-agent.json` `mcpServers` section
- Listing a server exposes ALL its tools by default
- Use `toolsAllowed`/`toolsDenied` to filter specific tools

---

### toolsAllowed

| Property | Value |
|----------|-------|
| Type | Configured per MCP server in `.ai-agent.json` |
| Default | All tools allowed |

**Description**: Whitelist of specific tools from an MCP server. Configured in `.ai-agent.json`, not frontmatter.

**What it affects**:
- Limits which tools from a server are available
- Reduces tool count shown to the LLM
- Improves focus for specific tasks

**Configuration** (in `.ai-agent.json`):
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem"],
      "toolsAllowed": ["read_file", "list_directory"]
    }
  }
}
```

**Notes**:
- Configured per MCP server in `.ai-agent.json`
- Only listed tools are exposed to the agent
- Cannot be set in frontmatter (system-level configuration)

---

### toolsDenied

| Property | Value |
|----------|-------|
| Type | Configured per MCP server in `.ai-agent.json` |
| Default | No tools denied |

**Description**: Blacklist of specific tools from an MCP server. Configured in `.ai-agent.json`, not frontmatter.

**What it affects**:
- Blocks specific tools from being available
- All other tools from the server remain available
- Useful for removing dangerous or irrelevant tools

**Configuration** (in `.ai-agent.json`):
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem"],
      "toolsDenied": ["delete_file", "write_file"]
    }
  }
}
```

**Notes**:
- Configured per MCP server in `.ai-agent.json`
- Listed tools are blocked; all others allowed
- Cannot be set in frontmatter (system-level configuration)
- If both `toolsAllowed` and `toolsDenied` are set, `toolsAllowed` takes precedence

---

### toolResponseMaxBytes

| Property | Value |
|----------|-------|
| Type | `integer` |
| Default | `12288` (12 KB) |
| Valid values | `0` or greater |

**Description**: Maximum tool output size kept in conversation. Larger outputs are stored separately and replaced with a `tool_output` handle.

**What it affects**:
- How large tool outputs are handled
- Context window usage
- Whether `tool_output` tool is invoked for retrieval

**Example**:
```yaml
---
toolResponseMaxBytes: 24576    # 24 KB before storing
---

---
toolResponseMaxBytes: 8192     # 8 KB (more aggressive)
---
```

**How It Works**:
1. Tool returns output larger than `toolResponseMaxBytes`
2. Output is stored in `/tmp/ai-agent-<run-hash>/`
3. Model receives a handle message instead of full output
4. Model can call `tool_output` tool to retrieve chunks

**Notes**:
- Prevents large outputs from consuming context window
- The model is told how to retrieve stored data
- Useful for tools returning large files or data sets

---

### toolOutput

| Property | Value |
|----------|-------|
| Type | `object` |
| Default | Uses global defaults |

**Description**: Overrides for the `tool_output` module behavior (chunking large outputs).

**Sub-keys**:

| Sub-key | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | `boolean` | `true` | Enable/disable tool output chunking |
| `maxChunks` | `number` | Varies | Maximum chunks for large outputs |
| `overlapPercent` | `number` | Varies | Overlap between chunks (%) |
| `avgLineBytesThreshold` | `number` | Varies | Threshold for line-based chunking |
| `models` | `string` or `string[]` | None | Models for extraction (optional) |

**Example**:
```yaml
---
toolOutput:
  enabled: true
  maxChunks: 5
  overlapPercent: 10
  avgLineBytesThreshold: 1000
  models: openai/gpt-4o-mini
---
```

**Notes**:
- Controls how oversized tool outputs are processed
- `storeDir` is accepted but ignored (always `/tmp/ai-agent-<run-hash>`)
- `models` can specify cheaper models for extraction tasks

---

## Tool Types

### MCP Servers

MCP (Model Context Protocol) servers are the primary way to add tools. They're configured in `.ai-agent.json` and referenced by name in frontmatter.

**Configuration** (`.ai-agent.json`):
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem", "/path/to/dir"]
    },
    "brave": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-brave-search"],
      "env": {
        "BRAVE_API_KEY": "${BRAVE_API_KEY}"
      }
    }
  }
}
```

**Usage in agent**:
```yaml
---
tools:
  - filesystem
  - brave
---
```

**Common MCP Servers**:

| Server | Purpose |
|--------|---------|
| `@anthropic/mcp-filesystem` | File operations |
| `@anthropic/mcp-brave-search` | Web search |
| `@anthropic/mcp-fetch` | URL fetching |
| `@anthropic/mcp-github` | GitHub operations |
| `@anthropic/mcp-slack` | Slack integration |

### REST Tools

REST tools are HTTP APIs configured in `.ai-agent.json` and used with the `rest__` prefix.

**Configuration** (`.ai-agent.json`):
```json
{
  "restTools": {
    "catalog": {
      "url": "https://api.example.com/catalog",
      "method": "GET",
      "headers": {
        "Authorization": "Bearer ${API_KEY}"
      },
      "description": "Search product catalog"
    }
  }
}
```

**Usage in agent**:
```yaml
---
tools:
  - rest__catalog
---
```

### OpenAPI Tools

OpenAPI tools import all operations from an OpenAPI specification.

**Configuration** (`.ai-agent.json`):
```json
{
  "openapi": {
    "github": {
      "spec": "https://api.github.com/openapi.json",
      "auth": {
        "type": "bearer",
        "token": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

**Usage in agent**:
```yaml
---
tools:
  - openapi:github
---
```

---

## Tool Filtering

### When to Filter Tools

Filter tools when:
- An MCP server exposes dangerous operations (delete, write)
- You want to reduce the tool count for the LLM
- Specific tools are irrelevant to the agent's purpose

### Allow Specific Tools Only

Use `toolsAllowed` in `.ai-agent.json` to whitelist:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem", "/data"],
      "toolsAllowed": [
        "read_file",
        "list_directory",
        "search_files"
      ]
    }
  }
}
```

Only `read_file`, `list_directory`, and `search_files` are available.

### Deny Specific Tools

Use `toolsDenied` in `.ai-agent.json` to blacklist:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-filesystem", "/data"],
      "toolsDenied": [
        "delete_file",
        "move_file",
        "write_file"
      ]
    }
  }
}
```

All tools EXCEPT the listed ones are available.

### Precedence

If both are set, `toolsAllowed` takes precedence:
- Only tools in `toolsAllowed` are considered
- Then `toolsDenied` is applied to that set

---

## Common Patterns

### Read-Only File Agent

```yaml
---
description: Reads and analyzes files
models:
  - openai/gpt-4o
tools:
  - filesystem  # Configured with toolsAllowed: [read_file, list_directory]
---

You can read files but cannot modify them.
```

### Research Agent

```yaml
---
description: Research agent with web access
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave     # Web search
  - fetcher   # URL fetching
maxTurns: 15
---

Search the web and synthesize findings.
```

### Multi-Source Agent

```yaml
---
description: Agent with multiple tool sources
models:
  - openai/gpt-4o
tools:
  - filesystem        # MCP: file operations
  - brave             # MCP: web search
  - rest__catalog     # REST: product catalog
  - openapi:github    # OpenAPI: GitHub operations
---
```

### No Tools (Pure LLM)

```yaml
---
description: Pure language agent, no tools
models:
  - openai/gpt-4o
# No tools key = no tools
---

Answer questions using only your knowledge.
```

### Large Output Handler

```yaml
---
description: Handles large data files
models:
  - openai/gpt-4o
tools:
  - filesystem
toolResponseMaxBytes: 32768  # 32 KB
toolOutput:
  enabled: true
  maxChunks: 10
  models: openai/gpt-4o-mini  # Cheap model for extraction
---
```

---

## Troubleshooting

### "MCP server 'xyz' not found"

**Problem**: Using an MCP server not defined in `.ai-agent.json`.

**Cause**: Server name doesn't match configuration.

**Solution**: Add server to `.ai-agent.json`:
```json
{
  "mcpServers": {
    "xyz": {
      "command": "npx",
      "args": ["-y", "@example/mcp-xyz"]
    }
  }
}
```

### "No tools available"

**Problem**: Agent has no tools despite `tools:` being set.

**Causes**:
1. MCP server failed to start
2. All tools filtered by `toolsDenied`
3. Server name misspelled

**Solutions**:
1. Check server logs with `--trace-mcp`
2. Verify `toolsAllowed`/`toolsDenied` in `.ai-agent.json`
3. Check spelling of server names

### Tool Calls Timing Out

**Problem**: Tool calls consistently fail with timeout.

**Cause**: Tool takes longer than `toolTimeout`.

**Solution**: Increase timeout:
```yaml
---
toolTimeout: 5m   # 5 minutes
---
```

### Large Tool Output Lost

**Problem**: Tool output is replaced with a handle but model doesn't retrieve it.

**Cause**: Output exceeded `toolResponseMaxBytes` and model didn't call `tool_output`.

**Solutions**:
1. Increase threshold:
   ```yaml
   toolResponseMaxBytes: 65536  # 64 KB
   ```
2. Improve prompt to mention large output handling
3. Configure `toolOutput.maxChunks` for better chunking

### Tool Count Too High

**Problem**: Too many tools confuse the model.

**Cause**: MCP servers expose many tools, overwhelming the LLM.

**Solutions**:
1. Use `toolsAllowed` to whitelist needed tools
2. Use `toolsDenied` to blacklist irrelevant tools
3. Split into focused agents with fewer tools each

### REST Tool Authentication Failing

**Problem**: REST tool returns 401/403 errors.

**Cause**: Missing or invalid API key.

**Solutions**:
1. Check API key in environment
2. Verify header configuration in `.ai-agent.json`
3. Test the endpoint manually

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Configuration-MCP-Servers](Configuration-MCP-Servers) - MCP server configuration
- [Configuration-REST-Tools](Configuration-REST-Tools) - REST tool configuration
- [Agent-Files-Behavior](Agent-Files-Behavior) - Tool timeout configuration
- [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents) - Using agents as tools
