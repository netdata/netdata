<!-- markdownlint-disable MD013 MD043 MD060 -->

# LiteLLM operator model

## Responsibility

This document explains the human rationale behind `PROFILE-DESIGN.yaml`: operator domains, useful entity grain, reduction
policy, observation populations, and exclusion choices. It does not repeat exact source facts, routes, or replay outcomes.
External `SOURCE-SEMANTICS.yaml` owns source evidence; `PROFILE-DESIGN.yaml` owns the exact authored semantic contract; and
`proof.yaml` plus production replay own realizable cases and machine-verifiable behavior.

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

## Operator domains

1. Gateway: end-to-end workload, client-visible failures, queue pressure, latency, and LiteLLM overhead.
2. Policy and identity: authentication, budgets, limits, and remaining quota by accounting owner.
3. Routing and deployments: deployment health, provider capacity, cooldowns, fallbacks, and per-deployment latency.
4. Provider API: provider-facing latency and time to first token.
5. Usage and cost: tokens, media, generated outputs, and spend.
6. Cache: hit/miss outcomes and cache-token populations.
7. Guardrails: invocation outcomes, errors, and execution latency by guardrail/hook.
8. MCP gateway: tool-call work and spend by server/tool.
9. Managed batch/files: creation, deletion, completion, cost tracking, and duration.
10. Callbacks and inventory: delivery failures and user/team inventory.
11. Internal services: service outcomes, failure causes, and latency.

## Identity and aggregation policy

- One authored context answers one operator question at the finest useful entity grain. NIDL provides coarser grouping, so
  the profile does not duplicate global and detailed versions of the same view.
- Stable configured/accounting keys define identity. Request metadata, caller attributes, and custom labels do not become
  chart identity merely because they make raw series unique. Stable multiprocess worker `pid` is retained for per-worker
  gauges so operators can aggregate workers in Netdata without irreversibly collapsing their values at collection time.
- Deployment views use the configured deployment key, with provider-served tier retained only where it changes latency or
  spend meaning. API key, team, user, organization, end user, and requested model remain separate accounting questions.
- Client traffic separates route/status outcomes, exception/status causes, and optional rate-limit attribution instead of
  creating one high-cardinality Cartesian identity.
- Guardrail, MCP, callback, managed-object, and internal-service views use their configured operational entities. Internal
  service failure causes preserve function and exception identity while service is a bounded dimension.
- Open categories are NIDL identity/labels, never dynamic dimensions. Dynamic dimensions are reserved for bounded status,
  outcome, service, or histogram partitions.
- `label_promotion` is used only when a non-identity label is useful and source-stable at the chosen entity grain. It is not
  a substitute for identity and is deliberately empty where exact status, request metadata, or noisy aliases add no value.

## Reduction policy

- Additive counters and histogram components use explicit `sum` when caller, request, or custom-label children can collide
  at the operator grain. Per-worker gauge identity remains explicit; a reducer only resolves other labels projected from
  that same worker/entity grain.
- Configured ceilings use `max`; remaining headroom and reset windows use conservative `min`. They are separate charts
  because combining them would make dimension totals meaningless and could select values from different workers.
- Global user/team inventory gauges use `max` as the highest worker-reported database snapshot. This is useful after
  convergence but can remain stale-high after deletion.
- Source gauges whose update lifecycle cannot reconstruct current state are not repaired with an arbitrary reducer. Their
  lost question is recorded and fallback publication is suppressed exactly.
- Optional end-user counters retain a per-end-user view. Other accounting views accept the documented one-interval loss
  risk when LiteLLM's non-default TTL/LRU end-user tracking removes cumulative children.

## Population and unit rules

- Counters use incremental semantics; Netdata owns rate calculation and reset detection. Gauges use absolute semantics.
- Histogram buckets render observation-rate heatmaps. Counts retain the exact observed population, and sums retain
  accumulated source units per second; no sum is presented as an average.
- Workload totals are not combined with overlapping successful/failed populations. When both are useful, total workload,
  classified outcomes, and diagnostic causes use separate contexts.
- Lifecycle stages such as batch completion and later cost tracking likewise remain separate; adding their dimensions would
  produce a meaningless total.
- Exact HTTP/exception status is normalized to bounded classes with an `unclassified` fallback. The exact raw status remains
  internal rather than multiplying public identity or chart labels.
- Basic source units are retained. Multipliers/divisors are used only for a real unit conversion, not normalization by habit.

## Exclusion semantics

- Python-client `_created` companions are registration epochs, not operational state, and profile relabeling removes the
  exporter-scoped suffix class.
- Deprecated LiteLLM request counters are delegated to the supported proxy surfaces without claiming exact equivalence.
- The raw batch-poll epoch cannot express staleness without current-time arithmetic, so it is removed rather than charted as
  an absolute timestamp.
- Internal-service total counters and guardrail histogram counts are left unrendered only where source relationships prove a
  canonical rendered composition.
- Managed-file size, batch backlog, queue-size, and lock gauges remain writable but intentionally unrendered because their
  update/freshness lifecycle cannot support an exact lower-cardinality current view.
- Derived ratios, averages, utilization, and timestamp ages are not fabricated when the chart language cannot compute them.

## Forward compatibility

Profile relabeling is limited to source-proven normalization/exclusion grammars. Exact family-level fallback suppression is
limited to declared canonical duplicates and lifecycle-defective questions. Unknown future `litellm_*` families remain
eligible for generic fallback until evidence can assign ownership, identity, population, units, and a curated destination.
