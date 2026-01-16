# Logging

Structured logging system for AI Agent.

---

## Log Levels

| Level | Meaning | When |
|-------|---------|------|
| `VRB` | Verbose | With `--verbose` flag |
| `WRN` | Warning | Recoverable issues |
| `ERR` | Error | Failures requiring attention |
| `TRC` | Trace | With `--trace-*` flags |

---

## Output Destination

- **stdout**: Agent responses only
- **stderr**: All logs, errors, debug info

```bash
# Separate outputs
ai-agent --agent test.ai "query" > response.txt 2> logs.txt
```

---

## Log Format

### TTY (Interactive)

```
[WRN] Tool timeout exceeded: mcp__api__slow_query
[ERR] LLM request failed: timeout after 120000ms
```

With colors:
- `VRB`: Gray
- `WRN`: Yellow
- `ERR`: Red
- `TRC`: Cyan

### Non-TTY (Piped/Files)

Same format without ANSI colors.

---

## Verbose Mode

```bash
ai-agent --agent test.ai --verbose "query"
```

Output includes:

```
[llm] req: openai, gpt-4o, messages 3, 1523 chars
[llm] res: input 1523, output 456, cached 0 tokens, tools 2, latency 2341 ms
[mcp] req: 001 github, search_code
[mcp] res: 001 github, search_code, latency 523 ms, size 12456 chars
[fin] finally: llm requests 2 (tokens: 3046 in, 912 out), mcp requests 3
```

---

## Trace Modes

### LLM Tracing

```bash
ai-agent --agent test.ai --trace-llm "query"
```

Shows:
- Request headers (Authorization redacted)
- Request body (JSON)
- Response body (pretty JSON)
- SSE events for streaming

### MCP Tracing

```bash
ai-agent --agent test.ai --trace-mcp "query"
```

Shows:
- Server initialization
- `tools/list` requests/responses
- `prompts/list` requests/responses
- `callTool` requests/responses
- Server stderr output

---

## Turn Logging

Each turn logs:

```
[turn 1] starting
[turn 1] llm: gpt-4o, 1523 input, 456 output tokens
[turn 1] tools: 2 calls (search_code, get_file_contents)
[turn 1] completed in 3452ms
```

Failed turns:

```
[WRN] turn 2 failed: empty_response, no tools called
```

---

## Error Logging

### Single WRN for Failed Turns

Failed turns emit exactly one warning with:
- Slugged reasons
- LLM response (truncated to ~128KB)

### Single ERR for Session Failure

Session failures emit one error:
- Reason
- Retry attempts made
- Synthetic final report

---

## journald Integration

When running as systemd service:

```bash
journalctl -u ai-agent -f
journalctl -u ai-agent --since "1 hour ago"
journalctl -u ai-agent --grep "ERR"
```

---

## OTLP Export

Export logs to OTLP collector:

```bash
ai-agent --agent test.ai \
  --telemetry-logging-otlp-endpoint http://collector:4317 \
  "query"
```

---

## Log Analysis

### Count Errors

```bash
cat logs.txt | grep -c '\[ERR\]'
```

### Extract Warnings

```bash
cat logs.txt | grep '\[WRN\]' | sort | uniq -c | sort -rn
```

### Find Slow Operations

```bash
cat logs.txt | grep 'latency' | awk -F'latency ' '{print $2}' | sort -rn | head
```

---

## See Also

- [docs/LOGS.md](../docs/LOGS.md) - Detailed logging documentation
- [docs/specs/logging-overview.md](../docs/specs/logging-overview.md) - Technical spec
