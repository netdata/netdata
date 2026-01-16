# Hidden CLI Options

CLI options not shown in `--help` output.

---

## Overview

Some CLI options are hidden from `--help` with `showInHelp: false`. They're for specialized use cases and may change between versions.

---

## --advisors

**Purpose**: Run parallel pre-run agents whose outputs inject as `<advisory>` blocks.

**Syntax**:
```bash
ai-agent --agent main.ai --advisors advisor1.ai,advisor2.ai "query"
```

**Behavior**:
1. Before the main session, advisor agents run in parallel
2. Each advisor receives the original user prompt
3. Advisor outputs are wrapped in `<advisory source="agent-path">` blocks
4. The main agent receives an enriched prompt with advisory blocks
5. Synthetic failures from advisors are included

**Use Cases**:
- Gather context from specialized agents
- Pre-analyze the task from different perspectives
- Parallel research before main execution

**Notes**:
- Advisors run independently (no tool access to main session)
- Advisory blocks are informational, not actionable
- Failed advisors contribute failure notices

---

## --handoff

**Purpose**: Post-run execution chain for multi-stage workflows.

**Syntax**:
```bash
ai-agent --agent analyzer.ai --handoff reporter.ai "query"
```

**Behavior**:
1. Main session runs to completion
2. If successful, handoff agent receives:
   - `<original_user_request>` block
   - `<response>` block with main agent's output
3. Handoff agent produces the final output

**Use Cases**:
- Two-stage workflows (analyze → report)
- Post-processing of agent output
- Format transformation (e.g., JSON → narrative)

**Notes**:
- Handoff only runs if main session succeeds
- Handoff result becomes the final output
- Can chain multiple stages via frontmatter

---

## Configuration in Frontmatter

Instead of CLI flags, you can configure these in agent frontmatter:

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

---

## Router (Related Feature)

Router is similar to handoff but with dynamic selection:

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

The model chooses which destination via `router__handoff-to` tool.

---

## Debugging

### Enable Verbose Output

```bash
ai-agent --agent main.ai --advisors advisor.ai --verbose "query"
```

### Check Orchestration Logs

Look for these log entries:
- `agent:orchestrator` - Orchestration flow
- `agent:advisor` - Advisor execution
- `agent:handoff` - Handoff execution
- `agent:router` - Router selection

---

## Discovery

To find all hidden options:

```bash
grep -r "showInHelp: false" src/options-registry.ts
```

Or check `src/options-registry.ts` directly.

---

## Stability

Hidden options are:
- **Functional**: They work as designed
- **Less Stable**: May change without deprecation warnings
- **Less Documented**: Source code is the authority

---

## See Also

- [Agent-Development-Multi-Agent](Agent-Development-Multi-Agent) - Multi-agent patterns
- [Technical-Specs-Session-Lifecycle](Technical-Specs-Session-Lifecycle) - Session flow

