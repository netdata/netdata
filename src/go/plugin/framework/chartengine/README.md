# chartengine

`chartengine` compiles chart templates and builds deterministic chart plans (`create`, `update`, `remove`) from `metrix.Reader` snapshots.

**Audience**: `CollectorV2` collector authors and framework contributors.

**See also**: [charttpl](/src/go/plugin/framework/charttpl/README.md) (template DSL),
[metrix](/src/go/pkg/metrix/README.md) (metrics storage and read API).

## Purpose

| Stage                 | Responsibility                                           |
|-----------------------|----------------------------------------------------------|
| `charttpl`            | Template decode/defaults/validation                      |
| `chartengine.Compile` | Build immutable program IR                               |
| `Engine.PreparePlan`  | Prepare plan actions plus explicit commit/abort boundary |
| `chartemit.ApplyPlan` | Emit plan to Netdata wire protocol                       |

## Collector-Facing Contract

For `CollectorV2` collectors, the runtime integration expects:

| Requirement                                                               | Why it matters                                                      |
|---------------------------------------------------------------------------|---------------------------------------------------------------------|
| `MetricStore()` returns `metrix.CollectorStore` (cycle-managed)           | Job runtime controls cycle boundaries and success/failure semantics |
| Exactly one chart provider: `StaticChartTemplateProvider` or `ChartTemplateSetProvider` | Static YAML is captured after Check; native sets can change with Collect |
| Collector writes metrics during `Collect()` only                          | Planner runs after a successful cycle commit                        |
| Metric names used in template selectors are present in group metric scope | Compile/validate consistency                                        |

## Public API Surface

| API                                                 | Purpose                                                                                                                                                                 |
|-----------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `New(opts...)`                                      | Create engine with policy/runtime options                                                                                                                               |
| `Load(spec, revision)` / `LoadYAML(data, revision)` | Compile and publish program revision                                                                                                                                    |
| `NewTemplateSet(spec)` / `NewTemplateSetYAML(data)` | Prepare an immutable named native set or a compatible singleton document |
| `PrepareTemplateSet(set, opts...)` | Bind fixed global job overrides without recompiling entries |
| `PreparePlanWithOptions(reader, PlanOptions{...})` | Stage a desired set and optional host reset with the plan |
| `TemplateSet.Entries()` / `GlobalPolicy()` | Inspect detached definitions and effective global policy |
| `TemplateSet.ChartTemplateIDAt(entryID, path, i)` / `FallbackContextNamespace()` | Resolve an entry chart position to the template ID plans and route facts report; read the fixed automatic-chart namespace |
| `PreparePlan(reader)`                               | Build deterministic action plan from reader snapshot and return an explicit attempt                                                                                     |
| `RuntimeStore()`                                    | Access chartengine internal runtime metrics store                                                                                                                       |
| `WithEnginePolicy(...)`                             | Configure selector + autogen behavior                                                                                                                                   |
| `WithRuntimeStore(...)`                             | Override/disable self-metrics store                                                                                                                                     |
| `WithSeriesSelectionAllVisible()`                   | Process all visible series instead of filtering to latest successful collect cycle. Intended for runtime/internal stores that commit immediately (no cycle boundaries). |
| `WithEmitTypeIDBudgetPrefix(...)`                   | Set the effective type-id prefix used by autogen budget checks and collision-warning rate limiting                                                                      |
| `WithRuntimePlannerMode(...)`                       | Enable runtime planner mode with no-write-tick semantics, for jobs/tests that drive planning directly from runtime metrics instead of collect-cycle boundaries.         |
| `WithPlanRouteDiagnosticObserver(...)`              | Stream complete, synchronous route facts for one plan attempt; intended for validation and tests                                                                        |
| `ChartTemplateIDAt(...)`                            | Correlate an authored chart position with the compiler-assigned template identity used by route facts                                                                   |
| `ResolveInstanceLabelPolicy(...)`                   | Inspect `instances.by_labels` through the same parsing and exclusion-precedence rules used by the planner                                                               |

## Named Active Template Sets

A collector with changing template membership implements
`collectorapi.ChartTemplateSetProvider.ChartTemplateSet() *chartengine.TemplateSet`.
Build the snapshot where errors can be handled, normally during `Init` or `Collect`:

```go
set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
    Entries: []chartengine.TemplateEntry{{
        ID: "application",
        ContextNamespace: "myapp",
        Groups: groups,
    }},
    FallbackContextNamespace: "myapp",
    Policy: chartengine.EnginePolicy{
        Autogen: &chartengine.AutogenPolicy{Enabled: true},
    },
})
// Handle err, then retain set for the provider getter.
```

Construction deep-copies native definitions, applies the same inherited defaults as YAML, validates and compiles.
The getter returns the same immutable pointer until desired content changes. An empty set is valid; nil and a zero-value
`TemplateSet` are invalid. `Entries()` and `GlobalPolicy()` return detached data for tooling. Applicability and source
preprocessing remain collector responsibilities; the framework combines the selected entries and shared fallback.

The getter runs on the job goroutine, serialized with `Collect`. Keeping the stored snapshot pointer owned by that
same goroutine needs no additional synchronization. If `Run` or another goroutine shares the pointer across
replacements, synchronize pointer reads and writes.

Entry IDs identify independently owned definitions within a job. A new snapshot may reuse an ID. Equal normalized
content preserves chart/dimension state and expiry timestamps, regardless of entry-list order. Equality uses compiled
metadata and defaults, local chart paths, group structure, declared metrics and entry restrictions; it does not infer
mathematical equivalence between differently written selectors. A changed entry is replaced as a whole: all its old
reservations are released, observed charts recreate, quiet siblings retire, and Go expiry bookkeeping restarts.

Ownership is independent of routing precedence. When one series routes to several templates that render the same
unowned chart ID, native sets rank those routes by compile order: entry order, then depth-first authored order within
the entry, where a group's charts precede its nested groups. Adding or removing an entry never changes the relative order
of two others. YAML documents keep comparing positional `g<path>.c<index>` IDs as strings. Across different series, the
first series in scan order (metric name, then series) claims a new chart, and an existing owner keeps it. Reordering can
change future unowned winners but preserves current active and quiet incumbents. Authored charts retain precedence over
fallback. Replacement retirement compares
old public identities with final survivors; a surviving chart or dimension is never also marked obsolete, and dimension
retirement headers use the surviving chart's metadata. Existing Agent redefinition, history, counter and context behavior
applies; this API makes no stronger continuity guarantee for changed definitions.

Dimension definitions explicitly emit `type=float` or `type=int`. Integer is the initial Agent default, but omitting
this option during redefinition preserves the existing numeric mode. Explicit `type=int` clears a prior float mode,
including when the Agent retains a dimension across a collector restart.

Global selector/autogen policy and fallback namespace are fixed for a running job. `CollectorV2EnginePolicy` is captured
once and overrides global fields. An explicit autogen override replaces the entire global autogen policy and its rules.
Entry `AutogenRules` are independent restrictions: they combine with global rules and disappear when the entry is removed.
Overrides cannot remove them. Later effective global changes require job recreation; entry content and restrictions can
change. Rules replaced by an override in a legacy YAML document do not become permanent entry restrictions.

`collectorapi.ChartTemplateSource` resolves providers and captures policy for runtime and coverage helpers. Static YAML
remains a supported singleton, preserving original group paths and diagnostics. Existing collectors may retain their
composition and cached-YAML getters; native providers require no YAML getter. Metric jobs with both or neither provider
fail startup validation. Function-only jobs retain their exemption.

Direct engine callers bind job overrides with `PrepareTemplateSet` before passing the snapshot to
`PreparePlanWithOptions`. `PlanOptions.TemplateSet` selects the desired prepared snapshot; nil keeps the loaded program.
`ResetMaterialized` stages a new host's presentation without retiring the previous host on the new host. Commit the
attempt only after complete output admission; abort preserves the previously committed program and presentation.
`Load`, `LoadYAML`, `Compile`, `ChartTemplateIDAt` and immediate `ResetMaterialized` retain their document/component
contracts. `Load` and `Compile` still expect defaults already applied; active-set transitions do not call destructive Load.

The unchanged-snapshot path adds constant bookkeeping to the existing planner. Changed snapshots compare template
content and filter retained charts once, then stage retained lifecycle state through the ordinary planner. Retirement
comparison visits only replaced or transferred chart identities and their dimensions. Programs/indexes are shared
between scopes; mutable route caches and lifecycle state remain scope-owned.

## End-to-End Example (Single Flow)

```go
// 1) Collector writes metrics.
store := metrix.NewCollectorStore()
meter := store.Write().SnapshotMeter("app")
meter.Counter("requests_total").ObserveTotal(100)

// 2) Engine loads chart template.
engine, err := chartengine.New(
	chartengine.WithEnginePolicy(chartengine.EnginePolicy{
		Autogen: &chartengine.AutogenPolicy{Enabled: false},
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

// 3) Prepare plan from flattened+raw reader, emit it, then commit.
// ReadFlatten() is included even for templates with static dimensions
// because it is required for inferred dimensions, structured-family autogen,
// and is the standard pattern.
attempt, err := engine.PreparePlan(store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
// handle err
plan := attempt.Plan()
defer attempt.Abort()

err = chartemit.ApplyPlan(api, plan, chartemit.EmitEnv{
	TypeID:      "plugin.job",
	UpdateEvery: 1,
	Plugin:      "example",
	Module:      "example",
	JobName:     "example",
})
// handle err
err = attempt.Commit()
// handle err

```

## PreparePlan Lifecycle

`PreparePlan` executes a deterministic phase pipeline.
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
| Reconcile       | Retire only old identities absent from the final presentation after replacement or ownership transfer |
| Sort            | Deterministically sort inferred dimension output                                               |

## Route Diagnostics and Policy Inspection

`WithPlanRouteDiagnosticObserver` is an opt-in validation/test surface. When enabled, the planner synchronously streams
attempt-local facts for filtering, candidate rejection, chart-identity rejection, resolved routes, accepted routes,
autogen displacement, collisions, lifecycle rejection, and unmatched series.

- Diagnostic planning bypasses route-cache lookup and storage so every scanned series produces complete facts.
- The default path is unchanged when no observer is configured; it continues to use the route cache.
- The callback MUST NOT call back into the same `Engine` and SHOULD aggregate bounded facts rather than retain every event.
- Resolved facts include the compiler template identity, series identity, rendered chart/dimension names, raw instance
  identity, ordered instance-label keys, and the label key used for a dynamic dimension name. Accepted facts additionally
  expose the effective context, family, units, algorithm, aggregation, presentation, series kind, scale, and label-promotion
  policy. They do not include the full input label set; a rendered chart or dynamic dimension name can itself be derived
  from label values and must be handled accordingly by consumers.
- `TemplateSet.ChartTemplateIDAt` resolves an entry's group/chart position to the template ID that plan actions and facts
  report, for named sets and the YAML singleton entry alike; the package `ChartTemplateIDAt` does the same for a decoded
  document. Named-set facts additionally expose `TemplateEntryID` and `LocalChartTemplateID`, corresponding fields for an
  existing collision owner, and entry/local provenance for rejected autogen rules. Consumers need not parse encoded
  ownership strings.

`ResolveInstanceLabelPolicy` is the corresponding read-only inspection helper for `instances.by_labels` and
`instances.optional_by_labels`. It returns the runtime-normalized required keys, optional keys, exclusions, and include-all
flag, so validation tooling does not reproduce compiler or planner precedence rules.

## Reader Requirements

| Scenario                                                                       | Required reader mode                                                       |
|--------------------------------------------------------------------------------|----------------------------------------------------------------------------|
| Static named dimensions only                                                   | `Read(...)` is sufficient (no flatten needed)                              |
| Inferred dimensions (`name` and `name_from_label` omitted)                     | Must use flattened reader metadata (`ReadFlatten`)                         |
| Structured autogen families (`Histogram`, `Summary`, `StateSet`, `MeasureSet`) | Must use flattened reader metadata (`ReadFlatten`) or they are not visible |
| Runtime/default `CollectorV2` path                                                | `Read(ReadRaw(), ReadFlatten())`                                           |

If inferred dimensions are present without flattened reader metadata, `PreparePlan` returns an explicit error.

## Action Semantics

| Action                  | Meaning                                                                                                                              |
|-------------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `CreateChartAction`     | Materialize chart instance (with chart metadata and labels)                                                                          |
| `CreateDimensionAction` | Materialize dimension for a chart                                                                                                    |
| `UpdateChartLabelsAction` | Re-emit chart metadata and replace chart labels                                                                                   |
| `UpdateChartAction`     | Emit chart values for current cycle; unseen dims, and dims whose value is non-finite (NaN/Inf), become `IsEmpty=true` (gap, never 0) |
| `RemoveDimensionAction` | Obsolete one dimension                                                                                                               |
| `RemoveChartAction`     | Obsolete one chart                                                                                                                   |

`chartemit` normalizes emitted action order by phase:

1. create chart/dimensions
2. update chart labels
3. update values
4. remove dimensions/charts

## Routing and Collision Rules

Each metric series is routed to a chart and dimension based on template selectors.
The following rules apply when routing conflicts arise:

| Rule                                          | Behavior                                                                                       |
|-----------------------------------------------|------------------------------------------------------------------------------------------------|
| Template vs autogen chart ID collision        | Template wins; autogen chart is replaced                                                       |
| Cross-template chart ID collision             | Existing owner keeps ownership; subsequent series are **dropped** (see warning below)          |
| Duplicate dimension observations within build | First observed dimension metadata wins; values use the chart's configured reducer            |

> [!WARNING]
> Cross-template chart ID collisions lose data: conflicting authored series are dropped. At most one warning per hour is logged per chart type-ID namespace (`WithEmitTypeIDBudgetPrefix`), so once per job across its host scopes and once per runtime metrics component, naming the dropped series-route count, the chart, its owner and the rejected template. Give every chart a unique rendered ID (`id` or `context`). Which template owns a new contested chart follows [Named Active Template Sets](#named-active-template-sets): compile-order precedence among one series' routes, scan order across series.

Authored charts can set one reducer for all their dimensions. Supported values are `sum` (default), `min`, `max`, and
`avg`. Reduction is scoped to one successful plan build and happens before multiplier/divisor and
`absolute`/`incremental` chart processing. Autogen routes retain their existing sum behavior. See the chart-template
format's [aggregation section](../charttpl/README.md#aggregation) for semantics and metric-type constraints.

### Chart Label Intersection

- Promoted non-identity labels are common metadata: a key/value survives only when it is present with the same value on
  every routed contributor to the chart.
- An unlabeled contributor participates as an empty label set. If it shares a chart with labeled contributors, no
  promoted non-identity label can describe the entire chart.
- A changed intersection emits a complete replacement label set through `UpdateChartLabelsAction`; chart identity and
  dimensions are not recreated.

Authored chart-label promotion is presence-aware. Omitted `label_promotion` uses automatic intersection, a non-empty list
uses explicit intersection over that allowlist, and an explicit empty list retains only instance identity labels. Group
defaults preserve the same three states through inheritance.

### Instance Identity Resolution

- `instances.by_labels` defines required identity. A missing explicit label rejects that series from the chart.
- `instances.optional_by_labels` defines declaration-ordered explicit keys that participate only when their value is
  present and nonblank. Missing or blank optional values retain the base/required chart identity.
- Required values render before optional key/value pairs in the chart-ID suffix. For example, present `pid="1234"`
  contributes `_pid_1234`. Every participating key is emitted as an identity chart label.
- Optional identity is resolved independently for each series. Present and absent source shapes can therefore materialize
  refined and base chart instances in the same plan; a series is routed once, never copied into an aggregate view.
- Presence/value changes are ordinary identity changes. Existing lifecycle policy expires the old chart; the engine keeps
  no migration or alias state.
- Explicit required/optional resolution reuses one compiled per-template plan and uses binary search over canonical
  sorted source labels on a route-cache miss. The existing `by_labels: ["*"]` path scans and sorts visible labels;
  wildcard identity cannot be combined with optional keys.

### Algorithm Resolution

- An omitted authored `algorithm` remains `AlgorithmAuto` in the immutable program, route cache, and chart-level action
  metadata.
- After route lookup, the planner resolves the local route's dimension algorithm from the current `SeriesMeta.Kind`:
  counters use `incremental`; gauges and every other or unknown kind use `absolute`.
- An explicit authored `absolute` or `incremental` value overrides the runtime kind for every dimension in that chart.
- Authored and autogen routes use the same runtime resolver. Metric-name suffixes do not determine chart algorithms.
- Flattened structured families use their emitted scalar `Kind`. `SourceKind` and `FlattenRole` still control chart shape
  and identity: histogram buckets/count/sum and summary count/sum are counters; summary quantiles and StateSet states are
  gauges; MeasureSet fields follow their declared semantics.
- Different kinds may coexist in distinct rendered dimensions. Series aggregated into one rendered dimension must have
  the same kind when the chart omits `algorithm`; use an explicit chart algorithm for an intentional mixed-kind collapse.
  Chartengine does not diagnose a violation at runtime, so authoring validation and real-path tests must enforce this rule;
  the resulting dimension metadata is otherwise unspecified.
- A live metric identity must keep the same kind while its dimension is materialized. Kind changes are resolved for route
  planning, but an existing Netdata dimension keeps its creation-time wire algorithm until it expires and is recreated.
  The expired-definition recovery rule below also applies when a rejected output plan delayed that recreation.

Route-cache entries are immutable discovery results. Authored and successful autogen routes share the
series-identity/revision cache. Autogen hits additionally validate current metric metadata (including its presence),
exposed kind, source kind, and flatten role before reuse. A changed descriptor rebuilds the route; template reload
clears the cache, and diagnostic planning bypasses it. Cached autogen discovery follows the same snapshot-membership
retention as authored routes and trades additional retained route/guard memory for less per-cycle construction.
Algorithm resolution happens after cache lookup, so a cached route
cannot retain the effective algorithm from an earlier series kind. Only concrete algorithms reach accumulated dimension
state and dimension actions; chart-level metadata retains the configured policy.

## Lifecycle Defaults and Policy

Default lifecycle policy when template omits lifecycle:

| Policy                           | Default        |
|----------------------------------|----------------|
| `max_instances`                  | `0` (disabled) |
| `expire_after_cycles`            | `5`            |
| `dimensions.max_dims`            | `0` (disabled) |
| `dimensions.expire_after_cycles` | `0`            |

Metric collection can advance while output plans are aborted, leaving published definitions past their expiry.
When an identity returns after it was eligible for expiry in an intervening cycle, the planner recreates its staged
definition if its settings differ from the last committed emitted definition. Ordinary observations do not advance
that definition baseline. Chart metadata re-emitted by label updates or dimension creation/removal does advance it:

- Chart settings are title, units, family, context, type and priority. An expired chart also recreates when a returning
  dimension's creation settings changed.
- Dimension settings are the resolved algorithm, effective multiplier/divisor, hidden state and float/int mode.
  An expired dimension under a live chart recreates independently; unchanged dimensions keep their definitions.
- Unchanged definitions retain the existing recovery behavior. Returning exactly at the expiry boundary remains an
  ordinary observation; continuous input with an expiry of one cycle does not recreate. Disabled expiry stays disabled.

Recreation is transactional: abort retains the prior definition and a later plan retries. Definitions precede recovered
values; only old identities absent from the final plan are marked obsolete. Chart labels retain their current membership
and promotion rules. This uses ordinary Agent redefinition behavior and does not reset stored history or preserve a
collector's lost counter baseline.

## Autogen Notes

| Topic                 | Behavior                                                                                                                                       |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| Trigger               | Unmatched series only when autogen is enabled                                                                                                  |
| Authored precedence   | All authored routes are resolved first; conditional rules and fallback apply only when none matched                                             |
| Conditional rules     | `AutogenPolicy.Rules` scope fallback selectors by source family; all applicable selectors must accept, so any rejection suppresses fallback     |
| Context namespace     | Autogen context = top-level `context_namespace` + the full metric name (which includes any `SnapshotMeter` prefix); empty namespace leaves the bare name. A non-empty meter prefix stacks after `context_namespace`, so pair `context_namespace` with `SnapshotMeter("")` to avoid a doubled prefix |
| Structured families   | Histogram/summary components use the base family and retain structural labels; StateSet keeps its name; MeasureSet fields use the source before `_<field>` |
| Metric metadata usage | Uses `metrix.MetricMeta` hints for title/family/unit where allowed                                                                             |
| Type ID budget        | Enforced via `AutogenPolicy.MaxTypeIDLen` + effective emit type-id prefix (`WithEmitTypeIDBudgetPrefix(...)`)                                  |
| Lifecycle             | Autogen applies `ExpireAfterSuccessCycles` to **both** chart and dimension expiry (unlike template lifecycle where they default independently) |

Rejected fallback series continue through collection and remain accounted as unmatched routing; no separate runtime
counter is emitted. Use an upstream selector or relabeling/filtering mechanism when data, rather than only fallback
charts, must be removed.

Histogram bucket charts use non-overlapping range bucket totals from
`metrix.ReadFlatten()` and are emitted as `heatmap` charts in both autogen and
template-driven paths. The `le` label remains the upper-bound identity for the
bucket dimension. Autogen and template-inferred bucket dimensions are named by
the bare `le` value (for example `0.005`, `0.025`, `+Inf`). Bucket dimensions
are ordered by numeric upper bound with `+Inf` last, not by lexical dimension
name.

`MeasureSet` autogen specifics:

- chartengine treats `MeasureSet` as a structured family, similar to `StateSet`, not as grouped scalar coincidence
- flattened `MeasureSet` inputs are expected to carry:
    - `SourceKind = MetricKindMeasureSet`
    - `FlattenRole = FlattenRoleMeasureSetField`
    - per-field metric names like `<name>_<field>`
    - a synthetic reserved field label (`measure_field=<field>`)
- the synthetic `measure_field` label is the authoritative field-identity channel; the per-field metric-name suffix remains for `MetricMeta(name)` compatibility
- gauge-like `MeasureSet` fields autogen with absolute algorithm behavior; counter-like `MeasureSet` fields autogen with incremental algorithm behavior
- A snapshot gauge field carrying NaN is observed but unavailable: it keeps its dimension alive and emits a gap.
  An all-NaN family still creates its chart and declared dimensions. Omitting the family follows normal lifecycle
  expiry instead. The write/read contract is in [metrix field availability](../../../pkg/metrix/README.md#field-availability).

### Reserved Flattened Label Keys

These label keys are treated specially by chartengine when consuming flattened structured-family or distribution inputs:

| Key / Pattern   | Meaning                                                                                                                |
|-----------------|------------------------------------------------------------------------------------------------------------------------|
| `le`            | Histogram range bucket upper-bound label                                                                               |
| `quantile`      | Summary quantile label                                                                                                 |
| `measure_field` | `MeasureSet` field identity label                                                                                      |
| `<metric-name>` | `StateSet` special case: the flattened state name is carried under a synthetic label whose key is the base metric name |

Notes:

- `le`, `quantile`, and `measure_field` are static reserved flattened-label keys in chartengine.
- `StateSet` is different: it does not use a global static key; it uses the base metric name itself as the synthetic flattened label key.
- These keys are part of the flatten contract between `metrix` and chartengine. Reusing them as ordinary user labels on those flattened inputs is not supported.

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

`WithRuntimeSampleObserver` also hands each build's samples to a callback; jobs that aggregate across engines pass
`WithRuntimeStore(nil)` and a `RuntimeAggregator`'s `Observe`. The aggregator flushes its rollup into a runtime store
once per cycle; when the store implements `metrix.RuntimeBatchWriter`, a flush publishes one snapshot.

## Engine State

| Area               | Design                                                                                                                                      |
|--------------------|---------------------------------------------------------------------------------------------------------------------------------------------|
| Program            | Immutable compiled IR per revision                                                                                                          |
| Engine state       | Serialized under `Engine.mu` for load/build transitions                                                                                     |
| Route cache        | Series identity + revision keyed authored/autogen discovery; autogen validates current metadata and kinds; retained by successful sequence, pruned on each build                                               |
| Materialized state | Tracks existing chart/dimension instances for incremental create/update/remove decisions; persists across cycles; direct Load resets it, active sets preserve unchanged entries |
| Attempt staging    | A build stages lifecycle changes in the materialized state in place and records the committed values it overwrites in the attempt's journal; Commit keeps the changes, Abort (or a failed build) replays the journal, so later plans only ever start from committed state. After a commit, a chart, dimension or scratch map whose deletions since its last rebuild exceed its live size is rebuilt, so capacity follows live cardinality after a spike |
| Determinism        | Sorted chart IDs and inferred dimensions provide stable action ordering                                                                     |

## Performance Validation

Steady collector-mode benchmarks must advance the successful collection sequence. Replanning a previously committed
sequence intentionally deduplicates its updates, so a repeated-snapshot benchmark must instead use runtime planner mode.
Benchmarks should verify emitted chart/dimension counts and values after warmup.

Planning work is O(scanned series + retained charts); allocations follow what changed and the plan output. An
unchanged chart allocates only its boxed update action: per-build chart state and update values come from blocks sized
by the previous build, and the abort journal records only sequence stamps and scratch values of observed charts
(transient per attempt). New or changed charts, labels and dimensions additionally allocate their definitions. Each
plan also pays a small constant setup (build context and its maps and blocks, the attempt with its inline journal, one
callback per series pass). `TestAutogenWarmPlanAllocationEnvelope` guards the steady envelope and
`TestPlanAbortRestoresCommittedState` the abort contract.

`BenchmarkAutogenMixedChurn` measures a bounded population with stable, partially replaced, or fully replaced
identities; `BenchmarkCollectExpiryRemovals` covers stable, partial, and mass expiry across chart/dimension shapes.
Metric collection and fixture setup are outside those planner/expiry timers. Expiry scans retained charts and enabled
dimensions but sorts only actual removals. Steady expiry allocates no removal memory; staging and output still scale
with retained/output cardinality. Unit allocation envelopes guard these properties; elapsed benchmark timings are
development-machine trends, not CI limits.
