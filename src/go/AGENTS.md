# Go Area Instructions

This file routes Go-specific work under `src/go/`. Repository-wide rules in
the repo-root `AGENTS.md` still apply. More specific `AGENTS.md` files under
subdirectories override this file for that subtree. Paths below are
repo-relative unless stated otherwise.

## Sensitive Data

The repo-root `AGENTS.md` sensitive-data policy applies in full to all durable
Go artifacts, including code comments, docs, specs, skills, and SOWs.

## Task Routing

| Work area | Start here | Notes |
|---|---|---|
| New go.d collector | `src/go/plugin/go.d/docs/how-to-write-a-collector.md` | New go.d collectors use framework V2. |
| go.d V2 implementation details | `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md`, `src/go/pkg/metrix/README.md`, `src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/chartengine/README.md` | Use the skill for maintainer style and the READMEs for framework API contracts. |
| Collector design across plugins | `.agents/skills/project-writing-collectors/SKILL.md` | Use for NIDL, cardinality, obsoletion, missing data, logging, and config discipline. |
| Integration metadata, taxonomy, generated docs | `.agents/skills/integrations-lifecycle/SKILL.md`, `.agents/skills/integrations-lifecycle/consistency.md` | Source artifacts and generated artifacts must stay synchronized. |
| IBM.d work | `src/go/plugin/ibm.d/AGENTS.md` | IBM.d has a generator-driven workflow; do not apply go.d V2 layout rules there. |
| Function handlers | `src/go/plugin/framework/functions/README.md`, `src/go/tools/functions-validation/README.md` | Collector Functions should be isolated behind narrow dependencies. |
| Topology payloads | `.agents/skills/project-create-topology/SKILL.md`, `.agents/sow/specs/topology-function-schema.md`, `src/go/pkg/topology/v1` | New topology producers must use the production `netdata.topology.v1` schema. |
| Host scopes / vnodes | `.agents/sow/specs/go-v2-host-scope.md`, `src/go/plugin/go.d/collector/azure_monitor/` | Use host scopes when one job emits metrics for resources that should appear as separate Netdata nodes. |
| Matchers/selectors | `src/go/pkg/matcher/README.md` | Prefer existing matcher APIs over custom selector grammars. |
| Core framework changes | See `Core Framework Change Gate` below | Human approval is required before implementation. |

## New go.d Collector Rules

- Use framework V2: `CreateV2`, `metrix.CollectorStore`,
  `MetricStore()`, embedded `charts.yaml`, and `ChartTemplateYAML()`.
- Do not teach or copy the V1 `Collect() map[string]int64` pattern for new
  collectors.
- Keep the full collector artifact set synchronized: code, `metadata.yaml`,
  `taxonomy.yaml`, `config_schema.json`, stock `.conf`, health alerts, and
  generated/symlinked README content. The detailed checklist lives in
  `.agents/skills/integrations-lifecycle/consistency.md`.
- Add public config options only when they represent a real operator decision.
  Use internal constants for implementation tuning such as page sizes, scan
  windows, retry limits, and cadence unless user control is clearly justified.
- Do not inherit unsupported config knobs from adjacent collectors or generic
  templates.
- If the collector exposes Functions, put Function code in a dedicated
  `<name>func/` package behind a narrow `Deps` interface declared in that
  package. The Function package must not import the collector package or hold
  `*Collector`.
- If the collector emits topology, use `netdata.topology.v1`, the Go producer
  model in `src/go/pkg/topology/v1`, and validate payloads against
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`. Do not add new producers using
  legacy topology payloads.
- If one job emits metrics for multiple remote resources that should appear as
  separate Netdata nodes, use V2 host scopes/vnodes when the product semantics
  call for separate node identity.

## Core Framework Change Gate

Changes to shared Go framework code are high blast-radius. This includes, but is
not limited to, `src/go/plugin/framework/`,
`src/go/plugin/go.d/pkg/collecttest`, `src/go/pkg/metrix`,
`src/go/pkg/funcapi`, `src/go/pkg/topology`, `src/go/pkg/matcher`, and shared
runtime or chart-template code.

Before implementation, prepare the design and get explicit human approval. The
design must cover:

1. Root cause and why a collector-local fix is insufficient.
2. Affected public contracts: collector API, metrix behavior, chart templates,
   Functions, host scopes/vnodes, topology schema, generated docs, tests, or
   existing collector behavior.
3. Existing patterns to reuse and representative collectors to check.
4. Compatibility impact and whether a clean breaking change was explicitly
   accepted.
5. Implementation plan split into reviewable batches.
6. Validation plan with framework tests and affected-collector tests.
7. Documentation/spec/skill updates required by the behavior change.

Prefer a clean framework extension over collector-local glue, global variables,
or private package coupling when the problem is general. Clean end state is more
important than minimizing churn.

## Batching And Review

- Keep changes atomic. If a collector or framework task grows, split it into
  coherent batches before review becomes difficult.
- Do not mix framework changes, collector migrations, and integration-doc
  regeneration unless they are required for one coherent behavior change.
- For Go test style, follow the repo-root `AGENTS.md` "Go test style" section.
