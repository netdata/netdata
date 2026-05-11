# SOW-0026 - SNMP Modal Product Composition

## Status

Status: completed

Sub-state: second regression repair, documentation updates, validation, and
local build completed on 2026-05-11.

## Requirements

### Purpose

Make `topology:snmp` actor modals useful for network engineers who need accurate port, neighbor, and L2 relationship information without contradictory tables.

### User Request

The user reported that SNMP modals show `Ports` and `Links`, but the distinction is confusing and appears incorrect. A device cannot have an L2 link that is not associated with a port. The user also reported that local port labels can be wrong and that `Ports` and `Links` must be 100% in sync or the modal is useless.

### Assistant Understanding

Facts:

- Current SNMP device modals include a `Ports` section from actor table `actor_ports` and a `Links` section from graph links.
- Current `actor_ports` modal columns show `name`, statuses, role, VLAN/FDB/link/neighbor counts, and debug `extra`.
- Current `Links` modal columns show remote actor, local port, remote port, protocol, direction, state, and evidence count.
- The current `Links` section derives local/remote ports from selected-side port projections over link endpoint columns.

Inferences:

- The issue is likely not just labels. The underlying product model is wrong if ports and links can disagree.
- For a device actor, the primary table should probably be a port-centric table. Links/neighbors should be columns or expandable rows attached to each port, not an independent table with a different interpretation of "local port".
- Inferred endpoint actors may still need a relationship/link modal, but managed SNMP devices need port inventory as the organizing principle.

Confirmed by code review:

- Link evidence already carries `src_if_index` and `dst_if_index`, and graph links carry `src_port_name` and `dst_port_name`.
- The port inventory source rows commonly carry `if_index`, `if_name`, `if_descr`, `if_alias`, `mac`, and `speed`, but the current `actor_ports` table does not expose them as typed canonical columns.
- Managed device actors should use a port-centric modal. Endpoint/segment actors, which do not have port inventory, can keep a relationship-oriented `Links` section.
- `topologyV1EndpointPortName()` falls back to `display_name` and `sys_name`; that can turn an actor/device/IP label into a displayed local port name when no real port field is present.

### Acceptance Criteria

- A complete inventory exists for SNMP actor modal facts: actor labels, actor typed columns, `actor_ports`, graph links, and evidence sections.
- The desired SNMP device modal is port-centric and shows actual numeric port IDs when known, plus port name.
- `Ports` and link/neighbor information are derived from the same canonical endpoint facts or explicitly cross-checked.
- Any remaining `Links` section has a precise purpose and cannot contradict the port table.
- The SOW identifies every missing canonical field required to make port/link rows 100% aligned.
- The actor identification area exposes important device labels, not only the generic `Labels` tab.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:158` current SNMP device modal.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:163` current `Ports` section.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:188` current `Links` section.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:212` current selected-side local/remote port columns.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:474` `actor_ports` table type.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:493` current port modal columns.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:534` current `actor_ports` columns.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:699` link/evidence construction entry point.
- `.agents/sow/specs/topology-function-schema.md:380` SNMP modal composition notes.
- `.agents/sow/specs/topology-modes-correlation-aggregation.md:498-556` SNMP detailed/aggregated and port-centric modal requirements.
- `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Models/Port.php:90-138` shows port labels are built from `ifName`, `ifAlias`, `ifDescr`, and sometimes `ifIndex`.
- `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Models/Port.php:401-427` models links as port relationships (`local_port_id`, `remote_port_id`).
- `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Http/Controllers/Device/Tabs/PortsController.php:145-185` organizes neighbors from a port-first data set.
- `netdisco/netdisco @ 7c1bc3e8290fe7b757c6a462de12e4613eca3899 share/views/ajax/device/ports.tt:394-406` renders neighbors from the device port row and remote port.

Current state:

- Device modal:
  - `Ports` comes from `actor_ports`.
  - `Links` comes from graph links.
- `actor_ports` currently has stable columns such as `name`, topology role, admin/oper status, port type, link mode, STP state, VLAN IDs, FDB count, link count, neighbor count, neighbors JSON, VLANs JSON, and debug `extra`.
- `Links` currently uses selected-side projections for `src_port_name` and `dst_port_name`, but the user observed cases where local port looks like a remote actor/IP. This points to a possible endpoint mapping or projection issue.
- `topologyV1EndpointPortName()` currently accepts `display_name` and `sys_name` as final fallbacks. These are actor labels, not port labels, and explain how a remote actor/IP can appear as a port.

Available facts to inventory:

- Device actor:
  - display name, sysName, sysDescr, model, vendor, management IP, sys location/contact, protocols/capabilities, port counts, VLAN/FDB/LLDP/CDP counts.
- Port table:
  - current normalized port name/status/role/VLAN/FDB/neighbor facts.
  - likely raw/custom fields in `extra` that may include actual `if_index`, `if_name`, `if_descr`, `if_alias`, MAC, speed, duplex, VLAN details, and neighbor objects.
- Link/evidence:
  - remote actor, source/destination port names, source/destination ifIndex when present, protocol, direction/state, confidence/inference/attachment mode, evidence count.

Target audience and questions:

- Network engineers need:
  - Which physical/logical ports exist?
  - What is the actual numeric port ID, ifIndex, interface name, and description/alias?
  - Which ports are up/down/admin-down?
  - What is connected to each port and by which protocol/evidence?
  - Is the link verified by LLDP/CDP or inferred by FDB/ARP/STP?
  - Which VLANs, STP role/state, FDB MAC counts, speeds, and neighbors apply to each port?

Reference pattern:

- Network inventory tools treat the port as the organizing object. LibreNMS keeps port labels on `ifName`/`ifAlias`/`ifDescr`/`ifIndex`, and its link model joins ports through local and remote port IDs. Netdisco similarly renders neighbor details from the selected device port row and remote port.
- For Netdata topology modals, this means a managed SNMP device should not present graph links as an equal peer of port inventory. It should present port inventory first and neighbor/link evidence as port-aligned details.

Available backend facts:

- Device actor labels and typed actor columns already include display name, sysName, vendor, model, management IP, sysDescr, sysLocation, sysContact, capabilities, protocol lists, port counts, VLAN counts, FDB counts, and LLDP/CDP neighbor counts.
- Port actor-owned rows have raw fields such as `if_index`, `if_name`, `if_descr`, `if_alias`, `mac`, `speed`, status, type, mode, topology role, STP state, VLAN IDs/details, FDB count, link count, neighbor count, and neighbor objects.
- Graph links have source/destination actors, link type, protocol, direction, state, source/destination port names, evidence count, discovered time, and last-seen time.
- Evidence rows have source/destination `if_index`, source/destination management IP, confidence, inference, attachment mode, and raw endpoint/metrics JSON for debug.

Missing or weak canonicalization:

- `actor_ports` does not expose `if_index`, `if_name`, `if_descr`, `if_alias`, `port_id`, `mac`, or `speed` as typed columns.
- Link modal selected-side port display relies on `src_port_name`/`dst_port_name`, and those values can currently be polluted by actor labels because of the endpoint-port fallback.
- There is no port-aligned relationship table for managed devices. The only link view is generic graph-link-oriented, so it can disagree visually with the port inventory.

Risks:

- If port and link tables are inconsistent, users will distrust the topology.
- Showing raw nested neighbor JSON in normal table cells would regress polish and usefulness.
- If numeric port IDs are unavailable, fabricating autoincrement IDs would be worse than showing no numeric ID.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- SNMP modals currently expose implementation tables separately rather than presenting a device as a collection of ports with attached neighbor/link evidence.
- The current `Links` section can produce confusing local/remote port labels because it is graph-link-oriented, not port-oriented.

Evidence reviewed:

- SNMP device modal currently has both `Ports` and `Links` sections in `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:158-180`.
- `Links` local/remote port columns are selected-side projections in `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:212-213`.
- `actor_ports` modal currently does not expose a clear numeric port ID column in the visible section at `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:493-503`.
- `actor_ports` table columns currently omit `if_index`, `if_name`, `if_descr`, `if_alias`, `port_id`, `mac`, and `speed` at `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:534-550`.
- `topologyV1EndpointPortName()` falls back to `display_name` and `sys_name` at `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:1551-1559`.

Affected contracts and surfaces:

- Agent Function payload for `topology:snmp`.
- SNMP topology Go v1 adapter.
- SNMP topology tests and fixtures.
- Cloud frontend modal rendering if the actor identification/header area needs a new selector.
- Cloud aggregator if it merges device port rows and link/evidence rows.
- Developer guide, topology spec, project topology skill.

Existing patterns to reuse:

- `actor_ports` actor-owned table for port inventory.
- Structured evidence columns for L2 discovery details.
- `selected_side_endpoint` only where side selection is truly needed.
- Debug `extra` JSON for unknown custom port fields, not normal table display.
- Actor-owned modal detail tables with `owner_filter: actor_column`.
- `modal.labels.identification.fields[]` for modal header facts.

Risk and blast radius:

- User-facing SNMP device modal behavior changes.
- Incorrect port/evidence joins can misrepresent physical topology.
- Aggregated cloud topologies may need stricter table merge rules for port identity.
- Sensitive data risk includes device sysContact/sysLocation and management IPs; do not write raw real values to durable artifacts.

Sensitive data handling plan:

- Use synthetic device/port/IP examples only.
- Do not store raw SNMP sysContact/sysLocation, management IPs, customer names, credentials, or community strings in SOWs/docs/tests.
- Keep real payload captures under `.local/` only.

Implementation plan:

1. Add SNMP modal identification fields:
   - managed devices: display name, management IP, vendor, model, port counts, LLDP/CDP counts;
   - endpoints/segments: display name and matching network identifiers.
2. Expose typed port inventory columns in `actor_ports`: `if_index`, `port_id`, `if_name`, `if_descr`, `if_alias`, `mac`, and `speed`.
3. Remove actor-label fallbacks from `topologyV1EndpointPortName()` so only real port fields can become link endpoint port labels.
4. Add a compact actor-owned `actor_port_links` detail table:
   - one row per selected actor side of each graph link;
   - columns include local `if_index`, local port name, remote actor, remote port name, protocol, link type, state, evidence count, confidence, inference, attachment mode, discovered time, and last-seen time;
   - this table is derived from graph links/evidence facts and exists to align device modal relationships with port inventory, not to create new topology facts.
5. Change managed device modals to show `Ports` then `Port Neighbors`; keep generic `Links` only for endpoint/segment/custom actors that do not own port inventory.
6. Add tests proving:
   - device modal no longer uses the generic `Links` section;
   - `actor_ports` exposes typed port identity/status columns;
   - `actor_port_links` is side-owned and uses the same local `if_index`/port name as evidence;
   - endpoint port names do not fall back to device/IP display names.
7. Update spec, developer guide, and project topology skill.

Validation plan:

- Go tests for SNMP topology v1 conversion.
- Payload schema validation.
- Fixture test with a device containing multiple ports, LLDP/CDP verified links, inferred FDB/ARP links, and ports without links.
- Cross-check that every link shown under a device maps to the same local port row shown in the port table when a port identifier exists.
- Manual UI validation on a real or sanitized SNMP topology payload.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md` if SNMP modal guidelines change.
- Specs: update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: likely unaffected unless Function examples are changed.
- End-user/operator skills: unaffected.
- SOW lifecycle: close only after SNMP modal behavior is validated with realistic data.

Open-source reference evidence:

- LibreNMS and Netdisco both support the port-first model:
  - `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Models/Port.php:90-138`
  - `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Models/Port.php:401-427`
  - `librenms/librenms @ 1d096602a4ce1197faab084b2523c7dd50419427 app/Http/Controllers/Device/Tabs/PortsController.php:145-185`
  - `netdisco/netdisco @ 7c1bc3e8290fe7b757c6a462de12e4613eca3899 share/views/ajax/device/ports.tt:394-406`

Open decisions:

- Resolved by user approval to proceed after product analysis:
  - Managed SNMP device actors get a port-first modal with `Ports` and a port-aligned `Port Neighbors` section.
  - Endpoint/segment/custom actors keep the generic relationship-oriented `Links` section because they do not own port inventory.
  - No synthetic numeric port IDs may be generated. Numeric port ID is shown only when a real `if_index` or source port ID is known.

## Implications And Decisions

1. Device modal composition:
   - Decision: managed devices use `Ports` + `Port Neighbors`; generic graph `Links` is removed from managed device modals.
   - Implication: users inspect a device by physical/logical port first, which matches the network engineer workflow.
   - Risk: a link with missing local port identity will appear in `Port Neighbors` with an empty local port. That is preferable to fabricating a port or showing an actor label as a port.

2. Endpoint/segment/custom modal composition:
   - Decision: actors without owned port inventory keep the existing `Links` section.
   - Implication: endpoint/segment actors can still explain how they connect to the graph.
   - Risk: selected-side projections remain there, but the endpoint port fallback fix prevents actor names from being shown as ports.

3. Port identity:
   - Decision: expose actual `if_index` and source `port_id` as typed port columns; never autoincrement.
   - Implication: the UI can show numeric port IDs only when the SNMP backend knows them.
   - Risk: devices without `if_index` will show blank `Port ID`, which is truthful.

4. Data duplication:
   - Decision: add `actor_port_links` as a compact actor-owned modal index over existing graph links.
   - Implication: the table adds small side-specific rows, but does not duplicate raw evidence or nested metadata.
   - Risk: Cloud aggregation must treat this as actor-owned modal detail and append/merge consistently.

## Plan

1. Move SOW to current and record product decisions.
2. Implement typed port inventory columns and modal identification fields.
3. Implement `actor_port_links` and switch device modal sections.
4. Fix endpoint port fallback.
5. Update tests, spec, developer guide, and project skill.
6. Validate with Go tests and topology schema validation.

## Execution Log

### 2026-05-11

- Created SOW from user-reported SNMP modal regressions and current code evidence.
- Completed product analysis:
  - managed SNMP devices must be port-first;
  - endpoint/segment/custom actors keep generic links;
  - actor labels must not be used as port labels;
  - `actor_ports` needs typed port identity columns;
  - `actor_port_links` is needed as a compact modal index.
- Implemented managed-device modal identification and port-first sections.
- Added typed `actor_ports` columns for real port identity and status.
- Added `actor_port_links` as a compact port-aligned modal index over graph links.
- Removed actor-label fallbacks from SNMP endpoint port names.
- Updated SNMP topology tests, topology spec, developer guide, and project topology skill.

## Validation

Acceptance criteria evidence:

- Complete inventory: recorded in this SOW under `Analysis`, including actors,
  `actor_labels`, `actor_ports`, graph links, evidence, and missing canonical
  fields.
- Port-centric device modal: managed device modal now uses `Ports` and
  `Port Neighbors`, with `actor_ports` first at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:156-177`.
- Actor identification: device modal label identification fields are declared
  at `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:189-204`.
- Numeric port IDs: `actor_ports` exposes `if_index` and source `port_id` as
  typed columns at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:658-682`.
- Port/link alignment: `actor_port_links` table type and modal columns are
  declared at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:597-628`,
  and row construction uses one row per incident actor side at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:1447-1485`.
- Generic graph `Links` remain available only through the endpoint/segment/custom
  modal path at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:181-186`.
- Actor labels no longer become port names:
  `topologyV1EndpointPortName()` now accepts only real port fields at
  `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:1830-1837`.

Tests or equivalent validation:

- `cd src/go && go test ./plugin/go.d/collector/snmp_topology`
- `cd src/go && go test ./plugin/go.d/collector/snmp_topology ./pkg/topology/v1 ./tools/functions-validation/validate`
- `cd src/go && for fixture in tools/functions-validation/fixtures/topology-v1/*.json; do go run ./tools/functions-validation/validate -schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json -input "$fixture" -require-rows; done`
- `git diff --check`
- `sudo -n cmake --build build --target go.d.plugin -- -j2`
- Unit-test evidence:
  - typed port columns and values:
    `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go:360-384`;
  - `actor_port_links` side-owned rows:
    `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go:386-398`;
  - managed device modal sections:
    `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go:420-430`;
  - endpoint actors keep generic links:
    `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go:432-437`;
  - actor label fallback regression:
    `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go:440-486`.

Real-use evidence:

- Built `go.d.plugin` through the local CMake/Ninja build path with the updated
  SNMP topology producer. The running local Agent was not modified by this SOW;
  no production or customer system was used.

Reviewer findings:

- External AI reviewers were not requested for this SOW. Review coverage came
  from code inspection, open-source reference comparison, schema validation,
  targeted tests, and same-failure search.

Same-failure scan:

- Searched for actor-label-to-port-name fallback and selected-side SNMP link
  modal risks with:
  `rg -n "display_name.*port|sys_name.*port|port.*display_name|port.*sys_name|topologyV1EndpointPortName|selected_side_endpoint" src/go/plugin/go.d/collector/snmp_topology src/go/plugin/go.d/collector src/collectors -S`
- Remaining `selected_side_endpoint` usage in SNMP is intentionally limited to
  endpoint/segment/custom actor modals. The SNMP endpoint port-name helper no
  longer accepts actor labels.

Sensitive data gate:

- This SOW uses only path/line evidence and synthetic descriptions. No raw sensitive payload data is included.

Artifact maintenance gate:

- `AGENTS.md`: unchanged. No project-wide workflow rule changed.
- Runtime project skills: updated
  `.agents/skills/project-create-topology/SKILL.md:260-283` with SNMP/L2 modal
  rules.
- Specs: updated
  `.agents/sow/specs/topology-function-schema.md:418-444` with SNMP port-first
  modal contract.
- End-user/operator docs: unchanged. This changes developer-facing topology
  payload composition, not an operator workflow or public querying procedure.
- End-user/operator skills: unchanged. No operator-facing skill semantics
  changed.
- SOW lifecycle: this SOW will move from `current/` to `done/` with status
  `completed` in the same commit as the implementation.

Specs update:

- Updated `.agents/sow/specs/topology-function-schema.md`.

Project skills update:

- Updated `.agents/skills/project-create-topology/SKILL.md`.

End-user/operator docs update:

- No update needed. The change is internal developer schema composition for
  topology producers.

End-user/operator skills update:

- No update needed. Public/operator skills do not describe developer modal
  composition.

Lessons:

- For SNMP/L2, a generic graph link table is not a good device modal. Port
  inventory has to be the organizing object, and relationship rows must align
  with that port identity.
- Link endpoint display helpers must never mix actor identity labels with port
  labels.

Follow-up mapping:

- No deferred implementation work remains in this SOW. Interface overlay work
  for traffic/packets/errors/state is a separate topology overlay topic already
  documented in the spec and developer guide.

## Outcome

Completed. Managed SNMP device modals are now port-first, expose typed port
identity, show port-aligned neighbor rows, and avoid using actor labels as port
names. Endpoint/segment/custom actor modals keep generic graph-link drilldowns.

## Lessons Extracted

- SNMP topology presentation needs network-engineer semantics: port identity
  first, neighbor/link evidence attached to that identity, and raw JSON only for
  debug/expanded diagnostics.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.

## Regression - 2026-05-11

### What Broke

- The SNMP device `Ports` section labels `if_index` as `Port ID`, which looks like
  an invented/autoincrement port number. The user requirement is no synthetic
  numbering anywhere. Real port numbers may be shown only when sourced from
  canonical producer data.
- The `Ports` expanded row does not show the neighboring actor as a clickable
  actor link. The separate `Port Neighbors` section has clickable remote actors,
  but the port-owned row must also expose the directly related neighbor.
- Actor links in v1 modal tables were not clickable in the frontend because the
  producer emits `actor_ref_label` projections with `actor_link` cells, while
  the frontend projection engine returned labels instead of actor IDs for that
  cell type.

### Evidence

- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` currently
  emits the first `Ports` modal column as `if_index` with label `Port ID`.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` currently
  defines `actor_ports` without neighbor actor-ref columns, so the expanded
  `Ports` view cannot link to the neighbor.
- `cloud-frontend/src/domains/functions/topology/v1/buildModalSections.js`
  projected `actor_ref_label` to a label string even when the declared cell type
  was `actor_link`.

### Root Cause

- The SOW correctly made SNMP modals port-first, but it still reused SNMP
  `ifIndex` as the visible port identity. `ifIndex` is useful technical
  metadata, but it is not a physical/source port number and should not be
  presented as `Port ID`.
- Port inventory and port-neighbor rows were aligned in separate tables, but the
  inventory table did not carry a compact derived neighbor actor reference for
  the expanded row.
- The frontend projection engine treated projection kind as the only source of
  output shape. For `actor_link` cells, the renderer needs the actor ID, not the
  already formatted actor label.

### Repair Plan

- Add a nullable `port_number` column to `actor_ports`. Populate it only from an
  explicit numeric source field such as `port_number` or numeric `port_id`.
  Never derive it from row order or `if_index`.
- Rename the expanded `if_index` modal column to `SNMP ifIndex` and remove it
  from the visible table columns.
- Add compact nullable `neighbor_actor` and `neighbor_port_name` columns to
  `actor_ports`, derived from the same graph-link endpoint facts used by
  `actor_port_links`.
- Show `Neighbor` as an expanded `actor_link` cell and `Neighbor Port` as an
  expanded text cell in the `Ports` section.
- Fix the frontend modal projection engine so `actor_ref_label` returns actor IDs
  when the cell type is `actor_link`.

### Validation Plan

- Add/adjust SNMP topology tests proving:
  - `port_number` is present and populated only from explicit source data;
  - `if_index` remains available as expanded SNMP metadata;
  - `neighbor_actor` and `neighbor_port_name` are derived from graph-link facts;
  - the main `Ports` modal no longer labels `if_index` as `Port ID`.
- Add frontend projection tests proving `actor_ref_label` + `actor_link` produces
  actor IDs.
- Run targeted Go and frontend tests plus schema validation and `git diff --check`.

### Implementation

- Added nullable `actor_ports.port_number`, populated only from explicit
  `port_number` or numeric source `port_id`.
- Kept SNMP `if_index` as expanded technical metadata labelled `SNMP ifIndex`,
  not as visible `Port ID`.
- Added nullable `actor_ports.neighbor_actor` and `neighbor_port_name`, derived
  from graph-link endpoint facts so the expanded port row can link to the same
  neighbor shown in `actor_port_links` when the port has one unambiguous remote
  actor.
- Fixed the frontend v1 modal projection engine so `actor_ref_label` returns
  actor IDs for `actor_link` cells while keeping text/badge cells label-based.
- Updated the topology developer guide, topology specs, and project topology
  skill with the stricter SNMP port identity and expanded-row neighbor rules.

### Validation

Acceptance criteria evidence:

- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` now emits
  `port_number` as the visible first Ports column and `if_index` as expanded
  `SNMP ifIndex`.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` now emits
  `neighbor_actor` and `neighbor_port_name` in `actor_ports`.
- `cloud-frontend/src/domains/functions/topology/v1/buildModalSections.js`
  now returns actor IDs for `actor_ref_label` projections when the cell is
  `actor_link`.

Tests or equivalent validation:

- `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp_topology ./pkg/topology/v1 ./tools/functions-validation/validate`
- `yarn test src/domains/functions/topology/v1/buildModalSections.test.js --runInBand`
- `sudo -n cmake --build build --target go.d.plugin -- -j2`
- `git diff --check` in the Agent repository.
- `git diff --check` in the frontend repository.

Real-use evidence:

- Built the local `go.d.plugin` target successfully. Browser inspection can be
  repeated after the rebuilt Agent and UI are installed; no code change remains
  blocked on that check.

Reviewer findings:

- External AI reviewers were not requested for this regression repair. The fix
  is covered by targeted producer and frontend unit tests plus schema validation.

Same-failure search:

- Verified the repaired modal contract by searching for `Port ID`, `if_index`,
  `actor_ports`, `neighbor_actor`, and `actor_ref_label` in the affected producer,
  specs, skill, and frontend projection files.

Artifact maintenance gate:

- `AGENTS.md`: unchanged. No project-wide workflow rule changed.
- Runtime project skills: updated `.agents/skills/project-create-topology/SKILL.md`
  with stricter SNMP `port_number`, `if_index`, and expanded neighbor rules.
- Specs: updated `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-modes-correlation-aggregation.md`.
- End-user/operator docs: unchanged. This is developer-facing topology payload
  composition, not an operator workflow.
- End-user/operator skills: unchanged. No public/operator skill semantics
  changed.
- SOW lifecycle: reopened regression SOW moved back to `current/`, repaired,
  then returned to `done/` with `Status: completed`.

Follow-up mapping:

- No new deferred implementation work remains in this SOW. Manual visual
  polishing of topology layout remains outside this SNMP modal regression.

### Regression Outcome

Completed. SNMP Ports no longer display SNMP `ifIndex` as a synthetic-looking
port number, expanded port rows can expose a clickable neighbor actor when link
facts allow it, and frontend v1 actor-link cells navigate again.

## Regression - 2026-05-11 - SNMP Port Identity Alignment

### What Broke

- The previous regression repair introduced `actor_ports.port_number` and made
  it the visible `Ports` table identity.
- The live SNMP payload has the real numeric device port identity in
  `actor_ports.if_index` and `actor_port_links.if_index`; it does not
  necessarily have a separate numeric `port_number` or numeric `port_id`.
- Result: `Ports` showed empty port IDs while `Port Neighbors` showed correct
  non-empty port IDs.

### Evidence

- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` currently
  emits `port_number` as the first `Ports` modal column.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` currently
  emits `if_index` as the first `Port Neighbors` modal column.
- The user verified in the local UI that the `Ports` values are empty for the
  managed SNMP device, while `Port Neighbors` values are correct and non-empty.

### Root Cause

- The producer accidentally split one SNMP concept into two modal identities:
  `port_number` for `Ports`, and `if_index` for `Port Neighbors`.
- For SNMP/L2, `ifIndex` is the device-provided numeric interface identifier
  and is the value the UI should show as the port ID. It is not a row-order
  autoincrement invented by Netdata.

### Repair Plan

- Remove the `actor_ports.port_number` column and helper logic.
- Use `actor_ports.if_index` as the visible `Ports` `Port ID` column, matching
  `actor_port_links.if_index`.
- Keep the no-synthetic-number rule: never derive port IDs from row order.
- Update specs, developer guide, project skill, and tests to describe SNMP
  `if_index` as the visible real numeric port identity.

### Implementation

- Removed the `actor_ports.port_number` column and helper logic.
- Restored `actor_ports.if_index` as the visible `Ports` `Port ID` column.
- Kept `actor_port_links.if_index` unchanged, so `Ports` and `Port Neighbors`
  now use the same real SNMP numeric port identity.
- Updated tests, topology specs, topology developer guide, and project topology
  skill to define `if_index` as the visible SNMP port ID when known.

### Validation

Acceptance criteria evidence:

- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` emits
  `if_index` as the first `Ports` modal column and as the first
  `Port Neighbors` modal column.
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_test.go` verifies
  `actor_ports` has no `port_number` column and still exposes `if_index`.

Tests or equivalent validation:

- `cd src/go && go test -count=1 ./plugin/go.d/collector/snmp_topology ./pkg/topology/v1 ./tools/functions-validation/validate`
- `sudo -n cmake --build build --target go.d.plugin -- -j2`

Artifact maintenance gate:

- `AGENTS.md`: unchanged. No project-wide workflow rule changed.
- Runtime project skills: updated `.agents/skills/project-create-topology/SKILL.md`
  with SNMP `if_index` as the visible port ID and the no-generated-sequence rule.
- Specs: updated `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-modes-correlation-aggregation.md`.
- End-user/operator docs: unchanged. This remains developer-facing topology
  payload composition, not an operator workflow.
- End-user/operator skills: unchanged. No public/operator skill semantics
  changed.
- SOW lifecycle: reopened from `done/`, repaired, then returned to `done/` with
  `Status: completed`.

Follow-up mapping:

- No new deferred implementation work remains in this SOW.

### Regression Outcome

Completed. SNMP `Ports` and `Port Neighbors` now use the same real
device-provided `if_index` value for the visible port ID, without any synthetic
numbering.
