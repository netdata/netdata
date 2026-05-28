# Topology Containers IPC Contract

Status: current
Last updated: 2026-05-28

## Purpose

This spec records the durable contract for the Netdata Agent topology container enrichment chain:

- `cgroups.plugin` serves `CGROUPS_LOOKUP` on the `cgroups-lookup` netipc service.
- `apps.plugin` consumes `CGROUPS_LOOKUP`, serves `APPS_LOOKUP` on the `apps-lookup` netipc service, and keeps its normal `/proc` collection authoritative.
- `network-viewer.plugin` consumes `APPS_LOOKUP` asynchronously and uses the cache for topology and network-connections enrichment.

The goal is reliable container, orchestrator, and service identity in network topology without creating hard runtime dependencies between plugins.

## Wire Contract

- The netipc lookup layout version is `1`.
- The vendored wire-format baseline is commit `dba0a065f0`.
- Size and offset contracts are enforced by `_Static_assert` in `src/libnetdata/netipc/src/protocol/netipc_protocol.c`.
- `CGROUPS_LOOKUP` request/response headers are 16 bytes; each response item header is 28 bytes.
- `APPS_LOOKUP` request/response headers are 16 bytes; each response item header is 60 bytes; each PID/starttime key is 8 bytes.
- Producers and consumers must reject unknown layout versions. Any field addition or semantic change requires a layout-version bump and a new SOW.

## Services

- `cgroups-lookup` is the service name for `CGROUPS_LOOKUP`.
- `apps-lookup` is the service name for `APPS_LOOKUP`.
- Service names are constants in plugin source, not operator configuration.
- IPC is over local Unix domain sockets under the existing netipc runtime directory.
- A missing service is normal. Consumers must keep their existing behavior and retry later.

## Authoritative State

- `cgroups.plugin` is authoritative for discovered cgroups, cgroup names, labels, and orchestrator classification.
- `apps.plugin` is authoritative for live PIDs and PID starttime identity. It reads `/proc/<pid>/cgroup` and links PIDs to `CGROUPS_LOOKUP` cache entries when available.
- `network-viewer.plugin` is authoritative for live sockets and connection working sets. It does not perform synchronous lookup IPC from Function handlers.

Every plugin iteration refreshes its working set from its own authoritative source:

- `cgroups.plugin`: `/sys/fs/cgroup`.
- `apps.plugin`: `/proc`.
- `network-viewer.plugin`: `/proc/net/*` socket tables and related process-resolution state.

## Cache Lifetime

Lookup caches have no time-based TTL.

An entry stays valid until one of these events happens:

- The producer returns `UNKNOWN_PERMANENT` for the entry.
- The consumer observes a concrete key identity change, such as PID starttime reuse or cgroup path change.
- The consumer stops seeing the item in its own working set and prunes it as part of normal bounded-cache maintenance.
- Bounded-cache pressure or plugin shutdown removes the entry.

Consumers must not periodically refresh known entries only because time passed, and must not invalidate a whole lookup cache during normal operation only because a producer generation changed.

## Lookup Status

`CGROUPS_LOOKUP` item status:

- `KNOWN`: all returned item fields are valid and cacheable.
- `UNKNOWN_RETRY_LATER`: the producer may know later; do not cache the miss; query again only if the key remains in the consumer's working set.
- `UNKNOWN_PERMANENT`: the producer will not know this key in the current generation; evict any cached entry for the key.

`APPS_LOOKUP` has two layers:

- The outer PID status says whether apps.plugin knows the PID/starttime key.
- The inner cgroup status says whether the PID has known cgroup enrichment.

Inner cgroup status:

- `KNOWN`: cgroup enrichment is valid and cacheable.
- `UNKNOWN_RETRY_LATER`: cgroup enrichment is incomplete; if the outer PID status is `KNOWN`, cache the valid PID/process facts as incomplete and re-query while the PID remains in the consumer's current working set.
- `UNKNOWN_PERMANENT`: evict cgroup enrichment for this PID key.
- `HOST_ROOT`: the PID belongs to the host/root cgroup case; this is a valid non-container result.

Consumers must branch on the inner `cgroup_status`; outer `KNOWN` is enough to cache PID/process facts, but not enough to cache complete container enrichment.

## Generation

- `cgroups.plugin` increments `cgroup_discovery_generation` on full discovery cycles.
- `apps.plugin` increments `apps_collection_generation` on collection cycles.
- Producers include the current generation in every lookup response.
- Consumers may record observed generations for telemetry or diagnostics.
- Consumers must not invalidate whole lookup caches, rewrite all entries to retry-later, or timer-refresh known entries because of a normal generation bump.
- Per-entry status, current working-set demand, PID/cgroup cleanup, identity changes, bounded pressure, and shutdown are the cache-lifetime contract.

## Fallback And Blocking

The IPC chain is best-effort enrichment.

Required fallback behavior:

- On initial connect failure: log once at INFO, keep current behavior, and retry later.
- On disconnect or protocol error: keep current behavior and retry later.
- On peer absence: no plugin exits, no chart context disappears, and collection continues.

The current POSIX netipc client call path has no per-request timeout. A consumer that cannot tolerate blocking in its collection path must isolate IPC behind a worker thread or another non-collection-path mechanism. `network-viewer.plugin` uses an asynchronous APPS_LOOKUP worker for this reason.

## Concurrency

- IPC handlers are read-only against collection state.
- IPC handlers may take the relevant state mutex only for bounded snapshots.
- `cgroups.plugin` lookup reads `cgroup_root` under `cgroup_root_mutex`.
- `apps.plugin` lookup reads the PID table under the apps PID-table lock introduced with the lookup server.
- Collection loops remain the operational guarantee; IPC is opportunistic enrichment.

## Telemetry

Implemented telemetry contexts:

- `netdata.collector.ipc.cgroups_lookup.server.*` in `cgroups.plugin`.
- `netdata.collector.ipc.cgroups_lookup.client.*` in `apps.plugin`.
- `netdata.collector.ipc.apps_lookup.server.*` in `apps.plugin`.
- `netdata.collector.ipc.apps_lookup.client.*` in `network-viewer.plugin`.

Role-specific telemetry covers request outcomes, cache outcomes, peer lifecycle, queue or intake depth where applicable, and request/worker duration buckets where applicable. Each implemented context must be documented in the owning plugin's `metadata.yaml`.

## Security And Data Handling

- IPC is local-only over Unix domain sockets.
- IPC does not intentionally expose data unavailable to the same local `netdata` user through existing plugin state.
- Container and Kubernetes labels may contain identifying information. SOWs, tests, logs copied into durable artifacts, and documentation examples must use synthetic or redacted labels.
- Connection failures and layout mismatches are logged; per-request churn errors must avoid log flooding.

## User-Visible Topology Contract

`network-viewer.plugin` topology payloads use the enrichment cache to expose stable grouping dimensions:

- process/actor identity remains available for the existing views;
- container grouping is available when APPS_LOOKUP returns known container metadata;
- orchestrator grouping is available from the `CGROUPS_LOOKUP` orchestrator value propagated through apps.plugin;
- host/root and unknown cases remain explicit fallback states rather than fabricated container identities.

The Cloud topology aggregator must preserve the grouping fields, `view.group_by`, actor rows, and `evidence_policy: "preserve"` string form. Changing `evidence_policy` to an object is a schema change and requires Cloud schema coordination first.

## Build Knobs

- `ENABLE_CGROUPS_LOOKUP_SERVER` controls the cgroups lookup server build on Linux.
- `ENABLE_CGROUPS_LOOKUP_TEST_CLIENT` controls the test client build.
- `ENABLE_CGROUP_NAME` controls the Go `cgroup-name` helper build on Linux.
- The apps and network-viewer lookup pieces are built with their owning plugins on Linux.

These build knobs are for build composition and tests. They are not runtime feature toggles.
