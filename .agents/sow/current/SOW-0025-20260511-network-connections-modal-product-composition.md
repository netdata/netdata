# SOW-0025 - Network Connections Modal Product Composition

## Status

Status: paused

Sub-state: paused by SOW-0028. The network-connections modal work exposed a
larger topology mode, correlation, aggregation, and modal-identification
contract gap that must be specified and implemented before this narrower modal
composition SOW can resume.

## Requirements

### Purpose

Make `topology:network-connections` actor modals useful for sysadmins, DevOps engineers, and SREs who need to understand process/socket dependencies quickly and accurately.

### User Request

The user reported that network-connections modals currently show `Connections`, `Ports`, and, in detailed mode, `Socket Evidence`, which creates three confusing views over the same socket facts. The user wants the work handled mechanically and separately from SNMP and streaming.

### Assistant Understanding

Facts:

- The v1 schema can express actor labels, modal sections, projections, owner filters, and table columns without duplicating rows.
- Current network-connections modal recipes are generated in `src/collectors/network-viewer.plugin/network-viewer.c`.
- Current actor modals are declared generically for actor types and add:
  - `Connections` from `links`, excluding `ownership`;
  - `Ports` from actor table `socket_ports` when port bullets are enabled;
  - `Socket Evidence` from `evidence.socket` when detailed evidence exists.
- `network-viewer.c` currently emits actor labels and typed actor columns such as process, username, command line, namespace, local IP, address space, socket count, PID, UID, and netns inode.

Inferences:

- The schema contract is not the main problem for this function. The issue is table product design: the current tabs reflect implementation sources instead of the troubleshooting questions users ask.
- `Ports` and `Socket Evidence` are not independently useful as top-level tabs unless their relationship to the connection rows is obvious.

Unknowns:

- Whether the frontend can already promote selected actor labels into the modal identification/header area, or whether this requires a frontend contract extension in addition to producer recipe changes.
- Whether all old-network-connections modal columns can be reconstructed from current v1 rows without adding new canonical columns.

### Acceptance Criteria

- A complete inventory exists for network-connections actor modal facts: actors, actor labels, links, `socket_ports`, and `evidence.socket`.
- The SOW maps every useful old modal/table field to a v1 source, or records the missing canonical field to add.
- The final modal design avoids three duplicate tabs for the same socket relationship.
- Process, endpoint, and node/self actors each have an explicit useful modal shape.
- The actor identification area has a declared source for important labels, not only the generic `Labels` tab.
- Backend producer changes, schema/doc updates if needed, frontend TODO, and validation plan are completed before this SOW closes.

## Analysis

Sources checked:

- `src/collectors/network-viewer.plugin/network-viewer.c:2505` modal column helper.
- `src/collectors/network-viewer.plugin/network-viewer.c:2543` selected-side socket endpoint projection.
- `src/collectors/network-viewer.plugin/network-viewer.c:2647` actor modal emission.
- `src/collectors/network-viewer.plugin/network-viewer.c:2667` current `Connections` section.
- `src/collectors/network-viewer.plugin/network-viewer.c:2710` current `Ports` section.
- `src/collectors/network-viewer.plugin/network-viewer.c:2738` current `Socket Evidence` section.
- `src/collectors/network-viewer.plugin/network-viewer.c:2867` process actor type enables port bullets from `socket_ports`.
- `src/collectors/network-viewer.plugin/network-viewer.c:2937` `socket_ports` table type.
- `src/collectors/network-viewer.plugin/network-viewer.c:2962` `actor_labels` table type.
- `.agents/sow/specs/topology-function-schema.md:351` network-connections actor/link/socket-port schema notes.

Current state:

- The current modal exposes source tables directly:
  - graph links for high-level connections;
  - actor-owned socket-port inventory;
  - detailed socket evidence rows.
- This is technically correct but product-confusing because a user sees multiple tabs that appear to describe the same thing with different missing columns.
- Actor identification facts exist in actor labels and typed actor columns, but the modal header/identification area is not using selected important labels.

Available facts to inventory:

- Node/self actor:
  - `display_name`, `type`, `hostname`, `machine_guid`, `socket_count`, `local_ip_count`.
- Process actor:
  - `display_name`, `type`, `process`, `username`, `cmdline`, `namespace_type`, `local_ip`, `local_address_space`, `socket_count`, optional `pid`, optional `uid`, optional `net_ns_inode`.
- Endpoint actor:
  - `display_name`, `type`, `ip`, `address_space`, `socket_count`.
- Graph links:
  - opposite actor, link type, protocol, direction, state, socket count, and link-level metrics.
- `socket_ports`:
  - actor, port, protocol, direction, socket count.
- `evidence.socket`:
  - exact source/destination actors and socket endpoint details, protocol, direction, state, RTT/retransmission fields when available.

Field inventory and mapping:

| Useful old/current fact | Existing v1 source | Gap / action |
|---|---|---|
| Actor display name | `actors.display_name`, `actor_labels.display_name` | Available. Use in actor label policy and modal header. |
| Hostname | `actors.hostname`, `actor_labels.hostname` | Available for self actor. Promote to modal header. |
| Machine GUID | `actors.machine_guid`, `actor_labels.machine_guid` | Available. Keep in labels; promote only if useful for debugging. |
| Process name | `actors.process`, `actor_labels.process` | Available. Promote to process modal header. |
| Process user | `actors.username`, `actor_labels.username` | Available. Promote to process modal header. |
| Process command line | `actors.cmdline`, `actor_labels.cmdline` | Available. Promote to process modal header or expanded header; sensitive but allowed by Function classification. |
| Namespace type | `actors.namespace_type`, `actor_labels.namespace_type` | Available. Promote to process modal header. |
| Process local IP/address space | `actors.local_ip`, `actors.local_address_space`, labels | Available. Promote when present. |
| PID/UID/netns inode | typed actor columns and labels when `processes:by_pid` | Available. Header for `by_pid`, labels for `by_name`. |
| Endpoint IP/address space | `actors.ip`, `actors.address_space`, labels | Available. Promote to endpoint modal header. |
| Socket count | actor/link/evidence metrics, labels | Available. Promote to header and tables. |
| Direction/protocol/state | `links`, `evidence.socket`, internal `NV_TOPOLOGY_LINK` | Available. Use badges in connection rows. |
| Local/remote IP and port | `evidence.socket` in detailed mode; internal `NV_TOPOLOGY_LINK` in all modes | Missing from aggregated v1 modal source. Add one relationship-summary table from existing internal rows. |
| Server port/service name | internal `NV_TOPOLOGY_LINK.remote_port` or `local_port`, `port_name` | Missing from emitted v1 tables except exact socket evidence has ports but not `port_name`. Add to relationship summary; add `service_name` to evidence if needed. |
| RTT / receiver RTT / retransmissions | `links` and `evidence.socket` metrics | Available. Relationship summary should expose the same metrics per summary row. |
| Local process ownership / node keeps graph coherent | `ownership` links | Available. Self modal should use this as a process list; process modal should not show it as a network connection. |

Important evidence:

- Internal connection summary rows already contain process identity, local/remote IPs, local/remote ports, peer port, protocol, direction, state, address spaces, service name, command line, socket count, and latency/retransmission metrics in `NV_TOPOLOGY_LINK` (`src/collectors/network-viewer.plugin/network-viewer.c:80`).
- Those internal rows are keyed by process, namespace, local IP, remote IP, protocol, direction, state, local port, and endpoint port (`src/collectors/network-viewer.plugin/network-viewer.c:1283`).
- v1 graph links intentionally collapse those rows by source actor, destination actor, link type, protocol, direction, and state only (`src/collectors/network-viewer.plugin/network-viewer.c:2135`), so port pairing and service name cannot be reconstructed from graph links alone.
- Current modal sections expose `links`, `socket_ports`, and `evidence.socket` as peer sections (`src/collectors/network-viewer.plugin/network-viewer.c:2647`). This mirrors storage tables, not the user's troubleshooting workflow.
- The current schema supports `relationship_table` modal sources and `relationship_summary` table roles, so an aggregated connection-summary table fits the existing contract without inventing a network-connections-specific UI path.

Target audience and questions:

- Sysadmin/SRE process drilldown:
  - What is this process?
  - Which user/cmdline/container/namespace is it?
  - What is it listening on?
  - What remote endpoints or local processes is it connected to?
  - How many sockets are involved?
  - Which entries are unresolved correlation endpoints?
- Sysadmin/SRE endpoint drilldown:
  - Which processes connect to this endpoint?
  - Is it inbound, outbound, listening, local, TCP, UDP?
  - Is the endpoint local/private/public?
- Sysadmin/SRE node drilldown:
  - Which processes on this node participate in network activity?
  - Which local IPs are observed?

Target product model:

- Self/node actor modal:
  - Header: hostname, observed socket count, local IP count.
  - Primary table: `Processes` derived from `ownership` links, showing process actor and socket count.
  - No raw socket table; self is a scope/root actor, not a socket endpoint.
- Process actor modal:
  - Header: process name/display name, user, namespace type, command line, local IP/address space, socket count, and PID/netns when available.
  - Primary table: `Connections`, sourced from one relationship-summary table in aggregated mode and from `evidence.socket` in detailed mode.
  - Columns should answer "where does this process connect/listen?": peer actor or endpoint, local endpoint, remote endpoint, service/port, protocol, direction, state, sockets, RTT/retransmissions.
  - Port bullets remain visual graph affordances. A separate `Ports` modal tab should not be shown unless it provides a distinct, clearly labeled local-port inventory.
- Endpoint actor modal:
  - Header: IP, address space, socket count.
  - Primary table: `Processes`, sourced from the same connection rows, showing the local process, local endpoint, endpoint port/service, protocol, direction, state, sockets, RTT/retransmissions.
  - No `Ports` tab; the endpoint is already the port/IP drilldown target.

Risks:

- Leaving `Connections`, `Ports`, and `Socket Evidence` as peers makes users compare inconsistent row sets and assume data is wrong.
- Hiding process identity under `Labels` makes the modal feel empty even when the payload contains the information.
- Detailed evidence can be high-cardinality; it should be expandable or secondary, not a default duplicated top-level list unless the user explicitly asked for detailed sockets.
- If aggregated mode continues to use only graph links plus `socket_ports`, it cannot show accurate local/remote port pairing. `socket_ports` is actor inventory, not a relationship table.
- If the frontend guesses important labels from names such as `process`, `username`, or `cmdline`, the UI becomes topology-specific and violates the v1 contract that producers define presentation.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The schema migration preserved facts but lost product intent. The producer emits table recipes that mirror internal sources instead of user workflows.
- The actor identification area is empty because important labels are only exposed through the `actor_labels` table and not selected for header display.

Evidence reviewed:

- Current network-connections modal sections are emitted in `src/collectors/network-viewer.plugin/network-viewer.c:2647-2760`.
- Current network-connections actor labels are emitted in `src/collectors/network-viewer.plugin/network-viewer.c:2029-2102`.
- Current network-connections table-type presentation for `socket_ports` and `actor_labels` is emitted in `src/collectors/network-viewer.plugin/network-viewer.c:2937-2979`.

Affected contracts and surfaces:

- Agent Function payload for `topology:network-connections`.
- v1 topology schema only if the identification/header area requires a new contract.
- Cloud frontend actor modal rendering.
- Cloud aggregator table/label preservation.
- Developer guide, durable topology spec, and project topology skill.

Existing patterns to reuse:

- `actor_labels` for labels and identity facts.
- Modal sections over existing `links`, `evidence`, and `actor_table` sources.
- `selected_side_endpoint` for socket endpoint rendering.
- `socket_ports` with numeric `socket_count` for aggregated port bullets.

Risk and blast radius:

- User-facing modal behavior changes for network-connections only.
- Payload size risk is moderate if detailed evidence is duplicated. This SOW must not duplicate socket rows only for modal display.
- Sensitive data risk is high because process command lines, usernames, and local endpoints are intentionally exposed to authorized users.

Sensitive data handling plan:

- Do not copy raw command lines, usernames, bearer tokens, cookies, public IPs from user systems, or customer-identifying endpoint data into durable artifacts.
- Use synthetic examples only.
- Treat all `actor_labels` and socket evidence as topology Function sensitive data.

Implementation plan:

1. Inventory the old and current modal fields for node/self, process, and endpoint actors.
2. Define the intended modal composition for each actor type:
   - selected identity/header labels;
   - one primary connection table;
   - optional listening/local-port summary;
   - optional detailed evidence expansion/section only when detailed evidence exists and adds information.
3. Add or adjust canonical columns only when the useful field is missing from actors, links, `socket_ports`, or `evidence.socket`.
4. Update `network-viewer.c` modal recipes without duplicating socket evidence.
5. Update docs/spec/skill if the schema contract or recommended network-connections shape changes.
6. Produce a frontend TODO if the header/identification area needs schema support or UI changes.

Validation plan:

- Validate generated payload JSON against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Run C syntax check for `network-viewer.c`.
- Use local Agent Function output in aggregated and detailed modes.
- Confirm actor modals for self/node, process, and endpoint actors show non-empty identity and a non-duplicative useful connection view.
- Check payload size before/after on a realistic network-connections payload.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md` if the recommended network-connections modal shape changes.
- Specs: update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: likely unaffected unless Function output docs expose examples.
- End-user/operator skills: unaffected.
- SOW lifecycle: close only after integrated local Agent/UI validation or an explicit tracked follow-up SOW exists.

Open-source reference evidence:

- Not checked yet. This SOW is about Netdata-specific Function modal semantics; external references may be useful only for general socket table UX and should be recorded if used during analysis.

Open decisions:

- Decide how the producer tells the UI which actor labels belong in the modal identification/header area.
- Decide whether to add a relationship-summary table for aggregated network-connections drilldown rows.
- Decide whether detailed socket evidence should be the primary process/endpoint table in detailed mode, or remain a separate secondary tab.

## Implications And Decisions

### Decision 1: Modal Identification/Header Contract

Evidence:

- The schema has `presentation.modal.labels` and `presentation.modal.sections`, but no field that selects important labels for the modal header (`src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:1073`).
- The producer already emits the needed labels for self, process, and endpoint actors (`src/collectors/network-viewer.plugin/network-viewer.c:2029`, `src/collectors/network-viewer.plugin/network-viewer.c:2065`, `src/collectors/network-viewer.plugin/network-viewer.c:2101`).

Options:

- **A. Extend `presentation.modal.labels` with ordered `summary_keys` / `identity_keys`. Recommended.**
  - Pros: small schema change; keeps UI topology-agnostic; reuses `actor_labels`; no row duplication.
  - Cons: requires frontend and aggregator to preserve/read the new label presentation field.
  - Implication: each actor type can say which label keys appear in the header, while the full `Labels` tab remains complete.
- **B. Add a more general `presentation.modal.identity.fields[]` using modal projections.**
  - Pros: most flexible; can pull from actor columns or labels.
  - Cons: larger schema/UI work; more ways for producers to create inconsistent header/table semantics.
  - Implication: useful later, but heavier than this SOW needs.
- **C. Let the frontend guess from label keys.**
  - Pros: no schema change.
  - Cons: violates producer-driven presentation; the UI would learn network-connections-specific labels.
  - Risk: repeats the old problem in another form.

Recommendation: **A**. The header is a curated subset of `actor_labels`, not a new data source.

### Decision 2: Aggregated Mode Connection Detail Source

Evidence:

- Internal `NV_TOPOLOGY_LINK` rows preserve aggregated connection detail, including local/remote ports and service name (`src/collectors/network-viewer.plugin/network-viewer.c:80`).
- v1 graph links intentionally remove port pairing from the graph-link key (`src/collectors/network-viewer.plugin/network-viewer.c:2135`).
- `socket_ports` stores only actor-owned port summaries (`src/collectors/network-viewer.plugin/network-viewer.c:2235`), so it cannot answer "which remote endpoint used which port?".

Options:

- **A. Add `data.tables.relationship.connections` as a `relationship_summary` table. Recommended.**
  - Pros: uses existing internal rows once; restores accurate aggregated modal drilldown; no high-cardinality socket duplication in aggregated mode.
  - Cons: increases aggregated payload size by one compact row per internal connection summary.
  - Implication: graph links stay small, while modals use a relationship table for precise rows.
- **B. Keep using graph links for aggregated modals.**
  - Pros: smallest payload.
  - Cons: cannot show local/remote port pairing or service names; leaves the current confusing modal mostly intact.
  - Risk: not fit for the requested SRE workflow.
- **C. Use only `socket_ports` as the drilldown.**
  - Pros: compact and already emitted.
  - Cons: actor inventory is not relationship data; outbound local ephemeral ports and remote server ports get confused.
  - Risk: exactly the kind of misleading table the user reported.

Recommendation: **A**. The summary table is not duplicated presentation data; it is the missing relationship fact plane for aggregated drilldowns.

### Decision 3: Detailed Mode Modal Shape

Evidence:

- Detailed mode already emits `evidence.socket` rows with exact local/remote socket details (`src/collectors/network-viewer.plugin/network-viewer.c:2254`).
- Current detailed modal shows both graph `Connections`, actor `Ports`, and `Socket Evidence` (`src/collectors/network-viewer.plugin/network-viewer.c:2664`, `src/collectors/network-viewer.plugin/network-viewer.c:2706`, `src/collectors/network-viewer.plugin/network-viewer.c:2735`).

Options:

- **A. In detailed mode, make exact socket evidence the single primary `Sockets` / `Connections` section. Recommended.**
  - Pros: no duplicated tabs; exact rows are shown when the user selected detailed mode; expanded columns can show less-common fields.
  - Cons: detailed mode tables can be large, as expected by the mode.
  - Implication: hide graph-link and port-summary sections from detailed actor modals unless they add a distinct summary later.
- **B. Keep graph `Connections` and make socket evidence expandable below each row.**
  - Pros: best UX if frontend supports row-level grouping by link.
  - Cons: more frontend work; current schema cannot directly express nested row groups, only expanded columns.
  - Implication: good future improvement, but not the fastest reliable repair.
- **C. Keep the three current tabs with clearer labels.**
  - Pros: smallest backend change.
  - Cons: still forces users to reconcile three views of the same fact.
  - Risk: fails the purpose of this SOW.

Recommendation: **A**. Detailed mode should show the exact socket rows as the primary drilldown; aggregated mode should show the relationship summary rows.

### Schema Support Assessment

Supported by the current schema:

- Actor modal recipes are already supported under `types.actor_types.<id>.presentation.modal`.
- Modal sections can already read `links`, `evidence`, `actor_table`, and `relationship_table` sources.
- `relationship_summary` table types are already valid.
- Compact table columns can already use `actor_ref`, `link_ref`, `ip`, `uint`, metrics, strings, and dictionary encoding.
- Owner filters can already bind a relationship table to the selected actor when the table has `src_actor` and `dst_actor` columns.
- `selected_side_endpoint` already supports rendering a local/remote endpoint from table columns without topology-specific frontend code.

Not supported yet:

- There is no schema field for "show these actor label keys in the modal identification/header area". Current `modal.labels` only tells the UI how to render the full labels table.

Duplication policy:

- Do not duplicate raw detailed socket evidence only for modal display.
- Aggregated mode needs one relationship-summary row per existing internal connection summary row because graph links intentionally collapse port pairing and service name. This is not duplicate evidence; it is a distinct compact drilldown grain between graph links and detailed socket evidence.
- Some small scalar values such as protocol, direction, state, socket count, and actor refs will appear both on graph links and relationship-summary rows. This is acceptable because the rows have different grain, are dictionary/number encoded, and are needed for standalone modal filtering/sorting.
- The producer should avoid copying fields that can be projected from actor labels or actor rows. Process identity stays in actors/labels; relationship rows carry relationship facts.

## Plan

1. Analyze field inventory and old/current modal parity for `topology:network-connections`.
2. Propose the product-oriented modal tables and header labels.
3. Implement only this function's backend changes after the design is accepted.
4. Hand off required frontend/aggregator behavior if needed.
5. Validate with real local Agent payloads in aggregated and detailed modes.

## Execution Log

### 2026-05-11

- Created SOW from user-reported modal regressions and current code evidence.
- Paused this SOW because SOW-0028 owns the broader cross-repo topology mode,
  correlation, aggregation, and actor-identification contract that this SOW now
  depends on.

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
