# TODO - Code Review Agents Assessment

## TL;DR
- Read docs/AI-AGENT-GUIDE.md plus the other required design docs and audit the code-review/**.ai agents for alignment.
- Produce a review noting gaps between the guide's operational requirements and the current prompts (format placeholders, turn budgeting, tool usage, etc.).

## Analysis
- docs/AI-AGENT-GUIDE.md demands that every prompt mention `${FORMAT}` and `${MAX_TURNS}` (Section 3 "Best practices"), and that internal tools like `agent__batch` follow strict schemas.
- code-review/code-review.ai lacks any `${FORMAT}` mention even though the orchestrator emits JSON; discovery + specialist prompts do include `${FORMAT}` but none reference `${MAX_TURNS}` anywhere in code-review/*.ai despite Guide's explicit directive.
- README promises "specialists reuse discovery data (no redundant file reads)", yet every specialist (`code-review-*.ai`) still registers filesystem and GitHub tools, allowing them to re-read the repo instead of relying on discovery output.
- Discovery output schema (code-review/schemas/discovery-output.json) defines required fields the orchestrator depends on; need to verify prompts enforce those requirements and reject missing data (current instructions describe validation but no fallback beyond final failure message).
- Final report schema requires deduped issues with severity/category metadata; orchestrator instructions explain deduplication but there is no explicit guardrail ensuring schema-compliant summaries when derived insights trigger iterations.

## Decisions Needed
- Confirm whether specialists should lose direct MCP tools (as README implies) or if we merely need to instruct them not to use those tools.
- Decide if we should bake `${MAX_TURNS}` reminder into every prompt (likely yes per Guide) or if some agents are exempt.
- Clarify if orchestrator should enforce discovery schema validation beyond "fail final" (e.g., should it re-try discovery, or is one failure acceptable?).

## Plan
1. Inventory Guide requirements relevant to prompts (placeholders, reasoning, batch usage, tool access) and capture citations for the review write-up.
2. Audit each code-review/*.ai file for compliance (format placeholders, mention of `${MAX_TURNS}`, tool sections, iteration instructions, schema coverage).
3. Compare README claims vs. actual capabilities (e.g., specialists having filesystem tools) and note contradictions.
4. Draft the review for Costa highlighting concrete issues with file references plus recommended fixes, citing the Guide/README sections.
5. Agree on remediation scope (prompt edits, schema updates, doc sync) once Costa responds.

## Implied Decisions
- Any prompt edits to add `${FORMAT}`/`${MAX_TURNS}` will likely require coordinating schema references in code-review/schemas/* and README examples.
- Removing MCP tools from specialists might necessitate introducing richer discovery output (snippets, metadata) so they can operate without filesystem access.

## Testing Requirements
- No automated tests for this documentation review; if prompts change later we must re-run lint/build plus targeted prompt/unit tests.

## Documentation Updates Required
- If we alter agent prompts or specialist capabilities, update code-review/README.md (architecture + usage), docs/AI-AGENT-GUIDE.md (if new behaviors emerge), and any schema references.
