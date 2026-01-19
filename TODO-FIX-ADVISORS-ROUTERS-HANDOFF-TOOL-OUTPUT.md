# TODO: Fix Advisors, Routers, Handoff, Tool-Output Sub-Sessions (Restart)

## TL;DR

- Keep semantics unchanged: **turn = LLM turn**, **session = one user request**, orchestration lives inside the session.
- Add **Steps** as the canonical timeline for orchestration (advisors/router/handoff) and tool_output visibility.
- **Defer User Turns** until requirements are clear.

---

## Scope & Goals

- Sub-sessions (advisors/router/handoff/tool_output) must be visible in opTree/snapshots with correct ordering.
- opTree must show a truthful timeline using **Steps** without changing LLM turn semantics.
- tool_output must behave like subagents (opTree/logs/progress), while **full-chunked** captures only LLM req/resp.
- Clarify tool_output full‑chunked placement: child session is attached to the tool op (turn); steps live **inside** that child session.

---

## Analysis (Current Codebase - Remaining Items)

- None. Docs/tests aligned with tool_output full-chunked nesting; orchestration checks updated to steps. Doc sweep found no additional stale orchestration kind references.

---

## Decisions (Made by Costa)

1. **Restart implementation** and re-do the work with a clean, correct approach.
2. **Turn semantics unchanged**: `turn = LLM turn` (internal attempts/cycles).
3. **Session semantics unchanged**: one user request per session with multiple turns and orchestration.
4. **User Turns** concept defined (aligned with **headend turns**); implementation deferred.
5. **Steps are canonical timeline** for orchestration and sub-session visibility.
6. tool_output must behave exactly like subagents (opTree, logs, task-status/progress).
7. Advisors must follow the same rules as subagents (opTree, logs, progress).
8. tool_output full‑chunked: keep internal SessionTreeBuilder; **only** capture LLM req/resp (no progress/task_status).
9. Explicit orchestration kinds: **advisors**, **router_handoff**, **handoff**.
10. Step path format: `S{step}-{op}` (e.g., `S3-2`).
11. Step kinds: `system`, `user`, `advisors`, `router_handoff`, `handoff`, `internal`.
12. Snapshot version: bump to `2` for the new schema.
13. **Do not implement multi-session conversation modeling now**; defer until requirements are clearer.
14. **Defer User Turns** implementation until future requirements are clear.
15. **Decision 1A**: keep current code; update docs/tests to match tool_output full‑chunked nesting under tool op child session.
16. **Decision 2A**: update phase2 harness expectations to use steps and new step kinds.
17. **Doc gaps fixed**: `docs/specs/MULTI-AGENT.md`, `docs/Operations-Snapshots.md`, and snapshots skill docs updated to reflect step nesting and orchestration kinds.
18. **Doc sweep**: scan all docs for old orchestration kind references and update to the current opTree kinds/locations.

---

## Decisions (Pending)

None.

---

## Plan

- None. All planned work completed; pending user verification.

---

## Implied Decisions

- tool_output remains a `session` op (not a `tool` op), like subagents.
- tool_output full‑chunked steps are nested inside the tool op child session (not top‑level steps).
- Avoid double-counting child accounting in parent arrays.
- LLM turns remain in opTree as-is; Steps provide orchestration timeline.

---

## Testing Requirements

- Validate Steps presence and ordering for advisors/router/handoff/tool_output.
- Ensure LLM turns remain intact and unchanged.
- Update Phase 1/2 harness scenarios to align with new opTree schema.
- Run `npm run lint` and `npm run build`.

---

## Documentation Updates Required

- `docs/specs/optree.md` and `docs/specs/snapshots.md` for Steps.
- `docs/skills/ai-agent-session-snapshots.md` for new jq paths.
- `docs/Operations-*.md` references to opTree turns/steps.

---
