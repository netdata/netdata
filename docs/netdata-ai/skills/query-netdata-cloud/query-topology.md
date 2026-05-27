# Query topology Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.
For the generic Function transport, see [query-functions.md](./query-functions.md).

Topology Functions return compact graph payloads using the production topology
schema:

- [FUNCTION_TOPOLOGY_SCHEMA.json](../../../../src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json)

The response contains actors, graph links, relationship evidence, optional
actor detail tables, and optional telemetry overlay refs. Large sections use
compact columnar tables.

## Function namespace

Topology Functions use the `topology:<source>` namespace.

Known producer families:

| Function | Source | Typical topology |
|---|---|---|
| `topology:network-connections` | Network Viewer plugin | process, endpoint, socket evidence |
| `topology:streaming` | Netdata streaming subsystem | parent/child streaming graph |
| `topology:snmp` | SNMP topology collector | L2 devices, interfaces, endpoints, adjacencies |
| `topology:vsphere` | vSphere collector (planned) | inventory and virtualization relationships |

Always start with an info request to discover the parameters supported by the
Agent version you are querying.

## Network-connections grouping

`topology:network-connections` process actors can expose container and
orchestrator columns when the Agent has APPS_LOOKUP cache data for the PID:

`cgroup_path`, `cgroup_name`, `orchestrator`, `k8s_pod_name`,
`k8s_namespace`, `k8s_workload`, `docker_container_name`, `docker_image`, and
`systemd_unit_name`.

The payload advertises these `view.group_by` ids:

`pid`, `process_name`, `cgroup`, `container`, `orchestrator`, `pod`,
`namespace`, `workload`, and `service`.

Useful request arguments:

- `processes:by_pid` returns per-PID process actors, which gives exact
  container attribution when the same process name runs in multiple containers.
- `labels:<pattern>` allows optional free-form actor labels. Omit it to hide
  free-form labels. Tokens are pipe-separated, for example
  `labels:team|app|version-*`; commas are literal.
- `cgroup-paths:hide` hides full `cgroup_path` values while leaving the other
  grouping columns available.

If a grouping column is null or hidden, consumers should preserve actor identity
for that row instead of merging every null row into one bucket.

## Endpoint

Use the standard Cloud Function endpoint:

`POST /api/v2/nodes/{nodeId}/function?function=topology:<source>`

Example info request:

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections" \
  -d '{"info":true,"timeout":30000}'
```

Example data request:

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "selections": {
    "mode": ["aggregated"]
  },
  "timeout": 60000
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections" \
  -d "$PAYLOAD"
```

Example with exact per-PID container columns, selected labels, and hidden full
cgroup paths:

```bash
curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections%20processes:by_pid%20labels:team%7Capp%20cgroup-paths:hide" \
  -d '{"timeout":60000}' \
  | jq '.data | {
      group_by: .view.group_by,
      process_scopes: .types.actor_types.process.aggregation_scopes,
      actor_columns: [.actors.columns[].id]
    }'
```

Example Kubernetes pod/namespace view inspection:

```bash
curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=topology:network-connections%20processes:by_pid" \
  -d '{"timeout":60000}' \
  | jq '.data.actors as $actors
        | ($actors.columns | map(.id)) as $cols
        | ($cols | index("k8s_namespace")) as $ns
        | ($cols | index("k8s_pod_name")) as $pod
        | {namespaces: $actors.values[$ns].values, pods: $actors.values[$pod].values}'
```

## Response shape

Top-level response:

```json
{
  "status": 200,
  "type": "topology",
  "has_history": false,
  "data": {
    "schema_version": "netdata.topology.v1",
    "producer": {},
    "collected_at": "2026-05-09T10:00:00Z",
    "dictionaries": {},
    "types": {},
    "actors": {},
    "links": {},
    "evidence": {},
    "tables": {},
    "overlays": {},
    "stats": {}
  }
}
```

The important fields:

| Field | Description |
|---|---|
| `data.schema_version` | Topology contract version, currently `netdata.topology.v1` |
| `data.producer` | Producer source, instance, node, plugin, and version metadata |
| `data.dictionaries` | Shared dictionaries, especially `strings` |
| `data.types.actor_types` | Actor identity and aggregation-scope metadata |
| `data.types.link_types` | Link direction and aggregation policy |
| `data.types.evidence_types` | Evidence role and exact match columns |
| `data.actors` | Compact table of graph actors |
| `data.links` | Compact table of renderable graph links |
| `data.evidence` | Compact relationship evidence sections |
| `data.tables` | Optional actor or relationship detail tables |
| `data.overlays` | Optional metric/function overlay refs |
| `data.stats` | Producer and payload counters |

## Decode compact tables

Every table has:

- `rows`: number of rows;
- `columns`: column definitions;
- `values`: parallel array of column encodings.

Supported codecs:

| Codec | Meaning |
|---|---|
| `const` | one value repeated for all rows |
| `values` | one value per row |
| `dict` | per-column dictionary plus row indexes |

Minimal jq-friendly counts:

```bash
jq '.data | {
  schema: .schema_version,
  actors: .actors.rows,
  links: .links.rows,
  evidence_rows: ([.evidence[]?.table.rows] | add // 0),
  stats
}'
```

## Interpretation rules

- Actors are entities.
- Links are graph edges.
- Evidence rows are the exact facts behind links.
- Actor custom tables are separate from relationship evidence.
- Direction semantics come from `data.types.link_types`.
- Telemetry overlays come from `data.types.overlay_templates` plus
  `data.overlays.refs`.

Do not assume every evidence row is rendered as a graph edge. A single graph
link may summarize many evidence rows.

## See also

- [query-functions.md](./query-functions.md) -- generic Function transport.
- [../query-netdata-agents/query-topology.md](../query-netdata-agents/query-topology.md)
  -- direct-agent transport for the same topology payload.
