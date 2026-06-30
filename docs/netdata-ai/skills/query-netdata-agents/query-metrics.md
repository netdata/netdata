# Query agent metrics directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the full request body (scope / selectors / window /
aggregations / format / options), the response envelope (jsonwrap
with summary / view / result / db / timings), the time-aggregation
and dimension-aggregation rules, and worked examples, see
[../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md).
The Cloud `/api/v3/spaces/{sp}/rooms/{rm}/data` endpoint forwards
the same body to the agent's `/api/v3/data` endpoint.

---

## Endpoint (agent v3)

`POST /api/v3/data` on the agent. The request body is identical to
the Cloud `/data` body except `scope.nodes` (cloud) is implicit
on the agent (you're already targeting one node).

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

read -r -d '' BODY <<'JSON'
{
  "scope":     {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window":    {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["dimension"], "aggregation": "sum"}],
    "time":    {"time_group": "average"}
  },
  "format":  "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
JSON

agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST /api/v3/data "$BODY" \
  | jq '{view: .view.dimensions.names, points: (.result.data | length)}'
```

## Discover available contexts on the agent

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET '/api/v3/contexts'
```

`/api/v3/contexts` returns the metric contexts the agent currently
collects (e.g. `system.cpu`, `disk.space`, `nginx.connections`).
Use these as `scope.contexts` values.

## Time resolution: `duration ÷ points = seconds per point`

The number of `points` is NOT "give me per-second data". It is
"split the duration into N equal buckets". Actual time
resolution:

```
seconds_per_point = abs(after)  ÷  points       (when before = 0)
seconds_per_point = abs(duration) ÷ points      (when duration is set)
```

**To get per-second data, set `points` equal to the duration in
seconds.**

| You want | Set `after` | Set `points` | Result |
|---|---|---|---|
| Per-second resolution, last 2 minutes | `-120` | `120` | 1 second per point |
| Per-second resolution, last 5 minutes | `-300` | `300` | 1 second per point |
| 10-second buckets, last 10 minutes | `-600` | `60` | 10 seconds per point |
| Per-minute resolution, last hour | `-3600` | `60` | 60 seconds per point |

**Common mistake**: `after: -600, points: 30` is NOT per-second
data over 10 minutes -- it is 20-seconds-per-point heavily
aggregated data. Per-second resolution over 10 minutes requires
`points: 600` (at the 500-point server cap; reduce duration or
accept coarser resolution).

**Per-second data also requires dbengine tier 0** (per-second
storage) covers the requested time range. If tier 0 retention is
shorter than `abs(after)`, the engine auto-selects a coarser
tier silently. Force tier 0 with `"tier": 0` in the window to
fail loudly rather than silently downsample.

**`points: 0` is NOT "per-second"** -- it means "all available
points within the 500 cap", which the engine still aggregates
when the duration exceeds 500 seconds.

## Limits and gotchas

- **`scope.contexts` MUST be set.** Without it, the response
  contains metadata for every context on the agent.
- **`unaligned`**: include in `options` for API queries to avoid
  wall-clock alignment of the time window.
- **Max points ≈ 500** per query (server-side cap).
- **Single host.** For multi-node aggregation, use the Cloud
  `/data` path documented in
  [../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md).

## See also

- [../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md)
  -- full body / response / examples.
- [query-functions.md](./query-functions.md) -- generic Function
  transport (Functions != metrics, but they share the same
  Cloud-proxy and bearer semantics).
