<!-- markdownlint-disable MD013 MD043 MD060 -->

# SGLang operator model

Start with the endpoint's HTTP activity and completed inference work. Then use model and engine-role views to
identify the affected workload, worker views to distinguish queueing from execution pressure, and cache, storage
or encoder views to locate a dependency that is holding up progress.

## Independent summaries and detail

Service, model and model/engine-role summaries consume the complete source population. Detail uses SGLang's
built-in identities: model and engine role, worker ranks, priority, cache type, storage backend, HTTP route and
runtime component. Configured custom request labels are deliberately aggregated; they do not create instances.

The existing collection limits apply independently to completed contexts. A large priority or raw-route context
can be omitted while smaller summaries remain available. Every admitted view represents its complete input;
none is a first-N subset. Rejected detail is eligible again when its cardinality falls below the same limits.

These rollups are deliberate additional views: their independent availability matters when the detailed view is
not collected. Raw scrape and metric-store memory still scale with the exporter population.

## Request accounting and latency

Finished requests, abort calls, structured-output requests and HTTP responses describe different populations.
Finished does not mean successful. Structured-output requests are a subset of finished requests; abort calls
are not a disjoint outcome partition. Do not infer a success or error percentage from these counters.

Prompt and generated token counters are reported when requests finish. Scheduler realtime and effective-prefill
token counters describe worker execution instead. Cache-tier accounting can use a total fallback when tier
details are absent. Keep that fallback separate from tier comparisons.

Latency histograms retain their distributions and component counts/sums. Inter-token observations are weighted
by generated tokens, not requests. Source summary sums and counts remain available without inventing quantiles
or a cross-series average. Sums of observed transfer speeds are labelled as observation sums, not achieved
bandwidth; their rate divided by the observation-count rate describes an unweighted mean observation.

## Scheduling and resources

SGLang can repeat scheduler observations across ranks. Model queue and resource-pressure views therefore show
the maximum worker value, or minimum free capacity. They do not claim unique request totals or sum replicated
logical capacity. Worker views retain the source rank identity and expose the underlying observations.

Priority-enabled queues publish both a total under empty priority and per-priority values. Total and priority
contexts select these populations separately. Gauge routing-key distributions contain current non-overlapping
request-count ranges; they are not cumulative Prometheus histograms and must not be differentiated as counters.

Keep physical memory, logical token capacity and current occupancy distinct. The pinned source's SLO-utilization
gauge has a documented unset-limit defect and a prefill sentinel; it is a reported worker diagnostic. Queue depth,
KV pressure and forward occupancy provide the overview's pressure signals. Its registered SLO-capacity gauge
has no emitting call path in the pinned implementation, so no populated chart or synthetic capacity is invented.

## Cache and multimodal ownership

Radix-cache metrics identify cache type without default model or rank labels. Storage metrics identify backend
and parallel ranks. Preserve those ownership boundaries. GPU-to-host backup, host-to-GPU load-back and
host-to-storage transfer are separate operations; physical shard transfer totals are not unique prompt tokens.

Encoder metrics identify model and data-parallel worker, with modality, outcome and transfer-backend comparisons.
Cache hit counters and total counters overlap; they are comparison lines, not stacked partitions.

## Transport, runtime and evidence boundary

HTTP service summaries use bounded method and status categories. Route detail retains raw endpoint, method and
status identity. An unmatched request can contribute an arbitrary raw path, so route detail is independently
limited. SGLang's multiprocess registry does not imply that generic Python process or GC collectors are exposed.
Use its own component CPU, function and startup instrumentation.

The source contract pins the native Python language runtime and independently records its registration, update,
unit and label evidence. Source-derived fixtures cover distinct serving roles and optional capabilities; they do
not assert that every metric exists simultaneously in one endpoint. The historical live census is not a complete
source proof. Current live replay remains unavailable while the inspected SGLang endpoint is down.

Cumulative reducers retain the existing sum-before-rate behavior. They do not repair individual contributor
resets or disappearing populations. Source-side worker/process lifecycle and the scope of the evidence are part
of the proof; no storage or rate-calculation redesign is included.
