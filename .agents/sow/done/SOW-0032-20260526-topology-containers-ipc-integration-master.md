# SOW-0032 - Topology Containers IPC Integration - Master Plan

## Status

Status: completed

Sub-state: completed 2026-05-27. All six implementation SOWs completed and committed; the durable IPC contract spec was added.

Sub-state (final-review cleanup 2026-05-26): post-review cleanup removed stale "Unknowns" text, removed stale placeholder wording in the affected-surface list after D9 chose `src/collectors/cgroups.plugin/cgroup-name/`, rewrote the obsolete user-decision-pass plan step after D8-D11 were recorded, and corrected D10 wording to remove a factual contradiction: netipc has no per-request timeout in the current POSIX client call path (`src/libnetdata/netipc/src/service/netipc_service.c`, `call_with_retry`). No semantic decisions changed.

## Requirements

### Purpose

Integrate the two newly-vendored netipc methods (`CGROUPS_LOOKUP`, `APPS_LOOKUP`) into the Netdata Agent so that `network-viewer.plugin` can present network topology grouped by container, orchestrator (Docker/Kubernetes/systemd/LXC/podman/nspawn/KVM), and pod/service. Each plugin in the enrichment chain becomes both a client (asks upstream) and a server (answers downstream). This is "fit for purpose" for DevOps/SREs running mixed container/service workloads — they should see network dependencies at the level of containers and services, not at the level of anonymous PIDs.

### User Request

User direction (paraphrased and committed to before this SOW was written):

1. Implement the integration in six steps, each landing as its own commit and receiving an independent readiness review before the next dependent step.
2. Steps:
   1. cgroups orchestrator identification (cgroups.plugin becomes CGROUPS_LOOKUP server with structured orchestrator data).
   2. cgroups → apps (apps.plugin becomes CGROUPS_LOOKUP client and APPS_LOOKUP server).
   3. apps → network-viewer (network-viewer.plugin becomes APPS_LOOKUP client).
   4. network-viewer new topology groupings (user-visible payoff).
   5. cgroups targeted refresh triggered by APPS_LOOKUP requests for unknown cgroups.
   6. Replace `cgroup-name.sh` with a Go binary that runs detection methods in parallel; remove the shell script entirely.
3. Reviewers must review the master plan **before** any implementation begins. Then review each implementation step.
4. Plan expressed as SOWs.

Constraints recorded during planning:

- Backwards compatibility is mandatory and non-blocking: when an IPC peer is missing or unresponsive, the consuming plugin MUST continue with its existing behavior. No new hard dependency between plugins.
- Cache lifetime: cache entries live until the underlying resource (cgroup or PID) disappears. No TTL.
- On plugin restart: cache stays as-is at the surviving side. Both apps and cgroups respond `UNKNOWN_RETRY_LATER` for items not in their own state. Callers refresh their working set every iteration from the authoritative source (cgroups.plugin from `/sys/fs/cgroup`; apps.plugin from `/proc`; network-viewer.plugin from `/proc/net/*`), so stale items naturally fall out as the resource disappears.
- Targeted refresh (step 5) ships before user-visible step 4.
- `cgroup-name.sh` is removed entirely — no compatibility shim.
- Discovery is a separate thread from data collection in cgroups.plugin. Slow discovery (waiting on a slow `docker inspect`) is acceptable; correct name resolution is more important than fast resolution. Do NOT enforce artificial timeouts that risk wrong names. The Go binary in step 6 parallelises detection methods within a single resolution; it does not impose timeouts.
- The replacement binary runs as the unprivileged `netdata` user (same as the shell script today). No privilege uplift required.

### Assistant Understanding

Facts (verified against the working tree):

- netipc protocol vendored at commit `dba0a065f0` (master branch, 2026-05-26). Wire format frozen: `NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE=16`, `NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE=16`, `NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE=28`, `NIPC_APPS_LOOKUP_REQ_HDR_SIZE=16`, `NIPC_APPS_LOOKUP_RESP_HDR_SIZE=16`, `NIPC_APPS_LOOKUP_ITEM_HDR_SIZE=60`, `NIPC_LOOKUP_DIR_ENTRY_SIZE=8`, `NIPC_LOOKUP_LABEL_ENTRY_SIZE=16`, `NIPC_APPS_LOOKUP_KEY_SIZE=8`. `_Static_assert`s validate at compile time (`src/libnetdata/netipc/src/protocol/netipc_protocol.c:200-230`). All three language bindings (C, Rust, Go) build and pass round-trip tests; Rust generated 89 cross-language fixtures and decoded all 89 successfully.
- `cgroups.plugin` is a thread inside the netdata daemon (not a separate process). Confirmed: no binary at `/usr/libexec/netdata/plugins.d/cgroups.plugin`; thread entry `cgroups_main` at `src/collectors/cgroups.plugin/sys_fs_cgroup.c:1375`.
- Discovery is a separate thread (`discovery_thread`) from collection (`cgroups_main`). They share `cgroup_root` under `cgroup_root_mutex`.
- A netipc server already exists in cgroups.plugin, serving the existing `CGROUPS_SNAPSHOT` method on UDS service name `"cgroups-snapshot"` (`src/collectors/cgroups.plugin/cgroup-netipc.c`). Its current consumer is `ebpf.plugin`. Adding `CGROUPS_LOOKUP` requires a SEPARATE netipc service (different service name, different socket) because the netipc service abstraction is per-method.
- The existing `enum cgroups_container_orchestrator` (`cgroup-internals.h:133-137`) has only `UNSET`, `UNKNOWN`, `K8S`. The netipc wire enum (`nipc_orchestrator_t`) has 8 values: `UNKNOWN`, `SYSTEMD`, `DOCKER`, `K8S`, `KVM`, `LXC`, `PODMAN`, `NSPAWN`. The plugin enum must be broadened to match.
- cgroup-name.sh has a single caller: `cgroups.plugin` via `spawn_popen_run_variadic` at `src/collectors/cgroups.plugin/cgroup-discovery.c:235`. It runs as the `netdata` user (netdata daemon drops privileges via `become_user()` at `src/daemon/daemon.c:73`).
- `apps.plugin` does **NOT** currently read `/proc/<pid>/cgroup` (verified 2026-05-26: `grep -r "cgroup" src/collectors/apps.plugin/` returns zero matches). `struct pid_stat` has no cgroup field and no `starttime` field. apps.plugin is also single-threaded with only one mutex (`apps_and_stdout_mutex`) used to serialize stdout writes. SOW-0034 must therefore (a) ADD cgroup path reading as new code, (b) add the missing fields to `struct pid_stat`, (c) add a new mutex protecting the PID table for concurrent IPC handler reads.
- `network-viewer.plugin` (`src/collectors/network-viewer.plugin/network-viewer.c`, 4622 lines) emits per-connection topology rows with limited per-PID metadata. It already distinguishes "system" vs "container" namespace types in output but does not have cgroup-path-level identity.
- Capability model: `apps.plugin` has file caps `cap_dac_read_search,cap_sys_ptrace`; `network-viewer.plugin` has `cap_dac_read_search,cap_sys_admin,cap_sys_ptrace`. `cgroups.plugin` (a thread) inherits the daemon's UID (`netdata` user) and has no file caps.

Inferences:

- The "two services per plugin" pattern (one for snapshot, one for lookup) is the cleanest extension of the existing netipc design and avoids breaking the `ebpf.plugin → cgroups-snapshot` consumer.
- An apps.plugin APPS_LOOKUP server thread parallel to the existing apps collection loop is the right structural fit (mirror of `cgroup-netipc.c`).
- Network-viewer.plugin queries APPS_LOOKUP only for the working set of PIDs visible in current network connections; the cache will stabilise quickly under normal load.
- Generation tracking is the only durable way to detect peer restart (server uptime, sequence numbers, and PID checks all fail in some scenarios — but a strictly-monotonic generation bump on every full scan is robust).

Open plan-level decisions: NONE remaining — see D8-D11 in the Decisions section.

### Acceptance Criteria

Plan-level (this SOW):

- Independent reviewers reviewed SOW-0032 and SOW-0033..0038 as a unit during Phase 1.
- Blocking reviewer findings were resolved before Phase 2 implementation began.
- All open decisions in this SOW were resolved by the user before SOW-0033 transitioned to implementation.
- Shared contracts were made durable in `.agents/sow/specs/topology-containers-ipc-contract.md`.

End-state (after all six implementation SOWs complete):

- `network-viewer.plugin` topology presents per-container and per-orchestrator groupings in addition to the existing per-PID view. SOW-0036 records the runnable validation used for this branch; external-host mixed-runtime validation was not run in this closeout because external host access requires explicit user approval.
- No regression in existing `apps.plugin`, `cgroups.plugin`, or `network-viewer.plugin` behaviour when any IPC peer is absent or unresponsive. Verified by stopping/restarting peer threads and confirming the consumer continues with its current-state behaviour.
- `cgroup-name.sh` is removed from the source tree, from packaging manifests (rpm spec, deb postinst, makeself install scripts), and from CMakeLists.txt. The replacement Go binary is installed in its place and used by `cgroups.plugin` discovery for all cgroup name resolutions.
- All seven SOWs (this master + six implementation) are marked `Status: completed` and moved to `.agents/sow/done/`.

## Analysis

Sources checked:

- `src/libnetdata/netipc/include/netipc/netipc_protocol.h` (wire format, enums, builder/decoder APIs).
- `src/libnetdata/netipc/src/protocol/netipc_protocol.c` (C codec implementation, validation rules, dispatch helpers).
- `src/libnetdata/netipc/include/netipc/netipc_service.h` (server init helpers, handler structs).
- `src/collectors/cgroups.plugin/cgroup-netipc.c` (existing CGROUPS_SNAPSHOT server pattern to mirror for CGROUPS_LOOKUP).
- `src/collectors/cgroups.plugin/cgroup-internals.h` (cgroup struct, orchestrator enum, helper inlines).
- `src/collectors/cgroups.plugin/cgroup-discovery.c` (discovery loop, rename script invocation, k8s detection at line 909).
- `src/collectors/cgroups.plugin/sys_fs_cgroup.c` (cgroups main thread, discovery thread struct, mutex).
- `src/collectors/cgroups.plugin/cgroup-name.sh.in` (shell script being replaced, 741 lines).
- `src/collectors/apps.plugin/apps_plugin.c`, `apps_pid.c`, `apps_targets.c`, `apps_os_linux.c` (apps structure and PID processing).
- `src/collectors/network-viewer.plugin/network-viewer.c` (network-viewer output structure, current container/system distinction).
- `src/daemon/daemon.c:73-148` (`become_user` privilege drop).
- `src/daemon/main.c:240` (run-as-user config default).
- `system/systemd/netdata.service.in` (User=root with bounding capabilities).
- `packaging/installer/install.sh` and `packaging/cmake/pkg-files/deb/plugin-*/postinst` (setcap entries per plugin).
- `netdata.spec.in` (rpm packaging manifest of cgroup-name.sh).
- `CMakeLists.txt:3671-3676` (cgroup-name.sh configure_file and install rules).

Current state:

- The "what each plugin knows" picture today:
  - `cgroups.plugin`: knows cgroup path, friendly name (after rename), labels, K8s detection only as boolean. Other orchestrators are not in the structured enum — `enum cgroups_container_orchestrator` has 3 values.
  - `apps.plugin`: knows PID, parent PID, command, UID, computed uptime (raw starttime is computed and discarded — only `PDF_UPTIME` derived value is stored). Does NOT read `/proc/<pid>/cgroup` today. No structured cgroup or orchestrator data. Single-threaded.
  - `network-viewer.plugin`: knows source/destination PID, command, basic namespace type (system vs container) for output rows.
- Inter-plugin awareness: zero today. Each plugin discovers its own data independently.
- The netipc machinery is in place but unused for the lookup methods. The CGROUPS_SNAPSHOT server is used by `ebpf.plugin` already.

Risks (high-level — each implementation SOW details its own):

- Concurrency contention on `cgroup_root_mutex`: the new CGROUPS_LOOKUP handler will take the same mutex as the discovery thread. Long lookups could delay discovery, and vice versa. Mitigation: lookups are O(matching only, not full scan), handler holds the mutex only while reading.
- Cache-invalidation correctness across plugin restarts. Mitigation: generation-driven eviction (Contract 4) + caller iteration discipline (Contract 3).
- Apps.plugin becoming an APPS_LOOKUP server may add CPU under high-PID workloads. Mitigation: handler is read-only against existing per-PID state; no extra collection work.
- Network-viewer cold-start: empty cache, burst of APPS_LOOKUP queries. Mitigation: bounded by the number of active connections, not the number of all PIDs.
- IPC peer absent → consumer must degrade silently to its current behaviour. Mitigation: every implementation SOW must include an explicit fallback test in its validation plan.
- Go binary in step 6 must produce byte-identical output to cgroup-name.sh for all detection cases that succeed today. Mitigation: behavioural-parity test using captured shell-script outputs across container distros (Ubuntu, Debian, RHEL, Manjaro; with Docker, Podman, Kubernetes, systemd-nspawn, LXC). Step 6's SOW will list the test matrix.
- Telemetry: each plugin must emit consistent counters under known chart contexts so reviewers can audit IPC health. Mitigation: Contract 6 fixes the counter names and chart context names; all six implementation SOWs follow it.

## Pre-Implementation Gate

Status: satisfied. All plan-level decisions were resolved, all implementation SOWs completed, and the durable contract spec was added.

Problem / root-cause model:

- Network-viewer cannot currently present per-container topology because per-PID metadata available to it does not include the cgroup path or the orchestrator that owns the cgroup. The information exists in cgroups.plugin and is partially derivable in apps.plugin, but the two plugins have no IPC mechanism to share it. The vendored netipc CGROUPS_LOOKUP and APPS_LOOKUP methods provide the transport; this plan is the integration.

Evidence reviewed:

- See "Sources checked" above. Each citation is a file path; line numbers cited inline where specific behaviour is referenced.
- `dba0a065f0` netipc vendor commit on master branch (2026-05-26): full wire format, validation rules, three-language bindings, 329 Rust tests + Go protocol tests + 89 cross-language fixtures all passing.

Affected contracts and surfaces:

- C code in `src/collectors/cgroups.plugin/`, `src/collectors/apps.plugin/`, `src/collectors/network-viewer.plugin/`.
- New Go code under `src/collectors/cgroups.plugin/cgroup-name/` (per D9).
- CMakeLists.txt: add CGROUPS_LOOKUP / APPS_LOOKUP server thread sources; add the Go binary build/install; remove cgroup-name.sh configure_file and install rule.
- `netdata.spec.in`, `packaging/cmake/pkg-files/deb/*/postinst`, `packaging/makeself/install-or-update.sh`: remove cgroup-name.sh reference; add new Go binary reference (if it ends up installed under `plugins.d/`).
- netipc service registry on the filesystem under `os_run_dir(true)`: new UDS sockets `cgroups-lookup.sock` and `apps-lookup.sock` join the existing `cgroups-snapshot.sock`.
- Internal telemetry charts under app/netdata internals: new counters for IPC request counts, latencies, cache hits/misses, peer connect attempts (per Contract 6).
- End-user-facing taxonomy: network-viewer topology output gains new group dimensions (per step 4). Update integration metadata.yaml + taxonomy.yaml for network-viewer.
- Specs under `.agents/sow/specs/`: ADD `topology-containers-ipc-contract.md` that captures Contracts 1-9 below as a durable artifact (referenced by all six implementation SOWs).
- Project skills: `project-create-topology` and `project-writing-collectors` may need pointers to the new IPC contract — assess at end of step 4 / step 6.

Existing patterns to reuse:

- `cgroup-netipc.c` as the canonical template for adding a netipc server thread inside a C plugin (init / run loop / cleanup / mutex coordination with the plugin's own state).
- `spawn_popen_run_variadic` and `spawn_popen_wait` for invoking the new Go binary (unchanged from the current shell-script invocation pattern — only the target executable changes).
- The existing `discovery_thread` struct and condition-variable wakeup pattern for triggering targeted refresh (step 5).
- For step 6's Go binary: existing `go.d` infrastructure conventions (build tags, `Makefile`, golangci-lint configuration). Use them even though the binary is a one-shot CLI, not a daemon.

Risk and blast radius:

- High-risk areas: cgroup_root_mutex contention; APPS_LOOKUP handler thread interactions with the apps collection loop; cache-invalidation correctness across restart scenarios; behavioural parity of the Go binary vs the shell script.
- Medium-risk areas: network-viewer topology grouping correctness (user-visible); telemetry consistency.
- Low-risk: wire format compatibility (frozen and validated); per-plugin fallback to current behaviour (each SOW must demonstrate it).
- Operational: removing cgroup-name.sh in step 6 affects packaging artifacts. All packaging variants (rpm, deb, makeself, static, container, snap) must be updated in the same SOW.

Sensitive data handling plan:

- Cgroup labels (K8s pod labels, Docker container labels) can contain customer-identifying information. They are already exposed today via Netdata's existing label mechanism, so the IPC integration does not change the threat surface — but SOWs and validation evidence MUST follow the existing repository policy: redact customer/account/endpoint identifiers, use stable aliases like `customer-a` only if needed, never paste raw labels.
- Validation evidence in SOWs must NOT include real cgroup names, container IDs, or pod labels from production systems. Use synthetic fixtures or local test containers only.

Implementation plan (the six implementation SOWs in execution order):

1. **SOW-0033 (Step 1)** - cgroups orchestrator identification + CGROUPS_LOOKUP server. Foundation. No external clients yet; tested standalone with a test client tool.
2. **SOW-0034 (Step 2)** - apps.plugin becomes CGROUPS_LOOKUP client and APPS_LOOKUP server. First end-to-end exercise of the cgroups→apps path. No external apps client yet.
3. **SOW-0037 (Step 5)** - cgroups targeted refresh from CGROUPS_LOOKUP request signals. Shipped before user-visible work to avoid showing stale data to users.
4. **SOW-0035 (Step 3)** - network-viewer.plugin becomes APPS_LOOKUP client. End-to-end chain alive; no UI grouping change yet.
5. **SOW-0036 (Step 4)** - network-viewer.plugin presents new container/orchestrator topology groupings. First user-visible payoff.
6. **SOW-0038 (Step 6)** - cgroup-name.sh → Go binary; remove shell script. Independent of the IPC chain; can run in parallel with any of the above. Recommended schedule: in parallel with Step 1 since it has no dependency on netipc.

Validation plan (per Contract framework):

- Each step has its own validation gate (see per-SOW Acceptance Criteria).
- Cross-step integration validation runs after Step 4 lands: bring up a synthetic 3-tier workload (containerised) and inspect network-viewer Function output.
- Real-use validation runs on an explicitly approved representative validation host after step 4 when external-host access is needed. No external customer system is used.
- Reviewer voting (PRODUCTION GRADE READY / NOT) happens after each implementation SOW's PR is opened and CI passes.

Artifact impact plan:

- AGENTS.md: no change expected — the SOW framework remains as-is.
- Runtime project skills: `project-writing-collectors` and `project-create-topology` may gain a paragraph pointing to the new spec; assessed at end of step 4.
- Specs: ADD `.agents/sow/specs/topology-containers-ipc-contract.md` (the canonical reference for the nine shared contracts below). Created as part of completing SOW-0032 (after reviewer approval), not as part of step 1.
- End-user/operator docs: network-viewer integration `metadata.yaml` and the topology section of the public docs gain a description of container/orchestrator grouping. Updated in step 4.
- End-user/operator skills: `query-netdata-cloud` and `query-netdata-agents` may need a how-to entry for "query network topology grouped by container". Assessed at end of step 4.
- SOW lifecycle: this SOW transitions to in-progress when reviewers approve the plan; it transitions to completed when SOW-0038 completes and the contract spec is committed. The six implementation SOWs are each split, reviewed, and completed independently.

Open-source reference evidence:

- None checked for the master plan itself. Each implementation SOW lists OSS references where relevant (e.g. step 4's UI grouping may reference Datadog / Grafana network topology UX for prior art; step 6's Go binary may reference how `cadvisor` or `prometheus` node-exporter detect container runtimes).

Open decisions (none remaining at plan level — all four resolved as decisions D8-D11 in "Implications And Decisions" below). Per-step local decisions remain inside each child SOW and will be resolved when that SOW transitions to in-progress.

## Implications And Decisions

User decisions already recorded (before this SOW was written):

- **D1** Backwards compatibility is mandatory. Consumer plugins MUST continue working when an IPC peer is missing or unresponsive. No new hard inter-plugin dependency.
- **D2** Cache lifetime = until the underlying resource (cgroup or PID) disappears. No TTL-based eviction.
- **D3** On plugin restart, surviving caches stay as-is. Both server-side plugins respond `UNKNOWN_RETRY_LATER` for unknown items. Callers refresh their working set every iteration from authoritative sources; stale items fall out naturally as the resource is no longer queried.
- **D4** Targeted refresh (step 5) ships before user-visible step 4. No user-visible feature ships with a known stale-data window from the cache-miss latency cycle.
- **D5** `cgroup-name.sh` is removed entirely. No compatibility shim, no deprecation grace period.
- **D6** No artificial timeouts in the Go binary or anywhere in the cgroup-name resolution path. Discovery is on a separate thread from data collection; slow discovery is acceptable; correctness (the right name eventually) wins over speed (a wrong "fallback" name immediately). The binary's reliability win comes from running detection methods in parallel within a single resolution, not from enforcing deadlines.
- **D7** No changes to `spawn_popen_wait()` or any netdata spawn-server code. Timeout responsibility (where it exists at all) belongs to the spawned callee, not the caller.
- **D8** Execution order: Step 6 (Go binary) runs in parallel with Step 1 (cgroups CGROUPS_LOOKUP server). After that the sequential chain is 1 → 2 → 5 → 3 → 4. Rationale: step 6 is independent of the IPC chain and delivers immediate value (faster discovery via parallel detection methods). Step 5 (targeted refresh) ships before user-visible step 4 (network-viewer groupings) per D4.
- **D9** Go binary location: `src/collectors/cgroups.plugin/cgroup-name/` (collocated with the C plugin it serves). Install path remains `${libexecdir}/netdata/plugins.d/cgroup-name`. Rationale: the binary is a private helper of cgroups.plugin, not a general-purpose tool. CMakeLists.txt install rules stay adjacent to the related C install rules.
- **D10** IPC enable knob: NONE (implicit). Each consumer plugin tries its peer at startup; on connect failure or transport-level disconnect, the consumer plugin silently falls back to its current-state behaviour and retries connect every 30s. netipc has NO per-request timeout (verified at src/libnetdata/netipc/src/netipc_service.c — call_with_retry contains no timeout primitive); consumers must therefore architect their use of netipc to handle the possibility of an unboundedly-slow response, either via an async worker thread (e.g. SOW-0035 worker pattern) or by accepting that the server will block the IPC client thread indefinitely under peer pathology. No `[plugin:*] use ipc ... = yes|no` config option. Rationale: knobs accrete; the contract per Contract 5 is "best effort, must work without peer".
- **D11** Service names are hard-coded constants in plugin source: `CGROUP_NETIPC_LOOKUP_SERVICE_NAME = "cgroups-lookup"`, `APPS_NETIPC_LOOKUP_SERVICE_NAME = "apps-lookup"`. The existing `"cgroups-snapshot"` precedent stands. Rationale: stable, predictable wire-up; no operator footgun.

All plan-level open decisions are now resolved. Per-step local decisions (see each child SOW's "Implications And Decisions" section) will be resolved when that SOW transitions to in-progress, with reviewer input invited at that time.

## Shared Contracts

These nine contracts apply to all six implementation SOWs. When SOW-0032 reaches `completed` they MUST also exist as a durable spec at `.agents/sow/specs/topology-containers-ipc-contract.md`. Implementation SOWs cite this section (or the spec) instead of re-stating the rules.

**Contract 1 — Cache lifetime.**
Cache entries in any consumer (apps.plugin caching CGROUPS_LOOKUP results, network-viewer caching APPS_LOOKUP results) live until one of:

- The producer signals `UNKNOWN_PERMANENT` for a previously-KNOWN entry → evict.
- The producer signals a new generation (Contract 4) → evict everything from the previous generation.
- The consumer's own iteration discipline (Contract 3) stops asking about the key → entry naturally ages out via normal cache pruning (LRU bounded by max-known-set is acceptable; no TTL on the time axis).

No TTL-based eviction. No "refresh every N seconds even if known". The cache is a faithful mirror of what the producer has told us, scoped to what we still care about.

**Contract 2 — Three-state UNKNOWN semantics.**

- `KNOWN` → data is valid; consumer caches and uses it.
- `UNKNOWN_RETRY_LATER` → producer doesn't know yet; consumer does NOT cache; consumer asks again on the next iteration where the key is still in its working set.
- `UNKNOWN_PERMANENT` → producer will never know (resource gone); consumer evicts from cache and never asks again about this key (within the current generation).

For APPS_LOOKUP, the outer `status` field is two-state (KNOWN/UNKNOWN); the inner `cgroup_status` field is four-state (KNOWN/RETRY/PERMANENT/HOST_ROOT). Same semantics, applied per-field.

**Contract 3 — Caller iteration discipline.**
Every plugin iteration refreshes its working set from the authoritative source:

- cgroups.plugin from `/sys/fs/cgroup` (discovery loop).
- apps.plugin from `/proc` (PID enumeration).
- network-viewer.plugin from `/proc/net/tcp`/`tcp6`/`udp`/`udp6`/etc. (active connection enumeration).

The plugin queries IPC only for items currently in its working set. Items that have left the working set (exited container, exited PID, closed socket) are no longer asked about. This is the eviction mechanism for stale entries — the cache only contains entries the consumer still cares about.

**Contract 4 — Generation semantics.**
Each producer maintains a strictly-monotonic `generation` counter, bumped on every full scan (discovery cycle for cgroups, collection cycle for apps). The generation is returned in every CGROUPS_LOOKUP / APPS_LOOKUP response. Consumers compare the generation on each response; on bump:

- Evict all cached entries from the previous generation (their data may be stale relative to the new producer state).
- Re-query items still in the working set during the next iteration.
- Continue serving from cache for already-cached entries that match the new generation (the cache is replenished gradually as the consumer's working set is re-validated).

The generation field is the only durable mechanism for detecting peer restart. Wall clock, PID, sequence numbers, and uptime are all unreliable across restart.

**Contract 5 — Fallback when peer absent.**
Every IPC-enabled plugin MUST work standalone when the peer is absent or unresponsive. Required behaviour:

- Connect-once at startup. On failure: log once at INFO, fall back to current-state behaviour, retry every N seconds (default 30s).
- The current netipc POSIX client call path has no per-request timeout. Consumers that cannot tolerate a blocking call in their collection path MUST isolate IPC behind a worker thread or otherwise keep the collection path independent of peer responsiveness.
- On connect failure, disconnect, protocol error, or worker-level failure: continue with current-state behaviour (consumer keeps whatever it had cached, or treats the key as UNKNOWN), then retry on the next eligible iteration.
- No panic, no exit, no degraded chart contexts. The plugin's existing functionality is unaffected.

This is testable: stop the producer thread (or kill its netipc server worker), confirm the consumer keeps producing data with its existing behaviour, and restart the producer to confirm the consumer reconnects.

**Contract 6 — Telemetry.**
Each IPC-enabled plugin emits telemetry under consistent chart contexts (per plugin: `netdata.collector.ipc.cgroups_lookup.*` for the cgroups-lookup client/server and `netdata.collector.ipc.apps_lookup.*` for the apps-lookup client/server). The exact counter set is role-specific and includes the applicable subset of:

- `requests_sent` — total requests sent to peer.
- `requests_responded` — total successful responses received.
- `requests_timeout` — total requests that timed out where the role has an explicit timeout or timeout-equivalent worker failure path.
- `requests_error` — total requests that returned a protocol error.
- `cache_hits`, `cache_misses_retry`, `cache_misses_permanent`, `cache_evictions` — cache behaviour.
- `peer_connect_attempts`, `peer_disconnects` — connection lifecycle.

Plus a histogram of per-request round-trip latency: `request_duration_ms`.

Each IPC-enabled plugin's `metadata.yaml` documents these contexts so they appear as standard plugin telemetry.

**Contract 7 — Security.**

- All IPC over UDS only (per netipc design). Filesystem permissions on the socket restrict to the `netdata` user.
- No data crosses the IPC boundary that wasn't already accessible to both plugins running as the same `netdata` user.
- Labels (container labels, K8s pod labels) may contain customer-identifying information. They are handled per the existing repository-wide label exposure policy; the IPC layer is transparent.
- Audit log: connection failures, layout-version mismatches, and authentication failures are logged at ERROR level. Successful connect at INFO. Per-request errors at DEBUG (to avoid log flooding under churn).

**Contract 8 — Schema version.**
The netipc wire format is frozen at commit `dba0a065f0` (2026-05-26). All implementation SOWs use `layout_version = 1`. Any subsequent field addition or semantic change requires a `layout_version` bump and user approval. Producers MUST always accept `layout_version = 1` (decoder forward-compat rule from the netipc design). Consumers MUST validate `layout_version` on every response.

**Contract 9 — Discovery vs collection separation.**
In cgroups.plugin and apps.plugin, IPC server/client work that can block on a peer runs outside the data-collection loop. IPC handlers may take the mutex that protects shared state (`cgroup_root_mutex`, apps PID table) only for bounded read-only snapshots; they do not modify collection state. Each plugin's existing collection cadence is the operational guarantee; IPC is opportunistic enrichment and must not introduce a hard dependency on a peer.

## Plan

1. **Reviewer pass on the plan** (this SOW + the six implementation SOWs).
   - Review all seven SOWs together before implementation starts.
   - Reviewers vote READY / NOT on the plan with rationale.
   - Iterate until blocking findings are resolved. Repeated reviews use the same scope plus short notes about fixes applied.
2. **Plan-level decisions resolved.** D8 (execution order), D9 (Go binary location), D10 (IPC enable knob), D11 (service name hardcoding) are all recorded above under "Decisions already recorded". No further user input required at the master-plan level; per-step local decisions are owned by each implementation SOW.
3. **Spec creation.** When SOW-0032 transitions to completed (after plan approval), commit `.agents/sow/specs/topology-containers-ipc-contract.md` containing the nine contracts. This becomes the durable reference for the six implementation SOWs.
4. **Implementation pass.** Execute SOW-0033..0038 one at a time in the user-approved order. Each implementation SOW follows its own Pre-Implementation Gate → implementation → multi-reviewer pass → PRODUCTION GRADE READY vote → merge → SOW transitions to completed.
5. **Integration validation.** After step 4 (SOW-0036) completes, run end-to-end validation on an explicitly approved representative validation host when available: synthetic mixed workload, confirm topology output, inspect telemetry, verify fallback by stopping each peer in turn.
6. **Master SOW close.** When SOW-0038 is completed and integration validation passes, this SOW transitions to completed and moves to `.agents/sow/done/`.

## Execution Log

### 2026-05-26

- Master plan drafted. No code touched.
- Verified context: netipc vendored at `dba0a065f0`; cgroups.plugin is a thread; cgroup-name.sh runs as `netdata` user; `cgroups-snapshot` netipc service already exists; existing `enum cgroups_container_orchestrator` only has UNSET/UNKNOWN/K8S.
- Recorded user decisions D1-D7.
- Listed four open decisions for resolution before SOW-0033 starts.

### 2026-05-27

- Completed SOW-0033 in commit `db74f17d6c` (`cgroups: add lookup netipc server`).
- Completed SOW-0034 in commit `fdf9cb6beb` (`apps: add lookup netipc bridge`).
- Completed SOW-0037 in commit `02978eeb03` (`cgroups: wake discovery on lookup misses`).
- Completed SOW-0035 in commit `fa74a23cd3` (`network-viewer: warm apps lookup cache asynchronously`).
- Completed SOW-0036 in commit `bf1c681467` (`network-viewer: add topology container groupings`).
- Completed SOW-0038 in commit `da92411f54` (`cgroups: replace cgroup-name shell helper`).
- Added `.agents/sow/specs/topology-containers-ipc-contract.md`.

## Validation

Acceptance criteria evidence:

- SOW-0033..0038 are all `Status: completed` and live under `.agents/sow/done/`.
- The implementation chain completed in the planned dependency order: SOW-0033, SOW-0034, SOW-0037, SOW-0035, SOW-0036, with SOW-0038 completed independently.
- The durable contract spec was added at `.agents/sow/specs/topology-containers-ipc-contract.md`.
- This master SOW is `Status: completed` and moved to `.agents/sow/done/`.

Tests or equivalent validation:

- The code-level validation is recorded in each child SOW's `## Validation` section.
- Master-level structural validation: `.agents/sow/audit.sh` passed. It reported only the pre-existing non-project skill classification warnings already present in this repository's SOW framework.

Real-use evidence:

- Real-use and runnable smoke evidence is recorded in the child SOWs. No external-host validation was run during this final master closeout because external host access requires explicit user approval.

Reviewer findings:

- Phase 1 reviewer findings were resolved before Phase 2 implementation began. Child SOWs record their own reviewer findings and implementation-time fixes.

Same-failure scan:

- Searched done/ and pending/ SOWs for prior topology-containers / cgroups-IPC / apps-IPC work. None overlap with this scope. Closest: SOW-0028 (topology mode correlation), SOW-0031 (topology v1 zero-heuristic) — both address presentation contracts, not the producer/consumer IPC chain.

Sensitive data gate:

- This SOW contains no raw secrets, no customer names, no real cgroup labels, no production endpoints, and no personal host references.

Artifact maintenance gate:

- AGENTS.md: no update needed (SOW framework intact).
- Runtime project skills: assessed and updated in child SOWs where affected; no additional master-level runtime skill update needed.
- Specs: added `.agents/sow/specs/topology-containers-ipc-contract.md`.
- End-user/operator docs: handled in child SOWs where user-visible behavior changed.
- End-user/operator skills: child SOWs recorded evidence-backed reasons that no public operator skill change was required.
- SOW lifecycle: SOW-0033..0038 and this master SOW are completed and in `.agents/sow/done/`.

Specs update:

- Added `.agents/sow/specs/topology-containers-ipc-contract.md`.

Project skills update:

- Assessed in child SOWs. No additional master-level change needed.

End-user/operator docs update:

- Covered by SOW-0036 and SOW-0038 where behavior changed.

End-user/operator skills update:

- Assessed in child SOWs. No additional master-level public skill change needed.

Lessons:

- A plan-level spec must be kept aligned with implementation reality; the original Contract 5 timeout wording was corrected because the current netipc POSIX client path has no per-request timeout primitive.

Follow-up mapping:

- Six implementation SOWs (SOW-0033..0038) were the followups of this master plan. All are completed and moved to `.agents/sow/done/`.

## Outcome

Completed. The topology containers IPC integration plan produced six completed implementation SOWs, each committed separately, and a durable shared-contract spec for ongoing maintenance.

## Lessons Extracted

- Keep plan-level contracts short and durable; detailed implementation trade-offs belong in child SOWs.
- Avoid treating performance measurements as pre-implementation blockers unless they decide correctness or an irreversible architecture fork.

## Followup

The six implementation SOWs were explicit followups and are completed:

- SOW-0033 - cgroups orchestrator identification + CGROUPS_LOOKUP server
- SOW-0034 - apps.plugin CGROUPS_LOOKUP client + APPS_LOOKUP server
- SOW-0035 - network-viewer APPS_LOOKUP client
- SOW-0036 - network-viewer topology groupings (user-visible)
- SOW-0037 - cgroups targeted refresh
- SOW-0038 - cgroup-name Go binary; remove cgroup-name.sh

## Regression Log

None yet.
