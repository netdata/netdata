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
| Migrating go.d V1 collector to V2 | `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` | Preserve public contracts unless a breaking change is explicitly approved. |
| go.d V2 implementation details | `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md`, `src/go/pkg/metrix/README.md`, `src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/chartengine/README.md` | Use the skill for maintainer style and the READMEs for framework API contracts. Editing `metrix` or framework packages is framework-gated work. |
| go.d helper packages | `src/go/plugin/go.d/docs/helper-packages.md` | Check existing HTTP, config-option, matcher, logger, socket, command, SQL, ping, log-file, and cloud-auth helpers before adding custom plumbing. |
| Collector design across plugins | `.agents/skills/project-writing-collectors/SKILL.md` | Use for NIDL, cardinality, obsoletion, missing data, logging, and config discipline. |
| Integration metadata, taxonomy, generated docs | `.agents/skills/integrations-lifecycle/SKILL.md`, `.agents/skills/integrations-lifecycle/consistency.md` | Source artifacts and generated artifacts MUST stay synchronized. |
| IBM.d work | `src/go/plugin/ibm.d/AGENTS.md` | IBM.d has a generator-driven workflow; go.d V2 layout rules MUST NOT be applied there. |
| Function handlers | `src/go/plugin/framework/functions/README.md`, `src/go/tools/functions-validation/README.md` | Collector Functions SHOULD be isolated behind narrow dependencies. |
| Topology payloads | `.agents/skills/project-create-topology/SKILL.md`, `.agents/sow/specs/topology-function-schema.md`, `src/go/pkg/topology/v1` | New topology producers MUST use the production `netdata.topology.v1` schema. |
| Host scopes / vnodes | `.agents/sow/specs/go-v2-host-scope.md`, `src/go/plugin/go.d/collector/azure_monitor/` | Use host scopes when one job emits metrics for resources that SHOULD appear as separate Netdata nodes. |
| Matchers/selectors | `src/go/pkg/matcher/README.md` | Prefer existing matcher APIs over custom selector grammars. |
| Core framework changes | `src/go/plugin/framework/docs/changing-framework-code.md` and `Core Framework Change Gate` below | The applicable approval tier MUST be satisfied before implementation. |

## New go.d Collector Rules

- New go.d collectors MUST implement `collectorapi.CollectorV2` from
  `src/go/plugin/framework/collectorapi/collector.go` and register via
  `CreateV2`. This includes writing metrics through `metrix.CollectorStore` and
  providing `ChartTemplateYAML()`.
- New go.d collector guidance MUST NOT teach or copy the V1
  `Collect() map[string]int64` pattern for new
  collectors.
- Collector runtime, metric, chart, config, alert, taxonomy, and documentation
  changes MUST follow the repository collector consistency policy. The detailed
  checklist lives in `.agents/skills/integrations-lifecycle/consistency.md`.
- Public config options SHOULD be added only when they represent a real
  operator decision. Implementation tuning such as page sizes, scan windows,
  retry limits, and cadence SHOULD use internal constants unless user control is
  clearly justified.
- New collectors MUST NOT inherit unsupported config knobs from adjacent
  collectors or generic templates.
- Collector-local globals, singletons, adapters, caches, or glue layers that
  substitute for missing shared framework/helper capabilities are
  framework-scope work and MUST follow
  `src/go/plugin/framework/docs/changing-framework-code.md` before
  implementation.
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

## go.d V1-to-V2 Migration Rules

- V1-to-V2 migrations MUST start with
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`.
- Migrations MUST preserve chart IDs, contexts, dimensions, config keys,
  defaults, health lookups, metadata, taxonomy, stock config, and service
  discovery behavior unless the user explicitly approves a breaking change.
- Compatibility migration SHOULD be separate from enrichment such as new labels,
  host scopes, topology, Functions, or config expansion.
- Completed migrations MUST NOT keep a runtime V1-to-V2 bridge. Temporary V1
  logic can be used during development for parity checks, but it MUST be
  removed from the final migrated collector.

## Core Framework Change Gate

Changes to shared Go framework code are high-blast-radius work. Before changing
these areas, read `src/go/plugin/framework/docs/changing-framework-code.md`.
That guide is the canonical owner of the framework-change scope list, required
design note, validation expectations, and artifact checks.

Framework-change implementation MUST NOT begin until the applicable approval
tier in that guide is satisfied. You SHOULD prefer a clean framework extension
over collector-local glue, global variables, or private package coupling when
the problem is general.

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
