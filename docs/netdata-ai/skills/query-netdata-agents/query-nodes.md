# Query an agent's node identity and metadata directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first.

The agent exposes its identity, capabilities, hardware, OS, and
collection-job state via several endpoints. Unlike Cloud's `/nodes`
(which lists multiple nodes in a room), agent-direct calls return
data for the **single host** the agent runs on (plus any virtual
hosts / parents / streamed children, see
[query-streaming.md](./query-streaming.md)).

For the Cloud-side per-room enumeration (`POST
/api/v3/spaces/{sp}/rooms/{rm}/nodes`), see
[../query-netdata-cloud/query-nodes.md](../query-netdata-cloud/query-nodes.md).

---

## Endpoints (agent v3)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v3/info` | Identity (node_id, machine_guid, claim_id), agent version, application info, capabilities. **No auth required**, but only basic info. |
| `GET` | `/api/v3/contexts` | Metric contexts the agent collects (= what data is available) |
| `GET` | `/api/v3/nodes` | Multi-host listing if this agent acts as a parent (see [query-streaming.md](./query-streaming.md)) |
| `GET` | `/api/v3/info?host=<node_id>` | Detail for a specific host (when multi-host) |

## Use the wrappers

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# /info is unauthenticated -- you can call it without the bearer.
# But going through agents_query_agent uses the bearer flow, which
# is fine and consistent.
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET /api/v3/info \
  | jq '.agents[0] | {nm, nd, mg, cloud, application: .application.package.version}'

# Hardware and OS labels (typically exposed under .labels in /info or
# in the chart-labels namespace; see the response shape).
agents_query_agent \
    --node    "$NODE_UUID" \
    --host    "$AGENT_HOST:19999" \
    --machine-guid "$AGENT_MG" \
    GET /api/v3/info \
  | jq '.agents[0].application, .agents[0].cloud'

# Claim_id specifically (used by the bearer-mint flow).
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET /api/v3/info | jq -r '.agents[0].cloud.claim_id'
```

## Top-level response shape (`/api/v3/info`)

```text
{
  "api": 2,
  "agents": [
    {
      "mg":     "<machine_guid UUID>",
      "nd":     "<node UUID>",
      "nm":     "<hostname>",
      "now":    <unix-seconds>,
      "ai":     <agent index>,
      "application": {
        "package": { "version": "vX.Y.Z-...", "type": "binpkg-deb|...", "arch": "x86_64|...", ... },
        "configure": "cmake -...",
        ...
      },
      "cloud": {
        "claim_id": "<UUID>",
        "aclk":     "available|online|...",
        ...
      },
      "labels": { ... },
      ...
    }
  ],
  ...
}
```

For the full per-agent label set, the `chart-labels` are usually
exposed via the metrics path's `summary.nodes[].labels` (see
[query-metrics.md](./query-metrics.md)).

## Hardware / OS query patterns

Hardware and OS facts live primarily in the agent's host-labels.
On a fully-running agent the labels are present in `/api/v3/info`
and copied verbatim into the Cloud `/nodes` `.labels` field (the
fast path for cross-fleet queries).

| Field | Where it lives |
|---|---|
| Architecture, kernel, OS name/version | `agents[0].application` (build-time) AND `summary.nodes[].labels._architecture`, `_kernel_version`, `_os_name`, `_os_version` |
| CPU count, RAM, disk space | `summary.nodes[].labels._system_cores`, `_system_ram_total`, `_system_disk_space` (chart-labels) |
| Cloud provider / region / instance type | `summary.nodes[].labels._cloud_provider_type`, `_cloud_instance_region`, `_cloud_instance_type` |
| Container/virtualization | `summary.nodes[].labels._container`, `_container_detection`, `_is_k8s_node`, `_is_parent`, `_is_ephemeral` |

To fetch chart-labels as a structured object, run a metrics query
and read `summary.nodes[].labels`:

```bash
read -r -d '' BODY <<'JSON'
{
  "scope":     {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"]},
  "window":    {"after": -60, "before": 0, "points": 1},
  "aggregations": {"metrics": [{"group_by": ["selected"]}], "time": {"time_group": "average"}},
  "format":  "json2",
  "options": ["jsonwrap", "minify", "unaligned"]
}
JSON
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    POST /api/v3/data "$BODY" \
  | jq '.summary.nodes[0].labels'
```

## Collection-job state (failed / disabled jobs)

The agent's DynCfg surface lists every collection job and its
status. See [query-dyncfg.md](./query-dyncfg.md):

```bash
# List every go.d.plugin job and its current state.
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET '/api/v3/config?action=tree&path=/collectors/go.d/Jobs' \
  | jq '.tree["/collectors/go.d/Jobs"]'
```

DynCfg job statuses include `running` (200), `accepted` (202),
`accepted-disabled` (298), `accepted-restart-required` (299),
plus error states (4xx/5xx). A failed-collection job appears with
a 4xx/5xx status and an error message.

## Vnodes

Virtual nodes (configured via `/etc/netdata/vnodes/`) are listed
under `/collectors/go.d/Vnodes` and `/collectors/ibm.d/Vnodes` in
the DynCfg tree. Use the same DynCfg path:

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET '/api/v3/config?action=tree&path=/collectors/go.d/Vnodes'
```

## Limits and gotchas

- **`/api/v3/info` is unauthenticated**, but most other paths
  require the bearer. The wrapper always uses the bearer; that's
  fine for `/info` too.
- **`labels` location varies by version.** On older agents some
  labels appear only in `summary.nodes[].labels` of metrics
  responses; on newer agents they're also under
  `agents[0].labels`. Check both.
- **Streaming roles** (parent / child) are at
  `summary.nodes[].labels._is_parent` (true/false), and the
  full streaming surface lives in
  [query-streaming.md](./query-streaming.md).

## See also

- [../query-netdata-cloud/query-nodes.md](../query-netdata-cloud/query-nodes.md)
  -- per-room / per-space node enumeration via Cloud.
- [query-dyncfg.md](./query-dyncfg.md) -- DynCfg surface (jobs,
  vnodes, config).
- [query-streaming.md](./query-streaming.md) -- parent/child
  streaming relationships and replication state.
- [query-metrics.md](./query-metrics.md) -- chart-labels via
  `summary.nodes[].labels` of metric queries.
