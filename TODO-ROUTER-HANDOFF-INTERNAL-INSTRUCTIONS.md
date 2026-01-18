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

## Decisions (Pending)

### Decision 1: Show router section only when router is configured?
**Context:** We can add the section conditionally or always.

**Options:**
1) **Conditional (recommended):** Show only when router destinations exist.
   - **Pros:** No noise when router isn’t available; avoids misleading guidance.
   - **Cons:** Slightly more wiring (pass flag to InternalToolProvider).
2) **Always show:** Always include router section.
   - **Pros:** No conditional logic.
   - **Cons:** Misleads models when router tool isn’t available.

**Recommendation:** Option 1.

### Decision 2: How explicit should the wording be?
**Context:** The goal is to stop models thinking handoff means no user answer.

**Options:**
1) **Direct + explicit (recommended):** “`router__handoff-to` hands off the ORIGINAL user request (plus your optional message) to another agent, which will answer the user directly.”
   - **Pros:** Clear, minimal ambiguity.
   - **Cons:** Hard-coded phrasing.
2) **Soft wording:** “Handoff delegates the request to another agent to complete the response.”
   - **Pros:** Shorter.
   - **Cons:** Less explicit about “answers the user directly.”

**Recommendation:** Option 1.

### Decision 3: Include an example tool call?
**Options:**
1) **Yes** (brief JSON example)
   - **Pros:** Clear usage pattern.
   - **Cons:** Adds prompt length.
2) **No** (text-only explanation)
   - **Pros:** Minimal prompt size.
   - **Cons:** Some models still misformat tool calls.

**Recommendation:** Option 1 if you want maximum clarity; otherwise Option 2 to keep prompts short.

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

