# Troubleshooting

Problem/Cause/Solution reference for AI Agent issues.

---

## Table of Contents

- [Configuration Issues](#configuration-issues) - Config file and frontmatter problems
- [Provider Issues](#provider-issues) - LLM provider errors
- [MCP and Tool Issues](#mcp-and-tool-issues) - Tool execution problems
- [Agent Behavior Issues](#agent-behavior-issues) - Runtime behavior problems
- [Context and Token Issues](#context-and-token-issues) - Context window problems
- [Output Issues](#output-issues) - Missing or wrong output
- [Performance Issues](#performance-issues) - Speed and cost concerns
- [Network Issues](#network-issues) - Connectivity problems
- [Getting Help](#getting-help) - Debug data collection
- [See Also](#see-also) - Related documentation

---

## Configuration Issues

### Config File Not Found

**Problem**: `Configuration file not found`

**Cause**: `.ai-agent.json` not in current directory or parent directories.

**Solution**:

```bash
# Check file exists
ls -la .ai-agent.json

# Specify explicit path
ai-agent --config /path/to/.ai-agent.json --agent test.ai "query"

# Check working directory
pwd
```

---

### Invalid JSON in Config

**Problem**: `SyntaxError: Unexpected token in JSON`

**Cause**: Malformed JSON in configuration file.

**Solution**:

```bash
# Validate JSON syntax
jq . .ai-agent.json

# Common issues:
# - Trailing commas: {"key": "value",}
# - Unquoted keys: {key: "value"}
# - Single quotes: {'key': 'value'}
```

---

### Invalid YAML in Frontmatter

**Problem**: `Failed to parse agent frontmatter`

**Cause**: YAML syntax error in agent file frontmatter.

**Solution**:

```bash
# Extract and validate frontmatter
head -50 myagent.ai | sed -n '/^---$/,/^---$/p'

# Common issues:
# - Incorrect indentation (use spaces, not tabs)
# - Missing colon after key
# - Unquoted special characters
```

---

### Unknown Frontmatter Key

**Problem**: `Unknown frontmatter key: unknownKey`

**Cause**: Typo or invalid property name in frontmatter.

**Solution**:

1. Check spelling against valid keys
2. Use `--dry-run` to validate
3. Review frontmatter documentation

Valid keys: `models`, `maxTurns`, `maxToolCallsPerTurn`, `toolResponseMaxBytes`, `cache`, `toolsAllowed`, etc.

---

### Environment Variable Not Expanded

**Problem**: API key shows as literal `${OPENAI_API_KEY}`

**Cause**: Variable not set or incorrect syntax.

**Solution**:

```bash
# Check variable is set
echo $OPENAI_API_KEY | head -c 10

# Check .ai-agent.env file location
ls -la .ai-agent.env
ls -la ~/.ai-agent/ai-agent.env

# Correct syntax in config
"apiKey": "${OPENAI_API_KEY}"  # Correct
"apiKey": "$OPENAI_API_KEY"    # Wrong - missing braces
```

---

## Provider Issues

### Invalid API Key (401)

**Problem**: `LLM communication error: 401 Unauthorized`

**Cause**: API key is invalid, expired, or missing.

**Solution**:

```bash
# Verify key is set (show first 10 chars)
echo $OPENAI_API_KEY | head -c 10

# Test key directly
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"

# Check for whitespace
printf '%q\n' "$OPENAI_API_KEY" | head -c 20
```

---

### Rate Limited (429)

**Problem**: `LLM request failed: 429 Too Many Requests`

**Cause**: Provider rate limit exceeded.

**Solution**:

1. Add retry configuration:

```yaml
---
maxRetries: 5
---
```

2. Add fallback models:

```yaml
---
models:
  - openai/gpt-4o
  - anthropic/claude-3-haiku
---
```

3. Enable caching:

```yaml
---
cache: 1h
---
```

---

### Model Not Found

**Problem**: `Model not found: xyz` or `Invalid model`

**Cause**: Model name incorrect or not available.

**Solution**:

```bash
# Check model format
# Format: provider/model-name
# Examples: openai/gpt-4o, anthropic/claude-3-opus

# List available models (OpenAI)
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY" | jq '.data[].id'

# Verify in dry-run
ai-agent --agent myagent.ai --dry-run
```

---

### Provider Timeout

**Problem**: `Request timeout after 120000ms`

**Cause**: LLM taking too long to respond.

**Solution**:

```bash
# Increase LLM timeout
ai-agent --agent myagent.ai --llm-timeout-ms 180000 "query"

# Check provider status
# https://status.openai.com/
# https://status.anthropic.com/
```

---

### All Retries Exhausted

**Problem**: `All retries exhausted`

**Cause**: Provider consistently failing despite retries.

**Solution**:

1. Add fallback models
2. Check provider status
3. Increase retry count:

```yaml
---
maxRetries: 5
models:
  - openai/gpt-4o
  - anthropic/claude-3-haiku # Fallback
---
```

---

## MCP and Tool Issues

### MCP Server Won't Start

**Problem**: `Failed to initialize MCP server: servername`

**Cause**: Server command not found or execution failed.

**Solution**:

```bash
# Test server independently
npx -y @modelcontextprotocol/server-filesystem --help

# Check command in config
jq '.mcpServers.servername.command' .ai-agent.json

# Check PATH
which npx
echo $PATH

# Enable MCP tracing
ai-agent --agent myagent.ai --trace-mcp "query"
```

---

### Tool Timeout

**Problem**: `Tool timeout after 30000 ms`

**Cause**: Tool execution taking too long.

**Solution**:

```bash
# Increase default timeout
ai-agent --agent myagent.ai --tool-timeout-ms 120000 "query"
```

```json
{
  "mcpServers": {
    "slowserver": {
      "command": "...",
      "queue": "slow"
    }
  },
  "queues": {
    "slow": {
      "concurrent": 2
    }
  }
}
```

---

### Tool Not Found

**Problem**: `Tool not found: mcp__server__toolname`

**Cause**: Tool not available or filtered out.

**Solution**:

```bash
# List available tools
ai-agent --agent myagent.ai --dry-run --verbose

# Check toolsAllowed filter
# If set, only listed tools are available

# Check server is configured
jq '.mcpServers' .ai-agent.json
```

---

### Tool Returned Error

**Problem**: `Tool execution failed: error message`

**Cause**: Tool encountered an error during execution.

**Solution**:

```bash
# Enable MCP tracing to see request/response
ai-agent --agent myagent.ai --trace-mcp "query"

# Check tool error in snapshot
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | select(.kind == "tool" and .status == "failed")]'
```

---

### MCP Server Disconnect

**Problem**: `MCP server disconnected unexpectedly`

**Cause**: Server process crashed or was killed.

**Solution**:

```bash
# Check MCP tracing for errors
ai-agent --agent myagent.ai --trace-mcp "query"

# Check server stderr output (in trace)
# Look for memory errors, uncaught exceptions

# Increase server timeout
# Some servers need longer initialization
```

---

## Agent Behavior Issues

### Agent Hangs (No Output)

**Problem**: Agent runs but produces no output and doesn't complete.

**Cause**: Tool waiting for input, infinite loop, or network hang.

**Solution**:

```bash
# Add timeout
timeout 60 ai-agent --agent myagent.ai --verbose "query"

# Check maxTurns (prevent infinite loops)
# Add in frontmatter: maxTurns: 10

# Check for interactive tools (stdin waiting)
```

---

### Agent Loops Without Progress

**Problem**: Agent keeps calling tools but doesn't finish.

**Cause**: LLM not recognizing completion criteria.

**Solution**:

1. Lower `maxTurns`:

```yaml
---
maxTurns: 10
---
```

2. Add explicit completion instructions:

```yaml
---
maxTurns: 15
---
Complete your task within the turn limit.
When you have enough information, produce a final answer immediately.
Do not continue gathering data if you already have what you need.
```

---

### Wrong Tool Being Called

**Problem**: Agent calls incorrect or unnecessary tools.

**Cause**: Unclear instructions or tool descriptions.

**Solution**:

1. Add explicit tool guidance in prompt
2. Use `toolsAllowed` to restrict available tools:

```yaml
---
toolsAllowed:
  - mcp__github__search_code
  - mcp__github__get_file
---
```

---

### Sub-Agent Not Starting

**Problem**: Parent agent calls sub-agent but it doesn't execute.

**Cause**: Configuration or tool availability issue.

**Solution**:

```bash
# Verify sub-agent tools available
ai-agent --agent parent.ai --dry-run --verbose

# Check if agent tool is properly configured
# Sub-agent calls appear as kind="session" in snapshot
```

---

## Context and Token Issues

### Context Window Exceeded

**Problem**: `tool failed: context window budget exceeded`

**Cause**: Too much data accumulated in conversation.

**Solution**:

```bash
# Enable context debugging
CONTEXT_DEBUG=true ai-agent --agent myagent.ai "query"
```

1. Reduce tool response size:

```yaml
---
toolResponseMaxBytes: 8192
---
```

2. Increase context window (if model supports):

```yaml
---
contextWindow: 128000
---
```

3. Reduce `maxTurns` to limit accumulation

---

### Token Budget Exhausted

**Problem**: Tool responses being stored even when small.

**Cause**: Running out of token budget dynamically.

**Solution**:

```bash
# Check what's consuming budget
CONTEXT_DEBUG=true ai-agent --agent myagent.ai "query"

# Review tool_output handles in verbose mode
ai-agent --agent myagent.ai --verbose "query"
```

---

### Cache Not Working

**Problem**: Repeated requests not hitting cache.

**Cause**: Cache configuration or provider support issue.

**Solution**:

1. Verify cache is enabled:

```yaml
---
cache: 1h
---
```

2. Check provider supports caching (Anthropic does, some don't)

3. Verify in accounting (check `cacheReadInputTokens`):

```bash
tail -10 ~/.ai-agent/accounting.jsonl | jq '.tokens'
```

---

## Output Issues

### Empty Response

**Problem**: Agent completes but stdout is empty.

**Cause**: LLM didn't produce final report or schema mismatch.

**Solution**:

```bash
# Check for final report in snapshot
zcat "$SNAPSHOT" | jq '.opTree.turns[-1].ops[] | select(.attributes.name | test("final"))'

# Check output schema matches
# Review LLM response in snapshot
zcat "$SNAPSHOT" | jq '.opTree.turns[-1].ops[] | select(.kind == "llm") | .response'
```

---

### Wrong Output Format

**Problem**: Output doesn't match expected schema.

**Cause**: LLM not following output schema.

**Solution**:

1. Verify schema in frontmatter
2. Add explicit output instructions
3. Check for schema validation errors in logs

---

### Truncated Output

**Problem**: Output appears cut off.

**Cause**: Model output length limit or truncation.

**Solution**:

```bash
# Check for truncation in snapshot
zcat "$SNAPSHOT" | jq '.opTree.turns[].ops[] | select(.response.truncated == true)'

# Full output preserved in snapshot
zcat "$SNAPSHOT" | jq '.opTree.turns[-1].ops[] | select(.kind == "llm") | .response.payload'
```

---

## Performance Issues

### Slow Responses

**Problem**: Agent takes too long to respond.

**Cause**: Model latency, tool latency, or too many turns.

**Solution**:

1. Enable streaming: `--stream`
2. Use faster models for simple tasks
3. Reduce tool calls per turn
4. Check slowest operations in snapshot:

```bash
zcat "$SNAPSHOT" | jq '[.opTree.turns[].ops[] | {kind, name:.attributes.name, latencyMs:(.endedAt - .startedAt)}] | sort_by(-.latencyMs) | .[0:5]'
```

---

### High Costs

**Problem**: Sessions costing more than expected.

**Cause**: Large contexts, expensive models, or many turns.

**Solution**:

1. Use smaller models for simple tasks
2. Enable caching
3. Monitor with accounting:

```bash
cat ~/.ai-agent/accounting.jsonl | jq -s 'map(select(.type == "llm")) | group_by(.model) | map({model: .[0].model, cost: (map(.costUsd // 0) | add)})'
```

4. Reduce context with lower `toolResponseMaxBytes`

---

## Network Issues

### Connection Refused

**Problem**: `ECONNREFUSED` errors.

**Cause**: Service not running or wrong endpoint.

**Solution**:

```bash
# Test endpoint connectivity
curl https://api.openai.com/v1/models -I

# Check DNS resolution
nslookup api.openai.com

# Check firewall/proxy
```

---

### SSL Certificate Error

**Problem**: `CERT_HAS_EXPIRED` or certificate errors.

**Cause**: SSL certificate issue or proxy interference.

**Solution**:

```bash
# Check certificate
openssl s_client -connect api.openai.com:443 -servername api.openai.com

# If using proxy, check proxy certificates
# Update CA certificates
```

---

### Proxy Not Working

**Problem**: Requests not going through proxy.

**Cause**: Proxy environment variables not set.

**Solution**:

```bash
# Set proxy environment variables
export HTTP_PROXY=http://proxy:8080
export HTTPS_PROXY=http://proxy:8080

# Verify proxy is set
env | grep -i proxy
```

---

## Getting Help

### Collect Debug Data

```bash
# Full debug session
DEBUG=true \
CONTEXT_DEBUG=true \
ai-agent --agent myagent.ai \
  --verbose \
  --trace-llm \
  --trace-mcp \
  "reproduce the issue" 2> debug.log
```

### Extract Snapshot Data

```bash
# Find latest snapshot
SNAPSHOT=$(ls -t ~/.ai-agent/sessions/*.json.gz | head -1)

# Extract summary
zcat "$SNAPSHOT" | jq '{
  success: .opTree.success,
  error: .opTree.error,
  turns: (.opTree.turns | length),
  errors: ([.. | objects | select(.severity == "ERR")] | length)
}'

# Extract errors
zcat "$SNAPSHOT" | jq '[.. | objects | select(.severity == "ERR" or .severity == "WRN")]'
```

### Check Logs for Patterns

```bash
# Count errors and warnings
grep -c '\[ERR\]' debug.log
grep -c '\[WRN\]' debug.log

# Find specific error types
grep '\[ERR\]' debug.log | sort | uniq -c | sort -rn
```

---

## See Also

- [Operations](Operations) - Operations overview
- [Debugging Guide](Operations-Debugging) - Step-by-step debugging workflow
- [Exit Codes](Operations-Exit-Codes) - Exit code reference
- [Logging](Operations-Logging) - Logging system reference
- [Session Snapshots](Operations-Snapshots) - Snapshot extraction guide
