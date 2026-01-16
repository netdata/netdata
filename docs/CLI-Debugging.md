# CLI: Debugging

How to debug agent execution. Covers verbose logging, tracing, dry runs, and diagnostic techniques.

---

## Table of Contents

- [Overview](#overview) - Debugging approach and tools
- [Verbose Mode](#verbose-mode) - General execution logging
- [Tracing Options](#tracing-options) - Detailed protocol tracing
- [Dry Run](#dry-run) - Validation without execution
- [Session Inspection](#session-inspection) - Analyzing saved sessions
- [Common Issues](#common-issues) - Troubleshooting patterns
- [See Also](#see-also) - Related pages

---

## Overview

ai-agent provides several debugging levels:

1. **Verbose mode** - High-level execution flow (turns, tokens, timing)
2. **Tracing** - Protocol-level details (LLM requests, MCP calls)
3. **Dry run** - Configuration validation without LLM calls
4. **Session snapshots** - Post-mortem analysis of saved sessions

Start with verbose mode, then add specific traces as needed.

---

## Verbose Mode

### The `--verbose` Flag

| Property | Value       |
| -------- | ----------- |
| Flag     | `--verbose` |
| Type     | Boolean     |
| Default  | `false`     |

**Description**: Show detailed execution logs including timing, token usage, and internal state.

```bash
ai-agent --agent chat.ai --verbose "Test query"
```

**Output includes:**

- Turn numbers and timing
- Token usage per turn
- Tool calls and results
- Model selection and fallbacks
- Context window usage

**Example output:**

```
ts=2025-01-16T10:23:45.123Z level=vrb priority=6 type=llm direction=request turn=1 subturn=0 remote=agent:start message=agent: your query
ts=2025-01-16T10:23:47.456Z level=vrb priority=6 type=llm direction=response turn=1 subturn=0 remote=summary message=requests=1 failed=0, tokens prompt=1234 output=567 cacheR=0 cacheW=0 total=1801, cost total=$0.00000 upstream=$0.00000, latency sum=2333ms avg=2333ms, providers/models: openai/gpt-4o
```

---

## Tracing Options

### LLM Tracing

| Property | Value         |
| -------- | ------------- |
| Flag     | `--trace-llm` |
| Type     | Boolean       |
| Default  | `false`       |

**Description**: Show detailed logs of all LLM API calls. Includes request/response bodies, headers, and timing.

```bash
ai-agent --agent chat.ai --trace-llm "Debug this"
```

**Use when:**

- Debugging model behavior
- Investigating unexpected responses
- Verifying prompt content
- Checking token counts

**Output includes:**

- Full request payload (messages, tools, parameters)
- Response streaming events
- Error details and retry attempts
- Rate limiting information

### MCP Tracing

| Property | Value         |
| -------- | ------------- |
| Flag     | `--trace-mcp` |
| Type     | Boolean       |
| Default  | `false`       |

**Description**: Show detailed logs of all tool calls (MCP protocol). Includes tool inputs, outputs, and errors.

```bash
ai-agent --agent tools.ai --trace-mcp "Use the search tool"
```

**Use when:**

- Debugging tool failures
- Verifying tool parameters
- Checking tool response sizes
- Investigating MCP server issues

**Output includes:**

- Tool call requests with arguments
- Tool responses (truncated if large)
- Timeout and error conditions
- MCP server initialization

### SDK Tracing

| Property | Value         |
| -------- | ------------- |
| Flag     | `--trace-sdk` |
| Type     | Boolean       |
| Default  | `false`       |

**Description**: Dump raw payloads exchanged with the AI SDK. Low-level debugging.

```bash
ai-agent --agent debug.ai --trace-sdk "Test"
```

**Use when:**

- Debugging provider integration issues
- Investigating SDK-level bugs
- Verifying message format conversion

### Slack Tracing

| Property | Value           |
| -------- | --------------- |
| Flag     | `--trace-slack` |
| Type     | Boolean         |
| Default  | `false`         |

**Description**: Show detailed logs of Slack bot communication.

```bash
ai-agent --agent slackbot.ai --slack --trace-slack
```

**Use when:**

- Debugging Slack integration
- Verifying message formatting
- Investigating connection issues

### Combining Traces

Multiple traces can be combined:

```bash
# Full debugging
ai-agent --agent debug.ai --verbose --trace-llm --trace-mcp "Debug everything"

# LLM and MCP together
ai-agent --agent tools.ai --trace-llm --trace-mcp "Debug tool usage"
```

---

## Dry Run

### The `--dry-run` Flag

| Property | Value       |
| -------- | ----------- |
| Flag     | `--dry-run` |
| Type     | Boolean     |
| Default  | `false`     |

**Description**: Validate configuration and setup without executing the agent. No LLM calls are made.

```bash
ai-agent --agent test.ai --dry-run "Test query"
```

**Validates**

- Configuration parsing and agent file syntax
- Model availability
- MCP server connectivity

Note: Tool schema validation occurs during normal agent loading, not during dry-run.

**Use for:**

- CI/CD pipeline validation
- Pre-deployment checks
- Configuration testing
- Syntax verification

**Example:**

```bash
# Validate before deployment
ai-agent --agent production.ai --dry-run "test" && echo "Config valid"
```

---

## Session Inspection

### Saving Sessions

| Property | Value                   |
| -------- | ----------------------- |
| Flag     | `--sessions-dir <path>` |
| Default  | `~/.ai-agent/sessions`  |

**Description**: Directory for saving session snapshots.

```bash
ai-agent --agent chat.ai --sessions-dir ./debug-sessions "Test"
```

### Resuming Sessions

| Property | Value           |
| -------- | --------------- |
| Flag     | `--resume <id>` |

**Description**: Resume a previously interrupted session.

```bash
ai-agent --agent chat.ai --resume abc123
```

### Analyzing Snapshots

Session snapshots are gzipped JSON files containing:

- Complete conversation history
- Tool calls and responses
- Token usage
- Timing information
- Error details

**Extract and inspect:**

```bash
# View session structure
zcat ~/.ai-agent/sessions/abc123.json.gz | jq 'keys'

# Extract LLM operations
zcat ~/.ai-agent/sessions/abc123.json.gz | jq '.opTree.turns[].ops[] | select(.kind == "llm")'

# Find errors
zcat ~/.ai-agent/sessions/abc123.json.gz | jq '[.. | objects | select(.severity == "ERR")]'
```

See [Operations-Snapshots](Operations-Snapshots) for detailed extraction commands.

---

## Common Issues

### Agent File Not Found

**Symptom**: Exit with "agent file not found" error

**Debug**:

```bash
# Check file exists
ls -la myagent.ai

# Verify path resolution
ai-agent --agent ./myagent.ai --dry-run "test"
```

### Model Not Available

**Symptom**: "No models available" or authentication errors

**Debug**:

```bash
# Check with verbose
ai-agent --agent chat.ai --verbose --trace-llm "test"

# Verify environment variables
env | grep -i api_key
```

### Tool Timeout

**Symptom**: Tool calls timing out

**Debug**:

```bash
# Trace MCP to see which tool
ai-agent --agent tools.ai --trace-mcp "use slow tool"

# Increase timeout
ai-agent --agent tools.ai --tool-timeout-ms 300000 "use slow tool"
```

### Unexpected Response Format

**Symptom**: Agent returns wrong format or structure

**Debug**:

```bash
# Trace LLM to see actual response
ai-agent --agent format.ai --trace-llm "return json"

# Validate output against JSON schema (expectedOutputSchema)
ai-agent --agent format.ai --schema @schema.json "return data"
```

### Context Window Exceeded

**Symptom**: "Context too long" or truncation errors

**Debug**:

```bash
# Check token usage with verbose
ai-agent --agent chat.ai --verbose "long conversation"

# Override context window if needed
ai-agent --agent chat.ai --override contextWindow=128000 "long conversation"
```

---

## Diagnostic Checklist

When debugging, work through this checklist:

1. **Start with dry-run**: `--dry-run` validates config without cost
2. **Add verbose**: `--verbose` shows execution flow
3. **Trace specific protocols**: `--trace-llm` or `--trace-mcp` for details
4. **Check session snapshots**: Post-mortem analysis
5. **Verify environment**: API keys, network, file permissions

**Full debug command:**

```bash
ai-agent --agent debug.ai \
  --verbose \
  --trace-llm \
  --trace-mcp \
  --sessions-dir ./debug \
  "Debug this query"
```

---

## See Also

- [CLI](CLI) - CLI overview and quick reference
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents
- [CLI-Overrides](CLI-Overrides) - Runtime overrides
- [Operations-Debugging](Operations-Debugging) - Production debugging
- [Operations-Snapshots](Operations-Snapshots) - Session snapshot analysis
- [Operations-Troubleshooting](Operations-Troubleshooting) - Common problems
