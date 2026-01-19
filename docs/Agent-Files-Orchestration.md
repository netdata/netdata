# Multi-Agent Orchestration

Configure advanced multi-agent patterns: advisors for pre-consultation, routers for dynamic delegation, and handoff for post-processing.

---

## Table of Contents

- [Overview](#overview) - What orchestration patterns enable
- [Quick Example](#quick-example) - Basic orchestration examples
- [Advisors](#advisors) - Pre-execution consultation
- [Router](#router) - Dynamic agent delegation
- [Handoff](#handoff) - Post-execution processing
- [Combining Patterns](#combining-patterns) - Using multiple patterns together
- [Common Patterns](#common-patterns) - Typical orchestration configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Orchestration patterns run **outside** the main session loop:

- **Advisors**: Run in parallel BEFORE the main session, inject context
- **Router**: Enables the main session to hand off to a destination agent
- **Handoff**: Runs AFTER the main session, post-processes the output

These differ from `agents` (sub-agents), which are tools called DURING the session.

---

## Quick Example

### Advisors

```yaml
---
description: Agent with compliance and security advisors
models:
  - anthropic/claude-sonnet-4-20250514
advisors:
  - ./advisors/compliance.ai
  - ./advisors/security.ai
---

[Advisor outputs are injected here automatically]

You help users with their requests while respecting compliance and security guidance.
```

### Router

```yaml
---
description: Customer service router
models:
  - openai/gpt-4o-mini
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
    - ./handlers/general.ai
maxTurns: 3
temperature: 0
---
Analyze the request and route to the appropriate handler.
Use `router__handoff-to` with the destination tool name (frontmatter `toolName`, or derived from the destination filename).
```

### Handoff

```yaml
---
description: Research agent with formatting handoff
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
handoff: ./formatters/report.ai
---
Research the topic thoroughly. Results will be formatted automatically.
```

---

## Advisors

Advisors are agents that run **before** the main session. Their outputs are injected as context for the main agent.

### Configuration

| Property     | Value                  |
| ------------ | ---------------------- |
| Type         | `string` or `string[]` |
| Default      | `[]` (no advisors)     |
| Valid values | Paths to `.ai` files   |

```yaml
---
advisors:
  - ./advisors/compliance.ai
  - ./advisors/security.ai
---
```

### How Advisors Work

1. All advisors run **in parallel** before the main session (outside the main session's turn loop)
2. Each advisor receives the user's prompt
3. Advisor outputs are collected
4. Outputs are injected as tagged XML blocks at the start of the user prompt
5. Main session runs with advisory blocks in the prompt

### Execution Flow

```
User Request
     │
     ▼
┌─────────────────────────────┐
│    Advisors (parallel)       │
│  ┌─────────┐  ┌─────────┐   │
│  │Compliance│  │Security │   │
│  └────┬────┘  └────┬────┘   │
│       │            │         │
│       ▼            ▼         │
│   [advice 1]   [advice 2]    │
└─────────────────────────────┘
              │
              ▼
┌─────────────────────────────┐
│    Main Session              │
│  (with advisor context)      │
└─────────────────────────────┘
              │
              ▼
         Response
```

### Advisor Requirements

Advisors are regular `.ai` files. They should:

- Have a `description`
- Return useful advisory content (final_report or last assistant message)
- Be fast (they block the main session)

**Example advisor** (`./advisors/compliance.ai`):

```yaml
---
description: Reviews requests for compliance issues
models:
  - openai/gpt-4o-mini
maxTurns: 3
temperature: 0
---

Review the user's request for compliance issues.

If issues found:
- List each issue briefly
- Suggest modifications

If no issues: respond "No compliance issues identified."
```

### Advisor Failure Handling

If an advisor fails:

- It becomes a synthetic advisory block with error info
- Main session still runs
- Failure does NOT stop execution

### Best Practices

**Performance**:

- Keep advisors **fast** (low `maxTurns`, simple tasks)
- Use cheaper/faster models for advisors
- Design for parallel execution (no dependencies between advisors)

**Pattern: Pre-Classification**

Use advisors to classify user intent before the main agent runs:

```yaml
---
advisors:
  - ./advisors/intent-classifier.ai
---
React based on the classification in the advisory block.
Do not re-analyze the user's raw request.
```

The main agent trusts the advisor's classification rather than re-interpreting raw input.

**Pattern: Security Screening**

Use advisors to screen user input for injection attacks, PII, or policy violations:

```yaml
---
advisors:
  - ./advisors/security-screen.ai
  - ./advisors/pii-detector.ai
---
If security issues were flagged in advisories, refuse the request.
If PII was detected, ask user to remove it before proceeding.
```

The main agent reacts to **advisory flags**, not raw user input. This separates security logic from business logic.

**Pattern: Context Enrichment**

Use advisors to gather context the main agent needs:

```yaml
---
advisors:
  - ./advisors/user-history.ai
  - ./advisors/account-status.ai
---
```

Advisors fetch user history, account info, or other context in parallel while the main agent focuses on the task.

---

## Router

Router pattern enables an agent to dynamically delegate to specialized handlers.

### Configuration

| Property | Value                                       |
| -------- | ------------------------------------------- |
| Type     | `object` with `destinations: string[]`      |
| Default  | None (no routing)                           |
| Required | `destinations` array with at least one path |

```yaml
---
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
    - ./handlers/general.ai
---
```

### How Router Works

1. Router agent receives the user's request
2. Router analyzes and decides which destination to use
3. Router calls `router__handoff-to` tool with the destination to declare intent
4. Router session completes (its response is discarded if a destination is selected)
5. Orchestration layer spawns the destination agent (passing router's optional message as an advisory block, plus original user request)
6. Destination agent runs and produces output (this is the user-visible answer)
7. If a handoff is configured, the destination agent's output is passed to the handoff agent for processing; otherwise the destination agent's output is the final result

### The router\_\_handoff-to Tool

When `router.destinations` is configured, a special tool becomes available:

**Tool**: `router__handoff-to`
**Input**:

```json
{
  "agent": "billing", // Destination toolName (from frontmatter toolName or derived from filename)
  "message": "..." // Optional message to pass along
}
```

The `agent` parameter is an enum of destination tool names derived from `router.destinations` (frontmatter `toolName` override or sanitized filename). If provided, the `message` is converted to an advisory block and combined with the original user request as the destination agent's prompt.

### Router Example

**Router** (`router.ai`):

```yaml
---
description: Customer service router
models:
  - openai/gpt-4o-mini
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
    - ./handlers/general.ai
maxTurns: 3
temperature: 0
---

You are a customer service router.

Analyze the user's request and route to the appropriate handler:
- `./handlers/billing.ai`: Payment, invoices, subscription issues
- `./handlers/technical.ai`: Product bugs, API issues, integration help
- `./handlers/general.ai`: Everything else

Use the `router__handoff-to` tool with the appropriate destination tool name.
```

**Destination** (`./handlers/billing.ai`):

```yaml
---
description: Handles billing and payment issues
toolName: billing
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - stripe
---
You handle billing and payment issues.
```

### Router Best Practices

- Keep router agent **simple and fast** (few turns, simple logic)
- Use cheaper/faster models for routing decisions
- Provide clear routing criteria in the prompt
- Set low `temperature` for deterministic routing

---

## Handoff

Handoff runs an agent **after** the main session completes, for post-processing.

### Configuration

| Property     | Value                     |
| ------------ | ------------------------- |
| Type         | `string`                  |
| Default      | None (no handoff)         |
| Valid values | Single path to `.ai` file |

```yaml
---
handoff: ./formatters/report.ai
---
```

**Note**: Arrays are NOT supported. Only single handoff agent.

### How Handoff Works

1. Main session runs and produces output
2. Handoff agent receives:
   - Original user request
   - Main session's response (when no router) or destination's response (when using router)
3. Handoff agent processes and produces final output
4. Handoff's output becomes the final response

### Execution Flow

```
User Request
     │
     ▼
┌─────────────────────────────┐
│    Main Session              │
└─────────────────────────────┘
              │
              ▼
         Response
              │
              ▼
┌─────────────────────────────┐
│    Handoff Agent             │
│  (receives request +         │
│   main response)             │
└─────────────────────────────┘
              │
              ▼
       Final Response
```

### Handoff Example

**Main agent** (`research.ai`):

```yaml
---
description: Research agent
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
  - fetcher
handoff: ./formatters/executive-summary.ai
maxTurns: 15
---
Research the topic thoroughly. Gather data from multiple sources.
```

**Handoff agent** (`./formatters/executive-summary.ai`):

```yaml
---
description: Creates executive summaries
models:
  - openai/gpt-4o
maxTurns: 3
temperature: 0.3
---

You receive research findings.

Create an executive summary:
1. Key takeaways (3-5 bullet points)
2. Main findings
3. Recommendations

Keep it concise and actionable.
```

### Handoff with Router

If the main agent uses router AND handoff:

1. Router agent runs
2. Router delegates to destination
3. Destination agent runs
4. Handoff receives destination's output
5. Handoff produces final output

```
Request → Router → Destination → Handoff → Final Response
(Handoff processes the destination agent's output; router's response is discarded)
```

### Handoff Best Practices

**General**:

- Keep handoff agents focused and fast
- Don't add new information in handoff (it has all context)
- Single handoff only (no chaining via frontmatter)

**Pattern: Formatting**

Handoff keeps main agents focused on work, not presentation:

```yaml
# research.ai - focused on research only
---
handoff: ./formatters/executive-report.ai
---
Research thoroughly. Don't worry about formatting.
```

```yaml
# formatters/executive-report.ai
---
description: Formats research into executive summary
---
Transform the research into:
  - 3-5 key takeaways
  - Findings organized by theme
  - Action items
```

Main agents produce raw output; handoff handles presentation.

**Pattern: Sensitive Data Redaction**

Use handoff to scrub sensitive data before final output:

```yaml
# worker.ai
---
handoff: ./security/redactor.ai
---
```

```yaml
# security/redactor.ai
---
description: Redacts PII and sensitive data
---
Review the response and redact:
- Personal identifiable information (names, emails, phones)
- Internal system identifiers
- API keys or credentials
- Internal URLs or paths

Replace with [REDACTED] or generic placeholders.
```

Workers produce complete output; handoff sanitizes for external consumption.

**Pattern: Response Classification**

Use handoff to classify/tag the response:

```yaml
# support.ai
---
handoff: ./classifiers/response-tagger.ai
---
```

The handoff adds metadata like sentiment, category, escalation flags, or confidence scores to the response.

**Pattern: Quality Assurance**

Use handoff as a QA pass before delivery:

```yaml
# worker.ai
---
handoff: ./qa/reviewer.ai
---
```

```yaml
# qa/reviewer.ai
---
description: Reviews response for quality
---
Review the response for:
- Factual accuracy
- Completeness (all questions answered?)
- Tone appropriateness
- Grammar and clarity

If issues found, fix them. Do not add disclaimers about your review.
```

The QA handoff catches errors and improves quality without the main agent needing to self-review.

---

## Combining Patterns

You can use multiple orchestration patterns together.

### Advisors + Main Session

```yaml
---
advisors:
  - ./advisors/compliance.ai
  - ./advisors/security.ai
models:
  - anthropic/claude-sonnet-4-20250514
---
```

Flow: Advisors → Main Session → Response

### Router + Destinations

```yaml
---
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
---
```

Flow: Request → Router → Selected Destination → Response

### Main + Handoff

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
handoff: ./formatters/report.ai
---
```

Flow: Request → Main Session → Handoff → Final Response

### Full Pipeline: Advisors + Router + Handoff

```yaml
---
advisors:
  - ./advisors/compliance.ai
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
handoff: ./formatters/report.ai
---
```

Flow:

1. Advisors run (parallel)
2. Router runs (with advisor context)
3. Router selects destination
4. Destination runs
5. Handoff processes destination output
6. Final response

### Orchestration vs Sub-Agents

| Feature          | Orchestration             | Sub-Agents              |
| ---------------- | ------------------------- | ----------------------- |
| **When runs**    | Outside main session loop | During main session     |
| **Control**      | Automatic (frontmatter)   | LLM decides (tool call) |
| **Relationship** | Pipeline stages           | On-demand delegation    |
| **Use case**     | Pre/post processing       | Task specialization     |

**Use orchestration when**:

- You need consistent pre/post processing
- Flow is deterministic
- Pattern is always applied

**Use sub-agents when**:

- LLM should decide when to delegate
- Multiple specialists available on-demand
- Dynamic task decomposition

---

## Common Patterns

### Compliance Gateway

All requests reviewed by compliance:

```yaml
---
description: Compliant assistant
advisors:
  - ./advisors/compliance-checker.ai
models:
  - anthropic/claude-sonnet-4-20250514
---
```

### Customer Service Router

Route to specialized handlers:

```yaml
---
description: Customer service entry point
models:
  - openai/gpt-4o-mini
router:
  destinations:
    - ./handlers/billing.ai
    - ./handlers/technical.ai
    - ./handlers/sales.ai
    - ./handlers/general.ai
temperature: 0
maxTurns: 3
---
```

### Research with Executive Summary

Research followed by formatting:

```yaml
---
description: Research with summary
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
handoff: ./formatters/executive-summary.ai
---
```

### Multi-Advisor Review

Multiple perspectives before response:

```yaml
---
description: Reviewed assistant
advisors:
  - ./advisors/legal.ai
  - ./advisors/security.ai
  - ./advisors/brand.ai
models:
  - anthropic/claude-sonnet-4-20250514
---
```

### Handoff Pipeline

Chain handoffs for multi-step processing:

```yaml
# support-entry.ai
---
handoff: ./pipeline/find-docs.ai
---
```

```yaml
# pipeline/find-docs.ai
---
tools:
  - docs-search
handoff: ./pipeline/search-tickets.ai
---
```

```yaml
# pipeline/search-tickets.ai
---
tools:
  - freshdesk
handoff: ./pipeline/redact-and-respond.ai
---
```

```yaml
# pipeline/redact-and-respond.ai
---
description: Final stage - redact and synthesize
---
```

Flow: `Request → Find Docs → Search Tickets → Redact & Respond`

Each stage focuses on one task; handoff chains them into a pipeline.

### Full Support Pipeline (Router + Advisors + Handoff)

The most powerful pattern combines everything:

```
User Request
     │
     ▼
┌─────────────────────────────────────────────┐
│  SCREENING ROUTER                           │
│  (advisors: security-screen.ai)             │
│                                             │
│  router.destinations:                       │
│    - ./handlers/reject.ai    ← blocked      │
│    - ./handlers/approved.ai  ← approved     │
└─────────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┐
        │                       │
        ▼                       ▼
   [REJECTED]              [APPROVED]
   "Request denied"             │
                                ▼
              ┌─────────────────────────────────────┐
              │  APPROVED HANDLER                   │
              │  (parallel advisors)                │
              │                                     │
              │  ┌─────────┐ ┌─────────┐ ┌───────┐ │
              │  │  Docs   │ │ Tickets │ │ Code  │ │
              │  │ Search  │ │ Search  │ │Analysis│ │
              │  └────┬────┘ └────┬────┘ └───┬───┘ │
              │       └───────────┼──────────┘     │
              │                   ▼                │
              │          Response Synthesis        │
              │                                    │
              │  handoff: ./redactor.ai            │
              └─────────────────────────────────────┘
                                │
                                ▼
              ┌─────────────────────────────────────┐
              │  REDACTOR (handoff)                 │
              │  - Redact PII                       │
              │  - Classify response                │
              │  - Add metadata                     │
              └─────────────────────────────────────┘
                                │
                                ▼
                         Final Response
```

**Implementation**:

```yaml
# support.ai (entry point - screening router)
---
description: Support request handler
models:
  - openai/gpt-4o-mini
advisors:
  - ./security/screen.ai
router:
  destinations:
    - ./handlers/reject.ai
    - ./handlers/approved.ai
temperature: 0
maxTurns: 3
---
Screen the request. Route to reject.ai if blocked, approved.ai if safe.
```

```yaml
# handlers/approved.ai (parallel research + synthesis)
---
description: Handles approved support requests
models:
  - anthropic/claude-sonnet-4-20250514
advisors:
  - ./research/docs-search.ai
  - ./research/ticket-search.ai
  - ./research/code-analysis.ai
handoff: ./security/redactor.ai
---
Synthesize a response using the research from advisors.
```

---

## Orchestration Transparency

**Important**: Orchestration is invisible to the user.

When a user talks to an agent, they don't know:

- Advisors ran before the conversation started
- A router decided which agent actually handles their request
- A handoff agent processed the response after

From the user's perspective, they're talking to **one agent**. The orchestration machinery is hidden.

### What Users See vs What Happens

| User Sees                  | What Actually Happens                                                           |
| -------------------------- | ------------------------------------------------------------------------------- |
| "Talking to support agent" | Screening advisor → Router → Handler with research advisors → Redaction handoff |
| "Got a response"           | 5+ agents collaborated invisibly                                                |
| "Fast answer"              | Advisors ran in parallel                                                        |

### Router Has Full Orchestration Access

A router agent can use **all orchestration methods simultaneously**:

| Method         | When It Runs      | Purpose                              |
| -------------- | ----------------- | ------------------------------------ |
| Advisors       | Before router     | Screen/classify incoming request     |
| Tools          | During router     | Gather info to make routing decision |
| Sub-agents     | During router     | Delegate subtasks                    |
| Router handoff | Router decides    | Select destination handler           |
| Final handoff  | After destination | Post-process the response            |

This makes router the most powerful orchestration pattern - it's a complete pipeline coordinator.

---

## Troubleshooting

### "Advisor failed but session continued"

**Expected behavior**: Advisor failures don't stop execution.

**If you need strict advisors**: Handle in the main agent's prompt:

```yaml
If compliance issues were identified, do not proceed.
```

### "router\_\_handoff-to not available"

**Problem**: Tool not appearing for router agent.

**Cause**: `router.destinations` not configured or empty.

**Solution**: Ensure destinations are defined:

```yaml
router:
  destinations:
    - ./handlers/one.ai
    - ./handlers/two.ai
```

### "Invalid destination"

**Problem**: Router tries to hand off to unknown destination.

**Cause**: Destination tool name doesn't match any configured destination tool name.

**Solution**: Check destination tool names (frontmatter `toolName` or derived from filename) match the router tool enum for your `router.destinations`:

```yaml
router:
  destinations:
    - ./handlers/billing.ai # Tool name defaults to "billing"
```

### "Handoff not receiving main response"

**Problem**: Handoff agent doesn't see main session output.

**Cause**: Main session failed or handoff misconfigured.

**Solution**: Check:

1. Main session completes successfully
2. Handoff path is correct
3. Single string (not array) for handoff

### "Multiple handoffs needed"

**Problem**: Want to chain multiple post-processors.

**Limitation**: Only single handoff supported in frontmatter.

**Workaround**: The handoff agent can have its own handoff:

```yaml
# main.ai
handoff: ./stage1.ai

# stage1.ai
handoff: ./stage2.ai
```

Or use sub-agents within handoff:

```yaml
# main.ai
handoff: ./post-processor.ai

# post-processor.ai
agents:
  - ./formatters/step1.ai
  - ./formatters/step2.ai
```

### Advisors Running Sequentially

**Problem**: Advisors seem slow, running one after another.

**Expected**: Advisors run in parallel.

**If slow**:

- Check advisor complexity
- Reduce advisor `maxTurns`
- Use faster models for advisors

---

## See Also

- [Agent-Files](Agent-Files.md) - Overview of .ai file structure
- [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents.md) - On-demand agent delegation (tools)
- [Agent-Files-Contracts](Agent-Files-Contracts.md) - Input/output for orchestration agents
- [Technical-Specs-Session-Lifecycle](Technical-Specs-Session-Lifecycle.md) - How sessions execute
