# Decisions

## Open (awaiting user)


## Superseded

- **D2** (include broker deadline refactor now vs defer) — absorbed into plan
  v2 as C5b (step 7), shape fixed by the deadline-audit consult verdict
  (broker-keyed accessor; see round1-consolidation.md). No longer a standalone
  fork; final acceptance rides on overall plan approval.

## Taken

- 2026-08-16 — **D7 = (a) transport handle + the pluginsd-path teardown
  reorder** (`rrd_collector_finished()` before `pluginsd_process_cleanup`,
  pluginsd_parser.c:1529-1530, lands step 5). Transport remains the required
  and sufficient mechanism (sole mechanism on streaming, where no
  per-receiver reorder exists); the reorder is verified-safe defense-in-depth
  on the plugin path (4 models × 2 rounds).
- 2026-08-16 — **D6 = (a) LEAVE the lazy-GC wart, document + GitHub issue**
  (pre-existing: a wait-mode timeout never sends FUNCTION_CANCEL until the
  next submission on that parser; GC's only two triggers are in execute_cb).
  Rationale: both fixes change plugin-visible behavior, excluded from this
  plan's frozen-behavior contract. Issue to be opened at engagement close
  per Followup Discipline (bundled with the pre-existing GC-write interleave
  note and the inline-sync-on-stream-thread scheduling fork, per plan.md
  out-of-scope).
- 2026-08-16 — **D5 = (a) RENAME `rrd_collector` → `rrd_function_provider`**
  (files fold into rrdfunctions-provider.*; ~16 files; mechanical; own
  commit/PR, LAST in the chain). Guardrails recorded: the thread-exit
  cleanup registration (rrd.c:81) survives the rename; the rrd.h↔
  rrdcollector.h circular include is folded in only if confined to the
  renamed files. Deeper provider/collector concept split stays rejected.
- 2026-08-16 — **D4 = (a) DICTIONARY behind the transport API** (4/4 consult
  consensus: per-item refcounts + delete-cb-at-final-free timing ARE the
  UAF-B fix; deferred destruction covers plugin-death-mid-stream; the 503
  sweep is the delete-cb mechanism). Includes the two small generic
  libnetdata extensions the reviews proved necessary: the
  detach-pending-then-deliver primitive and the leaf spinlock over swapped
  entry fields. Spinlock+Judy alternative rejected — would re-implement the
  exact machinery whose misuse caused the bugs.
- 2026-08-16 — **D3 = (a) one PR per step (~10 PRs)**, each independently
  buildable/tested, later steps rebased on merged predecessors. Accepted
  cost: ~10 review/merge cycles. Noted: nightlies between steps 1 and 5
  carry the refactored registry with today's known UAFs still present (no
  worse than today; fixes land steps 5-7).
- 2026-08-16 — **D1 = (b) DELETE the systemd-journal→logs tags shim**
  (rrdfunctions.c:244). Decision informed by git archaeology: the shim, the
  tags wire field, and the plugin's explicit "logs" tag were all introduced
  by the SAME commit (da32dd8be, #16574, first release v1.45.0, 2024-03), so
  the shim's only beneficiaries are pre-v1.45.0 child agents streaming to
  modern parents. Accepted regression (cosmetic, recorded): such children's
  systemd-journal function groups under the generic "top" instead of "Logs"
  in the parent UI. Registry keeps only the generic "top" default for
  tagless arrivals.

- 2026-08-15 — User: apply vk-feature process to the function-mechanism
  refactor; planning phase started in worktree `~/repos/nd/refunc`.
- 2026-08-15 — User scope directive: **long-term best**. Any refactoring that
  serves the main function-mechanism goal is in scope regardless of churn.
  Consequence: re-evaluate the plan's "out of scope" list in the round-2
  revision — the pluginsd transport adapter structure (protocol I/O inside
  dictionary callbacks) and rrd_collector lifecycle internals are promoted to
  candidate scope; MAX_FUNCTION_LENGTH (wire contract) and the inline-sync-on-
  stream-thread scheduling policy remain excluded as protocol/behavior
  decisions, not structure.
- 2026-08-15 — User: linfra is effectively free; spawn targeted consultations
  proactively (also saved as a persistent memory).
