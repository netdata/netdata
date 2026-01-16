# Tool Filtering

Control which tools agents can access with allowlists and denylists.

---

## Table of Contents

- [Overview](#overview) - Defense-in-depth tool access control
- [Tool Naming Convention](#tool-naming-convention) - Understanding tool identifiers
- [Config-Level Filtering](#config-level-filtering) - Filter at MCP server level
- [Provider-Level Filtering](#provider-level-filtering) - Filter by provider capabilities
- [Agent-Level Filtering](#agent-level-filtering) - Filter in agent frontmatter
- [Wildcard Patterns](#wildcard-patterns) - Pattern matching syntax
- [Precedence Rules](#precedence-rules) - How filtering layers combine
- [Common Patterns](#common-patterns) - Read-only, minimal access, safety gates
- [Configuration Reference](#configuration-reference) - All filtering options
- [Debugging](#debugging) - Verify filtering behavior
- [See Also](#see-also) - Related documentation

---

## Overview

Tool filtering provides defense-in-depth access control:

| Layer | Scope | Configuration |
|-------|-------|---------------|
| Config | All agents using a server | `mcpServers.<name>.toolsAllowed/Denied` |
| Provider | All agents using a provider | `providers.<name>.toolsAllowed/Denied` |
| Agent | Single agent | Frontmatter `toolsAllowed/toolsDenied` |

Filtering is applied at tool discovery time. Tools that don't pass filters are never exposed to the LLM.

---

## Tool Naming Convention

Tools follow a consistent naming pattern:

| Source | Pattern | Example |
|--------|---------|---------|
| MCP servers | `mcp__<server>__<tool>` | `mcp__github__search_code` |
| REST tools | `rest__<name>` | `rest__weather` |
| Internal | `agent__<name>` | `agent__final_report` |

Understanding this convention is essential for writing filter patterns.

---

## Config-Level Filtering

Filter tools at the MCP server level in `.ai-agent.json`.

### Allow Specific Tools

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsAllowed": [
        "search_code",
        "get_file_contents",
        "list_issues",
        "get_issue"
      ]
    }
  }
}
```

Only listed tools are available. All others are hidden.

### Deny Specific Tools

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-github"],
      "toolsDenied": [
        "create_or_update_file",
        "push_files",
        "create_issue",
        "delete_branch",
        "create_pull_request"
      ]
    }
  }
}
```

All tools are available except those listed.

### MCP Server Filtering Reference

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `toolsAllowed` | `string[]` | `[]` (all allowed) | Tools to expose (empty = all) |
| `toolsDenied` | `string[]` | `[]` | Tools to hide |

---

## Provider-Level Filtering

Filter tools available to all agents using a specific provider.

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "toolsAllowed": ["mcp__*", "rest__*"],
      "toolsDenied": ["agent__batch"]
    }
  }
}
```

### String Schema Format Filtering

Some providers don't support certain JSON Schema string formats. Filter them:

```json
{
  "providers": {
    "ollama": {
      "type": "ollama",
      "baseUrl": "http://localhost:11434",
      "stringSchemaFormatsAllowed": ["date", "time"],
      "stringSchemaFormatsDenied": ["uri", "email", "uuid"]
    }
  }
}
```

### Provider Filtering Reference

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `toolsAllowed` | `string[]` | `[]` (all allowed) | Tools to expose |
| `toolsDenied` | `string[]` | `[]` | Tools to hide |
| `stringSchemaFormatsAllowed` | `string[]` | `[]` (all allowed) | JSON Schema formats to allow |
| `stringSchemaFormatsDenied` | `string[]` | `[]` | JSON Schema formats to strip |

---

## Agent-Level Filtering

Filter tools in agent frontmatter for per-agent control.

```yaml
---
models:
  - openai/gpt-4o
tools:
  - github
  - filesystem
toolsAllowed:
  - mcp__github__search_code
  - mcp__github__get_file_contents
  - mcp__filesystem__read_file
toolsDenied:
  - mcp__filesystem__write_file
  - mcp__filesystem__delete_file
---
You are a code review assistant with read-only access.
```

### Frontmatter Filtering Reference

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `tools` | `string[]` | `[]` | Tool sources to include (servers, `restTools`, `agents`) |
| `toolsAllowed` | `string[]` | `[]` (all allowed) | Specific tools to expose |
| `toolsDenied` | `string[]` | `[]` | Specific tools to hide |

---

## Wildcard Patterns

Use `*` for pattern matching in filter lists.

### Match All Tools from a Server

```yaml
toolsAllowed:
  - mcp__github__*        # All GitHub tools
  - mcp__filesystem__*    # All filesystem tools
```

### Match Tool Prefixes

```yaml
toolsAllowed:
  - mcp__filesystem__read_*   # read_file, read_dir, etc.
  - mcp__github__get_*        # get_file_contents, get_issue, etc.
```

### Match Across Servers

```yaml
toolsDenied:
  - mcp__*__delete_*      # All delete operations from any MCP
  - mcp__*__write_*       # All write operations
  - mcp__*__create_*      # All create operations
```

### Wildcard Rules

| Pattern | Matches |
|---------|---------|
| `*` | Any characters (including none) |
| `mcp__*` | All MCP tools |
| `mcp__github__*` | All tools from `github` server |
| `*__search_*` | Any tool containing `search_` |

---

## Precedence Rules

When multiple filter levels apply, they combine with AND logic:

### Evaluation Order

1. **Config level** (MCP server filters) - Applied first
2. **Provider level** - Applied second
3. **Agent level** - Applied last

### Combined Filter Logic

```
tool_visible = (passes_config_filters) AND (passes_provider_filters) AND (passes_agent_filters)
```

### Within a Single Level

If both `toolsAllowed` and `toolsDenied` are specified at the same level:

1. Tool must be in `toolsAllowed` (or `toolsAllowed` is empty)
2. Tool must NOT be in `toolsDenied`

```json
{
  "mcpServers": {
    "api": {
      "toolsAllowed": ["query_*", "search_*"],
      "toolsDenied": ["query_sensitive"]
    }
  }
}
```

Result: `query_users` allowed, `query_sensitive` denied, `delete_user` denied.

---

## Common Patterns

### Read-Only Database Access

**Config level:**

```json
{
  "mcpServers": {
    "database": {
      "type": "stdio",
      "command": "./db-mcp",
      "toolsDenied": [
        "insert",
        "update",
        "delete",
        "drop",
        "truncate",
        "create"
      ]
    }
  }
}
```

**Agent level:**

```yaml
---
models:
  - openai/gpt-4o
tools:
  - database
toolsAllowed:
  - mcp__database__select
  - mcp__database__describe
---
You have read-only database access. Query data but never modify it.
```

### Minimal Freshdesk Access

```yaml
---
description: Freshdesk ticket lookup (read-only)
models:
  - openai/gpt-4o
tools:
  - freshdesk
toolsDenied:
  - mcp__freshdesk__create_ticket
  - mcp__freshdesk__update_ticket
  - mcp__freshdesk__delete_ticket
  - mcp__freshdesk__create_note
  - mcp__freshdesk__update_note
  - mcp__freshdesk__delete_note
  - mcp__freshdesk__create_contact
  - mcp__freshdesk__update_contact
  - mcp__freshdesk__delete_contact
  - mcp__freshdesk__create_company
  - mcp__freshdesk__update_company
  - mcp__freshdesk__delete_company
---
```

### Combined Safety (Filtering + Prompt Gates)

For maximum safety, combine tool filtering with prompt safety instructions:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - github
toolsDenied:
  - mcp__github__push_files
  - mcp__github__create_or_update_file
  - mcp__github__create_pull_request
---
You are a code review assistant.

## Safety Gate (Mandatory)

You can ONLY:
- Search code
- Read files
- List and read issues/PRs

You CANNOT and MUST NOT:
- Create or modify files
- Push commits
- Create issues or PRs

If asked to write code, provide it in your response but DO NOT commit it.
```

### Deny All Write Operations

```yaml
toolsDenied:
  - mcp__*__create_*
  - mcp__*__update_*
  - mcp__*__delete_*
  - mcp__*__write_*
  - mcp__*__push_*
  - mcp__*__insert_*
  - mcp__*__drop_*
```

---

## Configuration Reference

### Complete Filtering Schema

```json
{
  "providers": {
    "<name>": {
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"],
      "stringSchemaFormatsAllowed": ["string"],
      "stringSchemaFormatsDenied": ["string"]
    }
  },
  "mcpServers": {
    "<name>": {
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"]
    }
  }
}
```

### Frontmatter Schema

```yaml
---
tools: ["string"]
toolsAllowed: ["string"]
toolsDenied: ["string"]
---
```

### All Filtering Properties

| Location | Property | Type | Description |
|----------|----------|------|-------------|
| Provider | `toolsAllowed` | `string[]` | Tools to expose |
| Provider | `toolsDenied` | `string[]` | Tools to hide |
| Provider | `stringSchemaFormatsAllowed` | `string[]` | JSON Schema formats to allow |
| Provider | `stringSchemaFormatsDenied` | `string[]` | JSON Schema formats to strip |
| MCP Server | `toolsAllowed` | `string[]` | Tools to expose (local names) |
| MCP Server | `toolsDenied` | `string[]` | Tools to hide (local names) |
| Frontmatter | `tools` | `string[]` | Tool sources to include |
| Frontmatter | `toolsAllowed` | `string[]` | Tools to expose (full names) |
| Frontmatter | `toolsDenied` | `string[]` | Tools to hide (full names) |

---

## Debugging

### List Available Tools

```bash
ai-agent --agent test.ai --dry-run --verbose
```

Shows which tools are available after all filtering layers are applied.

### Trace Tool Discovery

```bash
ai-agent --agent test.ai --trace-mcp "test query"
```

Shows:
- Tools discovered from each MCP server
- Filtering decisions at each layer
- Final tool list exposed to LLM

### Verify Specific Tool

```bash
ai-agent --agent test.ai --dry-run --verbose 2>&1 | grep "mcp__github__push"
```

If the tool doesn't appear, it was filtered out.

### Common Issues

**Tool not available:**
- Check tool name spelling (use exact name from server)
- Verify server is in `tools:` list
- Check all filtering layers (config, provider, agent)

**Wrong tool name pattern:**
- MCP tools use local names in server config: `search_code`
- MCP tools use full names in agent frontmatter: `mcp__github__search_code`

**Wildcard not matching:**
- Wildcards match only at the position specified
- `mcp__*__search` does not match `mcp__github__search_code`
- Use `mcp__*__search_*` for prefix matching

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [MCP Servers](Configuration-MCP-Servers) - MCP server configuration
- [Providers](Configuration-Providers) - Provider configuration
- [Frontmatter](Agent-Development-Frontmatter) - Agent frontmatter reference
- [Safety Gates](Agent-Development-Safety) - Prompt-level safety patterns
