# SOW-0028 - Topology Mode Correlation Aggregation Contract

## Status

Status: completed

Sub-state: completed. The cross-repo compatibility contract is implemented and
validated. True one-sided detailed network-connections graph rows are split to
SOW-0029 because they require a separate Agent/UI/aggregator execution pass.

## Requirements

### Purpose

Make topology maps and actor modals fit for SRE, DevOps, sysadmin, and network
engineer workflows by defining one contract for detailed evidence, aggregated
views, cross-payload correlation, table merging, and actor modal identification.

### User Request

The user asked to create the full specification first, then create SOW/TODO
handoff artifacts for every affected repo, then implement the Agent, Cloud
frontend, and Cloud topology aggregation service without relying on chat
context.

The user explicitly requested the spec to cover:

- Agent detailed and aggregated views;
- aggregator detailed and aggregated views while consuming detailed input;
- UI detailed and aggregated views;
- network-connections;
- SNMP/L2;
- streaming;
- actor identification from selected labels in actor modals.

### Assistant Understanding

Facts:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` already has
  `data.view.mode`, actor/link type presentation, modal recipes, actor labels,
  and a correlation section.
- Current correlation documentation is centered on pure correlation actors and
  absorb/link actions.
- The current network-connections modal analysis showed a broader contract gap:
  actor modal identification needs producer-selected labels, and aggregated
  network-connections needs relationship-summary rows for useful drilldowns.
- SNMP and streaming do not currently have a meaningful detailed/aggregated
  mode split.

Inferences:

- The previous pure-correlation-actor model is not enough for
  network-connections detailed mode because exact remote `IP:PORT` tuples should
  often be loose relationship facts rather than actors.
- SNMP/L2 needs replacement semantics, not loose-side socket resolution.
- Streaming needs merge/enrichment semantics, not replacement.
- The aggregator must consume detailed payloads before returning an aggregated
  view, otherwise cross-node correlation loses facts too early.

Unknowns:

- Exact final schema field names may need small adjustments during
  implementation to fit existing v1 schema style and frontend normalizer
  patterns.
- The aggregator service may already implement a subset of SOW-0028 behavior;
  this must be verified against its current code before patching.

### Acceptance Criteria

- `.agents/sow/specs/topology-modes-correlation-aggregation.md` defines the
  full contract and examples for network-connections, SNMP/L2, and streaming.
- This SOW records the cross-repo pending checklist for Agent, UI, and
  aggregator work.
- A frontend TODO exists in the Cloud frontend repo and references the new spec.
- A Cloud topology service SOW exists in the aggregator repo and references the
  new spec.
- Agent schema and topology producers are updated to the new contract where the
  Agent owns the data.
- Cloud frontend is updated to decode/render actor modal identification and the
  new mode/correlation semantics it must handle.
- Cloud topology service is updated to aggregate according to the new contract.
- Validation evidence records schema tests, relevant unit tests, and local
  Function/API checks that were possible.

## Analysis

Sources checked:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `.agents/sow/specs/topology-function-schema.md`
- `.agents/skills/project-create-topology/SKILL.md`
- `.agents/sow/done/SOW-0025-20260511-network-connections-modal-product-composition.md`
- Cloud frontend `TODO-topology-modal-composition-contract.md`
- Cloud topology service `AGENTS.md`
- Cloud topology service `.agents/sow/specs/cloud-topology-service-contract.md`

Current state:

- `data.view.mode` can state `aggregated` or `detailed`, but the durable specs
  do not yet define layer-by-layer mode behavior.
- `modal.labels` defines the label table shape, but cannot yet select important
  labels for the actor modal identification/header area.
- Existing correlation docs assume pure correlation actors. The new model needs
  loose-side resolution for network-connections, replacement for SNMP/L2, and
  enrichment/table merging for streaming.
- SOW-0025, SOW-0026, and SOW-0027 are narrower modal composition SOWs that
  depend on this broader contract.

Risks:

- A weak spec will recreate the same bug in three places: producer emits one
  meaning, aggregator merges another, UI renders a third.
- Actor-per-`IP:PORT` detailed network-connections output can explode graph
  size and make the map unreadable.
- Random conflict resolution in the aggregator can hide real dependencies.
- Duplicating evidence rows for modal display can recreate the original payload
  size problem.
- Actor labels may contain sensitive system metadata, users, command lines, or
  endpoint data. Durable artifacts must use synthetic examples only.

## Pre-Implementation Gate (Historical Snapshot at Implementation Start)

Status at implementation start: ready (historical snapshot; final closure evidence is in the Execution Log and Validation sections).

Problem / root-cause model:

- The v1 topology contract optimized payload size and presentation tokens, but
  mode and correlation semantics were still incomplete. Network-connections,
  SNMP/L2, and streaming require different correlation outcomes: loose-side
  resolution, actor replacement, and actor enrichment.
- Actor modals lost important identification because the schema exposes labels
  as a full table but does not let producers select the labels that belong in
  the modal header.

Evidence reviewed:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:179` defines `data.view.mode`.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:469` defines current
  `data.correlation`.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json:1097` defines current
  `modal_labels_presentation` without selected identification fields.
- `.agents/sow/done/SOW-0025-20260511-network-connections-modal-product-composition.md`
  records the missing modal header label contract and the need for
  relationship-summary rows in aggregated network-connections.

Affected contracts and surfaces:

- Agent topology JSON schema and developer guide.
- `topology:network-connections` producer.
- `topology:snmp` producer/adapters, to ensure no false mode selector and to
  prepare replacement metadata.
- `topology:streaming` producer, to ensure no false mode selector and to prepare
  merge/enrichment metadata.
- Cloud frontend v1 normalizer and actor modal.
- Cloud topology service aggregation core and request fanout.
- Project topology skill and durable specs.

Existing patterns to reuse:

- Compact tables with `nullable` actor-ref columns.
- Actor labels through `tables.actor.actor_labels`.
- Type-level presentation under `types.actor_types.<id>.presentation` and
  `types.link_types.<id>.presentation`.
- Modal sections over existing facts, not duplicate modal-only row stores.
- Cloud topology service SOW framework and aggregator tests.
- Cloud frontend v1 decoder/normalizer and modal table work.

Risk and blast radius:

- High semantic risk: wrong correlation can hide real dependencies or create
  false dependencies.
- Medium payload risk: relationship-summary rows add payload size, but avoid
  much larger actor-per-socket graphs.
- Medium UI risk: modal identification and loose-side materialization must not
  introduce topology-specific frontend code.
- Low operational risk for Agent if schema validation and local Function output
  remain valid.

Sensitive data handling plan:

- Use only synthetic RFC 5737 IP ranges and synthetic host/process names in
  specs, SOWs, TODOs, tests, and examples.
- Do not store raw Function captures, bearer tokens, cookies, usernames,
  command lines from local systems, SNMP communities, customer names, or
  customer-identifying public endpoints in durable artifacts.
- Treat `actor_labels`, socket tuples, host labels, process command lines, and
  SNMP metadata as sensitive Function data.

Implementation plan:

1. Create the durable spec and cross-repo work items.
2. Extend the Agent schema for modal label identification and mode/correlation
   metadata needed by the spec.
3. Update topology developer docs and project topology skill.
4. Update Agent producers:
   - network-connections mode and loose-side/relationship-summary semantics;
   - SNMP mode/correlation metadata where Agent owns it;
   - streaming merge/table metadata where Agent owns it.
5. Update Cloud frontend to decode/render modal identification and supported
   mode/correlation metadata without domain-specific guesses.
6. Update Cloud topology service to consume detailed inputs, correlate/merge by
   declared policies, and return detailed/aggregated outputs.
7. Validate all touched repositories.

Validation plan:

- Agent: JSON schema validation, narrow C checks/build checks, and local
  Function payload checks for network-connections, SNMP, and streaming where
  available.
- UI: unit tests for decoder/normalizer/modal identification and local build or
  focused test command where available.
- Aggregator: Go tests for request fanout mode rewrite, loose-side resolution,
  SNMP replacement, streaming enrichment, and table merge policies.
- Same-failure search for stale pure-correlation-only guidance.

Artifact impact plan:

- AGENTS.md: likely unchanged; existing SOW and public-skill boundary rules are
  already clear.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`
  so future topology work follows this spec.
- Specs: add
  `.agents/sow/specs/topology-modes-correlation-aggregation.md` and update
  `.agents/sow/specs/topology-function-schema.md` if needed.
- End-user/operator docs: likely unchanged because this is developer/internal
  topology schema work.
- End-user/operator skills: unchanged unless a public querying skill documents
  developer validation, which must remain avoided.
- SOW lifecycle: pause SOW-0025, keep SOW-0026 and SOW-0027 pending, create
  aggregator SOW and UI TODO, and map all remaining follow-ups before close.

Open-source reference evidence:

- Not checked for this gate. This contract is Netdata-specific and is defined
  by existing Agent, Cloud frontend, and Cloud topology service behavior.

Open decisions:

- No user decision is currently blocking the specification. The user already
  selected the key product direction: spec first, then durable repo work items,
  then implementation.

## Implications And Decisions

### Decision 1: Treat Actor Identification As Part Of This Contract

Selection: implement in SOW-0028.

Reasoning:

- Actor identification affects schema, producer payloads, UI rendering, and
  aggregator preservation.
- Keeping it separate would leave actor modals visually incomplete even if mode
  and correlation semantics are fixed.

### Decision 2: Aggregator Consumes Detailed Input

Selection: Cloud aggregator rewrites `__topology_mode=aggregated` fanout to
`__topology_mode=detailed` only for producers that support the mode.

Reasoning:

- Exact cross-node socket correlation needs detailed tuples.
- Aggregating before correlation loses facts that cannot be recovered.
- SNMP/L2 and streaming currently do not have meaningful detailed/aggregated
  producer modes, so the aggregator should not send an invented parameter.

### Decision 3: Network-Connections Detailed Uses Loose Sides

Selection: known actors remain actors; unknown remote socket peers are
loose-side facts until the aggregator or UI materializes them according to the
schema.

Reasoning:

- Actor-per-`IP:PORT` detailed output can explode graph cardinality.
- Exact tuples are still preserved for matching and drilldown.

### Decision 4: SNMP Uses Replacement, Streaming Uses Enrichment

Selection:

- SNMP/L2 aggregation replaces weaker placeholder actors with stronger managed
  actors.
- Streaming aggregation merges/enriches actors by `machine_guid` and table
  policy.

Reasoning:

- SNMP placeholder actors and managed devices represent the same physical
  entity at different confidence levels.
- Streaming payloads from multiple parents may contain complementary facts for
  the same node; losing either side is wrong.

## Cross-Repo Pending Checklist

Agent repository:

- Add/maintain the spec under `.agents/sow/specs/`.
- Update `FUNCTION_TOPOLOGY_SCHEMA.json`.
- Update `FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- Update `.agents/sow/specs/topology-function-schema.md` as needed.
- Update `.agents/skills/project-create-topology/SKILL.md`.
- Implement producer changes for network-connections, SNMP/L2, and streaming
  as far as the Agent owns the emitted payload.

Cloud frontend repository:

- Create `TODO-topology-mode-correlation-aggregation.md`.
- Implement modal label identification rendering.
- Implement mode capability behavior and hide no-op mode toggles.
- Preserve and render loose-side/materialization metadata without
  domain-specific guesses.
- Keep old-schema support isolated.

Cloud topology service repository:

- Create a new current SOW for topology mode, loose-side resolution,
  replacement, enrichment, and table merge policy.
- Implement request fanout mode rewrite.
- Implement generic rule classes and table merge policies.
- Add fixtures/tests for network-connections, SNMP/L2, and streaming.

## Plan

1. Write the spec and create cross-repo work items.
2. Patch Agent schema/docs/skill.
3. Patch Agent producers and validate local payloads.
4. Patch Cloud frontend and run focused tests.
5. Patch Cloud topology service and run Go tests.
6. Update SOW validation and follow-up mapping.

## Execution Log

### 2026-05-11

- Created the cross-topology mode/correlation/aggregation spec.
- Paused SOW-0025 because it depends on this broader contract.
- Created Cloud frontend handoff TODO:
  `<cloud-frontend-repo>/TODO-topology-mode-correlation-aggregation.md`.
- Created and completed Cloud topology service SOW:
  `<cloud-topology-service-repo>/.agents/sow/done/SOW-0011-20260511-topology-mode-correlation-aggregation.md`.
- Updated Agent schema, developer guide, project topology skill, Go topology
  structs/validation, network-connections producer, SNMP topology tests, and
  streaming actor modal identification.
- Implemented UI support for modal label identification and hiding
  `__topology_mode` controls when `data.view.supported_modes` does not
  advertise a real split.
- Implemented Cloud topology service compatibility for supported modes, modal
  label identification, correlation rule classes, fanout mode rewrite helpers,
  and detail-table dedupe/merge policies.

## Validation

Acceptance criteria evidence:

- Spec:
  - `.agents/sow/specs/topology-modes-correlation-aggregation.md` defines the
    layer responsibilities and examples for network-connections, SNMP/L2, and
    streaming.
- Agent schema/docs:
  - `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` includes
    `data.view.supported_modes`, modal label identification, port source
    `value_column`, correlation rule `class`, and expanded table aggregation
    tokens.
  - `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` documents the new
    fields.
  - `.agents/skills/project-create-topology/SKILL.md` was updated for the
    developer workflow.
- Agent producers:
  - `src/collectors/network-viewer.plugin/network-viewer.c` accepts
    `__topology_mode`, advertises supported modes, emits selected actor modal
    identification labels, emits relationship-summary rows for aggregated
    connections, and marks socket correlation rules with
    `class: resolve_loose_side`.
  - `src/web/api/functions/function-topology-streaming.c` emits selected actor
    modal identification labels and remains mode-invariant.
  - `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go` remains
    mode-invariant and is covered by updated tests.
- Cloud frontend:
  - `src/domains/functions/topology/v1/buildModalPresentation.js` decodes
    modal label identification.
  - `src/domains/functions/components/topology/actorModal/index.js` renders
    selected v1 identification labels in the actor modal header.
  - `src/domains/functions/useFetch/normalizers/topology/index.js` hides
    `__topology_mode` unless `supported_modes` advertises both modes.
- Cloud topology service:
  - `internal/topology/schema/payload.go`, `internal/topology/validate/validate.go`,
    `internal/topology/mode/mode.go`, and
    `internal/topology/aggregate/aggregate.go` implement the service-side
    compatibility layer.

Tests or equivalent validation:

- Agent:
  - `git diff --check` passed.
  - `go test ./pkg/topology/v1 ./plugin/go.d/collector/snmp_topology` passed
    from `src/go`.
  - `sudo -n cmake --build build --target network-viewer.plugin -- -j2`
    passed after non-sudo build was blocked by existing build-directory
    permissions.
  - `.agents/sow/audit.sh` passed with existing non-project-skill
    classification warnings.
- UI:
  - `git diff --check` passed.
  - `yarn test src/domains/functions/topology/v1/buildModalPresentation.test.js src/domains/functions/useFetch/normalizers/topology/index.test.js --runInBand`
    passed.
- Cloud topology service:
  - `go test ./internal/topology/...` passed.
  - `go test ./...` passed.
  - `go vet ./internal/topology/...` passed.
  - `git diff --check` passed.
  - `.agents/sow/audit.sh` passed.

Real-use evidence:

- Installed Function checks through token-safe Cloud-proxied Agent calls passed
  on 2026-05-11:
  - `topology:network-connections ... mode:aggregated` returned status 200,
    `view.mode: aggregated`, `view.supported_modes: [aggregated, detailed]`,
    actor rows, link rows, `actor_labels`, `socket_ports`, relationship table
    `connections`, and `data.correlation`.
  - `topology:network-connections ... mode:detailed` returned status 200,
    `view.mode: detailed`, the same supported modes, graph rows, socket
    evidence rows, and `data.correlation`.
  - `topology:streaming` returned status 200, no `supported_modes`, and modal
    identification metadata for its actor types.
  - `topology:snmp` returned status 200 and no `supported_modes`, so the UI
    must not show a fake detailed/aggregated toggle. SNMP-specific modal
    identification polish remains tracked by SOW-0026.

Reviewer findings:

- No external reviewer loop was run for this SOW pass. The user asked for
  implementation continuity after the spec/SOW/TODO artifacts were created.

Same-failure scan:

- `rg` checks were used across Agent, UI, and Cloud topology service for the
  new contract fields: `supported_modes`, `identification`, `class`,
  `value_column`, `merge_metrics`, `set_union`, `__topology_mode`, and
  `RewriteFunctionCall`.

Sensitive data gate:

- Passed by inspection for this pass. Durable artifacts contain synthetic
  examples only; no raw Function captures, bearer tokens, cookies, usernames,
  command lines from local systems, SNMP communities, customer names,
  customer-identifying public endpoints, private endpoints, raw node IDs, or
  raw machine GUIDs were written.

Artifact maintenance gate:

- AGENTS.md: unchanged; existing SOW, public-skill boundary, and artifact
  maintenance rules already cover this workflow.
- Runtime project skills: updated
  `.agents/skills/project-create-topology/SKILL.md`.
- Specs: added
  `.agents/sow/specs/topology-modes-correlation-aggregation.md` and updated
  `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: unaffected; this is an internal developer topology
  schema/producer/service/UI contract.
- End-user/operator skills: unaffected; no operator querying skill was changed.
- SOW lifecycle: SOW-0025 remains paused; SOW-0026 and SOW-0027 remain pending;
  SOW-0029 tracks true detailed loose-side graph rows; SOW-0028 is completed
  and moved to `done/`.

Specs update:

- Added `.agents/sow/specs/topology-modes-correlation-aggregation.md`.

Project skills update:

- Updated `.agents/skills/project-create-topology/SKILL.md`.

End-user/operator docs update:

- Not affected.

End-user/operator skills update:

- Not affected.

Lessons and follow-up mapping:

- Completed in the final Lessons Extracted and Followup sections below.

## Outcome

Implementation checkpoint is complete across Agent, Cloud frontend, and Cloud
topology service for the documented schema compatibility layer:

- schema/docs/specs/skill were updated first;
- Agent producers and topology validation were updated;
- UI modal identification and mode-control behavior were updated;
- Cloud topology service schema/validation/mode/table-policy compatibility was
  updated;
- focused validation passed in all three repositories.

The SOW is complete for the implemented cross-repo compatibility contract.
Remaining one-sided detailed network-connections graph rows are explicitly
split to SOW-0029.

## Lessons Extracted

- The durable spec/SOW/TODO order worked: after compaction, the repo artifacts
  were enough to recover the intended cross-repo work without relying on chat
  memory.
- Loose-side rows are the hardest part of the model because they affect schema
  shape, UI graph materialization, and aggregation execution together. The
  current checkpoint preserves exact socket facts and correlation metadata but
  does not yet switch the Agent producer to one-sided detailed graph rows.

## Followup

- True detailed one-sided network-connections graph rows and UI
  materialization are tracked by
  `.agents/sow/pending/SOW-0029-20260511-network-connections-detailed-loose-sides.md`.
  The current Agent producer still emits endpoint actors for local direct
  views while preserving exact socket evidence for correlation.
- Cloud topology service production fanout/fetch integration remains tracked by
  its service handoff gates; SOW-0028 implemented the internal mode rewrite
  helper and aggregation compatibility layer.
- SOW-0025, SOW-0026, and SOW-0027 remain the function-specific modal product
  polish work for network-connections, SNMP/L2, and streaming.

## Regression Log

None yet.
