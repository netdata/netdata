# Application evidence guide: LiteLLM proxy

Use these domain notes to orient research. Verify names, types, deprecation
status, labels, and feature availability against the supplied dump and the
matching LiteLLM release before authoring.

## Functional areas

LiteLLM proxy operates as an LLM API gateway. Useful operator stories commonly
include:

- request intake and in-flight work;
- routing/deployment outcomes and fallbacks;
- user-visible failures;
- provider, proxy-overhead, queue, first-token, and end-to-end latency;
- input/output/cached/reasoning token throughput;
- spend, budgets, and cost-control jobs;
- cache outcomes;
- internal background-job health;
- bounded tenant/team capacity where privacy and cardinality allow it.

These are potential family/navigation concepts, not a prescribed chart list.
Lead with the surfaces present in the evidence and important to the deployment.

## Labels and entity levels

- Deployment/model labels may support per-deployment instances. Check whether
  names are stable, whether several labels are needed for uniqueness, and
  whether service-wide metrics lack them.
- `hashed_api_key`, user, team, and end-user labels can be high-cardinality and
  privacy-sensitive. Never copy values into artifacts. Do not make them chart
  identity/dimensions without explicit product need, bounded evidence, and a
  privacy/cardinality review.
- Per-key budget gauges should not be mechanically summed into a global budget;
  the aggregate may have no valid business meaning. Choose an identity,
  supported aggregate, or intentional exclusion based on the operator question.

## Metric lifecycle cautions

Some releases expose both older and replacement request/failure families. Treat
a family as deprecated only when current `HELP` or authoritative versioned
source says so and the replacement is observed. Excluding an old family before
its replacement exists creates a blind spot.

Latency and overhead metrics may be histograms or summaries. Apply the actual
writer behavior:

- histogram buckets, count, and sum are distinct flattened surfaces;
- a summary without quantiles is rejected completely;
- count is workload rate;
- latency sum rate is not an average and implies concurrency only when the
  observation semantics justify it;
- there is no fixed four-chart requirement.

Queue time near in-flight work can support a saturation story, but shared
navigation does not require mixing different units on one chart.
