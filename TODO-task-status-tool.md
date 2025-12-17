# TODO – Task Status Tool (Progress Report Redesign)

> **Potentially obsoletes:** `TODO-progress-report-standalone-loop.md`
>
> This is an alternative approach to the standalone loop problem. Instead of punitive disable/re-enable logic, this design channels the model's reporting behavior and makes loops self-terminating.

## Implementation Design

**Decision: Rename `agent__progress_report` → `agent__task_status`**
- Schema: `{status: 'starting'|'in-progress'|'completed', done: string, pending: string, now: string}`
- Behavior: Standalone calls allowed (turns consumed), self-terminating at 2nd call, retry exhaustion forces final-turn
- Migration: Immediate breaking change (all tests and docs updated at once)

## USER DECISIONS - FINALIZED (2025-12-16)

1. ✅ **Turn Advancement**: Option B - Turns consumed, everything else works
2. ✅ **Forced Final Turn Reasons**: `'context' | 'max_turns' | 'task_status_completed' | 'task_status_standalone_limit' | 'retry_exhaustion'`
3. ✅ **Status: Starting**: No special behavior, treat same as 'in-progress'
4. ✅ **Task Status Logging**: Emit to trace via `updateStatus()` callback
5. ✅ **Progress Gating Renaming**: `progressToolEnabled` → `taskStatusToolEnabled`
6. ✅ **Migration Strategy**: Immediate breaking change, replace all tests at once

## TL;DR

Rename `progress_report` to `task_status` with structured fields and new behavioral rules:
- Standalone calls are allowed (turns consumed) but self-terminating (2nd standalone → final-turn)
- Model can explicitly signal completion via `status: completed`
- Retry exhaustion forces final-turn instead of session failure

## Comparison: Old vs. New Approach

| Aspect | Old (disable/re-enable) | New (task-status) |
|--------|-------------------------|-------------------|
| **Philosophy** | Punitive — misuse = lose tool | Channeling — report freely, conclude gracefully |
| **Loop solution** | Abuse tracking, dynamic disable | Self-terminating (2nd standalone → final-turn) |
| **State complexity** | High (flags in 2 states, sync) | Low (simple counter) |
| **Model agency** | None | Can signal `completed` |
| **Failure mode** | Session fails on retry exhaustion | Graceful → force final-turn |
| **API change** | None | Breaking (rename + new schema) |
| **Code churn** | ~4 files, complex logic | ~3 files, simpler logic |

## New Tool Schema

```typescript
interface TaskStatusArgs {
  status: 'starting' | 'in-progress' | 'completed';
  done: string;    // What has been accomplished (up to 15 words)
  pending: string; // What remains to be done (up to 15 words)
  now: string;    // Your immediate step (up to 15 words)
  ready_for_final_report: boolean;  // True when you have enough info to provide final report
  need_to_run_more_tools: boolean;  // True when you plan to run more tools
}
```

### Triple Confirmation Logic

Task completion forces final-turn ONLY when all three conditions are met:
1. `status: 'completed'`
2. `ready_for_final_report: true`
3. `need_to_run_more_tools: false`

**2x2 Matrix of States**:
| ready_for_final_report | need_to_run_more_tools | Meaning |
|------------------------|------------------------|---------|
| true | true | "I could answer, but want to verify" |
| false | false | "Stuck" (forces final-turn) |
| false | true | "Working on it" (normal in-progress) |
| true | false | "Done" ← Forces final-turn |

## Behavioral Rules

### Rule 1: Standalone Tolerance (First Call)

| Condition | Behavior |
|-----------|----------|
| `task_status` called alone (no other tools) | **Allowed** — display status, **turn consumed** |

### Rule 2: Standalone Termination (Second Call)

| Condition | Behavior |
|-----------|----------|
| `task_status` called alone for 2nd time | **Force final-turn** — model must conclude |

### Rule 3: Explicit Completion (Triple Confirmation)

| Condition | Behavior |
|-----------|----------|
| `status: completed` + `ready_for_final_report: true` + `need_to_run_more_tools: false` | **Force final-turn** immediately |
| `status: completed` but other fields don't confirm | Normal operation, no force |

### Rule 4: Normal Usage

| Condition | Behavior |
|-----------|----------|
| `task_status` alongside real tools | Normal — just status reporting, counter resets |

### Rule 5: Graceful Exhaustion

| Condition | Behavior |
|-----------|----------|
| Retry exhaustion (any cause) | **Force final-turn** instead of session failure |
| Exception | If already in final-turn, fail normally |

## State Tracking

### New State Field

Add to `TurnRunnerState`:

```typescript
interface TurnRunnerState {
  // ... existing fields
  standaloneTaskStatusCount: number; // 0, 1, or triggers final-turn at 2
}
```

## Implementation Plan

### Phase 1: Tool Schema & Definition (File: src/tools/internal-provider.ts)
1. **Rename tool**: `agent__progress_report` → `agent__task_status`
2. **Update schema**: `{progress: string}` → `{status, done, pending, now, ready_for_final_report, need_to_run_more_tools}`
3. **Update validation logic**:
   - Parse `status` as enum: 'starting' | 'in-progress' | 'completed'
   - No special behavior for 'starting' (treat same as 'in-progress')
   - Validate done/pending/now as strings (soft guidance for 15-word limit)
   - Validate ready_for_final_report and need_to_run_more_tools as booleans (required)
   - Triple confirmation: completion only when status='completed' AND ready_for_final_report=true AND need_to_run_more_tools=false
   - Response: concatenate status + 3 strings as status message, return "ok"
4. **Update instructions**: Tool description and usage examples

### Phase 2: State Management (File: src/session-turn-runner.ts)
1. **Add state field**: `standaloneTaskStatusCount: number` to TurnRunnerState
2. **Initialize counter**: Set to 0 in constructor (line ~182)
3. **State persistence**: Counter persists across turns, resets on new sessions
4. **Rename gating**: `progressToolEnabled` → `taskStatusToolEnabled`

### Phase 3: Behavioral Rules Implementation (File: src/session-turn-runner.ts)
1. **Detection logic**: Replace `hasProgressOnly` with standalone detection
2. **Counter management**: 
   - Increment on standalone calls (turns consumed, rest works)
   - Reset to 0 when real tools succeed
   - Force final-turn when counter >= 2
3. **Status-based final-turn**: When `status: completed` detected, next turn should be final-turn (but don't interrupt current tools)

### Phase 3.1: Forced-Final Plumbing Integration
1. **Context Guard Updates**: Extend `forcedFinalTurnReason` type with new enum values
2. **Log/Exit Integration**: Add new forced-final reasons to exit codes, log slugs, and telemetry
3. **Context Guard Implementation**: Implement new reason handling in `context-guard.ts`
4. **Turn Runner Integration**: Wire new reasons into final-turn logic

### Phase 3.2: Progress Classification Sweep
1. **SessionToolExecutor**: Update `isProgressTool` detection from `'agent__progress_report'` → `'agent__task_status'`
2. **Success Criteria**: Update `hasNonProgressTools` logic to recognize task_status
3. **Batch Integration**: Update batch inner counting to exclude task_status as progress tool
4. **Failure Slugs**: Update failure slug generation to use new tool name

### Phase 3.3: Accounting Contract Update
1. **Update docs/AI-AGENT-INTERNAL-API.md**: Change `charactersOut=15` to variable length for task_status
2. **Update Contract**: Define new accounting expectations for multi-field payload
3. **Update Test Expectations**: Modify test fixtures to expect new accounting format

### Phase 4: Type System Changes
1. **Update ToolExecutionState**: Add `taskStatusCompleted: boolean`
2. **Update forcedFinalTurnReason**: Add `'task_status_completed' | 'task_status_standalone_limit' | 'retry_exhaustion'`
3. **Update ToolExecuteResult extras**:
   ```typescript
   interface ToolExecuteResult {
     // ... existing fields
     extras?: {
       taskStatusCompleted?: boolean;
       taskStatusData?: {
         status: 'starting'|'in-progress'|'completed';
         done: string;
         pending: string;
         now: string;
         ready_for_final_report: boolean;
         need_to_run_more_tools: boolean;
       };
     };
   }
   ```
4. **Update ContextGuard**: Support new forcedFinalTurnReason types

### Phase 5: Documentation & Testing
1. **Update documentation**: llm-messages.ts, docs/, test examples
2. **Test suite**: Replace ALL existing progress_report tests + add new tests (immediate breaking change)
3. **Migration tests**: Verify old tool calls are rejected (breaking change verified)

## Files Requiring Updates (34 files total)

### Core Implementation (4 files)
- `src/tools/internal-provider.ts` - Tool definition, schema, validation
- `src/session-turn-runner.ts` - State, detection logic, behavioral rules
- `src/session-tool-executor.ts` - Execution logic
- `src/llm-messages.ts` - Instructions and documentation

### Test Files (4 files)
- `src/tests/fixtures/test-llm-scenarios.ts`
- `src/tests/phase1/runner.ts`
- `src/tests/unit/xml-transport.spec.ts`
- `src/tests/unit/xml-tools.spec.ts`

### Documentation (7 files)
- `docs/AI-AGENT-GUIDE.md`
- `docs/SPECS.md`
- `docs/specs/tools-batch.md`
- `docs/AI-AGENT-INTERNAL-API.md`
- `docs/specs/tools-overview.md`
- `docs/specs/optree.md`
- `docs/CONTRACT.md`

### Configuration & Support (19 files)
- `src/internal-tools.ts` (reserved names)
- XML helpers, progress gating, batch tool integration
- All progressToolEnabled references
- API contract updates

## Testing Requirements

### New Test Scenarios

1. **`run-test-task-status-standalone-first`**
   - Model calls `task_status` standalone
   - Verify: status displayed, counter = 1

2. **`run-test-task-status-standalone-second`**
   - Model calls `task_status` standalone twice
   - Verify: second call triggers final-turn

3. **`run-test-task-status-completed`**
   - Model calls `task_status` with `status: completed`
   - Verify: final-turn enabled immediately

4. **`run-test-task-status-reset-on-real-tool`**
   - Model calls `task_status` standalone (counter = 1)
   - Model calls real tool successfully
   - Model calls `task_status` standalone again
   - Verify: counter was reset, no final-turn yet

5. **`run-test-retry-exhaustion-forces-final`**
   - Model exhausts retries (via any failure mode)
   - Verify: final-turn enabled instead of session failure

6. **`run-test-old-progress-report-rejected`** (BREAKING CHANGE)
   - Model calls old `agent__progress_report`
   - Verify: unknown tool error (immediate breaking change verified)

### Migration Testing
- **Scope**: Update ALL existing progress_report tests (immediate breaking change)
- **Verification**: All old tool calls fail, all new tool calls work

## Complexity Assessment

**Scope**: ~15 files affected (not 3 as initially estimated)
**Risk Level**: HIGH (affects core session mechanics)
**Timeline Impact**: Significant due to breaking changes across entire codebase
**Migration**: Immediate breaking change - all references updated simultaneously

