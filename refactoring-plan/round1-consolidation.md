# Round 1 consolidation (plan review, job_071f…2453)

Panel: deepseek-v4-pro_max, glm-5.2-max, mimo-v2.5-pro, minimax-m3-coder.
All four: direction sound, evidence base accurate, design idiomatic — with
material corrections. Every finding below was verified against the code by me
before acceptance.

## Accepted (verified) — plan v2 must incorporate

1. **T2 consumer list wrong** (3/4 models; verified): MCP passes `NULL` for
   `out_acquired` (`mcp-tools-execute-function.c:2638`). Real consumers of the
   acquired item: `rrd_function_run` internally + `rrdfunctions-unittest.c:94-136`.
2. **Exporter filters are NOT uniform** (verified): 7 loops; the 5 JSON/dict
   loops filter `DYNCFG|RESTRICTED`; chart-streaming filters `DYNCFG` only
   (`rrdfunctions-exporters.c:13`); global-streaming filters `LOCAL` +
   `DYNCFG`-with-count (`:37-42`); **RESTRICTED functions DO stream**. T5 needs
   three filters (EXPORTABLE, STREAMABLE_CHART, STREAMABLE_GLOBAL) and the
   view struct needs `timeout`.
3. **Availability asymmetry** (verified): `chart_functions_to_dict` passes
   `host=NULL` to `rrd_function_is_available` (`:136`) — skips the state-id
   check. Must be preserved via a variant, or the v1 chart path changes
   behavior after host re-association.
4. **Byte-copy discipline** (verified): `function_v2_entry`
   (`api_v2_contexts.c:24,694`) keeps borrowed `STRING *help/tags` after
   iteration; the manifest comment (`rrdfunctions-exporters.c:200-204`)
   documents why that is unsafe under the conflict callback. T5's view struct
   uses byte copies; the api_v2_contexts consumer changes; this is a latent
   bug fix, record it as such.
5. **Unittests are coupled work, not validation tail** (verified):
   `rrdfunctions-unittest.c:136,143-145,271` (acquire/release + del bools),
   `mcp-tools-execute-function-unittest.c:108,187`, `dyncfg-unittest.c:802`
   (inflight init). Migrate in the steps that break them. Also add
   `rrdfunctions_manifest_pacer_unittest` (`main.c:513`) to the validation list.
6. **T3 blast radius** (verified): `dyncfg.c:386,427`, `dyncfg-tree.c:294`
   (INTERNAL), `rrdfunctions-inline.c:35` (absorbs the source so its ~10
   callers don't churn), `pluginsd_functions.c:415,448` (COLLECTOR/STREAMING).
   Truth table to encode: COLLECTOR+dyncfg-name=reject;
   STREAMING+dyncfg-name=accept (child "config" proxy);
   INTERNAL+dyncfg-name=accept (localhost dyncfg).
7. **T4 constraints** (verified): dispatch synchronous, on the mutating
   thread, after the dict operation, outside dict locks; NEVER via the ACLK
   event loop (`sqlite_aclk.c:1398-1415` forbids it). Event semantics: add
   carries post-insert options, arms ACLK iff `!(options&DYNCFG)` (RESTRICTED
   add still arms); delete carries pre-delete options, arms iff
   `!(options&(DYNCFG|RESTRICTED))`; the streaming flag is set iff the call is
   host-scoped (`st==NULL`) — **even for dyncfg deletes** (`rrdfunctions.c:372`)
   — while FUNCTION_DEL additionally requires `!is_dyncfg` (`:369`); the name
   is the sanitized key. Do not re-derive global-ness from stored options.
   Design: **fixed two-callback registration** (streaming, ACLK), not a
   generic observer list (2/4 models pushed this; accepted — no repo precedent
   for observer lists, exactly two subscribers exist).
   Exclusions to record: availability-driven arms stay direct calls
   (`pluginsd_parser.c:1532-1536`, `stream-receiver.c:1352`, `rrdhost.c:925`);
   flag consumer named: `command-begin-set-end-init.c:48`.
8. **Step 1 not self-contained as drafted** (verified): exporters read
   `host->functions` at 7 sites; fold their mechanical pointer migration into
   step 1 (internal accessor in rrdfunctions-internals.h), keep the
   filter/foreach rewrite for the T5 step. Merge T1+T2 (glm) — T2 is small and
   lands naturally on the new type. Comment updates (`sqlite_aclk.c:1406`,
   `sqlite_aclk_node.c:182`, `rrdhost.h:358`) move into the steps that rename
   the things they reference, killing the separate docs step.
9. **T6 split** (deepseek): (a) mechanical named-type commit (renames
   init/destroy; fixes `daemon-shutdown.c:22` header; migrates the two
   unittest init call sites; note destroy runs only under
   `#if defined(FSANITIZE_ADDRESS)` — verified), then (b) the deadline
   ownership commit. Public `deadline_extend` has no external caller
   (extension happens broker-internally at `rrdfunctions-inflight.c:743`;
   plugin-side evloop owns a SEPARATE deadline) — keep extend internal.
   Not-found semantics: skip, never expire.
10. **New tests** (accepted): golden-output test for both streaming emitters
    (written BEFORE the T5 rewrite); negative test "COLLECTOR registering
    'config' is rejected" (T3); notification-conditions test pinning the
    add/del arm+flag+FUNCTION_DEL truth table (T4); deadline-extension
    visibility test (T6b).

## Rejected / adjusted findings (my judgment, with reasons)

- **mimo: "defer T6 entirely — it fixes no bug, blast radius = every plugin's
  progress path"** — REJECTED. Blast-radius claim is wrong: plugin-side evloop
  owns a separate worker-local deadline (`functions_evloop.c:289`); the change
  is daemon-side only (minimax verified the same). The invariant is real but
  convention-enforced; the user's long-term-best directive says fix it.
- **minimax: "defect 8 (is_available needs host) is not a defect"** —
  ACCEPTED as reframe: it's an API-shape consequence of missing back-pointer,
  not a standalone defect. Plan v2 rewords.
- **minimax: "merge T1+T4 so step 1 pays for itself"** — REJECTED in favor of
  merge T1+T2; T4 carries its own correctness contract (event semantics,
  sync-dispatch) and deserves its own review.
- **Ordering** — no panel consensus (4 different orders). My call for v2:
  baseline → T1+T2 → T3(+shim move, D1) → T4 → T6a → T6b → T5 last (widest
  consumer churn lands on a stable registry contract; T6b mid-chain so the
  GHSA/dyncfg gates catch deadline regressions before T5's churn).
- **T6b deadline model — my synthesis, needs round-2 validation**: instead of
  per-GC-pass txid lookups (lock-ordering questions) or a documented-but-
  unenforced borrowed pointer, the parser's `inflight_function` holds an
  **acquired reference on the broker's dictionary item** (refcount-backed
  borrowed pointer): acquire in `pluginsd_function_execute_cb`, release in the
  parser's delete callback after the result chain. Keeps lock-free GC reads
  and the `smaller_monotonic_timeout_ut` minimum computation; converts the
  convention into a refcount guarantee. Cross-check against the dedicated
  deadline-audit consult (job_83fd…) — its question list includes exactly this
  option (c).

## Pending inputs before plan v2

- job_83fd… deadline/lifetime audit (running)
- job_c655… notification idiom survey (running)
- job_a72b… transport adapter + rrd_collector (running; covers the two areas
  the user promoted to candidate scope — round-1 panel called the transport
  exclusion "defensible" but that predates the long-term-best directive)

# Notification-idiom consult consolidation (job_c655…2374)

Panel consensus (deepseek, glm, mimo; minimax dissent refuted): **T4's observer
seam is dropped entirely.** Superseded design:

- **Key reframe** (deepseek, verified): the ADD path already has the clean
  direction — the registry sets `RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED`, and
  *streaming calls down* into database-owned renderers
  (`stream_sender_send_global_rrdhost_functions` lives in
  rrdfunctions-exporters.c, invoked from command-begin-set-end-init.c:48 and
  command-function.c:16). The only upward wart is the DELETE path.
- **ACLK arm stays as-is** (deepseek; glm's dirty-flag variant rejected):
  `aclk_arm_node_manifest()` lives in `src/database/sqlite/` — not an upward
  dep — and IS the poll idiom in timestamp form (non-blocking CAS + 30s
  coalescing window; its own comment forbids the event-loop route). The host
  flag space is nearly exhausted (bits 5-6 free) — don't spend one.
- **Delete path fix**: per-host pending-deleted-names set (lazily created,
  keyed by sanitized name), populated by `rrd_function_del` for
  `!st && !is_dyncfg` instead of calling `stream_send_function_del`; drained
  by a new renderer in rrdfunctions-exporters.c at the existing flag-poll
  site: FUNCTION_DEL lines first, then the full re-list, one buffer/commit.
  The drain re-checks `STREAM_CAP_FUNCTION_DEL` +
  `rrdhost_can_stream_metadata_to_parent()` (today in command-function-del.c:8-11).
  `stream_send_function_del()` loses its only caller → absorbed/removed;
  the WIRE keyword and capability stay.
- **Bonus fix**: removes a real blocking hazard — today the deleting thread
  (collector, or receiver parser re-broadcasting a child's delete to the
  grandparent) can block on sender backpressure (`waitq_acquire`,
  stream-sender-commit.c:80).
- **minimax dissent REJECTED on evidence**: it claimed the bulk re-emit is
  the source of truth so FUNCTION_DEL can be dropped. Verified false: the
  parent's `pluginsd_function()` only ADDS; no prune-on-relist exists; a
  deleted function would persist on the parent — and stay *available*, since
  its collector is the still-running receiver thread. FUNCTION_DEL is
  mandatory for correctness.
- **Ordering/races** (glm, verified reasoning): DELs-then-relist in the same
  buffer resolves del-then-readd (name reappears in relist after its DEL) and
  stray DELs (parent logs debug, harmless). Reconnect self-heals via
  `stream_sender_on_ready_to_dispatch` full re-send.
- **Documented edge**: flag-poll starvation on a fully-idle host (no
  rrdset_done ticks) delays the DEL — same pre-existing property as
  UPSTREAM_SEND_VARIABLES; record in SOW, do not re-couple for it.
- **No pub/sub helper exists in libnetdata** (all four models) — every
  subsystem hand-rolls flags/queues/arms; the deleted-names set is consistent.

# Deadline-audit consult consolidation (job_83fd…af0a)

Verdicts: deepseek=(b) broker accessor; glm=(b); mimo=(d) by-value copy;
minimax=(c) refcount. **Decision: (b)**, my earlier (c) synthesis withdrawn —
glm's refutation is decisive: the delete callback frees interior pointers
(r->transaction/cmd) at logical deletion, so a refcount-alive record with
dangling interiors is a trap. mimo's (d) refuted by glm: progress-incapable
children (no STREAM_CAP_PROGRESS) never registered a progresser, so the
parser-side copy would not extend while the broker's does → early GC cancels —
a behavior change. Unanimous: today's code has NO deadline UAF (safe by a
causal invariant: global record freed only downstream of parser-entry unlink,
all under the parser write lock) — but the invariant is convention, not
ownership.

**T6b shape (agreed)**: broker accessor `rrd_function_transaction_deadline(
transaction, *out)` = get-and-acquire + __atomic_load_n + release; parser
records drop the `usec_t *stop_monotonic_ut` field; GC/minimum computation call
the accessor (one O(1) lookup per entry per GC pass — GC only runs on next
submission with an expired minimum). Lock order verified safe by both models:
parser-write → global lookup is the EXISTING direction (delete-cb chain already
does parser-write → global-write); no path holds global lock then takes parser
lock. On lookup miss: SKIP + log, never reap (invoking the delete-cb chain when
the invariant already failed would UAF via result.data=r; round-1 glm position
adopted over audit-glm's "reap"). rfe keeps the pointer with a documented
validity window (execute_cb invocation only; all executors are synchronous —
dyncfg chain verified earlier).

**NEW REAL BUGS found (both must enter plan v2 as fix steps):**

- **UAF-A (deepseek + glm, independently constructed):** `r->canceller.data =
  t` (parser dict value, pluginsd_functions.c:298) and `r->progresser.data =
  parser` (:303) outlive the parser: cancel/progress threads holding an
  acquired global item invoke callbacks on freed parser memory. The dispatcher
  drain does NOT protect this — parser destroy runs BEFORE
  `rrd_collector_finished()` (pluginsd_parser.c:1529-1530; streaming:
  stream-receiver.c:1375 vs stream-thread.c:659). Reachable in normal
  operation (plugin exit with in-flight cancel/progress). Fix (both parts):
  (i) canceller becomes keyed/self-validating like the progresser — register
  `data = parser`, pass the transaction, re-validate via
  get_and_acquire; (ii) reorder teardown: `rrd_collector_finished()` BEFORE
  `pluginsd_process_cleanup(parser)` in both paths (this is the ordering the
  dispatcher-refcount protocol always assumed; re-verify the streaming path
  and the manifest-refresh comment at pluginsd_parser.c:1479-1493).
- **UAF-B (glm):** `inflight_function_find` uses raw `dictionary_get`
  (pluginsd_functions.c:472); `pluginsd_function_result_begin` points
  `parser->defer.response` at `pf->result_body_wb` (:515) and body lines
  stream in with no reference across parser_action iterations. Parser GC (from
  a concurrent submission on the same parser) can delete the entry mid-stream;
  in wait mode the waiter frees temp_wb while the parser appends. Fix: acquire
  in `inflight_function_find`; stash the acquired item in `parser->defer`,
  release in `pluginsd_function_result_end`; same for
  `pluginsd_function_progress`.

**Smaller agreed fixes for plan v2:** atomicize all deadline reads (32-bit
tear risk today) — free via the accessor; `smaller_monotonic_timeout_ut`
plain-load/store race → relaxed atomics (minimax); inverted `sent` logic in
`pluginsd_function_cancel` (:150-176, logs "didn't match" after every
successful send) (glm); conflict-callback leaks `pf->payload`/`pf->source`
(:74-83) (glm); dead field `rrd_function_call_wait.host_function_acquired`
(:158,262 — assigned, never used) (deepseek); centralize the
RRDFUNCTIONS_TIMEOUT_EXTENSION_UT application in one effective-deadline helper
(4 divergence-prone sites today; minimax's bake-into-stored-value variant
rejected — it silently adds +1s to the plugin-visible timeout_s at :241).

**Flagged, decision needed (fold into round 2):** lazy GC — a wait-mode caller
timeout never sends FUNCTION_CANCEL to the plugin until the NEXT submission on
that parser (no timer consumes smaller_monotonic_timeout_ut). Candidate fix
aligned with long-term-best; behavior change (plugins see cancels sooner).
Blocking `send_to_plugin` (up to 100ms) under the parser write lock (insert cb
+ GC): overlaps transport-adapter consult (job_a72b) — decide there.

**Must-not-touch (unanimous):** free_with_signal handshake; waiter's raw `r`
(sole-deleter path); DONT_OVERWRITE double rejection at both layers;
`host_function_acquired` release protocol (rrdfunctions-inflight.c:97) incl.
early-error paths; callbacks.spinlock discipline + lock order
callbacks.spinlock → parser items lock; collector drain loop + never gate
dispatcher_acquire on `running`.

**Implementation verification item (minimax):** confirm every parser thread is
registered with nd_thread machinery so `nd_thread_join_threads()` precedes the
ASAN-only inflight destroy.

# Transport/collector consult consolidation (job_a72b…397b)

## Area A — pluginsd transport adapter (now in scope per long-term-best)

Unanimous: extract the four hidden hook behaviors into named operations
(register_and_send / deliver_result / cancel / progress / gc / destroy_all),
send no longer inside the insert hook. Guarantees that MUST survive, mapped by
all four models: (G1) duplicate-transaction rejection answering the SECOND
caller (defensive; global broker already rejects first,
rrdfunctions-inflight.c:598-612); (G2) insert+send+check atomic vs result
delivery/GC (today via explicit items write lock, pluginsd_functions.c:268);
(G3) destroy-time 503 sweep to every pending caller via the pre-set
`pf->code = HTTP_RESP_SERVICE_UNAVAILABLE` planted at insert (:16) —
"the delete callback can't tell result-received from plugin-died" (deepseek);
(G4) the five wire-format sites move VERBATIM (:26-38, :41-49, :136, :161,
:219) + capability gates (STREAM_CAP_PROGRESS :300-303).

Container fork (2-2): (i) keep DICTIONARY as private index behind the named
API (deepseek, mimo — refcounts fix UAF-B naturally; consistency with sibling
registries; stats) vs (ii) explicit SPINLOCK+Judy table (glm, minimax — no
hook machinery at all; deliver outside locks; deterministic destroy; glm:
deferred-destruction of referenced items at destroy is actively harmful).
**My recommendation: (i)**, adopting from (ii) what composes: result.cb
delivery moved outside container locks where achievable, explicit
reject_duplicate freeing all three heap members (fixes the conflict leak).
Round-2 panel question.

**UAF-C (glm, new — third real bug):** `execute_cb_data = parser` is a raw
unrefcounted pointer in host->functions (pluginsd_functions.c:415-417);
availability checks (collector->running, rrdhost_state_id) are TOCTOU.
Plugin-exit window is instructions-wide, but the STREAMING window is huge:
object_state_deactivate (stream-receiver.c:1335) → parser freed (:1375) spans
ml/sender/rrdcontext teardown while the stream THREAD's collector stays
running until stream-thread.c:659. A web thread passing the check then calling
pluginsd_function_execute_cb on the freed parser = UAF. Fix (glm): refcounted
`struct pluginsd_function_transport { REFCOUNT; PARSER * }` as the
execute_cb_data; acquire-or-503 in execute_cb; release on function
delete/re-register (rrd_functions_cleanup hook point, rrdfunctions.c:33-37)
and in parser_destroy after the tx drain. ~40 lines; rrd_function_execute ABI
untouched. Composes with the deadline-audit UAF-A fixes (keyed canceller;
teardown reorder) — round 2 must reconcile which combination is the end state.

A3 verified by two models: parser->inflight.functions and struct
inflight_function never escape pluginsd_functions.c/.h (+1 doc pseudocode
mention). The transport refactor is fully local. minimax: consider the same
explicit-API treatment for the GLOBAL broker as follow-up — matches T6a.

## Area B — rrd_collector lifecycle

- **Drain loop (adopt, glm):** replace the `while(!acquire_for_deletion)
  sleep(1ms)` retry loop with `refcount_acquire_for_deletion_and_wait()`
  (refcount.h:139-158) — marks DELETED immediately (new dispatches fail from
  teardown start; today's loop leaves 0 between retries so new dispatches can
  extend the drain unboundedly), then waits for holders. Same idiom as
  object_state_deactivate (object-state.c:41-73). Strictly stronger; 1 line.
- **Caller map (glm/deepseek, recorded):** started(): pluginsd_parser.c:1420,
  stream-thread.c:522 (ONE provider per stream thread, not per child),
  rrdfunctions.c:20,51 (implicit via add), dyncfg.c:385, rrdfunctions-inline.c:33,
  plugin_proc.c:214, windows_plugin.c:267, + module threads. finished():
  pluginsd_parser.c:1530, stream-thread.c:659, plugin_diskspace.c:871, and the
  global per-thread-exit cleanup rrd.c:81 → threads.c:400 (idempotent TLS NULL
  check) — the safety net all implicit starts rely on; any rename must keep it.
- **Rename fork:** glm: rename to `rrd_function_provider` (struct is a
  per-thread function-provider token; name collides with the UNRELATED
  st->collector_tid chart-ownership scheme), fold rrdcollector.* into
  rrdfunctions-provider.*, ~13 files mechanical; keep per-thread semantics
  (per-connection availability is already handled by the state_id epoch;
  thread identity is load-bearing for the drain guarantee). mimo: don't rename
  in this refactor. **Recommendation: rename, as its own mechanical commit**
  (long-term-best directive) — user fork, flagged.
- Split provider-vs-collector concepts: rejected by panel (struct already IS
  only the provider; odd "just in case" starts become visible after rename —
  follow-up deletions, tracked separately).

## Sequencing input (glm): (1) drain-loop swap (zero risk) → (2) explicit tx
table + transport refcount (fixes UAF-C, conflict leak, log inversion, and
lock-held delivery in one coherent local change) → (3) mechanical rename.
