# Hidden CLI Options

CLI options not shown in `--help` output, used for specialized orchestration patterns.

---

## Table of Contents

- [Overview](#overview) - What hidden options are and why they exist
- [--advisors](#--advisors) - Parallel pre-run agents
- [--handoff](#--handoff) - Post-run execution chains
- [Frontmatter Configuration](#frontmatter-configuration) - Declarative alternatives to CLI
- [Router Feature](#router-feature) - Dynamic agent selection
- [Debugging](#debugging) - How to debug orchestration flows
- [Discovery](#discovery) - Finding all hidden options
- [Stability](#stability) - What to expect from these features
- [See Also](#see-also) - Related documentation

---

## Overview

Some CLI options are hidden from `--help` with `showInHelp: false`. These options are:

- **Fully functional**: Work as designed
- **Specialized**: For advanced orchestration patterns
- **Less stable**: May change between versions

Hidden options exist for features that:

1. Are still evolving
2. Have complex interactions
3. Are intended for power users

---

## --advisors

| Property | Value                         |
| -------- | ----------------------------- |
| Type     | `string[]` (frontmatter only) |
| Default  | Empty (no advisors)           |
| Hidden   | Yes                           |
| Scope    | Master agent only             |

**Description**: Run parallel pre-run agents whose outputs inject as `<advisory>` blocks into the main agent's prompt.

> **Note**: This option is only configurable via agent frontmatter. CLI argument is not supported.

### Configuration

Configure advisors in agent frontmatter:

```yaml
---
advisors:
  - advisor1.ai
  - advisor2.ai
---
```

### Behavior

1. Before the main session starts, advisor agents run in parallel
2. Each advisor receives the original user prompt
3. Advisor outputs are wrapped in `<advisory__HEX agent="agent-name">` blocks
4. The main agent receives an enriched prompt with all advisory blocks
5. Failed advisors contribute failure notices (not fatal to main session)

### Example

```yaml
---
# analyzer.ai
description: Analyzes customer complaints
models:
  - openai/gpt-4o
advisors:
  - context-gatherer.ai
  - background-researcher.ai
---
```

**What the main agent sees**:

```xml
<advisory__HEX agent="context-gatherer">
Customer has been with us for 3 years, Enterprise tier, 500 nodes.
Last support ticket was about alerting configuration.
</advisory__HEX>

<advisory__HEX agent="background-researcher">
Similar complaints often relate to data retention settings or
query timeout configuration.
</advisory__HEX>

<original_user_request__HEX>
Analyze the customer complaint about slow performance
</original_user_request__HEX>
```

### Use Cases

- Gather context from specialized agents before main analysis
- Pre-analyze the task from different perspectives
- Parallel research before main execution
- Enrich prompts with domain knowledge

### Notes

- Advisors run independently
- Advisory blocks are informational, not actionable
- Failed advisors contribute failure notices (main session continues)
- Advisors receive parent session configuration including tools and defaults
- XML tags include random nonce suffixes (`__HEX`) for prompt uniqueness and security

---

## --handoff

| Property | Value                       |
| -------- | --------------------------- |
| Type     | `string` (frontmatter only) |
| Default  | None                        |
| Hidden   | Yes                         |
| Scope    | Master agent only           |

**Description**: Post-run execution chain for multi-stage workflows. The handoff agent receives the main agent's output and produces the final response.

> **Note**: This option is only configurable via agent frontmatter. CLI argument is not supported.

### Configuration

Configure handoff in agent frontmatter:

```yaml
---
handoff: reporter.ai
---
```

### Behavior

1. Main session runs to completion
2. If successful, handoff agent receives:
   - `<original_user_request__HEX>` block with the original query
   - `<response__HEX agent="agent-name">` block with main agent's output
3. Handoff agent produces the final output

### Example

```yaml
# data-analyzer.ai
---
description: Analyzes data and passes to formatter
handoff: executive-reporter.ai
---
```

**What the handoff agent sees**:

```xml
<response__HEX agent="analyzer">
Revenue analysis shows:
- October: $1.2M (+5% MoM)
- November: $1.4M (+17% MoM)
- December: $1.1M (-21% MoM)

The December dip correlates with holiday seasonality.
Year-over-year Q4 growth is 23%.
</response__HEX>

<original_user_request__HEX>
What's our Q4 revenue trend?
</original_user_request__HEX>
```

### Use Cases

- Two-stage workflows (analyze -> report)
- Post-processing of agent output
- Format transformation (e.g., JSON -> narrative)
- Quality assurance passes
- Translation or localization

### Notes

- Handoff result becomes the final output to the user
- Can chain multiple stages via frontmatter
- Handoff agent has full tool access
- XML tags include random nonce suffixes (`__HEX`) for prompt uniqueness and security

---

## Frontmatter Configuration

Orchestration is configured in agent frontmatter:

```yaml
---
models:
  - openai/gpt-4o
advisors:
  - context-gatherer.ai
  - background-researcher.ai
handoff: final-reporter.ai
---
Your task prompt here.
```

> **Note**: Orchestration options (`advisors`, `handoff`, `router`) are only available via frontmatter configuration. CLI arguments for these options are not supported.

---

## Router Feature

Router is a related feature for dynamic agent selection (not a CLI flag).

### Configuration

```yaml
---
models:
  - openai/gpt-4o
router:
  destinations:
    - analyzer.ai
    - summarizer.ai
    - translator.ai
---
Select the appropriate agent for this task.
```

### Behavior

1. Router agent receives the user query
2. Model chooses which destination via `router__handoff-to` tool
3. Selected agent runs with the original query
4. Selected agent's output becomes the final response

### Use Cases

- Intent classification and routing
- Skill-based agent selection
- Load distribution across specialized agents

---

## Debugging

### Enable Verbose Output

```bash
ai-agent --agent main.ai --verbose "query"
```

### Check Orchestration Logs

Look for these log entries:

| Logger               | Description                             |
| -------------------- | --------------------------------------- |
| `agent:orchestrator` | Orchestration flow decisions and errors |

### Example Log Output

```
ERR [agent:orchestrator] router_destination_missing: unknown-destination
ERR [agent:orchestrator] router_handoff_failed: connection failed
ERR [agent:orchestrator] handoff_failed: timeout
```

---

## Discovery

### Find All Hidden Options

```bash
grep -r "showInHelp: false" src/options-registry.ts
```

### Current Hidden Options

| Option       | Purpose                  | Configuration Method |
| ------------ | ------------------------ | -------------------- |
| `--advisors` | Parallel pre-run agents  | Frontmatter only     |
| `--handoff`  | Post-run execution chain | Frontmatter only     |

> **Note**: These options are hidden from help and are only configurable via agent frontmatter. CLI arguments are not supported.

### Source Reference

Check `src/options-registry.ts` for the definitive list:

```typescript
strArrDef({
  key: "advisors",
  description: "Advisor agents to consult (list of .ai paths)",
  cli: { names: ["--advisors"], showInHelp: false },
  fm: { allowed: true, key: "advisors" },
  scope: "masterOnly",
  // ...
}),
strDef({
  key: "handoff",
  description: "Handoff target agent (.ai path)",
  cli: { names: ["--handoff"], showInHelp: false },
  fm: { allowed: true, key: "handoff" },
  scope: "masterOnly",
  // ...
}),
```

---

## Stability

| Aspect              | Status                                  |
| ------------------- | --------------------------------------- |
| **Functional**      | Work as designed                        |
| **Less Stable**     | May change without deprecation warnings |
| **Less Documented** | Source code is the authority            |

> **Tip:** When using hidden options in production, pin your ai-agent version and test upgrades thoroughly.

---

## See Also

- [Agent-Files-Orchestration](Agent-Files-Orchestration) - Orchestration configuration in frontmatter
- [Technical-Specs-Session-Lifecycle](Technical-Specs-Session-Lifecycle) - Session flow internals
- [Advanced](Advanced) - Advanced features overview
