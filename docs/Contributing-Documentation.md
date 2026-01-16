# Documentation Standards

Guidelines for maintaining and contributing to AI Agent documentation.

---

## Table of Contents

- [Core Principle](#core-principle) - The golden rule for documentation
- [Document Hierarchy](#document-hierarchy) - Which documents serve which audience
- [Spec Document Structure](#spec-document-structure) - Template for technical specifications
- [Wiki Page Structure](#wiki-page-structure) - Templates for user documentation
- [Writing Guidelines](#writing-guidelines) - Voice, tone, and formatting rules
- [Maintenance Rules](#maintenance-rules) - When and how to update documentation
- [Cross-Document Syncing](#cross-document-syncing) - Keeping documents consistent
- [What NOT to Document](#what-not-to-document) - Implementation details to skip
- [See Also](#see-also) - Related documentation

---

## Core Principle

> **Every code change that affects runtime behavior, defaults, schemas, or tooling MUST update documentation at the same commit.**

This is non-negotiable. Documentation and code must stay synchronized.

---

## Document Hierarchy

### Source of Truth by Audience

| Document                                | Purpose                       | Primary Audience |
| --------------------------------------- | ----------------------------- | ---------------- |
| `docs/skills/ai-agent-configuration.md` | AI assistants creating agents | AI/LLMs          |
| `docs/specs/*.md`                       | Technical specifications      | Contributors     |
| `docs/specs/CONTRACT.md`                | User-facing guarantees        | Operators        |
| Wiki pages (`docs/*.md`)                | User documentation            | End users        |

### When to Update Each

| Change Type               | Update These                                    |
| ------------------------- | ----------------------------------------------- |
| New configuration option  | Spec file, ai-agent-configuration.md, wiki page |
| Behavior change           | Spec file, CONTRACT.md if affects guarantees    |
| New feature               | Spec file, ai-agent-configuration.md, wiki page |
| Bug fix changing behavior | Spec file, wiki if user-visible                 |

---

## Spec Document Structure

Each technical specification follows this flexible structure (not all sections required):

```markdown
# Feature Name

## TL;DR

One-line description of what this feature does.

## Source Files

- `src/file.ts:100-150` - Main implementation
- `src/other.ts:50-75` - Supporting logic

## [Feature-Specific Sections]

Specs include relevant sections based on the feature:

- Data Structures
- Algorithm/Flow descriptions
- Error classifications
- Input/output schemas
```

### Code References

Always use `file:line` format for traceability:

```markdown
**Location**: `src/ai-agent.ts:1490-1553`
```

This allows verification that documentation matches implementation.

---

## Wiki Page Structure

Wiki pages follow flexible patterns documented in `docs/USER-DOCS-STANDARD.md`:

- **Landing/Index Pages**: Navigation pages with section overviews (e.g., Home.md, Technical-Specs.md)
- **Conceptual Pages**: Detailed explanations with TL;DR, examples, and See Also sections
- **Reference Pages**: Comprehensive documentation with tables of contents and detailed sections

Each page includes:

- Clear purpose statement
- Table of Contents for navigation
- Links to related pages (See Also)

Refer to `USER-DOCS-STANDARD.md` for detailed templates and quality standards.

---

## Test Coverage

Test files follow these patterns:

- Unit tests: `src/tests/unit/<feature>.spec.ts`
- Phase 2 scenarios: `src/tests/phase2/phase2-suite.spec.ts`
- Phase 3 integration: `src/tests/phase3/phase3-suite.spec.ts`
- Other test files: `src/tests/<feature>.spec.ts`

Test reference pattern for Phase 2: scenario names use `run-test-XX` format (e.g., `run-test-11`, `run-test-21`) within the harness file.

---

## Writing Guidelines

### Voice and Tone

| Do                         | Do Not                                     |
| -------------------------- | ------------------------------------------ |
| "Configure the provider"   | "You might want to configure the provider" |
| "The agent calls the tool" | "The tool is called by the agent"          |
| "Use X for Y"              | "You could potentially use X for Y"        |
| Assume intelligence        | Talk down to users                         |

### Formatting Rules

| Element        | Use For                  | Example                         |
| -------------- | ------------------------ | ------------------------------- |
| Tables         | Configuration references | Option/Type/Default/Description |
| Code blocks    | All examples             | ` ```yaml `                     |
| Bullet points  | Lists of 3+ items        | Features, options               |
| Numbered lists | Sequential steps only    | Step 1, Step 2, Step 3          |
| Bold           | Key terms on first use   | **frontmatter**                 |
| Admonitions    | Sparingly                | `> **Note:**`, `> **Warning:**` |

### Linking Rules

| Link Type | Format        | Example                            |
| --------- | ------------- | ---------------------------------- |
| Wiki page | Relative      | `[Page Name](Page-Name)`           |
| Spec file | Relative path | `[specs/file.md](specs/file.md)`   |
| External  | Full URL      | `[GitHub](https://github.com/...)` |
| Anchor    | Hash link     | `[Section](#section-name)`         |

**Rule**: Never have a dead-end page. Always provide "See also" or "Next steps".

---

## Maintenance Rules

### When Adding Features

1. **Write spec BEFORE implementation** - Design is documented first
2. **Update ai-agent-configuration.md** - If user-facing
3. **Update wiki after merge** - User documentation

### When Changing Behavior

1. **Update spec first** - Document the change
2. **Cross-reference related specs** - Check for impacts
3. **Verify ai-agent-configuration.md accuracy** - Update if needed
4. **Check CONTRACT.md implications** - Update if affects guarantees

### When Fixing Bugs

1. **If fix changes documented behavior** - Update documentation
2. **Add test coverage reference** - Link test in spec

### Verification Requirements

Spec documents follow actual file structure, not a template pattern. Each spec includes:

- **TL;DR**: Brief feature description
- **Source Files**: Specific file and line references
- **Verification status**: Spec verification dates are tracked in `docs/specs/index.md` (e.g., "Verified 2025-11-16")

---

## Cross-Document Syncing

When updating specs that affect runtime behavior, also update:

| If You Change         | Also Update                             |
| --------------------- | --------------------------------------- |
| Configuration options | `docs/skills/ai-agent-configuration.md` |
| Default values        | `docs/specs/CONTRACT.md`                |
| User-visible behavior | Relevant wiki page                      |
| API contracts         | `docs/specs/index.md`                   |

### Traceable Evidence

Each behavior statement must cite:

- **Specific source file and line**: `src/file.ts:100`
- **Deterministic test ID**: Phase 2 scenario name

This keeps specs machine-verifiable.

---

## What NOT to Document

| Skip                        | Reason                            |
| --------------------------- | --------------------------------- |
| Internal log strings        | Implementation detail, may change |
| Internal state names        | Implementation detail             |
| Performance characteristics | May change with optimization      |
| Undocumented behaviors      | Unless making them stable         |

### Example

```markdown
# Bad: Documents implementation details

The session emits log entry "session-start-001" when beginning.

# Good: Documents observable behavior

The session logs a start message at INFO level when beginning.
```

---

## Review Process

For spec changes, use multi-agent review:

1. **Initial review** - Check accuracy against implementation
2. **Cross-check** - Verify code references are correct
3. **Reference validation** - Ensure all links work
4. **Reconciliation** - Fix any discrepancies found

---

## See Also

- [Contributing](Contributing) - Contribution overview
- [Technical-Specs](Technical-Specs) - Technical specification documents
- [USER-DOCS-STANDARD.md](https://github.com/netdata/ai-agent/blob/master/docs/USER-DOCS-STANDARD.md) - Full documentation quality standard
