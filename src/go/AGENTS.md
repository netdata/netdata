# Go Area Instructions

This file routes Go-specific work under `src/go/`. Repository-wide rules in
the repo-root `AGENTS.md` still apply. More specific `AGENTS.md` files under
subdirectories override this file for that subtree. Paths below are
repo-relative unless stated otherwise.

## Sensitive Data

The repo-root `AGENTS.md` sensitive-data policy applies in full to all durable
Go artifacts, including code comments, docs, specs, skills, and SOWs.

## Mandatory Development Principles

The repo-root clean-end-state and scope-discipline principles are mandatory and
apply in full. For Go work, enforce them explicitly when changing collectors,
framework code, runtime behavior, `metrix`, chart templates, Functions,
topology, tests, specs, or skills.

- You MUST prefer the clean framework or collector shape over preserving a
  smaller diff.
- You MUST re-check scope after each coherent batch. If a separate framework fix,
  collector cleanup, or docs rewrite becomes necessary, split it into its own
  step or submit it independently before continuing.
- You SHOULD use RFC-style requirement language (`MUST`, `SHOULD`, `MAY`) in
  Go-area specs, skills, and instructions when documenting enforceable rules.

## Task Routing

| Work area | Start here | Notes |
|---|---|---|
| New go.d collector | `src/go/plugin/go.d/docs/how-to-write-a-collector.md` | New go.d collectors use framework V2. |
| go.d V2 implementation details | `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md`, `src/go/pkg/metrix/README.md`, `src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/chartengine/README.md` | Use the skill for maintainer style and the READMEs for framework API contracts. |
| Collector design across plugins | `.agents/skills/project-writing-collectors/SKILL.md` | Use for NIDL, cardinality, obsoletion, missing data, logging, and config discipline. |
| Integration metadata, taxonomy, generated docs | `.agents/skills/integrations-lifecycle/SKILL.md`, `.agents/skills/integrations-lifecycle/consistency.md` | Source artifacts and generated artifacts MUST stay synchronized. |
| IBM.d work | `src/go/plugin/ibm.d/AGENTS.md` | IBM.d has a generator-driven workflow; go.d V2 layout rules MUST NOT be applied there. |
| Function handlers | `src/go/plugin/framework/functions/README.md`, `src/go/tools/functions-validation/README.md` | Collector Functions SHOULD be isolated behind narrow dependencies. |
| Topology payloads | `.agents/skills/project-create-topology/SKILL.md`, `.agents/sow/specs/topology-function-schema.md`, `src/go/pkg/topology/v1` | New topology producers MUST use the production `netdata.topology.v1` schema. |
| Host scopes / vnodes | `.agents/sow/specs/go-v2-host-scope.md`, `src/go/plugin/go.d/collector/azure_monitor/` | Use host scopes when one job emits metrics for resources that SHOULD appear as separate Netdata nodes. |
| Matchers/selectors | `src/go/pkg/matcher/README.md` | Prefer existing matcher APIs over custom selector grammars. |
| Core framework changes | See `Core Framework Change Gate` below | Human approval is REQUIRED before implementation. |

## New go.d Collector Rules

- New go.d collectors MUST implement `collectorapi.CollectorV2` from
  `src/go/plugin/framework/collectorapi/collector.go` and register via
  `CreateV2`. This includes writing metrics through `metrix.CollectorStore` and
  providing `ChartTemplateYAML()`.
- New go.d collector guidance MUST NOT teach or copy the V1
  `Collect() map[string]int64` pattern for new
  collectors.
- The full collector artifact set MUST stay synchronized: code, `metadata.yaml`,
  `taxonomy.yaml`, `config_schema.json`, stock `.conf`, health alerts, and
  generated/symlinked README content. The detailed checklist lives in
  `.agents/skills/integrations-lifecycle/consistency.md`.
- Public config options SHOULD be added only when they represent a real
  operator decision. Implementation tuning such as page sizes, scan windows,
  retry limits, and cadence SHOULD use internal constants unless user control is
  clearly justified.
- New collectors MUST NOT inherit unsupported config knobs from adjacent
  collectors or generic templates.
- If the collector exposes Functions, put Function code in a dedicated
  `<name>func/` package behind a narrow `Deps` interface declared in that
  package. The Function package MUST NOT import the collector package or hold
  `*Collector`.
- If the collector emits topology, it MUST use `netdata.topology.v1`, the Go
  producer model in `src/go/pkg/topology/v1`, and validate payloads against
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`. New producers MUST NOT use
  legacy topology payloads.
- If one job emits metrics for multiple remote resources that SHOULD appear as
  separate Netdata nodes, it MUST use V2 host scopes/vnodes.

## Core Framework Change Gate

Changes to shared Go framework code are high blast-radius. This includes, but is
not limited to, `src/go/plugin/framework/`,
`src/go/plugin/go.d/pkg/collecttest`, `src/go/pkg/metrix`,
`src/go/pkg/funcapi`, `src/go/pkg/topology`, `src/go/pkg/matcher`, and shared
runtime or chart-template code.

Before implementation, you MUST prepare the design and get explicit human
approval. The design MUST cover:

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
8. Mandatory clean-end-state check: the intended final framework shape and why it is
   better than a local workaround or smaller diff.
9. Mandatory scope check: what belongs in this change, what is explicitly
   deferred, and whether any independent prerequisite should land first.

You SHOULD prefer a clean framework extension over collector-local glue, global
variables, or private package coupling when the problem is general. Clean end
state is more important than minimizing churn.

## Batching And Review

- Changes SHOULD stay atomic. If a collector or framework task grows, split it
  into coherent batches before review becomes difficult.
- At every batch boundary, you MUST re-evaluate clean end state and scope. If
  the branch now contains independent work, pause and submit that work
  separately or defer it before continuing.
- Changes MUST NOT mix framework changes, collector migrations, and
  integration-doc regeneration unless they are required for one coherent
  behavior change.
- For Go test style, follow the repo-root `AGENTS.md` "Go test style" section.
