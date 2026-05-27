# Query agent topology directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

The topology body and response payload are the same as the Cloud-proxied
transport. For the production topology schema, response fields, compact table
format, and interpretation rules, see
[../query-netdata-cloud/query-topology.md](../query-netdata-cloud/query-topology.md).

For `topology:network-connections`, process actors may include container and
orchestrator columns: `cgroup_path`, `cgroup_name`, `orchestrator`,
`k8s_pod_name`, `k8s_namespace`, `k8s_workload`, `docker_container_name`,
`docker_image`, and `systemd_unit_name`. Supported grouping ids are `pid`,
`process_name`, `cgroup`, `container`, `orchestrator`, `pod`, `namespace`,
`workload`, and `service`.

Use `processes:by_pid` for exact per-PID container attribution. Use
`labels:<pattern>` to opt in to free-form labels with pipe-separated
`simple_pattern` tokens, for example `labels:team|app`. Use
`cgroup-paths:hide` to suppress full cgroup paths.

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

Example with exact per-PID container grouping metadata:

```bash
agents_query_agent \
    --node "$NODE_UUID" \
    --host "$AGENT_TARGET" \
    --machine-guid "$AGENT_MG" \
    POST '/api/v3/function?function=topology:network-connections%20processes:by_pid%20labels:team%7Capp%20cgroup-paths:hide' \
    '{"timeout":60000}' \
  | jq '.data | {
      group_by: .view.group_by,
      process_scopes: .types.actor_types.process.aggregation_scopes,
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
