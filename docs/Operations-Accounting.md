# Accounting

Track token usage and costs.

---

## Enable Accounting

### Configuration

```json
{
  "accounting": {
    "file": "${HOME}/ai-agent-accounting.jsonl"
  }
}
```

### CLI

```bash
ai-agent --agent test.ai --accounting ./accounting.jsonl "query"
```

---

## Entry Types

### LLM Entries

```json
{
  "type": "llm",
  "status": "ok",
  "timestamp": "2025-01-15T10:30:00.000Z",
  "provider": "openai",
  "model": "gpt-4o",
  "inputTokens": 1523,
  "outputTokens": 456,
  "cacheReadTokens": 0,
  "cacheWriteTokens": 1523,
  "cost": 0.0084,
  "latencyMs": 2341,
  "sessionId": "abc123",
  "agentPath": "chat.ai",
  "turn": 1
}
```

### Tool Entries

```json
{
  "type": "tool",
  "status": "ok",
  "timestamp": "2025-01-15T10:30:01.000Z",
  "server": "github",
  "tool": "search_code",
  "latencyMs": 523,
  "requestBytes": 45,
  "responseBytes": 12456,
  "sessionId": "abc123",
  "agentPath": "chat.ai",
  "turn": 1
}
```

---

## Analysis Queries

### Total Cost

```bash
cat accounting.jsonl | jq -s 'map(select(.cost)) | map(.cost) | add'
```

### Cost by Model

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.model) |
  map({model: .[0].model, cost: (map(.cost) | add), requests: length})
'
```

### Cost by Agent

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.agentPath) |
  map({agent: .[0].agentPath, cost: (map(.cost) | add)})
'
```

### Daily Summary

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.timestamp[:10]) |
  map({
    date: .[0].timestamp[:10],
    cost: (map(.cost) | add),
    tokens: (map(.inputTokens + .outputTokens) | add)
  })
'
```

### Slowest Tools

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "tool")) |
  sort_by(-.latencyMs) |
  .[0:10] |
  map({tool: .tool, latency: .latencyMs})
'
```

---

## Privacy

Accounting entries **never** include:
- Prompt content
- Response content
- Tool arguments
- User data

Only metadata (counts, timing, costs) is logged.

---

## Library Mode

In library mode, accounting is delivered via callbacks:

```typescript
const callbacks = {
  onEvent: (event) => {
    if (event.type === 'accounting') {
      myDatabase.insert(event.entry);
    }
  }
};
```

File writing is skipped when callbacks are provided.

---

## Rotation

For long-running deployments, rotate accounting files:

```bash
# logrotate config
/var/log/ai-agent-accounting.jsonl {
  daily
  rotate 30
  compress
  missingok
  notifempty
}
```

---

## See Also

- [Pricing](Configuration-Pricing) - Cost configuration
- [Telemetry](Operations-Telemetry) - Prometheus metrics
- [docs/specs/accounting.md](../docs/specs/accounting.md) - Technical spec
