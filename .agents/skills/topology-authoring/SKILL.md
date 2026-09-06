---
name: topology-authoring
description: Developer workflow for creating or changing Netdata topology producers and their `netdata.topology.v1` Function payloads (`topology:network-connections`, `topology:streaming`, `topology:snmp`, vSphere, `topology:cato_networks`, or a new producer), including actor and link design, evidence and detail tables, correlation rules, graph presentation, modal composition, telemetry overlays, validation, and the Cloud aggregator contract a producer relies on. Not for querying topology as an operator (query-netdata-cloud, query-netdata-agents) or for SNMP profile `topology:` rows (collectors-snmp-profiles).
---

# Topology Producers

Developer skill for assistants working in this repository, not an operator skill. It routes to the documents that
own each fact and keeps only the rules and workflow that have no other owner. When you find a fact here and in an
owner document, the owner document wins; fix this file.

## Owners

Read the owner for the plane you touch; do not work from memory of it.

| Owner | Owns |
|---|---|
| `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` | Every required field and closed token (icons, colors, layout, cell types, visibility, projections, arrow, direction, severities, rule classes); every object is `additionalProperties: false`. Type ids (`actor_types`, `link_types`, `evidence_types`, `table_types`) are open, tokens are closed. |
| `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` | The payload contract, plane by plane, plus per-producer Shape sections and the aggregator rules a producer relies on. |
| `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md` | Migration state per producer, Cloud aggregator and frontend scope, the CTS gate, and design that is not yet in the schema. |
| `src/go/pkg/topology/v1` | Go builders and the semantic validator (`validate.go`, `validate_notification.go`). |
| `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md` | SNMP producer internals, where to change what, and its validation commands. |
| `src/go/pkg/l2topology/parity/README.md` | The L2 parity runbook against the enlinkd oracle. |
| `src/go/tools/functions-validation/README.md#validate-topology-v1-fixtures` | Validating any topology payload file from the CLI. |
| `src/plugins.d/FUNCTION_UI_REFERENCE.md`, `src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md` | Function transport: envelope, `v: 3`, `selections`, `info` responses. |
| `docs/npm/topology/` | What operators are told the topology means. Change it when a user-visible meaning changes. |

If you are also changing the collector that hosts the producer, `.agents/skills/collectors-authoring/SKILL.md` routes
that work. SNMP profile `topology:` rows (which OIDs feed the producer) belong to `collectors-snmp-profiles`; this skill
starts where those rows have become observations.

## Producers

| Function | Producer | Tests |
|---|---|---|
| `topology:network-connections` | `src/collectors/network-viewer.plugin/network-viewer-topology.c` (shared renderer; Windows main in `network-viewer-windows.c`, container rules in `network-viewer-topology-containers.c`) | `src/collectors/network-viewer.plugin/tests/validate_topology_payload.py`, `src/collectors/network-viewer.plugin/tests/validate_topology_container_fixtures.py`, fixtures under `src/collectors/network-viewer.plugin/tests/fixtures/topology/` |
| `topology:streaming` | `src/web/api/functions/function-topology-streaming.c` | fixture `src/go/tools/functions-validation/fixtures/topology-v1/streaming.json` |
| `topology:snmp` | `src/go/plugin/go.d/collector/snmp_topology/` (render in `internal/topologyv1`) | `topology_scenario_golden_test.go`, `internal/topologyv1/golden_test.go`, see `src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#validation-checklist` |
| vSphere | `src/go/plugin/go.d/collector/vsphere/func_topology*.go` | `func_topology_test.go` |
| `topology:cato_networks` | `src/go/plugin/go.d/collector/cato_networks/topology.go`, `catofunc/topology.go` | `topology_test.go` |

All five emit `netdata.topology.v1`; there is no pending producer migration. Cross-producer fixtures live in
`src/go/tools/functions-validation/fixtures/topology-v1/`; the Cloud aggregator itself is not in this repository.

## What The Code Enforces

State these as facts in reviews; do not re-derive them, and do not claim enforcement the code does not have.

- JSON Schema (`FUNCTION_TOPOLOGY_SCHEMA.json`): structure, required fields, closed tokens, unknown properties. A
  violation is an error from any validator that loads the schema. It does not check cross-references or row counts.
- Go semantic validator (`topologyv1.ValidateDecodedData` on `data`, `topologyv1.ValidateDecodedResponse` on a whole
  envelope; both return an error, there is no warning level): reference bounds, column-length equality with `rows`,
  dictionary indexes, label-policy display types, search columns, every `ports.sources[]` rule, highlight-path
  column types, every overlay-refs convention rule, correlation rules and point/claim key columns, all modal
  projections, and the closed tokens again. Producer tests call `ValidateDecodedData`
  (`cato_networks/topology_test.go` is the reference shape); the CLI tool calls `ValidateDecodedResponse`.
- The CLI tool (`src/go/tools/functions-validation`) counts a topology payload's rows as
  `max(actor rows, link rows)` (`topologyv1.GraphRowsFromDecodedData`) for its `--min-rows` and `--require-rows`
  gates, so an actor-only payload passes; nothing in `pkg/topology` caps row counts.
- Not checked by the Go validator although the contract states them: `actor_labels` key, value, source, and kind
  column types (only `actor_column` is type-checked), `search.label_keys[]` against any label registry (string shape
  only), correlation `key_space` values (schema-required, never read), and unknown optional column ids on
  `actor_table` port sources.
- `topology:network-connections` returns a Function error with HTTP `413` above its 64 MiB budget and never
  truncates (`src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#network-connections-shape`).
- The SNMP scenario golden suite skips, not fails, without the external fixture checkout
  (`src/go/plugin/go.d/collector/snmp_topology/ARCHITECTURE.md#scenario-golden-suite`). A green run without the
  skip line checked proves nothing.
- Nothing enforces: payload size on realistic data, actor identity stability across restarts, that a modal section is
  a recipe rather than a duplicated table, or that a type id follows the semantic conventions. Those are review items.

## Workflow

Each step names the owner section that holds the rules; read it before designing. Guide means
`src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.

1. Purpose and scale: name the graph users need and estimate actor, link, and evidence row counts and raw and gzip
   payload size on realistic data before choosing shapes.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#mental-model`.
2. Actors: stable identity, display separate from identity, `identity` / `merge_identity` / `parent_identity`,
   aggregation scopes.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#required-actor-semantics`;
   network-connections grouping:
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#network-connections-actor-grouping`.
3. Links and direction: compact renderable relationships, one-to-many detail in evidence, semantic link types per
   meaning, `orientation` plus `direction_role`.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#required-link-semantics`.
4. Evidence and tables: lossless relationship proof versus actor-owned detail, table roles, `actor_labels`.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#evidence-plane`,
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#detail-tables`,
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#actor-labels-and-modal-composition`.
5. Overlays: templates once, compact refs per row, built with `topologyv1.NewActorOverlayRefsBuilder` or
   `NewLinkOverlayRefsBuilder`; in go.d pass `job.Name()` as `collect_job`, never `job.FullName()`.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#telemetry-overlays`.
6. Correlation: rule classes, actions, points and claims, and what the aggregator does with them.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#correlation-plane`,
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#aggregator-behavior`.
7. Presentation and modals: type-level tokens only, `label_policy`, `search`, `ports.sources[]`, highlight paths,
   modal recipes over existing sources; recipes in `how-tos/add-graph-presentation.md`.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#presentation-plane`,
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#closed-token-vocabulary`,
   `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#actor-labels-and-modal-composition`.
8. Compact tables: `const` / `dict` / `values` codecs through `topologyv1.NewTableBuilder`, `MustTable`, and
   `NewStringDictionary` in Go.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#compact-tables`.
9. Notifications, if any: `data.notifications` only, origin kept separate from `affected_node_id`, CTS acceptance
   verified before Agents emit.
   Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#notifications`.
10. Per-producer contract.
    Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#network-connections-shape`,
    `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#streaming-shape` and
    `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#streaming-modal-sections`,
    `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#snmpl2-shape` and
    `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#snmp-logical-l3-and-control-plane-links`,
    `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#vsphere-shape`.
11. Validate and measure: schema plus semantic validation in tests, the producer's own suites (table above), size
    on realistic data, and the checklist.
    Guide: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#producer-checklist`.

## Rules Without A Code Owner

- Raw captures of real payloads stay under `.local/` (repository rule, `AGENTS.md`). `actor_labels` and streaming
  detail tables inherit the Function's sensitive-data classification
  (`src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#actor-labels-and-modal-composition`,
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#streaming-shape`).
- Sparse grouping columns: test that a consumer keeps actor identity for null or empty grouping keys instead of
  merging every null row into one bucket.
- Fail explicitly on any size or row limit; never truncate a topology and present it as complete.
- Validators need negative tests: missing label-policy columns, non-display label columns, a missing port-bullet
  source table, bad highlight-path columns, an invalid token, and a modal projection over a missing source.
- A C producer can be syntax-checked without a full build: configure with
  `cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON`, then run the file's command from `build/compile_commands.json` with
  `-fsyntax-only`.
- Design the schema does not carry yet (four-dimension table merge policy, loose-side materialization) is recorded in
  `src/plugins.d/FUNCTION_TOPOLOGY_IMPLEMENTATION_SCOPE.md#design-not-yet-in-the-schema`; do not emit it and do not
  document it as contract.

## How-Tos

- `how-tos/add-graph-presentation.md`: add type-level presentation, port bullets, legends, and per-actor highlight
  paths to a producer, with the negative tests that catch the usual mistakes.
- `how-tos/preserve-semantic-link-types.md`: keep protocol, confidence, and inferred state as distinct link types.
- `how-tos/verify-network-connections-layout-tokens.md`: check a live local Agent's link layout tokens and
  correlation wiring without exposing identifiers.

Whenever you answer a developer question that needed analysis across more than one owner document and no how-to
covers it, write the how-to here and list it above before finishing.
