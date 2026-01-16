# CLI: Scripting and Automation

How to use ai-agent in scripts and automation pipelines. Covers exit codes, JSON output, error handling, and batch processing.

---

## Table of Contents

- [Overview](#overview) - Scripting considerations
- [Exit Codes](#exit-codes) - Return code reference
- [Input Handling](#input-handling) - Stdin, arguments, files
- [Output Handling](#output-handling) - Capturing and parsing output
- [Error Handling](#error-handling) - Detecting and recovering from failures
- [Batch Processing](#batch-processing) - Processing multiple inputs
- [Integration Patterns](#integration-patterns) - Common automation scenarios
- [See Also](#see-also) - Related pages

---

## Overview

ai-agent is designed for scripting:

- **Deterministic exit codes** for error detection
- **JSON output mode** for structured parsing
- **Stdin support** for piping
- **Quiet mode** for clean output
- **Non-interactive** by default (no prompts)

**Basic scripting pattern:**

```bash
#!/bin/bash
if result=$(ai-agent --agent task.ai --quiet "Query" 2>/dev/null); then
  echo "Success: $result"
else
  echo "Agent failed"
  exit 1
fi
```

---

## Exit Codes

ai-agent uses consistent exit codes:

| Code | Meaning             | Common Causes                        |
| ---- | ------------------- | ------------------------------------ |
| `0`  | Success             | Agent completed normally             |
| `1`  | General error       | Runtime failures, tool errors        |
| `3`  | Configuration error | Missing MCP servers, bad settings    |
| `4`  | Invalid arguments   | Bad CLI flags, missing required args |
| `5`  | Validation error    | Schema validation failed             |

### Checking Exit Codes

```bash
# Simple check
ai-agent --agent task.ai "Query"
if [[ $? -ne 0 ]]; then
  echo "Agent failed"
  exit 1
fi

# Capture exit code
ai-agent --agent task.ai "Query"
exit_code=$?
case $exit_code in
  0) echo "Success" ;;
  3) echo "Config error - check MCP servers" ;;
  4) echo "Invalid arguments" ;;
  *) echo "Unknown error: $exit_code" ;;
esac
```

### Exit Code Tags

In verbose mode, exit codes include tags for debugging:

```
[VRB] ← [0.0] agent EXIT-CLI: reason (fatal=true)
```

Common tags:

- `EXIT-CLI` - General CLI error
- `EXIT-COMMANDER` - Argument parsing error
- `EXIT-HEADEND-*` - Headend startup errors
- `EXIT-LIST-TOOLS-*` - Tool listing errors

---

## Input Handling

### Command Line Arguments

```bash
# Direct argument
ai-agent --agent chat.ai "What is 2+2?"

# Variable substitution
query="What is the capital of $country?"
ai-agent --agent chat.ai "$query"

# Multi-line with quoting
ai-agent --agent chat.ai "Line 1
Line 2
Line 3"
```

### Standard Input

Use `-` placeholder to read from stdin:

```bash
# Pipe content
echo "Summarize this text" | ai-agent --agent summarizer.ai -

# Redirect file
ai-agent --agent analyzer.ai - < document.txt

# Heredoc
ai-agent --agent coder.ai - <<'EOF'
Fix this function:
function broken() { return undefined }
EOF
```

### File Content

Read files into prompts:

```bash
# Using cat
ai-agent --agent reviewer.ai "Review this code: $(cat script.py)"

# Using command substitution
content=$(cat data.json)
ai-agent --agent parser.ai "Parse this JSON: $content"
```

---

## Output Handling

### Capturing Output

```bash
# Capture stdout only
result=$(ai-agent --agent chat.ai --quiet "Query")

# Capture with stderr
result=$(ai-agent --agent chat.ai --quiet "Query" 2>&1)

# Separate stdout and stderr
ai-agent --agent chat.ai "Query" >output.txt 2>error.txt
```

### JSON Output

Request JSON format for structured parsing:

```bash
# Using --format
result=$(ai-agent --agent data.ai --format json --quiet "Get user data")
name=$(echo "$result" | jq -r '.name')

# Using --schema to specify expected output format
ai-agent --agent extractor.ai \
  --schema '{"type":"object","properties":{"items":{"type":"array"}}}' \
  --quiet \
  "Extract items" | jq '.items[]'
```

### Quiet Mode

Suppress logs for clean output:

```bash
# Only agent response, no logs
ai-agent --agent chat.ai --quiet "Query" > result.txt

# Discard stderr entirely
ai-agent --agent chat.ai --quiet "Query" 2>/dev/null
```

### Disabling Streaming

For scripts, disable streaming to get complete output:

```bash
# Wait for complete response
result=$(ai-agent --agent chat.ai --no-stream --quiet "Generate report")
```

---

## Error Handling

### Basic Error Handling

```bash
#!/bin/bash
set -e  # Exit on error

# Run agent
if ! result=$(ai-agent --agent task.ai --quiet "Query" 2>&1); then
  echo "Agent failed: $result" >&2
  exit 1
fi

echo "Result: $result"
```

### Retry Logic

```bash
#!/bin/bash
max_retries=3
retry_count=0

while [[ $retry_count -lt $max_retries ]]; do
  if result=$(ai-agent --agent task.ai --quiet "Query" 2>/dev/null); then
    echo "$result"
    exit 0
  fi

  retry_count=$((retry_count + 1))
  echo "Attempt $retry_count failed, retrying..." >&2
  sleep 2
done

echo "All retries failed" >&2
exit 1
```

### Timeout Handling

```bash
# Using timeout command
if ! timeout 60 ai-agent --agent chat.ai --quiet "Query"; then
  echo "Agent timed out or failed"
  exit 1
fi

# Using ai-agent's built-in timeout
ai-agent --agent chat.ai --llm-timeout-ms 30000 --quiet "Quick query"
```

### Validation Before Execution

```bash
# Dry run to validate config
if ! ai-agent --agent task.ai --dry-run 2>/dev/null; then
  echo "Configuration invalid"
  exit 1
fi

# Now run for real
ai-agent --agent task.ai "Real query"
```

---

## Batch Processing

### Processing Multiple Inputs

```bash
#!/bin/bash
# Process lines from a file
while IFS= read -r line; do
  result=$(ai-agent --agent processor.ai --quiet "$line" 2>/dev/null)
  echo "$line -> $result"
done < inputs.txt
```

### Parallel Processing

```bash
#!/bin/bash
# Using GNU parallel
cat queries.txt | parallel -j4 \
  'ai-agent --agent chat.ai --quiet "{}" 2>/dev/null'

# Using xargs
cat queries.txt | xargs -P4 -I{} \
  sh -c 'ai-agent --agent chat.ai --quiet "{}" 2>/dev/null'
```

### Processing Files

```bash
#!/bin/bash
# Process each file in directory
for file in documents/*.txt; do
  echo "Processing: $file"
  ai-agent --agent summarizer.ai --quiet - < "$file" > "summaries/$(basename "$file")"
done
```

### JSON Lines Processing

```bash
#!/bin/bash
# Process JSON lines (one JSON object per line)
while IFS= read -r json_line; do
  result=$(echo "$json_line" | ai-agent --agent transformer.ai --format json --quiet -)
  echo "$result"
done < input.jsonl > output.jsonl
```

---

## Integration Patterns

### CI/CD Pipeline

```bash
#!/bin/bash
# Code review in CI
changed_files=$(git diff --name-only HEAD~1)

for file in $changed_files; do
  if [[ $file == *.py ]]; then
    echo "Reviewing: $file"
    review=$(ai-agent --agent reviewer.ai --quiet "Review this Python code: $(cat "$file")")

    if echo "$review" | grep -q "CRITICAL"; then
      echo "Critical issues found in $file"
      exit 1
    fi
  fi
done

echo "Code review passed"
```

### Git Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit
# Generate commit message suggestion

staged_diff=$(git diff --cached)
if [[ -n "$staged_diff" ]]; then
  suggestion=$(echo "$staged_diff" | ai-agent --agent commit-helper.ai --quiet -)
  echo "Suggested commit message:"
  echo "$suggestion"
fi
```

### Cron Job

```bash
#!/bin/bash
# Daily report generation
# Run as: 0 9 * * * /path/to/daily-report.sh

LOG_DIR=/var/log/myapp
REPORT_DIR=/var/reports

# Generate summary
summary=$(ai-agent --agent reporter.ai --quiet \
  "Summarize these logs: $(tail -1000 "$LOG_DIR/app.log")")

# Save report
date_str=$(date +%Y-%m-%d)
echo "$summary" > "$REPORT_DIR/report-$date_str.txt"
```

### Web Service Integration

```bash
#!/bin/bash
# Process webhook payload
# Called by webhook receiver

payload=$(cat -)  # Read from stdin
event_type=$(echo "$payload" | jq -r '.event')

case $event_type in
  "issue_opened")
    response=$(echo "$payload" | ai-agent --agent triage.ai --format json --quiet -)
    # Post response back to API
    curl -X POST -d "$response" https://api.example.com/respond
    ;;
esac
```

### Environment Variables

```bash
#!/bin/bash
# Environment-aware execution

# Set model based on environment
if [[ "$ENVIRONMENT" == "production" ]]; then
  MODEL="openai/gpt-4o"
else
  MODEL="ollama/llama3.2"
fi

ai-agent --agent task.ai --models "$MODEL" --quiet "Query"
```

---

## Best Practices

### Script Template

```bash
#!/bin/bash
set -euo pipefail

# Configuration
AGENT="./agents/task.ai"
TIMEOUT=120
MAX_RETRIES=3

# Logging
log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2; }

# Run agent with retries
run_agent() {
  local query="$1"
  local attempt=0

  while [[ $attempt -lt $MAX_RETRIES ]]; do
    if result=$(timeout "$TIMEOUT" ai-agent --agent "$AGENT" --quiet "$query" 2>/dev/null); then
      echo "$result"
      return 0
    fi
    attempt=$((attempt + 1))
    log "Attempt $attempt failed"
    sleep 2
  done

  return 1
}

# Main
main() {
  log "Starting processing"

  if ! run_agent "$1"; then
    log "All attempts failed"
    exit 1
  fi

  log "Completed successfully"
}

main "$@"
```

### Checklist

Before deploying scripts:

- [ ] Handle all exit codes appropriately
- [ ] Use `--quiet` for clean output
- [ ] Use `--no-stream` for complete responses
- [ ] Implement retry logic for transient failures
- [ ] Set appropriate timeouts
- [ ] Validate with `--dry-run` first
- [ ] Log errors to stderr
- [ ] Test with edge cases (empty input, large input)

---

## See Also

- [CLI](CLI) - CLI overview and quick reference
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents
- [CLI-Debugging](CLI-Debugging) - Debugging execution
- [CLI-Overrides](CLI-Overrides) - Runtime configuration overrides
- [Operations-Troubleshooting](Operations-Troubleshooting) - Common problems
