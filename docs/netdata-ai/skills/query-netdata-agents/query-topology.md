# Query agent topology directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

The topology body and response payload are the same as the Cloud-proxied
transport. For the production topology schema, response fields, compact table
format, and interpretation rules, see
[../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md).

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
      evidence_rows: (.evidence | to_entries | map(.value.table.rows) | add // 0)
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
