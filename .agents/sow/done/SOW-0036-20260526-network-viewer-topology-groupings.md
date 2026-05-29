# SOW-0036 - Step 4: network-viewer topology groupings (user-visible)

## Status

Status: completed

Regression correction (2026-05-28): the earlier implementation exposed a `cgroup-paths:show|hide` control and treated container metadata as enrichment on process actors. That was the wrong product surface. This SOW is corrected to the actor-level selector model: `group_by=process_name`, `group_by=pid`, and `group_by=container`.

## Requirements

Purpose: elevate `topology:network-connections` from a process-only topology to a user-selectable actor-level topology.

The supported actor levels are:

- `group_by=process_name`: group by process name. This may aggregate multiple PIDs, including PIDs from different containers or nodes. Fields that can vary per PID must not be emitted as actor attributes in this grouped view.
- `group_by=pid`: one actor per PID. This is the only view that may emit all raw per-PID details, including PID, UID, command line, network namespace, cgroup path, and container/orchestrator metadata.
- `group_by=container`: group by canonical container/service actor name. For systemd services, the container actor name is the service name. For non-container and non-service processes, the container actor name falls back to the process name.

The removed product surface:

- There is no user-facing `cgroup-paths:show|hide` control.
- There is no separate cgroup-path visibility selector.
- Raw cgroup path remains a PID-mode detail only.

## Acceptance Criteria

- Function metadata exposes a clear `group_by` selector with `process_name`, `pid`, and `container` choices.
- `group_by=container` emits local actor rows of type `container`, not process actors with a container label bolted on.
- `group_by=container` keeps local container actors linked to `self` through ownership links and to endpoints through socket links.
- Repeated refreshes do not mix process actors into a container response.
- `group_by=pid` emits per-PID enrichment fields and modal labels for container/orchestrator/cgroup details when APPS_LOOKUP has them.
- Grouped views do not emit per-PID variable enrichment fields as actor attributes.
- Cloud/schema grouped-attribute merging is not implemented in this SOW; grouped raw enrichment remains out of scope by user decision 1A in SOW-0043.

## Analysis

Root cause of the previous confusion:

- The old implementation added enrichment controls instead of a topology actor-level selector.
- The hide/show cgroup-path control was an implementation/privacy detail, not a topology grouping decision.
- The product goal is actor selection: processes vs PIDs vs containers.
- The cache bug from SOW-0043 could periodically make `group_by=container` degrade into mixed container/process output because partial APPS_LOOKUP updates were discarded and later refreshes could rebuild actors from process fallback state.

Evidence in current implementation:

- `group_by` parsing accepts `process_name`, `pid`, and `container`: `src/collectors/network-viewer.plugin/network-viewer.c:330`, `src/collectors/network-viewer.plugin/network-viewer.c:336`, and `src/collectors/network-viewer.plugin/network-viewer.c:341`.
- Function metadata advertises `group_by`: `src/collectors/network-viewer.plugin/network-viewer.c:1823` and `src/collectors/network-viewer.plugin/network-viewer.c:1836`.
- Container actor identity uses canonical container name and falls back to process name: `src/collectors/network-viewer.plugin/network-viewer.c:932`, `src/collectors/network-viewer.plugin/network-viewer.c:963`, and `src/collectors/network-viewer.plugin/network-viewer.c:986`.
- Container-mode actor rows are emitted as actor type `container`: `src/collectors/network-viewer.plugin/network-viewer.c:2572`, `src/collectors/network-viewer.plugin/network-viewer.c:2590`, and `src/collectors/network-viewer.plugin/network-viewer.c:2625`.
- PID-mode modal/search enrichment fields are present: `src/collectors/network-viewer.plugin/network-viewer.c:2393`, `src/collectors/network-viewer.plugin/network-viewer.c:3311`, and `src/collectors/network-viewer.plugin/network-viewer.c:3390`.
- Metadata/docs describe `group_by` and not the removed cgroup-path control: `src/collectors/network-viewer.plugin/metadata.yaml:183`, `src/collectors/network-viewer.plugin/README.md:219`, and `src/collectors/network-viewer.plugin/integrations/network_connections.md:219`.

## Pre-Implementation Gate

Status: completed after regression repair.

Problem / root-cause model: the visible Function control must be actor grouping, not cgroup-path display suppression. Container grouping depends on coherent APPS_LOOKUP cache updates from SOW-0043.

Evidence reviewed: current source citations above, SOW-0043 runtime validation, metadata/docs, installed Agent output.

Affected contracts and surfaces: topology Function parameters, topology actor rows, actor type metadata, actor labels/modal metadata, network-viewer README/integration metadata, topology schema/spec text, and public topology query skills.

Existing patterns to reuse: netdata.topology.v1 `view.group_by`, actor table columns, actor label table, ownership links from `self` to local actors, and endpoint socket links.

Risk and blast radius: grouped views intentionally suppress variable per-PID fields. Users needing raw details must select `group_by=pid`. Cloud grouped-attribute merge behavior remains a separate Cloud-side SOW.

Sensitive data handling: raw cgroup paths and detailed container metadata are exposed only in PID mode; free-form labels are denied by default. Runtime validation artifacts live under `.local/` and are not committed.

Implementation plan: remove the cgroup-path display control, expose the `group_by` selector, implement `container` actors, keep PID enrichment labels, and validate installed Agent behavior.

Validation plan: build tests, install/restart, direct authenticated Function calls for all three `group_by` values, repeated `container` query after the old one-minute failure window, socket/client count check, and journal check.

Artifact impact plan: source, tests, metadata, README/integration docs, topology specs, public query skills, and SOW corrections updated.

Open decisions: none. User decision 1A in SOW-0043 fixes grouped-attribute behavior for this SOW.

## Implications And Decisions

- The UI/API selector is `group_by`, not a cgroup-path display toggle.
- `group_by=pid` is the raw/detail view.
- `group_by=process_name` and `group_by=container` are aggregated views and must not expose variable per-PID raw fields as actor attributes.
- Container actors use service name for systemd services and process-name fallback for host processes without container/service identity.
- Cloud-side merging of raw enrichment across grouped actors is out of scope until a separate Cloud/schema SOW defines it.

## Plan

- Keep `process_name`, `pid`, and `container` as the only visible topology actor grouping choices in this SOW.
- Remove the stale cgroup-path display control from durable docs/specs/SOW text.
- Keep PID actor enrichment fields and modal labels.
- Keep container grouped actor rows minimal and stable.
- Validate against the installed Agent, not only unit tests.

## Execution Log

- Implemented `group_by` parser and Function metadata. Evidence: `src/collectors/network-viewer.plugin/network-viewer.c:330`, `src/collectors/network-viewer.plugin/network-viewer.c:341`, and `src/collectors/network-viewer.plugin/network-viewer.c:1823`.
- Implemented container actor naming and snapshot consistency per render. Evidence: `src/collectors/network-viewer.plugin/network-viewer.c:995`, `src/collectors/network-viewer.plugin/network-viewer.c:1736`, and `src/collectors/network-viewer.plugin/network-viewer.c:2564`.
- Implemented PID actor enrichment labels/modal fields. Evidence: `src/collectors/network-viewer.plugin/network-viewer.c:2393` and `src/collectors/network-viewer.plugin/network-viewer.c:3311`.
- Updated docs/metadata for the actor-level selector. Evidence: `src/collectors/network-viewer.plugin/metadata.yaml:183`, `src/collectors/network-viewer.plugin/README.md:191`, and `src/collectors/network-viewer.plugin/integrations/network_connections.md:191`.
- Runtime repair for mixed container/process updates is implemented in SOW-0043 by changing APPS_LOOKUP cache refresh to current-working-set demand and partial retry.

## Validation

- Build/test: `sudo cmake --build build --target network-viewer-apps-lookup-client-test apps-lookup-protocol-test network-viewer.plugin apps.plugin -j 8` passed.
- Test run: `sudo build/network-viewer-apps-lookup-client-test` passed.
- Test run: `sudo build/apps-lookup-protocol-test` passed.
- Install/runtime: `./install.sh` completed and restarted `netdata`.
- Immediate authenticated direct Agent query for `group_by=["container"]` returned `status=200`, `data.view.scope="container"`, `data.stats.group_by="container"`, 29 container actors, 53 endpoint actors, 1 self actor, zero local non-container actors, and zero unlinked actors.
- Repeated authenticated direct Agent query after 75 seconds returned 22 container actors, 55 endpoint actors, 1 self actor, zero local non-container actors, and zero unlinked actors. This covers the old once-per-minute mixed-output failure window.
- Authenticated direct Agent Function info returned `status=200`, `type="topology"`, accepted param `group_by`, group options `process_name`, `pid`, and `container`, and no `cgroup-paths` accepted param.
- Authenticated direct Agent query for `group_by=["process_name"]` returned `data.stats.group_by="process_name"`, 32 process actors, 63 endpoint actors, 1 self actor, zero process actor rows with PID, and zero process actor rows with container-name enrichment.
- Authenticated direct Agent query for `group_by=["pid"]` returned the enrichment columns and actor modal label definitions for cgroup/container/orchestrator fields. The `actor_labels` table contained cgroup/container/orchestrator labels for 43 actors in this runtime sample.
- `/run/netdata` contained one `apps-lookup` shm file after Function use and one `cgroups-lookup` shm file, matching one live client per lookup service.
- `lsof -U` showed one live `apps-lookup` connection from `network-viewer.plugin` to `apps.plugin` and one live `cgroups-lookup` connection from `apps.plugin` to `netdata`.
- Journal check since restart showed the lookup servers and apps/cgroups connection startup without post-restart lookup fatal/error/disconnect messages matching the APPS_LOOKUP/CGROUPS_LOOKUP filters.

## Outcome

SOW-0036 is complete after regression repair. `topology:network-connections` now exposes actor-level grouping through `group_by=process_name|pid|container`; PID mode carries raw enrichment; grouped modes remain stable and do not mix process actors into container responses.

## Correction - 2026-05-28

SOW-0044 supersedes this SOW's earlier grouped-view enrichment restriction.
The current product contract is:

- `group_by=pid` emits scalar per-PID process, cgroup, container, and service
  enrichment.
- `group_by=process_name` and `group_by=container` preserve variable per-PID
  enrichment through deduplicated actor labels and schema-declared `set`
  aggregation metadata.
- grouped views must not silently drop cgroup/container/service enrichment and
  must not replace variable facts with an arbitrary PID's scalar values.

This correction is recorded here so future readers do not treat the original
PID-only enrichment wording as current design.
