# SOW-0026 - SNMP Modal Product Composition

## Status

Status: open

Sub-state: pending product analysis and implementation. This SOW is intentionally separate from network-connections and streaming modal work.

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

Unknowns:

- Whether every link row currently carries enough local endpoint information to map it back to `actor_ports` by actual port ID/name.
- Whether the SNMP topology engine has stable `if_index`, `if_name`, `if_descr`, and alias fields for every port row and link endpoint.
- Whether endpoint/inferred actors should keep a `Links` section, while devices use a port-centric `Ports / Neighbors` section.

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

Current state:

- Device modal:
  - `Ports` comes from `actor_ports`.
  - `Links` comes from graph links.
- `actor_ports` currently has stable columns such as `name`, topology role, admin/oper status, port type, link mode, STP state, VLAN IDs, FDB count, link count, neighbor count, neighbors JSON, VLANs JSON, and debug `extra`.
- `Links` currently uses selected-side projections for `src_port_name` and `dst_port_name`, but the user observed cases where local port looks like a remote actor/IP. This points to a possible endpoint mapping or projection issue.

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

Risks:

- If port and link tables are inconsistent, users will distrust the topology.
- Showing raw nested neighbor JSON in normal table cells would regress polish and usefulness.
- If numeric port IDs are unavailable, fabricating autoincrement IDs would be worse than showing no numeric ID.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- SNMP modals currently expose implementation tables separately rather than presenting a device as a collection of ports with attached neighbor/link evidence.
- The current `Links` section can produce confusing local/remote port labels because it is graph-link-oriented, not port-oriented.

Evidence reviewed:

- SNMP device modal currently has both `Ports` and `Links` sections in `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:158-180`.
- `Links` local/remote port columns are selected-side projections in `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:212-213`.
- `actor_ports` modal currently does not expose a clear numeric port ID column in the visible section at `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go:493-503`.

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

1. Inventory all current SNMP modal facts and old-schema table behavior.
2. Define a port-centric device modal:
   - actor header labels;
   - primary `Ports` table with numeric port ID when known and port name;
   - neighbor/link columns derived from the same canonical port/link mapping;
   - expandable or secondary evidence details only when useful.
3. Determine whether `Links` should be removed for device actors, renamed to `Neighbors`, or retained only for inferred endpoint/segment actors.
4. Add missing canonical columns to `actor_ports` and/or links/evidence, especially actual `if_index` and stable port identifiers.
5. Add cross-check tests proving port rows and link endpoint rows agree.
6. Update docs/spec/skill and frontend/aggregator handoff if needed.

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

- Not checked yet. If port/link presentation design needs external grounding, compare with network inventory tools such as Netdisco/LibreNMS/OpenNMS from local mirrors and record upstream repo/commit references.

Open decisions:

- Decide whether managed SNMP device actors should have one port-centric table instead of separate `Ports` and `Links` sections.
- Decide whether endpoint/segment actors keep a relationship-oriented `Links` table.

## Implications And Decisions

Pending. User decision is required after the field inventory and proposed modal layout are written.

## Plan

1. Inventory old/current SNMP modal fields and current v1 canonical data.
2. Design a port-centric modal model for managed devices.
3. Identify missing canonical port/link fields.
4. Implement only the SNMP producer changes after design acceptance.
5. Validate with tests and real/sanitized SNMP payloads.

## Execution Log

### 2026-05-11

- Created SOW from user-reported SNMP modal regressions and current code evidence.

## Validation

Acceptance criteria evidence:

- Pending.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending.

Reviewer findings:

- Pending.

Same-failure scan:

- Pending.

Sensitive data gate:

- This SOW uses only path/line evidence and synthetic descriptions. No raw sensitive payload data is included.

Artifact maintenance gate:

- Pending.

Specs update:

- Pending.

Project skills update:

- Pending.

End-user/operator docs update:

- Pending.

End-user/operator skills update:

- Pending.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
