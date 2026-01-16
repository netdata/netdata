# Sub-Agent Configuration

Configure sub-agents that your agent can call as tools. Sub-agents enable delegation and specialization.

---

## Table of Contents

- [Overview](#overview) - What sub-agents are and how they work
- [Quick Example](#quick-example) - Basic sub-agent configuration
- [Configuration Reference](#configuration-reference) - The agents key
- [How Sub-Agents Work](#how-sub-agents-work) - Execution model and communication
- [Sub-Agent Requirements](#sub-agent-requirements) - What sub-agents need
- [Common Patterns](#common-patterns) - Typical sub-agent configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Sub-agents are other `.ai` agent files that your agent can call as tools. This enables:
- **Specialization**: Each agent focuses on one task
- **Reusability**: Same sub-agent used by multiple parents
- **Composition**: Build complex systems from simple parts

**User questions answered**: "How do I call another agent?" / "How do agents work together?"

**Key concept**: When you list an agent in `agents:`, it becomes a tool the parent can call. The tool name is `agent__<toolName>`.

---

## Quick Example

Parent agent with sub-agents:

```yaml
---
description: Research coordinator
models:
  - anthropic/claude-sonnet-4-20250514
agents:
  - ./helpers/researcher.ai
  - ./helpers/writer.ai
maxTurns: 20
---

You coordinate research tasks.

Available specialists:
- `agent__researcher`: Web research and data gathering
- `agent__writer`: Report writing and formatting

Delegate appropriately and synthesize results.
```

Sub-agent (`./helpers/researcher.ai`):

```yaml
---
description: Searches the web and gathers information
toolName: researcher
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
output:
  format: json
  schema:
    type: object
    properties:
      findings:
        type: array
        items:
          type: string
      sources:
        type: array
        items:
          type: string
---

Search for information on the requested topic. Return findings with sources.
```

---

## Configuration Reference

### agents

| Property | Value |
|----------|-------|
| Type | `string` or `string[]` |
| Default | `[]` (no sub-agents) |
| Valid values | Relative or absolute paths to `.ai` files |

**Description**: Sub-agent `.ai` files to load as tools. Each sub-agent becomes callable via `agent__<toolName>`.

**What it affects**:
- Available sub-agent tools
- Multi-agent composition capabilities
- Agent hierarchy and recursion
- Resource usage (each sub-agent is a separate session)

**Example**:
```yaml
---
# Single sub-agent
agents: ./helpers/researcher.ai

# Multiple sub-agents
agents:
  - ./helpers/researcher.ai
  - ./helpers/writer.ai
  - ./helpers/analyst.ai

# Absolute path
agents:
  - /path/to/shared/validator.ai
---
```

**Path Resolution**:
- **Relative paths** resolve from the parent agent's directory
- **Absolute paths** are used as-is

**Tool Naming**:

The tool name is `agent__<toolName>`:
- If sub-agent has `toolName: researcher`, parent uses `agent__researcher`
- If no `toolName`, derived from filename: `my-agent.ai` → `agent__my_agent`

---

## How Sub-Agents Work

### Execution Model

1. Parent agent decides to call a sub-agent (via tool call)
2. A fresh `AIAgentSession` is created for the sub-agent
3. Sub-agent runs independently with its own:
   - Model configuration
   - Tool access
   - Turn limits
   - Conversation state
4. Sub-agent returns final output to parent as tool result
5. Parent continues with the result

### Communication

**Parent → Sub-Agent**:
```
Tool call: agent__researcher
Input: { "prompt": "Research AI trends in 2024" }
```

**Sub-Agent → Parent**:
```
Tool result: {
  "findings": ["AI spending increased 50%", "LLMs dominate"],
  "sources": ["https://...", "https://..."]
}
```

### Isolation

Each sub-agent session is completely isolated:
- Own model and provider instances
- Own MCP server connections
- Own turn count and limits
- Own accounting (aggregated to parent)

The parent cannot:
- Access sub-agent's internal state
- Modify sub-agent's conversation
- Share tools directly with sub-agent

### Tool Call Format

When parent calls a sub-agent:

```json
{
  "tool": "agent__researcher",
  "input": {
    "prompt": "Research AI trends"
  }
}
```

If sub-agent has `input` schema, the input must match:

```json
{
  "tool": "agent__researcher",
  "input": {
    "query": "AI trends",
    "maxResults": 10
  }
}
```

---

## Sub-Agent Requirements

### Required: description

Every sub-agent MUST have a `description`:

```yaml
---
description: Searches the web and summarizes findings
---
```

Without `description`, the agent loader throws an error. The description is used as the tool description for the parent.

### Recommended: toolName

Explicit `toolName` is safer than relying on filename:

```yaml
---
description: Web research specialist
toolName: web_researcher
---
```

### Recommended: output specification

Define expected output format:

```yaml
---
description: Company analyzer
toolName: company_analyzer
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      analysis:
        type: string
---
```

### Optional: input specification

For structured input, define an input schema:

```yaml
---
description: Data processor
toolName: data_processor
input:
  format: json
  schema:
    type: object
    properties:
      data:
        type: array
      operation:
        type: string
        enum: ["sum", "average", "max", "min"]
    required:
      - data
      - operation
---
```

---

## Common Patterns

### Specialist Team

Parent coordinates specialists:

**Parent** (`coordinator.ai`):
```yaml
---
description: Research project coordinator
models:
  - anthropic/claude-sonnet-4-20250514
agents:
  - ./specialists/researcher.ai
  - ./specialists/analyst.ai
  - ./specialists/writer.ai
maxTurns: 25
---

You coordinate research projects.

Specialists:
- `agent__researcher`: Gathers information
- `agent__analyst`: Analyzes data
- `agent__writer`: Writes reports

Workflow:
1. Gather information with researcher
2. Analyze with analyst
3. Create report with writer
```

### Pipeline Processing

Sequential processing through agents:

```yaml
---
description: Document processing pipeline
models:
  - openai/gpt-4o
agents:
  - ./pipeline/extractor.ai
  - ./pipeline/validator.ai
  - ./pipeline/formatter.ai
maxTurns: 15
---

Process documents through the pipeline:
1. Extract data with `agent__extractor`
2. Validate with `agent__validator`
3. Format with `agent__formatter`
```

### Shared Sub-Agent

Same sub-agent used by multiple parents:

**Shared** (`/shared/fact_checker.ai`):
```yaml
---
description: Verifies facts against reliable sources
toolName: fact_checker
models:
  - openai/gpt-4o
tools:
  - brave
output:
  format: json
  schema:
    type: object
    properties:
      verified:
        type: boolean
      explanation:
        type: string
---
```

**Parent A** uses it:
```yaml
agents:
  - /shared/fact_checker.ai
```

**Parent B** uses it:
```yaml
agents:
  - /shared/fact_checker.ai
```

### Nested Sub-Agents

Sub-agents can have their own sub-agents:

**Level 1** (`main.ai`):
```yaml
---
agents:
  - ./research/coordinator.ai
---
```

**Level 2** (`./research/coordinator.ai`):
```yaml
---
agents:
  - ./helpers/searcher.ai
  - ./helpers/summarizer.ai
---
```

**Notes on nesting**:
- Recursion is detected and prevented (A→B→A)
- Max depth is configurable (default ~3)
- Deep nesting increases latency and cost

### Structured Input/Output

Sub-agent with validated I/O:

```yaml
---
description: Company analyzer returning structured data
toolName: company_analyzer
models:
  - openai/gpt-4o
input:
  format: json
  schema:
    type: object
    properties:
      company:
        type: string
        description: Company name or domain
    required:
      - company
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      industry:
        type: string
      employees:
        type: number
      summary:
        type: string
    required:
      - name
      - summary
---

Analyze the provided company and return structured data.
```

Parent calls it:
```json
{
  "tool": "agent__company_analyzer",
  "input": {
    "company": "Acme Corp"
  }
}
```

---

## Troubleshooting

### "Sub-agent missing 'description'"

**Problem**: Loading sub-agent fails.

**Cause**: Sub-agents require `description` for tool listing.

**Solution**: Add description to sub-agent:
```yaml
---
description: Handles data processing tasks
models:
  - openai/gpt-4o
---
```

### "Duplicate toolName"

**Problem**: Two sub-agents have the same tool name.

**Cause**: Same explicit `toolName` or derived from similar filenames.

**Solution**: Use unique `toolName` values:
```yaml
# researcher-v1.ai
---
toolName: researcher_v1
---

# researcher-v2.ai
---
toolName: researcher_v2
---
```

### "Recursive agent reference"

**Problem**: Error about circular agent reference.

**Cause**: Agent A includes Agent B which includes Agent A.

**Solution**: Remove the circular reference:
```yaml
# A.ai includes B.ai
# B.ai should NOT include A.ai
```

### Sub-Agent Not Found

**Problem**: Parent can't find sub-agent file.

**Cause**: Path is incorrect or file doesn't exist.

**Solution**: Check path resolution:
```yaml
# Relative to parent's directory
agents:
  - ./helpers/researcher.ai  # Must exist relative to parent

# Or use absolute path
agents:
  - /absolute/path/to/researcher.ai
```

### Parent Not Using Sub-Agent

**Problem**: Parent agent never calls the sub-agent.

**Causes**:
1. Sub-agent's `description` unclear
2. Parent prompt doesn't mention the sub-agent
3. Task doesn't require delegation

**Solutions**:
1. Improve sub-agent description
2. Mention sub-agent in parent prompt:
   ```yaml
   Available: `agent__researcher` for web research
   ```
3. Make delegation explicit in instructions

### Sub-Agent Hitting Turn Limit

**Problem**: Sub-agent reaches `maxTurns` without completing.

**Cause**: Task too complex for sub-agent's turn limit.

**Solutions**:
1. Increase sub-agent's `maxTurns`
2. Simplify sub-agent's task scope
3. Split into multiple sub-agents

### Input Validation Failed

**Problem**: Sub-agent rejects input from parent.

**Cause**: Parent's input doesn't match sub-agent's `input` schema.

**Solution**: Check schema and parent's call format:
```yaml
# Sub-agent expects:
input:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
    required:
      - query

# Parent must call with:
# { "query": "search term" }
```

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Agent-Files-Identity](Agent-Files-Identity) - toolName and description
- [Agent-Files-Contracts](Agent-Files-Contracts) - Input/output schemas
- [Agent-Files-Orchestration](Agent-Files-Orchestration) - Advisors, router, handoff patterns
- [Technical-Specs-Tool-System](Technical-Specs-Tool-System) - How tools work internally
