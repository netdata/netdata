# Query agent topology directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

The topology body and response payload are the same as the Cloud-proxied
transport. For the production topology schema, response fields, compact table
format, and interpretation rules, see
[../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md).

For `topology:network-connections`, supported grouping ids are `process_name`,
`pid`, and `container`. `group_by:pid` emits one process actor per PID and is
the only view that exposes raw fields such as PID, UID, command line, cgroup
path, and detailed container metadata. `group_by:container` emits container
actors grouped by canonical `container_name`.

Use `labels:<pattern>` to opt in to free-form labels with pipe-separated
`simple_pattern` tokens, for example `labels:team|app`.

## Endpoint

`POST /api/v3/function?function=topology:<source>`

Example:

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
AGENT_URL="${AGENT_URL:-http://${AGENT_HOST:-127.0.0.1}:${AGENT_PORT:-19999}}"
AGENT_TARGET="${AGENT_URL#http://}"
AGENT_TARGET="${AGENT_TARGET#https://}"
AGENT_TARGET="${AGENT_TARGET%%/*}"

read -r -d '' BODY <<'JSON'
{
  "selections": {
    "mode": ["aggregated"]
  },
  "timeout": 60000
}
JSON

agents_query_agent \
    --node "$NODE_UUID" \
    --host "$AGENT_TARGET" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:network-connections' "$BODY" \
  | jq '.data | {
      schema: .schema_version,
      actors: .actors.rows,
      links: .links.rows,
      evidence_rows: ([.evidence[]?.table.rows] | add // 0)
    }'
```

Example with exact per-PID raw fields and grouping metadata:

```bash
agents_query_agent \
    --node "$NODE_UUID" \
    --host "$AGENT_TARGET" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:network-connections' \
    '{"timeout":60000,"selections":{"group_by":["pid"],"labels":["team|app"]}}' \
  | jq '.data | {
      group_by: .view.group_by,
      process_scopes: .types.actor_types.process.aggregation_scopes,
      container_scopes: .types.actor_types.container.aggregation_scopes,
      actor_columns: [.actors.columns[].id]
    }'
```

## Discover supported parameters

```bash
agents_query_agent \
    --node "$NODE_UUID" \
    --host "$AGENT_TARGET" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:network-connections' \
    '{"info":true,"timeout":30000}' \
  | jq '.required_params'
```

## Notes

- The graph is the perspective of the queried Agent or producer instance.
- Fleet-wide views require Cloud aggregation over multiple topology payloads.
- High-cardinality relationship facts live in evidence sections, not graph
  links.
- Topology Functions should fail explicitly on size limits; they must not
  silently truncate evidence.

## See also

- [../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md)
  -- full response reference.
- [query-functions.md](./query-functions.md) -- generic direct-agent Function
  transport.
