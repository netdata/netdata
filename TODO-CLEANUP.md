This is a live application developed by ai assistants. I have the impression that over time the application lost a few key principles:

1. separation of concerns (isolated modules should not touch the internals of each other)
2. simplicity (refactors introduced transformations instead of completing the refactoring)
3. single point of reference (multiple similar functions with tiny or no differences)
4. unecessary complexity (features that could implemented in a much simpler way, but the assistants overcomplicated things)
5. bad practices, smelly code

I need you to:

1. Complete a thorough code review to identify a single, risk-free, consolidation, simplification, improvement on separation of concerns - if this affects multiple modules, batch them all together
2. Make sure this aspect is well tested before touching the code - if you need to add tests, add them before consolidating
3. Improve it
4. Make sure the tests still pass
5. Make sure documentation is updated as necessary

Do not ask any questions. If you have to ask questions, the candidate you picked is not right. We want only low-risk candidates that are obvious how to fix without breaking any of the existing features and functionality.

We are mainly interested in:

- simplifying, reducing the code size of the core agent loops
- splitting the AISession god object into smaller manageable entities
- splitting very long files into submodules/components

Append all you findings in this document.

---

## TL;DR
- Target a single low-risk cleanup: remove unused duplicate stop/cancel finalization helpers from `AIAgentSession`.
- Keep `TurnRunner` as the single point of reference for stop/cancel finalization.
- No runtime behavior change expected; tests already cover `agent:fin` log emission for stop/cancel paths.

## Analysis (code review evidence)
- Duplicate stop/cancel finalization helpers exist in **`AIAgentSession`** but are **unused**:
  - Definitions: `src/ai-agent.ts:1793-1825` (`finalizeCanceledSession`, `finalizeGracefulStopSession`).
  - `rg` shows no call sites in `src/ai-agent.ts` for these methods.
- The **actual** stop/cancel finalization flow is implemented and used in **`TurnRunner`**:
  - Call sites: `src/session-turn-runner.ts:314-317`, `src/session-turn-runner.ts:416`, `src/session-turn-runner.ts:1706-1709`.
  - Implementations: `src/session-turn-runner.ts:2375-2383`.
- This is dead/duplicate code inside the “god object” (`AIAgentSession`) and violates “single point of reference.”

## Decisions
- None required. Safe removal of unused private methods only.

## Plan
1. Remove `finalizeCanceledSession` and `finalizeGracefulStopSession` from `src/ai-agent.ts`.
2. Ensure `TurnRunner` remains the only stop/cancel finalization logic (no behavior change).
3. Run required quality checks.

## Implied decisions
- Keep `TurnRunner` as the canonical stop/cancel finalization owner.
- No new tests added because behavior is unchanged and Phase 2 scenarios already validate `agent:fin` logs.

## Testing requirements
- `npm run lint`
- `npm run build`
- Recommended (core-loop coverage): `npm run test:phase2` (existing scenarios validate stop/cancel finalization logs).

## Documentation updates required
- None (no runtime behavior change).

---

## Cleanup Candidate 3: Consolidate duplicate opTree finalization blocks

## TL;DR
- The `agent:fin` opTree finalization block is duplicated three times in `AIAgentSession.run()`.
- Consolidate into one local helper to keep a single source of truth.
- Behavior stays identical: same messages, same `endSession` args.

## Analysis (code review evidence)
- Duplicate finalization blocks in `src/ai-agent.ts`:
  - Cache-hit path: `src/ai-agent.ts:1354-1361`
  - Normal path: `src/ai-agent.ts:1645-1653`
  - Uncaught error path: `src/ai-agent.ts:1740-1747`
- Each block:
  - Ensures system turn is started
  - Opens `fin` op
  - Logs `agent:fin` with a specific message
  - Ends op/turn and closes the session

## Decisions
- None required. Pure consolidation with no behavior change.

## Plan
1. Add a local helper inside `AIAgentSession.run()` to finalize opTree.
2. Replace the three duplicate blocks with calls to this helper.

## Implied decisions
- Keep current `agent:fin` messages (`SESSION_FINALIZED_MESSAGE` vs `session finalization (error)`).
- Keep `endSession` arguments unchanged per path.

## Testing requirements
- `npm run lint`
- `npm run build`
- Phase 2 optional (no behavior change).

## Documentation updates required
- None (no runtime behavior change).

---

## Cleanup Candidate 4: Consolidate repeated opTree flatten fallback

## TL;DR
- The opTree flatten fallback block is duplicated three times in `AIAgentSession.run()`.
- Extract a small local helper (`safeFlatten`) to keep one reference.
- Behavior stays identical.

## Analysis (code review evidence)
- Duplicated fallback block in `src/ai-agent.ts`:
  - Cache-hit path: `src/ai-agent.ts:1362`
  - Normal path: `src/ai-agent.ts:1656`
  - Uncaught error path: `src/ai-agent.ts:1744`
- The block is identical: try `opTree.flatten()` and fallback to `{ logs: this.logs, accounting: this.accounting }`.

## Decisions
- None required. Pure consolidation with no behavior change.

## Plan
1. Add `safeFlatten()` helper inside `AIAgentSession.run()` (near other helpers).
2. Replace the three inline blocks with calls to `safeFlatten()`.

## Implied decisions
- Keep fallback behavior exactly the same.

## Testing requirements
- `npm run lint`
- `npm run build`
- Phase 2 optional (no behavior change).

## Documentation updates required
- None (no runtime behavior change).

---

## Cleanup Candidate 2: Consolidate duplicate log-merge logic (single point of reference)

## TL;DR
- There are three identical log-merge blocks inside `AIAgentSession.run()`.
- Consolidate them into one local helper to reduce duplication and keep logic in one place.
- Behavior stays identical; no test changes needed.

## Analysis (code review evidence)
- Duplicate log-merge block appears three times in `src/ai-agent.ts`:
  - Cache-hit path: `src/ai-agent.ts:1347-1362`
  - Normal path: `src/ai-agent.ts:1656-1670`
  - Uncaught error path: `src/ai-agent.ts:1745-1759`
- Each block uses the same algorithm:
  - Start with `flat.logs`
  - Build a `seen` set using `timestamp + remoteIdentifier + message`
  - Append missing entries from `this.logs`

## Decisions
- None required. Pure consolidation with no behavior change.

## Plan
1. Add a local helper inside `AIAgentSession.run()` (near `mergeAccountingEntries`) to merge logs.
2. Replace the three duplicated blocks with calls to that helper.

## Implied decisions
- Keep the current deduplication key (`timestamp + remoteIdentifier + message`) unchanged.

## Testing requirements
- `npm run lint`
- `npm run build`
- Phase 2 optional (no behavior change).

## Documentation updates required
- None (no runtime behavior change).
