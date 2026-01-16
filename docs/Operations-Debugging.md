# Debugging Guide

Step-by-step workflow for debugging agent issues.

---

## Quick Checklist

1. Enable verbose mode: `--verbose`
2. Check exit code
3. Review stderr logs
4. Examine session snapshot
5. Enable tracing if needed

---

## Step 1: Enable Verbose Mode

```bash
ai-agent --agent myagent.ai --verbose "query"
```

Verbose output includes:
- LLM request/response summaries
- Tool call timing
- Token counts
- Turn progress

---

## Step 2: Check Exit Code

```bash
ai-agent --agent myagent.ai "query"
echo "Exit code: $?"
```

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Success | - |
| 1 | Config error | Check `.ai-agent.json` |
| 2 | LLM error | Check provider credentials |
| 3 | Tool error | Check MCP server |
| 4 | CLI error | Check arguments |

---

## Step 3: Review Logs

Logs go to stderr:

```bash
ai-agent --agent myagent.ai "query" 2> debug.log
cat debug.log | grep -E '\[ERR\]|\[WRN\]'
```

### Log Levels

| Level | Meaning |
|-------|---------|
| `VRB` | Verbose (with --verbose) |
| `WRN` | Warning |
| `ERR` | Error |
| `TRC` | Trace (with --trace-*) |

---

## Step 4: Examine Session Snapshot

Snapshots capture the complete session:

```bash
# Find latest snapshot
ls -lt ~/.ai-agent/sessions/*.json.gz | head -1

# Extract and view
zcat ~/.ai-agent/sessions/abc123.json.gz | jq '.opTree.turns | length'
```

### Key Extractions

```bash
# View errors/warnings
zcat session.json.gz | jq '[.. | objects | select(.severity == "ERR" or .severity == "WRN")]'

# View LLM calls
zcat session.json.gz | jq '.opTree.turns[].ops[] | select(.kind == "llm")'

# View tool calls
zcat session.json.gz | jq '.opTree.turns[].ops[] | select(.kind == "tool")'
```

---

## Step 5: Enable Tracing

### LLM Tracing

```bash
ai-agent --agent myagent.ai --trace-llm "query"
```

Shows:
- Full request headers/body
- Response JSON (pretty-printed)
- Authorization redacted

### MCP Tracing

```bash
ai-agent --agent myagent.ai --trace-mcp "query"
```

Shows:
- MCP server initialization
- Tool discovery
- Tool call payloads and responses
- Server stderr

---

## Common Issues

### Agent Hangs

**Symptoms**: No output, no completion

**Debug**:
```bash
# Check with timeout
timeout 30 ai-agent --agent myagent.ai --verbose "query"
```

**Causes**:
- Tool waiting for input
- Infinite loop (check maxTurns)
- Network timeout

**Fix**: Add `--llm-timeout` and `--tool-timeout`

---

### Empty Response

**Symptoms**: Agent completes but no output

**Debug**:
```bash
# Check session snapshot for final_report
zcat session.json.gz | jq '.opTree.turns[-1].ops[] | select(.kind == "final")'
```

**Causes**:
- LLM returned empty content
- Output format mismatch
- Schema validation failure

**Fix**: Check output format in frontmatter

---

### Tool Not Found

**Symptoms**: `ERR: Tool not found: xxx`

**Debug**:
```bash
ai-agent --agent myagent.ai --dry-run --verbose
```

**Causes**:
- MCP server not configured
- Tool name mismatch
- Server failed to start

**Fix**: Check `mcpServers` in config

---

### Context Window Exceeded

**Symptoms**: `tool failed: context window budget exceeded`

**Debug**:
```bash
CONTEXT_DEBUG=true ai-agent --agent myagent.ai "query"
```

**Causes**:
- Too many tool responses
- Large responses not stored
- Insufficient context window

**Fix**:
- Increase `contextWindow`
- Lower `toolResponseMaxBytes`
- Reduce conversation length

---

### Provider Errors

**Symptoms**: `LLM communication error`

**Debug**:
```bash
ai-agent --agent myagent.ai --trace-llm "query"
```

**Causes**:
- Invalid API key
- Rate limiting
- Model not available

**Fix**:
- Verify credentials
- Add fallback models
- Check provider status

---

## Debug Environment Variables

| Variable | Purpose |
|----------|---------|
| `DEBUG=true` | AI SDK debug mode |
| `CONTEXT_DEBUG=true` | Context guard debugging |

---

## Production Debugging

For production issues:

1. Get session ID from logs
2. Extract snapshot:
   ```bash
   zcat /opt/neda/.ai-agent/sessions/${SESSION_ID}.json.gz > session.json
   ```
3. Analyze locally

---

## See Also

- [Session Snapshots](Operations-Snapshots) - Snapshot analysis
- [Exit Codes](Operations-Exit-Codes) - Exit code reference
- [Troubleshooting](Operations-Troubleshooting) - Common solutions
