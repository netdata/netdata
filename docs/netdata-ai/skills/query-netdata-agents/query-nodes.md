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

| Method | Path                          | Purpose                                                                                                                               |
|--------|-------------------------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `GET`  | `/api/v3/info`                | Identity (node_id, machine_guid, claim_id), agent version, application info, capabilities. **No auth required**, but only basic info. |
| `GET`  | `/api/v1/info`                | Host labels (`.host_labels`): OS, kernel, architecture, hardware, cloud provider, container/virtualization, streaming role. See "Hardware / OS query patterns" below. |
| `GET`  | `/api/v3/contexts`            | Metric contexts the agent collects (= what data is available)                                                                         |
| `GET`  | `/api/v3/nodes`               | Multi-host listing if this agent acts as a parent (see [query-streaming.md](./query-streaming.md))                                    |
| `GET`  | `/api/v3/info?host=<node_id>` | Detail for a specific host (when multi-host)                                                                                          |

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

# Hardware and OS labels are NOT under /api/v3/info -- that endpoint's
# agents[0] object has no `labels` key (verified: v2.10.3 returns only
# ai, api, application, capabilities, cloud, contexts, db_size,
# instances, metrics, mg, nd, nm, nodes, now, timings). See "Hardware
# / OS query patterns" below for the endpoint that actually has them.
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
      ...
    }
  ],
  ...
}
```

`agents[0]` has no `labels` key. For host OS/hardware/cloud facts,
use `/api/v1/info` instead (see below). `summary.nodes[]` in a
metrics-query response (see [query-metrics.md](./query-metrics.md))
also has no `.labels` field -- it carries only `mg`, `nd`, `nm`,
`ni`, `st`, `is`, `ds`, `al`, `sts`. A metrics response's
`summary.labels[]` (flat, not per-node) carries **chart labels**
(e.g. a disk's `device_type`/`model`/`serial`) -- useful for
per-instance/per-dimension metadata, but not host OS/hardware facts.

## Hardware / OS query patterns

Hardware, OS, and cloud-provider facts live in the agent's
**host labels**, exposed at `GET /api/v1/info` under `.host_labels`
(verified live on v2.10.3; not exposed anywhere under `/api/v3/info`
or a v3 metrics response):

```bash
agents_query_agent --node "$NODE_UUID" --host "$AGENT_HOST:19999" --machine-guid "$AGENT_MG" \
    GET /api/v1/info \
  | jq '.host_labels'
```

| Field                                   | `.host_labels` key(s)                                                             |
|-----------------------------------------|-------------------------------------------------------------------------------------|
| Architecture, kernel, OS name/version   | `_architecture`, `_kernel_version`, `_os_name`, `_os_version`, `_os`                |
| CPU count, RAM, disk space               | `_system_cores`, `_system_cpu_model`, `_system_ram_total`, `_system_disk_space`     |
| Hardware vendor/product                 | `_hw_sys_vendor`, `_hw_product_name`, `_hw_product_type`                           |
| Cloud provider / region / instance type | `_cloud_provider_type`, `_cloud_instance_region`, `_cloud_instance_type`            |
| Container/virtualization                | `_container`, `_container_detection`, `_virtualization`, `_virt_detection`, `_is_k8s_node` |
| Streaming role / ephemerality            | `_is_parent`, `_is_ephemeral`                                                       |

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

- **`/api/v1/info` and `/api/v3/info` are unauthenticated**, but
  most other paths require the bearer. The wrapper always uses the
  bearer; that's fine for `/info` too.
- **Host OS/hardware/cloud facts live only in `/api/v1/info`'s
  `.host_labels`.** Neither `/api/v3/info`'s `agents[0]` nor a v3
  metrics response's `summary.nodes[]` carries a `labels` field
  (verified live on v2.10.3) -- don't assume the v3 endpoints expose
  this data under a differently-shaped field.
- **Streaming role** (parent / child) is `.host_labels._is_parent`
  (true/false) from `/api/v1/info`. The full streaming surface
  lives in [query-streaming.md](./query-streaming.md).

## See also

- [../query-netdata-cloud/query-nodes.md](../query-netdata-cloud/query-nodes.md)
  -- per-room / per-space node enumeration via Cloud.
- [query-dyncfg.md](./query-dyncfg.md) -- DynCfg surface (jobs,
  vnodes, config).
- [query-streaming.md](./query-streaming.md) -- parent/child
  streaming relationships and replication state.
- [query-metrics.md](./query-metrics.md) -- chart-labels (per
  instance/dimension, e.g. disk `device_type`/`model`/`serial`) via
  `summary.labels[]` of metric queries -- distinct from host
  OS/hardware facts, which are `/api/v1/info`'s `.host_labels`.
