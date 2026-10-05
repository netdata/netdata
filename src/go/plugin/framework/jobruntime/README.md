# Collector job runtime

`Job` and `JobV2` manage collector lifecycle and protocol publication for V1 and V2 collectors.

## Runtime Readiness And Termination

A V2 collector with a long-lived receiver or background loop MAY implement `CollectorV2Runner.Run(ctx context.Context,
ready func()) error`. `Init` and `Check` prepare and validate the candidate; DynCfg `test` never calls `Run`.
Exclusive runtime resources belong in `Run`, after the predecessor has physically finished its loop and cleanup. The
hook MUST call `ready()` only after all fallible startup prerequisites succeed and its state is safe for concurrent
`Collect`. Readiness does not require a first packet or successful remote observation. Function availability may still
depend on observation data.

`ManagedRun` settles startup exactly once. Accepted readiness enables collection and running availability; duplicate
or late callbacks cannot revive canceled or failed startup. Job Manager bounds logical startup waiting with a
separate timer using the process-attempt fuse's default two-minute duration. The process-owned worker starts this timer immediately
before launching the managed loop; it applies only before readiness, not to the successful runtime lifetime. Timeout cancels the attempt but
does not release physical ownership: `Run` must return before collector cleanup begins, and cleanup must finish before
a same-job successor can acquire the runtime identity.

A normal startup error uses the existing configured autodetection retry cadence and tries. Runtime acquisition failures
retain a Failed configuration, including stock jobs; `RunFailure` reports the collector's class and whether startup may
be retried. UPDATE that accepts a replacement for activation and non-running ENABLE acknowledge configuration with
202 before this runtime outcome; Job Manager publishes later health separately. RESTART observes the exact activation
outside the mutation lane.
Logical stop revokes output and Function admission promptly; physical cleanup still waits for their admitted work.
A startup error classified with
`collectorapi.PermanentError`, unexpected early nil return and recovered `Run` panic are non-retrying failures. Unexpected return after readiness, including nil, immediately cuts new ordinary output
and running availability, then reconciles the exact generation to Failed without automatic retry. If the failure races
with installation, settlement rechecks the live startup and terminal outcome before publishing Running. Stale events
cannot remove its successor. Already-admitted output may finish; terminal observation does not wait for a blocked `Collect` or
write lease before revoking future admission.

On requested stop, nil and cancellation-only returns are normal. Mixed or unrelated errors, recovered panics and
already settled failures remain failures. Collector error text is sanitized separately from its phase, code and retry
eligibility. Jobs with resolved secret references retain blanket lifecycle-error redaction, including endpoint
details. A recovered collector `Run` panic permits explicit restart after successful cleanup; independent lifecycle,
owner or cleanup failures retain the existing containment/quarantine behavior. V1 collectors and V2 collectors without
`Run` keep their existing collection lifecycle.

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

## First-sample storage

A V2 collector MAY set `StoreFirst: true` in its `collectorapi.Creator` registration to enable the Agent's existing
`store_first` chart option. The factory copies this setting into `JobV2Config.StoreFirst` at construction, fixing it
for the job lifetime. It defaults to `false`, preserving the Agent's default behavior. This is collector-wide
registration metadata, not a job configuration, metric or chart-template option.

The option applies to all collector-produced charts, authored or automatic, in every host scope. It accompanies every
`CHART` definition, including later dimensions, label updates, obsoletion and cleanup: the Agent clears the option
when a definition omits it. Framework collection-status/duration charts retain their own defaults. This option does
not preserve counter baselines across a collector restart or supply custom first-interval timing.

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
