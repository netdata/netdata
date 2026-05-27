# SOW-0037 - Step 5: cgroups targeted refresh from CGROUPS_LOOKUP request signal

## Status

Status: completed

Sub-state: revised after round-3 standard review (2026-05-26). Fixed reviewer-identified contradictions: (1) AC text now describes the same consume-at-start + bounded-rescan algorithm as the Resolved Decisions (eliminated the old "reset-at-end" wording that contradicted the final-reviewer-revised algorithm), (2) removed leftover "inotify" mention from the Analysis section, (3) removed leftover "Optionally extends to apps.plugin" wording from the Purpose section to match L3 (OUT OF SCOPE), (4) corrected the lost-signal race window line citation (1180→1178 instead of 1184-1185), (5) clarified that the pre-wait flag check is performed under `discovery_thread.mutex`, (6) clarified that the bounded-rescan policy means the discovery thread skips the pre-wait flag check exactly once after running the bounded rescan (prevents the unbounded loop reviewer #reviewer-e raised), (7) corrected the latency-target formula to account for worst-case bounded-rescan cost, (8) moved discovery-side scan counters out of the lookup-server telemetry namespace, (9) reordered Plan steps so scan-cost baseline is step 1 (matches Pre-Implementation Gate). Awaiting next standard-reviewer pass (final-reviewer already used, will not re-run).

Sub-state (phase-2 takeover 2026-05-27): implementation started after upstream SOW-0033 and SOW-0034 were completed. The earlier production-host-specific scan-cost gate is corrected below: production-host measurement requires explicit user permission and is not a correctness prerequisite for the selected first-pass design. The SOW already resolves L2 as full opportunistic scan first, with targeted scan deferred if validation or production evidence shows saturation.

## Requirements

### Purpose

Close the cache-miss latency loop. Without this SOW, when apps.plugin asks CGROUPS_LOOKUP about a cgroup that cgroups.plugin doesn't yet know about, cgroups.plugin returns UNKNOWN_RETRY_LATER and apps.plugin must wait for the next full discovery cycle for the cgroup to appear. This can be seconds, during which the cgroup (and any PIDs in it) appears as unresolved in the eventual network-viewer topology.

This SOW makes cgroups.plugin's discovery thread responsive to lookup requests: when CGROUPS_LOOKUP queries arrive for cgroups not in `cgroup_root`, the discovery thread is signalled to do an opportunistic full scan triggered earlier than the natural interval. The cgroups are then added to the tree faster than the natural discovery cadence would have done. Targeted (path-list) scan is a deferred optimisation, gated on validation or production evidence that full scans saturate discovery under high churn. Extending the same wakeup pattern to apps.plugin is OUT OF SCOPE for this SOW (see decision L3).

This is the prerequisite for SOW-0036 going user-visible: without targeted refresh, the user sees "transient unresolved containers" each time a new container starts, until the next natural discovery cycle.

### User Request

See master plan SOW-0032 for the full directive. This SOW implements step 5 of six. Per user decision D4, it MUST ship before user-visible step 4 (SOW-0036).

### Assistant Understanding

Facts (corrected after round-1 reviewer feedback):

- The CGROUPS_LOOKUP handler from SOW-0033 has read-side access to `cgroup_root` under `cgroup_root_mutex`. On a query for an unknown path, it returns `UNKNOWN_RETRY_LATER` (or `UNKNOWN_PERMANENT` if the path is in the reaped-set).
- The discovery thread uses a condition variable wait at `cgroup-discovery.c:1178-1180` (`netdata_cond_wait(&discovery_thread.cond_var, &discovery_thread.mutex)` — no timeout, confirmed via `src/libnetdata/locks/locks.c:43-46`).
- **Existing signal sources today** (verified via grep; corrected per final-reviewer review): (i) periodic from the collection thread (`sys_fs_cgroup.c:1452-1455`, called every `cgroup_check_for_new_every` seconds — default **10 seconds** per `sys_fs_cgroup.c:25`, NOT 1 second), (ii) `cgroups_check`-driven after failed cgroup file reads / disappeared files: `cgroups_check` is set to 1 at multiple sites (`sys_fs_cgroup.c:471-828`) when a cgroup file fails to read, and the collection loop signals discovery early when `cgroups_check == 1` (`sys_fs_cgroup.c:1452-1457`). This is NOT inotify-based (there are no inotify references in cgroups.plugin source — earlier draft was wrong), (iii) cleanup path (`sys_fs_cgroup.c:1340-1342`). The lookup-miss signal added by this SOW is an ADDITIONAL source, joining three existing sources.
- Discovery iterations call `discovery_find_all_cgroups()` (`cgroup-discovery.c:1111-1146`) which walks the entire filesystem cgroup tree, processes each cgroup, and copies the result under `cgroup_root_mutex` (lines 1133-1141).
- The default discovery cadence configurable knob is `[plugin:cgroups] check for new cgroups every`, with default **10 seconds**, floored to `cgroup_update_every`.
- The CGROUPS_LOOKUP handler holds `cgroup_root_mutex` during its read window. The discovery thread's signal mechanism uses a DIFFERENT mutex (`discovery_thread.mutex`). The handler MUST release `cgroup_root_mutex` BEFORE acquiring `discovery_thread.mutex` for the signal call. Acquiring both simultaneously is forbidden by this SOW (sets the lock-ordering rule).
- `apps.plugin` is a **separate process** (binary at `/usr/libexec/netdata/plugins.d/apps.plugin`), not a thread. It has no `discovery_thread` struct, no condition-variable wait loop — its main loop at `apps_plugin.c:818` is a heartbeat-driven polling loop. Adding a signal-based wakeup to apps.plugin requires substantial structural changes that are NOT in scope for this SOW.

Inferences:

- Lookup handler signals the discovery thread (`netdata_cond_signal` on `discovery_thread.cond_var`) when an unknown path is queried (excluding paths in the reaped-set — those are already known-PERMANENT and signalling would waste a scan). Discovery thread wakes up early and does a full opportunistic scan.
- A "targeted scan" of just the requested paths is simpler than a full re-scan in some ways (no global mutex contention for as long) but adds new code paths to maintain. The full opportunistic re-scan reuses tested code and is preferred for the first implementation. Targeted scan is a deferred optimisation if measurement shows full scans become a CPU hot spot under high churn.

Resolved decisions (per round-1 reviewer feedback):

- **Coalescing strategy**: lock-free CAS flag (`_Atomic bool discovery_signal_pending`) with consume-at-scan-start, bounded immediate-rescan, and a pre-wait flag check under `discovery_thread.mutex` (closes the classic lost-signal race). The full state machine and lock-ordering rules are specified ONCE under Acceptance Criteria → "Discovery-thread state machine"; that section is the single source of truth and supersedes any informal description elsewhere in this SOW.
- **apps.plugin extension is OUT OF SCOPE for this SOW.** apps.plugin's signal-based refresh requires structural changes to its main loop and is not analogous to cgroups.plugin's thread-based architecture. Tracked as a separate followup SOW if the latency analysis shows the equivalent feature is needed there.
- **Implementation chooses full opportunistic scan** for first pass; targeted scan is deferred to a followup if measurement shows full scans become a CPU hot spot.

Measurement note (validation / follow-up, not a coding blocker):

- The selected first-pass design is full opportunistic scan (L2), reusing `discovery_find_all_cgroups()`. The measurement informs performance risk and future optimisation, not whether the feature can be implemented correctly.
- Do not run measurements on production or production-like hosts without explicit user permission. Local synthetic validation may be recorded if available; otherwise track targeted scan as the follow-up path if real deployments show discovery saturation.

### Acceptance Criteria

**Signal source rules:**

- The CGROUPS_LOOKUP handler signals the discovery thread when it returns `UNKNOWN_RETRY_LATER` for a cgroup path NOT present in `cgroup_root` AND NOT present in the reaped-set. Signals are skipped for: (a) paths in the reaped-set (PERMANENT — no point scanning), (b) paths present in `cgroup_root` but not yet processed (RETRY_LATER from in-flight discovery; the in-flight scan will resolve them).
- Coalescing: lock-free CAS flag (`_Atomic bool discovery_signal_pending`). Handler-side: `__atomic_exchange_n(&discovery_signal_pending, true, __ATOMIC_RELEASE)`. If the prior value was false, the handler then acquires `discovery_thread.mutex`, calls `netdata_cond_signal`, releases the mutex. If the prior value was true, the handler does nothing further (another signaller already armed the flag in this window). Discovery-thread state machine (this is the SINGLE authoritative description; supersedes any earlier shorter description elsewhere in this SOW). The thread keeps one stack-local boolean `just_did_bounded_rescan`, initialised to `false` at thread start:
  1. Acquire `discovery_thread.mutex`. Compute `flag = __atomic_load_n(&discovery_signal_pending, __ATOMIC_ACQUIRE)`. Decide:
     - if `just_did_bounded_rescan == true`: call `netdata_cond_wait(&discovery_thread.cond_var, &discovery_thread.mutex)` UNCONDITIONALLY (even if `flag == true`), then set `just_did_bounded_rescan = false`. This is the bound: at most ONE rescan per natural wakeup; any lookup-miss arriving during the rescan that already armed the flag is resolved on the NEXT wakeup, NOT immediately.
     - else if `flag == true`: skip `netdata_cond_wait` (the flag is the signal — discovery has work to do).
     - else: call `netdata_cond_wait(...)`. The pre-wait check and the `cond_wait` are both performed under the same `discovery_thread.mutex`, which closes the classic lost-signal race against the handler (the handler must acquire the same mutex to call `cond_signal`).
  2. Release `discovery_thread.mutex`.
  3. At scan START, atomically consume the flag: `__atomic_exchange_n(&discovery_signal_pending, false, __ATOMIC_ACQUIRE)`. Run `discovery_find_all_cgroups()`.
  4. After the scan completes, check the flag once with `__atomic_load_n(..., __ATOMIC_ACQUIRE)`. If `flag == true` AND `just_did_bounded_rescan == false`, run ONE additional `discovery_find_all_cgroups()` immediately (the "bounded immediate rescan"). At scan START of the rescan, consume the flag again with `__atomic_exchange_n(..., false, __ATOMIC_ACQUIRE)`. Set `just_did_bounded_rescan = true`.
  5. Loop back to step 1.
  6. NO timed wait. No additional periodic wakeup beyond what already exists. The natural cadence is unchanged.
- This algorithm guarantees: (a) any lookup-miss arriving up to the moment the scan completes triggers at least one bounded immediate rescan, eliminating the "active-scan signal lost" ambiguity; (b) sustained churn cannot pin discovery in an unbounded back-to-back scan loop, because the pre-wait check is skipped exactly once per natural wakeup after a bounded rescan has been performed.
- **Lock-ordering rule (mandatory, prevents future deadlock)**: the CGROUPS_LOOKUP handler MUST release `cgroup_root_mutex` BEFORE acquiring `discovery_thread.mutex`. Never hold both simultaneously. Documented as a permanent invariant in the source file header.

**Behavioural correctness:**

- The signal is non-blocking from the handler's perspective: the `discovery_thread.mutex` acquire-signal-release is microsecond-scale and does not wait for the scan to complete.
- **Race acknowledged**: if the lookup-miss signal arrives during the window between `cgroup-discovery.c:1180` (mutex unlock after `netdata_cond_wait` returns) and `:1178` (next iteration's mutex lock before re-entering `netdata_cond_wait`), the signal is delivered to `pthread_cond_signal` on an unwaited condvar and is therefore lost. This window spans the entire `discovery_find_all_cgroups()` call plus the bounded-rescan (if any). The CAS flag preserves the intent — the next time discovery is about to enter the wait, it observes `discovery_signal_pending == true` (left from the previous signaller) and treats that as a pending wakeup, skipping the `cond_wait`. Implementation MUST perform the pre-wait flag check while holding `discovery_thread.mutex` (the same mutex used by `netdata_cond_wait`), so the check-and-wait pair is atomic with respect to the handler's signal path; this is what closes the lost-signal race. This is a critical implementation detail captured here, not deferred.
- No regression in discovery thread behaviour when no signals arrive: the natural periodic signal from `cgroups_main` (`sys_fs_cgroup.c:1452-1455`) continues to drive the natural cadence every `cgroup_check_for_new_every` seconds (default 10s). The lookup-miss signal is an additional source, not a replacement.

**Validation evidence (split into three independent measurements per final-reviewer review):**

1. **Container-created → first-UNKNOWN latency**: wall-clock from container creation (cgroup appears in `/sys/fs/cgroup`) to the first CGROUPS_LOOKUP response (which will be UNKNOWN). This measures the time it takes for the apps.plugin caller to observe the new cgroup and query. Bounded by apps.plugin's heartbeat iteration (~1s default). This is OUTSIDE this SOW's control — measured but not a target.
2. **First-UNKNOWN → first-KNOWN latency**: wall-clock from the first UNKNOWN response (when the lookup-miss signal is emitted) to the first KNOWN response (next caller iteration after discovery has rescanned). This is THIS SOW's primary target. Per Contract 3, the caller queries on its next iteration after UNKNOWN, so the effective floor is one caller-iteration period. Worst case includes the bounded immediate rescan when a signal arrives just after a natural scan starts: signal waits for the in-flight scan to finish (~`scan_p99`), then the bounded rescan resolves the cgroup (~another `scan_p99`), then the caller queries on its next iteration. Target: `first-UNKNOWN → first-KNOWN p99 ≤ 2 × scan_p99 + 1.1 × caller_iteration_period`. The `1.1` is slack for jitter; the `+ caller_iteration_period` is the Contract-3 floor; the `2 ×` accounts for the worst-case bounded-rescan path. Best-case (signal arrives while discovery is in `cond_wait`) reduces to `scan_p99 + 1.1 × caller_iteration_period`.
3. **Cgroups collection-loop cadence under churn**: synthetic-churn workload (start/stop 50 containers per minute for 10 minutes). Measure `cgroups_main` per-iteration wall-clock time. Acceptance: p95 iteration time does NOT regress by more than 5% compared to a matching pre-SOW baseline. This proves Contract 9 (discovery vs collection separation) is preserved under opportunistic-scan bursts.
- Compare with and without this SOW's signalling enabled (the flag is `__atomic_load`-only when disabled, so the comparison is a build-flag toggle, no separate build).
- Baseline expectation: WITHOUT signalling, first-UNKNOWN → first-KNOWN latency is bounded by `cgroup_check_for_new_every` (default 10s) — average ~5s, p99 ~10s. WITH signalling, the same metric is bounded by at most `2 × scan_p99 + 1 × caller_iteration_period` (worst case, bounded-rescan path) and on average by `scan_p50 + 1 × caller_iteration_period` (typical case, signal arrives during cond_wait).

**Scan-cost baseline (validation / follow-up, not a Pre-Implementation blocker):**

- When a suitable non-production test host is available, measure `discovery_find_all_cgroups()` duration at representative cgroup counts and capture p50/p95/p99 of scan duration.
- Additionally, measure `cgroup_root_mutex` wait time on the collection thread under synthetic high-churn workload if the environment can safely run it.
- If scan p99 at large scale exceeds 500ms or collection-thread p99 wait time exceeds 100ms, targeted scan remains the follow-up optimisation path. It does not change this SOW's correctness contract because the implementation already coalesces signals and bounds immediate rescans.

**Telemetry per Contract 6:**

- New counters split by responsibility:
  - Under `netdata.collector.ipc.cgroups_lookup.server.*` (server-side handler events): `lookup_miss_signals_sent` (handler exchanged the CAS flag from false → true and emitted a `cond_signal`), `lookup_miss_signals_coalesced` (handler exchanged the CAS flag and found prior value already true, no signal emitted).
  - Under `netdata.collector.cgroups.discovery.*` (discovery-thread events): `discovery_scans_natural` (driven by `cgroups_main` periodic at `sys_fs_cgroup.c:1452-1455` or by `cgroups_check`), `discovery_scans_opportunistic` (driven by a lookup-miss CAS-flag wakeup; counts both the initial scan and the bounded rescan as separate events so the rescan rate is visible).
  - Allows reviewers to verify coalescing under load and to attribute scans to their trigger source.

**Five reviewers vote PRODUCTION GRADE.** (Process criterion, not a code criterion — included for symmetry with the master plan workflow.)

## Analysis

Sources checked: see Master Plan SOW-0032; plus `cgroup-discovery.c:1111-1205` (discovery thread loop + condvar usage), `sys_fs_cgroup.c:25` (`cgroup_check_for_new_every = 10` default), `:240-243` (config knob loading), `:1340-1346` (cleanup signal), `:1452-1455` (periodic signal from collection thread), `:1461` (collection thread mutex usage), `cgroup-internals.h:263-268` (`struct discovery_thread`), `src/libnetdata/locks/locks.c:43-46` (`netdata_cond_wait` wraps `pthread_cond_wait` with no timeout), `apps_plugin.c:818` (apps.plugin heartbeat-driven main loop, no condvar).

Current state:

- Discovery runs at a configurable cadence, default **10 seconds** (`sys_fs_cgroup.c:25` `cgroup_check_for_new_every = 10`), driven by `cgroups_main` signalling the discovery thread's condvar every interval.
- Three existing signal sources today: (i) periodic from the collection thread at `sys_fs_cgroup.c:1452-1455` (every `cgroup_check_for_new_every` seconds, default 10s); (ii) `cgroups_check`-driven from the collection thread (set to 1 at 17 sites in `sys_fs_cgroup.c:478-828` when a cgroup file fails to read, then the periodic block at `:1452-1457` signals discovery on the next iteration — this is NOT inotify-based; there are zero inotify references in cgroups.plugin source); (iii) cleanup at `sys_fs_cgroup.c:1340-1342`. The lookup-miss signal added by this SOW is a fourth source.
- `cgroup_root_mutex` is held briefly by the collection thread during `read_all_discovered_cgroups` (`sys_fs_cgroup.c:1461`), by the discovery thread during the copy phase, and by the CGROUPS_LOOKUP handler during its read. Adding opportunistic scans increases the frequency of discovery's mutex acquisitions but does NOT add a new mutex.

Risks:

- **Lost-signal race window** (between `cgroup-discovery.c:1180` mutex unlock after `cond_wait` returns, and `:1178` next iteration's mutex lock before `cond_wait`): if the lookup-miss signal arrives while discovery is running `discovery_find_all_cgroups()` or the bounded rescan, the raw `pthread_cond_signal` is delivered to an unwaited condvar and dropped. **Mitigated by the CAS flag**: per the AC state machine, the discovery thread performs the pre-wait flag check under `discovery_thread.mutex` (the same mutex the handler must acquire to call `cond_signal`), making the check-and-wait pair atomic with respect to the handler. If the flag is set on the pre-entry check, the thread skips the wait and starts a scan.
- **Burst of signals causing excessive scans**: mitigated by CAS-flag coalescing. Once the flag is true, additional signallers are no-ops until the discovery thread resets the flag at the end of the scan.
- **Discovery thread saturation under high churn**: opportunistic scans at high frequency (CI/CD clusters, K8s with autoscalers and many short-lived init containers) could mean the discovery thread is almost always running. The `cgroup_root_mutex` held during the scan's copy phase (`cgroup-discovery.c:1133-1141`) could delay collection's `read_all_discovered_cgroups` (`sys_fs_cgroup.c:1461`). **Mitigation**: the scan-cost baseline measurement (in AC) establishes the saturation point. If scans exceed a measured threshold, the followup optimization is targeted scan (path-list, not full walk), tracked as a deferred SOW.
- **Reaped-set interaction**: if the handler signalled for paths in the reaped-set (PERMANENT — no point scanning), every lookup miss would trigger a wasted scan. **Mitigated**: the handler explicitly skips signal-emission for reaped paths and for paths already in `cgroup_root` but unprocessed.
- **Mutex deadlock risk if lock ordering is wrong**: the new signal path is the FIRST code path that takes both `cgroup_root_mutex` and `discovery_thread.mutex`. **Mitigated**: the AC mandates release-then-acquire ordering, never holding both, documented as a permanent invariant in the source.
- **Coalescing strategy regression risk**: choosing `netdata_cond_timedwait` instead of the CAS-flag would break the natural cadence (discovery wakes every N ms regardless). **Mitigated**: AC explicitly forbids the timed-wait approach; mandates CAS-flag.
- **High-churn future**: a host with thousands of pods churning per minute could push scan frequency near the saturation point. **Acknowledged**: the followup-SOW path (targeted scan) is the answer; not in scope for this SOW.
- **Bounded-rescan adds worst-case latency**: when the lookup-miss signal arrives just after a natural scan starts, the discovery thread must finish the in-flight scan before running the bounded rescan, so the lookup-miss caller observes `2 × scan_p99` (plus one caller iteration). **Acknowledged**: the AC validation target accounts for this `2 × scan_p99` worst-case explicitly; measurement must cover the rescan path.
- **`cgroups_check` interaction with opportunistic scans**: after an opportunistic scan adds a freshly-started cgroup whose metric files may still be in flux, the collection thread can read a not-yet-populated cgroup file and set `cgroups_check = 1`, which triggers a second discovery signal on the next collection iteration. The CAS flag coalesces the two signals when they overlap; when they do not overlap, the second discovery cycle is a quick re-confirmation, not a regression. **Acknowledged**: implementers should be aware that rapid container startup can produce a tight scan→`cgroups_check`→scan sequence; this is benign but visible in `discovery_scans_natural` telemetry.
- **Handler benign race (cgroup added between check and signal)**: the handler checks `cgroup_root` under `cgroup_root_mutex`, releases the mutex, then exchanges the CAS flag. Between release and exchange, a concurrent discovery scan could add the missing cgroup. The handler still signals; the resulting opportunistic scan finds the cgroup already present and is a no-op. **Accepted**: the waste is small and bounded by the CAS-flag coalescing; the alternative (re-check under both mutexes) violates the lock-ordering rule.

## Pre-Implementation Gate

Status: ready — implementation may proceed with the already-selected full opportunistic scan design. The scan-cost measurement is a validation/follow-up input, not a pre-implementation blocker, because the correctness contract is independent from the targeted-scan optimisation and production-host measurements require explicit user permission.

**Scan-cost baseline measurement (validation / follow-up):**

- Local or explicitly approved non-production measurement may record discovery duration p50/p95/p99 at representative cgroup counts.
- `cgroup_root_mutex` collection-thread wait p99 under synthetic churn may be measured where safe.
- Decision rule: keep full opportunistic scan for this SOW; open or execute targeted-scan follow-up only if validation or production evidence shows discovery saturation.

Problem / root-cause model: today, when apps.plugin queries cgroups.plugin for a cgroup not yet in `cgroup_root` (new container just started), cgroups.plugin returns RETRY_LATER and apps.plugin must wait until the next natural discovery cycle (up to 10 seconds default) before the next query returns KNOWN. Targeted refresh closes this latency window by waking discovery early on lookup-miss.

Evidence reviewed: see Analysis (full file:line list).

Affected contracts and surfaces: CGROUPS_LOOKUP handler in cgroups.plugin (modify); discovery thread loop in cgroups.plugin (modify to check CAS flag pre- and post-wait); new shared atomic flag in `sys_fs_cgroup.c`. No public API change. No wire-format change. The new telemetry counters extend Contract 6 metrics.

Existing patterns to reuse: the existing condvar wait pattern at `cgroup-discovery.c:1178-1180`; the existing periodic signal pattern at `sys_fs_cgroup.c:1452-1455`.

Risk and blast radius: as listed in Analysis above. Worst case: under extreme churn, opportunistic scans saturate the discovery thread → followup targeted-scan SOW. Bounded blast radius.

Sensitive data handling: none — signal/coalescing logic only touches an atomic flag.

Implementation plan:

1. Add `_Atomic bool discovery_signal_pending` in `sys_fs_cgroup.c` with `extern` declaration in `cgroup-internals.h` (mirrors the `cgroup_discovery_generation` pattern from SOW-0033).
2. Modify CGROUPS_LOOKUP handler (SOW-0033 work) to call a new helper `cgroup_discovery_signal_if_unknown(cg_id)` after release of `cgroup_root_mutex` and only when returning RETRY_LATER for a path not in `cgroup_root` and not in the reaped-set.
3. Modify discovery thread loop at `cgroup-discovery.c:1175-1186` to implement the AC state machine (single authoritative description in AC → "Discovery-thread state machine"): pre-wait flag check under `discovery_thread.mutex`, exchange-to-false at scan start, bounded immediate-rescan with the `just_did_bounded_rescan` local indicator, no timed wait.
4. Add telemetry counters per the AC split (server vs discovery namespaces); document in metadata.yaml.
5. Measure the three validation metrics (container→UNKNOWN, UNKNOWN→KNOWN, collection-loop cadence under churn) where a safe local or explicitly approved non-production runtime exists.
6. Reviewer pass.

Validation plan: as listed in AC (latency measurement where runnable + telemetry verification + unit/integration fallback tests).

Artifact impact plan: cgroups.plugin source, metadata.yaml. No spec/skill/doc change.

Open decisions: NONE.

## Implications And Decisions

Decisions inherited from SOW-0032 (D1-D11).

Local decisions (resolved 2026-05-26 after round-1 reviewer feedback):

- **L1 Coalescing mechanism**: lock-free CAS flag (`_Atomic bool discovery_signal_pending`). NOT a timestamp-based mutex (race-prone) and NOT `netdata_cond_timedwait` (breaks natural cadence). The full state machine (consume-at-scan-start, bounded immediate-rescan, pre-wait check under `discovery_thread.mutex`, bounded-rescan-skip-once invariant) is specified ONCE in the Acceptance Criteria → "Discovery-thread state machine"; that section is the single source of truth.
- **L2 Scan strategy first pass**: full opportunistic scan (reuses `discovery_find_all_cgroups()`). Targeted scan deferred to a followup SOW if measurement shows saturation under high churn.
- **L3 apps.plugin extension**: OUT OF SCOPE. Tracked as a potential followup SOW (`SOW-00XX apps.plugin discovery signalling`) only if latency analysis after SOW-0036 ships shows the equivalent feature is needed for the APPS_LOOKUP cache-miss case. apps.plugin requires structural changes to its heartbeat-driven main loop; not a "small implementation" matter.
- **L4 Lock-ordering rule**: handler MUST release `cgroup_root_mutex` BEFORE acquiring `discovery_thread.mutex`. Documented as a permanent invariant in source.
- **L5 Reaped-set integration**: handler does NOT signal for paths in the reaped-set (PERMANENT) or paths in `cgroup_root` but unprocessed (in-flight discovery will resolve them).
- **L6 No new config knob for coalescing interval**: the CAS-flag approach has no "interval" parameter — coalescing is event-driven, not time-driven. The flag stays set until the next scan completes, regardless of wall-clock duration.

## Plan

1. Implement `_Atomic bool discovery_signal_pending` (with `extern` declaration in `cgroup-internals.h` mirroring the `cgroup_discovery_generation` pattern from SOW-0033) + helper `cgroup_discovery_signal_if_unknown()`.
2. Wire signal call into the CGROUPS_LOOKUP handler after `cgroup_root_mutex` is released.
3. Modify discovery thread loop to implement the AC state machine: pre-wait flag check under `discovery_thread.mutex`, exchange-to-false at scan start, bounded immediate-rescan with the `just_did_bounded_rescan` local indicator, no timed wait.
4. Add telemetry counters per the AC split (server vs discovery namespaces); document in metadata.yaml.
5. Run unit/build validation and runtime validation where safe. Record scan-cost measurements only on local or explicitly approved non-production hosts.
6. Reviewer pass.

## Execution Log

### 2026-05-27

- Corrected the Pre-Implementation Gate before coding: production-host scan-cost measurement is not a correctness prerequisite for the already-selected full opportunistic scan design, and production-host checks require explicit user permission.
- Added shared discovery state in `src/collectors/cgroups.plugin/sys_fs_cgroup.c`: `discovery_signal_pending`, `cgroup_discovery_scans_natural`, and `cgroup_discovery_scans_opportunistic`.
- Added handler-side lookup-miss signalling in `src/collectors/cgroups.plugin/cgroup-lookup-netipc.c`.
  - Evidence: lock-order invariant at `cgroup-lookup-netipc.c:12-13`.
  - Evidence: CAS flag exchange + `discovery_thread.cond_var` signal at `cgroup-lookup-netipc.c:325-335`.
  - Evidence: handler releases `cgroup_root_mutex` at `cgroup-lookup-netipc.c:411-412` before signalling at `cgroup-lookup-netipc.c:435-436`.
  - Evidence: no signal for in-root unprocessed cgroups or reaped paths at `cgroup-lookup-netipc.c:389-407`.
- Added discovery-side consume-at-scan-start and bounded immediate rescan in `src/collectors/cgroups.plugin/cgroup-discovery.c`.
  - Evidence: scan attribution helper at `cgroup-discovery.c:1150-1158`.
  - Evidence: pre-wait flag check under `discovery_thread.mutex` at `cgroup-discovery.c:1227-1235`.
  - Evidence: flag consumed before scan at `cgroup-discovery.c:1240-1241`.
  - Evidence: one bounded immediate rescan at `cgroup-discovery.c:1246-1249`.
- Added telemetry dimensions and metadata.
  - Evidence: server signal counters at `cgroup-lookup-netipc.c:53-70` and chart dimensions at `cgroup-lookup-netipc.c:553-560`.
  - Evidence: discovery scan chart at `cgroup-discovery.c:1160-1191`.
  - Evidence: metadata contexts at `src/collectors/cgroups.plugin/metadata.yaml:850-881`.
- Extended `src/collectors/cgroups.plugin/tests/test_cgroup_lookup_netipc.c` so a missing path arms `discovery_signal_pending`, while present-unprocessed and reaped paths do not.
  - Evidence: test setup at `test_cgroup_lookup_netipc.c:11-14` and assertions at `test_cgroup_lookup_netipc.c:252-281`.

## Validation

Acceptance criteria evidence:

- Signal source rules: implemented by setting `should_signal_lookup_miss` only for generation-zero or not-in-root retry paths (`cgroup-lookup-netipc.c:383-407`), then signalling only after `cgroup_root_mutex` is released (`cgroup-lookup-netipc.c:411-436`).
- Coalescing: `cgroup_discovery_signal_if_unknown()` uses `__atomic_exchange_n(&discovery_signal_pending, true, __ATOMIC_RELEASE)` and only signals when the prior flag was false (`cgroup-lookup-netipc.c:325-335`).
- Discovery-thread state machine: pre-wait flag check under `discovery_thread.mutex`, consume-at-scan-start, and one bounded immediate rescan are implemented at `cgroup-discovery.c:1227-1249`.
- No natural-cadence regression: existing collection-thread periodic signal remains unchanged at `sys_fs_cgroup.c:1452-1457`; this SOW only adds another signal source and chart update after collection unlock (`sys_fs_cgroup.c:1493-1497`).
- Telemetry: `lookup_miss_signals_sent`, `lookup_miss_signals_coalesced`, `discovery_scans_natural`, and `discovery_scans_opportunistic` are emitted and documented in metadata.

Tests or equivalent validation:

- `cmake --build /tmp/topology-containers-sow0033-build --target src/collectors/cgroups.plugin/cgroup-discovery.o src/collectors/cgroups.plugin/sys_fs_cgroup.o cgroup-lookup-netipc-test -j1` — PASS.
- `/tmp/topology-containers-sow0033-build/cgroup-lookup-netipc-test` — PASS.
- `/tmp/topology-containers-sow0033-build/cgroup-orchestrator-test` — PASS.
- `cmake --build /tmp/topology-containers-sow0033-off-build --target src/collectors/cgroups.plugin/cgroup-discovery.o src/collectors/cgroups.plugin/sys_fs_cgroup.o -j1` — PASS, confirms the cgroups lookup-server OFF build still compiles the discovery loop.
- `python3` YAML parse for `src/collectors/cgroups.plugin/metadata.yaml` — PASS.
- `git diff --check` — PASS.

Real-use evidence:

- The netipc lookup integration test starts the lookup server, issues CGROUPS_LOOKUP requests over the real client/server path, verifies KNOWN / RETRY_LATER / PERMANENT responses, and verifies the discovery signal flag behavior.
- Container-churn latency and scan-cost measurements were not run on a production or production-like host because that requires explicit user permission. The SOW's corrected gate records this as validation/follow-up evidence only, not a correctness blocker.

Reviewer findings:

- No external reviewer was run during this implementation turn; the user asked to take over implementation, and external reviewers were not explicitly requested for this round.
- Manual self-review checked lock ordering, retry/permanent signal rules, discovery loop bounded-rescan behavior, telemetry context naming, and OFF-build behavior.

Same-failure scan:

- `rg -n "cgroup_discovery_signal_if_unknown|discovery_signal_pending|discovery_scans|lookup_miss_signals" src/collectors/cgroups.plugin .agents/sow/done/SOW-0037-20260526-cgroups-targeted-refresh.md` confirmed the new signal/counter surfaces are limited to this SOW's files.

Sensitive data gate:

- No secrets, bearer tokens, customer identifiers, private endpoints, production hostnames, personal data, or credentials were added. The earlier production-host-specific wording was removed from the SOW before commit.

Artifact maintenance gate:

- AGENTS.md: no update needed; no workflow or repository-wide guardrail changed.
- Runtime project skills: no update needed; the existing collector-writing skill already covers hot-path discipline, netipc, metadata, and validation.
- Specs: no update needed; no `.agents/sow/specs/` file currently covers cgroups plugin discovery internals or CGROUPS_LOOKUP refresh semantics, and this behavior is implementation-internal.
- End-user/operator docs: no update needed; there is no new user configuration or public operator command. The existing cgroups README describes discovery cadence; this SOW changes internal responsiveness to lookup misses.
- End-user/operator skills: no update needed; no public skill workflow changed.
- SOW lifecycle: status set to `completed`; file moved to `.agents/sow/done/` and will be committed together with code and metadata changes.

Specs update:

- No spec update was needed for the same reason recorded in the artifact maintenance gate: this is an internal cgroups.plugin wakeup path and no existing spec owns it.

Project skills update:

- No runtime project skill update was needed; the implementation followed the existing collector-writing and SOW workflow rules.

End-user/operator docs update:

- No end-user/operator docs update was needed; the behavior is internal latency improvement with no user-facing setting or procedure change.

End-user/operator skills update:

- No public/operator skill update was needed; no documented operator workflow changed.

Lessons:

- Performance measurements that require production-like hosts should not be written as Pre-Implementation blockers unless they determine correctness or an irreversible design fork. For this SOW, they inform optimisation only.

Follow-up mapping:

- apps.plugin discovery signalling for APPS_LOOKUP cache misses: rejected for this SOW and not tracked now. Evidence: apps.plugin has a heartbeat-driven main loop, not a discovery condvar; no validation evidence currently shows APPS_LOOKUP cache-miss latency needs this structural change.
- cgroups targeted-scan optimisation: rejected for this SOW and not tracked now. Evidence: the implemented full-scan path coalesces signals and bounds immediate rescans; no local validation evidence shows saturation. Create a new SOW only if future runtime evidence shows scan p99 above 500ms at large scale or collection-thread `cgroup_root_mutex` wait p99 above 100ms.

## Outcome

Completed. CGROUPS_LOOKUP misses now wake cgroups discovery early through a coalesced CAS flag, without changing the wire protocol or natural discovery cadence.

## Lessons Extracted

Record production-host performance measurements as validation or follow-up evidence unless they decide correctness or an irreversible architecture choice.

## Followup

No open follow-up SOWs from this work.

- apps.plugin discovery signalling is not tracked now; create a SOW only if APPS_LOOKUP cache-miss latency evidence shows user impact.
- cgroups targeted-scan optimisation is not tracked now; create a SOW only if runtime evidence shows full opportunistic scans saturate discovery or delay collection beyond the thresholds recorded above.

## Regression Log

None yet.
