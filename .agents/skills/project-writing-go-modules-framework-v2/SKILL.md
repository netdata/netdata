---
name: project-writing-go-modules-framework-v2
description: Use when creating or migrating a Go go.d collector to framework V2, touching CollectorV2, metrix.CollectorStore, ChartTemplateYAML/charts.yaml, charttpl/chartengine, V2 host scopes/vnodes, or V2 collector tests. Focuses on concise maintainer-preferred V2 collector patterns.
---

# Writing Go go.d Modules With Framework V2

Use with `project-writing-collectors`. Keep this skill loaded for style; read
source files for evidence.

## Read First

- Contract: `src/go/plugin/framework/collectorapi/collector.go`
- Framework-change workflow:
  `src/go/plugin/framework/docs/changing-framework-code.md`
- Canonical new-collector guide:
  `src/go/plugin/go.d/docs/how-to-write-a-collector.md`
- Helper-package guide:
  `src/go/plugin/go.d/docs/helper-packages.md`
- V1-to-V2 migration guide:
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`
- Runtime/chart lifecycle: `src/go/plugin/framework/chartengine/README.md`
- Template format: `src/go/plugin/framework/charttpl/README.md`
- Host scopes/vnodes: `.agents/skills/project-writing-go-modules-framework-v2/go-v2-host-scope.md`
- Primary modern example: `src/go/plugin/go.d/collector/cato_networks/`.
  Use focused pieces from it, not the whole collector shape.
- Older V2 collectors can still be useful for local patterns, but review them
  for stale style before treating them as examples.

## Decision Discipline

- You MUST aim for the clean end state, not the smallest collector diff. If a
  framework capability is missing and the problem is general, design the
  framework change instead of hiding the issue in collector-local glue.
- If any framework-scope package changes, stop and satisfy
  `src/go/plugin/framework/docs/changing-framework-code.md` before writing code.
- You MUST re-check scope after each coherent batch. If the work reveals an
  independent collector cleanup, framework fix, or integration-doc change,
  either defer it explicitly or land it separately before continuing.

## Core Style

- New collectors MUST implement `collectorapi.CollectorV2` from
  `src/go/plugin/framework/collectorapi/collector.go` and register via
  `CreateV2`.
- `New()` SHOULD own defaults, `metrix.NewCollectorStore()`, typed metric
  instruments, and test seams.
- V2 collectors MUST write metrics through `metrix.CollectorStore` during
  `Collect()` and provide chart template YAML through `ChartTemplateYAML()`;
  embedded `charts.yaml` is RECOMMENDED.
- `Collect(ctx)` MUST return `error` and write metrics to `metrix`; it MUST NOT
  return a V1 `map[string]int64`.
- Long-running side-effect loops that must start only with the running job MAY
  implement optional `collectorapi.CollectorV2Runner`. `Run(ctx)` MUST return
  promptly after cancellation. Do not start operational polling from `Init()` or
  `Check()`, because DynCfg `test` and autodetection use those methods without
  starting the runtime job.
- Collector `Cleanup(ctx)` MUST be idempotent. The framework may call it more
  than once, including after partial `Init` / `Check` setup.
- Files SHOULD stay boring: public lifecycle methods in `collector.go`, setup
  helpers in `init.go` when needed, orchestration in `collect.go`, distinct
  upstream operations in `collect_<operation>.go`, metrics in `metrix.go` /
  `write_metrics.go`, focused tests.
- Before adding custom HTTP, selector, logging, command-execution, SQL, ping,
  or log-file plumbing, check `src/go/plugin/go.d/docs/helper-packages.md` and
  reuse an existing helper when it fits.
- If Functions exist, isolate them in a `<name>func/` subpackage with a narrow
  `Deps` interface declared there. The Function package MUST NOT import the
  collector package or hold `*Collector`.
- If a single-instance collector exposes `Creator.SharedFunctions`, its
  `MethodHandler(job)` receives the running canonical runtime job and the public
  Function shape has no `__job` parameter. Use `job.Collector()` to bind the
  Function handler to collector-owned state; do not add a package-global
  registry to bridge Function dispatch. The Function is still job-backed:
  publication waits for the canonical job to be running and available, and
  dispatch rejects unavailable jobs before calling `MethodHandler`.
- Shared and instance job-backed Functions are published only while their
  backing running jobs are available. By default, every running job is available
  for every shared or instance Function. If a collector needs runtime readiness
  gating per job-backed Function, implement `collectorapi.FunctionAvailability`;
  keep `FunctionAvailable(functionID)` cheap and non-blocking.
  `funcapi.FunctionConfig.Available` applies to `AgentFunctions`, not
  job-backed `SharedFunctions` or `InstanceFunctions`.
- `collectorapi.Creator.InstancePolicy` defaults to
  `InstancePolicyPerJob`. Use `InstancePolicySingle` only for collectors that
  are intentionally one canonical job per agent. Single-instance configs MUST
  use `name == module` after defaults are applied. DynCfg exposes opted-in
  single-instance configs as `single` objects with the module-level collector
  config ID, no collector template, and no `add`/`remove`; updates target that
  single object.
- Before opting in a production collector to `InstancePolicySingle`, decide how
  its initial `single` object appears in DynCfg. The framework exposes a single
  object only after a config exists; it does not publish a template placeholder,
  and plain stock enable failures can remove the stock object.
- Public config options SHOULD stay small and justified. A proposed config
  option MUST name the concrete operator decision it enables; "operators may
  want to tune it" is not enough. Internal tuning SHOULD use constants unless
  the operator has a real decision to make.

## Metrics And Charts

- Instruments SHOULD be created once when the metric surface is known.
- Use `store.Write().SnapshotMeter("")` for normal metrics.
- Use `Vec(...)` for labels, `Gauge` for current values,
  `Counter.ObserveTotal()` for source counters, and `StateSet` for fixed
  one-active-state values.
- Metric names MUST be stable and selected by `charts.yaml`.
- In `charts.yaml`: use `version: v1`, `context_namespace`, `instances.by_labels`,
  `label_promotion`, and an explicit `algorithm` on every chart:
  `incremental` for counters and `absolute` for gauges.
- `Counter.ObserveTotal()` only records monotonic values in `metrix`; it does
  NOT set the chart `DIMENSION` algorithm by itself. Every chart that presents
  a counter as a rate MUST explicitly set `algorithm: incremental`, including
  dynamically built `charttpl.Chart` values.
- Do NOT rely on chartengine's metric-name suffix inference for generated
  Netdata metrics. Suffix inference is only a fallback and MUST NOT be used as
  the correctness mechanism for V2 collector charts.
- Put multipliers, divisors, hidden flags, and float formatting in the chart
  template, not ad hoc chart-emission code.
- `metrix` registers a descriptor per metric NAME permanently (no unregister), so
  re-registering a name with a changed kind, summary quantile set, or histogram
  bounds PANICS. When a name's contract can drift across cycles, keep the per-name
  handle for the job lifetime and SKIP a drifted series instead of re-registering.
- To reproduce a V1 chart context in a migration, inject `context_namespace` (the
  fixed prefix, or `prefix.<app>` per job) so autogen rebuilds `prefix.<metric>` /
  `prefix.<app>.<metric>` without hand-built chart IDs.
- When a collector builds its chart template at RUNTIME (not a static `charts.yaml`):
  - Emit it with `charttpl.Spec.MarshalTemplate()` (runs `Validate()` only, then
    marshals with `yaml.v2`, the decoder's library). Do NOT hand-roll `Validate()` +
    `yaml.Marshal`, and do NOT marshal with `yaml.v3`.
  - If you mutate a `charttpl.Group` borrowed from a shared profile/catalog, deep-copy
    it first with `Group.Clone()` so per-job edits cannot corrupt the shared template.
    A `Group` you decoded yourself per job is already owned and needs no clone.
- Skip empty distributions -- e.g. a summary whose every quantile is NaN -- so a
  chart waits for real data, matching how scalar NaN values are already skipped.
- For dynamic surfaces whose label sets churn, `metrix`'s `Vec` handle cache is
  unbounded; cache per-series instruments yourself and evict handles unseen for N
  cycles to stay bounded. Prefer a framework fix if the need is general
  (Decision Discipline).

## Compatibility Rules

- For V1-to-V2 migrations, start with
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`.

### Migration Hard Stops

- A collector using V1 chart `Vars` is blocked until framework support, an
  approved equivalent design, or explicit breaking-alert approval exists.
- `collecttest.AssertChartCoverage` is not chart-identity parity; it cannot
  prove old chart IDs, family, priority, lifecycle, labels, or alert variables.
- A finished migration MUST pass an import/runtime-path audit proving no V1
  collection path or V1 map-to-`metrix` bridge remains reachable from normal
  execution.

- Temporary V1-to-V2 parity bridges MAY be used during development, but the
  finished collector MUST NOT keep a runtime V1 map-to-`metrix` bridge.
- For migrations, first create a compatibility manifest covering chart IDs,
  contexts, dimension IDs/names, labels, config keys, DynCfg schema keys,
  stock config, alerts, docs, and lifecycle behavior.
- Migrations MUST preserve existing public contracts unless the SOW records an
  explicit breaking decision.
- Migrations MUST keep old YAML/JSON field names. Add new config as opt-in when
  cardinality, cost, or user-visible identity could surprise existing users.
- Collector integration artifacts MUST follow
  `.agents/skills/integrations-lifecycle/consistency.md`; do not preserve a
  partial local artifact checklist in V2 collector work.
- MUST NOT log raw secrets, DSNs, bearer tokens, or URLs with embedded
  credentials.

## Hot-Path Logging

- Collectors MUST NOT emit `Warningf`/`Errorf` every collection cycle for a
  recoverable partial failure. Use the built-in logger limiter:
  `c.Limit("collector:stable-operation-key", 1, time.Hour).Warningf(...)`.
- Limiter keys MUST be stable and low-cardinality. Use operation names, not
  entity IDs, labels, URLs, raw errors, or user-controlled values.
- `Once()` is reset by `JobV2.runOnce()`, so it is useful inside one cycle only;
  it is not cross-cycle spam protection.
- Full collection failure SHOULD still return an error with context so the job
  retry path handles it. Limit only fail-soft warnings/errors where collection
  continues with partial or stale data.

## Host Scopes

- Host scopes SHOULD be used only after a product decision says the data belongs
  on a generated vnode.
- `ScopeKey` and `GUID` MUST be deterministic.
- Collector-generated vnodes MUST set `_vnode_type=<source>`.
- Host-scope cardinality MUST be bounded and documented. Collectors SHOULD NOT
  create VM/disk/NIC/path/sensor scopes by default.
- Scope identity MUST use stable IDs. Human-readable names SHOULD be hostnames
  or promoted labels only.

## Tests

At minimum, V2 work MUST include these tests, or the PR/SOW MUST justify why a
specific item does not apply:

- config YAML/JSON serialization compatibility;
- `Init`, `Check`, `Collect`, and `Cleanup` lifecycle coverage;
- explicit metric-store cycle tests with `BeginCycle`, success commit, and abort
  on expected collection errors;
- chart-template schema/decode/validate/compile coverage;
- chart coverage assertions for fixtures expected to materialize all dimensions;
- host-scope tests when scopes/vnodes are used.

## Pre-PR Check

- A finished V1-to-V2 migration MUST NOT keep a runtime
  `map[string]int64` collection path or V1 map-to-`metrix` bridge.
- The PR description or design note MUST enumerate affected collector
  consistency artifacts and justify every artifact that did not need a matching
  change. SHOULD-level exceptions and escape hatches MUST be reviewer-visible.
- Existing public chart/metric/config identity MUST be preserved unless the SOW
  records an explicit breaking decision.
- New labels and scopes MUST be bounded and documented.
- Enrichment SHOULD be split from the V2 compatibility migration when possible.
