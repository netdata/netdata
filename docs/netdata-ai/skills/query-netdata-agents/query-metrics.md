# Query agent metrics directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the full request body (scope / selectors / window /
aggregations / format / options), the response envelope (jsonwrap
with summary / view / result / db / timings), the time-aggregation
and dimension-aggregation rules, and worked examples, see
[../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md).
The Cloud `/api/v3/spaces/{sp}/rooms/{rm}/data` endpoint accepts that
body, maps its fields to Agent query parameters, and forwards the
result to the Agent's `/api/v3/data` endpoint.

---

## Endpoint (agent v3)

`GET /api/v3/data` on the agent. Unlike the Cloud endpoint, the
direct Agent endpoint reads URL query parameters, not a JSON request
body. Cloud body fields map to Agent parameters such as
`scope.contexts` -> `scope_contexts`, `window.after` -> `after`, and
`aggregations.metrics[0].group_by` -> `group_by`. Always pass the
target node as `scope_nodes`; the `/host/{node}` URL prefix does not
scope the v3 data query itself.

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

QUERY="$(jq -rn \
  --arg scope_nodes "$NODE_UUID" \
  --arg scope_contexts 'system.cpu' \
  --arg nodes '*' \
  --arg contexts '*' \
  --arg instances '*' \
  --arg dimensions '*' \
  --arg labels '*' \
  --arg alerts '*' \
  --arg after '-600' \
  --arg before '0' \
  --arg points '5' \
  --arg group_by 'dimension' \
  --arg aggregation 'sum' \
  --arg time_group 'average' \
  --arg format 'json2' \
  --arg options 'jsonwrap,minify,unaligned' \
  --arg timeout '30000' \
  '$ARGS.named | to_entries
   | map("\(.key)=\(.value | @uri)")
   | join("&")')"

agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET "/api/v3/data?$QUERY" \
  | jq '{view: .view.dimensions.names, points: (.result.data | length)}'
```

## Discover available contexts on the agent

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET '/api/v3/contexts'
```

`/api/v3/contexts` returns the metric contexts the agent currently
collects (e.g. `system.cpu`, `disk.space`, `nginx.connections`).
Use these as `scope_contexts` values.

## Time resolution: `duration ÷ points = seconds per point`

The number of `points` is NOT "give me per-second data". It is
"split the duration into N equal buckets". Actual time
resolution:

```
seconds_per_point = abs(after)  ÷  points       (when before = 0)
seconds_per_point = abs(duration) ÷ points      (when duration is set)
```

**To request per-second buckets, set `points` equal to the duration
in seconds.** The matched metrics must also have a one-second native
collection interval covering that window; asking for more points
cannot create samples the database does not hold.

| You want                              | Set `after` | Set `points` | Result               |
|---------------------------------------|-------------|--------------|----------------------|
| Per-second resolution, last 2 minutes | `-120`      | `120`        | 1 second per point   |
| Per-second resolution, last 5 minutes | `-300`      | `300`        | 1 second per point   |
| 10-second buckets, last 10 minutes    | `-600`      | `60`         | 10 seconds per point |
| Per-minute resolution, last hour      | `-3600`     | `60`         | 60 seconds per point |

**Common mistake**: `after: -600, points: 30` is NOT per-second
data over 10 minutes -- it is 20-seconds-per-point heavily
aggregated data. Per-second resolution over 10 minutes requires
`points: 600`.

**Native-resolution data requires dbengine tier 0** to cover the
requested time range. Tier 0 follows each metric's collection
interval; it is per-second only for metrics collected every second.
Requesting `"tier": 0` is not an availability assertion. A valid tier
with partial overlap returns only that overlap, without gap-filling; a
valid tier with no overlap returns no data for that metric; and a
structurally invalid tier request can fall back to automatic tier
selection. Check `db.per_tier` to confirm which tier supplied data.
Add `debug` to `options` when you also need
`view.partial_data_trimming` details.

**`points: 0` is NOT "per-second" or "all available points"** -- on
the v3 data endpoint it leaves the point target to the query planner's
default virtual-point behavior. The planner derives, groups, and
normalizes the final row count from the requested window and available
data. Do not rely on `points: 0` producing a particular resolution or
row count.

## Limits and gotchas

- **`scope_contexts` MUST be set.** Without it, the response
  contains metadata for every context on the agent.
- **`scope_nodes` MUST identify the target node.** The
  `/host/{node}` prefix routes and authorizes the request, but the v3
  data query can still enumerate every node hosted by that Agent or
  Parent unless its query scope is explicit.
- **`unaligned`**: include in `options` for API queries to avoid
  wall-clock alignment of the time window.
- **No stable 86,400-row guarantee.** The agent query planner
  initially clamps a larger requested target to 86,400 points, but
  later window normalization can adjust the final row count above
  that value. Treat 86,400 as an internal target, not an absolute
  output limit.
- **Cloud's 500-point clamp is conditional.** Netdata Cloud applies
  `ScopeDataRequestMaxPoints` only when it must aggregate responses
  from multiple Agent routes. A single-route Cloud request is passed
  through, and `points: 0` does not trigger the `points > 500`
  condition. The direct Agent REST API never applies this Cloud
  clamp. See
  [../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md).
- **Single host.** For multi-node aggregation, use the Cloud
  `/data` path documented in
  [../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md).

## See also

- [../query-netdata-cloud/query-metrics.md](../query-netdata-cloud/query-metrics.md)
  -- full body / response / examples.
- [query-functions.md](./query-functions.md) -- generic Function
  transport (Functions != metrics, but they share the same
  Cloud-proxy and bearer semantics).
