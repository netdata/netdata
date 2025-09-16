## YOU MUST READ

- docs/SPECS.md for specifications.
- docs/IMPLEMENTATION.md for AI SDK/ModelContextProtocol implementation.
- docs/DESIGN.md for the design of the application.
- docs/MULTI-AGENT.md for the recursive multi-agent design.
- docs/docs/AI-AGENT-INTERNAL-API.md current status of the ai-agent internal agent API.
- README.md for end-user documentation of the application.

The application source code is in the root directory (src/, package.json, etc).

## TESTING MODELS

For your tests, we have ollama on host 'nova'.
Use it with model 'gpt-oss:20b'

## TESTING SCRIPT

Script ./run.sh provides a working example for using the application

## SECURITY AND PRIVACY

The file .env has provider API keys for many our MCP tools and llm providers.
YOU ARE NOT ALLOWED TO READ THIS FILE.
READING THIS FILE MEANS WE NEED TO INVALIDATE ALL KEYS AND CREATE NEW ONES.
THIS IS TREMENDOUS AMOUNT OF WORK. NEVER READ .env FILE.

## QUALITY REQUIREMENTS

After ever change you **MUST** ensure the application builds and that linter report zero warnings and zero errors, without exceptions.

This is code created by AI agent, which I will spend a tone of time testing and polishing.
You should not expect me to find issues by hand, when there are obvious issues identified by linters and build systems.
It will be weeks for me, probably an hour for you.
Fix them all. No questions. No pushback. No workarounds.

## CODE ORGANIZATION

1. Try to keep source code files small and separated based on concerns.
2. Try to keep functions small and separated based on concerns.

We want a simple and easy to manage application. Always keep code well organized, in small manageable entities, with logical separation.

Each function should have ONLY ONE PURPOSE, do ONLY ONE WORK.

## Linting and TypeScript Guidelines (to reduce churn)

These rules help generate code that passes `npm run lint` and `npm run build` on first try. Follow them unless the repo’s ESLint config changes.

- Prefer precise types over `any`:
  - Use `Record<string, unknown>` for generic objects.
  - Use `unknown` for untyped values and then narrow with runtime guards (see below).
  - Avoid `as any` and blanket assertions.
- Add small, reusable type guards:
  - Example: `private isPlainObject(val: unknown): val is Record<string, unknown> { return val !== null && typeof val === 'object' && !Array.isArray(val); }`
  - Use these to safely access nested properties without assertions.
- Avoid unnecessary assertions:
  - Linter flags “unnecessary type assertion” and “unnecessary condition”. Don’t add `as Type` when the type is already known.
  - Don’t use `?? 0` unless a field is truly optional at that point. Do defaulting once when parsing, then keep the type definite.
- Use dot notation over brackets when possible:
  - `obj.key` instead of `obj['key']`. Use bracket notation only for dynamic keys or non-identifier property names.
- Prefer `Record<string, unknown>` over index signatures:
  - Avoid `{ [k: string]: unknown }` where `Record<string, unknown>` suffices.
- Merging objects: implement a typed `deepMerge(target: Record<string, unknown>, source: Record<string, unknown>)` that uses `isPlainObject` and recurses. Don’t spread/merge unknown/any.
- No unused vars/imports:
  - Remove unused imports. If a param is required but unused, rename to `_param` to satisfy the linter.
- Functional rule: no loops by default:
  - Use map/reduce/filter where natural.
  - If a loop is clearly more readable/performant for streaming or iterative logic, add a targeted disable: `// eslint-disable-next-line functional/no-loop-statements` on the line above the loop, not globally.
- Import ordering:
  - Keep import order and groups consistent (Node builtins, external, local). Let `eslint-plugin-perfectionist` pass.
- Error mapping and defaults:
  - Map external errors to internal types without `any`. Carry messages as strings.
  - When a library returns partially populated fields, convert once to your internal shape with proper defaults (e.g., `TokenUsage`) and then treat as definite types throughout.
- Don’t overuse `??` and truthy checks:
  - Linter flags redundant nullish checks. Only coalesce where a value can realistically be undefined.
  - Use explicit `typeof` checks for `unknown` values.
- Minimal, targeted disable comments:
  - If you must disable a rule, do it on the narrowest possible scope (single line or block) and add a short reason.
- Provider-specific patterns:
  - Centralize cross-provider logic in the base class (e.g., splitting bundled tool results, message conversions). Keep providers thin.
  - For provider option merging, read user config once, type-guard it, and deep-merge into a typed base object (avoid casting).
- Before finishing a change:
  - Run `npm run lint` and `npm run build` locally.
  - If a lint rule keeps firing, rework the code to satisfy it rather than adding exceptions.

## Specific Context

### Build Commands
- Build: `npm run build` (TypeScript → dist)
- Lint: `npm run lint` (ESLint with --max-warnings 0)
- Full build script: `./build-and-install.sh` (builds + lints + installs to /opt/ai-agent)

### Test Commands
- No dedicated test framework configured
- For manual testing use: `npm run start`
- Test models: Use ollama on host 'nova' with model 'gpt-oss:20b'

### Code Style Guidelines

#### TypeScript
- Target: ES2022, strict mode enabled
- Use `Record<string, unknown>` for generic objects, avoid `any`
- Prefer `unknown` for untyped values, narrow with type guards
- Avoid unnecessary assertions (`as any` unallowed)

#### Naming Conventions
- kebab-case for filenames
- camelCase for variables/types
- PascalCase for classes/interfaces
- Detailed, descriptive type names preferred

#### Imports
- Use type imports where possible: `import type { Foo } from 'bar'`
- Import order: builtin → external → type → internal → parent → sibling → index → object
- Alphabetize within groups, natural sort
- Remove unused imports (linting enforced)

#### Error Handling
- Map external errors to internal types without `any`
- Use union types for status/error handling
- Prefer nullish coalescing (`??`) over truthy checks when appropriate
- Explicit `typeof` checks for `unknown` values

#### Functional Programming
- Prefer map/reduce/filter over loops
- Add targeted ESLint disable for loop statements when necessary
- `// eslint-disable-next-line functional/no-loop-statements` with comment

#### Other Rules
- No default exports
- Remove unused vars (rename to `_param` if needed)
- Prefer dot notation over brackets
- No duplicate strings (warns after threshold)
- Node protocol usage required for imports

#### Quality Standards
- Lint must pass with zero warnings/errors
- Build must succeed before commits
- Read docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md for context

# PRINCIPLES

PR-001:
ai-agent MUST load all configuration at load-time.
ai-agent MUST NOT depend on any file or external service for retreiving configuration at runtime.

PR-002:
There are no permanent tools available to all agents, except `progress_report` and `final_report`.
Per agent tools are subject to its configuration (frontmatter, command-line).


