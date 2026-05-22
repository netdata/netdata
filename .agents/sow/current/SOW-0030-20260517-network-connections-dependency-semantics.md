# SOW-0030 - Network Connections Dependency Semantics

## Status

Status: paused

Sub-state: selected by the user on 2026-05-17; implementation starting after
pausing unrelated streaming modal validation work. Paused on 2026-05-17 while
the user requested a streaming graph presentation regression fix under
SOW-0027.

## Requirements

### Purpose

Make `topology:network-connections` show real service dependency direction for
sysadmins, DevOps engineers, and SREs: dependency arrows must point from client
to server, actor modals must separate dependencies from dependants, and
aggregated views must collapse remote endpoint noise without leaking
observer-relative socket terminology.

### User Request

The user asked for these network-connections-only topology changes:

- Direction is important and must be painted with an arrow.
- Actor modals must split socket tables into `Dependencies` and `Dependants`.
- Aggregated backend views should collapse inbound connections by remote IP and
  outbound connections by remote IP.
- `local` / `remote` names are wrong for topology and must be removed from the
  topology contract.
- Prefer `client` / `server`; if `src` / `dst` remains, then `src` must always
  mean client and `dst` must always mean server.
- The topology `local` socket filter should be removed.
- Changes must not affect other topology producers.

### Assistant Understanding

Facts:

- The socket scanner already distinguishes local inbound and local outbound
  enum values before the network-viewer producer serializes them.
- The current topology payload uses observer-relative `local_*` and `remote_*`
  columns in socket evidence and relationship-summary rows.
- The current topology graph link `src_actor` is the process that owns the local
  socket row, not necessarily the client actor.
- The current modal recipe has one `Connections`/`Sockets` section for process
  actors instead of separate dependencies and dependants.
- The v1 schema already supports directed link types, actor-column owner
  filters, formatted endpoint projections, and arbitrary typed relationship
  table columns.

Inferences:

- No generic JSON Schema mechanism is needed. This is a network-connections
  producer contract change.
- Correlation can remain generic if the producer emits correlation claims for
  client/server endpoint ownership and correlation points for visible endpoint
  actors using the existing declarative correlation tables.
- Aggregated mode can collapse relationship rows by client actor, server actor,
  protocol, and state. The endpoint actor identity carries the remote IP
  grouping, while detailed evidence preserves exact client/server ports.

Unknowns:

- Live UI polish may still need separate cloud-frontend changes after this
  payload changes, but the schema already has enough modal primitives for the
  producer to declare the desired sections.

### Acceptance Criteria

- `topology:network-connections` type definitions declare socket link types as
  directed dependency links with forward arrows.
- Topology graph links use `src_actor = client_actor` and
  `dst_actor = server_actor` for network dependency links.
- Topology evidence and relationship-summary rows use `client_*` and
  `server_*` endpoint columns, not `local_*` / `remote_*`.
- Topology modal recipes expose `Dependencies` and `Dependants` sections for
  process actors using actor-column owner filters.
- The topology Function `info`/required params no longer expose a `local`
  socket filter. Local sockets are included through inbound/outbound
  dependency classification.
- Aggregated topology relationship rows collapse by client actor, server actor,
  protocol, and state, with endpoint actor identity carrying remote IP grouping
  instead of local ephemeral port grouping.
- Specs, developer guide, project topology skill, validation fixture, and
  focused validation tests are updated.
- The change is limited to `topology:network-connections` and does not alter
  SNMP, streaming, vSphere, or legacy non-topology schemas except for the shared
  direction string mapping that stops rendering local sockets as `local`.

## Analysis

Sources checked:

- `.agents/skills/project-create-topology/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/sow/done/SOW-0025-20260511-network-connections-modal-product-composition.md`
- `.agents/sow/done/SOW-0028-20260511-topology-mode-correlation-aggregation.md`
- `.agents/sow/pending/SOW-0029-20260511-network-connections-detailed-loose-sides.md`
- `.agents/sow/specs/topology-function-schema.md`
- `.agents/sow/specs/topology-modes-correlation-aggregation.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/libnetdata/local-sockets/local-sockets.h`
- `src/go/tools/functions-validation/fixtures/topology-v1/network-connections.json`
- `src/go/tools/functions-validation/validate/main_test.go`

Current state:

- Generic v1 link types already define `orientation`, `direction_role`, and
  aggregation direction policy.
- Network-connections currently emits `socket`, `endpoint_socket`, and
  `correlated_socket` with `direction_role: flow`.
- Network-connections currently emits observer-relative endpoint columns:
  `local_ip`, `local_port`, `remote_ip`, `remote_port`.
- The shared `SOCKET_DIRECTION_2str()` map collapses local inbound and local
  outbound into the string `local`.
- The topology required params expose a `local` socket filter.

Risks:

- Renaming endpoint columns is a contract-breaking change for any external UI
  or aggregator code that hardcoded network-connections table columns.
- Getting client/server actor selection wrong would invert dependency arrows,
  which is worse than no arrow.
- Aggregating by remote IP can hide exact port-level rows in aggregated mode;
  detailed mode must retain exact client/server tuples.
- Existing dirty streaming/spec changes must not be reverted or mixed into this
  SOW outcome.

## Pre-Implementation Gate

Status at implementation start: ready (historical snapshot; current SOW state
is recorded in the top-level Status section).

Problem / root-cause model:

- The current topology model exposes observer-relative socket facts as if they
  were topology semantics. This makes actor modals and aggregation confusing:
  `local` means "this socket row's local endpoint", not dependency direction.
- The socket scanner has enough information to derive dependency direction:
  inbound/local-inbound maps remote endpoint to client and local endpoint to
  server; outbound/local-outbound maps local endpoint to client and remote
  endpoint to server; listen has a server only.
- The producer must encode dependency direction directly through
  client/server columns and actor refs, so the UI and aggregator do not infer
  it from local/remote names.

Evidence reviewed:

- `src/libnetdata/local-sockets/local-sockets.h` has distinct
  `SOCKET_DIRECTION_LOCAL_INBOUND` and `SOCKET_DIRECTION_LOCAL_OUTBOUND` enum
  values and converts loopback/local peers after inbound/outbound detection.
- `src/collectors/network-viewer.plugin/network-viewer.c` maps both local enum
  values to the string `local`.
- `src/collectors/network-viewer.plugin/network-viewer.c` currently builds
  topology rows with `local_ip`, `remote_ip`, and row-owner `src_actor`.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` already supports directed link
  types, relationship tables, actor-column modal owner filters, and formatted
  endpoint projections.

Affected contracts and surfaces:

- Agent Function payload for `topology:network-connections`.
- Topology developer guide and specs.
- Project topology skill guidance.
- Function validation fixture and semantic tests.
- Potential external consumers of network-connections v1 columns.

Existing patterns to reuse:

- Existing compact table encoder helpers in `network-viewer.c`.
- Existing modal `actor_column` owner filters and `formatted_endpoint`
  projection.
- Existing type-level link presentation and link aggregation metadata.
- Existing correlation rows with declarative protocol/address-space/IP/port
  keys.

Risk and blast radius:

- Medium/high semantic risk inside network-connections only.
- Low schema risk because no generic schema mechanism is added.
- Medium compatibility risk for cloud-frontend and cloud-topology-service if
  they hardcoded old network-connections column names despite the v1 contract.
- Performance risk is bounded by aggregating rows more aggressively in
  aggregated mode.

Sensitive data handling plan:

- Use synthetic examples in SOW/spec/docs.
- Do not commit raw live Function payloads, process command lines, private
  endpoints, public customer-identifying endpoints, tokens, cookies, machine
  GUIDs, or node IDs.
- Store any live captures under `.local/` only if needed.

Implementation plan:

1. Update the network-viewer topology producer internal row shape with derived
   client/server endpoints and dependency actor selection.
2. Change topology output columns, match columns, correlation rows, and modal
   recipes to use client/server semantics.
3. Remove the topology `local` socket filter while still collecting local
   sockets through inbound/outbound classification.
4. Update specs, developer guide, project skill, fixtures, and focused tests.
5. Validate with schema checks, semantic tests, and a network-viewer compile
   check or build.

Validation plan:

- `git diff --check`
- `go test ./tools/functions-validation/validate`
- Network-viewer plugin build target if the local build tree is available, or
  the equivalent compile command from `compile_commands.json` with output
  redirected to `/tmp`.
- Fixture/schema checks proving network-connections uses client/server columns
  and no topology `local` filter.
- Same-failure search for remaining network-connections topology uses of
  `local_ip`/`remote_ip` where they would leak topology semantics.

Artifact impact plan:

- AGENTS.md: not expected; workflow rules do not change.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`.
- Specs: update `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-modes-correlation-aggregation.md`.
- End-user/operator docs: network-connections integration metadata may mention
  `local`; update only if the user-facing non-topology Function output changes.
- End-user/operator skills: not expected.
- SOW lifecycle: create SOW-0030 as current/in-progress and pause SOW-0027.

Open-source reference evidence:

- Not checked. This is an internal topology payload contract correction, not an
  external protocol interpretation.

Open decisions:

- Resolved by user: use client/server dependency semantics, remove local/remote
  from network-connections topology, remove local topology filter, and keep the
  change scoped to network-connections.

## Implications And Decisions

1. User decision: `local` / `remote` are not meaningful topology concepts and
   must be removed from the network-connections topology contract.
2. User decision: prefer `client` / `server`; if `src` / `dst` remains, then
   `src = client` and `dst = server` in all cases.
3. User decision: graph arrows must show dependency direction.
4. User decision: process actor modals must split dependency rows into
   `Dependencies` and `Dependants`.
5. User decision: topology `local` socket filter must be removed.

## Plan

1. Patch network-viewer topology row model and option parsing.
2. Patch network-viewer topology table schemas, values, modals, and correlation.
3. Update docs/specs/project skill/fixture/tests.
4. Validate narrow commands and record results.

## Execution Log

### 2026-05-17

- Created SOW and paused SOW-0027 before implementation.
- Changed network-connections topology row semantics from observer-relative
  `local_*` / `remote_*` columns to dependency-oriented `client_*` /
  `server_*` columns.
- Reoriented socket graph links so `src_actor` is the client/dependant and
  `dst_actor` is the server/dependency target.
- Removed the topology `local` socket selector from option parsing and Function
  metadata; local sockets are collected under inbound/outbound classification.
- Split non-node network-connections modal recipes into `Dependencies` and
  `Dependants`.
- Updated the network-connections topology validation fixture, developer guide,
  specs, project topology skill, and network-viewer integration text.

## Validation

Acceptance criteria evidence:

- `src/collectors/network-viewer.plugin/network-viewer.c` emits
  `direction_role: "dependency"` for `socket`, `endpoint_socket`, and
  `correlated_socket`.
- `src/collectors/network-viewer.plugin/network-viewer.c` resolves dependency
  actors as client/server and writes `client_*` / `server_*` columns for
  relationship and evidence tables.
- `src/collectors/network-viewer.plugin/network-viewer.c` no longer exposes a
  topology `local` socket option and maps local inbound/outbound socket enums
  to `inbound` / `outbound`.
- `src/go/tools/functions-validation/fixtures/topology-v1/network-connections.json`
  validates the updated client/server table contract.

Tests or equivalent validation:

- `cmd=$(jq -r '.[] | select(.file|endswith("src/collectors/network-viewer.plugin/network-viewer.c")) | .command' build/compile_commands.json | sed 's# -o [^ ]*# -o /tmp/network-viewer.c.o#'); eval "$cmd"` passed.
- `(cd src/go && go test -count=1 ./tools/functions-validation/validate)` passed.
- `git diff --check` passed.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py --collector network-viewer.plugin/network-viewer.plugin` passed.

Real-use evidence:

- Pending user install/run validation. The local `build/` directory is owned by
  `root:root`, so `cmake --build build --target network-viewer.plugin` could
  not write `.ninja_lock`; compile validation used the same compile command
  from `build/compile_commands.json` with object output redirected to `/tmp`.

Reviewer findings:

- No external assistant review requested for this SOW.

Same-failure scan:

- Searched network-connections topology docs/fixtures/skill for stale
  `local_ip`, `local_port`, `remote_ip`, `remote_port`, and `sockets_local`
  contract references. Remaining matches are generic loose-side/correlation
  terminology or non-topology local-IP actor labels.

Sensitive data gate:

- No raw live payloads, cookies, tokens, machine GUIDs, or private endpoints
  were written. Fixture and docs use documentation-reserved example IP ranges.

Artifact maintenance gate:

- AGENTS.md: not expected.
- Runtime project skills: updated `.agents/skills/project-create-topology/SKILL.md`.
- Specs: updated topology schema/mode specs.
- End-user/operator docs: updated `metadata.yaml` and generated integration
  markdown to remove the `local` direction wording.
- End-user/operator skills: not expected.
- SOW lifecycle: SOW-0030 current/in-progress; SOW-0027 paused.

Specs update:

- Updated `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-modes-correlation-aggregation.md`.

Project skills update:

- Updated `.agents/skills/project-create-topology/SKILL.md`.

End-user/operator docs update:

- Updated `src/collectors/network-viewer.plugin/metadata.yaml` and
  `src/collectors/network-viewer.plugin/integrations/network_connections.md`.

End-user/operator skills update:

- Not expected.

Lessons:

- The generic topology JSON Schema did not need a change; the contract change
  is a producer profile change using existing open table columns,
  actor-column modal filters, and formatted endpoint projections.

Follow-up mapping:

- UI and aggregator may still need compatibility updates if they hardcoded old
  network-connections `local_*` / `remote_*` columns; this SOW updates the Agent
  producer contract and fixture.
- SOW-0029 remains the pending tracker for any future detailed loose-side model.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
