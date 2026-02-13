# Phase 2: Metrics Package and Store (V2)

## TL;DR

- This file is the phase-2 metrics design contract.
- Goal: collectors emit typed metrics + labels; chart engine reads a stable store API.
- Key hard rule: counters keep two last **collected** samples (`current`, `previous`) so functions can compute deltas from real points.
- Implementation baseline (cross-phase): Go 1.25 (latest language/runtime features available).

## Scope

- Design and implement `src/go/plugin/go.d/agent/metrics`.
- Replace collector-facing `map[string]int64` / `map[string]float64` emission with typed metric writes.
- Provide metric state and query helpers needed by:
  - functions (delta on counters),
  - chart engine (selector-driven reads, lifecycle scans),
  - migration adapters (legacy collectors and parser outputs).

## Not in Scope

- Chart template syntax and template compilation (Phase 1).
- Chart create/update/remove lifecycle engine (Phase 3).
- Collector migration execution (later phases).
- OpenMetrics wire/exposition formats (text/protobuf transport details).

## Inputs and Evidence

- Existing collector/module contracts:
  - `src/go/plugin/go.d/agent/module/module.go`
  - `src/go/plugin/go.d/agent/module/job.go`
- Existing parser/model:
  - `src/go/pkg/prometheus/metric_family.go`
  - `src/go/pkg/prometheus/metric_series.go`
- Existing helper packages:
  - `src/go/plugin/go.d/pkg/metrix/*`
  - `src/go/plugin/go.d/pkg/stm/stm.go`
- Existing virtual-metrics precedent:
  - `vm.md`
  - `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/collector_vmetrics.go`
- OpenMetrics type semantics (data-model only):
  - `OpenMetrics.md`

## Resolved Decisions Included Here

- `2A`: package path is `agent/metrics` (not `agent/collect`).
- `3A`: reset-aware counter semantics (`Delta` returns `(current, true)` on reset).
- `legacy-69B`: cycle-sensitive transitions happen on successful collect cycles.
- `legacy-77A`: phase-2 starts with metrics/store before chart engine.
- `86A`: counters must keep two last collected samples for function deltas.
- `88A`: support two metric input styles in the API: scrape-like snapshot inputs and instrument-like stateful inputs.
- `89A`: superseded by `147A` (collector-owned store accessor contract).
- `90A`: superseded by `147A` (collector no longer depends on `CollectContext.Store()` for metric write path).
- `91A`: original stateful-instrument scope decision (superseded by `81B`/`132A` expanded typed non-scalar surface on snapshot+stateful meters).
- `92A`: OTel-like API shape is not a hard requirement; phase-2 interface can diverge to simplify both write and read paths.
- `93B`: phase-2 top-level API uses a unified readable+writable `CollectorStore` handle for collectors/functions, with runtime free to expose restricted internal views where needed.
- `94A`: concurrency model is hybrid: single-writer mutable store plus atomically swapped immutable read snapshots.
- `95A`: collector-level store accessor should return `metrics.CollectorStore` interface, not concrete implementation type.
- `97A`: function-vs-collection write discipline is documentation-only in phase-2; hard authorization/guarding is deferred to a later phase.
- `98A`: rename write-side `AttrSet` to `LabelSet` (and `Attr` to `Label`) for consistent label terminology across read/write APIs.
- `99A`: meter-level scoped labels use `WithLabelSet(...)` (not constructor label params).
- `100A`: write calls accept optional variadic label sets (e.g. `Observe(v, ...LabelSet)`), enabling `Observe(v)` default path.
- `101A`: scoped-label and per-call-label key conflicts are fail-fast in phase-2.
- `102A`: meter constructors accept `prefix` only (`SnapshotMeter(prefix)` / `StatefulMeter(prefix)`); version is not an input parameter.
- `103A`: `CollectorStore` exposes explicit grouped views `Read()` and `Write()` instead of a flat mixed method set.
- `104A`: freshness semantics use dual-clock behavior: success-based eviction/retention plus freshness metadata for readers/functions.
- `105A`: counter deltas are strict contiguous-only; if collect-attempt gaps exist between `previous` and `current`, `Delta` is unavailable.
- `106A`: reader defaults to fresh-only visibility; raw visibility is explicit via `ReadRaw()`.
- `107A`: series visibility is driven by per-cycle `seen` markers; successful partial cycles expose only seen series.
- `108A`: write/commit model is staged-frame based; abort discards staged writes with zero effect on committed snapshot.
- `109B`: freshness scope is split by instrument mode: snapshot metrics are cycle-fresh; stateful metrics are last-committed by default.
- `110B`: label API uses typed immutable handles/views for safety; iterator callbacks do not expose mutable maps.
- `111A`: reader views are bound to one immutable snapshot at acquisition time.
- `112B`: cycle control is runtime-only, separated from collector writer API.
- `113B`: test matrix explicitly covers fresh/raw visibility, partial cycles, failed attempts, and strict contiguous delta behavior.
- `114C`: writes outside active cycle are invalid (`debug panic`, `production warn+drop`).
- `115A`: collection store is cycle-scoped only; out-of-cycle runtime/internal instrumentation uses separate dedicated store.
- `116A`: `StatefulCounter.Add` baselines from committed current and accumulates deltas in-cycle.
- `117A`: gauge commit semantics are explicit for snapshot vs stateful modes.
- `118A`: for stateful summary/histogram, `window=cycle` implies `FreshnessCycle`.
- `119B`: collector receives restricted `CollectorStore` view; runtime cycle-control capability is not reachable from collector-facing API.
- `120A`: `Read()`/`ReadRaw()` are committed-only during active cycle; staged writes are never visible before commit.
- `121A`: `AbortCycle` publishes metadata-only snapshot (failed attempt status) while preserving last committed data series.
- `122B`: freshness overrides are mode-restricted (`snapshot` cannot use `FreshnessCommitted`).
- `123A`: fail-fast runtime policy is unified (`debug panic`, `production warn+drop`), with declaration-time config errors failing startup.
- `124B`: phase-2 defines explicit seam for dedicated runtime/internal metrics store.
- `125B`: runtime/internal store uses cycleless immediate-commit stateful write semantics.
- `126B`: runtime/internal store is thread-safe multi-writer.
- `127B`: runtime/internal store uses wall-clock retention + max-series cap.
- `128B`: `LabelSet` handles are validated; invalid/foreign handles follow fail-fast policy.
- `129B`: declaration-time panics are recovered in init/check path and converted into job init/check errors.
- `63B`: runtime store exposes stateful-only write surface.
- `64A`: instrument option set includes explicit window declaration (`WithWindow`).
- `65B`: public store constructor does not expose cycle-control-capable concrete type.
- `66B`: runtime store metadata uses dedicated runtime commit sequence semantics.
- `67A`: runtime store ownership is collector-side via explicit accessor.
- `68A`: counter reset delta returns current value (`current`, `true`).
- `69A`: `StatefulGauge.Add` baselines from committed value (or zero) and accumulates in-cycle.
- `70A`: runtime store freshness policy is fixed to `FreshnessCommitted`.
- `71A`: `WithWindow` is valid only for stateful summary/histogram; other instruments fail-fast.
- `72A`: `Delta()` on non-counter series returns `(0, false)`.
- `73A`: repeated `SnapshotCounter.ObserveTotal` writes in one cycle use last-write-wins.
- `74B`: `CounterState` is internal storage type, not part of phase-2 public reader contract.
- `75B`: runtime acquires cycle control via package-internal helper/view, not public API.
- `76A`: keep `Family` and `ForEachByName`; document `Family` as reusable view and `ForEachByName` as one-shot convenience.
- `77A`: `Value()` returns latest committed raw value for all series types (counters return `current`).
- `78A`: cross-meter same-identity snapshot/stateful conflicts are fail-fast.
- `79A`: runtime commit sequence semantics are observable per-write commit (internal batching only if externally equivalent).
- `80A`: histogram bucket boundaries and stateset state domains are schema-stable per metric family after first successful schema capture; drift is rejected.
- `81B`: canonical non-scalar families are first-class in store (Histogram/Summary/StateSet) with snapshot-scoped flatten view/cache for scalar consumers.
- `82B`: summary supports count/sum and quantiles when explicitly configured.
- `83A`: runtime/internal store baseline uses sharded-lock write path with atomic snapshot publication.
- `84A`: `CounterVec`/`GaugeVec` are instrument-family (bundle) API concepts, not metric kinds; they are ergonomic facades over labeled series of a single kind.
- `133B`: non-scalar write APIs are split by mode:
  - snapshot path writes full non-scalar points,
  - stateful path writes observation samples.
- `134C`: histogram bounds declaration is mode-dependent:
  - stateful histogram requires `WithHistogramBounds(...)`,
  - snapshot histogram may omit bounds and capture schema from first successful `ObservePoint(...)`.
  - descriptor model detail:
    - declaration-time shared instrument descriptor remains immutable after publish,
    - per-series committed snapshots may carry a cloned descriptor with captured histogram schema attached.
- `135B`: stateset writes use full-state replace per sample (`ObserveStateSet(...)`), not per-state incremental writes.
- `139B`: stateset keeps canonical bitset semantics with declaration-time mode (`ModeBitSet`/`ModeEnum`) and enum helper (`Enable(...)`) for one-hot updates.
- `136A`: `Flatten()` returns a scalar-series reader view and is idempotent (`Flatten().Flatten()` yields equivalent view for the same snapshot).
- `137A`: flatten synthetic naming follows OpenMetrics compatibility:
  - histogram: `_bucket{le}`, `_count`, `_sum`,
  - summary: base + `quantile`, plus `_count`/`_sum`,
  - stateset: base + label key equal to metric family name.
- `138A`: flatten preserves source visibility mode:
  - `Read().Flatten()` => fresh-policy filtered scalar view,
  - `ReadRaw().Flatten()` => full committed scalar view.
- `140A`: `Flatten()` remains reader-level in public API; per-family flattening/caching is internal implementation detail.
- `141A`: phase-2 implementation kickoff approved; implementation proceeds metric-kind by metric-kind (`Gauge` first).
- `142F`: phase-2 package naming set to `pkg/metrix` with package name `metrix` (post-legacy-rename plan).
- `143B`: legacy `pkg/metrix` will be renamed by user to free `metrix` naming for phase-2 package usage.
- `144A`: apply soft decoupling now: instrument/meter write path depends on an internal backend interface (not concrete store type), while `CollectorStore` remains the default backend implementation.
- `145A`: use atomics for standalone scalar cells where lifecycle/map coordination is not required; keep mutex/shard locking for shared map/cycle transitions (superseded by `146A`).
- `146A`: drop scalar atomics for phase-2 value updates; use mutex/shard-lock paths for scalar updates and keep atomics only for immutable snapshot publication/sequencing where already part of store design.
- `147A`: collector-facing metrics access model is collector-owned collection store via `MetricsStore() CollectorStore`; `Collect()` writes through that store and can lazily declare/get-or-create instruments per cycle; runtime owns cycle boundaries.
- `148A`: collector is not required to keep one struct field per metric handle; handle caching is optional performance optimization, not a correctness requirement.
- `130A`: phase-2 implementation design must explicitly study and borrow applicable thread-safety/performance patterns from `client_golang/prometheus/*` before coding.
- `131A`: before phase-2 implementation begins, produce a metric-by-metric implementation contract (data structures, concurrency model, write/read/flatten behavior) for all supported metric kinds.
- `132A`: histogram and stateset use dedicated typed writer instruments (not generic family-write path), with strict schema validation.

## Requirements

### R1. Collector Write Model

- Collector code should write via declared instruments (meter/instrument handles), not undeclared string writes.
- API must support both input styles:
  - scrape-like snapshot inputs (absolute sampled values),
  - instrument-like stateful inputs (maintained/changed values).
- Default collector path is snapshot mode; stateful mode is primarily for runtime/internal metrics and selected collectors.
- Attributes/labels are attached at record time via stable attribute sets.
- Collector must not manage prev/current state manually.
- Collector owns one collection `CollectorStore` for collector lifetime (created in `New()`), exposed via `MetricsStore() CollectorStore`.
- Collector `Collect` writes through its owned store (`store.Write().SnapshotMeter(...)` / `StatefulMeter(...)`).
- Metric declaration in `Collect` is lazy get-or-create by contract; re-declaring same instrument each cycle is valid when schema/options match.
- Caching instrument handles in collector fields is optional optimization; not required for correctness.
- Chart engine and functions read from the same underlying store model through read APIs.
- Collectors use declaration/write APIs only; cycle lifecycle (`BeginCycle`/`Commit`/`Abort`) is runtime-owned and not part of collector writer surface.
- Non-scalar families must use dedicated typed instruments (`Histogram`, `Summary`, `StateSet`) rather than generic family-write calls.
- Runtime obtains cycle-control capability via internal `CycleManagedStore` seam, not through collector-facing store methods.
- Collection store in phase-2 is cycle-scoped; runtime/internal out-of-cycle instrumentation must use a separate dedicated store path.
- Dedicated runtime/internal store seam must be explicit in phase-2 via collector-side accessor (`RuntimeMetricsStoreProvider` on V2 collectors).
- Runtime store write API is stateful-only (no snapshot meter in runtime path).
- Freshness override validation is mode-restricted: snapshot instruments cannot use `FreshnessCommitted`.
- Runtime store freshness policy is fixed to `FreshnessCommitted`; runtime-side `FreshnessCycle` is invalid.
- Declaration-time panics (instrument creation/options validation) must be recovered by framework init/check boundaries and returned as job init/check errors.

### R2. Counter State Contract

- Per counter series keep:
  - `current` sample,
  - `previous` sample,
  - `hasPrev` flag,
  - `currentAttemptSeq`,
  - `previousAttemptSeq`.
- `Delta` is valid only when:
  - `hasPrev == true`, and
  - `currentAttemptSeq == previousAttemptSeq + 1` (strict contiguous samples).
- If there is any collect-attempt gap between samples, `Delta` is unavailable (`ok=false`).
- Within valid delta conditions above, if a reset is detected (`current < previous`), `Delta` returns `(current, true)`.
- Counter transitions are computed on commit from staged cycle frame to committed snapshot (not by mutating committed state during writes).
- State transition updates only counters written in current successful cycle.
- Counters not written in a cycle keep prior two collected points unchanged.
- `StatefulCounter.Add` uses previous committed `current` as baseline on first write in a cycle, then accumulates deltas in staged frame.

### R3. Label Model

- Labels are key-value pairs (`string -> string`).
- Label set canonicalization is deterministic (sorted keys).
- Metric identity is `(metric name, canonical label set)`.
- Label names must be unique inside one label set.
- When merging meter-scoped and per-call label sets, duplicate keys are fail-fast errors in phase-2.
- Public API should expose typed immutable label handles/views; mutable map labels are input-only convenience for lookups, not iterator output.
- `LabelSet` handles must be validated by store implementation; invalid/foreign handles follow fail-fast policy.

### R4. Numeric Types

- Store value type in phase-2 is `float64` for all scalar series.
- Integer-source metrics are converted to `float64` on write.
- Emit paths that require integer representation can cast (`int64(v)`).
- Known tradeoff: integers above `2^53` may lose precision in `float64`.

### R5. Query Surface

- Must support:
  - point reads by `(name, labels)`,
  - `Value()` returning latest committed raw value for all series types
    (counters => committed `current`, gauges => committed gauge value),
  - counter delta reads,
  - freshness/status reads for overall collect loop and per-series recency,
  - dual read modes (`Read` fresh policy + `ReadRaw` all committed series),
  - committed-only reads during active cycle (no staged visibility),
  - snapshot-bound reader consistency (one reader sees one immutable snapshot),
  - iteration over all series by metric name,
  - global series iteration for chartengine autogen-unmatched mode,
  - selector-oriented iteration hooks for chart engine (without full PromQL runtime).

### R6. Migration Surface

- Must provide adapters for incremental rollout:
  - `map[string]int64` -> store
  - `map[string]float64` -> store
  - parser output (`src/go/pkg/prometheus/*`) -> store writes
  - helper path for `metrix/stm`-style emitters

### R7. Concurrency (Phase-2)

- Write path is single-writer and cycle-scoped.
- Read path (functions/engine) uses immutable snapshots published atomically after collect cycle processing.
- No direct concurrent read/write access to mutable writer store.
- Function handlers may run concurrently with collect; in phase-2, writes to collector-owned collection store from function code are disallowed by documented convention (no hard runtime enforcement yet).
- Cycle control ownership is runtime-only via dedicated controller surface; collector-facing writer API does not expose lifecycle control methods.
- Writes attempted without active cycle frame are invalid: `debug panic`, `production warn+drop`.

### R8. Store Retention and Cleanup (Phase-2)

- Store must define behavior for series that stop appearing (instances come/go).
- Collection-store cleanup clock should follow successful collect-cycle semantics (same rule used elsewhere).
- Policy must prevent unbounded memory growth under changing label cardinality.

### R9. Runtime/Internal Store (Phase-2)

- Runtime/internal store is separate from collection store and is used for out-of-cycle instrumentation.
- Runtime/internal store write semantics are cycleless immediate-commit (stateful model).
- Runtime/internal store concurrency is thread-safe multi-writer.
- Runtime/internal store retention is wall-clock-based with max-series cap.
- Runtime/internal store ownership is collector-side and exposed through explicit runtime-store accessor.
- Runtime/internal store metadata is sequence-based per runtime commit:
  - `CollectMeta.LastAttemptSeq`: last runtime commit sequence,
  - `CollectMeta.LastAttemptStatus`: `CollectStatusSuccess` after commits (`CollectStatusUnknown` before first commit),
  - `CollectMeta.LastSuccessSeq`: equals runtime commit sequence,
  - `SeriesMeta.LastSeenSuccessSeq`: last runtime commit where series was written.

### R10. Schema Stability for Non-Scalar Families

- StateSet state domain must be stable per metric family after first successful schema capture.
- Histogram bucket boundary set (including `+Inf` presence) must be stable per metric family after first successful schema capture.
- New label sets still form new metric series identities and are valid; this rule governs only state-domain/bucket-domain drift.
- Drift behavior in phase-2 strict mode:
  - reject (drop) the offending family component for the current collection attempt,
  - emit warning and increment internal error counters,
  - do not fail the whole collect cycle.

### R11. Metric Inventory and Implementation Contract (Phase-2 Baseline)

- Supported metric kinds in phase-2:
  - Gauge
  - Counter
  - Histogram
  - Summary
  - StateSet
- Instrument-family/bundle model (not a metric kind):
  - `CounterVec` and `GaugeVec` represent one instrument family over variable labels.
  - Each vec member is still one normal series identity: `(metric name + canonical labels)`.
  - Vecs do not introduce new storage kinds or new read semantics.
  - Phase-2 baseline API already supports vec behavior through `WithLabels(...)` / `WithLabelSet(...)`.
  - Optional explicit vec wrappers can be added as API ergonomics/perf sugar without changing store internals.
- Mode support:
  - Snapshot meter: Gauge, Counter, Histogram, Summary, StateSet
  - Stateful meter: Gauge, Counter, Histogram, Summary, StateSet
  - Histogram/StateSet are declared via dedicated typed instruments (strict schema path).
- Per-kind internal state contract:
  - Gauge:
    - committed value (`float64`), mode, freshness policy, last-seen success sequence
    - staged value per active cycle frame (`last-write-wins` for snapshot `Observe`, overwrite for stateful `Set`, baseline+accumulate for stateful `Add`)
  - Counter:
    - committed `current`, `previous`, `hasPrev`, `currentAttemptSeq`, `previousAttemptSeq`
    - staged `current` per active cycle frame
    - snapshot `ObserveTotal`: in-cycle last-write-wins
    - stateful `Add`: baseline from committed `current` on first write in cycle, then accumulate
  - Histogram:
    - schema descriptor (stable bucket boundaries including `+Inf`)
    - per-series bucket counts + count + sum
    - `Observe(v)` updates one bucket, count, sum
    - schema drift (boundary set mismatch) is rejected per `R10`
  - Summary:
    - count + sum mandatory
    - quantiles optional (enabled only when explicitly configured on instrument)
    - `Observe(v)` updates count/sum (and quantile structures when enabled)
  - StateSet:
    - stable state-domain set per metric family
    - per-state boolean value in canonical stateset family representation
    - mode-constrained semantics:
      - `ModeBitSet`: 0..N true states allowed,
      - `ModeEnum`: exactly 1 true state required.
    - state-domain drift (new/removed state keys) rejected per `R10`
- Commit/abort semantics:
  - collection store commit materializes immutable read snapshot
  - abort publishes metadata-only snapshot, preserving previously committed data
  - runtime store is immediate-commit stateful path (no cycle frame)

### R12. Performance Patterns Borrowed from `client_golang/prometheus`

- Descriptor precomputation:
  - precompute stable identity/dimension hashes and sorted const label pairs at declaration time
  - reference: `client_golang/prometheus/desc.go`
- Vector lookup path:
  - hash-indexed lookup for label sets
  - collision-safe bucket lists with full label-value match verification
  - RWMutex with double-check lock pattern on create path
  - reference: `client_golang/prometheus/vec.go`
- Counter/Gauge hot paths:
  - fast atomic paths where appropriate (`Set` fast path for gauge; integer fast path for counter increments)
  - reference: `client_golang/prometheus/gauge.go`, `client_golang/prometheus/counter.go`
- Histogram bucket-index path:
  - hybrid search strategy: linear search for small bucket arrays, binary search for larger arrays
  - reference: `client_golang/prometheus/histogram.go` (`findBucket`)
- Read/write separation:
  - keep write path minimal and predictable; publish immutable snapshots for read path
  - reference approach aligns with hot-write / controlled-scrape-read strategy in histogram/summary write paths
  - reference: `client_golang/prometheus/histogram.go`, `client_golang/prometheus/summary.go`

### R13. Metric-by-Metric Implementation Contract (Concrete)

Canonical store representation (decision `81B`):

- Scalar families are canonical for:
  - Gauge
  - Counter
- Non-scalar families are canonical first-class types for:
  - Histogram
  - Summary
  - StateSet
- Flattening to scalar series is a read-view concern:
  - `Read().Flatten()` (or equivalent snapshot-scoped flatten view) exposes synthetic scalar series for consumers that need scalar-only paths.
  - Flatten caches are snapshot-bound and deterministic (same snapshot => same flattened key set/order).

Core indexing/identity baseline:

- Identity key is `(metric name + canonical labels)` with deterministic hashing.
- By-name index is mandatory for cheap family iteration.
- Global one-pass iteration is available for unmatched/autogen paths.

#### Gauge

- Snapshot write: `Observe(v)` with in-cycle last-write-wins.
- Stateful write: `Set(v)` overwrite, `Add(delta)` baseline-from-committed then accumulate.
- Read: `Value()` returns committed value; `Delta()` is unavailable (`0,false`).

#### Counter

- Snapshot write: `ObserveTotal(v)` with in-cycle last-write-wins.
- Stateful write: `Add(delta)` baseline-from-committed then accumulate.
- Commit tracks `current`, `previous`, `hasPrev`, `currentAttemptSeq`, `previousAttemptSeq`.
- Read:
  - `Value()` => committed `current`.
  - `Delta()` => contiguous-only; reset-aware (`current < previous => current`).

#### Histogram (canonical typed family)

- Writer API is dedicated typed instrument (`132A`).
- Mode-specific writes (`133B`):
  - snapshot meter: `ObservePoint(HistogramPoint, ...)`
  - stateful meter: `Observe(v, ...)`
- Schema declaration rule (`134C`):
  - stateful histogram declarations must provide `WithHistogramBounds(...)` (missing bounds => declaration fail-fast),
  - snapshot histogram declarations may omit bounds and capture schema from first successful `ObservePoint(...)`.
- Canonical state per series:
  - stable bucket schema (`le` domain, including `+Inf`),
  - bucket counts,
  - `_count`, `_sum`.
- Schema drift is rejected per `R10`.
- Flatten view maps typed histogram into scalar synthetic series:
  - `_bucket{le="..."}`
  - `_count`
  - `_sum`

#### Summary (canonical typed family)

- Writer API is dedicated typed instrument.
- Mode-specific writes (`133B`):
  - snapshot meter: `ObservePoint(SummaryPoint, ...)`
  - stateful meter: `Observe(v, ...)`
- Canonical state per series:
  - `_count`, `_sum` always.
  - quantiles only when explicitly configured (`82B`).
- Flatten view maps typed summary into scalar synthetic series:
  - `_count`
  - `_sum`
  - optional quantile series.

#### StateSet (canonical typed family)

- Writer API is dedicated typed instrument (`132A`).
- Canonical state per series:
  - stable state-domain schema,
  - boolean value per declared state.
- Write shape (`135B`):
  - one call replaces entire state-domain sample (`ObserveStateSet(StateSetPoint, ...)`),
  - partial per-state writes are not part of phase-2 contract.
- Mode semantics (`139B`):
  - `ModeBitSet`: multiple true states are valid,
  - `ModeEnum`: exactly one true state is required.
- Enum helper (`139B`):
  - `Enable(actives...)` is convenience one-shot replacement;
  - in `ModeEnum`, helper enforces exactly one active state.
- Schema drift is rejected per `R10`.
- Flatten view maps typed stateset into scalar synthetic series:
  - one series per state using a synthetic label whose key equals the metric family name
    (e.g. `service_mode{service_mode="operational"} => 1`).

#### Runtime/Internal Store Baseline (`83A`)

- Dedicated store (`NewRuntimeStore`) with stateful-only writer surface.
- Multi-writer safe with per-write immediate commit semantics (`79A`).
- Baseline implementation:
  - sharded-lock write path + atomic immutable snapshot publication.
  - lock-free hot/cold algorithms are deferred unless benchmarks require them.

#### Reference Performance Patterns to Borrow Exactly

- Hash+bucket lookup with collision verification (metric vec style):
  - `client_golang/prometheus/vec.go`
- Counter integer fast-path + float CAS fallback:
  - `client_golang/prometheus/counter.go`
- Gauge `Set` atomic-store fast path:
  - `client_golang/prometheus/gauge.go`
- Descriptor precompute and sorted label hashing:
  - `client_golang/prometheus/desc.go`
- Hybrid bucket-index strategy (linear for small arrays, binary otherwise):
  - `client_golang/prometheus/histogram.go` (`findBucket`)

## OpenMetrics Data-Model Mapping (No Wire Format)

This phase consumes semantics of metric kinds, not transport format details.

### Gauge

- Semantics: current absolute measurement.
- Store shape: scalar series (`current` only).

### Counter

- Semantics: monotonic total with possible reset.
- Store shape: scalar series with `current` + `previous`.
- Delta/reset behavior handled by store helpers.

### StateSet

- Semantics: set of boolean states (`0`/`1`) per sample.
- Phase-2 canonical representation: typed stateset family with stable state-domain.
- Flatten view representation: scalar gauge-like series per state.
- Domain rule: for flattened StateSet, label key equals metric family name; allowed label values
  (state names) are schema-stable per metric family after first successful capture.

### Summary

- Semantics: count/sum (+ optional quantiles).
- Phase-2 canonical representation: typed summary family.
- Flatten view representation:
  - `_count` (counter semantics),
  - `_sum` (counter semantics),
  - optional `quantile` series (gauge semantics) when quantiles are explicitly configured.

### Histogram

- Semantics: cumulative buckets + count + sum.
- Phase-2 canonical representation: typed histogram family with stable bucket schema.
- Flatten view representation:
  - `_bucket{le="..."}` (counter semantics),
  - `_count` (counter semantics),
  - `_sum` (counter semantics).
- Domain rule: bucket boundary set (`le` domain, including `+Inf`) is schema-stable per metric family after first successful capture.

### Notes

- Canonical store keeps typed non-scalar families; flattening is an explicit read view.
- Selector/chart paths that are scalar-only consume the flatten view.

## Proposed Package Layout

- `src/go/plugin/go.d/agent/metrics/types.go`
- `src/go/plugin/go.d/agent/metrics/labels.go`
- `src/go/plugin/go.d/agent/metrics/store.go`
- `src/go/plugin/go.d/agent/metrics/query.go`
- `src/go/plugin/go.d/agent/metrics/adapter_legacy.go`
- `src/go/plugin/go.d/agent/metrics/adapter_prom.go`

## Core Types (Contract Level)

```go
package metrics

// Lookup input labels (caller-supplied; store canonicalizes internally).
type Labels map[string]string

type SampleValue = float64
type Label struct{ Key, Value string }
type LabelSet struct{ _opaque uintptr } // immutable compiled label-set handle
type InstrumentOption interface{}

type SeriesID string // canonical (metric name + canonical labels)

type LabelView interface {
    Len() int
    Get(key string) (string, bool)
    Range(fn func(key, value string) bool)
    CloneMap() map[string]string
}

type MetricMode int

const (
    MetricModeSnapshot MetricMode = iota
    MetricModeStateful
)

type FreshnessPolicy int

const (
    FreshnessCycle FreshnessPolicy = iota
    FreshnessCommitted
)

type MetricWindow int

const (
    WindowCumulative MetricWindow = iota
    WindowCycle
)

type StateSetMode int

const (
    ModeBitSet StateSetMode = iota
    ModeEnum
)

// Optional instrument declaration override.
// Primarily used for stateful instruments; snapshot instruments are cycle-fresh by default.
// Validation rule: snapshot instruments cannot set `FreshnessCommitted`.
func WithFreshnessPolicy(p FreshnessPolicy) InstrumentOption
// WithWindow is valid only for stateful Summary and Histogram instruments.
// Passing it to gauge/counter constructors follows fail-fast policy.
func WithWindow(w MetricWindow) InstrumentOption
// Enables quantiles for Summary. If not set, Summary stores count+sum only.
func WithSummaryQuantiles(q []float64) InstrumentOption
// Declares histogram bounds.
// Required for stateful histograms.
// Optional for snapshot histograms; when omitted, schema may be captured from first successful ObservePoint.
func WithHistogramBounds(bounds []float64) InstrumentOption
// Declares StateSet state-domain; when omitted, first successful write can capture schema.
func WithStateDomain(states []string) InstrumentOption
// Declares StateSet semantics mode.
// ModeBitSet allows 0..N enabled states; ModeEnum requires exactly one enabled state.
func WithStateSetMode(m StateSetMode) InstrumentOption

type CollectStatus int

const (
    CollectStatusUnknown CollectStatus = iota
    CollectStatusSuccess
    CollectStatusFailed
)

type CollectMeta struct {
    LastAttemptSeq    uint64
    LastAttemptStatus CollectStatus
    LastSuccessSeq    uint64
}

type SeriesMeta struct {
    LastSeenSuccessSeq uint64
    Mode               MetricMode
    Freshness          FreshnessPolicy
}

type HistogramBucket struct {
    UpperBound float64
    Count      SampleValue
}

type HistogramPoint struct {
    Buckets []HistogramBucket
    Count   SampleValue
    Sum     SampleValue
}

type SummaryQuantile struct {
    Quantile float64
    Value    SampleValue
}

type SummaryPoint struct {
    Count     SampleValue
    Sum       SampleValue
    Quantiles []SummaryQuantile
}

type StatePoint struct {
    State   string
    Enabled bool
}

type StateSetPoint struct {
    States []StatePoint
}
```

## Internal Storage Types (Non-Contract)

```go
// Internal storage representation; not exposed through phase-2 Reader API.
type CounterState struct {
    Current SampleValue
    Previous SampleValue
    HasPrev bool
    CurrentAttemptSeq  uint64
    PreviousAttemptSeq uint64
}
```

## Collector API (Contract Level)

```go
// Required collector contract for phase-2 collection metrics.
// Collector owns the store and runtime discovers it via this accessor.
type MetricsStoreProvider interface {
    MetricsStore() CollectorStore
}

// V2 collector shape (outside metrics package, shown here for contract clarity).
// Collect remains iteration-agnostic; runtime drives cycle boundaries.
type ModuleV2 interface {
    Collect(ctx context.Context) error
    MetricsStore() CollectorStore
}

type CollectContext interface {
    // Optional runtime context seam.
    // Collector metric writes do not depend on this accessor.
    Store() CollectorStore
}

// Optional collector contract (wired outside metrics package):
// dedicated runtime/internal store for out-of-cycle instrumentation.
type RuntimeMetricsStoreProvider interface {
    RuntimeStore() RuntimeStore
}

type CollectorStore interface {
    Read() Reader
    ReadRaw() Reader
    Write() Writer
}

type RuntimeStore interface {
    Read() Reader
    ReadRaw() Reader
    Write() RuntimeWriter
}

// Runtime-only control surface. Not exposed to collector code.
type CycleController interface {
    BeginCycle()
    CommitCycleSuccess()
    AbortCycle()
}

// CycleManagedStore is used by runtime internals that own collect cycle boundaries.
// Public constructors do not expose this capability.
type CycleManagedStore interface {
    CollectorStore
    CycleController() CycleController
}

type Reader interface {
    // Read surface
    Value(name string, labels Labels) (SampleValue, bool)
    Delta(name string, labels Labels) (SampleValue, bool)
    Histogram(name string, labels Labels) (HistogramPoint, bool)
    Summary(name string, labels Labels) (SummaryPoint, bool)
    StateSet(name string, labels Labels) (StateSetPoint, bool)
    SeriesMeta(name string, labels Labels) (SeriesMeta, bool)
    CollectMeta() CollectMeta
    // Flatten returns a snapshot-bound scalar-series view.
    // Contract:
    //   - idempotent for a given snapshot (`r.Flatten().Flatten()` is equivalent to `r.Flatten()`),
    //   - preserves source visibility mode (`Read` vs `ReadRaw`),
    //   - typed getters (`Histogram`/`Summary`/`StateSet`) return `(zero,false)` on flattened views.
    // Implementation detail:
    //   - flatten generation may be delegated to typed family internals with snapshot-scoped caches.
    Flatten() Reader
    // Family returns a reusable snapshot-bound view for repeated iteration.
    Family(name string) (FamilyView, bool)
    // ForEachByName is a one-shot convenience iterator by metric name.
    ForEachByName(name string, fn func(labels LabelView, v SampleValue))
    // ForEachSeries provides one-pass global iteration for autogen-unmatched scans.
    ForEachSeries(fn func(name string, labels LabelView, v SampleValue))
    ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue))
}

type Writer interface {
    // Write/declaration surface
    SnapshotMeter(prefix string) SnapshotMeter
    StatefulMeter(prefix string) StatefulMeter
}

type RuntimeWriter interface {
    // Runtime/internal path is stateful-only.
    StatefulMeter(prefix string) StatefulMeter
}

type SnapshotMeter interface {
    WithLabels(labels ...Label) SnapshotMeter
    WithLabelSet(labels ...LabelSet) SnapshotMeter
    Gauge(name string, opts ...InstrumentOption) SnapshotGauge
    Counter(name string, opts ...InstrumentOption) SnapshotCounter
    Histogram(name string, opts ...InstrumentOption) SnapshotHistogram
    Summary(name string, opts ...InstrumentOption) SnapshotSummary
    StateSet(name string, opts ...InstrumentOption) StateSetInstrument
    LabelSet(labels ...Label) LabelSet
}

type StatefulMeter interface {
    WithLabels(labels ...Label) StatefulMeter
    WithLabelSet(labels ...LabelSet) StatefulMeter
    Gauge(name string, opts ...InstrumentOption) StatefulGauge
    Counter(name string, opts ...InstrumentOption) StatefulCounter
    Histogram(name string, opts ...InstrumentOption) StatefulHistogram
    Summary(name string, opts ...InstrumentOption) StatefulSummary
    StateSet(name string, opts ...InstrumentOption) StateSetInstrument
    LabelSet(labels ...Label) LabelSet
}

type SnapshotGauge interface {
    Observe(v SampleValue, labels ...LabelSet)
}

type StatefulGauge interface {
    Set(v SampleValue, labels ...LabelSet)
    Add(delta SampleValue, labels ...LabelSet)
}

type SnapshotCounter interface {
    ObserveTotal(v SampleValue, labels ...LabelSet)
}

type StatefulCounter interface {
    Add(delta SampleValue, labels ...LabelSet)
}

type SnapshotHistogram interface {
    ObservePoint(p HistogramPoint, labels ...LabelSet)
}

type StatefulHistogram interface {
    Observe(v SampleValue, labels ...LabelSet)
}

type SnapshotSummary interface {
    ObservePoint(p SummaryPoint, labels ...LabelSet)
}

type StatefulSummary interface {
    Observe(v SampleValue, labels ...LabelSet)
}

type StateSetInstrument interface {
    // ObserveStateSet replaces full state-domain values for the series in one call.
    // Input must provide exactly one value per declared/captured state key.
    ObserveStateSet(p StateSetPoint, labels ...LabelSet)
    // Enable is a convenience helper that builds a full replacement point internally.
    // In ModeEnum, exactly one state must be enabled.
    // In ModeBitSet, zero or more states can be enabled with this helper.
    // For per-call labels, use ObserveStateSet(..., labels...).
    Enable(actives ...string)
}

type FamilyView interface {
    Name() string
    ForEach(fn func(labels LabelView, v SampleValue))
}

// Public constructors.
func NewCollectorStore() CollectorStore
func NewRuntimeStore() RuntimeStore
// Runtime obtains cycle-control capability from collection store through
// package-internal helper/view only (not public API).

// Read methods (via Reader view)
func (r *StoreReadView) Value(name string, labels Labels) (SampleValue, bool)
func (r *StoreReadView) Delta(name string, labels Labels) (SampleValue, bool)
func (r *StoreReadView) Histogram(name string, labels Labels) (HistogramPoint, bool)
func (r *StoreReadView) Summary(name string, labels Labels) (SummaryPoint, bool)
func (r *StoreReadView) StateSet(name string, labels Labels) (StateSetPoint, bool)
func (r *StoreReadView) SeriesMeta(name string, labels Labels) (SeriesMeta, bool)
func (r *StoreReadView) CollectMeta() CollectMeta
func (r *StoreReadView) Flatten() Reader
func (r *StoreReadView) Family(name string) (FamilyView, bool)

// Iteration (via Reader view)
func (r *StoreReadView) ForEachByName(name string, fn func(labels LabelView, v SampleValue))
func (r *StoreReadView) ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue))

// Write methods (via Writer view)
func (w *StoreWriteView) SnapshotMeter(prefix string) SnapshotMeter
func (w *StoreWriteView) StatefulMeter(prefix string) StatefulMeter
func (w *RuntimeStoreWriteView) StatefulMeter(prefix string) StatefulMeter

// Cycle methods (runtime-only controller view)
func (c *StoreCycleController) BeginCycle()
func (c *StoreCycleController) CommitCycleSuccess()
func (c *StoreCycleController) AbortCycle()
```

## Collector Usage Pattern (Locked)

```go
type Collector struct {
    store metrix.CollectorStore
}

func New() *Collector {
    return &Collector{store: metrix.NewCollectorStore()}
}

func (c *Collector) MetricsStore() metrix.CollectorStore { return c.store }

func (c *Collector) Collect(ctx context.Context) error {
    sm := c.store.Write().SnapshotMeter("apache")

    // Lazy get-or-create declarations are valid in every cycle.
    sm.Gauge("workers_busy").Observe(12)
    sm.Gauge("workers_idle").Observe(34)
    sm.Counter("requests_total").ObserveTotal(123456)
    return nil
}
```

Runtime/job loop owns cycle transitions:

```go
func runOnce(ctx context.Context, c ModuleV2) error {
    ms, ok := metrix.AsCycleManagedStore(c.MetricsStore())
    if !ok {
        return errors.New("collector store does not expose cycle management view")
    }
    cc := ms.CycleController()

    cc.BeginCycle()
    if err := c.Collect(ctx); err != nil {
        cc.AbortCycle()
        return err
    }
    cc.CommitCycleSuccess()
    return nil
}
```

Notes:
- Collector does not need to know whether this is 1st or 21st collect iteration.
- Counter prev/current transitions, freshness, and snapshot publication are runtime/store responsibilities.
- Caching metric handles on collector struct is optional optimization only.

## Lifecycle Semantics

### Scope

- Phase-2 collection store is cycle-scoped only.
- Runtime/internal metrics emitted outside collect cycles must use a separate dedicated store path.

### Write

- All writes target the active staged cycle frame only (committed snapshot is never mutated during writes).
- Gauge writes (`Observe`/`Set`/`Add`) update staged gauge value according to instrument semantics.
- Counter writes (`ObserveTotal`/`Add`) update staged counter current value.
- Snapshot histogram writes (`ObservePoint`) set staged typed histogram point state (bucket/count/sum) for the cycle.
- Stateful histogram writes (`Observe`) update staged typed histogram state incrementally from observed samples.
- Stateful histogram declaration without `WithHistogramBounds(...)` is invalid and follows declaration fail-fast policy (`134C`).
- Snapshot summary writes (`ObservePoint`) set staged typed summary point state (`count`/`sum` and optional quantiles) for the cycle.
- Stateful summary writes (`Observe`) update staged typed summary state from observed samples:
  - count/sum always,
  - quantiles only when explicitly configured on the instrument.
- StateSet writes (`ObserveStateSet`) replace staged typed stateset state for the whole declared/captured domain.
- StateSet helper writes (`Enable(...)`) are converted internally into full replacement `ObserveStateSet(...)`.
- `StatefulGauge.Add` baseline rule:
  - on first `Add` for a series in a cycle, initialize staged value from prior committed value (or zero if absent),
  - subsequent `Add` calls in same cycle accumulate on staged value.
- `StatefulGauge.Set` overwrites staged value directly.
- `SnapshotGauge.Observe` overwrites staged value directly (last-write-wins in-cycle).
- `SnapshotCounter.ObserveTotal` repeated writes for the same series in one cycle use last-write-wins (latest observed total overwrites staged value).
- `StatefulCounter.Add` baseline rule:
  - on first `Add` for a series in a cycle, initialize staged `current` from prior committed `current` (or zero if absent),
  - subsequent `Add` calls in same cycle accumulate on staged `current`.
- `StateSet` schema/shape rules:
  - sample must contain exactly the declared/captured stable domain (`R10`),
  - unknown or missing state keys are rejected by fail-fast policy and dropped for that write.
- `StateSet` mode rules:
  - `ModeEnum`: exactly one state must be true,
  - `ModeBitSet`: zero or more states may be true.
- Each write marks the series as `seen` in the active cycle frame.
- Freshness mode defaults:
  - snapshot instruments: `FreshnessCycle`,
  - stateful instruments: `FreshnessCommitted` unless explicitly overridden by instrument option.
- Freshness override restriction:
  - snapshot instruments cannot set `FreshnessCommitted`,
  - stateful instruments may set either policy (except `window=cycle` coupling below).
- Window option applicability restriction:
  - `WithWindow(...)` is valid only for stateful summary/histogram instruments,
  - passing `WithWindow(...)` to gauge/counter instruments is fail-fast.
- Stateful summary/histogram coupling:
  - if `window=cycle`, effective freshness is `FreshnessCycle` (implicit override).
- Invalid write outside active cycle frame:
  - debug mode: panic,
  - production mode: warn and drop write.

### Runtime Fail-Fast Policy

- Runtime invariant violations use unified policy:
  - debug mode: panic (fail-fast),
  - production mode: structured warn and drop (narrowest possible scope: sample/instrument/operation).
- Applies to:
  - write without active cycle frame,
  - `BeginCycle()` while a cycle is already active (double-begin),
  - `CommitCycleSuccess()` or `AbortCycle()` without an active cycle,
  - label merge conflicts / duplicate keys,
  - flattened synthetic-label reserved-key collisions (`le`, `quantile`, metric-family-name for stateset),
  - mode-mixing violations,
  - cross-meter same-identity mode conflicts (snapshot vs stateful on same `name+labels`),
  - invalid freshness override combinations,
  - invalid window/freshness option combinations.
- Declaration-time invalid configuration remains startup hard-fail.

### BeginCycle

- Runtime starts a new active staged cycle frame and increments collect-attempt sequence.
- Reset per-cycle `seen` state logically for all series by advancing cycle sequence/frame identity
  (implementation should avoid O(n) full-map flag clears).
- No committed snapshot data changes at this stage.

### End of Successful Collect Cycle

- Commit publishes a new immutable snapshot from:
  - previous committed snapshot, and
  - active staged cycle frame.
- Update collect meta:
  - `LastAttemptSeq = activeAttemptSeq`,
  - `LastAttemptStatus = CollectStatusSuccess`,
  - `LastSuccessSeq = activeAttemptSeq`.
- For each counter series marked `seen` in active cycle:
  - `previous = oldCommitted.current` (if exists),
  - `current = staged.current`,
  - `previousAttemptSeq = oldCommitted.currentAttemptSeq` (if exists),
  - `currentAttemptSeq = activeAttemptSeq`,
  - `hasPrev = true` only when old committed sample existed.
- For counter series not seen in active cycle:
  - carry forward old committed counter state unchanged.
- Gauge commit rules:
  - snapshot gauge series marked `seen`:
    - commit staged value to committed snapshot.
  - snapshot gauge series not `seen`:
    - no new sample in committed snapshot for this cycle; visibility follows `FreshnessCycle` policy.
  - stateful gauge series marked `seen`:
    - commit staged value to committed snapshot.
  - stateful gauge series not `seen`:
    - carry forward prior committed value unchanged; visibility follows `FreshnessCommitted` policy by default.
- For each series marked `seen` in active cycle:
  - set `SeriesMeta.LastSeenSuccessSeq = activeAttemptSeq`.

### Delta

- `Delta()` evaluation order:
  1. If the series is not a counter, return `(0, false)`.
  2. If `hasPrev == false` or `currentAttemptSeq != previousAttemptSeq + 1`, return `(0, false)`.
  3. If `current < previous` (reset), return `(current, true)`.
  4. Otherwise, return `(current - previous, true)`.

### Freshness and Status Visibility

- Use `CollectMeta()` for global collect-loop freshness:
  - `LastAttemptStatus` answers whether the latest collect attempt succeeded or failed.
  - `LastSuccessSeq` identifies the latest successful attempt sequence.
- Use `SeriesMeta(name, labels)` for per-series recency:
  - `LastSeenSuccessSeq` identifies the last successful attempt where the series was observed.
- "Series is current for latest successful collection" check:
  - `SeriesMeta.LastSeenSuccessSeq == CollectMeta.LastSuccessSeq`.
- Reader mode behavior:
  - `Read()` applies per-series freshness policy:
    - `FreshnessCycle`: expose only fresh series,
    - `FreshnessCommitted`: expose from latest committed snapshot even if not seen in last successful cycle.
  - `ReadRaw()` exposes all stored series (fresh + stale) for diagnostics/advanced consumers.
  - During active cycle (before commit/abort):
    - `Read()` and `ReadRaw()` remain committed-only and do not include staged writes.
  - `FreshnessCycle` predicate in phase-2:
    - `CollectMeta.LastAttemptStatus == CollectStatusSuccess`, and
    - `SeriesMeta.LastSeenSuccessSeq == CollectMeta.LastSuccessSeq`.
  - Consequence:
    - if latest collect attempt failed, `Read()` hides `FreshnessCycle` series (including empty iterators for purely cycle-fresh families),
    - `FreshnessCommitted` series remain visible from last committed snapshot,
    - stale last-committed values remain available via `ReadRaw()`.
    - if latest attempt succeeded but was partial:
      - `FreshnessCycle` series expose only `seen` entries,
      - `FreshnessCommitted` series stay visible by committed-state rule.

### Reader Snapshot Binding

- `Read()` and `ReadRaw()` return reader views bound to one immutable snapshot at acquisition time.
- One reader instance must not switch snapshots across calls.
- Callers must reacquire reader view to observe newer commits.

### AbortCycle

- Discard active cycle frame (including cycle `seen` markers and staged writes).
- Publish metadata-only immutable snapshot:
  - data series remain from last committed success snapshot,
  - update collect meta:
    - `LastAttemptSeq = activeAttemptSeq`,
    - `LastAttemptStatus = CollectStatusFailed`,
    - `LastSuccessSeq` unchanged.
- Do not advance per-series `LastSeenSuccessSeq`.
- `Read()` visibility remains based on last committed snapshot + per-series freshness policy.

### Runtime/Internal Store Lifecycle (Cycleless)

- Runtime/internal store has no collect-cycle transaction (`BeginCycle`/`Commit`/`Abort` are not part of runtime-store writer path).
- Runtime/internal writer surface is stateful-only (`StatefulMeter`).
- Each runtime write operation that passes validation updates immutable-read snapshot state and advances runtime commit sequence by one.
- Observable contract: runtime commits are per write operation (`Set`/`Add`/`Observe`);
  internal batching is allowed only if externally equivalent.
- Runtime store freshness policy is fixed:
  - all runtime-store instruments use `FreshnessCommitted`,
  - runtime-store `FreshnessCycle` configuration is invalid and follows fail-fast policy.
- Runtime metadata model:
  - before first commit: `CollectMeta.LastAttemptStatus = CollectStatusUnknown`,
  - after commit: `CollectMeta.LastAttemptStatus = CollectStatusSuccess`,
  - `CollectMeta.LastAttemptSeq` and `CollectMeta.LastSuccessSeq` both equal latest runtime commit sequence.
- For series written in a runtime commit, `SeriesMeta.LastSeenSuccessSeq` is set to that runtime commit sequence.
- Runtime `Read()`/`ReadRaw()` keep the same snapshot-bound reader contract as collection store.

## Label Canonicalization and Identity

- Canonical key format should be deterministic and allocation-aware.
- Recommended separator strategy (from SNMP VM precedent):
  - metric + `\x00` + sorted `k=v` joined by `\x1F`.
- Implementation may evolve internally, but externally:
  - same `(name, labels)` must always map to same series identity.

## Adapters (Required for Incremental Migration)

### Legacy Collect Maps

- Adapter: `map[string]int64` to snapshot-gauge `Observe(float64(v))`.
- Adapter: `map[string]float64` to snapshot-gauge `Observe(v)`.
- Keep old path operational while migrating collectors one by one.

### Prometheus Parser Output

- Adapter from parsed families/series into canonical typed-family writes.
- Flattening for chart engine/functions is a reader concern (`Read().Flatten()`), not an adapter concern.

### Adapter Validation Checklist (Strict Mode)

- Global:
  - Metric name must be non-empty.
  - Label names must be unique within one label set.
  - Label values must be valid UTF-8 strings.
  - Reject or sanitize reserved/internal label keys according to adapter policy.
- Gauge:
  - Accept single scalar value.
  - Allow `NaN`, `+Inf`, `-Inf` only if downstream path supports them; otherwise drop sample with warning.
- Counter:
  - Value must be non-`NaN`.
  - Value should be non-negative.
  - Reset detection remains store-level (`current < previous`) after ingestion.
- StateSet (canonical typed):
  - State values must be boolean-like (`0` or `1`).
  - Incoming state keys must belong to declared/captured state-domain schema.
  - State domain must match previously captured schema for the metric family (after first successful capture).
- Summary (canonical typed):
  - `_count` must be integer-like and non-negative.
  - `_sum` must be non-negative for standard summary semantics.
  - Quantile label must be parseable as float and within `[0,1]`.
  - Quantile value may be `NaN` (for no observations) and must not be negative otherwise.
- Histogram (canonical typed):
  - Must include `+Inf` bucket.
  - Bucket thresholds must be unique.
  - Buckets must be cumulative (non-decreasing by threshold order).
  - Bucket boundary set must match previously captured schema for the metric family (after first successful capture).
  - Bucket values must be integer-like and non-negative.
  - `_count`, when present, must equal `+Inf` bucket value.
  - `_sum` must be non-negative when histogram semantics are counter-like.
- GaugeHistogram (normalized as scalar series):
  - Must include `+Inf` bucket.
  - Bucket thresholds must be unique and buckets cumulative.
  - Bucket values must be non-negative.
  - `_gsum` must be non-`NaN`.
- Info (normalized):
  - Scalar carrier value should be `1`.
  - Info labels should be treated as metadata labels; avoid collisions with metric identity labels.
- Unknown:
  - Accept as scalar gauge-like fallback with warning, or drop by strict policy toggle.

Strict-mode failure behavior:

- Drop only the offending sample/family component, not the whole collect cycle.
- Emit structured warning with metric name + labels + violated rule.
- Increment internal adapter validation error counters for observability.
- Keep adapter output deterministic (same input + same policy => same dropped/accepted set).

### `metrix/stm` Transitional Helper

- Provide thin helper to minimize collector rewrite cost where feasible.

## Concurrency Contract

- Collection store mutable path is single-writer (collector/job cycle path).
- Functions/engine read from immutable snapshots (store-backed read views).
- Snapshot publication is atomic at cycle boundaries.
- Reader views are safe for concurrent access by design (immutable).
- Reader views are snapshot-bound: one reader instance references one immutable snapshot.
- Cycle control (`BeginCycle`/`CommitCycleSuccess`/`AbortCycle`) is runtime-owned via dedicated controller surface, not collector-facing writer API.
- Runtime/internal store is separate and thread-safe multi-writer (`59B`).

## Performance Requirements

- No per-write map-scan on hot path.
- Deterministic key creation with low allocations.
- Iteration by metric name should be indexed (avoid full-store scans by default).
- Benchmark targets:
  - key encoding cost at label cardinalities 0/1/3/10,
  - write throughput (`Observe`, `ObserveTotal`, `Set`, `Add`),
  - `ForEachByName` and matcher iteration,
  - successful-cycle counter transition cost.

## Test Matrix (Phase-2)

- Key canonicalization:
  - stable ordering,
  - nil/empty labels,
  - duplicate-key rejection.
- Label API safety:
  - `LabelSet` typed-handle contract (no arbitrary runtime types),
  - iterator labels are immutable views (mutation attempts cannot alter store state),
  - `LabelView.CloneMap()` returns detached copy.
- Numeric behavior:
  - float-only,
  - int-source conversion to float store values,
  - precision-bound behavior around `2^53`.
- Counter state:
  - first sample (`hasPrev=false`),
  - second collected sample (`hasPrev=true`),
  - strict contiguous delta availability (`currentAttemptSeq == previousAttemptSeq + 1`),
  - gap across failed attempts returns no delta,
  - missing-cycle preservation,
  - reset case (`current < previous`) returns `(current, true)`.
  - non-counter `Delta()` returns `(0, false)`.
- Cycle transaction semantics:
  - staged writes are invisible before commit,
  - `CommitCycleSuccess()` publishes staged frame atomically,
  - `AbortCycle()` discards staged frame with zero committed-state mutation.
  - `AbortCycle()` publishes metadata-only snapshot with failed attempt status.
  - write attempts without active cycle:
    - debug panic path is covered,
    - production warn+drop path is covered.
  - invalid lifecycle transitions:
    - double `BeginCycle()` follows fail-fast policy,
    - `CommitCycleSuccess()`/`AbortCycle()` without active cycle follows fail-fast policy.
- Reads/iteration:
  - direct value lookup,
  - `Value()` on counters returns committed `current`,
  - delta lookup,
  - by-name iteration,
  - matcher iteration.
- Fresh/raw visibility matrix:
  - latest attempt = success, series seen this cycle, policy `FreshnessCycle` => visible in `Read` + `ReadRaw`,
  - latest attempt = success, series unseen this cycle, policy `FreshnessCycle` => hidden in `Read`, visible in `ReadRaw`,
  - latest attempt = failed, policy `FreshnessCycle` => hidden in `Read`, visible in `ReadRaw`,
  - latest attempt = success or failed, policy `FreshnessCommitted` => visible in `Read` + `ReadRaw` (until evicted),
  - `window=cycle` summary/hist series => effective `FreshnessCycle` visibility behavior,
  - partial successful cycle => only seen `FreshnessCycle` series are visible.
- Stateful counter add semantics:
  - first in-cycle add baselines from committed current,
  - multiple in-cycle adds accumulate deterministically.
- Gauge semantics:
  - snapshot gauge unseen in successful cycle is not fresh-visible,
  - repeated `SnapshotGauge.Observe` writes in one cycle are last-write-wins,
  - stateful gauge unseen carries forward committed value and remains visible by policy.
  - `StatefulGauge.Add` first write baselines from committed value (or zero), then accumulates in-cycle.
  - `StatefulGauge.Set` overwrites staged value.
- Snapshot counter observe semantics:
  - repeated `ObserveTotal` writes in one cycle are last-write-wins.
- Reader binding semantics:
  - one reader instance remains on one snapshot even after subsequent commits,
  - reacquiring reader observes new snapshot.
  - during active cycle, reads are committed-only (no staged visibility).
- Freshness override validation:
  - snapshot + `FreshnessCommitted` is rejected by fail-fast policy,
  - stateful override combinations follow declared restrictions.
- Window option validation:
  - `WithWindow(WindowCycle|WindowCumulative)` is accepted only for stateful summary/histogram instruments,
  - `WithWindow(...)` on gauge/counter is rejected by fail-fast policy,
  - invalid window/freshness combinations are rejected by fail-fast policy.
- Histogram schema declaration validation:
  - stateful histogram without `WithHistogramBounds(...)` is rejected by declaration fail-fast policy,
  - snapshot histogram without bounds can capture schema on first successful `ObservePoint(...)`,
  - snapshot histogram schema capture is one-time; subsequent schema drift is rejected by `R10`.
- StateSet sample-shape validation:
  - `ObserveStateSet(...)` must include exactly the declared/captured state-domain keys,
  - partial/missing/extra state keys are rejected by fail-fast policy.
  - `ModeEnum` enforces exactly one true state.
  - `ModeBitSet` allows zero or more true states.
  - `Enable(...)` helper builds full replacement state samples and follows mode rules.
- Flatten naming / reserved-label validation:
  - histogram rejects base labels containing `le`,
  - summary rejects base labels containing `quantile`,
  - stateset rejects base labels containing metric-family-name label key.
- Adapters:
  - legacy int map,
  - legacy float map,
  - canonical typed histogram/summary/stateset ingestion.
  - flatten-view scalar projection correctness for typed families.
- Concurrency:
  - snapshot publication atomicity tests,
  - reader immutability/concurrent-read tests,
  - writer/read-snapshot isolation tests,
  - runtime-only cycle controller ownership tests (collector writer cannot call lifecycle methods),
  - metadata-only abort publication visibility tests.
- Runtime/internal seam:
  - dedicated runtime/internal store accessor contract tests for out-of-cycle instrumentation path.
  - runtime store exposes stateful-only writer surface (no snapshot meter path).
  - runtime obtains collection-store cycle control via internal helper/view only (not public API).
  - runtime-store metadata sequence behavior tests (`CollectMeta`/`SeriesMeta` under cycleless commits).
  - runtime store rejects `FreshnessCycle` configuration (fail-fast).
  - runtime commit sequence observable per-write behavior tests.
  - runtime store `Read()` vs `ReadRaw()` visibility behavior tests.
  - runtime store multi-writer concurrent write correctness tests.
  - runtime store wall-clock retention eviction tests.
  - runtime store max-series-cap eviction tests.
  - public constructors do not expose cycle-control capability.
- Mode/identity safety:
  - cross-meter same-identity snapshot/stateful conflict is fail-fast.
- Benchmarks:
  - write/read/transition benchmarks with realistic label cardinality.

## Decisions (Resolved)

1. Numeric range model: **Resolved (User picked float64-only on 2026-02-08)**
   - Outcome: use `float64` storage for all values in phase-2; cast to `int64` on emit paths that require integer semantics.
   - Risk note: integer precision above `2^53` can be lost in float64 representation.
2. Attribute-set mutability model: **Resolved (User picked A on 2026-02-08)**
   - A) copy/freeze attributes on record
   - B) trust caller mutability discipline
   - Outcome: A. Superseded by decisions `81B` and `132A` on 2026-02-12
     for canonical typed non-scalar families and dedicated typed writer instruments
     available on snapshot and stateful meter surfaces.
3. Selector iteration API breadth: **Resolved (User picked B on 2026-02-08)**
   - A) only by-name iteration
   - B) include matcher-based iteration now
   - Outcome: B.
4. Concurrency strategy: **Resolved (User picked A on 2026-02-08)**
   - A) explicit non-thread-safe store
   - B) thread-safe store contract
   - Outcome: A. Superseded by decision `16C` (2026-02-08).
5. Non-scalar family storage: **Resolved (User picked A on 2026-02-08)**
   - A) normalize to scalar series (phase-2)
   - B) first-class summary/histogram/state objects in core store
   - Outcome: A. Superseded by decision `81B` on 2026-02-12.
6. Metrics API declaration model (collector write contract): **Resolved (User picked B on 2026-02-08)**
   - Context:
     - current module path is undeclared map writes (`Collect() map[string]int64`) (`src/go/plugin/go.d/agent/module/module.go:32`),
     - initial phase-2 draft allowed direct writes (`SetGauge/SetCounter`) without instrument declaration,
     - existing `metrix` package already supports explicit metric objects/vectors (`NewCounterVec` + `Get(name)` + `Inc/Add`) (`src/go/plugin/go.d/pkg/metrix/counter.go:66`, `src/go/plugin/go.d/pkg/metrix/counter.go:79`) and (`NewGaugeVec` + `Get(name)` + `Set`) (`src/go/plugin/go.d/pkg/metrix/gauge.go:76`, `src/go/plugin/go.d/pkg/metrix/gauge.go:89`).
   - A) Keep direct-write API only (`SetGauge/SetCounter`), no pre-registration.
       - Pros: lowest friction for new collectors; simplest migration from map writes.
       - Cons: weaker type/contracts; typos and type flips detected only at runtime.
   - B) Require OTel-like instrument declaration first, then record via instrument handles.
       - Pros: explicit schema/type/unit/attributes; catches misuse early; aligns with your preference.
       - Cons: heavier collector boilerplate and migration cost.
   - C) Hybrid: support declaration API + direct-write compatibility mode; optional strict mode rejects undeclared writes.
       - Pros: keeps migration easy while enabling strong contracts for new collectors.
       - Cons: dual API surface to maintain.
   - Outcome: B. Modern instrument-first API is the phase-2 design target; legacy go.d patterns are not a design constraint.
7. Metric input styles in declared instruments: **Resolved (User clarified on 2026-02-08)**
   - Outcome: support two input styles:
     - scrape-like snapshot input (set/observe absolute sampled value),
     - instrument-like stateful input (maintain/change value via operations like add/set).
8. Per-instrument mode-mixing policy: **Resolved (User picked A on 2026-02-08)**
   - Context:
     - with dual input styles, same instrument may be called with both snapshot and stateful operations unless constrained.
   - A) Strict mode lock per instrument at declaration (`snapshot` OR `stateful`), mixed calls are hard errors.
       - Pros: deterministic semantics, easier testing, fewer hidden bugs.
       - Cons: less flexibility.
   - B) Allow mixed mode on one instrument (`ObserveTotal` + `Add`) with last-write-wins semantics per cycle.
       - Pros: flexible.
       - Cons: ambiguous semantics and high bug risk.
   - C) Allow mixed mode but only behind explicit opt-in per instrument.
       - Pros: controlled flexibility.
       - Cons: extra API/config surface.
   - Outcome: A.
9. V2 `Collect` metrics argument type: **Resolved (User picked C on 2026-02-08)**
   - Context:
     - user direction: keep explicit metrics injection into `Collect` (`context + metrics argument`),
     - instrument-first API is already chosen (`MeterProvider`/`Meter`/`Instrument`),
     - current V1 signature has no metrics argument (`Collect(context.Context) map[string]int64`) (`src/go/plugin/go.d/agent/module/module.go:32`).
   - A) `Collect(ctx context.Context, meter metrics.Meter) error`
       - Pros: simple collector API; meter already scoped and ready to use.
       - Cons: no access to provider-level functions inside collect.
   - B) `Collect(ctx context.Context, mp metrics.MeterProvider) error`
       - Pros: matches OTel-style root API and user preference.
       - Cons: collectors may re-resolve meter each cycle unless disciplined.
   - C) `Collect(ctx context.Context, mc metrics.CollectContext) error`, where `CollectContext` exposes a store handle and cycle-local facilities.
       - Pros: future-proof seam for cycle-scoped behavior without changing collector signature later.
       - Cons: one extra abstraction.
   - Outcome: C.
10. Provider topology for snapshot vs stateful input styles: **Resolved (User picked C and clarified topology on 2026-02-08)**
   - Context:
     - dual input styles are required (`88A`) and mode-mixing is forbidden per instrument (`8A`),
     - current API sketches a single `MeterProvider` with per-instrument mode declarations.
   - A) Single provider, single meter space; mode is per-instrument declaration.
       - Pros: one registry/index; simpler wiring in `CollectContext`; easier selector/query integration.
       - Cons: authors must pay attention to mode declarations.
   - B) Two providers (or two logical roots): one for snapshot instruments, one for stateful instruments.
       - Pros: hard separation by construction; clearer mental model in large collectors.
       - Cons: duplicated registration paths, naming collisions across roots, more plumbing in runtime/adapters.
   - C) Single provider with two explicit meter namespaces/factories (e.g. `SnapshotMeter()` / `StatefulMeter()`).
       - Pros: clearer intent than A with less overhead than B.
       - Cons: still adds API surface and namespace rules.
   - Outcome: C. Store API exposes explicit `SnapshotMeter()` and `StatefulMeter()` accessors.
11. Stateful instrument family scope: **Resolved (User clarified on 2026-02-08)**
   - Context:
     - stateful metrics are not limited to gauge/counter; existing patterns include summaries and histograms (`src/go/plugin/go.d/pkg/metrix/summary.go:19`, `src/go/plugin/go.d/pkg/metrix/histogram.go:24`).
   - A) Stateful meter supports `Gauge`, `Counter`, `Summary`, `Histogram`; snapshot meter supports `Gauge`, `Counter`.
       - Pros: matches primary use-cases while keeping snapshot path simple.
       - Cons: summary/hist lifecycle semantics still require explicit contract.
   - B) Both snapshot and stateful meters support all instrument families.
       - Pros: maximal flexibility.
       - Cons: larger API and higher misuse risk.
   - C) Stateful meter supports only `Summary`/`Histogram`, snapshot meter supports `Gauge`/`Counter`.
       - Pros: strict separation.
       - Cons: too rigid for mixed stateful gauge/counter needs.
   - Outcome: A.
12. Stateful summary/histogram window semantics: **Resolved (User picked C on 2026-02-08)**
   - Context:
     - legacy `metrix` summaries are often reset each scrape window (`src/go/plugin/go.d/pkg/metrix/summary.go:76`),
     - histogram/summary instruments in new API need explicit accumulation window semantics.
     - user clarified stateful metrics may be used outside collect-cycle scope (runtime/internal instrumentation), so collect-cycle-coupled implicit reset may be incorrect.
   - A) Cumulative-by-default (never implicit reset; only explicit reset API).
       - Pros: predictable monotonic behavior and easier deltas.
       - Cons: not identical to some current `metrix` reset-per-scrape patterns.
   - B) Collect-cycle windowed by default (implicit reset each successful cycle).
       - Pros: matches current reset-per-scrape mental model.
       - Cons: hidden state transitions and less explicit behavior.
   - C) Declared per instrument via option (`window=cumulative|cycle`).
       - Pros: explicit and flexible.
       - Cons: extra API surface.
   - D) Add plain `Reset()` method on stateful summary/histogram instruments and use it for cycle windows.
       - Pros: straightforward, close to current `metrix` summary usage.
       - Cons: reset ownership/race ambiguity and risk of non-atomic reset relative to reads.
   - Outcome: C, with runtime-managed snapshot+reset semantics for `window=cycle` (prefer atomic checkpoint API over bare `Reset()`).
13. Instrument declaration return contract: **Resolved (User clarified on 2026-02-08)**
   - Context:
     - user direction: declaration methods must not return `(instrument, error)` because that adds noisy callsites.
   - A) Return instrument only; invalid declaration is programmer/config error (panic or startup hard-fail).
       - Pros: clean collector code and fail-fast behavior.
       - Cons: requires robust validation during init/startup paths.
   - B) Return `(instrument, error)` and force handling at each declaration call.
       - Pros: explicit Go error handling style.
       - Cons: verbose and repetitive collector code.
   - Outcome: A.
14. Metrics exposure seam (`module.Base` injection vs `ModuleV2` accessors): **Resolved (User picked B on 2026-02-11)**
   - Context:
     - `module.Base` currently contains logger only (`src/go/plugin/go.d/agent/module/module.go:49`),
     - logger is injected by job (`src/go/plugin/go.d/agent/module/job.go:133`),
     - `22C` selected explicit `ModuleV2` boundary for V2 collectors,
     - `19B` selected collector-owned metric store,
     - functions already reach concrete collector through `job.Module()` cast (e.g. `src/go/plugin/go.d/collector/mysql/func_router.go:62`).
   - A) Extend `module.Base` with metrics fields and inject from job (logger-like pattern).
       - Pros: consistent with existing logger injection path.
       - Cons: broad implicit state on shared base struct used by many collectors; weaker API boundaries.
   - B) Keep `module.Base` unchanged; expose metrics via explicit `ModuleV2` methods (e.g. `MetricStore() metrics.CollectorStore`).
       - Pros: explicit V2-only contract; clean migration boundary; avoids hidden base-coupling.
       - Cons: requires runtime branching and explicit calls through `ModuleV2`.
   - C) Hybrid: keep `module.Base` unchanged for write path, but inject read-only metrics view into `Base`.
       - Pros: read convenience in `Init/Check` without widening write capability.
       - Cons: adds extra view types and still introduces implicit base-coupling.
   - Outcome: B.
15. Phase-2 top-level interface style (simplicity-first rewrite): **Resolved (User picked C on 2026-02-08, superseded by 17B on 2026-02-11)**
   - Context:
     - user direction: prioritize a simpler interface for both write and read; OTel mimic is optional (`92A`),
     - current draft favors instrument-first model with split snapshot/stateful meters.
   - A) Keep current instrument-first model (collectors declare instruments, write via handles; read via store API).
       - Pros: explicit schema and type safety.
       - Cons: heavier API surface and more concepts.
   - B) Store-first unified API (`Collect(ctx, store)`): direct write helpers + typed handle helpers + built-in read API.
       - Pros: minimal cognitive load; straightforward access for collectors and functions.
       - Cons: requires careful namespacing/validation to avoid write-time mistakes.
   - C) Two-facade model: simple writer facade for collectors + reader facade for functions/engine, both backed by one runtime store.
       - Pros: separates concerns cleanly while keeping one data model.
       - Cons: additional facade types and lifecycle wiring.
   - Outcome: C. Note: superseded by `17B`; see decision `17` for current API shape.
16. Concurrency strategy revisit (superseding strict `4A` if chosen): **Resolved (User picked C on 2026-02-08)**
   - Context:
     - current decision `4A` is non-thread-safe store,
     - user explicitly allows making it thread-safe from the beginning if it simplifies write+read and function access.
   - A) Keep non-thread-safe (`4A`) and force synchronized access at runtime boundaries.
       - Pros: simplest store internals.
       - Cons: more synchronization burden in job/function plumbing.
   - B) Make store thread-safe by default (RW lock or snapshot/copy-on-write readers).
       - Pros: simpler external usage; fewer integration hazards for functions.
       - Cons: more implementation complexity and potential overhead.
   - C) Hybrid: write path single-threaded store, read path served via atomically swapped immutable snapshots.
       - Pros: very safe reads for functions; predictable write cost.
       - Cons: snapshot memory/copy overhead.
   - Outcome: C.
17. Collector-facing API shape (reader/writer split vs unified store): **Resolved (User picked B on 2026-02-11)**
   - Context:
     - previous draft exposed explicit split APIs (`Writer()`/`Reader()`), which made simple read+write collector code noisy,
     - function handlers get direct access to collector struct via `job.Module()` cast (`src/go/plugin/go.d/collector/mysql/func_router.go:62`, `src/go/plugin/go.d/agent/module/job.go:339`),
     - in practice, collector code often needs write and later read/iterate in same flow.
   - A) Keep strict split (`MetricsWriter` + `MetricsReader`) as public collector API.
       - Pros: clear capability boundaries; easiest to reason about permissions.
       - Cons: heavier/simple-case ergonomics (extra facade navigation).
   - B) Unified collector-facing store API (single handle exposing read+write), keep internal restricted views for engine/runtime internals.
       - Pros: simplest collector/function ergonomics; read-after-write and iteration are straightforward.
       - Cons: requires stronger runtime safeguards to prevent misuse across lifecycle phases.
   - C) Mixed API: `CollectContext` exposes unified cycle handle, while `module.Base` gets read-only + stateful-write subset.
       - Pros: simpler collect path while preserving stronger constraints outside collect.
       - Cons: two public handle types remain.
   - Outcome: B.
18. Function-write policy to metrics store: **Resolved (Superseded by 21A on 2026-02-11)**
   - Context:
     - function handlers run in functions manager loop (`handler(*fn)` in `src/go/plugin/go.d/agent/functions/manager.go:94`) and can execute concurrently with job collect loops (`Job.Start` / `runOnce` in `src/go/plugin/go.d/agent/module/job.go:350` and `src/go/plugin/go.d/agent/module/job.go:493`),
     - user requirement: functions may also emit metrics (not read-only usage).
   - A) Allow function writes only to `stateful` namespace; snapshot namespace remains collect-cycle-owned.
       - Pros: preserves cycle semantics and counter transition determinism.
       - Cons: functions cannot directly publish scrape-like sampled series.
   - B) Allow function writes to both snapshot and stateful namespaces anytime.
       - Pros: maximal flexibility.
       - Cons: risks breaking cycle boundaries/delta semantics and chart update determinism.
   - C) Allow both, but snapshot writes require explicit cycle token/lease issued by runtime.
       - Pros: controlled flexibility with preserved cycle ownership.
       - Cons: highest complexity and API overhead.
   - Outcome: superseded by dedicated function-store separation (`21A`), so this policy no longer applies to collection store writes.
19. Collection metrics store ownership and access seam: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - current module contract has no metrics-store accessor (`src/go/plugin/go.d/agent/module/module.go:18`),
     - job collect loop owns collect cadence and would be the natural cycle boundary owner (`src/go/plugin/go.d/agent/module/job.go:514`),
     - function handlers access concrete collector via job cast (`src/go/plugin/go.d/collector/mysql/func_router.go:62`).
   - A) Job/runtime owns store and injects it to collector (`CollectContext.Store()` only).
       - Pros: strong runtime control of lifecycle/cycle boundaries.
       - Cons: collector/functions need extra plumbing for out-of-collect access.
   - B) Collector owns store (created in `New()`), exposes via accessor (e.g. `MetricStore()`), runtime/engine reads it through that seam.
       - Pros: simplest for function handlers and collector-local helpers; explicit ownership.
       - Cons: lifecycle hooks must be clearly runtime-driven to avoid ad hoc cycle management.
   - C) Hybrid: runtime injects collection store, collector may optionally create separate auxiliary stores (e.g. function store).
       - Pros: flexible, supports both strict runtime control and local function needs.
       - Cons: highest conceptual surface.
   - Outcome: B.
20. Collection cycle freshness/staleness contract: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - with persistent store ownership, stale-vs-current data must be unambiguous for engine reads.
   - A) Mutable in-place updates + `updated` flag only.
       - Pros: simplest internals.
       - Cons: partial-write visibility risk on failed/panicked collects; stale filtering burden on readers.
   - B) Explicit cycle transaction on store:
       - `BeginCycle()` before collect,
       - writes during collect,
       - `CommitCycleSuccess()` publishes new readable snapshot,
       - `AbortCycle()` discards cycle writes.
       - Pros: deterministic visibility and failure isolation.
       - Cons: more runtime/store wiring.
   - C) Double-buffered cycle frames with implicit commit on successful collect return.
       - Pros: less API surface than explicit transaction methods.
       - Cons: less explicit failure semantics and harder debugging.
   - Outcome: B.
21. Function metrics write path separation: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - function requests execute independently from collect loop (`src/go/plugin/go.d/agent/functions/manager.go:94`),
     - function handlers may need both DB access and metrics writes/reads.
   - A) Dedicated function store for writes; collection store is read-only from function code.
       - Pros: clean separation; avoids collect-cycle semantic corruption.
       - Cons: two stores to manage and possibly expose.
   - B) Single shared store for both collect and function writes.
       - Pros: one store only.
       - Cons: highest race/semantics risk unless heavily synchronized and namespaced.
   - C) Shared store with strict namespace split (`collect.*` vs `func.*`) and write guards.
       - Pros: one backing store while reducing collisions.
       - Cons: still more complex than fully separate stores.
   - Outcome: A.
22. Store accessor compatibility seam (incremental migration safety): **Resolved (User picked C on 2026-02-11)**
   - Context:
     - hard constraint is parallel old/new framework coexistence,
     - current `module.Module` interface is implemented by all existing collectors (`src/go/plugin/go.d/agent/module/module.go:18`),
     - adding `MetricStore()` directly to `module.Module` would force touching all collectors immediately.
   - A) Add `MetricStore()` directly to `module.Module`.
       - Pros: one obvious interface.
       - Cons: breaks incremental migration; requires sweeping repo-wide collector changes.
   - B) Keep `module.Module` unchanged; add optional extension interface for V2 collectors:
       - `type MetricStoreProvider interface { MetricStore() metrics.CollectorStore }`
       - Pros: zero breakage for legacy collectors; engine can type-assert for V2 path.
       - Cons: dynamic assertion path and split capability discovery.
   - C) Introduce `module.ModuleV2` interface for all V2 collectors (includes store + V2 collect contract).
       - Pros: explicit boundary and strong compile-time contract for V2.
       - Cons: requires additional runtime branching (`Module` vs `ModuleV2`).
   - Outcome: C (introduce explicit `module.ModuleV2` boundary for V2 collectors).
23. Function store accessor contract: **Resolved (User picked D on 2026-02-11)**
   - Context:
     - `21A` requires dedicated function write store with collection-store read-only access for functions.
     - after `17B`, `CollectorStore` is unified read+write API, so exposing collection store directly to functions would violate `21A` unless runtime wraps it in a read-only view.
   - A) Mandatory on V2 collectors:
       - `FunctionStore() metrics.CollectorStore`
       - `CollectionStoreReader() metrics.Reader`
       - Pros: predictable runtime behavior.
       - Cons: extra boilerplate for collectors with no function metrics.
   - B) Optional accessor:
       - `type FunctionStoreProvider interface { FunctionStore() metrics.CollectorStore }`
       - `type CollectionStoreReaderProvider interface { CollectionStoreReader() metrics.Reader }`
       - default runtime fallback: no-op function store + empty reader.
       - Pros: low friction; only function-heavy collectors implement.
       - Cons: more runtime conditional paths; requires warning strategy to avoid silent missing instrumentation.
   - C) Single accessor returning both:
       - `Stores() (collectReader metrics.Reader, functionStore metrics.CollectorStore)`
       - Pros: one call-site.
       - Cons: less extensible and awkward if one side is absent.
   - D) Documentation-only convention:
       - no dedicated accessor contract; function code can access collector fields directly,
       - documented rule: function metrics must use function-owned store and treat collection store as read-only.
       - Pros: zero framework/API overhead.
       - Cons: not enforceable; accidental writes to collection store can silently corrupt cycle semantics.
   - Outcome: D.
       - No dedicated framework accessor contract for function store.
       - Documented convention: function metrics use function-owned store; collection store is read-only for function code.
       - Note: hard enforcement is intentionally deferred.
24. Collection-store write authorization during active cycle: **Resolved (User deferred on 2026-02-11)**
   - Context:
     - function handlers run concurrently with collect (`src/go/plugin/go.d/agent/functions/manager.go:94` vs `src/go/plugin/go.d/agent/module/job.go:514`),
     - function code can access collector fields directly (same package pattern, e.g. `src/go/plugin/go.d/collector/mysql/func_router.go:62`),
     - therefore a simple "active cycle" boolean guard is insufficient: function writes can occur while collect cycle is active.
   - A) Keep active-cycle guard only (`write allowed when cycle active`).
       - Pros: simplest.
       - Cons: incorrect under concurrent function calls; cannot distinguish collect-writes from function-writes.
   - B) Capability-based cycle writer (recommended):
       - collection store object exposed on collector is read-only view,
       - runtime starts cycle and creates cycle-scoped writable handle (`CycleWriter`),
       - only `Collect` receives this writable handle; write APIs require this capability,
       - function code has no writable capability for collection store, even while cycle is active.
       - Pros: correct authorization model; deterministic semantics.
       - Cons: adds one more runtime type (`CycleWriter`/`CollectStore`).
   - C) Global lock + caller convention (no capability split).
       - Pros: small API delta.
       - Cons: still not enforceable by type system; accidental misuse remains possible.
   - D) Defer hard authorization to later phase; keep documentation-only convention now.
       - Pros: no additional phase-2 complexity.
       - Cons: relies on discipline; misuse remains technically possible.
   - Outcome: D.
25. `Base.MetricsStore()` default-nil pattern vs explicit `ModuleV2` accessor: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - `14B` selected explicit `ModuleV2` accessor seam,
     - proposal: add `func (b *Base) MetricsStore() metrics.CollectorStore { return nil }` so all collectors have method via embedding, and only migrated collectors override it.
   - A) Keep `14B` only (explicit `ModuleV2` accessor).
       - Pros: explicit V2 capability boundary; compile-time clarity.
       - Cons: requires type assertions/branching for `ModuleV2`.
   - B) Add default-nil `Base.MetricsStore()` and use nil-check for capability.
       - Pros: very low migration friction; no extra interface needed for accessor discovery.
       - Cons: capability becomes implicit and easy to misuse; nil-handling becomes runtime footgun; weak signal for migration completeness.
   - C) Hybrid: keep `14B` as canonical, but also add default `Base.MetricsStore()` that returns a no-op store (not nil) for convenience.
       - Pros: convenience without nil panics.
       - Cons: still blurs capability boundary and can hide misconfiguration.
   - Outcome: A.
26. Label naming consistency (`AttrSet` vs `LabelSet`): **Resolved (User picked rename on 2026-02-11)**
   - Context:
     - current draft uses `Labels` for read paths and `Attr`/`AttrSet` for write paths, which is semantically equivalent but cognitively inconsistent.
   - A) Keep `Attr`/`AttrSet` names.
       - Pros: mirrors OpenTelemetry terminology.
       - Cons: confusing mixed terminology in go.d V2 API.
   - B) Rename to `Label`/`LabelSet` on write side.
       - Pros: consistent terminology across read/write APIs.
       - Cons: departs from OTel naming.
   - Outcome: B.
27. Meter-level default labels ergonomics: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - current API requires passing `LabelSet` on each write call (`Observe`, `ObserveTotal`, `Add`, `Set`) in the contract block (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - user proposal: bind shared labels once at meter scope to reduce repeated boilerplate in collectors.
   - A) Constructor-level binding:
       - `SnapshotMeter(prefix string, labels ...Label) SnapshotMeter`
       - `StatefulMeter(prefix string, labels ...Label) StatefulMeter`
       - Pros: shortest callsites for one common label set.
       - Cons: less composable for nested scopes; recreates meter instances for each label variation.
   - B) Keep constructor stable and add meter scoping method:
       - `WithLabelSet(labels LabelSet) SnapshotMeter`
       - `WithLabelSet(labels LabelSet) StatefulMeter`
       - Pros: composable and chainable scope (`instance` -> `database`), less object churn.
       - Cons: one extra method/concept.
   - C) Support both constructor labels and `WithLabelSet`.
       - Pros: flexible.
       - Cons: larger API surface and duplication.
   - Outcome: B.
28. Write call label argument ergonomics: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - user proposal: allow omitting per-call labels when meter scope already provides defaults, e.g. `Observe(v)` and optionally `Observe(v, extra...)`.
   - A) Keep required `LabelSet` argument (`Observe(v, labels)`).
       - Pros: explicit at callsite.
       - Cons: verbose in common shared-label patterns.
   - B) Variadic optional label sets:
       - `Observe(v SampleValue, labels ...LabelSet)` (same for `Set/Add/ObserveTotal`).
       - Pros: ergonomic default `Observe(v)`; still supports override/extra labels.
       - Cons: requires strict merge semantics and validation.
   - C) Split methods:
       - `Observe(v)` and `ObserveWithLabels(v, labels LabelSet)`.
       - Pros: explicit and avoids variadic ambiguity.
       - Cons: larger API surface.
   - Outcome: B.
29. Label merge/override semantics for scoped meter + per-call labels: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - if meter has scoped labels and write call also provides labels, merge behavior must be deterministic.
   - A) Disallow overlapping keys (return error/panic on conflict).
       - Pros: catches ambiguity early.
       - Cons: stricter; may require extra collector code.
   - B) Per-call labels override scoped labels on key conflict.
       - Pros: flexible for occasional overrides.
       - Cons: easier to hide mistakes.
   - C) Scoped labels override per-call labels (per-call conflict ignored/rejected).
       - Pros: stable meter identity.
       - Cons: less flexible.
   - Outcome: A.
30. Meter prefix API shape (`SnapshotMeter(...)` args vs `WithPrefix(...)`): **Resolved (User picked A on 2026-02-11)**
   - Context:
     - previous contract used `SnapshotMeter(name string, version string)` / `StatefulMeter(name string, version string)`,
     - label scoping already moved to chainable meter methods (`WithLabelSet(...)`) via `27B`,
     - user asks whether prefix should also be a chainable scope or constructor argument.
   - A) Keep prefix in constructor:
       - `SnapshotMeter(prefix string)` / `StatefulMeter(prefix string)`
       - optional `WithPrefix(...)` for nested prefixes if needed.
       - Pros: clear required identity at creation time; fewer runtime checks; simple/fast.
       - Cons: slightly less uniform with `WithLabelSet`.
   - B) No-arg constructor + required prefix scoping:
       - `SnapshotMeter()` / `StatefulMeter()`, then `.WithPrefix("apache")`.
       - Pros: fully uniform scoping style (`WithPrefix`, `WithLabelSet`).
       - Cons: easy to forget prefix; requires validation on first instrument call; noisier callsite.
   - C) Support both equally.
       - Pros: flexible.
       - Cons: redundant API and inconsistent team usage.
   - Outcome: A.
       - Constructor takes required prefix.
       - Meter version is removed from constructor inputs in phase-2 API.
31. User-facing label API ergonomics (`LabelSet(...)` explicit vs internal compilation): **Resolved (User picked C on 2026-02-11)**
   - Context:
     - current contract exposes meter-level `LabelSet(labels ...Label) LabelSet` and APIs like `WithLabelSet(...)`,
     - example callsite is verbose: `sm.WithLabelSet(c.store.LabelSet(metrics.Label{...}))`,
     - user asks whether raw labels should be accepted and internally compiled for efficiency.
     - Go does not support generic methods on interfaces/types for this use-case; if one method is desired, it should be done via a shared argument interface + runtime type handling.
   - A) Keep explicit `LabelSet` construction in callsites.
       - Pros: performance intent is explicit; no hidden conversions.
       - Cons: noisy API and leaks internal optimization detail into collector code.
   - B) Hide compilation fully: accept raw labels in user-facing methods, compile internally.
       - Pros: simplest collector ergonomics.
       - Cons: no explicit precompiled fast path for hot loops.
   - C) Dual path (recommended):
       - ergonomic default methods accept raw labels (`WithLabels(...Label)`),
       - optional optimized methods accept compiled sets (`WithLabelSet(...LabelSet)`),
       - meter-level `LabelSet(...)` helper kept as advanced/perf API.
       - Pros: clean default API + available optimization when profiling justifies it.
       - Cons: slightly larger API surface.
   - D) Single method accepting both raw and compiled labels:
       - `WithLabels(labels ...LabelArg)` where `LabelArg` can be `Label` or `LabelSet`,
       - same pattern for write calls (`Observe(v, labels ...LabelArg)` etc),
       - implementation compiles/merges internally and applies conflict rule from `29A`.
       - Pros: one method only; concise callsites; still supports precompiled fast path.
       - Cons: internal runtime type-switch and validation logic is more complex.
   - Outcome: C.
32. Store API organization (flat methods vs explicit `Read/Write` groups): **Resolved (User picked B on 2026-02-11)**
   - Context:
     - current `CollectorStore` contract places read/write grouped views on one top-level interface (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - user feedback: separate groups would make intent clearer at callsites.
   - A) Keep flat `CollectorStore` interface (current shape).
       - Pros: fewer types; shortest calls (`store.Value(...)`, `store.SnapshotMeter(...)`).
       - Cons: mixed concerns in one surface; less explicit read/write intent.
   - B) One store with explicit grouped views (recommended):
       - `store.Write().SnapshotMeter(...)`
       - `store.Read().Value(...)`
       - both views are backed by same underlying store.
       - Pros: explicit intent, cleaner mental model, preserves single-store architecture.
       - Cons: one extra method call and a couple of interface types.
   - C) Full split at context boundary (`CollectContext.Write()` / `CollectContext.Read()`), separate from store object.
       - Pros: strongest separation.
       - Cons: reintroduces the split model we intentionally simplified away.
   - Outcome: B.
33. Store cleanup policy for stale series (instances come/go): **Resolved (User picked B on 2026-02-11)**
   - Context:
     - old map-return path naturally drops absent series each cycle (`Collect()` returns fresh maps; absent keys disappear) (`src/go/plugin/go.d/agent/module/job.go:528`..`532`),
     - phase-2 store is persistent (needed for `Delta`/history), so absent series can accumulate unless evicted,
     - phase-1 chart lifecycle already uses successful-cycle-based expiration for chart instances/dimensions (`src/go/plugin/go.d/TODO-codex-godv2/phase1-templates.md:48`, `src/go/plugin/go.d/TODO-codex-godv2/phase1-templates.md:404`).
   - A) No store-level eviction in phase-2 (engine-only cleanup later).
       - Pros: simplest now.
       - Cons: unbounded store growth risk; stale series pollute reads/functions.
   - B) Built-in store eviction by successful-cycle staleness (recommended):
       - track `lastSeenCycle` per series,
       - on `CommitCycleSuccess()`, evict series not seen for `store_expire_after_cycles` (default),
       - optional global `max_series` hard cap with deterministic LRU-by-lastSeen eviction.
       - Pros: bounded memory, deterministic behavior, aligned with phase-1 lifecycle clock semantics.
       - Cons: introduces retention config and eviction code in phase-2.
   - C) Manual explicit prune API only:
       - runtime/engine calls `Prune(matcher|predicate)` on policy schedule.
       - Pros: flexible and explicit.
       - Cons: easy to forget; policy fragmented across components.
   - Outcome: B.
34. Eviction parameter source and application timing (for `33B`): **Resolved (User picked B on 2026-02-11)**
   - Context:
     - collector config is loaded before `Init()` and used for runtime behavior (`src/go/plugin/go.d/collector/apache/collector.go:47`, `src/go/plugin/go.d/collector/mysql/collector.go:70`),
     - `Init()` runs before regular collect loop and before job start checks (`src/go/plugin/go.d/agent/module/module.go:22`, `src/go/plugin/go.d/agent/module/job.go:454`),
     - collector-owned store is chosen (`19B`), so policy wiring belongs close to collector/store creation.
   - A) Fixed internal defaults only (no user config), set in `metrics.NewCollectorStore(...)`.
       - Pros: simplest rollout.
       - Cons: no tuning for high-cardinality collectors/environments.
   - B) Collector-configurable policy (recommended):
       - add optional collector config block (e.g. `metrics_store`) with:
         - `expire_after_cycles`
         - `max_series` (optional hard cap)
       - default constructor is zero-arg `metrics.NewCollectorStore()` (built-in defaults),
       - apply config overrides in `Init()` before first collect/check.
       - Pros: explicit per-job control; stable behavior after reloads; no global hidden knobs.
       - Cons: config/schema updates for migrated collectors.
   - C) Runtime/job-level policy only (job manager injects policy to store).
       - Pros: centralized tuning.
       - Cons: conflicts with collector-owned store model; extra runtime coupling.
   - Outcome: B.
35. Store constructor shape for retention defaults: **Resolved (User picked default constructor on 2026-02-11)**
   - Context:
     - most collectors will use default retention policy.
   - A) `metrics.NewCollectorStore(defaultRetentionPolicy)` required at callsite.
       - Pros: explicit policy at construction.
       - Cons: noisy for common case; repeats defaults across collectors.
   - B) `metrics.NewCollectorStore()` with built-in defaults (selected), plus optional future advanced API:
       - `metrics.NewCollectorStoreWithRetention(...)` or options pattern when needed.
       - Pros: minimal boilerplate; clean migration path; preserves extensibility.
       - Cons: default policy is less visible at callsite.
   - Outcome: B.
36. Eviction aging clock for stale-series retention: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - phase-1 lifecycle semantics advance expiry on successful collect cycles (`src/go/plugin/go.d/TODO-codex-godv2/phase1-templates.md:48`, `src/go/plugin/go.d/TODO-codex-godv2/phase1-templates.md:410`),
     - on failed cycles, collector does not produce a reliable world-state diff (no trustworthy observed "missing" entities),
     - stale-series memory growth is primarily caused by successful cycles discovering new label sets, not by failed cycles.
   - A) Advance stale counters only on successful cycles (current aligned rule).
       - Pros: correctness-first; avoids evicting valid entities during transient outages; aligned with phase-1 lifecycle model.
       - Cons: stale entries remain longer during prolonged failure periods.
   - B) Advance stale counters on both successful and failed cycles.
       - Pros: faster cleanup when collector is failing for long periods.
       - Cons: can evict healthy-but-unobserved entities due to temporary source/network failures; causes unnecessary metrics-store state churn on recovery.
   - C) Hybrid:
       - primary clock = successful cycles (A),
       - optional safety wall-clock TTL to cap extreme long-lived stale entries regardless of success/failure.
       - Pros: keeps correctness model while bounding very long stale retention.
       - Cons: extra policy parameter/complexity.
   - Recommendation: A for phase-2 baseline; consider C later if real memory pressure appears.
   - Outcome: A.
37. Read freshness behavior after failed collect cycles: **Resolved (User picked C on 2026-02-11)**
   - Context:
     - user concern: returning previous values after failed collects can mislead real-time function users,
     - prior draft read API returned only `(value, ok)` without freshness metadata (`Value`, `Delta`),
     - collect status already exists conceptually in runtime (`success`/`failed` status chart path in `src/go/plugin/go.d/agent/module/job.go:627`..`636`).
   - A) Keep current behavior (return last committed value) with no extra freshness metadata.
       - Pros: simplest.
       - Cons: callers cannot distinguish fresh vs stale data after failures.
   - B) On failed cycle, invalidate all series reads (`ok=false`) until next successful collect.
       - Pros: no risk of showing stale data as current.
       - Cons: harsh behavior; loses continuity for functions/charts that can tolerate stale-last-good values.
   - C) Dual-clock model (recommended):
       - retention/eviction clock remains success-based (`33B`/`36A`),
       - freshness age advances on every collect attempt (success + fail) or wall-clock age,
       - read API exposes freshness metadata (e.g. `CollectMeta`, `SeriesMeta`),
       - functions can enforce policy (`require_fresh=true` or `max_stale_cycles=0`) and return unavailable when stale.
       - Pros: preserves correctness for lifecycle while giving real-time callers explicit stale detection/control.
       - Cons: extends read API and function logic.
   - Outcome: C.
   - Note:
       - chart cleanup is not performed on failed collections (separate lifecycle rule);
       - freshness policy applies to read semantics, not chart removal behavior.
38. Counter delta continuity across collect-attempt gaps: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - user scenario: counter seen at `100`, then many failed collects, then seen at `100000`,
     - naive `current - previous` across this gap is misleading for real-time/function use.
   - A) Strict contiguous delta (recommended):
       - delta is valid only for consecutive sample attempt sequences,
       - when sample continuity is broken, `Delta` returns unavailable (`ok=false`).
       - Pros: correctness-first; avoids fabricated post-outage deltas.
       - Cons: fewer delta points during unstable periods.
   - B) Relaxed bridged delta:
       - always compute `current - previous`, even across gaps.
       - Pros: always returns a value.
       - Cons: can materially misrepresent interval work.
   - C) Dual API:
       - strict delta default + optional bridged delta accessor.
       - Pros: flexibility.
       - Cons: larger API surface and misuse risk.
   - Outcome: A.
39. Reader behavior for not-fresh series: **Resolved (User picked C on 2026-02-11)**
   - Context:
     - prior draft read contract exposed freshness metadata (`CollectMeta`, `SeriesMeta`) and left filtering to callers,
     - user question: should reader hide not-fresh metrics so reads/iteration do not find them.
   - A) Hard-hide stale series in base reader:
       - `Value`/`Family`/iterators skip stale series automatically.
       - Pros: simplest caller behavior; low misuse risk.
       - Cons: loses explicit observability/debuggability; stale visibility unavailable unless separate debug API is added.
   - B) Keep base reader raw; caller checks freshness metadata.
       - Pros: maximal visibility and explicitness.
       - Cons: easier caller mistakes (forget freshness checks).
   - C) Dual-read modes (recommended):
       - default reader exposes only fresh series (`Read()`),
       - explicit raw reader for diagnostics and advanced use (`ReadRaw()` or read option).
       - Pros: safe default with explicit escape hatch; keeps diagnostics possible.
       - Cons: adds one more reader surface/mode.
   - Recommendation: C.
   - Outcome: C.
40. Partial successful cycle visibility contract: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - successful collect cycles can be partial (some metric families/series observed, others not),
     - without cycle-scoped seen tracking, reader could accidentally expose stale series as fresh.
   - A) Per-cycle seen tracking (selected):
       - writes mark series as `seen` in active cycle frame,
       - `BeginCycle()` resets seen state logically for new frame,
       - `CommitCycleSuccess()` updates `LastSeenSuccessSeq` only for seen series,
       - `Read()` (fresh-only) on successful attempt exposes only seen series.
       - Pros: exact partial-cycle freshness behavior; matches user requirement.
       - Cons: requires cycle-frame bookkeeping.
   - B) Keep last-value behavior and require explicit freshness checks per caller.
       - Pros: simpler internals.
       - Cons: higher misuse risk; stale leakage in default reads.
   - C) Family-level seen tracking only (not per series).
       - Pros: lower bookkeeping cost.
       - Cons: too coarse; incorrect with partial label-series updates inside one family.
   - Outcome: A.
41. Commit model consistency (staged vs in-place writes): **Resolved (User picked A on 2026-02-11)**
   - Context:
     - prior draft had contradictory semantics: in-place current updates during writes, plus staged-discard semantics on abort.
   - A) Explicit staged cycle frame (recommended):
       - writes update staged frame only during active cycle,
       - commit promotes staged to committed snapshot and shifts counter committed `current -> previous`,
       - abort drops staged frame with zero effect on committed snapshot.
       - Pros: resolves semantic contradiction; correct delta and abort behavior.
       - Cons: requires explicit two-frame implementation contract.
   - B) In-place mutable current + rollback on abort.
       - Pros: simpler-looking write path.
       - Cons: complex/fragile rollback; high correctness risk.
   - Outcome: A.
42. Freshness scope for stateful instruments: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - stateful metrics may be used outside collect-cycle scope (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - prior draft treated all series as cycle-fresh in `Read()`.
   - A) Keep fresh-only visibility across all instrument types in collection store.
       - Pros: single predictable rule.
       - Cons: stateful series not updated each cycle disappear from `Read()`.
   - B) Fresh-only applies to snapshot metrics; stateful metrics use last-committed visibility unless explicitly configured fresh-only (recommended).
       - Pros: preserves stateful semantics outside scrape cadence.
       - Cons: slightly more complex read policy.
   - C) Split stores by intent (collection-snapshot store vs stateful-runtime store) and keep current fresh-only semantics per collection store.
       - Pros: strongest semantic isolation.
       - Cons: more plumbing and migration overhead.
   - Outcome: B.
43. Label type safety in public API: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - prior draft used untyped/mutable label surfaces (`LabelSet interface{}` and map labels in iterators).
   - A) Keep current types and enforce discipline by docs/debug checks.
       - Pros: minimal API changes.
       - Cons: runtime type ambiguity and mutation/race footguns.
   - B) Introduce typed immutable handles/views (recommended):
       - replace empty-interface `LabelSet` with opaque typed handle,
       - expose immutable label view in iterators (or copied labels with explicit cost contract).
       - Pros: safer API; clearer performance profile.
       - Cons: API adjustment before implementation.
   - Outcome: B.
44. Reader snapshot lifetime semantics: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - prior draft had `Read()` / `ReadRaw()` but did not specify reader snapshot binding semantics.
   - A) Reader view is bound to one immutable snapshot at creation time (recommended).
       - Pros: deterministic multi-call consistency.
       - Cons: caller must reacquire reader to see newer data.
   - B) Reader methods always resolve latest snapshot dynamically.
       - Pros: fewer reader reacquisitions.
       - Cons: intra-callset inconsistency risk during snapshot swaps.
   - Outcome: A.
45. Cycle control surface ownership: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - prior draft exposed `BeginCycle/Commit/Abort` on collector-facing writer API,
     - runtime is intended cycle owner (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - hard authorization is deferred (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Keep public methods; rely on convention.
       - Pros: simplest now.
       - Cons: easy misuse by collectors/functions.
   - B) Separate runtime-only cycle controller from collector writer API (recommended).
       - Pros: strong boundary, fewer accidental lifecycle bugs.
       - Cons: one additional internal interface/type.
   - Outcome: B.
46. Test matrix completeness for new freshness rules: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - test matrix currently lacks explicit cases for `Read` vs `ReadRaw`, failed-attempt visibility, partial successful cycles, and strict contiguous delta (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Keep current matrix and rely on implementation-time additions.
       - Pros: less upfront spec work.
       - Cons: freeze may miss regressions in core semantics.
   - B) Expand matrix now with explicit scenario table (recommended).
       - Pros: implementation target is unambiguous; catches regressions early.
       - Cons: larger spec section.
   - Outcome: B.
47. Runtime cycle-control capability leakage through type assertions: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - collector-facing `CollectContext.Store()` returns `CollectorStore` (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - runtime-only API is modeled by `RuntimeStore` + `CycleController` (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - if collectors receive a concrete type implementing `RuntimeStore`, they can type-assert and invoke cycle methods, bypassing decision `45B`.
   - A) Keep as-is and rely on convention.
       - Pros: no extra plumbing.
       - Cons: weak boundary; easy accidental misuse.
   - B) Runtime passes collector a non-runtime wrapper that implements `CollectorStore` only (recommended).
       - Pros: enforces `45B` boundary by construction.
       - Cons: adds one wrapper/view type.
   - C) Move `RuntimeStore` and cycle controller types to internal package only.
       - Pros: strongest compile-time boundary.
       - Cons: tighter package coupling and slightly more refactor.
   - Outcome: B.
   - Clarification:
       - store ownership remains collector-side (`New()` + `MetricStore() metrics.CollectorStore`),
       - `CollectContext.Store()` exposes the same underlying store via restricted `CollectorStore` view,
       - runtime holds cycle-control capability via internal/runtime-only view.
48. Write behavior when no active cycle frame exists: **Resolved (User picked C on 2026-02-11)**
   - Context:
     - write contract is staged-cycle only (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - write methods have no error return, so failure policy must be explicit.
   - A) Silent no-op.
       - Pros: minimal disruption.
       - Cons: hides bugs.
   - B) Panic always.
       - Pros: strict correctness.
       - Cons: can crash plugin in production due to misuse.
   - C) Debug panic + production warn+drop (recommended).
       - Pros: catches during development; safe in production.
       - Cons: behavior differs by build/runtime mode.
   - Outcome: C.
49. Phase-2 scope for stateful writes outside collect cycle: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - decision `12C` context allows stateful usage outside collect-cycle scope (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - current lifecycle requires active staged cycle for all writes (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`) and cycle control is runtime-only (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Phase-2 collection store is cycle-scoped only; runtime/internal out-of-cycle instrumentation must use separate dedicated store (recommended).
       - Pros: clear semantics and simpler correctness model.
       - Cons: requires second store path for runtime/internal metrics.
   - B) Allow out-of-cycle stateful writes in same store.
       - Pros: one store for everything.
       - Cons: reintroduces concurrency/ordering complexity against staged commit model.
   - C) Hybrid namespace split in one store (`collect.*` cycle-scoped, `runtime.*` out-of-cycle).
       - Pros: shared infrastructure with explicit split.
       - Cons: added complexity and policy surface.
   - Outcome: A.
50. `StatefulCounter.Add` baseline semantics at cycle start: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - API defines `StatefulCounter.Add(delta)` (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - prior draft did not specify first `Add` baseline for staged frame.
   - A) Baseline from previous committed `current`, then accumulate deltas in-cycle (recommended).
       - Pros: intuitive additive stateful behavior.
       - Cons: requires explicit staged initialization rule.
   - B) Baseline at zero each cycle (delta sum only per-cycle).
       - Pros: simple implementation.
       - Cons: behaves unlike long-lived stateful counter total.
   - C) Remove `StatefulCounter.Add`, require `ObserveTotal` style only.
       - Pros: one counter semantic.
       - Cons: loses explicit stateful delta API already selected.
   - Outcome: A.
51. Gauge commit semantics parity (snapshot vs stateful): **Resolved (User picked A on 2026-02-11)**
   - Context:
     - commit semantics are explicit for counters (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - prior draft did not specify gauge commit behavior under partial cycles equivalently.
   - A) Add explicit gauge rules:
       - snapshot gauge: visible only when seen in successful cycle (`FreshnessCycle`);
       - stateful gauge: carry forward last committed value when unseen (`FreshnessCommitted` by default). (recommended)
       - Pros: removes ambiguity and aligns with `109B`.
       - Cons: longer lifecycle spec section.
   - B) Keep implicit “according to freshness policy” only.
       - Pros: shorter doc.
       - Cons: behavior can drift in implementation.
   - Outcome: A.
52. Freshness and window interaction for stateful summary/histogram: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - window semantics are per instrument (`window=cumulative|cycle`) via decision `12C` (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - freshness defaults changed via decision `42B` (`FreshnessCommitted` for stateful by default).
   - A) `window=cycle` implies `FreshnessCycle` automatically (recommended).
       - Pros: coherent cycle-window visibility.
       - Cons: one implicit coupling rule.
   - B) Keep window and freshness independent.
       - Pros: maximum flexibility.
       - Cons: easy contradictory configs (`window=cycle` + `FreshnessCommitted`).
   - C) Forbid contradictory combinations at declaration time.
       - Pros: explicit safety.
       - Cons: stricter validation surface.
   - Outcome: A.
53. Reader behavior for read-after-write during active collect cycle: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - reader views are snapshot-bound (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - writes are staged until commit (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - decision `17B` motivation favored simple collector read+write ergonomics in same flow.
   - A) Keep strict committed-only reads (recommended):
       - `Read()`/`ReadRaw()` during collect show last committed snapshot only (no staged visibility).
       - Pros: simple consistency model; no partial-write leakage.
       - Cons: no in-cycle read-after-write from collector code.
   - B) Add staged read view for collect path (`CollectContext.StagedRead()` or writer query helpers).
       - Pros: supports read-after-write use-cases without affecting external readers.
       - Cons: additional API surface and complexity.
   - C) Make `Read()` merge staged + committed while cycle active.
       - Pros: seemingly intuitive during collect.
       - Cons: breaks snapshot-bound determinism and complicates concurrency semantics.
   - Outcome: A.
54. AbortCycle metadata publication semantics: **Resolved (User picked A on 2026-02-11)**
   - Context:
     - `AbortCycle` updates `CollectMeta.LastAttemptStatus` to failed (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - reader views are immutable snapshots (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Publish metadata-only snapshot on abort (recommended).
       - data series remain from last committed success, but collect meta is updated to failed atomically.
       - Pros: preserves freshness signaling correctness after failures.
       - Cons: one extra snapshot publication path.
   - B) Do not publish on abort; keep previous snapshot/meta.
       - Pros: simpler implementation.
       - Cons: readers/functions cannot observe failed attempt promptly (breaks `37C` intent).
   - C) Publish status out-of-band (not part of reader snapshot).
       - Pros: avoids snapshot churn.
       - Cons: split-brain state between data and metadata.
   - Outcome: A.
55. Mode-specific freshness override allowance: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - defaults are mode-specific (`FreshnessCycle` for snapshot, `FreshnessCommitted` for stateful) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - `WithFreshnessPolicy(...)` allows overrides (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Allow any override for any mode.
       - Pros: maximum flexibility.
       - Cons: easy semantic misconfiguration.
   - B) Restrict overrides by mode (recommended):
       - snapshot instruments cannot set `FreshnessCommitted`,
       - stateful instruments can set either policy (with `window=cycle` implying cycle freshness).
       - Pros: guards against contradictory snapshot semantics.
       - Cons: slightly stricter validation.
   - C) Forbid overrides entirely; defaults only.
       - Pros: simplest mental model.
       - Cons: loses useful controlled flexibility.
   - Outcome: B.
56. Fail-fast policy surface unification (conflicts and invariant violations): **Resolved (User picked A on 2026-02-11)**
   - Context:
     - some invalid operations use panic/warn-drop policy (`48C`),
     - label conflict and declaration conflicts are described as fail-fast (`13A`, `29A`, `101A`) but runtime behavior is not unified.
   - A) Uniform policy (recommended):
       - debug: panic,
       - production: structured warn+drop (sample-level where possible).
       - Pros: consistent operator experience and safer production behavior.
       - Cons: requires explicit policy application table.
   - B) Panic for all fail-fast conditions in all modes.
       - Pros: strict correctness.
       - Cons: higher production crash risk.
   - C) Warn+drop for all fail-fast conditions in all modes.
       - Pros: uptime-friendly.
       - Cons: bugs can be masked.
   - Outcome: A.
57. Dedicated runtime/internal store seam in phase-2 contract: **Resolved (User picked B on 2026-02-11)**
   - Context:
     - decision `49A` requires separate store path for out-of-cycle instrumentation,
     - current contract names this requirement but does not define minimal seam/accessor.
   - A) Document-only (no explicit API seam in phase-2).
       - Pros: no extra API now.
       - Cons: implementers may invent divergent patterns.
   - B) Define minimal seam now (recommended):
       - e.g. `ModuleV2.RuntimeStore() metrics.RuntimeStore` or runtime-owned registry per job.
       - Pros: consistent migration path and predictable ownership.
       - Cons: one more explicit contract to maintain.
   - C) Defer seam definition to phase-3.
       - Pros: keep phase-2 narrower.
       - Cons: phase-2 is not fully implementation-complete for runtime metrics usage.
   - Outcome: B.
   - Clarification:
       - phase-2 contract includes explicit dedicated runtime/internal seam
         (e.g. `RuntimeMetricsStoreProvider { RuntimeStore() RuntimeStore }`).
58. Runtime/internal store write semantics (cycle-scoped vs immediate): **Resolved**
   - Context:
     - phase-2 collection store writes are valid only inside active cycle (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - `49A` requires out-of-cycle runtime/internal instrumentation using separate dedicated store path (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - `57B` adds seam but does not define write semantics of that runtime store (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Runtime store uses same cycle model as collection store.
       - Pros: one semantic model.
       - Cons: runtime code must drive cycles explicitly; awkward for ad-hoc instrumentation.
   - B) Runtime store is cycleless immediate-commit stateful store (recommended).
       - Pros: natural for runtime/internal instrumentation; avoids cycle-management overhead.
       - Cons: introduces second semantic model (collection vs runtime stores).
   - C) Defer runtime store write semantics to phase-3.
       - Pros: less phase-2 scope.
       - Cons: phase-2 seam remains underspecified.
   - Outcome: B.
   - Clarification:
       - runtime/internal store writes are cycleless immediate-commit stateful operations,
         while collection store remains cycle-scoped with staged commit/abort.
59. Runtime/internal store concurrency model: **Resolved**
   - Context:
     - collection store is single-writer by contract (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - runtime/internal instrumentation may come from multiple goroutines (function handlers, background routines).
   - A) Runtime store single-writer only; callers must serialize externally.
       - Pros: simplest internals.
       - Cons: easy misuse and data races without strict wrappers.
   - B) Runtime store thread-safe multi-writer (recommended).
       - Pros: robust for real runtime instrumentation patterns.
       - Cons: additional synchronization overhead.
   - C) Hybrid: single-writer by default with optional locked wrapper.
       - Pros: flexible.
       - Cons: more API variants and complexity.
   - Outcome: B.
   - Clarification:
       - runtime/internal store supports concurrent writes from multiple goroutines;
         collection store single-writer contract remains unchanged.
60. Runtime/internal store retention policy: **Resolved**
   - Context:
     - retention/eviction rules are cycle-based for collection store (`33B` + `36A`),
     - runtime store may be cycleless (if `58B`), so cycle-based retention does not apply directly.
   - A) No retention/eviction for runtime store.
       - Pros: simplest.
       - Cons: unbounded growth risk for high-cardinality runtime labels.
   - B) Wall-clock retention + max-series cap (recommended).
       - Pros: bounded memory without cycle dependency.
       - Cons: needs additional policy knobs.
   - C) Manual prune API only.
       - Pros: explicit control.
       - Cons: operational burden and easy to forget.
   - Outcome: B.
   - Clarification:
       - runtime/internal store retention is wall-clock based with max-series caps;
         collection-store cycle-based retention rules are unchanged.
61. `LabelSet` handle validity and forgery behavior: **Resolved**
   - Context:
     - public `LabelSet` is opaque typed handle (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - callers can still construct zero/default handles directly in Go, so validity behavior must be explicit.
   - A) Zero/default `LabelSet` means empty labels; unknown handles are ignored.
       - Pros: permissive and simple.
       - Cons: can hide bugs from forged/invalid handles.
   - B) Runtime validates handles; invalid/foreign handles follow fail-fast policy (recommended).
       - Pros: preserves safety intent of typed handle design.
       - Cons: requires internal handle registry/checks.
   - C) Replace `LabelSet` value type with interface/private token to prevent direct construction.
       - Pros: stronger type-level safety.
       - Cons: API and implementation complexity.
   - Outcome: B.
   - Clarification:
       - only handles created by the same store are accepted;
         invalid/foreign handles follow unified fail-fast behavior.
62. Declaration-time panic handling path (startup hard-fail semantics): **Resolved**
   - Context:
     - `13A` chose instrument declaration methods returning no error with fail-fast behavior,
     - `123A` defines runtime fail-fast policy, but startup/declaration panic-to-error flow is not explicitly bound to job lifecycle.
   - A) Allow declaration panics to crash plugin process.
       - Pros: strictest fail-fast.
       - Cons: too disruptive operationally.
   - B) Recover declaration panics in init/check path and convert to job init/check error (recommended).
       - Pros: fail-fast per-job while keeping plugin process alive.
       - Cons: needs explicit recovery boundary and error wrapping.
   - C) Require collectors to self-recover and return errors.
       - Pros: no framework recovery layer.
       - Cons: inconsistent implementation quality across collectors.
   - Outcome: B.
   - Clarification:
       - declaration-time panic recovery is done at framework init/check boundaries and
         returned as job-scoped init/check errors (plugin process stays alive).

63. Runtime store instrument surface (stateful-only vs full `CollectorStore`): **Resolved**
   - Context:
     - `58B` defines runtime/internal store as cycleless immediate-commit **stateful** model (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - earlier seam used generic `CollectorStore` semantics for runtime path, which included `SnapshotMeter(...)`,
     - snapshot semantics are collect-cycle/freshness oriented in this phase (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Keep full `CollectorStore` surface for runtime store and define snapshot writes as immediate-commit.
       - Pros: one API surface everywhere.
       - Cons: blurs snapshot-vs-stateful contract; increases semantic ambiguity.
   - B) Runtime store exposes stateful-only writer surface (recommended).
       - Pros: matches `58B` exactly; removes ambiguous snapshot behavior out of cycle.
       - Cons: introduces one extra runtime-store interface.
   - C) Keep full `CollectorStore`, but runtime store rejects `SnapshotMeter(...)` by fail-fast policy.
       - Pros: no new interface.
       - Cons: API advertises capabilities that are intentionally unusable.
   - Outcome: B.
   - Clarification:
       - runtime/internal store uses a dedicated `RuntimeStore` interface with stateful-only
         writer surface (`RuntimeWriter.StatefulMeter(...)`).

64. Missing API for summary/histogram window declaration (`12C`): **Resolved**
   - Context:
     - `12C` resolved per-instrument window semantics (`window=cumulative|cycle`) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - current contract declares `WithFreshnessPolicy(...)` and `WithWindow(...)` as instrument options (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - lifecycle already depends on `window=cycle` behavior (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Add explicit window option API now (recommended):
       - `type MetricWindow int { WindowCumulative, WindowCycle }`
       - `func WithWindow(w MetricWindow) InstrumentOption`
       - Pros: completes contract and keeps `12C` implementable.
       - Cons: one more option type.
   - B) Encode window indirectly through freshness option only.
       - Pros: smaller API.
       - Cons: loses explicit semantic dimension and conflicts with `12C` intent.
   - C) Defer window API to phase-3 and keep phase-2 wording only.
       - Pros: smaller phase-2 surface.
       - Cons: contract is internally incomplete for selected behavior.
   - Outcome: A.
   - Clarification:
       - phase-2 contract explicitly includes `MetricWindow` and
         `WithWindow(WindowCumulative|WindowCycle)` instrument option.

65. Cycle-control leakage through exported concrete store type: **Resolved**
   - Context:
     - runtime ownership boundary requires collectors not to control cycles (`45B`) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - collector-owned store model is selected (`19B`) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - earlier contract exposed concrete constructor/type and made cycle-capable assertions too easy.
   - A) Keep exported concrete type and rely on convention.
       - Pros: simplest implementation.
       - Cons: collector code can call cycle controls directly; weak boundary.
   - B) Hide cycle control from public concrete type (recommended):
       - `NewCollectorStore()` returns `CollectorStore` (or public non-cycle interface),
       - runtime gets cycle-control capability via internal/runtime-only wrapper.
       - Pros: enforces boundary by construction.
       - Cons: slightly more internal plumbing.
   - C) Keep concrete type but require lint/tests to block collector calls to `CycleController()`.
       - Pros: no API churn.
       - Cons: policy is external, not type-enforced.
   - Outcome: B.
   - Clarification:
       - public constructors return interfaces (`CollectorStore`, `RuntimeStore`) and do not expose
         cycle-control-capable concrete type to collectors.

66. `CollectMeta`/`SeriesMeta` semantics for cycleless runtime store: **Resolved**
   - Context:
     - reader contract requires `CollectMeta()` and `SeriesMeta(...)` (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - these are cycle/attempt-based by definition (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - runtime store is cycleless immediate-commit (`58B`) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Runtime store returns zero/unknown metadata always.
       - Pros: minimal implementation.
       - Cons: weak observability; mixed behavior under same interface.
   - B) Runtime store uses its own commit sequence metadata (recommended).
       - Pros: preserves metadata usefulness with consistent semantics.
       - Cons: adds one runtime-store-specific metadata policy.
   - C) Split runtime store read interface to drop collect metadata methods.
       - Pros: semantically clean.
       - Cons: larger API split and adapter complexity.
   - Outcome: B.
   - Clarification:
       - runtime store maintains its own commit sequence and maps it into
         `CollectMeta`/`SeriesMeta` fields.

67. Runtime store ownership seam (collector-owned vs runtime-owned): **Resolved**
   - Context:
     - dedicated runtime store seam is required (`57B`) (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`),
     - the text currently gives alternatives (`ModuleV2.RuntimeStore()` or runtime registry) without a final selection (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Collector-owned runtime store via explicit accessor (`ModuleV2.RuntimeStore() metrics.RuntimeStore`) (recommended).
       - Pros: consistent with collector-owned collection store (`19B`); easy function access.
       - Cons: two stores owned per collector.
   - B) Runtime-owned registry per job for runtime metrics.
       - Pros: centralized control and lifecycle.
       - Cons: additional registry plumbing and indirection.
   - C) Hybrid: collector accessor optional, runtime fallback no-op store.
       - Pros: migration flexibility.
       - Cons: can hide misconfiguration in production.
   - Outcome: A.
   - Clarification:
       - runtime store is collector-owned and exposed via explicit V2 accessor
         (`RuntimeMetricsStoreProvider`).

68. Counter reset delta behavior (`current < previous`): **Resolved**
   - Context:
     - lifecycle currently references reset-aware `3A` but does not define exact `Delta` return behavior (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Return `(current, true)` on reset (recommended).
       - Pros: matches common counter-reset semantics used in Prometheus ecosystems.
       - Cons: if source wraps/noises in non-reset ways, may over-report.
   - B) Return `(0, false)` on reset.
       - Pros: conservative; avoids potentially fabricated value.
       - Cons: drops a data point on every reset.
   - Outcome: A.
   - Clarification:
       - on reset (`current < previous`), `Delta()` returns `(current, true)`.

69. `StatefulGauge.Add` first-write baseline in cycle store: **Resolved**
   - Context:
     - `StatefulGauge.Add` exists but baseline rule is only specified for `StatefulCounter.Add`
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Baseline from prior committed value (or zero if absent), then accumulate (recommended).
       - Pros: deterministic additive semantics aligned with stateful counter model.
       - Cons: slightly more explicit lifecycle wording.
   - B) Baseline from zero each cycle.
       - Pros: simpler local-cycle accumulator behavior.
       - Cons: inconsistent with carry-forward stateful gauge visibility model.
   - Outcome: A.
   - Clarification:
       - `StatefulGauge.Add` first write in cycle baselines from committed value
         (or zero if absent) and accumulates thereafter.

70. Runtime-store freshness override policy: **Resolved**
   - Context:
     - runtime store is cycleless stateful path, but reader freshness predicates are defined in collection-cycle terms
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Runtime store forces `FreshnessCommitted` only (recommended).
       - Pros: removes semantic ambiguity for cycleless runtime path.
       - Cons: no runtime-side fresh-only mode.
   - B) Allow `FreshnessCycle` on runtime store with dedicated runtime-specific semantics.
       - Pros: more flexibility.
       - Cons: adds another freshness model to specify and test.
   - Outcome: A.
   - Clarification:
       - runtime store uses `FreshnessCommitted` only; runtime-side
         `FreshnessCycle` configuration is invalid (fail-fast).

71. `WithWindow(...)` applicability scope: **Resolved**
   - Context:
     - `WithWindow(...)` is generic `InstrumentOption`, but semantic coupling is defined for stateful summary/hist only
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Valid only for stateful summary/histogram; fail-fast for gauge/counter (recommended).
       - Pros: clear contract and consistent lifecycle model.
       - Cons: stricter validation.
   - B) Accept on gauge/counter as no-op.
       - Pros: tolerant API.
       - Cons: hides misconfiguration and weakens contract clarity.
   - Outcome: A.
   - Clarification:
       - `WithWindow(...)` is accepted only for stateful summary/histogram and
         rejected for gauge/counter by fail-fast policy.

72. `Delta()` on non-counter series: **Resolved**
   - Context:
     - reader exposes generic `Delta(name, labels)` but non-counter behavior is not explicit
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Return `(0, false)` for non-counter series (recommended).
       - Pros: safe and explicit; no panics in callers.
       - Cons: requires callers to check `ok`.
   - B) Fail-fast on non-counter `Delta()` calls.
       - Pros: catches misuse aggressively.
       - Cons: disruptive for generic call paths in functions/engine.
   - Outcome: A.
   - Clarification:
       - non-counter `Delta()` returns `(0, false)`.

73. `SnapshotCounter.ObserveTotal` repeated writes in one cycle: **Resolved**
   - Context:
     - update rule for multiple `ObserveTotal` writes to the same series in a single cycle is not explicit
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Last-write-wins in staged frame (recommended).
       - Pros: deterministic and natural for sampled total observations.
       - Cons: earlier writes in the same cycle are overwritten.
   - B) Fail-fast on second write in same cycle.
       - Pros: strictness and error visibility.
       - Cons: brittle for adapters/composed pipelines.
   - Outcome: A.
   - Clarification:
       - repeated `ObserveTotal` for the same series in one cycle is
         last-write-wins in staged frame.

74. Public API exposure of `CounterState` contract type: **Resolved**
   - Context:
     - `CounterState` is currently listed in core contract types, but no `Reader` method returns it
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Add public reader accessor:
       - `CounterState(name string, labels Labels) (CounterState, bool)`
       - Pros: type is fully usable in public contract.
       - Cons: broadens public API surface for phase-2.
   - B) Keep `CounterState` internal/non-contract (recommended).
       - Pros: preserves lean public reader API (`Value`/`Delta`/`Meta`) and avoids premature API lock-in.
       - Cons: raw counter internals unavailable to API consumers.
   - Outcome: B.
   - Clarification:
       - `CounterState` is an internal storage representation and not part of
         phase-2 public Reader contract.

75. Runtime acquisition seam for cycle control from collector-owned collection store: **Resolved**
   - Context:
     - collection store is collector-owned, while cycle control is runtime-owned and intentionally hidden from collector-facing constructor result
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Public runtime constructor `NewManagedStore() CycleManagedStore`.
       - Pros: explicit and simple runtime wiring.
       - Cons: exposes cycle-capable constructor publicly (weaker boundary).
   - B) Package-internal extraction helper (recommended):
       - runtime obtains cycle controller via internal helper/view; not public API.
       - Pros: keeps collector-facing public API minimal and preserves boundary guarantees.
       - Cons: requires internal package plumbing.
   - C) Public `AsCycleManagedStore(Store) (CycleManagedStore, bool)` helper.
       - Pros: explicit conversion path.
       - Cons: public capability-discovery seam can be misused by collector code.
   - Outcome: B.
   - Clarification:
       - runtime acquires cycle-control capability via package-internal helper/view;
         no public API is added for this capability escalation.

76. Reader iteration surface overlap (`Family` vs `ForEachByName`): **Resolved**
   - Context:
     - both APIs iterate by metric name and overlap semantically
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Keep both and document intent split (recommended):
       - `Family` for reusable repeated iteration,
       - `ForEachByName` as one-shot convenience.
       - Pros: no API churn; preserves ergonomics.
       - Cons: slightly larger reader API.
   - B) Remove `ForEachByName`, keep `Family` only.
       - Pros: smaller API surface.
       - Cons: more verbose callsites for simple iteration.
   - Outcome: A.
   - Clarification:
       - both methods remain; interface docs define intent split (`Family` reusable,
         `ForEachByName` one-shot).

77. `Reader.Value()` semantics for counter series: **Resolved**
   - Context:
     - `Value(name, labels)` exists but counter-specific return semantics are not explicitly stated
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) `Value()` returns latest committed raw value for all series types (recommended):
       - counters => committed `current` total,
       - gauges => committed gauge value.
       - Pros: straightforward for chart engine and functions.
       - Cons: requires one explicit sentence in query contract.
   - B) `Value()` undefined for counters (force callers to use dedicated counter API).
       - Pros: stricter API intent.
       - Cons: awkward and surprising for generic readers.
   - Outcome: A.
   - Clarification:
       - `Value()` returns latest committed raw value for all series types;
         counters return committed `current`.

78. Cross-meter same-identity mode conflict policy: **Resolved**
   - Context:
     - metric identity is `(name, canonical labels)`, not meter instance,
       and mode-mixing is fail-fast, but cross-meter identity conflict is not explicitly named
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Fail-fast on cross-meter mode conflict (recommended):
       - same identity cannot be declared/written as both snapshot and stateful.
       - Pros: prevents silent semantic corruption.
       - Cons: adds one explicit validation path.
   - B) Allow last-writer/mode wins.
       - Pros: permissive.
       - Cons: unsafe and non-deterministic semantics.
   - Outcome: A.
   - Clarification:
       - same metric identity (`name+labels`) cannot be declared/written in both
         snapshot and stateful modes; this is fail-fast.

79. Runtime commit sequence granularity contract: **Resolved**
   - Context:
     - runtime store is immediate-commit, but sequence advancement granularity can be read as per-write or per-batch
       (`src/go/plugin/go.d/TODO-codex-godv2/phase2-metrics.md`).
   - A) Observable semantics are per-write commit (recommended):
       - each write operation is committed immediately from API perspective;
         internal batching is allowed only if externally equivalent.
       - Pros: unambiguous metadata semantics.
       - Cons: stronger wording around observable behavior.
   - B) Leave granularity implementation-defined.
       - Pros: maximal implementation flexibility.
       - Cons: inconsistent metadata interpretation across implementations.
   - Outcome: A.
   - Clarification:
       - runtime sequence semantics are observable per write operation;
         internal batching is allowed only if behavior is externally equivalent.

80. StateSet/Histogram schema drift policy: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - user clarification: new labels create new metric series identities and are expected,
       but state-domain (`state`) and bucket-domain (`le`) drift should not be accepted.
   - A) Enforce schema stability per metric family after first successful capture (recommended):
       - stateset state values and histogram bucket boundaries are immutable schema,
       - runtime drift is rejected by dropping offending family component with warning/error counters,
       - collect cycle continues.
       - Pros: deterministic chart/template behavior; predictable flatten/route mappings.
       - Cons: exporters that legitimately change schema at runtime need restart/reload handling.
   - B) Allow runtime schema drift and adapt dynamically.
       - Pros: more permissive with changing exporters.
       - Cons: unstable chart semantics and cache churn.
   - C) Treat schema drift as hard collect failure.
       - Pros: strongest enforcement.
       - Cons: too disruptive for transient exporter issues.
   - Outcome: A.
   - Clarification:
       - this rule does not forbid new label-set series identities;
         it forbids state-domain/bucket-domain shape drift for a metric family.

81. Non-scalar storage model follow-up (re-open after user direction): **Resolved (User picked B on 2026-02-12)**
   - Context:
     - previous decision `5A` selected scalar normalization in phase-2,
     - user now prefers same metric model for read/write and flattening on-demand for engine/functions.
   - A) Keep `5A` as-is (normalize Histogram/Summary/StateSet in adapter; reader is scalar series only).
       - Pros: smallest change to current phase-2 contract.
       - Cons: diverges from unified read/write model direction.
   - B) First-class non-scalar families in canonical store + snapshot-scoped flatten view/cache for consumers (recommended).
       - Pros: read/write model parity; flatten is a view concern; supports cached structure with per-commit value refresh.
       - Cons: larger phase-2 implementation surface; requires API changes (`Read().Flatten()` or equivalent).
   - C) Hybrid:
       - keep scalar canonical for Summary/Histogram now, add first-class StateSet only.
       - Pros: partial alignment with lower risk.
       - Cons: inconsistent model across non-scalar kinds.
   - Outcome: B.
   - Clarification:
       - canonical store keeps first-class Histogram/Summary/StateSet families,
       - flattening to scalar series is a snapshot-bound read view concern.

82. Summary quantile scope in phase-2: **Resolved (User picked B on 2026-02-12)**
   - Context:
     - OpenMetrics Summary can include quantiles, but quantile maintenance is costly and not always needed.
   - A) Count+sum only in phase-2 (quantiles deferred).
       - Pros: simpler and faster baseline.
       - Cons: less feature-complete summary support.
   - B) Count+sum+quantiles in phase-2 with explicit objectives config.
       - Pros: feature-complete summary support.
       - Cons: complexity and memory/CPU overhead.
   - C) Dual mode:
       - default count+sum, optional quantiles when objectives are configured.
       - Pros: balanced flexibility and performance.
       - Cons: more branching in implementation/tests.
   - Outcome: B.
   - Clarification:
       - phase-2 supports summary quantiles when explicitly configured on instrument declaration.

83. Runtime/internal store write path complexity: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - runtime store is multi-writer and immediate-commit (`125B`, `126B`),
     - `client_golang` uses sophisticated lock-free hot/cold paths for histogram/summary.
   - A) Sharded-lock write path with simple per-series structures (recommended baseline).
       - Pros: easier correctness and maintainability.
       - Cons: lower peak throughput than lock-free designs.
   - B) Adopt lock-free hot/cold algorithms for histogram/summary from start.
       - Pros: highest write throughput under contention.
       - Cons: high complexity and higher bug risk.
   - C) Mixed:
       - simple path for gauge/counter, hot/cold only for histogram/summary.
       - Pros: targeted optimization.
       - Cons: mixed complexity model.
   - Outcome: A.
   - Clarification:
       - baseline implementation is sharded-lock write path with atomic snapshot publish,
         with lock-free alternatives deferred unless benchmarks justify complexity.

84. Metric kind taxonomy vs vec-style families: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - phase-2 had possible confusion between metric kinds and convenience bundle APIs.
     - user explicitly asked about `CounterVec`/`GaugeVec`.
   - A) Treat vecs as non-kind instrument families (recommended):
       - metric kinds remain `Gauge`, `Counter`, `Histogram`, `Summary`, `StateSet`,
       - vecs are ergonomic facades over labeled series for one kind.
       - Pros: clean taxonomy, no storage/read model complexity.
       - Cons: introduces one extra conceptual layer (kind vs family API).
   - B) Treat vecs as additional metric kinds.
       - Pros: simpler wording for users familiar with prometheus naming.
       - Cons: incorrect data-model semantics; confuses kind with declaration shape.
   - C) No vec concept in phase-2 at all.
       - Pros: smallest surface.
       - Cons: loses a familiar ergonomic pattern and obscures labeled-family usage.
   - Outcome: A.

132. StateSet/Histogram writer API style: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - phase-2 requires strict schema handling for non-scalar families.
   - A) Dedicated typed instruments (`Histogram(...)`, `StateSet(...)`) with strict schema validation.
       - Pros: explicit contracts, better compile-time API clarity, cleaner validation path.
       - Cons: slightly larger writer API surface.
   - B) Generic family-write API.
       - Pros: one generic path for all family kinds.
       - Cons: weaker contracts and more runtime ambiguity.
   - Outcome: A.

133. Snapshot vs stateful API split for non-scalar instruments: **Resolved (User picked B on 2026-02-12)**
   - Context:
     - previous draft exposed the same non-scalar write methods on both snapshot and stateful meters.
     - for snapshot/scrape flows, collectors usually ingest pre-aggregated points (bucket/count/sum, count/sum/quantiles), not raw observations.
   - A) Keep one API (`Observe(v)`) for both modes.
       - Pros: smallest API surface.
       - Cons: semantically incorrect for snapshot non-scalar ingestion; forces adapter indirection and ambiguity.
   - B) Split by mode:
       - snapshot non-scalars accept point/snapshot writes (`ObservePoint(...)`),
       - stateful non-scalars keep sample writes (`Observe(v)`).
       - Pros: explicit semantics, clean ingestion paths, fewer hidden conversions.
       - Cons: larger API surface.
   - C) Keep unified API but add explicit mode option per call.
       - Pros: one type surface.
       - Cons: noisy call sites and easy misuse.
   - Outcome: B.
   - Clarification:
       - snapshot non-scalar writes are point-based (`ObservePoint(...)`),
       - stateful non-scalar writes are sample-based (`Observe(...)`).

134. Histogram schema declaration requirement: **Resolved (User picked C on 2026-02-12)**
   - Context:
     - histogram bounds capture is feasible for snapshot `ObservePoint(...)` (point carries full schema),
       but stateful `Observe(v)` does not carry boundary schema.
   - A) Require `WithHistogramBounds(...)` for stateful histogram declaration.
       - Pros: deterministic and explicit; avoids impossible auto-capture path.
       - Cons: a bit more declaration boilerplate.
   - B) Keep optional bounds and infer from runtime observations.
       - Pros: less config.
       - Cons: mathematically/semantically impossible from scalar observations alone.
   - C) Optional bounds only for snapshot point-writes that include full schema; required for stateful `Observe(v)`.
       - Pros: flexible and correct by mode.
       - Cons: more rules to document.
   - Outcome: C.
   - Clarification:
       - stateful histogram requires explicit bounds declaration (`WithHistogramBounds(...)`),
       - snapshot histogram may omit bounds and capture schema from first successful `ObservePoint(...)`.

135. StateSet write atomicity model: **Resolved (User picked B on 2026-02-12)**
   - Context:
     - current `Set(state, enabled, ...)` is per-state write.
     - this can leave stale states when a cycle writes only a subset.
   - A) Keep per-state writes only.
       - Pros: simple API.
       - Cons: partial updates can produce stale states unless callers are perfect.
   - B) Require full-state replace call per sample (`SetAll(...)` / `ObserveStateSet(...)`).
       - Pros: deterministic state snapshots and simpler freshness semantics.
       - Cons: slightly heavier call shape.
   - C) Support both:
       - per-state incremental writes and explicit replace mode.
       - Pros: flexible.
       - Cons: more complex semantics/testing.
   - Outcome: B.
   - Clarification:
       - state set updates are full-state replace per sample (`ObserveStateSet(...)`),
       - partial per-state updates are not part of phase-2 contract.

136. Flatten view contract shape: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - `Reader.Flatten() Reader` exists, but behavior for repeated flatten calls and name/type visibility is not fully fixed.
   - A) `Flatten()` returns scalar-only reader view; `Flatten().Flatten()` is idempotent no-op.
       - Pros: simple and predictable for engine/functions.
       - Cons: hides typed getters on flattened view by convention only.
   - B) Return dedicated `ScalarReader` interface from `Flatten()`.
       - Pros: strongest type clarity.
       - Cons: adds one more interface.
   - C) Keep current signature and leave behavior implementation-defined.
       - Pros: flexible.
       - Cons: high ambiguity and migration risk.
   - Outcome: A.
   - Clarification:
       - `Flatten()` returns a scalar-series reader view,
       - `Flatten().Flatten()` is idempotent and yields an equivalent scalar view for the same snapshot.

137. Flatten naming contract for typed families: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - canonical typed families need deterministic synthetic series names in flatten view.
   - A) OpenMetrics-compatible names:
       - histogram: `_bucket{le}`, `_count`, `_sum`;
       - summary: base metric with `quantile` label, plus `_count`, `_sum`;
       - stateset: base metric with a synthetic label whose key equals metric family name.
       - Pros: familiar and interoperable.
       - Cons: imports OpenMetrics naming conventions into internal layer.
   - B) Internal custom naming scheme.
       - Pros: full internal control.
       - Cons: extra translation everywhere and higher migration risk.
   - Outcome: A.
   - Clarification:
       - reserved-label/key constraints for flattened synthetic series follow OpenMetrics compatibility:
         - histogram base labels MUST NOT contain `le`,
         - summary base labels MUST NOT contain `quantile`,
         - stateset base labels MUST NOT contain the metric-family-name label key,
       - violating reserved-label constraints is fail-fast for the offending write/declaration scope.

138. Read/ReadRaw interaction with Flatten: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - both `Read()` and `ReadRaw()` return `Reader`, and reader has `Flatten()`.
   - A) Flatten preserves source visibility mode:
       - `Read().Flatten()` => fresh-policy filtered scalar view,
       - `ReadRaw().Flatten()` => full committed scalar view.
       - Pros: compositional and predictable.
       - Cons: requires strict implementation discipline.
   - B) Flatten always behaves like `ReadRaw`.
       - Pros: simpler implementation.
       - Cons: breaks fresh-only expectations for normal readers.
   - C) Flatten always behaves like `Read`.
       - Pros: safer default.
       - Cons: diagnostics path loses stale visibility.
   - Outcome: A.
   - Clarification:
       - flatten must not change reader visibility semantics,
       - `Read().Flatten()` and `ReadRaw().Flatten()` differ only by underlying source visibility mode.

139. StateSet semantics profile (bitset vs enum helper): **Resolved (User picked B on 2026-02-12)**
   - Context:
     - OpenMetrics StateSet explicitly allows multiple true states in one point (`OpenMetrics.md:218`),
     - when used to encode ENUMs, exactly one true state is required (`OpenMetrics.md:222`),
     - user proposed an enum-style convenience (`SetEnabled("state")` => one-hot state assignment).
   - A) Global one-hot only semantics for all StateSet instruments.
       - Pros: simplest chart/mental model for status-style metrics.
       - Cons: incompatible with general bitset semantics and loses valid multi-true use-cases.
   - B) Keep canonical StateSet as general bitset + add enum convenience helper (recommended):
       - canonical write remains full-state replace (`ObserveStateSet(...)`),
       - optional helper (`SetEnabled(...)`/equivalent) builds one-hot points for enum-style instruments.
       - Pros: preserves standard semantics while making enum use ergonomic.
       - Cons: one extra helper/option to document.
   - C) Keep general bitset only; no enum helper.
       - Pros: minimal API surface.
       - Cons: less ergonomic for common status/phase metrics.
   - Outcome: B.
   - Clarification:
       - canonical write remains full-state replace (`ObserveStateSet(...)`),
       - declaration-time mode controls semantics (`ModeBitSet` vs `ModeEnum`),
       - enum convenience helper `Enable(...)` is supported.

140. Flatten placement (public API vs typed internals): **Resolved (User picked A on 2026-02-12)**
   - Context:
     - question: should flatten live only on non-scalar metric families instead of on `Reader`?
   - A) Keep `Flatten()` on `Reader` as public contract; implement via typed family flattening internally (recommended).
       - Pros: composable API (`Read().Flatten()` / `ReadRaw().Flatten()`), one-pass consumer model, keeps chart engine/functions simple.
       - Cons: requires contract discipline (flattened reader typed getters return `(zero,false)`).
   - B) Move flatten to non-scalar families only.
       - Pros: explicit typed internals.
       - Cons: pushes type-switch complexity to consumers; weakens read-path composability.
   - C) Support both equally in public API.
       - Pros: flexible.
       - Cons: bigger surface and ambiguity on preferred path.
   - Outcome: A.
   - Clarification:
       - `Reader.Flatten()` stays as the public entrypoint,
       - implementations may delegate to family-level flatteners/caches internally,
       - externally visible behavior is defined solely by `Reader` flatten contract.

141. Phase-2 implementation kickoff: **Resolved (User picked A on 2026-02-12)**
   - Context:
     - after locking core phase-2 decisions, implementation begins metric-kind by metric-kind.
   - A) start implementation now with `Gauge` as first metric kind (recommended).
       - Pros: smallest vertical slice for write/read/cycle semantics.
       - Cons: non-scalar paths come later.
   - B) delay implementation until phase-3 design is finalized.
       - Pros: reduces cross-phase churn risk.
       - Cons: blocks execution progress on phase-2 package.
   - Outcome: A.

142. Phase-2 package naming (collector ergonomics): **Resolved (User picked F on 2026-02-13)**
   - Context:
     - user concern: package name `metrics` collides with natural collector local variable names (e.g. `metrics := ...`).
   - A) Keep `agent/metrics` with package name `metrics`.
       - Pros: matches current phase-2 document and terminology.
       - Cons: higher collision/alias pressure in collectors using `metrics` locals.
   - B) Use `agent/metric` with package name `metric`.
       - Pros: keeps terminology clear while reducing collision with common local `metrics` variables.
       - Cons: singular naming is slightly less common for broad metric APIs.
   - C) Use `agent/meter` with package name `meter`.
       - Pros: aligns with meter-centric API surface (`SnapshotMeter`/`StatefulMeter`).
       - Cons: package also contains store/reader/query APIs, not just meters.
   - D) Keep path `agent/metrics` but recommend import alias (`mtr`) everywhere.
       - Pros: no package rename.
       - Cons: style burden on every caller; weaker ergonomics.
   - E) Use `agent/metrix` with package name `metrix`.
       - Pros: avoids natural `metrics` local-name collisions in collectors; keeps short call sites.
       - Cons: retains historical spelling.
   - F) Use `pkg/metrix` with package name `metrix`.
       - Pros: matches established package tier for reusable collector utilities (`go.d/pkg/*`), avoids introducing `agent/*` dependency for collector code.
       - Cons: keeps historical spelling.
   - Outcome: F.
   - Clarification:
       - phase-2 package path/name target is `src/go/plugin/go.d/pkg/metrix` / `package metrix`,
       - this depends on user-managed legacy-rename work in decision `143B`.

143. Rename legacy `pkg/metrix` to free `metrix` for phase-2: **Resolved (User picked B on 2026-02-13)**
   - Context:
     - legacy package `src/go/plugin/go.d/pkg/metrix` is currently imported broadly by collectors/runtime.
     - measured current usage in repo:
       - import sites: 43,
       - symbol references: 243.
   - A) Keep legacy `pkg/metrix` unchanged; use a different phase-2 package name/path (recommended).
       - Pros: avoids broad migration churn and break risk during phase-2 bring-up.
       - Cons: keeps historical `metrix` naming in legacy code.
   - B) Rename legacy package/path to `pkg/oldmetrix` and use `metrix` name for phase-2 package.
       - Pros: frees preferred short name for new API.
       - Cons: immediate large-touch migration across many collectors/tests with no functional gain.
   - C) Introduce compatibility shim package at old path after rename.
       - Pros: can reduce immediate breakage.
       - Cons: extra maintenance and temporary indirection.
   - Outcome: B.
   - Clarification:
       - user will perform legacy rename separately,
       - phase-2 implementation proceeds after that rename lands.

## Implementation Exit Criteria

- Package `agent/metrics` compiles with unit tests and benchmarks.
- Counter two-sample contract is tested and verified.
- Adapters are available for incremental migration paths.
- Store API is sufficient for:
  - functions needing deltas,
  - selector-driven chart-engine reads in next phase.
