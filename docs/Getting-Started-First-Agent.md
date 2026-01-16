# First Agent Tutorial

**Goal**: Build a practical web research agent with tools, step by step.

---

## Table of Contents

- [Prerequisites](#prerequisites) - What you need before starting
- [What You'll Build](#what-youll-build) - Overview of the agent
- [Step 1: Configure MCP Tools](#step-1-configure-mcp-tools) - Add web search and fetch tools
- [Step 2: Create the Agent](#step-2-create-the-agent) - Write the .ai file
- [Step 3: Test the Agent](#step-3-test-the-agent) - Run and verify
- [Step 4: Add Caching](#step-4-add-caching) - Avoid repeated API calls
- [Step 5: Add Model Fallbacks](#step-5-add-model-fallbacks) - Handle failures gracefully
- [Step 6: Deploy as a Service](#step-6-deploy-as-a-service) - Run as REST API
- [Complete Example](#complete-example) - Final agent file
- [Verification](#verification) - Confirm it works
- [Next Steps](#next-steps) - Where to go from here
- [See Also](#see-also) - Related documentation

---

## Prerequisites

Before starting this tutorial, ensure you have:

- **ai-agent installed and working** - See [Installation](Getting-Started-Installation)
- **Completed Quick Start** - See [Quick Start](Getting-Started-Quick-Start)
- **A working LLM provider** - OpenAI or Anthropic configured
- **A Brave Search API key** - Free at [brave.com/search/api](https://brave.com/search/api/)

> **Note:** You can complete this tutorial without the Brave API key by skipping the search functionality and using only the fetch tool.

---

## What You'll Build

A web research agent that can:

1. **Search the web** using Brave Search
2. **Fetch and read** web pages
3. **Synthesize** findings into a structured summary

**Example interaction:**

```
Input:  "What are the latest developments in quantum computing?"
Output: A 3-paragraph summary with key points and source URLs
```

---

## Step 1: Configure MCP Tools

MCP (Model Context Protocol) servers provide tools for your agents. Add these to your `.ai-agent.json`:

**Update or create `.ai-agent.json`:**

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
      "url": "https://mcp.jina.ai/v1"
    }
  }
}
```

**Explanation:**

| Server    | Type  | Purpose                                                     |
| --------- | ----- | ----------------------------------------------------------- |
| `brave`   | stdio | Runs locally, provides `brave_search` tool                  |
| `fetcher` | http  | Remote service, provides `fetch` tool for reading web pages |

**Set the Brave API key:**

```bash
export BRAVE_API_KEY="your-brave-api-key"
```

Or add to `~/.ai-agent/ai-agent.env`:

```bash
BRAVE_API_KEY=your-brave-api-key
```

---

## Step 2: Create the Agent

Create a file named `researcher.ai`:

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

**File structure explained:**

| Section               | Purpose                                           |
| --------------------- | ------------------------------------------------- |
| `description`         | Human-readable description shown in `--help`      |
| `models`              | LLM model(s) to use for this agent                |
| `tools`               | MCP servers to enable (must be defined in config) |
| `maxTurns`            | Maximum LLM request-response cycles               |
| `maxToolCallsPerTurn` | Limit tool calls per turn to prevent runaway      |
| `temperature`         | Lower = more focused/deterministic                |
| `output.format`       | Hint for output formatting                        |
| System prompt         | Instructions after the closing `---`              |

---

## Step 3: Test the Agent

Run the agent with a research question:

```bash
ai-agent --agent researcher.ai "What are the latest developments in quantum computing?"
```

**Expected behavior:**

1. Agent calls `brave_search` with a relevant query
2. Agent receives search results with URLs
3. Agent calls `fetch` on 2-3 promising URLs
4. Agent reads the page content
5. Agent synthesizes a summary with sources

**Expected output (structure):**

```markdown
## Summary

Quantum computing has seen significant advances in 2024-2025...
[2-3 paragraphs of synthesized information]

## Key Points

- IBM achieved a 1000+ qubit processor
- Google demonstrated quantum error correction
- Microsoft announced topological qubit progress
- [more points...]

## Sources

1. [IBM Quantum Roadmap](https://example.com/ibm-quantum)
2. [Google AI Blog](https://example.com/google-quantum)
3. [Nature: Quantum Computing Review](https://example.com/nature)
```

**Verbose mode for debugging:**

```bash
ai-agent --agent researcher.ai --verbose "What is quantum computing?"
```

This shows tool calls, timing, and model responses.

---

## Step 4: Add Caching

Avoid repeated API calls for identical queries by adding caching:

**Update `researcher.ai` frontmatter:**

```yaml
---
description: Web research agent that searches and summarizes
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
maxTurns: 15
cache: 1h
---
```

**Cache values:**

| Value | Duration  |
| ----- | --------- |
| `5m`  | 5 minutes |
| `1h`  | 1 hour    |
| `1d`  | 1 day     |
| `1w`  | 1 week    |

**How it works:**

- Same agent + same prompt = cached response returned
- Reduces costs and latency for repeated queries
- Cache is stored locally in `~/.ai-agent/cache.db`

**Test caching:**

```bash
# First run (calls LLM)
time ai-agent --agent researcher.ai "What is TypeScript?"

# Second run (returns cached response, much faster)
time ai-agent --agent researcher.ai "What is TypeScript?"
```

---

## Step 5: Add Model Fallbacks

Handle provider failures gracefully with a fallback chain:

**Update `researcher.ai` frontmatter:**

```yaml
---
description: Web research agent that searches and summarizes
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
  - anthropic/claude-sonnet-4-20250514
maxRetries: 3
---
```

**How fallbacks work:**

1. First, tries `gpt-4o`
2. If that fails (rate limit, timeout, error), tries `gpt-4o-mini`
3. If that fails, tries `claude-sonnet-4-20250514`
4. Each model gets `maxRetries` attempts before falling back

**Requirements:**

- Each provider in the fallback chain must be configured in `.ai-agent.json`
- For the example above, both OpenAI and Anthropic need to be configured

---

## Step 6: Deploy as a Service

Run your agent as a REST API for integration with other applications:

**Start the server:**

```bash
ai-agent --agent researcher.ai --api 8080
```

**Expected output:**

```
REST API server started on http://localhost:8080
Available endpoints:
  GET  /v1/researcher?q=<prompt>
```

**Call via GET:**

```bash
curl -G "http://localhost:8080/v1/researcher" \
  --data-urlencode "q=What is quantum computing?" \
  --data-urlencode "format=markdown"
```

**Expected response:**

```json
{
  "success": true,
  "output": "## Summary\n\nQuantum computing is...",
  "finalReport": {
    "answer": "## Summary\n\nQuantum computing is..."
  },
  "reasoning": ""
}
```

For production deployment options, see [Headends REST](Headends-REST).

---

## Complete Example

Here's the final `researcher.ai` with all features:

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

**And the matching `.ai-agent.json`:**

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    },
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
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
      "url": "https://mcp.jina.ai/v1"
    }
  },
  "defaults": {
    "llmTimeout": 120000,
    "toolTimeout": 60000
  }
}
```

---

## Verification

Run these tests to confirm your agent works correctly:

**Test 1: Basic query**

```bash
ai-agent --agent researcher.ai "What is the capital of France?"
```

Expected: Quick response mentioning Paris.

**Test 2: Research query (uses tools)**

```bash
ai-agent --agent researcher.ai "What are the latest AI developments in 2025?"
```

Expected: Agent searches, reads sources, and provides cited summary.

**Test 3: Verbose mode**

```bash
ai-agent --agent researcher.ai --verbose "What is Python?"
```

Expected: See tool calls (`brave_search`, `fetch`), timing, and model used.

**Test 4: Cache hit**

```bash
# Run twice with same query
ai-agent --agent researcher.ai "What is JavaScript?"
ai-agent --agent researcher.ai "What is JavaScript?"
```

Expected: Second run is much faster (cached).

---

## Next Steps

Congratulations on building your first practical agent. Continue learning:

| Goal                          | Page                                                       |
| ----------------------------- | ---------------------------------------------------------- |
| Learn all frontmatter options | [Agent Files](Agent-Files)                                 |
| Write better system prompts   | [System Prompts](System-Prompts)                           |
| Build multi-agent systems     | [Multi-Agent Orchestration](Agent-Development-Multi-Agent) |
| Deploy to production          | [Headends](Headends)                                       |
| Add more tools                | [Configuration MCP Servers](Configuration-MCP-Servers)     |

---

## See Also

- [Getting Started](Getting-Started) - Chapter overview
- [Quick Start](Getting-Started-Quick-Start) - Simpler first agent
- [Agent Files](Agent-Files) - Complete `.ai` file reference
- [Agent Files Tools](Agent-Files-Tools) - Tool configuration options
- [Configuration MCP Servers](Configuration-MCP-Servers) - Configure tool servers
- [CLI Running Agents](CLI-Running-Agents) - All CLI options for running agents
