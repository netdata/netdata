# SOW-0043 - topology container cache coherence regression

## Status

Status: completed

Sub-state: completed 2026-05-28. Implementation, install/restart, runtime
Function validation, SOW/spec repair, and hygiene checks are recorded below.

## Requirements

### Purpose

Make `topology:network-connections` fit the product purpose: users can choose
the actor level of the network topology, at minimum `process_name`, `pid`, and
`container`, and can inspect actor details appropriate to that actor grain.

The cache chain that powers this must be stable and on-demand:

- no cache is invalidated as a whole during normal operation;
- no timer exists only to refresh already-known cache entries;
- lookup updates are driven by current working-set demand;
- cache entries are evicted by explicit producer status, PID/cgroup cleanup,
  path/starttime changes, bounded-cache pressure, or plugin shutdown.

### User Request

The user reported that the current implementation does not match the agreed
design and asked for an SOW first, then analysis of each bug before code is
changed.

Reported bugs and product gaps:

1. Cache update design is wrong: cgroups/apps lookup caches must not be flushed
   wholesale and must not use timer refreshes. Updates should happen on demand,
   using netipc throughput, and evictions should happen on explicit cleanup
   events.
2. `group_by=container` periodically degrades into process-name-like actors.
   The view can change under automatic refresh, about once per minute.
3. When the fallback/mixed update happens, some process actors are not linked
   to `self` or to any endpoint, so they float in the topology.
4. Per-PID view must show all additional enrichment fields.
5. Grouped views must not show variable per-PID fields as scalar facts, but the
   additional enrichment facts still need a correct merge/presentation model
   when the product wants them available for inspection.
6. Actor modal/details do not currently present the enriched fields anywhere
   useful.
7. The Cloud topology aggregation/schema behavior must be understood before
   choosing the grouped-attribute merge model.
8. The removed `cgroup-paths:show|hide` control must stay removed from this
   product surface.

### Assistant Understanding

Facts:

- SOW-0032 records the user decision that cache lifetime is until the underlying
  cgroup or PID disappears and that there is no TTL-based eviction.
- The same SOW also introduced a contradictory generation contract that evicts
  all cached entries from a previous generation.
- The current `network-viewer.plugin` APPS_LOOKUP client has both a refresh
  timer and a whole-cache flush on response-generation bump.
- The current `apps.plugin` CGROUPS_LOOKUP client resets all cgroup lookup
  cache entries to retry-later on cgroups generation bump.
- Current topology actor creation and link creation recompute container actor
  identity from the live async cache in separate passes.
- Current modal identification is driven from `actor_labels`; enriched fields
  are not added to labels except for `container_name` in container mode and
  basic PID fields in PID mode.
- Current Cloud schema supports a per-column `aggregation` string. The Cloud
  aggregator can merge conflicts with declared aggregation such as `set`; it
  rejects conflicting non-aggregatable values.
- The current source and docs already removed the `cgroup-paths` user control
  from the active Function metadata path. Historical SOW text still mentions it.

Inferences:

- The periodic mixed topology is most likely caused by the network-viewer
  worker's timer probe observing an apps generation bump, flushing the whole
  APPS_LOOKUP cache, and then repopulating only a small subset before the next
  Function render.
- Floating actors are most likely caused by actor rows and links using different
  cache snapshots inside the same render.
- The implementation followed parts of the later child-SOW generation design,
  but that design conflicts with the user's stated cache-lifetime contract and
  the master-plan no-refresh language.

Unknowns:

- Whether Cloud frontend is also mixing older cached payloads into the local
  refresh sequence has not been proven. Current direct-agent evidence is enough
  to fix the Agent cache/render consistency first.
- Grouped-attribute presentation is resolved by user decision 1A on
  2026-05-28: grouped actors show no merged enrichment; users drill into
  `group_by=pid` for raw per-PID details.

### Acceptance Criteria

- Cache contract is corrected in SOW/spec text: no normal-operation whole-cache
  invalidation and no timer refresh of known entries.
- `apps.plugin` no longer resets all CGROUPS_LOOKUP cache entries on cgroup
  discovery generation bump.
- `network-viewer.plugin` no longer flushes the full APPS_LOOKUP cache on normal
  apps collection generation bumps.
- `network-viewer.plugin` no longer has a timer whose purpose is to refresh or
  probe known APPS_LOOKUP entries.
- Current working-set misses still trigger on-demand lookup and are retried
  while the key remains in the working set.
- Cache entries are evicted on explicit cleanup/status events: PID exit,
  PID starttime reuse, cgroup path change, outer PID `UNKNOWN`, APPS_LOOKUP
  cgroup `UNKNOWN_PERMANENT`, bounded-cache pressure, or plugin shutdown.
  CGROUPS_LOOKUP `UNKNOWN_PERMANENT` remains a referenced negative cache entry
  in apps.plugin until the last PID reference disappears, to avoid a retry loop.
- One topology render uses one stable per-render PID-to-container snapshot for
  actor identity, actor rows, ownership links, and socket links.
- `group_by=container` responses contain container actors for local process
  actors and do not intermittently return unlinked process actors.
- `group_by=pid` exposes the full per-PID enrichment fields in actor rows and
  actor modal labels/details.
- `group_by=process_name` and `group_by=container` do not expose variable
  per-PID fields as scalar actor facts.
- Grouped enrichment is not exposed in grouped actor rows or modals; raw
  variable details are available only through `group_by=pid`.
- Partial APPS_LOOKUP responses (`PID_KNOWN` with cgroup
  `UNKNOWN_RETRY_LATER`) are cached as incomplete PID facts and re-queried while
  the PID remains in the current network-viewer working set.
- `cgroup-paths` does not appear in active Function info metadata, docs, public
  skills, or tests, except historical SOW text.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0032-20260526-topology-containers-ipc-integration-master.md`
- `.agents/sow/done/SOW-0034-20260526-apps-plugin-lookup-client-and-server.md`
- `.agents/sow/done/SOW-0035-20260526-network-viewer-apps-lookup-client.md`
- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`
- `.agents/sow/specs/topology-containers-ipc-contract.md`
- `.agents/sow/specs/topology-function-schema.md`
- `.agents/skills/project-create-topology/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `src/collectors/apps.plugin/apps-cgroups-lookup-client.c`
- `src/collectors/apps.plugin/apps-lookup-netipc.c`
- `src/collectors/apps.plugin/apps_plugin.c`
- `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/collectors/network-viewer.plugin/metadata.yaml`
- `src/collectors/network-viewer.plugin/README.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `docs/netdata-ai/skills/query-netdata-cloud/query-topology.md`
- `docs/netdata-ai/skills/query-netdata-agents/query-topology.md`
- `netdata/cloud-topology-service @ 3199152`
  - `internal/topology/schema/payload.go`
  - `internal/topology/schema/column.go`
  - `internal/topology/aggregate/aggregate.go`

### Bug 1 - cache design violates the agreed no-timer/no-whole-invalidation model

Evidence:

- Master-plan decision D2 says cache lifetime is until the underlying resource
  disappears and explicitly says no TTL-based eviction:
  `.agents/sow/done/SOW-0032-20260526-topology-containers-ipc-integration-master.md:200`.
- Master contract 1 says "No TTL-based eviction. No refresh every N seconds
  even if known":
  `.agents/sow/done/SOW-0032-20260526-topology-containers-ipc-integration-master.md:218`.
- The same master contract later contradicts this by requiring generation bump
  to evict all cached entries:
  `.agents/sow/done/SOW-0032-20260526-topology-containers-ipc-integration-master.md:244`.
- The durable spec repeats that generation bump invalidates generation-scoped
  entries:
  `.agents/sow/specs/topology-containers-ipc-contract.md:47`.
- `network-viewer.plugin` has a refresh interval variable:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:55`.
- Its worker polls with that timeout and creates a probe request when the timer
  fires:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:645`.
- It flushes the entire APPS_LOOKUP cache on response generation bump:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:525`.
- `apps.plugin` resets every CGROUPS_LOOKUP cache entry to retry-later on
  cgroups generation bump:
  `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:315` and
  `src/collectors/apps.plugin/apps-cgroups-lookup-client.c:499`.

Finding:

- The user is correct. The implementation does not match the no-timer,
  no-whole-invalidation model. It implemented the later generation-flush
  interpretation instead.

Repair direction:

- Treat producer generations as diagnostic/restart hints only, not as normal
  "flush all cache" triggers.
- Keep known entries until their concrete key becomes invalid: PID exit,
  starttime change, cgroup path change, producer `UNKNOWN_PERMANENT`, bounded
  cache eviction, or plugin shutdown.
- For `CGROUPS_LOOKUP` `UNKNOWN_RETRY_LATER`, do not cache a complete result;
  keep querying only while the cgroup path remains referenced by live PIDs.
- For `APPS_LOOKUP` `outer=KNOWN, inner=UNKNOWN_RETRY_LATER`, cache the valid
  PID/process facts as incomplete and keep querying while the PID remains in the
  current network-viewer working set.

### Bug 2 - periodic container view fallback

Evidence:

- `apps.plugin` increments `apps_collection_generation` on every collection
  cycle:
  `src/collectors/apps.plugin/apps_plugin.c:869`.
- APPS_LOOKUP responses return that collection generation:
  `src/collectors/apps.plugin/apps-lookup-netipc.c:234`.
- `network-viewer.plugin` flushes the entire cache when that generation is
  higher than the previous observed value:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:525`.
- The network-viewer worker issues timer probes even when no PID miss exists:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:669`.

Finding:

- A normal apps collection tick is enough to make the next network-viewer timer
  probe flush APPS_LOOKUP cache state. After that, `group_by=container` can
  derive many actor names from the process fallback until on-demand warming
  catches up.

Repair direction:

- Remove timer probes.
- Remove whole-cache flush on normal apps collection generation.
- Drive warming from the Function working set and any explicit collection
  cleanup signals.

### Bug 3 - fallback/mixed actors can become unlinked

Evidence:

- Container actor fields default `container_name` to process name before cache
  lookup:
  `src/collectors/network-viewer.plugin/network-viewer.c:960`.
- If `nv_cache_lookup_pid()` misses, the fallback process name remains:
  `src/collectors/network-viewer.plugin/network-viewer.c:967`.
- Actor rows compute actor identity during the actor pass:
  `src/collectors/network-viewer.plugin/network-viewer.c:2511`.
- Link rows recompute actor identity later during the link pass:
  `src/collectors/network-viewer.plugin/network-viewer.c:2627`.
- Ownership links are skipped when the recomputed actor id cannot be found:
  `src/collectors/network-viewer.plugin/network-viewer.c:2743`.
- Socket links are skipped when the recomputed actor id cannot be found:
  `src/collectors/network-viewer.plugin/network-viewer.c:2757`.

Finding:

- The render path is not snapshot-consistent. If APPS_LOOKUP cache state changes
  between actor creation and link creation, the same PID can map to a different
  actor id. That explains actors without ownership/socket links.

Repair direction:

- Build a Function-local immutable PID enrichment snapshot before actor and link
  emission.
- Use that snapshot for actor id, actor row, ownership link, socket link,
  modal labels, and evidence rows during the entire render.

### Bug 4 - per-PID enrichment is not presented as actor details

Evidence:

- Actor table columns for enrichment are declared:
  `src/collectors/network-viewer.plugin/network-viewer.c:3014`.
- PID-mode actor row enrichment is populated through
  `topology_v1_enrich_process_actor_from_cache()`:
  `src/collectors/network-viewer.plugin/network-viewer.c:2545`.
- Actor modal identification only lists process/user/namespace/local IP/command
  for PID process actors; it does not list cgroup/container/orchestrator fields:
  `src/collectors/network-viewer.plugin/network-viewer.c:3252`.
- Actor labels only add display/type/process/container_name and basic PID-mode
  process labels:
  `src/collectors/network-viewer.plugin/network-viewer.c:2571`.

Finding:

- The data can exist in actor table columns, but the modal/labels path does not
  project it. The UI is not expected to guess important details from arbitrary
  actor columns.

Repair direction:

- In `group_by=pid`, add the enriched fields to `actor_labels` and modal
  identification/details according to the topology v1 presentation contract.

### Bug 5 - grouped views intentionally do not expose merged enrichment

Evidence:

- `group_by=process_name` currently populates only `process`:
  `src/collectors/network-viewer.plugin/network-viewer.c:2562`.
- `group_by=container` currently populates only `container_name`:
  `src/collectors/network-viewer.plugin/network-viewer.c:2565`.
- The developer guide states raw per-PID metadata is populated only in
  `group_by=pid`:
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md:1020`.

Finding:

- The current Agent behavior matches the rule that grouped actors must not show
  variable per-PID fields as scalar facts. It does not yet solve the separate
  product need of inspecting merged grouped attributes.
- User decision 1A resolves this by making PID mode the raw-detail drilldown
  instead of adding merged enrichment to grouped actors.

Repair direction:

- Do not emit variable grouped attributes as single scalar actor fields.
- Keep `group_by=process_name` and `group_by=container` actor details minimal.
- Make `group_by=pid` complete enough for inspection.

### Bug 6 - Cloud schema can merge attributes, but this SOW will not use it for grouped enrichment

Evidence:

- Cloud schema has a column `aggregation` field:
  `netdata/cloud-topology-service @ 3199152 internal/topology/schema/payload.go:460`.
- Unknown column object fields are rejected; the accepted field list includes
  `aggregation`:
  `netdata/cloud-topology-service @ 3199152 internal/topology/schema/column.go:23`.
- Actor merge identity uses `merge_identity`, falling back to `identity`:
  `netdata/cloud-topology-service @ 3199152 internal/topology/aggregate/aggregate.go:893`.
- Merged actor row conflicts without aggregation fail:
  `netdata/cloud-topology-service @ 3199152 internal/topology/aggregate/aggregate.go:2451`.
- `set` aggregation is supported:
  `netdata/cloud-topology-service @ 3199152 internal/topology/aggregate/aggregate.go:2470`.
- `actor_labels` set/dedupe merge requires actor/key/value:
  `netdata/cloud-topology-service @ 3199152 internal/topology/aggregate/aggregate.go:3685`.
- Set aggregation serializes values as arrays:
  `netdata/cloud-topology-service @ 3199152 internal/topology/aggregate/aggregate.go:3889`.

Finding:

- Cloud can merge attributes, but only if the Agent emits schema-valid columns
  or detail/label tables with explicit merge semantics. Cloud cannot recover
  attributes that the Agent dropped before emitting the local grouped payload.
- Per user decision 1A, grouped network-connections actors will not expose
  merged enrichment. Cloud merge support remains evidence that option 1B was
  possible, not selected.

Repair direction:

- Keep grouped actors clean and rely on `group_by=pid` for raw details.
- Do not add new grouped set-valued columns or actor-label tables in this SOW.

### Bug 7 - partial APPS_LOOKUP data must be cacheable but re-queried

Evidence:

- APPS_LOOKUP has separate outer PID status and inner cgroup status:
  `src/libnetdata/netipc/include/netipc/netipc_protocol.h:123` and
  `src/libnetdata/netipc/include/netipc/netipc_protocol.h:128`.
- APPS_LOOKUP item rows carry PID facts, `comm`, `cgroup_path`,
  `cgroup_name`, labels, and inner `cgroup_status`:
  `src/libnetdata/netipc/include/netipc/netipc_protocol.h:539`.
- apps.plugin already emits `NIPC_PID_LOOKUP_KNOWN` with
  `NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER` when the PID exists but cgroup data is
  not ready:
  `src/collectors/apps.plugin/apps-lookup-netipc.c:119` and
  `src/collectors/apps.plugin/apps-lookup-netipc.c:161`.
- network-viewer previously discarded inner `UNKNOWN_RETRY_LATER` items. The
  repaired implementation now caches them as incomplete and counts them for
  retry visibility:
  `src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c:580`.

Finding:

- No new netipc enum is needed. The protocol already represents the case:
  PID known, partial PID/process data available, cgroup/container data not ready,
  and the consumer should ask again later.
- The implementation bug was that network-viewer treated the whole item as
  uncacheable instead of caching partial PID facts with an incomplete flag.
  This is repaired in the current source.

Repair direction:

- Cache partial APPS_LOOKUP entries when outer status is `PID_KNOWN` and inner
  `cgroup_status` is `UNKNOWN_RETRY_LATER`.
- Mark those entries incomplete and enqueue them again whenever the PID remains
  in the current network-viewer working set.
- Upgrade the same entry in place when apps.plugin later returns
  `NIPC_APPS_CGROUP_KNOWN` or `NIPC_APPS_CGROUP_HOST_ROOT`.
- Evict on `UNKNOWN_PERMANENT`, PID starttime mismatch, bounded LRU pressure,
  PID disappearance, or plugin shutdown.

### Bug 8 - `cgroup-paths` control must stay removed

Evidence:

- Current Function info accepted params include `group_by`,
  `__topology_mode`, `mode`, `sockets`, `protocols`, `endpoints`, and `labels`;
  `cgroup-paths` is not emitted there:
  `src/collectors/network-viewer.plugin/network-viewer.c:1823`.
- Current metadata documents the grouping selector and no `cgroup-paths`
  parameter:
  `src/collectors/network-viewer.plugin/metadata.yaml:183`.
- Current README documents the grouping selector and no `cgroup-paths`
  parameter:
  `src/collectors/network-viewer.plugin/README.md:219`.
- Historical SOW references have been rewritten or retained only as explicit
  negative regression guards.

Finding:

- The active source has already removed the show/hide control. This SOW keeps
  that as a regression guard because the user explicitly rejected that product
  direction.

## Pre-Implementation Gate

Status: ready. User decision 1A is recorded, and the partial-data scenario is
covered by existing APPS_LOOKUP enums without a wire-format extension.

Problem / root-cause model:

- The core root cause is a contract drift between the master no-TTL/no-refresh
  cache decision and the child-SOW generation-flush/timer implementation.
- A second root cause is render-time cache non-determinism: the same live cache
  is consulted multiple times during one topology render.
- A third root cause is presentation incompleteness: enriched fields are emitted
  as actor columns but not selected into actor labels/modal sections.

Evidence reviewed:

- Listed in `## Analysis`.

Affected contracts and surfaces:

- `CGROUPS_LOOKUP` client cache in apps.plugin.
- `APPS_LOOKUP` client cache in network-viewer.plugin.
- `APPS_LOOKUP` server generation semantics in apps.plugin.
- `topology:network-connections` actor identity, actor rows, actor labels,
  modal presentation, graph links, metadata, tests, fixtures, docs, public
  query skills, and Cloud aggregation behavior.
- Durable specs:
  `.agents/sow/specs/topology-containers-ipc-contract.md` and
  `.agents/sow/specs/topology-function-schema.md`.

Existing patterns to reuse:

- apps.plugin already has concrete cleanup events: PID exit, cgroup path
  change, and PID starttime identity.
- network-viewer already builds per-Function context dictionaries for process
  actors and links; the per-render enrichment snapshot should live in that
  Function-local context, not as mutable global state.
- topology v1 modal presentation should use actor labels and declared modal
  fields rather than UI heuristics.
- APPS_LOOKUP already has an outer-known/inner-retry partial data state; reuse
  it instead of extending the wire enum.

Risk and blast radius:

- Medium to high. The work touches hot-path plugin caches, cross-plugin IPC,
  Function payload shape, and Cloud aggregation semantics.
- Removing whole-cache invalidation can retain stale entries if cleanup events
  are missed. Validation must explicitly test PID exit, PID reuse/starttime
  change, cgroup path change, `UNKNOWN_PERMANENT`, and bounded cache eviction.
- Changing actor labels/modal presentation is user-visible and must remain
  aligned with the topology schema.

Sensitive data handling plan:

- No raw local cgroup paths, pod names, container names, registry names, tokens,
  node IDs, or private endpoint values are written to durable artifacts.
- Fixtures use synthetic process/container/service names.
- Runtime captures, if needed, stay under `.local/` and are not committed.

Implementation plan:

1. Correct SOW/spec contracts for cache lifetime and generation semantics.
2. Remove normal-generation whole-cache reset from apps.plugin
   CGROUPS_LOOKUP client.
3. Remove timer probe and normal-generation whole-cache flush from
   network-viewer APPS_LOOKUP client.
4. Add explicit cleanup-driven eviction coverage in apps.plugin and
   network-viewer tests.
5. Cache partial APPS_LOOKUP responses and keep incomplete live-PID entries in
   the on-demand intake until they become complete or are evicted.
6. Add a per-render PID enrichment snapshot in network-viewer and use it for
   all actor/link/modal/evidence emission.
7. Add PID-mode enriched actor labels/modal details.
8. Validate direct Agent Function output and Cloud Function path.
9. Run install/restart validation and inspect journal/socket state.

Validation plan:

- Unit tests for apps.plugin cgroup cache retention across normal cgroup
  generation bumps and eviction on concrete cleanup events.
- Unit tests for network-viewer APPS_LOOKUP cache retention across normal apps
  collection generation bumps and no timer refresh.
- Topology fixtures for `group_by=pid`, `group_by=process_name`, and
  `group_by=container`.
- Regression fixture proving one render cannot create actors under one
  enrichment snapshot and links under another.
- Direct Agent Function calls for all groupings.
- Cloud Function call for all groupings; no grouped enrichment merge behavior is
  expected under decision 1A.
- Journal/socket checks after `./install.sh` restart.
- Same-failure search for `dictionary_flush`, timer refresh variables, and
  `cgroup-paths` active metadata.

Artifact impact plan:

- AGENTS.md: not expected to change.
- Runtime project skills: update `.agents/skills/project-create-topology/` if a
  new grouped-attribute pattern is selected.
- Specs: update `.agents/sow/specs/topology-containers-ipc-contract.md` and
  `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: update network-viewer Function docs/metadata if modal
  or grouped attributes change.
- End-user/operator skills: update query topology skills if output fields or
  examples change.
- SOW lifecycle: this consolidation SOW covers regressions across SOW-0032,
  SOW-0034, SOW-0035, and SOW-0036 because the bugs cross the original SOW
  boundaries.

Open-source reference evidence:

- Not needed for this first analysis. The bug is in local Netdata cache and
  topology contract conformance, with Cloud aggregation behavior checked in
  `netdata/cloud-topology-service @ 3199152`.

Open decisions:

- None.

## Implications And Decisions

1. Grouped enrichment presentation:
   - A. Grouped actors show no merged enrichment; users drill into PID mode.
   - B. Grouped actors show merged enrichment as set-valued labels/detail
     fields using schema-declared merge semantics.
   - C. Grouped actors show only a representative first/last value.
   - Decision: A, selected by the user on 2026-05-28.
   - Implication: `group_by=process_name` and `group_by=container` remain clean
     actor-level views. All raw variable details live in `group_by=pid`.

2. Partial APPS_LOOKUP data:
   - Existing enum support: yes. Outer `NIPC_PID_LOOKUP_KNOWN` plus inner
     `NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER` means PID/process data is known,
     cgroup/container data is not ready yet, and the consumer should retry while
     the PID remains in its working set.
   - Decision: no netipc wire enum change is needed for this case.

## Plan

1. Update specs/SOW contract text before code.
2. Implement cache coherence repair.
3. Implement render-snapshot and actor-detail repair.
4. Validate locally and through Cloud.

## Execution Log

### 2026-05-28

- Created the SOW and recorded the reported bugs plus code/spec/Cloud evidence.
- Recorded user decision 1A: grouped views expose no merged enrichment; PID mode
  is the raw-detail drilldown.
- Verified APPS_LOOKUP already supports partial PID-known/cgroup-retry data;
  no enum extension is needed for the scenario.
- Repaired master/child SOW and IPC spec text so the durable contract is
  demand-driven per PID, with no timer refresh and no whole-cache invalidation
  on normal producer generation changes.
- Repaired apps.plugin CGROUPS_LOOKUP cache cleanup: PID unlink now evicts the
  cgroup cache entry when the refcount reaches zero, and generation changes are
  recorded only for diagnostics.
- Repaired network-viewer APPS_LOOKUP client: removed the idle refresh/probe
  timer, removed generation-bump cache flush behavior, enqueued current
  working-set PIDs on each Function call, pruned entries outside that working
  set, cached `UNKNOWN_RETRY_LATER` partial entries, and retried them while
  current.
- Repaired topology rendering: container actor fields are snapshotted per PID
  for one render, `group_by=container` emits `container` actors, PID mode emits
  enrichment fields and actor modal labels, and grouped modes avoid raw
  per-PID variable enrichment.
- Updated network-viewer metadata, README, and integration docs to expose the
  `group_by` selector and not the removed cgroup-path visibility control.
- Ran `./install.sh`; it rebuilt, installed, and restarted the local Agent.

## Validation

- `git diff --check` passed before install and will be rerun before commit.
- `sudo cmake --build build --target network-viewer-apps-lookup-client-test
  apps-lookup-protocol-test network-viewer.plugin apps.plugin -j 8` passed.
- `sudo build/network-viewer-apps-lookup-client-test` passed.
- `sudo build/apps-lookup-protocol-test` passed.
- `./install.sh` completed and restarted `netdata`.
- Service validation: `systemctl status netdata --no-pager -l` reported
  `active (running)` after restart.
- Runtime sockets before Function demand: `/run/netdata` had one
  `cgroups-lookup` shm file and no `apps-lookup` shm file, matching no idle
  APPS_LOOKUP timer/probe client.
- Runtime sockets after Function demand: `/run/netdata` had one
  `apps-lookup` shm file and one `cgroups-lookup` shm file. `lsof -U` showed
  one live `apps-lookup` connection from `network-viewer.plugin` to
  `apps.plugin`, and one live `cgroups-lookup` connection from `apps.plugin` to
  `netdata`.
- Direct unauthenticated local Function call returned HTTP 412, so validation
  used the token-safe cached direct-agent bearer under `.local/` without
  printing or committing token bytes.
- Immediate authenticated direct Agent query for `group_by=["container"]`
  returned `status=200`, `data.view.scope="container"`,
  `data.stats.group_by="container"`, 29 container actors, 53 endpoint actors,
  1 self actor, zero local non-container actors, and zero unlinked actors.
- Repeated authenticated direct Agent query after 75 seconds returned 22
  container actors, 55 endpoint actors, 1 self actor, zero local non-container
  actors, and zero unlinked actors. This covers the old once-per-minute mixed
  actor refresh failure window.
- Authenticated direct Agent query for `group_by=["pid"]` returned all
  enrichment actor columns and modal label fields. The runtime sample included
  cgroup/container/orchestrator labels for 43 actors.
- Authenticated direct Agent Function info returned `status=200`,
  `type="topology"`, accepted param `group_by`, group options `process_name`,
  `pid`, and `container`, and no `cgroup-paths` accepted param.
- Authenticated direct Agent query for `group_by=["process_name"]` returned
  `data.stats.group_by="process_name"`, 32 process actors, 63 endpoint actors,
  1 self actor, zero process actor rows with PID, and zero process actor rows
  with container-name enrichment.
- Journal validation since restart showed lookup server startup and the
  apps.plugin CGROUPS_LOOKUP connection. The focused post-restart filter found
  no APPS_LOOKUP/CGROUPS_LOOKUP fatal/error/disconnect messages.
- Same-failure source/doc search found no active source, metadata, README,
  integration doc, or public topology skill reference to the removed
  generation-refresh variables/counters or cgroup-path visibility control.
- Cloud-proxied validation was not run because this checkout has no `.env`
  Cloud token. Direct-agent validation exercises the Agent Function output,
  which is the repaired surface in this SOW; Cloud grouped-attribute merging is
  intentionally out of scope under decision 1A.

Sensitive data gate:

- Current SOW text contains no raw secrets, bearer tokens, private endpoints,
  raw runtime cgroup paths, raw container names, raw pod names, or registry
  names. Runtime response bodies and bearer cache remain under `.local/` and
  are not committed.

## Outcome

Implementation and local runtime validation are complete. The SOW is closed
with the implementation commit so the lifecycle move and source changes land
together.

## Lessons Extracted

- Cache-generation fields are too easy to misuse as invalidation triggers.
  Future SOWs should explicitly say whether a generation is diagnostic or a
  cache-lifetime signal.
- Grouped topology actor views must define which fields survive aggregation
  before implementation. Otherwise enrichment work can accidentally leak
  variable per-PID fields into aggregated actor rows.

## Followup

- Cloud grouped-attribute merge support was out of scope for this regression
  repair and is superseded by SOW-0044's grouped enrichment consistency work.
- Cloud-proxied Function validation can be added when a `.env` Cloud token is
  available in the checkout; direct-agent validation already covers the Agent
  producer behavior.

## Regression Log

- This SOW is the regression repair for SOW-0035/SOW-0036 cache coherence and
  actor grouping behavior discovered during local runtime inspection.

## Correction - 2026-05-28

SOW-0044 supersedes this SOW's earlier decision that grouped topology views
should leave variable enrichment out of actor-visible metadata. The current
contract is:

- PID grouping emits scalar per-PID enrichment.
- Process-name and container grouping merge per-PID enrichment through
  deduplicated actor labels and schema-declared `set` aggregation metadata.
- Cloud/UI remain generic topology consumers; Cloud work, if needed, is generic
  schema/aggregation validation or repair, not container-specific logic.

The cache lifetime decision from this SOW remains current: no whole-cache
invalidation and no timer refresh; consumers re-query incomplete entries while
the PID remains in their current working set.
