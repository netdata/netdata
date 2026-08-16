# Round-3 consolidation (job_097fb646…5ee3, 2026-08-15)

Crew: k3:max, glm-5.3:max, deepseek-v4-pro:max, qwen3.8-max — all returned.
Outputs: `~/.cache/linfra/consult-refunc-round3/<model>.md`.

Verdict: **NOT converged, but sharply narrowed.** All four models declared the
round-2 redesigns of C3 (protocol + memory ordering), C5b, D7 (rescope), the
destroy-mid-defer release, the two-refcount core (drain ordering, deadlock
freedom), and UAF-D closure VERIFIED against code. Every new material finding
is localized to C6's ownership/lifetime *specification* (who owns
`execute_cb_data`, where refs are acquired/released). Plan revised to v4;
round 4 focuses on C6 only.

## Accepted findings (encoded in plan v4)

- **R3-1 (4/4 — k3 F1, glm F1, deepseek F2, qwen F3): the conflict SWAP must
  swap the ownership tag with `execute_cb_data`.** The registry conflict
  callback (rrdfunctions.c:168-181) swaps only the data pointer; the source
  is a call parameter, not a stored property. Mixed-source INTERNAL↔COLLECTOR
  collisions on localhost are production-reachable (inline names are
  unreserved: sensors, mount-points, block-devices, network-interfaces,
  netdata-streaming, netdata-api-calls, containers-vms, systemd-services…;
  C2 only guards dyncfg names). As specified: COLLECTOR-over-INTERNAL makes
  `rrd_functions_cleanup(new_rdcf)` refcount-release a raw function pointer →
  crash/fatal; the reverse leaks the transport's entry ref forever; the
  delete cb has the same wrong-provenance problem. Fix (encoded): store the
  source (equivalently a data-is-transport bit) in `struct rrd_host_function`,
  SWAP it together with `execute_cb_data`, gate cleanup-hook releases on the
  POST-SWAP (displaced) value's tag. Preserves today's takeover semantics; no
  behavior change. (k3's alternative — reject cross-source re-registration —
  rejected: behavior change with no need once the tag travels.)
- **R3-2 (3/4 — k3 F2, glm F3, qwen F1): the DYNCFG-node transport pin had no
  owner/release protocol — UAF-C would be recreated one level down.** The
  `config <id>` registry entry carries `execute_cb_data = NULL`
  (dyncfg.c:386-398); the plugin's pointer lives ONLY in the DYNCFG node
  (`df->execute_cb_data`, dyncfg.c:233-234, invoked at dyncfg-intercept.c:534
  and the template fan-out :494). Nodes deliberately outlive parser death;
  `dyncfg_conflict_cb` (dyncfg.c:161-166) blindly overwrites the pointer on
  every dyncfg-plugin restart; `dyncfg_cleanup` also runs on the conflict
  loser (:168). With no pin: parser death frees the transport while the node
  still points at it (plugin exit × concurrent config call = UAF; recycled
  slab can even mis-route to the wrong plugin). With an ad-hoc pin: either
  the conflict path dangles or every restart leaks. Fix (encoded): the DYNCFG
  node holds an entry-ref pin — acquired when the transport is installed
  (add and overwrite-install), released on the overwrite branch of
  `dyncfg_conflict_cb` (old value), in the nodes-dict delete path, and in
  `dyncfg_shutdown_low_level`; NO release of `nv` in `dyncfg_cleanup` after
  ownership transfer. dyncfg.c added to C6's touched files; DYNCFG node added
  to the entry-ref enumeration; shutdown ordering vs parser teardown stated.
- **R3-3 (deepseek F1, mechanics verified): `pluginsd_function_result_end`
  must RELEASE the stashed item BEFORE `dictionary_del`.** Del-then-release
  (the rrdfunctions.c:361-367 idiom) on a referenced item marks it deleted
  with the delete callback deferred; the parser inflight dict has no GC
  linkage and traversals skip deleted items, so delivery waits for the next
  submission or parser destroy — a wait-mode caller whose result ARRIVED
  gets 504 on an idle parser. Silent, exactly the kind that survives review.
  Fix (encoded): release→del in result_end (and progress), with a comment
  explaining why the registry idiom is inverted here.
- **R3-4 (glm F2): delivery-outside-container-locks becomes a MUST, and the
  canceller/waiter-mutex delivery context is specified.** Verified ABBA:
  waiter holds `tmp->mutex` and calls the canceller (which takes the parser
  container read lock) while GC holds the container write lock and its
  delete-cb chain blocks on `tmp->mutex` (signal_when_ready) — permanent
  hang, container write lock never released. Pre-existing shape, but v3 left
  delivery "may move" permissive — an implementation keeping in-lock delivery
  keeps the deadlock, and the keyed canceller adds a self-deadlock variant
  (canceller's release being the final free runs the delete cb in its own
  thread while holding the non-recursive waiter mutex). Fix (encoded):
  `garbage_collect` and `destroy_all` MUST collect under the lock and deliver
  after releasing it; the canceller/progresser MUST NOT perform the
  delivering release while holding the waiter mutex (hand the final release
  to a context outside the mutex). Wire-send-in-critical-section (G2) is
  unchanged — this governs waiter delivery, not the FUNCTION line.
- **R3-5 (qwen F2): equal-pointer conflicts must net zero refs.** The SWAP is
  conditional (`!=`, rrdfunctions.c:168); equal-pointer conflicts are ROUTINE
  (a child re-sends its whole global function list on every flag set and
  every reconnect → parent-side conflicts with the same transport per
  connection). A caller-side per-add acquire then leaks one entry ref per
  conflict, unbounded on long-running parents. Fix (encoded): the acquire
  lives INSIDE the registry callbacks — insert cb acquires; conflict cb
  acquires the incoming ref only when it installs a new pointer, and the
  displaced ref is released via cleanup(new_rdcf); equal-pointer conflict
  acquires nothing and releases nothing (net zero). Stated as an explicit
  invariant.
- **R3-6 (k3 F3): the step-6 test oracle was unsatisfiable; resolved by a
  recorded behavior change.** RESULT_BEGIN overwrites the 503 preset with the
  plugin's code (:506) and the delete cb substitutes 503 only when
  `code==503 && body empty` (:96-97), so destroy-mid-defer after a 200 BEGIN
  delivers 200 + truncated body — today and under v3 alike. Decision
  (encoded, recorded behavior fix): the destroy-mid-defer path explicitly
  forces an error code (503) when a defer span is pending, so a truncated
  stream is never reported as success. Test oracle now matches mechanism.
- **R3-7 (deepseek F3): the C5b "always under the write lock" invariant was
  false.** `pluginsd_function_execute_cb` unlocks at :291 (the
  `!sent_successfully` branch) and calls GC at :293 unlocked; GC's
  `smaller_monotonic_timeout_ut = 0` reset (:120) races the locked
  readers/writers. Pre-existing, low impact. Fix (encoded): C6's
  `garbage_collect` runs its container phase under the write lock in ALL
  invocations (making the invariant true); plan text corrected.
- **R3-8 (k3 F4): keyed-canceller insertion-window CANCEL drop — behavior
  note.** `dict_item_find_and_acquire` returns NULL for BEING_CREATED items
  (no retry); today's scan blocked on the write lock and then found the
  entry. Window ≈ insert+send (≤~100ms on a stuck pipe); consequence is only
  a lost plugin-side early-abort (GC/timeout backstop; waiter unaffected;
  G2 wire order verified preserved — send completes before acquirability).
  Below materiality; recorded as a behavior note with optional
  retry-once-after-creation mitigation.
- **R3-9 (deepseek F4): C3 render-time gate widens the del-drop window vs
  today's del-time check — behavior note.** A del queued while the parent was
  ready but drained after a disconnect is discarded (today it would have been
  sent at del time). Masked by the reconnect full re-list (state of record);
  parent's stale entry is unavailable and freed at host free. Recorded as a
  note; the split alternative (discard only on capability-fail, keep on
  readiness-fail) rejected — complexity without a user-visible win.
- **R3-10 — smaller coupled spec details (encoded).**
  (a) `rrd_function_add` NULL-checks `dictionary_set_and_acquire_item`
  (shutdown-only NULL today at rrdfunctions.c:287) and releases the
  entry-ref it took (deepseek).
  (b) The global-record pin is per REGISTRATION (canceller unconditional,
  progresser conditional — pluginsd_functions.c:297-303), stored in
  `struct rrd_function_inflight`, NULL-safe so `rrd_function_run`'s early
  error paths (rrdfunctions-inflight.c:589,605) release a no-op (k3 tail +
  deepseek).
  (c) Drain proof obligation extended (qwen): safety also depends on
  `parser_destroy` running UNDER the receiver lock — command-nodeid.c:42 and
  stream-path.c:246 send to the child under `rrdhost_receiver_lock` and are
  serialized with the drain by that same lock; moving parser_destroy out of
  the lock would turn them into UAF-D-class races. Recorded as a constraint.
  (d) JSON defer users (pluginsd_parser.c:1332-1341) have `action_data=NULL`
  and an OWNED `defer.response`; the function defer's response is an ALIAS —
  one line added so the implementer doesn't "fix" the pre-existing JSON leak
  into a double-free of the alias (k3).
  (e) C5b text: no UUID→compact conversion needed — the parser dict key IS
  the compact string (`pf_dfe.name`) (glm/qwen).
  (f) Step-8 golden output is captured against the post-C3 emitters (which
  already carry the drain), not pre-plan code (deepseek sequencing nit).
- **R3-11 (glm, non-material, tracked):** `stream-sender-execute.c:13-40`
  holds a raw `sender_state*` in result callbacks — host-lifetime object, so
  shutdown-ordering exposure only. Added to the out-of-scope list with the
  existing stream-sender-execute follow-up issue so no one claims send-path
  lifetime completeness.

## Rejected / no action

- k3's "reject cross-source re-registration" alternative for R3-1 — behavior
  change; tag-swap preserves semantics (see R3-1).
- deepseek's C3 discard-split option — rejected under R3-9.
- glm's suggestion to also track the current-code ABBA as a separate issue —
  unnecessary: C6 removes it by construction (R3-4) in the same PR chain.

## Convergence assessment

Round 3 = material findings still arriving ⇒ round 4 required (round 4 of
max 5). Trajectory is converging: rounds 1-2 attacked the whole design;
round 3's material findings are confined to C6's ownership enumeration, and
all four models explicitly closed C3, C5b, D7, destroy-mid-defer, the
two-refcount core, and UAF-D. Round 4 reviews plan v4 with focus on the C6
amendments (R3-1..R3-7) only; if it produces no new material finding, declare
convergence and proceed to the clean-context subagent review.
