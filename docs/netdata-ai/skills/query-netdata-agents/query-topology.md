# Query agent topology directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

For the body parameters (`nodes_identity`, `map_type`,
`inference_strategy`, `managed_snmp_device_focus`, `depth`),
the response envelope (top-level `data.actors[]` + `data.links[]`),
and per-actor / per-link field semantics, see
[../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md).
The body and response are identical between Cloud-proxied and
direct-agent calls.

Today only `topology:snmp` is registered. Future topology Functions
will follow the same `topology:<source>` namespace and the same
envelope.

---

## Endpoint (agent v3)

`POST /api/v3/function?function=topology:snmp`

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

read -r -d '' BODY <<'JSON'
{
  "selections": {
    "nodes_identity":            ["mac"],
    "map_type":                  ["lldp_cdp_managed"],
    "inference_strategy":        ["fdb_minimum_knowledge"],
    "managed_snmp_device_focus": ["all_devices"],
    "depth":                     ["all"]
  },
  "timeout": 60000
}
JSON

agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:snmp' "$BODY" \
  | jq '.data | {actors: (.actors|length), links: (.links|length), view, layer}'
```

## Discover supported parameters

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:snmp' '{"info":true}' \
  | jq '.required_params'
```

## Limits and gotchas

- **Slow queries.** A full SNMP sweep on a busy network can take
  60+ seconds. Set `timeout` accordingly.
- **MAC-list `actor_id` values can be very long.** Use
  `nodes_identity:["ip"]` to collapse devices by IP if you prefer
  shorter ids.
- **Single-agent perspective.** The topology graph is what THIS
  agent has discovered. Multi-agent fleets need merge logic on
  the client side.

## See also

- [../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md)
  -- full reference, parameter values, body / response detail.
- [query-functions.md](./query-functions.md) -- generic Function
  transport.
