TL;DR
- Enforce new contract: when batch is available, only batch contains all tools (except final-report); final-report never in batch; final-report preempts all other tools.
- progress_report always exposed; per-turn rule: exactly one progress_report + at least one other tool (unless final_report present). Violations fail the turn with specific stub messages, WRN log, and system notice.
- Need contract doc update, implementation changes (tool exposure, orchestration, batch handling, per-turn validation), and Vitest/Phase tests covering new rules.

Analysis
- Current code exposes all tools top-level even when batch is enabled; no pruning for batch-only. progress_report gated by headend/availability (`enableProgressTool`), so it may be absent.
- No per-turn validation for progress_report presence/count/parallel use; turns with 0/many/progress-only currently pass.
- Final-report does not preempt other tools; other tools still execute when final_report is present.
- Multiple batch calls per turn currently allowed; batch rejects only nested batch/final_report at inner level.
- Batch container is not truncated (good); inner tool outputs can be truncated per response cap.
- Reasoning-only turn is allowed (no retry) if reasoning chunks exist; this may conflict with new “turn must have progress + other tool unless final_report”.
- Tool stubs today are generic; no specific messages requested by user; no system notice/WRN tied to these new rules.
- Tool schema selection (`selectToolsForTurn`, `filterToolsForProvider`) does not remap exposure when batch is present; forced-final turn only shrinks to final_report.
- Internal provider currently blocks final_report and nested batch inside batch; handles progress_report inside batch and respects `progressToolEnabled`.

Decisions (from user, binding)
- Remove headend gating: progress_report must always be available/exposed.
- When batch is available: all tools except final_report are only callable inside batch; top-level tools list contains batch + final_report only. final_report must never appear inside batch schema.
- final_report present in a turn: all other tools (batch or not) do not execute and fail with stub: "final-report called, tool not executed". final_report executes. This has highest priority over other rules.
- progress_report rule per turn (when final_report is absent): exactly one progress_report AND at least one other tool in the same turn (same assistant response). Multiple progress_reports, zero progress_report, or progress_report-only → turn failure.
- Batch semantics: batch is transport only and should not fail as a container except for parse/empty/truncation rules already present. If a second batch call appears in the same turn, it is parsed, but every inner tool in that second batch fails with stub: "only one batch per turn is allowed, tool not executed".
- Batch cannot include final_report; cannot include nested batch (existing behavior retained).
- System notice on turn failure (non-final_report rule violations) must be exactly:
  "Turn failed: {reason}"
  "Follow these rules:"
  "1. Call final_report to give your report to the user, without any other tools"
  "2. In all other cases, you must call exactly 1 progress_report per turn AND 1+ other tools"
- Tool failure stubs for progress rules:
  a) "(tool failed: turn is invalid, no progress_report called - you must call progress_report once per turn)"
  b) "(tool failed: turn is invalid, progress_report is called without other tools - you must call 1 progress_report and at least 1 other tool per turn)"
  c) "(tool failed: turn is invalid, multiple progress_report called - you must call 1 progress_report and at least 1 other tool per turn)"
- WRN log once per failed turn; message should include reason (user ok with suggested multi-version logs).
- System notice required on turn failure; apply even when progress rules violated.
- progress_report is just another tool inside batch schema (no special placement), but always exposed (headend gating removed).
- Contract doc update: consolidate batch/progress/final into dedicated sections; aggregate other relevant rules without changing them unless conflicting.

Open Questions (need answers to proceed)
1) Definition of "other tool": should any tool except agent__progress_report, agent__final_report, agent__batch qualify (including internal helper tools like agent__set_title), or only non-internal external tools?
2) Forced-final turn edge: when tool list is restricted to final_report (context/turn guard), and the model still fails to call final_report, should progress-rule enforcement run (and fail the turn) or be skipped in forced-final mode?
3) System notice placement: should the notice be an ephemeral retry message (not persisted in conversation history, using pushSystemRetryMessage) or a persistent system message appended to conversation?
4) Turn flow after violation: confirm we proceed to the next turn as usual (no special max-turn adjustment), counting the turn as failed but continuing session.
5) WRN variants: confirm one WRN per failed turn with reason codes (missing_progress, progress_only, multiple_progress, second_batch) is acceptable.
6) Reasoning-only turn: current code allows reasoning-only without tools to pass; should we now treat reasoning-only (no tool calls) as violating the progress rule and fail the turn with reason (a)?

Implementation Plan (draft; to refine after answers)
- Tool exposure layer: adjust selection/allowed tool sets so when batch is enabled, only batch + final_report are top-level; ensure progress_report always present (remove headend gating). Ensure schemaCtxTokens recomputed for reduced toolset.
- Batch execution: detect multiple batch calls per turn; for second and later batches, execute container, but mark each inner tool with the second-batch stub; ensure accounting entries reflect failures; keep container success unless parse/empty fails.
- Final-report precedence: pre-scan tool calls per turn; if any final_report present, fail all other tool calls (including inside batch) with the given stub; skip progress-rule checks in that case; ensure accounting + conversation tool messages use stub.
- Progress-rule validator: per turn (when no final_report), evaluate combined tool calls (including expanded batch). Enforce exactly one progress_report and >=1 other tool; on violation, mark all tools failed with the appropriate stub, log WRN once, append system notice.
- Conversation/system notice: inject per agreed mechanism (pending Q3); ensure not double-added across retries; ensure accounting entries for failed tools.
- Docs: update docs/CONTRACT.md with dedicated sections for tool exposure, per-turn constraints, failure handling; update AI-AGENT-GUIDE/DESIGN if needed to keep in sync.
- Tests: add Vitest/phase tests covering each rule: batch-only exposure; progress required; multiple progress; progress-only; missing progress; final_report preemption; second batch; system notice presence; WRN logs; accounting stubs.

Testing Requirements
- Run npm run lint and npm run build.
- Add/execute Vitest suites for the new rules (phase1/phase2 harness as applicable).
- Ensure batch/non-batch scenarios covered; final-report precedence; second batch rejection; progress-rule violations produce stubs + system notice + WRN.

Documentation Updates
- docs/CONTRACT.md: new sections for batch/progress/final exposure, per-turn tool constraints, failure handling; keep other rules intact unless conflicting.
- If AI-AGENT-GUIDE or DESIGN detail batch/progress availability, update for consistency.

Notes
- Do not change existing handling of empty/invalid batch parse/truncation.
- Maintain batch non-truncation at container level; allow per-tool truncation as today.
