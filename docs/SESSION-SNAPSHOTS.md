# Session Snapshots Guide

## TL;DR

Session snapshots are gzipped JSON files (`{UUID}.json.gz`) containing complete session state:

- opTree hierarchy (turns → operations)
- LLM request/response payloads
- Tool request/response payloads
- All logs (including WRN/ERR)
- Accounting entries
- Nested sub-agent sessions

Default location: `~/.ai-agent/sessions/`

---

## File Structure

### Top-Level Structure

```json
{
  "version": 1,
  "reason": "final" | "subagent_finish",
  "opTree": { ... }
}
```

| Field     | Description                                                                |
| --------- | -------------------------------------------------------------------------- |
| `version` | Snapshot schema version (currently 1)                                      |
| `reason`  | Trigger: `"final"` (session end) or `"subagent_finish"` (child agent done) |
| `opTree`  | Complete session hierarchy (see below)                                     |

### OpTree Structure

```
SessionNode
├── id                    // Internal session ID
├── traceId               // Public transaction ID (matches filename)
├── agentId               // Agent name (e.g., "phase2-run-test-1")
├── callPath              // Hierarchical call path for sub-agents
├── sessionTitle         // Session title
├── latestStatus         // Current status message
├── startedAt            // Unix timestamp (ms)
├── endedAt              // Unix timestamp (ms)
├── success              // Boolean session outcome
├── error                // Error message if failed
├── attributes           // Additional metadata (e.g., ingress info)
├── totals               // Aggregated metrics
│   ├── tokensIn
│   ├── tokensOut
│   ├── tokensCacheRead
│   ├── tokensCacheWrite
│   ├── costUsd
│   ├── toolsRun
│   └── agentsRun
└── turns                // Array of TurnNode
    └── TurnNode
        ├── id
        ├── index       // 1-based turn number
        ├── startedAt
        ├── endedAt
        ├── attributes  // Contains prompts: {system, user}
        └── ops         // Array of OperationNode
            └── OperationNode
                ├── opId
                ├── kind         // "llm" | "tool" | "system" | "session"
                ├── startedAt
                ├── endedAt
                ├── status       // "ok" | "failed"
                ├── attributes   // {provider, model, name, toolKind, etc.}
                ├── logs        // Array of LogEntry
                ├── accounting  // Array of AccountingEntry
                ├── reasoning   // { chunks: [{text, ts}], final?: string }
                ├── request      // {kind, payload, size}
                ├── response     // {payload, size, truncated}
                └── childSession // Nested SessionNode (for sub-agents and tool_output)
```

### LogEntry Structure

```typescript
interface LogEntry {
  timestamp: number; // Unix timestamp (ms)
  severity: "VRB" | "WRN" | "ERR" | "TRC" | "THK" | "FIN";
  turn: number; // Turn index (0-based, 0=system init)
  subturn: number; // Tool call index within turn
  path?: string; // Hierarchical path (e.g., "1-2.3-1" = turn1-op2.subturn3-op1)
  direction: "request" | "response";
  type: "llm" | "tool";
  toolKind?: "mcp" | "rest" | "agent" | "command";
  remoteIdentifier: string; // "provider:model" or "protocol:namespace:tool"
  fatal: boolean; // Caused session to stop?
  message: string; // Human-readable summary
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  agentPath?: string;
  turnPath?: string;
  max_turns?: number;
  max_subturns?: number;
  bold?: boolean;
  headendId?: string;
  stack?: string;
  details?: Record<string, LogDetailValue>;
  llmRequestPayload?: LogPayload;
  llmResponsePayload?: LogPayload;
  toolRequestPayload?: LogPayload;
  toolResponsePayload?: LogPayload;
}
```

### AccountingEntry Structure

```typescript
interface AccountingEntry {
  type: "llm" | "tool";
  timestamp: number;
  status: "ok" | "failed";
  latency: number;
  // LLM entries:
  provider?: string;
  model?: string;
  tokens?: {
    inputTokens;
    outputTokens;
    totalTokens;
    cacheReadInputTokens;
    cacheWriteInputTokens;
  };
  costUsd?: number;
  // Tool entries:
  mcpServer?: string;
  command?: string;
  charactersIn?: number;
  charactersOut?: number;
}
```

---

## Common Extraction Commands

### Setup

```bash
# Define variables
SNAPSHOT_DIR="${HOME}/.ai-agent/sessions"
TXN_ID="756b8ce8-3ad8-4a5a-8094-e45f0ba23a11"
SNAPSHOT="${SNAPSHOT_DIR}/${TXN_ID}.json.gz"
```

### 1. Extract Full Snapshot (Pretty JSON)

```bash
# Decompress to readable JSON
zcat "$SNAPSHOT" | jq . > "${TXN_ID}.json"

# View structure summary
zcat "$SNAPSHOT" | jq '{version, reason, opTree: {id, traceId, agentId, turns: [.opTree.turns[].index]}}'
```

### 2. Extract Session Metadata

```bash
# Session info
zcat "$SNAPSHOT" | jq '{sessionId:.opTree.id, traceId:.opTree.traceId, agentId:.opTree.agentId, callPath:.opTree.callPath, success:.opTree.success, error:.opTree.error, totals:.opTree.totals}'

# Example output:
# {
#   "sessionId": "mgbsnq2h-dx4iw2",
#   "traceId": "756b8ce8-3ad8-4a5a-8094-e45f0ba23a11",
#   "agentId": "phase2-run-test-1",
#   "callPath": "phase2-run-test-1",
#   "success": true,
#   "error": null,
#   "totals": { "tokensIn": 120, "tokensOut": 40, ... }
# }
```

### 3. Extract All Turns Summary

```bash
# List all turns with their operation counts
zcat "$SNAPSHOT" | jq '[.opTree.turns[] | {index:.index, opCount:.ops | length, startedAt:.startedAt}]'
```

### 4. Extract LLM Operations Only

```bash
# All LLM operations with request/response previews
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | {opId:.opId, turn:.attributes, provider:.attributes.provider, model:.attributes.model, request:.request, response:.response}]'

# Extract just LLM request payloads
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | .request'

# Extract LLM accounting (tokens, cost)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[]]'
```

### 5. Extract Tool Operations Only

```bash
# All tool operations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | {opId:.opId, tool:.attributes.name, provider:.attributes.provider, request:.request, response:.response}]'

# Tool accounting
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .accounting[]]'
```

### 6. Extract Sub-Agent Sessions

```bash
# Find all sub-agent operations with metadata
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {opId:.opId, name:.attributes.name, childSessionId:.childSession.id, childTurns:.childSession.turns | length}]'

# Extract nested child session to file
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session") | .childSession' > child-session.json

# Extract specific sub-agent by name (e.g., "agent__bigquery")
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession'

# Extract sub-agent by opId
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session" and .opId == "mfpy4i9m-5nrtwm") | .childSession'

# Count total sub-agents
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session")] | length'

# List all sub-agent names
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .attributes.name] | unique'

# Extract sub-agent call paths (hierarchical)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {opId:.opId, name:.attributes.name, callPath:.childSession.callPath}]'
```

### 7. Extract All Logs

```bash
# All logs sorted by timestamp
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity"))] | sort_by(.timestamp)'

# Logs with severity filter
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity")) | {severity:.severity, message:.message, timestamp:.timestamp, path:.path}] | sort_by(.timestamp)'
```

### 8. Extract WRN and ERR Logs Only

```bash
# All warnings and errors
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity")) | select(.severity == "WRN" or .severity == "ERR") | {severity:.severity, message:.message, timestamp:.timestamp, path:.path, turn:.turn, subturn:.subturn}] | sort_by(.timestamp)'

# Count of each severity
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("severity")) | .severity] | group_by(.) | map({severity:.[0], count:length})'

# Example output:
# [
#   { "severity": "ERR", "count": 1 },
#   { "severity": "WRN", "count": 5 },
#   { "severity": "VRB", "count": 150 }
# ]
```

### 9. Extract LLM Request Payloads

```bash
# Full request payload (includes messages, isFinalTurn, etc.)
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | .request.payload'

# Request size and metadata
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | {opId:.opId, requestSize:.request.size, requestKind:.request.kind}'
```

### 10. Extract LLM Response Payloads

```bash
# Response payload (textPreview for non-streaming)
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | .response.payload'

# Response metadata
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm") | {opId:.opId, responseSize:.response.size, truncated:.response.truncated}'
```

### 11. Extract Tool Request Payloads

```bash
# Tool request payload
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | .request.payload'

# Tool request metadata
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | {opId:.opId, toolName:.attributes.name, requestSize:.request.size}'
```

### 12. Extract Tool Response Payloads

```bash
# Tool response payload
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | .response.payload'

# Tool response metadata
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool") | {opId:.opId, responseSize:.response.size, truncated:.response.truncated}'
```

### 13. Extract Accounting Entries

```bash
# All accounting entries
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[].accounting[]]'

# LLM accounting only
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[]]'

# Tool accounting only
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .accounting[]]'

# Session totals
zcat "$SNAPSHOT" | jq '.opTree.totals'
```

### 14. Extract Prompts (System + User)

```bash
# System prompt from first turn
zcat "$SNAPSHOT" | jq '.opTree.turns[0].attributes.prompts.system'

# User prompt from first turn
zcat "$SNAPSHOT" | jq '.opTree.turns[0].attributes.prompts.user'

# All prompts across turns
zcat "$SNAPSHOT" | jq '[.opTree.turns[] | {turn:.index, prompts:.attributes.prompts}]'
```

### 15. Extract by Operation Path

```bash
# Find operation by path (e.g., "1.2" = turn 1, op 2)
zcat "$SNAPSHOT" | jq '.opTree.turns[] | select(.index == 1) | .ops[] | select(.opId == "mgbsnq4q-s66mpu")'

# Get operation by hierarchical path label
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("path")) | select(.path == "1-2")]'
```

---

## Advanced Sub-Agent Patterns

### Extract Nested Sub-Agents (Sub-Agent Within Sub-Agent)

```bash
# Find all nested sub-agents (sub-agent calling another sub-agent)
zcat "$SNAPSHOT" | jq '.. | objects | select(has("childSession")) | .. | objects | select(has("childSession")) | {opId:.opId, name:.attributes.name, childSessionId:.childSession.id}'

# Extract sub-agent's sub-agents
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "session") | {parentOpId:.opId, subAgentName:.attributes.name, subAgentSessionId:.childSession.id}'
```

### Extract Logs from Sub-Agent Sessions

```bash
# All logs from sub-agent sessions
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[].logs[]]'

# WRN/ERR logs from sub-agents only
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[].logs[] | select(.severity == "WRN" or .severity == "ERR")]'

# Logs from specific sub-agent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession.turns[].ops[].logs[]]'
```

### Extract LLM Operations from Sub-Agents

```bash
# All LLM operations from all sub-agents
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "llm")]'

# LLM operations from specific sub-agent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession.turns[].ops[] | select(.kind == "llm")]'

# LLM request payloads from sub-agents
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "llm") | .request.payload]'
```

### Extract Tool Operations from Sub-Agents

```bash
# All tool operations from all sub-agents
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "tool")]'

# Tool operations from specific sub-agent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession.turns[].ops[] | select(.kind == "tool")]'

# Tool accounting from sub-agents
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[].ops[] | select(.kind == "tool") | .accounting[]]'
```

### Extract Sub-Agent Session Metadata

```bash
# Sub-agent session IDs and trace IDs
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {opId:.opId, name:.attributes.name, sessionId:.childSession.id, traceId:.childSession.traceId, callPath:.childSession.callPath, success:.childSession.success}]'

# Sub-agent totals (tokens, cost, tools run)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {name:.attributes.name, totals:.childSession.totals}]'

# Sub-agent ingress metadata (if available)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | {name:.attributes.name, ingress:.childSession.attributes.ingress}]'
```

### Extract Sub-Agent Prompts

```bash
# System prompt from first turn of sub-agent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[0].attributes.prompts.system]'

# User prompt from first turn of sub-agent
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "session") | .childSession.turns[0].attributes.prompts.user]'

# Prompts from specific sub-agent
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "session" and .attributes.name == "agent__bigquery") | .childSession.turns[0].attributes.prompts'
```

### Flatten All Sub-Agent Sessions to Individual Files

```bash
# Extract each sub-agent to its own JSON file
for i in $(zcat "$SNAPSHOT" | jq -r '.opTree.turns[].ops[] | select(.kind == "session") | "\(.opId)|\(.attributes.name)|\(.childSession.id)"'); do
  opId=$(echo "$i" | cut -d'|' -f1)
  name=$(echo "$i" | cut -d'|' -f2)
  sessionId=$(echo "$i" | cut -d'|' -f3)
  safe_name=$(echo "$name" | tr '/' '-')
  zcat "$SNAPSHOT" | jq ".opTree.turns[].ops[] | select(.opId == \"$opId\") | .childSession" > "${TXN_ID}-subagent-${safe_name}-${sessionId}.json"
done
```

---

## Advanced Patterns

### Extract All LLM Messages (Conversation History)

```bash
# Extract all messages sent to LLM
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .request.payload.messages]'
```

### Extract Raw HTTP Payloads (if available)

```bash
# Check for raw payload in logs
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("llmRequestPayload")) | .llmRequestPayload]'

# Check for tool raw payload
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("toolRequestPayload")) | .toolRequestPayload]'
```

### Extract Failed Operations

```bash
# Operations with failed status
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.status == "failed")]'

# Failed operations with error info
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.status == "failed") | {opId:.opId, kind:.kind, error:.error, attributes:.attributes}]'
```

### Extract Timing Information

```bash
# Session duration
zcat "$SNAPSHOT" | jq '{started:.opTree.startedAt, ended:.opTree.endedAt, durationMs:(.opTree.endedAt - .opTree.startedAt)}'

# Operation durations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {opId:.opId, kind:.kind, durationMs:(.endedAt - .startedAt)}]'
```

### Extract Reasoning/Thinking Content (Extended Thinking Models)

```bash
# Extract reasoning chunks from LLM operations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm" and .reasoning != null) | {opId:.opId, reasoning:.reasoning}]'

# Extract final reasoning text only
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .reasoning.final // empty]'
```

### Extract Specific Turn by Index

```bash
# Get turn 3 operations
zcat "$SNAPSHOT" | jq '.opTree.turns[] | select(.index == 3)'

# List all turn indices
zcat "$SNAPSHOT" | jq '[.opTree.turns[].index]'

# Get turn 2, operation 3 (0-based indexing)
zcat "$SNAPSHOT" | jq '.opTree.turns[] | select(.index == 2) | .ops[2]'
```

### Extract Tool Calls by Name

```bash
# Find all calls to a specific tool (e.g., "bigquery__execute_sql")
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .attributes.name == "bigquery__execute_sql")]'

# Find all calls matching a pattern
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .attributes.name | test("web"; i))]'
```

### Extract Latency Information

```bash
# Get latency per operation
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {opId:.opId, kind:.kind, latencyMs:(.endedAt - .startedAt)}] | sort_by(.latencyMs) | reverse'

# Slowest 5 operations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {opId:.opId, kind:.kind, name:.attributes.name, latencyMs:(.endedAt - .startedAt)}] | sort_by(.latencyMs) | reverse | .[0:5]'

# Session duration in seconds
zcat "$SNAPSHOT" | jq '(.opTree.endedAt - .opTree.startedAt) / 1000'
```

### Extract Cost Breakdown

```bash
# Cost per LLM call
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[] | {provider:.provider, model:.model, costUsd:.costUsd, tokens:.tokens}]'

# Total LLM cost
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[].costUsd // 0] | add'
```

### Count Operations by Type

```bash
# Operations breakdown
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[].kind] | group_by(.) | map({kind:.[0], count:length})'
```

### Debug Failed Operations

```bash
# Failed tool operations with details
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .status == "failed") | {name:.attributes.name, request:.request.payload, response:.response.payload, logs:.logs}]'

# Failed LLM operations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm" and .status == "failed")]'

# All failed operations
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.status == "failed")]'

# Extract logs with stack traces
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("stack")) | {severity:.severity, message:.message, stack:.stack}]'
```

### Extract Final Report Content

```bash
# Extract final_report tool output
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .attributes.name == "agent__final_report") | .response.payload]'

# Extract final report from any tool
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.attributes.name | test("final_report"; i)) | .response.payload'
```

### Extract Actual Model Used (Router Providers)

```bash
# Get actual provider/model when using OpenRouter or similar
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "llm") | .accounting[] | {provider:.provider, model:.model, actualProvider:.actualProvider, actualModel:.actualModel}]'
```

### Extract Unique Tool Names

```bash
# All unique tool names used in session
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .attributes.name] | unique'
```

### Extract Session Title and Status

```bash
zcat "$SNAPSHOT" | jq '{sessionTitle:.opTree.sessionTitle, latestStatus:.opTree.latestStatus, success:.opTree.success, error:.opTree.error}'
```

### Decode Base64 Raw Payloads

```bash
# Decode raw LLM request payload (base64)
zcat "$SNAPSHOT" | jq -r '.opTree.turns[].ops[] | select(.kind == "llm") | .request.payload.raw // empty' | base64 -d

# Decode raw tool request payload (base64)
zcat "$SNAPSHOT" | jq -r '.opTree.turns[].ops[] | select(.kind == "tool") | .request.payload.raw // empty' | base64 -d
```

### Extract Request/Response Size Statistics

```bash
# Request/response sizes per operation
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {opId:.opId, kind:.kind, requestSize:.request.size, responseSize:.response.size, truncated:.response.truncated}]'

# Total bytes transferred
zcat "$SNAPSHOT" | jq '{totalRequestBytes:[.opTree.turns[].ops[].request.size | select(. != null)] | add, totalResponseBytes:[.opTree.turns[].ops[].response.size | select(. != null)] | add}'
```

### Extract Session Attributes (Ingress Metadata)

```bash
# Extract session-level attributes (e.g., ingress info from Slack)
zcat "$SNAPSHOT" | jq '.opTree.attributes'

# Extract ingress metadata if present
zcat "$SNAPSHOT" | jq '.opTree.attributes.ingress // "No ingress metadata"'
```

### Extract Shortened/Handle Responses

```bash
# Find operations where responses were shortened or replaced (tool_output handle or upstream truncation)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.response.truncated == true) | {opId:.opId, kind:.kind, name:.attributes.name, truncated:.response.truncated}]'
```

### Extract Unique MCP Servers

```bash
# All unique MCP servers used in session
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool") | .accounting[].mcpServer] | unique'
```

### Extract Human-Readable Duration

```bash
# Session duration in human format (h:m:s)
zcat "$SNAPSHOT" | jq '((.opTree.endedAt - .opTree.startedAt) / 1000) | "\(. / 3600 | floor)h \((. % 3600) / 60 | floor)m \(. % 60 | floor)s"'
```

### Extract Fatal Logs (Session-Stopping Errors)

```bash
# Logs that caused session to stop
zcat "$SNAPSHOT" | jq '[.. | objects | select(has("fatal")) | select(.fatal == true) | {severity:.severity, message:.message, turn:.turn, subturn:.subturn}]'
```

### Verify Accounting vs Totals

```bash
# Compare computed totals with accounting entries
zcat "$SNAPSHOT" | jq '{totals:.opTree.totals, computedFromAccounting:(.opTree.turns[].ops[].accounting[] | select(.type == "llm") | {tokens:.tokens, cost:.costUsd})}'
```

### Extract System Operations

```bash
# All system operations (init/fin)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "system") | {opId:.opId, label:.attributes.label, logs:.logs | length}]'
```

### Extract Tool Errors

```bash
# Tool operations that failed with error messages
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .status == "failed") | {name:.attributes.name, error:.error, response:.response.payload}]'
```

### Faster Log Extraction (Large Snapshots)

```bash
# Faster alternative for logs (structured access, not recursive)
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[].logs[]]'

# All logs with structured access
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | .logs[]] | add | sort_by(.timestamp)'
```

### Pretty-Print Full Snapshot

```bash
# Human-readable full dump
zcat "$SNAPSHOT" | jq > "${TXN_ID}-full.json" . && echo "Written to ${TXN_ID}-full.json"
```

---

## Quick Reference Table

| What You Need        | Command                                                                                 |
| -------------------- | --------------------------------------------------------------------------------------- | ----------------------------------------- |
| Full snapshot        | `zcat "$SNAPSHOT" > out.json`                                                           |
| Session metadata     | `jq '.opTree                                                                            | {id, traceId, agentId, success, totals}'` |
| All turns            | `jq '[.opTree.turns[]]'`                                                                |
| LLM operations       | `jq '[.opTree.turns[].ops[] \| select(.kind == "llm")]'`                                |
| Tool operations      | `jq '[.opTree.turns[].ops[] \| select(.kind == "tool")]'`                               |
| Sub-agents           | `jq '[.opTree.turns[].ops[] \| select(.kind == "session")]'`                            |
| All logs             | `jq '[.. \| objects \| select(has("severity"))]'`                                       |
| WRN/ERR only         | `jq '[.. \| objects \| select(.severity == "WRN" or .severity == "ERR")]'`              |
| LLM accounting       | `jq '[.opTree.turns[].ops[] \| select(.kind == "llm") \| .accounting[]]'`               |
| Tool accounting      | `jq '[.opTree.turns[].ops[] \| select(.kind == "tool") \| .accounting[]]'`              |
| Prompts              | `jq '.opTree.turns[0].attributes.prompts'`                                              |
| Totals               | `jq '.opTree.totals'`                                                                   |
| Reasoning/thinking   | `jq '[.opTree.turns[].ops[] \| select(.reasoning) \| .reasoning]'`                      |
| Specific turn        | `jq '.opTree.turns[] \| select(.index == N)'`                                           |
| Tool by name         | `jq '[.opTree.turns[].ops[] \| select(.attributes.name == "NAME")]'`                    |
| Cost breakdown       | `jq '[.opTree.turns[].ops[].accounting[] \| select(.type == "llm") \| .costUsd]'`       |
| Latency stats        | `jq '[.opTree.turns[].ops[] \| (.endedAt - .startedAt)]'`                               |
| Failed ops           | `jq '[.opTree.turns[].ops[] \| select(.status == "failed")]'`                           |
| Unique tools         | `jq '[.opTree.turns[].ops[] \| select(.kind == "tool") \| .attributes.name] \| unique'` |
| Session title/status | `jq '{sessionTitle:.opTree.sessionTitle, latestStatus:.opTree.latestStatus}'`           |
| Stack traces         | `jq '[.. \| objects \| select(has("stack"))]'`                                          |
| Faster logs          | `jq '[.opTree.turns[].ops[].logs[]]'`                                                   |

---

## File Locations

| Path                                                          | Purpose                      |
| ------------------------------------------------------------- | ---------------------------- |
| `~/.ai-agent/sessions/{UUID}.json.gz`                         | Default session snapshots    |
| `/opt/neda/.ai-agent/sessions/{UUID}.json.gz`                 | Production session snapshots |
| `~/.ai-agent/accounting.jsonl`                                | Accounting ledger (JSONL)    |
| `/home/costa/src/ai-agent.git/neda/bigquery-snapshot-dump.sh` | Existing extraction script   |

### Using Different Snapshot Directories

```bash
# Default location
SNAPSHOT_DIR="${HOME}/.ai-agent/sessions"
SNAPSHOT_DIR="/opt/neda/.ai-agent/sessions"  # Production snapshots

# Set based on environment
SNAPSHOT_DIR="${SNAPSHOT_DIR:-${HOME}/.ai-agent/sessions}"
TXN_ID="008adec0-9000-4700-affe-63d17308fe4e"
SNAPSHOT="${SNAPSHOT_DIR}/${TXN_ID}.json.gz"
```

---

## Related Documentation

- [Snapshots Spec](specs/snapshots.md) - Technical specification
- [Session Tree Source](../src/session-tree.ts) - OpTree implementation
- [Persistence Source](../src/persistence.ts) - File I/O implementation
- [Types Source](../src/types.ts) - Type definitions
