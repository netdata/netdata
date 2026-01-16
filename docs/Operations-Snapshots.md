# Session Snapshots

Capture and analyze complete session state.

---

## Overview

Session snapshots capture:
- Complete conversation history
- LLM requests/responses
- Tool calls and results
- Logs (WRN/ERR)
- Accounting data
- Sub-agent sessions

---

## Snapshot Location

| Environment | Path |
|-------------|------|
| Default | `~/.ai-agent/sessions/{originId}.json.gz` |
| Production | `/opt/neda/.ai-agent/sessions/{originId}.json.gz` |

---

## Basic Extraction

### Decompress

```bash
zcat session.json.gz > session.json
```

### View Structure

```bash
zcat session.json.gz | jq 'keys'
```

Output:
```json
["meta", "opTree", "accounting", "logs"]
```

---

## Common Extractions

### Turn Count

```bash
zcat session.json.gz | jq '.opTree.turns | length'
```

### LLM Requests

```bash
zcat session.json.gz | jq '.opTree.turns[].ops[] | select(.kind == "llm")'
```

### Tool Calls

```bash
zcat session.json.gz | jq '.opTree.turns[].ops[] | select(.kind == "tool")'
```

### Errors and Warnings

```bash
zcat session.json.gz | jq '[.. | objects | select(.severity == "ERR" or .severity == "WRN")]'
```

### Final Report

```bash
zcat session.json.gz | jq '.opTree.turns[-1].ops[] | select(.kind == "final")'
```

---

## Detailed Extractions

### LLM Request Payload

```bash
zcat session.json.gz | jq '
  .opTree.turns[0].ops[] |
  select(.kind == "llm") |
  .request
'
```

### LLM Response

```bash
zcat session.json.gz | jq '
  .opTree.turns[0].ops[] |
  select(.kind == "llm") |
  .response
'
```

### Tool Request/Response Pairs

```bash
zcat session.json.gz | jq '
  .opTree.turns[].ops[] |
  select(.kind == "tool") |
  {tool: .name, request: .request, response: .response}
'
```

### Sub-Agent Sessions

```bash
zcat session.json.gz | jq '
  .opTree.turns[].ops[] |
  select(.kind == "session") |
  .childSession
'
```

---

## Snapshot Structure

```
session.json.gz
├── meta
│   ├── originId
│   ├── agentPath
│   ├── startTime
│   └── endTime
├── opTree
│   └── turns[]
│       └── ops[]
│           ├── kind: "llm" | "tool" | "final" | "session"
│           ├── request
│           └── response
├── accounting[]
│   ├── type: "llm" | "tool"
│   └── ... metrics
└── logs[]
    ├── severity
    ├── message
    └── timestamp
```

---

## Debugging Workflows

### Why Did Agent Fail?

```bash
# 1. Check for errors
zcat session.json.gz | jq '[.. | objects | select(.severity == "ERR")] | .[0]'

# 2. Check last turn
zcat session.json.gz | jq '.opTree.turns[-1]'

# 3. Check final report status
zcat session.json.gz | jq '.opTree.turns[-1].ops[] | select(.kind == "final") | .status'
```

### What Tools Were Called?

```bash
zcat session.json.gz | jq '
  [.opTree.turns[].ops[] | select(.kind == "tool") | .name] |
  group_by(.) |
  map({tool: .[0], count: length})
'
```

### Token Usage Per Turn

```bash
zcat session.json.gz | jq '
  .accounting |
  map(select(.type == "llm")) |
  map({turn: .turn, input: .inputTokens, output: .outputTokens})
'
```

---

## Production Debugging

### Get Session from Production

```bash
# Copy snapshot
scp server:/opt/neda/.ai-agent/sessions/abc123.json.gz .

# Or use journald to find session ID
journalctl -u ai-agent | grep "session started" | tail -1
```

### Compare Sessions

```bash
# Extract key metrics from two sessions
for f in session1.json.gz session2.json.gz; do
  echo "=== $f ==="
  zcat $f | jq '{
    turns: (.opTree.turns | length),
    tools: ([.opTree.turns[].ops[] | select(.kind == "tool")] | length),
    errors: ([.. | objects | select(.severity == "ERR")] | length)
  }'
done
```

---

## See Also

- [skills/ai-agent-session-snapshots.md](skills/ai-agent-session-snapshots.md) - Complete extraction guide
- [specs/snapshots.md](specs/snapshots.md) - Technical spec
