This is a live application developed by ai assistants. I have the impression that over time the application lost a few key principles:

1. separation of concerns (isolated modules that do not touch the internals of each other)
2. simplicity (refactors introduced transformations instead of completing the refactoring)
3. single point of reference (multiple similar functions with tiny or no differences)
4. unecessary complexity (features that could implemented in a much simpler way, but the assistants overcomplicated things)
5. bad practices, smelly code

I need you to:

1. Complete the thorough code review to identify a single, risk-free, consolidation, simplification, improvement on separation of concerns
2. Make sure this aspect is well tested before touching the code - if you need to add tests, add them
3. Improve it
4. Make sure the tests still pass
5. Make sure documentation is updated as necessary

Do not ask any questions. If you have to ask questions, the candidate you picked is not right. We want only low-risk candidates that are obvious how to fix without breaking any of the existing features and functionality.

Append all you findings in this document.

---

## TL;DR
- Consolidate duplicated prompt-section formatting used by the OpenAI and Anthropic completions headends into a single helper to keep one source of truth.
- Add a focused unit test for the shared helper, then refactor both headends to use it with no behavior change.
- Run lint + build + unit tests to confirm zero regressions.

## Analysis (facts only)
- Duplicate prompt section prefixes and assembly exist in both completions headends:
  - `src/headends/openai-completions-headend.ts:75-77` and `src/headends/openai-completions-headend.ts:999-1027`
  - `src/headends/anthropic-completions-headend.ts:75-77` and `src/headends/anthropic-completions-headend.ts:964-991`
- The formatting is identical: build `System context`, `Conversation so far`, and `User request` sections, joined with `\n\n`.
- This duplication violates the “single point of reference” principle and risks divergence if one headend changes the format while the other does not.

## Decisions
- No user decisions required. Scope is a safe refactor with no behavior change.

## Plan
1. Add `src/headends/completions-prompt.ts` with a single `buildPromptSections()` helper that formats the three sections exactly as today.
2. Add a unit test for `buildPromptSections()` under `src/tests/unit/` to lock behavior.
3. Replace the duplicated prompt assembly in `openai-completions-headend.ts` and `anthropic-completions-headend.ts` to call the helper.
4. Run `npm run lint`, `npm run build`, and `npm run test:phase1`.

## Implied Decisions
- Keep the existing error messages and validation logic in each headend (the helper only formats strings).
- No change to output schemas, routing, or streaming behavior.

## Testing requirements
- Unit test: new test for shared prompt formatting helper.
- Required checks: `npm run lint`, `npm run build`, `npm run test:phase1`.

## Documentation updates required
- None. This is a refactor with no behavior or schema changes.

## Findings (append-only)
- Duplicated prompt-prefix constants and prompt assembly logic in both completions headends:
  - `src/headends/openai-completions-headend.ts:75-77`, `src/headends/openai-completions-headend.ts:999-1027`
  - `src/headends/anthropic-completions-headend.ts:75-77`, `src/headends/anthropic-completions-headend.ts:964-991`
- Instructions reference `docs/docs/AI-AGENT-INTERNAL-API.md`, but the file lives at `docs/AI-AGENT-INTERNAL-API.md` (path mismatch only; not in scope for this change).
