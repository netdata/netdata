# TODO - run-test-90 sanitizer retry bug

## TL;DR
- Phase1 harness scenario `run-test-90` fails: expected two assistant messages after sanitizer retry, but conversation count mismatches.
- First LLM turn sends malformed tool params (`parameters: 'malformed'`) which sanitizer drops; retries proceed and final report succeeds.
- Need to adjust conversation/sanitizer retry handling so invalid attempt doesn't leave extra/ missing assistant message while preserving logs and retry flow.

## Analysis
- Reproduction: harness intercepts `LLMClient.executeTurn` (src/tests/phase1/runner.ts ~7048). Turn1 returns assistant message with invalid `test__test` tool call (`parameters` string). Turn2 returns valid tool call (`{ text: SANITIZER_VALID_ARGUMENT }`). Turn3 returns `agent__final_report` with final content.
- Expectations: sanitizer log present; assistantMessages length == 2; first assistant message should contain sanitized `test__test` call with text `sanitizer-valid-call`; retry warning log emitted; llmAttempts == 3; final report success.
- Current behavior (from code review):
  - sanitizeTurnMessages (src/ai-agent.ts ~3870) drops invalid tool calls, increments `dropped`, logs WRN, but returns sanitized message even if all tool calls dropped and content empty.
  - executeAgentLoop (src/ai-agent.ts ~1700-2240) always pushes `sanitizedMessages` into `conversation` before evaluating retry conditions. When `droppedInvalidToolCalls > 0`, it logs warning and sets `lastErrorType = 'invalid_response'`, causing retry while retaining the empty assistant message in conversation.
  - This likely yields 3 assistant messages (invalid+valid+final) instead of expected 2.
- Docs: docs/specs/retry-strategy.md says invalid tool params trigger retry and system notice; content-without-tools synthetic retry explicitly should **not** add conversation content. Behavior seems misaligned: invalid-tool-call attempt shouldn't persist empty assistant message in final conversation output used by harness expectations.
- Risk: need to ensure other scenarios (run-test-89, run-test-90-string, adopt-text/json-content/no-retry) remain satisfied; dropping only empty assistant messages with fully dropped tool calls should preserve cases where at least one valid call survives.

## Decisions
- Keep all assistant messages in the conversation history, even when sanitizer drops every tool call. Do not prune failed attempts.
- Tests must reflect the retention rule; expectations should not assume the invalid attempt is removed.

## Plan
1) Update run-test-90 expectation to allow retained failed-attempt assistant while still validating sanitized call/logs. ✅
2) Re-run run-test-90 harness to verify pass. ✅
3) Check sibling scenarios (run-test-89, run-test-90-string, etc.) for expectation alignment; adjust only if they conflict with retention rule. ⏳

## Implied decisions
- No code changes to sanitizer/retry logic; only test expectations align with retained messages.
- Logging (`Invalid tool call dropped... Will retry.`) remains unchanged.

## Testing requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1` or targeted phase1 harness covering run-test-90 family (minimum scenario run-test-90). ✅ (run-test-90)

## Documentation updates required
- None (behavior unchanged; only tests updated).
