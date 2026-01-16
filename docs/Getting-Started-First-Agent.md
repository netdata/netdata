# First Agent Tutorial

Build a practical web research agent step-by-step.

---

## Goal

Create an agent that can:
1. Search the web
2. Fetch and read web pages
3. Summarize findings

---

## Step 1: Set Up Configuration

Create `.ai-agent.json`:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "mcpServers": {
    "brave": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-server-brave-search"],
      "env": {
        "BRAVE_API_KEY": "${BRAVE_API_KEY}"
      }
    },
    "fetcher": {
      "type": "http",
      "url": "https://mcp.jina.ai/v1",
      "cache": "1h"
    }
  },
  "queues": {
    "default": { "concurrent": 4 }
  }
}
```

---

## Step 2: Create the Agent

Create `researcher.ai`:

```yaml
#!/usr/bin/env ai-agent
---
description: Web research agent that searches and summarizes
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
maxTurns: 15
maxToolCallsPerTurn: 5
temperature: 0.3
output:
  format: markdown
---
You are a thorough web researcher.

## Your Process

1. **Search**: Use brave_search to find relevant sources
2. **Read**: Use fetcher to read the most promising pages
3. **Synthesize**: Combine findings into a coherent summary

## Guidelines

- Search with specific, targeted queries
- Read at least 3 sources before summarizing
- Cite your sources with URLs
- Be factual and objective

## Output Format

Provide your findings as:
1. **Summary** (2-3 paragraphs)
2. **Key Points** (bullet list)
3. **Sources** (numbered list with URLs)
```

---

## Step 3: Test It

```bash
ai-agent --agent researcher.ai "What are the latest developments in quantum computing?"
```

---

## Step 4: Add Caching

Avoid repeated API calls by adding agent-level caching:

```yaml
---
description: Web research agent that searches and summarizes
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
maxTurns: 15
cache: 1h   # Cache responses for 1 hour
---
```

---

## Step 5: Add Model Fallback

Handle failures gracefully:

```yaml
---
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
  - anthropic/claude-3-haiku
maxRetries: 3
---
```

If `gpt-4o` fails, it tries `gpt-4o-mini`, then `claude-3-haiku`.

---

## Step 6: Run as a Service

Deploy as REST API:

```bash
ai-agent --agent researcher.ai --api 8080
```

Call it:

```bash
curl -G "http://localhost:8080/v1/researcher" \
  --data-urlencode "q=Latest AI developments" \
  --data-urlencode "format=markdown"
```

---

## Complete Example

Final `researcher.ai`:

```yaml
#!/usr/bin/env ai-agent
---
description: Web research agent that searches and summarizes
usage: |
  Ask any research question. Example:
  "What are the latest developments in quantum computing?"
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
tools:
  - brave
  - fetcher
maxTurns: 15
maxToolCallsPerTurn: 5
maxRetries: 3
temperature: 0.3
cache: 1h
output:
  format: markdown
---
You are a thorough web researcher.

Current date: ${DATETIME}

## Your Process

1. **Search**: Use brave_search to find relevant sources
2. **Read**: Use fetcher to read the most promising pages
3. **Synthesize**: Combine findings into a coherent summary

## Guidelines

- Search with specific, targeted queries
- Read at least 3 sources before summarizing
- Cite your sources with URLs
- Be factual and objective
- Note when information might be outdated

## Output Format

Provide your findings as:
1. **Summary** (2-3 paragraphs)
2. **Key Points** (bullet list)
3. **Sources** (numbered list with URLs)
```

---

## What's Next?

- [Agent Development](Agent-Development) - Full frontmatter reference
- [Multi-Agent Orchestration](Agent-Development-Multi-Agent) - Build agent teams
- [Configuration](Configuration) - Advanced tool and provider setup
