# Re-hydration — read this first

You are resuming a `vk-feature` engagement (user skill at
`~/.claude/skills/vk-feature/SKILL.md` — load it) to refactor the Netdata
Agent's function-call mechanism into a first-class component.

- **Code**: worktree `~/repos/nd/refunc`, branch `refunc`, base `master`
  @ 9d4c991fa. No implementation has started — Phase 1 (planning) only.
- **Plan**: `plan.md` (v2) in this repo (linked at `.local/status/plan.md`
  in the worktree). Read it fully. Consult evidence and my accepted/rejected
  judgments: `round1-consolidation.md`. Progress: `progress.md`.
  User decisions so far: `decisions.md` (D1, D3-D7 still OPEN).
- **Phase 1 state**: round 2 complete and consolidated
  (round2-consolidation.md — accepted findings R2-1..R2-14 + rejections);
  **plan v3 written** (UAF-D added, D7 rescoped to pluginsd-only reorder,
  transport two-refcount model, C3 lost-DEL protocol, C4 availability
  filters, C6 scope += pluginsd_dyncfg.c). All 5 consultation rounds complete + consolidated
  (round{1..5}-consolidation.md) — **the loop is CLOSED at the cap; do NOT
  submit further linfra rounds.** Round 5 (delta review of C6): k3 = full
  convergence, zero new findings; glm/deepseek/qwen = four narrow,
  mutually consistent spec amendments, all encoded in **plan v6** (the
  current plan). No unresolved inter-model disagreement exists.
  The clean-context subagent review is DONE: verdict
  **SOUND-WITH-AMENDMENTS**, 3 material findings all accepted and encoded
  in **plan v7** (detach-then-deliver dictionary primitive; leaf lock over
  swapped STRINGs; DYNCFG pin in insert/transfer callbacks only) — see
  progress.md. **Next action: the MANDATORY human approval gate** — the
  gate presentation has been made to the user (goal, step breakdown,
  trade-offs, cap trajectory, subagent verdict, decisions D1, D3-D7).
  **WAIT for the user's explicit decisions — NO implementation, no
  further linfra rounds, no code changes before explicit approval.** When
  approval arrives: record decisions in decisions.md, set plan Status to
  APPROVED, then begin Phase 2 step 0 (baseline) per plan.md's ordered
  steps.
  Convergence = no NEW material finding; then clean-context subagent plan
  review, then the human approval gate (D1, D3-D7).
- After round-2 convergence: clean-context subagent plan review (prompt must
  be self-contained: plan path via `.local/status/plan.md`, verify claims
  against code, adversarial posture, verdict + material findings only), then
  the MANDATORY human approval gate with decisions D1, D3-D7.
- The vk-feature skill supersedes the repo's SOW system for this engagement
  (deliberate user override); AGENTS.md's other rules remain in force.
- Session notes: linfra skills are symlinks into ~/repos/nd/linfra/skills/
  (do not hand-edit); a persistent memory exists about spawning targeted
  linfra consults proactively (they're free — use panels liberally).
