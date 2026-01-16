# CLI Headend

Direct command-line execution of agents.

---

## Basic Usage

```bash
ai-agent --agent <path.ai> "user prompt"
```

---

## Examples

### Simple Query

```bash
ai-agent --agent chat.ai "What is the weather today?"
```

### With Verbose Output

```bash
ai-agent --agent chat.ai --verbose "Explain quantum computing"
```

### With Tracing

```bash
ai-agent --agent research.ai --trace-llm --trace-mcp "Research topic"
```

---

## Input Methods

### Inline Prompt

```bash
ai-agent --agent chat.ai "Your question here"
```

### From File

```bash
ai-agent --agent chat.ai @prompt.txt
```

### From Stdin

```bash
echo "Your question" | ai-agent --agent chat.ai -
```

### JSON Input (for agents with input schemas)

```bash
ai-agent --agent analyzer.ai '{"text": "content to analyze", "detailed": true}'
```

---

## Output

### Standard Output

Agent responses stream to stdout:

```bash
ai-agent --agent chat.ai "Hello" > response.txt
```

### Standard Error

Logs, errors, and debug info go to stderr:

```bash
ai-agent --agent chat.ai "Hello" 2> logs.txt
```

### Separate Both

```bash
ai-agent --agent chat.ai "Hello" > response.txt 2> logs.txt
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Configuration error |
| 2 | LLM communication error |
| 3 | Tool execution error |
| 4 | Invalid command line arguments |

---

## Executable Agents

Make `.ai` files directly executable:

```yaml
#!/usr/bin/env ai-agent
---
models:
  - openai/gpt-4o
---
You are a helpful assistant.
```

```bash
chmod +x chat.ai
./chat.ai "Hello"
```

---

## Session Management

### Save Conversation

```bash
ai-agent --agent chat.ai --save conversation.json "Hello"
```

### Load and Continue

```bash
ai-agent --agent chat.ai --load conversation.json "Follow-up question"
```

### Resume Session

```bash
ai-agent --agent chat.ai --resume <session-id> "Continue..."
```

---

## Dry Run

Validate configuration without calling LLM:

```bash
ai-agent --agent chat.ai --dry-run "test"
```

---

## Pipeline Integration

### In Scripts

```bash
#!/bin/bash
result=$(ai-agent --agent summarizer.ai "$input")
echo "Summary: $result"
```

### With jq

```bash
ai-agent --agent analyzer.ai --format json "$data" | jq '.summary'
```

### In CI/CD

```yaml
- name: Run AI Analysis
  run: |
    ai-agent --agent code-review.ai @diff.txt > review.md
```

---

## See Also

- [Getting Started](Getting-Started) - Installation and first agent
- [CLI Reference](Getting-Started-CLI-Reference) - All CLI options
