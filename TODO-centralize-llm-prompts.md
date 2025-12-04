TL;DR
- Centralize all LLM-facing strings (including XML System Notice blocks) into `src/llm-messages.ts` without changing wording; update callers to import/use them.

Analysis
- Some LLM strings live in `src/xml-tools.ts` (renderXmlNext/renderXmlPast system notices and XML examples).
- `finalReportXmlInstructions` already in `llm-messages.ts`, but XML System Notice templates are not.
- Multiple files currently modified; must avoid changing their existing content beyond relocating strings.

Decisions
- Approved: move existing XML System Notice / tool list / past responses strings verbatim into `llm-messages.ts` and reference them from `xml-tools.ts`.

Plan
- Extract current strings from `xml-tools.ts` into new exported constants/functions in `llm-messages.ts`.
- Update `xml-tools.ts` to import those constants/functions and remove inline string assembly logic while preserving exact output.
- Run lint/build if required.

Implied decisions
- No wording changes; pure relocation.

Testing requirements
- Likely `npm run lint` and `npm run build` after refactor.

Documentation updates required
- None if wording unchanged; otherwise update docs if any behavior surface changes (not planned).
