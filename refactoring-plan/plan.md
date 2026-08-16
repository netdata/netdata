# Plan v7: Promote the Netdata function mechanism to a first-class component

Status: DRAFT v7 — consultation loop closed at the cap (5 rounds,
round{1..5}-consolidation.md); clean-context subagent review complete:
**SOUND-WITH-AMENDMENTS** — architecture confirmed, three contained
amendments encoded here (pending-victim delivery via a new dictionary
detach-then-deliver primitive; leaf lock extended over swapped STRINGs for
C4's copies; DYNCFG pin moved into the insert/transfer callbacks) plus one
softened behavior note. Next: the human approval gate (D1, D3-D7).
Worktree: `~/repos/nd/refunc` (branch `refunc`, base `master` @ 9d4c991fa)
Status repo: `~/repos/nd/refunc-status` (linked at `.local/status`)
Supersedes: plan v5. Consolidations: `round1-consolidation.md` …
`round5-consolidation.md`.
Scope directive (user, 2026-08-15): LONG-TERM BEST — any refactor serving the
function-mechanism goal is in scope regardless of churn. Wire protocol frozen.

## Goal

Refactor the function-call mechanism (`src/database/rrdfunctions*`,
`rrdcollector*`, `src/plugins.d/pluginsd_functions.*`) into a named component
with owned state and defined interfaces, remove the layering violations, and
**fix four real lifetime bugs found during planning**. No wire-protocol
changes (FUNCTION / FUNCTION_PAYLOAD / FUNCTION_CANCEL / FUNCTION_PROGRESS /
FUNCTION_RESULT_BEGIN..END byte-identical; the five format sites in
pluginsd_functions.c:26-49,136,161,219 move verbatim). No user-visible
behavior changes except items recorded as decisions or behavior notes.

## Real bugs this plan fixes (verified in rounds 1+2, 4-model consensus)

- **UAF-A (canceller/progresser data):** `r->canceller.data = t` (parser dict
  value, pluginsd_functions.c:298) and `r->progresser.data = parser` (:303)
  outlive their referents. Cancel/progress threads holding an acquired global
  item invoke callbacks on freed memory. Reachable (a) on plugin exit /
  receiver disconnect (parser destroy precedes the dispatcher drain:
  pluginsd_parser.c:1529-1530; streaming: stream-receiver.c:1375 vs
  stream-thread.c:659), and (b) WITHOUT any exit: normal completion deletes
  the parser entry (:459-463) while a concurrent cancel holds the global
  record. The parser dict is not fixed-size (:107), so recycled storage can
  false-match the pointer-identity scan (:155) ⇒ FUNCTION_CANCEL for the
  wrong transaction. The keyed fix also converts the O(n) identity scan to
  O(1) (deliberate improvement, recorded).
- **UAF-B (unrefcounted result streaming):** `inflight_function_find` uses raw
  `dictionary_get` (pluginsd_functions.c:472); `parser->defer.response`
  aliases `pf->result_body_wb` (:515) with no reference. GC runs only from
  `execute_cb` on submitter threads (:293,:311); its `dictionary_del` fires
  the delete callback ⇒ `rrd_async_function_signal_when_ready` frees
  `temp_wb` (rrdfunctions-inflight.c:202-205) while the parser thread appends
  deferred body lines to the same buffer.
- **UAF-C (execute_cb parser TOCTOU):** `execute_cb_data = parser` raw and
  unrefcounted (pluginsd_functions.c:415-417); availability gates
  (`running`, `rrdhost_state_id`) are check-then-use and execute holds NO
  dispatcher ref (unlike cancel/progress). Windows: streaming
  `object_state_deactivate` (stream-receiver.c:1335) → parser freed (:1375)
  spans ml/sender/rrdcontext teardown while the stream thread's collector
  runs until stream-thread.c:659; pluginsd has a narrow window too (parser
  freed at pluginsd_parser.c:1529 while `running` stays true until :1530);
  and the SAME class exists via dyncfg on localhost: pluginsd_dyncfg.c:49-50
  stores raw `parser` in the DYNCFG entry, invoked at dyncfg-intercept.c:534.
- **UAF-D (streaming send race on `rpt`):** every dispatcher-side send for a
  streaming parser goes through `send_to_child(rpt)` (stream-receiver.c:517,
  :378), and `rpt` is freed in `stream_receiver_remove_internal` immediately
  after `rrdhost_clear_receiver` returns, with no synchronization against
  in-flight cancel/progress/GC sends. Closed by the transport ONLY because it
  gates the send paths (insert :54, GC-cancel :137, canceller :164,
  progresser :222) inside transport-guarded sections — a design constraint,
  not a side effect.

Deadline sharing (`stop_monotonic_ut` raw pointer across layers) is NOT a UAF
today — re-confirmed 4/4 in round 2: the global record is freed only
downstream of parser-entry unlink. But the invariant is convention, not
ownership, and 64-bit reads are mixed atomic/plain (32-bit tear risk). Fixed
by design below (C5b).

## Design (final shapes; forks marked D#)

### C1 — `RRD_FUNCTIONS` opaque registry + `RRD_FUNCTION_ACQUIRED` handle

Opaque per-host registry owning the definitions dictionary (idiom:
`RRDSET_ACQUIRED` rrdset.h:9, `RRDHOST_ACQUIRED` rrdhost.h:18).
`RRDHOST.functions` becomes `RRD_FUNCTIONS *`; ctor/dtor at the same sites
(rrdhost.c:470,729,921 — the dtor placement is an ACLK shutdown contract,
sqlite_aclk.c:1398-1415). Dictionary callbacks (rrdfunctions.c:16-184) move
inside verbatim (fixed-size slot + SWAP merge semantics byte-identical; the
callback `data` becomes the registry struct with a host back-pointer).
`RRD_FUNCTION_ACQUIRED` replaces the raw `DICTIONARY_ITEM*` in
`rrd_function_verify_access` (real consumers: `rrd_function_run` internally +
`rrdfunctions-unittest.c:94-136`; MCP passes NULL — no change there) and in
the broker record. **Lookup-time transport pin (closes the read-then-acquire
residual):** a dedicated LEAF spinlock guards the entry's SWAPPED fields —
the (tag, data) pair AND the help/tags STRINGs (subagent review: an item
ref does not pin a displaced STRING, so C4's byte copies must also happen
under this lock or the copy itself reads freed bytes): the conflict cb
takes it (inside the index wrlock) around the swaps and the displaced
releases/frees; the registry find takes it standalone AFTER the standard
item acquire (the item ref already pins the entry memory) to read the pair
and entry-pin the transport; C4's view-struct copy takes it per entry while
copying strings — leaf lock, so no ordering cycle (a
registry-rwlock-across-find variant would ABBA against the conflict cb and
is rejected; the raw "pin under the index rdlock" has no dictionary hook and
is unimplementable). **Capture-at-find (MUST):** `RRD_FUNCTION_ACQUIRED`
captures (execute_cb, transport, pin) at find; executors NEVER re-read
`rdcf->execute_cb_data` at call time (rrdfunctions-inflight.c:252,:306,:650)
— a stale capture degrades to a clean 503 via acquire-or-fail. The pin
attaches ONLY to the item the find returns (never to prefix-retry
intermediates released at rrdfunctions.c:432). Released wherever
`host_function_acquired` is released today (rrdfunctions-inflight.c:97,
:590, :606, the verify_access early-outs, and rrdfunctions-unittest.c:136);
the dyncfg node lookup and the template fan-out use the same leaf-lock
helper. (Pin machinery lands with C6; C1 carries the handle shape.) Exporters keep module-internal access via
rrdfunctions-internals.h in this step (mechanical), full API in C4.

### C2 — Registration source enum + registry-enforced dyncfg namespace

`RRD_FUNCTION_REG_SOURCE { COLLECTOR, STREAMING, INTERNAL }` replaces the
`from_streaming`/`internal` bool pair (del) and is added to add. Registry
enforces on its own sanitized key: COLLECTOR+dyncfg-name = reject (log
sanitized name, keep the reasoning comment from pluginsd_functions.c:357-395);
STREAMING+dyncfg-name = accept (child "config" proxy); INTERNAL+dyncfg-name =
accept (dyncfg.c:386, dyncfg-tree.c:294). Caller-side guard deleted.
`rrd_function_add_inline` absorbs INTERNAL for its ~12 callers. Call sites:
pluginsd_functions.c:415,448; dyncfg.c:386,427; dyncfg-tree.c:294;
rrdfunctions-inline.c:35; unittests — BOTH the del sites
(rrdfunctions-unittest.c:143-145,271; mcp-tools-execute-function-
unittest.c:187) AND the add sites (rrdfunctions-unittest.c:40,45,49,229).
The source enum is also load-bearing for C6: the registry cleanup hook's
transport release is gated on it (COLLECTOR/STREAMING only — INTERNAL
`execute_cb_data` is caller-owned: dyncfg-tree passes host, dyncfg NULL,
inline the caller's data). **The source is STORED per entry in
`struct rrd_host_function` and participates in the conflict SWAP** together
with `execute_cb_data` (rrdfunctions.c:168-175), so cleanup/delete releases
always key on the value they actually hold (see C6 — mixed-source
INTERNAL↔COLLECTOR collisions on localhost are reachable: inline names are
unreserved). **Compat shim (D1 = DELETE, user decision 2026-08-16):** the
`"systemd-journal"→"logs"` default (rrdfunctions.c:244) is REMOVED. Git
archaeology: the shim, the tags wire field, and the plugin's explicit
"logs" tag all arrived in one commit (da32dd8be, #16574, first release
v1.45.0, 2024-03) — its only beneficiaries are pre-v1.45.0 children.
Accepted cosmetic regression (recorded): such children's systemd-journal
function groups under "top" instead of "Logs" on modern parents. Registry
keeps only its generic "top" default for tagless arrivals (only wire
arrivals can be tagless — verified: all in-tree inline/dyncfg callers pass
tags; the in-repo plugin sends "logs", systemd-main.c:465).

### C3 — Streaming delete-path decoupling (redesigned in round 2)

The ADD path already has the clean direction (registry sets
`RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED`; streaming polls at
command-begin-set-end-init.c:48 and calls DOWN into database-owned
renderers). Fix only the DELETE path, with the round-2 protocol:

- Per-host pending-deleted-names set: **eagerly created** with the registry
  (deleters include non-stream threads, e.g. dyncfg_del_low_level via
  dyncfg.c:427), internally locked, keyed by sanitized name, **destroyed with
  the registry at host free**. `rrd_function_del` for `!st && !is_dyncfg`
  populates it instead of calling `stream_send_function_del()`; flag still
  set for `!st` (dyncfg quirk preserved: dyncfg global deletes set the flag
  but never emit FUNCTION_DEL — rrdfunctions.c:369-373). **Not populated when
  the host has no parent/sender** (else it grows for process lifetime on
  never-streaming hosts).
- **Ordering contract (mandatory, prevents lost DELs):** deleter inserts into
  the set BEFORE setting the flag (release order); the renderer clears the
  flag FIRST, then snapshots-and-clears the set under its lock, then emits
  FUNCTION_DEL lines, then the full re-list — one buffer/commit. A del
  landing after the snapshot re-sets the flag with its entry already queued;
  nothing is stranded.
- **The drain lives INSIDE `stream_sender_send_global_rrdhost_functions`**
  (rrdfunctions-exporters.c:29), where the flag is already cleared — the
  renderer has multiple callers (the poll site AND the reconnect on-ready
  push, command-function.c:16 via stream-sender.c:178); draining only at the
  poll site would strand queued DELs across reconnects.
- **Layering:** the renderer stays free of streaming headers. The streaming
  caller resolves `STREAM_CAP_FUNCTION_DEL` +
  `rrdhost_can_stream_metadata_to_parent()` (absorbing
  command-function-del.c:8-11, which loses its only caller) and passes the
  verdict in (e.g. a `can_function_del` argument). Gate false ⇒ the snapshot
  is **discarded** (matches today's silent drop; prevents unbounded growth on
  no-FUNCDEL parents). `#include "streaming/protocol/commands.h"` leaves
  rrdfunctions.c and is NOT re-introduced in rrdfunctions-exporters.c.
- `aclk_arm_node_manifest()` calls STAY (src/database/sqlite/ — not an upward
  dep; already the poll idiom; its comment forbids the event-loop route). The
  `in_manifest` capture at delete time (rrdfunctions.c:339) stays at delete
  time.
- Bonus fix: the deleting thread can no longer block on sender backpressure
  (today: waitq_acquire, stream-sender-commit.c:80).
- Behavior notes (recorded): FUNCTION_DEL delivery becomes eventual (flag
  poll) instead of synchronous-blocking; flag-poll starvation on a fully idle
  host delays the DEL (same property as UPSTREAM_SEND_VARIABLES); reconnect
  self-heals (stream-sender.c:165-179) and now also drains the queue; the
  readiness gate moves from del time to render time, so a del queued while
  the parent was ready but drained after a disconnect is discarded — a
  slightly wider drop window than today, masked by the reconnect full
  re-list (state of record; the parent's stale entry is unavailable and
  freed at host free).

### C4 — Iteration/visibility API

`rrd_functions_foreach(host|st, FILTER, cb, data)` with THREE filters —
**every filter includes the availability check** (verified: both streaming
loops check `rrd_function_is_available` first, exporters.c:12,:36):
`EXPORTABLE` (availability + skip DYNCFG|RESTRICTED), `STREAMABLE_CHART`
(availability + skip DYNCFG), `STREAMABLE_GLOBAL` (availability + skip LOCAL,
count DYNCFG for `dyncfg_add_streaming()`); RESTRICTED functions DO stream.
View struct: name, help, tags, **timeout**, access, priority, version,
options — strings as byte copies valid for the callback (the conflict
callback can free STRINGs under the INDEX lock, per the comment at
rrdfunctions-exporters.c:200-204). Availability variant preserving
`host=NULL` semantics for `chart_functions_to_dict` (:136). Consumers
migrated: all 7 exporter loops, their callers (api_v1_functions.c:15,
api_v1_info.c:133 for host_functions2json), `rrdinstance_acquired_functions`
(rrdcontext-instance.c:59 → jsonwrap.c:57), rrdset2json.c:106,
api_v2_contexts.c:560,694. Borrowed-STRING hazards fixed in ALL of: `struct function_v2_entry`
(api_v2_contexts.c:24 keeps borrowed `STRING*` after iteration — closed by
the byte copy alone), and functions2json / host_functions2json /
host_functions_to_dict (string2str results used while the conflict cb frees
displaced STRINGs under the INDEX lock) — the latter closed ONLY because the
copies are made under the C1 leaf spinlock that the conflict cb holds while
swapping/freeing (subagent review: a byte copy outside that lock still READS
freed bytes; an item ref does not pin a displaced STRING). Recorded as fixes
with a small allocation-behavior change. `st->functions_view` destroy at
rrdset-index-id.c:149 → `rrd_functions_rrdset_view_destroy(st)`.

### C5 — Named transaction broker + deadline ownership

**C5a (mechanical):** the global inflight dictionary becomes
`struct rrd_function_transactions` with `_create()/_destroy()`;
daemon-shutdown.c:22 local prototype removed (header used); unittest init
call sites migrated (mcp-tools-execute-function-unittest.c:108,
dyncfg-unittest.c:802); destroy remains ASAN-only
(`#if defined(FSANITIZE_ADDRESS)`, daemon-shutdown.c:350-360) — documented.
**C5b (deadline, broker-keyed accessor):**
`rrd_function_transaction_deadline(transaction, *out)` = get-and-acquire +
`__atomic_load_n` + release. Parser records DROP the
`usec_t *stop_monotonic_ut` field; **only GC uses the accessor** (one O(1)
lookup per entry per pass, under the parser write lock — bounded by in-flight
functions per connection; the parser dict key IS the compact transaction
string, `pf_dfe.name`, so no conversion is needed). Note: no reachable path today
creates a parser entry whose global record dies first (pluginsd always
registers sync=false, incl. the dyncfg intercept), so no "fixes a latent
dangling read" claim is made — miss→skip is the defensively correct choice
regardless (subagent review softened an earlier overstatement here). The insert-path minimum
computation keeps using `rfe->stop_monotonic_ut` directly (in the documented
validity window). Key-format invariant stated: both sides use
`uuid_unparse_lower_compact` (pluginsd_functions.c:266,
rrdfunctions-inflight.c:544) — a key-format change would silently turn every
lookup into a miss. Lookup miss = SKIP + log, never reap (reaping would
invoke the delete-cb chain into memory whose invariant already failed);
**the all-skipped case is guarded** so `smaller_monotonic_timeout_ut` staying
0 does not fire a no-op GC with log spam on every submission.
`smaller_monotonic_timeout_ut` stays a plain field, made valid by C6: today
the invariant is BROKEN in one spot — execute_cb unlocks at
pluginsd_functions.c:291 (the `!sent_successfully` branch) and calls GC at
:293 unlocked, so GC's `= 0` reset (:120) races the locked readers/writers
(pre-existing, low impact). C6's `garbage_collect` runs its container phase
under the parser write lock in ALL invocations, which makes the plain field
sound; if a later change moves that phase outside the lock it needs a CAS
loop, not relaxed atomics.
`rrd_function_execute.stop_monotonic_ut` keeps the pointer with a documented
validity window (the execute_cb invocation only; no daemon-side executor
stashes it past the invocation — dyncfg chains it synchronously,
dyncfg-inline.c:17; evloop copies at submit, functions_evloop.c:176).
Extension-on-progress unchanged (same atomic, broker-internal,
rrdfunctions-inflight.c:743); no public extend. Centralize
`RRDFUNCTIONS_TIMEOUT_EXTENSION_UT` application in one effective-deadline
helper (4 sites: pluginsd_functions.c:123,:142-143,:306-307,
rrdfunctions-inflight.c:317). Coupled doc: FUNCTION_UI_REFERENCE.md:1502
documents the removed `*pf->stop_monotonic_ut + EXTENSION` pattern — updated
in the same step. Rejected variant: baking the extension into the stored
value (silently adds +1s to the plugin-visible timeout_s at
pluginsd_functions.c:241).

### C6 — Transport explicit API + lifetime fixes (pluginsd_functions.* +
pluginsd_dyncfg.c)

Scope: the pluginsd transport trio (pluginsd_functions.c/h + the init/cleanup
calls in pluginsd_internals.c:62,69) PLUS pluginsd_dyncfg.c AND dyncfg.c (the
dyncfg execute path registers the same callback with raw `parser` — see
UAF-C — and the DYNCFG node's pin release sites live in dyncfg.c).
Named operations replace container-hook side effects: `register_and_send`
(dup-check answering the SECOND caller + insert + format + send in one
critical section — G2: the FUNCTION line send MUST stay inside the critical
section, else a concurrent FUNCTION_CANCEL can overtake it on the wire;
format-before-lock is allowed), `deliver_result`, `cancel`,
`progress_to_plugin`, `garbage_collect`, `destroy_all` (preserves the
destroy-time 503 sweep, G3 — the pre-set
`pf->code = HTTP_RESP_SERVICE_UNAVAILABLE` trick at :16 survives).
**D4 (DECIDED (a), 2026-08-16):** the DICTIONARY stays as a private index
behind the API (4/4 consult consensus: per-item refcounts are exactly the
UAF-B fix; the dictionary's deferred destruction and delete-cb-at-final-free
timing IS the G3 sweep mechanism; a Judy+spinlock table would re-implement
the bug class this plan exists to fix).
**Delivery discipline (MUST, not may — closes a verified ABBA deadlock):**
`garbage_collect` and `destroy_all` collect their victims under the container
lock and deliver `result.cb` AFTER releasing it. Rationale: today's shape can
already deadlock — a waiter holding `tmp->mutex` invokes the canceller
(container read lock, pluginsd_functions.c:154) while GC under the container
write lock runs the delete-cb chain into `signal_when_ready`'s
`tmp->mutex` (rrdfunctions-inflight.c:328-340; api_v1_function.c:39-43 makes
the cancel-while-waiting path production-reachable). Mechanism obligations
(rounds 4-5): victims are dup'd under the traversal
(`dictionary_acquired_item_dup`, lock-free CAS) **with their keys COPIED
(strdupz) under the same locked traversal** — the item-owned key is freed
with the item, and a concurrent insert-triggered sweep can free it between
the release-dup and the del (UAF read; qwen round 5); after unlocking,
per-victim **release-dup-then-del-by-the-copied-key** — the zero-ref del
path is the only one that fires the delete cb lock-free, and external
releases NEVER fire callbacks (the load-bearing dictionary invariant,
dictionary-refcount.h:92-150, which also makes the canceller final-release
discipline trivially satisfiable: the canceller/progresser must merely never
RUN A SWEEP while holding the waiter mutex). **The stock
`garbage_collect_pending_deletes` fires delete callbacks under the items
WRITE lock (dictionary.c:172-215) and is therefore NEVER used where a
delivery can result:** result_end's drain, GC's trailing drain, and
destroy_all's pre-destroy drain use a **NEW small dictionary primitive**
(libnetdata — generic, reusable): under the items write lock, DETACH the
pending-deletion victims (they are already hashtable-unlinked; unlink from
the items list, clear the pending marks), then run their delete callbacks
after unlock. Dup-based collection CANNOT reach these victims (subagent
review: traversals and acquire refuse DELETED items,
dictionary-refcount.h:173-176; a blind dup would RESURRECT them by clearing
pending; a del-by-key misses the unlinked item) — the detach primitive is
the only correct shape. **Enabling invariant (hard
constraint, recorded):** no thread may hold a waiter mutex while acquiring
this dictionary's items (linked-list) lock — the keyed canceller/progresser
touch ONLY the index lock, and execute_cb always precedes the wait-mutex
acquisition (rrdfunctions-inflight.c:306-310); this discipline is
load-bearing, and a future dfe-style cancel path would re-arm a
waiter⇄parser-thread deadlock. Honest residue: a straggler that goes
pending AFTER destroy_all's snapshot is delivered by `dictionary_destroy`
under the lock — bounded, and post-drain no waiter-mutex counter-party
exists. `destroy_all` makes the container reject inserts BEFORE snapshotting
victims (destroyed-flag gating; the load-bearing re-check under the lock is
dictionary-item.h:463-466) and runs AFTER the transport mark-dead, so a late
execute_cb insert gets NULL and the existing 503 branch answers
(pluginsd_functions.c:273-287) — G3 stays complete. The wire send stays inside the critical section (G2 above); this
discipline governs waiter delivery only. Implementation comment to carry:
the dup-conflict `result.cb` delivery inside `register_and_send`'s critical
section is safe — the second caller's waiter mutex is not yet taken
(rrdfunctions-inflight.c:310).

**Transport lifetime model (round-2 redesign — the rrd_collector shape,
rrdcollector.c:13-18):** `struct pluginsd_function_transport` carries TWO
refcounts plus an alive flag:

- **Entry refcount** — one ref per holder that stores the transport:
  registry entries, **the DYNCFG node (see below)**, the global-record pins,
  the lookup-time pin (C1), plus a base ref held by the parser. Released via
  the registry cleanup hook (rrdfunctions.c:33-37) **gated on the STORED,
  per-entry ownership tag** (the C2 source enum; INTERNAL data is
  caller-owned and never released). **Conflict accounting — the collector
  pattern** (the literal precedent in the SAME callback: `new_rdcf->collector`
  is populated only when a swap occurs, and cleanup's release is a no-op
  otherwise, rrdfunctions.c:60-69):
  - tag and `execute_cb_data` swap as **ONE pair** inside one conditional —
    never as two independently-conditional swaps;
  - the insert cb acquires iff the stored tag is transport-bearing
    (COLLECTOR/STREAMING);
  - the conflict cb, when it installs a different pointer, acquires iff the
    INSTALLED tag is transport-bearing, and the displaced (tag, data) pair
    lands in `new_rdcf` for `rrd_functions_cleanup(new_rdcf)` at :181 to
    release under the DISPLACED tag;
  - when NO swap occurs (equal pointer — routine: a child re-sends its whole
    global function list on every flag set and reconnect, and the dictionary
    fires the conflict cb on every re-set even for identical values,
    dictionary-item.h:505-507), the conflict cb NEUTRALIZES `new_rdcf`
    (data NULL, non-transport tag) so the unconditional cleanup release is a
    no-op. **Invariant: an equal-pointer conflict nets zero refs — by
    neutralization, not by skipping the release** (the v4 text paired a
    conditional acquire with an unconditional release: net −1 per re-send,
    a use-after-free; caught round 4, 3/4 models).
  `rrd_function_add` NULL-checks `dictionary_set_and_acquire_item`
  (shutdown-only NULL today, rrdfunctions.c:287) and unwinds cleanly. The
  struct is freed at entry-count 0 AND dispatcher counter drained (the
  collector shape) — possibly long after the parser died (normal for
  streaming: entries survive disconnect); the transport destructor is
  parser-independent (pin holders outlive parser death; no `PARSER*` deref
  after death).
- **Dispatcher refcount** — transient holders (execute_cb, canceller,
  progresser, GC sends): acquire-or-503; ALL send paths run inside the
  guarded section (closes UAF-D). `parser_destroy` marks the transport dead,
  drains this counter (`refcount_acquire_for_deletion_and_wait`,
  refcount.h:139-158 — deadlock-free: entry refs live on the OTHER counter),
  then frees the parser, then drops the base entry ref. Post-drain acquires
  fail; survivors' `execute_cb_data` stays a valid (dead) transport until the
  entries release it.
- **Global-record pin:** registering a canceller/progresser stores the
  transport (not raw t/parser) with an entry-refcount pin **per registration**
  — an async record can hold TWO pins (canceller unconditional, progresser
  conditional — pluginsd_functions.c:297-303); both are stored in
  `struct rrd_function_inflight` and BOTH are released (no dedup) in
  `rrd_functions_inflight_cleanup` (rrdfunctions-inflight.c:78-89), NULL-safe
  for `rrd_function_run`'s early error paths (rrdfunctions-inflight.c:589,605
  → cleanup on a never-inserted record releases no-ops). Closes the
  reconnect/re-register late-cancel residual (a late cancel after the old
  transport died acquire-fails instead of chasing a freed pointer).
- **DYNCFG-node pin (discriminator + release protocol):** the `config <id>`
  registry entry carries `execute_cb_data = NULL` (dyncfg.c:386-398); the
  transport lives ONLY in the DYNCFG node (invoked at dyncfg-intercept.c:534
  and the template fan-out :494), and nodes deliberately outlive parser
  death. `df->execute_cb_data` is NOT always a transport (NULL for
  health/inline/registry-intercept adds; a raw struct pointer in
  dyncfg-unittest — in this plan's own validation set, an ungated release
  would fatal), so the node gains a **dedicated `transport` field: the pin
  exists iff that field is set**. Protocol: the pin is acquired INSIDE the
  nodes-dict callbacks only — `dyncfg_insert_cb` (dyncfg.c:58) for fresh
  nodes and `dyncfg_conflict_cb`'s transfer branch (dyncfg.c:161-166)
  INCLUDING the `!v->execute_cb` rescue arm (displaced value provably NULL
  there) — NEVER on the tmp value built at dyncfg.c:233-234 (subagent
  review: a tmp-side pin leaks one entry-ref on every same-parser re-CREATE,
  where the transfer condition is false and the loser's cleanup deliberately
  releases nothing). The pin is never carried by the incoming tmp/nv, so
  conflict losers hold nothing. Released ONLY in
  `dyncfg_delete_cb` — NOT in `dyncfg_cleanup` (which also serves conflict
  losers, dyncfg.c:168, and file-load error paths), and with NO extra
  release in `dyncfg_shutdown_low_level` (`dictionary_destroy` fires the
  delete cb per node; an explicit site would double-release). On the
  transfer branch the DISPLACED pin is released before the new one is
  installed. The fan-out's copy of the template's transport into a new job
  node (dyncfg-intercept.c:138-139) is a lock-free read that could race a
  conflict-transfer's displaced release — so the fan-out reads AND
  entry-pins the template's transport via the same leaf-lock helper as the
  lookup pin before installing it in the job node (qwen round 5). Behavior
  note: existing fan-out jobs are not re-pointed on plugin restart — they
  keep the dead transport, and acquire-or-503 turns today's UAF into a
  clean 503 (strictly better; pre-existing gap).
  Without this pin, parser death frees the transport while the node still
  points at it — UAF-C recreated one level down.
- Drain-safety proof obligation (recorded): streaming parser cleanup runs
  under the receiver lock (stream-receiver.c:1374-1375); the drain is safe
  because no gated send path takes `rrdhost_receiver_lock` (send_to_child
  takes only send_to_child.spinlock; opcode path takes the stream-thread
  queue) — AND because `parser_destroy` itself runs UNDER the receiver lock:
  the two remaining child-directed senders (command-nodeid.c:42,
  stream-path.c:246) send under `rrdhost_receiver_lock` and are serialized
  with the drain by that same lock. Moving parser_destroy out of the receiver
  lock would turn them into UAF-D-class races; a future send path taking the
  receiver lock would deadlock the drain. Both directions are constraints.

`rrd_function_execute` plugin-visible behavior untouched;
`rrd_function_cancel_cb_t` (rrdfunctions.h:17) **gains a transaction
parameter** (daemon-internal: sole implementer is pluginsd; functions_evloop
cancels via a polled flag — disclosed contract change, see must-not-touch).

Lifetime fixes bundled here:
- **UAF-B:** `inflight_function_find` → `dictionary_get_and_acquire_item`;
  the acquired item is stashed in an explicit `parser->defer` slot (struct
  gains the item + transaction key) across the RESULT_BEGIN..END span. In
  `pluginsd_function_result_end` the order is **release → del → sweep** —
  the inverse of the registry idiom at rrdfunctions.c:361-367 PLUS a drain
  using the detach-then-deliver dictionary primitive (never the stock
  `garbage_collect_pending_deletes`, which would deliver under the items
  write lock — see the delivery discipline above), both load-bearing:
  del-then-release defers the delete callback (this dict has no GC linkage;
  traversals skip deleted items), and even release-then-del alone does not
  deliver when a GC del landed mid-defer (dict_item_del unlinks from the
  hashtable FIRST, dictionary-item.h:399-427 — verified round 4 — so
  result_end's del is then a hashtable miss and the external release only
  marks pending). The sweep early-returns when nothing is pending, so the
  happy path pays nothing. A comment at the site records why the idiom is
  inverted. `pluginsd_function_progress` is release-only — no gap.
  **Destroy-mid-defer (mandatory):** `parser_destroy` releases a pending
  stashed item (release-without-del) BEFORE
  `pluginsd_inflight_functions_cleanup`, so the G3 sweep delivers through
  the normal delete callback and the dictionary is never
  queued-for-destruction forever; the path also **forces the entry's code to
  503 when the recorded code indicates success (2xx)** — a truncated stream
  must not report success; a plugin's own error code (e.g. 404) survives
  truncation (recorded behavior fix, narrowed round 4). Destroy-time defer
  handling branches on defer FAMILY (only two exist): the function family
  carries the stashed item and an ALIASED response (`pf->result_body_wb` —
  never freed here) with `action_data` an owned STRING; the JSON family
  (pluginsd_parser.c:1332-1341) has `action_data = NULL` and an OWNED
  response. Rule: free `defer.action_data` always (owned or NULL); free
  `defer.response` only for the JSON family; release the stashed item only
  for the function family. Behavior note: a GC del mid-stream now delivers
  at result_end (504 + partial body) in ALL interleavings — via the sweep —
  instead of an immediate 504 (recorded improvement). (Round-3's
  "keyed canceller drops CANCEL during the insertion window" note was
  WRONG and is withdrawn: the find path does not reject BEING_CREATED items
  and the insert cb runs under the index write lock — a keyed cancel blocks
  then finds, same as today; verified round 4.)
- **UAF-C:** transport acquire-or-503 in execute_cb (dispatcher counter);
  dyncfg path threaded through `pluginsd_config` so the DYNCFG entry stores
  the transport, not the parser.
- **UAF-A:** canceller becomes keyed/self-validating like the progresser
  (register `data = transport`, pass the transaction, re-validate via
  get_and_acquire — also removes the O(n) pointer-identity scan and the
  recycled-storage wrong-cancel hazard); progresser's raw parser pointer
  likewise routes through the transport handle.
- **D7 (rescoped in round 2):** additionally reorder teardown on the
  **pluginsd path only** (`rrd_collector_finished()` before
  `pluginsd_process_cleanup`, pluginsd_parser.c:1529-1530) as
  defense-in-depth — verified safe: chart sweep keys on collector_tid, vnode
  snapshot precedes both, manifest arms stay after finished() as their
  comment requires, finished() is idempotent per worker iteration
  (plugins_d.c:123-185). **A per-receiver reorder does NOT EXIST on
  streaming** (the collector is per stream THREAD, `thread_rrd_collector`
  __thread, shared by all receivers — a per-receiver finished() would kill
  sibling receivers; explicitly forbidden in `rrdhost_clear_receiver`). The
  transport stands alone for streaming (and is load-bearing for execute on
  every path — execute never held a dispatcher ref). Optional extra: swap
  stream_receiver_cleanup/finished order at thread exit (:631/:659) —
  shutdown-only window, low value.
- Small bugs: conflict-path leak of `pf->payload`/`pf->source` (:74-83);
  inverted `sent` logging in `pluginsd_function_cancel` (:150-176); dead
  field `rrd_function_call_wait.host_function_acquired`
  (rrdfunctions-inflight.c:158,262).

### C7 — Provider lifecycle (rrdcollector.c)

- Drain-loop swap: `refcount_acquire_for_deletion_and_wait()`
  (refcount.h:139-158, same idiom as object-state.c:41-73) replaces the
  1ms-sleep retry loop (which leaves the refcount at 0 between retries, so
  new dispatches can extend the drain unboundedly). Strictly stronger; 1
  line. Implementation note: `_and_wait` spins on tinysleep (1ns) vs today's
  1ms sleep — bounded by the ≤100ms writer persist; acceptable.
- **D5 (rename fork):** `struct rrd_collector` → `rrd_function_provider`
  (files fold into rrdfunctions-provider.*): the struct is a per-thread
  function-provider token, and "collector" collides with the unrelated
  `st->collector_tid` scheme. ~16 files (diskspace/proc/windows plugins,
  dyncfg pair, rrd.c/h, rrdcollector*, rrdfunctions* ×4, pluginsd_parser.c,
  stream-thread.c), mechanical, own commit [recommended: yes, per
  long-term-best]. Split provider-vs-collector concepts: rejected (panel: the
  struct already IS only the provider). Keep per-thread semantics
  (per-connection availability is the state_id epoch's job; thread identity
  is load-bearing for the drain guarantee). The global thread-exit cleanup
  registration (rrd.c:81 → threads.c:400) must survive the rename. The
  rrd.h:120 ↔ rrdcollector.h circular include is folded in ONLY if confined
  to the renamed files; else tracked as follow-up.

### Must-not-touch (consult-verified constraints)

free_with_signal handshake; the waiter's raw `r` (sole-deleter path);
DONT_OVERWRITE double rejection at both layers; the `host_function_acquired`
release protocol (rrdfunctions-inflight.c:97) incl. early-error paths;
**locking constraint (corrected in round 2):** the only real nesting is
parser-write → callbacks.spinlock (registration under execute_cb,
rrdfunctions-inflight.c:138-150); cancel/progress copy cb+data under the
spinlock and release it BEFORE invoking (:698-704,:748-754) — never hold
callbacks.spinlock while acquiring parser locks; never gate
`rrd_collector_dispatcher_acquire` on `running`; **no plugin/wire-visible ABI
change** (the daemon-internal `rrd_function_cancel_cb_t` typedef change is
disclosed above and does not reach plugins — functions_evloop registers no
canceller); capability gates (STREAM_CAP_PROGRESS, STREAM_CAP_FUNCTION_DEL);
wire bytes.

## Out of scope (tracked, with reasons)

- Protocol keywords living in libnetdata/functions_evloop.h (shared with every
  plugin; churn, zero behavior gain).
- `MAX_FUNCTION_LENGTH` / sanitization wire contract.
- Inline sync functions executing on the stream thread
  (stream-sender-execute.c:81): scheduling-policy fork → GitHub issue at
  close (Followup Discipline). Same file: `inflight_stream_function.sender`
  is a raw `sender_state*` used from result callbacks (:13-40) —
  host-lifetime object, shutdown-ordering exposure only; recorded here so
  no one claims send-path lifetime completeness; folded into the same issue.
- Lazy GC / no periodic tick (**D6**): a wait-mode caller timeout never sends
  FUNCTION_CANCEL until the next submission on that parser (verified: GC has
  exactly two triggers, both in execute_cb). Options: (a) leave + document
  [panel lean: independent wart], (b) send CANCEL from the waiter's timeout
  path, (c) periodic tick. Recommended: (a) now + tracked issue; (b/c) change
  plugin-visible behavior.
- Pre-existing write interleave: GC's `rrd_call_function_error` writes into
  `result_body_wb` (:129) can race the parser thread's deferred appends —
  pre-existing, not worsened by the refcount fix; recorded, tracked with D6's
  issue.
- "Just in case" `rrd_collector_started()` calls in proc/windows plugin
  threads: made visible by D5's rename; per-thread verifiable deletions →
  follow-up issue.
- Availability-driven ACLK arms stay direct calls (pluginsd_parser.c:1532-1536,
  stream-receiver.c:1352, rrdhost.c:925) — not registry mutations.

## Ordered steps (each one commit on `refunc`, independently buildable/tested)

0. **Baseline**: record `netdata -W unittest` pass state (dyncfg,
   verify-access, manifest, manifest-pacer, mcp-function-access); build; live
   smoke (netdata-streaming fn, plugin fn, config listing).
1. **C1** registry + acquired handle (+ unittest migration; comment fixes for
   rrdhost.h:358 copy-paste, sqlite_aclk.c:1406/sqlite_aclk_node.c:182 use the
   new names — they reference contracts renamed HERE).
2. **C2** source enum + dyncfg enforcement + shim deletion (D1) + NEW negative
   test: COLLECTOR registering "config" (and " config") is rejected.
   (Add-signature churn includes rrdfunctions-unittest.c:40,45,49,229.)
3. **C3** delete-path queue + NEW tests: add/del truth table (arm/flag/
   FUNCTION_DEL conditions incl. the dyncfg-del flag quirk), drain-order, AND
   the concurrent del-during-drain race (no lost DEL under the
   insert-before-flag / clear-flag-before-snapshot protocol).
4. **C5a** broker named type (mechanical; header fix; unittest call sites).
5. **C7** drain-loop swap + pluginsd-only teardown reorder (D7 approved —
   note: narrows UAF-A's plugin path only; streaming UAF-A/C/D close in
   step 6).
6. **C6** transport (two-refcount model, per-entry ownership tag, DYNCFG-node
   pin) + UAF-A/B/C/D fixes + dyncfg-path threading + defer-destroy release
   (forced 503) + small bugs. NEW tests: RESULT_BEGIN without END + parser
   destroy ⇒ caller gets 503 (via the forced code), no deferred-dictionary
   shutdown warning (ASAN run); mixed-source re-registration
   (COLLECTOR-over-INTERNAL and back) neither crashes nor leaks; equal-
   pointer conflict nets zero refs; GC-del-mid-defer followed by RESULT_END
   delivers at result_end (the detach-primitive test); dyncfg node
   same-parser re-CREATE (equal cb/data) nets zero pins; ASAN build
   exercising the function paths.
7. **C5b** deadline accessor + effective-deadline helper + doc update
   (+ NEW test: extension-on-progress visible through the accessor).
8. **C4** iteration API + consumer migration — golden-output test for BOTH
   streaming emitters written FIRST against the post-C3 emitters (which
   already carry the drain), then the rewrite must reproduce it
   byte-identically; fixtures include the FUNCDEL quirk and the dyncfg-count
   (`dyncfg_add_streaming`) cases.
9. **D5** provider rename (mechanical, if approved).

Rationale: registry contract stabilizes first (1-3); lifetime fixes mid-chain
(5-7) where the GHSA/dyncfg unittests catch regressions; the widest consumer
churn (8) lands on a stable contract; cosmetic rename last.

## Validation (per step definition-of-done)

Full build (netdata-build MCP, worktree build/); the five unittests above;
live agent exercising an inline function, a plugin function (systemd-journal
if present), dyncfg config listing, and the functions listing; parent/child
streaming smoke via two declared agents if the MCP tooling permits (else
recorded); reference searches re-run before steps 1, 2, 3, 6, 8
(`->functions\b`, `functions_view`, `RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED`,
`stop_monotonic_ut`, `rrd_function_del(`, `stream_send_function_del`,
`inflight.functions`, `execute_cb_data`, `rrd_function_is_available`,
`rrd_functions_find_by_name`); ASAN build for step 6; implementation-time
check that all parser threads are nd_thread-registered (shutdown ordering).

## Open decisions for the user

- **D1 — DECIDED (b) delete** (2026-08-16, see decisions.md).
- **D3 — DECIDED (a) one PR per step** (2026-08-16, see decisions.md).
- **D4 — DECIDED (a) dictionary-behind-API** (2026-08-16, see decisions.md).
- **D5 — DECIDED (a) rename, own commit, last step** (2026-08-16, see
  decisions.md).
- **D6 — DECIDED (a) document + tracked issue** (2026-08-16, see
  decisions.md).
- **D7 — DECIDED (a) transport + pluginsd reorder** (2026-08-16, see
  decisions.md).
