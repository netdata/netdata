# TODO - Batch Tool Instructions Max Tools Per Turn

## TL;DR
Add the per-turn tool-call limit to the `agent__batch` instructions so users know the allowed maximum tools per turn when batching.

## Analysis
- `src/tools/internal-provider.ts` builds the internal tools instructions; the batch section currently explains `calls[]` but never mentions the per-turn tool-call cap.
- Tool-call limits are enforced in `AIAgentSession.executeSingleTurnInternal` (see `maxToolCallsPerTurn` derived from session config, default 10) with an error when `subturnCounter` exceeds the limit.
- `InternalToolProviderOptions` and `buildInstructions()` currently have no awareness of that limit, so instructions cannot surface it.
- `maxToolCallsPerTurn` is configurable via config/frontmatter/CLI (defaults: `options-resolver`→10) and is documented in `docs/AI-AGENT-GUIDE.md` but not exposed in the batch instructions presented to models.

## Decisions (need from Costa)
- Preferred phrasing/location: add a bullet in the batch section (under `#### agent__batch` or the mandatory rules) stating the numeric per-turn tool-call limit. OK? **Yes, approved**
- Use the resolved numeric limit for this session (e.g., "You can call up to X tools per turn"), or keep a generic reminder without dynamic numbers? (Recommendation: dynamic value to avoid drift.) **Use exact numeric per session**

## Plan
1) Thread the resolved `maxToolCallsPerTurn` into `InternalToolProvider` options and store it for instruction building (use the same `Math.max(1, value ?? 10)` logic as the turn executor).
2) Update `buildInstructions()` batch section to mention the per-turn tool-call cap using that resolved number, keeping guidance concise.
3) Confirm no other instruction copy needs adjustment; ensure progress/final report rules stay untouched.
4) Run lint/build to ensure zero warnings/errors.

## Implied Decisions
- Default limit remains 10 when unset; instructions will show that value unless a session override is provided.
- No behavior change to the actual limit—only surfaced text. If the limit logic changes later, instructions will still reflect the resolved value we pass.

## Testing Requirements
- `npm run lint`
- `npm run build`

## Documentation Updates Required
- Likely none beyond code-generated instruction text; verify after change. If any doc explicitly quotes batch instructions, update accordingly.
