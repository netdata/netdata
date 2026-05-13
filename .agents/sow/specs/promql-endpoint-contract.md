# PromQL Endpoint Contract

This spec documents the behavior of the Netdata PromQL evaluator as it
exists after SOW-0017 (Phase 1). The evaluator is mounted at four URLs:
the Netdata-namespaced paths `/api/v3/promql/query` and
`/api/v3/promql/query_range`, plus the Prometheus mirror paths
`/api/v1/query` and `/api/v1/query_range`. All four routes go through the
same handler in `src/web/api/v3/api_v3_promql.c` and produce identical
responses for identical input.

The evaluator is built and linked unconditionally in the default build.
Operators who need to opt out (hostile build environments, minimal
profiles) can do so with `cmake -DENABLE_PROMQL=OFF`; the Rust crate, the
C shim, the v3 handler, and the v1 dispatch entries are all removed from
the build in that case.

## Supported function set

Phase 1 covers the function set needed for the most common Grafana
panels and Prometheus alerting rules:

* Vector selectors with label matchers (`=`, `!=`, `=~`, `!~`) and
  `offset` modifier.
* Matrix selectors with range and `offset`.
* Counter family: `rate`, `irate`, `increase`, `delta`.
* Aggregations: `sum`, `avg`, `min`, `max`, `count`, each with optional
  `by` or `without` grouping.
* Arithmetic: `+`, `-`, `*`, `/`, `%`, `^` across scalar / vector
  combinations.
* Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=` with or without the
  `bool` modifier.
* Unary minus.
* `histogram_quantile(phi, vector)` with `le`-labeled cumulative buckets.

The following are explicitly out of scope for Phase 1 and reject at
lowering time with a clear error:

* Subqueries (`metric[1h:5m]`).
* Vector matching with `on`, `ignoring`, `group_left`, `group_right`.
* Set operators (`and`, `or`, `unless`).
* `topk`, `bottomk`, `quantile`, `count_values`.
* The full `*_over_time` family (`avg_over_time`, `sum_over_time`,
  `max_over_time`, `min_over_time`, `count_over_time`,
  `last_over_time`). These names are recognized by the parser and the
  lowering pass but the evaluator returns "not yet implemented".
* The `@` modifier.

## Naming: metric names and labels

Netdata identifies measurements by a four-level hierarchy: host,
context, instance, dimension. PromQL identifies measurements by a flat
`(metric_name, labels)` tuple. The mapping is:

* `__name__` carries the Netdata context with dots replaced by
  underscores. A Netdata context named `system.cpu` is queryable as
  `system_cpu`; a context named `disk.io` as `disk_io`. The shim
  iterates every context on a host, sanitizes each name with
  `prometheus_rrdlabels_sanitize_name`, and matches against the
  user's `__name__` value. Many-to-one sanitization is fine -- if two
  raw contexts collapse to the same sanitized name, both contribute
  to the result.
* `dimension` carries the Netdata dimension name verbatim. Every
  emitted series has a `dimension` label; the evaluator always emits
  one series per dimension regardless of chart homogeneity. **This is
  a documented divergence** from `/api/v1/allmetrics?format=prometheus`,
  which collapses homogeneous charts into one metric with the dimension
  as a label value. Users with dashboards built against the exporter
  may see different metric/label combinations through PromQL.
* `chart` carries the Netdata instance id (the raw form, not
  sanitized) for diagnostic purposes.
* `family` carries the chart family.
* `instance` carries the host's hostname. On a parent agent querying
  across hosts (`host_machine_guid="*"`), the `instance` label
  distinguishes per-host series.
* Per-instance labels (the chart's `RRDLABELS`) are emitted in full.
* Per-host labels are emitted with one filter: names starting with `_`
  are skipped. Netdata uses `_*` prefix for internal metadata (host
  system info, install fingerprints, cloud detection) that would
  otherwise drown the per-series label set.

## Counter semantics

Netdata's `RRD_ALGORITHM_INCREMENTAL` dimensions store per-bucket deltas,
not cumulative counter values. The evaluator works on those deltas
directly:

* `rate(metric[w])` is computed as `sum(stored_deltas_in_w) / w` where
  `w` is the actual span between the first and last sample's
  timestamps, in seconds. Arithmetically equivalent to Prometheus'
  formula in the no-reset case.
* `increase(metric[w])` is the sum of stored deltas in the window;
  same as `rate * w`.
* `irate(metric[w])` is the last stored delta divided by the gap
  between the last two timestamps.
* `delta(metric[w])` returns `last - first`. For INCREMENTAL
  dimensions, where stored values are already deltas, this returns the
  difference of two deltas -- not what the user wants. Use rate /
  increase on counters and `delta` on gauges.

Reset handling. Netdata's collectors detect counter resets at
collection time and flush the corrected delta into storage. The shim
surfaces a per-sample reset marker bit in the storage flags, but the
evaluator does not consume it: by the time the delta reaches us it is
already correct, and sample-to-sample decreases are not treated as
implicit resets. **Documented divergence from Prometheus:** if you
restart the exporter producing a counter, rate over the restart
boundary may differ between Netdata-as-storage and Prometheus-as-storage
by up to one bucket's worth.

## Staleness and lookback

For instant vector selectors, the evaluator applies Prometheus' lookback
rule: if no sample exists in the trailing five minutes ending at the
query time (adjusted by `offset`), the series is dropped from the
result. The default lookback delta is five minutes; Phase 1 does not
yet expose a per-query knob.

NaN values in storage are filtered out before lookback runs. A series
with samples that are all NaN is treated the same as a series with no
samples.

## Time alignment

For range queries, the evaluator runs at every step grid point in
`[start_ms, end_ms]` at the requested `step_ms` cadence and collates
per-step results into a matrix:

* Scalar top-level expressions collapse to a single empty-metric
  series with samples at every step.
* Instant-vector top-level expressions collate by signature: each
  unique label set contributes one series to the matrix, with one
  sample per step where the underlying evaluation found data.
* Range-vector top-level expressions are rejected with `bad_data`; a
  range query cannot have a matrix selector at the top level.

The maximum points per timeseries (Prometheus default 11,000) is
enforced in the handler before evaluation begins.

## Host scoping

The handler passes the host scope through to `nd_pds_resolve` via the
`host_machine_guid` parameter:

* `NULL` (no `host` URL parameter): localhost only.
* `"*"`: localhost plus every known child host on this parent agent.
* Any other value: Phase 1 chunk 1 treats anything except `*` as
  localhost. Specific-host lookup by machine_guid or hostname is a
  Phase 2 follow-up; the synthetic `instance` label is intended to
  carry hostname so users can filter via `{instance="server-7"}` once
  multi-host resolution lands.

## Cardinality backstop

Every resolve call is bounded by `max_series` (default 10,000). If
resolution would yield more series, the request fails with
`bad_data`:`exceeded max_series; tighten the query or raise the cap`.

The cap is the only safety net against runaway label cardinality on
parent agents with hundreds of child hosts; it is not yet a per-endpoint
configurable.

## Response shape

Success responses follow the Prometheus HTTP API contract:

```json
{"status":"success","data":{"resultType":"<type>","result":<payload>}}
```

`resultType` is `scalar` (instant query, scalar expression),
`vector` (instant query, instant-vector expression), or `matrix` (range
query, or instant query of a matrix selector).

Each `[timestamp, value]` element renders timestamps as
Unix-seconds with three decimal places (`1715594400.123`) and values
as JSON strings (`"42"`). `NaN`, `+Inf`, and `-Inf` render as their
Prometheus string forms.

Error responses use the documented error envelope:

```json
{"status":"error","errorType":"<class>","error":"<message>"}
```

with `errorType` in `bad_data` (HTTP 400; parse, type, and
cardinality errors), `execution` (HTTP 422; evaluation errors,
including "not yet implemented" for Phase 2 features), or
`not_found` (HTTP 404; unknown subpath under `/api/v3/promql/`).

Empty results render as a success response with an empty
`data.result` array. Range queries that find no series across any step
return `{"resultType":"matrix","result":[]}`.

## Build flag

The evaluator is enabled by default. `cmake -DENABLE_PROMQL=OFF`
removes:

* The `netdata_promql` Rust crate from the cargo workspace import.
* The `target_link_libraries(netdata netdata_promql)` linkage.
* The shim sources (`promql-data-source.{c,h}`) from libnetdata.
* The handler (`api_v3_promql.c`) from the daemon.
* The dispatch entries under `/api/v3/promql/` and `/api/v1/query{,_range}`.

When disabled, the daemon links without any PromQL footprint and the
endpoints return HTTP 404.

## Open items deferred to later phases

Per the SOW-0017 close gate:

* Subqueries and full vector matching are Phase 2.
* The full `*_over_time` family is Phase 2.
* `topk`, `bottomk`, `quantile` are Phase 2.
* Specific-host resolution by `machine_guid` or hostname is a
  Phase 2 refinement of the shim.
* Tier selection beyond tier 0 is a Phase 2 refinement.
* The Prometheus compliance test suite is Phase 3.

The contract documented above is what the evaluator implements today;
when any of the above land, the spec is updated in the same SOW that
ships the change.
