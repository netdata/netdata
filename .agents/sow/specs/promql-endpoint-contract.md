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

The current function set covers the most common Grafana panels and
Prometheus alerting rules:

* Vector selectors with label matchers (`=`, `!=`, `=~`, `!~`) and
  `offset` modifier.
* Matrix selectors with range and `offset`.
* Counter family: `rate`, `irate`, `increase`, `delta`.
* Windowed aggregates (`*_over_time`): `avg_over_time`,
  `sum_over_time`, `min_over_time`, `max_over_time`,
  `count_over_time`, `last_over_time`, `present_over_time` (Phase
  3b, SOW-0020), plus `stddev_over_time`, `stdvar_over_time`,
  `quantile_over_time(phi, v)` (Phase 3e, SOW-0023). NaN samples
  are skipped; empty windows drop the series. `__name__` is
  preserved for `last_over_time` and stripped for the others
  (Prometheus convention). `stdvar`/`stddev` use the population
  formula via `sum_of_squares - mean^2` (round-off negatives
  clamp to zero). `quantile_over_time` clamps phi to ±Inf outside
  [0, 1].
* Predictive functions (Phase 3e, SOW-0023):
  * `predict_linear(range_vector, t)` -- ordinary least squares on
    the (timestamp, value) pairs of non-NaN samples, extrapolated
    `t` seconds past the last sample. Windows with fewer than two
    distinct timestamps drop the series.
  * `holt_winters(range_vector, sf, tf)` -- double-exponential
    smoothing with smoothing factor `sf` and trend factor `tf`,
    each required to be in `(0, 1]`. Out-of-range factors surface
    an evaluation error (HTTP 422) so the user notices the typo.
    Windows shorter than two non-NaN samples drop the series.
    Init: `s_0 = v_0`, `b_0 = v_1 - v_0` (matches Prometheus).
* Aggregations: `sum`, `avg`, `min`, `max`, `count`, each with optional
  `by` or `without` grouping. NaN values are skipped; empty buckets
  drop.
* Parametrized aggregators: `topk(k, expr)`, `bottomk(k, expr)`,
  `quantile(phi, expr)` (Phase 3c, SOW-0021), and
  `count_values(label, expr)` (SOW-0024), each with optional
  `by`/`without` grouping. Semantics:
  * `count_values("label", v)`: bucket input series by their
    per-series value (stringified). One output series per
    distinct value carrying `<label>=<value_string>` plus the
    grouping labels. NaN values bucket under `"NaN"`; +/-Inf
    under `"+Inf"`/`"-Inf"`. `__name__` stripped (aggregation
    convention).
  * `topk(k, v)` / `bottomk(k, v)`: per bucket, keep the k
    highest/lowest-valued series. Output preserves the original
    series labels minus `__name__` (Prometheus convention -- the
    selected series themselves are returned, not collapsed).
    Non-integer `k` truncates; `k <= 0` or NaN yields an empty
    result.
  * `quantile(phi, v)`: per bucket, compute the phi-quantile via
    linear interpolation between adjacent ranked observations
    (the standard rank-based formula `rank = phi * (n - 1)`).
    Output: one series per bucket carrying the grouping labels
    only, same shape as `sum`/`avg`. Phi outside `[0, 1]` clamps
    to `+Inf` (phi > 1) or `-Inf` (phi < 0) per Prometheus.
    NaN-only buckets drop.
* Arithmetic: `+`, `-`, `*`, `/`, `%`, `^` across scalar / vector
  combinations.
* Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=` with or without the
  `bool` modifier.
* Vector matching: `on(labels)` and `ignoring(labels)` for arithmetic,
  comparison, and set operators. Default matching (no clause) joins
  by every label except `__name__` -- matching Prometheus, and
  meaning that arithmetic between two differently-named metrics
  (`a + b`) joins on the rest of the label set rather than producing
  an empty result. Added in Phase 3d (SOW-0022).
* Cardinality modifiers: `group_left(includes)` for many-to-one and
  `group_right(includes)` for one-to-many. The "one" side is indexed
  by matching key; duplicates on the one side surface as an
  evaluation error. The `include` labels are copied from the one side
  onto each result series; result labels otherwise come from the
  "many" side (with `__name__` dropped for arithmetic). Set
  operators do not accept `group_left`/`group_right` (Prometheus
  itself rejects them at parse time).
* Set operators: `and`, `or`, `unless`. Each uses the matching key
  (default, `on`, or `ignoring`) to decide which lhs series pass
  through. `or` includes rhs series whose key is not present on
  the lhs. Cardinality is implicit many-to-many for set operators;
  no `group_*` modifier needed.
* Unary minus.
* The `@` modifier on vector and matrix selectors (SOW-0025).
  `<selector> @ <unix_ts>` pins the evaluation time to a fixed
  timestamp; `@ start()` / `@ end()` pin to the outer query
  range's start / end (for an instant query both collapse to the
  query time). The modifier shifts only the lookback / window
  right-edge; output samples remain stamped at the natural query
  step time. The `@` modifier is not yet supported on subqueries
  (subqueries are still out of scope).
* `histogram_quantile(phi, vector)` with `le`-labeled cumulative buckets.

The following are explicitly out of scope and reject at lowering or
evaluation time with a clear error:

* Subqueries (`metric[1h:5m]`).
* Many-to-many cardinality for arithmetic/comparison binops. Set
  operators are inherently many-to-many and don't accept the
  `group_*` modifier.
<!-- count_values shipped in SOW-0024. -->
<!-- `stddev_over_time`, `stdvar_over_time`, `quantile_over_time`
     shipped in Phase 3e (SOW-0023). -->
* `keep_metric_names` -- a query-level modifier that preserves
  `__name__` through transforms that would normally strip it.
  Blocked upstream: the `promql-parser` crate this project depends
  on does not tokenize the keyword, so any query containing it
  fails at parse time with `bad_data: parse error: invalid promql
  query`. Lighting up this feature requires either forking
  `promql-parser` or landing the change upstream first; deferred
  until that decision is made.
<!-- `@` modifier on selectors shipped in SOW-0025; `@` on subqueries
     waits for subqueries themselves. -->
* Subqueries (`<expr>[1h:5m]`).

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
`host_machine_guid` parameter. As of Phase 2 chunk 2 (SOW-0018), the
four cases are:

* `NULL` (no `host` URL parameter): localhost only.
* `"*"`: every known host on this agent (localhost plus every child).
* A `machine_guid`: dictionary lookup against `rrdhost_root_index`;
  matches exactly the host whose `machine_guid` equals the supplied
  value.
* Any other string: treated as a hostname; matched against
  `host->hostname` across `rrdhost_root_index`. `"localhost"` is
  aliased to the local host.
* On miss (a specific value that resolves to neither a known GUID nor
  a known hostname): the call returns an empty success envelope with
  zero series. The shim deliberately does **not** fall back to
  localhost, because a silent fallback would mask client-side typos
  and surface unrelated data.

GUID-first lookup is intentional: a child agent connected with both a
known hostname and a known machine_guid is reachable by either form,
and the GUID dictionary is O(1) so the cost is paid once.

The host scope applies uniformly to `/api/v1/query`,
`/api/v1/query_range`, `/api/v1/series`, `/api/v1/labels`,
`/api/v1/label/<name>/values`, and `/api/v1/metadata`. The synthetic
`instance` label on every emitted series carries the matched host's
hostname so a Grafana panel can filter further with
`{instance="server-7"}` if needed.

## Cardinality backstop

Every resolve call is bounded by `max_series` (default 10,000). If
resolution would yield more series, the request fails with
`bad_data`:`exceeded max_series; tighten the query or raise the cap`.

The cap is the only safety net against runaway label cardinality on
parent agents with hundreds of child hosts; it is not yet a per-endpoint
configurable.

## HTTP methods

Both endpoint families accept `GET` and `POST`. GET requests carry
parameters in the URL query string. POST requests carry parameters in
the body, `Content-Type: application/x-www-form-urlencoded`. The
handler URL-decodes the body and parses it the same way it parses the
URL query string; if both are populated, the URL query string takes
precedence and the body is ignored. POST is what Grafana uses when a
query is long enough to push the URL past common length thresholds, so
both methods need to work for the typical datasource configuration.

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

## Discovery endpoints

Phase 2 (SOW-0018) adds five endpoints that Grafana's Prometheus
datasource probes for UI features (metric browser, label autocomplete,
datasource health-check / Prometheus-vs-Mimir heuristics). All five live
under `/api/v1/` per Prometheus convention and route through a single C
dispatcher in `src/web/api/v3/api_v3_promql_discovery.c`.

**`start`/`end` semantics** (Phase 3a, SOW-0019): the `start` and `end`
query parameters are honored across `/api/v1/series`, `/api/v1/labels`,
`/api/v1/label/<name>/values`, and `/api/v1/metadata`. When both are
non-zero, contexts whose last-collected timestamp predates `start` are
skipped (Prometheus convention: "metrics with at least one series in
the queried window"). When either is zero, retention filtering is
disabled and every live (non-obsolete) context contributes -- the
Phase 2 behavior. Phase 2's "accepted but ignored" wording on these
parameters is superseded by this paragraph.

### `/api/v1/labels`

Returns the union of label names across all series resolved by the
supplied matchers, or across every series on the host when no `match[]`
is given.

Query parameters: `start`, `end`, `limit`, repeatable `match[]`,
`host`. `start`/`end` are accepted for spec conformance but currently
ignored; the underlying resolve takes its own default lookback. `limit`
is honored on the output list, after dedupe.

Response: `{"status":"success","data":["<label_name>",...]}`. The
result is sorted lexicographically. Accepts both `GET` and `POST`
(form-encoded body) per Grafana's `httpMethod=POST` default.

### `/api/v1/label/<name>/values`

Returns the distinct values of the label named in the URL path. The
metric-browser feeder uses `/api/v1/label/__name__/values` to populate
its dropdown; the response there is the full sanitized context name
list (e.g. `["system_cpu","disk_io",...]`).

Query parameters: same as `/labels`. `GET` only -- Grafana's
`GET_AND_POST_METADATA_ENDPOINTS` does not include this path.

Response: `{"status":"success","data":["<value>",...]}`, sorted.

**Fast path** (Phase 3a, SOW-0019): when the requested label is
`__name__` and no `match[]` selectors are supplied, the handler
short-circuits through `nd_pds_metric_names_collect`, which walks
`host->rrdctx.contexts` directly instead of resolving series. Same
correctness (the live-instance check inside the walk ensures
contexts with no non-obsolete charts are excluded, matching the
slow-path semantics), 20-50x faster end to end. Any `match[]` other
than the no-op case routes through the slow series-resolve path
because labels like `{instance="server-7"}` cannot be evaluated
without per-series data.

### `/api/v1/series`

Returns the full label set of each unique series matched by the
supplied selectors.

Query parameters: same as `/labels`. Multiple `match[]` values are
OR-unioned at the result-series level: a series matching any one of the
selectors appears once, deduped by signature. Grafana's
`MATCH_ALL_LABELS = '{__name__!=""}'` sentinel resolves to every named
series on the host scope.

Response:
`{"status":"success","data":[{"__name__":"...","instance":"...",...},...]}`.
Accepts both `GET` and `POST`.

### `/api/v1/metadata`

Returns per-metric TYPE / HELP / unit, optionally filtered to a single
metric.

Query parameters: `metric` (optional; sanitized name; selects one
metric), `limit` (caps the number of entries), `host`. `match[]` is
**not** accepted -- this endpoint is a flat catalog walk, not a series
resolve.

TYPE mapping: a context whose first non-obsolete dimension has
algorithm `RRD_ALGORITHM_INCREMENTAL` reports `counter`; every other
algorithm (absolute, percent-over-diff-total, percent-over-row-total)
reports `gauge`. HELP is the chart's title via `rrdset_title`. `unit`
is empty in Phase 2; the chart's units are already encoded in the
dimension axis label and Grafana rarely consumes the Prometheus `unit`
field.

Response:
`{"status":"success","data":{"<metric>":[{"type":"<counter|gauge>","help":"<title>","unit":""}],...}}`.

`GET` only.

### `/api/v1/status/buildinfo`

A static JSON response that Grafana's Go backend hits for the
Prometheus-vs-Mimir heuristic
(`grafana-prometheus-datasource/pkg/promlib/heuristics.go`).

Response:

```json
{
  "status": "success",
  "data": {
    "version": "2.45.0",
    "revision": "netdata",
    "branch": "netdata",
    "buildUser": "netdata",
    "buildDate": "",
    "goVersion": "",
    "features": {}
  }
}
```

The empty `features` map is **load-bearing**: Grafana classifies a
datasource as Prometheus when `len(features)==0` and as Mimir when the
map has entries. The version string `2.45.0` is diagnostic-only;
Grafana's frontend feature gating reads
`instanceSettings.jsonData.prometheusVersion` (a user-set datasource
field), not the buildinfo response.

`GET` only.

## Truncation and the `warnings` envelope field

Phase 1's `nd_pds_resolve` treated `max_series` overflow as a hard
error and returned a `bad_data` envelope. Phase 2 keeps that strict
behavior for the query path (`/query` / `/query_range`), where wrong
results would be silently wrong, but relaxes it for the discovery
path. Discovery responses gain an optional `warnings` field per
Prometheus convention:

```json
{
  "status": "success",
  "data": [...],
  "warnings": [
    "result truncated at max_series=10000; tighten match[] or raise the cap"
  ]
}
```

The `warnings` field is emitted only when truncation actually occurred;
its absence means the response is complete. Grafana surfaces
`warnings` at debug level only, so the UI continues to function with a
capped result set.

The shim accomplishes this with a `truncated` flag on `nd_pds_query`,
exposed through `nd_pds_was_truncated`. The query path treats `true`
as an error; the discovery path projects it into the envelope.

## HTTP method matrix

```
endpoint                              GET   POST
-----------------------------------------------
/api/v1/query                          x     x
/api/v1/query_range                    x     x
/api/v1/series                         x     x
/api/v1/labels                         x     x
/api/v1/label/<name>/values            x
/api/v1/metadata                       x
/api/v1/status/buildinfo               x
```

`POST` carries the parameters as `application/x-www-form-urlencoded`
in the request body. When both URL parameters and a body are present
on a POST request, the body is decoded first and the URL parameters
append after, so the URL effectively wins on duplicate keys (the
parameter parser uses last-write semantics for non-repeated keys).
`match[]` is the only repeatable parameter; the parser appends each
occurrence to the list.

## Open items deferred to later phases

Per the SOW-0017 close gate:

* Subqueries and full vector matching are Phase 3.
* The full `*_over_time` family is Phase 3.
* `topk`, `bottomk`, `quantile` are Phase 3.
* Tier selection beyond tier 0 is a Phase 3 refinement.
* The Prometheus compliance test suite is Phase 3.

Per the SOW-0018 close gate, specific-host resolution by `machine_guid`
or hostname is now implemented -- see the Host scoping section above.
macOS verification remains deferred to Phase 3 alongside production
hardening.

The contract documented above is what the evaluator implements today;
when any of the above land, the spec is updated in the same SOW that
ships the change.
