# Code Style

TypeScript guidelines, naming conventions, and ESLint rules for AI Agent.

---

## Table of Contents

- [Build Requirements](#build-requirements) - Commands that must pass before commit
- [TypeScript Guidelines](#typescript-guidelines) - Strict mode, type preferences, type guards
- [Naming Conventions](#naming-conventions) - Files, variables, types, constants
- [Import Order](#import-order) - Required import organization
- [ESLint Rules](#eslint-rules) - Key rules and how to disable when necessary
- [Error Handling](#error-handling) - Mapping errors and applying defaults
- [Functional Style](#functional-style) - Prefer functional, when loops are acceptable
- [File Organization](#file-organization) - Keep files and functions small
- [Comments](#comments) - Explain why, not what
- [Before Commit Checklist](#before-commit-checklist) - Final verification steps
- [See Also](#see-also) - Related documentation

---

## Build Requirements

These commands must pass before every commit:

```bash
# Build (required)
npm run build

# Lint (zero warnings required)
npm run lint
```

**Build output**: TypeScript compiles to `dist/` with no errors.

**Lint output**: Zero warnings, zero errors. Any warning fails the check.

---

## TypeScript Guidelines

### Strict Mode

TypeScript strict mode is enabled (target: ES2023). All code must:

- Pass type checking without errors
- Have explicit types where inference is insufficient
- Avoid `any` entirely

### Type Preferences

| Prefer                     | Over                                | Why                               |
| -------------------------- | ----------------------------------- | --------------------------------- |
| `Record<string, unknown>`  | `any` or `{ [k: string]: unknown }` | Explicit generic object type      |
| `unknown` with type guards | `any`                               | Forces safe narrowing             |
| Precise types              | Generic types                       | Better documentation and checking |
| Dot notation               | Bracket notation                    | Cleaner, lint-enforced            |

### Type Guards

Add small, reusable type guards for narrowing `unknown` values:

```typescript
private isPlainObject(val: unknown): val is Record<string, unknown> {
  return val !== null && typeof val === 'object' && !Array.isArray(val);
}

private isStringArray(val: unknown): val is string[] {
  return Array.isArray(val) && val.every(item => typeof item === 'string');
}
```

**Usage**:

```typescript
function processConfig(input: unknown): Config {
  if (!isPlainObject(input)) {
    throw new Error("Config must be an object");
  }
  // Now input is typed as Record<string, unknown>
  const maxTurns = typeof input.maxTurns === "number" ? input.maxTurns : 10;
  return { maxTurns };
}
```

### Avoid Unnecessary Assertions

```typescript
// Bad: Unnecessary assertion
const value = result as string; // ESLint: unnecessary type assertion

// Good: Let TypeScript infer
const value = result; // If result is already string
```

---

## Naming Conventions

| Entity           | Convention                       | Example             |
| ---------------- | -------------------------------- | ------------------- |
| Files            | kebab-case                       | `session-runner.ts` |
| Variables        | camelCase                        | `sessionConfig`     |
| Functions        | camelCase                        | `processToolCall`   |
| Types/Interfaces | PascalCase                       | `AIAgentResult`     |
| Classes          | PascalCase                       | `SessionRunner`     |
| Constants        | SCREAMING_SNAKE_CASE             | `MAX_RETRIES`       |
| Private members  | camelCase (no underscore prefix) | `private config`    |

### Examples

```typescript
// File: tool-executor.ts

const MAX_TOOL_TIMEOUT = 30000;

interface ToolExecutorConfig {
  timeout: number;
  retries: number;
}

class ToolExecutor {
  private config: ToolExecutorConfig;

  executeToolCall(toolName: string): ToolResult {
    // ...
  }
}
```

---

## Import Order

Imports must follow this order, with blank lines between groups:

1. Node builtins (with `node:` protocol)
2. External packages
3. Type and value imports (grouped together by module)

### Example

```typescript
import fs from "node:fs";
import path from "node:path";

import Ajv from "ajv";
import { z } from "zod";

import type { Configuration, ToolDefinition } from "./types.js";
import { formatError } from "../utils/errors.js";
import { Logger } from "../utils/logger.js";

import { parseConfig } from "./config.js";
import { validateSchema } from "./validation.js";
```

The linter enforces this order automatically.

---

## ESLint Rules

### Key Rules

| Rule                  | Requirement                    | Fix                             |
| --------------------- | ------------------------------ | ------------------------------- |
| No unused vars        | Remove unused variables        | Delete or prefix with `_`       |
| No unused imports     | Remove unused imports          | Delete the import               |
| No `any`              | Use precise types              | Use `unknown` with guards       |
| No loops (functional) | Prefer map/reduce/filter       | Refactor or add disable comment |
| No duplicate strings  | Extract to constants           | Create named constant           |
| Dot notation          | Use `obj.key` not `obj['key']` | Use dot notation                |

### Disabling Rules

When a rule must be disabled, do it narrowly with a reason:

```typescript
// Single line disable with reason
// eslint-disable-next-line functional/no-loop-statements -- streaming requires iteration
for await (const chunk of stream) {
  yield chunk;
}
```

**Never disable globally**. Target the specific line or block.

### Unused Parameters

If a parameter is required by an interface but unused, prefix with underscore:

```typescript
// Interface requires both parameters
function handleEvent(event: Event, _context: Context): void {
  // Only using event, context required by interface
  console.log(event.type);
}
```

---

## Error Handling

### Mapping External Errors

Map external errors to internal types without using `any`:

```typescript
// Good: Map error with explicit message handling
try {
  await provider.complete(messages);
} catch (err) {
  const message = err instanceof Error ? err.message : String(err);
  const error = new Error(`Provider failed: ${message}`);
  error.name = 'ProviderError';
  throw error;
}

// Bad: Pass error as any
catch (err) {
  return { error: err as any };  // Never do this
}
```

### Applying Defaults

Apply defaults once during parsing, then treat as definite:

```typescript
// Good: Default during parse, then definite
interface ResolvedConfig {
  maxTurns: number;    // Not optional
  maxRetries: number;  // Not optional
}

function resolveConfig(input: Partial<Config>): ResolvedConfig {
  return {
    maxTurns: input.maxTurns ?? 10,
    maxRetries: input.maxRetries ?? 3
  };
}

// Then use without checking
const config = resolveConfig(userInput);
if (turn > config.maxTurns) { ... }  // No ?? needed

// Bad: Check everywhere
if ((config.maxTurns ?? 10) > ...) { ... }  // Repetitive, error-prone
```

---

## Functional Style

### Prefer Functional Methods

Use `map`, `filter`, `reduce` instead of loops when natural:

```typescript
// Good: Functional
const results = items
  .map((item) => processItem(item))
  .filter((result) => result.valid);

const total = values.reduce((sum, val) => sum + val, 0);

// Avoid: Imperative
const results = [];
for (const item of items) {
  const result = processItem(item);
  if (result.valid) {
    results.push(result);
  }
}
```

### When Loops Are Acceptable

Loops are acceptable for:

- Streaming/iterative processing
- Performance-critical paths
- Complex control flow (early exit, multiple conditions)

Add a disable comment with reason:

```typescript
// eslint-disable-next-line functional/no-loop-statements -- streaming requires iteration
for await (const chunk of stream) {
  yield chunk;
}

// eslint-disable-next-line functional/no-loop-statements -- early exit on match
for (const provider of providers) {
  const result = await provider.tryConnect();
  if (result.success) {
    return result;
  }
}
```

---

## File Organization

### Keep Files Small

- **One concern per file**: A file should have one primary purpose
- Split large files by extracting modules as they grow

### Keep Functions Small

- **One purpose per function**: Each function does one thing
- **Extract complex logic**: Named helper functions improve readability

### Example Structure

```
src/
  session/
    session-runner.ts      # Main session loop
    session-config.ts      # Configuration handling
    session-state.ts       # State management
  tools/
    tool-executor.ts       # Tool execution
    tool-validator.ts      # Tool validation
    tool-types.ts          # Type definitions
```

---

## Comments

### Explain Why, Not What

```typescript
// Good: Explains why
// Skip validation for internal tools (already validated at registration)
if (tool.internal) {
  return tool;
}

// Bad: Explains what (obvious from code)
// Check if tool is internal
if (tool.internal) {
  return tool;
}
```

### Comment Guidelines

| Do                              | Do Not                         |
| ------------------------------- | ------------------------------ |
| Explain non-obvious decisions   | State what the code does       |
| Document edge cases             | Add comments to unchanged code |
| Note performance considerations | Use comments as code deodorant |
| Reference related issues/specs  | Write novels                   |

### Examples

```typescript
// Good: Context for future readers
// Retry with exponential backoff; max 3 attempts per provider rate limits
await retry(operation, { maxAttempts: 3, backoff: "exponential" });

// Good: Warning about non-obvious behavior
// Order matters: tools must be registered before agents to resolve references
registerTools(config);
registerAgents(config);
```

---

## Before Commit Checklist

| Step | Check                          | Command             |
| ---- | ------------------------------ | ------------------- |
| 1    | Build passes                   | `npm run build`     |
| 2    | Lint passes with zero warnings | `npm run lint`      |
| 3    | No `any` types added           | Manual review       |
| 4    | Imports ordered correctly      | Linter catches this |
| 5    | Unused code removed            | Linter catches this |
| 6    | Comments explain "why"         | Manual review       |

---

## See Also

- [Contributing](Contributing) - Contribution overview and setup
- [Testing Guide](Contributing-Testing) - Test phases and coverage
- [Documentation Standards](Contributing-Documentation) - Documentation guidelines
- [CLAUDE.md](https://github.com/netdata/ai-agent/blob/master/CLAUDE.md) - Full development instructions
