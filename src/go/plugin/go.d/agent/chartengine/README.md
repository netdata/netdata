# chartengine

`chartengine` compiles chart templates and builds deterministic chart plans (`create`, `update`, `remove`) from `metrix.Reader` snapshots.

**Audience**: `ModuleV2` collector authors and framework contributors.

**See also**: [charttpl](/src/go/plugin/go.d/agent/charttpl/README.md) (template DSL),
[metrix](/src/go/pkg/metrix/README.md) (metrics storage and read API).

## Purpose

| Stage                 | Responsibility                            |
|-----------------------|-------------------------------------------|
| `charttpl`            | Template decode/defaults/validation       |
| `chartengine.Compile` | Build immutable program IR                |
| `Engine.BuildPlan`    | Produce plan actions from metric snapshot |
| `chartemit.ApplyPlan` | Emit plan to Netdata wire protocol        |

## Collector-Facing Contract

For `ModuleV2` collectors, the runtime integration expects:

| Requirement                                                               | Why it matters                                                      |
|---------------------------------------------------------------------------|---------------------------------------------------------------------|
| `MetricStore()` returns `metrix.CollectorStore` (cycle-managed)           | Job runtime controls cycle boundaries and success/failure semantics |
| `ChartTemplateYAML()` returns valid `charttpl` YAML                       | Loaded once at autodetection/post-check                             |
| Collector writes metrics during `Collect()` only                          | Planner runs after a successful cycle commit                        |
| Metric names used in template selectors are present in group metric scope | Compile/validate consistency                                        |

## Public API Surface

| API                                                 | Purpose                                                                                                                                                                 |
|-----------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `New(opts...)`                                      | Create engine with policy/runtime options                                                                                                                               |
| `Load(spec, revision)` / `LoadYAML(data, revision)` | Compile and publish program revision                                                                                                                                    |
| `BuildPlan(reader)`                                 | Build deterministic action plan from reader snapshot                                                                                                                    |
| `RuntimeStore()`                                    | Access chartengine internal runtime metrics store                                                                                                                       |
| `WithEnginePolicy(...)`                             | Configure selector + autogen behavior                                                                                                                                   |
| `WithRuntimeStore(...)`                             | Override/disable self-metrics store                                                                                                                                     |
| `WithSeriesSelectionAllVisible()`                   | Process all visible series instead of filtering to latest successful collect cycle. Intended for runtime/internal stores that commit immediately (no cycle boundaries). |

## End-to-End Example (Single Flow)

```go
// 1) Collector writes metrics.
store := metrix.NewCollectorStore()
meter := store.Write().SnapshotMeter("app")
meter.Counter("requests_total").ObserveTotal(100)

// 2) Engine loads chart template.
engine, err := chartengine.New(
    chartengine.WithEnginePolicy(chartengine.EnginePolicy{
        Autogen: chartengine.AutogenPolicy{Enabled: false},
    }),
)
// handle err

err = engine.LoadYAML([]byte(`
version: v1
groups:
  - family: App
    metrics: [app.requests_total]
    charts:
      - id: requests
        title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: app.requests_total
            name: requests
`), 1)
// handle err

// 3) Build plan from flattened+raw reader and emit.
// ReadFlatten() is included even for templates with static dimensions
// because it is required for inferred dimensions and is the standard pattern.
plan, err := engine.BuildPlan(store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
// handle err

err = chartemit.ApplyPlan(api, plan, chartemit.EmitEnv{
TypeID:      "plugin.job",
UpdateEvery: 1,
Plugin:      "example",
Module:      "example",
JobName:     "example",
})
// handle err
```

## BuildPlan Lifecycle

`BuildPlan` executes a deterministic phase pipeline.
Terms like "materialized state" and "route cache" are defined in the Engine State section below.

| Phase           | Summary                                                                                        |
|-----------------|------------------------------------------------------------------------------------------------|
| Prepare         | Resolve program/index/cache/materialized state                                                 |
| Validate reader | Ensure flattened metadata is available when inferred dimensions are used                       |
| Scan            | Iterate series, filter by success sequence and selector, route to chart/dimension accumulators |
| Cache retain    | Prune route cache entries not seen in the latest successful sequence                           |
| Lifecycle caps  | Enforce chart/dimension cap policy                                                             |
| Materialize     | Emit create/update actions from accumulated state                                              |
| Expiry          | Emit removals for stale charts/dimensions                                                      |
| Sort            | Deterministically sort inferred dimension output                                               |

## Reader Requirements

| Scenario                                                   | Required reader mode                               |
|------------------------------------------------------------|----------------------------------------------------|
| Static named dimensions only                               | `Read(...)` is sufficient (no flatten needed)      |
| Inferred dimensions (`name` and `name_from_label` omitted) | Must use flattened reader metadata (`ReadFlatten`) |
| Runtime/default `ModuleV2` path                            | `Read(ReadRaw(), ReadFlatten())`                   |

If inferred dimensions are present without flattened reader metadata, `BuildPlan` returns an explicit error.

## Action Semantics

| Action                  | Meaning                                                                |
|-------------------------|------------------------------------------------------------------------|
| `CreateChartAction`     | Materialize chart instance (with chart metadata and labels)            |
| `CreateDimensionAction` | Materialize dimension for a chart                                      |
| `UpdateChartAction`     | Emit chart values for current cycle; unseen dims become `IsEmpty=true` |
| `RemoveDimensionAction` | Obsolete one dimension                                                 |
| `RemoveChartAction`     | Obsolete one chart                                                     |

`chartemit` normalizes emitted action order by phase:

1. create chart/dimensions
2. update values
3. remove dimensions/charts

## Routing and Collision Rules

Each metric series is routed to a chart and dimension based on template selectors.
The following rules apply when routing conflicts arise:

| Rule                                          | Behavior                                                                                       |
|-----------------------------------------------|------------------------------------------------------------------------------------------------|
| Template vs autogen chart ID collision        | Template wins; autogen chart is replaced                                                       |
| Cross-template chart ID collision             | Existing owner keeps ownership; subsequent series are **silently ignored** (see warning below) |
| Duplicate dimension observations within build | First observed dimension metadata wins; values are reduced (summed)                            |

> [!WARNING]
> Cross-template chart ID collisions cause silent data loss — conflicting series are dropped with no error and no log entry. If metrics are missing, check for duplicate rendered chart IDs across template groups.

## Lifecycle Defaults and Policy

Default lifecycle policy when template omits lifecycle:

| Policy                           | Default        |
|----------------------------------|----------------|
| `max_instances`                  | `0` (disabled) |
| `expire_after_cycles`            | `5`            |
| `dimensions.max_dims`            | `0` (disabled) |
| `dimensions.expire_after_cycles` | `0`            |

## Autogen Notes

| Topic                 | Behavior                                                                                                                                       |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| Trigger               | Unmatched series only when autogen is enabled                                                                                                  |
| Metric metadata usage | Uses `metrix.MetricMeta` hints for title/family/unit where allowed                                                                             |
| Type ID budget        | Enforced via `AutogenPolicy.MaxTypeIDLen` (`type.id` length guard)                                                                             |
| Lifecycle             | Autogen applies `ExpireAfterSuccessCycles` to **both** chart and dimension expiry (unlike template lifecycle where they default independently) |

## Runtime Metrics

`chartengine` self-instruments to a runtime store by default (disable with `WithRuntimeStore(nil)`).

| Family                    | Examples                                                       |
|---------------------------|----------------------------------------------------------------|
| `ChartEngine/Build`       | build success/error/skipped counters, build duration summaries |
| `ChartEngine/Actions`     | action counters by kind                                        |
| `ChartEngine/Series`      | scanned/matched/filtered series counters                       |
| `ChartEngine/Route Cache` | hit/miss/entries/prune counters                                |
| `ChartEngine/Lifecycle`   | removal counters by scope/reason                               |
| `ChartEngine/Plan`        | gauges for chart instances/inferred dimensions                 |

## Engine State

| Area               | Design                                                                                                                                      |
|--------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| Program            | Immutable compiled IR per revision                                                                                                          |
| Engine state       | Serialized under `Engine.mu` for load/build transitions                                                                                     |
| Route cache        | Series identity + revision keyed cache; retained by successful sequence, pruned on each build                                               |
| Materialized state | Tracks existing chart/dimension instances for incremental create/update/remove decisions; persists across cycles, resets on template reload |
| Determinism        | Sorted chart IDs and inferred dimensions provide stable action ordering                                                                     |
