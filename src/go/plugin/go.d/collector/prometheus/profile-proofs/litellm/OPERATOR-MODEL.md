<!-- markdownlint-disable MD013 MD043 MD060 -->

# LiteLLM operator model

## Evidence boundary

- **Observed deployment:** LiteLLM 1.92.0, source `BerriAI/litellm @
  b3086ccd74553565c9a39716e72303ae985555f9`.
- **Source-derived fixture scope:** `BerriAI/litellm @ 23de7a15d9d40006ee596e617475ba101d60c5e9`.
- **Current-source comparison:** `BerriAI/litellm @ de706a35a6f1e9cb8c3cb527271df0b76a69f410`. The Prometheus
  registration, type, HELP, bucket, and service files used by the fixture are byte-identical across these revisions.
- **Observed exposition:** a local-only private authenticated capture. It proves the enabled local surface only and is not a
  committed proof input.
- **Synthetic exposition:** the committed fixture is a structural union of observed and source-only optional families. It does
  not represent one realizable configuration.

- **Runtime registry:** the running LiteLLM 1.92.0 environment uses `prometheus-client` 0.24.1; default collector
  registrations are traced to `prometheus/client_python @ f417f6ea8f058165a1934e368fed245e91aafc14`.

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

1. **Gateway:** end-to-end request workload, client-visible failures, queue pressure, total latency, and LiteLLM overhead.
2. **Policy and identity:** authentication, budgets, limits, and remaining quota at API key, user, team, organization, provider,
   and deployment scope.
3. **Routing and deployments:** deployment health, provider capacity, cooldowns, fallback outcomes, and per-deployment latency.
4. **Provider API:** provider-facing latency and time to first token.
5. **Usage and cost:** input/output/media token populations, generated media, and spend.
6. **Cache:** cache hit/miss outcomes and cache-token populations.
7. **Guardrails:** invocation outcomes, errors, and execution latency by guardrail/hook.
8. **MCP gateway:** tool-call workload and spend by server/tool.
9. **Managed batch and files:** creation/deletion work, batch polling/completion, duration, and last-seen file size.
10. **Callbacks and inventory:** callback delivery failures and billable/user/team inventory.
11. **Internal services:** service requests/failures/latency plus queue/lock saturation for Redis, Postgres, auth, router,
    pre-call processing, database writes, budget reset, and spend updates.

## Identity and label policy

- LiteLLM has a configurable `prometheus_metrics_config`; operators can enable metrics and retain or omit supported label axes.
- A profile does not impose privacy policy on data already exported by the monitored endpoint. API-key, user, team,
  organization, endpoint, route, and client labels remain eligible observability inputs when the operator question requires
  them.
- A context describes one semantic chart type and its instances count one monitored entity type. The complete instance key may
  include an ownership path needed for uniqueness without changing that leaf type. Bounded aspects such as status, stream
  mode, validated rate-limit classification, and service tier are dimensions; stable ownership and descriptive details remain
  filterable chart metadata.
- Every additive counter and histogram component first routes to a complete service/capability view. Omitted identity labels
  are deliberately raw-summed because these populations are additive; Netdata subsequently applies the incremental algorithm
  and reset handling.
- Selected additional views use explicit identities for deployment/provider/model, route, accounting entities,
  guardrail/hook, MCP server/tool, managed batch, and callback questions. Every breakdown selector requires its identity labels
  to be present and nonempty, so a filtered label contract removes only that optional entity view while the coarser view remains
  complete.
- LiteLLM can independently omit status labels on its proxy request/failure and deployment failure/cooldown counters. Their 10
  affected charts keep the same context and instance identity: nonempty status values become dynamic dimensions, while missing
  or empty values enter a fixed `unclassified` dimension. Fixed-constructor guardrail, batch-error, and file-deletion labels do
  not receive speculative fallbacks.
- Current upstream can add `service_tier` to end-to-end latency, provider API latency, provider time to first token, and spend.
  Separate count/sum/spend contexts compare tiers as dimensions while the existing service heatmaps remain complete across
  tiers. Non-whitespace values other than the source sentinel `None` become dynamic dimensions. The literal `None`, an
  omitted label caused by `include_labels` filtering, and defensive blank/whitespace forms enter `unclassified` through one
  mutually exclusive selector. A second dimension axis cannot be added to histogram buckets without conflating service tier
  with the `le` bucket axis.
- Model identity follows the source contract: provider/model views use `model` or `litellm_model_name`; proxy response/failure
  views use provider/deployment because those families export only the deployment `model_id`.
- Every non-timestamp gauge keeps all labels emitted for that series. This includes Python multiprocess `pid`: summing
  last-seen, state, configuration, limit, budget, queue, or inventory gauges across workers can fabricate values.
- Opt-in user-budget `user_email`/`user_alias` labels and deployment-configured custom metadata/`tag_*` labels remain in the
  complete gauge identity when emitted. Additive service charts intentionally aggregate configured custom labels unless a
  dedicated semantic breakdown owns them. Arbitrary configured label names are not dynamic dimensions because their names and
  values are deployment-controlled and unbounded rather than one source-defined bounded aspect.
- The complete gauge identity can legitimately be large. That cardinality reflects the endpoint's configured gauge
  populations and is not a reason to delete an operator-visible metric from the stock profile.

## Population and unit rules

- Counter totals use incremental algorithms and render the exact counted object per second.
- Gauges use absolute algorithms and retain last-seen/state/capacity semantics.
- Histogram buckets are cumulative observation counters and render as heatmaps.
- Histogram counts are routed with compatible workload counters when they answer the same owner/population question; otherwise
  they remain explicit observation-rate evidence.
- Histogram sums are cumulative measured-value totals. Only sums with the same raw unit and lifecycle owner may share a chart.
- Latency components are not added together unless the source defines an additive relationship.

## Layered dashboard contract

1. **Stable views:** existing contexts answer service-wide or capability-wide questions and survive every valid LiteLLM label
   filter. Optional bounded status labels change dimensions, not context or instance identity.
2. **Operational breakdowns:** provider/model, provider/deployment, deployment, route, guardrail/hook, MCP server/tool,
   managed-batch, and callback contexts answer component-specific questions without treating the full Cartesian label set as
   one entity.
3. **Accounting breakdowns:** token and spend contexts retain independent API-key, team, user, organization, end-user, and
   requested-model views. Each view sums all other additive axes intentionally.
4. **Point-in-time entities:** gauges remain per complete exported identity. No counter/histogram aggregation rule is reused for
   a gauge merely to reduce chart count.

## Binding exclusions

- Every source-known creation-epoch family ending in `_created` is denied by exact name because the profile schema cannot
  transform the raw value into age. Unknown future names are not covered by these exclusions and remain eligible for generic
  fallback. Lost question: when each declared series was created or restarted. Process/runtime monitoring remains the
  appropriate replacement.
- `litellm_requests_metric_total`: deprecated alias of the current proxy request family. Displaying both double-represents the
  same workload.
- `litellm_llm_api_failed_requests_metric_total`: deprecated failure alias. Displaying both double-represents failures.
- `litellm_check_batch_cost_last_run_timestamp`: raw epoch cannot be transformed into job age. The profile retains current jobs,
  successes, and errors but cannot honestly display staleness from this family.
- `process_start_time_seconds`: raw process-start epoch cannot be transformed into process age. Process CPU, memory, and file
  descriptors remain charted.
- `python_info`: the collector writer does not retain information-gauge metadata. Lost question: Python implementation/version
  labels; the application runtime work/resource families remain available.

## Diagnostic completeness

### Forward fallback boundary

- The source-complete fixture has zero generic fallback and zero unmatched series.
- The explicit deny list defensively suppresses generated epochs, deprecated aliases, and the raw batch-cost timestamp when
  job policy does not.
- Unknown future `litellm_*` families remain generically visible until source evidence can assign identity, unit,
  population, owner, and a curated destination.

- **Observed:** gateway, deployments, provider latency, usage/cost, cache, budgets, inventory, and selected background gauges.
- **Source-derived synthetic:** guardrails, MCP, managed batch/files, optional `prometheus_system` internal-service families,
  service-tier variants, opt-in user-budget identity, and configured custom metadata/tag label shapes.
- **Not exported:** derived success/error ratios, average latency, cache hit ratio, budget utilization, and timestamp age. Netdata
  profile templates do not compute arbitrary cross-family ratios or current-time deltas.
- **Observed or source-derived runtime:** single-process Python GC plus process CPU, resident/virtual memory, and file-descriptor
  state are charted from the exporter itself.
- **Delegated:** host/container-wide CPU, memory, network, and lifecycle remain with system/container integrations; they are not
  inferred from application counters.

## Source-contract caveat

- `litellm_overhead_latency_metric` says milliseconds in its HELP text, but the observed source records
  `litellm_overhead_time_ms / 1000`. The profile follows the update callsite and renders seconds.

## Reconciliation ledger

- `src/go/testdata/prometheus/profiles/litellm/SOURCE-INVENTORY.tsv` is the binding per-family and exact-selector semantic ledger.
- It accounts for **201 source families** and **273 authored selector routes** in 356 rows: **273 chart routes**, **82 job
  exclusions**, and **1 writer-ineligible information family**.
- The profile contains **159 authored charts**. The structural-union fixture materializes **231 chart instances** and **975
  dimensions**.
- Every row records operator owner, entity identity, role, observation population, cross-family relationship, unit algebra,
  label roles and optionality, availability gate, evidence limitation, disposition, destination, and pinned source path.
- Single-process runtime families are source-derived from `prometheus-client` 0.24.1; multiprocess LiteLLM deployments omit
  that default registry surface.
- Unresolved source families and authored selectors: **0**.
