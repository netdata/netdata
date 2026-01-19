# TODO: Router Handoff Clarification in Internal Tools Instructions

## TL;DR
- Add a router-specific explanation in the **internal tools** section of the injected system prompt.
- Explain that `router__handoff-to` delegates the **original user request** (plus optional message) to another agent **who answers the user directly**.
- Only show this section when router destinations are configured.

---

## Analysis (Current Status)

### 1) Internal tools section is built in InternalToolProvider
- **Evidence**: `src/tools/internal-provider.ts:236-304`
  - `buildInstructions()` creates "### Internal Tools" and lists `agent__task_status` and `agent__batch` instructions.
  - This is the injected system prompt internal tools section.

### 2) Router tool instructions exist but are minimal
- **Evidence**: `src/orchestration/router.ts:120-129`
  - `getInstructions()` returns: “Use router__handoff-to to route to one of: ...”
- **Impact**: It does not explain what handoff means or that the next agent will answer the user.

### 3) Internal tools instructions are computed before router provider is registered
- **Evidence**: `src/ai-agent.ts:780-910`
  - Internal provider is constructed and its instructions are built **before** `RouterToolProvider` is registered.
- **Implication**: Internal-provider cannot detect router tool at runtime unless we pass a flag at construction time (e.g., from `sessionConfig.orchestration?.router`).

---

## Decisions (Made by Costa)

1) **Show router section only when router is configured** (conditional).
2) **Use explicit wording**: original user request + optional message, next agent answers user directly.
3) **No example tool call** (text-only).

---

## Plan
1) Add a `hasRouterHandoff` flag to `InternalToolProviderOptions`.
2) Pass `hasRouterHandoff` from `AIAgent` using `sessionConfig.orchestration?.router?.destinations`.
3) In `buildInstructions()`, insert a router-specific subsection under `### Internal Tools` when enabled.
4) Update docs that describe internal tool instructions and router usage.
5) Add/adjust tests if needed (optional unit test for instruction text).
6) Run `npm run lint` and `npm run build`.

---

## Implied Decisions
- The router section is informational only (no behavior changes).
- The router section belongs under **Internal Tools**, per the user request.

---

## Testing Requirements
- `npm run lint`
- `npm run build`
- (Optional) `npm run test:phase1` if we add a unit test.

---

## Documentation Updates Required
- `docs/skills/ai-agent-guide.md` (Internal Tools section)
- `docs/Technical-Specs-Tool-System.md` (router tool description)
- `docs/Agent-Development-Multi-Agent.md` and/or `docs/Agent-Files-Orchestration.md` if they mention router semantics
