<!-- markdownlint-disable MD013 MD043 -->

# FastAPI Prometheus operator model

## Responsibility

This document owns the human semantic decisions for the shared HTTP surface. The machine semantic documents own exact source
and route facts; `proof.yaml` and replay tests own executable behavior.

## Entity and signal model

- Route charts use `{handler, method}` identity; the producer-configured HTTP status class or exact code is an outcome
  dimension.
- The high-resolution duration histogram is service-wide because it exposes no route identity.
- The producer may change both duration histograms to exclude response-body streaming time and may restrict the
  high-resolution histogram to successful responses. The metric names do not reveal those settings, so chart wording is
  configuration-neutral.
- Histogram buckets are rendered as non-overlapping observation-rate heatmaps.
- Counts retain request rates. Duration sums are completion-attributed elapsed time, rendered as seconds of completed request
  work per second; they are neither live concurrency nor average latency.
- The optional in-progress Gauge is the live request-population signal. One chart is service-wide when the Gauge is
  unlabeled and refines to `{handler, method}` instances when the producer enables labels.
- Request and response size sums retain bytes observed per second, grouped by handler, as one request/response throughput
  comparison.
- Source-complete proof uses the default empty `custom_labels` mapping. Arbitrary user-configured constant label keys remain
  supported by collection but are outside this finite semantic label-key contract.
- The stock profile and proof use empty `metric_namespace`/`metric_subsystem` values and the default
  `http_requests_inprogress` Gauge name. Producer-configured family prefixes or a custom in-progress name remain collectable
  as Prometheus metrics, but they do not match this exact-family stock profile contract.

## Exclusion semantics

- Generated `_created` gauges are registration epochs, not operational state, and profile relabeling removes them.
- The pinned size summaries legitimately expose count and sum without quantiles. Their sums are charted as body throughput.
- Each size-summary count measures the same handled-request population already represented by request outcomes, but at a
  coarser handler-only grain. It is deliberately left unrendered instead of creating a duplicate operator view.
- Autogen filtering is family-aware, so the exact base summary families are denied only after authored routing has retained
  their sums. For the pinned count-and-sum shape this suppresses only the duplicate counts. A newly introduced unmatched
  component in either family will also remain unrendered until the evidence and profile are reviewed.

## Composition boundary

The profile deliberately has no `app`. An automatically selected application profile supplies the resolved application
namespace, allowing the same HTTP views to compose while keeping application-specific chart contexts. A standalone job
may still set `app` when no application profile is present.
