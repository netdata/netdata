# Session Snapshots

Capture and analyze complete session state for debugging and post-mortem analysis.

---

## Table of Contents

- [Overview](#overview) - What snapshots contain
- [Snapshot Locations](#snapshot-locations) - Where to find them
- [Basic Extraction](#basic-extraction) - Decompress and view
- [Snapshot Structure](#snapshot-structure) - Data organization
- [Common Extractions](#common-extractions) - Frequently needed data
- [LLM Data Extraction](#llm-data-extraction) - LLM requests and responses
- [Tool Data Extraction](#tool-data-extraction) - Tool calls and results
- [Log Extraction](#log-extraction) - WRN/ERR and all logs
- [Sub-Agent Extraction](#sub-agent-extraction) - Nested sessions
- [Debugging Workflows](#debugging-workflows) - Common analysis patterns
- [Configuration](#configuration) - Persistence settings
- [Troubleshooting](#troubleshooting) - Common snapshot issues
- [See Also](#see-also) - Related documentation

---

## Overview

Session snapshots capture **complete session state** including:

- **opTree hierarchy**: Turns and operations
- **LLM request/response payloads**: Full HTTP bodies (base64 encoded)
- **Tool request/response payloads**: Arguments and results
- **All logs**: VRB, WRN, ERR, TRC, THK, FIN entries
- **Accounting entries**: Token counts and costs
- **Sub-agent sessions**: Nested child sessions
- **Session metadata**: Timestamps, IDs, totals

Snapshots are saved automatically at session end and can be used for:

- **Post-mortem debugging**: Understand what went wrong
- **Regression testing**: Compare session behavior
- **Auditing**: Review agent actions
- **Cost analysis**: Calculate actual costs

---

## Snapshot Locations

| Environment | Path                                              |
| ----------- | ------------------------------------------------- |
| Default     | `~/.ai-agent/sessions/{originId}.json.gz`         |
| Production  | `/opt/neda/.ai-agent/sessions/{originId}.json.gz` |

**Filename format**: `{UUID}.json.gz` where UUID is the origin transaction ID.

### Find Latest Snapshot

```bash
ls -lt ~/.ai-agent/sessions/*.json.gz | head -1
```

### Find Snapshot by Partial ID

```bash
ls ~/.ai-agent/sessions/ | grep "756b8ce8"
```

---

## Basic Extraction

### Decompress to JSON

```bash
SNAPSHOT=~/.ai-agent/sessions/756b8ce8-3ad8-4a5a-8094-e45f0ba23a11.json.gz

# Decompress to readable JSON
zcat "$SNAPSHOT" | jq . > session.json

# View structure summary
zcat "$SNAPSHOT" | jq 'keys'
# Output: ["version", "reason", "opTree"]
```

### Quick Session Summary

```bash
zcat "$SNAPSHOT" | jq '{
  traceId: .opTree.traceId,
  agentId: .opTree.agentId,
  success: .opTree.success,
  error: .opTree.error,
  turns: (.opTree.turns | length),
  totals: .opTree.totals
}'
```

**Example output**:

```json
{
  "traceId": "756b8ce8-3ad8-4a5a-8094-e45f0ba23a11",
  "agentId": "research-agent",
  "success": true,
  "error": null,
  "turns": 5,
  "totals": {
    "tokensIn": 4523,
    "tokensOut": 1234,
    "costUsd": 0.12,
    "toolsRun": 8
  }
}
```

---

## Snapshot Structure

```
snapshot.json.gz
├── version: 1                    # Schema version
├── reason: "final"               # Optional trigger (e.g., "final", "subagent_finish")
└── opTree
    ├── id                        # Internal session ID
    ├── traceId?                  # Public transaction ID (matches filename)
    ├── agentId?                  # Agent name
    ├── callPath?                 # Hierarchical path for sub-agents
    ├── sessionTitle              # Session title
    ├── latestStatus?             # Latest status string
    ├── startedAt                 # Unix timestamp (ms)
    ├── endedAt?                  # Unix timestamp (ms)
    ├── success?                  # Boolean outcome
    ├── error?                    # Error message if failed
    ├── attributes?               # Optional session-level attributes
    ├── totals?                   # Aggregated metrics
    │   ├── tokensIn
    │   ├── tokensOut
    │   ├── tokensCacheRead
    │   ├── tokensCacheWrite
    │   ├── costUsd
    │   ├── toolsRun
    │   └── agentsRun
    └── turns[]                   # Array of TurnNode
        ├── id                    # Turn ID
        ├── index                 # 1-based turn number
        ├── startedAt
        ├── endedAt?
        ├── attributes?
        │   └── prompts          # {system, user}
        └── ops[]                # Array of OperationNode
            ├── opId
            ├── kind             # "llm" | "tool" | "system" | "session"
            ├── startedAt
            ├── endedAt?
            ├── status?          # "ok" | "failed"
            ├── attributes?      # {provider, model, name, toolKind}
            ├── logs[]           # LogEntry array
            ├── accounting[]     # AccountingEntry array
            ├── request?         # {kind, payload, size}
            ├── response?        # {payload, size, truncated}
            ├── reasoning?       # {chunks, final} for thinking models
            └── childSession?   # Nested SessionNode for sub-agents
```

---

## Common Extractions

### Turn Count

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns | length'
```

### Session Duration

```bash
zcat "$SNAPSHOT" | jq '(.opTree.endedAt - .opTree.startedAt) / 1000 | "\(.)s"'
```

### Total Cost

```bash
zcat "$SNAPSHOT" | jq '.opTree.totals.costUsd'
```

### All Turns Summary

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[] | {
  index: .index,
  ops: (.ops | length),
  duration: ((.endedAt - .startedAt) / 1000)
}]'
```

### Final Report Content

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] |
  select(.attributes.name | test("final_report"; "i")) |
  .response.payload]'
```

---

## LLM Data Extraction

### All LLM Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm")]'
```

### LLM Operations with Metadata

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | {
  opId: .opId,
  provider: .attributes.provider,
  model: .attributes.model,
  status: .status,
  requestSize: .request.size,
  responseSize: .response.size
}]'
```

### LLM Request Payloads

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | .request.payload'
```

### LLM Response Payloads

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | .response.payload'
```

### LLM Accounting (Tokens, Cost)

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[]]'
```

### Decode Raw HTTP Payload (Base64)

```bash
zcat "$SNAPSHOT" | jq -r '.opTree.turns[0].ops[] | select(.kind == "llm") | .request.payload.raw // empty' | base64 -d
```

---

## Tool Data Extraction

### All Tool Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool")]'
```

### Tool Operations with Metadata

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | {
  opId: .opId,
  name: .attributes.name,
  toolKind: .attributes.toolKind,
  status: .status,
  latency: ((.endedAt - .startedAt) / 1000)
}]'
```

### Tool Request Payloads

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | .request.payload'
```

### Tool Response Payloads

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | .response.payload'
```

### Find Tool by Name

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .attributes.name == "search_code")]'
```

### Unique Tools Used

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .attributes.name] | unique'
```

### Tool Accounting

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .accounting[]]'
```

### Slowest Tools

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | {
  name: .attributes.name,
  latencyMs: (.endedAt - .startedAt)
}] | sort_by(-.latencyMs) | .[0:5]'
```

---

## Log Extraction

### All Logs (Sorted by Timestamp)

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity"))] | sort_by(.timestamp)'
```

### WRN and ERR Only

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(.severity == "WRN" or .severity == "ERR")] | sort_by(.timestamp)'
```

### Error Count by Severity

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity")) | .severity] | group_by(.) | map({severity: .[0], count: length})'
```

### Logs with Stack Traces

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("stack")) | {severity, message, stack}]'
```

### Faster Log Extraction (Large Snapshots)

```bash
# Structured access instead of recursive descent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[].logs[]]'
```

### Fatal Logs (Session-Stopping)

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(.fatal == true)]'
```

---

## Sub-Agent Extraction

### All Sub-Agent Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session")]'
```

### Sub-Agent Names

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .attributes.name] | unique'
```

### Sub-Agent Session Metadata

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {
  name: .attributes.name,
  sessionId: .childSession.id,
  success: .childSession.success,
  totals: .childSession.totals
}]'
```

### Extract Specific Sub-Agent

```bash
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession'
```

### Sub-Agent LLM Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "llm")]'
```

### Sub-Agent Tool Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "tool")]'
```

### Sub-Agent Logs (WRN/ERR)

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[].logs[] | select(.severity == "WRN" or .severity == "ERR")]'
```

### Extract Each Sub-Agent to File

```bash
for name in $(zcat "$SNAPSHOT" | jq -r '.opTree.turns[].ops[] | select(.kind == "session") | .attributes.name' | sort -u); do
  safe_name=$(echo "$name" | tr '/' '-')
  zcat "$SNAPSHOT" | jq ".opTree.turns[].ops[] | select(.kind == \"session\" and .attributes.name == \"$name\") | .childSession" > "subagent-${safe_name}.json"
done
```

---

## Debugging Workflows

### Why Did the Agent Fail?

```bash
# 1. Check for errors
zcat "$SNAPSHOT" | jq '[.. | objects | select(.severity == "ERR")] | .[0]'

# 2. Check last turn
zcat "$SNAPSHOT" | jq '.opTree.turns[-1]'

# 3. Check final status
zcat "$SNAPSHOT" | jq '{success: .opTree.success, error: .opTree.error}'
```

### What Tools Were Called?

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .attributes.name] | group_by(.) | map({tool: .[0], count: length})'
```

### Token Usage Per Turn

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[] | {turn: .turn, input: .tokens.inputTokens, output: .tokens.outputTokens}]'
```

### Failed Operations

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.status == "failed") | {kind, name: .attributes.name, error}]'
```

### Compare Two Sessions

```bash
for f in good.json.gz bad.json.gz; do
  echo "=== $f ==="
  zcat "$f" | jq '{
    turns: (.opTree.turns | length),
    tools: ([.opTree.turns[].ops[] | select(.kind == "tool")] | length),
    errors: ([.. | objects | select(.severity == "ERR")] | length),
    success: .opTree.success
  }'
done
```

---

## Configuration

### Persistence Settings

```json
{
  "persistence": {
    "sessionsDir": "~/.ai-agent/sessions/",
    "billingFile": "~/.ai-agent/accounting.jsonl"
  }
}
```

| Setting       | Description                  | Default                        |
| ------------- | ---------------------------- | ------------------------------ |
| `sessionsDir` | Directory for snapshot files | `~/.ai-agent/sessions/`        |
| `billingFile` | Path for accounting ledger   | `~/.ai-agent/accounting.jsonl` |

### Custom Snapshot Handling (Library Mode)

```typescript
const callbacks = {
  onEvent: (event) => {
    if (event.type === "snapshot") {
      // Custom snapshot handling
      myStorage.save(event.payload);
    }
  },
};
```

---

## Troubleshooting

### Problem: Snapshot not saved

**Cause**: Callback error or disk permission issue.

**Solution**:

1. Check disk space: `df -h ~/.ai-agent/`
2. Check permissions: `ls -la ~/.ai-agent/sessions/`
3. Check for errors in stderr during session

---

### Problem: Missing data in snapshot

**Cause**: Session ended before operation completed.

**Solution**:

- Check `endedAt` timestamp on operations
- Verify operation `status` field
- LLM payloads are always captured; check `request.payload.raw`

---

### Problem: Large snapshot size

**Cause**: Many tool responses or large payloads.

**Solution**:

- Lower `toolResponseMaxBytes` to store large responses externally
- Use tool_output handles for large responses
- Review tool response sizes:
  ```bash
  zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {kind, name: .attributes.name, size: .response.size}] | sort_by(-.size) | .[0:10]'
  ```

---

### Problem: Cannot decompress snapshot

**Cause**: Corrupted file or incomplete write.

**Solution**:

1. Check file size: `ls -la "$SNAPSHOT"`
2. Try gzip test: `gzip -t "$SNAPSHOT"`
3. Check for incomplete session (crashed before flush)

---

### Problem: Snapshot filename not matching session

**Cause**: Filename uses `originId` (root transaction ID). This always matches the root session's `traceId`, but for sub-agents their `traceId` differs from the filename.

**Solution**: Use the `traceId` field inside the snapshot for each session:

```bash
zcat "$SNAPSHOT" | jq '.opTree.traceId'
```

---

## See Also

- [Operations](Operations) - Operations overview
- [Debugging Guide](Operations-Debugging) - Debugging workflow
- [Accounting](Operations-Accounting) - Cost tracking
- [specs/snapshots.md](specs/snapshots.md) - Technical specification
- [skills/ai-agent-session-snapshots.md](skills/ai-agent-session-snapshots.md) - Complete extraction commands
