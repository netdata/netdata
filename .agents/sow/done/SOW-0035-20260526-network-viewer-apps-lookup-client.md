# SOW-0035 - Step 3: network-viewer.plugin APPS_LOOKUP client

## Status

Status: completed

Sub-state (phase-2 takeover 2026-05-27): implementation started after SOW-0033, SOW-0034, and SOW-0037 were completed. The SOW text was sanitized for durable artifact rules before coding.

Sub-state (cache-contract correction 2026-05-28): SOW-0043 supersedes the earlier generation-driven cache design in this SOW. The APPS_LOOKUP worker must wake from current working-set intake, not from an idle probe whose purpose is to refresh known entries. Function handlers enqueue current working-set PIDs on demand. `outer=KNOWN, inner=UNKNOWN_RETRY_LATER` is partial data: cache valid PID/process fields as incomplete, and re-query the PID while it remains in the current network-viewer working set.

Historical note: the earlier multi-round review narrative in this SOW is superseded by SOW-0043. It described a generation-refresh timer, a remembered last-seen PID set for generation rewarm, and whole-cache clearing on producer generation changes. That design is no longer valid and must not be implemented or reintroduced.

## Requirements

Purpose: network-viewer needs cgroup/container enrichment from apps.plugin without making the Function hot path wait on IPC and without making cache state jump between complete and incomplete snapshots.

The final contract is:

- Function handlers derive the current working-set PIDs from the Function call.
- Function handlers read the APPS_LOOKUP cache and enqueue current working-set PIDs for the worker.
- The APPS_LOOKUP worker is the only code that calls netipc.
- The worker wakes on current working-set demand or shutdown; there is no timer whose purpose is to refresh already-known entries.
- Cache entries have no TTL and are not flushed on normal producer generation changes.
- Eviction is per-entry: PID disappeared from the consumer working set, outer PID unknown, inner cgroup permanent unknown, PID starttime reuse, bounded-cache pressure, plugin cleanup, or shutdown.
- `outer=KNOWN, inner=UNKNOWN_RETRY_LATER` is partial data. It is cached as incomplete PID/process data and re-queried while the PID remains in the current network-viewer working set.

### Acceptance Criteria

- No Function handler performs APPS_LOOKUP IPC.
- No APPS_LOOKUP refresh/probe timer exists.
- No normal apps generation bump flushes the whole network-viewer APPS_LOOKUP cache.
- Every Function call can enqueue the current working-set PIDs so partial and stale entries can be refreshed on demand.
- `UNKNOWN_RETRY_LATER` is stored as an incomplete cache entry and retried on the next working-set demand.
- `UNKNOWN_PERMANENT` and outer `UNKNOWN` evict/remove the affected PID entry.
- Telemetry and docs do not advertise removed timer/probe/generation-flush counters or config.

## Analysis

Root cause corrected by SOW-0043:

- The previous design treated producer generation as a cache-invalidation signal. That was wrong for this product goal because a normal apps or cgroups collection cycle is not proof that every cached PID is invalid.
- The previous design also used a timer/probe path to observe generation changes even when no user was asking for topology. That created idle reconnects and allowed a timed refresh to overwrite a coherent container grouping with partial process-style fallback.
- The correct model is demand-driven: each Function request supplies the current PID working set; that working set drives lookup refreshes, including partial entries.

Evidence in current implementation:

- Worker response handling no longer flushes the cache on generation change and explicitly treats `UNKNOWN_RETRY_LATER` as cacheable incomplete state: `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:517` and `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:580`.
- Worker wait has no refresh timeout: `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:650`.
- Current working-set refresh/prune path is in `nv_apps_lookup_warm_pids()`: `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:807` and `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:830`.
- apps.plugin records cgroups generation only for diagnostics and evicts cgroup cache entries when a PID unlink drops the refcount to zero: `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:199` and `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:496`.

## Pre-Implementation Gate

Status: completed, corrected by SOW-0043.

Problem / root-cause model: network-viewer needs APPS_LOOKUP enrichment, but correctness depends on coherent on-demand cache updates, not generation-level cache invalidation.

Evidence reviewed: current source citations above, SOW-0043 regression analysis, and runtime validation recorded in SOW-0043.

Affected contracts and surfaces: `network-viewer-apps-lookup-client.c`, `network-viewer.c` APPS_LOOKUP warm paths, APPS_LOOKUP telemetry metadata, and the topology IPC contract spec.

Existing patterns to reuse: file-scope worker-owned netipc client, bounded dictionary cache with explicit cleanup, Function hot path cache read plus bounded enqueue.

Risk and blast radius: stale PID entries can persist if cleanup events are missed. The mitigation is current working-set pruning, PID starttime replacement, producer status eviction, and bounded-cache pressure. The Function hot path remains non-blocking.

Sensitive data handling: cgroup paths and labels can expose operator-chosen names. Raw cgroup-path fields remain a PID-mode detail and are documented as sensitive.

Implementation plan: follow the demand-driven cache contract in this SOW and SOW-0043. Do not restore the generation-refresh timer, remembered generation-rewarm set, generation-flush counter, or timer-probe counter.

Validation plan: focused APPS_LOOKUP client mock test, source grep for removed timer/generation-reset paths, installed-agent runtime query for stable `group_by=container`, and PID-mode enrichment check.

Artifact impact plan: source, test, metadata, integration docs, README, master SOW, child SOWs, and IPC spec updated as part of SOW-0043.

Open decisions: none.

## Implications And Decisions

- The cache key remains PID, with starttime held in the cache entry and checked on worker responses.
- The worker is the only netipc caller.
- The worker signal is eventfd; the wait is demand-only plus shutdown.
- Producer generation is telemetry/diagnostics only.
- Partial APPS_LOOKUP data is a first-class state, not a miss to forget forever.

## Plan

- Keep APPS_LOOKUP off the Function hot path.
- Enqueue current working-set PIDs on each Function call.
- Cache partial `UNKNOWN_RETRY_LATER` entries and retry them while current.
- Evict per PID on concrete cleanup/status/identity/bounded-cache events.
- Keep telemetry/docs aligned with the demand-driven contract.

## Execution Log

- Implemented the demand-driven APPS_LOOKUP worker and cache path. Evidence: worker response handling at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:517`, partial retry handling at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:580`, worker wait at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:650`, and current working-set warm path at `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:807`.
- Implemented per-entry current-working-set pruning instead of whole-cache invalidation. Evidence: `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:407` and `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:830`.
- Implemented apps.plugin cgroup cache eviction on PID unlink/refcount cleanup and removed generation-reset behavior. Evidence: `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:199` and `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:496`.
- Updated the focused mock-peer test so retry-later partial entries are cached and retried. Evidence: `src/collectors/network-viewer.plugin/tests/test_network_viewer_apps_lookup_client.c:217` and `src/collectors/network-viewer.plugin/tests/test_network_viewer_apps_lookup_client.c:234`.
- Updated metadata and docs to expose only `apps lookup cache size` and removed the obsolete generation-refresh knob/counters. Evidence: `src/collectors/network-viewer.plugin/metadata.yaml:61`, `src/collectors/network-viewer.plugin/README.md:71`, and `src/collectors/network-viewer.plugin/integrations/network_connections.md:71`.

## Validation

- Build/test: `sudo cmake --build build --target network-viewer-apps-lookup-client-test apps-lookup-protocol-test network-viewer.plugin apps.plugin -j 8` passed.
- Test run: `sudo build/network-viewer-apps-lookup-client-test` passed.
- Test run: `sudo build/apps-lookup-protocol-test` passed.
- Install/runtime: `./install.sh` completed and restarted `netdata`.
- Runtime sockets after restart: one live `cgroups-lookup` client appeared before Function use; one live `apps-lookup` client appeared only after a Function request, matching demand-driven lookup.
- Runtime Function validation is recorded in SOW-0043 because it covers the full cross-SOW regression.
- Hygiene: source/SOW grep for removed active code paths is part of SOW-0043 validation.

## Outcome

SOW-0035 is complete with the SOW-0043 correction applied. The final APPS_LOOKUP client is demand-driven, does not refresh already-known entries on a timer, does not invalidate the whole cache on normal producer generations, and treats partial cgroup data as retryable cache state.
