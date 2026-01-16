# Agent Identity Configuration

Configure how your agent identifies itself and how it appears when used as a sub-agent tool.

---

## Table of Contents

- [Overview](#overview) - What identity keys control
- [Quick Example](#quick-example) - Minimal identity configuration
- [Configuration Reference](#configuration-reference) - All identity keys with full details
- [Common Patterns](#common-patterns) - Typical identity configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Identity keys define:
- **What** the agent does (`description`)
- **How** to use it (`usage`)
- **Name** when exposed as a tool (`toolName`)

These keys are especially important for **sub-agents** - agents called by other agents as tools. The parent agent sees the `description` and `usage` when deciding how to use the sub-agent.

**User question answered**: "How do I name my agent?" / "How do I describe what my agent does?"

---

## Quick Example

Minimal identity for a standalone agent:

```yaml
---
description: A helpful assistant that answers questions
models:
  - openai/gpt-4o
---
```

Complete identity for a sub-agent:

```yaml
---
description: Analyzes company financials and returns structured reports
usage: "Provide a company name or domain. Returns analysis in JSON format."
toolName: company_analyzer
models:
  - openai/gpt-4o
output:
  format: json
  schema:
    type: object
    properties:
      company:
        type: string
      analysis:
        type: string
---
```

---

## Configuration Reference

### description

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | None |
| Required | **Yes** (for sub-agents) |

**Description**: Short description of what the agent does. Shown in tool listings when the agent is exposed as a sub-agent tool.

**What it affects**:
- Displayed to parent agents when this agent is a sub-agent tool
- Used in `--list-tools` output
- Helps the LLM understand when to use this agent
- Required for sub-agents (agent loader throws an error if missing)

**Example**:
```yaml
---
description: Analyzes company financials and returns structured reports
---
```

**Best Practices**:
- Keep it concise (one sentence, under 100 characters)
- Describe the agent's purpose, not its implementation
- Focus on what it does, not how it works
- Use action verbs ("Analyzes...", "Searches...", "Generates...")

**Good Examples**:
```yaml
description: Searches the web and summarizes findings
description: Analyzes code for security vulnerabilities
description: Translates text between languages
```

**Bad Examples**:
```yaml
# Too vague
description: A helpful agent

# Too long
description: This agent uses GPT-4 to analyze company data by calling multiple APIs and then formats the results into a structured JSON response

# Implementation details
description: Uses Claude 3.5 Sonnet with Brave search MCP
```

---

### usage

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | None |
| Required | No |

**Description**: Usage instructions for users or parent agents. Describes how to interact with the agent.

**What it affects**:
- Displayed in tool documentation
- Helps parent agents understand how to call this agent
- Shown in `--help` output when running the agent directly
- Included in the tool's input schema description

**Example**:
```yaml
---
usage: "Provide a company name or domain. Returns financial analysis in JSON."
---
```

```yaml
---
usage: |
  Input: A research question or topic
  Output: Structured findings with sources
  Example: "What are the latest trends in AI?"
---
```

**Best Practices**:
- Describe expected input format
- Describe expected output format
- Include an example if helpful
- Be specific about what the agent can and cannot do

**Good Examples**:
```yaml
usage: "Provide a URL to fetch and summarize"
usage: "Ask any research question. Returns findings with citations."
usage: |
  Input: Code snippet to analyze
  Returns: Security issues and recommendations
```

---

### toolName

| Property | Value |
|----------|-------|
| Type | `string` |
| Default | Derived from filename |
| Required | No (but recommended for sub-agents) |

**Description**: Stable identifier when exposing the agent as a callable tool. Parent agents call this agent using `agent__<toolName>`.

**What it affects**:
- Tool name exposed to parent agents
- Must be unique among sibling sub-agents
- Used in tool calls: `agent__<toolName>`
- Affects prompt references and debugging

**Example**:
```yaml
---
toolName: company_researcher
---
```

When a parent agent calls this sub-agent, it uses:
```
Tool: agent__company_researcher
Input: { "prompt": "Analyze Acme Corp" }
```

**Default Behavior**:

If `toolName` is omitted, it's derived from the filename:

| Filename | Derived toolName |
|----------|-----------------|
| `company-researcher.ai` | `company_researcher` |
| `web_search.ai` | `web_search` |
| `DataAnalyzer.ai` | `dataanalyzer` |

**Naming Rules**:
- Use lowercase letters, numbers, and underscores
- No spaces, hyphens are converted to underscores
- Cannot use reserved names: `final_report`, `task_status`, `batch`
- Must be unique among sibling sub-agents

**Best Practices**:
- Explicitly set `toolName` for sub-agents (safer for refactoring)
- Use descriptive, action-oriented names
- Use underscores between words
- Keep it short but meaningful

**Good Examples**:
```yaml
toolName: web_researcher
toolName: code_analyzer
toolName: data_extractor
toolName: report_generator
```

**Bad Examples**:
```yaml
# Reserved name
toolName: final_report

# Spaces (invalid)
toolName: web researcher

# Too generic
toolName: helper

# Too long
toolName: analyze_company_financials_and_generate_structured_reports
```

---

## Common Patterns

### Standalone Agent (No Sub-Agents)

For agents run directly from CLI, only `description` is needed:

```yaml
---
description: General-purpose assistant
models:
  - openai/gpt-4o
---

You are a helpful assistant. Answer questions clearly.
```

### Sub-Agent for Parent Orchestration

When an agent will be called by other agents:

```yaml
---
description: Searches the web for current information
usage: "Provide a search query. Returns relevant findings."
toolName: web_searcher
models:
  - openai/gpt-4o
tools:
  - brave
output:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
      results:
        type: array
        items:
          type: object
          properties:
            title:
              type: string
            url:
              type: string
            snippet:
              type: string
---

Search for the requested information and return structured results.
```

### Multiple Sub-Agents in Same Directory

When you have multiple sub-agents, explicit `toolName` prevents conflicts:

**researchers/company.ai**:
```yaml
---
description: Researches company information
toolName: company_researcher
---
```

**researchers/market.ai**:
```yaml
---
description: Researches market trends
toolName: market_researcher
---
```

**Parent agent uses them via**:
- `agent__company_researcher`
- `agent__market_researcher`

---

## Troubleshooting

### "Sub-agent missing 'description'"

**Problem**: Loading a sub-agent fails with missing description error.

**Cause**: Sub-agents require a `description` so parent agents know what the tool does.

**Solution**: Add `description` to the sub-agent's frontmatter:
```yaml
---
description: Handles data processing tasks
models:
  - openai/gpt-4o
---
```

### "Duplicate toolName"

**Problem**: Two sub-agents have the same `toolName`.

**Cause**: Either explicitly set to the same name or derived from similar filenames.

**Solution**: Use explicit unique `toolName` values:
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

### "Reserved toolName"

**Problem**: Error about using a reserved tool name.

**Cause**: Using `final_report`, `task_status`, or `batch` as `toolName`.

**Solution**: Choose a different name:
```yaml
# Wrong
toolName: final_report

# Correct
toolName: report_generator
```

### Parent Agent Not Using Sub-Agent Correctly

**Problem**: Parent agent doesn't call the sub-agent or misuses it.

**Cause**: Often due to unclear `description` or missing `usage`.

**Solution**: Improve identity metadata:
```yaml
---
# Clear description tells parent WHAT this does
description: Searches company databases and returns employee counts

# Clear usage tells parent HOW to call it
usage: "Provide company name. Returns { company, employees }."

toolName: employee_counter
---
```

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents) - Configuring sub-agent delegation
- [Agent-Files-Contracts](Agent-Files-Contracts) - Input/output schemas for sub-agents
- [CLI-Running-Agents](CLI-Running-Agents) - Running agents from command line
