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

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | `12288` (12 KB) |
| Valid values | `1024` to `1000000` |
| Required | No |

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
  enabled: true        # Enable/disable storage (default: true)
  maxChunks: 10        # Max chunks per extraction
  overlapPercent: 5    # Chunk overlap percentage
---
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | boolean | `true` | Enable tool output storage |
| `maxChunks` | number | `10` | Maximum chunks for extraction |
| `overlapPercent` | number | `5` | Overlap between chunks (0-20) |

---

## How It Works

### Storage Triggers

Tool output storage triggers when any of these conditions are met:

| Trigger | Condition | Description |
|---------|-----------|-------------|
| `size_cap` | Response exceeds `toolResponseMaxBytes` | Hard size limit |
| `token_budget` | Response would overflow context window | Dynamic budget |
| `reserve_failed` | Cannot reserve tokens for response | Budget exhausted |

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
Tool response stored.
Handle: session-abc123/file-xyz789

Response metadata:
- Original size: 45678 bytes
- Estimated tokens: ~8900
- Line count: 1234

To extract specific content, call tool_output with:
- handle: "session-abc123/file-xyz789"
- extract: Choose one of:
  - "lines 1-100" (specific line range)
  - "first 50 lines" (from start)
  - "last 50 lines" (from end)
  - "grep pattern" (matching lines)
  - "summary" (AI-generated overview)
```

The LLM can then decide what content to extract based on the task.

---

## Extraction Tool

The `tool_output` tool allows targeted content retrieval:

### Tool Schema

```json
{
  "name": "tool_output",
  "description": "Extract content from a stored tool response",
  "parameters": {
    "handle": {
      "type": "string",
      "description": "The handle from the stored response message"
    },
    "extract": {
      "type": "string",
      "description": "What to extract from the response"
    }
  }
}
```

### Extract Options

| Extract Command | Description | Example |
|-----------------|-------------|---------|
| `lines N-M` | Specific line range | `lines 1-100` |
| `first N lines` | First N lines | `first 50 lines` |
| `last N lines` | Last N lines | `last 50 lines` |
| `grep PATTERN` | Lines matching regex | `grep "function.*export"` |
| `summary` | AI-generated summary | `summary` |

### Example Usage

**LLM request**:
```json
{
  "tool": "tool_output",
  "arguments": {
    "handle": "session-abc123/file-xyz789",
    "extract": "lines 1-100"
  }
}
```

**Response**:
```
[Lines 1-100 of 1234]

function search(query: string) {
  // ... first 100 lines of content
}

[Showing lines 1-100. Use tool_output to get more lines.]
```

---

## Storage Location

Tool output files are stored in a temporary directory:

```
/tmp/ai-agent-<run-hash>/
└── session-<uuid>/
    └── <file-uuid>
```

| Component | Description |
|-----------|-------------|
| `run-hash` | Hash of process start time |
| `session-uuid` | Session transaction ID |
| `file-uuid` | Unique file identifier |

**Lifecycle**:
- Created at process start
- Cleaned up on process exit
- Survives session end within same process (for headend mode)

---

## Logging and Accounting

### Warning Log

Storage events are logged:

```
[WRN] Tool output stored: handle=session-abc/file-xyz, reason=size_cap, bytes=45678, lines=1234, tokens=~8900
```

### Accounting Entry

Tool accounting includes storage info:

```json
{
  "type": "tool",
  "tool": "search_code",
  "status": "ok",
  "stored": true,
  "handle": "session-abc/file-xyz",
  "reason": "size_cap",
  "originalBytes": 45678,
  "charactersOut": 256
}
```

| Field | Description |
|-------|-------------|
| `stored` | Whether response was stored |
| `handle` | Storage handle |
| `reason` | Trigger reason |
| `originalBytes` | Original response size |
| `charactersOut` | Handle message size sent to LLM |

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
- If stored, use grep to find relevant patterns first
- Then extract specific line ranges
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
2. Guide LLM to extract larger chunks
3. Use `summary` extraction first to understand content

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
