# Include Directives

Reuse prompt content across multiple agents with `${include:path}`.

---

## Syntax

```
${include:path/to/file.md}
```

- Path is relative to the including `.ai` file
- File content is inserted at the directive location
- Includes are resolved before other variable substitution
- Nested includes are supported

---

## Use Cases

### 1. Shared Guidelines

Create `shared/guidelines.md`:

```markdown
## Guidelines

- Be helpful and concise
- Cite sources when possible
- Admit uncertainty rather than guessing
```

Use in agents:

```yaml
---
models:
  - openai/gpt-4o
---
You are a research assistant.

${include:shared/guidelines.md}
```

---

### 2. Tone and Voice

Create `shared/tone.md`:

```markdown
## Tone and Voice

- Professional but friendly
- Use simple language
- Avoid jargon unless necessary
- Be direct and actionable
```

Include in all customer-facing agents:

```yaml
---
models:
  - openai/gpt-4o
---
You are a customer support agent.

${include:shared/tone.md}
```

---

### 3. Domain Knowledge

Create `shared/domain/product-info.md`:

```markdown
## Product Information

Our product is XYZ, which provides:
- Feature A: Does this
- Feature B: Does that
- Pricing: $X/month

Key differentiators:
- Fast performance
- Easy integration
- 24/7 support
```

---

### 4. Safety Rules

Create `shared/safety-rules.md`:

```markdown
## Safety Rules

NEVER:
- Share API keys or credentials
- Execute destructive operations without confirmation
- Access data outside the specified scope

ALWAYS:
- Verify user permissions before sensitive operations
- Log all actions for audit
- Ask for confirmation on irreversible actions
```

---

## Real-World Pattern

From production systems (Neda CRM):

```
neda/
├── neda.ai                    # Main orchestrator
├── company.ai                 # Company research agent
├── contact.ai                 # Contact research agent
├── shared/
│   ├── tone-and-language.md   # Voice guidelines
│   ├── neda-core.md           # Core capabilities
│   └── safety-gates.md        # Safety rules
```

Every agent includes:

```yaml
---
models:
  - openai/gpt-4o
---
${include:shared/tone-and-language.md}

You are a specialized agent for...

${include:shared/safety-gates.md}
```

---

## Nested Includes

File A can include File B, which includes File C:

`guidelines.md`:
```markdown
## General Guidelines
${include:safety.md}
${include:tone.md}
```

`safety.md`:
```markdown
### Safety
- Never share credentials
```

`tone.md`:
```markdown
### Tone
- Be professional
```

Result in agent:
```markdown
## General Guidelines
### Safety
- Never share credentials
### Tone
- Be professional
```

---

## Cache Implications

- Include content is part of the agent hash
- Changes to included files invalidate agent cache
- This ensures cache consistency

---

## Best Practices

1. **Organize by purpose**: `shared/tone/`, `shared/safety/`, `shared/domain/`
2. **Keep includes focused**: One topic per file
3. **Document includes**: Add comments about what's included
4. **Version control**: Track shared files carefully

---

## Common Mistakes

### Wrong: Absolute paths
```
${include:/home/user/project/shared/file.md}  # Won't work
```

### Right: Relative paths
```
${include:shared/file.md}
${include:../common/file.md}
```

### Wrong: Missing file
```
${include:nonexistent.md}  # Error at load time
```

---

## See Also

- [Agent Files](Agent-Development-Agent-Files)
- [Prompt Variables](Agent-Development-Variables)
