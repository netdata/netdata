# TL;DR
- Update the final-turn log message to stop referencing the final_report tool and include the current/max turn (e.g., 11/30).
- Add a final-turn reason slug to the log and set severity based on the reason (task_status_completed should not be WRN).
- Identify and adjust all log messages that imply final_report is a tool (at least two in session-turn-runner + ai-agent).

# Analysis
- Log message in the user’s example comes from the final-turn entry log:
  - `src/session-turn-runner.ts` → `logEnteringFinalTurn(...)`:
    - Message today:
      - `Context guard enforced: restricting tools to \`agent__final_report\` and injecting finalization instruction.`
      - `Final turn (${turn}) detected: restricting tools to \`agent__final_report\`.`
    - Severity today: `WRN` for all reasons.
    - Triggered whenever `isFinalTurn` is true; reason derived from `forcedFinalTurnReason`.
  - `src/ai-agent.ts` → `logEnteringFinalTurn(...)`:
    - Same message and severity as session-turn-runner (duplicate assumption).
- Logs that explicitly assume final_report is a tool:
  - `src/session-turn-runner.ts` `logEnteringFinalTurn`: “restricting tools to `agent__final_report`”.
  - `src/ai-agent.ts` `logEnteringFinalTurn`: same wording.
  - Failure summaries/log content: “The final_report tool failed.”
    - `src/session-turn-runner.ts` uses this string in failure summaries (may appear in logs + synthetic final report content).
- Final-turn trigger sources currently in code:
  - `max_turns` (natural: `currentTurn === maxTurns`).
  - `context` (context guard enforces final turn: tool/turn preflight or during execution).
  - `task_status_completed` (context guard `setTaskCompletionReason()`).
  - `task_status_only` (5 consecutive standalone task_status calls → `setTaskStatusOnlyReason()`).
  - `retry_exhaustion` (context guard `setRetryExhaustedReason()`).
  - “Collapse remaining turns” paths (incomplete final report, final_report_attempt, xml_wrapper_as_tool) reduce `maxTurns` → next turn becomes final, but no explicit forcedFinalTurnReason is recorded for the log.
- The final-turn log currently only differentiates `context` vs default, so it cannot alter severity or wording for task_status_completed.
- The log line does not include the original max turn, only the single value passed into `logEnteringFinalTurn`. In one call site, `maxTurns` (post-collapse) is passed instead of `currentTurn`, so the log can be misleading.

# Decisions
- Decision (user): Only task_status_completed is a natural session end → log as VRB. All other final-turn reasons are unexpected/runaway mitigations → log as WRN.
- Decision (user): Add explicit reason slugs for collapse triggers (incomplete_final_report, final_report_attempt, xml_wrapper_as_tool) and include in final-turn logging.
- Decision (user): Rename “final_report tool failed” wording to avoid implying final_report is a tool.

# Plan
- ✅ Enumerate all log sites that mention “restricting tools to `agent__final_report`” and update to “removing all tools to force final-report”.
- ✅ Update `logEnteringFinalTurn` signature to accept `currentTurn` and `maxTurns` (original + current) and include both in the log message (e.g., `turn=11/30`).
- ✅ Extend `logEnteringFinalTurn` to accept a reason slug and map severity per reason (task_status_completed → VRB).
- ✅ Add new reason slugs for collapse paths and thread them into the final-turn log.
- ✅ Update Phase1 tests (where needed) to align with wording changes.

# Implied Decisions
- None.

# Testing Requirements
- Completed: `npm run lint`, `npm run build`, `npm run test:phase1` after log changes (252/252; expected warnings only).

# Documentation Updates Required
- None expected (unless we change public logging semantics and want to document).
