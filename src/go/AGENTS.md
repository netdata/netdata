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
| Topology payloads | `.agents/skills/project-create-topology/SKILL.md`, `.agents/skills/project-create-topology/topology-function-schema.md`, `src/go/pkg/topology/v1` | New topology producers MUST use the production `netdata.topology.v1` schema. |
| Host scopes / vnodes | `.agents/skills/project-writing-go-modules-framework-v2/go-v2-host-scope.md`, `src/go/plugin/go.d/collector/azure_monitor/` | Use host scopes when one job emits metrics for resources that SHOULD appear as separate Netdata nodes. |
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

Changes to shared Go framework code (`metrix`, `chartengine`, `charttpl`, the job
runtime, `collectorapi`) are high-blast-radius: this code runs in EVERY collector,
most of it on the per-cycle hot path. Before changing these areas, read
`src/go/plugin/framework/docs/changing-framework-code.md` — the canonical owner of
the framework-change scope list, required design note, validation expectations, and
artifact checks. Implementation MUST NOT begin until that guide's applicable
approval tier is satisfied.

- Clean extension over glue: prefer a clean framework extension over
  collector-local globals, singletons, adapters, or private-package coupling when
  the problem is general.
- Behavior preservation: when a framework change also touches shipped collectors,
  preserve their observable behavior (chart IDs, contexts, dimensions, config keys,
  defaults) and validate representative consumers, not only the framework package
  (see "Validation For Go Changes").
- metrix contract: `metrix` keeps one descriptor per metric NAME, resolved
  atomically at commit and bounded — a name idle past `expireAfterSuccessCycles +
  descriptorGraceCycles` is evicted, and re-registers cleanly afterward. A consumer
  that caches per-name state across cycles MUST couple its lifetime to the optional
  `metrix.DescriptorRetention` accessor. See `src/go/pkg/metrix/README.md`
  ("Descriptor Lifecycle and Retention").

## Hot-Path And Benchmark Discipline

metrix commit/collect, per-sample/per-write, and per-cycle code are hot paths that
run for every collector on every cycle.

- Before/after REQUIRED: a hot-path change MUST include before/after
  `go test -bench` numbers (use `git stash` for the "before" baseline), not just
  "tests pass".
- Allocation count is the gate: assert allocs stay within the intended envelope
  (e.g. a sparse commit stays ~O(touched), never O(retained)). `ns/op` is a
  dev-machine trend indicator, NOT a CI gate — label it as such inline, and never
  record a personal name in the file.
- State the complexity envelope explicitly (e.g. "commit is O(live-series +
  touched + distinct-names)") and prove the change did not introduce an
  O(samples)/O(retained)/O(n^2) regression.
- Keep bench comments in sync with the code they measure, in the same change, with
  self-contained wording (no round/session references).

## Go Formatting

`gofmt` and `goimports` are the baseline. This repository additionally
RECOMMENDS (not CI-enforced) a fuller formatting pipeline that keeps line
wrapping tight and keyed struct literals readable:

1. `golines -m 120 -t 4 -w <paths>` — join over-wrapped signatures/calls and
   split lines past ~120 columns. SKIP this step if `golines` is not installed
   (`go install github.com/segmentio/golines@latest`).
2. `go run ./tools/expandstructs <paths>` — put each keyed struct-literal field
   on its own line. Runs `gofmt` internally.
3. `goimports -w <paths>` — order imports.

Conventions this encodes:

- Keep a signature / call / return / composite literal on ONE line when it fits
  within ~120 columns; wrap only when it does not.
- Keyed struct literals of a named type go ONE field per line.

`gofmt`/`goimports` cannot express these two rules, so they are not CI-enforced;
re-run the pipeline if code drifts. See `tools/expandstructs/README.md`.

## Validation For Go Changes

Run the narrowest command that actually exercises the change; do not claim
full-project validation from a narrow one.

- Always: formatted per "Go Formatting" above (at minimum `gofmt` clean) and
  `go vet ./<pkg>/` clean.
- Unit: `go test -count=1 ./<pkg>/...` for every package you touched.
- Concurrency: add `-race` for concurrency-sensitive packages (`metrix`, the job
  runtime, `plugin/agent/jobmgr`).
- Shared framework code: ALSO build and test representative consumers (a couple of
  real collectors) plus `-race ./plugin/agent/jobmgr/`, so a framework change is
  proven against real users, not just its own package.
- Reproduce a reviewer-reported bug as a FAILING test first, then fix to green
  (repo-wide working-style rule).

## Batching And Review

- Changes SHOULD stay atomic. If a collector or framework task grows, split it
  into coherent batches before review becomes difficult.
- At every batch boundary, you MUST re-evaluate clean end state and scope. If
  the branch now contains independent work, pause and submit that work
  separately or defer it before continuing.
- Changes MUST NOT mix framework changes, collector migrations, and
  integration-doc regeneration unless they are required for one coherent
  behavior change.
- Multi-round review: checkpoint-commit each validated change before its review and
  squash at PR time; if findings keep clustering in one subsystem (~2-3 rounds),
  stop patching and fix the class (repo-wide review rules).
- For Go test style, follow the repo-root `AGENTS.md` "Go test style" section.
