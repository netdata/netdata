# SOW-0023 - Topology cross-payload matching

## Status

Status: completed

Sub-state: netdata-side schema, spec, docs, developer skill, validators, fixtures,
network-connections producer changes, and local frontend v1 rendering alignment
are implemented and validated. Cloud service aggregation remains out of scope
for the aggregated local UI fix because the Cloud aggregator receives detailed
payloads.

## Requirements

### Purpose

Define how Cloud reconciles actors and links that arrive from different topology producers and may describe the same real-world entity or relationship with different identity keys.

### User Request

The user identified that the Cloud aggregator must eventually know how to match across topology payloads:

- network-connections by socket endpoint identity such as IP, port, protocol, direction, and address-space context;
- SNMP/L2 by device, MAC, interface, port, chassis, and management identity;
- streaming by machine or node identity;
- vSphere by stable inventory object identity.

The user accepted splitting this structural problem out of SOW-0021 so SOW-0021 can focus on presentation.

### Assistant Understanding

Facts:

- `netdata.topology.v1` currently has per-actor-type `identity` and `merge_identity`.
- Evidence types have `match_columns`, but those preserve relationship detail inside one payload and do not by themselves define cross-producer actor replacement.
- Different topology producers use different identity vocabularies and may legitimately fail to correlate.
- If two observations do not correlate, both remain valid; this is not a factual contradiction.

Inferences:

- Cross-payload matching needs a shared identity vocabulary or strategy registry, normalization rules, ambiguity policy, confidence, and tests.
- This is structural graph reconciliation, not presentation.
- The schema may need additional producer declarations, but those should be designed in this SOW, not hidden in presentation profiles.

Unknowns:

- Exact shared identity vocabulary and normalization rules.
- Whether matching is pairwise between producer kinds or generic through typed identity facts.
- How Cloud should handle ambiguous matches, partial matches, and conflicting confidence.
- MVP scope is limited to the netdata-side generic contract plus
  network-connections producer emission. Cloud implementation details are
  tracked by the service worker SOW.
- Exact producer migration order after the schema contract lands.

### Acceptance Criteria

- Inventory identity and match evidence emitted by network-connections, SNMP/L2, streaming, and vSphere.
- Define a compact schema contract for cross-payload identity declarations if needed.
- Define Cloud aggregator matching strategies, normalization rules, ambiguity policy, and diagnostics.
- Define how endpoint actors are replaced, merged, or left separate.
- Create the Cloud service handoff SOW that requires fixtures for successful
  match, no match, ambiguous match, partial link match, and conflicting
  presentation/type definitions.
- Create the Cloud frontend handoff TODO for correlation rendering and link
  layout tokens.
- Implement the network-connections producer changes needed to emit semantic
  ownership/resolved/correlation link types and socket correlation rows.
- Update topology schema/spec/docs/skill if producer declarations are added.

## Analysis

Sources checked:

- `.agents/sow/current/SOW-0021-20260509-topology-presentation-contract.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`
- `src/web/api/functions/function-topology-streaming.c`
- Cloud topology service evidence recorded in SOW-0021.

Current state:

- SOW-0021 records that `merge_identity` is per actor type and does not define shared identity classes across producers.
- SOW-0021 records that evidence `match_columns` preserve exact relationship details but do not declare endpoint replacement across topology payloads.
- SOW-0021 records the user decision to split this into SOW-0023.

Risks:

- False positive matches could collapse unrelated actors.
- False negative matches could duplicate actors that represent the same entity.
- NAT, load balancers, address reuse, namespaces, and reused MAC/IP identities can make simple exact matching unsafe.
- Matching strategies can leak sensitive infrastructure identities if durable artifacts include raw examples.

## Pre-Implementation Gate (Historical Snapshot at Implementation Start)

Status: unblocked for netdata-side contract and network-connections producer
implementation. Cloud and frontend implementation are delegated through the
handoff artifacts created by this SOW.

Problem / root-cause model:

- The compact topology schema has enough identity to aggregate within producer-defined types, but it does not yet define how Cloud should reconcile actors across different producer domains.
- The schema also lacked generic link-layout force/distance tokens, causing dense or weak semantic relationships to use the same graph forces as strong relationships.
- Network-connections currently uses one graph link family for process-to-endpoint socket links, which prevents the UI and aggregator from distinguishing local resolved links from correlation links.

Evidence reviewed:

- SOW-0021 reviewer findings and user decision notes.
- Current topology schema identity and evidence match-column fields.
- Current topology schema link presentation fields: color, opacity, line style, width, curve, arrow, variable, and hover, with no layout strength/distance token.
- Current network-viewer socket evidence and link type shape.
- Current SNMP actor identity arrays for chassis, MAC, IP, and sys-name.
- Current streaming actor identity through machine GUID and node id.

Affected contracts and surfaces:

- `netdata.topology.v1` schema.
- Cloud topology service aggregation algorithm.
- Topology producers for network-connections, SNMP/L2, streaming, vSphere, and future topology domains.
- Cloud frontend behavior when merged actors replace endpoint actors.
- Cloud frontend force-layout behavior for dense, weak, ownership, and partial-correlation links.

Existing patterns to reuse:

- Actor `identity`, `merge_identity`, and `parent_identity`.
- Evidence `match_columns`.
- Link direction and aggregation semantics.
- Type-level presentation tokens and legend entries from SOW-0021.
- Compact tables for high-cardinality rows.

Risk and blast radius:

- High: incorrect matching can materially change topology meaning.
- High: Cloud aggregation behavior changes across all topology kinds.
- Medium: producer schemas may need new typed identity declarations.
- Medium: layout tokens influence graph readability but must not leak raw frontend physics into producer payloads.

Sensitive data handling plan:

- Do not copy raw IP addresses, MAC addresses, hostnames, machine GUIDs, node IDs, account IDs, customer identifiers, credentials, secrets, API tokens, bearer tokens, session cookies, SNMP communities, or private topology examples into this SOW, specs, docs, skills, code comments, commits, or PR text.
- Use sanitized synthetic fixtures and placeholder identifiers.
- Keep any raw captured payloads under `.local/` only.

Implementation plan:

1. Define the netdata-side schema contract for generic correlation rules, pure correlation actors, points, claims, actions, priorities, and output link types.
2. Define generic link layout tokens for all link types.
3. Update the Go topology v1 model and semantic validator.
4. Update the topology spec, developer guide, implementation scope, and
   developer `project-create-topology` skill.
5. Update fixtures so validators exercise correlation rules and link-layout tokens.
6. Implement network-connections link taxonomy and correlation rows.
7. Create Cloud aggregator and UI force-layout handoff artifacts for their
   owning workers.

Validation plan:

- Service-level fixtures for match, no match, ambiguous match, and unsafe match.
- Schema validation for any new identity declarations.
- Payload-size checks for added declarations.
- Same-failure search across topology producers.
- Netdata-side JSON Schema validation for correlation objects and link layout tokens.
- Go semantic validation for correlation rule references, point/claim table shape, rule key columns, and link-layout token parity.
- Go semantic validation must require only the key columns for rules actually
  referenced by each correlation table, not every column for every defined rule.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: likely unaffected unless topology workflow changes.
- Specs: update topology function schema spec.
- End-user/operator docs: update topology developer guide if producer contract changes.
- Runtime project skills: update `project-create-topology` when producer
  authoring workflow changes.
- End-user/operator skills: not affected unless public operator workflows
  change.
- JSON Schema: update `FUNCTION_TOPOLOGY_SCHEMA.json`.
- Go producer helper: update `src/go/pkg/topology/v1`.
- Function validation fixtures: update topology v1 fixtures.
- SOW lifecycle: this SOW tracks the SOW-0021 split decision.

Open-source reference evidence:

- No external open-source implementation was used as normative evidence for
  this schema contract. The contract is driven by Netdata producer semantics,
  existing topology payload requirements, and the Cloud aggregator handoff.

Open decisions:

- None for the Netdata-side SOW-0023 scope. Cross-kind identity policy remains
  tracked by SOW-0002, table/modal composition remains tracked by SOW-0022, and
  vSphere migration remains tracked by SOW-0024.

## Correlation Contract

The production schema must describe producer-visible correlation facts only. It
must not expose aggregator internals. The aggregator may maintain any internal
state needed, but the final emitted topology is a normal `netdata.topology.v1`
payload with actors, links, evidence, tables, overlays, presentation, stats, and
diagnostics as appropriate.

Correlation points:

- are pure topology actors;
- have normal actor types, presentation, labels, legend entries, and links;
- remain visible when unmatched, partial, or ambiguous;
- disappear only when an exact unambiguous absorb rule resolves them.

Correlation rules:

- are declared by producers under `data.correlation.rules`;
- are generic and topology-agnostic;
- build exact keys from declarative column/literal templates;
- have a `priority` so exact rules can run before broader/partial rules;
- have a `key_space` to avoid accidental matches between unrelated domains;
- have `action: absorb` or `action: link`;
- list pure `point_actor_types`;
- may list `claim_actor_types` that can satisfy a point;
- may list `correlation_link_types` that connect real actors to correlation actors;
- state an `output_link_type` for final rewritten or partial links.

Correlation tables:

- `data.correlation.points` contains actor refs for pure correlation actors plus
  rule id and key columns.
- `data.correlation.claims` contains actor refs for real actors plus rule id and
  key columns.
- A real actor can claim many keys without bloating the actor row itself.
- A correlation actor can have several point rows for aliases such as NAT-derived
  additional keys.

Actions:

- `absorb`: exact, unambiguous matches remove all matched correlation actors from
  the final aggregated output and rewire incident correlation links to the
  matched real actors using the rule's `output_link_type`.
- `link`: broader or partial matches keep the correlation actor visible and emit
  or preserve a weak semantic link to the matched actor using the rule's
  `output_link_type`.

No match leaves the correlation actor visible. Ambiguous matches must not be
guessed; they remain unresolved and the aggregator records diagnostics.

The aggregator must stay agnostic. It should not need releases to learn new
strategy names such as `ip_port`, `mac`, or `vsphere_moid`. It should build keys
from declared columns and literals, normalize by column type, respect priority
and key space, and apply the declared action.

## Link Layout Contract

Link layout is generic for all link types. It is not special to correlation.

Every link type may define:

- `types.link_types.<id>.presentation.layout.strength`
- `types.link_types.<id>.presentation.layout.distance`

Allowed strength tokens:

- `weakest`
- `weaker`
- `normal`
- `stronger`
- `strongest`

Allowed distance tokens:

- `closest`
- `closer`
- `normal`
- `farther`
- `farthest`

These are UI-owned relative tokens, not numeric force values. Producers use them
to classify relationship strength and preferred separation:

- ownership/containment: stronger + closer;
- resolved normal dependency/flow: normal + normal;
- local noise, dense mesh, inferred, or weak evidence: weaker/weakest +
  farther/farthest;
- partial or cross-topology correlation links: weaker/weakest + farther.

The legend must reflect visible semantic differences introduced by these link
types. The UI must not infer forces from topology kind or actor names.

## Network-Connections Required Shape

Network-connections should use three graph-link families:

1. Node-to-process ownership links that keep the graph clustered by node.
2. Local process-to-process links when both endpoints are already resolved.
3. Process-to-correlation-endpoint links for unresolved or cross-node remote
   socket endpoints.

Socket tuple interpretation:

- outbound: the process claims `protocol + local_ip + local_port`; the
  correlation endpoint points at `protocol + remote_ip + remote_port`;
- inbound: the process claims the local destination tuple; the correlation
  endpoint points at the remote source tuple;
- local: both process actors are already identified and should use a resolved
  local process-to-process link;
- listen: no remote correlation point exists.

Exact cross-node absorb example:

1. Node A emits `process-a -> correlation-endpoint(server-ip:server-port)`.
2. Node B emits `process-b` claiming `server-ip:server-port`.
3. Aggregation removes the matched correlation endpoint and rewires the link to
   `process-a -> process-b`.

Partial example:

1. Node A emits `process-a -> correlation-endpoint(protocol:ip:port)`.
2. Node B does not have a matching process claim, but emits a broader node/IP
   claim through a lower-priority `link` rule.
3. Aggregation keeps the correlation endpoint visible and links it to Node B:
   `process-a -> correlation-endpoint(protocol:ip:port) -> node-b`.

## Concrete Change Inventory

Netdata repository changes required by this SOW:

- Extend `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` with:
  - `data.correlation`;
  - `correlation.rules`;
  - compact `correlation.points` and `correlation.claims` tables;
  - `correlation_rule.action`, `priority`, `key_space`, `key`,
    `point_actor_types`, `claim_actor_types`, `correlation_link_types`, and
    `output_link_type`;
  - declarative `correlation_key_part` column/literal templates;
  - `link_type.presentation.layout.strength` and `.distance` tokens.
- Extend `src/go/pkg/topology/v1` structs and semantic validation with:
  - correlation data types;
  - point/claim table validation;
  - rule reference validation against actor/link types;
  - rule key column validation;
  - link layout token validation and schema-token parity tests.
- Update fixtures under `src/go/tools/functions-validation/fixtures/topology-v1/`
  with at least one correlation rule and link layout tokens.
- Update `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- Update `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`.
- Update `.agents/sow/specs/topology-function-schema.md`.
- Update `.agents/skills/project-create-topology/SKILL.md` with topology
  correlation and layout authoring guidance.

Producer changes implemented or scoped after the contract:

- Network-viewer:
  - split link types into ownership, local/resolved socket, and correlation
    socket links;
  - emit correlation actor rows for remote endpoints;
  - emit correlation point rows for unresolved remote endpoint keys;
  - emit process claim rows for locally owned socket keys;
  - emit legend entries for ownership, resolved socket, and correlation socket;
  - assign layout tokens to each link type.
- Streaming:
  - likely no correlation actors for the current topology; verify whether
    machine/node identity claims should be emitted for future cross-kind
    resolution.
- SNMP/L2:
  - preserve semantic link types; evaluate whether inferred endpoints should
    emit MAC/IP/chassis correlation points or claims.
- vSphere:
  - remains tracked by SOW-0024; when migrated, stable object identity can be
    expressed as claims or normal actor identity depending on topology scope.

External repository handoffs required:

- Cloud topology service:
  - implement rule parsing and generic key building;
  - apply absorb/link actions;
  - keep unmatched/ambiguous correlation actors visible;
  - add match/no-match/partial/ambiguous fixtures;
  - emit diagnostics without exposing internal states in the output graph.
- Cloud frontend:
  - honor link layout strength/distance tokens for all link types;
  - show semantic correlation actor/link legend entries;
  - avoid topology-name-specific force/layout hardcoding.

## Implications And Decisions

1. User decision from SOW-0021: cross-payload actor reconciliation should be solved, but it can be split into SOW-0023.
2. User decision from SOW-0021: execution order is SOW-0021, then SOW-0023, then SOW-0022.
3. User decision from 2026-05-10: correlation points are pure topology actors, not flags on real actors. Exact resolved correlation may remove correlation actors from the aggregated output, but internal aggregator states such as absorbed/candidate/rewrite-plan must not be exposed in the production schema or final UI payload.
4. User decision from 2026-05-10: the schema should define the producer-visible correlation contract for independently produced topology maps of the same kind. The aggregator may choose any internal indexing, matching, rewrite, and diagnostic implementation as long as the final output is a normal topology payload.
5. User decision from 2026-05-10: visible partial or cross-topology correlation links should be weaker in the force layout so related topology clusters stay readable instead of blending into one dense actor soup.
6. User decision from 2026-05-10: link layout force classification must be generic for all link types, not special-cased to correlation links. It should use five-step token scales with `normal` in the middle: strength `weakest`, `weaker`, `normal`, `stronger`, `strongest`; distance `closest`, `closer`, `normal`, `farther`, `farthest`.
7. User decision from 2026-05-10: links between a real actor and a correlation actor must be marked with a distinct semantic link type even in a single unaggregated topology. The UI and aggregator must not infer correlation-link behavior from actor names or topology kind.
8. User decision from 2026-05-10: network-connections graph links should be split into three semantic families: node-to-process ownership links that keep the graph together; local process-to-process links for already resolved local sockets; and process-to-correlation-endpoint links for unresolved/correlatable remote socket endpoints.

## Plan

1. Complete netdata-side contract artifacts: schema, Go types/validator, docs,
   spec, skill, and fixtures.
2. Validate the contract with JSON Schema, Go tests, function-validation
   fixtures, and SOW audit.
3. Implement network-viewer producer migration in SOW-0023.
4. Create or update Cloud service and Cloud frontend handoff documents after the
   contract is validated.
5. Implement Cloud aggregator and UI behavior in their owning repositories.

## Execution Log

### 2026-05-09

- Created as follow-up from SOW-0021 decision discussion.

### 2026-05-10

- Recorded user decisions that correlation points are pure topology actors, that
  aggregator internals must not leak into the schema or final UI payload, and
  that the schema should describe producer-visible correlation rules only.
- Recorded user decisions for generic declarative correlation keys, priorities,
  `absorb` and `link` actions, exact and partial correlation behavior, NAT/alias
  enrichment through additional keys, semantic correlation link types, and
  five-step link layout strength/distance tokens.
- Added netdata-side schema/Go/docs/spec/skill work items to this SOW.
- Implemented the network-viewer producer split into `ownership`, `socket`,
  `endpoint_socket`, and `correlated_socket` link types.
- Implemented network-viewer `data.correlation.rules`, `points`, and `claims`
  emission for socket tuple correlation.
- Created Cloud frontend and Cloud topology service handoff artifacts so their
  owning workers can port the correlation and layout contract.

## Validation

Acceptance criteria evidence:

- Identity/evidence inventory is recorded in `## Correlation Contract`,
  `## Network-Connections Required Shape`, and `## Concrete Change Inventory`.
- Netdata-side schema contract now defines generic correlation rules, compact
  points/claims tables, declarative key parts, absorb/link actions, priorities,
  point/claim actor types, correlation link types, output link type, and
  five-step link layout strength/distance tokens.
- Cloud aggregator and Cloud frontend implementation requirements are scoped in
  `## Concrete Change Inventory` and handed off to their owning workers.
- Network-connections now emits semantic ownership/resolved/correlation link
  types, correlation endpoint actors, `data.correlation.points`, and
  `data.correlation.claims`.

Tests or equivalent validation:

- `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- `jq empty src/go/tools/functions-validation/fixtures/topology-v1/network-connections.json` passed.
- `go test ./pkg/topology/v1 ./tools/functions-validation/validate` passed from `src/go`.
- Function validator passed for every fixture under
  `src/go/tools/functions-validation/fixtures/topology-v1/*.json`.
- C syntax-only validation for
  `src/collectors/network-viewer.plugin/network-viewer.c` passed using the
  local `build/compile_commands.json` command with `-fsyntax-only`.
- `ninja -C build network-viewer.plugin` could not run because the local build
  directory is root-owned and Ninja cannot create `.ninja_lock` or update the
  build log.
- `git diff --check` passed.
- `.agents/sow/audit.sh` exited successfully. It still reports the repository's
  existing non-project skill classification warning, which is outside this SOW;
  `project-create-topology` is classified as a runtime project skill.

Real-use evidence:

- A local bearer-protected Agent was queried through the token-safe direct-agent
  wrapper. The aggregated network-connections topology response returned
  `netdata.topology.v1` with 108 actors, 144 links, 96 correlation points, 179
  correlation claims, and 45 ownership links.
- The live payload defined `endpoint_socket` as `strength: weakest` and
  `distance: normal`, `correlated_socket` as `strength: weakest` and
  `distance: farthest`, `socket` as `strength: stronger` and `distance:
  farther`, and `ownership` as dotted/faded normal-distance graph-coherence
  links.
- The same live payload confirmed the `socket_exact` rule consumes
  `endpoint_socket` and emits `correlated_socket`, matching the intended
  single-node versus aggregated-layout split.
- Full `ninja -C build network-viewer.plugin` validation remains blocked by the
  root-owned local build directory, but schema validation, Go tests, semantic
  fixture validation, C syntax validation, and live Function output validation
  all passed.

Reviewer findings:

- No external read-only reviewer was run for this close. User live testing found
  the remaining force-layout stretch for high-fanout endpoint leaves; analysis
  traced that to Cloud frontend physics, not to the Netdata producer contract,
  and it is mapped to the frontend polishing work.

Same-failure scan:

- Current same-failure class is schema ambiguity rather than a runtime crash.
  The netdata-side validator now rejects missing correlation key columns,
  unknown rule references, unknown actor/link type references, and invalid link
  layout tokens.
- The validator also covers the multi-rule case where a table references only
  one of several defined rules, so unrelated rule key columns are not forced
  into every correlation table.

Sensitive data gate:

- This SOW currently contains only sanitized generic examples.

Artifact maintenance gate:

- AGENTS.md: updated the project skills index so topology authoring guidance is
  a runtime project skill, not a public operator skill.
- Runtime project skills: `.agents/skills/project-create-topology/SKILL.md`
  added as the developer-facing topology authoring workflow.
- Specs: `.agents/sow/specs/topology-function-schema.md` updated with
  correlation and link-layout contracts.
- End-user/operator docs: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`,
  `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`, and
  `src/plugins.d/FUNCTION_UI_REFERENCE.md` updated.
- End-user/operator skills: `query-netdata-agents` and `query-netdata-cloud`
  were audited for developer-contract leakage. The misplaced public
  `create-topology` developer skill was removed from the public skill tree, and
  public query skills now state that developer validation recipes belong in
  project skills.
- SOW lifecycle: SOW moved from pending to current for implementation and is
  marked `completed`; it is moved to `.agents/sow/done/` in the same commit as
  the implementation.

Specs update:

- `.agents/sow/specs/topology-function-schema.md` updated.

Project skills update:

- Added `.agents/skills/project-create-topology/SKILL.md`.
- Moved topology developer how-tos from the public skill tree into
  `.agents/skills/project-create-topology/how-tos/`.
- Added a developer how-to for verifying local network-connections layout
  tokens and correlation rule wiring with token-safe direct-agent wrappers.

End-user/operator docs update:

- Updated topology developer guide, topology implementation scope, and Function
  UI reference.

End-user/operator skills update:

- Removed the developer-facing `docs/netdata-ai/skills/create-topology/` skill
  and its `.agents/skills/create-topology` symlink.
- Removed topology producer authoring links from public query-topology guides.
- Removed the collector-author implementation section from the public Cloud
  query-functions guide.
- Moved skill-verification seed question lists out of public skill directories
  into `.agents/skill-verification/` and updated the pending verification
  harness SOW.
- Updated public query skills to keep future how-tos operator-facing.

Lessons:

- Correlation actors and correlation rules need a contract separate from actor
  identity. Actor `merge_identity` is not enough when one real actor owns many
  correlation keys.
- Link force/layout must be type-level and generic. Correlation links exposed
  this need, but dense local meshes and ownership links need it too.

Follow-up mapping:

- Cloud service implementation is handed off through the service SOW created by
  this SOW.
- Cloud frontend force-layout and correlation rendering implementation is handed
  off through the frontend TODO created by this SOW.
- Cross-kind identity policy remains tracked by SOW-0002.
- Table/modal composition remains tracked by SOW-0022.
- vSphere migration remains tracked by SOW-0024.

## Outcome

Completed. Netdata now has a generic producer-visible topology correlation
contract, generic link layout strength/distance tokens, semantic validation for
the new contract, updated schema/spec/docs/developer skill artifacts, and a
network-connections producer that emits ownership, local socket, unresolved
endpoint socket, and post-correlation socket semantics separately.

The local Agent runtime payload matches the intended split. Remaining visual
layout stretch around high-fanout endpoint leaves is a Cloud frontend physics
polish issue, not a backend payload issue.

## 2026-05-10 Aggregated Network-Connections UI Alignment

User-visible issue:

- Aggregated network-connections process actors lost resize behavior and port
  bullets because the producer only enabled process bullets in detailed mode.
- Endpoint links were rendered as dotted/secondary links even though they are
  the main unresolved network dependencies in a single-node view.
- Node-to-process links rendered too prominently even though they only keep the
  graph coherent and do not represent network traffic.

Root cause:

- Aggregated mode intentionally omits detailed socket evidence, but process port
  bullets were still defined only from socket evidence rows.
- The UI treated every bullet source row as one visible bullet and had no
  `value_column` to say that one compact row represents multiple sockets.
- Process sizing still relied on graph degree instead of the producer's
  `socket_count` metric.

Implemented contract and producer changes:

- Added optional numeric `ports.sources[].value_column` to the schema, Go model,
  semantic validator, spec, and developer guidance.
- Network-connections now emits an actor-owned `socket_ports` inventory table
  with `actor`, `port`, `protocol`, `direction`, and `socket_count`.
- Process actor presentation now uses
  `size: {"mode": "metric", "metric_column": "socket_count"}` and
  `ports.sources[]` from `socket_ports` with `value_column: "socket_count"`.
- Link presentation was aligned with the intended semantics:
  `endpoint_socket` is solid/colored/thin/weakest/normal-distance,
  `correlated_socket` is solid/colored/thin/weakest/farthest,
  `socket` is gray/thin/stronger/farther and variable by `socket_count`, and
  `ownership` is dotted/faded/dim/thin/normal/normal.

Implemented frontend changes:

- The v1 port-bullet decoder preserves `value_column` and sums duplicate bullet
  keys.
- The v1 renderer adapter passes `size_metric_column` to the graph renderer.
- The graph renderer uses actor metric sizing, weighted port capacity, weighted
  visible bullet count, and correct overflow without expanding unbounded data.
- v1-derived socket bullets default to active topology bullets so process rings
  are visible even when there is no legacy SNMP-style port status.

Validation added or rerun:

- `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- `jq empty src/go/tools/functions-validation/fixtures/topology-v1/network-connections.json` passed.
- `go test ./pkg/topology/v1 ./tools/functions-validation/validate` passed from
  `src/go`.
- Function validator passed for
  `src/go/tools/functions-validation/fixtures/topology-v1/network-connections.json`.
- C syntax-only validation passed for
  `src/collectors/network-viewer.plugin/network-viewer.c` using
  `-Ibuild -include build/config.h`.
- Cloud frontend targeted Jest tests passed:
  `portBullets.test.js`, `buildRenderPresentation.test.js`,
  `portUtils.test.js`, and `useForceSimulation.test.js`.

## 2026-05-10 Endpoint vs Correlated Socket Layout Split

User-visible issue:

- Unresolved endpoint links were set to `distance: farthest`, which makes
  single-node network-connections maps zoom out too much and shrink the useful
  process cluster.
- The same link type was also planned as the aggregator's consumed correlation
  link, where `farthest` is appropriate after cross-payload absorption because
  it keeps independent topology clusters from blending.

Implemented changes:

- Split the overloaded network-connections link semantics:
  `endpoint_socket` is the visible process-to-endpoint unresolved link, while
  `correlated_socket` is the aggregator output link after exact absorption.
- `endpoint_socket` uses solid/colored/thin, `strength: weakest`,
  `distance: normal`.
- `correlated_socket` uses solid/colored/thin, `strength: weakest`,
  `distance: farthest`, and can vary by `socket_count`.
- The `socket_exact` rule now lists `correlation_link_types:
  ["endpoint_socket"]` and `output_link_type: "correlated_socket"`.
- Fixtures, developer docs, and the topology developer skill were updated to
  use the split names.

## Lessons Extracted

- Pure correlation actors keep the producer contract simple: the aggregator can
  absorb, link, or leave them visible without exposing internal matching state.
- Link layout tokens are necessary but not sufficient for final graph polish:
  force-directed renderers may still need frontend-side handling for high-fanout
  leaves, collision radius, zoom-to-fit, and initial layout seeding.
- Aggregated process port bullets need an actor-owned inventory table with a
  numeric value column. Counting compact rows is wrong when one row represents
  many sockets.
- Distinguishing unresolved endpoint links from aggregated correlated links is
  required. Single-node endpoint leaves should not use the same farthest layout
  token as cross-payload correlation output.

## Followup Mapping

- Cloud topology service generic rule-based correlation is handed off through
  the Cloud service worker SOW.
- Cloud frontend link layout token handling and correlation rendering are handed
  off through the Cloud frontend worker TODO.
- The high-fanout endpoint leaf layout stretch observed during live testing is
  mapped to the Cloud frontend polishing work; the live Netdata payload already
  emits `endpoint_socket` as normal-distance and weakest-strength.
- Cross-kind identity policy remains tracked by SOW-0002.
- Table/modal composition remains tracked by SOW-0022.
- vSphere topology v1 migration remains tracked by SOW-0024.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
