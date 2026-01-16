# Troubleshooting

Common issues and solutions.

---

## Configuration Issues

### Config File Not Found

**Error**: `Configuration file not found`

**Solution**:
1. Check file exists: `ls -la .ai-agent.json`
2. Check current directory
3. Use explicit path: `--config /path/to/.ai-agent.json`

---

### Invalid JSON

**Error**: `SyntaxError: Unexpected token`

**Solution**:
1. Validate JSON: `jq . .ai-agent.json`
2. Check for trailing commas
3. Check for unquoted strings

---

### Environment Variable Not Expanded

**Error**: API key shows as `${OPENAI_API_KEY}`

**Solution**:
1. Check variable is set: `echo $OPENAI_API_KEY`
2. Check `.ai-agent.env` location
3. Verify syntax: `"apiKey": "${OPENAI_API_KEY}"`

---

## Provider Issues

### Invalid API Key

**Error**: `401 Unauthorized` or `Invalid API key`

**Solution**:
1. Verify key in provider dashboard
2. Check for extra whitespace
3. Ensure key has correct permissions

---

### Rate Limited

**Error**: `429 Too Many Requests`

**Solution**:
1. Add retry configuration
2. Add fallback models
3. Implement caching

```yaml
maxRetries: 5
models:
  - openai/gpt-4o
  - anthropic/claude-3-haiku
cache: 1h
```

---

### Model Not Found

**Error**: `Model not found: xyz`

**Solution**:
1. Check model name spelling
2. Verify model availability in your region
3. Check provider documentation for current model names

---

## MCP/Tool Issues

### MCP Server Won't Start

**Error**: `Failed to initialize MCP server`

**Solution**:
1. Check command exists: `which npx`
2. Verify package: `npx -y @package/name --version`
3. Check environment variables
4. Enable MCP tracing: `--trace-mcp`

---

### Tool Timeout

**Error**: `Tool timeout after X ms`

**Solution**:
1. Increase timeout: `--tool-timeout 120000`
2. Check tool is responsive
3. Add to slower queue

```json
{
  "queues": {
    "slow": { "concurrent": 2 }
  },
  "mcpServers": {
    "slowtool": {
      "queue": "slow"
    }
  }
}
```

---

### Tool Not Found

**Error**: `Tool not found: mcp__server__tool`

**Solution**:
1. Check server name in config
2. Verify tool exists: `--dry-run --verbose`
3. Check `toolsAllowed` filtering

---

## Agent Issues

### Agent Hangs

**Symptoms**: No output, no completion

**Solutions**:
1. Add timeout: `timeout 60 ai-agent ...`
2. Check `maxTurns` setting
3. Enable verbose: `--verbose`
4. Check for interactive tools

---

### Empty Response

**Symptoms**: Agent completes but no output

**Solutions**:
1. Check output format matches schema
2. Verify final report in snapshot
3. Check for errors: `2>&1 | grep ERR`

---

### Context Window Exceeded

**Error**: `context window budget exceeded`

**Solutions**:
1. Reduce tool response size: `toolResponseMaxBytes: 8192`
2. Increase context window
3. Reduce conversation history
4. Use summarization

---

### Loop/Stuck Agent

**Symptoms**: Agent keeps calling tools without progress

**Solutions**:
1. Lower `maxTurns`
2. Add explicit instructions to finish
3. Review tool responses for guidance

```yaml
maxTurns: 10
---
Complete your task within the turn limit.
If you have enough information, produce a final answer.
```

---

## Performance Issues

### Slow Responses

**Solutions**:
1. Enable streaming: `--stream`
2. Use faster models for simple tasks
3. Enable caching: `cache: 1h`
4. Reduce tool calls per turn

---

### High Costs

**Solutions**:
1. Use smaller models for simple tasks
2. Enable caching
3. Reduce context with summarization
4. Monitor with accounting file

---

## Getting Help

1. Enable all debugging:
   ```bash
   DEBUG=true CONTEXT_DEBUG=true \
   ai-agent --agent test.ai --verbose --trace-llm --trace-mcp "query" 2> debug.log
   ```

2. Extract session snapshot
3. Check logs for errors
4. Search existing issues

---

## See Also

- [Debugging Guide](Operations-Debugging) - Full debugging workflow
- [Exit Codes](Operations-Exit-Codes) - Exit code reference
