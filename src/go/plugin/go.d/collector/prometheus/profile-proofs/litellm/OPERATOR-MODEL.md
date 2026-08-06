<!-- markdownlint-disable MD013 MD043 MD060 -->

# LiteLLM operator model

## Responsibility

This document owns human semantic decisions: operator domains, entity identity, aggregation boundaries, observation
populations, units, and the reasons for exclusions. `EVIDENCE.md` owns provenance. The external source inventory owns exact
family/selector dispositions. `proof.yaml` and replay tests own all machine-verifiable behavior and counts.

## Capability and processing map

```text
client
  -> proxy admission and request queue
  -> authentication and budget/rate policy
  -> routing and deployment selection
  -> provider API
  -> response, token/accounting, and callbacks

cross-cutting:
  guardrails -> pre/post-call policy and latency
  cache -> local/provider cache effectiveness
  MCP gateway -> tool calls and spend
  managed batch/files -> asynchronous job lifecycle
  internal services -> Redis/Postgres/background queues and jobs
```

## Operator owners

1. Gateway: end-to-end workload, client-visible failures, queue pressure, latency, and LiteLLM overhead.
2. Policy and identity: authentication, budgets, limits, and remaining quota by accounting owner.
3. Routing and deployments: deployment health, provider capacity, cooldowns, fallbacks, and per-deployment latency.
4. Provider API: provider-facing latency and time to first token.
5. Usage and cost: tokens, media, generated outputs, and spend.
6. Cache: hit/miss outcomes and cache-token populations.
7. Guardrails: invocation outcomes, errors, and execution latency by guardrail/hook.
8. MCP gateway: tool-call work and spend by server/tool.
9. Managed batch/files: lifecycle work, polling/completion, duration, and last-seen file state.
10. Callbacks and inventory: delivery failures and billable/user/team inventory.
11. Internal services: service work/failures/latency plus queue and lock saturation.

## Identity and aggregation policy

- A context describes one semantic chart type; its instances represent one monitored entity type. Stable ownership labels are
  identity, while bounded outcomes such as status or stream mode are dimensions.
- Additive counters and histogram components first route to a complete service/capability view. Labels filtered by
  `prometheus_metrics_config` are raw-summed only for additive populations.
- Optional breakdowns require their identity labels to be present and nonblank. Losing an optional label removes only that
  breakdown, not the coarser complete view.
- Missing optional bounded status or service-tier values use an explicit `unclassified` dimension only where source proves
  the label optional. Fixed-constructor labels do not receive speculative fallback routes.
- Provider/model, deployment, route, accounting owner, guardrail/hook, MCP server/tool, managed batch, and callback are
  separate operator views rather than one Cartesian identity.
- Point-in-time gauges preserve their complete emitted identity, including multiprocess `pid` and deployment-defined labels.
  Summing last-seen, state, limits, budgets, queues, or inventory across workers can fabricate values.
- Arbitrary custom label names are not dynamic dimensions. Their domains are deployment-controlled and unbounded.

## Layered dashboard contract

1. Stable service/capability views survive valid label filtering.
2. Operational breakdowns isolate provider, deployment, route, guardrail, MCP, managed-job, and callback questions.
3. Accounting breakdowns isolate API key, team, user, organization, end-user, and requested-model views while intentionally
   summing other additive axes.
4. Point-in-time entities retain the endpoint's full gauge identity and do not inherit counter aggregation rules.

## Population and unit rules

- Counter totals use incremental algorithms and display the counted object per second.
- Gauges use absolute algorithms and retain last-seen, state, capacity, or configuration meaning.
- Histogram buckets are cumulative observation counters rendered as heatmaps.
- Histogram counts may share a workload chart only when they count the same owner and population.
- Histogram sums remain cumulative measured-value totals. Only sums with the same raw unit, population, and lifecycle owner
  may share a chart.
- Latency components are not added unless the source defines an additive relationship.
- LiteLLM overhead follows the update callsite's seconds conversion despite contradictory HELP wording.

## Exclusion semantics

- Source-known creation epochs and process-start time cannot be transformed into age. Runtime resource monitoring remains.
- Deprecated request/failure aliases are excluded because the canonical families represent the same observations.
- The raw batch-cost timestamp cannot honestly express staleness without a current-time transform; job workload/outcome state
  remains charted.
- Writer-ineligible Python information metadata retains its lost question in the source inventory.
- Derived ratios, averages, utilization, and timestamp ages are not fabricated when the profile schema cannot compute them.

## Forward compatibility

Exact exclusions cover source-proven epochs, duplicates, and ineligible routes only. Unknown future `litellm_*` families
remain eligible for generic fallback until evidence can assign ownership, identity, population, units, and a curated
destination.
