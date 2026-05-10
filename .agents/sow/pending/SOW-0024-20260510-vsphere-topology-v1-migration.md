# SOW-0024 - vSphere topology v1 migration

## Status

Status: open

`completed` is the successful terminal status. `done` is a directory name, not a status value. Do not use `Status: done` or `Status: complete`.

Sub-state: pending until SOW-0021 graph presentation, SOW-0023 cross-payload
matching/link layout, and SOW-0022 table/modal composition are finished.

## Requirements

### Purpose

Migrate the vSphere topology producer from the superseded topology schema to `netdata.topology.v1`, preserving vSphere inventory semantics, presentation, and drilldown data while keeping the Cloud frontend producer-agnostic.

### User Request

The user asked to add a later SOW to transform vSphere to the new schema because the vSphere topology is still legacy.

### Assistant Understanding

Facts:

- The vSphere topology producer lives in the separate worktree `~/src/PRs/vsphere-extensions`.
- The vSphere producer currently uses the legacy Go topology package and `WithPresentation()`.
- Agent SOW-0021 explicitly did not migrate vSphere, but added color/icon tokens so the later vSphere migration should not need another graph-presentation schema redesign.
- Agent SOW-0023 added generic correlation rules and link layout tokens that
  vSphere must respect when migrated.
- The frontend must remain topology-schema agnostic; it must not branch on vSphere as a special UI domain.

Inferences:

- This migration should happen only after the schema, Cloud aggregator, and UI can consume the full v1 graph-presentation/table contract.
- The migration should produce v1 topology facts and presentation profiles directly, not old-schema compatibility payloads.

Unknowns:

- Whether the vSphere worktree will still be clean and owned by the same worker when this SOW starts.
- Whether SOW-0022 table/modal composition will require additional vSphere-specific actor detail table fields by the time this migration starts.

### Acceptance Criteria

- vSphere `topology:vsphere` emits `schema_version: "netdata.topology.v1"`.
- The producer no longer uses the superseded topology package for the production payload.
- Actor types cover datacenter, cluster, host, VM, datastore, network, datastore cluster, and resource pool.
- Link types cover contains, connects, runs, and any other vSphere relationship emitted by the producer.
- Actor identities use stable vSphere managed object identifiers where available, with display labels kept separate through `presentation.label_policy`.
- vSphere graph presentation is expressed through `types.actor_types.<id>.presentation`, `types.link_types.<id>.presentation`, optional `types.port_types`, and `data.presentation`.
- vSphere link types use SOW-0023 `presentation.layout.strength` and
  `presentation.layout.distance` tokens where ownership, dependency, inferred,
  or weak relationships need different graph forces.
- vSphere stable object identifiers are evaluated for SOW-0023
  `data.correlation.claims` only when they can help cross-payload matching;
  producer output must not expose aggregator internal states.
- vSphere actor detail/inventory tables use the SOW-0022 table/modal composition contract if that contract is complete before this SOW starts.
- JSON Schema validation and Go semantic validation pass.
- Payload size is measured on realistic or synthetic vSphere inventory shapes.
- The vSphere worktree owner is informed before edits start.

## Analysis

Sources checked:

- `.agents/skills/project-create-topology/SKILL.md`
- `.agents/sow/current/SOW-0021-20260509-topology-presentation-contract.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `.agents/sow/specs/topology-function-schema.md`
- `~/src/PRs/vsphere-extensions/src/go/plugin/go.d/collector/vsphere/func_topology.go`
- `~/src/PRs/vsphere-extensions/src/go/plugin/go.d/collector/vsphere/func_topology_presentation.go`

Current state:

- `func_topology.go` in the vSphere worktree declares `vsphereTopologySchemaVersion = "2.0"` and returns `topology.Data`.
- `vsphereTopologyMethodConfig()` attaches legacy presentation with `WithPresentation(vsphereTopologyPresentation())`.
- `func_topology_presentation.go` defines legacy actor/link presentation for vSphere actor and link types.
- The vSphere worktree was checked when this SOW was created and had no modified files under the vSphere collector.

Risks:

- Starting this before SOW-0021/SOW-0023/SOW-0022 integration settles may force duplicate migration work.
- Editing the vSphere worktree without coordination may collide with another worker.
- Mixing old-schema reconstruction metadata into the v1 payload would violate the topology schema contract.
- Treating vSphere display names as identities would make aggregation and cross-payload matching fragile.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- vSphere remains on the superseded topology schema while the rest of the topology work is moving to `netdata.topology.v1`.
- SOW-0021 intentionally left vSphere unmigrated and only preserved the token vocabulary needed for a later migration.
- This SOW is blocked until the core v1 contract, frontend rendering, Cloud aggregation, cross-payload matching, and table/modal composition are complete enough to avoid rework.

Evidence reviewed:

- SOW-0021 vSphere migration posture records that vSphere is not migrated by SOW-0021 and should be coordinated separately.
- SOW-0023 records the final correlation and link layout contract that vSphere
  must use when migrated.
- The vSphere worktree producer still uses the legacy topology package and `WithPresentation()`.
- The project topology skill states the vSphere worktree must not be edited
  before telling the user because another agent may be working there.

Affected contracts and surfaces:

- vSphere Function output schema.
- vSphere topology producer implementation in the separate worktree.
- Topology JSON Schema and semantic validator compatibility.
- Cloud frontend graph presentation and actor modal/table rendering.
- Cloud topology service aggregation and cross-payload matching.
- Tests/fixtures for vSphere inventory topology.

Existing patterns to reuse:

- `src/go/pkg/topology/v1` compact-table helpers from this worktree.
- Network-connections, streaming, and SNMP v1 producers after SOW-0021.
- SOW-0022 table/modal composition contract once complete.
- SOW-0023 correlation rules, pure correlation actors, point/claim tables, and
  link layout tokens once complete.

Risk and blast radius:

- Medium: vSphere topology is a producer-specific migration, but it touches public Function payload shape.
- Compatibility: Cloud frontend must continue old-schema support until the Agent rollout is complete.
- Performance: large vSphere inventories may produce many actors/links and require compact tables.
- Security: vSphere managed object ids, inventory paths, hostnames, datastores, networks, and labels can identify private infrastructure and must not be copied raw into durable artifacts.

Sensitive data handling plan:

- Do not commit raw vSphere payload captures.
- Use synthetic or sanitized fixtures.
- Redact private inventory names, managed object ids, hostnames, datastore names, network names, private endpoints, credentials, tokens, and customer data from SOWs, docs, skills, tests, and review artifacts.

Implementation plan:

1. Reconfirm with the user before editing `~/src/PRs/vsphere-extensions`.
2. Read completed SOW-0021, SOW-0023, and SOW-0022 outcomes.
3. Inventory current vSphere actors, links, attributes, labels, and presentation.
4. Design the v1 actor/link/evidence/table types for vSphere.
5. Implement v1 payload generation using compact tables.
6. Preserve graph presentation through v1 type-level and graph-level presentation.
7. Add fixtures/tests and validate against the schema and semantic validator,
   including link layout tokens and any vSphere correlation claims.
8. Coordinate Cloud frontend and Cloud aggregator validation with generic topology fixtures.

Validation plan:

- Go unit tests for vSphere v1 payload shape.
- JSON Schema validation against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Semantic validation using `src/go/pkg/topology/v1`.
- Payload size measurement on synthetic or sanitized vSphere inventory shapes.
- Frontend manual check through generic topology rendering after UI support lands.
- Cloud aggregator fixture once the service supports the final v1 presentation/table/matching contracts.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`
  only if migration reveals new reusable vSphere topology guidance.
- Specs: update `.agents/sow/specs/topology-function-schema.md` if this migration changes durable topology semantics.
- End-user/operator docs: update only if public vSphere topology behavior or Function docs exist and change.
- End-user/operator skills: update only if public topology skill guidance changes.
- SOW lifecycle: keep this SOW pending until SOW-0021, SOW-0023, and SOW-0022 are done.

Open-source reference evidence:

- None checked. This is an internal producer migration from the legacy Netdata topology contract to the new Netdata topology contract.

Open decisions:

- None for creating this pending SOW.
- Before implementation starts, confirm the vSphere worktree ownership and whether SOW-0022 table/modal composition is mandatory for the first vSphere v1 payload.

## Implications And Decisions

- User decision: vSphere migration is tracked as a later SOW after the current topology schema/UI/Cloud work.
- This SOW must not be started before the other topology SOWs are finished unless the user explicitly changes the order.

## Plan

1. Wait for SOW-0021, SOW-0023, and SOW-0022 completion.
2. Confirm vSphere worktree ownership with the user.
3. Move this SOW to current and fill any newly discovered implementation specifics.
4. Implement and validate the vSphere migration to `netdata.topology.v1`.

## Execution Log

### 2026-05-10

- Created pending SOW after the user noted that vSphere remains legacy and should be migrated after the other topology SOWs.

- Updated prerequisites after SOW-0023 added declarative correlation rules and
  link layout tokens. vSphere migration must use the final v1 contract rather
  than the older presentation-only contract.

## Validation

Acceptance criteria evidence:

- Pending; this SOW has not started implementation.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending.

Reviewer findings:

- Pending.

Same-failure scan:

- Pending.

Sensitive data gate:

- This SOW contains only sanitized file references and no raw vSphere inventory payloads or credentials.

Artifact maintenance gate:

- AGENTS.md: not updated; workflow rules did not change.
- Runtime project skills: not updated; this SOW only records future work.
- Specs: not updated; behavior has not changed yet.
- End-user/operator docs: not updated; behavior has not changed yet.
- End-user/operator skills: not updated; behavior has not changed yet.
- SOW lifecycle: open in `.agents/sow/pending/` and blocked on earlier topology SOWs.

Specs update:

- No spec update yet because no behavior changed.

Project skills update:

- No project skill update yet because no workflow changed.

End-user/operator docs update:

- No docs update yet because no behavior changed.

End-user/operator skills update:

- No skill update yet because no behavior changed.

Lessons:

- vSphere must be tracked explicitly because SOW-0021 prepared the schema for it but did not migrate its producer.

Follow-up mapping:

- Implemented by this pending SOW after SOW-0021, SOW-0023, and SOW-0022 finish.

## Outcome

Pending.

## Lessons Extracted

Pending until implementation.

## Followup

- Start this after SOW-0021, SOW-0023, and SOW-0022 are complete.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
