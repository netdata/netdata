# Tool Filtering

Control which tools agents can access with allowlists and denylists.

**Note:** Provider-level tool filtering (`providers.<name>.toolsAllowed/toolsDenied`) and agent-level tool filtering (`toolsAllowed/toolsDenied` in frontmatter) are not implemented. Use MCP server-level filtering for tool access control.

---

## Table of Contents

- [Overview](#overview) - Tool access control
- [Tool Naming Convention](#tool-naming-convention) - Understanding tool identifiers
- [Config-Level Filtering](#config-level-filtering) - Filter at MCP server level
- [Wildcard Patterns](#wildcard-patterns) - Pattern matching syntax
- [Common Patterns](#common-patterns) - Read-only, minimal access, safety gates
- [Configuration Reference](#configuration-reference) - All filtering options
- [Debugging](#debugging) - Verify filtering behavior
- [See Also](#see-also) - Related documentation

---

## Overview

Tool filtering provides control over which tools are exposed to agents:

| Layer  | Scope                     | Configuration                           |
| ------ | ------------------------- | --------------------------------------- |
| Config | All agents using a server | `mcpServers.<name>.toolsAllowed/Denied` |

Filtering is applied at tool discovery time. Tools that don't pass filters are never exposed to the LLM.

---

## Tool Naming Convention

Tools follow a consistent naming pattern:

| Source      | Pattern            | Example               |
| ----------- | ------------------ | --------------------- |
| MCP servers | `<server>__<tool>` | `github__search_code` |
| REST tools  | `rest__<name>`     | `rest__weather`       |
| Internal    | `agent__<name>`    | `agent__final_report` |

Understanding this convention is essential for writing filter patterns. When configuring filters in `mcpServers.<name>.toolsAllowed/Denied`, use the **local tool name** only (e.g., `search_code`), not the full `<server>__<tool>` format.

---

## Config-Level Filtering

Filter tools at the MCP server level in `.ai-agent.json`. Use **local tool names** (not the full `mcp__<server>__<tool>` format).

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

| Property       | Type       | Default            | Description                   |
| -------------- | ---------- | ------------------ | ----------------------------- |
| `toolsAllowed` | `string[]` | `[]` (all allowed) | Tools to expose (empty = all) |
| `toolsDenied`  | `string[]` | `[]`               | Tools to hide                 |

---

## Wildcard Patterns

Use literal `*` or `any` to match all tools in filter lists.

### Allow All Tools (Default Behavior)

If `toolsAllowed` is not specified or is empty, all tools are allowed:

```json
{
  "mcpServers": {
    "github": {
      "toolsAllowed": [],
      "toolsDenied": ["create_or_update_file", "delete_repository"]
    }
  }
}
```

Or explicitly allow all:

```json
{
  "mcpServers": {
    "github": {
      "toolsAllowed": ["*"],
      "toolsDenied": ["create_or_update_file"]
    }
  }
}
```

### Deny All Tools

Use `*` or `any` in `toolsDenied` to block all tools from a server:

```json
{
  "mcpServers": {
    "experimental": {
      "toolsDenied": ["*"]
    }
  }
}
```

### Case Insensitivity

Tool names are matched case-insensitively:

```json
{
  "mcpServers": {
    "filesystem": {
      "toolsAllowed": ["read_file", "write_file"]
    }
  }
}
```

This matches `READ_FILE`, `read_file`, or `Read_File`.

### Within a Single Level

If both `toolsAllowed` and `toolsDenied` are specified at the same level:

1. Tool must be in `toolsAllowed` (if `toolsAllowed` is specified with entries)
2. Tool must NOT be in `toolsDenied`

```json
{
  "mcpServers": {
    "filesystem": {
      "toolsAllowed": ["read_file", "write_file"],
      "toolsDenied": ["delete_file", "sensitive_data"]
    }
  }
}
```

Result: `read_file` allowed, `write_file` allowed, `delete_file` denied, `sensitive_data` denied.

**Important:** Only exact tool name matching is supported (case-insensitive). Pattern matching like `query_*` or `search_*` is NOT supported.

---

## String Schema Format Filtering

Some LLM providers don't support certain JSON Schema string formats. Configure this in the provider configuration:

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

See [Providers](Configuration-Providers) for more details on provider configuration.

---

## Common Patterns

### Read-Only Database Access

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

### Minimal Freshdesk Access

```json
{
  "mcpServers": {
    "freshdesk": {
      "type": "stdio",
      "command": "npx -y @anthropic/mcp-server-freshdesk",
      "toolsDenied": [
        "create_ticket",
        "update_ticket",
        "delete_ticket",
        "create_note",
        "update_note",
        "delete_note",
        "create_contact",
        "update_contact",
        "delete_contact",
        "create_company",
        "update_company",
        "delete_company"
      ]
    }
  }
}
```

### Combined Safety (Filtering + Prompt Gates)

For maximum safety, combine tool filtering with prompt safety instructions:

```yaml
---
models:
  - openai/gpt-4o
tools:
  - github
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

```json
{
  "mcpServers": {
    "github": {
      "toolsDenied": [
        "create_or_update_file",
        "push_files",
        "create_issue",
        "update_issue",
        "delete_issue",
        "create_pull_request"
      ]
    }
  }
}
```

---

## Configuration Reference

### Complete Filtering Schema

```json
{
  "providers": {
    "<name>": {
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
---
```

The `tools` array controls which tool sources are loaded (servers, `restTools`, agents). It does NOT support per-tool filtering.

### All Filtering Properties

| Location    | Property                     | Type       | Description                                        |
| ----------- | ---------------------------- | ---------- | -------------------------------------------------- |
| Provider    | `stringSchemaFormatsAllowed` | `string[]` | JSON Schema formats to allow                       |
| Provider    | `stringSchemaFormatsDenied`  | `string[]` | JSON Schema formats to strip                       |
| MCP Server  | `toolsAllowed`               | `string[]` | Tools to expose (local names)                      |
| MCP Server  | `toolsDenied`                | `string[]` | Tools to hide (local names)                        |
| Frontmatter | `tools`                      | `string[]` | Tool sources to include (servers, restTools, etc.) |

**Important:**

- Provider-level `toolsAllowed` and `toolsDenied` are defined in the configuration schema but **not implemented**.
- Frontmatter `toolsAllowed` and `toolsDenied` are **not supported**.
- Use MCP server-level filtering for tool access control.
- Provider `stringSchemaFormatsAllowed/Denied` filters JSON Schema string formats, not tools.

---

## Debugging

### List Available Tools

```bash
ai-agent --list-tools <server>
```

List tools from a specific MCP server (use `all` to list all servers).

### Trace Tool Discovery

```bash
ai-agent --agent test.ai --trace-mcp
```

Enables detailed MCP protocol logging to see:

- Tools discovered from each MCP server
- Filtering decisions at server level
- Final tool list exposed to LLM

### Verify Specific Tool

```bash
ai-agent --list-tools github 2>&1 | grep "search_code"
```

If the tool doesn't appear, it was filtered out.

### Common Issues

**Tool not available:**

- Check tool name spelling (use exact name from server)
- Verify server is in agent's `tools:` list

**Wrong tool name pattern:**

- MCP tools use local names in server config: `search_code`
- Only exact string matching is supported (case-insensitive)

**Wildcard not matching:**

- Only literal `*` or `any` wildcard is supported
- Pattern matching like `server__*` or `prefix_*` is NOT supported
- Use `*` or `any` alone to match all tools, not for prefix/partial matching

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [MCP Servers](Configuration-MCP-Servers) - MCP server configuration
- [Providers](Configuration-Providers) - Provider configuration
- [Frontmatter](Agent-Development-Frontmatter) - Agent frontmatter reference
- [Safety Gates](Agent-Development-Safety) - Prompt-level safety patterns
