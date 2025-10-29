# Hierarchical Console Logging Implementation

## Summary
Implemented differentiated logging formats for console vs server modes to improve readability during interactive use while maintaining full structured logging for production monitoring.

## Problem Statement
In `--verbose` mode, the logfmt format produced extremely long log lines with 20+ key-value pairs, making interactive console use difficult to read and follow the execution flow.

## Solution
Created a dual-format logging system that automatically selects the appropriate format based on the execution context:
- **Console Mode**: Simplified, hierarchical format for interactive terminal use
- **Server Mode**: Full structured logfmt for monitoring and debugging in production

## Implementation Details

### 1. New Console Format (`src/logging/console-format.ts`)
- **Hierarchical Display**: Shows execution flow with `turn.subturn` notation
- **Concise Format**: `SEVERITY turn.subturn agent:context`
- **Smart Metric Extraction**:
  - Parses tool parameters from `request_preview` in log details
  - Extracts key metrics (latency, tokens, cost, bytes) from messages
  - Shows provider/model for LLM calls
  - Formats tool calls with inline parameters

### 2. Automatic Format Selection (`src/log-sink-tty.ts`)
- **Console Mode**: Activates when `process.stderr.isTTY` and not in server mode
- **Server Mode**: Uses full logfmt for MCP/Slack headends
- **Fallback**: Uses logfmt when output is piped/redirected
- **Override**: Respects explicit `--telemetry-log-format` settings

### 3. Server Mode Detection (`src/cli.ts`)
- Passes `serverMode: true` for headend operations
- Passes `serverMode: false` for interactive CLI use
- Detects MCP, Slack, and other headend configurations

## Example Output Comparison

### Before (logfmt - verbose and hard to follow):
```
ts=2025-10-28T20:30:45.123Z level=vrb priority=6 type=llm direction=request turn=1 subturn=0 message_id=8f8c1c67 remote=anthropic:claude-3-haiku headend=cli agent=main call_path=main txn_id=3a426ce0 parent_txn_id=3a426ce0 origin_txn_id=3a426ce0 provider=anthropic model=claude-3-haiku severity=VRB mode=cli messages=2 request_bytes=1027 final_turn=false reasoning=unset message="LLM request prepared"
```

### After (console format - clear hierarchy):
```
VRB 1.0 main: llm request anthropic/claude-3-haiku
VRB 1.0 main: llm response: 1500ms, tokens:806/79, $0.00030
VRB 1.1 main:final_report(status:success, content:hello)
VRB 1.1 main:final_report: 11b, 1ms
```

## Key Features

### Hierarchical Structure
- **Turn.Subturn**: `1.0`, `1.1`, `2.0` - shows execution flow
- **Parent Path**: Visible through call path when using subagents
- **Agent Context**: Immediately shows which agent is executing

### Event-Specific Formatting
1. **Agent Lifecycle**:
   - `VRB 0.0 main: starting`
   - `VRB 0.0 main: completed`

2. **LLM Operations**:
   - Request: `VRB 1.0 main: llm request provider/model`
   - Response: `VRB 1.0 main: llm response: 1500ms, tokens:806/79, $0.00030`

3. **Tool Calls**:
   - Request: `VRB 1.1 main:tool_name(param1:value1, param2:value2)`
   - Response: `VRB 1.1 main:tool_name: 256b, 10ms`

4. **Progress Updates**:
   - Shown with current agent and status

### Color Coding (TTY only)
- **Gray**: VRB (verbose) logs
- **Blue**: LLM operations
- **Green**: Tool operations
- **Yellow**: Warnings (WRN)
- **Red**: Errors (ERR)
- **Cyan**: Final summaries (FIN)

## Benefits

1. **Improved Readability**:
   - Single-line format is much easier to scan
   - Key information extracted and highlighted
   - Hierarchical structure shows execution flow

2. **Preserved Functionality**:
   - Full structured logging for production monitoring
   - All telemetry and accounting data preserved
   - Backward compatible with existing tooling

3. **Smart Detection**:
   - Automatic format selection based on context
   - No configuration needed for common use cases
   - Respects user overrides when specified

## Testing
- ✅ Build and lint checks pass
- ✅ Format selection based on TTY detection
- ✅ Metrics extraction from various message formats
- ✅ Tool parameter extraction from log details
- ✅ Color coding in TTY mode
- ✅ Fallback to logfmt when appropriate

## Usage

### Interactive Console (automatic)
```bash
# In a real terminal, automatically uses console format
./ai-agent --verbose --models anthropic/claude-3-haiku 'You are helpful' 'Say hello'
```

### Server Mode (automatic)
```bash
# Automatically uses logfmt for structured logging
./ai-agent --mcp stdio
```

### Force Specific Format
```bash
# Force logfmt even in TTY
./ai-agent --telemetry-log-format logfmt --verbose ...

# Force JSON format
./ai-agent --telemetry-log-format json --verbose ...
```

## Files Modified
- `src/logging/console-format.ts` - New console formatter
- `src/logging/structured-logger.ts` - Added console format support
- `src/log-sink-tty.ts` - Format selection logic
- `src/cli.ts` - Server mode detection

## Future Improvements
1. Add configuration for console format customization
2. Support for custom color schemes
3. Option to show/hide specific metrics
4. Interactive filtering based on severity or agent