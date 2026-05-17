# SOW-0031 - Topology V1 Zero-Heuristic Rendering Contract

## Status

Status: in-progress

`completed` is the successful terminal status. `done` is a directory name, not a status value. Do not use `Status: done` or `Status: complete`.

Sub-state: implementation approved by the user after the Cloud frontend TODO review loop.

## Requirements

### Purpose

Make `netdata.topology.v1` fit for polished, topology-agnostic graph rendering. The UI must not hardcode domain words such as self, segment, endpoint, device, SNMP, LLDP, CDP, parent, child, client, server, router, or switch when rendering v1 payloads.

### User Request

Create a SOW, analyze the remaining frontend heuristics, propose the missing schema fields, update local schema/spec/docs/skills, create Cloud aggregator and Cloud frontend handoff artifacts, then implement the approved zero-heuristic contract in the Agent producers/shared helpers, Cloud frontend, and Cloud topology service.

### Assistant Understanding

Facts:

- The current v1 schema already carries actor/link/port presentation, modal recipes, legend, highlight behavior, port-bullet sources, and link layout distance/strength.
- The Cloud frontend report identified remaining heuristics outside the v1 decoder/modal path: self detection, segment/endpoint/device/inferred detection, SNMP/LLDP/CDP detection, hardcoded search paths, capability-to-icon fallback, and v1-to-legacy `protocol`/port shims.
- Raw SVG icons are explicitly not allowed by the current topology documentation and should remain disallowed.

Inferences:

- The missing contract is not more producer-specific fields. The missing contract is a small set of topology-agnostic type-level policies that let the UI treat v1 as data-driven.
- Link behavior classification belongs on `link_types.<id>.semantic_role`, not under `presentation`, because discovery, ownership, traffic, correlation, and control are graph semantics. Visual appearance remains under `presentation`.

Unknowns:

- Exact numeric UI mappings for `size.scale` and `layout.repulsion` are UI-owned and must be tuned visually after implementation.

### Acceptance Criteria

- Agent repo has an active SOW with the zero-heuristic contract and implementation boundary.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` defines the proposed optional schema fields without requiring producer migration immediately.
- Local topology spec, developer guide, implementation scope, and project topology skill document the new contract.
- Cloud frontend implements v1 renderer behavior from the TODO while preserving legacy behavior in the legacy path.
- Cloud topology service decodes, preserves, namespaces/deduplicates, and emits the new fields.
- Agent shared topology helpers and producers emit the new fields for network-connections, SNMP/L2, and streaming.
- Validation covers Agent schema/producer fixtures, Cloud frontend tests, Cloud topology service tests, and real local topology payload smoke checks where practical.

## Analysis

Sources checked:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`
- `.agents/sow/specs/topology-function-schema.md`
- `.agents/sow/specs/topology-modes-correlation-aggregation.md`
- `.agents/skills/project-create-topology/SKILL.md`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/utils.js`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/components/graph/forceGraph.js`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/components/graph/useForceSimulation.js`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/components/topology/actorModal/portTable.js`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/v1/buildLinks.js`
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/v1/buildRenderableLinks.js`

Current state:

- Actor presentation exists, but actor size only has `mode` and optional `metric_column`; it lacks a type-level fixed scale token.
- Link presentation has `layout.strength` and `layout.distance`; actor presentation lacks a corresponding repulsion token.
- Link type has `direction_role`, but no generic semantic classification for discovery, ownership, traffic, correlation, or control behavior.
- Actor type has no search contract, so the frontend still indexes hardcoded paths from legacy `details.match`, `details.attributes`, and `details.labels`.
- Icon tokens are closed, which is correct, but producers cannot fully replace capability-based frontend icon inference unless every v1 actor type declares a suitable token.

Risks:

- If the UI keeps heuristics in v1, every new topology can regress visually when its actor/link names do not match existing frontend guesses.
- If semantics are placed under `presentation`, the aggregator may need to parse a visual object to make graph-behavior decisions.
- If raw SVG is allowed, topology payloads become a new script/rendering attack surface and every consumer must sanitize untrusted markup.
- If repulsion reuses link strength without a separate field name, producers and UI developers will conflate two different force-graph quantities.

## Proposed Schema

### Actor Type Search

```json
{
  "types": {
    "actor_types": {
      "process": {
        "search": {
          "enabled": true,
          "columns": ["display_name", "process_name"],
          "label_keys": ["cmdline", "username"]
        }
      }
    }
  }
}
```

Rules:

- `search.columns[]` references actor-table scalar columns.
- `search.label_keys[]` references `actor_labels.key` values.
- `search.enabled: false` removes helper actors from graph search.
- UI must not traverse producer-specific `details`, `match`, `attributes`, or label paths for v1 search.

### Actor Presentation Size And Repulsion

```json
{
  "presentation": {
    "size": {
      "mode": "metric",
      "metric_column": "socket_count",
      "scale": "normal"
    },
    "layout": {
      "repulsion": "normal"
    }
  }
}
```

Rules:

- `size.scale` is `compact`, `normal`, or `emphasized`.
- `layout.repulsion` is `weakest`, `weaker`, `normal`, `stronger`, or `strongest`.
- Producers emit tokens only. The UI owns numeric radius and charge mappings.
- Actor repulsion is separate from link strength.
- `size.scale` composes with `size.mode`.
- Missing `size.scale` and missing `layout.repulsion` use `normal`; the UI must
  not fall back to self/device/SNMP/endpoint heuristics for v1.
- Initial UI-owned mappings are `compact=0.85`, `normal=1.0`,
  `emphasized=1.18` for size scale and `weakest=-200`, `weaker=-300`,
  `normal=-450`, `stronger=-700`, `strongest=-1000` for repulsion. These
  numbers are not schema.

### Link Semantic Role

```json
{
  "types": {
    "link_types": {
      "lldp": {
        "orientation": "observed_bidirectional",
        "direction_role": "observation",
        "semantic_role": "discovery",
        "aggregation": {
          "direction": "canonicalize_unordered",
          "evidence": "append"
        }
      }
    }
  }
}
```

Allowed roles:

- `normal`
- `discovery`
- `ownership`
- `traffic`
- `correlation`
- `control`

Rules:

- `semantic_role` drives behavior such as discovery filtering, ownership/coherence handling, traffic emphasis, and correlation treatment.
- Link appearance still comes from `presentation`.
- UI and aggregator must not infer role from `link.type`, `link.protocol`, LLDP/CDP string checks, or label names.
- Day-1 UI behavior only requires a concrete behavior difference for
  `semantic_role: discovery`. Other semantic roles are preserved and render
  through presentation until future behavior is specified.

### Arrow Auto

`presentation.arrow` remains the authoritative visual signal. When it is
`auto` or omitted, the UI derives arrows from `orientation` and
`direction_role`:

- `undirected` -> no arrow;
- `observed_bidirectional` -> no arrow;
- `direction_role: none` -> no arrow;
- `direction_role: observation` -> no arrow;
- `directed` with `flow` or `dependency` -> forward from `src_actor` to
  `dst_actor`;
- `hierarchical` with `ownership` -> forward from `src_actor` to `dst_actor`;
- all other combinations -> no arrow and a diagnostic if the combination is
  schema-valid but semantically unusual.

`observed_bidirectional` does not mean draw arrows at both ends. Producers must
set `presentation.arrow: "both"` or `"reverse"` explicitly when needed.

`direction_role` is required by the v1 schema. Missing `direction_role` is
invalid input and must not produce an inferred arrow from `orientation:
"directed"` alone. The UI should render `auto` as no arrow and emit the normal
missing/invalid-field diagnostic.

For schema-valid values, the semantic diagnostic boundary is explicit:

- no diagnostic for `directed+flow`, `directed+dependency`,
  `hierarchical+ownership`, `undirected+none`, `undirected+observation`,
  `observed_bidirectional+none`, or `observed_bidirectional+observation`;
- diagnostic for `directed+none`, `directed+observation`,
  `directed+ownership`, `hierarchical+none`, `hierarchical+flow`,
  `hierarchical+dependency`, `hierarchical+observation`, `undirected+flow`,
  `undirected+dependency`, `undirected+ownership`,
  `observed_bidirectional+flow`, `observed_bidirectional+dependency`, or
  `observed_bidirectional+ownership`.

### Closed Icon Tokens Only

Allowed icons remain schema-owned tokens. This SOW adds generic tokens needed to remove capability inference:

- `device`
- `endpoint`
- `correlation`
- `interface`
- `group`
- `unknown`

Rules:

- Raw SVG remains disallowed.
- Capability-to-icon inference moves to producers, where producer-specific capabilities are already known.
- UI maps icon tokens to safe bundled icons only.

## Pre-Implementation Gate

Status: approved

Problem / root-cause model:

- The v1 payload is mostly schema-driven, but renderer behavior still depends on legacy frontend heuristics. The root cause is missing v1 contract fields for actor search, actor fixed emphasis, actor repulsion, and link semantic behavior.

Evidence reviewed:

- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` already defines `actor_type.presentation`, `link_type.presentation`, and link `presentation.layout`.
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` had no actor `search`, no actor `presentation.layout`, no `size.scale`, and no link `semantic_role` before this SOW.
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/utils.js` contains actor/link kind heuristics such as self, segment, endpoint/derived, inferred, device, SNMP, LLDP, and CDP detection.
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/components/graph/forceGraph.js` contains hardcoded graph search path extraction and icon fallback from capability inference.
- `${CLOUD_FRONTEND_REPO}/src/domains/functions/topology/v1/buildLinks.js` mirrors v1 row fields into legacy `protocol`, `sourcePort`, and `targetPort` properties only for legacy renderer paths.

Affected contracts and surfaces:

- Agent topology JSON schema.
- Agent topology developer guide.
- Agent topology durable spec.
- Agent project topology skill.
- Cloud frontend v1 decoder, renderer, search, force simulation, legend, icon mapping, port table, and legacy adapter.
- Cloud topology service type registry decode, aggregation merge, type namespace/dedup, and returned schema preservation.
- Producer migrations for network-connections, SNMP/L2, streaming, and future vSphere v1.

Existing patterns to reuse:

- Closed token pattern from color, opacity, width, icon, link layout strength, and link layout distance.
- Existing type registry pattern for actor/link/port type presentation.
- Existing frontend diagnostics pattern for unknown tokens.
- Existing Cloud aggregator namespacing/dedup behavior for type definitions.

Risk and blast radius:

- UI renderer refactor has high visual regression risk because it touches the graph hot path.
- Producer migration is broad but optional fields allow incremental rollout.
- Aggregator should preserve and merge the new fields; it must not invent type semantics.
- Raw SVG remains rejected to avoid new untrusted markup risk.

Sensitive data handling plan:

- This planning pass uses only schema/docs/code paths and sanitized descriptions. Durable artifacts must not include raw payload captures, credentials, cookies, bearer tokens, SNMP communities, customer names, personal data, non-private customer-identifying IPs, or private endpoints.

Implementation plan:

1. Update shared Go topology structs and validators for `actor.search`, `actor.presentation.size.scale`, `actor.presentation.layout.repulsion`, and `link.semantic_role`.
2. Update each Agent producer to emit the new optional fields for every v1 actor/link type it owns.
3. Implement Cloud frontend behavior using the TODO created by this SOW.
4. Implement Cloud topology service behavior using the SOW created by this SOW.
5. Validate with real topology payloads across network-connections, SNMP/L2, and streaming.

Validation plan:

- Validate JSON schema syntax with `jq`.
- Validate SOW status/directory consistency with project audit.
- During implementation, add schema validation fixtures and UI/aggregator tests that prove v1 rendering does not call legacy heuristics.

Artifact impact plan:

- AGENTS.md: no update expected; this does not change project-wide workflow.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`.
- Specs: update `.agents/sow/specs/topology-function-schema.md`.
- End-user/operator docs: no update expected; this is developer/schema contract work.
- End-user/operator skills: no update expected; this is not an operator workflow.
- SOW lifecycle: SOW-0031 is the active implementation ledger for this approved cross-repo work.

Open-source reference evidence:

- None checked. This is a local Netdata schema/frontend/aggregator contract gap, and the user requested local contract handoff rather than external product research.

Open decisions:

- None. The user approved implementation after the frontend TODO follow-up clarifications were recorded.

## Implications And Decisions

1. User accepted adding a schema contract to remove frontend v1 heuristics.
2. User accepted keeping raw SVG out of the payload.
3. User accepted adding a dedicated SOW/TODO handoff for Cloud frontend and Cloud aggregator before implementation.

## Plan

1. Record the zero-heuristic schema contract in local Agent schema/spec/docs/skill artifacts.
2. Create a Cloud frontend TODO that asks the UI agent to remove v1 renderer heuristics only after schema support lands.
3. Create a Cloud topology service SOW that asks the aggregator agent to decode, preserve, namespace, deduplicate, and emit the new optional fields without inventing producer semantics.
4. Implement the approved contract in Agent, Cloud frontend, and Cloud topology service.

## Execution Log

### 2026-05-17

- Created this SOW.
- Added optional JSON schema fields for actor search, actor size scale, actor layout repulsion, link semantic role, and generic closed icon tokens.
- Updated local topology spec, developer guide, implementation scope, and project topology skill.
- Created Cloud frontend and Cloud topology service handoff artifacts.
- Appended backend answers to frontend follow-up questions: arrow auto mapping,
  direction_role UI usage, initial UI-owned numeric mappings, transitional
  neutral defaults, and current aggregator unknown-field behavior.
- Appended final `arrow: auto` clarifications covering required
  `direction_role`, current producer status, and the semantic diagnostic
  boundary.
- User approved implementation after the Cloud frontend TODO review loop.

## Validation

Acceptance criteria evidence:

- SOW active at `.agents/sow/current/SOW-0031-20260517-topology-v1-zero-heuristic-rendering-contract.md`.
- JSON schema updated at `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Spec/docs/skill updates are present in `.agents/sow/specs/topology-function-schema.md`, `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`, `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md`, and `.agents/skills/project-create-topology/SKILL.md`.
- Cloud frontend TODO created as `${CLOUD_FRONTEND_REPO}/TODO-topology-v1-zero-heuristic-rendering-contract.md`.
- Cloud topology service SOW active as `${CLOUD_TOPOLOGY_SERVICE_REPO}/.agents/sow/current/SOW-0017-20260517-topology-v1-zero-heuristic-contract.md`.
- Agent topology structs and validators implement actor search, actor size scale, actor layout repulsion, link semantic role, and closed icon tokens in `src/go/pkg/topology/v1/types.go` and `src/go/pkg/topology/v1/validate.go`.
- Agent producers emit the contract fields in `src/collectors/network-viewer.plugin/network-viewer.c`, `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`, and `src/web/api/functions/function-topology-streaming.c`.
- Cloud frontend v1 rendering consumes schema-driven search, size, repulsion, semantic role, and port lookup fields in `${CLOUD_FRONTEND_REPO}`.
- Cloud topology service decodes, validates, preserves, and namespaces conflicting definitions for the new fields in `${CLOUD_TOPOLOGY_SERVICE_REPO}`.

Tests or equivalent validation:

- `jq empty src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` passed.
- Agent repo `.agents/sow/audit.sh` accepted SOW-0031 status/directory placement and reported no sensitive-data findings. The audit still reports unrelated pre-existing framework warnings: one older done SOW has a status/directory mismatch, and legacy non-project skill directories need classification.
- Cloud topology service `.agents/sow/audit.sh` passed cleanly after creating SOW-0017.
- `go test ./pkg/topology/v1 ./plugin/go.d/collector/snmp_topology` passed from `src/go`.
- `git diff --check` passed in the Agent repo.
- `git diff --check` passed in the Cloud frontend repo.
- `git diff --check` passed in the Cloud topology service repo.
- Cloud frontend focused tests passed for v1 normalization, presentation adapter, renderable links, color token handling, port utilities, and force simulation helpers.
- Cloud topology service tests passed: `go test ./internal/topology/schema ./internal/topology/validate ./internal/topology/aggregate`.
- Full Cloud frontend test suite passed: 269 suites passed, 2,139 tests passed, 6 skipped, 7 snapshots passed.
- Agent C build validation is blocked by local filesystem permissions: `build`, `build/.ninja_log`, and `build/.ninja_deps` are owned by `root:root`; `ninja -C build` cannot create `build/.ninja_lock` in this worktree.

Real-use evidence:

- Pending final local install/browser validation because the Agent C build is blocked by the root-owned local build directory.

Reviewer findings:

- Frontend agent report is the input to this SOW. Additional external review can be run after implementation validation if requested.

Same-failure scan:

- `rg` verified the new contract terms appear in the schema, developer guide, implementation scope, durable spec, project skill, and SOW.

Sensitive data gate:

- Durable artifacts use only schema names, file paths, sanitized repo placeholders, and generic examples. No raw secrets, credentials, bearer tokens, SNMP communities, personal names, customer identifiers, non-private customer-identifying IPs, private endpoints, or proprietary incidents are included.

Artifact maintenance gate:

- AGENTS.md: not changed; no workflow or project-wide guardrail changed.
- Runtime project skills: `.agents/skills/project-create-topology/SKILL.md` updated.
- Specs: `.agents/sow/specs/topology-function-schema.md` updated.
- End-user/operator docs: not affected; this is developer schema work.
- End-user/operator skills: not affected; no operator workflow changed.
- SOW lifecycle: SOW is `in-progress` in `current/`; do not move to `done/` until remaining validation and commit handling are complete.

Specs update:

- `.agents/sow/specs/topology-function-schema.md` updated.

Project skills update:

- `.agents/skills/project-create-topology/SKILL.md` updated.

End-user/operator docs update:

- Not affected. The change is internal topology producer/UI/aggregator contract behavior.

End-user/operator skills update:

- Not affected. Public query skills do not teach topology producer development or frontend renderer internals.

Lessons:

- v1 schemas need behavior contracts for renderer decisions, not only visual tokens. Otherwise legacy UI heuristics leak into new topology types.

Follow-up mapping:

- Cloud frontend implementation is in progress in `${CLOUD_FRONTEND_REPO}` and tracked by its TODO.
- Cloud topology service implementation is in progress in `${CLOUD_TOPOLOGY_SERVICE_REPO}` and tracked by SOW-0017.
- Remaining work is validation, lifecycle closure, and commit handling after all repositories are ready.

## Outcome

Implementation in progress. Core Agent, Cloud frontend, and Cloud topology service code paths are implemented; final validation and lifecycle closure remain.

## Lessons Extracted

- The v1 renderer cannot become topology-agnostic from presentation colors alone. It needs producer-owned behavior tokens for search, discovery behavior, actor sizing, and graph-layout repulsion.
- Aggregators must namespace contradictory type definitions instead of inventing precedence rules for producer-owned presentation or behavior metadata.

## Followup

- Full Cloud frontend validation is complete.
- Resolve or explicitly record the local Agent build-directory permission blocker.
- Decide commit split across Agent, Cloud frontend, and Cloud topology service after validation.

## Regression Log

None yet.
