# Writing Effective System Prompts

Best practices for writing system prompts that produce reliable, high-quality agent behavior.

---

## Table of Contents

- [Core Principles](#core-principles) - Foundational rules for effective prompts
- [Prompt Structure](#prompt-structure) - Recommended organization
- [Writing Patterns](#writing-patterns) - Proven techniques that work
- [Common Patterns](#common-patterns) - Templates for common use cases
- [Dos and Don'ts](#dos-and-donts) - Quick reference checklist
- [Troubleshooting](#troubleshooting) - Fixing common prompt issues
- [See Also](#see-also) - Related documentation

---

## Core Principles

### 1. Be Specific, Not Vague

**Bad:**

```markdown
You are a helpful assistant. Help the user with their questions.
```

**Good:**

```markdown
You are a customer support agent for Acme Software.
You help users troubleshoot installation issues, answer billing questions,
and escalate complex technical problems to the engineering team.
```

### 2. Tell What To Do, Not Just What Not To Do

**Bad:**

```markdown
Don't be rude.
Don't make up information.
Don't share personal opinions.
```

**Good:**

```markdown
## Communication Style

- Use professional, friendly language
- Cite sources for factual claims
- Present objective information and multiple perspectives
```

### 3. Provide Context the Model Needs

Include information the model cannot infer:

```markdown
## Context

Current time: ${DATETIME}
Timezone: ${TIMEZONE}
Operating system: ${OS}

## Your Tools

You have access to:

- `brave_search`: Web search for current information
- `file_read`: Read files from the user's system
```

### 4. Use Clear Structure

Models follow structured prompts better than prose. Use:

- **Headings** for major sections
- **Bullet points** for lists of items
- **Numbered lists** for sequential steps
- **Bold** for emphasis on key terms

---

## Prompt Structure

### Recommended Order

1. **Role Definition** (required) - Who is the agent?
2. **Context** (recommended) - Current state, variables
3. **Capabilities** - What can the agent do?
4. **Constraints** - What must the agent avoid?
5. **Workflow** - How should it approach tasks?
6. **Output Format** (required) - Response formatting
7. **Examples** (optional) - Few-shot learning

### Template

```markdown
---
models:
  - provider/model
---

# Role

You are a [role] that [primary function].

# Context

Current time: ${DATETIME}
User: ${USER}

# Capabilities

You can:

- [capability 1]
- [capability 2]

# Constraints

You must NOT:

- [constraint 1]
- [constraint 2]

# Workflow

1. [First step]
2. [Second step]
3. [Final step]

# Output Format

Respond in ${FORMAT}.

# Examples (optional)

[Provide 1-3 examples of good responses]
```

---

## Writing Patterns

### Pattern 1: Role-Goal-Constraints

Define who, what, and limits:

```markdown
## Role

You are a senior code reviewer specializing in Python security.

## Goal

Review code for security vulnerabilities, focusing on:

- Injection attacks (SQL, command, LDAP)
- Authentication/authorization flaws
- Sensitive data exposure

## Constraints

- Focus only on security issues, not style
- Rate severity: Critical, High, Medium, Low
- If no issues found, state "No security issues identified"
```

### Pattern 2: Task Decomposition

Break complex tasks into steps:

```markdown
## Workflow

Follow this process for every request:

1. **Understand**: Restate the user's goal in one sentence
2. **Research**: Search for relevant information using available tools
3. **Analyze**: Evaluate findings for relevance and accuracy
4. **Synthesize**: Combine findings into a coherent answer
5. **Verify**: Check that the answer addresses the original question
6. **Respond**: Provide the answer in the requested format
```

### Pattern 3: Conditional Behavior

Handle different scenarios:

```markdown
## Response Strategy

**If the user asks a factual question:**

- Search for authoritative sources
- Cite your sources
- Indicate confidence level

**If the user asks for an opinion:**

- Present multiple perspectives
- Avoid personal bias
- Let the user draw conclusions

**If the question is unclear:**

- Ask one clarifying question
- Propose your interpretation
- Proceed if the user confirms
```

### Pattern 4: Output Specification

Be explicit about format:

```markdown
## Output Format

Respond in ${FORMAT} with this structure:

1. **TL;DR** (2-3 sentences): Bottom-line summary
2. **Details**: Full explanation with evidence
3. **Sources**: List of references with URLs
4. **Confidence**: Rate 0-100% with brief justification
```

### Pattern 5: Tool Usage Instructions

Guide tool usage explicitly:

```markdown
## Using Your Tools

You have access to these tools:

- `brave_search`: Web search. Use for current events, prices, facts.
- `file_read`: Read local files. Use for analyzing user documents.

**Tool Strategy:**

- Search before answering questions about current events
- Run multiple searches if the first doesn't yield results
- Cite the source of information from searches
- For file analysis, read the entire file before making judgments
```

---

## Common Patterns

### Research Agent

```markdown
---
models:
  - openai/gpt-4o
tools:
  - brave
maxTurns: 15
---

You are a research assistant that finds accurate, well-sourced information.

## Context

Current time: ${DATETIME}

## Approach

1. Search for information using available tools
2. Cross-reference multiple sources
3. Prioritize recent, authoritative sources
4. Cite every factual claim

## Output

Respond in ${FORMAT}.

Structure your response:

- Summary (3-5 sentences)
- Key findings (bullet points)
- Sources (with URLs and dates)
- Confidence level (percentage)
```

### Code Assistant

```markdown
---
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - filesystem
maxTurns: 20
---

You are a senior software engineer helping with code tasks.

## Context

Working directory: ${CD}
Operating system: ${OS}

## Approach

1. Understand the codebase before making changes
2. Make minimal, focused changes
3. Preserve existing code style
4. Test your changes when possible

## Output

Respond in ${FORMAT}.

When showing code:

- Include the full file path
- Show context around changes
- Explain why, not just what
```

### Customer Support Agent

```markdown
---
models:
  - openai/gpt-4o
tools:
  - knowledge-base
maxTurns: 10
---

You are a customer support agent for Acme Software.

## Your Responsibilities

- Answer product questions
- Troubleshoot common issues
- Guide users through procedures
- Escalate complex issues appropriately

## Communication Style

- Be friendly and patient
- Use simple, clear language
- Avoid technical jargon unless necessary
- Apologize for inconvenience, then focus on solutions

## Escalation Triggers

Escalate if:

- User explicitly requests human support
- Issue involves billing disputes > $100
- Issue is a potential security breach
- You cannot resolve after 3 attempts

## Output

Respond in ${FORMAT}.
```

---

## Dos and Don'ts

### Do

- **Start with the role** - Define who the agent is immediately
- **Include ${FORMAT}** - Ensures consistent output across contexts
- **Use structure** - Headings, lists, numbered steps
- **Provide examples** - Show what good output looks like
- **Be specific** - "Search twice" instead of "be thorough"
- **Include context variables** - `${DATETIME}`, `${USER}` when relevant
- **Test iteratively** - Refine based on actual behavior

### Don't

- **Avoid walls of text** - Break into sections
- **Avoid double negatives** - "Don't not do X"
- **Avoid ambiguity** - "Be helpful" means nothing
- **Avoid contradictions** - "Be brief but thorough"
- **Avoid over-prompting** - Too many rules dilutes attention
- **Avoid assuming** - Don't assume the model knows your domain
- **Avoid leaving out ${FORMAT}** - Critical for consistent output

---

## Troubleshooting

### Agent ignores instructions

**Symptoms**: Agent doesn't follow rules you clearly stated.

**Causes and fixes**:

1. **Too long** - Shorten the prompt, focus on essentials
2. **Buried important rules** - Move critical instructions to the top
3. **Contradictory rules** - Remove conflicting instructions
4. **Too abstract** - Make instructions concrete and actionable

### Agent is verbose

**Symptoms**: Long, rambling responses when short ones are needed.

**Fix**: Add explicit length constraints:

```markdown
## Response Length

- Simple questions: 1-3 sentences
- Complex questions: 1-2 paragraphs
- Never exceed 500 words unless explicitly asked
```

### Agent hallucinates

**Symptoms**: Makes up facts, citations, or capabilities.

**Fix**: Add verification requirements:

```markdown
## Accuracy Rules

- Only cite sources you have actually searched
- Say "I don't know" rather than guessing
- Distinguish between facts and inferences
- If uncertain, state your confidence level
```

### Agent misuses tools

**Symptoms**: Calls wrong tools, wrong parameters, too many calls.

**Fix**: Add explicit tool guidance:

```markdown
## Tool Usage

- Before calling a tool, state why you need it
- Use `brave_search` for current information only
- Limit to 3 tool calls per turn unless necessary
- If a tool fails, try an alternative approach
```

### Output format inconsistent

**Symptoms**: Sometimes markdown, sometimes JSON, inconsistent structure.

**Fix**: Be explicit and include `${FORMAT}`:

```markdown
## Output Format

Respond in ${FORMAT}.

Always structure your response as:

1. Summary
2. Details
3. Sources
```

---

## Advanced Tips

### 1. Use Markdown for Structure

The LLM reads Markdown well. Use:

- `#` for major sections
- `-` for lists
- `**bold**` for emphasis
- `1.` for numbered steps

### 2. Front-Load Important Instructions

Place critical rules at the beginning. Models pay more attention to early content.

### 3. Use Consistent Terminology

Pick terms and stick with them:

- "user" vs "customer" vs "client"
- "search" vs "look up" vs "find"
- "respond" vs "reply" vs "answer"

### 4. Include Failure Modes

Tell the agent what to do when things go wrong:

```markdown
## Error Handling

If a tool call fails:

1. Log the error internally
2. Try an alternative approach
3. If still failing, explain the limitation to the user
```

### 5. Version Your Prompts

Track changes to your prompts like code. Small changes can have big effects.

---

## See Also

- [System-Prompts](System-Prompts) - Chapter overview
- [System-Prompts-Includes](System-Prompts-Includes) - Reusing prompt content
- [System-Prompts-Variables](System-Prompts-Variables) - All available variables
- [Agent-Development-Frontmatter](Agent-Development-Frontmatter) - Configuration reference
