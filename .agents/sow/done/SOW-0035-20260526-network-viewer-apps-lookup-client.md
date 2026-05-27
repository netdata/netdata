# SOW-0035 - Step 3: network-viewer.plugin APPS_LOOKUP client

## Status

Status: completed

Sub-state (phase-2 takeover 2026-05-27): implementation started after SOW-0033, SOW-0034, and SOW-0037 were completed. The SOW text was sanitized for durable artifact rules before coding.

Sub-state: revised after round-11 review (2026-05-26). Round-11 vote: 4 READY (reviewer-a, reviewer-b, reviewer-e, reviewer-d-partial) + 2 NOT READY (reviewer-f, reviewer-c). Both NOT-READY reviewers flagged genuine correctness gaps. ROUND-11 FIXES APPLIED:

(r11-MAJOR-1, reviewer-c BLOCKING — Contracts 1 & 2 violation) Worker pseudocode Phase 4 only checked the OUTER `status` field of APPS_LOOKUP responses (`NIPC_PID_LOOKUP_KNOWN` vs `NIPC_PID_LOOKUP_UNKNOWN`) and ignored the INNER `cgroup_status` field (`nipc_apps_cgroup_status_t` at `netipc_protocol.h:128-133`, embedded in `nipc_apps_lookup_item_view_t` at `netipc_protocol.h:539-554`). The wire format permits `outer=KNOWN` with `inner ∈ {KNOWN, UNKNOWN_RETRY_LATER, UNKNOWN_PERMANENT, HOST_ROOT}` (validated at `netipc_protocol.c:1965-2013`); SOW-0034:132-136 specifies that apps.plugin returns `inner=UNKNOWN_RETRY_LATER` when the `cgroup_cache` link is missing. Caching such responses violated Contract 1 (producer signals `UNKNOWN_PERMANENT` → consumer must EVICT) and Contract 2 (`UNKNOWN_RETRY_LATER` → consumer must NOT cache; asks again next iteration). Worker pseudocode Phase 4 now branches on `cgroup_status`: `UNKNOWN_PERMANENT` evicts any existing cache entry and decrements `cache_size`; `UNKNOWN_RETRY_LATER` is a no-op (do not insert, do not update existing); `KNOWN` and `HOST_ROOT` follow the existing insert/update/PID-reuse paths. Telemetry gains a `cache_evictions_unknown_permanent` counter so SREs see Contract-1 evictions distinctly from LRU/PID-reuse/generation-bump evictions. Contract-1/2 compliance note added under Architecture.

(r11-MAJOR-2, reviewer-f BLOCKING — `functions_evloop_cancel_threads` does not join) `src/libnetdata/functions_evloop/functions_evloop.c:392-396` only calls `nd_thread_signal_cancel` on the reader thread and on each worker; it never `nd_thread_join`s. Worker callbacks executing `j->cb(...)` at `functions_evloop.c:134` can still be inside the Function handler — holding `apps_lookup_cache_mutex` or `apps_lookup_intake_mutex` — when the main thread proceeds to step 11(d)-(h) and destroys those mutexes / the cache. Use-after-free / undefined behaviour on clean shutdown. SOW step 11(a) was claiming "cancel and join" but only cancel happened. **FIX (Option A — chosen)**: add an explicit join helper to the evloop library, `functions_evloop_join_threads(struct functions_evloop_globals *wg)`, that calls `nd_thread_join(wg->reader_thread)` then `nd_thread_join(wg->worker_threads[i])` for each `i ∈ [0, wg->workers)`. The struct is opaque (defined in `functions_evloop.c:42-66`, not in `.h`), so direct field access from network-viewer is not possible — the helper MUST live in the evloop library. Step 11(a) split into 11(a1) `functions_evloop_cancel_threads(wg)` and 11(a2) `functions_evloop_join_threads(wg)`, with both invoked BEFORE step 11(b) (signal worker eventfd) so the Function workers cannot observe a torn-down cache or intake. Rationale for not falling back to Option B (sleep): a 100ms yield only narrows the race; it does not eliminate it, and it adds a fixed shutdown latency cost. The Option A helper is a 5-line addition to the evloop library — small upstream change, low risk. A Followup item is added in case the netipc/evloop maintainer prefers to land the helper in a separate plumbing PR ahead of SOW-0035 implementation.

(r11-MAJOR-3, reviewer-f BLOCKING — `cache_size` scope under-specified) Phase 4's `dictionary_flush` + `cache_size = 0` sequence is correct only because `cache_size` is worker-private. SOW now states this explicitly under Architecture and at the `cache_size` declaration in step 1: `cache_size` is read/written EXCLUSIVELY by the background worker thread under `apps_lookup_cache_mutex`. Function handlers never access it. This invariant is what makes the `dictionary_flush` + `cache_size = 0` sequence correct (no other reader observes the stale interval); if a future SOW adds a concurrent cache writer, the flush+reset sequence must be revisited.

(r11-MINOR-1, reviewer-f) Citation `netipc_service.h:197-198` is for the cgroups-snapshot call's view lifetime contract, not the apps-lookup call. The same borrowed-buffer convention applies to all typed `nipc_client_call_*` functions, but the SOW should say so. Citation reworded throughout to: "per the lifetime contract documented at `netipc_service.h:195-198` for the cgroups-snapshot call; the same borrowed-buffer convention applies to all typed `nipc_client_call_*` functions, including `nipc_client_call_apps_lookup` (line 220)."

(r11-MINOR-2, reviewer-f) Worker thread name `nv-applkup` API choice now documented inline: `nd_thread_create("nv-applkup", 0, nv_apps_lookup_worker_main, NULL)`. The tag is stored internally up to `NETDATA_THREAD_TAG_MAX` characters; the OS-level name is `pthread_setname_np` truncated to 15 characters.

(r11-MINOR-3, reviewer-f) LRU eviction site `cache_size` race — added a code-comment site note (echoed in this SOW): `cache_size` is worker-private; no concurrent insert path exists today; if a future SOW adds one, this scan needs to be re-evaluated.

PRIOR ROUND-10 sub-state (preserved for traceability):

Revised after round-10 review (2026-05-26). Round-10 reviewer-c voted NOT READY with 1 MAJOR + 2 MEDIUM + 2 LOW; the other four reviewers (reviewer-a, reviewer-f, reviewer-b, reviewer-d) voted READY. ROUND-10 FIXES APPLIED:

(r10-MAJOR-1) Binary DICTIONARY keys would have triggered debug-log spam AND undefined behaviour (and outright rejection in non-debug builds for many PIDs). Verified `src/libnetdata/dictionary/dictionary.c:516-550` (`api_is_name_good_with_trace`): (a) in production builds, line 524's `if(!*name)` REJECTS keys whose first byte is `\0` — e.g. on little-endian, any PID that is a multiple of 256 (256, 512, ..., 65536, ...) would be silently dropped because the low-byte-first uint32 starts with `\0`; (b) in debug builds, lines 531-547 unconditionally evaluate `strlen(name)` inside `dictionary_internal_error`, which is UB on 4-byte binary PIDs that are not NUL-terminated. All existing callers of the `_advanced` API (`weights.c:148`, `rrdcalc.c:423`, `health_prototypes.c`, `api_v2_contexts_alerts.c`) pass STRING keys with string lengths. There is no precedent for raw binary keys in libnetdata DICTIONARY. SOW switched to **stringified PID keys** — `char key[16]; snprintfz(key, sizeof(key), "%u", pid); dictionary_set(apps_lookup_cache, key, &entry, sizeof(entry));`. The "avoiding the per-PID snprintf cost" optimisation rationale (Architecture bullet m4) is removed — for ≤8192 entries the snprintf cost is on the order of microseconds and is negligible relative to the cache-mutex hold time and the IPC round-trip in the worker. Architecture bullet m4 and Implementation plan step 1 updated accordingly.

(r10-MEDIUM-2) `dictionary_flush` (`dictionary.c:644-661`) deletes every item via `dict_item_del` (fires registered delete-callback), but does NOT touch the SOW's externally-maintained `uint32_t cache_size` counter. After `dictionary_flush(apps_lookup_cache)` the worker MUST set `cache_size = 0` explicitly. Worker pseudocode Phase 4 updated.

(r10-MEDIUM-3) The two SOW-0035-local histograms (`worker_request_duration_ms`, `function_handler_overhead_ms`) are noted as candidates for renaming/folding into a shared Contract 6 telemetry surface in a future revision once other clients (cgroups.plugin, network-viewer, future consumers) have shipped and the cross-client overhead-vs-IPC split pattern is observable. One-line note added in the Telemetry section.

(r10-LOW-4) `eventfd_create` failure path made explicit. Implementation plan step 1 and the Risk Register now state: if `eventfd(...)` fails at startup, the plugin logs FATAL and exits. Rationale: the worker architecture mandates the eventfd as its sole signalling mechanism between Function handlers and the background worker; a pipe-based fallback would force a second drain loop and double the file-descriptor budget for no operational benefit, and `eventfd` failures are only expected under exhaustion conditions (FD limit, kernel without `CONFIG_EVENTFD`) where the daemon respawning the plugin is the right outcome.

(r10-LOW-5) `last_seen_pids` bounded eviction semantics — round-8 accepted bounded staleness. Phase-2 implementation strengthens this by refreshing an existing PID's sequence on every Function-handler sighting, so the set evicts the least-recently-observed entry rather than a still-active PID that was inserted early.

PRIOR ROUND-9 sub-state (preserved for traceability):

Revised after round-9 review (2026-05-26). Round-9 reviewer-c voted NOT READY with 1 MAJOR + 1 MEDIUM + 3 MINOR; round-9 reviewer-a added 1 NIT on the worker-exit FATAL gating. ROUND-9 FIXES APPLIED:

(r9-MAJOR-1; r11-MINOR-1 citation fix) Worker pseudocode Phase 5 no longer calls the non-existent `nipc_client_release_view(view_out)`. There is no such function in `src/libnetdata/netipc/include/netipc/netipc_service.h`. The `view_out` strings are borrowed from the context's internal recv buffer and remain valid only until the next call on the same context, per the lifetime contract documented at `netipc_service.h:195-198` for the cgroups-snapshot call; the same borrowed-buffer convention applies to all typed `nipc_client_call_*` functions, including `nipc_client_call_apps_lookup` (line 220). Phase 5 is now an explicit comment documenting this — no release call, the next `nipc_client_call_apps_lookup` on `apps_lookup_client_ctx` implicitly invalidates the view.

(r9-MEDIUM-1) "UNKNOWN → entry evicted" prose tightened. UNKNOWN does NOT evict an existing cache entry for a PID no longer in the working set — such an entry survives until generation bump, LRU overflow, or PID-reuse detection. UNKNOWN only prevents caching for the PID currently in the intake. AC "Cache lifetime" paragraph and Inferences bullet now state this accurately.

(r9-MINOR-1) "lock-free intake queue" terminology corrected to "mutex-protected intake set" — the intake is explicitly protected by `apps_lookup_intake_mutex`. The earlier "lock-free" phrasing in the round-6 sub-state and the Purpose paragraph was inaccurate.

(r9-MINOR-2) Worker pseudocode Phase 3 now shows the explicit error branch for `nipc_client_call_apps_lookup`: on non-`NIPC_OK`, increment `requests_failed`, release `client_mutex`, and `continue` the worker loop. Drained PIDs are lost from intake; the next Function call's miss-scan re-enqueues them per Contract 5 fallback semantics.

(r9-MINOR-3) `nipc_client_init` call site is now explicit: invoked from `main()` BEFORE starting the worker thread, populating the file-scope `apps_lookup_client_ctx`. The worker starts with the context already initialised; only connect/reconnect happens inside the worker loop.

(r9-NIT-1, reviewer-a round-9) Worker sets `worker_thread_exited = true` on BOTH the unrecoverable-error path AND the clean-shutdown path (when `plugin_should_exit` was already observed). The keepalive-loop check in step 11 now gates the FATAL log on `!plugin_should_exit` so a clean shutdown does not emit a spurious "worker exited unexpectedly" FATAL. Pseudocode also clarifies the single write site and the gated check.

PRIOR ROUND-8 sub-state (preserved for traceability):

Revised after round-8 review (2026-05-26). Round-8 reviewer-a voted PRODUCTION-GRADE conditional on TWO LOW spec-hygiene items (architecture is sound; both items are specification cleanups, not architectural defects). ROUND-8 FIXES APPLIED:

(r8-LOW-1, contradiction 5.1 in round-8 review) `cleared_pids_vector` removed from AC line 90 prose and from Phase 4 of the worker pseudocode. Chose Option A (simpler): the worker re-enqueue path after a generation bump merges `apps_lookup_last_seen_pids` ONLY into intake — no separate `cleared_pids_vector` snapshot of cache keys. Rationale recorded inline at AC line 90: `last_seen_pids` is a superset of recently-observed working-set PIDs because every Function handler records its full deduplicated working set under `intake_mutex`. The bounded staleness edge case (a PID warmed by an early Function call, still in cache, evicted from `last_seen_pids` before the bump because both are capped at 8192) is accepted — such a PID is by definition among the least-recently-observed in the working set and is re-warmed by the next Function call that includes it. Avoids an extra O(n) cache-key snapshot inside the cache critical section. Pseudocode Phase 4 now initialises `generation_bumped = false`, drops the stale "see below" comment on `dictionary_flush`, and notes that Phase 6's `last_seen_pids` merge is the re-enqueue path. Implementation plan step 4 already aligns (only `last_seen_pids` snapshot).

(r8-LOW-2, contradiction 5.2 in round-8 review) Worker-crash sentinel `worker_thread_exited` (Analysis line 274) now reflected in Implementation plan. Step 1 declares `_Atomic bool worker_thread_exited` file-scope state initialised to `false`. Step 4 (worker lifecycle) specifies the SINGLE write site: the worker sets it to `true` (atomic store, release semantics) immediately before returning. Step 11 specifies the keepalive-loop check: each iteration of `main`'s `while(!plugin_should_exit)` loop loads the sentinel with acquire semantics and, on observation, logs a FATAL line and sets `plugin_should_exit = true` to trigger clean shutdown and daemon respawn — rather than leaving Function handlers serving stale-cache data with no cache warming for an unbounded period.

PRIOR ROUND-7 sub-state (preserved for traceability):

Revised after round-7 review (2026-05-26). Round-7 reviewers (reviewer-c NOT READY: 2 MAJOR + 3 MEDIUM + 2 LOW; reviewer-a PRODUCTION-GRADE conditional on W2/W3) flagged the `last_seen_pids` write/bound gap plus several specification cleanups. ROUND-7 FIXES APPLIED:

(r7-MAJOR-1) `last_seen_pids` write path is now explicitly specified: **Function handlers populate `last_seen_pids` with the full deduplicated working-set PIDs** (not just misses) under `apps_lookup_intake_mutex`, immediately after the intake-enqueue step inside the same critical section (avoids extra lock/unlock cycle). This guarantees that on a generation bump the worker has a complete recent-working-set snapshot to re-enqueue.

(r7-MAJOR-2 / reviewer-a W2) `last_seen_pids` is bounded at the same cap as `apps_lookup_cache` (default 8192). On overflow during Function-handler population, the least-recently-observed entry is dropped (tracked via a sequence refreshed on duplicate sightings). The worker re-enqueue path moves OUT of `apps_lookup_cache_mutex` scope: the worker (a) under `cache_mutex` collects the local PID list to clear and clears the cache; (b) releases `cache_mutex`; (c) acquires `intake_mutex`, snapshots `last_seen_pids` into a local vector, merges into intake (deduped, bounded), releases `intake_mutex`; (d) loops to drain the merged intake. This preserves the "cache_mutex and intake_mutex never held simultaneously" invariant.

(r7-MEDIUM-1) Cache entry destructor specified: `apps_lookup_cache` is registered with a DICTIONARY `react_callback`/delete callback (`nv_apps_lookup_cache_entry_destroy`) that `freez()`s `cgroup_path`, `cgroup_name`, and `cgroup_labels`. Invoked on LRU eviction, PID-reuse eviction, generation-bump clear, and plugin-shutdown `dictionary_destroy`. Prevents heap leaks under churn.

(r7-MEDIUM-2) Eventfd vs condvar ambiguity resolved: **eventfd** is chosen explicitly. Rationale: network-viewer is Linux-only (matches existing plugin posture); eventfd coalesces multiple signals into one wake (matching the worker's "drain the whole intake" semantics); integrates with `poll`/`epoll`/`select` so the worker can wait on the intake-signal AND a refresh timer (`timerfd_create` or `poll` timeout) in one syscall. All "eventfd or condvar" prose is replaced with "eventfd".

(r7-MEDIUM-3) `functions_evloop_cancel_threads(wg)` added to the shutdown sequence in step 11, BEFORE signalling/joining the background worker and BEFORE destroying shared state. Order: (a) signal plugin_should_exit; (b) `functions_evloop_cancel_threads(wg)` to stop the 5 Function-evloop workers so no Function handler is mid-cache-read; (c) signal worker eventfd, join worker; (d) destroy cache, intake, eventfd, client, mutexes.

(r7-LOW-1) `cache_size` is plain `uint32_t`, not `_Atomic`. All reads/writes happen under `apps_lookup_cache_mutex`; the atomic was redundant. (Alternative `dictionary_entries(apps_lookup_cache)` would also be acceptable but adds an indirection per insert path.)

(r7-LOW-2) PID-reuse correctness within a generation clarified: the SOW already documents the bounded-staleness trade-off (L2, lines 117-122). Added a one-line cross-reference at the top of the cache-key description so reviewers/implementors don't expect `(pid, starttime)` composite keying.

(r7 reviewer-a W3) The Function-handler-side step list (lookup integration) now explicitly states `last_seen_pids` population under `apps_lookup_intake_mutex`, with the cap and least-recently-observed eviction policy.

(r7 reviewer-a W4) Operator-facing chart description for `cache_misses_unknown` now notes that a small steady-state UNKNOWN rate may originate from refresh-probe lookups hitting LRU-residual PIDs.

PRIOR ROUND-6 sub-state (preserved for traceability):

Sub-state from round-6 final-reviewer review (2026-05-26). final-reviewer flagged THREE CRITICAL findings that were architecturally incompatible with the round-5 design (synchronous-in-Function APPS_LOOKUP), plus THREE MAJOR findings. ROUND-6 FIXES APPLIED:

(r6-CRITICAL-1) Replaced synchronous in-Function APPS_LOOKUP with **async cache-warming**: Function handlers NEVER perform IPC. They (a) collect the working-set PIDs, (b) serve whatever cache state exists (warm hits enrich, misses emit unenriched rows just like today), (c) enqueue uncached/stale PIDs into a mutex-protected intake set (r9-MINOR-1 — protected by `apps_lookup_intake_mutex`, not lock-free). A dedicated **background lookup worker thread** drains the set, performs APPS_LOOKUP, and writes results into the cache. Function handler overhead becomes constant-time (cache lookups under cache_mutex only) — no IPC latency on the Function path, period. Resolves the Cloud/UI latency degradation.

(r6-CRITICAL-2) Since the only thread that takes `apps_lookup_client_mutex` is the background worker, the previous "5 workers serialise on client_mutex" failure mode is impossible. Function handlers NEVER acquire `apps_lookup_client_mutex`. A slow/hung peer stalls ONLY the background worker thread; the 5 Function workers and the `main` keepalive thread are completely unaffected.

(r6-CRITICAL-3) Generation refresh is now driven by the background worker on a **periodic timer** (default 30s, configurable `[plugin:network-viewer] apps lookup generation refresh seconds`). Even when every working-set PID is a cache hit and the intake queue is empty, the worker still issues a small probe APPS_LOOKUP request (e.g., the lowest-PID cache entry or an empty key set if the protocol allows) at least every refresh interval. On observing a higher response generation, the worker clears the cache and re-enqueues the most-recent working set captured by Function handlers (tracked via a separate `last_seen_pids` set, also drained by the worker).

(r6-MAJOR-4) Cache-key decision recorded **explicitly** as PID-only with user-visible rationale. The L2 decision now reads: cache is keyed by PID; bounded staleness is accepted within a single apps.plugin generation; PID-reuse correctness is guaranteed only across generation bumps (default 1s). Reading `/proc/<pid>/stat` field 22 in network-viewer to capture local starttime BEFORE cache lookup was considered and explicitly REJECTED in this SOW: it is significant work (new `/proc` reader, new error paths, defeats the cache-first design by adding a `/proc` syscall per PID per Function call), and the staleness window is sub-second. If operators later demand tighter PID-reuse correctness, a follow-up SOW will add local-starttime capture as a separate, measurable change.

(r6-MAJOR-5) Detailed-mode (`sockets:detailed`) language is now consistent throughout. Removed all phrasing that implied "every PID seen in a network connection is enriched in SOW-0035". Replaced with "every PID seen in a network connection via the topology Function or via the aggregated sub-mode of the network-connections Function". The Purpose paragraph, the Acceptance Criteria, the Implementation plan, and the Followup section all align: detailed-mode is SOW-0036 work.

(r6-MAJOR-6) `pkill -9 apps.plugin` replaced with a **targeted-PID procedure** in the fallback test: `pgrep -x apps.plugin` to discover candidate PIDs; verify each is a `netdata`-owned `apps.plugin` (`ps -o user= -p <pid>` and `readlink /proc/<pid>/exe`); send `SIGKILL` only to the verified PID. Same targeted-PID pattern for SIGSTOP/SIGCONT.

Net effect on SOW: the architecture flipped from "synchronous Function-path IPC + 5-worker serialisation mitigation" to "async cache-warming + dedicated background worker", which makes the Function-path latency story straightforward and removes the worker-stall mitigation entirely from Function-facing code paths.

## Requirements

### Purpose

Make `network-viewer.plugin` an APPS_LOOKUP client of apps.plugin. For each PID seen in a network connection **by the topology Function or by the aggregated sub-mode of the network-connections Function** (detailed sub-mode `sockets:detailed` is out of scope — deferred to SOW-0036), an asynchronous background worker queries APPS_LOOKUP to obtain per-PID enriched metadata (cgroup, orchestrator, container name, labels). Cache results keyed by PID. Function handlers NEVER perform IPC — they only read the cache and enqueue uncached PIDs into a background-worker **mutex-protected intake set** (r9-MINOR-1; protected by `apps_lookup_intake_mutex`). No user-visible output changes in this SOW — that comes in SOW-0036. This SOW just brings the data INTO network-viewer.

After this SOW completes, the end-to-end IPC chain (cgroups → apps → network-viewer) is alive. The next SOW (SOW-0036) turns the data into user-visible topology groupings.

### User Request

See master plan SOW-0032 for the full directive. This SOW implements step 3 of six.

### Assistant Understanding

Facts (corrected after round-1 reviewer feedback, verified 2026-05-26):

- `network-viewer.plugin` is a binary (`/usr/libexec/netdata/plugins.d/network-viewer.plugin`) with caps `cap_dac_read_search,cap_sys_admin,cap_sys_ptrace`.
- Source: `src/collectors/network-viewer.plugin/network-viewer.c` (4622 lines).
- **network-viewer is Function-driven, NOT iteration-driven.** `main()` at `network-viewer.c:4587-4616` registers two Functions (`network-connections`, `topology`) via `functions_evloop_add_function()` and enters a keepalive loop that only sends newlines. There is NO periodic connection enumeration. All connection enumeration happens INSIDE the Function handlers when invoked by the daemon.
- `network_viewer_topology_function()` at `network-viewer.c:3936-3960` calls `topology_prepare_context()` which calls `local_sockets_process()` (`network-viewer.c:1420`) — connection enumeration is on-demand per Function call.
- `topology_prepare_context()` (`network-viewer.c:1354-1422`) creates EPHEMERAL dictionaries (`process_actors`, `links`, etc.) destroyed by `topology_context_destroy()` (`network-viewer.c:1332-1352`) after the response is written. Per-Function-call data is transient; any persistent cache must live OUTSIDE this context.
- It serves Function output (the topology table) to Netdata Cloud / local UI. The Function response shape is the API contract that SOW-0036 will extend.
- network-viewer's existing per-PID metadata (`LOCAL_SOCKET`, `NV_PROCESS_ACTOR`) reads `comm`, `cmdline`, `uid`, `ppid`, `net_ns_inode` per `local-sockets.h:584-737` — but **does NOT read `/proc/<pid>/stat` and has no `starttime` field** today.

Inferences and resolved decisions:

- The "working set" of PIDs is established INSIDE each Function call (via `local_sockets_process()`). The persistent APPS_LOOKUP cache lives OUTSIDE the per-call topology context — at file scope in `network-viewer-apps-lookup-client.c`, with its own mutex.
- **APPS_LOOKUP runs ASYNCHRONOUSLY in a dedicated background worker thread** (round-6 architecture flip). Function handlers (a) take the cache mutex, (b) look up each working-set PID, (c) emit cached fields when present (warm hit), emit unenriched rows when absent (cold miss — same as today, no behavioural change), and (d) push uncached/stale PIDs into a mutex-protected intake set protected by `apps_lookup_intake_mutex` (r9-MINOR-1 — not lock-free). The background worker drains the intake set, performs batched APPS_LOOKUP requests, and writes results into the cache. **Function-handler latency contains NO IPC** — only cache reads under `apps_lookup_cache_mutex` and bounded enqueue operations under `apps_lookup_intake_mutex`. The worker is the SOLE holder of `apps_lookup_client_mutex` and the SOLE caller of `nipc_client_call_apps_lookup`.
- **Cache key resolution (the chicken-and-egg)**: on first contact with a PID, network-viewer has only `pid` (not starttime). The cache is keyed by `pid`; the cache entry stores the `starttime` returned in the APPS_LOOKUP response. On subsequent lookups, the worker compares the response's `starttime` against the cached `starttime`; on mismatch, the cached entry is evicted and replaced (PID reuse detected at the next worker pass). Function handlers never compare starttimes themselves — they only see the cache snapshot the worker has produced. No need for network-viewer to read `/proc/<pid>/stat` itself — apps.plugin owns starttime, network-viewer trusts what it gets. See L2 for the explicit PID-only decision.
- Cache lifetime (r9-MEDIUM-1 — tightened): an existing cache entry is evicted by exactly three paths — (a) generation bump (Contract 4) → full cache clear, (b) LRU overflow → single least-recently-used entry evicted, (c) worker detects PID reuse via cached-vs-response `starttime` mismatch → entry evicted and replaced. An `UNKNOWN` response from apps.plugin does NOT evict an existing cache entry: a previously-cached entry for a PID that has since left the working set is no longer in the intake, so the worker never re-asks about it; it survives until generation bump, LRU, or PID-reuse. `UNKNOWN` only prevents caching for PIDs currently in the intake (status = UNKNOWN → no insert; the next Function call's miss-scan re-enqueues if still in the working set). Per Contract 3, PIDs that leave the network connection set naturally fall out of the cache through LRU within the bounded size.
- Cache size: bounded LRU max 8192 entries (matches typical "unique PIDs network-viewer has ever seen across recent Function calls"); evicted entries are simply re-queried on next encounter.
- Batching: one APPS_LOOKUP request per worker drain pass, capped at 8192 PIDs to match the APPS_LOOKUP server limit (`APPS_LOOKUP_MAX_PIDS_PER_REQUEST`). The intake set can hold 16384 PIDs; any remainder stays queued and is drained by the next worker pass. The lower server cap wins over the raw `NIPC_MAX_PAYLOAD_CAP` byte-cap calculation.
- PIDs not yet in apps.plugin's state (apps.plugin enumerates `/proc` at its own cadence; network-viewer may see a PID a few seconds before apps.plugin does): apps.plugin returns `status = UNKNOWN` → background worker caches NOTHING for this PID. On the next Function call, the PID is still uncached and the handler re-enqueues it, so the worker re-queries on its next drain. SOW-0037's targeted refresh signal does NOT extend to apps.plugin (per SOW-0037 L3), so the lag is bounded by apps.plugin's natural collection cadence.
- Function-call latency target: APPS_LOOKUP integration adds <5ms p95 to Function handler overhead in ALL cache states (cold or warm). Cold cache means rows are emitted unenriched — the Function still returns immediately. Worker cadence determines how quickly cold misses become warm hits for subsequent Function calls (target: <1s under typical load).
- Worker cadence: the background worker wakes on intake-queue signal (eventfd — see L9) AND on a periodic timer (`poll` timeout, default 30s, configurable `[plugin:network-viewer] apps lookup generation refresh seconds`). The timer guarantees generation refresh even when the intake queue stays empty (warm working set with no churn).

### Acceptance Criteria

**Architecture (corrected per round-2 reviewer feedback):**

- network-viewer.plugin maintains a **process-lifetime cache** at file scope in `network-viewer-apps-lookup-client.c` (separate from the ephemeral `NV_TOPOLOGY_CONTEXT`). Cache is a `DICTIONARY *apps_lookup_cache` keyed by PID (uint32 → cache entry). Cache entry stores: `starttime`, `cgroup_status`, `orchestrator`, `cgroup_path`, `cgroup_name`, `cgroup_labels`, `apps_lookup_generation_observed`, `last_used_usec` (for LRU).
- **Async cache-warming architecture (round-6 redesign, r7-clarified)**: the system has THREE mutex-protected state pieces and FOUR thread roles. State: (a) `apps_lookup_cache` (bounded LRU, max 8192 entries) protected by `apps_lookup_cache_mutex`, with a registered DICTIONARY delete-callback (`nv_apps_lookup_cache_entry_destroy`) that `freez`es heap-owned strings (cgroup_path, cgroup_name, cgroup_labels) on every removal path (LRU, PID-reuse, generation-bump clear, `dictionary_destroy` on shutdown); (b) `apps_lookup_intake` (a bounded recent-sequence set of PIDs awaiting lookup, max 16384 entries) PLUS `apps_lookup_last_seen_pids` (a bounded recent-sequence set of recently-seen working-set PIDs, max 8192 entries — same cap as the cache — used by the worker for generation-bump re-population), BOTH protected by `apps_lookup_intake_mutex` and signalled via an **eventfd** (`apps_lookup_intake_eventfd`, `EFD_NONBLOCK | EFD_CLOEXEC`, semaphore-less coalesced wakes); both sets refresh an existing PID's sequence on duplicate sighting and evict the lowest-sequence entry on overflow; (c) APPS_LOOKUP client connection protected by `apps_lookup_client_mutex`. Threads: (1) **main/keepalive** — writes stdout heartbeat only, touches none of the above; (2) **functions-evloop workers (5×)** — Function handlers; acquire `apps_lookup_cache_mutex` for cache reads, release it, then acquire `apps_lookup_intake_mutex` to (i) enqueue misses into `apps_lookup_intake` AND (ii) record the full working-set PIDs into `apps_lookup_last_seen_pids` (deduped, bounded), then signal the worker eventfd. They NEVER acquire `apps_lookup_client_mutex` and NEVER call `nipc_client_call_apps_lookup`; (3) **background lookup worker (NEW, 1 thread)** — the SOLE owner of `apps_lookup_client_mutex` and the SOLE caller of any netipc function. It waits on the intake eventfd via `poll(eventfd, timeout=refresh_interval_seconds)`, drains the intake set into a batched APPS_LOOKUP request, processes the response, writes the cache under `apps_lookup_cache_mutex`; (4) **plugin shutdown thread** — sets `plugin_should_exit`, signals the worker eventfd to wake it for clean exit.
- **DICTIONARY key shape** (m4 — round-10 r10-MAJOR-1 corrected): keys are **stringified PIDs** generated via `char key[16]; snprintfz(key, sizeof(key), "%u", pid);` and passed via the standard `dictionary_set(cache, key, &entry, sizeof(entry))` form. Binary 4-byte PID keys passed via the `_advanced` API were considered (to "avoid the per-PID snprintf cost") and REJECTED after verifying `src/libnetdata/dictionary/dictionary.c:516-550`: (a) `api_is_name_good_with_trace` line 524 rejects keys whose first byte is `\0` even in production builds (silently dropping any PID that is a multiple of 256 on little-endian); (b) lines 531-547 call `strlen(name)` unconditionally in debug builds, which is UB on non-NUL-terminated binary keys. There is also no precedent for binary DICTIONARY keys in the codebase (all existing `_advanced` callers — `weights.c:148`, `rrdcalc.c:423`, `health_prototypes.c`, `api_v2_contexts_alerts.c` — pass string keys with string lengths). The snprintf cost for ≤8192 entries is on the order of microseconds and is negligible relative to the cache-mutex hold time and the IPC round-trip in the worker. The cache `DICTIONARY` is created with `DICT_OPTION_SINGLE_THREADED` because the external `apps_lookup_cache_mutex` provides the only required serialisation — relying on libnetdata's per-dictionary internal lock would be redundant and double-locking.
- **APPS_LOOKUP client connection** lives at file scope; the background worker owns connect-once + retry-with-backoff per Contract 5. Function handlers never touch `nipc_client_ready` or `nipc_client_refresh` — those are worker-internal. Reconnect cadence is driven entirely inside the worker loop (refresh on disconnect detection; no separate timer in `main`).
- **Function-handler latency is bounded by mutex hold times, NOT by IPC** (round-6 critical change): a Function handler's APPS_LOOKUP integration cost is the sum of (a) one cache-mutex acquire + one dictionary lookup per working-set PID, and (b) one intake-mutex acquire + one bounded enqueue per cache miss, plus an eventfd write to wake the worker. No `recv`, no syscall to apps.plugin, no waiting on apps.plugin. A slow/hung peer affects ONLY the background worker thread; the 5 Function workers continue at cache-snapshot speed and the daemon-side per-Function 10s deadline is never reached because of IPC.
- **Mutex ordering (r7-clarified; r8-simplified)**: `apps_lookup_cache_mutex` and `apps_lookup_intake_mutex` are NEVER held simultaneously, anywhere, by any thread. The Function handler pattern is: lock cache → for each PID, look up → if miss, remember PID locally → unlock cache → lock intake → enqueue all misses into `apps_lookup_intake` AND record all working-set PIDs into `apps_lookup_last_seen_pids` (deduped, bounded with duplicate sightings refreshing sequence) → unlock intake → signal worker eventfd. The background worker pattern is: lock intake → snapshot up to 8192 PIDs into local `drain_vector`, remove those entries from intake, leave any remainder queued and re-signal the eventfd → unlock intake → lock `apps_lookup_client_mutex` → perform `nipc_client_call_apps_lookup(drain_vector)` → lock `apps_lookup_cache_mutex` (nested INSIDE client_mutex; documented inner mutex) → if generation bumped: clear the cache, advance `last_observed_generation`, set local `generation_bumped = true` → apply response items (insert/update/evict) → unlock `apps_lookup_cache_mutex` → unlock `apps_lookup_client_mutex` → **if generation_bumped**: lock intake → merge a snapshot of `apps_lookup_last_seen_pids` into intake (deduped, bounded) → unlock intake → loop continues (next iteration drains the merged intake). The worker NEVER holds cache_mutex and intake_mutex simultaneously; the only nesting in the worker is `client_mutex` ⊃ `cache_mutex` for the response application phase. Function handlers never touch client_mutex. **Rationale for last_seen_pids-only re-enqueue (r8 option A)**: `last_seen_pids` is a superset of recently-observed working-set PIDs (every Function handler records its full deduplicated working set under intake_mutex), so it is sufficient for re-warming after a generation bump. The bounded staleness edge case — a PID warmed by an early Function call, still in the cache, but evicted from `last_seen_pids` (both capped at 8192) before the bump — is accepted: such a PID is by definition among the least-recently-observed in the working set and will be re-warmed by the next Function call that includes it. This avoids an extra O(n) cache-key snapshot inside the cache critical section.
- **`view_out` consumption pattern** (worker-only, was m5; r11-MINOR-1 citation fix): the worker iterates the borrowed `view_out` while holding `apps_lookup_client_mutex` (because the view's strings are "valid only until the next call on this context" per the lifetime contract at `netipc_service.h:195-198` for the cgroups-snapshot call; the same borrowed-buffer convention applies to all typed `nipc_client_call_*` functions, including `nipc_client_call_apps_lookup` at line 220) and acquires `apps_lookup_cache_mutex` INSIDE that scope to `strdup`/intern each label into cache-owned storage. After the iteration both mutexes are released. Function handlers never see `view_out` and never participate in this nesting.
- **Two cache-warming-trigger paths in SOW-0035** (round-6: "enrichment" reworded to "cache-warming" because Function output is unchanged in SOW-0035; helpers no longer perform IPC — they read the cache and enqueue misses):
  - **Topology Function** (`network_viewer_topology_function`, `network-viewer.c:3936`): calls `topology_prepare_context()` which calls `local_sockets_process()` with `local_sockets_cb_to_topology` (`network-viewer.c:1115` — body extends to ~1300, populates `ctx->process_actors` via `dictionary_set` at 1214). The callback populates `ctx->process_actors` (a Function-owned `DICTIONARY *` of `NV_PROCESS_ACTOR`) which SURVIVES `local_sockets_cleanup` because it lives in the per-call `NV_TOPOLOGY_CONTEXT`, not in `LS_STATE`. Helper `nv_warm_cache_from_topology_actors(ctx)` is called AFTER `topology_prepare_context()` returns and BEFORE `topology_write_data()` (`network-viewer.c:3956`). The helper enumerates unique PIDs in `ctx->process_actors` via `dfe_start_read`, looks each up in the file-scope cache, collects misses, enqueues them to the worker via `apps_lookup_intake_enqueue(pids, n)`. **No IPC, no waiting** — output proceeds immediately. **`NV_PROCESS_ACTOR` is NOT modified** in this SOW — the cache lives separately, consumed by SOW-0036.
    - **Comm-keyed mode caveat** (m2, forward-reference for SOW-0036): when `ctx->options.processes_by_pid == false` (`network-viewer.c:1188-1198`), actors are comm-keyed and `pa->pid` stores the FIRST PID seen — multiple distinct PIDs sharing a comm collapse into one `NV_PROCESS_ACTOR`. The cache will only see those first-seen PIDs in comm-keyed mode. Zero impact in SOW-0035 (cache is not read by output here). SOW-0036 must decide whether to (a) modify `NV_PROCESS_ACTOR` to carry a full PID set, or (b) add a side-channel PID set in `local_sockets_cb_to_topology` so all PIDs are enriched in comm-keyed mode.
  - **Network-connections Function — aggregated sub-mode ONLY** (`network_viewer_function`, `network-viewer.c:3967`, `sockets:aggregated`): callback `local_sockets_cb_to_aggregation` (`network-viewer.c:983`) buffers sockets into a Function-owned `SIMPLE_HASHTABLE_AGGREGATED_SOCKETS ht` (declared at `network-viewer.c:4088`). The aggregated entries are `mallocz`'d INDEPENDENTLY of `ls->local_socket_aral` at `network-viewer.c:1090-1093`, so they SURVIVE `local_sockets_cleanup`. The helper `nv_warm_cache_from_pids(pids[], n)` is called AFTER `local_sockets_process()` returns, BEFORE the output loop at `network-viewer.c:4118`. The caller iterates `ht` between lines 4099 and 4118, dedups PIDs into a uint32 vector, and invokes the helper. The helper looks each PID up in the cache, enqueues misses to the worker, and returns immediately. Output (`local_socket_to_json_array` loop at 4118-4122) remains untouched in SOW-0035.
  - **Network-connections Function — detailed sub-mode (`sockets:detailed`): DEFERRED to SOW-0036.** Rationale: callback `local_sockets_cb_to_json` (`network-viewer.c:972-976`) writes JSON INLINE during `local_sockets_process()` enumeration. After `local_sockets_process()` returns, it has internally called `local_sockets_cleanup(ls)` (`src/libnetdata/local-sockets/local-sockets.h:1856`), which destroys `ls->sockets_hashtable` (line 1293), `ls->pid_sockets_hashtable` (line 1290), and frees every `LOCAL_SOCKET` and `pid_socket` via `aral_destroy(ls->local_socket_aral)` (line 1295) / `aral_destroy(ls->pid_socket_aral)` (line 1296). There is NO post-`local_sockets_process()` PID source in the detailed path. The SOW's no-modify list (below) prohibits modifying `local_sockets_cb_to_json` to push PIDs into a side-channel set. The cache is never warmed for PIDs that appear ONLY through the detailed sub-mode in SOW-0035 — they will only appear in the cache if they also appear via the topology Function or the aggregated sub-mode (the typical case in practice). SOW-0036, which adds new columns to the detailed view, will decide between: (a) amending the no-modify list to permit a one-line non-output side-channel PID push inside `local_sockets_cb_to_json`; or (b) restructuring the detailed-view emission to a two-pass model (collect into Function-owned storage first, then enrich, then emit). SOW-0035 has zero behavioural dependence on this decision because the cache is not read by any output path here.
  - **Helper signatures** (round-6 renamed to reflect async cache-warming, not synchronous enrichment): `nv_warm_cache_from_topology_actors(NV_TOPOLOGY_CONTEXT *ctx)` for the topology Function; `nv_warm_cache_from_pids(const uint32_t *pids, size_t pid_count)` for the aggregated sub-mode of the network-connections Function. Both are non-blocking. The actual `nipc_client_call_apps_lookup` happens inside the background worker thread defined below.

**Lookup integration (round-6 — fully asynchronous):**

- **Function-handler-side (5 worker threads, hot path)** — runs inside each Function handler between `local_sockets_process()` (working-set producer) and output emission:
  1. Enumerate unique PIDs in the working set.
  2. Acquire `apps_lookup_cache_mutex`. For each PID: look it up; if present, copy needed fields (cgroup_status, orchestrator, cgroup_path, cgroup_name, cgroup_labels, observed_generation, starttime) into a Function-local buffer and update `last_used_usec`; if absent, append the PID to a local "miss" vector. Release the cache mutex.
  3. Acquire `apps_lookup_intake_mutex` (single critical section for both writes):
     - (a) If the miss vector is non-empty, add those PIDs to `apps_lookup_intake` (deduped against existing entries, refreshing sequence on duplicate). **Bounded enqueue**: if `apps_lookup_intake` is at its cap (default 16384), evict the lowest-sequence entry, increment `cache_misses_intake_dropped`; the next Function call will re-derive and re-enqueue.
     - (b) Add ALL working-set PIDs (the full deduplicated list from step 1, not just the misses) to `apps_lookup_last_seen_pids` (deduped against existing entries, refreshing sequence on duplicate; bounded at 8192 — same cap as the cache; on overflow, evict the lowest-sequence entry). This guarantees that on a future generation bump the worker has a complete recent-working-set snapshot to re-enqueue without needing to scan the cache or wait for the next Function call.
     - Release `apps_lookup_intake_mutex` and signal the worker eventfd.
  4. Emit Function output using the Function-local buffer captured in step 2. Misses emit unenriched fields (same shape as today's pre-SOW-0035 output) — no waiting, no IPC. **Function handler returns immediately after output.**

- **Background-lookup-worker-side (1 thread, dedicated)** — created at plugin startup, joined at shutdown. The pseudocode below preserves the strict mutex-ordering invariant: `apps_lookup_cache_mutex` and `apps_lookup_intake_mutex` are NEVER held simultaneously.
  ```
  while (!plugin_should_exit) {
      poll(intake_eventfd, timeout = refresh_interval_seconds)
      timer_fired = (poll returned 0)
      eventfd_read drains the eventfd counter

      // -- Phase 1: snapshot intake (cache_mutex NOT held) --
      lock intake_mutex
      drain_vector = move-out(apps_lookup_intake)
      unlock intake_mutex

      // -- Phase 2: pick probe PID if idle wakeup with empty drain --
      if (drain_vector empty AND !timer_fired) continue
      if (drain_vector empty AND timer_fired) {
          lock cache_mutex
          probe_pid = lowest-PID entry in cache (one read), or 0 if cache empty
          unlock cache_mutex
          if (probe_pid == 0) continue   // nothing to probe
          drain_vector = [probe_pid]
          worker_refresh_probes++
      }

      // -- Phase 3: IPC (client_mutex held) --
      lock client_mutex
      err = nipc_client_call_apps_lookup(&apps_lookup_client_ctx, drain_vector, n, &view_out)
      if (err != NIPC_OK) {
          // r9-MINOR-2: explicit error branch. PIDs are lost from intake;
          // the next Function call's miss-scan re-enqueues them (Contract 5).
          requests_failed++
          unlock client_mutex
          continue                       // back to top of while-loop
      }
      requests_responded++

      // -- Phase 4: apply response (client_mutex ⊃ cache_mutex; required because
      //    view_out strings are borrowed and only valid until the next IPC call) --
      lock cache_mutex
      generation_bumped = false
      if (response_generation > last_observed_generation) {
          // CRITICAL-3 fix: generation bump observed
          // Note: cache delete-callback (nv_apps_lookup_cache_entry_destroy)
          //       freez()s cgroup_path/cgroup_name/cgroup_labels on each entry.
          dictionary_flush(apps_lookup_cache)
          // r10-MEDIUM-2: dictionary_flush() removes items (firing the delete
          // callback) but does NOT touch the externally-maintained cache_size
          // counter. After flush, reset cache_size to 0 explicitly.
          cache_evictions_generation_bump += cache_size
          cache_size = 0
          last_observed_generation = response_generation
          // (No re-enqueue here — that violates the cache/intake never-simultaneous
          // invariant. We only flag that a re-enqueue is needed; Phase 6 does it
          // by merging last_seen_pids, which is a superset of recently-seen
          // working-set PIDs — see "Mutex ordering" rationale above.)
          generation_bumped = true
      }
      // r11-MAJOR-1: branch on BOTH outer status AND inner cgroup_status
      // (Contracts 1 & 2). Wire format: nipc_apps_lookup_item_view_t at
      // netipc_protocol.h:539-554 carries both fields; validator at
      // netipc_protocol.c:1965-2013 allows outer=KNOWN with any inner state.
      for each response item:
          // Outer UNKNOWN -- producer has no record of this PID at all.
          // Do not cache; next Function-handler miss will re-enqueue (Contract 5).
          if (item.status == NIPC_PID_LOOKUP_UNKNOWN):
              cache_misses_unknown++
              continue
          // Outer KNOWN -- branch on inner cgroup_status.
          switch (item.cgroup_status):
              case NIPC_APPS_CGROUP_UNKNOWN_PERMANENT:
                  // Contract 1: producer signals UNKNOWN_PERMANENT -> EVICT.
                  // Process exists in apps.plugin but cgroup is permanently
                  // unresolvable. Do not cache; remove any stale entry.
                  if (cache entry exists for item.pid):
                      dictionary_del(apps_lookup_cache, key)
                      cache_size--
                      cache_evictions_unknown_permanent++
                  continue
              case NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER:
                  // Contract 2: producer signals UNKNOWN_RETRY_LATER -> consumer
                  // does NOT cache and asks again next iteration. Leave any
                  // existing entry untouched (it survives until generation bump,
                  // LRU, or PID-reuse per Contract 1).
                  cache_misses_unknown++
                  continue
              case NIPC_APPS_CGROUP_KNOWN:
              case NIPC_APPS_CGROUP_HOST_ROOT:
                  // Safe to cache. HOST_ROOT carries empty cgroup_path/name --
                  // store as such; SOW-0036 consumer treats HOST_ROOT as the
                  // host-namespace default.
                  if (no cache entry for item.pid):
                      insert with response.starttime (delete-callback owns string lifecycle)
                      cache_size++
                      // LRU overflow check; if cache_size > max_cache_size,
                      // run the LRU-evict scan documented under "LRU eviction"
                      // (see r11-MINOR-3 code-comment note: cache_size is
                      // worker-private; no concurrent insert path exists today)
                  else if (cached_starttime == response.starttime):
                      update fields (free old strings via dict delete-callback path or in-place free+strdup), bump last_used_usec
                  else:
                      // PID-reuse: evict + insert (cache_size unchanged net)
                      dictionary_del then insert
                      cache_evictions_pid_reuse++
                  continue
      unlock cache_mutex
      unlock client_mutex
      // view_out is no longer dereferenced after this point.

      // -- Phase 5: no explicit release needed (r9-MAJOR-1) --
      // view_out strings are borrowed from apps_lookup_client_ctx's internal
      // recv buffer and remain valid only until the next nipc_client_call_*
      // on this context (per the lifetime contract at netipc_service.h:195-198
      // for the cgroups-snapshot call; same borrowed-buffer convention applies
      // to all typed nipc_client_call_* functions, including
      // nipc_client_call_apps_lookup at line 220). The next loop iteration's
      // Phase 3 call implicitly invalidates view_out. We must NOT dereference
      // view_out beyond this point.

      // -- Phase 6: re-enqueue after generation bump (intake_mutex held; cache_mutex NOT held) --
      if (generation_bumped) {
          lock intake_mutex
          merge apps_lookup_last_seen_pids into apps_lookup_intake (deduped, bounded)
          unlock intake_mutex
          // Next loop iteration drains the merged intake and re-warms the working set.
          // (Optionally signal eventfd for promptness; loop will iterate anyway.)
      }
  }
  ```
- The worker is the SOLE caller of any `nipc_*` API. The worker's only fast-path responsibility is to drain the intake and refresh the cache; it never blocks on Function workers and is never blocked by them (intake_mutex is held only for vector swap-out; cache_mutex is held only during the response application loop).
- **Generation refresh interval** (round-6 CRITICAL-3 fix): default 30 seconds, configurable via `[plugin:network-viewer] apps lookup generation refresh seconds` (range 5..300). The worker's wait timeout is set to this value; on a clean wakeup with no intake work, the worker performs a probe APPS_LOOKUP to observe the current peer generation. This guarantees that a 100%-warm-cache working set (no churn, no Function-handler misses) STILL detects apps.plugin restarts within `refresh_interval` seconds.

- **PID-reuse staleness window within a single generation (round-3 reviewer concern, EXPLICITLY ACCEPTED per round-6 MAJOR-4 decision)**:
  - When the Function handler hits a cached entry, it serves the cached fields without comparing starttime (the handler doesn't have local starttime; see L2). The starttime comparison happens only inside the worker on the next refresh pass for that PID.
  - **Why this is acceptable** (user-visible rationale recorded per round-6 MAJOR-4):
    1. The staleness window is bounded by the generation refresh interval (default 30s) PLUS apps.plugin's own collection cadence — each generation bump clears the cache and re-populates from the last_seen_pids set.
    2. PID reuse requires the original process to exit AND the kernel to recycle the PID number AND the new process to open a network socket AND network-viewer's Function handler to fire — all within the same refresh window. Empirically rare on hosts with default PID space (~32k); rarer still in containerised environments where PID namespaces isolate the recycling.
    3. The downstream impact (SOW-0036) is misattributed cgroup/orchestrator/labels for at most a few Function calls. SOW-0036 does not use these fields for safety-critical decisions; it groups topology nodes for display.
    4. The fix proposed by reviewers (read `/proc/<pid>/stat` field 22 in network-viewer to local-validate starttime against cache before serving a hit) was considered and REJECTED per L2 — it requires a new `/proc` reader, adds a syscall per PID per Function call (defeating the cache-first design), and only narrows the race; it does not eliminate it. If operators later demand tighter PID-reuse correctness, a follow-up SOW will add local-starttime capture as a separate, measurable change.
  - **Acceptance**: explicitly accept the bounded staleness as a known limitation. Documented in code comment at the cache-hit site and in the operator-facing telemetry chart description so SREs understand what `cache_evictions_pid_reuse` indicates.

**LRU eviction (custom implementation — libnetdata DICTIONARY has no native LRU):**

- The cache entry tracks `last_used_usec` (microseconds since epoch) updated on every cache hit.
- The cache size counter is tracked as a plain `uint32_t cache_size` (r7-LOW-1: no atomic needed — every read/write of `cache_size` happens under `apps_lookup_cache_mutex`, so the external mutex serialises access). `dictionary_entries(apps_lookup_cache)` is an acceptable alternative if its cost is negligible at this cache size; the plain counter is chosen for clarity.
- When inserting a new entry would push `cache_size > max_cache_size` (default 8192): the cache walker scans entries under the mutex and evicts the entry with the smallest `last_used_usec` (linear scan, O(n) — acceptable at 8192 entries for occasional eviction). Eviction removes the entry via `dictionary_del`, which invokes the registered `nv_apps_lookup_cache_entry_destroy` callback to `freez()` all heap-owned strings; the entry's `cache_size` is decremented (per r11-MAJOR-3 scope — `cache_size` is worker-private, so the scan → find smallest → delete → `cache_size--` sequence is correct under the single-writer invariant). **r11-MINOR-3 code-comment note** (must appear at the LRU eviction site in `network-viewer-apps-lookup-client.c`): `cache_size` is worker-private; no concurrent insert path exists today; if a future SOW adds one, this scan needs to be re-evaluated. For higher performance, a doubly-linked-list LRU could be implemented later; deferred unless measurement shows the linear scan is a hot spot.

**Contracts 1 & 2 compliance (r11-MAJOR-1):**

The worker MUST inspect BOTH the outer `status` and the inner `cgroup_status` field of every response item, per the wire format `nipc_apps_lookup_item_view_t` at `netipc_protocol.h:539-554` (enum `nipc_apps_cgroup_status_t` at `netipc_protocol.h:128-133`, validator at `netipc_protocol.c:1965-2013`):

- `status == NIPC_PID_LOOKUP_UNKNOWN`: producer has no record of the PID. Worker does NOT cache; next Function-handler miss re-enqueues per Contract 5. Increment `cache_misses_unknown`.
- `status == NIPC_PID_LOOKUP_KNOWN && cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_PERMANENT`: **Contract 1** says producer signals permanent unknown → consumer EVICTS. Worker removes any existing cache entry for the PID (via `dictionary_del`, which fires the delete-callback to free strings) and decrements `cache_size`. Increment `cache_evictions_unknown_permanent`.
- `status == NIPC_PID_LOOKUP_KNOWN && cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER`: **Contract 2** says producer signals transient unknown → consumer does NOT cache and asks again next iteration. Worker does NOT insert and does NOT update any existing entry. The PID will be re-enqueued by the next Function-handler miss-scan. Increment `cache_misses_unknown`.
- `status == NIPC_PID_LOOKUP_KNOWN && cgroup_status ∈ {NIPC_APPS_CGROUP_KNOWN, NIPC_APPS_CGROUP_HOST_ROOT}`: safe to cache. Worker inserts on absence, updates on starttime-match, evicts-and-inserts on starttime-mismatch (PID-reuse). For `HOST_ROOT` the validator guarantees empty `cgroup_path`/`cgroup_name`/`label_count`; SOW-0036 consumers treat `HOST_ROOT` as the host-namespace default.

SOW-0034:132-136 specifies that `apps.plugin` returns `cgroup_status = UNKNOWN_RETRY_LATER` whenever a process's `cgroup_cache` link is missing (typical for newly-discovered PIDs whose cgroup attribution has not yet completed), so this branch is operationally hot during PID churn — caching it would inflate hit ratio without delivering enriched data and would never self-correct (the cached entry would never be re-queried until generation bump / LRU / PID-reuse).

**Cache entry lifecycle (r7-MEDIUM-1):**

- `apps_lookup_cache` is created with a registered delete-callback `nv_apps_lookup_cache_entry_destroy(const DICTIONARY_ITEM *item, void *entry, void *data)`. The callback unconditionally `freez()`s `entry->cgroup_path`, `entry->cgroup_name`, and `entry->cgroup_labels` (all heap-allocated by the worker via `strdupz`). `freez(NULL)` is safe per libnetdata semantics.
- The callback is invoked on every removal path: per-entry LRU eviction, PID-reuse eviction, generation-bump cache clear (`dictionary_flush`), and plugin-shutdown `dictionary_destroy`. This is the SINGLE point where cache-entry heap memory is reclaimed; cache write paths only allocate and never free directly.

**Cache bounding:**

- `apps_lookup_cache` is a bounded LRU at max 8192 entries (configurable via `[plugin:network-viewer] apps lookup cache size`). On overflow, the least-recently-used entry is evicted. Per Contract 1.

**No user-visible output changes:**

- Topology Function output is unchanged in this SOW. (SOW-0036 introduces visible changes.) Verified by diffing Function output before/after with synthetic input (same fixtures produce byte-identical output structure; only internal state changes).
- The new cache and IPC client are observable only via telemetry counters.
- **Explicit no-modify guarantee (round-3 reviewer clarification; round-4 citation tightening m6)**: this SOW does NOT modify any of:
  - `NV_PROCESS_ACTOR` struct (`network-viewer.c:45-57`) — no new fields. (Citation corrected from 50-57; struct starts at typedef line 45 with fields on 46-56 and close on 57.)
  - `NV_TOPOLOGY_V1_ACTOR` struct (`network-viewer.c:1621-1646`) — no new fields.
  - `local_sockets_cb_to_topology()` (`network-viewer.c:1115` onward; body extends past the `dictionary_set` at 1214 to ~line 1300 with endpoint owner registration) — callback body unchanged.
  - `local_sockets_cb_to_json()` (`network-viewer.c:972-976`) — callback body unchanged. (This is what blocks detailed-view enrichment in SOW-0035; deferred to SOW-0036.)
  - `local_sockets_cb_to_aggregation()` (`network-viewer.c:983`) — callback body unchanged.
  - `local_socket_to_json_array()` (`network-viewer.c:780`) — emitter unchanged.
  - `topology_v1_collect_actors()` (`network-viewer.c:2079`) — actor builder unchanged.
  - `topology_v1_emit_actor_columns()` (`network-viewer.c:2531`) — column emitter unchanged.
  - `topology_write_data()` (`network-viewer.c:3819`) — response builder unchanged.
- The cached APPS_LOOKUP data lives in a separate file-scope dictionary and is NOT read by any output path in this SOW. SOW-0036 introduces the consumers.

**Fallback behaviour:**

- APPS_LOOKUP server absent at startup: the background worker logs INFO once, retries connect on its next loop iteration (next intake signal or refresh-timer wake). Function handlers continue to serve unenriched output identically to today's pre-SOW-0035 behaviour. Per Contract 5. The worker calls `nipc_client_ready(ctx)`; Function handlers never touch the IPC client.
- APPS_LOOKUP server disappears mid-life (clean disconnect, EOF on `recv` inside the worker): `call_with_retry` reconnects once at `netipc_service.c:597-616`; if reconnect fails the worker keeps the cache as-is, sleeps until next signal/timer, and retries. Function handlers continue serving cached fields until the next generation bump is eventually observed by the worker (which, when reconnect succeeds, clears the cache as usual).
- **APPS_LOOKUP server present but slow or hung (round-6 — Function path is immune)**: the netipc client has NO per-request timeout (verified `netipc_uds.h:59-68`, `netipc_service.c:573-645`, `netipc_uds.c` `raw_recv` uses blocking `recv` with no `SO_RCVTIMEO`). With the round-6 async architecture, this affects ONLY the background worker thread. The 5 Function-evloop worker threads keep serving Function calls at cache-snapshot speed; the `main` keepalive thread keeps emitting the stdout heartbeat; the daemon never marks the plugin as dead. Misses simply remain unwarmed for the duration of the worker stall — Function output emits unenriched fields exactly as it does on a cold start. The functions-evloop framework's per-Function deadline (`PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT = 10s`, applied daemon-side) is irrelevant here because Function handlers do not block on IPC. If operational measurement shows worker stalls happen, a follow-up SOW must add either a per-request timeout to netipc (`SO_RCVTIMEO` + `poll`-with-timeout in `raw_recv`) or a non-blocking IPC wrapper — but the user-visible impact is now zero stalls on Function output, regardless.
- **Fallback test procedure (round-6 — targeted-PID, no `pkill`)**: discover the test apps.plugin's exact PID with `pgrep -x apps.plugin`, then verify each candidate before signalling:
  1. `APPS_PID=$(pgrep -x apps.plugin)` — fails the test if zero or multiple candidates are returned (the test must run in a controlled environment where exactly one apps.plugin is present).
  2. Verify ownership: `[ "$(ps -o user= -p "$APPS_PID")" = "netdata" ]`.
  3. Verify the binary path: `readlink "/proc/$APPS_PID/exe"` resolves to the expected `*/plugins.d/apps.plugin`.
  4. **Absent-peer test**: `kill -KILL "$APPS_PID"`. Wait for the process to actually exit (`while kill -0 "$APPS_PID" 2>/dev/null; do sleep 0.1; done`). Issue a network-viewer Function call. Verify HTTP status 200, response contains topology data, and the response is byte-identical (excluding telemetry counters) to a pre-SOW-0035 baseline.
  5. **Slow-peer test**: re-launch apps.plugin (via the daemon), re-discover its PID using the same `pgrep -x` + verify ritual into `APPS_PID2`. `kill -STOP "$APPS_PID2"`. Issue several Function calls; verify they ALL return successfully (round-6: ALL 5 worker threads are unaffected — there is no longer a "one worker stalls" outcome because Function handlers do not perform IPC). Observe the background lookup worker is the only thread blocked in `recv` (verified by inspecting `/proc/$NETVIEW_PID/task/<worker-tid>/stack` for the worker thread, located via its known thread name `nv-applkup`). `kill -CONT "$APPS_PID2"`; confirm the worker drains and the cache resumes warming on subsequent Function calls.
  6. Under NO circumstance use `pkill`, `killall`, or any pattern-matching kill in the test procedure.

**Telemetry per Contract 6:**

- Client-side counters under `netdata.collector.ipc.apps_lookup.client.*`: `requests_sent`, `requests_responded`, `requests_failed` (covers all non-success returns from `nipc_client_call_apps_lookup` — connect failures, overflow recovery exhaustion, peer disconnects; there is no timeout error class because netipc has no per-request timeout), `cache_hits`, `cache_misses_unknown` (response was outer UNKNOWN OR outer KNOWN + inner `UNKNOWN_RETRY_LATER` — both classes are skipped without caching; see note below and r11-MAJOR-1 Contracts compliance), `cache_misses_intake_dropped` (round-6: PIDs dropped because the intake set was at its cap; should normally stay at 0), `cache_evictions_pid_reuse`, `cache_evictions_lru`, `cache_evictions_generation_bump`, `cache_evictions_unknown_permanent` (r11-MAJOR-1: producer signalled `cgroup_status = UNKNOWN_PERMANENT` for a PID with a live cache entry; entry evicted per Contract 1; distinct from LRU/PID-reuse/generation-bump), `peer_connect_attempts`, `peer_disconnects`, `worker_refresh_probes` (round-6: probe APPS_LOOKUP calls issued on the refresh timer with no intake work pending — confirms generation refresh is healthy). Plus `worker_request_duration_ms` histogram (round-6: IPC duration, observed in the background worker only) and `function_handler_overhead_ms` histogram (Function-handler-side cache+enqueue overhead, expected to stay sub-ms). **Round-6 note**: the two histograms are SOW-0035-local extensions to Contract 6, not part of the shared cross-collector contract — they exist because network-viewer's split between Function-handler-side and worker-side overhead is unique among Contract-6 clients and operators need to see both sides directly. **Round-10 note (r10-MEDIUM-3)**: these two histogram names are candidates for renaming or folding into a shared Contract 6 telemetry surface in a future revision once other Contract-6 clients (cgroups.plugin, apps.plugin, future consumers) have shipped and the cross-client "handler overhead vs IPC duration" split pattern is observable. Until then they stay SOW-0035-local with names that clearly identify which side they measure.
- **Counter naming note (round-3 reviewer clarification)**: master plan Contract 6 lists `cache_misses_retry` + `cache_misses_permanent` as separate counters, but APPS_LOOKUP's outer `status` field is two-state (KNOWN/UNKNOWN per `netipc_protocol.h:537`). Distinguishing "retry-later" from "permanently gone" requires tracking previously-known PIDs client-side, which adds memory and complexity for marginal observability benefit. This SOW collapses them into a single `cache_misses_unknown` counter; if operators later require the split, network-viewer can add a "previously-known PID set" structure to derive the heuristic (`previously KNOWN → permanent`; `never seen → retry`). Deferred unless a use case emerges.
- **Operator-facing chart description for `cache_misses_unknown` (r7 reviewer-a W4)**: a small steady-state UNKNOWN rate may originate from worker refresh-probes that pick a long-dead PID still resident in the cache (it has not yet been LRU-evicted and the next generation bump has not yet cleared it). apps.plugin returns UNKNOWN for that PID; the response generation is still observed correctly, so the probe still serves its purpose. The chart description MUST tell SREs: "elevated rates correlate with PID churn during periods of idle Function traffic; sustained high rates suggest tuning `apps lookup generation refresh seconds` or investigating apps.plugin enumeration lag".

**Acceptance (verifiable):**

- **Function-handler overhead (round-6, async model)**: APPS_LOOKUP integration adds <5ms p95 to Function-handler latency in ALL cache states (cold, warm, peer-absent, peer-slow). Measured via the `function_handler_overhead_ms` histogram. The cold-vs-warm split is no longer a Function-handler-latency concern — both states return immediately; cold differs from warm only in the unenriched fields emitted.
- **Background-worker IPC latency**: `worker_request_duration_ms` p95 < 50ms on localhost UDS. **Cross-reference**: depends on SOW-0034's APPS_LOOKUP handler p99 staying within its own measurement gate (<50ms). If SOW-0034 exceeds that, SOW-0035 re-baselines this number but the Function-handler-overhead AC remains <5ms regardless.
- **Cache warming latency (round-6 added)**: time from the first Function call that enqueues a PID to the moment a subsequent Function call observes that PID as a warm hit, measured on an otherwise-idle plugin. Target p95 <1s. Bounded by intake-signal wake + one IPC round trip + cache write.
- Cache hit ratio: >80% after 10 rapid consecutive Function calls separated by ≥1s each (giving the worker time to drain the intake set between calls), on an otherwise-stable working set (no container churn, no generation bump).
- **Concurrent-load latency (round-6 corrected, async model)**: with 5 concurrent Function calls (matching worker count), Function-handler p95 overhead must remain <10ms in ALL cache states. Reasoning: Function handlers no longer serialise on `apps_lookup_client_mutex` (only the worker touches it); the only contention is on `apps_lookup_cache_mutex` and `apps_lookup_intake_mutex`, both held only for memcpy/dict-write-scoped intervals.
- PID-reuse correctness across generations: synthetic test that exits a process, recycles the PID via `unshare`/`fork-many`, forces an apps.plugin generation bump, then issues a network-viewer Function call. Verify the cache entry is replaced with the new starttime. The same-generation case is documented as accepted staleness per "PID-reuse staleness window" above.
- **Generation-refresh correctness on a warm cache (round-6 added, CRITICAL-3 fix)**: warm the cache (issue Function calls until hit ratio >80%), STOP issuing Function calls, restart apps.plugin to force a generation bump, wait `refresh_interval + 2s`, then issue ONE Function call. Verify the cache was cleared by the worker BEFORE this Function call (observable via `cache_evictions_generation_bump` counter > 0 and `worker_refresh_probes` counter increment).
- Generation-bump correctness via working-set churn: restart apps.plugin, immediately issue a Function call. Verify the call enqueues PIDs as misses (cache may not yet be cleared by the worker), and the SECOND Function call after worker drain observes the cleared+re-warmed cache.
- LRU eviction cost: measure p99 cache-mutex hold time during eviction. If the O(n) linear scan at 8192 entries exceeds 5ms p99 under sustained churn, upgrade to a doubly-linked-list LRU in a follow-up. Cache mutex is held only during the scan (not during IPC, which happens in the worker under client_mutex).
- Fallback HTTP status: when APPS_LOOKUP peer is absent, the Function HTTP status code MUST remain 200 with valid response structure.
- **No `pkill` / `killall` in tests (round-6)**: the test harness MUST use the targeted-PID procedure in the Fallback section. CI step that grep's the test scripts for `pkill\|killall` and fails if any match is found.

**Five reviewers vote PRODUCTION GRADE** (process criterion, included for symmetry).

## Analysis

Sources checked: see Master Plan SOW-0032; plus `network-viewer.c` (`main` at 4587-4616 — Function-driven, no iteration loop; `network_viewer_topology_function` at 3936-3960; `topology_prepare_context` at 1354-1422; `topology_context_destroy` at 1332-1352; `local_sockets_process` invocation at 1420), `local-sockets.h:584-737` (per-PID `/proc` reads — no `/proc/<pid>/stat`, no `starttime`).

Current state:

- network-viewer is purely Function-driven. No periodic loop. All work happens inside Function handlers.
- `NV_TOPOLOGY_CONTEXT` is created and destroyed per Function call — cannot host a persistent cache.
- network-viewer reads `comm`, `cmdline`, `uid`, `ppid`, `net_ns_inode` per PID from `/proc` — but NOT `starttime`.

Risks:

- **Function latency (round-6 — eliminated as a risk)**: with async cache-warming, Function handlers no longer perform IPC. Cold cache emits unenriched rows immediately, warm cache emits enriched rows immediately. Function-handler overhead is bounded by mutex hold times and cache-lookup costs only.
- **Background-worker stall on slow/hung APPS_LOOKUP server (round-6 — Function path unaffected)**: the netipc client has no per-request timeout. If apps.plugin's APPS_LOOKUP server stalls, the lookup worker blocks indefinitely in `recv`. ACCEPTED MITIGATION: Function handlers never touch the IPC client and are not blocked by the stall. The cache stops being warmed until the stall resolves; Function output remains valid (unenriched fields for new PIDs, last-known cached fields for previously-known PIDs). The `main` keepalive heartbeat runs on a separate thread and is unaffected. No worker thread blocks indefinitely on a Function call. Follow-up SOW required only if cache warming staleness becomes operationally relevant; user-visible Function output is correct throughout.
- **Cache thread-safety**: three threads can touch the cache (5 Function workers as readers, the background worker as writer). Mitigation: explicit `apps_lookup_cache_mutex` serialises all reads/writes; intake mutex is separate and never held with cache mutex.
- **Intake-set overflow** (round-6 added): under extreme PID churn (e.g., thousands of new PIDs per Function call), the bounded intake set (default 16384) may drop PIDs. Mitigation: dropped PIDs are simply re-derived on the next Function call's working-set scan and re-enqueued — the next call always sees the same /proc/net/* state. Counter `cache_misses_intake_dropped` exposes the rate so operators can size the cap or accept the throttling.
- **Cold start**: empty cache at startup → first few Function calls emit unenriched output while the worker drains the intake set. Bounded by working set (hundreds of PIDs typical) and worker round-trip latency. Acceptable per master-plan risk register; the cold start is now strictly Function-output unenriched, not Function-call slow.
- **PID reuse**: kernel may recycle a PID quickly. Mitigation: cache stores the response's `starttime`; mismatch on subsequent response evicts the stale entry. The first lookup after PID reuse pays the cache-miss cost; subsequent lookups are warm.
- **PID not yet in apps.plugin**: network-viewer's Function handler may run before apps.plugin's next collection cycle has enumerated the new PID. Response is `status = UNKNOWN`; cache nothing; next Function call retries. Lag bounded by apps.plugin's `update_every` (default 1s).
- **Cache memory**: bounded at 8192 entries × (~200 bytes/entry incl. labels) ≈ 1.6 MiB. Negligible for typical label sizes. **Round-3 caveat**: per-entry label bytes are not capped — pathological pods with hundreds of labels per cgroup could push individual entries to several KiB. Measured during validation; if observed entry size > 1 KiB p99, add a per-entry label-byte cap.
- **Cache invalidation on apps.plugin restart**: generation bump triggers full cache clear. Brief window of higher cache-miss rate until cache is re-populated.
- **`last_seen_pids` growth (r7-MAJOR-2, mitigated)**: bounded at 8192 (same cap as cache). On overflow during Function-handler population, the lowest-sequence entry is evicted; duplicate sightings refresh sequence, so active PIDs stay represented longer during churn. Worst-case memory: 8192 × 4 bytes (uint32 PID) + per-entry overhead in the chosen data structure (e.g., ~32 bytes for a small linked-list node + hashtable slot) ≈ 256 KiB. Negligible. Without this bound, a high-churn host could grow the set without limit; with it, the set tracks a sliding window of recent working-set PIDs.
- **Background worker thread crash/panic (r7 reviewer-c item 9)**: a panic inside `nv_apps_lookup_worker_main` (e.g., from a malformed APPS_LOOKUP response that defeats validation) would tear down the entire plugin process per pthread semantics on an unhandled `abort`. Mitigation: defensive input validation inside the worker (all `view_out` field accesses go through netipc-library accessor helpers; any inconsistent response increments `requests_failed` and continues to the next loop iteration rather than asserting). If the worker exits cleanly (e.g., on an unrecoverable IPC handshake error), the plugin should log FATAL and exit so the daemon respawns it — Function handlers serving stale-cache data with no cache warming for an unbounded period is worse than a restart. Implementation must add a sentinel `worker_thread_exited` flag checked by `main`; on observation, log FATAL and exit cleanly to allow daemon respawn.
- **Refresh-probe selecting a long-dead PID (reviewer-a W4)**: when the worker fires the periodic probe on an idle warm cache, it picks the lowest-PID cache entry. That entry may be a process that exited but has not yet been LRU-evicted (e.g., a long-lived cache slot with a low PID). apps.plugin returns UNKNOWN; `cache_misses_unknown` is incremented; the response's generation is still observed correctly, so the probe still serves its purpose. The operator-facing chart description for `cache_misses_unknown` documents this expected steady-state contribution.
- **`eventfd(...)` failure at startup (r10-LOW-4)**: the worker architecture mandates the eventfd as its sole signalling channel between Function handlers and the background worker. Mitigation: on `eventfd(...)` failure during plugin init, log FATAL and exit so the daemon respawns the plugin. Pipe-based fallback was rejected because it would force a second drain loop, double the FD budget, and add no operational benefit — `eventfd` only fails under FD exhaustion or kernels without `CONFIG_EVENTFD`, conditions where retry-by-respawn is the right outcome.

## Pre-Implementation Gate

Status: ready (architectural premise corrected; local decisions resolved; depends on SOW-0034 being merged for the APPS_LOOKUP server to exist).

Problem / root-cause model: network-viewer's Function output has limited per-PID metadata. To enable SOW-0036's container/orchestrator topology groupings, network-viewer needs enriched per-PID data from apps.plugin's APPS_LOOKUP server. Cache must be persistent across Function calls (separate from ephemeral per-call context).

Evidence reviewed: file:line citations above.

Affected contracts and surfaces: new file-scope cache dictionary + mutexes in `network-viewer-apps-lookup-client.c`; new APPS_LOOKUP client wrapper header; `network-viewer.c` cache-warming call sites; CMakeLists.txt source list; metadata.yaml for telemetry charts; no Function output schema change in this SOW.

Existing patterns to reuse: netipc client pattern from `src/libnetdata/netipc/include/netipc/netipc_service.h`; `DICTIONARY *` API from libnetdata; mutex discipline from cgroups.plugin's existing pattern.

Risk and blast radius: as listed in Analysis.

Sensitive data handling: cached labels may contain customer-identifying info; same as today's exposure for cgroup labels. No new surface.

Implementation plan:

1. New file-scope state in `network-viewer-apps-lookup-client.c`: `DICTIONARY *apps_lookup_cache` (`DICT_OPTION_SINGLE_THREADED`, **keyed by stringified PID** via `char key[16]; snprintfz(key, sizeof(key), "%u", pid);` per r10-MAJOR-1 — see Architecture m4 for the verification that binary 4-byte keys would have been silently rejected by `api_is_name_good_with_trace` for any PID whose first byte is `\0`, plus UB on `strlen()` in debug builds) with registered delete-callback `nv_apps_lookup_cache_entry_destroy` that `freez`es `cgroup_path`/`cgroup_name`/`cgroup_labels`; `apps_lookup_intake` recent-sequence set (capped at 16384 entries, drained in 8192-PID request batches to match APPS_LOOKUP server limits); `apps_lookup_last_seen_pids` recent-sequence set (capped at 8192 entries — same cap as cache); eventfd `apps_lookup_intake_eventfd` (`EFD_NONBLOCK | EFD_CLOEXEC`) for worker signalling — **on `eventfd(...)` failure at startup (r10-LOW-4), log FATAL and exit**; the worker architecture mandates the eventfd as its sole signalling mechanism, a pipe-based fallback would force a second drain loop and double the FD budget for no operational benefit, and `eventfd` failure is only expected under FD exhaustion or kernels without `CONFIG_EVENTFD` where daemon respawn is the right outcome; three mutexes (`apps_lookup_cache_mutex`, `apps_lookup_intake_mutex`, `apps_lookup_client_mutex`); `last_observed_generation` (uint64 — protected by `cache_mutex` since it is only read/written by the worker inside the cache critical section); `uint32_t cache_size` (plain, protected by `cache_mutex` — r7-LOW-1; **r11-MAJOR-3 scope**: read/written EXCLUSIVELY by the background worker thread under `apps_lookup_cache_mutex`. Function handlers never read or write `cache_size`. This invariant is what makes the worker's `dictionary_flush` + `cache_size = 0` reset sequence and the worker's LRU `cache_size--` decrement correct — no other reader observes the stale interval. If a future SOW adds a concurrent cache-writer path, this invariant and the flush/decrement sequences MUST be revisited.); `apps_lookup_worker_thread` handle; and `_Atomic bool worker_thread_exited` sentinel (r8-LOW-2 / Analysis "Background worker thread crash/panic") initialised to `false`, set to `true` by the worker just before returning from `nv_apps_lookup_worker_main`, observed by `main`'s keepalive loop (see step 4 and step 11). Function handlers populate `apps_lookup_last_seen_pids` with the full working-set PIDs under `apps_lookup_intake_mutex` in the same critical section as intake-enqueue (see "Lookup integration / Function-handler-side" step 3b).
2. New file `network-viewer-apps-lookup-client.c`:
   - Worker-thread entry `nv_apps_lookup_worker_main(void *arg)` implementing the round-6 worker loop (eventfd+timer wait, drain intake, batched `nipc_client_call_apps_lookup`, generation observation, cache update, refresh-probe on idle timer fire).
   - **`nipc_client_init` call site (r9-MINOR-3)**: invoked from `main()` BEFORE starting the worker thread, populating the file-scope `apps_lookup_client_ctx`. The worker starts with the context already initialised; only connect/reconnect (`nipc_client_ready` / `nipc_client_refresh`) happens inside the worker loop, with retry-with-backoff per Contract 5. Worker is the SOLE caller of all `nipc_*` APIs after init.
   - Function-handler-side cache helpers `nv_cache_lookup_pid(uint32_t pid, nv_cached_fields_t *out)` and `nv_apps_lookup_intake_enqueue(const uint32_t *pids, size_t n)`.
   - `nv_warm_cache_from_topology_actors(NV_TOPOLOGY_CONTEXT *ctx)` and `nv_warm_cache_from_pids(const uint32_t *pids, size_t n)` cache-warming triggers (call lookup + enqueue misses).
3. Wire into the two SOW-0035 cache-warming-trigger surfaces (detailed-view surface dropped — see round-4 sub-state; reaffirmed round-6 MAJOR-5):
   - `nv_warm_cache_from_topology_actors(ctx)` — called from `network_viewer_topology_function` between `topology_prepare_context()` and `topology_write_data()`. Enumerates unique PIDs in `ctx->process_actors` via `dfe_start_read`, calls `nv_cache_lookup_pid` for each, enqueues misses via `nv_apps_lookup_intake_enqueue`. `NV_PROCESS_ACTOR` is NOT modified.
   - `nv_warm_cache_from_pids(pids[], n)` — called from `network_viewer_function` AGGREGATED sub-mode only: after `local_sockets_process()` returns and before the output loop at line 4118; caller iterates the Function-owned `SIMPLE_HASHTABLE_AGGREGATED_SOCKETS ht` (entries mallocz'd at `network-viewer.c:1090-1093`, survive `local_sockets_cleanup`), dedups PIDs into a uint32 vector.
   - **Detailed sub-mode (`sockets:detailed`) is NOT wired in SOW-0035** — `local_sockets_process()` destroys the working set on return (`local-sockets.h:1856`) and the no-modify list prohibits modifying `local_sockets_cb_to_json`. Deferred to SOW-0036.
4. **Worker lifecycle and refresh**:
   - Worker thread started from `main` BEFORE the keepalive loop, named `nv-applkup` (≤15 chars per Linux thread-name limit). **r11-MINOR-2 API choice**: created via `nd_thread_create("nv-applkup", 0, nv_apps_lookup_worker_main, NULL)`. The tag string is stored internally up to `NETDATA_THREAD_TAG_MAX` characters; the OS-level name is set via `pthread_setname_np` (truncated to 15 chars by the kernel). `nv-applkup` fits within the 15-char limit so the OS-level and internal names match.
   - Worker `select`/`poll`/`epoll_wait`s on the intake eventfd with timeout = `apps_lookup_generation_refresh_seconds` (default 30, config `[plugin:network-viewer] apps lookup generation refresh seconds`, range 5..300).
   - On wake: drain intake; if non-empty, perform batched APPS_LOOKUP. If empty AND timer fired AND cache has at least one entry, send a 1-PID probe APPS_LOOKUP (lowest cache PID) to observe peer generation.
   - On observing higher response generation (Phase 4 of worker pseudocode): clear cache under `cache_mutex`, set `generation_bumped = true`. AFTER releasing `cache_mutex` (and `client_mutex`), execute Phase 6: acquire `intake_mutex` and merge `apps_lookup_last_seen_pids` snapshot into the intake set (deduped, bounded). Next loop iteration drains the merged intake and re-warms the working set. This split phase enforces the "cache_mutex and intake_mutex never held simultaneously" invariant (r7-MAJOR-2 / reviewer-a W2).
   - **Worker-exit sentinel (r8-LOW-2 / Analysis "Background worker thread crash/panic"; r9-NIT-1 gating)**: defensive input validation inside the worker treats inconsistent APPS_LOOKUP responses as `requests_failed++` and continues, NEVER asserts. If the worker decides to exit (unrecoverable IPC handshake error, OR `plugin_should_exit` observed during a clean shutdown), it sets `worker_thread_exited = true` (atomic store, release semantics) immediately before returning. This is the SINGLE write site for the sentinel and is set on BOTH the unrecoverable-error path and the clean-shutdown path. The keepalive-loop check (step 11) MUST gate the FATAL log on `!plugin_should_exit` so a clean shutdown — where the keepalive loop is already breaking on its own `plugin_should_exit` check — does not also emit a spurious "worker exited unexpectedly" FATAL.
   - On shutdown: `plugin_should_exit` causes the worker to break its loop; `main` joins the worker thread before plugin teardown.
5. Cache lifecycle: insert on KNOWN; evict on PID-reuse / LRU-overflow / generation-bump (the only three eviction paths). UNKNOWN responses are NOT cached and do NOT evict an existing entry — they only prevent caching for the PID currently in the intake. A previously-cached entry whose PID has left the working set survives until one of the three eviction paths fires (next Function call's miss re-enqueues only if the PID is still in the working set).
6. CMakeLists.txt update.
7. Telemetry charts + metadata.yaml (incl. new round-6 counters/histograms listed in Telemetry section).
8. Fallback tests using the targeted-PID procedure (round-6 MAJOR-6 — no `pkill`).
9. Latency measurement: Function-handler-overhead histogram (target <5ms p95 all states); worker-IPC histogram (target <50ms p95); cache-warming-latency end-to-end (target <1s p95).
10. Generation-refresh-on-warm-cache test (round-6 CRITICAL-3 fix).
11. **Cleanup on exit (r7-MEDIUM-3 — ordered to drain Function workers first; r8-LOW-2 keepalive sentinel check; r9-NIT-1 FATAL gating)**: inside the keepalive loop, each iteration checks `if (!plugin_should_exit && atomic_load_explicit(&worker_thread_exited, memory_order_acquire)) { netdata_log_error("FATAL: network-viewer apps-lookup worker exited unexpectedly; requesting daemon respawn"); plugin_should_exit = true; break; }`. The `!plugin_should_exit` guard prevents a spurious "exited unexpectedly" FATAL during clean shutdown — in that case the worker also sets `worker_thread_exited = true` on its way out, but `plugin_should_exit` is already true, so the FATAL branch is skipped and the keepalive loop simply exits on its own `while(!plugin_should_exit)` condition. A genuine worker crash is observed BEFORE any shutdown signal, so `!plugin_should_exit` is true and the FATAL is logged, requesting daemon respawn rather than letting Function handlers serve stale-cache data with no warming for an unbounded period. After the keepalive `while(!plugin_should_exit)` loop exits:
    - (a1) `functions_evloop_cancel_threads(wg)` to SIGNAL cancellation to the reader thread and the 5 Function-evloop worker threads (verified at `src/libnetdata/functions_evloop/functions_evloop.c:392-396` — this only calls `nd_thread_signal_cancel`, it does NOT join).
    - (a2) **`functions_evloop_join_threads(wg)`** to JOIN the reader thread and the 5 Function-evloop worker threads (**r11-MAJOR-2 fix**). This helper does NOT exist in the evloop library today and MUST be added as part of SOW-0035 — it is a 5-line addition that iterates `wg->reader_thread` and `wg->worker_threads[i]` (for `i ∈ [0, wg->workers)`) and calls `nd_thread_join` on each. The struct `functions_evloop_globals` is opaque (defined in `functions_evloop.c:42-66`, not exposed in `.h`), so direct field access from network-viewer is impossible — the helper MUST live alongside `functions_evloop_cancel_threads`. Steps (a1)+(a2) together guarantee that no Function worker is mid-callback (line 134 `j->cb(...)`) when `apps_lookup_cache_mutex`, `apps_lookup_intake_mutex`, or the cache dictionary itself are destroyed in steps (d)-(h). Without the explicit join, `nd_thread_signal_cancel` only sets an atomic flag + signals the worker condvar, and a worker currently executing a Function callback does NOT observe the flag until the callback returns — producing a use-after-free window during shutdown.
    - (b) Signal the worker eventfd (`eventfd_write(apps_lookup_intake_eventfd, 1)`) to wake the background worker; the worker observes `plugin_should_exit` at the top of its loop and returns.
    - (c) Join the background worker thread.
    - (d) `dictionary_destroy(apps_lookup_cache)` — the registered delete-callback `freez`es every entry's heap-owned strings.
    - (e) Destroy the intake set and `apps_lookup_last_seen_pids`.
    - (f) `close(apps_lookup_intake_eventfd)`.
    - (g) `nipc_client_close(&apps_lookup_client_ctx)`.
    - (h) `netdata_mutex_destroy()` for all three mutexes.
    - Each call wrapped in NULL/initialised-flag check. The strict ordering (Function workers → background worker → shared state) MUST be preserved to avoid use-after-free.
12. Multi-reviewer pass.

Validation plan: as in AC (latency targets, hit ratio, PID-reuse test, generation-bump test, fallback test), with the APPS_LOOKUP peer behavior covered by a local mock-peer test because standalone `apps.plugin` is not a reliable peer harness in this unprivileged build environment.

Artifact impact plan: source files + focused test target + CMakeLists.txt + metadata.yaml. No spec/skill/end-user doc change in this step because SOW-0035 only warms an internal cache and exposes collector telemetry; SOW-0036 is the first visible topology-output consumer.

Open decisions: NONE.

## Implications And Decisions

Decisions inherited from SOW-0032 (D1-D11).

Local decisions (resolved 2026-05-26 after round-1 reviewer feedback):

- **L1 Architecture (round-6 redesigned)**: persistent cache at file scope in `network-viewer-apps-lookup-client.c`. Function-driven (no periodic Function call). APPS_LOOKUP runs ASYNCHRONOUSLY in a dedicated background worker thread; Function handlers only read the cache and enqueue misses.
- **L2 Cache key — PID-ONLY decision (round-6 MAJOR-4 explicit user-visible decision; r7-LOW-2 reminder)**: cache is keyed by PID alone — NOT `(pid, starttime)` composite. Implementors and reviewers expecting the composite form should consult this decision before raising the question again. Cache entries STORE the `starttime` returned by APPS_LOOKUP; the BACKGROUND WORKER detects PID reuse by comparing cached vs response starttime on the next refresh pass for that PID. Function handlers do NOT compare starttimes (no local starttime read in network-viewer). Within a single generation, PID reuse may briefly serve stale cgroup/orchestrator/label metadata for a recycled PID; the staleness window is bounded by `apps_lookup_generation_refresh_seconds` (default 30s) PLUS apps.plugin's own `update_every`. Reading `/proc/<pid>/stat` field 22 locally in network-viewer to add a local-starttime cache key was considered and REJECTED in SOW-0035: it requires a new `/proc` reader, costs a syscall per PID per Function call (defeating the cache-first design), and only narrows the race, not eliminates it. Rationale recorded for operators: this trade-off favours Function-handler latency over PID-reuse precision; SOW-0036 consumers (topology grouping for display) tolerate brief misattribution. If operators later demand tighter PID-reuse correctness, a follow-up SOW will add local-starttime capture as a separate, measurable change.
- **L3 Cache size**: bounded LRU max 8192 entries, configurable `[plugin:network-viewer] apps lookup cache size`.
- **L4 Batching**: one APPS_LOOKUP request per worker drain pass, capped at 8192 PIDs to match `apps.plugin`'s `APPS_LOOKUP_MAX_PIDS_PER_REQUEST`. The intake set remains capped at 16384 so bursty Function calls can queue more than one request worth of misses; any remainder is left queued and drained on the next pass.
- **L5 Mutex inventory**: three mutexes: `apps_lookup_cache_mutex` (cache reads by Function workers, cache writes by background worker), `apps_lookup_intake_mutex` (enqueue by Function workers, drain by background worker), `apps_lookup_client_mutex` (background worker only — no Function worker ever acquires it). Cache and intake mutexes never held simultaneously.
- **L6 Dedicated background thread (round-6 reversed)**: a single background lookup worker thread is added. This is a reversal of the round-1..round-5 "no background thread" decision; the reversal is forced by (a) round-6 CRITICAL-1 removing IPC from the Function path, and (b) round-6 CRITICAL-3 requiring periodic generation refresh on a warm cache. The thread runs the loop documented in the "Lookup integration" section and is the SOLE caller of any `nipc_*` API.
- **L7 Per-request timeout (round-5 verified, round-6 risk-reframed)**: the netipc client library has NO per-request timeout. Verified: `nipc_uds_client_config_t` (`src/libnetdata/netipc/include/netipc/netipc_uds.h:59-68`) has no timeout field; `call_with_retry` (`src/libnetdata/netipc/src/service/netipc_service.c:573-645`) has no timeout around the `attempt()` call; `raw_recv` in `src/libnetdata/netipc/src/transport/posix/netipc_uds.c` uses blocking `recv(fd, buf, len, 0)` with no `SO_RCVTIMEO`. SOW-0034 round-4 work independently verified the same limitation. No `[plugin:network-viewer] apps lookup request timeout ms` config knob is added in this SOW because there is nothing in the netipc client to thread it into. **Round-6 risk reframe**: under the async architecture, the absence of a per-request timeout only stalls the background lookup worker. Function-handler-side behaviour is unaffected. The worker may sit indefinitely in `recv` if apps.plugin's server stalls — cache warming pauses; Function output continues to serve last-known cached fields and emit unenriched fields for new PIDs. Adding a per-request timeout to netipc remains a deferred follow-up driven by a separate netipc-library SOW, but the user-visible motivation is now "speed up cache warming under a degraded peer", not "unblock Function calls".
- **L8 Generation refresh interval (round-6 added, CRITICAL-3 fix)**: default 30 seconds, configurable `[plugin:network-viewer] apps lookup generation refresh seconds` (range 5..300). Drives the worker's wait timeout; on timer fire with no intake work AND non-empty cache, the worker issues a 1-PID probe APPS_LOOKUP to observe the current peer generation.
- **L9 Worker wake mechanism — eventfd (r7-MEDIUM-2)**: the worker is signalled via **eventfd** (`apps_lookup_intake_eventfd`, created with `EFD_NONBLOCK | EFD_CLOEXEC`). Rationale: (a) Linux-only matches plugin posture; (b) coalesces multiple Function-handler signals into a single wake (matches the worker's "drain everything" semantics); (c) integrates with `poll`/`epoll` so the worker can wait on the eventfd AND the refresh timer (poll timeout) in one syscall, without the cancellation hazards of `pthread_cond_timedwait`. Condvar was rejected because the refresh-timer + signal coalescing fit eventfd's semantics exactly.
- **L10 `last_seen_pids` set bounds and write path (r7-MAJOR-1, r7-MAJOR-2)**: `apps_lookup_last_seen_pids` is a recent-sequence-bounded set capped at 8192 entries (same cap as `apps_lookup_cache`). It is WRITTEN by Function handlers under `apps_lookup_intake_mutex` (full working-set PIDs per call, deduped against existing entries with sequence refreshed on duplicate; lowest-sequence entry evicted on overflow). It is READ by the worker after a generation-bump cache clear, exclusively in Phase 6 of the worker pseudocode, where the worker acquires `apps_lookup_intake_mutex` (cache_mutex already released) and merges the snapshot into the intake set. This keeps the "cache_mutex and intake_mutex never held simultaneously" invariant intact (resolves reviewer-a W2/C2).
- **L11 Cache entry destructor (r7-MEDIUM-1)**: `apps_lookup_cache` registers a delete-callback `nv_apps_lookup_cache_entry_destroy` that `freez`es `cgroup_path`, `cgroup_name`, `cgroup_labels` on every removal path (LRU, PID-reuse, generation-bump flush, `dictionary_destroy`). All heap allocation for these strings happens via `strdupz` in the worker; all deallocation happens via this single callback.

## Plan

Implementation chunks as in Pre-Implementation Gate (12 chunks: 1 state, 2 worker file, 3 wire surfaces, 4 worker lifecycle/refresh, 5 cache lifecycle, 6 CMakeLists, 7 telemetry/metadata, 8 fallback tests, 9 latency measurement, 10 generation-refresh-on-warm-cache test, 11 cleanup on exit, 12 multi-reviewer pass).

Note: no Function output changes in this SOW. Reviewers should confirm no behaviour change in either the topology table or the network-connections table — including the aggregated AND detailed sub-modes (byte-identical output structure when cached fields are not yet used by SOW-0036). The detailed-view sub-mode of network-connections does NOT contribute PIDs to the cache in SOW-0035 (deferred to SOW-0036, see round-4 sub-state); detailed-view output is identical to pre-SOW-0035 behaviour.

## Execution Log

- Implemented `network-viewer-apps-lookup-client.{c,h}` as the file-scope APPS_LOOKUP client/cache module. Evidence: state, mutexes, worker sentinel, active-fd cancel guard, and telemetry counters at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:52-95`; cache delete callback at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:211-222`; duplicate PID sightings refresh recent-sequence state at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:297-330`.
- Implemented bounded async drain and worker-only IPC. Evidence: intake drains at most 8192 PIDs and re-signals when more remain at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:354-391`; peer retry/backoff at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:591-625`; worker poll/IPC/active-fd cancellation path at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:628-728`.
- Implemented Contract 1/2 response handling. Evidence: generation bump flush/reset at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:525-531`; outer UNKNOWN skip at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:541-544`; inner UNKNOWN_PERMANENT eviction at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:550-558`; inner UNKNOWN_RETRY_LATER no-cache path at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:560-563`; KNOWN/HOST_ROOT cache insert/update/PID-reuse handling at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:565-581`.
- Wired cache warming into SOW-0035 surfaces only. Evidence: PID sort/unique helpers and topology/aggregated warmers at `src/collectors/network-viewer.plugin/network-viewer.c:1355-1424`; topology Function warms after context preparation and before data writing at `src/collectors/network-viewer.plugin/network-viewer.c:4025-4030`; aggregated network-connections mode warms before output sorting at `src/collectors/network-viewer.plugin/network-viewer.c:4179-4191`.
- Added lifecycle and telemetry output. Evidence: init/start/stop/public warm path at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:740-884`; charts at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:888-1012`; plugin main starts the worker, checks unexpected exit, emits charts, cancels+joins Function workers, then stops APPS_LOOKUP at `src/collectors/network-viewer.plugin/network-viewer.c:4677-4710`.
- Added `functions_evloop_join_threads()` to close the shutdown race identified in review. Evidence: implementation at `src/libnetdata/functions_evloop/functions_evloop.c:399-404`; declaration at `src/libnetdata/functions_evloop/functions_evloop.h:93-96`.
- Added build/metadata coverage. Evidence: new source and test target in `CMakeLists.txt:3177-3203`; network-viewer config and metric metadata in `src/collectors/network-viewer.plugin/metadata.yaml:50-154`.
- Added a focused mock-peer test for the APPS_LOOKUP client worker. Evidence: mock server and response cases at `src/collectors/network-viewer.plugin/tests/test_network_viewer_apps_lookup_client.c:54-147`; end-to-end worker/cache/retry/latency assertions at `src/collectors/network-viewer.plugin/tests/test_network_viewer_apps_lookup_client.c:188-222`.

## Validation

- Configure: `cmake -S . -B /tmp/topology-containers-sow0035-nv-build -DENABLE_PLUGIN_NETWORK_VIEWER=ON -DENABLE_PLUGIN_XENSTAT=OFF -DENABLE_PLUGIN_DEBUGFS=OFF -DENABLE_CGROUPS_LOOKUP_SERVER=ON` passed.
- Build: `cmake --build /tmp/topology-containers-sow0035-nv-build --target network-viewer.plugin apps-lookup-protocol-test cgroup-lookup-netipc-test network-viewer-apps-lookup-client-test -j1` passed.
- Protocol regression: `/tmp/topology-containers-sow0035-nv-build/apps-lookup-protocol-test` passed.
- Upstream netipc regression: `/tmp/topology-containers-sow0035-nv-build/cgroup-lookup-netipc-test` passed.
- APPS_LOOKUP client mock-peer test: `/tmp/topology-containers-sow0035-nv-build/network-viewer-apps-lookup-client-test` passed. This test verifies worker IPC reaches the mock server, a known PID is cached and later served as a hit, `UNKNOWN_RETRY_LATER` is not cached and is retried, mock worker requests land in the <=50ms local bucket, Function-handler warm calls land in the <=5ms local bucket, and cleanup joins the worker cleanly.
- Runnable smoke: `network-viewer.plugin` accepted `FUNCTION tx1 60 "network-connections sockets:aggregated" "0x13" "smoke"` and returned `FUNCTION_RESULT_BEGIN "tx1" 200 "application/json" ...`; APPS_LOOKUP telemetry charts were emitted. In this unprivileged local run, stderr also contained expected `/proc/*/fd` permission and namespace-switch warnings from local-sockets plus an absent-peer APPS_LOOKUP retry notice; no worker FATAL or Function 404 occurred.
- Metadata parse: `python - <<'PY' ... yaml.safe_load(open('src/collectors/network-viewer.plugin/metadata.yaml')) ... PY` passed.
- Hygiene: `git diff --check` passed before close-out; final staged check is recorded in the commit step.
- Durable artifact scan: searched changed SOW/source/test/metadata files for personal names and external-tool/vendor traces; no matches.
- SOW audit: `.agents/sow/audit.sh` passed except pre-existing project-skill classification warnings already present outside this SOW.

## Outcome

SOW-0035 is complete. `network-viewer.plugin` now warms an internal APPS_LOOKUP cache asynchronously for topology and aggregated network-connections Function calls, with no visible Function output changes in this SOW. The implementation preserves the async/no-IPC Function path, respects the 8192 APPS_LOOKUP request cap, retries absent peers without blocking Function workers, records APPS_LOOKUP client telemetry, and shuts down Function workers before destroying the cache.

## Lessons Extracted

- Standalone `apps.plugin` is not a dependable APPS_LOOKUP validation peer in an unprivileged local build; a focused mock netipc peer gives stronger deterministic coverage for the client worker contract.
- The APPS_LOOKUP duration buckets are exclusive counters even though the chart descriptions call them cumulative histograms; tests must sum the relevant buckets when checking local latency thresholds.
- The bounded intake can be larger than a single APPS_LOOKUP request, so request draining must cap each IPC call at 8192 and re-signal when queued PIDs remain.

## Followup

- Tracked by SOW-0036: consume this cache for visible topology groupings.
- Tracked by SOW-0036: detailed-view (`sockets:detailed`) enrichment remains out of SOW-0035 because `local_sockets_process()` cleans up detailed-mode socket storage before a post-process warm step can run.
- Tracked by SOW-0036: comm-keyed topology mode still needs a full PID side channel or actor structure change if all same-comm PIDs must be enriched.
- Implemented in this SOW: `functions_evloop_join_threads()` plumbing, APPS_LOOKUP batch cap, duplicate PID recency refresh, telemetry metadata, and mock-peer client validation.
- Rejected for now, no active follow-up SOW: split `cache_misses_unknown`, connection pool, doubly-linked-list LRU, and per-entry label-byte cap. Current validation and code paths do not show evidence that these are needed before the first visible consumer. If production telemetry later shows a concrete issue, open a new SOW with measurements.

## Regression Log

None yet.
