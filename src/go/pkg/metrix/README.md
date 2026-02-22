# metrix

`metrix` is the metrics storage and read API used by go.d `ModuleV2` collectors and runtime/internal components.

**Audience**: `ModuleV2` collector authors and framework contributors.

**See also**: [charttpl](/src/go/plugin/go.d/agent/charttpl/README.md) (template DSL),
[chartengine](/src/go/plugin/go.d/agent/chartengine/README.md) (compile + plan).

## Purpose

| Consumer                         | Store type       | Typical usage                                              |
|----------------------------------|------------------|------------------------------------------------------------|
| Collector jobs (`ModuleV2`)      | `CollectorStore` | Cycle-scoped writes, snapshot reads, chart planning input  |
| Internal/runtime instrumentation | `RuntimeStore`   | Stateful immediate-commit writes, runtime metrics planning |

## Core Concepts

- **Immutable reads** — Readers observe immutable snapshots that are swapped atomically on commit. Multiple goroutines can read concurrently without locking.
- **Cycle-scoped collector writes** — `CollectorStore` writes are staged between `BeginCycle` and `CommitCycleSuccess`. Nothing is visible to readers until commit.
- **Stateful runtime writes** — `RuntimeStore` writes are committed immediately (no cycle API). Each write produces a new overlay snapshot.
- **Label canonicalization** — Label maps (`map[string]string`) are sorted and encoded into a canonical key that uniquely identifies a series (metric name + labels).
- **Typed + flattened views** — Reader supports canonical typed families (Histogram, Summary, StateSet) and a flattened scalar view where complex types are projected into individual scalar series.

## Key Definitions

- **Freshness** controls which series appear in non-raw reads.
  `FreshnessCycle` = series must be observed in the latest successful cycle to be visible.
  `FreshnessCommitted` = series is visible as long as it's committed, even if not re-observed.
- **Window** controls how stateful histogram/summary instruments accumulate observations.
  `WindowCumulative` = observations accumulate across cycles.
  `WindowCycle` = observations reset each cycle.

## Stores and Interfaces

| Interface           | Key methods                                  | Notes                                   |
|---------------------|----------------------------------------------|-----------------------------------------|
| `CollectorStore`    | `Read(...)`, `Write()`                       | Default collector-facing store          |
| `RuntimeStore`      | `Read(...)`, `Write()`                       | Stateful-only writes                    |
| `CycleManagedStore` | `CycleController()`                          | Runtime/orchestrator-only cycle control |
| `Reader`            | `Value/Delta/Histogram/Summary/StateSet/...` | Immutable snapshot read API             |

## Write Model

### Collector store

| Phase   | Action                                                                                        |
|---------|-----------------------------------------------------------------------------------------------|
| Begin   | Open staged frame (`BeginCycle`)                                                              |
| Collect | Collector writes metrics through `Write().SnapshotMeter(...)` or `Write().StatefulMeter(...)` |
| Success | `CommitCycleSuccess` publishes new snapshot and advances success sequence                     |
| Failure | `AbortCycle` drops staged writes                                                              |

`ModuleV2` collectors should write metrics only; cycle control is handled by job runtime.

### Runtime store

- **No cycle API** — writes commit immediately.
- **Stateful only** — snapshot-mode instrument registration returns an error.

> [!CAUTION]
> Calling snapshot-mode record methods (`ObserveTotal`, `ObservePoint`) on a `RuntimeStore` **panics**.

- **Fixed freshness** — runtime store enforces `FreshnessCommitted` semantics; other freshness policies are rejected.

## Instrument Modes and Defaults

| Mode     | Typical meter        | Freshness default    | Window default     |
|----------|----------------------|----------------------|--------------------|
| Snapshot | `SnapshotMeter(...)` | `FreshnessCycle`     | `WindowCumulative` |
| Stateful | `StatefulMeter(...)` | `FreshnessCommitted` | `WindowCumulative` |

## Instrument Options

| Option                                                          | Scope                                                                                       |
|-----------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| `WithFreshness(...)`                                            | Freshness policy override (subject to mode constraints)                                     |
| `WithWindow(...)`                                               | Stateful histogram/summary window mode                                                      |
| `WithHistogramBounds(...)`                                      | Histogram bucket boundaries                                                                 |
| `WithSummaryQuantiles(...)`                                     | Summary quantile output (required for quantile series in flattened view)                    |
| `WithSummaryReservoirSize(...)`                                 | Stateful summary estimator size                                                             |
| `WithStateSetStates(...)`                                       | StateSet allowed states                                                                     |
| `WithStateSetMode(...)`                                         | `ModeBitSet` (multiple simultaneous active states) or `ModeEnum` (exactly one active state) |
| `WithDescription(...)`, `WithChartFamily(...)`, `WithUnit(...)` | Metric metadata hints for downstream consumers (e.g., autogen)                              |

## Read Modes

`Read(...)` accepts option functions that control two independent axes:

- **Raw** (`ReadRaw()`) — bypasses freshness filtering, returning all committed series regardless of when they were last observed.
- **Flatten** (`ReadFlatten()`) — projects complex types (Histogram, Summary, StateSet) into individual scalar series.

| Read options                     | Visibility           | Shape                    |
|----------------------------------|----------------------|--------------------------|
| `Read()`                         | Freshness-filtered   | Canonical typed families |
| `Read(ReadRaw())`                | All committed series | Canonical typed families |
| `Read(ReadFlatten())`            | Freshness-filtered   | Flattened scalar view    |
| `Read(ReadRaw(), ReadFlatten())` | All committed series | Flattened scalar view    |

## Flattened View Mapping

`Read(ReadFlatten())` projects non-scalar families into scalar series:

| Source kind | Flattened outputs                                                                                                |
|-------------|------------------------------------------------------------------------------------------------------------------|
| Histogram   | `<name>_bucket{le=...}`, `<name>_count`, `<name>_sum`                                                            |
| Summary     | `<name>_count`, `<name>_sum` (always); `<name>{quantile=...}` (only when `WithSummaryQuantiles()` is configured) |
| StateSet    | `<name>{<name>=state}` with scalar 0/1 values                                                                    |

Flatten metadata is exposed via `SeriesMeta.Kind`, `SeriesMeta.SourceKind`, and `SeriesMeta.FlattenRole`.

## Minimal Usage Snippets

### Collector write path

```go
store := metrix.NewCollectorStore()
meter := store.Write().SnapshotMeter("mysql")
qps := meter.Counter("queries_total")
qps.ObserveTotal(42)
```

### Read path for planning

```go
reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
value, ok := reader.Value("mysql.queries_total", nil)
_ = value
_ = ok
```

For a complete collector integration pattern (cycle management, error handling),
see [how-to-write-a-module.md](/src/go/plugin/go.d/docs/how-to-write-a-module.md).

## Contracts and Pitfalls

- **Label sets** — `LabelSet` is store-owned; do not share between different stores.
- **Counter deltas** — `Delta()` requires contiguous sequence (N, N+1).
  In `CollectorStore` this is per-cycle: missing one successful cycle breaks the delta.
  In `RuntimeStore` this is per-series per-write: the sequence always increments on each write, so skipping a write cycle does not break deltas.
- **Snapshot freshness** — Snapshot-mode instruments cannot use `FreshnessCommitted`.
- **Runtime writes** — `RuntimeStore` rejects snapshot-mode instrument registration with an error.
  Calling snapshot-mode record methods (`ObserveTotal`, `ObservePoint`) **panics**.
- **Window/freshness coupling** — Stateful histogram/summary with `WindowCycle` requires (and silently forces) `FreshnessCycle`. Setting an explicit non-Cycle freshness with `WindowCycle` returns an error.
- **Schema stability** — Re-registering an existing metric name with different kind/mode/schema returns an error (or panics in strict runtime paths).
- **Collector retention** — `CollectorStore` evicts series not seen for 10 successful cycles by default.

## Internal Architecture Notes

| Area             | Implementation pattern                                           |
|------------------|------------------------------------------------------------------|
| Snapshot publish | Read snapshots are immutable and atomically swapped              |
| Collector commit | Staged frame merges into new snapshot on successful cycle commit |
| Runtime commit   | Overlay/compaction strategy with retention pruning               |
| Iteration        | Name-indexed deterministic iteration for reader traversal        |
| Identity         | Canonical metric+labels key with stable `SeriesIdentity` hash    |
