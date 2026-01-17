# TODO: Split Large Source Files

## TL;DR

6 production files exceed ~10k tokens. This document outlines a safe, test-first approach to split them while maintaining **zero functional changes**. The strategy: measure baseline → add tests → extract types and pure helpers only → verify behavior unchanged.

**Key Constraint:** Only extract code that ALREADY EXISTS as isolatable units. No refactoring logic during splitting.

---

## Guiding Principles

1. **Zero Functional Changes** - Code behavior must be identical before and after splitting
2. **No Refactoring** - Only MOVE existing code; do not restructure logic
3. **Test-First** - Add comprehensive tests BEFORE any extraction
4. **Types First** - Always extract interfaces/types before functions
5. **Pure Helpers Only** - Only extract functions that don't depend on class `this` context
6. **Align with Existing Structure** - Use existing `src/cache/`, `src/orchestration/` folders
7. **Strangler Pattern** - Incremental extraction with explicit dependency injection

---

## Production Files to Split (Priority Order)

| Priority | File | Lines | Est. Tokens | Target |
|----------|------|-------|-------------|--------|
| 1 | `src/tools/mcp-provider.ts` | 1,417 | ~14k | <10k (easiest, class already separable) |
| 2 | `src/cli.ts` | 2,225 | ~24k | <10k (well-contained functions) |
| 3 | `src/ai-agent.ts` | 2,428 | ~26k | <10k (align with existing modules) |
| 4 | `src/llm-providers/base.ts` | 2,570 | ~28k | <10k (extract pure helpers only) |
| 5 | `src/session-turn-runner.ts` | 3,763 | ~54k | <10k (hardest - tightly coupled) |
| 6 | `src/llm-client.ts` | 1,140 | ~11k | <10k (if needed) |

**Note:** Test files (`src/tests/phase2-harness-scenarios/phase2-runner.ts`) are handled separately in Phase 5 - they are lower priority than production code stability.

---

## Phase 0: Baseline Metrics & Test Coverage Assessment

**Status:** NOT STARTED

Before ANY splitting, establish measurable baselines.

### 0.1 Capture Baseline Metrics

Run and record:
```bash
# Line counts for each file
wc -l src/session-turn-runner.ts src/llm-providers/base.ts src/ai-agent.ts src/cli.ts src/tools/mcp-provider.ts

# Function/class counts
grep -c "^export function\|^export class\|^export interface" src/*.ts src/**/*.ts

# Build time
time npm run build

# Test execution time
time npm run test
```

### 0.2 Verify Existing Test Coverage

Check what unit tests exist for modules being split:

| File | Unit Test | Phase 2 Coverage | Status |
|------|-----------|------------------|--------|
| `src/session-turn-runner.ts` | None | Yes (via harness) | NEEDS UNIT TESTS |
| `src/llm-providers/base.ts` | None | Partial | NEEDS UNIT TESTS |
| `src/ai-agent.ts` | None | Yes (via harness) | NEEDS UNIT TESTS |
| `src/cli.ts` | None | No | NEEDS UNIT TESTS |
| `src/tools/mcp-provider.ts` | None | Partial | NEEDS UNIT TESTS |

### 0.3 Enable Vitest Coverage

In `vitest.config.ts`, enable coverage:
```typescript
coverage: {
  enabled: true,  // Currently false
  // ...
}
```

Run coverage report to identify untested paths.

---

## Phase 1: Add Unit Tests for Production Code

**Status:** NOT STARTED

Before splitting any production file, add unit tests for the specific functionality being extracted.

### 1.1 Tests for `src/tools/mcp-provider.ts`

Create `src/tests/unit/mcp-provider.spec.ts`:

- [ ] `MCPSharedRegistry` - server registration, acquisition, release
- [ ] `MCPProvider` - tool discovery, execution, error handling
- [ ] `filterToolsForServer()` - tool filtering logic
- [ ] `createStdioTransport()` - transport creation

### 1.2 Tests for `src/cli.ts`

Create `src/tests/unit/cli.spec.ts`:

- [ ] `parsePort()` - port parsing validation
- [ ] `parsePositive()` - positive number validation
- [ ] `parseDurationOption()` - duration string parsing
- [ ] `buildGlobalOverrides()` - override construction
- [ ] `createCallbacks()` - callback factory
- [ ] `listMcpTools()` - MCP tool listing

### 1.3 Tests for `src/ai-agent.ts`

Create `src/tests/unit/ai-agent-session.spec.ts`:

- [ ] `ToolSelection` interface behavior
- [ ] `isAgentCachePayload()` - type guard
- [ ] `filterToolsForProvider()` - tool filtering
- [ ] Tool budget calculation
- [ ] Event emission patterns

### 1.4 Tests for `src/llm-providers/base.ts`

Create `src/tests/unit/llm-provider-base.spec.ts`:

- [ ] `resolveReasoningValue()` - reasoning configuration
- [ ] `shouldAutoEnableReasoningStream()` - stream decision
- [ ] `normalizeFormatPolicy()` - format policy handling
- [ ] `clampTokenBudget()` - if exists as pure function
- [ ] Message normalization helpers

### 1.5 Tests for `src/session-turn-runner.ts`

Create `src/tests/unit/session-turn-runner.spec.ts`:

- [ ] `TurnRunnerContext` construction/validation
- [ ] `TurnRunnerState` transitions
- [ ] Turn status determination
- [ ] Final report detection logic

---

## Phase 2: Split Production Code

### 2.1 `src/tools/mcp-provider.ts` → 2 files (EASIEST)

**Current Structure:**
- Lines 1-186: Imports, types, `MCPRestartError` classes
- Lines 187-683: `MCPSharedRegistry` class (~500 lines)
- Lines 684-776: Helper functions
- Lines 777-1417: `MCPProvider` class

**What to Extract:**

```
src/tools/
├── mcp-provider.ts               # MCPProvider class + exports
├── mcp-shared-registry.ts        # MCPSharedRegistry class (NEW)
└── mcp-types.ts                  # SharedRegistryHandle, SharedRegistry interfaces (NEW)
```

**Extraction (types.ts):**
```typescript
// src/tools/mcp-types.ts - MOVE existing interfaces
export interface SharedAcquireOptions { ... }  // Line 105
export interface SharedRegistryHandle { ... }  // Line 133
export interface SharedRegistry { ... }        // Line 140
export type LogFn = ...                        // Line 28
```

**Extraction (mcp-shared-registry.ts):**
```typescript
// src/tools/mcp-shared-registry.ts - MOVE existing class
import type { SharedAcquireOptions, SharedRegistry, SharedRegistryHandle, LogFn } from './mcp-types.js';
export class MCPSharedRegistry implements SharedRegistry { ... }  // Lines 187-683
```

**Verification:**
- [ ] `npm run build` passes
- [ ] `npm run lint` passes with zero warnings
- [ ] All existing imports still work
- [ ] Phase 2 harness passes
- [ ] `shutdownSharedRegistry()` still exported from `mcp-provider.ts`

---

### 2.2 `src/cli.ts` → 3 files

**Current Structure:**
- Lines 1-110: Imports, exit handling, telemetry setup
- Lines 111-442: Option parsing, banner
- Lines 443-574: `listMcpTools()` function
- Lines 575-949: Help builders, defaults display
- Lines 950-1229: Headend config types, helpers
- Lines 1230-2042: `runHeadendMode()` function (~800 lines)
- Lines 2043-2225: Main run logic, `createCallbacks()`

**What to Extract:**

```
src/
├── cli.ts                        # Main entry (~1,200 lines)
├── cli/
│   ├── headend-mode.ts           # runHeadendMode() function (NEW)
│   ├── mcp-tools.ts              # listMcpTools() function (NEW)
│   └── types.ts                  # HeadendModeConfig interface (NEW)
```

**Extraction (cli/types.ts):**
```typescript
// MOVE existing interface
export interface HeadendModeConfig { ... }  // Line 950
```

**Extraction (cli/headend-mode.ts):**
```typescript
// MOVE existing function - it's already self-contained
import type { HeadendModeConfig } from './types.js';
export async function runHeadendMode(config: HeadendModeConfig): Promise<void> { ... }
```

**Extraction (cli/mcp-tools.ts):**
```typescript
// MOVE existing function
export async function listMcpTools(targets: string[], promptPath: string | undefined, options: Record<string, unknown>): Promise<void> { ... }
```

**Critical:** Keep exit handling centralized in `cli.ts` (lines 48-88).

---

### 2.3 `src/ai-agent.ts` → Align with Existing Modules

**IMPORTANT:** This file already imports from `src/cache/*` and `src/orchestration/*`. Do NOT create duplicate `session/cache.ts` or `session/orchestration.ts`.

**Current Structure:**
- Lines 1-110: Imports, types, cache payload helpers
- Lines 111-2194: `AIAgentSession` class
- Lines 2195-2280: Helper functions
- Lines 2281-2428: `AIAgent` class

**What to Extract (minimal, aligned with existing structure):**

```
src/
├── ai-agent.ts                   # Main classes (~2,000 lines)
├── ai-agent-types.ts             # ToolSelection, AgentCachePayload (NEW)
```

**Extraction (ai-agent-types.ts):**
```typescript
// MOVE existing interfaces and type guards
export interface ToolSelection { ... }        // Line 81
export interface AgentCachePayload { ... }    // Line 87
export const isAgentCachePayload = ...        // Line 93
```

**DO NOT create:**
- `session/cache.ts` - use existing `src/cache/*`
- `session/orchestration.ts` - use existing `src/orchestration/*`

---

### 2.4 `src/llm-providers/base.ts` → Extract Pure Helpers Only

**CRITICAL:** Many methods depend on `this` context. Only extract functions that are ALREADY pure or can be made pure without behavior changes.

**Current Structure:**
- Lines 1-76: Imports, interfaces, constructor
- Lines 77-156: Reasoning resolution methods
- Lines 157-238: Message normalization
- Lines 239-500: Tool handling
- Lines 501-1000: Request building
- Lines 1001-1500: Streaming execution
- Lines 1501-2000: Non-streaming execution
- Lines 2001-2550: Response processing

**What to Extract (PURE HELPERS ONLY):**

```
src/llm-providers/
├── base.ts                       # BaseLLMProvider class
├── provider-types.ts             # Shared interfaces (NEW)
├── format-policy.ts              # Format policy utilities (NEW - if pure)
```

**Extraction (provider-types.ts):**
```typescript
// MOVE existing interfaces
export interface LLMProviderInterface { ... }  // Line 31
export interface FormatList { ... }            // Line 36
export interface FormatPolicyNormalized { ... } // Line 42
export interface FormatPolicyInput { ... }     // Line 47
export interface ResponseMessage { ... }       // Line 2550
```

**DO NOT extract:**
- Message conversion methods (depend on `this`)
- Streaming/non-streaming execution (deeply coupled)
- Response parsing (uses protected methods)

---

### 2.5 `src/session-turn-runner.ts` → Types Only (HARDEST)

**CRITICAL:** This is the most tightly coupled file. The `TurnRunner` class has extensive internal state mutations. Extracting logic risks behavior drift.

**Recommended Approach:** Extract ONLY types initially.

```
src/
├── session-turn-runner.ts        # TurnRunner class (unchanged logic)
├── turn-runner/
│   └── types.ts                  # Interfaces only (NEW)
```

**Extraction (turn-runner/types.ts):**
```typescript
// MOVE existing interfaces - NO LOGIC
export interface TurnRunnerContext { ... }     // Line 61
export interface TurnRunnerCallbacks { ... }   // Line 97
export interface TurnRunnerState { ... }       // Line 113 (if exported)
```

**DO NOT extract (yet):**
- `sanitizeAssistantMessage` - doesn't exist as named function
- `evaluateRetryCondition` - doesn't exist as named function
- `extractFinalReport` - doesn't exist as named function
- Any retry/provider cycling logic - deeply coupled to state

**Future Work:** After types extraction is stable, identify pure helper candidates by looking for methods that:
1. Take all inputs as parameters
2. Return values without mutating class state
3. Don't call other instance methods

---

## Phase 3: Verify Each Split

After EACH file split:

### Verification Checklist

- [ ] `npm run build` passes
- [ ] `npm run lint` passes with zero warnings
- [ ] `npm run test` passes (all unit tests)
- [ ] Phase 2 harness passes: `npm run test:phase2`
- [ ] No new circular dependencies: check import graph
- [ ] All previous public exports still available from original file
- [ ] Commit separately with descriptive message

### Behavior Verification

```bash
# Before split - capture baseline
./run.sh "test prompt" > /tmp/before.txt 2>&1

# After split - compare
./run.sh "test prompt" > /tmp/after.txt 2>&1
diff /tmp/before.txt /tmp/after.txt
```

---

## Phase 4: Documentation Updates

After all splits complete:

- [ ] `docs/specs/IMPLEMENTATION.md` - Update module organization
- [ ] `docs/specs/DESIGN.md` - Update if architecture diagram affected
- [ ] `docs/specs/AI-AGENT-INTERNAL-API.md` - Update file references
- [ ] `README.md` - Update if user-facing paths changed
- [ ] Update any hardcoded paths in configs

---

## Phase 5: Split Test Files (Lower Priority)

Test file splitting is cosmetic - it doesn't affect production reliability. Do this AFTER production code is stable.

### 5.1 `src/tests/phase2-harness-scenarios/phase2-runner.ts` (~13,749 lines)

**Split Plan:**

```
src/tests/phase2-harness-scenarios/
├── phase2-runner.ts              # Main runner (~1,200 lines)
├── phase2-utilities.ts           # Helper functions (~1,300 lines)
├── phase2-types.ts               # HarnessTest interface, types
├── scenarios/
│   ├── index.ts                  # Combines all scenario arrays
│   ├── context-guard.ts          # Context guard scenarios
│   ├── tool-execution.ts         # Tool execution scenarios
│   ├── retry-recovery.ts         # Retry scenarios
│   ├── format-output.ts          # Format scenarios
│   └── ...                       # Other categories
```

**Critical:** Update `src/tests/phase2-harness.ts` import after split:
```typescript
// Before
import { runPhaseOneSuite } from './phase2-harness-scenarios/phase2-runner.js';

// After - must still work
import { runPhaseOneSuite } from './phase2-harness-scenarios/phase2-runner.js';
```

**Preserve scenario ordering** - test execution order must remain stable.

---

## Decisions Made

Based on Codex + GLM-4.7 review:

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Test paths | Use `src/tests/...` | Matches existing repo structure |
| Production vs test priority | Production first | Test file splitting is cosmetic |
| New folders vs existing | Use existing `cache/`, `orchestration/` | Avoid duplication |
| Extraction approach | Types first, pure helpers only | Minimizes risk of behavior changes |
| `session/` folder | DO NOT CREATE | Conflicts with existing modules |
| Barrel files (`index.ts`) | AVOID | Risk of circular imports |

---

## Execution Order

| Step | Phase | Action | Risk |
|------|-------|--------|------|
| 1 | 0 | Baseline metrics | None |
| 2 | 0 | Enable coverage | None |
| 3 | 1.1 | Add tests for `mcp-provider.ts` | None |
| 4 | 2.1 | Split `mcp-provider.ts` | LOW |
| 5 | 1.2 | Add tests for `cli.ts` | None |
| 6 | 2.2 | Split `cli.ts` | LOW |
| 7 | 1.3 | Add tests for `ai-agent.ts` | None |
| 8 | 2.3 | Split `ai-agent.ts` (types only) | LOW |
| 9 | 1.4 | Add tests for `base.ts` | None |
| 10 | 2.4 | Split `base.ts` (types only) | MEDIUM |
| 11 | 1.5 | Add tests for `session-turn-runner.ts` | None |
| 12 | 2.5 | Split `session-turn-runner.ts` (types only) | MEDIUM |
| 13 | 5 | Split test files | LOW |
| 14 | 4 | Update documentation | None |

---

## Risk Mitigation

### Circular Dependencies
- Check import graph before each split
- Never import from parent into extracted module
- Use type-only imports where possible

### Behavior Drift
- Run phase2 harness after each split
- Compare session snapshots before/after
- Keep extraction minimal - types and pure helpers only

### Build/Lint Failures
- Run `npm run build && npm run lint` before committing
- Fix all warnings, not just errors

### Test Breakage
- Update imports in test files when source moves
- Preserve scenario ordering in phase2 harness

---

## Notes

- Each split = separate commit for easy rollback
- DO NOT use `git add -A` - commit only specific files
- Keep backward-compatible re-exports during transition
- Avoid barrel files (`index.ts`) - they cause circular import issues
- When in doubt, extract LESS not more

---

## Review Notes (2026-01-17)

- Status: Plan is close, but not sound yet. Several items conflict with "only move code" and doc-sync requirements.
- Critical conflicts to resolve:
  - "Strangler pattern" + "explicit dependency injection" conflicts with "only move existing code" (Guiding Principles #2 vs #7).
  - Docs are updated only after all splits, but commits are per split (doc-sync risk).
  - Base provider helpers listed for tests/extraction may be methods, which would require refactoring.
- Missing considerations:
  - Non-deterministic behavior checks (run.sh diff) and module side-effect order.
  - Whether `npm run test` and `test:phase2` are valid scripts and stable in CI.
  - How unit tests will access private/internal functions without changing exports.

## Decisions Required (Review)

1. **Test-first interpretation**
   - A) Add tests only for the next file before splitting that file (current plan).
   - B) Add tests for all targeted production files before any splitting starts.
   - Recommendation: B. It is the safest reading of "tests must be added BEFORE splitting production code."

2. **Documentation timing**
   - A) Update docs after all splits (current plan).
   - B) Update docs in the same commit as each split that changes file structure.
   - Recommendation: B. It keeps docs synchronized per repo policy.

3. **LLM provider helper extraction**
   - A) Move only top-level pure helpers/types that already exist.
   - B) Allow turning class methods into pure helpers if behavior stays identical.
   - Recommendation: A. Option B is refactoring and violates "only move code."

4. **"Strangler pattern / dependency injection"**
   - A) Remove from plan for this task.
   - B) Keep, but only if no code changes are required beyond moves.
   - Recommendation: A. It reduces scope and avoids refactoring risk.
