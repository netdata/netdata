# Exit Codes

Exit code reference for debugging.

---

## Standard Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 0 | Success | Agent completed successfully |
| 1 | Configuration Error | Invalid config, missing required fields |
| 2 | LLM Error | Provider communication failure |
| 3 | Tool Error | MCP/tool execution failure |
| 4 | CLI Error | Invalid command line arguments |

---

## Detailed Exit Scenarios

### Exit 0: Success

Agent completed normally:
- Final report generated
- Output written to stdout
- No errors

### Exit 1: Configuration Error

Common causes:
- Config file not found
- Invalid JSON syntax
- Missing required provider
- Invalid frontmatter

Debug:
```bash
ai-agent --agent test.ai --dry-run
```

### Exit 2: LLM Error

Common causes:
- Invalid API key
- Provider rate limiting
- Network timeout
- Model not available
- All retries exhausted

Debug:
```bash
ai-agent --agent test.ai --trace-llm "query"
```

### Exit 3: Tool Error

Common causes:
- MCP server failed to start
- Tool execution timeout
- Tool returned fatal error

Debug:
```bash
ai-agent --agent test.ai --trace-mcp "query"
```

### Exit 4: CLI Error

Common causes:
- Unknown option
- Missing required argument
- Invalid option value

Debug:
```bash
ai-agent --help
```

---

## Exit Code in Scripts

```bash
#!/bin/bash

ai-agent --agent analyzer.ai "$input"
code=$?

case $code in
  0)
    echo "Success"
    ;;
  1)
    echo "Config error - check .ai-agent.json"
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
  *)
    echo "Unknown error: $code"
    exit $code
    ;;
esac
```

---

## CI/CD Integration

```yaml
# GitHub Actions
- name: Run AI Analysis
  run: |
    ai-agent --agent review.ai @code.txt > review.md
  continue-on-error: false

# Handle specific codes
- name: Run with fallback
  run: |
    ai-agent --agent primary.ai "query" || \
    ai-agent --agent fallback.ai "query"
```

---

## See Also

- [Debugging Guide](Operations-Debugging) - Debugging workflow
- [Troubleshooting](Operations-Troubleshooting) - Common issues
