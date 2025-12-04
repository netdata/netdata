# TODO - master loop logging and build fix

## TL;DR
- Fix current build failure in `src/cli.ts` (missing `CallbackMeta` type) so `npm run build` passes.
- Analyze master/global turn loops to catalog “no final-report, no tools” failure conditions and ensure single detailed log with raw response dump.

## Analysis
- Build currently fails: `src/cli.ts(2080,37): error TS2304: Cannot find name 'CallbackMeta'.` and same at line 2086. Indicates missing type import for callback metadata in CLI base callbacks.
- Main agent orchestration lives in `src/ai-agent.ts` (session wrapper) and `src/session-turn-runner.ts` (turn loop, provider retries, tool orchestration). Failure modes likely handled there; need thorough pass.

## Decisions (pending)
1. Logging format/location for the consolidated “no final-report, no tools” failures:
   - Need to choose log severity/remoteIdentifier, payload shape (tools/content/reasoning/raw response), and ensure emitted once per failed turn.
2. Whether to reuse existing error codes (e.g., `EXIT-EMPTY-RESPONSE`, `TURN_FAILED_NO_TOOLS_NO_REPORT_CONTENT_PRESENT`) or introduce new identifiers.

## Plan
1. ✅ Fix build error by importing `CallbackMeta` type in `src/cli.ts`; rerun `npm run build` to confirm.
2. Map master/global and per-turn loop failure conditions related to: empty response, reasoning-only, content-only without final report tag, unparsable tools, or other tool failures.
3. Identify current logging for these cases; verify whether single detailed log exists; note gaps.
4. Propose and implement logging changes (after user sign-off) to emit unified log entry with tools/content/reasoning and raw response when conditions hit.
5. Update documentation if behavior changes; run lint/build.

## Implied Decisions
- Assume no additional runtime behavior changes beyond logging unless user approves.

## Testing Requirements
- `npm run build`
- `npm run lint` (after changes)

## Documentation Updates
- If logging behavior changes or new error identifiers are added, update relevant docs (likely `docs/IMPLEMENTATION.md` or logging docs).
