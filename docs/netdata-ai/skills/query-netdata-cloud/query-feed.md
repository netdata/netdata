# Query the event feed via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

The Cloud event feed is an audit + activity log: node lifecycle
events, alert transitions, agent connection events, space and room
membership changes, and configuration changes. It is served by the
**`cloud-feed-service`** (separate microservice from
spaceroom/charts) and answers via Elasticsearch under the hood.

There is no agent-side equivalent. The feed is Cloud-only.

---

## Endpoint

`POST /api/v1/feed/search` -- search the feed.

This is a v1 path (the only supported path for this service today).

The companion search-lean variant (`/api/v1/feed/search/lean`)
returns hits without the full source documents -- use it when you
only need aggregations / counts.

The facets endpoint
(`GET /api/v1/feed/static/facets`) returns the supported facet
field schema (mostly for UI rendering).

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Last 10 events in a space.
read -r -d '' BODY <<EOF
{
  "space_id":  "$SPACE",
  "page_size": 10
}
EOF

agents_query_cloud POST /api/v1/feed/search "$BODY"
```

## Body parameters

| Field | Type | Purpose |
|---|---|---|
| `space_id` | string (UUID) | **REQUIRED.** Space to search within |
| `room_ids` | array<string> | Filter to specific rooms |
| `agents` | array<string> | Filter to specific agent ids (`mg` field of nodes) |
| `node_ids` | array<string> | Filter to specific node ids (`nd` field) |
| `actions` | array<string> | Filter by event action (see enum below) |
| `alert_classes`, `alert_components`, `alert_names`, `alert_roles`, `alert_statuses`, `alert_transitions`, `alert_types` | array<string> | Alert-event filters |
| `chart_names`, `chart_contexts`, `chart_types` | array<string> | Chart-related event filters |
| `from`, `to` | int (Unix-millis) | Time range |
| `query` | string | Free-text search |
| `page_size` | int | Page size |
| `from_offset` | int | Pagination offset |

### `actions` enum (verified live)

Node lifecycle:
- `node-created`, `node-removed`, `node-deleted`, `node-restored`
- `node-state-live`, `node-state-stale`, `node-state-offline`

Agent lifecycle:
- `agent-connected`, `agent-disconnected`, `agent-claimed`

Alerts:
- `alert-node-transition`, `alert-node_instance-transition`

User / space / room:
- `user-create`, `user-created`
- `space-created`, `space-deleted`, `space-settings-changed`
- `space-user-added`, `space-user-removed`
- `user-space-permissions-changed`
- `room-created`, `room-deleted`
- `room-user-added`, `room-user-removed`
- `user-room-permissions-changed`

## Response shape

```text
{
  "page_size": <int>,
  "results": {
    "hits": {
      "total": { "value": <int> },
      "hits": [
        {
          "_source": {
            "@timestamp":  "<RFC3339>",
            "trace":       { "id": "<UUID>" },
            "agent":       { "version": "..." },
            "host":        { "id": "<machine_guid>", "name": "<hostname>", ... },
            "Netdata":     { "alert": {...}, "event": {...}, ... },
            "ecs":         { "version": "..." }
          },
          "_index": "...",
          "_id":    "...",
          "_score": <float>
        },
        ...
      ]
    },
    "aggregations": {
      "actions":         { "buckets": [...] },
      "agents":          { "buckets": [...] },
      "alert_classes":   { "buckets": [...] },
      "alert_components":{ "buckets": [...] },
      "alert_names":     { "buckets": [...] },
      "alert_roles":     { "buckets": [...] },
      ...
    }
  }
}
```

The hit fields under `_source` follow the **ECS (Elastic Common
Schema) v8.4.0** layout for shared keys (`@timestamp`, `host.*`,
`agent.*`, `ecs.*`) plus a Netdata-specific `Netdata.*` envelope
that holds the per-event payload.

## Common patterns

```bash
# Last hour of node-state changes.
read -r -d '' BODY <<EOF
{
  "space_id":  "$SPACE",
  "actions":   ["node-state-live","node-state-stale","node-state-offline"],
  "from":      $(( ($(date +%s) - 3600) * 1000 )),
  "to":        $(( $(date +%s) * 1000 )),
  "page_size": 50
}
EOF
agents_query_cloud POST /api/v1/feed/search "$BODY" \
  | jq -r '.results.hits.hits[]._source | "\(.["@timestamp"])\t\(.Netdata.event.action // "?")\t\(.host.name // "?")"'

# Distribution of alert classes triggered in the last 24h.
read -r -d '' BODY <<EOF
{
  "space_id":  "$SPACE",
  "actions":   ["alert-node_instance-transition"],
  "from":      $(( ($(date +%s) - 86400) * 1000 )),
  "to":        $(( $(date +%s) * 1000 )),
  "page_size": 0
}
EOF
agents_query_cloud POST /api/v1/feed/search "$BODY" \
  | jq -r '.results.aggregations.alert_classes.buckets[] | "\(.key)\t\(.doc_count)"'

# All space-user-added events for a given account in the last 7 days.
ACCT="<account-uuid>"
read -r -d '' BODY <<EOF
{
  "space_id":  "$SPACE",
  "actions":   ["space-user-added"],
  "from":      $(( ($(date +%s) - 604800) * 1000 )),
  "to":        $(( $(date +%s) * 1000 )),
  "page_size": 50
}
EOF
agents_query_cloud POST /api/v1/feed/search "$BODY" \
  | jq --arg id "$ACCT" '.results.hits.hits[] | select(._source.user.id // "" == $id)'
```

## Limits and gotchas

- **`from`/`to` are Unix milliseconds.** Easy to confuse with
  seconds.
- **Total hit count is paginated.** Use `from_offset` to walk
  past the first page; `total.value` tells you the size.
- **`actions` is the most useful facet.** Most queries should
  start by narrowing by `actions[]`; the per-action `_source`
  shape varies, so filter first then read the appropriate
  per-event fields.
- **Hits include personal data.** `host.name`, `user.id`,
  `user.email`, `agent.version`, alert config_hash UUIDs --
  treat raw responses as semi-sensitive; never paste into
  committed artifacts.
- **Retention is finite.** The feed-service has retention
  enforcement (`errInvalidRetention` error message in source);
  very old time windows return errors.
- **No agent-side equivalent.** This is the only path to the
  audit/activity feed; agents do not retain it locally.

## See also

- [query-rooms.md](./query-rooms.md), [query-members.md](./query-members.md)
  -- the surfaces whose changes generate room-/member-related
  feed events.
- [query-alerts.md](./query-alerts.md) -- alert transitions are
  also emitted into the feed via `alert-node-transition` and
  `alert-node_instance-transition` actions, in addition to the
  per-room alert-transitions endpoint.
