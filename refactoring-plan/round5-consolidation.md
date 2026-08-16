# Round-5 consolidation (job_c26fbd7e…dfef, 2026-08-16) — FINAL ROUND (cap)

Crew: k3:max, glm-5.3:max, deepseek-v4-pro:max, qwen3.8-max — all returned.
Outputs: `~/.cache/linfra/consult-refunc-round5/<model>.md`.
Scope: DELTA review of plan v5's five C6 amendments (A conflict accounting,
B result_end sweep, C DYNCFG pin, D lookup pin, E delivery obligations).

## Verdict

- **k3: "no new material findings — round 5 converges"** — full walk of all
  five deltas including a complete conflict-combination table, the sweep
  lock-graph, and a complete release-site enumeration; three non-material
  precision notes.
- **glm, deepseek, qwen: design holds; each found 1-2 narrow spec-precision
  amendments** with explicit one-paragraph fixes; all three state the
  findings do NOT overturn the design.
- No inter-model disagreement exists in this round: the three finding sets
  are disjoint-or-consistent, and each model's "everything else" section
  independently verifies the other models' subjects clean (e.g. qwen and k3
  verified delta B's lock context acceptable; glm/deepseek verified E's
  exactly-once).

Round cap (5) reached ⇒ per the vk-feature rules the loop STOPS here. The
four accepted amendments below are mechanical, mutually consistent, and
carry agreed fixes — encoded as plan v6 without a further round. The cap
outcome, the trajectory (18 material findings r2 → 5 r3 → C6-text-only r4 →
4 narrow amendments r5, with 1/4 reviewers at full convergence), and these
final amendments are presented to the user at the approval gate.

## Accepted findings (encoded in plan v6)

- **R5-1 (deepseek material + glm F1, same subject; also resolves qwen E2
  and k3 note 2): result_end must not use the stock sweep, and the MUST's
  true enabling invariant must be recorded.** The stock
  `garbage_collect_pending_deletes` fires delete callbacks under the dict's
  items (linked-list) WRITE lock (dictionary.c:172→196→215), so v5's
  result_end sweep delivered `result.cb` under the container lock — the
  letter of the plan's own MUST — and v5's recorded justification ("result_
  end holds no waiter mutex") argued about the wrong thread (the hazard is a
  CONCURRENT waiter). glm additionally proved the shape is deadlock-free in
  v5's FINAL state only because the keyed canceller/progresser touch only
  the index lock and execute_cb precedes wait-mutex acquisition — an
  unstated, load-bearing constraint. Fix (encoded): a collect-then-deliver
  sweep variant (dup pending victims under the lock, deliver after unlock —
  the exact shape the MUST already mandates for GC/destroy_all) is used at
  result_end AND as GC's trailing drain; the enabling invariant is recorded
  as a hard constraint ("no thread may hold a waiter mutex while acquiring
  this dict's items lock; the keyed canceller/progresser index-lock-only
  discipline is load-bearing — a future dfe-style cancel path re-arms a
  waiter⇄parser deadlock"); destroy_all's honest residue is recorded (a
  straggler that goes pending after the snapshot is delivered by
  `dictionary_destroy` under the lock — bounded, and post-drain no
  waiter-mutex counter-party exists). With the trailing drain, the
  "delivers at result_end in ALL interleavings" behavior note becomes
  literally true (qwen E2).
- **R5-2 (glm F2 material + qwen D-MUST + k3 note 1): the lookup pin gets a
  named, implementable mechanism and a capture-at-find rule.** v5's "pin
  UNDER the index rdlock" is unimplementable as written (no dictionary
  lookup hook exists), and the naive registry-rwlock-across-find fallback
  ABBAs against the conflict cb (find{registry→index} vs
  conflict{index→registry}). Fix (encoded): a dedicated LEAF spinlock
  guarding the (tag, data) pair — the conflict cb takes it (inside the
  index wrlock) around the pair swap + displaced release; the find takes it
  standalone AFTER the standard item acquire (the item ref already pins the
  entry memory) to read the pair and entry-pin the transport. Leaf = no
  ordering cycle. Capture-at-find mandate (glm F2b ≡ qwen's MUST):
  `RRD_FUNCTION_ACQUIRED` captures (execute_cb, transport, pin) at find;
  executors NEVER re-read `rdcf->execute_cb_data` at call time
  (rrdfunctions-inflight.c:252,:306,:650) — a stale capture degrades to a
  clean 503 via acquire-or-fail. Pin attaches only to the item the find
  RETURNS (k3: not to prefix-retry intermediates released at
  rrdfunctions.c:432). Transport destructor: parser-independent AND takes
  no locks (glm).
- **R5-3 (qwen E1 material): the per-victim del must not use the item-owned
  key after the last reference drops.** In release-dup-then-del, a
  concurrent insert-triggered sweep (item_linked_list_add →
  garbage_collect_pending_deletes, dictionary-item.h:275) can free the item
  between GC's release-dup and its `dictionary_del(dict, name)` → UAF read
  of the freed key (dictionary-item.h:390-391; the key is freed with the
  item at :243-244). Fix (encoded): copy each victim's key (strdupz) during
  the locked traversal; del by the copy; free the copies after.
- **R5-4 (qwen C-residual): the dyncfg template fan-out's transport copy is
  pinned.** The fan-out copies `df_template->execute_cb_data` lock-free
  (dyncfg-intercept.c:138-139); a concurrent conflict-transfer can release
  the displaced pin in that window (first-job-add case → dead pointer).
  Fix (encoded): the fan-out reads + entry-pins the template's transport
  via the same leaf-lock helper as R5-2 before copying into the job node.
- **R5-5 (k3 note 3 + citation nits, cosmetic):** refcount.h path is
  src/libnetdata/atomics/; the destroyed-gate's load-bearing check is
  dictionary-item.h:463-466; the never-fires-callbacks invariant spans
  dictionary-refcount.h:92-150. Corrected in passing.

## Explicitly re-verified clean this round (by multiple models)

Delta A's full combination table (k3 + qwen tables agree cell-for-cell;
deepseek/glm walked it independently): every INTERNAL/COLLECTOR/STREAMING ×
equal/different-pointer case nets zero; neutralization is load-bearing;
equal-pointer-different-tag is unreachable. Delta C: no double-release or
leak on any enumerated path (all four). Delta D's release-site enumeration:
complete (k3 + qwen independent greps). Delta E: exactly-once in every
constructed interleaving (all four). The v4→v5 fixes themselves (Fact A/B
resolutions, collector pattern, forced-503-2xx) drew zero objections.

## Convergence assessment (final)

The consultation loop ends at the cap. Assessment for the approval gate:
the design is stable and 4/4-verified at the architecture level since round
3; rounds 4-5 were exclusively C6 ownership-spec text; round 5 produced one
full-convergence verdict and four narrow, agreed, now-encoded amendments.
There is NO unresolved inter-model disagreement to escalate — the cap
escalation to the user consists of presenting this trajectory and plan v6
at the (already mandatory) human approval gate, after the clean-context
subagent review.
