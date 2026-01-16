# Tool Filtering

Control which tools agents can access with allowlists and denylists.

---

## Overview

Tool filtering provides defense-in-depth:

1. **Config level**: Filter tools available to all agents
2. **Agent level**: Filter tools for specific agents
3. **Prompt level**: Safety gates in prompts

---

## Config-Level Filtering

### MCP Server Filtering

Allow only specific tools from an MCP server:

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

Deny specific tools (allow all others):

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

### Precedence

If both `toolsAllowed` and `toolsDenied` are specified:
1. Tool must be in `toolsAllowed`
2. Tool must NOT be in `toolsDenied`

---

## Agent-Level Filtering

Filter tools in agent frontmatter:

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
```

### Tool Naming

Tools follow this naming convention:
- MCP tools: `mcp__servername__toolname`
- REST tools: `rest__toolname`
- Internal tools: `agent__toolname`

---

## Provider-Level Filtering

Filter tools by provider capabilities:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "toolsAllowed": ["mcp__*", "rest__*"],
      "toolsDenied": ["agent__batch"]
    }
  }
}
```

### String Schema Format Filtering

Some providers don't support certain JSON Schema formats:

```json
{
  "providers": {
    "ollama": {
      "type": "ollama",
      "stringSchemaFormatsAllowed": ["date", "time"],
      "stringSchemaFormatsDenied": ["uri", "email"]
    }
  }
}
```

---

## Wildcards

Use `*` for pattern matching:

```yaml
toolsAllowed:
  - mcp__github__*        # All GitHub tools
  - mcp__filesystem__read_*  # All read operations
```

```yaml
toolsDenied:
  - mcp__*__delete_*      # All delete operations from any MCP
  - mcp__*__write_*       # All write operations
```

---

## Read-Only Pattern

Create read-only access to a server:

### Config

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

### Agent

```yaml
---
tools:
  - database
toolsAllowed:
  - mcp__database__select
  - mcp__database__describe
---
You have read-only database access.
```

---

## Real-World Example

From production (Freshdesk agent):

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

---

## Combining with Safety Gates

For maximum safety, combine tool filtering with prompt safety gates:

```yaml
---
tools:
  - github
toolsDenied:
  - mcp__github__push_files
  - mcp__github__create_or_update_file
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

---

## Debugging

### List Available Tools

```bash
ai-agent --agent test.ai --dry-run --verbose
```

Shows which tools are available after filtering.

### Trace Tool Calls

```bash
ai-agent --agent test.ai --trace-mcp "test query"
```

Shows tool discovery and filtering decisions.

---

## See Also

- [MCP Tools](Configuration-MCP-Tools) - MCP configuration
- [Safety Gates](Agent-Development-Safety) - Prompt-level safety
