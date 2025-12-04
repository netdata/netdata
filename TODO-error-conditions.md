# TODO: Identify error conditions in master loops triggering retries or turn advancement

## TL;DR
- Goal: define error/turn-failure semantics. New structure agreed: single-turn runner returns success/failure/finished/aborted (no retries/rate-limit); master loop handles retries/backoff/model switching; session ends on final report, max turns, or retry exhaustion. Turn success requires at least one non-progress/batch tool call attempt or non-empty final_report; tool failures recorded but turn fails only if zero tools ran and no final_report.
- Status: decisions captured; ready to design refactor plan.

## Analysis
- Pending: need to review docs (SPECS, IMPLEMENTATION, DESIGN, MULTI-AGENT, AI-AGENT-GUIDE, AI-AGENT-INTERNAL-API) and master loop code to map retry/advance error flows.

## Decisions
1. Architecture split: **Chosen 1A** – single-turn runner has no retries/backoff; master loop owns retries/backoff/model switching.
2. Turn success criteria: **Chosen 2A** – success requires ≥1 non-progress/batch tool call attempt or non-empty final_report.
3. Tool failures vs zero tools: **Chosen** – tools may fail; turn fails only if zero tools executed and no final_report (unknown/invalid tools don’t count as “executed”).
4. Synthetic final reports location: **Chosen 4A** – caller/entry-point ensures a report is always produced; master/turn runners do not synthesize.
5. Rate-limit/backoff location: **Chosen 5A** – entirely in master loop.
6. Testing stance: tests assert public contract via counters (turns, retries, tools called/failed, llm calls, tokens, cost, failure codes, etc.); every test validates all counters; tests ignore internals/logs.
7. Context guard ownership: context management must be a separate module reused by runners, not embedded.
8. Open: Confirm or replace the current simplification with an alternative architecture (state-machine core, deterministic reducer, stricter error taxonomy). Pending user choice.
9. Separation of concerns directive: turn runner must treat tools as opaque; no knowledge of tool internals, queues, model switching, opTree, or logging. It receives tool handles/requests, invokes via provided abstraction, and returns the raw response/status only.

## Implied Decisions
- Turn runner output contract: `success | failure | finished (final_report) | aborted`; master loop interprets for retries.
- Master loop stop conditions: final_report, max turns, retry exhaustion (per decisions 1,4,5).
- Entry-point (library) must synthesize failure report if master loop ends without final_report (per synthetic fallback guidance).
- Counters must be exposed publicly for tests to assert; framework may need redesign to consume them uniformly.

## Plan
1. Read required design/spec docs sections about orchestration/master loops.
2. Locate master loop implementation in codebase; map states, retry logic, turn advancement conditions.
3. Extract all error conditions that trigger retries.
4. Extract error conditions that allow advancing the turn (before/after retries).
5. Summarize findings back to user; update documentation pointers if needed.

## Implied Decisions
- None yet.

## Testing requirements
- No code changes planned; no tests required.

## Documentation updates required
- None anticipated unless discrepancies found.
