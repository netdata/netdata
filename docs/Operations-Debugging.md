# Debugging Guide

Step-by-step workflow for diagnosing and resolving AI Agent issues.

---

## Table of Contents

- [Quick Checklist](#quick-checklist) - Fast diagnostic steps
- [Step 1: Validate Configuration](#step-1-validate-configuration) - Check before running
- [Step 2: Enable Verbose Mode](#step-2-enable-verbose-mode) - See what's happening
- [Step 3: Check Exit Code](#step-3-check-exit-code) - Understand the outcome
- [Step 4: Review Logs](#step-4-review-logs) - Find warnings and errors
- [Step 5: Examine Session Snapshot](#step-5-examine-session-snapshot) - Deep analysis
- [Step 6: Enable Tracing](#step-6-enable-tracing) - Protocol-level debugging
- [Common Issues](#common-issues) - Frequently encountered problems
- [Debug Environment Variables](#debug-environment-variables) - Advanced debugging
- [Production Debugging](#production-debugging) - Server-side troubleshooting
- [See Also](#see-also) - Related documentation

---

## Quick Checklist

When an agent isn't working as expected:

1. **Validate config**: `ai-agent --agent myagent.ai --dry-run`
2. **Enable verbose**: `ai-agent --agent myagent.ai --verbose "query"`
3. **Check exit code**: `echo $?` after the command
4. **Review stderr**: Look for `[WRN]` and `[ERR]` entries
5. **Examine snapshot**: Extract from `~/.ai-agent/sessions/`
6. **Enable tracing**: `--trace-llm` and `--trace-mcp` for protocol details

---

## Step 1: Validate Configuration

Before running with a real query, validate the configuration:

```bash
ai-agent --agent myagent.ai --dry-run
```

**What to check**:
- Frontmatter parses without errors
- Model chain resolves correctly
- Tools are discovered and listed
- No missing environment variables

**Example output**:
```
Configuration valid.
Models: openai/gpt-4o -> anthropic/claude-3-haiku (fallback)
Tools: 12 tools from 3 MCP servers
```

---

## Step 2: Enable Verbose Mode

Run with verbose output to see turn-by-turn progress:

```bash
ai-agent --agent myagent.ai --verbose "your query here"
```

**Verbose output shows**:
- LLM request/response summaries
- Tool call timing and sizes
- Token counts per turn
- Turn progression

**Example verbose output**:
```
[llm] req: openai, gpt-4o, messages 3, 1523 chars
[llm] res: input 1523, output 456, cached 0 tokens, tools 2, latency 2341 ms
[mcp] req: 001 github, search_code
[mcp] res: 001 github, search_code, latency 523 ms, size 12456 chars
[turn 1] completed in 3452ms
```

---

## Step 3: Check Exit Code

After running, check the exit code:

```bash
ai-agent --agent myagent.ai "query"
echo "Exit code: $?"
```

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Success | Agent completed normally |
| 1 | Configuration Error | Check config file and frontmatter |
| 2 | LLM Error | Check provider credentials and status |
| 3 | Tool Error | Check MCP server configuration |
| 4 | CLI Error | Check command line arguments |

See [Exit Codes](Operations-Exit-Codes) for detailed reference.

---

## Step 4: Review Logs

Logs go to stderr. Capture and analyze them:

```bash
ai-agent --agent myagent.ai "query" 2> debug.log
```

### Filter for Problems

```bash
# Show only warnings and errors
grep -E '\[WRN\]|\[ERR\]' debug.log

# Show errors only
grep '\[ERR\]' debug.log

# Count by severity
grep -o '\[WRN\]\|\[ERR\]' debug.log | sort | uniq -c
```

### Log Levels

| Level | Meaning | Action |
|-------|---------|--------|
| `VRB` | Verbose info | Normal operation (with --verbose) |
| `WRN` | Warning | Review but may not be fatal |
| `ERR` | Error | Investigate immediately |
| `TRC` | Trace | Protocol details (with --trace-*) |

---

## Step 5: Examine Session Snapshot

Every session saves a complete snapshot for post-mortem analysis.

### Find the Latest Snapshot

```bash
ls -lt ~/.ai-agent/sessions/*.json.gz | head -1
```

### Extract Key Information

```bash
SNAPSHOT=~/.ai-agent/sessions/YOUR_SESSION_ID.json.gz

# View session summary
zcat "$SNAPSHOT" | jq '{
  traceId: .opTree.traceId,
  agentId: .opTree.agentId,
  success: .opTree.success,
  error: .opTree.error,
  turns: (.opTree.turns | length),
  totals: .opTree.totals
}'

# View errors and warnings
zcat "$SNAPSHOT" | jq '[.. | objects | select(.severity == "ERR" or .severity == "WRN")]'

# View last turn (where failure usually occurs)
zcat "$SNAPSHOT" | jq '.opTree.turns[-1]'

# View LLM calls
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "llm")'

# View tool calls
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.kind == "tool")'
```

See [Session Snapshots](Operations-Snapshots) for complete extraction guide.

---

## Step 6: Enable Tracing

For protocol-level debugging, enable tracing:

### LLM Tracing

```bash
ai-agent --agent myagent.ai --trace-llm "query"
```

Shows:
- Full request headers (auth redacted)
- Complete request body JSON
- Response body (pretty-printed)
- SSE events for streaming

### MCP Tracing

```bash
ai-agent --agent myagent.ai --trace-mcp "query"
```

Shows:
- MCP server initialization
- Tool discovery (`tools/list`)
- Tool call payloads and responses
- Server stderr output

### Combined Tracing

```bash
ai-agent --agent myagent.ai --trace-llm --trace-mcp "query" 2> full-trace.log
```

---

## Common Issues

### Agent Hangs (No Output, No Completion)

**Symptoms**: Agent runs but produces no output and doesn't complete.

**Debug**:
```bash
timeout 60 ai-agent --agent myagent.ai --verbose "query"
```

**Common causes**:
- Tool waiting for input (interactive tool)
- Infinite loop (check maxTurns setting)
- Network timeout to provider

**Solutions**:
- Add `--llm-timeout 60000` to limit LLM call time
- Add `--tool-timeout 30000` to limit tool execution
- Set `maxTurns: 10` in frontmatter

---

### Empty Response (Agent Completes but No Output)

**Symptoms**: Exit code 0 but stdout is empty.

**Debug**:
```bash
# Check for final report in snapshot
zcat "$SNAPSHOT" | jq '.opTree.turns[-1].ops[] | select(.kind == "final" or .attributes.name | test("final_report"))'
```

**Common causes**:
- LLM returned empty content
- Output format mismatch (expecting JSON, got text)
- Schema validation failure

**Solutions**:
- Check `output` schema in frontmatter
- Verify expected output format
- Review LLM response in snapshot

---

### Tool Not Found

**Error**: `ERR: Tool not found: mcp__server__toolname`

**Debug**:
```bash
ai-agent --agent myagent.ai --dry-run --verbose
```

**Common causes**:
- MCP server not configured in `.ai-agent.json`
- Tool name mismatch (case sensitivity)
- Server failed to start

**Solutions**:
- Verify `mcpServers` configuration
- Check tool name in server output
- Test MCP server independently

---

### Context Window Exceeded

**Error**: `tool failed: context window budget exceeded`

**Debug**:
```bash
CONTEXT_DEBUG=true ai-agent --agent myagent.ai "query"
```

**Common causes**:
- Too many tool responses accumulating
- Large responses not being stored to disk
- Insufficient context window for model

**Solutions**:
- Increase `contextWindow` in frontmatter
- Lower `toolResponseMaxBytes` to store large responses
- Reduce conversation length

---

### Provider Errors (401, 429, 500)

**Error**: `LLM communication error: 401 Unauthorized`

**Debug**:
```bash
ai-agent --agent myagent.ai --trace-llm "query"
```

**Common causes**:
- Invalid or expired API key
- Rate limiting (429)
- Model not available in your plan
- Provider outage

**Solutions**:
- Verify API key: `echo $OPENAI_API_KEY | head -c 10`
- Check provider status page
- Add fallback models
- Implement retry with backoff

---

### MCP Server Won't Start

**Error**: `Failed to initialize MCP server: xyz`

**Debug**:
```bash
ai-agent --agent myagent.ai --trace-mcp "query"
```

**Common causes**:
- Command not found (`npx` not in PATH)
- Package not installed
- Missing environment variables

**Solutions**:
1. Test server independently:
   ```bash
   npx -y @package/server --help
   ```
2. Check environment variables
3. Verify command in configuration

---

### Tool Timeout

**Error**: `Tool timeout after 30000 ms`

**Debug**:
```bash
ai-agent --agent myagent.ai --verbose --trace-mcp "query"
```

**Common causes**:
- Tool operation takes too long
- Network issues to external service
- Tool deadlock

**Solutions**:
- Increase timeout: `--tool-timeout 120000`
- Configure queue with longer timeout
- Check tool service health

---

## Debug Environment Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `DEBUG=true` | AI SDK debug mode | Shows SDK internals |
| `CONTEXT_DEBUG=true` | Context guard debugging | Shows token budget tracking |

### Full Debug Session

```bash
DEBUG=true \
CONTEXT_DEBUG=true \
ai-agent --agent myagent.ai \
  --verbose \
  --trace-llm \
  --trace-mcp \
  "debug this query" 2> full-debug.log
```

---

## Production Debugging

### Get Session from Production Server

```bash
# Find session ID in logs
journalctl -u ai-agent | grep "session started" | tail -1

# Copy snapshot locally
scp server:/opt/neda/.ai-agent/sessions/${SESSION_ID}.json.gz .

# Analyze locally
zcat ${SESSION_ID}.json.gz | jq '.opTree.totals'
```

### Compare Sessions

```bash
# Extract metrics from two sessions
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

### Monitor Live Sessions

```bash
# Follow AI Agent logs
journalctl -u ai-agent -f

# Filter for errors
journalctl -u ai-agent -f | grep -E '\[ERR\]|\[WRN\]'
```

---

## See Also

- [Logging](Operations-Logging) - Logging system reference
- [Session Snapshots](Operations-Snapshots) - Snapshot extraction guide
- [Exit Codes](Operations-Exit-Codes) - Exit code reference
- [Troubleshooting](Operations-Troubleshooting) - Problem/cause/solution reference
