# TODO: XML-NEXT Router Handoff Messaging on Final Turn

## TL;DR
- XML-NEXT final-turn messaging currently says “you must provide final report,” even when `router__handoff-to` is allowed.
- Update XML-NEXT to explicitly allow **either** final report **or** `router__handoff-to` when router destinations exist.
- Keep docs/tests aligned with the new router-aware final-turn message.

---

## Analysis (Current Status)

### 1) XML-NEXT final-turn warnings always require final report
- **Evidence**: `src/llm-messages-xml-next.ts:25-55`
  - `XML_NEXT_SLUGS.final_turn_*` messages say “You MUST now provide your final report/answer.”
- **Impact**: On final turns, the warning implies **only** final report is allowed, even when router handoff is permitted.

### 2) XML-NEXT final-turn footer only lists allowed tools (no choice text)
- **Evidence**: `src/llm-messages-xml-next.ts:232-242`
  - Footer always says “You must now provide your final report/answer…” and then adds “Allowed tools for this final turn: …”.
- **Impact**: The “Allowed tools” line is not framed as a valid alternative, so the model interprets the final report as mandatory.

### 3) Router tool **is** allowed on final turns, but messaging ignores it
- **Evidence**: `src/ai-agent.ts:1724`
  - `finalTurnAllowedTools` is set to the router tool name when router is configured.
- **Evidence**: `src/session-turn-runner.ts:584-597`
  - `finalTurnTools` is passed into `buildXmlNextNotice` for final turn messaging.
- **Evidence**: `src/orchestration/router.ts:11-15`
  - Router tool name is `router__handoff-to`.
- **Impact**: The capability exists, but the XML-NEXT text does not communicate the choice.

---

## Decisions (Made by Costa)

1) **Update both warnings and footer** (router-aware).
2) **Use explicit tool name** in messaging: `router__handoff-to`.

---

## Plan
1) Add router-aware context to XML-NEXT builder (e.g., detect `router__handoff-to` in `finalTurnTools`).
2) Update final-turn warning/footer text to allow **final report OR router handoff** when router tool is allowed.
3) Update tests that assert final-turn XML-NEXT text (phase2 harness + any unit tests referencing `XML_NEXT_SLUGS`).
4) Update docs that show XML-NEXT final-turn examples.
5) Run lint/build and required tests.

---

## Implied Decisions
- Router option is shown **only** when `router__handoff-to` is allowed on final turns.
- No change to tool filtering logic; only messaging changes.

---

## Testing Requirements
- `npm run test:phase1` (if unit tests touched)
- `npm run test:phase2` (deterministic harness; XML-NEXT checks live here)
- `npm run lint`
- `npm run build`

---

## Documentation Updates Required
- `docs/specs/retry-strategy.md` (final-turn XML-NEXT examples)
- `docs/specs/CONTRACT.md` (forced-final guidance; mention allowed router handoff)
- `docs/specs/tools-xml-transport.md` (XML-NEXT guidance text)
- `docs/specs/AI-AGENT-INTERNAL-API.md` (final-turn tools note may need clarification)
