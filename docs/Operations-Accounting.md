# Accounting

Track token usage and costs for AI Agent sessions.

---

## Table of Contents

- [Overview](#overview) - What accounting tracks
- [Enable Accounting](#enable-accounting) - Configuration options
- [Entry Types](#entry-types) - LLM and tool entries
- [Entry Structure](#entry-structure) - Complete field reference
- [Analysis Queries](#analysis-queries) - Extract insights from accounting data
- [Privacy](#privacy) - What data is not logged
- [Library Mode](#library-mode) - Callback-based accounting
- [File Rotation](#file-rotation) - Managing log files
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related documentation

---

## Overview

Accounting tracks every LLM request and tool execution:

- **Token usage**: Input, output, cache read/write
- **Costs**: Per-request and cumulative
- **Latency**: Response times
- **Status**: Success or failure
- **Trace context**: Session and agent IDs for correlation

**Use cases**:

- Cost monitoring and budgeting
- Usage analysis and optimization
- Billing reconciliation
- Performance tracking

---

## Enable Accounting

### Configuration File

```json
{
  "persistence": {
    "billingFile": "${HOME}/.ai-agent/accounting.jsonl"
  }
}
```

### CLI Override

```bash
ai-agent --agent test.ai --billing-file ./accounting.jsonl "query"
```

### Default Location

When not configured, accounting writes to:

```
~/.ai-agent/accounting.jsonl
```

---

## Entry Types

### LLM Entries

Created for every LLM request (including retries):

```json
{
  "type": "llm",
  "status": "ok",
  "timestamp": 1736944200000,
  "provider": "openai",
  "model": "gpt-4o",
  "actualProvider": "actual-provider",
  "actualModel": "actual-model",
  "costUsd": 0.0084,
  "upstreamInferenceCostUsd": 0.005,
  "stopReason": "stop",
  "latency": 2341,
  "tokens": {
    "inputTokens": 1523,
    "outputTokens": 456,
    "totalTokens": 1979,
    "cacheReadInputTokens": 0,
    "cacheWriteInputTokens": 1523
  },
  "error": "error message",
  "agentId": "agent-id",
  "callPath": "path",
  "txnId": "abc123",
  "parentTxnId": "parent-id",
  "originTxnId": "root-id",
  "details": {}
}
```

### Tool Entries

Created for every tool execution:

```json
{
  "type": "tool",
  "status": "ok",
  "timestamp": 1736944201000,
  "mcpServer": "github",
  "command": "search_code",
  "latency": 523,
  "charactersIn": 45,
  "charactersOut": 12456,
  "error": "error message",
  "agentId": "agent-id",
  "callPath": "path",
  "txnId": "abc123",
  "parentTxnId": "parent-id",
  "originTxnId": "root-id",
  "details": {}
}
```

---

## Entry Structure

### LLM Entry Fields

| Field                      | Type   | Description                    |
| -------------------------- | ------ | ------------------------------ |
| `type`                     | string | Always `"llm"`                 |
| `status`                   | string | `"ok"` or `"failed"`           |
| `timestamp`                | number | Unix timestamp (ms)            |
| `provider`                 | string | Provider name (e.g., "openai") |
| `model`                    | string | Model name (e.g., "gpt-4o")    |
| `actualProvider`           | string | Actual provider (for routers)  |
| `actualModel`              | string | Actual model (for routers)     |
| `latency`                  | number | Request latency (ms)           |
| `costUsd`                  | number | Cost in USD                    |
| `upstreamInferenceCostUsd` | number | Upstream cost (routers)        |
| `stopReason`               | string | Why generation stopped         |
| `error`                    | string | Error message if failed        |
| `details`                  | object | Optional structured details    |

**Token Fields**:

| Field                          | Type   | Description            |
| ------------------------------ | ------ | ---------------------- |
| `tokens.inputTokens`           | number | Input token count      |
| `tokens.outputTokens`          | number | Output token count     |
| `tokens.totalTokens`           | number | Total (includes cache) |
| `tokens.cacheReadInputTokens`  | number | Cached tokens read     |
| `tokens.cacheWriteInputTokens` | number | Cached tokens written  |

**Trace Context**:

| Field         | Type   | Description            |
| ------------- | ------ | ---------------------- |
| `agentId`     | string | Agent identifier       |
| `callPath`    | string | Hierarchical call path |
| `txnId`       | string | Session transaction ID |
| `parentTxnId` | string | Parent session ID      |
| `originTxnId` | string | Root session ID        |

### Tool Entry Fields

| Field           | Type   | Description                 |
| --------------- | ------ | --------------------------- |
| `type`          | string | Always `"tool"`             |
| `status`        | string | `"ok"` or `"failed"`        |
| `timestamp`     | number | Unix timestamp (ms)         |
| `mcpServer`     | string | MCP server name             |
| `command`       | string | Tool name                   |
| `latency`       | number | Execution latency (ms)      |
| `charactersIn`  | number | Request character count     |
| `charactersOut` | number | Response character count    |
| `error`         | string | Error message if failed     |
| `details`       | object | Optional structured details |

Plus same trace context fields as LLM entries.

---

## Analysis Queries

### Total Cost

```bash
cat accounting.jsonl | jq -s 'map(select(.costUsd)) | map(.costUsd) | add'
```

### Cost by Model

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.model) |
  map({model: .[0].model, cost: (map(.costUsd // 0) | add), requests: length})
'
```

### Cost by Agent

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.agentPath) |
  map({agent: .[0].agentPath, cost: (map(.costUsd // 0) | add)})
'
```

### Daily Summary

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.timestamp | . / 86400000 | floor) |
  map({
    date: (.[0].timestamp / 1000 | strftime("%Y-%m-%d")),
    cost: (map(.costUsd // 0) | add),
    tokens: (map(.tokens.totalTokens // 0) | add),
    requests: length
  })
'
```

### Hourly Token Usage

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.timestamp | . / 3600000 | floor) |
  map({
    hour: (.[0].timestamp / 1000 | strftime("%Y-%m-%d %H:00")),
    inputTokens: (map(.tokens.inputTokens // 0) | add),
    outputTokens: (map(.tokens.outputTokens // 0) | add)
  })
'
```

### Slowest LLM Requests

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  sort_by(-.latency) |
  .[0:10] |
  map({model: .model, latency: .latency, tokens: .tokens.totalTokens})
'
```

### Slowest Tools

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "tool")) |
  sort_by(-.latency) |
  .[0:10] |
  map({tool: .command, server: .mcpServer, latency: .latency})
'
```

### Error Rate by Provider

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.provider) |
  map({
    provider: .[0].provider,
    total: length,
    errors: (map(select(.status == "failed")) | length),
    errorRate: ((map(select(.status == "failed")) | length) / length)
  })
'
```

### Cache Effectiveness

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm" and .tokens.cacheReadInputTokens != null)) |
  {
    totalInput: (map(.tokens.inputTokens) | add),
    cacheRead: (map(.tokens.cacheReadInputTokens) | add),
    cacheRate: ((map(.tokens.cacheReadInputTokens) | add) / (map(.tokens.inputTokens + .tokens.cacheReadInputTokens) | add))
  }
'
```

### Cost by Session

```bash
cat accounting.jsonl | jq -s '
  map(select(.type == "llm")) |
  group_by(.originTxnId) |
  map({session: .[0].originTxnId, cost: (map(.costUsd // 0) | add)}) |
  sort_by(-.cost) |
  .[0:10]
'
```

---

## Privacy

Accounting entries **never** include:

- Prompt content (messages, system prompts)
- Response content (LLM output)
- Tool arguments (parameters passed to tools)
- Tool responses (data returned by tools)
- User data or PII

Only metadata (counts, timing, costs) is logged.

---

## Library Mode

When using ai-agent as a library, accounting is delivered via callbacks:

```typescript
const callbacks = {
  onEvent: (event) => {
    if (event.type === "accounting") {
      // Single entry (real-time)
      myDatabase.insert(event.entry);
    }
    if (event.type === "accounting_flush") {
      // Batch of entries (at session end)
      myDatabase.insertBatch(event.payload.entries);
    }
  },
};
```

### Event Types

| Event Type         | When               | Payload                  |
| ------------------ | ------------------ | ------------------------ |
| `accounting`       | Each LLM/tool call | Single `AccountingEntry` |
| `accounting_flush` | Session end        | Array of all entries     |

File writing is skipped when custom callbacks handle accounting.

---

## File Rotation

For long-running deployments, rotate accounting files:

### logrotate Configuration

```
# /etc/logrotate.d/ai-agent
/var/log/ai-agent-accounting.jsonl {
  daily
  rotate 30
  compress
  delaycompress
  missingok
  notifempty
  copytruncate
}
```

### Manual Rotation

```bash
# Rotate with timestamp
mv accounting.jsonl accounting-$(date +%Y%m%d).jsonl

# Compress old files
gzip accounting-*.jsonl
```

### Archive Analysis

```bash
# Query across compressed archives
zcat accounting-2025*.jsonl.gz | jq -s '
  map(select(.type == "llm")) |
  {total_cost: (map(.costUsd // 0) | add)}
'
```

---

## Troubleshooting

### Problem: No accounting file created

**Cause**: File path not configured or directory doesn't exist.

**Solution**:

1. Check configuration: `cat .ai-agent.json | jq '.persistence'`
2. Create directory: `mkdir -p ~/.ai-agent/`
3. Verify permissions: `ls -la ~/.ai-agent/`

---

### Problem: Missing cost information

**Cause**: Pricing table doesn't include the model.

**Solution**:

1. Check pricing configuration
2. Verify provider/model names match exactly
3. Cost defaults to `undefined` when pricing unknown

---

### Problem: Token counts don't match provider

**Cause**: Token normalization includes cache tokens.

**Solution**: AI Agent normalizes `totalTokens` to include cache:

```
totalTokens = inputTokens + outputTokens + cacheRead + cacheWrite
```

Compare individual fields, not totals.

---

### Problem: Entries not appearing in real-time

**Cause**: Entries are buffered until session end.

**Solution**:

- Use `onEvent(type='accounting')` for real-time entries
- `accounting_flush` delivers all entries at once

---

### Problem: Wrong trace context

**Cause**: Session configuration not propagating IDs.

**Solution**: Verify session configuration includes:

- `agentId`
- `txnId`
- `parentTxnId`
- `originTxnId`

---

### Problem: Cost mismatch with provider bill

**Cause**: Pricing table outdated or router costs different.

**Solution**:

1. Update pricing configuration
2. For routers (OpenRouter), check `upstreamInferenceCostUsd`
3. Compare `actualProvider`/`actualModel` for routing decisions

---

## See Also

- [Operations](Operations) - Operations overview
- [Telemetry](Operations-Telemetry) - Prometheus metrics
- [Configuration-Pricing](Configuration-Pricing) - Pricing configuration
- [specs/accounting.md](specs/accounting.md) - Technical specification
