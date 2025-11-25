# TODO - XML Final Transport Fixes

## TL;DR
- Keep `xml-final` hybrid: native tool_calls for non-final tools; final report via XML tag only.
- Never drop XML silently: every malformed/mismatched tag must log + send feedback to the model (per RULE 1 & 2).
- Align prompts/tests/docs with the contract; progress-report stays suppressed in `xml-final`.
- Final-report must advertise and accept only the agent-required format (JSON schema or markdown); no dual-format fallback.
- Retry feedback must describe exact failure reasons (RULE 1/2); no silent failures (RULE 4).
- Decouple XML handling from main loop to simplify retries/error surfacing.

## Analysis (current state)
- `resolveToolChoice` now forces `tool_choice='auto'` when transport != native, even if `tools` is empty → provider request errors (OpenAI/Anthropic reject auto with no tools). Configuration overrides are ignored.
- `xml-final` currently merges parsed XML with native tool_calls but still says “No tools available” in XML-NEXT; tools list is hidden from the model and schemas aren’t surfaced.
- Allowed XML tools are too broad; non-final tools could be parsed in final slot. Progress slot missing when only internal tools exist.
- XML payload handling: malformed/non-JSON payloads are coerced to objects; final-report defaults to success/markdown; non-final tools receive raw strings → validation failures and silent drops.
- Final-report instructions hardcode markdown/text and ignore expected format/schema; JSON/structured finals can’t be advertised or validated.
- Partial/streamed XML tags are discarded on flush with no warning; model doesn’t get feedback (violates RULE 1/2/4).
- Docs and tests are out of sync: specs repeat CLI line, claim JSON optional; Phase1 scenarios expect wrong tool visibility; progress slot numbering complicates UX.
- Context guard: XML-NEXT counts toward schema tokens; XML-PAST counts toward tool-output tokens (RULE 3). Need to verify guard logic but early review suggests it’s wired correctly—keep it intact.
- XML logic still embedded in `ai-agent.ts`, making retry/error feedback paths messy.

## Decisions (pending/confirmed)
- Keep `xml-final` hybrid: native tool_calls for non-final tools; final via XML; progress suppressed. (Confirmed.)
- Restrict XML-acceptable tools in `xml-final` to final-report only (progress if ever re-enabled). Non-final XML tags must be rejected with logged + model-visible feedback. (RULE 1/4.)
- Surface available native tools in XML-NEXT for `xml-final`; keep schemas advertised in a concise form. (Align prompt messaging.)
- Final-report must support both JSON and text/markdown payloads; advertise expected format/schema per agent in XML-NEXT and enforce in validation. (RULE 1/2.)
- Progress-report & final-report should avoid slot numbers if doing so simplifies UX; propose dedicated identifiers (e.g., FINAL, PROGRESS) while keeping nonce scoping. Await your go/no-go.
- Context guard stays: XML-NEXT counts as schema tokens; XML-PAST counts as tool-output tokens. No change unless verification disproves.
- Every failed turn must be logged and fed back to the model with exact reason; no silent drops. (RULE 1/2/4.)

## Plan

1) Tool choice / provider contract
   - Fix `resolveToolChoice` to honor config/model overrides; avoid `tool_choice=auto` when `tools.length=0` (especially xml mode). Add regression test.

2) XML parsing & payload handling
   - Make parser streaming-safe: don’t drop partial tags on flush; accumulate and log/feedback malformed/incomplete tags.
   - Enforce allowed-tool set per mode; reject non-final XML in `xml-final` with logged + model-visible feedback (RULE 1/2/4).
   - Parse payloads: if JSON parse fails, log + send feedback; for final-report accept raw text/markdown or JSON object with status/format/content; for other tools require JSON object and fail loudly otherwise.

3) Prompting & slots
   - Update XML-NEXT: list native tools for `xml-final`, include concise schemas; progress suppressed. Switch final/progress slots to NONCE-FINAL / NONCE-PROGRESS.
   - Ensure XML-NEXT final-report template reflects the single expected format/schema (no dual-format option).

4) Feedback & retries
   - On every failed turn (malformed XML, missing closing tag, nonce/slot mismatch, invalid status/format/content, empty payload, no final-report), log and inject a clear retry notice to the model (RULE 1/2). Keep XML-PAST as normal payload per RULE 3.

5) Tests
   - Update Phase1 scenarios: xml-final should see native tools; progress test must not depend on numbered slots. Add unit tests for parser streaming, payload validation, tool_choice with empty tools.
   - Add regression for xml mode with tools=[] ensuring tool_choice not set to auto (provider error path is surfaced with model feedback).

6) Docs
   - Fix specs/tools-xml-transport.md duplication and reflect JSON+text finals; update README, AI-AGENT-GUIDE, other specs to new contract and RULE 1/2/4 expectations.

7) Cleanup
   - Revisit linger-handle reporting to include ChildProcess handles again.
   - Keep XML logic separated from main loop (keep `xml-tools.ts`; further decouple if needed).

## Notes from Claude review
- Tool choice fix should only trigger when `llmTools.length === 0`; xml-final still has native tools so auto is acceptable there.
- Parser reset per turn is fine; instead of accumulating across turns, log + feedback when `flush()` drops buffered partial XML.
- Dedicated slot IDs (FINAL/PROGRESS) may disrupt existing prompts; treat as optional/after validation.
- Schema/format propagation requires pulling from `sessionConfig.outputFormat` and threading into renderXmlNext and final-report validation; refactor carefully.
- Add Phase1 coverage for provider error scenario (xml mode, tools empty, tool_choice auto) to ensure graceful feedback instead of provider 4xx.

## Implied decisions
- `xml` mode remains XML-only for all tools; `xml-final` becomes hybrid (native tools + XML final-report).
- Progress-report stays suppressed in `xml-final` even if progress tool exists.
- Error feedback to the model is mandatory; logging alone is insufficient.

## Testing requirements
- `npm run lint`
- `npm run build`
- `npm run test:phase1`
- Manual repro: `./ai-agent "say hello" "hello" --models nova/default-model --tooling-transport xml-final --trace-llm --verbose` and `./ai-agent neda/github-search.ai "how many open PRs are in netdata/netdata" --models nova/default-model --tooling-transport xml-final --trace-llm --verbose`

## Documentation updates required
- README transport bullet; AI-AGENT-GUIDE XML section.
- specs: tools-xml-transport.md, tools-final-report.md, tools-overview.md, session-lifecycle.md, specs/index.md as needed.
