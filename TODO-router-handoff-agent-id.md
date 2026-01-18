# TODO: Router handoff-to enum should use toolName (not path)

## TL;DR
- Update `router__handoff-to` tool enum to list destination **toolName** values (not paths).
- Ensure router selection mapping resolves by toolName end-to-end and update docs/tests to match.
- Reuse the exact sub-agent toolName derivation + collision handling (no duplication).
- Keep lint/build green; update docs in the same commit.

## Analysis (current state + evidence)
- Router tool enum and router selection have historically been **path-based** (now being updated to toolName).
- Sub-agent tool names are derived and collisions **fail fast**:
  - `src/agent-loader.ts:514-520` throws on duplicate derived toolName.
  - `src/subagent-registry.ts:55-66` throws on duplicate tool name during registry build.
- ToolName derivation uses `deriveToolNameFromPath` (sanitize + clamp):
  - `src/agent-loader.ts:98-102`.

## Decisions made (Costa)
- **Router destinations must reuse the exact same tool-name derivation & collision handling used for sub-agents** (no rewrites/duplication). This implies we must funnel router destination refs through the same `toolName` resolution path as sub-agent tools (frontmatter `toolName` override + sanitization + collision detection).
- **Option 3 chosen**: expose `toolName` once (computed in loader) and reuse it across router enum + selection matching. "Update both paths" = tool enum generation and router selection resolution.

## Decisions needed (Costa)
1) **Unrelated modified files in git status**
   - Evidence: `TODO-FIX-ADVISORS-ROUTERS-HANDOFF-TOOL-OUTPUT.md`, `neda/support-engineer.ai`, `neda/support.ai` are modified but not touched in this task.
   - Options:
     1. **Ignore for this task** (recommended): leave them as-is and do not include in any commit for this change.
     2. **Include in this task**: we review and fold them into the same change set.
   - Recommendation: Option 1 to keep scope tight.

## Decisions made (Costa)
- **Ignore unrelated neda files** (`neda/support-engineer.ai`, `neda/support.ai`) for this task.

## Plan
1) Compute toolName once in the loader (frontmatter toolName override or derived filename).
2) Store toolName on orchestration refs/runtime agents and use it for router tool enum.
3) Resolve router selection by toolName (not path).
4) Apply the same collision handling to router destinations as sub-agents.
5) Update unit tests + harness + docs to use toolName.
6) Run `npm run lint` and `npm run build`.

## Implied decisions
- Update docs and tests to match the toolName-based router contract.

## Testing requirements
- `npm run lint`
- `npm run build`
- (Optional but recommended) `npm run test:phase1` if unit tests are updated.

## Documentation updates required
- `docs/Agent-Files-Orchestration.md` (router tool example + description)
- `docs/Agent-Development-Multi-Agent.md` (router tool usage)
- `docs/skills/ai-agent-guide.md` (router tool format + examples)
- `docs/specs/frontmatter.md` and `docs/specs/tools-overview.md` if they mention path-based enum
- Any other router tool references surfaced by search
