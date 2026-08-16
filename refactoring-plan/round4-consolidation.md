# Round-4 consolidation (job_0ed8f6dd…be369, 2026-08-15)

Crew: k3:max, glm-5.3:max, deepseek-v4-pro:max, qwen3.8-max — all returned.
Outputs: `~/.cache/linfra/consult-refunc-round4/<model>.md`.

Verdict: **NOT converged, but all findings are C6 spec-text amendments with
agreed fixes; no closed item re-opened.** glm explicitly judged "no further
full round warranted — a delta-check of the amended text suffices." Round 5
(the cap) runs as a tight delta review of plan v5's C6 text.

Two cross-model factual disputes were resolved by me against the code:

- **Fact A (glm/k3 RIGHT, deepseek wrong):** `dict_item_del` unlinks from the
  hashtable BEFORE `dict_item_free_or_mark_deleted`
  (dictionary-item.h:399-427, verified). So in the GC-del-first interleaving
  a later `dictionary_del` from result_end is a hashtable miss, and an
  external release only marks pending — delivery defers to the next
  GC/insert/destroy. deepseek's "result_end's del finds the item still in the
  index" is rejected.
- **Fact B (deepseek RIGHT, k3's round-3 note wrong):** the find path's
  `item_check_and_acquire_advanced` checks only `refcount < 0` and
  `ITEM_FLAG_DELETED` — NOT `BEING_CREATED` (dictionary-refcount.h:156-235,
  verified; the BEING_CREATED rejection at :264 is in the DELETE-candidate
  path). The insert callback (the FUNCTION send) runs under the index WRITE
  lock (dictionary-item.h:459-488), so a keyed cancel blocks on the index
  lock and then finds the item — same block-then-send as today. Round 3's
  R3-8 behavior note ("keyed canceller drops CANCEL in the insertion
  window") is FALSE and is removed from the plan; no retry mitigation needed.

## Accepted findings (encoded in plan v5)

- **R4-1 (k3 F1 + deepseek F1 + qwen R4-1; glm N1 adjacent) — equal-pointer
  conflict underflow.** v4 paired a conditional acquire ("only when
  installing a DIFFERENT pointer") with an UNCONDITIONAL release
  (`rrd_functions_cleanup(new_rdcf)` at rrdfunctions.c:181 runs on every
  conflict; the dictionary fires the conflict cb on every re-set even for
  identical values, dictionary-item.h:505-507). Equal-pointer re-sends are
  routine (child re-lists on every flag set) ⇒ net −1 per re-registration ⇒
  transport freed while the entry still points at it (UAF) or refcount
  fatal. Fix (encoded — the collector pattern, the literal precedent in the
  SAME callback, rrdfunctions.c:60-69 + rrd_collector_release(NULL) no-op):
  tag+data swap as ONE pair (glm N1); when NO swap occurs the conflict cb
  NEUTRALIZES `new_rdcf` (data=NULL, non-transport tag) so the unconditional
  cleanup release is a no-op; acquire only on actual install AND gated on
  the stored tag being transport-bearing (qwen's second aspect — "different
  pointer" alone would acquire on caller-owned INTERNAL data in mixed-source
  installs). k3's alternative (unconditional acquire, STRING pattern) is
  equivalent-correct but rejected in favor of the in-callback precedent.
- **R4-2 (glm F1 + k3 F3; Fact A) — result_end protocol gains a sweep.**
  With plain release→del, a GC del landing mid-defer defers delivery (and
  the transport pin, temp_wb, global record) until the next submission on
  that parser — a wait-mode caller whose result ARRIVED burns its full
  deadline and 504s; the v4 behavior note claiming "delivers at the
  result_end release" was false. Fix (encoded): result_end = **release →
  del → sweep** (`garbage_collect_pending_deletes` early-returns when
  nothing is pending, so the happy path pays nothing); behavior note
  corrected to "delivery at result_end in ALL interleavings".
  `pluginsd_function_progress` is release-only — no gap.
- **R4-3 (glm F2 + qwen R4-2 + k3 focus-2 + deepseek gap) — DYNCFG pin
  discriminator + release-site precision.** `df->execute_cb_data` is NOT
  always a transport (NULL for health/inline/registry-intercept adds; a raw
  `t2` struct in dyncfg-unittest — which is IN the plan's own validation
  set and would fatal on an ungated release). Fix (encoded): a dedicated
  `transport` field on the DYNCFG node (pin exists iff set; decouples the
  pin from the cb-data overload); pin acquired at INSTALL only (add path +
  the conflict transfer branch INCLUDING the `!v->execute_cb` rescue arm,
  where the displaced value is provably NULL), never carried by the
  incoming tmp/nv; released ONLY in `dyncfg_delete_cb` — NOT in
  `dyncfg_cleanup` (which also serves conflict losers, dyncfg.c:168, and
  file-load error paths); `dyncfg_shutdown_low_level` needs NO extra
  release (`dictionary_destroy` fires the delete cb per node — an explicit
  site would double-release, qwen).
- **R4-4 (k3 F4 + glm N2 + qwen R4-3) — delivery-MUST mechanism
  obligations.** Encoded: victims are dup'd under the traversal
  (`dictionary_acquired_item_dup` — lock-free CAS); after unlocking,
  per-victim **release-dup-then-del** (the zero-ref del path is the only
  one that fires the delete cb lock-free; external releases NEVER fire
  callbacks — the invariant that also makes the canceller final-release
  discipline trivially satisfiable); `destroy_all` makes the container
  reject inserts BEFORE snapshotting victims (destroyed-flag gating exists,
  dictionary-item.h:436-442) and runs after the transport mark-dead, so a
  late execute_cb insert gets NULL → the existing 503 branch answers;
  canceller/progresser never RUN A SWEEP while holding the waiter mutex.
- **R4-5 (k3 F2) — entry-point read-then-acquire residual race closed by a
  lookup-time pin.** An executor reads `r->rdcf->execute_cb_data` holding
  an item ref but NO lock; a concurrent conflict swap can release the last
  entry ref and free the transport between the read and the dispatcher CAS
  (~2-instruction window; the same shape pre-exists for `rdcf->collector`).
  Given the LONG-TERM BEST directive and the plan's "UAF-C closed" claim:
  encoded — C1's `RRD_FUNCTION_ACQUIRED` carries a transport entry-pin
  acquired inside the registry find UNDER the index rdlock (mutually
  excluded with the conflict cb's wrlock — race-free), released wherever
  `host_function_acquired` is released today (rrdfunctions-inflight.c:97,
  :590,:606; verify_access paths); same helper on the dyncfg node lookup.
  The canceller/progresser were already safe (global-record pin).
- **R4-6 (deepseek F2; Fact B) — round-3 canceller behavior note removed**
  (see Fact B above).
- **R4-7 (k3 F5) — destroy-mid-defer wording fixed.** Destroy frees
  `defer.action_data` always (owned STRING or NULL — only two defer
  families exist); frees `defer.response` only when OWNED (JSON family),
  never the function family's alias; the path branches on defer type (only
  the function family carries a stashed item). Contradictory parenthetical
  dropped.
- **R4-8 (glm minor) — forced-503 narrowed to success codes.** Force 503
  only when the recorded code indicates success (2xx); a truncated 404
  keeps the plugin's 404. Matches the rationale exactly ("a truncated
  stream must not report SUCCESS").
- **R4-9 (deepseek precision + glm restate) — pin arithmetic and transport
  dtor constraints.** An async record can hold TWO pins (canceller
  unconditional + progresser conditional) — `rrd_functions_inflight_cleanup`
  releases BOTH, no dedup. The transport destructor is parser-independent
  (pin holders outlive parser death; no `PARSER*` deref after death). The
  struct frees only at entry-count 0 AND dispatcher counter drained (the
  collector shape restated).
- **R4-10 (k3 + glm, notes).** Template fan-out jobs are not re-pointed on
  plugin restart → they keep the dead transport → acquire-or-503 turns
  today's UAF into a clean 503 (strictly better; pre-existing functional
  gap; recorded). The dup-conflict `result.cb` delivery inside
  `register_and_send`'s critical section is safe (the second caller's
  waiter mutex is not yet taken) — implementation comment.

## Rejected

- deepseek's account of the GC-del interleaving (Fact A) and k3's round-3
  BEING_CREATED note (Fact B) — both resolved against the code.
- k3's Finding-2 "accepted risk" alternative — rejected; the lookup pin is
  contained and the plan claims closure (R4-5).
- qwen's Option B / k3's unconditional-acquire variant for R4-1 — correct
  but the collector pattern is the in-callback precedent (see R4-1).

## Convergence assessment

Round 4 = material findings still arriving ⇒ round 5 (the cap) runs as a
**delta review of plan v5's amended C6 text only**, per glm's explicit
recommendation. If round 5 surfaces no new material finding: convergence →
clean-context subagent review → human approval gate. If it does: stop
looping (cap reached) and escalate the unresolved point(s) to the user with
a recommendation, per the vk-feature convergence rules.
