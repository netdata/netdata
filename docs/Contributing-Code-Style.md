# Code Style

TypeScript and ESLint guidelines.

---

## Build Requirements

```bash
# Build (required before commit)
npm run build

# Lint (zero warnings required)
npm run lint
```

---

## TypeScript Guidelines

### Strict Mode

TypeScript strict mode is enabled. All code must:
- Pass type checking
- Have explicit types where needed
- Avoid `any`

### Type Preferences

| Prefer | Over |
|--------|------|
| `Record<string, unknown>` | `any` or index signatures |
| `unknown` with type guards | `any` |
| Precise types | Generic types |
| Dot notation | Bracket notation |

### Type Guards

Add small, reusable type guards:

```typescript
private isPlainObject(val: unknown): val is Record<string, unknown> {
  return val !== null && typeof val === 'object' && !Array.isArray(val);
}
```

---

## Naming Conventions

| Entity | Convention | Example |
|--------|------------|---------|
| Files | kebab-case | `session-runner.ts` |
| Variables | camelCase | `sessionConfig` |
| Types/Interfaces | PascalCase | `AIAgentResult` |
| Constants | SCREAMING_SNAKE | `MAX_RETRIES` |

---

## Import Order

1. Node builtins
2. External packages
3. Type imports
4. Internal modules
5. Parent imports
6. Sibling imports
7. Index imports

```typescript
import { readFile } from 'node:fs/promises';

import Ajv from 'ajv';

import type { Configuration } from './types.js';

import { Logger } from '../utils/logger.js';
import { parseConfig } from './config.js';
```

---

## ESLint Rules

### Key Rules

| Rule | Requirement |
|------|-------------|
| No unused vars | Remove or prefix with `_` |
| No unused imports | Remove |
| No `any` | Use precise types |
| No loops (functional) | Prefer map/reduce/filter |
| No duplicate strings | Extract constants |

### Disabling Rules

When necessary, disable narrowly:

```typescript
// eslint-disable-next-line functional/no-loop-statements
for (const item of items) {
  // Streaming logic where loop is clearer
}
```

---

## Error Handling

### Mapping External Errors

```typescript
// Good: Map to internal type
const error: AgentError = {
  code: 'PROVIDER_ERROR',
  message: providerError.message,
  details: { provider, model }
};

// Bad: Pass any
return { error: providerError as any };
```

### Defaults

Apply defaults once when parsing:

```typescript
// Good: Default during parse, then definite
const config: ResolvedConfig = {
  maxTurns: input.maxTurns ?? 10,
  maxRetries: input.maxRetries ?? 3
};

// Bad: Check everywhere
if (config.maxTurns ?? 10 > ...
```

---

## Functional Style

### Prefer Functional

```typescript
// Good
const results = items.map(process).filter(Boolean);

// Avoid unless clearer
for (const item of items) {
  results.push(process(item));
}
```

### When Loops Are OK

- Streaming/iterative processing
- Performance-critical paths
- Complex control flow

Add disable comment with reason:

```typescript
// eslint-disable-next-line functional/no-loop-statements -- streaming requires iteration
for await (const chunk of stream) {
  yield chunk;
}
```

---

## File Organization

### Keep Files Small

- One concern per file
- Split large files into modules
- Group related functionality

### Keep Functions Small

- One purpose per function
- Extract complex logic
- Name functions descriptively

---

## Comments

- Explain "why", not "what"
- Keep professional and short
- Don't add to unchanged code

```typescript
// Good: Explains why
// Skip validation for internal tools (already validated at registration)

// Bad: Explains what
// Check if tool is internal
```

---

## Before Commit Checklist

1. `npm run build` passes
2. `npm run lint` passes with zero warnings
3. No `any` types added
4. Imports ordered correctly
5. Unused code removed
6. Comments explain "why"

---

## See Also

- [Contributing](Contributing) - Overview
- [CLAUDE.md](../CLAUDE.md) - Full development instructions

