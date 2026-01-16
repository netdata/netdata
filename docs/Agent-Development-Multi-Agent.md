# Multi-Agent Orchestration

Build complex workflows with advisors, routers, handoffs, and sub-agents. Each pattern solves different coordination challenges.

---

## Table of Contents

- [Overview](#overview) - The four orchestration patterns
- [Sub-Agents](#sub-agents-agent-as-tool) - Delegate tasks to specialized agents
- [Advisors](#advisors-parallel-pre-run) - Get parallel expert input before decisions
- [Router](#router-dynamic-delegation) - Let the LLM choose which agent handles a request
- [Handoff](#handoff-sequential-chain) - Pass output through a pipeline of agents
- [Combining Patterns](#combining-patterns) - Use multiple patterns together
- [Best Practices](#best-practices) - Guidelines for effective orchestration
- [See Also](#see-also) - Related documentation

---

## Overview

AI Agent supports four orchestration patterns:

| Pattern        | When to Use               | How it Works                                |
| -------------- | ------------------------- | ------------------------------------------- |
| **Sub-Agents** | Delegate specific tasks   | Parent calls child as a tool                |
| **Advisors**   | Get expert opinions first | Multiple agents run in parallel before main |
| **Router**     | Dynamic request routing   | LLM decides which handler to invoke         |
| **Handoff**    | Sequential processing     | Output passes to next agent                 |

**Execution order when combined:**

1. Advisors run in parallel
2. Main agent runs with advisory context
3. If router used, destination runs
4. Handoff receives final output

---

## Sub-Agents (Agent-as-Tool)

Any `.ai` file can become a callable tool. The parent orchestrates; children do specialized work.

### Configuration

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
- `agent__researcher`: Gathers information on any topic
- `agent__writer`: Creates polished content
- `agent__reviewer`: Reviews and improves content
```

### Sub-Agent Definition

Define clear input/output contracts for predictable behavior:

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

1. Parent loads sub-agent configurations at startup
2. Each sub-agent becomes a tool (named by filename without `.ai`, prefixed with `agent__`)
3. Parent calls them like any other tool: `{"name": "agent__researcher", "arguments": {"topic": "..."}}`
4. Sub-agent runs in a completely isolated session
5. Result returns to parent as tool response

**Key point:** Sub-agents have zero shared state with the parent. Each invocation is independent.

---

## Advisors (Parallel Pre-Run)

Run multiple agents in parallel BEFORE the main agent, injecting their outputs as context.

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

### Advisor Output Injection

Advisors run with the same user prompt. Their outputs are injected as XML blocks:

```xml
<advisory__XXXXXXXXXX agent="technical">
Technical analysis: The proposed solution uses a microservices architecture
which requires Kubernetes expertise...
</advisory__XXXXXXXXXX>

<advisory__XXXXXXXXXX agent="business">
Business impact: This would affect revenue by approximately 15% in Q3...
</advisory__XXXXXXXXXX>

<advisory__XXXXXXXXXX agent="legal">
Legal considerations: GDPR compliance requires explicit consent...
</advisory__XXXXXXXXXX>
```

The main agent sees all advisory blocks before processing.

**Note:** Tag names include a random nonce suffix (`__XXXXXXXXXX`) for uniqueness and security.

### Handling Advisor Failures

If an advisor fails (timeout, error), a synthetic advisory is created:

```xml
<advisory__XXXXXXXXXX agent="legal">
Advisor consultation failed for legal: timeout after 60000ms
</advisory__XXXXXXXXXX>
```

The main agent continues regardless - advisory failures don't block execution.

---

## Router (Dynamic Delegation)

Let the LLM analyze the request and decide which specialized agent should handle it.

### Configuration

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
- **./handlers/sales.ai**: Pricing, demos, features questions
- **./handlers/support.ai**: Technical issues, bugs, how-to questions
- **./handlers/billing.ai**: Invoices, payments, subscriptions

Use the `router__handoff-to` tool to delegate with the exact destination path.
```

### Router Tool

The router pattern exposes a special `router__handoff-to` tool:

```json
{
  "name": "router__handoff-to",
  "arguments": {
    "agent": "./handlers/support.ai",
    "message": "User is experiencing login issues on mobile"
  }
}
```

**Arguments:**

- `agent` (required): Destination agent path (exact string from `router.destinations`)
- `message` (optional): Context to pass to the destination

### Message Injection

The optional `message` becomes an advisory for the destination:

```xml
<advisory__XXXXXXXXXX agent="router">
User is experiencing login issues on mobile
</advisory__XXXXXXXXXX>
```

The advisory agent is the name of the agent that made the routing decision. This helps the destination agent understand why it was invoked.

---

## Handoff (Sequential Chain)

Pass output to another agent after completion. Use for pipelines like: draft -> review -> finalize.

### Configuration

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
Review and improve the draft. The draft is in `<response__XXXXXXXXXX>` tags.
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

The previous agent's output is wrapped in XML:

```xml
<response__XXXXXXXXXX agent="draft">
Here is the draft content I created...
</response__XXXXXXXXXX>
```

The receiving agent processes this wrapped content.

### Use Cases

- **Content pipeline**: draft -> edit -> proofread -> publish
- **Analysis chain**: gather-data -> analyze -> summarize
- **Approval flow**: propose -> review -> approve/reject

---

## Combining Patterns

Use multiple patterns together for complex workflows:

```yaml
---
models:
  - openai/gpt-4o
advisors:
  - ./advisors/context.ai # Gather context first
agents:
  - ./workers/researcher.ai # Callable workers
  - ./workers/analyst.ai
router:
  destinations: # Dynamic routing option
    - ./handlers/simple.ai
    - ./handlers/complex.ai
handoff: ./finalizer.ai # Post-process result
---
```

### Execution Flow

1. **Advisors** (`context.ai`) run in parallel - results injected
2. **Main agent** runs with advisory context
3. Main agent may call **sub-agents** (`researcher.ai`, `analyst.ai`) as tools
4. Main agent may use **router** to delegate to `simple.ai` or `complex.ai`
5. **Handoff** (`finalizer.ai`) receives and processes final output

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
      confidence:
        type: number
```

**Why:** Clear contracts make agents predictable and easier to debug.

### 2. Reasonable Depth

Avoid deep nesting (3+ levels). Flatten when possible:

```yaml
# Good: Flat structure
agents:
  - researcher.ai
  - writer.ai
  - reviewer.ai

# Avoid: Deep nesting
# orchestrator -> manager -> worker -> helper
```

**Why:** Deep nesting increases latency, cost, and debugging complexity.

### 3. Timeout Awareness

Sub-agents have their own timeouts. Set appropriate limits:

```yaml
---
agents:
  - ./slow-research.ai
llmTimeout: 300000 # 5 minutes for complex sub-tasks
toolTimeout: 180000 # 3 minutes per tool (including sub-agents)
---
```

**Why:** Sub-agent invocations count as tool calls; they need adequate time.

### 4. Error Handling

Sub-agent failures return error results. Handle gracefully in the prompt:

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

**Why:** The parent agent sees error responses; guide it on recovery.

### 5. Single Responsibility

Each agent should do one thing well:

```yaml
# Good: Focused agents
- researcher.ai # Only research
- synthesizer.ai # Only synthesis
- formatter.ai # Only formatting

# Avoid: Kitchen-sink agents
- do-everything.ai # Research, analyze, format, publish...
```

**Why:** Focused agents are easier to test, debug, and reuse.

---

## See Also

- [Agent Files: Sub-Agents](Agent-Files-Sub-Agents) - Configuration reference
- [Agent Files: Orchestration](Agent-Files-Orchestration) - Advisors, router, handoff config
- [Technical Specs: Architecture](Technical-Specs-Architecture) - System internals
- [specs/MULTI-AGENT.md](specs/MULTI-AGENT.md) - Detailed design document
- [AI Agent Configuration Guide](skills/ai-agent-configuration.md) - Complete reference
