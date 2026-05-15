# List nodes via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

For a single agent's own identity (`/api/v3/info` direct, hardware
labels, vnodes, parent/child role), see
[../query-netdata-agents/query-nodes.md](../query-netdata-agents/query-nodes.md).
This file covers the **Cloud-side** enumeration -- nodes across a
room, across a space, with full metadata payloads.

---

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v3/spaces/{spaceID}/rooms/{roomID}/nodes` | List nodes in a room (full metadata) |

The body is `{}` for "all nodes in the room" or accepts
filters/options that mirror the metrics-query body's
`scope`/`selectors` shape -- see
[query-metrics.md](./query-metrics.md) for the cross-cutting
filter language.

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}'
```

The wrapper emits only the response body; `NETDATA_CLOUD_TOKEN`
never reaches stdout.

## Per-node response fields

Verified live -- response is a JSON array, each entry an object:

| Field | Description |
|---|---|
| `nd` | **Node UUID.** This is the value to pass anywhere the API expects a node id (e.g. `/api/v2/nodes/{nd}/function?...`) |
| `mg` | Machine GUID (stable per OS install) |
| `nm` | Hostname |
| `state` | `reachable` (live) / `stale` (disconnected) / `offline` |
| `v` | Agent version (e.g. `v2.10.3-nightly`) |
| `labels` | Object: all `_*` chart-labels keyed by name (architecture, kernel, OS, CPU count, RAM, container/k8s/cloud-provider info) |
| `hw` | `{cpus, memory, disk_space, architecture}` summary |
| `os` | `{nm, v, kernel}` summary |
| `health` | Alert-status summary: `{status, alerts: {warning, critical}}` |
| `capabilities` | Feature flags: `ml`, `funcs`, `health`, etc. |
| `room_memberships` | Other rooms this node is in |
| `eligibility` | Per-feature eligibility (e.g. for paid features) |
| `replication`, `replication_factor` | Streaming / parent-child replication state |
| `isPreferred` | Whether the node is the preferred parent for its room |

## Common patterns

```bash
# Hostname -> node UUID lookup.
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}' \
  | jq -r --arg HOST "costa-desktop" '.[] | select(.nm==$HOST) | .nd'

# All "reachable" nodes' (UUID, hostname, version) tuples.
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}' \
  | jq -r '.[] | select(.state=="reachable") | "\(.nd)\t\(.nm)\t\(.v)"'

# Nodes whose label `_is_parent` is true.
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}' \
  | jq -r '.[] | select(.labels._is_parent=="true") | .nm'

# Aggregate by cloud provider.
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}' \
  | jq -r '[.[] | .labels._cloud_provider_type // "unknown"] | group_by(.) | map({(.[0]): length}) | add'
```

## Hardware / OS facts

Hardware and OS facts live in `.labels`:

| Question | Field |
|---|---|
| CPU architecture | `.labels._architecture` |
| Kernel version | `.labels._kernel_version` |
| OS name / version | `.labels._os_name`, `._os_version` |
| CPU cores | `.labels._system_cores` |
| Total RAM (bytes) | `.labels._system_ram_total` |
| Total disk space (bytes) | `.labels._system_disk_space` |
| Container / virt | `.labels._container`, `._is_k8s_node` |
| Cloud provider / region / instance type | `.labels._cloud_provider_type`, `._cloud_instance_region`, `._cloud_instance_type` |
| Parent role | `.labels._is_parent` (`"true"` / `"false"` strings) |

For deeper per-host introspection (vnodes, failed jobs, claim_id),
fall through to the agent-direct path in
[../query-netdata-agents/query-nodes.md](../query-netdata-agents/query-nodes.md)
and [../query-netdata-agents/query-dyncfg.md](../query-netdata-agents/query-dyncfg.md).

## Limits and gotchas

- **Stale nodes appear in the list** -- always check `.state`
  before issuing further queries against `nd`.
- **The full label set is large** (50+ keys per node). Use
  `jq` projections to keep responses readable.
- **Cross-room view requires re-querying.** A node can be in
  multiple rooms; use `room_memberships` to detect duplicates
  when aggregating across rooms.
- **Multi-space view requires multiple Cloud calls.** Iterate
  over `/api/v2/spaces` -> `/api/v2/spaces/{sp}/rooms` ->
  `/api/v3/spaces/{sp}/rooms/{rm}/nodes`.

## See also

- [query-rooms.md](./query-rooms.md) -- enumerate rooms (and
  their `node_count`, `member_count`, permissions).
- [query-functions.md](./query-functions.md) -- invoke a Function
  on a specific node by UUID.
- [../query-netdata-agents/query-nodes.md](../query-netdata-agents/query-nodes.md)
  -- single-host identity, vnodes, claim_id.
