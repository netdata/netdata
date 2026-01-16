# Tool Output Handles

Handle oversized tool responses.

---

## Overview

When tool responses exceed size limits, AI Agent:
1. Stores the full response on disk
2. Replaces it with a handle message
3. Provides `tool_output` tool for extraction

---

## Configuration

### Size Limit

```yaml
# Frontmatter
toolResponseMaxBytes: 12288  # 12 KB (default)
```

```bash
# CLI
ai-agent --agent test.ai --tool-response-max-bytes 25000 "query"
```

### Tool Output Options

```yaml
# Frontmatter
toolOutput:
  enabled: true
  maxChunks: 10
  overlapPercent: 5
```

---

## Storage Location

```
/tmp/ai-agent-<run-hash>/
└── session-<uuid>/
    └── <file-uuid>
```

- Created at process start
- Cleaned up on exit

---

## Handle Message

When a response is stored:

```json
{
  "content": "Tool response stored. Handle: session-abc/file-xyz\n\nTo extract, call tool_output with:\n- handle: session-abc/file-xyz\n- extract: lines 1-100 (or specific query)"
}
```

---

## Extraction Tool

The `tool_output` tool allows the LLM to retrieve stored content:

```json
{
  "tool": "tool_output",
  "arguments": {
    "handle": "session-abc/file-xyz",
    "extract": "lines 1-100"
  }
}
```

### Extract Options

| Extract | Description |
|---------|-------------|
| `lines 1-100` | Specific line range |
| `first 50 lines` | First N lines |
| `last 50 lines` | Last N lines |
| `grep pattern` | Lines matching pattern |
| `summary` | AI-generated summary |

---

## Triggers

Tool output storage triggers when:

| Trigger | Condition |
|---------|-----------|
| `size_cap` | Response exceeds `toolResponseMaxBytes` |
| `token_budget` | Response would overflow context window |
| `reserve_failed` | Cannot reserve tokens for response |

---

## Logging

Storage events are logged:

```
[WRN] Tool output stored: handle=session-abc/file-xyz, reason=size_cap, bytes=45678, lines=1234, tokens=~8900
```

---

## Accounting

Tool accounting entries include storage info:

```json
{
  "type": "tool",
  "tool": "search_code",
  "stored": true,
  "handle": "session-abc/file-xyz",
  "reason": "size_cap",
  "originalBytes": 45678
}
```

---

## Best Practices

### Research Agents

Higher limits for agents that need full context:

```yaml
toolResponseMaxBytes: 50000
maxTurns: 20
```

### API Agents

Lower limits for quick responses:

```yaml
toolResponseMaxBytes: 8192
maxTurns: 10
```

### Chunked Extraction

Guide the LLM to extract in chunks:

```yaml
---
models:
  - openai/gpt-4o
toolResponseMaxBytes: 10000
---
When tool output is stored, extract relevant sections using tool_output.
Process large responses in chunks rather than all at once.
```

---

## Error Handling

If extraction fails:
1. Error message returned to LLM
2. LLM can try different extraction
3. Truncation as last resort

---

## See Also

- [Context Window](Configuration-Context-Window) - Token budgets
- [docs/specs/tools-overview.md](../docs/specs/tools-overview.md) - Tool system spec
