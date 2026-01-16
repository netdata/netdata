# Logging

Structured logging system for monitoring and debugging AI Agent sessions.

---

## Table of Contents

- [Overview](#overview) - Logging architecture and concepts
- [Log Levels](#log-levels) - Severity classifications
- [Output Destinations](#output-destinations) - Where logs go
- [Log Formats](#log-formats) - TTY, logfmt, and JSON output
- [Verbose Mode](#verbose-mode) - Detailed operation logging
- [Trace Modes](#trace-modes) - Protocol-level debugging
- [Turn and Event Logging](#turn-and-event-logging) - Session lifecycle events
- [Log Analysis](#log-analysis) - Useful extraction commands
- [Configuration Reference](#configuration-reference) - All logging options
- [Troubleshooting](#troubleshooting) - Common logging issues
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent uses structured logging with:
- **Severity levels**: VRB, WRN, ERR, TRC, THK, FIN
- **Typed entries**: Each log includes turn, subturn, direction, type
- **Multiple sinks**: Console, journald, OTLP export
- **Trace context**: Session and operation IDs for correlation

**Key principle**: All logs go to **stderr**. Only the agent's final output goes to **stdout**.

```bash
# Separate agent output from logs
ai-agent --agent test.ai "query" > response.txt 2> logs.txt
```

---

## Log Levels

| Level | Code | Purpose | When Emitted |
|-------|------|---------|--------------|
| `VRB` | Verbose | Normal operations | With `--verbose` flag |
| `WRN` | Warning | Recoverable issues | Non-fatal problems |
| `ERR` | Error | Fatal failures | Session-stopping errors |
| `TRC` | Trace | Debug data | With `--trace-*` flags |
| `THK` | Thinking | Reasoning output | Extended thinking models |
| `FIN` | Final | Session summary | End of every session |

### Severity Examples

```
[VRB] [llm] res: input 1523, output 456, cached 0 tokens, latency 2341 ms
[WRN] Tool timeout exceeded: mcp__api__slow_query
[ERR] LLM request failed: timeout after 120000ms
[TRC] MCP request: {"method":"tools/call","params":{"name":"search"}}
[THK] <thinking> Let me analyze this step by step...
[FIN] Session complete: 5 turns, 4523 tokens, $0.12
```

---

## Output Destinations

### Console (stderr)

Default output with TTY detection:
- **TTY mode**: Colored output with ANSI codes
- **Non-TTY mode**: Plain text (for piping/files)

### journald Integration

When running as a systemd service, logs integrate with journald:

```bash
# Follow live logs
journalctl -u ai-agent -f

# View logs from last hour
journalctl -u ai-agent --since "1 hour ago"

# Filter by severity (errors only)
journalctl -u ai-agent --grep "ERR"

# JSON output for parsing
journalctl -u ai-agent -o json
```

### OTLP Export

Export structured logs to OpenTelemetry collector:

```bash
ai-agent --agent test.ai \
  --telemetry-logging-otlp-endpoint http://collector:4317 \
  "query"
```

---

## Log Formats

### Rich Format (TTY)

Interactive terminals show colored, formatted output:

```
[VRB] [llm] res: openai/gpt-4o, 1523→456 tokens, 2341ms
[WRN] ⚠ Tool timeout: mcp__github__search_code
[ERR] ✗ LLM request failed: 429 rate limited
```

**Color scheme**:
| Severity | Color |
|----------|-------|
| VRB | Gray |
| WRN | Yellow |
| ERR | Red |
| TRC | Cyan |
| THK | Purple |
| FIN | Green |

### Logfmt Format

Machine-parseable key=value pairs:

```
ts=1699999999999 level=VRB turn=1 subturn=0 dir=response type=llm remote="openai:gpt-4" msg="LLM response received" latency_ms=1234
```

### JSON Format

Full structured output:

```json
{
  "timestamp": 1699999999999,
  "severity": "VRB",
  "turn": 1,
  "subturn": 0,
  "direction": "response",
  "type": "llm",
  "remoteIdentifier": "openai:gpt-4",
  "message": "LLM response received",
  "details": {
    "latency_ms": 1234,
    "input_tokens": 100,
    "output_tokens": 50
  }
}
```

---

## Verbose Mode

Enable with `--verbose` flag:

```bash
ai-agent --agent test.ai --verbose "query"
```

### Verbose Output Includes

**LLM Operations**:
```
[llm] req: openai, gpt-4o, messages 3, 1523 chars
[llm] res: input 1523, output 456, cached 0 tokens, tools 2, latency 2341 ms
```

**Tool Operations**:
```
[mcp] req: 001 github, search_code
[mcp] res: 001 github, search_code, latency 523 ms, size 12456 chars
```

**Session Summary**:
```
[fin] finally: llm requests 2 (tokens: 3046 in, 912 out), mcp requests 3
```

**Turn Progress**:
```
[turn 1] starting
[turn 1] llm: gpt-4o, 1523 input, 456 output tokens
[turn 1] tools: 2 calls (search_code, get_file_contents)
[turn 1] completed in 3452ms
```

---

## Trace Modes

### LLM Tracing

```bash
ai-agent --agent test.ai --trace-llm "query"
```

Shows complete LLM protocol:
- Request headers (Authorization redacted)
- Request body (full JSON)
- Response body (pretty-printed JSON)
- SSE events for streaming responses

### MCP Tracing

```bash
ai-agent --agent test.ai --trace-mcp "query"
```

Shows MCP protocol details:
- Server initialization
- `tools/list` discovery
- `prompts/list` discovery
- `callTool` requests/responses
- Server stderr output

### Combined Tracing

```bash
ai-agent --agent test.ai --trace-llm --trace-mcp "query"
```

---

## Turn and Event Logging

### Session Lifecycle Events

| Event ID | Description |
|----------|-------------|
| `agent:init` | Session initialized |
| `agent:settings` | Configuration summary (verbose) |
| `agent:tools` | Available tools banner |
| `agent:pricing` | Missing pricing warning |
| `agent:start` | User prompt received |
| `agent:fin` | Session finalized |
| `agent:error` | Uncaught exception |

### Turn Events

| Event ID | Description |
|----------|-------------|
| `agent:turn-start` | New LLM turn begins |
| `agent:final-turn` | Final turn detected |
| `agent:context` | Context guard triggered |
| `{provider}:{model}` | LLM request/response |

### Exit Events

| Event ID | Description |
|----------|-------------|
| `agent:EXIT-FINAL-ANSWER` | Normal completion |
| `agent:EXIT-MAX-TURNS-WITH-RESPONSE` | Turn limit with output |
| `agent:EXIT-TOKEN-LIMIT` | Context window exhausted |
| `agent:EXIT-AUTH-FAILURE` | Authentication error |
| `agent:EXIT-QUOTA-EXCEEDED` | Rate/quota limit |
| `agent:EXIT-TOOL-TIMEOUT` | Tool execution timeout |

---

## Log Analysis

### Count Errors and Warnings

```bash
# Count errors
grep -c '\[ERR\]' logs.txt

# Count warnings
grep -c '\[WRN\]' logs.txt
```

### Extract Unique Warnings

```bash
grep '\[WRN\]' logs.txt | sort | uniq -c | sort -rn
```

### Find Slow Operations

```bash
grep 'latency' logs.txt | awk -F'latency ' '{print $2}' | sort -rn | head -10
```

### Extract Turn Summaries

```bash
grep '\[turn' logs.txt | grep 'completed'
```

### Filter by Session ID

```bash
grep 'txnId=abc123' logs.txt
```

---

## Configuration Reference

### CLI Flags

| Flag | Description |
|------|-------------|
| `--verbose` | Enable verbose logging |
| `--trace-llm` | Enable LLM protocol tracing |
| `--trace-mcp` | Enable MCP protocol tracing |
| `--telemetry-logging-otlp-endpoint URL` | OTLP log export endpoint |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `DEBUG=true` | Enable AI SDK debug output |
| `CONTEXT_DEBUG=true` | Enable context guard debugging |

### LogEntry Structure

```typescript
interface LogEntry {
  timestamp: number;              // Unix timestamp (ms)
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN';
  turn: number;                   // Turn index
  subturn: number;                // Tool call index within turn
  path?: string;                  // Hierarchical path (e.g., "1.2.1")
  direction: 'request' | 'response';
  type: 'llm' | 'tool';
  toolKind?: 'mcp' | 'rest' | 'agent' | 'command';
  remoteIdentifier: string;       // "provider:model" or "namespace:tool"
  fatal: boolean;                 // Caused agent stop
  message: string;                // Human readable
  details?: Record<string, any>;  // Structured metadata
  stack?: string;                 // Stack trace (WRN/ERR only)
}
```

---

## Troubleshooting

### Problem: No logs appearing

**Cause**: Logs go to stderr, not stdout.

**Solution**:
```bash
# View stderr in terminal
ai-agent --agent test.ai "query"

# Capture both streams
ai-agent --agent test.ai "query" 2>&1 | tee output.log
```

---

### Problem: Missing verbose output

**Cause**: `--verbose` flag not set.

**Solution**:
```bash
ai-agent --agent test.ai --verbose "query"
```

---

### Problem: Colors not showing

**Cause**: Output is not a TTY (piped or redirected).

**Solution**: Colors are intentionally disabled for non-TTY output. Use `less -R` to view colored output:
```bash
ai-agent --agent test.ai --verbose "query" 2>&1 | less -R
```

---

### Problem: Logs truncated in terminal

**Cause**: Long messages are truncated for readability.

**Solution**: Full content is preserved in session snapshots:
```bash
zcat ~/.ai-agent/sessions/SESSION_ID.json.gz | jq '.opTree.turns[].ops[].logs[]'
```

---

### Problem: Stack traces not appearing

**Cause**: Stack traces only captured for WRN and ERR severity.

**Solution**: Check severity level of the log entry. Stack traces are attached to `.stack` field.

---

### Problem: journald not receiving logs

**Cause**: systemd socket not available or permissions issue.

**Solution**:
1. Verify systemd is running: `systemctl status`
2. Check journal socket: `ls -la /run/systemd/journal/`
3. Verify permissions: `journalctl --verify`

---

## See Also

- [Operations](Operations) - Operations overview
- [Debugging Guide](Operations-Debugging) - Debugging workflow
- [Session Snapshots](Operations-Snapshots) - Complete session capture
- [specs/logging-overview.md](specs/logging-overview.md) - Technical specification
