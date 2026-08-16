# Round-2 consolidation (job_d195fd8d…1370, 2026-08-15)

Crew: k3:max, glm-5.3:max, deepseek-v4-pro:max, qwen3.8-max — all returned.
Outputs: `~/.cache/linfra/consult-refunc-round2/<model>.md`.

Verdict: **NOT converged.** The bug inventory (UAF-A/B/C, deadline non-UAF,
small bugs) survived adversarial re-verification intact in all four reviews,
but the round surfaced unanimous material findings against three of the
planned FIXES (D7, UAF-B defer handling, transport lifetime) and the C3
protocol. Plan revised to v3; round 3 required.

## Accepted findings (encoded in plan v3)

- **R2-1 (unanimous, 4/4) — D7 "both paths" is factually wrong.** The
  streaming collector is per stream THREAD (`thread_rrd_collector` __thread,
  started stream-thread.c:522, finished :659), shared by all receivers on the
  thread. A per-receiver `rrd_collector_finished()` in `rrdhost_clear_receiver`
  would kill sibling receivers' availability, drain the shared dispatcher on
  the stream thread, and NULL the thread collector (glm enumerated the
  failure; qwen cites internal_fatal rrdcollector.c:90-91). Rescope: reorder
  is pluginsd-path-only (verified safe by all four against
  pluginsd_parser.c:1520-1540); transport must stand alone for streaming;
  optional extra: swap stream_receiver_cleanup/finished at thread exit
  (shutdown-only window). D7 decision reframed accordingly.
- **R2-2 (unanimous, 4/4) — UAF-B fix as written breaks G3 and leaks.**
  `parser_destroy` never runs/clears a pending defer (pluginsd_parser.h:216;
  oversized-defer abort :197-209 likewise). A stashed acquired item at destroy
  ⇒ `dictionary_destroy` takes the deferred path (dictionary.c:690-702), the
  503 sweep for that entry never fires, waiter hangs/leaks (temp_wb, r), dict
  queued forever (shutdown warning via daemon-shutdown.c:366). Fix is part of
  C6: release the stashed item + free `defer.action_data` (pre-existing STRING
  leak, pluginsd_functions.c:518) in parser_destroy BEFORE
  `pluginsd_inflight_functions_cleanup`. New negative test required.
- **R2-3 (4/4 in variants) — transport lifetime model was underspecified;
  single refcount deadlocks or dangles.** Registry entries outlive the parser
  (both paths — entries persist with running=false / across reconnect), so
  entry refs would block a destroy-time drain forever; draining only
  transients leaves `execute_cb_data` dangling for survivors (deepseek F1,
  qwen F1). Adopt the rrd_collector shape: entry refcount (struct freed at 0,
  possibly long after the parser; released via the rrdfunctions.c:33-37
  cleanup hook) + dispatcher refcount for transients (execute/cancel/progress
  acquire-or-503; parser_destroy marks dead then
  `refcount_acquire_for_deletion_and_wait`-drains it before freez(parser)) +
  alive flag checked after acquire. Conflict-path symmetry: every
  `rrd_function_add` acquires exactly once on insert AND conflict (SWAP at
  rrdfunctions.c:168-175 hands the old ref to cleanup(new_rdcf) at :181).
  Plus glm's registration-pin: canceller/progresser transport refs held by
  GLOBAL inflight records, released in `rrd_functions_inflight_cleanup`
  (rrdfunctions-inflight.c:78-89) — closes the reconnect/re-register late-
  cancel residual UAF glm demonstrated.
- **R2-4 (k3 F4) — cleanup-hook release must be typed.** `execute_cb_data` is
  not always a transport: dyncfg-tree.c:294 passes host, dyncfg.c:386 NULL,
  inline the caller's data. Release gated on the C2 source enum
  (COLLECTOR/STREAMING only); the C2→C6 coupling is now stated in the plan.
- **R2-5 (deepseek major) — C6 is not "fully local": dyncfg entry point.**
  `pluginsd_dyncfg.c:49-50` registers `pluginsd_function_execute_cb` with raw
  `parser` via dyncfg_add_low_level → stored in the DYNCFG entry → invoked at
  dyncfg-intercept.c:534. Same dangling-parser class. Transport is created at
  parser init and threaded through `pluginsd_config`; C6 scope includes
  pluginsd_dyncfg.c. ("File pair" was also really a trio via
  pluginsd_internals.c:62,69 — glm/k3.)
- **R2-6 (unanimous) — C3 flag/set protocol race (lost DEL).** Plain
  flag_set/clear have no test-and-clear; drain-then-clear loses a concurrent
  del (deepseek's interleaving), a regression vs today's synchronous send.
  Mandated protocol: deleter inserts into the set BEFORE setting the flag
  (release order); renderer clears the flag FIRST, then snapshots+clears the
  set under its lock, then emits DELs, then the re-list, one buffer/commit.
  Drain lives INSIDE `stream_sender_send_global_rrdhost_functions` (k3: the
  renderer has multiple callers incl. the reconnect on-ready push,
  command-function.c:16 — drain-at-poll-site-only strands queued DELs across
  reconnects). Set: eagerly created (glm: dyncfg deletes run on non-stream
  threads), internally locked, destroyed with the registry at host free
  (qwen), discarded when gates fail (qwen: else unbounded growth on
  no-FUNCDEL parents), not populated when the host has no parent
  (k3: else slow leak for process lifetime). Behavior note recorded: DEL
  delivery becomes eventual (was synchronous-blocking).
- **R2-7 (deepseek+glm) — C3 layering: gates passed in.** Re-checking
  STREAM_CAP_FUNCTION_DEL inside rrdfunctions-exporters.c would re-import
  streaming headers into database code. The streaming caller resolves the
  gates and passes booleans/args; the renderer stays dependency-free.
- **R2-8 (k3 F6, code-verified by me) — C4 filter spec wrong.** BOTH
  streaming loops check `rrd_function_is_available` (exporters.c:12, :36 —
  verified in-tree) before option filters. All three foreach filters include
  availability; spec fixed. (deepseek/qwen "taxonomy verified" missed that
  the plan text dropped the availability clause — resolved against code.)
- **R2-9 (k3 F5) — must-not-touch lock order was misstated.** The code never
  takes callbacks.spinlock → parser-items. Real nesting: parser-write →
  callbacks.spinlock (registration under execute_cb,
  rrdfunctions-inflight.c:138-150); cancel/progress copy cb+data under the
  spinlock and RELEASE it before invoking (:698-704, :748-754). Constraint
  rewritten: never hold callbacks.spinlock while acquiring parser locks.
- **R2-10 (glm/qwen/k3) — canceller typedef change disclosed.**
  `rrd_function_cancel_cb_t` (rrdfunctions.h:17) gains a transaction param
  (progresser :20 already has one). Sole implementer is pluginsd
  (functions_evloop cancels via polled flag — verified by glm). Must-not-touch
  reworded to "no plugin/wire-visible ABI"; the daemon-internal typedef change
  is a recorded plan item, not a violation.
- **R2-11 (k3 #6) — fourth lifetime bug recorded: UAF-D (rpt send race).**
  Streaming `send_to_plugin` = `send_to_child(rpt)` (stream-receiver.c:517,
  :378); rpt freed in `stream_receiver_remove_internal` right after
  clear_receiver with zero sync against dispatcher holders. Closed by the
  transport iff it gates the SEND paths (insert :54, GC-cancel :137,
  canceller :164, progresser :222), not just dict access — now an explicit
  design constraint and goal.
- **R2-12 (deepseek/qwen) — C5b corrections.** (a) `smaller_monotonic_
  timeout_ut` atomics dropped — every access is under the parser dict write
  lock; if C6 ever moves GC delivery outside locks it needs a CAS loop, not
  relaxed ops (decide then). (b) Guard the all-skipped case (skipped entries
  leave smaller=0 ⇒ per-submission no-op GC + log spam — qwen). (c) State the
  key-format invariant (both sides uuid_unparse_lower_compact,
  pluginsd_functions.c:266 vs rrdfunctions-inflight.c:544). (d) Insert path
  keeps using `rfe->stop_monotonic_ut` directly (glm) — only GC uses the
  accessor. (e) Wording: NOT "all daemon executors synchronous" (dyncfg is
  sync=false) but "no daemon executor stashes the pointer past the
  invocation" (k3; dyncfg-inline.c:17 chains it synchronously). (f)
  FUNCTION_UI_REFERENCE.md:1502 documents the removed pattern — coupled doc
  update (deepseek). (g) `parser->defer` needs an explicit slot for the
  acquired item + transaction key (struct change, C6).
- **R2-13 — bug-record refinements.** UAF-A also reachable WITHOUT plugin
  exit (normal result_end deletes the entry; concurrent cancel holding the
  global record then hits freed t — k3), and the parser dict is NOT fixed-size
  (pluginsd_functions.c:107) so recycled storage can false-match the
  pointer-identity scan ⇒ wrong-transaction FUNCTION_CANCEL (k3). Keyed
  canceller also converts the O(n) identity scan to O(1) (recorded as
  deliberate improvement). UAF-B mechanism corrected (deepseek): the waiter
  frees temp_wb only after data_are_ready; the mid-stream free comes from the
  GC-driven `dictionary_del` firing the delete callback (parser thread's own
  free via signal_when_ready with free_with_signal). UAF-C window exists on
  the pluginsd path too (parser freed :1529 while running true until :1530 —
  k3). Behavior change recorded (deepseek): with the defer holding a ref, a
  GC del mid-stream delivers at result_end release (504 + partial body)
  instead of immediate 504 — an improvement, recorded.
- **R2-14 — completeness adds.** Step 2 also touches rrd_function_add callers
  rrdfunctions-unittest.c:40,45,49,229 (qwen). C4 consumers also
  api_v1_functions.c:15, api_v1_info.c:133 (qwen), and the same borrowed-
  STRING hazard in functions2json/host_functions2json/host_functions_to_dict
  is fixed by the same view struct — recorded so the diff is expected
  (deepseek). D5 is ~16 files, not ~13 (deepseek). Reference-search tokens
  add `execute_cb_data`, `rrd_function_is_available`,
  `rrd_functions_find_by_name` (k3). Nit fixed: exporters comment says INDEX
  lock, not ITEMS (qwen).

## Rejected / noted-only (with reasons)

- **k3's `_and_wait` tinysleep spin note** — real (1ns tinysleep vs 1ms
  sleep) but bounded by the ≤100ms writer persist; recorded as an
  implementation note, not a design change. No plan structure change.
- **deepseek's rrd.h:120 ↔ rrdcollector.h circular-include debt** — adjacent
  debt; folded into the D5 step ONLY if it stays confined to the renamed
  files, else follow-up issue. Not load-bearing for the design.
- **"Deliver result.cb outside container locks"** — retained but narrowed
  (qwen's wire-ordering objection): the FUNCTION-line send on the insert path
  MUST stay inside the critical section (else a concurrent FUNCTION_CANCEL
  can overtake the FUNCTION line on the wire); only format-before-lock and
  result.cb delivery (waiter-side) may move out.
- **glm's "verify base commit" preamble** — the linfra sandbox squashes to a
  synthetic head; line numbers verified to match. No action.

## Cross-model agreement map

- D7 wrong for streaming: 4/4. Defer-destroy hole: 4/4. Transport model
  underspecified: 4/4 (variants converge on the collector two-refcount
  shape). C3 flag/set race: 4/4 (k3 via drain-placement, others via
  interleaving). Dictionary-behind-API for D4: 4/4 — CONFIRMED with stronger
  arguments (dictionary's deferred destruction + delete-cb timing IS the G3
  mechanism; hand-rolled Judy table re-implements the bug class). D4 → (a)
  is now settled evidence-wise; still listed for user sign-off.
- No inherited round-1 claim collapsed. Deadline non-UAF verdict re-confirmed
  4/4.
