# Phase 2 Harness Coverage Matrix

This document maps core loop branches in `TurnRunner.execute()` to existing test scenarios.

## Coverage Summary

| Category | Branches | Covered | Coverage |
|----------|----------|---------|----------|
| Turn Loop Control | 4 | 4 | 100% |
| Context Guard | 4 | 4 | 100% |
| Retry Loop Control | 10 | 10 | 100% |
| Turn Exhaustion | 3 | 3 | 100% |
| Post-Loop Handling | 2 | 2 | 100% |
| Final Report Validation | 8 | 8 | 100% |
| **Total** | **31** | **31** | **100%** |

---

## Detailed Branch Coverage

### Turn Loop Control

| Branch | Description | Test(s) |
|--------|-------------|---------|
| `isCanceled()` check | Early exit on cancel signal | `run-test-49`, `run-test-44` |
| Stop reason: `stop` | Set forced final turn | `run-test-stopref-reason-stop` |
| Stop reason: `abort`/`shutdown` | Immediate canceled session | `run-test-stopref-reason-abort`, `run-test-stopref-reason-shutdown` |
| Stop reason: undefined | Legacy graceful stop | `run-test-44`, `run-test-43` |

### Context Guard

| Branch | Description | Test(s) |
|--------|-------------|---------|
| Provider status: `final` | Enforce final turn due to context | `context_guard__forced_final`, `suite-context-guard-forced-final` |
| Provider status: `skip` | Skip provider, warn about context | `context_guard__skip_provider` |
| Base overflow check | `enforceDueToBaseOverflow` | `context_guard__overflow` |
| Post-enforce still blocked | Log forced final turn warning | `context_guard__multi_turn` |

### Retry Loop Control

| Branch | Description | Test(s) |
|--------|-------------|---------|
| Abort/shutdown during retry | Immediate exit on abort | `run-test-stopref-reason-abort`, `run-test-stopref-reason-shutdown` |
| Empty response without tools | Retry - `empty_without_tools` | `run-test-37`, `suite-error-retry-success` |
| Reasoning only response | Log reasoning_only failure | `suite-reasoning-content-no-tools-retries` |
| Tool message fallback | Adopt final report from tool result | `run-test-final-report-tool-message-fallback` |
| Turn success determination | `turnSuccessful = true` | All passing tests |
| Final turn without report | Retry - `final_turn_no_report` | `run-test-102` |
| No tools executed | Retry - `no_tools` | `suite-task-status-exhausts-turns` |
| Rate limit handling | Provider cycling, backoff | `run-test-90-rate-limit` |
| Rate limit - all providers | Sleep with abort handling | `run-test-90-rate-limit` |
| Non-rate-limit retry | Retry directive execution | `run-test-29`, `suite-error-multiple-retries` |

### Turn Exhaustion

| Branch | Description | Test(s) |
|--------|-------------|---------|
| User stop during retry | Continue to final turn | `run-test-stopref-reason-stop` |
| Already in final turn | Fail session | `run-test-max-turn-limit` |
| Not final turn | Set retry exhaustion reason | `suite-task-status-exhausts-turns` |

### Post-Loop Handling

| Branch | Description | Test(s) |
|--------|-------------|---------|
| No final report, tool didn't fail | Synthesize final report | `run-test-101` |
| Final report tool failed | Different handling | `run-test-102` |

### Final Report Validation

| Branch | Description | Test(s) |
|--------|-------------|---------|
| Tool failure detected | Skip adoption | `run-test-102` |
| Format mismatch | Fail - `final_report_format_mismatch` | `run-test-final-report-format-mismatch` |
| Sub-agent format | Opaque blob validation | `run-test-subagent-format` |
| JSON format | Parse and validate | `suite-format-json-valid-object` |
| Slack format | Block kit validation | `suite-format-slack-valid-blocks` |
| Text formats | Content validation | `suite-format-pipe-plain`, `suite-format-markdown-formatting` |
| Schema validation | Pre-commit validation | `run-test-schema-validation` |
| Commit final report | Success path | All passing tests with final reports |

---

## Test Suite Distribution

### Suite Tests (34 tests)

| Suite | Tests | Coverage Area |
|-------|-------|---------------|
| core-orchestration.test.ts | 3 | Turn loop basics |
| final-report.test.ts | 3 | Report validation |
| tool-execution.test.ts | 3 | MCP tool handling |
| context-guard.test.ts | 3 | Context limits |
| task-status.test.ts | 4 | Progress tracking |
| router-handoff.test.ts | 3 | Agent delegation |
| error-handling.test.ts | 3 | Retry recovery |
| reasoning.test.ts | 4 | Content handling |
| format-specific.test.ts | 4 | Output formats |
| coverage.test.ts | 4 | Edge cases |

### Base Tests (257 tests)

Located in `phase2-runner.ts`:
- Core orchestration: ~30 tests
- Final report: ~25 tests
- Tool execution: ~20 tests
- Context guard: ~15 tests
- Task status: ~10 tests
- Router/handoff: ~10 tests
- Error handling: ~20 tests
- Reasoning: ~10 tests
- Coverage/misc: ~100+ tests
- **stopRef reason coverage: 3 tests**
- **Sleep abort path coverage: 10 tests** (rate limit + retry backoff × cancel/stop/abort/shutdown/legacy)

---

## stopRef.reason Branch Coverage (session-turn-runner.ts:318-332)

The turn loop checks `stopRef.reason` to determine behavior:

```typescript
if (this.ctx.stopRef?.stopping === true) {
    const reason = this.ctx.stopRef.reason;
    if (reason === 'stop') {           // → run-test-stopref-reason-stop
        // Force final turn, continue execution
    } else if (reason === 'abort' || reason === 'shutdown') {  // → run-test-stopref-reason-abort, run-test-stopref-reason-shutdown
        return this.finalizeCanceledSession(...);  // success=false, error='canceled'
    } else {  // undefined (legacy)    // → run-test-44, run-test-43
        return this.finalizeGracefulStopSession(...);  // success=true
    }
}
```

| Test | Reason | Expected Behavior |
|------|--------|-------------------|
| `run-test-stopref-reason-stop` | `'stop'` | Forces final turn, allows final report |
| `run-test-stopref-reason-abort` | `'abort'` | Immediate exit, `success=false, error='canceled'` |
| `run-test-stopref-reason-shutdown` | `'shutdown'` | Immediate exit, `success=false, error='canceled'` |
| `run-test-44`, `run-test-43` | `undefined` | Legacy graceful stop, `success=true` |

---

## Sleep Abort Path Coverage (session-turn-runner.ts:1826-1882)

During rate limit and retry backoff sleeps, the system checks for abort/stop signals. These paths are exercised by dedicated tests:

### Rate Limit Sleep Abort (lines 1826-1843)

| Test | Signal | Expected Behavior |
|------|--------|-------------------|
| `run-test-ratelimit-sleep-abort-cancel` | AbortController.abort() | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-ratelimit-sleep-abort-stop` | `reason: 'stop'` | Forces final turn, allows final report |
| `run-test-ratelimit-sleep-abort-abort` | `reason: 'abort'` | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-ratelimit-sleep-abort-shutdown` | `reason: 'shutdown'` | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-ratelimit-sleep-abort-legacy` | `reason: undefined` | `finalizeGracefulStopSession`, `success=true` |

### Retry Backoff Sleep Abort (lines 1864-1882)

| Test | Signal | Expected Behavior |
|------|--------|-------------------|
| `run-test-retry-backoff-abort-cancel` | AbortController.abort() | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-retry-backoff-abort-stop` | `reason: 'stop'` | Forces final turn, allows final report |
| `run-test-retry-backoff-abort-abort` | `reason: 'abort'` | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-retry-backoff-abort-shutdown` | `reason: 'shutdown'` | `finalizeCanceledSession`, `success=false, error='canceled'` |
| `run-test-retry-backoff-abort-legacy` | `reason: undefined` | `finalizeGracefulStopSession`, `success=true` |

---

## Validation

- Build: PASS
- Lint: PASS
- Tests: 291/291 PASS (249 parallel, 42 sequential)
