# Tool Output Handling

Handle oversized tool responses with automatic storage and extraction.

---

## Table of Contents

- [Overview](#overview) - How large responses are handled
- [Configuration](#configuration) - Size limits and options
- [How It Works](#how-it-works) - Storage and retrieval flow
- [Handle Messages](#handle-messages) - Format of stored response references
- [Extraction Tool](#extraction-tool) - The tool_output tool
- [Storage Location](#storage-location) - Where files are stored
- [Logging and Accounting](#logging-and-accounting) - Tracking stored responses
- [Best Practices](#best-practices) - Recommended configurations
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related documentation

---

## Overview

When tool responses exceed configured size limits, AI Agent:

1. **Stores** the full response on disk
2. **Replaces** it with a handle message in the conversation
3. **Provides** a `tool_output` tool for the LLM to extract specific content

This prevents context window overflow while preserving all data for retrieval.

**Why this matters**:

- Large tool responses (code search results, API data) can consume most of the context window
- Token limits force truncation, losing important information
- Chunked extraction lets the LLM retrieve only what it needs

---

## Configuration

### Size Limit (toolResponseMaxBytes)

The threshold for triggering storage:

| Property     | Value                               |
| ------------ | ----------------------------------- |
| Type         | `number`                            |
| Default      | `12288` (12 KB)                     |
| Valid values | `0` (no lower bound) to `1,000,000` |
| Required     | No                                  |

**Frontmatter**:

```yaml
---
toolResponseMaxBytes: 12288
---
```

**CLI override**:

```bash
ai-agent --agent test.ai --tool-response-max-bytes 25000 "query"
```

### Tool Output Options

Fine-tune extraction behavior:

```yaml
---
toolOutput:
  enabled: true # Enable/disable storage (default: true)
  maxChunks: 1 # Max chunks per extraction
  overlapPercent: 10 # Chunk overlap percentage (0-50)
  avgLineBytesThreshold: 1000 # Threshold for choosing full-chunked mode
  models: ["openai/gpt-4o"] # Target models for extraction
---
```

| Option                  | Type             | Default   | Description                                     |
| ----------------------- | ---------------- | --------- | ----------------------------------------------- |
| `enabled`               | boolean          | `true`    | Enable tool output storage                      |
| `maxChunks`             | number           | `1`       | Maximum chunks for extraction                   |
| `overlapPercent`        | number           | `10`      | Overlap between chunks (0-50)                   |
| `avgLineBytesThreshold` | number           | `1000`    | Threshold for choosing full-chunked mode        |
| `models`                | string\|string[] | undefined | Target models for extraction (provider/modelId) |

**Note**: The `storeDir` option is accepted but ignored. Storage root is always `/tmp/ai-agent-<run-hash>`.

---

## How It Works

### Storage Triggers

Tool output storage triggers when any of these conditions are met:

| Trigger          | Condition                               | Description      |
| ---------------- | --------------------------------------- | ---------------- |
| `size_cap`       | Response exceeds `toolResponseMaxBytes` | Hard size limit  |
| `token_budget`   | Response would overflow context window  | Dynamic budget   |
| `reserve_failed` | Cannot reserve tokens for response      | Budget exhausted |

### Flow

```
Tool Response (50KB)
       ↓
Check Size: 50KB > 12KB limit
       ↓
Store to disk: /tmp/ai-agent-xxx/session-yyy/file-zzz
       ↓
Replace in conversation:
  "Tool response stored. Handle: session-yyy/file-zzz
   To extract, call tool_output..."
       ↓
LLM calls tool_output to retrieve content
```

---

## Handle Messages

When a response is stored, the LLM receives a handle message:

```
Tool output is too large (45678 bytes, 1234 lines, 8900 tokens).
Call tool_output(handle = "session-abc123/file-xyz789", extract = "what to extract").
The handle is a relative path under the tool_output root.
Provide precise and detailed instructions in `extract` about what you are looking for.
```

The LLM should provide specific, detailed instructions in the `extract` parameter describing exactly what content it needs from the stored output.

---

## Extraction Tool

The `tool_output` tool allows targeted content retrieval:

### Tool Schema

```json
{
  "name": "tool_output",
  "description": "Extract information from a stored oversized tool output by handle",
  "parameters": {
    "handle": {
      "type": "string",
      "description": "Handle of the stored tool output (relative path under the tool_output root, e.g. session-<uuid>/<file-uuid>; provided in the tool-result message)"
    },
    "extract": {
      "type": "string",
      "description": "Provide precise, detailed instructions about what you need from the stored output (be specific, include keys/fields/sections if known)"
    },
    "mode": {
      "type": "string",
      "enum": ["auto", "full-chunked", "read-grep", "truncate"],
      "description": "Optional override. auto=module decides; full-chunked=LLM chunk+reduce; read-grep=dynamic sub-agent with Read/Grep; truncate=keeps top and bottom, truncates in the middle"
    }
  }
}
```

### Extract Parameter

The `extract` parameter accepts free-form natural language instructions. Provide precise, detailed instructions about what content you need from the stored output, including specific keys, fields, sections, or patterns if known.

**Examples**:

- "Extract all function definitions"
- "Find lines containing 'export default' and show 10 lines of context around each"
- "List all error messages with their line numbers"
- "Get the configuration section and summarize its values"
- "Find all occurrences of pattern 'TODO' or 'FIXME'"

### Example Usage

**LLM request**:

```json
{
  "tool": "tool_output",
  "arguments": {
    "handle": "session-abc123/file-xyz789",
    "extract": "Extract all function definitions with their parameter types"
  }
}
```

**Response**:

```
ABSTRACT FROM TOOL OUTPUT tool_name WITH HANDLE session-abc123/file-xyz789, STRATEGY:full-chunked:

function search(query: string): SearchResult {
  // implementation...
}

function processData(input: Data): void {
  // implementation...
}
```

---

## Storage Location

Tool output files are stored in a temporary directory:

```
/tmp/ai-agent-<run-hash>/
└── session-<uuid>/
    └── <file-uuid>
```

| Component      | Description                                                                |
| -------------- | -------------------------------------------------------------------------- |
| `run-hash`     | 12-character hash of process identifier (PID), start time, and random UUID |
| `session-uuid` | Session transaction ID                                                     |
| `file-uuid`    | Unique file identifier                                                     |

**Lifecycle**:

- Created at process start
- Cleaned up on process exit
- Survives session end within same process (for headend mode)

---

## Logging and Accounting

### Warning Log

Storage events are logged:

```
[WRN] Tool 'tool_name' output stored for tool_output (size_cap).
```

**Log details** include:

- `tool`: Composed tool name (e.g., `mcp:filesystem/read_file`)
- `tool_namespace`: Tool namespace (e.g., `filesystem`)
- `provider`: Provider label (e.g., `mcp:filesystem`)
- `tool_kind`: Tool kind (`mcp`, `rest`, `agent`, etc.)
- `handle`: Storage handle (e.g. `session-abc/file-xyz`)
- `reason`: Trigger reason (`size_cap`, `token_budget`, or `reserve_failed`)
- `bytes`: Original response size in bytes
- `lines`: Line count
- `tokens`: Estimated token count
- `tool_output`: Always `true` for stored outputs

### Accounting Entry

Tool accounting includes storage info:

```json
{
  "type": "tool",
  "timestamp": 1737012800000,
  "status": "ok",
  "latency": 150,
  "mcpServer": "filesystem",
  "command": "read_file",
  "charactersIn": 256,
  "charactersOut": 345
}
```

**Note**: When a response is stored, `charactersOut` is the length of the handle message (not the original output). `mcpServer` is the server namespace (not including the `mcp:` prefix).

**Tool response details** (in `details` object) include:

- `tool_output_handle`: Storage handle (e.g. `session-abc/file-xyz`)
- `tool_output_reason`: Trigger reason (`size_cap`, `token_budget`, or `reserve_failed`)
- `truncated`: `true` when output was stored

| Field                | Description                                        |
| -------------------- | -------------------------------------------------- |
| `tool_output_handle` | Storage handle (when stored)                       |
| `tool_output_reason` | Trigger reason (when stored)                       |
| `truncated`          | Whether response was stored (replaced with handle) |
| `charactersIn`       | Tool request size (parameters)                     |
| `charactersOut`      | Response size (handle message or actual output)    |

---

## Best Practices

### Research Agents

Higher limits for agents that need comprehensive context:

```yaml
---
models:
  - openai/gpt-4o
toolResponseMaxBytes: 50000
maxTurns: 20
contextWindow: 128000
---
Research agents need full context. Allow large responses and more turns.
```

### API Agents

Lower limits for quick, focused responses:

```yaml
---
models:
  - openai/gpt-4o-mini
toolResponseMaxBytes: 8192
maxTurns: 10
---
API agents prioritize speed. Store large responses early.
```

### Chunked Extraction Pattern

Guide the LLM to process large responses incrementally:

```yaml
---
models:
  - openai/gpt-4o
toolResponseMaxBytes: 10000
---
When tool output is stored:
1. First, extract a summary to understand the content
2. Then extract specific sections as needed
3. Process in chunks rather than retrieving everything at once
```

### Code Search Pattern

```yaml
---
models:
  - openai/gpt-4o
toolResponseMaxBytes: 15000
---
For code search results:
  - If stored, ask for pattern matching first
  - Then extract specific code sections with context
  - Focus on the most relevant matches
```

---

## Troubleshooting

### Problem: Response stored but LLM doesn't extract

**Cause**: LLM not instructed to use tool_output.

**Solution**: Add explicit guidance in system prompt:

```yaml
---
When you see "Tool response stored", use the tool_output tool to extract
the content you need. Start with a summary or grep for relevant patterns.
---
```

---

### Problem: Extraction returns "handle not found"

**Cause**: Handle expired (process restarted) or typo.

**Solution**:

1. Check handle format matches exactly
2. For long-running sessions, handles may expire on process restart
3. Verify storage directory exists: `ls /tmp/ai-agent-*/`

---

### Problem: Too many tool_output calls

**Cause**: LLM extracting small chunks repeatedly.

**Solution**:

1. Increase `toolResponseMaxBytes` for this agent
2. Guide LLM to extract more content in each call by using detailed instructions
3. Ask LLM to summarize sections first to understand what's available

---

### Problem: Context still overflowing

**Cause**: Even handle messages can accumulate.

**Solution**:

1. Lower `toolResponseMaxBytes` further
2. Reduce `maxTurns` to limit accumulation
3. Use more aggressive context management

---

### Problem: Original content lost

**Cause**: Process exited before extraction.

**Solution**: Original content is preserved in session snapshots:

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | {name: .attributes.name, response: .response}'
```

Note: Snapshots capture the handle, not the full stored content. For full recovery, extract from the storage directory before process exit.

---

## See Also

- [Operations](Operations) - Operations overview
- [Session Snapshots](Operations-Snapshots) - Snapshot extraction
- [Agent-Files-Behavior](Agent-Files-Behavior) - Context configuration
- [specs/tools-overview.md](specs/tools-overview.md) - Tool system specification
