<!-- markdownlint-disable MD013 MD043 MD060 -->

# vLLM Prometheus operator model

## Responsibility

This document records the human rationale behind the dashboard. External `SOURCE-SEMANTICS.yaml` owns source facts,
`PROFILE-DESIGN.yaml` owns the complete authored design, and `proof.yaml` plus executable replay own expected behavior. Exact
families, labels, routes, units, counts, and exclusions intentionally live only in those machine-readable artifacts.

## Dashboard hierarchy

The dashboard follows the request path through scheduling, prefill, decode, execution, cache behavior, connectors, and the
serving boundary. This causal order is more useful during diagnosis than global drawers such as “Latency”, “Throughput”, or
“Errors”: an operator can begin at the affected phase and inspect its workload, state, outcomes, and distributions together.

Optional subsystems stay beneath the phase they extend. They do not create a second hierarchy based on metric type or on
whether a source happens to expose a histogram, counter, or gauge.

## Entity and cardinality rationale

Charts retain the finest source entity that has direct diagnostic value. Operators can aggregate those charts into broader
service views in Netdata; emitting both fine and coarse copies would duplicate observations and inflate chart cardinality.
Labels that distinguish a real entity or observation population therefore remain chart identity. Bounded states within one
entity are better represented as dimensions, while stable descriptive metadata remains available for filtering and grouping.

Ray transport metadata refines model-engine identity only when the source exposes it. Keeping replica and worker identity
preserves fault isolation without making their absence a different profile. Native and Ray inputs therefore share one
operator model even though their physical metric names and available metadata differ.

Service-wide HTTP, process, and language-runtime measurements remain service-wide. They are not attached to a model engine
merely to make all charts use the same identity.

## Population and presentation rationale

Each chart represents one observation population. Counts from requests, choices, connector operations, cache blocks, and
other source events are not combined merely because their units match. Totals and their successful or failed subsets are
kept separate when combining them would make the sum of dimensions meaningless.

Counters are presented as rates and gauges as current values unless the design explicitly overrides that interpretation.
Histogram buckets remain distributions; accumulated sums remain their source quantities rather than being mislabeled as
averages that the profile cannot compute. Queue, prefill, decode, inference, and request-duration totals are attributed when
the whole request completes; time-to-first-token observations are attributed when the first token is emitted. Their sums are
work-time rates in `seconds/s`; only current Gauges are presented as live concurrency.

## Exclusion rationale

Raw creation epochs are not useful operational values because the profile cannot transform them into ages. Source-proven
compatibility aliases and superseded families are excluded to prevent duplicate observations, while unknown future families
remain eligible for generic fallback. Information-only families are left uncharted when the numeric chart path cannot
represent their meaning honestly.
