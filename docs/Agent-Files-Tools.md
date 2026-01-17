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
  - brave # MCP server: web search
  - fetcher # MCP server: URL fetching
  - catalog # REST tool: product catalog (from restTools config)
---
```

---

## Configuration Reference

### tools

| Property     | Value                                                  |
| ------------ | ------------------------------------------------------ |
| Type         | `string` or `string[]`                                 |
| Default      | `[]` (no tools)                                        |
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
  - catalog         # REST tool (from restTools config)
  - openapi:github  # All tools from OpenAPI spec
---
```

**Tool Naming**:

Internally, ai-agent uses the format `namespace__toolname` (double underscore) for all tools. In frontmatter, you reference tools by their config key name:

| Source Type | Frontmatter Format              | Internal Format              | Example                           |
| ----------- | ------------------------------- | ---------------------------- | --------------------------------- |
| MCP Server  | `<server-name>`                 | `<server-name>__<tool>`      | `github` → `github__search_code`  |
| REST Tool   | `<tool-name>`                   | `rest__<tool-name>`          | `catalog` → `rest__catalog`       |
| OpenAPI     | `openapi:<provider-name>`       | `openapi__<provider>__<op>`  | `openapi:github`                  |

**Notes**:

- MCP server names in `.ai-agent.json` become the namespace prefix
- Same MCP server can be configured multiple times with different namespaces (see [Configuration-MCP-Servers](Configuration-MCP-Servers#multiple-namespaces-for-same-server))
- Listing a namespace exposes ALL its tools by default
- Use `toolsAllowed`/`toolsDenied` in server config to filter specific tools

**Verify Available Tools**:

Use `--list-tools` with the namespace (as used in frontmatter) to see tools visible to the model:

```bash
ai-agent --list-tools github
ai-agent --list-tools github-readonly
ai-agent --list-tools all  # List all tools from all servers
```

**Verify Agent Configuration**:

Run an agent with `--help` to see its flattened frontmatter (resolved values from frontmatter + config defaults) and config files resolution order:

```bash
./myagent.ai --help
```

---

### toolsAllowed

| Property | Value                                         |
| -------- | --------------------------------------------- |
| Type     | Configured per MCP server in `.ai-agent.json` |
| Default  | All tools allowed                             |

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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/allowed/path"],
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

| Property | Value                                         |
| -------- | --------------------------------------------- |
| Type     | Configured per MCP server in `.ai-agent.json` |
| Default  | No tools denied                               |

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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/allowed/path"],
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

| Property     | Value           |
| ------------ | --------------- |
| Type         | `integer`       |
| Default      | `12288` (12 KB) |
| Valid values | `0` or greater  |

**Description**: Maximum tool output size kept in conversation. Larger outputs are stored separately and replaced with a `tool_output` handle.

**What it affects**:

- How large tool outputs are handled
- Context window usage
- Whether `tool_output` tool is invoked for retrieval

**Example**:

```yaml
---
toolResponseMaxBytes: 24576 # 24 KB before storing
---
---
toolResponseMaxBytes: 8192 # 8 KB (more aggressive)
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

| Property | Value                |
| -------- | -------------------- |
| Type     | `object`             |
| Default  | Uses global defaults |

**Description**: Overrides for the `tool_output` module behavior (chunking large outputs).

**Sub-keys**:

| Sub-key                 | Type                   | Default | Description                         |
| ----------------------- | ---------------------- | ------- | ----------------------------------- |
| `enabled`               | `boolean`              | `true`  | Enable/disable tool output chunking |
| `maxChunks`             | `number`               | Varies  | Maximum chunks for large outputs    |
| `overlapPercent`        | `number`               | Varies  | Overlap between chunks (%)          |
| `avgLineBytesThreshold` | `number`               | Varies  | Threshold for line-based chunking   |
| `models`                | `string` or `string[]` | None    | Models for extraction (optional)    |

**Example**:

```yaml
---
toolOutput:
  enabled: true
  maxChunks: 1
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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/path/to/dir"]
    },
    "brave": {
      "command": "npx",
      "args": ["-y", "@brave/brave-search-mcp-server", "--transport", "stdio"],
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

| Server                                        | Purpose           |
| --------------------------------------------- | ----------------- |
| `/opt/ai-agent/mcp/fs/fs-mcp-server.js` (built-in) | File operations   |
| `@brave/brave-search-mcp-server`              | Web search        |
| `https://mcp.jina.ai/v1` (HTTP)               | URL fetching      |
| `@modelcontextprotocol/server-github`         | GitHub operations |

### REST Tools

REST tools are HTTP APIs configured in `.ai-agent.json`. In frontmatter, use the config key name; internally they are exposed with the `rest__` prefix.

**Configuration** (`.ai-agent.json`):

```json
{
  "restTools": {
    "catalog": {
      "url": "/path/to/local/api.json",
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
  - catalog
---
```

**Usage in agent**:

```yaml
---
tools:
  - catalog
---
```

### OpenAPI Tools

OpenAPI tools import all operations from an OpenAPI specification.

**Configuration** (`.ai-agent.json`):

```json
{
  "openapi": {
    "github": {
      "spec": "/path/to/local/openapi.json",
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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/data"],
      "toolsAllowed": ["read_file", "list_directory", "search_files"]
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
      "command": "node",
      "args": ["/opt/ai-agent/mcp/fs/fs-mcp-server.js", "/data"],
      "toolsDenied": ["delete_file", "move_file", "write_file"]
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
  - filesystem # Configured with toolsAllowed: [read_file, list_directory]
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
  - brave # Web search
  - fetcher # URL fetching
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
  - filesystem # MCP: file operations
  - brave # MCP: web search
  - catalog # REST: product catalog
  - openapi:github # OpenAPI: GitHub operations
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
toolResponseMaxBytes: 32768 # 32 KB
toolOutput:
  enabled: true
  maxChunks: 10
  models: openai/gpt-4o-mini # Cheap model for extraction
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
toolTimeout: 5m # 5 minutes
---
```

### Large Tool Output Lost

**Problem**: Tool output is replaced with a handle but model doesn't retrieve it.

**Cause**: Output exceeded `toolResponseMaxBytes` and model didn't call `tool_output`.

**Solutions**:

1. Increase threshold:
   ```yaml
   toolResponseMaxBytes: 65536 # 64 KB
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
