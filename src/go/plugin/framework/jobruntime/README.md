# Collector job runtime

`Job` and `JobV2` manage collector lifecycle and protocol publication for V1 and V2 collectors.

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
- V2 success follows its existing overall cycle result. An empty cycle without errors is successful. Collection or
  metric-store commit errors fail the cycle. Scope emission retains partial-success semantics: one accepted scope can
  keep the cycle successful when another scope fails; failure of every attempted scope fails the cycle.
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
