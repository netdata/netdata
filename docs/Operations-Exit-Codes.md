# Exit Codes

Reference for AI Agent exit codes used in scripting and CI/CD pipelines.

---

## Table of Contents

- [Overview](#overview) - Exit code summary
- [Exit Code Reference](#exit-code-reference) - Detailed descriptions
- [Session Exit Reasons](#session-exit-reasons) - Internal exit conditions
- [Scripting Examples](#scripting-examples) - Handling exit codes
- [CI/CD Integration](#cicd-integration) - Pipeline usage
- [Troubleshooting by Exit Code](#troubleshooting-by-exit-code) - Diagnostic steps
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent uses standardized exit codes:

| Code | Category                | Description                          |
| ---- | ----------------------- | ------------------------------------ |
| 0    | Success                 | Agent completed successfully         |
| 1    | Configuration Error     | Invalid configuration or agent file  |
| 2    | LLM Error               | Provider or model failure            |
| 3    | Tool Error              | MCP server or tool execution failure |
| 4    | CLI Error               | Invalid command-line arguments       |
| 5    | Schema Validation Error | Tool schema validation failed        |

---

## Exit Code Reference

### Exit Code 0: Success

**Meaning**: Agent completed successfully with a final report.

**Conditions**:

- Agent produced a valid final report
- All required outputs generated
- No fatal errors occurred

**Script check**:

```bash
ai-agent --agent test.ai "query"
if [ $? -eq 0 ]; then
  echo "Success"
fi
```

---

### Exit Code 1: Configuration Error

**Meaning**: Configuration file or agent file is invalid.

**Common causes**:

- Invalid YAML frontmatter
- Unknown frontmatter keys
- Missing required configuration
- Malformed JSON in `.ai-agent.json`
- Invalid model chain specification

**Examples**:

```
ERR: Failed to parse agent frontmatter: invalid YAML
ERR: Unknown frontmatter key: unknownKey
ERR: Configuration file not found: .ai-agent.json
```

**Resolution**:

1. Validate agent file: `ai-agent --agent test.ai --dry-run`
2. Check YAML syntax in frontmatter
3. Verify `.ai-agent.json` JSON syntax

---

### Exit Code 2: LLM Error

**Meaning**: LLM provider returned an unrecoverable error.

**Common causes**:

- Invalid API key (401 Unauthorized)
- Rate limiting (429 Too Many Requests)
- Model not available
- Provider timeout
- All retries exhausted
- Context window exceeded

**Examples**:

```
ERR: LLM communication error: 401 Unauthorized
ERR: LLM request failed: 429 rate limited
ERR: All retries exhausted for provider openai
ERR: Context window exceeded
```

**Resolution**:

1. Verify API key: `echo $OPENAI_API_KEY | head -c 10`
2. Check provider status page
3. Add fallback models
4. Reduce context or tool responses

---

### Exit Code 3: Tool Error

**Meaning**: MCP server or tool execution failed fatally.

**Common causes**:

- MCP server failed to start
- Tool execution timeout
- Required tool not found
- Tool returned fatal error

**Examples**:

```
ERR: Failed to initialize MCP server: github
ERR: Tool timeout after 30000 ms: mcp__api__slow_query
ERR: Tool not found: mcp__server__unknown_tool
```

**Resolution**:

1. Verify MCP server configuration
2. Test server independently: `npx -y @server/package --help`
3. Check tool availability with `--dry-run`
4. Increase tool timeout if needed

---

### Exit Code 4: CLI Error

**Meaning**: Invalid command-line arguments or usage.

**Common causes**:

- Missing required argument
- Invalid flag value
- Conflicting options
- Unknown options

**Examples**:

```
ERR: Missing required argument: --agent
ERR: Invalid --reasoning-tokens value: 'invalid'
ERR: Unknown option: --invalid-flag
```

**Resolution**:

1. Check help: `ai-agent --help`
2. Verify argument syntax
3. Check for typos in option names

---

### Exit Code 5: Schema Validation Error

**Meaning**: Tool schema validation failed during `--list-tools` operation.

**Common causes**:

- Tool input schema does not conform to the specified JSON Schema draft
- Tool output schema validation errors

**Examples**:

```
ERR: Schema validation failed for 3 tool(s)
```

**Resolution**:

1. Review tool schemas against the specified JSON Schema draft
2. Check `--schema-validate <draft>` argument
3. Validate schemas independently using schema validation tools

---

## Session Exit Reasons

Internal exit codes logged during session execution (visible in logs and snapshots):

| Exit Reason                    | Description                                         | Fatal |
| ------------------------------ | --------------------------------------------------- | ----- |
| **Success Exits**              |                                                     |       |
| `EXIT-FINAL-ANSWER`            | Normal completion with final report                 | No    |
| `EXIT-MAX-TURNS-WITH-RESPONSE` | Turn limit reached with valid output                | No    |
| `EXIT-ROUTER-HANDOFF`          | Router selected destination (orchestration handoff) | No    |
| `EXIT-USER-STOP`               | User requested stop via stopRef                     | No    |
| **LLM Failures**               |                                                     |       |
| `EXIT-NO-LLM-RESPONSE`         | No response from LLM provider                       | Yes   |
| `EXIT-EMPTY-RESPONSE`          | Empty response from LLM provider                    | Yes   |
| `EXIT-AUTH-FAILURE`            | Authentication error (401, etc.)                    | Yes   |
| `EXIT-QUOTA-EXCEEDED`          | Rate or quota limit (429, etc.)                     | Yes   |
| `EXIT-MODEL-ERROR`             | Model or provider-specific error                    | Yes   |
| **Tool Failures**              |                                                     |       |
| `EXIT-TOOL-FAILURE`            | General tool execution failure                      | Yes   |
| `EXIT-MCP-CONNECTION-LOST`     | MCP server connection lost                          | Yes   |
| `EXIT-TOOL-NOT-AVAILABLE`      | Requested tool not found/available                  | Yes   |
| `EXIT-TOOL-TIMEOUT`            | Tool execution timeout                              | Yes   |
| **Configuration**              |                                                     |       |
| `EXIT-NO-PROVIDERS`            | No LLM providers configured                         | Yes   |
| `EXIT-INVALID-MODEL`           | Invalid model specified                             | Yes   |
| `EXIT-MCP-INIT-FAILED`         | MCP server initialization failed                    | Yes   |
| **Timeout/Limits**             |                                                     |       |
| `EXIT-INACTIVITY-TIMEOUT`      | Session inactivity timeout                          | Yes   |
| `EXIT-MAX-RETRIES`             | All retries exhausted                               | Yes   |
| `EXIT-TOKEN-LIMIT`             | Context window exhausted                            | Yes   |
| `EXIT-MAX-TURNS-NO-RESPONSE`   | Turn limit reached without output                   | Yes   |
| **Unexpected**                 |                                                     |       |
| `EXIT-UNCAUGHT-EXCEPTION`      | Uncaught exception in agent                         | Yes   |
| `EXIT-SIGNAL-RECEIVED`         | Process signal received (SIGTERM, etc.)             | Yes   |
| `EXIT-UNKNOWN`                 | Unknown error condition                             | Yes   |

### Finding Exit Reason in Snapshot

```bash
zcat "$SNAPSHOT" | jq '[.. | objects | select(.remoteIdentifier | test("agent:EXIT"))]'
```

### Exit Reason in Logs

```
[VRB] EXIT-FINAL-ANSWER: Agent completed with final report (fatal=false)
```

---

## Scripting Examples

### Basic Exit Code Handling

```bash
#!/bin/bash
ai-agent --agent myagent.ai "$@"
exit_code=$?

case $exit_code in
  0)
    echo "Success"
    ;;
  1)
    echo "Configuration error - check agent file"
    exit 1
    ;;
  2)
    echo "LLM error - check provider credentials"
    exit 2
    ;;
  3)
    echo "Tool error - check MCP servers"
    exit 3
    ;;
  4)
    echo "CLI error - check arguments"
    exit 4
    ;;
  *)
    echo "Unknown error: $exit_code"
    exit $exit_code
    ;;
esac
```

### Retry on Transient Errors

```bash
#!/bin/bash
max_retries=3
retry_count=0

while [ $retry_count -lt $max_retries ]; do
  ai-agent --agent myagent.ai "$@"
  exit_code=$?

  case $exit_code in
    0)
      exit 0  # Success
      ;;
    2)
      # LLM error - might be transient (rate limit)
      retry_count=$((retry_count + 1))
      echo "LLM error, retry $retry_count of $max_retries"
      sleep $((retry_count * 10))
      ;;
    *)
      # Other errors - don't retry
      exit $exit_code
      ;;
  esac
done

echo "Max retries exceeded"
exit 2
```

### Conditional Fallback

```bash
#!/bin/bash
# Try primary agent, fall back to simpler one on error
ai-agent --agent advanced.ai "$@"
if [ $? -ne 0 ]; then
  echo "Primary agent failed, trying fallback..."
  ai-agent --agent simple.ai "$@"
fi
```

### Capture Output with Exit Code

```bash
#!/bin/bash
output=$(ai-agent --agent myagent.ai "$@" 2>&1)
exit_code=$?

if [ $exit_code -eq 0 ]; then
  echo "$output"
else
  echo "Error (code $exit_code): $output" >&2
  exit $exit_code
fi
```

---

## CI/CD Integration

### GitHub Actions

```yaml
jobs:
  run-agent:
    runs-on: ubuntu-latest
    steps:
      - name: Run AI Agent
        id: agent
        run: |
          ai-agent --agent analyze.ai "${{ github.event.inputs.query }}"
        continue-on-error: true

      - name: Handle Exit Code
        if: always()
        run: |
          case "${{ steps.agent.outcome }}" in
            success)
              echo "Agent completed successfully"
              ;;
            failure)
              echo "Agent failed - check logs"
              exit 1
              ;;
          esac
```

### GitLab CI

```yaml
run-agent:
  script:
    - |
      ai-agent --agent analyze.ai "$QUERY"
      exit_code=$?
      if [ $exit_code -ne 0 ]; then
        echo "Agent failed with exit code $exit_code"
        exit $exit_code
      fi
  rules:
    - when: manual
```

### Jenkins Pipeline

```groovy
pipeline {
    stages {
        stage('Run Agent') {
            steps {
                script {
                    def result = sh(
                        script: 'ai-agent --agent analyze.ai "${QUERY}"',
                        returnStatus: true
                    )
                    if (result == 0) {
                        echo 'Agent completed successfully'
                    } else if (result == 2) {
                        error 'LLM provider error - check credentials'
                    } else {
                        error "Agent failed with exit code ${result}"
                    }
                }
            }
        }
    }
}
```

### Make Target

```makefile
.PHONY: run-agent
run-agent:
	@ai-agent --agent myagent.ai "$(QUERY)" || \
		(echo "Agent failed with exit code $$?" && exit 1)
```

---

## Troubleshooting by Exit Code

### Exit Code 1 (Configuration)

```bash
# Validate configuration
ai-agent --agent myagent.ai --dry-run

# Check JSON syntax
jq . .ai-agent.json

# Validate YAML frontmatter
head -50 myagent.ai
```

### Exit Code 2 (LLM)

```bash
# Enable verbose output
ai-agent --agent myagent.ai --verbose --trace-llm "test"

# Check API key (partial)
echo $OPENAI_API_KEY | head -c 10

# Test provider directly
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

### Exit Code 3 (Tool)

```bash
# Enable MCP tracing
ai-agent --agent myagent.ai --verbose --trace-mcp "test"

# Test MCP server independently
npx -y @modelcontextprotocol/server-filesystem --help

# Check MCP server configuration
jq '.mcpServers' .ai-agent.json
```

### Exit Code 4 (CLI)

```bash
# Check available options
ai-agent --help

# Validate specific option
ai-agent --agent myagent.ai --dry-run
```

---

## See Also

- [Operations](Operations) - Operations overview
- [Debugging Guide](Operations-Debugging) - Debugging workflow
- [Troubleshooting](Operations-Troubleshooting) - Problem/cause/solution reference
- [Logging](Operations-Logging) - Logging system
