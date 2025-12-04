# TODO - Reliability logging & streaming filters

## TL;DR
- Improve diagnostics for missing tools/final-report cases, XML malformation, and retries; fail session after exhausted attempts.
- Preserve subagent streams with metadata while preventing leakage to headends that expect only root agent output; ensure onOutput/onThinking meta.
- Clarify final-report structure (no wrapper) and schema expectations across headends.

## Analysis
- Logs show multiple "Synthetic retry: no tools/final_report" without payload dumps; retry loop can continue after 5 invalid attempts instead of failing session.
- Malformed XML errors are generic and lack raw response dumps.
- Streaming headends currently receive subagent outputs; need meta to allow filtering by root vs child.
- OpenAI/Anthropic headends filtering only thinking; need parity for output; REST/MCP non-streaming unaffected.
- Final-report wrapper removal is done, but headends may still expect wrapped JSON; confirm schema expectations.

## Decisions
1. Add full payload dumps (no truncation) for synthetic retry and malformed XML cases; include raw content bytes and reasoning flags.
2. Abort session when invalid-response retries are exhausted (e.g., 5 attempts) instead of advancing silently.
3. Propagate callback metadata with agent identity; headends may filter non-root streams, but streams must carry agentId for future multi-viewers.
4. Final-report JSON should be bare payload (no wrapper); headends should not depend on wrapper.
5. Remove final-report instructions from XML-NEXT; keep them only in the system prompt.

## Plan
1) Instrument session-turn-runner: emit detailed dumps for no-tools/no-final-report and malformed XML; add helper for reuse; include raw assistant response.
2) Enforce session failure when retry budget is exhausted for invalid responses.
3) Propagate meta on onOutput/onThinking; ensure subagent registry forwards meta; headends filter by root agent only where required (OpenAI/Anthropic).
4) Verify final-report dump reflects actual model payload; ensure no wrapper required in prompts.
5) Run `npm run lint` and `npm run build`.

## Implied Decisions
- REST/MCP/CLI headends keep default behavior (non-streaming) so no filtering change needed now.
- Future headends can leverage meta to display subagent streams; filtering stays in headend layer.

## Testing Requirements
- `npm run lint`
- `npm run build`

## Documentation Updates Required
- Not yet identified; update if schemas/prompts change.
