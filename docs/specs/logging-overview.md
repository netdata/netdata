# Logging Overview

## TL;DR
Structured logging with severity levels, multiple output formats (logfmt, JSON, rich TTY), and sinks (console, journald, file). All logs enriched with trace context.

## Source Files
- `src/logging/structured-logger.ts` - Logger implementation
- `src/logging/structured-log-event.ts` - Event structure
- `src/logging/message-ids.ts` - Log message identifiers
- `src/logging/rich-format.ts` - Rich terminal formatting
- `src/logging/logfmt.ts` - logfmt output format
- `src/logging/journald-sink.ts` - systemd journald sink
- `src/logging/console-format.ts` - Console formatting
- `src/types.ts:108-154` - LogEntry definition
- `src/log-formatter.ts` - Formatter utilities
- `src/log-sink-tty.ts` - TTY sink implementation

## LogEntry Structure

**Location**: `src/types.ts:108-154`

```typescript
interface LogEntry {
  timestamp: number;                    // Unix timestamp (ms)
  severity: 'VRB' | 'WRN' | 'ERR' | 'TRC' | 'THK' | 'FIN';
  turn: number;                         // Sequential turn ID
  subturn: number;                      // Sequential tool ID within turn
  path?: string;                        // Hierarchical path (e.g., 1.2.1)
  direction: 'request' | 'response';    // Request or response
  type: 'llm' | 'tool';                 // Operation type
  toolKind?: 'mcp' | 'rest' | 'agent' | 'command';
  remoteIdentifier: string;             // 'provider:model' or 'namespace:tool'
  fatal: boolean;                       // Caused agent stop
  message: string;                      // Human readable message
  bold?: boolean;                       // Emphasis hint for TTY
  headendId?: string;                   // Multi-headend context

  // Tracing fields
  agentId?: string;
  callPath?: string;
  txnId?: string;
  parentTxnId?: string;
  originTxnId?: string;
  agentPath?: string;
  turnPath?: string;

  // Planning fields
  'max_turns'?: number;
  'max_subturns'?: number;

  // Structured details
  details?: Record<string, LogDetailValue>;
  stack?: string;                       // Captured stack trace

  // Raw payloads
  llmRequestPayload?: LogPayload;
  llmResponsePayload?: LogPayload;
  toolRequestPayload?: LogPayload;
  toolResponsePayload?: LogPayload;
}
```

## Severity Levels

| Level | Code | Purpose |
|-------|------|---------|
| VRB | Verbose | Normal operation info |
| WRN | Warning | Non-fatal issues |
| ERR | Error | Fatal errors |
| TRC | Trace | Debug tracing |
| THK | Thinking | Reasoning output |
| FIN | Final | End-of-run summary |

## Log Categories

### LLM Logs (type: 'llm')
- Request preparation
- Response received
- Token usage
- Cost information
- Retry decisions
- Context guard events

### Tool Logs (type: 'tool')
- Tool invocation
- Execution results
- Timeouts
- Size cap events
- Progress updates

### System Logs
- Session initialization
- Configuration summary
- Tools banner
- Exit codes
- Final summary

## Output Formats

### Rich Format (TTY)
**Location**: `src/logging/rich-format.ts`

**Features**:
- ANSI color coding by severity
- Emoji indicators
- Indentation for hierarchy
- Bold emphasis for important logs
- Timestamp formatting
- Truncation for long messages

**Color Scheme**:
- VRB: Gray
- WRN: Yellow
- ERR: Red
- TRC: Cyan
- THK: Purple
- FIN: Green

### Logfmt Format
**Location**: `src/logging/logfmt.ts`

**Features**:
- Key=value pairs
- Machine-parseable
- Flat structure
- String escaping
- Numeric formatting

**Example**:
```
ts=1699999999999 level=VRB turn=1 subturn=0 dir=response type=llm remote="openai:gpt-4" msg="LLM response received" latency_ms=1234
```

### JSON Format
**Features**:
- Full structured output
- Nested details
- Array support
- Type preservation

**Example**:
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

## Log Sinks

### Console Sink
**Location**: `src/logging/console-format.ts`

- Direct stderr output with rich/logfmt/json rendering
- ANSI color + verbose options
- Automatic TTY detection

### Journald Sink
**Location**: `src/logging/journald-sink.ts`

- systemd journal integration when `/run/systemd/journal` available
- Structured fields + priority mapping
- Falls back to logfmt when journald unavailable or disabled at runtime

### Callback Sink
- `onEvent(type='log')` callback passed via `AIAgentSessionConfig`
- Custom processing / streaming to external systems
- Receives the same enriched entries as built-in sinks

## Log Enrichment

All logs enriched with:
1. **Trace Context**: agentId, callPath, txnId, parentTxnId, originTxnId
2. **Hierarchy**: turn, subturn, turnPath
3. **Planning**: max_turns, max_subturns
4. **Headend**: headendId
5. **Stack Trace**: Captured for WRN/ERR

**Location**: `src/ai-agent.ts:873-944` (`addLog` method)

## Key Log Events

### Session Lifecycle
- `agent:init` - Session initialized
- `agent:settings` - Configuration (verbose)
- `agent:tools` - Available tools
- `agent:pricing` - Missing pricing warning
- `agent:start` - User prompt (verbose)
- `agent:fin` - Session finalized
- `agent:error` - Uncaught exception

### Turn Events
- `agent:turn-start` - A new LLM turn begins (VRB)
- `agent:final-turn` - Final turn detected (WRN)
- `agent:context` - Context guard events
- `{provider}:{model}` - LLM request/response
- `agent:text-extraction` - Parsed a final report candidate from assistant text/tool output (still pending)
- `agent:fallback-report` - Pending fallback accepted because retries exhausted on the final turn
- `agent:final-report-accepted` - Final report committed; `details.source` ∈ {`tool-call`,`text-fallback`,`tool-message`,`synthetic`}
- `agent:failure-report` - Synthetic failure report generated when no valid final report was produced

### Tool Events
- `{namespace}:{tool}` - Tool execution
- `agent:progress` - Progress update
- `agent:title` - Title change
- `agent:batch` - Batch execution

### Exit Codes
- `agent:EXIT-FINAL-ANSWER`
- `agent:EXIT-MAX-TURNS-WITH-RESPONSE`
- `agent:EXIT-AUTH-FAILURE`
- `agent:EXIT-QUOTA-EXCEEDED`
- `agent:EXIT-TOOL-TIMEOUT`
- etc.

## Configuration

### Structured Logger Options
```typescript
createStructuredLogger({
  formats: ['console', 'logfmt'],
  color: process.stdout.isTTY,
  verbose: true,
});
```

- `formats`: preference list (`'journald' | 'logfmt' | 'json' | 'console'`). First available format wins; `'none'` disables emission.
- `color` / `verbose`: influence console output only.
- `logfmtWriter` / `jsonWriter` / `consoleWriter`: optional sinks for custom transports.

### Telemetry Logging Defaults

- When no explicit `formats` are provided, the logger consults `telemetry.logging.formats` from the runtime telemetry config (`src/telemetry/runtime-config.ts`).
- Additional `telemetry.logging.extra` and `telemetry.logging.otlp` options control OTLP log export (outside the structured logger scope).

### Trace Flags (`AIAgentSessionConfig`)
```typescript
{
  traceLLM?: boolean;
  traceMCP?: boolean;
  traceSdk?: boolean;
  verbose?: boolean;
}
```

- Trace flags control **extra TRC diagnostics** (SDK request/response dumps, MCP traces, verbose session logs).
- **LLM request/response payloads are captured unconditionally** and attached to `llmRequestPayload` / `llmResponsePayload` for logs, then encoded into opTree snapshots as base64 under `payload.raw` (full HTTP/SSE body capture) and `payload.sdk` (serialized SDK request/response for verification). If `payload.raw` ever contains `[unavailable]`, treat it as a capture bug. Trace flags only add TRC lines.
- Tool payload capture remains tool-specific and may still depend on tracing hooks.
- `verbose` toggles additional session logs (settings, prompts, etc.) for human troubleshooting.

## Business Logic Coverage (Verified 2025-11-16)

- **Severity mapping**: `structured-logger` maps VRB/THK/TRC/WRN/ERR/FIN to both console and OTLP severities so downstream log systems can filter consistently (`src/logging/structured-logger.ts:40-120`).
- **Payload capture**: LLM payloads are attached to `llmRequestPayload` / `llmResponsePayload` for every turn, with no truncation. Trace flags only affect extra diagnostic TRC lines (`src/llm-client.ts`, `src/ai-agent.ts`).
- **FIN summaries**: Every session ends with `FIN` logs summarizing counts/costs and includes a JSON payload inside `details` for machine parsing (`src/ai-agent.ts:2100-2180`).
- **Headend attribution**: Headends inject `headendId` into every log entry they emit, allowing multi-headend deployments to filter logs per surface (`src/headends/*-headend.ts:180-260`).

## Telemetry Integration

Logs feed into:
- OpenTelemetry traces (span events)
- Metrics (counters, histograms)
- OpTree (hierarchical attachment)

## Test Coverage

**Unit Tests**:
- Format output correctness
- Severity filtering
- Detail serialization
- Stack trace capture

**Integration Tests**:
- End-to-end log flow
- Multi-sink output
- Callback invocation

**Gaps**:
- Journald integration tests
- Log rotation scenarios
- Performance under load

## Troubleshooting

### Missing logs
- Check severity filter level
- Check callback registration
- Verify onEvent callback doesn't throw

### Truncated messages
- Check format configuration
- Check terminal width
- Check max message length settings

### Stack traces not appearing
- Only captured for WRN/ERR
- Check severity level
- Verify Error object propagation

### High memory usage
- Log retention settings
- Payload capture disabled
- Flush frequency

### Journald not receiving logs
- Check systemd socket
- Verify journal permissions
- Check priority mapping

### Trace fields missing
- Check session trace context
- Verify enrichment in addLog
- Check callback receives enriched entry
