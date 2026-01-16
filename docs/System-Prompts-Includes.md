# Include Directives

Reuse prompt content across multiple agents with `${include:path}`.

---

## Table of Contents

- [Overview](#overview) - What includes do and why they matter
- [Syntax](#syntax) - How to write include directives
- [Path Resolution](#path-resolution) - How paths are resolved
- [Nesting](#nesting) - Includes within includes
- [Common Patterns](#common-patterns) - Typical use cases and examples
- [Best Practices](#best-practices) - Organization and maintenance
- [Troubleshooting](#troubleshooting) - Common errors and fixes
- [See Also](#see-also) - Related documentation

---

## Overview

Include directives let you share prompt content across multiple agents. This is useful for:

- **Consistency**: Same guidelines, tone, safety rules across all agents
- **Maintenance**: Update once, apply everywhere
- **Organization**: Keep prompts focused; extract reusable content
- **Composition**: Build agents from modular pieces

**Key facts**:
- Includes are resolved at load time, before variable substitution
- Nested includes are supported (includes can include other files)
- Maximum nesting depth is 8 levels (prevents infinite recursion)
- `.env` files cannot be included (security protection)

---

## Syntax

Both forms are supported:

```markdown
${include:path/to/file.md}
{{include:path/to/file.md}}
```

The content of the referenced file replaces the directive entirely.

**Example**:

`shared/tone.md`:
```markdown
## Tone and Voice
- Be professional but friendly
- Use simple, clear language
- Avoid jargon
```

`my-agent.ai`:
```yaml
---
models:
  - openai/gpt-4o
---
You are a helpful assistant.

${include:shared/tone.md}

Answer the user's questions clearly.
```

**Result** (after include resolution):
```markdown
You are a helpful assistant.

## Tone and Voice
- Be professional but friendly
- Use simple, clear language
- Avoid jargon

Answer the user's questions clearly.
```

---

## Path Resolution

Paths are resolved relative to the file containing the include directive.

### Relative Paths (Recommended)

```markdown
${include:shared/file.md}          # Same directory, then shared/
${include:../common/file.md}       # Parent directory, then common/
${include:./helpers/file.md}       # Explicit current directory
```

### Project Structure Example

```
my-project/
├── agents/
│   ├── main.ai              # Contains ${include:../shared/tone.md}
│   └── helper.ai            # Contains ${include:../shared/tone.md}
├── shared/
│   ├── tone.md
│   ├── safety.md
│   └── domain/
│       └── product-info.md
```

From `agents/main.ai`:
```markdown
${include:../shared/tone.md}              # Goes up one level, into shared/
${include:../shared/domain/product-info.md}
```

### Absolute Paths (Not Supported)

Absolute paths do not work:
```markdown
${include:/home/user/project/shared/file.md}  # Will fail
```

Use relative paths instead.

---

## Nesting

Includes can contain other includes. Each file's includes are resolved relative to that file's location.

### Example

`shared/all-guidelines.md`:
```markdown
## Guidelines
${include:safety.md}
${include:tone.md}
```

`shared/safety.md`:
```markdown
### Safety Rules
- Never share credentials
- Verify before destructive actions
```

`shared/tone.md`:
```markdown
### Tone
- Be professional
- Be concise
```

`agents/main.ai`:
```yaml
---
models:
  - openai/gpt-4o
---
You are a helpful assistant.

${include:../shared/all-guidelines.md}
```

**Final result**:
```markdown
You are a helpful assistant.

## Guidelines
### Safety Rules
- Never share credentials
- Verify before destructive actions
### Tone
- Be professional
- Be concise
```

### Depth Limit

Maximum nesting depth is 8 levels. If exceeded, you'll get an error:

```
Maximum include depth (8) exceeded - possible circular reference or deeply nested includes
```

---

## Common Patterns

### Pattern 1: Shared Guidelines

Create consistent behavior across all agents.

`shared/guidelines.md`:
```markdown
## Guidelines
- Be helpful and concise
- Cite sources for factual claims
- Acknowledge uncertainty rather than guessing
- Respect user privacy
```

`any-agent.ai`:
```yaml
---
models:
  - openai/gpt-4o
---
You are a research assistant.

${include:shared/guidelines.md}

Help users find accurate information.
```

### Pattern 2: Tone and Voice

Maintain consistent personality.

`shared/tone.md`:
```markdown
## Tone and Voice
- Professional but friendly
- Use simple language (no jargon)
- Be direct and actionable
- Show empathy when appropriate
```

### Pattern 3: Safety Rules

Enforce security boundaries.

`shared/safety-gates.md`:
```markdown
## Safety Rules

**NEVER:**
- Share API keys, passwords, or credentials
- Execute destructive operations without confirmation
- Access files outside the specified scope
- Make changes to production systems without approval

**ALWAYS:**
- Verify user permissions before sensitive operations
- Ask for confirmation on irreversible actions
- Log all significant actions
- Redact sensitive information in outputs
```

### Pattern 4: Domain Knowledge

Share product/company information.

`shared/domain/product-info.md`:
```markdown
## Product Information

**Product Name**: Acme Analytics
**Version**: 3.2.1
**Pricing**: $49/month (Starter), $199/month (Pro), $499/month (Enterprise)

**Key Features**:
- Real-time dashboards
- Custom alerts
- API access (Pro and above)
- SSO integration (Enterprise only)

**Support Channels**:
- Email: support@acme.com
- Chat: Available 9am-6pm EST
- Phone: Enterprise only
```

### Pattern 5: Output Templates

Standardize response structure.

`shared/output-template.md`:
```markdown
## Response Structure

Always format your response as:

1. **TL;DR** (2-3 sentences): Bottom-line summary
2. **Details**: Full explanation with supporting evidence
3. **Sources**: List of references (if applicable)
4. **Confidence**: Rate 0-100% with brief justification
5. **Next Steps**: Suggested actions (if applicable)
```

### Real-World Example

From a production CRM system:

```
neda/
├── neda.ai                    # Main orchestrator
├── company.ai                 # Company research agent
├── contact.ai                 # Contact research agent
├── support.ai                 # Support agent
├── tone-and-language.md       # Shared voice
├── neda-core.md              # Core capabilities
└── safety-gates.md           # Security rules
```

Each agent includes shared content:

```yaml
---
description: Company research agent
models:
  - openai/gpt-4o
---
${include:tone-and-language.md}

You are a company research specialist.

${include:neda-core.md}

${include:safety-gates.md}
```

---

## Best Practices

### 1. Organize by Purpose

```
shared/
├── tone/
│   ├── professional.md
│   ├── casual.md
│   └── technical.md
├── safety/
│   ├── basic.md
│   ├── strict.md
│   └── medical.md
├── domain/
│   ├── product-info.md
│   ├── pricing.md
│   └── competitors.md
└── output/
    ├── standard.md
    └── detailed.md
```

### 2. Keep Includes Focused

One topic per file. Easier to mix and match.

**Good**:
```
safety.md      - Safety rules only
tone.md        - Tone guidelines only
output.md      - Output formatting only
```

**Avoid**:
```
everything.md  - All guidelines in one file
```

### 3. Document Your Includes

Add comments explaining what's included:

```yaml
---
models:
  - openai/gpt-4o
---
You are a customer support agent.

# Standard tone for customer-facing agents
${include:shared/tone/professional.md}

# Product knowledge
${include:shared/domain/product-info.md}

# Safety rules for handling customer data
${include:shared/safety/customer-data.md}
```

### 4. Version Control Shared Files

Shared includes affect multiple agents. Track changes carefully:
- Review changes to shared files before deploying
- Consider the impact on all agents using the include
- Test representative agents after modifying shared content

### 5. Avoid Deep Nesting

Keep nesting shallow (2-3 levels max). Deep nesting makes debugging harder.

**Prefer**:
```markdown
${include:safety.md}
${include:tone.md}
${include:output.md}
```

**Over**:
```markdown
${include:all-config.md}  # Which includes safety.md, which includes...
```

---

## Troubleshooting

### "failed to include 'path': ENOENT"

**Problem**: File not found.

**Cause**: Path is wrong or file doesn't exist.

**Fix**:
1. Check the path is relative to the including file
2. Verify the file exists
3. Check for typos in the filename

```markdown
# If you're in agents/main.ai and shared/ is at the project root:
${include:../shared/tone.md}    # Correct - go up one level
${include:shared/tone.md}       # Wrong - looks in agents/shared/
```

### "including this file is forbidden"

**Problem**: Trying to include a protected file.

**Cause**: Attempting to include `.env` or other sensitive files.

**Fix**: Don't include `.env` files. If you need configuration values, use environment variables in your `.ai-agent.json` config instead.

### "Maximum include depth exceeded"

**Problem**: Include nesting is too deep (> 8 levels).

**Cause**: Circular reference or excessively nested includes.

**Fix**:
1. Check for circular includes (A includes B, B includes A)
2. Flatten the structure (include files directly instead of through intermediaries)
3. Combine small includes into larger files

### Include content not appearing

**Problem**: Include directive shows up literally in output.

**Cause**: Syntax error in the directive.

**Fix**: Check syntax:
```markdown
${include:file.md}     # Correct
${ include:file.md}    # Wrong - space after {
${include: file.md}    # Wrong - space after :
${include:file.md }    # Wrong - space before }
{{include:file.md}}    # Also correct (alternative syntax)
```

### Wrong content included

**Problem**: Getting content from unexpected file.

**Cause**: Path resolution confusion.

**Fix**: Remember paths are relative to the file containing the include, not your working directory.

```
project/
├── agents/
│   └── main.ai         # ${include:../shared/tone.md}
└── shared/
    └── tone.md
```

From `agents/main.ai`, you must go up one level (`..`) to reach `shared/`.

---

## See Also

- [System-Prompts](System-Prompts) - Chapter overview
- [System-Prompts-Writing](System-Prompts-Writing) - Best practices for prompts
- [System-Prompts-Variables](System-Prompts-Variables) - Variable reference
- [Agent-Development-Agent-Files](Agent-Development-Agent-Files) - `.ai` file structure
