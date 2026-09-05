# Go Area Instructions

This file routes Go-specific work under `src/go/`. The repo-root `AGENTS.md` applies in full; where the two
conflict, this more specific file wins. A more specific
`AGENTS.md` in a subdirectory (today `src/go/plugin/ibm.d/AGENTS.md`) overrides this file for that subtree only where
the two conflict; everything here that it does not contradict still applies there. Paths are repo-relative unless
they are Go commands or Go package paths, which are relative to this tree's module root `src/go/` (its `go.mod`); run
them from there.

## Task Routing

| Work area | Start here | Notes |
|---|---|---|
| New go.d collector, or a public-contract change (option, mode, metric meaning, ownership, Functions, vnodes) | `.agents/skills/project-go-collector-design/SKILL.md`, then `src/go/plugin/go.d/docs/how-to-write-a-collector.md` | Design note in the SOW gate first; new go.d collectors use framework V2. |
| `config_schema.json` (the DynCfg form) | `.agents/skills/project-go-collector-design/config-schema.md`, then `src/plugins.d/DYNCFG.md` ("JSON Schema for Configuration UI") | Every visible property has a title and description; tabs equal `metadata.yaml` groups; secrets use `ui:widget: password`; the repo-wide `TestConfigSchemas*` rules must pass. |
| Migrating go.d V1 collector to V2 | `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` | Preserve public contracts unless a breaking change is explicitly approved. |
| go.d V2 implementation details | `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md`, `src/go/pkg/metrix/README.md`, `src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/chartengine/README.md` | Skill for maintainer style, READMEs for framework API contracts. Editing `metrix` or framework packages is framework-gated work. |
| go.d helper packages | `src/go/plugin/go.d/docs/helper-packages.md` | Check existing HTTP, config-option, matcher, logger, socket, command, SQL, ping, log-file, and cloud-auth helpers before adding custom plumbing. |
| Collector design across plugins | `.agents/skills/project-writing-collectors/SKILL.md` | NIDL, cardinality, obsoletion, missing data, logging, config discipline. |
| `metadata.yaml` content (what the integration page says) | `.agents/skills/project-collector-metadata/SKILL.md` | One contract per field; metric, option, and alert rows mirror the code; an empty default-behavior field renders a placeholder claim that MUST be true. |
| Integration pipeline, taxonomy, generated docs | `.agents/skills/integrations-lifecycle/SKILL.md`, `.agents/skills/integrations-lifecycle/consistency.md` | Source and generated artifacts MUST stay synchronized. |
| IBM.d work | `src/go/plugin/ibm.d/AGENTS.md` | Generator-driven workflow; go.d V2 layout rules MUST NOT be applied there. |
| Function handlers | `src/go/plugin/framework/functions/README.md`, `src/go/tools/functions-validation/README.md` | Collector Functions SHOULD be isolated behind narrow dependencies. |
| Topology payloads | `.agents/skills/project-create-topology/SKILL.md`, `.agents/skills/project-create-topology/topology-function-schema.md`, `src/go/pkg/topology/v1` | New producers MUST use the production `netdata.topology.v1` schema. |
| Host scopes / vnodes | `.agents/skills/project-writing-go-modules-framework-v2/go-v2-host-scope.md`, `src/go/plugin/go.d/collector/azure_monitor/` | One job emitting metrics for resources that SHOULD appear as separate Netdata nodes MUST use host scopes. |
| Matchers/selectors | `src/go/pkg/matcher/README.md` | Prefer existing matcher APIs over custom selector grammars. |
| Core framework changes | `src/go/plugin/framework/docs/changing-framework-code.md` and "Core Framework Change Gate" below | The applicable approval tier MUST be satisfied before implementation. |

## New go.d Collector Rules

- New go.d collectors MUST implement `collectorapi.CollectorV2` from
  `src/go/plugin/framework/collectorapi/collector.go` and register via `CreateV2`: metrics through
  `metrix.CollectorStore`, charts through `ChartTemplateYAML()`.
- Guidance for new collectors MUST NOT teach or copy the V1 `Collect() map[string]int64` pattern.
- Public config options SHOULD exist only for real operator decisions. Implementation tuning (page sizes, scan
  windows, retry limits, cadence) SHOULD be internal constants unless user control is clearly justified.
- New collectors MUST NOT inherit unsupported config knobs from adjacent collectors or generic templates.
- Collector-local glue that substitutes for a missing shared capability is framework-scope work: see "Core
  Framework Change Gate".
- Functions: put Function code in a dedicated `<name>func/` package behind a narrow `Deps` interface declared in
  that package. The Function package MUST NOT import the collector package or hold `*Collector`.
- Topology: producers MUST use `netdata.topology.v1` with the Go producer model in `src/go/pkg/topology/v1` and
  MUST validate payloads against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`. New producers MUST NOT use legacy
  topology payloads.
- One job emitting metrics for multiple remote resources that SHOULD appear as separate Netdata nodes MUST use V2
  host scopes/vnodes.

## go.d V1-to-V2 Migration Rules

- Migrations MUST start with `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`.
- Migrations MUST preserve chart IDs, contexts, dimensions, config keys, defaults, health lookups, metadata,
  taxonomy, stock config, and service discovery behavior unless the user explicitly approves a breaking change.
- Compatibility migration SHOULD be separate from enrichment (new labels, host scopes, topology, Functions, config
  expansion).
- A completed migration MUST NOT keep a runtime V1-to-V2 bridge. Temporary V1 logic MAY serve parity checks during
  development but MUST be removed from the final collector.

## Core Framework Change Gate

Shared Go framework code (`metrix`, `chartengine`, `charttpl`, the job runtime, `collectorapi`) is
high-blast-radius: it runs in EVERY collector, most of it on the per-cycle hot path. Before changing it, read
`src/go/plugin/framework/docs/changing-framework-code.md`, the canonical owner of the framework-change scope list,
required design note, validation expectations, and artifact checks. Implementation MUST NOT begin until that guide's
applicable approval tier is satisfied.

- Clean extension over glue: when the problem is general, prefer a clean framework extension over collector-local
  globals, singletons, adapters, caches, or private-package coupling. Such glue that substitutes for a missing
  shared framework or helper capability MUST go through this gate before implementation.
- Behavior preservation: when a framework change also touches shipped collectors, preserve their observable
  behavior (chart IDs, contexts, dimensions, config keys, defaults) and validate representative consumers, not only
  the framework package (see "Validation For Go Changes").
- metrix contract: `metrix` keeps one descriptor per metric NAME, resolved atomically at commit and bounded: a name
  idle past `expireAfterSuccessCycles + descriptorGraceCycles` is evicted and re-registers cleanly afterward. A
  consumer that caches per-name state across cycles MUST couple its lifetime to the optional
  `metrix.DescriptorRetention` accessor. See `src/go/pkg/metrix/README.md` ("Descriptor Lifecycle and Retention").

## Evidence Before Complexity

Readability and maintainability are the default for Go control-plane and framework code. Complexity needs evidence;
a plausible future problem is not a requirement.

- Code MUST NOT introduce population, byte, concurrency, queue, retry, or backpressure limits, nor add accounting,
  scheduling, pooling, caching, custom data structures, or lifecycle machinery, unless the design follows from a
  concrete correctness, liveness, protocol, compatibility, or security contract, a documented scale requirement, or
  a measured production workload.
- The justification MUST identify the input or producer, the failure being prevented, the expected scale, and why
  simpler slices, maps, channels, or direct ownership are insufficient. A round number, a hypothetical abuse case,
  or a benchmark in isolation is not evidence of a product requirement.
- Protocol- and security-derived bounds MUST stay tied to their source with a concise rationale and boundary tests.
  Do not generalize one per-item bound into an aggregate process policy without separate evidence.
- When a limit has no valid requirement, remove the policy and its coupled machinery. Raising it to an effectively
  unreachable value is not the clean end state.
- Do not add allocation- or latency-oriented complexity until the path is shown hot by production frequency, a
  profile, or a representative benchmark. Once hot, follow "Hot-Path And Benchmark Discipline".
- Coupling one job to another job's state, durable state for independent reads, schedulers, and queues are answered
  by the Architecture Gate in `.agents/skills/project-go-collector-design/SKILL.md` before implementation.

## Hot-Path And Benchmark Discipline

metrix commit/collect, per-sample/per-write, and per-cycle code are hot paths: they run for every collector on
every cycle.

- Before/after REQUIRED: a hot-path change MUST include before/after `go test -bench` numbers (use `git stash` for the
  "before" baseline), not just "tests pass".
- Allocation count is the gate: assert allocs stay within the intended envelope (for example a sparse commit stays
  ~O(touched), never O(retained)). `ns/op` is a dev-machine trend indicator, NOT a CI gate; label it as such inline
  and never record a personal name in the file.
- State the complexity envelope explicitly (for example "commit is O(live-series + touched + distinct-names)") and
  prove the change introduced no O(samples), O(retained), or O(n^2) regression.
- Keep bench comments in sync with the code they measure, in the same change, with self-contained wording (no
  round or session references).

## Go Formatting

`gofmt` and `goimports` are the baseline. The repository additionally RECOMMENDS (not CI-enforced) this pipeline,
which keeps wrapping tight and keyed struct literals readable:

1. `golines -m 120 -t 4 -w <paths>`: join over-wrapped signatures/calls and split lines past ~120 columns. SKIP this
   step if `golines` is not installed (`go install github.com/segmentio/golines@latest`).
2. `go run ./tools/expandstructs <paths>`: one keyed struct-literal field per line (formats in-process with
   `go/format`).
3. `goimports -w <paths>`: order imports.

Conventions this encodes:

- A signature, call, return, or composite literal stays on ONE line when it fits within ~120 columns; wrap only when
  it does not.
- Keyed struct literals of a named type go ONE field per line.

`gofmt`/`goimports` cannot express these two rules, so they are not CI-enforced; re-run the pipeline if code drifts.
See `src/go/tools/expandstructs/README.md`.

## Validation For Go Changes

- Always: formatted per "Go Formatting" (at minimum `gofmt` clean) and `go vet ./<pkg>/` clean.
- Unit: `go test -count=1 ./<pkg>/...` for every package touched.
- Concurrency: add `-race` for concurrency-sensitive packages (`metrix`, the job runtime, `plugin/agent/jobmgr`).
- Shared framework code: ALSO build and test representative consumers (a couple of real collectors) plus
  `-race ./plugin/agent/jobmgr/`, so the change is proven against real users, not only its own package.
- Prefer `testify/require` for prerequisites and `testify/assert` for comparisons. Use `t.Fatal`/`t.Fatalf`
  directly for genuine test control flow or when no clearer assertion exists. Test shape: root `AGENTS.md`, "Go
  Test Style".

## Go Review And Reachability

- Start review at the declared interface and the shipped production adapter. Inspect implementation internals
  behind that boundary only when the contract leaves relevant behavior unspecified or concrete evidence points
  across it. Do not make a caller responsible for arbitrary private behavior of every implementation.
- Before deleting Go code as unused, check every reference form: selector reads and writes, struct literals,
  constructor arguments, method values, interface satisfaction, reflection, build-tagged and platform files,
  generated code, and test-only injection. A call-site-only search is not proof of unreachability.

## Batching

- Changes SHOULD stay atomic. If a collector or framework task grows, split it into coherent batches before review
  becomes difficult.
- Treat every batch boundary as an additional Re-evaluation checkpoint (root `AGENTS.md`, "Clean End State Over Less
  Churn"): you MUST re-evaluate clean end state and scope there. If a separate framework fix, collector cleanup, or docs
  rewrite has become necessary, split it into its own step or submit it independently before continuing.
- Changes MUST NOT mix framework changes, collector migrations, and integration-doc regeneration unless one coherent
  behavior change requires them.
