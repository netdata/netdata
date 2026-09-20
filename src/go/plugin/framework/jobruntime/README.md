# Collector job runtime

`Job` and `JobV2` manage collector lifecycle and protocol publication for V1 and V2 collectors.

## V2 Template Capture And Publication

Metric jobs implement exactly one of `collectorapi.StaticChartTemplateProvider` or
`collectorapi.ChartTemplateSetProvider`. After successful Check, `ChartTemplateSource` captures the initial template
and optional `CollectorV2EnginePolicy`. Static YAML is read once. A native getter is called once after each successful
Collect, before metric commit, and returns a stable immutable pointer until the desired set changes. See
[chartengine's named-set contract](/src/go/plugin/framework/chartengine/README.md#named-active-template-sets).

Invalid candidates (including nil or unprepared snapshots and effective global policy changes) abort the entire staged
metric cycle. Previous committed series and presentation remain intact apart from failed-attempt metadata. Collect
errors/panics and metric-commit failures cannot publish a candidate.

After metric commit, each live or previously materialized host scope prepares the same captured candidate against its
own last published state. Complete output admission commits that scope's program, route cache and lifecycle together.
A failed scope retains its previous state and may jump directly to a newer candidate on a later collection. Successful
peer scopes are not rolled back. Empty output can commit state. There is no intermediate-version queue, cross-host
transaction, replay, or Agent acknowledgment; short writes retain existing output-poison behavior.

Cleanup inventories belong to the host that last accepted nonempty chart output. Failed or empty host switches
retain that inventory; the next nonempty admission on a different host replaces it.

Host changes stage materialized reset in the plan attempt. Rejected output preserves the old host's state, and old-host
retirements are never sent to the new host. Function-only jobs do not require a chart provider.

## Per-job collection charts

Both runtimes own two self-monitoring charts. They MUST emit them on the local Agent host (`HOST ''`), outside the
configured job vnode and every collector-created host scope. They MUST NOT put these measurements into a collector's
metric store or multiply them per target scope.

| Chart ID | Context | Dimensions | Units |
|---|---|---|---|
| `netdata.<plugin>_<full-job>_data_collection_status` | `netdata.plugin_data_collection_status` | `success`, `failed` | status |
| `netdata.<plugin>_<full-job>_data_collection_duration` | `netdata.plugin_data_collection_duration` | `duration` | ms |

`<plugin>` uses the existing `cleanPluginName` transformation, for example `go.d` becomes `go_d`. Charts retain the
plugin/module identity, configured job labels, `_collect_job`, and the job's collection interval.

- V1 success means at least one collector chart was updated with a value, preserving its existing contract.
- V2 success follows its existing overall cycle result. An empty cycle without errors is successful.
  Collection, template-capture or metric-store commit errors fail the cycle. Scope emission retains partial-success
  semantics: one accepted scope can keep the cycle successful when another scope fails; failure of every attempted
  scope fails the cycle.
- Status reports complementary `success`/`failed` values on ordinary cycles. Duration is sampled only on success and
  measured before output I/O, so output backpressure is excluded. A panic does not publish self samples.
- Autodetection and function-only jobs do not publish these charts.

`jobSelfMetrics` shares definitions, rendering and committed publication state between versions. V1 includes self
output in its existing collector-output transaction. V2 admits one separate local-host self frame after its scope
transactions settle. A rejected self frame MUST NOT commit definition/update flags or undo accepted target data.

Cleanup uses the dedicated cleanup sink and obsoletes only successfully published self charts. Vnode staleness MUST
NOT suppress their cleanup. Rejected-job cleanup emits nothing; repeated cleanup does not repeat obsoletion.

Compatibility and regression coverage live in `job_self_metrics_test.go`, including a real Prometheus collector
using an HTTP test exporter. Existing V1 and V2 collection benchmarks cover the per-job cost of these charts.
