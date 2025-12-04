TL;DR
- Need richer, category-specific logging for “no tools, no final-report” outcomes (empty/whitespace, text-only, reasoning-only, malformed tool/final_report).
- Goal: make every such branch emit enough data to diagnose whether the model or our prompts are at fault.

Analysis
- Current handling in `src/session-turn-runner.ts`:
  - Empty/whitespace without tools → `logNoToolsNoReport` (provider + orchestrator) and marks `invalid_response`.
  - Text without tools/final_report on non-final turn → synthetic retry, logs via `logNoToolsNoReport` (provider + orchestrator).
  - Final-answer without tools → orchestrator warning; may accept pending fallback; no category detail.
  - Text-extraction fallback is only attempted when native transport, no tools, text present.
  - Malformed tool calls are logged separately via `REMOTE_SANITIZER`, but not linked to “no tools/final report” categorization.
  - `logNoToolsNoReport` already collects bytes, preview, reasoning bytes, response bytes, provider/model metadata, but does not emit a category code or note whether reasoning-only or empty.
- Missing today:
  - A unified category code for each subcondition (empty, text-only, reasoning-only, malformed-tool, malformed-final-report).
  - Correlation to final turn vs non-final turn.
  - Explicit “model vs prompt” hints (e.g., assistant_reasoning=true & content='' should be differentiated).
  - De-duplication guard to avoid spamming identical logs per attempt while still capturing provider + orchestrator views.
  - Aggregated summary in the final failure/synthetic report.

Decisions (Costa)
1. Category taxonomy: pick Option A ({EMPTY, TEXT_ONLY, REASONING_ONLY, MALFORMED_TOOL, MALFORMED_FINAL_REPORT, TOOL_CALL_DROPPED}).
2. Log destinations: pick Option B (consolidated orchestrator log with structured details; keep provider log for symmetry).
3. Final-turn behavior: choose Option A (no extra summary log; keep existing retry/fallback logging only).
4. Data fields: choose Option B (include bytes/preview/reasoning/finish_reason/tool_count/turn/attempt plus pending extraction flag, finalReportAttempted, tokenizer counts).
5. Malformed tool/final_report: choose Option B (keep sanitizer logs separate; categorize as MALFORMED_TOOL when no valid tools remain).
6. Turn advancement on invalid-response exhaustion: keep current master-loop design; only surgical fixes (no new cap/loop redesign).

Plan (draft; will execute after decisions)
- Map all “no tools/no final report” entry points and tag each with category codes.
- Extend `logNoToolsNoReport` to accept a category and emit structured details (turn, attempt, isFinalTurn, provider/model, content/response/bytes, reasoning flag, tool counts).
- Wire the categories into:
  - empty/whitespace path,
  - text-only path,
  - reasoning-only path,
  - final-answer-without-tools path,
  - malformed tool/final-report paths.
- Add summary log on final turn when unresolved.
- Keep retry behavior unchanged; only logging is enriched.
- Tests: add Phase1 harness cases for each category; rerun full `npm run test:phase1`, plus `npm run lint`, `npm run build`.

Implied decisions
- No behavior change to retry/collapse logic unless user requests; focus is observability.
- Documentation update likely in `docs/TESTING.md` or `docs/AI-AGENT-GUIDE.md` to describe new diagnostics.

Testing requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1` (full suite)
- New targeted Phase1 scenarios for empty/text/reasoning/malformed categories.

Documentation updates required
- Add short section outlining “no tools / no final-report” diagnostic categories and where to find logs.
