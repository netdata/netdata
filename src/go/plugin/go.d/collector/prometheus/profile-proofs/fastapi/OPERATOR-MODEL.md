<!-- markdownlint-disable MD013 MD043 -->

# FastAPI Prometheus operator model

## Responsibility

This document owns the human semantic decisions for the shared HTTP surface. The external inventory owns exact family and
selector dispositions; `proof.yaml` and replay tests own executable behavior.

## Entity and signal model

- Route charts use `{handler, method}` identity; HTTP status is a bounded outcome dimension.
- The high-resolution duration histogram is service-wide because it exposes no route identity.
- Histogram buckets are rendered as non-overlapping observation-rate heatmaps.
- Counts retain request rates. Duration sums retain accumulated elapsed time per second and therefore represent concurrency,
  not average latency.

## Exclusion semantics

- Generated `_created` gauges are registration epochs, not operational state, and profile relabeling removes them.
- Request and response size summaries expose no quantiles. The collector writer rejects their incomplete logical summary
  shape instead of treating `_sum` and `_count` as independent scalar families.

## Composition boundary

The profile deliberately has no `app`. An automatically selected application profile supplies the resolved application
namespace, allowing the same HTTP views to compose while keeping application-specific chart contexts. A standalone job
may still set `app` when no application profile is present.
