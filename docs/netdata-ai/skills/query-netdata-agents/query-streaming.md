# Query agent streaming (parent / child / replication)

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

Netdata agents form a streaming graph: a **child** sends its
metrics to a **parent**, which can in turn forward to another
parent. The same agent can be both a parent (receiving from
children) and a child (sending upstream). This guide covers how to
query that graph from a specific agent's perspective.

There is **no Cloud-side equivalent** for the agent-internal
streaming/replication state -- this surface is agent-only.

---

## Function name

The agent registers a Function called `netdata-streaming` (verified
live; see
[../query-netdata-cloud/query-functions.md#frequently-registered-functions](../query-netdata-cloud/query-functions.md#frequently-registered-functions)).
It returns:

- Per-streaming-peer connection state (replication progress,
  bytes-in / bytes-out, last-error).
- Whether this agent is acting as a parent, a child, or both.
- The list of children currently streaming to this agent.
- The parent endpoints this agent is streaming to.
- ML status of the streaming pipeline (if ML is enabled).

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Discover the parameters first.
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=netdata-streaming' '{"info":true}' \
  | jq '{accepted_params, required_params}'

# Real query: top-level streaming state.
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=netdata-streaming' '{"timeout":30000}'
```

The response uses the standard Function envelope; `data` holds
the per-peer rows. See
[../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md)
for the envelope definition and the canonical
`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`.

## Question-to-query cheatsheet

| Question | Approach |
|---|---|
| "Is this node a parent?" | Check `summary.nodes[0].labels._is_parent` from a metrics query (cheapest), OR look at the `netdata-streaming` Function's per-peer rows -- if any `direction:incoming` rows exist, this node is a parent. |
| "Of how many and which nodes?" | Filter the Function's rows where `direction:incoming` -- one per child. Each row carries the child's hostname and node id. |
| "Is this node a child? Where does it stream?" | Check `summary.nodes[0].labels._is_parent == false` AND look for `direction:outgoing` rows in the Function -- the destination is the upstream parent. |
| "What is the replication progress?" | Each row has replication-related fields (`replication_progress`, `replication_lag`, `replication_eta`). |
| "Are there any disconnected peers?" | Filter rows where `state` != `connected` / `streaming`. |

## Ancillary surfaces

- **Per-stream metrics** (bandwidth, packets, dropped points) are
  exposed as Netdata charts under the `netdata.streaming.*`
  context family. Use [query-metrics.md](./query-metrics.md) to
  query them as time series.
- **Streaming configuration** (which parents this agent connects
  to, retention settings) lives in
  `/etc/netdata/stream.conf`. The DynCfg path is
  `/streaming` if your agent version exposes streaming config
  through DynCfg; check via
  `agents_query_agent ... GET '/api/v3/config?action=tree&path=/streaming'`.
- **`_is_parent` / `_is_ephemeral`** chart-labels surface the
  parent / ephemeral state succinctly; see
  [query-nodes.md](./query-nodes.md).

## Limits and gotchas

- **Function name**: literal `netdata-streaming` (note the dash,
  not a colon). The listing endpoint is the only authoritative
  source -- if your agent is older, the Function name may differ.
- **Cloud aggregation does not exist** for streaming state. To
  build a fleet-wide view, fan out per-agent calls and merge
  client-side.
- **Per-peer rows can be tens of KB each** when there are many
  children. Use `last` or pagination knobs in the Function body
  if needed.
- **Agent must have streaming enabled.** A standalone (non-
  streaming) agent has no rows; the Function still returns 200
  but with an empty `data` array.

## See also

- [query-functions.md](./query-functions.md) -- generic Function
  transport.
- [../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md)
  -- canonical Function reference + envelope.
- [query-nodes.md](./query-nodes.md) -- node identity, parent /
  child labels.
- `<repo>/src/streaming/` -- agent-side streaming implementation.
