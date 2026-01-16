# Documentation Standards

Guidelines for maintaining documentation.

---

## Core Principle

> Every code change that affects runtime behavior, defaults, schemas, or tooling MUST update DOCUMENTATION at the same commit.

---

## Document Hierarchy

### Source of Truth

| Document | Purpose | Audience |
|----------|---------|----------|
| `docs/skills/ai-agent-configuration.md` | AI assistants creating agents | AI/LLMs |
| `docs/specs/*.md` | Technical specifications | Contributors |
| `docs/specs/CONTRACT.md` | User-facing guarantees | Operators |
| Wiki pages | User documentation | End users |

### Keep Synchronized

When changing behavior:
1. Update the relevant spec file
2. Update skills/ai-agent-configuration.md if it affects agent creation
3. Update specs/CONTRACT.md if it affects guarantees
4. Update wiki pages for user-facing changes

---

## Spec Document Structure

Each spec follows this template:

```markdown
# Feature Name

## TL;DR
One-line description.

## Source Files
- `src/file.ts:line` - Description

## Behaviors
Exhaustive behavior list with conditions.

## Configuration
Settings that affect behavior.

## Telemetry
Metrics/traces emitted.

## Logging
Log entries produced.

## Events
Events emitted/consumed.

## Test Coverage
Existing tests and gaps.

## Invariants
Conditions that must always hold.
```

---

## Code References

Use file:line format for traceability:

```markdown
**Location**: `src/ai-agent.ts:1490-1553`
```

---

## Business Logic Coverage

Each spec should include:

```markdown
## Business Logic Coverage (Verified YYYY-MM-DD)

- **Feature X**: Description with code reference (`src/file.ts:100-200`)
- **Feature Y**: Description with code reference
```

---

## Wiki Page Structure

### Landing Pages

```markdown
# Section Name

Brief description.

---

## Overview

What this section covers.

---

## Documents

| Document | Description |
|----------|-------------|
| [Page](Link) | Brief description |

---

## See Also

- Related links
```

### Content Pages

```markdown
# Feature Name

Brief description.

---

## TL;DR

One-sentence summary.

---

## Configuration

How to configure.

---

## Usage

How to use.

---

## See Also

- Related links
```

---

## Maintenance Rules

### When Adding Features

1. Write spec BEFORE implementation
2. Update AI-AGENT-GUIDE.md if user-facing
3. Update wiki after merge

### When Changing Behavior

1. Update spec first
2. Cross-reference related specs
3. Verify AI-AGENT-GUIDE.md accuracy
4. Check CONTRACT.md implications

### When Fixing Bugs

1. If fix changes documented behavior → update docs
2. Add test coverage reference to spec

---

## Cross-Document Syncing

Specs referencing runtime behavior must also update:
- `docs/skills/ai-agent-configuration.md`
- `docs/specs/index.md`

This ensures AI assistants and developers see consistent information.

---

## Traceable Evidence

Each behavior statement must cite:
- Specific source file/line
- Deterministic test ID

This keeps specs machine-verifiable.

---

## Review Process

Multi-agent review for spec changes:
1. Initial review for accuracy
2. Cross-check against implementation
3. Verify all references valid
4. Reconcile any discrepancies

---

## What NOT to Document

- Internal log strings (implementation detail)
- Internal state names (implementation detail)
- Performance characteristics (may change)
- Undocumented behaviors (unless making them stable)

---

## See Also

- [Technical-Specs](Technical-Specs) - Spec documents
- [Contributing](Contributing) - Contribution overview

