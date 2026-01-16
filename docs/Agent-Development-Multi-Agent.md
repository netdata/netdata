# Multi-Agent Orchestration

Build complex workflows with advisors, routers, handoffs, and sub-agents.

---

## Overview

AI Agent supports four orchestration patterns:

| Pattern | When | How |
|---------|------|-----|
| **Sub-Agents** | Delegate tasks | `agents:` in frontmatter |
| **Advisors** | Get parallel input | `advisors:` pre-run |
| **Router** | Dynamic delegation | `router__handoff-to` tool |
| **Handoff** | Chain execution | `handoff:` post-run |

---

## Sub-Agents (Agent-as-Tool)

Any `.ai` file can become a callable tool.

### Define Sub-Agents

```yaml
# orchestrator.ai
---
models:
  - openai/gpt-4o
agents:
  - ./specialists/researcher.ai
  - ./specialists/writer.ai
  - ./specialists/reviewer.ai
maxTurns: 20
---
You coordinate a team of specialists.

## Your Team

Call these tools to delegate work:
- `researcher`: Gathers information on any topic
- `writer`: Creates polished content
- `reviewer`: Reviews and improves content
```

### Sub-Agent Definition

```yaml
# specialists/researcher.ai
---
description: Research specialist - finds and synthesizes information
models:
  - openai/gpt-4o
tools:
  - brave
  - fetcher
input:
  type: object
  properties:
    topic:
      type: string
  required:
    - topic
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
Research the given topic thoroughly.
```

### How It Works

1. Parent registers sub-agents
2. Sub-agents become tools (named by filename without `.ai`)
3. Parent calls them like any other tool
4. Sub-agent runs in isolated session
5. Result returns to parent

---

## Advisors (Parallel Pre-Run)

Run multiple agents in parallel BEFORE the main agent, injecting their outputs.

### Use Case

Get expert opinions before making a decision:

```yaml
# decision-maker.ai
---
models:
  - openai/gpt-4o
advisors:
  - ./advisors/technical.ai
  - ./advisors/business.ai
  - ./advisors/legal.ai
---
You make decisions based on expert input.

You will receive advisory blocks from specialists.
Consider all perspectives before deciding.
```

### Advisor Output

Advisors run with the same user prompt. Their outputs are injected as:

```xml
<advisory agent="technical">
Technical analysis: The proposed solution uses...
</advisory>

<advisory agent="business">
Business impact: This would affect revenue by...
</advisory>
```

### Advisor Failures

If an advisor fails, a synthetic advisory is created:

```xml
<advisory agent="legal" status="failed">
Advisory unavailable: timeout after 60000ms
</advisory>
```

The main agent continues regardless.

---

## Router (Dynamic Delegation)

Let the LLM decide which agent to delegate to.

### Define Router

```yaml
# router.ai
---
models:
  - openai/gpt-4o
router:
  destinations:
    - ./handlers/sales.ai
    - ./handlers/support.ai
    - ./handlers/billing.ai
---
You are a request router.

Analyze the user's request and route to the appropriate handler:
- **sales**: Pricing, demos, features questions
- **support**: Technical issues, bugs, how-to questions
- **billing**: Invoices, payments, subscriptions

Use the `router__handoff-to` tool to delegate.
```

### Router Tool

The router exposes `router__handoff-to`:

```json
{
  "name": "router__handoff-to",
  "arguments": {
    "agent": "support",
    "message": "User is experiencing login issues on mobile"
  }
}
```

### Message Injection

The optional `message` becomes an advisory for the target:

```xml
<advisory agent="router">
User is experiencing login issues on mobile
</advisory>
```

---

## Handoff (Sequential Chain)

Pass output to another agent after completion.

### Use Case

Content pipeline:

```yaml
# draft.ai
---
models:
  - openai/gpt-4o
handoff: ./review.ai
---
Create a first draft of the requested content.
```

```yaml
# review.ai
---
models:
  - openai/gpt-4o
handoff: ./final.ai
---
Review and improve the draft. The draft is in <response> tags.
```

```yaml
# final.ai
---
models:
  - openai/gpt-4o
---
Produce the final polished version.
```

### Handoff Payload

The previous agent's output is wrapped:

```xml
<response agent="draft">
Here is the draft content...
</response>
```

---

## Combining Patterns

### Orchestrator with All Patterns

```yaml
---
models:
  - openai/gpt-4o
advisors:
  - ./advisors/context.ai      # Gather context first
agents:
  - ./workers/researcher.ai    # Callable workers
  - ./workers/analyst.ai
router:
  destinations:                 # Dynamic routing option
    - ./handlers/simple.ai
    - ./handlers/complex.ai
handoff: ./finalizer.ai        # Post-process result
---
```

### Execution Order

1. **Advisors** run in parallel
2. **Main agent** runs with advisory context
3. If router used, **destination** runs
4. **Handoff** receives final output

---

## Best Practices

### 1. Clear Interfaces

Define input/output schemas for sub-agents:

```yaml
# sub-agent
input:
  type: object
  properties:
    task:
      type: string
  required:
    - task
output:
  format: json
  schema:
    type: object
    properties:
      result:
        type: string
```

### 2. Reasonable Depth

Avoid deep nesting (3+ levels). Flatten when possible:

```yaml
# Good: Flat structure
agents:
  - researcher.ai
  - writer.ai
  - reviewer.ai

# Avoid: Deep nesting
# orchestrator → manager → worker → helper
```

### 3. Timeout Awareness

Sub-agents have their own timeouts. Set appropriate limits:

```yaml
---
agents:
  - ./slow-research.ai
llmTimeout: 300000    # 5 minutes for complex sub-tasks
toolTimeout: 180000   # 3 minutes per tool (including sub-agents)
---
```

### 4. Error Handling

Sub-agent failures return error results. Handle gracefully:

```yaml
---
models:
  - openai/gpt-4o
agents:
  - ./risky-operation.ai
---
If a sub-agent fails, explain the failure and suggest alternatives.
Don't retry failed operations without user approval.
```

---

## See Also

- [specs/MULTI-AGENT.md](specs/MULTI-AGENT.md) - Detailed design document
- [AI Agent Configuration Guide](skills/ai-agent-configuration.md) - Complete reference
