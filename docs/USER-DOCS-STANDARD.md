# User Documentation Standard

This document defines the quality standard for ai-agent user documentation. Every wiki page must meet these criteria to achieve A+ grade.

---

## Core Principles

### 1. User-Centered Design
- **Ask first**: What does the user want to accomplish when they land on this page?
- **No assumptions**: State prerequisites explicitly. Link to where users can learn them.
- **Respect time**: Users are busy. Get to the point. Make information scannable.

### 2. Complete Information
- Every configuration key must document: type, default value, valid values, what it affects, example usage
- Every concept must be explained before it's used
- Every page must be self-sufficient (with links to prerequisites, not inline explanations of unrelated concepts)

### 3. Progressive Disclosure
- Start with the most common use case
- Basic configuration first, advanced options after
- "Quick start" before "deep dive"

### 4. Real Examples
- Every feature needs a working example
- Examples should be copy-pasteable
- Show the expected output/behavior

---

## Page Types and Templates

### Type A: Landing/Index Pages
**Purpose**: Navigation and orientation
**Structure**:
1. One-sentence description of the section
2. "What you'll learn" bullet list
3. Page table with descriptions
4. "Prerequisites" section if any
5. "Quick links" to most common tasks

### Type B: Conceptual Pages
**Purpose**: Explain a concept or architecture
**Structure**:
1. **TL;DR** - 2-3 sentences explaining the concept
2. **Why this matters** - When/why users need this
3. **How it works** - Clear explanation with diagrams if helpful
4. **Key terms** - Define any jargon
5. **Examples** - Real-world usage
6. **See also** - Related pages

### Type C: Configuration Reference Pages
**Purpose**: Document configuration options
**Structure**:
1. **Overview** - What this configuration controls
2. **Quick example** - Minimal working configuration
3. **Configuration reference** - Table or detailed list of ALL options
4. **Common patterns** - Typical configurations for common use cases
5. **Troubleshooting** - Common mistakes and how to fix them
6. **See also** - Related pages

### Type D: How-To/Tutorial Pages
**Purpose**: Step-by-step instructions
**Structure**:
1. **Goal** - What you'll achieve
2. **Prerequisites** - What you need before starting (with links)
3. **Steps** - Numbered, clear, one action per step
4. **Verification** - How to confirm it worked
5. **Next steps** - Where to go from here

### Type E: Reference Pages
**Purpose**: Exhaustive technical reference
**Structure**:
1. **Overview** - What this reference covers
2. **Reference table/list** - Complete, alphabetized or logically grouped
3. **Details** - Expanded information for complex items
4. **See also** - Related references

---

## Configuration Documentation Rules

Every configuration key MUST include:

| Field | Required | Description |
|-------|----------|-------------|
| **Key name** | Yes | Exact name as used in config |
| **Type** | Yes | string, number, boolean, array, object |
| **Default** | Yes | Default value, or "Required" if no default |
| **Valid values** | Yes | Enumeration, range, or format description |
| **Description** | Yes | What this option does |
| **Affects** | Yes | What behavior/feature this controls |
| **Example** | Yes | Working example showing usage |
| **Notes** | If applicable | Gotchas, interactions with other options |

**Example of proper configuration documentation:**

```markdown
### maxTurns

| Property | Value |
|----------|-------|
| Type | `number` |
| Default | `25` |
| Valid values | `1` to `100` |
| Required | No |

**Description**: Maximum number of LLM turns (request-response cycles) before the agent must complete.

**What it affects**:
- Prevents infinite loops in agentic behavior
- On the final turn, tools are disabled and the agent is forced to provide a final answer
- Higher values allow more complex multi-step tasks but increase cost and time

**Example**:
```yaml
---
maxTurns: 15
---
```

**Notes**:
- If your agent consistently hits maxTurns without completing, consider: (1) simplifying the task, (2) improving the prompt, or (3) increasing the limit
- See [Context Management](Technical-Specs-Context-Management) for how turns interact with context window limits
```

---

## Writing Guidelines

### Table of Contents Requirement

Every page longer than 3 sections MUST have a Table of Contents immediately after the title/TL;DR:

```markdown
# Page Title

One-sentence description of what this page covers.

---

## Table of Contents

- [Section Name](#section-name) - Brief description of what's in this section
- [Another Section](#another-section) - What users will find here
- [See Also](#see-also) - Related pages

---
```

**TOC Rules**:
- Place after the opening description, before main content
- Use anchor links (`#section-name`) for in-page navigation
- Include a brief description (5-10 words) for each link
- Only list top-level sections (h2), not subsections
- Helps users immediately understand page structure and find what they need

### Voice and Tone
- **Direct**: "Configure the provider" not "You might want to configure the provider"
- **Active**: "The agent calls the tool" not "The tool is called by the agent"
- **Confident**: "Use X for Y" not "You could potentially use X for Y"
- **Respectful**: Assume users are intelligent but may be new to this specific tool

### Formatting Rules
- Use tables for configuration references (scannable)
- Use code blocks for all configuration examples
- Use bullet points for lists of 3+ items
- Use numbered lists only for sequential steps
- Bold key terms on first use
- Use admonitions sparingly: `> **Note:**`, `> **Warning:**`, `> **Tip:**`

### Linking Rules
- Link to prerequisites at the top of the page
- Link to related concepts on first mention
- Use relative links within wiki: `[Page Name](Page-Name)`
- Link to specs for deep technical details: `[specs/file.md](specs/file.md)`
- Never have a dead-end page - always provide "See also" or "Next steps"

---

## Quality Checklist (A+ Grade Requirements)

A page achieves A+ grade when ALL of these are true:

### Completeness
- [ ] All configuration options documented with full details
- [ ] All concepts explained before use
- [ ] Prerequisites stated and linked
- [ ] Working examples for every feature
- [ ] No placeholder text or TODOs

### Clarity
- [ ] A new user can understand the page without prior knowledge (beyond stated prerequisites)
- [ ] Technical terms defined or linked on first use
- [ ] Examples are realistic and copy-pasteable
- [ ] Expected behavior/output shown

### Navigation
- [ ] Table of Contents with anchor links (for pages with 3+ sections)
- [ ] Clear "See also" section with relevant links
- [ ] Breadcrumb context (what section this belongs to)
- [ ] Links to both simpler (prerequisites) and advanced (deep dives) content

### Accuracy
- [ ] Configuration defaults match actual code behavior
- [ ] Valid values match actual validation
- [ ] Examples tested and working

---

## Wiki Structure and Page Goals

The wiki is organized into focused chapters. Each page should be 100-300 lines and answer ONE primary question.

### Home.md
**Type**: Landing (A)
**Goal**: Orient users, help them find what they need quickly
**Users arrive knowing**: Nothing - this may be their first contact
**Users leave knowing**: What ai-agent is, where to start based on their goal

---

### Getting Started Chapter

For new users who need to install and run their first agent.

#### Getting-Started.md
**Type**: Landing (A)
**Goal**: Guide new users to the right starting point

#### Getting-Started-Installation.md
**Type**: How-To (D)
**Goal**: Get ai-agent installed and runnable
**Prerequisites**: Node.js 18+, npm

#### Getting-Started-Quick-Start.md
**Type**: How-To (D)
**Goal**: Run their first agent in under 5 minutes
**Prerequisites**: Installation complete, API key

#### Getting-Started-First-Agent.md
**Type**: How-To (D)
**Goal**: Build a useful agent with tools
**Prerequisites**: Quick-Start complete

#### Getting-Started-Environment-Variables.md
**Type**: Reference (E)
**Goal**: All environment variables that affect ai-agent

---

### Agent Files Chapter ⭐ CORE

**Daily reference** for users building and configuring agents. Each page covers ONE aspect of .ai file configuration.

#### Agent-Files.md
**Type**: Landing (A)
**Goal**: Understand .ai file structure and find the right configuration page
**Users arrive knowing**: They need to configure an agent
**Users leave knowing**: File structure, which page has their answer

#### Agent-Files-Identity.md
**Type**: Configuration Reference (C)
**Goal**: Configure agent identity and metadata
**Covers**: `description`, `usage`, `toolName`
**User question**: "How do I name my agent?" / "How do I describe what my agent does?"

#### Agent-Files-Models.md
**Type**: Configuration Reference (C)
**Goal**: Configure which LLMs the agent uses
**Covers**: `models`, fallback chains, model selection, reasoning models, caching
**User question**: "How do I change the model?" / "How do I set up fallbacks?"

#### Agent-Files-Tools.md
**Type**: Configuration Reference (C)
**Goal**: Configure tool access
**Covers**: `tools`, `toolsAllowed`, `toolsDenied`, MCP server references
**User question**: "How do I give my agent tools?" / "How do I restrict tools?"

#### Agent-Files-Sub-Agents.md
**Type**: Configuration Reference (C)
**Goal**: Configure sub-agent delegation
**Covers**: `agents`, nesting, communication patterns
**User question**: "How do I call another agent?" / "How do agents work together?"

#### Agent-Files-Orchestration.md
**Type**: Configuration Reference (C)
**Goal**: Configure multi-agent patterns
**Covers**: `advisors`, `router`, `handoff`
**User question**: "How do I set up advisors?" / "How do I route to different agents?"

#### Agent-Files-Behavior.md
**Type**: Configuration Reference (C)
**Goal**: Configure agent limits and sampling
**Covers**: `maxTurns`, `maxRetries`, `temperature`, `topP`, `topK`, timeouts
**User question**: "How do I limit turns?" / "How do I adjust temperature?"

#### Agent-Files-Contracts.md
**Type**: Configuration Reference (C)
**Goal**: Configure structured input/output
**Covers**: `input`, `output`, JSON schemas, validation
**User question**: "How do I get structured output?" / "How do I validate input?"

---

### System Prompts Chapter ⭐ CORE

**Daily reference** for writing effective prompts.

#### System-Prompts.md
**Type**: Landing (A)
**Goal**: Understand prompt structure and find the right topic
**Users arrive knowing**: Prompts go after frontmatter
**Users leave knowing**: Prompt anatomy, which page has their answer

#### System-Prompts-Writing.md
**Type**: How-To (D)
**Goal**: Write effective system prompts
**Covers**: Best practices, structure, common patterns, dos and don'ts
**User question**: "How do I write a good prompt?"

#### System-Prompts-Includes.md
**Type**: Configuration Reference (C)
**Goal**: Reuse prompt content across agents
**Covers**: `@include` syntax, resolution paths, nesting
**User question**: "How do I share prompt content?"

#### System-Prompts-Variables.md
**Type**: Reference (E)
**Goal**: All available prompt variables
**Covers**: Every variable, when available, examples
**User question**: "What variables can I use?" / "What does ${FORMAT} do?"

---

### CLI Reference Chapter ⭐ CORE

**Daily reference** for running agents from command line.

#### CLI.md
**Type**: Landing (A)
**Goal**: Find the right CLI topic
**Users arrive knowing**: They want to run agents from CLI
**Users leave knowing**: Which page has their answer

#### CLI-Running-Agents.md
**Type**: Reference (E)
**Goal**: Execute agents from command line
**Covers**: `--agent`, prompts, stdin, piping, output formats
**User question**: "How do I run an agent?" / "How do I pipe input?"

#### CLI-Debugging.md
**Type**: Reference (E)
**Goal**: Debug agent execution
**Covers**: `--verbose`, `--trace-*`, `--dry-run`, diagnostic output
**User question**: "Why isn't my agent working?" / "How do I see what's happening?"

#### CLI-Overrides.md
**Type**: Reference (E)
**Goal**: Override configuration at runtime
**Covers**: `--model`, `--temperature`, all runtime overrides
**User question**: "How do I use a different model?" / "How do I override settings?"

#### CLI-Scripting.md
**Type**: How-To (D)
**Goal**: Use ai-agent in scripts and automation
**Covers**: Exit codes, JSON output, error handling, batch processing
**User question**: "How do I use ai-agent in a script?"

---

### Configuration Chapter

For configuring providers, MCP servers, and system-wide settings.

#### Configuration.md
**Type**: Landing (A)
**Goal**: Find the right configuration topic

#### Configuration-Files.md
**Type**: Conceptual (B)
**Goal**: Understand config file resolution and layering
**Covers**: File locations, precedence, merging behavior

#### Configuration-Providers.md
**Type**: Configuration Reference (C)
**Goal**: Configure LLM providers
**Covers**: OpenAI, Anthropic, Ollama, OpenRouter, custom endpoints
**Critical**: Every provider type must have complete examples.

#### Configuration-MCP-Servers.md
**Type**: Configuration Reference (C)
**Goal**: Configure MCP tool servers
**Covers**: stdio, HTTP, SSE, WebSocket transports

#### Configuration-REST-Tools.md
**Type**: Configuration Reference (C)
**Goal**: Configure REST/OpenAPI tools
**Covers**: OpenAPI spec, authentication, endpoints

#### Configuration-Queues.md
**Type**: Configuration Reference (C)
**Goal**: Configure concurrency and rate limiting
**Covers**: Queue configuration, concurrency limits, backoff

#### Configuration-Pricing.md
**Type**: Configuration Reference (C)
**Goal**: Configure cost tracking
**Covers**: Pricing configuration, cost calculation, budget limits

---

### Headends Chapter

For deploying agents via different interfaces.

#### Headends.md
**Type**: Landing (A)
**Goal**: Understand deployment options

#### Headends-REST.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Deploy agents as REST APIs

#### Headends-MCP.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Expose agents as MCP tools

#### Headends-OpenAI-Compatible.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: OpenAI-compatible API endpoint

#### Headends-Anthropic-Compatible.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Anthropic-compatible API endpoint

#### Headends-Slack.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Deploy agents in Slack
**Critical**: Complex topic. Needs exhaustive coverage.

#### Headends-Embed.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Embed chat widget in websites

#### Headends-Library.md
**Type**: Configuration Reference (C) + How-To (D)
**Goal**: Embed ai-agent in Node.js applications

---

### Operations Chapter

For running agents in production.

#### Operations.md
**Type**: Landing (A)
**Goal**: Find operational information

#### Operations-Logging.md
**Type**: Configuration Reference (C)
**Goal**: Configure and understand logging

#### Operations-Debugging.md
**Type**: How-To (D)
**Goal**: Debug agent issues

#### Operations-Snapshots.md
**Type**: Conceptual (B) + How-To (D)
**Goal**: Capture and analyze session state

#### Operations-Telemetry.md
**Type**: Configuration Reference (C)
**Goal**: Configure metrics and tracing

#### Operations-Accounting.md
**Type**: Configuration Reference (C)
**Goal**: Track usage and costs

#### Operations-Troubleshooting.md
**Type**: Reference (E)
**Goal**: Solve common problems
**Structure**: Problem → Cause → Solution format

---

### Technical Specs Chapter

For understanding internals.

#### Technical-Specs.md
**Type**: Landing (A)
**Goal**: Find technical specifications

#### Technical-Specs-Architecture.md
**Type**: Conceptual (B)
**Goal**: Understand system architecture

#### Technical-Specs-Session-Lifecycle.md
**Type**: Conceptual (B)
**Goal**: Understand session flow

#### Technical-Specs-Context-Management.md
**Type**: Conceptual (B)
**Goal**: Deep dive on context handling

#### Technical-Specs-Tool-System.md
**Type**: Conceptual (B)
**Goal**: Understand tool architecture

---

### Advanced Chapter

For power users.

#### Advanced.md
**Type**: Landing (A)
**Goal**: Find advanced features

#### Advanced-Extended-Reasoning.md
**Type**: Configuration Reference (C)
**Goal**: Configure extended thinking

#### Advanced-Hidden-CLI.md
**Type**: Reference (E)
**Goal**: Undocumented CLI options

#### Advanced-Internal-API.md
**Type**: Reference (E)
**Goal**: Internal API for library embedding

---

### Contributing Chapter

For contributors.

#### Contributing.md
**Type**: Landing (A)
**Goal**: Guide potential contributors

#### Contributing-Testing.md
**Type**: How-To (D)
**Goal**: Run and write tests

#### Contributing-Code-Style.md
**Type**: Reference (E)
**Goal**: Code style requirements

#### Contributing-Documentation.md
**Type**: How-To (D)
**Goal**: Contribute to documentation

---

## Grading Rubric

### A+ (95-100%)
- Meets ALL checklist items
- Exemplary clarity and completeness
- Users can accomplish their goal without confusion
- Examples are production-quality

### A (90-94%)
- Meets most checklist items
- Minor gaps in examples or cross-linking
- Users can accomplish their goal with minimal friction

### B (80-89%)
- Missing some configuration details
- Incomplete examples
- Users need to look elsewhere for some information

### C (70-79%)
- Significant gaps in coverage
- Assumptions not stated
- Users will be confused

### D (<70%)
- Incomplete or inaccurate
- Missing critical information
- Users cannot accomplish their goal

**Target: Every page must be A+ before wiki publication.**
